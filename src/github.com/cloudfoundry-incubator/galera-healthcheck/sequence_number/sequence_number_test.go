package sequence_number_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/erikstmartin/go-testdb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager/v3/lagertest"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysqld_cmd/fakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
)

var _ = Describe("GaleraSequenceChecker", func() {

	const (
		expectedSeqNumber       = "32"
		arbitratorSeqnoResponse = "no sequence number - running on arbitrator node"
	)

	var (
		sequenceChecker *sequence_number.SequenceNumberChecker
		mysqldCmd       *fakes.FakeMysqldCmd
		rootConfig      config.Config
		logger          *lagertest.TestLogger
		db              *sql.DB
	)

	BeforeEach(func() {
		rootConfig = config.Config{}
		logger = lagertest.NewTestLogger("sequence_number test")
		db, _ = sql.Open("testdb", "")

		mysqldCmd = &fakes.FakeMysqldCmd{}
		mysqldCmd.RecoverSeqnoReturns(expectedSeqNumber, nil)
	})

	JustBeforeEach(func() {
		sequenceChecker = sequence_number.New(db, mysqldCmd, rootConfig, logger, &sync.Mutex{})
	})

	AfterEach(func() {
		testdb.Reset()
	})

	Describe("Check", func() {
		Context("db works", func() {

			BeforeEach(func() {
				testdb.SetExecFunc(func(query string) (driver.Result, error) {
					return nil, nil
				})
			})

			It("returns an unsuccessful check", func() {
				_, err := sequenceChecker.Check(createReq())
				Expect(err).To(MatchError("can't determine sequence number when database is running"))
			})
		})

		Context("db is down", func() {
			BeforeEach(func() {
				testdb.SetExecFunc(func(query string) (driver.Result, error) {
					return nil, errors.New("failed to connect")
				})
			})

			It("returns a successful sequence number", func() {
				seq, err := sequenceChecker.Check(createReq())
				Expect(err).ToNot(HaveOccurred())
				Expect(seq).To(ContainSubstring(expectedSeqNumber))
			})

			Context("and recover cmd returns -1", func() {
				BeforeEach(func() {
					mysqldCmd.RecoverSeqnoReturns("-1", nil)
				})

				It("returns an error", func() {
					_, err := sequenceChecker.Check(createReq())
					Expect(err).To(MatchError("Invalid sequence number -1"))
				})
			})

			Context("and recover cmd returns error", func() {
				BeforeEach(func() {
					mysqldCmd.RecoverSeqnoReturns("", errors.New("something went wrong"))
				})

				It("returns an unsuccessful Check", func() {
					_, err := sequenceChecker.Check(createReq())
					Expect(err).To(MatchError("something went wrong"))
				})
			})

			Context("and sst_in_progress is present in DataDir", func() {
				BeforeEach(func() {
					dataDir := GinkgoT().TempDir()
					Expect(os.WriteFile(filepath.Join(dataDir, "sst_in_progress"), []byte{}, 0644)).To(Succeed())
					rootConfig.DataDir = dataDir
				})

				It("returns an error and does not invoke mysqld --wsrep-recover", func() {
					_, err := sequenceChecker.Check(createReq())
					Expect(err).To(MatchError("SST is in progress: refusing to run mysqld --wsrep-recover"))
					Expect(mysqldCmd.RecoverSeqnoCallCount()).To(Equal(0))
				})
			})

			Context("and sst_in_progress is absent from DataDir", func() {
				BeforeEach(func() {
					rootConfig.DataDir = GinkgoT().TempDir()
				})

				It("invokes mysqld --wsrep-recover and returns the sequence number", func() {
					seq, err := sequenceChecker.Check(createReq())
					Expect(err).ToNot(HaveOccurred())
					Expect(seq).To(Equal(expectedSeqNumber))
					Expect(mysqldCmd.RecoverSeqnoCallCount()).To(Equal(1))
				})
			})
		})

		Context("running on an arbitrator node", func() {
			BeforeEach(func() {
				rootConfig = config.Config{
					GaleraInit: config.GaleraInitConfig{
						ServiceName: "garbd",
					},
				}
			})

			It("returns a message stating it is an arbitrator node", func() {
				seq, err := sequenceChecker.Check(createReq())
				Expect(err).ToNot(HaveOccurred())
				Expect(seq).To(ContainSubstring(arbitratorSeqnoResponse))
			})
		})
	})
})

func createReq() *http.Request {
	req, err := http.NewRequest("", "/example.com", nil)
	Expect(err).ToNot(HaveOccurred())
	return req
}
