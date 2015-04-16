package healthcheck

import (
	"database/sql"
	"github.com/pivotal-golang/lager"
)

const (
	SYNCED_STATE         = "4"
	DONOR_DESYNCED_STATE = "2"
)

type Healthchecker struct {
	db     *sql.DB
	config Config
	logger lager.Logger
}

type Config struct {
	DB                    DBConfig
	Host                  string `json:",omitempty"`
	Port                  int    `json:",omitempty"`
	AvailableWhenDonor    bool   `json:",omitempty"`
	AvailableWhenReadOnly bool   `json:",omitempty"`
}

type DBConfig struct {
	Host     string `json:",omitempty"`
	User     string `json:",omitempty"`
	Port     int    `json:",omitempty"`
	Password string `json:",omitempty"`
}

func New(db *sql.DB, config Config, logger lager.Logger) *Healthchecker {
	return &Healthchecker{
		db:     db,
		config: config,
		logger: logger,
	}
}

func (h *Healthchecker) Check() (bool, string) {

	h.logger.Info("Checking state of galera...")

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
