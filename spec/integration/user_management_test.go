package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"integration/internal/docker"
)

type UserRole struct {
	Password           string `json:"password"`
	Role               string `json:"role"`
	Schema             string `json:"schema,omitempty"`
	Host               string `json:"host"`
	MaxUserConnections int    `json:"max_user_connections,omitempty"`
}

type SeededDatabaseEntry struct {
	Schema   string `json:"name"`
	Password string `json:"password"`
	Username string `json:"username"`
}

type MySQLJobSpec struct {
	MySQLVersion        string                `json:"mysql_version,omitempty"`
	MySQLBackupPassword string                `json:"mysql_backup_password,omitempty"`
	SeededUsers         map[string]UserRole   `json:"seeded_users"`
	SeededDatabases     []SeededDatabaseEntry `json:"seeded_databases"`
}

var _ = Describe("UserManagement", Ordered, func() {
	var (
		dbUsers         MySQLJobSpec
		resource        string
		mysqlVersionTag string
	)

	BeforeEach(func() {
		mysqlVersionTag = "8.0"
		dbUsers = MySQLJobSpec{}
	})

	JustBeforeEach(func() {
		doc, err := json.Marshal(dbUsers)
		Expect(err).NotTo(HaveOccurred())

		f, err := os.CreateTemp("", "db_init_")
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()
		Expect(os.Chmod(f.Name(), 0644)).To(Succeed())

		cmd := exec.Command("./scripts/render-db_init")
		cmd.Stdin = bytes.NewBuffer(doc)
		cmd.Stdout = f
		cmd.Stderr = GinkgoWriter
		GinkgoWriter.Println("$ ./scripts/render-db_init")
		Expect(cmd.Run()).To(Succeed())

		// Initialize the data volume first, so our db_init does not interfere with percona's entrypoint bootstrapping
		resource = startMySQL(mysqlVersionTag, nil, nil)
		Expect(docker.RemoveContainer(resource)).To(Succeed())

		resource = startMySQL(
			mysqlVersionTag,
			[]string{"--init-file=/db_init"},
			[]string{f.Name() + ":/db_init"},
		)
		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}

			Expect(docker.RemoveContainer(resource)).To(Succeed())
			Expect(docker.RemoveVolume(volumeID)).To(Succeed())
		})
	})

	showGrants := func(db *sql.DB, username, host string) (grants []string) {
		GinkgoHelper()

		rows, err := db.Query(`SHOW GRANTS FOR ?@?`, username, host)
		Expect(err).NotTo(HaveOccurred())
		for rows.Next() {
			var grant string
			Expect(rows.Scan(&grant)).To(Succeed())
			grants = append(grants, grant)
		}

		return grants
	}

	showSchemas := func(db *sql.DB, username, host string) (schemas []string) {
		GinkgoHelper()

		rows, err := db.Query(`SHOW SCHEMAS`)
		Expect(err).NotTo(HaveOccurred())
		for rows.Next() {
			var s string
			Expect(rows.Scan(&s)).To(Succeed())
			schemas = append(schemas, s)
		}

		return schemas
	}

	// verifyUser proves that a given username:password credential can connect to a given schema and has the expected permissions
	verifyUser := func(username, password, schema string, expectedGrants []string) {
		db := docker.MySQLDB(resource, docker.WithUsernamePassword(username, password))
		defer db.Close()
		Expect(showGrants(db, username, "%")).To(ConsistOf(expectedGrants))
	}

	verifyGrantsForLocalUser := func(username, password string, expectedGrants []string) {
		db := docker.MySQLDB(resource)
		defer db.Close()

		Expect(showGrants(db, username, "localhost")).To(ConsistOf(expectedGrants))
	}

	verifySchemasForLocalUser := func(username, password string, expectedSchemas []string) {
		db := docker.MySQLDB(resource)
		defer db.Close() // TODO: check other helpers for Close() calls.

		Expect(showSchemas(db, username, "localhost")).To(ContainElements(expectedSchemas))
	}

	queryAvailablePrivileges := func(db *sql.DB) (privileges []string, err error) {
		rows, err := db.Query(`SHOW PRIVILEGES`)
		if err != nil {
			return nil, err
		}
		Expect(err).NotTo(HaveOccurred())

		for rows.Next() {
			var privName, unused string
			if err = rows.Scan(&privName, &unused, &unused); err != nil {
				return nil, err
			}
			privileges = append(privileges, strings.ToUpper(privName))
		}

		return privileges, rows.Err()
	}

	queryGrantedPrivileges := func(db *sql.DB, username string) (grantedPrivs []string, err error) {
		rows, err := db.Query(`SHOW GRANTS FOR ?@localhost`, username)
		if err != nil {
			return nil, err
		}

		var foundGrantOption bool
		for rows.Next() {
			var grant string
			if err := rows.Scan(&grant); err != nil {
				return nil, err
			}

			if !foundGrantOption && strings.HasSuffix(grant, "WITH GRANT OPTION") {
				grantedPrivs = append(grantedPrivs, "GRANT OPTION")
				foundGrantOption = true
			}

			privs := strings.SplitN(strings.TrimPrefix(grant, "GRANT "), " ON ", 2)[0]
			for _, p := range strings.Split(privs, ",") {
				grantedPrivs = append(grantedPrivs, strings.TrimSpace(p))
			}
		}

		grantedPrivs = append(grantedPrivs, "USAGE")

		return grantedPrivs, rows.Err()
	}

	verifyLocalUser := func(username, password string) {
		var out bytes.Buffer
		cmd := exec.Command("docker",
			"exec", resource,
			"mysql",
			"--user="+username,
			"--password="+password,
			"--batch", "--silent",
			"--execute=SELECT 1",
		)
		cmd.Stdout = &out
		Expect(cmd.Run()).To(Succeed(), `Expected to be able to log in with credentials for user %q, but this failed`, username)
	}

	verifyMaxUserConnections := func(username, host string, expectedValue int) {
		db := docker.MySQLDB(resource)
		defer db.Close()

		var actualMaxUserConnections int
		const query = `SELECT max_user_connections FROM mysql.user WHERE User = ? AND Host = ?`
		ExpectWithOffset(1, db.QueryRow(query, username, host).
			Scan(&actualMaxUserConnections)).To(Succeed(), `Failed to query max_user_connections for user %s@%s`, username, host)

		ExpectWithOffset(1, actualMaxUserConnections).To(Equal(expectedValue), `Expected max_user_connections = %d for user %s@%s`, expectedValue, username, host)
	}

	verifyLocalAdminUser := func(username, password string) {
		db := docker.MySQLDB(resource)
		defer db.Close()

		var (
			expectedPrivileges []string
			err                error
		)
		// MySQL 8.0 and later versions always enumerate specific privileges and does not emit "ALL PRIVILEGES"
		expectedPrivileges, err = queryAvailablePrivileges(db)
		Expect(err).NotTo(HaveOccurred())

		grantedPrivileges, err := queryGrantedPrivileges(db, username)
		Expect(err).NotTo(HaveOccurred())

		Expect(grantedPrivileges).To(ConsistOf(expectedPrivileges))

		// Validate the admin user can actually log in. I.e. we set credentials correctly
		cmd := exec.Command("docker",
			"exec", resource,
			"mysql",
			"--user="+username,
			"--password="+password,
			"--batch", "--silent",
			"--execute=SELECT 1",
		)
		Expect(cmd.Run()).To(Succeed())
	}

	verifyUserFunc := func(username, password string, cb func(db *sql.DB)) {
		db := docker.MySQLDB(resource, docker.WithUsernamePassword(username, password))
		defer db.Close()
		cb(db)
	}

	When("the mysql job is configured with user properties", func() {
		BeforeEach(func() {
			dbUsers = MySQLJobSpec{
				MySQLBackupPassword: uuid.NewString(),
				SeededDatabases: []SeededDatabaseEntry{
					{
						Schema:   "cloud_controller",
						Username: "ccdb",
						Password: uuid.NewString(),
					},
					// SeededUsers take precedence over SeededDatabases so this entry should be ignored
					{
						Schema:   "ignored",
						Username: "app-user1",
						Password: "ignored",
					},
				},
				SeededUsers: map[string]UserRole{
					"app-user1": {
						Role:     "schema-admin",
						Password: uuid.NewString(),
						Schema:   "app_user_db1",
						Host:     "any",
					},
					"app-user2": {
						Role:               "schema-admin",
						Password:           uuid.NewString(),
						Schema:             "app_user_db2",
						Host:               "any",
						MaxUserConnections: 42,
					},
					"healthcheck-user": {
						Role:     "minimal",
						Password: uuid.NewString(),
						Host:     "any",
					},
					"admin-user": {
						Role:     "admin",
						Password: uuid.NewString(),
						Host:     "localhost",
					},
					"mysql-metrics": {
						Role:               "mysql-metrics",
						Password:           uuid.NewString(),
						Host:               "any",
						Schema:             "metrics_db",
						MaxUserConnections: 3,
					},
					"multi-db-user": {
						Role:     "multi-schema-admin",
						Password: uuid.NewString(),
						Host:     "any",
						Schema:   `some\_db\_prefix\_%`,
					},
				},
			}
		})

		It("initializes the users successfully", func() {
			verifyUser("multi-db-user", dbUsers.SeededUsers["multi-db-user"].Password, "", []string{
				"GRANT USAGE ON *.* TO `multi-db-user`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `some\\_db\\_prefix\\_%`.* TO `multi-db-user`@`%`",
			})

			verifyUserFunc("root", "", func(db *sql.DB) {
				var schemaCount int
				err := db.QueryRow(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = 'some\_db\_prefix\_%'`).Scan(&schemaCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(schemaCount).To(Equal(0), `Expected the schema pattern for a multi-schema-admin role to NOT be created, but it was!`)
			})

			verifyUserFunc("multi-db-user", dbUsers.SeededUsers["multi-db-user"].Password, func(db *sql.DB) {
				var schemaCount int
				err := db.QueryRow(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME LIKE 'some\_db\_prefix\_%'`).Scan(&schemaCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(schemaCount).To(BeZero())

				Expect(db.Exec(`CREATE DATABASE some_db_prefix_a`)).Error().ShouldNot(HaveOccurred())
				Expect(db.Exec(`CREATE DATABASE some_db_prefix_special`)).Error().ShouldNot(HaveOccurred())
				Expect(db.Exec(`CREATE DATABASE mysql`)).Error().Should(MatchError(ContainSubstring(`Access denied`)))

				err = db.QueryRow(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME LIKE 'some\_db\_prefix\_%'`).Scan(&schemaCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(schemaCount).To(Equal(2))
			})

			verifyUser("mysql-metrics", dbUsers.SeededUsers["mysql-metrics"].Password, dbUsers.SeededUsers["mysql-metrics"].Schema, []string{
				"GRANT SELECT, PROCESS, REPLICATION CLIENT ON *.* TO `mysql-metrics`@`%`",
			})
			verifyMaxUserConnections("mysql-metrics", "%", 3)
			verifySchemasForLocalUser("mysql-metrics", dbUsers.SeededUsers["mysql-metrics"].Password, []string{"metrics_db"})

			verifyUser("ccdb", dbUsers.SeededDatabases[0].Password, dbUsers.SeededDatabases[0].Schema, []string{
				"GRANT USAGE ON *.* TO `ccdb`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `cloud\\_controller`.* TO `ccdb`@`%`",
			})
			verifyMaxUserConnections("ccdb", "%", 0)

			verifyUser("app-user1", dbUsers.SeededUsers["app-user1"].Password, dbUsers.SeededUsers["app-user1"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user1`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db1`.* TO `app-user1`@`%`",
			})
			verifyMaxUserConnections("app-user1", "%", 0)

			verifyUser("app-user2", dbUsers.SeededUsers["app-user2"].Password, dbUsers.SeededUsers["app-user2"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user2`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db2`.* TO `app-user2`@`%`",
			})
			verifyMaxUserConnections("app-user2", "%", 42)

			verifyUser("healthcheck-user", dbUsers.SeededUsers["healthcheck-user"].Password, "", []string{
				"GRANT USAGE ON *.* TO `healthcheck-user`@`%`",
			})
			verifyMaxUserConnections("healthcheck-user", "%", 0)

			verifyLocalAdminUser("admin-user", dbUsers.SeededUsers["admin-user"].Password)

			verifyLocalUser("mysql-backup", dbUsers.MySQLBackupPassword)
			verifyGrantsForLocalUser("mysql-backup", dbUsers.MySQLBackupPassword, []string{
				"GRANT RELOAD, PROCESS, LOCK TABLES, REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO `mysql-backup`@`localhost`",
				"GRANT BACKUP_ADMIN ON *.* TO `mysql-backup`@`localhost`",
				"GRANT SELECT ON `performance_schema`.`keyring_component_status` TO `mysql-backup`@`localhost`",
				"GRANT SELECT ON `performance_schema`.`log_status` TO `mysql-backup`@`localhost`",
			})
			verifyMaxUserConnections("mysql-backup", "localhost", 0)
		})
	})
})
