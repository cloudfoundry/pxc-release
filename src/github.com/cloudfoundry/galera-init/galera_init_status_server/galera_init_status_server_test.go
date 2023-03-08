package galera_init_status_server_test

import (
	"github.com/cloudfoundry/galera-init/galera_init_status_server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net"
	"net/http"
)

var _ = Describe("GaleraInitStatusServer", func() {
	var (
		serviceStatusServer *galera_init_status_server.GaleraInitStatusServer
	)

	BeforeEach(func() {
		listener, err := net.Listen("tcp", "127.0.0.1:8999")
		Expect(err).ToNot(HaveOccurred())
		serviceStatusServer = galera_init_status_server.NewGaleraInitStatusServer(listener)
	})

	It("start a service status server listen on the port configured", func() {
		Expect(serviceStatusServer.Start()).To(Succeed())
		resp, err := http.Get("http://127.0.0.1:8999")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
