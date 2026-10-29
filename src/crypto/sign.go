package crypto

import (
	"crypto/ed25519"

	"github.com/ngthdong/minimal-layer1-blockchain/encoding"
)

const (
	DomainTx       = "TX"
	DomainHeader   = "HEADER"
	DomainProposal = "PROPOSAL"
	DomainVote     = "VOTE"
)

func Context(kind, chainID string) string {
	return kind + ":" + chainID
}

func signingInput(context string, payload []byte) []byte {
	return encoding.NewWriter().String(context).Bytes(payload).Out()
}

func Sign(priv PrivateKey, context string, payload []byte) []byte {
	return ed25519.Sign(priv, signingInput(context, payload))
}

func Verify(pub PublicKey, context string, payload []byte, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, signingInput(context, payload), sig)
}
