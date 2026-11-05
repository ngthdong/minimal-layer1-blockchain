package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/node"
	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// nodeNet wires n nodes on a full-mesh network. dup > 0 turns on duplicates
// (GST is then far in the future so the pre-GST faults apply).
func nodeNet(n int, dup float64) (*sim.Scheduler, []*node.Node, func() string) {
	sched := sim.NewScheduler()
	var buf bytes.Buffer
	log := logging.New(&buf, sched.Now)
	cfg := simconfig.NetworkConfig{MinDelayMs: 10, DeltaMs: 50}
	if dup > 0 {
		cfg.GSTMs = 1 << 40
		cfg.PreGST = simconfig.PreGSTConfig{DupProb: dup, MaxDelayMs: 50}
	}
	net := network.New(cfg, sched, sim.NewRNG(9, "network"), log)
	mesh := network.FullMesh(n)
	nodes := make([]*node.Node, n)
	for i := 0; i < n; i++ {
		id := types.NodeID(i)
		nodes[i] = node.New(id, chain, registry(), net, log, sched)
		net.Register(id, nodes[i].HandleMessage)
		net.SetPeers(id, mesh[id])
	}
	readLog := func() string { _ = log.Flush(); return buf.String() }
	return sched, nodes, readLog
}

func mempoolSizes(nodes []*node.Node) []int {
	out := make([]int, len(nodes))
	for i, nd := range nodes {
		out[i] = nd.Mempool.Size()
	}
	return out
}

func TestTxGossipReachesEveryNodeOnce(t *testing.T) {
	sched, nodes, readLog := nodeNet(10, 0)
	nodes[0].SubmitTx(goodTx("Alice", 1, "hello"))
	sched.Run(10_000, nil)
	for i, size := range mempoolSizes(nodes) {
		if size != 1 {
			t.Fatalf("node %d mempool size %d, want 1", i, size)
		}
	}
	// Node 0 sends to 9 peers; each of them relays once to its 8 other peers.
	if sends := strings.Count(readLog(), "ev=SEND          h=0 r=0 type=TX"); sends != 9+9*8 {
		t.Fatalf("%d tx sends, want 81 (every node relays exactly once)", sends)
	}
}

// at node level: duplicated deliveries are absorbed silently.
func TestDuplicatedDeliveriesAreIgnored(t *testing.T) {
	sched, nodes, readLog := nodeNet(4, 1)
	nodes[0].SubmitTx(goodTx("Alice", 1, "hello"))
	sched.Run(10_000, nil)
	log := readLog()
	if !strings.Contains(log, "ev=DUP") || strings.Contains(log, "ev=REJECT") {
		t.Fatal("expected duplicates in the network and no rejection")
	}
	for i, size := range mempoolSizes(nodes) {
		if size != 1 {
			t.Fatalf("node %d mempool size %d, want 1", i, size)
		}
	}
}

// at node level: bad txs are logged with their reason and go no further.
func TestInvalidTxIsRejectedAndNotRelayed(t *testing.T) {
	forged := goodTx("Alice", 1, "hello")
	forged.Value = []byte("changed")
	cases := map[string]types.Tx{
		"bad_signature":           forged,
		"key_not_owned_by_sender": signedTx("Alice", 1, "Bob/message", "x", "TX"),
		"unknown_sender":          signedTx("Mallory", 1, "Mallory/x", "x", "TX"),
	}
	for reason, tx := range cases {
		sched, nodes, readLog := nodeNet(4, 0)
		nodes[0].SubmitTx(tx)
		sched.Run(10_000, nil)
		log := readLog()
		if !strings.Contains(log, "node=00 ev=REJECT") || !strings.Contains(log, "reason="+reason) {
			t.Fatalf("%s: no REJECT line:\n%s", reason, log)
		}
		if strings.Contains(log, "ev=SEND") {
			t.Fatalf("%s: invalid tx was relayed", reason)
		}
	}
}

// The network may deliver nonce 2 before nonce 1: both must be kept.
func TestFutureNonceIsKeptNotRejected(t *testing.T) {
	sched, nodes, readLog := nodeNet(4, 0)
	nodes[0].SubmitTx(goodTx("Alice", 2, "second"))
	nodes[0].SubmitTx(goodTx("Alice", 1, "first"))
	sched.Run(10_000, nil)
	if strings.Contains(readLog(), "ev=REJECT") {
		t.Fatal("a future nonce was rejected")
	}
	for i, size := range mempoolSizes(nodes) {
		if size != 2 {
			t.Fatalf("node %d mempool size %d, want 2", i, size)
		}
	}
}

// at node level: a tx whose nonce is already used is a replay.
func TestReplayedTxIsRejected(t *testing.T) {
	sched, nodes, readLog := nodeNet(4, 0)
	tx := goodTx("Alice", 1, "hello")
	if err := nodes[0].State.ApplyTx(chain, registry(), &tx); err != nil {
		t.Fatal(err)
	}
	nodes[0].SubmitTx(tx)
	sched.Run(10_000, nil)
	log := readLog()
	if !strings.Contains(log, "reason=stale_nonce") || strings.Contains(log, "ev=SEND") {
		t.Fatalf("replay not rejected or was relayed:\n%s", log)
	}
}
