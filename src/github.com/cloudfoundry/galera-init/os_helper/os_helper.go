package os_helper

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"
)

//go:generate counterfeiter . OsHelper
type OsHelper interface {
	RunCommand(executable string, args ...string) (string, error)
	StartCommand(logFileName string, executable string, args ...string) (*exec.Cmd, error)
	WaitForCommand(cmd *exec.Cmd) chan error
	FileExists(filename string) bool
	ReadFile(filename string) (string, error)
	WriteStringToFile(filename string, contents string) error
	Sleep(duration time.Duration)
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
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if len(os.Getenv("APPEND_TO_PATH")) > 0 {
		pathString := fmt.Sprintf("%s:%s", os.Getenv("PATH"), os.Getenv("APPEND_TO_PATH"))
		os.Setenv("PATH", pathString)
	}
	cmd.Env = os.Environ()

	cmd.Start()
	return cmd, nil
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
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b[:]), nil
}

// Overwrite the contents, creating if necessary. Panic on err
func (h OsHelperImpl) WriteStringToFile(filename string, contents string) error {
	err := ioutil.WriteFile(filename, []byte(contents), 0644)
	return err
}

func (h OsHelperImpl) Sleep(duration time.Duration) {
	time.Sleep(duration)
}
