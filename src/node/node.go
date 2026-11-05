package node

import (
	"errors"
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/ledger"
	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/mempool"
	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

type Node struct {
	ID      types.NodeID
	chainID string
	reg     state.Registry
	net     *network.Network
	log     *logging.Logger

	State   *state.State
	Ledger  *ledger.Ledger
	Mempool *mempool.Mempool

	seen map[types.Hash]struct{} // message IDs already handled (gossip dedup)
}

func New(id types.NodeID, chainID string, reg state.Registry, net *network.Network, log *logging.Logger) *Node {
	return &Node{
		ID: id, chainID: chainID, reg: reg, net: net, log: log,
		State: state.New(), Ledger: ledger.New(), Mempool: mempool.New(),
		seen: make(map[types.Hash]struct{}),
	}
}

func (n *Node) HandleMessage(env types.Envelope) {
	id := env.Msg.ID()
	if _, dup := n.seen[id]; dup {
		return
	}
	n.seen[id] = struct{}{}

	switch m := env.Msg.(type) {
	case *types.TxMsg:
		n.onTx(env.From, m)
	default:
		// Proposals, votes, bodies and sync messages: consensus.
	}
}

// SubmitTx is how a client hands a transaction to this node (workload).
// It is logged as RECV from=client, so every tx's entry point is in the log.
func (n *Node) SubmitTx(tx types.Tx) {
	m := &types.TxMsg{Tx: tx.Clone()} // never share memory with the client
	id := m.ID()
	n.log.Log(int(n.ID), logging.EvRecv, 0, 0, "type", "TX", "id", id.Short(), "from", "client")
	if _, dup := n.seen[id]; dup {
		return
	}
	n.seen[id] = struct{}{}
	n.onTx(n.ID, m)
}

// onTx: check (without nonce) → mempool → relay to every peer except the
// one it came from. Invalid or replayed txs are logged and NOT relayed.
func (n *Node) onTx(from types.NodeID, m *types.TxMsg) {
	tx := &m.Tx
	if err := state.CheckTx(n.chainID, n.reg, tx); err != nil {
		n.reject(m, from, err)
		return
	}
	switch err := n.Mempool.Add(*tx, n.State.Nonce(tx.Sender)); {
	case errors.Is(err, mempool.ErrDuplicate):
		return // same tx with another message ID: nothing new to tell peers
	case err != nil:
		n.reject(m, from, err) // stale_nonce: a replay (T4)
		return
	}
	n.relay(from, m)
}

// relay sends m to every neighbour except the one it came from, in NodeID order.
func (n *Node) relay(from types.NodeID, m types.Message) {
	for _, p := range n.net.Peers(n.ID) {
		if p != from {
			n.net.Send(n.ID, p, m)
		}
	}
}

// reject logs: REJECT h r type=<kind> id=<short> from=<peer> reason=<token>
func (n *Node) reject(m types.Message, from types.NodeID, err error) {
	n.log.Log(int(n.ID), logging.EvReject, m.MsgHeight(), 0,
		"type", m.Kind().String(), "id", m.ID().Short(), "from", peer(from), "reason", err.Error())
}

func peer(id types.NodeID) string { return fmt.Sprintf("%02d", id) }
