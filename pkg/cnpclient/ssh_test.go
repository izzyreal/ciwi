package cnpclient

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHDeviceKeyRoundTripsAndBuildsRestrictedLine(t *testing.T) {
	privateKey, publicKey, err := GenerateSSHDeviceKey("ciwi-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParsePrivateKey(privateKey); err != nil {
		t.Fatalf("parse generated key: %v", err)
	}
	if !strings.HasPrefix(publicKey, "ssh-ed25519 ") || !strings.HasSuffix(publicKey, " ciwi-test") {
		t.Fatalf("public key = %q", publicKey)
	}
	line := RestrictedAuthorizedKey(publicKey, "10.77.77.2:8113")
	if !strings.HasPrefix(line, `restrict,port-forwarding,permitopen="10.77.77.2:8113" ssh-ed25519 `) {
		t.Fatalf("restricted key = %q", line)
	}
}

func TestNormalizedHostPortUsesDefaultsWithoutGuessingIPv6(t *testing.T) {
	if got, err := normalizedHostPort("192.0.2.1", "22"); err != nil || got != "192.0.2.1:22" {
		t.Fatalf("address = %q, %v", got, err)
	}
	if _, err := normalizedHostPort("2001:db8::1", "22"); err == nil {
		t.Fatal("expected unbracketed IPv6 to require an explicit port")
	}
}
