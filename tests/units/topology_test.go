package unit

import (
	"os"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// The ring of the scenario file: 10 nodes, 4 neighbours each, both ways.
func TestRingTopologyFile(t *testing.T) {
	adj, err := network.LoadTopology("../../config/topology_ring10.txt", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	for id := types.NodeID(0); id < 10; id++ {
		if len(adj[id]) != 4 {
			t.Fatalf("node %d has %d neighbours, want 4", id, len(adj[id]))
		}
		for _, p := range adj[id] {
			if !containsID(adj[p], id) {
				t.Fatalf("edge %d-%d is one-way", id, p)
			}
		}
	}
	// Any 3 crashed nodes leave the ring connected (connectivity 4 = f+1).
	if _, err := network.LoadTopology("../../config/topology_ring10.txt", 10, []types.NodeID{3, 6, 9}); err != nil {
		t.Fatal(err)
	}
}

func containsID(ids []types.NodeID, id types.NodeID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestBadTopologies(t *testing.T) {
	cases := map[string]string{
		"0 1 2":    "want",
		"0 x":      "integers",
		"0 4":      "integers", // n = 4: node 4 does not exist
		"2 2":      "self-loop",
		"0 1\n1 0": "twice",
	}
	for text, want := range cases {
		if _, err := network.ParseTopology(strings.NewReader(text), 4); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: err = %v, want %q", text, err, want)
		}
	}
}

// A line 0-1-2-3 is connected, but not without node 1 or 2: a faulty node
// in the middle would cut the honest nodes apart.
func TestTopologyMustSurviveTheFaultyNodes(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/line.txt"
	writeFile(t, path, "0 1\n1 2\n2 3\n")
	if _, err := network.LoadTopology(path, 4, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := network.LoadTopology(path, 4, []types.NodeID{1}); err == nil ||
		!strings.Contains(err.Error(), "without the faulty nodes") {
		t.Fatalf("err = %v", err)
	}
	writeFile(t, path, "0 1\n2 3\n")
	if _, err := network.LoadTopology(path, 4, nil); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("disconnected graph accepted: %v", err)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
