package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
)

const t1Path = "../../config/t1_normal.json"

func TestLoadT1Config(t *testing.T) {
	c, err := simconfig.Load(t1Path)
	if err != nil {
		t.Fatal(err)
	}
	if c.NumValidators != 10 || c.F() != 3 || c.ChainID != "ml1-chain" {
		t.Fatalf("n=%d f=%d chain=%q", c.NumValidators, c.F(), c.ChainID)
	}
}

// mutate loads t1, applies change and validates the result.
func mutate(t *testing.T, change func(c *simconfig.Config)) error {
	t.Helper()
	c, err := simconfig.Load(t1Path)
	if err != nil {
		t.Fatal(err)
	}
	change(c)
	return c.Validate()
}

func TestValidateRejectsBadConfigs(t *testing.T) {
	cases := map[string]func(c *simconfig.Config){
		"n not 3f+1":       func(c *simconfig.Config) { c.NumValidators = 9 },
		"n below 8":        func(c *simconfig.Config) { c.NumValidators = 7 },
		"too many byz":     func(c *simconfig.Config) { c.Byzantine = make([]simconfig.ByzantineSpec, 4) },
		"byz out of range": func(c *simconfig.Config) { c.Byzantine = []simconfig.ByzantineSpec{{Node: 10}} },
		"byz twice": func(c *simconfig.Config) {
			c.Byzantine = []simconfig.ByzantineSpec{{Node: 2}, {Node: 2}}
		},
		"no chain id":      func(c *simconfig.Config) { c.ChainID = "" },
		"bad probability":  func(c *simconfig.Config) { c.Network.PreGST.DropProb = 1.5 },
		"delta < min":      func(c *simconfig.Config) { c.Network.DeltaMs = 5 },
		"zero timeout":     func(c *simconfig.Config) { c.Consensus.TimeoutPrevoteMs = 0 },
		"no accounts":      func(c *simconfig.Config) { c.Workload.Accounts = nil },
		"reserved account": func(c *simconfig.Config) { c.Workload.Accounts = []string{"sys"} },
		"slash in account": func(c *simconfig.Config) { c.Workload.Accounts = []string{"Al/ice"} },
		"bad topology":     func(c *simconfig.Config) { c.Network.Topology = "ring" },
		"no refill":        func(c *simconfig.Config) { c.Network.RateLimit.RefillPerMs = 0 },
		"unknown inject": func(c *simconfig.Config) {
			c.Workload.Inject = []simconfig.InjectTx{{Kind: "teleport", Account: "Alice"}}
		},
		"inject account": func(c *simconfig.Config) {
			c.Workload.Inject = []simconfig.InjectTx{{Kind: "bad_signature", Account: "Mallory"}}
		},
		"foreign_key, 1 account": func(c *simconfig.Config) {
			c.Workload.Accounts = []string{"Alice"}
			c.Workload.Inject = []simconfig.InjectTx{{Kind: "foreign_key", Account: "Alice"}}
		},
	}
	for name, change := range cases {
		if mutate(t, change) == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if err := mutate(t, func(c *simconfig.Config) {}); err != nil {
		t.Fatalf("unchanged t1 rejected: %v", err)
	}
}

// A typo in a field name must fail instead of silently using the zero value.
func TestLoadRejectsUnknownFields(t *testing.T) {
	raw, err := os.ReadFile(t1Path)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"drop_prob"`, `"drop_prop"`, 1)
	path := filepath.Join(t.TempDir(), "typo.json")
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := simconfig.Load(path); err == nil {
		t.Fatal("config with unknown field accepted")
	}
}
