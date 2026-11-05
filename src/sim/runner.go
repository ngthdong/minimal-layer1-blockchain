package sim

import (
	"fmt"
	"io"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

type Result struct {
	Validators     []crypto.PublicKey // index = NodeID
	Honest         []types.NodeID     // validators not listed as Byzantine, ascending
	Agreed         bool               // honest nodes have identical finalized chains
	FinalStateHash types.Hash         // state hash agreed by honest nodes
	EndTimeMs      uint64             // virtual time when the run stopped
}

func Run(cfg *simconfig.Config, logOut io.Writer) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	sched := NewScheduler()
	log := logging.New(logOut, sched.Now)

	res := &Result{Validators: make([]crypto.PublicKey, cfg.NumValidators)}
	byz := make(map[int]bool, len(cfg.Byzantine))
	for _, b := range cfg.Byzantine {
		byz[b.Node] = true
	}
	for i := 0; i < cfg.NumValidators; i++ {
		res.Validators[i], _ = crypto.KeyFromSeed(cfg.Seed, fmt.Sprintf("validator/%d", i))
		if !byz[i] {
			res.Honest = append(res.Honest, types.NodeID(i))
		}
	}

	sched.Run(cfg.MaxTimeMs, nil)
	res.EndTimeMs = sched.Now()

	// Nothing is finalized yet: every honest node trivially agrees on the
	// empty chain and the zero state hash.
	res.Agreed = true
	for i, pub := range res.Validators {
		log.Log(i, logging.EvFinal, 0, 0,
			"pub", fmt.Sprintf("%x", pub[:4]),
			"state", res.FinalStateHash.Short())
	}
	return res, log.Flush()
}
