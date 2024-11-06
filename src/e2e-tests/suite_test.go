package e2e_tests

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/proxy"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC E2E Tests")
}

var (
	httpClient           *http.Client
	expectedMysqlVersion string
)

var _ = BeforeSuite(func() {
	var missingEnvs []string
	expectedMysqlVersion = "8.0"
	if envMysqlVersion := os.Getenv("MYSQL_VERSION"); envMysqlVersion != "" {
		expectedMysqlVersion = envMysqlVersion
	}

	for _, v := range []string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"CREDHUB_SERVER",
		"CREDHUB_CLIENT",
		"CREDHUB_SECRET",
	} {
		if os.Getenv(v) == "" {
			missingEnvs = append(missingEnvs, v)
		}
	}
	Expect(missingEnvs).To(BeEmpty(), "Missing environment variables: %s", strings.Join(missingEnvs, ", "))

	if proxySpec := os.Getenv("BOSH_ALL_PROXY"); proxySpec != "" {
		var err error
		proxyDialer, err := proxy.NewDialer(proxySpec)
		Expect(err).NotTo(HaveOccurred())

		mysql.RegisterDialContext("tcp", func(ctx context.Context, addr string) (net.Conn, error) {
			return proxyDialer(ctx, "tcp", addr)
		})

		httpClient = &http.Client{
			Transport: &http.Transport{
				DialContext: proxyDialer,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}
})
