package looks

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tmod is a synthetic Mod folder.
type tmod struct {
	t    *testing.T
	root string
}

func newMod(t *testing.T, base, name string) *tmod {
	t.Helper()
	m := &tmod{t, filepath.Join(base, name)}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *tmod) write(rel string, data []byte) {
	m.t.Helper()
	p := filepath.Join(m.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		m.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		m.t.Fatal(err)
	}
}

// pngBytes is a valid w×w PNG; shade changes the content.
func pngBytes(t *testing.T, w int, shade uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, w))
	for i := 0; i < w*w; i++ {
		img.SetNRGBA(i%w, i/w, color.NRGBA{shade, uint8(i), 7, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// tex writes 2×2 textures with one shade under Textures/.
func (m *tmod) tex(shade uint8, rels ...string) {
	for _, r := range rels {
		m.write("Textures/"+r, pngBytes(m.t, 2, shade))
	}
}

func (m *tmod) defs(name, body string) {
	m.write("Defs/"+name, []byte(`<?xml version="1.0" encoding="utf-8"?><Defs>`+body+`</Defs>`))
}

// plan lists the Mods in load order, each with its root as the only load folder.
func plan(ms ...*tmod) []Mod {
	var out []Mod
	for i, m := range ms {
		out = append(out, Mod{Pos: i, ID: filepath.Base(m.root), Name: filepath.Base(m.root), Root: m.root, Folders: []string{m.root}})
	}
	return out
}

func build(t *testing.T, mods []Mod, o Options) *Index {
	t.Helper()
	idx, err := Build(context.Background(), mods, o)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func look(t *testing.T, idx *Index, id string) *Look {
	t.Helper()
	l := idx.ByID[id]
	if l == nil {
		var ids []string
		for _, x := range idx.Looks {
			ids = append(ids, x.ID)
		}
		t.Fatalf("no look %q; have %v", id, ids)
	}
	return l
}

func wantSlots(t *testing.T, l *Look, want ...string) {
	t.Helper()
	if got := l.SlotNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("%s slots = %v, want %v", l.ID, got, want)
	}
}

func TestGrouping(t *testing.T) {
	a := newMod(t, t.TempDir(), "A")
	a.tex(1, "Things/Item/Gun.png", "Things/Item/Gun_m.png",
		"Things/Pawn/Dog_south.png", "Things/Pawn/Dog_east.png", "Things/Pawn/Dog_north.png", "Things/Pawn/Dog_southm.png", "Things/Pawn/Dog_east_m.png",
		"Things/Apparel/Shirt/Shirt.png", "Things/Apparel/Shirt/Shirt_Female_south.png", "Things/Apparel/Shirt/Shirt_Female_east.png",
		"Things/Apparel/Shirt/Shirt_Male_south.png", "Things/Apparel/Shirt/Shirt_Male_southm.png", "Things/Apparel/Shirt/Shirt_Thin_north.png",
		"Things/Apparel/Shirt/Shirt_Alien_south.png",
		"Things/Building/Natural/Rocks/RockB.png", "Things/Building/Natural/Rocks/RockA.png", "Things/Building/Natural/Rocks/RockA_m.png",
		"Things/Loose/Pebble_south.png", "Source/Skipped.png", "top.png")
	a.write("Textures/Things/Item/notes.txt", []byte("not a texture"))
	a.defs("a.xml", `
<ThingDef><defName>Shirt</defName><apparel><wornGraphicPath>Things/Apparel/Shirt/Shirt</wornGraphicPath></apparel></ThingDef>
<ThingDef><defName>Rocks</defName><graphicData><texPath>Things\Building\Natural\Rocks/</texPath><graphicClass>Graphic_Random</graphicClass></graphicData></ThingDef>`)
	idx := build(t, plan(a), Options{})

	var ids []string
	for _, l := range idx.Looks {
		ids = append(ids, l.ID+":"+l.Kind)
	}
	want := []string{"things/apparel/shirt/shirt:worn", "things/building/natural/rocks:folder", "things/item/gun:plain",
		"things/loose/pebble:plain", "things/pawn/dog:plain", "top:plain"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("looks = %v, want %v", ids, want)
	}
	wantSlots(t, look(t, idx, "things/item/gun"), "single", "single_mask")
	wantSlots(t, look(t, idx, "things/pawn/dog"), "south", "south_mask", "east", "east_mask", "north")
	wantSlots(t, look(t, idx, "things/apparel/shirt/shirt"), "single", "male_south", "male_south_mask", "female_south", "female_east", "thin_north", "alien_south")
	rocks := look(t, idx, "things/building/natural/rocks")
	wantSlots(t, rocks, "rocka", "rocka_mask", "rockb")
	if !reflect.DeepEqual(rocks.Tags, []string{"random"}) || rocks.Category != "things/building/natural" {
		t.Errorf("rocks tags %v category %q", rocks.Tags, rocks.Category)
	}
	if c := look(t, idx, "things/item/gun").Category; c != "things/item" {
		t.Errorf("gun category %q", c)
	}
	if c := look(t, idx, "top").Category; c != "" {
		t.Errorf("top category %q", c)
	}
	f := look(t, idx, "things/item/gun").Providers[0].Slots["single"]
	if f.Rel != "Textures/Things/Item/Gun.png" || f.Format != "png" || f.W != 2 || f.H != 2 || f.Size == 0 || f.MTime == 0 || f.SameAs != -1 || f.Hash != "" {
		t.Errorf("gun file %+v", *f)
	}
	if !IsMask("south_mask") || IsMask("south") {
		t.Error("IsMask")
	}
}

// Without Def data nothing is a folder look, and a body-type suffix is part of a plain id.
func TestGroupingWithoutDefs(t *testing.T) {
	a := newMod(t, t.TempDir(), "A")
	a.tex(1, "Rocks/RockA.png", "Rocks/RockB.png", "Shirt/Shirt_Female_south.png")
	idx := build(t, plan(a), Options{})
	if len(idx.Looks) != 3 || idx.ByID["rocks/rocka"] == nil || idx.ByID["shirt/shirt_female"] == nil {
		t.Fatalf("looks %v", idx.ByID)
	}
	wantSlots(t, look(t, idx, "shirt/shirt_female"), "south")
}

func TestLoadFolderPriority(t *testing.T) {
	a := newMod(t, t.TempDir(), "A")
	a.write("1.6/textures/A/X.png", pngBytes(t, 4, 1))
	a.write("1.6/textures/A/X.dds", ddsBytes(8, 16, "DXT5"))
	a.write("Textures/A/x.png", pngBytes(t, 2, 1))
	a.write("Textures/A/Y.png", pngBytes(t, 2, 1))
	mods := plan(a)
	mods[0].Folders = []string{filepath.Join(a.root, "1.6"), a.root, a.root}
	idx := build(t, mods, Options{})
	x := look(t, idx, "a/x").Providers[0].Slots["single"]
	if x.Rel != "1.6/textures/A/X.png" || x.W != 4 || x.Path != filepath.Join(a.root, "1.6", "textures", "A", "X.png") {
		t.Errorf("x = %+v", *x)
	}
	if x.Alt == nil || x.Alt.Format != "dds:DXT5" || x.Alt.W != 8 || x.Alt.H != 16 || x.Alt.Rel != "1.6/textures/A/X.dds" {
		t.Errorf("x.Alt = %+v", x.Alt)
	}
	if y := look(t, idx, "a/y").Providers[0].Slots["single"]; y.Rel != "Textures/A/Y.png" {
		t.Errorf("y = %+v", *y)
	}
}

func TestIdenticalAndStates(t *testing.T) {
	base := t.TempDir()
	a, b, c := newMod(t, base, "A"), newMod(t, base, "B"), newMod(t, base, "C")
	a.tex(1, "T/X_south.png", "T/X_east.png", "T/Y.png", "T/Z.png", "T/W_south.png", "T/W_east.png", "F/One.png")
	b.tex(1, "T/X_south.png", "T/Y.png", "T/W_south.png") // identical copies of A's
	c.tex(2, "T/X_south.png", "F/Two.png")
	c.defs("c.xml", `<ThingDef><defName>F</defName><graphicData><texPath>F</texPath><graphicClass>Graphic_Collection</graphicClass></graphicData></ThingDef>`)
	official := newMod(t, base, "Core")
	official.tex(3, "T/X_south.png")
	mods := plan(a, b, c, official)
	mods[3].Official = true
	idx := build(t, mods, Options{})

	x := look(t, idx, "t/x")
	if len(x.Providers) != 3 {
		t.Fatalf("official content supplies no textures; providers = %d", len(x.Providers))
	}
	for pos, want := range [][2]bool{{true, false}, {true, false}, {false, true}} {
		if x.Overridden(pos) != want[0] || x.Overriding(pos) != want[1] {
			t.Errorf("x provider %d: overridden %v overriding %v, want %v", pos, x.Overridden(pos), x.Overriding(pos), want)
		}
	}
	if s := x.Providers[1].Slots["south"]; s.SameAs != 0 || s.Hash == "" || x.Providers[2].Slots["south"].SameAs != -1 {
		t.Errorf("x south of B: %+v", *s)
	}
	if x.OrderWinner("south").Pos != 2 || x.OrderWinner("east").Pos != 0 || x.OrderWinner("north") != nil {
		t.Error("x order winners")
	}
	if !x.Mixed(nil) {
		t.Error("x takes south from C and east from A: mixed")
	}
	y := look(t, idx, "t/y")
	if y.Overridden(0) || y.Overriding(1) || y.Providers[1].Slots["single"].SameAs != 0 || y.Mixed(nil) {
		t.Error("an identical copy is no override")
	}
	if z := look(t, idx, "t/z").Providers[0].Slots["single"]; z.Hash != "" {
		t.Error("a texture one Mod supplies is not hashed")
	}
	if look(t, idx, "t/w").Mixed(nil) {
		t.Error("w: B's south is identical to A's, so A supplies everything the game shows")
	}
	f := look(t, idx, "f")
	if f.Kind != KindFolder || !reflect.DeepEqual(f.Tags, []string{"collection"}) || f.Mixed(nil) || f.Mixed(map[string]int{"one": 0}) {
		t.Errorf("folder look %+v is never mixed", *f)
	}
}

const defsCore = `
<ThingDef Name="BaseGun" Abstract="True"><graphicData><graphicClass>Graphic_Single</graphicClass></graphicData></ThingDef>
<ThingStyleDef Name="StyleBase" Abstract="True"><graphicData><graphicClass>Graphic_Multi</graphicClass></graphicData></ThingStyleDef>
<ThingDef Name="LoopA" ParentName="LoopB"><defName>LoopA</defName></ThingDef>
<ThingDef Name="LoopB" ParentName="LoopA"><defName>LoopB</defName><uiIconPath>UI/Loop</uiIconPath></ThingDef>`

const defsA = `
<ThingDef Name="GunParent" ParentName="BaseGun" Abstract="True"><label>a gun</label><graphicData><texPath>Things\Item\Gun</texPath></graphicData></ThingDef>
<ThingDef ParentName="GunParent"><defName>GunChild</defName><uiIconPath>UI/GunIcon_south</uiIconPath></ThingDef>
<ThingDef ParentName="GunParent"><defName>GunOther</defName><label>other</label><graphicData><texPath>Things/Item/Other</texPath></graphicData></ThingDef>
<ThingStyleDef ParentName="StyleBase"><defName>Gun_Fancy</defName><graphicData><texPath>Things/Item/GunFancy</texPath></graphicData></ThingStyleDef>
<ThingStyleDef><defName>Shirt_Fancy</defName><wornGraphicPath>Things/Apparel/ShirtFancy</wornGraphicPath></ThingStyleDef>
<StyleCategoryDef><defName>Fancy</defName><thingDefStyles>
  <li><thingDef>GunChild</thingDef><styleDef>Gun_Fancy</styleDef></li>
  <li><thingDef>Shirt</thingDef><styleDef>Shirt_Fancy</styleDef></li>
  <li><thingDef>Bed</thingDef><styleDef>Bed_Official</styleDef></li>
</thingDefStyles></StyleCategoryDef>
<StyleCategoryDef><defName>Plain</defName></StyleCategoryDef>
<AbilityDef><defName>Shoot</defName><iconPath>ui/gunicon_south</iconPath></AbilityDef>`

func TestDefsThroughParentName(t *testing.T) {
	base := t.TempDir()
	core, a, late := newMod(t, base, "Core"), newMod(t, base, "A"), newMod(t, base, "Late")
	core.defs("core.xml", defsCore)
	a.defs("Sub/a.xml", defsA)
	a.defs("broken.xml", `<ThingDef><defName>Broken</defName><uiIconPath>UI/GunIcon_south</uiIconPath>`)
	a.write("Defs/utf16.xml", utf16le(`<?xml version="1.0" encoding="utf-16"?><Defs><ThingDef><defName>Wide</defName><uiIconPath>Things/Item/Other</uiIconPath></ThingDef></Defs>`))
	a.tex(1, "Things/Item/Gun.png", "Things/Item/Other.png", "Things/Item/GunFancy_south.png",
		"Things/Apparel/ShirtFancy_Male_south.png", "UI/GunIcon_south.png", "UI/Loop.png")
	// A later Mod's base with the same Name replaces the earlier one.
	late.defs("late.xml", `<ThingDef Name="BaseGun" Abstract="True"><graphicData><graphicClass>Graphic_StackCount</graphicClass></graphicData></ThingDef>`)
	mods := plan(core, a, late)
	mods[0].Official = true
	idx := build(t, mods, Options{})

	if len(idx.Warnings) != 1 || !strings.Contains(idx.Warnings[0], "broken.xml") {
		t.Errorf("warnings = %v", idx.Warnings)
	}
	gun := look(t, idx, "things/item/gun")
	wantDefs := []DefRef{
		{Type: "ThingDef", Name: "GunChild", Label: "a gun", Field: "graphicData/texPath", GraphicClass: "Graphic_StackCount", ModPos: 1},
		{Type: "ThingDef", Name: "GunParent", Label: "a gun", Field: "graphicData/texPath", GraphicClass: "Graphic_StackCount", ModPos: 1, Abstract: true},
	}
	if !reflect.DeepEqual(gun.Defs, wantDefs) {
		t.Errorf("gun defs = %+v", gun.Defs)
	}
	// Gun.png is a file, not a folder with textures: the folder class makes no folder look.
	if gun.Kind != KindPlain || len(gun.Tags) != 0 {
		t.Errorf("gun kind %s tags %v", gun.Kind, gun.Tags)
	}
	other := look(t, idx, "things/item/other")
	if len(other.Defs) != 2 || other.Defs[0].Name != "GunOther" || other.Defs[0].Label != "other" || other.Defs[1].Name != "Wide" || other.Defs[1].Field != "uiIconPath" {
		t.Errorf("other defs = %+v", other.Defs)
	}
	// An icon path that ends in a facing leads to the look its texture was grouped into.
	icon := look(t, idx, "ui/gunicon")
	if len(icon.Defs) != 2 || icon.Defs[0] != (DefRef{Type: "AbilityDef", Name: "Shoot", Field: "iconPath", ModPos: 1}) || icon.Defs[1].Field != "uiIconPath" {
		t.Errorf("icon defs = %+v", icon.Defs)
	}
	if loop := look(t, idx, "ui/loop"); len(loop.Defs) != 2 {
		t.Errorf("a ParentName cycle still resolves: %+v", loop.Defs)
	}
	fancy := look(t, idx, "things/item/gunfancy")
	wantStyle := []LookStyle{{StyleDef: "Gun_Fancy", Categories: []string{"Fancy"}, ThingDefs: []string{"GunChild"}}}
	if !reflect.DeepEqual(fancy.Styles, wantStyle) || !reflect.DeepEqual(fancy.Tags, []string{"style"}) || fancy.Defs[0].GraphicClass != "Graphic_Multi" {
		t.Errorf("fancy = %+v", *fancy)
	}
	shirt := look(t, idx, "things/apparel/shirtfancy")
	if shirt.Kind != KindWorn || len(shirt.Styles) != 1 || shirt.Defs[0].Field != "wornGraphicPath" {
		t.Errorf("shirt = %+v", *shirt)
	}
	wantSlots(t, shirt, "male_south")
	wantStyles := map[string][]StyleRef{
		"GunChild": {{StyleDef: "Gun_Fancy", Category: "Fancy", LookID: "things/item/gunfancy"}},
		"Shirt":    {{StyleDef: "Shirt_Fancy", Category: "Fancy", LookID: "things/apparel/shirtfancy"}},
		"Bed":      {{StyleDef: "Bed_Official", Category: "Fancy"}},
	}
	if !reflect.DeepEqual(idx.Styles, wantStyles) {
		t.Errorf("styles = %+v", idx.Styles)
	}
	if !reflect.DeepEqual(idx.Categories, []string{"Fancy", "Plain"}) {
		t.Errorf("categories = %v", idx.Categories)
	}
}

func utf16le(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, r := range s {
		out = binary.LittleEndian.AppendUint16(out, uint16(r))
	}
	return out
}

func TestInGameAndMixed(t *testing.T) {
	base := t.TempDir()
	a, b, c := newMod(t, base, "A"), newMod(t, base, "B"), newMod(t, base, "C")
	a.tex(1, "T/X_south.png", "T/X_east.png", "T/X_southm.png")
	b.tex(2, "T/X_south.png", "T/X_east.png")
	c.tex(3, "T/X_south.png", "T/X_southm.png")
	x := look(t, build(t, plan(a, b, c), Options{}), "t/x")

	type w struct {
		pos      int
		byChoice bool
	}
	cases := []struct {
		name   string
		choice map[string]int
		want   map[string]w
		mixed  bool
	}{
		{"load order", nil, map[string]w{"south": {2, false}, "east": {1, false}, "south_mask": {2, false}}, true},
		{"whole look", map[string]int{All: 0}, map[string]w{"south": {0, true}, "east": {0, true}, "south_mask": {0, true}}, false},
		{"slot over all; mask falls to the last provider that has one", map[string]int{All: 0, "south": 1},
			map[string]w{"south": {1, true}, "east": {0, true}, "south_mask": {2, false}}, true},
		{"a Mod that lacks the slot", map[string]int{"east": 2}, map[string]w{"south": {2, false}, "east": {1, false}, "south_mask": {2, false}}, true},
		{"a slot choice that fails falls to all", map[string]int{"east": 2, All: 0}, map[string]w{"south": {0, true}, "east": {0, true}, "south_mask": {0, true}}, false},
		{"a Mod that is not a provider", map[string]int{All: 9}, map[string]w{"south": {2, false}, "east": {1, false}, "south_mask": {2, false}}, true},
		{"a mask is not chosen", map[string]int{"south_mask": 0}, map[string]w{"south": {2, false}, "east": {1, false}, "south_mask": {2, false}}, true},
	}
	for _, tc := range cases {
		got := x.InGame(tc.choice)
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d slots", tc.name, len(got))
		}
		for slot, want := range tc.want {
			g := got[slot]
			if g.Pos != want.pos || g.ByChoice != want.byChoice || g.File != x.provider(want.pos).Slots[slot] {
				t.Errorf("%s: %s = pos %d byChoice %v, want %+v", tc.name, slot, g.Pos, g.ByChoice, want)
			}
		}
		if m := x.Mixed(tc.choice); m != tc.mixed {
			t.Errorf("%s: mixed = %v", tc.name, m)
		}
	}
}

func ddsBytes(w, h uint32, fourCC string) []byte {
	b := make([]byte, 148)
	copy(b, "DDS ")
	binary.LittleEndian.PutUint32(b[4:], 124)
	binary.LittleEndian.PutUint32(b[12:], h)
	binary.LittleEndian.PutUint32(b[16:], w)
	binary.LittleEndian.PutUint32(b[76:], 32)
	if fourCC != "" {
		binary.LittleEndian.PutUint32(b[80:], 0x4)
		copy(b[84:88], fourCC)
	} else {
		binary.LittleEndian.PutUint32(b[80:], 0x41)
	}
	return b
}

func TestHeaders(t *testing.T) {
	a := newMod(t, t.TempDir(), "A")
	a.write("Textures/H/dxt1.dds", ddsBytes(64, 32, "DXT1"))
	a.write("Textures/H/dx10.dds", ddsBytes(128, 256, "DX10"))
	a.write("Textures/H/raw.dds", ddsBytes(4, 8, ""))
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil); err != nil {
		t.Fatal(err)
	}
	a.write("Textures/H/photo.JPG", jb.Bytes())
	a.write("Textures/H/photo2.jpeg", jb.Bytes())
	psd := make([]byte, 30)
	copy(psd, "8BPS")
	binary.BigEndian.PutUint32(psd[14:], 7)
	binary.BigEndian.PutUint32(psd[18:], 9)
	a.write("Textures/H/layers.psd", psd)
	a.write("Textures/H/broken.png", []byte("not a png"))
	a.write("Textures/H/short.dds", []byte("DDS "))
	idx := build(t, plan(a), Options{})
	want := map[string]File{
		"h/dxt1": {Format: "dds:DXT1", W: 64, H: 32}, "h/dx10": {Format: "dds:DX10", W: 128, H: 256}, "h/raw": {Format: "dds:RGBA", W: 4, H: 8},
		"h/photo": {Format: "jpg", W: 20, H: 10}, "h/photo2": {Format: "jpg", W: 20, H: 10}, "h/layers": {Format: "psd", W: 9, H: 7},
		"h/broken": {Format: "png"}, "h/short": {Format: "dds"},
	}
	if len(idx.Looks) != len(want) {
		t.Fatalf("%d looks", len(idx.Looks))
	}
	for id, w := range want {
		f := look(t, idx, id).Providers[0].Slots["single"]
		if f.Format != w.Format || f.W != w.W || f.H != w.H {
			t.Errorf("%s = %s %dx%d, want %s %dx%d", id, f.Format, f.W, f.H, w.Format, w.W, w.H)
		}
	}
}

func TestCacheReuse(t *testing.T) {
	base := t.TempDir()
	a, b := newMod(t, base, "A"), newMod(t, base, "B")
	a.tex(1, "T/X.png", "T/Y_south.png", "T/Only.png")
	b.tex(1, "T/X.png")
	b.tex(2, "T/Y_south.png")
	b.write("Textures/T/X.dds", ddsBytes(4, 4, "DXT5"))
	o := Options{CachePath: filepath.Join(base, "cache", "looks", "index.json")}
	count := func() (*Index, int64, int64) {
		h, x := headerReads.Load(), hashReads.Load()
		idx := build(t, plan(a, b), o)
		return idx, headerReads.Load() - h, hashReads.Load() - x
	}
	first, headers, hashes := count()
	wantHashes := int64(2) // X; Y_south too when the two shades compress to one size
	if len(pngBytes(t, 2, 1)) == len(pngBytes(t, 2, 2)) {
		wantHashes = 4
	}
	if headers != 6 || hashes != wantHashes {
		t.Fatalf("first build: %d header reads, %d hashes, want 6 and %d", headers, hashes, wantHashes)
	}
	second, headers, hashes := count()
	if headers != 0 || hashes != 0 {
		t.Fatalf("second build: %d header reads, %d hashes, want none", headers, hashes)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("the cached index differs from the first")
	}
	if f := look(t, second, "t/x").Providers[1].Slots["single"]; f.SameAs != 0 || f.W != 2 || f.Alt == nil || f.Alt.Format != "dds:DXT5" {
		t.Errorf("x of B from the cache: %+v", *f)
	}
	// A changed file is read again; the others are not.
	a.write("Textures/T/Only.png", pngBytes(t, 8, 5))
	third, headers, hashes := count()
	if headers != 1 || hashes != 0 {
		t.Errorf("after one change: %d header reads, %d hashes, want 1 and 0", headers, hashes)
	}
	if f := look(t, third, "t/only").Providers[0].Slots["single"]; f.W != 8 {
		t.Errorf("changed file: %+v", *f)
	}
	// A damaged cache is a miss, not an error.
	if err := os.WriteFile(o.CachePath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, headers, _ = count(); headers != 6 {
		t.Errorf("damaged cache: %d header reads, want 6", headers)
	}
}

func TestCancel(t *testing.T) {
	base := t.TempDir()
	var ms []*tmod
	for _, n := range []string{"A", "B", "C", "D", "E", "F"} {
		m := newMod(t, base, n)
		m.tex(1, "T/X.png", "T/Y.png")
		m.defs("d.xml", `<ThingDef><defName>X</defName></ThingDef>`)
		ms = append(ms, m)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if idx, err := Build(ctx, plan(ms...), Options{}); !errors.Is(err, context.Canceled) || idx != nil {
		t.Errorf("cancelled before the start: %v, %v", idx, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	idx, err := Build(ctx, plan(ms...), Options{Workers: 1, Progress: func(done, total int) {
		calls++
		if done != calls || total != len(ms) {
			t.Errorf("progress %d/%d on call %d", done, total, calls)
		}
		if done == 2 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) || idx != nil || calls != 2 {
		t.Errorf("cancelled after two Mods: index %v, err %v, %d progress calls", idx != nil, err, calls)
	}
}

// 21 §2.3: Index.Things keeps, for a concrete ThingDef whose graphic or worn path leads to
// a look, the path as written, the graphic class and the other children of graphicData as
// XML, with what the Def inherits; a thing whose textures no Mod ships is not kept.
func TestThingGraphics(t *testing.T) {
	a := newMod(t, t.TempDir(), "A")
	a.tex(1, "Things/Item/Hat.png", "Things/Worn/Cap/Cap_Male_south.png")
	a.defs("Things.xml", `
<ThingDef Name="Base" Abstract="True"><graphicData><graphicClass>Graphic_Single</graphicClass><drawSize>(2,2)</drawSize><shadowData><volume>(0.3, 0.5, 0.3)</volume></shadowData></graphicData></ThingDef>
<ThingDef ParentName="Base"><defName>Hat</defName><label>hat</label><graphicData><texPath>Things\Item\Hat</texPath><drawSize>3</drawSize><color>(1,2,3)</color></graphicData></ThingDef>
<ThingDef ParentName="Base"><defName>Cap</defName><graphicData><texPath>Things/Item/CapItem</texPath></graphicData><apparel><wornGraphicPath>Things/Worn/Cap/Cap</wornGraphicPath></apparel></ThingDef>
<ThingDef ParentName="Base"><defName>Official</defName><graphicData><texPath>Things/Item/InABundle</texPath></graphicData></ThingDef>`)
	idx := build(t, plan(a), Options{})
	hat := idx.Things["Hat"]
	if hat == nil || hat.TexPath != "Things/Item/Hat" || hat.GraphicClass != "Graphic_Single" || hat.Label != "hat" || hat.ModPos != 0 ||
		hat.Graphic != "<drawSize>3</drawSize><color>(1,2,3)</color><shadowData><volume>(0.3, 0.5, 0.3)</volume></shadowData>" {
		t.Fatalf("Hat = %+v", hat)
	}
	cap := idx.Things["Cap"]
	if cap == nil || cap.TexPath != "Things/Item/CapItem" || cap.WornPath != "Things/Worn/Cap/Cap" || cap.Graphic != "<drawSize>(2,2)</drawSize><shadowData><volume>(0.3, 0.5, 0.3)</volume></shadowData>" {
		t.Fatalf("Cap = %+v", cap)
	}
	if len(idx.Things) != 2 {
		t.Fatalf("things: %v", idx.Things)
	}
}
