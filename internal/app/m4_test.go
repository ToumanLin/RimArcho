package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

// B5 replays the rim_mod_inspector tests that apply to RimArcho (copy-local arrives in M5).

// B5 / test_mod_selector_and_instance_active_state_are_distinct: a packageId selects every
// instance, and each one reports its own enabled state.
func TestSelectorVersusActiveState(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.AddWorkshop("4000000001", testkit.Mod{PackageID: "tester.moda", Name: "Mod A (workshop)", Author: "Tester"})
	e.MustRun("scan")
	out, code := e.Run("mod", "pkg:tester.moda")
	if code != 3 || !strings.Contains(out, "Mod A (workshop)") || !strings.Contains(out, "Mod A ") {
		t.Fatalf("pkg: selects both instances (exit 3):\n%s", out)
	}
	find := e.MustRun("find", "tester.moda")
	var active, inactive int
	for _, l := range strings.Split(find, "\n") {
		switch {
		case strings.Contains(l, "Mod A (workshop)") && !strings.Contains(l, "active "):
			inactive++
		case strings.Contains(l, "Mod A") && strings.Contains(l, "active 3/"):
			active++
		}
	}
	if active != 1 || inactive != 1 {
		t.Fatalf("find shows each instance's own state (active=%d inactive=%d):\n%s", active, inactive, find)
	}
}

// B5 / test_active_rg_excludes_languages_and_enriches_matches.
func TestFilesSearchLanguages(t *testing.T) {
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Needle"), testkit.Mod{PackageID: "example.mod", Name: "Example Mod", Files: map[string]string{
		"Defs/Thing.xml":             "<defName>Needle</defName>\n",
		"Languages/English/Keys.xml": "<Needle>Text</Needle>\n",
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "example.mod")
	e.AddEnv(false)
	e.MustRun("scan")
	out := e.MustRun("files", "search", "Needle")
	if !strings.Contains(out, ": 1 matches") || strings.Contains(out, "Languages") || !strings.Contains(out, "Defs/Thing.xml:1") {
		t.Fatalf("Languages excluded, Defs match kept:\n%s", out)
	}
	ix, _ := e.App.Index()
	if r := ix.Ref(ix.Mods[len(ix.Mods)-1].ID); !strings.Contains(out, "#") {
		t.Fatalf("match carries the mod ref (%s):\n%s", r, out)
	}
	if all := e.MustRun("files", "search", "Needle", "--languages"); !strings.Contains(all, ": 2 matches") {
		t.Fatalf("--languages includes translations:\n%s", all)
	}
}

// fakeILSpy writes one .cs file per call and counts the calls.
type fakeILSpy struct {
	mu    sync.Mutex // source search decompiles in parallel
	calls []string
}

func (f *fakeILSpy) run(_ context.Context, dll, outDir, typ string, refs []string) error {
	f.mu.Lock()
	f.calls = append(f.calls, filepath.Base(dll)+"|"+typ)
	f.mu.Unlock()
	os.MkdirAll(filepath.Join(outDir, "ModA"), 0o755)
	if typ != "" {
		return os.WriteFile(filepath.Join(outDir, strings.ReplaceAll(typ, "+", ".")+".cs"), []byte("// "+typ+"\nclass X {\n\tvoid CompTick() { parent.AllComps.Remove(this); }\n}\n"), 0o644)
	}
	return os.WriteFile(filepath.Join(outDir, "ModA", "CompBroken.cs"), []byte("namespace ModA {\n\tpublic class CompBroken {\n\t\tpublic void CompTick() { parent.AllComps.Remove(this); }\n\t}\n}\n"), 0o644)
}

// B5 / test_inspector_db_survives_clean_and_marks_outputs_stale: the decompile cache is
// keyed by DLL hash, survives a deleted cache directory (decompiled again) and is
// invalidated by a changed DLL.
func TestSourceCache(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	modA := ref(t, e, "Mod A")
	if out := e.MustRun("source", modA); !strings.Contains(out, "ModA.dll  decompiled now, 1 .cs files") {
		t.Fatalf("first decompile:\n%s", out)
	}
	if out := e.MustRun("source", modA); !strings.Contains(out, "ModA.dll  cached") || len(f.calls) != 1 {
		t.Fatalf("second call uses the cache (calls %v):\n%s", f.calls, out)
	}
	os.RemoveAll(filepath.Join(e.Home, "cache", "source"))
	if out := e.MustRun("source", modA); !strings.Contains(out, "decompiled now") || len(f.calls) != 2 {
		t.Fatalf("deleted cache is rebuilt (calls %v):\n%s", f.calls, out)
	}
	dll := filepath.Join(e.Game, "Mods", "ModA", "Assemblies", "ModA.dll")
	b, _ := os.ReadFile(dll)
	os.WriteFile(dll, append(b, 0), 0o644)
	if out := e.MustRun("source", modA); !strings.Contains(out, "decompiled now") || len(f.calls) != 3 {
		t.Fatalf("a changed DLL is decompiled again (calls %v):\n%s", f.calls, out)
	}
	ix, _ := e.App.Index()
	m, _ := ix.ResolveOne(modA)
	recs, err := e.App.Store.DecompileRecords(m.ID)
	if err != nil || len(recs) != 2 {
		t.Fatalf("decompile records survive the cache dir (one per hash): %d %v", len(recs), err)
	}
	out := e.MustRun("source", modA, "--type", "ModA.CompBroken")
	if !strings.Contains(out, "ModA.CompBroken  (5 lines)") || !strings.Contains(out, "   3          public void CompTick()") {
		t.Fatalf("--type from the cached project (tabs become spaces):\n%s", out)
	}
	if out := e.MustRun("source", "search", "AllComps.Remove(", "--in", modA); !strings.Contains(out, "1 matches") || !strings.Contains(out, "CompBroken.cs:3") {
		t.Fatalf("source search:\n%s", out)
	}
	// A type of an assembly that is not cached is decompiled alone (-t).
	modB := ref(t, e, "Mod B")
	if out := e.MustRun("source", modB, "--type", "ModB.Pawn_Tick_B"); !strings.Contains(out, "// ModB.Pawn_Tick_B") || f.calls[len(f.calls)-1] != "ModB.dll|ModB.Pawn_Tick_B" {
		t.Fatalf("single-type decompile (calls %v):\n%s", f.calls, out)
	}
}

// Without ilspycmd the text says how to install it; nothing is installed.
func TestSourceWithoutILSpy(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.App.Decompile = func(context.Context, string, string, string, []string) error { return app.ErrNoILSpy }
	out, code := e.Run("source", ref(t, e, "Mod A"))
	if code != 1 || !strings.Contains(out, "dotnet tool install --global ilspycmd") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if out := e.MustRun("source", ref(t, e, "Harmony")); !strings.Contains(out, "no assemblies to decompile") {
		t.Fatalf("0Harmony.dll is a known library:\n%s", out)
	}
}

func ref(t *testing.T, e *testkit.Env, name string) string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.Name == name {
			return ix.Ref(m.ID)
		}
	}
	t.Fatalf("no mod %q", name)
	return ""
}

// decisions-m4 §7.1/§7.2: skip-prefix is info until the newest log's stack passes through
// the method; the newest Player.log joins the problems without --select.
func TestSkipPrefixEscalation(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	sev := func() (string, string) {
		ps := problems(t, e)
		for _, p := range ps.List {
			if p.Code == "harmony.skip-prefix" {
				ev := ""
				for _, x := range p.Evidence {
					ev += x.Text + ";"
				}
				return p.Severity, ev
			}
		}
		t.Fatal("no skip-prefix")
		return "", ""
	}
	if s, ev := sev(); s != "warning" || !strings.Contains(ev, "Verse.Pawn.Tick is in the stack of log:E3 (x500)") {
		t.Fatalf("with the log: %s %s", s, ev)
	}
	logs := 0
	for _, p := range problems(t, e).List {
		if strings.HasPrefix(p.Code, "log.") {
			logs++
		}
	}
	if logs != 4 {
		t.Fatalf("newest log not included automatically: %d log problems", logs)
	}
	os.Remove(filepath.Join(e.UserData, "Player.log"))
	if s, _ := sev(); s != "info" {
		t.Fatalf("without a log: %s", s)
	}
}

// decisions-m4 §7.3: a log from another game version is attributed against that version.
func TestLogOwnVersion(t *testing.T) {
	e := testkit.New(t)
	log := e.HarmonyLogFixture()
	p := filepath.Join(e.UserData, "Player.log")
	os.WriteFile(p, []byte(strings.Replace(log, "RimWorld 1.6.4633 rev1261", "RimWorld 1.5.4409 rev1118", 1)), 0o644)
	out := e.MustRun("log")
	if !strings.Contains(out, "analysed: as 1.5 (log version); environment is 1.6") {
		t.Fatalf("header:\n%s", out)
	}
	if out := e.MustRun("log", "--game-version", "1.6"); strings.Contains(out, "analysed:") {
		t.Fatalf("explicit --game-version wins:\n%s", out)
	}
}

// decisions-m4 §7.4: searching every enabled mod prints an estimate and needs --yes;
// --implicated covers the newest log's candidates.
func TestSourceSearchScope(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	if _, code := e.Run("source", "search", "AllComps"); code != 2 {
		t.Fatalf("no scope is a usage error, exit %d", code)
	}
	out := e.MustRun("source", "search", "AllComps", "--active")
	if !strings.Contains(out, "4 not decompiled yet") || !strings.Contains(out, "nothing was decompiled") || len(f.calls) != 0 {
		t.Fatalf("estimate (calls %v):\n%s", f.calls, out)
	}
	out = e.MustRun("source", "search", "AllComps.Remove(", "--implicated")
	if !strings.Contains(out, "implicated mods") || len(f.calls) == 0 || len(f.calls) > 3 {
		t.Fatalf("implicated (calls %v):\n%s", f.calls, out)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "ModManual.dll") {
			t.Fatalf("Manual Patcher is not implicated by the log: %v", f.calls)
		}
	}
	if out := e.MustRun("source", "search", "AllComps", "--active", "--yes"); !strings.Contains(out, "in the enabled mods") || len(f.calls) != 4 {
		t.Fatalf("--yes decompiles the rest (calls %v):\n%s", f.calls, out)
	}
}
