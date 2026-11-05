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

func (n *Network) Send(from, to types.NodeID, msg types.Message) {
	env := types.Envelope{From: from, To: to, Msg: msg.Clone()}
	delay := n.rng.Range(n.cfg.MinDelayMs, n.cfg.DeltaMs)
	n.logEvent(int(from), logging.EvSend, env, "to", node(to), "delay", strconv.FormatUint(delay, 10))
	n.clock.At(n.clock.Now()+delay, func() { n.deliver(env) })
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
