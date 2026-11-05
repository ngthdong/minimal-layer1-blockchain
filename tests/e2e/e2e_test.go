package e2e

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// Running the same configuration twice gives byte-identical logs and the
// same final state hash. It must stay green after every later commit.
func TestT8Determinism(t *testing.T) {
	res1, log1 := testutil.Run(t, testutil.LoadConfig(t, "t1_normal"), "T8_run1")
	res2, log2 := testutil.Run(t, testutil.LoadConfig(t, "t1_normal"), "T8_run2")
	if len(log1) == 0 {
		t.Fatal("empty log")
	}
	if !bytes.Equal(log1, log2) {
		t.Fatal("logs are not byte-identical")
	}
	if res1.FinalStateHash != res2.FinalStateHash {
		t.Fatal("final state hashes differ")
	}
}

// Workload + gossip: every client tx reaches every node's mempool.
func TestWorkloadReachesEveryMempool(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	res, _ := testutil.Run(t, cfg, "workload")
	for _, nd := range res.Nodes {
		if nd.Mempool.Size() != cfg.Workload.TxCount {
			t.Fatalf("node %d has %d txs, want %d", nd.ID, nd.Mempool.Size(), cfg.Workload.TxCount)
		}
	}
}

// Header-first flow: every non-proposer accepts the header of height 1,
// then fetches and accepts its body; nobody rejects anything.
func TestProposalHeaderThenBodyReachesEveryNode(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	_, log := testutil.Run(t, cfg, "proposal_flow")
	want := cfg.NumValidators - 1
	for _, ev := range []string{"ev=HEADER_ACCEPT h=1 r=0", "ev=BODY_ACCEPT   h=1 r=0"} {
		if got := testutil.CountLines(log, ev); got != want {
			t.Fatalf("%q appears %d times, want %d", ev, got, want)
		}
	}
	if testutil.CountLines(log, "ev=REJECT") != 0 || testutil.CountLines(log, "ev=PROPOSE ") != 1 {
		t.Fatal("expected exactly one proposal and no rejection")
	}
}

// Every node prevotes the height-1 block, sees 2f+1 prevotes, locks and
// precommits it.
func TestEveryNodePrevotesLocksAndPrecommits(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	_, log := testutil.Run(t, cfg, "prevote_lock")
	for _, ev := range []string{"ev=PREVOTE       h=1 r=0", "ev=LOCK          h=1 r=0", "ev=PRECOMMIT     h=1 r=0"} {
		if got := testutil.CountLines(log, ev); got != cfg.NumValidators {
			t.Fatalf("%q appears %d times, want %d", ev, got, cfg.NumValidators)
		}
	}
	if testutil.CountLines(log, "block=nil") != 0 {
		t.Fatal("nobody should vote NIL in a normal run")
	}
}
