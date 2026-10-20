package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func hide(e *testkit.Env, dir string) {
	e.Move(filepath.Join(dir, "About", "About.xml"), filepath.Join(dir, "About", "About.xml.disabled"))
}

func problemCodes(t *testing.T, e *testkit.Env) map[string]string {
	t.Helper()
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range ps.List {
		if strings.HasPrefix(p.Code, "mod.") {
			out[p.Code] = string(p.Severity)
		}
	}
	return out
}

// H1, H2: a RimCrow-style pin (hidden workshop copy, enabled Mods/ copy) is "pinned", not
// unloadable; once the workshop copy changes after the copy was made, it is a warning with
// a file diff.
func TestHiddenPinned(t *testing.T) {
	e := setup(t, false)
	m := testkit.Sample("tester.pin", "Pinned")
	m.Files = map[string]string{"Defs/a.xml": "<Defs><ThingDef><defName>A</defName></ThingDef></Defs>"}
	ws := e.AddWorkshop("4100", m)
	e.CopyTree(ws, filepath.Join(e.Game, "Mods", "_4100_"))
	hide(e, ws)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.pin")
	scanNow(t, e)
	got := problemCodes(t, e)
	if got["mod.pinned"] != "info" || got["mod.unloadable"] != "" {
		t.Fatalf("H1: %v", got)
	}
	st, _ := e.App.Loadout()
	h := st.HiddenOf(refID(t, e, "Pinned", source.Workshop))
	if h == nil || h.Kind != app.HiddenPinned || h.Upstream {
		t.Fatalf("H1: %+v", h)
	}

	e.Advance(48 * 3600e9)
	e.WriteFile(filepath.Join(ws, "Defs", "b.xml"), "<Defs><ThingDef><defName>B</defName></ThingDef></Defs>")
	e.SetTimeUpdated("4100", e.Now().Unix())
	scanNow(t, e)
	if got := problemCodes(t, e); got["mod.pinned"] != "warning" {
		t.Fatalf("H2: %v", got)
	}
	out := e.MustRun("mod", refOf(t, e, "Pinned", source.Local), "--diff", refOf(t, e, "Pinned", source.Workshop))
	if !strings.Contains(out, "+ defs/b.xml") || strings.Contains(out, "about.xml") {
		t.Fatalf("H2 diff:\n%s", out)
	}

	// Unpin: the workshop copy is restored and replaces the pinning copy in the workspace.
	e.MustRun("unpin", refOf(t, e, "Pinned", source.Workshop))
	if _, err := os.Stat(filepath.Join(ws, "About", "About.xml")); err != nil {
		t.Fatal("unpin did not restore About.xml")
	}
	st, _ = e.App.Loadout()
	if st.List.Index(refID(t, e, "Pinned", source.Workshop)) < 0 {
		t.Fatal("unpin did not switch the workspace to the workshop copy")
	}
}

// H3: an unsubscribed hidden folder goes to the recycle bin (here: a recorder).
func TestHiddenResidueTrash(t *testing.T) {
	e := setup(t, false)
	ws := e.AddWorkshop("4200", testkit.Sample("tester.gone", "Gone"))
	hide(e, ws)
	e.Unsubscribe("4200")
	scanNow(t, e)
	if got := problemCodes(t, e); got["mod.hidden-residue"] != "warning" {
		t.Fatalf("H3: %v", got)
	}
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, p); return os.RemoveAll(p) }
	ref := refOf(t, e, "Gone", "")
	out := e.MustRun("trash", ref, "--dry-run")
	if !strings.Contains(out, "dry run") || len(trashed) != 0 {
		t.Fatalf("dry run:\n%s", out)
	}
	e.MustRun("trash", ref)
	if len(trashed) != 1 || !strings.EqualFold(trashed[0], mustReal(t, filepath.Dir(ws))+string(filepath.Separator)+"4200") {
		t.Fatalf("trashed %v", trashed)
	}
	if m := one(t, e, "Gone"); m.Status == identity.Live {
		t.Fatal("still live after trash + rescan")
	}
}

// H4, H5, H6: a hidden subscribed mod can be shown again; an existing About.xml is never
// overwritten; local folders are never trashed.
func TestHiddenAlone(t *testing.T) {
	e := setup(t, false)
	ws := e.AddWorkshop("4300", testkit.Sample("tester.alone", "Alone"))
	hide(e, ws)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Mine"), testkit.Sample("tester.mine", "Mine"))
	scanNow(t, e)
	if got := problemCodes(t, e); got["mod.hidden"] != "warning" {
		t.Fatalf("H4: %v", got)
	}
	ref := refOf(t, e, "Alone", "")
	e.WriteFile(filepath.Join(ws, "About", "About.xml"), "<ModMetaData/>")
	if out, code := e.Run("unhide", ref); code == 0 || !strings.Contains(out, "does not overwrite") {
		t.Fatalf("H5: exit %d\n%s", code, out)
	}
	e.Remove(filepath.Join(ws, "About", "About.xml"))
	e.MustRun("unhide", ref)
	e.MustRun("enable", refOf(t, e, "Alone", ""))
	if out, code := e.Run("trash", refOf(t, e, "Mine", "")); code == 0 || !strings.Contains(out, "src:local") {
		t.Fatalf("H6: exit %d\n%s", code, out)
	}
}

func refID(t *testing.T, e *testkit.Env, name string, kind source.Kind) string {
	t.Helper()
	for _, m := range named(t, e, name) {
		if kind == "" || m.Kind == kind {
			return m.ID
		}
	}
	t.Fatalf("no %s instance of %s", kind, name)
	return ""
}
