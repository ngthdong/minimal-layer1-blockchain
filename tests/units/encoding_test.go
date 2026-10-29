package unit

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ngthdong/minimal-layer1-blockchain/encoding"
)

func TestIntegersAreBigEndianFixedWidth(t *testing.T) {
	got := encoding.NewWriter().U8(0x01).U32(0x02030405).U64(6).I32(-1).Out()
	want := "01" + "02030405" + "0000000000000006" + "ffffffff"
	if hex.EncodeToString(got) != want {
		t.Fatalf("got %x, want %s", got, want)
	}
}

func TestBytesAndStringsAreLengthPrefixed(t *testing.T) {
	got := encoding.NewWriter().String("ab").Bytes([]byte{0xff}).Out()
	want := "00000002" + "6162" + "00000001" + "ff"
	if hex.EncodeToString(got) != want {
		t.Fatalf("got %x, want %s", got, want)
	}
}

// Without length prefixes ("ab","c") and ("a","bc") would both become "abc".
func TestEncodingIsUnambiguous(t *testing.T) {
	a := encoding.NewWriter().String("ab").String("c").Out()
	b := encoding.NewWriter().String("a").String("bc").Out()
	if bytes.Equal(a, b) {
		t.Fatal("different field splits produced the same bytes")
	}
}

func TestNilAndEmptyEncodeTheSame(t *testing.T) {
	a := encoding.NewWriter().Bytes(nil).Out()
	b := encoding.NewWriter().Bytes([]byte{}).Out()
	if !bytes.Equal(a, b) || len(a) != 4 {
		t.Fatalf("nil=%x empty=%x, want both 00000000", a, b)
	}
}

func TestFixedHasNoPrefix(t *testing.T) {
	h := bytes.Repeat([]byte{0xaa}, 32)
	w := encoding.NewWriter().Fixed(h)
	if w.Len() != 32 || !bytes.Equal(w.Out(), h) {
		t.Fatalf("Fixed wrote %d bytes, want 32 raw bytes", w.Len())
	}
}

func TestOutReturnsACopy(t *testing.T) {
	w := encoding.NewWriter().U8(1)
	first := w.Out()
	first[0] = 99
	w.U8(2)
	if got := w.Out(); got[0] != 1 || len(got) != 2 {
		t.Fatalf("writer was changed through Out(): %x", got)
	}
}

func TestSameInputSameBytes(t *testing.T) {
	enc := func() []byte {
		return encoding.NewWriter().String("Alice").U64(7).String("Alice/message").Bytes([]byte("hello")).Out()
	}
	if !bytes.Equal(enc(), enc()) {
		t.Fatal("encoding is not deterministic")
	}
}
