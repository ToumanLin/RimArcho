package gui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestIdentityQuestionActionsAndWhy(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	dir := filepath.Join(e.Game, "Mods", "Old")
	e.WriteMod(dir, testkit.Sample("tester.old", "Shared payload"))
	e.MustRun("scan")
	e.Remove(dir)
	e.MustRun("scan")
	dir = filepath.Join(e.Game, "Mods", "New")
	e.WriteMod(dir, testkit.Sample("tester.new", "Shared payload"))
	e.MustRun("scan")
	api := NewAPI(e.App, func(string, any) {})
	t.Cleanup(api.Close)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	needs := view.BuildNeeds(e.App, st, &app.ProblemSet{})
	var actions []view.Action
	for _, it := range needs.Items {
		if it.Group == view.GroupConfirm {
			if it.Evidence != nil || !strings.Contains(it.Why, "3") {
				t.Fatalf("generic question row: %+v", it)
			}
			actions = it.Actions
			break
		}
	}
	if len(actions) != 4 {
		t.Fatalf("actions=%+v", actions)
	}
	for n, answer := range []string{"same", "copy", "unrelated"} {
		a := actions[n]
		if a.Kind != view.ActAnswer || a.Answer != answer || a.Title == "" || a.Confirm == "" || !strings.Contains(a.Warn, a.Title) {
			t.Fatalf("answer=%+v", a)
		}
	}
	if a := actions[3]; a.Kind != view.ActView || a.Key != "v" || a.Confirm != "" || strings.Join(a.Argv[:2], " ") != "identity why" {
		t.Fatalf("why=%+v", a)
	}
	qid := actions[0].QID
	before, _ := e.App.Store.QuestionByID(qid)
	// The comparison uses the database even if the mod folder cannot be read now.
	e.Remove(dir)
	got, err := api.Inspect(actions[3].Argv)
	if err != nil {
		t.Fatal(err)
	}
	want, err := view.IdentityReasons(e.App, qid)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v err=%v", got, want, err)
	}
	panel, err := api.IdentityWhy(qid)
	if err != nil {
		t.Fatal(err)
	}
	if panel.Headline != view.Tf("%s; differences: %s", view.Tf("%d content files are identical", 3), view.T("package ID")) || len(panel.Answers) != 3 {
		t.Fatalf("panel=%+v", panel)
	}
	for _, row := range panel.Rows {
		if row.Label == view.T("Workshop ID") {
			t.Fatal("both-missing Workshop row shown")
		}
		if row.NewPath != "" && row.New != filepath.Base(row.NewPath) {
			t.Fatal("full folder path shown in comparison")
		}
	}
	after, _ := e.App.Store.QuestionByID(qid)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("why answered the question")
	}
	out := e.MustRun("identity", "why", fmt.Sprintf("q:%d", qid))
	if !strings.Contains(out, "package ID: tester.new / tester.old") || !strings.Contains(out, "3 content files are identical") {
		t.Fatal(out)
	}
	if _, err := api.Inspect([]string{"identity", "resolve", fmt.Sprintf("q:%d", qid), "same"}); err == nil {
		t.Fatal("Inspect admitted an identity write")
	}
}

func TestIdentityComparisonPruningAndStoredHeadline(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	files := map[string]string{}
	for n := 0; n < 7; n++ {
		files[fmt.Sprintf("Defs/%d.xml", n)] = fmt.Sprintf("<Defs><ThingDef><defName>Q%d</defName></ThingDef></Defs>", n)
	}
	old := filepath.Join(e.Game, "Mods", "qmod-a")
	e.WriteMod(old, testkit.Mod{PackageID: "tester.qa", Name: "Q Mod A", Author: "A", Files: files})
	e.MustRun("scan")
	e.Remove(old)
	e.WriteMod(filepath.Join(e.Game, "Mods", "qmod-b"), testkit.Mod{PackageID: "tester.qb", Name: "Q Mod B", Author: "B", Files: files})
	e.MustRun("scan")
	env, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	qs, err := e.App.Store.Questions(env.ID, false)
	if err != nil || len(qs) != 1 {
		t.Fatalf("questions=%+v err=%v", qs, err)
	}
	api := NewAPI(e.App, func(string, any) {})
	t.Cleanup(api.Close)
	panel, err := api.IdentityWhy(qs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(panel.Files) != 5 || panel.More != 2 || len(panel.Rows) != 4 {
		t.Fatalf("panel=%+v", panel)
	}
	if panel.Rows[0].Match || panel.Rows[1].Match || panel.Rows[2].Match {
		t.Fatal("metadata differences hidden")
	}
	out := e.MustRun("identity", "why", fmt.Sprintf("q:%d", qs[0].ID))
	if strings.Count(out, "matching file:") != 5 || !strings.Contains(out, "and 2 more files") {
		t.Fatal(out)
	}
	for _, noise := range []string{"Workshop ID", "not recorded", "shared paths", "checked files", "About files", "recorded files and declarations", e.Game} {
		if strings.Contains(out, noise) {
			t.Fatalf("comparison contains noise %q: %s", noise, out)
		}
	}
	// A page can render the full reason even when the file-record table is unavailable.
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Store.DB.Exec(`ALTER TABLE snapshot_file RENAME TO comparison_files_unavailable`); err != nil {
		t.Fatal(err)
	}
	needs := view.BuildNeeds(e.App, st, &app.ProblemSet{})
	found := false
	for _, it := range needs.Items {
		if it.Group == view.GroupConfirm {
			found = true
			if it.Why != panel.Headline || it.Evidence != nil {
				t.Fatalf("row lost stored reason: %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("question row read file records or disappeared")
	}
}
