package seeder_test

import (
	"database/sql"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	s "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Seeder", func() {
	var (
		testLogger lagertest.TestLogger
		dbConfig   config.PreseededDatabase
		fakeDB     *sql.DB
		seeder     s.Seeder
	)

	BeforeEach(func() {
		var err error
		testLogger = *lagertest.NewTestLogger("seeder")
		fakeDB, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		dbConfig = config.PreseededDatabase{
			DBName:   "DB1",
			User:     "user1",
			Password: "password1",
		}
	})

	JustBeforeEach(func() {
		seeder = s.NewSeeder(
			fakeDB,
			dbConfig,
			testLogger,
		)
	})

	AfterEach(func() {
		err := fakeDB.Close()
		Expect(err).ToNot(HaveOccurred())
	})

	const lastInsertId = -1
	const rowsAffected = 1

	Describe("CreateDBIfNeeded", func() {
		var createDbExec string

		BeforeEach(func() {
			createDbExec = fmt.Sprintf(
				"CREATE DATABASE IF NOT EXISTS `%s`",
				dbConfig.DBName)
		})

		It("creates the database", func() {
			sqlmock.ExpectExec(createDbExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.CreateDBIfNeeded()
		})

		Context("when creating the database returns an error", func() {
			It("bubbles the error up", func() {
				sqlmock.ExpectExec(createDbExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.CreateDBIfNeeded()
				Expect(err).To(HaveOccurred())
			})
		})

	})

	Describe("IsExistingUser", func() {
		var selectUserQuery string

		BeforeEach(func() {
			selectUserQuery = fmt.Sprintf(
				"SELECT User FROM mysql.user WHERE User = '%s'",
				dbConfig.User)
		})

		Context("user exists in the database", func() {
			It("returns true", func() {
				expectedRow := sqlmock.NewRows([]string{"User"}).
					AddRow(dbConfig.User)

				sqlmock.ExpectQuery(selectUserQuery).
					WithArgs().
					WillReturnRows(expectedRow)

				result, err := seeder.IsExistingUser()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("user does not exist in the database", func() {
			It("returns false", func() {
				noExpectedRow := sqlmock.NewRows([]string{"User"})

				sqlmock.ExpectQuery(selectUserQuery).
					WithArgs().
					WillReturnRows(noExpectedRow)

				result, err := seeder.IsExistingUser()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("determining if the user exists returns an error", func() {
			It("returns the error", func() {
				sqlmock.ExpectQuery(selectUserQuery).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				_, err := seeder.IsExistingUser()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CreateUserForDB", func() {
		var createUserExec string

		BeforeEach(func() {
			createUserExec = fmt.Sprintf(
				"CREATE USER `%s` IDENTIFIED BY '%s'",
				dbConfig.User,
				dbConfig.Password,
			)
		})

		It("creates the user", func() {
			sqlmock.ExpectExec(createUserExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.CreateUserForDB()
		})

		Context("when creating the user returns an error", func() {
			It("bubbles the error up", func() {
				sqlmock.ExpectExec(createUserExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.CreateUserForDB()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CreateUser", func() {
		var createUserExec string

		BeforeEach(func() {
			createUserExec = fmt.Sprintf(
				"CREATE USER `%s` IDENTIFIED BY '%s'",
				dbConfig.User,
				dbConfig.Password,
			)
		})

		It("creates the user", func() {
			sqlmock.ExpectExec(createUserExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.CreateUser("foo", "bar")
		})

		Context("when creating the user returns an error", func() {
			It("bubbles the error up", func() {
				sqlmock.ExpectExec(createUserExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.CreateUser("foo", "bar")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("GrantUserAllPrivileges", func() {
		var grantUserPrivilegesExec string

		BeforeEach(func() {
			grantUserPrivilegesExec = fmt.Sprintf(
				"GRANT ALL ON `%s`.* TO `%s`",
				dbConfig.DBName,
				dbConfig.User)
		})

		It("grants the user all privileges", func() {
			sqlmock.ExpectExec(grantUserPrivilegesExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.GrantUserAllPrivileges()
		})

		Context("when creating the database returns an error", func() {
			It("bubbles the error up", func() {
				sqlmock.ExpectExec(grantUserPrivilegesExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.GrantUserAllPrivileges()
				Expect(err).To(HaveOccurred())
			})
		})

	})

	Describe("GrantUserSuperROPrivileges", func() {
		var grantUserPrivilegesExec string

		BeforeEach(func() {
			grantUserPrivilegesExec = fmt.Sprintf(
				"GRANT SELECT ON `%s`.* TO `%s`",
				"*",
				"foo")
		})

		It("grants the user read-only privileges", func() {
			sqlmock.ExpectExec(grantUserPrivilegesExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.GrantUserSuperROPrivileges("foo")
		})

		Context("when creating the database returns an error", func() {
			It("bubbles the error up", func() {
				sqlmock.ExpectExec(grantUserPrivilegesExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.GrantUserSuperROPrivileges("foo")
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
