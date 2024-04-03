package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

const binlogBlockSize = 4 * 1024

type GenerateValues struct {
	TotalMem               uint64
	TotalDiskinKB          uint64
	TargetPercentageofMem  float64
	TargetPercentageofDisk float64
	NumCPUs                int
}

func Generate(values GenerateValues, writer io.Writer) error {

	bufferPoolSize := float64(values.TotalMem) * values.TargetPercentageofMem / 100.0

	var params []string
	params = append(params, fmt.Sprintf("innodb_buffer_pool_size = %d", uint64(bufferPoolSize)))

	if values.TargetPercentageofDisk != 0.0 {
		totalDisk := values.TotalDiskinKB * 1024
		binLogSpaceLimit := float64(totalDisk) * values.TargetPercentageofDisk / 100.0
		params = append(params, fmt.Sprintf("binlog_space_limit = %d", uint64(binLogSpaceLimit)))

		maxBinlogSize := min(int64(binLogSpaceLimit/3), 1024*1024*1024)
		maxBinlogSize = (maxBinlogSize / binlogBlockSize) * binlogBlockSize

		params = append(params, fmt.Sprintf("max_binlog_size = %d", uint64(maxBinlogSize)))
	}

	config := "\n[mysqld]\n" + strings.Join(params, "\n") + "\n"
	_, err := writer.Write([]byte(config))
	return errors.Wrap(err, "failed to emit mysql configuration")
}
