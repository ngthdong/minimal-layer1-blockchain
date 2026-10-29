package unit

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
)

// Known SHA-256 test vectors (FIPS 180-2).
func TestHashKnownVectors(t *testing.T) {
	cases := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for in, want := range cases {
		h := crypto.Hash([]byte(in))
		if got := hex.EncodeToString(h[:]); got != want {
			t.Fatalf("Hash(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestKeysAreDeterministic(t *testing.T) {
	p1, s1 := crypto.KeyFromSeed(42, "validator/0")
	p2, s2 := crypto.KeyFromSeed(42, "validator/0")
	if !bytes.Equal(p1, p2) || !bytes.Equal(s1, s2) {
		t.Fatal("same seed and label must give the same key pair")
	}
}

func TestKeysDifferBySeedAndLabel(t *testing.T) {
	base, _ := crypto.KeyFromSeed(42, "validator/0")
	otherLabel, _ := crypto.KeyFromSeed(42, "validator/1")
	otherSeed, _ := crypto.KeyFromSeed(43, "validator/0")
	if bytes.Equal(base, otherLabel) {
		t.Fatal("different labels gave the same key")
	}
	if bytes.Equal(base, otherSeed) {
		t.Fatal("different seeds gave the same key")
	}
}

// Pins the derivation. If this fails, every key, signature and log changed:
// only update the expected value on purpose.
func TestKeyDerivationIsPinned(t *testing.T) {
	pub, _ := crypto.KeyFromSeed(42, "validator/0")
	const want = "a1c6383db7b44411fff45c097008f748e4346efdfc0a1e438136f71132af8f4d"
	if got := hex.EncodeToString(pub); got != want {
		t.Fatalf("validator/0 public key = %s, want %s", got, want)
	}
}

func TestDerivedKeysAreUsableEd25519Keys(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(7, "account/Alice")
	if len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("key sizes %d/%d", len(pub), len(priv))
	}
	msg := []byte("hello")
	if !ed25519.Verify(pub, msg, ed25519.Sign(priv, msg)) {
		t.Fatal("derived key pair cannot sign and verify")
	}
}
