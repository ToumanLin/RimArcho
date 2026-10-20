package winres

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParse(t *testing.T) {
	for in, want := range map[string][4]uint16{"0.7.0": {0, 7, 0, 0}, "1.2.3-rc1": {1, 2, 3, 0}, "1.0.0+abc": {1, 0, 0, 0}} {
		if got, err := parse(in); err != nil || got != want {
			t.Fatalf("%s: %v %v", in, got, err)
		}
	}
	if _, err := parse("1.x"); err == nil {
		t.Fatal("bad version accepted")
	}
}

// The object is an AMD64 COFF with one .rsrc section whose root directory lists the icon,
// icon group and version types, and one relocation per resource. (Windows itself reading
// the result was checked by linking it into RimArcho.exe, 08 §15.4.)
func TestSyso(t *testing.T) {
	icons := []Icon{{16, []byte("png16")}, {256, []byte("png256")}}
	b, err := Syso(icons, Version{Version: "1.2.3", Filename: "archo.exe", Product: "RimArcho"})
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if le.Uint16(b) != 0x8664 || le.Uint16(b[2:]) != 1 || string(b[20:25]) != ".rsrc" {
		t.Fatal("bad COFF header")
	}
	raw := b[le.Uint32(b[20+20:]):]
	if n := le.Uint16(raw[14:]); n != 3 {
		t.Fatalf("root directory has %d types, want 3", n)
	}
	for i, want := range []uint32{rtIcon, rtGroupIcon, rtVersion} {
		if got := le.Uint32(raw[16+8*i:]); got != want {
			t.Fatalf("type %d = %d, want %d", i, got, want)
		}
	}
	if n := le.Uint16(b[20+32:]); n != 4 {
		t.Fatalf("%d relocations, want 4", n)
	}
	if !bytes.Contains(b, utf16z("VS_VERSION_INFO")) || !bytes.Contains(b, utf16z("1.2.3")) {
		t.Fatal("no version block")
	}
}
