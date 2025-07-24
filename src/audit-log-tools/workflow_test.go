package auditlogtools_test

import (
	"database/sql"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"auditlogtools"
)

var _ = Describe("Audit Log Installation Workflow", Label("unit", "audit", "workflow"), func() {
	var repo *fakeRepository

	BeforeEach(func() {
		repo = &fakeRepository{
			mysqlVersion: "8.4.4",
		}
	})

	It("successfully applies the audit log setup workflow", func() {
		w := auditlogtools.NewWorkflow(repo)
		err := w.Run()
		Expect(err).NotTo(HaveOccurred())
	})

	It("applies operations in the right order", func() {
		w := auditlogtools.NewWorkflow(repo)
		err := w.Run()
		Expect(err).NotTo(HaveOccurred())

		Expect(repo.operations).To(HaveExactElements(
			"MySQLVersion",
			"Install",
			`CreateFilter(name=log_all, definition={"filter":{"log":true}})`,
			`CreateFilter(name=log_none, definition={"filter":{"log":false}})`,
			`SetUserFilter(user=%, filter=log_all)`,
		))
	})

	When("querying the running mysql version fails", func() {
		BeforeEach(func() {
			repo.mysqlVersionErr = fmt.Errorf("error querying mysql version")
		})

		It("returns an error", func() {
			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError(`error querying mysql version`))
		})
	})

	When("attempting to run the workflow on an old mysql version", func() {
		BeforeEach(func() {
			repo.mysqlVersion = "8.0.41"
		})

		It("returns an error", func() {
			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError(`audit log filter component may only be configured on MySQL v8.4 or later but MySQL server reports version="8.0.41"`))
		})
	})

	When("exclusions are specified to disable logging for specific users", func() {
		It("still succeeds", func() {
			w := auditlogtools.NewWorkflow(repo)
			err := w.Run("alice", "bob", "frank")
			Expect(err).NotTo(HaveOccurred())
		})

		It("applies the expected order of operations", func() {
			w := auditlogtools.NewWorkflow(repo)
			err := w.Run("alice", "bob", "frank")
			Expect(err).NotTo(HaveOccurred())

			Expect(repo.operations).To(HaveExactElements(
				`MySQLVersion`,
				"Install",
				`CreateFilter(name=log_all, definition={"filter":{"log":true}})`,
				`CreateFilter(name=log_none, definition={"filter":{"log":false}})`,
				`SetUserFilter(user=%, filter=log_all)`,
				`SetUserFilter(user=alice, filter=log_none)`,
				`SetUserFilter(user=bob, filter=log_none)`,
				`SetUserFilter(user=frank, filter=log_none)`,
			))
		})
	})

	When("installing audit logs fails", func() {
		It("returns an error", func() {
			repo.installErr = fmt.Errorf("some install error")

			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError("error installing audit log filter: some install error"))
		})
	})

	When("creating the log_all filter fails", func() {
		It("returns an error", func() {
			repo.createFilterErrFunc = func(name, definition string) error {
				switch name {
				case "log_all":
					return fmt.Errorf("some create filter error")
				default:
					return fmt.Errorf("unexpected name")
				}
			}

			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError("error creating filter 'log_all': some create filter error"))
		})
	})

	When("creating the log_none filter fails", func() {
		It("returns an error", func() {
			repo.createFilterErrFunc = func(name, definition string) error {
				switch name {
				case "log_none":
					return fmt.Errorf("some specific create filter error")
				default:
					return nil
				}
			}

			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError("error creating filter 'log_none': some specific create filter error"))
		})
	})

	When("applying the log_all filter to the default user fails", func() {
		It("returns an error", func() {
			repo.applyFilterErrFunc = func(username, filter string) error {
				return fmt.Errorf("some apply filter error")
			}

			w := auditlogtools.NewWorkflow(repo)
			err := w.Run()
			Expect(err).To(MatchError("error setting filter=log_all for user=%: some apply filter error"))
		})
	})

	When("excluding users from audit logging fails", func() {
		It("returns an error", func() {
			repo.applyFilterErrFunc = func(username, filter string) error {
				switch username {
				case "bob@localhost":
					return fmt.Errorf("some error setting filter for bob")
				default:
					return nil
				}
			}

			w := auditlogtools.NewWorkflow(repo)
			err := w.Run("alice@localhost", "bob@localhost")
			Expect(err).To(MatchError("error setting filter=log_none for user=bob@localhost: some error setting filter for bob"))
		})
	})
})

type fakeRepository struct {
	operations []string

	mysqlVersion        string
	mysqlVersionErr     error
	installErr          error
	createFilterErrFunc func(name, definition string) error
	applyFilterErrFunc  func(user, filter string) error
}

func (f *fakeRepository) MySQLVersion(version any) error {
	f.operations = append(f.operations, "MySQLVersion")

	if f.mysqlVersionErr != nil {
		return f.mysqlVersionErr
	}

	scanner, ok := version.(sql.Scanner)
	if !ok {
		return fmt.Errorf("expected mysql version value to implement sql.Scanner but it did not")
	}

	if err := scanner.Scan(f.mysqlVersion); err != nil {
		return fmt.Errorf("failed to parse mysql version apparently: %w", err)
	}

	return nil
}

func (f *fakeRepository) Install() error {
	f.operations = append(f.operations, "Install")
	return f.installErr
}

func (f *fakeRepository) CreateFilter(name, definition string) error {
	f.operations = append(f.operations, fmt.Sprintf("CreateFilter(name=%s, definition=%s)", name, definition))

	// default: no error unless an error func was defined
	if f.createFilterErrFunc == nil {
		return nil
	}

	return f.createFilterErrFunc(name, definition)
}

func (f *fakeRepository) SetUserFilter(username, filterName string) error {
	f.operations = append(f.operations, fmt.Sprintf("SetUserFilter(user=%s, filter=%s)", username, filterName))

	if f.applyFilterErrFunc == nil {
		return nil
	}

	return f.applyFilterErrFunc(username, filterName)
}

var _ auditlogtools.AuditLogRepository = (*fakeRepository)(nil)
