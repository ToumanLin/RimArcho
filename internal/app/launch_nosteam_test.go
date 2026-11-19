package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// workshopLink is the _Archo_ link name a launch without Steam gives the Workshop copy of
// pkg (09 §3 "Workshop Mods without Steam").
func workshopLink(t *testing.T, a *app.App, pkg string) string {
	t.Helper()
	ix, err := a.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.PackageID == pkg && m.Kind == source.Workshop {
			return app.DeployName(m)
		}
	}
	t.Fatalf("no Workshop copy of %s", pkg)
	return ""
}

func lastLaunch(t *testing.T, s *store.Store) string {
	t.Helper()
	var method string
	if err := s.DB.QueryRow(`SELECT method FROM launch ORDER BY id DESC LIMIT 1`).Scan(&method); err != nil {
		t.Fatal(err)
	}
	return method
}

// 09 §2–§3: a direct launch without the Steam client writes steam_appid.txt (so the game
// does not restart through Steam) and links the list's Workshop Mods into Mods/; with
// Steam running, the next launch removes those links again and the game loads them from
// the Workshop. The player's own steam_appid.txt is never replaced.
func TestDirectLaunchWithoutSteam(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	steam := false
	e.App.SteamRunning = func() bool { return steam }
	appid := filepath.Join(e.Game, app.AppIDFile)
	harmony, a := workshopLink(t, e.App, "brrainz.harmony"), workshopLink(t, e.App, "tester.a")
	mods := filepath.Join(e.Game, "Mods")

	out := e.MustRun("launch", "--dry-run")
	for _, want := range []string{"+ " + harmony, "+ " + a, "RimWorldWin64.exe", "writes: " + appid} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry run lacks %q:\n%s", want, out)
		}
	}
	if exists(appid) || exists(filepath.Join(mods, a)) {
		t.Fatal("the dry run wrote")
	}

	out = e.MustRun("launch")
	if b, err := os.ReadFile(appid); err != nil || string(b) != "294100" {
		t.Fatalf("steam_appid.txt: %q %v", b, err)
	}
	for _, l := range []string{harmony, a} {
		if !source.IsLink(filepath.Join(mods, l)) {
			t.Fatalf("%s not linked:\n%s", l, out)
		}
	}
	if !strings.Contains(out, "2 Workshop Mod(s) linked") {
		t.Fatalf("launch output:\n%s", out)
	}
	mc := e.ReadModsConfig()
	if !strings.Contains(mc, "<li>tester.a</li>") || strings.Contains(mc, "_steam") {
		t.Fatalf("ModsConfig.xml:\n%s", mc)
	}
	if got := lastLaunch(t, e.App.Store); got != "direct" {
		t.Fatalf("method %s", got)
	}
	// The scan sees the links as aliases, not as Mods/ copies: the list stays the Workshop copies.
	scanNow(t, e)
	if out := e.MustRun("problems"); strings.Contains(out, "pkg.duplicate") {
		t.Fatalf("the links count as duplicates:\n%s", out)
	}

	// Steam runs now: a direct start uses it, so the links go and nothing is linked twice.
	steam = true
	e.Advance(time.Hour)
	out = e.MustRun("launch")
	for _, l := range []string{harmony, a} {
		if _, err := os.Lstat(filepath.Join(mods, l)); !os.IsNotExist(err) {
			t.Fatalf("%s stayed with Steam running:\n%s", l, out)
		}
	}
	if !strings.Contains(e.Launched[len(e.Launched)-1], "RimWorldWin64.exe") {
		t.Fatalf("launched %v", e.Launched)
	}

	// Another tool's steam_appid.txt stays as it is, and is not RimArcho's to remove.
	if err := os.Remove(appid); err != nil {
		t.Fatal(err)
	}
	e.App.Store.SetMeta("steam-appid:"+env.PathKey(e.Game), "")
	e.WriteFile(appid, "294100\n")
	steam = false
	e.Advance(time.Hour)
	if out := e.MustRun("launch"); strings.Contains(out, "writes:") {
		t.Fatalf("would write over the existing file:\n%s", out)
	}
	if b, _ := os.ReadFile(appid); string(b) != "294100\n" {
		t.Fatalf("steam_appid.txt changed: %q", b)
	}
}

// 09 §2 "Without Steam": a Game Config that launches through Steam starts the game
// directly when the Steam client is not running, with the Workshop links; once Steam
// runs it goes through Steam again and the links go.
func TestSteamLaunchFallsBackWithoutSteam(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	e.MustRun("env", "set", "main", "--launch", "steam")
	steam := false
	e.App.SteamRunning = func() bool { return steam }
	a := workshopLink(t, e.App, "tester.a")
	mods := filepath.Join(e.Game, "Mods")

	out := e.MustRun("launch")
	if !strings.Contains(out, "launched RimWorld directly: Steam is not running") {
		t.Fatalf("launch output:\n%s", out)
	}
	if got := e.Launched[0]; !strings.HasPrefix(got, filepath.Join(e.Game, "RimWorldWin64.exe")) || strings.Contains(got, "-applaunch") {
		t.Fatalf("launched %q", got)
	}
	if !source.IsLink(filepath.Join(mods, a)) || !exists(filepath.Join(e.Game, app.AppIDFile)) {
		t.Fatal("no Workshop link or steam_appid.txt")
	}
	if got := lastLaunch(t, e.App.Store); got != "direct" {
		t.Fatalf("method %s", got)
	}

	steam = true
	e.Advance(time.Hour)
	e.MustRun("launch")
	if got := e.Launched[1]; !strings.Contains(got, "steam.exe -applaunch 294100") {
		t.Fatalf("launched %q", got)
	}
	if exists(filepath.Join(mods, a)) {
		t.Fatal("the Workshop link stayed for a Steam launch")
	}
	if got := lastLaunch(t, e.App.Store); got != "steam" {
		t.Fatalf("method %s", got)
	}
}

// 09 §3 "Settings follow the Mod": a Workshop Mod played through its link without Steam
// and through its Workshop folder with Steam keeps one set of settings: the newer file of
// the two names replaces the older one, and an equal or older one changes nothing.
func TestWorkshopSettingsFollowBothNames(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	steam := false
	e.App.SteamRunning = func() bool { return steam }
	cfg := filepath.Join(e.UserData, "Config")
	ws := filepath.Join(cfg, "Mod_1001_AMod.xml")
	linked := filepath.Join(cfg, app.SettingsFile(workshopLink(t, e.App, "tester.a"), "AMod"))
	e.WriteFile(ws, "<SettingsBlock>workshop</SettingsBlock>")
	old := e.Now().Add(-48 * time.Hour)
	os.Chtimes(ws, old, old)

	// Without Steam: the link name has no file yet, so it gets the Workshop one.
	if out := e.MustRun("launch"); !strings.Contains(out, "copied 1 settings file") {
		t.Fatalf("launch:\n%s", out)
	}
	if got := readFile(t, linked); got != "<SettingsBlock>workshop</SettingsBlock>" {
		t.Fatalf("carried %q", got)
	}

	// Played without Steam, the settings change; with Steam the Workshop name gets them.
	e.WriteFile(linked, "<SettingsBlock>changed</SettingsBlock>")
	later := e.Now().Add(-time.Hour)
	os.Chtimes(linked, later, later)
	steam = true
	e.Advance(time.Hour)
	if out := e.MustRun("launch"); !strings.Contains(out, "copied 1 settings file") {
		t.Fatalf("launch with Steam:\n%s", out)
	}
	if got := readFile(t, ws); got != "<SettingsBlock>changed</SettingsBlock>" {
		t.Fatalf("Workshop file %q", got)
	}

	// Nothing newer on either side: nothing is copied either way.
	e.Advance(time.Hour)
	if out := e.MustRun("launch"); strings.Contains(out, "settings file") {
		t.Fatalf("copied again:\n%s", out)
	}
	steam = false
	e.Advance(time.Hour)
	if out := e.MustRun("launch"); strings.Contains(out, "settings file") {
		t.Fatalf("copied again without Steam:\n%s", out)
	}
}

// 02 §1: uninstall removes the steam_appid.txt a direct launch wrote, and only that one.
func TestUninstallRemovesOwnAppID(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.App.SteamRunning = func() bool { return true }
	appid := filepath.Join(e.Game, app.AppIDFile)
	e.MustRun("launch")
	p, err := e.App.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range p.Step(app.UnLinks).Items {
		found = found || it.Path == appid && it.Do == app.DoAppID
	}
	if !found {
		t.Fatalf("steam_appid.txt not in the plan: %+v", p.Step(app.UnLinks).Items)
	}
	// The player put their own text there since: not RimArcho's any more.
	e.WriteFile(appid, "480")
	p, _ = e.App.PlanUninstall(app.UninstallOptions{})
	if s := p.Step(app.UnLinks); s != nil {
		for _, it := range s.Items {
			if it.Path == appid {
				t.Fatalf("a changed steam_appid.txt is planned: %+v", it)
			}
		}
	}
}

// 09 §3: a save while the game started without Steam still runs keeps its Workshop links;
// the first save after it exits removes them.
func TestSaveWhileRunningKeepsWorkshopLinks(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.App.SteamRunning = func() bool { return false }
	link := filepath.Join(e.Game, "Mods", workshopLink(t, e.App, "tester.a"))
	e.MustRun("launch")
	running := true
	e.App.GameProbe = func(string) *app.GameRun {
		if running {
			return &app.GameRun{PID: 42, Exe: filepath.Join(e.Game, "RimWorldWin64.exe"), Started: e.Now()}
		}
		return nil
	}
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.MustRun("save")
	if !source.IsLink(link) {
		t.Fatal("a save while the game runs removed its Workshop link")
	}
	running = false
	e.MustRun("save")
	if exists(link) {
		t.Fatal("the Workshop link stayed after the game exited")
	}
}
