package node

import "github.com/ngthdong/minimal-layer1-blockchain/types"

const (
	// maxSyncBlocks bounds one sync answer, and how far ahead of our ledger
	// we keep blocks that arrived out of order.
	maxSyncBlocks = 20
	// syncRetryMs: at most one SyncRequest per period while the previous one
	// may still be answered (every vote from far ahead would trigger one).
	syncRetryMs = 1000
)

type syncedBlock struct {
	from types.NodeID
	resp types.SyncResponse
}

// RequestSync (consensus.Env) asks peer for our missing finalized blocks.
func (n *Node) RequestSync(peer types.NodeID) {
	now := n.clock.Now()
	if n.syncAsked && now < n.syncAt+syncRetryMs {
		return
	}
	n.syncAsked, n.syncAt = true, now
	req := &types.SyncRequest{FromHeight: n.Ledger.Height() + 1}
	if n.isPeer(peer) {
		n.net.Send(n.ID, peer, req)
		return
	}
	// Not a neighbour (a validator whose precommits were relayed to us):
	// the neighbours that relayed them have most likely finalized too.
	for _, p := range n.net.Peers(n.ID) {
		n.net.Send(n.ID, p, req)
	}
}

func (n *Node) isPeer(id types.NodeID) bool {
	for _, p := range n.net.Peers(n.ID) {
		if p == id {
			return true
		}
	}
	return false
}

// onSyncRequest answers with up to maxSyncBlocks finalized blocks, each with
// the commit certificate that proves it final, one message per block.
func (n *Node) onSyncRequest(from types.NodeID, req *types.SyncRequest) {
	for h := req.FromHeight; h <= n.Ledger.Height() && h < req.FromHeight+maxSyncBlocks; h++ {
		e, _ := n.Ledger.Get(h)
		n.net.Send(n.ID, from, &types.SyncResponse{Block: e.Block, Cert: e.Cert})
	}
}

// onSyncResponse keeps the block until it is the next one we need (the
// network reorders), then hands the consecutive blocks to the engine, which
// verifies each one against its certificate before committing it.
func (n *Node) onSyncResponse(from types.NodeID, m *types.SyncResponse) {
	h := m.Block.Header.Height
	if h <= n.Ledger.Height() || h > n.Ledger.Height()+maxSyncBlocks {
		return
	}
	if _, have := n.synced[h]; !have {
		n.synced[h] = syncedBlock{from: from, resp: *m}
	}
	for {
		next := n.Ledger.Height() + 1
		s, ok := n.synced[next]
		if !ok || next != n.Engine.Height {
			break
		}
		delete(n.synced, next)
		if n.Engine.CommitSynced(s.from, &s.resp.Block, &s.resp.Cert) != nil {
			break // a bad block: keep waiting for a good copy
		}
		n.syncAsked = false // progress: the next request may go out at once
	}
	for k := range n.synced {
		if k <= n.Ledger.Height() {
			delete(n.synced, k)
		}
	}
}
