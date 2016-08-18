package preparer_test

import (
	"flag"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/preparer"
	"github.com/pivotal-cf-experimental/service-config"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Preparer", func() {

	var prep preparer.Preparer
	var testLogger lager.Logger

	BeforeEach(func() {
		var rootConfig config.Config
		var serviceConfig *service_config.ServiceConfig

		serviceConfig = service_config.New()
		flags := flag.NewFlagSet("mariadb_ctrl", flag.ExitOnError)
		serviceConfig.AddFlags(flags)

		serviceConfig.AddDefaults(config.Config{
			Db: config.DBHelper{
				User: "root",
			},
		})

		flags.Parse([]string{
			"-configPath=../example-config.yml",
		})

		err := serviceConfig.Read(&rootConfig)
		Expect(err).NotTo(HaveOccurred())
		testLogger = lagertest.NewTestLogger("preparer")
		prep = preparer.New(testLogger, rootConfig)
	})

	Context("Happy path", func() {
		It("instantiates all the objects", func() {
			prep.Prepare()
			Expect(prep.GetOsHelper()).ToNot(Equal(nil))
			Expect(prep.GetDBHelper()).ToNot(Equal(nil))
			Expect(prep.GetUpgrader()).ToNot(Equal(nil))
			Expect(prep.GetHealthChecker()).ToNot(Equal(nil))
			Expect(prep.GetStartManager()).ToNot(Equal(nil))
			Expect(prep.GetRunner()).ToNot(Equal(nil))
		})
	})
})
