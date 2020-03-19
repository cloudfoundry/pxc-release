package disk_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"errors"
	"github.com/cloudfoundry/gosigar"
	"github.com/cloudfoundry/migrate-to-pxc/disk"
	"github.com/cloudfoundry/migrate-to-pxc/disk/diskfakes"
)

var _ = Describe("Disk", func() {
	Context("when there isn't enough free space to copy the data in the mysql dir", func() {
		It("returns an error message", func() {
			fakeFileSystemUsage := sigar.FileSystemUsage{
				Total: 10000000,
				Used:  8000000,
			}
			var fakeSigar diskfakes.FakeSigar

			fakeSigar.GetFileSystemUsageReturns(fakeFileSystemUsage, nil)
			err := disk.RoomToMigrate(&fakeSigar)
			Expect(err).To(MatchError("Cannot continue, insufficient disk space to complete migration"))
		})
	})

	It("returns nil when there is enough disk space", func() {
		fakeFileSystemUsage := sigar.FileSystemUsage{
			Total: 10000000,
			Used:  7000000,
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
