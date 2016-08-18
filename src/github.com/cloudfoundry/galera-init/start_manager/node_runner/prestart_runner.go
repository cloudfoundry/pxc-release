package node_runner

import (
	"os"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
)

type PrestartRunner struct {
	mgr    start_manager.StartManager
	logger lager.Logger
}

func NewPrestartRunner(mgr start_manager.StartManager, logger lager.Logger) PrestartRunner {
	return PrestartRunner{
		mgr:    mgr,
		logger: logger,
	}
}

func (r PrestartRunner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
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

	r.logger.Info("start_manager process is ready")
	close(ready)

	//fmt.Println("Starting to listen on signals channel...")
	s := <-signals
	if s == os.Kill {
		r.logger.Info("Received shutdown signal. Shutting down mysqld.")
	}
	return nil
}
