package main_test

import (
	"database/sql"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"setup-audit-log-filter-component"
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

	Context("Install", func() {
		When("querying whether the audit_log_filter table exists fails", func() {
			BeforeEach(func() {
				const expectedSQL = `SELECT COUNT\(\*\) FROM information_schema\.tables WHERE table_schema = \? AND table_name = \?`
				mock.ExpectQuery(expectedSQL).
					WithArgs("mysql", "audit_log_filter").
					WillReturnError(fmt.Errorf("some database error"))
			})

			It("returns an error", func() {
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`error checking whether component audit_log_filter is installed: some database error`))
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
				repo := main.NewRepository(db)

				err := repo.Install()
				Expect(err).To(MatchError(`failed to install mysql audit_log_filter component: some db error`))
			})
		})
	})

	Context("CreateFilter", func() {
		When("querying whether an audit log filter exists fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT`).
					WithArgs("some-filter", "some-json-filter-expression").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := main.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed when checking if audit log filter exists name=some-filter: some db error`))
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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

				err := repo.CreateFilter("some-filter", "some-json-filter-expression")
				Expect(err).To(MatchError(`failed to set audit log filter name=some-filter definition=some-json-filter-expression: ERROR: Incorrect rule definition`))
			})
		})
	})

	Context("SetUserFilter", func() {
		When("checking if the user  filter exists", func() {
			BeforeEach(func() {
				mock.ExpectQuery(`SELECT COUNT.*FROM mysql[.]audit_log_user`).
					WithArgs("some-user", "some-user").
					WillReturnError(fmt.Errorf("some db error"))
			})

			It("returns an error", func() {
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

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
				repo := main.NewRepository(db)

				err := repo.SetUserFilter("some-user", "some-filter-name")
				Expect(err).To(MatchError(`failed to set mysql audit log user filter user=some-user filter=some-filter-name: ERROR: Unknown filtering rule name 'some-filter-name'`))
			})
		})
	})
})
