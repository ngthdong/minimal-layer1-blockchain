package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

type arrival struct {
	at  uint64
	env types.Envelope
}

// testNet builds a network of n nodes (full mesh) whose handlers record what
// arrives and when.
func testNet(n int, seed uint64) (*sim.Scheduler, *network.Network, *[]arrival) {
	sched := sim.NewScheduler()
	log := logging.New(&bytes.Buffer{}, sched.Now)
	cfg := simconfig.NetworkConfig{MinDelayMs: 10, DeltaMs: 50}
	net := network.New(cfg, sched, sim.NewRNG(seed, "network"), log)
	got := &[]arrival{}
	for id, peers := range network.FullMesh(n) {
		net.SetPeers(id, peers)
		net.Register(id, func(env types.Envelope) { *got = append(*got, arrival{sched.Now(), env}) })
	}
	return sched, net, got
}

func voteMsg(id types.NodeID, round int32) *types.VoteMsg {
	v := sampleVote()
	v.Validator, v.Round, v.Sig = id, round, []byte{1, 2, 3}
	return &types.VoteMsg{Vote: v}
}

func TestDeliveryWithinDelta(t *testing.T) {
	sched, net, got := testNet(4, 1)
	for i := 0; i < 50; i++ {
		net.Send(0, 1, voteMsg(0, int32(i)))
	}
	sched.Run(1000, nil)
	if len(*got) != 50 {
		t.Fatalf("delivered %d of 50", len(*got))
	}
	for _, a := range *got {
		if a.at < 10 || a.at > 50 || a.env.From != 0 || a.env.To != 1 {
			t.Fatalf("delivered at %d from %d to %d", a.at, a.env.From, a.env.To)
		}
	}
}

// Random delays reorder messages: the order of arrival is not the order of sending.
func TestMessagesAreReordered(t *testing.T) {
	sched, net, got := testNet(2, 1)
	for i := 0; i < 20; i++ {
		net.Send(0, 1, voteMsg(0, int32(i)))
	}
	sched.Run(1000, nil)
	inOrder := true
	for i, a := range *got {
		if a.env.Msg.(*types.VoteMsg).Vote.Round != int32(i) {
			inOrder = false
		}
	}
	if inOrder {
		t.Fatal("20 messages with random delays arrived in sending order")
	}
}

func TestBroadcastReachesEveryPeerOnce(t *testing.T) {
	sched, net, got := testNet(5, 1)
	net.Broadcast(2, voteMsg(2, 0))
	sched.Run(1000, nil)
	seen := map[types.NodeID]int{}
	for _, a := range *got {
		seen[a.env.To]++
	}
	if len(seen) != 4 || seen[2] != 0 {
		t.Fatalf("receivers = %v, want 0,1,3,4 once each", seen)
	}
}

// Sender and every receiver hold separate copies.
func TestNoSharedMemoryBetweenNodes(t *testing.T) {
	sched, net, got := testNet(3, 1)
	m := voteMsg(0, 0)
	net.Broadcast(0, m)
	m.Vote.Sig[0] = 99 // sender edits its object after sending
	sched.Run(1000, nil)
	first := (*got)[0].env.Msg.(*types.VoteMsg)
	first.Vote.Sig[1] = 77 // one receiver edits its copy
	second := (*got)[1].env.Msg.(*types.VoteMsg)
	if first.Vote.Sig[0] == 99 || second.Vote.Sig[0] == 99 || second.Vote.Sig[1] == 77 {
		t.Fatal("nodes share message memory")
	}
}

func TestNetworkLogIsCompleteAndDeterministic(t *testing.T) {
	run := func() string {
		var buf bytes.Buffer
		sched := sim.NewScheduler()
		log := logging.New(&buf, sched.Now)
		net := network.New(simconfig.NetworkConfig{MinDelayMs: 10, DeltaMs: 50}, sched, sim.NewRNG(7, "network"), log)
		for id, peers := range network.FullMesh(4) {
			net.SetPeers(id, peers)
			net.Register(id, func(types.Envelope) {})
		}
		net.Broadcast(1, voteMsg(1, 3))
		net.Send(1, 9, voteMsg(1, 3)) // node 9 does not exist
		sched.Run(1000, nil)
		if err := log.Flush(); err != nil { // the logger is buffered
			t.Fatal(err)
		}
		return buf.String()
	}
	a, b := run(), run()
	if a != b {
		t.Fatal("same seed gave a different network log")
	}
	if strings.Count(a, "ev=SEND") != 4 || strings.Count(a, "ev=RECV") != 3 ||
		!strings.Contains(a, "ev=DROP          h=5 r=3 type=VOTE") || !strings.Contains(a, "reason=unknown_peer") {
		t.Fatalf("unexpected log:\n%s", a)
	}
}
