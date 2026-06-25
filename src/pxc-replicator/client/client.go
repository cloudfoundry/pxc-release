// Package client holds the client and the required sql calls
package client

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/dumper"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	"github.com/go-sql-driver/mysql"
)

const (
	COLUMN_IO_RUNNING        = "Replica_IO_Running"
	COLUMN_SQL_RUNNING       = "Replica_SQL_Running"
	COLUMN_SQL_RUNNING_STATE = "Replica_SQL_Running_State"
	COLUMN_SQL_DELAY         = "SQL_Delay"
	COLUMN_SECONDS_BEHIND    = "Seconds_Behind_Source"
	COLUMN_LAST_IO_ERR       = "Last_IO_Error"
	COLUMN_LAST_IO_ERR_TIME  = "Last_IO_Error_Timestamp"
	COLUMN_LAST_SQL_ERR      = "Last_SQL_Error"
	COLUMN_LAST_SQL_ERR_TIME = "Last_SQL_Error_Timestamp"
	DATE_LAYOUT              = "060102 15:04:05"
)

type ReplState struct {
	Enabled          bool
	IORunning        string
	SQLRunning       string
	SQLRunningState  string
	SQLDelay         int
	SecondsBehind    int
	LastSQLErr       string
	LastIOErr        string
	LastIOErrorTime  *time.Time
	LastSQLErrorTime *time.Time
	Misc             map[string]string
}

func (r ReplState) String() string {
	if !r.Enabled {
		return "disabled"
	}
	line := fmt.Sprintf(
		"IORunning: %s, SQLRunning: %s, SQLDelay: %v, SecondsBehind %v",
		r.IORunning,
		r.SQLRunning,
		r.SQLDelay,
		r.SecondsBehind,
	)

	fiveMinutesAgo := time.Now().Add(time.Minute * -5)
	if r.LastIOErrorTime != nil && r.LastIOErrorTime.After(fiveMinutesAgo) {
		line = fmt.Sprintf("%s, IOErr within last 5 minutes: %s",
			line, r.LastIOErr,
		)
	}
	if r.LastSQLErrorTime != nil && r.LastSQLErrorTime.After(fiveMinutesAgo) {
		line = fmt.Sprintf("%s, SQLErr within last 5 minutes: %s",
			line, r.LastSQLErr,
		)
	}
	return line
}

var resetStatements = map[string]string{
	"8.4": "REPLICA",
	"8.0": "SLAVE",
}

type ReplClient struct {
	Source  config.Target `yaml:"source"`
	Target  config.Target `yaml:"target"`
	DataDir string        `yaml:"datadir"`
	BinDir  string        `yaml:"bindir"`
}

func (r ReplClient) CheckVersion() error {
	source, err := r.ConnectSource()
	if err != nil {
		return fmt.Errorf("failed connecting to source: %s", err)
	}
	defer utils.CloseAndLogError(source)
	target, err := r.ConnectTarget()
	if err != nil {
		return fmt.Errorf("failed connecting to target: %s", err)
	}
	defer utils.CloseAndLogError(target)

	var sourceVersion, targetVersion string
	rows, err := source.Query("SELECT VERSION();")
	if err != nil {
		return fmt.Errorf("failed to query source for version: %s", err)
	}
	defer utils.CloseAndLogError(rows)

	if !rows.Next() {
		return fmt.Errorf("could not determine Version of source")
	}
	err = rows.Scan(&sourceVersion)
	if err != nil {
		return fmt.Errorf("failed reading source response: %s", err)
	}

	log.Default().Printf("source response: %s", sourceVersion)

	rows, err = target.Query("SELECT VERSION();")
	if err != nil {
		return fmt.Errorf("failed to query source for version: %s", err)
	}

	if !rows.Next() {
		return fmt.Errorf("could not determine Version of target")
	}
	err = rows.Scan(&targetVersion)
	if err != nil {
		return fmt.Errorf("failed reading target response: %s", err)
	}

	log.Default().Printf("target response: %s", targetVersion)

	elems := strings.Split(sourceVersion, ".")
	sourceMajMin := fmt.Sprintf("%s.%s", elems[0], elems[1])
	log.Default().Printf("source version is: %s", sourceMajMin)

	elems = strings.Split(targetVersion, ".")
	targetMajMin := fmt.Sprintf("%s.%s", elems[0], elems[1])
	log.Default().Printf("target version is: %s", targetMajMin)

	if sourceMajMin != targetMajMin {
		return fmt.Errorf("sourceVersion: %s does not match targetVersion: %s", sourceMajMin, targetMajMin)
	}

	return nil
}

func (r ReplClient) Setup() error {
	log.Default().Println("setting up replica", "target:", r.Target.Name, "source:", r.Source.Name)

	if err := r.CheckVersion(); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	sourceCon, err := r.ConnectSource()
	if err != nil {
		return fmt.Errorf("setup failed: couldn't connect to `%s`: %w", r.Source.Name, err)
	}
	defer utils.CloseAndLogError(sourceCon)

	targetCon, err := r.ConnectTarget()
	if err != nil {
		return fmt.Errorf("setup failed: couldn't connect to `%s`: %w", r.Target.Name, err)
	}
	defer utils.CloseAndLogError(targetCon)

	state, err := r.CheckReplication(targetCon)
	if err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	if !state.Enabled {
		log.Println("running initial sync as there is no current replication setup")
		if err := r.SyncSourceToTarget(); err != nil {
			return fmt.Errorf("setup failed: %w", err)
		}
	}
	return r.Configure(targetCon)
}

func (r ReplClient) CheckReplication(db *sql.DB) (ReplState, error) {
	result, err := db.Query("SHOW REPLICA STATUS")
	if err != nil {
		log.Println("failed querying replica")
		return ReplState{}, err
	}
	defer utils.CloseAndLogError(result)

	state := ReplState{
		Misc: make(map[string]string),
	}

	if result.Next() {
		log.Default().Println("replication check returned non empty resultset")
		state.Enabled = true
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
				if len(*v.(*sql.RawBytes)) == 0 {
					continue
				}
				t, err := time.Parse(DATE_LAYOUT, string(*v.(*sql.RawBytes)))
				if err != nil {
					return state, fmt.Errorf("failed parsing LastIOErrTime as date: %w", err)
				}
				state.LastIOErrorTime = &t
			case COLUMN_LAST_SQL_ERR_TIME:
				if len(*v.(*sql.RawBytes)) == 0 {
					continue
				}
				t, err := time.Parse(DATE_LAYOUT, string(*v.(*sql.RawBytes)))
				if err != nil {
					return state, fmt.Errorf("failed parsing LastSQLErrTime as date: %w", err)
				}
				state.LastSQLErrorTime = &t
			case COLUMN_LAST_SQL_ERR:
				state.LastSQLErr = string(*v.(*sql.RawBytes))
			default:

				state.Misc[columnNames[k]] = string(*v.(*sql.RawBytes))
				continue
			}
		}
	}
	return state, nil
}

func (r ReplClient) SyncSourceToTarget() error {
	dumpClient, err := dumper.New(r.Source, r.DataDir, r.BinDir)
	if err != nil {
		return fmt.Errorf("failed creating dumpClient for sync: %w", err)
	}

	backupFullPath, err := dumpClient.Dump()
	if err != nil {
		return fmt.Errorf("failed backing up source: %w", err)
	}

	err = dumpClient.Restore(backupFullPath, r.Target)
	if err != nil {
		return fmt.Errorf("failed restoring to target: %w", err)
	}

	return nil
}

func (r ReplClient) Configure(db *sql.DB) error {
	log.Default().Println("stopping replication")
	_, err := db.Exec(`STOP REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed stopping replication: %w", err)
	}
	log.Default().Println("updating replication")
	query := `CHANGE REPLICATION SOURCE TO
	SOURCE_HOST=?,
	SOURCE_PORT=?,
	SOURCE_USER=?,
	SOURCE_PASSWORD=?`
	args := []any{r.Source.Host, r.Source.Port, r.Source.Creds.Username, r.Source.Creds.Password}

	if r.Source.Certs.CA != nil {
		log.Println("found certs for encryption")
		if len(r.Source.Certs.CA) > 0 {
			caFileName := fmt.Sprintf("%s/source-server-ca.pem", r.DataDir)
			err = os.WriteFile(caFileName, r.Source.Certs.CA, 0o644)
			if err != nil {
				return fmt.Errorf("failed writing source-server-ca file: %w", err)
			}
			args = append(args, caFileName)
			log.Default().Println("found TLS DATA, will encrypt the replication connection")
			query = fmt.Sprintf(`%s,
		SOURCE_SSL_CA=?,
		SOURCE_SSL=1;`,
				query)
		}
	}

	_, err = db.Exec(query, args...)
	if err != nil {
		log.Default().Printf("query failed: %s", query)
		return fmt.Errorf("failed configuring the source data on the replica: %w", err)
	}

	log.Default().Println("starting replication")
	_, err = db.Exec(`START REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed starting replication: %w", err)
	}

	log.Default().Println("finished configuration of replica")

	return nil
}

func (r ReplClient) ConnectTarget(dbname ...string) (*sql.DB, error) {
	return r.connect(r.Target.Name, r.Target.DSN(), r.Target.Certs, dbname...)
}

func (r ReplClient) ConnectSource(dbname ...string) (*sql.DB, error) {
	return r.connect(r.Source.Name, r.Source.DSN(), r.Source.Certs, dbname...)
}

func registerTLSConfig(name string, certs config.Certs) error {
	rootCertPool := x509.NewCertPool()
	if ok := rootCertPool.AppendCertsFromPEM(certs.CA); !ok {
		return fmt.Errorf("failed to append ca cert to pool")
	}
	return mysql.RegisterTLSConfig(name, &tls.Config{
		RootCAs:      rootCertPool,
		Certificates: []tls.Certificate{},
	})

	//tlsCerts, err := tls.X509KeyPair(certs.Certificate, certs.PrivateKey)
	//if err != nil {
	//	return fmt.Errorf("failed parsing certs: %w", err)
	//}

	//	return mysql.RegisterTLSConfig(name, &tls.Config{
	//		RootCAs:      rootCertPool,
	//		Certificates: []tls.Certificate{tlsCerts},
	//	})
}

func (r ReplClient) connect(name, connectionString string, certs config.Certs, dbname ...string) (*sql.DB, error) {
	databaseName := ""
	if len(dbname) > 0 {
		databaseName = dbname[0]
	}
	connectionString = fmt.Sprintf("%s%s?interpolateParams=true", connectionString, databaseName)
	if certs.CA != nil {
		if err := registerTLSConfig(name, certs); err != nil {
			return nil, fmt.Errorf("failed creating tls config for connection: %w", err)
		}
		connectionString = fmt.Sprintf("%s&tls=%s", connectionString, name)
	}

	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed pinging target: %s after connecting: %w", connectionString, err)
	}
	// TODO figure out if we should set any connection defaults.
	// db.SetConnMaxLifetime(time.Second * 15)
	// db.SetConnMaxIdleTime(time.Second * 5)
	log.Printf("successfully connected to: %s", name)
	return db, nil
}
