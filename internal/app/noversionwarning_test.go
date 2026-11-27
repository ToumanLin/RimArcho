package app_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// nvwZip builds a repository zip like GitHub's archive: <root>/<version>/ModIdsToFix.xml.
func nvwZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, body := range files {
		f, err := w.Create("NoVersionWarning-main/" + name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func nvwList(ids ...string) string {
	s := "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModIdsToFix>\n"
	for _, id := range ids {
		s += "  <!-- " + id + " -->\n  <li>" + id + "</li>\n"
	}
	return s + "</ModIdsToFix>\n"
}

// 08 §2.3 D7, 05 §4.2: No Version Warning answers a version warning for the game version it
// lists the Mod under (case-insensitive), apart from the player's ignores, and never as a
// declaration.
func TestNoVersionWarning(t *testing.T) {
	t.Parallel()
	e := versionMuteEnv(t) // game 1.7; Alpha lists 1.6, Beta 1.5 1.6
	if e.App.VersionConfirmed("tester.a", "1.7") {
		t.Fatal("confirmed without the dataset")
	}
	if active, muted := versionProblemCounts(t, e); active != 2 || muted != 0 {
		t.Fatalf("without the dataset %d %d", active, muted)
	}
	body := nvwZip(t, map[string]string{
		"1.7/ModIdsToFix.xml":   nvwList("Tester.A"),
		"1.6/ModIdsToFix.xml":   nvwList("tester.b", "tester.other"),
		"1.5/ModIdsToFix.xml":   "<ModIdsToFix><li>broken", // one bad list is skipped alone
		"About/About.xml":       "<ModMetaData/>",
		"Source/ModIdsToFix.cs": "not a list",
	})
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("If-None-Match") == `"nvw"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"nvw"`)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	e.MustRun("config", "set", "data.noversionwarning.url", srv.URL+"/main.zip")
	e.MustRun("data", "update", "noversionwarning")
	if out := e.MustRun("data", "status"); !strings.Contains(out, "noversionwarning: 3 entries") || !strings.Contains(out, "1.6 1.7") {
		t.Fatalf("status:\n%s", out)
	}
	if out := e.MustRun("data", "update", "noversionwarning"); !strings.Contains(out, "unchanged") || hits != 2 {
		t.Fatalf("conditional request (%d):\n%s", hits, out)
	}

	// Matching: case-insensitive, _steam stripped, per game version.
	for _, c := range []struct {
		pkg, mm string
		want    bool
	}{{"tester.a", "1.7", true}, {"TESTER.A_steam", "1.7", true}, {"tester.a", "1.6", false},
		{"tester.b", "1.7", false}, {"tester.b", "1.6", true}, {"tester.b", "1.5", false}, {"tester.a", "", false}} {
		if got := e.App.VersionConfirmed(c.pkg, c.mm); got != c.want {
			t.Errorf("VersionConfirmed(%q, %q) = %v", c.pkg, c.mm, got)
		}
	}

	// Alpha's warning leaves the list for Confirmed, even when the player also muted it.
	e.MustRun("problems", "mute-version", "pkg:tester.a")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Confirmed) != 1 || len(ps.Muted) != 0 {
		t.Fatalf("confirmed %d muted %d", len(ps.Confirmed), len(ps.Muted))
	}
	p := ps.Confirmed[0]
	if !p.VersionConfirmed || !slices.ContainsFunc(p.Evidence, func(ev finding.Evidence) bool {
		return ev.Source == finding.Community && strings.Contains(ev.Text, app.NoVersionWarningName)
	}) {
		t.Fatalf("evidence %+v", p.Evidence)
	}
	for _, q := range ps.List {
		if q.Code == "version.unsupported" && q.SubjectInstance() == p.SubjectInstance() {
			t.Fatal("confirmed warning still counted")
		}
	}

	// The declaration is unchanged: 1.7 is not common to the Mods.
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	pv, err := e.App.PlanVersionsOf(st, st.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Missing["1.7"] != 1 || pv.Confirmed["1.7"] != 1 || pv.Muted["1.7"] != 0 || slices.Contains(pv.Common, "1.7") {
		t.Fatalf("plan versions %+v", pv)
	}
	if out := e.MustRun("plan", "versions"); !strings.Contains(out, "1 confirmed working by community data (No Version Warning)") {
		t.Fatalf("plan versions:\n%s", out)
	}

	// Can it run, the version panel and the Mod's version line say it apart.
	// Can it run: one muted line inside the version item, never an item of its own.
	n := view.BuildNeeds(e.App, st, ps)
	var version *view.Need
	for i, it := range n.Items {
		if it.Kind == "version.confirmed" {
			t.Fatalf("separate confirmed item beside a warning: %+v", it)
		}
		if it.Kind == "version.unsupported" {
			version = &n.Items[i]
		}
	}
	if version == nil || !slices.Contains(version.Also, "1 more confirmed working by community data") || len(version.Mods) != 1 {
		t.Fatalf("version item %+v", version)
	}
	vw, err := view.ReadVersionWarnings(e.App, "1.7", false)
	if err != nil {
		t.Fatal(err)
	}
	last := vw.Groups[len(vw.Groups)-1]
	if !strings.Contains(last.Title, "No Version Warning") || last.Action.Kind != "" || len(last.Rows) != 1 || last.Rows[0].Name != "Alpha" || last.Rows[0].Action.Kind != "" {
		t.Fatalf("panel group %+v", last)
	}
	if len(vw.All.Refs) != 1 {
		t.Fatalf("ignore all covers %v", vw.All.Refs)
	}
	if muted, _ := view.ReadVersionWarnings(e.App, "1.7", true); len(muted.Groups) != 0 {
		t.Fatalf("confirmed shown as ignored: %+v", muted.Groups)
	}
	r, err := view.VersionReason(e.App, st, st.Ix.ByID[p.SubjectInstance()])
	if err != nil || r == nil || !strings.Contains(r.Text, "confirmed working on 1.7") || len(r.Actions) != 0 || r.Evidence == nil || r.Evidence.Kind != view.EvCommunity {
		t.Fatalf("reason %+v %v", r, err)
	}

	out := e.MustRun("problems")
	if !strings.Contains(out, "confirmed-version-warnings: 1 mods confirmed working by community data") || strings.Contains(out, "ignored-version-warnings") {
		t.Fatalf("problems:\n%s", out)
	}
	if out := e.MustRun("problems", "--include-muted", "--code", "version.unsupported"); !strings.Contains(out, "[confirmed working by No Version Warning]") {
		t.Fatalf("include-muted:\n%s", out)
	}
}

// When community data answers every version warning, nothing breaks the game: one quiet,
// uncounted line worth knowing, with no button (05 §4.2).
func TestNoVersionWarningAllConfirmed(t *testing.T) {
	t.Parallel()
	e := versionMuteEnv(t)
	src := filepath.Join(e.Root, "sources", "nvw.zip")
	e.WriteFile(src, string(nvwZip(t, map[string]string{"1.7/ModIdsToFix.xml": nvwList("tester.a", "TESTER.B")})))
	e.MustRun("config", "set", "data.noversionwarning.url", src)
	e.MustRun("data", "update", "noversionwarning")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, st, ps)
	found := 0
	for _, it := range n.Items {
		switch it.Kind {
		case "version.unsupported":
			t.Fatalf("version item with every warning confirmed: %+v", it)
		case "version.confirmed":
			found++
			if it.Counted || len(it.Actions) != 0 || it.Section != view.SectionHint || it.Text != "2 mods confirmed working on 1.7 by community data (No Version Warning)" {
				t.Fatalf("quiet line %+v", it)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%d confirmed lines", found)
	}
}

// A plain ModIdsToFix.xml as the source applies to every game version; an archive without
// any list fails the update and keeps what was stored.
func TestNoVersionWarningPlainFileAndInvalid(t *testing.T) {
	t.Parallel()
	e := versionMuteEnv(t)
	src := filepath.Join(e.Root, "sources", "ModIdsToFix.xml")
	e.WriteFile(src, nvwList("tester.b"))
	e.MustRun("config", "set", "data.noversionwarning.url", src)
	e.MustRun("data", "update", "noversionwarning")
	if !e.App.VersionConfirmed("tester.b", "1.7") || !e.App.VersionConfirmed("tester.b", "1.4") || e.App.VersionConfirmed("tester.a", "1.7") {
		t.Fatal("plain list not applied to every version")
	}
	bad := filepath.Join(e.Root, "sources", "empty.zip")
	e.WriteFile(bad, string(nvwZip(t, map[string]string{"About/About.xml": "<ModMetaData/>"})))
	e.MustRun("config", "set", "data.noversionwarning.url", bad)
	if out, code := e.Run("data", "update", "noversionwarning"); code == 0 || !strings.Contains(out, "ModIdsToFix.xml") {
		t.Fatalf("archive without lists accepted (%d):\n%s", code, out)
	}
	if !e.App.VersionConfirmed("tester.b", "1.7") {
		t.Fatal("stored list lost on a failed update")
	}
}
