package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Votes for height 2 that arrive while we are at height 1 are kept and
// counted as soon as we reach height 2.
func TestNextHeightVotesAreReplayed(t *testing.T) {
	e, _, bh := readyEngine(t)
	early := []types.NodeID{3, 4, 5}
	for _, id := range early {
		v := signedVote(id, types.Prevote, 2, 0, blockB)
		if !e.OnVote(id, &v) {
			t.Fatal("a valid vote for the next height must be kept and relayed")
		}
	}
	far := signedVote(6, types.Prevote, 3, 0, blockB) // two heights ahead: dropped
	e.OnVote(6, &far)

	votesFrom(e, sixOthers, types.Prevote, bh)
	votesFrom(e, sixOthers, types.Precommit, bh) // commit height 1
	if e.Height != 2 {
		t.Fatalf("height = %d, want 2", e.Height)
	}
	key := consensus.VoteKey{Height: 2, Round: 0, Type: types.Prevote}
	if got := e.Votes.Count(key, blockB); got != len(early) {
		t.Fatalf("replayed %d early votes, want %d", got, len(early))
	}
}

// Forged votes are rejected at once, even for a future height.
func TestForgedFutureVoteIsRejected(t *testing.T) {
	e, env, _ := readyEngine(t)
	v := signedVote(3, types.Prevote, 2, 0, blockB)
	v.Validator = 4
	if e.OnVote(3, &v) || !env.logged("REJECT h=2 r=0 type VOTE from 03 reason bad_signature") {
		t.Fatalf("logs = %q", env.logs)
	}
}
