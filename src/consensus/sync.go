package consensus

import (
	"errors"
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ErrSyncCertMismatch: the certificate is for another block than the one sent.
var ErrSyncCertMismatch = errors.New("sync_cert_mismatch")

// noticeAhead: a proposal or vote from height h shows that peer `from` is
// ahead of us. One height ahead is normal (the buffer keeps those messages);
// two or more means we missed whole decisions: ask that peer for the blocks.
func (e *Engine) noticeAhead(from types.NodeID, h uint64) {
	if h > e.Height+1 {
		e.env.RequestSync(from)
	}
}

// lagging is asked when our round is stuck. It names a peer that already
// has this height's block if (a) peers work on the next height (we missed
// the decision), or (b) we hold 2f+1 precommits for a block whose body
// never reached us. A body or decision merely late is not lagging: only a
// stuck round (rebroadcast timer) asks.
func (e *Engine) lagging() (types.NodeID, bool) {
	if next := e.future[e.Height+1]; len(next) > 0 {
		return next[len(next)-1].from, true
	}
	for _, r := range e.Votes.Rounds(e.Height, types.Precommit) {
		key := VoteKey{Height: e.Height, Round: r, Type: types.Precommit}
		if bh, ok := e.Votes.Majority(key, e.vals.Quorum()); ok && !bh.IsZero() && e.bodies[bh] == nil {
			return e.Votes.VotesFor(key, bh)[0].Validator, true
		}
	}
	return 0, false
}

// CommitSynced finalizes block b of our current height, received through
// block sync from peer `from`. Nothing about the sender is trusted:
//
//  1. block and certificate are for our current height  → wrong_height
//  2. the certificate is for exactly this block          → sync_cert_mismatch
//  3. the block extends our chain                        → wrong_chain / bad_parent
//  4. 2f+1 valid precommits from distinct validators     → cert_* (VerifyCommitCert)
//  5. the body executes to the header's StateHash        → bad_tx_root, bad_state_hash, ...
//
// Then it is committed exactly as if we had collected the precommits.
func (e *Engine) CommitSynced(from types.NodeID, b *types.Block, cert *types.CommitCert) error {
	err := e.checkSynced(b, cert)
	var post *state.State
	if err == nil {
		post, err = e.env.ExecuteBody(&b.Header, b.Txs, e.cfg.MaxBlockTxs)
	}
	if err != nil {
		e.reject("SYNC", b.Header.Height, cert.Round, from, err)
		return err
	}
	e.commit(b.Clone(), cert.Clone(), post, "sync_from", node(from))
	return nil
}

func (e *Engine) checkSynced(b *types.Block, cert *types.CommitCert) error {
	switch {
	case b.Header.Height != e.Height || cert.Height != e.Height:
		return ErrWrongHeight
	case cert.BlockHash != b.Hash():
		return ErrSyncCertMismatch
	case b.Header.ChainID != e.chainID:
		return ErrWrongChain
	case b.Header.ParentHash != e.env.LastHash():
		return ErrBadParent
	}
	return VerifyCommitCert(e.chainID, e.vals, cert)
}

// commit finalizes block b with its certificate, installs post and moves to
// the next height. kv is appended to the COMMIT log line.
func (e *Engine) commit(b types.Block, cert types.CommitCert, post *state.State, kv ...string) {
	bh := b.Hash()
	if err := e.env.Commit(b, cert, post); err != nil {
		e.env.Log(logging.EvReject, e.Height, cert.Round, "type", "COMMIT", "block", bh.Short(), "reason", err.Error())
		return // our own ledger refuses: a safety alarm, never move on silently
	}
	e.env.Log(logging.EvCommit, e.Height, cert.Round, append([]string{"block", bh.Short(),
		"txs", strconv.Itoa(len(b.Txs)), "state", post.Hash().Short()}, kv...)...)
	e.StartHeight(e.Height + 1)
}
