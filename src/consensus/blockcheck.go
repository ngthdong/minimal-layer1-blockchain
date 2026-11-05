package consensus

import (
	"errors"

	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var (
	ErrCertNil      = errors.New("cert_nil_block") // a certificate for "no block"
	ErrCertBadVote  = errors.New("cert_bad_vote")  // a vote that does not belong in it
	ErrCertNoQuorum = errors.New("cert_no_quorum") // fewer than 2f+1 validators
	ErrBadTxRoot    = errors.New("bad_tx_root")
	ErrTooManyTxs   = errors.New("too_many_txs")
	ErrBadStateHash = errors.New("bad_state_hash")
)

// VerifyCommitCert checks that cert proves its block is final. Every vote must
// be a correctly signed PRECOMMIT for exactly (cert.Height, cert.Round,
// cert.BlockHash), validators strictly increasing (sorted, no duplicates),
// and there must be at least 2f+1 of them.
//
// A node that receives a block through sync trusts nothing but
// this check: not the peer that sent it, not the block's own fields.
func VerifyCommitCert(chainID string, vs *ValidatorSet, cert *types.CommitCert) error {
	if cert.BlockHash.IsZero() {
		return ErrCertNil
	}
	for i := range cert.Precommits {
		v := &cert.Precommits[i]
		if v.Type != types.Precommit || v.Height != cert.Height || v.Round != cert.Round ||
			v.BlockHash != cert.BlockHash {
			return ErrCertBadVote
		}
		if i > 0 && v.Validator <= cert.Precommits[i-1].Validator {
			return ErrCertBadVote // unsorted or the same validator twice
		}
		if VerifyVote(chainID, vs, v) != nil {
			return ErrCertBadVote
		}
	}
	if len(cert.Precommits) < vs.Quorum() {
		return ErrCertNoQuorum
	}
	return nil
}

func ExecuteBody(chainID string, reg state.Registry, st *state.State, hdr *types.BlockHeader,
	txs []types.Tx, maxTxs int) (*state.State, error) {
	if len(txs) > maxTxs {
		return nil, ErrTooManyTxs
	}
	if types.TxRoot(txs) != hdr.TxRoot {
		return nil, ErrBadTxRoot
	}
	next := st.Clone()
	if err := next.ApplyBlock(chainID, reg, txs); err != nil {
		return nil, err
	}
	if next.Hash() != hdr.StateHash {
		return nil, ErrBadStateHash
	}
	return next, nil
}
