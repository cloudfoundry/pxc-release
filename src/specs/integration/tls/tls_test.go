package tls_test

import (
	"database/sql"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
)

var _ = Describe("TLS", func() {
	var (
		rootPassword string
		proxyHost    string
	)
	BeforeEach(func() {
		var err error
		rootPassword, err = helpers.GetMySQLAdminPassword()
		Expect(err).NotTo(HaveOccurred())

		proxyHost, err = helpers.FirstProxyHost(helpers.BoshDeployment)
		Expect(err).NotTo(HaveOccurred())
	})

	It("requires a secure transport for client connections", func() {
		dsn := fmt.Sprintf("root:%s@tcp(%s:3306)/?tls=false", rootPassword, proxyHost)
		db, err := sql.Open("mysql", dsn)
		Expect(err).NotTo(HaveOccurred())

		err = db.Ping()
		Expect(err).To(MatchError(`Error 3159: Connections using insecure transport are prohibited while --require_secure_transport=ON.`))
	})

	It("requires TLSv1.2 for connections", func() {
		dsn := fmt.Sprintf("root:%s@tcp(%s:3306)/?tls=deprecated-tls11", rootPassword, proxyHost)
		db, err := sql.Open("mysql", dsn)
		Expect(err).NotTo(HaveOccurred())

		err = db.Ping()
		Expect(err).To(HaveOccurred())
	})

	It("accepts valid TLS connections", func() {
		// certificates aren't setup such that we can do proper TLS verification
		// This test exists to prove TLS < v1.2, fails but normal TLS connections succeed
		dsn := fmt.Sprintf("root:%s@tcp(%s:3306)/?tls=skip-verify", rootPassword, proxyHost)
		db, err := sql.Open("mysql", dsn)
		Expect(err).NotTo(HaveOccurred())

		err = db.Ping()
		Expect(err).ToNot(HaveOccurred())
	})
})
