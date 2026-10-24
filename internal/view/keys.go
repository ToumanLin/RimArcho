package view

import "github.com/ToumanLin/RimArcho/internal/app"

// The key table (06 §3.5): the plan editor and Mod 库存 › 全部 share one, the page's own keys
// come after it, and `?` lists the keys of the page it is pressed on, the same for the GUI
// and the TUI but for the few each writes in §8.5 and §9.1.

// KeyRow is one line of the help: the keys and what they do.
type KeyRow struct {
	Keys string `json:"keys"`
	What string `json:"what"`
}

// KeyHelp is the help of a page (EditorPage, LibraryPage, or any other page's name) for
// the GUI or the TUI.
func KeyHelp(page string, tui bool) []KeyRow {
	r := func(k, w string) KeyRow { return KeyRow{k, T(w)} }
	pick := func(gui, term string) string {
		if tui {
			return term
		}
		return gui
	}
	var out []KeyRow
	switch page {
	case EditorPage, LibraryPage:
		lib := page == LibraryPage
		space := "enable / disable; in staging, put back in the list"
		if lib {
			space = "enable / disable (the current list)"
		}
		out = append(out,
			r("↑↓ / j k", "move the cursor (in the details: scroll)"),
			r("PgUp PgDn / Home End", "move the cursor a page / to the top or the bottom"),
			r(pick("shift+↑↓ / shift+click", "shift+↑↓"), "select a range"),
			r(pick("ctrl+space / ctrl+click", "ctrl+space / v"), "select or unselect one"),
			r("ctrl+a", "select everything shown"),
			r("enter", "open / close the details"),
			r("esc", "back one level: the details, the selection, the search and filters"),
			r("/  ctrl+f", "search"),
			r(`\`, "open / close the filter bar (in it, ctrl+↑↓ moves its lower edge)"),
			r("space", space),
			r("m", "enable at… / move to…"),
			r("t", "stage / put back"),
			r("x", "switch version…"),
			r("g", "tags"),
			r("n", "note"),
			r("ctrl+o / ctrl+w", "open the folder, the Workshop page"),
			r("ctrl+s", "steam page"),
			r("u / ctrl+z", pick("undo", "undo")),
			r("ctrl+y / ctrl+r", "redo"),
			r("Del", "unsubscribe / move to the recycle bin (asks first)"),
			r(pick("menu / shift+F10 / right click", "menu / shift+F10"), "the row's menu: everything it can do"),
		)
		if lib {
			out = append(out,
				r("→ ←", "open / close a mod's copies; → on a copy goes to its details"),
				r("s", "sort and columns (the headers' menu)"),
				r("C", "this mod's changes"),
				r("R", "rescan"),
			)
		} else {
			out = append(out,
				r(pick("← → / 1–4", "← → / 1–4 / h l"), "go to an area"),
				r("J K / alt+J K", "move down / up 1 (10); with a filter, past the rows shown"),
				r("T", "put everything back from staging; elsewhere, stage everything enabled"),
				r("S / w / L", pick("sort, write to the game, to the launch page (the palette previews them)", "sort, write to the game, launch (the palette previews them)")),
				r(pick("i / e", "e"), pick("import, export the mod list", "export the mod list")),
				r("R / A", "rescan, check the files again"),
				r("P / E", "lists, games"),
				r(":", "run an archo command"),
				r("H", "the history: steps and versions"),
				r("C", "this mod's changes"),
				r("< > =", "column width (= resets)"),
			)
			if tui {
				out = append(out,
					r("i", "technical information in the details"),
					r("r", "add a rule"),
					r("z", "wrap names"),
					r("[ ]", "switch area (narrow)"),
				)
			} else {
				out = append(out, r(T("drag"), "between areas: enable, disable, stage; within Enabled: reorder"))
			}
		}
		if lib {
			out = append(out, r("u", "undo applies to list edits (space, m, t, x)"))
		}
	}
	// Management and its forms use native tab focus, with esc cancelling the draft (06 §2.17).
	if page == PagePlans {
		out = append(out, r("↑↓", "pick a list"), r("enter", "edit"), r("n", "new list"), r("i", "import"), r("e", "export"), r("B", "create a pack"), r("F2", "rename"), r("Delete", "delete…"), r("esc", "cancel"), r("tab", "the next field or button"))
	}
	if page == "plan-import" {
		out = append(out, r("tab", "the next field or button"), r("enter", "import"), r("esc", "cancel"))
	}
	if page == "plan-export" {
		out = append(out, r("↑↓", "format"), r("ctrl+s", "save as a file…"), r("ctrl+c", "copy"), r("esc", "cancel"))
	}
	if page == ChangesPage {
		out = append(out,
			r("↑↓ / PgUp PgDn", "move"),
			r("enter", "open or close a mod's changes"),
			r("i", "the raw file list"),
			r("a", "the current list / the whole library"),
			r("ctrl+w", "the Workshop change notes"),
			r("ctrl+o", "open the location"),
			r("D", "see dependencies (能不能跑)"),
			r("S", "sort the list"),
			r("p", "lock this version (dry run first)"),
			r("esc", "back; with one mod shown, every mod again"),
		)
	}
	if page == PackPages {
		out = append(out,
			r("↑↓ / enter", "the packs: move, open one"),
			r("tab / shift+tab", "the next / previous pack"),
			r("L", "play the pack"),
			r("e", "export the pack to a file"),
			r("E", "export only its list, each mod with its version"),
			r("Del", "move the pack to the recycle bin (asks first)"),
			r("B", "build a pack from the current plan"),
		)
	}
	if page == BuildPageID {
		out = append(out,
			r("n  /", "type the name"),
			r("D", "textures: next choice"),
			r("B / enter in the name", "build"),
			r("V", "see the list first, write nothing"),
			r("tab", "the next field or button"),
			r("esc", "close the preview / back to the list"),
		)
	}
	if page == PageTasks {
		out = append(out,
			r("↑↓ / PgUp PgDn / Home End", "move the cursor a page / to the top or the bottom"),
			r("enter / →", "the task's items"),
			r("←", "back to the tasks"),
			r("x", "stop the rest / cancel the wait"),
			r("r", "try the failed again / do the ones left"),
			r("J K", "a waiting task later / sooner"),
			r("o", "go to the page it changed"),
			r("ctrl+o", "open the folder"),
			r("Del / ctrl+z", "take off the record, put it back"),
			r("C", "take the done ones off the record"),
			r("s / < > =", "the items' headers: sort and columns, the sorted column's width"),
			r("shift+↑↓ / ctrl+space / ctrl+a", "select tasks"),
			r("menu / shift+F10 / right click", "the row's menu: everything it can do"),
		)
	}
	if !tui && (page == LibraryPage || page == "packs" || page == "textures") {
		out = append(out, r("s / < > =", "the headers: sort and columns; < > = changes or resets the sorted column's width"))
	}
	if page == "home" && !tui {
		// The launch page has three numbered columns (06 §2.3).
		out = append(out, r("1–3", "focus the page's areas (the numbers in their headings)"))
	} else if page != EditorPage {
		out = append(out, r("1–9", "focus the page's areas (the numbers in their headings)"))
	}
	out = append(out,
		r("tab / shift+tab", "next / previous page under the same sidebar entry"),
		r("shift+1–8", "go to a page in the sidebar"),
	)
	if !tui {
		out = append(out, r("alt+← / alt+→", "page back / forward"), r("ctrl+b", "navigation at the side / at the top"), r("ctrl+j", "background tasks; again goes back"))
	}
	out = append(out,
		r(pick("c / l", "c"), pick("mod changes, game log", "mod changes")),
		r("ctrl+k", "command palette"),
		r("?", "this help"),
	)
	if tui {
		out = append(out, r("q", "quit"))
	}
	return out
}

// OpKeyed reports whether op code has a key of its own or is reachable from the row menu
// (every op is: the menu lists the row's ops, 06 §3.1 rule 6).
func OpKeyed(code string) bool {
	o := OpOf(code)
	return o.Code != "" && (o.Key != "" || o.Group > 0)
}

// LibOpsOf is CopyOps of the copy ref in the library (nil: not there).
func LibOpsOf(lib *app.Library, ref string, page string) []string {
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if i.Ref == ref {
				return CopyOps(lib, l, i, page)
			}
		}
	}
	return nil
}
