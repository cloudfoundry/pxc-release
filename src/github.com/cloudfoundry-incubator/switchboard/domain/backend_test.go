package domain_test

import (
	"net"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/domain/domainfakes"
)

var _ = Describe("Backend", func() {
	var backend *domain.Backend
	var bridges *domainfakes.FakeBridges

	BeforeEach(func() {
		bridges = new(domainfakes.FakeBridges)

		domain.BridgesProvider = func(lager.Logger) domain.Bridges {
			return bridges
		}

		logger := lagertest.NewTestLogger("Backend test")
		backend = domain.NewBackend("backend-0", "1.2.3.4", 3306, 9902, "status", logger)
	})

	AfterEach(func() {
		domain.BridgesProvider = domain.NewBridges
	})

	Describe("HealthcheckUrls", func() {
		When("using TLS for agent communication", func() {
			It("returns a pair of TLS/non-TLS URLs", func() {
				healthcheckURLs := backend.HealthcheckUrls(true)
				Expect(len(healthcheckURLs)).To(Equal(2))

				By("constructing a TLS URL0 with the correct protocol, backend host and health check port", func() {
					healthcheckURLs := backend.HealthcheckUrls(true)
					Expect(healthcheckURLs[0]).To(Equal("https://1.2.3.4:9902/status"))
				})
				By("constructing a non-TLS URL1 with has the correct protocol, backend host and health check port", func() {
					Expect(healthcheckURLs[1]).To(Equal("http://1.2.3.4:9200/status"))
				})
			})
		})
		When("NOT using TLS for agent communication", func() {
			It("returns a one-URL slice of the correct protocol, backend host and health check port", func() {
				healthcheckURLs := backend.HealthcheckUrls(false)
				Expect(len(healthcheckURLs)).To(Equal(1))
				Expect(healthcheckURLs[0]).To(Equal("http://1.2.3.4:9902/status"))
			})
		})
	})

	Describe("SeverConnections", func() {
		It("removes and closes all bridges", func() {
			backend.SeverConnections()
			Expect(bridges.RemoveAndCloseAllCallCount()).To(Equal(1))
		})
	})

	Describe("Bridge", func() {
		var backendConn *domainfakes.FakeConn
		var clientConn *domainfakes.FakeConn

		var dialErr error
		var dialedProtocol, dialedAddress string
		var bridge *domainfakes.FakeBridge
		var connectReadyChan, disconnectChan chan interface{}

		BeforeEach(func() {
			bridge = new(domainfakes.FakeBridge)

			connectReadyChan = make(chan interface{})
			disconnectChan = make(chan interface{})

			bridge.ConnectStub = func(connectReadyChan, disconnectChan chan interface{}) func() {
				return func() {
					close(connectReadyChan)
					<-disconnectChan
				}
			}(connectReadyChan, disconnectChan)

			bridges.CreateReturns(bridge)

			clientConn = new(domainfakes.FakeConn)
			backendConn = new(domainfakes.FakeConn)

			dialErr = nil
			dialedAddress = ""

			domain.Dialer = func(protocol, address string) (net.Conn, error) {
				dialedProtocol = protocol
				dialedAddress = address
				return backendConn, dialErr
			}
		})

		AfterEach(func() {
			domain.Dialer = net.Dial
		})

		It("dials the backend address", func(done Done) {
			defer close(done)
			defer close(disconnectChan)

			go func() {
				err := backend.Bridge(clientConn)
				Expect(err).NotTo(HaveOccurred())
			}()

			<-connectReadyChan

			Eventually(dialedProtocol).Should(Equal("tcp"))
			Eventually(dialedAddress).Should(Equal("1.2.3.4:3306"))
		}, 5)

		It("asynchronously creates and connects to a bridge", func(done Done) {
			defer close(done)
			defer close(disconnectChan)

			go func() {
				err := backend.Bridge(clientConn)
				Expect(err).NotTo(HaveOccurred())
			}()

			<-connectReadyChan

			Expect(bridges.CreateCallCount()).Should(Equal(1))
			actualClientConn, actualBackendConn := bridges.CreateArgsForCall(0)
			Expect(actualClientConn).To(Equal(clientConn))
			Expect(actualBackendConn).To(Equal(backendConn))

			Expect(bridge.ConnectCallCount()).To(Equal(1))
		}, 5)

		Context("when the bridge is disconnected", func() {
			It("removes the bridge", func(done Done) {
				defer close(done)

				go func() {
					err := backend.Bridge(clientConn)
					Expect(err).NotTo(HaveOccurred())
				}()

				<-connectReadyChan

				Consistently(bridges.RemoveCallCount).Should(Equal(0))

				close(disconnectChan)

				Eventually(bridges.RemoveCallCount).Should(Equal(1))
				Expect(bridges.RemoveArgsForCall(0)).To(Equal(bridge))
			}, 5)
		})
	})
})
