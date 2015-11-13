package monit_status

import (
	"encoding/xml"
	"fmt"
	"github.com/pivotal-golang/lager"
	"golang.org/x/net/html/charset"
	"io"
)

const (
	CHARSETENCODING = "ISO-8859-1"
)

type MonitStatus struct {
	XMLName  xml.Name     `xml:"monit"`
	Services []serviceTag `xml:"service"`
}

type serviceTag struct {
	XMLName xml.Name `xml:"service"`
	Name    string   `xml:"name"`
	Status  int      `xml:"status"`
	Monitor int      `xml:"monitor"`
}

func (s serviceTag) StatusString() (statusString string) {
	switch {
	case s.Monitor == 0:
		statusString = "unknown"
	case s.Monitor == 2:
		statusString = "starting"
	case s.Status == 0:
		statusString = "running"
	default:
		statusString = "failing"
	}
	return
}

func (st MonitStatus) NewMonitStatus(xmlReader io.Reader, logger lager.Logger) (MonitStatus, error) {
	status, err := ParseXML(xmlReader)
	logger.Info("Parsing xml response from monit", lager.Data{
		"monit_status": status,
	})
	if err != nil {
		err = fmt.Errorf("Failed to create a monit status object %s", err.Error())
		return status, err
	}
	return status, nil
}

func (monitStatusObject MonitStatus) GetStatus(name string) (string, error) {

	for _, serviceTag := range monitStatusObject.Services {
		if serviceTag.Name == name {
			return serviceTag.StatusString(), nil
		}
	}

	err := fmt.Errorf("Could not find process %s in the monit status", name)
	return "", err
}

func ParseXML(xmlReader io.Reader) (MonitStatus, error) {
	result := MonitStatus{}
	decoder := xml.NewDecoder(xmlReader)

	decoder.CharsetReader = func(characterSet string, xmlReader io.Reader) (io.Reader, error) {
		return charset.NewReader(xmlReader, CHARSETENCODING)
	}
	err := decoder.Decode(&result)

	if err != nil {
		err := fmt.Errorf("Failed to unmarshal the xml with error %s",
			err.Error(),
		)
		return result, err
	}

	return result, nil
}
