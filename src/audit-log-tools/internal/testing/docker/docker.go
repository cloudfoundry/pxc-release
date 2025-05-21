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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

type ContainerSpec struct {
	Image          string
	User           string
	Ports          []string
	HealthCmd      string
	HealthInterval string
	Env            []string
	Volumes        []string
	Args           []string
	TmpFS          string
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

func CreateContainer(spec ContainerSpec) (string, error) {
	args := []string{
		"create",
	}

	if spec.TmpFS != "" {
		args = append(args, "--tmpfs="+spec.TmpFS)
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

func Copy(src, dst string) error {
	_, err := Command("container", "cp", "--archive", src, dst)
	return err
}

func ContainerPort(containerID, portSpec string) string {
	ginkgo.GinkgoHelper()

	hostPort, err := Command("container", "port", containerID, portSpec)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	_, port, err := net.SplitHostPort(strings.Fields(hostPort)[0])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return port
}

func MySQLDB(containerName string) (*sql.DB, error) {
	var (
		mysqlPort = ContainerPort(containerName, "3306/tcp")
		dsn       = "root@tcp(127.0.0.1:" + mysqlPort + ")/?interpolateParams=true"
	)

	return sql.Open("mysql", dsn)
}
