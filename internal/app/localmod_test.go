package app_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 16 §2.9, 13 (subscribe local): a Mod folder or zip dropped on the window is copied into the
// game's Mods folder as a local Mod under a free name; a dry run changes nothing; something
// that is not a Mod is refused before anything is copied.
func TestInstallLocalMod(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")

	// a folder, its Mod inside one wrapper folder
	src := filepath.Join(e.Root, "Downloads", "Colony Tweaks")
	e.WriteMod(filepath.Join(src, "Colony Tweaks 1.2"), testkit.Sample("tester.tweaks", "Colony Tweaks"))
	dry, err := e.App.InstallLocalMod(src, true)
	if err != nil || !dry.DryRun || dry.Items[0].Folder != filepath.Join(e.Game, "Mods", "Colony Tweaks") {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if _, err := os.Stat(dry.Items[0].Folder); !os.IsNotExist(err) {
		t.Fatal("a dry run must not copy")
	}
	op, err := e.App.InstallLocalMod(src, false)
	if err != nil || op.Items[0].Result != "installed" || op.Items[0].Name != "Colony Tweaks" || op.Items[0].Ref == "" {
		t.Fatalf("install: %+v %v", op, err)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "Colony Tweaks", "About", "About.xml")); err != nil {
		t.Fatal("the Mod is not in the Mods folder:", err)
	}
	if out := e.MustRun("mod", "tester.tweaks"); !strings.Contains(out, "Colony Tweaks") {
		t.Fatalf("the scan should know it:\n%s", out)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("the source must stay")
	}

	// a zip with the same name gets a free one
	z := filepath.Join(e.Root, "Downloads", "Colony Tweaks.zip")
	f, _ := os.Create(z)
	w := zip.NewWriter(f)
	about, _ := w.Create("About/About.xml")
	about.Write([]byte(testkit.Sample("tester.tweaks2", "Colony Tweaks Two").AboutXML()))
	w.Close()
	f.Close()
	op, err = e.App.InstallLocalMod(z, false)
	if err != nil || op.Items[0].Folder != filepath.Join(e.Game, "Mods", "Colony Tweaks-2") {
		t.Fatalf("zip: %+v %v", op, err)
	}

	// not a Mod
	junk := filepath.Join(e.Root, "Downloads", "Screenshots")
	os.MkdirAll(filepath.Join(junk, "a"), 0o755)
	os.MkdirAll(filepath.Join(junk, "b"), 0o755)
	if _, err := e.App.InstallLocalMod(junk, true); err == nil || !strings.Contains(err.Error(), "not a Mod folder") {
		t.Fatalf("a folder with no Mod: %v", err)
	}
	if _, err := e.App.InstallLocalMod(filepath.Join(e.Root, "nothing.zip"), true); err == nil {
		t.Fatal("a missing zip must be refused")
	}
}
