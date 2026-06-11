package bridge

import (
	"errors"
	"net"
	"strings"
	"sync"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("blockingAccept", func() {
	It("logs and retries on a non-shutdown accept error", func() {
		logger := lagertest.NewTestLogger("test")
		r := Runner{logger: logger}
		c := make(chan net.Conn)
		shutdown := make(chan struct{})
		done := make(chan struct{})

		fl := newFakeListener([]error{errors.New("simulated accept error")})
		go func() {
			r.blockingAccept(fl, c, shutdown)
			close(done)
		}()

		// The error should be logged; blockingAccept must not exit on its own.
		Eventually(func() bool {
			for _, entry := range logger.Logs() {
				if strings.Contains(entry.Message, "Error accepting client connection") {
					return true
				}
			}
			return false
		}, "500ms", "10ms").Should(BeTrue())

		// Verify blockingAccept is still running (it continued, not returned).
		Consistently(done, "20ms").ShouldNot(BeClosed())

		close(shutdown)
		fl.Close()
		Eventually(done, "200ms").Should(BeClosed())
	})

	It("exits cleanly when shutdown fires while a connection is pending send", func() {
		logger := lagertest.NewTestLogger("test")
		r := Runner{logger: logger}
		c := make(chan net.Conn) // no reader: send on c will block
		shutdown := make(chan struct{})
		done := make(chan struct{})

		// Arm shutdown before the goroutine starts. Accept() returns a conn
		// immediately, but c has no reader so only the <-shutdown case can proceed.
		fl := newFakeListener([]error{nil})
		close(shutdown)

		go func() {
			r.blockingAccept(fl, c, shutdown)
			close(done)
		}()

		Eventually(done, "200ms").Should(BeClosed())
	})
})

// fakeListener is a net.Listener backed by a canned response list.
// nil entries produce a real in-memory net.Pipe connection; non-nil entries
// are returned as errors. Once the list is exhausted Accept blocks until
// Close is called.
type fakeListener struct {
	mu        sync.Mutex
	calls     int
	responses []error
	done      chan struct{}
}

func newFakeListener(responses []error) *fakeListener {
	return &fakeListener{responses: responses, done: make(chan struct{})}
}

func (f *fakeListener) Accept() (net.Conn, error) {
	f.mu.Lock()
	i := f.calls
	f.calls++
	f.mu.Unlock()

	if i < len(f.responses) {
		if f.responses[i] == nil {
			c, s := net.Pipe()
			_ = s.Close()
			return c, nil
		}
		return nil, f.responses[i]
	}
	<-f.done
	return nil, errors.New("listener closed")
}

func (f *fakeListener) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func (f *fakeListener) Addr() net.Addr { return &net.TCPAddr{} }
