package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/ngthdong/minimal-layer1-blockchain/encoding"
)

const HashSize = sha256.Size // 32 bytes

type (
	PublicKey  = ed25519.PublicKey
	PrivateKey = ed25519.PrivateKey
)

// keyDerivationTag versions the key derivation. Changing it changes every key,
// every signature and therefore every log line of every run.
const keyDerivationTag = "ml1b/key/v1"

func Hash(data []byte) [HashSize]byte {
	return sha256.Sum256(data)
}

func KeyFromSeed(seed uint64, label string) (PublicKey, PrivateKey) {
	material := Hash(encoding.NewWriter().
		String(keyDerivationTag).
		U64(seed).
		String(label).
		Out())
	priv := ed25519.NewKeyFromSeed(material[:]) // needs exactly 32 bytes
	return priv.Public().(ed25519.PublicKey), priv
}
