package auditlogtools_test

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"auditlogtools"
)

// These unit tests use a sql mock to drive out hard-to-test query failure cases
// Happy path integration testing against real database is tested at a higher level in the workflow tests
var _ = Describe("Repository Tests", Label("unit", "repository"), func() {
	var (
		db   *sql.DB
		mock sqlmock.Sqlmock
	)

	BeforeEach(func() {
		var err error
		db, mock, err = sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	Context("MySQLVersion", func() {
		It("queries the current mysql version into a variable", func() {
			repo := auditlogtools.NewRepository(db)
			{
				mock.ExpectQuery(`SELECT @@global.version`).
					WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(`8.4.4-4`))

				var value string
				err := repo.MySQLVersion(&value)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("8.4.4-4"))
			}

			// blang/sermver/v4.Version implements sql.Scanner and can be read directly via a database call
			{
				mock.ExpectQuery(`SELECT @@global.version`).
					WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(`8.4.4-4`))

				var value semver.Version
				err := repo.MySQLVersion(&value)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(BeEquivalentTo(semver.MustParse("8.4.4-4")))
			}
		})

		When("querying a mysql version fails", func() {
			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				mock.ExpectQuery(`SELECT @@global.version`).
					WillReturnError(errors.New("error"))

				var value string
				err := repo.MySQLVersion(&value)
				Expect(err).To(MatchError("unable to query mysql version: error"))
			})
		})

		When("a bad version value is provided", func() {
			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				mock.ExpectQuery(`SELECT @@global.version`).
					WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(`8.4.4-4`))

				// struct{} does not implement the sql.Scanner interface and will fail
				var value struct{}
				err := repo.MySQLVersion(&value)
				Expect(err).To(MatchError(ContainSubstring("unable to query mysql version: sql: Scan error")))
			})
		})
	})
	Context("Install", func() {
		It("installs the audit log filter component", func() {
			mock.ExpectQuery(`SELECT COUNT`).
				WithArgs("mysql", "audit_log_filter").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`0`))
			mock.ExpectExec(`CREATE TABLE IF NOT EXISTS mysql\.audit_log_filter`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery(`SELECT COUNT`).
				WithArgs("mysql", "audit_log_user").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`0`))
			mock.ExpectExec(`CREATE TABLE IF NOT EXISTS mysql\.audit_log_user`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery(`SELECT COUNT.*FROM mysql\.component`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`0`))
			mock.ExpectExec(`INSTALL COMPONENT`).
				WillReturnResult(sqlmock.NewResult(0, 0))

			repo := auditlogtools.NewRepository(db)

			err := repo.Install()
			Expect(err).NotTo(HaveOccurred())
		})

		When("querying whether the audit_log_filter table exists fails", func() {
			BeforeEach(func() {
				const expectedSQL = `SELECT COUNT\(\*\) FROM information_schema\.tables WHERE table_schema = \? AND table_name = \?`
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_filter").
					WillReturnError(fmt.Errorf("some database error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`error checking whether mysql.audit_log_filter exists: some database error`))
			})
		})

		When("creating the audit_log_filter table fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec(`CREATE TABLE`).
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`failed to create table mysql.audit_log_filter: some db error`))
			})
		})

		When("querying whether the audit_log_user table exists fails", func() {
			BeforeEach(func() {
				const expectedSQL = `SELECT COUNT\(\*\) FROM information_schema\.tables WHERE table_schema = \? AND table_name = \?`

				// querying for mysql.audit_log_filter succeeds
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

				// querying for mysql.audit_log_user fails, i.e. mysql crashed after the first query
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_user").
					WillReturnError(fmt.Errorf("some database error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`error checking whether mysql.audit_log_user exists: some database error`))
			})
		})

		When("creating the audit_log_user table fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec(`CREATE TABLE`).
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`failed to create table mysql.audit_log_user: some db error`))
			})
		})

		When("querying whether the audit_log_filter component is installed fails", func() {
			BeforeEach(func() {
				// querying for mysql.audit_log_filter succeeds
				const expectedSQL = `SELECT COUNT\(\*\) FROM information_schema\.tables WHERE table_schema = \? AND table_name = \?`

				// querying for mysql.audit_log_filter succeeds
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// querying for mysql.audit_log_filter succeeds
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

				// querying mysql.component fails (access issue or database crashed after previous query, etc.)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM mysql\.component`).
					WillReturnError(fmt.Errorf("some database error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`error checking whether component audit_log_filter is installed: some database error`))
			})
		})

		When("the audit_log_filter component is already installed", func() {
			BeforeEach(func() {
				// Boilerplate setup: audit log filter tables exist
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`1`))
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`1`))

				// Component is already installed
				mock.ExpectQuery(`SELECT COUNT.*FROM mysql\.component`).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(`1`))
			})
			It("skips the INSTALL COMPONENT step", func() {
				repo := auditlogtools.NewRepository(db)
				err := repo.Install()
				Expect(err).NotTo(HaveOccurred())

				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})
		When("installing the audit_log_filter component fails", func() {
			BeforeEach(func() {
				// Ensure expected queries succeed
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_filter").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("mysql", "audit_log_user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`SELECT .* FROM mysql.component`).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

				// Fail the expected query operation
				mock.ExpectExec(`INSTALL COMPONENT`).
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`failed to install mysql audit_log_filter component: some db error`))
			})
		})
	})

	Context("CreateFilter", func() {
		It("creates a log filter from a JSON expression", func() {
			mock.ExpectQuery(`SELECT COUNT.* FROM mysql\.audit_log_filter`).
				WithArgs("some-filter-name", "some-filter-definition").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			mock.ExpectExec(`DO audit_log_filter_remove_filter`).
				WithArgs("some-filter-name").
				WillReturnResult(sqlmock.NewResult(0, 0))

			mock.ExpectQuery(`SELECT audit_log_filter_set_filter`).
				WithArgs("some-filter-name", `some-filter-definition`).
				WillReturnRows(sqlmock.NewRows([]string{"result"}).AddRow("OK"))

			repo := auditlogtools.NewRepository(db)
			err := repo.CreateFilter("some-filter-name", `some-filter-definition`)
			Expect(err).NotTo(HaveOccurred())
		})

		When("querying whether an audit log filter exists fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed when checking if audit log filter exists name=some-filter: some db error`))
			})
		})

		When("a user filter already exists with the same definition", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.* FROM mysql\.audit_log_filter`).
					WithArgs("filter-name", "filter-definition").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			})

			It("does not attempt to recreate the filter", func() {
				repo := auditlogtools.NewRepository(db)
				err := repo.CreateFilter("filter-name", "filter-definition")
				Expect(err).NotTo(HaveOccurred())

				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

		When("removing the existing filter fails", func() {
			BeforeEach(func() {
				// Existing filter doesn't exist or does not match
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

				// Remove fails when clearing out the old filter
				mock.ExpectExec(`DO audit_log_filter_remove_filter`).
					WithArgs("some-filter").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed to remove exisiting audit log filter name=some-filter: some db error`))
			})
		})

		When("creating an audit log filter fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec(`DO audit_log_filter_remove_filter`).
					WithArgs("some-filter").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(`SELECT audit_log_filter_set_filter`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed to set audit log filter name=some-filter definition=some-json-filter-expression: some db error`))
			})
		})

		When("creating the audit log filter returns an unsuccessful response", func() {
			BeforeEach(func() {
				// Query succeeds, but does not return "OK" so should fail
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectExec(`DO audit_log_filter_remove_filter`).
					WithArgs("some-filter").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery(`SELECT audit_log_filter_set_filter`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnRows(sqlmock.NewRows([]string{"audit_log_filter_set_filter"}).AddRow("ERROR: Incorrect rule definition"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed to set audit log filter name=some-filter definition=some-json-filter-expression: ERROR: Incorrect rule definition`))
			})
		})
	})

	Context("SetUserFilter", func() {
		It("associates a mysql user pattern with a previously configured filter name", func() {
			mock.ExpectQuery(`SELECT COUNT.* FROM mysql\.audit_log_user`).
				WithArgs("some-username", "some-username").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(`SELECT audit_log_filter_set_user`).
				WithArgs("some-username", "some-filter-name").
				WillReturnRows(sqlmock.NewRows([]string{"result"}).AddRow("OK"))

			repo := auditlogtools.NewRepository(db)
			err := repo.SetUserFilter("some-username", "some-filter-name")
			Expect(err).NotTo(HaveOccurred())
		})

		When("checking if the user  filter exists", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.*FROM mysql[.]audit_log_user`).
					WithArgs("some-user", "some-user").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.SetUserFilter("some-user", "some-filter-name")
				Expect(err).To(MatchError(`failed when checking if audit log filter is configured for user=some-user: some db error`))
			})
		})

		When("setting a user filter fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.*FROM mysql[.]audit_log_user`).
					WithArgs("some-user", "some-user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectQuery(`SELECT audit_log_filter_set_user`).
					WithArgs("some-user", "some-filter-name").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.SetUserFilter("some-user", "some-filter-name")
				Expect(err).To(MatchError(`failed to set mysql audit log user filter user=some-user filter=some-filter-name: some db error`))
			})
		})

		When("setting a user filter returns an unsuccessful response", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.*FROM mysql[.]audit_log_user`).
					WithArgs("some-user", "some-user").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectQuery(`SELECT audit_log_filter_set_user`).
					WithArgs("some-user", "some-filter-name").
					WillReturnRows(sqlmock.NewRows([]string{"audit_log_filter_set_user"}).AddRow("ERROR: Unknown filtering rule name 'some-filter-name'"))
			})

			It("returns an error", func() {
				repo := auditlogtools.NewRepository(db)

				err := repo.SetUserFilter("some-user", "some-filter-name")
				Expect(err).To(MatchError(`failed to set mysql audit log user filter user=some-user filter=some-filter-name: ERROR: Unknown filtering rule name 'some-filter-name'`))
			})
		})

		When("a user filter already exists", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.* FROM mysql\.audit_log_user`).
					WithArgs("some-user@some-host", "some-user@some-host").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			})

			It("does not overwrite a second time", func() {
				repo := auditlogtools.NewRepository(db)
				err := repo.SetUserFilter("some-user@some-host", "some-filter-name")
				Expect(err).NotTo(HaveOccurred())

				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})
})
