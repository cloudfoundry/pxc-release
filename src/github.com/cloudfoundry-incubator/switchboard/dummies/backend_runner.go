package dummies

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/onsi/ginkgo/v2"
)

type BackendRunner struct {
	index uint
	port  uint
}

func NewBackendRunner(index uint, backend config.Backend) *BackendRunner {
	return &BackendRunner{
		index: index,
		port:  backend.Port,
	}
}

func (fb *BackendRunner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	address := fmt.Sprintf("%s:%d", "localhost", fb.port)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New(fmt.Sprintf("Backend error listening on address: %s - %s\n", address, err.Error()))
	}
	defer listener.Close()

	errChan := make(chan error, 1)
	var conn net.Conn
	go func() {
		for {
			conn, err = listener.Accept()
			if err != nil {
				errChan <- errors.New(fmt.Sprintf("Error accepting: %v", err.Error()))
			} else {
				defer conn.Close()
				go fb.handleRequest(conn)
			}
		}
	}()

	fmt.Fprintf(ginkgo.GinkgoWriter, "Backend listening on port %s\n", address)
	close(ready)

	for {
		select {
		case err := <-errChan:
			return err
		case <-signals:
			listener.Close()
			return nil
		}
	}
}

func (fb *BackendRunner) handleRequest(conn net.Conn) {
	dataCh := make(chan []byte)
	errCh := make(chan error)

	go func(ch chan []byte, eCh chan error) {
		for {
			data := make([]byte, 1024)
			n, err := conn.Read(data)
			if err != nil {
				fmt.Fprintln(ginkgo.GinkgoWriter, fmt.Sprintf("Dummy backend received error on connection: %v", err))
				eCh <- err
				return
			}
			fmt.Fprintln(ginkgo.GinkgoWriter, "Dummy backend received on connection: "+string(data))
			ch <- data[:n]
		}
	}(dataCh, errCh)

	for {
		select {
		case data := <-dataCh:
			response := fmt.Sprintf(
				`{"BackendPort": %d, "BackendIndex": %d, "Message": "%s"}`,
				fb.port,
				fb.index,
				string(data),
			)
			fmt.Fprintln(ginkgo.GinkgoWriter, "Dummy backend writing to connection: Echo: "+response)
			conn.Write([]byte(response))
		case err := <-errCh:
			fmt.Fprintln(ginkgo.GinkgoWriter, "Error: "+err.Error())
			conn.Close()
			break
		}
	}
}
