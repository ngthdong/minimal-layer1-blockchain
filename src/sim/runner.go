package sim

import (
	"fmt"
	"io"
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/logging"
	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/node"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
	"github.com/ngthdong/minimal-layer1-blockchain/workload"
)

// Result summarises one run for tests and the CLI.
type Result struct {
	Validators     []crypto.PublicKey // index = NodeID
	Nodes          []*node.Node       // index = NodeID
	Honest         []types.NodeID     // validators not listed as Byzantine, ascending
	Agreed         bool               // honest nodes have identical finalized chains
	FinalStateHash types.Hash         // state hash agreed by honest nodes
	EndTimeMs      uint64             // virtual time when the run stopped
}

// Run executes one scenario and writes its event log to logOut.
// Everything is built in NodeID order and scheduled from config + seed only,
// so the same config always produces the same log.
func Run(cfg *simconfig.Config, logOut io.Writer) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	sched := NewScheduler()
	log := logging.New(logOut, sched.Now)
	net := network.New(cfg.Network, sched, NewRNG(cfg.Seed, "network"), log)

	accounts := workload.Accounts(cfg.Seed, cfg.Workload.Accounts)
	reg := workload.Registry(accounts)

	res := &Result{Validators: make([]crypto.PublicKey, cfg.NumValidators)}
	byz := make(map[int]bool, len(cfg.Byzantine))
	for _, b := range cfg.Byzantine {
		byz[b.Node] = true
	}
	mesh := network.FullMesh(cfg.NumValidators)
	for i := 0; i < cfg.NumValidators; i++ {
		id := types.NodeID(i)
		res.Validators[i], _ = crypto.KeyFromSeed(cfg.Seed, fmt.Sprintf("validator/%d", i))
		nd := node.New(id, cfg.ChainID, reg, net, log)
		net.Register(id, nd.HandleMessage)
		net.SetPeers(id, mesh[id])
		res.Nodes = append(res.Nodes, nd)
		if !byz[i] {
			res.Honest = append(res.Honest, id)
		}
	}

	// Client workload: every submission is an event at its planned time.
	for _, s := range workload.Plan(cfg, accounts, NewRNG(cfg.Seed, "workload")) {
		nd, tx := res.Nodes[s.To], s.Tx
		sched.At(s.AtMs, func() { nd.SubmitTx(tx) })
	}

	sched.Run(cfg.MaxTimeMs, nil)
	res.EndTimeMs = sched.Now()

	// No consensus yet: nothing is finalized, so every honest node
	// trivially agrees on the empty chain.
	res.Agreed = true
	if len(res.Honest) > 0 {
		res.FinalStateHash = res.Nodes[res.Honest[0]].State.Hash()
	}
	for _, nd := range res.Nodes {
		log.Log(int(nd.ID), logging.EvFinal, nd.Ledger.Height(), 0,
			"state", nd.State.Hash().Short(),
			"mempool", strconv.Itoa(nd.Mempool.Size()))
	}
	return res, log.Flush()
}
