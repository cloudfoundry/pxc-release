package testing

import (
	"crypto/tls"
	"crypto/x509"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
)

func GenerateSelfSignedCertificate(names ...string) (caPEM []byte, tlsCert tls.Certificate, err error) {
	authority, err := certtest.BuildCA("testCA")
	if err != nil {
		return nil, tlsCert, err
	}

	caPEM, err = authority.CertificatePEM()
	if err != nil {
		return nil, tlsCert, err
	}

	certificate, err := authority.BuildSignedCertificate("localhost", certtest.WithDomains(names...))
	if err != nil {
		return nil, tlsCert, err
	}

	tlsCert, err = certificate.TLSCertificate()
	if err != nil {
		return nil, tlsCert, err
	}

	return caPEM, tlsCert, err
}

func ServerConfigFromCertificate(certificate tls.Certificate) (*tls.Config, error) {
	return tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentity(certificate),
	).Server()
}

func ClientConfigFromAuthority(caPEM []byte) (*tls.Config, error) {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(caPEM))
	return tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
	).Client(tlsconfig.WithAuthority(caCertPool))
}
