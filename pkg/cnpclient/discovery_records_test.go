package cnpclient

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestMDNSRecordAccumulatorPreservesAllAddressesAndResponseSource(t *testing.T) {
	service := "_ciwi-native._tcp.local."
	instance := "ciwi-bhakti." + service
	host := "bhakti."
	message := &dns.Msg{
		Answer: []dns.RR{&dns.PTR{Hdr: dns.RR_Header{Name: service}, Ptr: instance}},
		Extra: []dns.RR{
			&dns.SRV{Hdr: dns.RR_Header{Name: instance}, Port: 8113, Target: host},
			&dns.TXT{Hdr: dns.RR_Header{Name: instance}, Txt: []string{"version=v0.2.3", "api_version=1"}},
			&dns.A{Hdr: dns.RR_Header{Name: host}, A: net.ParseIP("192.168.1.235")},
			&dns.A{Hdr: dns.RR_Header{Name: host}, A: net.ParseIP("192.168.1.238")},
			&dns.A{Hdr: dns.RR_Header{Name: host}, A: net.ParseIP("192.168.56.1")},
			&dns.AAAA{Hdr: dns.RR_Header{Name: host}, AAAA: net.ParseIP("fd94:80cb:33e:10:3d40:b1c9:692e:ecee")},
		},
	}
	accumulator := newMDNSRecordAccumulator(service)
	accumulator.add(message, net.ParseIP("192.168.1.235"))
	entries := accumulator.entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	entry := entries[0]
	if entry.Host != host || entry.Port != 8113 || len(entry.AddrIPv4) != 3 || len(entry.AddrIPv6) != 1 {
		t.Fatalf("entry = %#v", entry)
	}
	wantV4 := []string{"192.168.1.235", "192.168.1.238", "192.168.56.1"}
	for index, want := range wantV4 {
		if got := entry.AddrIPv4[index].String(); got != want {
			t.Fatalf("IPv4 address %d = %q, want %q", index, got, want)
		}
	}
}
