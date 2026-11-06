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

// fixedNet is testNet (3 nodes) with a fixed 10 ms delay, so arrival order
// is send order; it also returns the log.
func fixedNet(cfg simconfig.NetworkConfig) (*sim.Scheduler, *network.Network, *[]arrival, func() string) {
	sched := sim.NewScheduler()
	var buf bytes.Buffer
	log := logging.New(&buf, sched.Now)
	cfg.MinDelayMs, cfg.DeltaMs = 10, 10
	net := network.New(cfg, sched, sim.NewRNG(1, "network"), log)
	got := &[]arrival{}
	for id, peers := range network.FullMesh(3) {
		net.SetPeers(id, peers)
		net.Register(id, func(env types.Envelope) { *got = append(*got, arrival{sched.Now(), env}) })
	}
	return sched, net, got, func() string { _ = log.Flush(); return buf.String() }
}

func limitedNet(rl simconfig.RateLimit) (*sim.Scheduler, *network.Network, *[]arrival, func() string) {
	return fixedNet(simconfig.NetworkConfig{RateLimit: rl})
}

// 10 messages at once on a link with 3 tokens, 1 token/ms, queue of 5:
// 3 leave now, 5 wait and leave one per ms IN ORDER, 2 are dropped. A
// message sent at 2 ms, when a token is back but others still wait, must
// queue behind them instead of jumping ahead.
func TestRateLimitQueuesInOrderThenDrops(t *testing.T) {
	sched, net, got, readLog := limitedNet(simconfig.RateLimit{Capacity: 3, RefillPerMs: 1, QueueLimit: 5})
	for i := 0; i < 10; i++ {
		net.Send(0, 1, voteMsg(0, int32(i)))
	}
	sched.At(2, func() { net.Send(0, 1, voteMsg(0, 10)) })
	sched.Run(1000, nil)
	wantRound := []int32{0, 1, 2, 3, 4, 5, 6, 7, 10}
	wantAt := []uint64{10, 10, 10, 11, 12, 13, 14, 15, 16}
	if len(*got) != len(wantAt) {
		t.Fatalf("delivered %d, want %d", len(*got), len(wantAt))
	}
	for i, a := range *got {
		if r := a.env.Msg.(*types.VoteMsg).Vote.Round; a.at != wantAt[i] || r != wantRound[i] {
			t.Fatalf("delivery %d: round %d at %d ms, want round %d at %d ms", i, r, a.at, wantRound[i], wantAt[i])
		}
	}
	log := readLog()
	if strings.Count(log, "ev=DELAY") != 6 || strings.Count(log, "ev=DROP") != 2 {
		t.Fatalf("want 6 DELAY and 2 DROP reason=rate:\n%s", log)
	}
}

// Each link has its own bucket, and a bucket refills with time.
func TestRateLimitIsPerLinkAndRefills(t *testing.T) {
	sched, net, got, _ := limitedNet(simconfig.RateLimit{Capacity: 2, RefillPerMs: 1, QueueLimit: 10})
	net.Send(0, 1, voteMsg(0, 0))
	net.Send(0, 1, voteMsg(0, 1))                         // link 0→1 is now empty
	net.Send(0, 2, voteMsg(0, 2))                         // another link: still full
	sched.At(5, func() { net.Send(0, 1, voteMsg(0, 3)) }) // refilled by then
	sched.Run(1000, nil)
	for _, a := range *got {
		if a.env.To == 2 && a.at != 10 || a.env.Msg.(*types.VoteMsg).Vote.Round == 3 && a.at != 15 {
			t.Fatalf("round %d to %d delivered at %d ms: it waited", a.env.Msg.(*types.VoteMsg).Vote.Round, a.env.To, a.at)
		}
	}
	if len(*got) != 4 {
		t.Fatalf("delivered %d of 4", len(*got))
	}
}
