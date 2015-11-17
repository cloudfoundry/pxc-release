package monit_cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client/fakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_cmd"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("MonitCmd", func() {
	var monitClient *fakes.FakeMonitClient
	var w http.ResponseWriter
	var r *http.Request

	BeforeEach(func() {
		monitClient = &fakes.FakeMonitClient{}
		w = httptest.NewRecorder()
		r, _ = http.NewRequest("POST", "http://localhost:1234", bytes.NewReader([]byte("fake")))
		monitClient.GetLoggerReturns(lagertest.NewTestLogger("mysql_cmd"))
	})

	It("Calls StopService on the monit client when a stop command is sent", func() {
		monitClient.StopServiceReturns(true, nil)
		stopCmd := monit_cmd.NewStopMysqlCmd(monitClient)

		stopCmd.ServeHTTP(w, r)
		Expect(monitClient.StopServiceCallCount()).To(Equal(1))
	})

	It("Calls StartService(join) on the monit client when a start command is sent in join mode", func() {
		monitClient.StartServiceReturns(true, nil)
		startCmd := monit_cmd.NewStartMysqlCmd(monitClient, "join")

		startCmd.ServeHTTP(w, r)
		Expect(monitClient.StartServiceCallCount()).To(Equal(1))
		Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("join"))
	})

	It("Calls StartService(bootstrap) on the monit client when a start command is sent in bootstrap mode", func() {
		monitClient.StartServiceReturns(true, nil)
		startCmd := monit_cmd.NewStartMysqlCmd(monitClient, "bootstrap")

		startCmd.ServeHTTP(w, r)
		Expect(monitClient.StartServiceCallCount()).To(Equal(1))
		Expect(monitClient.StartServiceArgsForCall(0)).To(Equal("bootstrap"))
	})

	It("Calls GetStatus on the monit client when a new GetStatusCmd is created", func() {
		monitClient.GetStatusReturns("running", nil)
		getStatusCmd := monit_cmd.NewGetStatusCmd(monitClient)

		getStatusCmd.ServeHTTP(w, r)
		Expect(monitClient.GetStatusCallCount()).To(Equal(1))
	})

})
