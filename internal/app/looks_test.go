package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// lookPNG is a valid 2×2 PNG; shade changes the content.
func lookPNG(t *testing.T, shade uint8) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < 4; i++ {
		img.SetNRGBA(i%2, i/2, color.NRGBA{shade, uint8(i), 7, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const (
	gunLook = "things/item/gun"
	hatLook = "things/item/hat"
)

// looksEnv: Core and the Mods A, B (Workshop) and C (Mods/), in that load order. All three
// ship the look things/item/gun (south, north; A also the mask of south), each with its
// own content; A and C ship things/item/hat.
func looksEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := setup(t, true)
	mod := func(pkg, name string, shade uint8, files ...string) testkit.Mod {
		m := testkit.Mod{PackageID: pkg, Name: name, Files: map[string]string{}}
		for i, f := range files {
			m.Files["Textures/"+f] = lookPNG(t, shade+uint8(i))
		}
		return m
	}
	e.AddWorkshop("1001", mod("looks.a", "A", 10, "Things/Item/Gun_south.png", "Things/Item/Gun_north.png", "Things/Item/Gun_southm.png", "Things/Item/Hat.png"))
	e.AddWorkshop("1002", mod("looks.b", "B", 60, "Things/Item/Gun_south.png", "Things/Item/Gun_north.png"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), mod("looks.c", "C", 120, "Things/Item/Gun_south.png", "Things/Item/Gun_north.png", "Things/Item/Hat.png"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b", "looks.c")
	scanNow(t, e)
	if got := joined(active(t, e)); got != "Core, A, B, C" {
		t.Fatalf("plan: %s", got)
	}
	return e
}

func looksOf(t *testing.T, e *testkit.Env) *app.LooksState {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ls, err := e.App.LooksState(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

// inGame names the Mod the game shows for each texture slot of a look: "south=A north=B".
func inGame(t *testing.T, e *testkit.Env, look string) string {
	t.Helper()
	ls := looksOf(t, e)
	l := ls.Index.ByID[look]
	if l == nil {
		t.Fatalf("no look %s", look)
	}
	wins := l.InGame(ls.ChoiceOf(look))
	var out []string
	for _, s := range l.SlotNames() {
		if !looks.IsMask(s) {
			out = append(out, s+"="+ls.Mods[wins[s].Pos].Name)
		}
	}
	return strings.Join(out, " ")
}

func applyLook(t *testing.T, e *testkit.Env, look, from, scope string, slots ...string) *app.LooksResult {
	t.Helper()
	r, err := e.App.LooksApply(context.Background(), look, from, slots, scope, false)
	if err != nil {
		t.Fatalf("apply %s from %s: %v", look, from, err)
	}
	return r
}

func looksModDir(e *testkit.Env) string {
	return filepath.Join(e.Root, "LocalAppData", "RimArcho", "looks", "Looks")
}

// choiceStates lists a look's choices as "scope/slot=state".
func choiceStates(ls *app.LooksState, look string) string {
	s := ls.Looks[look]
	if s == nil {
		return ""
	}
	var out []string
	for _, c := range s.Choices {
		scope, state := "all", c.State
		if c.Plan {
			scope = "plan"
		}
		if c.Overruled {
			state = "overruled"
		}
		out = append(out, scope+"/"+c.Slot+"="+state)
	}
	return strings.Join(out, " ")
}

// 21 §3.2, §7: the plan's choice wins over the one for all plans, a slot's over the whole look's.
func TestLooksResolutionOrder(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	if got := inGame(t, e, gunLook); got != "south=C north=C" {
		t.Fatalf("load order: %s", got)
	}
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksAll)
	if got := inGame(t, e, gunLook); got != "south=A north=A" {
		t.Fatalf("all plans, whole look: %s", got)
	}
	applyLook(t, e, gunLook, "pkg:looks.b", app.LooksPlan)
	if got := inGame(t, e, gunLook); got != "south=B north=B" {
		t.Fatalf("the plan's choice does not win: %s", got)
	}
	if got := choiceStates(looksOf(t, e), gunLook); got != "plan/all=applied all/all=overruled" {
		t.Fatalf("states: %s", got)
	}
	r := applyLook(t, e, gunLook, "pkg:looks.a", app.LooksAll, "north")
	if got := inGame(t, e, gunLook); got != "south=B north=B" {
		t.Fatalf("a slot for all plans beat the plan's whole look: %s", got)
	}
	if r.Mixed {
		t.Fatal("not mixed: B has every slot")
	}
	r = applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan, "south")
	if got := inGame(t, e, gunLook); got != "south=A north=B" {
		t.Fatalf("the slot's choice does not win: %s", got)
	}
	if !r.Mixed {
		t.Fatal("south from A and north from B is mixed: the apply warns")
	}
	// The mask follows its texture: A wins south and has its mask.
	ls := looksOf(t, e)
	wins := ls.Index.ByID[gunLook].InGame(ls.ChoiceOf(gunLook))
	if w := wins[looks.MaskOf("south")]; ls.Mods[w.Pos].Name != "A" {
		t.Fatalf("mask of south from %s", ls.Mods[w.Pos].Name)
	}
	// A new plan has only the choices for all plans.
	e.MustRun("plan", "new", "second", "--from", "current")
	e.MustRun("plan", "use", "second")
	if got := inGame(t, e, gunLook); got != "south=A north=A" {
		t.Fatalf("another plan: %s", got)
	}
}

// 21 §3.4: applied, not needed, asleep and broken, with counts; a choice is never deleted.
func TestLooksStates(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	applyLook(t, e, hatLook, "pkg:looks.c", app.LooksPlan) // C already wins by load order
	ls := looksOf(t, e)
	if a, b := choiceStates(ls, gunLook), choiceStates(ls, hatLook); a != "plan/all=applied" || b != "plan/all=not needed" {
		t.Fatalf("states: gun %q, hat %q", a, b)
	}
	if ls.Applied != 1 || ls.NotNeeded != 1 || ls.Asleep != 0 || ls.Broken != 0 {
		t.Fatalf("counts: %+v", ls)
	}
	// Nothing is written for the choice that is not needed.
	if _, err := os.Stat(filepath.Join(looksModDir(e), "Textures", "Things", "Item", "Hat.png")); err == nil {
		t.Fatal("a file was written for a choice the load order already gives")
	}

	// broken: A is in the plan but no longer has the look's textures.
	for _, f := range []string{"Gun_south.png", "Gun_north.png", "Gun_southm.png"} {
		e.Remove(filepath.Join(e.Workshop, "1001", "Textures", "Things", "Item", f))
	}
	ls = looksOf(t, e)
	if got := choiceStates(ls, gunLook); got != "plan/all=broken" || ls.Broken != 1 {
		t.Fatalf("broken: %q, %d", got, ls.Broken)
	}
	if got := inGame(t, e, gunLook); got != "south=C north=C" {
		t.Fatalf("a broken choice leaves the load-order result: %s", got)
	}

	// asleep: no Instance of the chosen Lineage is in the plan; the choice stays.
	e.MustRun("disable", "pkg:looks.a")
	ls = looksOf(t, e)
	if got := choiceStates(ls, gunLook); got != "plan/all=asleep" || ls.Asleep != 1 {
		t.Fatalf("asleep: %q, %d", got, ls.Asleep)
	}
	rows, err := e.App.Store.LookChoices()
	if err != nil || len(rows) != 2 {
		t.Fatalf("choices are never deleted automatically: %v %v", rows, err)
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(fa, fb)
}

// 21 §4: a rebuild writes hard links on one volume and records them; an unchanged rebuild
// writes nothing; a Mod update that replaced a linked file is picked up; without hard
// links the files are copies.
func TestLooksRebuildLinks(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	r := applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	dir := looksModDir(e)
	if !r.DirChosen || r.Dir != filepath.Dir(dir) || r.Build == nil || r.BuildErr != "" {
		t.Fatalf("first apply: %+v (%s)", r, r.BuildErr)
	}
	if got := e.MustRun("config", "get", "looks.dir"); !strings.Contains(got, filepath.Dir(dir)) {
		t.Fatalf("looks.dir not stored: %s", got)
	}
	srcDir := filepath.Join(e.Workshop, "1001", "Textures", "Things", "Item")
	want := []string{"Gun_north.png", "Gun_south.png", "Gun_southm.png"}
	if r.Build.Links != 3 || r.Build.Copies != 0 || len(r.Build.Files) != 3 {
		t.Fatalf("build: %+v", r.Build)
	}
	for _, f := range want {
		if !sameFile(t, filepath.Join(srcDir, f), filepath.Join(dir, "Textures", "Things", "Item", f)) {
			t.Fatalf("%s is not a hard link to A's file", f)
		}
	}
	rec, err := e.App.Store.LookFiles()
	if err != nil || len(rec) != 3 {
		t.Fatalf("look_file: %v %v", rec, err)
	}
	for i, f := range rec {
		if f.Rel != "Textures/Things/Item/"+want[i] || !f.Linked || !sameFile(t, f.Source, filepath.Join(srcDir, want[i])) || f.Size == 0 || f.MTime == 0 {
			t.Fatalf("look_file[%d] = %+v", i, f)
		}
	}
	about, err := os.ReadFile(filepath.Join(dir, "About", "About.xml"))
	if err != nil || !strings.Contains(string(about), "<packageId>rimarcho.looks</packageId>") || !strings.Contains(string(about), "<name>_Archo_Looks</name>") {
		t.Fatalf("About.xml: %s %v", about, err)
	}

	// Unchanged: nothing is written (the folder is not swapped, no operation-log row).
	marker := filepath.Join(dir, "marker")
	e.WriteFile(marker, "x")
	builds := func() int {
		rows, _ := e.App.Operations(oplog.Query{Op: oplog.LooksBuild})
		return len(rows)
	}
	n := builds()
	r2, err := e.App.LooksRebuild(context.Background(), false)
	if err != nil || r2.Build == nil || !r2.Build.Unchanged || r2.Build.Links != 3 {
		t.Fatalf("unchanged rebuild: %+v %v", r2.Build, err)
	}
	if _, err := os.Stat(marker); err != nil || builds() != n {
		t.Fatal("an unchanged rebuild wrote")
	}

	// A Mod update replaces A's file (a new file, new size and time): the rebuild relinks.
	south := filepath.Join(srcDir, "Gun_south.png")
	e.Remove(south)
	e.Advance(time.Minute)
	e.WriteFile(south, lookPNG(t, 200)+"updated")
	if sameFile(t, south, filepath.Join(dir, "Textures", "Things", "Item", "Gun_south.png")) {
		t.Fatal("the old link cannot be the new file")
	}
	r3, err := e.App.LooksRebuild(context.Background(), false)
	if err != nil || r3.Build.Unchanged || r3.BuildErr != "" {
		t.Fatalf("rebuild after an update: %+v %v", r3, err)
	}
	if !sameFile(t, south, filepath.Join(dir, "Textures", "Things", "Item", "Gun_south.png")) {
		t.Fatal("the rebuild did not relink the updated file")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the folder was edited in place, not rebuilt and swapped")
	}
	for _, leftover := range []string{dir + ".archo-tmp", dir + ".archo-old"} {
		if _, err := os.Lstat(leftover); err == nil {
			t.Fatalf("%s left behind", leftover)
		}
	}

	// No hard links (another volume, a file system without them): copies, recorded as such.
	e.App.HardLink = func(string, string) error { return errors.New("no hard links here") }
	applyLook(t, e, gunLook, "pkg:looks.b", app.LooksPlan)
	rec, _ = e.App.Store.LookFiles()
	if len(rec) != 2 {
		t.Fatalf("B has two files: %+v", rec)
	}
	for _, f := range rec {
		dst := filepath.Join(dir, filepath.FromSlash(f.Rel))
		if f.Linked || sameFile(t, f.Source, dst) {
			t.Fatalf("%s should be a copy: %+v", f.Rel, f)
		}
		a, _ := os.ReadFile(f.Source)
		b, _ := os.ReadFile(dst)
		if !bytes.Equal(a, b) || len(a) == 0 {
			t.Fatalf("%s: the copy differs", f.Rel)
		}
	}
}

// 21 §3.3: undo removes one look's choices in one scope only.
func TestLooksUndoOneLookOneScope(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	applyLook(t, e, gunLook, "pkg:looks.b", app.LooksAll)
	applyLook(t, e, hatLook, "pkg:looks.a", app.LooksPlan)
	before := active(t, e)
	r, err := e.App.LooksUndo(context.Background(), gunLook, app.LooksPlan, false)
	if err != nil || r.Removed != 1 {
		t.Fatalf("undo: %+v %v", r, err)
	}
	rows, _ := e.App.Store.LookChoices()
	var got []string
	for _, c := range rows {
		scope := "plan"
		if c.Scope == store.LookScopeAll {
			scope = "all"
		}
		got = append(got, scope+":"+c.Look)
	}
	if joined(got) != "plan:"+hatLook+", all:"+gunLook && joined(got) != "all:"+gunLook+", plan:"+hatLook {
		t.Fatalf("after undo: %v", got)
	}
	if got := inGame(t, e, gunLook); got != "south=B north=B" {
		t.Fatalf("the choice for all plans stands: %s", got)
	}
	if joined(active(t, e)) != joined(before) {
		t.Fatalf("undo changed the plan: %v", active(t, e))
	}
	// The generated Mod now holds B's gun and A's hat.
	rec, _ := e.App.Store.LookFiles()
	var rels []string
	for _, f := range rec {
		rels = append(rels, filepath.Base(filepath.Dir(f.Source))+"/"+filepath.Base(f.Rel))
	}
	if len(rec) != 3 || !strings.Contains(rec[0].Source, "1002") || !strings.Contains(rec[2].Source, "1001") {
		t.Fatalf("files after undo: %v", rels)
	}
	if _, err := e.App.LooksUndo(context.Background(), gunLook, app.LooksPlan, false); err == nil {
		t.Fatal("nothing left to undo in this scope")
	}
}

// 21 §4 "In the plan": the first apply adds _Archo_Looks as the last entry in one undoable
// edit; a sort keeps it last and does not report it; its Mods/ link is _Archo_Looks.
func TestLooksEntryLastAndLinkName(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	r := applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	if !r.Added {
		t.Fatal("the first apply adds the entry")
	}
	if got := joined(active(t, e)); got != "Core, A, B, C, "+app.LooksName {
		t.Fatalf("after apply: %s", got)
	}
	if r := applyLook(t, e, hatLook, "pkg:looks.a", app.LooksPlan); r.Added {
		t.Fatal("a second apply does not add it again")
	}
	if _, op, err := e.App.UndoRedo(false); err != nil || op != app.LooksAddOp {
		t.Fatalf("undo: %q %v", op, err)
	}
	if got := joined(active(t, e)); got != "Core, A, B, C" {
		t.Fatalf("one undo step removes it: %s", got)
	}
	if _, _, err := e.App.UndoRedo(true); err != nil {
		t.Fatal(err)
	}

	// Moved up by hand, a sort puts it back last; the sort does not treat it as a problem.
	e.MustRun("move", "pkg:"+app.LooksPackage, "--to", "2")
	if got := active(t, e); got[1] != app.LooksName {
		t.Fatalf("move: %v", got)
	}
	sr, err := e.App.Sort(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := active(t, e); got[len(got)-1] != app.LooksName || len(got) != 5 {
		t.Fatalf("sort did not keep it last: %v", got)
	}
	if len(sr.MissingDependencies) != 0 || len(sr.MissingMods) != 0 || len(sr.Result.Dropped) != 0 || len(sr.Result.Cycles) != 0 {
		t.Fatalf("the sort reports a problem: %+v %+v %+v", sr.MissingDependencies, sr.MissingMods, sr.Result.Dropped)
	}
	for _, nd := range sr.Input.Nodes {
		if nd.PackageID == app.LooksPackage {
			t.Fatal("the sort orders _Archo_Looks")
		}
	}
	if sr2, err := e.App.Sort(true); err != nil || len(sr2.Changes) != 0 {
		t.Fatalf("a sorted list with the entry last is not stable: %+v %v", sr2.Changes, err)
	}

	sv, err := e.App.Save(false)
	if err != nil || sv.LooksError != "" {
		t.Fatalf("save: %+v %v", sv, err)
	}
	link := filepath.Join(e.Game, "Mods", "_Archo_Looks")
	if !source.IsLink(link) {
		t.Fatalf("no link %s", link)
	}
	if real, err := source.RealPath(link); err != nil || !sameFile(t, real, looksModDir(e)) {
		t.Fatalf("the link points to %s (%v)", real, err)
	}
	if cfg := e.ReadModsConfig(); !strings.Contains(cfg, "<li>rimarcho.looks</li>") || strings.Index(cfg, "rimarcho.looks") < strings.Index(cfg, "looks.c") {
		t.Fatalf("ModsConfig.xml: %s", cfg)
	}
}

// 21 §4 "Failure": a rebuild that fails leaves the write to the game without the entry; the
// result says why, the choices and the plan's entry stay. Here the rebuild is refused
// because a game runs (09 §4).
func TestLooksFailedRebuildLeavesEntryOut(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	if _, err := e.App.Save(false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(e.Game, "Mods", "_Archo_Looks")
	if !source.IsLink(link) {
		t.Fatal("no link after the first save")
	}

	// An unchanged rebuild is no failure, even while a game runs.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 7, Started: e.Now()} }
	e.MustRun("disable", "pkg:looks.b")
	if sv, err := e.App.Save(false); err != nil || sv.LooksError != "" || !strings.Contains(e.ReadModsConfig(), "rimarcho.looks") {
		t.Fatalf("save with an unchanged generated Mod while a game runs: %+v %v", sv, err)
	}

	// A new choice while the game runs is kept, but the generated Mod is not rebuilt.
	r := applyLook(t, e, hatLook, "pkg:looks.a", app.LooksPlan)
	if !strings.Contains(r.BuildErr, "while a game runs") {
		t.Fatalf("apply while a game runs: %q", r.BuildErr)
	}
	rec, _ := e.App.Store.LookFiles()
	if len(rec) != 3 {
		t.Fatalf("the generated Mod was rebuilt while a game runs: %+v", rec)
	}
	dry, err := e.App.Save(true)
	if err != nil || dry.LooksError == "" {
		t.Fatalf("the dry run does not say the entry is left out: %+v %v", dry, err)
	}
	sv, err := e.App.Save(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sv.LooksError, "while a game runs") {
		t.Fatalf("LooksError = %q", sv.LooksError)
	}
	if strings.Contains(e.ReadModsConfig(), "rimarcho.looks") {
		t.Fatalf("the entry was written: %s", e.ReadModsConfig())
	}
	if !source.IsLink(link) {
		t.Fatal("the link is left as it is: a link change is refused while that game runs")
	}
	if got := active(t, e); got[len(got)-1] != app.LooksName {
		t.Fatalf("the plan lost its entry: %v", got)
	}
	if rows, _ := e.App.Store.LookChoices(); len(rows) != 2 {
		t.Fatalf("the choices did not stay: %+v", rows)
	}
	if out := e.MustRun("save", "--dry-run"); !strings.Contains(out, "looks: _Archo_Looks left out of this write") {
		t.Fatalf("archo save does not say why:\n%s", out)
	}
	rows, _ := e.App.Operations(oplog.Query{Op: oplog.LooksBuild, Failed: true})
	if len(rows) == 0 || rows[0].Result != oplog.Refused {
		t.Fatalf("no refused looks.build row: %+v", rows)
	}

	// The game has closed: the next write rebuilds and the entry is back.
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	sv, err = e.App.Save(false)
	if err != nil || sv.LooksError != "" {
		t.Fatalf("save after the game closed: %+v %v", sv, err)
	}
	if !strings.Contains(e.ReadModsConfig(), "rimarcho.looks") || !source.IsLink(link) {
		t.Fatal("the entry did not come back")
	}
	if rec, _ := e.App.Store.LookFiles(); len(rec) != 4 {
		t.Fatalf("files after the rebuild: %+v", rec)
	}
}

// modsDigest fingerprints Mod folders: every file's path, size, modification time and content.
func modsDigest(t *testing.T, dirs ...string) string {
	t.Helper()
	h := sha256.New()
	for _, root := range dirs {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			if d.IsDir() {
				fmt.Fprintf(h, "d %s\n", rel)
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "f %s %d %d %x %v\n", rel, fi.Size(), fi.ModTime().UnixNano(), sha256.Sum256(b), fi.Mode().Perm())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// 21 §7: nothing is written into a Mod folder: apply, rebuild (links and copies), the write
// to the game, undo and uninstalling the generated Mod leave every Mod's tree as it was.
func TestLooksWritesNothingIntoMods(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	dirs := []string{filepath.Join(e.Workshop, "1001"), filepath.Join(e.Workshop, "1002"), filepath.Join(e.Game, "Mods", "C"), filepath.Join(e.Game, "Data")}
	before := modsDigest(t, dirs...)
	check := func(step string) {
		t.Helper()
		if got := modsDigest(t, dirs...); got != before {
			t.Fatalf("%s changed a Mod folder", step)
		}
	}
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	check("apply")
	applyLook(t, e, hatLook, "pkg:looks.a", app.LooksAll, "single")
	check("apply a slot")
	if _, err := e.App.Save(false); err != nil {
		t.Fatal(err)
	}
	check("save")
	e.App.HardLink = func(string, string) error { return errors.New("no hard links") }
	applyLook(t, e, gunLook, "pkg:looks.b", app.LooksPlan)
	check("rebuild with copies")
	e.App.HardLink = nil
	if _, err := e.App.LooksUndo(context.Background(), gunLook, app.LooksPlan, false); err != nil {
		t.Fatal(err)
	}
	check("undo")
	if _, err := e.App.LooksRebuild(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	check("rebuild")

	// Uninstall deletes the generated Mod (hard links only), never the Mods' files.
	p, err := e.App.Uninstall(app.UninstallOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range p.Steps {
		if s.Key == app.UnLooks {
			found = len(s.Items) == 1 && s.Items[0].Done
		}
	}
	if !found {
		t.Fatalf("no looks step: %+v", p.Steps)
	}
	if _, err := os.Lstat(filepath.Dir(looksModDir(e))); err == nil {
		t.Fatal("the looks directory stayed")
	}
	check("uninstall")
}

// 21 §4 "Where": the looks directory is on the game's volume; when its folder there cannot
// be made, it is the local folder's.
func TestLooksDirOnGameVolume(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	local := filepath.Join(e.Root, "LocalAppData", "RimArcho", "looks")
	if got := e.App.LooksDir(); got != "" {
		t.Fatalf("before the first apply: %q", got)
	}
	if got := e.App.LooksDirDefault(e.Game); got != local {
		t.Fatalf("same volume: %s", got)
	}
	// The game is on another volume than the local folder.
	vol := filepath.Join(e.Root, "Steam")
	e.App.SameVolume = func(a, b string) bool {
		in := func(p string) bool {
			return strings.Contains(strings.ToLower(p), `steam`+string(filepath.Separator)+`steamapps`)
		}
		return in(a) == in(b)
	}
	blocked := filepath.Join(vol, "blocked")
	e.WriteFile(blocked, "a file where the volume root would be")
	e.App.VolumeRoot = func(string) string { return blocked }
	if got := e.App.LooksDirDefault(e.Game); got != filepath.Join(blocked, "RimArcho", "looks") {
		t.Fatalf("other volume: %s", got)
	}
	r, err := e.App.LooksApply(context.Background(), gunLook, "pkg:looks.a", nil, app.LooksPlan, true)
	if err != nil || !r.DirChosen || r.Dir != filepath.Join(blocked, "RimArcho", "looks") || e.App.LooksDir() != "" {
		t.Fatalf("dry run: %+v %v", r, err)
	}
	if rows, _ := e.App.Store.LookChoices(); len(rows) != 0 || contains(active(t, e), app.LooksName) {
		t.Fatal("the dry run wrote")
	}
	// It cannot be created: the local folder is used.
	r = applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	if r.Dir != local || e.App.LooksDir() != local {
		t.Fatalf("fallback: %s", r.Dir)
	}
	if r.Build.Copies != 3 || r.Build.Links != 0 {
		t.Fatalf("files across volumes are copies: %+v", r.Build)
	}
}

// 10 §2: texture optimisation never takes the generated Mod; 03 §2: its folder is RimArcho's own.
func TestLooksModIsLeftAlone(t *testing.T) {
	t.Parallel()
	e := looksEnv(t)
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	ts, err := e.App.TextureTargets(nil, app.TexPlan)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("targets: %d", len(ts))
	}
	for _, tt := range ts {
		if tt.Mod.Name == app.LooksName {
			t.Fatal("texture optimisation takes _Archo_Looks")
		}
	}
	all, err := e.App.TextureTargets(nil, app.TexAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range all {
		if tt.Mod.Name == app.LooksName {
			t.Fatal("texture optimisation --all takes _Archo_Looks")
		}
	}
	if out, code := e.Run("folder", "remove", app.LooksLabel); code == 0 {
		t.Fatalf("the looks folder was removed:\n%s", out)
	}
	// The generated Mod is no provider of any look.
	ls := looksOf(t, e)
	for pos, m := range ls.Mods {
		if app.IsLooks(m) {
			t.Fatalf("_Archo_Looks is a provider at %d", pos)
		}
	}
	if l := ls.Index.ByID[gunLook]; len(l.Providers) != 3 {
		t.Fatalf("providers: %d", len(l.Providers))
	}
}
