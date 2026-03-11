// Package sshclient provides a thin wrapper around golang.org/x/crypto/ssh
// for executing commands and writing files on remote Linux endpoints.
package sshclient

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client holds a connected SSH session to a remote host.
type Client struct {
	conn *ssh.Client
	host string
}

// Config holds SSH connection parameters.
type Config struct {
	Host       string // IP or hostname
	Port       int    // default 22
	User       string // default root
	PrivateKey string // PEM-encoded private key
	Password   string // fallback if no key
	Timeout    time.Duration
}

// Connect establishes an SSH connection to the remote host.
func Connect(cfg Config) (*Client, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	var authMethods []ssh.AuthMethod

	if cfg.PrivateKey != "" {
		normalizedKey := normalizePEMKey(cfg.PrivateKey)
		signer, err := ssh.ParsePrivateKey([]byte(normalizedKey))
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no ssh authentication method provided")
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // user-managed VPS
		Timeout:         cfg.Timeout,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	return &Client{conn: conn, host: cfg.Host}, nil
}

// Host returns the host this client is connected to.
func (c *Client) Host() string {
	return c.host
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Run executes a command on the remote host and returns combined stdout+stderr.
func (c *Client) Run(_ context.Context, cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return combined, fmt.Errorf("ssh run %q: %w — %s", cmd, err, combined)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// RunQuiet is like Run but does not treat non-zero exit as error when the
// command output is all that matters (e.g. checking if a file exists).
func (c *Client) RunQuiet(_ context.Context, cmd string) (stdout string, exitCode int, err error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	runErr := session.Run(cmd)
	output := strings.TrimSpace(outBuf.String())

	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			return output, exitErr.ExitStatus(), nil
		}
		return output, -1, fmt.Errorf("ssh run %q: %w", cmd, runErr)
	}

	return output, 0, nil
}

// WriteFile writes content to a file on the remote host via cat heredoc.
// Creates parent directories as needed. Sets the given permissions.
func (c *Client) WriteFile(_ context.Context, path, content string, mode string) error {
	// Use a random delimiter unlikely to appear in WireGuard configs.
	delimiter := "GATOR_EOF_7f3a"
	cmd := fmt.Sprintf("mkdir -p $(dirname %q) && cat > %q << '%s'\n%s\n%s\nchmod %s %q",
		path, path, delimiter, content, delimiter, mode, path)

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("ssh write file %q: %w — %s", path, err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// FileExists checks if a path exists on the remote host.
func (c *Client) FileExists(ctx context.Context, path string) (bool, error) {
	_, code, err := c.RunQuiet(ctx, fmt.Sprintf("test -e %q && echo yes", path))
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

// TestConnection verifies the SSH connection is alive and returns basic system info.
func (c *Client) TestConnection(ctx context.Context) (map[string]string, error) {
	info := make(map[string]string)

	hostname, err := c.Run(ctx, "hostname")
	if err != nil {
		return nil, fmt.Errorf("test connection: %w", err)
	}
	info["hostname"] = hostname

	os, _ := c.Run(ctx, "cat /etc/os-release 2>/dev/null | grep ^PRETTY_NAME= | cut -d'\"' -f2")
	if os != "" {
		info["os"] = os
	}

	kernel, _ := c.Run(ctx, "uname -r")
	if kernel != "" {
		info["kernel"] = kernel
	}

	return info, nil
}

// normalizePEMKey fixes common formatting issues with PEM keys pasted from
// web textareas: strips carriage returns, ensures trailing newline, and
// re-wraps lines that got concatenated.
func normalizePEMKey(key string) string {
	// Trim surrounding whitespace.
	key = strings.TrimSpace(key)
	if key == "" {
		return key
	}

	// Normalize line endings.
	key = strings.ReplaceAll(key, "\r\n", "\n")
	key = strings.ReplaceAll(key, "\r", "\n")

	// If the key has no newlines at all (everything on one line), try to
	// reconstruct the PEM structure. This happens when a textarea strips newlines.
	if !strings.Contains(key, "\n") {
		// Find the header and footer boundaries.
		headerEnd := strings.Index(key, "-----BEGIN ")
		if headerEnd >= 0 {
			// Find end of header line (the closing dashes).
			afterBegin := strings.Index(key[headerEnd:], "-----")
			if afterBegin >= 0 {
				secondDash := strings.Index(key[headerEnd+afterBegin+5:], "-----")
				if secondDash >= 0 {
					headerEndPos := headerEnd + afterBegin + 5 + secondDash + 5
					// Find the footer.
					footerStart := strings.LastIndex(key, "-----END ")
					if footerStart > headerEndPos {
						header := key[:headerEndPos]
						body := key[headerEndPos:footerStart]
						footer := key[footerStart:]

						// Re-wrap body at 64 chars (PEM standard).
						var wrapped strings.Builder
						wrapped.WriteString(header)
						wrapped.WriteString("\n")
						for i := 0; i < len(body); i += 64 {
							end := i + 64
							if end > len(body) {
								end = len(body)
							}
							chunk := strings.TrimSpace(body[i:end])
							if chunk != "" {
								wrapped.WriteString(chunk)
								wrapped.WriteString("\n")
							}
						}
						wrapped.WriteString(footer)
						wrapped.WriteString("\n")
						return wrapped.String()
					}
				}
			}
		}
	}

	// Ensure trailing newline (PEM parsers require it).
	if !strings.HasSuffix(key, "\n") {
		key += "\n"
	}

	return key
}
