package gui

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func fixture(t *testing.T) (*testkit.Env, *API, chan string) {
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	a := testkit.Sample("tester.a", "Alpha")
	a.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "Beta"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	events := make(chan string, 16)
	api := NewAPI(e.App, func(name string, data any) {
		b, _ := json.Marshal(data)
		events <- name + " " + string(b)
	})
	return e, api, events
}

func names(items []view.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name)
	}
	return strings.Join(out, ",")
}

// G1: the GUI shows view's structures; nothing is rebuilt in the surface.
func TestSameViewModels(t *testing.T) {
	e, api, _ := fixture(t)
	b := api.Board()
	want, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.Cols, want.Cols) || b.Unsaved != want.Unsaved || b.EnvName != want.EnvName {
		t.Fatalf("board differs from view.BuildBoard:\n%+v\n%+v", b.Cols, want.Cols)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := api.needsLocked(false)
	vn := view.BuildNeeds(e.App, want.St, ps)
	if !reflect.DeepEqual(n.Items, vn.Items) || n.Count != vn.Count {
		t.Fatalf("needs differ:\n%+v\n%+v", n.Items, vn.Items)
	}
	if got := names(b.Cols[view.Active]); got != "Core,Alpha" {
		t.Fatalf("enabled: %s", got)
	}
	// "Alpha needs Harmony, but Harmony is not enabled" with its enable action
	found := false
	for _, it := range n.Items {
		if strings.Contains(it.Text, "Harmony is not enabled") && it.Actions[0].Kind == view.ActEnable {
			found = true
		}
	}
	if !found {
		t.Fatalf("no dep.inactive item: %+v", n.Items)
	}
}

// G3: each verb acts at once and u undoes it.
func TestVerbsAndUndo(t *testing.T) {
	_, api, _ := fixture(t)
	b := api.Board()
	id := func(col view.Column, name string) string {
		for _, it := range b.Cols[col] {
			if it.Name == name {
				return it.ID
			}
		}
		t.Fatalf("no %s in column %d", name, col)
		return ""
	}
	harmony, alpha, beta := id(view.Inactive, "Harmony"), id(view.Active, "Alpha"), id(view.Inactive, "Beta")
	step := func(what string, r *view.Result, err error, want string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		b = api.Board()
		if got := names(b.Cols[view.Active]) + " | " + names(b.Cols[view.Staging]); got != want {
			t.Fatalf("%s: %s, want %s (%s)", what, got, want, r.Text)
		}
	}
	r, err := api.Toggle(int(view.Inactive), []string{harmony}, nil)
	step("enable", r, err, "Core,Harmony,Alpha | ")
	if r.Text == "" || len(r.Placed) != 1 {
		t.Fatalf("enable result %+v", r)
	}
	r, err = api.MoveBy([]string{harmony}, 1)
	step("J", r, err, "Core,Alpha,Harmony | ")
	r, err = api.Stage(int(view.Active), []string{alpha})
	step("stage", r, err, "Core,Harmony | Alpha")
	r, err = api.Place(int(view.Inactive), []string{beta}, 1)
	step("place", r, err, "Beta,Core,Harmony | Alpha")
	r, err = api.Toggle(int(view.Staging), []string{alpha}, nil)
	step("put back", r, err, "Beta,Core,Alpha,Harmony | ")
	for _, want := range []string{"Beta,Core,Harmony | Alpha", "Core,Harmony | Alpha", "Core,Alpha,Harmony | ", "Core,Harmony,Alpha | ", "Core,Alpha | "} {
		if _, err := api.Undo(); err != nil {
			t.Fatal(err)
		}
		b = api.Board()
		got := names(b.Cols[view.Active]) + " | " + names(b.Cols[view.Staging])
		// Undo restores the list; staging is the player's separate list (04 §5.4).
		if strings.Split(got, " | ")[0] != strings.Split(want, " | ")[0] {
			t.Fatalf("undo: %s, want %s", got, want)
		}
	}
}

func TestPreviewAndInspect(t *testing.T) {
	_, api, _ := fixture(t)
	p, err := api.Preview(view.ListSort)
	if err != nil || len(p.Lines) == 0 {
		t.Fatalf("sort preview: %+v %v", p, err)
	}
	if _, err := api.Inspect([]string{"enable", "x"}); err == nil {
		t.Fatal("Inspect ran a command that changes data")
	}
	out, err := api.Inspect([]string{"problems"})
	if err != nil || len(out) == 0 {
		t.Fatalf("inspect: %v %v", out, err)
	}
	for _, l := range out {
		if strings.Contains(l, "#") && strings.Contains(l, "Alpha") {
			t.Fatalf("a #ref is left next to a name: %q", l)
		}
	}
	if err := api.OpenURL("javascript:alert(1)"); err == nil {
		t.Fatal("OpenURL took a javascript: link")
	}
	if err := api.SetSetting("cfg:ui.lang", "zh"); err == nil {
		t.Fatal("SetSetting wrote a non-GUI key")
	}
}

// The scan runs in the background and says when it is done (an event, 08 §9).
func TestScanEvent(t *testing.T) {
	e, api, events := fixture(t)
	exe := filepath.Join(t.TempDir(), "none.exe")
	api.Exe = exe // no such program: the scan fails and the failure is reported
	if !api.Scan() {
		t.Fatal("scan did not start")
	}
	if api.Scan() {
		t.Fatal("a second scan started")
	}
	select {
	case ev := <-events:
		if !strings.HasPrefix(ev, "scan ") || !strings.Contains(ev, `"ok":false`) {
			t.Fatalf("event %s", ev)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no scan event")
	}
	_ = e
}

func TestSplitArgs(t *testing.T) {
	got := splitArgs(`list --filter "tag:my tag"  -x`)
	if strings.Join(got, "|") != "list|--filter|tag:my tag|-x" {
		t.Fatalf("%q", got)
	}
}

// T: the whole enabled list goes to staging (Core stays), one step; put all back restores
// the order.
func TestStageAll(t *testing.T) {
	_, api, _ := fixture(t)
	b := api.Board()
	var harmony string
	for _, it := range b.Cols[view.Inactive] {
		if it.Name == "Harmony" {
			harmony = it.ID
		}
	}
	if _, err := api.Toggle(int(view.Inactive), []string{harmony}, nil); err != nil {
		t.Fatal(err)
	}
	cols := func() string {
		b := api.Board()
		return names(b.Cols[view.Active]) + " | " + names(b.Cols[view.Staging])
	}
	before := cols()
	r, err := api.StageAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := cols(); got != "Core | Harmony,Alpha" && got != "Core | Alpha,Harmony" {
		t.Fatalf("stage all: %s (%s)", got, r.Text)
	}
	if _, err := api.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if got := cols(); got != before {
		t.Fatalf("put all back: %s, want %s", got, before)
	}
}
