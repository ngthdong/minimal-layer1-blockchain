package types

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return append([]byte(nil), b...)
}

func (tx Tx) Clone() Tx {
	tx.Value = cloneBytes(tx.Value)
	tx.Sig = cloneBytes(tx.Sig)
	return tx
}

func cloneTxs(txs []Tx) []Tx {
	if txs == nil {
		return nil
	}
	out := make([]Tx, len(txs))
	for i := range txs {
		out[i] = txs[i].Clone()
	}
	return out
}

func (h BlockHeader) Clone() BlockHeader { h.Sig = cloneBytes(h.Sig); return h }
func (v Vote) Clone() Vote               { v.Sig = cloneBytes(v.Sig); return v }

func (b Block) Clone() Block { return Block{Header: b.Header.Clone(), Txs: cloneTxs(b.Txs)} }

func (p Proposal) Clone() Proposal {
	p.Header = p.Header.Clone()
	p.Sig = cloneBytes(p.Sig)
	p.POL = CloneVotes(p.POL)
	return p
}

func CloneVotes(vs []Vote) []Vote {
	if vs == nil {
		return nil
	}
	out := make([]Vote, len(vs))
	for i := range vs {
		out[i] = vs[i].Clone()
	}
	return out
}

func (c CommitCert) Clone() CommitCert {
	if c.Precommits != nil {
		votes := make([]Vote, len(c.Precommits))
		for i := range c.Precommits {
			votes[i] = c.Precommits[i].Clone()
		}
		c.Precommits = votes
	}
	return c
}
