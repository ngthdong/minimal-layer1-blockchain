package unit

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ------------------------------------------------------------- vote signer

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
}

// ------------------------------------------------------------------ engine

type fakeEnv struct {
	logs []string
	sent []types.Message
}

func (f *fakeEnv) Broadcast(m types.Message) { f.sent = append(f.sent, m) }
func (f *fakeEnv) Log(ev string, h uint64, r int32, kv ...string) {
	f.logs = append(f.logs, fmt.Sprintf("%s h=%d r=%d %s", ev, h, r, strings.Join(kv, " ")))
}

func newEngine(t *testing.T, id types.NodeID) (*consensus.Engine, *fakeEnv) {
	t.Helper()
	env := &fakeEnv{}
	signer := consensus.NewVoteSigner(id, vPrivs[id], chain)
	return consensus.NewEngine(id, vset(t), signer, consensus.Config{}, env), env
}

func TestStartHeightResetsAndEntersRoundZero(t *testing.T) {
	e, env := newEngine(t, 0)
	e.StartHeight(7)
	if e.Height != 7 || e.Round != 0 || e.Step != consensus.StepPropose {
		t.Fatalf("h=%d r=%d step=%d", e.Height, e.Round, e.Step)
	}
	if e.LockedRound != -1 || e.ValidRound != -1 || !e.LockedHash.IsZero() {
		t.Fatal("a new height must start unlocked")
	}
	want := fmt.Sprintf("NEW_ROUND h=7 r=0 proposer %02d", vset(t).Proposer(7, 0))
	if len(env.logs) != 1 || env.logs[0] != want {
		t.Fatalf("logs = %q, want %q", env.logs, want)
	}
	if len(env.sent) != 0 {
		t.Fatal("entering a round sends nothing yet")
	}
}
