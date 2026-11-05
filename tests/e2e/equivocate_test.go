package e2e

import (
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T7: f = 3 Byzantine validators. Nodes 2 and 5 double-vote every prevote
// and precommit, one version to each half of the network; every honest node
// sees both signed votes and logs EQUIVOCATION against each. Node 8 proposes
// an invalid block whenever it is proposer (h = 8, 18, 28: a bad tx
// signature, a wrong state hash, a wrong parent): every other node refuses
// it with the right reason and that round fails. The honest nodes finalize
// one chain, with valid certificates and no block built by node 8.
func TestT7ByzantineValidators(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t7_byzantine")
	res, log := testutil.Run(t, cfg, "T7_byzantine")
	checkT7(t, cfg, res, log)
	for _, want := range []string{"type=BODY from=08 reason=bad_signature",
		"type=BODY from=08 reason=bad_state_hash", "type=PROPOSAL from=08 reason=bad_parent"} {
		for _, id := range res.Honest {
			if !hasLine(log, fmt.Sprintf("node=%02d ev=REJECT", id), want) {
				t.Fatalf("node %d: no REJECT %s", id, want)
			}
		}
	}
	for h := uint64(1); h <= cfg.MaxHeight; h++ {
		if e, _ := res.Nodes[0].Ledger.Get(h); e.Block.Header.Proposer == 8 {
			t.Fatalf("height %d: a block built by the invalid proposer was finalized", h)
		}
	}
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

// T7 with 20% loss before GST (10 s). With "first copy wins" counting this
// run never finalized block 1: one honest node locked on a block whose POL
// included the equivocators' real votes, while the others held their NIL
// copies and could never see that POL. Counting every signed vote once per
// tally makes all nodes agree on what each validator signed.
func TestT7EquivocationWithDropsStaysLive(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t7_byzantine")
	cfg.Seed = 1
	cfg.Network.GSTMs = 10_000
	cfg.Network.PreGST = simconfig.PreGSTConfig{DropProb: 0.2, DupProb: 0.1, MaxDelayMs: 600}
	res, log := testutil.Run(t, cfg, "T7_byzantine_drops")
	checkT7(t, cfg, res, log)
}
