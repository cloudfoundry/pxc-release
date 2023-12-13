package domain

import (
	"fmt"
	"math"
)

type WsrepLocalState uint
type WsrepLocalStateComment string

// https://docs.percona.com/percona-xtradb-cluster/8.0/wsrep-status-index.html#wsrep_local_state
const (
	Initialized WsrepLocalState = iota
	Joining                     // https://splice.com/blog/iota-elegant-constants-golang/
	DonorDesynced
	Joined
	Synced
)

type DBState struct {
	WsrepLocalIndex    uint            `json:"wsrep_local_index"`
	WsrepLocalState    WsrepLocalState `json:"wsrep_local_state"`
	ReadOnly           bool            `json:"read_only"`
	MaintenanceEnabled bool            `json:"maintenance_enabled"`
}

const InvalidIndex = math.MaxUint64

func (s DBState) IsHealthy(availableWhenReadOnly bool) bool {
	switch {
	case s.WsrepLocalIndex == InvalidIndex:
		return false
	case s.ReadOnly && !availableWhenReadOnly:
		return false
	case s.MaintenanceEnabled:
		return false
	case s.WsrepLocalState == Synced, s.WsrepLocalState == DonorDesynced:
		return true
	default:
		return false
	}
}

func (w WsrepLocalState) Comment() WsrepLocalStateComment {
	switch w {
	case Initialized:
		return "Initialized"
	case Joining:
		return "Joining"
	case DonorDesynced:
		return "Donor/Desynced"
	case Joined:
		return "Joined"
	case Synced:
		return "Synced"
	default:
		return WsrepLocalStateComment(fmt.Sprintf("Unrecognized state: %d", w))
	}
}
