package sequence_number_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/erikstmartin/go-testdb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysqld_cmd/fakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("GaleraSequenceChecker", func() {

	const expectedSeqNumber = "32"

	var (
		sequenceChecker *sequence_number.SequenceNumberchecker
		mysqldCmd       *fakes.FakeMysqldCmd
	)

	BeforeEach(func() {
		rootConfig := config.Config{}
		logger := lagertest.NewTestLogger("sequence_number test")
		db, _ := sql.Open("testdb", "")

		mysqldCmd = &fakes.FakeMysqldCmd{}
		mysqldCmd.RecoverSeqnoReturns(expectedSeqNumber, nil)

		sequenceChecker = sequence_number.New(db, mysqldCmd, rootConfig, logger)
	})

	AfterEach(func() {
		testdb.Reset()
	})

	Describe("ServeHTTP", func() {
		Context("db works", func() {

			BeforeEach(func() {
				testdb.SetExecFunc(func(query string) (driver.Result, error) {
					return nil, nil
				})
			})

			It("returns an unsuccessful HTTP status", func() {
				req, err := http.NewRequest("GET", "/sequence_number", nil)
				Expect(err).ToNot(HaveOccurred())

				w := httptest.NewRecorder()
				sequenceChecker.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusInternalServerError))
				Expect(w.Body.String()).To(Equal("Failed to determine sequence number: can't determine sequence number when database is running"))
			})
		})

		Context("db is down", func() {
			BeforeEach(func() {
				testdb.SetExecFunc(func(query string) (driver.Result, error) {
					return nil, errors.New("failed to connect")
				})
			})

			It("returns a successful HTTP status", func() {
				req, err := http.NewRequest("GET", "/sequence_number", nil)
				Expect(err).ToNot(HaveOccurred())

				w := httptest.NewRecorder()
				sequenceChecker.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(w.Body.String()).To(ContainSubstring(expectedSeqNumber))
			})
		})
	})
})
