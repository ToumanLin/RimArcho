# 16 · Interface · Interaction rules (§3)

> Part of [16 · Interface](README.md). Section numbers run through all of 16 and stay unchanged when split across files.

## 3. Interaction rules

These rules apply in full to the plan editor and Mod library › all mods. The two pages share the interface, filter bar, key table and context menu. Other pages follow the same principles: arrows only navigate, buttons or verb keys act, and `esc` returns one level. The GUI and TUI share keyboard bindings except where §8.5 and §9.1 state otherwise.

### 3.1 Rules

1. **Arrows only navigate; they never change data.** `↑↓←→` only move the cursor, change the focus or scroll. The exceptions are the GUI's off / on segmented controls (§3.10) and a focused slider (the background's dimming and blur, §2.14), which set their own value as native controls do. Arrows never move Mods, enable them or apply tags. GUI clicks and the mouse wheel do not change data either.
2. **Verb keys act at once, and `u` undoes them.** An action applies to the selected Instances, or to the cursor row if nothing is selected. It enters no mode and needs no separate drop or cancel step. Each verb makes one app edit and one undo step.
3. **The current position is always visible.** The position line identifies the focused area, cursor index, selection count, and any open overlay (§3.2).
4. **Available destinations are always visible.** The hint line lists up to 7 actions available for the current objects. `?` lists all actions, and `ctrl+k` reaches anywhere from anywhere.
5. **Keep overlays to a minimum.** The overlays are the command panel, tag candidates, position input, confirmation, help, and version list (§5). Each has a title and appears in the position line. `esc` closes it, and overlays never nest. The GUI uses the same panel for `:` and `ctrl+k` (§3.5); the TUI uses `:` for the command line (§9.4). The `r` rule input appears only in `?`. In the GUI, `r` expands an inline form in the editor's sorting rules section rather than an overlay (§2.5).
6. **Everything the mouse can do must also be possible with the keyboard.** Every click, right-click, drag, and double-click on every page has a key or can be reached through keyboard focus. Row actions have their own keys or appear in the context menu. The Menu key and `shift+F10` open the same menu at the cursor row: `↑↓` selects an item, `enter` executes it, and `esc` closes the menu. The keys displayed in the menu also execute their actions directly. This applies equally to filters, headers (`s`), history (`H`), column widths (`<` `>` `=`), and detail links (`C`, `ctrl+o`, `ctrl+w`, `g`, `n`). New features get keyboard access before mouse access. `TestOpsReachable` in view and `TestOpsRunnable` in the GUI check that each action has a key or menu entry and runs on both pages.
7. **Dialogs are only for warnings and prompts.** A dialog makes the player deal with it before doing anything else, which interrupts free navigation. **Unsolicited dialogs** are only for decisions that must be made immediately: confirming an irreversible action (§6), or presenting commands that resolve a refusal. **Temporary layers that the player opens and can dismiss with `esc` are not dialogs.** These include the command panel, context and `⋯` menus, tag candidates, position input, help, the identity question's why-this-question explanation (§5), and the version list (§5). A finding's explanation and action buttons (§5) may also appear in a temporary layer that the player opens and dismisses with `esc`. Forms such as new list and import, other views such as lists and diffs, progress, and records belong on pages, subpages, or inline sections. They never become dialogs: background tasks have a page (§2.16), and import has a subpage (§2.17).
8. **Collapse content that is not in use.** Sections not needed at the moment shrink to one line. The expanded filter bar **collapses immediately when focus leaves it**, whether by an outside click, `tab`, or `esc`. Moving focus into a dropdown or input opened by the filter bar does not count as leaving it. Effective conditions remain visible in the collapsed line. Long lists go behind N more, and finished or secondary information is muted. Collapsing a section never discards text the player has entered.
9. **Every action must respond, and background work must not block foreground work.**
   - **Foreground work** means entering and reading pages. **Enter the page before reading its data**: navigate immediately to a skeleton or the previous content, then fill in the data when it arrives. Late results must never overwrite a newer page (§8.2).
   - Every click and keypress produces a visible response within **100ms**. A pressed state, skeleton, status `…`, or progress indicator counts as a response.
   - **Background work** includes anything likely to take more than about 100ms or access the disk or network: moving or deleting files, unsubscribing, downloading, importing a game and scanning it for the first time, initializing Mods through scanning and analysis, calculating sizes, and reading data when switching games. None of this runs on the UI thread. Other pages remain usable, and only conflicting actions are disabled, with a reason. For example, the file queue waits during a scan (§2.16).
   - A new request for the same object invalidates the previous request. If the player changes a dropdown rapidly, only the latest value is computed; cancel the old computation or discard its result.
   - Work that the player initiates, takes time, and produces a result to inspect becomes a background task (§2.16). Incidental calculations such as sizes and page reads only show progress where they occur.
10. **Splitters between cards that share space are draggable** (§7). Adjacent cards that share a row or column can be resized by dragging the gap between them. A double-click resets their sizes, which are saved per page. The gap is a focusable separator with `role="separator"`, reachable with `tab`. When focused, arrow keys move it 40px per press, and `enter` resets it. The filter bar keeps its `ctrl+↑↓` binding (§2.5). Cards whose heights follow their content and are stacked vertically, such as can it run sections and settings groups, do not have draggable gaps.
11. **Pages support back and forward navigation** (GUI; the TUI exception: §8.5). XButton1 goes back, and XButton2 goes forward. The keyboard bindings are `alt+←` / `alt+→`; `BrowserBack` / `BrowserForward` are also accepted.
   - Record each completed page entry, whether reached through the sidebar, panel, breadcrumbs, links, or a subpage. If the player goes back while a page is still loading, retain that explicitly opened destination for a forward revisit. Reopening the current page, refreshing it, and receiving late read results add no entries. Visiting a new page after going back discards the forward branch. Keep only the latest 50 entries in this session, without writing them to disk.
   - Back first closes the top overlay or menu and stops there. Within the technical-page stack, it follows the one-level `backspace` rule; GUI technical information is inside full-screen details, so back returns to the list that opened those details. Otherwise, back visits the previous history page. Forward visits the next history page. If no destination exists, stay on the current page.
   - Skip deleted plans or packs, completed guides, and other pages that cannot be entered now. Never switch games automatically to revisit history. Continue to respect the navigation lock during and after uninstallation (§2.14).
   - Revisit pages through `go()`, following rule 9: enter first and invalidate stale reads. Forms use the normal behavior for leaving and reentering a page, without an unsaved-changes confirmation. On All lists, new drafts are kept per game while the app stays open; drafts whose names were never edited are discarded when the player leaves the form (§2.17). A list's details on All lists and the new-list view are separate history entries. Navigation never edits, undoes, imports, or builds data.
   - The GUI intercepts the default side-button down, up, and auxiliary-click behavior so each press moves exactly one step. It does not use browser history or leave the Wails page address.
12. **Addresses support display, copying, and editing** (GUI). Files, folders, and URLs share `copyablePath` / `pathContent` in `fields.js`; so do copyable identifiers in the detail field list (Archo ID, package ID, Workshop ID). A click on the text copies nothing. The text is selectable, and a trailing copy button shows the copy tooltip, briefly changes to ✓ on success, and always copies the full value. Display the address on one line with a middle ellipsis based on its actual width, preserving the initial directory and final directory or filename where possible. Recalculate on resize and show the full value on hover. Keep a readable minimum width for the address; wrap adjacent buttons first when space runs out. The copy button is reachable with `tab` and works with Enter or Space. Clicking read-only address text does nothing; separate actions open its folder or web page. For an editable local address, clicking the text or change… / add… opens the system folder or file picker directly, with a tooltip explaining this. Apply the selection immediately, leave the value unchanged on cancel, and retain any existing preview or confirmation. Do not provide a manual-path input. URLs are normally read-only; change… opens a small dialog with one input that validates a complete HTTP(S) address. Cancel discards it, OK saves it, Enter accepts it, and Esc cancels it. Relative filenames may display relative paths, but hover and copy use the full path supplied by the data. The TUI keeps text entry and full addresses without a native picker. CLI and MCP commands are not affected.
13. **Text is selectable** (GUI). By default, the player can select visible text and copy it with `ctrl+c`: Mod names, paths, descriptions, detail fields, raw logs, finding explanations, hover and pinned hints, dialog bodies, and status text. Controls are excluded: buttons, the sidebar and breadcrumbs, area headings and splitters, tag chips, and rows that support multiple selection in the editor's three lists, the Mod library table and background tasks. Pressing these rows selects them, `shift` and `ctrl` extend the selection, and dragging moves Mods; select their names in details instead. In `style.css`, the body uses `user-select: text`, and these controls use `none`. New controls follow the same rule. For addresses (rule 12), `ctrl+c` copies the selected text, while the trailing button copies the full address. The TUI leaves selection to the terminal.

**Base state** means the current page with no selection, overlay, or menu, and with collapsed filters. In the editor, the focused list also has no search. Filters that remain effective while collapsed are part of the page state and do not prevent the base state; another `esc` clears them. At most 3 presses of `esc` return to the base state from anywhere (T20, G5):
- Close overlays and menus first, including GUI context and `⋯` menus opened with the mouse while focus is elsewhere. Then return one level at a time: details that cover the page or have focus → selection → filters. From a row, clear and collapse filters together; from inside the filter bar, only collapse it.
- A secondary screen, such as full-screen details, a technical page, an expanded can it run item, or TUI pack or settings text, returns to its opener. In technical pages, `esc` closes the entire stack, while `backspace` returns one level. This keeps the limit at 3 presses at any depth.
- Esc does not move between peer sidebar pages. Use the sidebar, breadcrumbs, or `ctrl+k` to change pages, and use the side buttons or alt arrows for history (rule 11).

### 3.2 Position

The editor has four focus areas: `1` Disabled, `2` Enabled, `3` Staging / history, and `4` details. Number keys jump directly to an area, and the number appears before its heading in muted text.

- The focused heading uses the accent color, and its cursor row has a faint background across the full row (TUI `#1A1C24`). An unfocused cursor only changes the name to the text color, without a background or `▸`. When details have the focus, all lists look unfocused, so there is only one brightest place on the screen.
- The details heading is `4 details`, changing to `▲ name` when scrolled. It uses the accent color when focused. Content begins on the next line, aligned with the first list row.
- The position line appears at the top right in the TUI, and on the left of the status line in the GUI editor and full-screen details:
  - A list shows `Enabled  322 / 488  selected 3`. Positions use the visible row count after filtering; add filtered when a search is active.
  - Details show `details  Combat Extended  description 40%`. Use the section heading above the current top line, such as summary, description, history, or technical, followed by the scroll progress.
  - Append an open layer, for example `Enabled  322 / 488 › tags`. Search adds `› search`, and a confirmation waiting for the second keypress adds `› confirm`.
  - Full details show `Enabled › Combat Extended (322 / 488) › Harmony`.
  - On other pages, including full-screen details, technical pages, can it run and updates, the breadcrumbs serve as the position line.

### 3.3 Arrows

| State | `↑↓` | `←→` |
| --- | --- | --- |
| List | Move the cursor | Change the focus area |
| Details | Scroll the details | Change the focus area |
| Full-screen details | Scroll the content | Visit the previous or next Mod; this is still navigation |
| Overlay: panel, tags, position | Move among options | Move among options |

- `PgUp` / `PgDn` move the list cursor one page, and `Home` / `End` move it to the ends, on both pages. With details focused, these keys scroll the details instead. The mouse wheel scrolls details wherever the pointer is; it does not move the list cursor. Use keys or clicks to navigate lists.
- From details, `←` returns to the list containing the displayed Mod, rather than always returning to area 3. At a width of 130 columns, Staging is not visible, and area 3 would show another Mod when arriving from Enabled. From Staging, area 3, `→` still enters area 4. Arrows do not wrap. Only arrows and `1`–`4` change areas; tab moves the focus in the GUI and changes subpages in the TUI (§3.5).
- In the TUI, `j` `k` mean down and up, and `h` `l` mean left and right. Home / End go to the list ends; `g` opens tags, and `G` is unused.
- The description initially shows only 12 lines. Scrolling or focusing details reveals the full description. A PgDn pressed before details finish loading is retained until they are ready.

### 3.4 Selection

- `shift+↑↓` extends a contiguous selection from the cursor.
- `ctrl+space`, or `v` in the TUI, toggles the cursor row. `ctrl+a` selects all displayed rows, which means the filtered rows when a filter is active. Normal arrows and ctrl arrows **do not** clear the selection, so moving the cursor and toggling rows allows scattered selection.
- A click moves the cursor, shift-click selects a range, and ctrl-click toggles a row. A click in the same list preserves the selection, just as arrows do; clicking a different list clears it.
- Esc clears the selection, as does changing to another list area. The hint warns `←→ changing lists clears selection`.
- Selected rows have a faint accent background across the whole row, with a deeper shade at the cursor, in both the GUI and TUI. **Do not draw a vertical bar at the left of a row or card** (§7 principle 4). The hint becomes `selected 3  space disable  t stage  m move  g tags  esc clear selection`. This is only a hint line, not a popup or mode.
- After an action, `g` retains the selection so it can be repeated. Space and t clear it because the rows leave the list. J / K and m within Enabled retain it.

### 3.5 Verb keys

The editor and Mod library › all mods share **one key table**. The GUI and TUI use the same bindings except in §8.5 and §9.1. `?` shows the current page's `view.KeyHelp`. In the GUI, a focused button, link or disclosure keeps its own Enter and Space (native activation), so the table's `space` and `enter` act only when no such control has the focus.

| Key | Action on selected objects, otherwise the cursor row |
| --- | --- |
| `↑↓` / `j` `k` | Move the cursor, or scroll details |
| `shift+↑↓`, `ctrl+space`, `ctrl+a` | Select a range, toggle one row, or select all displayed rows |
| `PgUp` `PgDn` `Home` `End` | Move the cursor by a page or to an end, or scroll focused details |
| `enter` | Open full details (§4.2); the editor and the Mod library use the same page |
| `esc` | Return one level: details → selection → search and filters |
| `/`, `ctrl+f` | Search in the filter bar; Staging has its own search |
| `\` | Expand or collapse the filter bar (§2.5) |
| `space` | Make the objects effective or ineffective in the current plan. In the editor, enable or disable them; in Staging, put them back. Enable at the position suggested by sorting constraints and show ✓ placed at 352, m to move. Keep the cursor in Disabled for consecutive enables. An immediate m moves the objects just placed; any other key forgets them |
| `m` | Editor only: enable at or move to a position through the position input (§3.6) |
| `t` | Editor only: stage or put back objects, retaining Staging |
| `x` | Editor only: switch to another copy of the same Mod (`swap`) |
| `g` | Open tags (§3.8); use Home to go to the top in the TUI |
| `n` | Edit the Mod's Lineage note in the editor (`archo note`), or this copy's note in the Mod library |
| `ctrl+o`, `ctrl+w` | Open the folder or the Workshop page. Without an ID, show “no Workshop ID: not a Workshop mod”. The GUI calls `preventDefault` so that Wails does not take these keys. Their hint is combined as ctrl+o ctrl+w folder / workshop |
| `u`, `ctrl+z` / `ctrl+y`, `ctrl+r` | Undo or redo in the editor |
| `Del` | Unsubscribe the Workshop copy or delete an owned copy, with a dry run before confirmation |
| Menu key, `shift+F10` | Open the context menu at the cursor (§8.5) |
| `shift+1`–`shift+8` | Visit the 8 sidebar entries: Launch, Can it run, Game log, Lists (opens the current list), Packs, Mod library, Texture optimisation and Game (the environment page). Saves has no key. The GUI uses physical keys, so they work in any keyboard layout; the TUI receives `!@#$%^&*` in the US layout. `!` and `#` have no other meaning |
| `tab`, `shift+tab` | GUI: move the focus to the next or previous control, as on a web page, including the address copy buttons (§3.1 rule 12); in a search field, Tab first accepts the ghost completion (§3.7). Subpages are switched with the sidebar, the breadcrumb or the command panel. TUI: visit the next or previous sibling subpage. The Mod library cycles through all mods ↔ updates ↔ add a subscription ↔ search files. Plans open for editing without changing the current plan (07 §1.3). Packs cycle through the packs, starting with the first; full details use Tab to switch their own pages |

Editor only:

| Key | Action |
| --- | --- |
| `←→`, `1`–`4` | Change areas; tab does not switch areas |
| `J` / `K`, `alt+J` / `alt+K` | Move down or up 1 position in Enabled, or 10 with alt; `alt+j` / `alt+k` also work. A terminal cannot distinguish shift+J from J, so alt handles 10 positions. Filtered moves cross displayed rows (§2.5) |
| `T` | In Staging, put all staged Mods back at their original positions; elsewhere, stage all Enabled Mods. The TUI does the same |
| `S` / `w` / `L` | Sort, save or launch the whole plan. Sorting is undoable, and previews are in the panel. L goes to Launch with the launch button focused; another L launches. The TUI does the same (§2.5, §9.1) |
| `i` / `e` | Import or export in the GUI; TUI i toggles technical details |
| `R` / `A` | Rescan, or check the files again. In the TUI, A works in the editor and on Can it run |
| `c` / `C` | Open updates for all Mods, or only the cursor Mod's Lineage (§2.9) |
| `H` | Open the picker for history steps and versions. The TUI has it too, alongside area 3 history |
| `<` `>` `=` | Narrow, widen, or reset the focused column |
| `r` | Add a rule for the cursor Mod through the sorting-rules form (§2.5), in the GUI left column or a TUI secondary screen. In full details, r refetches the Workshop description |
| `P`, `E` | Switch, create, rename, or delete plans; switch environments (the environment page in the GUI) |

Global:

| Key | Action |
| --- | --- |
| `c` | Open updates for all Mods |
| `l` | Open the game log; on the TUI's Can it run, open the log picker |
| `ctrl+k` | Open the command panel |
| `alt+←` / `alt+→` | Go back or forward in the GUI (§3.1 rule 11) |
| `ctrl+b` | Switch between side and top navigation in the GUI (§2.1) |
| `ctrl+\` | Toggle GUI reference-number mode. Give every control, paragraph, image, and chart a thin red box and an upper-left number in white on red: the page abbreviation plus a sequence number, such as lib12, so feedback can say change lib12. Count only rendered objects that are visible and uncovered, in page order; frames follow scrolling. A number stays with its element across visits and restarts (GUI setting `ui.gui.refnums`), so a page can show gaps; ctrl+shift+\ numbers the page afresh. Once the page is still, and at the latest 1.5 s after a change, the page's numbers are written to `refnums/<prefix>.txt` in the data folder (17 §3): a line per number with whether it is shown now, its place in the page and its text, so a number can be looked up without the window (`refnums.js`). The overlay does not block clicks or change the layout |
| `1`–`9` | Focus page areas, on every page, with numbered headings like editor areas 1–4. Launch has 1 game, 2 list or pack, 3 launch (§2.3); the Mod library has 1 filters, 2 table, 3 details; updates has 1 the list (waiting and updated), 2 details. Can it run follows the displayed section order and selects the first item in the section. The game log has 1 errors and 2 raw text; the arrows scroll the raw text when it has the focus. The environment page has 1 all games, then its settings groups; Settings uses its groups, and building a pack uses its sections. The GUI gives the selected area an accent border. The TUI's Can it run and Mod library follow the same rule: 1 opens the filters, and 3 opens copy details, filling the screen when space is short |
| `:` | Open the unified GUI panel from any page, like ctrl+k; open the TUI command line (§9.4) |
| `?` | Show all keys for the current page |
| `q` | Quit the TUI |

**Command panel**, opened with ctrl+k or colon in the GUI, and ctrl+k in the TUI:
- The GUI panel sits at the bottom of the page, above the status line, with the width and look of an ordinary overlay. Results and CLI output grow upward from the input. Long output can be scrolled, selected and copied. Closing keeps the draft and the latest output; opening another layer closes the panel first, so layers never stack.
- The contents are the commands, by their name in the interface language and their English alias (`view.Commands`), all Mods to jump to, the other environments to switch to, and the lists to open. The same input also runs archo commands, without a separate box. The mouse or ctrl+n/p selects results and runs items; ctrl+enter runs the input directly.
- Scores are prefix or suffix 6 > substring 5 > word starts 4 > subsequence 3 > word starts with typos 2 > description 1. A suffix scores like a prefix because a Chinese name ends in its head noun: searching for sort gives automatic sorting and sorting rules the same score, and automatic sorting comes first because it comes first in the command table. **Word starts**: the input splits into words, and each word must begin a different word of the name or alias, in any order; names split at spaces, punctuation and lower-to-upper case changes, so `ad cu` finds Add Cube and `lib hug` finds HugsLib. **Typos**: the same, but a query word of 4–7 letters may be one edit from the start of its name word and one of 8 or more two edits; an edit inserts, deletes or changes a letter or swaps two neighbours (`sotr` finds sort). Shorter words must match exactly, so short queries stay precise. Descriptions match by substring or word starts (no typos); they include keys, so keys can be searched too. Ties go to commands first, then Mods, then environments, then shorter names. Empty input lists only commands and other environments. Ranking 6,000 Mods takes about 6 ms per keystroke (`BenchmarkPalRank`).
- In the GUI, typing filters the results, and up/down browses the command history. Tab completes a command, argument or object; pressing Tab again cycles through the candidates, and Enter accepts a completion without running it. ctrl+n/p selects results, and Enter runs. Esc or ctrl+k closes the panel and restores the focus; ctrl+u clears the input. A colon typed in the input stays literal text. The TUI panel uses the arrows and Tab to select results; the TUI command line: §9.4.
- Both interfaces share the quote parsing, completion and history (the last 50, without duplicates) in `internal/cmdline`. They store their histories separately. Per-command `--env`, Windows backslashes and `#` keep their CLI/TUI meaning. Commands do not go through a shell.
- The GUI checks commands with `cmds.Check` and calls `ExecuteContext` in the background, in a separate session. It runs one command at a time without holding the page lock. Only commands still running after one second get a row that can be stopped on the background tasks page (§2.16); quick commands report only in the panel. Scans and analysis run in subprocesses. `log --follow` is refused; the raw CLI output and the exit code are kept, and destructive commands still need an explicit CLI `--yes`.
- Selecting a Mod jumps to its column and selects its row. If that column's search hides the Mod, the search is cleared.

### 3.6 Position input (`m`)

```
 move 3 to ❯ harm▌
   after  Harmony                       (position 1)
   after  Harmony Patches for VFE        (position 212)
   position 352 (suggested)
   first   last
```

- A number means move to position N. A Mod name produces up to 5 fuzzy matches, ordered by prefix, substring, then alphabetically. The default is after the matched Mod; Tab changes it to before while retaining the selected row.
- Enter with empty input uses the suggested position from `App.SuggestAt`. It follows the sorter's constraint edges: immediately after the last enabled predecessor, before the first target if there are only before constraints, or at the end if unconstrained. The official-content-first edge alone does not count as a constraint. With constraints, the suggestion never precedes official content; for example, a Mod that only depends on Harmony is not placed between Harmony and Core. Remove already enabled objects before calculating the position, so m also suggests positions for moves.
- Each option occupies one row, except first and last, which share a row. Both arrow pairs move among options. Positions refer to the list after removing the Mods being moved, as used by `MoveBlock` and `Enable`; the number beside a candidate's name is its current position.
- For objects in Disabled or Staging, m enables them at the selected position. Enabling multiple staged objects and removing them from Staging is one undo step.
- The input occupies the bottom rows of the list. Its data comes from `view.PosOptions` and `view.Place`.

### 3.7 Search (`/`)

- Search is part of the shared filter bar (§2.5) and filters Disabled and Enabled together, with immediate results. Enter ends input, but the filter remains until Esc clears it. The heading shows `Enabled 12 / 485`. Staging has a separate search with the same rules.
- Both lists are filtered together, so Tab does not jump to another list; it only accepts the ghost completion.

A query consists of space-separated words. All free words must match (AND). Structured conditions use OR within the same key and AND across keys. Exclusions remove matches individually:

| Syntax | Meaning |
| --- | --- |
| `word` | Case-insensitive substring in the name, alias, or author. If the complete query has no matches, free words fall back to an ordered subsequence of name letters: hsl finds Hair Style Lib |
| `name:word` / `author:word` / `tag:word` | Match only the name including its alias, author, or tag, respectively. tag: matches the prefix of any word in a tag: tag:facial finds Facial Hair and Facial Animation, tag:animation finds only the latter, and tag:"facial hair" finds only the former |
| `id:value` | Match a package ID substring, an exact Workshop ID, or a `#reference` |
| `tags:value` | Select an exact tag; filter clicks use this syntax too. The tag: prefix query also works |
| `lists:"<name>"` | Keep the exact Mod copies enabled by the named list, case-insensitively. Quote names containing spaces. Unknown names match nothing; `-lists:"<name>"` excludes its copies. Staged entries and packs do not count. Completion offers real list names |
| `source:value` | Match the directory role official, workshop, local, steamcmd, locked, or other, or a stable custom-directory alias (03 §2.2) |
| `content:value` | Match assembly, xml, or language content |
| `type:value` legacy | Match sources official, workshop, local, copy, git, or steamcmd; or content assembly (has a DLL), xml (no DLL, also xml-only), or language (language files only, also language-pack) |
| `has:problem` / `has:error` | Match an actionable can it run item or a finding that causes loading to fail, respectively. Also supports has:tag, has:note, and has:alias |
| `version:1.6` | Match declared support for this game version |
| `-word`, `-tag:x` | Negate a condition |
| `"with spaces"` | Treat the quoted text as one word |

In the GUI, clicking search or pressing slash opens a small popup below the input. It matches the word at the cursor against available keys and values, including tags, sources, content, versions, and authors. Raw location: is not a candidate. The first candidate appears as a ghost suffix, which changes when up/down selects another candidate. Tab accepts it without changing the arrows' editing behavior in the input. Values containing spaces are quoted automatically, and completion works in the middle of a query. The popup shows no Mod counts or repeated tag text. Fixed help is localized; author, tag, and custom-directory names remain unchanged as data. Enter applies the query and leaves the input; Esc closes the popup first, then leaves the input. Filter clicks add or remove conditions in the same query text without creating chips. Legacy tag:, type: and location: queries still work, and the TUI keeps its own text completion.

In the GUI editor, arrows follow the physical order: details → Enabled → Disabled. Numbers `1`–`4` retain the logical area IDs, with Staging in the right column. Space enablement and undo rules remain unchanged.

Content types such as `type:assembly` come from the current snapshot's `snapshot.kinds`, calculated from the file list when the snapshot is written (04 §4) and read by primary key. Load them on first use and retain them until the database changes (`view.ContentCache`).

### 3.8 Tags

- Mod tags appear in the detail fields (§4.1), next to edit. They never appear in list rows, where they would displace long names and be unreadable across more than a thousand rows. Find tagged Mods with tag: search (§3.7), the Mod library's tag filters, or its Tags view, which groups the rows under a header row per tag (§2.9).
- Tags belong to the Mod's Lineage (07 §6). The corresponding interfaces are CLI `tag add|remove <ref> <tag…> --mod <ref>…` and MCP `set_player_data`. Renaming, merging, deleting a tag and its note are on the Mod library's Tag manager (§2.9), CLI `tag rename|merge|delete|note` and the same MCP tool.
- Tags are player data rather than list edits, so they do not enter the undo stack. u does not undo them, and the GUI status line offers no undo link for a tag change.

`g` opens a candidate row below the hints or details, rather than an input box:

```
 ✎ type…   [framework] [visuals] [apparel] [races] [weapons] [buildings] [QoL] [testing] …
           ▲ first candidate initially selected
```

- Both arrow pairs move the selection. Enter applies the tag and closes the candidate row while retaining the object selection, so g can be repeated. Esc closes the row.
- The ✎ type… entry is before the first candidate. Press left from the first candidate to reach it, then type and press Enter to create and apply a tag. Typing while any candidate is selected also jumps to this entry.
- A tag on **all** objects appears as `[✓framework]`; Enter removes it from those objects. A tag on only some objects appears as `[~framework]`; Enter applies it to all of them.
- The tags the selected objects already have come first: those on all of them, then those on some. The rest follow: pinned tags (`archo config set tags.pinned "framework,visuals,…"`, accepting both ASCII and full-width commas), then existing tags ordered by usage count. Each group keeps that pinned-then-usage order. The row scrolls horizontally when needed. Data comes from `view.TagCandidates` and `view.PinnedTags`.
- GUI: the candidates are a popup under the ✎ input, wrapping onto lines; past half the window's height they scroll inside the popup. Typing narrows them to the tags containing the text, ignoring case, as the filter bar's `tags:` suggestions do (§3.7). When the text is no tag yet, a "new tag …" line says Enter makes it; when it names an existing tag in another case, Enter applies that tag with its own spelling. The TUI row does not filter.
- The presets are the defaults for `tags.pinned`, rather than values inserted into the database on first run. config get shows them, and config set replaces the entire value: framework, visuals, apparel, races, weapons, buildings, furniture, QoL, balance, story, performance, language packs, testing, to delete.
- Without a selection, the candidate row replaces the status line.

### 3.9 Tables

**Every data-table header** follows Windows Explorer's Details view and the Mod library table rules (§2.9), on every page. A data table has one object per row and at least two sortable columns. Fixed item → value tables such as settings, and summaries with only one or two rows, are excluded.
- Clicking a header sorts by that column; clicking again reverses the order. An arrow shows the direction. Each column has a natural direction: names A–Z, sizes largest first, dates newest first, and statuses most urgent first.
- Drag a header to reorder columns, or its right edge to resize it. Double-click the edge to reset the width. The name column takes the surplus width.
- The header context menu can sort by the column, widen or narrow it, show or hide each column, and reset the columns. The name column is always visible.
- For keyboard access (§3.1 rule 6), s opens the same menu at the sorted column, and < > = resize or reset that column.
- Column order, widths, and visibility are stored per table in `ui.gui.<table>.columns`.
- These rules are implemented for the Mod library (`library`), task items (`tasks.items`, §2.16), pack list (packs, §2.7), Mods within a pack (`pack.mods`, §2.7), texture list (textures, §2.13), and Mod settings (`modsettings`, §2.9).
- New tables must follow this section.

### 3.10 Choice controls (GUI)

Choice controls share `choiceControl` in `js/choices.js`. Fields whose only values are off / on use two segments, with off on the left and on on the right, using the field's localized labels. Other single-value choices use rounded dropdowns. Preserve the existing values, disabled state, and change callback for saving; programmatic assignment must not trigger a save. Checkboxes still select items, attachments, and how to handle a confirmation.

- Tab focuses the control. Space or Enter toggles a switch; left sets it off, and right sets it on.
- Space, Enter, or down opens a dropdown. Arrows move among candidates, Home / End go to the ends, and typing jumps by prefix, skipping disabled items. Enter or Space confirms. Moving among candidates never saves; only confirmation saves.
- Esc closes the menu and restores focus to the button. Tab closes it and moves to the next control in page order. Outside clicks, lost focus, page scrolling, window resizing, or removal or disabling of the control close the menu. Scrolling inside the menu keeps it open. Back closes the menu first.
- Only one menu can be open. It floats outside the scrolling container, opens upward when there is insufficient space below, and shifts inward if it would overflow the right edge. Long lists scroll inside the menu.
- Controls and options support reference numbers. A switch exposes switch and aria-checked; a dropdown exposes combobox, its expanded state, and its current candidate, with listbox / option selection and disabled states. Settings descriptions are associated with their controls.
