package test_helpers

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gbytes"
)

type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type ContainerOption func(*docker.CreateContainerOptions)

func ContainerDBConnection(container *docker.Container, port docker.Port) (*sql.DB, error) {
	hostPort := HostPort(port, container)
	dbURI := fmt.Sprintf("root@tcp(localhost:%s)/", hostPort)
	return sql.Open("mysql", dbURI)
}

func CreateNetwork(dockerClient *docker.Client, name string) (*docker.Network, error) {
	return dockerClient.CreateNetwork(docker.CreateNetworkOptions{Name: name})
}

func HardKillMySQL(dockerClient *docker.Client, container *docker.Container) (*ExecResult, error) {
	exec, err := dockerClient.CreateExec(docker.CreateExecOptions{
		Container: container.ID,
		Cmd: []string{
			"bash",
			"-c",
			"kill -9 $(pidof mysqld)",
		},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, err
	}

	return RunExec(dockerClient, exec)
}

func HostPort(containerPort docker.Port, container *docker.Container) string {
	// FIXME: handle errors if network settings are not configured properly
	return container.NetworkSettings.Ports[containerPort][0].HostPort
}

func FetchContainerFileContents(dockerClient *docker.Client, container *docker.Container, filePath string) (string, error) {
	logContents := bytes.Buffer{}
	err := dockerClient.DownloadFromContainer(
		container.ID,
		docker.DownloadFromContainerOptions{
			OutputStream: &logContents,
			Path:         filePath,
		},
	)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(
		strings.Trim(logContents.String(), "\x00"),
	), nil
}

func PullImage(dockerClient *docker.Client, repository string) error {
	return dockerClient.PullImage(docker.PullImageOptions{
		Repository:   repository,
		OutputStream: ginkgo.GinkgoWriter,
	}, docker.AuthConfiguration{})
}

func RemoveNetwork(dockerClient *docker.Client, network *docker.Network) error {
	return dockerClient.RemoveNetwork(network.ID)
}

func RemoveContainer(dockerClient *docker.Client, container *docker.Container) error {
	switch err := dockerClient.RemoveContainer(docker.RemoveContainerOptions{
		ID:            container.ID,
		RemoveVolumes: true,
		Force:         true,
	}); err.(type) {
	case *docker.NoSuchContainer:
		return nil
	default:
		return err
	}
}

func RunContainer(dockerClient *docker.Client, containerName string, options ...ContainerOption) (*docker.Container, error) {
	createContainerOptions := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			ExposedPorts: map[docker.Port]struct{}{},
		},
		HostConfig: &docker.HostConfig{
			PublishAllPorts: true,
		},
	}

	for _, opt := range options {
		opt(&createContainerOptions)
	}

	container, err := dockerClient.CreateContainer(createContainerOptions)
	if err != nil {
		return container, err
	}

	if err := dockerClient.StartContainer(container.ID, nil); err != nil {
		return container, err
	}

	inspectedContainer, err := dockerClient.InspectContainer(container.ID)
	if err != nil {
		return container, err
	}

	return inspectedContainer, nil
}

func RunExec(dockerClient *docker.Client, exec *docker.Exec) (*ExecResult, error) {
	var (
		stdout, stderr bytes.Buffer
	)

	err := dockerClient.StartExec(exec.ID, docker.StartExecOptions{
		OutputStream: io.MultiWriter(ginkgo.GinkgoWriter, &stdout),
		ErrorStream:  io.MultiWriter(ginkgo.GinkgoWriter, &stderr),
	})
	if err != nil {
		return nil, err
	}

	execInspect, err := dockerClient.InspectExec(exec.ID)
	if err != nil {
		return nil, err
	}

	return &ExecResult{
		ExitCode: execInspect.ExitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func StreamLogs(dockerClient *docker.Client, container *docker.Container) *gbytes.Buffer {
	result := gbytes.NewBuffer()
	go dockerClient.Logs(docker.LogsOptions{
		Container:    container.ID,
		OutputStream: io.MultiWriter(ginkgo.GinkgoWriter, result),
		ErrorStream:  io.MultiWriter(ginkgo.GinkgoWriter, result),
		Follow:       true,
		Stdout:       true,
		Stderr:       true,
		Tail:         "0",
	})
	return result
}

func AddBinds(binds ...string) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.HostConfig.Binds = append(createOpts.HostConfig.Binds, binds...)
	}
}

func AddEnvVars(envVars ...string) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.Config.Env = append(createOpts.Config.Env, envVars...)
	}
}

func AddExposedPorts(ports ...docker.Port) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		for _, port := range ports {
			createOpts.Config.ExposedPorts[port] = struct{}{}
		}
	}

}

func WithCmd(cmd ...string) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.Config.Cmd = cmd
	}
}

func WithEntrypoint(cmd string) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.Config.Entrypoint = []string{cmd}
	}
}

func WithImage(imageID string) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.Config.Image = imageID
	}
}

func WithNetwork(network *docker.Network) ContainerOption {
	return func(createOpts *docker.CreateContainerOptions) {
		createOpts.NetworkingConfig = &docker.NetworkingConfig{
			EndpointsConfig: map[string]*docker.EndpointConfig{
				network.Name: {NetworkID: network.ID},
			},
		}
	}
}
