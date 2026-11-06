package types

import "github.com/ngthdong/minimal-layer1-blockchain/encoding"

// Proposal announces a block header for (Height, Round). It is signed by the
// proposer OF THAT ROUND, which may differ from Header.Proposer when a block
// that was already valid in an earlier round is proposed again (POLRound >= 0).
type Proposal struct {
	Height   uint64
	Round    int32
	POLRound int32 // round in which the block got 2f+1 prevotes, or -1
	Header   BlockHeader
	Proposer NodeID
	Sig      []byte // under "PROPOSAL:<chain_id>"

	// POL (proof of lock): when POLRound >= 0, the 2f+1 signed prevotes for
	// this block in round POLRound, so that every receiver can observe the
	// quorum itself (rule 5). Not covered by Sig: each vote carries its own.
	POL []Vote
}

func (p *Proposal) BlockHash() Hash { return p.Header.Hash() }

func (p *Proposal) SigningBytes() []byte {
	bh := p.BlockHash()
	return encoding.NewWriter().
		U64(p.Height).
		I32(p.Round).
		I32(p.POLRound).
		Fixed(bh[:]).
		U32(uint32(p.Proposer)).
		Out()
}

type VoteType uint8

const (
	Prevote   VoteType = 1
	Precommit VoteType = 2
)

func (t VoteType) String() string {
	switch t {
	case Prevote:
		return "PREVOTE"
	case Precommit:
		return "PRECOMMIT"
	}
	return "UNKNOWN"
}

type Vote struct {
	Type      VoteType
	Height    uint64
	Round     int32
	BlockHash Hash // ZeroHash = NIL ("no block this round")
	Validator NodeID
	Sig       []byte // under "VOTE:<chain_id>"
}

func (v *Vote) IsNil() bool { return v.BlockHash.IsZero() }

func (v *Vote) SigningBytes() []byte {
	return encoding.NewWriter().
		U8(uint8(v.Type)).
		U64(v.Height).
		I32(v.Round).
		Fixed(v.BlockHash[:]).
		U32(uint32(v.Validator)).
		Out()
}

type CommitCert struct {
	Height     uint64
	Round      int32
	BlockHash  Hash
	Precommits []Vote // sorted by Validator
}
