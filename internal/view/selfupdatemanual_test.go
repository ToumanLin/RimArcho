package view_test

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/updatechannel"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The RimArcho row says what each source answered, and once the automatic download
// failed it offers the Lanzou page (with its password) and a picked zip (17 §5.1, 16 §2.14).
func TestSelfUpdateRowSourcesAndManual(t *testing.T) {
	e := testkit.New(t)
	e.App.SelfVersion = "1.0.10"
	record := func(s app.SelfUpdate) {
		b, _ := json.Marshal(s)
		if err := e.App.Store.SetMeta("self-update", string(b)); err != nil {
			t.Fatal(err)
		}
	}
	s := app.SelfUpdate{Checked: e.App.Now().UTC(), State: "new version available", Source: "Lanzou",
		Release: &selfupdate.Release{Version: "1.0.11", Mirror: true, Zip: selfupdate.Asset{Name: selfupdate.ZipName("1.0.11", "windows", "amd64")}},
		Sources: []updatechannel.SourceResult{
			{Source: "github", Status: "failed", Error: "GitHub release lookup failed"},
			{Source: "monitor", Status: "unconfigured"},
			{Source: "lanzou", Status: "ok", Version: "1.0.11"},
		}}
	record(s)
	if got, want := view.SelfUpdateSources(e.App.SelfUpdateInfo()), "GitHub failed  Lanzou 1.0.11"; got != want {
		t.Fatalf("sources %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" {
		return // the install buttons are Windows-only
	}
	it := view.SelfUpdateItem(e.App)
	for _, a := range it.Actions {
		if a.URL != "" || a.Pick != "" {
			t.Fatalf("manual path offered before an automatic download failed: %+v", a)
		}
	}

	s.Manual = "1.0.11"
	record(s)
	it = view.SelfUpdateItem(e.App)
	var page, pick bool
	for _, a := range it.Actions {
		switch {
		case a.URL != "":
			page = a.URL == lanzou.DefaultShareURL && a.Copy == lanzou.DefaultPassword
		case a.Pick == "zip":
			pick = len(a.Argv) == 3 && a.Argv[1] == "--from" && a.Argv[2] == "{}" && a.Task == view.TaskSelfUpdate && a.Confirm != ""
		}
	}
	if !page || !pick || it.Actions[0].Label != "try again" {
		t.Fatalf("manual actions %+v", it.Actions)
	}

	// A newer check result is a new chance for the automatic download.
	s.Manual = "1.0.10"
	record(s)
	for _, a := range view.SelfUpdateItem(e.App).Actions {
		if a.URL != "" {
			t.Fatal("manual path offered for an older failed version")
		}
	}
}
