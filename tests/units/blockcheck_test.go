package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// cert returns a commit certificate signed by the given validators.
func cert(ids []types.NodeID, bh types.Hash) types.CommitCert {
	c := types.CommitCert{Height: 5, Round: 1, BlockHash: bh}
	for _, id := range ids {
		c.Precommits = append(c.Precommits, signedVote(id, types.Precommit, 5, 1, bh))
	}
	return c
}

var seven = []types.NodeID{0, 2, 3, 4, 6, 8, 9}

func TestVerifyCommitCert(t *testing.T) {
	vs := vset(t)
	good := cert(seven, blockA)
	if err := consensus.VerifyCommitCert(chain, vs, &good); err != nil {
		t.Fatalf("valid cert rejected: %v", err)
	}
	all := cert([]types.NodeID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, blockA)
	if err := consensus.VerifyCommitCert(chain, vs, &all); err != nil {
		t.Fatalf("cert with all 10 votes rejected: %v", err)
	}
}

func TestVerifyCommitCertRejects(t *testing.T) {
	vs := vset(t)
	edit := func(f func(c *types.CommitCert)) types.CommitCert {
		c := cert(seven, blockA)
		f(&c)
		return c
	}
	cases := []struct {
		name string
		c    types.CommitCert
		want error
	}{
		{"only 2f votes", cert(seven[:6], blockA), consensus.ErrCertNoQuorum},
		{"same validator twice", edit(func(c *types.CommitCert) { c.Precommits[1] = c.Precommits[0] }),
			consensus.ErrCertBadVote},
		{"unsorted", edit(func(c *types.CommitCert) {
			c.Precommits[0], c.Precommits[1] = c.Precommits[1], c.Precommits[0]
		}), consensus.ErrCertBadVote},
		{"a prevote inside", edit(func(c *types.CommitCert) {
			c.Precommits[2] = signedVote(3, types.Prevote, 5, 1, blockA)
		}), consensus.ErrCertBadVote},
		{"vote for another block", edit(func(c *types.CommitCert) {
			c.Precommits[2] = signedVote(3, types.Precommit, 5, 1, blockB)
		}), consensus.ErrCertBadVote},
		{"vote from another round", edit(func(c *types.CommitCert) {
			c.Precommits[2] = signedVote(3, types.Precommit, 5, 0, blockA)
		}), consensus.ErrCertBadVote},
		{"forged signature", edit(func(c *types.CommitCert) { c.Precommits[2].Sig[0] ^= 1 }),
			consensus.ErrCertBadVote},
		{"cert for NIL", cert(seven, types.ZeroHash), consensus.ErrCertNil},
	}
	for _, c := range cases {
		if err := consensus.VerifyCommitCert(chain, vs, &c.c); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// validBody returns txs and a header whose TxRoot and StateHash match them
// when executed on st.
func validBody(t *testing.T, st *state.State) (types.BlockHeader, []types.Tx) {
	t.Helper()
	txs := []types.Tx{goodTx("Alice", 1, "a"), goodTx("Bob", 1, "b")}
	after := st.Clone()
	if err := after.ApplyBlock(chain, registry(), txs); err != nil {
		t.Fatal(err)
	}
	return types.BlockHeader{ChainID: chain, Height: 1, TxRoot: types.TxRoot(txs), StateHash: after.Hash()}, txs
}

func TestExecuteBodyAcceptsValidBlock(t *testing.T) {
	st := state.New()
	before := st.Hash()
	hdr, txs := validBody(t, st)
	next, err := consensus.ExecuteBody(chain, registry(), st, &hdr, txs, 10)
	if err != nil {
		t.Fatal(err)
	}
	if next.Hash() != hdr.StateHash || st.Hash() != before {
		t.Fatal("must return the post-state and leave the input state alone")
	}
	empty := types.BlockHeader{TxRoot: types.TxRoot(nil), StateHash: before}
	if _, err := consensus.ExecuteBody(chain, registry(), st, &empty, nil, 10); err != nil {
		t.Fatalf("empty block rejected: %v", err)
	}
}

func TestExecuteBodyRejects(t *testing.T) {
	st, reg := state.New(), registry()
	before := st.Hash()

	hdr, txs := validBody(t, st)
	if _, err := consensus.ExecuteBody(chain, reg, st, &hdr, txs, 1); !errors.Is(err, consensus.ErrTooManyTxs) {
		t.Fatalf("too many txs: %v", err)
	}

	swapped := []types.Tx{txs[1], txs[0]} // body does not match the signed TxRoot
	if _, err := consensus.ExecuteBody(chain, reg, st, &hdr, swapped, 10); !errors.Is(err, consensus.ErrBadTxRoot) {
		t.Fatalf("other tx order: %v", err)
	}

	badHdr := hdr
	badHdr.StateHash[0] ^= 1 // proposer lies about the result
	if _, err := consensus.ExecuteBody(chain, reg, st, &badHdr, txs, 10); !errors.Is(err, consensus.ErrBadStateHash) {
		t.Fatalf("wrong state hash: %v", err)
	}

	forged := []types.Tx{goodTx("Alice", 1, "a")}
	forged[0].Value = []byte("changed after signing")
	fHdr := types.BlockHeader{TxRoot: types.TxRoot(forged)} // root honestly covers the bad tx
	_, err := consensus.ExecuteBody(chain, reg, st, &fHdr, forged, 10)
	var txErr *state.TxError
	if !errors.As(err, &txErr) || !errors.Is(err, state.ErrBadSignature) {
		t.Fatalf("forged tx in body: %v", err)
	}

	if st.Hash() != before {
		t.Fatal("a rejected body changed the state")
	}
}
