package main

import (
	"fmt"
	"io"
	"text/template"
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

	tmpl := template.Must(template.New("auto_tune").Parse(`
[mysqld]
innodb_buffer_pool_size = {{.bufferPoolSize}}
{{if .binlogSpaceLimit -}}
binlog_space_limit = {{.binlogSpaceLimit}}
{{end -}}
{{if .maxBinlogSize -}}
max_binlog_size = {{.maxBinlogSize}}
{{end -}}
[mysqld-8.4]
wsrep_applier_threads = {{.numCPUs}}
[mysqld-8.0]
wsrep_applier_threads = {{.numCPUs}}
`))

	data := map[string]any{
		"bufferPoolSize":         uint64(bufferPoolSize),
		"binlogSpaceLimit":       uint64(binLogSpaceLimit),
		"maxBinlogSize":          uint64(maxBinlogSize),
		"numCPUs":                values.NumCPUs,
		"targetPercentageOfDisk": values.TargetPercentageofDisk,
	}
	if err := tmpl.Execute(writer, data); err != nil {
		return fmt.Errorf("failed to emit mysql configuration: %w", err)
	}

	return nil
}
