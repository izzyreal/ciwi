package cnpclient

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var mdnsIPv4Address = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

type mdnsPacket struct {
	message *dns.Msg
	source  net.IP
}

func collectMDNSServiceRecords(ctx context.Context, service string, timeout time.Duration) ([]*discoveryEntry, error) {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	unicast, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, fmt.Errorf("listen for unicast mDNS responses: %w", err)
	}
	defer unicast.Close()
	multicast, err := net.ListenMulticastUDP("udp4", nil, mdnsIPv4Address)
	if err != nil {
		return nil, fmt.Errorf("listen for multicast mDNS responses: %w", err)
	}
	defer multicast.Close()
	packets := make(chan mdnsPacket, 64)
	go readMDNSPackets(queryCtx, unicast, packets)
	go readMDNSPackets(queryCtx, multicast, packets)

	serviceName := canonicalDNSName(service + ".local")
	query := new(dns.Msg)
	query.SetQuestion(serviceName, dns.TypePTR)
	query.RecursionDesired = false
	payload, err := query.Pack()
	if err != nil {
		return nil, fmt.Errorf("encode mDNS service query: %w", err)
	}
	if _, err := unicast.WriteToUDP(payload, mdnsIPv4Address); err != nil {
		return nil, fmt.Errorf("send mDNS service query: %w", err)
	}

	accumulator := newMDNSRecordAccumulator(serviceName)
	for {
		select {
		case packet := <-packets:
			accumulator.add(packet.message, packet.source)
		case <-queryCtx.Done():
			return accumulator.entries(), ctx.Err()
		}
	}
}

func readMDNSPackets(ctx context.Context, connection *net.UDPConn, packets chan<- mdnsPacket) {
	buffer := make([]byte, 65536)
	for {
		n, source, err := connection.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		message := new(dns.Msg)
		if err := message.Unpack(buffer[:n]); err != nil {
			continue
		}
		select {
		case packets <- mdnsPacket{message: message, source: source.IP}:
		case <-ctx.Done():
			return
		}
	}
}

type mdnsRecordAccumulator struct {
	serviceName   string
	byInstance    map[string]*discoveryEntry
	addressesV4   map[string][]net.IP
	addressesV6   map[string][]net.IP
	sourceByEntry map[string][]net.IP
}

func newMDNSRecordAccumulator(serviceName string) *mdnsRecordAccumulator {
	return &mdnsRecordAccumulator{
		serviceName: canonicalDNSName(serviceName), byInstance: map[string]*discoveryEntry{},
		addressesV4: map[string][]net.IP{}, addressesV6: map[string][]net.IP{}, sourceByEntry: map[string][]net.IP{},
	}
}

func (a *mdnsRecordAccumulator) add(message *dns.Msg, source net.IP) {
	if message == nil {
		return
	}
	touched := map[string]struct{}{}
	sections := append(append(append([]dns.RR{}, message.Answer...), message.Ns...), message.Extra...)
	for _, answer := range sections {
		switch record := answer.(type) {
		case *dns.PTR:
			if canonicalDNSName(record.Hdr.Name) != a.serviceName {
				continue
			}
			key := canonicalDNSName(record.Ptr)
			a.ensureEntry(key)
			touched[key] = struct{}{}
		case *dns.SRV:
			key := canonicalDNSName(record.Hdr.Name)
			if !a.isServiceInstance(key) {
				continue
			}
			entry := a.ensureEntry(key)
			entry.Host = record.Target
			entry.Port = int(record.Port)
			touched[key] = struct{}{}
		case *dns.TXT:
			key := canonicalDNSName(record.Hdr.Name)
			if !a.isServiceInstance(key) {
				continue
			}
			entry := a.ensureEntry(key)
			entry.InfoFields = append([]string(nil), record.Txt...)
			touched[key] = struct{}{}
		case *dns.A:
			host := canonicalDNSName(record.Hdr.Name)
			a.addressesV4[host] = compactIPs(append(a.addressesV4[host], record.A)...)
		case *dns.AAAA:
			host := canonicalDNSName(record.Hdr.Name)
			a.addressesV6[host] = compactIPs(append(a.addressesV6[host], record.AAAA)...)
		}
	}
	for key := range touched {
		if source != nil {
			a.sourceByEntry[key] = compactIPs(append(a.sourceByEntry[key], source)...)
		}
	}
}

func (a *mdnsRecordAccumulator) ensureEntry(key string) *discoveryEntry {
	if entry := a.byInstance[key]; entry != nil {
		return entry
	}
	entry := &discoveryEntry{Name: strings.TrimSuffix(key, ".")}
	a.byInstance[key] = entry
	return entry
}

func (a *mdnsRecordAccumulator) isServiceInstance(name string) bool {
	return strings.HasSuffix(name, "."+a.serviceName)
}

func (a *mdnsRecordAccumulator) entries() []*discoveryEntry {
	keys := make([]string, 0, len(a.byInstance))
	for key := range a.byInstance {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*discoveryEntry, 0, len(keys))
	for _, key := range keys {
		entry := a.byInstance[key]
		if entry == nil || entry.Port <= 0 {
			continue
		}
		host := canonicalDNSName(entry.Host)
		entry.AddrIPv4 = compactIPs(append(append(entry.AddrIPv4, a.addressesV4[host]...), a.sourceByEntry[key]...)...)
		entry.AddrIPv6 = compactIPs(append(entry.AddrIPv6, a.addressesV6[host]...)...)
		out = append(out, entry)
	}
	return out
}

func canonicalDNSName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}
