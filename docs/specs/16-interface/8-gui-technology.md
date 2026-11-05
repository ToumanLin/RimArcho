# 16 · Interface · GUI technology (§8)

> Part of [16 · Interface](README.md). Section numbers run through all of 16 and stay unchanged when split across files.

## 8. GUI technology

### 8.1 Delivery

The same source tree is linked twice:

| File | Subsystem | Double-click | Purpose |
| --- | --- | --- | --- |
| `archo.exe` | Console | TUI in the default terminal (§9.2) | CLI, `archo mcp`, and `archo tui`; `archo gui` starts the GUI |
| `RimArcho.exe` | Windows GUI (`-H windowsgui`, no console window) | GUI | The player's double-click entry, accepting `--env <name>`; command arguments run as the CLI for GUI scan and analysis subprocesses |

- Both executables share the data directory and database and may run together; SQLite WAL supports multiple processes. When the GUI gains focus, it compares `Stamp()` (the database state, `ChangeKey`) and reloads if it changed. An edit from a stale view never overwrites the other process's changes (G8).
- If an adjacent `RimArcho.exe` exists, `archo gui` starts it and immediately returns. Otherwise, it runs the GUI in the current process.
- When opening the database runs upgrade steps for longer than 300 ms, `RimArcho.exe` shows a small progress window, which cannot be closed, before the main window; the TUI and CLI write the progress to stderr (04 §4).
- Releases contain both executables; see 18 for their sizes.

### 8.2 Frontend

- The GUI uses Wails v3, at the beta version pinned in 17 §7.1. CGO is not required (`CGO_ENABLED=0`).
- The frontend uses plain HTML, CSS, and native JavaScript ES modules. **It has no framework, npm dependency, or build step.** The files are in `internal/surface/gui/assets/`: `index.html`, `style.css`, and `js/`, with one module per area (for example `api.js`, `state.js`, `shell.js`, `lists.js`, `screens.js`, `fulldetail.js`, and `main.js`). They are embedded in the executable with `go:embed`.
- To call Go, the handwritten `api.js` uses `import { Call } from "/wails/runtime.js"` and `Call.ByName`. A `Proxy` assembles the full method name, `github.com/ToumanLin/RimArcho/internal/surface/gui.API.<method>`. There are no `wails3`-generated bindings; building requires only `go build`.
- In `internal/surface/gui`, `api.go` provides the Wails service, whose methods only call view; `gui.go` owns the window, the `/preview/` and `/img/` local image handlers, the CSP, and the icon routes; `exchange.go` provides the import and export file dialogs; `Init` sends the interface text (`view.Table`).
- Go sends Wails events to the frontend: `scan` for scan completion, `analyze` for analysis progress and completion, and `pack` for build progress. The frontend polls `Live` for game runtime status (§2.16.1 G01).
- Virtual scrolling creates only visible rows.
- **Page reads** follow §3.1 rule 9, entering before reading. **Every page** uses `beginPageRead` to navigate and show a skeleton before waiting for data: Launch, Can it run, updates, packs and the build preview, settings, game, guide, uninstall preview, Mod library, editor, game log, texture optimisation, All lists, background tasks, and full-screen details. A same-page refresh keeps the displayed content. Switching Mods or packs shows corresponding placeholders so the old object's actions cannot still be clicked. Each read validates the page code, request sequence, environment, and plan. A late success or error cannot overwrite a new page. Repeated requests on the same page accept only the latest result; a failure ends loading and allows a retry. Delayed Workshop descriptions in full-screen details follow the same checks. See §2.16.1 for polling, read retries, detail and subscription-detection debouncing, and idle-session release.
- **Preview action state:** changing the pack type or texture choices rereads the preview. While it is pending, even a shortcut cannot start a build from the old preview. Re-estimating an uninstall selection removes the old confirmation buttons; the next actions appear only after the complete result arrives.
- The first stylesheet rule is `[hidden]{display:none !important}`. Without it, an overlay container's `display:flex` overrides hidden and leaves an invisible mask that intercepts every click.
- WebView2 stores user data in `<data directory>/webview2`, not `%APPDATA%\RimArcho.exe`.
- The development-only `spikes/guidev` serves pages through the same API on 127.0.0.1 for browser debugging. With `-assets <directory>`, it reads page files from disk so refreshing shows edits. It is not shipped.
- `spikes/guidev`'s `/dev/pagecheck` uses mocked APIs to check immediate page feedback, late results and errors, reentry, and retries after failure. It also covers variable-height large logs and keyboard positioning. It uses the real frontend modules without connecting to the product database.

**The Windows main window** is borderless and uses WebView2 composition hosting while retaining Wails' default window decorations and resizing support. `window-chrome.js` provides minimize, maximize/restore, and close controls. Browser debug pages and other platforms retain their existing window form without these buttons. Blank space in the topmost navigation row drags the window, and double-clicking maximizes or restores it. Buttons, links, inputs, and splitters do not drag the window. The maximize button uses Wails' native hit region for Win11 Snap Layouts. Wails and Windows handle edge resizing, Aero Snap, the maximized work area, shadows, and corners; these require VM verification. Close must call `Window.Close()` to trigger the cancellable `WindowClosing`, sharing the close interception used by Alt+F4 and the taskbar. It must not call the application quit interface. Buttons follow the page's light or dark theme; see §2.2 for their placement.

Window-control initialization allows Wails system information to arrive after the page. Once Windows is confirmed asynchronously, the buttons are added in the current navigation position. First paint does not wait for that information.

### 8.3 Security

- The page's CSP is `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`. Wails works under `script-src 'self'` without `unsafe-eval`.
- Workshop descriptions and images are external content. Go parses descriptions into BBCode blocks, and the frontend builds elements using `textContent`; it **never inserts external text as HTML**. Images are read only from the local cache through `/img/`, never directly from remote URLs.
- Links open in the system browser. `OpenURL` refuses non-http(s) URLs.
- On Windows, Wails intercepts WebView2 resources and opens no TCP port.
- Settings runs only commands allowed by `view.SettingCommand` (§2.14).

### 8.4 Long tasks

- In the unified command panel, `RunCLI` immediately returns the task state, and `CommandStatus` reads the latest result. The worker in `command.go` owns an independent App session; closing the window cancels it and waits for release. Ordinary commands use `cmds.Check` and `ExecuteContext`. Scanning and analysis use `ChildCommandContext` to start subprocesses without a console, preserving the original text and exit code. Arguments are passed directly without a shell. Completion, history, and parsing share `internal/cmdline` (§3.5).
- Scanning and analysis run in subprocesses: `archo scan`, `archo analyze`, and `app.ChildScan` and `ChildAnalyze`, with `CREATE_NO_WINDOW`. Events push progress to the status line without blocking the interface. The TUI does the same, including `:scan`.
- At startup, an existing index lets the list appear immediately, followed by an incremental background scan. If no scan has ever run, scanning starts automatically.
- When adding a game (§2.15, §2.8), scanning and the first analysis run in subprocesses so the guide and other pages remain usable. The new game's pages show skeletons until its data is ready. Go GUI methods do not perform inline work likely to exceed about 100ms; longer work uses goroutines or subprocesses and returns results through events (§3.1 rule 9).
- `Workspace` reads the list, sidebar, and history together and returns them through events. An unchanged needs key reuses the full workspace snapshot. A rebuild reuses a separate read session, shared index (17 §7.3), and editor-row cache, building within a short-lived WAL read-only snapshot. It does not load the entire library while holding the page lock. The sidebar shares the same needs result. Publication checks the environment, request sequence, list edit generation, and input key.
- Pack builds run in the background, push progress to the build page, and can be stopped (§2.6).
- All these operations, plus library file and Steam actions, are recorded as background tasks (§2.16, `app.Tasks`). Running, queued, and past work appears in one place, with results persisted to `<data directory>/tasks.json`.

### 8.5 Mouse

In the plan editor, mouse actions are another way to trigger the same verbs:

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

- **Drag rules:** dragging from Disabled or Staging onto an Enabled row places the Mod at that row's number. Dropping below the last row appends it; dragging within Enabled moves it. Dragging to Staging stages it; dragging from Enabled to Disabled disables it; dragging from Staging to Disabled removes it from staging without enabling it (`view.Unstage`). Pressing and releasing on the same row is a click. The status line describes the drop target during the drag; `esc` cancels. Releasing applies the change, and `u` undoes it. Dragging runs on mouse events in `lists.js`, not HTML drag and drop: under WebView2 composition hosting (§8.2) the window has no OLE drop target, so a native drag starts but never drops. A press becomes a drag after 5px; the pointer within 28px of a list's top or bottom scrolls it; leaving the window cancels.
- **The context menu** is shared by the editor and the Mod library. It uses the same action table and group order, shows each key on the right, and lists only applicable actions (`view.RowOps` and `view.CopyOps`, with names and keys from `view.OpTable`):
  1. Plan actions appear only in the editor; the Mod library omits the whole group: `space` to enable, disable, or put back, then `m`, `t`, `x`, and disable in every list.
  2. Organize includes tags `g` and note `n`.
  3. Open includes the folder `ctrl+o` and Workshop page `ctrl+w`.
  4. Versions includes lock this version, update to the Workshop version, unlock, and move into the lock library.
  5. Disk includes unhide, remove the converted textures, resubscribe, and the subscription actions, with red unsubscribe / move to the recycle bin (`Del`) last. The Mod library offers no single-Mod DDS conversion. The resubscribe confirmation explains that RimArcho first moves the folder to the recycle bin, then Steam downloads it again. See §2.9 for the conditions and 11 §2 for cleanup verification and Steam calls.

  A thin line separates groups. The Menu key or `shift+F10` opens the menu on the cursor row with the first item focused; on a list's sidebar entry or breadcrumb tab they open that list's menu, as right-click does. The `↑↓` keys or `tab` move between items, `enter` executes, and `esc` closes. The displayed keys also execute directly. With multiple selections, only actions that support a batch appear; folder, page, note, position, and switching copy require a single item. `esc` and running an item put the focus back where the menu opened, also after right-click: the row, the entry or the button; a column header's menu goes back to the table's rows. For agents (§11 A7) every menu built this way is a menu named by what it acts on (the Mod, list, column, sidebar entry, task or log file), its items are menu items and the column toggles are checked or not.
- Arrow keys, the wheel, and a single click never change data (§3.1 rule 1). Only double-clicks, drags, buttons, and verb keys do.
- **GUI and TUI differences:** page history and `alt+←` / `alt+→` are GUI-only. The TUI keeps its existing page switching and technical-page stack return and does not handle mouse side buttons.
- In the TUI, clicking moves the cursor, `ctrl+click` adds a selection, `shift+click` selects a range, and dragging the blank column gap resizes columns. Clicking the detail area focuses it; the wheel scrolls details. The TUI supports neither dragging Mods nor rectangle selection.

### 8.6 Icons and Windows resources

- **The artwork** is `internal/icon/portrait.svg`, a 1024×1024 vector on a rounded dark `#1B1D23` background. The radius is 0.22 of the side length. Sizes of 48px and above show the full image; sizes of 40px and below show only the head, including hair and face, in viewbox x 120–910, y 170–960. This remains recognizable on the taskbar. A face-only crop is too tight and becomes a patch of skin at small sizes.
- **The renderer** is the small dependency-free SVG renderer in `internal/icon/svgr.go`. It draws the element tree, supporting `<g>` inheritance of fill and stroke; matrix, translate, scale, rotate, and skewX/Y transforms; paths, plain and rounded rectangles, circles, ellipses, lines, polylines, and polygons; nonzero and evenodd fills; rounded stroke joins; solid colors and linear and radial gradients in user or bounding-box coordinates, including gradientTransform; opacity, fill-opacity, and stroke-opacity; and clip-path. Filters support only feGaussianBlur. Content inside defs, clipPath, gradients, and filters is drawn only when referenced. The renderer uses 4× supersampling; all sizes through 256px take about 1 second.
- **Binaries are not committed** to the public repository. `icon.PNG(size)` uses antialiased per-pixel supersampling for the window icon and resources. `icon.SVG(size)` provides the same vector composition; it generates the page favicons `/icon.svg` (256) and `/icon-small.svg` (16).
- **The window icon** comes first from the executable's icon group ID 3, with the supplied PNG as fallback. Release builds therefore do no rendering. A development build without `.syso` renders one icon at the system large-icon size (`SM_CXICON`, 32px at 100% scaling), then immediately calls `FreeOSMemory`.
- The release script calls `go run ./scripts/winres` to write `cmd/archo/rsrc_windows_amd64.syso`, a COFF resource object containing the icons and version information. The `.syso` is excluded by `.gitignore` and is not committed.
- **Icon group ID 3** is the group Wails reads for the window. Explorer reads the first group, which is the same group. It contains one PNG at each size: 16, 20, 24, 32, 40, 48, 64, and 256.
- **Version information** sets `FileVersion` and `ProductVersion` to the release version and `ProductName` to RimArcho. `FileDescription` is RimArcho command line for `archo.exe` and RimArcho for `RimArcho.exe`. Both executables are built from the same package; the release script writes a separate `.syso` between the two builds.
- **Without resources**, development builds and `spikes/guidev` use the PNG in `application.Options.Icon` (256 px) as fallback.

### 8.7 List findings and needs cache API

The editor and Can it run read the same backend snapshot. The entry points are `API.Needs`, `API.ListProblems`, and `Board.problems`. The structures are defined in [`view.Needs`](../../../internal/view/needs.go) and [`view.ListProblems`](../../../internal/view/listproblems.go); the cache is implemented in [`gui/needscache.go`](../../../internal/surface/gui/needscache.go).

**Calls occur at the following points:**

- The Workspace `environment` event already includes `board.problems`, which is used directly for first paint.
- Opening Can it run calls `Needs()`. Reading only the editor's findings calls `ListProblems()`. Both use the same cache.
- Edits returning `view.Result`, such as `Toggle`, `Stage`, `Unstage`, `MoveBy`, `Place`, and `StageAll`, include an updated summary in `result.problems`. String-returning operations such as `Undo`, `Redo`, and `DoAction` publish the same structure through `list-problems`.
- Completing background log or content analysis also emits `list-problems`; the `analyze` progress and completion events remain. No extra request is needed just to refresh markers. If Can it run is open, it redraws with `Needs()` on the event and preserves the cursor. It also rereads when the game exits (`Live` changes from running to stopped), so the completed analysis replaces Current run.

The main `ListProblems` JSON fields are:

| Field | Purpose |
| --- | --- |
| `environment`, `plan` | The environment ID and open-list ID filter late events from before navigation; Launch still reads the current list. |
| `listVersion` | The fingerprint of the list content used for this result. |
| `revision` | The cache input fingerprint; identical inputs are reused. Do not compare it lexicographically. |
| `sequence` | An increasing publication number within the API session; for the same environment and list, accept only a sequence at least as large as the displayed one. |
| `updating`, `error?` | While background work completes, retain the previous content findings with `updating=true`. On failure, retain the findings and return the error text. |
| `count`, `severity` | The shared needs count and maximum severity, `error` / `warning` / `info`; hidden maintenance and muted information items follow needs counting rules. |
| `mods` | An Instance-ID index whose values contain `count`, `severity`, `summary`, and `problemIds`. An absent Mod has no displayable finding. |
| `items` | The full list-level `Need` array, with each finding sent once and looked up through `problemIds`. |
| `status`, `reasons` | Row icons and per-Mod explanations from the same snapshot, keyed by Instance ID. Replace these directly on an event; do not rebuild them from `items` aggregate titles or actions (version warnings: §5). |

For `version-mute` / `version-unmute`, `refs` contains package references and `gameVersion` is the target major.minor version. The `version-list` action carries only `gameVersion` and `versionMuted`. Opening it reads `API.VersionWarnings` → `view.ReadVersionWarnings`, which returns the title, all actions, groups, and their `{id, name, supported, action}` rows. These rows are excluded from normal Board and Needs responses. Individual, group, and all-item operations still use `DoAction(action, 0)`; batch ignore follows `confirm` and `warn`. See §5 for the interaction.

`Need` adds `id`, optional `target`, and `requiredBy` to its original fields. A dependency's `target` is the packageId after trimming whitespace, normalizing case, removing `_steam`, and resolving directory aliases through the registry. The `requiredBy` field is an array of `{id, name}`. Declared dependencies, corpus-inferred missing dependencies, and patches with disabled targets can merge by target; every requiring Mod receives ownership of the finding. If an existing uninstalled list entry covers the dependency, ownership attaches to that entry's aggregate item. Strong ordering findings still merge into one automatic-sort item; individual strong constraints also have a `move` action.

**To execute an action**, pass the full Action object from `item.actions` to `DoAction(action, option)`. The `id` identifies the action; execution arguments remain in the original `kind`, `refs`, `after`, `options`, and other fields. Passing only the ID is insufficient. For `choose-disable`, `option` is the zero-based option index; other data actions use `0`. The `enable` action positions dependencies by constraints, `move` uses one `refs` and `after`, `sort` sorts directly, and `subscribe` enters the existing task queue. DLC store links use `open-url`. Interface actions such as `view`, `list`, `fold`, and `save` are still dispatched by the surface, and confirmations still follow `confirm` and `warn`.

**Using installed copies:** `API.UseInstalled(packageIDs []string) (*view.Result, error)` binds missing entries in the open list, preserves their positions, and is undoable as a batch. See 07 §4.2 for semantics and refusal rules. Its argument is a packageId array accepting case differences, surrounding whitespace, and `_steam` suffixes, rather than Instance references. An empty array changes nothing. The result's `text`, `undo`, and `problems` follow the edit contract above. Can it run separates `MissingMods.CanEnable` entries as can use installed copies. The primary action has `kind: "use-installed"`, a packageId array in `refs`, `undo: true`, `key: "space"`, and `label`, `done`, and action `id`, without `confirm` or `warn`. Passing the full action to `DoAction(action, 0)` returns a string and publishes the problems event; passing `refs` to `UseInstalled` returns an edit result. Only entries without an available copy retain subscribe and remove actions. The sort Outcome's `missingMods` contract is unchanged; the frontend offers repair through `canEnable`.

**Cache bounds:** each API session retains at most eight slots keyed by environment, list, game version, and language. Inputs also include list content, the database change key, scan snapshot and analyzer version (`AnalysisKey`), log path, size and mtime, and freshly checked directory and game-link states. Workspace reuses the full Board, Shell, and History. Rebuilding uses a separate connection without holding the main lock.

List edits use `App.IncrementalProblems` to synchronously recalculate the declaration layer while retaining reusable findings from the previous content layer. Old `patch.no-match` hints for newly enabled targets are removed immediately. Subsequent log reads and full analysis run in independent sessions or the existing analysis subprocess. Inputs are rechecked before publication, so stale work cannot overwrite newer edits. Content analysis still uses slots saved by enabled set and Def and assembly facts saved by each Mod's content key. Patch simulation depends on order and the whole document and is still recalculated in the background.

### 8.8 Steam credit API

The Wails methods, JSON, task and events behind the Steam credit prompt and the gradual replacement task (§2.16) are in the [Steam credit GUI API contract](../../api/steam-credit.md); the batch rules are in 11 §1.
