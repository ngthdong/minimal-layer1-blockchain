package encoding

import (
	"encoding/binary"
	"math"
)

type Writer struct {
	buf []byte
}

func NewWriter() *Writer {
	return &Writer{
		buf: make([]byte, 0, 128),
	}
}

func (w *Writer) U8(v uint8) *Writer {
	w.buf = append(w.buf, v)
	return w
}

func (w *Writer) U32(v uint32) *Writer {
	w.buf = binary.BigEndian.AppendUint32(w.buf, v)
	return w
}

func (w *Writer) U64(v uint64) *Writer {
	w.buf = binary.BigEndian.AppendUint64(w.buf, v)
	return w
}

func (w *Writer) I32(v int32) *Writer {
	return w.U32(uint32(v))
}

func (w *Writer) Bytes(b []byte) *Writer {
	if uint64(len(b)) > math.MaxUint32 {
		panic("encoding: byte slice longer than 2^32-1")
	}
	w.U32(uint32(len(b)))
	w.buf = append(w.buf, b...)
	return w
}

func (w *Writer) String(s string) *Writer { 
	return w.Bytes([]byte(s)) 
}

func (w *Writer) Fixed(b []byte) *Writer {
	w.buf = append(w.buf, b...)
	return w
}

func (w* Writer) Len() int {
	return len(w.buf)
}

func (w* Writer) Out() []byte {
	out := make([]byte, len(w.buf))
	copy(out, w.buf)
	return out
}
