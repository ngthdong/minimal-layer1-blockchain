package unit

import (
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

func sampleTx(sender string, nonce uint64, value string) types.Tx {
	return types.Tx{Sender: sender, Nonce: nonce, Key: sender + "/message", Value: []byte(value)}
}

func sampleHeader() types.BlockHeader {
	return types.BlockHeader{
		ChainID:    chain,
		Height:     7,
		ParentHash: types.Hash{1},
		TxRoot:     types.Hash{2},
		StateHash:  types.Hash{3},
		Proposer:   4,
	}
}

func TestHashHelpers(t *testing.T) {
	h := types.Hash{0xab, 0xcd, 0xef, 0x01, 0x23}
	if h.Short() != "abcdef01" || len(h.Hex()) != 64 {
		t.Fatalf("Short=%s Hex len=%d", h.Short(), len(h.Hex()))
	}
	if !types.ZeroHash.IsZero() || h.IsZero() {
		t.Fatal("IsZero wrong")
	}
}

// The block hash is what validators vote for: it must not change when the
// proposer's signature changes.
func TestBlockHashIgnoresSignature(t *testing.T) {
	h := sampleHeader()
	before := h.Hash()
	_, priv := crypto.KeyFromSeed(42, "validator/4")
	h.Sig = crypto.Sign(priv, crypto.Context(crypto.DomainHeader, chain), h.SigningBytes())
	if h.Hash() != before {
		t.Fatal("block hash changed after signing")
	}
}

// Every header field must be covered by the hash, or two different blocks
// could share one hash.
func TestBlockHashCoversEveryField(t *testing.T) {
	base := sampleHeader()
	mutations := map[string]func(h *types.BlockHeader){
		"ChainID":    func(h *types.BlockHeader) { h.ChainID = "other" },
		"Height":     func(h *types.BlockHeader) { h.Height++ },
		"ParentHash": func(h *types.BlockHeader) { h.ParentHash[31] ^= 1 },
		"TxRoot":     func(h *types.BlockHeader) { h.TxRoot[31] ^= 1 },
		"StateHash":  func(h *types.BlockHeader) { h.StateHash[31] ^= 1 },
		"Proposer":   func(h *types.BlockHeader) { h.Proposer++ },
	}
	for field, mutate := range mutations {
		h := base
		mutate(&h)
		if h.Hash() == base.Hash() {
			t.Fatalf("changing %s did not change the block hash", field)
		}
	}
}

func TestTxHashIgnoresSignatureButCoversFields(t *testing.T) {
	tx := sampleTx("Alice", 1, "hello")
	before := tx.Hash()
	tx.Sig = []byte{1, 2, 3}
	if tx.Hash() != before {
		t.Fatal("tx hash must not depend on the signature")
	}
	other := sampleTx("Alice", 2, "hello")
	if other.Hash() == before {
		t.Fatal("tx hash must depend on the nonce")
	}
}

// Hashes carry a per-type tag: a Tx and a header whose encodings happened to
// be equal would still hash differently. Here: the tag must be present.
func TestHashesAreTaggedByType(t *testing.T) {
	tx := sampleTx("Alice", 1, "x")
	if tx.Hash() == types.Hash(crypto.Hash(tx.SigningBytes())) {
		t.Fatal("tx hash is an untagged SHA-256 of its bytes")
	}
	h := sampleHeader()
	if h.Hash() == types.Hash(crypto.Hash(h.SigningBytes())) {
		t.Fatal("header hash is an untagged SHA-256 of its bytes")
	}
}

func TestTxRootIsOrderSensitive(t *testing.T) {
	a, b := sampleTx("Alice", 1, "x"), sampleTx("Bob", 1, "y")
	if types.TxRoot([]types.Tx{a, b}) == types.TxRoot([]types.Tx{b, a}) {
		t.Fatal("TxRoot must depend on tx order")
	}
	if types.TxRoot([]types.Tx{a}) == types.TxRoot([]types.Tx{a, a}) {
		t.Fatal("TxRoot must depend on tx count")
	}
}

func TestEmptyTxRootIsStableAndNonZero(t *testing.T) {
	if types.TxRoot(nil) != types.TxRoot([]types.Tx{}) {
		t.Fatal("nil and empty tx lists must have the same root")
	}
	if types.TxRoot(nil).IsZero() {
		t.Fatal("empty root should not be ZeroHash (ZeroHash means 'no hash')")
	}
}

func TestBlockHashEqualsHeaderHash(t *testing.T) {
	b := types.Block{Header: sampleHeader(), Txs: []types.Tx{sampleTx("Alice", 1, "x")}}
	if b.Hash() != b.Header.Hash() {
		t.Fatal("Block.Hash must be the header hash")
	}
}

// The root covers signatures: a body with one signature byte changed no
// longer matches the header.
func TestTxRootCoversSignatures(t *testing.T) {
	a := goodTx("Alice", 1, "a")
	b := a.Clone()
	b.Sig[0] ^= 1
	if types.TxRoot([]types.Tx{a}) == types.TxRoot([]types.Tx{b}) {
		t.Fatal("TxRoot must change when a signature changes")
	}
}
