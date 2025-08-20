package main_test

import (
	"bytes"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegaformat "github.com/onsi/gomega/format"

	generateAutoTuneMysql "github.com/cloudfoundry/generate-auto-tune-mysql"
)

var sampleConfig1 = `
[mysqld]
innodb_buffer_pool_size = 84
binlog_space_limit = 214748364
max_binlog_size = 71581696
[mysqld-8.4]
wsrep_applier_threads = 3
[mysqld-8.0]
wsrep_applier_threads = 3
`

var sampleConfig2 = `
[mysqld]
innodb_buffer_pool_size = 6
binlog_space_limit = 2149135626
max_binlog_size = 716378112
[mysqld-8.4]
wsrep_applier_threads = 3
[mysqld-8.0]
wsrep_applier_threads = 3
`

var sampleConfig3 = `
[mysqld]
innodb_buffer_pool_size = 84
binlog_space_limit = 5368709120
max_binlog_size = 1073741824
[mysqld-8.4]
wsrep_applier_threads = 3
[mysqld-8.0]
wsrep_applier_threads = 3
`

var sampleConfig4 = `
[mysqld]
innodb_buffer_pool_size = 84
[mysqld-8.4]
wsrep_applier_threads = 3
[mysqld-8.0]
wsrep_applier_threads = 3
`

var _ = Describe("AutoTuneGenerator", func() {
	Describe("Generate", func() {
		var (
			values generateAutoTuneMysql.GenerateValues
		)

		BeforeEach(func() {
			gomegaformat.TruncatedDiff = false
			values.TotalMem = uint64(200)
			values.TotalDiskinKB = uint64(2 * 1024 * 1024)
			values.TargetPercentageofMem = float64(42)
			values.TargetPercentageofDisk = float64(10)
			values.NumCPUs = 3
		})

		It("writes file with correct parameters", func() {
			writer := &bytes.Buffer{}
			Expect(generateAutoTuneMysql.Generate(values, writer)).To(Succeed())
			Expect(writer.String()).To(Equal(sampleConfig1))
		})

		Context("when the calculations result in floating numbers", func() {
			BeforeEach(func() {
				values.TotalMem = uint64(10)
				values.TotalDiskinKB = uint64(12345678)
				values.TargetPercentageofMem = float64(66)
				values.TargetPercentageofDisk = float64(17)
				values.NumCPUs = 3
			})

			It("floors floating numbers to whole integers of bytes", func() {
				writer := &bytes.Buffer{}
				Expect(generateAutoTuneMysql.Generate(values, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig2))
			})
		})

		Context("when using binlog space limit is more than 3GB", func() {
			BeforeEach(func() {
				values.TotalDiskinKB = uint64(10 * 1024 * 1024)
				values.TargetPercentageofDisk = float64(50)
				values.NumCPUs = 3
			})

			It("sets the maximum binlog file size to 1GB", func() {
				writer := &bytes.Buffer{}
				Expect(generateAutoTuneMysql.Generate(values, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig3))
			})
		})

		Context("when using the default target percentage of disk of 0", func() {
			BeforeEach(func() {
				values.TargetPercentageofDisk = 0.0
				values.NumCPUs = 3
			})

			It("does not set binlog parameters", func() {
				writer := &bytes.Buffer{}
				Expect(generateAutoTuneMysql.Generate(values, writer)).To(Succeed())
				Expect(writer.String()).To(Equal(sampleConfig4))
			})
		})

		Context("when writing the config file fails", func() {
			It("returns an error", func() {
				writer := FailingWriter{}
				err := generateAutoTuneMysql.Generate(values, writer)
				Expect(err).To(MatchError(`failed to emit mysql configuration: write failed`))
			})
		})
	})
})

type FailingWriter struct{}

func (FailingWriter) Write(_ []byte) (n int, err error) {
	return -1, errors.New("write failed")
}
