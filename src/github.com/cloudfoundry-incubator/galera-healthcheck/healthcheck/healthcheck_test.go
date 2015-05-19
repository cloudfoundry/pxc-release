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
			It("It returns false and not synced", func() {
				config := HealthcheckTestHelperConfig{
					wsrepStatus:           healthcheck.STATE_JOINING,
					readOnly:              "OFF",
					availableWhenDonor:    false,
					availableWhenReadOnly: false,
				}

				result, msg := HealthcheckTestHelper(config)

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("not synced"))
			})
		})

		Context("when WSREP_STATUS is joined", func() {
			It("It returns false and not synced", func() {
				config := HealthcheckTestHelperConfig{
					wsrepStatus:           healthcheck.STATE_JOINED,
					readOnly:              "OFF",
					availableWhenDonor:    false,
					availableWhenReadOnly: false,
				}

				result, msg := HealthcheckTestHelper(config)

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("not synced"))
			})
		})

		Context("when WSREP_STATUS is donor", func() {
			Context("when not AVAILABLE_WHEN_DONOR", func() {
				It("It returns false and not-synced", func() {
					config := HealthcheckTestHelperConfig{
						wsrepStatus:           healthcheck.STATE_DONOR_DESYNCED,
						readOnly:              "OFF",
						availableWhenDonor:    false,
						availableWhenReadOnly: false,
					}

					result, msg := HealthcheckTestHelper(config)

					Expect(result).To(BeFalse())
					Expect(msg).To(Equal("not synced"))
				})
			})

			Context("when AVAILABLE_WHEN_DONOR", func() {
				Context("when READ_ONLY is ON", func() {
					Context("when AVAILABLE_WHEN_READONLY is true", func() {
						It("It returns true and synced", func() {
							config := HealthcheckTestHelperConfig{
								wsrepStatus:           healthcheck.STATE_DONOR_DESYNCED,
								readOnly:              "ON",
								availableWhenDonor:    true,
								availableWhenReadOnly: true,
							}

							result, msg := HealthcheckTestHelper(config)

							Expect(result).To(BeTrue())
							Expect(msg).To(Equal("synced"))
						})
					})

					Context("when AVAILABLE_WHEN_READONLY is false", func() {
						It("It returns false and read-only", func() {
							config := HealthcheckTestHelperConfig{
								wsrepStatus:           healthcheck.STATE_DONOR_DESYNCED,
								readOnly:              "ON",
								availableWhenDonor:    true,
								availableWhenReadOnly: false,
							}

							result, msg := HealthcheckTestHelper(config)

							Expect(result).To(BeFalse())
							Expect(msg).To(Equal("read-only"))
						})
					})
				})

				Context("when READ_ONLY is OFF", func() {
					It("It returns true and synced", func() {
						config := HealthcheckTestHelperConfig{
							wsrepStatus:           healthcheck.STATE_DONOR_DESYNCED,
							readOnly:              "OFF",
							availableWhenDonor:    true,
							availableWhenReadOnly: false,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})
			})

		})

		Context("when WSREP_STATUS is synced", func() {
			Context("when READ_ONLY is ON", func() {
				Context("when AVAILABLE_WHEN_READONLY is true", func() {
					It("It returns true and synced", func() {
						config := HealthcheckTestHelperConfig{
							wsrepStatus:           healthcheck.STATE_SYNCED,
							readOnly:              "ON",
							availableWhenDonor:    false,
							availableWhenReadOnly: true,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})

				Context("when AVAILABLE_WHEN_READONLY is false", func() {
					It("It returns false and read-only", func() {
						config := HealthcheckTestHelperConfig{
							wsrepStatus:           healthcheck.STATE_SYNCED,
							readOnly:              "ON",
							availableWhenDonor:    false,
							availableWhenReadOnly: false,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeFalse())
						Expect(msg).To(Equal("read-only"))
					})
				})
			})

			Context("when READ_ONLY is OFF", func() {
				It("It returns true and synced", func() {
					config := HealthcheckTestHelperConfig{
						wsrepStatus:           healthcheck.STATE_SYNCED,
						readOnly:              "OFF",
						availableWhenDonor:    false,
						availableWhenReadOnly: false,
					}

					result, msg := HealthcheckTestHelper(config)

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

				logger := lagertest.NewTestLogger("Healthcheck test")
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

				logger := lagertest.NewTestLogger("Healthcheck test")
				healthchecker := healthcheck.New(db, config, logger)

				res, msg := healthchecker.Check()

				Expect(res).To(BeFalse())
				Expect(msg).To(Equal("another test error"))
			})
		})
	})
})

type HealthcheckTestHelperConfig struct {
	wsrepStatus           int
	readOnly              string
	availableWhenDonor    bool
	availableWhenReadOnly bool
}

func HealthcheckTestHelper(testConfig HealthcheckTestHelperConfig) (bool, string) {
	db, _ := sql.Open("testdb", "")

	sql := "SHOW STATUS LIKE 'wsrep_local_state'"
	columns := []string{"Variable_name", "Value"}
	result := fmt.Sprintf("wsrep_local_state,%d", testConfig.wsrepStatus)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	sql = "SHOW GLOBAL VARIABLES LIKE 'read_only'"
	columns = []string{"Variable_name", "Value"}
	result = fmt.Sprintf("read_only,%s", testConfig.readOnly)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	config := healthcheck.Config{
		AvailableWhenDonor:    testConfig.availableWhenDonor,
		AvailableWhenReadOnly: testConfig.availableWhenReadOnly,
	}

	logger := lagertest.NewTestLogger("Healthcheck test")
	healthchecker := healthcheck.New(db, config, logger)

	return healthchecker.Check()
}
