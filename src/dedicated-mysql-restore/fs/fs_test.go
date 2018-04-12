package fs_test

import (
	"io/ioutil"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"dedicated-mysql-restore/fs"
)

var _ = Describe("Fs", func() {
	var (
		dir     string
		tmpFile *os.File
	)

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "fs_test_")
		Expect(err).NotTo(HaveOccurred())

		tmpFile, err = ioutil.TempFile(dir, "fs_test_")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(dir)
	})

	Describe("Chown", func() {
		BeforeEach(func() {
			if os.Getuid() != 0 {
				Skip("you must have root access to run this test.")
			}
		})

		It("changes the user of a file", func() {
			Expect(
				fs.Chown(tmpFile.Name(), "nobody"),
			).To(Succeed())

			output, err := exec.Command("ls", "-l", tmpFile.Name()).Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("nobody"))
		})

		It("fails with an invalid username", func() {
			Expect(
				fs.Chown(tmpFile.Name(), "does-not-exist"),
			).ToNot(Succeed())
		})

		It("fails with an invalid path", func() {
			Expect(
				fs.Chown("/path/that/does/not/exist", "does-not-exist"),
			).ToNot(Succeed())
		})
	})

	Context("RecursiveChown", func() {
		BeforeEach(func() {
			if os.Getuid() != 0 {
				Skip("you must have root access to run this test.")
			}
		})

		It("recursively changes the owner of every item in a directory", func() {
			Expect(
				fs.RecursiveChown(dir, "nobody"),
			).To(Succeed())

			output, err := exec.Command("ls", "-ld", dir).Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("nobody"))

			output, err = exec.Command("ls", "-l", tmpFile.Name()).Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("nobody"))
		})

		It("fails with an invalid username", func() {
			Expect(
				fs.RecursiveChown(dir, "does-not-exist"),
			).ToNot(Succeed())
		})

		It("fails with an invalid path", func() {
			Expect(
				fs.RecursiveChown("/path/that/does/not/exist", "does-not-exist"),
			).ToNot(Succeed())
		})
	})

	Context("CleanDirectory", func() {
		It("removes the contents of a directory", func() {
			err := fs.CleanDirectory(dir)
			Expect(err).NotTo(HaveOccurred())

			Expect(tmpFile.Name()).ToNot(BeAnExistingFile())
		})
	})
})
