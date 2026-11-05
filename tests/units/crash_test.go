package unit

import (
	"io"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
)

// A crashed node receives nothing (no RECV line), accepts no client tx and
// relays nothing; the others carry on without it.
func TestCrashedNodeIgnoresEverything(t *testing.T) {
	sched, nodes, readLog := nodeNet(4, 0)
	nodes[2].Crash()
	nodes[2].Crash() // idempotent: logged once
	nodes[0].SubmitTx(goodTx("Alice", 1, "a"))
	nodes[2].SubmitTx(goodTx("Bob", 1, "b"))
	sched.Run(10_000, nil)
	if got := mempoolSizes(nodes); got[0] != 1 || got[1] != 1 || got[2] != 0 || got[3] != 1 {
		t.Fatalf("mempool sizes %v, want [1 1 0 1]", got)
	}
	log := readLog()
	if strings.Count(log, "node=02 ev=CRASH") != 1 || strings.Contains(log, "node=02 ev=RECV") {
		t.Fatalf("log:\n%s", log)
	}
	if strings.Contains(log, "from=02") {
		t.Fatal("a crashed node relayed a message")
	}
}

// A behavior the simulator does not implement is an error, never a node that
// silently stays honest.
func TestUnknownBehaviorIsAnError(t *testing.T) {
	c, err := simconfig.Load(t1Path)
	if err != nil {
		t.Fatal(err)
	}
	c.Byzantine = []simconfig.ByzantineSpec{{Node: 1, Behavior: "teleport"}}
	if _, err := sim.Run(c, io.Discard); err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Fatalf("err = %v", err)
	}
}
