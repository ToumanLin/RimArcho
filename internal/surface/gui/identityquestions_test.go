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
	after, _ := e.App.Store.QuestionByID(qid)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("why answered the question")
	}
	out := e.MustRun("identity", "why", fmt.Sprintf("q:%d", qid))
	if !strings.Contains(out, "package ID differs: tester.new / tester.old") || !strings.Contains(out, "3 of 3 / 3") {
		t.Fatal(out)
	}
	if _, err := api.Inspect([]string{"identity", "resolve", fmt.Sprintf("q:%d", qid), "same"}); err == nil {
		t.Fatal("Inspect admitted an identity write")
	}
}
