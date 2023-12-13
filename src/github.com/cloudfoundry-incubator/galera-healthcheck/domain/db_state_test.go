package domain_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
)

var _ = Describe("WsrepLocalState", func() {
	DescribeTable("Comment",
		func(state domain.WsrepLocalState, comment domain.WsrepLocalStateComment) {
			Expect(state.Comment()).To(Equal(comment))
		},
		Entry("maps joining", domain.Joining, domain.WsrepLocalStateComment("Joining")),
		Entry("maps donor desynced", domain.DonorDesynced, domain.WsrepLocalStateComment("Donor/Desynced")),
		Entry("maps joined", domain.Joined, domain.WsrepLocalStateComment("Joined")),
		Entry("maps synced", domain.Synced, domain.WsrepLocalStateComment("Synced")),
		Entry("maps initialized", domain.WsrepLocalState(0), domain.WsrepLocalStateComment("Initialized")),
		Entry("maps unknown value greater than 4", domain.WsrepLocalState(1234), domain.WsrepLocalStateComment("Unrecognized state: 1234")),
	)
})

const (
	Healthy   = true
	Unhealthy = false
)

var _ = DescribeTable("IsHealthy and not available when readonly",
	func(state domain.DBState, expected bool) {
		availableWhenReadOnly := false
		Expect(state.IsHealthy(availableWhenReadOnly)).To(Equal(expected))
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
	Entry("Synced / not read-only / maintenance / wsrep_local_index == MaxInt is unhealthy", domain.DBState{
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

var _ = DescribeTable("IsHealthy and available when readonly",
	func(state domain.DBState, expected bool) {
		availableWhenReadOnly := true
		Expect(state.IsHealthy(availableWhenReadOnly)).To(Equal(expected))
	},
	Entry("Synced / read-only / no maintenance is healthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Healthy),
	Entry("Synced / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.Synced,
		ReadOnly:           true,
		MaintenanceEnabled: true,
	}, Unhealthy),
	Entry("DonorDesynced / read-only / no maintenance is healthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           true,
		MaintenanceEnabled: false,
	}, Healthy),
	Entry("DonorDesynced / read-only / maintenance is unhealthy", domain.DBState{
		WsrepLocalState:    domain.DonorDesynced,
		ReadOnly:           true,
		MaintenanceEnabled: true,
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
	Entry("Any other state is unhealthy", domain.DBState{
		WsrepLocalState: domain.WsrepLocalState(42),
	}, Unhealthy),
)
