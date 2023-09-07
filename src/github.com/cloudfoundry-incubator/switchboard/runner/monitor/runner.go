package monitor

import (
	"log/slog"
	"os"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Monitor
type Monitor interface {
	Monitor(<-chan interface{})
}

type Runner struct {
	logger  *slog.Logger
	monitor Monitor
}

func NewRunner(monitor Monitor, logger *slog.Logger) Runner {
	return Runner{
		logger:  logger,
		monitor: monitor,
	}
}

func (pr Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	shutdown := make(chan interface{})
	pr.monitor.Monitor(shutdown)

	close(ready)

	signal := <-signals
	pr.logger.Info("Received signal", "signal", signal.String())
	close(shutdown)

	return nil
}
