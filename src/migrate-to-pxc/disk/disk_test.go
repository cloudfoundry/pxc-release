package disk_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"migrate-to-pxc/disk"
	"syscall"
	"errors"
)

var _ = Describe("Disk", func() {
	It("returns an error message when there is not enough disk space", func() {
		fakeStatsFunc := func(path string, stat *syscall.Statfs_t) error {
			stat.Blocks = 100
			stat.Bfree = 54
			return nil
		}
		err := disk.RoomToMigrate(fakeStatsFunc)
		Expect(err).To(MatchError("Cannot continue, insufficient disk space to complete migration"))
	})

	It("returns true when there is enough disk space", func() {
		fakeStatsFunc := func(path string, stat *syscall.Statfs_t) error {
			stat.Blocks = 100
			stat.Bfree = 55
			return nil
		}
		err := disk.RoomToMigrate(fakeStatsFunc)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error when the syscall errors", func() {
		fakeStatsFunc := func(path string, stat *syscall.Statfs_t) error {
			return errors.New("syscall error")
		}
		err := disk.RoomToMigrate(fakeStatsFunc)
		Expect(err).To(HaveOccurred())
	})
})
