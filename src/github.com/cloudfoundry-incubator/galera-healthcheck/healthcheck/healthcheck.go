package healthcheck

import (
	"database/sql"
	"fmt"

	"github.com/pivotal-golang/lager"
)

const (
	STATE_JOINING        = 1
	STATE_DONOR_DESYNCED = 2
	STATE_JOINED         = 3
	STATE_SYNCED         = 4
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

	var unused string
	var value int
	err := h.db.QueryRow("SHOW STATUS LIKE 'wsrep_local_state'").Scan(&unused, &value)

	if err == sql.ErrNoRows {
		return false, "wsrep_local_state variable not set (possibly not a galera db)"
	}

	if err != nil {
		return false, err.Error()
	}

	switch value {
	case STATE_JOINING:
		return false, "joining"
	case STATE_DONOR_DESYNCED:
		if h.config.AvailableWhenDonor {
			return h.healthy(value)
		}
		return false, "not synced"
	case STATE_JOINED:
		return false, "joined"
	case STATE_SYNCED:
		return h.healthy(value)
	default:
		return false, fmt.Sprintf("Unrecognized state: %d", value)
	}

}

func (h Healthchecker) healthy(value int) (bool, string) {
	if !h.config.AvailableWhenReadOnly {
		readOnly, err := h.isReadOnly()
		if err != nil {
			return false, err.Error()
		}

		if readOnly {
			return false, "read-only"
		}
	}
	return true, "synced"
}

func (h Healthchecker) isReadOnly() (bool, error) {
	var unused, readOnly string
	err := h.db.QueryRow("SHOW GLOBAL VARIABLES LIKE 'read_only'").Scan(&unused, &readOnly)
	if err != nil {
		return false, err
	}

	if readOnly == "ON" {
		return true, nil
	}
	return false, nil
}
