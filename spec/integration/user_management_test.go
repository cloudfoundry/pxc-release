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

type UserSpec struct {
	SeededUsers     map[string]UserRole   `json:"seeded_users"`
	SeededDatabases []SeededDatabaseEntry `json:"seeded_databases"`
}

var _ = Describe("UserManagement", Ordered, func() {
	var (
		dbUsers  UserSpec
		resource *dockertest.Resource
	)

	BeforeEach(func() {
		dbUsers = UserSpec{}
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

		resource, err = startMySQL(
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

	verifyLocalAdminUser := func(username, password string) {
		db, err := sql.Open("mysql", fmt.Sprintf("root@(localhost:%s)/mysql?interpolateParams=true",
			resource.GetPort("3306/tcp")))
		Expect(err).NotTo(HaveOccurred())

		expectedPrivileges, err := queryAvailablePrivileges(db)
		Expect(err).NotTo(HaveOccurred())

		grantedPrivileges, err := queryGrantedPrivileges(db, username)
		Expect(err).NotTo(HaveOccurred())

		Expect(grantedPrivileges).To(ConsistOf(expectedPrivileges))

		// Validate the admin user can actually login. I.e. we set credentials correctly
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

	When("a seeded_users property is provided", func() {
		BeforeEach(func() {
			dbUsers = UserSpec{
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
				"GRANT ALL PRIVILEGES ON `cloud\\_controller`.* TO `ccdb`@`%`",
			})
			verifyUser("app-user1", dbUsers.SeededUsers["app-user1"].Password, dbUsers.SeededUsers["app-user1"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user1`@`%`",
				"GRANT ALL PRIVILEGES ON `app\\_user\\_db1`.* TO `app-user1`@`%`",
			})
			verifyUser("app-user2", dbUsers.SeededUsers["app-user2"].Password, dbUsers.SeededUsers["app-user2"].Schema, []string{
				"GRANT USAGE ON *.* TO `app-user2`@`%`",
				"GRANT ALL PRIVILEGES ON `app\\_user\\_db2`.* TO `app-user2`@`%`",
			})
			verifyUser("healthcheck-user", dbUsers.SeededUsers["healthcheck-user"].Password, "", []string{
				"GRANT USAGE ON *.* TO `healthcheck-user`@`%`",
			})

			verifyLocalAdminUser("admin-user", dbUsers.SeededUsers["admin-user"].Password)
		})
	})
})
