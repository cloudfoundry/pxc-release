package mysql_start_mode

import (
	"fmt"
	"io/ioutil"
)

type MysqlStartMode struct {
	stateFilePath string
	mode          string
}

func NewMysqlStartMode(stateFilePath string, mode string) *MysqlStartMode {
	return &MysqlStartMode{
		stateFilePath: stateFilePath,
		mode:          mode,
	}
}

func (ms *MysqlStartMode) Start() (bool, error) {
	var err error
	switch ms.mode {
	case "bootstrap":
		err = ms.mysqlStartModeInBootstrap()
	case "join":
		err = ms.mysqlStartModeInJoin()
	default:
		err = fmt.Errorf("Unrecognized value for start mode!")
	}

	if err != nil {
		return false, err
	}
	return true, err
}

func (ms *MysqlStartMode) mysqlStartModeInBootstrap() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("NEEDS_BOOTSTRAP"), 0777)
	if err != nil {
		return err
	}
	return nil
}

func (ms *MysqlStartMode) mysqlStartModeInJoin() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("CLUSTERED"), 0777)
	if err != nil {
		return err
	}
	return nil
}
