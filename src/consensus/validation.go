package consensus

import (
	"errors"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Rejection reasons, logged as "reason=<err>". One token each.
var (
	ErrUnknownValidator = errors.New("unknown_validator")
	ErrBadSignature     = errors.New("bad_signature")
	ErrBadHeaderSig     = errors.New("bad_header_signature")
	ErrBadVoteType      = errors.New("bad_vote_type")
	ErrBadRound         = errors.New("bad_round")
	ErrBadPOLRound      = errors.New("bad_pol_round")
	ErrWrongProposer    = errors.New("wrong_proposer")
	ErrWrongChain       = errors.New("wrong_chain")
	ErrWrongHeight      = errors.New("wrong_height")
	ErrBadParent        = errors.New("bad_parent")
)

// VerifyVote checks that v is well formed and really signed by the validator
// it names, under "VOTE:<chain_id>". It does NOT check whether the vote is
// for the node's current height or round: the engine decides that.
// Call it BEFORE VoteSet.Add (a forged vote kept as "first" would make the
// real vote look like an equivocation).
func VerifyVote(chainID string, vs *ValidatorSet, v *types.Vote) error {
	if v.Type != types.Prevote && v.Type != types.Precommit {
		return ErrBadVoteType
	}
	if v.Round < 0 {
		return ErrBadRound
	}
	pub, ok := vs.Key(v.Validator)
	if !ok {
		return ErrUnknownValidator
	}
	if !crypto.Verify(pub, crypto.Context(crypto.DomainVote, chainID), v.SigningBytes(), v.Sig) {
		return ErrBadSignature
	}
	return nil
}

func VerifyProposal(chainID string, vs *ValidatorSet, parent types.Hash, p *types.Proposal) error {
	if p.Round < 0 {
		return ErrBadRound
	}
	if p.POLRound < -1 || p.POLRound >= p.Round {
		return ErrBadPOLRound
	}
	if p.Proposer != vs.Proposer(p.Height, p.Round) {
		return ErrWrongProposer
	}
	pub, _ := vs.Key(p.Proposer) // always a validator: Proposer() picked it
	if !crypto.Verify(pub, crypto.Context(crypto.DomainProposal, chainID), p.SigningBytes(), p.Sig) {
		return ErrBadSignature
	}
	h := &p.Header
	if h.ChainID != chainID {
		return ErrWrongChain
	}
	if h.Height != p.Height {
		return ErrWrongHeight
	}
	builder, ok := vs.Key(h.Proposer)
	if !ok {
		return ErrUnknownValidator
	}
	if !crypto.Verify(builder, crypto.Context(crypto.DomainHeader, chainID), h.SigningBytes(), h.Sig) {
		return ErrBadHeaderSig
	}
	if h.ParentHash != parent {
		return ErrBadParent
	}
	return nil
}
