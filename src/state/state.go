package state

import (
	"sort"
	"strings"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/encoding"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// stateHashTag versions the state commitment. Changing it changes every
// header's StateHash and therefore every block hash.
const stateHashTag = "ml1b/state/v1"

type State struct {
	kv map[string][]byte
}

func New() *State { return &State{kv: make(map[string][]byte)} }

// FromMap builds a state from initial data (genesis, tests). The map is
// copied, so later changes to m do not affect the state.
func FromMap(m map[string][]byte) *State {
	s := New()
	for k, v := range m {
		s.kv[k] = cloneValue(v)
	}
	return s
}

func (s *State) Get(key string) ([]byte, bool) {
	v, ok := s.kv[key]
	if !ok {
		return nil, false
	}
	return cloneValue(v), true
}

func (s *State) Len() int { return len(s.kv) }

func (s *State) Clone() *State {
	c := &State{kv: make(map[string][]byte, len(s.kv))}
	for k, v := range s.kv {
		c.kv[k] = cloneValue(v)
	}
	return c
}

func (s *State) SortedKeys() []string {
	keys := make([]string, 0, len(s.kv))
	for k := range s.kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Hash is the state commitment stored in every block header:
//
//	SHA-256( tag || count || key_1 || value_1 || ... || key_n || value_n )
//
// with keys in ascending order and every key and value length-prefixed.
// Two nodes with the same key/value pairs always get the same hash, no matter
// in which order the keys were written.
func (s *State) Hash() types.Hash {
	w := encoding.NewWriter().String(stateHashTag).U32(uint32(len(s.kv)))
	for _, k := range s.SortedKeys() {
		w.String(k).Bytes(s.kv[k])
	}
	return types.Hash(crypto.Hash(w.Out()))
}

// Owner returns the owner part of a key ("Alice" for "Alice/message"), or ""
// if the key has no owner part.
func Owner(key string) string {
	if i := strings.IndexByte(key, '/'); i > 0 {
		return key[:i]
	}
	return ""
}

func cloneValue(v []byte) []byte {
	if v == nil {
		return []byte{}
	}
	return append([]byte(nil), v...)
}
