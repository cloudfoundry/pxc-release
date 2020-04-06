package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
)

var (
	targetPercentage float64
	outputFile string
)

func main() {
	flag.Float64Var(&targetPercentage, "P", 50.0,
			"Set this to an integer which represents the percentage of system RAM to reserve for InnoDB's buffer pool")
	flag.StringVar(&outputFile, "f", "",
		       "Target file for rendering MySQL option file")
	flag.Parse()


	mem := sigar.Mem{}
	mem.Get()
	totalMem := mem.Total

	fmt.Printf("%s Total memory in bytes: %d\n", time.Now().UTC().Format(time.RFC3339Nano), mem.Total)

	file, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := Generate(totalMem, targetPercentage, file); err != nil {
		fmt.Printf("%s generating %s failed: %s\n",
			time.Now().UTC().Format(time.RFC3339Nano), outputFile, err)
		os.Exit(1)
	}
}
