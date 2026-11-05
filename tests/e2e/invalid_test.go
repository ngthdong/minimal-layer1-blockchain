package e2e

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T3: clients inject txs with a corrupted signature, a signature made under
// the VOTE domain, and a write to another account's key (3 copies each, to
// random nodes). Every node that receives one logs REJECT with the exact
// reason and drops it: it is never relayed, never in a block. The chain
// is therefore IDENTICAL to the same run without the injections.
func TestT3InvalidTransactions(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t3_invalid_sig")
	res, log := testutil.Run(t, cfg, "T3_invalid_sig")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	for _, reason := range []string{"bad_signature", "wrong_domain", "key_not_owned_by_sender"} {
		if testutil.CountLines(log, "from=client reason="+reason) == 0 {
			t.Fatalf("no REJECT with reason=%s from a client", reason)
		}
	}
	for _, line := range bytes.Split(log, []byte("\n")) {
		if bytes.Contains(line, []byte("ev=REJECT")) && !bytes.Contains(line, []byte("from=client")) {
			t.Fatalf("a bad tx travelled between nodes: %s", line)
		}
	}

	clean := testutil.LoadConfig(t, "t3_invalid_sig")
	clean.Workload.Inject = nil
	ref, _ := testutil.Run(t, clean, "T3_without_injections")
	if res.FinalStateHash != ref.FinalStateHash {
		t.Fatal("bad txs changed the final state")
	}
	for h := uint64(1); h <= cfg.MaxHeight; h++ {
		a, _ := res.Nodes[0].Ledger.BlockHash(h)
		b, _ := ref.Nodes[0].Ledger.BlockHash(h)
		if a != b {
			t.Fatalf("height %d: chain differs from the run without injections", h)
		}
	}
}
