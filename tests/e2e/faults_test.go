package e2e

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// T2: half of all messages are delivered twice and delays vary from 10 to
// 500 ms (heavy reordering), for the whole run. A duplicate must never count
// as a second vote: no EQUIVOCATION, and every commit certificate holds each
// validator at most once.
func TestT2DuplicatesAndReordering(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t2_dup_reorder")
	res, log := testutil.Run(t, cfg, "T2_dup_reorder")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	if testutil.CountLines(log, "ev=DUP") == 0 {
		t.Fatal("the scenario duplicated nothing")
	}
	if n := testutil.CountLines(log, "ev=EQUIVOCATION"); n != 0 {
		t.Fatalf("%d duplicates were taken for equivocations", n)
	}
	assertCertificates(t, res)
}

// T5: before GST (15 s) 80% of messages are lost. Without rebroadcast every
// node waits forever for votes that were dropped; with it, the chain resumes
// after GST and never finalizes two different blocks at one height.
func TestT5DropsBeforeGST(t *testing.T) {
	cfg := testutil.LoadConfig(t, "t5_presync_drop")
	res, log := testutil.Run(t, cfg, "T5_presync_drop")
	testutil.AssertSafety(t, res)
	testutil.AssertProgress(t, res, cfg.MaxHeight)
	if testutil.CountLines(log, "ev=DROP") == 0 || testutil.CountLines(log, "ev=TIMEOUT") == 0 {
		t.Fatal("the scenario dropped nothing or never timed out")
	}
	assertCertificates(t, res)
}

// assertCertificates verifies the commit certificate of every finalized block
// of every honest node: 2f+1 valid precommits from distinct validators.
func assertCertificates(t *testing.T, res *sim.Result) {
	t.Helper()
	vals, err := consensus.NewValidatorSet(res.Validators)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range res.Honest {
		l := res.Nodes[id].Ledger
		for h := uint64(1); h <= l.Height(); h++ {
			e, _ := l.Get(h)
			if err := consensus.VerifyCommitCert(e.Block.Header.ChainID, vals, &e.Cert); err != nil {
				t.Fatalf("node %d height %d: %v", id, h, err)
			}
		}
	}
}
