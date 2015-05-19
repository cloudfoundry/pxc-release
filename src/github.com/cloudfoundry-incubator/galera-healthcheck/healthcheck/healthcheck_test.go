package healthcheck_test

import (
	"errors"
	"fmt"

	"database/sql"

	testdb "github.com/erikstmartin/go-testdb"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("GaleraHealthChecker", func() {

	Describe("Check", func() {
		Context("when WSREP_STATUS is joining", func() {
			It("returns false and joining", func() {
				config := healthcheckTestHelperConfig{
					wsrepStatus: healthcheck.STATE_JOINING,
				}

				result, msg := healthcheckTestHelper(config)

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("joining"))
			})
		})

		Context("when WSREP_STATUS is joined", func() {
			It("returns false and joined", func() {
				config := healthcheckTestHelperConfig{
					wsrepStatus: healthcheck.STATE_JOINED,
				}

				result, msg := healthcheckTestHelper(config)

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("joined"))
			})
		})

		Context("when WSREP_STATUS is donor", func() {
			Context("when not AVAILABLE_WHEN_DONOR", func() {
				It("returns false and not-synced", func() {
					config := healthcheckTestHelperConfig{
						wsrepStatus: healthcheck.STATE_DONOR_DESYNCED,
					}

					result, msg := healthcheckTestHelper(config)

					Expect(result).To(BeFalse())
					Expect(msg).To(Equal("not synced"))
				})
			})

			Context("when AVAILABLE_WHEN_DONOR", func() {
				Context("when READ_ONLY is ON", func() {
					Context("when AVAILABLE_WHEN_READONLY is true", func() {
						It("returns true and synced", func() {
							config := healthcheckTestHelperConfig{
								wsrepStatus:           healthcheck.STATE_DONOR_DESYNCED,
								readOnly:              true,
								availableWhenDonor:    true,
								availableWhenReadOnly: true,
							}

							result, msg := healthcheckTestHelper(config)

							Expect(result).To(BeTrue())
							Expect(msg).To(Equal("synced"))
						})
					})

					Context("when AVAILABLE_WHEN_READONLY is false", func() {
						It("returns false and read-only", func() {
							config := healthcheckTestHelperConfig{
								wsrepStatus:        healthcheck.STATE_DONOR_DESYNCED,
								readOnly:           true,
								availableWhenDonor: true,
							}

							result, msg := healthcheckTestHelper(config)

							Expect(result).To(BeFalse())
							Expect(msg).To(Equal("read-only"))
						})
					})
				})

				Context("when READ_ONLY is OFF", func() {
					It("returns true and synced", func() {
						config := healthcheckTestHelperConfig{
							wsrepStatus:        healthcheck.STATE_DONOR_DESYNCED,
							availableWhenDonor: true,
						}

						result, msg := healthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})
			})

		})

		Context("when WSREP_STATUS is synced", func() {
			Context("when READ_ONLY is ON", func() {
				Context("when AVAILABLE_WHEN_READONLY is true", func() {
					It("returns true and synced", func() {
						config := healthcheckTestHelperConfig{
							wsrepStatus:           healthcheck.STATE_SYNCED,
							readOnly:              true,
							availableWhenReadOnly: true,
						}

						result, msg := healthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})

				Context("when AVAILABLE_WHEN_READONLY is false", func() {
					It("returns false and read-only", func() {
						config := healthcheckTestHelperConfig{
							wsrepStatus: healthcheck.STATE_SYNCED,
							readOnly:    true,
						}

						result, msg := healthcheckTestHelper(config)

						Expect(result).To(BeFalse())
						Expect(msg).To(Equal("read-only"))
					})
				})
			})

			Context("when READ_ONLY is OFF", func() {
				It("returns true and synced", func() {
					config := healthcheckTestHelperConfig{
						wsrepStatus: healthcheck.STATE_SYNCED,
					}

					result, msg := healthcheckTestHelper(config)

					Expect(result).To(BeTrue())
					Expect(msg).To(Equal("synced"))
				})
			})
		})

		Context("when SHOW STATUS returns an error", func() {
			It("returns false and the error message", func() {
				db, _ := sql.Open("testdb", "")

				sql := "SHOW STATUS LIKE 'wsrep_local_state'"
				testdb.StubQueryError(sql, errors.New("test error"))

				config := healthcheck.Config{
					AvailableWhenDonor:    false,
					AvailableWhenReadOnly: false,
				}

				logger := lagertest.NewTestLogger("healthcheck test")
				healthchecker := healthcheck.New(db, config, logger)

				result, msg := healthchecker.Check()

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("test error"))
			})
		})

		Context("when SHOW GLOBAL VARIABLES LIKE returns an error", func() {
			It("returns false and the error message", func() {
				db, _ := sql.Open("testdb", "")

				sql := "SHOW STATUS LIKE 'wsrep_local_state'"
				columns := []string{"Variable_name", "Value"}
				result := "wsrep_local_state,4"
				testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

				sql = "SHOW GLOBAL VARIABLES LIKE 'read_only'"
				testdb.StubQueryError(sql, errors.New("another test error"))

				config := healthcheck.Config{
					AvailableWhenDonor:    false,
					AvailableWhenReadOnly: false,
				}

				logger := lagertest.NewTestLogger("healthcheck test")
				healthchecker := healthcheck.New(db, config, logger)

				res, msg := healthchecker.Check()

				Expect(res).To(BeFalse())
				Expect(msg).To(Equal("another test error"))
			})
		})
	})
})

type healthcheckTestHelperConfig struct {
	wsrepStatus           int
	readOnly              bool
	availableWhenDonor    bool
	availableWhenReadOnly bool
}

func healthcheckTestHelper(testConfig healthcheckTestHelperConfig) (bool, string) {
	db, _ := sql.Open("testdb", "")

	sql := "SHOW STATUS LIKE 'wsrep_local_state'"
	columns := []string{"Variable_name", "Value"}
	result := fmt.Sprintf("wsrep_local_state,%d", testConfig.wsrepStatus)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	sql = "SHOW GLOBAL VARIABLES LIKE 'read_only'"
	columns = []string{"Variable_name", "Value"}
	var readOnlyText string
	if testConfig.readOnly {
		readOnlyText = "ON"
	} else {
		readOnlyText = "OFF"
	}
	result = fmt.Sprintf("read_only,%s", readOnlyText)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	config := healthcheck.Config{
		AvailableWhenDonor:    testConfig.availableWhenDonor,
		AvailableWhenReadOnly: testConfig.availableWhenReadOnly,
	}

	logger := lagertest.NewTestLogger("healthcheck test")
	healthchecker := healthcheck.New(db, config, logger)

	return healthchecker.Check()
}
