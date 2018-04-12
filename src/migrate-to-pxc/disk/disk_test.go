package disk_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"migrate-to-pxc/disk"
	"migrate-to-pxc/disk/diskfakes"
	"github.com/cloudfoundry/gosigar"
	"errors"
)

var _ = Describe("Disk", func() {
	Context("when there isn't enough free space to copy the data in the mysql dir", func() {
		It("returns an error message", func() {
			fakeFileSystemUsage := sigar.FileSystemUsage{
				Total:     10000000000,
				Used:      8000000000,
			}
			var fakeSigar diskfakes.FakeSigar

			fakeSigar.GetFileSystemUsageReturns(fakeFileSystemUsage, nil)
			err := disk.RoomToMigrate(&fakeSigar)
			Expect(err).To(MatchError("Cannot continue, insufficient disk space to complete migration"))
		})
	})

	It("returns nil when there is enough disk space", func() {
		fakeFileSystemUsage := sigar.FileSystemUsage{
			Total:     10000000000,
			Used:      7000000000,
		}
		var fakeSigar diskfakes.FakeSigar

		fakeSigar.GetFileSystemUsageReturns(fakeFileSystemUsage, nil)
		err := disk.RoomToMigrate(&fakeSigar)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error when GetFileSystemUsage errors", func() {
		var fakeSigar diskfakes.FakeSigar

		fakeSigar.GetFileSystemUsageReturns(sigar.FileSystemUsage{}, errors.New("GetFileSystemUsage"))
		err := disk.RoomToMigrate(&fakeSigar)
		Expect(err).To(HaveOccurred())
	})
})
