# 06 · Interface · Resource targets and acceptance (§10–11)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 10. Resource targets

**Pages must load fast and use little memory.** This is a key difference between RimArcho and RimCrow (400–500 MB resident, 07 §6); new features must not sacrifice it:

- **Show the page first.** Show existing data on navigation. Fill slow parts (files, analysis, network, Steam) in the background without blocking pages or keys.
- **Do not retain memory indefinitely.** Release page data on leaving (07 §8.3); cap caches or invalidate on data changes; idle memory must not grow with usage time.
- **The table sets merge thresholds.** Do not merge changes exceeding any threshold. For page loading, lists, scans, or caches, measure before / after and record numbers in the commit description and 07 §6.

| Metric | Target | Measured |
| --- | --- | --- |
| TUI: archo working set after 10 idle minutes | ≤ 80 MB | 07 §6 |
| TUI: each key response (1354 Mods, scrolling and filtering) | ≤ 50 ms | 07 §6 |
| GUI: idle Go memory (private working set) | ≤ 80 MB | 07 §6 |
| GUI: total memory (Go + WebView2, Task Manager Memory metric, 10 idle minutes) | ≤ 200 MB | 07 §6 |
| GUI: cold start to interactive list | ≤ 1.5 seconds | 07 §6 |
| GUI: page switch to interactive content (1354 Mods) | ≤ 200 ms; file-reading pages show structure and existing data first | Editor 106 ms (07 §6); other pages unmeasured |
| GUI: single interaction (call + redraw) | ≤ 50 ms | 07 §6 |
| GUI: launch background (§2.3, §2.14) | No change in Go idle memory | 2026-10-22 development debug server: generated 58 thumbnails (about 8.6 seconds), Private 77 MB before, 75 MB 6 seconds after |

- 00's proposed 80 MB applies only to the Go process; WebView2 cannot fit this range. TUI runs in the player's terminal, whose process is excluded (§9.2). Both remain far below RimCrow (07 §6).
- On minimize, GUI sets WebView2 memory target to low, restoring normal on restore (07 §8.3): after using Mod inventory, minimizing reduced total 151 → 103 MB.
- Without WebView2 (rare stripped systems), show an explanation and Microsoft download link, never download automatically; `archo.exe` TUI remains usable.

## 11. Acceptance

### TUI

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| T1 | Four-area scrolling / search with 1354 Mods, including CJK | Each key ≤ 50 ms | Passed |
| T2 | Terminal widths 160, 130, 100, 78 | Correct §2.5 degradation, all operations usable (`TestBreakpointsAndCJK`) | Passed |
| T3 | Chinese names / aliases | Correct alignment, no split characters, no U+FFFD | Passed |
| T4 | New user enables, sorts, launches without docs | Within 5 minutes, human usability test | Unverified |
| T5 | Idle 10 minutes | archo working set ≤ 80 MB | Passed |
| T7 | Long descriptions, paths, mixed Chinese / English details | Widths 24–80, detail area / full-screen, technical info expanded / collapsed: correct wrapping within width (`TestT7Wrapping`, `TestWrap`) | Passed |
| T8 | Real environment | Every can it run item understandable in one plain sentence, with executable key; no finding codes / `#id`; top count equals counted items (`TestT8NeedsReal`) | Passed |
| T9 | Stage 20 Mods, put all back | Every original position restored (`TestStagingT9`) | Passed |
| T13 | Select two Mods, `g`, no typing, just `→` `enter` | Add second candidate tag to both, repeat removes it (`TestT13TagBar`) | Passed |
| T15 | In full-screen details, `↓` to bottom, `→` | Next Mod, content starts at top (`TestT15FullScreenNext`) | Passed |
| T16 | Resize columns, enable name wrapping, restart | Width / wrapping persist; details ≥ 40 cells; `=` resets (`TestT16WidthsPersist`) | Passed |
| T17 | 1000 random navigation keys in every state, including `PgUp` / `PgDn`, shift / ctrl arrows, wheel | List, staging, tags, undo stack unchanged (`TestT17ArrowsOnlyNavigate`, 15 states) | Passed |
| T18 | Script §3 key sequences for 6 tasks below | No more keys than table, correct results (`TestT18TaskSteps`) | Passed |
| T19 | Render every state, including 5 overlays / full-screen details | Position line always gives focused area / position, including overlay names (`TestT19PositionLine`) | Passed |
| T20 | Repeat `esc` from any state until stable | At most 3 to base: current page, no selection / overlay, collapsed filter bar (`TestT20EscBack`: 15 editor states + 7 nested; `TestT20EscBackPages`: 27 states across launch, can it run, game log, Mod inventory > all, subscriptions and updates, search files, sorting rules, packs, texture optimization, settings, uninstall; `TestT20PagesBuilt` verifies each state really built) | Passed |
| T21 | Start TUI without arguments | Current plan editor; command panel reaches every TUI page in §9.1 | Passed |
| T22 | Two plans in one environment; command-panel switch | Editor shows other plan's list, staging, history | Passed |
| T24 | GUI alignment (§9.1, §9.5): every command-panel action responds; top / breadcrumbs use GUI names can it run and mod library > subscriptions and updates; file search covers open list's enabled Mods, results Mod → file → line, toggle and rerun, stop on leaving; rules `r` add, `Del` delete, immediate `u` restore, `a` scope; editor `H` picker, narrow `4` details, details `?`; `shift+1` / editor `L` visit launch, only `L` there launches; `P` opens without changing current, top says launching X; `T` stages all outside Staging | All correct (`TestWindowNames`, `TestFileSearchPage`, `TestFileSearchLeaveStops`, `TestRulesScreen`, `TestEditorKeysOfTheWindow`, `TestStartPage`, `TestOpenListIsNotCurrent`, `TestStageAllKey`) | Passed |
| T23 | `:` (§9.4): `pl`, `tab`, choose `plan new`, add name; `enable`, `tab` completes cursor Mod; `↑` recalls; invalid flags; multiline command | Correct candidates / help; usage error preserves input, red error; single line status / multiline panel; quoted name one argument; background execution without blocking; history survives restart (`TestCmdLine*`) | Passed |

T11 / T12 were removed with session picking; T6 / T14 (thumbnails) and T10 (download WezTerm) with removal of the WezTerm host / images on 2026-10-30 (§9.2).

T18 tasks (+ name means typing a few characters):

| Task | Keys |
| --- | --- |
| Enable known-name Mod at suitable position, focus Enabled | `/` name `tab` `space`: 3 keys + name |
| Move Mod behind another, 120 positions apart | `m` name `enter`: 2 keys + name |
| Disable 3 nonadjacent Mods | `↑↓` movement plus 3 `ctrl+space`, `space`: movement + 4 |
| Read current Mod description | `PgDn` `PgDn`: 2 keys |
| Add common tag to 5 consecutive Mods | `shift+↓` 4 times, `g` `→` `enter`: 7 keys |
| Stage half the Mods to investigate findings | `shift+↓` N times, `t` |

### GUI

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| G1 | Same synthetic environment state | TUI / GUI columns / needs exactly match `view.BuildBoard` / `view.BuildNeeds`; GUI Go mutations use typed view / app calls, `cmds.Execute` only §1 read-only text / user commands (`gui.TestSameViewModels`) | Passed |
| G2 | Scroll / filter 1354 Mods | Each interaction ≤ 50 ms | Passed |
| G3 | Each keyboard, mouse, drag, context-menu action | Same result as TUI, every step `u` undoable (`gui.TestVerbsAndUndo`) | Passed |
| G4 | 1000 random clicks, scrolls, arrows | Database / three columns unchanged (§3.1 rule 1) | Passed |
| G5 | Repeat `esc` anywhere | At most 3 to base; subscriptions / updates collapse sources then clear single-Mod condition; add subscription clears paste then returns | Existing pages: `spikes/guidev/esccheck.js`; new panels / selection / record conditions: `/dev/pagecheck` |
| G6 | Workshop description with scripts, `onerror`, `javascript:`, remote images | No execution / loading; literal text, images from local cache | Passed |
| G7 | Real environment can it run | Same list as TUI, executable button per item | Passed |
| G8 | TUI / GUI open together, edit in one | Other refreshes on focus without overwriting (`app` concurrency tests) | Passed |
| G9 | Idle 10 minutes | §10 targets met | Passed |
| G10 | Double-click `RimArcho.exe` | Direct GUI, no console, scan / analysis subprocesses no windows | Passed |
| G11 | Every sidebar entry | Only current children expand; breadcrumbs match location, every level clickable | Passed |
| G12 | Home, two environments / three plans / one pack | Last-played order; row status equals plan can it run error count; launch disabled with explanation while running | Partial: ordering, each plan row, runtime disable passed; two environments untested |
| G13 | can it run | Left errors / hints / confirmation, right last-run errors, TUI same order one column; hidden / locked Mods and stale textures bottom links only; badge counts actionable items, excludes hints; evidence chips | Passed: GUI `TestNeedsSectionsAndEvidence`; TUI `TestG13NeedsSectionsTUI`, `TestNeedsSectionsLive`, `TestNeedsMergedAndElsewhere`, both icon modes; view `TestCorpusNeeds`, `TestHiddenModsNotCounted`, `TestNeedsElsewhereAndLinks` |
| G29 | Buttons / finished items (2026-10-29) | All actions bottom buttons, no `⋯`; one button per choice; inline confirmation; vanished completed item retained as ✓ …, newest undoable has undo; last-run / related left items cross-link | Passed: GUI `scripts/vm/needs.test.mjs`, `identity-answers.test.mjs`; view `TestNeedsRelated`, `TestNeedsSortAlso`; native window / real data pending VM |
| G14 | Create pack with error findings | Build disabled with specific findings; see the list first writes nothing | Passed (`gui.TestBuildPageG14`; browser checked on data copy) |
| G15 | Guide, two environments, RimSort import | Both checked environments created; import preview then first plan on confirm | Implemented (`guide.js`, §2.15), unverified |
| G16 | Open Archo anywhere | Right panel, `esc` closes, page state unchanged | Pending (1.1) |
| G17 | Runtime status, sidebar row removed 2026-10-30 | Launch page running within 3 seconds; new errors update badge without dialog; new log counted automatically on exit (`TestReadNewLogCountsTheRun`) | Passed; pre-removal record: 2026-10-22 development server `-fakegame`, 1.6 seconds after start showed game running since 03:51 and badge 1, absent from top bar; row vanished on exit. TUI status right: `TestLiveGame`, `TestStatusLineAfterGame` |
| G18 | All lists, four lists, one followed collection changed | Clicking lists opens all lists; ungrouped left; current / make current at row right; version changes expose reset to default / undo; catch up undo returns original in one step; no ModsConfig.xml writes here | Implemented |
| G19 | Export from all lists and editor | Breadcrumbs library / lists / <list> / export; every format exactly matches `archo loadout export --format`; `esc` returns to origin | Implemented |
| G21 | Add 2000+ Mod game, switch to it, rapidly change texture options on build / texture pages | Each click responds within 100ms; sidebar / other pages usable during scan / switch / measurement; only conflicting actions disabled; latest value wins (§3.1 rule 9) | Partial: synthetic 2100 Mods allow navigation during scan / first analysis, final rapid-switch state correct; stale measurements cannot overwrite build / texture options, `/dev/pagecheck`; real 2000+ game / native window unverified |
| G22 | Every sidebar entry | New page skeleton / old content first, fill on arrival; late results cannot overwrite new page (§8.2) | Passed headless Chrome `/dev/pagecheck`: immediate feedback, late success / error isolation, failure retries, same-page refresh, environment / plan checks, old virtual rows inert during editor / library object changes |
| G23 | New list, import, load history, see list first | No dialogs: right-column new list, import subpage, inline list; history loads without confirmation, `u` undoes (§3.1 rule 7) | Passed in `spikes/guidev`: right form, import subpage (share code / test Steam collection), history / undo; inline list (`codex/ui-tables`) |
| G24 | Every pair of space-sharing cards | Drag splitter, double-click reset, tab then arrows resize, persists (§3.1 rule 10) | Passed `gui.TestSeparators`; headless `spikes/guidev/gapcheck.cjs` measured 14 splitters: editor, expanded staging / history, three filter bars, library, log, tasks, lists, sidebar; drag, hit area, 40px keys, double-click / Enter reset, restart persistence, narrow disable. Synthetic log / empty tasks, real GUI settings API |
| G25 | `TestStyleRules`, `TestWording` | Two radii, four font sizes ≥ 12px, hairlines only, no left accent bars, no `·` in UI (§7.3) | Passed: style tests check radii / font shorthand / variables / hairlines / accent bars, report line / value; wording checks Chinese-English tables, command panel, Go UI literals, GUI JS; CLI retains format |
| G26 | A→B→C, back twice, forward once, new page; side buttons / `alt+←/→`; overlays, details, deleted pages, uninstall lock | §3.1 rule 11: no duplicates, truncate branches, skip invalid pages, late results never reclaim page, address stays Wails / dev server, navigation never mutates data | Partial (2026-10-24 headless Chrome `/dev/pagecheck`): history, side-event interception / deduplication, keys, invalid pages, overlays, details, guide end, uninstall lock, late isolation; native WebView2 physical side buttons unverified |
| G27 | Subscriptions / updates unified waiting / updated list with details, scope, source summary; add subpage; home / detail changes links | Read-only source / account status in inline sources; queued checks / updates, preview / runtime refusal; scope filters both groups, counts hidden updates; paste recognizes Git / LoversLab / other page / Workshop, one decision and `--dry-run`; home / detail scope preserved; no all-page subscription entry | Automated view fixture / golden, GUI API, `spikes/guidev/pagecheck`; real Steam / SteamCMD / LoversLab / native window unverified |

| G28 | Shared editor / library filters, multiple Mod versions | Query text sole condition; only common tags / source / content groups; popup / ghost text / Tab completion without counts; consistent copy abstractions; 38:31:31 columns, swap / undo, Space / staging | Automated `TestEditorFilterBehavior`, `TestTypedBarConditions`, `TestDirectorySourceIdentityAndCopyFilters`, `TestCopyCaptionsAcrossPages`; production GUI dev server + isolated game: clicks, completion, blur collapse, swap / undo, Space / staging; native WebView2 unverified |
| G30 | Editor rules, current / all, `r` add, edit, delete, undo | Above history; default enabled-list Mods only; all scope includes all, counts update; inline `r` form excludes self; edit retains `r:<n>`; undo restores deleted number; form keys cannot affect list; order findings / badge refresh | Automated `TestRulesFold` incl. `InList`, `TestRulesIgnoreRow`, `TestRuleCandidates`, `TestRuleEditCommand`; guidev synthetic clicks / keys cover scope, add, edit, delete, undo; native WebView2 unverified |
| G31 | Mod library > search files | Enter searches, new invalidates old, esc / leave stops; Mod → file → line pagination; click path copies; C# only named Mods | Automated `TestFileSearchAPI`, guidev synthetic fixtures; C# needs ilspycmd, not tested on fixtures |

### History area and settings

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| P1 | Change language in settings | Immediate, persists restart | Unverified |
| P2 | Clear thumbnails | Zero size, rebuilt on demand (`TestSettingsCommands`) | Passed |
| P3 | `db backup` | Openable copy, original unaffected (`TestSettingsCommands`) | Passed |
| P4 | Settings submits disallowed command | Refused, not executed; confirmation where required | Unverified |

### Icons

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| G18 | Both release executables | Portrait in Explorer, version / description in properties | Unverified |
| G19 | Window | Taskbar / Alt+Tab icons; `go build` without `.syso` also has icon; Windows interaction §8.2 | Unverified |
| G20 | 16 px icon | Recognizable head only, hair and face | Unverified |
