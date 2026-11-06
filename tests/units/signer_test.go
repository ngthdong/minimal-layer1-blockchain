package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

func TestSignerSignsValidVotes(t *testing.T) {
	s := consensus.NewVoteSigner(3, vPrivs[3], chain)
	v, err := s.Sign(types.Prevote, 5, 0, blockA)
	if err != nil {
		t.Fatal(err)
	}
	if err := consensus.VerifyVote(chain, vset(t), &v); err != nil || v.Validator != 3 {
		t.Fatalf("signed vote does not verify: %v", err)
	}
}

// Rules 1 and 2: never two different votes for the same (height, round, type).
func TestSignerRefusesDoubleSign(t *testing.T) {
	s := consensus.NewVoteSigner(3, vPrivs[3], chain)
	first, _ := s.Sign(types.Prevote, 5, 0, blockA)
	if _, err := s.Sign(types.Prevote, 5, 0, blockB); !errors.Is(err, consensus.ErrDoubleSign) {
		t.Fatalf("second prevote for another block: err = %v", err)
	}
	if _, err := s.Sign(types.Prevote, 5, 0, types.ZeroHash); !errors.Is(err, consensus.ErrDoubleSign) {
		t.Fatalf("switching to NIL in the same step: err = %v", err)
	}
	again, err := s.Sign(types.Prevote, 5, 0, blockA) // same vote: allowed (re-send)
	if err != nil || string(again.Sig) != string(first.Sig) {
		t.Fatal("asking again for the same vote must return the stored vote")
	}
}

// Other rounds, phases and heights are different steps.
func TestSignerAllowsOtherSteps(t *testing.T) {
	s := consensus.NewVoteSigner(3, vPrivs[3], chain)
	steps := []struct {
		typ types.VoteType
		h   uint64
		r   int32
		bh  types.Hash
	}{
		{types.Prevote, 5, 0, blockA},
		{types.Precommit, 5, 0, types.ZeroHash},
		{types.Prevote, 5, 1, blockB},
		{types.Prevote, 6, 0, blockB},
	}
	for _, st := range steps {
		if _, err := s.Sign(st.typ, st.h, st.r, st.bh); err != nil {
			t.Fatalf("%v h=%d r=%d: %v", st.typ, st.h, st.r, err)
		}
	}
	if v, ok := s.Signed(types.Prevote, 5, 1); !ok || v.BlockHash != blockB {
		t.Fatal("Signed did not return the stored vote")
	}
	s.Prune(6)
	if _, ok := s.Signed(types.Prevote, 5, 1); ok {
		t.Fatal("Prune kept a vote below the height")
	}
	// The forgotten height can never be signed again, with any block.
	if _, err := s.Sign(types.Prevote, 5, 1, blockA); !errors.Is(err, consensus.ErrStaleHeight) {
		t.Fatalf("vote below the floor: %v, want stale_height", err)
	}
	p := types.Proposal{Height: 5, Round: 2, POLRound: -1, Header: types.BlockHeader{Height: 5}}
	if err := s.SignProposal(&p); !errors.Is(err, consensus.ErrStaleHeight) {
		t.Fatalf("proposal below the floor: %v, want stale_height", err)
	}
}

// Rule 6: no vote of a wrong phase or round. (Chain and domain cannot be
// wrong: the signer holds its chain id and always signs under "VOTE:".)
func TestSignerRefusesWrongPhaseOrRound(t *testing.T) {
	s := consensus.NewVoteSigner(3, vPrivs[3], chain)
	if _, err := s.Sign(types.VoteType(7), 5, 0, blockA); !errors.Is(err, consensus.ErrBadVoteType) {
		t.Fatalf("phase 7: %v, want bad_vote_type", err)
	}
	if _, err := s.Sign(types.Prevote, 5, -1, blockA); !errors.Is(err, consensus.ErrBadRound) {
		t.Fatalf("round -1: %v, want bad_round", err)
	}
}

// Proposals get the same guard: one per (height, round).
func TestSignerRefusesTwoProposalsPerRound(t *testing.T) {
	s := consensus.NewVoteSigner(1, vPrivs[1], chain)
	p1 := types.Proposal{Height: 5, Round: 0, POLRound: -1, Header: types.BlockHeader{Height: 5}}
	p2 := p1
	p2.Header.StateHash[0] = 1 // another block
	if err := s.SignProposal(&p1); err != nil {
		t.Fatal(err)
	}
	if err := s.SignProposal(&p2); !errors.Is(err, consensus.ErrDoubleSign) {
		t.Fatalf("second proposal in the same round: %v", err)
	}
}
