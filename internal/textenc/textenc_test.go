package textenc

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func enc16(s string, order binary.AppendByteOrder, bom bool) []byte {
	var out []byte
	if bom {
		out = order.AppendUint16(out, 0xFEFF)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		out = order.AppendUint16(out, u)
	}
	return out
}

func TestUTF8(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="utf-16"?><ModMetaData><name>Ünïcode 模组</name></ModMetaData>`
	for name, in := range map[string][]byte{
		"utf16le-bom":   enc16(doc, binary.LittleEndian, true),
		"utf16be-bom":   enc16(doc, binary.BigEndian, true),
		"utf16le-nobom": enc16(doc, binary.LittleEndian, false),
		"utf8-bom":      append([]byte{0xEF, 0xBB, 0xBF}, doc...),
		"utf8-declared": []byte(doc),
	} {
		if got := string(UTF8(in)); got != doc {
			t.Errorf("%s: %q", name, got)
		}
	}
}
