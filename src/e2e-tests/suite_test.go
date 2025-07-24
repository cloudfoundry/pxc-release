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

var (
	httpClient           *http.Client
	expectedMysqlVersion string
)

func TestE2E(t *testing.T) {
	expectedMysqlVersion = "8.4"
	if envMysqlVersion := os.Getenv("MYSQL_VERSION"); envMysqlVersion != "" {
		expectedMysqlVersion = envMysqlVersion
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC E2E Tests")
}

var _ = BeforeSuite(func() {
	GinkgoT().Setenv("MYSQL_VERSION", expectedMysqlVersion)

	var missingEnvs []string
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

	if _, ok := os.LookupEnv("PXC_TEST_azs"); !ok {
		GinkgoT().Setenv("PXC_TEST_azs", "[z1,z2,z3]")
	}

	if _, ok := os.LookupEnv("PXC_TEST_network"); !ok {
		GinkgoT().Setenv("PXC_TEST_network", "default")
	}

	if _, ok := os.LookupEnv("PXC_TEST_vm_type"); !ok {
		GinkgoT().Setenv("PXC_TEST_vm_type", "small")
	}

	GinkgoWriter.Println("Using PXC_TEST_azs=" + os.Getenv("PXC_TEST_azs"))
	GinkgoWriter.Println("Using PXC_TEST_network=" + os.Getenv("PXC_TEST_network"))
	GinkgoWriter.Println("Using PXC_TEST_vm_type=" + os.Getenv("PXC_TEST_vm_type"))

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
