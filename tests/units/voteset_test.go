package unit

import (
	"reflect"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var (
	blockA  = types.Hash{0xa}
	blockB  = types.Hash{0xb}
	prevote = consensus.VoteKey{Height: 1, Round: 0, Type: types.Prevote}
)

func voteFor(id types.NodeID, typ types.VoteType, round int32, h types.Hash) types.Vote {
	return types.Vote{Type: typ, Height: 1, Round: round, BlockHash: h, Validator: id, Sig: []byte{byte(id)}}
}

// The same vote delivered many times is counted once.
func TestDuplicateVotesCountOnce(t *testing.T) {
	vs := consensus.NewVoteSet()
	v := voteFor(0, types.Prevote, 0, blockA)
	if r, _ := vs.Add(v); r != consensus.Added {
		t.Fatalf("first add = %v", r)
	}
	for i := 0; i < 5; i++ {
		if r, _ := vs.Add(v); r != consensus.Duplicate {
			t.Fatalf("repeat add = %v, want duplicate", r)
		}
	}
	if vs.Count(prevote, blockA) != 1 || vs.CountAny(prevote) != 1 {
		t.Fatal("duplicate vote was counted again")
	}
}

// T7: a second, different vote by the same validator is flagged, the first
// one is returned as evidence, and each vote counts toward its own block.
func TestEquivocationDetected(t *testing.T) {
	vs := consensus.NewVoteSet()
	vs.Add(voteFor(3, types.Prevote, 0, blockA))
	r, first := vs.Add(voteFor(3, types.Prevote, 0, blockB))
	if r != consensus.Equivocation || first.BlockHash != blockA {
		t.Fatalf("result = %v, kept = %x", r, first.BlockHash[:1])
	}
	if r, _ := vs.Add(voteFor(3, types.Prevote, 0, blockB)); r != consensus.Duplicate {
		t.Fatalf("the same second vote again = %v, want duplicate", r)
	}
	if vs.Count(prevote, blockA) != 1 || vs.Count(prevote, blockB) != 1 || vs.CountAny(prevote) != 1 {
		t.Fatal("want one count for A, one for B, one validator")
	}
	if !reflect.DeepEqual(vs.Equivocators(prevote), []types.NodeID{3}) {
		t.Fatalf("equivocators = %v", vs.Equivocators(prevote))
	}
}

// The case that stalled T7: honest 0..5 vote A; equivocator 6 sent A to some
// nodes and NIL to others. Whichever copy arrived first, every node counts
// 7 for A — a quorum, and a certificate with 7 distinct validators.
func TestEquivocatorCountsOncePerTally(t *testing.T) {
	for _, order := range [][]types.Hash{{blockA, types.ZeroHash}, {types.ZeroHash, blockA}} {
		vs := consensus.NewVoteSet()
		for id := types.NodeID(0); id < 6; id++ {
			vs.Add(voteFor(id, types.Prevote, 0, blockA))
		}
		vs.Add(voteFor(6, types.Prevote, 0, order[0]))
		vs.Add(voteFor(6, types.Prevote, 0, order[1]))
		if h, ok := vs.Majority(prevote, 7); !ok || h != blockA || len(vs.VotesFor(prevote, blockA)) != 7 {
			t.Fatalf("order %x: no quorum for A", order[0][:1])
		}
		if vs.Count(prevote, types.ZeroHash) != 1 {
			t.Fatal("the NIL vote counts once for NIL")
		}
	}
	vs := consensus.NewVoteSet()
	for i := 0; i < 10; i++ { // one validator, ten different blocks
		vs.Add(voteFor(9, types.Prevote, 0, types.Hash{byte(i + 1)}))
	}
	if n := vs.Count(prevote, types.Hash{10}); n != 0 {
		t.Fatal("conflicting votes beyond the cap must not be kept")
	}
}

func TestQuorumNeedsTwoFPlusOneForTheSameBlock(t *testing.T) {
	vs, q := consensus.NewVoteSet(), 7 // n = 10, f = 3
	for id := types.NodeID(0); id < 6; id++ {
		vs.Add(voteFor(id, types.Prevote, 0, blockA))
	}
	if _, ok := vs.Majority(prevote, q); ok {
		t.Fatal("quorum reported with 2f votes")
	}
	vs.Add(voteFor(6, types.Prevote, 0, blockA))
	if h, ok := vs.Majority(prevote, q); !ok || h != blockA {
		t.Fatal("no quorum with 2f+1 votes")
	}
}

// NIL votes are counted on their own: 4 for A plus 3 NIL is not a quorum
// for A, but it is "2f+1 of any" (which starts a timeout).
func TestNilVotesAreSeparate(t *testing.T) {
	vs, q := consensus.NewVoteSet(), 7
	for id := types.NodeID(0); id < 4; id++ {
		vs.Add(voteFor(id, types.Prevote, 0, blockA))
	}
	for id := types.NodeID(4); id < 7; id++ {
		vs.Add(voteFor(id, types.Prevote, 0, types.ZeroHash))
	}
	if _, ok := vs.Majority(prevote, q); ok || vs.CountAny(prevote) != 7 {
		t.Fatal("mixed A/NIL votes must not form a quorum, but count as 7 votes")
	}
	vs2 := consensus.NewVoteSet()
	for id := types.NodeID(0); id < 7; id++ {
		vs2.Add(voteFor(id, types.Prevote, 0, types.ZeroHash))
	}
	if h, ok := vs2.Majority(prevote, q); !ok || !h.IsZero() {
		t.Fatal("2f+1 NIL votes must be a NIL quorum")
	}
}

// Votes for another round or phase never mix with this step.
func TestStepsAreSeparate(t *testing.T) {
	vs := consensus.NewVoteSet()
	vs.Add(voteFor(0, types.Prevote, 0, blockA))
	vs.Add(voteFor(0, types.Precommit, 0, blockB)) // not an equivocation
	if r, _ := vs.Add(voteFor(0, types.Prevote, 1, blockB)); r != consensus.Added {
		t.Fatal("a vote in round 1 is a new step, not an equivocation")
	}
	if vs.Count(prevote, blockA) != 1 || vs.CountAny(prevote) != 1 {
		t.Fatal("votes leaked between steps")
	}
}

// Same votes added in different orders -> same certificate content.
func TestVotesForIsSortedAndCopied(t *testing.T) {
	order1, order2 := []types.NodeID{5, 1, 9, 3}, []types.NodeID{9, 3, 5, 1}
	var got [][]types.NodeID
	for _, order := range [][]types.NodeID{order1, order2} {
		vs := consensus.NewVoteSet()
		for _, id := range order {
			vs.Add(voteFor(id, types.Precommit, 0, blockA))
		}
		votes := vs.VotesFor(consensus.VoteKey{Height: 1, Type: types.Precommit}, blockA)
		ids := []types.NodeID{}
		for _, v := range votes {
			ids = append(ids, v.Validator)
		}
		got = append(got, ids)
		votes[0].Sig[0] = 99 // must not reach the set
		if again := vs.VotesFor(consensus.VoteKey{Height: 1, Type: types.Precommit}, blockA); again[0].Sig[0] == 99 {
			t.Fatal("VotesFor returned the stored slice")
		}
	}
	want := []types.NodeID{1, 3, 5, 9}
	if !reflect.DeepEqual(got[0], want) || !reflect.DeepEqual(got[1], want) {
		t.Fatalf("orders = %v / %v, want %v", got[0], got[1], want)
	}
}
