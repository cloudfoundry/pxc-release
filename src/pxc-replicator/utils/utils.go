// Package utils holds share helpers
package utils

import (
	"io"
	"log"
)

func CloseAndLogError(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Default().Println(err)
	}
}
