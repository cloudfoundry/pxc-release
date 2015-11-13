package monit_status_test

import (
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_status"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("GaleraStatusChecker", func() {

	var (
		statusObject monit_status.MonitStatus
		xmlStatus    []byte
	)

	BeforeEach(func() {
		xmlFile, err := os.Open("example_status.xml")

		Expect(err).ToNot(HaveOccurred())
		xmlStatus := make([]byte, 20000)
		count, err := xmlFile.Read(xmlStatus)

		Expect(count).ToNot(Equal(0))

		statusObject, err = statusObject.NewMonitStatus(xmlStatus)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when passed a valid XML", func() {

		Context("and when passed a valid process name", func() {

			It("returns unknown", func() {
				stat, err := statusObject.GetStatus("mariadb_ctrl")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("unknown"))
			})

			It("returns running", func() {
				stat, err := statusObject.GetStatus("galera-healthcheck")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("running"))
			})

			It("returns unknown", func() {
				stat, err := statusObject.GetStatus("log-purger")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("unknown"))
			})

			It("returns failing", func() {
				stat, err := statusObject.GetStatus("system")
				Expect(err).ToNot(HaveOccurred())
				Expect(stat).To(Equal("failing"))
			})
		})

		Context("and when passed an invalid process name", func() {
			It("returns an error", func() {
				_, err := statusObject.GetStatus("fake_process")
				Expect(err.Error()).To(ContainSubstring("Could not find process fake_process"))
			})
		})
	})

	Context("when passed an invalid XML", func() {
		It("returns an error", func() {
			xmlStatus = []byte("fake XML status!!")
			_, err := statusObject.NewMonitStatus(xmlStatus)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to unmarshal the xml"))
		})
	})
})
