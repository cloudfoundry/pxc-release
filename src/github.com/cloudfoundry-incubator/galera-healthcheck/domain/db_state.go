package domain

type WsrepLocalState uint

const (
	Joining WsrepLocalState = iota + 1 // https://splice.com/blog/iota-elegant-constants-golang/
	DonorDesynced
	Joined
	Synced
)

type DBState struct {
	WsrepLocalIndex uint
	WsrepLocalState uint
	ReadOnly        bool
}
