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
	from     types.NodeID
	resp     types.SyncResponse
	accepted bool // header checked, body requested
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

// onSyncRequest answers with up to maxSyncBlocks finalized HEADERS, each
// with the commit certificate that proves it final, one message per block.
// Bodies follow only on request (onBodyRequest serves finalized blocks).
func (n *Node) onSyncRequest(from types.NodeID, req *types.SyncRequest) {
	for h := req.FromHeight; h <= n.Ledger.Height() && h < req.FromHeight+maxSyncBlocks; h++ {
		e, _ := n.Ledger.Get(h)
		n.net.Send(n.ID, from, &types.SyncResponse{Header: e.Block.Header, Cert: e.Cert})
	}
}

// onSyncResponse keeps the header until it is the next one we need (the
// network reorders), then asks for its body.
func (n *Node) onSyncResponse(from types.NodeID, m *types.SyncResponse) {
	h := m.Header.Height
	if h <= n.Ledger.Height() || h > n.Ledger.Height()+maxSyncBlocks {
		return
	}
	s, have := n.synced[h]
	switch {
	case !have:
		n.synced[h] = syncedBlock{from: from, resp: *m}
	case s.accepted && s.resp.Header.Hash() == m.Header.Hash():
		// We asked for this body already, but it (or our request) may have
		// been lost: ask again, this peer, which holds the block too.
		s.from, s.accepted = from, false
		n.synced[h] = s
	}
	n.advanceSync()
}

// advanceSync: if the header of the next height we need is waiting, the
// engine checks it against its certificate; only an accepted header gets
// its body requested, from the peer that sent the header.
func (n *Node) advanceSync() {
	for k := range n.synced {
		if k <= n.Ledger.Height() {
			delete(n.synced, k)
		}
	}
	next := n.Ledger.Height() + 1
	s, ok := n.synced[next]
	if !ok || next != n.Engine.Height {
		return
	}
	if s.accepted {
		return // body already requested
	}
	if n.Engine.AcceptSyncedHeader(s.from, &s.resp.Header, &s.resp.Cert) != nil {
		delete(n.synced, next) // a bad header: keep waiting for a good copy
		return
	}
	s.accepted = true
	n.synced[next] = s
	n.net.Send(n.ID, s.from, &types.BodyRequest{Height: next, BlockHash: s.resp.Header.Hash()})
}

// onSyncBody handles a body for the accepted sync header of our next height.
// It returns false if no such header waits for it (the engine's body).
func (n *Node) onSyncBody(from types.NodeID, b *types.BodyResponse) bool {
	s, ok := n.synced[b.Height]
	if !ok || !s.accepted || b.Height != n.Engine.Height || s.resp.Header.Hash() != b.BlockHash {
		return false
	}
	block := types.Block{Header: s.resp.Header, Txs: b.Txs}
	if n.Engine.CommitSynced(from, &block, &s.resp.Cert) != nil {
		return true // bad body (logged): the header stays, a good body may follow
	}
	n.syncAsked = false // progress: the next request may go out at once
	n.advanceSync()
	return true
}
