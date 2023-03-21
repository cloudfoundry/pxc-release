package domain_test

import (
	"net"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/domain/domainfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bridges", func() {
	var bridges domain.Bridges
	var bridge1 domain.Bridge
	var bridge2 domain.Bridge
	var bridge3 domain.Bridge

	BeforeEach(func() {
		logger := lagertest.NewTestLogger("Bridges Test")
		bridges = domain.NewBridges(logger)
	})

	JustBeforeEach(func() {
		bridge1 = bridges.Create(nil, nil)
		bridge2 = bridges.Create(nil, nil)
		bridge3 = bridges.Create(nil, nil)
	})

	Describe("Concurrent operations", func() {
		It("do not result in a race", func() {
			readySetGo := make(chan interface{})

			doneChans := []chan interface{}{
				make(chan interface{}),
				make(chan interface{}),
				make(chan interface{}),
				make(chan interface{}),
				make(chan interface{}),
			}

			go func() {
				<-readySetGo
				bridges.Create(nil, nil)
				close(doneChans[0])
			}()

			go func() {
				<-readySetGo
				bridges.Contains(bridge1)
				close(doneChans[1])
			}()

			go func() {
				<-readySetGo
				bridges.Remove(bridge2)
				close(doneChans[2])
			}()

			go func() {
				<-readySetGo
				bridges.Size()
				close(doneChans[3])
			}()

			go func() {
				<-readySetGo
				bridges.RemoveAndCloseAll()
				close(doneChans[4])
			}()

			close(readySetGo)

			for _, done := range doneChans {
				<-done
			}
		})
	})

	Describe("Remove", func() {
		It("removes only the given bridge", func() {
			err := bridges.Remove(bridge2)
			Expect(err).NotTo(HaveOccurred())

			Expect(bridges.Contains(bridge1)).To(BeTrue())
			Expect(bridges.Contains(bridge2)).To(BeFalse())
			Expect(bridges.Contains(bridge3)).To(BeTrue())

			Expect(bridges.Size()).To(BeNumerically("==", 2))
		})

		Context("when the bridge cannot be found", func() {
			It("returns an error", func() {
				err := bridges.Remove(domain.NewBridge(new(domainfakes.FakeConn), new(domainfakes.FakeConn), nil))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("Bridge not found"))
			})
		})
	})

	Describe("RemoveAndCloseAll", func() {
		BeforeEach(func() {
			domain.BridgeProvider = func(_, _ net.Conn, logger lager.Logger) domain.Bridge {
				return new(domainfakes.FakeBridge)
			}
		})

		AfterEach(func() {
			domain.BridgeProvider = domain.NewBridge
		})

		It("closes all bridges", func() {
			bridges.RemoveAndCloseAll()

			Expect(bridge1.(*domainfakes.FakeBridge).CloseCallCount()).To(Equal(1))
			Expect(bridge2.(*domainfakes.FakeBridge).CloseCallCount()).To(Equal(1))
			Expect(bridge3.(*domainfakes.FakeBridge).CloseCallCount()).To(Equal(1))
		})

		It("removes all bridges", func() {
			bridges.RemoveAndCloseAll()

			Expect(bridges.Size()).To(BeNumerically("==", 0))
		})
	})
})
