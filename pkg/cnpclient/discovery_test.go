package cnpclient

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
)

func TestEndpointFromEntry(t *testing.T) {
	endpoint, ok := endpointFromEntry(&mdns.ServiceEntry{
		Name: "ciwi-buildbox._ciwi-native._udp.local.", AddrV4: net.ParseIP("192.0.2.8"), Port: 8113,
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
	endpoint, ok := endpointFromEntry(&mdns.ServiceEntry{
		Name: "ciwi-buildbox._ciwi-native._tcp.local.", Host: "buildbox.local.", Port: 8113,
	}, TransportTCP)
	if !ok {
		t.Fatal("entry was rejected")
	}
	if endpoint.Transport != TransportTCP || endpoint.Address != "buildbox.local:8113" || endpoint.Target().String() != "tcp://buildbox.local:8113" {
		t.Fatalf("endpoint = %#v", endpoint)
	}
}

func TestDiscoveryUsesIPv4QueryWhenIPv6MulticastIsUnavailable(t *testing.T) {
	params := newDiscoveryParams(time.Second, make(chan *mdns.ServiceEntry))
	if !params.DisableIPv6 || params.DisableIPv4 {
		t.Fatalf("discovery address families: disableIPv4=%v disableIPv6=%v", params.DisableIPv4, params.DisableIPv6)
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
