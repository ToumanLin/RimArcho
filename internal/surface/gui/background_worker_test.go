package gui

import (
	"archive/zip"
	"bytes"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/menubg"
	"github.com/ToumanLin/RimArcho/internal/view"
	"image"
	"image/color"
	"image/png"
)

func menuGame(t *testing.T, game string) {
	t.Helper()
	dir := filepath.Join(game, "Data/Core/Defs/Misc/ExpansionDefs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defs := `<Defs><ExpansionDef><defName>Core</defName><label>Core</label><backgroundPath>UI/Core</backgroundPath></ExpansionDef><ExpansionDef><defName>Royalty</defName><label>Royalty</label><linkedMod>Ludeon.RimWorld.Royalty</linkedMod><backgroundPath>UI/Royalty</backgroundPath></ExpansionDef></Defs>`
	if err := os.WriteFile(filepath.Join(dir, "ExpansionDefs.xml"), []byte(defs), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../menubg/testdata/bc7.dds")
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, name := range []string{"Core", "Royalty"} {
		f, err := w.Create("Resources/Textures/UI/" + name + ".png.dds")
		if err != nil {
			t.Fatal(err)
		}
		f.Write(raw)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "Assets.zip"), data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func awaitBackgrounds(t *testing.T, events <-chan string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if strings.HasPrefix(event, "backgrounds ") {
				return
			}
		case <-deadline:
			t.Fatal("background extraction did not publish a refresh")
		}
	}
}

func TestMenuBackgroundSettingsAndVersionRefresh(t *testing.T) {
	e, api, events := fixture(t)
	menuGame(t, e.Game)
	old := BgSetting{Key: "game:ludeon.rimworld.royalty", Stars: []string{app.BgDefault, "game:ludeon.rimworld.royalty"}, Dim: 37, Focus: "right"}
	if err := api.SetBackground(old); err != nil {
		t.Fatal(err)
	}
	// The first call schedules the work; the UI and independent API calls can return.
	api.Backgrounds()
	stampResponds(t, api)
	awaitBackgrounds(t, events)
	p := api.Backgrounds()
	items := p.Groups[0].Items
	if len(items) != 3 || items[0].Key != app.BgDefault || items[1].Key != old.Key || items[2].Key != BgPlain {
		t.Fatalf("menu choices: %+v", items)
	}
	if p.Setting.Key != old.Key || len(p.Setting.Stars) != 2 || p.Setting.Stars[1] != old.Key || p.Setting.Dim != 37 || p.Setting.Focus != "right" {
		t.Fatalf("old choice did not survive: %+v", p.Setting)
	}
	bg := api.HomeBackground()
	if bg.URL != items[1].Full {
		t.Fatalf("old DLC selection resolved to %q", bg.URL)
	}
	for _, url := range []string{items[0].Thumb, items[1].Full} {
		res := httptest.NewRecorder()
		api.serveBg(res, httptest.NewRequest("GET", url, nil))
		if res.Code != 200 {
			t.Fatalf("cached menu JPEG: %d", res.Code)
		}
		if _, err := jpeg.DecodeConfig(res.Body); err != nil {
			t.Fatal(err)
		}
	}
	if len(api.Tasks().Rows) != 0 {
		t.Fatal("menu extraction entered the task page")
	}
	// A later settings read repairs a missing cache file without changing the
	// published choices or generating another image-ready event.
	api.bgRead.workers.Wait()
	if err := os.Remove(api.bgs[0].Thumb); err != nil {
		t.Fatal(err)
	}
	api.Backgrounds()
	api.bgRead.workers.Wait()
	if _, err := os.Stat(api.bgs[0].Thumb); err != nil {
		t.Fatal("settings did not repair missing cached thumbnail", err)
	}
	// Reopening settings must not launch another batch or rewrite the cache.
	api.Backgrounds()
	api.HomeBackground()
	select {
	case ev := <-events:
		if strings.HasPrefix(ev, "backgrounds ") {
			t.Fatal("duplicate background refresh")
		}
	default:
	}
	os.Chtimes(filepath.Join(e.Game, "Assets.zip"), time.Unix(100, 0), time.Unix(100, 0))
	api.HomeBackground()
	awaitBackgrounds(t, events)
	if next := api.Backgrounds().Groups[0].Items[1].Full; next == items[1].Full {
		t.Fatal("updated game kept the previous browser image URL")
	}
	res := httptest.NewRecorder()
	api.serveBg(res, httptest.NewRequest("GET", items[1].Full, nil))
	if res.Code != 404 {
		t.Fatal("stale image URL served a different game's pixels")
	}
	// Rebind this environment to an old game without Assets.zip: no previous game's art.
	env, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	env.GameRoot = t.TempDir()
	if err := e.App.Store.UpdateEnv(env); err != nil {
		t.Fatal(err)
	}
	api.HomeBackground()
	api.bgRead.workers.Wait()
	if got := api.HomeBackground().URL; got != "" {
		t.Fatalf("previous game's background leaked: %s", got)
	}
	if got := api.Backgrounds().Groups[0].Items; len(got) != 1 || got[0].Key != BgPlain {
		t.Fatalf("missing assets should fall back to plain: %+v", got)
	}
}

func TestBackgroundEventsOnlyWhenListChanges(t *testing.T) {
	e, api, events := fixture(t)
	noRefresh := func() {
		t.Helper()
		api.bgRead.workers.Wait()
		for {
			select {
			case event := <-events:
				if strings.HasPrefix(event, "backgrounds ") {
					t.Fatal("unchanged list emitted a background refresh")
				}
			default:
				return
			}
		}
	}
	// Discovering an empty list for the first time does not change the plain page.
	api.HomeBackground()
	noRefresh()
	// A new worker key alone is not a change to the empty image list.
	lang := view.Lang()
	t.Cleanup(func() { view.SetLang(lang) })
	if lang == "zh" {
		view.SetLang("en")
	} else {
		view.SetLang("zh")
	}
	api.HomeBackground()
	noRefresh()
	view.SetLang(lang)
	menuGame(t, e.Game)
	api.HomeBackground()
	awaitBackgrounds(t, events)
	api.bgRead.workers.Wait()
	for range 3 {
		api.HomeBackground()
		api.Backgrounds() // recheck assets without changing their published choices
		noRefresh()
	}
}

func TestMenuPreviewMerge(t *testing.T) {
	previews := []app.Background{
		{Key: app.BgDefault, Src: "core-preview.png"},
		{Key: "game:ludeon.rimworld.royalty", Src: "dlc-preview.png"},
		{Key: "mod:background/x", Group: "Background mod", Src: "mod.png"},
	}
	for _, menu := range [][]menubg.Item{nil, {{Key: "game:ludeon.rimworld.royalty", Full: "royalty.jpg", Thumb: "royalty-thumb.jpg"}}} {
		got := mergeMenuBackgrounds(menu, previews)
		if len(got) != 3 || got[0].Key != app.BgDefault || got[2].Group != "Background mod" {
			t.Fatalf("fallback merge: %+v", got)
		}
		if len(menu) > 0 && (got[1].Src != "royalty.jpg" || got[1].Thumb != "royalty-thumb.jpg") {
			t.Fatalf("extracted image lost to preview: %+v", got)
		}
	}
}

func TestMissingAndBrokenAssetsKeepOfficialPreview(t *testing.T) {
	for _, brokenZIP := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-resources", true: "broken-zip"}[brokenZIP], func(t *testing.T) {
			e, api, events := fixture(t)
			path := filepath.Join(e.Game, "Data", "Core", "About", "Preview.png")
			img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
			for y := 0; y < 16; y++ {
				for x := 0; x < 32; x++ {
					img.SetNRGBA(x, y, color.NRGBA{100, 50, 200, 255})
				}
			}
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if brokenZIP {
				if err := os.WriteFile(filepath.Join(e.Game, "Assets.zip"), []byte("broken zip"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			api.HomeBackground()
			awaitBackgrounds(t, events)
			bg := api.HomeBackground()
			if bg.URL == "" {
				t.Fatal("official preview regressed to plain")
			}
			res := httptest.NewRecorder()
			api.serveBg(res, httptest.NewRequest("GET", bg.URL, nil))
			if res.Code != 200 {
				t.Fatalf("preview status %d", res.Code)
			}
			if cfg, err := jpeg.DecodeConfig(res.Body); err != nil || cfg.Width != 32 || cfg.Height != 16 {
				t.Fatalf("preview: %+v %v", cfg, err)
			}
		})
	}
}
