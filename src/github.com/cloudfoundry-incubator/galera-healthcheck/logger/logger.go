package logger

import (
	"fmt"
	"time"
)

func LogWithTimestamp(format string, args ...interface{}) {
	fmt.Printf("[%s] - ", time.Now().Local())
	if (nil == args) {
		fmt.Printf(format)
	} else {
		fmt.Printf(format, args...)
	}
}
