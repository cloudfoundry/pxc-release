package logger

import (
	"fmt"
	"time"
)

type Logger interface {
	Log(message string)
}

type StdOutLogger struct {
	loggingOn bool
}

func NewStdOutLogger(loggingOn bool) *StdOutLogger {
	return &StdOutLogger{
		loggingOn: loggingOn,
	}
}

func (l StdOutLogger) Log(info string) {
	if l.loggingOn {
		fmt.Printf("%v ----- %v\n", time.Now().Local(), info)
	}
}
