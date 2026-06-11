package bridge

import (
	"fmt"
	"net"
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"

	"github.com/cloudfoundry-incubator/switchboard/domain"
)

type Runner struct {
	logger             lager.Logger
	address            string
	TrafficEnabledChan chan bool
	ActiveBackendChan  chan *domain.Backend
	timeout            time.Duration
}

func NewRunner(
	address string,
	timeout time.Duration,
	logger lager.Logger,
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

	shutdown := make(chan struct{})
	c := make(chan net.Conn)
	go r.blockingAccept(listener, c, shutdown)

	go func(shutdown <-chan struct{}, c <-chan net.Conn) {
		trafficEnabled := true
		var activeBackend *domain.Backend
		for {
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
					r.logger.Info("Done severing connections, new active backend:", lager.Data{"backend": a.AsJSON()})
				} else {
					r.logger.Info("Done severing connections, new active backend:", lager.Data{"backend": nil})
				}

			case clientConn := <-c:
				if !trafficEnabled {
					_ = clientConn.Close()
					continue
				}

				go func(clientConn net.Conn, activeBackend *domain.Backend) {
					if activeBackend == nil {
						_ = clientConn.Close()
						r.logger.Info("No active backend")
						return
					}

					err := activeBackend.Bridge(clientConn)
					if err != nil {
						_ = clientConn.Close()
						r.logger.Error("Error routing to backend", err)
					}
				}(clientConn, activeBackend)
			}
		}
	}(shutdown, c)

	close(ready)

	signal := <-signals
	r.logger.Info("Received signal", lager.Data{"signal": signal})

	time.Sleep(r.timeout)

	close(shutdown)
	_ = listener.Close()

	r.logger.Info("Proxy runner has exited")
	return nil
}

func (r Runner) blockingAccept(l net.Listener, c chan<- net.Conn, shutdown <-chan struct{}) {
	for {
		clientConn, err := l.Accept()
		if err != nil {
			select {
			case <-shutdown:
				return
			default:
			}
			r.logger.Error("Error accepting client connection", err)
			continue
		}

		select {
		case c <- clientConn:
		case <-shutdown:
			_ = clientConn.Close()
			return
		}
	}
}
