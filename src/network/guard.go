package network

import (
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Peer guard: every node counts what it receives from each peer in fixed
// windows of peer_block.window_ms. A peer that sends more than max_msgs in
// one window is BLOCKED for block_ms: its messages are dropped on arrival
// (DROP reason=blocked) until UNBLOCK. max_msgs 0 turns the guard off.
//
// The threshold must be far above what an honest peer ever sends (about 60
// messages per second per peer in T1–T7, duplicates included): blocking an
// honest node would cut it out of every quorum and could break liveness.

type guard struct {
	window  uint64 // index of the counting window (now / window_ms)
	count   int
	blocked bool
	dropped int // messages dropped during the current block
}

// admit decides whether the delivery of env goes ahead (false: dropped).
func (n *Network) admit(env types.Envelope) bool {
	pb := n.cfg.PeerBlock
	if pb.MaxMsgs == 0 {
		return true
	}
	key := linkKey{env.From, env.To}
	g := n.guards[key]
	if g == nil {
		g = &guard{}
		n.guards[key] = g
	}
	now := n.clock.Now()
	if g.blocked {
		g.dropped++
		n.logEvent(int(env.To), logging.EvDrop, env, "from", node(env.From), "reason", "blocked")
		return false
	}
	if w := now / pb.WindowMs; w != g.window {
		g.window, g.count = w, 0
	}
	g.count++
	if g.count <= pb.MaxMsgs {
		return true
	}
	// BLOCK and UNBLOCK carry the type and height of the message that
	// crossed the threshold, like every other network event.
	g.blocked, g.dropped = true, 1
	n.logEvent(int(env.To), logging.EvBlock, env, "peer", node(env.From),
		"msgs", strconv.Itoa(g.count), "window_ms", strconv.FormatUint(pb.WindowMs, 10),
		"until", strconv.FormatUint(now+pb.BlockMs, 10))
	n.logEvent(int(env.To), logging.EvDrop, env, "from", node(env.From), "reason", "blocked")
	n.clock.At(now+pb.BlockMs, func() {
		n.logEvent(int(env.To), logging.EvUnblock, env, "peer", node(env.From), "dropped", strconv.Itoa(g.dropped))
		g.blocked, g.count, g.window = false, 0, n.clock.Now()/pb.WindowMs
	})
	return false
}
