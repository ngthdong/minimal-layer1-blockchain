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
	// Commit finalizes block: appends it with its certificate to the ledger,
	// installs post as the node's state and cleans the mempool.
	Commit(block types.Block, cert types.CommitCert, post *state.State) error
	// ScheduleTimeout calls engine.OnTimeout(t) after afterMs of virtual time.
	ScheduleTimeout(afterMs uint64, t Timeout)
	// RequestSync asks peer for the finalized blocks we are missing; the
	// node calls AcceptSyncedHeader for each header that arrives, then
	// CommitSynced once its body came.
	RequestSync(peer types.NodeID)
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

	// Locking (rules 4 and 5). -1 = none.
	LockedRound int32
	LockedHash  types.Hash
	ValidRound  int32
	ValidHash   types.Hash

	Votes *VoteSet // votes of the current height

	proposals map[int32]types.Proposal         // accepted proposal per round
	pols      map[int32][]types.Vote           // verified POL (2f+1 prevotes) per round
	headers   map[types.Hash]types.BlockHeader // accepted headers, waiting for or with a body
	bodies    map[types.Hash]*pending          // blocks whose body executed correctly
	invalid   map[types.Hash]bool              // blocks whose body is provably wrong

	future map[uint64][]buffered // messages for the next height (buffer.go)
	timers map[Timeout]bool      // timers already scheduled in this height
}

func NewEngine(id types.NodeID, chainID string, vals *ValidatorSet, signer *VoteSigner, cfg Config, env Env) *Engine {
	return &Engine{ID: id, chainID: chainID, vals: vals, cfg: cfg, env: env, signer: signer,
		future: make(map[uint64][]buffered)}
}

// StartHeight resets everything that belongs to one height and enters round 0.
// Locks never survive a height: they only protect the block of THIS height.
func (e *Engine) StartHeight(h uint64) {
	e.Height = h
	e.Votes = NewVoteSet()
	e.LockedRound, e.LockedHash = -1, types.ZeroHash
	e.ValidRound, e.ValidHash = -1, types.ZeroHash
	e.proposals = make(map[int32]types.Proposal)
	e.pols = make(map[int32][]types.Vote)
	e.headers = make(map[types.Hash]types.BlockHeader)
	e.bodies = make(map[types.Hash]*pending)
	e.invalid = make(map[types.Hash]bool)
	e.timers = make(map[Timeout]bool)
	e.signer.Prune(h)
	e.enterNewRound(0)
	e.replay() // proposal and votes of h that arrived while we were still at h-1
}

// enterNewRound moves to round r of the current height; the proposer of the
// round builds and announces a block. Locks are KEPT: they only end with the
// height. The propose timer starts now. kv is appended to the log line.
func (e *Engine) enterNewRound(r int32, kv ...string) {
	e.Round, e.Step = r, StepPropose
	proposer := e.vals.Proposer(e.Height, r)
	e.env.Log(logging.EvNewRound, e.Height, r, append([]string{"proposer", node(proposer)}, kv...)...)
	e.schedule(StepPropose, e.cfg.TimeoutProposeMs)
	if proposer == e.ID {
		e.propose()
	}
}

// propose announces the block of this round. If an earlier round of this
// height produced 2f+1 prevotes for a block (ValidHash), that block is
// proposed AGAIN, unchanged, with POLRound = ValidRound: honest nodes may be
// locked on it, and a fresh block would only collect their NIL prevotes.
// Otherwise a fresh block is built from the mempool. Only the HEADER is
// broadcast, inside a signed proposal; peers fetch the body from us.
func (e *Engine) propose() {
	p := types.Proposal{Height: e.Height, Round: e.Round, POLRound: -1}
	if e.ValidRound >= 0 && e.bodies[e.ValidHash] != nil {
		p.POLRound, p.Header = e.ValidRound, e.headers[e.ValidHash] // builder's signature kept
		p.POL = e.polFor(e.ValidRound, e.ValidHash)
	} else {
		block, post := e.env.BuildBlock(e.Height, e.env.LastHash(), e.cfg.MaxBlockTxs)
		e.signer.SignHeader(&block.Header)
		p.Header = block.Header
		e.headers[block.Hash()] = block.Header
		e.bodies[block.Hash()] = &pending{block: block.Clone(), post: post}
	}
	if e.signer.SignProposal(&p) != nil {
		return
	}
	bh := p.BlockHash()
	e.proposals[e.Round] = p
	e.env.Log(logging.EvPropose, e.Height, e.Round, "block", bh.Short(),
		"txs", strconv.Itoa(len(e.bodies[bh].block.Txs)), "pol", strconv.Itoa(int(p.POLRound)))
	e.env.Broadcast(&types.ProposalMsg{Proposal: p})
	e.progress()
}

// OnProposal handles a proposal (header only) received from peer `from`.
// Header-first: the header is checked, and only then is the body requested
// — from the same peer, which announced it and therefore holds it.
func (e *Engine) OnProposal(from types.NodeID, p *types.Proposal) {
	if p.Height > e.Height {
		e.noticeAhead(from, p.Height)
		cp := p.Clone()
		e.buffer(p.Height, buffered{from: from, prop: &cp}) // checked when we get there
		return
	}
	if p.Height < e.Height {
		return // a finished height
	}
	if _, have := e.proposals[p.Round]; have {
		return // one proposal per round is enough (a second one is equivocation)
	}
	if err := VerifyProposal(e.chainID, e.vals, e.env.LastHash(), p); err != nil {
		e.reject("PROPOSAL", p.Height, p.Round, from, err)
		return
	}
	bh := p.BlockHash()
	e.proposals[p.Round] = p.Clone()
	if len(p.POL) > 0 { // verified above: 2f+1 prevotes for bh in POLRound
		e.pols[p.POLRound] = p.Clone().POL
	}
	e.env.Log(logging.EvHeaderAccept, p.Height, p.Round, "block", bh.Short(), "from", node(from))
	if _, known := e.headers[bh]; !known {
		e.headers[bh] = p.Header
	}
	switch {
	case e.bodies[bh] != nil:
		// The same block proposed again in a later round: we already hold
		// (and can serve) its body, so we relay the proposal at once.
		e.env.Broadcast(&types.ProposalMsg{Proposal: *p})
	case !e.invalid[bh]:
		// New block, or a re-proposal whose body never reached us: ask the
		// peer that just announced it (it holds the body).
		e.env.Send(from, &types.BodyRequest{Height: p.Height, BlockHash: bh})
	}
	e.progress()
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
			e.progress()                  // → prevote NIL
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
	e.progress()
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
