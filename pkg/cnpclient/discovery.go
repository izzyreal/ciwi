package cnpclient

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	DiscoveryQUICService = "_ciwi-native._udp"
	DiscoveryTCPService  = "_ciwi-native._tcp"
)

type Transport string

const (
	TransportQUIC Transport = "quic"
	TransportTCP  Transport = "tcp"
)

type Endpoint struct {
	Name       string
	Address    string
	Transport  Transport
	Host       string
	Port       int
	Version    string
	APIVersion string
}

func (e Endpoint) Target() Target {
	transport := e.Transport
	if transport == "" {
		transport = TransportQUIC
	}
	return Target{Transport: transport, Address: e.Address}
}

// Discover finds CNP endpoints advertised on the local network. Callers can
// always bypass multicast discovery and pass an explicit address to Dial.
func Discover(ctx context.Context, timeout time.Duration) ([]Endpoint, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	type result struct {
		endpoints []Endpoint
		err       error
	}
	results := make(chan result, 2)
	for _, service := range []string{DiscoveryQUICService, DiscoveryTCPService} {
		service := service
		go func() {
			endpoints, err := discoverService(ctx, service, timeout)
			results <- result{endpoints: endpoints, err: err}
		}()
	}
	var endpoints []Endpoint
	var firstError error
	for range 2 {
		result := <-results
		endpoints = append(endpoints, result.endpoints...)
		if firstError == nil && result.err != nil {
			firstError = result.err
		}
	}
	if len(endpoints) == 0 && firstError != nil && ctx.Err() == nil {
		return nil, firstError
	}
	return uniqueEndpoints(endpoints), ctx.Err()
}

func discoverService(ctx context.Context, service string, timeout time.Duration) ([]Endpoint, error) {
	entries := make(chan *mdns.ServiceEntry, 64)
	params := newDiscoveryParamsForService(service, timeout, entries)
	transport := TransportQUIC
	if service == DiscoveryTCPService {
		transport = TransportTCP
	}
	collected := make(chan []Endpoint, 1)
	go func() {
		var endpoints []Endpoint
		for entry := range entries {
			if endpoint, ok := endpointFromEntry(entry, transport); ok {
				endpoints = append(endpoints, endpoint)
			}
		}
		collected <- endpoints
	}()
	err := mdns.QueryContext(ctx, params)
	close(entries)
	endpoints := <-collected
	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("discover ciwi native endpoints: %w", err)
	}
	return endpoints, ctx.Err()
}

func newDiscoveryParams(timeout time.Duration, entries chan<- *mdns.ServiceEntry) *mdns.QueryParam {
	return newDiscoveryParamsForService(DiscoveryQUICService, timeout, entries)
}

func newDiscoveryParamsForService(service string, timeout time.Duration, entries chan<- *mdns.ServiceEntry) *mdns.QueryParam {
	params := mdns.DefaultParams(service)
	params.Timeout = timeout
	params.Entries = entries
	// Some otherwise healthy macOS/network configurations have no IPv6
	// multicast route. The mdns package treats that send failure as fatal even
	// when IPv4 discovery is available. IPv6 responses can still be discovered;
	// this only disables sending the query over IPv6.
	params.DisableIPv6 = true
	return params
}

func endpointFromEntry(entry *mdns.ServiceEntry, transport Transport) (Endpoint, bool) {
	if entry == nil || entry.Port <= 0 {
		return Endpoint{}, false
	}
	host := ""
	if entry.AddrV4 != nil {
		host = entry.AddrV4.String()
	} else if entry.AddrV6IPAddr != nil {
		host = entry.AddrV6IPAddr.String()
	} else if entry.AddrV6 != nil {
		host = entry.AddrV6.String()
	} else {
		host = strings.TrimSuffix(entry.Host, ".")
	}
	if host == "" {
		return Endpoint{}, false
	}
	metadata := map[string]string{}
	for _, field := range entry.InfoFields {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			metadata[key] = value
		}
	}
	if transport != TransportTCP {
		transport = TransportQUIC
	}
	return Endpoint{
		Name: strings.TrimSuffix(entry.Name, "."), Address: net.JoinHostPort(host, strconv.Itoa(entry.Port)), Transport: transport,
		Host: host, Port: entry.Port, Version: metadata["version"], APIVersion: metadata["api_version"],
	}, true
}

func uniqueEndpoints(input []Endpoint) []Endpoint {
	seen := map[string]struct{}{}
	out := make([]Endpoint, 0, len(input))
	for _, endpoint := range input {
		if endpoint.Transport == "" {
			endpoint.Transport = TransportQUIC
		}
		key := string(endpoint.Transport) + "://" + endpoint.Address
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, endpoint)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Transport != out[j].Transport {
			return out[i].Transport == TransportQUIC
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Address < out[j].Address
	})
	return out
}
