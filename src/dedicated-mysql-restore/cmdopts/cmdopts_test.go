package cmdopts_test

import (
	"dedicated-mysql-restore/cmdopts"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cmdopts", func() {

	var args []string

	It("parses the required arguments", func() {
		args = []string{"--restore-file", "tmp/fake-file",
			"--encryption-key", "supersecret",
			"--mysql-username", "user",
			"--mysql-password", "password",
		}
		opts, err := cmdopts.ParseArgs(args)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.EncryptionKey).To(Equal("supersecret"))
		Expect(opts.MySQLPassword).To(Equal("password"))
		Expect(opts.MySQLUser).To(Equal("user"))
		Expect(opts.RestoreFile).To(Equal("tmp/fake-file"))
	})

	It("ignores unknown flag arguments", func() {
		args = []string{"--restore-file", "tmp/fake-file",
			"--encryption-key", "supersecret",
			"--mysql-username", "user",
			"--mysql-password", "password",
			"--pickles", "nope",
		}
		opts, err := cmdopts.ParseArgs(args)
		Expect(err).ToNot(HaveOccurred())

		Expect(opts.EncryptionKey).To(Equal("supersecret"))
		Expect(opts.MySQLPassword).To(Equal("password"))
		Expect(opts.MySQLUser).To(Equal("user"))
		Expect(opts.RestoreFile).To(Equal("tmp/fake-file"))
	})

	It("returns usage information when --help is passed", func() {
		args = []string{"--help"}

		_, err := cmdopts.ParseArgs(args)
		Expect(err.Error()).To(MatchRegexp("--restore-file --encryption-key --mysql-username --mysql-password"))
		Expect(err.Error()).To(MatchRegexp("Usage:"))
		Expect(err.Error()).To(MatchRegexp("Application Options:"))
		Expect(err.Error()).To(MatchRegexp("Help Options:"))

	})

	It("returns an error if one of the required arguments is not passed", func() {
		args = []string{
			"--encryption-key", "supersecret",
			"--mysql-username", "user",
			"--mysql-password", "password",
		}
		_, err := cmdopts.ParseArgs(args)
		Expect(err.Error()).To(MatchRegexp("the required flag `--restore-file' was not specified"))
	})

	It("returns an error and exits if no options are passed", func() {
		_, err := cmdopts.ParseArgs([]string{})
		Expect(err.Error()).To(MatchRegexp("the required flags .* were not specified"))
	})
})
