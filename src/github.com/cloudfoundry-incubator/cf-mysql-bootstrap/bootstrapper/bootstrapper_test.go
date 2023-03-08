package bootstrapper_test

import (
	"errors"

	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	bootstrapper    *bootstrapperPkg.Bootstrapper
	fakeNodeManager *fakes.FakeNodeManager
)

var _ = Describe("Bootstrap", func() {
	BeforeEach(func() {
		fakeNodeManager = &fakes.FakeNodeManager{}
		bootstrapper = bootstrapperPkg.New(fakeNodeManager)

		fakeNodeManager.VerifyClusterIsUnhealthyStub = func() (bool, error) {
			return true, nil
		}
	})

	Context("when all nodeManager calls succeed", func() {
		BeforeEach(func() {
			fakeNodeManager.GetSequenceNumbersReturns(map[string]int{
				"url1": 1,
				"url3": 3,
				"url2": 2,
			}, nil)
		})

		It("bootstraps the node with the highest sequence number", func() {
			actionTaken, err := bootstrapper.Bootstrap()
			Expect(err).ToNot(HaveOccurred())
			Expect(actionTaken).To(BeTrue())

			Expect(fakeNodeManager.VerifyClusterIsUnhealthyCallCount()).To(Equal(1))
			Expect(fakeNodeManager.VerifyAllNodesAreReachableCallCount()).To(Equal(1))
			Expect(fakeNodeManager.StopAllNodesCallCount()).To(Equal(1))

			Expect(fakeNodeManager.GetSequenceNumbersCallCount()).To(Equal(1))

			Expect(fakeNodeManager.BootstrapNodeCallCount()).To(Equal(1))
			Expect(fakeNodeManager.BootstrapNodeArgsForCall(0)).To(Equal("url3"))

			Expect(fakeNodeManager.JoinNodeCallCount()).To(Equal(2))
			joinNodes := []string{
				fakeNodeManager.JoinNodeArgsForCall(0),
				fakeNodeManager.JoinNodeArgsForCall(1),
			}
			Expect(joinNodes).To(ConsistOf("url1", "url2"))
		})
	})
})

var _ = Describe("rejoin-unsafe", func() {
	BeforeEach(func() {
		fakeNodeManager = &fakes.FakeNodeManager{}
		bootstrapper = bootstrapperPkg.New(fakeNodeManager)

		fakeNodeManager.VerifyClusterIsUnhealthyStub = func() (bool, error) {
			return true, nil
		}

		fakeNodeManager.FindUnhealthyNodeStub = func() (string, error) {
			return "fake-url", nil
		}
	})

	Context("when all nodeManager calls succeed", func() {
		It("makes the unhealthy node rejoin the cluster", func() {
			actionTaken, err := bootstrapper.RejoinUnsafe()
			Expect(err).ToNot(HaveOccurred())
			Expect(actionTaken).To(BeTrue())

			Expect(fakeNodeManager.VerifyClusterIsUnhealthyCallCount()).To(Equal(1))
			Expect(fakeNodeManager.FindUnhealthyNodeCallCount()).To(Equal(1))
			Expect(fakeNodeManager.StopNodeCallCount()).To(Equal(1))
			Expect(fakeNodeManager.StopNodeArgsForCall(0)).To(Equal("fake-url"))
			Expect(fakeNodeManager.JoinNodeCallCount()).To(Equal(1))
			Expect(fakeNodeManager.JoinNodeArgsForCall(0)).To(Equal("fake-url"))
		})
	})

	Context("when cluster is healthy", func() {
		It("returns false,nil", func() {
			fakeNodeManager.VerifyClusterIsUnhealthyStub = func() (bool, error) {
				return false, nil
			}
			actionTaken, err := bootstrapper.RejoinUnsafe()
			Expect(err).NotTo(HaveOccurred())
			Expect(actionTaken).To(BeFalse())
		})
	})

	Context("if FindUnhealthyNode returns an error", func() {
		It("returns an error", func() {
			fakeNodeManager.FindUnhealthyNodeStub = func() (string, error) {
				return "", errors.New("fake-error")
			}
			actionTaken, err := bootstrapper.RejoinUnsafe()
			Expect(err).To(HaveOccurred())
			Expect(actionTaken).To(BeFalse())
		})
	})

	Context("if JoinNode returns an error", func() {
		It("returns an error", func() {
			fakeNodeManager.JoinNodeStub = func(string) error {
				return errors.New("fake-error")
			}
			actionTaken, err := bootstrapper.RejoinUnsafe()
			Expect(err).To(HaveOccurred())
			Expect(actionTaken).To(BeFalse())
		})
	})

	Context("if StopNode returns an error", func() {
		It("returns an error", func() {
			fakeNodeManager.StopNodeStub = func(string) error {
				return errors.New("fake-error")
			}
			actionTaken, err := bootstrapper.RejoinUnsafe()
			Expect(err).To(HaveOccurred())
			Expect(actionTaken).To(BeFalse())
		})
	})
})
