package seeder_test

import (
	"database/sql"
	"fmt"

	"code.cloudfoundry.org/lager/lagertest"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	s "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Seeder", func() {
	var (
		testLogger lagertest.TestLogger
		dbConfig   config.PreseededDatabase
		fakeDB     *sql.DB
		seeder     s.Seeder
		mock       sqlmock.Sqlmock
	)

	BeforeEach(func() {
		var err error
		testLogger = *lagertest.NewTestLogger("seeder")
		fakeDB, mock, err = sqlmock.New()
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
		Expect(mock.ExpectationsWereMet()).To(Succeed())
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
			mock.ExpectExec(createDbExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.CreateDBIfNeeded()
		})

		Context("when creating the database returns an error", func() {
			It("bubbles the error up", func() {
				mock.ExpectExec(createDbExec).
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

				mock.ExpectQuery(selectUserQuery).
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

				mock.ExpectQuery(selectUserQuery).
					WithArgs().
					WillReturnRows(noExpectedRow)

				result, err := seeder.IsExistingUser()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("determining if the user exists returns an error", func() {
			It("returns the error", func() {
				mock.ExpectQuery(selectUserQuery).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				_, err := seeder.IsExistingUser()
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
			mock.ExpectExec(createUserExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.CreateUser()
		})

		Context("when creating the user returns an error", func() {
			It("bubbles the error up", func() {
				mock.ExpectExec(createUserExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.CreateUser()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("UpdateUser", func() {
		var updateUserExec string

		BeforeEach(func() {
			updateUserExec = fmt.Sprintf(
				"SET PASSWORD FOR `%s` = PASSWORD\\('%s'\\)",
				dbConfig.User,
				dbConfig.Password,
			)
		})

		It("updates the user with the new password", func() {
			mock.ExpectExec(updateUserExec).
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			seeder.UpdateUser()
		})

		Context("when updating the user returns an error", func() {
			It("bubbles the error up", func() {
				mock.ExpectExec(updateUserExec).
					WithArgs().
					WillReturnError(fmt.Errorf("some error"))

				err := seeder.UpdateUser()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("some error"))
			})
		})
	})

	Describe("GrantUserPrivileges", func() {
		var (
			grantAllExec         string
			revokePrivilegesExec string
		)

		BeforeEach(func() {
			grantAllExec = fmt.Sprintf("GRANT ALL ON `%s`.* TO '%s'@'%%", dbConfig.DBName, dbConfig.User)
			revokePrivilegesExec = fmt.Sprintf("REVOKE LOCK TABLES ON `%s`.* FROM '%s'@'%%", dbConfig.DBName, dbConfig.User)
		})

		It("grants them all privileges and then revokes LOCK TABLES", func() {
			mock.ExpectExec(grantAllExec).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(revokePrivilegesExec).WillReturnResult(sqlmock.NewResult(0, 0))

			Expect(seeder.GrantUserPrivileges()).To(Succeed())
		})

		It("returns an error if granting privileges errors", func() {
			err := errors.New("error")

			mock.ExpectExec(grantAllExec).WillReturnError(err)

			Expect(seeder.GrantUserPrivileges()).To(MatchError(err))
		})

		It("returns an error if revoking LOCK TABLES privileges errors", func() {
			err := errors.New("error")

			mock.ExpectExec(grantAllExec).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(revokePrivilegesExec).WillReturnError(err)

			Expect(seeder.GrantUserPrivileges()).To(MatchError(err))
		})
	})
})
