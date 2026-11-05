package e2e

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T1 squeezed through a rate limit of 1 message per ms per link with room
// for 1 waiting message: many messages wait (DELAY) and hundreds are
// dropped (DROP reason=rate), yet rebroadcast and sync keep the chain
// alive and safe.
func TestT1UnderTightRateLimit(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t1_normal")
	cfg.Network.RateLimit = simconfig.RateLimit{Capacity: 1, RefillPerMs: 1, QueueLimit: 1}
	res, log := testutil.Run(t, cfg, "T1_rate_limited")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	if !hasLine(log, "ev=DELAY", "reason=rate") || !hasLine(log, "ev=DROP", "reason=rate") {
		t.Fatal("the rate limit never held back or dropped a message")
	}
}
