package network

import (
	"sort"
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Clock is the part of sim.Scheduler the network needs. Using an interface
// keeps the import graph acyclic (sim's runner imports network).
type Clock interface {
	Now() uint64
	At(t uint64, fn func())
}

// Rand is the part of sim.RNG the network needs.
type Rand interface {
	Range(lo, hi uint64) uint64
	Chance(p float64) bool
}

// Handler receives a delivered envelope. The message inside is the
// receiver's own deep copy.
type Handler func(env types.Envelope)

type Network struct {
	cfg      simconfig.NetworkConfig
	clock    Clock
	rng      Rand
	log      *logging.Logger
	handlers map[types.NodeID]Handler
	peers    map[types.NodeID][]types.NodeID // sorted
}

func New(cfg simconfig.NetworkConfig, clock Clock, rng Rand, log *logging.Logger) *Network {
	return &Network{
		cfg: cfg, clock: clock, rng: rng, log: log,
		handlers: make(map[types.NodeID]Handler),
		peers:    make(map[types.NodeID][]types.NodeID),
	}
}

// Register attaches the inbound handler of node id.
func (n *Network) Register(id types.NodeID, h Handler) { n.handlers[id] = h }

// SetPeers sets the neighbours of id.
func (n *Network) SetPeers(id types.NodeID, peers []types.NodeID) {
	p := append([]types.NodeID(nil), peers...)
	sort.Slice(p, func(i, j int) bool { return p[i] < p[j] })
	n.peers[id] = p
}

func (n *Network) Peers(id types.NodeID) []types.NodeID { return n.peers[id] }

// Send transmits msg from one node to another. The message is deep-copied
// NOW, so the sender may reuse or change its object afterwards.
//
// After GST: one delay in [min_delay, Δ], always delivered.
// Before GST: the message may be dropped, duplicated, and delayed up to
// pre_gst.max_delay_ms (which may exceed Δ). Messages sent before GST and
// lost are NOT re-sent by the network: nodes re-send on timeout (stage 29).
//
// The number of random draws per Send is fixed (1 after GST, 4 before),
// whatever the outcome, so changing a probability in the config does not
// shift the delays of every later message.
func (n *Network) Send(from, to types.NodeID, msg types.Message) {
	env := types.Envelope{From: from, To: to, Msg: msg.Clone()}
	now := n.clock.Now()
	if now >= n.cfg.GSTMs {
		delay := n.rng.Range(n.cfg.MinDelayMs, n.cfg.DeltaMs)
		n.logSend(env, delay)
		n.clock.At(now+delay, func() { n.deliver(env) })
		return
	}

	pre := n.cfg.PreGST
	drop := n.rng.Chance(pre.DropProb)
	dup := n.rng.Chance(pre.DupProb)
	delay := n.rng.Range(n.cfg.MinDelayMs, pre.MaxDelayMs)
	dupDelay := n.rng.Range(n.cfg.MinDelayMs, pre.MaxDelayMs)

	n.logSend(env, delay)
	if drop {
		n.logEvent(int(from), logging.EvDrop, env, "to", node(to), "reason", "pre_gst")
		return
	}
	n.clock.At(now+delay, func() { n.deliver(env) })
	if dup {
		copyEnv := types.Envelope{From: from, To: to, Msg: msg.Clone()} // its own copy
		n.logEvent(int(from), logging.EvDup, copyEnv, "to", node(to), "delay", strconv.FormatUint(dupDelay, 10))
		n.clock.At(now+dupDelay, func() { n.deliver(copyEnv) })
	}
}

func (n *Network) logSend(env types.Envelope, delay uint64) {
	n.logEvent(int(env.From), logging.EvSend, env, "to", node(env.To), "delay", strconv.FormatUint(delay, 10))
}

func (n *Network) Broadcast(from types.NodeID, msg types.Message) {
	for _, to := range n.peers[from] {
		n.Send(from, to, msg)
	}
}

func (n *Network) deliver(env types.Envelope) {
	h, ok := n.handlers[env.To]
	if !ok {
		n.logEvent(int(env.To), logging.EvDrop, env, "from", node(env.From), "reason", "unknown_peer")
		return
	}
	n.logEvent(int(env.To), logging.EvRecv, env, "from", node(env.From))
	h(env)
}

// logEvent writes: ev h=<msg height> r=<msg round> type=<kind> id=<short> extra...
func (n *Network) logEvent(at int, ev string, env types.Envelope, extra ...string) {
	kv := append([]string{"type", env.Msg.Kind().String(), "id", env.Msg.ID().Short()}, extra...)
	n.log.Log(at, ev, env.Msg.MsgHeight(), msgRound(env.Msg), kv...)
}

func msgRound(m types.Message) int32 {
	switch x := m.(type) {
	case *types.ProposalMsg:
		return x.Proposal.Round
	case *types.VoteMsg:
		return x.Vote.Round
	}
	return 0
}

func node(id types.NodeID) string {
	s := strconv.FormatUint(uint64(id), 10)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

// FullMesh returns, for each of n nodes, the sorted list of all other nodes.
func FullMesh(n int) map[types.NodeID][]types.NodeID {
	out := make(map[types.NodeID][]types.NodeID, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j {
				out[types.NodeID(i)] = append(out[types.NodeID(i)], types.NodeID(j))
			}
		}
	}
	return out
}
