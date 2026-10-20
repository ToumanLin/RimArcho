# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## 0.8.0 — toward 1.0 (unreleased)

Spec: `docs/specs/10-1.0实现.md`.

### Hidden and pinned mods (10 §2)
- Folders another manager hid by renaming About.xml are classified: **pinned** (a loadable copy
  of the lineage is what the game uses; a warning once the workshop copy changed since, with
  `archo mod <copy> --diff <workshop>`), **left over** (unsubscribed) or **hidden** on their own.
- 需要处理 gets a "hidden mods" group: keep hidden, show again (`archo unhide`), use the workshop
  version (`archo unpin`), move to the recycle bin (`archo trash`). Trash never deletes
  permanently and only takes hidden or unsubscribed workshop/SteamCMD folders.

### Live game log (10 §3)
- `archo log --follow [--once] [--json]` follows Player.log while the game runs: only new bytes
  are read, clusters instead of text are kept, attribution is the full-log engine's.
- RimCrow's companion log (`RimCrow_Realtime.log`) adds timestamps, levels and related mods.
- The window says "game running since … · n errors" and puts this run's errors first in
  需要处理; `archo status` shows a running game.

### Lists from other tools (10 §4)
- `loadout import` reads RimSort (XML, JSON, clipboard, Rentry), RimWorld's `.rml`, RimCrow's
  ModList.xml, JSON and share codes (also pasted as the argument), and workshop-ID lists.
- `loadout export --format rimsort|rml|rimcrow`.
- `archo collection <link|ID> [--import]` compares a Steam Workshop collection with what is
  installed and can load it into the workspace.

### Community data (10 §5)
- `archo data update` / `archo data status`: RimSort's rules and Workshop database, Use This
  Instead's replacements. All-or-nothing; the only command that fetches them.
- Missing mods get direct Workshop links; enabled mods with a maintained replacement are listed.

### Packs (10 §8)
- `archo pack build|list|launch|export|import|remove`: compile the current list into a pack
  (checked, sorted for the pack only, each mod copied without other game versions' folders),
  play it with its own list and saves through links in `Mods/`, share it as a zip or a list.
- The window has a Packs screen: the packs with when they were last played, Play, and build
  from the current list with a preview.

### Settings (10 §7)
- A settings screen (gear in the top bar): environment folders, launch method and arguments,
  language, Workshop descriptions, community data sources, database backup and compaction,
  and each cache with its size and a clear button.
- `archo db status|backup|vacuum`, `archo cache status|clear <kind>`, `env set --launch-args`.

### History (10 §6)
- `archo history`, `archo undo <n>`, `archo redo <n>`. The window shows the history in the empty
  staging list: redo steps, now, undo steps in plain words, then the saved versions.

### Other
- The window's look follows magpie (MIT): cards with hairline borders and 12px corners, a
  centred view switch with a raised thumb, icon and text buttons, frosted popovers.
- The top bar: the environment on the left, the pages as tabs in the middle (Mod 列表 · 需要处理
  · 日志 · 整合包 · 设置, `shift+1`–`5` by physical key), rescan, sort, save and launch on the
  right; no name or icon (the window's title bar has them). Switching pages no longer shows the
  mod list in between. The position line moved to the bottom bar. Tags are `g` in the window
  (`#` still works where it is not shift+3).
- 日志 (`l`, `c`): the game log (its error clusters with the mods they are tied to, a cluster's
  evidence beside it, Player.log or Player-prev.log), mod changes, and the manager's actions
  (unsaved steps and saved versions). A log of another game version is analysed in the
  background instead of holding the window for seconds.
- 需要处理 is three columns (the list · the game · your call); ←→ between them. Moving the
  cursor no longer rebuilds the page.
- `T` in the window keeps only Core and the DLCs enabled, in one undo step (also in the
  palette of both interfaces; put-all-back from staging stays in the palette).
- The Enabled list's position column is as wide as its longest number.
- About.xml, LoadFolders.xml and Defs saved as UTF-16 are read (10 §1).

## 0.7.0 — the window (unreleased)

### Icon (08 §15)
- The author's portrait artwork (`internal/icon/portrait.svg`) on a dark rounded base; below
  48 px only the face is shown so it still reads in the taskbar. (It replaces the keystone arch
  drawn earlier the same day.)
- Rendered by a small built-in SVG renderer, so the repository holds no image files apart from
  the SVG source and the generated `docs/assets/rimarcho.svg`.
- Both exes carry the icon and Windows version information (FileVersion, description). The
  release script writes the resource object with `go run ./scripts/winres`; no resource compiler is
  needed. The window, its title bar and the page header use the same icon.

### Link deployment (09 §8)
- Mods in SteamCMD and extra folders can be enabled. `save` links each one into `Mods/` as
  `_Archo_<source>_<folder>` before it writes ModsConfig.xml, and removes the link once the mod is
  disabled. `save --dry-run` and the save preview list the links. Windows uses junctions.
- The deployed copy registers like any `Mods/` folder: a `Mods/` mod with the same packageId wins,
  and next to a workshop copy it can only be swapped in.
- Only links with the `_Archo_` prefix are touched; a real folder with that name stops the save.

### GUI (08)
- `RimArcho.exe`: a desktop window (Wails v3 and plain HTML/CSS/JavaScript, no frontend build
  step) next to the terminal UI. A double click opens it; `archo gui` opens it from a terminal.
- The window and the TUI share one layer, `internal/view`: the columns, the search syntax, the
  details, 需要处理 with its actions, the changes, tags, the verbs and the position input, the
  palette and the interface text. Neither interface builds these itself.
- The window follows the TUI's v3 rules (06 §13) and adds the mouse: click and modifier clicks,
  double-click, drag and drop between and within the lists, a context menu, resizable columns.
- Workshop descriptions are built from parsed BBCode blocks and never inserted as HTML; images
  come only from RimArcho's cache. The page loads nothing remote and the window opens no port.
- Children of the window (scan, analysis) open no console window.
- Typed app calls for tags, alias and note (`SetTags`, `SetAlias`, `SetNote`).

## 0.5.0 — MVP (unreleased)

### M5: local copies, RimCrow import, exchange, polish
- `copy-local <ref> [--name] [--git]` copies a mod into `Mods/`, writes `.rimarcho/origin.json`
  and can add a git baseline commit. It never overwrites a folder, never copies the upstream `.git`,
  never follows links that leave the mod folder, and does not change the Loadout. The MCP tool is
  `copy_local`.
- Player data on lineages: `alias`, `note`, `tag add|remove|list`. The alias is shown as the mod's
  name; `list --filter tag:<t>` filters by tag.
- `import rimcrow --db <mod_manager.db> [--dry-run]` imports aliases, notes, groups (as tags),
  interlocks (as attach rules) and user rules. It reads copies only and can be run again safely.
  The report also lists the about-disabled mods, with guidance.
- `loadout export [--format text|modsconfig] [--out]` and `loadout import <file>` (a text list,
  a ModsConfig.xml or a save's mod list).
- TUI Changes view: identity questions pinned on top (same / copy / unrelated), load or ignore an
  external ModsConfig.xml change, events grouped by scan and type, and file-level diffs on `enter`.
- Mouse support in the TUI: the wheel scrolls and a click selects. `config set ui.mouse off`
  turns it off.
- `archo version` / `--version`; `archo <command> --help`.
- `scripts/release.ps1` builds one `archo.exe` with the version stamped in through ldflags.
- M4 rulings:
  - `harmony.skip-prefix` is info and becomes a warning only when the method is in a stack of the
    newest log.
  - The newest Player.log is included in `problems` automatically.
  - A log from another game version is attributed against that version.
  - `source search` needs a scope (`--in`, `--implicated`, or `--active --yes`, which shows an
    estimate first).

### M4: Harmony, logs, decompiling, MCP
- Harmony patches are read from mod assemblies: prefixes, postfixes, transpilers, finalizers,
  bool prefixes, manual `Harmony.Patch` and Harmony IDs. New findings: `harmony.multi-transpiler`,
  `harmony.skip-prefix`, `harmony.shared-target` and `harmony.manual`. New command: `harmony`.
- `log`: Player.log clusters, each attributed to mods as source / patcher / dependency / mentioned,
  with evidence. Patch failures are matched to the simulation, and missing cross-references list
  their referrers and providers.
- `source` / `source search`: on-demand ilspycmd, cached per DLL hash.
- `archo mcp`: 15 tools over stdio whose text is identical to the CLI, with readOnly/destructive
  annotations and descriptions that cover the diagnosis workflow.
- `--game-version X.Y` on the analysis commands; `version.unsupported` is folded into one row;
  noisy info is hidden by default.

### M3: problems
- The declaration layer (dependencies, incompatibilities, order, duplicates, versions) and the Def
  layer (inheritance after patching, overrides, abstract collisions).
- A PatchOperation simulator with no-match reasons (`target-inactive`, `target-missing`,
  `target-later`, `wrong-version-route`, `stale-name`, `unguarded-optional`, `node-absent`).
- Commands `problems`, `def`, `patches`, `why` and `files search`; TUI Problems view and Inspect.

### M2: Loadout
- A workspace Loadout with history, undo/redo, versions, tags, diff and checkout; `save` takes a
  backup and replaces ModsConfig.xml atomically.
- Constraint sort with explanations, user rules, and `launch`. TUI list view.

### M1: fact base
- Environment detection, sources (official, local, Workshop, SteamCMD, extra), incremental scans,
  identity reconciliation (lineages, copies, moves, tombstones, questions) and change events.
- Commands `env`, `scan`, `status`, `list`, `find`, `mod` and `changes`.
