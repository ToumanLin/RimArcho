package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func directory(t *testing.T, a *app.App, id string) app.ManagedDirectory {
	t.Helper()
	ds, err := a.ManagedDirectories()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("missing managed directory %s: %+v", id, ds)
	return app.ManagedDirectory{}
}

func TestManagedDirectoriesReadOnlyAndStartup(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	for _, id := range []string{"lock", "packs", "monitor", "steamcmd-tool"} {
		d := directory(t, e.App, id)
		if d.Status != "unused" || d.Note == "" {
			t.Fatalf("%s: %+v", id, d)
		}
		if _, err := os.Stat(d.Path); !os.IsNotExist(err) {
			t.Fatalf("check created %s", d.Path)
		}
	}
	e.AddEnv(false)
	ss := sourcesOf(t, e)
	wsID := "source:" + ss["workshop"].ID
	if err := os.Rename(e.Workshop, e.Workshop+"-moved"); err != nil {
		t.Fatal(err)
	}
	key := e.App.Store.ChangeKey()
	if d := directory(t, e.App, wsID); d.Status != "missing" {
		t.Fatalf("missing: %+v", d)
	}
	if e.App.Store.ChangeKey() != key {
		t.Fatal("ManagedDirectories changed the database")
	}
	e.Open() // startup does not hide missing directories or use the pre-override LockBase
	if directory(t, e.App, wsID).Status != "missing" {
		t.Fatal("startup hid the missing source")
	}
	if directory(t, e.App, "lock").Status != "unused" {
		t.Fatal("default lock falsely missing")
	}
	if _, err := os.Stat(e.Workshop); !os.IsNotExist(err) {
		t.Fatal("startup recreated missing source")
	}
	if err := os.WriteFile(e.Workshop, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if directory(t, e.App, wsID).Status != "changed" {
		t.Fatal("a file at the directory path was not changed")
	}
}

func TestSetDirectoryRetainsIdentityAndPending(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.directory", "Directory"))
	e.AddEnv(false)
	scanNow(t, e)
	id := refID(t, e, "Directory", source.Workshop)
	e.MustRun("tag", "add", "#"+id[:6], "keeper")
	e.MustRun("enable", "#"+id[:6])
	src := sourcesOf(t, e)["workshop"]
	to := filepath.Join(e.Root, "moved-workshop")
	if err := os.Rename(e.Workshop, to); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e) // missing roots retain the instance before the user identifies its new home
	ix, _ := e.App.Index()
	if ix.ByID[id].Status != identity.Live {
		t.Fatal("missing source lost its identity")
	}
	src.Status, src.PendingRoot = source.StatusRebindPending, filepath.Join(e.Root, "wrong")
	if err := e.App.Store.UpdateSource(src); err != nil {
		t.Fatal(err)
	}
	if directory(t, e.App, "source:"+src.ID).Status != "rebind-pending" {
		t.Fatal("pending not reported")
	}
	if err := e.App.SetDirectory("source:"+src.ID, to); err != nil {
		t.Fatal(err)
	}
	ix, _ = e.App.Index()
	m := ix.ByID[id]
	realTo, _ := source.RealPath(to)
	if m == nil || !slices.Contains(m.Tags, "keeper") || !source.Within(m.RealPath, realTo) {
		t.Fatalf("records after set: %+v", m)
	}
	if s := ix.Source(src.ID); s.PendingRoot != "" || s.Status != source.StatusOK || env.PathKey(s.Root) != env.PathKey(to) {
		t.Fatalf("source: %+v", s)
	}
	scanNow(t, e)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.List.Index(id) < 0 || st.Ix.ByID[id].Status != identity.Live {
		t.Fatal("relocation lost list entry")
	}
}

func TestSetGameDirectoryRetainsSources(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("tester.localdirectory", "Local Directory"))
	e.AddEnv(false)
	scanNow(t, e)
	id := refID(t, e, "Local Directory", source.Local)
	e.MustRun("tag", "add", "#"+id[:6], "keeper")
	e.MustRun("enable", "#"+id[:6])
	game, _ := e.App.Env()
	ss := sourcesOf(t, e)
	to := filepath.Join(e.Root, "GameMoved")
	if err := os.Rename(e.Game, to); err != nil {
		t.Fatal(err)
	}
	if err := e.App.SetDirectory("game:"+game.ID, to); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.List.Index(id) < 0 || st.Ix.ByID[id] == nil || !slices.Contains(st.Ix.ByID[id].Tags, "keeper") {
		t.Fatal("game relocation lost ID/tag/list")
	}
	if st.Ix.Source(ss["local"].ID).Root != filepath.Join(to, "Mods") {
		t.Fatal("local source changed identity")
	}
	if st.Ix.Source(ss["workshop"].ID).Root != e.Workshop {
		t.Fatal("unrelated Workshop moved")
	}
}

func TestLockDirectoryRelocationAndLinks(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.lockdirectory", "Lock Directory"))
	e.AddEnv(false)
	scanNow(t, e)
	e.MustRun("enable", "ws:5100")
	e.MustRun("pin", "ws:5100")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, m := range st.Ix.Mods {
		if m.SourceLabel == app.LockLabel {
			id = m.ID
		}
	}
	if id == "" {
		t.Fatal("no locked copy")
	}
	e.MustRun("tag", "add", "#"+id[:6], "locked-keeper")
	e.MustRun("save")
	m := st.Ix.ByID[id]
	link := filepath.Join(e.Game, "Mods", app.DeployName(m))
	if !source.IsLink(link) {
		t.Fatal("locked copy was not deployed")
	}
	old := e.App.LockDir()
	to := filepath.Join(e.Root, "MovedLocks")
	if err := os.Rename(old, to); err != nil {
		t.Fatal(err)
	}
	if directory(t, e.App, "lock").Status != "missing" {
		t.Fatal("used lock not missing")
	}
	// An attempted new pin cannot recreate the vanished root.
	if _, err := e.App.Pin("ws:5100", "another", false, false); err == nil {
		t.Fatal("pin to a missing lock root succeeded")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("pin recreated missing lock root")
	}
	if err := e.App.SetDirectory("lock", to); err != nil {
		t.Fatal(err)
	}
	if env.PathKey(e.App.LockDir()) != env.PathKey(to) {
		t.Fatal("lock config not updated")
	}
	actual, err := source.RealPath(link)
	realTo, _ := source.RealPath(to)
	if err != nil || !source.Within(actual, realTo) {
		t.Fatalf("stale deployment link: %s %v", actual, err)
	}
	scanNow(t, e)
	st, _ = e.App.Loadout()
	if st.List.Index(id) < 0 || st.Ix.ByID[id] == nil || st.Ix.ByID[id].Status != identity.Live || !slices.Contains(st.Ix.ByID[id].Tags, "locked-keeper") {
		t.Fatal("lock relocation lost ID/tag/list")
	}
}

// Another drive: a checked copy, the original to the recycle bin (outside the allow-list,
// which refuses a whole source folder), RimArcho's links in the game's Mods/ made again at
// the new place. A link of the player's that points outside stops the copy, untouched.
func TestMoveDirectoryAcrossDrives(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("tester.localacross", "Local Across"))
	e.AddWorkshop("5100", testkit.Sample("tester.acrossdrives", "Across Drives"))
	e.AddEnv(false)
	scanNow(t, e)
	e.MustRun("enable", "ws:5100")
	e.MustRun("pin", "ws:5100")
	e.MustRun("save")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, m := range st.Ix.Mods {
		if m.SourceLabel == app.LockLabel {
			id = m.ID
		}
	}
	if id == "" {
		t.Fatal("no locked copy")
	}
	deployed := app.DeployName(st.Ix.ByID[id])
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.App.SameVolume = func(string, string) bool { return false }
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, p); return os.RemoveAll(p) }

	// The lock library: copied, the old folder in the recycle bin, the link follows.
	old := e.App.LockDir()
	locks := filepath.Join(e.Root, "LocksOtherDrive")
	copied, err := e.App.MoveDirectory("lock", locks)
	if err != nil || !copied {
		t.Fatalf("lock move: copied=%v err=%v", copied, err)
	}
	if len(trashed) != 1 || env.PathKey(trashed[0]) != env.PathKey(old) {
		t.Fatalf("recycle bin got %v, want %s", trashed, old)
	}
	realLocks, _ := source.RealPath(locks)
	if actual, err := source.RealPath(filepath.Join(e.Game, "Mods", deployed)); err != nil || !source.Within(actual, realLocks) {
		t.Fatalf("deployment link after lock move: %s %v", actual, err)
	}

	// The game folder, with a link of the player's pointing outside: refused, nothing moved,
	// RimArcho's link back in place.
	game, _ := e.App.Env()
	gameTo := filepath.Join(e.Root, "GameOtherDrive")
	mine := filepath.Join(e.Game, "Mods", "MyLink")
	if err := app.CreateLink(mine, e.Workshop); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.MoveDirectory("game:"+game.ID, gameTo); err == nil || !strings.Contains(err.Error(), "MyLink") {
		t.Fatalf("outside link not refused: %v", err)
	}
	if _, err := os.Stat(gameTo); !os.IsNotExist(err) {
		t.Fatal("refused move left a copy")
	}
	if !source.IsLink(filepath.Join(e.Game, "Mods", deployed)) {
		t.Fatal("refused move lost RimArcho's link")
	}
	if err := source.RemoveLink(mine); err != nil {
		t.Fatal(err)
	}

	// Without it: the game moves, its own mods with it, RimArcho's link made again there.
	trashed = nil
	if copied, err := e.App.MoveDirectory("game:"+game.ID, gameTo); err != nil || !copied {
		t.Fatalf("game move: copied=%v err=%v", copied, err)
	}
	if len(trashed) != 1 || env.PathKey(trashed[0]) != env.PathKey(e.Game) {
		t.Fatalf("recycle bin got %v, want %s", trashed, e.Game)
	}
	if actual, err := source.RealPath(filepath.Join(gameTo, "Mods", deployed)); err != nil || !source.Within(actual, realLocks) {
		t.Fatalf("deployment link after game move: %s %v", actual, err)
	}
	scanNow(t, e)
	st, _ = e.App.Loadout()
	local := refID(t, e, "Local Across", source.Local)
	if st.List.Index(id) < 0 || st.Ix.ByID[id] == nil || st.Ix.ByID[id].Status != identity.Live || st.Ix.ByID[local].Status != identity.Live {
		t.Fatal("moving across drives lost mods")
	}
}

func TestMoveDirectoryAndErrors(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	src := sourcesOf(t, e)["workshop"]
	id := "source:" + src.ID
	to := filepath.Join(e.Root, "MovedWorkshop")
	for _, p := range []string{"", filepath.Join(e.Root, "absent"), e.ModsConfig, e.Home, e.Game} {
		if err := e.App.SetDirectory(id, p); err == nil {
			t.Errorf("accepted invalid target %q", p)
		}
	}
	if err := e.App.SetDirectory("unknown", e.Workshop); err == nil {
		t.Fatal("unknown ID accepted")
	}
	if _, err := e.App.MoveDirectory(id, filepath.Join(e.Workshop, "inside")); err == nil {
		t.Fatal("overlapping move accepted")
	}
	if _, err := e.App.MoveDirectory(id, e.Game); err == nil {
		t.Fatal("occupied target accepted")
	}
	if _, err := e.App.MoveDirectory(id, filepath.Join(e.Root, "absent-parent", "target")); err == nil {
		t.Fatal("missing parent accepted")
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 123} }
	if err := e.App.SetDirectory(id, e.Workshop); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("set while running: %v", err)
	}
	if _, err := e.App.MoveDirectory(id, to); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running game: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	if _, err := e.App.MoveDirectory(id, to); err != nil {
		t.Fatal(err)
	}
	if directory(t, e.App, id).Path != to {
		t.Fatal("move did not update source")
	}
	if _, err := os.Stat(e.Workshop); !os.IsNotExist(err) {
		t.Fatal("old directory remains")
	}
	if _, err := e.App.MoveDirectory(id, to); err == nil {
		t.Fatal("same path move accepted")
	}
}

func TestMissingMonitorIsNotRecreated(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	scanNow(t, e)
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	if _, err := e.App.EnsureMonitor(false); err != nil {
		t.Fatal(err)
	}
	old := e.App.MonitorDir()
	to := filepath.Join(e.Root, "MovedMonitor")
	if err := os.Rename(old, to); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.EnsureMonitor(false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Deferred == "" || directory(t, e.App, "monitor").Status != "missing" {
		t.Fatal("missing monitor not deferred")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("monitor was recreated")
	}
	if err := e.App.SetDirectory("monitor", to); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.EnsureMonitor(false); err != nil {
		t.Fatal(err)
	}
	if e.App.MonitorDir() != to {
		t.Fatal("monitor config not updated")
	}
}

func TestManagedPackAndSteamCMDDirectories(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	scanNow(t, e)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	if _, err := e.App.BuildPack("Managed", false); err != nil {
		t.Fatal(err)
	}
	old := e.App.PacksDir()
	to := filepath.Join(e.Root, "MovedPacks")
	if err := os.Rename(old, to); err != nil {
		t.Fatal(err)
	}
	if directory(t, e.App, "packs").Status != "missing" {
		t.Fatal("used pack root not missing")
	}
	if _, err := e.App.BuildPack("Other", false); err == nil {
		t.Fatal("build recreated missing packs")
	}
	if err := e.App.SetDirectory("packs", to); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.ReadPack("Managed"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(e.Root, "PacksAgain")
	if _, err := e.App.MoveDirectory("packs", moved); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.ReadPack("Managed"); err != nil {
		t.Fatal(err)
	}

	tool := filepath.Join(e.Root, "Tool")
	e.WriteFile(filepath.Join(tool, deps.SteamCMDExecutable()), "")
	if err := e.App.SetDirectory("steamcmd-tool", tool); err != nil {
		t.Fatal(err)
	}
	if e.App.SteamCMDDir() != tool || e.App.SteamCMDPath() != filepath.Join(tool, deps.SteamCMDExecutable()) {
		t.Fatal("SteamCMD config did not follow set")
	}
	tool2 := filepath.Join(e.Root, "ToolMoved")
	if _, err := e.App.MoveDirectory("steamcmd-tool", tool2); err != nil {
		t.Fatal(err)
	}
	if e.App.SteamCMDDir() != tool2 || e.App.SteamCMDPath() != filepath.Join(tool2, deps.SteamCMDExecutable()) {
		t.Fatal("SteamCMD config did not follow move")
	}
}

// A directory move whose records cannot be written puts the directory back where the
// records still point, so the deployment link returns to it; across drives the original is
// never recycled (03 §4.2).
func TestMoveDirectoryRecordFailureMovesBack(t *testing.T) {
	t.Parallel()
	for _, cross := range []bool{false, true} {
		t.Run(map[bool]string{false: "rename", true: "cross-volume"}[cross], func(t *testing.T) {
			e := testkit.New(t)
			e.AddWorkshop("5100", testkit.Sample("tester.moveback", "Move Back"))
			e.AddEnv(false)
			scanNow(t, e)
			e.MustRun("enable", "ws:5100")
			e.MustRun("pin", "ws:5100")
			e.MustRun("save")
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			var id string
			for _, m := range st.Ix.Mods {
				if m.SourceLabel == app.LockLabel {
					id = m.ID
				}
			}
			link := filepath.Join(e.Game, "Mods", app.DeployName(st.Ix.ByID[id]))
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			e.App.SameVolume = func(string, string) bool { return !cross }
			e.App.TrashFunc = func(p string) error { t.Fatal("recycled", p); return nil }
			if _, err := e.App.Store.DB.Exec(`CREATE TRIGGER fail_move BEFORE UPDATE OF root ON source BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
				t.Fatal(err)
			}
			old := e.App.LockDir()
			to := filepath.Join(e.Root, "MovedLocks")
			if _, err := e.App.MoveDirectory("lock", to); err == nil || !strings.Contains(err.Error(), "stays at") {
				t.Fatal(err)
			}
			if _, err := os.Stat(to); !os.IsNotExist(err) {
				t.Fatal("directory left at the new place", err)
			}
			if env.PathKey(e.App.LockDir()) != env.PathKey(old) || directory(t, e.App, "lock").Status != "ok" {
				t.Fatal("settings changed", e.App.LockDir(), directory(t, e.App, "lock").Status)
			}
			realOld, _ := source.RealPath(old)
			if actual, err := source.RealPath(link); err != nil || !source.Within(actual, realOld) {
				t.Fatalf("deployment link not back: %s %v", actual, err)
			}
			rows, _ := e.App.Store.ManagedLinks()
			for _, r := range rows {
				if !store.LinkFinished(r.Phase) {
					t.Fatalf("ledger left %s in %q", r.Name, r.Phase)
				}
			}
		})
	}
}

// Changing an environment's game folder writes the environment and its game folders
// together: a failed write changes neither, and a later error (removing an unknown folder)
// leaves both on the new game.
func TestEnvSetGameMoveIsOneWrite(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	scanNow(t, e)
	game, _ := e.App.Env()
	moved := filepath.Join(e.Root, "MovedGame")
	e.CopyTree(e.Game, moved)
	rootsOf := func() (string, []string) {
		cur, err := e.App.Store.EnvByID(game.ID)
		if err != nil {
			t.Fatal(err)
		}
		srcs, _ := e.App.Store.Sources(game.ID)
		var roots []string
		for _, s := range srcs {
			if s.EnvID == game.ID && (s.Kind == source.Official || s.Kind == source.Local) && s.Status != source.StatusRetired {
				roots = append(roots, filepath.Dir(s.Root))
			}
		}
		return cur.GameRoot, roots
	}
	if _, err := e.App.Store.DB.Exec(`CREATE TRIGGER fail_env BEFORE UPDATE ON environment BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.App.EnvSet(game.Name, app.EnvSpec{GameRoot: moved}); err == nil {
		t.Fatal("want the injected error")
	}
	if root, srcs := rootsOf(); env.PathKey(root) != env.PathKey(e.Game) || len(srcs) != 2 || env.PathKey(srcs[0]) != env.PathKey(e.Game) || env.PathKey(srcs[1]) != env.PathKey(e.Game) {
		t.Fatalf("split after a failed write: game %s, sources in %v", root, srcs)
	}
	e.App.Store.DB.Exec(`DROP TRIGGER fail_env`)
	if _, _, err := e.App.EnvSet(game.Name, app.EnvSpec{GameRoot: moved, RemoveSources: []string{"no-such-folder"}}); err == nil {
		t.Fatal("want the unknown folder refused")
	}
	if root, srcs := rootsOf(); env.PathKey(root) != env.PathKey(moved) || len(srcs) != 2 || env.PathKey(srcs[0]) != env.PathKey(moved) || env.PathKey(srcs[1]) != env.PathKey(moved) {
		t.Fatalf("split after a later error: game %s, sources in %v", root, srcs)
	}
}

// Removing a folder whose re-binding was pending drops the pending root, so adding the
// folder back later does not bring a stale re-binding with it.
func TestFolderRemoveDropsPendingRebind(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	extra := filepath.Join(e.Root, "Extra")
	e.WriteMod(filepath.Join(extra, "A"), testkit.Sample("tester.extra", "Extra"))
	e.MustRun("folder", "add", "extra", extra)
	shared, err := e.App.Store.SharedSources()
	if err != nil {
		t.Fatal(err)
	}
	var s *source.Source
	for _, x := range shared {
		if env.PathKey(x.Root) == env.PathKey(extra) {
			s = x
		}
	}
	if s == nil {
		t.Fatal("no extra source")
	}
	s.Status, s.PendingRoot = source.StatusRebindPending, filepath.Join(e.Root, "Elsewhere")
	if err := e.App.Store.UpdateSource(s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.FolderRemove(s.Label); err != nil {
		t.Fatal(err)
	}
	e.MustRun("folder", "add", "extra", extra)
	all, _ := e.App.Store.AllSources()
	for _, x := range all {
		if x.ID == s.ID && (x.Status == source.StatusRetired || x.PendingRoot != "") {
			t.Fatalf("revived with %q, pending %q", x.Status, x.PendingRoot)
		}
	}
}
