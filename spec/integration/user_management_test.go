package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

type UserRole struct {
	Password string `json:"password"`
	Role     string `json:"role"`
	Schema   string `json:"schema,omitempty"`
	Host     string `json:"host"`
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
		resource        *dockertest.Resource
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
		resource, err = startMySQL(mysqlVersionTag, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Close()).To(Succeed())

		resource, err = startMySQL(
			mysqlVersionTag,
			[]string{"--init-file=/db_init"},
			[]string{f.Name() + ":/db_init"},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			return
		}
		Expect(pool.Purge(resource)).To(Succeed())
		Expect(pool.Client.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{
			Name:  volumeID,
			Force: true,
		})).To(Succeed())
	})

	showGrants := func(db *sql.DB, username, host string) (grants []string) {
		rows, err := db.Query(`SHOW GRANTS FOR ?@?`, username, host)
		Expect(err).NotTo(HaveOccurred())
		for rows.Next() {
			var grant string
			Expect(rows.Scan(&grant)).To(Succeed())
			grants = append(grants, grant)
		}

		return grants
	}

	// verifyUser proves that a given username:password credential can connect to a given schema and has the expected permissions
	verifyUser := func(username, password, schema string, expectedGrants []string) {
		db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@(localhost:%s)/%s?interpolateParams=true",
			username, password, resource.GetPort("3306/tcp"), schema))
		Expect(err).NotTo(HaveOccurred())
		Expect(showGrants(db, username, "%")).To(ConsistOf(expectedGrants))
	}

	verifyGrantsForLocalUser := func(username, password string, expectedGrants []string) {
		dsn := fmt.Sprintf("root@(localhost:%s)/?interpolateParams=true", resource.GetPort("3306/tcp"))
		db, err := sql.Open("mysql", dsn)
		Expect(err).NotTo(HaveOccurred())
		Expect(showGrants(db, username, "localhost")).To(ConsistOf(expectedGrants))
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
			"exec", resource.Container.ID,
			"mysql",
			"--user="+username,
			"--password="+password,
			"--batch", "--silent",
			"--execute=SELECT 1",
		)
		cmd.Stdout = &out
		Expect(cmd.Run()).To(Succeed(), `Expected to be able to log in with credentials for user %q, but this failed`, username)
	}

	verifyLocalAdminUser := func(username, password string) {
		db, err := sql.Open("mysql", fmt.Sprintf("root@(localhost:%s)/mysql?interpolateParams=true",
			resource.GetPort("3306/tcp")))
		Expect(err).NotTo(HaveOccurred())

		var expectedPrivileges []string
		if mysqlVersionTag == "5.7" {
			expectedPrivileges = []string{"GRANT OPTION", "ALL PRIVILEGES", "PROXY", "USAGE"}
		} else {
			// MySQL 8.0 always enumerates specific privileges and does not emit "ALL PRIVILEGES"
			expectedPrivileges, err = queryAvailablePrivileges(db)
			Expect(err).NotTo(HaveOccurred())
		}

		grantedPrivileges, err := queryGrantedPrivileges(db, username)
		Expect(err).NotTo(HaveOccurred())

		Expect(grantedPrivileges).To(ConsistOf(expectedPrivileges))

		// Validate the admin user can actually log in. I.e. we set credentials correctly
		cmd := exec.Command("docker",
			"exec", resource.Container.ID,
			"mysql",
			"--user="+username,
			"--password="+password,
			"--batch", "--silent",
			"--execute=SELECT 1",
		)
		Expect(cmd.Run()).To(Succeed())
	}

	When("the mysql job is configured with user properties", func() {
		BeforeEach(func() {
			dbUsers = MySQLJobSpec{
				MySQLBackupPassword: uuid.NewString(),
				SeededDatabases: []SeededDatabaseEntry{
					{
						Schema:   "cloud_controller",
						Username: "ccdb",
						Password: uuid.New().String(),
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
						Password: uuid.New().String(),
						Schema:   "app_user_db1",
						Host:     "any",
					},
					"app-user2": {
						Role:     "schema-admin",
						Password: uuid.New().String(),
						Schema:   "app_user_db2",
						Host:     "any",
					},
					"healthcheck-user": {
						Role:     "minimal",
						Password: uuid.New().String(),
						Host:     "any",
					},
					"admin-user": {
						Role:     "admin",
						Password: uuid.New().String(),
						Host:     "localhost",
					},
				},
			}
		})

		It("initializes the users successfully", func() {
			verifyUser("ccdb", dbUsers.SeededDatabases[0].Password, dbUsers.SeededDatabases[0].Schema, []string{
				"GRANT USAGE ON *.* TO `ccdb`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `cloud\\_controller`.* TO `ccdb`@`%`",
			})
			verifyUser("app-user1", dbUsers.SeededUsers["app-user1"].Password, dbUsers.SeededUsers["app-user1"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user1`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db1`.* TO `app-user1`@`%`",
			})
			verifyUser("app-user2", dbUsers.SeededUsers["app-user2"].Password, dbUsers.SeededUsers["app-user2"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user2`@`%`",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db2`.* TO `app-user2`@`%`",
			})
			verifyUser("healthcheck-user", dbUsers.SeededUsers["healthcheck-user"].Password, "", []string{
				"GRANT USAGE ON *.* TO `healthcheck-user`@`%`",
			})

			verifyLocalAdminUser("admin-user", dbUsers.SeededUsers["admin-user"].Password)

			verifyLocalUser("mysql-backup", dbUsers.MySQLBackupPassword)
			verifyGrantsForLocalUser("mysql-backup", dbUsers.MySQLBackupPassword, []string{
				"GRANT RELOAD, PROCESS, LOCK TABLES, REPLICATION CLIENT ON *.* TO `mysql-backup`@`localhost`",
				"GRANT BACKUP_ADMIN ON *.* TO `mysql-backup`@`localhost`",
				"GRANT SELECT ON `performance_schema`.`keyring_component_status` TO `mysql-backup`@`localhost`",
				"GRANT SELECT ON `performance_schema`.`log_status` TO `mysql-backup`@`localhost`",
			})
		})
	})

	When("the mysql job is configured with user properties AND mysql_version=5.7", func() {
		BeforeEach(func() {
			mysqlVersionTag = "5.7"
			dbUsers = MySQLJobSpec{
				MySQLVersion:        "5.7",
				MySQLBackupPassword: uuid.NewString(),
				SeededDatabases: []SeededDatabaseEntry{
					{
						Schema:   "cloud_controller",
						Username: "ccdb",
						Password: uuid.New().String(),
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
						Password: uuid.New().String(),
						Schema:   "app_user_db1",
						Host:     "any",
					},
					"app-user2": {
						Role:     "schema-admin",
						Password: uuid.New().String(),
						Schema:   "app_user_db2",
						Host:     "any",
					},
					"healthcheck-user": {
						Role:     "minimal",
						Password: uuid.New().String(),
						Host:     "any",
					},
					"admin-user": {
						Role:     "admin",
						Password: uuid.New().String(),
						Host:     "localhost",
					},
				},
			}
		})

		It("initializes the users successfully", func() {
			verifyUser("ccdb", dbUsers.SeededDatabases[0].Password, dbUsers.SeededDatabases[0].Schema, []string{
				"GRANT USAGE ON *.* TO 'ccdb'@'%'",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `cloud\\_controller`.* TO 'ccdb'@'%'",
			})
			verifyUser("app-user1", dbUsers.SeededUsers["app-user1"].Password, dbUsers.SeededUsers["app-user1"].Schema, []string{
				"GRANT USAGE ON *.* TO 'app-user1'@'%'",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db1`.* TO 'app-user1'@'%'",
			})
			verifyUser("app-user2", dbUsers.SeededUsers["app-user2"].Password, dbUsers.SeededUsers["app-user2"].Schema, []string{
				"GRANT USAGE ON *.* TO 'app-user2'@'%'",
				"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, CREATE TEMPORARY TABLES, EXECUTE, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, EVENT, TRIGGER ON `app\\_user\\_db2`.* TO 'app-user2'@'%'",
			})
			verifyUser("healthcheck-user", dbUsers.SeededUsers["healthcheck-user"].Password, "", []string{
				"GRANT USAGE ON *.* TO 'healthcheck-user'@'%'",
			})

			verifyLocalAdminUser("admin-user", dbUsers.SeededUsers["admin-user"].Password)

			verifyLocalUser("mysql-backup", dbUsers.MySQLBackupPassword)
			verifyGrantsForLocalUser("mysql-backup", dbUsers.MySQLBackupPassword, []string{
				"GRANT RELOAD, PROCESS, LOCK TABLES, REPLICATION CLIENT ON *.* TO 'mysql-backup'@'localhost'",
			})
		})
	})
})
