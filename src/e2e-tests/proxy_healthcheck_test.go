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
		deploymentName = "pxc-proxy-healthcheck" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`iaas/cluster.yml`),
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

	It("reports all proxies as healthy", func() {
		healthyList, err := bosh.InterrogateDNS(deploymentName, "mysql/0", "proxy", bosh.DnsHealthy)
		Expect(err).NotTo(HaveOccurred(), "error checking dns proxy health")
		Expect(healthyList).To(HaveLen(numProxies))
	})

	When("one proxy is stopped", func() {
		var stoppedProxyIP string
		BeforeAll(func() {
			_, err := bosh.RemoteCommand(deploymentName, "proxy/0",
				"sudo monit stop proxy")
			Expect(err).NotTo(HaveOccurred())

			// recover proxy/0 IP for later dns lookup checks
			stoppedIndex := slices.IndexFunc(proxies, func(s bosh.Instance) bool { return s.Index == "0" })
			Expect(stoppedIndex).NotTo(Equal(-1), "unable to retrieve proxy/0 IP address")
			stoppedProxyIP = proxies[stoppedIndex].IP
		})
		AfterAll(func() {
			_, err := bosh.RemoteCommand(deploymentName, "proxy/0",
				"sudo monit start proxy")
			Expect(err).NotTo(HaveOccurred())
		})
		It("appears unhealthy to bosh DNS", func() {
			Eventually(
				func() error {
					downProxyIPs, err := bosh.InterrogateDNS(deploymentName,
						"mysql/0", "proxy", bosh.DnsUnhealthy)
					if err != nil {
						return err
					}
					if len(downProxyIPs) == 1 {
						if downProxyIPs[0] == stoppedProxyIP {
							return nil
						}
						return fmt.Errorf("expected unhealthy proxy %s, got %s", stoppedProxyIP, downProxyIPs[0])
					}
					return fmt.Errorf("expected single unhealthy proxy but got %d", len(downProxyIPs))
				}).WithTimeout(time.Second * 20).WithPolling(time.Second * 2).Should(Succeed())
		})
		It("other running proxies appear healthy to bosh DNS", func() {
			upProxyIPs, err := bosh.InterrogateDNS(deploymentName,
				"mysql/0", "proxy", bosh.DnsHealthy)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(upProxyIPs)).To(Equal(numProxies-1), "unexpected number of healthy proxies")
			Expect(slices.Contains(upProxyIPs, stoppedProxyIP)).To(BeFalse(), "healthy proxies unexpectedly contain stopped proxy")
		})

		// TODO: "When back-end MySQL is hung but (somehow) appears healthy, proxies report healthy

	})

})
