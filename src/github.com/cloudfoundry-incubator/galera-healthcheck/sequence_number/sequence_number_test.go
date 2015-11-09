package sequence_number_test

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/erikstmartin/go-testdb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("GaleraSequenceChecker", func() {

	const startPositionQuery = "SHOW variables LIKE 'wsrep_start_position'"
	const expectedSeqNumber = "32"

	var (
		sequenceChecker *sequence_number.SequenceNumberchecker
	)

	BeforeEach(func() {
		rootConfig := config.Config{}
		logger := lagertest.NewTestLogger("sequence_number test")
		db, _ := sql.Open("testdb", "")
		sequenceChecker = sequence_number.New(db, rootConfig, logger)
	})

	AfterEach(func() {
		testdb.Reset()
	})

	Describe("ServeHTTP", func() {
		Context("db works", func() {

			BeforeEach(func() {
				fake_result := fmt.Sprintf("fake-guid:%s", expectedSeqNumber)
				columns := []string{"Variable_name", "Value:Id"}
				result := fmt.Sprintf("wsrep_start_position,%s", fake_result)
				testdb.StubQuery(startPositionQuery, testdb.RowsFromCSVString(columns, result))
			})

			It("returns a successful HTTP status", func() {
				req, err := http.NewRequest("GET", "/sequence_number", nil)
				Expect(err).ToNot(HaveOccurred())

				w := httptest.NewRecorder()
				sequenceChecker.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(w.Body.String()).To(Equal(expectedSeqNumber))
			})
		})

		Context("db is down", func() {
			BeforeEach(func() {
				testdb.StubQueryError(startPositionQuery, errors.New("failed to connect"))
			})

			It("returns a failed HTTP status", func() {
				req, err := http.NewRequest("GET", "/sequence_number", nil)
				Expect(err).ToNot(HaveOccurred())

				w := httptest.NewRecorder()
				sequenceChecker.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(http.StatusInternalServerError))
				Expect(w.Body.String()).ToNot(ContainSubstring(expectedSeqNumber))
				Expect(w.Body.String()).To(ContainSubstring("failed to connect"))
			})
		})
	})
})
