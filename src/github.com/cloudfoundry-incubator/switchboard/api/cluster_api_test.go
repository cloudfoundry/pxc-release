package api_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/cloudfoundry-incubator/switchboard/api"
	"github.com/cloudfoundry-incubator/switchboard/domain"
)

var _ = Describe("ClusterAPI", func() {
	var (
		logger              lager.Logger
		cluster             *api.ClusterAPI
		trafficEnabledChan1 chan bool
		trafficEnabledChan2 chan bool
	)

	BeforeEach(func() {
		trafficEnabledChan1 = make(chan bool, 10)
		trafficEnabledChan2 = make(chan bool, 10)
	})

	JustBeforeEach(func() {
		logger = lagertest.NewTestLogger("Cluster test")
		cluster = api.NewClusterAPI(
			logger,
		)
		cluster.RegisterTrafficEnabledChan(trafficEnabledChan1)
		cluster.RegisterTrafficEnabledChan(trafficEnabledChan2)
	})

	Describe("Active Backends", func() {
		Context("when there is not yet an active backend", func() {
			It("returns nil", func() {
				clusterJSON := cluster.AsJSON()
				Expect(clusterJSON.ActiveBackend).To(BeNil())
			})
		})

		Context("when there is an active backend", func() {
			It("returns the backend", func() {
				go cluster.ListenForActiveBackend()
				cluster.ActiveBackendChan <- domain.NewBackend(
					"backend-0",
					"192.0.2.10",
					3306,
					9292,
					"",
					logger,
				)

				Eventually(func() *api.BackendJSON {
					return cluster.AsJSON().ActiveBackend
				}).Should(Equal(
					&api.BackendJSON{
						Host: "192.0.2.10",
						Port: 3306,
						Name: "backend-0",
					},
				))
			})
		})

		Context("when there are no available active backends", func() {
			It("returns nil", func() {
				go cluster.ListenForActiveBackend()
				cluster.ActiveBackendChan <- nil
				Expect(cluster.AsJSON().ActiveBackend).To(BeNil())
			})
		})
	})

	Describe("EnableTraffic", func() {
		var (
			message string
		)

		BeforeEach(func() {
			message = "some message"
		})

		It("records the message", func() {
			cluster.EnableTraffic(message)

			clusterJSON := cluster.AsJSON()

			Expect(clusterJSON.Message).To(Equal(message))
		})

		It("records the current time", func() {
			beforeTime := time.Now()
			cluster.EnableTraffic(message)
			afterTime := time.Now()

			clusterJSON := cluster.AsJSON()

			Expect(clusterJSON.LastUpdated.After(beforeTime)).To(BeTrue())
			Expect(clusterJSON.LastUpdated.Before(afterTime)).To(BeTrue())
		})

		It("records that traffic is enabled", func() {
			cluster.EnableTraffic(message)

			clusterJSON := cluster.AsJSON()

			Expect(clusterJSON.TrafficEnabled).To(BeTrue())
		})

		It("publishes that traffic is enabled", func() {
			cluster.EnableTraffic(message)

			Eventually(trafficEnabledChan1).Should(Receive(BeTrue()))
			Eventually(trafficEnabledChan2).Should(Receive(BeTrue()))
		})
	})

	Describe("DisableTraffic", func() {
		var (
			message string
		)

		BeforeEach(func() {
			message = "some message"
		})

		It("records the message", func() {
			cluster.DisableTraffic(message)

			clusterJSON := cluster.AsJSON()

			Expect(clusterJSON.Message).To(Equal(message))
		})

		It("records the current time", func() {
			beforeTime := time.Now()
			cluster.DisableTraffic(message)
			afterTime := time.Now()

			clusterJSON := cluster.AsJSON()

			Expect(clusterJSON.LastUpdated.After(beforeTime)).To(BeTrue())
			Expect(clusterJSON.LastUpdated.Before(afterTime)).To(BeTrue())
		})
	})
})
