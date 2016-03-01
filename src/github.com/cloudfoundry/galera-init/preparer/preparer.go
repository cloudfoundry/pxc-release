package preparer

import (
	"os"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_runner"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

type preparer struct {
	rootConfig    config.Config
	osHelper      os_helper.OsHelper
	mariaDBHelper mariadb_helper.DBHelper
	upgrader      upgrader.Upgrader
	galeraHelper  cluster_health_checker.ClusterHealthChecker
	starter       node_starter.Starter
	mgr           start_manager.StartManager
	runner        ifrit.Runner
	logger        lager.Logger
}

type Preparer interface {
	Prepare() ifrit.Runner
}

func New(logger lager.Logger, rootConfig config.Config) Preparer {
	return &preparer{
		rootConfig: rootConfig,
		logger:     logger,
	}
}

func (p *preparer) Prepare() ifrit.Runner {
	p.osHelper = os_helper.NewImpl()

	p.mariaDBHelper = mariadb_helper.NewMariaDBHelper(
		p.osHelper,
		p.rootConfig.Db,
		p.rootConfig.LogFileLocation,
		p.logger,
	)

	p.upgrader = upgrader.NewUpgrader(
		p.osHelper,
		p.rootConfig.Upgrader,
		p.logger,
		p.mariaDBHelper,
	)

	p.galeraHelper = cluster_health_checker.NewClusterHealthChecker(
		p.rootConfig.Manager.ClusterIps,
		p.logger,
	)

	p.starter = p.makeStarter()

	p.mgr = start_manager.New(
		p.osHelper,
		p.rootConfig.Manager,
		p.mariaDBHelper,
		p.upgrader,
		p.starter,
		p.logger,
		p.galeraHelper,
	)

	p.runner = p.makeRunner()

	sigRunner := sigmon.New(p.runner, os.Kill)

	return sigRunner
}

func (p *preparer) makeStarter() node_starter.Starter {
	var starter node_starter.Starter

	if p.rootConfig.Prestart {
		starter = node_starter.NewPreStarter(
			p.mariaDBHelper,
			p.osHelper,
			p.rootConfig.Manager,
			p.logger,
			p.galeraHelper,
		)
	} else {
		starter = node_starter.NewStarter(
			p.mariaDBHelper,
			p.osHelper,
			p.rootConfig.Manager,
			p.logger,
			p.galeraHelper,
		)
	}

	return starter
}

func (p *preparer) makeRunner() ifrit.Runner {
	var runner ifrit.Runner

	if p.rootConfig.Prestart {
		runner = node_runner.NewPrestartRunner(p.mgr, p.logger)
	} else {
		runner = node_runner.NewRunner(p.mgr, p.logger)
	}

	return runner
}
