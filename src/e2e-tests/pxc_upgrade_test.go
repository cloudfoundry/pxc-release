package e2e_tests

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

var _ = Describe("Legacy upgrades from existing Percona XtraDB Cluster 5.7 deployments", Label("legacy-upgrade"), Ordered, func() {
	It("rejects the upgrade", func() {
		deploymentName := "pxc57-upgrade-" + uuid.New().String()

		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}
			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		By("deploying pxc-release with mysql_version=5.7")
		Expect(bosh.Deploy(deploymentName, "src/e2e-tests/manifest/pxc-5.7.yml",
			bosh.Var("deployment_name", deploymentName),
			bosh.VarsEnv("BOSH_VAR"),
		)).To(Succeed())

		By("upgrading to mysql_version=" + expectedMysqlVersion)
		Expect(bosh.Deploy(deploymentName, "src/e2e-tests/manifest/pxc-5.7.yml",
			bosh.Var("deployment_name", deploymentName),
			bosh.VarsEnv("BOSH_VAR"),
			bosh.Operation(`mysql-version.yml`),
			bosh.Var("mysql_version", expectedMysqlVersion),
			bosh.Operation("dev-release.yml"),
		)).NotTo(Succeed(), `Expected the upgrade to fail but it did not!`)

		By("verifying the upgrade failed for the right reasons")
		mysqlPrestartOutput, err := bosh.Logs(deploymentName, "mysql/0", "pxc-mysql/pre-start.stdout.log")
		Expect(err).NotTo(HaveOccurred())
		Expect(mysqlPrestartOutput.String()).Should(ContainSubstring(`MySQL 5.7 reached EOL in October 2023 and direct upgrades from MySQL v5.7 are no longer supported.`))
	})
})
