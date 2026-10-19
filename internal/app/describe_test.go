package app_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// fakeSteam answers GetPublishedFileDetails for any id, and serves /img.png (a 1500×500
// image) and /bad.png (not an image).
func fakeSteam(t *testing.T, desc string, updated int64) (*httptest.Server, *int32, *int32) {
	var api, imgs int32
	png1500 := func() []byte {
		img := image.NewRGBA(image.Rect(0, 0, 1500, 500))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		img.Set(0, 0, color.RGBA{1, 2, 3, 255})
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes()
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			atomic.AddInt32(&api, 1)
			r.ParseForm()
			fmt.Fprintf(w, `{"response":{"result":1,"resultcount":1,"publishedfiledetails":[{"publishedfileid":%q,"result":1,"title":"T","description":%q,"time_updated":%d}]}}`,
				r.Form.Get("publishedfileids[0]"), desc, updated)
		case "/img.png":
			atomic.AddInt32(&imgs, 1)
			w.Write(png1500)
		case "/bad.png":
			w.Write([]byte("not a png"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &api, &imgs
}

func TestDescribe(t *testing.T) {
	e := testkit.New(t)
	short := testkit.Sample("tester.short", "Short About")
	dir := e.AddWorkshop("1001", short)
	e.WriteFile(dir+`\About\About.xml`, strings.Replace(short.AboutXML(), "</ModMetaData>", "<description>Adds a thing.</description></ModMetaData>", 1))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	ws := "[h1]Short About[/h1]\n[img]https://example.invalid/x.png[/img]\n" + strings.Repeat("A long Workshop description. ", 20)
	srv, api, imgs := fakeSteam(t, ws, 1790000000)
	e.App.SteamAPI = srv.URL + "/api"
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:1001")
	if err != nil {
		t.Fatal(err)
	}
	// Without fetch: nothing cached yet.
	d := e.App.Describe(ix, m, false, false)
	if d.WorkshopState != "not cached" || d.Show != "about" || d.About != "Adds a thing." || *api != 0 {
		t.Fatalf("no fetch: %+v, api %d", d, *api)
	}
	d = e.App.Describe(ix, m, true, false)
	if d.WorkshopState != "fetched" || d.Show != "workshop" || !strings.Contains(d.Workshop, "A long Workshop description") || *api != 1 {
		t.Fatalf("fetch: %+v, api %d", d, *api)
	}
	// The cache is fresh: its time_updated is the ACF's.
	d = e.App.Describe(ix, m, true, false)
	if d.WorkshopState != "cached" || *api != 1 {
		t.Fatalf("cached: %s, api %d", d.WorkshopState, *api)
	}
	d = e.App.Describe(ix, m, true, true) // refresh
	if d.WorkshopState != "fetched" || *api != 2 {
		t.Fatalf("refresh: %s, api %d", d.WorkshopState, *api)
	}
	// Offline: the cache still answers; nothing is fetched.
	e.MustRun("config", "set", "workshop.fetch", "off")
	d = e.App.Describe(ix, m, true, true)
	if d.WorkshopState != "cached" || d.Workshop == "" || *api != 2 {
		t.Fatalf("offline with cache: %s, api %d", d.WorkshopState, *api)
	}
	if _, _, _, err := e.App.DescImage(srv.URL + "/img.png"); err == nil || *imgs != 0 {
		t.Fatalf("image downloaded while offline: %v", err)
	}
	e.MustRun("config", "set", "workshop.fetch", "on")
	// Images: scaled to 720 wide, cached.
	p, w, h, err := e.App.DescImage(srv.URL + "/img.png")
	if err != nil || w != 720 || h != 240 || *imgs != 1 {
		t.Fatalf("image: %s %dx%d %v, downloads %d", p, w, h, err, *imgs)
	}
	if _, _, _, err := e.App.DescImage(srv.URL + "/img.png"); err != nil || *imgs != 1 {
		t.Fatalf("cached image: %v, downloads %d", err, *imgs)
	}
	e.MustRun("config", "set", "workshop.images", "off")
	if _, _, _, err := e.App.DescImage(srv.URL + "/img.png"); err != nil {
		t.Fatalf("a cached image is shown with downloads off: %v", err)
	}
	e.MustRun("config", "set", "workshop.images", "on")
	for _, bad := range []string{srv.URL + "/bad.png", "file:///C:/Windows/win.ini", "javascript:x", srv.URL + "/missing.png"} {
		if _, _, _, err := e.App.DescImage(bad); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
	// The CLI.
	out := e.MustRun("describe", "ws:1001", "--offline")
	for _, want := range []string{"workshop: 1001 (cached", "Workshop page:", "[image: ", "A long Workshop description"} {
		if !strings.Contains(out, want) {
			t.Fatalf("describe output lacks %q:\n%s", want, out)
		}
	}
	if out := e.MustRun("describe", "ws:1001", "--offline", "--source", "both"); !strings.Contains(out, "About.xml:") {
		t.Fatalf("--source both:\n%s", out)
	}
	// A local mod has no Workshop page.
	e.WriteMod(e.Game+`\Mods\Loc`, testkit.Sample("tester.loc", "Local One"))
	e.MustRun("scan")
	if out := e.MustRun("describe", "pkg:tester.loc"); !strings.Contains(out, "not a workshop mod") || *api != 2 {
		t.Fatalf("local mod:\n%s", out)
	}
}

func TestDescribeChoice(t *testing.T) {
	e := testkit.New(t)
	long := strings.Repeat("About text that is long enough. ", 15)
	m := testkit.Sample("tester.long", "Long About")
	e.AddWorkshop("2002", m)
	dir := e.Workshop + `\2002\About\About.xml`
	e.WriteFile(dir, strings.Replace(m.AboutXML(), "</ModMetaData>", "<description>"+long+"</description></ModMetaData>", 1))
	e.AddEnv(false)
	e.MustRun("scan")
	srv, _, _ := fakeSteam(t, "Something else entirely. "+strings.Repeat("Workshop words. ", 30), 1790000000)
	e.App.SteamAPI = srv.URL + "/api"
	ix, _ := e.App.Index()
	mod, _ := ix.ResolveOne("ws:2002")
	if d := e.App.Describe(ix, mod, true, false); d.Show != "both" {
		t.Fatalf("both long and different: show %s", d.Show)
	}
	srv2, _, _ := fakeSteam(t, long+"[img]x[/img] more", 1790000000)
	e.App.SteamAPI = srv2.URL + "/api"
	if d := e.App.Describe(ix, mod, true, true); d.Show != "workshop" {
		t.Fatalf("same start: show %s", d.Show)
	}
}
