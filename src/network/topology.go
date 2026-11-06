package network

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// LoadTopology reads the undirected edge list of an n-node network:
//
//	# a comment; blank lines are ignored
//	0 1
//	0 2
//
// One edge "a b" per line, 0 <= a, b < n, a != b, each edge once. Two checks
// keep a scenario meaningful: the whole graph is connected, and it stays
// connected without the faulty nodes. Otherwise two honest nodes could talk
// only through a Byzantine relay, which may simply drop everything: the run
// would test the topology, not the protocol.
func LoadTopology(path string, n int, faulty []types.NodeID) (map[types.NodeID][]types.NodeID, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	adj, err := ParseTopology(f, n)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := connected(adj, n, nil); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := connected(adj, n, faulty); err != nil {
		return nil, fmt.Errorf("%s: without the faulty nodes %v: %w", path, faulty, err)
	}
	return adj, nil
}

// ParseTopology parses an edge list (see LoadTopology) into sorted
// neighbour lists, both directions.
func ParseTopology(r io.Reader, n int) (map[types.NodeID][]types.NodeID, error) {
	adj := make(map[types.NodeID][]types.NodeID, n)
	seen := map[[2]int]bool{}
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		f := strings.Fields(text)
		if len(f) != 2 {
			return nil, fmt.Errorf("line %d: want \"a b\", got %q", line, text)
		}
		a, errA := strconv.Atoi(f[0])
		b, errB := strconv.Atoi(f[1])
		switch {
		case errA != nil || errB != nil || a < 0 || b < 0 || a >= n || b >= n:
			return nil, fmt.Errorf("line %d: nodes must be integers in [0, %d)", line, n)
		case a == b:
			return nil, fmt.Errorf("line %d: self-loop on node %d", line, a)
		case seen[[2]int{min(a, b), max(a, b)}]:
			return nil, fmt.Errorf("line %d: edge %d-%d listed twice", line, a, b)
		}
		seen[[2]int{min(a, b), max(a, b)}] = true
		adj[types.NodeID(a)] = append(adj[types.NodeID(a)], types.NodeID(b))
		adj[types.NodeID(b)] = append(adj[types.NodeID(b)], types.NodeID(a))
	}
	return adj, sc.Err() // SetPeers sorts each list
}

// connected checks that the nodes not in skip form one connected graph.
func connected(adj map[types.NodeID][]types.NodeID, n int, skip []types.NodeID) error {
	out := map[types.NodeID]bool{}
	for _, id := range skip {
		out[id] = true
	}
	start := types.NodeID(0)
	for out[start] {
		start++
	}
	reached, queue := map[types.NodeID]bool{start: true}, []types.NodeID{start}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, w := range adj[v] {
			if !out[w] && !reached[w] {
				reached[w] = true
				queue = append(queue, w)
			}
		}
	}
	for id := types.NodeID(0); int(id) < n; id++ {
		if !out[id] && !reached[id] {
			return fmt.Errorf("node %d cannot reach node %d", start, id)
		}
	}
	return nil
}
