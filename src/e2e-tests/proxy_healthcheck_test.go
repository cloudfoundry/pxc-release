package e2e_tests

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"e2e-tests/utilities/bosh"
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

	// bosh-dns takes up to N seconds to report its monitoring results.
	It("begins monitoring proxy health", func() {
		testStartTime := time.Now()
		Eventually(
			func() error {
				boshDnsLogs, err := bosh.Logs(deploymentName, "proxy/0", "bosh-dns/bosh_dns_health.stdout.log")
				if err != nil {
					return fmt.Errorf("error recovering proxy bosh_dns_health logs: %w", err)
				}

				proxyMonitored, err := ObserveProxyMonitoring(boshDnsLogs)
				if err != nil {
					return fmt.Errorf("error recovering proxy monitoring logs: %w", err)
				}
				if !proxyMonitored {
					return fmt.Errorf("proxy monitoring not observed")
				}
				return nil
			}).WithTimeout(time.Minute * 5).WithPolling(time.Second * 5).Should(Succeed())

		By(fmt.Sprintf("proxy health checks active after %s", time.Since(testStartTime)))
	})

	It("patiently waits until it can report all proxies as healthy", func() {
		healthyList, err := bosh.InterrogateDNS(deploymentName, "mysql/0", "proxy", bosh.DnsHealthy)
		Expect(err).NotTo(HaveOccurred(), "error checking dns proxy health")

		testStartTime := time.Now()
		Eventually(
			func() int {

				healthyList, err := bosh.InterrogateDNS(deploymentName, "mysql/0", "proxy", bosh.DnsHealthy)
				if err != nil {
					return -1
				}
				return len(healthyList)
			}).WithTimeout(time.Minute*5).WithPolling(time.Second*2).Should(Equal(numProxies), fmt.Sprintf("expected %d healthy proxies, got %d", numProxies, len(healthyList)))

		By(fmt.Sprintf("bosh dns reports proxies healthy after %s", time.Since(testStartTime)))
	})

	It("reports all proxies as healthy", func() {
		healthyList, err := bosh.InterrogateDNS(deploymentName, "mysql/0", "proxy", bosh.DnsHealthy)
		Expect(err).NotTo(HaveOccurred(), "error checking dns proxy health")
		Expect(healthyList).To(HaveLen(numProxies), fmt.Sprintf("expected %d proxies got %d", numProxies, len(healthyList)))
	})

	When("one proxy is stopped", func() {
		var stoppedProxyIP string
		BeforeAll(func() {
			stopProxy(deploymentName, "proxy/0")

			// recover proxy/0 IP for later dns lookup checks
			stoppedIndex := slices.IndexFunc(proxies, func(s bosh.Instance) bool { return s.Index == "0" })
			Expect(stoppedIndex).NotTo(Equal(-1), "unable to retrieve proxy/0 IP address")
			stoppedProxyIP = proxies[stoppedIndex].IP
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
					By(fmt.Sprintf("*** observing %d down proxies", len(downProxyIPs)))

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
			By(fmt.Sprintf("*** observing %d up proxies", len(upProxyIPs)))
			Expect(len(upProxyIPs)).To(Equal(numProxies-1), "unexpected number of healthy proxies")
			Expect(slices.Contains(upProxyIPs, stoppedProxyIP)).To(BeFalse(), "healthy proxies unexpectedly contain stopped proxy")
		})

		// TODO: "When back-end MySQL is hung but (somehow) appears healthy, proxies report healthy

	})

})

func ObserveProxyMonitoring(boshDnsLogs *bytes.Buffer) (bool, error) {
	pattern := `Monitored jobs:.*HealthExecutablePath:/var/vcap/jobs/proxy/bin/dns/healthy.*JobName:proxy`
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}

	// Create a scanner to read the buffer line by line
	scanner := bufio.NewScanner(boshDnsLogs)

	// Scan through each line
	for scanner.Scan() {
		line := scanner.Text()

		// Check if line contains "Monitored jobs" (case-sensitive)
		if strings.Contains(line, "Monitored jobs") {
			// Validate the line matches the expected regex pattern
			if regex.MatchString(line) {
				return true, nil
			}
			// If we found "Monitored jobs" but it doesn't match pattern, return false
			return false, nil
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return false, err
	}

	// No "Monitored jobs" line found
	return false, nil
}

func stopProxy(deploymentName, instance string) {
	GinkgoHelper()
	pid, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo ps ax | grep proxy | grep -v grep | grep -v tini | awk '{print $1}'")

	Expect(err).NotTo(HaveOccurred())
	_, err = bosh.RemoteCommand(deploymentName, instance,
		"sudo kill -SIGSTOP "+pid)
	Expect(err).NotTo(HaveOccurred())
}
func resumeProxy(deploymentName, instance string) {
	GinkgoHelper()
	pid, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo ps ax | grep proxy | grep -v grep | grep -v tini | awk '{print $1}'")

	Expect(err).NotTo(HaveOccurred())
	_, err = bosh.RemoteCommand(deploymentName, instance,
		"sudo kill -SIGCONT "+pid)
	Expect(err).NotTo(HaveOccurred())
}
