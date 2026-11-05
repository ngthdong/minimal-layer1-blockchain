package e2e

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
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

// Workload + gossip: every client tx reaches every node, where it is either
// already committed or still waiting in the mempool. The run stops as soon as
// max_height is reached, so the workload is shortened to end well before.
func TestWorkloadReachesEveryNode(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	cfg.Workload.TxCount = 20 // last tx at 2 s; the run ends later
	res, _ := testutil.Run(t, cfg, "workload")
	for _, nd := range res.Nodes {
		committed := 0
		for h := uint64(1); h <= nd.Ledger.Height(); h++ {
			e, _ := nd.Ledger.Get(h)
			committed += len(e.Block.Txs)
		}
		if committed+nd.Mempool.Size() != cfg.Workload.TxCount {
			t.Fatalf("node %d: %d committed + %d pending, want %d",
				nd.ID, committed, nd.Mempool.Size(), cfg.Workload.TxCount)
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
	if testutil.CountLines(log, "ev=REJECT") != 0 || testutil.CountLines(log, "ev=PROPOSE       h=1 r=0") != 1 {
		t.Fatal("expected exactly one proposal at height 1 and no rejection")
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

// T1: every honest node finalizes the same chain up to max_height and ends
// with the same state.
func TestT1Normal(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	res, _ := testutil.Run(t, cfg, "T1_normal")
	checkT1(t, res, cfg.MaxHeight)
}

// The same with twice the network delay: messages for the next height now
// arrive even more often before a node gets there.
func TestT1SlowerNetwork(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	cfg.Network.DeltaMs, cfg.Network.PreGST.MaxDelayMs = 400, 400
	res, _ := testutil.Run(t, cfg, "T1_delta400")
	checkT1(t, res, cfg.MaxHeight)
}

func checkT1(t *testing.T, res *sim.Result, maxHeight uint64) {
	t.Helper()
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, maxHeight)
	if res.FinalStateHash.IsZero() {
		t.Fatal("no final state hash")
	}
	for _, id := range res.Honest {
		e, _ := res.Nodes[id].Ledger.Get(maxHeight)
		if e.Block.Header.StateHash != res.FinalStateHash {
			t.Fatalf("node %d has another state at height %d", id, maxHeight)
		}
	}
}
