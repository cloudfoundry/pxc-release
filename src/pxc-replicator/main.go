// Package main contains the entrypoint for the executable binary
package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	"go.yaml.in/yaml/v3"
)

var (
	dataDir      = flag.String("data-dir", "/tmp", "directory to store ca certs, needs to be accessible by mysqld process")
	dumpDir      = flag.String("dump-dir", "/tmp", "directory to store dumps")
	mysqlBinPath = flag.String("mysql-bin-path", "", "path to MySQL binaries; if empty, rely on $PATH")
	configFile   = flag.String("config", "/var/vcap/jobs/pxc-replicator/config/config.yml", "path to YAML config file")
	mysqlVersion = flag.String("mysql-version", "8.4", "the mysql MAJ.MIN version of source and target")
)

func main() {
	flag.Parse()

	if *configFile == "" {
		log.Fatalf("-config flag must be provided")
	}
	confBytes, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("failed reading manifest file %s: %s", *configFile, err)
	}

	replClient := &client.ReplClient{}
	if err = yaml.Unmarshal(confBytes, replClient); err != nil {
		log.Fatalf("failed parsing manifest YAML: %v", err)
	}
	replClient.Version = *mysqlVersion
	replClient.DataDir = *dataDir
	replClient.BinPath = *mysqlBinPath
	replClient.DumpDir = *dumpDir

	log.Printf("Parsed config:\n  Source: %+v\n  Target: %+v\n", replClient.Source.Host, replClient.Target.Host)
	log.Printf("DataDir: %s, MysqlBinPath: %s\n", *dataDir, *mysqlBinPath)

	if err = replClient.Setup(); err != nil {
		log.Fatalf("starting job failed: %s", err)
	}

	conn, err := replClient.ConnectTarget()
	if err != nil {
		log.Fatalf("failed setting up connection for healthcheck: %s", err)
	}
	defer utils.CloseAndLogError(conn)
	consecutiveFailureCount := 0
	for {
		state, err := replClient.CheckReplication(conn)
		if err != nil {
			consecutiveFailureCount += 1
			log.Printf("failed checking replication. Consecutive failures: %v, error: %s", consecutiveFailureCount, err)
			if consecutiveFailureCount >= 5 {
				log.Fatalf("failed to check replication %v in a row, resyncing", consecutiveFailureCount)
				if err := replClient.SyncSourceToTarget(); err != nil {
					log.Fatalf("failed to resync: %s", err)
				}
			}
		}
		consecutiveFailureCount = 0
		log.Printf("replication state: %s", state.String())
		time.Sleep(time.Second * 5)
	}
}
