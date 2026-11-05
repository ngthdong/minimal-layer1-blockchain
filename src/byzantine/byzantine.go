// Package byzantine holds the faulty behaviours a scenario can give to at
// most f validators. A behaviour only changes what ITS node sends, through
// the network's Outbound hook: it cannot touch another node's memory, and
// whatever it signs, it signs with its own key (it cannot forge honest
// signatures, as the lab assumes).
package byzantine

import (
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/crypto"
	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

// Identity is what a Byzantine validator owns: its id, its OWN key (never
// another's), the chain it signs for, and the size of the validator set.
type Identity struct {
	ID      types.NodeID
	Priv    crypto.PrivateKey
	ChainID string
	N       int
}

// Behavior decides what its node really sends when the honest code of that
// node sends msg to peer `to`.
type Behavior interface {
	Outgoing(to types.NodeID, msg types.Message) []types.Message
}

// Honest sends exactly what the honest code sends. It exists to show that
// the hook itself changes nothing.
type Honest struct{}

func (Honest) Outgoing(_ types.NodeID, m types.Message) []types.Message { return []types.Message{m} }

// Silent sends nothing at all — no proposal, vote, body or relay — while it
// still receives and runs the protocol. To the others it looks like a crash,
// but it is a decision: it withholds its share of every quorum.
type Silent struct{}

func (Silent) Outgoing(types.NodeID, types.Message) []types.Message { return nil }

// New returns the behaviour named in a scenario file for validator self
// ("crash" is not one: it also stops the node's inputs, see node.Crash).
func New(name string, self Identity) (Behavior, error) {
	switch name {
	case "honest":
		return Honest{}, nil
	case "silent":
		return Silent{}, nil
	case "equivocate":
		return Equivocate{Self: self}, nil
	case "invalid_proposal":
		return &InvalidProposal{}, nil
	}
	return nil, fmt.Errorf("byzantine behavior %q is not implemented", name)
}

// BlockCorrupter is implemented by behaviours that tamper with the blocks
// their own node builds as proposer (the runner installs it with
// node.SetBlockHook). The bad block is then signed with the node's own key
// and served on request like any other: what is wrong is the block itself,
// not a signature, so only executing it can tell.
type BlockCorrupter interface {
	CorruptBlock(b types.Block) types.Block
}

// BlockHook turns c into a node block hook, active from fromMs on.
func BlockHook(c BlockCorrupter, fromMs uint64, now func() uint64) func(types.Block) types.Block {
	return func(b types.Block) types.Block {
		if now() < fromMs {
			return b
		}
		return c.CorruptBlock(b)
	}
}

// Hook turns b into the node's network send hook, active from fromMs on:
// before that the node behaves honestly.
func Hook(b Behavior, fromMs uint64, now func() uint64) network.Outbound {
	return func(to types.NodeID, msg types.Message) []types.Message {
		if now() < fromMs {
			return []types.Message{msg}
		}
		return b.Outgoing(to, msg)
	}
}
