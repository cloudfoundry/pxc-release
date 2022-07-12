package apiaggregator_test

import (
	"fmt"
	"net"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/switchboard/runner/apiaggregator"
)

var _ = Describe("APIRunner", func() {
	It("shuts down gracefully when signalled", func() {
		apiPort := 20000 + GinkgoParallelNode()
		apiRunner := apiaggregator.NewRunner("127.0.0.1:"+strconv.Itoa(apiPort), nil)
		apiProcess := ifrit.Invoke(apiRunner)
		apiProcess.Signal(os.Kill)
		Eventually(apiProcess.Wait()).Should(Receive())

		_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", apiPort))
		Expect(err).To(HaveOccurred())
	})
})
