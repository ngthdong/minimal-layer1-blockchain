package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/byzantine"
	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// The Outbound hook decides what really leaves the node: here nothing from
// node 0, and two copies of everything from node 1. Other nodes are untouched.
func TestOutboundHookRewritesSends(t *testing.T) {
	sched, net, got := testNet(4, 1)
	net.SetOutbound(0, byzantine.Silent{}.Outgoing)
	net.SetOutbound(1, func(_ types.NodeID, m types.Message) []types.Message { return []types.Message{m, m} })
	net.Broadcast(0, voteMsg(0, 0))
	net.Broadcast(1, voteMsg(1, 0))
	net.Broadcast(2, voteMsg(2, 0))
	sched.Run(1000, nil)
	from := map[types.NodeID]int{}
	for _, a := range *got {
		from[a.env.From]++
	}
	if from[0] != 0 || from[1] != 6 || from[2] != 3 {
		t.Fatalf("deliveries by sender = %v, want 0:0 1:6 2:3", from)
	}
}

// Before from_ms the node is honest; from then on its behaviour applies.
func TestHookStartsAtFromMs(t *testing.T) {
	now := uint64(0)
	hook := byzantine.Hook(byzantine.Silent{}, 100, func() uint64 { return now })
	m := voteMsg(3, 0)
	if out := hook(1, m); len(out) != 1 || out[0] != m {
		t.Fatal("before from_ms the message must go out unchanged")
	}
	now = 100
	if out := hook(1, m); len(out) != 0 {
		t.Fatal("from from_ms on a silent node sends nothing")
	}
	if b, err := byzantine.New("honest", byzantine.Identity{}); err != nil || len(b.Outgoing(1, m)) != 1 {
		t.Fatal("honest must pass messages through")
	}
	if _, err := byzantine.New("teleport", byzantine.Identity{}); err == nil {
		t.Fatal("unknown behaviour accepted")
	}
}

// Equivocate: its own votes reach the upper half (id >= n/2) as a second,
// validly signed vote for the other choice; everything else is unchanged.
func TestEquivocateSplitsItsOwnVotes(t *testing.T) {
	self := byzantine.Identity{ID: 3, Priv: vPrivs[3], ChainID: chain, N: 10}
	b, err := byzantine.New("equivocate", self)
	if err != nil {
		t.Fatal(err)
	}
	vs := vset(t)
	real := &types.VoteMsg{Vote: signedVote(3, types.Prevote, 1, 0, blockB)}
	low, high := b.Outgoing(4, real), b.Outgoing(5, real)
	if len(low) != 1 || low[0] != real {
		t.Fatal("the lower half must get the real vote")
	}
	other := high[0].(*types.VoteMsg).Vote
	if other.BlockHash != types.ZeroHash || other.Round != 0 || other.Type != types.Prevote ||
		consensus.VerifyVote(chain, vs, &other) != nil {
		t.Fatalf("the upper half must get a valid NIL prevote of the same step: %+v", other)
	}
	nilVote := &types.VoteMsg{Vote: signedVote(3, types.Precommit, 1, 0, types.ZeroHash)}
	if v := b.Outgoing(9, nilVote)[0].(*types.VoteMsg).Vote; v.BlockHash.IsZero() {
		t.Fatal("a NIL vote must be doubled with a block vote")
	}
	relayed := &types.VoteMsg{Vote: signedVote(6, types.Prevote, 1, 0, blockB)} // someone else's
	if out := b.Outgoing(9, relayed); out[0] != relayed {
		t.Fatal("votes of other validators are relayed unchanged")
	}
}

// InvalidProposal: block 1 breaks a tx signature (the body still matches its
// TxRoot), block 2 the state hash, block 3 the parent; then it starts again.
// An empty block cannot carry a bad tx and gets a bad state hash instead.
func TestInvalidProposalKinds(t *testing.T) {
	b, err := byzantine.New("invalid_proposal", byzantine.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	c := b.(byzantine.BlockCorrupter)
	hdr, txs := block(8, goodTx("Alice", 1, "a"))
	good := types.Block{Header: hdr, Txs: txs}
	execute := func(blk types.Block) error {
		_, err := consensus.ExecuteBody(chain, registry(), state.New(), &blk.Header, blk.Txs, 10)
		return err
	}
	if b1 := c.CorruptBlock(good); !errors.Is(execute(b1), state.ErrBadSignature) {
		t.Fatalf("block 1: %v, want bad_signature", execute(b1))
	}
	if b2 := c.CorruptBlock(good); !errors.Is(execute(b2), consensus.ErrBadStateHash) {
		t.Fatalf("block 2: %v, want bad_state_hash", execute(b2))
	}
	if b3 := c.CorruptBlock(good); b3.Header.ParentHash == good.Header.ParentHash || execute(b3) != nil {
		t.Fatal("block 3 must keep a valid body but point to another parent")
	}
	empty := types.Block{Header: types.BlockHeader{ChainID: chain, Height: 1, TxRoot: types.TxRoot(nil),
		StateHash: state.New().Hash()}}
	if b4 := c.CorruptBlock(empty); !errors.Is(execute(b4), consensus.ErrBadStateHash) {
		t.Fatalf("empty block 4: %v, want bad_state_hash", execute(b4))
	}
	if execute(good) != nil {
		t.Fatal("CorruptBlock must not change the block it was given")
	}
}
