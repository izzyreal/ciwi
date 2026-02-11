package server

import (
	"log"
	"net"
	"os"
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
	service, err := mdns.NewMDNSService(instance, "_ciwi._tcp", "", "", portNum, nil, meta)
	if err != nil {
		log.Printf("mdns advertise service setup failed: %v", err)
		return func() {}
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		log.Printf("mdns advertise start failed: %v", err)
		return func() {}
	}
	log.Printf("mdns advertising enabled: service=_ciwi._tcp instance=%q port=%s", instance, port)

	return func() {
		server.Shutdown()
	}
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
