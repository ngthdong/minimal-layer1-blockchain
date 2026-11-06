package e2e

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// withByzantine is T1 with one behaviour given to node 3.
func withByzantine(t *testing.T, behavior string) *simconfig.Config {
	cfg := testutil.LoadConfig(t, "t1_normal")
	cfg.Byzantine = []simconfig.ByzantineSpec{{Node: 3, Behavior: behavior}}
	return cfg
}

// dropLines removes the lines containing any of the given events.
func dropLines(log []byte, events ...string) []byte {
	var out [][]byte
	for _, l := range bytes.SplitAfter(log, []byte("\n")) {
		keep := true
		for _, ev := range events {
			keep = keep && !bytes.Contains(l, []byte("ev="+ev+" "))
		}
		if keep {
			out = append(out, l)
		}
	}
	return bytes.Join(out, nil)
}

// The send hook itself is invisible: with behaviour "honest" the run is T1
// line for line (it may stop a few ms earlier, since node 3 is no longer
// waited for, and it has one BYZANTINE line).
func TestHonestHookChangesNothing(t *testing.T) {
	_, ref := testutil.Run(t, testutil.LoadConfig(t, "t1_normal"), "T1_reference")
	res, log := testutil.Run(t, withByzantine(t, "honest"), "T1_honest_hook")
	got, want := dropLines(log, "BYZANTINE", "FINAL"), dropLines(ref, "FINAL")
	if len(got) < len(want)*9/10 || !bytes.HasPrefix(want, got) {
		t.Fatal("the honest hook changed the run")
	}
	testutil.AssertSafety(t, res)
}

// T1 with node 3 silent from the start: it never sends anything; at h=3,
// where it is the round-0 proposer, the others time out and commit in
// round 1. All txs still get in (clients avoid the silent node).
func TestT1WithSilentValidator(t *testing.T) {
	cfg := withByzantine(t, "silent")
	res, log := testutil.Run(t, cfg, "T1_silent_node3")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	if testutil.CountLines(log, "node=03 ev=SEND") != 0 {
		t.Fatal("the silent node sent something")
	}
	if !hasLine(log, "ev=TIMEOUT", "h=3 r=0 step=propose") || !hasLine(log, "ev=COMMIT", "h=3 r=1") {
		t.Fatal("no timeout and round-1 commit at the silent proposer's height")
	}
	for _, id := range res.Honest {
		if n := res.Nodes[id].Mempool.Size(); n != 0 {
			t.Fatalf("node %d still holds %d txs", id, n)
		}
	}
}

// T1 with node 3 proposing an invalid block whenever it is proposer (h = 3,
// 13, 23: a bad tx signature, a wrong state hash, a wrong parent). Every
// other node refuses it with the reason logged, prevotes NIL, and the next
// round commits a valid block (rule 3). No block built by node 3 is final.
func TestT1WithInvalidProposer(t *testing.T) {
	cfg := withByzantine(t, "invalid_proposal")
	cfg.MaxHeight = 25
	res, log := testutil.Run(t, cfg, "T1_invalid_proposer3")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	for _, want := range []string{"type=BODY from=03 reason=bad_signature",
		"type=BODY from=03 reason=bad_state_hash", "type=PROPOSAL from=03 reason=bad_parent"} {
		for _, id := range res.Honest {
			if !hasLine(log, fmt.Sprintf("node=%02d ev=REJECT", id), want) {
				t.Fatalf("node %d: no REJECT %s", id, want)
			}
		}
	}
	for h := uint64(1); h <= cfg.MaxHeight; h++ {
		if e, _ := res.Nodes[0].Ledger.Get(h); e.Block.Header.Proposer == 3 {
			t.Fatalf("height %d: a block built by the invalid proposer was finalized", h)
		}
	}
}
