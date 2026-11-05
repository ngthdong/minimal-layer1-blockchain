package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/byzantine"
	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
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
