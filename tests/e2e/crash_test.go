package e2e

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T6: f = 3 validators crash (node 3 at 0 s, 6 at 2 s, 9 at 4 s); each is the
// round-0 proposer of a later height (h = 3, 6, 9). There the 7 survivors
// time out, prevote NIL, move to round 1 and finalize with the next
// proposer: exactly 2f+1 nodes are enough.
func TestT6CrashedProposers(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t6_silent_proposer")
	res, log := testutil.Run(t, cfg, "T6_silent_proposer")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	for _, h := range []int{3, 6, 9} {
		for _, want := range []string{
			fmt.Sprintf("ev=TIMEOUT       h=%d r=0 step=propose", h),
			fmt.Sprintf("ev=NEW_ROUND     h=%d r=1", h),
			fmt.Sprintf("ev=COMMIT        h=%d r=1", h),
		} {
			if testutil.CountLines(log, want) == 0 {
				t.Fatalf("no %q in the log", want)
			}
		}
	}
	for _, b := range cfg.Byzantine {
		id := b.Node
		crash := bytes.Index(log, []byte(fmt.Sprintf("node=%02d ev=CRASH", id)))
		final := bytes.Index(log, []byte(fmt.Sprintf("node=%02d ev=FINAL", id)))
		if crash < 0 || bytes.Contains(log[crash+1:final], []byte(fmt.Sprintf("node=%02d ", id))) {
			t.Fatalf("node %d logged something between CRASH and FINAL", id)
		}
		// What a node finalized before crashing is part of the common chain.
		l, ref := res.Nodes[id].Ledger, res.Nodes[res.Honest[0]].Ledger
		for h := uint64(1); h <= l.Height(); h++ {
			a, _ := l.BlockHash(h)
			b, _ := ref.BlockHash(h)
			if a != b {
				t.Fatalf("crashed node %d disagrees at height %d", id, h)
			}
		}
	}
	for _, id := range res.Honest {
		if n := res.Nodes[id].Mempool.Size(); n != 0 {
			t.Fatalf("node %d still holds %d txs: client failover lost some", id, n)
		}
	}
}
