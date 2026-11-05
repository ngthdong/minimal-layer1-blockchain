// Package workload plays the clients (Alice, Bob, ...): it decides, before the
// run starts, which signed transactions are handed to which node and when.
// Crafted bad transactions (T3) and replays (T4) are in inject.go.
package workload

import (
	"fmt"
	"sort"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/state"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Account is one client. Nonce is the last nonce it has used.
type Account struct {
	Name  string
	Pub   crypto.PublicKey
	Priv  crypto.PrivateKey
	Nonce uint64
}

// Accounts derives deterministic keys for the configured names, in config order.
func Accounts(seed uint64, names []string) []*Account {
	out := make([]*Account, len(names))
	for i, name := range names {
		pub, priv := crypto.KeyFromSeed(seed, "account/"+name)
		out[i] = &Account{Name: name, Pub: pub, Priv: priv}
	}
	return out
}

// Registry is the genesis account -> public key map every node starts with.
func Registry(accounts []*Account) state.Registry {
	reg := make(state.Registry, len(accounts))
	for _, a := range accounts {
		reg[a.Name] = a.Pub
	}
	return reg
}

// Sign builds and signs a tx from account a, using the NEXT nonce.
func (a *Account) Sign(chainID, key string, value []byte) types.Tx {
	a.Nonce++
	return a.signAs(crypto.DomainTx, chainID, a.Nonce, key, value)
}

// signAs signs a tx with an explicit nonce under any domain (bad txs use it).
func (a *Account) signAs(domain, chainID string, nonce uint64, key string, value []byte) types.Tx {
	tx := types.Tx{Sender: a.Name, Nonce: nonce, Key: key, Value: value}
	tx.Sig = crypto.Sign(a.Priv, crypto.Context(domain, chainID), tx.SigningBytes())
	return tx
}

// Rand is the part of sim.RNG the workload needs.
type Rand interface{ IntN(n int) int }

// Submission is one tx handed by a client to node To at virtual time AtMs.
type Submission struct {
	AtMs uint64
	To   types.NodeID
	Tx   types.Tx
}

// Plan builds the whole list of client submissions for a run, in time order
// (at equal times, normal txs first).
// Tx i (0-based) is sent at (i+1)*interval_ms by accounts[i mod len] to a
// random node, writing "<name>/message" = "msg-<i>". Everything depends only
// on the config and rng, so the same seed gives the same plan.
func Plan(cfg *simconfig.Config, accounts []*Account, rng Rand) []Submission {
	w := cfg.Workload
	out := make([]Submission, 0, w.TxCount)
	for i := 0; i < w.TxCount; i++ {
		a := accounts[i%len(accounts)]
		tx := a.Sign(cfg.ChainID, a.Name+"/message", []byte(fmt.Sprintf("msg-%d", i)))
		out = append(out, Submission{
			AtMs: uint64(i+1) * w.IntervalMs,
			To:   types.NodeID(rng.IntN(cfg.NumValidators)),
			Tx:   tx,
		})
	}
	// Bad txs draw from rng only after every normal tx: a scenario without
	// injections gets exactly the same plan as before they existed.
	out = append(out, injected(cfg, accounts, out, rng)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].AtMs < out[j].AtMs })
	return out
}
