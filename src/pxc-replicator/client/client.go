// Package client holds the client and the required sql calls
package client

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	_ "github.com/go-sql-driver/mysql"
)

const (
	COLUMN_IO_RUNNING        = "Replica_IO_Running"
	COLUMN_SQL_RUNNING       = "Replica_SQL_Running"
	COLUMN_SQL_RUNNING_STATE = "Replica_SQL_Running_State"
	COLUMN_SQL_DELAY         = "SQL_Delay"
	COLUMN_SECONDS_BEHIND    = "Seconds_Behind_Source"
	COLUMN_LAST_IO_ERR       = "Last_IO_Error"
	COLUMN_LAST_IO_ERR_TIME  = "Last_IO_Error_Timestamp"
	COLUMN_LAST_SQL_ERR_TIME = "Last_SQL_Error"
	COLUMN_LAST_SQL_ERR      = "Last_SQL_Error_Timestamp"
	SOURCE                   = "source"
	TARGET                   = "target"
)

type ReplState struct {
	IORunning        string
	SQLRunning       string
	SQLRunningState  string
	SQLDelay         int
	SecondsBehind    int
	LastSQLErr       string
	LastIOErr        string
	LastIOErrorTime  string
	LastSQLErrorTime string
}

var resetStatements = map[string]string{
	"8.4": "REPLICA",
	"8.0": "SLAVE",
}

type ReplClient struct {
	Source config.Target
	Target config.Target
}

func (r *ReplClient) Setup() error {
	log.Default().Println("setting up replica", "target", r.Target.Host, "source", r.Source.Host)

	log.Default().Println("pinging", "host", SOURCE)
	sourceCon, err := r.Connect(SOURCE)
	if err != nil {
		return fmt.Errorf("failed connecting to %s: %w", SOURCE, err)
	}
	defer closeAndLogError(sourceCon)

	return r.Configure(sourceCon)

	//targetCon, err := r.Connect(TARGET)
	//if err != nil {
	//	return fmt.Errorf("failed connecting to %s: %w", TARGET, err)
	//}
	//defer closeAndLogError(targetCon)
	//log.Default().Println("successfully pinged source and target")
}

func closeAndLogError(db *sql.DB) {
	err := db.Close()
	if err != nil {
		log.Default().Println(err)
	}
}

func (r *ReplClient) CheckReplicationEnabled() (string, error) {
	return "", nil
}

func (r *ReplClient) CheckReplication(db *sql.DB) (ReplState, error) {
	result, err := db.Query("SHOW REPLICA STATUS")
	if err != nil {
		return ReplState{}, err
	}
	state := ReplState{}
	for result.Next() {
		data := []any{}
		columnNames := []string{}
		columns, err := result.Columns()
		if err != nil {
			return ReplState{}, err
		}
		for _, cName := range columns {
			data = append(data, &sql.RawBytes{})
			columnNames = append(columnNames, cName)
		}
		err = result.Scan(data...)
		if err != nil {
			return ReplState{}, err
		}
		for k, v := range data {
			switch columnNames[k] {
			case COLUMN_IO_RUNNING:
				state.IORunning = string(*v.(*sql.RawBytes))
			case COLUMN_SQL_RUNNING:
				state.SQLRunning = string(*v.(*sql.RawBytes))
			case COLUMN_SQL_RUNNING_STATE:
				state.SQLRunningState = string(*v.(*sql.RawBytes))
			case COLUMN_SQL_DELAY:
				val, err := strconv.Atoi(string(*v.(*sql.RawBytes)))
				if err != nil {
					return state, fmt.Errorf("failed converting SQL Delay to int %w", err)
				}
				state.SQLDelay = val
			case COLUMN_SECONDS_BEHIND:
				val, err := strconv.Atoi(string(*v.(*sql.RawBytes)))
				if err != nil {
					return state, fmt.Errorf("failed converting Seconds Behind to int %w", err)
				}
				state.SecondsBehind = val
			case COLUMN_LAST_IO_ERR:
				state.LastIOErr = string(*v.(*sql.RawBytes))
			case COLUMN_LAST_IO_ERR_TIME:
				state.LastIOErrorTime = string(*v.(*sql.RawBytes))
			case COLUMN_LAST_SQL_ERR_TIME:
				state.LastSQLErrorTime = string(*v.(*sql.RawBytes))
			case COLUMN_LAST_SQL_ERR:
				state.LastSQLErr = string(*v.(*sql.RawBytes))
			default:
				fmt.Println(columnNames[k], ":", string(*v.(*sql.RawBytes)))
				continue
			}
		}
	}
	return state, nil
}

func (r *ReplClient) CheckSQLRunning() (bool, error) {
	return false, nil
}

func (r *ReplClient) Configure(db *sql.DB) error {
	_, err := db.Exec(`STOP REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed stopping replication: %w", err)
	}
	query := fmt.Sprintf(`CHANGE REPLICATION SOURCE TO
    SOURCE_HOST='%s',
		SOURCE_PORT=%d,
    SOURCE_USER='%s',
    SOURCE_PASSWORD='%s',
    SOURCE_AUTO_POSITION=1`,
		r.Source.Host,
		r.Source.Port,
		r.Source.Creds.Username,
		r.Source.Creds.Password,
	)

	if r.Target.TLS.CA != "" {
		query = fmt.Sprintf(`%s,
		SOURCE_SSL_CA='/var/vcap/jobs/pxc-replicator/config/source.ca.pem',
		SOURCE_SSL_VERIFY_SERVER_CERT=1;
		`, query)
	}
	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed configuring the source data on the replica: %w", err)
	}

	_, err = db.Exec(`START REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed starting replication: %w", err)
	}

	return nil
}

func (r *ReplClient) Connect(host string) (*sql.DB, error) {
	var connectionString string
	switch host {
	case SOURCE:
		connectionString = r.Source.String()
	case TARGET:
		connectionString = r.Target.String()
	}

	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed pinging source: %w", err)
	}

	return db, nil
}
