package node_runner

import (
	"os"

	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"code.cloudfoundry.org/lager"
)

type Runner struct {
	mgr    start_manager.StartManager
	logger lager.Logger
}

func NewRunner(mgr start_manager.StartManager, logger lager.Logger) Runner {
	return Runner{
		mgr:    mgr,
		logger: logger,
	}
}

func (r Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := r.mgr.Execute()
	if err != nil {
		r.logger.Error("Failed starting mysqld with error:", err)
		//database may have started but failed to accept connections
		shutdownErr := r.mgr.Shutdown()
		if shutdownErr != nil {
			r.logger.Error("Error stopping mysqld process", shutdownErr)
		}
		return err
	}

	cmd, err := r.mgr.GetMysqlCmd()
	if err != nil {
		r.logger.Error("Error getting mysqld process", err)
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
		r.logger.Info("Received shutdown signal. Shutting down mysqld.")
		shutdownErr = r.mgr.Shutdown()
	case err = <-mariaExited:
		r.logger.Error("mysqld process exited with error", err)
		shutdownErr = err
	}
	return shutdownErr
}
