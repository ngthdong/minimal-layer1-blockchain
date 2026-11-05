package unit

import (
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// readyEngine is node 0 at height 1 holding the round-0 proposal and its body.
func readyEngine(t *testing.T) (*consensus.Engine, *fakeEnv, types.Hash) {
	t.Helper()
	_, p, txs := proposerEngine(t)
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: p.BlockHash(), Txs: txs})
	return e, env, p.BlockHash()
}

func count(env *fakeEnv, prefix string) int {
	n := 0
	for _, l := range env.logs {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// Rule 3: with a valid body, prevote the block.
func TestPrevoteForValidBlock(t *testing.T) {
	e, env, bh := readyEngine(t)
	if e.Step != consensus.StepPrevote || !env.logged("PREVOTE h=1 r=0 block "+bh.Short()) {
		t.Fatalf("step=%d logs=%q", e.Step, env.logs)
	}
	v := env.bcast[len(env.bcast)-1].(*types.VoteMsg).Vote
	if v.Type != types.Prevote || v.BlockHash != bh || v.Validator != 0 {
		t.Fatal("broadcast vote is not our prevote for the block")
	}
}

// Rule 3: a block whose body does not execute to its StateHash gets a NIL prevote.
func TestPrevoteNilForInvalidBlock(t *testing.T) {
	_, p, txs := proposerEngine(t)
	e, env := newEngine(t, 0)
	env.st = env.st.Clone()
	bad := goodTx("Alice", 1, "a")
	_ = env.st.ApplyTx(chain, registry(), &bad) // our state already used Alice's nonce 1
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: p.BlockHash(), Txs: txs})
	if !env.logged("REJECT h=1 r=0 type BODY") || !env.logged("PREVOTE h=1 r=0 block nil") {
		t.Fatalf("logs = %q", env.logs)
	}
}

// T7 at engine level: the evidence is logged once and the vote not counted.
func TestEquivocationIsLoggedOnce(t *testing.T) {
	e, env, bh := readyEngine(t)
	a, b := signedVote(3, types.Prevote, 1, 0, bh), signedVote(3, types.Prevote, 1, 0, blockB)
	e.OnVote(3, &a)
	if !e.OnVote(3, &b) || !e.OnVote(3, &b) {
		t.Fatal("an equivocating vote must be relayed as evidence")
	}
	if count(env, "EQUIVOCATION h=1 r=0 validator 03") != 1 {
		t.Fatalf("logs = %q", env.logs)
	}
	key := consensus.VoteKey{Height: 1, Round: 0, Type: types.Prevote}
	if e.Votes.Count(key, blockB) != 0 {
		t.Fatal("equivocating vote was counted")
	}
}

func TestForgedVoteIsRejected(t *testing.T) {
	e, env, bh := readyEngine(t)
	v := signedVote(3, types.Prevote, 1, 0, bh)
	v.Validator = 4 // validator 3's signature under 4's name
	if e.OnVote(3, &v) || !env.logged("REJECT h=1 r=0 type VOTE from 03 reason bad_signature") {
		t.Fatalf("logs = %q", env.logs)
	}
}
