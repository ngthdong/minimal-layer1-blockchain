package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var accounts = []string{"Alice", "Bob"}

func registry() state.Registry {
	reg := state.Registry{}
	for _, a := range accounts {
		reg[a], _ = crypto.KeyFromSeed(1, "account/"+a)
	}
	return reg
}

// signedTx builds a tx signed by sender's key under the given domain.
func signedTx(sender string, nonce uint64, key, value, domain string) types.Tx {
	_, priv := crypto.KeyFromSeed(1, "account/"+sender)
	tx := types.Tx{Sender: sender, Nonce: nonce, Key: key, Value: []byte(value)}
	tx.Sig = crypto.Sign(priv, crypto.Context(domain, chain), tx.SigningBytes())
	return tx
}

func goodTx(sender string, nonce uint64, value string) types.Tx {
	return signedTx(sender, nonce, sender+"/message", value, crypto.DomainTx)
}

func TestApplyValidTx(t *testing.T) {
	s, reg := state.New(), registry()
	tx := goodTx("Alice", 1, "hello")
	if err := s.ApplyTx(chain, reg, &tx); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get("Alice/message"); string(v) != "hello" {
		t.Fatalf("value = %q", v)
	}
	if s.Nonce("Alice") != 1 || s.Nonce("Bob") != 0 {
		t.Fatalf("nonces = %d/%d, want 1/0", s.Nonce("Alice"), s.Nonce("Bob"))
	}
}

// Each bad tx must be rejected with its exact reason and change nothing.
func TestRejectedTxReasons(t *testing.T) {
	tampered := goodTx("Alice", 1, "hello")
	tampered.Value = []byte("HELLO")
	cases := []struct {
		name string
		tx   types.Tx
		want error
	}{
		{"unknown sender", signedTx("Mallory", 1, "Mallory/x", "v", crypto.DomainTx), state.ErrUnknownSender},
		{"tampered value", tampered, state.ErrBadSignature},
		{"vote domain", signedTx("Alice", 1, "Alice/message", "v", crypto.DomainVote), state.ErrBadSignature},
		{"foreign key", signedTx("Alice", 1, "Bob/message", "v", crypto.DomainTx), state.ErrNotOwner},
		{"nonce key", signedTx("Alice", 1, "sys/nonce/Alice", "v", crypto.DomainTx), state.ErrNotOwner},
		{"no owner", signedTx("Alice", 1, "message", "v", crypto.DomainTx), state.ErrNotOwner},
		{"nonce gap", goodTx("Alice", 3, "v"), state.ErrBadNonce},
		{"nonce zero", goodTx("Alice", 0, "v"), state.ErrBadNonce},
	}
	for _, c := range cases {
		s := state.New()
		before := s.Hash()
		if err := s.ApplyTx(chain, registry(), &c.tx); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if s.Hash() != before {
			t.Fatalf("%s: rejected tx changed the state", c.name)
		}
	}
}

func TestReplayIsRejected(t *testing.T) {
	s, reg := state.New(), registry()
	tx := goodTx("Alice", 1, "hello")
	if err := s.ApplyTx(chain, reg, &tx); err != nil {
		t.Fatal(err)
	}
	after := s.Hash()
	if err := s.ApplyTx(chain, reg, &tx); !errors.Is(err, state.ErrBadNonce) {
		t.Fatalf("replay err = %v, want bad_nonce", err)
	}
	if s.Hash() != after {
		t.Fatal("replay changed the state")
	}
}

func TestApplyBlockIsAtomic(t *testing.T) {
	s, reg := state.New(), registry()
	ok := []types.Tx{goodTx("Alice", 1, "a"), goodTx("Bob", 1, "b"), goodTx("Alice", 2, "c")}
	if err := s.ApplyBlock(chain, reg, ok); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get("Alice/message"); string(v) != "c" || s.Nonce("Alice") != 2 {
		t.Fatalf("Alice/message=%q nonce=%d, want c/2", v, s.Nonce("Alice"))
	}

	before := s.Hash()
	bad := []types.Tx{goodTx("Bob", 2, "fine"), goodTx("Alice", 2, "replay")}
	err := s.ApplyBlock(chain, reg, bad)
	var txErr *state.TxError
	if !errors.As(err, &txErr) || txErr.Index != 1 || !errors.Is(err, state.ErrBadNonce) {
		t.Fatalf("err = %v, want tx 1: bad_nonce", err)
	}
	if s.Hash() != before {
		t.Fatal("failed block partially applied (Bob's tx must be undone)")
	}
}

// The nonce is part of the committed state.
func TestStateHashCoversNonces(t *testing.T) {
	reg := registry()
	a, b := state.New(), state.New()
	tx1, tx2 := goodTx("Alice", 1, "x"), goodTx("Alice", 2, "x")
	_ = a.ApplyTx(chain, reg, &tx1)
	_ = b.ApplyTx(chain, reg, &tx1)
	_ = b.ApplyTx(chain, reg, &tx2) // same value again, only the nonce moves
	if a.Hash() == b.Hash() {
		t.Fatal("state hash ignores the nonce")
	}
}

func TestStoredValueIsIsolatedFromTx(t *testing.T) {
	s := state.New()
	tx := goodTx("Alice", 1, "hello")
	_ = s.ApplyTx(chain, registry(), &tx)
	tx.Value[0] = 'J'
	if v, _ := s.Get("Alice/message"); string(v) != "hello" {
		t.Fatalf("editing the tx changed the state: %q", v)
	}
}

func TestApplyOnCloneLeavesOriginal(t *testing.T) {
	s := state.New()
	c := s.Clone()
	tx := goodTx("Alice", 1, "hello")
	_ = c.ApplyTx(chain, registry(), &tx)
	if s.Len() != 0 || c.Len() != 2 {
		t.Fatalf("len original=%d clone=%d, want 0/2", s.Len(), c.Len())
	}
}
