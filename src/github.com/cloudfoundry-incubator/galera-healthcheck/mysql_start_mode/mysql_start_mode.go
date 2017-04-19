package mysql_start_mode

import (
	"fmt"
	"io/ioutil"
	"strings"
)

type MysqlStartMode struct {
	stateFilePath    string
	grastateFilePath string
	mode             string
}

func NewMysqlStartMode(stateFilePath string, grastateFilePath string, mode string) *MysqlStartMode {
	return &MysqlStartMode{
		stateFilePath:    stateFilePath,
		grastateFilePath: grastateFilePath,
		mode:             mode,
	}
}

func (ms *MysqlStartMode) Start() error {
	var err error
	switch ms.mode {
	case "bootstrap":
		err = ms.mysqlStartModeInBootstrap()
	case "join":
		err = ms.mysqlStartModeInJoin()
	case "singleNode":
		err = ms.mysqlStartModeInSingleNode()
	default:
		err = fmt.Errorf("Unrecognized value for start mode!")
	}

	if err != nil {
		return fmt.Errorf("Failed to write state file in mode '%s' to location '%s': %s", ms.mode, ms.stateFilePath, err.Error())
	}
	return nil
}

func (ms *MysqlStartMode) mysqlStartModeInBootstrap() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("NEEDS_BOOTSTRAP"), 0777)
	if err != nil {
		return err
	}

	read, err := ioutil.ReadFile(ms.grastateFilePath)
	if err != nil {
		return err
	}
	subbed := strings.Replace(string(read), "safe_to_bootstrap: 0", "safe_to_bootstrap: 1", -1)
	err = ioutil.WriteFile(ms.grastateFilePath, []byte(subbed), 0777)

	return err
}

func (ms *MysqlStartMode) mysqlStartModeInJoin() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("CLUSTERED"), 0777)
	if err != nil {
		return err
	}
	return nil
}

func (ms *MysqlStartMode) mysqlStartModeInSingleNode() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("SINGLE_NODE"), 0777)
	if err != nil {
		return err
	}
	return nil
}
