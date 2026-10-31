package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
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
	t.Cleanup(api.Close)
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
	t.Parallel()
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
	n := api.needsLocked()
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
	t.Parallel()
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
		// Undo restores the list; staging is the player's separate list (04 §6).
		if strings.Split(got, " | ")[0] != strings.Split(want, " | ")[0] {
			t.Fatalf("undo: %s, want %s", got, want)
		}
	}
}

// 06 §6: sort, write and launch act on one click (no confirmation); sort offers 撤销, and
// a refusal comes back with the commands that resolve it, writing nothing.
func TestRunAtOnce(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	if _, err := api.Toggle(int(view.Inactive), []string{api.Board().Cols[view.Inactive][0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	before := names(api.Board().Cols[view.Active])
	o, err := api.Run(view.ListSort)
	if err != nil || o.Refused != nil {
		t.Fatalf("sort: %+v %v", o, err)
	}
	if after := names(api.Board().Cols[view.Active]); after != before {
		if !o.Undo {
			t.Fatalf("sort moved mods but offers no undo: %+v", o)
		}
		if _, err := api.Undo(); err != nil || names(api.Board().Cols[view.Active]) != before {
			t.Fatalf("undo after sort: %v %s", err, names(api.Board().Cols[view.Active]))
		}
	}
	if o, _ := api.Run(view.ListSort); o.Undo == (o.Text == view.T("the order is already right")) {
		t.Fatalf("sort undo flag: %+v", o)
	}
	o, err = api.Run(view.ListSave)
	if err != nil || o.Refused != nil || !strings.Contains(o.Text, "v") {
		t.Fatalf("save: %+v %v", o, err)
	}
	// An outside change not loaded: launch is refused, nothing is written or started.
	e.MustRun("enable", "pkg:tester.b")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	file := e.ReadModsConfig()
	o, err = api.Run(view.ListLaunch)
	if err != nil || len(o.Refused) < 2 || o.Text != o.Refused[0] || len(e.Launched) != 0 || e.ReadModsConfig() != file {
		t.Fatalf("launch over an outside change: %+v %v, launched %v", o, err, e.Launched)
	}
	if strings.HasPrefix(o.Text, "unloaded external change") || !strings.Contains(strings.Join(o.Refused[1:], " "), "archo ") {
		t.Fatalf("refusal lines: %q", o.Refused)
	}
	if _, err := api.Run("rm"); err == nil {
		t.Fatal("Run took an unknown action")
	}
	// The preview stays reachable (the palette's 预览启动) and starts nothing.
	if p, err := api.Preview(view.ListLaunch); err != nil || len(p.Lines) == 0 || len(e.Launched) != 0 {
		t.Fatalf("launch preview: %+v %v", p, err)
	}
}

func TestPreviewAndInspect(t *testing.T) {
	t.Parallel()
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

// The scan runs in the background and says when it is done (an event, 06 §8.4).
func TestScanEvent(t *testing.T) {
	t.Parallel()
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

// T: the whole enabled list goes to staging (Core stays), one step; put all back restores
// the order.
func TestStageAll(t *testing.T) {
	t.Parallel()
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

// A pack build runs in the background: the build call returns at once, the other pages
// answer while it copies, a second build is refused, and "pack" events carry the
// progress and the end.
func TestPackBuildBackground(t *testing.T) {
	e, api, events := fixture(t)
	release := make(chan struct{})
	api.PackRunner = func(name string) error {
		tmp := filepath.Join(e.App.PacksDir(), name+app.TmpSuffix)
		os.MkdirAll(tmp, 0o755)
		b, _ := json.Marshal(app.BuildProgress{Name: name, Step: app.StepCopy, Mods: 1, ModsTotal: 2, Bytes: 50, BytesTotal: 100, Beat: time.Now()})
		os.WriteFile(filepath.Join(tmp, app.BuildFile), b, 0o644)
		<-release
		os.RemoveAll(tmp)
		return nil
	}
	start := time.Now()
	if _, err := api.PackBuild("first", 0); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("PackBuild waited for the build")
	}
	answered := make(chan *PackList, 1)
	go func() {
		api.Settings()
		api.Needs()
		l, _ := api.Packs()
		answered <- l
	}()
	select {
	case l := <-answered:
		if l.Building != "first" {
			t.Fatalf("packs during the build: %+v", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the pages wait for the build")
	}
	if _, err := api.PackBuild("second", 0); err == nil {
		t.Fatal("a second build started")
	}
	waitPack := func(want string) string {
		deadline := time.After(10 * time.Second)
		for {
			select {
			case ev := <-events:
				if strings.HasPrefix(ev, "pack ") && strings.Contains(ev, want) {
					return ev
				}
			case <-deadline:
				t.Fatalf("no pack event with %s", want)
			}
		}
	}
	if ev := waitPack(`"frac":0.5`); !strings.Contains(ev, `"step":2`) || !strings.Contains(ev, `"running":true`) {
		t.Fatalf("progress event %s", ev)
	}
	close(release)
	if ev := waitPack(`"done":true`); !strings.Contains(ev, `"ok":true`) {
		t.Fatalf("end event %s", ev)
	}
	if l, _ := api.Packs(); l.Building != "" {
		t.Fatalf("still building: %+v", l)
	}
}

// A build cut short shows on the packs page and can be removed from there.
func TestPackUnfinishedRows(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	tmp := filepath.Join(e.App.PacksDir(), "cut"+app.TmpSuffix)
	os.MkdirAll(tmp, 0o755)
	b, _ := json.Marshal(app.BuildProgress{Name: "cut", Step: app.StepCopy, Mods: 4, ModsTotal: 10, Bytes: 3 << 30, Beat: time.Now().Add(-time.Hour)})
	os.WriteFile(filepath.Join(tmp, app.BuildFile), b, 0o644)
	l, err := api.Packs()
	if err != nil || len(l.Unfinished) != 1 || l.Unfinished[0].Running || !strings.Contains(l.Unfinished[0].Facts, "3.0 GB") {
		t.Fatalf("unfinished: %+v %v", l, err)
	}
	if _, err := api.PackRemoveUnfinished("cut"); err != nil {
		t.Fatal(err)
	}
	if l, _ := api.Packs(); len(l.Unfinished) != 0 {
		t.Fatalf("still listed: %+v", l.Unfinished)
	}
}

// G14 (06 §2.6): with an error-level problem the build page disables 编译 and says why,
// pointing to 能不能跑; 先看清单 writes nothing; once fixed, 编译 is enabled with the sizes.
func TestBuildPageG14(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	p, err := buildPage(t, api, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ok || p.Why == "" || !p.ToNeed || len(p.Steps) != 5 || p.Steps[0].Level != view.LevelBad || !p.Steps[0].Needs || len(p.Steps[0].More) == 0 {
		t.Fatalf("with a problem: %+v", p)
	}
	if p.Name == "" || api.PackNameCheck(p.Name) != "" {
		t.Fatalf("default name %q: %s", p.Name, api.PackNameCheck(p.Name))
	}
	lines := api.PackDryRun(p.Name, 0)
	if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "cannot build") {
		t.Fatalf("dry run: %v", lines)
	}
	if ents, _ := os.ReadDir(e.App.PacksDir()); len(ents) != 0 {
		t.Fatalf("the dry run wrote %d entries", len(ents))
	}
	e.MustRun("enable", "pkg:brrainz.harmony")
	api.board, api.needs = nil, nil
	p, err = buildPage(t, api, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ok || p.Why != "" || p.Before == "" || p.After == "" || p.Mods != 2 || p.Steps[0].Level == view.LevelBad {
		t.Fatalf("fixed: %+v", p)
	}
	// Textures (08 §5.2): a step of the build with the choices; without todds, 编译 says
	// to install it there; with an encoder the step says what it converts.
	if p.Steps[2].Kind != "textures" || p.Tex != 0 || len(p.TexChoices) != 8 {
		t.Fatalf("textures step: %+v", p.Steps[2])
	}
	if p, _ := buildPage(t, api, 100); p.Steps[2].Go == "textures" && (p.Ok || p.ToGo != "textures") {
		t.Fatalf("no todds: %+v", p)
	}
	e.App.TextureEncoder = fakeEncoder{}
	if p, _ := buildPage(t, api, 75); !p.Ok || p.Tex != 75 || p.Steps[2].Go != "" || p.Steps[2].Level == view.LevelBad {
		t.Fatalf("with an encoder: %+v", p)
	}
	if lines := strings.Join(api.PackDryRun("tex one", 75), " | "); !strings.Contains(lines, "dry run") {
		t.Fatalf("dry run with textures: %s", lines)
	}
	e.App.TextureEncoder = nil
	for name, bad := range map[string]bool{"": true, ".x": true, "ok name": false} {
		if (api.PackNameCheck(name) != "") != bad {
			t.Errorf("name %q: %q", name, api.PackNameCheck(name))
		}
	}
	e.MustRun("pack", "build", "one")
	if api.PackNameCheck("one") == "" {
		t.Fatal("the name of an existing pack is accepted")
	}
	pg, err := api.Pack("one")
	if err != nil || len(pg.Mods) == 0 {
		t.Fatalf("pack page: %+v %v", pg, err)
	}
	for i, m := range pg.Mods {
		if m.Order != i+1 || (m.Size != "" && m.Size != view.SizeText(m.Bytes)) {
			t.Errorf("pack mod has no raw sort values: %+v", m)
		}
	}
	rows, err := api.Packs()
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].Size != view.SizeText(rows.Rows[0].Bytes) {
		t.Fatalf("pack table values: %+v %v", rows, err)
	}
	wire, _ := json.Marshal(rows.Rows[0])
	if !strings.Contains(string(wire), `"playedAt":`) || !strings.Contains(string(wire), `"bytes":`) {
		t.Fatalf("pack table sort values absent from JSON: %s", wire)
	}
	// its list alone, as a RimArcho list (no dialog here: the path is typed); the guide's
	// 打开 unpacks packs only, a list goes through the import preview
	list := filepath.Join(e.Root, "one.archolist.json")
	if text, err := api.PackExport("one", list, true); err != nil || !strings.Contains(text, "exported the list") {
		t.Fatalf("export the list: %q %v", text, err)
	}
	if b, _ := os.ReadFile(list); !strings.Contains(string(b), `"hash"`) || strings.Contains(string(b), `"folder"`) {
		t.Fatalf("the list:\n%s", b)
	}
	if _, err := api.PackImport(list); err == nil || !strings.Contains(err.Error(), "not a pack") {
		t.Fatalf("a list was unpacked as a pack: %v", err)
	}
}

// fakeEncoder stands in for todds (it converts nothing).
type fakeEncoder struct{}

func (fakeEncoder) Encode(context.Context, []string, int, bool) error { return nil }

// Another version in the way of a pack copy (08 §5.3): the launch comes back with the moves
// for one confirmation; with move it goes on.
func TestPackLaunchMovesConfirm(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.MustRun("enable", "pkg:brrainz.harmony")
	e.MustRun("enable", "pkg:tester.b")
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("pack", "build", "p")
	e.WriteFile(filepath.Join(e.Game, "Mods", "B", "Defs", "New.xml"), "<Defs/>")
	e.MustRun("scan")
	o, err := api.PackLaunch("p", false)
	if err != nil || len(o.Moves) < 2 || !strings.Contains(strings.Join(o.Moves, "\n"), "Mods/B →") {
		t.Fatalf("first: %+v %v", o, err)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "B")); err != nil {
		t.Fatal("moved before the confirmation")
	}
	o, err = api.PackLaunch("p", true)
	if err != nil || o.Refused != nil {
		t.Fatalf("with move: %+v %v", o, err)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "B")); err == nil {
		t.Fatal("Mods/B is still there")
	}
}
