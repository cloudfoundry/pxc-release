package start_manager

import (
	"os"

	"github.com/pivotal-golang/lager"
)

type Runner struct {
	mgr    *StartManager
	logger lager.Logger
}

func NewRunner(mgr *StartManager, logger lager.Logger) Runner {
	return Runner{
		mgr:    mgr,
		logger: logger,
	}
}

func (r Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := r.mgr.Execute()
	if err != nil {
		return err
	}
	close(ready)

	<-signals
	err = r.mgr.Shutdown()
	return err
}
