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

	"github.com/go-sql-driver/mysql"
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

func RemoveContainer(name string) error {
	_, err := Command("container", "rm", "--force", "--volumes", name)
	return err
}

func RemoveVolume(name string) error {
	_, err := Command("volume", "rm", "--force", name)
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

// DSNOpt is a functional option for configuring MySQL database connections.
// Use it to customize connection parameters like username, password, or database.
type DSNOpt func(cfg *mysql.Config)

func MySQLDB(containerName string, opts ...DSNOpt) *sql.DB {
	GinkgoHelper()

	mysqlPort := ContainerPort(containerName, "3306/tcp")

	cfg := &mysql.Config{
		User:                 "root",
		Net:                  "tcp",
		Addr:                 net.JoinHostPort("127.0.0.1", mysqlPort),
		InterpolateParams:    true,
		AllowNativePasswords: true,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	connector, err := mysql.NewConnector(cfg)
	Expect(err).NotTo(HaveOccurred())

	return sql.OpenDB(connector)
}

func WithUsernamePassword(username, password string) DSNOpt {
	return func(cfg *mysql.Config) {
		cfg.User = username
		cfg.Passwd = password
	}
}
