package consensus

import (
	"errors"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ValidatorSet is the fixed, ordered list of validators for a whole run.
// The index of a key is that validator's NodeID.
type ValidatorSet struct {
	keys []crypto.PublicKey
}

// NewValidatorSet copies keys. n must have the form 3f+1 with f >= 1, so
// that Quorum() really gives the BFT guarantees described below.
func NewValidatorSet(keys []crypto.PublicKey) (*ValidatorSet, error) {
	n := len(keys)
	if n < 4 || (n-1)%3 != 0 {
		return nil, errors.New("consensus: validator count must be 3f+1 with f >= 1")
	}
	return &ValidatorSet{keys: append([]crypto.PublicKey(nil), keys...)}, nil
}

// N is the number of validators.
func (vs *ValidatorSet) N() int { return len(vs.keys) }

// F is the largest number of Byzantine validators tolerated: n = 3f+1.
func (vs *ValidatorSet) F() int { return (vs.N() - 1) / 3 }

// Quorum is 2f+1 votes. Any two quorums share at least f+1 validators, so at
// least one HONEST validator is in both. An honest validator never votes for
// two different blocks in the same step, hence two conflicting blocks can
// never both gather a quorum: that is the core of safety.
func (vs *ValidatorSet) Quorum() int { return 2*vs.F() + 1 }

// Proposer returns the proposer of (height, round): round-robin over
// height + round. Every node computes it locally and gets the same answer.
// Moving to the next round moves to the next validator, so a silent or
// crashed proposer only stalls one round.
func (vs *ValidatorSet) Proposer(height uint64, round int32) types.NodeID {
	return types.NodeID((height + uint64(round)) % uint64(vs.N()))
}

// Key returns the public key of validator id, or false if id is not a validator.
func (vs *ValidatorSet) Key(id types.NodeID) (crypto.PublicKey, bool) {
	if int(id) >= vs.N() {
		return nil, false
	}
	return vs.keys[id], true
}
