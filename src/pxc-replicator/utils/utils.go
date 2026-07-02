// Package utils holds share helpers
package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	"github.com/cloudfoundry/pxc-release/replicator/config"
)

func CloseAndLogError(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Println(err)
	}
}

const (
	CASuffix = "ca.pem"
)

func WriteCertFiles(target config.Target, dataDir string) error {
	if target.Certs.CA == "" {
		return nil
	}
	caCertFile := fmt.Sprintf("%s/%s.%s", dataDir, target.Name, CASuffix)
	if err := os.WriteFile(caCertFile, []byte(target.Certs.CA), 0o600); err != nil {
		return fmt.Errorf("failed writing ca.pem for `%s`: %w", target.Name, err)
	}
	log.Printf("wrote ca cert: %s", caCertFile)
	return nil
}

func WriteMysqlCnf(target config.Target, dataDir string, admin bool) (string, error) {
	defaultsFile := fmt.Sprintf("%s/%s.mysql.cnf", dataDir, target.Name)
	user := target.Creds.Username
	pass := target.Creds.Password
	if admin {
		user = target.Creds.AdminUsername
		pass = target.Creds.AdminPassword
	}
	defaultFileContents := fmt.Sprintf(`[client]
  user = '%s'
  password = '%s'
  protocol = tcp
  host = '%s'
  port = '%d'`,
		user,
		pass,
		target.Host,
		target.Port,
	)
	err := os.WriteFile(defaultsFile, []byte(defaultFileContents), 0o600)
	if err != nil {
		return "", fmt.Errorf("failed writing defaults file: %w", err)
	}
	log.Printf("wrote config: %s", defaultsFile)
	return defaultsFile, nil
}

var gtidRegex = regexp.MustCompile(`SET @@GLOBAL\.GTID_PURGED=(?:\/\*.*?\*\/\s*)?'(?P<GTID>[a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12}:[0-9]*-[0-9]*)'`)

func ParseGTIDFromLine(line string) (string, bool) {
	match := gtidRegex.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}

	return match[1], true
}
