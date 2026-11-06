package unit

import (
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
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

// finalChain returns n consecutive finalized blocks (one Alice tx each) as a
// peer serves them: the SyncResponses (header + certificate) and, on
// request, the BodyResponses.
func finalChain(n int) ([]types.SyncResponse, []types.BodyResponse) {
	st, parent := state.New(), types.ZeroHash
	var heads []types.SyncResponse
	var bodies []types.BodyResponse
	for h := uint64(1); h <= uint64(n); h++ {
		txs := []types.Tx{goodTx("Alice", h, "v")}
		if err := st.ApplyBlock(chain, registry(), txs); err != nil {
			panic(err)
		}
		hdr := types.BlockHeader{ChainID: chain, Height: h, ParentHash: parent,
			TxRoot: types.TxRoot(txs), StateHash: st.Hash()}
		consensus.NewVoteSigner(1, vPrivs[1], chain).SignHeader(&hdr)
		parent = hdr.Hash()
		heads = append(heads, types.SyncResponse{Header: hdr, Cert: certAt(h, parent, 1, 2, 3, 4, 5, 6, 7)})
		bodies = append(bodies, types.BodyResponse{Height: h, BlockHash: parent, Txs: txs})
	}
	return heads, bodies
}

// Sync is header first: a body is requested only for the accepted header of
// the next height, and a body sent before that is ignored. Headers that the
// network delivers early wait for the missing one.
func TestSyncHeaderBeforeBody(t *testing.T) {
	_, nodes, readLog := nodeNet(2, 0)
	nd := nodes[0]
	nd.Engine = consensus.NewEngine(0, chain, vset(t), consensus.NewVoteSigner(0, vPrivs[0], chain),
		testConsensusConfig, nd)
	nd.Engine.StartHeight(1)
	heads, bodies := finalChain(3)
	nd.HandleMessage(types.Envelope{From: 1, To: 0, Msg: &bodies[0]}) // no header yet
	for _, i := range []int{2, 1} {
		nd.HandleMessage(types.Envelope{From: 1, To: 0, Msg: &heads[i]})
	}
	if nd.Ledger.Height() != 0 || strings.Contains(readLog(), "type=BODY_REQ") {
		t.Fatal("a body was used or requested before the header of height 1")
	}
	nd.HandleMessage(types.Envelope{From: 1, To: 0, Msg: &heads[0]})
	if strings.Count(readLog(), "type=BODY_REQ") != 1 {
		t.Fatalf("accepted header 1: want one body request: %s", readLog())
	}
	// The body is lost; the header comes again (next sync answer): ask again.
	nd.HandleMessage(types.Envelope{From: 1, To: 0, Msg: &heads[0]})
	if strings.Count(readLog(), "type=BODY_REQ") != 2 {
		t.Fatal("a lost body was never requested again")
	}
	for i := range bodies {
		nd.HandleMessage(types.Envelope{From: 1, To: 0, Msg: &bodies[i]})
	}
	if nd.Ledger.Height() != 3 || nd.Engine.Height != 4 {
		t.Fatalf("ledger %d, engine at %d: want 3 and 4", nd.Ledger.Height(), nd.Engine.Height)
	}
}

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
	short := certFor(a, 1, 2, 3, 4, 5, 6)
	if e.AcceptSyncedHeader(6, &hdr, &short) == nil || !env.logged("REJECT h=1 r=0 type SYNC from 06 reason cert_no_quorum") {
		t.Fatal("a header without a real certificate was accepted")
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
	e.OnVote(3, &next) // relayed by 3, signed by 5: ask the signer
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
