package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// X4: a collection with an installed mod, one that is not installed and a nested
// collection; sortorder has gaps. --import keeps the installed ones in order.
func TestCollection(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/collection":
			if r.Form.Get("publishedfileids[0]") != "3000000001" {
				w.Write([]byte(`{"response":{"result":1,"resultcount":0,"collectiondetails":[{"publishedfileid":"` + r.Form.Get("publishedfileids[0]") + `","result":9}]}}`))
				return
			}
			w.Write([]byte(`{"response":{"result":1,"resultcount":1,"collectiondetails":[{"publishedfileid":"3000000001","result":1,"children":[
{"publishedfileid":"1001","sortorder":5,"filetype":0},
{"publishedfileid":"2009463077","sortorder":1,"filetype":0},
{"publishedfileid":"5550001","sortorder":12,"filetype":0},
{"publishedfileid":"3000000002","sortorder":16,"filetype":2}]}]}}`))
		case "/api":
			var b strings.Builder
			b.WriteString(`{"response":{"publishedfiledetails":[`)
			for i := 0; ; i++ {
				id := r.Form.Get("publishedfileids[" + string(rune('0'+i)) + "]")
				if id == "" {
					break
				}
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`{"publishedfileid":"` + id + `","result":1,"title":"Title ` + id + `"}`)
			}
			b.WriteString(`]}}`)
			w.Write([]byte(b.String()))
		}
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"

	out := e.MustRun("collection", "https://steamcommunity.com/sharedfiles/filedetails/?id=3000000001")
	for _, want := range []string{"collection Title 3000000001: 4 items", "installed: 2", "not installed: 1", "Title 5550001", "nested collections (not expanded): 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, "Harmony"), strings.Index(out, " A\n"); i < 0 || j < 0 || i > j {
		t.Fatalf("installed items not in sortorder:\n%s", out)
	}
	out = e.MustRun("collection", "3000000001", "--import")
	if got := joined(active(t, e)); got != "Harmony, A" || !strings.Contains(out, "not installed (subscribe in the Workshop): 1") {
		t.Fatalf("import: %s\n%s", got, out)
	}
	if out, code := e.Run("collection", "1001"); code == 0 || !strings.Contains(out, "not a Workshop collection") {
		t.Fatalf("not a collection: exit %d\n%s", code, out)
	}
}
