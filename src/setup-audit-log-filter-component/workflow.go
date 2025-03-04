package main

import (
	"fmt"
)

type AuditLogRepository interface {
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

func (w Workflow) Run(excludeUsers ...string) error {
	if err := w.r.Install(); err != nil {
		return fmt.Errorf("error installing audit log filter: %s", err)
	}

	if err := w.r.CreateFilter("log_all", `{"filter":{"log":true}}`); err != nil {
		return fmt.Errorf("error creating filter 'log_all': %s", err)
	}

	if err := w.r.CreateFilter("log_none", `{"filter":{"log":false}}`); err != nil {
		return fmt.Errorf("error creating filter 'log_none': %s", err)
	}

	if err := w.r.SetUserFilter("%", "log_all"); err != nil {
		return fmt.Errorf("error setting filter=log_all for user=%%: %s", err)
	}

	for _, username := range excludeUsers {
		if err := w.r.SetUserFilter(username, "log_none"); err != nil {
			return fmt.Errorf("error setting filter=log_none for user=%s: %s", username, err)
		}
	}

	return nil
}
