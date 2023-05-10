package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	sigar "github.com/cloudfoundry/gosigar"
)

func main() {
	var (
		outputFile string
		values     GenerateValues
	)

	flag.Float64Var(&values.TargetPercentageofMem, "P", 50.0,
		"Set this to an integer which represents the percentage of system RAM to reserve for InnoDB's buffer pool")
	flag.Float64Var(&values.TargetPercentageofDisk, "D", 0,
		"Set this to an integer which represents the percentage of disk to limit how much space is used by binary logs")
	flag.StringVar(&outputFile, "f", "",
		"Target file for rendering MySQL option file")
	flag.Parse()

	mem := sigar.Mem{}
	//cpuList := sigar.CpuList{}
	//cpuCount := len(cpuList.List)
	mem.Get()
	values.TotalMem = mem.Total

	fsu := sigar.FileSystemUsage{}
	fsu.Get("/var/vcap/store/")
	values.TotalDiskinKB = fsu.Total

	fmt.Printf("%s Total memory in bytes: %d\n", time.Now().UTC().Format(time.RFC3339Nano), values.TotalMem)
	fmt.Printf("%s Total disk in kilobytes: %d\n", time.Now().UTC().Format(time.RFC3339Nano), values.TotalDiskinKB)

	file, err := os.OpenFile(outputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := Generate(values, file); err != nil {
		fmt.Printf("%s generating %s failed: %s\n",
			time.Now().UTC().Format(time.RFC3339Nano), outputFile, err)
		os.Exit(1)
	}
}
