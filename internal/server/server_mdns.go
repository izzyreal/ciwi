package server

import (
	"log/slog"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/mdns"
)

func startMDNSAdvertiser(serverAddr string) func() {
	if strings.TrimSpace(envOrDefault("CIWI_MDNS_ENABLE", "true")) == "false" {
		return func() {}
	}

	port := listenPortFromAddr(serverAddr)
	if port == "" {
		return func() {}
	}
	if _, err := strconv.Atoi(port); err != nil {
		return func() {}
	}
	portNum, _ := strconv.Atoi(port)

	host, _ := os.Hostname()
	if strings.TrimSpace(host) == "" {
		host = "ciwi"
	}
	instance := strings.TrimSpace(envOrDefault("CIWI_MDNS_INSTANCE", "ciwi-"+host))
	if instance == "" {
		instance = "ciwi"
	}

	meta := []string{
		"name=ciwi",
		"api_version=1",
		"version=" + currentVersion(),
	}
	ips := discoverAdvertiseIPs()
	service, err := mdns.NewMDNSService(instance, "_ciwi._tcp", "", "", portNum, ips, meta)
	if err != nil {
		slog.Error("mdns advertise service setup failed", "error", err)
		return func() {}
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		slog.Error("mdns advertise start failed", "error", err)
		return func() {}
	}
	slog.Info("mdns advertising enabled", "service", "_ciwi._tcp", "instance", instance, "port", port)

	return func() {
		server.Shutdown()
	}
}

func discoverAdvertiseIPs() []net.IP {
	ifAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return filterAdvertiseIPs(ifAddrs)
}

func filterAdvertiseIPs(addrs []net.Addr) []net.IP {
	if len(addrs) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet == nil || ipNet.IP == nil {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.IsUnspecified() {
			continue
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		normalized := ip.To16()
		if normalized == nil {
			continue
		}
		key := normalized.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		ai := out[i].To4() != nil
		aj := out[j].To4() != nil
		if ai != aj {
			return ai
		}
		return out[i].String() < out[j].String()
	})
	return out
}

func listenPortFromAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "8112"
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}
	if strings.Count(addr, ":") == 0 {
		return addr
	}
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return p
}
