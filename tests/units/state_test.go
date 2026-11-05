package unit

import (
	"reflect"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/state"
)

func sampleState() *state.State {
	return state.FromMap(map[string][]byte{
		"Alice/message": []byte("hello"),
		"Bob/message":   []byte("hi"),
		"Carol/score":   []byte{0, 7},
	})
}

// Go randomises map iteration order on every loop; the hash must not care.
func TestStateHashIsStable(t *testing.T) {
	want := sampleState().Hash()
	for i := 0; i < 50; i++ {
		if sampleState().Hash() != want {
			t.Fatal("same content gave a different state hash")
		}
	}
}

func TestStateHashCoversContent(t *testing.T) {
	base := sampleState().Hash()
	variants := map[string]map[string][]byte{
		"value changed": {"Alice/message": []byte("hellO"), "Bob/message": []byte("hi"), "Carol/score": {0, 7}},
		"key removed":   {"Alice/message": []byte("hello"), "Bob/message": []byte("hi")},
		"key added": {"Alice/message": []byte("hello"), "Bob/message": []byte("hi"), "Carol/score": {0, 7},
			"Dave/x": []byte("1")},
		"empty value": {"Alice/message": []byte("hello"), "Bob/message": []byte("hi"), "Carol/score": {}},
	}
	for name, m := range variants {
		if state.FromMap(m).Hash() == base {
			t.Fatalf("%s: state hash did not change", name)
		}
	}
}

// Without length prefixes these two states would hash the same bytes.
func TestStateHashKeyValueBoundary(t *testing.T) {
	a := state.FromMap(map[string][]byte{"Alice/ab": []byte("c")})
	b := state.FromMap(map[string][]byte{"Alice/a": []byte("bc")})
	if a.Hash() == b.Hash() {
		t.Fatal("key/value boundary is ambiguous")
	}
}

// A key holding an empty value is not the same state as a missing key.
func TestEmptyValueDiffersFromMissingKey(t *testing.T) {
	empty := state.FromMap(map[string][]byte{"Alice/x": {}})
	if empty.Hash() == state.New().Hash() {
		t.Fatal("empty value and missing key share a hash")
	}
	if state.New().Hash().IsZero() {
		t.Fatal("empty state must have a real hash, not ZeroHash")
	}
}

func TestStateIsIsolatedFromCallers(t *testing.T) {
	src := map[string][]byte{"Alice/message": []byte("hello")}
	s := state.FromMap(src)
	before := s.Hash()

	src["Alice/message"][0] = 'J' // edit the caller's map after FromMap
	v, _ := s.Get("Alice/message")
	v[0] = 'X' // edit the slice returned by Get
	c := s.Clone()

	if s.Hash() != before || c.Hash() != before {
		t.Fatal("state changed through a slice it gave out or received")
	}
	if got, _ := s.Get("Alice/message"); string(got) != "hello" {
		t.Fatalf("value = %q, want hello", got)
	}
	if _, ok := s.Get("Bob/message"); ok {
		t.Fatal("missing key reported as present")
	}
}

func TestSortedKeys(t *testing.T) {
	got := sampleState().SortedKeys()
	want := []string{"Alice/message", "Bob/message", "Carol/score"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedKeys = %v", got)
	}
	if len(state.New().SortedKeys()) != 0 || sampleState().Len() != 3 {
		t.Fatal("Len or empty SortedKeys wrong")
	}
}

func TestOwner(t *testing.T) {
	cases := map[string]string{
		"Alice/message":   "Alice",
		"Alice/a/b":       "Alice",
		"sys/nonce/Alice": "sys",
		"/message":        "",
		"message":         "",
		"":                "",
	}
	for key, want := range cases {
		if got := state.Owner(key); got != want {
			t.Fatalf("Owner(%q) = %q, want %q", key, got, want)
		}
	}
}
