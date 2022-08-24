package healthcheck

import (
	"database/sql"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
)

type DBStateSnapshotter struct {
	DB     *sql.DB
	Logger lager.Logger
}

func (s *DBStateSnapshotter) State() (state domain.DBState, err error) {
	err = s.DB.QueryRow(`SELECT (SELECT VARIABLE_VALUE
        FROM performance_schema.global_status
        WHERE VARIABLE_NAME = 'wsrep_local_index') AS wsrep_local_index,
       (SELECT VARIABLE_VALUE
        FROM performance_schema.global_status
        WHERE VARIABLE_NAME = 'wsrep_local_state') AS wsrep_local_state,
       @@global.read_only                          AS read_only,
       @@global.pxc_maint_mode != 'DISABLED'       AS maintenance_enabled
`).Scan(
		&state.WsrepLocalIndex,
		&state.WsrepLocalState,
		&state.ReadOnly,
		&state.MaintenanceEnabled,
	)

	s.Logger.Info("dbState", lager.Data{
		"state": state,
	})
	return state, err
}
