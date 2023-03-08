package domain_test

import (
	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WsrepLocalState", func() {
	DescribeTable("Comment",
		func(state domain.WsrepLocalState, comment domain.WsrepLocalStateComment) {
			Expect(state.Comment()).To(Equal(comment))
		},
		Entry("maps joining", domain.Joining, domain.JoiningString),
		Entry("maps donor desynced", domain.DonorDesynced, domain.DonorDesyncedString),
		Entry("maps joined", domain.Joined, domain.JoinedString),
		Entry("maps synced", domain.Synced, domain.SyncedString),
		Entry("maps unknown value of 0", domain.WsrepLocalState(0), domain.WsrepLocalStateComment("Unrecognized state: 0")),
		Entry("maps unknown value greater than 4", domain.WsrepLocalState(1234), domain.WsrepLocalStateComment("Unrecognized state: 1234")),
	)
})

const (
	Healthy   = true
	Unhealthy = false
)

var _ = DescribeTable("IsHealthy",
	func(state domain.DBState, expected bool) {
		Expect(state.IsHealthy()).To(Equal(expected))
	},
	Entry("Synced / not read-only / no maintenance is healthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Healthy),
	Entry("Synced / not read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           false,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("Synced / read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Synced / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           true,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("Synced / read-only / maintenance / wsrep_local_index == MaxInt is unhealthy", domain.DBState{
		WsrepLocalIndex:    18446744073709551615,
		WsrepLocalState:    domain.Synced,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),

	Entry("DonorDesynced / not read-only / no maintenance is healthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Healthy),
	Entry("DonorDesynced / not read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           false,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("DonorDesynced / read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("DonorDesynced / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           true,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("DonorDesynced / not read-only / no maintenance / wsrep_local_index == MaxInt is unhealthy", domain.DBState{
		WsrepLocalIndex:    18446744073709551615,
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),

	Entry("Joined / not read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joined,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Joined / read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joined,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Joined / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joined,
		ReadOnly:           true,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("Joined / not read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joined,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Joining / not read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joining,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Joining / read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joining,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Joining / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joining,
		ReadOnly:           true,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("Joining / not read-only / no maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Joining,
		ReadOnly:           false,
		MaintenanceEnabled: false,
	}, Unhealthy),
	Entry("Any other state is unhealthy", domain.DBState{
		WsrepLocalState: domain.WsrepLocalState(42),
	}, Unhealthy),
)
