package consensus

import (
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// OnVote handles a prevote or precommit from the network. It returns true if
// the vote is new and valid (also when it is an equivocation: peers must see
// the evidence too), i.e. when the node should relay it.
func (e *Engine) OnVote(from types.NodeID, v *types.Vote) bool {
	if err := VerifyVote(e.chainID, e.vals, v); err != nil {
		e.reject("VOTE", v.Height, v.Round, from, err)
		return false
	}
	if v.Height > e.Height {
		e.noticeAhead(from, v.Height)
		cp := v.Clone()
		e.buffer(v.Height, buffered{from: from, vote: &cp})
		return true // valid signature: worth relaying, peers may need it now
	}
	if v.Height < e.Height {
		return false // a finished height
	}
	key := KeyOf(v)
	known := len(e.Votes.Equivocators(key))
	res, first := e.Votes.Add(*v)
	switch res {
	case Duplicate:
		return false
	case Equivocation:
		if len(e.Votes.Equivocators(key)) > known { // first evidence for this validator and step
			e.env.Log(logging.EvEquivocation, v.Height, v.Round, "validator", node(v.Validator),
				"type", v.Type.String(), "first", blockLabel(first.BlockHash), "second", blockLabel(v.BlockHash))
		}
		// The second vote is signed too: it now counts toward ITS block
		// (once), so the rules must be re-checked like for any new vote.
	}
	e.progress()
	return true
}

// progress applies every rule whose condition may have become true. It is
// called after each event (proposal, body, vote) and is idempotent: each rule
// fires at most once per round thanks to Step and the vote signer.
func (e *Engine) progress() {
	e.checkRoundSkip() // first: the other rules act on the round we end up in
	e.tryPrevote()
	e.checkPrevoteQuorum()
	e.updateValid()
	e.checkTimers()
	e.checkCommit() // last: it may start the next height
}

// checkRoundSkip: if f+1 validators already vote in a higher round r of
// this height, at least one honest node is there: we fell behind (lost
// votes, a timeout that fired later than theirs). Jump straight to r instead
// of timing out round after round. f+1, not 2f+1: f Byzantine validators
// alone can never drag us into a new round.
func (e *Engine) checkRoundSkip() {
	target := e.Round
	for _, typ := range []types.VoteType{types.Prevote, types.Precommit} {
		for _, r := range e.Votes.Rounds(e.Height, typ) {
			if r > target && e.Votes.Voters(e.Height, r) >= e.vals.F()+1 {
				target = r
			}
		}
	}
	if target > e.Round {
		e.enterNewRound(target, "skip_from", strconv.Itoa(int(e.Round)))
	}
}

// tryPrevote: rules 3 and 5 for the proposal of the current round.
//
//   - body provably invalid → prevote NIL;
//   - body not known yet    → wait (the propose timeout prevotes NIL);
//   - POLRound = -1 (fresh block): prevote B unless locked on another block;
//   - POLRound = vr >= 0 (B proposed again): wait until we also see the 2f+1
//     prevotes for B in round vr, then prevote B if we are not locked, locked
//     on B, or locked in a round <= vr (we UNLOCK: the network has moved on
//     to B since our lock); otherwise prevote NIL.
func (e *Engine) tryPrevote() {
	if e.Step != StepPropose {
		return
	}
	p, ok := e.proposals[e.Round]
	if !ok {
		return
	}
	bh := p.BlockHash()
	switch {
	case e.invalid[bh]:
		bh = types.ZeroHash
	case e.bodies[bh] == nil:
		return
	case p.POLRound >= 0 && !e.hasPOL(p.POLRound, bh):
		return // the proof-of-lock has not reached us (yet)
	case e.LockedRound == -1 || e.LockedHash == bh:
		// not locked, or locked on B itself: prevote B
	case p.POLRound >= 0 && e.LockedRound <= p.POLRound:
		e.unlock(p.POLRound)
	default:
		bh = types.ZeroHash // locked on another block (rule 5)
	}
	if e.castVote(types.Prevote, bh) {
		e.Step = StepPrevote
	}
}

// hasPOL reports whether we hold 2f+1 prevotes for bh in round r.
func (e *Engine) hasPOL(r int32, bh types.Hash) bool {
	got, ok := e.Votes.Majority(VoteKey{Height: e.Height, Round: r, Type: types.Prevote}, e.vals.Quorum())
	return ok && got == bh
}

// unlock releases our lock because a later round (pol) proved 2f+1 prevotes
// for another block. Safe: if our locked block had been decided, no other
// block could have gathered 2f+1 prevotes in a round after it.
func (e *Engine) unlock(pol int32) {
	e.env.Log(logging.EvUnlock, e.Height, e.Round, "block", e.LockedHash.Short(),
		"locked_round", strconv.Itoa(int(e.LockedRound)), "pol", strconv.Itoa(int(pol)))
	e.LockedRound, e.LockedHash = -1, types.ZeroHash
}

// updateValid remembers the latest block that got 2f+1 prevotes in a round
// up to the current one, even if we did not lock on it (we had already left
// that round, or precommitted NIL on a timeout). It is what we propose again.
func (e *Engine) updateValid() {
	for _, r := range e.Votes.Rounds(e.Height, types.Prevote) {
		if r <= e.ValidRound || r > e.Round {
			continue
		}
		bh, ok := e.Votes.Majority(VoteKey{Height: e.Height, Round: r, Type: types.Prevote}, e.vals.Quorum())
		if ok && !bh.IsZero() && e.bodies[bh] != nil {
			e.ValidRound, e.ValidHash = r, bh
		}
	}
}

// checkPrevoteQuorum: once we have prevoted in the current round, 2f+1
// prevotes for a block B whose body we hold make us LOCK on B and precommit B
// (rule 4). 2f+1 prevotes for NIL make us precommit NIL. Either happens at
// most once per round (Step moves to Precommit).
func (e *Engine) checkPrevoteQuorum() {
	if e.Step != StepPrevote {
		return
	}
	key := VoteKey{Height: e.Height, Round: e.Round, Type: types.Prevote}
	bh, ok := e.Votes.Majority(key, e.vals.Quorum())
	switch {
	case !ok:
		return // no quorum yet (the prevote timeout precommits NIL)
	case bh.IsZero():
		if e.castVote(types.Precommit, types.ZeroHash) {
			e.Step = StepPrecommit
		}
	case e.bodies[bh] != nil:
		e.LockedRound, e.LockedHash = e.Round, bh
		e.ValidRound, e.ValidHash = e.Round, bh
		e.env.Log(logging.EvLock, e.Height, e.Round, "block", bh.Short())
		if e.castVote(types.Precommit, bh) {
			e.Step = StepPrecommit
		}
	}
	// 2f+1 for a block whose body we lack: wait for the body (OnBody calls progress).
}

// checkCommit: 2f+1 precommits for a block B in ANY round of this height
// decide the height (the round we are in does not matter: a slow node may
// still be in an earlier round). With B's body at hand we build the
// certificate, commit and move to the next height.
func (e *Engine) checkCommit() {
	for _, r := range e.Votes.Rounds(e.Height, types.Precommit) {
		key := VoteKey{Height: e.Height, Round: r, Type: types.Precommit}
		bh, ok := e.Votes.Majority(key, e.vals.Quorum())
		if !ok || bh.IsZero() {
			continue
		}
		p := e.bodies[bh]
		if p == nil {
			return // decided, but the body is still on its way (if it never comes: lagging())
		}
		cert := types.CommitCert{Height: e.Height, Round: r, BlockHash: bh, Precommits: e.Votes.VotesFor(key, bh)}
		e.commit(p.block.Clone(), cert, p.post)
		return
	}
}
