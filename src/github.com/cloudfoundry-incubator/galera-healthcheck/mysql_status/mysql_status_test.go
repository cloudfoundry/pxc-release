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
		monitConfig config.MonitConfig
		ts          *httptest.Server
	)

	BeforeEach(func() {
		logger := lagertest.NewTestLogger("mysql_status")

		ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			xmlFile, _ := os.Open("example_status.xml")
			xmlStatus := make([]byte, 20000)
			_, _ = xmlFile.Read(xmlStatus)
			fmt.Fprintln(w, string(xmlStatus))
		}))

		testHost, testPort := SplitHostandPort(ts.URL)
		testPortInt, _ := strconv.Atoi(testPort)

		monitConfig = config.MonitConfig{
			User:     "fake-user",
			Password: "fake-password",
			Host:     testHost,
			Port:     testPortInt,
		}
		mysql_status.New(monitConfig, logger)
	})

	Context("When mariadb_ctrl is running", func() {

		It("returns status with running", func() {
			st, err := mysql_status.MySQLStatusHandler(monitConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(Equal("unknown"))
		})
	})

	AfterEach(func() {
		defer ts.Close()
	})
})

func SplitHostandPort(url string) (string, string) {
	urlparts := strings.Split(url, ":")
	urlparts[1] = strings.TrimPrefix(urlparts[1], "//")
	return urlparts[1], urlparts[2]
}
