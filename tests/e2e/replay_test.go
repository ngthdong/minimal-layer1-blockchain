package e2e

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// T4: clients resend old txs byte for byte: Alice's last tx at 2 s to 3
// other nodes (probably still pending), Carol's last tx at 5 s to all 9
// other nodes (long committed). Every copy is refused (duplicate_tx); each
// of the 30 txs is in the chain exactly once; the state is the one of the
// same run without replays.
func TestT4ReplayedTransactions(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t4_replay_tx")
	res, log := testutil.Run(t, cfg, "T4_replay_tx")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	for _, at := range []string{"t=0000002000 ", "t=0000005000 "} {
		if !hasLine(log, at, "from=client reason=duplicate_tx") {
			t.Fatalf("no replay refused at %s", at)
		}
	}
	for _, id := range res.Honest {
		nd, seen := res.Nodes[id], map[types.Hash]bool{}
		for h := uint64(1); h <= nd.Ledger.Height(); h++ {
			e, _ := nd.Ledger.Get(h)
			for _, tx := range e.Block.Txs {
				if seen[tx.Hash()] {
					t.Fatalf("node %d: tx %s/%d applied twice", id, tx.Sender, tx.Nonce)
				}
				seen[tx.Hash()] = true
			}
		}
		if len(seen) != cfg.Workload.TxCount || nd.Mempool.Size() != 0 {
			t.Fatalf("node %d: %d txs committed, %d pending; want %d and 0",
				id, len(seen), nd.Mempool.Size(), cfg.Workload.TxCount)
		}
	}
	clean := testutil.LoadConfig(t, "t4_replay_tx")
	clean.Workload.Inject = nil
	if ref, _ := testutil.Run(t, clean, "T4_without_replays"); ref.FinalStateHash != res.FinalStateHash {
		t.Fatal("replays changed the final state")
	}
}

// hasLine reports whether one log line contains every part.
func hasLine(log []byte, parts ...string) bool {
	for _, line := range bytes.Split(log, []byte("\n")) {
		all := true
		for _, p := range parts {
			all = all && bytes.Contains(line, []byte(p))
		}
		if all {
			return true
		}
	}
	return false
}
