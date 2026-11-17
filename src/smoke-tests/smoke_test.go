package smoke_test

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Smoke Test Database Connectivity", func() {
	var db *sql.DB

	// tlsParam returns the DSN tls parameter for connecting to host. When a CA is
	// configured, the server certificate chain and name are verified.
	tlsParam := func(host string) (string, error) {
		caPath := os.Getenv("MYSQL_CA_PATH")
		if caPath == "" {
			return "preferred", nil
		}
		caPEM, err := os.ReadFile(caPath)
		if err != nil {
			return "", err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return "", fmt.Errorf("no certificates found in %s", caPath)
		}
		serverName := os.Getenv("MYSQL_TLS_SERVER_NAME")
		if serverName == "" {
			serverName = host
		}
		name := "smoke-tests-" + host
		err = mysql.RegisterTLSConfig(name, &tls.Config{
			RootCAs:    pool,
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
		return name, err
	}

	openDB := func(host string) (*sql.DB, error) {
		var (
			mysqlUser     = os.Getenv("MYSQL_USERNAME")
			mysqlPassword = os.Getenv("MYSQL_PASSWORD")
		)
		tlsMode, err := tlsParam(host)
		if err != nil {
			return nil, err
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/?tls=%s&wsrep_sync_wait=1",
			mysqlUser, mysqlPassword, host, tlsMode)
		return sql.Open("mysql", dsn)
	}

	testConnection := func(db *sql.DB) (string, error) {
		var data string
		err := db.QueryRow("SELECT * FROM pxc_release_test_db.connection_test_table").
			Scan(&data)
		return data, err
	}

	BeforeEach(func() {
		var err error
		db, err = openDB(os.Getenv("PROXY_HOST"))
		Expect(err).NotTo(HaveOccurred())

		Expect(db.Exec("CREATE DATABASE IF NOT EXISTS pxc_release_test_db")).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec("CREATE TABLE IF NOT EXISTS pxc_release_test_db.connection_test_table(test_data varchar(255) PRIMARY KEY)")).
			Error().NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(db.Exec("DROP DATABASE pxc_release_test_db")).
			Error().NotTo(HaveOccurred())
	})

	It("allows reading and writing data", func() {
		Expect(db.Exec("INSERT INTO pxc_release_test_db.connection_test_table VALUES('connecting!')")).
			Error().NotTo(HaveOccurred())

		Expect(testConnection(db)).To(Equal("connecting!"),
			`Expected to read back data through the proxy connection, but this failed`)
		for _, host := range strings.Fields(os.Getenv("MYSQL_HOSTS")) {
			db, err := openDB(host)
			Expect(err).NotTo(HaveOccurred())
			Expect(testConnection(db)).To(Equal("connecting!"),
				`Expected to read back data from node %q, but this failed`, host)
		}
	})
})
