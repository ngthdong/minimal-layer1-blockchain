package node

import (
	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/ledger"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// The node is the consensus engine's environment (consensus.Env).
var _ consensus.Env = (*Node)(nil)

func (n *Node) Broadcast(m types.Message)                      { n.net.Broadcast(n.ID, m) }
func (n *Node) Send(to types.NodeID, m types.Message)          { n.net.Send(n.ID, to, m) }
func (n *Node) LastHash() types.Hash                           { return n.Ledger.LastHash() }
func (n *Node) Log(ev string, h uint64, r int32, kv ...string) { n.log.Log(int(n.ID), ev, h, r, kv...) }

// BuildBlock takes up to maxTxs ready transactions from the mempool and
// executes them on a copy of the state. Reap only returns txs that apply in
// order, so execution cannot fail; if it ever did, the block is left empty
// rather than proposing something invalid.
func (n *Node) BuildBlock(height uint64, parent types.Hash, maxTxs int) (types.Block, *state.State) {
	txs := n.Mempool.Reap(maxTxs, n.State.Nonce)
	post := n.State.Clone()
	if err := post.ApplyBlock(n.chainID, n.reg, txs); err != nil {
		txs, post = nil, n.State.Clone()
	}
	hdr := types.BlockHeader{
		ChainID:    n.chainID,
		Height:     height,
		ParentHash: parent,
		TxRoot:     types.TxRoot(txs),
		StateHash:  post.Hash(),
	}
	return types.Block{Header: hdr, Txs: txs}, post
}

// ExecuteBody checks a received body against the node's current state.
func (n *Node) ExecuteBody(hdr *types.BlockHeader, txs []types.Tx, maxTxs int) (*state.State, error) {
	return consensus.ExecuteBody(n.chainID, n.reg, n.State, hdr, txs, maxTxs)
}

// Commit finalizes a block. Order matters: the ledger first (it may refuse a
// conflicting block, and then nothing else changes), then the state computed
// when the body was checked, then the mempool, using the NEW nonces.
func (n *Node) Commit(block types.Block, cert types.CommitCert, post *state.State) error {
	if err := n.Ledger.Append(ledger.Entry{Block: block, Cert: cert}); err != nil {
		return err
	}
	n.State = post
	n.Mempool.RemoveCommitted(block.Txs, n.State.Nonce)
	return nil
}

// ScheduleTimeout fires the engine timer t after afterMs of virtual time.
func (n *Node) ScheduleTimeout(afterMs uint64, t consensus.Timeout) {
	n.clock.At(n.clock.Now()+afterMs, func() {
		if !n.crashed { // a dead node's timers fire into the void
			n.Engine.OnTimeout(t)
		}
	})
}
