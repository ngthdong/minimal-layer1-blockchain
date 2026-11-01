package types

import (
	"encoding/hex"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/encoding"
)

type Hash [crypto.HashSize]byte

var ZeroHash Hash

func (h Hash) IsZero() bool {
	return h == ZeroHash
}

func (h Hash) Hex() string {
	return hex.EncodeToString(h[:])
}

func (h Hash) Short() string {
	return hex.EncodeToString(h[:4])
}

func taggedHash(tag string, data []byte) Hash {
	return Hash(crypto.Hash(encoding.NewWriter().String(tag).Bytes(data).Out()))
}

type NodeID uint32
