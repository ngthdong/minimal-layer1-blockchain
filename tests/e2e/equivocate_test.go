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
