package autotune_test

import (
	"math"

	"gopkg.in/yaml.v2"

	helpers "specs/test_helpers"
	"strconv"

	boshdir "github.com/cloudfoundry/bosh-cli/director"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func deployWithBufferPoolSizePercent(bufferPoolSizePercent int) {
	director, err := helpers.BuildBoshDirector()
	Expect(err).NotTo(HaveOccurred())

	deployment, err := director.FindDeployment(helpers.BoshDeploymentName())
	Expect(err).NotTo(HaveOccurred())

	manifestString, err := deployment.Manifest()
	Expect(err).NotTo(HaveOccurred())

	var manifest map[string]interface{}
	err = yaml.Unmarshal([]byte(manifestString), &manifest)
	Expect(err).NotTo(HaveOccurred())

	instanceGroups := manifest["instance_groups"].([]interface{})
	for _, instanceGroup := range instanceGroups {
		if instanceGroup.(map[interface{}]interface{})["name"] == "mysql" {
			jobs := instanceGroup.(map[interface{}]interface{})["jobs"].([]interface{})
			for _, job := range jobs {
				if job.(map[interface{}]interface{})["name"] == "pxc-mysql" {
					properties := job.(map[interface{}]interface{})["properties"]
					engineConfig := properties.(map[interface{}]interface{})["engine_config"]
					engineConfig.(map[interface{}]interface{})["innodb_buffer_pool_size_percent"] = bufferPoolSizePercent
					break
				}
			}
			break
		}
	}

	updatedManifest, err := yaml.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())

	err = deployment.Update(updatedManifest, boshdir.UpdateOpts{})
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("CF PXC MySQL Autotune", func() {
	It("correctly configures innodb_buffer_pool_size", func() {
		var bufferPoolSizePercent = 14
		deployWithBufferPoolSizePercent(bufferPoolSizePercent)

		director, err := helpers.BuildBoshDirector()
		Expect(err).NotTo(HaveOccurred())

		deployment, err := director.FindDeployment(helpers.BoshDeploymentName())
		Expect(err).NotTo(HaveOccurred())

		var mysqlVm boshdir.VMInfo
		vmInfos, _ := deployment.VMInfos()
		for _, vmInfo := range vmInfos {
			if vmInfo.JobName == "mysql" {
				mysqlVm = vmInfo
				break
			}
		}

		vmUsedMemInKb, err := strconv.Atoi(mysqlVm.Vitals.Mem.KB)
		Expect(err).NotTo(HaveOccurred())
		vmUsedMemPercent, err := strconv.Atoi(mysqlVm.Vitals.Mem.Percent)
		Expect(err).NotTo(HaveOccurred())

		vmTotalMemoryInMB := float64(vmUsedMemInKb / vmUsedMemPercent * 100 / 1024)
		var variableName, variableValue string

		query := "SHOW variables LIKE 'innodb_buffer_pool_size'"
		rows, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&variableName, &variableValue)
		innodbBufferPoolSizeInBytes, err := strconv.Atoi(variableValue)
		Expect(err).NotTo(HaveOccurred())

		innodbBufferPoolSizeInMb := innodbBufferPoolSizeInBytes / 1024 / 1024

		expectedBufferPoolSize := vmTotalMemoryInMB * (float64(bufferPoolSizePercent) / 100.0)
		if expectedBufferPoolSize > 1024 {
			expectedBufferPoolSize = math.Ceil(expectedBufferPoolSize/1024) * 1024
		} else {
			expectedBufferPoolSize = math.Ceil(expectedBufferPoolSize/128) * 128
		}

		Expect(innodbBufferPoolSizeInMb).To(Equal(int(expectedBufferPoolSize)))
	})

})
