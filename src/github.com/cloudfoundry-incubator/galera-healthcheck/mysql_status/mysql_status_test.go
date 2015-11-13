package mysql_status_test

import (
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysql_status"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
)

var _ = Describe("GaleraStatusChecker", func() {

	var (
		monitStatus *mysql_status.MySQLStatus
		ts          *httptest.Server
	)

	BeforeEach(func() {
		monitStatus, ts = NewTestConfig()
	})

	Context("When mariadb_ctrl is running", func() {

		It("returns status with running", func() {
			st, err := monitStatus.MySQLStatusHandler()
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(Equal("unknown"))
		})
	})

	AfterEach(func() {
		defer ts.Close()
	})
})

func NewTestConfig() (*mysql_status.MySQLStatus, *httptest.Server) {
	logger := lagertest.NewTestLogger("mysql_status")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xmlFile, _ := os.Open("example_status.xml")
		xmlStatus := make([]byte, 20000)
		_, _ = xmlFile.Read(xmlStatus)
		fmt.Fprintln(w, string(xmlStatus))
	}))

	testHost, testPort := splitHostandPort(ts.URL)
	monitConfig := config.MonitConfig{
		User:     "fake-user",
		Password: "fake-password",
		Host:     testHost,
		Port:     testPort,
	}
	return mysql_status.New(monitConfig, logger), ts
}

func splitHostandPort(url string) (string, int) {
	urlparts := strings.Split(url, ":")
	host := strings.TrimPrefix(urlparts[1], "//")
	port, _ := strconv.Atoi(urlparts[2])
	return host, port
}
