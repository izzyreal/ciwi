package server

import (
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

	host, _ := os.Hostname()
	if strings.TrimSpace(host) == "" {
		host = "ciwi"
	}
	instance := strings.TrimSpace(envOrDefault("CIWI_MDNS_INSTANCE", "ciwi-"+host))
	if instance == "" {
		instance = "ciwi"
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("dns-sd"); err != nil {
			return func() {}
		}
		cmd = exec.Command("dns-sd", "-R", instance, "_ciwi._tcp", "local", port)
	case "linux":
		if _, err := exec.LookPath("avahi-publish-service"); err != nil {
			return func() {}
		}
		cmd = exec.Command("avahi-publish-service", instance, "_ciwi._tcp", port)
	default:
		return func() {}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("mdns advertise start failed: %v", err)
		return func() {}
	}
	log.Printf("mdns advertising enabled: service=_ciwi._tcp instance=%q port=%s", instance, port)

	return func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
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
