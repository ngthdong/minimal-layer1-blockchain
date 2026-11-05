package state

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var (
	ErrUnknownSender = errors.New("unknown_sender")
	ErrBadSignature  = errors.New("bad_signature")
	ErrNotOwner      = errors.New("key_not_owned_by_sender")
	ErrBadNonce      = errors.New("bad_nonce")
)

// NoncePrefix is where each sender's last used nonce is stored:
// "sys/nonce/<Sender>". The owner "sys" is not a valid account, so
// no transaction can write these keys, yet they are part of the state hash.
const NoncePrefix = "sys/nonce/"

// Registry maps account names to public keys. It is genesis data: fixed for
// the whole run and identical on every node.
type Registry map[string]crypto.PublicKey

func (s *State) Nonce(sender string) uint64 {
	v, ok := s.kv[NoncePrefix+sender]
	if !ok || len(v) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(v)
}

func (s *State) VerifyTx(chainID string, reg Registry, tx *types.Tx) error {
	if err := CheckTx(chainID, reg, tx); err != nil {
		return err
	}
	if tx.Nonce != s.Nonce(tx.Sender)+1 {
		return ErrBadNonce
	}
	return nil
}

// CheckTx runs checks 1–3 of VerifyTx: everything that does not depend on the
// current state. Nodes use it when a tx arrives from the network, because a
// tx whose nonce is still in the future is valid and must be kept, not
// rejected (the mempool waits for the missing nonces).
func CheckTx(chainID string, reg Registry, tx *types.Tx) error {
	pub, ok := reg[tx.Sender]
	if !ok {
		return ErrUnknownSender
	}
	if !crypto.Verify(pub, crypto.Context(crypto.DomainTx, chainID), tx.SigningBytes(), tx.Sig) {
		return ErrBadSignature
	}
	if Owner(tx.Key) != tx.Sender {
		return ErrNotOwner
	}
	return nil
}

func (s *State) ApplyTx(chainID string, reg Registry, tx *types.Tx) error {
	if err := s.VerifyTx(chainID, reg, tx); err != nil {
		return err
	}
	s.set(tx.Key, tx.Value)
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], tx.Nonce)
	s.set(NoncePrefix+tx.Sender, n[:])
	return nil
}

type TxError struct {
	Index int
	Err   error
}

func (e *TxError) Error() string { return fmt.Sprintf("tx %d: %v", e.Index, e.Err) }
func (e *TxError) Unwrap() error { return e.Err }

func (s *State) ApplyBlock(chainID string, reg Registry, txs []types.Tx) error {
	next := s.Clone()
	for i := range txs {
		if err := next.ApplyTx(chainID, reg, &txs[i]); err != nil {
			return &TxError{Index: i, Err: err}
		}
	}
	s.kv = next.kv
	return nil
}

func (s *State) set(key string, value []byte) { s.kv[key] = cloneValue(value) }
