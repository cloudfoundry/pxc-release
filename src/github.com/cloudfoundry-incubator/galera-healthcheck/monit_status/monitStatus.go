package monitStatus

import (
	"encoding/xml"
	"fmt"
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

func (st MonitStatus) NewMonitStatus(xmlStatus string) (MonitStatus, error) {
	status, err := ParseXML(xmlStatus)
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

func ParseXML(xmlString string) (MonitStatus, error) {
	result := MonitStatus{}
	err := xml.Unmarshal([]byte(xmlString), &result)

	if err != nil {
		err := fmt.Errorf("Failed to unmarshal the xml %s with error %s",
			xmlString,
			err.Error(),
		)
		return result, err
	}

	return result, nil
}
