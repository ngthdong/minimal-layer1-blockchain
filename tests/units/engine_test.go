package unit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ------------------------------------------------------------------ engine

type sentTo struct {
	to  types.NodeID
	msg types.Message
}

// fakeEnv is a node without a network: it records what the engine sends and
// logs, and executes blocks against its own state.
type fakeEnv struct {
	logs   []string
	bcast  []types.Message
	sent   []sentTo
	st     *state.State
	toPack []types.Tx // what BuildBlock puts in the next block

	commits   []types.CommitCert // what Commit received
	commitErr error              // if set, Commit refuses
	timers    []scheduled        // what ScheduleTimeout received
}

type scheduled struct {
	after uint64
	t     consensus.Timeout
}

func (f *fakeEnv) ScheduleTimeout(after uint64, t consensus.Timeout) {
	f.timers = append(f.timers, scheduled{after, t})
}

func newFakeEnv() *fakeEnv { return &fakeEnv{st: state.New()} }

func (f *fakeEnv) Broadcast(m types.Message)             { f.bcast = append(f.bcast, m) }
func (f *fakeEnv) Send(to types.NodeID, m types.Message) { f.sent = append(f.sent, sentTo{to, m}) }
func (f *fakeEnv) Log(ev string, h uint64, r int32, kv ...string) {
	f.logs = append(f.logs, fmt.Sprintf("%s h=%d r=%d %s", ev, h, r, strings.Join(kv, " ")))
}
func (f *fakeEnv) LastHash() types.Hash { return types.ZeroHash }
func (f *fakeEnv) BuildBlock(h uint64, parent types.Hash, max int) (types.Block, *state.State) {
	post := f.st.Clone()
	if err := post.ApplyBlock(chain, registry(), f.toPack); err != nil {
		panic(err)
	}
	hdr := types.BlockHeader{ChainID: chain, Height: h, ParentHash: parent,
		TxRoot: types.TxRoot(f.toPack), StateHash: post.Hash()}
	return types.Block{Header: hdr, Txs: f.toPack}, post
}
func (f *fakeEnv) ExecuteBody(hdr *types.BlockHeader, txs []types.Tx, max int) (*state.State, error) {
	return consensus.ExecuteBody(chain, registry(), f.st, hdr, txs, max)
}
func (f *fakeEnv) Commit(b types.Block, cert types.CommitCert, post *state.State) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	f.commits = append(f.commits, cert)
	f.st = post
	return nil
}

// proposals counts the proposals broadcast (votes are broadcast too).
func proposals(f *fakeEnv) int {
	n := 0
	for _, m := range f.bcast {
		if _, ok := m.(*types.ProposalMsg); ok {
			n++
		}
	}
	return n
}

func (f *fakeEnv) logged(prefix string) bool {
	for _, l := range f.logs {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

var testConsensusConfig = consensus.Config{
	TimeoutProposeMs: 1000, TimeoutPrevoteMs: 500, TimeoutPrecommitMs: 500, TimeoutDeltaMs: 300, MaxBlockTxs: 10,
}

func newEngine(t *testing.T, id types.NodeID) (*consensus.Engine, *fakeEnv) {
	t.Helper()
	env := newFakeEnv()
	signer := consensus.NewVoteSigner(id, vPrivs[id], chain)
	return consensus.NewEngine(id, chain, vset(t), signer, testConsensusConfig, env), env
}

func TestStartHeightResetsAndEntersRoundZero(t *testing.T) {
	e, env := newEngine(t, 0) // node 0 is not the proposer of (1, 0)
	e.StartHeight(1)
	if e.Height != 1 || e.Round != 0 || e.Step != consensus.StepPropose {
		t.Fatalf("h=%d r=%d step=%d", e.Height, e.Round, e.Step)
	}
	if e.LockedRound != -1 || e.ValidRound != -1 || !e.LockedHash.IsZero() {
		t.Fatal("a new height must start unlocked")
	}
	if !env.logged("NEW_ROUND h=1 r=0 proposer 01") || len(env.bcast) != 0 {
		t.Fatalf("logs = %q, sent %d", env.logs, len(env.bcast))
	}
}

// proposerEngine starts height 1 on the proposer of (1, 0) and returns the
// proposal it broadcast and its txs.
func proposerEngine(t *testing.T) (*consensus.Engine, types.Proposal, []types.Tx) {
	t.Helper()
	id := vset(t).Proposer(1, 0)
	e, env := newEngine(t, id)
	env.toPack = []types.Tx{goodTx("Alice", 1, "a"), goodTx("Bob", 1, "b")}
	e.StartHeight(1)
	if proposals(env) != 1 || !env.logged("PROPOSE h=1 r=0") {
		t.Fatalf("proposer sent %d proposals, logs %q", proposals(env), env.logs)
	}
	return e, env.bcast[0].(*types.ProposalMsg).Proposal, env.toPack
}

func TestProposerAnnouncesASignedHeader(t *testing.T) {
	e, p, txs := proposerEngine(t)
	if err := consensus.VerifyProposal(chain, vset(t), types.ZeroHash, &p); err != nil {
		t.Fatalf("own proposal does not verify: %v", err)
	}
	if got, ok := e.Body(1, p.BlockHash()); !ok || len(got) != len(txs) {
		t.Fatal("the proposer must be able to serve its own body")
	}
}

// Header first, body only after the header is accepted, from the announcer.
func TestHeaderFirstThenBody(t *testing.T) {
	_, p, txs := proposerEngine(t)
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	bh := p.BlockHash()
	if !env.logged("HEADER_ACCEPT h=1 r=0 block "+bh.Short()) || e.HasBody(bh) {
		t.Fatalf("logs = %q", env.logs)
	}
	if len(env.sent) != 1 || env.sent[0].to != p.Proposer {
		t.Fatal("body must be requested from the peer that announced the header")
	}
	if req := env.sent[0].msg.(*types.BodyRequest); req.BlockHash != bh {
		t.Fatal("body request for the wrong block")
	}
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: txs})
	if !e.HasBody(bh) || !env.logged("BODY_ACCEPT h=1 r=0 block "+bh.Short()) {
		t.Fatalf("body not accepted: %q", env.logs)
	}
	if proposals(env) != 1 { // we can serve the body now: header relayed
		t.Fatalf("relayed %d proposals, want 1", proposals(env))
	}
}

func TestBadProposalIsRejectedWithoutBodyRequest(t *testing.T) {
	_, p, _ := proposerEngine(t)
	p.Round = 1 // round 1 belongs to another proposer; signature now wrong too
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnProposal(p.Proposer, &p)
	if !env.logged("REJECT h=1 r=1 type PROPOSAL") || len(env.sent) != 0 {
		t.Fatalf("logs = %q, sent = %d", env.logs, len(env.sent))
	}
}

func TestBadBodies(t *testing.T) {
	_, p, txs := proposerEngine(t)
	bh := p.BlockHash()
	e, env := newEngine(t, 0)
	e.StartHeight(1)
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: txs})
	if e.HasBody(bh) || len(env.logs) != 1 {
		t.Fatal("a body for a header we never accepted must be ignored")
	}
	e.OnProposal(p.Proposer, &p)
	swapped := []types.Tx{txs[1], txs[0]}
	e.OnBody(3, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: swapped})
	if e.HasBody(bh) || !env.logged("REJECT h=1 r=0 type BODY from 03 reason bad_tx_root") {
		t.Fatalf("tampered body accepted: %q", env.logs)
	}
	e.OnBody(p.Proposer, &types.BodyResponse{Height: 1, BlockHash: bh, Txs: txs})
	if !e.HasBody(bh) {
		t.Fatal("a corrupt copy from one peer must not block the correct body")
	}
}
