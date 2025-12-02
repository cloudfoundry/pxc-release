package e2e_tests

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

var _ = Describe("Proxy healthcheck", Ordered, Label("proxy", "healthchecks"), func() {
	var (
		deploymentName  string
		proxyDnsAddress string
		proxies         []bosh.Instance
		proxyIPs        []string
	)
	BeforeAll(func() {
		deploymentName = "pxc-proxy-healthcheck-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`iaas/cluster.yml`),
			bosh.Operation(`require-tls.yml`),
			bosh.Operation(`test/proxy-dns-alias.yml`),
		)).To(Succeed())

		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}
			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		var err error
		proxies, err = bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())

		for _, proxy := range proxies {
			proxyIPs = append(proxyIPs, proxy.IP)
		}

		proxyDnsAddress = "proxy." + deploymentName + ".mysql.internal"
	})

	It("reports all proxies healthy", func() {
		var healthyProxyIPs []string
		Eventually(func() []string {
			healthyProxyIPs = interrogateDNS(deploymentName, "mysql/0", proxyDnsAddress)
			return healthyProxyIPs
		}).WithTimeout(time.Second*30).WithPolling(time.Second*2).Should(ConsistOf(proxyIPs), "expected to discover all proxy IPS, but did not")
	})

	When("one proxy is stopped", func() {
		var stoppedIP string
		BeforeAll(func() {
			pauseProxy(deploymentName, "proxy/0")
			DeferCleanup(func() {
				resumeProxy(deploymentName, "proxy/0")
			})

			// recover proxy/0 IP for later dns lookup checks
			stoppedIndex := slices.IndexFunc(proxies, func(s bosh.Instance) bool { return s.Index == "0" })
			Expect(stoppedIndex).NotTo(Equal(-1), "unable to retrieve proxy/0 IP address")
			stoppedIP = proxies[stoppedIndex].IP
		})

		It("removes the unhealthy proxy from DNS checks", func() {
			var expectedProxyIPs []string
			for _, proxyIP := range proxyIPs {
				if proxyIP == stoppedIP {
					continue
				}
				expectedProxyIPs = append(expectedProxyIPs, proxyIP)
			}

			var upProxyIPs []string
			Eventually(func() []string {
				upProxyIPs = interrogateDNS(deploymentName, "mysql/0", proxyDnsAddress)
				return upProxyIPs
			}).WithTimeout(time.Second*30).WithPolling(time.Second*2).Should(ConsistOf(expectedProxyIPs), "expected to discover all proxy IPS, but did not")
		})
	})
})

// Halt the "proxy" process running on the provided instance
func pauseProxy(deploymentName, instance string) {
	GinkgoHelper()
	_, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo pkill -SIGSTOP proxy")
	Expect(err).NotTo(HaveOccurred())
}

// Unhalt the "proxy" process running on the provided instance
func resumeProxy(deploymentName, instance string) {
	GinkgoHelper()

	_, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo pkill -SIGCONT proxy")
	Expect(err).NotTo(HaveOccurred())
}

func interrogateDNS(deploymentName, sourceInstance, dnsQuery string) []string {
	GinkgoHelper()
	output, err := bosh.RemoteCommand(deploymentName, sourceInstance, "dig +short "+dnsQuery)
	Expect(err).NotTo(HaveOccurred(), "remote dig lookup %s failed: %w\n output: %s", dnsQuery, err, output)
	return strings.Fields(output)
}
