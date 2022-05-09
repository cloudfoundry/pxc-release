package galera_init_status_server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

type GaleraInitStatusServer struct {
	listener net.Listener
}

func NewGaleraInitStatusServer(listener net.Listener) *GaleraInitStatusServer {
	return &GaleraInitStatusServer{
		listener: listener,
	}
}

func (s GaleraInitStatusServer) Start() error {
	server := &http.Server{
		Handler:        http.HandlerFunc(s.Status),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		log.Fatal(server.Serve(s.listener))
	}()

	return nil
}

func (s GaleraInitStatusServer) Status(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "galera init done")
}
