package bosh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"

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
		"--auto",
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
		"--no-redact",
	}

	for _, o := range options {
		o(&args)
	}

	return cmd.RunCustom(cmd.WithCwd("../.."), "bosh", args...)
}

func RedeployPXC(deploymentName string, options ...DeployOptionFunc) error {
	manifestFile, err := os.CreateTemp("", "pxc_manifest_")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for bosh manifest: %w", err)
	}
	defer func(name string) {
		_ = os.Remove(name)
	}(manifestFile.Name())

	if err = cmd.RunWithoutOutput(manifestFile, "bosh",
		"--deployment="+deploymentName,
		"manifest"); err != nil {
		return fmt.Errorf("failed to retrieve bosh manifest: %w", err)
	}

	if err = manifestFile.Close(); err != nil {
		return fmt.Errorf("failed to close manifest file: %w", err)
	}

	args := []string{
		"--non-interactive",
		"--deployment=" + deploymentName,
		"deploy",
		"--no-redact",
		"--tty",
		manifestFile.Name(),
	}
	for _, o := range options {
		o(&args)
	}

	return cmd.RunCustom(cmd.WithCwd("../.."), "bosh", args...)
}

// DeployPXC deploys the top-level pxc-deployment.yml manifest from the top-level of this pxc-release repo
// This function sets the current working directory to the top-level of this repo
func DeployPXC(deploymentName string, options ...DeployOptionFunc) error {
	args := []string{
		"--non-interactive",
		"--deployment=" + deploymentName,
		"deploy",
		"--no-redact",
		"pxc-deployment.yml",
		"--ops-file=operations/deployment-name.yml",
		"--var=deployment_name=" + deploymentName,
		"--vars-env=BOSH_VAR",
	}

	if pxcVersion := os.Getenv("PXC_DEPLOY_VERSION"); pxcVersion != "" {
		args = append(args,
			"--ops-file=operations/pxc-version.yml",
			"--var=pxc_version="+pxcVersion,
		)
	}

	if v := os.Getenv("MYSQL_VERSION"); v != "" {
		args = append(args,
			"--ops-file=operations/mysql-version.yml",
			fmt.Sprintf("--var=mysql_version=%q", v),
		)
	}

	if stemcellOS := os.Getenv("STEMCELL_OS"); stemcellOS != "" {
		args = append(args,
			"--ops-file=operations/stemcell-os.yml",
			fmt.Sprintf("--var=stemcell_os=%q", stemcellOS),
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

// Var is a helper method to set a single interpolated key=value pair in a bosh manifest
// Syntactic sugar for bosh deploy --var=$key=$value
func Var(key, value string) DeployOptionFunc {
	return func(args *[]string) {
		*args = append(*args, fmt.Sprintf("--var=%s=%q", key, value))
	}
}

// VarsEnv is a helper method to read bosh variables from the environment
// Syntactic sugar for bosh deploy --vars-env=$prefix
// Example VarsEnv("BOSH_VAR") will resolve variables by looking up BOSH_VAR_${var_name}
func VarsEnv(prefix string) DeployOptionFunc {
	return func(args *[]string) {
		*args = append(*args, "--vars-env="+prefix)
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
	if err := cmd.RunWithoutOutput(io.MultiWriter(&output, GinkgoWriter),
		"bosh",
		"--deployment="+deploymentName,
		"ssh",
		instanceSpec,
		"--json",
		"--results",
		"--command="+cmdString,
	); err != nil {
		return output.String(), fmt.Errorf("remote command %q failed: %w", cmdString, err)
	}

	var result struct {
		Tables []struct {
			Rows []struct {
				Stdout string `json:"stdout"`
			} `json:"Rows"`
		} `json:"Tables"`
	}

	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return output.String(), fmt.Errorf("failed to unmarshal bosh cli response (%q): %w", output.String(), err)
	}

	if len(result.Tables) == 0 || len(result.Tables[0].Rows) == 0 {
		return output.String(), fmt.Errorf("no tables or rows provided by bosh cli (%q)", output.String())
	}

	var out strings.Builder

	for _, row := range result.Tables[0].Rows {
		_, _ = out.WriteString(row.Stdout)
	}

	return strings.TrimSpace(out.String()), nil
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
