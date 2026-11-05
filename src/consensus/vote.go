package consensus

import (
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
	if v.Height != e.Height {
		return false // other heights
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
		return true
	}
	e.progress()
	return true
}

// progress applies every rule whose condition may have become true. It is
// called after each event (proposal, body, vote) and is idempotent: each rule
// fires at most once per round thanks to Step and the vote signer.
func (e *Engine) progress() {
	e.tryPrevote()
	e.checkPrevoteQuorum()
}

// tryPrevote: rule 3 (prevote only valid blocks) and rule 5 (respect the lock).
//
// With a proposal for the current round:
//   - body executed correctly → prevote B, unless locked on another block;
//   - body provably invalid   → prevote NIL;
//   - body not known yet      → wait.
//
// Proposals that carry a POL round.
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
	case e.LockedRound != -1 && e.LockedHash != bh:
		bh = types.ZeroHash // locked on something else (rule 5)
	}
	if e.castVote(types.Prevote, bh) {
		e.Step = StepPrevote
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
		return // no quorum yet
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
