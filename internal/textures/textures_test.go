package textures

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, p string, w, h int, alpha bool) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x == 0 {
				a = 10
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 7, a})
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeDDS writes a header the game accepts.
func fakeDDS(p string, w, h int, bc7 bool) error {
	b := make([]byte, 148)
	copy(b, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(b[4:], 124)
	le.PutUint32(b[12:], uint32(h))
	le.PutUint32(b[16:], uint32(w))
	le.PutUint32(b[28:], 1)
	le.PutUint32(b[76:], 32)
	le.PutUint32(b[80:], 0x4)
	if bc7 {
		copy(b[84:], "DX10")
		le.PutUint32(b[128:], 98)
	} else {
		copy(b[84:], "DXT1")
		b = b[:128]
	}
	return os.WriteFile(p, b, 0o644)
}

func TestPNGHeader(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), 64, 32, true)
	writePNG(t, filepath.Join(dir, "b.png"), 16, 8, false)
	writeFile(t, filepath.Join(dir, "c.png"), "not a png")
	h, ok := ReadPNGHeader(filepath.Join(dir, "a.png"))
	if !ok || h.W != 64 || h.H != 32 || !h.Alpha {
		t.Fatalf("a: %+v %v", h, ok)
	}
	h, ok = ReadPNGHeader(filepath.Join(dir, "b.png"))
	if !ok || h.W != 16 || h.H != 8 || h.Alpha {
		t.Fatalf("b (opaque, so Go writes RGB): %+v %v", h, ok)
	}
	if _, ok := ReadPNGHeader(filepath.Join(dir, "c.png")); ok {
		t.Fatal("c is not a PNG")
	}
}

// The game's rules: textures under each load folder's Textures/, Source/ skipped, keys
// relative to the load folder, a key converted whole or not at all.
func TestCollectAndPlan(t *testing.T) {
	root := t.TempDir()
	v16, common := filepath.Join(root, "1.6"), root
	writePNG(t, filepath.Join(v16, "Textures", "Things", "A.png"), 256, 256, false)
	writePNG(t, filepath.Join(common, "Textures", "Things", "A.png"), 256, 256, false) // overridden, still converted
	writePNG(t, filepath.Join(common, "Textures", "Things", "B_m.png"), 256, 256, true)
	writePNG(t, filepath.Join(common, "Textures", "UI", "Small.png"), 64, 64, true)
	writePNG(t, filepath.Join(common, "Textures", "Own.png"), 128, 128, false)
	if err := fakeDDS(filepath.Join(common, "Textures", "Own.dds"), 128, 128, false); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(common, "Textures", "Mixed.png"), 128, 128, false)
	writeFile(t, filepath.Join(v16, "Textures", "Mixed.jpg"), "jpg")
	writeFile(t, filepath.Join(common, "Textures", "Fake.png"), "not a png")
	writePNG(t, filepath.Join(common, "Textures", "Source", "Work.png"), 32, 32, false)
	writePNG(t, filepath.Join(common, "About", "Preview.png"), 32, 32, false)

	set, err := Collect(root, []string{v16, common})
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, s := range set.Sources {
		rels = append(rels, s.Rel+"@"+s.Key)
	}
	want := "1.6/Textures/Things/A.png@textures/things/a Textures/Mixed.png@textures/mixed Textures/Own.png@textures/own Textures/Things/A.png@textures/things/a Textures/Things/B_m.png@textures/things/b_m Textures/UI/Small.png@textures/ui/small"
	if got := strings.Join(rels, " "); got != want {
		t.Fatalf("sources:\n got %s\nwant %s", got, want)
	}
	if len(set.Others) != 3 { // Own.dds, Mixed.jpg, Fake.png
		t.Fatalf("others: %+v", set.Others)
	}

	p := MakePlan(set, Options{Scale: 50}, Owned{}, false)
	scales := map[string]int{}
	for _, j := range p.Jobs {
		scales[j.Rel] = j.Scale
	}
	if len(p.Jobs) != 4 || p.Skipped[SkipOwnDDS] != 1 || p.Skipped[SkipOtherFmt] != 1 {
		t.Fatalf("jobs %v skipped %v", scales, p.Skipped)
	}
	// A at 50% is 128 ≥ 128; the mask stays full size; Small (64) cannot shrink at all
	if scales["Textures/Things/A.png"] != 50 || scales["1.6/Textures/Things/A.png"] != 50 || scales["Textures/Things/B_m.png"] != 100 || scales["Textures/UI/Small.png"] != 100 {
		t.Fatalf("scales %v", scales)
	}
	// Own.dds made by RimArcho: its key is planned again unless fresh
	ours := Owned{Ours: func(p string) bool { return strings.HasSuffix(p, "Own.dds") }}
	if p := MakePlan(set, Options{}, ours, false); len(p.Jobs) != 5 {
		t.Fatalf("with our own DDS: %d jobs", len(p.Jobs))
	}
	ours.Fresh = func(s *Source) bool { return s.Key == "textures/own" }
	if p := MakePlan(set, Options{}, ours, false); len(p.Jobs) != 4 || p.Skipped[SkipFresh] != 1 {
		t.Fatalf("fresh: %d jobs %v", len(p.Jobs), p.Skipped)
	}
	if p := MakePlan(set, Options{}, ours, true); len(p.Jobs) != 5 {
		t.Fatalf("forced: %d jobs", len(p.Jobs))
	}
}

func TestAllFolders(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Textures/Things/Textures", "1.5/Textures", "1.6/Textures", "Mods/Extra/Textures", "Source/Textures", ".git/Textures", "Defs"} {
		os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755)
	}
	var got []string
	for _, f := range AllFolders(root) {
		r, _ := filepath.Rel(root, f)
		got = append(got, filepath.ToSlash(r))
	}
	if s := strings.Join(got, " "); s != ". 1.5 1.6 Mods/Extra" {
		t.Fatal(s)
	}
}

func TestScaleFallback(t *testing.T) {
	o := Options{Scale: 20}
	// shorter side 400: 20% → 80, 25% → 100, 40% → 160 ≥ 128
	if got := o.ScaleFor(&Source{W: 1000, H: 400}); got != 40 {
		t.Fatalf("got %d", got)
	}
	if got := o.ScaleFor(&Source{W: 100, H: 100}); got != 100 {
		t.Fatalf("small: %d", got)
	}
	// 300×200 at 75% is 225×150 (not a multiple of 4); 80% gives 240×160
	if got := (Options{Scale: 75}).ScaleFor(&Source{W: 300, H: 200}); got != 80 {
		t.Fatalf("multiple of 4: %d", got)
	}
	// 256 at 20% is 51.2: 25% → 64 is the first that fits (and ≥ MinSide 32 here)
	if got := (Options{Scale: 20, MinSide: 32}).ScaleFor(&Source{W: 256, H: 256}); got != 25 {
		t.Fatalf("256 at 20%%: %d", got)
	}
	if got := (Options{}).ScaleFor(&Source{W: 1000, H: 1000}); got != 100 {
		t.Fatalf("none: %d", got)
	}
}

func TestDDSSize(t *testing.T) {
	// 256×256 BC1 with mips: 128 + 8·(64² + 32² + … + 1) blocks
	var blocks int64
	for s := 64; ; s /= 2 {
		blocks += int64(s * s)
		if s == 1 {
			break
		}
	}
	blocks += 2 // the 2×2 and 1×1 levels are one block each (the loop stopped at 4×4)
	if got, want := DDSSize(256, 256, false, 100, true), 128+8*blocks; got != want {
		t.Fatalf("got %d want %d", got, want)
	}
	if got := DDSSize(256, 256, true, 100, false); got != 148+16*64*64 {
		t.Fatalf("BC7 no mips: %d", got)
	}
	if got := DDSSize(256, 256, false, 50, false); got != 128+8*32*32 {
		t.Fatalf("50%%: %d", got)
	}
	if got := DDSSize(30, 30, false, 100, false); got != 128+8*8*8 { // 30 → 32
		t.Fatalf("fix to 4: %d", got)
	}
}

func TestReadDDS(t *testing.T) {
	dir := t.TempDir()
	fakeDDS(filepath.Join(dir, "a.dds"), 64, 32, true)
	in, err := ReadDDS(filepath.Join(dir, "a.dds"))
	if err != nil || in.W != 64 || in.H != 32 || in.Format != "BC7" {
		t.Fatalf("%+v %v", in, err)
	}
	writeFile(t, filepath.Join(dir, "b.dds"), "nope")
	if CheckDDS(filepath.Join(dir, "b.dds")) == nil {
		t.Fatal("b is not a DDS")
	}
}

type fakeEnc struct {
	fail  string // a file name it skips
	calls [][]string
	err   error
}

func (f *fakeEnc) Encode(ctx context.Context, files []string, scale int, mips bool) error {
	f.calls = append(f.calls, files)
	for _, p := range files {
		if strings.Contains(p, f.fail) && f.fail != "" {
			continue
		}
		fakeDDS(strings.TrimSuffix(p, ".png")+".dds", 4, 4, false)
	}
	return f.err
}

func TestRun(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		writePNG(t, filepath.Join(root, "Textures", n+".png"), 256, 256, false)
	}
	set, _ := Collect(root, []string{root})
	p := MakePlan(set, Options{Scale: 50}, Owned{}, false)
	p.Jobs[2].Scale = 100 // two batches
	enc := &fakeEnc{fail: "b.png", err: errors.New("exit status 1")}
	var before, after, prog int
	res, err := Run(context.Background(), enc, p.Jobs, Options{Scale: 50},
		func(j []Job) error { before += len(j); return nil },
		func(r []Result) error { after += len(r); return nil },
		func(Progress) { prog++ })
	if err != nil {
		t.Fatal(err)
	}
	if len(enc.calls) != 2 || before != 3 || after != 3 || prog != 2 {
		t.Fatalf("calls %d before %d after %d progress %d", len(enc.calls), before, after, prog)
	}
	if !res[0].OK || res[1].OK || !res[2].OK || res[1].Err != "no DDS written" {
		t.Fatalf("%+v", res)
	}
	// a stop between batches
	ctx, cancel := context.WithCancel(context.Background())
	enc2 := &fakeEnc{}
	res, err = Run(ctx, enc2, p.Jobs, Options{}, nil, nil, func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) || len(enc2.calls) != 1 {
		t.Fatalf("stop: %v, %d calls", err, len(enc2.calls))
	}
	_ = res
}

func TestArgs(t *testing.T) {
	got := strings.Join(Args("l.txt", 50, false, 8), " ")
	if got != "-f BC1 -af BC7 -vf -fs -o -nm -sc 50 -th 8 l.txt" {
		t.Fatal(got)
	}
	if got := strings.Join(Args("l.txt", 100, true, 0), " "); got != "-f BC1 -af BC7 -vf -fs -o l.txt" {
		t.Fatal(got)
	}
}

// With a real todds (ARCHO_TODDS=path\todds.exe): the DDS it writes is what DDSSize expects
// and what the game reads.
func TestRealTodds(t *testing.T) {
	exe := os.Getenv("ARCHO_TODDS")
	if exe == "" {
		t.Skip("ARCHO_TODDS not set")
	}
	td := &Todds{Path: exe}
	v, err := td.Version()
	if err != nil || v != ToddsVersion {
		t.Fatalf("version %q %v", v, err)
	}
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "Textures", "Opaque.png"), 256, 128, false)
	writePNG(t, filepath.Join(root, "Textures", "Alpha.png"), 300, 200, true)
	writePNG(t, filepath.Join(root, "Textures", "Big.png"), 512, 512, false)
	set, _ := Collect(root, []string{root})
	o := Options{Scale: 50}
	p := MakePlan(set, o, Owned{}, false)
	res, err := Run(context.Background(), td, p.Jobs, o, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if !r.OK {
			t.Fatalf("%s: %s", r.Rel, r.Err)
		}
		in, _ := ReadDDS(r.DDS)
		want := DDSSize(r.W, r.H, r.Alpha, r.Scale, true)
		t.Logf("%s %dx%d scale %d → %+v, %d bytes (estimated %d)", r.Rel, r.W, r.H, r.Scale, in, r.DDSSize, want)
		if r.DDSSize != want {
			t.Errorf("%s: %d bytes, estimated %d", r.Rel, r.DDSSize, want)
		}
		if wantF := map[bool]string{true: "BC7", false: "BC1"}[r.Alpha]; in.Format != wantF {
			t.Errorf("%s: %s, want %s", r.Rel, in.Format, wantF)
		}
	}
}
