package types

import "github.com/ngthdong/minimal-layer1-blockchain/encoding"

type MsgKind uint8

const (
	MsgTx MsgKind = iota + 1
	MsgProposal
	MsgBodyRequest
	MsgBodyResponse
	MsgVote
	MsgSyncRequest
	MsgSyncResponse
)

var kindNames = [...]string{"UNKNOWN", "TX", "PROPOSAL", "BODY_REQ", "BODY_RESP", "VOTE", "SYNC_REQ", "SYNC_RESP"}

func (k MsgKind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "UNKNOWN"
}

type Message interface {
	Kind() MsgKind
	ID() Hash
	MsgHeight() uint64
	Clone() Message
}

type Envelope struct {
	From NodeID
	To   NodeID
	Msg  Message
}

type (
	TxMsg       struct{ Tx Tx }
	ProposalMsg struct{ Proposal Proposal }
	VoteMsg     struct{ Vote Vote }
	// BodyRequest is sent only after the requester has accepted the header.
	BodyRequest struct {
		Height    uint64
		BlockHash Hash
	}
	BodyResponse struct {
		Height    uint64
		BlockHash Hash
		Txs       []Tx
	}
	// SyncRequest asks a peer for finalized blocks starting at FromHeight.
	SyncRequest  struct{ FromHeight uint64 }
	SyncResponse struct {
		Block Block
		Cert  CommitCert
	}
)

func (*TxMsg) Kind() MsgKind        { return MsgTx }
func (*ProposalMsg) Kind() MsgKind  { return MsgProposal }
func (*VoteMsg) Kind() MsgKind      { return MsgVote }
func (*BodyRequest) Kind() MsgKind  { return MsgBodyRequest }
func (*BodyResponse) Kind() MsgKind { return MsgBodyResponse }
func (*SyncRequest) Kind() MsgKind  { return MsgSyncRequest }
func (*SyncResponse) Kind() MsgKind { return MsgSyncResponse }

func (m *TxMsg) MsgHeight() uint64        { return 0 }
func (m *ProposalMsg) MsgHeight() uint64  { return m.Proposal.Height }
func (m *VoteMsg) MsgHeight() uint64      { return m.Vote.Height }
func (m *BodyRequest) MsgHeight() uint64  { return m.Height }
func (m *BodyResponse) MsgHeight() uint64 { return m.Height }
func (m *SyncRequest) MsgHeight() uint64  { return m.FromHeight }
func (m *SyncResponse) MsgHeight() uint64 { return m.Block.Header.Height }

// msgID = H("ml1b/msg" || kind || content). Content always includes the
// signature, so two conflicting votes (equivocation) get two IDs and both
// reach the vote set, and a forged copy cannot shadow the real message.
func msgID(kind MsgKind, w *encoding.Writer) Hash {
	return taggedHash("ml1b/msg", encoding.NewWriter().U8(uint8(kind)).Bytes(w.Out()).Out())
}

func (m *TxMsg) ID() Hash {
	return msgID(MsgTx, encoding.NewWriter().Bytes(m.Tx.SigningBytes()).Bytes(m.Tx.Sig))
}

func (m *ProposalMsg) ID() Hash {
	p := &m.Proposal
	return msgID(MsgProposal, encoding.NewWriter().
		Bytes(p.SigningBytes()).Bytes(p.Sig).Bytes(p.Header.Sig))
}

func (m *VoteMsg) ID() Hash {
	return msgID(MsgVote, encoding.NewWriter().Bytes(m.Vote.SigningBytes()).Bytes(m.Vote.Sig))
}

func (m *BodyRequest) ID() Hash {
	return msgID(MsgBodyRequest, encoding.NewWriter().U64(m.Height).Fixed(m.BlockHash[:]))
}

// The ID covers the transactions, not just the block hash: a bad body sent
// first must not make a node ignore the correct body that comes later.
func (m *BodyResponse) ID() Hash {
	root := TxRoot(m.Txs)
	return msgID(MsgBodyResponse, encoding.NewWriter().
		U64(m.Height).Fixed(m.BlockHash[:]).Fixed(root[:]))
}

func (m *SyncRequest) ID() Hash {
	return msgID(MsgSyncRequest, encoding.NewWriter().U64(m.FromHeight))
}

func (m *SyncResponse) ID() Hash {
	w := encoding.NewWriter().
		Bytes(m.Block.Header.SigningBytes()).Bytes(m.Block.Header.Sig)
	root := TxRoot(m.Block.Txs)
	w.Fixed(root[:]).U32(uint32(len(m.Cert.Precommits)))
	for i := range m.Cert.Precommits {
		v := &m.Cert.Precommits[i]
		w.Bytes(v.SigningBytes()).Bytes(v.Sig)
	}
	return msgID(MsgSyncResponse, w)
}

func (m *TxMsg) Clone() Message       { return &TxMsg{Tx: m.Tx.Clone()} }
func (m *ProposalMsg) Clone() Message { return &ProposalMsg{Proposal: m.Proposal.Clone()} }
func (m *VoteMsg) Clone() Message     { return &VoteMsg{Vote: m.Vote.Clone()} }
func (m *BodyRequest) Clone() Message { c := *m; return &c }
func (m *SyncRequest) Clone() Message { c := *m; return &c }
func (m *BodyResponse) Clone() Message {
	return &BodyResponse{Height: m.Height, BlockHash: m.BlockHash, Txs: cloneTxs(m.Txs)}
}
func (m *SyncResponse) Clone() Message {
	return &SyncResponse{Block: m.Block.Clone(), Cert: m.Cert.Clone()}
}
