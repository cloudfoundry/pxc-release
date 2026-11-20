package sequence_number_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
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
		mock            sqlmock.Sqlmock
	)

	BeforeEach(func() {
		rootConfig = config.Config{}
		logger = lagertest.NewTestLogger("sequence_number test")

		var err error
		db, mock, err = sqlmock.New(sqlmock.MonitorPingsOption(true))
		Expect(err).NotTo(HaveOccurred())

		mysqldCmd = &fakes.FakeMysqldCmd{}
		mysqldCmd.RecoverSeqnoReturns(expectedSeqNumber, nil)
	})

	JustBeforeEach(func() {
		sequenceChecker = sequence_number.New(db, mysqldCmd, rootConfig, logger, &sync.Mutex{})
	})

	AfterEach(func() {
		_ = db.Close()
	})

	Describe("Check", func() {
		Context("db works", func() {

			BeforeEach(func() {
				mock.ExpectPing()
			})

			It("returns an unsuccessful check", func() {
				_, err := sequenceChecker.Check(createReq())
				Expect(err).To(MatchError("can't determine sequence number when database is running"))
			})
		})

		Context("db is down", func() {
			BeforeEach(func() {
				mock.ExpectPing().WillReturnError(errors.New("failed to connect"))
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

		Context("db responds with a MySQL error", func() {
			DescribeTable("treats the database as running and does not invoke mysqld --wsrep-recover",
				func(number uint16, message string) {
					mock.ExpectPing().WillReturnError(&mysql.MySQLError{Number: number, Message: message})

					_, err := sequenceChecker.Check(createReq())
					Expect(err).To(MatchError("can't determine sequence number when database is running"))
					Expect(mysqldCmd.RecoverSeqnoCallCount()).To(Equal(0))
				},
				Entry("access denied", uint16(1045), "Access denied for user 'root'@'localhost'"),
				Entry("too many connections", uint16(1040), "Too many connections"),
				Entry("max user connections", uint16(1203), "User already has more than 'max_user_connections' active connections"),
				Entry("WSREP not ready", uint16(1047), "WSREP has not yet prepared node for application use"),
			)
		})

		Context("db does not respond", func() {
			var (
				reqCtx  context.Context
				cancel  context.CancelFunc
				checker *sequence_number.SequenceNumberChecker
			)

			BeforeEach(func() {
				mock.ExpectPing().WillDelayFor(5 * time.Second)

				reqCtx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
				rootConfig.DataDir = GinkgoT().TempDir()
				checker = sequence_number.New(db, mysqldCmd, rootConfig, logger, &sync.Mutex{})
			})

			AfterEach(func() {
				cancel()
			})

			It("treats the database as running, releases the mutex, and does not invoke mysqld --wsrep-recover", func() {
				req := createReq().WithContext(reqCtx)

				start := time.Now()
				_, err := checker.Check(req)
				Expect(err).To(MatchError("can't determine sequence number when database is running"))
				Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
				Expect(mysqldCmd.RecoverSeqnoCallCount()).To(Equal(0))
			})
		})

		Context("when the process mutex is held by a concurrent start request", func() {
			var (
				mutex     *sync.Mutex
				dbUp      atomic.Bool
				dynamicDB *sql.DB
				results   chan error
			)

			BeforeEach(func() {
				mutex = &sync.Mutex{}
				dbUp.Store(false)
				rootConfig.DataDir = GinkgoT().TempDir()
				dynamicDB = sql.OpenDB(&togglingConnector{up: &dbUp})
			})

			AfterEach(func() {
				_ = dynamicDB.Close()
			})

			It("evaluates db state after acquiring the mutex and does not run mysqld --wsrep-recover", func() {
				checker := sequence_number.New(dynamicDB, mysqldCmd, rootConfig, logger, mutex)

				mutex.Lock()
				results = make(chan error, 1)
				go func() {
					defer GinkgoRecover()
					_, err := checker.Check(createReq())
					results <- err
				}()

				// Give the request a chance to run any checks that precede the lock,
				// then simulate /start_mysql_* bringing the database up.
				time.Sleep(100 * time.Millisecond)
				dbUp.Store(true)
				mutex.Unlock()

				var err error
				Eventually(results).Should(Receive(&err))
				Expect(err).To(MatchError("can't determine sequence number when database is running"))
				Expect(mysqldCmd.RecoverSeqnoCallCount()).To(Equal(0))
			})
		})

		Context("running on an arbitrator node", func() {
			BeforeEach(func() {
				rootConfig = config.Config{
					Monit: config.MonitConfig{
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

// togglingConnector simulates a database that refuses connections until up is
// set, then accepts them. Pings on the resulting connection succeed.
type togglingConnector struct{ up *atomic.Bool }

func (c *togglingConnector) Connect(context.Context) (driver.Conn, error) {
	if !c.up.Load() {
		return nil, errors.New("failed to connect")
	}
	return togglingConn{}, nil
}

func (c *togglingConnector) Driver() driver.Driver { return nil }

type togglingConn struct{}

func (togglingConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (togglingConn) Close() error                        { return nil }
func (togglingConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }
