package view_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSubscriptionsThreeSections(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.AddWorkshop("1001", testkit.Sample("tester.workshop", "Workshop Mod"))
	e.SetTimeUpdated("1001", e.Now().Unix())
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("tester.local", "Local Mod"))
	e.WriteMod(filepath.Join(e.SteamCMD, "1002"), testkit.Sample("tester.steamcmd", "SteamCMD Mod"))
	e.AddEnv(true)
	e.MustRun("scan")
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	git := op.Items[0]
	if err := subscribe.Write(git.Folder, &subscribe.Record{Kind: "git", URL: g.Bare, Installed: "oldcommit", Latest: "newcommit", InstalledAt: e.Now(), CheckedAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	pageDir := filepath.Join(e.App.SubscribedDir(), "Page")
	e.WriteMod(pageDir, testkit.Sample("tester.page", "Page Mod"))
	if err := subscribe.Write(pageDir, &subscribe.Record{Kind: "page", URL: "https://example.invalid/file", Installed: "1.0", Latest: "2.0", InstalledAt: e.Now(), CheckedAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := subscribe.Write(filepath.Join(e.SteamCMD, "1002"), &subscribe.Record{Kind: "steamcmd", WorkshopID: "1002", InstalledAt: e.Now(), Installed: e.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	e.MustRun("enable", git.Ref, "pkg:tester.page", "pkg:tester.local")
	e.RecordLaunch(e.Now())
	e.Advance(time.Hour)
	local := testkit.Sample("tester.local", "Local Mod")
	local.Files["Defs/Changed.xml"] = "<Defs/>"
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), local)
	e.MustRun("scan")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = r.ParseForm()
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":%q,"result":1,"time_updated":%d}]}}`, r.Form.Get("publishedfileids[0]"), e.Now().Unix())
	}))
	defer server.Close()
	e.App.SteamAPI = server.URL
	for _, kind := range []string{app.LibWorkshop, app.LibSteamCMD} {
		if err := e.App.CheckSubscriptionSource(kind); err != nil {
			t.Fatal(err)
		}
	}
	before := requests
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if requests != before || len(p.Sources) != 5 || len(p.Pending) != 4 {
		t.Fatalf("not offline or missing sections: %+v requests %d", p, requests)
	}
	for _, source := range p.Sources {
		if source.Count != 1 {
			t.Fatalf("count: %+v", source)
		}
	}
	if len(p.History.Rows) != 1 || p.History.Rows[0].Mod != "Local Mod" {
		t.Fatalf("local changes missing from history: %+v", p.History)
	}
	for _, row := range p.Pending {
		if row.Kind == app.LibLocal || row.Available == "—" {
			t.Fatalf("invalid pending row: %+v", row)
		}
		if row.Kind == app.LibGit && len(row.UsedIn) != 1 {
			t.Fatalf("list usage: %+v", row)
		}
	}
	// Instance IDs are incidental to the section contents.
	for i := range p.Pending {
		p.Pending[i].ID, p.Pending[i].Ref = "id", "ref"
	}
	for i := range p.History.Rows {
		p.History.Rows[i].ID, p.History.Rows[i].Ref = "id", "ref"
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	actual := string(b) + "\n"
	golden := "testdata/subscriptions.golden.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, []byte(actual), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != actual {
		t.Fatalf("subscriptions golden differs:\n%s", actual)
	}
}

func TestSubscriptionSourceStates(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe"), "fixture executable")
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		if err := e.App.Store.SetMeta("account:loverslab:enabled", fmt.Sprint(enabled)); err != nil {
			t.Fatal(err)
		}
		p, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
		if err != nil {
			t.Fatal(err)
		}
		want := "未启用"
		if enabled {
			want = "未连接"
		}
		if p.Sources[1].Account != "匿名" || p.Sources[3].Account != want {
			t.Fatalf("source account states: %+v", p.Sources)
		}
		for _, s := range p.Sources {
			if s.Checked != "" || s.Result != "not checked yet" {
				t.Fatalf("unchecked source: %+v", s)
			}
		}
	}
}
