package bootstrapper

import "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"

const PollingIntervalInSec = 5

type Bootstrapper struct {
	nodeManager node_manager.NodeManager
}

func New(nodeManager node_manager.NodeManager) *Bootstrapper {
	return &Bootstrapper{
		nodeManager: nodeManager,
	}
}

func (b *Bootstrapper) RejoinUnsafe() (bool, error) {
	unhealthy, err := b.nodeManager.VerifyClusterIsUnhealthy()
	if err != nil {
		return false, err
	}

	if !unhealthy {
		return false, nil
	}

	url, err := b.nodeManager.FindUnhealthyNode()
	if err != nil {
		return false, err
	}

	err = b.nodeManager.StopNode(url)
	if err != nil {
		return false, err
	}

	err = b.nodeManager.JoinNode(url)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (b *Bootstrapper) Bootstrap() (bool, error) {
	unhealthy, err := b.nodeManager.VerifyClusterIsUnhealthy()
	if err != nil {
		return false, err
	}

	if !unhealthy {
		return false, nil
	}

	err = b.nodeManager.VerifyAllNodesAreReachable()
	if err != nil {
		return false, err
	}

	err = b.nodeManager.StopAllNodes()
	if err != nil {
		return false, err
	}

	sequenceNumberMap, err := b.nodeManager.GetSequenceNumbers()
	if err != nil {
		return false, err
	}

	bootstrapNodeURL, joinNodes := largestSequenceNumber(sequenceNumberMap)
	err = b.nodeManager.BootstrapNode(bootstrapNodeURL)
	if err != nil {
		return false, err
	}

	// galera recommends joining nodes one at a time
	for _, url := range joinNodes {
		err = b.nodeManager.JoinNode(url)
		if err != nil {
			return false, err
		}
	}

	return true, nil
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
