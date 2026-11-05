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

// faultNet is a 2-node network with GST at 1000 ms and the given pre-GST
// faults. It returns the scheduler, network, arrivals and log buffer.
func faultNet(drop, dup float64) (*sim.Scheduler, *network.Network, *[]arrival, func() string) {
	sched := sim.NewScheduler()
	var buf bytes.Buffer
	log := logging.New(&buf, sched.Now)
	cfg := simconfig.NetworkConfig{
		GSTMs: 1000, MinDelayMs: 10, DeltaMs: 50,
		PreGST: simconfig.PreGSTConfig{DropProb: drop, DupProb: dup, MaxDelayMs: 800},
	}
	net := network.New(cfg, sched, sim.NewRNG(3, "network"), log)
	got := &[]arrival{}
	for id, peers := range network.FullMesh(2) {
		net.SetPeers(id, peers)
		net.Register(id, func(env types.Envelope) { *got = append(*got, arrival{sched.Now(), env}) })
	}
	readLog := func() string { _ = log.Flush(); return buf.String() }
	return sched, net, got, readLog
}

// sendBurst sends count votes from 0 to 1 at virtual time `at`.
func sendBurst(sched *sim.Scheduler, net *network.Network, at uint64, count int, firstRound int32) {
	sched.At(at, func() {
		for i := 0; i < count; i++ {
			net.Send(0, 1, voteMsg(0, firstRound+int32(i)))
		}
	})
}

func TestAllDroppedBeforeGSTAllDeliveredAfter(t *testing.T) {
	sched, net, got, readLog := faultNet(1, 0)
	sendBurst(sched, net, 0, 20, 0)     // before GST: all lost
	sendBurst(sched, net, 1000, 20, 20) // at GST: all delivered within Δ
	sched.Run(5000, nil)
	if len(*got) != 20 {
		t.Fatalf("delivered %d, want the 20 sent after GST", len(*got))
	}
	for _, a := range *got {
		if a.env.Msg.(*types.VoteMsg).Vote.Round < 20 || a.at > 1000+50 {
			t.Fatalf("round %d arrived at %d", a.env.Msg.(*types.VoteMsg).Vote.Round, a.at)
		}
	}
	if c := strings.Count(readLog(), "reason=pre_gst"); c != 20 {
		t.Fatalf("logged %d pre-GST drops, want 20", c)
	}
}

// Before GST delays may exceed Δ (up to max_delay_ms): that is what makes
// the network asynchronous until GST.
func TestDelaysBeforeGSTCanExceedDelta(t *testing.T) {
	sched, net, got, _ := faultNet(0, 0)
	sendBurst(sched, net, 0, 50, 0)
	sched.Run(5000, nil)
	late := 0
	for _, a := range *got {
		if a.at > 800 {
			t.Fatalf("delivered at %d, above max_delay_ms", a.at)
		}
		if a.at > 50 {
			late++
		}
	}
	if late == 0 {
		t.Fatal("no message slower than Δ before GST")
	}
}

func TestDuplicatesAreSeparateCopies(t *testing.T) {
	sched, net, got, readLog := faultNet(0, 1)
	sendBurst(sched, net, 0, 10, 0)
	sched.Run(5000, nil)
	if len(*got) != 20 {
		t.Fatalf("delivered %d, want every message twice (20)", len(*got))
	}
	byID := map[types.Hash][]*types.VoteMsg{}
	for _, a := range *got {
		m := a.env.Msg.(*types.VoteMsg)
		byID[m.ID()] = append(byID[m.ID()], m)
	}
	for _, pair := range byID {
		pair[0].Vote.Sig[0] = 99
		if len(pair) != 2 || pair[1].Vote.Sig[0] == 99 {
			t.Fatal("a message and its duplicate share memory")
		}
	}
	if c := strings.Count(readLog(), "ev=DUP"); c != 10 {
		t.Fatalf("logged %d DUP lines, want 10", c)
	}
}

// The drop probability changes WHICH messages arrive, not WHEN the surviving
// ones arrive: every Send draws the same number of random numbers.
func TestDropProbabilityDoesNotShiftDelays(t *testing.T) {
	arrivals := func(drop float64) map[int32]uint64 {
		sched, net, got, _ := faultNet(drop, 0)
		sendBurst(sched, net, 0, 200, 0)
		sched.Run(5000, nil)
		out := map[int32]uint64{}
		for _, a := range *got {
			out[a.env.Msg.(*types.VoteMsg).Vote.Round] = a.at
		}
		return out
	}
	all, some := arrivals(0), arrivals(0.3)
	if len(some) < 100 || len(some) > 180 {
		t.Fatalf("drop 0.3 kept %d of 200", len(some))
	}
	for round, at := range some {
		if all[round] != at {
			t.Fatalf("message %d arrives at %d with drops, %d without", round, at, all[round])
		}
	}
}
