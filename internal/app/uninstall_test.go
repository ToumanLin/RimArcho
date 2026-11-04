package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// unFix is everything RimArcho leaves on a machine (01 §4): two games ("main" from Steam,
// "old" downloaded with SteamCMD and keeping its own saves), RimArcho links in main's Mods/
// (a SteamCMD mod, a lock library copy, Archo Monitor), a stray _Archo_ folder that is not a
// link, ModsConfig.xml written with its backup, a pack, a save backup, RimArcho's SteamCMD
// and Steam runtime folders, and a program folder.
type unFix struct {
	*testkit.Env
	local, lock, old, oldSaves, program, keep string
	cmdMod                                    string // the SteamCMD mod the link points at
	bak                                       string // ModsConfig.xml.rimarcho.bak as saved
	trashed                                   []string
	after                                     *app.AfterExit
}

func uninstallFixture(t *testing.T) *unFix {
	e := loadoutEnv(t)
	f := &unFix{Env: e}
	f.cmdMod = filepath.Join(e.SteamCMD, "8888")
	e.WriteMod(f.cmdMod, testkit.Sample("tester.cmd", "CmdOnly"))
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "CmdOnly", ""))
	e.MustRun("pin", refOf(t, e, "A", ""))
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	e.MustRun("save")
	e.MustRun("save") // a second write: the backup now holds a list RimArcho wrote
	f.bak = readFile(t, e.ModsConfig+".rimarcho.bak")
	// a settings change RimArcho made, with its backups (06 §2.9)
	e.WriteFile(filepath.Join(e.UserData, "Config", "Mod_1001_AMod.xml.rimarcho.bak"), "<SettingsBlock/>")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml.rimarcho.bak"), "<settings/>")

	f.local = filepath.Join(e.Root, "LocalAppData", "RimArcho")
	f.lock = filepath.Join(f.local, "locked")
	// A folder named like a link that is not one: never touched.
	e.WriteFile(filepath.Join(e.Game, "Mods", "_Archo_mine", "About", "About.xml"), "<ModMetaData><packageId>me.mine</packageId></ModMetaData>")

	// The downloaded game, with its own saves in the data folder.
	f.old = e.OtherGame("old", "1.5.4409 rev1118")
	e.WriteFile(filepath.Join(f.old, "steamapps", "appmanifest_294100.acf"), `"AppState" { "appid" "294100" }`)
	e.MustRun("env", "add", "old", "--game", f.old, "--saves", "own", "--launch", "direct")
	f.oldSaves = filepath.Join(e.Home, "userdata", "old")
	e.WriteFile(filepath.Join(f.oldSaves, "Saves", "colony.rws"), "a save")
	e.WriteFile(filepath.Join(e.Home, "backups", "saves", "20261020-120000", "colony.rws"), "a backup")

	e.WriteFile(filepath.Join(e.Home, "packs", "P1", "pack.json"), `{"format":1,"name":"P1","entries":[]}`)
	e.WriteFile(filepath.Join(e.Home, "packs", "P1", "userdata", "Saves", "pack.rws"), "a pack save")
	e.WriteFile(filepath.Join(f.local, "steamcmd", "steamcmd.exe"), "MZ")
	e.WriteFile(filepath.Join(f.local, "steam", "steam_appid.txt"), "294100")

	f.program = filepath.Join(e.Root, "Program")
	for _, n := range []string{"archo.exe", "RimArcho.exe", "README.md", "CHANGELOG.md", "LICENSE", "fonts/legacy.ttf", "monitor/ArchoMonitor/About/About.xml"} {
		e.WriteFile(filepath.Join(f.program, n), "x")
	}
	e.App.UninstallExe = filepath.Join(f.program, "archo.exe")
	f.keep = filepath.Join(e.Root, "Documents")
	e.App.UninstallKeepBase = f.keep
	e.App.ArchoProbe = func() []int { return nil }
	e.App.TrashFunc = func(p string) error {
		f.trashed = append(f.trashed, p)
		return os.RemoveAll(p)
	}
	e.App.AfterExitFunc = func(x app.AfterExit) error { f.after = &x; return nil }
	e.MustRun("env", "use", "main")
	return f
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func (f *unFix) mods() []string {
	ents, _ := os.ReadDir(filepath.Join(f.Game, "Mods"))
	var out []string
	for _, d := range ents {
		out = append(out, d.Name())
	}
	sort.Strings(out)
	return out
}

// lockCopy is the one folder in the lock library.
func (f *unFix) lockCopy(t *testing.T) string {
	t.Helper()
	ents, _ := os.ReadDir(f.lock)
	if len(ents) != 1 {
		t.Fatalf("lock library: %d entries", len(ents))
	}
	return filepath.Join(f.lock, ents[0].Name())
}

func (f *unFix) run(t *testing.T, o app.UninstallOptions) *app.UninstallPlan {
	t.Helper()
	p, err := f.App.Uninstall(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Steps {
		for _, it := range s.Items {
			if it.Err != "" {
				t.Fatalf("%s %s: %s", s.Key, it.Path, it.Err)
			}
		}
	}
	return p
}

// The defaults: links gone (targets untouched), the stray folder kept, the list cleaned of
// what disappears, the lock library copy and Monitor gone, the pack and the saves moved to
// Documents, the downloaded game kept, SteamCMD gone, and the data folders and the program
// left to the script; the script removes only RimArcho's files from the program folder.
func TestUninstallDefaults(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	copyDir := f.lockCopy(t)
	plan, err := f.App.PlanUninstall(app.UninstallOptions{})
	if err != nil || plan.Blocked() != nil {
		t.Fatal(err, plan.Blocked())
	}
	if exists(filepath.Join(f.Root, "Documents")) || !exists(copyDir) {
		t.Fatal("planning changed something")
	}

	p := f.run(t, app.UninstallOptions{})
	if got := strings.Join(f.mods(), ","); got != "B,_Archo_mine" {
		t.Fatalf("Mods/ after: %s", got)
	}
	if !source.HasAbout(f.cmdMod) {
		t.Fatal("the link's target outside was touched")
	}
	if it := p.Step(app.UnLinks).Items; !hasItem(it, "_Archo_mine", app.DoLeave) {
		t.Fatalf("the stray folder is not reported: %+v", it)
	}
	mc := f.ReadModsConfig()
	for _, gone := range []string{"tester.cmd", "toumanlin.archomonitor"} {
		if strings.Contains(mc, "<li>"+gone+"</li>") {
			t.Fatalf("%s left in ModsConfig.xml:\n%s", gone, mc)
		}
	}
	for _, kept := range []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"} {
		if !strings.Contains(mc, "<li>"+kept+"</li>") {
			t.Fatalf("%s taken out of ModsConfig.xml (it is still installed):\n%s", kept, mc)
		}
	}
	if exists(f.ModsConfig + ".rimarcho.bak") {
		t.Fatal("the backup is RimArcho's: it goes")
	}
	if exists(filepath.Join(f.UserData, "Config", "Mod_1001_AMod.xml.rimarcho.bak")) || exists(filepath.Join(f.UserData, "HugsLib", "ModSettings.xml.rimarcho.bak")) {
		t.Fatal("the settings backups are RimArcho's: they go")
	}
	if exists(copyDir) || len(f.trashed) != 1 || f.trashed[0] != copyDir {
		t.Fatalf("the lock library copy goes to the recycle bin: %v", f.trashed)
	}
	if exists(filepath.Join(f.local, "monitor", "ArchoMonitor-0.9.0")) {
		t.Fatal("Monitor's folder is left")
	}
	if readFile(t, filepath.Join(f.keep, "RimArcho packs", "P1", "userdata", "Saves", "pack.rws")) != "a pack save" {
		t.Fatal("the pack was not moved out of the data folder")
	}
	if readFile(t, filepath.Join(f.keep, "RimArcho saves", "old", "Saves", "colony.rws")) != "a save" ||
		readFile(t, filepath.Join(f.keep, "RimArcho saves", "backups", "20261020-120000", "colony.rws")) != "a backup" {
		t.Fatal("the saves were not moved out")
	}
	if !exists(filepath.Join(f.old, "RimWorldWin64.exe")) {
		t.Fatal("the downloaded game is kept by default")
	}
	if exists(filepath.Join(f.local, "steamcmd")) {
		t.Fatal("SteamCMD is left")
	}
	if !f.App.Uninstalled {
		t.Fatal("not marked uninstalled")
	}
	if out, code := f.Run("status"); code == 0 || !strings.Contains(out, "uninstalled") {
		t.Fatalf("commands after uninstall: %d\n%s", code, out)
	}

	// The last step, as the script does it once RimArcho has closed.
	if f.after == nil {
		t.Fatal("the clean-up was not started")
	}
	f.WriteFile(filepath.Join(f.program, "my notes.txt"), "the player's")
	f.App.Close()
	if left := app.RunAfterExit(*f.after); len(left) > 0 {
		t.Fatalf("left: %v", left)
	}
	for _, gone := range []string{f.Home, f.local, filepath.Join(f.program, "archo.exe"), filepath.Join(f.program, "fonts"), filepath.Join(f.program, "README.md")} {
		if exists(gone) {
			t.Fatalf("%s is left", gone)
		}
	}
	if readFile(t, filepath.Join(f.program, "my notes.txt")) != "the player's" {
		t.Fatal("a file that is not RimArcho's was deleted")
	}
	for _, kept := range []string{f.Game, f.UserData, f.Workshop, f.cmdMod, f.old} {
		if !exists(kept) {
			t.Fatalf("%s was deleted", kept)
		}
	}
	// Without the player's file the program folder goes too.
	os.Remove(filepath.Join(f.program, "my notes.txt"))
	app.RunAfterExit(*f.after)
	if exists(f.program) {
		t.Fatal("the empty program folder is left")
	}
}

func hasItem(items []*app.UninstallItem, base, do string) bool {
	for _, it := range items {
		if filepath.Base(it.Path) == base && it.Do == do {
			return true
		}
	}
	return false
}

// Every other answer: the copies kept as ordinary mods in a game's Mods/ (without
// RimArcho's note), packs, saves and the downloaded game to the recycle bin, and the backup
// of ModsConfig.xml put back.
func TestUninstallChoices(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	copyDir := f.lockCopy(t)
	pack := filepath.Join(f.Home, "packs", "P1")
	p := f.run(t, app.UninstallOptions{KeepLocked: true, RemovePacks: true, RemoveSaves: true, RemoveGames: true, RestoreBackup: true})
	if p.LockedTo != "main" {
		t.Fatalf("kept copies go to the current game: %q", p.LockedTo)
	}
	moved := filepath.Join(f.Game, "Mods", filepath.Base(copyDir))
	if !source.HasAbout(moved) || exists(copyDir) {
		t.Fatalf("the copy is not in Mods/: %v", f.mods())
	}
	if exists(filepath.Join(moved, ".rimarcho")) {
		t.Fatal("RimArcho's note is left in the kept copy")
	}
	if f.ReadModsConfig() != f.bak || exists(f.ModsConfig+".rimarcho.bak") {
		t.Fatalf("the backup was not put back:\n%s", f.ReadModsConfig())
	}
	want := map[string]bool{pack: true, f.oldSaves: true, filepath.Join(f.Home, "backups", "saves"): true, f.old: true}
	for _, tr := range f.trashed {
		delete(want, tr)
	}
	if len(want) > 0 {
		t.Fatalf("not in the recycle bin: %v (trashed %v)", want, f.trashed)
	}
	if exists(f.keep) {
		t.Fatal("nothing is kept in Documents")
	}

	// Kept copies into another game: the one picked.
	g := uninstallFixture(t)
	p = g.run(t, app.UninstallOptions{KeepLocked: true, LockedTo: "old"})
	if p.LockedTo != "old" || len(lockFolders(t, filepath.Join(g.old, "Mods"))) != 1 {
		t.Fatalf("copies into old: %q", p.LockedTo)
	}
}

func lockFolders(t *testing.T, dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, d := range ents {
		out = append(out, d.Name())
	}
	return out
}

// 08 §7: with no recycle bin the copy is refused and stays; the data folders and the
// program are kept so the uninstall can run again.
func TestUninstallNoRecycleBin(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	copyDir := f.lockCopy(t)
	f.App.TrashFunc = func(p string) error { return errors.New("no recycle bin on this volume") }
	p, err := f.App.Uninstall(app.UninstallOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	it := p.Step(app.UnLocked).Items[0]
	if it.Err == "" || it.Done || !source.HasAbout(copyDir) {
		t.Fatalf("the copy: %+v", it)
	}
	if f.after != nil {
		t.Fatal("the clean-up started after a failed step")
	}
	for _, s := range []string{app.UnData, app.UnProgram} {
		for _, it := range p.Step(s).Items {
			if it.Do != app.DoLeave {
				t.Fatalf("%s: %s, want kept", s, it.Do)
			}
		}
	}
	if !exists(filepath.Join(f.Home, "archo.db")) {
		t.Fatal("the database must stay to run again")
	}
	// The real recycle bin refuses on a missing path instead of deleting anything.
	if err := source.Trash(filepath.Join(f.Root, "no such folder")); err == nil {
		t.Fatal("trashing a missing folder must fail")
	}
}

// Refused while a game or another RimArcho runs, and when kept copies would go into a
// game that is removed; nothing changes.
func TestUninstallRefused(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	before := strings.Join(f.mods(), ",")
	f.App.GameProbe = func(root string) *app.GameRun {
		if root == f.Game {
			return &app.GameRun{PID: 42, Exe: filepath.Join(f.Game, "RimWorldWin64.exe")}
		}
		return nil
	}
	if _, err := f.App.Uninstall(app.UninstallOptions{}, nil); !errors.Is(err, app.ErrUninstallBlocked) || !strings.Contains(err.Error(), "game is running") {
		t.Fatalf("with the game running: %v", err)
	}
	f.App.GameProbe = func(string) *app.GameRun { return nil }
	f.App.ArchoProbe = func() []int { return []int{7} }
	if _, err := f.App.Uninstall(app.UninstallOptions{}, nil); !errors.Is(err, app.ErrUninstallBlocked) {
		t.Fatalf("with another RimArcho: %v", err)
	}
	f.App.ArchoProbe = func() []int { return nil }
	if _, err := f.App.Uninstall(app.UninstallOptions{KeepLocked: true, LockedTo: "old", RemoveGames: true}, nil); err == nil || !strings.Contains(err.Error(), "removed too") {
		t.Fatalf("copies into a removed game: %v", err)
	}
	if _, err := f.App.Uninstall(app.UninstallOptions{SavesTo: filepath.Join(f.Home, "x")}, nil); err == nil {
		t.Fatal("saves moved into the data folder")
	}
	if strings.Join(f.mods(), ",") != before || f.App.Uninstalled || len(f.trashed) > 0 {
		t.Fatal("a refused uninstall changed something")
	}
}

func TestUninstallExecutionSkipsDirectorySizes(t *testing.T) {
	t.Parallel()
	f := uninstallFixture(t)
	o := app.UninstallOptions{KeepLocked: true}
	preview, err := f.App.PlanUninstall(o)
	if err != nil {
		t.Fatal(err)
	}
	measured := 0
	for _, step := range preview.Steps {
		for _, it := range step.Items {
			if it.Bytes > 0 {
				measured++
			}
			it.Bytes = 0
		}
	}
	if measured == 0 {
		t.Fatal("fixture has no directory sizes")
	}
	checked, err := f.App.CheckUninstall(o)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(checked)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("checks changed beyond sizes:\n%s\n%s", want, got)
	}
	// Exercise the actual execution entry point, with a blocker so nothing is removed.
	f.App.ArchoProbe = func() []int { return []int{77} }
	ran := false
	blocked, err := f.App.Uninstall(o, func(app.UninstallProgress) { ran = true })
	if !errors.Is(err, app.ErrUninstallBlocked) {
		t.Fatalf("uninstall: %v", err)
	}
	for _, step := range blocked.Steps {
		for _, it := range step.Items {
			if it.Bytes != 0 {
				t.Fatalf("execution measured %s: %d bytes", it.Path, it.Bytes)
			}
		}
	}
	if ran || f.App.Uninstalled || len(f.trashed) > 0 {
		t.Fatal("blocked execution changed files")
	}
}
