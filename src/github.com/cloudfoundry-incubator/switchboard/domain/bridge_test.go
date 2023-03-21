package domain_test

import (
	"errors"
	"io"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/domain/domainfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridge", func() {
	Describe("#Connect", func() {
		var (
			bridge          domain.Bridge
			client, backend *domainfakes.FakeConn
			logger          lager.Logger
		)

		BeforeEach(func() {
			logger = lagertest.NewTestLogger("Bridge test")
			backend = new(domainfakes.FakeConn)
			client = new(domainfakes.FakeConn)

			backend.ReadReturns(0, io.EOF)
			client.ReadReturns(0, io.EOF)

			bridge = domain.NewBridge(client, backend, logger)
		})

		Context("When operating normally", func() {
			It("forwards data from the client to backend", func() {
				expectedText := "hello"
				var copiedToBackend string
				clientReadCount := 0
				client.ReadStub = func(p []byte) (int, error) {
					if clientReadCount == 0 {
						copy(p, expectedText)
						clientReadCount++
						return len(expectedText), nil
					}
					return 0, io.EOF
				}

				backend.WriteStub = func(p []byte) (int, error) {
					copiedToBackend = string(p)
					return len(expectedText), nil
				}

				go bridge.Connect()
				defer bridge.Close()
				Eventually(client.ReadCallCount).Should(Equal(2))
				Eventually(backend.WriteCallCount).Should(Equal(1))
				Expect(copiedToBackend).To(Equal(expectedText))
			})

			It("forwards data from the backend to client", func() {
				expectedText := "echo: hello"
				var copiedToClient string

				backendReadCount := 0
				backend.ReadStub = func(p []byte) (int, error) {
					if backendReadCount == 0 {
						copy(p, expectedText)
						backendReadCount++
						return len(expectedText), nil
					}
					return 0, io.EOF
				}

				client.WriteStub = func(p []byte) (int, error) {
					copiedToClient = string(p)
					return len(expectedText), nil
				}

				go bridge.Connect()
				defer bridge.Close()
				Eventually(backend.ReadCallCount).Should(Equal(2))
				Eventually(client.WriteCallCount).Should(Equal(1))
				Expect(copiedToClient).To(Equal(expectedText))
			})
		})

		Context("when the client returns an error", func() {
			BeforeEach(func() {
				client.ReadReturns(0, errors.New("Error reading from client"))
			})

			It("Closes the backend", func() {
				bridge.Connect()
				Expect(backend.CloseCallCount()).To(Equal(1))
			})
		})

		Context("when the client returns EOF", func() {
			BeforeEach(func() {
				client.ReadStub = func(p []byte) (int, error) {
					return 0, io.EOF
				}
			})

			It("Closes the backend", func() {
				bridge.Connect()
				Expect(backend.CloseCallCount()).To(Equal(1))
			})
		})

		Context("when the backend returns an error", func() {
			BeforeEach(func() {
				backend.ReadReturns(0, errors.New("Error reading from backend"))
			})

			It("Closes the client", func() {
				bridge.Connect()
				Expect(client.CloseCallCount()).To(Equal(1))
			})
		})

		Context("when the backend returns EOF", func() {
			BeforeEach(func() {
				backend.ReadStub = func(p []byte) (int, error) {
					return 0, io.EOF
				}
			})

			It("Closes the client", func() {
				bridge.Connect()
				Expect(client.CloseCallCount()).To(Equal(1))
			})
		})

		Context("When the connection is closed by calling Close()", func() {
			It("Closes the client and backend", func() {
				go bridge.Connect()
				bridge.Close()
				Eventually(backend.CloseCallCount).Should(Equal(1))
				Eventually(client.CloseCallCount).Should(Equal(1))
			})
		})
	})
})
