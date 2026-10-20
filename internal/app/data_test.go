package app_test

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func gz(s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

// D1–D3 (10 §5): download, all-or-nothing replacement, and what the data adds to 需要处理.
func TestCommunityData(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	files := map[string][]byte{
		"/rules.json":   []byte(`{"timestamp": 1789239946, "rules": {"tester.b": {"loadAfter": {"tester.a": {"name": ["A"]}}}}}`),
		"/steamdb.json": []byte(`{"version": 1790337009, "database": {"4242": {"packageId": "Gone.Mod", "name": "Gone Mod"}, "294100": {"appid": true, "packageid": "ludeon.rimworld"}}}`),
		"/repl.json.gz": gz(`{"version": "2026-10-19T06:29:54Z", "rules": [{"oldWorkshopId": "1001", "oldPackageId": "tester.a", "oldName": "A", "oldAuthor": "x", "oldVersions": ["1.5"],
			"newWorkshopId": "9999", "newPackageId": "tester.a2", "newName": "A Continued", "newAuthor": "Mlie", "newVersions": ["1.6"]}]}`),
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		tag := `"` + string(rune('a'+len(b)%26)) + `"`
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", tag)
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	e.MustRun("config", "set", "data.rules.url", srv.URL+"/rules.json")
	e.MustRun("config", "set", "data.steamdb.url", srv.URL+"/steamdb.json")
	e.MustRun("config", "set", "data.replacements.url", srv.URL+"/repl.json.gz")

	if out := e.MustRun("data", "update", "--dry-run"); hits != 0 || !strings.Contains(out, srv.URL+"/rules.json") {
		t.Fatalf("dry run fetched %d:\n%s", hits, out)
	}
	e.MustRun("data", "update")
	out := e.MustRun("data", "status")
	for _, want := range []string{"rules: 1 entries", "steamdb: 2 entries", "replacements: 1 entries"} {
		if !strings.Contains(out, want) {
			t.Fatalf("D1 status lacks %q:\n%s", want, out)
		}
	}
	if out := e.MustRun("data", "update"); strings.Count(out, "unchanged") != 3 {
		t.Fatalf("ETag: %s", out)
	}

	// D3: the replacement and the direct link for the missing entry.
	st, _ := e.App.Loadout()
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := view.BuildNeeds(e.App, st, ps)
	var texts, urls []string
	for _, it := range n.Items {
		texts = append(texts, it.Text)
		for _, a := range it.Actions {
			urls = append(urls, a.URL)
		}
	}
	joinedT, joinedU := strings.Join(texts, "\n"), strings.Join(urls, "\n")
	if !strings.Contains(joinedT, "has a maintained replacement: A Continued") || !strings.Contains(joinedU, "?id=9999") {
		t.Fatalf("D3 replacement:\n%s\n%s", joinedT, joinedU)
	}
	if !strings.Contains(joinedU, "?id=4242") {
		t.Fatalf("missing entry has no direct link:\n%s", joinedU)
	}

	// D2: one invalid file stops the whole update; the stored files stay.
	rulesBefore, _ := os.ReadFile(filepath.Join(e.Home, "cache", "communityRules.json"))
	files["/rules.json"] = []byte(`{"timestamp": 1, "rules": {"tester.c": {}}}`)
	files["/steamdb.json"] = []byte(`{"version": 2, "database": [`)
	if out, code := e.Run("data", "update"); code == 0 || !strings.Contains(out, "unchanged") {
		t.Fatalf("D2: exit %d\n%s", code, out)
	}
	if after, _ := os.ReadFile(filepath.Join(e.Home, "cache", "communityRules.json")); !bytes.Equal(after, rulesBefore) {
		t.Fatal("D2: rules replaced although steamDB failed")
	}
	if left, _ := filepath.Glob(filepath.Join(e.Home, "cache", "*.tmp")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
	if _, ok := e.App.WorkshopFor("gone.mod"); !ok {
		t.Fatal("index lost")
	}
	_ = app.Datasets
}
