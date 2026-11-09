package view

import "github.com/ToumanLin/RimArcho/internal/app"

// The key table (16 §3.5): the plan editor and Mod library > All share one, the page's own keys
// come after it, and `?` lists the keys of the page it is pressed on, the same for the GUI
// and the TUI but for the few each writes in 16 §8.5 and §9.1.

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
			r("g", "tags"),
			r("n", "note"),
			r("ctrl+o / ctrl+w", "open the folder, the Workshop page"),
			r("ctrl+s", "steam page"),
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
			if !tui { // the Tags view is the window's (16 §2.9)
				out = append(out,
					r("v", "every mod / group by tag"),
					r("F2 / n / M / Del", "on a tag's group row: rename, note, merge, delete the tag"),
				)
			}
		} else {
			out = append(out,
				r("space", "enable / disable; in staging, put back in the list"),
				r("m", "enable at… / move to…"),
				r("t", "stage / put back"),
				r("x", "switch version…"),
				r("u / ctrl+z", "undo"),
				r("ctrl+y / ctrl+r", "redo"),
				r(pick("← → / 1–4", "← → / 1–4 / h l"), "go to an area"),
				r("J K / alt+J K", "move down / up 1 (10); with a filter, past the rows shown"),
				r("T", "put everything back from staging; elsewhere, stage everything enabled"),
				r("S / w / L", "sort, save, to the launch page (the palette previews them)"),
				r(pick("i / e", "e"), pick("import, export the mod list", "export the mod list")),
				r("R / A", "rescan, check the files again"),
				r("P / E", "lists, games"),
				r(":", "run an archo command"),
				r("H", "the history: steps and versions"),
				r("C", "this mod's changes"),
				r("< > =", "column width (= resets)"),
				r("r", "add a rule"),
			)
			if tui {
				out = append(out,
					r("i", "technical information in the details"),
					r("z", "wrap names"),
					r("[ ]", "switch area (narrow)"),
				)
			} else {
				out = append(out, r(T("drag"), "between areas: enable, disable, stage; within Enabled: reorder"))
				// the Disabled column's tag groups are the window's (16 §2.5)
				out = append(out,
					r("v", "Disabled: one row per copy / grouped by tag"),
					r("← → / enter / click", "on a tag group's header in Disabled: collapse / expand"),
				)
			}
		}
	}
	// Management and its forms use native tab focus; esc closes the form (16 §2.17).
	if page == PagePlans {
		out = append(out, r("↑↓", "pick a list"), r("enter", "edit"), r("n", "new / import"), r("i", "import"), r("F2", "rename"), r("D", "duplicate"), r("Delete", "delete…"), r("e", "export"), r("B", "create a pack"), r("esc", "close"), r("tab", "the next field or button"))
	}
	if page == "plan-import" {
		out = append(out, r("tab", "the next field or button"), r("enter", "import"), r("esc", "cancel"))
	}
	if page == "plan-export" {
		out = append(out, r("↑↓", "format"), r("ctrl+s", "save as a file…"), r("ctrl+c", "copy"), r("esc", "cancel"))
	}
	if page == ChangesPage {
		if !tui {
			out = append(out, r("+", "add a subscription"), r("K", "check for updates"), r("U", "update all"), r("u", "update"), r("1 / 2", "list / details"))
		} else {
			// 16 §9.1: the subscription actions, each a background task (16 §2.16)
			out = append(out,
				r("+ / paste", "add a subscription"),
				r("K", "check every source for updates (enter on a source: that one)"),
				r("u / U", "update this one / every one shown (dry run first)"),
				r("Del", "remove the subscription (dry run first)"),
			)
			out = append(out, r("1 / 2 / 3", "sources, waiting for you to update, already updated"))
		}
		out = append(out,
			r("l", "show in the mod library"),
			r("↑↓ / PgUp PgDn", "move"),
			r("enter", "open or close a mod's changes"),
			r("i", "the raw file list"),
			r("a", "the current list / the whole library"),
			r("ctrl+w", "the Workshop change notes"),
			r("ctrl+o", "open the location"),
			r("D", "see dependencies (can it run)"),
			r("S", "sort the list"),
			r("p", "lock this version (dry run first)"),
			r("esc", "show every mod"),
		)
	}
	if page == SubscribePage && tui { // the window's form is buttons and native fields
		out = append(out,
			r("type / paste", "the address: a Git repository, a download page, Workshop links or IDs"),
			r("↑↓", "choose what the card asks"),
			r("tab", "the branch, tag or commit name"),
			r("enter", "subscribe, once the dry run is shown"),
			r("ctrl+u", "clear the box"),
			r("esc", "clear, then back to updates"),
		)
	}
	if page == PackPages {
		out = append(out, r("↑↓ / enter", "the packs: move, open one"))
		if tui {
			out = append(out, r("tab / shift+tab", "the next / previous pack"))
		}
		out = append(out,
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
	if page == RulesPage { // the TUI's Load-order rules screen (16 §9.1); the window's card has buttons
		out = append(out,
			r("↑↓ / PgUp PgDn / Home End", "move the cursor"),
			r("a", "only the current list / all load-order rules"),
			r("r", "add a rule for the mod at the editor's cursor"),
			r("enter", "change the rule"),
			r("Del / u", "remove the rule; u right after puts it back"),
			r("o", "show its mod in the list"),
			r("tab / shift+tab", "in the form: the next / previous field"),
			r("← →", "in the form: the kind"),
			r("esc", "close the form; back to the editor"),
		)
	}
	if page == PageSearch { // Search files (16 §2.9)
		out = append(out,
			r("/  ctrl+f", "search"),
			r("alt+c / alt+r", "case, regex"),
			r("tab", "complete"),
		)
		if tui {
			out = append(out, r("enter", "in the box: search; on a result: open its folder"), r("↑↓", "fields"))
		} else {
			out = append(out, r("enter", "in a field: search; on a result: open the file"), r("\\", "filter"), r("n / N", "next / previous match"), r("alt+z", "wrap lines"))
		}
		out = append(out,
			r("↑↓ / j k / PgUp PgDn / Home End", "move the cursor"),
			r("← →", "collapse / expand"),
			r("ctrl+o", "open the folder"),
		)
		if !tui { // the terminal selects text itself (16 §3.1 rule 12)
			out = append(out, r("ctrl+c", "copy the path"))
		}
		out = append(out, r("esc", "stop the search; in the box: leave it"))
	}
	if page == PageModSettings && !tui { // Mod settings (16 §2.9); the TUI's is a panel
		out = append(out,
			r("↑↓ / j k / PgUp PgDn / Home End", "move the cursor; the values show the cursor's file"),
			r("1 / 2", "focus the table / the values"),
			r("ctrl+o", "open the Config folder"),
		)
	}
	if page == PageSaves && !tui { // Saves (16 §2.11); the TUI has the saves commands
		out = append(out,
			r("/", "search colonies and saves; esc clears it"),
			r("↑↓", "the previous / next save in the tree"),
			r("← →", "fold / unfold the colony"),
			r("enter", "the quick fix: launch, recycle or make a list (and analyse it when the save's Mods are not)"),
			r("F2", "rename the save"),
		)
	}
	if !tui && (page == LibraryPage || page == "packs" || page == "textures" || page == PageModSettings) {
		out = append(out, r("s / < > =", "the headers: sort and columns; < > = changes or resets the sorted column's width"))
	}
	if page == "home" && !tui {
		// The launch page has three numbered columns (16 §2.3).
		out = append(out, r("1–3", "focus the page's areas (the numbers in their headings)"),
			r("w", "save the current list's edits to the game without launching (while there are any)"))
	} else if page == "home" {
		// The terminal's is one column: the current list and its launch key (16 §9.1).
		out = append(out, r("L / enter", "launch the game"),
			r("w", "save the current list's edits to the game without launching (while there are any)"),
			r("P", "lists"), r("esc", "back"))
	} else if page != EditorPage && page != RulesPage { // Load-order rules is one list
		out = append(out, r("1–9", "focus the page's areas (the numbers in their headings)"))
	}
	// The GUI's tab moves the focus on every page (it reaches the address copy buttons); the
	// terminal's goes to the next sub-page (16 §3.5).
	tabText := "next / previous page under the same sidebar entry"
	if !tui {
		tabText = "the next field or button"
	}
	if page != RulesPage { // its tab is the form's (above)
		out = append(out, r("tab / shift+tab", tabText))
	}
	out = append(out, r("shift+1–8", "go to a page in the sidebar"))
	if !tui {
		out = append(out, r("alt+← / alt+→", "page back / forward"), r("ctrl+b", "navigation at the side / at the top"), r("ctrl+j", "background tasks; again goes back"), r("ctrl+\\", "reference numbers on every control, text and image"))
	}
	out = append(out,
		r(pick("c / l", "c"), pick("updates, game log", "updates")),
		r("ctrl+k", "command palette"),
		r("?", "this help"),
	)
	if tui {
		out = append(out, r("q", "quit"))
	}
	return out
}

// OpKeyed reports whether op code has a key of its own or is reachable from the row menu
// (every op is: the menu lists the row's ops, 16 §3.1 rule 6).
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
