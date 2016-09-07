package domain

type DBState struct {
	WsrepLocalIndex uint
	WsrepLocalState uint
	ReadOnly        bool
}
