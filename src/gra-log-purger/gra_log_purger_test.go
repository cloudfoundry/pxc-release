package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "gra-log-purger"
)

var _ = Describe("gra-log-purger", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "gra-log-test")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(tempDir)
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

	It("requires a pidfile option", func() {
		cmd := exec.Command(
			graLogPurgerBinPath,
			"-graLogDir=some/path/to/datadir",
		)
		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session).Should(gexec.Exit())
		Expect(session.ExitCode()).NotTo(BeZero())
		Expect(session.Err).To(gbytes.Say(`No pidfile supplied`))
	})

	It("manages pid-files", func() {
		cmd := exec.Command(
			graLogPurgerBinPath,
			"-graLogDir="+tempDir,
			"-pidfile="+tempDir+"/gra-log-purger.pid",
		)
		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a pid-file in the specified location", func() {
			Eventually(func() string {
				return tempDir + "/gra-log-purger.pid"
			}, "1m").Should(BeARegularFile())
		})

		By("Removing the pid-file when terminated cleanly", func() {
			session.Terminate()

			Eventually(session).Should(gexec.Exit(0))

			Expect(tempDir + "/gra-log-purger.pid").NotTo(BeAnExistingFile())
		})
	})

	Context("when GRA log files exist in a directory", func() {
		var (
			expectedRetainedFiles []string
		)
		BeforeEach(func() {
			expectedRetainedFiles = setupGraLogFiles(tempDir)
		})

		It("removes only the GRA logs", func() {
			cmd := exec.Command(
				graLogPurgerBinPath,
				"-graLogDir="+tempDir,
				"-graLogDaysToKeep=1",
				"-pidfile=/tmp/gra-log-purger.pid",
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
		})
	})
})

var _ = Describe("FindOldGraLogs", func() {
	var (
		err     error
		tempDir string
	)

	BeforeEach(func() {
		tempDir, err = ioutil.TempDir("", "gra-log-test")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(tempDir)
	})

	Context("When an empty directory is passed in", func() {
		It("returns an empty list and succeeds", func() {
			files, err := FindOldGraLogs(tempDir, time.Hour*24*2)

			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})
	})

	Context("When a file that is not a directory is passed in", func() {
		var (
			tempFileFd *os.File
			tempFile   string
		)

		BeforeEach(func() {
			tempFileFd, err = ioutil.TempFile(tempDir, "not-a-directory")
			Expect(err).NotTo(HaveOccurred())
			tempFile = tempFileFd.Name()
		})

		AfterEach(func() {
			os.Remove(tempFile)
		})

		It("returns an out of sandbox error", func() {
			_, err := FindOldGraLogs(tempFile, time.Hour*24*2)
			Expect(err).To(BeAssignableToTypeOf(&os.SyscallError{}))
		})
	})

	Context("When a directory with GRA files is passed in", func() {
		BeforeEach(func() {
			setupGraLogFiles(tempDir)
		})

		It("returns GRA log files older than the cutoff", func() {
			graLogs, err := FindOldGraLogs(tempDir, time.Hour*24*2)
			Expect(err).NotTo(HaveOccurred())
			Expect(graLogs).To(HaveLen(100))

			for _, log := range graLogs {
				// only returns the OLD files, no NEW and no NOT_A_GRA_LOG
				Expect(log).To(MatchRegexp(`%s/GRA_OLD_.*\.log`, tempDir))
			}
		})

		It("doesn't return nested files", func() {
			tempSubDir, err := ioutil.TempDir(tempDir, "sub-directory")
			Expect(err).NotTo(HaveOccurred())

			// create an OLD GRA log inside the sub directory
			tempGraFileFd, err := ioutil.TempFile(tempSubDir, "GRA_OLD_SUB_DIR_*.log")
			Expect(err).NotTo(HaveOccurred())

			tempGraFile := tempGraFileFd.Name()
			Expect(tempGraFileFd.Close()).To(Succeed())

			fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
			err = os.Chtimes(tempGraFile, fiveDaysAgo, fiveDaysAgo)
			Expect(err).NotTo(HaveOccurred())

			// check that we don't find this file
			graLogs, err := FindOldGraLogs(tempDir, time.Hour*24*2)
			Expect(err).NotTo(HaveOccurred())
			Expect(graLogs).To(HaveLen(100))

			Expect(graLogs).NotTo(ContainElement(tempGraFile))
		})

	})

})

var _ = Describe("DeleteFiles", func() {
	var (
		err     error
		tempDir string
		fileA   *os.File
		fileB   *os.File
	)

	BeforeEach(func() {
		tempDir, err = ioutil.TempDir("", "gra-log-test")
		Expect(err).NotTo(HaveOccurred())

		fileA, err = ioutil.TempFile(tempDir, "A")
		Expect(err).NotTo(HaveOccurred())

		fileB, err = ioutil.TempFile(tempDir, "B")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.Remove(tempDir)
	})

	Context("When an empty list is passed in", func() {
		It("does nothing and succeeds", func() {
			deleted, failed := DeleteFiles(nil)
			Expect(deleted).To(BeZero())
			Expect(failed).To(BeZero())
		})
	})

	It("deletes all the file paths passed in", func() {
		deleted, failed := DeleteFiles([]string{
			fileA.Name(),
			fileB.Name(),
		})

		Expect(deleted).To(Equal(2))
		Expect(failed).To(BeZero())
	})

	Context("When a non-existent path is passed in", func() {
		It("deletes the valid paths and counts the failure", func() {
			deleted, failed := DeleteFiles([]string{
				fileA.Name(),
				"FAKE-FILE",
				fileB.Name(),
			})

			Expect(deleted).To(Equal(2))
			Expect(failed).To(Equal(1))
		})
	})
})

func setupGraLogFiles(path string) (pathsToRetain []string) {
	// mysql datadir fixtures
	mysqlFixtures := []string{
		filepath.Join(path, "ibdata1"),
		filepath.Join(path, "ib_logfile0"),
		filepath.Join(path, "ib_logfile1"),
		filepath.Join(path, "mysql-bin.000001"),
		filepath.Join(path, "mysql-bin.index"),
	}

	for _, name := range mysqlFixtures {
		Expect(ioutil.WriteFile(name, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(name, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
		Expect(os.Chtimes(name, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
	}

	pathsToRetain = append(pathsToRetain, mysqlFixtures...)

	// old GRA logs
	for i := 0; i < 100; i++ {
		graLogPath := filepath.Join(path, fmt.Sprintf("GRA_OLD_%d.log", i))

		Expect(ioutil.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(graLogPath, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())
	}

	// new GRA logs
	for i := 0; i < 100; i++ {
		graLogPath := filepath.Join(path, fmt.Sprintf("GRA_NEW_%d.log", i))

		Expect(ioutil.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		oneHourAgo := time.Now().Add(-time.Hour)
		Expect(os.Chtimes(graLogPath, oneHourAgo, oneHourAgo)).
			To(Succeed())

		pathsToRetain = append(pathsToRetain, graLogPath)
	}

	// non GRA logs
	for i := 0; i < 100; i++ {

		graLogPath := filepath.Join(path, fmt.Sprintf("NOT_A_GRA_%d.log", i))

		Expect(ioutil.WriteFile(graLogPath, nil, 0640)).
			To(Succeed())

		fiveDaysAgo := time.Now().Add(-time.Hour * 24 * 5)
		Expect(os.Chtimes(graLogPath, fiveDaysAgo, fiveDaysAgo)).
			To(Succeed())

		pathsToRetain = append(pathsToRetain, graLogPath)
	}

	return pathsToRetain
}
