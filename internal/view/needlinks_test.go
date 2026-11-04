package view

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func linkFixture(t testing.TB) (*testkit.Env, *app.LoadoutState) {
	t.Helper()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	for i, name := range []string{"TakeCover", "Achtung!", "同名", "同名", "Framework", "Mods", "Load"} {
		e.AddWorkshop(fmt.Sprint(1001+i), testkit.Sample(fmt.Sprintf("tester.m%d", i), name))
	}
	e.WriteModsConfig("1.6", "ludeon.rimworld", "brrainz.harmony", "tester.m0", "tester.m1")
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	return e, st
}

func flatParts(ps []NeedPart) string {
	var s strings.Builder
	for _, p := range ps {
		s.WriteString(p.Text)
	}
	return s.String()
}

func TestNeedLinksAcrossSurfaces(t *testing.T) {
	SetLang("zh")
	t.Cleanup(func() { SetLang("en") })
	e, st := linkFixture(t)
	a, _ := st.Ix.ResolveOne("pkg:tester.m0")
	b, _ := st.Ix.ResolveOne("pkg:tester.m1")
	ps := &app.ProblemSet{List: []*app.Problem{{Finding: &finding.Finding{Code: "incompat.active", Severity: finding.Error,
		Subjects: []string{finding.Inst(a.ID), finding.Inst(b.ID)}}}}}
	n := BuildNeeds(e.App, st, ps)
	var item *Need
	for i := range n.Items {
		if n.Items[i].code == "incompat.active" {
			item = &n.Items[i]
		}
	}
	if item == nil || item.Content == nil {
		t.Fatalf("missing content: %+v", n.Items)
	}
	parts := item.Content.Text
	if flatParts(parts) != item.Text || parts[0].ID != a.ID || parts[2].ID != b.ID {
		t.Fatalf("both mod names must navigate independently: %+v", parts)
	}
	r := n.Reasons[a.ID][0]
	if flatParts(r.Content.Text) != r.Text || r.Content.Text[2].ID != b.ID {
		t.Fatalf("badge lost second name: %+v", r)
	}
	encoded, err := json.Marshal(ProblemsOf(n))
	if err != nil || !strings.Contains(string(encoded), `"content"`) || !strings.Contains(string(encoded), b.ID) {
		t.Fatalf("editor serialization: %s %v", encoded, err)
	}
	// Identical display names still point to their respective copies, in question order.
	x, _ := st.Ix.ResolveOne("pkg:tester.m2")
	y, _ := st.Ix.ResolveOne("pkg:tester.m3")
	q := questionNeed(st, &store.Question{InstanceID: y.ID, CandidateID: x.ID})
	names := newNeedNames(e.App, st, nil)
	names.fill(&q)
	if q.Content.Text[0].ID != y.ID || q.Content.Text[2].ID != x.ID || flatParts(q.Content.Text) != q.Text {
		t.Fatalf("identity question links: %+v", q.Content.Text)
	}
}

func TestMissingAndFoldedNameLinks(t *testing.T) {
	t.Parallel()
	e, st := linkFixture(t)
	ps := &app.ProblemSet{List: []*app.Problem{{Finding: &finding.Finding{Code: "dep.missing",
		Subjects: []string{"pkg:tester.absent"}, Claim: "requires Absent Framework (pkg:tester.absent)",
		Suggestions: []string{"subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=123456"}}}}}
	r := newNeedNames(e.App, st, nil)
	a, _ := st.Ix.ResolveOne("pkg:tester.m0")
	b, _ := st.Ix.ResolveOne("pkg:tester.m1")
	n := Need{Title: "missing: Absent Framework", Why: "TakeCover needs Absent Framework",
		Parts: []string{"TakeCover should load after Achtung!", "tester.unknown"}, Fold: true,
		Mods: []string{a.ID, b.ID}, linkProblems: ps.List}
	r.fill(&n)
	last := n.Content.Title[len(n.Content.Title)-1]
	if last.Text != "Absent Framework" || last.ID != "" || !strings.HasSuffix(last.URL, "=123456") {
		t.Fatalf("uninstalled dependency: %+v", last)
	}
	if len(n.Content.Parts[0]) != 3 || n.Content.Parts[0][0].ID == "" || n.Content.Parts[0][2].ID == "" || n.Content.Parts[1] != nil {
		t.Fatalf("folded or unknown links: %+v", n.Content.Parts)
	}
	// Never match a name inside another English word or interpret markup as HTML.
	trie := &needNames{}
	trie.add(NeedPart{Text: "Framework", ID: "short"})
	trie.add(NeedPart{Text: "<b>Mod</b>", ID: "markup"})
	if trie.parts("Frameworks", nil) != nil || flatParts(trie.parts("<b>Mod</b>", nil)) != "<b>Mod</b>" {
		t.Fatal("name boundaries or literal text lost")
	}
	trie.add(NeedPart{Text: "Framework Extended", ID: "longer"})
	if p := trie.parts("Framework Extended", nil); len(p) != 1 || p[0].ID != "longer" {
		t.Fatalf("short name swallowed longer name: %+v", p)
	}
	trie.add(NeedPart{Text: "TakeCover", ID: a.ID})
	trie.addFor(NeedPart{Text: "TakeCover", URL: "https://steamcommunity.com/sharedfiles/filedetails/?id=9999"}, "removed")
	if p := trie.parts("TakeCover", map[string]int{"removed": 1}); len(p) != 1 || p[0].ID != "" || p[0].URL == "" {
		t.Fatalf("removed copy linked to another installed copy: %+v", p)
	}
	trie.addFor(NeedPart{Text: "TakeCover"}, "unknown-removed")
	if p := trie.parts("TakeCover", map[string]int{"unknown-removed": 1}); len(p) != 1 || p[0].ID != "" || p[0].URL != "" {
		t.Fatalf("unknown removed copy must stay plain text: %+v", p)
	}
	// A title with no names keeps its title even when the full sentence contains names.
	n = Need{Title: "missing dependency", Text: "TakeCover needs Achtung!", Mods: []string{a.ID, b.ID}}
	r.fill(&n)
	if reasonContent(n).Text != nil {
		t.Fatal("badge replaced title with full sentence")
	}
}

func TestNeedLinksLimitedToItem(t *testing.T) {
	t.Parallel()
	e, st := linkFixture(t)
	a, _ := st.Ix.ResolveOne("pkg:tester.m0")
	b, _ := st.Ix.ResolveOne("pkg:tester.m1")
	index := newNeedNames(e.App, st, nil)
	for label, n := range map[string]Need{
		"row owner":   {Mods: []string{a.ID}},
		"required by": {RequiredBy: []ModOwner{{ID: a.ID}}},
		"action ref":  {Actions: []Action{{Refs: []string{st.Ix.Ref(a.ID)}}}},
		"subject":     {linkProblems: []*app.Problem{{Finding: &finding.Finding{Subjects: []string{finding.Inst(a.ID)}}}}},
	} {
		t.Run(label, func(t *testing.T) {
			n.Title = "Load Mods: TakeCover, Achtung!, Harmony, Framework"
			n.Text, n.Why = n.Title, n.Title
			n.Parts = []string{n.Title}
			index.fill(&n)
			for _, parts := range [][]NeedPart{n.Content.Title, n.Content.Text, n.Content.Why, n.Content.Parts[0]} {
				if flatParts(parts) != n.Title {
					t.Fatalf("text changed: %+v", parts)
				}
				links := 0
				for _, p := range parts {
					if p.ID != "" || p.URL != "" {
						links++
						if p.ID != a.ID || p.Text != "TakeCover" {
							t.Fatalf("unrelated library mod linked: %+v", p)
						}
					}
				}
				if links != 1 {
					t.Fatalf("wanted only the item's own mod: %+v", parts)
				}
			}
		})
	}
	// Order folds keep the target even though only the mod to move gets a row badge.
	n := Need{Mods: []string{a.ID}, Text: "TakeCover after Achtung!", Actions: []Action{{After: st.Ix.Ref(b.ID)}}}
	index.fill(&n)
	if len(n.Content.Text) != 3 || n.Content.Text[2].ID != b.ID {
		t.Fatalf("action target lost: %+v", n.Content)
	}
	// A package target is enough; no owner or action has to repeat the target instance.
	n = Need{Target: "tester.m1", Text: "Achtung!"}
	index.fill(&n)
	if n.Content.Text[0].ID != b.ID {
		t.Fatalf("dependency target lost: %+v", n.Content)
	}
}

func TestNeedLinksKeepEachProblemsMissingTarget(t *testing.T) {
	t.Parallel()
	e, st := linkFixture(t)
	a, _ := st.Ix.ResolveOne("pkg:tester.m0")
	ps := &app.ProblemSet{}
	for i, name := range []string{"Missing Framework", "Other Library"} {
		pkg := fmt.Sprintf("tester.absent%d", i)
		ps.List = append(ps.List, &app.Problem{Finding: &finding.Finding{Code: "dep.missing", Severity: finding.Warning,
			Subjects:    []string{finding.Inst(a.ID), "pkg:" + pkg},
			Claim:       fmt.Sprintf("requires %s (pkg:%s)", name, pkg),
			Suggestions: []string{fmt.Sprintf("subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=%d", 123456+i)}}})
	}
	n := BuildNeeds(e.App, st, ps)
	if len(n.Items) != 3 || n.Items[2].Kind != "memory.capacity" {
		t.Fatalf("expected two dependencies: %+v", n.Items)
	}
	for _, it := range n.Items[:2] {
		want := "https://steamcommunity.com/sharedfiles/filedetails/?id=123456"
		if strings.HasSuffix(it.Target, "1") {
			want = "https://steamcommunity.com/sharedfiles/filedetails/?id=123457"
		}
		links := 0
		for _, p := range it.Content.Title {
			if p.URL != "" {
				links++
				if p.ID != "" || p.URL != want {
					t.Fatalf("another item's target leaked into %s: %+v", it.Title, p)
				}
			}
		}
		if links != 1 {
			t.Fatalf("Workshop target lost behind plain package alias: %+v", it.Content.Title)
		}
	}
}

func TestCorpusAndDeclaredDependenciesStaySeparate(t *testing.T) {
	t.Parallel()
	e, st := linkFixture(t)
	a, _ := st.Ix.ResolveOne("pkg:tester.m0")
	b, _ := st.Ix.ResolveOne("pkg:tester.m1")
	f, _ := st.Ix.ResolveOne("pkg:tester.m4")
	ps := &app.ProblemSet{}
	for i, code := range []string{"dep.observed-missing", "dep.inactive"} {
		ps.List = append(ps.List, &app.Problem{Finding: &finding.Finding{Code: code, Severity: finding.Warning,
			Subjects: []string{finding.Inst([]string{a.ID, b.ID}[i]), finding.Inst(f.ID)}}})
	}
	n := BuildNeeds(e.App, st, ps)
	if len(n.Items) != 3 || n.Items[2].Kind != "memory.capacity" || n.Count != 1 || n.Status[a.ID] != StatusHint || n.Status[b.ID] != StatusWarn {
		t.Fatalf("hint merged with author requirement: %+v %+v", n.Items, n.Status)
	}
	if n.Items[0].Status != StatusWarn || n.Items[0].ID == n.Items[1].ID {
		t.Fatalf("hint displaced or shares identity with warning: %+v", n.Items)
	}
	ps.List[1].Code = "dep.observed-missing"
	merged := BuildNeeds(e.App, st, ps)
	if len(merged.Items) != 2 || merged.Items[1].Kind != "memory.capacity" || merged.Count != 0 || !strings.Contains(merged.Items[0].Why, "may be needed") {
		t.Fatalf("merged hints must keep uncertainty: %+v", merged.Items)
	}
	if flatParts(merged.Items[0].Content.Why) != merged.Items[0].Why {
		t.Fatal("merged hint owners lost their links")
	}
	// A real warning wins on a row carrying both warning and hint, in either order.
	for _, reverse := range []bool{false, true} {
		its := []Need{{Counted: false, Status: StatusHint, Mods: []string{a.ID}}, {Counted: true, Status: StatusWarn, Mods: []string{a.ID}}}
		if reverse {
			its[0], its[1] = its[1], its[0]
		}
		mixed := &Needs{Items: its}
		indexNeeds(mixed)
		if mixed.Status[a.ID] != StatusWarn || mixed.Count != 1 || mixed.Reasons[a.ID][0].Status != StatusWarn {
			t.Fatalf("hint hid warning: %+v", mixed)
		}
	}
}

func TestCorpusCutoffComesFromEmbeddedData(t *testing.T) {
	// Not parallel: known.SetForTesting replaces the process-wide corpus.
	if known.Get().Cutoff == "" {
		t.Fatal("embedded corpus must carry its cutoff")
	}
	defer known.SetForTesting(&known.Data{Cutoff: "2038-11"})()
	if e := evidence(EvCorpus, ""); e.Cutoff != "2038-11" || !strings.Contains(e.Label, "2038-11") {
		t.Fatalf("cutoff not from loader: %+v", e)
	}
}
