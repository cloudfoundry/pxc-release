package test_helpers

import (
	_ "context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/cloudfoundry/socks5-proxy"
	"github.com/go-sql-driver/mysql"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var HttpClient = http.DefaultClient

func NewSocks5Dialer(proxyURL string, logger *log.Logger) (proxy.DialFunc, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "ssh+socks5" {
		return nil, errors.Errorf("Unexpected proxy schema %q", u.Scheme)
	}

	if len(u.Query()["private-key"]) == 0 {
		return nil, errors.New("no private key specified in proxy url")
	}

	if len(u.Query()["private-key"]) > 1 {
		return nil, errors.New("multiple private keys specified in proxy url")
	}

	privateKey := u.Query()["private-key"][0]
	userName := u.User.Username()

	sshKey, err := ioutil.ReadFile(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, `failed to load private key from disk`)
	}

	socks5Proxy := proxy.NewSocks5Proxy(proxy.NewHostKey(), logger, time.Minute)

	return socks5Proxy.Dialer(userName, string(sshKey), u.Host)
}

func SetupSocks5Proxy() {
	dialer, err := NewSocks5Dialer(
		os.Getenv("BOSH_ALL_PROXY"),
		log.New(ginkgo.GinkgoWriter, "[socks5proxy] ", log.LstdFlags),
	)
	Expect(err).NotTo(HaveOccurred())
	CA, _ := GetDeploymentCAByName("cf_mysql_mysql_galera_healthcheck_endpoint_tls")

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM([]byte(CA)); !ok {
		// TODO: should we handle the failure parsing a CA?
	}

	HttpClient = &http.Client{
		Transport: &http.Transport{
			Dial: dialer,
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				ServerName: "galera_healthcheck_endpoint_tls",
			},
		},
		Timeout: 2 * time.Minute,
	}

	mysql.RegisterDial("tcp", func(addr string) (net.Conn, error) {
		return dialer("tcp", addr)
	})
}
