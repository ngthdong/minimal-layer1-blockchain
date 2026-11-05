package consensus

import "github.com/ngthdong/minimal-layer1-blockchain/types"

// maxBuffered bounds the messages kept for the next height, so a peer cannot
// fill our memory with future votes (it can sign as many rounds as it likes).
const maxBuffered = 4096

// buffered is a proposal or a vote that arrived for the NEXT height while we
// were still finishing the current one. Nodes commit at slightly different
// times, so the fastest ones always start height h+1 before the slowest:
// without this buffer the slow nodes lose the proposal and the first votes
// of h+1 and may never catch up.
type buffered struct {
	from types.NodeID
	prop *types.Proposal
	vote *types.Vote
}

// buffer keeps msg if it is for the next height. Messages further ahead are
// dropped: a node that is two heights behind must catch up by block sync,
// not by replaying a whole height of messages.
func (e *Engine) buffer(h uint64, b buffered) {
	if h != e.Height+1 || len(e.future[h]) >= maxBuffered {
		return
	}
	e.future[h] = append(e.future[h], b)
}

// replay feeds the messages buffered for the current height back into the
// engine, in arrival order. Any of them may complete this height and start
// the next one; then the rest belong to a finished height and are dropped.
func (e *Engine) replay() {
	h := e.Height
	pending := e.future[h]
	for k := range e.future {
		if k <= h {
			delete(e.future, k)
		}
	}
	for _, b := range pending {
		if e.Height != h {
			return
		}
		if b.prop != nil {
			e.OnProposal(b.from, b.prop)
		} else {
			e.OnVote(b.from, b.vote)
		}
	}
}
