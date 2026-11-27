package app_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func generatedMod(t *testing.T, ix *app.Index, pkg string) *store.Mod {
	t.Helper()
	for _, m := range ix.Mods {
		if app.IsGenerated(m) && strings.EqualFold(m.PackageID, pkg) {
			return m
		}
	}
	t.Fatalf("no generated Mod %s", pkg)
	return nil
}

// 16 §2.9 "Generated Mods": the actions on a Mod's folder and identity are refused for
// RimArcho Looks Preference and RimArcho Styles Preference, in the app layer (so the CLI,
// MCP and the window get the same answer), and their folders stay as they were. The Mod
// library names their source, and they do not count as Mods the player added.
func TestGeneratedModsRefuseModActions(t *testing.T) {
	t.Parallel()
	e := stylesEnv(t, "")
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	addStyle(t, e, app.StyleAddInput{Look: hatLook, Mod: "pkg:looks.a", Kind: app.StyleKindStyle, Target: "Rustic"})
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	dir := e.App.LooksDir()
	before := modsDigest(t, dir)
	ix := mustIndex(t, e)
	other := ix.ByID[func() string {
		for _, m := range ix.Mods {
			if m.PackageID == "looks.c" {
				return m.ID
			}
		}
		return ""
	}()]
	if other == nil {
		t.Fatal("fixture: no Mod C")
	}
	for _, pkg := range []string{app.LooksPackage, app.StylesPackage} {
		m := generatedMod(t, ix, pkg)
		ref := ix.Ref(m.ID)
		refused := func(what string, err error) {
			t.Helper()
			if err == nil || !errors.Is(err, app.ErrGenerated) || !strings.Contains(err.Error(), "archo looks") || !strings.Contains(err.Error(), m.Name) {
				t.Errorf("%s of %s must be refused as generated: %v", what, m.Name, err)
			}
		}
		for _, dry := range []bool{true, false} {
			_, err := e.App.Trash(ref, dry)
			refused("recycle", err)
			_, err = e.App.Pin(ref, "", false, dry)
			refused("lock", err)
			_, err = e.App.Pin(ref, "Copy", true, dry)
			refused("local copy", err)
			_, err = e.App.PinUpdate(ref, "", false, dry)
			refused("lock update", err)
			_, err = e.App.MoveToLocked(ref, dry)
			refused("move to the lock library", err)
			_, err = e.App.UpdateToWorkshop(ref, dry)
			refused("update to the Workshop version", err)
		}
		_, err := e.App.SplitLineage(ix, m)
		refused("split", err)
		refused("merge into", e.App.MergeLineages(ix, m.LineageID, other.LineageID))
		refused("merge from", e.App.MergeLineages(ix, other.LineageID, m.LineageID))
		for _, args := range [][]string{{"trash", ref}, {"pin", ref}, {"copy-local", ref}, {"lineage", "split", ref}} {
			if out, code := e.Run(args...); code == 0 || !strings.Contains(out, "archo looks") {
				t.Errorf("archo %s: exit %d\n%s", strings.Join(args, " "), code, out)
			}
		}
		// what stays: player data and the plan's own edits
		if err := e.App.SetNote(ref, "mine"); err != nil {
			t.Errorf("a note on %s: %v", m.Name, err)
		}
	}
	if modsDigest(t, dir) != before {
		t.Fatal("a refused action changed the looks directory")
	}
	if filepath.Base(dir) == "" || joined(active(t, e)) != "Core, A, B, C, S, "+app.StylesName+", "+app.LooksName {
		t.Fatalf("plan: %s", joined(active(t, e)))
	}

	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	added := app.Added(st)
	seen := 0
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if !app.IsGenerated(i.Mod) {
				continue
			}
			seen++
			if i.Source != app.LibLooks {
				t.Errorf("%s: library source %q, want %q", i.Mod.Name, i.Source, app.LibLooks)
			}
			if at, ok := added[i.Mod.LineageID]; ok && !at.IsZero() {
				t.Errorf("%s counts as a Mod the player added", i.Mod.Name)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("the library lists %d generated Mods", seen)
	}
}

// A list exported with the generated Mods in it is imported on a machine that has neither
// like any list with Mods that are not installed: reported, not failed.
func TestGeneratedModsMissingOnImport(t *testing.T) {
	t.Parallel()
	e := stylesEnv(t, "")
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	addStyle(t, e, app.StyleAddInput{Look: hatLook, Mod: "pkg:looks.a", Kind: app.StyleKindStyle, Target: "Rustic"})
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	list := filepath.Join(e.Root, "plan.archolist.json")
	e.MustRun("loadout", "export", "--format", "archo", "--out", list)
	ids := filepath.Join(e.Root, "plan.txt")
	e.MustRun("loadout", "export", "--format", "text", "--out", ids)

	o := stylesEnv(t, "") // another machine: the same Mods, no looks directory
	if o.App.LooksDir() != "" {
		t.Fatal("fixture: the other home has a looks directory")
	}
	out := o.MustRun("pack", "import", list)
	if !strings.Contains(out, "not installed: 2") || !strings.Contains(out, app.LooksName) || !strings.Contains(out, app.StylesName) {
		t.Fatalf("pack import of the list:\n%s", out)
	}
	for _, f := range []string{list, ids} {
		out = o.MustRun("loadout", "import", f)
		got := joined(active(t, o))
		if !strings.Contains(got, "?"+app.LooksPackage) || !strings.Contains(got, "?"+app.StylesPackage) || !strings.Contains(got, "A, B, C, S") {
			t.Fatalf("after importing %s the plan is %s\n%s", filepath.Base(f), got, out)
		}
	}
}
