package disk

import (
	"errors"
	"github.com/cloudfoundry/gosigar"
)


//go:generate counterfeiter . Sigar
type Sigar interface {
	GetFileSystemUsage(string) (sigar.FileSystemUsage, error)
}

func RoomToMigrate(systemInfoGatherer Sigar) error {

	fileSystemUsage, err := systemInfoGatherer.GetFileSystemUsage("/var/vcap/store")
	if err != nil {
		return err
	}

	emptyDBSizeBytes := uint64(2500000000) // Approximate size of an empty Percona installation
	usedBytes := fileSystemUsage.Used
	totalBytes := fileSystemUsage.Total
	if 100*(usedBytes-emptyDBSizeBytes)/totalBytes > 45 {
		return errors.New("Cannot continue, insufficient disk space to complete migration")
	}
	return nil
}
