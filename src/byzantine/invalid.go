package byzantine

import "github.com/ngthdong/minimal-layer1-blockchain/types"

// InvalidProposal proposes invalid blocks (T7), cycling through three
// kinds, one per block it builds:
//
//  1. a tx with a broken signature  → body check fails: bad_signature
//     (TxRoot recomputed: the body matches its header, the tx is the fault;
//     an empty block has no tx to break, so it gets kind 2 instead)
//  2. wrong StateHash               → body check fails: bad_state_hash
//  3. wrong ParentHash              → header check fails: bad_parent
//
// Honest nodes log REJECT and prevote NIL (or time out on a header they
// refused); the round fails and the next proposer's block is finalized.
// Its votes, relays and bodies of other blocks stay honest.
type InvalidProposal struct{ built int }

func (*InvalidProposal) Outgoing(_ types.NodeID, m types.Message) []types.Message {
	return []types.Message{m}
}

func (p *InvalidProposal) CorruptBlock(b types.Block) types.Block {
	b = b.Clone()
	kind := p.built % 3
	p.built++
	if kind == 0 && len(b.Txs) == 0 {
		kind = 1
	}
	switch kind {
	case 0:
		b.Txs[0].Sig[0] ^= 0xff
		b.Header.TxRoot = types.TxRoot(b.Txs)
	case 1:
		b.Header.StateHash[0] ^= 0xff
	case 2:
		b.Header.ParentHash[0] ^= 0xff
	}
	return b
}
