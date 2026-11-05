package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Compile-time check: every message type implements types.Message.
var _ = []types.Message{
	(*types.TxMsg)(nil), (*types.ProposalMsg)(nil), (*types.VoteMsg)(nil),
	(*types.BodyRequest)(nil), (*types.BodyResponse)(nil),
	(*types.SyncRequest)(nil), (*types.SyncResponse)(nil),
}

func sampleMessages() []types.Message {
	tx := sampleTx("Alice", 1, "hello")
	tx.Sig = []byte{1, 2, 3}
	h := sampleHeader()
	h.Sig = []byte{4, 5, 6}
	v := sampleVote()
	v.Sig = []byte{7, 8, 9}
	return []types.Message{
		&types.TxMsg{Tx: tx},
		&types.ProposalMsg{Proposal: types.Proposal{Height: 7, POLRound: -1, Header: h, Sig: []byte{1}}},
		&types.VoteMsg{Vote: v},
		&types.BodyRequest{Height: 7, BlockHash: h.Hash()},
		&types.BodyResponse{Height: 7, BlockHash: h.Hash(), Txs: []types.Tx{tx}},
		&types.SyncRequest{FromHeight: 3},
		&types.SyncResponse{
			Block: types.Block{Header: h, Txs: []types.Tx{tx}},
			Cert:  types.CommitCert{Height: 7, BlockHash: h.Hash(), Precommits: []types.Vote{v}},
		},
	}
}

func TestKindsAreDistinctAndNamed(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range sampleMessages() {
		name := m.Kind().String()
		if name == "UNKNOWN" || seen[name] {
			t.Fatalf("kind %d has bad or duplicate name %q", m.Kind(), name)
		}
		seen[name] = true
	}
	if types.MsgKind(0).String() != "UNKNOWN" || types.MsgKind(200).String() != "UNKNOWN" {
		t.Fatal("unused kinds must be UNKNOWN")
	}
}

func TestIDsAreStableAndUnique(t *testing.T) {
	ids := map[types.Hash]string{}
	for _, m := range sampleMessages() {
		if m.ID() != m.ID() {
			t.Fatalf("%s: ID not stable", m.Kind())
		}
		if other, dup := ids[m.ID()]; dup {
			t.Fatalf("%s and %s share an ID", m.Kind(), other)
		}
		ids[m.ID()] = m.Kind().String()
	}
}

// Changing every slice inside a clone must leave the original untouched.
func TestCloneIsDeepAndKeepsID(t *testing.T) {
	for _, m := range sampleMessages() {
		before := m.ID()
		c := m.Clone()
		if c.ID() != before {
			t.Fatalf("%s: clone has a different ID", m.Kind())
		}
		switch x := c.(type) {
		case *types.TxMsg:
			x.Tx.Sig[0]++
			x.Tx.Value[0]++
		case *types.ProposalMsg:
			x.Proposal.Sig[0]++
			x.Proposal.Header.Sig[0]++
		case *types.VoteMsg:
			x.Vote.Sig[0]++
		case *types.BodyResponse:
			x.Txs[0].Value[0]++
		case *types.SyncResponse:
			x.Block.Header.Sig[0]++
			x.Block.Txs[0].Sig[0]++
			x.Cert.Precommits[0].Sig[0]++
		default:
			continue // no slices inside
		}
		if m.ID() != before {
			t.Fatalf("%s: editing the clone changed the original", m.Kind())
		}
		if c.ID() == before {
			t.Fatalf("%s: ID does not cover the edited fields", m.Kind())
		}
	}
}

// T7: two conflicting votes from one validator must both be delivered.
func TestEquivocatingVotesHaveDifferentIDs(t *testing.T) {
	a, b := sampleVote(), sampleVote()
	b.BlockHash[0] ^= 1
	if (&types.VoteMsg{Vote: a}).ID() == (&types.VoteMsg{Vote: b}).ID() {
		t.Fatal("conflicting votes share a gossip ID")
	}
}

// A bad body sent first must not shadow the correct body for the same block.
func TestBodyResponseIDCoversTxs(t *testing.T) {
	h := sampleHeader()
	bh := h.Hash()
	good := &types.BodyResponse{Height: 7, BlockHash: bh, Txs: []types.Tx{sampleTx("Alice", 1, "x")}}
	bad := &types.BodyResponse{Height: 7, BlockHash: bh, Txs: []types.Tx{sampleTx("Alice", 1, "y")}}
	if good.ID() == bad.ID() {
		t.Fatal("bodies with different txs share an ID")
	}
}

func TestMsgHeight(t *testing.T) {
	want := []uint64{0, 7, 5, 7, 7, 3, 7}
	for i, m := range sampleMessages() {
		if m.MsgHeight() != want[i] {
			t.Fatalf("%s: MsgHeight = %d, want %d", m.Kind(), m.MsgHeight(), want[i])
		}
	}
}
