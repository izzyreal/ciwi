package cnpclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

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

func TestDialSSHRequiresExplicitHostKeyTrust(t *testing.T) {
	privateKey, _, err := GenerateSSHDeviceKey("ciwi-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"", "SHA256:not-the-server-key"} {
		address, fingerprint := startSSHHandshakeServer(t, false)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, dialErr := DialSSH(ctx, SSHConfig{
			JumpAddress: address, Username: "ciwi", Destination: "127.0.0.1:8113",
			PrivateKeyPEM: privateKey, HostKeyFingerprint: expected,
		}, "ciwi-test", "test")
		cancel()
		var hostKeyErr *SSHHostKeyError
		if !errors.As(dialErr, &hostKeyErr) {
			t.Fatalf("expected SSHHostKeyError, got %v", dialErr)
		}
		if hostKeyErr.Fingerprint != fingerprint || hostKeyErr.Expected != expected || hostKeyErr.Address != address {
			t.Fatalf("host key error = %#v", hostKeyErr)
		}
	}
}

func TestDialSSHReportsAuthenticationFailureAfterTrust(t *testing.T) {
	address, fingerprint := startSSHHandshakeServer(t, true)
	privateKey, _, err := GenerateSSHDeviceKey("ciwi-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = DialSSH(ctx, SSHConfig{
		JumpAddress: address, Username: "ciwi", Destination: "127.0.0.1:8113",
		PrivateKeyPEM: privateKey, HostKeyFingerprint: fingerprint,
	}, "ciwi-test", "test")
	if err == nil || !strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("authentication error = %v", err)
	}
	var hostKeyErr *SSHHostKeyError
	if errors.As(err, &hostKeyErr) {
		t.Fatalf("trusted key was reported as untrusted: %v", err)
	}
}

func TestDialSSHValidatesConfigurationBeforeNetwork(t *testing.T) {
	privateKey, _, err := GenerateSSHDeviceKey("ciwi-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, config := range []SSHConfig{
		{Username: "ciwi", Destination: "server:8113", PrivateKeyPEM: privateKey},
		{JumpAddress: "jump:22", Destination: "server:8113", PrivateKeyPEM: privateKey},
		{JumpAddress: "jump:22", Username: "ciwi", PrivateKeyPEM: privateKey},
		{JumpAddress: "jump:22", Username: "ciwi", Destination: "server:8113", PrivateKeyPEM: []byte("invalid")},
	} {
		if _, err := DialSSH(context.Background(), config, "ciwi-test", "test"); err == nil {
			t.Fatalf("configuration %#v unexpectedly succeeded", config)
		}
	}
}

func TestDialSSHCancellationInterruptsStalledHandshake(t *testing.T) {
	privateKey, _, err := GenerateSSHDeviceKey("ciwi-test")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, dialErr := DialSSH(ctx, SSHConfig{
			JumpAddress: listener.Addr().String(), Username: "ciwi", Destination: "127.0.0.1:8113",
			PrivateKeyPEM: privateKey, HostKeyFingerprint: "SHA256:test",
		}, "ciwi-test", "test")
		result <- dialErr
	}()
	var serverConnection net.Conn
	select {
	case serverConnection = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("SSH test connection was not accepted")
	}
	t.Cleanup(func() { _ = serverConnection.Close() })
	cancel()
	select {
	case dialErr := <-result:
		if dialErr == nil {
			t.Fatal("cancelled SSH handshake unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled SSH handshake did not stop promptly")
	}
}

func startSSHHandshakeServer(t *testing.T, rejectClientKey bool) (string, string) {
	t.Helper()
	_, hostPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: !rejectClientKey}
	if rejectClientKey {
		config.PublicKeyCallback = func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, fmt.Errorf("test key rejected")
		}
	}
	config.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		serverConnection, channels, requests, _ := ssh.NewServerConn(connection, config)
		if serverConnection != nil {
			_ = serverConnection.Close()
		}
		if channels != nil {
			for channel := range channels {
				_ = channel.Reject(ssh.Prohibited, "test server")
			}
		}
		if requests != nil {
			for request := range requests {
				if request.WantReply {
					_ = request.Reply(false, nil)
				}
			}
		}
	}()
	return listener.Addr().String(), ssh.FingerprintSHA256(hostSigner.PublicKey())
}
