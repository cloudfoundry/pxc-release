package sequence_number

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/pivotal-golang/lager"
)

type SequenceNumberchecker struct {
	db     *sql.DB
	config config.Config
	logger lager.Logger
}

func New(db *sql.DB, config config.Config, logger lager.Logger) *SequenceNumberchecker {
	return &SequenceNumberchecker{
		db:     db,
		config: config,
		logger: logger,
	}
}
func (h *SequenceNumberchecker) Check() (int, error) {
	h.logger.Info("Checking sequence number of mariadb node...")

	var (
		unused string
		value  string
		err    error
	)

	err = h.db.QueryRow("SHOW variables LIKE 'wsrep_start_position'").Scan(&unused, &value)

	if err == sql.ErrNoRows {
		return -1, errors.New("wsrep_start_position variable not set")
	}

	if err != nil {
		return -1, err
	}

	wsrep_status_array := strings.Split(value, ":")
	if len(wsrep_status_array) != 2 {
		err = fmt.Errorf("wsrep_start_position variable returns %s. Expected format guid:integer", value)
		return -1, err
	}
	seq_number, err := strconv.Atoi(wsrep_status_array[1])

	if err != nil {
		return -1, err
	}

	return seq_number, nil
}
