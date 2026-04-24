package bpm_client

import (
	"strings"
	"time"

	"github.com/pkg/errors"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . CommandRunner
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type BpmClient struct {
	BpmBinary     string
	JobName       string
	ProcessName   string
	Timeout       time.Duration
	CommandRunner CommandRunner
}

func NewClient(bpmBinary, jobName, processName string, timeout time.Duration, commandRunner CommandRunner) *BpmClient {
	return &BpmClient{
		BpmBinary:     bpmBinary,
		JobName:       jobName,
		ProcessName:   processName,
		Timeout:       timeout,
		CommandRunner: commandRunner,
	}
}

func New(bpmBinary, jobName, processName string, timeout time.Duration) *BpmClient {
	return NewClient(bpmBinary, jobName, processName, timeout, NewRealCommandRunner(timeout))
}

func (c *BpmClient) Start(serviceName string) error {
	args := []string{"start", c.JobName, "-p", c.ProcessName}
	_, err := c.CommandRunner.Run(c.BpmBinary, args...)
	if err != nil {
		return errors.Wrap(err, "failed to start service")
	}
	
	// Wait for process to actually be running (matching MonitClient behavior)
	if err := c.waitForStatus(serviceName, "running"); err != nil {
		return errors.Wrapf(err, "timed out waiting for %s BPM service to start", serviceName)
	}
	
	return nil
}

func (c *BpmClient) Stop(serviceName string) error {
	args := []string{"stop", c.JobName, "-p", c.ProcessName}
	_, err := c.CommandRunner.Run(c.BpmBinary, args...)
	if err != nil {
		return errors.Wrap(err, "failed to stop service")
	}
	
	// Wait for process to actually be stopped (matching MonitClient behavior)
	if err := c.waitForStatus(serviceName, "stopped"); err != nil {
		return errors.Wrapf(err, "timed out waiting for %s BPM service to stop", serviceName)
	}
	
	return nil
}

func (c *BpmClient) Status(serviceName string) (string, error) {
	args := []string{"pid", c.JobName, "-p", c.ProcessName}
	output, err := c.CommandRunner.Run(c.BpmBinary, args...)
	if err != nil {
		if strings.Contains(err.Error(), "exit status") {
			return "stopped", nil
		}
		return "", errors.Wrap(err, "failed to get service status")
	}
	
	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput != "" {
		return "running", nil
	}
	
	return "stopped", nil
}

func (c *BpmClient) waitForStatus(serviceName, desiredStatus string) error {
	timer := time.NewTimer(c.Timeout)
	ticker := time.NewTicker(time.Second)
	defer timer.Stop()
	defer ticker.Stop()

	var lastStatus = "unknown"
	for {
		select {
		case <-timer.C:
			return errors.Errorf("service status=%v", lastStatus)
		case <-ticker.C:
			var err error
			lastStatus, err = c.Status(serviceName)
			if err != nil {
				return err
			}

			if lastStatus == desiredStatus {
				return nil
			}
		}
	}
}