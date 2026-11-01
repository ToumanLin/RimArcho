package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// preconvert in a pack (08 §5.2): the copies' PNGs become DDS and the PNGs go; a key that failed
// keeps its PNGs and loses its DDS; the author's DDS and its PNG stay; the mod itself is
// never touched; pack.json and origin.json say what was done.
func TestPackTextures(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	enc := &texEnc{fail: "Things/Bad.png"}
	e.App.TextureEncoder = enc
	src := filepath.Join(e.Game, "Mods", "Textured")
	e.WriteMod(src, testkit.Sample("tester.textured", "Textured"))
	texPNG(t, filepath.Join(src, "Textures", "Things", "A.png"), 64, 64)
	texPNG(t, filepath.Join(src, "1.6", "Textures", "Things", "A.png"), 64, 64)
	texPNG(t, filepath.Join(src, "1.5", "Textures", "Things", "Old.png"), 64, 64) // trimmed
	texPNG(t, filepath.Join(src, "Textures", "Things", "Bad.png"), 64, 64)
	texPNG(t, filepath.Join(src, "Textures", "Own.png"), 64, 64)
	texDDS(filepath.Join(src, "Textures", "Own.dds"))
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.textured")
	// the mod already has RimArcho's own DDS next to its PNGs: they are not the mod's, so
	// the pack neither copies them nor treats them as the author's
	if r, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:tester.textured"}}); err != nil || r.Converted != 3 { // 1.5 too: in place covers every version folder
		t.Fatalf("in place: %+v %v", r, err)
	}
	enc.files = 0

	dry, err := e.App.BuildPackWith("tex", app.BuildOptions{DryRun: true, Textures: &textures.Options{}})
	if err != nil {
		t.Fatal(err)
	}
	if dry.TexEst.Files != 3 || dry.TexSkipped != 1 || dry.Final == dry.Need || dry.Peak < dry.Final {
		t.Fatalf("dry run: files %d skipped %d need %d final %d peak %d", dry.TexEst.Files, dry.TexSkipped, dry.Need, dry.Final, dry.Peak)
	}
	r, err := e.App.BuildPackWith("tex", app.BuildOptions{Textures: &textures.Options{}})
	if err != nil {
		t.Fatal(err)
	}
	pt := r.Pack.Textures
	if pt == nil || pt.Converted != 2 || pt.Failed != 1 || pt.Kept != 2 || pt.PNGBytes == 0 {
		t.Fatalf("pack textures %+v", pt)
	}
	if problems, err := e.App.VerifyPack("tex"); err != nil || len(problems) != 0 {
		t.Fatalf("digest must cover the converted textures: %+v %v", problems, err)
	}
	var folder string
	for _, pe := range r.Pack.Entries {
		if pe.PackageID == "tester.textured" {
			folder = pe.Folder
		}
	}
	mod := filepath.Join(e.Home, "packs", "tex", "mods", folder)
	for rel, want := range map[string]bool{
		"Textures/Things/A.png": false, "Textures/Things/A.dds": true,
		"1.6/Textures/Things/A.png": false, "1.6/Textures/Things/A.dds": true,
		"Textures/Things/Bad.png": true, "Textures/Things/Bad.dds": false,
		"Textures/Own.png": true, "Textures/Own.dds": true, "1.5": false,
	} {
		if exists(filepath.Join(mod, filepath.FromSlash(rel))) != want {
			t.Errorf("%s present=%v", rel, !want)
		}
	}
	if !exists(filepath.Join(src, "Textures", "Things", "A.png")) || !exists(filepath.Join(src, "Textures", "Things", "A.dds")) {
		t.Fatal("the mod itself was changed")
	}
	var og scan.Origin
	b, _ := os.ReadFile(filepath.Join(mod, filepath.FromSlash(scan.OriginFile)))
	if json.Unmarshal(b, &og) != nil || og.Textures != "dds" {
		t.Fatalf("origin %s", b)
	}
	b, _ = os.ReadFile(filepath.Join(e.Home, "packs", "tex", "pack.json"))
	if !strings.Contains(string(b), `"converted": 2`) {
		t.Fatalf("pack.json:\n%s", b)
	}
	// the CLI flags reach the same build; the exported list drops the textures record
	out := e.MustRun("pack", "build", "tex2", "--textures", "--texture-scale", "50", "--dry-run")
	if !strings.Contains(out, "textures: slim to 50%") {
		t.Fatalf("cli dry run:\n%s", out)
	}
	file := filepath.Join(e.Root, "tex.archolist.json")
	if _, err := e.App.ExportPack("tex", file, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); strings.Contains(string(b), "textures") {
		t.Fatal("the exported list kept the textures record")
	}
	if got := strings.Join(app.PackBuildArgs(&textures.Options{Scale: 50}), " "); got != "--textures --texture-scale 50" {
		t.Fatal(got)
	}
}
