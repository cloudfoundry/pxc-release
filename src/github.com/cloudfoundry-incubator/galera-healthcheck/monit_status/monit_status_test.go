package monit_status_test

import (
	"bytes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_status"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"os"
)

var _ = Describe("GaleraStatusChecker", func() {

	var (
		statusObject monit_status.MonitStatus
		logger       *lagertest.TestLogger
	)

	BeforeEach(func() {
		xmlFile, err := os.Open("example_status.xml")
		logger = lagertest.NewTestLogger("monit_status")

		statusObject, err = statusObject.NewMonitStatus(xmlFile, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when passed a valid XML response", func() {

		Context("when passed a valid process name", func() {

			It("returns stopped", func() {
				stat, err := statusObject.GetStatus("mariadb_ctrl")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("stopped"))
			})

			It("returns running", func() {
				stat, err := statusObject.GetStatus("galera-healthcheck")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("running"))
			})

			It("returns stopped", func() {
				stat, err := statusObject.GetStatus("log-purger")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("stopped"))
			})

			It("returns failing", func() {
				stat, err := statusObject.GetStatus("system")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("failing"))
			})
		})

		Context("when passed an invalid process name", func() {
			It("returns an error", func() {
				_, err := statusObject.GetStatus("fake_process")
				Expect(err.Error()).To(ContainSubstring("Could not find process fake_process"))
			})
		})
	})

	Context("when passed an invalid XML", func() {
		It("returns an error", func() {
			xmlFile := bytes.NewReader([]byte("fake XML status!!"))
			_, err := statusObject.NewMonitStatus(xmlFile, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to unmarshal the xml"))
		})
	})
})
