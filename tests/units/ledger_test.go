package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/ledger"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// entryAt builds a block at height h on top of parent, with a matching cert.
// salt makes two different blocks at the same height.
func entryAt(h uint64, parent types.Hash, salt byte) ledger.Entry {
	b := types.Block{
		Header: types.BlockHeader{ChainID: chain, Height: h, ParentHash: parent, StateHash: types.Hash{salt}},
		Txs:    []types.Tx{goodTx("Alice", h, "v")},
	}
	return ledger.Entry{Block: b, Cert: types.CommitCert{Height: h, BlockHash: b.Hash()}}
}

// chainOf appends n blocks and returns the ledger.
func chainOf(t *testing.T, n uint64) *ledger.Ledger {
	t.Helper()
	l := ledger.New()
	for h := uint64(1); h <= n; h++ {
		if err := l.Append(entryAt(h, l.LastHash(), 0)); err != nil {
			t.Fatalf("append height %d: %v", h, err)
		}
	}
	return l
}

func TestLedgerGenesis(t *testing.T) {
	l := ledger.New()
	if l.Height() != 0 || !l.LastHash().IsZero() {
		t.Fatal("empty ledger must have height 0 and ZeroHash parent")
	}
	if _, ok := l.Get(0); ok {
		t.Fatal("height 0 is not a block")
	}
}

func TestLedgerAppendsChain(t *testing.T) {
	l := chainOf(t, 3)
	if l.Height() != 3 {
		t.Fatalf("height = %d, want 3", l.Height())
	}
	e2, _ := l.Get(2)
	h1, _ := l.BlockHash(1)
	if e2.Block.Header.ParentHash != h1 {
		t.Fatal("block 2 does not point to block 1")
	}
	if h3, _ := l.BlockHash(3); h3 != l.LastHash() {
		t.Fatal("LastHash is not the tip")
	}
}

func TestLedgerRejects(t *testing.T) {
	l := chainOf(t, 2)
	tip := l.LastHash()
	h1, _ := l.BlockHash(1)
	badCert := entryAt(3, tip, 0)
	badCert.Cert.BlockHash[0] ^= 1
	sameAgain, _ := l.Get(2)

	cases := []struct {
		name  string
		entry ledger.Entry
		want  error
	}{
		{"different block at height 2", entryAt(2, h1, 9), ledger.ErrConflict},
		{"same block at height 2", sameAgain, ledger.ErrAlreadyFinal},
		{"skips height 3", entryAt(4, tip, 0), ledger.ErrGap},
		{"wrong parent", entryAt(3, types.Hash{7}, 0), ledger.ErrBadParent},
		{"cert for another block", badCert, ledger.ErrCertMismatch},
	}
	for _, c := range cases {
		if err := l.Append(c.entry); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if l.Height() != 2 || l.LastHash() != tip {
			t.Fatalf("%s: rejected entry changed the ledger", c.name)
		}
	}
}

func TestLedgerIsIsolatedFromCallers(t *testing.T) {
	l := ledger.New()
	e := entryAt(1, types.ZeroHash, 0)
	if err := l.Append(e); err != nil {
		t.Fatal(err)
	}
	want := l.LastHash()
	e.Block.Txs[0].Value[0] = 'X' // caller edits what it appended
	got, _ := l.Get(1)
	got.Block.Header.Sig = []byte{1} // caller edits what Get returned
	got.Block.Txs[0].Value[0] = 'Y'
	if again, _ := l.Get(1); string(again.Block.Txs[0].Value) != "v" || l.LastHash() != want {
		t.Fatal("ledger changed through a slice it received or gave out")
	}
}
