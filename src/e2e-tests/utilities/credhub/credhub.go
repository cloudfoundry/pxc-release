package credhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"e2e-tests/utilities/cmd"
)

type CertificateCredential struct {
	CA          string `json:"ca"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}

func FindCredentialName(pattern string) (string, error) {
	var credentialResponse struct {
		Credentials []struct {
			Name string `json:"name"`
		} `json:"credentials"`
	}

	var output bytes.Buffer
	if err := cmd.RunWithoutOutput(&output,
		"credhub", "find", "--name-like="+pattern, "--output-json",
	); err != nil {
		return "", err
	}

	if err := json.Unmarshal(output.Bytes(), &credentialResponse); err != nil {
		return "", err
	}

	if len(credentialResponse.Credentials) != 1 {
		return "", fmt.Errorf("unexpected credentials found for %q. Expected 1 credentials but found %d: %v", pattern, len(credentialResponse.Credentials), credentialResponse.Credentials)
	}

	return credentialResponse.Credentials[0].Name, nil
}

func GetCredhubPassword(partialName string) (string, error) {
	name, err := FindCredentialName(partialName)
	if err != nil {
		return "", err
	}

	var output bytes.Buffer
	err = cmd.RunWithoutOutput(&output, "credhub", "get", "--name="+name, "--quiet")

	return strings.TrimSpace(output.String()), err
}

func GetCredhubCertificate(partialName string) (cert CertificateCredential, err error) {
	name, err := FindCredentialName(partialName)
	if err != nil {
		return cert, err
	}

	var output bytes.Buffer
	if err = cmd.RunWithoutOutput(&output, "credhub", "get", "--name="+name, "--output-json"); err != nil {
		return cert, err
	}

	var result struct {
		Value CertificateCredential `json:"value"`
	}

	if err = json.Unmarshal(output.Bytes(), &result); err != nil {
		return cert, err
	}

	return result.Value, nil
}
