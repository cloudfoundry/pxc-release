package unpack_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	. "dedicated-mysql-restore/unpack"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Unpack", func() {
	Context("GPGReader", func() {
		var (
			plaintext       string = "some-string"
			encryptedStream io.Reader
		)

		BeforeEach(func() {
			var err error
			encryptedStream, err = gpgEncrypt(bytes.NewBufferString(plaintext))
			Expect(err).ToNot(HaveOccurred())
		})
		It("decrypts GPG encrypted data", func() {
			gpgReader := GPGReader{Passphrase: "secret-key"}
			plaintextStream, err := gpgReader.Open(encryptedStream)
			Expect(err).ToNot(HaveOccurred())
			contents, err := ioutil.ReadAll(plaintextStream)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(contents)).To(Equal(plaintext))
		})

		It("fails fast with a bad passphrase", func() {
			gpgReader := GPGReader{Passphrase: "bad-secret-key"}
			_, err := gpgReader.Open(encryptedStream)
			Expect(err).To(MatchError("failed to decrypt with encryption key"))
		})
	})

	Context("ExtractTar", func() {
		var (
			tmpDir  string
			tmpFile *os.File
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "fs_test_")
			Expect(err).NotTo(HaveOccurred())

			tmpFile, err = ioutil.TempFile(tmpDir, "fs_test_")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})
		It("can unpack a tar file backup", func() {
			destDir, err := ioutil.TempDir("", "tar_test_")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(destDir)

			tarCreateCmd := exec.Command("tar", "-vvvv", "-c", "-C", tmpDir, ".")
			stdout, err := tarCreateCmd.StdoutPipe()
			Expect(err).ToNot(HaveOccurred())
			tarCreateCmd.Stderr = GinkgoWriter
			err = tarCreateCmd.Start()
			Expect(err).ToNot(HaveOccurred())

			err = ExtractTar(stdout, destDir, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			err = tarCreateCmd.Wait()
			Expect(err).ToNot(HaveOccurred())

			restoredPath := filepath.Join(
				destDir,
				filepath.Base(tmpFile.Name()),
			)
			Expect(restoredPath).To(BeAnExistingFile())
		})

		It("Can unpack a gpg decrypted output stream", func() {
			destDir, err := ioutil.TempDir("", "tar_test_")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(destDir)

			tarCreateCmd := exec.Command("tar", "-vvvv", "-c", "-C", tmpDir, ".")
			stdout, err := tarCreateCmd.StdoutPipe()
			Expect(err).ToNot(HaveOccurred())
			tarCreateCmd.Stderr = GinkgoWriter
			err = tarCreateCmd.Start()
			Expect(err).ToNot(HaveOccurred())

			encryptedStream, err := gpgEncrypt(stdout)
			Expect(err).ToNot(HaveOccurred())

			gpgReader := GPGReader{Passphrase: "secret-key"}
			plaintextStream, err := gpgReader.Open(encryptedStream)
			Expect(err).ToNot(HaveOccurred())

			err = ExtractTar(plaintextStream, destDir, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			err = tarCreateCmd.Wait()
			Expect(err).ToNot(HaveOccurred())

			restoredPath := filepath.Join(
				destDir,
				filepath.Base(tmpFile.Name()),
			)
			Expect(restoredPath).To(BeAnExistingFile())
		})
	})

})

func gpgEncrypt(r io.Reader) (io.Reader, error) {
	output := &bytes.Buffer{}
	gpgCmd := exec.Command(
		"gpg",
		"--batch", "--yes", "--no-tty", // non-interactive
		"--symmetric",
		"--cipher-algo=AES256",
		"--compress-algo=zip",
		"--compress-level=3",
		"--passphrase=secret-key",
	)
	gpgCmd.Stdin = r
	gpgCmd.Stdout = output
	gpgCmd.Stderr = GinkgoWriter
	if err := gpgCmd.Run(); err != nil {
		return nil, err
	}
	return output, nil
}
