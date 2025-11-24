package docker

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type ContainerSpec struct {
	Name           string
	Image          string
	Network        string
	User           string
	Ports          []string
	HealthCmd      string
	HealthInterval string
	Env            []string
	Volumes        []string
	Entrypoint     string
	Args           []string
}

func Command(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out := bytes.Buffer{}
	cmd.Stdout = io.MultiWriter(&out, GinkgoWriter)
	cmd.Stderr = GinkgoWriter
	GinkgoWriter.Println("$", strings.Join(cmd.Args, " "))
	err := cmd.Run()

	return strings.TrimSpace(out.String()), err
}

func RunContainer(spec ContainerSpec) string {
	GinkgoHelper()

	containerID, err := CreateContainer(spec)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to create docker container: %s`, err)

	DeferCleanup(func() {
		_ = RemoveContainer(containerID)
	})

	StartContainer(containerID)

	return containerID
}

func StartContainer(name string) {
	GinkgoHelper()
	_, err := Command("start", name)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to start docker container: %s`, err)
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

	if spec.Network != "" {
		args = append(args, "--network="+spec.Network)
	}

	if spec.User != "" {
		args = append(args, "--user="+spec.User)
	}

	if spec.HealthCmd != "" {
		args = append(args, "--health-cmd="+spec.HealthCmd)
	}

	if spec.HealthInterval != "" {
		args = append(args, "--health-interval="+spec.HealthInterval)
	}

	for _, e := range spec.Env {
		args = append(args, "--env="+e)
	}

	for _, v := range spec.Volumes {
		args = append(args, "--volume="+v)
	}

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

func WaitExited(container string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return errors.New("timeout waiting for healthy container")
		case <-ticker.C:
			result, err := Command("container", "inspect", "--format={{.State.Status}}", container)
			if err != nil {
				return fmt.Errorf("error inspecting container: %v", err)
			}

			if result == "exited" {
				return nil
			}
		}
	}
}

func RemoveContainer(name string) error {
	_, err := Command("container", "rm", "--force", "--volumes", name)
	return err
}

func CreateNetwork(name string) {
	GinkgoHelper()
	_, err := Command("network", "create", name)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to create docker network %s`, name)
}

func RemoveNetwork(name string) {
	GinkgoHelper()

	_, err := Command("network", "rm", "--force", name)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to remove docker network %s`, name)
}

func Copy(src, dst string) error {
	_, err := Command("container", "cp", "--archive", src, dst)
	return err
}

func Kill(containerID string, signal string) error {
	_, err := Command("container", "kill", "--signal="+signal, containerID)
	return err
}

func ContainerPort(containerID, portSpec string) string {
	hostPort, err := Command("container", "port", containerID, portSpec)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to get container port for container %s and port %s`, containerID, portSpec)
	Expect(hostPort).NotTo(BeEmpty())

	_, port, err := net.SplitHostPort(strings.Fields(hostPort)[0])
	Expect(err).NotTo(HaveOccurred(),
		`Error extracting port from host address %s`, hostPort)

	return port
}

func MySQLDB(containerName string) (*sql.DB, error) {
	GinkgoHelper()

	mysqlPort := ContainerPort(containerName, "3306/tcp")

	dsn := "root@tcp(127.0.0.1:" + mysqlPort + ")/?interpolateParams=true"

	return sql.Open("mysql", dsn)
}

type CommandOption func(*exec.Cmd)

func Logs(containerID string, options ...CommandOption) error {
	GinkgoWriter.Println("$ docker logs", containerID)
	cmd := exec.Command("docker", "logs", containerID)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	for _, option := range options {
		option(cmd)
	}

	return cmd.Run()
}

func ExitCode(containerID string) (string, error) {
	exitCode, err := Command("container", "inspect", containerID, "-f", "{{.State.ExitCode}}")
	if err != nil {
		return "", err
	}

	return exitCode, nil
}
