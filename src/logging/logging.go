package logging

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	// network
	EvSend    = "SEND"
	EvRecv    = "RECV"
	EvDrop    = "DROP"
	EvDup     = "DUP"
	EvBlock   = "BLOCK"
	EvUnblock = "UNBLOCK"
	// consensus
	EvNewRound     = "NEW_ROUND"
	EvPropose      = "PROPOSE"
	EvHeaderAccept = "HEADER_ACCEPT"
	EvBodyAccept   = "BODY_ACCEPT"
	EvPrevote      = "PREVOTE"
	EvPrecommit    = "PRECOMMIT"
	EvLock         = "LOCK"
	EvUnlock       = "UNLOCK"
	EvTimeout      = "TIMEOUT"
	EvCommit       = "COMMIT"
	EvEquivocation = "EQUIVOCATION"
	EvReject       = "REJECT"
	// run summary
	EvFinal = "FINAL"
)

type Logger struct {
	w     *bufio.Writer
	clock func() uint64
	err   error
}

func New(w io.Writer, clock func() uint64) *Logger {
	return &Logger{w: bufio.NewWriter(w), clock: clock}
}

// Log writes one line:
//
//	t=0000001234 node=03 ev=PREVOTE       h=5 r=0 k1=v1 k2=v2
//
// kv is a list of key/value pairs written in the order given. Keys and values
// must not contain spaces, '=' or newlines: that would make lines ambiguous,
// so Log panics (a programming error, caught by the first test that hits it).
func (l *Logger) Log(node int, ev string, height uint64, round int32, kv ...string) {
	if len(kv)%2 != 0 {
		panic("logging: odd number of key/value arguments")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "t=%010d node=%02d ev=%-13s h=%d r=%d", l.clock(), node, ev, height, round)
	for i := 0; i < len(kv); i += 2 {
		checkToken(kv[i])
		checkToken(kv[i+1])
		b.WriteByte(' ')
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(kv[i+1])
	}
	b.WriteByte('\n')
	if _, err := l.w.WriteString(b.String()); err != nil && l.err == nil {
		l.err = err
	}
}

func checkToken(s string) {
	if s == "" || strings.ContainsAny(s, " =\n\r\t") {
		panic(fmt.Sprintf("logging: invalid key or value %q", s))
	}
}

// Flush writes buffered lines and returns the first error seen, if any.
// Call it once at the end of every run.
func (l *Logger) Flush() error {
	if err := l.w.Flush(); err != nil && l.err == nil {
		l.err = err
	}
	return l.err
}
