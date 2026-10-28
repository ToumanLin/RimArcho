package gui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The sidebar's badge is the board's needs count, and every page has its breadcrumb (G11).
func TestShellAndHome(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	b := api.Board()
	sh := api.Shell()
	if sh.Envs != 1 || sh.Env != "main  1.6" {
		t.Fatalf("shell %+v", sh)
	}
	items := sh.Nav[0].Items
	if items[1].ID != "needs" || items[1].Badge != b.Needs || b.Needs == 0 {
		t.Fatalf("badge %d, board %d", items[1].Badge, b.Needs)
	}
	for _, id := range []string{"home", "needs", "plan", "packs", "env", "library", "changes", "log", "settings"} {
		if len(sh.Crumbs[id]) == 0 {
			t.Errorf("no breadcrumb for %s", id)
		}
	}
	h := api.Home()
	if len(h.Rows) != 1 || h.Rows[0].Kind != view.RowPlan || h.Rows[0].Env != sh.Env {
		t.Fatalf("home %+v", h)
	}
}

// G15 in part: the guide finds the fixture's game, adds it as an environment and scans it.
func TestGuide(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	events := make(chan map[string]any, 16)
	added := make(chan *EnvironmentResult, 1)
	api := NewAPI(e.App, func(name string, data any) {
		if name == "environment" {
			added <- data.(*EnvironmentResult)
		}
		if name != "guide" {
			return
		}
		b, _ := json.Marshal(data)
		var m map[string]any
		json.Unmarshal(b, &m)
		events <- m
	})
	t.Cleanup(api.Close)
	api.ScanRunner = func(env string) (string, error) {
		e.App.EnvName = env // the child gets it as --env
		out, code := cmds.Execute(e.App, []string{"scan"})
		if code != cmds.ExitOK {
			t.Errorf("scan: %s", out)
		}
		return out, nil
	}
	if sh := api.Shell(); sh.Envs != 0 {
		t.Fatalf("envs %d", sh.Envs)
	}
	found := api.GuideFind()
	if len(found) != 1 || found[0].Name != "1.6" {
		t.Fatalf("found %+v", found)
	}
	if !api.GuideAdd(1, found) {
		t.Fatal("add did not start")
	}
	var result *EnvironmentResult
	select {
	case result = <-added:
	case <-time.After(30 * time.Second):
		t.Fatal("no add result")
	}
	names := result.Names
	if result.Error != "" || strings.Join(names, ",") != "1.6" {
		t.Fatalf("added %+v", result)
	}
	current, _ := e.App.Env()
	if scan, _, err := e.App.LastScan(current.ID); err != nil || scan != nil {
		t.Fatalf("GuideAdd scanned before GuideScan: %+v %v", scan, err)
	}
	if !api.GuideScan(names) {
		t.Fatal("scan did not start")
	}
	var last map[string]any
	var texts []string
	timeout := time.After(30 * time.Second)
	for last == nil || last["done"] != true {
		select {
		case last = <-events:
			if s, ok := last["text"].(string); ok {
				texts = append(texts, s)
			}
		case <-timeout:
			t.Fatal("no end of the scan")
		}
	}
	if last["ok"] != true || len(texts) != 2 || !strings.Contains(texts[1], "mods found") {
		t.Fatalf("end %v, texts %q", last, texts)
	}
	if b := api.Board(); b.Never || b.EnvName != "1.6" {
		t.Fatalf("board after the guide: never %v env %s", b.Never, b.EnvName)
	}
	if _, err := api.PackImport(e.Root); err == nil {
		t.Fatal("a folder was imported as a pack")
	}
}

// Plans in the window (06 §2.1, §2.5): each plan is a sidebar entry with its breadcrumb;
// opening one switches the board; new, import-as-plan, rename and delete go through view.
func TestPlansInWindow(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	b := api.Board()
	enabled := len(b.Cols[view.Active])
	d, err := api.NewPlan("Empty", "")
	if err != nil {
		t.Fatal(err)
	}
	sh := api.Shell()
	if len(sh.Plans) != 2 || sh.Plan == nil || sh.Plan.Name != "Empty" || d.Page != sh.Plan.Page {
		t.Fatalf("shell plans %+v, done %+v", sh.Plans, d)
	}
	if got := sh.Crumbs[d.Page]; len(got) != 3 || got[2].Label != "Empty" || got[1].ID != "plan" {
		t.Fatalf("crumbs %+v", got)
	}
	if len(api.Board().Cols[view.Active]) != 0 {
		t.Fatal("the board still shows the old plan")
	}
	if h := api.Home(); len(h.Rows) != 2 {
		t.Fatalf("home rows %+v", h.Rows)
	}
	def := sh.Plans[0]
	if _, err := api.UsePlan(def.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(api.Board().Cols[view.Active]); n != enabled {
		t.Fatalf("back on default: %d enabled, want %d", n, enabled)
	}
	// The import being previewed becomes a plan of its own.
	if _, err := api.ImportPreview("text", "tester.b\nludeon.rimworld"); err != nil {
		t.Fatal(err)
	}
	d, err = api.ImportAsPlan("Pasted")
	if err != nil || !strings.HasPrefix(d.Text, "new list Pasted: 2 mods") {
		t.Fatalf("import as plan %+v %v", d, err)
	}
	if names(api.Board().Cols[view.Active]) != "Beta,Core" {
		t.Fatalf("imported list %s", names(api.Board().Cols[view.Active]))
	}
	if _, err := api.RenamePlan(def.ID, "pasted"); err == nil {
		t.Fatal("renamed onto a taken name")
	}
	if _, err := api.RenamePlan(def.ID, "Main"); err != nil {
		t.Fatal(err)
	}
	if w, err := api.RemovePlanWarn(def.ID); err != nil || w == "" {
		t.Fatalf("warn %q %v", w, err)
	}
	if _, err := api.RemovePlan(def.ID); err != nil {
		t.Fatal(err)
	}
	if sh := api.Shell(); len(sh.Plans) != 2 || sh.Nav[1].Items[0].Kids[1].Label != "Empty" {
		t.Fatalf("after delete %+v", sh.Plans)
	}
	// The palette goes to the other plans.
	var plans []string
	for _, p := range api.Palette("") {
		if p.Kind == view.PalPlan {
			plans = append(plans, p.Name)
		}
	}
	if strings.Join(plans, ",") != "list Empty" {
		t.Fatalf("palette plans %v", plans)
	}
	_ = e
}

// A game added after the shared folders were read is scanned alone: the guide counts its
// Data/ and Mods/ (02 §0 item 8).
func TestScanLineGameOnly(t *testing.T) {
	view.SetLang("en")
	out := "scan:3  env:1.5  1s  [game only]\n  sources:\n    src:official  1 mod\n    src:local     2 mods (1 about-disabled)\n  instances: 900 live, 0 tombstones\n"
	if got := scanLine(out); got != "3 mods in its Data and Mods" {
		t.Fatalf("game only: %q", got)
	}
	if got := scanLine(strings.Replace(out, "  [game only]", "", 1)); got != "900 mods found" {
		t.Fatalf("whole scan: %q", got)
	}
}

// Opening a list from the sidebar only shows it (04 §1.3, 06 §2.5): the current list — the
// one the launch page shows and launches — stays until the player makes another current
// or writes the open one to the game.
func TestOpenListKeepsCurrent(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	enabled := len(api.Board().Cols[view.Active])
	d, err := api.NewPlan("Empty", "")
	if err != nil {
		t.Fatal(err)
	}
	sh := api.Shell()
	def := sh.Plans[0]
	if !def.Current || def.Open || sh.Plan == nil || sh.Plan.Name != "Empty" || sh.Current == nil || sh.Current.ID != def.ID {
		t.Fatalf("a new list is open, not current: %+v", sh.Plans)
	}
	if kids := sh.Nav[1].Items[0].Kids; len(kids) != 3 || kids[0].ID != view.PagePlans || kids[1].ID != def.Page {
		t.Fatalf("全部列表 first, then the current list: %+v", kids)
	}
	if h := api.Home(); h.Featured < 0 || h.Rows[h.Featured].Name != def.Name {
		t.Fatalf("the launch page features the open list: %+v", h)
	}
	// Going to the current list and back opens without choosing.
	if _, err := api.OpenPlan(def.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(api.Board().Cols[view.Active]); n != enabled {
		t.Fatalf("default shows %d enabled, want %d", n, enabled)
	}
	if _, err := api.OpenPlan(view.PlanOfPage(d.Page)); err != nil {
		t.Fatal(err)
	}
	if sh := api.Shell(); sh.Plan.Name != "Empty" || !sh.Plans[0].Current || sh.Plans[0].ID != def.ID {
		t.Fatalf("opening changed the current list: %+v", sh.Plans)
	}
	// Writing the open list to the game chooses it.
	before := e.ReadModsConfig()
	if o, err := api.Run(view.ListSave); err != nil || len(o.Refused) > 0 {
		t.Fatalf("save %+v %v", o, err)
	}
	if e.ReadModsConfig() == before {
		t.Fatal("nothing written")
	}
	if sh := api.Shell(); sh.Plans[0].Name != "Empty" || !sh.Plans[0].Current || !sh.Plans[0].Open || sh.Current != nil {
		t.Fatalf("the written list is not current: %+v", sh.Plans)
	}
}
