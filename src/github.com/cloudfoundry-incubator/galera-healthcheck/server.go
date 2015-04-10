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
	_ "github.com/go-sql-driver/mysql"
    "github.com/cloudfoundry-incubator/cf-lager"
    "github.com/pivotal-golang/lager"
)



var healthchecker *healthcheck.Healthchecker

func handler(w http.ResponseWriter, r *http.Request, logger lager.Logger) {
	result, msg := healthchecker.Check()
	if result {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

    body := fmt.Sprintf("Galera Cluster Node Status: %s", msg)
	fmt.Fprint(w, body)

    logger.Debug(fmt.Sprintf("Healhcheck Response Body: %s", body))
}

func main() {
    flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
    var (
        host = flags.String("host", "0.0.0.0", "Specifies the host of the healthcheck server")
        port = flags.Int("port", 8080, "Specifies the port of the healthcheck server")
        dbHost = flags.String("dbHost", "127.0.0.1", "Specifies the MySQL host to connect to")
        dbPort = flags.Int("dbPort", 3306, "Specifies the MySQL port to connect to")
        dbUser = flags.String("dbUser", "root", "Specifies the MySQL user to connect with")
        dbPassword = flags.String("dbPassword", "", "Specifies the MySQL password to connect with")
        availableWhenDonor = flags.Bool("availWhenDonor", true, "Specifies if the healthcheck allows availability when in donor state")
        availableWhenReadOnly = flags.Bool("availWhenReadOnly", false, "Specifies if the healthcheck allows availability when in read only mode")
        pidFile = flags.String("pidFile", "", "Path to create a pid file when the healthcheck server has started")
    )
    cf_lager.AddFlags(flags)
    flags.Parse(os.Args[1:])
    logger, _ := cf_lager.New("Quota Enforcer")

	db, _ := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/", *dbUser, *dbPassword, *dbHost, *dbPort))
	config := healthcheck.HealthcheckerConfig{
		*availableWhenDonor,
		*availableWhenReadOnly,
	}

	healthchecker = healthcheck.New(db, config)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        handler(w, r, logger)
    })

    address := fmt.Sprintf("%s:%d", *host, *port)

    go func() {
        resp, err := http.Get(fmt.Sprintf("http://%s/", address))
        if err != nil {
            panic(err)
        }
        defer resp.Body.Close()
        body, err := ioutil.ReadAll(resp.Body)
        if err != nil {
            panic(err)
        }
        logger.Info(fmt.Sprintf("Initial Response: %s", body))

        if *pidFile != "" {
            // existence of pid file means the server is running
            err = ioutil.WriteFile(*pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
            if err != nil {
                panic(err)
            }
        }

        // Used by tests to deterministically know that the healthcheck is accepting incoming connections
        logger.Info("Healthcheck Started")
    }()

    server := &http.Server{Addr: address}
    server.ListenAndServe()

}
