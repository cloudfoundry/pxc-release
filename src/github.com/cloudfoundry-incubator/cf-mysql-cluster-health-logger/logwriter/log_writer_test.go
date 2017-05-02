package logwriter_test

import (
	"database/sql"
	"errors"

	testdb "github.com/erikstmartin/go-testdb"

	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/cf-mysql-cluster-health-logger/logwriter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const expectedStatusSQL = `
 SHOW STATUS
 WHERE Variable_name like 'wsrep%'`
const expectedVariablesSQL = `
 SHOW VARIABLES
 WHERE Variable_name = 'sql_log_bin'`

var (
	logFile *os.File
	err     error
)

var _ = Describe("Cluster Health Logger", func() {

	BeforeEach(func() {
		logFile, err = ioutil.TempFile(os.TempDir(), "cluster-health-logger")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err = os.Remove(logFile.Name())
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when the log file does not exist", func() {
		BeforeEach(func() {
			err = os.Remove(logFile.Name())
			Expect(err).ToNot(HaveOccurred())
		})

		It("writes headers to the file", func() {
			logWriter := logWriterTestHelper(logFile.Name())
			ts := "happy-time"
			err = logWriter.Write(ts)
			Expect(err).ToNot(HaveOccurred())
			contents, err := ioutil.ReadFile(logFile.Name())
			Expect(err).ToNot(HaveOccurred())
			contentsStr := string(contents)
			Expect(contentsStr).To(Equal("timestamp|a|b|c|d|e|f|g|h|i|j\nhappy-time|1|2|3|4|5|6|7|8|9|10\n"))
		})
	})

	Context("when the log file exists with content", func() {
		BeforeEach(func() {
			err = os.Remove(logFile.Name())
			Expect(err).ToNot(HaveOccurred())

			logWriter := logWriterTestHelper(logFile.Name())
			ts1 := "happy-time"
			err = logWriter.Write(ts1)
			Expect(err).ToNot(HaveOccurred())
		})

		It("writes a new row", func() {
			logWriter := logWriterTestHelper(logFile.Name())
			ts2 := "sad-time"
			err = logWriter.Write(ts2)
			Expect(err).ToNot(HaveOccurred())
			contents, err := ioutil.ReadFile(logFile.Name())
			Expect(err).ToNot(HaveOccurred())
			contentsStr := string(contents)
			Expect(contentsStr).To(Equal("timestamp|a|b|c|d|e|f|g|h|i|j\nhappy-time|1|2|3|4|5|6|7|8|9|10\nsad-time|1|2|3|4|5|6|7|8|9|10\n"))
		})
	})

	Context("when the log file has been truncated", func() {
		BeforeEach(func() {
			err = logFile.Truncate(0)
			Expect(err).ToNot(HaveOccurred())
		})

		It("writes new headers with the next row", func() {
			logWriter := logWriterTestHelper(logFile.Name())
			ts := "happy-time"
			err = logWriter.Write(ts)
			Expect(err).ToNot(HaveOccurred())
			contents, err := ioutil.ReadFile(logFile.Name())
			Expect(err).ToNot(HaveOccurred())
			contentsStr := string(contents)
			Expect(contentsStr).To(Equal("timestamp|a|b|c|d|e|f|g|h|i|j\nhappy-time|1|2|3|4|5|6|7|8|9|10\n"))
		})
	})

	Context("when the query errors", func() {
		It("returns the error", func() {
			db, err := sql.Open("testdb", "")
			Expect(err).ToNot(HaveOccurred())

			testdb.StubQueryError(expectedStatusSQL, errors.New("Database connection failure"))
			logWriter := logwriter.New(db, logFile.Name())
			err = logWriter.Write("sad-time")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(errors.New("Database connection failure")))
		})
	})

})

func logWriterTestHelper(filePath string) logwriter.LogWriter {
	db, err := sql.Open("testdb", "")
	Expect(err).ToNot(HaveOccurred())

	columns := []string{"Variable_name", "Value"}
	statusResult := "a,1\nb,2\nc,3\nd,4\ne,5\nf,6\ng,7\nh,8\ni,9"
	testdb.StubQuery(expectedStatusSQL, testdb.RowsFromCSVString(columns, statusResult))

	variablesResult := "j,10"
	testdb.StubQuery(expectedVariablesSQL, testdb.RowsFromCSVString(columns, variablesResult))

	return logwriter.New(db, filePath)
}
