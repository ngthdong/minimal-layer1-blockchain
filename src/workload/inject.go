package workload

import (
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// injected builds the crafted bad txs of cfg.Workload.Inject (T3). Each kind
// breaks exactly one check, so every node must log exactly that reason:
//
//	bad_signature → bad_signature            (one signature byte flipped)
//	wrong_domain  → wrong_domain             (signed under "VOTE:<chain>")
//	foreign_key   → key_not_owned_by_sender  (writes the next account's key)
//
// A bad tx carries the account's next nonce at its time, i.e. the nonce of
// its next real tx: if any node wrongly accepted it, that real tx would be
// refused later and the final state would change. T3 checks it does not.
func injected(cfg *simconfig.Config, accounts []*Account, plan []Submission, rng Rand) []Submission {
	var out []Submission
	for i, in := range cfg.Workload.Inject {
		idx := accountIndex(accounts, in.Account)
		a := accounts[idx]
		nonce := lastNonceAt(plan, a.Name, in.AtMs) + 1
		key, value := a.Name+"/message", []byte(fmt.Sprintf("inject-%d-%s", i, in.Kind))
		domain := crypto.DomainTx
		switch in.Kind {
		case "wrong_domain":
			domain = crypto.DomainVote
		case "foreign_key":
			key = accounts[(idx+1)%len(accounts)].Name + "/message"
		}
		tx := a.signAs(domain, cfg.ChainID, nonce, key, value)
		if in.Kind == "bad_signature" {
			tx.Sig[0] ^= 0x01
		}
		for k := 0; k < max(in.Repeat, 1); k++ {
			out = append(out, Submission{AtMs: in.AtMs, To: types.NodeID(rng.IntN(cfg.NumValidators)), Tx: tx.Clone()})
		}
	}
	return out
}

func accountIndex(accounts []*Account, name string) int {
	for i, a := range accounts {
		if a.Name == name {
			return i
		}
	}
	panic("workload: unknown account " + name) // simconfig.Validate rules this out
}

// lastNonceAt is the highest nonce of sender among the normal txs sent up to
// and including time at (0 if none).
func lastNonceAt(plan []Submission, sender string, at uint64) uint64 {
	var n uint64
	for _, s := range plan {
		if s.AtMs <= at && s.Tx.Sender == sender && s.Tx.Nonce > n {
			n = s.Tx.Nonce
		}
	}
	return n
}
