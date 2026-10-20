// Package winres writes a Windows resource object (.syso) the Go linker picks up: the icon
// group and the version information of an executable (08 §15.3). It replaces a resource
// compiler so the release needs no extra tool.
package winres

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Resource types and IDs.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16

	// IconGroupID is the icon group the Wails window loads (application ID 3); Explorer
	// shows the first group, which is this one.
	IconGroupID = 3

	langEnUS = 0x0409
)

// Icon is one image of the icon group: a PNG of size×size.
type Icon struct {
	Size int
	PNG  []byte
}

// Version is the version information block.
type Version struct {
	Version     string // 1.2.3 or 1.2.3-rc1
	Description string // FileDescription
	Filename    string // OriginalFilename and InternalName
	Product     string
	Copyright   string
}

type res struct {
	typ, id uint16
	data    []byte
}

// Syso builds an AMD64 COFF object with an .rsrc section holding the icons and the version.
func Syso(icons []Icon, v Version) ([]byte, error) {
	var rs []res
	group := new(bytes.Buffer)
	le := binary.LittleEndian
	binary.Write(group, le, [3]uint16{0, 1, uint16(len(icons))})
	for i, ic := range icons {
		if ic.Size < 1 || ic.Size > 256 {
			return nil, fmt.Errorf("icon size %d out of range", ic.Size)
		}
		id := uint16(i + 1)
		rs = append(rs, res{rtIcon, id, ic.PNG})
		dim := uint8(ic.Size % 256) // 0 means 256
		group.Write([]byte{dim, dim, 0, 0})
		binary.Write(group, le, struct {
			Planes, Bits uint16
			Bytes        uint32
			ID           uint16
		}{1, 32, uint32(len(ic.PNG)), id})
	}
	rs = append(rs, res{rtGroupIcon, IconGroupID, group.Bytes()})
	vi, err := versionInfo(v)
	if err != nil {
		return nil, err
	}
	rs = append(rs, res{rtVersion, 1, vi})
	return coff(rs), nil
}

// coff lays out the resource directory (type → id → language → data entry), the data,
// and one ADDR32NB relocation per data entry so the linker turns section offsets into RVAs.
func coff(rs []res) []byte {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].typ != rs[j].typ {
			return rs[i].typ < rs[j].typ
		}
		return rs[i].id < rs[j].id
	})
	var types []uint16
	byType := map[uint16][]int{}
	for i, r := range rs {
		if len(byType[r.typ]) == 0 {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], i)
	}
	const dirSize, entSize, dataEntSize = 16, 8, 16
	// Offsets: root dir, type dirs, language dirs, data entries, then data.
	off := dirSize + entSize*len(types)
	typeDir := map[uint16]int{}
	for _, t := range types {
		typeDir[t] = off
		off += dirSize + entSize*len(byType[t])
	}
	langDir := make([]int, len(rs))
	for i := range rs {
		langDir[i] = off
		off += dirSize + entSize
	}
	dataEnt := make([]int, len(rs))
	for i := range rs {
		dataEnt[i] = off
		off += dataEntSize
	}
	dataAt := make([]int, len(rs))
	for i, r := range rs {
		off = (off + 7) &^ 7
		dataAt[i] = off
		off += len(r.data)
	}
	sec := make([]byte, (off+3)&^3)
	le := binary.LittleEndian
	dir := func(at, n int) { le.PutUint16(sec[at+14:], uint16(n)) } // ID entries only
	ent := func(at int, id uint32, target int, sub bool) {
		le.PutUint32(sec[at:], id)
		v := uint32(target)
		if sub {
			v |= 0x80000000
		}
		le.PutUint32(sec[at+4:], v)
	}
	dir(0, len(types))
	for k, t := range types {
		ent(dirSize+entSize*k, uint32(t), typeDir[t], true)
		dir(typeDir[t], len(byType[t]))
		for j, i := range byType[t] {
			ent(typeDir[t]+dirSize+entSize*j, uint32(rs[i].id), langDir[i], true)
		}
	}
	var relocs []uint32
	for i, r := range rs {
		dir(langDir[i], 1)
		ent(langDir[i]+dirSize, langEnUS, dataEnt[i], false)
		le.PutUint32(sec[dataEnt[i]:], uint32(dataAt[i])) // section offset, relocated to an RVA
		le.PutUint32(sec[dataEnt[i]+4:], uint32(len(r.data)))
		relocs = append(relocs, uint32(dataEnt[i]))
		copy(sec[dataAt[i]:], r.data)
	}

	const hdr, shdr = 20, 40
	rawAt := hdr + shdr
	relAt := rawAt + len(sec)
	symAt := relAt + 10*len(relocs)
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, le, v) }
	w(uint16(0x8664)) // Machine: AMD64
	w(uint16(1))      // NumberOfSections
	w(uint32(0))      // TimeDateStamp
	w(uint32(symAt))  // PointerToSymbolTable
	w(uint32(1))      // NumberOfSymbols
	w(uint16(0))      // SizeOfOptionalHeader
	w(uint16(0))      // Characteristics
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))           // VirtualSize
	w(uint32(0))           // VirtualAddress
	w(uint32(len(sec)))    // SizeOfRawData
	w(uint32(rawAt))       // PointerToRawData
	w(uint32(relAt))       // PointerToRelocations
	w(uint32(0))           // PointerToLinenumbers
	w(uint16(len(relocs))) // NumberOfRelocations
	w(uint16(0))           // NumberOfLinenumbers
	w(uint32(0x40000040))  // initialized data, readable
	b.Write(sec)
	for _, r := range relocs {
		w(r)         // VirtualAddress: the field inside the section
		w(uint32(0)) // symbol 0: .rsrc
		w(uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))   // Value
	w(int16(1))    // SectionNumber
	w(uint16(0))   // Type
	b.WriteByte(3) // StorageClass: static
	b.WriteByte(0) // NumberOfAuxSymbols
	w(uint32(4))   // empty string table
	return b.Bytes()
}

// parse turns 1.2.3[-x] into four 16-bit parts (the fourth is 0).
func parse(v string) ([4]uint16, error) {
	var out [4]uint16
	core, _, _ := strings.Cut(strings.SplitN(v, "+", 2)[0], "-")
	parts := strings.Split(core, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return out, fmt.Errorf("version %q", v)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("version %q: %v", v, err)
		}
		out[i] = uint16(n)
	}
	return out, nil
}

func versionInfo(v Version) ([]byte, error) {
	n, err := parse(v.Version)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	fixed := new(bytes.Buffer)
	ms, ls := uint32(n[0])<<16|uint32(n[1]), uint32(n[2])<<16|uint32(n[3])
	binary.Write(fixed, le, [13]uint32{0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x00040004, 1, 0, 0, 0})
	strs := [][2]string{
		{"CompanyName", v.Product},
		{"FileDescription", v.Description},
		{"FileVersion", v.Version},
		{"InternalName", strings.TrimSuffix(v.Filename, ".exe")},
		{"LegalCopyright", v.Copyright},
		{"OriginalFilename", v.Filename},
		{"ProductName", v.Product},
		{"ProductVersion", v.Version},
	}
	var kids [][]byte
	for _, s := range strs {
		val := utf16z(s[1])
		kids = append(kids, block(s[0], val, 1, len(val)/2, nil))
	}
	table := block("040904b0", nil, 1, 0, kids)
	sfi := block("StringFileInfo", nil, 1, 0, [][]byte{table})
	tr := []byte{0x09, 0x04, 0xB0, 0x04} // en-US, Unicode
	vfi := block("VarFileInfo", nil, 1, 0, [][]byte{block("Translation", tr, 0, len(tr), nil)})
	return block("VS_VERSION_INFO", fixed.Bytes(), 0, fixed.Len(), [][]byte{sfi, vfi}), nil
}

// block is one VS_VERSIONINFO-style node: length, value length, type, key, value, children,
// each part aligned to 4 bytes.
func block(key string, value []byte, typ uint16, valueLen int, kids [][]byte) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 6))
	b.Write(utf16z(key))
	pad := func() {
		for b.Len()%4 != 0 {
			b.WriteByte(0)
		}
	}
	pad()
	b.Write(value)
	for _, k := range kids {
		pad()
		b.Write(k)
	}
	out := b.Bytes()
	le := binary.LittleEndian
	le.PutUint16(out[0:], uint16(len(out)))
	le.PutUint16(out[2:], uint16(valueLen))
	le.PutUint16(out[4:], typ)
	return out
}

func utf16z(s string) []byte {
	u := append(utf16.Encode([]rune(s)), 0)
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}
