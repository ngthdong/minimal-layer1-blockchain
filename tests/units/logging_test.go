package unit

import (
	"bytes"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/logging"
)

func TestLogLineFormatIsFixed(t *testing.T) {
	var buf bytes.Buffer
	now := uint64(1234)
	l := logging.New(&buf, func() uint64 { return now })
	l.Log(3, logging.EvPrevote, 5, 0, "block", "9f3e12ab", "from", "07")
	now = 2000
	l.Log(10, logging.EvCommit, 6, -1)
	if err := l.Flush(); err != nil {
		t.Fatal(err)
	}
	want := "t=0000001234 node=03 ev=PREVOTE       h=5 r=0 block=9f3e12ab from=07\n" +
		"t=0000002000 node=10 ev=COMMIT        h=6 r=-1\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestLogRejectsAmbiguousTokens(t *testing.T) {
	l := logging.New(&bytes.Buffer{}, func() uint64 { return 0 })
	for name, kv := range map[string][]string{
		"odd":   {"reason"},
		"space": {"reason", "bad signature"},
		"equal": {"a=b", "c"},
		"empty": {"reason", ""},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: expected panic", name)
				}
			}()
			l.Log(0, logging.EvReject, 1, 0, kv...)
		}()
	}
}
