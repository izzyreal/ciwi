package cnpclient

import "testing"

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		transport Transport
		address   string
		formatted string
	}{
		{name: "bare QUIC address", value: "buildbox:8113", transport: TransportQUIC, address: "buildbox:8113", formatted: "quic://buildbox:8113"},
		{name: "explicit QUIC", value: "quic://buildbox:8113", transport: TransportQUIC, address: "buildbox:8113", formatted: "quic://buildbox:8113"},
		{name: "explicit TCP", value: "tcp://127.0.0.1:8113", transport: TransportTCP, address: "127.0.0.1:8113", formatted: "tcp://127.0.0.1:8113"},
		{name: "local shorthand", value: ":8113", transport: TransportQUIC, address: "127.0.0.1:8113", formatted: "quic://127.0.0.1:8113"},
		{name: "IPv6 TCP", value: "tcp://[2001:db8::1]:8113", transport: TransportTCP, address: "[2001:db8::1]:8113", formatted: "tcp://[2001:db8::1]:8113"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := ParseTarget(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if target.Transport != test.transport || target.Address != test.address || target.String() != test.formatted {
				t.Fatalf("target = %#v (%q), want transport=%q address=%q formatted=%q", target, target.String(), test.transport, test.address, test.formatted)
			}
		})
	}
}

func TestParseTargetRejectsInvalidEndpoints(t *testing.T) {
	for _, value := range []string{
		"", "tcp://buildbox", "http://buildbox:8113", "tcp://user@buildbox:8113",
		"tcp://buildbox:8113/path", "tcp://buildbox:8113?option=true", "buildbox",
		"tcp://buildbox:service", "tcp://buildbox:0", "tcp://buildbox:65536",
	} {
		t.Run(value, func(t *testing.T) {
			if target, err := ParseTarget(value); err == nil {
				t.Fatalf("ParseTarget(%q) = %#v, want error", value, target)
			}
		})
	}
}

func TestTargetStringDefaultsToQUIC(t *testing.T) {
	if got := (Target{Address: "buildbox:8113"}).String(); got != "quic://buildbox:8113" {
		t.Fatalf("Target.String() = %q", got)
	}
}
