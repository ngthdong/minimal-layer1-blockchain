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

// votesFrom feeds signed votes of the given validators into e.
func votesFrom(e *consensus.Engine, ids []types.NodeID, typ types.VoteType, bh types.Hash) {
	for _, id := range ids {
		v := signedVote(id, typ, 1, 0, bh)
		e.OnVote(id, &v)
	}
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

// Rule 4: 2f+1 prevotes (ours + 6) → lock and precommit; 2f is not enough.
func TestLockAndPrecommitOnPrevoteQuorum(t *testing.T) {
	e, env, bh := readyEngine(t)
	votesFrom(e, []types.NodeID{2, 3, 4, 5, 6}, types.Prevote, bh) // 6 with ours
	if e.Step != consensus.StepPrevote || e.LockedRound != -1 {
		t.Fatal("locked with only 2f prevotes")
	}
	votesFrom(e, []types.NodeID{7}, types.Prevote, bh) // 7 = 2f+1
	if e.Step != consensus.StepPrecommit || e.LockedRound != 0 || e.LockedHash != bh {
		t.Fatalf("step=%d locked=%d", e.Step, e.LockedRound)
	}
	if !env.logged("LOCK h=1 r=0 block "+bh.Short()) || !env.logged("PRECOMMIT h=1 r=0 block "+bh.Short()) {
		t.Fatalf("logs = %q", env.logs)
	}
	votesFrom(e, []types.NodeID{8, 9}, types.Prevote, bh)
	if count(env, "PRECOMMIT") != 1 || count(env, "LOCK") != 1 {
		t.Fatal("more prevotes must not lock or precommit again")
	}
}

// 2f+1 NIL prevotes → precommit NIL, no lock.
func TestPrecommitNilOnNilQuorum(t *testing.T) {
	e, env, _ := readyEngine(t)
	votesFrom(e, []types.NodeID{2, 3, 4, 5, 6, 7, 8}, types.Prevote, types.ZeroHash)
	if !env.logged("PRECOMMIT h=1 r=0 block nil") || e.LockedRound != -1 {
		t.Fatalf("locked=%d logs=%q", e.LockedRound, env.logs)
	}
}

// A prevote quorum seen before our own prevote waits for us (we need the body).
func TestQuorumBeforeBodyWaits(t *testing.T) {
	_, p, txs := proposerEngine(t)
	bh := p.BlockHash()
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	votesFrom(e, []types.NodeID{1, 2, 3, 4, 5, 6, 7}, types.Prevote, bh)
	if count(env, "PRECOMMIT") != 0 {
		t.Fatal("precommitted a block whose body we do not have")
	}
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: txs})
	if !env.logged("PREVOTE h=1 r=0 block "+bh.Short()) || !env.logged("PRECOMMIT h=1 r=0 block "+bh.Short()) {
		t.Fatalf("logs = %q", env.logs)
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

// T7 at engine level: the evidence is logged once and relayed once; the
// second vote counts toward its own block, the validator still once per tally.
func TestEquivocationIsLoggedOnce(t *testing.T) {
	e, env, bh := readyEngine(t)
	a, b := signedVote(3, types.Prevote, 1, 0, bh), signedVote(3, types.Prevote, 1, 0, blockB)
	e.OnVote(3, &a)
	if !e.OnVote(3, &b) {
		t.Fatal("an equivocating vote must be relayed as evidence")
	}
	if e.OnVote(3, &b) {
		t.Fatal("the same evidence again is a duplicate")
	}
	if count(env, "EQUIVOCATION h=1 r=0 validator 03") != 1 {
		t.Fatalf("logs = %q", env.logs)
	}
	key := consensus.VoteKey{Height: 1, Round: 0, Type: types.Prevote}
	if e.Votes.Count(key, bh) != 2 || e.Votes.Count(key, blockB) != 1 || e.Votes.CountAny(key) != 2 {
		t.Fatal("want 2 votes for the block (ours, 3), 1 for B (3), 2 validators")
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

// A peer that tampers with a tx signature in the body of a good block only
// gets ITS copy refused (bad_tx_root): the block is not marked invalid, and
// the real body from another peer is still accepted and prevoted.
func TestTamperedBodyDoesNotCondemnTheBlock(t *testing.T) {
	_, p, txs := proposerEngine(t)
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	bad := types.BodyResponse{Height: 1, BlockHash: p.BlockHash(), Txs: []types.Tx{txs[0].Clone(), txs[1]}}
	bad.Txs[0].Sig[5] ^= 1
	e.OnBody(7, &bad)
	if !env.logged("REJECT h=1 r=0 type BODY from 07 reason bad_tx_root") || count(env, "PREVOTE") != 0 {
		t.Fatalf("logs = %q", env.logs)
	}
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: p.BlockHash(), Txs: txs})
	if !env.logged("PREVOTE h=1 r=0 block " + p.BlockHash().Short()) {
		t.Fatalf("the good body was not accepted: %q", env.logs)
	}
}
