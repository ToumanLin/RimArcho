package view_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestChangesAfterMaintenance(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddEnv(false)
	e.MustRun("scan")
	var id string
	if err := e.App.Store.DB.QueryRow(`SELECT id FROM instance WHERE rel_path='1001'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// Real synthetic scans exercise the same snapshots the history page reads.
	for v := 0; v < 6; v++ {
		e.Advance(time.Hour)
		m := testkit.Sample("tester.alpha", "Alpha")
		m.Files["Defs/updated.xml"] = strings.Repeat("x", v+1)
		e.WriteMod(filepath.Join(e.Workshop, "1001"), m)
		e.MustRun("scan")
	}
	if _, err := e.App.Maintain(context.Background(), false, false); err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := modchanges.Build(e.App, st, modchanges.Opts{Of: []string{id}}, modchanges.English(time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, day := range p.Older {
		for _, row := range day.Rows {
			if slices.Contains(row.Details, "(file lists no longer stored)") {
				found = true
				if strings.Contains(row.Text, "only") {
					t.Fatalf("invented partial file comparison: %+v", row)
				}
			}
		}
	}
	if !found {
		t.Fatal("history missing cleanup explanation")
	}
}

// 06 §2.9: one row per mod since the plan was last played, tiered by what the change means
// and worded from the recorded data only; the whole library adds the other mods and new
// ones; older changes go by day; one mod's Lineage lists every change.
func TestChangesPage(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	mod := func(pkg, name string, f func(*testkit.Mod)) testkit.Mod {
		m := testkit.Sample(pkg, name)
		m.Supported = []string{"1.5", "1.6"}
		if f != nil {
			f(&m)
		}
		return m
	}
	e.AddWorkshop("1001", mod("tester.alpha", "Alpha", nil))
	e.AddWorkshop("1002", mod("tester.beta", "Beta", nil))
	e.AddWorkshop("1003", mod("tester.gamma", "Gamma", nil))
	e.AddWorkshop("1004", mod("tester.delta", "Delta", nil))
	e.AddWorkshop("1005", mod("tester.dep", "Dep", nil))
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.alpha", "pkg:tester.beta", "pkg:tester.gamma")
	e.Advance(time.Hour)
	e.RecordLaunch(e.Now())

	e.Advance(time.Hour)
	// Alpha drops the game's version and needs a mod that is not installed and one that
	// is not enabled; Beta changes a texture; Gamma its Defs; Delta (not in the plan) too.
	e.WriteMod(filepath.Join(e.Workshop, "1001"), mod("tester.alpha", "Alpha", func(m *testkit.Mod) {
		m.Supported = []string{"1.5"}
		m.Deps = []string{"tester.missing", "tester.dep"}
	}))
	e.WriteMod(filepath.Join(e.Workshop, "1002"), mod("tester.beta", "Beta", func(m *testkit.Mod) {
		m.Files["Textures/Things/beta/a.png"] = strings.Repeat("y", 4096)
	}))
	e.WriteMod(filepath.Join(e.Workshop, "1003"), mod("tester.gamma", "Gamma", func(m *testkit.Mod) {
		m.Files["Defs/ThingDefs/gamma.xml"] = "<Defs><ThingDef><defName>gamma_Other</defName></ThingDef></Defs>\n"
	}))
	e.WriteMod(filepath.Join(e.Workshop, "1004"), mod("tester.delta", "Delta", func(m *testkit.Mod) {
		m.Files["Patches/more.xml"] = "<Patch/>\n"
	}))
	e.AddWorkshop("1006", mod("tester.new", "Newcomer", nil))
	e.MustRun("scan")

	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildChanges(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	names := func(rs []modchanges.Row) (out []string) {
		for _, r := range rs {
			out = append(out, r.Mod)
		}
		return
	}
	if got := names(p.Rows); !slices.Equal(got, []string{"Alpha", "Gamma", "Beta"}) || p.Arrivals != nil {
		t.Fatalf("plan rows %v, arrivals %+v", got, p.Arrivals)
	}
	if !p.Played || !strings.HasPrefix(p.Since, "since you last played") || p.Launch != 1 {
		t.Fatalf("since %q played %v launch %d", p.Since, p.Played, p.Launch)
	}
	a, g, b := p.Rows[0], p.Rows[1], p.Rows[2]
	if a.Tier != modchanges.TierLaunch || !strings.Contains(a.Text, "no longer supports 1.6 (your game)") ||
		!strings.Contains(a.Text, "needs tester.missing now (not installed)") || !strings.Contains(a.Text, "needs Dep now (not enabled)") {
		t.Fatalf("Alpha: %+v", a)
	}
	kinds := func(r modchanges.Row) (out []string) {
		for _, x := range r.Actions {
			out = append(out, x.Kind)
		}
		return
	}
	if got := kinds(a); !slices.Equal(got, []string{modchanges.ActWorkshop, modchanges.ActDeps, modchanges.ActSort, modchanges.ActLock}) {
		t.Fatalf("Alpha's actions %v", got)
	}
	if g.Tier != modchanges.TierWatch || g.Text != "Defs changed" || len(g.Details) == 0 || !strings.HasPrefix(g.Details[0], "Defs 1 changed") {
		t.Fatalf("Gamma: %+v", g)
	}
	if b.Tier != modchanges.TierContent || b.Text != "only textures changed" || !strings.HasPrefix(b.Details[0], "textures +") {
		t.Fatalf("Beta: %+v", b)
	}
	if fs := view.ChangeFiles(e.App, b.Before, b.After); len(fs) != 1 || !strings.HasPrefix(fs[0], "~ Textures/Things/beta/a.png") {
		t.Fatalf("Beta's files: %v", fs)
	}

	// The whole library: Delta too, and the new mod as the summary row.
	lib, _ := view.BuildChanges(e.App, st, modchanges.Opts{Library: true})
	if got := names(lib.Rows); !slices.Contains(got, "Delta") || lib.Arrivals == nil || lib.Arrivals.Text != "1 new subscription" {
		t.Fatalf("library rows %v, arrivals %+v", got, lib.Arrivals)
	}
	// Delta is not in the plan: the same kind of change is only worth a look.
	if h := view.ChangeHome(e.App, st); h.Count != 3 || h.Launch != 1 || h.Text != "3 mods changed, 1 need attention" {
		t.Fatalf("home %+v", h)
	}

	// Played again, then Beta changes: the earlier changes move under their day.
	e.Advance(26 * time.Hour)
	e.RecordLaunch(e.Now())
	e.Advance(time.Hour)
	e.WriteMod(filepath.Join(e.Workshop, "1002"), mod("tester.beta", "Beta", func(m *testkit.Mod) {
		m.Files["Textures/Things/beta/a.png"] = strings.Repeat("y", 4096)
		m.Files["Languages/English/Keyed/beta.xml"] = "<LanguageData><x>y</x></LanguageData>\n"
	}))
	e.MustRun("scan")
	st, _ = e.App.Loadout()
	p, _ = view.BuildChanges(e.App, st, modchanges.Opts{})
	if got := names(p.Rows); !slices.Equal(got, []string{"Beta"}) || p.Rows[0].Text != "only translations changed" || len(p.Older) != 1 || len(p.Older[0].Rows) != 3 {
		t.Fatalf("rows %v (%+v), older %+v", got, p.Rows, p.Older)
	}

	// One mod's Lineage: every change by day, nothing else.
	beta := p.Rows[0].ID
	of, _ := view.BuildChanges(e.App, st, modchanges.Opts{Of: view.LineageIDs(st.Ix, beta), OfName: "Beta"})
	if len(of.Rows) != 0 || len(of.Older) != 3 || of.Count() != 3 {
		t.Fatalf("Beta's history: %+v", of)
	}
	for _, d := range of.Older {
		for _, r := range append(d.Rows, rowsOf(d.Arrivals)...) {
			if r.Mod != "Beta" {
				t.Fatalf("another mod: %+v", r)
			}
		}
	}
}

func rowsOf(g *modchanges.Group) []modchanges.Row {
	if g == nil {
		return nil
	}
	return g.Rows
}

// Shared modchanges output keeps CLI punctuation while both interface languages use gaps.
func TestChangesWording(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	e.Advance(time.Hour)
	e.AddWorkshop("1001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("1002", testkit.Sample("tester.beta", "Beta"))
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { view.SetLang("en") })
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		p, err := view.BuildChanges(e.App, st, modchanges.Opts{Library: true})
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if p.Arrivals == nil || p.Arrivals.Text == "" || !strings.Contains(p.Since, "  ") {
			t.Fatalf("missing fixture text: %+v", p)
		}
		for _, problem := range view.WordingTextProblems(lang+" changes", string(b)) {
			t.Error(problem)
		}
	}
	p, err := modchanges.Build(e.App, st, modchanges.Opts{Library: true}, modchanges.English(e.App.Loc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsRune(p.Since, '\u00b7') {
		t.Errorf("CLI punctuation changed: %q", p.Since)
	}
}
