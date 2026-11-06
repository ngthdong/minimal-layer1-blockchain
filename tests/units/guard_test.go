package unit

import (
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

func guardNet() (func(uint64, types.NodeID, int32), func(), *[]arrival, func() string) {
	sched, net, got, readLog := fixedNet(simconfig.NetworkConfig{
		PeerBlock: simconfig.PeerBlock{WindowMs: 100, MaxMsgs: 5, BlockMs: 300}})
	send := func(at uint64, from types.NodeID, round int32) {
		sched.At(at, func() { net.Send(from, 1, voteMsg(from, round)) })
	}
	return send, func() { sched.Run(10_000, nil) }, got, readLog
}

func from(got []arrival, id types.NodeID) int {
	n := 0
	for _, a := range got {
		if a.env.From == id {
			n++
		}
	}
	return n
}

// Node 0 sends 8 messages in one window (limit 5): the 6th makes node 1 block
// it for 300 ms; until UNBLOCK everything from 0 is dropped, then it is
// heard again. Node 2, at its normal pace, is never affected.
func TestGuardBlocksAFloodingPeer(t *testing.T) {
	send, run, got, readLog := guardNet()
	for i := int32(0); i < 8; i++ {
		send(0, 0, i) // arrive at 10 ms
	}
	send(0, 2, 0)
	send(200, 0, 8) // arrives at 210: still blocked (until 310)
	send(400, 0, 9) // arrives at 410: unblocked
	run()
	if from(*got, 0) != 6 || from(*got, 2) != 1 {
		t.Fatalf("delivered from 0: %d (want 5 + 1 after UNBLOCK), from 2: %d", from(*got, 0), from(*got, 2))
	}
	log := readLog()
	for _, want := range []string{
		"t=0000000010 node=01 ev=BLOCK         h=0 r=0 peer=00 msgs=6 window_ms=100 until=310",
		"t=0000000310 node=01 ev=UNBLOCK       h=0 r=0 peer=00 dropped=4",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("missing %q in:\n%s", want, log)
		}
	}
	if strings.Count(log, "reason=blocked") != 4 || strings.Count(log, "peer=02") != 0 {
		t.Fatalf("want 4 blocked drops and node 2 never blocked:\n%s", log)
	}
}

// The count starts again in every window: 5 per window is never too many.
func TestGuardCountsPerWindow(t *testing.T) {
	send, run, got, readLog := guardNet()
	for i := int32(0); i < 5; i++ {
		send(0, 0, i)
		send(150, 0, 5+i)
	}
	run()
	if from(*got, 0) != 10 || strings.Contains(readLog(), "ev=BLOCK") {
		t.Fatal("a peer under the limit in each window was blocked")
	}
}
