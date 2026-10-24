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
	awaitBackgrounds(t, events)
	if got := api.HomeBackground().URL; got != "" {
		t.Fatalf("previous game's background leaked: %s", got)
	}
	if got := api.Backgrounds().Groups[0].Items; len(got) != 1 || got[0].Key != BgPlain {
		t.Fatalf("missing assets should fall back to plain: %+v", got)
	}
}
