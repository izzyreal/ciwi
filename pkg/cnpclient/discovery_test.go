package cnpclient

import (
	"net"
	"testing"
)

func TestEndpointFromEntry(t *testing.T) {
	endpoint, ok := endpointFromEntry(&discoveryEntry{
		Name: "ciwi-buildbox", AddrIPv4: []net.IP{net.ParseIP("192.0.2.8")}, Port: 8113,
		InfoFields: []string{"name=ciwi", "api_version=1", "version=v0.2.0"},
	}, TransportQUIC)
	if !ok {
		t.Fatal("entry was rejected")
	}
	if endpoint.Address != "192.0.2.8:8113" || endpoint.Transport != TransportQUIC || endpoint.Version != "v0.2.0" || endpoint.APIVersion != "1" {
		t.Fatalf("endpoint = %#v", endpoint)
	}
}

func TestEndpointFromTCPEntry(t *testing.T) {
	endpoint, ok := endpointFromEntry(&discoveryEntry{
		Name: "ciwi-buildbox", Host: "buildbox.local.", Port: 8113,
	}, TransportTCP)
	if !ok {
		t.Fatal("entry was rejected")
	}
	if endpoint.Transport != TransportTCP || endpoint.Address != "buildbox.local:8113" || endpoint.Target().String() != "tcp://buildbox.local:8113" {
		t.Fatalf("endpoint = %#v", endpoint)
	}
}

func TestEndpointsFromMultihomedEntryPreferHostname(t *testing.T) {
	entry := &discoveryEntry{
		Name: "ciwi-bhakti", Host: "bhakti.local.",
		AddrIPv4: []net.IP{
			net.ParseIP("192.168.1.235"),
			net.ParseIP("192.168.1.238"),
			net.ParseIP("192.168.56.1"),
		},
		Port: 8113,
	}
	endpoints := endpointsFromEntry(entry, TransportTCP)
	if len(endpoints) != 4 {
		t.Fatalf("endpoints = %#v", endpoints)
	}
	want := []string{
		"tcp://bhakti.local:8113",
		"tcp://192.168.1.235:8113",
		"tcp://192.168.1.238:8113",
		"tcp://192.168.56.1:8113",
	}
	for i := range want {
		if endpoints[i].Target().String() != want[i] {
			t.Fatalf("endpoint %d = %#v, want %q", i, endpoints[i], want[i])
		}
	}
	if endpoints[0].Target().String() != "tcp://bhakti.local:8113" {
		t.Fatalf("endpoints = %#v", endpoints)
	}
}

func TestEndpointsFromBareMDNSHostnameUseLocalDomain(t *testing.T) {
	endpoints := endpointsFromEntry(&discoveryEntry{
		Name: "ciwi-bhakti", Host: "bhakti.",
		AddrIPv4: []net.IP{net.ParseIP("192.168.56.1")}, Port: 8113,
	}, TransportTCP)
	if len(endpoints) != 2 || endpoints[0].Target().String() != "tcp://bhakti.local:8113" {
		t.Fatalf("endpoints = %#v", endpoints)
	}
}

func TestUniqueEndpoints(t *testing.T) {
	endpoints := uniqueEndpoints([]Endpoint{
		{Name: "tcp", Address: "192.0.2.8:8113", Transport: TransportTCP},
		{Name: "z", Address: "192.0.2.8:8113", Transport: TransportQUIC},
		{Name: "a", Address: "192.0.2.9:8113"},
		{Name: "duplicate", Address: "192.0.2.8:8113", Transport: TransportQUIC},
	})
	if len(endpoints) != 3 || endpoints[0].Name != "a" || endpoints[1].Name != "z" || endpoints[2].Name != "tcp" {
		t.Fatalf("endpoints = %#v", endpoints)
	}
}

func TestUniqueEndpointsTryHostnameTransportsBeforeAddressFallbacks(t *testing.T) {
	endpoints := uniqueEndpoints([]Endpoint{
		{Name: "ciwi", Host: "192.168.56.1", Address: "192.168.56.1:8113", Transport: TransportQUIC},
		{Name: "ciwi", Host: "192.168.56.1", Address: "192.168.56.1:8113", Transport: TransportTCP},
		{Name: "ciwi", Host: "bhakti.local", Address: "bhakti.local:8113", Transport: TransportTCP},
		{Name: "ciwi", Host: "bhakti.local", Address: "bhakti.local:8113", Transport: TransportQUIC},
	})
	want := []string{
		"quic://bhakti.local:8113",
		"tcp://bhakti.local:8113",
		"quic://192.168.56.1:8113",
		"tcp://192.168.56.1:8113",
	}
	if len(endpoints) != len(want) {
		t.Fatalf("endpoints = %#v", endpoints)
	}
	for i := range want {
		if got := endpoints[i].Target().String(); got != want[i] {
			t.Fatalf("endpoint %d = %q, want %q (%#v)", i, got, want[i], endpoints)
		}
	}
}
