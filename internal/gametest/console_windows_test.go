package gametest

import (
	"testing"
	"unicode/utf8"
)

func TestDecodeANSI(t *testing.T) {
	if got := decodeANSI([]byte("ASCII only")); got != "ASCII only" {
		t.Fatalf("ASCII: %q", got)
	}
	var high []byte
	for b := 0x80; b <= 0xff; b++ {
		high = append(high, byte(b))
	}
	if got := decodeANSI(high); !utf8.ValidString(got) || got == "" {
		t.Fatalf("high bytes: %q", got)
	}
}
