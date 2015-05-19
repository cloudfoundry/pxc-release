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
					healthcheck.STATE_JOINING,
					"OFF",
					false,
					false,
				}

				result, msg := HealthcheckTestHelper(config)

				Expect(result).To(BeFalse())
				Expect(msg).To(Equal("not synced"))
			})
		})

		Context("when WSREP_STATUS is joined", func() {
			It("It returns false and not synced", func() {
				config := HealthcheckTestHelperConfig{
					healthcheck.STATE_JOINED,
					"OFF",
					false,
					false,
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
						healthcheck.STATE_DONOR_DESYNCED,
						"OFF",
						false,
						false,
					}

					result, msg := HealthcheckTestHelper(config)

					Expect(result).To(BeFalse())
					Expect(msg).To(Equal("not synced"))
				})
			})

			Context("when AVAILABLE_WHEN_DONOR", func() {
				Context("when READ_ONLY is ON and AVAILABLE_WHEN_READONLY is true", func() {
					It("It returns true and synced", func() {
						config := HealthcheckTestHelperConfig{
							healthcheck.STATE_DONOR_DESYNCED,
							"ON",
							true,
							true,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})

				Context("when READ_ONLY is ON and AVAILABLE_WHEN_READONLY is false", func() {
					It("It returns false and read-only", func() {
						config := HealthcheckTestHelperConfig{
							healthcheck.STATE_DONOR_DESYNCED,
							"ON",
							true,
							false,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeFalse())
						Expect(msg).To(Equal("read-only"))
					})
				})

				Context("when READ_ONLY is OFF", func() {
					It("It returns true and synced", func() {
						config := HealthcheckTestHelperConfig{
							healthcheck.STATE_DONOR_DESYNCED,
							"OFF",
							true,
							false,
						}

						result, msg := HealthcheckTestHelper(config)

						Expect(result).To(BeTrue())
						Expect(msg).To(Equal("synced"))
					})
				})
			})

		})

		Context("when WSREP_STATUS is synced", func() {

			Context("when READ_ONLY is ON and AVAILABLE_WHEN_READONLY is true", func() {
				It("It returns true and synced", func() {

					config := HealthcheckTestHelperConfig{
						healthcheck.STATE_SYNCED,
						"ON",
						false,
						true,
					}

					result, msg := HealthcheckTestHelper(config)

					Expect(result).To(BeTrue())
					Expect(msg).To(Equal("synced"))
				})
			})

			Context("when READ_ONLY is ON and AVAILABLE_WHEN_READONLY is false", func() {
				It("It returns false and read-only", func() {

					config := HealthcheckTestHelperConfig{
						healthcheck.STATE_SYNCED,
						"ON",
						false,
						false,
					}

					result, msg := HealthcheckTestHelper(config)

					Expect(result).To(BeFalse())
					Expect(msg).To(Equal("read-only"))
				})
			})

			Context("when READ_ONLY is OFF", func() {
				It("It returns true and synced", func() {
					config := HealthcheckTestHelperConfig{
						healthcheck.STATE_SYNCED,
						"OFF",
						false,
						false,
					}

					result, msg := HealthcheckTestHelper(config)

					Expect(result).To(BeTrue())
					Expect(msg).To(Equal("synced"))
				})
			})
		})

		Context("when SHOW STATUS has errors", func() {
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

		Context("when SHOW STATUS has errors", func() {
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
	wsrep_status             string
	read_only                string
	available_when_donor     bool
	available_when_read_only bool
}

func HealthcheckTestHelper(testConfig HealthcheckTestHelperConfig) (bool, string) {
	db, _ := sql.Open("testdb", "")

	sql := "SHOW STATUS LIKE 'wsrep_local_state'"
	columns := []string{"Variable_name", "Value"}
	result := fmt.Sprintf("wsrep_local_state,%s", testConfig.wsrep_status)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	sql = "SHOW GLOBAL VARIABLES LIKE 'read_only'"
	columns = []string{"Variable_name", "Value"}
	result = fmt.Sprintf("read_only,%s", testConfig.read_only)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))

	config := healthcheck.Config{
		AvailableWhenDonor:    testConfig.available_when_donor,
		AvailableWhenReadOnly: testConfig.available_when_read_only,
	}

	logger := lagertest.NewTestLogger("Healthcheck test")
	healthchecker := healthcheck.New(db, config, logger)

	return healthchecker.Check()
}
