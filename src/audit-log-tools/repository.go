package auditlogtools

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "embed"
)

var (
	//go:embed assets/audit_log_filter.sql
	auditLogFilterTableSQL string
	//go:embed assets/audit_log_user.sql
	auditLogUserTableSQL string
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{
		db: db,
	}
}

func (r Repository) MySQLVersion(version any) error {
	if err := r.db.QueryRow(`SELECT @@global.version`).Scan(version); err != nil {
		return fmt.Errorf("unable to query mysql version: %s", err)
	}

	return nil
}

func (r Repository) Install() error {
	if err := r.createTableIfNotExists("mysql", "audit_log_filter", auditLogFilterTableSQL); err != nil {
		return err
	}

	if err := r.createTableIfNotExists("mysql", "audit_log_user", auditLogUserTableSQL); err != nil {
		return err
	}

	if err := r.installComponentIfNotExists(); err != nil {
		return err
	}

	return nil
}

func (r Repository) createTableIfNotExists(schema, name, tableSQL string) error {
	const tableExistsQuery = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?`
	var exists bool
	if err := r.db.QueryRow(tableExistsQuery, schema, name).Scan(&exists); err != nil {
		return fmt.Errorf("error checking whether %s.%s exists: %w", schema, name, err)
	}

	if exists {
		slog.Info("Audit log table already exists", "schema", schema, "name", name)
		return nil
	}

	if _, err := r.db.Exec(tableSQL); err != nil {
		return fmt.Errorf("failed to create table %s.%s: %w", schema, name, err)
	}
	slog.Info("Audit log table created", "schema", schema, "name", name)

	return nil
}

func (r Repository) installComponentIfNotExists() error {
	const componentExistsQuery = `SELECT COUNT(*) FROM mysql.component WHERE component_urn = 'file://component_audit_log_filter'`
	var exists bool
	if err := r.db.QueryRow(componentExistsQuery).Scan(&exists); err != nil {
		return fmt.Errorf("error checking whether component audit_log_filter is installed: %w", err)
	}

	if exists {
		slog.Info("Audit log component audit_log_filter already installed")
		return nil
	}

	if _, err := r.db.Exec(`INSTALL COMPONENT 'file://component_audit_log_filter'`); err != nil {
		return fmt.Errorf("failed to install mysql audit_log_filter component: %s", err)
	}

	slog.Info("Audit log filter component installed")

	return nil
}

func (r Repository) CreateFilter(filterName, filterDefinition string) error {
	const filterExistsQuery = `SELECT COUNT(*) FROM mysql.audit_log_filter WHERE name = ? AND filter = CAST(? AS JSON)`
	var exists bool
	if err := r.db.QueryRow(filterExistsQuery, filterName, filterDefinition).Scan(&exists); err != nil {
		return fmt.Errorf("failed when checking if audit log filter exists name=%s: %s", filterName, err)
	}

	if exists {
		slog.Info("Audit log filter already exists with the expected definition", "name", filterName, "definition", filterDefinition)
		return nil
	}

	if _, err := r.db.Exec(`DO audit_log_filter_remove_filter(?)`, filterName); err != nil {
		return fmt.Errorf("failed to remove exisiting audit log filter name=%s: %s", filterName, err)
	}
	slog.Info("Existing audit log filter removed", "name", filterName)

	const setFilterQuery = `SELECT audit_log_filter_set_filter(?, ?)`
	var result string
	if err := r.db.QueryRow(setFilterQuery, filterName, filterDefinition).Scan(&result); err != nil {
		return fmt.Errorf("failed to set audit log filter name=%s definition=%s: %s", filterName, filterDefinition, err)
	}
	slog.Info("Configured audit log filter", "name", filterName, "definition", filterDefinition)

	if result != "OK" {
		return fmt.Errorf("failed to set audit log filter name=%s definition=%s: %s", filterName, filterDefinition, result)
	}

	return nil
}

func (r Repository) SetUserFilter(user, filterName string) error {
	const userFilterExistsQuery = `SELECT COUNT(*) FROM mysql.audit_log_user WHERE username = SUBSTRING_INDEX(?, '@', 1) AND userhost = SUBSTRING_INDEX(?, '@', -1)`
	var exists bool
	if err := r.db.QueryRow(userFilterExistsQuery, user, user).Scan(&exists); err != nil {
		return fmt.Errorf("failed when checking if audit log filter is configured for user=%s: %s", user, err)
	}
	if exists {
		slog.Info("Audit log filter already configured for user", "user", user, "filter", filterName)
		return nil
	}

	const setFilterQuery = `SELECT audit_log_filter_set_user(?, ?)`
	var result string
	if err := r.db.QueryRow(setFilterQuery, user, filterName).Scan(&result); err != nil {
		return fmt.Errorf("failed to set mysql audit log user filter user=%s filter=%s: %s", user, filterName, err)
	}
	if result != "OK" {
		return fmt.Errorf("failed to set mysql audit log user filter user=%s filter=%s: %s", user, filterName, result)
	}

	slog.Info("Configured audit log filter for user", "user", user, "filter", filterName)

	return nil
}
