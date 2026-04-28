package galera_init_status_server_test

import (
	"encoding/json"
	"net"
	"net/http"

	"code.cloudfoundry.org/lager/v3/lagertest"

	"github.com/cloudfoundry/galera-init/galera_init_status_server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GaleraInitStatusServer", func() {
	var (
		listener            net.Listener
		serviceStatusServer *galera_init_status_server.GaleraInitStatusServer
	)

	BeforeEach(func() {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ToNot(HaveOccurred())
		logger := lagertest.NewTestLogger("galera_init_status_server")
		h := testReadinessHandler()
		serviceStatusServer = galera_init_status_server.NewGaleraInitStatusServer(listener, h, logger)
	})

	It("start a service status server listen on the port configured", func() {
		Expect(serviceStatusServer.Start()).To(Succeed())
		resp, err := http.Get("http://" + listener.Addr().String())
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var body galera_init_status_server.ReadinessResponse
		defer resp.Body.Close()
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body.Ready).To(BeTrue())
	})
})

func testReadinessHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(galera_init_status_server.ReadinessResponse{Ready: true, Phase: "running"})
	}))
	mux.Handle("GET /status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(galera_init_status_server.ReadinessResponse{Ready: true, Phase: "running"})
	}))
	return mux
}
