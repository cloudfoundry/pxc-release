package healthcheck_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	_ "github.com/go-sql-driver/mysql"
	"time"
)

var _ = Describe("Healthcheck Integration", func() {

	Describe("Writing pid file", func() {
		var (
			tempDirPath string
			session     *gexec.Session
			pidFilePath string
			pidFileFlag string
		)

		BeforeEach(func() {
			var err error
			tempDirPath, err = ioutil.TempDir(os.TempDir(), "galera-healthcheck-integration-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(tempDirPath)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when the pid file location is valid", func() {
			BeforeEach(func() {
				pidFilePath = fmt.Sprintf("%s/healthcheck.pid", tempDirPath)
				pidFileFlag = fmt.Sprintf("-pidFile=%s", pidFilePath)
			})

			It("writes its pid to the provided file", func() {
				Expect(fileExists(pidFilePath)).To(BeFalse())
				session = startHealthcheck(pidFileFlag)
				awaitHealthcheckStarted(session)
				Expect(fileExists(pidFilePath)).To(BeTrue())
			})

			AfterEach(func() {
				stopHealthcheck(session)
			})
		})

		Context("when the pid file location is invalid", func() {
			BeforeEach(func() {
				pidFilePath = fmt.Sprintf("%s/invalid_path/healthcheck.pid", tempDirPath)
				pidFileFlag = fmt.Sprintf("-pidFile=%s", pidFilePath)
			})

			It("exits with error", func() {
				session = startHealthcheck(pidFileFlag)

				Eventually(session.Err).Should(gbytes.Say(pidFilePath))
				Eventually(session).Should(gexec.Exit())
				Expect(session.ExitCode()).ToNot(Equal(0))
			})
		})
	})

	Describe("Signal handling", func() {
		var session *gexec.Session

		BeforeEach(func() {
			session = startHealthcheck()
			awaitHealthcheckStarted(session)
		})

		It("shuts downs when interrupted", func() {
			session.Interrupt()
			session.Wait(5 * time.Second)
			Eventually(session).Should(gexec.Exit())
		})

		It("shuts downs when terminated", func() {
			session.Terminate()
			session.Wait(5 * time.Second)
			Eventually(session).Should(gexec.Exit())
		})

		It("shuts downs when killed", func() {
			session.Kill()
			session.Wait(5 * time.Second)
			Eventually(session).Should(gexec.Exit())
		})
	})

	Describe("Healthcheck", func() {
		Context("when galera db is synced", func() {
			var session *gexec.Session

			BeforeEach(func() {
				session = startHealthcheck()
				awaitHealthcheckStarted(session)
			})

			AfterEach(func() {
				stopHealthcheck(session)
			})

			// Can't set up the database to be in a state we expect, because we don't own it.
			It("responds with the cluster status", func() {
				resp, err := http.Get(endpoint())
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())

				switch resp.StatusCode {
				case http.StatusOK:
					Expect(string(body)).To(Equal("Galera Cluster Node Status: synced"))
				case http.StatusServiceUnavailable:
					Expect(string(body)).To(Equal("Galera Cluster Node Status: wsrep_local_state variable not set (possibly not a galera db)"))
				default:
					Fail(fmt.Sprintf("Unexpected status code: %d", resp.StatusCode))
				}
			})
		})
	})
})

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
