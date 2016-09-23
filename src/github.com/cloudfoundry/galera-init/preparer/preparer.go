package preparer

import (
	"os"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_runner"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

type Preparer struct {
	rootConfig           config.Config
	OsHelper             os_helper.OsHelper
	DBHelper             mariadb_helper.DBHelper
	Upgrader             upgrader.Upgrader
	ClusterHealthChecker cluster_health_checker.ClusterHealthChecker
	NodeStarter          node_starter.Starter
	NodeStartManager     start_manager.StartManager
	Logger               lager.Logger
}

func New(logger lager.Logger, rootConfig config.Config) *Preparer {
	p := &Preparer{}

	p.Logger = logger
	p.rootConfig = rootConfig

	p.OsHelper = os_helper.NewImpl()

	p.DBHelper = mariadb_helper.NewMariaDBHelper(
		p.OsHelper,
		p.rootConfig.Db,
		p.rootConfig.LogFileLocation,
		p.Logger,
	)

	p.Upgrader = upgrader.NewUpgrader(
		p.OsHelper,
		p.rootConfig.Upgrader,
		p.Logger,
		p.DBHelper,
	)

	p.ClusterHealthChecker = cluster_health_checker.NewClusterHealthChecker(
		p.rootConfig.Manager.ClusterIps,
		p.Logger,
	)

	if p.rootConfig.Prestart {
		p.NodeStarter = node_starter.NewPreStarter(
			p.DBHelper,
			p.OsHelper,
			p.rootConfig.Manager,
			p.Logger,
			p.ClusterHealthChecker,
		)
	} else {
		p.NodeStarter = node_starter.NewStarter(
			p.DBHelper,
			p.OsHelper,
			p.rootConfig.Manager,
			p.Logger,
			p.ClusterHealthChecker,
		)
	}

	p.NodeStartManager = start_manager.New(
		p.OsHelper,
		p.rootConfig.Manager,
		p.DBHelper,
		p.Upgrader,
		p.NodeStarter,
		p.Logger,
		p.ClusterHealthChecker,
	)

	return p
}

func (p *Preparer) Prepare() ifrit.Runner {
	var runner ifrit.Runner

	if p.rootConfig.Prestart {
		runner = node_runner.NewPrestartRunner(p.NodeStartManager, p.Logger)
	} else {
		runner = node_runner.NewRunner(p.NodeStartManager, p.Logger)
	}

	sigRunner := sigmon.New(runner, os.Kill)

	return sigRunner
}
