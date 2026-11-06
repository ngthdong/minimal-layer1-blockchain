package e2e

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// T1 on the ring of config/topology_ring10.txt (4 neighbours per node):
// headers, bodies, votes and txs travel hop by hop, and every honest node
// still finalizes the same chain and state. No message ever goes to a node
// that is not a neighbour.
func TestT1OnTopologyFile(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t_topology_file")
	res, log := testutil.Run(t, cfg, "T1_topology_ring")
	checkT1(t, res, cfg.MaxHeight)
	adj, err := network.LoadTopology(cfg.Network.TopologyFile, cfg.NumValidators, nil)
	if err != nil {
		t.Fatal(err)
	}
	send := regexp.MustCompile(`node=(\d+) ev=SEND .* to=(\d+)`)
	for _, line := range bytes.Split(log, []byte("\n")) {
		m := send.FindSubmatch(line)
		if m == nil {
			continue
		}
		from, to := types.NodeID(atoi(m[1])), types.NodeID(atoi(m[2]))
		if !containsID(adj[from], to) {
			t.Fatalf("%d sent to %d, which is not a neighbour: %s", from, to, line)
		}
	}
	if testutil.CountLines(log, "reason=no_link") != 0 {
		t.Fatal("a node tried to reach a non-neighbour")
	}
}

func atoi(b []byte) int {
	n := 0
	for _, c := range b {
		n = n*10 + int(c-'0')
	}
	return n
}

func containsID(ids []types.NodeID, id types.NodeID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
