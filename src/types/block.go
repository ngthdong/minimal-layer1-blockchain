package types

import "github.com/ngthdong/minimal-layer1-blockchain/encoding"

const (
	tagTx     = "ml1b/tx"
	tagHeader = "ml1b/header"
	tagTxRoot = "ml1b/txroot"
)

type Tx struct {
	Sender string
	Nonce  uint64
	Key    string
	Value  []byte
	Sig    []byte
}

func (tx *Tx) SigningBytes() []byte {
	return encoding.NewWriter().
		String(tx.Sender).
		U64(tx.Nonce).
		String(tx.Key).
		Bytes(tx.Value).
		Out()
}

func (tx *Tx) Hash() Hash {
	return taggedHash(tagTx, tx.SigningBytes())
}

type BlockHeader struct {
	ChainID    string
	Height     uint64
	ParentHash Hash
	TxRoot     Hash
	StateHash  Hash
	Proposer   NodeID
	Sig        []byte
}

func (h *BlockHeader) SigningBytes() []byte {
	return encoding.NewWriter().
		String(h.ChainID).
		U64(h.Height).
		Fixed(h.ParentHash[:]).
		Fixed(h.TxRoot[:]).
		Fixed(h.StateHash[:]).
		U32(uint32(h.Proposer)).
		Out()
}

func (h *BlockHeader) Hash() Hash {
	return taggedHash(tagHeader, h.SigningBytes())
}

type Block struct {
	Header BlockHeader
	Txs    []Tx
}

func (b *Block) Hash() Hash {
	return b.Header.Hash()
}

// TxRoot commits to the transactions IN ORDER, signatures included:
//
//	H(tag || count || (signingBytes_1, sig_1) || ... || (signingBytes_k, sig_k))
//
// The signatures must be in it. Otherwise a relaying peer could flip one
// signature byte in the body of a GOOD block: the copy would still match the
// header's TxRoot, fail with bad_signature, and honest nodes would mark the
// block itself invalid. With them, any change to a body is bad_tx_root:
// only that copy is refused, and a body that matches the root IS the block.
// (Tx.Hash, the tx's identity in the mempool, stays signature-free.)
//
// An empty block has a well-defined root (count = 0).
func TxRoot(txs []Tx) Hash {
	w := encoding.NewWriter().U32(uint32(len(txs)))
	for i := range txs {
		w.Bytes(txs[i].SigningBytes()).Bytes(txs[i].Sig)
	}
	return taggedHash(tagTxRoot, w.Out())
}
