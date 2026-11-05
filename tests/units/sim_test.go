package unit

import (
	"reflect"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
)

func TestSchedulerOrdersByTimeThenInsertion(t *testing.T) {
	s := sim.NewScheduler()
	var got []int
	s.At(10, func() { got = append(got, 3) })
	s.At(5, func() { got = append(got, 1) })
	s.At(10, func() { got = append(got, 4) })
	s.At(5, func() { got = append(got, 2) })
	s.Run(100, nil)
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("order = %v", got)
	}
}

// Events scheduled while running, for the current instant or the past, run
// after everything already queued for that instant.
func TestSchedulerNestedAndPastEvents(t *testing.T) {
	s := sim.NewScheduler()
	var got []string
	s.At(5, func() {
		got = append(got, "a")
		s.After(0, func() { got = append(got, "c") })
		s.At(1, func() { got = append(got, "d") }) // past: runs at 5
	})
	s.At(5, func() { got = append(got, "b") })
	s.Run(100, nil)
	if !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("order = %v", got)
	}
	if s.Now() != 5 {
		t.Fatalf("now = %d, want 5", s.Now())
	}
}

func TestSchedulerRunUntilAndStop(t *testing.T) {
	s := sim.NewScheduler()
	ran := 0
	for _, at := range []uint64{10, 20, 30, 40} {
		s.At(at, func() { ran++ })
	}
	s.Run(25, nil)
	if ran != 2 || s.Now() != 25 || s.Pending() != 2 {
		t.Fatalf("ran=%d now=%d pending=%d, want 2/25/2", ran, s.Now(), s.Pending())
	}
	s.Run(1000, func() bool { return ran == 3 })
	if ran != 3 || s.Now() != 30 {
		t.Fatalf("stop: ran=%d now=%d, want 3/30", ran, s.Now())
	}
}

func TestRNGIsReproducibleAndPinned(t *testing.T) {
	a, b := sim.NewRNG(42, "network"), sim.NewRNG(42, "network")
	want := []uint64{0x72534e524783be1d, 0x5256709f2c857606, 0xdfe789f84614c3ef}
	for i, w := range want {
		x, y := a.Uint64(), b.Uint64()
		if x != y || x != w {
			t.Fatalf("draw %d: %x / %x, want %x", i, x, y, w)
		}
	}
}

func TestRNGStreamsAreIndependent(t *testing.T) {
	net, work, other := sim.NewRNG(42, "network"), sim.NewRNG(42, "workload"), sim.NewRNG(43, "network")
	n := net.Uint64()
	if n == work.Uint64() || n == other.Uint64() {
		t.Fatal("different streams or seeds gave the same first draw")
	}
}

func TestRNGRanges(t *testing.T) {
	r := sim.NewRNG(1, "ranges")
	counts := make([]int, 4)
	for i := 0; i < 4000; i++ {
		counts[r.IntN(4)]++
		if v := r.Range(10, 20); v < 10 || v > 20 {
			t.Fatalf("Range out of bounds: %d", v)
		}
		if f := r.Float64(); f < 0 || f >= 1 {
			t.Fatalf("Float64 out of bounds: %v", f)
		}
	}
	for i, c := range counts {
		if c < 800 || c > 1200 {
			t.Fatalf("IntN(4) bucket %d has %d of 4000: not uniform", i, c)
		}
	}
	if r.Range(7, 7) != 7 || r.Range(9, 3) != 9 {
		t.Fatal("degenerate Range")
	}
}

// Chance must consume exactly one draw whatever p is, so changing a drop
// probability in the config does not shift every later random number.
func TestChanceAlwaysDrawsOnce(t *testing.T) {
	for _, p := range []float64{0, 0.5, 1} {
		a, b := sim.NewRNG(5, "x"), sim.NewRNG(5, "x")
		a.Chance(p)
		b.Uint64()
		if a.Uint64() != b.Uint64() {
			t.Fatalf("Chance(%v) did not consume exactly one draw", p)
		}
	}
}
