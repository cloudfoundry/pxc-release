// Package client provides the ReplClient type for managing MySQL replication.
// It handles connection caching, replica user creation, backup management,
// and full replication setup between a source and target Percona XtraDB Cluster.
package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
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

// ReplState holds the parsed result of SHOW REPLICA STATUS.
// Enabled indicates whether the replica returned any row set.
// IORunning, SQLRunning, and SecondsBehind reflect the live replication status.
// LastIOErrorTime and LastSQLErrorTime are nil when no recent errors exist.
// Misc contains any unreferenced columns from the status row.
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

// String returns a human-readable summary of the replication state.
// When replication is disabled it returns "disabled".
// Recent IO and SQL errors (within the last 5 minutes) are included.
func (r ReplState) String() string {
	if !r.Enabled {
		return "disabled"
	}
	line := fmt.Sprintf(
		"IORunning: %s, SQLRunning: %s, SQLDelay: %v, SecondsBehind: %v",
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

// ReplClient manages MySQL replication between a source and a target cluster.
// It caches database connections keyed by config.Target.Name and provides
// helpers for replication setup, health checks, backup management, and cleanup.
// Unexported fields mu and dbCache are used for concurrent-safe connection caching.
type ReplClient struct {
	Source              config.Target `yaml:"source"`
	Target              config.Target `yaml:"target"`
	DataDir             string        `yaml:"datadir"`
	DumpDir             string        `yaml:"dumpdir"`
	BinPath             string        `yaml:"bindir"`
	Version             string        `yaml:"version"`
	CleanExpiredBackups bool          `yaml:"clean_expired_backups"`
	mu                  sync.Mutex
	dbCache             map[string]*sql.DB
}

func (r *ReplClient) getCachedDB(name, connectionString string, certs config.Certs) (*sql.DB, error) {
	r.mu.Lock()
	if r.dbCache == nil {
		r.dbCache = make(map[string]*sql.DB)
	}
	db, ok := r.dbCache[name]
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := db.PingContext(ctx)
		if err == nil {
			r.mu.Unlock()
			return db, nil
		}
		log.Printf("cached connection `%s` is stale, reconnecting", name)
		utils.CloseAndLogError(db)
		delete(r.dbCache, name)
	}
	r.mu.Unlock()

	db, err := r.connect(name, connectionString, certs)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.dbCache[name] = db
	r.mu.Unlock()
	return db, nil
}

// CleanBackups removes backup files that are expired or not restorable.
// It only operates when CleanExpiredBackups is enabled. Restorable backups
// are determined by checking GTID_SUBSET against the source's gtid_purged.
func (r *ReplClient) CleanBackups() {
	if !r.CleanExpiredBackups {
		return
	}
	paths, err := r.ListBackups()
	if err != nil {
		log.Printf("failed cleaning backups: %s", err.Error())
	}
	db, err := r.ConnectSource()
	if err != nil {
		log.Printf("failed cleaning backups, cannot reach source: %s", err.Error())
		return
	}
	foundRestorable := false
	for _, path := range paths {
		if foundRestorable {
			log.Printf("cleaning `%s`, found newer restorable backup", path)
			err := os.Remove(path)
			if err != nil {
				log.Printf("failed cleaning: `%s`", err.Error())
			}
			continue
		}
		GTIDPurged, ok := r.GetGTIDFromBackupFile(path)
		if !ok {
			log.Printf("failed getting gtid from `%s`, skipping clean", path)
			continue
		}
		restorable, err := r.CheckGTIDRestorable(db, GTIDPurged)
		if err != nil {
			log.Printf("failed checking gtid from `%s`, skipping clean", path)
			continue
		}
		if !restorable {
			err = os.Remove(path)
			if err != nil {
				log.Printf("failed cleaning backup `%s`", path)
			}
			continue
		}
		foundRestorable = true
	}
}

// CheckVersion verifies that the source and target MySQL versions
// share the same major.minor release. It queries VERSION() on both
// connections and compares the first two segments of the version string.
func (r *ReplClient) CheckVersion(source, target *sql.DB) error {
	var sourceVersion, targetVersion string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err := source.QueryContext(ctx, "SELECT VERSION();")
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

	log.Printf("source response: %s", sourceVersion)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err = target.QueryContext(ctx, "SELECT VERSION();")
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

	log.Printf("target response: %s", targetVersion)

	elems := strings.Split(sourceVersion, ".")
	sourceMajMin := fmt.Sprintf("%s.%s", elems[0], elems[1])
	log.Printf("source version is: %s", sourceMajMin)

	elems = strings.Split(targetVersion, ".")
	targetMajMin := fmt.Sprintf("%s.%s", elems[0], elems[1])
	log.Printf("target version is: %s", targetMajMin)

	if sourceMajMin != targetMajMin {
		return fmt.Errorf("sourceVersion: %s does not match targetVersion: %s", sourceMajMin, targetMajMin)
	}

	return nil
}

// ListBackups returns the full paths of regular files in DumpDir.
// It does not recurse into subdirectories and skips non-regular entries.
func (r *ReplClient) ListBackups() ([]string, error) {
	entries, err := os.ReadDir(r.DumpDir)
	if err != nil {
		return nil, fmt.Errorf("failed listing dumpdir `%s`: %w", r.DumpDir, err)
	}
	result := []string{}
	for _, dirEntry := range entries {
		if !dirEntry.Type().IsRegular() {
			continue
		}
		fileName := fmt.Sprintf("%s/%s", r.DumpDir, dirEntry.Name())
		result = append(result, fileName)
	}
	return result, nil
}

// GetGTIDFromBackupFile reads the GTID_PURGED value from a mysqldump file.
// It scans the file line by line looking for a SET @@GLOBAL.GTID_PURGED statement.
// Returns the GTID string and true on success, or empty string and false otherwise.
func (r *ReplClient) GetGTIDFromBackupFile(fileName string) (string, bool) {
	dump, err := os.Open(fileName)
	if err != nil {
		log.Printf("failed open to open `%s` for GTID search: %s", fileName, err.Error())
		return "", false
	}
	defer utils.CloseAndLogError(dump)
	fileReader := bufio.NewReader(dump)
	for {
		line, err := fileReader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("failed reading line of file `%s`: %s", fileName, err.Error())
			continue
		}
		GTIDPurged, found := utils.ParseGTIDFromLine(line)
		if found {
			return GTIDPurged, true
		}
	}
	return "", false
}

// CheckGTIDRestorable determines whether a given GTID is a subset of the
// source's gtid_purged using GTID_SUBSET. Returns true if the backup GTID
// is restorable, false otherwise. Errors are returned on query failures.
func (r *ReplClient) CheckGTIDRestorable(db *sql.DB, gtid string) (bool, error) {
	query := `SELECT GTID_SUBSET(@@global.gtid_purged, ?) AS is_backup_usable;`
	args := []any{gtid}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to query source to check if backup `%s` is elligible: %w", gtid, err)
	}
	for result.Next() {
		success := sql.NullBool{}
		err := result.Scan(&success)
		if err != nil {
			return false, fmt.Errorf("failed scanning check result for `%s`: `%w`", gtid, err)
		}
		log.Printf("backup GTID `%s` restoreable: %v", gtid, success.Bool && success.Valid)
		if success.Bool && success.Valid {
			return true, nil
		}
	}
	return false, nil
}

// FindElligibleBackup returns the path of the first backup file in DumpDir
// whose GTID is restorable on the source. It iterates backups in directory
// order and returns the first match. Returns empty string if none found.
func (r *ReplClient) FindElligibleBackup() (string, error) {
	backupsPaths, err := r.ListBackups()
	if err != nil {
		return "", fmt.Errorf("failed discovering backups: %w", err)
	}
	db, err := r.ConnectSource()
	if err != nil {
		return "", fmt.Errorf("failed creating connection to check backup usability: %w", err)
	}
	for _, fileName := range backupsPaths {
		GTIDPurged, found := r.GetGTIDFromBackupFile(fileName)
		if !found {
			continue
		}
		restorable, err := r.CheckGTIDRestorable(db, GTIDPurged)
		if err != nil {
			log.Printf("failed to query source to check if backup `%s` is elligible", fileName)
			continue
		}
		if restorable {
			return fileName, nil
		}
	}
	log.Println("no matching backup found")
	return "", nil
}

// InitFiles writes TLS certificate files and MySQL defaults files for both
// source and target into DataDir. The source defaults file uses non-admin
// credentials (for mysqldump), while the target defaults file uses admin
// credentials (for mysql restore).
func (r *ReplClient) InitFiles() error {
	if err := utils.WriteCertFiles(r.Source, r.DataDir); err != nil {
		return fmt.Errorf("failed writing source certs: %w", err)
	}

	if err := utils.WriteCertFiles(r.Target, r.DataDir); err != nil {
		return fmt.Errorf("failed writing target certs: %w", err)
	}
	// the source defaults file is used by mysqldump, it should use the non admin creds
	if _, err := utils.WriteMysqlCnf(r.Source, r.DataDir, false); err != nil {
		return fmt.Errorf("failed writing source defaults-file: %w", err)
	}

	// the target defaults file is used by mysql cli for the restore, it should use the admin creds
	if _, err := utils.WriteMysqlCnf(r.Target, r.DataDir, true); err != nil {
		return fmt.Errorf("failed writing source defaults-file: %w", err)
	}

	return nil
}

// Setup performs the full replication setup sequence:
//  1. Write certs and defaults files (InitFiles)
//  2. Create or update the replica user if admin credentials exist
//  3. Connect to source and target, check version compatibility
//  4. If replication is not enabled, sync source to target
//  5. Configure replication on the target (STOP/CHANGE/START REPLICA)
func (r *ReplClient) Setup() error {
	log.Println("setting up replica", "target:", r.Target.Name, "source:", r.Source.Name)

	if err := r.InitFiles(); err != nil {
		log.Printf("failed writing config and certificate files to %s", r.DataDir)
		return fmt.Errorf("failed to init files: %w", err)
	}

	if r.Source.Creds.AdminPassword != "" && r.Source.Creds.AdminUsername != "" && r.Source.Creds.Username != "" && r.Source.Creds.Password != "" {
		log.Println("found user & admin creds, will ensure user exists & has permissions")
		if err := r.CreateReplicaUserWithAdminConnection(); err != nil {
			return fmt.Errorf("failed ensuring backup user: %w", err)
		}
	}
	sourceCon, err := r.ConnectSource()
	if err != nil {
		return fmt.Errorf("setup failed: couldn't connect to `%s`: %w", r.Source.Name, err)
	}

	targetCon, err := r.ConnectTarget()
	if err != nil {
		return fmt.Errorf("setup failed: couldn't connect to `%s`: %w", r.Target.Name, err)
	}

	if err = r.CheckVersion(sourceCon, targetCon); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

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

// CheckReplication queries SHOW REPLICA STATUS and returns a ReplState
// summarizing the current replication health. When no row set is returned,
// Enabled is false and all other fields are zero-valued.
func (r *ReplClient) CheckReplication(db *sql.DB) (ReplState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := db.QueryContext(ctx, "SHOW REPLICA STATUS")
	if err != nil {
		log.Println("failed querying replica")
		return ReplState{}, err
	}
	defer utils.CloseAndLogError(result)

	state := ReplState{
		Misc: make(map[string]string),
	}

	if result.Next() {
		log.Println("replication check returned resultset")
		state.Enabled = true
		columns, err := result.Columns()
		if err != nil {
			return ReplState{}, err
		}
		data := make([]sql.RawBytes, len(columns))
		dataPointers := make([]any, len(columns))
		for i := range dataPointers {
			dataPointers[i] = &data[i]
		}
		err = result.Scan(dataPointers...)
		if err != nil {
			return ReplState{}, err
		}

		for k, rawVal := range data {
			if len(rawVal) == 0 {
				continue
			}
			v := string(append([]byte{}, rawVal...))
			switch columns[k] {
			case COLUMN_IO_RUNNING:
				state.IORunning = v
			case COLUMN_SQL_RUNNING:
				state.SQLRunning = v
			case COLUMN_SQL_RUNNING_STATE:
				state.SQLRunningState = v
			case COLUMN_SQL_DELAY:
				state.SQLDelay, err = strconv.Atoi(v)
			case COLUMN_SECONDS_BEHIND:
				state.SecondsBehind, err = strconv.Atoi(v)
			case COLUMN_LAST_IO_ERR:
				state.LastIOErr = v
			case COLUMN_LAST_IO_ERR_TIME:
				state.LastIOErrorTime = &time.Time{}
				*state.LastIOErrorTime, err = time.Parse(DATE_LAYOUT, v)
			case COLUMN_LAST_SQL_ERR_TIME:
				state.LastSQLErrorTime = &time.Time{}
				*state.LastSQLErrorTime, err = time.Parse(DATE_LAYOUT, v)
			case COLUMN_LAST_SQL_ERR:
				state.LastSQLErr = v
			default:
				state.Misc[columns[k]] = v
				continue
			}
			if err != nil {
				log.Printf("failed converting value for %s from %s", columns[k], v)
			}
		}
	}
	log.Println(state.Misc)
	return state, nil
}

// SyncSourceToTarget creates a logical dump of the source using mysqldump
// and restores it to the target using mysql. If a restorable backup already
// exists in DumpDir it is reused instead of taking a new dump.
func (r *ReplClient) SyncSourceToTarget() error {
	dumpClient, err := dumper.New(r.Source, r.DumpDir, r.DataDir, r.BinPath)
	if err != nil {
		return fmt.Errorf("failed creating dumpClient for sync: %w", err)
	}
	backupFullPath, err := r.FindElligibleBackup()
	if err != nil {
		log.Printf("failed checking elligibility of existing backups: %s", err.Error())
	}
	if backupFullPath == "" {
		backupFullPath, err = dumpClient.Dump()
		if err != nil {
			return fmt.Errorf("failed backing up source: %w", err)
		}
	}

	err = dumpClient.Restore(backupFullPath, r.Target)
	if err != nil {
		return fmt.Errorf("failed restoring to target: %w", err)
	}

	return nil
}

// Configure sets up replication on the target by executing
// STOP REPLICA, CHANGE REPLICATION SOURCE TO, and START REPLICA.
// TLS CA certificate path is included in the CHANGE command when
// Source.Certs.CA is set.
func (r *ReplClient) Configure(db *sql.DB) error {
	log.Println("stopping replication")
	_, err := db.Exec(`STOP REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed stopping replication: %w", err)
	}
	log.Println("updating replication")
	query := `CHANGE REPLICATION SOURCE TO
	SOURCE_HOST=?,
	SOURCE_PORT=?,
	SOURCE_USER=?,
	SOURCE_PASSWORD=?`
	args := []any{r.Source.Host, r.Source.Port, r.Source.Creds.Username, r.Source.Creds.Password}

	if r.Source.Certs.CA != "" {
		log.Println("found certs for encryption")
		if len(r.Source.Certs.CA) > 0 {
			caFileName := fmt.Sprintf("%s/%s.ca.pem", r.DataDir, r.Source.Name)
			args = append(args, caFileName)
			log.Println("found TLS DATA, will encrypt the replication connection")
			query = fmt.Sprintf(`%s,
		SOURCE_SSL_CA=?,
		SOURCE_SSL=1;`,
				query)
		}
	}

	_, err = db.Exec(query, args...)
	if err != nil {
		log.Printf("query failed: %s", query)
		return fmt.Errorf("failed configuring the source data on the replica: %w", err)
	}

	log.Println("starting replication")
	_, err = db.Exec(`START REPLICA;`)
	if err != nil {
		return fmt.Errorf("failed starting replication: %w", err)
	}

	log.Println("finished configuration of replica")

	return nil
}

// CreateReplicaUserWithAdminConnection opens an admin connection to the source, creates or updates
// the replica user with the correct permissions, then closes the connection.
// The connection is NOT cached; the caller does not need to close it.
func (r *ReplClient) CreateReplicaUserWithAdminConnection() error {
	if r.Source.Creds.AdminUsername == "" || r.Source.Creds.AdminPassword == "" {
		log.Println("no admin creds found, skipping replica user creation")
		return nil
	}
	if r.Source.Creds.Username == "" || r.Source.Creds.Password == "" {
		return errors.New("admin credentials provided but backup user name and password are missing.\nwill not continue")
	}

	log.Println("found admin creds. Will attempt to generate replica user")
	db, err := r.connect(r.Source.Name, r.Source.AdminDSN(), r.Source.Certs)
	if err != nil {
		return fmt.Errorf("failed connecting with admin user: %w", err)
	}
	defer utils.CloseAndLogError(db)

	return r.createReplicaUser(db)
}

// ConnectSource returns a cached connection to the source using non-admin credentials.
// The connection is cached and reused; use ConnectSourceDBUncached for a
// temporary connection to a specific database.
func (r *ReplClient) ConnectSource() (*sql.DB, error) {
	return r.getCachedDB(r.Source.Name, r.Source.DSN(), r.Source.Certs)
}

// ConnectSourceDBUncached opens a temporary connection to a specific database on the source
// using non-admin credentials. The connection is NOT cached; the caller MUST close it.
func (r *ReplClient) ConnectSourceDBUncached(dbname string) (*sql.DB, error) {
	return r.connect(r.Source.Name, r.Source.DSN(), r.Source.Certs, dbname)
}

// ConnectTarget returns a cached connection to the target using admin credentials.
// The connection is cached and reused; use ConnectTargetDBUncached for a
// temporary connection to a specific database.
func (r *ReplClient) ConnectTarget() (*sql.DB, error) {
	return r.getCachedDB(r.Target.Name, r.Target.AdminDSN(), r.Target.Certs)
}

// ConnectTargetDBUncached opens a temporary connection to a specific database on the target
// using admin credentials. The connection is NOT cached; the caller MUST close it.
func (r *ReplClient) ConnectTargetDBUncached(dbname string) (*sql.DB, error) {
	return r.connect(r.Target.Name, r.Target.AdminDSN(), r.Target.Certs, dbname)
}

// Close closes all cached database connections and clears the cache.
func (r *ReplClient) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dbCache == nil {
		return
	}
	for name, db := range r.dbCache {
		log.Printf("closing cached connection: %s", name)
		err := db.Close()
		if err != nil {
			log.Printf("failed closing connection for host `%s`: `%s`", name, err)
		}
	}
	r.dbCache = nil
}

func (r *ReplClient) createReplicaUser(db *sql.DB) error {
	args := []any{
		r.Source.Creds.Username, "%", r.Source.Creds.Password,
	}
	log.Println("creating replica user")
	query := `CREATE USER IF NOT EXISTS ?@? IDENTIFIED BY ?;`
	_, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed ensuring user existence: %w", err)
	}

	log.Println("updating replica user")
	query = `ALTER USER ?@? IDENTIFIED BY ?;`
	_, err = db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed ensuring user password: %w", err)
	}

	log.Println("setting replica user permissions")

	query = `GRANT SELECT, EVENT, RELOAD, LOCK TABLES, PROCESS, /*!80001 BACKUP_ADMIN,*/ REPLICATION CLIENT, REPLICATION SLAVE ON *.* TO ?@?;`
	_, err = db.Exec(query, []any{r.Source.Creds.Username, "%"}...)
	if err != nil {
		return fmt.Errorf("failed ensuring user permissions: %w", err)
	}

	return nil
}

func registerTLSConfig(name string, certs config.Certs) error {
	rootCertPool := x509.NewCertPool()
	if ok := rootCertPool.AppendCertsFromPEM([]byte(certs.CA)); !ok {
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

func (r *ReplClient) connect(name, connectionString string, certs config.Certs, dbname ...string) (*sql.DB, error) {
	databaseName := ""
	if len(dbname) > 0 {
		databaseName = dbname[0]
	}
	connectionString = fmt.Sprintf("%s%s?interpolateParams=true", connectionString, databaseName)
	if certs.CA != "" {
		if err := registerTLSConfig(name, certs); err != nil {
			return nil, fmt.Errorf("failed creating tls config for connection: %w", err)
		}
		connectionString = fmt.Sprintf("%s&tls=%s", connectionString, name)
	}

	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed pinging target: %s after connecting: %w", name, err)
	}
	// TODO figure out if we should set any connection defaults.
	// db.SetConnMaxLifetime(time.Second * 15)
	// db.SetConnMaxIdleTime(time.Second * 5)
	log.Printf("successfully connected to: %s", name)
	return db, nil
}
