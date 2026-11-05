package consensus

import (
	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Timeout is a timer the engine asks its environment to fire later.
type Timeout struct {
	Height uint64
	Round  int32
	Step   Step
}

func (s Step) String() string {
	switch s {
	case StepPropose:
		return "propose"
	case StepPrevote:
		return "prevote"
	case StepPrecommit:
		return "precommit"
	}
	return "unknown"
}

// timeout returns base + round*delta. Waiting a little longer in every round
// guarantees that after GST some round lasts long enough for an honest
// proposer's block and the votes on it to arrive: that is liveness.
func (e *Engine) timeout(base uint64) uint64 {
	return base + uint64(e.Round)*e.cfg.TimeoutDeltaMs
}

// schedule asks for timer (current height, round, step) once per height.
func (e *Engine) schedule(step Step, base uint64) {
	t := Timeout{Height: e.Height, Round: e.Round, Step: step}
	if e.timers[t] {
		return
	}
	e.timers[t] = true
	e.env.ScheduleTimeout(e.timeout(base), t)
}

// OnTimeout is called when a timer fires. A timer for a height, round or
// step we have already left is stale and ignored. Otherwise:
//
//	propose   → no usable proposal in time: prevote NIL
//	prevote   → 2f+1 prevotes but no quorum for one value: precommit NIL
//	precommit → 2f+1 precommits but no decision: go to the next round
func (e *Engine) OnTimeout(t Timeout) {
	if t.Height != e.Height || t.Round != e.Round {
		return
	}
	switch {
	case t.Step == StepPropose && e.Step == StepPropose:
		e.logTimeout(t)
		if e.castVote(types.Prevote, types.ZeroHash) {
			e.Step = StepPrevote
		}
	case t.Step == StepPrevote && e.Step == StepPrevote:
		e.logTimeout(t)
		if e.castVote(types.Precommit, types.ZeroHash) {
			e.Step = StepPrecommit
		}
	case t.Step == StepPrecommit:
		e.logTimeout(t)
		e.enterNewRound(e.Round + 1)
	default:
		return
	}
	e.progress()
}

func (e *Engine) logTimeout(t Timeout) {
	e.env.Log(logging.EvTimeout, t.Height, t.Round, "step", t.Step.String())
}

// checkTimers starts the prevote and precommit timers of the current round
// the first time 2f+1 votes of that kind have arrived, whatever they are for.
func (e *Engine) checkTimers() {
	q := e.vals.Quorum()
	prevotes := VoteKey{Height: e.Height, Round: e.Round, Type: types.Prevote}
	if e.Step == StepPrevote && e.Votes.CountAny(prevotes) >= q {
		e.schedule(StepPrevote, e.cfg.TimeoutPrevoteMs)
	}
	precommits := VoteKey{Height: e.Height, Round: e.Round, Type: types.Precommit}
	if e.Votes.CountAny(precommits) >= q {
		e.schedule(StepPrecommit, e.cfg.TimeoutPrecommitMs)
	}
}
