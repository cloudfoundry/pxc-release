package sequence_number

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
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

func (s *SequenceNumberchecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result, err := s.Check()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg := fmt.Sprintf("Failed to determine sequence number: %s", err.Error())
		s.logger.Error(errMsg, err)
		w.Write([]byte(errMsg))
		return
	}

	resultStr := []byte(strconv.Itoa(result))
	s.logger.Debug(fmt.Sprintf("Response body: %s", resultStr))
	w.Write(resultStr)
}

func (s *SequenceNumberchecker) Check() (int, error) {
	s.logger.Info("Checking sequence number of mariadb node...")

	var (
		unused string
		value  string
		err    error
	)

	err = s.db.QueryRow("SHOW variables LIKE 'wsrep_start_position'").Scan(&unused, &value)

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
