package app_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func identityReferenceFixture(t *testing.T) (*testkit.Env, *store.Question) {
	t.Helper()
	e := setup(t, false)
	old := filepath.Join(e.Game, "Mods", "qmod-a")
	files := map[string]string{}
	for n := 0; n < 4; n++ {
		files[fmt.Sprintf("Defs/%d.xml", n)] = fmt.Sprintf("<Defs><ThingDef><defName>Q%d</defName></ThingDef></Defs>", n)
	}
	e.WriteMod(old, testkit.Mod{PackageID: "tester.qa", Name: "Q Mod A", Author: "Author A", Files: files})
	scanNow(t, e)
	e.Remove(old)
	e.WriteMod(filepath.Join(e.Game, "Mods", "qmod-b"), testkit.Mod{PackageID: "tester.qb", Name: "Q Mod B", Author: "Author B", Files: files})
	scanNow(t, e)
	qs, err := e.App.Store.Questions(mustEnv(t, e), false)
	if err != nil || len(qs) != 1 {
		t.Fatalf("questions=%+v err=%v", qs, err)
	}
	return e, qs[0]
}

// A decision must leave every restorable list usable, including older versions,
// a second plan, both history stacks and staged item anchors. Copy/unrelated keep
// the new instance, so their references must stay byte-for-byte unchanged.
func TestIdentityAnswersPreserveListReferences(t *testing.T) {
	for _, answer := range []string{"same", "copy", "unrelated"} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			e, q := identityReferenceFixture(t)
			e.MustRun("enable", "pkg:tester.qb")
			e.MustRun("save")
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			entries := st.List.Encode()
			versions, err := e.App.Store.Versions(st.Plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			oldVersion := versions[0].N
			if _, err := e.App.Store.AddVersion(st.Plan.ID, &store.Version{Created: e.Now(), Source: "user", Entries: entries, Note: "newer version"}); err != nil {
				t.Fatal(err)
			}
			// A second plan includes both IDs: the surviving identity occurs only once.
			second, err := e.App.Store.CreatePlan(mustEnv(t, e), "other", e.Now())
			if err != nil {
				t.Fatal(err)
			}
			duplicates := loadout.List{{Instance: q.InstanceID}, {Missing: "pkg:still.missing"}, {Instance: q.CandidateID}}
			if err := e.App.Store.SetWorkspace(&store.Workspace{PlanID: second.ID, Entries: duplicates.Encode(), Updated: e.Now()}); err != nil {
				t.Fatal(err)
			}
			staged := fmt.Sprintf(`[{"i":%q,"a":%q,"where":"after"}]`, q.InstanceID, q.InstanceID)
			if err := e.App.Store.SetMeta(store.StageKey(second.ID), staged); err != nil {
				t.Fatal(err)
			}
			for _, stack := range []string{"undo", "redo"} {
				if _, err := e.App.Store.DB.Exec(`INSERT INTO workspace_undo(plan_id,stack,entries,op,staging) VALUES(?,?,?,?,?)`, st.Plan.ID, stack, entries, "fixture", staged); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: mustEnv(t, e), PlanID: st.Plan.ID, Started: e.Now(), Method: "direct", Fingerprint: store.EntriesFingerprint(entries)}); err != nil {
				t.Fatal(err)
			}
			pack := app.Pack{Format: app.PackFormat, Name: "fixture", Env: "main", Entries: []app.PackEntry{{Instance: q.InstanceID, Lineage: one(t, e, "Q Mod B").LineageID, Name: "Q Mod B", PackageID: "tester.qb"}}}
			raw, err := json.Marshal(pack)
			if err != nil {
				t.Fatal(err)
			}
			packPath := filepath.Join(e.Home, "packs", "fixture", "pack.json")
			e.WriteFile(packPath, string(raw))
			if _, err := e.App.Store.DB.Exec(`INSERT INTO id_alias(old_id,new_id,kind) VALUES('earlier-copy',?,'instance')`, q.InstanceID); err != nil {
				t.Fatal(err)
			}
			if _, err := e.App.ResolveQuestion(q.ID, answer); err != nil {
				t.Fatal(err)
			}
			target := q.InstanceID
			if answer == "same" {
				target = q.CandidateID
			}
			want := strings.ReplaceAll(entries, q.InstanceID, target)
			ws, err := e.App.Store.Workspace(st.Plan.ID)
			if err != nil || ws.Entries != want {
				t.Fatalf("workspace=%+v err=%v", ws, err)
			}
			old, err := e.App.Store.Version(st.Plan.ID, oldVersion)
			if err != nil || old.Entries != want {
				t.Fatalf("old version=%+v err=%v", old, err)
			}
			for _, stack := range []string{"undo", "redo"} {
				step, err := e.App.Store.PeekStep(st.Plan.ID, stack)
				if err != nil || step.Entries != want || step.Staging == nil || strings.Contains(*step.Staging, `"`+q.InstanceID+`"`) && answer == "same" {
					t.Fatalf("%s=%+v err=%v", stack, step, err)
				}
			}
			secondWS, err := e.App.Store.Workspace(second.ID)
			if err != nil {
				t.Fatal(err)
			}
			secondList := loadout.Decode(secondWS.Entries)
			if answer == "same" {
				if len(secondList) != 2 || secondList[0].Instance != target || secondList[1].Missing != "pkg:still.missing" {
					t.Fatalf("dedup lost order or missing entry: %+v", secondList)
				}
			} else if secondWS.Entries != duplicates.Encode() {
				t.Fatal("independent list changed")
			}
			stage := e.App.Store.Meta(store.StageKey(second.ID))
			var stageItems []map[string]any
			if err := json.Unmarshal([]byte(stage), &stageItems); err != nil {
				t.Fatal(err)
			}
			if stageItems[0]["i"] != target || stageItems[0]["a"] != target || stageItems[0]["where"] != "after" {
				t.Fatalf("stage=%s", stage)
			}
			launches, err := e.App.Store.Launches(mustEnv(t, e), 5)
			if err != nil || launches[0].Fingerprint != store.EntriesFingerprint(want) {
				t.Fatalf("launches=%+v err=%v", launches, err)
			}
			read, err := e.App.ReadPack("fixture")
			if err != nil || read.Entries[0].Instance != target {
				t.Fatalf("pack=%+v err=%v", read, err)
			}
			if answer == "same" {
				aliases, err := e.App.Store.IDAliases()
				if err != nil || aliases["earlier-copy"] != target || aliases[q.InstanceID] != target {
					t.Fatalf("aliases=%v err=%v", aliases, err)
				}
				if read.Entries[0].Lineage != one(t, e, "Q Mod B").LineageID {
					t.Fatal("pack lineage not remapped")
				}
				var count int
				if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM instance WHERE id=?`, q.InstanceID).Scan(&count); err != nil || count != 0 {
					t.Fatal("retired identity retained")
				}
			}
			current, err := e.App.Loadout()
			if err != nil || current.List.Index(target) < 0 {
				t.Fatalf("list became unloadable: %+v err=%v", current, err)
			}
			e.MustRun("loadout", "checkout", fmt.Sprintf("v%d", oldVersion))
			restored, err := e.App.Loadout()
			if err != nil || restored.List.Index(target) < 0 {
				t.Fatal("checkout resurrected a retired identity")
			}
		})
	}
}

func TestResolveSameReferenceRewriteRollback(t *testing.T) {
	t.Parallel()
	e, q := identityReferenceFixture(t)
	e.MustRun("enable", "pkg:tester.qb")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	// Fail after the lists and alias map have been rewritten, before deletion commits.
	if _, err := e.App.Store.DB.Exec(`CREATE TRIGGER reject_identity_merge BEFORE DELETE ON instance BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.ResolveQuestion(q.ID, "same"); err == nil {
		t.Fatal("expected injected failure")
	}
	after, err := e.App.Store.Workspace(st.Plan.ID)
	if err != nil || after.Entries != st.WS.Entries {
		t.Fatal("failed merge changed list")
	}
	aliases, err := e.App.Store.IDAliases()
	if err != nil || aliases[q.InstanceID] != "" {
		t.Fatal("failed merge committed alias")
	}
	got, err := e.App.Store.QuestionByID(q.ID)
	if err != nil || got.Status != "open" {
		t.Fatal("failed merge answered question")
	}
	if len(named(t, e, "Q Mod B")) != 1 {
		t.Fatal("failed merge deleted mod")
	}
}
