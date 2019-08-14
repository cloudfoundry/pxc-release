package mtls_test

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"

	"code.cloudfoundry.org/tlsconfig/certtest"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"specs/test_helpers"
)

var _ = Describe("Mutual TLS", func() {
	var (
		username string
		password string
	)

	BeforeEach(func() {
		username = strings.Replace(uuid.New().String(), "-", "", -1)
		password = uuid.New().String()
		_, err := mysqlConn.Exec(`CREATE USER ?@'%' IDENTIFIED BY ? REQUIRE SUBJECT '/CN=mysql_client_certificate'`, username, password)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_, err := mysqlConn.Exec(`DROP USER IF EXISTS ?@'%'`, username)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("When connecting without client certificates", func() {
		It("should reject the connection attempt", func() {
			db, err := connectWithTLSConfig(username, password, "false")
			Expect(err).NotTo(HaveOccurred())

			Expect(db.Ping()).To(MatchError(ContainSubstring(`Access denied for user '%s'`, username)))
		})
	})

	Context("When connecting with untrusted client certificates", func() {
		BeforeEach(func() {
			untrustedCert, err := createUntrustedCertificate()
			Expect(err).NotTo(HaveOccurred())

			Expect(mysql.RegisterTLSConfig("untrusted-tls", &tls.Config{
				Certificates: []tls.Certificate{
					untrustedCert,
				},
				InsecureSkipVerify: true,
			})).To(Succeed())
		})

		It("should reject the connection attempt", func() {
			db, err := connectWithTLSConfig(username, password, "untrusted-tls")
			Expect(err).NotTo(HaveOccurred())

			Expect(db.Ping()).To(MatchError(ContainSubstring(`tls: unknown certificate authority`)))
		})
	})

	Context("When connecting with valid client certificate", func() {
		BeforeEach(func() {
			trustedCert, err := test_helpers.GetDeploymentCertificateByName(`mysql_client_certificate`)
			Expect(err).NotTo(HaveOccurred())

			Expect(mysql.RegisterTLSConfig("trusted-tls", &tls.Config{
				Certificates: []tls.Certificate{
					trustedCert,
				},
				InsecureSkipVerify: true,
			})).To(Succeed())
		})

		It("should allow the connection attempt", func() {
			db, err := connectWithTLSConfig(username, password, "trusted-tls")
			Expect(err).NotTo(HaveOccurred())

			Expect(db.Ping()).To(Succeed())
		})
	})
})

func connectWithTLSConfig(username, password, tlsConfigName string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/?tls=%s",
		username,
		password,
		ProxyHost,
		3306,
		tlsConfigName,
	)

	return sql.Open("mysql", dsn)
}

func createUntrustedCertificate() (tls.Certificate, error) {
	untrustedAuthority, err := certtest.BuildCA("some-CA")
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := untrustedAuthority.BuildSignedCertificate("client")
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert.TLSCertificate()
}
