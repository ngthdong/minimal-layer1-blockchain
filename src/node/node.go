package node

import (
	"errors"
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
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
	clock   network.Clock

	State   *state.State
	Ledger  *ledger.Ledger
	Mempool *mempool.Mempool
	Engine  *consensus.Engine // set by the runner; nil in mempool-only tests

	seen      map[types.Hash]struct{}       // message IDs already handled (gossip dedup)
	crashed   bool                          // stopped for good (Crash)
	blockHook func(types.Block) types.Block // Byzantine proposer, nil when honest

	synced    map[uint64]syncedBlock // sync answers waiting for their turn
	syncAsked bool                   // a SyncRequest went out at syncAt
	syncAt    uint64
}

func New(id types.NodeID, chainID string, reg state.Registry, net *network.Network,
	log *logging.Logger, clock network.Clock) *Node {
	return &Node{
		ID: id, chainID: chainID, reg: reg, net: net, log: log, clock: clock,
		State: state.New(), Ledger: ledger.New(), Mempool: mempool.New(),
		seen:   make(map[types.Hash]struct{}),
		synced: make(map[uint64]syncedBlock),
	}
}

// HandleMessage is the node's network handler (network.Register).
//
// Gossiped messages (tx, proposal, vote) are handled at most once: a copy
// that arrives again — from another peer, or duplicated by the network (T2)
// — is ignored. Their IDs include the signature, so a forged copy has its
// own ID and can never hide the real message.
//
// Point-to-point messages (body request/response) are NOT de-duplicated by
// ID: two peers asking us for the same body send identical requests, and
// both must be answered.
func (n *Node) HandleMessage(env types.Envelope) {
	if n.crashed {
		return
	}
	switch env.Msg.Kind() {
	case types.MsgTx, types.MsgProposal, types.MsgVote:
		id := env.Msg.ID()
		if _, dup := n.seen[id]; dup {
			return
		}
		n.seen[id] = struct{}{}
	}

	switch m := env.Msg.(type) {
	case *types.TxMsg:
		n.onTx(env.From, m)
	case *types.ProposalMsg:
		if n.Engine != nil {
			n.Engine.OnProposal(env.From, &m.Proposal)
		}
	case *types.BodyRequest:
		n.onBodyRequest(env.From, m)
	case *types.BodyResponse:
		if n.Engine != nil {
			n.Engine.OnBody(env.From, m)
		}
	case *types.VoteMsg:
		if n.Engine != nil && n.Engine.OnVote(env.From, &m.Vote) {
			n.relay(env.From, m) // gossip: peers that missed the original still get it
		}
	case *types.SyncRequest:
		n.onSyncRequest(env.From, m)
	case *types.SyncResponse:
		if n.Engine != nil {
			n.onSyncResponse(env.From, m)
		}
	}
}

// onBodyRequest answers only with a body we have checked ourselves: a block
// of our current height, or one we already finalized (the requester may be
// a height behind us).
func (n *Node) onBodyRequest(from types.NodeID, req *types.BodyRequest) {
	if n.Engine == nil {
		return
	}
	txs, ok := n.Engine.Body(req.Height, req.BlockHash)
	if !ok {
		if e, final := n.Ledger.Get(req.Height); final && e.Block.Hash() == req.BlockHash {
			txs, ok = e.Block.Txs, true
		}
	}
	if ok {
		n.net.Send(n.ID, from, &types.BodyResponse{Height: req.Height, BlockHash: req.BlockHash, Txs: txs})
	}
}

// SubmitTx is how a client hands a transaction to this node (workload).
// It is logged as RECV from=client, so every tx's entry point is in the log.
// Unlike gossip, a client submission is never dropped silently as "seen":
// a client that resubmits a known tx (T4 replay) gets a REJECT with reason.
func (n *Node) SubmitTx(tx types.Tx) {
	if n.crashed {
		return // the client's tx is lost, as with a real dead server
	}
	m := &types.TxMsg{Tx: tx.Clone()} // never share memory with the client
	id := m.ID()
	n.log.Log(int(n.ID), logging.EvRecv, 0, 0, "type", "TX", "id", id.Short(), "from", "client")
	n.seen[id] = struct{}{} // peers relaying it back to us are still de-duplicated
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
		if from == n.ID {
			n.reject(m, from, err) // a client replays a pending or committed tx (T4)
		}
		return // nothing new to tell peers
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
	src := peer(from)
	if from == n.ID {
		src = "client" // SubmitTx: no peer ever sends us our own messages
	}
	n.log.Log(int(n.ID), logging.EvReject, m.MsgHeight(), 0,
		"type", m.Kind().String(), "id", m.ID().Short(), "from", src, "reason", err.Error())
}

func peer(id types.NodeID) string { return fmt.Sprintf("%02d", id) }
