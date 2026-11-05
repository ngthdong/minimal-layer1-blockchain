package unit

import (
	"fmt"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

func validatorKeys(n int) ([]crypto.PublicKey, []crypto.PrivateKey) {
	pubs, privs := make([]crypto.PublicKey, n), make([]crypto.PrivateKey, n)
	for i := 0; i < n; i++ {
		pubs[i], privs[i] = crypto.KeyFromSeed(42, fmt.Sprintf("validator/%d", i))
	}
	return pubs, privs
}

func validatorSet(t *testing.T, n int) *consensus.ValidatorSet {
	t.Helper()
	pubs, _ := validatorKeys(n)
	vs, err := consensus.NewValidatorSet(pubs)
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestQuorumSizes(t *testing.T) {
	for _, c := range []struct{ n, f, q int }{{4, 1, 3}, {7, 2, 5}, {10, 3, 7}, {13, 4, 9}} {
		vs := validatorSet(t, c.n)
		if vs.N() != c.n || vs.F() != c.f || vs.Quorum() != c.q {
			t.Fatalf("n=%d: f=%d quorum=%d, want f=%d quorum=%d", c.n, vs.F(), vs.Quorum(), c.f, c.q)
		}
	}
}

// Two quorums always share >= f+1 validators, i.e. at least one honest one.
func TestQuorumsIntersectInAnHonestValidator(t *testing.T) {
	for f := 1; f <= 30; f++ {
		n, q := 3*f+1, 2*f+1
		if overlap := 2*q - n; overlap < f+1 {
			t.Fatalf("n=%d: two quorums may overlap in only %d validators", n, overlap)
		}
	}
}

func TestRejectsBadValidatorCounts(t *testing.T) {
	for _, n := range []int{0, 1, 3, 5, 9} {
		pubs, _ := validatorKeys(n)
		if _, err := consensus.NewValidatorSet(pubs); err == nil {
			t.Fatalf("n=%d accepted", n)
		}
	}
}

// Within one height, rounds 0..n-1 give every validator exactly one turn,
// so at most f consecutive rounds can have a faulty proposer.
func TestProposerVisitsEveryValidator(t *testing.T) {
	vs := validatorSet(t, 10)
	for _, h := range []uint64{1, 7, 1 << 40} {
		seen := map[types.NodeID]bool{}
		for r := int32(0); r < int32(vs.N()); r++ {
			seen[vs.Proposer(h, r)] = true
		}
		if len(seen) != vs.N() {
			t.Fatalf("height %d: only %d distinct proposers in %d rounds", h, len(seen), vs.N())
		}
	}
	if vs.Proposer(1, 0) == vs.Proposer(2, 0) {
		t.Fatal("consecutive heights must start with different proposers")
	}
}

func TestKeyLookupAndCopy(t *testing.T) {
	pubs, _ := validatorKeys(10)
	vs, _ := consensus.NewValidatorSet(pubs)
	pubs[3] = nil // caller edits its own slice afterwards
	if k, ok := vs.Key(3); !ok || len(k) != 32 {
		t.Fatal("validator set changed through the caller's slice")
	}
	if _, ok := vs.Key(10); ok {
		t.Fatal("NodeID 10 is not a validator in a set of 10")
	}
}
