# Changelog

All notable changes to RimArcho. Versions follow semantic versioning; the MVP is 0.5.0.

## 0.9.0 — plans and launching (unreleased)

- Archo Monitor records game loading, save loading, new colony generation, playing and
  returns to the main menu. Can it run groups recorded errors by phase; the game log
  adds quiet phase separators. CLI and MCP include phase counts. Older logs keep their
  existing display. The bundled Monitor version now comes from `monitor/VERSION` and
  is updated independently of the launcher.

- Last-run errors in Can it run now name the kinds of errors (with a sample resource
  path for missing textures), include both error and warning counts, and open that run's
  game log with a related cluster selected. Unattributed errors open the complete log;
  earlier logs open their own file. The TUI keeps its existing log inspection view.
  The evidence line labels the mod's file and puts gaps between it, the log name and the
  time ("file Defs/…/Apparel.xml   ArchoMonitor.log   Oct 28 15:45"); before, the window
  ran them together so they read as one path.
  While the game runs, its "this run" items open the game log at that error group too
  ("open game log"), instead of "see the log and evidence" showing a block of text.

- When a save uses content from mods that are installed but not in the list (for
  example hairstyles of a hair mod you turned off), Can it run names those mods and adds
  "it is not in the list". Before, it named a mod that only patched the game's
  save-reading code (Vanilla Expanded Framework) and said "source not confirmed".
  `archo log` and MCP list the mod that defines the missing content first, with
  confidence "likely". Logs analysed by an earlier version are analysed again once.
- While the game runs, "this run" items no longer start with "this run," (the section
  heading says it) and their evidence shows only the log file, not the count again. When
  the previous run's errors follow them, a "last run" heading with that run's time
  separates the two, in the window and the TUI.
- An open Can it run updates itself when the background check finishes and when the game
  exits. Before, it kept the run's old time and items until you left the page and came
  back.

The new design's 1.0 work (01 §5.1). 1.0.0 follows once 01 §5.2 is verified, with a
`1.0.0-rc1` first.

- Downloading a game version no longer fails when SteamCMD drops its connection and
  reconnects on its own partway through: once SteamCMD says the game is fully installed,
  the version is added. Before, a finished 16 GB download was reported as "Steam could not
  be reached" and no game was added.
- Page widths follow the page's shape. Pages split into columns fill the window like the
  editor and the mod library: game log, all lists (list 38%, details 62%, draggable),
  packs, subscriptions and updates. One-column pages share one narrower width (960px) so
  labels and values sit closer: can it run, game, texture optimisation, settings. Before,
  all of them were centred in 1200px.
- The texture page's English hint no longer names its button in Chinese ("扫描统计").
- Sorting adds Archo Monitor right after Harmony when the setting "add Archo Monitor to
  lists" is on and the list has none, also after you took it out; it is part of the same
  undo step as the sort. Before, the setting only covered new lists, so a sort never put
  it back. When it cannot go in (no Harmony in the list, `Mods/` on FAT/exFAT) the sort
  says why. The sort message names what it added ("sorted: added Archo Monitor, 3
  moved"). Turn the setting off to keep it out.
- Game log: the error list takes 62% of the width by default and the log text 38%; the
  Archo log analysis card takes 40% of the right side's height. Double-click a gap to go
  back to these.
- Game log rows select without expanding or jumping to the top. Use the Archo Monitor
  row’s disclosure arrow or Enter for inline details; tab switches and live refresh keep
  the scroll position, and live refresh keeps expanded rows open.
- The launch page has more room again. The headline is no longer cut off: it shrinks to
  fit its line, takes two lines when it must, and a Workshop tag at the end of the name
  (`[UPDATED 1.7]`) moves to the line under it. Under ▶ the problem is one line, with its
  reason on hover, and × shows on hover. Unsaved edits are written inside the launch
  block ("2 unsaved") instead of a line of their own; the block no longer repeats the game
  and list. Only the applied list carries ▲ / ○; another list's state is on hover. Lists
  show just their mod count; packs still say "pack". Games show their short version
  ("1.7"), none when the name already says it, with the full build on hover. Over a
  background picture the three columns stand on a flat dark (or light) ground. The
  top-right lines line up with the launch column, and "nothing changed" is no longer
  shown. The launch page has no breadcrumb. Text on the page uses four sizes.
- Checking Workshop and SteamCMD mods for updates asks Steam about 100 items per request,
  four requests at a time, instead of one request per mod: a 5182-mod library took about
  two minutes and a few items failed every run. A failed request is sent once more after
  a second, and so is an item Steam answered as busy, timed out or rate limited.
- Mods that are no longer on the Workshop (deleted or private) are still listed under the
  check ("N no longer on the Workshop"), but they no longer make the check or the
  automatic mods check fail.
- Same-mod questions are rechecked on every scan even when the scan did not read one of
  the two folders (another game's `Mods/`, a partial scan). Before, such questions stayed
  however little content the two mods shared. Opening the database once (schema 24)
  clears the ones older scans left, such as 67 questions about one VRE patch; answered
  questions are kept.
- Commands that RimArcho prints (`fix:`, `next:`, `… N more (…)`, hints in errors) write
  mod references without `#`: `fix: archo enable nyh3`, not `archo enable #nyh3`. In
  PowerShell and bash a word starting with `#` is a comment, so a pasted suggestion ran
  without its argument. The bare form needs no quotes in cmd, PowerShell or bash, and
  every command already accepted it. Lists and other text keep `#nyh3`; `L#…` and MCP tool
  arguments are unchanged. In the TUI's technical pages, `enter` on such a line still
  opens the mod.
- A player who types `archo enable #nyh3` in PowerShell or bash runs `archo enable`
  without its argument. When the missing argument is a mod reference, the usage error now
  says why: a word starting with `#` is a comment there, so write `nyh3`.
- `stage restore` and `subscribe update` need mod references or `--all`: with neither
  they refuse instead of restoring every staged mod or updating every subscription, so a
  reference the shell dropped cannot widen the command. The TUI's `T` and MCP
  `edit_loadout restore` without refs still restore everything.
- The sidebar lists Mod library below Packs (lists, packs, Mod library). Page keys follow
  the sidebar: `shift+4` lists, `shift+5` packs, `shift+6` Mod library, in the window and
  the TUI.
- Missing base content on can it run now says which added things cannot load, explains
  the observed risk, and opens a focused problem panel with fixes. It finds disabled
  providers in the existing analysis cache and, when needed, searches installed inactive
  mods only when a problem is opened (bounded and cancellable). It suggests absent companions through
  the cached Steam DB, and links the mod's Workshop requirements. A matching last log
  with no errors attributed to the mod allows ignoring the group until the mod changes.
- A missing base template that a mod's own custom patch operation adds is no longer an
  error. RimArcho cannot simulate such operations and already counts them as successful;
  a Name written in one of them now counts as added (Chimera Creation (Continued) names
  Anomaly's chimera this way). The analysis cache is rebuilt once.
- The missing-content panel trusts a fresh analysis: it searches only the installed mods
  whose indexed content may change when enabled, so large libraries no longer end with
  "search incomplete, try again". Disabled SteamCMD and extra-folder mods are now indexed
  too, so their templates are found without reading files. When nothing installed supplies
  the template it says so;
  it no longer suggests trying again. Once installed providers cover every missing
  template, the panel and `problems --ref` drop unconfirmed candidates (such as a
  loadAfter mod), and the CLI no longer prints an empty `fixes:` heading.
- The LoversLab session check asked for an old settings address that the site now
  redirects, so a signed-in session was reported as needing login while the settings row
  still said it worked. It now reads the current settings page. Automatic mod checks
  count a file page as proof of login only when it shows the signed-in header, since
  file pages are public. The "needs login" notice uses a warning mark (⚠ in the window,
  `!` in the TUI) instead of ✓.
- Missing-content rows now keep one short explanation. The panel places enable/subscribe
  first as the primary action, shows source locations beside affected content, and only
  explains optional candidates when it lists them.
- `problems --ref p:…` (also `problems show` and MCP `list_problems ref=`) leads with
  meaning and fixes. Problem inspections show relevant evidence and affected content
  instead of opening a full mod card or all of a mod's patches; `mod` keeps its full card.
- Problem text flows naturally around linked mod names, without the spacing between
  separate facts. Links are limited to mods involved in each item; row popovers keep
  their own mod name as plain text. The editor panel shows evidence only for shared-log
  hints, keeping the cutoff date without repeating author-declaration lines.

- Can it run, the editor's problems panel and mod-row popovers now link each named mod
  separately, including identity questions and folded lists. Installed names reveal their
  editor row; known missing mods open the Workshop. Folded entries display as proper lists.
- Shared-log suggestions use a neutral information symbol in the window and terminal,
  show the corpus cutoff from embedded data, and no longer count as problems. With only
  suggestions, the editor button reads "suggestions". Diagnostic wording explains what to
  try, avoids XML field names and internal log references, and treats corpus rates as hints.

- Identity questions now require matching mod contents, exclude shared About/preview/routing
  files and empty or repeated contents, and stop asking when either mod has several
  candidates. A scan removes obsolete unanswered questions; player answers are preserved.
- Answering “same” now keeps references in every plan, older list versions, undo/redo and
  staged positions valid in the same transaction. Fixed packs follow the merged identity.
- All three identity answers remain equally visible with effects and confirmation.
  A quiet “Why this question?” link opens a small comparison with a conclusion, two columns,
  up to five matching files and the three answers. The main reason also appears on the row.
  CLI/MCP comparisons omit empty rows and method details; bare `identity` lists questions.
- A mod row's version warning now names that mod's declared versions. Version reminders
  can be ignored per mod and game minor version across all lists and games, survive updates,
  and can be restored from the ignored list or mod detail. The version list opens as a
  scrollable panel with clickable mod names, declared versions, compact ignore/restore
  controls, and groups by declared versions. Group/all ignore asks first; all restore is
  available. Panel rows load only when opened. Ignored warnings leave problem counts and
  pack-build version warnings; CLI/MCP help and output use ignore/restore consistently.
  CLI: `problems --include-muted`, `problems mute-version|unmute-version <mod>…
  [--game-version X.Y]`; MCP: `list_problems include_muted`, `mute_version_warnings`.
- While the window or TUI is open, one background scheduler checks SteamCMD, Git and
  download-page mods, managed tools, LoversLab sessions and community data. Checks remember
  their last attempt across restarts. Settings has one switch and interval control; mods
  and data default to daily, tools weekly. Mod updates remain the player's choice, and
  any registered game running blocks installation. New updates appear in subscriptions
  and Settings with quiet notices; CLI/MCP status and data output use one compact summary
  of pending updates or the latest check time. Settings keeps only the switch, interval
  and a note under automatic checks; dates appear with their subjects. Results say what
  has updates, intervals use daily/weekly wording, and Chinese switches show 开/关.
  Unconfigured LoversLab accounts have no status text. Tool rows display new versions.
- LoversLab sessions refresh through a signed-in request, saving renewed cookies in the
  existing encrypted session. Settings no longer waits for verification. If the site
  requires login or the session will expire soon, Settings and a notice say so ahead of
  downloads. Managed tool updates refuse tools in use; unsupported todds releases are
  shown without offering an incompatible install. Bundled fonts have no separate update.
- Community data refreshes automatically using ETag and Last-Modified. Unchanged responses
  advance the check time; failed downloads retain the previous data without a notice.
  Data, source records and the compact index are staged together; replacement or database
  failures restore the previous set. This does not cover process crashes or power loss.

- Launching from the window with a required mod missing or disabled now asks once ("launch
  anyway" / cancel) and lists what is missing: the game would not load the mods that need
  them and may reset its list to the base game. The CLI and TUI still launch without asking.
- When an outside change that RimArcho loads by itself drops mods from the list (the game
  resets its list after mods fail to load), `scan` says how many and which version still
  has them, and the can it run page shows a card: see the difference, put the version
  before back (one undoable step), or keep the change.
- The outside-change card's "see the difference" shows the two lists in the window, one
  mod a line; it used to run a CLI command the window does not allow. A save or launch
  refused over an outside change points to that card instead of listing `archo` commands.
- Downloading with SteamCMD as the account the running Steam client is signed in with signs
  the client out; the preview now says so. Steam's "no connection" (code 3) reads "the
  Steam client is offline" with a hint to restart Steam, and Workshop tasks show Steam's
  own reason instead of always "download was not confirmed".
- Steam subscribe and unsubscribe no longer get undone by Steam's own subscription refresh:
  the worker waits for the refresh to land and asks again if an item flips back.
- Background tasks: items handed to a new task by "continue" or "retry" are marked so, and
  the old task no longer offers the same action for them.
- A batch preview in the mod library goes ahead without the copies that cannot be done
  now and names them, instead of refusing the whole batch.
- Mod library cells stay on one line (long text is cut short), and analysis progress is
  shown in the window's language.

- Sorting a list with a constraint cycle now gives up the weakest constraint on the cycle
  (by the log corpus's strength: harmless, weak, then strong; then community rules,
  loadAfter, loadBefore, dependencies) and orders the rest by what holds, instead of keeping
  the whole cycle in its current order. A dependency the list had the wrong way round is now
  put right. `archo sort` marks the constraints it gives up `[broken]`; the suggested ignore
  is the weakest of them.
- Sorting honours loadTop / loadBottom (04 §5.5): from the community rules (RocketMan,
  RimThreaded, Performance Optimizer and others load last; XML Extensions first), from new
  user rules `archo rule add top|bottom <mod>`, and from RimCrow's user rules on import. A
  top mod takes what it needs up with it, a bottom mod takes what needs it down. They are
  preferences: they never break a constraint and official content stays first. `archo sort`
  lists them under "ends"; `archo rule add ignore community:<pkg>:loadbottom` drops a
  community one.
- `archo enable` without a position places each mod by the sort constraints instead of
  always at the end, with the same rule as the editor's suggested position: right after
  the last enabled mod it must load after (never ahead of Core and the DLCs); before the
  first enabled mod that needs it when that is all there is (a library, a DLC enabled
  later); at the end when nothing constrains it; an `attach` mod right after its target.
- The editor's suggested position (`m`) no longer puts a mod that only needs Harmony
  between Harmony and Core.
- The mod library now offers only library actions: list edits and single-mod DDS conversion
  stay off its row menu. Subscribed Workshop originals can be resubscribed through the
  Steam task queue, CLI `workshop resubscribe`, or MCP `manage_library`: confirmation warns
  that RimArcho moves the folder to the recycle bin before Steam downloads it again, losing
  local edits. Only confirmed unsubscribes whose folders are verified gone subscribe again;
  cleanup retries briefly and reports failures per item. Lists keep the same copy identity;
  a running game blocks the operation. Confirmed subscriptions, including leftovers, now
  request high-priority Steam downloads and wait for installed files, with live byte
  progress in background tasks and CLI output. A missing folder never counts as installed;
  stalled downloads fail with a check-Steam hint, and stopping ends the download worker.
- Smaller fixes: the sidebar's lists show 全部列表 first, then at most four lists (the
  current one, the open one, the most recently used); the all-lists page puts "current" /
  "make current" at the right of each row and drops the collection icon; the editor's
  disabled column labels the source only for mods with several copies; Workshop mods
  uploaded with a `.git` folder show as Workshop, not Git; the game log pairs a run with
  Archo Monitor's log first and no longer with a stale or empty RimCrow log, and opens on
  Archo Monitor's tab when the run has one.
- `ctrl+\` turns reference numbers on and off: every control, text block, image and chart
  gets a thin red frame numbered at its top-left (`lib12`: the page, then the number), so a
  change can be asked for as "change lib12". The frames take no clicks.
- Lists are shared by every game (04 §1.7). Switching games no longer hides the lists made
  in another one; each game keeps its own current list. A list's Core, DLC and game-folder
  `Mods/` entries are read as the open game's own copy by packageId. Removing a game keeps
  the lists. The database moves to schema v19: lists with the same name in two games get the
  game's name added ("default (1.6)").
- The mod library also lists every other game's own `Mods/` folder, and a list can enable
  those mods in any game: writing to the game links them in as `_Archo_env_<ID>`. When the
  open game has its own copy of the same package in `Mods/`, that copy is used. A game's
  `Mods/` shows as `local-<game>` (`local-main`, `local-1.6`); `source:local` still matches
  them all.
- Removing a game and adding it back restores the SteamCMD folder inside it, with its mods'
  IDs unchanged; it used to stay "removed". `folder add` on a removed folder also restores it
  instead of registering a new one.
- In the plan editor, hovering a row's warning or error icon says what the problem is;
  clicking it pins the popover so its text can be read and selected, with the same quick
  actions as "can it run" (Esc or a click elsewhere closes it).
- Most text in the window can now be selected and copied: mod names, paths, details, logs
  and popovers. Only controls and draggable rows keep selection off.
- The plan editor and mod library filter bars now include Location, with one value per
  registered source folder. Multiple locations, exclusions and quoted paths in search are
  supported; CLI/MCP library queries accept `location:<source folder path>` too.
- Opening a list no longer switches the list you play (04 §1.3). A list opened from the
  sidebar, the palette or list management is only shown and edited; the current list — the
  one the launch page shows and launches — changes when you make another current, write it
  to the game or launch it from its editor. New lists open without becoming current. The
  editor says which list launches and offers "make current". 列表 opens the current list,
  which comes first in the sidebar and in list management (管理 moves last), and reordered
  entries slide to their new place. `--open-plan <name>` runs one command on another list.
- List management is simpler (06 §2.17). The sidebar entry is now "all lists" (and the mod
  library's first entry "all mods"). One "new / import" button opens one form that starts
  empty; importing has a short guide, a file button that reads the file at once (no typed
  path) and drag-and-drop, and its one main button says "preview" until the content is read,
  then "create and edit". "back to management", "continue later" and "create only" are gone;
  leaving the form drops a draft whose name was never changed. The picked list shows rename,
  duplicate, delete and edit side by side; double-click the name (or F2) to rename in place,
  and "duplicate" copies the list at once and lets you name the copy.
- Mods an imported list has but you don't: the preview offers "subscribe all" (Steam or
  SteamCMD, one background task), and "can it run" folds them into one item with the same
  action instead of one error per mod (and none for the mods that need them). Once
  downloaded they take their place in the list.

### Breaking
- Database schema v10 → v18: plans (v11), analysis kept per plan (v12), copy notes (v13),
  shared mod folders (v14, below), an index for looking defs up by `Name` (v15), the DDS
  RimArcho wrote (v16), a plan's game versions and followed collections (v17), the link
  ledger (v18). Each environment's workspace, undo stack, versions and
  staging become its first plan, `default`; each `loadout tag` name becomes a plan of its own
  with one `import` version (04 §1.4).
- `launch --force` is removed: launching always writes the plan first (08 §2).
- Severities follow the log corpus (03 §4.3.1): `official.order` error → warning,
  `order.cycle` warning → info, `version.unsupported` reversed (stopping at the previous minor
  version → warning, older only → info), `harmony.multi-transpiler` warning → info (warning when
  a current log's stack passes through the method); `order.violated` is info for weak and
  harmless constraints (04 §5.3). Analyzer version 7: stored analyses are rebuilt once.
- `loadout tag` is removed: a named list is a plan (05 §4.3).
- `archo save` means "write to the game without launching" (04 §4.1).
- `save` and `launch` refuse while ModsConfig.xml holds an outside change that was not loaded
  (04 §4.1, §4.3).
- Locked copies made by `pin` live in the lock library, not in `Mods/` (04 §7).
- Light packs are gone: a pack always carries its mods' files. Sharing a list with each mod's
  version is the RimArcho list export format (Packs, below); `pack export --light` is now
  `pack export --list`, and `pack build --light` and the build page's full / light choice are
  removed. MCP `manage_packs` `list` replaces `light`.
- One environment is one game folder: `env add` refuses a game folder another environment
  uses, and the Workshop, SteamCMD and other folders are no longer an environment's (below).

### Environments and shared mod folders (02 §0)
- An environment is one game folder: its `Data` and `Mods`, version, launch, arguments,
  saves and plans. Saves are `--saves shared` (the game's default folder) or `--saves own`
  (`<data>/userdata/<env>`, started with `-savedatafolder`).
- The Workshop folder (found from Steam), SteamCMD folders, other folders, the Archo Monitor
  folder and the lock library are shared by every environment: `archo folder list|add|remove`,
  the settings page's Mod 文件夹, MCP `status show=folders`. One Workshop folder; naming another
  moves it. `env add/set --workshop/--steamcmd/--extra/--remove-source` still work and mean the
  folder commands.
- One record per mod: a shared folder's mods, every lineage, tags, notes, aliases, copy notes,
  history and user rules are the same in every environment; Mod 库存's 用在 lists every
  environment's plans. Problem ignores stay per environment. Analysis stays per game version.
- `scan` reads the shared folders once plus the game's own; `scan --game-only` reads only the
  game's (adding a game), and its `Mods/` copies still join the Workshop original's lineage.
- The lock library is `lock.dir` itself; older `<lock.dir>/<env>` folders keep working as
  `src:locked-<env>`.
- Migration v14 merges each environment's copies of a shared folder, their instances by
  folder name and the lineages through them, and rewrites every reference (plans, undo,
  versions, staging, aliases, snapshots and origin markers, events, questions, notes, rules,
  player data, ignores, launch fingerprints). IDs on disk (pack.json, origin.json) resolve
  through the new `id_alias` table.
- A SteamCMD game's own Workshop folder (`<game>\steamapps\workshop\content\294100`) is a
  shared SteamCMD folder, added on `env add`, on a download and, for games added earlier, on
  the next scan. It never replaces or moves the Steam client's Workshop folder; a removed one
  stays removed.
- 删除环境 on the environment page, in the window's environment chooser (`Delete`) and in the
  TUI's `E` (`d`): a dry run, then one confirmation. Only RimArcho's records go (plans, launch
  records, the mods read from its `Data`/`Mods`, history); game files, mods, saves and the
  shared folders stay. Refused while that game runs; another environment takes over, or the
  guide opens when none is left. `env remove <name>` without `--yes` is now that dry run.
- The environment page shows only the game folder, version, launch, saves and arguments. The
  add-environment guide offers 已有的游戏 (detected installs, a typed folder or the folder
  dialog; named after the version, e.g. 1.6) or 下载一个版本, and after adding reads only the
  new game's `Data` and `Mods`.
- A game outside a Steam library launches `direct` by default even when a Workshop folder is
  given (`env add --workshop`); only a game inside a Steam library defaults to `steam`.

### Downloading a game version (08 §11)
- `archo env branches` lists RimWorld's Steam branches from SteamCMD (anonymous
  `app_info_print`); `archo env download <branch> --to <dir>` downloads one
  with SteamCMD and adds it as an environment. The guide's 下载一个版本 does the same with a
  progress bar.
- Anonymous Steam login is the default — RimArcho also serves players without a Steam
  account. An account name can be stored in the `steamcmd.account` setting (settings page,
  SteamCMD group) or given for one download (the download form / `--account`); `anonymous`
  there overrides a saved account, and clearing the setting goes back to anonymous. The
  branch list is always read anonymously. The password and Steam Guard code are typed in
  SteamCMD's own window — RimArcho never asks for, receives, stores or passes them, only the
  account name. Progress and the result come from SteamCMD's `logs/console_log.txt`;
  failures are said plainly (no ownership, password, Steam Guard, disk full, branch,
  network; for an anonymous login, Steam refusing it without a license, with the way out:
  an account, or a game folder the player already has).
- `steamcmd.path` can be set (settings page, SteamCMD group) to a steamcmd the player
  already has.
- SteamCMD is downloaded from Valve into `%LOCALAPPDATA%\RimArcho\steamcmd` only after the
  player saw its size (`--install-steamcmd` in the CLI).

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
- `archo log` (MCP `analyze_log`) prints a cursor; `--since <cursor>` (MCP `since`) lists only
  the clusters that are new (`new`) or whose count grew (`+N`) since then, ArchoMonitor.log
  included, for an agent polling a running game (03 §6.6). When the game restarted, the cursor
  is called stale and the old run is Player-prev.log (`--prev --since`).
- The first query on a new log is fast again (07 §6): on corpus logs 1.1–14 s → 128–330 ms,
  where it was 568 ms on the reference log. Looking defs up by `Name` has an index (schema v15);
  the files mentioning each def name are indexed once per enabled set (`cache/refindex/`, built
  after an analysis), so cross-reference referrers no longer read every mod file; repeated log
  entries are classified once. Referrer evidence is listed by file.

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
  `edit_loadout` gains `checkout` and `ignore_external`; `list_library`, `manage_library`;
  `preview_uninstall` (read-only; there is no tool that uninstalls); `search_files`,
  `test_run`, `check_links`, `repair_links` (below). 38 tools in all.
- `manage_plans` gains `versions`, `follow`, `unfollow` and `sync`; `manage_packs` gains
  `check`.
- `exchange_list` and `maintenance` are open-world; the destructive ones (`save_loadout`,
  `launch_game`, `manage_plans`, `manage_packs`, `resolve_identity`, `manage_hidden`,
  `manage_library`, `maintenance`) are listed in the server instructions with "show the dry run and ask first".
- Instructions and descriptions speak of the current plan, writing to the game and locked
  versions.

### Uninstall (01 §4.1)
- `archo uninstall` shows what RimArcho put on the machine and what happens to each piece;
  `--yes` runs it. Every row of the write list (01 §4) has an uninstall step or a stated
  reason; a test keeps the two in step.
- RimArcho's `_Archo_` / `!ArchoPack_` links leave every game's `Mods/` (never their targets;
  a same-named folder that is not a link stays and is listed). Each ModsConfig.xml RimArcho
  wrote keeps the player's list without Archo Monitor and the mods only reached through a
  removed link, and loses `.rimarcho.bak` (`--restore-modsconfig-backup` puts the backup back
  instead). Monitor and the SteamCMD RimArcho downloaded are deleted.
- Choices, defaults first: lock library copies to the recycle bin (`--keep-locked
  [--locked-to <env>]`: into a game's `Mods/` as ordinary mods); packs kept, moved out of the
  data folder (`--remove-packs`, `--packs-to`); own save folders and save backups moved out
  (`--remove-saves`, `--saves-to`; default `Documents\RimArcho saves`); downloaded games kept
  (`--remove-games`). Refused while a game or another RimArcho from the same folder runs.
- The data folders and the program go last, by a script that waits for RimArcho to close; it
  deletes only the release zip's files, the program folder only when empty, and lists what it
  could not delete in `%TEMP%\RimArcho-uninstall.log`. A failed step keeps them, so it can
  run again.
- GUI 设置 › 卸载 RimArcho: the plan with the questions inline, one button, progress, the
  report and 关闭 RimArcho; TUI palette 卸载 RimArcho asks for the word `uninstall`; MCP
  `preview_uninstall` is the dry run only.

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
- The window's Mod 库存 › 全部: filters and a table like a file manager's details view.
  Click a header to sort (again reverses, with an arrow), drag it to move the column, drag
  its edge to resize, right-click the headers to show or hide columns (名称 大小 状态 来源
  用在 by default; 更新日期 版本 作者 标签 上次游玩 on request); the layout is kept in
  `ui.gui.library.columns`. The name cell holds only the name, the copy's kind is in 来源,
  tags are not in rows. Rows show only states to act on (hidden, leftover, upstream
  updated, unloadable, missing, ignored). 不支持 1.7 and 不在任何方案里 are not states at all:
  not in the filters, not `state:` values or completions, not in the CLI / MCP (`state:unsupported`
  and `state:unused` are gone; `used:none` still finds copies no plan uses).
  The filters mark the game's version and add 更新日期 (一周内 / 一月内 / 半年内 / 更早,
  the Workshop update time, else the folder's). No environment chip on this page; the
  button says 重新扫描. A selection action bar; the TUI has the list in the palette.
- A copy's details: its note at the top, then only the actions that apply — 恢复, 退订 /
  删除, 重新订阅, 备份锁定版本, 打开文件夹, 打开网页 — then one field list. 删除 now also takes the player's own copies in `Mods/` and the lock library
  (not links, not while a plan uses them).
- Copy notes (schema v13 `instance_note`): each copy has its own note, apart from the mod's.
  RimArcho writes the first version from how the copy was made ("该锁定版本创建于 10 月 21 日，
  为方案「测试」创建，自从创建后上游有 2 次更新，原因：…"), worked out each time so the update
  count stays current. Copies RimArcho did not make get one too: where they came from, the
  day RimArcho first saw them and, beside a Workshop copy, how often it updated since. Edit it in the window (click), the TUI (`n` in Mod 库存), the CLI
  (`archo note <ref> <text> --copy`, `--clear`) or MCP (`set_player_data` `copy_note`);
  `pin --reason`, `library move-to-locked --reason` and the MCP `reason` record why.
- 退订 and 重新订阅 go through the running Steam client, as RimCrow does (08 §10): a
  short-lived `archo steam-worker` process, started in `%LOCALAPPDATA%\RimArcho\steam` with
  the game's `steam_api64.dll` copied in and `steam_appid.txt`, calls the Steamworks flat API
  (no cgo) and exits. Unsubscribing makes Steam delete the folder, so it shows the dry run
  first and is refused while the mod is enabled, used by a plan, or the game runs. Only when
  Steam is not running or the DLL cannot be loaded does the window open the Workshop page
  instead, and say why. CLI `archo workshop unsubscribe|subscribe|state <ref…>`; MCP
  `manage_library` actions `unsubscribe`, `subscribe`, `steam_state`. Without Steam,
  `workshop state` / `steam_state` show whether each item's local folder is there, marked
  local, with the subscription and update state unknown (never "not subscribed");
  subscribing and unsubscribing still need Steam and say so.
- `archo library --sort` takes every column (name, size, changed, played, state, source,
  used, updated, version, author, tags; a leading `-` reverses) and `--include updated:week`
  (month, half-year, older); `--lineage` lists each copy's `updated:` and `note:`.
- RimArcho's own copy note is in the normal text colour, like the player's; only the small
  "RimArcho 写的 · 点击修改" line stays muted.
- A detail's 历史 keeps the newest three changes and links to 在变化记录查看全部: 变化 narrowed to
  that mod's Lineage, every scan that changed one of its copies, with a removable "只看 <名字>"
  (× or `esc` shows every mod again). The TUI opens it with `C`.
- `ctrl+o` opens the cursor's mod's folder and `ctrl+w` its Workshop page, in the plan editor
  and Mod 库存, in both interfaces; without a Workshop ID the status line says so.

### One interface for the plan editor and Mod 库存 (06 §2.5, §2.9, §3, §8.5)
- One filter bar on both pages: a search box (the grey completion and `tag:` word matching
  as before, now with `version:`), the filters in force as removable chips, and 筛选 (`\`).
  Open, it shows one line per group (标签, 来源, 版本 with the current game marked, 内容 and 有
  in the editor; Mod 库存 adds 状态, 用在, 更新日期): the values most used first with counts,
  then a box to type in for the rest. Left click (enter, space) keeps a value, right click
  (`x`) leaves it out; values of one group are "or", groups "and". The keyboard reaches all of
  it (↑↓ groups, ←→ values, a letter types into the group's box, backspace in an empty search
  takes the last chip off).
- The editor's bar sits above 未启用 and filters 未启用 and 已启用 together; the heads say
  "已启用 6 / 34". 暂存 keeps its own search. While the bar filters, `J` / `K` (and alt+) move
  past the next row shown (`view.StepAt`); `m`, `space` and drops still place by the whole
  load order.
- Mod 库存 loses its filter column: the bar is over the table, and a copy's details are a
  permanent right-hand column (the editor's detail card) that follows the cursor; `enter`
  lays them over the page, `enter` or `esc` closes them, `→` on a copy focuses them.
- One key table on both pages (06 §3.5), in both interfaces: `PgUp` / `PgDn` move the cursor
  (they scroll only the focused details), `ctrl+a` selects everything shown, `enter` opens /
  closes the details, `esc` backs out (details → selection → search and filters), `space`
  takes effect / stops (启用 / 停用 in the editor; 加入当前方案 / 从当前方案拿掉 / 换成这一份 in
  Mod 库存), `m`, `t`, `x`, `g`, `n` (the mod's note in the editor, the copy's in Mod 库存),
  `ctrl+o` / `ctrl+w`, `u` / `ctrl+z`, `ctrl+y` / `ctrl+r` (Mod 库存's undo is the plan's),
  `Del` (退订 / 删除, dry run first). `tab` / `shift+tab` switch the pages under one sidebar
  entry (全部 ↔ 变化, the plans); the editor's areas are `←→` and `1`–`4` only. `#`, `o`, `d`,
  `f` and `!` are no longer shortcuts (`shift+1`–`7` go to the sidebar's pages). `?` shows the
  keys of the page it is pressed on (`view.KeyHelp`).
- One row menu on both pages, from one op table (`view.OpTable`, `view.CopyOps`): 方案
  (space, m, t, x, 从所有方案里拿掉), 整理 (标签 g, 备注 n), 打开 (ctrl+o, ctrl+w), 版本 (锁定这个
  版本, 更新锁定版本, 解除锁定, 备份锁定版本), 磁盘 (恢复, 重新订阅, then 退订 / 删除 in red),
  each with its key, only what applies. The editor's menu now has the version and disk
  ops too.
- New rule (06 §3.1 rule 6): everything the mouse can do, the keyboard can do. The menu key
  and `shift+F10` open the row's menu at the cursor (↑↓, enter, esc); `H` lists the history,
  `C` a mod's changes, `<` `>` `=` set the editor's column widths, `s` in Mod 库存 is the
  headers' menu (sort, move, size, show columns). A test checks every op has a key or the
  menu and a runner on both pages.
- Mod 库存's page data is 0.77 MB instead of 1.47 MB on a 1,284-mod library: each copy
  carries op codes, the labels come once, and folder paths and Workshop URLs stay in Go.

### 变化 (06 §2.9, §2.3)
- One row per mod: its changes in the time range merged into "name · what it means", with a
  tier pill (会影响启动 / 留意 / 只是内容), sorted by tier then name. The sentence comes only from
  what the scans recorded: supported versions added or dropped (the game's own version
  flagged), the mod's version, dependencies and incompatibilities with their installed /
  enabled state, load rules (compared from the two declarations), files grouped as code / Defs /
  Patches / About / textures / sounds / translations / other with size changes, the Workshop
  update time, and a locked version left behind.
- Default: the current plan's mods since that plan was last played (the last scan when it never
  was); `a` / the two buttons switch to the whole library. Older changes fold under 更早 by
  day; new and gone mods are one summary row. One mod's history (the detail's link, TUI `C`)
  lists every change by scan.
- Opening a row shows the grouped file summary first, then the raw file list behind `i`, then
  its actions: 打开网页 (the Workshop change notes, `ctrl+w`), 看依赖 (`D`), 排序 (`S`),
  锁定这个版本 (`p`, dry run first). `tab` goes to 全部.
- `archo changes` prints the same page in English (`--library`, `--since`, a ref for one mod's
  history); MCP `list_changes` gains `library`. The old per-event listing is gone. The builder
  is the new `internal/modchanges`, shared by the GUI, the TUI, the CLI and MCP.
- Home page: the 「从上次扫描以来」 line is replaced by one sentence about the current plan
  ("当前方案自上次游玩以来 · 5 个 Mod 更新，1 个需要处理 →"), which opens 变化 with that scope and
  time. The 上次游戏 line shows only the result ("上次游戏 · 10 月 21 日 · 3 个错误"), reads a newer
  Player.log by itself when the page loads or the window regains focus, and is hidden when there
  was never a run.

### The window (06 §2)
- One full detail page (06 §4.2): `enter` in the plan editor and in Mod 库存 › 全部 opens the
  same page. The mod on the left (picture, name, author, state, tags, description); its
  copies (state, size, version, folder, note, and the selected copy's actions), where each is
  used with the current plan's `space`, the fields and the history on the right; 技术信息 under
  both, one tab per section of `view.Sections`. `← →` the previous / next mod of the list it
  came from, `esc` back there, `[ ]` another copy, the row menu and copy keys on that copy,
  `1`–`6` its parts. The editor's detail column and Mod 库存's details stay as its short form.
- `1`–`9` focus a page's areas on every page, as `1`–`4` do in the editor, and each area's
  heading shows its number: Mod 库存 (filter, table, details), 能不能跑's sections, 游戏日志 (the
  errors, the log text), 环境, 设置's groups, the build page's parts. The TUI's 能不能跑 and
  Mod 库存 do the same.
- 贴图优化 is a page of its own under 库, after 整合包 (`shift+7`; 环境 is now `shift+8`, `*` in
  the TUI, where `shift+7` shows the textures panel). 设置 keeps only where todds is.
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
- 启动 is laid out like the game's main menu over a background image (06 §2.3): the plans and
  packs down the left (kind first, then the name; the applied one raised with 编辑 ↵, the
  others with 应用 ↵ on the cursor), the last run and scan under them, ▶ 启动游戏 in the
  bottom right with the problem as a tab under it. `↑` `↓`, `enter` (apply / edit), `L`.
- 设置 › 启动页背景 (06 §2.14): the game's art (RimWorld's planet by default, the DLC
  covers), the images of every mod with "background" in its name (their
  `VBE.BackgroundImageDef`s: Vanilla Backgrounds Expanded, Shella Backgrounds, …), your own
  PNG / JPG, or plain; dimming, blur, focus, and rotation among starred images, with a small
  live preview. Each image is decoded once, one at a time, into 1920 / 480 px JPEGs in
  `cache/backgrounds/`; the page never loads an original, and drops the image when you leave.
- "未保存 N" is now "未写入 N" in both interfaces. The old 日志 › 操作记录 view is gone;
  the history stays in the editor's third column.
- Switching the language in 设置 happens in place: the page keeps its place, the lists their
  selection and scroll, the details stay open; it no longer reloads to the home page.
- The plan editor: 排序 (`S`) is in the 已启用 column's header, 导入 / 导出 (`i`) in the
  toolbar. Rows no longer show tags, in any column or interface (they are in the detail and
  `tag:` search, 06 §3.8). The detail is one field list — Archo ID, 包名, 位置 [打开位置],
  工坊 [打开页面], 标签 [编辑], 语言, 依赖, 冲突 — shared with Mod 库存; clicking Archo ID,
  包名, 位置 or the Workshop ID copies it and says 已复制 (06 §4.1); the list has no
  background and long values wrap. The TUI detail draws the same list.
- The TUI's 能不能跑 has the GUI's four sections, each heading with its count (and which run
  the run errors are from), and every item's evidence chip on the line under it: an icon
  (Nerd Font or the compatible set), the kind and the source, e.g. `≣ Mod 自己声明 · About.xml
  modDependencies`. ← → jump between sections. The top bar's 能不能跑 N now counts this
  run's errors too, the same number as the GUI's sidebar badge (G13).
- The running game is at the GUI sidebar's foot and, in the TUI, at the right end of the
  status line (no longer in the top bar): `● 游戏运行中 · 14:05 起` with this run's errors as
  an amber number, `● 分析本次日志` after the game when a new log waits, else nothing (G17).
- `esc` brings every page back to its base (no popup or menu, nothing selected, the filter
  bar closed) in at most 3 presses, checked on every page of both interfaces (G5, T20). In the
  window, a row menu or ⋯ menu opened with the mouse now closes with `esc`, and `esc` on the
  rows of the editor or Mod 库存 clears and closes an open filter bar in one press.
- The navigation sits at the side or at the top: the toggle (heading the sidebar, or the
  strip's left end; `ctrl+b`) moves it, and `ui.gui.nav` remembers it. At the top the same
  entries form one strip, groups as thin separators, sub-entries as a dropdown from the
  chevron (`↓` from the keyboard), 设置 and the running game at the right end; it scrolls
  when the window is narrow. The change is a view transition. (This replaces the expanded /
  icon rail / hidden sidebar made the same day.)
- The top bar has no buttons on any page, only the breadcrumb and the environment chip. Page
  actions sit at the right end of the page's description line; the plan editor has that line
  too ("启用了 34 个，按游戏加载的顺序") with its undo, import / export, build a pack, write to the
  game, ⋯ and 启动游戏.
- In the window, the editor's 启动游戏 (button, `L`, the palette's 启动) opens the 启动 page with
  the current plan applied and the focus on ▶ 启动游戏; `L` or enter there starts the game. The
  TUI's `L` and the CLI / MCP `launch` still launch at once.
- The gaps between cards are 4 px everywhere (`--card-gap`): the editor's columns and its
  stacked details / 暂存 / 历史, the filter card and what is under it, Mod 库存's two sides, the
  log pages' columns. The draggable gaps keep a 10 px grab area that reaches over the cards'
  edges.
- New icon artwork: the portrait with the accent rim light (1024×1024). 48 px and up show it
  whole; 40 px and below show the head (hair and face) instead of the face alone (06 §8.6).
  The icon renderer now draws groups with inherited fill and stroke, transforms, clip paths,
  radial and bounding-box gradients, evenodd fills and Gaussian blur, and skips what is only
  defined (defs, clip paths, gradients, filters).
- The bow from the portrait is RimArcho's mark (06 §7): in front of Archo's name on the log
  page's Archo 日志分析 card and the 让 Archo 分析 button. Brand colours: a `#4f46e5`
  rim in the light theme; a `#a5b4fc` rim and a grey outline in the dark theme.

### Memory (07 §8.3)
- 设置 › 界面 has GPU 加速 (`config set ui.gpu on|off`, default on). Off draws the window in
  software and saves about 20–35 MB; it takes effect the next time RimArcho opens.
- The window's Go process returns freed memory to the system 2 seconds after heavy work, and
  forgets a page's caches when the player leaves it (Mod 库存, 需要处理 / 日志, 首页); a 32 MiB
  soft cap (now 128 MiB, below) and a 256 KiB SQLite page cache as in the TUI. Idle 85 → 22 MB.
- The soft cap went from 32 MiB to 128 MiB: at 32 MiB a list edit ran garbage collection
  dozens to hundreds of times and was 2–3× slower. Idle memory still comes from the explicit
  release after 2 seconds.
- The window icon is no longer drawn at 256 px: not at all when the exe carries its icon
  (release builds), at the system icon size otherwise. Start-up peak 122 → 67 MB.
- WebView2 runs its network service inside the browser process (one process fewer), and its
  memory target is low while the window is minimised.
- Pack names are checked without a regular expression (about 1.4 MB less).
- Measured on the same data copy, GPU on: idle 202 → 138 MB, plan editor 204 → 138,
  Mod 库存 175 → 151, minimised 171 → 105. GPU off: 182 → 123, 185 → 123, 155 → 132, 150 → 88.

### Packs (06 §2.6, §2.7, 08 §5.3, §7)
- The recycle bin has one entry that checks the allow-list (08 §7) for every caller: a mod
  folder inside a source folder (not the folder itself), a pack directly in the pack folder,
  saves (a game's `Saves/`, own save folders, save backups) and a game downloaded with
  SteamCMD. A link, or any other path, is refused whoever asks (`trash`, `pack remove`,
  Mod 库存, moving copies to the lock library, uninstall).
- 编译成整合包 is the current plan's own page, laid out as drawn: the name, the four things building does (check, leave out, lock, put in place) with their
  numbers, and a summary (environment, mods, size before → after, own saves, location) with
  编译 and 先看清单，不写磁盘 (the text of `pack build --dry-run`). With error-level problems
  编译 is disabled and says 先修好 N 个问题 →, which opens 能不能跑 (G14); not enough space, an
  unreadable mod, a bad or taken name and a build in progress disable it too. The build's
  progress shows on the four steps with 停止; a finished build opens its pack. Builds cut
  short are listed below. Keys: `B` build, `V` the dry run, `n` the name,
  `esc` back to the plan. The old build popup is gone.
- Each pack is a sub-entry under 整合包 with a page of its own: what it is, 启动 (`L`), 导出
  (`e`, a save dialog) and 移到回收站 (`Del`), and its mods in load order with their sizes and
  the version folders left out. `tab` / `shift+tab` go from pack to pack (from the list: the
  first one); the list opens a pack with `enter`.
- `Mods/` never gets two copies of one packageId from RimArcho (the game logs "Tried loading
  mod with the same packageId multiple times" and ignores one; the player's 2026-10-21 log
  had six, from launching the pack 测试 next to RimCrow-style copies). Launching a pack uses
  the copy already in `Mods/` when it is the same version (listed as "from Mods/"), refuses
  and names the folders when it is another version, and drops a plan's `_Archo_` link of the
  same package for the run. Writing a plan refuses a new `_Archo_` link next to another copy
  in `Mods/`. Such pairs already there are `pkg.duplicate-mods` errors in 能不能跑 › 会让游戏出错,
  whoever made them; the fix only ever changes RimArcho's own links or the plan, never the
  player's folders. Pack links that duplicate a copy raise `pack.links-left` to an error.
- Another version in the way (08 §3, §5.3): when a pack or a plan's locked copy needs one
  version and `Mods/` holds a plain folder with another, RimArcho offers to move that folder
  into the lock library and go on, with one confirmation listing what moves where and what is
  linked instead. Plans that used the folder keep it (it comes back through a link). The
  refusal names the fix: `--move-duplicates` on `save`, `launch` and `pack launch` (MCP
  `move_duplicates`), or `archo library move-to-locked <ref>`. Links, folders another tool's
  link points at, and folders with links pointing outside never move. `pkg.duplicate-mods`
  gets a 收进锁定库 action for the plain folder.
- A pack's `!ArchoPack_` links left in `Mods/` while the pack is not being played (a game
  started from Steam would load the pack's copies) are a problem of the plan,
  `pack.links-left`: in 能不能跑 › 要你确认 with 移除链接, in the tab under 启动游戏, in
  `archo problems` and MCP `list_problems`. `archo pack unlink [--dry-run]` / MCP
  `manage_packs action=unlink` remove only the links; refused while a game runs.
- `.archopack` format 2 (08 §5.1, §5.4): tar + zstd with `pack.json` first, a SHA-256 digest
  per copy and a whitelist that never lets game files in. Import verifies every copy.
  `archo pack check <name|file>` (MCP `manage_packs action=check`) verifies a pack's copies,
  or an archive without extracting it.
- A pack is compared with the game it is imported into or launched on (08 §5.5): game build,
  DLC and mods. The import result says whether every copy matched and lists the differences;
  launching refuses only on errors.
- The RimArcho list (`loadout export --format archo`, `.archolist.json`) shares a list with
  each mod's version; every list import reads it, takes the installed copy with the listed
  content and marks mods installed in another version. A pack's page exports its list (`E`).
- Packs remember the list they were made from.

### 贴图优化 (08 §5.2)
- Database schema v15 → v16: the DDS RimArcho wrote, per file and per mod.
- `archo textures status|optimize|clean|tool`: converts a mod's PNG textures to DDS next to
  them with todds 0.4.1 (BC1 when opaque, BC7 otherwise, with mipmaps); the PNGs stay. Every
  folder with `Textures/` is covered, and a path is converted everywhere or nowhere. Only the
  files RimArcho recorded are ever deleted; an author's own DDS is left alone. Refused while
  the game runs; can be stopped.
- todds is downloaded only after the player agrees (`tool --install`, settings), or
  `textures.todds` points at an existing copy (e.g. RimSort's).
- A mod updated after its textures were converted shows `texture.stale` in 需要处理.
- Uninstall gains a `textures` step that deletes the recorded DDS.
- `pack build --textures [--texture-scale N]`: 预转换 (full size) or 精简 (scaled) in the
  pack's copies, which then drop their PNGs. Space is checked against the peak; `pack.json`
  records what was done.
- Opening 贴图优化 or 创建整合包 no longer reads every texture: 扫描统计 does, and can be
  stopped. Changing the scope or scale keeps the last numbers and says to count again;
  converting works without counting (only new or changed PNGs).
- GUI: a 贴图优化 page (estimate, scale, progress, stop, clean), Mod 库存's 优化贴图 /
  清除贴图优化, the pack build's textures choice on 编译成整合包 (its own step 贴图: off / 预转换 /
  精简 80–25%, with the PNGs converted, the size after and the peak while converting; `D` steps
  through it; without todds the step says so and 编译 points to 设置 › 贴图优化); TUI palette panel; MCP `manage_textures`.

### Log corpus (03 §4.3, §4.3.1, §5.3, §6.3, §6.8; 04 §5; 08 §2)
The research on 6,734 public 1.6 logs (docs/research/), merged from `master`.
- **Severities from the corpus** (G8): each code graded by how often its mods are error
  sources in the same log against chance; see Breaking. `official.order`'s first suggestion is
  the cheapest `archo move` (a DLC enabled late goes back after the previous official package).
- **Constraint strength** (G2, `internal/order/strength.go`, `observed.json`): dependencies,
  loadBefore, forced orders and user rules are strong (warning); loadAfter and community rules
  weak (info); rules the corpus saw broken without effect harmless (info, with the counts and an
  `ignore` suggestion). Constraints that cannot be satisfied (two official packages against the
  official order, a "load before Core" that closes a cycle) are left out and listed under
  "not used" in `archo sort`.
- **Before you play** (`app.LaunchChecks`): `archo launch` and its dry run list the problems
  that most often end in red errors (`patch.no-match`, `def.parent-missing`, `incompat.active`,
  `dep.inactive`, `dep.missing`, `dep.observed-missing`, `combo.known-bad`), one row per code
  with why (the corpus numbers) and fix. It never blocks the launch.
- **The log corpus layer** (G10, `internal/known`): `dep.observed-missing` (an undeclared
  prerequisite seen in shared logs), `combo.known-bad` (a pair or a version that errors),
  `mod.known-error` (a mod's own error), evidence `community`.
- **Manual Harmony targets** (G9, `internal/harmony/manual.go`, `internal/dotnet/il.go`):
  `Harmony.Patch` calls and `(MethodBase, HarmonyMethod, …)` wrappers are read from IL; resolved
  targets are ordinary patches, unresolved ones say why. `HugsLib.dll` is read in its own mod.
- **Log attribution** (G1/G7): callers of failing code, HugsLib's log upload, Harmony 1.x
  references, field errors with their Context, XML Extensions blocks, absolute file paths,
  texture providers, missing assemblies loaded reflection-only, save-load holders. The cluster
  view shows the full message, its extra lines and the messages around it (`nearby`). The
  referrer search keeps using the reference index (03 §6.5).
- **Error patterns** (G5/G11, `internal/patterns`): `[pattern:g:<id>]` on cluster lines, a
  `pattern:` section in the cluster view (community background: usual causes, route, fixes,
  repeat-upload evidence), `archo pattern` and MCP `analyze_log pattern=`; MCP Instructions end
  with a checklist for save loading, missing assets, config errors, assembly loading and noise,
  and say tool results are capped at 24 KB.
- Privacy: pattern templates with HugsLib's `[Rimworld_dir]` paths are dropped (2 of 1,036).
- **Data from 40,326 logs** (corpus expansion): `internal/patterns` now has 3,947 mod-specific
  errors seen in at least 25 logs (was 1,034 from at least 10) and `internal/known` 95 prerequisites and 486 combinations (was 38 and 180),
  same thresholds. Severities (G8) and constraint strength (G2) keep their 6,734-log measurements.
- **In the pages** (06 §2.3, §2.4, §2.5, §2.10, §4.2; TUI the same):
  - 启动: the tab under 启动游戏 names the most likely trouble (the first `LaunchChecks` code
    with an item in 能不能跑) and its one-line reason with the corpus numbers; it opens
    能不能跑 and never blocks. The TUI's `L` says it on the status line (“留意：…”).
  - 能不能跑: a 社区日志 evidence chip with the measured share (“缺了它时 40/50（80%）出错”);
    `dep.observed-missing` with the dependencies, `combo.known-bad` with the conflicts,
    `mod.known-error` a dimmed “这是 Mod 自己的问题” note. Weak and harmless order
    preferences are one dimmed, uncounted item; counts and the sidebar badge leave them out.
  - 游戏日志 (and the Archo Monitor view): a cluster's detail shows the whole message, extra
    lines and nearby messages (none for an entry only ArchoMonitor.log has), then a pattern card (“这类报错通常是…”, common causes, next
    step, kinds of fix) marked as community background, only when a pattern matches.
  - 方案编辑 › 排序: the status line counts the strong requirements the sort put right; the
    sort preview (`archo sort`) has a `why` section, strong / weak / harmless, strong first.
  - Mod detail › 技术信息 › Harmony (`archo harmony --mod`): patches from `Harmony.Patch`
    calls are marked, and every unresolved call is listed with its reason.

### Background tasks (06 §2.16)
- Mod 库's file and Steam actions are queued once confirmed: two queues (files, Steam), one
  failure does not stop the rest, and each item's result is kept (`<data>/tasks.json`).
  Scans, checks, pack builds, texture work, game downloads and the long actions of the
  settings page and the guide are recorded as tasks too.
- 后台任务 page: a line at the bottom of the sidebar and `ctrl+j`. The CLI and MCP still run
  in the foreground.
- The window stays usable during long work (06 §3.1 rule 9): adding a large game, switching
  games, the first scan, folder sizes, and the settings page's downloads and database actions
  run in the background, and a page opens before its data is read; stale results are dropped.

### Interface rules (06 §3.1, §3.9, §7.3)
- Popups are only for warnings: new list, import, the build checklist and loading an older
  version happen in the page. A filter bar folds away when it loses focus.
- Packs, a pack's mods and the texture list are data tables. The gaps between cards can be
  dragged or moved with the keyboard, and their sizes are kept.
- Two corner radii, four font sizes no smaller than 12 px, hairlines, shared motion timings; no
  left colour bars (能不能跑, the TUI, selected rows), no `·` in interface text, no purpose
  lines under page titles. `TestStyleRules` and `TestWording` check this.
- Every path shown in the window copies on click, by mouse or keyboard (rule 12); text never
  overlaps the controls next to it.
- Back and forward between pages (rule 11): the mouse side buttons, `alt+←` / `alt+→` and
  BrowserBack / BrowserForward share one session history of the last 50 pages; back first
  closes a menu or popup. Deleted lists, other games' pages and finished guides are skipped.
  The TUI keeps its own page switching (06 §8.5).
- 启动 is a grid: the current list or pack as the title, then 01 game → 02 list / pack → 03
  launch. Choosing applies it, choosing again opens it; a switch shows its progress on the
  launch block instead of a popup. The launch tab's problems can be folded away until they
  change. The page shows the game's full version (e.g. 1.6.4871 rev590).
- RimWorld 1.6's own main-menu background is read from the game's Unity assets in the
  background and cached, falling back to the official preview image.
- 设置 › 界面: an accent colour (`ui.accent`, the 15 Radix Colors scales or default), applied at
  once in light and dark. The native title bar and the launch backdrop follow the theme.

### Lists (04 §1.6, §9.6; 06 §2.17)
- 列表管理 (the sidebar's 列表 › 管理, `shift+5`): the lists on the left; on the right a list's
  game versions, the Steam collections it follows and sharing, with an export sub-page
  (`e` in the editor opens it too). New lists are created in the page, and unfinished drafts
  are kept.
- A list's game versions (`archo plan versions [<name> [<versions…>|default]]`): by default the
  versions every mod supports plus the game it was made on, as one range. Copies of the list
  and RimArcho list exports carry them. When the current game is not among them, 能不能跑's
  要你确认 says so (add 1.7, or go to 列表管理).
- Following a Steam collection (`archo plan follow <name> <link|ID> [--mode add|follow]`,
  `plan unfollow`, `plan sync`; `collection --import --follow`): RimArcho keeps a snapshot and
  tells you what changed since; it never changes the list by itself. Catching up is one undo
  step: `add` enables the new installed mods, `follow` also disables the ones the collection
  dropped. The network is used only when asked (列表管理 checks at most every 10 minutes).
- Export format 工坊链接: Markdown Workshop links, read back by Workshop ID on import.
- Load cost (`archo plan cost [<plan>] [--mods] [--json]`, 列表管理): a RAM and VRAM range for a
  list from its textures, sounds, XML and assemblies. 1.7 is calibrated on the real game;
  1.6 follows the loading rules only and says so. Results are cached per mod.
- Sorting puts the installed base game back when it is missing; the 未启用 area lists Core and
  the DLCs first.

### Subscriptions (02 §3.4, 06 §2.9)
- Mods from a Git repository (`archo subscribe git <url> [--branch|--tag|--commit]`), from a
  download page such as LoversLab (`subscribe page <url> [--git] [--from zip|folder]`: the
  page's Git repository if it has one, else the files the player downloaded; never downloaded
  automatically), and Workshop items through Steam with SteamCMD as the fallback when the
  Steam client is not available (`subscribe workshop <id|url…>`). Where a mod came from is
  kept in `.rimarcho/source.json`. `subscribe list|check|update|remove`; updates are refused
  while the game runs; removing warns when lists use the mod.
- Mod 库 › 订阅与更新 replaces 变化: a scope bar (lists / all mods), one list of 等你更新 then
  已经更新 (the change records), new and gone, earlier; a source summary line at the bottom;
  the selected item's details (update, choose files, open the source page, lock). Mod 库 ›
  添加订阅 takes a pasted link, recognises Git / LoversLab / another page / Workshop, asks one
  question and shows the dry run.
- Mod 库 gains a source column, the source-update state, fields and actions; MCP
  `list_library` knows the new sources and states.

### Accounts (07 §3.1)
- 设置 › 账户: one status line each for SteamCMD and LoversLab (log in, log in again, log out).
  SteamCMD's login runs in SteamCMD's own console and is verified, and RimWorld ownership is
  checked. LoversLab signs in in a separate WebView2 window (its own process, a throwaway
  profile); only the site's cookies are kept, encrypted for the current Windows user, so
  downloads that need a login work. Passwords are never asked for or stored. Uninstall
  forgets the accounts first.

### Searching files (03 §5.5, 05 §4)
- `archo files search <pattern>` / MCP `search_files` search only what the game loads: the
  enabled copies in load order, the version or LoadFolders branch for this game version, and
  the content folders (Defs, Patches, About; Languages, Textures, Sounds, Assemblies when
  asked). Results are grouped by mod and file, carry the enclosing def or patch XPath, and are
  paged. `--def` classifies hits (def, abstract, child, patch, patch value, translation,
  reference). Literal text or `--regex`; no index.

### Game test runs (08 §12)
- `archo test run [<plan>] [--map] [--timeout]` / MCP `test_run`: starts the game on isolated
  data with default settings, waits for the main menu (or a quick test map), stops it and
  reports the startup errors by mod (`report.json`, the last 10 runs kept). Missing mods and
  broken links stop it before the game starts.

### Game links (08 §3)
- `archo links check [--all]` / MCP `check_links` find RimArcho's `_Archo_` / `!ArchoPack_`
  links in every known game's `Mods/` that are dangling, point at the wrong mod, cannot be
  checked, or whose name a real folder holds (`mods.link-dangling`, `mods.link-foreign`,
  `mods.link-check-failed`, `mods.link-occupied`). A dangling link or failed check in the
  selected game blocks writing, launching and test runs; other games are only summarised.
- `archo links repair --dry-run|--yes [--include <path>]` / MCP `repair_links` (one
  confirmation in the window) remove only RimArcho's own links, after showing the game, the
  full path, the target, the reason and the ownership evidence. Links without evidence need
  `--include`; real folders and unknown entries are never touched; refused while that game
  runs.
- Moving, recycling, locking or removing a mod or pack first takes down every known game's
  links to it, then puts the needed ones back; each step goes into the link ledger (schema
  v18). An interrupted operation shows as `mods.link-recovery` and finishes through links
  repair. Links to a known mod count as RimArcho's own, old IDs too.

### Managed directories (06 §2.14)
- 设置 › RimArcho 管理的目录 lists the game folders, Workshop, SteamCMD, other mod folders, the
  lock library, packs, Archo Monitor and the SteamCMD program. Each can be relocated (you moved
  it) or moved by RimArcho (a verified copy across drives, the old folder to the recycle bin).
  Mods keep their identity, tags and lists, and links follow. Every start checks them and asks
  where a missing one went; 能不能跑 and the launch check only warn.

### External tools (07 §2.1)
- `archo deps`, `deps install <todds|steamcmd|wezterm>`, `deps remove <name>`: one place that
  finds the bundled font, todds, SteamCMD and WezTerm. 设置 › 外部工具 installs and updates them
  as background tasks (`wezterm.path`, `steamcmd.path`). Only RimArcho's own copy is removed,
  and SteamCMD not while it holds Workshop content, a game or a mod folder.

### Other
- Unsubscribing or recycling a mod that lists use only warns: the confirmation names the lists,
  and their entries then show as missing. Unsubscribing is still refused while the game runs.
- Mod detail: 不列为副本 separates a copy from its lineage and keeps its player data.
- Faster on 1,349 mods: a list edit 1.3 s → 16–30 ms, a detail 650 → 6 ms, 需要处理 730 → 11 ms,
  首页 800 → 125 ms (link checks no longer resolve every mod; a list edit no longer reloads the
  index). Folder sizes, scan fingerprints and texture summaries walk folders in parallel, with
  fewer workers on hard disks (the whole Workshop's size 5.05 → 0.75 s).

### Development
- CI on GitHub Actions (windows-latest; `master`, `dev/**` and pull requests): gofmt, `go vet`,
  staticcheck (`staticcheck.conf`) and `go test`.
- Tests that touch no process-wide state run in parallel, and the test App reaches only
  loopback addresses (no real Steam API). `release.ps1` runs `go test -short -count=1`.
- `scripts/vm`: build, push and drive the real window in a test VM over DevTools; `smoke.ps1`
  prints PASS / FAIL. `RIMARCHO_DEVTOOLS_PORT` exists only for this.
- `scripts/vm/round.ps1` plays a whole game round in the VM with no screenshots: new colony,
  play, save, load, main menu and quit. A test-only driver (`scripts/vm/drive`) runs inside
  the game for that one launch. The script then checks Archo Monitor's phases and the save
  name, and that the game ran the Monitor RimArcho bundles. It prints PASS / FAIL.
  `cdp.mjs text` reads a page's text.
- `vm.ps1 build` builds Archo Monitor from `monitor/` (as `release.ps1` does) instead of
  copying an older one from `dist/`, and names the bundled version. `-Monitor <dir>` bundles
  another one. `vm.ps1 quitgame` closes the game as its window's X does and checks the
  Monitor's exit line. `status` says when the VM is off, and `cli` shows archo's progress
  lines as plain text.

### Known gaps
- Command output still says "unsaved" in a few places (`1 change unsaved`).
- `collection <id> --import` replaces the list with only the collection's mods, dropping Core.
- TUI: no subscribe actions, 后台任务 page or 列表管理 page yet (the CLI and the window have
  them).
- MCP has no write tool for subscriptions yet (`archo subscribe` only).
- Followed collections are checked when 列表管理 opens, not yet on a schedule.

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
