package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/domain/domainfakes"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var _ = Describe("Emitter", func() {
	Describe("Handler", func() {
		var emitter *Emitter

		BeforeEach(func() {
			bridges := new(domainfakes.FakeBridges)
			bridges.SizeReturnsOnCall(0, 19)
			bridges.SizeReturnsOnCall(1, 11)
			bridges.SizeReturnsOnCall(2, 216)
			domain.BridgesProvider = func(lager.Logger) domain.Bridges {
				return bridges
			}

			logger := lagertest.NewTestLogger("Backend test")
			backend0 := domain.NewBackend("backend-0", "1.2.3.4", 3306, 9902, "status", logger)
			backend1 := domain.NewBackend("backend-1", "1.2.3.4", 3306, 9902, "status", logger)
			backend2 := domain.NewBackend("backend-2", "1.2.3.4", 3306, 9902, "status", logger)

			emitter = New([]*domain.Backend{backend0, backend1, backend2})
		})

		AfterEach(func() {
			domain.BridgesProvider = domain.NewBridges
		})

		It("Responds with backend connection metrics", func() {
			handler := emitter.Handler()

			responseRecorder := httptest.NewRecorder()
			request, _ := http.NewRequest("GET", "", nil)
			handler.ServeHTTP(responseRecorder, request)

			Expect(responseRecorder.Result().StatusCode).To(Equal(200))

			defer responseRecorder.Result().Body.Close()
			bodyBytes, err := io.ReadAll(responseRecorder.Result().Body)
			Expect(err).NotTo(HaveOccurred())

			body := strings.Split(string(bodyBytes), "\n")
			Expect(body).To(ContainElement("# HELP backend_sessions_total Gauge of the current sessions from this proxy to a mysql backend"))
			Expect(body).To(ContainElement("# TYPE backend_sessions_total gauge"))
			Expect(body).To(ContainElement(`backend_sessions_total{backend="backend-0"} 19`))
			Expect(body).To(ContainElement(`backend_sessions_total{backend="backend-1"} 11`))
			Expect(body).To(ContainElement(`backend_sessions_total{backend="backend-2"} 216`))
		})
	})
})
