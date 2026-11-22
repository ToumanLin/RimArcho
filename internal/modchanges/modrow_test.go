package modchanges_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// rowLine is a row as one comparable line: kind, tier, text, others, details, files, actions.
func rowLine(r modchanges.Row, kind string) string {
	var acts []string
	for _, a := range r.Actions {
		acts = append(acts, a.Kind)
	}
	var files []string
	for _, f := range r.Files {
		files = append(files, fmt.Sprintf("%s+%d~%d-%d(%d)", f.Kind, f.Added, f.Changed, f.Gone, f.Delta))
	}
	return fmt.Sprintf("%s|%d %s|%s|%s|%s|%s|%s", kind, r.Tier, r.TierText, r.Text, r.Others,
		strings.Join(r.Details, " / "), strings.Join(files, ","), strings.Join(acts, ","))
}

func declBlob(t *testing.T, d scan.Decl) string {
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func deps(pkgs ...string) []scan.Dependency {
	var out []scan.Dependency
	for _, p := range pkgs {
		out = append(out, scan.Dependency{PackageID: p})
	}
	return out
}

// modID is the instance id of the live mod with package id pkg.
func modID(t *testing.T, st *app.LoadoutState, pkg string) string {
	for _, m := range st.Ix.Mods {
		if m.PackageID == pkg && m.Status == identity.Live {
			return m.ID
		}
	}
	t.Fatalf("no mod %s", pkg)
	return ""
}

// 16 §2.9: each branch of a mod's row, from synthetic events and declarations: gone,
// new, copied, a declaration compared field by field, the events' own details when no
// declaration is there to compare, and the fallbacks.
func TestModRowBranches(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("1002", testkit.Sample("tester.beta", "Beta"))
	e.AddWorkshop("1003", testkit.Sample("tester.dep", "Dep"))
	e.AddWorkshop("1004", testkit.Sample("tester.en", "Enabled"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.alpha", "pkg:tester.en")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	alpha, beta, gamma := modID(t, st, "tester.alpha"), modID(t, st, "tester.beta"), modID(t, st, "tester.gamma")
	// Beta's locked copy keeps the old version: its row says the copy stays behind.
	e.MustRun("pin", "ws:1002")
	if st, err = e.App.Loadout(); err != nil {
		t.Fatal(err)
	}
	ev := func(id, kind, detail string) *store.Event {
		return &store.Event{InstanceID: id, Kind: kind, Detail: detail, Before: 901, After: 902}
	}
	full := map[int64]*store.SnapInfo{
		901: {ID: 901, ACFTimeUpdated: 100, TotalSize: 1000, DeclBlob: declBlob(t, scan.Decl{
			PackageID: "tester.alpha", Name: "Alpha Old", ModVersion: "1.0", SupportedVersions: []string{"1.4", "1.5", "1.6"},
			Dependencies: deps("tester.old"), IncompatibleWith: []string{"tester.x"},
			LoadAfter: []string{"tester.la"}, LoadBefore: []string{"tester.lb"}})},
		902: {ID: 902, ACFTimeUpdated: 200, TotalSize: 1500, DeclBlob: declBlob(t, scan.Decl{
			PackageID: "tester.alpha2", Name: "Alpha", ModVersion: "1.1", SupportedVersions: []string{"1.5", "1.7"},
			Dependencies: deps("tester.missing", "tester.dep"), DependenciesByVersion: map[string][]scan.Dependency{"v1.6": deps("tester.en")},
			IncompatibleWith: []string{"tester.en"}, IncompatibleByVersion: map[string][]string{"v1.6": {"tester.beta"}},
			LoadAfter: []string{"tester.a1", "tester.a2"}, ForceLoadAfter: []string{"tester.a3"},
			LoadBeforeByVersion: map[string][]string{"v1.6": {"tester.b1"}}})},
	}
	gained := map[int64]*store.SnapInfo{
		901: {ID: 901, DeclBlob: declBlob(t, scan.Decl{PackageID: "tester.beta", SupportedVersions: []string{"1.5"},
			LoadAfterByVersion: map[string][]string{"v1.5": {"tester.a"}}})},
		902: {ID: 902, DeclBlob: declBlob(t, scan.Decl{PackageID: "tester.beta", ModVersion: "2.0", SupportedVersions: []string{"1.5", "1.6", "1.7"},
			LoadAfterByVersion: map[string][]string{"v1.5": {"tester.b"}}})},
	}
	for _, c := range []struct {
		name string
		id   string
		evs  []*store.Event
		info map[int64]*store.SnapInfo
		want string
	}{
		{"gone, in the list", alpha, []*store.Event{ev(alpha, identity.EvDisappeared, "")}, nil,
			"disappeared|1 affects launch|removed from the mod folder; the current list uses it||||"},
		{"gone, local", gamma, []*store.Event{ev(gamma, identity.EvDisappeared, "")}, nil,
			"disappeared|2 worth a look|removed from the Mods folder||||"},
		{"gone and back", alpha, []*store.Event{ev(alpha, identity.EvDisappeared, ""), ev(alpha, identity.EvReappeared, "")}, nil,
			"|2 worth a look|came back|came back|||workshop"},
		{"new subscription", beta, []*store.Event{ev(beta, identity.EvDiscovered, "")}, nil,
			"discovered|3 content only|new subscription from Workshop||||"},
		{"new local", gamma, []*store.Event{ev(gamma, identity.EvDiscovered, "")}, nil,
			"discovered|3 content only|found in the Mods folder||||"},
		{"a new copy", alpha, []*store.Event{ev(alpha, identity.EvCopied, "")}, nil,
			"discovered|3 content only|a new copy||||"},
		{"new, unknown", "nosuch", []*store.Event{ev("nosuch", identity.EvDiscovered, "")}, nil,
			"discovered|3 content only|found in a mod folder||||"},
		{"declarations compared", alpha, []*store.Event{ev(alpha, identity.EvUpdated, ""), ev(alpha, identity.EvAuthorChanged, "Someone"), ev(alpha, identity.EvMoved, "")}, full,
			"|1 affects launch|package id changed: tester.alpha → tester.alpha2; no longer supports 1.6 (your game); needs tester.missing now (not installed)|package id changed: tester.alpha → tester.alpha2; no longer supports 1.6 (your game); needs tester.missing now (not installed)|size +500 B / needs Dep now (not enabled) / now incompatible with Enabled (enabled) / no longer supports 1.4 / now supports 1.7 / needs Enabled now / no longer needs tester.old / now incompatible with Beta / now loads after tester.a1, tester.a2, +1 / no longer loads after tester.la / now loads before tester.b1 / no longer loads before tester.lb / renamed from Alpha Old / no longer incompatible with tester.x / author: Someone / its folder moved / version 1.0 → 1.1 / updated on the Workshop Jan 1||workshop,deps,sort,lock"},
		{"declarations, gains", beta, []*store.Event{ev(beta, identity.EvUpdated, "")}, gained,
			"|2 worth a look|now supports 1.6 (your game); now supports 1.7; load rules changed|now supports 1.6 (your game); now supports 1.7; load rules changed|your locked version stays behind / version 2.0||workshop"},
		{"the events' details", alpha, []*store.Event{
			ev(alpha, identity.EvPackageIDChanged, "pkg:a → pkg:b"), ev(alpha, identity.EvVersionSupport, "1.5 → 1.6"),
			ev(alpha, identity.EvDependencies, ""), ev(alpha, identity.EvLoadRules, ""), ev(alpha, identity.EvRenamed, "Old → New")}, nil,
			"|1 affects launch|package id changed: a → b; supported versions: 1.5 → 1.6; dependencies changed|package id changed: a → b; supported versions: 1.5 → 1.6; dependencies changed|load rules changed / renamed: Old → New||workshop,deps,sort"},
		{"a locked copy behind", beta, []*store.Event{{InstanceID: beta, Kind: identity.EvUpdated}}, nil,
			"|2 worth a look|your locked version stays behind|your locked version stays behind|||workshop"},
		{"updated, nothing else", gamma, []*store.Event{{InstanceID: gamma, Kind: identity.EvUpdated}}, nil,
			"|3 content only|updated|updated|||"},
		{"an event with no fact", beta, []*store.Event{{InstanceID: beta, Kind: identity.EvVersionSupport}}, nil,
			"|3 content only|supported versions changed|supported versions changed|||workshop"},
	} {
		r, kind := modchanges.ModRow(e.App, st, true, c.id, c.evs, c.info)
		if got := rowLine(r, kind); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// 16 §2.9: rows from real scans: code, LoadFolders.xml, sounds, a new mod version beside
// code and beside media only, and a locked copy that stays behind its updated original.
func TestModRowsFromScans(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("1002", testkit.Sample("tester.beta", "Beta"))
	e.AddWorkshop("1003", testkit.Sample("tester.gamma", "Gamma"))
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:tester.alpha", "pkg:tester.beta")
	e.MustRun("pin", "ws:1001")
	e.Advance(time.Hour)
	e.RecordLaunch(e.Now())
	e.Advance(time.Hour)
	a := testkit.Sample("tester.alpha", "Alpha")
	a.Files["Defs/ThingDefs/alpha.xml"] = "<Defs><ThingDef><defName>alpha_Other</defName></ThingDef></Defs>\n"
	e.WriteMod(filepath.Join(e.Workshop, "1001"), a)
	b := testkit.Sample("tester.beta", "Beta")
	b.Files["Assemblies/Beta.dll"] = strings.Repeat("d", 2048)
	b.Files["About/About.xml"] = strings.Replace(b.AboutXML(), "<name>", "<modVersion>3.1</modVersion>\n  <name>", 1)
	b.Files["LoadFolders.xml"] = "<loadFolders><v1.6><li>/</li></v1.6></loadFolders>\n"
	e.WriteMod(filepath.Join(e.Workshop, "1002"), b)
	g := testkit.Sample("tester.gamma", "Gamma")
	g.Files["About/About.xml"] = strings.Replace(g.AboutXML(), "<name>", "<modVersion>1.2</modVersion>\n  <name>", 1)
	g.Files["Sounds/gamma.ogg"] = strings.Repeat("s", 512)
	e.WriteMod(filepath.Join(e.Workshop, "1003"), g)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := modchanges.Build(e.App, st, modchanges.Opts{Library: true}, modchanges.English(time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range p.Rows {
		got = append(got, r.Mod+": "+rowLine(r, ""))
	}
	want := []string{
		"Alpha: |2 worth a look|Defs changed; your locked version stays behind (1 updates)|your locked version stays behind (1 updates)|Defs 1 changed|Defs+0~1-0(0)|workshop",
		"Beta: |2 worth a look|LoadFolders.xml changed; code changed; version 3.1|LoadFolders.xml changed; version 3.1|code 1 new; About/LoadFolders 1 changed, 1 new · in all +2.1 KB|code+1~0-0(0),About/LoadFolders+1~1-0(0)|workshop,lock",
		"Gamma: |3 content only|version 1.2; only sounds changed|version 1.2|About/LoadFolders 1 changed; sounds +512 B · in all +543 B|sounds+1~0-0(512)|workshop",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
