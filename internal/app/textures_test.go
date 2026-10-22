package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

func texPNG(t *testing.T, p string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.SetNRGBA(0, 0, color.NRGBA{1, 2, 3, 255})
	os.MkdirAll(filepath.Dir(p), 0o755)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func texDDS(p string) error {
	b := make([]byte, 128)
	copy(b, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(b[4:], 124)
	le.PutUint32(b[12:], 4)
	le.PutUint32(b[16:], 4)
	le.PutUint32(b[76:], 32)
	le.PutUint32(b[80:], 0x4)
	copy(b[84:], "DXT1")
	return os.WriteFile(p, b, 0o644)
}

// texEnc writes a DDS header next to each PNG, except those containing fail.
type texEnc struct {
	fail  string
	files int
}

func (f *texEnc) Encode(ctx context.Context, files []string, scale int, mips bool) error {
	for _, p := range files {
		if f.fail != "" && strings.Contains(filepath.ToSlash(p), f.fail) {
			continue
		}
		f.files++
		if err := texDDS(strings.TrimSuffix(p, ".png") + ".dds"); err != nil {
			return err
		}
	}
	return nil
}

func texSetup(t *testing.T) (*testkit.Env, *texEnc, string) {
	e := setup(t, false)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	enc := &texEnc{}
	e.App.TextureEncoder = enc
	dir := e.AddWorkshop("3001", testkit.Sample("tester.tex", "Textured"))
	texPNG(t, filepath.Join(dir, "Textures", "Things", "A.png"), 64, 64)
	texPNG(t, filepath.Join(dir, "1.6", "Textures", "Things", "A.png"), 64, 64)
	texPNG(t, filepath.Join(dir, "1.5", "Textures", "Things", "B.png"), 64, 64)
	texPNG(t, filepath.Join(dir, "Textures", "Own.png"), 64, 64)
	if err := texDDS(filepath.Join(dir, "Textures", "Own.dds")); err != nil { // the author's
		t.Fatal(err)
	}
	scanNow(t, e)
	return e, enc, dir
}

// Optimising writes the DDS next to every PNG of a key (the 1.5 folder too: the Workshop
// folder serves every game version), never touches the author's DDS, is not an update of
// the mod, does nothing the second time, and clean takes back only its own files.
func TestTexturesOptimize(t *testing.T) {
	e, enc, dir := texSetup(t)
	ref := "pkg:tester.tex"
	st, err := e.App.TextureStatus([]string{ref}, app.TexRefs, textures.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := st.Mods[0]
	if m.PNG != 4 || m.Foreign != 1 || len(m.Plan.Jobs) != 3 || m.Plan.Skipped[textures.SkipOwnDDS] != 1 {
		t.Fatalf("status: png %d foreign %d jobs %d skipped %v", m.PNG, m.Foreign, len(m.Plan.Jobs), m.Plan.Skipped)
	}
	snap := one(t, e, "Textured").SnapHash
	r, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Converted != 3 || len(r.Failed) != 0 {
		t.Fatalf("converted %d failed %v", r.Converted, r.Failed)
	}
	for _, p := range []string{"Textures/Things/A.dds", "1.6/Textures/Things/A.dds", "1.5/Textures/Things/B.dds"} {
		if !exists(filepath.Join(dir, filepath.FromSlash(p))) {
			t.Fatalf("%s missing", p)
		}
	}
	// a rescan: the DDS are not part of the mod
	id := scanNow(t, e)
	if s := one(t, e, "Textured").SnapHash; !bytes.Equal(s, snap) {
		t.Fatal("the generated DDS changed the snapshot")
	}
	if k := kinds(eventsOf(t, e, id)); k["updated"] != 0 {
		t.Fatalf("events %v", k)
	}
	// nothing to do the second time
	enc.files = 0
	r, _ = e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}})
	if r.Converted != 0 || enc.files != 0 || r.Mods[0].Plan.Skipped[textures.SkipFresh] != 3 {
		t.Fatalf("second run: %d converted, %v", r.Converted, r.Mods[0].Plan.Skipped)
	}
	// another scale re-encodes
	r, _ = e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}, Options: textures.Options{Scale: 50, MinSide: 16}})
	if r.Converted != 3 {
		t.Fatalf("at 50%%: %d", r.Converted)
	}
	// clean: ours go, the author's stays, the PNGs stay
	c, err := e.App.CleanTextures([]string{ref}, app.TexRefs, false)
	if err != nil || c.Files != 3 {
		t.Fatalf("clean %+v %v", c, err)
	}
	if exists(filepath.Join(dir, "Textures", "Things", "A.dds")) || !exists(filepath.Join(dir, "Textures", "Own.dds")) || !exists(filepath.Join(dir, "Textures", "Things", "A.png")) {
		t.Fatal("clean deleted the wrong files")
	}
	if fs, _ := e.App.Store.TextureFilesOf(nil); len(fs) != 0 {
		t.Fatalf("records left: %d", len(fs))
	}
}

// A key that fails in one folder loses its DDS in every folder; a DDS whose PNG is gone is
// deleted; an update of the mod makes its DDS stale until they are made again.
func TestTexturesStaleAndFailures(t *testing.T) {
	e, enc, dir := texSetup(t)
	ref := "pkg:tester.tex"
	enc.fail = "1.6/Textures/Things/A.png"
	r, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Failed) != 1 || exists(filepath.Join(dir, "Textures", "Things", "A.dds")) || !exists(filepath.Join(dir, "1.5", "Textures", "Things", "B.dds")) {
		t.Fatalf("failed %v; common A.dds left: %v", r.Failed, exists(filepath.Join(dir, "Textures", "Things", "A.dds")))
	}
	enc.fail = ""
	if r, _ = e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}}); r.Converted != 2 {
		t.Fatalf("again: %d", r.Converted)
	}
	// the mod updates: B changes, A in 1.6 goes away
	e.Advance(3600e9)
	texPNG(t, filepath.Join(dir, "1.5", "Textures", "Things", "B.png"), 128, 64)
	os.Remove(filepath.Join(dir, "1.6", "Textures", "Things", "A.png"))
	scanNow(t, e)
	ix, _ := e.App.Index()
	if s := e.App.StaleTextures(ix); len(s) != 1 {
		t.Fatalf("stale: %d", len(s))
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ps.List {
		found = found || p.Code == "texture.stale"
	}
	if !found {
		t.Fatal("no texture.stale problem")
	}
	st, _ := e.App.TextureStatus([]string{ref}, app.TexRefs, textures.Options{})
	if m := st.Mods[0]; m.Stale != 1 || m.Orphans != 1 {
		t.Fatalf("stale %d orphans %d", m.Stale, m.Orphans)
	}
	r, _ = e.App.OptimizeTextures(app.TextureRun{Refs: []string{ref}})
	if r.Converted != 1 || r.Removed != 1 || exists(filepath.Join(dir, "1.6", "Textures", "Things", "A.dds")) {
		t.Fatalf("after the update: converted %d removed %d", r.Converted, r.Removed)
	}
	ix, _ = e.App.Index()
	if s := e.App.StaleTextures(ix); len(s) != 0 {
		t.Fatal("still stale")
	}
}

// A game running, Core, and a DDS that appeared since the plan are refused.
func TestTexturesRefusals(t *testing.T) {
	e, _, _ := texSetup(t)
	if _, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:ludeon.rimworld"}}); err == nil || !strings.Contains(err.Error(), "part of the game") {
		t.Fatalf("core: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:tester.tex"}}); !errors.Is(err, app.ErrGameRuns) {
		t.Fatalf("game: %v", err)
	}
	if _, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:tester.tex"}, DryRun: true}); err != nil {
		t.Fatalf("a dry run while the game runs: %v", err)
	}
}

func realDir(p string) string {
	if r, err := source.RealPath(p); err == nil {
		return r
	}
	return p
}

// Uninstall deletes the DDS RimArcho wrote and nothing else of the mod (01 §4.1).
func TestTexturesUninstall(t *testing.T) {
	e, _, dir := texSetup(t)
	if _, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:tester.tex"}}); err != nil {
		t.Fatal(err)
	}
	p, err := e.App.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := p.Step(app.UnTextures)
	if s == nil || len(s.Items) != 1 || s.Items[0].Do != app.DoDDS || !strings.EqualFold(realDir(s.Items[0].Path), realDir(dir)) || s.Bytes() == 0 {
		t.Fatalf("step %+v %q vs %q", *s.Items[0], s.Items[0].Path, dir)
	}
	if _, err := e.App.Uninstall(app.UninstallOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "Textures", "Things", "A.dds")) || !exists(filepath.Join(dir, "Textures", "Things", "A.png")) || !exists(filepath.Join(dir, "Textures", "Own.dds")) || !exists(filepath.Join(dir, "About", "About.xml")) {
		t.Fatal("uninstall touched the wrong files")
	}
}

// todds is downloaded from the release (a fake server here) into RimArcho's own folder.
func TestInstallTodds(t *testing.T) {
	e := setup(t, false)
	e.App.TextureEncoder = &texEnc{}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for _, n := range []string{"todds_Windows_0.4.1/todds.exe", "todds_Windows_0.4.1/LICENSE", "todds_Windows_0.4.1/README.md"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(zb.Bytes()) }))
	defer srv.Close()
	e.App.ToddsZip = srv.URL + "/todds.zip"
	e.App.ToddsNoPath = true
	if e.App.ToddsPath() != "" {
		t.Fatal("found a todds before installing")
	}
	exe, err := e.App.InstallTodds()
	if err != nil {
		t.Fatal(err)
	}
	if exe != filepath.Join(e.App.ToddsDir(), "todds.exe") || e.App.ToddsPath() != exe || !exists(filepath.Join(e.App.ToddsDir(), "LICENSE.txt")) || exists(filepath.Join(e.App.ToddsDir(), "README.md")) {
		t.Fatalf("installed %s; found %s", exe, e.App.ToddsPath())
	}
}
