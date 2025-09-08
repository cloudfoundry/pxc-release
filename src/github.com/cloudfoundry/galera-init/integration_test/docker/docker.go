package docker

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
)

type ContainerSpec struct {
	Image          string
	Network        string
	Env            []string
	Volumes        []string
	Args           []string
	Entrypoint     string
	Ports          []string
	HealthCmd      string
	HealthInterval string
	Name           string
}

func Command(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out := bytes.Buffer{}
	cmd.Stdout = io.MultiWriter(&out, ginkgo.GinkgoWriter)
	cmd.Stderr = ginkgo.GinkgoWriter
	ginkgo.GinkgoWriter.Println("$", strings.Join(cmd.Args, " "))
	err := cmd.Run()

	return strings.TrimSpace(out.String()), err
}

func RunContainer(spec ContainerSpec) (string, error) {
	containerID, err := CreateContainer(spec)
	if err != nil {
		return "", err
	}

	return containerID, StartContainer(containerID)
}

func StartContainer(name string) error {
	_, err := Command("start", name)
	return err
}

func StopContainer(name string) error {
	_, err := Command("stop", name)
	return err
}

func CreateContainer(spec ContainerSpec) (string, error) {
	args := []string{
		"create",
	}

	if spec.Name != "" {
		args = append(args, "--name="+spec.Name)
	}

	if spec.Entrypoint != "" {
		args = append(args, "--entrypoint="+spec.Entrypoint)
	}

	if spec.HealthCmd != "" {
		args = append(args, "--health-cmd="+spec.HealthCmd)
	}

	if spec.HealthInterval != "" {
		args = append(args, "--health-interval="+spec.HealthInterval)
	}

	if spec.Network != "" {
		args = append(args, "--network="+spec.Network)
	}

	for _, e := range spec.Env {
		args = append(args, "--env="+e)
	}

	for _, v := range spec.Volumes {
		args = append(args, "--volume="+v)
	}

	//for _, p := range spec.ExposePorts {
	//	args = append(args, "--expose="+p)
	//}

	for _, p := range spec.Ports {
		args = append(args, "--publish="+p)
	}

	args = append(args, spec.Image)
	args = append(args, spec.Args...)

	return Command(args...)
}

func WaitHealthy(container string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return errors.New("timeout waiting for healthy container")
		case <-ticker.C:
			result, err := Command("container", "inspect", "--format={{.State.Status}} {{.State.Health.Status}}", container)
			if err != nil {
				return fmt.Errorf("error inspecting container: %v", err)
			}

			if strings.HasPrefix(result, "exited ") {
				return fmt.Errorf("container exited")
			}

			if result == "running healthy" {
				return nil
			}
		}
	}
}

func RemoveContainer(name string) error {
	_, err := Command("container", "rm", "--force", "--volumes", name)
	return err
}

func CreateNetwork(name string) error {
	_, err := Command("network", "create", name)
	return err
}

func RemoveNetwork(name string) error {
	_, err := Command("network", "remove", name)
	return err
}

func Copy(src, dst string) error {
	_, err := Command("container", "cp", "--archive", src, dst)
	return err
}

func ContainerPort(containerID, portSpec string) (string, error) {
	hostPort, err := Command("container", "port", containerID, portSpec)
	if err != nil {
		return "", err
	}

	_, port, err := net.SplitHostPort(strings.Fields(hostPort)[0])
	if err != nil {
		return "", err
	}

	return port, nil
}

func MySQLDB(containerName string) (*sql.DB, error) {
	mysqlPort, err := ContainerPort(containerName, "3306/tcp")
	if err != nil {
		return nil, err
	}

	dsn := "root@tcp(127.0.0.1:" + mysqlPort + ")/mysql?interpolateParams=true"

	return sql.Open("mysql", dsn)
}

func Kill(containerID, signal string) error {
	_, err := Command("container", "kill", "--signal", signal, containerID)
	return err
}

func InspectStatus(containerID string) (string, error) {
	return Command("container", "inspect", "--format", "{{.State.Status}}", containerID)
}

func InspectExitCode(containerID string) (int, error) {
	code, err := Command("container", "inspect", "--format", "{{.State.ExitCode}}", containerID)
	if err != nil {
		return -1, err
	}

	value, err := strconv.Atoi(strings.TrimSpace(code))
	if err != nil {
		return -1, err
	}
	return value, nil
}

func Exec(containerID, command string) error {
	_, err := Command("exec", containerID, "/bin/bash", "-c", command)
	return err
}

func HardKillMySQL(containerID string) error {
	return Exec(containerID, "pkill --signal 9 mysql")
}
