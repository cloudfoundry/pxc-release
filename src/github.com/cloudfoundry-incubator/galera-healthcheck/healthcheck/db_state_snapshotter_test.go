package healthcheck_test

import (
	"database/sql"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
	. "github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DBStateSnapshotter", func() {
	Describe("State", func() {
		var (
			snapshotter *DBStateSnapshotter
			db          *sql.DB
			err         error
			mock        sqlmock.Sqlmock
		)

		BeforeEach(func() {
			db, mock, err = sqlmock.New()
			Expect(err).NotTo(HaveOccurred())

			snapshotter = &DBStateSnapshotter{DB: db}

			rand.Seed(time.Now().UnixNano())
		})

		AfterEach(func() {
			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})

		It("queries for the 'wsrep_local_state', 'wsrep_local_index', 'read_only' and 'pxc_maint_mode' attributes in a single query", func() {
			mock.ExpectQuery(`SELECT .*`).
				WillReturnRows(sqlmock.NewRows([]string{"wsrep_local_index", "wsrep_local_state", "read_only", "pxc_maint_mode"}).AddRow(
					"2", "4", "1", "1",
				))
			state, err := snapshotter.State()
			Expect(err).NotTo(HaveOccurred())

			Expect(state.WsrepLocalIndex).To(Equal(uint(2)), `wsrep_local_index was unexpectedly not 2!`)
			Expect(state.WsrepLocalState).To(Equal(domain.Synced), `wsrep_local_state was unexpectedly not "Synced"`)
			Expect(state.ReadOnly).To(BeTrue(), `read_only was unexpectedly not true!`)
			Expect(state.MaintenanceEnabled).To(BeTrue(), `pxc_maint_mode was unexpectedly not true!`)
		})

		intToBool := func(i int) bool {
			if i == 0 {
				return false
			}
			return true
		}
		It("does not simply return the values expected by this test", func() {
			for i := 0; i < 10; i++ {
				randomIndex := rand.Intn(4)
				randomWsrepState := rand.Intn(5)
				randomReadOnly := rand.Intn(2)
				randomMaint := rand.Intn(2)

				mock.ExpectQuery(`SELECT .*`).
					WillReturnRows(sqlmock.NewRows([]string{"wsrep_local_index", "wsrep_local_state", "read_only", "pxc_maint_mode"}).AddRow(
						strconv.Itoa(randomIndex), strconv.Itoa(randomWsrepState), strconv.Itoa(randomReadOnly), strconv.Itoa(randomMaint),
					))
				state, err := snapshotter.State()
				Expect(err).NotTo(HaveOccurred())

				Expect(state.WsrepLocalIndex).To(Equal(uint(randomIndex)),
					`wsrep_local_index was unexpectedly not 1!`)
				Expect(state.WsrepLocalState).To(Equal(domain.WsrepLocalState(randomWsrepState)),
					`wsrep_local_state was unexpectedly not "DonorDesynced"`)
				Expect(state.ReadOnly).To(Equal(intToBool(randomReadOnly)),
					`read_only was unexpectedly not false!`)
				Expect(state.MaintenanceEnabled).To(Equal(intToBool(randomMaint)),
					`pxc_maint_mode was unexpectedly not true!`)
			}
		})

		It("returns an error when it cannot query the instance state", func() {
			mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("error"))

			_, err = snapshotter.State()

			Expect(err).To(MatchError(errors.New("error")))
		})

	})
})
