package app_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/textures"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestLinuxToddsNeverDownloads(t *testing.T) {
	e := testkit.New(t)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer srv.Close()
	e.App.ToddsZip = srv.URL
	e.App.ToddsNoPath = true
	if offer := e.App.OfferTodds(); offer.URL != "" || offer.Bytes != 0 {
		t.Fatalf("download offered: %+v", offer)
	}
	if _, err := e.App.InstallTodds(); !errors.Is(err, app.ErrNoTodds) {
		t.Fatalf("install: %v", err)
	}
	if _, err := e.App.InstallDep(deps.Todds, nil); !errors.Is(err, app.ErrNoTodds) {
		t.Fatalf("dependency install: %v", err)
	}
	if _, err := e.App.OfferDep(deps.Todds); !errors.Is(err, app.ErrNoTodds) {
		t.Fatalf("dependency offer: %v", err)
	}
	if requests != 0 {
		t.Fatalf("made %d network requests", requests)
	}
	if info := e.App.Deps().Status(deps.Todds); info.Installable || info.OK || !strings.Contains(info.Err, "PATH") {
		t.Fatalf("missing guidance: %+v", info)
	}
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		page := view.TexturePageInfo(e.App, app.TexPlan, 100, e.App.TextureTool(), 0)
		if page.Offer != "" || page.ToolOK || page.Tool == "" || !strings.Contains(page.Tool, "textures.todds") {
			t.Fatalf("%s: %+v", lang, page)
		}
		if lang == "en" && !strings.Contains(page.Tool, textures.ToddsMissing) {
			t.Fatal(page.Tool)
		}
		if lang == "zh" && strings.Contains(page.Tool, "Texture optimisation needs") {
			t.Fatal("missing translation")
		}
	}
	view.SetLang("en")
	output := e.MustRun("textures", "tool")
	if strings.Contains(output, "--install") || strings.Contains(output, "download:") || !strings.Contains(output, "PATH") {
		t.Fatal(output)
	}
}
