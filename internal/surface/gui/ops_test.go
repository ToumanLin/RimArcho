package gui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §3.1 rule 6: everything the mouse can do, the keyboard can do. Every op the editor's
// and Mod 库存's rows can have has a key or the row menu (the menu key, shift+F10), and the
// page runs it: as one of the editor's verbs (main.js rowOp), one of Mod 库存's own cases
// (library.js run), or through LibDo.
func TestOpsRunnable(t *testing.T) {
	t.Parallel()
	read := func(p string) string {
		b, err := fs.ReadFile(assets, p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	main, lib := read("assets/js/main.js"), read("assets/js/library.js")
	verbs := regexp.MustCompile(`const verbs = \{([^}]*)\}`).FindStringSubmatch(main)
	if verbs == nil {
		t.Fatal("main.js has no rowOp verbs")
	}
	for _, o := range view.OpTable() {
		if !view.OpKeyed(o.Code) {
			t.Errorf("%s: no key and no menu", o.Code)
		}
		q := `"` + o.Code + `"`
		ed := strings.Contains(verbs[1], q) || strings.Contains(verbs[1], o.Code+":") || view.LibDoes(o.Code)
		lb := strings.Contains(lib, "case "+q) || view.LibDoes(o.Code)
		if !ed {
			t.Errorf("%s: the editor does not run it", o.Code)
		}
		if !lb {
			t.Errorf("%s: Mod 库存 does not run it", o.Code)
		}
	}
	// the keys the table names are the ones the pages read
	// Data table headers share mouse and keyboard behavior (06 §3.9).
	for _, name := range []string{"library", "tasks", "packs", "textures"} {
		source := read("assets/js/" + name + ".js")
		for _, marker := range []string{`from "./datatable.js"`, `.key(e)`} {
			if !strings.Contains(source, marker) {
				t.Errorf("%s: missing shared table entry %s", name, marker)
			}
		}
	}
	if !strings.Contains(main, `return tex.key(e)`) {
		t.Error("texture table keys do not reach the page")
	}
	for _, k := range []string{`Delete`, `"\\"`, `menuKey`} {
		if !strings.Contains(main, k) || !strings.Contains(lib, k) {
			t.Errorf("%s: not read on both pages", k)
		}
	}
}

// Player-invoked forms are pages; irreversible list deletion still confirms (06 §3.1, 06 §6).
func TestFormRoutesRunnable(t *testing.T) {
	t.Parallel()
	read := func(p string) string {
		b, err := fs.ReadFile(assets, "assets/js/"+p+".js")
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	main, navigation, plans, exchange := read("main"), read("navigation"), read("plans"), read("exchange")
	for _, route := range []string{`case "plan-import"`, `case "plan-export"`} {
		if !strings.Contains(navigation, route) {
			t.Errorf("missing page route %s", route)
		}
	}
	for _, route := range []string{`case "new-plan"`, `newList:`, `undoStatus()`} {
		if !strings.Contains(main, route) {
			t.Errorf("missing route %s", route)
		}
	}
	if strings.Contains(exchange, "pop.open") || strings.Contains(plans, `pop.open("plan-new"`) {
		t.Fatal("form still opens a popup")
	}
	if !strings.Contains(plans, `pop.confirm(T("delete the list")`) {
		t.Fatal("delete lost confirmation")
	}
	history := main[strings.Index(main, "async function goHistory"):]
	history = history[:strings.Index(history, "// pollLive")]
	if strings.Contains(history, "confirm") || !strings.Contains(history, "api.HistoryGo") || !strings.Contains(history, `h.kind === "version"`) {
		t.Fatal("history checkout must run directly with undo")
	}
}
