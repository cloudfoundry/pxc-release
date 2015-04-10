package healthcheck

import (
	"database/sql"
)

const (
	SYNCED_STATE         = "4"
	DONOR_DESYNCED_STATE = "2"
)

type Healthchecker struct {
	db     *sql.DB
	config HealthcheckerConfig
}

type HealthcheckerConfig struct {
	AvailableWhenDonor    bool
	AvailableWhenReadOnly bool
}

func New(db *sql.DB, config HealthcheckerConfig) *Healthchecker {
	return &Healthchecker{
		db:     db,
		config: config,
	}
}

func (h *Healthchecker) Check() (bool, string) {
	var variable_name string
	var value string
	err := h.db.QueryRow("SHOW STATUS LIKE 'wsrep_local_state'").Scan(&variable_name, &value)

    if err == sql.ErrNoRows {
        return false, "wsrep_local_state variable not set (possibly not a galera db)"
    }

    if err != nil {
        return false, err.Error()
    }

	if value == SYNCED_STATE || (value == DONOR_DESYNCED_STATE && h.config.AvailableWhenDonor) {
        if !h.config.AvailableWhenReadOnly {
            var ro_variable_name, ro_value string
            err = h.db.QueryRow("SHOW GLOBAL VARIABLES LIKE 'read_only'").Scan(&ro_variable_name, &ro_value)
            if err != nil {
                return false, err.Error()
            }

            if ro_value == "ON" {
                return false, "read-only"
            }
        }
        return true, "synced"
    }

	return false, "not synced"
}
