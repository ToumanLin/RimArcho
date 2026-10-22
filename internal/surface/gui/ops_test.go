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
	for _, k := range []string{`Delete`, `"\\"`, `menuKey`} {
		if !strings.Contains(main, k) || !strings.Contains(lib, k) {
			t.Errorf("%s: not read on both pages", k)
		}
	}
}
