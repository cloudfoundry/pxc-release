package monit_client_test

import (
	//"fmt"
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

var _ = Describe("GaleraStatusChecker", func() {

	var (
		monitClient *monit_client.Monit_Client
		ts          *httptest.Server
	)

	Context("stop mysql service from monit API", func() {

		It("returns http response non-200 and process has not stopped", func() {
			failingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
			monitClient, ts = NewTestConfig(failingHandler)
			st, err := monitClient.StopService()
			Expect(err).To(HaveOccurred())
			Expect(st).To(BeFalse())
		})

		It("returns http response 200 and process has stopped", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "running - unmonitor pending")
			})
			monitClient, ts = NewTestConfig(successHandler)
			st, err := monitClient.StopService()
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(BeTrue())
		})

		It("returns http response 200 and process has not stopped", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "running")
			})
			monitClient, ts = NewTestConfig(successHandler)
			st, err := monitClient.StopService()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to stop"))
			Expect(st).To(BeFalse())
		})

	})

	AfterEach(func() {
		ts.Close()
	})
})

func NewTestConfig(handler http.HandlerFunc) (*monit_client.Monit_Client, *httptest.Server) {

	ts := httptest.NewServer(handler)

	logger := lagertest.NewTestLogger("monit_client")
	testHost, testPort := splitHostandPort(ts.URL)
	monitConfig := config.MonitConfig{
		User:     "fake-user",
		Password: "fake-password",
		Host:     testHost,
		Port:     testPort,
	}

	return monit_client.New(monitConfig, logger, "mariadb_ctrl"), ts
}

func splitHostandPort(url string) (string, int) {
	urlparts := strings.Split(url, ":")
	host := strings.TrimPrefix(urlparts[1], "//")
	port, _ := strconv.Atoi(urlparts[2])
	return host, port
}
