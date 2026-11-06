package e2e

import (
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T1 with node 4 sending every message 20 times: every honest node blocks
// it within its first second (500 messages per window), unblocks it after
// 3 s and blocks it again. Its blocked votes and its proposal at h=4 are
// lost, which the protocol tolerates. No honest node is ever blocked
// (testutil.Run checks that on every scenario).
func TestSpammerIsBlocked(t *testing.T) {
	cfg := withByzantine(t, "spam")
	cfg.Byzantine[0].Node = 4
	res, log := testutil.Run(t, cfg, "T1_spammer_node4")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	for _, id := range res.Honest {
		if !hasLine(log, fmt.Sprintf("node=%02d ev=BLOCK", id), "peer=04") {
			t.Fatalf("node %d never blocked the spammer", id)
		}
	}
	if !hasLine(log, "ev=UNBLOCK", "peer=04") || !hasLine(log, "ev=DROP", "reason=blocked") {
		t.Fatal("no UNBLOCK or no blocked drop in the log")
	}
}
