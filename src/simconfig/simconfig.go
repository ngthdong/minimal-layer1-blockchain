package simconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Name          string          `json:"name"`
	ChainID       string          `json:"chain_id"`
	Seed          uint64          `json:"seed"`
	NumValidators int             `json:"num_validators"` // n = 3f+1, at least 8
	MaxHeight     uint64          `json:"max_height"`     // stop when every honest node reaches it
	MaxTimeMs     uint64          `json:"max_time_ms"`    // hard stop in virtual time
	Network       NetworkConfig   `json:"network"`
	Consensus     ConsensusConfig `json:"consensus"`
	Workload      WorkloadConfig  `json:"workload"`
	Byzantine     []ByzantineSpec `json:"byzantine"`
}

type NetworkConfig struct {
	GSTMs        uint64       `json:"gst_ms"`       // global stabilization time
	DeltaMs      uint64       `json:"delta_ms"`     // max delay after GST
	MinDelayMs   uint64       `json:"min_delay_ms"` // min delay, always
	PreGST       PreGSTConfig `json:"pre_gst"`
	RateLimit    RateLimit    `json:"rate_limit"`
	PeerBlock    PeerBlock    `json:"peer_block"`
	Topology     string       `json:"topology"`      // "full" or "file"
	TopologyFile string       `json:"topology_file"` // relative to the config file
}

type PreGSTConfig struct {
	DropProb   float64 `json:"drop_prob"`
	DupProb    float64 `json:"dup_prob"`
	MaxDelayMs uint64  `json:"max_delay_ms"`
}

type RateLimit struct {
	Capacity    int `json:"capacity"`      // token bucket size per outbound link
	RefillPerMs int `json:"refill_per_ms"` // tokens added per virtual ms
	QueueLimit  int `json:"queue_limit"`   // queued messages before DROP reason=rate
}

type PeerBlock struct {
	WindowMs uint64 `json:"window_ms"`
	MaxMsgs  int    `json:"max_msgs"`
	BlockMs  uint64 `json:"block_ms"`
}

type ConsensusConfig struct {
	TimeoutProposeMs   uint64 `json:"timeout_propose_ms"`
	TimeoutPrevoteMs   uint64 `json:"timeout_prevote_ms"`
	TimeoutPrecommitMs uint64 `json:"timeout_precommit_ms"`
	TimeoutDeltaMs     uint64 `json:"timeout_delta_ms"` // added per round
	MaxBlockTxs        int    `json:"max_block_txs"`
}

type WorkloadConfig struct {
	Accounts   []string   `json:"accounts"`
	TxCount    int        `json:"tx_count"`
	IntervalMs uint64     `json:"interval_ms"`
	Inject     []InjectTx `json:"inject"` // bad_signature, wrong_domain, foreign_key, replay
}

type InjectTx struct {
	Kind    string `json:"kind"`
	AtMs    uint64 `json:"at_ms"`
	Account string `json:"account"`
	Repeat  int    `json:"repeat"` // copies, each to a random node (0 = 1)
}

// ByzantineSpec: silent, crash, equivocate, invalid_proposal, spam.
type ByzantineSpec struct {
	Node     int    `json:"node"`
	Behavior string `json:"behavior"`
	FromMs   uint64 `json:"from_ms"`
}

// F is the fault threshold for n = 3f+1.
// InjectKinds are the crafted bad transactions the workload can add (T3).
var InjectKinds = map[string]bool{"bad_signature": true, "wrong_domain": true, "foreign_key": true}

func (c *Config) F() int { return (c.NumValidators - 1) / 3 }

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Network.TopologyFile != "" && !filepath.IsAbs(c.Network.TopologyFile) {
		c.Network.TopologyFile = filepath.Join(filepath.Dir(path), c.Network.TopologyFile)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) Validate() error {
	n, nw, cs := c.NumValidators, c.Network, c.Consensus
	switch {
	case c.Name == "" || strings.ContainsAny(c.Name, " /\\"):
		return fmt.Errorf("name %q must be non-empty without spaces or slashes", c.Name)
	case c.ChainID == "":
		return fmt.Errorf("chain_id is required")
	case n < 8 || (n-1)%3 != 0:
		return fmt.Errorf("num_validators=%d must be 3f+1 and at least 8", n)
	case c.MaxHeight == 0 || c.MaxTimeMs == 0:
		return fmt.Errorf("max_height and max_time_ms must be > 0")
	case nw.DeltaMs < nw.MinDelayMs || nw.PreGST.MaxDelayMs < nw.MinDelayMs:
		return fmt.Errorf("delta_ms and pre_gst.max_delay_ms must be >= min_delay_ms")
	case !isProb(nw.PreGST.DropProb) || !isProb(nw.PreGST.DupProb):
		return fmt.Errorf("pre_gst probabilities must be in [0, 1]")
	case nw.Topology != "full" && nw.Topology != "file":
		return fmt.Errorf("topology must be \"full\" or \"file\"")
	case nw.Topology == "file" && nw.TopologyFile == "":
		return fmt.Errorf("topology \"file\" needs topology_file")
	case cs.TimeoutProposeMs == 0 || cs.TimeoutPrevoteMs == 0 || cs.TimeoutPrecommitMs == 0:
		return fmt.Errorf("consensus timeouts must be > 0")
	case cs.MaxBlockTxs <= 0:
		return fmt.Errorf("max_block_txs must be > 0")
	case len(c.Workload.Accounts) == 0:
		return fmt.Errorf("workload needs at least one account")
	case len(c.Byzantine) > c.F():
		return fmt.Errorf("%d byzantine nodes exceeds f=%d", len(c.Byzantine), c.F())
	}
	seenAcc := map[string]bool{}
	for _, a := range c.Workload.Accounts {
		if a == "" || a == "sys" || strings.ContainsAny(a, "/ =") || seenAcc[a] {
			return fmt.Errorf("account %q is empty, reserved, duplicated or has '/', ' ' or '='", a)
		}
		seenAcc[a] = true
	}
	for _, in := range c.Workload.Inject {
		switch {
		case !InjectKinds[in.Kind]:
			return fmt.Errorf("inject kind %q is unknown", in.Kind)
		case !seenAcc[in.Account]:
			return fmt.Errorf("inject account %q is not in workload.accounts", in.Account)
		case in.Kind == "foreign_key" && len(c.Workload.Accounts) < 2:
			return fmt.Errorf("inject foreign_key needs a second account to steal a key from")
		case in.Repeat < 0:
			return fmt.Errorf("inject repeat must be >= 0")
		}
	}
	seenNode := map[int]bool{}
	for _, b := range c.Byzantine {
		if b.Node < 0 || b.Node >= n || seenNode[b.Node] {
			return fmt.Errorf("byzantine node %d is out of range or listed twice", b.Node)
		}
		seenNode[b.Node] = true
	}
	return nil
}

func isProb(p float64) bool { return p >= 0 && p <= 1 }
