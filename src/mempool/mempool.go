package mempool

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var (
	ErrDuplicate = errors.New("duplicate_tx") // already pending or already committed
	ErrStale     = errors.New("stale_nonce")  // nonce already used: a replay
)

// NonceFunc returns the last nonce committed for a sender (state.Nonce).
type NonceFunc func(sender string) uint64

type Mempool struct {
	pending   map[types.Hash]types.Tx
	committed map[types.Hash]struct{}
}

func New() *Mempool {
	return &Mempool{
		pending:   make(map[types.Hash]types.Tx),
		committed: make(map[types.Hash]struct{}),
	}
}

func (m *Mempool) Size() int { return len(m.pending) }

func (m *Mempool) Has(h types.Hash) bool { _, ok := m.pending[h]; return ok }

// Add stores a copy of tx. lastNonce is the sender's committed nonce.
// A nonce HIGHER than lastNonce+1 is kept: the missing ones may still arrive
// (the network reorders messages). Only nonce <= lastNonce is a replay.
func (m *Mempool) Add(tx types.Tx, lastNonce uint64) error {
	h := tx.Hash()
	if _, ok := m.committed[h]; ok {
		return ErrDuplicate
	}
	if _, ok := m.pending[h]; ok {
		return ErrDuplicate
	}
	if tx.Nonce <= lastNonce {
		return ErrStale
	}
	m.pending[h] = tx.Clone()
	return nil
}

// Reap returns up to max transactions that can be applied in order on top of
// the current state. The result never depends on map order or Add order:
//
//   - per sender, only nonces lastNonce+1, +2, ... with no gap are taken;
//   - if one sender signed two different txs with the same nonce, the one
//     with the smaller tx hash wins;
//   - senders are served round-robin in name order, so one busy sender
//     cannot fill the whole block.
func (m *Mempool) Reap(max int, nonceOf NonceFunc) []types.Tx {
	// best[sender][nonce] = chosen tx for that nonce
	best := map[string]map[uint64]types.Tx{}
	for h, tx := range m.pending {
		byNonce := best[tx.Sender]
		if byNonce == nil {
			byNonce = map[uint64]types.Tx{}
			best[tx.Sender] = byNonce
		}
		if cur, ok := byNonce[tx.Nonce]; !ok || hashLess(h, cur.Hash()) {
			byNonce[tx.Nonce] = tx
		}
	}

	senders := make([]string, 0, len(best))
	for s := range best {
		senders = append(senders, s)
	}
	sort.Strings(senders)

	// chains[i] = consecutive txs of senders[i], in nonce order
	chains := make([][]types.Tx, len(senders))
	for i, s := range senders {
		for n := nonceOf(s) + 1; ; n++ {
			tx, ok := best[s][n]
			if !ok {
				break
			}
			chains[i] = append(chains[i], tx)
		}
	}

	var out []types.Tx
	for round := 0; len(out) < max; round++ {
		took := false
		for i := range chains {
			if round < len(chains[i]) && len(out) < max {
				out = append(out, chains[i][round].Clone())
				took = true
			}
		}
		if !took {
			break
		}
	}
	return out
}

// RemoveCommitted is called after a block is finalized: its txs leave the
// pool for good, and pending txs whose nonce is now used (replays, or the
// losing side of a same-nonce conflict) are dropped too.
func (m *Mempool) RemoveCommitted(txs []types.Tx, nonceOf NonceFunc) {
	for i := range txs {
		h := txs[i].Hash()
		delete(m.pending, h)
		m.committed[h] = struct{}{}
	}
	for h, tx := range m.pending {
		if tx.Nonce <= nonceOf(tx.Sender) {
			delete(m.pending, h)
		}
	}
}

func hashLess(a, b types.Hash) bool { return bytes.Compare(a[:], b[:]) < 0 }
