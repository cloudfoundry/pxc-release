package bootstrapper

import (
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
)

const PollingIntervalInSec = 5

type Bootstrapper struct {
	rootConfig *config.Config
	clock      clock.Clock
}

func New(rootConfig *config.Config, clock clock.Clock) *Bootstrapper {
	return &Bootstrapper{
		rootConfig: rootConfig,
		clock:      clock,
	}
}

func (b *Bootstrapper) Run() error {
	logger := b.rootConfig.Logger

	nodeManager := node_manager.New(b.rootConfig, b.clock)

	err := nodeManager.VerifyClusterIsUnhealthy()
	if err != nil {
		return err
	}

	err = nodeManager.VerifyAllNodesAreReachable()
	if err != nil {
		return err
	}

	err = nodeManager.StopAllNodes()
	if err != nil {
		return err
	}

	sequenceNumberMap, err := nodeManager.GetSequenceNumbers()
	if err != nil {
		return err
	}

	bootstrapNodeURL, joinNodes := largestSequenceNumber(sequenceNumberMap)
	err = nodeManager.BootstrapNode(bootstrapNodeURL)
	if err != nil {
		return err
	}

	// galera recommends joining nodes one at a time
	for _, url := range joinNodes {
		err = nodeManager.JoinNode(url)
		if err != nil {
			return err
		}
	}

	logger.Info("Successfully started mysql process on all nodes")

	return nil
}

func largestSequenceNumber(seqMap map[string]int) (string, []string) {
	maxSeq := -1
	maxSeqURL := ""
	joinNodes := []string{}
	for url, seqno := range seqMap {
		if seqno > maxSeq {
			maxSeq = seqno
			maxSeqURL = url
		}
	}

	for url, _ := range seqMap {
		if url != maxSeqURL {
			joinNodes = append(joinNodes, url)
		}
	}

	return maxSeqURL, joinNodes
}
