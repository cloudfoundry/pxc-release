package monit_client_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var stateFile *os.File

var _ = Describe("monitClient", func() {

	var (
		monitClient monit_client.MonitClient
		ts          *httptest.Server
		logger      lager.Logger
		fakeHandler http.HandlerFunc
		processName string
	)

	BeforeEach(func() {
		stateFile, _ = ioutil.TempFile(os.TempDir(), "stateFile")
		stateFile.Chmod(0777)

		fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		processName = "mariadb_ctrl"
	})

	JustBeforeEach(func() {
		ts = httptest.NewServer(fakeHandler)
		testHost, testPort := splitHostandPort(ts.URL)
		monitConfig := config.MonitConfig{
			User:               "fake-user",
			Password:           "fake-password",
			Host:               testHost,
			Port:               testPort,
			MysqlStateFilePath: stateFile.Name(),
		}

		logger = lagertest.NewTestLogger("monit_client")

		monitClient = monit_client.New(monitConfig, logger, processName)
	})

	AfterEach(func() {
		ts.Close()
		os.Remove(stateFile.Name())
	})

	Describe("StopService", func() {

		Context("when monit returns successful stop response", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "running - unmonitor pending")
				})
			})

			It("returns http response 200 and process has stopped", func() {
				st, err := monitClient.StopService()
				Expect(err).ToNot(HaveOccurred())
				Expect(st).To(BeTrue())
			})
		})

		Context("when monit returns 500 error", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintln(w, "fake-internal-error")
				})
			})

			It("returns http response non-200 and process has not stopped", func() {
				st, err := monitClient.StopService()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-internal-error"))
				Expect(st).To(BeFalse())
			})
		})

		Context("when monit returns 200 response, but process is still running", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "running")
				})
			})

			It("returns http response 200 and process has not stopped", func() {
				st, err := monitClient.StopService()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmonitor"))
				Expect(st).To(BeFalse())
			})
		})
	})

	Describe("StartService", func() {

		Context("when monit returns 500 error", func() {

			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintln(w, "fake-internal-error")
				})
			})

			It("returns http response non-200 and process has not started", func() {
				st, err := monitClient.StartService("join")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-internal-error"))
				Expect(st).To(BeFalse())
			})
		})

		Context("when monit returns successful starting response", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "not monitored - monitor pending")
				})
			})

			It("returns http response 200 and process has started", func() {
				st, err := monitClient.StartService("join")
				Expect(err).ToNot(HaveOccurred())
				Expect(st).To(BeTrue())
			})
		})

		Context("when monit returns 200 response, but process is still unstarted", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "not monitored")
				})
			})

			It("returns http response 200 and process has not started", func() {
				st, err := monitClient.StartService("join")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to monitor"))
				Expect(st).To(BeFalse())
			})
		})
	})

	Describe("Status", func() {

		Context("when monit returns a valid XML response", func() {
			BeforeEach(func() {
				fixture := getRelativeFile("fixtures/monit_status.xml")
				xmlFile, err := os.Open(fixture)
				Expect(err).ToNot(HaveOccurred())
				defer xmlFile.Close()

				xmlContents, err := ioutil.ReadAll(xmlFile)
				Expect(err).ToNot(HaveOccurred())

				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write(xmlContents)
				})
			})

			Context("and process is running", func() {

				BeforeEach(func() {
					processName = "running_process"
				})

				It("returns running", func() {
					stat, err := monitClient.GetStatus()
					Expect(err).ToNot(HaveOccurred())
					Expect(stat).To(Equal("running"))
				})
			})

			Context("and process is stopped", func() {
				BeforeEach(func() {
					processName = "unmonitored_process"
				})

				It("returns stopped", func() {
					stat, err := monitClient.GetStatus()
					Expect(err).ToNot(HaveOccurred())
					Expect(stat).To(Equal("stopped"))
				})
			})

			Context("and process is failing", func() {
				BeforeEach(func() {
					processName = "failing_process"
				})
				It("returns failing", func() {
					stat, err := monitClient.GetStatus()
					Expect(err).ToNot(HaveOccurred())
					Expect(stat).To(Equal("failing"))
				})
			})

			Context("and process name is not found", func() {
				BeforeEach(func() {
					processName = "nonexistent_process"
				})
				It("returns an error", func() {
					_, err := monitClient.GetStatus()
					Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Could not find process %s", processName)))
				})
			})
		})

		Context("when monit returns invalid XML", func() {
			BeforeEach(func() {
				fakeHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "not-valid-xml")
				})
			})

			It("returns an error", func() {
				_, err := monitClient.GetStatus()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to unmarshal the xml"))
			})
		})
	})

})

func splitHostandPort(url string) (string, int) {
	urlparts := strings.Split(url, ":")
	host := strings.TrimPrefix(urlparts[1], "//")
	port, _ := strconv.Atoi(urlparts[2])
	return host, port
}

func getRelativeFile(relativeFilepath string) string {
	_, filename, _, _ := runtime.Caller(1)
	thisDir := filepath.Dir(filename)
	return filepath.Join(thisDir, relativeFilepath)
}
