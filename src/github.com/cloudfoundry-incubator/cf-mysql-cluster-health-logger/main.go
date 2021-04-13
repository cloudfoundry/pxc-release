package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"

	"github.com/cloudfoundry-incubator/cf-mysql-cluster-health-logger/logwriter"
)

func main() {
	var config logwriter.Config
	serviceConfig := service_config.New()

	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	serviceConfig.AddFlags(flags)
	flags.Parse(os.Args[1:])
	err := serviceConfig.Read(&config)

	if err != nil {
		log.Fatal("Failed to read config", err)
	}

	err = validator.Validate(config)
	if err != nil {
		log.Fatalf("Failed to validate config: %v", err)
	}

	db, err := sql.Open("mysql",
		fmt.Sprintf("%s:%s@unix(%s)/",
			config.User,
			config.Password,
			config.Socket,
		))

	if err != nil {
		log.Fatal("Failed to initialize database pool", err)
	}

	writer := logwriter.New(db, config.LogPath)

	for {
		err := writer.Write(time.Now().Format(time.RFC3339Nano))
		if err != nil {
			log.Println(err)
		}
		time.Sleep(time.Duration(config.Interval) * time.Second)
	}
}
