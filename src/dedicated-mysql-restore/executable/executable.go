package executable

import (
	"os/exec"
)

//go:generate counterfeiter . Executable
type Executable interface {
	Run(cmd *exec.Cmd) error
	Start(cmd *exec.Cmd) error
}
