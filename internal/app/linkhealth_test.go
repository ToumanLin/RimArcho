package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func linkHealthEnv(t *testing.T) (*testkit.Env, string, string) {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(true)
	e.Game = mustReal(t, e.Game)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	target := filepath.Join(e.Root, "health", "Known")
	e.WriteMod(target, testkit.Sample("health.known", "Known"))
	e.MustRun("folder", "add", "extra", filepath.Dir(target))
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.PackageID == "health.known" {
			return e, target, m.ID
		}
	}
	t.Fatal("missing fixture mod")
	return nil, "", ""
}

func healthByName(t *testing.T, e *testkit.Env) map[string]*app.GameLink {
	t.Helper()
	links, err := e.App.InspectGameLinks()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*app.GameLink{}
	for _, l := range links {
		out[filepath.Base(l.Path)] = l
	}
	return out
}

func TestLinkHealthClassificationFreshAndRepair(t *testing.T) {
	e, target, id := linkHealthEnv(t)
	mods := filepath.Join(e.Game, "Mods")
	known := filepath.Join(mods, "_Archo_extra_Known")
	foreign := filepath.Join(mods, "_Archo_old_unknown")
	occupied := filepath.Join(mods, "_Archo_real")
	envlink := filepath.Join(mods, "_Archo_env_"+id)
	player := filepath.Join(mods, "OtherManager")
	e.Junction(known, target)
	e.Junction(envlink, target)
	e.Junction(foreign, filepath.Join(e.Root, "missing"))
	e.MkdirAll(occupied)
	e.WriteFile(filepath.Join(occupied, "keep"), "keep")
	e.Junction(player, filepath.Join(e.Root, "missing-player"))
	nested := filepath.Join(occupied, "_Archo_nested")
	e.Junction(nested, filepath.Join(e.Root, "missing-nested"))
	links := healthByName(t, e)
	if links["_Archo_extra_Known"].Code != "" || links["_Archo_real"].Code != app.CodeLinkOccupied || links["_Archo_old_unknown"].Code != app.CodeLinkDangling || links["_Archo_old_unknown"].Owned {
		t.Fatalf("classification: %+v", links)
	}
	if links["_Archo_nested"] != nil || links["OtherManager"] != nil {
		t.Fatal("inspected outside first-level prefixes")
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.GameLinks) != len(links) {
		t.Fatal("Problems lost links")
	}
	// Delete only synthetic target content; Mods mtime does not change.
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	links = healthByName(t, e)
	if links["_Archo_extra_Known"].Code != app.CodeLinkDangling || !links["_Archo_extra_Known"].Owned {
		t.Fatal("target deletion was cached or lost ownership evidence")
	}
	ps, err = e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ps.List {
		if p.Code == app.CodeLinkDangling && p.Key != "" && strings.HasPrefix(p.Subjects[0], "path:") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing fresh game-level finding")
	}
	preview, err := e.App.PlanLinkRepair(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out, code := e.Run("links", "repair", "--all", "--dry-run", "--json"); code != 0 || !json.Valid([]byte(out)) {
		t.Fatalf("preview: %d %s", code, out)
	}
	if !source.IsLink(known) || !source.IsLink(foreign) {
		t.Fatal("dry run changed links")
	}
	if err := e.App.ApplyLinkRepair(preview); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{known, envlink} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("not removed: %s (%v)", p, err)
		}
	}
	for _, p := range []string{foreign, player, nested, occupied} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("repair touched protected entry %s: %v", p, err)
		}
	}
	// --yes alone leaves unproven links; --include names exactly what the player selected.
	e.MustRun("links", "repair", "--all", "--yes")
	if !source.IsLink(foreign) {
		t.Fatal("unproven link removed without inclusion")
	}
	e.MustRun("links", "repair", "--all", "--yes", "--include", foreign)
	if _, err := os.Lstat(foreign); !os.IsNotExist(err) {
		t.Fatal("explicitly included link remains")
	}
}

func TestLinkHealthForeignAndHealthyEnvStay(t *testing.T) {
	e, target, id := linkHealthEnv(t)
	mods := filepath.Join(e.Game, "Mods")
	wrong := filepath.Join(e.Root, "wrong")
	e.MkdirAll(wrong)
	e.WriteFile(filepath.Join(wrong, "keep"), "untouched")
	link := filepath.Join(mods, "_Archo_extra_Known")
	envlink := filepath.Join(mods, "_Archo_env_"+id)
	unproven := filepath.Join(mods, "!ArchoPack_unknown_mod")
	e.Junction(link, wrong)
	e.Junction(envlink, wrong)
	e.Junction(unproven, wrong)
	by := healthByName(t, e)
	if by[filepath.Base(link)].Code != app.CodeLinkForeign || by[filepath.Base(link)].Action != "remove link" || by[filepath.Base(envlink)].Action != "keep" {
		t.Fatalf("foreign: %+v", by)
	}
	e.MustRun("links", "repair", "--all", "--yes", "--include", envlink)
	if source.IsLink(link) || !source.IsLink(envlink) || !source.IsLink(unproven) {
		t.Fatal("foreign repair crossed its boundary")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("known target was touched")
	}
	if _, err := os.Stat(filepath.Join(wrong, "keep")); err != nil {
		t.Fatal("foreign target was touched")
	}
}

func TestLinkHealthRepairRaceAndRunning(t *testing.T) {
	for _, change := range []string{"retarget", "directory", "parent", "restored", "running"} {
		t.Run(change, func(t *testing.T) {
			e, target, id := linkHealthEnv(t)
			mods := filepath.Join(e.Game, "Mods")
			link := filepath.Join(mods, "_Archo_extra_Known")
			e.Junction(link, target)
			second := filepath.Join(mods, "_Archo_env_"+id)
			e.Junction(second, target)
			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			plan, err := e.App.PlanLinkRepair(true, nil)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "retarget":
				if err := source.RemoveLink(link); err != nil {
					t.Fatal(err)
				}
				e.Junction(link, filepath.Join(e.Root, "another-missing"))
			case "directory":
				if err := source.RemoveLink(link); err != nil {
					t.Fatal(err)
				}
				e.MkdirAll(link)
			case "parent":
				if err := os.Rename(mods, mods+"-old"); err != nil {
					t.Fatal(err)
				}
				e.MkdirAll(mods)
				e.Junction(link, target)
			case "restored":
				e.MkdirAll(target)
			case "running":
				e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
			}
			if err := e.App.ApplyLinkRepair(plan); err == nil {
				t.Fatal("changed preview was executed")
			}
			if _, err := os.Lstat(link); err != nil {
				t.Fatal("entry was removed on refusal")
			}
			if change == "parent" {
				second = filepath.Join(mods+"-old", filepath.Base(second))
			}
			if !source.IsLink(second) {
				t.Fatal("another link was removed before batch verification finished")
			}
		})
	}
}

func TestLinkHealthLaunchAndTestRunPreflight(t *testing.T) {
	e, _, _ := linkHealthEnv(t)
	e.MustRun("pack", "build", "healthpack")
	link := filepath.Join(e.Game, "Mods", "_Archo_old_missing")
	e.Junction(link, filepath.Join(e.Root, "gone"))
	before := e.ReadModsConfig()
	if _, err := e.App.Launch(false); !errors.Is(err, app.ErrLaunchBlocked) {
		t.Fatalf("launch: %v", err)
	}
	if _, err := e.App.LaunchPack("healthpack", false); !errors.Is(err, app.ErrLaunchBlocked) {
		t.Fatalf("pack launch: %v", err)
	}
	started := false
	e.App.TestGameStart = func(*exec.Cmd) error { started = true; return errors.New("must not start") }
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{})
	if err != nil || r == nil || r.Status != "links-blocked" || len(r.Links) != 1 || started {
		t.Fatalf("test run: %+v %v started=%v", r, err, started)
	}
	out, code := cmds.Execute(e.App, []string{"test", "run", "--json"})
	if code == 0 || !json.Valid([]byte(out)) || !strings.Contains(out, "mods.link-dangling") {
		t.Fatalf("structured refusal: %d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.Home, "tests")); !os.IsNotExist(err) {
		t.Fatal("test preflight prepared data")
	}
	if len(e.Launched) > 0 || e.ReadModsConfig() != before || !source.IsLink(link) {
		t.Fatal("preflight changed game state")
	}
	low, err := gametest.Run(context.Background(), gametest.Options{GameRoot: e.Game, DataDir: filepath.Join(e.Root, "should-not-exist"), Start: e.App.TestGameStart})
	if err != nil || low.Status != "links-blocked" || started {
		t.Fatalf("low-level preflight: %+v %v", low, err)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatal("runner prepared data")
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	needs := view.BuildNeeds(e.App, ps.State, ps)
	action := view.Action{}
	for _, n := range needs.Items {
		for _, a := range n.Actions {
			if a.Kind == view.ActRepairLinks {
				action = a
			}
		}
	}
	if action.Kind == "" || !strings.Contains(action.Warn, link) || !strings.Contains(action.Warn, "gone") {
		t.Fatal("needs confirmation lost link evidence")
	}
	// The unproven link must be deliberately included via its specific action.
	if _, err := view.Do(e.App, action, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("confirmed action did not remove link")
	}
}

func TestLinkHealthOtherGamesDoNotBlockAndRootsDeduplicate(t *testing.T) {
	e, _, _ := linkHealthEnv(t)
	other := secondGame(e, "Other")
	e.MustRun("env", "add", "other", "--game", other)
	e.Junction(filepath.Join(other, "Mods", "_Archo_missing"), filepath.Join(e.Root, "gone"))
	all, err := e.App.InspectGameLinks()
	if err != nil || len(all) != 1 {
		t.Fatalf("duplicate roots: %+v %v", all, err)
	}
	if out, code := e.Run("links", "check", "--json"); code != 0 || strings.Contains(out, "_Archo_missing") {
		t.Fatalf("selected scope: %d %s", code, out)
	}
	if out, code := e.Run("links", "check", "--all", "--json"); code == 0 || !strings.Contains(out, "_Archo_missing") {
		t.Fatalf("all scope: %d %s", code, out)
	}
	if _, err := e.App.Launch(true); err != nil {
		t.Fatalf("other game blocked healthy launch: %v", err)
	}
	out := e.MustRun("scan")
	if !strings.Contains(out, app.CodeLinkDangling) {
		t.Fatal("scan silently dropped broken link")
	}
}

func TestStaleIDLinkKnownLiveTarget(t *testing.T) {
	e, target, id := linkHealthEnv(t)
	name := "_Archo_env_deleted-instance"
	path := filepath.Join(e.Game, "Mods", name)
	// Resolve a physical alias and normalized path rather than relying on the name.
	alias := filepath.Join(e.Root, "target-alias")
	e.Junction(alias, filepath.Dir(target))
	e.Junction(path, filepath.Join(alias, filepath.Base(target)))
	l := healthByName(t, e)[name]
	if !l.Owned || l.Code != "" || l.Instance != id || l.Action != "keep" || l.Evidence != "target is known instance "+id {
		t.Fatalf("known live target: %+v", l)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		for _, subject := range p.Subjects {
			if subject == "path:"+path {
				t.Fatalf("healthy link reported: %+v", p)
			}
		}
	}
	needs := view.BuildNeeds(e.App, ps.State, ps)
	for _, n := range needs.Items {
		if len(n.LinkGroups) > 0 {
			t.Fatalf("healthy link on needs: %+v", n)
		}
	}
	out, code := e.Run("links", "check", "--all")
	if code != 0 || !strings.Contains(out, "keep") || strings.Contains(out, app.CodeLinkForeign) {
		t.Fatalf("check: %d %s", code, out)
	}
	// Ownership does not suppress the existing duplicate-entry finding.
	e.Junction(filepath.Join(e.Game, "Mods", "_Archo_env_another-old-id"), target)
	ps, err = e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		if p.Code == app.CodeModsDuplicate {
			return
		}
	}
	t.Fatal("duplicate entry was hidden by ownership fallback")
}

func TestStaleIDLinkUnknownTarget(t *testing.T) {
	e, _, _ := linkHealthEnv(t)
	target := filepath.Join(e.Root, "unknown-target")
	e.MkdirAll(target)
	name := "_Archo_env_deleted-instance"
	path := filepath.Join(e.Game, "Mods", name)
	e.Junction(path, target)
	l := healthByName(t, e)[name]
	if l.Owned || l.Code != app.CodeLinkForeign || l.Action != "keep" {
		t.Fatalf("unknown target adopted: %+v", l)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if l = healthByName(t, e)[name]; l.Code != app.CodeLinkDangling {
		t.Fatalf("dangling: %+v", l)
	}
}
