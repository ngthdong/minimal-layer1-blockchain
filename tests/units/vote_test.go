package unit

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

func sampleVote() types.Vote {
	return types.Vote{
		Type:      types.Prevote,
		Height:    5,
		Round:     0,
		BlockHash: types.Hash{9},
		Validator: 3,
	}
}

func signVote(v types.Vote, priv crypto.PrivateKey) types.Vote {
	v.Sig = crypto.Sign(priv, crypto.Context(crypto.DomainVote, chain), v.SigningBytes())
	return v
}

func TestVoteTypeString(t *testing.T) {
	if types.Prevote.String() != "PREVOTE" || types.Precommit.String() != "PRECOMMIT" {
		t.Fatal("wrong names")
	}
	if types.VoteType(0).String() != "UNKNOWN" {
		t.Fatal("zero value must not look like a real phase")
	}
}

func TestNilVote(t *testing.T) {
	v := sampleVote()
	if v.IsNil() {
		t.Fatal("vote for a block reported as NIL")
	}
	v.BlockHash = types.ZeroHash
	if !v.IsNil() {
		t.Fatal("ZeroHash vote must be NIL")
	}
}

// A signed vote must not verify after changing ANY field (rule 6 and T2/T7).
func TestVoteSignatureCannotBeReplayed(t *testing.T) {
	pub, priv := crypto.KeyFromSeed(42, "validator/3")
	ctx := crypto.Context(crypto.DomainVote, chain)
	signed := signVote(sampleVote(), priv)

	mutations := map[string]func(v *types.Vote){
		"height":    func(v *types.Vote) { v.Height++ },
		"round":     func(v *types.Vote) { v.Round++ },
		"phase":     func(v *types.Vote) { v.Type = types.Precommit },
		"block":     func(v *types.Vote) { v.BlockHash[0] ^= 1 },
		"to NIL":    func(v *types.Vote) { v.BlockHash = types.ZeroHash },
		"validator": func(v *types.Vote) { v.Validator = 4 },
	}
	for name, mutate := range mutations {
		v := signed
		mutate(&v)
		if crypto.Verify(pub, ctx, v.SigningBytes(), signed.Sig) {
			t.Fatalf("vote signature still valid after changing %s", name)
		}
	}
	if !crypto.Verify(pub, ctx, signed.SigningBytes(), signed.Sig) {
		t.Fatal("unchanged vote must verify")
	}
}

func TestProposalSigningBytesCoverEveryField(t *testing.T) {
	base := types.Proposal{Height: 7, Round: 2, POLRound: -1, Header: sampleHeader(), Proposer: 9}
	mutations := map[string]func(p *types.Proposal){
		"height":    func(p *types.Proposal) { p.Height++ },
		"round":     func(p *types.Proposal) { p.Round++ },
		"POL round": func(p *types.Proposal) { p.POLRound = 1 },
		"header":    func(p *types.Proposal) { p.Header.StateHash[0] ^= 1 },
		"proposer":  func(p *types.Proposal) { p.Proposer++ },
	}
	for name, mutate := range mutations {
		p := base
		mutate(&p)
		if bytes.Equal(p.SigningBytes(), base.SigningBytes()) {
			t.Fatalf("changing %s did not change the proposal signing bytes", name)
		}
	}
}

// Re-proposing the same block in a later round keeps the block hash (so locks
// still apply) but needs a new proposal signature (so rounds cannot be mixed).
func TestReProposalKeepsBlockHash(t *testing.T) {
	h := sampleHeader()
	r0 := types.Proposal{Height: 7, Round: 0, POLRound: -1, Header: h, Proposer: 4}
	r1 := types.Proposal{Height: 7, Round: 1, POLRound: 0, Header: h, Proposer: 5}
	if r0.BlockHash() != r1.BlockHash() {
		t.Fatal("same header must keep the same block hash across rounds")
	}
	if bytes.Equal(r0.SigningBytes(), r1.SigningBytes()) {
		t.Fatal("proposals for different rounds must sign different bytes")
	}
}

// The proposal commits to the header through its hash, which excludes the
// header signature.
func TestProposalIgnoresHeaderSignature(t *testing.T) {
	p := types.Proposal{Height: 7, Round: 0, POLRound: -1, Header: sampleHeader(), Proposer: 4}
	before := p.SigningBytes()
	p.Header.Sig = []byte{1, 2, 3}
	if !bytes.Equal(before, p.SigningBytes()) {
		t.Fatal("proposal signing bytes changed with the header signature")
	}
}
