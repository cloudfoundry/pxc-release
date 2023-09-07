package bridge

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/switchboard/domain"
)

type Runner struct {
	logger             *slog.Logger
	address            string
	TrafficEnabledChan chan bool
	ActiveBackendChan  chan *domain.Backend
	timeout            time.Duration
}

func NewRunner(
	address string,
	timeout time.Duration,
	logger *slog.Logger,
) Runner {
	backendChan := make(chan *domain.Backend)
	trafficEnabledChan := make(chan bool)

	return Runner{
		logger:             logger,
		ActiveBackendChan:  backendChan,
		TrafficEnabledChan: trafficEnabledChan,
		address:            address,
		timeout:            timeout,
	}
}

func (r Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	r.logger.Info(fmt.Sprintf("Proxy listening on %s", r.address))

	listener, err := net.Listen("tcp", r.address)
	if err != nil {
		return err
	}

	shutdown := make(chan interface{})
	go func(shutdown <-chan interface{}, listener net.Listener) {
		trafficEnabled := true
		var activeBackend *domain.Backend
		e := make(chan error)
		c := make(chan net.Conn)

		for {
			go blockingAccept(listener, c, e)
			select {
			case <-shutdown:
				return
			case t := <-r.TrafficEnabledChan:
				// ENABLED -> DISABLED
				if trafficEnabled && !t {
					if activeBackend != nil {
						activeBackend.SeverConnections()
					}
				}

				trafficEnabled = t

			case a := <-r.ActiveBackendChan:
				// NEW ACTIVE BACKEND
				if activeBackend != nil {
					activeBackend.SeverConnections()
				}

				activeBackend = a
				if a != nil {
					r.logger.Info(" Done severing connections, new active backend", "backend", a)
				} else {
					r.logger.Info("Done severing connections, new active backend", "backend", nil)
				}

			case clientConn := <-c:
				if !trafficEnabled {
					clientConn.Close()
					continue
				}

				go func(clientConn net.Conn, activeBackend *domain.Backend) {
					if activeBackend == nil {
						clientConn.Close()
						r.logger.Error("No active backend", "error", err)
						return
					}

					err = activeBackend.Bridge(clientConn)
					if err != nil {
						clientConn.Close()
						r.logger.Error("Error routing to backend", "error", err)
					}
				}(clientConn, activeBackend)
			case err = <-e:
				if err != nil {
					r.logger.Error("Error accepting client connection", "error", err)
					continue
				}
			}
		}
	}(shutdown, listener)

	close(ready)

	signal := <-signals
	r.logger.Info("Received signal", "signal", signal.String())

	time.Sleep(r.timeout)

	close(shutdown)
	listener.Close()

	r.logger.Info("Proxy runner has exited")
	return nil
}

func blockingAccept(l net.Listener, c chan<- net.Conn, e chan<- error) {
	clientConn, err := l.Accept()

	if err != nil {
		e <- err
		return
	}

	c <- clientConn
}
