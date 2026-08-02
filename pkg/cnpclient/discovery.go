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

const DiscoveryService = "_ciwi-native._udp"

type Endpoint struct {
	Name       string
	Address    string
	Host       string
	Port       int
	Version    string
	APIVersion string
}

// Discover finds CNP endpoints advertised on the local network. Callers can
// always bypass multicast discovery and pass an explicit address to Dial.
func Discover(ctx context.Context, timeout time.Duration) ([]Endpoint, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	entries := make(chan *mdns.ServiceEntry, 64)
	params := newDiscoveryParams(timeout, entries)
	collected := make(chan []Endpoint, 1)
	go func() {
		var endpoints []Endpoint
		for entry := range entries {
			if endpoint, ok := endpointFromEntry(entry); ok {
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
	return uniqueEndpoints(endpoints), ctx.Err()
}

func newDiscoveryParams(timeout time.Duration, entries chan<- *mdns.ServiceEntry) *mdns.QueryParam {
	params := mdns.DefaultParams(DiscoveryService)
	params.Timeout = timeout
	params.Entries = entries
	// Some otherwise healthy macOS/network configurations have no IPv6
	// multicast route. The mdns package treats that send failure as fatal even
	// when IPv4 discovery is available. IPv6 responses can still be discovered;
	// this only disables sending the query over IPv6.
	params.DisableIPv6 = true
	return params
}

func endpointFromEntry(entry *mdns.ServiceEntry) (Endpoint, bool) {
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
	return Endpoint{
		Name: strings.TrimSuffix(entry.Name, "."), Address: net.JoinHostPort(host, strconv.Itoa(entry.Port)),
		Host: host, Port: entry.Port, Version: metadata["version"], APIVersion: metadata["api_version"],
	}, true
}

func uniqueEndpoints(input []Endpoint) []Endpoint {
	seen := map[string]struct{}{}
	out := make([]Endpoint, 0, len(input))
	for _, endpoint := range input {
		if _, exists := seen[endpoint.Address]; exists {
			continue
		}
		seen[endpoint.Address] = struct{}{}
		out = append(out, endpoint)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Address < out[j].Address
	})
	return out
}
