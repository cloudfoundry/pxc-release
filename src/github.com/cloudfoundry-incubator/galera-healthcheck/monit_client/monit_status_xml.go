package monit_client

import (
	"encoding/xml"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/net/html/charset"
)

type MonitStatus struct {
	XMLName  xml.Name     `xml:"monit"`
	Services []ServiceTag `xml:"service"`
}

type ServiceTag struct {
	XMLName       xml.Name `xml:"service"`
	Name          string   `xml:"name"`
	Status        int      `xml:"status"`
	Monitor       int      `xml:"monitor"`
	PendingAction int      `xml:"pendingaction"`
}

type ServiceStatus string

const (
	MonitMonitorStatusStopped      = 0
	MonitMonitorStatusStarted      = 1
	MonitMonitorStatusInitializing = 2
	ServicePending                 = "pending"
	ServiceStopped                 = "stopped"
	ServiceInitializing            = "initializing"
	ServiceRunning                 = "running"
	ServiceFailing                 = "failing"
)

func (t ServiceTag) String() string {
	switch {
	case t.PendingAction != 0:
		return ServicePending
	case t.Monitor == MonitMonitorStatusStopped:
		return ServiceStopped
	case t.Monitor == MonitMonitorStatusInitializing:
		return ServiceInitializing
	case t.Status == 0 && t.Monitor == MonitMonitorStatusStarted:
		return ServiceRunning
	default:
		return ServiceFailing
	}
}

func ParseXML(xmlReader io.Reader) (MonitStatus, error) {
	var (
		result  MonitStatus
		decoder = xml.NewDecoder(xmlReader)
	)

	decoder.CharsetReader = func(characterSet string, xmlReader io.Reader) (io.Reader, error) {
		return charset.NewReader(xmlReader, "ISO-8859-1")
	}

	if err := decoder.Decode(&result); err != nil {
		return result, errors.Wrap(err, "failed to unmarshal monit status response")
	}

	return result, nil
}
