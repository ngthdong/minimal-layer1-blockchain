package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// votesAt feeds signed votes of height 1, round r into e.
func votesAt(e *consensus.Engine, ids []types.NodeID, typ types.VoteType, r int32, bh types.Hash) {
	for _, id := range ids {
		v := signedVote(id, typ, 1, r, bh)
		e.OnVote(id, &v)
	}
}

// others returns 2f+1 = 7 validators other than id.
func others(id types.NodeID) []types.NodeID {
	var out []types.NodeID
	for v := types.NodeID(1); len(out) < 7; v++ {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

// block returns a valid height-1 header built and signed by builder.
func block(builder types.NodeID, txs ...types.Tx) (types.BlockHeader, []types.Tx) {
	env := newFakeEnv()
	env.toPack = txs
	b, _ := env.BuildBlock(1, types.ZeroHash, 10)
	consensus.NewVoteSigner(builder, vPrivs[builder], chain).SignHeader(&b.Header)
	return b.Header, b.Txs
}

// proposeAt returns the proposal of round r for hdr, signed by r's proposer.
func proposeAt(t *testing.T, hdr types.BlockHeader, r, pol int32) types.Proposal {
	t.Helper()
	p := types.Proposal{Height: 1, Round: r, POLRound: pol, Header: hdr}
	id := vset(t).Proposer(1, r)
	if err := consensus.NewVoteSigner(id, vPrivs[id], chain).SignProposal(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

// deliver hands e a proposal and the body of its block.
func deliver(e *consensus.Engine, p types.Proposal, txs []types.Tx) {
	e.OnProposal(p.Proposer, &p)
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: p.BlockHash(), Txs: txs})
}

// nextRound ends e's round without a decision: 2f+1 NIL precommits, then the
// precommit timer fires.
func nextRound(t *testing.T, e *consensus.Engine) {
	t.Helper()
	r := e.Round
	votesAt(e, others(e.ID), types.Precommit, r, types.ZeroHash)
	e.OnTimeout(consensus.Timeout{Height: 1, Round: r, Step: consensus.StepPrecommit})
	if e.Round != r+1 {
		t.Fatalf("still in round %d", e.Round)
	}
}

// A proposer that saw 2f+1 prevotes for A proposes A again, unchanged, with
// POLRound = the round of those prevotes, instead of a fresh mempool block.
func TestProposerReProposesValidBlock(t *testing.T) {
	id := vset(t).Proposer(1, 1)
	e, env := newEngine(t, id)
	env.toPack = []types.Tx{goodTx("Bob", 1, "b")} // a fresh block would differ
	hdrA, txsA := block(1, goodTx("Alice", 1, "a"))
	a := hdrA.Hash()
	e.StartHeight(1)
	deliver(e, proposeAt(t, hdrA, 0, -1), txsA)
	votesAt(e, others(id), types.Prevote, 0, a) // lock on A, ValidRound 0
	nextRound(t, e)
	p := env.bcast[len(env.bcast)-2].(*types.ProposalMsg).Proposal // last is our prevote
	if p.Round != 1 || p.POLRound != 0 || p.BlockHash() != a || p.Header.Proposer != 1 {
		t.Fatalf("re-proposal = r%d pol%d builder %d", p.Round, p.POLRound, p.Header.Proposer)
	}
	if err := consensus.VerifyProposal(chain, vset(t), types.ZeroHash, &p); err != nil {
		t.Fatalf("re-proposal does not verify: %v", err)
	}
	if !env.logged("PROPOSE h=1 r=1 block "+a.Short()+" txs 1 pol 0") ||
		!env.logged("PREVOTE h=1 r=1 block "+a.Short()) {
		t.Fatalf("logs = %q", env.logs)
	}
}

// A re-proposal is prevoted only once its POL (2f+1 prevotes in POLRound)
// has reached us; then we also remember A as the valid block.
func TestReProposalWaitsForItsPOL(t *testing.T) {
	e, env := newEngine(t, 0)
	hdrA, txsA := block(1, goodTx("Alice", 1, "a"))
	a := hdrA.Hash()
	e.StartHeight(1)
	deliver(e, proposeAt(t, hdrA, 0, -1), txsA) // we prevote A, see no quorum
	nextRound(t, e)
	before := proposals(env)
	p1 := proposeAt(t, hdrA, 1, 0)
	e.OnProposal(p1.Proposer, &p1)
	if proposals(env) != before+1 || count(env, "PREVOTE h=1 r=1") != 0 {
		t.Fatal("known body: relay the re-proposal, but do not prevote without the POL")
	}
	votesAt(e, others(0)[:6], types.Prevote, 0, a) // late round-0 prevotes: 7 with ours
	if !env.logged("PREVOTE h=1 r=1 block "+a.Short()) || e.ValidRound != 0 || e.ValidHash != a {
		t.Fatalf("valid=%d logs=%q", e.ValidRound, env.logs)
	}
}

// Locked on A in round 0; C got 2f+1 prevotes in round 1 (which we missed).
// C re-proposed with POLRound 1 >= our lock round: unlock and prevote C.
func TestUnlockOnLaterPOL(t *testing.T) {
	e, env := newEngine(t, 0)
	hdrA, txsA := block(1, goodTx("Alice", 1, "a"))
	hdrC, txsC := block(2, goodTx("Bob", 1, "c"))
	a, c := hdrA.Hash(), hdrC.Hash()
	e.StartHeight(1)
	deliver(e, proposeAt(t, hdrA, 0, -1), txsA)
	votesAt(e, others(0), types.Prevote, 0, a) // lock A in round 0
	nextRound(t, e)
	deliver(e, proposeAt(t, hdrC, 1, -1), txsC)
	if !env.logged("PREVOTE h=1 r=1 block nil") {
		t.Fatal("locked on A: a fresh C must get a NIL prevote (rule 5)")
	}
	nextRound(t, e)
	p2 := proposeAt(t, hdrC, 2, 1)
	e.OnProposal(p2.Proposer, &p2)
	votesAt(e, others(0), types.Prevote, 1, c) // the round-1 POL for C arrives
	if !env.logged("UNLOCK h=1 r=2 block "+a.Short()+" locked_round 0 pol 1") ||
		!env.logged("PREVOTE h=1 r=2 block "+c.Short()) || e.LockedRound != -1 {
		t.Fatalf("locked=%d logs=%q", e.LockedRound, env.logs)
	}
}

// Locked on A in round 1: a POL for C from round 0 is OLDER than our lock and
// must not unlock us (that is what keeps a possibly decided A safe).
func TestOlderPOLDoesNotUnlock(t *testing.T) {
	e, env := newEngine(t, 0)
	hdrC, txsC := block(1, goodTx("Bob", 1, "c"))
	hdrA, txsA := block(2, goodTx("Alice", 1, "a"))
	a, c := hdrA.Hash(), hdrC.Hash()
	e.StartHeight(1)
	deliver(e, proposeAt(t, hdrC, 0, -1), txsC)
	nextRound(t, e)
	deliver(e, proposeAt(t, hdrA, 1, -1), txsA)
	votesAt(e, others(0), types.Prevote, 1, a) // lock A in round 1
	votesAt(e, others(0), types.Prevote, 0, c) // late POL for C, round 0
	nextRound(t, e)
	p2 := proposeAt(t, hdrC, 2, 0)
	e.OnProposal(p2.Proposer, &p2)
	if !env.logged("PREVOTE h=1 r=2 block nil") || e.LockedRound != 1 || e.LockedHash != a {
		t.Fatalf("locked=%d logs=%q", e.LockedRound, env.logs)
	}
	if count(env, "UNLOCK") != 0 {
		t.Fatal("unlocked on an older POL")
	}
}
