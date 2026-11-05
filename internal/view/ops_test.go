package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"slices"
	"strings"
	"testing"
)

// 16 §3.1 rule 6: everything the mouse can do, the keyboard can do. Every op a row can
// have is in the table with a key or a menu group (the menu key and shift+F10 open the
// menu), and every menu group's ops come in the menu's order.
func TestOpsReachable(t *testing.T) {
	t.Parallel()
	for _, page := range []string{LibraryPage, "packs", "textures", PageTasks} {
		reached := false
		for _, row := range KeyHelp(page, false) {
			if strings.Contains(row.Keys, "s") && strings.Contains(row.Keys, "< > =") {
				reached = true
			}
		}
		if !reached {
			t.Errorf("%s: table keyboard actions are absent from help", page)
		}
	}
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
	// Form pages expose their verbs and cancellation to keyboard users (16 §2.17).
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
	// unsubscribe / delete last, in red
	tab := OpTable()
	for _, o := range tab[len(tab)-2:] {
		if !o.Danger || o.Key != "Del" {
			t.Errorf("last: %+v", o)
		}
	}
}

// fixColumn: the editor's row keeps its column's plan ops.
func TestRowOpsColumns(t *testing.T) {
	t.Parallel()
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
	for _, page := range []string{EditorPage, LibraryPage, "home", ChangesPage, SubscribePage} {
		for _, tui := range []bool{false, true} {
			rows := KeyHelp(page, tui)
			history := slices.ContainsFunc(rows, func(r KeyRow) bool { return r.Keys == "alt+← / alt+→" })
			if history == tui {
				t.Errorf("%s tui=%v: page history help must be GUI-only", page, tui)
			}
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

// The launch page's numbered columns are focus keys (16 §2.3, 16 §3.5).
func TestHomeFocusKeyHelp(t *testing.T) {
	t.Parallel()
	if !slices.ContainsFunc(KeyHelp("home", false), func(r KeyRow) bool { return r.Keys == "1–3" }) {
		t.Fatal("home help does not list its three focus keys")
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

func TestLibraryHasOnlyLibraryOps(t *testing.T) {
	t.Parallel()
	for _, where := range []Where{Disabled, Enabled, Staged, Nowhere} {
		m := &store.Mod{ID: "copy", Kind: source.Workshop, Rel: "123"}
		i := &app.LibInstance{Mod: m, Source: app.LibWorkshop, WorkshopID: "123", Path: t.TempDir(), PNG: true, Textures: &app.TextureSummary{Files: 1}, InPlan: where == Enabled, Staged: where == Staged, UsedIn: []app.LibUse{{Kind: "plan"}}}
		l := &app.LibLineage{Instances: []*app.LibInstance{i, {Mod: &store.Mod{ID: "other"}}}}
		lib := &app.Library{St: &app.LoadoutState{Ix: &app.Index{ByID: map[string]*store.Mod{"copy": m}}}, Lineages: []*app.LibLineage{l}}
		if where == Nowhere {
			i.States = []string{app.LibMissing}
		}
		if got := WhereOf(lib, i); got != where {
			t.Fatalf("fixture where %v != %v", got, where)
		}
		codes := CopyOps(lib, l, i, LibraryPage)
		for _, code := range codes {
			if OpOf(code).Group == 1 || code == OpTextures {
				t.Errorf("where %d: unwanted library op %s", where, code)
			}
		}
		for _, code := range []string{OpTags, OpNote, OpWorkshop, OpResub, OpTexClean} {
			if !slices.Contains(codes, code) {
				t.Errorf("missing %s: %v", code, codes)
			}
		}
		for _, col := range []Column{Inactive, Active, Staging} {
			if got := RowOps(lib, "copy", col); !slices.Contains(got, map[Column]string{Inactive: OpEnable, Active: OpDisable, Staging: OpPutBack}[col]) || where != Nowhere && !slices.Contains(got, OpTextures) {
				t.Errorf("editor column %v: %v", col, got)
			}
		}
		i.Locked = &app.Locked{}
		if slices.Contains(CopyOps(lib, l, i, LibraryPage), OpResub) {
			t.Error("resubscribe on locked copy")
		}
		i.Locked = nil
		i.Source = app.LibSteamCMD
		if slices.Contains(CopyOps(lib, l, i, LibraryPage), OpResub) {
			t.Error("resubscribe on SteamCMD copy")
		}
	}
	for _, tui := range []bool{false, true} {
		for _, row := range KeyHelp(LibraryPage, tui) {
			if slices.Contains([]string{"space", "m", "t", "x", "u", "u / ctrl+z", "ctrl+y / ctrl+r"}, row.Keys) {
				t.Errorf("library help: %v", row)
			}
		}
	}
	if !OpOf(OpResub).Confirm {
		t.Fatal("resubscribe requires confirmation")
	}
}

func TestWorkshopUpdateCopyOps(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	for _, kind := range []source.Kind{source.Local, source.Elsewhere, source.Extra, source.Official, source.Workshop, source.SteamCMD} {
		for _, locked := range []bool{false, true} {
			for _, upstreamLive := range []bool{false, true} {
				m := &store.Mod{ID: "local", LineageID: "family", Kind: kind, Status: "live"}
				up := &store.Mod{ID: "workshop", LineageID: "family", Kind: source.Workshop, Status: "live"}
				if !upstreamLive {
					up.Status = "tombstone"
				}
				i := &app.LibInstance{Mod: m, Path: t.TempDir()}
				if locked {
					i.Locked = &app.Locked{Copy: m, Upstream: up, Updated: false}
				}
				ix := &app.Index{ByID: map[string]*store.Mod{m.ID: m, up.ID: up}, ByLineage: map[string][]*store.Mod{"family": {m, up}}}
				l := &app.LibLineage{Instances: []*app.LibInstance{i, {Mod: up}}}
				lib := &app.Library{St: &app.LoadoutState{Ix: ix}, Lineages: []*app.LibLineage{l}}
				want := kind.Managed() && !kind.WorkshopLike() && upstreamLive
				for _, page := range []string{EditorPage, LibraryPage} {
					if got := slices.Contains(CopyOps(lib, l, i, page), OpPinUpdate); got != want {
						t.Fatalf("kind=%s locked=%v upstream=%v page=%s: update=%v", kind, locked, upstreamLive, page, got)
					}
				}
				i.States = []string{app.LibMissing}
				if slices.Contains(CopyOps(lib, l, i, LibraryPage), OpPinUpdate) {
					t.Fatal("missing target offers update")
				}
			}
		}
	}
	op := OpOf(OpPinUpdate)
	if op.Code != "pin-update" || op.Group != 4 || !op.Confirm || TaskLane(op.Code) != app.LaneFiles {
		t.Fatalf("operation contract: %+v", op)
	}
	if Table()["update to the Workshop version"] != "更新至工坊版本" {
		t.Fatal("missing action translation")
	}
}
