package unit

import (
	"errors"
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

// Each injected kind fails exactly the check it targets, carries the nonce
// of its account's next real tx, and leaves the normal plan untouched.
func TestInjectedTxs(t *testing.T) {
	normal, cfg := planFor(t)
	cfg.Workload.Inject = []simconfig.InjectTx{
		{Kind: "bad_signature", AtMs: 1000, Account: "Alice", Repeat: 2},
		{Kind: "wrong_domain", AtMs: 2000, Account: "Bob"},
		{Kind: "foreign_key", AtMs: 3000, Account: "Carol"},
	}
	accts := workload.Accounts(cfg.Seed, cfg.Workload.Accounts)
	plan := workload.Plan(cfg, accts, sim.NewRNG(cfg.Seed, "workload"))
	if len(plan) != len(normal)+4 {
		t.Fatalf("%d submissions, want %d", len(plan), len(normal)+4)
	}
	reg := workload.Registry(accts)
	want := map[string]error{"inject-0-bad_signature": state.ErrBadSignature,
		"inject-1-wrong_domain": state.ErrWrongDomain, "inject-2-foreign_key": state.ErrNotOwner}
	var kept []workload.Submission
	for i, s := range plan {
		if i > 0 && s.AtMs < plan[i-1].AtMs {
			t.Fatal("plan not in time order")
		}
		reason, bad := want[string(s.Tx.Value)]
		if !bad {
			kept = append(kept, s)
			continue
		}
		if err := state.CheckTx(cfg.ChainID, reg, &s.Tx); !errors.Is(err, reason) {
			t.Fatalf("%s: err = %v, want %v", s.Tx.Value, err, reason)
		}
		// AtMs is a multiple of interval_ms (100): 10 normal txs before 1000 ms,
		// i.e. nonces 1..4 for Alice (txs 0,3,6,9) → the next one is 5.
		if s.AtMs == 1000 && s.Tx.Nonce != 5 {
			t.Fatalf("bad tx nonce %d, want Alice's next nonce 5", s.Tx.Nonce)
		}
	}
	if !reflect.DeepEqual(kept, normal) {
		t.Fatal("injections changed the normal plan")
	}
}
