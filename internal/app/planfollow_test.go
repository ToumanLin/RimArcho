package app_test

// Following a Steam Workshop collection (07 §7.6, C1–C4).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// fakeCollection serves collection 3000000009 with the children *items holds at each read.
func fakeCollection(t *testing.T, e *testkit.Env, items *[]string) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/collection":
			var b strings.Builder
			b.WriteString(`{"response":{"result":1,"resultcount":1,"collectiondetails":[{"publishedfileid":"3000000009","result":1,"children":[`)
			for i, id := range *items {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`{"publishedfileid":"` + id + `","sortorder":` + string(rune('1'+i)) + `,"filetype":0}`)
			}
			b.WriteString(`]}]}}`)
			w.Write([]byte(b.String()))
		case "/api":
			w.Write([]byte(`{"response":{"publishedfiledetails":[{"publishedfileid":"3000000009","result":1,"title":"Pack of Mods"},{"publishedfileid":"5550001","result":1,"title":"Far Mod"}]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"
}

func TestFollowCollection(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.AddWorkshop("1002", testkit.Sample("tester.d", "D"))
	scanNow(t, e)
	items := []string{"2009463077", "1001"}
	fakeCollection(t, e, &items)
	mc := e.ReadModsConfig()
	e.MustRun("plan", "new", "other", "--from", "current")

	out := e.MustRun("plan", "follow", "default", "https://steamcommunity.com/sharedfiles/filedetails/?id=3000000009")
	if !strings.Contains(out, "plan:default follows Pack of Mods") || !strings.Contains(out, "no change since the last catch-up") {
		t.Fatal(out)
	}
	e.MustRun("plan", "follow", "other", "3000000009", "--mode", "follow")

	// The collection drops A and gains D (installed) and Far Mod (not installed).
	items = []string{"2009463077", "1002", "5550001"}
	out = e.MustRun("plan", "sync", "default", "--check")
	for _, want := range []string{"1 changed", "new: 2", "+ Far Mod", "dropped: 1", "- ws:1001", "not installed: 1 (subscribe in the Workshop)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("check lacks %q:\n%s", want, out)
		}
	}
	// C1: add mode enables D next to Harmony, keeps A, lists Far Mod; one undoable step.
	out = e.MustRun("plan", "sync", "default")
	if !strings.Contains(out, "1 enabled, 0 disabled") || !strings.Contains(out, "kept: 1 mod the collection dropped stay") {
		t.Fatal(out)
	}
	if got := joined(active(t, e)); got != "Core, Harmony, D, A, ?gone.mod" {
		t.Fatalf("after sync: %s", got)
	}
	if out := e.MustRun("plan", "sync", "default", "--check"); !strings.Contains(out, "0 changed") {
		t.Fatalf("the snapshot moves on:\n%s", out)
	}
	// C3: the player disables D; the collection did not change, so nothing is offered.
	e.MustRun("disable", refOf(t, e, "D", ""))
	if out := e.MustRun("plan", "sync", "default", "--check"); !strings.Contains(out, "0 changed") {
		t.Fatalf("a mod the player disabled is not a change:\n%s", out)
	}
	e.MustRun("undo")
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("undo the sync: %s", got)
	}

	// C2: follow mode, on a plan that is not the current one.
	e.MustRun("plan", "sync", "other", "--check")
	out = e.MustRun("plan", "sync", "other", "--dry-run")
	if !strings.Contains(out, "would catch up with Pack of Mods: 1 enabled, 1 disabled") {
		t.Fatal(out)
	}
	e.MustRun("plan", "sync", "other")
	e.MustRun("plan", "use", "other")
	if got := joined(active(t, e)); got != "Core, Harmony, D, ?gone.mod" {
		t.Fatalf("follow mode: %s", got)
	}
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("undo in the other plan: %s", got)
	}

	// C4: unfollowing leaves the plan as it is.
	e.MustRun("plan", "unfollow", "other", "3000000009")
	if out, code := e.Run("plan", "sync", "other"); code == 0 || !strings.Contains(out, "follows no collection") {
		t.Fatalf("after unfollow: %s", out)
	}
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("unfollow changed the plan: %s", got)
	}
	if e.ReadModsConfig() != mc {
		t.Fatal("following writes no game file")
	}
}
