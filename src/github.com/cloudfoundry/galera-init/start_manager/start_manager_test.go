package start_manager_test

import (
	"errors"
	"fmt"

	health_checker_fakes "github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	db_helper_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	node_prestarter_fakes "github.com/cloudfoundry/mariadb_ctrl/start_manager/node_prestarter/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	node_starter_fakes "github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter/fakes"
	upgrader_fakes "github.com/cloudfoundry/mariadb_ctrl/upgrader/fakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry/mariadb_ctrl/start_manager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StartManager", func() {

	var mgr StartManager

	var testLogger *lagertest.TestLogger
	var fakeOs *os_fakes.FakeOsHelper
	var fakeUpgrader *upgrader_fakes.FakeUpgrader
	var fakeDBHelper *db_helper_fakes.FakeDBHelper
	var fakeStarter *node_starter_fakes.FakeStarter
	var fakePreStarter *node_prestarter_fakes.FakePreStarter
	var fakeHealthChecker *health_checker_fakes.FakeClusterHealthChecker
	var startNodeReturn string
	var startNodeReturnError error
	var preStartNodeReturn string
	var preStartNodeReturnError error

	const stateFileLocation = "/stateFileLocation"
	const databaseStartupTimeout = 10

	type managerArgs struct {
		NodeIndex int
		NodeCount int
	}

	ensureStateFileContentIs := func(expected string) {
		count := fakeOs.WriteStringToFileCallCount()
		filename, contents := fakeOs.WriteStringToFileArgsForCall(count - 1)
		Expect(filename).To(Equal(stateFileLocation))
		Expect(contents).To(Equal(expected))
	}

	ensureNoWriteToStateFile := func() {
		count := fakeOs.WriteStringToFileCallCount()
		Expect(count).To(Equal(0))
	}

	ensureStartNodeWithMode := func(state string) {
		Expect(fakeStarter.StartNodeFromStateCallCount()).To(Equal(1))
		Expect(fakeStarter.StartNodeFromStateArgsForCall(0)).To(Equal(state))
	}

	ensurePreStartNodeWithMode := func(state string) {
		Expect(fakePreStarter.PreStartNodeFromStateCallCount()).To(Equal(1))
		Expect(fakePreStarter.PreStartNodeFromStateArgsForCall(0)).To(Equal(state))
	}

	createManager := func(args managerArgs) StartManager {

		clusterIps := []string{}
		for i := 0; i < args.NodeCount; i++ {
			clusterIps = append(clusterIps, fmt.Sprintf("0.0.0.%d", i+1))
		}

		return New(
			fakeOs,
			config.StartManager{
				StateFileLocation:      stateFileLocation,
				MyIP:                   clusterIps[args.NodeIndex],
				ClusterIps:             clusterIps,
				DatabaseStartupTimeout: databaseStartupTimeout,
			},
			fakeDBHelper,
			fakeUpgrader,
			fakeStarter,
			fakePreStarter,
			testLogger,
			fakeHealthChecker,
		)
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeUpgrader = new(upgrader_fakes.FakeUpgrader)
		fakeStarter = new(node_starter_fakes.FakeStarter)
		fakePreStarter = new(node_prestarter_fakes.FakePreStarter)
		fakeDBHelper = new(db_helper_fakes.FakeDBHelper)
		fakeHealthChecker = new(health_checker_fakes.FakeClusterHealthChecker)

		fakeDBHelper.IsProcessRunningReturns(false)
		fakeDBHelper.IsDatabaseReachableReturns(true)
		startNodeReturn = "CLUSTERED"
		startNodeReturnError = nil
		preStartNodeReturn = "CLUSTERED"
		preStartNodeReturnError = nil
	})

	JustBeforeEach(func() {
		fakeStarter.StartNodeFromStateReturns(startNodeReturn, startNodeReturnError)
		fakePreStarter.PreStartNodeFromStateReturns(preStartNodeReturn, preStartNodeReturnError)
	})

	Context("When a mysql process is already running", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})
			fakeDBHelper.IsProcessRunningReturns(true)
		})

		It("kills the process before continuing", func() {
			err := mgr.Execute("start")
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDBHelper.StopMysqlCallCount()).To(Equal(1))
		})
	})

	Describe("Upgrading the cluster", func() {
		Context("When determining whether an upgrade is required exits with an error", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})

				fakeUpgrader.NeedsUpgradeReturns(false, errors.New("Error determining whether upgrade is required"))
			})

			It("forwards the error", func() {
				err := mgr.Execute("start")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("When upgrade is required", func() {
			Context("And performing the upgrade exits with an error", func() {
				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeCount: 3,
					})

					fakeUpgrader.NeedsUpgradeReturns(true, nil)
					fakeUpgrader.UpgradeReturns(errors.New("Error while performing upgrade"))
				})

				It("forwards the error", func() {
					err := mgr.Execute("start")
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Context("When starting in single-node deployment", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 1,
			})
			startNodeReturn = "SINGLE_NODE"
		})

		Context("And it's an initial deploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			It("starts the node in SingleNode mode", func() {
				err := mgr.Execute("start")
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
			})
		})

		Context("And it's a redeploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.SingleNode, nil)
			})

			It("starts the node in SingleNode mode", func() {
				err := mgr.Execute("start")
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
			})
		})
	})

	Context("When starting in multi-node deployment", func() {
		Context("And it's an initial deploy, so there's no state file", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			Context("And the IP of the current node is the first in the cluster", func() {
				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeCount: 3,
					})
				})

				It("starts the node in NeedsBootstrap mode", func() {
					err := mgr.Execute("start")
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
					ensureStateFileContentIs("CLUSTERED")
				})
			})

			Context("And the IP of the current node is not the first in the cluster", func() {
				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeIndex: 1,
						NodeCount: 3,
					})
				})

				It("starts the node in Clustered mode", func() {
					err := mgr.Execute("start")
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
				})
			})
		})

		Context("When state file is present", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})
				fakeOs.FileExistsReturns(true)
			})

			Context("And contains extra whitespace characters as well as a valid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(fmt.Sprintf("\n\n     %s \n", node_starter.Clustered), nil)
				})

				It("joins the cluster", func() {
					err := mgr.Execute("start")
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
				})
			})

			Context("And reads '"+node_starter.Clustered+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.Clustered, nil)
				})

				It("joins the cluster", func() {
					err := mgr.Execute("start")
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
				})
			})

			Context("And reads '"+node_starter.NeedsBootstrap+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.NeedsBootstrap, nil)
				})

				It("starts the node in bootstrap mode", func() {
					err := mgr.Execute("start")
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
					ensureStateFileContentIs("CLUSTERED")
				})

				Context("And the IP of the current node is not the first in the cluster", func() {
					BeforeEach(func() {
						mgr = createManager(managerArgs{
							NodeIndex: 1,
							NodeCount: 3,
						})
					})

					It("starts the node in join mode", func() {
						err := mgr.Execute("start")
						Expect(err).ToNot(HaveOccurred())
						ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
						ensureStateFileContentIs("CLUSTERED")
					})
				})
			})

			Context("And contains an invalid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns("INVALID_STATE", nil)
					startNodeReturn = ""
					startNodeReturnError = errors.New("some error")
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute("start")
					Expect(actualErr).To(HaveOccurred())
				})

				It("does not write the state file", func() {
					err := mgr.Execute("start")
					Expect(err).To(HaveOccurred())
					ensureNoWriteToStateFile()
				})
			})

			Context("But is unreadable", func() {
				var err error
				BeforeEach(func() {
					err = errors.New("some error")
					fakeOs.ReadFileReturns("", err)
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute("start")
					Expect(actualErr).To(HaveOccurred())
					Expect(actualErr).To(Equal(err))
				})

				It("does not join the cluster or seed the databases", func() {
					mgr.Execute("start")
					Expect(fakeStarter.StartNodeFromStateCallCount()).To(Equal(0))
					ensureNoWriteToStateFile()
				})
			})
		})
	})

	Context("When prestarting in multi-node deployment", func() {

		Context("When state file is present", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})
				fakeOs.FileExistsReturns(true)
			})

			Context("And contains extra whitespace characters as well as a valid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(fmt.Sprintf("\n\n     %s \n", node_starter.Clustered), nil)
				})

				It("joins the cluster", func() {
					err := mgr.Execute("prestart")
					Expect(err).ToNot(HaveOccurred())
					ensurePreStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
				})
			})

			Context("And reads '"+node_starter.Clustered+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.Clustered, nil)
				})

				It("joins the cluster", func() {
					err := mgr.Execute("prestart")
					Expect(err).ToNot(HaveOccurred())
					ensurePreStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
				})
			})

			Context("And reads '"+node_starter.NeedsBootstrap+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.NeedsBootstrap, nil)
					preStartNodeReturn = "NEEDS_BOOTSTRAP"
				})

				It("prestarts the node in bootstrap mode", func() {
					err := mgr.Execute("prestart")
					Expect(err).ToNot(HaveOccurred())
					ensurePreStartNodeWithMode("NEEDS_BOOTSTRAP")
					ensureStateFileContentIs("NEEDS_BOOTSTRAP")
				})

				Context("And the IP of the current node is not the first in the cluster", func() {
					BeforeEach(func() {
						mgr = createManager(managerArgs{
							NodeIndex: 1,
							NodeCount: 3,
						})
						preStartNodeReturn = "CLUSTERED"
					})

					It("starts the node in join mode", func() {
						err := mgr.Execute("prestart")
						Expect(err).ToNot(HaveOccurred())
						ensurePreStartNodeWithMode("NEEDS_BOOTSTRAP")
						ensureStateFileContentIs("CLUSTERED")
					})
				})
			})

			Context("And contains an invalid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns("INVALID_STATE", nil)
					preStartNodeReturn = ""
					preStartNodeReturnError = errors.New("some error")
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute("prestart")
					Expect(actualErr).To(HaveOccurred())
				})

				It("does not write the state file", func() {
					err := mgr.Execute("prestart")
					Expect(err).To(HaveOccurred())
					ensureNoWriteToStateFile()
				})
			})

			Context("But is unreadable", func() {
				var err error
				BeforeEach(func() {
					err = errors.New("some error")
					fakeOs.ReadFileReturns("", err)
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute("prestart")
					Expect(actualErr).To(HaveOccurred())
					Expect(actualErr).To(Equal(err))
				})

				It("does not join the cluster", func() {
					mgr.Execute("prestart")
					Expect(fakePreStarter.PreStartNodeFromStateCallCount()).To(Equal(0))
					ensureNoWriteToStateFile()
				})
			})
		})
	})

	Context("When scaling the cluster", func() {
		Context("And scaling down from many nodes to single", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 1,
				})
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.Clustered, nil)
				startNodeReturn = "SINGLE_NODE"
			})

			It("starts the cluster in single node mode", func() {
				err := mgr.Execute("start")
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
			})
		})

		Context("And scaling from one to many nodes", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.SingleNode, nil)
			})

			It("starts the cluster in needs bootstrap mode", func() {
				err := mgr.Execute("start")
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
				ensureStateFileContentIs("CLUSTERED")
			})
		})
	})

})
