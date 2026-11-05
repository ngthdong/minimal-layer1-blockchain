package e2e

import (
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T7: f = 3 validators (2, 5, 8) double-vote every prevote and precommit,
// one version to each half of the network. Every honest node sees both
// signed votes, logs EQUIVOCATION against each of them, and the honest
// nodes still finalize one chain with valid certificates.
func TestT7EquivocatingValidators(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t7_byzantine")
	res, log := testutil.Run(t, cfg, "T7_byzantine")
	checkT7(t, cfg, res, log)
}

func checkT7(t *testing.T, cfg *simconfig.Config, res *sim.Result, log []byte) {
	t.Helper()
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	assertCertificates(t, res)
	byz := map[int]bool{}
	for _, b := range cfg.Byzantine {
		byz[b.Node] = true
	}
	for _, id := range res.Honest {
		for v := 0; v < cfg.NumValidators; v++ {
			accused := hasLine(log, fmt.Sprintf("node=%02d ev=EQUIVOCATION", id), fmt.Sprintf("validator=%02d", v))
			if byz[v] && !accused {
				t.Fatalf("node %d never caught equivocator %d", id, v)
			}
			if !byz[v] && accused {
				t.Fatalf("node %d accused honest validator %d", id, v)
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
