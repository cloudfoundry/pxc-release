// Package testhelper contains setup code for percona provided pxc-xtradbcluster images:
// https://hub.docker.com/r/percona/percona-xtradb-cluster
// https://github.com/Percona-Lab/percona-docker/tree/master/percona-xtradb-cluster-8.0
package testhelper

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	mathRand "math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

const (
	TLSDisabled    = "DISABLED"
	VerifyCA       = "VERIFY_CA"
	VerifyIdentity = "VERIFY_IDENTITY"
)

var (
	Image       = "percona/percona-xtradb-cluster"
	Tag         = "8.4"
	MysqlBinDir = os.Getenv("MYSQL_BIN_DIR")
	DataDir     = os.Getenv("DATA_DIR")
)

func backtick(in string) string {
	return fmt.Sprintf("`%s`", in)
}

func CreateTestNetwork() *testcontainers.DockerNetwork {
	ctx := context.Background()
	newNetwork, err := network.New(ctx)
	Expect(err).ToNot(HaveOccurred())
	testcontainers.CleanupNetwork(ginkgo.GinkgoTB(), newNetwork)

	return newNetwork
}

func GeneratePassword() string {
	return uuid.NewString()[:20]
}

type TestDataRow struct {
	ID        *int
	CreatedAt *string
	UpdatedAt *string
	Value     *int
}

func GenerateTestData(target config.Target, dbName, tableName string, numberRows int) {
	r := client.ReplClient{
		Source: target,
	}
	db, err := r.ConnectSource()
	Expect(err).ToNot(HaveOccurred())

	_, err = db.Exec(fmt.Sprintf("Create DATABASE IF NOT EXISTS %s;", backtick(dbName)))
	Expect(err).ToNot(HaveOccurred())
	Expect(db.Close()).To(Succeed())

	db, err = r.ConnectSource(dbName)
	Expect(err).ToNot(HaveOccurred())
	_, err = db.Exec(fmt.Sprintf(`CREATE TABLE %s (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY ,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    value INT NOT NULL
  );`, backtick(tableName)))
	Expect(err).ToNot(HaveOccurred())
	for i := range numberRows {
		_, err = db.Exec(fmt.Sprintf("INSERT INTO `%s` (`value`) VALUES(?);", tableName), i)
		Expect(err).ToNot(HaveOccurred())
	}
}

type StdoutLogConsumer struct {
	Buffer *gbytes.Buffer
}

func (lc *StdoutLogConsumer) Accept(l testcontainers.Log) {
	log.Default().Println("mysql:", string(l.Content))
	if lc.Buffer != nil {
		lc.Buffer.Write(l.Content)
	}
}

type Log struct {
	LogType string
	Content []byte
}

func writeKeyFile(path, name string) (key *rsa.PrivateKey, bytes []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).ToNot(HaveOccurred())

	file, err := os.Create(fmt.Sprintf("%s/%s", path, name))
	defer utils.CloseAndLogError(file)
	Expect(err).ToNot(HaveOccurred())

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	log.Default().Printf("generated %s at %s", name, file.Name())

	keyBytes := pem.EncodeToMemory(block)
	_, err = file.Write(keyBytes)
	Expect(err).ToNot(HaveOccurred())
	return key, keyBytes
}

func writeCaFile(path, name string, caPrivateKey *rsa.PrivateKey) (*x509.Certificate, []byte) {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2026),
		Subject: pkix.Name{
			CommonName: name,
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(0, 0, 1), // 10 years
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certDERBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &caPrivateKey.PublicKey, caPrivateKey)
	Expect(err).ToNot(HaveOccurred())

	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDERBytes,
	}
	pemBytes := pem.EncodeToMemory(&block)
	CAFile, err := os.Create(fmt.Sprintf("%s/%s", path, "server-ca.pem"))
	Expect(err).ToNot(HaveOccurred())
	_, err = CAFile.Write(pemBytes)

	Expect(err).ToNot(HaveOccurred())

	return cert, pemBytes
}

func writeCertFile(filename, path string, names []string, serverKeyPublic *rsa.PublicKey, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, []byte) {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: names[0],
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(10, 0, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		DNSNames:    names,
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, serverKeyPublic, caKey)
	Expect(err).ToNot(HaveOccurred())

	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDERBytes,
	}
	pemBytes := pem.EncodeToMemory(&block)
	serverCertFile, err := os.Create(fmt.Sprintf("%s/%s-%s", path, filename, "cert.pem"))
	log.Default().Printf("wrote: %s", serverCertFile.Name())
	Expect(err).ToNot(HaveOccurred())

	_, err = serverCertFile.Write(pemBytes)
	Expect(err).ToNot(HaveOccurred())

	return cert, pemBytes
}

func InitCerts(name, path, tlsMode string, aliases []string) (serverCerts, clientCerts config.Certs) {
	caKey, _ := writeKeyFile(path, "server-ca-key.pem")
	serverCa, serverCABytes := writeCaFile(path, name, caKey)

	serverKey, serverKeyBytes := writeKeyFile(path, "server-key.pem")
	_, serverCertBytes := writeCertFile("server", path, append([]string{"localhost", name}, aliases...), &serverKey.PublicKey, serverCa, caKey)

	clientKey, clientKeyBytes := writeKeyFile(path, "client-key.pem")
	_, clientCertBytes := writeCertFile("client", path, []string{"localhost", "client", fmt.Sprintf("%s-client", name)}, &clientKey.PublicKey, serverCa, caKey)

	switch tlsMode {
	case VerifyCA:
		clientCerts = config.Certs{
			CA: string(serverCABytes), // append(serverCABytes, serverCertBytes...),
		}
	default:
		clientCerts = config.Certs{
			CA:          string(serverCABytes),
			PrivateKey:  string(clientKeyBytes),
			Certificate: string(clientCertBytes),
		}
	}
	serverCerts = config.Certs{
		CA:          string(serverCABytes),
		PrivateKey:  string(serverKeyBytes),
		Certificate: string(serverCertBytes),
	}

	return serverCerts, clientCerts
}

func StartReplicatorInContainer(version string, config []byte, net *testcontainers.DockerNetwork, logBuffer *gbytes.Buffer) *testcontainers.DockerContainer {
	cmd := exec.Command("go", "build", "-o", "./replicator")
	// TODO make arch not hardcoded
	cmd.Env = []string{"HOME=/tmp", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0"}
	out, err := cmd.CombinedOutput()
	Expect(string(out)).To(BeEmpty())
	Expect(err).ToNot(HaveOccurred())
	f, err := os.CreateTemp("", "config.yml")
	Expect(err).ToNot(HaveOccurred())
	_, err = f.Write(config)
	Expect(err).ToNot(HaveOccurred())

	opts := []testcontainers.ContainerCustomizer{
		network.WithNetwork([]string{"client"}, net),
		testcontainers.WithName("replicator"),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				HostFilePath:      "./replicator",
				ContainerFilePath: "/replicator",
				FileMode:          0o0755,
			},
		),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				HostFilePath:      f.Name(),
				ContainerFilePath: "/config/config.yml",
				FileMode:          0o0644,
			},
		),
		testcontainers.WithLogConsumerConfig(&testcontainers.LogConsumerConfig{
			Opts:      []testcontainers.LogProductionOption{testcontainers.WithLogProductionTimeout(10 * time.Second)},
			Consumers: []testcontainers.LogConsumer{&StdoutLogConsumer{Buffer: logBuffer}},
		}),
		testcontainers.WithEntrypoint("/replicator", "-config", "/config/config.yml", "-data-dir", "/tmp", "-mysql-bin-path", "/usr/bin"),
	}
	ctx := context.Background()
	rep, err := testcontainers.Run(ctx, fmt.Sprintf("%s:%s", Image, version), opts...)
	testcontainers.CleanupContainer(ginkgo.GinkgoTB(), rep, testcontainers.StopTimeout(120*time.Second))
	Expect(err).ToNot(HaveOccurred())
	return rep
}

func StartPXCInstance(name, password, version string, tlsMode string, netAliases []string, net *testcontainers.DockerNetwork) (fromContainer config.Target, fromHost config.Target, container *testcontainers.DockerContainer) {
	serverID := mathRand.Intn(999) + 1
	opts := []testcontainers.ContainerCustomizer{
		network.WithNetwork(netAliases, net),
		testcontainers.WithExposedPorts("3306"),
		testcontainers.WithName(name),
		testcontainers.WithEnv(map[string]string{
			"MYSQL_ROOT_PASSWORD": password,
			"CLUSTER_NAME":        name,
			"MYSQL_ROOT_HOST":     "%",
		}),
		testcontainers.WithCmdArgs("--gtid-mode=ON", "--enforce-gtid-consistency=ON", "--pxc_strict_mode=PERMISSIVE", fmt.Sprintf("--server-id=%d", serverID)),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Synchronized with group, ready for connections").WithStartupTimeout(180*time.Second),
			wait.ForListeningPort("3306/tcp").WithStartupTimeout(120*time.Second),
			wait.ForExposedPort().WithStartupTimeout(120*time.Second),
		),
	}
	var clientCerts config.Certs
	if tlsMode != TLSDisabled {
		certsDir, err := os.MkdirTemp("", name)
		Expect(err).ToNot(HaveOccurred())
		_, clientCerts = InitCerts(name, certsDir, tlsMode, netAliases)
		opts = append(opts,
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					HostFilePath:      fmt.Sprintf("%s/server-ca.pem", certsDir),
					ContainerFilePath: "/certs/server-ca.pem",
					FileMode:          0o644,
				},
				testcontainers.ContainerFile{
					HostFilePath:      fmt.Sprintf("%s/server-cert.pem", certsDir),
					ContainerFilePath: "/certs/server-cert.pem",
					FileMode:          0o644,
				},
				testcontainers.ContainerFile{
					HostFilePath:      fmt.Sprintf("%s/server-key.pem", certsDir),
					ContainerFilePath: "/certs/server-key.pem",
					FileMode:          0o644,
				},
			),
			testcontainers.WithCmdArgs("--require-secure-transport=ON", "--ssl-cert=/certs/server-cert.pem", "--ssl-key=/certs/server-key.pem", "--ssl-ca=/certs/server-ca.pem"),
		)
	}

	ctx := context.Background()
	if os.Getenv("TEST_DEBUG") == "true" {
		opts = append(opts, testcontainers.WithLogConsumerConfig(&testcontainers.LogConsumerConfig{
			Opts:      []testcontainers.LogProductionOption{testcontainers.WithLogProductionTimeout(10 * time.Second)},
			Consumers: []testcontainers.LogConsumer{&StdoutLogConsumer{}},
		}),
		)
	}
	pxc, err := testcontainers.Run(ctx, fmt.Sprintf("%s:%s", Image, version), opts...)

	Expect(err).ToNot(HaveOccurred())

	testcontainers.CleanupContainer(ginkgo.GinkgoTB(), pxc, testcontainers.StopTimeout(120*time.Second))
	port, err := pxc.MappedPort(context.Background(), "3306")
	Expect(err).ToNot(HaveOccurred())

	// the networking with testcontainers makes this a bit hard.. we need to configure the replica with the "inner view" using the ContainerIP and the default 3306 port
	// but to run external checks we need the Host view which is a mapped port on localhost...
	return config.Target{
			Name: name,
			Host: name,
			Port: 3306,
			Creds: config.Creds{
				Username:      "root",
				Password:      password,
				AdminUsername: "root",
				AdminPassword: password,
			},
			Certs: clientCerts,
		}, config.Target{
			Name: name,
			Host: "localhost",
			Port: port.Num(),
			Creds: config.Creds{
				Username:      "root",
				Password:      password,
				AdminUsername: "root",
				AdminPassword: password,
			},
			Certs: clientCerts,
		},
		pxc
}
