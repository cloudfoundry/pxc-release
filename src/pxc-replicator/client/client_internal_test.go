package client

import (
	"sync"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ReplClient internals", func() {
	Describe("Close", func() {
		It("does not panic on a nil cache", func() {
			r := &ReplClient{}
			r.Close()
		})

		It("does not panic on double close", func() {
			r := &ReplClient{}
			r.Close()
			r.Close()
		})
	})

	Describe("ConnectSource", func() {
		It("returns an error with an unreachable host", func() {
			r := &ReplClient{
				Source: config.Target{
					Name: "test",
					Host: "0.0.0.0",
					Port: 1,
					Creds: config.Creds{Username: "u", Password: "p"},
				},
			}
			_, err := r.ConnectSource()
			Expect(err).To(MatchError(ContainSubstring("failed pinging target: test after connecting")))
		})
	})

	Describe("ConnectTarget", func() {
		It("returns an error with an unreachable host", func() {
			r := &ReplClient{
				Target: config.Target{
					Name: "test",
					Host: "0.0.0.0",
					Port: 1,
					Creds: config.Creds{AdminUsername: "u", AdminPassword: "p"},
				},
			}
			_, err := r.ConnectTarget()
			Expect(err).To(MatchError(ContainSubstring("failed pinging target: test after connecting")))
		})
	})

	Describe("ConnectSourceDBUncached", func() {
		It("returns an error with an unreachable host", func() {
			r := &ReplClient{
				Source: config.Target{
					Name: "test",
					Host: "0.0.0.0",
					Port: 1,
					Creds: config.Creds{Username: "u", Password: "p"},
				},
			}
			_, err := r.ConnectSourceDBUncached("mydb")
			Expect(err).To(MatchError(ContainSubstring("failed pinging target: test after connecting")))
		})
	})

	Describe("ConnectTargetDBUncached", func() {
		It("returns an error with an unreachable host", func() {
			r := &ReplClient{
				Target: config.Target{
					Name: "test",
					Host: "0.0.0.0",
					Port: 1,
					Creds: config.Creds{AdminUsername: "u", AdminPassword: "p"},
				},
			}
			_, err := r.ConnectTargetDBUncached("mydb")
			Expect(err).To(MatchError(ContainSubstring("failed pinging target: test after connecting")))
		})
	})

	Describe("registerTLSConfig", func() {
		It("returns an error with an empty CA", func() {
			err := registerTLSConfig("test", config.Certs{CA: ""})
			Expect(err).To(MatchError(ContainSubstring("failed to append ca cert to pool")))
		})

		It("returns an error with invalid CA PEM", func() {
			err := registerTLSConfig("test", config.Certs{CA: "not-pem-data"})
			Expect(err).To(MatchError(ContainSubstring("failed to append ca cert to pool")))
		})
	})

	Describe("ConnectSource with TLS", func() {
		It("returns a TLS registration error when CA is invalid", func() {
			r := &ReplClient{
				Source: config.Target{
					Name: "test",
					Host: "1.2.3.4",
					Port: 3306,
					Creds: config.Creds{Username: "u", Password: "p"},
					Certs: config.Certs{CA: "invalid"},
				},
			}
			_, err := r.ConnectSource()
			Expect(err).To(MatchError(ContainSubstring("failed creating tls config for connection: failed to append ca cert to pool")))
		})
	})

	Describe("concurrent cache access", func() {
		It("does not panic under concurrent load", func() {
			r := &ReplClient{
				Source: config.Target{
					Name: "test",
					Host: "0.0.0.0",
					Port: 1,
					Creds: config.Creds{Username: "u", Password: "p"},
				},
			}
			var wg sync.WaitGroup
			errs := make(chan error, 10)
			for range 10 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := r.ConnectSource()
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				Expect(err).To(MatchError(ContainSubstring("failed pinging target: test after connecting")))
			}
		})
	})
})
