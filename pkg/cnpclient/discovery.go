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
	transport := TransportQUIC
	if service == DiscoveryTCPService {
		transport = TransportTCP
	}
	if entries, handled, err := discoverPlatformService(ctx, service, timeout); handled {
		endpoints := make([]Endpoint, 0, len(entries))
		for _, entry := range entries {
			endpoints = append(endpoints, endpointsFromEntry(entry, transport)...)
		}
		if len(endpoints) == 0 && err != nil && ctx.Err() == nil {
			return nil, err
		}
		return endpoints, ctx.Err()
	}
	type result struct {
		entries []*discoveryEntry
		err     error
	}
	results := make(chan result, 2)
	go func() {
		entries, err := collectMDNSServiceRecords(ctx, service, timeout)
		results <- result{entries: entries, err: err}
	}()
	go func() {
		entries, err := discoverHashicorpService(ctx, service, timeout)
		results <- result{entries: entries, err: err}
	}()
	var endpoints []Endpoint
	var firstError error
	for range 2 {
		result := <-results
		for _, entry := range result.entries {
			endpoints = append(endpoints, endpointsFromEntry(entry, transport)...)
		}
		if firstError == nil && result.err != nil {
			firstError = result.err
		}
	}
	if len(endpoints) == 0 && firstError != nil && ctx.Err() == nil {
		return nil, firstError
	}
	return endpoints, ctx.Err()
}

func discoverHashicorpService(ctx context.Context, service string, timeout time.Duration) ([]*discoveryEntry, error) {
	entries := make(chan *mdns.ServiceEntry, 64)
	params := mdns.DefaultParams(service)
	params.Timeout = timeout
	params.Entries = entries
	// Some otherwise healthy macOS/network configurations have no IPv6
	// multicast route. IPv6 records can still arrive in IPv4 responses.
	params.DisableIPv6 = true
	collected := make(chan []*discoveryEntry, 1)
	go func() {
		var out []*discoveryEntry
		for entry := range entries {
			out = append(out, &discoveryEntry{
				Name: entry.Name, Host: entry.Host, Port: entry.Port, InfoFields: entry.InfoFields,
				AddrIPv4: compactIPs(entry.AddrV4),
				AddrIPv6: compactIPs(ipFromIPAddr(entry.AddrV6IPAddr), entry.AddrV6),
			})
		}
		collected <- out
	}()
	err := mdns.QueryContext(ctx, params)
	close(entries)
	out := <-collected
	if err != nil && ctx.Err() == nil {
		return out, fmt.Errorf("discover ciwi native endpoints: %w", err)
	}
	return out, ctx.Err()
}

type discoveryEntry struct {
	Name       string
	Host       string
	Port       int
	InfoFields []string
	AddrIPv4   []net.IP
	AddrIPv6   []net.IP
}

func endpointFromEntry(entry *discoveryEntry, transport Transport) (Endpoint, bool) {
	endpoints := endpointsFromEntry(entry, transport)
	if len(endpoints) == 0 {
		return Endpoint{}, false
	}
	return endpoints[0], true
}

func endpointsFromEntry(entry *discoveryEntry, transport Transport) []Endpoint {
	if entry == nil || entry.Port <= 0 {
		return nil
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
	hosts := []string{normalizeMDNSHostname(entry.Host)}
	for _, address := range append(append([]net.IP{}, entry.AddrIPv4...), entry.AddrIPv6...) {
		if address != nil {
			hosts = append(hosts, address.String())
		}
	}
	seenHosts := map[string]struct{}{}
	endpoints := make([]Endpoint, 0, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, exists := seenHosts[host]; exists {
			continue
		}
		seenHosts[host] = struct{}{}
		endpoints = append(endpoints, Endpoint{
			Name: strings.TrimSuffix(entry.Name, "."), Address: net.JoinHostPort(host, strconv.Itoa(entry.Port)), Transport: transport,
			Host: host, Port: entry.Port, Version: metadata["version"], APIVersion: metadata["api_version"],
		})
	}
	return endpoints
}

func compactIPs(addresses ...net.IP) []net.IP {
	out := make([]net.IP, 0, len(addresses))
	seen := map[string]struct{}{}
	for _, address := range addresses {
		if address == nil {
			continue
		}
		key := address.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, address)
	}
	return out
}

func ipFromIPAddr(address *net.IPAddr) net.IP {
	if address == nil {
		return nil
	}
	return address.IP
}

func normalizeMDNSHostname(host string) string {
	host = strings.Trim(strings.TrimSpace(host), ".")
	if host == "" {
		return ""
	}
	// A single-label SRV target learned through an mDNS query belongs to the
	// local multicast domain. Resolving the bare label through ordinary DNS can
	// select an unrelated search-domain or public address.
	if !strings.Contains(host, ".") {
		return host + ".local"
	}
	return host
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
		iHostname := endpointUsesHostname(out[i])
		jHostname := endpointUsesHostname(out[j])
		if iHostname != jHostname {
			return iHostname
		}
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

func endpointUsesHostname(endpoint Endpoint) bool {
	host := endpoint.Host
	if host == "" {
		parsedHost, _, err := net.SplitHostPort(endpoint.Address)
		if err != nil {
			return false
		}
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if zoneIndex := strings.LastIndex(host, "%"); zoneIndex >= 0 {
		host = host[:zoneIndex]
	}
	return net.ParseIP(host) == nil
}
