package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ourVotes counts the votes of node id that e broadcast.
func ourVotes(env *fakeEnv, id types.NodeID) int {
	n := 0
	for _, m := range env.bcast {
		if v, ok := m.(*types.VoteMsg); ok && v.Vote.Validator == id {
			n++
		}
	}
	return n
}

// Stuck after voting: the propose timer re-sends our vote and starts a
// rebroadcast chain that lasts exactly as long as the round.
func TestStuckRoundRebroadcastsOurVotes(t *testing.T) {
	e, env, _ := readyEngine(t) // prevoted the block, no quorum will come
	sent := ourVotes(env, 0)
	e.OnTimeout(consensus.Timeout{Height: 1, Round: 0, Step: consensus.StepPropose})
	if ourVotes(env, 0) != sent+1 || count(env, "TIMEOUT") != 0 {
		t.Fatalf("want our prevote sent again (no TIMEOUT log): logs=%q", env.logs)
	}
	tm, n := timerFor(env, consensus.StepRebroadcast)
	if n != 1 || tm.after != 1000 {
		t.Fatalf("rebroadcast timer = %+v (%d)", tm, n)
	}
	e.OnTimeout(tm.t) // the chain goes on while the round is stuck
	if _, n := timerFor(env, consensus.StepRebroadcast); n != 2 || ourVotes(env, 0) != sent+2 {
		t.Fatal("the rebroadcast timer must re-send and re-arm itself")
	}
	votesAt(e, others(0), types.Precommit, 0, types.ZeroHash)
	e.OnTimeout(consensus.Timeout{Height: 1, Round: 0, Step: consensus.StepPrecommit}) // → round 1
	sent = ourVotes(env, 0)
	e.OnTimeout(tm.t)
	if _, n := timerFor(env, consensus.StepRebroadcast); n != 2 || ourVotes(env, 0) != sent {
		t.Fatal("a rebroadcast timer of a past round must die")
	}
}

// f+1 distinct validators in a higher round: jump there. f are not enough, and
// a validator with both a prevote and a precommit counts once.
func TestRoundSkipOnFPlusOneValidators(t *testing.T) {
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	votesAt(e, []types.NodeID{2, 3, 4}, types.Prevote, 2, blockB)
	votesAt(e, []types.NodeID{2, 3, 4}, types.Precommit, 2, types.ZeroHash)
	if e.Round != 0 {
		t.Fatal("skipped on f validators")
	}
	votesAt(e, []types.NodeID{5}, types.Precommit, 2, types.ZeroHash) // f+1 = 4
	if e.Round != 2 || !env.logged("NEW_ROUND h=1 r=2 proposer 03 skip_from 0") {
		t.Fatalf("round=%d logs=%q", e.Round, env.logs)
	}
	if _, n := timerFor(env, consensus.StepPropose); n != 2 {
		t.Fatal("the new round must start its own propose timer")
	}
}
