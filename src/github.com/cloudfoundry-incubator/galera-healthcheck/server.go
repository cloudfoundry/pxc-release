package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	. "github.com/cloudfoundry-incubator/galera-healthcheck/logger"
	_ "github.com/go-sql-driver/mysql"
)

var host = flag.String(
    "host",
    "0.0.0.0",
    "Specifies the host of the healthcheck server",
)

var port = flag.Int(
	"port",
	8080,
	"Specifies the port of the healthcheck server",
)

var dbHost = flag.String(
    "dbHost",
    "127.0.0.1",
    "Specifies the MySQL host to connect to",
)

var dbPort = flag.Int(
    "dbPort",
    3306,
    "Specifies the MySQL port to connect to",
)

var dbUser = flag.String(
	"dbUser",
	"root",
	"Specifies the MySQL user to connect with",
)

var dbPassword = flag.String(
	"dbPassword",
	"",
	"Specifies the MySQL password to connect with",
)

var availableWhenDonor = flag.Bool(
	"availWhenDonor",
	true,
	"Specifies if the healthcheck allows availability when in donor state",
)

var availableWhenReadOnly = flag.Bool(
	"availWhenReadOnly",
	false,
	"Specifies if the healthcheck allows availability when in read only mode",
)

var pidfile = flag.String(
	"pidfile",
	"",
	"Location for the pidfile",
)

var healthchecker *healthcheck.Healthchecker

func handler(w http.ResponseWriter, r *http.Request) {
	result, msg := healthchecker.Check()
	if result {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	fmt.Fprintf(w, "Galera Cluster Node status: %s", msg)
	LogWithTimestamp(msg)
}

func main() {
	flag.Parse()

	err := ioutil.WriteFile(*pidfile, []byte(strconv.Itoa(os.Getpid())), 0644)
	if err != nil {
		panic(err)
	}

	db, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/", *dbUser, *dbPassword, *dbHost, *dbPort))
	config := healthcheck.HealthcheckerConfig{
		*availableWhenDonor,
		*availableWhenReadOnly,
	}

	healthchecker = healthcheck.New(db, config)

	http.HandleFunc("/", handler)
	http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), nil)
}
