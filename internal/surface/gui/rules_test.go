package gui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The editor's 排序规则 (06 §2.5): list, add, change (r:<n> stays), remove and restore.
func TestRulesFold(t *testing.T) {
	view.SetLang("en")
	e, s, _ := fixture(t)
	rules, err := s.Rules()
	if err != nil || len(rules.Rows) != 0 || len(rules.Kinds) != 6 {
		t.Fatalf("empty rules %+v %v", rules, err)
	}
	text, err := s.SaveRule(0, RuleSpec{Kind: "after", A: "pkg:tester.a", B: "pkg:brrainz.harmony", Note: " needs it "})
	if err != nil || text != "rule added: Alpha loads after Harmony" {
		t.Fatalf("add %q %v", text, err)
	}
	if _, err := s.SaveRule(0, RuleSpec{Kind: "after", A: "pkg:tester.a"}); err == nil {
		t.Fatal("a rule without its second mod was added")
	}
	rules, _ = s.Rules()
	if len(rules.Rows) != 1 {
		t.Fatalf("rows %+v", rules.Rows)
	}
	r := rules.Rows[0]
	if r.ID != 1 || r.Text != "Alpha loads after Harmony" || r.Note != "needs it" || r.A == nil || r.A.Name != "Alpha" || r.B == nil || r.B.Name != "Harmony" || r.A.Pos == 0 {
		t.Fatalf("row %+v a %+v b %+v", r, r.A, r.B)
	}
	if len(r.Parts) != 3 || r.Parts[0].Mod != 1 || r.Parts[2].Mod != 2 {
		t.Fatalf("parts %+v", r.Parts)
	}
	// change: the kind and the note; the ID stays
	if text, err := s.SaveRule(r.ID, RuleSpec{Kind: "top", A: r.A.Ref, B: r.B.Ref, Note: ""}); err != nil || text != "rule changed: Alpha loads as early as it can" {
		t.Fatalf("change %q %v", text, err)
	}
	rules, _ = s.Rules()
	if got := rules.Rows[0]; got.ID != 1 || got.Kind != "top" || got.B != nil || got.Note != "" || got.Created != r.Created {
		t.Fatalf("changed %+v", got)
	}
	if out := e.MustRun("rule", "list"); !strings.Contains(out, "r:1  top") {
		t.Fatalf("cli list after change:\n%s", out)
	}
	// remove, then 撤销 puts it back with its ID
	if text, err := s.RemoveRule(1); err != nil || text != "rule removed: Alpha loads as early as it can" {
		t.Fatalf("remove %q %v", text, err)
	}
	if rules, _ = s.Rules(); len(rules.Rows) != 0 {
		t.Fatalf("not removed %+v", rules.Rows)
	}
	if _, err := s.RestoreRule(2); err == nil {
		t.Fatal("restored a rule that was not removed")
	}
	if text, err := s.RestoreRule(1); err != nil || !strings.HasPrefix(text, "rule restored: ") {
		t.Fatalf("restore %q %v", text, err)
	}
	if rules, _ = s.Rules(); len(rules.Rows) != 1 || rules.Rows[0].ID != 1 {
		t.Fatalf("restored %+v", rules.Rows)
	}
	// the next rule still gets a new number
	if _, err := s.SaveRule(0, RuleSpec{Kind: "bottom", A: "pkg:brrainz.harmony"}); err != nil {
		t.Fatal(err)
	}
	if rules, _ = s.Rules(); rules.Rows[0].ID != 2 {
		t.Fatalf("newest first, new number: %+v", rules.Rows)
	}
	// 只看当前列表: a rule shows there when one of its mods is enabled in the list
	if !rules.Rows[1].InList {
		t.Fatalf("Alpha is enabled, its rule is not in the list: %+v", rules.Rows[1])
	}
	beta, _ := s.RuleCandidates("beta", "")
	if len(beta) != 1 || beta[0].Pos != 0 {
		t.Fatalf("beta %+v", beta)
	}
	if _, err := s.SaveRule(0, RuleSpec{Kind: "top", A: beta[0].Ref}); err != nil {
		t.Fatal(err)
	}
	if rules, _ = s.Rules(); rules.Rows[0].InList {
		t.Fatalf("a rule about a mod not enabled counts as the list's: %+v", rules.Rows[0])
	}
}

// An ignore rule reads as the constraint it drops; only its note changes in the editor.
func TestRulesIgnoreRow(t *testing.T) {
	view.SetLang("en")
	e, s, _ := fixture(t)
	c, _ := s.RuleCandidates("alpha", "")
	e.MustRun("rule", "add", "ignore", "decl:"+c[0].Ref+":dependency:brrainz.harmony", "--note", "x")
	rules, err := s.Rules()
	if err != nil || len(rules.Rows) != 1 {
		t.Fatalf("%+v %v", rules, err)
	}
	r := rules.Rows[0]
	if r.Text != "ignore Alpha's About.xml: modDependencies Harmony" || r.A == nil || !strings.HasPrefix(r.Target, "decl:") {
		t.Fatalf("ignore row %+v", r)
	}
	if _, err := s.SaveRule(r.ID, RuleSpec{Kind: "ignore", Target: r.Target, Note: "changed"}); err != nil {
		t.Fatal(err)
	}
	if rules, _ = s.Rules(); rules.Rows[0].Note != "changed" || rules.Rows[0].Target != r.Target || rules.Rows[0].ID != r.ID {
		t.Fatalf("note change %+v", rules.Rows[0])
	}
}

// The second mod's candidates: by name, one per mod, never the rule's own mod.
func TestRuleCandidates(t *testing.T) {
	_, s, _ := fixture(t)
	c, err := s.RuleCandidates("a", "pkg:tester.a")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range c {
		if m.Name == "Alpha" {
			t.Fatalf("own mod offered: %+v", c)
		}
	}
	if c, _ := s.RuleCandidates("harm", ""); len(c) != 1 || c[0].Name != "Harmony" || !strings.HasPrefix(c[0].Ref, "L#") {
		t.Fatalf("harm %+v", c)
	}
	if c, _ := s.RuleCandidates("  ", ""); len(c) != 0 {
		t.Fatalf("empty text %+v", c)
	}
}

// archo rule edit keeps r:<n>.
func TestRuleEditCommand(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("1001", testkit.Sample("tester.a", "Alpha"))
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("rule", "add", "after", "pkg:tester.a", "pkg:brrainz.harmony")
	out := e.MustRun("rule", "edit", "r:1", "before", "pkg:tester.a", "pkg:brrainz.harmony", "--note", "n")
	if !strings.Contains(out, "r:1 changed") || !strings.Contains(out, "before") {
		t.Fatalf("edit:\n%s", out)
	}
	if out, code := e.Run("rule", "edit", "r:7", "top", "pkg:tester.a"); code == 0 || !strings.Contains(out, "no rule r:7") {
		t.Fatalf("edit of a missing rule: %d\n%s", code, out)
	}
	if out := e.MustRun("rule", "list"); !strings.Contains(out, "r:1  before") || strings.Contains(out, "r:2") {
		t.Fatalf("list:\n%s", out)
	}
}

// 搜索文件 (06 §2.9): results by mod and file, pages, and a superseded search is dropped.
func TestFileSearchAPI(t *testing.T) {
	view.SetLang("en")
	_, s, _ := fixture(t)
	if scope, err := s.FileSearchInfo(); err != nil || !strings.Contains(scope, "mods") {
		t.Fatalf("scope %q %v", scope, err)
	}
	n := s.BeginFileSearch()
	p, err := s.FileSearch(view.FileSearchReq{Pattern: "alpha_thing"}, n)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Mods) != 1 || p.Mods[0].Name != "Alpha" || len(p.Mods[0].Files) != 1 || p.Mods[0].Files[0].Lines[0].Line != 1 || !strings.HasSuffix(p.Mods[0].Files[0].Path, "alpha.xml") || p.More {
		t.Fatalf("page %+v", p)
	}
	// Beta is not enabled: named, it is searched
	if p, _ := s.FileSearch(view.FileSearchReq{Pattern: "beta_thing"}, n); len(p.Mods) != 0 || p.Hint == "" {
		t.Fatalf("not enabled searched %+v", p)
	}
	beta := ""
	if c, _ := s.ModCandidates("beta"); len(c) == 1 {
		beta = c[0].ID
	}
	if p, _ := s.FileSearch(view.FileSearchReq{Pattern: "beta_thing", Mods: []string{beta}}, n); len(p.Mods) != 1 {
		t.Fatalf("named mod %q not searched %+v", beta, p)
	}
	if _, err := s.FileSearch(view.FileSearchReq{Pattern: "  "}, n); err == nil {
		t.Fatal("empty pattern searched")
	}
	// a newer search supersedes this request
	s.BeginFileSearch()
	if _, err := s.FileSearch(view.FileSearchReq{Pattern: "alpha"}, n); !errors.Is(err, context.Canceled) {
		t.Fatalf("old request answered: %v", err)
	}
	// opening a result's folder: only inside the mod
	if err := s.OpenSearchFolder(p.Mods[0].ID, `C:\Windows\win.ini`); err == nil {
		t.Fatal("opened a folder outside the mod")
	}
}
