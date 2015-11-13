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

	Context("when mariadb_ctrl is running", func() {

		It("returns http response 200 and process as running", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				xmlFile, _ := os.Open("example_status.xml")
				xmlStatus := make([]byte, 20000)
				_, _ = xmlFile.Read(xmlStatus)
				fmt.Fprintln(w, string(xmlStatus))
			})

			monitStatus, ts = NewTestConfig(successHandler)
			st, err := monitStatus.MySQLStatusHandler()
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(Equal("unknown"))
		})

		It("returns non 200 http response for bad monit API request", func() {
			failingHandler := func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "something failed", http.StatusBadRequest)
			}
			monitStatus, ts = NewTestConfig(failingHandler)
			_, err := monitStatus.MySQLStatusHandler()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Received 400 response from monit"))
		})
		It("returns non 200 http response from monit API for parsing bad xml", func() {
			badXmlHandler := func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "fake-xml")
			}
			monitStatus, ts = NewTestConfig(badXmlHandler)
			_, err := monitStatus.MySQLStatusHandler()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to parse XML"))
		})
	})

	AfterEach(func() {
		defer ts.Close()
	})
})

func NewTestConfig(handler http.HandlerFunc) (*mysql_status.MySQLStatus, *httptest.Server) {
	logger := lagertest.NewTestLogger("mysql_status")

	ts := httptest.NewServer(handler)

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
