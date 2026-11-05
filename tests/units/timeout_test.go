package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// timerFor returns the last timer scheduled for step, and how many there were.
func timerFor(env *fakeEnv, step consensus.Step) (scheduled, int) {
	var last scheduled
	n := 0
	for _, s := range env.timers {
		if s.t.Step == step {
			last, n = s, n+1
		}
	}
	return last, n
}

// No proposal in time: prevote NIL when the propose timer fires.
func TestProposeTimeoutPrevotesNil(t *testing.T) {
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	tm, n := timerFor(env, consensus.StepPropose)
	if n != 1 || tm.after != 1000 || tm.t != (consensus.Timeout{Height: 1, Round: 0, Step: consensus.StepPropose}) {
		t.Fatalf("propose timer = %+v (%d)", tm, n)
	}
	e.OnTimeout(tm.t)
	if !env.logged("TIMEOUT h=1 r=0 step propose") || !env.logged("PREVOTE h=1 r=0 block nil") ||
		e.Step != consensus.StepPrevote {
		t.Fatalf("step=%d logs=%q", e.Step, env.logs)
	}
}

// A timer for a step, round or height we already left does nothing.
func TestStaleTimeoutsAreIgnored(t *testing.T) {
	e, env, _ := readyEngine(t) // already prevoted the block
	before := len(env.logs)
	for _, tm := range []consensus.Timeout{
		{Height: 1, Round: 0, Step: consensus.StepPropose},   // step already passed
		{Height: 1, Round: 5, Step: consensus.StepPrecommit}, // another round
		{Height: 9, Round: 0, Step: consensus.StepPrecommit}, // another height
	} {
		e.OnTimeout(tm)
	}
	if len(env.logs) != before || e.Step != consensus.StepPrevote {
		t.Fatalf("stale timer had an effect: %q", env.logs[before:])
	}
}

// 2f+1 prevotes split between B and NIL: wait, then precommit NIL.
func TestPrevoteTimeoutPrecommitsNil(t *testing.T) {
	e, env, bh := readyEngine(t)
	votesFrom(e, []types.NodeID{2, 3, 4}, types.Prevote, bh)
	votesFrom(e, []types.NodeID{5, 6, 7}, types.Prevote, types.ZeroHash) // 7 votes, no quorum
	tm, n := timerFor(env, consensus.StepPrevote)
	if n != 1 || tm.after != 500 {
		t.Fatalf("prevote timer = %+v (%d)", tm, n)
	}
	votesFrom(e, []types.NodeID{8}, types.Prevote, types.ZeroHash)
	if _, n := timerFor(env, consensus.StepPrevote); n != 1 {
		t.Fatal("the prevote timer must be started only once per round")
	}
	e.OnTimeout(tm.t)
	if !env.logged("PRECOMMIT h=1 r=0 block nil") || e.LockedRound != -1 {
		t.Fatalf("logs=%q", env.logs)
	}
}

// 2f+1 precommits without a decision: next round, lock kept, longer timers.
func TestPrecommitTimeoutStartsNextRoundKeepingTheLock(t *testing.T) {
	e, env, bh := readyEngine(t)
	votesFrom(e, sixOthers, types.Prevote, bh) // lock on B, precommit B
	votesFrom(e, sixOthers, types.Precommit, types.ZeroHash)
	tm, n := timerFor(env, consensus.StepPrecommit)
	if n != 1 {
		t.Fatal("no precommit timer after 2f+1 precommits")
	}
	e.OnTimeout(tm.t)
	if e.Round != 1 || !env.logged("NEW_ROUND h=1 r=1") {
		t.Fatalf("round=%d logs=%q", e.Round, env.logs)
	}
	if e.LockedRound != 0 || e.LockedHash != bh {
		t.Fatal("a new round must keep the lock (only a new height clears it)")
	}
	if next, _ := timerFor(env, consensus.StepPropose); next.after != 1000+300 {
		t.Fatalf("round-1 propose timer = %d ms, want base + delta = 1300", next.after)
	}
}
