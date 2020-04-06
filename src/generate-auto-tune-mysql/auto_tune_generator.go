package main

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

func Generate(totalMem uint64, targetPercentage float64, writer io.Writer) error {

	bufferPoolSize := float64(totalMem) * targetPercentage / 100.0
	_, err := writer.Write([]byte(fmt.Sprintf(`
[mysqld]
innodb_buffer_pool_size = %d
`, uint64(bufferPoolSize))))

	return errors.Wrap(err, "failed to emit mysql configuration")
}
