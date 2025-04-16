package main

import (
	"io"
	"text/template"

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

	totalDisk := values.TotalDiskinKB * 1024
	binLogSpaceLimit := float64(totalDisk) * values.TargetPercentageofDisk / 100.0

	maxBinlogSize := min(int64(binLogSpaceLimit/3), 1024*1024*1024)
	maxBinlogSize = (maxBinlogSize / binlogBlockSize) * binlogBlockSize

	tmpl, _ := template.New("auto_tune").Parse(`
[mysqld]
innodb_buffer_pool_size = {{.BufferPoolSize}}
{{if ne .TargetPercentageOfDisk 0.0 -}}
binlog_space_limit = {{.BinlogSpaceLimit}}
max_binlog_size = {{.MaxBinlogSize}}
{{end -}}
[mysqld-8.0]
wsrep_applier_threads = {{.NumCPUs}}
[mysqld-5.7]
wsrep_slave_threads = {{.NumCPUs}}
`)
	data := struct {
		BufferPoolSize         uint64
		BinlogSpaceLimit       uint64
		MaxBinlogSize          uint64
		NumCPUs                int
		TargetPercentageOfDisk float64
	}{
		BufferPoolSize:         uint64(bufferPoolSize),
		BinlogSpaceLimit:       uint64(binLogSpaceLimit),
		MaxBinlogSize:          uint64(maxBinlogSize),
		NumCPUs:                values.NumCPUs,
		TargetPercentageOfDisk: values.TargetPercentageofDisk,
	}
	err := tmpl.Execute(writer, data)

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
