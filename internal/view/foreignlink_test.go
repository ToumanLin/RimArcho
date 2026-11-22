package view_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Another manager's link in a game's Mods/ is how the game reaches a Mod, not a copy of it
// (04 §1.3 "Other managers' links", 16 §2.9): no copy row, no copy count, no copy actions;
// the Mod's details say who links it in. The game still loads it, so the index, the
// lists and the save preview keep it.

func lineageNamed(t *testing.T, lib *app.Library, name string) *app.LibLineage {
	t.Helper()
	for _, l := range lib.Lineages {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no lineage %q", name)
	return nil
}

func foreignRow(t *testing.T, lib *app.Library, name string) view.LibRow {
	t.Helper()
	p, err := view.BuildLibrary(lib, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.Rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q", name)
	return view.LibRow{}
}

func linkedField(fs []view.Field) *view.Field {
	for i := range fs {
		if fs[i].Kind == view.FieldLinked {
			return &fs[i]
		}
	}
	return nil
}

// A `_Link_` into a folder RimArcho scans (here the Workshop folder) is that copy: one
// Instance, the link an alias of it (04 §5 A11), and nothing to say about it.
func TestForeignLinkToScannedFolder(t *testing.T) {
	e := testkit.New(t)
	ws := e.AddWorkshop("2880970348", testkit.Sample("cixwow.pump", "Pump"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_2880970348"), ws)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "cixwow.pump")
	e.AddEnv(false)
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	l := lineageNamed(t, lib, "Pump")
	if len(l.Instances) != 1 || len(l.Linked) != 0 || l.Instances[0].Source != app.LibWorkshop || l.Instances[0].Link != "" {
		t.Fatalf("copies %d linked %d", len(l.Instances), len(l.Linked))
	}
	if f := linkedField(view.LibFields(e.App, lib, l.Instances[0], nil)); f != nil {
		t.Fatalf("a link to a scanned folder has a linked field: %+v", f)
	}
	if r := foreignRow(t, lib, "Pump"); r.Copies != 1 || !r.Single {
		t.Fatalf("row: %d copies", r.Copies)
	}
}

// A `_Link_` into RimCrow's own folder, beside the Workshop copy: the Mod keeps one copy,
// the link folds into Linked, and the details offer "show where it is". The index still
// has the Instance, so the lists that enable it, the save preview and `archo trash`'s
// refusal are unchanged.
func TestForeignLinkToUnscannedFolder(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("2880970348", testkit.Sample("cixwow.pump", "Pump"))
	outside := filepath.Join(e.Root, "RimCrow", "mods", "2880970348")
	e.WriteMod(outside, testkit.Sample("cixwow.pump", "Pump"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_2880970348"), outside)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "cixwow.pump")
	e.AddEnv(false)
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	l := lineageNamed(t, lib, "Pump")
	if len(l.Instances) != 1 || l.Instances[0].Source != app.LibWorkshop {
		t.Fatalf("copies: %d", len(l.Instances))
	}
	if len(l.Linked) != 1 || l.Linked[0].LinkedBy != "RimCrow" || !strings.HasSuffix(l.Linked[0].Link, "_Link_2880970348") {
		t.Fatalf("linked: %+v", l.Linked)
	}
	link := l.Linked[0]
	// the list enables the Mods/ copy (the game loads it): the Mod is still used
	if !link.InPlan || len(l.UsedIn()) == 0 {
		t.Fatalf("usage lost: in plan %v, used %v", link.InPlan, l.UsedIn())
	}
	r := foreignRow(t, lib, "Pump")
	if r.Copies != 1 || !r.Single || len(r.Insts) != 1 || r.Used == "" {
		t.Fatalf("row: %d copies, single %v, used %q", r.Copies, r.Single, r.Used)
	}
	ws := l.Instances[0]
	ops := view.CopyOps(lib, l, ws, view.LibraryPage)
	if slices.Contains(ops, view.OpSeparate) {
		t.Fatalf("the Workshop copy offers separate from a link: %v", ops)
	}
	f := linkedField(view.LibFields(e.App, lib, ws, nil))
	if f == nil || f.Act != view.ActLinked || f.Target != link.Mod.ID || !strings.Contains(f.Value, "RimCrow") || !strings.Contains(f.Value, "_Link_2880970348") || !strings.Contains(f.Value, "used in") {
		t.Fatalf("linked field: %+v", f)
	}
	// the plan editor still finds the linked copy and may swap to the Workshop copy, but
	// offers no copy actions on the link
	eops := view.CopyOps(lib, l, link, view.EditorPage)
	for _, op := range []string{view.OpSeparate, view.OpPin, view.OpDelete, view.OpTextures} {
		if slices.Contains(eops, op) {
			t.Fatalf("editor ops of the link have %s: %v", op, eops)
		}
	}
	if !slices.Contains(eops, view.OpSwap) || lib.Instance(link.Ref) != link {
		t.Fatalf("editor ops of the link: %v", eops)
	}
	// CLI (and MCP list_library, its argv): one copy, the link listed apart
	out := e.MustRun("library", "--lineage", l.Ref)
	if !strings.Contains(out, "[1 copy") || !strings.Contains(out, "linked in: "+link.Ref+" by RimCrow") || !strings.Contains(out, "not a copy") {
		t.Fatalf("library --lineage:\n%s", out)
	}
	if out := e.MustRun("library"); !strings.Contains(out, "1 copy,") || strings.Contains(out, "2 copies") {
		t.Fatalf("library:\n%s", out)
	}
	// the game loads it: the save preview keeps it and removes no link; trash still refuses
	if out := e.MustRun("save", "--dry-run"); strings.Contains(out, "_Link_") || strings.Contains(out, "missing") {
		t.Fatalf("save --dry-run:\n%s", out)
	}
	if out, code := e.Run("trash", link.Ref, "--dry-run"); code == 0 || !strings.Contains(out, "RimCrow") {
		t.Fatalf("trash a link: exit %d\n%s", code, out)
	}
}

// A foreign link that is the Mod's only copy, enabled: it stays the Mod's row (the game
// loads it and the list uses it), with "show where it is" in place of delete and no copy
// actions; its details carry the linked field for itself.
func TestForeignLinkOnlyCopyEnabled(t *testing.T) {
	e := testkit.New(t)
	outside := filepath.Join(e.Root, "Elsewhere", "Solo")
	e.WriteMod(outside, testkit.Sample("tester.solo", "Solo"))
	e.Junction(filepath.Join(e.Game, "Mods", "Solo link"), outside)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.solo")
	e.AddEnv(false)
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	l := lineageNamed(t, lib, "Solo")
	if len(l.Instances) != 1 || len(l.Linked) != 0 {
		t.Fatalf("copies %d linked %d", len(l.Instances), len(l.Linked))
	}
	i := l.Instances[0]
	if i.Link == "" || i.LinkedBy != "" || !i.InPlan {
		t.Fatalf("instance: link %q by %q in plan %v", i.Link, i.LinkedBy, i.InPlan)
	}
	ops := view.CopyOps(lib, l, i, view.LibraryPage)
	if !slices.Contains(ops, view.OpLinked) || slices.Contains(ops, view.OpDelete) || slices.Contains(ops, view.OpPin) || slices.Contains(ops, view.OpSeparate) {
		t.Fatalf("ops: %v", ops)
	}
	f := linkedField(view.LibFields(e.App, lib, i, nil))
	if f == nil || f.Target != i.Mod.ID || !strings.Contains(f.Value, "Solo link") {
		t.Fatalf("linked field: %+v", f)
	}
	if out := e.MustRun("status"); strings.Contains(out, "missing") {
		t.Fatalf("status:\n%s", out)
	}
	if out := e.MustRun("save", "--dry-run"); strings.Contains(out, "Solo link") || strings.Contains(out, "missing") {
		t.Fatalf("save --dry-run:\n%s", out)
	}
	if r := foreignRow(t, lib, "Solo"); r.Copies != 1 || r.Used == "" {
		t.Fatalf("row: %d copies, used %q", r.Copies, r.Used)
	}
}

// RimArcho's own `_Archo_` link stays an alias of the SteamCMD copy it deploys: one copy,
// nothing linked, its copy actions unchanged.
func TestOwnLinkStillAlias(t *testing.T) {
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "8888"), testkit.Sample("tester.cmd", "CmdOnly"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(true)
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("enable", lineageNamed(t, lib, "CmdOnly").Instances[0].Ref)
	e.MustRun("save")
	e.MustRun("scan")
	if lib, err = e.App.Library(); err != nil {
		t.Fatal(err)
	}
	l := lineageNamed(t, lib, "CmdOnly")
	if len(l.Instances) != 1 || len(l.Linked) != 0 || l.Instances[0].Source != app.LibSteamCMD || l.Instances[0].Link != "" {
		t.Fatalf("copies %d linked %d", len(l.Instances), len(l.Linked))
	}
	if f := linkedField(view.LibFields(e.App, lib, l.Instances[0], nil)); f != nil {
		t.Fatalf("own link has a linked field: %+v", f)
	}
	ops := view.CopyOps(lib, l, l.Instances[0], view.LibraryPage)
	if !slices.Contains(ops, view.OpPin) || slices.Contains(ops, view.OpLinked) {
		t.Fatalf("ops: %v", ops)
	}
}

// A link RimArcho first saw with its target outside every source, whose folder the player
// later adds (RimCrow's mods folder as an extra folder): the next scan makes the folder
// that copy and the link its alias, so the Mod shows the extra copy, not a link.
func TestForeignLinkTargetAddedLater(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("2880970348", testkit.Sample("cixwow.pump", "Pump"))
	crow := filepath.Join(e.Root, "RimCrow", "mods")
	e.WriteMod(filepath.Join(crow, "2880970348"), testkit.Sample("cixwow.pump", "Pump"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_2880970348"), filepath.Join(crow, "2880970348"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "cixwow.pump")
	e.AddEnv(false)
	e.MustRun("scan")
	before, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	was := lineageNamed(t, before, "Pump").Linked[0].Mod.ID
	e.MustRun("folder", "add", "extra", crow)
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	// the same Instance, now at its folder: the list that enabled it still does
	if i := lib.Instance(was); i == nil || i.Source != app.LibExtra || !i.InPlan {
		t.Fatalf("instance %s after adding its folder: %+v", was, i)
	}
	l := lineageNamed(t, lib, "Pump")
	var srcs []string
	for _, i := range l.Instances {
		srcs = append(srcs, i.Source)
		if i.Link != "" {
			t.Fatalf("%s is still read as a link: %s", i.Ref, i.Link)
		}
	}
	if len(l.Linked) != 0 || !slices.Contains(srcs, app.LibExtra) {
		t.Fatalf("sources %v, linked %d", srcs, len(l.Linked))
	}
}
