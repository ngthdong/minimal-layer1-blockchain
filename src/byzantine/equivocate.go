package byzantine

import (
	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Equivocate double-votes (T7). For every prevote and precommit of its own,
// it signs a second vote for the same (type, height, round) but another
// block, and sends that one to the upper half of the validators (NodeID >=
// n/2); the lower half gets the real vote. Honest nodes relay the first
// copy they see of each, so in the end every node holds both signed votes:
// the evidence it logs as EQUIVOCATION, each vote counting once toward its own block (rule 7).
//
// Everything else — proposals, bodies, other validators' votes it relays —
// goes out unchanged: the attack is the double vote alone.
type Equivocate struct{ Self Identity }

func (b Equivocate) Outgoing(to types.NodeID, msg types.Message) []types.Message {
	vm, ok := msg.(*types.VoteMsg)
	if !ok || vm.Vote.Validator != b.Self.ID || int(to) < b.Self.N/2 {
		return []types.Message{msg}
	}
	v := vm.Vote.Clone()
	v.BlockHash = conflicting(v.BlockHash)
	v.Sig = crypto.Sign(b.Self.Priv, crypto.Context(crypto.DomainVote, b.Self.ChainID), v.SigningBytes())
	return []types.Message{&types.VoteMsg{Vote: v}}
}

// conflicting is "the other choice": NIL instead of a block, and a block
// nobody proposed instead of NIL. Either way the vote is validly signed.
func conflicting(h types.Hash) types.Hash {
	if !h.IsZero() {
		return types.ZeroHash
	}
	return types.Hash(crypto.Hash([]byte("byzantine/equivocate")))
}
