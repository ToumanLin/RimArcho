package gui

import (
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"

	"github.com/ToumanLin/RimArcho/internal/view"
)

var textRe = regexp.MustCompile(`\bTf?\("((?:[^"\\]|\\.)*)"`)

// Every string the page translates has a Chinese entry (16 §7; the page has no table of
// its own: it gets view.Table and view.ExtraText).
func TestPageTextTranslated(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	table := maps.Clone(view.Table())
	maps.Copy(table, view.ExtraText())
	var missing []string
	fs.WalkDir(assets, "assets/js", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		b, _ := fs.ReadFile(assets, p)
		for _, m := range textRe.FindAllStringSubmatch(string(b), -1) {
			s := strings.ReplaceAll(m[1], `\"`, `"`)
			if _, ok := table[s]; !ok {
				missing = append(missing, s)
			}
		}
		return nil
	})
	sort.Strings(missing)
	missing = compactStrings(missing)
	if len(missing) > 0 {
		t.Errorf("%d page strings have no Chinese text:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func compactStrings(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// ui.gpu off adds the software-drawing switches; both keep the network service in the
// browser process and one renderer (17 §7.3).
func TestBrowserArgs(t *testing.T) {
	t.Parallel()
	on, off := strings.Join(BrowserArgs(true), " "), strings.Join(BrowserArgs(false), " ")
	for _, s := range []string{on, off} {
		if !strings.Contains(s, "NetworkServiceInProcess2") || !strings.Contains(s, "--renderer-process-limit=1") {
			t.Fatalf("missing the shared switches: %s", s)
		}
	}
	if strings.Contains(on, "--disable-gpu") || !strings.Contains(off, "--disable-gpu --disable-gpu-compositing --in-process-gpu") {
		t.Fatalf("gpu switches: on %q, off %q", on, off)
	}
}

// The same embedded files used by TestPageTextTranslated also contain untranslated
// separators and template literals. Inspect the whole source, excluding comments.
func TestWording(t *testing.T) {
	t.Parallel()
	commentsAndStrings := regexp.MustCompile("(?s)//[^\n]*|/\\*.*?\\*/|\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`(?:[^`\\\\]|\\\\.)*`")
	err := fs.WalkDir(assets, "assets/js", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		b, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, token := range commentsAndStrings.FindAllString(string(b), -1) {
			if strings.HasPrefix(token, "//") || strings.HasPrefix(token, "/*") {
				continue
			}
			text := strings.ReplaceAll(token, `\u00b7`, string(rune(0xB7)))
			for _, p := range view.WordingTextProblems(path, text) {
				t.Error(p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Synthetic files exercise the same handler without changing the embedded assets.
func TestArchoAssetCaching(t *testing.T) {
	version, commit, date := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.Date = version, commit, date })
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "1.0.0", "abc1234", "2026-11-06"
	files := fstest.MapFS{
		"index.html":                 {Data: []byte("<!doctype html>")},
		"archo/player.js":            {Data: []byte("export const player = true;")},
		"archo/archo-s/rig.json":     {Data: []byte(`{"rig":true}`)},
		"archo/archo-s/still.png":    {Data: []byte("synthetic png")},
		"archo/archo-s/texture.webp": {Data: []byte("synthetic webp")},
		"js/main.js":                 {Data: []byte("export const main = true;")},
		"archo-other/player.js":      {Data: []byte("export const other = true;")},
	}
	h := handler(nil, files)
	request := func(h http.Handler, method, path, tag string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		if tag != "" {
			r.Header.Set("If-None-Match", tag)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var oldTag string
	for _, path := range []string{"/archo/player.js", "/archo/archo-s/rig.json", "/archo/archo-s/still.png", "/archo/archo-s/texture.webp"} {
		res := request(h, "GET", path, "")
		tag := res.Header().Get("ETag")
		if res.Code != http.StatusOK || res.Body.Len() == 0 || res.Header().Get("Cache-Control") != "no-cache" || len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
			t.Fatalf("%s: status=%d, headers=%v, body=%s", path, res.Code, res.Header(), res.Body)
		}
		if res.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("asset response lost CSP")
		}
		for _, method := range []string{"GET", "HEAD"} {
			for _, match := range []string{tag, "W/" + tag, `"old", ` + tag, "*"} {
				cached := request(h, method, path, match)
				if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 ||
					cached.Header().Get("ETag") != tag || cached.Header().Get("Cache-Control") != "no-cache" {
					t.Fatalf("%s %s matching %s: status=%d headers=%v body=%s", method, path, match, cached.Code, cached.Header(), cached.Body)
				}
			}
		}
		stale := request(h, "GET", path, `"stale-build"`)
		if stale.Code != http.StatusOK || stale.Body.String() != res.Body.String() {
			t.Fatalf("stale validator did not return the asset: %+v", stale)
		}
		oldTag = tag
	}
	// Both release and commit changes invalidate the previous build's validator.
	for _, change := range []func(){
		func() { buildinfo.Version = "1.0.1" },
		func() { buildinfo.Commit = "def5678" },
	} {
		change()
		res := request(handler(nil, files), "GET", "/archo/player.js", oldTag)
		if res.Code != http.StatusOK || res.Header().Get("ETag") == oldTag || res.Body.Len() == 0 {
			t.Fatalf("new build reused old asset: status=%d headers=%v", res.Code, res.Header())
		}
		oldTag = res.Header().Get("ETag")
	}
	for _, path := range []string{"/", "/js/main.js", "/archo-other/player.js"} {
		res := request(h, "GET", path, oldTag)
		if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "" || res.Header().Get("ETag") != "" {
			t.Fatalf("cache policy leaked to %s: status=%d headers=%v", path, res.Code, res.Header())
		}
	}
	missing := request(h, "GET", "/archo/missing.js", "*")
	if missing.Code != http.StatusNotFound || missing.Header().Get("ETag") != "" {
		t.Fatalf("missing asset: status=%d headers=%v", missing.Code, missing.Header())
	}
}
