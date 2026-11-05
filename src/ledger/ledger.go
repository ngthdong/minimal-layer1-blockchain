package ledger

import (
	"errors"

	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var (
	// ErrConflict means safety is broken: two different blocks finalized at
	// the same height. It must never happen; tests treat it as fatal.
	ErrConflict = errors.New("conflicting_block")
	// ErrAlreadyFinal: the same block again (e.g. from block sync). Harmless.
	ErrAlreadyFinal = errors.New("already_final")
	ErrGap          = errors.New("height_gap")    // skips a height
	ErrBadParent    = errors.New("bad_parent")    // does not extend our chain
	ErrCertMismatch = errors.New("cert_mismatch") // cert is for another block
)

// Entry is one finalized block and its commit certificate.
type Entry struct {
	Block types.Block
	Cert  types.CommitCert
}

type Ledger struct {
	entries []Entry // entries[i] is height i+1
}

func New() *Ledger { return &Ledger{} }

func (l *Ledger) Height() uint64 { return uint64(len(l.entries)) }

func (l *Ledger) LastHash() types.Hash {
	if len(l.entries) == 0 {
		return types.ZeroHash
	}
	return l.entries[len(l.entries)-1].Block.Hash()
}

func (l *Ledger) BlockHash(h uint64) (types.Hash, bool) {
	if h == 0 || h > l.Height() {
		return types.ZeroHash, false
	}
	return l.entries[h-1].Block.Hash(), true
}

func (l *Ledger) Get(h uint64) (Entry, bool) {
	if h == 0 || h > l.Height() {
		return Entry{}, false
	}
	e := l.entries[h-1]
	return Entry{Block: e.Block.Clone(), Cert: e.Cert.Clone()}, true
}

func (l *Ledger) Append(e Entry) error {
	h, bh := e.Block.Header.Height, e.Block.Hash()
	if existing, ok := l.BlockHash(h); ok {
		if existing == bh {
			return ErrAlreadyFinal
		}
		return ErrConflict
	}
	if h != l.Height()+1 {
		return ErrGap
	}
	if e.Block.Header.ParentHash != l.LastHash() {
		return ErrBadParent
	}
	if e.Cert.Height != h || e.Cert.BlockHash != bh {
		return ErrCertMismatch
	}
	l.entries = append(l.entries, Entry{Block: e.Block.Clone(), Cert: e.Cert.Clone()})
	return nil
}
