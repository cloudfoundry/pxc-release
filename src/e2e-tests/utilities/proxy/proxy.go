package proxy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	"golang.org/x/crypto/ssh"
)

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func NewDialerViaSSH(_ context.Context, proxyURL string) (DialContextFunc, error) {
	u, err := parseProxyURL(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	sshClientConfig, err := sshConfig(u)
	if err != nil {
		return nil, err
	}

	conn, err := ssh.Dial("tcp", u.Host, sshClientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}

	startKeepAlive(conn)

	return func(_ context.Context, network, addr string) (net.Conn, error) {
		return conn.Dial(network, addr)
	}, nil
}

func sshConfig(proxyURL *url.URL) (*ssh.ClientConfig, error) {
	signer, err := ssh.ParsePrivateKey([]byte(proxyURL.Query().Get("private-key")))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &ssh.ClientConfig{
		Timeout:         30 * time.Second,
		User:            proxyURL.User.Username(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
	}, nil
}

func parseProxyURL(proxyURL string) (*url.URL, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if u.Scheme != "ssh+socks5" {
		return nil, fmt.Errorf("unsupported schema %q", u.Scheme)
	}

	if u.User.Username() == "" {
		u.User = url.User("jumpbox")
	}

	privateKey, err := os.ReadFile(u.Query().Get("private-key"))
	if err != nil {
		return nil, fmt.Errorf("unable to read private-key file %q: %w", u.Query().Get("private-key"), err)
	}

	values := u.Query()
	values.Set("private-key", string(privateKey))
	u.RawQuery = values.Encode()

	return u, nil
}

func startKeepAlive(conn *ssh.Client) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		for {
			select {
			case <-t.C:
				_, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("error sending ssh keep-alive: %s", err)
				}
			}
		}
	}()
}
