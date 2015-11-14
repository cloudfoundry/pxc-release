package start_mysql

import (
	"fmt"
	"io/ioutil"
)

type StartMysql struct {
	stateFilePath string
	mode          string
}

func NewStartMySql(stateFilePath string, mode string) *StartMysql {
	return &StartMysql{
		stateFilePath: stateFilePath,
		mode:          mode,
	}
}

func (ms *StartMysql) Start() (bool, error) {
	var err error
	switch ms.mode {
	case "bootstrap":
		err = ms.startMysqlInBootstrap()
	case "join":
		err = ms.startMysqlInJoin()
	default:
		err = fmt.Errorf("Unrecognized value for start mode!")
	}

	if err != nil {
		return false, err
	}
	return true, err
}

func (ms *StartMysql) startMysqlInBootstrap() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("NEEDS_BOOTSTRAP"), 0777)
	if err != nil {
		return err
	}
	return nil
}

func (ms *StartMysql) startMysqlInJoin() error {
	err := ioutil.WriteFile(ms.stateFilePath, []byte("CLUSTERED"), 0777)
	if err != nil {
		return err
	}
	return nil
}
