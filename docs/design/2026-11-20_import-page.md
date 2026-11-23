# Import page: the user's sketch, three arrangements of the right, drag and drop (2026-11-20)

Status: decided (16 §2.18, CHANGELOG, release-vm-tests). The user picked **2col-new** on 2026-11-20:
the bar over a result (`.xc-bar`, `dock()` and `place()` in `exchange.js`). The layout switch
(`data-import-layout`), 2col's in-card head and foot and 3col's rail are gone; the sections below keep
how the three compared.

## What the user asked

- Rename “Import & export” to “Import”; no export on the page.
- Round 1 (rejected): a kinds column (icons + short names) with the source over or beside the
  preview, and a four-part comparison bar. The user found both arrangements tasteless.
- Round 2 (the user's sketch): left, the ways in as rows with their buttons (the game's current list,
  a Mod list, Mod subscriptions, another manager's data with its cards and “Not found? Add it
  yourself”); right, an empty dashed pane with Archo in the middle and the drop sentence under her;
  “read” shows a side-by-side order comparison like RimCrow's; a manager's card previews its data
  there. Then, on seeing it: no icon tiles, a simpler right side, no connecting lines, Archo not under
  the left column.
- Import by dragging files or folders onto the window.

## The left column (both arrangements)

One card of rows, each a name, one muted line and its button; no icons:

| Row | Button | Goes to |
| --- | --- | --- |
| The current list of <Game Config> (N mods) | read | the order comparison on the right |
| Import a Mod list | import | All lists' new / import form (`newList("import")`): share codes, links, files |
| Import Mod subscriptions | import | add a subscription (Workshop, LoversLab, Git links) |
| Import a pack | import | `PickPackFile` → the pack's task on the right |
| Import another manager's data | — | manager cards (click: the tree on the right), then “Not found? Add it yourself” “add…” |

Deviation from the sketch: **Import a pack** is an extra row. The page was the pack's only way in
(the Packs page has no import), and drops do not reach the native window yet, so without the row a
pack could not be imported at all.

## The order comparison (`listdiff.js`)

`ImportPreview` gained `current` and `incoming`: both lists in order, keyed by package id, the
incoming rows marked missing or another version. `orderDiff` keeps a longest common subsequence of the
shared Mods as “unchanged” (same number, or shifted by additions and removals); every other shared Mod
“moved”; Mods only on the left are “removed”, only on the right “added”. Runs of more than four
unchanged rows fold (`folded`, one row kept at the head, and at the tail inside a list; a click opens).

Drawn as one line of counts (dot, word, number; the sentence on hover), “N not installed” with
“subscribe all”, a close mark; then the two lists, 30 px rows, tinted with a 3 px edge. Hovering a Mod
outlines it on both sides. RimCrow's curves between the columns, its “colour each block” switch and
its fold switch were built and then removed at the user's word: the lines cost a 64 px gutter and
read as noise, the switches were text the player had to learn. The source (format, file, game version)
moved into the incoming header's tooltip; another game version stays visible as an amber “RimWorld 1.5”
there.

## The two arrangements of the right

- **2col**: the comparison card fills the right; its head is the counts line, its foot the buttons
  (“add only what it lacks”, “save as a new list…”, “apply to <list>”). Archo appears only in the
  empty pane.
- **3col**: a 220 px rail at the right: Archo at its top, the counts as a column, the buttons stacked
  full width. The comparison card is only the two lists and gets the full height. The rail is there
  before any result too (Archo in it, the drop pane without her): when it appeared only with a result
  the left column shrank by 130 px on the first click (the user saw it jump on picking a manager).
- **2col-new** (the user, round 4): the 3col rail's content over the result in the right column: the
  counts card (one line) and the buttons under it at the left, inset 14 px, and Archo at her full size
  at the right (a first try laid it out as a 72 px bar with a small Archo; the user: keep her size, the
  card at the left), shown with a result. The lists keep 2col's width; the buttons sit at the top instead of the
  foot. Empty, the page is the same as 2col.

The left column keeps its width in all three whatever the right shows (measured in the harness:
330 / 549 px at 1280 / 2000 in 2col and 2col-new, 300 / 419 px in 3col, before and after a result).

## Round 4 (the user, on the 3col build)

- **One row at a time**: reading the game's list kept a picked manager's card marked next to the
  game row. A result from any other row now drops the managers' picks (`showResult`).
- **No jump**: see 3col above.
- **“view” on each part of another manager's data** (`API.ManagerPartView`, `app.MigrationPartView`):
  a saved list or a RimCrow profile gives its list file (a profile's `Config/ModsConfig.xml`), shown
  as the order comparison with only “back”; Mod data and rules come from the importers' dry runs, which
  now record each value (`RimCrowReport.Items`, `PartItem`: the Mod by lineage, the value, a rule's
  second Mod, whether RimArcho has a value there), at most 20,000 per run, shown as rows (Mod, value),
  grey where RimArcho has it. “back” redraws the tree with the picks kept.

At 1280 px 2col gives each list ~290 px (names rarely cut); 3col ~200 px (long names cut, the
full name on hover). At 2000 both are comfortable; 3col keeps Archo and the buttons in view beside a
long list.

**Picked: 2col-new** (the user, 2026-11-20; 2col and 3col were removed from the code). The recommendation was the same: It keeps 2col's list width and Archo beside a result, and the counts and
buttons are in view at the top of a long list without a third column. 2col stays the simplest; 3col
only wins on very wide windows.

## Drag and drop

- **Everywhere**: a file dragged over any page dims the window under “drop to import”; dropped, it
  opens Import. Forms that take drops themselves (All lists' new / import) keep them.
- **By name** (`dropKind`): `.xml .rml .json .txt .rws` → a list (compared); `.archopack` → a pack
  (its task); `.db`, `userRules.json` → another manager's data; a folder → a manager's data folder.
- **Failures, all on the right, Archo shaking, the status line saying it**: an unknown name → “Not
  imported” with what RimArcho imports; a list that cannot be read → “could not read it” and why; a
  folder or file with no manager's data → “no RimSort, RimCrow or RimPy data in it”; several files →
  the first, and the status line says one at a time; no path (browser only) → amber, which button to use.
- The user's answers to the open questions are under “Drops that are not imports” below.

### Finding: the Windows window received no drops (app bug, fixed 2026-11-20 with option 1)

Wails v3 beta.28 delivers dropped paths through `EnableFileDrop` + `WindowFilesDropped` (the runtime
posts the `File` objects with `chrome.webview.postMessageWithAdditionalObjects`, Go reads
`ICoreWebView2File.GetPath`). But RimArcho sets `WebView2CompositionHosting = true` (`gui.go`, for
Win11 Snap Layouts). Under composition hosting the host must register an OLE `IDropTarget` and forward
`DragEnter/DragOver/Drop` to `ICoreWebView2CompositionController3`; Wails beta.28 has the binding but
never calls it and never calls `RegisterDragDrop`. So no drop reaches the page in the native window.

Options:

1. **Own `IDropTarget` in Go** (recommended): a small COM object registered on the main HWND with
   `RegisterDragDrop` on the UI thread (after `OleInitialize`). `DragEnter/DragOver/DragLeave` emit
   `file-drag` (the page shows the overlay), `Drop` reads `CF_HDROP` with `DragQueryFileW` and emits
   `files-dropped` with paths; the page calls `dropped({ path })`. About 200 lines, Windows only; keeps
   Snap Layouts; folders come for free. Needs a VM check with a real Explorer drag.
2. **Forward to the composition controller**: the same `IDropTarget`, forwarding to
   `ICoreWebView2CompositionController3`, so Wails' own pipeline gives paths. Needs Wails' internal
   controller pointer: a Wails patch or `unsafe` reach into its window struct.
3. **Turn off composition hosting**: `EnableFileDrop` then works, but the maximise button loses Win11
   Snap Layouts (16 §8.2).
4. **Wait for Wails** to forward drops under composition hosting.

**Done (2026-11-20, at the user's word)**: option 1, `internal/surface/gui/drop_windows.go`. A Go COM object
(its vtable from `syscall.NewCallback`, typed pointer arguments so `go vet` passes) is registered with
`RegisterDragDrop` on the window's handle on the UI thread once the runtime is ready. It accepts a drag
that offers `CF_HDROP`, `UniformResourceLocatorW` or one http(s) address as text, and refuses the rest
(so the page's own drags still never drop). It sends `file-drag` and `files-dropped` (paths, folder or
not, link). Tried on the host with the real window on a synthetic home (`RIMARCHO_HOME`): a list and a
Mod folder dragged from Explorer arrived. Internal HTML5 drags (the tag order chips) still do not drop in
the window; forwarding to the composition controller (option 2) would fix that too.

## Drops that are not imports (the user's answers, round 5)

- Several files: only the first is taken.
- A Mod folder or zip, and a link: go to Library › add a subscription with it in the box; its card asks
  (dry run, then “install it” or “subscribe”) and only then installs as a local Mod or subscribes.
  `archo subscribe local <zip|folder>` (`app.InstallLocalMod`) copies into the game's Mods folder under
  a free name; `subscribe.Probe` checks for About/About.xml without copying.
- Add a subscription takes drops the same way (it is filled in place), and gets “choose zip…” and
  “choose folder…” for a local Mod.

## Evidence

`D:\RimArcho-test\import-redesign\v3\`: `shots\<layout>-<state>-<1280|2000>-<light-en|dark-zh>.png`,
contact sheets `sheet-<width>-<combo>.png` (rows: states; columns: 2col, 2col-new, 3col) and
`sheet-compare-<width>.png`. States: empty, first run, the game's list compared, a 205-vs-212 list
with 7 not installed and another game version, a list already the same, the managers' tree, loading,
apply busy, pack busy, move-in busy, read error, drag over Import and over Start, a refused drop, a pack
dropped without a path, a folder with no manager's data. Synthetic data over guidev, reduced motion.
Round 1's shots stay in `v2\` for comparison.
