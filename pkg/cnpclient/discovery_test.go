package cnpclient

import (
	"net"
	"testing"

	"github.com/hashicorp/mdns"
)

func TestEndpointFromEntry(t *testing.T) {
	endpoint, ok := endpointFromEntry(&mdns.ServiceEntry{
		Name: "ciwi-buildbox._ciwi-native._udp.local.", AddrV4: net.ParseIP("192.0.2.8"), Port: 8113,
		InfoFields: []string{"name=ciwi", "api_version=1", "version=v0.2.0"},
	})
	if !ok {
		t.Fatal("entry was rejected")
	}
	if endpoint.Address != "192.0.2.8:8113" || endpoint.Version != "v0.2.0" || endpoint.APIVersion != "1" {
		t.Fatalf("endpoint = %#v", endpoint)
	}
}

func TestUniqueEndpoints(t *testing.T) {
	endpoints := uniqueEndpoints([]Endpoint{
		{Name: "z", Address: "192.0.2.8:8113"},
		{Name: "a", Address: "192.0.2.9:8113"},
		{Name: "duplicate", Address: "192.0.2.8:8113"},
	})
	if len(endpoints) != 2 || endpoints[0].Name != "a" || endpoints[1].Name != "z" {
		t.Fatalf("endpoints = %#v", endpoints)
	}
}
