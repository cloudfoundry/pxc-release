package galera_init_client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Start_Stop_Status(t *testing.T) {
	var postStart, postStop atomic.Int32
	monitString := atomic.Value{}
	monitString.Store("pending")
	ready := atomic.Int32{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("start: %s", r.Method)
		}
		postStart.Add(1)
		monitString.Store("initializing")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		postStop.Add(1)
		monitString.Store("stopped")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /v1/moniteq", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprint(monitString.Load())))
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() < 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"ready":false,"phase":"bootstrapping"}`))
			return
		}
		monitString.Store("running")
		_, _ = w.Write([]byte(`{"ready":true,"phase":"running"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	time.AfterFunc(50*time.Millisecond, func() { ready.Store(1) })

	c, err := NewClient(srv.URL, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start("galera-init"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if postStart.Load() != 1 {
		t.Fatalf("expected 1 POST /start, got %d", postStart.Load())
	}
	if st, err := c.Status(""); err != nil || st != "running" {
		t.Fatalf("Status after start: %q, %v", st, err)
	}
	if err := c.Stop("galera-init"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if postStop.Load() != 1 {
		t.Fatalf("expected 1 POST /stop, got %d", postStop.Load())
	}
}

func TestClient_Start_ackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"x","message":"nope"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start("s"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected nope in error, got %v", err)
	}
}

func TestClient_NewClientForAddress(t *testing.T) {
	_, err := NewClientForAddress("127.0.0.1:9", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_NewClient_validation(t *testing.T) {
	_, err := NewClient("http://x", 0)
	if err == nil {
		t.Fatal("expected err")
	}
	_, err = NewClient("no-scheme", 1*time.Second)
	if err == nil {
		t.Fatal("expected err")
	}
}
