# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## 0.9.0 — plans and launching (unreleased)

The new design's 1.0 work (01 §5.1). 1.0.0 follows once 01 §5.2 is verified, with a
`1.0.0-rc1` first.

### Breaking
- Database schema v10 → v12: plans (v11) and analysis kept per plan (v12). Each environment's workspace, undo stack, versions and
  staging become its first plan, `default`; each `loadout tag` name becomes a plan of its own
  with one `import` version (04 §1.4).
- `launch --force` is removed: launching always writes the plan first (08 §2).
- `loadout tag` is removed: a named list is a plan (05 §4.3).
- `archo save` means "write to the game without launching" (04 §4.1).
- `save` and `launch` refuse while ModsConfig.xml holds an outside change that was not loaded
  (04 §4.1, §4.3).
- Locked copies made by `pin` live in the lock library, not in `Mods/` (04 §7).

### Archo Monitor (03 §6.7, 04 §1.5, 08 §9)
- RimArcho ships its own in-game companion mod, Archo Monitor (`toumanlin.archomonitor`,
  1.5 / 1.6 / 1.7). It writes `ArchoMonitor.log` in the save data folder: the XML file and def
  being loaded at an error, the mods on the stack and their Harmony patches, and errors after
  the game stops logging; its JSON lines are a superset of RimCrow Companion's.
- It is installed to `%LOCALAPPDATA%\RimArcho\monitor\ArchoMonitor-<version>` (`monitor.dir`),
  registered as `src:monitor` and linked into `Mods/` as `_Archo_monitor_…`. New plans, and
  every existing plan once, get it right after Harmony as one undoable edit;
  `config set monitor.default off` turns that off. Newer versions replace older ones in plans.
- `archo monitor`, `archo monitor install [--dry-run]`; `status` and `scan` report it; the
  settings page has a 游戏日志 group.
- The log analysis and the live tracker read `ArchoMonitor.log` (and RimCrow's
  `RimCrow_Realtime.log` / `RMM_Realtime.log`): files become source evidence and the 日志 chip
  in 能不能跑, counts and times come from it, errors only it saw become clusters.
- The 游戏日志 page shows the run's companion log as its own section, "Archo Monitor · 本次运行":
  one row per entry (time, level, kind such as 补丁错误 / 程序集冲突, first line, ×count, related
  mods by name and files), details on click, a link to the matching Player.log cluster, and
  the session's version, game and start–end as its header. It follows the live tracker while
  the game runs. The file picker lists ArchoMonitor.log under its Player.log. A plan without
  the monitor gets one line with 加入 Archo Monitor (one undoable step) (06 §2.10).
- Log lines on that page take RimWorld's own debug-log colours (error #ff6666, warning
  #ffeb04, message white; darkened for the light theme with the hue kept), and mods' Unity
  rich text (`<color>`, `<b>`, `<i>`) is shown as styled text, parsed in Go; other tags stay
  plain text. The first visit to the page no longer stays blank when the log reads quickly.
- The release zip has `monitor/ArchoMonitor/`; building it needs the .NET SDK 10.

### Plans (04 §1)
- `plan list|new <name> [--from current|v<n>|<file>]|use|rename|remove --yes`;
  `loadout import <file> --new [<name>]`. The last plan cannot be removed; names ignore case.
- Workspace, undo, versions (numbered per plan) and staging belong to a plan; `status`,
  `loadout log` and `stage list` name it. Share codes carry the plan name.
- The ModsConfig.xml write record is kept per user-data folder and names the writer (plan,
  version).
- The window: one sidebar entry per plan and + 新方案; new (empty, copy, a version, an
  import), rename, delete (confirmed; never the last) from the plan page's ⋯ menu, a
  right-click, `P` or the palette. The status bar says "游戏里现在是：<方案> v<n>". The TUI
  has the same switcher on `P` and names the plan in its top bar.

### Outside changes (04 §4.3)
- A change made to ModsConfig.xml outside RimArcho goes to the plan that last wrote the file,
  in whichever environment shares the folder: loaded as an `external` version when that plan
  is untouched since, else recorded and marked. With no record it goes to the current plan.
- `archo loadout ignore-external` clears the mark; the version stays. The mark shows in
  `status`, `loadout log`, 能不能跑 (with "see the difference"), the sidebar and the plan page.

### Launching (08 §1, §2, §4)
- `archo launch` writes the plan to the game (links, ModsConfig.xml, a `launch` version unless
  the list equals the newest one) and then starts it; Steam or the game exe is found before
  anything is written.
- `launch --dry-run` and `save --dry-run` show one preview: links, path, version rewrite,
  entries and missing, plus the command line for launch.
- `archo save` works while the game runs and says the game reads it on its next start.
- One game at a time: a game running from any registered environment blocks `launch` and
  `pack launch`, naming the environment and the plan or pack. The window disables launch.
- Launch records carry the plan, the enabled list's fingerprint and the version played.
  A game in `RimWorld2` no longer counts as running in `RimWorld`.
- Launch, write to the game and sort act on one click or key in the window and the TUI; the
  status line says what happened (sort: 撤销 / u). A refusal names its reason and lists the
  commands that resolve it. Loading a history version or an outside change is one undoable
  step, with no confirmation. The palette's 预览排序 / 预览写入 / 预览启动 show the dry runs
  (06 §6).

### Locked versions (04 §7)
- `archo pin <instance> [--name] [--git] [--dry-run]` (`copy-local` is an alias) copies the
  mod into the lock library (`lock.dir`, default `%LOCALAPPDATA%\RimArcho\locked`, one folder
  per environment, scanned as source `src:locked`) and swaps it into the current plan. The game
  sees a copy only through an `_Archo_` link, so several copies coexist and each plan uses
  its own; no About.xml is renamed.
- `mod.pinned` covers RimArcho's copies too, and turns into "上游有更新" when the upstream
  differs from `origin_snapshot`.
- `pin --update <instance>` makes a fresh copy and swaps it in; the old one stays.
  `pin --move-to-library` (and the first `--update`) moves an older RimArcho copy out of
  `Mods/` into the library — a rename on the same volume, else copy, verify, recycle.
- `unpin` on a RimArcho copy swaps the upstream back into the current plan.

### Logs (03 §3, §6.4)
- A log belongs to the newest launch before it was written (else the plan that last wrote
  ModsConfig.xml; else "source unknown"). It is stale for another game version, another
  plan, environment or pack, or a list changed since that launch; the header says why.
  ModsConfig.xml's modification time is no longer used.
- Analysis is kept per plan's enabled set: switching plans and back does not re-run it.
- "分析本次日志" shows in the sidebar foot and the TUI top bar when Player.log is newer than
  the last launch and the log last seen.

### Search (06 §3.7)
- `tag:` matches the start of any word in a tag: `tag:facial` finds Facial Hair and Facial
  Animation, `tag:animation` finds Facial Animation.
- From the second letter, grey ghost text completes field names (`au` → `author:`) and
  values after `tag:`, `type:` and `has:`; Tab (or → in the GUI) takes it, otherwise Tab
  still jumps to the next list with matches.

### MCP (05 §1.2, §5.2)
- New tools: `manage_packs`, `ignore_problem`, `resolve_identity`, `scan`, `exchange_list`,
  `manage_rules` (`add_rule` stays as an alias), `manage_hidden`, `maintenance`, `pin_mod`
  (`copy_local` stays as an alias), `unpin_mod`; `status` gains `show` pages (envs,
  settings, data, db, cache); `launch_game`, `manage_plans`, `plan_history`;
  `edit_loadout` gains `checkout` and `ignore_external`; `list_library`, `manage_library`. 33 tools in all.
- `exchange_list` and `maintenance` are open-world; the destructive ones (`save_loadout`,
  `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`,
  `manage_library`, `maintenance`) are listed in the server instructions with "show the dry run and ask first".
- Instructions and descriptions speak of the current plan, writing to the game and locked
  versions.

### Mod 库存 (06 §2.9)
- `archo library` lists every mod on this machine, one line per Lineage (copies, size,
  the most important state, the plans using it), with `--search`, `--include` /
  `--exclude key:value` (tag, source, version, state, used), `--sort name|size|changed|played`,
  `--facets` and `--lineage <ref>` for every copy's path, states, plans and last launch.
  "Used in" covers every environment of the same game and the packs; states include
  subscribed items with no folder, unsubscribed leftovers, copies the game ignores and
  locked versions whose upstream changed.
- `archo library move-to-locked <ref> [--dry-run]` moves a spare copy out of `Mods/` into the
  lock library; `archo library remove-from-plans <ref…>` takes mods out of every plan (one
  undo step each). Moving now keeps the instance (`pin --move-to-library` too), so every plan
  keeps it. `trash` refuses while any plan uses the folder and names the plans.
- MCP: `list_library` (read-only) and `manage_library` (destructive).
- The window's Mod 库存 › 全部: filters, the Lineage list with size · state · used-in, a
  copy's details in the right column, a selection action bar; the TUI has a minimal list in
  the palette.

### The window (06 §2)
- A sidebar replaces the top tabs, in two groups: 在玩 (启动, 能不能跑 with its badge,
  游戏日志) and 库 (Mod 库存 › 变化, 方案, 整合包, 环境), then 设置 (`shift+1`–`7`).
  Only 能不能跑 shows a number, and it leaves out hidden mods. Drag the sidebar's right
  edge to change its width (double-click resets it).
  It collapses, and starts collapsed below 760 px.
- The top bar has a clickable breadcrumb, the environment chip and the page's actions.
- A home page: each plan and pack, newest played first, with its state (another plan's from
  its stored analysis) and a launch button, and the last run and last scan's changes.
- A first-run guide: find games (several at once), scan them, start from the game's list, an
  import, a pack or a Steam collection. 环境 › 添加环境 opens it.
- The window follows the new design: no page title under the breadcrumb (it names the page)
  but one muted line saying what the page is for; the environment chip sits by the page name
  (plan and environment on 能不能跑); page actions are quiet icon + text buttons. Home rows
  have a fixed environment column, a 方案 / 整合包 tag, a meta line (mods, follows Workshop
  updates / versions locked, last played) and a state chip; the activity is a row of cards.
  The sidebar shows quiet counts (plans, packs, environments, the library).
- 能不能跑 is one column of four sections by consequence (会让游戏出错, 上次运行的报错,
  要你确认, 隐藏的 Mod). Each item is a card: a bold line, a plain sentence, an evidence
  chip (Mod 自己声明 / 推断 / 日志 / 社区数据 and the source) and one primary action, the
  rest under ⋯; ← → jump between sections. The view model carries `Section`, `Title`,
  `Why` and `Evidence` for both interfaces. A mod's missing inherited definitions, and its
  patches that miss the same disabled target, are now one item each (the count drops).
- Motion (06 §7.1): three duration tokens (120 / 180 / 260 ms); pages fade in with a 4px
  rise; the sidebar's highlight slides; list edits slide the visible rows to their new
  place and flash the changed ones; badges and counts bump; the status line slides in;
  lazily loaded parts shimmer. Only opacity and transform move, and nothing does under
  prefers-reduced-motion.
- Polish after a side-by-side with magpie: the sidebar takes magpie's sizes and edges (212 px,
  30 px entries, 14 px icons, small group titles), the open entry is raised, sub-entries hang
  16 px in on a guide line with their mod counts and their own sliding highlight, and they
  open and close smoothly (rows 0fr → 1fr, a turning chevron). Less always-on chrome: the
  sidebar's environment / "游戏没在运行" box is gone (one line shows only while a game runs or
  a new log waits), and the status line no longer says "游戏里现在是…" (with several plans, a
  green dot on that plan's entry) nor repeats the breadcrumb; the sidebar has no empty head.
  The home page lists the plans and packs of every environment: what is applied now is one
  large card (environment · version, name [方案 / 整合包], mods · last played, the problem
  if any, ▶ 启动游戏); every other one is a line on shared columns (game versions — a pack's
  from its mods' supportedVersions —, name, kind, mods, 应用, which switches to it without
  writing game files); the last run and scan are two lines. No page actions on home. The plan editor has three columns,
  未启用 | 已启用 | 详情, with 暂存 and 历史 as folds under 详情 (drag to a folded 暂存 still
  stages; `3` opens it); two splitters, old four-column widths carried over.
- "未保存 N" is now "未写入 N" in both interfaces. The old 日志 › 操作记录 view is gone;
  the history stays in the editor's third column.

### Known gaps
- Command output still says "unsaved" in a few places (`1 change unsaved`).
- `collection <id> --import` replaces the list with only the collection's mods, dropping Core.

## 0.8.0 — toward 1.0 (unreleased)

Specs: `docs/specs/` (rewritten 2026-10-21; the old `10-1.0实现.md` was folded into them). Section
numbers below are the current ones.

### Hidden and pinned mods (08 §6, 04 §7)
- Folders another manager hid by renaming About.xml are classified: **pinned** (a loadable copy
  of the lineage is what the game uses; a warning once the workshop copy changed since, with
  `archo mod <copy> --diff <workshop>`), **left over** (unsubscribed) or **hidden** on their own.
- 需要处理 gets a "hidden mods" group: keep hidden, show again (`archo unhide`), use the workshop
  version (`archo unpin`), move to the recycle bin (`archo trash`). Trash never deletes
  permanently and only takes hidden or unsubscribed workshop/SteamCMD folders.

### Live game log (03 §6.6)
- `archo log --follow [--once] [--json]` follows Player.log while the game runs: only new bytes
  are read, clusters instead of text are kept, attribution is the full-log engine's.
- RimCrow's companion log (`RimCrow_Realtime.log`) adds timestamps, levels and related mods.
- The window says "game running since … · n errors" and puts this run's errors first in
  需要处理; `archo status` shows a running game.

### Lists from other tools (04 §9)
- `loadout import` reads RimSort (XML, JSON, clipboard, Rentry), RimWorld's `.rml`, RimCrow's
  ModList.xml, JSON and share codes (also pasted as the argument), and workshop-ID lists.
- `loadout export --format rimsort|rml|rimcrow`.
- `archo collection <link|ID> [--import]` compares a Steam Workshop collection with what is
  installed and can load it into the workspace.
- The window imports and exports lists (导入 / 导出 in the Enabled list's head, `i` / `e`, the
  palette): a file (open dialog, typed path or drop), pasted text, a share code, a Rentry or
  collection link is previewed first — installed, not installed with Workshop links and known
  replacements, what replacing or only adding the missing mods would change — and applied as
  one undo step; ModsConfig.xml still changes only on save. Export writes RimSort, `.rml`,
  RimCrow ModList.xml, text or ModsConfig.xml (save dialog or a path; never the game's own
  file, never over a file without asking) or copies a share code. The TUI has both in the
  palette and `e` for export.

### Community data (04 §6.3)
- `archo data update` / `archo data status`: RimSort's rules and Workshop database, Use This
  Instead's replacements. All-or-nothing; the only command that fetches them.
- Missing mods get direct Workshop links; enabled mods with a maintained replacement are listed.

### Packs (08 §5)
- `archo pack build|list|launch|export|import|remove`: compile the current list into a pack
  (checked, sorted for the pack only, each mod copied without other game versions' folders),
  play it with its own list and saves through links in `Mods/`, share it as a zip or a list.
- The window has a Packs screen: the packs with when they were last played, Play, and build
  from the current list with a preview.
- Building a pack no longer freezes the window: it runs as an `archo pack build` child that
  goes on when the window closes, with its steps (check → sort → copy → write → ready) drawn
  as a diagram, a progress bar and a Stop button. The preview marks what needs attention:
  blockers, too little free space (checked before copying; the build is refused), mods that
  cannot be read, mods that do not list the game's version. A build cut short is shown as
  unfinished (build again, or delete what it copied) instead of disappearing; `archo pack
  list` shows it too, `archo pack stop` and `pack remove --unfinished` handle it, and `archo
  pack build` prints progress lines. Sizes show in GB from 1 GB up.

### Settings (06 §2.14)
- A settings screen (gear in the top bar): environment folders, launch method and arguments,
  language, Workshop descriptions, community data sources, database backup and compaction,
  and each cache with its size and a clear button.
- `archo db status|backup|vacuum`, `archo cache status|clear <kind>`, `env set --launch-args`.

### History (04 §3)
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
- The top bar holds the environment, the tabs, and the game: launch (the saved list) while it
  is not running, its state while it is. Sort and save sit in the Enabled list's head with
  the unsaved count, since they act on that list; packs have their own Play.
- The mod list no longer leaves the right side empty at narrower widths (a hidden staging
  column pushed the details into a zero-width track); staging hides below 900 px, the
  details below 680 px, and dragging a column edge skips hidden columns.
- With mods in staging the history stays, folded into a bar under them.
- The environment pill opens an environment page: the environments (switch there), this
  one's folders and launch method, and rescanning its mod folders. Settings keeps only what
  is RimArcho's (language, community data, database, caches, packs folder).
- Packs belong to the environment they were built in (a pack from another machine: to an
  environment of its game version); the packs page and the TUI say how many belong to
  others, `pack list` shows each pack's environment.
- A running game is told apart: its `-savedatafolder` (read from the process's command
  line) says whether a pack or the environment's list is playing; when the command line
  cannot be read, the launch records decide. The live log follows the log that run writes,
  the top bar says "整合包 X 运行中", `archo status` names it, and 需要处理's last-run group
  says whether the log came from the list or a pack. Its column is now called 游戏日志.
- Smoother pages: a new page fades in once (no slide that shook the scrollbars), a redraw or
  a sub-view switch does not animate, the tabs and sub-tabs have a sliding thumb, the game
  log shows what it read before at once and a loading line only after 150 ms, popups have no
  backdrop blur.
- `T` in the window moves the whole enabled list to staging in one step, each mod keeping
  its place; Core and the DLCs stay enabled, entries that are not installed stay in the list.
  In staging `T` puts everything back, as in the TUI. Both are in the palette of both
  interfaces.
- The Enabled list's position column is as wide as its longest number.
- About.xml, LoadFolders.xml and Defs saved as UTF-16 are read (02 §4.3).

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
