//go:build windows || cgo

package gui

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func looksPicture(t *testing.T, w, h int, shade uint8) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{shade, uint8(x), uint8(y), uint8(128 + x%2*127)})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// looksDDS is a 8×4 DDS: every block red, except the first row of blocks under BC3 has
// alpha 200. fourCC "DX10" gets the extended header and no decodable data.
func looksDDS(fourCC string) string {
	b := make([]byte, 128)
	copy(b, "DDS ")
	le := binary.LittleEndian
	le.PutUint32(b[4:], 124)
	le.PutUint32(b[12:], 4)
	le.PutUint32(b[16:], 8)
	le.PutUint32(b[76:], 32)
	le.PutUint32(b[80:], 0x4)
	copy(b[84:], fourCC)
	block := []byte{0x00, 0xF8, 0x1F, 0x00, 0, 0, 0, 0} // c0 red, c1 blue, every pixel c0
	for i := 0; i < 2; i++ {
		switch fourCC {
		case "DXT1":
			b = append(b, block...)
		case "DXT5":
			b = append(append(b, 200, 100, 0, 0, 0, 0, 0, 0), block...)
		default:
			b = append(b, make([]byte, 20)...)
		}
	}
	return string(b)
}

// looksFixture: Core, Alpha, Bravo, Charlie. Alpha and Bravo ship the gun (south, north);
// Alpha also a large texture, DDS files in BC1, BC3 and BC7, and a BC7 with its PNG twin.
func looksFixture(t *testing.T) (*testkit.Env, *API) {
	t.Helper()
	view.SetLang("en")
	e := testkit.New(t)
	e.AddEnv(false)
	alpha := testkit.Mod{PackageID: "looks.a", Name: "Alpha", Files: map[string]string{
		"Textures/Things/Item/Gun_south.png": looksPicture(t, 2, 2, 10),
		"Textures/Things/Item/Gun_north.png": looksPicture(t, 2, 2, 11),
		"Textures/Things/Big/Wall.png":       looksPicture(t, 300, 150, 12),
		"Textures/Things/Dds/One.dds":        looksDDS("DXT1"),
		"Textures/Things/Dds/Five.dds":       looksDDS("DXT5"),
		"Textures/Things/Dds/Seven.dds":      looksDDS("DX10"),
		"Textures/Things/Dds/Twin.dds":       looksDDS("DX10"),
		"Textures/Things/Dds/Twin.png":       looksPicture(t, 4, 4, 13),
		"About/Secret.png":                   looksPicture(t, 2, 2, 99),
	}}
	e.AddWorkshop("1001", alpha)
	e.AddWorkshop("1002", testkit.Mod{PackageID: "looks.b", Name: "Bravo", Files: map[string]string{
		"Textures/Things/Item/Gun_south.png": looksPicture(t, 2, 2, 60),
		"Textures/Things/Item/Gun_north.png": looksPicture(t, 2, 2, 61),
	}})
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), testkit.Mod{PackageID: "looks.c", Name: "Charlie", Files: map[string]string{
		"Textures/Things/Item/Gun_south.png": looksPicture(t, 2, 2, 120),
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b", "looks.c")
	e.MustRun("scan")
	api := NewAPI(e.App, func(string, any) {})
	t.Cleanup(api.Close)
	return e, api
}

func TestLooksIndexIsBuiltOncePerPlanState(t *testing.T) {
	e, api := looksFixture(t)
	o, err := api.LooksOverview()
	if err != nil {
		t.Fatal(err)
	}
	if o.Looks != 6 || len(o.Mods) != 3 || o.Mods[0].Name != "Alpha" || o.Mods[0].Looks != 6 || o.Mods[0].Overridden != 1 {
		t.Fatalf("overview %+v", o)
	}
	flight := "LooksIndex|" + o.Stamp
	idx, key := api.looks.current()
	if idx == nil || key != o.Stamp || api.flight.started(flight) != 1 {
		t.Fatalf("index kept under %q (stamp %q), %d builds", key, o.Stamp, api.flight.started(flight))
	}
	if p := api.LooksProgress(); p.Building || p.Total != 4 || p.Done != 4 {
		t.Fatalf("progress after the build %+v", p)
	}
	g, err := api.LooksGallery("1", "", "gun")
	if err != nil || g.Rows != 1 || g.Stamp != o.Stamp {
		t.Fatalf("gallery %+v %v", g, err)
	}
	d, err := api.LookDetail("things/item/gun", 1, "plan")
	if err != nil || d.Targets[0].Preview == nil || d.Targets[0].Preview.Outcome != view.LookWouldApply {
		t.Fatalf("detail %+v %v", d, err)
	}
	if p, err := api.LookPreview("things/item/gun", 1, []string{"south"}, "plan"); err != nil || p.Outcome != view.LookWouldMixed {
		t.Fatalf("preview %+v %v", p, err)
	}
	if again, _ := api.looks.current(); again != idx || api.flight.started(flight) != 1 {
		t.Fatal("a read built the index again")
	}

	// the confirmation's dry run writes nothing and keeps the board
	api.Board()
	dry, err := api.LooksApplyPreview([]string{"things/item/gun"}, o.Mods[0].Ref, nil, "plan")
	if err != nil || !dry.DryRun || !dry.Added || dry.Files != 2 || !dry.DirChosen {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	api.mu.Lock()
	board := api.board
	api.mu.Unlock()
	if board == nil {
		t.Fatal("the dry run dropped the board")
	}
	if now, _ := api.LooksOverview(); now.Chosen != 0 || now.Generated.Entry != 0 {
		t.Fatalf("the dry run wrote: %+v", now)
	}

	// an apply: the board goes (the plan gained _Archo_Looks), the choices are read again,
	// and the index stays: _Archo_Looks is no provider, so the plan's textures are the same
	done, err := api.LooksApply("things/item/gun", o.Mods[0].Ref, nil, "plan")
	if err != nil || !done.Added || done.Overview == nil || done.Overview.Chosen != 1 || done.Text != "the look gun now comes from Alpha" {
		t.Fatalf("apply %+v %v", done, err)
	}
	api.mu.Lock()
	board = api.board
	api.mu.Unlock()
	if board != nil {
		t.Fatal("the apply kept the board")
	}
	after, err := api.LooksOverview()
	if err != nil || after.Chosen != 1 || after.Generated.Entry != 5 || after.Generated.Applied != 1 || after.Generated.Files != 2 || after.Stamp != o.Stamp {
		t.Fatalf("overview after the apply %+v %v", after, err)
	}
	if last := after.Mods[len(after.Mods)-1]; !last.Generated || last.Looks != 1 {
		t.Fatalf("the generated Mod's row %+v", last)
	}
	if again, _ := api.looks.current(); again != idx {
		t.Fatal("the apply dropped an index that is still right")
	}
	if b := api.Board(); len(b.Cols[view.Active]) != 5 {
		t.Fatalf("the board after the apply has %d active rows", len(b.Cols[view.Active]))
	}
	if d, _ := api.LookDetail("things/item/gun", 1, "plan"); !d.CanUndo || d.Slots[0].InGame != 1 || !d.Slots[0].ByChoice {
		t.Fatalf("detail after the apply %+v", d)
	}
	// a choice written by another interface (the CLI) shows at the next read
	e.MustRun("looks", "undo", "things/item/gun")
	if now, _ := api.LooksOverview(); now.Chosen != 0 {
		t.Fatalf("the state was not resolved again: %+v", now)
	}
	if many, err := api.LooksApplyMany([]string{"things/item/gun", "things/big/wall"}, o.Mods[1].Ref, "all"); err != nil || many.Done != 1 || many.Items[1].Error == "" {
		t.Fatalf("apply many %+v %v", many, err)
	}
	if u, err := api.LooksUndoMany([]string{"things/item/gun", "things/big/wall"}, "all"); err != nil || u.Done != 1 || u.Overview.Chosen != 0 {
		t.Fatalf("undo many %+v %v", u, err)
	}
	if _, err := api.LooksUndo("things/item/gun", "all"); err == nil {
		t.Fatal("undo without a choice succeeded")
	}
	if r, err := api.LooksRebuild(); err != nil || r.Files != 0 {
		t.Fatalf("rebuild %+v %v", r, err)
	}

	// the plan changes: another key, another index
	e.MustRun("disable", "pkg:looks.c")
	changed, err := api.LooksOverview()
	if err != nil || changed.Stamp == o.Stamp || len(changed.Mods) != 3 || changed.Mods[2].Name != "_Archo_Looks" {
		t.Fatalf("overview after the plan changed %+v %v", changed, err)
	}
	if again, key := api.looks.current(); again == idx || key != changed.Stamp || api.flight.started("LooksIndex|"+changed.Stamp) != 1 {
		t.Fatal("the index was kept across a plan change")
	}
	// refresh reads the folders again
	idx, _ = api.looks.current()
	if r, err := api.LooksRefresh(); err != nil || r.Stamp != changed.Stamp {
		t.Fatalf("refresh %+v %v", r, err)
	}
	if again, _ := api.looks.current(); again == idx || api.flight.started("LooksIndex|"+changed.Stamp) != 2 {
		t.Fatal("refresh kept the index")
	}
	// an index nothing read for readerIdle goes; a recent read keeps it
	api.looks.expire()
	if kept, _ := api.looks.current(); kept == nil {
		t.Fatal("the expiry dropped an index that was just read")
	}
	api.looks.mu.Lock()
	api.looks.use.last = api.looks.use.last.Add(-2 * readerIdle)
	api.looks.mu.Unlock()
	api.looks.expire()
	api.looks.mu.Lock()
	gone := api.looks.idx == nil
	api.looks.mu.Unlock()
	if !gone {
		t.Fatal("an idle index was kept")
	}
	if r, err := api.LooksOverview(); err != nil || r.Stamp != changed.Stamp || api.flight.started("LooksIndex|"+changed.Stamp) != 3 {
		t.Fatalf("the read after the expiry: %+v %v", r, err)
	}
	if err := api.LookShow("things/item/gun", 1, "south"); err != nil {
		t.Fatal(err)
	}
	if err := api.LookShow("things/item/gun", 1, "../../About"); err == nil {
		t.Fatal("a slot that is not there was shown")
	}
	// leaving the view drops the index
	api.LooksLeave()
	if left, _ := api.looks.current(); left != nil {
		t.Fatal("the index stayed after the page left the view")
	}
	// the page keeps its choices with Setting / SetSetting
	for _, k := range []string{view.PlanViewKey, view.LooksScopeKey, view.LooksConfirmedKey} {
		if err := api.SetSetting(k, "all"); err != nil || api.Setting(k) != "all" {
			t.Fatalf("setting %s: %v", k, err)
		}
	}
	api.SetSetting(view.LooksConfirmedKey, "1")
	if now, _ := api.LooksOverview(); now.Scope != "all" || !now.Confirmed {
		t.Fatalf("overview does not carry the settings: %+v", now)
	}
}

func TestLookTexRoute(t *testing.T) {
	e, api := looksFixture(t)
	h := handler(api, fstest.MapFS{})
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	// no index in memory yet: nothing is served, and nothing is built for a picture
	if rec := get("/looktex/things/item/gun?p=1&s=south&w=96"); rec.Code != 404 {
		t.Fatalf("before the index: %d", rec.Code)
	}
	if idx, _ := api.looks.current(); idx != nil {
		t.Fatal("a picture request built the index")
	}
	o, err := api.LooksOverview()
	if err != nil {
		t.Fatal(err)
	}
	v := "&v=" + o.Stamp
	cache := filepath.Join(e.App.Home, "cache", "looktex")
	cached := func() []string {
		t.Helper()
		names, _ := filepath.Glob(filepath.Join(cache, "*"))
		return names
	}

	// a small PNG is served as the Mod ships it, and nothing is cached for it
	rec := get("/looktex/things/item/gun?p=1&s=south&w=96" + v)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "max-age=31536000, immutable" {
		t.Fatalf("gun: %d %v", rec.Code, rec.Header())
	}
	if rec.Body.String() != looksPicture(t, 2, 2, 10) || len(cached()) != 0 {
		t.Fatalf("the small texture was not served as it is (%d cached)", len(cached()))
	}
	if other := get("/looktex/things/item/gun?p=2&s=south&w=96" + v); other.Code != 200 || other.Body.String() != looksPicture(t, 2, 2, 60) {
		t.Fatalf("Bravo's gun: %d", other.Code)
	}
	if rec := get("/looktex/things/item/gun?p=1&s=south&w=96"); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("without the stamp: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}

	// a large one is scaled to fit the box, keeps its alpha, and is cached once
	rec = get("/looktex/things/big/wall?p=1&s=single&w=96" + v)
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if rec.Code != 200 || err != nil || img.Bounds().Dx() != 96 || img.Bounds().Dy() != 48 {
		t.Fatalf("thumbnail: %d %v %v", rec.Code, err, img)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0xFFFF {
		t.Fatal("the thumbnail lost its alpha")
	}
	files := cached()
	if len(files) != 1 || !strings.HasSuffix(files[0], "-96.png") {
		t.Fatalf("cache: %v", files)
	}
	first, _ := os.Stat(files[0])
	if again := get("/looktex/things/big/wall?p=1&s=single&w=96" + v); again.Code != 200 || !bytes.Equal(again.Body.Bytes(), rec.Body.Bytes()) {
		t.Fatal("the cached thumbnail differs")
	}
	if second, _ := os.Stat(files[0]); !second.ModTime().Equal(first.ModTime()) || len(cached()) != 1 {
		t.Fatal("the thumbnail was made twice")
	}
	if rec := get("/looktex/things/big/wall?p=1&s=single&w=192" + v); rec.Code != 200 || len(cached()) != 2 {
		t.Fatalf("192: %d, %d cached", rec.Code, len(cached()))
	}
	if rec := get("/looktex/things/big/wall?p=1&s=single&w=0" + v); rec.Code != 200 || rec.Body.String() != looksPicture(t, 300, 150, 12) || len(cached()) != 2 {
		t.Fatalf("the original: %d", rec.Code)
	}

	// DDS: BC1 and BC3 are decoded, also for the original; BC7 has no preview
	for _, c := range []struct {
		look  string
		alpha uint32
	}{{"one", 0xFFFF}, {"five", 200 * 0x101}} {
		rec := get("/looktex/things/dds/" + c.look + "?p=1&s=single&w=0" + v)
		img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || err != nil || img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
			t.Fatalf("%s: %d %v", c.look, rec.Code, err)
		}
		if r, g, b, a := img.At(5, 2).RGBA(); a != c.alpha || g != 0 || b != 0 || r == 0 {
			t.Fatalf("%s pixel: %x %x %x %x", c.look, r, g, b, a)
		}
	}
	if rec := get("/looktex/things/dds/seven?p=1&s=single&w=96" + v); rec.Code != 404 {
		t.Fatalf("BC7: %d", rec.Code)
	}
	if rec := get("/looktex/things/dds/twin?p=1&s=single&w=96" + v); rec.Code != 200 || rec.Body.String() != looksPicture(t, 4, 4, 13) {
		t.Fatalf("the PNG twin of a BC7: %d", rec.Code)
	}
	g, err := api.LooksGallery("1", "", "things/dds")
	if err != nil || g.Rows != 4 {
		t.Fatalf("gallery %+v %v", g, err)
	}
	thumbs, _ := json.Marshal(g.Groups[0].Rows)
	if !strings.Contains(string(thumbs), `"id":"things/dds/seven","kind":"plain","title":"seven","n":1,"inGame":true,"noDef":true,"thumbs":[{"p":1,"s":"single","noPreview":"dds:DX10"}]`) ||
		strings.Count(string(thumbs), "noPreview") != 1 {
		t.Fatalf("gallery rows %s", thumbs)
	}
	d, err := api.LookDetail("things/dds/seven", 1, "plan")
	if err != nil || d.Slots[0].Chain[0].File.Preview || d.Slots[0].Chain[0].File.Format != "dds:DX10" {
		t.Fatalf("BC7 detail %+v %v", d, err)
	}
	if d, _ := api.LookDetail("things/dds/twin", 1, "plan"); !d.Slots[0].Chain[0].File.Preview || d.Slots[0].Chain[0].File.Alt != "dds:DX10" {
		t.Fatalf("twin detail %+v", d.Slots[0].Chain[0])
	}

	// only a look, provider and slot of the index is served; nothing of the request is a path
	before := len(cached())
	for _, path := range []string{
		"/looktex/things/item/nothing?p=1&s=south&w=96",
		"/looktex/things/item/gun?p=0&s=south&w=96",  // Core supplies no texture
		"/looktex/things/item/gun?p=3&s=north&w=96",  // Charlie has no north
		"/looktex/things/item/gun?p=9&s=south&w=96",  // no such position
		"/looktex/things/item/gun?p=1&s=east&w=96",   // no such slot
		"/looktex/things/item/gun?p=1&s=south&w=100", // no such width
		"/looktex/things/item/gun?p=1&s=south&w=-1",
		"/looktex/things/item/gun?p=x&s=south&w=96",
		"/looktex/things/item/gun?s=south&w=96",
		"/looktex/things/item/gun?p=1&s=south&w=96&v=stale",
		"/looktex/Things/Item/Gun_south.png?p=1&s=south&w=0",
		"/looktex/?p=1&s=south&w=0",
		"/looktex/things/item/gun?p=1&s=" + url.QueryEscape("../../../About/Secret.png") + "&w=0",
		"/looktex/things/item/gun?p=1&s=" + url.QueryEscape(`..\..\..\About\Secret.png`) + "&w=0",
		"/looktex/" + url.PathEscape("../../About/Secret.png") + "?p=1&s=single&w=0",
		"/looktex/things/item/../../../About/Secret?p=1&s=single&w=0",
		"/looktex/" + url.PathEscape(filepath.Join(e.Workshop, "1001", "About", "Secret.png")) + "?p=1&s=single&w=0",
	} {
		rec := get(path)
		if rec.Code == 200 || strings.Contains(rec.Body.String(), "PNG") {
			t.Errorf("%s: %d, served %d bytes", path, rec.Code, rec.Body.Len())
		}
		if loc := rec.Header().Get("Location"); rec.Code != 404 && (rec.Code/100 != 3 || !strings.HasPrefix(loc, "/")) {
			t.Errorf("%s: %d %q", path, rec.Code, loc)
		}
	}
	if len(cached()) != before {
		t.Fatal("a refused request wrote into the cache")
	}
}
