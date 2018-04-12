package grpcacl_test

import (
	"crypto/tls"
	"fmt"
	"log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/pivotal-cf/paraphernalia/serve/grpcacl"
	"github.com/pivotal-cf/paraphernalia/serve/grpcrunner"
	"github.com/pivotal-cf/paraphernalia/test/certtest"
	"github.com/pivotal-cf/paraphernalia/test/grpctest"
)

var _ = Describe("GRPC Server", func() {
	var (
		logger      lager.Logger
		dummyServer *DummyServer

		listenAddr string
		runner     ifrit.Runner
		process    ifrit.Process

		ca *certtest.Authority
	)

	BeforeEach(func() {
		grpclog.SetLogger(log.New(GinkgoWriter, "", log.LstdFlags))

		listenAddr = fmt.Sprintf("localhost:%d", GinkgoParallelNode()+9100)
		logger = lagertest.NewTestLogger("grpc-server")
		dummyServer = &DummyServer{}

		var err error
		ca, err = certtest.BuildCA("grpcacl")
		Expect(err).NotTo(HaveOccurred())

		pool, err := ca.CertPool()
		Expect(err).NotTo(HaveOccurred())

		serverCert, err := ca.BuildSignedCertificate("server")
		Expect(err).NotTo(HaveOccurred())

		cert, err := serverCert.TLSCertificate()
		Expect(err).NotTo(HaveOccurred())

		config := &tls.Config{
			ClientAuth:   tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{cert},
			ClientCAs:    pool,
		}

		creds := grpcacl.NewTLS(config, "allowed-client")

		runner = grpcrunner.New(logger, listenAddr, func(server *grpc.Server) {
			grpctest.RegisterTestServiceServer(server, dummyServer)
		}, grpc.Creds(creds))
		process = ginkgomon.Invoke(runner)
	})

	AfterEach(func() {
		ginkgomon.Interrupt(process)
	})

	Context("when given a request that is from a server that is allowed", func() {
		It("allows the connection", func() {
			creds := credentials.NewTLS(clientConfig(ca, "allowed-client"))
			conn, err := grpc.Dial(
				listenAddr,
				grpc.WithTransportCredentials(creds),
				grpc.WithBlock(),
			)
			Expect(err).NotTo(HaveOccurred())

			client := grpctest.NewTestServiceClient(conn)
			_, err = client.SimpleCall(context.Background(), &grpctest.Empty{})
			Expect(err).NotTo(HaveOccurred())

			conn.Close()
		})
	})

	Context("when given a request that is from a server that is not allowed", func() {
		It("does not allow the connection", func() {
			creds := credentials.NewTLS(clientConfig(ca, "not-allowed-client"))
			conn, err := grpc.Dial(
				listenAddr,
				grpc.WithTransportCredentials(creds),
			)
			Expect(err).NotTo(HaveOccurred())

			client := grpctest.NewTestServiceClient(conn)
			_, err = client.SimpleCall(context.Background(), &grpctest.Empty{})
			Expect(err).To(HaveOccurred())

			conn.Close()
		})
	})
})

func clientConfig(ca *certtest.Authority, name string) *tls.Config {
	pool, err := ca.CertPool()
	Expect(err).NotTo(HaveOccurred())

	cert, err := ca.BuildSignedCertificate(name)
	Expect(err).NotTo(HaveOccurred())

	clientCert, err := cert.TLSCertificate()
	Expect(err).NotTo(HaveOccurred())

	return &tls.Config{
		ServerName:   "localhost",
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
	}
}

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
