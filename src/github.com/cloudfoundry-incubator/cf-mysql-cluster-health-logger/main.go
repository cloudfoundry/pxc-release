package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/validator.v2"
	"gopkg.in/yaml.v3"

	"github.com/cloudfoundry-incubator/cf-mysql-cluster-health-logger/logwriter"
)

func main() {
	var config logwriter.Config

	// Define command line flags
	var configData = flag.String("config", "", "json encoded configuration string")
	var configPath = flag.String("configPath", "", "path to configuration file with json encoded content")
	flag.Parse()

	// Load configuration from command line, file, or environment
	err := loadConfig(&config, *configData, *configPath)
	if err != nil {
		log.Fatal("Failed to read config:", err)
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

// Load configuration from sources in order of precedence:
// command line data "-config" or file "-configPath", environment variables
// for data CONFIG or file CONFIG_PATH.
func loadConfig(config *logwriter.Config, configData, configPath string) error {
	var yamlData []byte
	var err error

	if configData != "" {
		yamlData = []byte(configData)
	} else if configPath != "" {
		yamlData, err = os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("reading config file %s: %w", configPath, err)
		}
	} else if envConfig := os.Getenv("CONFIG"); envConfig != "" {
		yamlData = []byte(envConfig)
	} else if envConfigPath := os.Getenv("CONFIG_PATH"); envConfigPath != "" {
		yamlData, err = os.ReadFile(envConfigPath)
		if err != nil {
			return fmt.Errorf("reading config file from CONFIG_PATH %s: %w", envConfigPath, err)
		}
	} else {
		return fmt.Errorf("no configuration provided: use -config, -configPath, CONFIG, or CONFIG_PATH")
	}

	// Parse YAML
	if err := yaml.Unmarshal(yamlData, config); err != nil {
		return fmt.Errorf("parsing YAML config: %w", err)
	}

	return nil
}
