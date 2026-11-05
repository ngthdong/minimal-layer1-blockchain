package consensus

import (
	"bytes"
	"sort"

	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// VoteKey identifies one voting step: (height, round, prevote|precommit).
type VoteKey struct {
	Height uint64
	Round  int32
	Type   types.VoteType
}

// KeyOf returns the step a vote belongs to.
func KeyOf(v *types.Vote) VoteKey { return VoteKey{v.Height, v.Round, v.Type} }

// AddResult says what VoteSet.Add did with a vote.
type AddResult int

const (
	Added        AddResult = iota // first vote of this validator for this step: counted
	Duplicate                     // same vote again (gossip, T2): ignored
	Equivocation                  // same validator, same step, OTHER block (T7): evidence
)

func (r AddResult) String() string {
	switch r {
	case Added:
		return "added"
	case Duplicate:
		return "duplicate"
	case Equivocation:
		return "equivocation"
	}
	return "unknown"
}

// maxConflicting bounds the extra votes kept per validator and step: a
// Byzantine validator may sign thousands of blocks for one step, and must
// not be able to fill our memory with them.
const maxConflicting = 3

// VoteSet keeps every distinct signed vote of each validator per step, and
// counts a validator at most ONCE in any tally (rule 7): Count(key, B) is the
// number of distinct validators that signed a vote for B at key.
//
// An honest validator signs one vote per step, so for it nothing changes.
// An equivocator that signed B for some nodes and NIL for others counts once
// toward B and once toward NIL, exactly as Tendermint counts signed
// messages. Keeping only the first copy seen would let the ORDER OF ARRIVAL
// decide what each node counts: nodes would disagree forever on whether a
// POL exists, and with f equivocators the chain can stall.
//
// Safety is unchanged: two quorums of 2f+1 share at least f+1 validators,
// at least one of them honest, and an honest validator signed only one of
// the two blocks. So even counting every Byzantine vote on both sides, no
// two blocks reach a quorum at the same step.
//
// Signatures must be verified BEFORE Add: the set trusts what it is given.
type VoteSet struct {
	votes        map[VoteKey]map[types.NodeID]types.Vote   // first vote seen
	others       map[VoteKey]map[types.NodeID][]types.Vote // conflicting votes (other blocks)
	equivocators map[VoteKey]map[types.NodeID]bool
}

func NewVoteSet() *VoteSet {
	return &VoteSet{
		votes:        make(map[VoteKey]map[types.NodeID]types.Vote),
		others:       make(map[VoteKey]map[types.NodeID][]types.Vote),
		equivocators: make(map[VoteKey]map[types.NodeID]bool),
	}
}

// Add records v. On Equivocation it also returns the first vote of that
// validator, so the caller can log both block hashes as evidence.
func (vs *VoteSet) Add(v types.Vote) (AddResult, types.Vote) {
	key := KeyOf(&v)
	byVal := vs.votes[key]
	if byVal == nil {
		byVal = make(map[types.NodeID]types.Vote)
		vs.votes[key] = byVal
	}
	first, seen := byVal[v.Validator]
	switch {
	case !seen:
		byVal[v.Validator] = v.Clone()
		return Added, types.Vote{}
	case vs.signed(key, v.Validator, v.BlockHash):
		return Duplicate, types.Vote{}
	}
	if vs.equivocators[key] == nil {
		vs.equivocators[key] = make(map[types.NodeID]bool)
		vs.others[key] = make(map[types.NodeID][]types.Vote)
	}
	vs.equivocators[key][v.Validator] = true
	if len(vs.others[key][v.Validator]) < maxConflicting {
		vs.others[key][v.Validator] = append(vs.others[key][v.Validator], v.Clone())
	}
	return Equivocation, first.Clone()
}

// signed reports whether validator id signed a vote for hash at key.
func (vs *VoteSet) signed(key VoteKey, id types.NodeID, hash types.Hash) bool {
	if v, ok := vs.votes[key][id]; ok && v.BlockHash == hash {
		return true
	}
	for _, o := range vs.others[key][id] {
		if o.BlockHash == hash {
			return true
		}
	}
	return false
}

// Count returns how many distinct validators signed a vote for hash at key
// (ZeroHash = NIL).
func (vs *VoteSet) Count(key VoteKey, hash types.Hash) int {
	n := 0
	for id := range vs.votes[key] {
		if vs.signed(key, id, hash) {
			n++
		}
	}
	return n
}

// CountAny returns how many validators voted at key, for anything (NIL too).
// "2f+1 of any" is what starts the prevote and precommit timeouts.
func (vs *VoteSet) CountAny(key VoteKey) int { return len(vs.votes[key]) }

// Majority returns the hash (possibly ZeroHash = NIL) that has at least quorum
// votes at key. With quorum = 2f+1 > n/2 at most one hash can qualify; ties
// are still broken by hash order so the answer never depends on map order.
func (vs *VoteSet) Majority(key VoteKey, quorum int) (types.Hash, bool) {
	counts := map[types.Hash]int{} // a validator's hashes are distinct: one count each
	for id, v := range vs.votes[key] {
		counts[v.BlockHash]++
		for _, o := range vs.others[key][id] {
			counts[o.BlockHash]++
		}
	}
	var best types.Hash
	found := false
	for h, c := range counts {
		if c >= quorum && (!found || bytes.Compare(h[:], best[:]) < 0) {
			best, found = h, true
		}
	}
	return best, found
}

// VotesFor returns copies of the votes for hash at key, sorted by validator
// (the order used in a CommitCert).
func (vs *VoteSet) VotesFor(key VoteKey, hash types.Hash) []types.Vote {
	var out []types.Vote
	for id, v := range vs.votes[key] {
		for _, c := range append([]types.Vote{v}, vs.others[key][id]...) {
			if c.BlockHash == hash {
				out = append(out, c.Clone()) // at most one per validator
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Validator < out[j].Validator })
	return out
}

// Equivocators returns, sorted, the validators caught equivocating at key.
func (vs *VoteSet) Equivocators(key VoteKey) []types.NodeID {
	var out []types.NodeID
	for id := range vs.equivocators[key] {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Rounds returns, ascending, the rounds of height h that hold at least one
// vote of type typ.
func (vs *VoteSet) Rounds(h uint64, typ types.VoteType) []int32 {
	var out []int32
	for k := range vs.votes {
		if k.Height == h && k.Type == typ {
			out = append(out, k.Round)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Voters counts the distinct validators with a prevote or a precommit at
// (h, r): how many validators are known to have reached that round.
func (vs *VoteSet) Voters(h uint64, r int32) int {
	ids := map[types.NodeID]bool{}
	for _, typ := range []types.VoteType{types.Prevote, types.Precommit} {
		for id := range vs.votes[VoteKey{Height: h, Round: r, Type: typ}] {
			ids[id] = true
		}
	}
	return len(ids)
}
