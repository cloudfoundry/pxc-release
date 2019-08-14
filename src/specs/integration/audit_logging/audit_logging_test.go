package audit_logging_test

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	helpers "specs/test_helpers"
)

var _ = Describe("CF PXC MySQL Audit Logging", func() {

	var (
		db                         *sql.DB
		databaseName, auditLogPath string
		activeBackend              string
	)

	BeforeEach(func() {
		firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
		Expect(err).NotTo(HaveOccurred())

		mysqlPassword, err := helpers.GetMySQLAdminPassword()
		Expect(err).NotTo(HaveOccurred())

		db = helpers.DbConnWithUser("root", mysqlPassword, firstProxy)
		databaseName = helpers.DbSetup(db, "audit_logging_test_table")
		auditLogPath = os.Getenv("AUDIT_LOG_PATH")

		findActiveBackendSQL := `
		SELECT HOST_NAME
		FROM performance_schema.pxc_cluster_view
		ORDER BY LOCAL_INDEX ASC
		LIMIT 1`
		Expect(db.QueryRow(findActiveBackendSQL).Scan(&activeBackend)).
			To(Succeed())

	})

	AfterEach(func() {
		helpers.DbCleanup(db)
		cleanupUsers(db, "included_user")
		cleanupUsers(db, "excludeDBAudit1")
		cleanupUsers(db, "excludeDBAudit2")
	})

	Context("when reading and writing data as an excluded user", func() {
		var (
			excludedUser, excludedUserPassword string
		)
		Context("when the excluded user is from csv", func() {
			BeforeEach(func() {
				excludedUser = "excludeDBAudit1"
				excludedUserPassword = "password"

				createUserWithPermissions(db, databaseName, excludedUser, excludedUserPassword)
			})

			It("does not log any of the excluded user's activity in the audit log", func() {
				firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
				Expect(err).NotTo(HaveOccurred())
				dbConn := helpers.DbConnWithUser(excludedUser, excludedUserPassword, firstProxy)
				auditLogContents := readAndWriteDataAndGetAuditLogContents(dbConn, activeBackend, auditLogPath)

				Expect(string(auditLogContents)).ToNot(ContainSubstring("\"user\":\"excludeDBAudit1[excludeDBAudit1]"))
			})
		})
		Context("when the excluded user is not from csv", func() {
			BeforeEach(func() {
				excludedUser = "excludeDBAudit2"
				excludedUserPassword = "password"

				createUserWithPermissions(db, databaseName, excludedUser, excludedUserPassword)
			})

			It("does not log any of the excluded user's activity in the audit log", func() {
				firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
				Expect(err).NotTo(HaveOccurred())
				dbConn := helpers.DbConnWithUser(excludedUser, excludedUserPassword, firstProxy)
				auditLogContents := readAndWriteDataAndGetAuditLogContents(dbConn, activeBackend, auditLogPath)

				Expect(string(auditLogContents)).ToNot(ContainSubstring("\"user\":\"excludeDBAudit2[excludeDBAudit2]"))
			})
		})
	})

	Context("when reading and writing data as an included user", func() {
		var (
			includedUser, includedUserPassword string
		)

		BeforeEach(func() {
			includedUser = "included_user"
			includedUserPassword = "password"

			createUserWithPermissions(db, databaseName, includedUser, includedUserPassword)
		})

		It("does log all of the included user's activity in the audit log", func() {
			firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
			Expect(err).NotTo(HaveOccurred())

			dbConn := helpers.DbConnWithUser(includedUser, includedUserPassword, firstProxy)
			auditLogContents := readAndWriteDataAndGetAuditLogContents(dbConn, activeBackend, auditLogPath)

			Expect(string(auditLogContents)).To(ContainSubstring("\"user\":\"included_user[included_user]"))
		})
	})
})

// Get the size of the audit log file in bytes before reading or writing any data
// so we can read from that offset in the audit log file and return the contents from after that offset
func readAndWriteDataAndGetAuditLogContents(dbConn *sql.DB, activeBackend, auditLogPath string) string {
	logSizeBeforeTest := AuditLogSize(activeBackend, auditLogPath)

	readAndWriteFromDB(dbConn)

	destDir, err := ioutil.TempDir("", "audit_log_destination")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(destDir)

	fileName := filepath.Base(auditLogPath)
	destPath := fmt.Sprintf("%s/%s", destDir, fileName)

	BoshSCP(activeBackend, auditLogPath, destPath)

	auditLogContents := readFileFromOffset(destPath, logSizeBeforeTest)

	return auditLogContents
}

func readFileFromOffset(filePath string, offset int64) string {
	file, err := os.Open(filePath)
	Expect(err).NotTo(HaveOccurred())

	defer file.Close()

	fileInfo, err := os.Stat(filePath)
	Expect(err).NotTo(HaveOccurred())

	bufsize := fileInfo.Size() - offset
	buf := make([]byte, bufsize)

	_, err = file.ReadAt(buf, offset)
	Expect(err).NotTo(HaveOccurred())

	return string(buf)
}

func readAndWriteFromDB(dbConn *sql.DB) {
	var err error
	query := "REPLACE INTO pxc_release_test_db.audit_logging_test_table VALUES('writing data')"
	_, err = dbConn.Query(query)
	Expect(err).NotTo(HaveOccurred())
	query = "SELECT * FROM pxc_release_test_db.audit_logging_test_table"
	_, err = dbConn.Query(query)
	Expect(err).NotTo(HaveOccurred())
}

func BoshSCP(activeBackend, remoteFilePath, destPath string) {
	sourcePath := fmt.Sprintf("%s:%s", activeBackend, remoteFilePath)

	cmd := exec.Command("bosh", "scp", sourcePath, destPath)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(session, "30s", "1s").Should(gexec.Exit(0))
	Expect(destPath).To(BeARegularFile())
}

func AuditLogSize(activeBackend, remoteFilePath string) int64 {
	commandOnVM := strings.Join([]string{"\"wc -c ", remoteFilePath, "\""}, "")
	cmd := exec.Command("bosh", "ssh", activeBackend, "-c", commandOnVM)

	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	session.Wait(10 * time.Second)
	Expect(err).NotTo(HaveOccurred())

	exp := regexp.MustCompile(`stdout \| ([0-9]+) /var/vcap/store/mysql_audit_logs/mysql_server_audit.log`)
	size, err := strconv.Atoi(exp.FindStringSubmatch(string(session.Out.Contents()))[1])
	Expect(err).NotTo(HaveOccurred())

	return int64(size)
}

func createUserWithPermissions(db *sql.DB, databaseName, mysqlUsername, mysqlPassword string) {
	cleanupUsers(db, mysqlUsername)
	query := fmt.Sprintf("CREATE USER %s IDENTIFIED BY '%s';", mysqlUsername, mysqlPassword)
	_, err := db.Exec(query)
	Expect(err).NotTo(HaveOccurred())
	query = fmt.Sprintf("GRANT ALL ON `%s`.* TO '%s'@'%%';", databaseName, mysqlUsername)
	_, err = db.Exec(query)
	Expect(err).NotTo(HaveOccurred())
}

func cleanupUsers(db *sql.DB, mysqlUsername string) {
	query := fmt.Sprintf("DROP USER IF EXISTS %s;", mysqlUsername)
	_, err := db.Exec(query)
	Expect(err).NotTo(HaveOccurred())
}
