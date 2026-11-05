package node

import (
	"strconv"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
)

// Start enters height 1, unless the node crashed before it could start.
func (n *Node) Start() {
	if !n.crashed {
		n.Engine.StartHeight(1)
	}
}

// Crash stops the node for good (fault "crash" of the scenario file): from
// now on it ignores every input — network messages, client txs and its own
// timers — so it never sends anything again. Messages it sent before the
// crash are already in flight and still arrive.
func (n *Node) Crash() {
	if n.crashed {
		return
	}
	n.crashed = true
	n.net.Down(n.ID)
	var h uint64
	var r int32
	if n.Engine != nil { // nil in network-only tests
		h, r = n.Engine.Height, n.Engine.Round
	}
	n.log.Log(int(n.ID), logging.EvCrash, h, r, "finalized", strconv.FormatUint(n.Ledger.Height(), 10))
}

// Crashed reports whether Crash was called.
func (n *Node) Crashed() bool { return n.crashed }
