package unit

import (
	"errors"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

var vPubs, vPrivs = validatorKeys(10)

func vset(t *testing.T) *consensus.ValidatorSet {
	t.Helper()
	vs, err := consensus.NewValidatorSet(vPubs)
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

// signedVote returns a vote correctly signed by validator id.
func signedVote(id types.NodeID, typ types.VoteType, h uint64, r int32, bh types.Hash) types.Vote {
	v := types.Vote{Type: typ, Height: h, Round: r, BlockHash: bh, Validator: id}
	v.Sig = crypto.Sign(vPrivs[id], crypto.Context(crypto.DomainVote, chain), v.SigningBytes())
	return v
}

func TestVerifyVote(t *testing.T) {
	vs := vset(t)
	good := signedVote(1, types.Prevote, 5, 0, blockA)
	if err := consensus.VerifyVote(chain, vs, &good); err != nil {
		t.Fatalf("valid vote rejected: %v", err)
	}

	impersonated := good
	impersonated.Validator = 2 // validator 1's signature, claims to be 2
	wrongDomain := good
	wrongDomain.Sig = crypto.Sign(vPrivs[1], crypto.Context(crypto.DomainTx, chain), good.SigningBytes())
	unknown := good
	unknown.Validator = 10
	noType := good
	noType.Type = 0
	negRound := good
	negRound.Round = -1
	replayed := good
	replayed.Height = 6 // same signature, other height

	cases := []struct {
		name string
		v    types.Vote
		want error
	}{
		{"impersonation", impersonated, consensus.ErrBadSignature},
		{"TX domain", wrongDomain, consensus.ErrBadSignature},
		{"replayed at other height", replayed, consensus.ErrBadSignature},
		{"unknown validator", unknown, consensus.ErrUnknownValidator},
		{"type 0", noType, consensus.ErrBadVoteType},
		{"negative round", negRound, consensus.ErrBadRound},
	}
	for _, c := range cases {
		if err := consensus.VerifyVote(chain, vs, &c.v); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// proposal builds a header made by builder and a proposal for (h, r) signed by
// the real proposer of that round.
func proposal(vs *consensus.ValidatorSet, builder types.NodeID, h uint64, r, pol int32, parent types.Hash) types.Proposal {
	hdr := types.BlockHeader{ChainID: chain, Height: h, ParentHash: parent, Proposer: builder}
	hdr.Sig = crypto.Sign(vPrivs[builder], crypto.Context(crypto.DomainHeader, chain), hdr.SigningBytes())
	p := types.Proposal{Height: h, Round: r, POLRound: pol, Header: hdr, Proposer: vs.Proposer(h, r)}
	p.Sig = crypto.Sign(vPrivs[p.Proposer], crypto.Context(crypto.DomainProposal, chain), p.SigningBytes())
	return p
}

// resign re-signs p after a test changed it, so only the intended check fails.
func resign(p types.Proposal, signer types.NodeID) types.Proposal {
	p.Sig = crypto.Sign(vPrivs[signer], crypto.Context(crypto.DomainProposal, chain), p.SigningBytes())
	return p
}

func TestVerifyProposalAcceptsFreshAndReProposals(t *testing.T) {
	vs, parent := vset(t), types.Hash{1}
	fresh := proposal(vs, vs.Proposer(5, 0), 5, 0, -1, parent)
	if err := consensus.VerifyProposal(chain, vs, parent, &fresh); err != nil {
		t.Fatalf("fresh proposal rejected: %v", err)
	}
	// Block built in round 0, proposed again in round 2 by round 2's proposer.
	again := proposal(vs, vs.Proposer(5, 0), 5, 2, 0, parent)
	if err := consensus.VerifyProposal(chain, vs, parent, &again); err != nil {
		t.Fatalf("re-proposal rejected: %v", err)
	}
}

func TestVerifyProposalRejects(t *testing.T) {
	vs, parent := vset(t), types.Hash{1}
	base := func() types.Proposal { return proposal(vs, vs.Proposer(5, 1), 5, 1, -1, parent) }
	cases := map[string]struct {
		edit func(p *types.Proposal) types.Proposal
		want error
	}{
		"negative round": {func(p *types.Proposal) types.Proposal { p.Round = -1; return *p }, consensus.ErrBadRound},
		"POL == round": {func(p *types.Proposal) types.Proposal { p.POLRound = 1; return resign(*p, p.Proposer) },
			consensus.ErrBadPOLRound},
		"POL < -1": {func(p *types.Proposal) types.Proposal { p.POLRound = -2; return resign(*p, p.Proposer) },
			consensus.ErrBadPOLRound},
		"not my turn": {func(p *types.Proposal) types.Proposal { p.Proposer++; return resign(*p, p.Proposer) },
			consensus.ErrWrongProposer},
		"forged proposal sig": {func(p *types.Proposal) types.Proposal { p.Sig[0] ^= 1; return *p },
			consensus.ErrBadSignature},
		"other chain": {func(p *types.Proposal) types.Proposal { p.Header.ChainID = "x"; return resign(*p, p.Proposer) },
			consensus.ErrWrongChain},
		"height mismatch": {func(p *types.Proposal) types.Proposal { p.Header.Height = 4; return resign(*p, p.Proposer) },
			consensus.ErrWrongHeight},
		"unknown builder": {func(p *types.Proposal) types.Proposal { p.Header.Proposer = 99; return resign(*p, p.Proposer) },
			consensus.ErrUnknownValidator},
		"header edited after signing": {func(p *types.Proposal) types.Proposal {
			p.Header.StateHash[0] ^= 1
			return resign(*p, p.Proposer)
		}, consensus.ErrBadHeaderSig},
	}
	for name, c := range cases {
		p := base()
		p = c.edit(&p)
		if err := consensus.VerifyProposal(chain, vs, parent, &p); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	other := base()
	if err := consensus.VerifyProposal(chain, vs, types.Hash{2}, &other); !errors.Is(err, consensus.ErrBadParent) {
		t.Fatalf("wrong parent: err = %v", err)
	}
}
