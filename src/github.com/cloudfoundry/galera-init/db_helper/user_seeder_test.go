package db_helper_test

import (
	"database/sql"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/galera-init/db_helper"
)

var _ = Describe("User Seeder", func() {
	var (
		userSeeder db_helper.UserSeeder
		testLogger lagertest.TestLogger
		fakeDB     *sql.DB
		mock       sqlmock.Sqlmock
	)

	BeforeEach(func() {
		var err error
		testLogger = *lagertest.NewTestLogger("db_helper")

		fakeDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		userSeeder = db_helper.NewUserSeeder(fakeDB, testLogger)
	})

	AfterEach(func() {
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	Describe("SeedUser", func() {
		It("creates the user", func() {
			mock.ExpectExec("CREATE USER IF NOT EXISTS `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("ALTER USER `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))

			userSeeder.SeedUser("username", "password", "loopback", "admin")
		})

		It("grants full access when the role is admin", func() {
			mock.ExpectExec("CREATE USER IF NOT EXISTS `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("ALTER USER `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("GRANT ALL PRIVILEGES ON *.* TO `username`@`127.0.0.1` WITH GRANT OPTION").
				WillReturnResult(sqlmock.NewResult(1, 1))

			userSeeder.SeedUser("username", "password", "loopback", "admin")
		})

		It("grants no access when the role is minimal", func() {
			mock.ExpectExec("CREATE USER IF NOT EXISTS `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("ALTER USER `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("REVOKE ALL PRIVILEGES ON *.* FROM `username`@`127.0.0.1`").
				WillReturnResult(sqlmock.NewResult(1, 1))

			userSeeder.SeedUser("username", "password", "loopback", "minimal")
		})

		It("errors when the role in unknown", func() {
			err := userSeeder.SeedUser("username", "password", "loopback", "foo")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("Invalid role: foo"))
		})

		It("scopes grants to 127.0.0.1 correctly", func() {
			mock.ExpectExec("CREATE USER IF NOT EXISTS `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("ALTER USER `username`@`127.0.0.1` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("REVOKE ALL PRIVILEGES ON *.* FROM `username`@`127.0.0.1`").
				WillReturnResult(sqlmock.NewResult(1, 1))

			userSeeder.SeedUser("username", "password", "loopback", "minimal")
		})

		It("scopes grants to any correctly", func() {
			mock.ExpectExec("CREATE USER IF NOT EXISTS `username`@`%` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("ALTER USER `username`@`%` IDENTIFIED BY 'password'").
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("REVOKE ALL PRIVILEGES ON *.* FROM `username`@`%`").
				WillReturnResult(sqlmock.NewResult(1, 1))

			userSeeder.SeedUser("username", "password", "any", "minimal")
		})

		It("errors when the host in unknown", func() {
			err := userSeeder.SeedUser("username", "password", "unknown", "admin")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("Invalid host: unknown"))
		})
	})
})
