package app_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

const monitorLink = "_Archo_monitor_ArchoMonitor-0.11.0"

// copyAsExplorer copies a folder the way Explorer does: a junction or symlink becomes a
// real folder holding its target's files.
func copyAsExplorer(t *testing.T, src, dst string) {
	t.Helper()
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, de := range ents {
		from, to := filepath.Join(src, de.Name()), filepath.Join(dst, de.Name())
		if source.IsLink(from) {
			real, err := source.RealPath(from)
			if err != nil {
				t.Fatal(err)
			}
			copyAsExplorer(t, real, to)
			continue
		}
		if de.IsDir() {
			copyAsExplorer(t, from, to)
			continue
		}
		in, err := os.Open(from)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(to)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// copiedGame: the Monitor installed and linked into the first game, then that game's
// folder copied in Explorer and added as env:second, which is current and scanned. The
// recycle bin removes for real; trashed lists what went there.
func copiedGame(t *testing.T) (e *testkit.Env, game2 string, trashed *[]string) {
	t.Helper()
	e = loadoutEnv(t)
	scanNow(t, e)
	e.App.MonitorBundle = monitorBundle(t, e, "0.11.0")
	e.MustRun("monitor", "install")
	e.MustRun("save")
	game2 = filepath.Join(e.Root, "Game2")
	copyAsExplorer(t, e.Game, game2)
	if source.IsLink(filepath.Join(game2, "Mods", monitorLink)) {
		t.Fatal("the copy kept the link")
	}
	ud2 := filepath.Join(e.Root, "UserData2")
	e.MkdirAll(filepath.Join(ud2, "Config"))
	e.MustRun("env", "add", "second", "--game", game2, "--user-data", ud2, "--workshop", e.Workshop)
	e.MustRun("env", "use", "second")
	e.MustRun("scan")
	trashed = &[]string{}
	e.App.TrashFunc = func(p string) error { *trashed = append(*trashed, p); return os.RemoveAll(p) }
	return e, game2, trashed
}

// planMonitor is the Monitor instance in the current plan.
func planMonitor(t *testing.T, e *testkit.Env) *store.Mod {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range st.List {
		if m := st.Ix.ByID[en.Instance]; app.IsMonitor(m) {
			return m
		}
	}
	t.Fatal("no Monitor in the plan")
	return nil
}

func wantBundledMonitor(t *testing.T, e *testkit.Env, when string) {
	t.Helper()
	m := planMonitor(t, e)
	if !strings.EqualFold(filepath.Clean(m.RealPath), filepath.Clean(mustReal(t, filepath.Join(monitorDir(e), "ArchoMonitor-0.11.0")))) {
		t.Fatalf("%s: the plan uses the Monitor at %s (%s), not the installed one", when, m.RealPath, m.Kind)
	}
}

func copyLink(t *testing.T, e *testkit.Env, path string) *app.GameLink {
	t.Helper()
	links, err := e.App.InspectGameLinks()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if strings.EqualFold(filepath.Base(l.Path), filepath.Base(path)) && strings.EqualFold(filepath.Dir(l.Path), filepath.Dir(mustReal(t, path))) {
			return l
		}
	}
	t.Fatalf("no entry for %s", path)
	return nil
}

// 09 §3, 19 §6: a game folder copied in Explorer holds copies of RimArcho's links. The plans
// every game shares keep the installed Monitor; Can it run offers to move the copy to the
// recycle bin, refuses while the game runs, and saving then makes the link again.
func TestCopiedGameLinkCopyRepair(t *testing.T) {
	t.Parallel()
	e, game2, trashed := copiedGame(t)
	copied := filepath.Join(game2, "Mods", monitorLink)
	wantBundledMonitor(t, e, "after scanning the copied game")

	l := copyLink(t, e, copied)
	if l.Code != app.CodeLinkOccupied || l.Action != "recycle copy" || l.Copy == "" || !strings.Contains(l.ShortReason(), "copy") {
		t.Fatalf("copy: %+v", l)
	}
	out, code := e.Run("save")
	if code == 0 || !strings.Contains(out, "is a copy of the folder RimArcho links there") || !strings.Contains(out, "archo links repair --dry-run") {
		t.Fatalf("save with the copy (%d):\n%s", code, out)
	}

	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var action view.Action
	for _, n := range view.BuildNeeds(e.App, ps.State, ps).Items {
		for _, a := range n.Actions {
			if a.Kind == view.ActRepairLinks {
				action = a
			}
		}
	}
	if action.Kind == "" || action.Key != "R" || action.Label != "move the copies to the recycle bin" || !strings.Contains(action.Warn, "to the recycle bin") {
		t.Fatalf("Can it run action: %+v", action)
	}

	// The game is running: nothing moves.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{} }
	if _, err := view.Do(e.App, action, 0); err == nil || !strings.Contains(err.Error(), "game is running") {
		t.Fatalf("repair while the game runs: %v", err)
	}
	if len(*trashed) > 0 {
		t.Fatalf("moved while the game runs: %v", *trashed)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	out = e.MustRun("links", "repair", "--dry-run")
	if !strings.Contains(out, "action: recycle copy") || len(*trashed) > 0 {
		t.Fatalf("preview:\n%s", out)
	}
	out = e.MustRun("links", "repair", "--yes")
	if !strings.Contains(out, "recycled: 1") || len(*trashed) != 1 || !strings.EqualFold(filepath.Base((*trashed)[0]), monitorLink) {
		t.Fatalf("repair %v:\n%s", *trashed, out)
	}
	wantBundledMonitor(t, e, "after the repair")
	if out := e.MustRun("problems"); strings.Contains(out, "mods.link-occupied") || strings.Contains(out, "mod.unloadable") {
		t.Fatalf("problems after the repair:\n%s", out)
	}
	e.MustRun("save")
	if !source.IsLink(copied) {
		t.Fatal("saving did not make the Monitor link again")
	}
	e.MustRun("env", "use", "main")
	wantBundledMonitor(t, e, "back in the first game")
}

// The player's state before this fix: scanning the copied game had switched the shared plan
// to the copy. The next scan puts the installed Monitor back, in either game.
func TestCopiedGamePlanSwitchedBack(t *testing.T) {
	t.Parallel()
	for _, from := range []string{"second", "main"} {
		t.Run(from, func(t *testing.T) {
			t.Parallel()
			planSwitchedBack(t, from)
		})
	}
}

func planSwitchedBack(t *testing.T, from string) {
	e, game2, _ := copiedGame(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	var copyID string
	for _, m := range ix.Mods {
		if app.IsMonitor(m) && strings.EqualFold(filepath.Base(m.RealPath), monitorLink) {
			copyID = m.ID
		}
	}
	if copyID == "" {
		t.Fatal("the copy was not read as a mod")
	}
	e.MustRun("disable", "#"+planMonitor(t, e).ID)
	e.MustRun("enable", "#"+copyID)
	if planMonitor(t, e).ID != copyID {
		t.Fatal("could not make the old state")
	}
	e.MustRun("env", "use", from)
	e.MustRun("scan")
	wantBundledMonitor(t, e, "after the next scan in env:"+from)
	e.MustRun("env", "use", "second")
	wantBundledMonitor(t, e, "in the copied game")
	if !strings.EqualFold(filepath.Base(copyLink(t, e, filepath.Join(game2, "Mods", monitorLink)).Path), monitorLink) {
		t.Fatal("the copy is no longer reported")
	}
}

// Only an exact copy goes: different sizes keep the old "rename it yourself" entry, and a
// copy whose files changed but kept their sizes is refused when the repair runs.
func TestCopiedGameChangedCopyKept(t *testing.T) {
	t.Parallel()
	e, game2, trashed := copiedGame(t)
	copied := filepath.Join(game2, "Mods", monitorLink)
	about := filepath.Join(copied, "About", "About.xml")
	b, err := os.ReadFile(about)
	if err != nil {
		t.Fatal(err)
	}

	e.WriteFile(filepath.Join(copied, "notes.txt"), "mine")
	if l := copyLink(t, e, copied); l.Action != "keep" || l.Copy != "" {
		t.Fatalf("a copy with an extra file: %+v", l)
	}
	e.Remove(filepath.Join(copied, "notes.txt"))

	changed := []byte(strings.Replace(string(b), "<name>", "<nAme>", 1))
	if len(changed) != len(b) || string(changed) == string(b) {
		t.Fatal("fixture: About.xml has no <name>")
	}
	e.WriteFile(about, string(changed))
	if l := copyLink(t, e, copied); l.Action != "recycle copy" {
		t.Fatalf("same sizes: %+v", l)
	}
	out, code := e.Run("links", "repair", "--yes")
	if code == 0 || !strings.Contains(out, "is not an exact copy") || len(*trashed) > 0 {
		t.Fatalf("changed copy (%d) %v:\n%s", code, *trashed, out)
	}
	if _, err := os.Stat(about); err != nil {
		t.Fatal("the changed copy is gone")
	}
}
