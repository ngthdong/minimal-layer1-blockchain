package e2e

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T5 with only 10% loss before GST: unlike 80%, the chain does progress then,
// and the nodes that miss a decision fall more than one height behind. They
// can only catch up by block sync: blocks plus commit certificates, each
// verified before it is committed.
func TestT5LaggingNodesCatchUpBySync(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t5_lagging_sync") // t5_presync_drop with 10% loss
	res, log := testutil.Run(t, cfg, "T5_lagging_sync")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	if testutil.CountLines(log, "sync_from=") == 0 {
		t.Fatal("no block was committed through sync: the scenario does not test it")
	}
	if n := testutil.CountLines(log, "type=SYNC from="); n != 0 {
		t.Fatalf("%d sync blocks rejected from honest peers", n)
	}
	assertCertificates(t, res)
}
