package start_manager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper/db_helperfakes"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
	. "github.com/cloudfoundry/galera-init/start_manager"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter/node_starterfakes"
	"github.com/cloudfoundry/galera-init/start_manager/start_managerfakes"
)

var _ = Describe("StartManager", func() {

	var mgr StartManager

	var testLogger *lagertest.TestLogger
	var fakeOs *os_helperfakes.FakeOsHelper
	var fakeDBHelper *db_helperfakes.FakeDBHelper
	var fakeStarter *node_starterfakes.FakeStarter
	var fakeHealthChecker *cluster_health_checkerfakes.FakeClusterHealthChecker
	var startNodeReturn string
	var startNodeReturnError error
	var mysqldErrChan chan error
	var fakeserviceStatusServer *start_managerfakes.FakeServiceStatus

	const stateFileLocation = "/stateFileLocation"

	type managerArgs struct {
		BootstrapNode bool
		NodeCount     int
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

	createManager := func(args managerArgs) StartManager {

		clusterIps := []string{}
		for i := 0; i < args.NodeCount; i++ {
			clusterIps = append(clusterIps, fmt.Sprintf("0.0.0.%d", i+1))
		}

		return New(
			fakeOs,
			config.StartManager{
				StateFileLocation: stateFileLocation,
				BootstrapNode:     args.BootstrapNode,
				ClusterIps:        clusterIps,
			},
			fakeDBHelper,
			fakeStarter,
			testLogger,
			fakeHealthChecker,
			fakeserviceStatusServer,
		)
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_helperfakes.FakeOsHelper)
		fakeStarter = new(node_starterfakes.FakeStarter)
		fakeDBHelper = new(db_helperfakes.FakeDBHelper)
		fakeHealthChecker = new(cluster_health_checkerfakes.FakeClusterHealthChecker)
		fakeserviceStatusServer = new(start_managerfakes.FakeServiceStatus)
		fakeDBHelper.IsProcessRunningReturns(false)
		fakeDBHelper.IsDatabaseReachableReturns(true)
		startNodeReturn = "CLUSTERED"
		startNodeReturnError = nil

		mysqldErrChan = make(chan error, 1)
	})

	JustBeforeEach(func() {
		fakeStarter.StartNodeFromStateStub = func(state string) (newState string, mysqlErrCh <-chan error, e error) {
			mysqldErrChan <- nil
			return startNodeReturn, mysqldErrChan, startNodeReturnError
		}
	})

	Context("when the mysql process exits with an error", func() {
		JustBeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})

			fakeStarter.StartNodeFromStateStub = func(state string) (newState string, mysqlErrCh <-chan error, e error) {
				mysqldErrChan <- errors.New("some mysql error")
				return startNodeReturn, mysqldErrChan, startNodeReturnError
			}
		})
		It("returns an error", func() {
			err := mgr.Execute(context.TODO())
			Expect(err).To(MatchError(`some mysql error`))
		})
	})

	Context("When a mysql process is already running", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})
			fakeDBHelper.IsProcessRunningReturns(true)
		})

		It("kills the process before continuing", func() {
			err := mgr.Execute(context.TODO())
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDBHelper.StopMysqldCallCount()).To(Equal(1))
		})
	})

	Context("when the configured context indicates we should shutdown", func() {
		JustBeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})

			fakeStarter.StartNodeFromStateStub = func(state string) (newState string, mysqlErrCh <-chan error, e error) {
				return startNodeReturn, mysqldErrChan, startNodeReturnError
			}

			fakeOs.KillCommandStub = func(cmd *exec.Cmd, signal os.Signal) error {
				mysqldErrChan <- nil
				return nil
			}
		})

		ensureTimeoutOfMySQLIfExecuteHangs := func() {
			time.Sleep(2 * time.Second)
			mysqldErrChan <- errors.New("failed-to-cancel")
		}

		It("should gracefully stop mysqld", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			go ensureTimeoutOfMySQLIfExecuteHangs()

			err := mgr.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeOs.KillCommandCallCount()).To(Equal(1))
			_, signal := fakeOs.KillCommandArgsForCall(0)
			Expect(signal).To(Equal(syscall.SIGTERM))
		})

		It("should return an error if terminating mysql fails", func() {
			fakeOs.KillCommandReturns(errors.New("mysqld process does not exist"))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			go ensureTimeoutOfMySQLIfExecuteHangs()

			err := mgr.Execute(ctx)
			Expect(err).To(MatchError(`mysqld process does not exist`))
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
				err := mgr.Execute(context.TODO())
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
				Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
			})
		})

		Context("And it's a redeploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.SingleNode, nil)
			})

			It("starts the node in SingleNode mode", func() {
				err := mgr.Execute(context.TODO())
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
				Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
			})
		})
	})

	Context("When starting in multi-node deployment", func() {
		Context("And it's an initial deploy, so there's no state file", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			Context("And the current node is the bootstrap node", func() {
				BeforeEach(func() {
					mgr = createManager(managerArgs{
						BootstrapNode: true,
						NodeCount:     3,
					})
				})

				It("starts the node in NeedsBootstrap mode", func() {
					err := mgr.Execute(context.TODO())
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
					ensureStateFileContentIs("CLUSTERED")
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
				})
			})

			Context("And the current node is not the bootstrap node", func() {
				BeforeEach(func() {
					mgr = createManager(managerArgs{
						BootstrapNode: false,
						NodeCount:     3,
					})
				})

				It("starts the node in Clustered mode", func() {
					err := mgr.Execute(context.TODO())
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
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
					err := mgr.Execute(context.TODO())
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
				})
			})

			Context("And reads '"+node_starter.Clustered+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.Clustered, nil)
				})

				It("joins the cluster", func() {
					err := mgr.Execute(context.TODO())
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("CLUSTERED")
					ensureStateFileContentIs("CLUSTERED")
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
				})
			})

			Context("And reads '"+node_starter.NeedsBootstrap+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.NeedsBootstrap, nil)
				})

				It("starts the node in bootstrap mode", func() {
					err := mgr.Execute(context.TODO())
					Expect(err).ToNot(HaveOccurred())
					ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
					ensureStateFileContentIs("CLUSTERED")
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
				})

				Context("And the current node is not the bootstrap node", func() {
					BeforeEach(func() {
						mgr = createManager(managerArgs{
							BootstrapNode: false,
							NodeCount:     3,
						})
					})

					It("starts the node in join mode", func() {
						err := mgr.Execute(context.TODO())
						Expect(err).ToNot(HaveOccurred())
						ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
						ensureStateFileContentIs("CLUSTERED")
						Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
					})
				})

				Context("And writing the statefile fails", func() {
					BeforeEach(func() {
						fakeOs.WriteStringToFileReturns(errors.New("writing failed"))
					})

					It("returns the error", func() {
						actualErr := mgr.Execute(context.TODO())
						Expect(actualErr).To(HaveOccurred())
						Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(0))
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
					actualErr := mgr.Execute(context.TODO())
					Expect(actualErr).To(HaveOccurred())
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(0))
				})

				It("does not write the state file", func() {
					err := mgr.Execute(context.TODO())
					Expect(err).To(HaveOccurred())
					ensureNoWriteToStateFile()
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(0))
				})
			})

			Context("But is unreadable", func() {
				var err error
				BeforeEach(func() {
					err = errors.New("some error")
					fakeOs.ReadFileReturns("", err)
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute(context.TODO())
					Expect(actualErr).To(HaveOccurred())
					Expect(actualErr).To(Equal(err))
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(0))
				})

				It("does not join the cluster or seed the databases", func() {
					mgr.Execute(context.TODO())
					Expect(fakeStarter.StartNodeFromStateCallCount()).To(Equal(0))
					ensureNoWriteToStateFile()
					Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(0))
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
				err := mgr.Execute(context.TODO())
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("SINGLE_NODE")
				ensureStateFileContentIs("SINGLE_NODE")
				Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
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
				err := mgr.Execute(context.TODO())
				Expect(err).ToNot(HaveOccurred())
				ensureStartNodeWithMode("NEEDS_BOOTSTRAP")
				ensureStateFileContentIs("CLUSTERED")
				Expect(fakeserviceStatusServer.StartCallCount()).To(Equal(1))
			})
		})

	})
})
