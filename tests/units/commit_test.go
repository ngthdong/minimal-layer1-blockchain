package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/ledger"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var sixOthers = []types.NodeID{2, 3, 4, 5, 6, 7}

// 2f+1 precommits for B → a verifiable certificate, commit, next height.
func TestCommitOnPrecommitQuorum(t *testing.T) {
	e, env, bh := readyEngine(t)
	votesFrom(e, sixOthers, types.Prevote, bh)       // lock + our precommit
	votesFrom(e, sixOthers[:5], types.Precommit, bh) // 6 precommits: not yet
	if len(env.commits) != 0 {
		t.Fatal("committed with 2f precommits")
	}
	votesFrom(e, sixOthers[5:], types.Precommit, bh) // 7 = 2f+1
	if len(env.commits) != 1 {
		t.Fatalf("commits = %d, want 1", len(env.commits))
	}
	cert := env.commits[0]
	if err := consensus.VerifyCommitCert(chain, vset(t), &cert); err != nil || cert.BlockHash != bh {
		t.Fatalf("certificate does not verify: %v", err)
	}
	if e.Height != 2 || e.Round != 0 || e.LockedRound != -1 {
		t.Fatalf("after commit: h=%d r=%d locked=%d", e.Height, e.Round, e.LockedRound)
	}
	if !env.logged("COMMIT h=1 r=0 block "+bh.Short()) || !env.logged("NEW_ROUND h=2 r=0") {
		t.Fatalf("logs = %q", env.logs)
	}
}

func TestNoCommitOnNilPrecommits(t *testing.T) {
	e, env, _ := readyEngine(t)
	votesFrom(e, []types.NodeID{1, 2, 3, 4, 5, 6, 7}, types.Precommit, types.ZeroHash)
	if len(env.commits) != 0 || e.Height != 1 {
		t.Fatal("a NIL quorum decides nothing")
	}
}

// The decision can be seen before the body: commit as soon as it arrives.
func TestCommitWaitsForBody(t *testing.T) {
	_, p, txs := proposerEngine(t)
	bh := p.BlockHash()
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	votesFrom(e, []types.NodeID{1, 2, 3, 4, 5, 6, 7}, types.Precommit, bh)
	if len(env.commits) != 0 {
		t.Fatal("committed a block whose body we do not have")
	}
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: txs})
	if len(env.commits) != 1 || e.Height != 2 {
		t.Fatal("did not commit once the body arrived")
	}
}

// If our own ledger refuses the block (safety alarm), we must not move on.
func TestCommitRefusedStopsTheHeight(t *testing.T) {
	e, env, bh := readyEngine(t)
	env.commitErr = ledger.ErrConflict
	votesFrom(e, sixOthers, types.Prevote, bh)
	votesFrom(e, sixOthers, types.Precommit, bh)
	if e.Height != 1 || !env.logged("REJECT h=1 r=0 type COMMIT block "+bh.Short()+" reason conflicting_block") {
		t.Fatalf("h=%d logs=%q", e.Height, env.logs)
	}
}
