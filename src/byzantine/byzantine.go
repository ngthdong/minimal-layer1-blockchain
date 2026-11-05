// Package byzantine holds the faulty behaviours a scenario can give to at
// most f validators. A behaviour only changes what ITS node sends, through
// the network's Outbound hook: it cannot touch another node's memory, and
// whatever it signs, it signs with its own key (it cannot forge honest
// signatures, as the lab assumes).
package byzantine

import (
	"fmt"

	"github.com/ngthdong/minimal-layer1-blockchain/network"
	"github.com/ngthdong/minimal-layer1-blockchain/types"
)

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

// New returns the behaviour named in a scenario file ("crash" is not one:
// it also stops the node's inputs, see node.Crash).
func New(name string) (Behavior, error) {
	switch name {
	case "honest":
		return Honest{}, nil
	case "silent":
		return Silent{}, nil
	}
	return nil, fmt.Errorf("byzantine behavior %q is not implemented", name)
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
