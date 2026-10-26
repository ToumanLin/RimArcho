package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// A background mod's Defs name its images; a Def whose image is missing is left out.
func TestModBackgroundsFromDefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "1.6", "Defs"), 0o755)
	os.WriteFile(filepath.Join(dir, "1.6", "Defs", "Backgrounds.xml"), []byte(`<?xml version="1.0"?>
<Defs>
  <VBE.BackgroundImageDef><label>Adventure</label><defName>VBE_Adventure</defName><path>Adventure</path></VBE.BackgroundImageDef>
  <VBE.BackgroundImageDef MayRequire="x"><label>Gone</label><defName>VBE_Gone</defName><path>Gone</path></VBE.BackgroundImageDef>
  <VBE.BackgroundImageDef><defName>Deep</defName><path>Menu/Deep</path></VBE.BackgroundImageDef>
</Defs>`), 0o644)
	writePNG(t, filepath.Join(dir, "Textures", "Adventure.png"), 40, 20)
	writePNG(t, filepath.Join(dir, "Extra", "Textures", "Menu", "Deep.png"), 40, 20)
	got := modBackgrounds(dir)
	if len(got) != 2 || got[0].Key != "VBE_Adventure" || got[0].Label != "Adventure" || got[1].Label != "Deep" {
		t.Fatalf("got %+v", got)
	}
}

// Without Defs, the large landscape textures count; small or tall ones do not.
func TestModBackgroundsWithoutDefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "Textures", "UI", "Wide.png"), 1600, 900)
	writePNG(t, filepath.Join(dir, "Textures", "UI", "Small.png"), 300, 100)
	writePNG(t, filepath.Join(dir, "Textures", "UI", "Tall.png"), 1300, 2000)
	got := modBackgrounds(dir)
	if len(got) != 1 || got[0].Key != "ui/wide" {
		t.Fatalf("got %+v", got)
	}
}

// One decode makes both cached sizes, never wider than the original.
func TestBackgroundImageCache(t *testing.T) {
	t.Parallel()
	a := &App{Home: t.TempDir()}
	src := filepath.Join(t.TempDir(), "bg.png")
	writePNG(t, src, 1000, 500)
	thumb, err := a.BackgroundImage(src, BgThumb)
	if err != nil {
		t.Fatal(err)
	}
	full, err := a.BackgroundImage(src, BgFull)
	if err != nil {
		t.Fatal(err)
	}
	for path, w := range map[string]int{thumb: BgThumb, full: 1000} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != w || cfg.Height != w/2 {
			t.Fatalf("%s: %v %dx%d, want width %d", path, err, cfg.Width, cfg.Height, w)
		}
	}
}

// Transparent pixels go to black, opaque ones keep their colour.
func TestScaleDownAlpha(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	img.Set(1, 0, color.NRGBA{255, 255, 255, 0})
	out := scaleDown(img, 2, 1)
	if r, _, _, _ := out.At(0, 0).RGBA(); r>>8 != 255 {
		t.Fatalf("opaque red became %v", out.At(0, 0))
	}
	if r, g, b, _ := out.At(1, 0).RGBA(); r|g|b != 0 {
		t.Fatalf("transparent became %v", out.At(1, 0))
	}
}

func TestOfficialBackgroundPreviewFallbacks(t *testing.T) {
	t.Parallel()
	core, dlc := t.TempDir(), t.TempDir()
	writePNG(t, filepath.Join(core, "About", "pReViEw.PNG"), 40, 20)
	writePNG(t, filepath.Join(dlc, "About", "Preview.png"), 40, 20)
	got := CollectBackgrounds([]BackgroundTarget{
		{Dir: dlc, Name: "Royalty", PackageID: "Ludeon.RimWorld.Royalty", Official: true},
		{Dir: core, Name: "Core", PackageID: "Ludeon.RimWorld", Official: true},
		{Dir: t.TempDir(), Name: "Missing", PackageID: "Ludeon.RimWorld.Missing", Official: true},
	})
	if len(got) != 2 || got[0].Key != BgDefault || got[1].Key != "game:ludeon.rimworld.royalty" || got[0].Group != "" || got[0].Thumb != "" {
		t.Fatalf("official previews: %+v", got)
	}
}

// Every copy of a background mod with images is listed, each saying where it is; a copy
// without its textures (another game's local copy) is left out and hides nothing.
func TestBackgroundsEveryCopyListed(t *testing.T) {
	t.Parallel()
	empty, ws, local := t.TempDir(), t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(empty, "About"), 0o755)
	writePNG(t, filepath.Join(ws, "Textures", "UI", "Wide.png"), 1600, 900)
	writePNG(t, filepath.Join(local, "Textures", "UI", "Wide.png"), 1600, 900)
	got := CollectBackgrounds([]BackgroundTarget{
		{Dir: empty, Name: "Shella", LineageID: "L", ID: "a", Source: "local-1.6"},
		{Dir: ws, Name: "Shella", LineageID: "L", ID: "b", Source: "workshop"},
	})
	if len(got) != 1 || got[0].Key != "mod:L/ui/wide" || got[0].Group != "Shella" {
		t.Fatalf("one copy with images: %+v", got)
	}
	got = CollectBackgrounds([]BackgroundTarget{
		{Dir: local, Name: "Shella", LineageID: "L", ID: "a", Source: "local-1.6"},
		{Dir: ws, Name: "Shella", LineageID: "L", ID: "b", Source: "workshop"},
	})
	if len(got) != 2 || got[0].Key != "mod:L@a/ui/wide" || got[0].Group != "Shella  local-1.6" || got[1].Key != "mod:L@b/ui/wide" || got[1].Group != "Shella  workshop" {
		t.Fatalf("two copies: %+v", got)
	}
	if !SameBackground("mod:L/ui/wide", "mod:L@b/ui/wide") || !SameBackground("mod:L@a/ui/wide", "mod:L/ui/wide") || SameBackground("mod:L/ui/x", "mod:L@a/ui/wide") || SameBackground("mod:M/ui/wide", "mod:L/ui/wide") {
		t.Fatal("saved keys across copies")
	}
}
