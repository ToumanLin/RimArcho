# Can it run page drafts

Three candidates for "playing / can it run" (06 §2.4, §5), plus a "now" page drawn from current specifications for comparison. Colors, sidebar and items follow current GUI (`style.css` variables). Example data only (`ITEMS` in `shared.js`); buttons only show hints, never read/write files.

## Current problems

- The page became a catch-all: hidden Mods (locked-version updates, unsubscribed residue) and other games' broken links do not affect whether this list can run.
- Breaks-the-game mixes three uncounted kinds (corpus hints, weak order requirements, cycles), creating many exceptions to counting (§5).
- Item content is scattered: row sentence/explanation, cursor-only evidence, `⋯` menu, `f` / `enter` expansion, finding popover, identity-reason comparison and version-list panel.

## Changes shared by all three

- **Move hidden Mods out** to mod library › all (hidden filter), leaving a bottom link. Sidebar already excludes them from counts.
- **Link panel shows current game only.** Other games' broken links go to games page; retain just "game '1.5 test' has 1 link to check". Launch checks still block only current game.
- **Separate uncounted items into worth knowing**, collapsed by default. Counting becomes: all items except worth knowing.

## Candidates

| | A: Two columns | B: List + details | C: By action |
| --- | --- | --- | --- |
| Structure | Left before you start (breaks the game, for you to confirm, worth knowing), right last time you played (log errors), independent scrolling | One left row per item grouped by section; selected item's full content/actions right | One column: direct actions (numbered steps) → choices (inline answers) → last-run errors → worth knowing |
| Item appearance | Same as now, narrower | Left only sentence/count; explanation, evidence, expanded lists, comparison, version list and log snippets right | Each step states what it solves: automatic sorting fixes official order and 7 ordering issues, also weak order/cycles |
| Removed | Nothing, only rearranged | `⋯`, `f` expansion, finding popover, identity comparison, version panel all absorbed into details | No choose-which-to-disable popover; options directly in steps |
| Count | Same as now | Same as now | Remaining steps + log error groups (16 → 13 in example); one fewer per finished step |
| Benefit | While game runs, right updates live, left stays still; separates predicted/actual | All items on one screen; same list-left/details-right shape as game logs/subscriptions; delete several popover implementations | Easiest for players: 9 things left; matches §5 count-as-work principle |
| Cost | Left still long; complexity only split, not reduced; TUI 80 columns cannot fit both, differing GUI/TUI layout | Explanation requires click instead of inline; titles must fit one line, some too long | Largest change: step layer above `view.BuildNeeds`, merges finding codes by action; version summaries without one action need separate treatment; editor finding panel/row popovers keep original items |

## A second draft (current direction, 2026-10-29)

A was chosen; `a.html` polished as below (now/B/C unchanged):

- **Left order: breaks the game → worth knowing → for you to confirm.** Worth knowing is learned from shared logs, a core feature: expanded by default, accent heading/icon instead of muted, proportion bars in evidence. Still uncounted. Weak order/cycles merge into an automatic-sorting item's explanation (sorting already handles them), leaving only shared-log hints here.
- **No `⋯` menu.** All actions are bottom buttons aligned with title's left edge; evidence on same row at right, wrapping below when narrow. Primary first. Keys (`enter`, `S`, `space`, `1` `2`…) only on cursor item.
- **Choices are buttons.** Choose-which-to-disable becomes equal "disable Combat Extended" / "disable Yayo's Combat 3" buttons. Same for duplicate copies, identity questions and pairs often erroring together in shared logs. Standardize `1` `2` `3`: specification identity `u` (unrelated) collided with undo.
- **Confirm in place.** Repair links, ignore version reminders, load outside changes, answer identity: replace button row with consequence and confirm/cancel (`enter`/`esc`), no dialog. Identity confirmation states u cannot undo.
- **Finished items stay put.** Become a green "sorted automatically  Core and DLC must come first" row plus undo; others do not jump. One action fixing multiple items (sort fixes two, auto-fix four) changes/undoes them together. Section counts, opening sentence and sidebar badge follow; auto-fix N counts resolved items and disappears when done.
- **Columns link to each other.**
  - Missing dependency left says Vanilla Furniture Expanded errored 12 times last run, see right; right says dependency disabled, see left. Click jumps/flashes. After enabling, right says dependency just enabled, check next run.
  - Shared-log hint and last-run errors likewise: this Prison Labor version often errors; last run 41 times.
- **Right updates live while game runs.** Top simulate-game adds a current-run error every 2.5s:
  - Heading becomes current-run errors / ● game running, new rows first.
  - Thin divider and "last run  list  Oct 28 21:04", then previous run.
  - Left stays still.
- Section headings stick within columns; ← → remembers each cursor, Tab to buttons moves cursor along; `u` undoes last action, `F` auto-fixes.

Specification/code changes needed:

- 06 §2.4 layout and §5 one-click actions column (all actions visible, choice buttons, `1` `2` `3`).
- §5 corpus hint color/position.
- 03 §4.3.1 weak-order/cycle display.
- TUI 80 columns cannot fit two: same order in one column, three left sections then logs.

## First-draft recommendation (since changed to A)

Originally recommended **B** as the base: the only candidate that really simplified the page. Popovers/`⋯` merge into right details, allowing deletion (`versionpanel.js`, `popups.identity`, finding popover, folding actions), not another rendering method. TUI could share it: left/right above 120 columns, details below when narrower.

Two cheap C ideas could go straight into B:

1. Merge weak order/cycles into automatic sorting, which already handles them, instead of separate items.
2. Log errors name related item (its Vanilla Expanded Framework dependency is disabled, see that item). B's right already shows this.

At the time A seemed only to split the same volume across two sides. Second draft's visible actions, choice buttons, stable finished rows and cross-links gather content back into each item, invalidating that objection.

## Open

Pages share `shared.css`, `shared.js`; use a local server, for example:

```bash
python -m http.server 8765 --directory docs/design/canitrun-drafts
```

Open `http://localhost:8765/now.html`; top bar switches four pages.

- All pages: ↑↓ move, ← → change section (column in A), enter primary action.
- A: button keys execute directly, `u` undo; top simulate-game demonstrates live right column.
- B: left-row click changes right details.
- C: step buttons mark complete, count/progress update, undo restores.