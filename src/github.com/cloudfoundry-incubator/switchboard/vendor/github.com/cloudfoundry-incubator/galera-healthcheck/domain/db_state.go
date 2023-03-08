package domain

import (
	"fmt"
	"math"
)

type WsrepLocalState uint
type WsrepLocalStateComment string

const (
	Joining WsrepLocalState = iota + 1 // https://splice.com/blog/iota-elegant-constants-golang/
	DonorDesynced
	Joined
	Synced

	JoiningString       = WsrepLocalStateComment("Joining")
	DonorDesyncedString = WsrepLocalStateComment("Donor/Desynced")
	JoinedString        = WsrepLocalStateComment("Joined")
	SyncedString        = WsrepLocalStateComment("Synced")
)

type DBState struct {
	WsrepLocalIndex    uint            `json:"wsrep_local_index"`
	WsrepLocalState    WsrepLocalState `json:"wsrep_local_state"`
	ReadOnly           bool            `json:"read_only"`
	MaintenanceEnabled bool            `json:"maintenance_enabled"`
}

const InvalidIndex = math.MaxUint64

func (s DBState) IsHealthy() bool {
	switch {
	case s.WsrepLocalIndex == InvalidIndex:
		return false
	case s.ReadOnly:
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
	case Joining:
		return JoiningString
	case DonorDesynced:
		return DonorDesyncedString
	case Joined:
		return JoinedString
	case Synced:
		return SyncedString
	default:
		return WsrepLocalStateComment(fmt.Sprintf("Unrecognized state: %d", w))
	}
}
