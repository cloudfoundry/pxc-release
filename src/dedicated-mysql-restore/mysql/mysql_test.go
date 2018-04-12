package mysql_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"

	"dedicated-mysql-restore/mysql"

	"dedicated-mysql-restore/executable/executablefakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mysql", func() {
	var (
		mockExecutor *executablefakes.FakeExecutable
	)
	BeforeEach(func() {
		mockExecutor = &executablefakes.FakeExecutable{}
		mysql.Execer = mockExecutor
	})

	Context("DeleteBindingUsers", func() {
		It("Deletes cf binding users and the bindings table", func() {
			Expect(mysql.DeleteBindingUsers()).To(Succeed())
			Expect(mockExecutor.RunCallCount()).To(Equal(1))
			Expect(mockExecutor.RunArgsForCall(0).Args).To(ConsistOf(
				"/var/vcap/jobs/mysql/bin/mysql_ctl",
				"start",
				"--skip-daemonize",
				"--bootstrap",
			))

			contents, err := ioutil.ReadAll(mockExecutor.RunArgsForCall(0).Stdin)
			Expect(err).NotTo(HaveOccurred())
			contents = bytes.TrimSpace(contents)
			Expect(strings.Split(string(contents), "\n")).To(
				ConsistOf(
					`DELETE FROM mysql.user WHERE User <> 'mysql.sys';`,
					`DELETE FROM mysql.db WHERE User <> 'mysql.sys';`,
					`DROP TABLE IF EXISTS cf_metadata.bindings;`,
				))
		})

		It("Returns error when deleting binding users fails", func() {
			mockExecutor.RunReturnsOnCall(0, fmt.Errorf("mysqld bootstrap failed"))
			err := mysql.DeleteBindingUsers()
			Expect(err).To(MatchError("mysqld bootstrap failed"))
		})
	})
})
