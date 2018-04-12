package cmdopts

import (
	flags "github.com/jessevdk/go-flags"
)

type Options struct {
	EncryptionKey string `required:"true" long:"encryption-key" description:"Key used to decrypt backup artifact"`
	MySQLPassword string `required:"false" long:"mysql-password" description:"Password to authenticate to mysql instance"`
	MySQLUser     string `required:"false" long:"mysql-username" description:"Username to authenticate to mysql instance"`
	RestoreFile   string `required:"true" long:"restore-file" description:"Path to backup artifact to restore"`
}

const requiredArgs = `--restore-file --encryption-key --mysql-username --mysql-password`

func ParseArgs(args []string) (*Options, error) {
	opts := &Options{}

	parser := flags.NewParser(opts, flags.HelpFlag|flags.IgnoreUnknown)
	parser.Usage = requiredArgs
	_, err := parser.ParseArgs(args)
	if err != nil {
		return nil, err
	}

	return opts, nil
}

