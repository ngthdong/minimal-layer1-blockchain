package consensus

import (
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Step is where the engine is inside a round.
type Step uint8

const (
	StepPropose Step = iota + 1
	StepPrevote
	StepPrecommit
)

// Config holds the consensus parameters from the scenario file.
type Config struct {
	TimeoutProposeMs   uint64
	TimeoutPrevoteMs   uint64
	TimeoutPrecommitMs uint64
	TimeoutDeltaMs     uint64 // added per round: timeout(r) = base + r*delta
	MaxBlockTxs        int
}

// Env is everything the engine needs from the node around it. Keeping it an
// interface lets unit tests drive the engine without a network. 
type Env interface {
	Broadcast(msg types.Message)
	Log(ev string, height uint64, round int32, kv ...string)
}

// Engine is one validator's consensus state machine. It runs on the
// simulator's single goroutine: no locking.
type Engine struct {
	ID     types.NodeID
	vals   *ValidatorSet
	cfg    Config
	env    Env
	signer *VoteSigner

	Height uint64
	Round  int32
	Step   Step

	// Locking. -1 = none.
	LockedRound int32
	LockedHash  types.Hash
	ValidRound  int32
	ValidHash   types.Hash

	Votes *VoteSet // votes of the current height
}

func NewEngine(id types.NodeID, vals *ValidatorSet, signer *VoteSigner, cfg Config, env Env) *Engine {
	return &Engine{ID: id, vals: vals, cfg: cfg, env: env, signer: signer}
}

// StartHeight resets everything that belongs to one height and enters round 0.
// Locks never survive a height: they only protect the block of THIS height.
func (e *Engine) StartHeight(h uint64) {
	e.Height = h
	e.Votes = NewVoteSet()
	e.LockedRound, e.LockedHash = -1, types.ZeroHash
	e.ValidRound, e.ValidHash = -1, types.ZeroHash
	e.signer.Prune(h)
	e.enterNewRound(0)
}

// enterNewRound moves to round r of the current height. Proposing 
// and the propose timeout start here.
func (e *Engine) enterNewRound(r int32) {
	e.Round, e.Step = r, StepPropose
	e.env.Log(logging.EvNewRound, e.Height, r, "proposer", fmt.Sprintf("%02d", e.vals.Proposer(e.Height, r)))
}

// castVote signs and broadcasts our vote of type typ for hash in the current
// round, and counts it in our own vote set immediately (it never travels
// through the network back to us). It returns false, and sends nothing, if
// the signer refuses because a different vote was already signed.
func (e *Engine) castVote(typ types.VoteType, hash types.Hash) bool {
	v, err := e.signer.Sign(typ, e.Height, e.Round, hash)
	if err != nil {
		return false
	}
	e.Votes.Add(v)
	ev := logging.EvPrevote
	if typ == types.Precommit {
		ev = logging.EvPrecommit
	}
	e.env.Log(ev, e.Height, e.Round, "block", blockLabel(hash))
	e.env.Broadcast(&types.VoteMsg{Vote: v})
	return true
}

// blockLabel prints a block hash in logs, "nil" for a NIL vote.
func blockLabel(h types.Hash) string {
	if h.IsZero() {
		return "nil"
	}
	return h.Short()
}
