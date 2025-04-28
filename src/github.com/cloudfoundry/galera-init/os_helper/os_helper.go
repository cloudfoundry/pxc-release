package os_helper

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/renameio/v2"
	"github.com/pkg/errors"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . OsHelper
type OsHelper interface {
	RunCommand(executable string, args ...string) (string, error)
	StartCommand(logFileName string, executable string, args ...string) (*exec.Cmd, error)
	WaitForCommand(cmd *exec.Cmd) chan error
	FileExists(filename string) bool
	ReadFile(filename string) (string, error)
	WriteStringToFile(filename string, contents string) error
	Sleep(duration time.Duration)
	KillCommand(cmd *exec.Cmd, signal os.Signal) error
}

type OsHelperImpl struct{}

func NewImpl() *OsHelperImpl {
	return &OsHelperImpl{}
}

// Runs command with stdout and stderr pipes connected to process
func (h OsHelperImpl) RunCommand(executable string, args ...string) (string, error) {
	cmd := exec.Command(executable, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func (h OsHelperImpl) StartCommand(logFileName string, executable string, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(executable, args...)
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "error logging output for command %q to filename %q", executable, logFileName)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	return cmd, errors.Wrapf(cmd.Start(), "error starting %q", executable)
}

func (h OsHelperImpl) WaitForCommand(cmd *exec.Cmd) chan error {
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- cmd.Wait()
	}()
	return errChannel
}

func (h OsHelperImpl) FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return (err == nil)
}

// Read the whole file, panic on err
func (h OsHelperImpl) ReadFile(filename string) (string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b[:]), nil
}

// Overwrite the contents, creating if necessary. Panic on err
func (h OsHelperImpl) WriteStringToFile(filename string, contents string) error {
	return renameio.WriteFile(filename, []byte(contents), 0644)
}

func (h OsHelperImpl) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

func (h OsHelperImpl) KillCommand(cmd *exec.Cmd, signal os.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return errors.New("process-was-not-started")
	}

	if err := cmd.Process.Signal(signal); err != nil {
		return fmt.Errorf("unable-to-kill-process: %w", err)
	}

	return nil
}
