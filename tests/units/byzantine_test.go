package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/byzantine"
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
	if b, err := byzantine.New("honest"); err != nil || len(b.Outgoing(1, m)) != 1 {
		t.Fatal("honest must pass messages through")
	}
	if _, err := byzantine.New("teleport"); err == nil {
		t.Fatal("unknown behaviour accepted")
	}
}
