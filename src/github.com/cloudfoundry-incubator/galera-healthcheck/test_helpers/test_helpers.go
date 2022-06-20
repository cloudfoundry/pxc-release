package test_helpers

import (
	"net"
	"strconv"

	. "github.com/onsi/gomega"
)

func RandomPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	Expect(err).NotTo(HaveOccurred())

	_, port, err := net.SplitHostPort(l.Addr().String())
	Expect(err).NotTo(HaveOccurred())

	intPort, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())

	return intPort
}
