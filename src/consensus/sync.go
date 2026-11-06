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
//
// In (a) we ask the validator that SIGNED the latest buffered vote of h+1
// (votes are verified before buffering): it works on h+1, so it finalized h.
// The peer that relayed the vote may be stuck at h just like us.
func (e *Engine) lagging() (types.NodeID, bool) {
	if next := e.future[e.Height+1]; len(next) > 0 {
		for i := len(next) - 1; i >= 0; i-- {
			if v := next[i].vote; v != nil {
				return v.Validator, true
			}
		}
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

// AcceptSyncedHeader checks a finalized header received through block sync
// from peer `from`, BEFORE its body is requested. Nothing about the sender
// is trusted:
//
//  1. header and certificate are for our current height → wrong_height
//  2. the certificate is for exactly this header         → sync_cert_mismatch
//  3. the header extends our chain                       → wrong_chain / bad_parent
//  4. 2f+1 valid precommits from distinct validators     → cert_* (VerifyCommitCert)
func (e *Engine) AcceptSyncedHeader(from types.NodeID, hdr *types.BlockHeader, cert *types.CommitCert) error {
	err := e.checkSynced(hdr, cert)
	if err != nil {
		e.reject("SYNC", hdr.Height, cert.Round, from, err)
	}
	return err
}

// CommitSynced finalizes block b of our current height once its body came:
// checks 1-4 again, then
//
//  5. the body executes to the header's StateHash → bad_tx_root, bad_state_hash, ...
//
// Then it is committed exactly as if we had collected the precommits.
func (e *Engine) CommitSynced(from types.NodeID, b *types.Block, cert *types.CommitCert) error {
	err := e.checkSynced(&b.Header, cert)
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

func (e *Engine) checkSynced(hdr *types.BlockHeader, cert *types.CommitCert) error {
	switch {
	case hdr.Height != e.Height || cert.Height != e.Height:
		return ErrWrongHeight
	case cert.BlockHash != hdr.Hash():
		return ErrSyncCertMismatch
	case hdr.ChainID != e.chainID:
		return ErrWrongChain
	case hdr.ParentHash != e.env.LastHash():
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
