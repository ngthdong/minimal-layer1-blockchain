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

// TxRoot commits to the transactions in order
//
//	H(tag || count || txHash_1 || ... || txHash_k)
//
// An empty block has a well-defined root (count = 0).
func TxRoot(txs []Tx) Hash {
	w := encoding.NewWriter().U32(uint32(len(txs)))
	for i := range txs {
		h := txs[i].Hash()
		w.Fixed(h[:])
	}
	return taggedHash(tagTxRoot, w.Out())
}
