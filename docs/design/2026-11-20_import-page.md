# Import page: the user's sketch, two arrangements of the right, drag and drop (2026-11-20)

Status: export removed, drag and drop and the left column decided (16 §2.18, CHANGELOG,
release-vm-tests); the arrangement of the right side is **for the user to pick**. Until then
`<html data-import-layout="2col|3col">` or `?import-layout=2col|3col` switches it (`layout()` in
`exchange.js`; default 2col). Once one is picked: drop the other's CSS (`.xc-rail` and the
`[data-layout="3col"]` rules, or nothing for 2col), the switch and `place()`, the “under review” words in
16 §2.18 and §7.1, and add the layout's VM row.

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
- **3col**: with a result, a 220 px rail appears at the right: Archo at its top, the counts as a
  column, the buttons stacked full width. The comparison card is only the two lists and gets the full
  height. Empty, the page is the same as 2col.

At 1280 px 2col gives each list ~290 px (names rarely cut); 3col ~200 px (long names cut, the
full name on hover). At 2000 both are comfortable; 3col keeps Archo and the buttons in view beside a
long list.

**Recommendation: 2col.** It is the user's sketch as drawn, the lists get the width, and the right
has one place to look. 3col only wins on very wide windows.

## Drag and drop

- **Everywhere**: a file dragged over any page dims the window under “drop to import”; dropped, it
  opens Import. Forms that take drops themselves (All lists' new / import) keep them.
- **By name** (`dropKind`): `.xml .rml .json .txt .rws` → a list (compared); `.archopack` → a pack
  (its task); `.db`, `userRules.json` → another manager's data; a folder → a manager's data folder.
- **Failures, all on the right, Archo shaking, the status line saying it**: an unknown name → “Not
  imported” with what RimArcho imports; a list that cannot be read → “could not read it” and why; a
  folder or file with no manager's data → “no RimSort, RimCrow or RimPy data in it”; several files →
  the first, and the status line says one at a time; no path (browser only) → amber, which button to use.
- Open questions for the user: several files queued as several imports; a Mod folder or `.zip`
  installed as a local Mod; a dragged Workshop / LoversLab / GitHub link sent to add a subscription.

### Finding: the Windows window receives no drops (app bug)

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

## Evidence

`D:\RimArcho-test\import-redesign\v3\`: `shots\<layout>-<state>-<1280|2000>-<light-en|dark-zh>.png`,
contact sheets `sheet-<width>-<combo>.png` (rows: states; columns: 2col, 3col) and
`sheet-compare-<width>.png`. States: empty, first run, the game's list compared, a 205-vs-212 list
with 7 not installed and another game version, a list already the same, the managers' tree, loading,
apply busy, pack busy, move-in busy, read error, drag over Import and over Start, a refused drop, a pack
dropped without a path, a folder with no manager's data. Synthetic data over guidev, reduced motion.
Round 1's shots stay in `v2\` for comparison.
