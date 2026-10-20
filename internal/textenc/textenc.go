// Package textenc normalises the encodings mod XML files come in (10 §1).
package textenc

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// UTF8 returns b as UTF-8 without a byte order mark. UTF-16 is recognised by its BOM, or
// without one by the zero bytes of an ASCII "<" (a file that declares encoding="utf-16" but
// is really UTF-8 is read as UTF-8). Anything else is returned unchanged.
func UTF8(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, bomUTF8):
		return b[3:]
	case bytes.HasPrefix(b, bomUTF16LE):
		return decode16(b[2:], binary.LittleEndian)
	case bytes.HasPrefix(b, bomUTF16BE):
		return decode16(b[2:], binary.BigEndian)
	case len(b) >= 4 && b[0] == '<' && b[1] == 0 && b[2] != 0 && b[3] == 0:
		return decode16(b, binary.LittleEndian)
	case len(b) >= 4 && b[0] == 0 && b[1] == '<' && b[2] == 0 && b[3] != 0:
		return decode16(b, binary.BigEndian)
	}
	return b
}

func decode16(b []byte, order binary.ByteOrder) []byte {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	rs := utf16.Decode(u)
	out := make([]byte, 0, len(rs)+len(rs)/2)
	for _, r := range rs {
		out = utf8.AppendRune(out, r)
	}
	return out
}
