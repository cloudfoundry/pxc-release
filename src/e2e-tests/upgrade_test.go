package e2e_tests

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Upgrade", Label("upgrade"), func() {
	It("can upgrade from pxc-release 5.7 to pxc 8.0", func() {
		deploymentName := "pxc-upgrade-" + uuid.New().String()

		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}
			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		By("deploying pxc-release 0.x based on PXC 5.7")
		Expect(bosh.Deploy(deploymentName, "manifest/pxc-5.7.yml",
			bosh.Var("deployment_name", deploymentName),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		By("forcing credhub to rotate the galera-agent user's database credential")
		Expect(credhub.Regenerate("/" + deploymentName + "/cf_mysql_mysql_galera_healthcheck_db_password")).
			To(Succeed())

		By("upgrading pxc-release based on PXC 8.0")
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("dev-release.yml"),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())
	})
})
