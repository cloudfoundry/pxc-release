package dumper

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
)

// Package dumper provides helpers for creating logical MySQL dumps using mysqldump
// and restoring them via mysql. The API revolves around the Dumper type,
// which holds configuration for command execution.

// Dumper encapsulates the configuration used to create and restore dumps.
// BinPath holds path to mysqldump/mysql binaries; DataPath is where
// temporary configuration files and dumps are stored. The target field
// contains the Target node information used to generate command arguments.
// All fields are exported except target, which is unexported intentionally.
type Dumper struct {
	BinPath  string
	DataPath string
	target   config.Target
}

// GetRestoreCommand builds an *exec.Cmd configured to run `mysql` with the
// appropriate defaults file and any supplied flags. It returns an error if argument
// generation fails.
func (d Dumper) GetRestoreCommand(flags ...string) (*exec.Cmd, error) {
	args, err := d.args()
	args = append(args, flags...)
	log.Default().Println("args:", args)
	if err != nil {
		return nil, fmt.Errorf("failed generating mysql command: %w", err)
	}
	cmd := exec.Command("mysql",
		args...,
	)

	return cmd, nil
}

// GetDumpCommand builds an *exec.Cmd configured to run `mysqldump` with the
// appropriate defaults file and any supplied flags. It returns an error if argument
// generation fails.
func (d Dumper) GetDumpCommand(flags []string) (*exec.Cmd, error) {
	args, err := d.args()
	args = append(args, flags...)
	log.Default().Println("args:", args)
	if err != nil {
		return nil, fmt.Errorf("failed generating mysqldump command: %w", err)
	}
	cmd := exec.Command("mysqldump",
		args...,
	)

	return cmd, nil
}

// args generates the base argument slice used by both GetRestoreCommand and GetDumpCommand.
// It creates a temporary my.cnf file, writes TLS files if needed,
// and returns the flags slice. An error is returned on any I/O or os write failure.
func (d Dumper) args() ([]string, error) {
	defaultsFile := fmt.Sprintf("%s/%s.mysql.cnf", d.DataPath, d.target.Name)
	defaultFileContents := fmt.Sprintf(`[client]
  user = '%s'
  password = '%s'
  protocol = tcp
  host = '%s'
  port = '%d'`,
		d.target.Creds.Username,
		d.target.Creds.Password,
		d.target.Host,
		d.target.Port,
	)
	err := os.WriteFile(defaultsFile, []byte(defaultFileContents), 0o600)
	if err != nil {
		return []string{}, fmt.Errorf("failed writing defaults file: %w", err)
	}
	args := []string{
		fmt.Sprintf("--defaults-file=%s", defaultsFile), // param is positional. Needs to go first.
	}
	if d.target.Certs.CA != "" {
		if len(d.target.Certs.CA) > 0 {
			fileName := fmt.Sprintf("%s/%s-server-ca.pem", d.DataPath, d.target.Name)
			args = append(args, "--ssl-mode=VERIFY_CA", fmt.Sprintf("--ssl-ca=%s", fileName))
			err = os.WriteFile(fileName, []byte(d.target.Certs.CA), 0o600)
			if err != nil {
				return []string{}, fmt.Errorf("failed writing server-ca-file `%s`: %w", fileName, err)
			}
		}
		//if len(d.target.Certs.Certificate) > 0 && len(d.target.Certs.PrivateKey) > 0 {
		//	certPath := fmt.Sprintf("%s/%s-cert.pem", d.DataPath, d.target.Name)
		//	err := os.WriteFile(certPath, d.target.Certs.Certificate, 0o600)
		//	if err != nil {
		//		return []string{}, fmt.Errorf("failed creating certFile: %s", err)
		//	}
		//	keyPath := fmt.Sprintf("%s/%s-key.pem", d.target.Name, d.DataPath)
		//	err = os.WriteFile(keyPath, d.target.Certs.PrivateKey, 0o600)
		//	if err != nil {
		//		return []string{}, fmt.Errorf("failed creating keyFile: %s", err)
		//	}
		//	args = append(args, []string{
		//		fmt.Sprintf("--ssl-cert=%s", certPath),
		//		fmt.Sprintf("--ssl-key=%s", keyPath),
		//	}...)
		//}
	}
	log.Default().Printf("wrote config for: %s", d.target.Name)
	return args, nil
}

// New constructs a new Dumper. It validates that the data directory can be created
// and that mysqldump's version matches target.Version. An error is returned if any check fails.
func New(target config.Target, dataPath, binPath string) (Dumper, error) {
	d := Dumper{
		BinPath:  binPath,
		DataPath: dataPath,
		target:   target,
	}

	err := os.MkdirAll(dataPath, 0o755)
	if err != nil {
		return Dumper{}, fmt.Errorf("failed creating dataDir: %w", err)
	}

	cmd := exec.Command("mysqldump", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Dumper{}, fmt.Errorf("failed checking version of mysqldump: %w", err)
	}
	versionMatchRE := regexp.MustCompile(fmt.Sprintf("Ver %s", target.Version))
	if !versionMatchRE.Match(out) {
		return Dumper{}, fmt.Errorf("target uses version: `%s`, but mysqldump found reports: `%s`", target.Version, string(out))
	}

	return d, nil
}

// Restore feeds the file at filename into a mysql process to restore the database described by target.
// It rewrites d.target for this operation and returns an error on failure.
func (d Dumper) Restore(filename string, target config.Target) error {
	log.Default().Println("starting restore")
	inputFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed opening file containing the backup: %w", err)
	}
	defer inputFile.Close()

	// we only need to reset this in this scope, so no pointer receiver on the method.
	d.target = target

	cmd, err := d.GetRestoreCommand()
	if err != nil {
		return fmt.Errorf("failed generating restore command: %s", err)
	}
	log.Default().Printf("generated mysql args %s", cmd)
	cmd.Stdin = inputFile
	out, err := cmd.CombinedOutput()
	log.Default().Printf("importing dump %s", filename)
	if err != nil {
		log.Default().Printf("mysql output: %s", string(out))
		return fmt.Errorf("failed restoring dump at %s: %w", filename, err)
	}
	if len(out) > 0 {
		log.Default().Printf("import output: `%s`", string(out))
	}
	return nil
}

// Dump creates a full logical backup of d.target using mysqldump, stores it in DataPath with
// a timestamped filename, and returns the file path. Any error from running mysqldump
// is wrapped with details.
func (d Dumper) Dump() (string, error) {
	prefix := time.Now().UTC().Format(time.RFC3339)
	dumpFile, err := os.CreateTemp(d.DataPath, prefix)
	if err != nil {
		return "", fmt.Errorf("failed creating file: %w", err)
	}
	log.Default().Printf("will save dump at %s", dumpFile.Name())
	defer utils.CloseAndLogError(dumpFile)
	cmd, err := d.GetDumpCommand([]string{"--all-databases", "--triggers", "--routines", "--single-transaction"})
	if err != nil {
		return "", fmt.Errorf("failed generating dump of %s: %w", d.target.Name, err)
	}
	cmd.Stdout = dumpFile
	var errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed taking backup with mysqldump: Stderr: `%s`, err: %w", errBuffer.String(), err)
	}

	log.Default().Printf("finished backup: %s", dumpFile.Name())
	return dumpFile.Name(), nil
}
