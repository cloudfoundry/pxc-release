package disk

import (
	"errors"
	"syscall"
)

func RoomToMigrate(diskUsageSyscall func(path string, stat *syscall.Statfs_t) error) error {
	var stats syscall.Statfs_t

	err := diskUsageSyscall("/var/vcap/store", &stats)

	if err != nil {
		return err
	}

	totalBlocks := stats.Blocks
	freeBlocks := stats.Bfree

	if 100 * freeBlocks / totalBlocks < 55 {
		return errors.New("Cannot continue, insufficient disk space to complete migration")
	}
	return nil
}
