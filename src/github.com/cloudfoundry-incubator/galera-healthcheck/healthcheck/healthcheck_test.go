package healthcheck_test

import (
	"fmt"
	"errors"

	"database/sql"
	testdb "github.com/erikstmartin/go-testdb"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
)

var _ =	Describe("RiakHealthChecker", func() {

	Describe("Check", func() {
		Context("when WSREP_STATUS is joining", func() {
			It("It returns false and not synced", func() {
				config := HealthcheckTestHelperConfig{
					"1",
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
					"3",
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
						"2",
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
							"2",
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
							"2",
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
							"2",
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
						"4",
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
						"4",
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
						"4",
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

				config := healthcheck.HealthcheckerConfig{
					AvailableWhenDonor: false,
					AvailableWhenReadOnly: false,
				}

				healthchecker := healthcheck.New(db, config)

				result,msg := healthchecker.Check()

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

				config := healthcheck.HealthcheckerConfig{
					AvailableWhenDonor: false,
					AvailableWhenReadOnly: false,
				}

				healthchecker := healthcheck.New(db, config)

				res,msg := healthchecker.Check()

				Expect(res).To(BeFalse())
				Expect(msg).To(Equal("another test error"))
			})
		})
	})
})

type HealthcheckTestHelperConfig struct {
	wsrep_status string
	read_only string
	available_when_donor bool
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

	config := healthcheck.HealthcheckerConfig{
		AvailableWhenDonor: testConfig.available_when_donor,
		AvailableWhenReadOnly: testConfig.available_when_read_only,
	}

	healthchecker := healthcheck.New(db, config)

	return healthchecker.Check()
}






