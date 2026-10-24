package view

import (
	"slices"
	"testing"
)

// 06 §3.1 rule 6: everything the mouse can do, the keyboard can do. Every op a row can
// have is in the table with a key or a menu group (the menu key and shift+F10 open the
// menu), and every menu group's ops come in the menu's order.
func TestOpsReachable(t *testing.T) {
	seen := map[string]bool{}
	last := 0
	for _, o := range OpTable() {
		if seen[o.Code] {
			t.Errorf("%s twice", o.Code)
		}
		seen[o.Code] = true
		if !OpKeyed(o.Code) {
			t.Errorf("%s: no key and no menu", o.Code)
		}
		if o.Group < last {
			t.Errorf("%s: group %d after %d", o.Code, o.Group, last)
		}
		last = o.Group
		if o.Label == "" {
			t.Errorf("%s: no label", o.Code)
		}
	}
	// the library's action kinds and the editor's plan ops are ops
	for _, k := range []string{LibEnable, LibRemove, LibPin, LibPinUpdate, LibUnpin, LibUnhide, LibUnsub, LibDelete, LibResub, LibMove, LibFolder, LibWorkshop} {
		if !seen[k] {
			t.Errorf("library kind %s is no op", k)
		}
	}
	for _, col := range []Column{Inactive, Active, Staging} {
		for _, c := range RowOps(nil, "x", col) {
			if !seen[c] {
				t.Errorf("editor op %s is no op", c)
			}
		}
	}
	// Form pages expose their verbs and cancellation to keyboard users (06 §2.17).
	for page, required := range map[string][]string{PagePlans: {"n", "i", "e", "F2", "Delete", "esc"}, "plan-import": {"tab", "enter", "esc"}, "plan-export": {"↑↓", "ctrl+s", "ctrl+c", "esc"}} {
		keys := map[string]bool{}
		for _, row := range KeyHelp(page, false) {
			keys[row.Keys] = true
		}
		for _, key := range required {
			if !keys[key] {
				t.Errorf("%s has no %s", page, key)
			}
		}
	}
	// 退订 / 删除 last, in red
	tab := OpTable()
	for _, o := range tab[len(tab)-2:] {
		if !o.Danger || o.Key != "Del" {
			t.Errorf("last: %+v", o)
		}
	}
}

// fixColumn: the editor's row keeps its column's plan ops.
func TestRowOpsColumns(t *testing.T) {
	ops := []string{OpTakeOut, OpMoveTo, OpStage, OpSwap, OpTags, OpFolder}
	if got := fixColumn(ops, Inactive); !slices.Equal(got, []string{OpEnable, OpEnableAt, OpStage, OpTags, OpFolder}) {
		t.Errorf("inactive %v", got)
	}
	if got := fixColumn(ops, Active); !slices.Equal(got, []string{OpDisable, OpMoveTo, OpStage, OpSwap, OpTags, OpFolder}) {
		t.Errorf("active %v", got)
	}
	if got := fixColumn(ops, Staging); !slices.Equal(got, []string{OpPutBack, OpEnableAt, OpTags, OpFolder}) {
		t.Errorf("staging %v", got)
	}
}

// The help and the ops read in Chinese.
func TestKeyHelpText(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	for _, page := range []string{EditorPage, LibraryPage, "home"} {
		for _, tui := range []bool{false, true} {
			rows := KeyHelp(page, tui)
			if len(rows) < 5 {
				t.Fatalf("%s: %d rows", page, len(rows))
			}
			for _, r := range rows {
				if !wide(r.What) {
					t.Errorf("%s tui=%v: %q not translated", page, tui, r.What)
				}
			}
		}
	}
	for _, o := range OpTable() {
		if !wide(o.Label) {
			t.Errorf("op %s: %q", o.Code, o.Label)
		}
	}
	for _, k := range BarKeys {
		if !wide(barKeyTitle(k)) {
			t.Errorf("bar %s", k)
		}
		for _, v := range barValues[k] {
			if x := barValueText(k, v); !wide(x) && x != "git" && x != "SteamCMD" {
				t.Errorf("bar %s:%s = %q", k, v, x)
			}
		}
	}
}

func wide(s string) bool {
	for _, r := range s {
		if r >= 0x80 {
			return true
		}
	}
	return false
}
