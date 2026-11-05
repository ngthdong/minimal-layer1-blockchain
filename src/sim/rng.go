package sim

import "hash/fnv"

type RNG struct {
	state uint64
}

// NewRNG returns an independent stream for one component, e.g. "network",
// "workload" or "byzantine/9". Separate streams mean an extra random draw in
// one component does not shift the numbers seen by the others.
func NewRNG(seed uint64, stream string) *RNG {
	h := fnv.New64a()
	h.Write([]byte(stream))
	r := &RNG{state: seed ^ h.Sum64()}
	r.Uint64() // mix once so similar seeds diverge immediately
	return r
}

// Uint64 returns the next 64 random bits.
func (r *RNG) Uint64() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Uint64N returns a uniform value in [0, n). It panics if n == 0.
// Rejection sampling avoids the bias of a plain modulo.
func (r *RNG) Uint64N(n uint64) uint64 {
	if n == 0 {
		panic("sim: Uint64N(0)")
	}
	limit := ^uint64(0) - (^uint64(0) % n) // largest multiple of n
	for {
		if x := r.Uint64(); x < limit {
			return x % n
		}
	}
}

// IntN returns a uniform int in [0, n).
func (r *RNG) IntN(n int) int { return int(r.Uint64N(uint64(n))) }

// Range returns a uniform value in [lo, hi] (inclusive). If hi <= lo it returns lo.
func (r *RNG) Range(lo, hi uint64) uint64 {
	if hi <= lo {
		return lo
	}
	return lo + r.Uint64N(hi-lo+1)
}

// Float64 returns a uniform value in [0, 1) with 53 bits of precision.
func (r *RNG) Float64() float64 { return float64(r.Uint64()>>11) / (1 << 53) }

// Chance returns true with probability p. It ALWAYS draws one number, even
// for p <= 0 or p >= 1, so the stream advances the same way whatever p is.
func (r *RNG) Chance(p float64) bool { return r.Float64() < p }
