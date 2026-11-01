# 06 · Interface · GUI technology (§8)

> Part of [06 · Interface](README.md). Section numbers run through all of 06 and stay unchanged when split across files. References still use `06 §x.y`.

## 8. GUI technology

### 8.1 Delivery

One source tree, linked twice:

| File | Subsystem | Double-click | Purpose |
| --- | --- | --- | --- |
| `archo.exe` | Console | TUI in default terminal (§9.2) | CLI, `archo mcp`, `archo tui`; `archo gui` starts GUI |
| `RimArcho.exe` | Windows GUI (`-H windowsgui`, no console window) | GUI | Player's double-click entry, accepts `--env <name>`; with command arguments runs as CLI for GUI scan / analysis subprocesses |

- Both share data directory / database and may run together (multiprocess SQLite WAL). On focus, GUI compares `Stamp()` (database state, `ChangeKey`), reloading if changed; edits from stale views never overwrite the other's changes (G8).
- `archo gui` starts adjacent `RimArcho.exe` and immediately returns if present; otherwise runs GUI in current process.
- First opening an old database migrates it (seconds). After 300 ms, `RimArcho.exe` shows a small nonclosable progress window before the main window; TUI / CLI write one stderr line. Rules: 02 §7, startup upgrade progress.
- Releases contain both executables (sizes: 07 §6).

### 8.2 Frontend

- Wails `v3.0.0-beta.28` (v3 is beta; pinned, validate upgrades separately). No CGO (`CGO_ENABLED=0`).
- Plain HTML + CSS + native JavaScript (ES modules). **No framework, npm, or build step**. `internal/surface/gui/assets/`: `index.html`, `style.css`, `js/` containing `api.js` `state.js` `lists.js` `detail.js` `screens.js` `popups.js` `exchange.js` `icons.js` `shell.js` `home.js` `guide.js` `plans.js` `motion.js` `main.js`; embedded in exe with `go:embed`.
- Go calls: handwritten `api.js` uses `import { Call } from "/wails/runtime.js"`, `Call.ByName`, and `Proxy` to assemble `github.com/ToumanLin/RimArcho/internal/surface/gui.API.<method>`. No `wails3` generated bindings; only `go build` required.
- `internal/surface/gui`: `api.go` (Wails service, methods only call view), `gui.go` (window, `/preview/`, `/img/` local handlers, CSP, icon routes), `exchange.go` (import / export dialogs), `text.go`.
- Go → frontend: Wails events (`scan` complete, `analyze` progress / complete, `pack` build progress). Frontend polls `Live` for runtime status (§2.16.1 G01).
- Virtual scrolling creates visible rows only.
- **Page reads** (§3.1 rule 9, enter before reading): **every page** uses `beginPageRead` to navigate and show a skeleton before waiting: launch, can it run, changes, packs / build preview, settings, environments, guide, uninstall preview, library, editor, game log, texture optimization, all lists, background tasks, full-screen details. Same-page refresh keeps content; switching Mods / packs shows matching placeholders so old-object actions cannot be clicked. Validate page code, request sequence, environment, plan on every read; late success / error cannot overwrite new pages. Only latest repeated request accepted; failures end loading and permit retry. Delayed Workshop descriptions in full-screen details use the same checks. Polling, retries, detail / subscription debounce, idle session release: §2.16.1.
- **Preview action state**: reread preview after pack type / texture choices change; even shortcuts cannot build from old previews while pending. Remove old confirmation buttons while re-estimating uninstall selection; show next actions only after complete results.
- First stylesheet rule: `[hidden]{display:none !important}`; overlay container `display:flex` otherwise overrides hidden, leaving an invisible click-blocking mask.
- WebView2 data: `<data directory>/webview2`, not `%APPDATA%\RimArcho.exe`.
- Development-only `spikes/guidev` drives pages with the same API on 127.0.0.1 for browser debugging; `-assets <directory>` reads disk assets, refresh to see edits. Not shipped.
- `/dev/pagecheck` uses mocked APIs to check immediate feedback, late results / errors, reentry, failed retries; also variable-height large logs / keyboard positioning. Real frontend modules, no product database.

**Windows window**: borderless main window with WebView2 composition hosting, retaining Wails default decorations / resizing. `window-chrome.js` minimizes, maximizes / restores, closes. Browser debug pages / other platforms retain their native window form without these buttons. Blank space in top navigation row drags, double-click maximizes / restores; buttons, links, inputs, splitters never drag the window. Maximize uses Wails native hit region for Win11 Snap Layouts. Wails / Windows handle edge resize, Aero Snap, maximized work area, shadows, corners; verify in VM. Close must call `Window.Close()` and cancellable `WindowClosing`, sharing interception with Alt+F4 / taskbar close; never call application quit. Buttons follow page light / dark theme; placement §2.2.

Controls allow Wails system info to arrive after the page; add buttons once Windows is confirmed asynchronously, keeping current navigation position. Do not delay first paint.

### 8.3 Security

- CSP: `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`. Wails works with `script-src 'self'`, no `unsafe-eval`.
- Workshop descriptions / images are external: Go parses BBCode blocks, frontend builds elements with `textContent`, **never inserts external text as HTML**. Images only through `/img/` from local cache, never direct remote URLs.
- Links open system browser; `OpenURL` refuses non-http(s).
- On Windows, Wails intercepts WebView2 resources, opening no TCP ports.
- Settings executes only commands allowed by `view.SettingCommand` (§2.14).

### 8.4 Long tasks

- Unified command panel `RunCLI` immediately returns task state; `CommandStatus` reads latest result. `command.go` worker owns independent App session, cancelled and awaited on close. Normal commands use `cmds.Check` / `ExecuteContext`; scan / analysis use no-console `ChildCommandContext`, preserving text / exit code. Arguments passed directly, no shell. Completion, history, parsing share `internal/cmdline` (§3.5).

- Scan / analysis subprocesses: `archo scan`, `archo analyze`, `app.ChildScan` / `ChildAnalyze`, `CREATE_NO_WINDOW`; progress events to status, no blocking. Same in TUI, including `:scan`.
- Startup: show indexed list immediately, incremental scan in background; first-time scan starts automatically.
- Adding game (§2.15, §2.8): scan / first analysis in subprocesses; guide / other pages usable; skeletons until game data ready. Go GUI methods do no work likely exceeding about 100ms inline; goroutines / subprocesses return events (§3.1 rule 9).
- `Workspace` reads list / sidebar / history together, returning events. Unchanged needs key reuses full workspace snapshot. Rebuild reuses separate read session, detached index / editor-row cache, builds in short WAL read-only snapshot, never loads full library under page lock. Sidebar shares needs; publishing checks environment, request sequence, list edit generation, input key.
- Pack builds background, progress to build page, stoppable (§2.6).
- These and library file / Steam actions become background tasks (§2.16, `app.Tasks`): running, queued, past in one place, persisted to `<data directory>/tasks.json`.

### 8.5 Mouse

Mouse actions are another trigger for editor verbs:

| Action | Keyboard | Mouse |
| --- | --- | --- |
| Move cursor / area | `↑↓` / `←→`, `1`–`4` | Click |
| Select | `shift+↑↓`, `ctrl+space`, `esc` clears | `shift+click` range, `ctrl+click` toggle |
| Enable / disable | `space` | Double-click; drag between columns |
| Stage / put back | `t` | Drag into / out of Staging |
| Reorder | `J` `K`, `m` position input | Drag in Enabled; drop line says position N |
| Tags | `g` | Context menu > tags |
| Undo / redo | `u` / `ctrl+r`, `ctrl+z` / `ctrl+y` | Status undo link |
| Sort / write / launch | `S` / `w` / `L` | Top-right buttons |
| Search / filter | `/`, `ctrl+f`, `\` | Filter bar above Disabled; Staging search |
| Context menu | Menu key, `shift+F10` | Right-click |
| Column width | `<` `>` `=` | Drag separator, double-click reset |
| History | `H`, `u` / `ctrl+r` | Click history step |
| Page back / forward (§3.1 rule 11) | `alt+←` / `alt+→` | XButton1 / XButton2 |

- **Drag rules**: Disabled / Staging to an Enabled row places at its number; below last row appends; within Enabled moves; to Staging stages; Enabled to Disabled disables; Staging to Disabled unstages without enabling (`view.Unstage`). Press / release same row is click. Status describes drop target; `esc` cancels drag. Release applies, `u` undoes.
- **Context menu** (editor / inventory): shared action table / group order, keys at right, applicable only (`view.RowOps` / `view.CopyOps`, names / keys from `view.OpTable`):
  1. Plan (editor only, whole group absent in inventory): `space` enable / disable / put back, `m`, `t`, `x`, remove from all plans;
  2. Organize: tags `g`, note `n`;
  3. Open: folder `ctrl+o`, Workshop `ctrl+w`;
  4. Versions: lock this version, update locked version, unlock, back up locked version;
  5. Disk: restore, clear existing converted textures, resubscribe / subscription-source actions, red unsubscribe / delete (`Del`) last. Inventory has no single-Mod DDS conversion; resubscribe confirmation explains folder is first moved to recycle bin, then Steam redownloads; conditions §2.9, cleanup verification / Steam 08 §10.

  Thin line between groups. Menu key / `shift+F10` opens on cursor row, first item focused; `↑↓` / `tab` moves, `enter` executes, `esc` closes; displayed keys execute directly. Multiselection shows only batch-capable actions (folder, page, note, position, switch copy require one).
- Arrows, wheel, single-click never mutate (§3.1 rule 1); only double-click, drag, buttons, verbs do.
- **GUI / TUI differences** (2026-10-24): page history / `alt+←` / `alt+→` GUI only; TUI keeps page switching / technical stack return, no side-button handling.
- TUI mouse: click cursor, `ctrl+click` add selection, `shift+click` range, drag column gap to resize, click detail area to focus, wheel scroll details. No Mod drag or rectangle selection.

### 8.6 Icons and Windows resources

- **Artwork** (outline-glow revision 2026-10-25): `internal/icon/portrait.svg`, 1024×1024 vector, rounded dark `#1B1D23` background, radius 0.22 side length. Full image ≥ 48px; head / hair / face only ≤ 40px, viewbox x 120–910, y 170–960, recognizable in taskbar; face-only crop too tight, merely skin at small sizes.
- **Renderer**: dependency-free small SVG renderer `internal/icon/svgr.go`, element-tree drawing: `<g>` inherited fill / stroke; matrix, translate, scale, rotate, skewX/Y; paths, rounded / plain rectangles, circles, ellipses, lines, polylines, polygons; nonzero / evenodd fills; rounded stroke joins; solid / linear / radial gradients in user / bounding-box coordinates with gradientTransform; opacity / fill-opacity / stroke-opacity; clip-path; only feGaussianBlur filters. defs / clipPath / gradients / filters render only when referenced. 4× supersampling; all sizes through 256px about 1 second.
- **No committed binaries**: public repository contains no binaries. `icon.PNG(size)` antialiased per-pixel supersampling for window / resources; `icon.SVG(size)` same vector composition generates `/icon.svg` (256), `/icon-small.svg` (16) favicons.
- **Window icon**: Wails first reads exe icon group ID 3, then supplied PNG fallback. Release does no rendering; development without `.syso` renders system large-icon size (`SM_CXICON`, 32px at 100%), immediately `FreeOSMemory`. Previously 256px cost 43 MB heap (32 MB float64 4× buffer) just for 32px icon, with Go retaining memory afterward.
- Release calls `go run ./scripts/winres`, writing `cmd/archo/rsrc_windows_amd64.syso`, COFF resource with icons / version info; ignored by `.gitignore`, not committed.
- **Icon group ID 3**: Wails reads ID 3, Explorer first group; same group. Sizes 16, 20, 24, 32, 40, 48, 64, 256, one PNG each.
- **Version info**: `FileVersion` / `ProductVersion` release version, `ProductName` RimArcho; `FileDescription` RimArcho command line for `archo.exe`, RimArcho for `RimArcho.exe`. Same package builds both; release script writes `.syso` separately between builds.
- **Without resources**, development / `spikes/guidev` uses `application.Options.Icon` PNG (256 px) fallback.


### 8.7 List findings and needs cache API

Editor / can it run share backend snapshot. Entries `API.Needs`, `API.ListProblems`, `Board.problems`; structs [`view.Needs`](../../../internal/view/needs.go), [`view.ListProblems`](../../../internal/view/listproblems.go); cache [`gui/needscache.go`](../../../internal/surface/gui/needscache.go).

**When to call**:

- Workspace `environment` event already has `board.problems`; use directly for first paint.
- Can it run calls `Needs()`; editor-only calls `ListProblems()`, same cache.
- Edits returning `view.Result`, e.g. `Toggle`, `Stage`, `Unstage`, `MoveBy`, `Place`, `StageAll`, include summary in `result.problems`. String-returning `Undo`, `Redo`, `DoAction` publish same structure via `list-problems`.
- Background log / content completion also emits `list-problems`; retain `analyze` progress / completion events. No extra request just to refresh markers. Open can it run redraws with `Needs()` on events, preserving cursor; reread on game exit (`Live` running → stopped) replaces this run with full analysis. Previously sidebar alone refreshed, stale page kept mid-run time until leaving / returning (VM 2026-10-28).

Main `ListProblems` JSON fields:

| Field | Purpose |
| --- | --- |
| `environment`, `plan` | Environment ID / open list ID filter late pre-navigation events; launch still reads current list |
| `listVersion` | List-content fingerprint |
| `revision` | Cache input fingerprint; same input reused, no lexicographic comparison |
| `sequence` | Increasing API-session publication number; same environment / list accepts only ≥ displayed sequence |
| `updating`, `error?` | Retain previous content findings with `updating=true` while completing; failure retains findings and error text |
| `count`, `severity` | Shared needs count / maximum `error` / `warning` / `info`; hidden maintenance / muted info follow needs counting |
| `mods` | Instance-ID index; `count`, `severity`, `summary`, `problemIds`; absent Mods have no displayable findings |
| `items` | Full list-level `Need` array, each finding once, lookup by `problemIds` |
| `status`, `reasons` | Snapshot row icons / per-Mod explanations by Instance ID; replace directly on event, never rebuild from `items` aggregate titles / actions (version warnings §5) |

`version-mute` / `version-unmute` `refs` are package references; `gameVersion` target major.minor. `version-list` carries only `gameVersion`, `versionMuted`; opening reads `API.VersionWarnings` → `view.ReadVersionWarnings`, returning title, all actions, groups with `{id, name, supported, action}` rows. Rows excluded from normal Board / Needs. Individual / group / all actions still `DoAction(action, 0)`; batch ignore uses `confirm` / `warn`; interaction §5.

`Need` adds `id`, optional `target`, `requiredBy`. Dependency `target`: packageId trimmed, case-normalized, `_steam` removed, directory aliases resolved via registry; `requiredBy`: `{id, name}` array. Declared / corpus missing dependencies and patches with disabled targets merge by target; all requiring Mods own the finding. If an existing uninstalled list entry covers the dependency, attach ownership to its aggregate item. Strong order findings still merge into one automatic-sort item; individual strong constraints also have `move`.

**Execute**: pass full `item.actions` Action to `DoAction(action, option)`, not ID alone. `id` identifies; `kind`, `refs`, `after`, `options` retain arguments. `choose-disable` option zero-based; other data actions `0`. `enable` positions dependencies by constraints; `move` one `refs` / `after`; `sort` direct; `subscribe` existing task queue. DLC store via `open-url`. Surface dispatches `view`, `list`, `fold`, `save`; confirmation still `confirm` / `warn`.

**Use installed copies**: `API.UseInstalled(packageIDs []string) (*view.Result, error)` binds missing open-list entries, preserves positions, batch undoable; semantics / refusal 04 §4.2. packageId array accepts case / whitespace / `_steam`, not Instance refs; empty no-op. Result `text`, `undo`, `problems` follow edit contract. Can it run separates `MissingMods.CanEnable` as can use installed copies; primary `kind: "use-installed"`, packageId `refs`, `undo: true`, `key: "space"`, `label`, `done`, action `id`, no `confirm` / `warn`. Full action to `DoAction(action, 0)` returns string / publishes problems, or `refs` to `UseInstalled` returns edit result. Only no-copy entries keep subscribe / remove. Sort Outcome `missingMods` unchanged, frontend offers fixes via `canEnable`.

**Cache bounds**: at most eight environment / list / game-version / language slots per API session. Inputs also list content, database change key, scan snapshot / analyzer version (`AnalysisKey`), log path / size / mtime, freshly checked directories / game links. Workspace reuses full Board / Shell / History; rebuild with separate connection, no main lock held.

List edits use `App.IncrementalProblems`: synchronously recalculate declarations, retain reusable content findings, immediately remove old `patch.no-match` for newly enabled targets. Later log reads / full analysis use independent sessions / existing analysis subprocess, revalidate inputs before publish; stale work never overwrites edits. Content analysis still uses enabled-set slots, per-Mod content-keyed Def / assembly facts; patch simulation depends on order / whole document, still recomputed in background.
