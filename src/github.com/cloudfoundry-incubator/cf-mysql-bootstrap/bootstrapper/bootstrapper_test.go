package bootstrapper_test

import (
	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager/fakes"
	. "github.com/onsi/ginkgo"
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
			err := bootstrapper.Bootstrap()
			Expect(err).ToNot(HaveOccurred())

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
