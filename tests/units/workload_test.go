package unit

import (
	"reflect"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/workload"
)

func planFor(t *testing.T) ([]workload.Submission, *simconfig.Config) {
	t.Helper()
	cfg, err := simconfig.Load(t1Path)
	if err != nil {
		t.Fatal(err)
	}
	accts := workload.Accounts(cfg.Seed, cfg.Workload.Accounts)
	return workload.Plan(cfg, accts, sim.NewRNG(cfg.Seed, "workload")), cfg
}

func TestPlanIsDeterministic(t *testing.T) {
	a, _ := planFor(t)
	b, _ := planFor(t)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same config gave a different workload plan")
	}
}

// Every planned tx is valid, nonces of each account are 1, 2, 3, ... and
// submissions are in time order, spread over the nodes.
func TestPlanProducesValidOrderedTxs(t *testing.T) {
	plan, cfg := planFor(t)
	if len(plan) != cfg.Workload.TxCount {
		t.Fatalf("%d submissions, want %d", len(plan), cfg.Workload.TxCount)
	}
	reg := workload.Registry(workload.Accounts(cfg.Seed, cfg.Workload.Accounts))
	s := state.New()
	targets := map[int]bool{}
	for i, sub := range plan {
		if i > 0 && sub.AtMs <= plan[i-1].AtMs {
			t.Fatal("submissions not in time order")
		}
		if err := s.ApplyTx(cfg.ChainID, reg, &sub.Tx); err != nil {
			t.Fatalf("tx %d (%s nonce %d) invalid: %v", i, sub.Tx.Sender, sub.Tx.Nonce, err)
		}
		targets[int(sub.To)] = true
	}
	if len(targets) < cfg.NumValidators/2 {
		t.Fatalf("txs sent to only %d distinct nodes", len(targets))
	}
}
