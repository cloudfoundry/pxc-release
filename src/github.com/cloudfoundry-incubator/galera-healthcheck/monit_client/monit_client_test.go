package monit_client_test

import (
	"fmt"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
)

var stateFile *os.File

var _ = Describe("MonitClient", func() {

	var (
		MonitClient monit_client.MonitClient
		ts          *httptest.Server
	)

	BeforeEach(func() {
		stateFile, _ = ioutil.TempFile(os.TempDir(), "stateFile")
		stateFile.Chmod(0777)
	})

	Context("stop mysql service from monit API", func() {

		It("returns http response non-200 and process has not stopped", func() {
			failingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
			MonitClient, ts = NewTestConfig(failingHandler)
			st, err := MonitClient.StopService()
			Expect(err).To(HaveOccurred())
			Expect(st).To(BeFalse())
		})

		It("returns http response 200 and process has stopped", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "running - unmonitor pending")
			})
			MonitClient, ts = NewTestConfig(successHandler)
			st, err := MonitClient.StopService()
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(BeTrue())
		})

		It("returns http response 200 and process has not stopped", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "running")
			})
			MonitClient, ts = NewTestConfig(successHandler)
			st, err := MonitClient.StopService()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmonitor"))
			Expect(st).To(BeFalse())
		})

	})

	Context("start mysql service from monit API", func() {

		It("returns http response non-200 and process has not started", func() {
			failingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
			MonitClient, ts = NewTestConfig(failingHandler)
			st, err := MonitClient.StartService("join")
			Expect(err).To(HaveOccurred())
			Expect(st).To(BeFalse())
		})

		It("returns http response 200 and process has started", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "not monitored - monitor pending")
			})
			MonitClient, ts = NewTestConfig(successHandler)
			st, err := MonitClient.StartService("join")
			Expect(err).ToNot(HaveOccurred())
			Expect(st).To(BeTrue())
		})

		It("returns http response 200 and process has not started", func() {
			successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "not monitored")
			})
			MonitClient, ts = NewTestConfig(successHandler)
			st, err := MonitClient.StartService("join")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to monitor"))
			Expect(st).To(BeFalse())
		})

	})

	AfterEach(func() {
		ts.Close()
		os.Remove(stateFile.Name())
	})
})

func NewTestConfig(handler http.HandlerFunc) (monit_client.MonitClient, *httptest.Server) {

	ts := httptest.NewServer(handler)

	logger := lagertest.NewTestLogger("monit_client")
	testHost, testPort := splitHostandPort(ts.URL)
	monitConfig := config.MonitConfig{
		User:               "fake-user",
		Password:           "fake-password",
		Host:               testHost,
		Port:               testPort,
		MysqlStateFilePath: stateFile.Name(),
	}

	return monit_client.New(monitConfig, logger, "mariadb_ctrl"), ts
}

func splitHostandPort(url string) (string, int) {
	urlparts := strings.Split(url, ":")
	host := strings.TrimPrefix(urlparts[1], "//")
	port, _ := strconv.Atoi(urlparts[2])
	return host, port
}
