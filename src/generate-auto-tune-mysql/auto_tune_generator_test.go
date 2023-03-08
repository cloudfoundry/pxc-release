package main_test

import (
	"bytes"
	"errors"
	. "github.com/cloudfoundry/generate-auto-tune-mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var sampleConfig1 = `
[mysqld]
innodb_buffer_pool_size = 84
binlog_space_limit = 214748364
max_binlog_size = 71581696
`

var sampleConfig2 = `
[mysqld]
innodb_buffer_pool_size = 6
binlog_space_limit = 2149135626
max_binlog_size = 716378112
`

var sampleConfig3 = `
[mysqld]
innodb_buffer_pool_size = 84
binlog_space_limit = 5368709120
max_binlog_size = 1073741824
`

var sampleConfig4 = `
[mysqld]
innodb_buffer_pool_size = 84
`

var _ = Describe("AutoTuneGenerator", func() {
	Describe("Generate", func() {
		var (
			totalMem               uint64
			totalDiskinKB          uint64
			targetPercentageofMem  float64
			targetPercentageofDisk float64
		)

		BeforeEach(func() {
			totalMem = uint64(200)
			totalDiskinKB = uint64(2 * 1024 * 1024)
			targetPercentageofMem = float64(42)
			targetPercentageofDisk = float64(10)
		})

		It("writes file with correct parameters", func() {
			writer := &bytes.Buffer{}
			Expect(Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, writer)).To(Succeed())
			Expect(writer.String()).To(Equal(sampleConfig1))
		})

		Context("when the calculations result in floating numbers", func() {
			BeforeEach(func() {
				totalMem = uint64(10)
				totalDiskinKB = uint64(12345678)
				targetPercentageofMem = float64(66)
				targetPercentageofDisk = float64(17)
			})

			It("floors floating numbers to whole integers of bytes", func() {
				writer := &bytes.Buffer{}
				Expect(Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig2))
			})
		})

		Context("when using binlog space limit is more than 3GB", func() {
			BeforeEach(func() {
				totalDiskinKB = uint64(10 * 1024 * 1024)
				targetPercentageofDisk = float64(50)
			})

			It("sets the maximum binlog file size to 1GB", func() {
				writer := &bytes.Buffer{}
				Expect(Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig3))
			})
		})

		Context("when using the default target percentage of disk of 0", func() {
			BeforeEach(func() {
				targetPercentageofDisk = 0.0
			})

			It("does not set binlog parameters", func() {
				writer := &bytes.Buffer{}
				Expect(Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig4))
			})
		})

		Context("when writing the config file fails", func() {
			It("returns an error", func() {
				writer := FailingWriter{}
				err := Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, writer)
				Expect(err).To(MatchError(`failed to emit mysql configuration: write failed`))
			})
		})
	})
})

type FailingWriter struct{}

func (FailingWriter) Write(p []byte) (n int, err error) {
	return -1, errors.New("write failed")
}
