package auditlogtools

import (
	"fmt"

	"github.com/blang/semver/v4"
)

type AuditLogRepository interface {
	MySQLVersion(version any) error
	Install() error
	CreateFilter(name, definition string) error
	SetUserFilter(username, filterName string) error
}

type Workflow struct {
	r AuditLogRepository
}

func NewWorkflow(r AuditLogRepository) *Workflow {
	return &Workflow{r}
}

type WorkflowOptions struct {
	ExcludeUsers  []string
	DefaultFilter string
}

func (w Workflow) Run(options WorkflowOptions) error {
	var mysqlVersion semver.Version
	if err := w.r.MySQLVersion(&mysqlVersion); err != nil {
		return err
	}

	if mysqlVersion.LT(semver.MustParse("8.4.0")) {
		return fmt.Errorf("audit log filter component may only be configured on MySQL v8.4 or later but MySQL server reports version=%q", mysqlVersion)
	}

	if err := w.r.Install(); err != nil {
		return fmt.Errorf("error installing audit log filter: %s", err)
	}

	if err := w.r.CreateFilter("log_all", options.DefaultFilter); err != nil {
		return fmt.Errorf("error creating filter 'log_all': %s", err)
	}

	if err := w.r.CreateFilter("log_none", `{"filter":{"log":false}}`); err != nil {
		return fmt.Errorf("error creating filter 'log_none': %s", err)
	}

	if err := w.r.SetUserFilter("%", "log_all"); err != nil {
		return fmt.Errorf("error setting filter=log_all for user=%%: %s", err)
	}

	for _, username := range options.ExcludeUsers {
		if err := w.r.SetUserFilter(username, "log_none"); err != nil {
			return fmt.Errorf("error setting filter=log_none for user=%s: %s", username, err)
		}
	}

	return nil
}
