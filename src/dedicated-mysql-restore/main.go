package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"

	"dedicated-mysql-restore/cmdopts"
	"dedicated-mysql-restore/executable"
	"dedicated-mysql-restore/fs"
	"dedicated-mysql-restore/mysql"
	"dedicated-mysql-restore/unpack"
)

const monitPath = `/var/vcap/bosh/bin/monit`

var (
	execer       executable.Executable = executable.Executor{}
	rootUID                            = 0
	mysqlDir                           = "/var/vcap/store/mysql"
	mysqlDataDir                       = path.Join(mysqlDir, "data")
	vcapUser                           = "vcap"
	logger       Logger                = log.New(os.Stderr, "", log.LstdFlags)
)

type Logger interface {
	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Println(...interface{})
	Printf(string, ...interface{})
	SetOutput(io.Writer)
}

func main() {

	opts, err := cmdopts.ParseArgs(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if opts.MySQLUser != "" || opts.MySQLPassword != "" {
		logger.Println("NOTE: --mysql-user and --mysql-password options are deprecated and no longer required for this utility.")
	}

	if os.Geteuid() != rootUID {
		logger.Fatal("Restore utility requires root privileges. Please run as root user.")
	}

	encryptedFile, err := os.Open(opts.RestoreFile)
	if err != nil {
		logger.Fatalf("Failed to open restore file '%s': %s", opts.RestoreFile, err)
	}
	defer encryptedFile.Close()

	gpgReader := unpack.GPGReader{Passphrase: opts.EncryptionKey}
	gpgStream, err := gpgReader.Open(encryptedFile)
	if err != nil {
		logger.Fatalf("Failed to open gpg archive '%s': %s", opts.RestoreFile, err)
	}

	logger.Println("Stopping mysql job...")
	err = stopMysqlMonitProcesses()
	if err != nil {
		logger.Fatalf("Stopping mysql job failed: %s", err)
	}

	logger.Println("Removing mysql data directory...")
	err = fs.CleanDirectory(mysqlDataDir)
	if err != nil {
		logger.Fatalf("Removing mysql data directory failed: %s", err)
	}

	logger.Printf("Decrypting mysql backup %s...", opts.RestoreFile)
	if err = unpack.ExtractTar(gpgStream, mysqlDataDir, os.Stderr); err != nil {
		logger.Fatalf("Unpacking mysql backup failed: %s", err)
	}

	if err = fs.RecursiveChown(mysqlDir, vcapUser); err != nil {
		logger.Fatalf("Restoring vcap ownership to %s failed: %s", mysqlDir, err)
	}

	logger.Println("Removing stale database users...")
	err = mysql.DeleteBindingUsers()
	if err != nil {
		logger.Fatalf("Error when dropping users: %s", err)
	}

	logger.Println("Starting mysql job...")
	cmd := exec.Command(monitPath, "start", "mysql")
	err = execer.Run(cmd)
	if err != nil {
		logger.Fatalf("Starting mysql job failed: %s", err)
	}
}

func stopMysqlMonitProcesses() error {
	cmd := exec.Command(monitPath, "unmonitor", "mysql")
	err := execer.Run(cmd)
	if err != nil {
		return err
	}

	cmd = exec.Command("/var/vcap/jobs/mysql/bin/mysql_ctl", "stop")
	return execer.Run(cmd)
}
