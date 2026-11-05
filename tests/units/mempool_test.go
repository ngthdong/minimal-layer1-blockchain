package unit

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/mempool"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// noncesOf maps a fixed sender -> last committed nonce table to a NonceFunc.
func noncesOf(m map[string]uint64) mempool.NonceFunc {
	return func(s string) uint64 { return m[s] }
}

func hashes(txs []types.Tx) []types.Hash {
	out := make([]types.Hash, len(txs))
	for i := range txs {
		out[i] = txs[i].Hash()
	}
	return out
}

func TestAddRejectsDuplicatesAndReplays(t *testing.T) {
	m := mempool.New()
	tx := goodTx("Alice", 1, "a")
	if err := m.Add(tx, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(tx, 0); !errors.Is(err, mempool.ErrDuplicate) {
		t.Fatalf("second Add: %v, want duplicate", err)
	}
	if err := m.Add(goodTx("Bob", 1, "b"), 1); !errors.Is(err, mempool.ErrStale) {
		t.Fatalf("used nonce: %v, want stale", err)
	}
	if err := m.Add(goodTx("Alice", 5, "future"), 0); err != nil {
		t.Fatalf("future nonce must be kept: %v", err)
	}
	if m.Size() != 2 {
		t.Fatalf("size = %d, want 2", m.Size())
	}
}

// The network may deliver Alice's nonce 3 before nonce 2.
func TestReapWaitsForMissingNonce(t *testing.T) {
	m := mempool.New()
	_ = m.Add(goodTx("Alice", 1, "a1"), 0)
	_ = m.Add(goodTx("Alice", 3, "a3"), 0)
	if got := m.Reap(10, noncesOf(nil)); len(got) != 1 || got[0].Nonce != 1 {
		t.Fatalf("reaped %d txs, want only nonce 1", len(got))
	}
	_ = m.Add(goodTx("Alice", 2, "a2"), 0)
	if got := m.Reap(10, noncesOf(nil)); len(got) != 3 {
		t.Fatalf("reaped %d txs, want 3 once the gap is filled", len(got))
	}
}

// Same set of txs, different Add order -> identical Reap result.
func TestReapIsDeterministic(t *testing.T) {
	txs := []types.Tx{
		goodTx("Bob", 1, "b1"), goodTx("Alice", 2, "a2"), goodTx("Alice", 1, "a1"),
		goodTx("Bob", 2, "b2"), goodTx("Alice", 3, "a3"),
	}
	var want []types.Hash
	for rotation := 0; rotation < len(txs); rotation++ {
		m := mempool.New()
		for i := range txs {
			_ = m.Add(txs[(i+rotation)%len(txs)], 0)
		}
		got := hashes(m.Reap(10, noncesOf(nil)))
		if want == nil {
			want = got
		} else if !reflect.DeepEqual(got, want) {
			t.Fatalf("rotation %d gave a different order", rotation)
		}
	}
}

func TestReapRoundRobinAndMax(t *testing.T) {
	m := mempool.New()
	for n := uint64(1); n <= 3; n++ {
		_ = m.Add(goodTx("Alice", n, "a"), 0)
		_ = m.Add(goodTx("Bob", n, "b"), 0)
	}
	got := m.Reap(3, noncesOf(nil))
	order := []string{got[0].Sender, got[1].Sender, got[2].Sender}
	if !reflect.DeepEqual(order, []string{"Alice", "Bob", "Alice"}) {
		t.Fatalf("order = %v, want Alice, Bob, Alice", order)
	}
	if m.Reap(0, noncesOf(nil)) != nil {
		t.Fatal("max 0 must return nothing")
	}
}

// A reaped batch must always be a valid block for the current state.
func TestReapedTxsApplyCleanly(t *testing.T) {
	s, reg, m := state.New(), registry(), mempool.New()
	for _, tx := range []types.Tx{goodTx("Bob", 2, "b2"), goodTx("Alice", 1, "a1"),
		goodTx("Bob", 1, "b1"), goodTx("Alice", 3, "gap")} {
		_ = m.Add(tx, 0)
	}
	batch := m.Reap(10, s.Nonce)
	if err := s.ApplyBlock(chain, reg, batch); err != nil {
		t.Fatalf("reaped batch is not applicable: %v", err)
	}
	m.RemoveCommitted(batch, s.Nonce)
	if m.Size() != 1 || len(m.Reap(10, s.Nonce)) != 0 {
		t.Fatal("only Alice's nonce-3 tx should stay pending, and it is not ready")
	}
	if err := m.Add(batch[0], 99); !errors.Is(err, mempool.ErrDuplicate) {
		t.Fatalf("committed tx re-added: %v", err)
	}
}

// Alice signs two different txs with nonce 1: exactly one is chosen,
// always the same one, and the other is dropped once nonce 1 is used.
func TestSameNonceConflict(t *testing.T) {
	x, y := goodTx("Alice", 1, "x"), goodTx("Alice", 1, "y")
	m1, m2 := mempool.New(), mempool.New()
	_ = m1.Add(x, 0)
	_ = m1.Add(y, 0)
	_ = m2.Add(y, 0)
	_ = m2.Add(x, 0)
	a, b := m1.Reap(10, noncesOf(nil)), m2.Reap(10, noncesOf(nil))
	if len(a) != 1 || len(b) != 1 || a[0].Hash() != b[0].Hash() {
		t.Fatal("conflict not resolved to one deterministic tx")
	}
	m1.RemoveCommitted(a, noncesOf(map[string]uint64{"Alice": 1}))
	if m1.Size() != 0 {
		t.Fatal("losing same-nonce tx must be dropped after commit")
	}
}

func TestReapReturnsCopies(t *testing.T) {
	m := mempool.New()
	_ = m.Add(goodTx("Alice", 1, "hello"), 0)
	m.Reap(1, noncesOf(nil))[0].Value[0] = 'J'
	if v := m.Reap(1, noncesOf(nil))[0].Value; string(v) != "hello" {
		t.Fatalf("pool changed through a reaped tx: %q", v)
	}
}
