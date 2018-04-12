package grpcrunner_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/pivotal-cf/paraphernalia/serve/grpcrunner"
	"github.com/pivotal-cf/paraphernalia/test/grpctest"
)

var _ = Describe("GRPC Server", func() {
	var (
		logger      lager.Logger
		dummyServer *DummyServer

		listenAddr string
		runner     ifrit.Runner
		process    ifrit.Process
	)

	BeforeEach(func() {
		listenAddr = fmt.Sprintf("localhost:%d", GinkgoParallelNode()+9000)
		dummyServer = &DummyServer{}

		logger = lagertest.NewTestLogger("grpc-server")
		runner = grpcrunner.New(logger, listenAddr, func(server *grpc.Server) {
			grpctest.RegisterTestServiceServer(server, dummyServer)
		})
		process = ginkgomon.Invoke(runner)
	})

	AfterEach(func() {
		ginkgomon.Interrupt(process)
	})

	It("exits when signaled", func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
	})

	Context("when given a request", func() {
		var (
			conn   *grpc.ClientConn
			client grpctest.TestServiceClient
		)

		BeforeEach(func() {
			var err error
			conn, err = grpc.Dial(
				listenAddr,
				grpc.WithInsecure(),
				grpc.WithBlock(),
			)
			Expect(err).NotTo(HaveOccurred())

			client = grpctest.NewTestServiceClient(conn)
		})

		AfterEach(func() {
			conn.Close()
		})

		It("is a real GRPC server", func() {
			_, err := client.SimpleCall(context.Background(), &grpctest.Empty{})
			Expect(err).NotTo(HaveOccurred())

			Expect(dummyServer.CallCount()).To(Equal(1))
		})
	})
})

type DummyServer struct {
	callCount int
}

func (d *DummyServer) CallCount() int {
	return d.callCount
}

func (d *DummyServer) SimpleCall(ctx context.Context, e *grpctest.Empty) (*grpctest.Empty, error) {
	d.callCount++

	return e, nil
}
