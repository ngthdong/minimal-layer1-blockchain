package network

import (
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Outbound rate limiting: every link (from, to) has a token bucket of
// rate_limit.capacity tokens, refilled with refill_per_ms tokens per virtual
// millisecond. A message takes one token. Without a token it waits in the
// link's FIFO queue (logged DELAY reason=rate) and leaves, in order, as
// soon as tokens come back; when queue_limit messages already wait, it is
// dropped (DROP reason=rate). Capacity 0 turns the limiter off.
//
// It runs in virtual time like everything else: the bucket is refilled
// lazily from the time it was last looked at, and a waiting queue is
// drained by a scheduler event 1 ms later (at least one token is back).

type linkKey struct{ from, to types.NodeID }

type link struct {
	tokens   int
	last     uint64          // virtual time of the last refill
	queue    []types.Message // own copies, oldest first
	draining bool            // a drain event is scheduled
}

// send applies the rate limit of link (from, to), then transmits.
func (n *Network) send(from, to types.NodeID, msg types.Message) {
	rl := n.cfg.RateLimit
	if rl.Capacity == 0 {
		n.transmit(from, to, msg)
		return
	}
	l := n.link(from, to)
	if len(l.queue) == 0 && l.tokens > 0 { // nothing waits ahead of it
		l.tokens--
		n.transmit(from, to, msg)
		return
	}
	env := types.Envelope{From: from, To: to, Msg: msg}
	if len(l.queue) >= rl.QueueLimit {
		n.logEvent(int(from), logging.EvDrop, env, "to", node(to), "reason", "rate")
		return
	}
	l.queue = append(l.queue, msg.Clone()) // the sender may change msg later
	n.logEvent(int(from), logging.EvDelay, env, "to", node(to), "reason", "rate",
		"queued", strconv.Itoa(len(l.queue)))
	if !l.draining {
		l.draining = true
		n.clock.At(n.clock.Now()+1, func() { n.drain(from, to) })
	}
}

// drain sends as many queued messages as there are tokens, oldest first,
// and comes back 1 ms later while some still wait.
func (n *Network) drain(from, to types.NodeID) {
	l := n.link(from, to)
	l.draining = false
	for len(l.queue) > 0 && l.tokens > 0 {
		msg := l.queue[0]
		l.queue = l.queue[1:]
		l.tokens--
		n.transmit(from, to, msg)
	}
	if len(l.queue) > 0 {
		l.draining = true
		n.clock.At(n.clock.Now()+1, func() { n.drain(from, to) })
	}
}

// link returns the limiter of (from, to), refilled up to now. A new link
// starts with a full bucket.
func (n *Network) link(from, to types.NodeID) *link {
	now, rl := n.clock.Now(), n.cfg.RateLimit
	l := n.links[linkKey{from, to}]
	if l == nil {
		l = &link{tokens: rl.Capacity, last: now}
		n.links[linkKey{from, to}] = l
	}
	l.tokens = min(rl.Capacity, l.tokens+int(now-l.last)*rl.RefillPerMs)
	l.last = now
	return l
}
