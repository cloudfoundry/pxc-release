package main_test

import (
	"crypto/tls"
	"fmt"
	"gopkg.in/yaml.v3"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/test_helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ConcurrencyTracker tracks concurrent operations for testing mutex serialization
type ConcurrencyTracker struct {
	mu             sync.Mutex
	currentOps     map[string]time.Time
	maxConcurrent  int
	totalOps       int
	completedOps   int
	operationOrder []string
}

func NewConcurrencyTracker() *ConcurrencyTracker {
	return &ConcurrencyTracker{
		currentOps: make(map[string]time.Time),
	}
}

func (ct *ConcurrencyTracker) StartOperation(opType string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	opID := fmt.Sprintf("%s_%d", opType, ct.totalOps)
	ct.currentOps[opID] = time.Now()
	ct.totalOps++

	if len(ct.currentOps) > ct.maxConcurrent {
		ct.maxConcurrent = len(ct.currentOps)
	}

	ct.operationOrder = append(ct.operationOrder, fmt.Sprintf("START_%s", opID))
}

func (ct *ConcurrencyTracker) EndOperation(opType string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Find the matching start operation
	for opID := range ct.currentOps {
		if strings.HasPrefix(opID, opType) {
			delete(ct.currentOps, opID)
			ct.completedOps++
			ct.operationOrder = append(ct.operationOrder, fmt.Sprintf("END_%s", opID))
			break
		}
	}
}

func (ct *ConcurrencyTracker) MaxConcurrent() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.maxConcurrent
}

func (ct *ConcurrencyTracker) AllOperationsCompleted() bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.completedOps == ct.totalOps && len(ct.currentOps) == 0
}

func (ct *ConcurrencyTracker) OperationOrder() []string {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return append([]string(nil), ct.operationOrder...) // Return copy
}

func (ct *ConcurrencyTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.currentOps = make(map[string]time.Time)
	ct.maxConcurrent = 0
	ct.totalOps = 0
	ct.completedOps = 0
	ct.operationOrder = []string{}
}

var _ = Describe("Galera Agent Concurrency", func() {
	var (
		serverAuthority  *certtest.Authority
		tlsClientConfig  *tls.Config
		galeraAgentPort  int
		monitServer      *ghttp.Server
		callbackServer   *ghttp.Server
		stubMysqldScript string
		operationTracker *ConcurrencyTracker
		galeraInitServer *ghttp.Server
	)

	makeAuthenticatedRequest := func(method, path, body string) (*http.Response, error) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
			Timeout: 30 * time.Second,
		}

		url := fmt.Sprintf("https://127.0.0.1:%d%s", galeraAgentPort, path)
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			return nil, err
		}

		req.SetBasicAuth("basic-auth-username", "basic-auth-password")

		return client.Do(req)
	}

	BeforeEach(func() {
		var err error
		serverAuthority, err = certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := serverAuthority.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())

		serverCertPEM, serverKeyPEM, err := serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		// Setup TLS client config
		serverCertPool, err := serverAuthority.CertPool()
		Expect(err).ToNot(HaveOccurred())

		tlsClientConfig, err = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
		).Client(
			tlsconfig.WithAuthority(serverCertPool),
		)
		Expect(err).ToNot(HaveOccurred())

		// Initialize operation tracker
		operationTracker = NewConcurrencyTracker()

		// Setup callback server for mysqld stub to report to
		callbackServer = ghttp.NewServer()
		DeferCleanup(func() {
			callbackServer.Close()
		})
		setupCallbackHandlers(callbackServer, operationTracker)

		// Setup galera-init status server (for waitForGaleraInit)
		galeraInitServer = ghttp.NewServer()
		DeferCleanup(func() {
			galeraInitServer.Close()
		})

		galeraInitServer.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				ghttp.RespondWith(http.StatusOK, "galera-init ready"),
			),
		)

		// Create stub mysqld script that calls back to our test server
		stubMysqldScript = createMysqldStub(callbackServer.URL())
		DeferCleanup(func() {
			Expect(os.Remove(stubMysqldScript)).To(Succeed())
		})

		// Setup monit server with tracking
		monitServer = ghttp.NewServer()
		DeferCleanup(func() {
			monitServer.Close()
		})
		setupMonitHandlers(monitServer, operationTracker)

		galeraAgentPort = test_helpers.RandomPort()
		cfg := config.Config{
			DB: config.DBConfig{
				Password: "root-password",
			},
			Monit: config.MonitConfig{
				Host:                          extractHost(monitServer.URL()),
				User:                          "monit-user",
				Port:                          extractPort(monitServer.URL()),
				Password:                      "monit-password",
				MysqlStateFilePath:            "/tmp/mysql-state",
				ServiceName:                   "galera-init",
				GaleraInitStatusServerAddress: extractHostPort(galeraInitServer.URL()),
			},
			Host:       "localhost",
			Port:       galeraAgentPort,
			MysqldPath: stubMysqldScript,
			MyCnfPath:  "/tmp/my.cnf",
			SidecarEndpoint: config.SidecarEndpointConfig{
				Username: "basic-auth-username",
				Password: "basic-auth-password",
				TLS: config.EndpointTLS{
					Enabled:     true,
					Certificate: string(serverCertPEM),
					PrivateKey:  string(serverKeyPEM),
				},
			},
		}

		b, err := yaml.Marshal(&cfg)
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command(
			binaryPath,
			fmt.Sprintf("-config=%s", string(b)),
		)

		_, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			gexec.KillAndWait(time.Minute)
		})

		// Wait for the server to be ready
		Eventually(func() error {
			_, err := makeAuthenticatedRequest("GET", "/health", "")
			return err
		}, "10s", "500ms").Should(Succeed())
	})

	Context("API Operation Serialization", func() {
		BeforeEach(func() {
			operationTracker.Reset()
		})

		It("serializes all mutex-protected operations", func() {

			// Launch all types of operations concurrently
			operations := []func(){
				func() { _, _ = makeAuthenticatedRequest("POST", "/start_mysql_bootstrap", "") },
				func() { _, _ = makeAuthenticatedRequest("POST", "/start_mysql_join", "") },
				func() { _, _ = makeAuthenticatedRequest("POST", "/stop_mysql", "") },
				func() { _, _ = makeAuthenticatedRequest("GET", "/sequence_number", "") },
				func() { _, _ = makeAuthenticatedRequest("GET", "/sequence_number", "") },
			}

			var wg sync.WaitGroup
			// Stagger the requests slightly to ensure they overlap
			for i, op := range operations {
				i := i
				operation := op
				wg.Go(func() {
					delay := time.Duration(i*50) * time.Millisecond
					time.Sleep(delay)
					operation()
				})
			}
			wg.Wait()

			// Verify serialization
			Eventually(func() bool {
				return operationTracker.AllOperationsCompleted()
			}, "15s", "100ms").Should(BeTrue())

			Expect(operationTracker.MaxConcurrent()).To(Equal(1),
				"Expected max concurrent operations to be 1, but was %d", operationTracker.MaxConcurrent())
		})

		It("maintains operation ordering under load", func() {
			var wg sync.WaitGroup

			// Create a burst of mixed operations
			for i := 0; i < 3; i++ {
				wg.Go(func() {
					_, _ = makeAuthenticatedRequest("POST", "/start_mysql_bootstrap", "")
				})

				wg.Go(func() {
					time.Sleep(10 * time.Millisecond)
					_, _ = makeAuthenticatedRequest("GET", "/sequence_number", "")
				})

				wg.Go(func() {
					time.Sleep(20 * time.Millisecond)
					_, _ = makeAuthenticatedRequest("POST", "/stop_mysql", "")
				})

				wg.Go(func() {
					time.Sleep(30 * time.Millisecond)
					_, _ = makeAuthenticatedRequest("GET", "/sequence_number", "")
				})
			}
			wg.Wait()

			// Wait for all operations to complete
			Eventually(func() bool {
				return operationTracker.AllOperationsCompleted()
			}, "20s", "100ms").Should(BeTrue())

			// Verify no interleaving occurred
			order := operationTracker.OperationOrder()

			// Each operation should have matching START/END pairs without interleaving
			var stack []string
			for _, event := range order {
				if strings.HasPrefix(event, "START_") {
					stack = append(stack, event)
				} else if strings.HasPrefix(event, "END_") {
					// Should match the most recent start
					Expect(stack).NotTo(BeEmpty(), "END without matching START: %s", event)
					lastStart := stack[len(stack)-1]
					expectedEnd := strings.Replace(lastStart, "START_", "END_", 1)
					Expect(event).To(Equal(expectedEnd), "Operations interleaved: expected %s, got %s", expectedEnd, event)
					stack = stack[:len(stack)-1] // Pop
				}
			}

			Expect(stack).To(BeEmpty(), "Unmatched START operations: %v", stack)
			Expect(operationTracker.MaxConcurrent()).To(Equal(1))
		})

		It("verifies mutex is released on operation completion", func() {
			// This test verifies that operations complete in sequence and don't deadlock
			var wg sync.WaitGroup
			results := make(chan int, 2)

			// Launch two operations sequentially to verify mutex release
			wg.Go(func() {
				resp, err := makeAuthenticatedRequest("POST", "/start_mysql_bootstrap", "")
				if err != nil {
					results <- 0
					return
				}
				defer func() { _ = resp.Body.Close() }()

				results <- resp.StatusCode
			})

			// Small delay to ensure first operation starts
			time.Sleep(100 * time.Millisecond)

			wg.Go(func() {
				resp, err := makeAuthenticatedRequest("POST", "/start_mysql_join", "")
				if err != nil {
					results <- 0
					return
				}
				defer func() { _ = resp.Body.Close() }()

				results <- resp.StatusCode
			})
			wg.Wait()
			close(results)

			// Both operations should complete
			var statusCodes []int
			for code := range results {
				statusCodes = append(statusCodes, code)
			}

			Expect(len(statusCodes)).To(Equal(2))
			Eventually(func() bool {
				return operationTracker.AllOperationsCompleted()
			}, "10s", "100ms").Should(BeTrue())
			Expect(operationTracker.MaxConcurrent()).To(Equal(1))
		})
	})
})

// Helper functions

func setupCallbackHandlers(server *ghttp.Server, tracker *ConcurrencyTracker) {
	// Use RouteToHandler for more flexible handling of multiple requests
	server.RouteToHandler("POST", "/wsrep_recover_start", func(w http.ResponseWriter, r *http.Request) {
		tracker.StartOperation("sequence_number")
		w.WriteHeader(http.StatusOK)
	})

	server.RouteToHandler("POST", "/wsrep_recover_end", func(w http.ResponseWriter, r *http.Request) {
		tracker.EndOperation("sequence_number")
		w.WriteHeader(http.StatusOK)
	})
}

func setupMonitHandlers(server *ghttp.Server, tracker *ConcurrencyTracker) {
	// Create a more flexible handler that can handle multiple requests
	server.RouteToHandler("POST", "/galera-init", func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		err := r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		action := r.FormValue("action")
		switch action {
		case "start":
			tracker.StartOperation("start")
			time.Sleep(1 * time.Second) // Simulate work
			tracker.EndOperation("start")
			w.WriteHeader(http.StatusOK)
		case "stop":
			tracker.StartOperation("stop")
			time.Sleep(500 * time.Millisecond)
			tracker.EndOperation("stop")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	// Status handler for waitForGaleraInit
	server.RouteToHandler("GET", "/_status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<monit>
  <service name="galera-init">
    <status>0</status>
    <monitor>1</monitor>
    <pendingaction>0</pendingaction>
  </service>
</monit>`))
	})
}

func createMysqldStub(callbackURL string) string {
	scriptContent := fmt.Sprintf(`#!/bin/bash
# Stub mysqld script for testing concurrency

# Parse arguments to determine operation type
OPERATION="unknown"
for arg in "$@"; do
    case "$arg" in
        --wsrep-recover)
            OPERATION="wsrep_recover"
            ;;
    esac
done

# Report operation start
curl -s -X POST "%s/${OPERATION}_start" || true

# Simulate work with delay
sleep 2

# Output expected mysqld recovery result to stderr
if [[ "$OPERATION" == "wsrep_recover" ]]; then
    echo "WSREP: Recovered position: test-uuid:42" >&2
fi

# Report operation end
curl -s -X POST "%s/${OPERATION}_end" || true

exit 0
`, callbackURL, callbackURL)

	scriptFile, err := os.CreateTemp("", "mysqld-stub-*.sh")
	Expect(err).NotTo(HaveOccurred())

	Expect(scriptFile.WriteString(scriptContent)).Error().NotTo(HaveOccurred())
	Expect(scriptFile.Close()).To(Succeed())
	Expect(os.Chmod(scriptFile.Name(), 0755)).To(Succeed())

	return scriptFile.Name()
}

func extractHost(serverURL string) string {
	u, err := url.Parse(serverURL)
	Expect(err).NotTo(HaveOccurred())
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		// If no port, return the host as-is
		return u.Host
	}
	return host
}

func extractPort(serverURL string) string {
	u, err := url.Parse(serverURL)
	Expect(err).NotTo(HaveOccurred())
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		// Default HTTP port if no port specified
		return "80"
	}
	return port
}

func extractHostPort(serverURL string) string {
	u, err := url.Parse(serverURL)
	Expect(err).NotTo(HaveOccurred())
	return u.Host
}
