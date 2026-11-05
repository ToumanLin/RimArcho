package cmds_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// savesFixture is the shape of internal/app's savesFixture: Core, Alpha and Beta enabled,
// Gamma installed but not enabled, a launch an hour ago; saves with the same Mods, one after
// the launch with another order plus Gamma and a Workshop Mod that is not installed, its
// autosave, a RimSaves save and one cut off by a crash.
func savesFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	now := e.Now()
	e.RecordLaunch(now.Add(-time.Hour))
	same := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, ModTime: now.Add(-2 * time.Hour)}
	e.WriteSave("Tundra.rws", same)
	later := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "Tester.Beta", "tester.alpha", "tester.gamma", "tester.gone"},
		Names: []string{"Core", "Beta", "Alpha", "Gamma", "Gone Mod"}, SteamIDs: []string{"", "3000000003", "3000000002", "", "99"},
		ModTime: now.Add(-30 * time.Minute)}
	e.WriteSave("Tundra 2.rws", later)
	later.ModTime = now.Add(-20 * time.Minute)
	e.WriteSave("Autosave-1.rws", later)
	same.ModTime = now.Add(-3 * time.Hour)
	e.WriteSave("Ashfall#§#Landing.rws", same)
	e.WriteSave("Broken.rws", testkit.SaveSpec{Mods: same.Mods, CutOff: true, ModTime: now.Add(-10 * time.Minute)})
	return e
}

// fakeSteam answers the Steam worker as a signed-in client that subscribes and starts the
// downloads (internal/app's fakeSteamWorker).
func fakeSteam(e *testkit.Env) {
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake")
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		var out []string
		for _, s := range args[1:] {
			id, _ := strconv.ParseUint(s, 10, 64)
			out = append(out, steamworks.ItemLine(id, "ok", steamworks.ItemSubscribed|steamworks.ItemDownloadPending))
		}
		out = append(out, steamworks.DoneLine(args[0], len(args)-1))
		return strings.Join(out, "\n"), nil
	}
}

// packSave is the reference of the fixture's one save in a pack's folder.
func packSave(e *testkit.Env) string {
	l, err := e.App.ListSaves()
	if err != nil {
		e.T.Fatal(err)
	}
	for _, s := range l.Saves() {
		if s.Pack != "" {
			return s.Ref
		}
	}
	e.T.Fatal("no pack save")
	return ""
}

var saveRefRe = regexp.MustCompile(`sv:[0-9a-f]{6}`)

// saveRefs names each save reference after its save, so the golden text does not depend on
// the temporary folder (a reference hashes the real path, 05 §2). It learns new references
// as files are renamed and copied.
type saveRefs struct {
	e     *testkit.Env
	names map[string]string
}

func (r *saveRefs) learn() {
	l, err := r.e.App.ListSaves()
	if err != nil {
		r.e.T.Fatal(err)
	}
	for _, s := range l.Saves() {
		where := ""
		if s.Pack != "" {
			where = "pack:"
		}
		r.names[s.Ref] = "sv:<" + where + s.File + ">"
	}
}

func (r *saveRefs) scrub(s string) string {
	return saveRefRe.ReplaceAllStringFunc(s, func(ref string) string {
		if n, ok := r.names[ref]; ok {
			return n
		}
		return "sv:<unknown>"
	})
}

// runSavesGolden is runGolden with save references scrubbed in the command lines too.
func runSavesGolden(t *testing.T, name string, e *testkit.Env, steps []step) {
	t.Helper()
	refs := &saveRefs{e: e, names: map[string]string{}}
	var sb strings.Builder
	for _, s := range steps {
		refs.learn()
		args := s(e)
		if args == nil {
			continue
		}
		out, code := e.Run(args...)
		refs.learn()
		fmt.Fprintf(&sb, "$ archo %s\n", refs.scrub(e.Scrub(strings.Join(args, " "))))
		sb.WriteString(refs.scrub(e.Scrub(out)))
		if code != 0 {
			fmt.Fprintf(&sb, "[exit %d]\n", code)
		}
		sb.WriteString("\n")
		if n := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1; n > 40 {
			t.Errorf("archo %s: %d lines (05 §3.1: at most 40)", strings.Join(args, " "), n)
		}
	}
	got := sb.String()
	path := filepath.Join(goldenDir(), name+".txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/cmds -run %s -update)", err, t.Name())
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s differs from golden (go test ./internal/cmds -run %s -update):\n%s", name, t.Name(), diff(string(want), got))
	}
}

// archo saves (05 §4.2, 10 §4): the list and its filters, one save against its list, the
// fix with and without launching, rename and copy, backup and recycle with and without
// --yes, a pack's save, and an ambiguous reference.
func TestGoldenSaves(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	fakeSteam(e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, filepath.Base(p)); return os.Remove(p) }
	gamma := ref(e, "Gamma")
	runSavesGolden(t, "saves", e, []step{
		cmd("saves", "list"),
		cmd("saves", "list", "--attention"),
		cmd("saves", "list", "--mod", "tester.gone"),
		cmd("saves", "list", "--mod", gamma),
		cmd("saves", "list", "--colony", "ashfall"),
		cmd("saves", "list", "--colony", "Nope"),
		cmd("saves", "list", "--mod", "nothing.here"),
		cmd("saves", "show", "Tundra 2"),
		cmd("saves", "show", "Tundra 2.rws", "--mods"),
		cmd("saves", "show", "Broken"),
		cmd("saves", "show", "Ashfall/Landing"),
		cmd("saves", "show", "Nope"),
		cmd("saves", "fix", "Tundra 2", "--dry-run", "--launch"),
		cmd("saves", "fix", "Tundra 2", "--mod", "tester.gamma"),
		cmd("saves", "show", "Tundra 2"),
		cmd("undo"),
		cmd("saves", "fix", "Tundra 2", "--mod", "tester.nothing"),
		cmd("saves", "fix", "Tundra 2", "--launch"),
		cmd("saves", "fix", "Tundra", "--launch"),
		cmd("saves", "fix", "Broken"),
		cmd("saves", "rename", "Ashfall/Landing", "First day"),
		cmd("saves", "rename", "Tundra", "Tundra 2"),
		cmd("saves", "copy", "Tundra"),
		cmd("saves", "backup", "Tundra", "Broken"),
		cmd("saves", "backup", "Tundra", "Broken", "--yes"),
		cmd("saves", "trash", "Broken"),
		cmd("saves", "trash", "Broken", "--yes"),
		act(func(e *testkit.Env) {
			e.WriteFile(filepath.Join(e.Home, "packs", "P1", "pack.json"), `{"format":1,"name":"P1","entries":[]}`)
			p := filepath.Join(e.Home, "packs", "P1", "userdata", "Saves", "Tundra.rws")
			e.WriteFile(p, testkit.SaveXML(testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}}))
			at := e.Now().Add(-5 * time.Minute)
			os.Chtimes(p, at, at)
		}),
		cmd("saves", "show", "Tundra"),
		cmd("saves", "list", "--colony", "Tundra"),
		func(e *testkit.Env) []string { return []string{"saves", "fix", packSave(e)} },
		// A second game with its own saves: --env (the global flag) keeps one game's saves.
		act(func(e *testkit.Env) {
			old := e.OtherGame("old", "1.5.4409 rev1118")
			e.MustRun("env", "add", "old", "--game", old, "--saves", "own", "--launch", "direct")
			p := filepath.Join(e.Home, "userdata", "old", "Saves", "Outpost.rws")
			e.WriteFile(p, testkit.SaveXML(testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}}))
			at := e.Now().Add(-4 * time.Hour)
			os.Chtimes(p, at, at)
			e.App.EnvName = "old"
			e.MustRun("scan")
		}),
		cmd("saves", "list"),
		cmd("saves", "show", "Outpost"),
		act(func(e *testkit.Env) { e.App.EnvName = "" }),
	})
	if strings.Join(trashed, ",") != "Broken.rws" {
		t.Errorf("trashed %v, want only Broken.rws after --yes", trashed)
	}
	if len(e.Launched) != 1 {
		t.Errorf("launched %d times, want once (saves fix Tundra --launch)", len(e.Launched))
	}
}
