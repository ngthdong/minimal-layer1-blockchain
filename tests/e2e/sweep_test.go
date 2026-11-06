package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/tests/testutil"
)

// TestSeedSweep runs EVERY scenario of config/ with several seeds: the
// file's own seed and the next ones (3 in total, or $SWEEP_SEEDS). A new
// seed is a new schedule of delays, drops, client targets and so on: it
// finds what one lucky seed hides. Each run must be safe, reach max_height
// on every honest node, carry valid commit certificates, block no honest
// peer, and give the same log when run again.
func TestSeedSweep(t *testing.T) {
	seeds := 3
	if s, err := strconv.Atoi(os.Getenv("SWEEP_SEEDS")); err == nil && s > 0 {
		seeds = s
	}
	files, err := filepath.Glob(filepath.Join(testutil.RepoRoot(t), "config", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenario files: %v", err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		for k := 0; k < seeds; k++ {
			t.Run(fmt.Sprintf("%s/seed+%d", name, k), func(t *testing.T) {
				t.Parallel() // runs share nothing: one scheduler and RNG each
				cfg := testutil.LoadConfig(t, name)
				cfg.Seed += uint64(k)
				res, log := testutil.RunInMemory(t, cfg)
				testutil.AssertSafety(t, res)
				testutil.AssertProgress(t, res, cfg.MaxHeight)
				assertCertificates(t, res)
				if k == 0 { // the file's own seed: also byte-identical when rerun
					if _, again := testutil.RunInMemory(t, cfg); !bytes.Equal(log, again) {
						t.Fatal("same seed, different log")
					}
				}
			})
		}
	}
}
