package proxy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func NewDialer(proxyURL string) (DialContextFunc, error) {
	GinkgoHelper()
	u, err := parseProxyURL(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if u.Scheme == "socks5" {
		GinkgoWriter.Println()
		GinkgoWriter.Println("Setting up proxy access via direct socks5:", proxyURL)
		GinkgoWriter.Println()

		dialer, err := proxy.SOCKS5("tcp", u.Host, nil, proxy.Direct)
		Expect(err).NotTo(HaveOccurred())

		return func(_ context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}, nil
	}

	if u.Scheme != "ssh+socks5" {
		return nil, fmt.Errorf("unsupported BOSH_ALL_PROXY scheme: %s", u.Scheme)
	}

	GinkgoWriter.Println()
	GinkgoWriter.Println("Setting up proxy access via ssh+socks5:", proxyURL)
	GinkgoWriter.Println()

	sshClientConfig, err := sshConfig(u)
	if err != nil {
		return nil, err
	}

	conn, err := ssh.Dial("tcp", u.Host, sshClientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}

	startKeepAlive(conn)

	return conn.DialContext, nil
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

	if u.Scheme != "ssh+socks5" && u.Scheme != "socks5" {
		return nil, fmt.Errorf("unsupported schema %q", u.Scheme)
	}

	if privateKeyPath := u.Query().Get("private-key"); privateKeyPath != "" {
		privateKey, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read private-key file %q: %w", privateKeyPath, err)
		}

		values := u.Query()
		values.Set("private-key", string(privateKey))
		u.RawQuery = values.Encode()
	}

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
					GinkgoWriter.Printf("error sending ssh keep-alive: %s", err)
				}
			}
		}
	}()
}
