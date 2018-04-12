package unpack

import (
	"fmt"
	"io"

	"golang.org/x/crypto/openpgp"
)

type GPGReader struct {
	Passphrase string
}

func (gpg *GPGReader) Open(r io.Reader) (io.Reader, error) {
	alreadyAttemptedPassphrase := false
	promptFunction := func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		if alreadyAttemptedPassphrase {
			return nil, fmt.Errorf("failed to decrypt with encryption key")
		}
		alreadyAttemptedPassphrase = true
		return []byte(gpg.Passphrase), nil
	}

	md, err := openpgp.ReadMessage(r, nil, promptFunction, nil)
	if err != nil {
		return nil, err
	}

	return md.UnverifiedBody, nil
}
