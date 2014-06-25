package main

import (
	"flag"
	"fmt"
	"net/http"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	. "github.com/cloudfoundry-incubator/galera-healthcheck/logger"

)

var serverPort = flag.Int(
	"port",
	8080,
	"Specifies the port of the healthcheck server",
)

var mysqlUser = flag.String(
	"user",
	"root",
	"Specifies the MySQL user to connect as",
)

var mysqlPassword = flag.String(
	"password",
	"",
	"Specifies the MySQL password to connect with",
)

var availableWhenDonor = flag.Bool(
	"availWhenDonor",
	false,
	"Specifies if the healthcheck allows availability when in donor state",
)

var availableWhenReadOnly = flag.Bool(
	"availWhenReadOnly",
	false,
	"Specifies if the healthcheck allows availability when in read only mode",
)

var healthchecker *healthcheck.Healthchecker

func handler(w http.ResponseWriter, r *http.Request) {
	result,msg := healthchecker.Check()
	if (result) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	fmt.Fprintf(w, "Galera Cluster Node status: %s", msg)
	LogWithTimestamp(msg)
}

func main() {
	flag.Parse()

	db, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@/", *mysqlUser, *mysqlPassword))
	config := healthcheck.HealthcheckerConfig{
		*availableWhenDonor,
		*availableWhenReadOnly,
	}

	healthchecker = healthcheck.New(db, config)

	http.HandleFunc("/", handler)
	http.ListenAndServe(fmt.Sprintf(":%d", *serverPort), nil)
}
