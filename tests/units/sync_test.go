package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// certAt builds a commit certificate for bh at (h, 0) signed by ids (sorted).
func certAt(h uint64, bh types.Hash, ids ...types.NodeID) types.CommitCert {
	c := types.CommitCert{Height: h, Round: 0, BlockHash: bh}
	for _, id := range ids {
		c.Precommits = append(c.Precommits, signedVote(id, types.Precommit, h, 0, bh))
	}
	return c
}

func certFor(bh types.Hash, ids ...types.NodeID) types.CommitCert { return certAt(1, bh, ids...) }

// A synced block is committed only with a real certificate for exactly that
// block, and only if its body executes to the header's state hash.
func TestCommitSyncedTrustsOnlyTheCertificate(t *testing.T) {
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	hdr, txs := block(1, goodTx("Alice", 1, "a"))
	a := hdr.Hash()
	good := types.Block{Header: hdr, Txs: txs}
	wrongTxs := types.Block{Header: hdr, Txs: []types.Tx{goodTx("Bob", 1, "b")}}
	cases := []struct {
		b      types.Block
		cert   types.CommitCert
		reason string
	}{
		{good, certFor(a, 1, 2, 3, 4, 5, 6), "cert_no_quorum"}, // 2f only
		{good, certFor(blockB, 1, 2, 3, 4, 5, 6, 7), "sync_cert_mismatch"},
		{wrongTxs, certFor(a, 1, 2, 3, 4, 5, 6, 7), "bad_tx_root"}, // right header, other body
	}
	for _, c := range cases {
		if e.CommitSynced(5, &c.b, &c.cert) == nil || !env.logged("REJECT h=1 r=0 type SYNC from 05 reason "+c.reason) {
			t.Fatalf("%s: accepted or not logged: %q", c.reason, env.logs)
		}
	}
	if e.Height != 1 || len(env.commits) != 0 {
		t.Fatal("a rejected sync block was committed")
	}
	cert := certFor(a, 1, 2, 3, 4, 5, 6, 7)
	if err := e.CommitSynced(5, &good, &cert); err != nil {
		t.Fatal(err)
	}
	if e.Height != 2 || len(env.commits) != 1 || !env.logged("COMMIT h=1 r=0 block "+a.Short()+" txs 1") ||
		!env.logged("NEW_ROUND h=2 r=0") {
		t.Fatalf("h=%d logs=%q", e.Height, env.logs)
	}
}

// When to ask for sync: a message two heights ahead at once; a message one
// height ahead is normal, and only asks once our round is stuck.
func TestWhenToRequestSync(t *testing.T) {
	e, env, _ := readyEngine(t) // height 1, prevoted
	next := signedVote(5, types.Prevote, 2, 0, blockB)
	e.OnVote(5, &next)
	if len(env.syncs) != 0 {
		t.Fatal("a message from h+1 alone must not trigger sync")
	}
	e.OnTimeout(consensus.Timeout{Height: 1, Round: 0, Step: consensus.StepPropose}) // stuck → rebroadcast
	if len(env.syncs) != 1 || env.syncs[0] != 5 {
		t.Fatalf("stuck with peers at h+1: syncs = %v, want [5]", env.syncs)
	}
	far := signedVote(6, types.Prevote, 3, 0, blockB)
	e.OnVote(6, &far)
	if len(env.syncs) != 2 || env.syncs[1] != 6 {
		t.Fatalf("a message from h+2: syncs = %v, want [5 6]", env.syncs)
	}
}
