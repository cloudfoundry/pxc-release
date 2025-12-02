package e2e_tests

import (
	"fmt"
	"slices"
	"time"

	"e2e-tests/utilities/bosh"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy healthcheck", Ordered, Label("proxy", "healthchecks"), func() {
	var (
		deploymentName string
		proxyIPs       []string
		proxies        []bosh.Instance
		numProxies     int
		err            error
	)

	BeforeAll(func() {
		deploymentName = "pxc-proxy-healthcheck-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`iaas/cluster.yml`),
			bosh.Operation(`require-tls.yml`),
		)).To(Succeed())

		proxies, err = bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		proxyIPs, err = bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		numProxies = len(proxyIPs)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}
		Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	It("deploys proxy healthcheck scripts", func() {
		_, err := bosh.RemoteCommand(deploymentName, "proxy",
			"ls -l /var/vcap/jobs/proxy/bin/dns/healthy")
		Expect(err).NotTo(HaveOccurred(), "deployment missing expected healthcheck scripts")
	})

	It("reports all proxies healthy", func() {
		var healthyProxyIPs []string
		Eventually(
			func() int {
				healthyProxyIPs, err = bosh.InterrogateDNS(deploymentName, "mysql/0", "proxy", bosh.DnsHealthy)
				if err != nil {
					return -1
				}
				return len(healthyProxyIPs)
			}).WithTimeout(time.Second*30).WithPolling(time.Second*2).Should(Equal(numProxies), fmt.Sprintf("expected %d healthy proxies, got %d", numProxies, len(healthyProxyIPs)))
	})

	When("one proxy is stopped", func() {
		var stoppedIP string
		BeforeAll(func() {
			pauseProxy(deploymentName, "proxy/0")

			// recover proxy/0 IP for later dns lookup checks
			stoppedIndex := slices.IndexFunc(proxies, func(s bosh.Instance) bool { return s.Index == "0" })
			Expect(stoppedIndex).NotTo(Equal(-1), "unable to retrieve proxy/0 IP address")
			stoppedIP = proxies[stoppedIndex].IP
		})
		AfterAll(func() {
			resumeProxy(deploymentName, "proxy/0")
		})

		It("appears unhealthy to bosh DNS", func() {
			Eventually(
				func() error {
					downProxyIPs, err := bosh.InterrogateDNS(deploymentName,
						"mysql/0", "proxy", bosh.DnsUnhealthy)
					if err != nil {
						return err
					}
					if len(downProxyIPs) != 1 {
						return fmt.Errorf("expected single unhealthy proxy but got %d", len(downProxyIPs))
					}
					if downProxyIPs[0] != stoppedIP {
						return fmt.Errorf("expected unhealthy proxy IP %s but got IP %s", stoppedIP, downProxyIPs[0])
					}
					return nil
				}).WithTimeout(time.Second * 30).WithPolling(time.Second * 2).Should(Succeed())
		})

		It("other running proxies appear healthy to bosh DNS", func() {
			upProxyIPs, err := bosh.InterrogateDNS(deploymentName,
				"mysql/0", "proxy", bosh.DnsHealthy)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(upProxyIPs)).To(Equal(numProxies-1), "unexpected number of healthy proxies")
			Expect(slices.Contains(upProxyIPs, stoppedIP)).To(BeFalse(), "healthy proxies unexpectedly contain stopped proxy")
		})
	})
})

// Halt the process "proxy" running on the provided instance
func pauseProxy(deploymentName, instance string) {
	GinkgoHelper()

	pid, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo pgrep proxy")
	Expect(err).NotTo(HaveOccurred())

	_, err = bosh.RemoteCommand(deploymentName, instance,
		"sudo kill -SIGSTOP "+pid)
	Expect(err).NotTo(HaveOccurred())
}
func resumeProxy(deploymentName, instance string) {
	GinkgoHelper()

	pid, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo pgrep proxy")
	Expect(err).NotTo(HaveOccurred())

	_, err = bosh.RemoteCommand(deploymentName, instance,
		"sudo kill -SIGCONT "+pid)
	Expect(err).NotTo(HaveOccurred())
}
