package bosh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"e2e-tests/utilities/cmd"
)

// DeployOptionFunc implementations can be passed to DeployManifest
type DeployOptionFunc func(args *[]string)

type MatchInstanceFunc func(instance Instance) bool

type Instance struct {
	IP       string `json:"ips"`
	Instance string `json:"instance"`
	Index    string `json:"index"`
	VMCid    string `json:"vm_cid"`
}

func CloudCheck(deploymentName string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--non-interactive",
		"--tty",
		"cloud-check",
		"--resolution=recreate_vm",
	)
}

func DeleteDeployment(deploymentName string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--non-interactive",
		"delete-deployment",
		"--force",
	)
}

func DeleteVM(deploymentName, cid string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--tty",
		"--non-interactive",
		"delete-vm",
		cid,
	)
}

// Deploy run bosh deploy with an arbitrary manifest file and arguments
//
//	This function uses the default working directory
func Deploy(deploymentName, manifestPath string, options ...DeployOptionFunc) error {
	args := []string{
		"--non-interactive",
		"--tty",
		"--deployment=" + deploymentName,
		"deploy", manifestPath,
	}

	for _, o := range options {
		o(&args)
	}

	return cmd.Run("bosh", args...)
}

// DeployPXC deploys the top-level pxc-deployment.yml manifest from the top-level of this pxc-release repo
// This function sets the current working directory to the top-level of this repo
func DeployPXC(deploymentName string, options ...DeployOptionFunc) error {
	args := []string{
		"--non-interactive",
		"--tty",
		"--deployment=" + deploymentName,
		"deploy",
		"pxc-deployment.yml",
		"--ops-file=operations/deployment-name.yml",
		"--var=deployment_name=" + deploymentName,
	}

	if pxcVersion := os.Getenv("PXC_DEPLOY_VERSION"); pxcVersion != "" {
		args = append(args,
			"--ops-file=operations/pxc-version.yml",
			"--var=pxc_version="+pxcVersion,
		)
	}

	for _, o := range options {
		o(&args)
	}

	return cmd.RunCustom(cmd.WithCwd("../.."), "bosh", args...)
}

// Operation is a helper method to compute an ops-file path relative to the top-level ./operations directory
func Operation(path string) DeployOptionFunc {
	// This function assumes that bosh deploy is run with a cwd of the top of this repo.
	// All standard ops-files will be relative to ./operations/
	return func(args *[]string) {
		*args = append(*args, "--ops-file="+filepath.Join("operations", path))
	}
}

func Var(key, value string) DeployOptionFunc {
	// This function assumes that bosh deploy is run with a cwd of the top of this repo.
	// All standard ops-files will be relative to ./operations/
	return func(args *[]string) {
		*args = append(*args, "--var="+key+"="+value)
	}
}

func Instances(deploymentName string, matchInstanceFunc MatchInstanceFunc) ([]Instance, error) {
	var output bytes.Buffer

	if err := cmd.RunWithoutOutput(&output,
		"bosh",
		"--non-interactive",
		"--tty",
		"--deployment="+deploymentName,
		"instances",
		"--details",
		"--json",
	); err != nil {
		return nil, err
	}

	var result struct {
		Tables []struct {
			Rows []Instance
		}
	}

	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to decode bosh instances output: %v", err)
	}

	var instances []Instance

	for _, row := range result.Tables[0].Rows {
		if matchInstanceFunc(row) {
			instances = append(instances, row)
		}
	}

	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].Index < instances[j].Index
	})

	return instances, nil
}

func InstanceIPs(deploymentName string, matchInstanceFunc MatchInstanceFunc) (addresses []string, err error) {
	instances, err := Instances(deploymentName, matchInstanceFunc)
	if err != nil {
		return nil, err
	}

	for _, row := range instances {
		addresses = append(addresses, row.IP)
	}

	return addresses, nil
}

// MatchByInstanceGroup matches by comparing an instance's group against the provided name
func MatchByInstanceGroup(name string) MatchInstanceFunc {
	return func(i Instance) bool {
		components := strings.SplitN(i.Instance, "/", 2)
		return components[0] == name
	}
}

// MatchByIndexedName matches by comparing the provided name to INSTANCE-GROUP/INDEX
func MatchByIndexedName(name string) MatchInstanceFunc {
	return func(i Instance) bool {
		components := strings.SplitN(i.Instance, "/", 2)
		instanceGroup := components[0]
		return instanceGroup+"/"+i.Index == name
	}
}

func RunErrand(deploymentName, errandName, instanceSpec string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--non-interactive",
		"--tty",
		"run-errand",
		errandName,
		"--instance="+instanceSpec,
	)
}

func Restart(deploymentName, instanceSpec string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--non-interactive",
		"--tty",
		"restart",
		instanceSpec,
	)
}

func RemoteCommand(deploymentName, instanceSpec, cmdString string) (string, error) {
	var output bytes.Buffer
	err := cmd.RunWithoutOutput(&output,
		"bosh",
		"--deployment="+deploymentName,
		"ssh",
		instanceSpec,
		"--column=Stdout",
		"--results",
		"--command="+cmdString,
	)
	return strings.TrimSpace(output.String()), err
}

func Logs(deploymentName, instanceSpec, filter string) (*bytes.Buffer, error) {
	var output bytes.Buffer
	err := cmd.RunWithoutOutput(&output,
		"bosh",
		"--deployment="+deploymentName,
		"logs",
		"--num=1024",
		instanceSpec,
		"--only="+filter,
	)

	return &output, err
}

func Scp(deploymentName, sourcePath, destPath string) error {
	return cmd.Run("bosh",
		"--deployment="+deploymentName,
		"scp",
		sourcePath,
		destPath,
	)
}
