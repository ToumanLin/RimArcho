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

func TestCommunityDataIndexFailureKeepsWholeSet(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	payloads := map[string]string{
		"rules":        `{"timestamp":1,"rules":{}}`,
		"steamdb":      `{"version":1,"database":{"42":{"packageId":"tester.old","name":"Old"}}}`,
		"replacements": `{"version":"one","rules":[]}`,
	}
	for _, d := range app.Datasets {
		path := filepath.Join(e.Root, "sources", d.File)
		e.WriteFile(path, payloads[d.Key])
		e.MustRun("config", "set", "data."+d.Key+".url", path)
	}
	e.MustRun("data", "update")
	cache := filepath.Join(e.Home, "cache")
	previous := map[string]string{}
	files, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !f.IsDir() {
			b, err := os.ReadFile(filepath.Join(cache, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			previous[f.Name()] = string(b)
		}
	}
	generation := e.App.Store.Meta("community:generation")
	// The outer shape passes validation, but building the compact index fails.
	e.WriteFile(filepath.Join(e.Root, "sources", app.Datasets[0].File), `{"timestamp":2,"rules":{"tester.new":{}}}`)
	e.WriteFile(filepath.Join(e.Root, "sources", app.Datasets[1].File), `{"version":2,"database":{"42":"invalid entry"}}`)
	if _, err := e.App.UpdateData(nil, false); err == nil || !strings.Contains(err.Error(), "steamDB index") {
		t.Fatal(err)
	}
	for name, want := range previous {
		b, err := os.ReadFile(filepath.Join(cache, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s changed on index failure: %s %v", name, b, err)
		}
	}
	if e.App.Store.Meta("community:generation") != generation {
		t.Fatal("failed update published a new generation")
	}
	files, err = os.ReadDir(cache)
	if err != nil || len(files) != len(previous) {
		t.Fatal("staging files left", files, err)
	}
}

func TestCommunityDataEncoding(t *testing.T) {
	t.Parallel()
	const payload = `{"version":"test","rules":[{"oldPackageId":"tester.a","newPackageId":"tester.a2","newName":"A Continued"}]}`
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"plain", []byte(payload)},
		{"plain_bom", []byte("\xef\xbb\xbf" + payload)},
		{"gzip", gz(payload)},
		{"gzip_bom", gz("\xef\xbb\xbf" + payload)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := loadoutEnv(t)
			path := filepath.Join(t.TempDir(), "replacements.json.gz")
			if err := os.WriteFile(path, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			e.MustRun("config", "set", "data.replacements.url", path)
			e.MustRun("data", "update", "replacements")
			info := e.App.DataStatus()["replacements"]
			if info == nil || info.Entries != 1 || info.Version != "test" {
				t.Fatalf("unexpected replacements status: %+v", info)
			}
			if r := e.App.ReplacementFor("", "tester.a"); r == nil || r.NewPackageID != "tester.a2" {
				t.Fatalf("replacement not loaded: %+v", r)
			}
		})
	}
}

// One malformed record never drops a whole dataset: the shipped Use This Instead list has
// had rules whose oldPackageId is a number (08 §2.3).
func TestCommunityDataToleratesBadRecords(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	payloads := map[string]string{
		"rules": `{"timestamp":1,"rules":{}}`,
		"steamdb": `{"version":1,"database":{"41":"broken","42":{"packageId":"Tester.Gone","name":"Gone"},
			"43":{"packageId":7,"name":"Numeric"},"294100":{"appid":true,"packageId":"ludeon.rimworld"}}}`,
		"replacements": `{"version":"v","rules":[
			{"oldWorkshopId":"1797397487","oldPackageId":1797397487,"oldName":"Numeric","newWorkshopId":"2195469123","newPackageId":"Tester.Numeric2","newVersions":["1.6"]},
			{"oldWorkshopId":"1001","oldPackageId":"Tester.A","oldAuthor":null,"newWorkshopId":"9999","newPackageId":"Tester.A2","newName":"A Continued","newVersions":["1.6",1.7]},
			{"oldWorkshopId":"1002","oldPackageId":{"bad":true},"newPackageId":"tester.b2"},
			{"oldWorkshopId":"1003","oldPackageId":null,"newPackageId":"tester.c2"}]}`,
	}
	for _, d := range app.Datasets {
		path := filepath.Join(e.Root, "sources", d.File)
		e.WriteFile(path, payloads[d.Key])
		e.MustRun("config", "set", "data."+d.Key+".url", path)
	}
	e.MustRun("data", "update")
	if r := e.App.ReplacementFor("", "tester.a"); r == nil || r.NewPackageID != "tester.a2" || strings.Join(r.NewVersions, ",") != "1.6,1.7" {
		t.Fatalf("valid rule not loaded beside the numeric one: %+v", r)
	}
	if r := e.App.ReplacementFor("1001", "tester.a"); r == nil || r.NewWorkshopID != "9999" {
		t.Fatalf("old Workshop item not matched: %+v", r)
	}
	// A replacement that keeps the old packageId is not its own replacement, nor is another
	// Workshop item with that packageId.
	for _, ws := range []string{"9999", "5555"} {
		if r := e.App.ReplacementFor(ws, "tester.a"); r != nil {
			t.Fatalf("ws:%s with the old packageId matched: %+v", ws, r)
		}
	}
	if r := e.App.ReplacementFor("1797397487", ""); r == nil || r.NewPackageID != "tester.numeric2" {
		t.Fatalf("numeric oldPackageId rule: %+v", r)
	}
	if r := e.App.ReplacementFor("1003", ""); r == nil || r.NewPackageID != "tester.c2" {
		t.Fatalf("null oldPackageId rule: %+v", r)
	}
	if r := e.App.ReplacementFor("1002", ""); r != nil {
		t.Fatalf("rule with an object oldPackageId loaded: %+v", r)
	}
	if w, ok := e.App.WorkshopFor("tester.gone"); !ok || w.WorkshopID != "42" {
		t.Fatalf("steamDB index dropped by a broken entry: %+v %v", w, ok)
	}
	if _, ok := e.App.WorkshopFor("ludeon.rimworld"); ok {
		t.Fatal("app entry indexed")
	}
}

// D1–D3 (08 §2.3): download, all-or-nothing replacement, and what the data adds to Can it run.
func TestCommunityData(t *testing.T) {
	t.Parallel()
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
