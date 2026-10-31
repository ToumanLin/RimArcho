package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLatestStable(t *testing.T) {
	name := ZipName("0.9.1", "windows", "amd64")
	for _, tc := range []struct {
		name    string
		status  int
		missing bool
	}{
		{"complete", 200, false}, {"missing sum", 200, true},
		{"not published", 404, false}, {"server error", 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var base string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/releases/latest" || r.Method != http.MethodGet || r.Header.Get("User-Agent") != "selfupdate-test" {
					t.Errorf("unexpected request: %s %s, ua %q", r.Method, r.URL, r.Header.Get("User-Agent"))
				}
				w.WriteHeader(tc.status)
				assets := []map[string]any{{"name": name, "browser_download_url": base + "/zip", "size": 42}}
				if !tc.missing {
					assets = append(assets, map[string]any{"name": name + ".sha256", "browser_download_url": base + "/sum", "size": 80})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"tag_name": "v0.9.1", "name": "release name", "body": "# Notes\r\n\n**unchanged**",
					"html_url": base + "/page", "published_at": "2026-10-31T00:00:00Z", "assets": assets,
				})
			}))
			defer srv.Close()
			base = srv.URL
			c := Client{API: base, HTTP: srv.Client(), UserAgent: "selfupdate-test", GOOS: "windows", GOARCH: "amd64"}
			r, err := c.Latest(context.Background(), false)
			if tc.status == 404 {
				if !errors.Is(err, ErrNoRelease) {
					t.Fatalf("got %v, want ErrNoRelease", err)
				}
				return
			}
			if tc.status != 200 {
				if err == nil || err.Error() != "release lookup: HTTP 500" {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Version != "0.9.1" || r.Tag != "v0.9.1" || r.Name != "release name" || r.Notes != "# Notes\r\n\n**unchanged**" ||
				r.Page != base+"/page" || !r.Published.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) || r.Prerelease {
				t.Fatalf("wrong release fields: %+v", r)
			}
			if r.Zip != (Asset{Name: name, URL: base + "/zip", Size: 42}) {
				t.Fatalf("wrong zip: %+v", r.Zip)
			}
			if (r.Installable() != nil) != tc.missing {
				t.Fatalf("Installable() = %v", r.Installable())
			}
			if !tc.missing && r.Sum != (Asset{Name: name + ".sha256", URL: base + "/sum", Size: 80}) {
				t.Fatalf("wrong sum: %+v", r.Sum)
			}
		})
	}
	if (&Release{}).Installable() == nil {
		t.Fatal("missing zip accepted")
	}
}

func TestLatestPrerelease(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`[{"tag_name":"9.0.0","draft":true},{"tag_name":"v1.0.0-rc2","prerelease":true},{"tag_name":"0.9.1"},{"tag_name":"nightly"}]`, "1.0.0-rc2"},
		{`[{"tag_name":"1.0.0-rc10","prerelease":true},{"tag_name":"1.0.0"},{"tag_name":"1.0.0-rc2","prerelease":true}]`, "1.0.0"},
		{`[{"tag_name":"dev"},{"tag_name":"9.0.0","draft":true}]`, ""},
		{`[]`, ""},
	} {
		t.Run(tc.want+tc.body[:2], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/releases" || r.URL.RawQuery != "per_page=30" {
					t.Errorf("wrong URL: %s", r.URL)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := Client{API: srv.URL, HTTP: srv.Client()}
			r, err := c.Latest(context.Background(), true)
			if tc.want == "" {
				if !errors.Is(err, ErrNoRelease) {
					t.Fatalf("got %v, want ErrNoRelease", err)
				}
			} else if err != nil || r.Version != tc.want || r.Prerelease != strings.Contains(tc.want, "-") {
				t.Fatalf("got %+v, %v; want %s", r, err, tc.want)
			}
		})
	}
}

func TestLatestInvalidMetadata(t *testing.T) {
	for _, body := range []string{`{`, `{} {}`, strings.Repeat(" ", (1<<20)+1)} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		c := Client{API: srv.URL, HTTP: srv.Client()}
		_, err := c.Latest(context.Background(), false)
		srv.Close()
		if err == nil {
			t.Fatalf("accepted invalid metadata of length %d", len(body))
		}
	}
}

func TestPlatformDefaults(t *testing.T) {
	name := ZipName("1.2.3", runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "assets": []map[string]any{{"name": name, "browser_download_url": "unused"}}})
	}))
	defer server.Close()
	c := Client{API: server.URL}
	r, err := c.Latest(context.Background(), false)
	if err != nil || r.Zip.Name != name {
		t.Fatalf("got %+v, %v", r, err)
	}
	if !reflect.DeepEqual(Files, releaseFiles(runtime.GOOS)) || !reflect.DeepEqual(releaseFiles("linux"), []string{"archo", "README.md", "CHANGELOG.md", "LICENSE", "monitor"}) ||
		!reflect.DeepEqual(releaseFiles("windows"), []string{"archo.exe", "RimArcho.exe", "README.md", "CHANGELOG.md", "LICENSE", "monitor"}) {
		t.Fatal("incorrect platform file lists")
	}
}

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		cmp  int
	}{
		{"0.9.0", "0.9.1", -1}, {"0.10.0", "0.9.9", 1}, {"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0-rc1", "1.0.0-rc2", -1}, {"1.0.0-rc2", "1.0.0-rc10", -1}, {"1.0.0-beta2", "1.0.0-beta10", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1}, {"1.0.0-alpha.1", "1.0.0-beta", -1},
		{"v1.2.3", "1.2.3", 0}, {"1.2.3+build", "1.2.3", 0},
		{"1.0.0-2", "1.0.0-10", -1}, {"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1}, {"1.0.0-rc1+old", "1.0.0-rc1+new", 0},
		{"999999999999999999999999999.0.0", "1000000000000000000000000000.0.0", -1},
		{"1.0.0-rc99999999999999999999999", "1.0.0-rc100000000000000000000000", -1},
	} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			n, ok := Compare(tc.a, tc.b)
			if !ok || n != tc.cmp {
				t.Fatalf("Compare = %d, %v; want %d, true", n, ok, tc.cmp)
			}
			n, ok = Compare(tc.b, tc.a)
			if !ok || n != -tc.cmp || Newer(tc.a, tc.b) != (tc.cmp < 0) {
				t.Fatal("reverse ordering or Newer disagrees")
			}
		})
	}
	for _, bad := range []string{"dev", "", "1.2", "1.2.3.4", "01.2.3", "1.2.3-01", "1.2.3-", "1.2.3+", "1.2.3-a..b", "1.2.3-a_b", " 1.2.3", "V1.2.3"} {
		if _, ok := Compare(bad, "1.0.0"); ok {
			t.Errorf("accepted %q", bad)
		}
		if _, ok := Compare("1.0.0", bad); ok || Newer(bad, "1.0.0") || Newer("1.0.0", bad) {
			t.Errorf("invalid input accepted by Compare/Newer: %q", bad)
		}
	}
}
