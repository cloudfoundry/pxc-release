package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
)

var (
	targetPercentageofMem float64
	targetPercentageofDisk float64
	outputFile string
)

func main() {
	flag.Float64Var(&targetPercentageofMem, "P", 50.0,
			"Set this to an integer which represents the percentage of system RAM to reserve for InnoDB's buffer pool")
	flag.Float64Var(&targetPercentageofDisk, "D", 0,
		"Set this to an integer which represents the percentage of disk to limit how much space is used by binary logs")
	flag.StringVar(&outputFile, "f", "",
		       "Target file for rendering MySQL option file")
	flag.Parse()


	mem := sigar.Mem{}
	mem.Get()
	totalMem := mem.Total

	fsu := sigar.FileSystemUsage{}
	fsu.Get("/var/vcap/store/")
	totalDiskinKB := fsu.Total

	fmt.Printf("%s Total memory in bytes: %d\n", time.Now().UTC().Format(time.RFC3339Nano), totalMem)
	fmt.Printf("%s Total disk in kilobytes: %d\n", time.Now().UTC().Format(time.RFC3339Nano), totalDiskinKB)

	file, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := Generate(totalMem, totalDiskinKB, targetPercentageofMem, targetPercentageofDisk, file); err != nil {
		fmt.Printf("%s generating %s failed: %s\n",
			time.Now().UTC().Format(time.RFC3339Nano), outputFile, err)
		os.Exit(1)
	}
}
