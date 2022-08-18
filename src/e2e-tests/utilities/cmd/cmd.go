package cmd

import (
	"io"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

type MutatorFunc func(*exec.Cmd)

func Run(name string, args ...string) error {
	return RunCustom(nil, name, args...)
}

func RunWithoutOutput(w io.Writer, name string, args ...string) error {
	return RunCustom(func(cmd *exec.Cmd) { cmd.Stdout = w }, name, args...)
}

func RunCustom(mutator MutatorFunc, name string, args ...string) error {
	defer ginkgo.GinkgoWriter.Println()
	cmd := exec.Command(name, args...)
	cmd.Stderr = ginkgo.GinkgoWriter
	cmd.Stdout = ginkgo.GinkgoWriter

	if mutator != nil {
		mutator(cmd)
	}

	ginkgo.GinkgoWriter.Println("$", strings.Join(cmd.Args, " "))
	return cmd.Run()
}

func WithCwd(path string) MutatorFunc {
	return func(cmd *exec.Cmd) {
		cmd.Dir = path
	}
}
