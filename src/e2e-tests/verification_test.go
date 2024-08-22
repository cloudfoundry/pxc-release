package e2e_tests

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code.cloudfoundry.org/tlsconfig/certtest"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Feature Verification", Ordered, Label("verification"), func() {
	var (
		db             *sql.DB
		deploymentName string
		proxyHost      string
	)

	BeforeAll(func() {
		deploymentName = "pxc-feature-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`enable-mysql-gtid.yml`),
			bosh.Operation(`enable-mysql-backup-user.yml`),
			bosh.Operation(`test/seed-test-user.yml`),
			bosh.Operation(`require-tls.yml`),
			bosh.Operation(`test/test-audit-logging.yml`),
			bosh.Operation(`test/use-mtls.yml`),
			bosh.Operation(`test/tune-mysql-config.yml`),
			bosh.Operation(`test/with-wildcard-schema-access.yml`),
			bosh.Operation(`test/with-syslog.yml`),
			bosh.Operation(`test/optimize-vm-swappiness.yml`),
			bosh.Operation(`enable-jemalloc.yml`),
			bosh.Var(`innodb_buffer_pool_size_percent`, `14`),
			bosh.Var(`binlog_space_percent`, `20`),
		)).To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).To(HaveLen(2))
		proxyHost = proxyIPs[0]

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyHost+")/?tls=skip-verify&interpolateParams=true")
		Expect(err).NotTo(HaveOccurred())
		db.SetMaxIdleConns(0)
		db.SetMaxOpenConns(1)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}
		Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	Context("OS configuration", Label("os_config"), func() {
		It("configures vm.swappiness = 1", func() {
			swappinessValues, err := bosh.RemoteCommand(deploymentName, "mysql", "cat /proc/sys/vm/swappiness")
			Expect(err).NotTo(HaveOccurred())

			Expect(strings.Fields(swappinessValues)).To(ConsistOf("1", "1", "1"),
				`Expected vm.swappiness to be 1 on all mysql nodes, but it was not!`)

			sysctlOutput, err := bosh.RemoteCommand(deploymentName, "mysql/0", `sudo sysctl --load /etc/sysctl.d/70-mysql-swappiness.conf 2>&1`)
			Expect(err).NotTo(HaveOccurred(), "Expected sysctl to be able to read /etc/sysctl.d/70-mysql-swappiness.conf, but it failed!\noutput = %s", sysctlOutput)
			Expect(sysctlOutput).To(ContainSubstring(`vm.swappiness = 1`),
				"Expected vm.swappiness to be 1, but it was not!\nCommand output: %s", sysctlOutput)
		})
	})

	Context("MySQL Configuration", Label("configuration"), func() {
		It("sets the default sync_binlog value", Label("sync_binlog"), func() {
			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var syncBinlog string
				Expect(db.QueryRow("SELECT @@global.sync_binlog").Scan(&syncBinlog)).
					To(Succeed())
				Expect(syncBinlog).To(Equal(`1`))
				Expect(db.Close()).To(Succeed())
			}
		})

		It("sets the expected innodb_flush_method", Label("innodb_flush_method"), func() {
			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var innodbFlushMethod string
				Expect(db.QueryRow("SELECT @@global.innodb_flush_method").Scan(&innodbFlushMethod)).
					To(Succeed())
				Expect(innodbFlushMethod).To(Equal(`fsync`))
				Expect(db.Close()).To(Succeed())
			}
		})

		It("initializes a cluster with an empty gtid_executed", func() {
			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var queryResultString string
				Expect(db.QueryRow("SELECT @@global.gtid_executed;").Scan(&queryResultString)).
					To(Succeed())
				Expect(queryResultString).To(BeEmpty())
			}
		})

		It("records gtids with subsequent transactions", func() {
			Expect(db.Exec(`CREATE DATABASE binary_logs`)).
				Error().NotTo(HaveOccurred())
			var expectedGTIDExecuted string
			Expect(db.QueryRow("SELECT @@global.gtid_executed").Scan(&expectedGTIDExecuted)).
				To(Succeed())
			Expect(expectedGTIDExecuted).ToNot(BeEmpty())

			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				instanceDB, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					var memberGTIDExecuted string
					_ = instanceDB.QueryRow("SELECT @@global.gtid_executed").Scan(&memberGTIDExecuted)
					return memberGTIDExecuted
				}).Should(Equal(expectedGTIDExecuted))
			}
		})

		It("Sets the default character set to utf8mb4 ", func() {
			var characterSetServer string
			Expect(db.QueryRow(`SELECT @@global.character_set_server`).Scan(&characterSetServer)).To(Succeed())
			Expect(characterSetServer).To(Equal("utf8mb4"))
		})

		It("Sets the default collation to the MySQL Server default for utf8mb4", func() {
			var mysqlDefaultCollationForUTF8MB4 string
			Expect(db.QueryRow(`SELECT COLLATION_NAME FROM information_schema.COLLATIONS WHERE IS_DEFAULT = 'Yes' AND CHARACTER_SET_NAME = 'utf8mb4';`).
				Scan(&mysqlDefaultCollationForUTF8MB4)).To(Succeed())

			var collationServer string
			Expect(db.QueryRow(`SELECT @@global.collation_server`).Scan(&collationServer)).To(Succeed())
			Expect(collationServer).To(Equal(mysqlDefaultCollationForUTF8MB4))
		})

		It("configures a mysql-backup user", func() {
			rows, err := db.Query(`SHOW GRANTS FOR 'mysql-backup'@'localhost'`)
			Expect(err).NotTo(HaveOccurred())
			var grants []string
			for rows.Next() {
				var grant string
				Expect(rows.Scan(&grant)).To(Succeed())
				grants = append(grants, grant)
			}
			Expect(rows.Err()).ToNot(HaveOccurred())
			Expect(grants).ToNot(BeEmpty())
		})

		It("configures a user with access to a set of schemas based on a pattern from the seeded_users configuration", func() {
			db, err := sql.Open("mysql", "test-multi-schema-user:secret-multi-schema-admin-credential@tcp("+proxyHost+")/?tls=skip-verify&interpolateParams=true")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			db.SetMaxIdleConns(0)
			db.SetMaxOpenConns(1)

			Expect(db.Exec(`SELECT * FROM mysql.user`)).Error().To(MatchError(ContainSubstring(`SELECT command denied to user`)),
				`Expected a multi-schema-admin user to NOT have access to sensitive mysql system tables, but successfully read mysql.user!`)

			Expect(db.Exec(`CREATE DATABASE multi_db1`)).
				Error().NotTo(HaveOccurred())
			Expect(db.Exec(`CREATE TABLE multi_db1.t1 (id int primary key auto_increment, data varchar(36))`)).
				Error().NotTo(HaveOccurred())

			Expect(db.Exec(`CREATE DATABASE multi_db2`)).
				Error().NotTo(HaveOccurred())
			Expect(db.Exec(`CREATE TABLE multi_db2.t1 (id int primary key auto_increment, data varchar(36))`)).
				Error().NotTo(HaveOccurred())

			var (
				userValue   string
				storedValue string
			)
			userValue = uuid.NewString()
			Expect(db.Exec(`INSERT INTO multi_db1.t1 (data) VALUES (?)`, userValue)).
				Error().NotTo(HaveOccurred())

			Expect(db.QueryRow(`SELECT data FROM multi_db1.t1 WHERE id = 1`).Scan(&storedValue)).To(Succeed())
			Expect(storedValue).To(Equal(userValue))

			userValue = uuid.NewString()
			Expect(db.Exec(`INSERT INTO multi_db2.t1 (data) VALUES (?)`, userValue)).
				Error().NotTo(HaveOccurred())
			Expect(db.QueryRow(`SELECT data FROM multi_db2.t1 WHERE id = 1`).Scan(&storedValue)).To(Succeed())
			Expect(storedValue).To(Equal(userValue))
		})

		It("sets expected default values for selected properties", func() { // Ensure these values change on later redeployment test
			var maxAllowedPacket string
			Expect(db.QueryRow(`SELECT @@global.max_allowed_packet`).Scan(&maxAllowedPacket)).To(Succeed())
			Expect(maxAllowedPacket).To(Equal("268435456"), "max_allowed_packet value is not at expected 256M default value")

			var idbParallelThreads string
			Expect(db.QueryRow(`SELECT @@global.innodb_compression_level`).Scan(&idbParallelThreads)).To(Succeed())
			Expect(idbParallelThreads).To(Equal("6"), "innodb_compression_level value is not at expected 6 default value")
		})
	})

	Context("MySQL Configuration Tuning (autotune)", Label("autotune"), func() {
		totalVmDiskSize := func(instance string) float64 {
			diskSizeInBytesStr, err := bosh.RemoteCommand(deploymentName, instance, `df --output=size --block-size=1 /var/vcap/store/ | sed '1d'`)
			Expect(err).NotTo(HaveOccurred())

			result, err := strconv.ParseFloat(diskSizeInBytesStr, 64)
			Expect(err).NotTo(HaveOccurred())

			return result
		}

		It("observes a correctly configured innodb-buffer-pool-size based on the provided spec parameters", func() {
			memInMiBStr, err := bosh.RemoteCommand(deploymentName, "mysql/0", `awk '/MemTotal:/ {print $2/1024.0}' /proc/meminfo`)
			Expect(err).NotTo(HaveOccurred())
			totalMemInKiB, err := strconv.ParseFloat(memInMiBStr, 64)
			Expect(err).NotTo(HaveOccurred())

			var innodbBufferPoolSizeInMiB float64
			Expect(db.QueryRow(`SELECT @@global.innodb_buffer_pool_size / 1024 / 1024`).Scan(&innodbBufferPoolSizeInMiB)).To(Succeed())

			expectedBufferPoolSize := totalMemInKiB * 0.14
			if expectedBufferPoolSize > 1024 {
				expectedBufferPoolSize = math.Ceil(expectedBufferPoolSize/1024) * 1024
			} else {
				expectedBufferPoolSize = math.Ceil(expectedBufferPoolSize/128) * 128
			}

			Expect(int(innodbBufferPoolSizeInMiB)).To(Equal(int(expectedBufferPoolSize)))
		})

		It("observes correctly configured binlog-space-limit", func() {
			const binlogBlockSize = 4 * 1024

			vmTotalDiskInBytes := totalVmDiskSize("mysql/0")

			var (
				binlogSpaceLimit int
				maxBinlogSize    int
			)
			Expect(db.QueryRow(`SELECT @@global.binlog_space_limit`).Scan(&binlogSpaceLimit)).To(Succeed())
			Expect(db.QueryRow("SELECT @@global.max_binlog_size").Scan(&maxBinlogSize)).To(Succeed())

			expectedbinlogSpaceLimit := vmTotalDiskInBytes * 0.2

			expectedmaxBinlogSize := uint64(expectedbinlogSpaceLimit / 3)
			expectedmaxBinlogSize = (expectedmaxBinlogSize / binlogBlockSize) * binlogBlockSize
			if expectedmaxBinlogSize > 1024*1024*1024 {
				expectedmaxBinlogSize = 1024 * 1024 * 1024
			}

			Expect(binlogSpaceLimit).To(Equal(int(expectedbinlogSpaceLimit)))
			Expect(maxBinlogSize).To(Equal(int(expectedmaxBinlogSize)))
		})
	})

	Context("ClusterHealthLogger", Label("cluster-health-logger"), func() {
		It("writes metrics to the cluster health logging file", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "cluster-health-logger/cluster_health.log")
			Expect(err).NotTo(HaveOccurred())

			Expect(output.String()).
				To(ContainSubstring(`timestamp|wsrep_local_state_uuid|wsrep_protocol_version|wsrep_last_applied|wsrep_last_committed`),
					`Expected to find the expected header in the bosh logs output, but did not.  Output: %s Attempt: %d`, output.String())
		})

		It("does not write errors to the stderr file", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "cluster-health-logger/cluster-health-logger.stderr.log")
			Expect(err).NotTo(HaveOccurred())

			Expect(output.String()).NotTo(ContainSubstring(`Access denied for user 'cluster-health-logger'`))
		})
	})

	Context("TLS", Label("tls"), func() {
		BeforeAll(func() {
			Expect(mysql.RegisterTLSConfig("deprecated-tls11", &tls.Config{
				MaxVersion:         tls.VersionTLS11,
				InsecureSkipVerify: true,
			})).To(Succeed())
			Expect(mysql.RegisterTLSConfig("tls12", &tls.Config{
				MaxVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			})).To(Succeed())
			Expect(mysql.RegisterTLSConfig("tls13", &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
			})).To(Succeed())
		})

		It("requires a secure transport for client connections", func() {
			dsn := "test-admin:integration-tests@tcp(" + proxyHost + ":3306)/?tls=false"
			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			err = db.Ping()
			Expect(err).To(MatchError(ContainSubstring(`Connections using insecure transport are prohibited while --require_secure_transport=ON.`)))
		})

		It("rejects TLSv11 connections", func() {
			dsn := "test-admin:integration-tests@tcp(" + proxyHost + ":3306)/?tls=deprecated-tls11"
			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			err = db.Ping()
			Expect(err).To(MatchError(`tls: no supported versions satisfy MinVersion and MaxVersion`))
		})
		It("accepts TLSv1.2 for connections", func() {
			dsn := "test-admin:integration-tests@tcp(" + proxyHost + ":3306)/?tls=tls12"
			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			err = db.Ping()
			Expect(err).NotTo(HaveOccurred())
		})
		It("accepts TLSv1.3 for connections", func() {
			if expectedMysqlVersion == "5.7" {
				Skip("Skipping TLSv1.3 assertion because MySQL 5.7 does not support TLSv1.3.")
			}
			dsn := "test-admin:integration-tests@tcp(" + proxyHost + ":3306)/?tls=tls13"
			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			err = db.Ping()
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts valid TLS connections", func() {
			// certificates aren't setup such that we can do proper TLS verification
			// This test exists to prove TLS < v1.2, fails but normal TLS connections succeed
			dsn := "test-admin:integration-tests@tcp(" + proxyHost + ":3306)/?tls=skip-verify"
			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			Expect(db.Ping()).To(Succeed())
		})
	})

	Context("mutual-tls: MySQL x509 authentication", Label("mtls"), func() {
		var (
			username string
			password string
		)

		BeforeAll(func() {
			username = strings.Replace(uuid.New().String(), "-", "", -1)
			password = uuid.New().String()
			_, err := db.Exec(`CREATE USER ?@'%' IDENTIFIED BY ? REQUIRE SUBJECT '/CN=mysql_client_certificate'`, username, password)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			_, err := db.Exec(`DROP USER IF EXISTS ?@'%'`, username)
			Expect(err).NotTo(HaveOccurred())
		})

		connectWithTLSConfig := func(username, password, tlsConfigName string) (*sql.DB, error) {
			dsn := fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/?tls=%s",
				username,
				password,
				proxyHost,
				3306,
				tlsConfigName,
			)

			return sql.Open("mysql", dsn)
		}

		createUntrustedCertificate := func() (tls.Certificate, error) {
			untrustedAuthority, err := certtest.BuildCA("some-CA")
			if err != nil {
				return tls.Certificate{}, err
			}

			cert, err := untrustedAuthority.BuildSignedCertificate("client")
			if err != nil {
				return tls.Certificate{}, err
			}

			return cert.TLSCertificate()
		}

		When("connecting without client certificates", func() {
			It("should reject the connection attempt", func() {
				db, err := connectWithTLSConfig(username, password, "false")
				Expect(err).NotTo(HaveOccurred())
				defer db.Close()
				Expect(db.Ping()).To(MatchError(ContainSubstring(`Access denied for user '%s'`, username)))
			})
		})

		When("connecting with untrusted client certificates", func() {
			BeforeEach(func() {
				untrustedCert, err := createUntrustedCertificate()
				Expect(err).NotTo(HaveOccurred())

				Expect(mysql.RegisterTLSConfig("untrusted-tls", &tls.Config{
					Certificates: []tls.Certificate{
						untrustedCert,
					},
					MaxVersion:         tls.VersionTLS12,
					InsecureSkipVerify: true,
				})).To(Succeed())
			})

			It("should reject the connection attempt", func() {
				db, err := connectWithTLSConfig(username, password, "untrusted-tls")
				Expect(err).NotTo(HaveOccurred())
				defer db.Close()
				Expect(db.Ping()).To(SatisfyAny(
					MatchError(ContainSubstring(`tls: unknown certificate authority`)),
					// PXC 8 using OpenSSL v3 seems to give us a different error indicating the same problem
					MatchError(ContainSubstring(`tls: bad certificate`)),
				))
			})
		})

		When("connecting with valid client certificate", func() {
			BeforeEach(func() {
				cert, err := credhub.GetCredhubCertificate(`/` + deploymentName + `/mysql_client_certificate`)
				Expect(err).NotTo(HaveOccurred())

				trustedCert, err := tls.X509KeyPair([]byte(cert.Certificate), []byte(cert.PrivateKey))
				Expect(err).NotTo(HaveOccurred())

				Expect(mysql.RegisterTLSConfig("trusted-tls", &tls.Config{
					Certificates:       []tls.Certificate{trustedCert},
					InsecureSkipVerify: true,
				})).To(Succeed())
			})

			It("should allow the connection attempt", func() {
				db, err := connectWithTLSConfig(username, password, "trusted-tls")
				Expect(err).NotTo(HaveOccurred())
				defer db.Close()
				Expect(db.Ping()).To(Succeed())
			})
		})
	})

	Context("Remote Admin Access", Label("remote-admin-access"), func() {
		It("does not allow access to mysql from anywhere besides localhost", func() {
			password, err := credhub.GetCredhubPassword(`/` + deploymentName + `/cf_mysql_mysql_admin_password`)
			Expect(err).NotTo(HaveOccurred())

			dsn := fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/?tls=%s&interpolateParams=true",
				"root",
				password,
				proxyHost,
				3306,
				"skip-verify",
			)

			db, err := sql.Open("mysql", dsn)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()
			var result int
			err = db.QueryRow(`SELECT 1`).Scan(&result)
			Expect(err).To(MatchError(MatchRegexp("is not allowed to connect to this MySQL server|Access denied for user")))
		})
	})

	Context("Slow query logs", Label("slow-query"), func() {
		const (
			slowqueryLogPath = "/var/vcap/sys/log/pxc-mysql/mysql_slow_query"
		)

		var (
			activeBackend string
			tmpdir        string
		)

		getLogContents := func(db *sql.DB, activeBackend string, fileName string) string {
			Expect(bosh.Scp(deploymentName, activeBackend+":"+slowqueryLogPath, tmpdir)).To(Succeed())
			logContents, err := os.ReadFile(filepath.Join(tmpdir, fileName))
			Expect(err).NotTo(HaveOccurred())
			return string(logContents)
		}

		BeforeAll(func() {
			var err error
			tmpdir, err = os.MkdirTemp("", "slow_query_logs_")
			Expect(err).NotTo(HaveOccurred())

			Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).Scan(&activeBackend)).To(Succeed())
		})

		It("logs slow queries with details", func() {
			Expect(db.Exec(`DO sleep(10)`)).Error().NotTo(HaveOccurred())

			contents := getLogContents(db, activeBackend, "mysql_slow_query")

			Expect(contents).To(ContainSubstring("Tmp_tables: 0  Tmp_disk_tables: 0  Tmp_table_sizes: 0"))
			Expect(contents).To(ContainSubstring("Full_scan: No  Full_join: No  Tmp_table: No  Tmp_table_on_disk: No"))
			Expect(contents).To(ContainSubstring("Filesort: No  Filesort_on_disk: No  Merge_passes: 0"))
			Expect(contents).To(ContainSubstring("No InnoDB statistics available for this query"))
			Expect(contents).To(ContainSubstring("DO sleep(10);"))
		})
		It("syslog does not forward slow queries", func() {
			// execute a slow query
			Expect(db.Exec(`DO sleep(10)`)).Error().NotTo(HaveOccurred())
			// fetch forwarded logs
			output, err := bosh.RemoteCommand(deploymentName, "syslog_storer", "cat /var/vcap/store/syslog_storer/syslog.log | grep '47450'")
			Expect(err).NotTo(HaveOccurred())

			// assert the forwarded logs do not contain any slow queries
			Expect(output).To(Not(ContainSubstring("Tmp_tables: 0  Tmp_disk_tables: 0  Tmp_table_sizes: 0")))
			Expect(output).To(Not(ContainSubstring("Full_scan: No  Full_join: No  Tmp_table: No  Tmp_table_on_disk: No")))
			Expect(output).To(Not(ContainSubstring("Filesort: No  Filesort_on_disk: No  Merge_passes: 0")))
			Expect(output).To(Not(ContainSubstring("No InnoDB statistics available for this query")))
			Expect(output).To(Not(ContainSubstring("DO sleep(10);")))
		})
		It("rotates the slow query log", func() {
			By("allocating 51 MiB to the mysql_slow_query log")
			_, err := bosh.RemoteCommand(deploymentName, "mysql/0", "sudo fallocate -l 51MiB /var/vcap/sys/log/pxc-mysql/mysql_slow_query")
			Expect(err).NotTo(HaveOccurred())
			// call logrotate utility
			By("rotating the mysql_slow_query log")
			_, err = bosh.RemoteCommand(deploymentName, "mysql/0", "sudo /usr/sbin/logrotate /etc/logrotate.conf")
			Expect(err).NotTo(HaveOccurred())
			// assert the slow query log gets rotated
			By("asserting the log is now less than 51 MiB")
			output, err := bosh.RemoteCommand(deploymentName, "mysql/0", "stat --printf='%s' /var/vcap/sys/log/pxc-mysql/mysql_slow_query")
			var size int
			size, err = strconv.Atoi(output)
			Expect(err).NotTo(HaveOccurred())
			By("asserting a new log was created")
			Expect(size).Should(BeNumerically("<", 53477376))
			_, err = bosh.RemoteCommand(deploymentName, "mysql/0", "test -f /var/vcap/sys/log/pxc-mysql/mysql_slow_query.1.gz")
			Expect(err).NotTo(HaveOccurred())

		})
	})

	Context("Audit Logs", Label("audit-logs"), func() {
		const (
			databaseName      = "pxc_release_test_db"
			auditLogDirectory = "/var/vcap/store/mysql_audit_logs"
			auditLogPath      = "/var/vcap/store/mysql_audit_logs/mysql_server_audit.log"
		)

		var (
			activeBackend string
			tmpdir        string
		)

		cleanupUsers := func(db *sql.DB, mysqlUsername string) {
			query := fmt.Sprintf("DROP USER IF EXISTS %s;", mysqlUsername)
			_, err := db.Exec(query)
			Expect(err).NotTo(HaveOccurred())
		}

		createUserWithPermissions := func(db *sql.DB, databaseName, mysqlUsername, mysqlPassword string) {
			cleanupUsers(db, mysqlUsername)
			Expect(db.Exec("CREATE USER ? IDENTIFIED BY ?", mysqlUsername, mysqlPassword)).
				Error().NotTo(HaveOccurred())
			Expect(db.Exec("GRANT ALL ON `"+databaseName+"`.* TO ?", mysqlUsername)).
				Error().NotTo(HaveOccurred())
		}

		readAndWriteFromDB := func(db *sql.DB) {
			Expect(db.Query("REPLACE INTO pxc_release_test_db.audit_logging_test_table VALUES('writing data')")).
				Error().NotTo(HaveOccurred())
			Expect(db.Query(`SELECT * FROM pxc_release_test_db.audit_logging_test_table`)).
				Error().NotTo(HaveOccurred())
		}

		readAndWriteDataAndGetAuditLogContents := func(db *sql.DB, activeBackend string) string {
			readAndWriteFromDB(db)
			Expect(bosh.Scp(deploymentName, activeBackend+":"+auditLogPath, tmpdir)).
				To(Succeed())
			auditLogContents, err := os.ReadFile(filepath.Join(tmpdir, "mysql_server_audit.log"))
			Expect(err).NotTo(HaveOccurred())
			return string(auditLogContents)
		}

		enableAccessToAuditLogs := func(backend string) {
			Expect(bosh.RemoteCommand(
				deploymentName,
				backend,
				`sudo chmod g+rx `+auditLogDirectory,
			)).Error().To(Succeed())
		}

		BeforeAll(func() {
			var err error
			tmpdir, err = os.MkdirTemp("", "audit_logs_")
			Expect(err).NotTo(HaveOccurred())

			Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).
				Scan(&activeBackend)).To(Succeed())
			Expect(db.Exec(`CREATE DATABASE IF NOT EXISTS pxc_release_test_db`)).
				Error().NotTo(HaveOccurred())
			Expect(db.Exec(`CREATE TABLE IF NOT EXISTS pxc_release_test_db.audit_logging_test_table (test_data varchar(255) PRIMARY KEY)`)).
				Error().NotTo(HaveOccurred())

			enableAccessToAuditLogs(activeBackend)
		})

		AfterAll(func() {
			Expect(os.RemoveAll(tmpdir)).To(Succeed())
		})

		When("reading and writing data as an excluded user", func() {
			var (
				excludedUser, excludedUserPassword string
			)

			When("the excluded user is from csv", func() {
				BeforeAll(func() {
					excludedUser = "excludeDBAudit1"
					excludedUserPassword = "password"

					createUserWithPermissions(db, databaseName, excludedUser, excludedUserPassword)
				})

				It("does not log any of the excluded user's activity in the audit log", func() {
					dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?tls=skip-verify", excludedUser, excludedUserPassword, proxyHost, 3306)
					db, err := sql.Open("mysql", dsn)
					Expect(err).NotTo(HaveOccurred())
					auditLogContents := readAndWriteDataAndGetAuditLogContents(db, activeBackend)
					Expect(auditLogContents).ToNot(ContainSubstring("\"user\":\"excludeDBAudit1[excludeDBAudit1]"))
				})
			})

			When("the excluded user is not from csv", func() {
				BeforeEach(func() {
					excludedUser = "excludeDBAudit2"
					excludedUserPassword = "password"

					createUserWithPermissions(db, databaseName, excludedUser, excludedUserPassword)
				})

				It("does not log any of the excluded user's activity in the audit log", func() {
					dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?tls=skip-verify", excludedUser, excludedUserPassword, proxyHost, 3306)
					db, err := sql.Open("mysql", dsn)
					Expect(err).NotTo(HaveOccurred())
					auditLogContents := readAndWriteDataAndGetAuditLogContents(db, activeBackend)
					Expect(auditLogContents).ToNot(ContainSubstring("\"user\":\"excludeDBAudit2[excludeDBAudit2]"))
				})
			})
		})

		When("reading and writing data as an included user", func() {
			var (
				includedUser, includedUserPassword string
			)

			BeforeEach(func() {
				includedUser = "included_user"
				includedUserPassword = "password"

				createUserWithPermissions(db, databaseName, includedUser, includedUserPassword)
			})

			It("does log all of the included user's activity in the audit log", func() {
				dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?tls=skip-verify", includedUser, includedUserPassword, proxyHost, 3306)

				db, err := sql.Open("mysql", dsn)
				Expect(err).NotTo(HaveOccurred())
				auditLogContents := readAndWriteDataAndGetAuditLogContents(db, activeBackend)
				Expect(auditLogContents).To(ContainSubstring("\"user\":\"included_user[included_user]"))
			})

			It("does NOT log the user's LOGIN event in the audit log", func() {
				dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?tls=skip-verify", includedUser, includedUserPassword, proxyHost, 3306)

				db, err := sql.Open("mysql", dsn)
				Expect(err).NotTo(HaveOccurred())
				auditLogContents := readAndWriteDataAndGetAuditLogContents(db, activeBackend)
				Expect(auditLogContents).To(ContainSubstring("\"user\":\"included_user[included_user]"))
				Expect(auditLogContents).ToNot(ContainSubstring("{\"audit_record\":{\"name\":\"Connect\""))
			})
		})
	})

	Context("Proxy", Label("proxy"), func() {
		Describe("/v0/backends proxy api", func() {
			type Backend struct {
				Host                string `json:"host"`
				Port                int    `json:"port"`
				Healthy             bool   `json:"healthy"`
				Name                string `json:"name"`
				CurrentSessionCount int    `json:"currentSessionCount"`
				Active              bool   `json:"active"`
				TrafficEnabled      bool   `json:"trafficEnabled"`
			}

			It("reports the backend name with the full BOSH instance/uuid name", func() {
				req, err := http.NewRequest(http.MethodPost, "http://"+proxyHost+":8080/v0/backends", nil)
				Expect(err).NotTo(HaveOccurred())

				proxyPassword, err := credhub.GetCredhubPassword("/" + deploymentName + "/cf_mysql_proxy_api_password")
				Expect(err).NotTo(HaveOccurred())
				req.SetBasicAuth("proxy", proxyPassword)
				req.Header.Set("X-Forwarded-Proto", "https")

				res, err := httpClient.Do(req)
				Expect(err).NotTo(HaveOccurred())

				Expect(res.StatusCode).To(Equal(http.StatusOK), `Expected 200 OK but got %q`, res.Status)

				body, _ := io.ReadAll(res.Body)
				var backends []Backend
				Expect(json.Unmarshal(body, &backends)).To(Succeed())
				Expect(len(backends)).To(Equal(3))

				instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
				for _, i := range instances {
					name := i.Instance
					Expect(backends).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Name": Equal(name),
					})))
				}
			})
		})
	})

	Context("read_write_permissions", Label("read_write_permissions"), func() {
		It("can redeploy all MySQL nodes with read_only enabled", func() {
			Expect(bosh.RedeployPXC(deploymentName,
				bosh.Operation(`enable-read-only.yml`),
			)).To(Succeed())

			mysqlIps, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())

			for _, ip := range mysqlIps {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+ip+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var readOnly bool
				Expect(db.QueryRow(`SELECT @@global.read_only`).Scan(&readOnly)).To(Succeed())
				Expect(readOnly).To(BeTrue())
				Expect(db.Close())
			}
		})

		It("can connect to the MySQL nodes via the proxy", func() {
			Expect(bosh.RedeployPXC(deploymentName,
				bosh.Operation(`enable-read-only.yml`),
			)).To(Succeed())

			err := db.Ping()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("jemalloc", Label("jemalloc"), func() {
		It("uses jemalloc", func() {

			_, err := bosh.RemoteCommand(deploymentName, "mysql/0", "sudo grep -q jemalloc /proc/$(pidof mysqld)/maps")
			Expect(err).NotTo(HaveOccurred(),
				`Expected to see jemalloc in the memory map of the mysqld process, but it was not`)

			if expectedMysqlVersion != "8.0" {
				// MySQL v5.7 does not support the performance_schema.malloc_* tables present in MySQL v8.0
				return
			}

			var jemallocAllocated uint64
			Expect(db.QueryRow(`SELECT ALLOCATED FROM performance_schema.malloc_stats_totals`).
				Scan(&jemallocAllocated)).To(Succeed())

			Expect(jemallocAllocated).To(BeNumerically(">", 0),
				`Expected to see allocations from jemalloc but performance_schema.malloc_stats_totals.ALLOCATE was zero!`)
		})
	})

	When("redeploying with additional feature flags", func() {
		BeforeAll(func() {
			if expectedMysqlVersion != "8.0" {
				Skip("MYSQL_VERSION(" + expectedMysqlVersion + ") != 8.0. Skipping Percona v8.0+ jemalloc profiling feature test.")
			}

			By("enabling jemalloc profiling")
			By("enabling O_DIRECT")
			By("disabling sync_binlog")
			By("applying additional my.cnf entries")

			Expect(bosh.RedeployPXC(deploymentName,
				bosh.Operation("enable-jemalloc-profiling.yml"),
				bosh.Operation(`set-innodb-flush-method.yml`),
				bosh.Var(`innodb_flush_method`, `O_DIRECT`),
				bosh.Operation(`set-sync-binlog.yml`),
				bosh.Var(`sync_binlog`, `0`),
				bosh.Operation("test/additional-mycnf-entries.yml"),
			)).To(Succeed())
		})

		It("sets the explicitly configured sync_binlog value", Label("sync_binlog"), func() {
			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var syncBinlog string
				Expect(db.QueryRow("SELECT @@global.sync_binlog").Scan(&syncBinlog)).
					To(Succeed())
				Expect(syncBinlog).To(Equal(`0`))
				Expect(db.Close()).To(Succeed())
			}
		})

		It("sets the expected innodb_flush_method", Label("innodb_flush_method"), func() {
			instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			for _, i := range instances {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+i.IP+")/?tls=skip-verify&interpolateParams=true")
				Expect(err).NotTo(HaveOccurred())

				var innodbFlushMethod string
				Expect(db.QueryRow("SELECT @@global.innodb_flush_method").Scan(&innodbFlushMethod)).
					To(Succeed())
				Expect(innodbFlushMethod).To(Equal(`O_DIRECT`))
				Expect(db.Close()).To(Succeed())
			}
		})

		It("does not write memory profiles by default", func() {
			out, err := bosh.RemoteCommand(deploymentName, "mysql/0", "sudo find /var/vcap/data/pxc-mysql/tmp/ -type f -name 'jeprof*'")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal("-"), `Expected no memory profiles to be found in /var/vcap/data/pxc-mysql/tmp/`)
		})

		It("enables access to jemalloc memory profile when administrative commands are run", func() {
			By("writing profile files to the ephemeral disk after FLUSH MEMORY PROFILE is run")
			Expect(db.Exec(`FLUSH MEMORY PROFILE`)).Error().NotTo(HaveOccurred())

			var mysqlNode string
			Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).Scan(&mysqlNode)).To(Succeed())

			out, err := bosh.RemoteCommand(deploymentName, mysqlNode, "sudo find /var/vcap/data/pxc-mysql/tmp/ -type f -name 'jeprof*'")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(HavePrefix("/var/vcap/data/pxc-mysql/tmp/jeprof_mysqld"), `Expected a memory profile to be generated, but it was not.`)

			By("providing the jeprof utility as part of the jemalloc package for generating human readable memory profile reports")
			out, err = bosh.RemoteCommand(deploymentName,
				mysqlNode, "sudo /var/vcap/packages/jemalloc/bin/jeprof --show_bytes --text /var/vcap/packages/percona-xtradb-cluster-8.0/bin/mysqld "+out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(MatchRegexp(`Total: \d+ B`))
		})

		Context("dynamically injecting my.cnf entries", func() {
			It("sets a net new property within an existing section", func() {
				var idbParallelThreads string
				Expect(db.QueryRow(`SELECT @@global.innodb_compression_level`).Scan(&idbParallelThreads)).To(Succeed())
				Expect(idbParallelThreads).To(Equal("0"), "dynamic my.cnf failed to configure mysql with the expected innodb_compression_level")
			})
			It("overrides the value of an existing property (max_allowed_packet)", func() {
				var maxAllowedPacket string
				Expect(db.QueryRow(`SELECT @@global.max_allowed_packet`).Scan(&maxAllowedPacket)).To(Succeed())
				Expect(maxAllowedPacket).To(Equal("1073741824"), "dynamic my.cnf failed to configure mysql with the expected max_allowed_packet parameter")
			})
			It("creates a net new section with properties consumable by mysql processes", func() {
				xtrabackupPath := "/var/vcap/packages/percona-xtrabackup-" + expectedMysqlVersion + "/bin/"
				xtrabackupOptions, err := bosh.RemoteCommand(deploymentName, "mysql/0", "sudo "+
					xtrabackupPath+"xtrabackup --defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf --help --verbose | grep target")
				Expect(err).NotTo(HaveOccurred()) // note "xtrabackup --help ..." returns 1
				Expect(xtrabackupOptions).To(MatchRegexp(`target-dir\s*\/var\/vcap\/store\/custom-backup-dir\/`), "dynamic my.cnf failed to configure xtrabackup with the expected target-dir parameter")
			})
		})
	})
})
