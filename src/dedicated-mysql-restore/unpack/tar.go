package unpack

import (
	"io"
	"os/exec"
)

func ExtractTar(input io.Reader, destinationDirectory string, output io.Writer) error {
	tarCmd := exec.Command("tar", "-x", "-C", destinationDirectory)
	tarCmd.Stdin = input
	tarCmd.Stderr = output
	tarCmd.Stdout = output
	return tarCmd.Run()
}
