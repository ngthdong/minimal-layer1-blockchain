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
	Equivocation                  // same validator, same step, OTHER block (T7): not counted
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

// VoteSet counts at most ONE vote per validator per step.
// Signatures must be verified BEFORE Add: the set trusts what it is given.
type VoteSet struct {
	votes        map[VoteKey]map[types.NodeID]types.Vote // first vote seen
	equivocators map[VoteKey]map[types.NodeID]bool
}

func NewVoteSet() *VoteSet {
	return &VoteSet{
		votes:        make(map[VoteKey]map[types.NodeID]types.Vote),
		equivocators: make(map[VoteKey]map[types.NodeID]bool),
	}
}

// Add records v. On Equivocation it also returns the vote that was kept, so
// the caller can log both block hashes as evidence.
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
	case first.BlockHash == v.BlockHash:
		return Duplicate, types.Vote{}
	default:
		if vs.equivocators[key] == nil {
			vs.equivocators[key] = make(map[types.NodeID]bool)
		}
		vs.equivocators[key][v.Validator] = true
		return Equivocation, first.Clone()
	}
}

// Count returns how many validators voted for hash at key (ZeroHash = NIL).
func (vs *VoteSet) Count(key VoteKey, hash types.Hash) int {
	n := 0
	for _, v := range vs.votes[key] {
		if v.BlockHash == hash {
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
	counts := map[types.Hash]int{}
	for _, v := range vs.votes[key] {
		counts[v.BlockHash]++
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
	for _, v := range vs.votes[key] {
		if v.BlockHash == hash {
			out = append(out, v.Clone())
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
