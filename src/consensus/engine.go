package consensus

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
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
	Send(to types.NodeID, msg types.Message)
	Log(ev string, height uint64, round int32, kv ...string)

	// LastHash is the hash of the last finalized block (parent of the next).
	LastHash() types.Hash
	// BuildBlock reaps the mempool and executes the txs on a copy of the
	// state. The header has TxRoot and StateHash filled, but is unsigned.
	BuildBlock(height uint64, parent types.Hash, maxTxs int) (types.Block, *state.State)
	// ExecuteBody runs consensus.ExecuteBody against the node's state.
	ExecuteBody(hdr *types.BlockHeader, txs []types.Tx, maxTxs int) (*state.State, error)
}

// pending is a block of the current height whose body was checked.
type pending struct {
	block types.Block
	post  *state.State // state after the block, installed at commit
}

// Engine is one validator's consensus state machine. It runs on the
// simulator's single goroutine: no locking.
type Engine struct {
	ID      types.NodeID
	chainID string
	vals    *ValidatorSet
	cfg     Config
	env     Env
	signer  *VoteSigner

	Height uint64
	Round  int32
	Step   Step

	// Locking. -1 = none.
	LockedRound int32
	LockedHash  types.Hash
	ValidRound  int32
	ValidHash   types.Hash

	Votes *VoteSet // votes of the current height

	proposals map[int32]types.Proposal         // accepted proposal per round
	headers   map[types.Hash]types.BlockHeader // accepted headers, waiting for or with a body
	bodies    map[types.Hash]*pending          // blocks whose body executed correctly
	invalid   map[types.Hash]bool              // blocks whose body is provably wrong
}

func NewEngine(id types.NodeID, chainID string, vals *ValidatorSet, signer *VoteSigner, cfg Config, env Env) *Engine {
	return &Engine{ID: id, chainID: chainID, vals: vals, cfg: cfg, env: env, signer: signer}
}

// StartHeight resets everything that belongs to one height and enters round 0.
// Locks never survive a height: they only protect the block of THIS height.
func (e *Engine) StartHeight(h uint64) {
	e.Height = h
	e.Votes = NewVoteSet()
	e.LockedRound, e.LockedHash = -1, types.ZeroHash
	e.ValidRound, e.ValidHash = -1, types.ZeroHash
	e.proposals = make(map[int32]types.Proposal)
	e.headers = make(map[types.Hash]types.BlockHeader)
	e.bodies = make(map[types.Hash]*pending)
	e.invalid = make(map[types.Hash]bool)
	e.signer.Prune(h)
	e.enterNewRound(0)
}

// enterNewRound moves to round r of the current height; the proposer of the
// round builds and announces a block.
func (e *Engine) enterNewRound(r int32) {
	e.Round, e.Step = r, StepPropose
	proposer := e.vals.Proposer(e.Height, r)
	e.env.Log(logging.EvNewRound, e.Height, r, "proposer", node(proposer))
	if proposer == e.ID {
		e.propose()
	}
}

// propose builds a fresh block from the mempool and broadcasts its HEADER only, 
// inside a signed proposal. Peers fetch the body from us when they accept the header.
func (e *Engine) propose() {
	block, post := e.env.BuildBlock(e.Height, e.env.LastHash(), e.cfg.MaxBlockTxs)
	e.signer.SignHeader(&block.Header)
	p := types.Proposal{Height: e.Height, Round: e.Round, POLRound: -1, Header: block.Header}
	if e.signer.SignProposal(&p) != nil {
		return
	}
	bh := block.Hash()
	e.proposals[e.Round] = p
	e.headers[bh] = block.Header
	e.bodies[bh] = &pending{block: block.Clone(), post: post}
	e.env.Log(logging.EvPropose, e.Height, e.Round, "block", bh.Short(), "txs", strconv.Itoa(len(block.Txs)))
	e.env.Broadcast(&types.ProposalMsg{Proposal: p})
}

// OnProposal handles a proposal (header only) received from peer `from`.
// Header-first: the header is checked, and only then is the body requested
// — from the same peer, which announced it and therefore holds it.
func (e *Engine) OnProposal(from types.NodeID, p *types.Proposal) {
	if p.Height != e.Height {
		return // other heights
	}
	if _, have := e.proposals[p.Round]; have {
		return // one proposal per round is enough (a second one is equivocation)
	}
	if err := VerifyProposal(e.chainID, e.vals, e.env.LastHash(), p); err != nil {
		e.reject("PROPOSAL", p.Height, p.Round, from, err)
		return
	}
	bh := p.BlockHash()
	e.proposals[p.Round] = *p
	e.env.Log(logging.EvHeaderAccept, p.Height, p.Round, "block", bh.Short(), "from", node(from))
	if _, known := e.headers[bh]; known {
		return // same block re-proposed in a later round: body already requested or known
	}
	e.headers[bh] = p.Header
	e.env.Send(from, &types.BodyRequest{Height: p.Height, BlockHash: bh})
}

// OnBody handles the transactions of a block whose header we accepted.
func (e *Engine) OnBody(from types.NodeID, b *types.BodyResponse) {
	hdr, known := e.headers[b.BlockHash]
	if b.Height != e.Height || !known {
		return // not asked for (or another height)
	}
	if e.bodies[b.BlockHash] != nil || e.invalid[b.BlockHash] {
		return // already decided
	}
	post, err := e.env.ExecuteBody(&hdr, b.Txs, e.cfg.MaxBlockTxs)
	if err != nil {
		e.reject("BODY", b.Height, e.Round, from, err)
		if !errors.Is(err, ErrBadTxRoot) {
			e.invalid[b.BlockHash] = true // the block itself is bad, not just this copy
		}
		return
	}
	block := types.Block{Header: hdr, Txs: b.Txs}
	e.bodies[b.BlockHash] = &pending{block: block.Clone(), post: post}
	e.env.Log(logging.EvBodyAccept, b.Height, e.Round, "block", b.BlockHash.Short(), "txs", strconv.Itoa(len(b.Txs)))
	// Now we can serve this body, so we may announce the header to others,
	// in round order (never in map order: it decides the random delays).
	for _, r := range e.sortedRounds() {
		if p := e.proposals[r]; p.BlockHash() == b.BlockHash {
			e.env.Broadcast(&types.ProposalMsg{Proposal: p})
		}
	}
}

func (e *Engine) sortedRounds() []int32 {
	rounds := make([]int32, 0, len(e.proposals))
	for r := range e.proposals {
		rounds = append(rounds, r)
	}
	sort.Slice(rounds, func(i, j int) bool { return rounds[i] < rounds[j] })
	return rounds
}

// Body returns a copy of a checked body of the current height (to answer a
// BodyRequest). Unchecked bodies are never served.
func (e *Engine) Body(height uint64, bh types.Hash) ([]types.Tx, bool) {
	p := e.bodies[bh]
	if height != e.Height || p == nil {
		return nil, false
	}
	return p.block.Clone().Txs, true
}

// HasBody reports whether the body of block bh is known and valid.
func (e *Engine) HasBody(bh types.Hash) bool { return e.bodies[bh] != nil }

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

// reject logs: REJECT type=<what> from=<peer> reason=<token>.
func (e *Engine) reject(what string, h uint64, r int32, from types.NodeID, err error) {
	var txErr *state.TxError
	if errors.As(err, &txErr) {
		err = txErr.Err // log the bare token ("bad_signature"), not "tx 0: ..."
	}
	e.env.Log(logging.EvReject, h, r, "type", what, "from", node(from), "reason", err.Error())
}

// blockLabel prints a block hash in logs, "nil" for a NIL vote.
func blockLabel(h types.Hash) string {
	if h.IsZero() {
		return "nil"
	}
	return h.Short()
}

func node(id types.NodeID) string { return fmt.Sprintf("%02d", id) }
