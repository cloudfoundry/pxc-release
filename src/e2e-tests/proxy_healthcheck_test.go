package e2e_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
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
		deploymentName string
		proxyIPs       []string
		proxies        []bosh.Instance
		numProxies     int
		proxyDNSQuery  string
	)

	BeforeAll(func() {
		deploymentName = "pxc-proxy-healthcheck-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`iaas/cluster.yml`),
			bosh.Operation(`require-tls.yml`),
			bosh.Operation(`mysql-version.yml`),
			bosh.Var("mysql_version", "8.4"),
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
		proxyIPs, err = bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		numProxies = len(proxyIPs)

		proxyDNSQuery = boshDNSAddress(deploymentName, "proxy")
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
				healthyProxyIPs = interrogateDNS(deploymentName, "mysql/0", proxyDNSQuery)
				return len(healthyProxyIPs)
			}).WithTimeout(time.Second*30).WithPolling(time.Second*2).Should(Equal(numProxies), fmt.Sprintf("expected %d healthy proxies, got %d", numProxies, len(healthyProxyIPs)))
	})

	When("each proxy is temporarily unhealthy", func() {
		DescribeTable("app connections bypass unhealthy proxies",
			func(proxyID string) {

				By("temporarily pausing proxy " + proxyID)
				pausedProxyInstance := "proxy/" + proxyID
				pauseProxy(deploymentName, pausedProxyInstance)
				DeferCleanup(func() {
					resumeProxy(deploymentName, pausedProxyInstance)
				})

				By("seeing DNS exclude that proxy from the healthy proxies")
				var upProxyIPs, expectedProxyIPs []string
				pausedIndex := slices.IndexFunc(proxies, func(s bosh.Instance) bool { return s.Index == proxyID })
				Expect(pausedIndex).NotTo(Equal(-1), "unable to retrieve proxy/0 IP address")
				pausedIP := proxies[pausedIndex].IP
				for _, proxyIP := range proxyIPs {
					if proxyIP == pausedIP {
						continue
					}
					expectedProxyIPs = append(expectedProxyIPs, proxyIP)
				}
				Eventually(func() []string {
					upProxyIPs = interrogateDNS(deploymentName, "mysql/0", proxyDNSQuery)
					return upProxyIPs
				}).WithTimeout(time.Second*30).WithPolling(time.Second*2).Should(ConsistOf(expectedProxyIPs),
					fmt.Sprintf("expected DNS query to return expected proxy IPs %v, instead got %v", expectedProxyIPs, upProxyIPs))

				By("ensuring bosh DNS bypasses that unhealthy proxy for incoming connections")
				Expect(bosh.RunErrand(deploymentName, "smoke-tests",
					"mysql/0")).To(Succeed(),
					"smoke-tests unexpectedly failed while a proxy was still available")
			},
			Entry("when pausing proxy/0", "0"),
			Entry("when pausing proxy/1", "1"),
		)
	})
})

// Halt the process "proxy" running on the provided instance
func pauseProxy(deploymentName, instance string) {
	GinkgoHelper()
	_, err := bosh.RemoteCommand(deploymentName, instance,
		"sudo pkill -SIGSTOP proxy")
	Expect(err).NotTo(HaveOccurred())
}
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

func boshDNSAddress(deploymentName, jobName string) string {
	GinkgoHelper()

	// Get all links
	var deploymentLinks []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	Expect(boshCurl("/links?deployment="+deploymentName, &deploymentLinks)).To(Succeed())

	// Get first proxy link
	var targetLinkID string
	for _, link := range deploymentLinks {
		// Get ID of our target job's first link
		if link.Name == jobName {
			targetLinkID = link.ID
			break
		}
	}
	Expect(targetLinkID).NotTo(BeEmpty(), "link ID not detected for job %s", jobName)

	// Get links address
	var address struct {
		Address string `json:"address"`
	}

	Expect(boshCurl("/link_address?link_id="+targetLinkID, &address)).To(Succeed())

	return address.Address
}

func boshCurl(endpoint string, target any) error {
	var output bytes.Buffer
	cmd := exec.Command("bosh", "curl", endpoint)
	cmd.Stdout = &output
	cmd.Stderr = GinkgoWriter
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote boshCurl failed: %w", err)
	}

	if err := json.Unmarshal(output.Bytes(), target); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	return nil
}
