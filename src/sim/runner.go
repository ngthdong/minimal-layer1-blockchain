package sim

import (
	"fmt"
	"io"
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/consensus"
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
	privs := make([]crypto.PrivateKey, cfg.NumValidators)
	for i := range privs {
		res.Validators[i], privs[i] = crypto.KeyFromSeed(cfg.Seed, fmt.Sprintf("validator/%d", i))
	}
	vals, err := consensus.NewValidatorSet(res.Validators)
	if err != nil {
		return nil, err
	}
	ccfg := consensus.Config{
		TimeoutProposeMs:   cfg.Consensus.TimeoutProposeMs,
		TimeoutPrevoteMs:   cfg.Consensus.TimeoutPrevoteMs,
		TimeoutPrecommitMs: cfg.Consensus.TimeoutPrecommitMs,
		TimeoutDeltaMs:     cfg.Consensus.TimeoutDeltaMs,
		MaxBlockTxs:        cfg.Consensus.MaxBlockTxs,
	}

	byz := make(map[int]bool, len(cfg.Byzantine))
	for _, b := range cfg.Byzantine {
		byz[b.Node] = true
	}
	mesh := network.FullMesh(cfg.NumValidators)
	for i := 0; i < cfg.NumValidators; i++ {
		id := types.NodeID(i)
		nd := node.New(id, cfg.ChainID, reg, net, log, sched)
		signer := consensus.NewVoteSigner(id, privs[i], cfg.ChainID)
		nd.Engine = consensus.NewEngine(id, cfg.ChainID, vals, signer, ccfg, nd)
		net.Register(id, nd.HandleMessage)
		net.SetPeers(id, mesh[id])
		res.Nodes = append(res.Nodes, nd)
		if !byz[i] {
			res.Honest = append(res.Honest, id)
		}
	}

	// Faults, scheduled before anything else so that a crash at time t comes
	// first among the events of t (from_ms = 0: the node never starts).
	for _, b := range cfg.Byzantine {
		switch b.Behavior {
		case "crash":
			sched.At(b.FromMs, res.Nodes[b.Node].Crash)
		default:
			return nil, fmt.Errorf("byzantine behavior %q is not implemented", b.Behavior)
		}
	}

	// Client workload: every submission is an event at its planned time.
	for _, s := range workload.Plan(cfg, accounts, NewRNG(cfg.Seed, "workload")) {
		to, tx := s.To, s.Tx
		sched.At(s.AtMs, func() { submit(res.Nodes, to, tx) })
	}

	// Every node starts height 1 at time 0, in NodeID order.
	for _, nd := range res.Nodes {
		sched.At(0, nd.Start)
	}

	sched.Run(cfg.MaxTimeMs, func() bool { return allReached(res, cfg.MaxHeight) })
	res.EndTimeMs = sched.Now()

	res.Agreed, res.FinalStateHash = summarize(res, cfg.MaxHeight)

	for _, nd := range res.Nodes {
		log.Log(int(nd.ID), logging.EvFinal, nd.Ledger.Height(), 0,
			"state", nd.State.Hash().Short(),
			"mempool", strconv.Itoa(nd.Mempool.Size()))
	}
	return res, log.Flush()
}

// submit hands tx to node `to` or, like a client whose server does not
// answer, to the next node (in NodeID order) that is still up. Otherwise a
// tx lost on a crashed node leaves a nonce gap, and every later tx of that
// account would wait forever.
func submit(nodes []*node.Node, to types.NodeID, tx types.Tx) {
	for i := range nodes {
		if nd := nodes[(int(to)+i)%len(nodes)]; !nd.Crashed() {
			nd.SubmitTx(tx)
			return
		}
	}
}

// allReached reports whether every honest node finalized at least height h.
func allReached(res *Result, h uint64) bool {
	for _, id := range res.Honest {
		if res.Nodes[id].Ledger.Height() < h {
			return false
		}
	}
	return true
}

// summarize checks SAFETY over the honest ledgers: at every height that two
// honest nodes both finalized, the block hashes must be equal. It returns
// that verdict and the state hash committed by the block at maxHeight (zero
// if no honest node got there). Nodes may stop at slightly different heights,
// so the state is read from the ledger, not from each node's live state.
func summarize(res *Result, maxHeight uint64) (bool, types.Hash) {
	agreed := true
	var ref []types.Hash // ref[h-1] = first block hash seen at height h
	var final types.Hash
	for _, id := range res.Honest {
		l := res.Nodes[id].Ledger
		for h := uint64(1); h <= l.Height(); h++ {
			bh, _ := l.BlockHash(h)
			if h > uint64(len(ref)) {
				ref = append(ref, bh)
			} else if ref[h-1] != bh {
				agreed = false
			}
			if h == maxHeight {
				e, _ := l.Get(h)
				final = e.Block.Header.StateHash
			}
		}
	}
	return agreed, final
}
