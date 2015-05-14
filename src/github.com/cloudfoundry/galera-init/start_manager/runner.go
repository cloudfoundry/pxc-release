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
		r.logger.Error("Failed starting Maria with error:", err)
		return err
	}

	cmd, err := r.mgr.GetMysqlCmd()
	if err != nil {
		r.logger.Error("Error getting Maria process", err)
		return err
	}

	r.logger.Info("start_manager process is ready")
	close(ready)

	mariaExited := make(chan error)
	go func() {
		err = cmd.Wait()
		mariaExited <- err
	}()

	var shutdownErr error
	select {
	case <-signals:
		r.logger.Info("Received shutdown signal. Shutting down Maria.")
		shutdownErr = r.mgr.Shutdown()
	case err = <-mariaExited:
		r.logger.Error("Maria process exited", err)
		shutdownErr = err
	}
	return shutdownErr
}
