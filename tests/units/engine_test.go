package unit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

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
