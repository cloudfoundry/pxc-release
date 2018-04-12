package thermostat

import (
	"database/sql"
	"fmt"

	. "github.com/onsi/gomega"
)

func Db(config *Config) (*sql.DB, error) {
	password, err := config.Properties.FindString("admin_password")
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("admin:%s@unix(/tmp/mysql.sock)/", password)
	return sql.Open("mysql", dsn)
}

func DbPluginActive(db *sql.DB, plugin string) (bool, error) {
	var status string

	stmtOut, err := db.Prepare(
		"SELECT plugin_status FROM information_schema.plugins " +
			"WHERE plugin_name = ?")
	if err != nil {
		return false, err
	}
	defer func() { _ = stmtOut.Close() }()

	err = stmtOut.QueryRow(plugin).Scan(&status)

	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	default:
		return status == "ACTIVE", nil
	}
}

func DbVariableValue(db *sql.DB, name string) (string, error) {
	var key, value string

	stmtOut, err := db.Prepare("SHOW VARIABLES WHERE Variable_name = ?")
	if err != nil {
		return "", err
	}
	defer func() { _ = stmtOut.Close() }()

	err = stmtOut.QueryRow(name).Scan(&key, &value)

	if err != nil {
		return "", err
	}
	return value, nil
}

func DbVariableEnabled(db *sql.DB, name string) (bool, error) {
	value, err := DbVariableValue(db, name)
	if err != nil {
		return false, err
	}
	enabled := value == "ON"
	return enabled, nil
}

func DbExecuteQuery(db *sql.DB, query string, dest interface{}) error {
	return db.QueryRow(query).Scan(dest)
}

func DbSchemaExists(db *sql.DB, schemaName string) bool {
	sql := `SELECT COUNT(*) = 1 FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?`
	var result bool
	err := db.QueryRow(sql, schemaName).Scan(&result)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return result
}

func DbTableExists(db *sql.DB, schemaName, tableName string) bool {
	sql := `SELECT COUNT(*) = 1 FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`
	var result bool
	err := db.QueryRow(sql, schemaName, tableName).Scan(&result)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return result
}

func DbEventExists(db *sql.DB, schemaName, eventName string) bool {
	sql := `SELECT COUNT(*) = 1 FROM INFORMATION_SCHEMA.EVENTS WHERE EVENT_SCHEMA = ? AND EVENT_NAME = ?`
	var result bool
	err := db.QueryRow(sql, schemaName, eventName).Scan(&result)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return result
}
