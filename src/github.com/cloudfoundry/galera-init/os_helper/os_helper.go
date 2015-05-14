package os_helper

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"time"
)

type OsHelper interface {
	RunCommand(executable string, args ...string) (string, error)
	RunCommandWithTimeout(timeout int, logFileName string, executable string, args ...string) error
	StartCommand(logFileName string, executable string, args ...string) (*exec.Cmd, error)
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

// Runs command with stdout and stderr pipes connected to process
func (h OsHelperImpl) RunCommandWithTimeout(timeout int, logFileName string, executable string, args ...string) error {
	cmd := exec.Command(executable, args...)
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	maxRunTime := time.Duration(timeout) * time.Second
	errChannel := make(chan error, 1)
	readyChan := make(chan interface{})
	go func() {
		cmd.Start()
		close(readyChan)
		errChannel <- cmd.Wait()
	}()

	<-readyChan
	select {
	case <-time.After(maxRunTime):
		cmd.Process.Kill()
		return errors.New("Command timed out")
	case err := <-errChannel:
		return err
	}
}

func (h OsHelperImpl) StartCommand(logFileName string, executable string, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(executable, args...)
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	cmd.Start()
	return cmd, nil
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
