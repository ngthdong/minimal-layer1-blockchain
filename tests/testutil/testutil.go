package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
)

// RepoRoot walks up from the test's working directory to the folder that
// holds both src/ and config/, so tests never count "../" by hand.
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if isDir(filepath.Join(dir, "src")) && isDir(filepath.Join(dir, "config")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("project root (with src/ and config/) not found")
		}
		dir = parent
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// LoadConfig loads config/<name>.json.
func LoadConfig(t *testing.T, name string) *simconfig.Config {
	t.Helper()
	cfg, err := simconfig.Load(filepath.Join(RepoRoot(t), "config", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Run executes a scenario, saves its log as logs/<logName>.log and returns
// the result and the raw log bytes.
func Run(t *testing.T, cfg *simconfig.Config, logName string) (*sim.Result, []byte) {
	t.Helper()
	var buf bytes.Buffer
	res, err := sim.Run(cfg, &buf)
	if err != nil {
		t.Fatalf("run %s: %v", cfg.Name, err)
	}
	path := filepath.Join(RepoRoot(t), "logs", logName+".log")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	assertOnlyByzantineBlocked(t, cfg, buf.Bytes())
	return res, buf.Bytes()
}

// CountLines counts log lines that contain substr.
func CountLines(log []byte, substr string) int {
	return bytes.Count(log, []byte(substr))
}

// AssertSafety fails if two honest nodes finalized different blocks at the
// same height.
func AssertSafety(t *testing.T, res *sim.Result) {
	t.Helper()
	if !res.Agreed {
		t.Fatal("SAFETY VIOLATION: honest nodes finalized conflicting blocks")
	}
}

// AssertProgress fails unless every honest node finalized at least height h.
func AssertProgress(t *testing.T, res *sim.Result, h uint64) {
	t.Helper()
	for _, id := range res.Honest {
		if got := res.Nodes[id].Ledger.Height(); got < h {
			t.Fatalf("node %d finalized height %d, want >= %d", id, got, h)
		}
	}
}

// assertOnlyByzantineBlocked is checked on EVERY run: a peer guard may block
// a Byzantine node, never an honest one (that would cut it out of the
// quorums and could stall the chain).
func assertOnlyByzantineBlocked(t *testing.T, cfg *simconfig.Config, log []byte) {
	t.Helper()
	byz := map[string]bool{}
	for _, b := range cfg.Byzantine {
		byz[fmt.Sprintf("peer=%02d", b.Node)] = true
	}
	for _, line := range bytes.Split(log, []byte("\n")) {
		if !bytes.Contains(line, []byte(" ev=BLOCK ")) {
			continue
		}
		i := bytes.Index(line, []byte("peer="))
		if i < 0 || !byz[string(line[i:i+7])] {
			t.Fatalf("an honest peer was blocked: %s", line)
		}
	}
}
