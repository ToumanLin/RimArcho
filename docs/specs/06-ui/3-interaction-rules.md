# 06 · Interface · Interaction rules (§3)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 3. Interaction rules

These rules fully apply to the plan editor and Mod inventory > all (same interface, filter bar, key table, context menu; decided 2026-10-22). Other pages follow the same principles: arrows navigate only, actions use buttons / verb keys, `esc` returns one level. GUI / TUI share keys except §8.5 / §9.1.

### 3.1 Rules

1. **Arrows only navigate, never mutate.** Except GUI off / on segmented controls (§3.10), `↑↓←→` move cursor, focus, scroll only, never Mods, enablement, tags. GUI clicks / wheel likewise never mutate.
2. **Actions are verb keys, immediate, `u` undoes.** Object means selected instances if any, else cursor row. No mode, drop / cancel step. Each verb is one app edit, one undo step.
3. **Position always visible.** Position line gives focused area, cursor index, selection count, open overlay (§3.2).
4. **Destinations always visible.** Hints list currently available actions, at most 7; `?` all, `ctrl+k` goes anywhere from anywhere.
5. **Minimal overlays.** Only command panel, tag candidates, position input, confirmation, help, version list (§5). Every overlay titled and in position line, `esc` closes, no nesting. GUI `:` / `ctrl+k` share panel (§3.5); TUI `:` command line (§9.4). `r` rule input listed only in `?`; GUI `r` is inline editor sorting-rules form, not overlay (§2.5).
6. **Everything mouse can do, keyboard can do** (2026-10-22, for TUI). Every click, right-click, drag, double-click has a key or keyboard focus: row actions have keys or context-menu entries; menu key / `shift+F10` opens same menu at cursor, `↑↓` selects, `enter` executes, `esc` closes, displayed keys execute directly. Likewise filters, headers (`s`), history (`H`), widths (`<` `>` `=`), detail links (`C`, `ctrl+o`, `ctrl+w`, `g`, `n`). New features get keys before mouse. `TestOpsReachable` (view) / `TestOpsRunnable` (GUI) verify key / menu reachability and execution on both pages.
7. **Dialogs only for warnings / prompts** (2026-10-23, refined same day). Dialogs demand handling before other work, opposing knowing where to go. **Unsolicited dialogs** only for immediate decisions: irreversible confirmation (§6), commands solving a refusal. **Player-opened temporary layers dismissed by `esc` are not dialogs**: command / context / `⋯` menus, tag candidates, position input, help, identity why-this-question (§5), version list (§5). Player-opened finding explanation / actions (§5) may also be temporary layers. Forms (new list, import), other views (lists, diffs), progress, records are pages, subpages, inline sections, never dialogs (background tasks §2.16; import subpage §2.17).
8. **Collapse unused parts** (2026-10-23). Unneeded content becomes one line. Expanded filter bar **collapses immediately on focus leaving**, by outside click, tab, esc; entering its own dropdown / input is not leaving. Effective conditions remain in collapsed line. Long lists behind N more; completed / secondary info muted. Collapse never loses entered text.
9. **Always respond; background work never blocks foreground** (2026-10-23).
   - **Foreground**: page entry / reading; **enter before read**: navigate immediately to skeleton / previous content, fill on arrival; late results never overwrite newer page (§8.2).
   - Every click / key has visible response within **100ms**: pressed state, skeleton, status `…`, progress all count.
   - **Background**: anything likely over about 100ms or touching disk / network: moving / deleting files, unsubscribe, download, import game / first scan, initialize Mods (scan / analysis), size calculation, game-switch reads. Never on UI thread; other pages remain usable; only conflicting actions disable with reason, e.g. file queue waits during scan (§2.16).
   - New request for same object invalidates old; rapid dropdown changes keep latest only, cancel old computation or discard result.
   - Player-initiated work taking time with a result becomes background task (§2.16); incidental sizes / page reads only show local progress.
10. **Splitters between space-sharing cards are draggable** (2026-10-23, §7). Adjacent cards sharing a row / column resize via gap; double-click resets, page-specific persistence. Focusable `role="separator"`, tab reachable, arrows 40px per press, enter resets; filter `ctrl+↑↓` unchanged (§2.5). Content-height vertically stacked cards, e.g. needs sections / settings groups, have no draggable gaps.
11. **Pages support back / forward** (2026-10-24, GUI; TUI exception §8.5). XButton1 back, XButton2 forward; `alt+←` / `alt+→`, also `BrowserBack` / `BrowserForward`.
   - Record completed page entry, whether sidebar, panel, breadcrumbs, links, subpage. Back during reading retains explicitly opened target for forward revisit. Reopening current, refresh, late results add none. New page after back discards forward branch. Last 50 this session, never disk.
   - Back closes top overlay / menu and stops; technical-page stack follows backspace one-level rule (GUI technical info within full-screen details returns to source list); otherwise previous history page. Forward next; absent destination stays.
   - Skip deleted plans / packs, completed guides, other invalid pages; no automatic game switch for history. Respect uninstall navigation lock during / after (§2.14).
   - Revisit via `go()`, rule 9 enter-first / stale-read invalidation. Forms use normal leave / reentry, no unsaved confirmation. All-lists new drafts persist per game this session; untouched-name drafts discarded on leaving form (§2.17); management details / new view separate history entries. Navigation never edits, undoes, imports, builds.
   - GUI intercepts side-button down / up / auxiliary defaults, one step per press; no browser history, never leaves Wails address.
12. **Address display, copy, edit** (GUI). Files / folders / URLs share `fields.js` `copyablePath` / `pathContent`: selectable text, trailing copy icon with copy tooltip, brief ✓ on success, always full value. Single-line middle ellipsis by actual width, preserving initial directory / final directory or filename; recalculate on resize, full hover. Keep readable minimum width; adjacent buttons wrap first. Copy tab-focusable, Enter / Space. Read-only text click does nothing; separate open-folder / web actions. Editable local text or change… / add… directly opens system folder / file picker, tooltip explains; selection immediate, cancel unchanged, existing preview / confirmation retained, no manual-path box. URLs normally read-only; change… opens small one-input dialog, validates complete HTTP(S), cancel discards / OK saves, Enter OK / Esc cancel. Relative filenames may show relative paths; hover / copy use full provided path. TUI retains text entry / full addresses without native picker; CLI / MCP unchanged.
13. **Text is selectable** (2026-10-25, GUI). Player-visible text selectable / ctrl+c by default: Mod names, paths, descriptions, detail fields, raw logs, finding explanations, hover / pinned hints, dialog bodies, status. Controls excluded: buttons, sidebar / breadcrumbs, area headings / splitters, tag chips, multiselect rows (editor's three lists, inventory table, tasks); presses select rows, shift / ctrl extend, drags move Mods, so select names in details instead. `style.css` body `user-select: text`, controls `none`; new controls follow rule. Address selected text ctrl+c copies selection, trailing button full address (rule 12). TUI unchanged, terminal handles selection.

**Base state**: current page, no selection / layer / menu, collapsed filters; editor focused list also has no search. Remaining effective filters are page state, still base; another esc clears. At most 3 esc from anywhere (T20, G5):
- Close layers / menus first, including mouse-opened unfocused context / `⋯` menus; then details (covering / focused) → selection → filters (from rows, clear / collapse together; inside filter, collapse only).
- Secondary screens (full details, technical, expanded needs item, TUI pack / settings text) return to opener; technical esc closes whole stack, backspace one level, so any depth meets 3.
- Esc never navigates peer sidebar pages; use sidebar / crumbs / ctrl+k, side buttons / alt arrows for history (rule 11).

### 3.2 Position

Four editor focus areas: `1` Disabled, `2` Enabled, `3` Staging / history, `4` details. Numbers jump directly and prefix headings in muted text.

- Focused heading accent, cursor row faint full background (TUI `#1A1C24`). Unfocused cursor only name text-colored, no background / `▸`. Detail focus makes all lists unfocused: one brightest place only.
- Details heading `4 details`, becomes `▲ name` when scrolled, accent when focused; next line aligns with first list row.
- Position line: TUI top right, GUI editor / full details status left:
  - List: `Enabled  322 / 488  selected 3`, visible / filtered row counts; add filtered if searching.
  - Details: `details  Combat Extended  description 40%`; current top-line section heading (summary, description, history, technical) plus scroll progress.
  - Open layer appends, e.g. `Enabled  322 / 488 › tags`; search `› search`, second-press confirmation `› confirm`.
  - Full details: `Enabled › Combat Extended (322 / 488) › Harmony`.
  - Other pages (full-screen, technical, needs, changes): breadcrumbs are position.

### 3.3 Arrows

| State | `↑↓` | `←→` |
| --- | --- | --- |
| List | Cursor | Focus area |
| Details | Scroll | Focus area |
| Full-screen details | Scroll | Previous / next Mod, still navigation |
| Overlay: panel, tags, position | Options | Options |

- `PgUp` / `PgDn` page cursor, `Home` / `End` ends (2026-10-22, both pages); focused details scroll instead. Wheel anywhere scrolls details, not list cursor; lists use keys / click.
- `←` from details returns to displayed Mod's list, not area 3: at width 130 Staging absent, area 3 would show another Mod when coming from Enabled. Staging (area 3) `→` still enters area 4. No wrap. Area switching only arrows / `1`–`4`; tab switches sidebar entry's subpages (§3.5).
- TUI `j` `k` = down / up, `h` `l` = left / right. Home / End list ends; `g` now tags, `G` unused.
- Description initially 12 lines; any scrolling / focus shows full. PgDn before loading retained until ready.

### 3.4 Selection

- `shift+↑↓` extends contiguous selection from cursor.
- `ctrl+space` (TUI also `v`) toggles row; `ctrl+a` all displayed, filtered when applicable. Normal / ctrl arrows **do not** clear, enabling scattered selection by move → toggle.
- Click moves cursor, shift-click range, ctrl-click toggle. Same-list click preserves selection like arrows; different list clears.
- Esc clears, as does changing list area; hint warns `←→ changing lists clears selection`.
- Selected row faint full accent, deeper for cursor; same GUI / TUI; **no left vertical row / card bar** (2026-10-23, §7 principle 4). Hints become `selected 3  space disable  t stage  m move  g tags  esc clear selection`; just hints, not popup / mode.
- After action: `g` keeps selection for repeat; space / t clear as rows leave; J / K / Enabled-internal m keep.

### 3.5 Verb keys

Editor / inventory > all share **one key table** (player confirmed 2026-10-22), GUI / TUI except §8.5 / §9.1. `?` shows current-page `view.KeyHelp`.

| Key | Action on selected objects, otherwise cursor row |
| --- | --- |
| `↑↓` / `j` `k` | Cursor / details scroll |
| `shift+↑↓`, `ctrl+space`, `ctrl+a` | Range, toggle one, select displayed |
| `PgUp` `PgDn` `Home` `End` | Cursor page / ends, or details scroll |
| `enter` | Full details (§4.2), same editor / inventory page |
| `esc` | One level: details → selection → search / filters |
| `/`, `ctrl+f` | Filter search; Staging own search |
| `\` | Toggle filter bar (§2.5) |
| `space` | Effective / ineffective in current plan: editor enable / disable, Staging put back. Enable at suggested constraint position; status ✓ placed at 352, m to move. Cursor stays Disabled for consecutive enables; immediate m moves those just placed, any other key forgets |
| `m` | Editor only: enable / move to… position input (§3.6) |
| `t` | Editor stage / put back, Staging retained |
| `x` | Editor switch same Mod's copy (`swap`) |
| `g` | Tags (§3.8), TUI top now Home |
| `n` | Editor Lineage Mod note (`archo note`); inventory copy note |
| `ctrl+o`, `ctrl+w` | Folder / Workshop, replacing o / d. Without ID status no Workshop ID: not a Workshop mod. GUI `preventDefault`, Wails does not intercept. Combined hint ctrl+o ctrl+w folder / workshop |
| `u`, `ctrl+z` / `ctrl+y`, `ctrl+r` | Editor undo / redo |
| `Del` | Unsubscribe Workshop / delete own copy, dry-run then confirmation |
| Menu key, `shift+F10` | Cursor context menu (§8.5) |
| `shift+1`–`shift+8` | 8 entries: launch, needs, log, lists (current), packs, inventory, textures, environments. GUI physical keys work any layout; TUI `!@#$%^&*` US layout. ! / # no separate shortcuts |
| `tab`, `shift+tab` | Next / previous sibling subpage: inventory all ↔ subscriptions / updates ↔ add ↔ search (TUI same, search since 2026-10-31); plans open only, current unchanged (04 §1.3); packs, first from pack list. Search Tab first accepts ghost completion; forms (subscriptions, settings, build etc.) normal focus; full details tabs switch pages |

Editor only:

| Key | Action |
| --- | --- |
| `←→`, `1`–`4` | Areas, no tab area switching |
| `J` / `K`, `alt+J` / `alt+K` | Enabled down / up 1 or 10, also `alt+j` / `alt+k`; terminal cannot distinguish shift+J / J, hence alt for 10 positions. Filtered moves cross displayed rows (§2.5) |
| `T` | Staging put all back at original positions, elsewhere stage all Enabled; TUI same since 2026-10-31 |
| `S` / `w` / `L` | Whole plan sort / save / launch; sort undoable, previews in panel. L visits launch focused on launch, another L executes; TUI same since 2026-10-31 (§2.5, §9.1) |
| `i` / `e` | GUI import / export; TUI i toggles technical details |
| `R` / `A` | Rescan / recheck; TUI editor A since 2026-10-31, formerly needs only |
| `c` / `C` | Subscriptions / updates all / cursor Lineage only (§2.9) |
| `H` | History steps / versions picker; TUI also since 2026-10-31, plus area 3 history |
| `<` `>` `=` | Focused column narrower / wider / reset |
| `r` | Add cursor Mod rule: sorting-rules form (§2.5), GUI left column / TUI secondary. Full details r refetches Workshop description |
| `P`, `E` | Plans switch / new / rename / delete; environment switch (GUI environment page) |

Global:

| Key | Action |
| --- | --- |
| `c` | Subscriptions / updates all |
| `l` | Game log, TUI needs log picker |
| `ctrl+k` | Command panel |
| `alt+←` / `alt+→` | GUI back / forward (§3.1 rule 11) |
| `ctrl+b` | GUI side / top navigation (§2.1) |
| `ctrl+\` | GUI reference-number mode: thin red box on every control, paragraph, image, chart, red / white upper-left page abbreviation + number, e.g. lib12, so feedback can say change lib12. Only rendered visible / uncovered objects count in page order; contiguous, renumber on scroll. Overlay does not block clicks / alter layout |
| `1`–`9` | Page areas since 2026-10-22, numbered headings like editor 1–4. Launch 1 game / 2 list-pack / 3 launch (§2.3); inventory 1 filters / 2 table / 3 details; subscriptions 1 waiting-updated list / 2 details; needs displayed sections, cursor first item; log 1 errors / 2 raw, arrows scroll focused raw; environments 1 all, then settings groups; settings groups; build sections. GUI selected-area border accent; TUI needs / inventory same, 1 opens filters, 3 copy details fills screen if narrow |
| `:` | GUI unified panel like ctrl+k, any page; TUI command line (§9.4) |
| `?` | All current-page keys |
| `q` | TUI quit |

**Command panel**, GUI ctrl+k / colon, TUI ctrl+k:
- GUI at page bottom above status, ordinary overlay width / appearance; results / CLI output expand upward, long output scroll / select all / copy. Close retains draft / latest output; opening other layer closes first, no stacking.
- Contents: Chinese action names / English aliases (`view.Commands`), all Mods (jump), other environments (switch), lists (open). Input also runs archo commands without separate box; mouse / ctrl+n/p select results / run item; ctrl+enter runs input directly.
- Score: prefix or suffix 4 > substring 3 > subsequence 2 > description 1. Suffix equal prefix since 2026-10-31: Chinese headwords at end, sort matches automatic sorting / sorting rules equally, automatic sorting first by command table. Descriptions include keys, enabling key search. Ties: commands, Mods, environments, shorter names. Empty only commands / other environments.
- GUI typing filters, up/down execution history, Tab command / argument / object completion, repeated Tab cycles, Enter accepts without running; ctrl+n/p results, Enter execute, Esc / ctrl+k close / restore focus, ctrl+u clear. Colon in input literal. TUI panel arrows / Tab selects results; TUI command line §9.4.
- Shared `internal/cmdline` quotes, completion, last-50 deduplicated history algorithms; histories separate, preserving old TUI. Per-command `--env`, Windows backslash, and `#` retain CLI/TUI semantics; no shell.
- GUI `cmds.Check`, background `ExecuteContext` in independent session, one command, no page lock. Only commands exceeding one second get stoppable background-task row (§2.16); quick results only panel. Scan / analysis subprocesses; refuse log --follow. Retain raw CLI / exit code, destructive commands still need explicit CLI --yes.
- Mod selection jumps / selects its column row; clear that column's search if hiding it.

### 3.6 Position input (`m`)

```
 move 3 to ❯ harm▌
   after  Harmony                       (position 1)
   after  Harmony Patches for VFE        (position 212)
   position 352 (suggested)
   first   last
```

- Number means position N; Mod name fuzzy matches up to 5, prefix / substring / alphabetic. Default after; Tab switches before retaining selected row.
- Empty Enter uses suggested `App.SuggestAt`, sorter edges: after last enabled predecessor; only before constraints means before first target; unconstrained append. Official-first edge alone not a constraint; constrained suggestion never before official content, e.g. Harmony-dependent Mod not between Harmony / Core. Remove already enabled objects before calculation, so m also suggests moves.
- Options one row each except first / last share; both arrow pairs navigate. Positions count list after removing moved Mods (`MoveBlock`, `Enable`); number by candidate name is its current position.
- Disabled / Staging m enables at selection; multiple staged objects enabled / unstaged in one undo.
- Input occupies list bottom rows. `view.PosOptions`, `view.Place`.

### 3.7 Search (`/`)

- Shared filter bar (§2.5) filters Disabled / Enabled instantly; Enter stops typing, filter until Esc clears. Heading `Enabled 12 / 485`. Staging separate search, same rules.
- Old Tab-next-matching-list unnecessary with two-list filtering; Tab only ghost completion.

Space-separated query; free words AND; structured same-key OR, cross-key AND; exclusions individually removed:

| Syntax | Meaning |
| --- | --- |
| `word` | Case-insensitive name / alias / author substring; if the complete query has no matches, free words fall back to ordered name-letter subsequence: hsl finds Hair Style Lib |
| `name:word` / `author:word` / `tag:word` | Name incl. alias / author / tag only; tag matches any word prefix: tag:facial finds Facial Hair / Facial Animation, tag:animation latter, tag:"facial hair" former only |
| `id:value` | Package substring, exact Workshop ID, or `#reference` |
| `tags:value` | Exact tag selection, also click-filter syntax; old tag: prefix retained |
| `source:value` | Directory role official / workshop / local / steamcmd / locked / other, or stable custom-directory alias (02 §0.2) |
| `content:value` | assembly, xml, language |
| `type:value` legacy | Sources official / workshop / local / copy / git / steamcmd; assembly DLL, xml no DLL also xml-only, language language-only also language-pack |
| `has:problem` / `has:error` | Actionable needs / load-failure finding; also has:tag / has:note / has:alias |
| `version:1.6` | Declared version support |
| `-word`, `-tag:x` | Negation |
| `"with spaces"` | One quoted word |

GUI click search / slash shows small popup matching cursor word against available keys / values, tags, sources, content, versions, authors; raw location: excluded. First ghost suffix changes with up/down selection; Tab accepts without changing arrow editing semantics in input. Quote spaced values automatically; complete mid-query. No counts or repeated tag text. Fixed help translated; author / tag / custom directory data unchanged. Enter applies / leaves input; Esc closes popup then input. Filter clicks add / remove conditions in same text, no chips. Legacy tag:, type:, location: compatible; TUI existing completion retained.

GUI editor arrows follow physical order: details → Enabled → Disabled; numbers `1`–`4` retain logical IDs, Staging right. Space / undo unchanged.

Content types (`type:assembly`, etc.) come from current `snapshot.kinds`, precomputed from file list when writing snapshot (02 §7), primary-key reads. Lazy load, retain until database change (`view.ContentCache`).

### 3.8 Tags

- Mod tags in detail fields (§4.1) with edit; never row tags competing with long names across thousands of rows. Find via tag: (§3.7) / inventory tags (§2.9).
- Lineage-owned (04 §8), CLI `tag add|remove <ref> <tag…> --mod <ref>…`, MCP `set_player_data`.
- Player data, not list edits: no undo; GUI tag status has no undo link.

`g` opens candidate row under hints / details, not input:

```
 ✎ type…   [framework] [visuals] [apparel] [races] [weapons] [buildings] [QoL] [testing] …
           ▲ first candidate initially selected
```

- Both arrow pairs move, Enter applies / closes retaining selection for repeat g, Esc closes.
- Before first candidate is ✎ type…; left reaches it, typing / Enter creates and applies. Typing on any candidate jumps there.
- Tag on **all** objects `[✓framework]`, Enter removes; partial `[~framework]`, Enter fills all.
- Pinned first (`archo config set tags.pinned "framework,visuals,…"`, accepts ASCII / full-width commas), then existing by usage. Horizontal scroll. `view.TagCandidates`, `view.PinnedTags`.
- Defaults are `tags.pinned` defaults, not inserted on first run; config get shows, config set replaces whole: framework, visuals, apparel, races, weapons, buildings, furniture, QoL, balance, story, performance, language packs, testing, to delete.
- Without selection candidate row replaces status.

### 3.9 Tables

Decided 2026-10-23: **every data-table header** follows Windows Explorer Details, same as inventory (§2.9), not just that page. Data tables: one object per row, more than two sortable columns; settings item → value fixed tables / one-two-row summaries excluded.
- Click sorts, again reverses, directional arrow; natural direction per column: name A–Z, size descending, date newest, status urgent first.
- Drag headers reorder, right edges resize, double-click reset; name takes surplus width.
- Header context menu: sort column, widen, narrow, show / hide each (name always), reset columns.
- Keyboard (§3.1 rule 6): s opens same menu at sorted column; < > = resize / reset it.
- Column order / widths / visibility in `ui.gui.<table>.columns`.
- Implemented: library, `tasks.items` (§2.16), packs (§2.7), `pack.mods` (§2.7), textures (§2.13).
- Future tables follow this.

### 3.10 Choice controls (GUI)

Shared `js/choices.js` `choiceControl`: off / on only uses two segments, off left / on right, localized field labels; other single choices rounded dropdown. Preserve values / disabled / change saving, programmatic assignment never saves. Checkboxes still select items, attachments, confirmation handling.

- Tab focus; Space / Enter toggles, left off / right on.
- Dropdown Space / Enter / down opens; arrows navigate, Home / End ends, typing prefix jumps, skip disabled. Enter / Space confirms; moving never saves, confirmation only.
- Esc closes / restores button focus; Tab closes / next in page order. Outside click, blur, page scroll, resize, removal / disable closes; internal menu scroll stays. Back closes menu first.
- One menu; floats outside scrolling container, opens upward if needed, shifts inward at right; long list internally scrolls.
- Reference numbers supported on controls / options. Switch exposes switch / aria-checked, dropdown combobox / expanded / current candidate, listbox / option selected / disabled states; settings help associated to controls.
