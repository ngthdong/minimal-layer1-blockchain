package consensus

import (
	"errors"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// ErrDoubleSign is returned when asked to sign a second, DIFFERENT vote (or
// proposal) for a step that already has one. An honest validator must never
// do that: two conflicting signed messages for one step are equivocation.
var ErrDoubleSign = errors.New("double_sign_refused")

// VoteSigner is the only holder of a validator's private key for votes.
// It enforces rules 1, 2 and 6 of the lab in ONE place, whatever the engine
// logic above it does:
//
//   - at most one vote per (height, round, type);
//   - asking again for the SAME vote returns the stored one (used to re-send
//     votes after a timeout, stage 29);
//   - asking for a different vote in a step already signed is refused.
//
// It also signs headers and proposals, with the same guard: at most one
// proposal per (height, round).
type VoteSigner struct {
	id        types.NodeID
	priv      crypto.PrivateKey
	chainID   string
	signed    map[VoteKey]types.Vote
	proposals map[VoteKey]types.Hash // Type unused (0): key = (height, round)
}

func NewVoteSigner(id types.NodeID, priv crypto.PrivateKey, chainID string) *VoteSigner {
	return &VoteSigner{id: id, priv: priv, chainID: chainID,
		signed: make(map[VoteKey]types.Vote), proposals: make(map[VoteKey]types.Hash)}
}

// SignHeader signs a header this validator built, under "HEADER:<chain_id>".
func (s *VoteSigner) SignHeader(h *types.BlockHeader) {
	h.Proposer = s.id
	h.Sig = crypto.Sign(s.priv, crypto.Context(crypto.DomainHeader, s.chainID), h.SigningBytes())
}

// SignProposal signs p (Proposer is set to this validator) under
// "PROPOSAL:<chain_id>". A second proposal for another block in the same
// (height, round) is refused.
func (s *VoteSigner) SignProposal(p *types.Proposal) error {
	key := VoteKey{Height: p.Height, Round: p.Round}
	if prev, ok := s.proposals[key]; ok && prev != p.BlockHash() {
		return ErrDoubleSign
	}
	p.Proposer = s.id
	p.Sig = crypto.Sign(s.priv, crypto.Context(crypto.DomainProposal, s.chainID), p.SigningBytes())
	s.proposals[key] = p.BlockHash()
	return nil
}

// Sign returns this validator's vote for (h, r, typ) on hash (ZeroHash = NIL),
// signed under "VOTE:<chain_id>".
func (s *VoteSigner) Sign(typ types.VoteType, h uint64, r int32, hash types.Hash) (types.Vote, error) {
	key := VoteKey{Height: h, Round: r, Type: typ}
	if prev, ok := s.signed[key]; ok {
		if prev.BlockHash != hash {
			return types.Vote{}, ErrDoubleSign
		}
		return prev.Clone(), nil
	}
	v := types.Vote{Type: typ, Height: h, Round: r, BlockHash: hash, Validator: s.id}
	v.Sig = crypto.Sign(s.priv, crypto.Context(crypto.DomainVote, s.chainID), v.SigningBytes())
	s.signed[key] = v.Clone()
	return v, nil
}

// Signed returns the vote already signed for (h, r, typ), if any.
func (s *VoteSigner) Signed(typ types.VoteType, h uint64, r int32) (types.Vote, bool) {
	v, ok := s.signed[VoteKey{Height: h, Round: r, Type: typ}]
	return v.Clone(), ok
}

// Prune forgets votes below height h (they can never be needed again).
func (s *VoteSigner) Prune(h uint64) {
	for k := range s.signed {
		if k.Height < h {
			delete(s.signed, k)
		}
	}
	for k := range s.proposals {
		if k.Height < h {
			delete(s.proposals, k)
		}
	}
}
