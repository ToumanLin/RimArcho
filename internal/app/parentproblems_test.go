package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func parentFixture(t *testing.T) (*testkit.Env, *app.ProblemSet, *app.Problem, string) {
	t.Helper()
	e := testkit.New(t)
	mod := testkit.Mod{PackageID: "test.child", Name: "Child", Deps: []string{"test.required"}, Files: map[string]string{
		"Defs/Child.xml": `<Defs><HediffDef ParentName="Base"><defName>Illness</defName></HediffDef><PawnKindDef ParentName="Beast"><defName>Beast_A</defName></PawnKindDef><PawnKindDef ParentName="Beast"><defName>Beast_B</defName></PawnKindDef><ThingDef MayRequire="test.soft"><defName>Optional</defName></ThingDef></Defs>`,
	}}
	mod.Files["About/About.xml"] = strings.Replace(mod.AboutXML(), "</ModMetaData>", `<loadAfter><li>test.order</li><li>test.unknown</li></loadAfter><modDependenciesByVersion><v1.5><li><packageId>test.wrongversion</packageId></li></v1.5><v1.6><li><packageId>test.direct</packageId><displayName>Direct</displayName><steamWorkshopUrl>https://steamcommunity.com/sharedfiles/filedetails/?id=3001</steamWorkshopUrl></li></v1.6></modDependenciesByVersion></ModMetaData>`, 1)
	e.AddWorkshop("1001", mod)
	provider := e.AddWorkshop("1002", testkit.Mod{PackageID: "test.provider", Name: "Provider", Files: map[string]string{"Defs/Base.xml": `<Defs><HediffDef Name="Base" Abstract="True" /></Defs>`}})
	e.AddWorkshop("1003", testkit.Mod{PackageID: "test.decoy", Name: "Decoy", Files: map[string]string{"Defs/Base.xml": `<Defs><PawnKindDef><defName>Beast</defName></PawnKindDef><PawnKindDef Name="Beast" Abstract="True" MayRequire="test.absent" /></Defs>`}})
	e.WriteFile(filepath.Join(e.Home, "cache", "steamDB.index.json"), `{"test.required":["2001","Required"],"test.order":["2002","Ordering friend"],"test.soft":["2003","Optional friend"]}`)
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.child")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		if p.Code == "def.parent-missing" {
			return e, ps, p, provider
		}
	}
	t.Fatal("no missing parent")
	return nil, nil, nil, ""
}

func TestParentFixLookupAndFocusedCLI(t *testing.T) {
	e, ps, p, provider := parentFixture(t)
	// Cached lookup must work even when Defs are unavailable; no XML scan is allowed.
	if err := os.Remove(filepath.Join(provider, "Defs", "Base.xml")); err != nil {
		t.Fatal(err)
	}
	h, err := e.App.ParentHelp(ps, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Parts) != 3 || h.CacheIncomplete || h.Quiet {
		t.Fatalf("help %+v", h)
	}
	fixes := map[string]app.ParentFix{}
	for _, f := range h.Fixes {
		fixes[f.Kind+":"+f.Name] = f
	}
	for _, key := range []string{"enable:Provider", "subscribe:Required", "subscribe:Ordering friend", "subscribe:Optional friend", "subscribe:Direct", "search:test.unknown", "requirements:Child"} {
		if _, ok := fixes[key]; !ok {
			t.Errorf("missing %s in %+v", key, h.Fixes)
		}
	}
	if f := fixes["requirements:Child"]; !strings.HasSuffix(f.URL, "id=1001") {
		t.Fatal(f)
	}
	for _, f := range h.Fixes {
		if f.Name == "Decoy" || f.Name == "test.wrongversion" {
			t.Fatalf("false provider/candidate: %+v", f)
		}
	}
	out := e.MustRun("problems", "--ref", p.Ref)
	if out != e.MustRun("problems", "show", p.Ref) {
		t.Fatal("--ref differs from show")
	}
	for _, want := range []string{"3 added things cannot load", "fix: archo enable", "16%", "4.1%", "provider unconfirmed", "archo subscribe workshop 2001", "HediffDef Illness", "PawnKindDef Beast_A", "id=1001"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"ParentName=", "lineage:", "manifest:", "files:", "seen:", "archo changes", "confidence:", "subjects:"} {
		if strings.Contains(out, bad) {
			t.Errorf("dump field %q:\n%s", bad, out)
		}
	}
	t.Log(out)
	ps.Analysis.Fresh = false
	h, err = e.App.ParentHelp(ps, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range h.Fixes {
		if f.Kind == "enable" {
			t.Fatal("stale provider offered")
		}
	}
	if !h.CacheIncomplete {
		t.Fatal("stale analysis not explained")
	}
}

func TestParentQuietLogAndGroupedIgnore(t *testing.T) {
	e, ps, p, _ := parentFixture(t)
	run := &app.LogRun{Label: "Player.log"}
	check := func(want bool) {
		t.Helper()
		h, err := e.App.ParentHelp(ps, p)
		if err != nil {
			t.Fatal(err)
		}
		if h.Quiet != want {
			t.Fatalf("quiet=%v want=%v", h.Quiet, want)
		}
	}
	check(false)
	ps.Logs = []*app.LogRun{run}
	check(true)
	run.Pending = true
	check(false)
	run.Pending = false
	run.Unknown = true
	check(false)
	run.Unknown = false
	run.Changed = true
	check(false)
	run.Changed = false
	problem := &app.Problem{Finding: &finding.Finding{Code: "log.exception", Severity: finding.Error, Subjects: []string{finding.Inst(p.SubjectInstance())}}, Log: run}
	ps.Ignored = append(ps.Ignored, problem)
	check(false)
	ps.Ignored = nil
	view.SetLang("zh")
	defer view.SetLang("en")
	panel, err := view.BuildProblemPanel(e.App, ps, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(panel.Title, "3 项内容加载不了") || !strings.Contains(panel.Meaning, "2 项角色种类") {
		t.Fatalf("panel %+v", panel)
	}
	var ignore *view.Action
	for i, act := range panel.Actions {
		if act.Kind == view.ActIgnore {
			if ignore != nil {
				t.Fatal("duplicate ignore buttons")
			}
			ignore = &panel.Actions[i]
		}
	}
	if ignore == nil || len(ignore.Refs) != 3 {
		t.Fatalf("ignore %+v", ignore)
	}
	if _, err := view.Do(e.App, *ignore, 0); err != nil {
		t.Fatal(err)
	}
	after, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, q := range after.Ignored {
		if q.Code == p.Code {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("ignored %d", n)
	}
	path := filepath.Join(e.Workshop, "1001", "Defs", "Child.xml")
	xml, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(path, strings.Replace(string(xml), "</Defs>", `<ThingDef><defName>NewContent</defName></ThingDef></Defs>`, 1))
	e.MustRun("scan")
	updated, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	expired := 0
	for _, q := range updated.List {
		if q.Code == p.Code && q.IgnoreExpired {
			expired++
		}
	}
	if expired != 3 {
		t.Fatalf("expired ignores %d", expired)
	}

}

func TestParentProviderCacheAndConditionalEligibility(t *testing.T) {
	e, ps, p, _ := parentFixture(t)
	provider, err := ps.State.Ix.ResolveOne("pkg:test.provider")
	if err != nil {
		t.Fatal(err)
	}
	// A plan can be fresh while another plan has replaced an instance's content facts.
	if _, err := e.App.Store.DB.Exec(`UPDATE content_state SET content_key='other-plan' WHERE instance_id=?`, provider.ID); err != nil {
		t.Fatal(err)
	}
	h, err := e.App.ParentHelp(ps, p)
	if err != nil {
		t.Fatal(err)
	}
	if !h.CacheIncomplete {
		t.Fatal("cross-plan content key not checked")
	}
	for _, fix := range h.Fixes {
		if fix.Kind == "enable" {
			t.Fatal("wrong cached content offered")
		}
	}
}

func TestParentMayRequireSelfAndFolderSwitch(t *testing.T) {
	for _, folderSwitch := range []bool{false, true} {
		t.Run(fmt.Sprint(folderSwitch), func(t *testing.T) {
			e := testkit.New(t)
			e.AddWorkshop("1001", testkit.Mod{PackageID: "test.child", Name: "Child", Files: map[string]string{"Defs/Child.xml": `<Defs><HediffDef ParentName="Base"><defName>Illness</defName></HediffDef></Defs>`}})
			files := map[string]string{"Defs/Base.xml": `<Defs><HediffDef Name="Base" Abstract="True" MayRequire="test.provider" /></Defs>`}
			if folderSwitch {
				files["LoadFolders.xml"] = `<loadFolders><v1.6><li IfModNotActive="test.provider">/</li></v1.6></loadFolders>`
			}
			e.AddWorkshop("1002", testkit.Mod{PackageID: "test.provider", Name: "Provider", Files: files})
			e.WriteModsConfig("1.6", "ludeon.rimworld", "test.child")
			e.AddEnv(false)
			e.MustRun("scan")
			e.MustRun("analyze")
			ps, err := e.App.Problems(false, nil)
			if err != nil {
				t.Fatal(err)
			}
			var p *app.Problem
			for _, q := range ps.List {
				if q.Code == "def.parent-missing" {
					p = q
					break
				}
			}
			if p == nil {
				t.Fatal("no missing parent")
			}
			h, err := e.App.ParentHelp(ps, p)
			if err != nil {
				t.Fatal(err)
			}
			enabled := false
			for _, fix := range h.Fixes {
				enabled = enabled || fix.Kind == "enable"
			}
			if enabled == folderSwitch {
				t.Fatalf("folder switch %v: %+v", folderSwitch, h)
			}
		})
	}
}

// The provider is installed after analysis, so it has no Def facts at all.
func neverAnalysedChimera(t testing.TB, extras int) (*testkit.Env, *app.ProblemSet, *app.Problem) {
	t.Helper()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.chimera", Name: "Chimera Creation (Continued)", Files: map[string]string{
		"Defs/Chimera.xml": `<Defs>
 <HediffDef ParentName="MeatHunger_Original"><defName>MeatHungerChimera</defName></HediffDef>
 <PawnKindDef ParentName="ChimeraKind"><defName>ChimeraTame</defName></PawnKindDef>
 <ThingDef ParentName="ChimeraThing"><defName>Chimera</defName></ThingDef>
 </Defs>`,
	}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.chimera")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.chimerabase", Name: "Chimera Base", Files: map[string]string{"Defs/Base.xml": `<Defs><PawnKindDef Name="ChimeraKind" Abstract="True" /></Defs>`}})
	for i := 0; i < extras; i++ {
		files := map[string]string{}
		for j := 0; j < 3; j++ {
			var xml strings.Builder
			xml.WriteString("<Defs>")
			for k := 0; k < 100; k++ {
				fmt.Fprintf(&xml, `<ThingDef Name="Other_%d_%d_%d" Abstract="True" />`, i, j, k)
			}
			xml.WriteString("</Defs>")
			files[fmt.Sprintf("Defs/Things%d.xml", j)] = xml.String()
		}
		e.AddWorkshop(fmt.Sprint(2000+i), testkit.Mod{PackageID: fmt.Sprintf("test.other%d", i), Name: fmt.Sprintf("Other %03d", i), Files: files})
	}
	e.MustRun("scan")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps.List {
		if p.Code == "def.parent-missing" {
			return e, ps, p
		}
	}
	t.Fatal("no missing parent")
	return nil, nil, nil
}

func TestNeverAnalysedParentProvider(t *testing.T) {
	e, ps, p := neverAnalysedChimera(t, 0)
	defs, err := e.App.Store.DefsNamed("ChimeraKind", true)
	if err != nil || len(defs) != 0 {
		t.Fatalf("provider already analysed: %+v %v", defs, err)
	}
	view.SetLang("zh")
	defer view.SetLang("en")
	initial, err := view.BuildProblemPanel(e.App, ps, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Searching || len(initial.Evidence) != 0 || len(initial.Locations) != 3 {
		t.Fatalf("initial %+v", initial)
	}
	needs := view.BuildNeeds(e.App, ps.State, ps)
	for _, n := range needs.Items {
		if strings.Contains(n.Title, "3 项内容加载不了") {
			if n.Why != "缺少它们所基于的内容；分享的日志里这类 Mod 有 16% 出错（随便挑一个是 4.1%）" || n.Evidence != nil {
				t.Fatalf("row %+v", n)
			}
		}
	}
	panel, err := view.BuildProblemPanelContext(context.Background(), e.App, ps, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if panel.Searching || !panel.Primary || panel.Actions[0].Kind != view.ActEnable || panel.Actions[0].Label != "启用 Chimera Base" {
		t.Fatalf("panel %+v", panel)
	}
	if len(panel.Affected) != 3 || len(panel.Locations) != 3 {
		t.Fatal(panel)
	}
	for _, loc := range panel.Locations {
		if !strings.HasPrefix(loc, "Defs/Chimera.xml:") {
			t.Fatal(loc)
		}
	}
	for _, note := range panel.Notes {
		if strings.Contains(note, "候选") {
			t.Fatal(note)
		}
	}
	defs, err = e.App.Store.DefsNamed("ChimeraKind", true)
	if err != nil || len(defs) != 0 {
		t.Fatal("on-demand lookup wrote analysis", err)
	}
	out := e.MustRun("problems", "--ref", p.Ref)
	for _, want := range []string{"fix: archo enable", "Chimera Base defines ChimeraKind", "Defs/Chimera.xml:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "evidence:") {
		t.Fatal(out)
	}
	t.Log(out)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.App.FindParentHelp(ctx, ps, p); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func BenchmarkNeverAnalysedParentProviders400(b *testing.B) {
	e, ps, p := neverAnalysedChimera(b, 399)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h, err := e.App.FindParentHelp(context.Background(), ps, p)
		if err != nil || h.SearchIncomplete {
			b.Fatalf("lookup: %+v %v", h, err)
		}
		found := false
		for _, f := range h.Fixes {
			if f.Kind == "enable" && f.Name == "Chimera Base" {
				found = true
			}
		}
		if !found {
			b.Fatal("provider missing")
		}
	}
}

func TestParentSubscribeIsPrimaryWithoutProvider(t *testing.T) {
	e, ps, p, provider := parentFixture(t)
	if err := os.Remove(filepath.Join(provider, "Defs", "Base.xml")); err != nil {
		t.Fatal(err)
	}
	ps.Analysis.Fresh = false
	panel, err := view.BuildProblemPanelContext(context.Background(), e.App, ps, p.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !panel.Primary || panel.Actions[0].Kind != view.ActSubscribe || !strings.HasSuffix(panel.Actions[len(panel.Actions)-1].URL, "id=1001") {
		t.Fatalf("panel %+v", panel)
	}
	candidates := false
	for _, note := range panel.Notes {
		if strings.Contains(note, "optional companions") {
			candidates = true
		}
	}
	if !candidates {
		t.Fatal("missing candidate caveat", panel.Notes)
	}
}
