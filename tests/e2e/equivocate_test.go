package e2e

import (
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T7: up to f = 3 Byzantine validators equivocate. Nodes 2, 5 and 8 sign
// two prevotes and two precommits in every round, one version for each half
// of the network. Every honest node logs EQUIVOCATION against all three and
// counts at most one vote of each per (height, round, phase); the honest
// nodes still finalize one chain, with valid certificates.
func TestT7ByzantineValidators(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t7_byzantine")
	res, log := testutil.Run(t, cfg, "T7_byzantine")
	checkT7(t, cfg, res, log)
}

func checkT7(t *testing.T, cfg *simconfig.Config, res *sim.Result, log []byte) {
	t.Helper()
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	assertCertificates(t, res)
	byz := map[int]bool{} // the equivocators
	for _, b := range cfg.Byzantine {
		byz[b.Node] = b.Behavior == "equivocate"
	}
	for _, id := range res.Honest {
		for v := 0; v < cfg.NumValidators; v++ {
			accused := hasLine(log, fmt.Sprintf("node=%02d ev=EQUIVOCATION", id), fmt.Sprintf("validator=%02d", v))
			if byz[v] && !accused {
				t.Fatalf("node %d never caught equivocator %d", id, v)
			}
			if !byz[v] && accused {
				t.Fatalf("node %d accused validator %d, which never double-voted", id, v)
			}
		}
	}
}

// T7 with 20% loss before GST (10 s), seed 2. Counting only the first vote
// seen from each validator, nodes disagree on whether a round had 2f+1
// prevotes for a block (one saw an equivocator's vote for it, another its
// NIL copy). Without the POL carried by re-proposals this run
// stalls at height 3; with it, every node checks the quorum itself.
func TestT7EquivocationWithDropsStaysLive(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t7_byzantine_drops") // GST 10 s, 20% loss before
	res, log := testutil.Run(t, cfg, "T7_byzantine_drops")
	checkT7(t, cfg, res, log)
}
