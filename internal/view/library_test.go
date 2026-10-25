package view_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func libEnv(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	ws := e.AddWorkshop("5100", testkit.Sample("tester.lock", "Lockable"))
	e.AddWorkshop("5200", testkit.Sample("tester.idle", "Idle"))
	hidden := e.AddWorkshop("5600", testkit.Sample("tester.hid", "Hid"))
	e.Move(filepath.Join(hidden, "About", "About.xml"), filepath.Join(hidden, "About", "About.xml.disabled"))
	e.Subscribe("5300")
	e.CopyTree(ws, filepath.Join(e.Game, "Mods", "Lockable Copy"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lock")
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func libPage(t *testing.T, e *testkit.Env, search string, filters ...string) *view.LibPage {
	t.Helper()
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildLibrary(lib, search, filters, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func libRow(t *testing.T, p *view.LibPage, name string) view.LibRow {
	t.Helper()
	for _, r := range p.Rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q", name)
	return view.LibRow{}
}

func kinds(as []string) []string { return as }

func TestLibraryPage(t *testing.T) {
	view.SetLang("zh")
	defer view.SetLang("en")
	e := libEnv(t)
	p := libPage(t, e, "")
	if p.Total != 4 || p.Shown != 4 {
		t.Fatalf("total %d shown %d", p.Total, p.Shown)
	}
	lock := libRow(t, p, "Lockable")
	if lock.Single || len(lock.Insts) != 2 || lock.Insts[0].Label != "工坊" || lock.Insts[1].Label != "本地" {
		t.Fatalf("Lockable rows: %+v", lock.Insts)
	}
	if lock.State != "" || lock.Used != "default" {
		t.Errorf("a lineage with a used copy is not \"in no plan\": %q %q", lock.State, lock.Used)
	}
	if got := kinds(lock.Insts[1].Ops); !slices.Equal(got, []string{"take-out", "move-to", "stage", "swap", "remove", "tags", "note", "separate", "folder", "move", "textures", "delete"}) {
		t.Errorf("the Mods/ copy: %v", got)
	}
	if got := kinds(lock.Insts[0].Ops); !slices.Equal(got, []string{"use", "tags", "note", "separate", "folder", "workshop", "steam", "pin", "textures", "unsubscribe"}) {
		t.Errorf("the unused workshop copy beside it (enabling it would clash): %v", got)
	}
	hid := libRow(t, p, "Hid")
	if !hid.Single || hid.State != "被其它管理器隐藏" || hid.Source != "工坊" || hid.Insts[0].Name != "工坊" {
		t.Errorf("hidden: %+v", hid)
	}
	if got := kinds(hid.Insts[0].Ops); !slices.Equal(got, []string{"tags", "note", "folder", "workshop", "steam", "unhide", "unsubscribe"}) {
		t.Errorf("hidden actions: %v", got)
	}
	ghost := libRow(t, p, "工坊条目 5300")
	if ghost.Insts[0].ID != "" || ghost.Size != "—" || ghost.State != "订阅了但不在磁盘上" || !slices.Equal(kinds(ghost.Insts[0].Ops), []string{"workshop", "steam"}) {
		t.Errorf("ghost: %+v", ghost)
	}
	idle := libRow(t, p, "Idle")
	if got := kinds(idle.Insts[0].Ops); !slices.Contains(got, "add") {
		t.Errorf("idle: %v", got)
	}
	// filters and conditions
	p = libPage(t, e, "", "used:none", "-source:local")
	if p.Shown != 4 || len(p.Conds) != 2 || !p.Conds[1].Exclude || p.Conds[0].Label != "用在 不在任何列表里" {
		t.Errorf("filtered: shown %d conds %+v", p.Shown, p.Conds)
	}
	for _, g := range p.Groups {
		if g.Key == "source" && g.Title != "来源" {
			t.Errorf("source title %q", g.Title)
		}
	}
	if _, err := view.BuildLibrary(mustLibrary(t, e), "", []string{"nope:x"}, ""); err == nil {
		t.Error("bad filter")
	}
}

func TestLocationFiltersAcrossPages(t *testing.T) {
	t.Parallel()
	e := libEnv(t)
	local := filepath.Join(e.Game, "Mods")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	r := view.FilterBoard(b, "", []string{"location:" + local}, view.MatchFacts{}, false, "1.6")
	if len(r.Rows[view.Active]) != 1 || b.Cols[view.Active][r.Rows[view.Active][0]].Name != "Lockable" {
		t.Fatalf("board's local copy: %+v", r.Rows)
	}
	p := libPage(t, e, "", "location:"+local)
	if p.Shown != 1 || len(p.Rows[0].Insts) != 1 || p.Rows[0].Insts[0].Label != "local" {
		t.Fatalf("library's local copy: %+v", p.Rows)
	}
	// The same quoted path syntax works in both search boxes, including spaces and colons.
	words, filters := view.LibSplitQuery(`Lockable location:"D:\Steam Library\Mods" -location:"E:\Other Mods"`)
	if words != "Lockable" || !slices.Equal(filters, []string{`location:D:\Steam Library\Mods`, `-location:E:\Other Mods`}) {
		t.Fatalf("quoted path filters: %q %v", words, filters)
	}
	p = libPage(t, e, `location:"`+local+`"`)
	if p.Shown != 1 {
		t.Fatalf("typed location: %d rows", p.Shown)
	}
}

func mustLibrary(t *testing.T, e *testkit.Env) *app.Library {
	t.Helper()
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestLibraryDo(t *testing.T) {
	t.Parallel()
	e := libEnv(t)
	p := libPage(t, e, "Idle")
	idle := p.Rows[0].Insts[0]
	if _, err := view.LibDo(e.App, view.LibEnable, []string{idle.Ref}); err != nil {
		t.Fatal(err)
	}
	if !libRow(t, libPage(t, e, ""), "Idle").Insts[0].InPlan {
		t.Fatal("enable did not add it")
	}
	msg, err := view.LibDo(e.App, view.LibRemove, []string{idle.Ref})
	if err != nil || !strings.Contains(msg, "1 lists") {
		t.Fatalf("remove: %q %v", msg, err)
	}
	cp := libRow(t, libPage(t, e, ""), "Lockable").Insts[1]
	pv, err := view.LibPreview(e.App, view.LibMove, []string{cp.Ref})
	if err != nil || !pv.Apply || !strings.Contains(strings.Join(pv.Lines, "\n"), "would move to the lock library") {
		t.Fatalf("preview: %+v %v", pv, err)
	}
	if _, err := view.LibDo(e.App, view.LibMove, []string{cp.Ref}); err != nil {
		t.Fatal(err)
	}
	after := libRow(t, libPage(t, e, ""), "Lockable")
	if after.Insts[1].ID != cp.ID || !strings.HasPrefix(after.Insts[1].Label, "locked version  ") || !after.Insts[1].InPlan {
		t.Fatalf("after the move: %+v", after.Insts[1])
	}
	// the dry run of a refused action cannot be applied
	pv, err = view.LibPreview(e.App, view.LibTrash, []string{libRow(t, libPage(t, e, ""), "Idle").Insts[0].Ref})
	if err != nil || pv.Apply {
		t.Fatalf("trash of a subscribed mod: %+v %v", pv, err)
	}
}

// 06 §3.7 in the library: typed key:value words filter, tag: matches tag words, and the
// box completes the library's keys and values.
func TestLibrarySearchBox(t *testing.T) {
	t.Parallel()
	e := libEnv(t)
	e.MustRun("tag", "add", "pkg:tester.idle", "Facial Hair")
	e.MustRun("tag", "add", "pkg:tester.hid", "Facial Animation")
	lib := mustLibrary(t, e)
	names := func(p *view.LibPage) []string {
		var out []string
		for _, r := range p.Rows {
			out = append(out, r.Name)
		}
		return out
	}
	page := func(text string) *view.LibPage {
		p, err := view.BuildLibrary(lib, text, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := names(page("tag:facial")); !slices.Equal(got, []string{"Hid", "Idle"}) {
		t.Errorf("tag:facial: %v", got)
	}
	if got := names(page("tag:anim")); !slices.Equal(got, []string{"Hid"}) {
		t.Errorf("tag:anim: %v", got)
	}
	if got := names(page("tag:air")); len(got) != 0 {
		t.Errorf("tag:air must not match inside a word: %v", got)
	}
	// a click on a tag in the filter column is that tag only
	if p, _ := view.BuildLibrary(lib, "", []string{"tag:Facial Hair"}, ""); !slices.Equal(names(p), []string{"Idle"}) {
		t.Errorf("tag:Facial Hair: %v", names(p))
	}
	if got := names(page("-state:hidden lock")); !slices.Equal(got, []string{"Lockable"}) {
		t.Errorf("words and an exclusion: %v", got)
	}
	for text, want := range map[string]string{"so": "urce:", "st": "ate:", "state:un": "loadable", "state:unu": "", "state:res": "idue", "-source:wo": "rkshop", "tag:fa": "cial", "used:pl": "an:default", "lock": "", "x": ""} {
		if got := page(text).Complete; got != want {
			t.Errorf("complete %q: %q, want %q", text, got, want)
		}
	}
}

// 06 §4.1, §2.9: one field list, the editor's fields with the library's in between.
func TestLibFields(t *testing.T) {
	t.Parallel()
	e := libEnv(t)
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	r := libRow(t, libPage(t, e, ""), "Lockable")
	if len(r.Insts) != 2 {
		t.Fatalf("copies %d", len(r.Insts))
	}
	i := lib.Instance(r.Insts[0].Ref)
	var got []string
	for _, f := range view.LibFields(e.App, lib, i, nil) {
		got = append(got, f.Kind)
	}
	// the workshop copy has no state: no state row
	want := []string{"id", "package", "path", "tags", "size", "versions", "updated"}
	for _, k := range want {
		if !slices.Contains(got, k) {
			t.Fatalf("no %s field: %v", k, got)
		}
	}
	if got[0] != "id" || got[1] != "package" {
		t.Fatalf("order %v", got)
	}
	// the editor's: no library rows
	for _, f := range view.ModFields(e.App, lib.St, i.Mod, nil) {
		if f.Kind == "state" || f.Kind == "used" {
			t.Fatalf("editor field %s", f.Kind)
		}
	}
	// the page's columns: name first, the hidden ones last
	p := libPage(t, e, "")
	if len(p.Columns) == 0 || p.Columns[0].Key != "name" || !p.Columns[0].Shown || p.Columns[len(p.Columns)-1].Shown {
		t.Fatalf("columns %+v", p.Columns)
	}
}

// 06 §2.9: 退订 is a confirmed action through the Steam client; when Steam is out of reach
// LibDo answers OpenPages (the window opens the page and says so).
func TestLibSteam(t *testing.T) {
	t.Parallel()
	e := libEnv(t)
	idle := libRow(t, libPage(t, e, ""), "Idle").Insts[0]
	unsub := view.OpOf(view.LibUnsub)
	if !slices.Contains(idle.Ops, view.LibUnsub) || !unsub.Confirm || !unsub.Danger || unsub.Key != "Del" {
		t.Fatalf("unsubscribe op %+v %v", unsub, idle.Ops)
	}
	p, err := view.LibPreview(e.App, view.LibUnsub, []string{idle.Ref})
	if err != nil || !p.Apply || !strings.Contains(strings.Join(p.Lines, "\n"), "would unsubscribe") {
		t.Fatalf("preview %+v %v", p, err)
	}
	// no steam_api64.dll in the test game: out of reach
	_, err = view.LibDo(e.App, view.LibUnsub, []string{idle.Ref})
	var pages *view.OpenPages
	if !errors.As(err, &pages) || len(pages.URLs) != 1 || !strings.HasSuffix(pages.URLs[0], "?id=5200") {
		t.Fatalf("fallback %v", err)
	}
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake")
	var got []string
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		got = args
		return "ITEM 5200 ok 0\nDONE unsubscribe 1\n", nil
	}
	msg, err := view.LibDo(e.App, view.LibUnsub, []string{idle.Ref})
	if err != nil || !strings.Contains(msg, "Idle") || strings.Join(got, " ") != "unsubscribe 5200" {
		t.Fatalf("unsubscribe %q %v %v", msg, err, got)
	}
}

// 06 §2.9: the rows carry op codes only; the labels go once, in the page's op table, and
// the copies' paths and Workshop URLs stay on the Go side.
func TestLibraryPayload(t *testing.T) {
	view.SetLang("zh")
	defer view.SetLang("en")
	p := libPage(t, libEnv(t), "")
	rows, err := json.Marshal(p.Rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"打开文件夹", `"path"`, `"url"`, `"actions"`} {
		if strings.Contains(string(rows), s) {
			t.Errorf("rows carry %s", s)
		}
	}
	if len(p.Ops) != len(view.OpTable()) || p.Ops[0].Label == "" {
		t.Errorf("op table %+v", p.Ops)
	}
}
