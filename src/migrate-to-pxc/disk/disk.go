package disk

import (
	"errors"
	"syscall"
)

func RoomToMigrate(diskUsageSyscall func(path string, stat *syscall.Statfs_t) error) error {
	var stat syscall.Statfs_t

	err := diskUsageSyscall("/var/vcap/store", &stat)

	if err != nil {
		return err
	}

	totalBlocks := stat.Blocks
	freeBlocks := stat.Bfree
	usedBlocks := totalBlocks - freeBlocks

	emptyDBSizeBytes := 2500000000
	emptyDBSizeBlocks := uint64(emptyDBSizeBytes) / uint64(stat.Bsize)

	if 100 * (usedBlocks - emptyDBSizeBlocks) / totalBlocks > 45 {
		return errors.New("Cannot continue, insufficient disk space to complete migration")
	}

	return nil
}
