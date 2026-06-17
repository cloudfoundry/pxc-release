package dumper

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"

	"github.com/cloudfoundry/pxc-release/replicator/config"
)

type Dumper struct {
	BinPath  string
	DataPath string
	target   config.Target
}

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
	err := os.WriteFile(defaultsFile, []byte(defaultFileContents), 0o644)
	if err != nil {
		return []string{}, fmt.Errorf("failed writing defaults file: %w", err)
	}
	args := []string{
		fmt.Sprintf("--defaults-file=%s", defaultsFile), // param is positional. Needs to go first.
	}
	if d.target.Certs.CA != "" {
		fileName := fmt.Sprintf("%s/%s-server-ca.pem", d.DataPath, d.target.Name)
		args = append(args, fmt.Sprintf("--ssl-ca=%s", fileName))
	}
	if d.target.Certs.Certificate != "" && d.target.Certs.PrivateKey != "" {
		certPath := fmt.Sprintf("%s/%s-cert.pem", d.DataPath, d.target.Name)
		err := os.WriteFile(certPath, []byte(d.target.Certs.Certificate), 0o644)
		if err != nil {
			return []string{}, fmt.Errorf("failed creating certFile: %s", err)
		}
		keyPath := fmt.Sprintf("%s/%s-key.pem", d.target.Name, d.DataPath)
		err = os.WriteFile(keyPath, []byte(d.target.Certs.Certificate), 0o644)
		if err != nil {
			return []string{}, fmt.Errorf("failed creating keyFile: %s", err)
		}
		args = append(args, []string{
			fmt.Sprintf("--ssl-cert=%s", certPath),
			fmt.Sprintf("--ssl-key=%s", keyPath),
		}...)
	}
	log.Default().Printf("wrote config for: %s", d.target.Name)
	return args, nil
}

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

func (d Dumper) Restore(filename string, target config.Target) error {
	inputFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed opening file containing the backup: %w", err)
	}

	// we only need to reset this in this scope, so no pointer receiver on the method.
	d.target = target

	cmd, err := d.GetRestoreCommand()
	if err != nil {
		return fmt.Errorf("failed generating restore command: %s", err)
	}
	cmd.Stdin = inputFile
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Default().Println("mysql output: %s", string(out))
		return fmt.Errorf("failed restoring dump at %s: %w", filename, err)
	}
	return nil
}

func (d Dumper) Dump(fileName string) (string, error) {
	dumpFile, err := os.CreateTemp(d.DataPath, fileName)
	if err != nil {
		return "", fmt.Errorf("failed creating file: %w", err)
	}

	defer func() {
		deferErr := dumpFile.Close()
		if deferErr != nil {
			log.Default().Printf("failed closing dumpFile: %s\n", deferErr)
		}
	}()
	cmd, err := d.GetDumpCommand([]string{"--all-databases", "--triggers", "--routines", "--single-transaction"})
	if err != nil {
		return "", fmt.Errorf("failed generating dump of %s: %w", d.target.Name, err)
	}
	cmd.Stdout = dumpFile
	errBuffer := bytes.Buffer{}
	cmd.Stderr = &errBuffer

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed taking backup with mysqldump: Stderr: `%s`, err: %w", errBuffer.String(), err)
	}

	return dumpFile.Name(), nil
}
