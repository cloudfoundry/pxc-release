package mysql

import (
	"bytes"
	"os/exec"

	"dedicated-mysql-restore/executable"

	_ "github.com/go-sql-driver/mysql" // nolint
)

var (
	Execer executable.Executable = executable.Executor{}
)

const deleteUsersSQL = `
DELETE FROM mysql.user WHERE User <> 'mysql.sys';
DELETE FROM mysql.db WHERE User <> 'mysql.sys';
DROP TABLE IF EXISTS cf_metadata.bindings;
`

func DeleteBindingUsers() error {
	cmd := exec.Command(
		"/var/vcap/jobs/mysql/bin/mysql_ctl",
		"start",
		"--skip-daemonize",
		"--bootstrap",
	)
	cmd.Stdin = bytes.NewBufferString(deleteUsersSQL)
	return Execer.Run(cmd)
}
