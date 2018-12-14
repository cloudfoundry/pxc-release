package tls_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"

	"database/sql"
)

var _ = Describe("Tls", func() {
	It("tests all the connections are TLS", func() {

		query := `SELECT sbt.variable_value AS tls_version,  t2.variable_value AS cipher,
			processlist_user AS user, processlist_host AS host
			FROM performance_schema.status_by_thread  AS sbt
			JOIN performance_schema.threads AS t ON t.thread_id = sbt.thread_id
			JOIN performance_schema.status_by_thread AS t2 ON t2.thread_id = t.thread_id
			WHERE sbt.variable_name = 'Ssl_version' and t2.variable_name = 'Ssl_cipher' ORDER BY tls_version`
		rows, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		var (
			tls_version string
			cipher      string
			user        string
			host        string
		)

		defer rows.Close()
		for rows.Next() {
			err = rows.Scan(&tls_version, &cipher, &user, &host)
			Expect(err).NotTo(HaveOccurred())
			Expect(user).NotTo(BeNil())
			Expect(host).NotTo(BeNil())

			if !(host == "localhost" || host == "127.0.0.1") {
				Expect(tls_version).To(MatchRegexp("TLSv1\\.[1,2,3]"))
				Expect(cipher).To(MatchRegexp("ECDHE-RSA.+"))
			}
		}
	})
})
