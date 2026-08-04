package cnpclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHConfig struct {
	JumpAddress        string
	Username           string
	Destination        string
	PrivateKeyPEM      []byte
	HostKeyFingerprint string
}

type SSHHostKeyError struct {
	Address     string
	Fingerprint string
	Expected    string
}

func (e *SSHHostKeyError) Error() string {
	if e.Expected == "" {
		return fmt.Sprintf("SSH host %s presented untrusted key %s", e.Address, e.Fingerprint)
	}
	return fmt.Sprintf("SSH host key changed for %s: expected %s, received %s", e.Address, e.Expected, e.Fingerprint)
}

func GenerateSSHDeviceKey(comment string) (privateKeyPEM []byte, authorizedPublicKey string, err error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generate Ed25519 key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "ciwi native client")
	if err != nil {
		return nil, "", fmt.Errorf("encode Ed25519 private key: %w", err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		return nil, "", fmt.Errorf("encode Ed25519 public key: %w", err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))
	if comment = strings.TrimSpace(comment); comment != "" {
		line += " " + strings.ReplaceAll(comment, "\n", " ")
	}
	return pem.EncodeToMemory(block), line, nil
}

func RestrictedAuthorizedKey(publicKey, destination string) string {
	publicKey = strings.TrimSpace(publicKey)
	destination = strings.TrimSpace(destination)
	if publicKey == "" || destination == "" {
		return ""
	}
	return fmt.Sprintf("restrict,port-forwarding,permitopen=%q %s", destination, publicKey)
}

func DialSSH(ctx context.Context, config SSHConfig, clientName, clientVersion string) (*Client, error) {
	jumpAddress, err := normalizedHostPort(config.JumpAddress, "22")
	if err != nil {
		return nil, fmt.Errorf("invalid SSH jump host: %w", err)
	}
	destination, err := normalizedHostPort(config.Destination, "8113")
	if err != nil {
		return nil, fmt.Errorf("invalid SSH destination: %w", err)
	}
	username := strings.TrimSpace(config.Username)
	if username == "" {
		return nil, fmt.Errorf("SSH username is required")
	}
	signer, err := ssh.ParsePrivateKey(config.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse SSH device key: %w", err)
	}
	expectedFingerprint := strings.TrimSpace(config.HostKeyFingerprint)
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fingerprint := ssh.FingerprintSHA256(key)
			if expectedFingerprint == "" || fingerprint != expectedFingerprint {
				return &SSHHostKeyError{Address: jumpAddress, Fingerprint: fingerprint, Expected: expectedFingerprint}
			}
			return nil
		},
		Timeout: 10 * time.Second,
	}
	connection, err := (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", jumpAddress)
	if err != nil {
		return nil, fmt.Errorf("dial SSH jump host: %w", err)
	}
	sshConnection, channels, requests, err := ssh.NewClientConn(connection, jumpAddress, sshConfig)
	if err != nil {
		_ = connection.Close()
		var hostKeyErr *SSHHostKeyError
		if errors.As(err, &hostKeyErr) {
			return nil, hostKeyErr
		}
		return nil, fmt.Errorf("connect SSH jump host: %w", err)
	}
	sshClient := ssh.NewClient(sshConnection, channels, requests)
	tunnel, err := sshClient.Dial("tcp", destination)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("open SSH route to %s: %w", destination, err)
	}
	client, err := DialTCPConn(ctx, &sshTunnelConn{Conn: tunnel, client: sshClient}, clientName, clientVersion)
	if err != nil {
		_ = tunnel.Close()
		_ = sshClient.Close()
		return nil, err
	}
	return client, nil
}

type sshTunnelConn struct {
	net.Conn
	client *ssh.Client
}

func (c *sshTunnelConn) Close() error {
	streamErr := c.Conn.Close()
	clientErr := c.client.Close()
	return errors.Join(streamErr, clientErr)
}

func normalizedHostPort(value, defaultPort string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("address is required")
	}
	if _, _, err := net.SplitHostPort(value); err == nil {
		return value, nil
	}
	if strings.Contains(value, ":") {
		return "", fmt.Errorf("%q must be host:port", value)
	}
	return net.JoinHostPort(value, defaultPort), nil
}
