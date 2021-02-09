package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

const binlogBlockSize = 4 * 1024

func Generate(totalMem, totalDiskinKB uint64, targetPercentageofMem, targetPercentageofDisk float64, writer io.Writer) error {

	bufferPoolSize := float64(totalMem) * targetPercentageofMem / 100.0

	params := []string{}
	params = append(params, fmt.Sprintf("innodb_buffer_pool_size = %d", uint64(bufferPoolSize)))

	if targetPercentageofDisk != 0.0 {
		totalDisk := totalDiskinKB * 1024
		binLogSpaceLimit := float64(totalDisk) * targetPercentageofDisk / 100.0
		params = append(params, fmt.Sprintf("binlog_space_limit = %d", uint64(binLogSpaceLimit)))

		maxBinlogSize := min(int64(binLogSpaceLimit / 3), 1024*1024*1024)
		maxBinlogSize = (maxBinlogSize/binlogBlockSize)*binlogBlockSize

		params = append(params, fmt.Sprintf("max_binlog_size = %d", uint64(maxBinlogSize)))
	}

	config := "\n[mysqld]\n" + strings.Join(params, "\n") + "\n"
	_, err := writer.Write([]byte(config))
	return errors.Wrap(err, "failed to emit mysql configuration")
}

func min(a int64, b ...int64) int64 {
	m := a

	for _, i := range b {
		if i < m {
			m = i
		}
	}

	return m
}
