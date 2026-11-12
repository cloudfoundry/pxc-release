package sequence_number

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/go-sql-driver/mysql"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysqld_cmd"
)

// dbQueryTimeout bounds how long Check() may hold the process mutex waiting on
// mysqld to answer.
const dbQueryTimeout = 5 * time.Second

type SequenceNumberChecker struct {
	db        *sql.DB
	config    config.Config
	logger    lager.Logger
	mysqldCmd mysqld_cmd.MysqldCmd
	m         *sync.Mutex
}

func New(db *sql.DB, mysqldCmd mysqld_cmd.MysqldCmd, config config.Config, logger lager.Logger, mutex *sync.Mutex) *SequenceNumberChecker {
	return &SequenceNumberChecker{
		db:        db,
		config:    config,
		logger:    logger,
		mysqldCmd: mysqldCmd,
		m:         mutex,
	}
}

func (s *SequenceNumberChecker) Check(req *http.Request) (string, error) {
	s.logger.Info("Checking sequence number of database node...")

	if s.config.Monit.ServiceName == "garbd" {
		return "no sequence number - running on arbitrator node", nil
	}

	// Hold the mutex shared with node_manager before checking state so that a
	// concurrent /start_mysql_* cannot start mysqld between the checks and
	// mysqld --wsrep-recover.
	s.m.Lock()
	defer s.m.Unlock()

	if s.dbMayBeRunning(req.Context()) {
		return "", errors.New("can't determine sequence number when database is running")
	}

	if s.sstInProgress() {
		return "", errors.New("SST is in progress: refusing to run mysqld --wsrep-recover")
	}

	returnedSeqNo, err := s.readSeqNoFromRecoverCmd()
	if err != nil {
		return "", err
	}

	returnedSeqNoInt, converr := strconv.Atoi(returnedSeqNo)
	if converr != nil {
		return "", converr
	}

	if returnedSeqNoInt < 0 {
		return "", errors.New(fmt.Sprintf("Invalid sequence number %s", returnedSeqNo))
	}

	return returnedSeqNo, nil
}

func (s *SequenceNumberChecker) readSeqNoFromRecoverCmd() (string, error) {
	s.logger.Info("Reading seqno from logs")
	seqno, err := s.mysqldCmd.RecoverSeqno()
	if err != nil {
		s.logger.Error("Failed to retrieve seqno from logs", err)
		return "", err
	}

	return seqno, nil
}

// sstInProgress reports whether a State Snapshot Transfer is currently active.
// Galera writes sst_in_progress to DataDir when it invokes the SST script and
// removes it only after the transfer completes. Running mysqld --wsrep-recover
// while this file is present would corrupt the in-flight transfer.
func (s *SequenceNumberChecker) sstInProgress() bool {
	_, err := os.Stat(filepath.Join(s.config.DataDir, "sst_in_progress"))
	return err == nil
}

// dbMayBeRunning reports whether mysqld may be running, in which case running
// mysqld --wsrep-recover is unsafe. It errs on the side of "running":
//   - A MySQL error (access denied, too many connections, WSREP not ready, ...)
//     means a live server answered, so the database is up.
//   - A timeout or cancellation means mysqld is not answering but may still be
//     alive and holding the datadir, so it is not safe to assume it is down.
//   - Any other error (socket missing, connection refused, ...) means nothing is
//     listening, so the database is down.
func (s *SequenceNumberChecker) dbMayBeRunning(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, dbQueryTimeout)
	defer cancel()

	err := s.db.PingContext(ctx)
	if err == nil {
		return true
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		s.logger.Info("Database responded with an error, treating as running", lager.Data{"error": err.Error()})
		return true
	}

	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.logger.Info("Database did not respond in time, treating as running", lager.Data{"error": err.Error()})
		return true
	}

	s.logger.Info("Database not reachable, continuing", lager.Data{"error": err.Error()})
	return false
}
