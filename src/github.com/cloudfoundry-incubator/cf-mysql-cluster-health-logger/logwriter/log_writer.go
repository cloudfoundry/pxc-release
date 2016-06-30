package logwriter

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

type LogWriter interface {
	Write(string) error
}

type logWriter struct {
	db      *sql.DB
	logPath string
}

func New(db *sql.DB, logPath string) LogWriter {
	return &logWriter{
		db:      db,
		logPath: logPath,
	}
}

func (lw *logWriter) Write(ts string) error {
	var columnNames []string
	var columnValues []string

	statusQuery := `SHOW STATUS WHERE Variable_name like 'wsrep%'`
	status, err := lw.db.Query(statusQuery)

	if err != nil {
		return err
	}

	defer status.Close()
	for status.Next() {
		var varName string
		var varValue string
		status.Scan(&varName, &varValue)
		columnNames = append(columnNames, varName)
		columnValues = append(columnValues, varValue)
	}

	variablesQuery := `SHOW VARIABLES WHERE Variable_name = 'sql_log_bin'`
	variables, err := lw.db.Query(variablesQuery)

	if err != nil {
		return err
	}

	defer variables.Close()
	for variables.Next() {
		var varName string
		var varValue string
		variables.Scan(&varName, &varValue)
		columnNames = append(columnNames, varName)
		columnValues = append(columnValues, varValue)
	}

	info, err := os.Stat(lw.logPath)
	writeHeaders := false
	if err != nil || info.Size() == 0 {
		writeHeaders = true
	}

	f, err := os.OpenFile(lw.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	columnNamesStr := strings.Join(columnNames, ",")
	columnValuesStr := strings.Join(columnValues, ",")

	if writeHeaders {
		f.WriteString(fmt.Sprintf("%s,%s", "timestamp", columnNamesStr))
		f.WriteString("\n")
	}
	f.WriteString(fmt.Sprintf("%s,%s", ts, columnValuesStr))
	f.WriteString("\n")
	return nil
}
