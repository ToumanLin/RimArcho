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
	_, api, _ := fixture(t)
	b := api.Board()
	sh := api.Shell()
	if sh.Envs != 1 || sh.Env != "main · 1.6" {
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
	api := NewAPI(e.App, func(name string, data any) {
		if name != "guide" {
			return
		}
		b, _ := json.Marshal(data)
		var m map[string]any
		json.Unmarshal(b, &m)
		events <- m
	})
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
	if len(found) != 1 || found[0].Name != "steam" {
		t.Fatalf("found %+v", found)
	}
	names, err := api.GuideAdd(found)
	if err != nil || strings.Join(names, ",") != "steam" {
		t.Fatalf("added %v %v", names, err)
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
	if b := api.Board(); b.Never || b.EnvName != "steam" {
		t.Fatalf("board after the guide: never %v env %s", b.Never, b.EnvName)
	}
	if _, err := api.PackImport(e.Root); err == nil {
		t.Fatal("a folder was imported as a pack")
	}
}
