package app_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The old R6 selected the same vanished mod for all 67 unrelated folders because
// max(path Jaccard, content Jaccard) treats common About paths as identity evidence.
func TestIdentityQuestionsCommonPathsFanOut(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	oldDir := filepath.Join(e.Game, "Mods", "Old")
	files := map[string]string{"About/Preview.png": "shared preview", "Textures/Shared.png": "shared texture"}
	e.WriteMod(oldDir, testkit.Mod{PackageID: "tester.old", Name: "Old", Files: files})
	scanNow(t, e)
	old := one(t, e, "Old")
	e.Remove(oldDir)
	scanNow(t, e)
	for n := 0; n < 67; n++ {
		name := fmt.Sprintf("Unrelated %02d", n)
		e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Mod{PackageID: fmt.Sprintf("tester.other%d", n), Name: name, Files: files})
	}
	scanNow(t, e)
	oldFiles, err := e.App.Store.SemanticSet(old.SnapID)
	if err != nil {
		t.Fatal(err)
	}
	var observations []*scan.Observation
	for n := 0; n < 67; n++ {
		m := one(t, e, fmt.Sprintf("Unrelated %02d", n))
		cur, err := e.App.Store.SemanticSet(m.SnapID)
		if err != nil {
			t.Fatal(err)
		}
		if scan.Similarity(oldFiles, cur) < identity.QuestionSimilarity {
			t.Fatal("fixture no longer reproduces legacy path match")
		}
		if m.LineageID == old.LineageID {
			t.Fatal("unrelated mod inferred as a copy")
		}
		observations = append(observations, &scan.Observation{Loc: scan.Location{SourceID: m.SourceID, Kind: m.Kind, Rel: m.Rel}, Decl: m.Decl(), FP: scan.Fingerprint{Semantic: cur}})
	}
	next := 0
	result := identity.Reconcile(identity.Input{Obs: observations, Known: []*identity.Known{{ID: old.ID, LineageID: old.LineageID, SourceID: old.SourceID, Status: identity.Tombstone, Snap: &identity.Snap{Decl: old.Decl()}}},
		NewID: func() string { next++; return fmt.Sprintf("synthetic-%d", next) }, Semantic: func(*identity.Known) scan.SemanticSet { return oldFiles }})
	if len(result.Questions) != 0 {
		t.Fatalf("matcher raised %d questions about one unrelated mod", len(result.Questions))
	}

	if qs, err := e.App.Store.Questions(mustEnv(t, e), false); err != nil || len(qs) != 0 {
		t.Fatalf("questions=%+v err=%v", qs, err)
	}
}

func TestIdentityQuestionsAmbiguousContent(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			t.Parallel()
			e := setup(t, false)
			oldCount, newCount := 1, 67
			if reverse {
				oldCount, newCount = 67, 1
			}
			files := testkit.Sample("tester.template", "Shared payload").Files
			for n := 0; n < oldCount; n++ {
				name := fmt.Sprintf("Old %02d", n)
				e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Mod{PackageID: fmt.Sprintf("tester.old%d", n), Name: name, Files: files})
			}
			scanNow(t, e)
			for n := 0; n < oldCount; n++ {
				e.Remove(filepath.Join(e.Game, "Mods", fmt.Sprintf("Old %02d", n)))
			}
			scanNow(t, e)
			for n := 0; n < newCount; n++ {
				name := fmt.Sprintf("New %02d", n)
				e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Mod{PackageID: fmt.Sprintf("tester.new%d", n), Name: name, Files: files})
			}
			scanNow(t, e)
			if qs, err := e.App.Store.Questions(mustEnv(t, e), false); err != nil || len(qs) != 0 {
				t.Fatalf("ambiguous questions=%+v err=%v", qs, err)
			}
		})
	}
}

func TestIdentityQuestionsRecheckStored(t *testing.T) {
	for _, strong := range []bool{false, true} {
		t.Run(fmt.Sprint(strong), func(t *testing.T) {
			t.Parallel()
			e := setup(t, false)
			var files map[string]string
			if strong {
				files = testkit.Sample("tester.template", "Shared payload").Files
			}
			oldDir := filepath.Join(e.Game, "Mods", "Old")
			e.WriteMod(oldDir, testkit.Mod{PackageID: "tester.old", Name: "Old", Files: files})
			scanNow(t, e)
			old := one(t, e, "Old")
			e.Remove(oldDir)
			scanNow(t, e)
			for n := 0; n < 67; n++ {
				name := fmt.Sprintf("New %02d", n)
				e.WriteMod(filepath.Join(e.Game, "Mods", name), testkit.Mod{PackageID: fmt.Sprintf("tester.new%d", n), Name: name, Files: files})
			}
			sid := scanNow(t, e)
			var answered int64
			for n := 0; n < 67; n++ {
				m := one(t, e, fmt.Sprintf("New %02d", n))
				r, err := e.App.Store.DB.Exec(`INSERT INTO identity_question(environment_id,scan_id,instance_id,observation_snapshot_id,candidate_instance_id,score)
			VALUES(?,?,?,?,?,1)`, mustEnv(t, e), sid, m.ID, m.SnapID, old.ID)
				if err != nil {
					t.Fatal(err)
				}
				if n == 0 {
					answered, _ = r.LastInsertId()
				}
			}
			answer := "unrelated"
			if strong {
				answer = "copy"
			}
			q, err := e.App.ResolveQuestion(answered, answer)
			if err != nil {
				t.Fatal(err)
			}
			before, err := e.App.Store.QuestionByID(q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.ApplyScan(mustEnv(t, e), sid, e.Now(), &identity.Result{}, nil); err != nil {
				t.Fatal(err)
			}
			// A scan that read no folder still judges every open question from the stored snapshots.
			if pending, err := e.App.Store.Questions(mustEnv(t, e), false); err != nil || len(pending) != 0 {
				t.Fatalf("partial scan kept unsupported questions: %d err=%v", len(pending), err)
			}
			scanNow(t, e)
			all, err := e.App.Store.Questions(mustEnv(t, e), true)
			if err != nil || len(all) != 1 || *all[0] != *before {
				t.Fatalf("answered decision changed: %+v, err=%v", all, err)
			}
			if m := one(t, e, "New 01"); m.Status != identity.Live || m.LineageID == old.LineageID {
				t.Fatal("cleanup changed a mod")
			}
		})
	}
}

// A unique content match still asks, survives a no-change scan, and explains the
// original comparison even after the candidate is updated or the answer merges it.
func TestIdentityQuestionEvidenceAndResolution(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	oldDir := filepath.Join(e.Game, "Mods", "Old")
	e.WriteMod(oldDir, testkit.Sample("tester.old", "Shared payload"))
	scanNow(t, e)
	old := one(t, e, "Shared payload")
	e.Remove(oldDir)
	scanNow(t, e)
	e.WriteMod(filepath.Join(e.Game, "Mods", "New"), testkit.Sample("tester.new", "Shared payload"))
	scanNow(t, e)
	qs, err := e.App.Store.Questions(mustEnv(t, e), false)
	if err != nil || len(qs) != 1 {
		t.Fatalf("questions=%+v err=%v", qs, err)
	}
	q := qs[0]
	check := func(q *store.Question) {
		t.Helper()
		v, err := e.App.Store.QuestionEvidence(q)
		if err != nil {
			t.Fatal(err)
		}
		if v.Files.PayloadCommon != 3 || len(v.Files.Examples) != 3 || v.New.PackageID != "tester.new" || v.Candidate.PackageID != "tester.old" {
			t.Fatalf("evidence=%+v", v)
		}
	}
	check(q)
	scanNow(t, e)
	if qs, _ := e.App.Store.Questions(mustEnv(t, e), false); len(qs) != 1 || qs[0].ID != q.ID {
		t.Fatal("valid question lost or duplicated")
	}
	out := e.MustRun("identity", "list")
	if !strings.Contains(out, "Shared payload") {
		t.Fatal(out)
	}
	if _, err := e.App.ResolveQuestion(q.ID, "same"); err != nil {
		t.Fatal(err)
	}
	check(q)
	answered, err := e.App.Store.QuestionByID(q.ID)
	if err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	if after, err := e.App.Store.QuestionByID(q.ID); err != nil || *answered != *after {
		t.Fatalf("scan changed same decision: %+v err=%v", after, err)
	}
	if m := one(t, e, "Shared payload"); m.ID != old.ID {
		t.Fatal("same did not preserve identity")
	}
}
