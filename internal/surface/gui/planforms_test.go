package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPlanSourcesIncludesImportedHistoryWithoutSwitching(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	id := api.Shell().Plan.ID
	before, _ := os.ReadFile(e.ModsConfig)
	sources, err := api.PlanSources(id)
	if err != nil || sources.Count != 2 || len(sources.Versions) == 0 {
		t.Fatalf("sources: %+v %v", sources, err)
	}
	// The first version comes from importing ModsConfig, not a save/launch/user write.
	version := sources.Versions[len(sources.Versions)-1]
	if !strings.Contains(version.Label, "2 mods") || !strings.Contains(version.Label, ":") {
		t.Fatalf("history needs count and time: %+v", version)
	}
	other, err := api.NewPlan("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.PlanSources(id); err != nil {
		t.Fatal(err)
	}
	if api.Shell().Plan.ID != view.PlanOfPage(other.Page) {
		t.Fatal("reading sources switched the current list")
	}
	if _, err = api.CreatePlan(id, "Historic copy", version.From); err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != "Core,Alpha" {
		t.Fatal("copied a different list/version")
	}
	after, _ := os.ReadFile(e.ModsConfig)
	if string(before) != string(after) {
		t.Fatal("source selection or creation wrote game files")
	}
}

func TestNewListImportUsesOnlyItsPreview(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	id := api.Shell().Plan.ID
	before, _ := os.ReadFile(e.ModsConfig)
	old, err := api.ImportPreviewFor(id, view.ImportText, "tester.a")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := api.ImportPreviewFor(id, view.ImportText, "tester.b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.ImportAsPlanPreview("Old preview", old.Request, false); err == nil {
		t.Fatal("stale preview created a different import")
	}
	if api.Shell().Plan.ID != id {
		t.Fatal("rejected preview changed current list")
	}
	// A request from another game's draft must be rejected even if no newer import ran.
	api.x.envID = "another-game"
	if _, err := api.ImportAsPlanPreview("Wrong game", latest.Request, false); err == nil {
		t.Fatal("cross-game preview accepted")
	}
	env, _ := e.App.Env()
	api.x.envID = env.ID
	done, err := api.ImportAsPlanPreview("My chosen name", latest.Request, false)
	if err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != "Beta" {
		t.Fatal("created content differs from preview")
	}
	managed, err := api.PlanManage(view.PlanOfPage(done.Page))
	if err != nil || managed.Sel.Name != "My chosen name" {
		t.Fatalf("name lost: %+v %v", managed, err)
	}
	if _, err := api.ImportAsPlanPreview("Duplicate submit", latest.Request, false); err == nil {
		t.Fatal("consumed preview accepted twice")
	}
	after, _ := os.ReadFile(e.ModsConfig)
	if string(before) != string(after) {
		t.Fatal("import creation wrote game files")
	}
}

// Management targets the selected list; preview and cancelled/failed creation do not
// switch the editor. Versions/saves and copied version constraints survive (06 §2.17).
func TestPlanForms(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	original := api.Shell().Plan.ID
	e.MustRun("save")
	if _, err := api.PlanVersionsSet(original, []string{"1.5", "1.6"}); err != nil {
		t.Fatal(err)
	}
	save := filepath.Join(e.UserData, "Saves", "colony.rws")
	e.WriteFile(save, `<savegame><meta><gameVersion>1.6.4633</gameVersion><modIds><li>tester.b</li></modIds></meta></savegame>`)
	sources, err := api.PlanSources(original)
	if err != nil || len(sources.Versions) == 0 || len(sources.Saves) != 1 {
		t.Fatalf("sources: %+v %v", sources, err)
	}
	other, err := api.NewPlan("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	otherID := view.PlanOfPage(other.Page)
	preview, err := api.ImportPreviewFor(original, view.ImportText, "ludeon.rimworld\nbrrainz.harmony\ntester.a\ntester.b")
	if err != nil || preview.Replace.Added != 2 || preview.Allowed == "" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if api.Shell().Plan.ID != otherID || len(api.Board().Cols[view.Active]) != 0 {
		t.Fatal("preview switched the editor")
	}
	copy, err := api.CreatePlan(original, "Copy", "current")
	if err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != "Core,Alpha" {
		t.Fatal("copied the current rather than selected list")
	}
	managed, _ := api.PlanManage(view.PlanOfPage(copy.Page))
	if !managed.Sel.Custom || managed.Sel.Allowed != "1.5–1.6" {
		t.Fatalf("copy lost game versions: %+v", managed.Sel)
	}
	if _, err := api.CreatePlan(original, "Copy", "current"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if api.Shell().Plan.ID != view.PlanOfPage(copy.Page) {
		t.Fatal("failed creation changed current list")
	}
	if _, err := api.CreatePlan(original, "From save", save); err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != "Beta" {
		t.Fatal("save not read")
	}
	if _, err := api.ImportPreviewFor(original, view.ImportText, "tester.b"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(e.ModsConfig)
	if _, err := api.ImportApplyTo(original, false); err != nil {
		t.Fatal(err)
	}
	if api.Shell().Plan.ID != original || names(api.Board().Cols[view.Active]) != "Beta" {
		t.Fatal("import targeted wrong list")
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != "Core,Alpha" {
		t.Fatal("import not one undo step")
	}
	after, _ := os.ReadFile(e.ModsConfig)
	if string(before) != string(after) {
		t.Fatal("form wrote game files")
	}
}

// Checkout is an immediate undoable edit through the same API used by enter and the
// history's action; it never writes ModsConfig.xml (06 §2.5, 06 §6).
func TestHistoryCheckoutUndo(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.MustRun("save")
	if _, err := api.ImportPreview(view.ImportText, "tester.b"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ImportApply(false); err != nil {
		t.Fatal(err)
	}
	before := names(api.Board().Cols[view.Active])
	game, _ := os.ReadFile(e.ModsConfig)
	var version int
	history, err := api.History()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range history {
		if h.Kind == "version" {
			version = h.Version
			break
		}
	}
	if version == 0 {
		t.Fatal("no version")
	}
	if _, err := api.HistoryGo(0, version); err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) == before {
		t.Fatal("checkout did not load version")
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	if names(api.Board().Cols[view.Active]) != before {
		t.Fatal("checkout undo lost previous list")
	}
	after, _ := os.ReadFile(e.ModsConfig)
	if string(game) != string(after) {
		t.Fatal("checkout wrote game")
	}
}

func TestImportCollectionForm(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.URL.Path == "/collection" {
			fmt.Fprint(w, `{"response":{"collectiondetails":[{"publishedfileid":"3000000001","result":1,"children":[{"publishedfileid":"1001","sortorder":0,"filetype":0}]}]}}`)
		} else {
			fmt.Fprint(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"3000000001","result":1,"title":"Fixture collection"},{"publishedfileid":"1001","result":1,"title":"Alpha"}]}}`)
		}
	}))
	defer srv.Close()
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"
	id := api.Shell().Plan.ID
	p, err := api.ImportPreviewFor(id, view.ImportText, "https://steamcommunity.com/sharedfiles/filedetails/?id=3000000001")
	if err != nil || !p.Collection {
		t.Fatalf("preview: %+v %v", p, err)
	}
	d, err := api.ImportAsPlan("Collection import")
	if err != nil {
		t.Fatal(err)
	}
	managed, err := api.PlanManage(view.PlanOfPage(d.Page))
	if err != nil || len(managed.Sel.Follows) != 1 {
		t.Fatalf("default follow: %+v %v", managed, err)
	}
}
