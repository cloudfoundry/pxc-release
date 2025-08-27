package main_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/cloudfoundry/gra-log-purger"
)

var _ = Describe("gra-log-purger", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "gra-log-test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
		})
	})

	It("requires a graLogDir option", func() {
		cmd := exec.Command(
			graLogPurgerBinPath,
		)
		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session).Should(gexec.Exit())
		Expect(session.ExitCode()).NotTo(BeZero())
		Expect(session.Err).To(gbytes.Say(`No gra log directory supplied`))
	})

	It("validates graLogDaysToKeep is not less than 0", func() {
		cmd := exec.Command(
			graLogPurgerBinPath,
			"-graLogDir="+tempDir,
			"-graLogDaysToKeep=-1",
		)
		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session).Should(gexec.Exit())
		Expect(session.ExitCode()).NotTo(BeZero())
		Expect(session.Err).To(gbytes.Say(`graLogDaysToKeep should be >= 0`))
	})

	When("GRA log files exist in a directory", func() {
		var (
			expectedRetainedFiles []string
		)
		BeforeEach(func() {
			expectedRetainedFiles = setupGraLogFiles(tempDir, 2048)
		})

		It("removes only the GRA logs", func() {
			cmd := exec.Command(
				graLogPurgerBinPath,
				"-graLogDir="+tempDir,
				"-graLogDaysToKeep=1",
				"-timeFormat=rfc3339",
			)
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() ([]string, error) {
				f, err := os.Open(tempDir)
				if err != nil {
					return nil, err
				}

				names, err := f.Readdirnames(-1)
				if err != nil {
					return nil, err
				}

				for i, name := range names {
					names[i] = filepath.Join(tempDir, name)
				}

				return names, nil
			}, "10s").Should(ConsistOf(expectedRetainedFiles))

			session.Terminate()
			Eventually(session).Should(gexec.Exit(0))

			Eventually(func() ([]string, error) {
				f, err := os.Open(tempDir)
				if err != nil {
					return nil, err
				}

				names, err := f.Readdirnames(-1)
				if err != nil {
					return nil, err
				}

				for i, name := range names {
					names[i] = filepath.Join(tempDir, name)
				}

				return names, nil
			}, "1s").Should(ConsistOf(expectedRetainedFiles))

			Eventually(session.Out).Should(gbytes.Say(`\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z] - Deleted 2048 files, failed to delete 0 files`))
			Eventually(session.Out).Should(gbytes.Say(`\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z] - Sleeping for one hour`))
			Eventually(session.Out).Should(gbytes.Say(`\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z] - terminated`))
		})
	})
})

var _ = Describe("PurgeGraLogs", func() {
	var (
		err        error
		tempDir    string
		timeFormat string
	)

	BeforeEach(func() {
		timeFormat = "rfc3339"

		tempDir, err = os.MkdirTemp("", "gra-log-test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
		})
	})

	When("an empty directory is passed in", func() {
		It("returns an empty list and succeeds", func() {
			succeeded, failed, err := PurgeGraLogs(tempDir, timeFormat, time.Hour*24*2)

			Expect(err).NotTo(HaveOccurred())
			Expect(succeeded).To(BeZero())
			Expect(failed).To(BeZero())
		})
	})

	When("a directory matches a GRAlog filename", func() {
		BeforeEach(func() {
			Expect(os.Mkdir(filepath.Join(tempDir, "GRA_something.log"), 0755)).To(Succeed())
		})

		It("does not remove directories", func() {
			_, _, err := PurgeGraLogs(tempDir, timeFormat, 0)

			Expect(err).NotTo(HaveOccurred())
			Expect(filepath.Join(tempDir, "GRA_something.log")).Should(BeADirectory())
		})
	})

	When("a file that is not a directory is passed in", func() {
		var (
			tempFileFd *os.File
			tempFile   string
		)

		BeforeEach(func() {
			tempFileFd, err = os.CreateTemp(tempDir, "not-a-directory")
			Expect(err).NotTo(HaveOccurred())
			tempFile = tempFileFd.Name()
		})

		AfterEach(func() {
			_ = os.Remove(tempFile)
		})

		It("returns an error", func() {
			_, _, err := PurgeGraLogs(tempFile, timeFormat, time.Hour*24*2)
			Expect(err).To(HaveOccurred())

			var pathError *fs.PathError
			Expect(errors.As(err, &pathError)).To(BeTrue(), "should return fs.PathError, but got %#v", err)
		})
	})
})

func setupGraLogFiles(path string, numberOfGraLogs int) (pathsToRetain []string) {
	// mysql datadir fixtures
	mysqlFixtures := []string{
		filepath.Join(path, "ibdata1"),
		filepath.Join(path, "ib_logfile0"),
		filepath.Join(path, "ib_logfile1"),
		filepath.Join(path, "mysql-bin.000001"),
		filepath.Join(path, "mysql-bin.index"),
	}

	for _, name := range mysqlFixtures {
		Expect(os.WriteFile(name, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(name, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
		Expect(os.Chtimes(name, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
	}

	pathsToRetain = append(pathsToRetain, mysqlFixtures...)

	// old GRA logs
	for i := 0; i < numberOfGraLogs; i++ {
		graLogPath := filepath.Join(path, fmt.Sprintf("GRA_OLD_%d.log", i))

		Expect(os.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(graLogPath, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
	}

	// new GRA logs
	for i := 0; i < 100; i++ {
		graLogPath := filepath.Join(path, fmt.Sprintf("GRA_NEW_%d.log", i))

		Expect(os.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		oneHourAgo := time.Now().Add(-time.Hour)
		Expect(os.Chtimes(graLogPath, oneHourAgo, oneHourAgo)).
			To(Succeed())

		pathsToRetain = append(pathsToRetain, graLogPath)
	}

	// non GRA logs
	for i := 0; i < 100; i++ {

		graLogPath := filepath.Join(path, fmt.Sprintf("NOT_A_GRA_%d.log", i))

		Expect(os.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(graLogPath, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())

		pathsToRetain = append(pathsToRetain, graLogPath)
	}

	return pathsToRetain
}
