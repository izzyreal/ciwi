package cnpclient

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Target struct {
	Transport Transport
	Address   string
}

func ParseTarget(value string) (Target, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Target{}, fmt.Errorf("native endpoint is required")
	}
	transport := TransportQUIC
	address := value
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return Target{}, fmt.Errorf("parse native endpoint: %w", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case string(TransportQUIC):
			transport = TransportQUIC
		case string(TransportTCP):
			transport = TransportTCP
		default:
			return Target{}, fmt.Errorf("unsupported native transport %q", parsed.Scheme)
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return Target{}, fmt.Errorf("native endpoint must contain only a transport, host, and port")
		}
		address = parsed.Host
	}
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return Target{}, fmt.Errorf("native endpoint %q must be host:port: %w", address, err)
	}
	if strings.TrimSpace(host) == "" {
		return Target{}, fmt.Errorf("native endpoint %q must include a host", address)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return Target{}, fmt.Errorf("native endpoint %q must use a numeric port from 1 through 65535", address)
	}
	return Target{Transport: transport, Address: address}, nil
}

func (t Target) String() string {
	transport := t.Transport
	if transport == "" {
		transport = TransportQUIC
	}
	return string(transport) + "://" + t.Address
}
