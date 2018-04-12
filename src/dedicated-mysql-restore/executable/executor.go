package executable

import "os/exec"

type Executor struct{}

func (e Executor) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

func (e Executor) Start(cmd *exec.Cmd) error {
	return cmd.Start()
}
