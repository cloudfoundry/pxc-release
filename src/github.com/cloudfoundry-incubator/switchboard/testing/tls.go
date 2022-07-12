package testing

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
)

func CertificatePEM(derBytes []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func PrivateKeyPEM(key crypto.PrivateKey) []byte {
	derBytes, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: derBytes})
}

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
	caCertPool.AppendCertsFromPEM(caPEM)
	return tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
	).Client(tlsconfig.WithAuthority(caCertPool))
}
