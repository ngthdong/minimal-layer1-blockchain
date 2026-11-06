package byzantine

import "github.com/ngthdong/minimal-layer1-blockchain/types"

// spamCopies is how many times a spammer sends each of its messages.
const spamCopies = 20

// Spam floods its peers: every message its honest code sends goes out
// spamCopies times. The copies are identical, so they cost the receivers
// only bandwidth and de-duplication, never a double count, but they push
// the spammer far over peer_block.max_msgs: every peer BLOCKs it, and while
// blocked its real votes are lost too. That is the point: one Byzantine
// node may waste its own share of the quorum, not the honest nodes' time.
type Spam struct{}

func (Spam) Outgoing(_ types.NodeID, m types.Message) []types.Message {
	out := make([]types.Message, spamCopies)
	for i := range out {
		out[i] = m
	}
	return out
}
