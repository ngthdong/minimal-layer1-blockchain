package unit

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
)

const chain = "ml1-chain"

var allDomains = []string{crypto.DomainTx, crypto.DomainHeader, crypto.DomainProposal, crypto.DomainVote}

func TestContextFormat(t *testing.T) {
	if got := crypto.Context(crypto.DomainVote, chain); got != "VOTE:ml1-chain" {
		t.Fatalf("Context = %q", got)
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(1, "account/Alice")
	ctx := crypto.Context(crypto.DomainTx, chain)
	msg := []byte("hello")
	sig := crypto.Sign(priv, ctx, msg)

	if !crypto.Verify(pub, ctx, msg, sig) {
		t.Fatal("valid signature rejected")
	}
	if crypto.Verify(pub, ctx, []byte("hellO"), sig) {
		t.Fatal("tampered payload accepted")
	}
	bad := append([]byte(nil), sig...)
	bad[0] ^= 0xff
	if crypto.Verify(pub, ctx, msg, bad) {
		t.Fatal("tampered signature accepted")
	}
	otherPub, _ := crypto.KeyFromSeed(1, "account/Bob")
	if crypto.Verify(otherPub, ctx, msg, sig) {
		t.Fatal("signature accepted under someone else's key")
	}
}

// A signature made in one domain must verify in that domain only (T3).
func TestDomainSeparation(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(1, "validator/0")
	payload := []byte("same-bytes")
	for _, signDom := range allDomains {
		sig := crypto.Sign(priv, crypto.Context(signDom, chain), payload)
		for _, checkDom := range allDomains {
			ok := crypto.Verify(pub, crypto.Context(checkDom, chain), payload, sig)
			if ok != (signDom == checkDom) {
				t.Fatalf("signed under %s, verified under %s = %v", signDom, checkDom, ok)
			}
		}
	}
}

func TestChainSeparation(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(1, "validator/0")
	sig := crypto.Sign(priv, crypto.Context(crypto.DomainVote, chain), []byte("v"))
	if crypto.Verify(pub, crypto.Context(crypto.DomainVote, "other-chain"), []byte("v"), sig) {
		t.Fatal("signature accepted on another chain")
	}
}

// Moving bytes between context and payload must not keep a signature valid.
func TestContextPayloadBoundary(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(1, "validator/0")
	sig := crypto.Sign(priv, "VOTE:ml1-chain", []byte("X"))
	if crypto.Verify(pub, "VOTE:ml1-chai", []byte("nX"), sig) {
		t.Fatal("context/payload boundary is ambiguous")
	}
}

func TestSignaturesAreDeterministic(t *testing.T) {
	_, priv := crypto.KeyFromSeed(42, "validator/0")
	ctx := crypto.Context(crypto.DomainVote, chain)
	a := crypto.Sign(priv, ctx, []byte("payload"))
	b := crypto.Sign(priv, ctx, []byte("payload"))
	if !bytes.Equal(a, b) {
		t.Fatal("same input gave different signatures")
	}
}

// Pins the signing-input layout. If this fails, every signature and log changed.
func TestSigningInputIsPinned(t *testing.T) {
	_, priv := crypto.KeyFromSeed(42, "validator/0")
	sig := crypto.Sign(priv, crypto.Context(crypto.DomainVote, chain), []byte("payload"))
	const want = "40e6ead999cd43ce93e0b86b7a4e392e4706a109529e293814674ef6f882ff36cc50098c00707654becf15a41eb8e5062bb7f8aa7a3ed2ae345ec3170fd14801"
	if got := hex.EncodeToString(sig); got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

// Byzantine nodes can send garbage: Verify must return false, never panic.
func TestVerifyRejectsMalformedInput(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(1, "validator/0")
	ctx := crypto.Context(crypto.DomainVote, chain)
	sig := crypto.Sign(priv, ctx, []byte("v"))
	cases := map[string]struct {
		pub crypto.PublicKey
		sig []byte
	}{
		"nil key":         {nil, sig},
		"short key":       {pub[:31], sig},
		"nil signature":   {pub, nil},
		"short signature": {pub, sig[:63]},
		"long signature":  {pub, append(append([]byte(nil), sig...), 0)},
	}
	for name, c := range cases {
		if crypto.Verify(c.pub, ctx, []byte("v"), c.sig) {
			t.Fatalf("%s: accepted", name)
		}
	}
}
