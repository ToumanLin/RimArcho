package view_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/source"
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
	info, err := view.BuildAddSubscriptionInfo(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Recent, p.Recent) {
		t.Fatalf("light recent differs: %+v / %+v", info.Recent, p.Recent)
	}
	for _, src := range info.Sources {
		for _, full := range p.Sources {
			if src.Key == full.Key && src.Account != full.Account {
				t.Fatalf("light account differs: %+v / %+v", src, full)
			}
		}
	}
	for _, recent := range p.Recent {
		if recent.ID == "" || st.Ix.ByID[recent.ID] == nil {
			t.Fatalf("recent subscription lacks its exact installed instance: %+v", recent)
		}
	}
	// Instance IDs are incidental to the section contents.
	for i := range p.Recent {
		p.Recent[i].ID = "id"
	}
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

	// Recent installs have the same five-row limit and stable ties on both pages.
	for i := 0; i < 7; i++ {
		dir := filepath.Join(e.App.SubscribedDir(), fmt.Sprintf("Recent%d", i))
		e.WriteMod(dir, testkit.Sample(fmt.Sprintf("tester.recent%d", i), fmt.Sprintf("Recent %d", i)))
		if err := subscribe.Write(dir, &subscribe.Record{Kind: "page", URL: "https://example.invalid/file", InstalledAt: e.Now().Add(time.Duration(i/2) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	e.MustRun("scan")
	st, err = e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	full, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	info, err = view.BuildAddSubscriptionInfo(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Recent) != 5 || !reflect.DeepEqual(info.Recent, full.Recent) {
		t.Fatalf("recent limit/ties differ: %+v / %+v", info.Recent, full.Recent)
	}
}

func TestSubscriptionSourceStates(t *testing.T) {
	t.Parallel()
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
		ll := ""
		if enabled {
			ll = "unavailable, not logged in"
		}
		if p.Sources[1].Account != "mod downloads available, not logged in" || p.Sources[3].Account != ll {
			t.Fatalf("source account states: %+v", p.Sources)
		}
		for _, s := range p.Sources {
			if s.Checked != "" || s.Result != "not checked yet" {
				t.Fatalf("unchecked source: %+v", s)
			}
		}
	}
}

func TestWorkshopFailureReasonsAndGrouping(t *testing.T) {
	view.SetLang("zh")
	defer view.SetLang("en")
	e := testkit.New(t)
	e.AddEnv(true)
	codes := []int{9, 9, 2, 15, 16, 20, 24, 25, 41, 84, 999}
	for i := range codes {
		e.AddWorkshop(fmt.Sprint(10000+i), testkit.Sample(fmt.Sprint("tester.failure", i), fmt.Sprint("Fixture ", i)))
	}
	e.MustRun("scan")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		var out []string
		for i := 0; r.Form.Has(fmt.Sprintf("publishedfileids[%d]", i)); i++ {
			id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
			index := 0
			fmt.Sscan(id, &index)
			out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":%d}`, id, codes[index-10000]))
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(out, ","))
	}))
	defer server.Close()
	e.App.SteamAPI = server.URL
	if err := e.App.CheckSubscriptionSource(app.LibWorkshop); err == nil || strings.Contains(err.Error(), "result 9") {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	page, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	row := page.Sources[0]
	// Removed Workshop pages are not check failures and have their own installed-copy rows.
	if row.Result != view.Tf("%d checks failed", 9) || len(row.Failures) != 9 || len(page.Gone) != 2 {
		t.Fatalf("removed/check sections: %+v / %+v", row, page.Gone)
	}
	for _, gone := range page.Gone {
		if gone.ID == "" || gone.Ref == "" || !strings.HasPrefix(gone.Name, "Fixture ") {
			t.Fatalf("removed copy lacks identity: %+v", gone)
		}
	}
	if page.Summary.Failures != 9 || !strings.Contains(page.Summary.Line, view.Tf("%d checks failed", 9)) {
		t.Fatal(page.Summary)
	}
	for _, code := range []int{2, 15, 16, 20, 24, 25, 26, 41, 84, 999} {
		text := view.Text(app.WorkshopResultText(code))
		if text == "" || strings.Contains(text, "result ") {
			t.Fatal(code, text)
		}
	}
}

// open the source page (16 §2.9): two commits on GitHub / GitLab open their compare view; otherwise the
// repository or the download page.
func TestSubscriptionSourceURL(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		r    subscribe.Record
		want string
	}{
		{subscribe.Record{Kind: "git", URL: "https://github.com/a/b.git", Installed: "aaa", Latest: "bbb"}, "https://github.com/a/b/compare/aaa...bbb"},
		{subscribe.Record{Kind: "git", URL: "https://gitlab.com/g/s/p", Installed: "aaa", Latest: "bbb"}, "https://gitlab.com/g/s/p/-/compare/aaa...bbb"},
		{subscribe.Record{Kind: "git", URL: "https://gitgud.io/a/b", Installed: "aaa", Latest: "bbb"}, "https://gitgud.io/a/b"},
		{subscribe.Record{Kind: "git", URL: "https://github.com/a/b", Installed: "aaa", Latest: "aaa"}, "https://github.com/a/b"},
		{subscribe.Record{Kind: "page", URL: "https://www.loverslab.com/files/file/1-x/"}, "https://www.loverslab.com/files/file/1-x/"},
	} {
		if got := view.SubscriptionSourceURL(&c.r); got != c.want {
			t.Errorf("%+v: %q, want %q", c.r, got, c.want)
		}
	}
}

func TestAddSubscriptionRecentMatchesLibrarySourceRules(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	item := op.Items[0]
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	m := st.Ix.ByID[item.ID]
	if m == nil {
		t.Fatal("missing subscribed mod")
	}
	src := st.Ix.Source(m.SourceID)
	original := src.Kind
	defer func() { src.Kind = original }()
	for n, tc := range []struct {
		name, record, want string
		source             source.Kind
	}{
		{"git overrides extra", app.LibGit, app.LibGit, original},
		{"page overrides extra", app.LibPage, app.LibPage, original},
		{"steamcmd record preserves extra", app.LibSteamCMD, "", original},
		{"unknown record preserves extra", "future", "", original},
		{"unknown source is skipped", app.LibGit, "", source.Kind("future")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src.Kind = tc.source
			r := &subscribe.Record{Kind: tc.record, URL: g.Bare, Installed: "old", InstalledAt: e.Now()}
			if err := subscribe.Write(item.Folder, r); err != nil {
				t.Fatal(err)
			}
			stamp := e.Now().Add(time.Duration(n+1) * time.Hour)
			if err := os.Chtimes(filepath.Join(item.Folder, subscribe.File), stamp, stamp); err != nil {
				t.Fatal(err)
			}
			full, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
			if err != nil {
				t.Fatal(err)
			}
			light, err := view.BuildAddSubscriptionInfo(e.App)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(light.Recent, full.Recent) {
				t.Fatalf("recent differs: light=%+v full=%+v", light.Recent, full.Recent)
			}
			if tc.want == "" {
				if len(light.Recent) != 0 {
					t.Fatalf("unexpected recent: %+v", light.Recent)
				}
			} else if len(light.Recent) != 1 || light.Recent[0].Kind != tc.want {
				t.Fatalf("recent: %+v, want %s", light.Recent, tc.want)
			}
		})
	}
}
