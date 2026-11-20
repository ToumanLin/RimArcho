# Import page: two arrangements, drag and drop (2026-11-20)

Status: export removed and drag and drop decided (16 §2.18, CHANGELOG, release-vm-tests); the
arrangement is **for the user to pick**. Until then `<html data-import-layout="2col|3col">` or
`?import-layout=2col|3col` switches it (`layout()` in `exchange.js`; default 2col). Once one is
picked: drop the other's CSS (`.xc-lay[data-layout="3col"]` or the 2col areas), the switch, and
the "under review" words in 16 §2.18 and §7.1, and add the layout's VM row.

## What the user asked

- Rename "Import & export" to "Import"; no export on the page.
- Re-explore the page: (1) two columns, the left choosing the kind of import, the right
  previewing what comes in and comparing it with what the player has now; (2) three columns.
- Import by dragging files or folders onto the window.

## What the page imports (inventory, unchanged paths)

| Kind | Source | Preview / apply |
| --- | --- | --- |
| Game's list | `Exchange.Game` (the current Game Config's ModsConfig.xml) | `ImportPreviewFor(target, "file", path)` → destinations → `ImportAsPlan` / `ImportApplyTo` (+ `ImportFollow`, `ImportSettings`) |
| List file | `PickAnyImport` → `ManagerFileKind`; a browser file's text | same preview, `kind "text"` for content read in the page |
| Link or code | pasted text, `Clipboard` | same preview, `kind "text"` (links fetched from Steam or Rentry) |
| Pack | `PickPackFile` | `PackUpdateOffer` → `StartPackImport` → `TaskByID` polling |
| Other managers | `ImportSurvey(extra)` on open; `PickManagerImport` adds a source | tree of parts → `StartMigration` → `TaskByID` polling |

## The two arrangements

Both share: a **kinds column** (200 px, sticky): five rows, an icon and a short name each
(game's list, list file, link or code, pack, other managers), the game list's Mod count and the
number of managers found (with a dot when one has news) after the name, the full sentence on
hover; ↑↓ move through them. Picking the game's list reads it at once. **Archo stands under the
kinds** at 176 px, left-aligned, in that column's blank space: never the centre, nothing grows,
her head and hair never cut (her chest fades as on Can it run), and since the column is sticky she
stays in view while a long preview scrolls. The **preview** gained a comparison row: one bar of
four kinds with icon + count under it (✓ already in your list, + installed but not in it, versions
mark for another version, ✕ not installed) and, at its right, the current list's name with what
replacing it does as + − ↕ ⇄. Words are on hover and in `aria-label` (`compareParts`).

### 2col: kinds | source over preview

Page up to 1400 px wide, centred. The right column holds the picked kind's source card (a short
row for the game's list, a 168 px drop area for a file or a pack, a three-line field for a link,
the manager rows) and the preview under it.

- + One reading column: the source is directly above what it produced; the preview gets the full
  right width (long Mod names, the subscribe buttons, the managers' tree with "what it becomes").
- + At 1280 nothing is cramped; at 2000 the 1400 px cap keeps lines readable.
- − A long preview scrolls the source out of view (the page scrolls to the preview), so going back
  to "choose another file" means scrolling up or picking the kind again.
- − Other managers: the manager rows and their tree are stacked, so a long tree pushes the rows
  away.

### 3col: kinds | source | preview

From 960 px of page width the source and the preview stand side by side (`200px | 1fr | 2fr`);
narrower it falls back to 2col. The page fills the window like the other multi-column pages.

- + Source and result are both always in view: drop another file next to the preview you are
  reading; the manager rows stay beside their tree (the best case for other managers).
- + At 2000 the window is used; nothing is capped.
- − At 1280 the preview is ~500 px: Mod rows wrap ("subscribe" drops under long names), the source
  facts wrap to three lines, and the source column is mostly empty for the game's list, a pack or a
  list file once it is read.
- − Three things side by side is more to scan than the user's "visuals over text" direction wants
  on a page most players visit once.

Why this split for 3col: the source is the only part whose size depends on the kind (a button, a
drop area, a field, a list of managers) and the preview is the only long part, so keeping them in
separate columns lets each keep its own height; kinds | (source + preview) | compare was tried in
thought and rejected because the comparison is one row, not a column's worth.

## Recommendation

**2col.** It answers the user's description (left: what kind; right: what comes in and how it
compares) literally, reads well at 1280 (the most common width) and keeps one place to look. Its
weakness, the source scrolling away under a long preview, is small because the kinds column (and
Archo) stay in view and a click on the kind brings the source back. 3col wins only for other
managers at wide widths; if the user often moves in from other managers on a wide screen, 3col is
the alternative.

## Drag and drop

- **Everywhere, not just on Import.** A file dragged over any page dims the window under one
  dashed box ("drop to import", list / pack / manager icons; `pointer-events: none`). Dropped, it
  opens Import with the right kind. Chosen because the player does not know which page owns an
  import (C1), and the spec already promised "a file dropped anywhere". Forms that take drops
  themselves (All lists' new / import) keep them (`defaultPrevented`).
- **Kind by name** (`dropKind`): `.xml .rml .json .txt .rws` → list; `.archopack` → pack; `.db`,
  `userRules.json` → other managers; a folder → other managers; anything else → refused in the
  preview ("Not imported", a red stop mark, what RimArcho imports), Archo shakes, the status line
  says it. Several files: the first one, with a status line saying so.
- **Paths.** A browser `File` has no path, so a list is read from its content; a pack, a database
  or a folder needs a path. `dropped({ path })` sends a path where "choose a file…" would.

### Finding: the Windows window receives no drops (app bug)

Wails v3 beta.28 delivers dropped paths through `EnableFileDrop` + `WindowFilesDropped`: the
runtime's `drop` listener posts the `File` objects with
`chrome.webview.postMessageWithAdditionalObjects`, Go reads `ICoreWebView2File.GetPath`, and the
event's `DroppedFiles()` holds the paths (`examples/drag-n-drop`). But RimArcho sets
`WebView2CompositionHosting = true` (`gui.go`, for Win11 Snap Layouts). Under composition hosting
the host must register an OLE `IDropTarget` and forward `DragEnter/DragOver/Drop` to
`ICoreWebView2CompositionController3`; Wails beta.28 has the binding
(`internal/webview2/.../ICoreWebView2CompositionController3.go`) but never calls it and never calls
`RegisterDragDrop`. So no drop reaches the page at all in the native window: not the old
content-reading drop the spec described, not Wails' path drop (`lists.js` already notes that the
window has no OLE drop target for row drags). The brief said to stop and report options rather
than build a larger change, so the page's drop UI works in the browser harness only for now.

Options:

1. **Own `IDropTarget` in Go** (recommended): a small COM object (like `winmem_windows.go`'s raw
   COM calls) registered on the main HWND with `RegisterDragDrop` on the UI thread (after
   `OleInitialize`). `DragEnter/DragOver/DragLeave` emit `file-drag` events (the page shows the
   overlay), `Drop` reads `CF_HDROP` with `DragQueryFileW` and emits `files-dropped` with paths;
   the page calls `dropped({ path })`. About 200 lines, Windows only; keeps Snap Layouts. Folders
   come for free (CF_HDROP lists them). Needs a VM check with a real Explorer drag.
2. **Forward to the composition controller**: the same `IDropTarget`, but forwarding to
   `ICoreWebView2CompositionController3.DragEnter/…/Drop`, so the webview gets real HTML5 drag
   events and Wails' `EnableFileDrop` pipeline gives paths. Needs Wails' internal controller
   pointer (not exported): a Wails patch or `unsafe` reach into its window struct.
3. **Turn off composition hosting**: `EnableFileDrop: true` then works as in Wails' example, but
   the maximise button loses Win11 Snap Layouts (16 §8.2) and the window's frame code changes.
4. **Wait for Wails** to forward drops under composition hosting (upstream issue).

## Evidence

`D:\RimArcho-test\import-redesign\v2\`: `shots\<layout>-<state>-<1280|2000>-<light-en|dark-zh>.png`
(18 states × 2 layouts × 2 widths × 2 combos), contact sheets `sheet-<width>-<combo>.png` (rows:
states; columns: 2col, 3col) and `sheet-compare-<width>.png` (key states, both layouts and combos).
States: empty, first run, each kind (game's list read at once, list file, link or code, pack, other
managers), the managers' tree, loading, a preview with differences (another game version, 7 not
installed, 3 in another version), apply busy (import, pack, move-in), read error, drag over Import
and over Start, a refused drop, a pack dropped without a path. Synthetic data (stubbed API calls
over guidev with a synthetic home), reduced motion. Harness: `shoot.mjs`, `sheet.py` there.
