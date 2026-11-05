# 07 · Plans

> Status: 2026-10-21. Implemented: plan editing, multiple plans, history, write rules, writing on launch, sorting, rules, community data, locked versions (lock library), list exchange, player data and RimCrow import, and attributing outside changes to the writer. Added 2026-10-23: allowed game versions (§1.6), following Steam collections (§7.6), "Workshop links" export (§7.3); implemented (CLI, MCP, GUI). On 2026-10-25, plans became shared across all games (§1.1, §1.7); implemented (schema v19).
> Basis: 00, 00a, 01, 04 §2–§3, 05 §4, 13 §4, 16 §2.5, 18–17 §6, 09 §2–02 §3.

A plan is an ordered enabled list. This document covers storage, editing, rollback, writing to the game's `ModsConfig.xml`, automatic sorting, locked versions, player data and list exchange. Launch flow: 09 §2; UI: 16 §2.5.

## 1. Plan model

### 1.1 What a plan is

- Plans are **shared by all games (environments)** (decided 2026-10-25, §1.7). Every game shows all plans; switching games does not make lists "disappear". The first plan is `default`. Names are case-insensitive.
- Content: an ordered list of entries, each an Instance or a **missing entry** (a packageId that cannot resolve to an Instance, preserved as-is, §4.2).
- Plans store Instances, not packageIds: the player chooses "this copy" (00a §4).
- All plans share the game's default saves and settings; plans have no own saves. Build a pack for own saves (10).
- Edits **persist immediately** in RimArcho's database, surviving program closure; every step is undoable (§2.1). "Save" is separate, occurring only on plan launch (09 §2) or `archo save` (§4.1).
- Names are unique across all plans. A named list is a plan; naming a version no longer exists (`loadout tag` removed).
- Each environment remembers one **current plan** (any plan): the editor's plan and the default target of CLI/MCP commands. Two games may share a current plan.
- User rules (08 §2.1) and player data (aliases, tags, notes, §6.1) belong to Lineages and are shared across plans; plans decide only what is enabled and in what order.

### 1.2 Data owned by each plan

Workspace, undo stack, version history and staging belong to the plan (schema v11). Identity confirmation merging Mods moves these records to the retained ID under 04 §2.4.

| Data | Content | v10 storage | v11 storage |
| --- | --- | --- | --- |
| Plan | Name, creation environment, created time, last modified, last launched | None | New `plan` table (`id`, `environment_id`, `name`, `created_at`, `updated_at`). Since v19, `environment_id` means only "game where created", not where usable; names globally unique (application-enforced) |
| Workspace | Current list; originating version (`origin_version`, `origin_kind`: `import`, `external`, `checkout`, `edited`, `save`); unloaded outside version (`conflict_version`, §4.3) | `workspace`, one row per environment | One row per plan |
| Undo stack | `undo` and `redo`, at most 100 steps each; each stores the complete pre-edit list, staging (entries, order, original positions and anchors), and operation name | `workspace_undo`, per environment | Per plan |
| Version history | `v<n>`: time, source (§3.1), complete list, game-version string, notes | `loadout_version`, numbered per environment | Numbered per plan |
| Staging | Staged Mods and original positions (§2.2) | `stage:<environment ID>` in `meta` | `stage:<plan ID>` |
| Current plan | One per environment | None | Field on environment |

Data not owned by plans:

| Data | Owner | Notes |
| --- | --- | --- |
| ModsConfig.xml write record | **User data directory** | Last written hash, last seen hash, writer (plan and version), write time (§4.3). `modsconfig_state` has one row per user data directory, with writer (plan, version number). Writer means "what the file currently contains": the written version after saving; the `external` version after loading or recording outside changes (otherwise the next outside change would be falsely classified as a conflict) |
| Launch records | Environment | `launch` records environment, time, method, command line, plus plan, list fingerprint and version number (plan empty for packs); filled on launch (09 §2) |
| User rules | Lineage (stored per environment) | `user_rule`, 08 §2.1 |
| Player data | Lineage | `lineage_data`, `lineage_tag`, §6.1 |

### 1.3 Managing plans

| Action | Rules |
| --- | --- |
| Create | From an empty list, current-plan copy, historical version, imported list (§7.1), or save's Mod list (12 §4). First version source `import`, notes identify origin (copies: "copied from <plan> v<n>") |
| Switch | Changes only the environment's current plan, never game files. Current plan is used for launch, game writes and CLI/MCP plan commands. **Opening is not switching** (decided 2026-10-25): GUI sidebar, command panel and all lists, and TUI `P`, command panel or editor `tab` (since 2026-10-31), only view/edit the plan in this window (session's "open plan", `App.OpenPlan`), leaving current plan unchanged. Selecting it on launch, clicking "make current", saving from its editor or clicking launch makes it current. Newly created plans are only opened too. CLI `--open-plan <name>` targets another plan for one command without switching (used by GUI analysis/pack-build children). Actual game content follows the user-data-directory write record, displayed as "in the game now: <list> v<n>" (GUI: green dot on that sidebar child when multiple plans exist, 16 §2.5; TUI: status bar) |
| Rename | Name only; history unchanged |
| Delete | Player confirmation required; cannot delete the last plan. Delete its workspace, undo stack, versions and staging too; games using it as current switch to the oldest other plan |

GUI centralizes these actions on all lists (16 §2.17). Commands: `archo plan list`, `archo plan new <name> [--from current|v<n>|<file>]`, `archo plan use <name>`, `archo plan rename <old> <new>`, `archo plan remove <name>`. Other CLI/MCP plan commands (`enable`, `sort`, `loadout log`, etc.) target the current plan (13 §4).

### 1.4 First use

- When an environment is first read without a current plan (`environment.current_plan = 0`):
  - If ModsConfig.xml still matches a plan's last write (hash equals write record), make that plan current; create none.
  - If ModsConfig.xml is absent (for example, a new game with own saves) and plans exist, make the most recently edited plan current; create none.
  - Otherwise read ModsConfig.xml and create a plan, v1 source `import`, note `ModsConfig.xml` (absent file: empty list, note `no ModsConfig.xml`). Name `default`; if taken, environment name, then numbered (`1.6 2`). First-run guide also offers "move over from RimSort or RimCrow" or "open a pack or a Steam collection" (16 §2.15).

### 1.5 Archo Monitor added by default

Archo Monitor is RimArcho's bundled in-game helper Mod (06 §7; installation/updates: 09 §6). **On by default** means RimArcho adds it to plans; afterward it is an ordinary Mod.

- **Setting**: `monitor.default` (`on` / `off`, default `on`): `archo config set monitor.default off`, or settings "game log" group (16 §2.14). Controls automatic addition only, not updates (09 §6).
- **Plans affected**: new plans (all origins: empty, current-plan copy, history, imported list or save), and every existing plan in the environment when RimArcho first installs Monitor (upgrade). Check timing: "When to check" in 09 §6; new plans immediately after creation.
- **Once per plan**: record processed plans in meta `monitor.added:<plan ID>` (added, or already present). Players may then disable, move or stage normally; RimArcho does not re-add it except during sorting (next item).
- **Sorting adds it too** (2026-10-28): if enabled in settings, absent from plan and installed, every sort (`archo sort`, editor, can it run) places the latest version after Harmony in the same undo step. Re-add even if the player removed it; disable the setting to avoid this. If impossible (no Harmony in plan, `Mods/` cannot hold links), sort normally and give the reason (`SortResult.MonitorSkipped`, GUI notice and `archo sort`). Test: `TestSortAddsMonitor`.
- **Position**: immediately after Harmony (`brrainz.harmony`). Monitor About.xml declares `loadAfter brrainz.harmony`, `loadBefore Ludeon.RimWorld`; sorting (08 §1) keeps it after Harmony and before Core to observe Core and all Mod loading.
- **Neither add nor mark processed** (retry next check) if setting `off`, no Harmony (required), `Mods/` cannot hold links (FAT, exFAT, 09 §6), or no installed Monitor.
- **One reversible edit**, named `add Archo Monitor after Harmony (default; archo config set monitor.default off)`. Also applies to noncurrent plans: record in that plan's own undo stack; `archo undo` there reverses it. Edit result explanations (CLI output, UI notices) list affected plans. Tests: `TestMonitorInstallAndDefault`, `TestMonitorDefaultOff`.

### 1.6 Allowed game versions

Decided 2026-10-23, implemented. Each plan records **allowed game versions** (major/minor, such as 1.5, 1.6, 1.7), answering "which versions is this list intended for?" UI: all lists (16 §2.17).

- **Default**: intersect all Mods' declared `supportedVersions`, add the game major/minor at creation, then take the continuous range from lowest to highest. Example: intersection 1.5, 1.6, created under 1.7 → default 1.5–1.7.
  - Exclude official content (Core/DLC declare only their own version), missing entries and Mods without `supportedVersions` from the intersection.
  - Empty intersection: use only creation version.
  - Defaults are not stored; recalculate from the current list on every read, so enabling/disabling Mods changes them.
- **Player overrides**: store the chosen set, which need not be continuous. It no longer follows Mods until reset.
- **Storage**: `plan.created_game` (creation game major/minor) and `plan.versions` (comma-separated selection, empty = default).
- **Copying plans** copies the player's selection. **Import** uses `versions` from RimArcho lists if present (§7.3), otherwise defaults; `created_game` is the current environment version.
- **Uses**:
  - Show plan versions on launch and all lists (16 §2.3, §2.17).
  - If current game is outside the allowed set, "for you to confirm" in "can it run" shows "<list> allows 1.5–1.6; this game is 1.7", with "add 1.7" and "go to all lists" actions. Does not block launch.
  - Exported RimArcho lists include `versions` (§7.3).
  - `version.unsupported` (05 §4.3) still checks current game version, independently.
- **Commands**: `archo plan versions [<name>]` shows allowed versions and the count of Mods not declaring each; `archo plan versions <name> <versions…>` sets them, e.g. `1.5 1.6 1.7` or `1.5-1.7`; `archo plan versions <name> default` resets. Version settings are not list edits and do not enter undo stacks; UI status offers "undo", which restores the previous value. MCP has corresponding tools.
- **Warning counts**: `PlanVersions.Missing` and version-option notices in all lists exclude ignored version warnings and list `Muted` separately (05 §4.2). If all ignored, say "no version warnings", never "every Mod supports it". `Common`, defaults, allowed versions and player selections still use actual declarations; ignoring warnings does not count as support.

### 1.7 Plans shared across all games

Decided 2026-10-25, implemented. Player feedback (translated): "I created three lists in 1.7, switched to 1.6 and saw only `default`; I thought the lists were lost." Plans are just "what is enabled and in what order", independent of game directories. Allowed game versions (§1.6) already describe intended versions, so plans no longer belong to environments.

- **List page, sidebar and launch page** show all plans without grouping by game. Show allowed versions normally; "can it run" warns if the current game is outside them (§1.6), rather than hiding plans.
- **Reading entries in another game**: plans store Instances. Shared-folder Instances (Workshop, SteamCMD, extra folders, lock library, Monitor) are global and recognized by all games. Another game's own Instances (Core/DLC in its `Data/`, its `Mods/`) read as missing packageId entries, then resolve to this game's copy under §4.2 (`app.localizer` + `Registry.Heal`); absent copies remain missing. Reading does not modify storage. Editing here stores this game's Instances; returning resolves back by packageId in the same way.
- **Links on game switch** (03 §2 item 14): Mods from the previous game's `Mods/` in the current plan still link into the target game as `_Archo_env_<Instance ID>`.
- **Other games' `Mods/` are usable in every game** (decided 2026-10-25): each game index also loads live Mods from other games' `Mods/` (excluding `_Archo_*` links), with in-memory source type `elsewhere` (`source.Elsewhere`, not stored; label `local-<environment name>`, `app.withElsewhere`). They appear in mod library (source `local-<environment name>`, location that game's `Mods/`, 03 §2 item 11) and editor Disabled. Once enabled, they deploy like SteamCMD/extra-folder Mods: save links them into this game's `Mods/` as `_Archo_env_<Instance ID>` (`DeployName`), retained on save/disable (same links as game-switch links). If this game already has an Instance of the same folder (a previously scanned link became a local Instance), show only this game's copy. If this game's `Mods/` has the same package ID, read the other game's plan entry as this game's copy (`Index.Elsewhere`, "retain target copy with same package ID" in 03 §2 item 14). Read-only Data/ is not shared across games. Test: `TestOtherGamesModsUsableAnywhere`.
- **Write records** also store the written game (`modsconfig_state.writer_env`). Resolve outside changes in that game (§4.3); "in the game now" includes its name when another game sharing the directory wrote it.
- **Deleting an environment** preserves plans: rewrite entries referencing its own Instances in workspaces, versions and undo steps to packageIds before deleting Instances (`store.keepLists`).
- Tests: `TestPlansSharedAcrossGames`, `TestPlanStore`.

## 2. Editing

### 2.1 Operations

Every operation is a command shared by GUI, TUI, CLI and MCP (13 §1). Each operation changing the list or staging is one undo step; record none if neither changes.

| Operation | Command | Rules |
| --- | --- | --- |
| Enable | `archo enable <refs…> [--at <n>\|--before <ref>\|--after <ref>]` | Check loadability first (§2.3); explain refusals. Notify and skip already-enabled Mods. With an explicit position, insert multiple Mods consecutively in supplied order. Otherwise replace a missing entry with the same packageId in place; if none, place individually under sorting constraints (`order.Place`, same as editor suggestions `App.SuggestAt`, 16 §3): immediately after the last enabled Mod it must follow (not before official content); if only enabled Mods require following it, before the first of those (libraries, dependencies, DLC; later-enabled Odyssey thus returns to official order); unconstrained at end; `attach` immediately after target. Settled by corpus replay on 2026-10-26: remove DLC and 3 random Mods per log, then re-enable. This rule restores 95% of DLC and 17% of other Mods to original positions (median distance 31); "latest position constraints permit" gives 15% (median 35), always append gives 0% / 2% and other Mods violate 83,000 strong constraints (739 with this rule) |
| Disable | `archo disable <refs…>` | Accept `pkg:<packageId>` to remove missing entries (with or without `_steam`). Notify if already disabled |
| Move | `archo move <ref> --to <n>\|--before <ref>\|--after <ref>\|--up\|--down` | Require one position form; error for disabled entries, suggesting `archo enable` |
| Swap instance | `archo swap <ref> [<ref>]` | Replace an enabled Instance with another in the same Lineage, preserving position. References may be in either order. With one reference, use the sole other loadable Instance; if several, list choices. Different Lineages error, suggesting `archo lineage merge` or disable then enable |
| Sort | `archo sort [--dry-run]` | 08 §1; `--dry-run` only displays results |
| Stage | `archo stage add/drop/restore/list` | §2.2 |
| Undo / redo | `archo undo [<n>]`, `archo redo [<n>]` | Up to 100 steps per stack, shared by all interfaces. New edits clear `redo`. `checkout` (§3.2) and import replacement (§7.1) are reversible edits too |
| History | `archo history [--all]` | Unsaved edits and versions (§3.3); latest 10 versions by default |

- Two processes (e.g. GUI and MCP) may share one RimArcho data directory (`%APPDATA%\RimArcho`, 17 §3). A database change key (`data_version` + `total_changes`) change makes the next read see the other process's changes; stale-view edits do not overwrite them. Test: `TestTwoProcessesShareTheWorkspace`.
- Undo/redo snapshots preserve the list's original stored Instance bindings (including Core, DLC and game-owned Mods), not current-game-resolved copies. Restore them unchanged, then resolve for display in the current game. Edit batches follow the same rule.
- Plan reads are cached until the database or ModsConfig.xml (size, modification time) changes.

**SteamCMD batch handoff**: after the player accepts credit replacement (11 §1), replace original Instances in lists with corresponding workshop Instances, preserving position, only while the game is stopped and every environment able to load the old copy can load the new one. Current content, historical versions, undo/redo, staged entries and position anchors follow in one transaction. Handoff creates no undo step that would restore the old copy; historical references alone do not retain it. If any environment fails the condition, retain the entire item and change no shared lists; corrupt staging likewise ends that item's handoff with retention. Fixed packs, game deployments not yet handed off and incoming links retain original identity/copy with reasons. Existing game projections reuse §4's writer and registry for `_steam`, replacing only this item and detaching only its owned deployment link, without committing other unsaved edits or switching the player's current list. Merge notes inside the remapping transaction. DDS ledgers do not migrate across physical directories; results suggest optimizing again. List edits and handoff share the launch lock; hold it from read through save. Late snapshots or long-lived edit batches conflicting with handoff must reread, never write old IDs back. Persist deployment obligations in SQLite first so work can resume after exit. Full flow and collection conditions: 11 §1.

### 2.2 Staging

Staging is one persistent list per plan that is **never written to the game**. Two uses:

- **Preparation**: hold newly subscribed Mods until ready to enable.
- **Troubleshooting**: move a batch from Enabled into staging, then restore original positions in one action.

Rules:

- Staging contains only **disabled** Mods. Re-enabling a staged Mod (enable, undo, checkout or import) persistently removes it from staging in the same transaction; disabling later does not bring it back.
- A Mod staged from Enabled remembers its original 1-based position and preceding Instance (anchor; empty if first or preceded by a missing entry, falling back to position). If the anchor was itself staged earlier from between entries, advance the anchor to that Mod so restoration preserves their original order in the middle.
- Mods staged from Disabled have no original position (0).
- Restore after the anchor; if the anchor is staged, follow the anchor chain; if none found, use original position. For batch restoration, restore anchors in the batch first, others by original position; entries without positions append in staging order. Bound anchor traversal to prevent cycles after editing.
- With position arguments (`--at`, `--before`, `--after`), restore only one Mod at that position.
- Check loadability before restoring (§2.3).
- Each staging add / drop / restore is one reversible edit, including staging from Disabled or removing without enabling. Each step records full entries and staging state (entries, order, original positions, anchors); undo/redo restores both in one transaction. Other list edits also record staging so undo can restore items removed by enable, checkout or import. Combined batch edits follow the same rule.
- Old undo steps without staging snapshots restore only entries, retain unrelated staged items, and persistently remove staged items enabled in the current or target list. Do not infer old positions/anchors; subsequent reverse steps save full snapshots.

| Command | Meaning |
| --- | --- |
| `archo stage list` | Staged Mods and original positions |
| `archo stage add <refs…>` | Stage; enabled Mods leave the list and remember position. One reversible edit |
| `archo stage drop <refs…>` | Remove from staging without enabling. One reversible edit |
| `archo stage restore <refs…> \| --all [--at\|--before\|--after]` | Restore; `--all` restores everything; refuse if neither references nor `--all` supplied (13 §3.3). One reversible edit |

Tests: `TestStageRestoreAllKeepsPositions`, `TestStageRestoreOneFollowsAnchor`, `TestStageRestoreAllRandomBatches`. Staging column and keys: 16 §2.5.

### 2.3 Loadability

An Instance admitted to a plan must be live, not residue or `unloadable`, and visible to the game or deployable by RimArcho:

- In `Data/`, `Mods/` or subscribed Workshop content; aliases there count (e.g. SteamCMD directory junction in `Mods/`). RimArcho-owned `_Archo_` links do not count here;
- Or **deployable**: source `steamcmd` or `extra`, linked into `Mods/` with `_Archo_` on game write (09 §3).

Refusal explanations for `enable`, `swap`, `stage restore`:

| Condition | Explanation |
| --- | --- |
| Tombstone | No longer on disk |
| `unloadable: about-disabled` | About.xml was renamed; the game cannot read it (handling: 09 §5) |
| Residue | Workshop folder is no longer subscribed; game ignores it |
| Source neither read by game nor deployable | Game does not read Mods from `src:<label>`; link or copy into `Mods/` |
| Another copy with same packageId at same location | `pkg.duplicate-installed`: first by folder name wins; game ignores this copy (05 §4.3) |
| Workshop and non-Workshop copies of same packageId | Cannot enable both (game retains only unsuffixed copy, §4.1 item 4); suggest `archo swap <enabled> <this copy>` |

## 3. History

### 3.1 Versions

Each plan version `v<n>` records time, source, complete list, game-version string and notes. Numbers increase within a plan.

| Source | Meaning |
| --- | --- |
| `import` | Initial list from outside: first ModsConfig.xml read (§1.4), or new plan from list file, share code, collection, save or copy (§1.3, §7.1). Notes identify origin |
| `external` | Outside changes: game Mod menu or another tool changed ModsConfig.xml; attribute to its last writer plan (§4.3). Note `ModsConfig.xml changed outside RimArcho`. Also serves as "startup snapshot": outside changes discovered on RimArcho startup use this source, not a separate one |
| `user` | List written to game by `archo save` (UI "save") |
| `undo` | Historical version checked out then written without further editing. Note `restored v<n>` |
| `launch` | List written on plan launch (09 §2). If identical to plan's latest version, create none; launch record points there. Launch after checkout without editing gives new version note `restored v<n>` |

Undo steps are not versions: versions are lists "written to or read from the game"; undo steps stay inside RimArcho.

A plan's **unsaved changes** are the difference between latest version and workspace. This is informational and blocks nothing; launch writes the current list (09 §2).

### 3.2 Commands

| Command | Meaning |
| --- | --- |
| `archo loadout log [--all]` | Current plan versions: number, time, source, entry count, notes; mark unloaded outside versions |
| `archo loadout diff [<v>] [<v>]` | Four categories: added / removed / moved / instance swaps. Default latest version versus workspace. Versions accept `v3` or `3`; workspace accepts `workspace`, `ws`, `current` |
| `archo loadout checkout <v>` | Load a version into workspace as one reversible edit; no game write. Immediate game write afterward uses source `undo` |

If the loaded version, resolved for the current game, exactly matches the workspace list, it is a no-op: no origin, conflict, staging or undo/redo changes. UI says only "no changes", without offering undo.

Diff algorithm: changed Instance within one Lineage is an "instance swap". Compute longest common subsequence of remaining shared entries; only entries outside it are "moved". Moving one Mod thus reports one move, not moves for entries whose positions were unaffected.

### 3.3 History area logic

UI: third column in plan editor (16 §2.5). Data and behavior:

- **Unsaved steps**: undo stack, one row per step, newest first. Cursor marks current state; `u` / `ctrl+r` moves it. Selecting and confirming an earlier step undoes through it (`StepTo`), stopping at stack bottom if insufficient. Clicking an unsaved step applies immediately; `u` / `ctrl+r` can still return.
- Titles derive from before/after lists (e.g. "enable Harmony · position 2"), not separately stored.
- **Versions**: one row per version with source. Selection shows diff against workspace; confirmation runs `loadout checkout`. Confirm before loading a version.
- Unloaded outside versions (§4.3) can also be diffed, loaded or ignored here.

## 4. Writing to the game

### 4.1 ModsConfig.xml write rules

Shared by **plan launch** (09 §2) and **`archo save`** (write without launching, for players launching via Steam etc.; UI "save").

1. **Synchronize first**: check outside changes before writing (§4.3), preserving existing outside content as a version rather than silently overwriting. **Refuse with unloaded outside versions.** If the target plan has one, or the file contains another plan's unloaded outside content, `save` and `launch` (including `--dry-run`) refuse without writes and give diff / checkout / ignore commands (05 I8). Writing would overwrite changes the player has not reviewed.
2. **Prechecks**: refuse environments without user data directory, suggesting `archo env set --user-data <directory>`; refuse unreadable existing files, asking the player to repair or move them. Write paths must pass safety-boundary checks (02 §1).
3. **Synchronize links**: before writing ModsConfig.xml, synchronize `_Archo_` links and remove pack `!ArchoPack_` links (09 §3).
4. **`activeMods`**: lowercase packageIds in plan order. When both Workshop and non-Workshop copies exist for a packageId, write `<packageId>_steam` if the plan chooses Workshop, otherwise unsuffixed `<packageId>`. Reason: game registers official → `Mods/` → subscribed Workshop, folder-name order within each group. Workshop duplicates of registered non-Workshop copies gain `_steam`; other duplicates are rejected (17 §6). Enabling both makes the game retain only the unsuffixed copy, so a plan cannot contain both Instances (§2.3).
5. Write **missing entries** unchanged. An Instance already in the plan but no longer registered by the game (deleted or disabled) is written as its packageId so it does not silently vanish. Automatically heal to an Instance when resolvable again (§4.2).
6. **`version`**: current environment game version, the full `Version.txt` string, as the game writes it; preserve original if environment has no game version. The game **discards the whole list** and resets if major/minor differs (17 §6), hence write the current version every time.
7. **Other content**: replace only `<version>` and `<activeMods>`, preserving `knownExpansions`, unknown elements, BOM and newline style; XML-escape text. If absent, create in game format, with installed official DLC in `knownExpansions` (live official Instances, excluding Core).
8. **Backup and atomic replacement**: back up original to `<user data directory>/Config/ModsConfig.xml.rimarcho.bak`, retaining only the latest (full history in RimArcho database). Write new content to a same-directory temporary file, flush, then rename over original.
9. **Record**: add plan version (`archo save`: `user` or `undo`; launch: `launch`, §3.1); record hash, writer (plan/version) and time against user data directory (§4.3); set workspace origin to this version and clear unloaded-outside-version marker.

Preview and execution:

- `archo save --dry-run` and command-panel "preview saving" show diff from latest version, version rewrite ("will change 1.6 to 1.7"), missing count, and links to create/delete. Preview is read-only, without "continue".
- UI "save" (`w`) executes directly like CLI `archo save`; status "saved as vN". Refusals in items 1/2 show reasons and commands without confirmation (16 §6). MCP allowance belongs to the host (14).

If read `version` major/minor differs from the game, status bar and `archo status` warn: "This list was saved for version X; the game will reset it on launch. Launch or save once from RimArcho to save for the current version." Blocks nothing (launch always writes current version, 09 §2).

### 4.2 Instance resolution on reading

When reading ModsConfig.xml (and imported lists, §7), resolve each entry in game order (17 §6):

1. Match full ID first. `<pkg>_steam` maps to a Workshop Instance only if a same-named non-Workshop copy also exists.
2. Otherwise match **folder name** and rewrite the entry to that Mod's packageId.
3. If still unmatched and ending `_steam`, strip and retry. This covers a deleted local copy leaving only Workshop.
4. For several Instances matching a plain packageId, prefer non-Workshop. With same-source duplicates (e.g. two in `Mods/`), game accepts only the first by folder-name order, errors and ignores others. Both cases yield `pkg.duplicate-installed` (05 §4.3).

- Duplicate Instance entries retain the first position only (game disables the later one).
- Unresolved entries remain **missing entries**, visible with `dep.missing`-style findings (05 §4.3), never silently discarded.
- **Healing**: a later scan finding a resolvable Instance automatically turns the missing entry into an Instance, without manual repair (unless already in the list).
- **Installed deployable copy**: unbound packageIds do not automatically select copies outside the game registry (e.g. another game's `Mods/`). `App.UseInstalled(packageIDs)` binds copies marked `CanEnable` in `MissingMods` in place, one reversible batch edit; duplicates retain the first position. Recheck missing status, loadability and enabled same-package conflicts at execution; reject the batch if any request is unavailable. Binding creates no links; save/launch deploys under 09 §3. UI contract: 16 §8.7.

These rules are based only on decompilation; local/Workshop same-package coexistence and `_steam` entries have not been verified in-game (01 §4).

### 4.3 Outside changes

Several plans and environments share ModsConfig.xml (default user data directory, 03 §3.4). RimArcho records last-written hash, last-seen hash and writer **per user data directory** (game, plan, version; game recorded separately because plans are shared, §1.7).

**Check timing**: every scan, GUI/TUI startup and command reading a plan.

**Decision**:

1. Hash equals last seen: unchanged.
2. Equals RimArcho's last write: update "seen" only.
3. Neither, but parsed list and `version` equal writer's recorded version (format-only changes, e.g. game rewrote file): not an outside change; update "seen" only.
4. Otherwise **outside change** (e.g. player edited the in-game Mod menu). Attribute to the **last writer plan** (resolve entries in its environment, §4.2):
   - Plan **unedited** since write (workspace still equals written version): load file as its `external` version and into workspace; undoable. Notify "ModsConfig.xml changed outside RimArcho: loaded as <list> v<n>". If Mods were removed (game resets to vanilla/DLC after Mod loading fails), append "; N Mods are no longer in the list (present in v<m>)". Add a "needs your answer" card in "can it run" listing removed Mods and actions "see the difference" (`loadout diff v<m> v<n>`), "restore v<m>" (one undoable step), "keep this change". Dismiss after player response, subsequent workspace edit or checkout of another version.
   - Plan **edited** since write: record only an `external` version, do not load; mark "unloaded outside version" and notify conflict. Player can:
     - **Diff**: `archo loadout diff v<n> workspace`;
     - **Load**: `archo loadout checkout v<n>` (reversible edit), clearing marker;
     - **Ignore**: `archo loadout ignore-external`, clearing marker but retaining history. (Marker blocks game writes, §4.1 item 1, so writing never reaches marker clearing.)
   - Show marker in `archo status`, `archo loadout log`, status bar and plan page.

- Without a write record (new user data directory or lost record), attribute to current environment's current plan and handle the two cases above; that plan becomes writer. Same if writer plan was deleted.
- First reading with no plan uses §1.4 import, not outside change. Import does not mark the file "seen", so a file written by another environment's plan then changed still attributes that change to that plan.

Implementation: `internal/app/external.go`, called during synchronization on every plan read. Markers in `archo status`, `loadout log`, "can it run" (with diff action), sidebar and plan page; also list markers on other plans, with `plan use <name>` or `--env <name>`. Tests: `TestExternalChangeAndConflict`, V6–V8.

## 5. Locked versions

A locked version fixes **one Mod** to specific content, unaffected by Workshop updates. It is not a new identity concept, just another Instance in the same Lineage (04 §3). Player-facing wording is "locked version"; `pin` creates it, `copy-local` remains an alias.

### 5.1 Two forms

| Form | Shape | Detection |
| --- | --- | --- |
| Created by RimArcho | Copy directory in **lock library**, with `.rimarcho/origin.json`; Workshop unchanged. Setting `lock.dir`, default `%LOCALAPPDATA%\RimArcho\locked`, shared by all environments (03 §2), registered as shared `extra` source (`src:locked`). Copies, like upstream, are global records usable by any game's plan. Pre-0.9.0 per-environment `<lock.dir>/<environment>` subdirectories are shared folders `src:locked-<environment>`; their copies are still locked versions. Game sees them only through `_Archo_` links (09 §3) | Scan reads `origin.json`, merges into same Lineage under R5, marks "confirmed" (04 §2.1). `origin_instance` is upstream. Older `Mods/` copies with `origin.json` still count |
| RimCrow style | Workshop `About/About.xml` renamed to `About.xml.disabled` (invisible to game), local copy in `Mods/` (often `_<WorkshopID>_`) | Same Lineage has an about-disabled Workshop Instance and another live, loadable non-Workshop Instance (RimCrow copy, `Mods/` copy or deployed copy all count; prefer enabled if several). Workshop is upstream |

- Locked versions produce `mod.pinned` (info, 05 §4.3), **no longer** `mod.unloadable`. List shows "locked": copy is effective, Workshop is "upstream".
- Recompute classification during analysis, not stored. About-disabled Workshop directories without copies are "hidden Mods" (09 §5), not locked versions.
- Pack copies also have `origin.json`, but scanning skips pack links, so they are not locked versions (10).

**Why lock library rather than `Mods/`.** For duplicate packageIds in `Mods/`, game registers only first folder-name order (17 §6). Two locked versions cannot coexist there; updating requires moving the old copy or renaming About.xml. Lock-library storage links only the current plan's selected copy into `Mods/`: old/new copies coexist, each plan chooses its own, without About.xml changes or manual work. Cost: if `Mods/` volume lacks junctions (FAT, exFAT), plans using locked versions cannot write to game; refuse with explanation under 09 §3.

### 5.2 Creating

`archo pin <instance> [--name <directory name>] [--git] [--dry-run]` (`copy-local` alias).

1. On first use, register lock-library source (check it neither contains nor is contained by data directory/source roots, 02 §1). Copy Instance there, default `<original directory>_local`; suffix if occupied (`_local2`, `_local3` …), **never overwrite**. `--name` must be a plain directory name (no separators, `.`, `..`, `<>:"|?*`).
2. Copy details:
   - Copy source real path (resolve links first).
   - Copy internal links by content; skip links outside the Mod or cyclic links, listing them in output.
   - Exclude upstream `.git` and existing `.rimarcho/`.
   - Preserve file modification times.
   - Assemble in `<target>.archo-tmp`, then rename into place. On failure delete temporary directory, leaving no residue.
3. Write `.rimarcho/origin.json`:

   ```json
   {"origin_instance": "<ID>", "origin_lineage": "<ID>", "workshop_id": "<ID>", "origin_snapshot": "<hash>", "copied_at": "<ISO time>"}
   ```
   `origin_snapshot` is the source snapshot hash at copy time (hexadecimal); `copied_at` is UTC.
4. With `--git`: `git init`, `git add --all --force` in the copy (include ignored files for a complete baseline), commit with fixed identity (`RimArcho` / `rimarcho@localhost`), `core.autocrlf=false`, `commit.gpgsign=false`, `--no-verify`, message `Baseline: copy of <name> (#<instance ID>)`. Missing git does not prevent copying; report git failure.
5. Rescan, merging copy into original Lineage.
6. Swap original for copy **only in current plan** (`swap`, one reversible edit; no plan change if original disabled). Other plans keep Workshop. On launch/save, copy links into `Mods/` through `_Archo_`; Workshop uses `<packageId>_steam` (§4.1 item 4). If `Mods/` already has another non-Workshop copy with the same package ID, do not create the link (09 §3: game errors on same-source duplicates). If an ordinary folder, offer one confirmation to move it into lock library too (`--move-duplicates`; its plans keep using it through links), then link this copy. If a link or targeted by another tool's link, do not move; refuse with explanation, suggesting using that copy or moving it manually. Never rename About.xml.
7. Output copy reference, path, file count, bytes and skipped links.

### 5.3 Upstream updates

- **RimCrow style**: Workshop snapshot differs from copy (both snapshot ID and semantic hash), and ACF `timeupdated` is later than copy's first appearance.
- **RimArcho-created**: upstream's current snapshot hash differs from `origin_snapshot` in `origin.json`.

Then upgrade `mod.pinned` to warning, saying "upstream has an update", with "see the changes" and "update the locked version":

`archo mod <instance> --diff <instance>` lists differing semantic files, `+ only in latter`, `~ changed`, `- only in former`, sorted by path. Compare about-disabled `About/About.xml.disabled` as `About/About.xml`. Write nothing.

### 5.4 Updating a locked version

Original move-to-library confirmation / CLI dry-run also lists every affected game, link path, original target and detach/rebuild destination. Detach all games' owned incoming links before physical movement; reconnect after updating Instance. Keep one effective entry per instance per game, never both env and locked entries. Refuse foreign incoming links and running games; failure/recovery contract: 09 §3.

`archo pin --update <instance> [--name <directory name>] [--dry-run]`: reference copy or upstream. Show diff first; create a new lock-library copy (new directory) under §5.2 and swap into current plan. Retain old copy (may contain player edits; this is why `--git` baseline exists).

**Old copies in `Mods/`.** Before lock library, RimArcho created copies there. Move such copies (only with `origin.json`) into lock library on first `pin --update` or `archo pin <copy> --move-to-library [--dry-run]`. Same volume: rename; cross-volume: copy (including `.git`, `.rimarcho/`), verify each file, recycle original. Refuse links pointing outside the copy. This command never moves directories without `origin.json` (including RimCrow copies); other backup copies in `Mods/` use mod library "move into the lock library" (`archo library move-to-locked`, 16 §2.9). Both are the same move: point Instance to new location, then scan, preserving ID and all plan references (previously replaced with a new Instance only in current plan). CLI executes without `--dry-run`; UI previews then confirms.

### 5.5 Unpinning

`archo unpin <instance> [--dry-run]`: reference copy or upstream.

- **RimCrow style**: rename Workshop `About/About.xml.disabled` back to `About/About.xml` (refuse without changes if About.xml exists), rescan immediately. Swap copy in current plan to Workshop if present. Leave copy where it is.
- **RimArcho-created**: swap current-plan copy back to upstream if present; retain copy in lock library.
- Current plan only; others unchanged.
- CLI executes without `--dry-run` (like `save`); UI previews then confirms.

Recycling Mod folders: 09 §5–02 §3.

## 6. Player data and RimCrow import

### 6.1 Player data

| Data | Owner | Description | Command |
| --- | --- | --- | --- |
| Alias | Lineage | Display name in all interfaces; `find`/search match it. Log attribution still matches About.xml name because the game prints `[Name]`, not alias | `archo alias <ref> [<name>] [--clear]` |
| Tags | Lineage | Multiple, case-insensitive; RimCrow groups import as tags | `archo tag add\|remove <ref> <tags…> [--mod <ref>]…`, `archo tag list`; filter `list --filter tag:<tag>` |
| Note | Lineage | Plain text | `archo note <ref> [<text>] [--clear]` |

Storage: `lineage_data` (environment, Lineage, alias, note), `lineage_tag` (environment, Lineage, tag). Lineage-owned, shared by original/locked versions and all plans.

### 6.2 One-time RimCrow import

`archo import rimcrow --db <path> [--rules <path>] [--dry-run] [--include-disabled-rules] [--all]`

1. `--db` required: RimCrow stores its database at `<installation directory>/data/mod_manager.db`, with no registry/config discovery. `--rules` defaults to `data\rules\user_rules.json` beside the database.
2. Copy database and WAL into temporary `rimcrow-*` directory under RimArcho data, open copy read-only, delete afterward. Never open RimCrow originals for writing.
3. RimCrow player data uses packageId. First map resource paths (absolute → source + relative path) to Instances, then packageId on failure. If packageId maps to several Lineages, do not import; report it.
4. Mapping:

   | RimCrow | RimArcho |
   | --- | --- |
   | Alias, note | Alias, note; do not overwrite differing existing values, report conflicts |
   | Groups, tags | Tags |
   | Interlock | `attach` rule |
   | `user_rules.json` loadAfter / loadBefore / incompatibleWith | `after` / `before` / `incompatible` |
   | `user_rules.json` loadTop / loadBottom (`value` true) | `top` / `bottom` (08 §1.5) |
   | sign_color, user_mod_type, ignored_issues | No equivalent; count only |

5. **Disabled** RimCrow user rules are excluded unless `--include-disabled-rules`; silently importing would alter sorting.
6. Report mapped counts (path/packageId), success, existing, conflicts; list every unmapped/conflicting item, RimCrow-disabled resource count. Count About.xml.disabled-only directories by source, with handling guidance (rename back or re-enable in RimCrow then `archo scan`; alternatively unsubscribe/delete; RimArcho handling: 09 §5).
7. Repeatable with identical results: skip identical values, deduplicate rules by kind + Lineage. No log-table import needed.
8. `--dry-run` reports only, writes nothing. Default 6 details per category; `--all` shows all.

One local import has run; results: 18.

## 7. List exchange

### 7.1 Import destinations

Every imported list (file, pasted text, share code, Rentry link, Steam collection, save Mod list) is previewed, then offers three choices:

- **import as a new list**: new plan with imported list, v1 source `import`, origin note (filename, `share code`, `collection ws:<ID>`, `rentry <ID>`). Default name from list (share-code name, collection title), otherwise filename. Command `loadout import --new` (13 §4.2), MCP `manage_plans action=import`.
- **replace the whole list**: replace whole list as one reversible edit; no game write.
- **add the ones you lack** (merge): add imported Mods absent from current plan at constraint-suggested positions, preserving existing entries; one reversible edit.

None writes ModsConfig.xml.

Steam collection import adds a checked-by-default "follow this collection from now on" checkbox below preview, linking the collection to the new/current plan afterward (§7.6).

Command: `archo loadout import <file|share code> [--new [<name>]]`; without `--new`, replace current plan.

Preview: format, source, list name, game version (warn "the list is for RimWorld X, this game runs Y" if major/minor differs), total, installed/uninstalled counts. Uninstalled entries get Workshop links (supplied IDs or steamDB lookup) and replacement suggestions (08 §2.3). Show added/removed/moved/swapped counts for replacing current plan. RimArcho lists (§7.3) include per-Mod versions: installed entries without a local matching version show "another version"; append "N in another version" to total; expand installed group by default.

Resolution: lowercase packageIds; retain `_steam` tokens and resolve under §4.2; unresolved become missing entries. Workshop-ID-only entries map to installed Instances' packageIds (prefer loadable); absent local copies do not become missing entries (no packageId), but show Workshop links in preview/results. If a list contains no official content (Core/DLC), replacement and new-plan import retain current plan's enabled Core/DLC at the front; if none, add installed Core (2026-10-30). Applies because Steam collections, Workshop-link exports (§7.3) and ID-only text cannot contain official content, and handwritten packageId lists often omit it. Preview totals and installed/uninstalled count only list entries; retained Core/DLC count only in replacement changes. If Core or any DLC appears in the list, follow it exactly. Names/Workshop IDs serve missing-entry display and links only. RimArcho lists also include semantic hashes: if packageId resolves to different content but another live/loadable Instance has the exact hash, swap to it (unless already in list).

### 7.2 Import formats

Detect by content, not extension (except `.rws`, read as save).

| Format | Detection | Read |
| --- | --- | --- |
| RimArcho list (`.archolist.json`) | Starts `{`, has `entries` array and numeric `format` | `pack.json` format (10 §1): `name` is list name, `game`, `entries[]` fields `package_id`, `name`, `workshop_id`, `hash`; reject newer `format`. Read pack-directory `pack.json` the same way |
| Text list | Other text | One packageId per line (`author.name`), `#` comments, skip headings/prose |
| ModsConfig.xml / RimSort exported list | Root `ModsConfigData` | `version`, `activeMods/li` |
| Save | `.rws` or `<savegame` in first 4096 bytes | `gameVersion`, `modIds`, `modNames` under `<meta>`; stop at `</meta>` (saves are tens of MB); names label missing entries |
| RimWorld `.rml` (in-game "save Mod list", also written by RimCrow) | Root `savedModList` | Prefer `modList/ids` and `names`, else `meta/modIds`, `modNames`; matching `meta/modSteamIds` supplies Workshop IDs |
| RimCrow `ModList.xml` | Root `ModList` | Parallel `modIds`, `modNames`, `modSteamWorkshopIds` (`0` = none) |
| RimSort JSON | Starts `{` | `version`; first existing of `activeMods`, `active_mods`, `mods`, `mod_list`, `modList` |
| RimCrow JSON | `{"format":"rimcrow_json", …}` or array | Prefer `modlist` (`{package_id, name, workshop_id}`), else `package_ids` |
| RimSort clipboard text | Each line `name [packageId][link]` | Link `?id=` supplies Workshop ID |
| Rentry markdown | Inline `{packageid: x}` or `packageid: x` | packageId; `?id=digits` supplies Workshop ID; last `[name](` supplies name |
| Workshop ID list | ≥ 7-digit number per line, or steamcommunity link with `id=` | Workshop ID |
| RimCrow share code | See §7.4 | |

- JSON entries may be strings or objects with `packageId` / `package_id` / `packageid` / `id`, `name`, `workshopId` / `workshop_id` / `publishedFileId`.
- Retain first occurrence of duplicate packageIds. Detect UTF-16 / UTF-8 BOM and normalize to UTF-8; read files under 16 MB whole.
- If no Mods found, error: "no mods found in it: a list from RimSort, RimCrow or RimWorld, a share code, packageIds or workshop links".
- Pasting one line in UI: existing file path reads file; Rentry link fetches `https://rentry.co/<id>/raw` (4 MB cap; on failure suggest opening page, copying list and pasting); Workshop link or ≥ 6-digit number reads Steam collection (§7.5). Only the latter two use networking.
- Pack (`.archopack`) import: 10 §4. Its exported list is a RimArcho list imported here.

### 7.3 Export

`archo loadout export [--format <format>] [--out <file>]` exports current plan. Without `--out`, stdout; existing `--out` files are refused, never overwritten.

| Format | Content |
| --- | --- |
| `archo` | RimArcho list (`.archolist.json`): `pack.json` format (10 §1), `light` true, `name` plan name. Entries include `lineage`, `instance`, `workshop_id`, semantic `hash` (Core/DLC only `official`); missing entries only `package_id`. Importer sees which versions match (§7.1). First UI export option, "RimArcho list" |
| `text` (default) | Two `#` header lines (environment, Mod count, time; import instructions), then `<packageId>  # <name>` per line; missing names `missing` |
| `modsconfig` / `rimsort` | ModsConfig XML (`ModsConfigData`: `version`, `activeMods/li`, `knownExpansions/li`); same-package Workshop copy uses `_steam`. RimSort exports this format; `rimsort` is an alias |
| `rml` | RimWorld `savedModList`: `meta` contains `gameVersion`, `modIds`, `modSteamIds` (`0` if absent), `modNames`; `modList` contains `ids`, `names`; load directly in game's Mod menu |
| `rimcrow` | Share code, one line (§7.4) |
| `rimcrow-xml` | RimCrow `ModList.xml`: `modIds`, `modNames`, `modSteamWorkshopIds` (`0` if absent) |
| `workshop` | Workshop links (added 2026-10-23): one `#` header (list name, Mod count, Workshop count), then `- [<name>](https://steamcommunity.com/sharedfiles/filedetails/?id=<Workshop ID>)` per line in load order; final comment lists non-Workshop entries (Core, DLC, local Mods, missing entries). Lets recipients subscribe; imports as Workshop ID list (§7.2) |

`archo` adds `versions`: plan allowed game versions (§1.6), e.g. `["1.5","1.6","1.7"]`. Older RimArcho ignores it.

UI export is a list subpage (16 §2.17): preview each format, then save to file or copy; share codes are copy-only.

Entry serialization matches game writing (§4.1 items 4/5); game version from environment, falling back to ModsConfig.xml.

### 7.4 RimCrow share codes

- Format: `RC-<8 uppercase CRC32 digits>-<unpadded URL-safe base64>`. Also accept old `RMM1-`; strip `share://RC/` prefix.
- Base64 decodes to zlib-compressed JSON: `[1, list name, game version, entries]`. Entries: `"pkg"`, `["pkg","WorkshopID"]` or `["pkg","WorkshopID","name"]`.
- CRC32 covers compressed bytes; reject mismatch ("share code is damaged or incomplete"); reject first element other than 1 (unsupported version).
- Export at maximum compression; named entries as triples, Workshop-ID-only as pairs, otherwise strings. List name is plan name.

### 7.5 Steam collections
- API: `POST https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/`, form `collectioncount=1&publishedfileids[0]=<ID>`, no API key. Sort `children[]` by `sortorder` (gaps allowed); `filetype` 0 = Mod, 2 = subcollection. Noncollection/private IDs have `result` other than 1 and no `children`; report "this is not a collection".
- Fill collection/item titles through Workshop details API, 100 per request; title failures do not prevent comparison.
- Collection URL/ID: take `?id=` or leading ≥ 6 digits.
- `archo collection <URL or ID>` reads and compares locally in three groups: installed (matching Instance, prefer loadable for duplicate Workshop IDs), uninstalled (Workshop links), subcollections (listed, no recursion). Online only on execution.
- `archo collection <URL or ID> --import` imports installed Mods in collection order, destination under §7.1 (currently replace current plan), prefixed with current Core/DLC (§7.1). Uninstalled entries show links, not missing entries.
- Missing items in "can it run" offer "subscribe in the Workshop" links; one-click subscribe/unsubscribe planned for 1.2 (01 §1).

### 7.6 Following Steam collections

Decided 2026-10-23, implemented (checks when opening all lists, due conditions in 16 §2.16.1 A14; scheduled background checks TODO). Plans may **link** one or more Steam collections. When a collection changes, RimArcho tells the player what changed; syncing is the player's choice, never automatic plan mutation.

- **Data**: new `plan_collection` table: plan, Workshop ID, title, sync mode (`add` / `follow`), last check time, **snapshot** (collection entries/order when last linked or synced).
- **Link**: paste collection URL or check "follow this collection from now on" during import (§7.1). Fetch once and snapshot current content, so newly linked collections have no changes. Link each collection once per plan.
- **Check**: online only when opening all lists (interval: 16 §2.16.1 A14) or `archo plan sync --check`, same API as §7.5. No launch check or background polling.
- **Changes**: compare current collection against snapshot: added (not in snapshot), removed (not in current). A player-disabled collection Mod is not a change; comparison is against snapshot, not plan.
- **Sync**: one reversible edit `sync collection <title>`, then update snapshot to current collection.
  - `add` (default, UI "add new only"): enable installed additions at constraint-suggested positions, like adding missing Mods (§7.1). Retain collection-removed Mods in plan.
  - `follow` (UI "also disable dropped"): also disable collection-removed Mods still in plan. Disable only; no unsubscribe or deletion.
  - Uninstalled additions show Workshop links, not missing entries (§7.5).
- **Uninstalled count**: collection entries without any local copy, with "subscribe in the Workshop" links.
- **Unlink**: delete association/snapshot, preserve plan. Deleting a plan also deletes associations.
- **Commands**: `archo plan follow <name> <collection URL or ID> [--mode add|follow]`, `archo plan unfollow <name> <collection>`, `archo plan sync <name> [<collection>] [--check|--dry-run]`. `--check` checks/lists changes only; `--dry-run` shows proposed enables/disables. MCP has corresponding tools; `sync` without `--check`/`--dry-run` is marked as mutating a plan.

## 8. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| V1 | Start RimArcho after outside ModsConfig.xml edit | New `external` version; none if unchanged | Pass (`TestExternalChangeAndConflict`) |
| V2 | Select first history step after three edits | Workspace returns to after first step; `ctrl+r` advances | Pass (`TestHistorySteps`) |
| V3 | Select v1 in history | Show diff; after confirmation workspace equals v1 | Pass |
| V4 | Game changes only ModsConfig.xml formatting (indentation) | No new version | Pass (`TestExternalChangeAndConflict`) |
| V5 | Outside edit with unsaved plan changes | Record `external`, do not load, flag conflict; checkout clears flag | Pass (`TestExternalChangeAndConflict`, per environment) |
| V6 | Plans A/B; write A, edit in game Mod menu, current B | Load outside content as A `external`; B unchanged | Pass |
| V7 | V6 but A edited again after write | A records `external` and conflict; diff/load/ignore available | Pass |
| V8 | Two environments share user data; environment 1 writes, open 2 | No outside-change report | Pass |
| V9 | Create (empty/current/v<n> copy), switch, rename, delete plans | Independent workspaces/undo/versions/staging; cannot delete last; switching writes no game files | Pass |
| V11 | `archo save` and plan launch | `save`: `user`; save directly after checkout: `undo`, note `restored v<n>`; launch: `launch`; writer is that plan | Partial pass (`user`, `undo`: `TestHistoryCheckoutUndoSource`; launch version/writer implemented, `internal/app/launch_test.go`; plan in launch record: `loglaunch_test.go`) |
| V12 | Game write: `_steam`, version rewrite, backup, BOM/CRLF/unknown elements, missing entries | Per §4.1 | Pass (`TestSaveSteamSuffixVersionAndBackup`, `TestRenderPreservesUnknown`, `TestWriteFileBackupAtomic`, `TestRegistrySteamSuffix`) |
| V13 | Import missing entry, install Mod, scan | Missing entry automatically becomes Instance | Pass (`TestLoadoutImportHealAndEdits`) |
| V14 | Enable residue, about-disabled, nondeployable-source-only, same-location duplicate package | Refuse with reason | Pass (`TestEnableRefusals`) |
| V15 | Stage 20 enabled Mods, restore all | Each returns to original position | Pass (`TestStageRestoreAllKeepsPositions`, `TestStageRestoreAllRandomBatches`) |
| V16 | Enable needed library, unrelated Mod, DLC without positions | Library before dependent, unrelated at end, DLC after official content; Harmony-only requirement does not put before Core | Pass (`TestEnablePlacesByConstraints`, `order.TestPlace`) |
| H10 | `archo pin` enabled Workshop Mod used by two plans | Copy in lock library with `origin.json`, original Lineage (confirmed); current plan swaps to copy, deployed via `_Archo_`; other plan keeps Workshop written `_steam`; Workshop untouched | Partial pass (current plan: lock-library tests; other plan unchanged untested) |
| H11 | Workshop update after H10 | `mod.pinned` warning "upstream has an update"; `mod --diff` lists changes; `pin --update` creates/swaps new copy, retains old | Pass |
| H12 | `unpin` H10 copy | Current plan returns to Workshop, copy retained | Pass |
| X1 | Import RimSort XML, JSON, clipboard | Original order, missing entries retained | Pass (`TestParseRimSortFormats`) |
| X2 | Export RimSort then reimport | Round trip matches | Pass (`TestExportImportRoundTrip`) |
| X3 | Import `.rml`, ModList.xml, share code; export/reimport rml, rimcrow-xml, code | Round trip matches; reject one-digit CRC error | Pass (`TestRimCrowAndRMLFormats`, `TestExportImportRoundTrip`) |
| X4 | Collection with 1 subcollection, 1 uninstalled entry | Three groups correct; `--import` lists link for uninstalled item, not missing entry; Core stays first | Pass (`TestCollection`) |
| X4a | Plan enables Core/one DLC, another installed but disabled; export/import Workshop links (replace/new), then text containing Core | First two retain Core/enabled DLC/list Mods, exclude disabled DLC; explicit Core follows input | Pass (`TestImportWithoutOfficialKeepsCoreAndDLC`, `TestExportWorkshopLinks`) |
| X5 | Import as new plan | New plan, v1 `import`, current unchanged | Pass |
| X6 | Replace current plan then undo | One undo restores original; no ModsConfig.xml write | Pass (`TestImportApplyIsOneStep`, `TestLoadoutExchange`) |
| X7 | `import rimcrow` twice | Second all "already exists"; disabled rules excluded unless `--include-disabled-rules` | Pass (`TestImportRimCrow`) |
| P1 | supportedVersions intersection 1.5/1.6, created under 1.7 | Default 1.5–1.7; empty intersection only 1.7 | Implemented |
| P2 | Player sets 1.6/1.7 then enables 1.4-only Mod | Allowed versions unchanged; `plan versions <name> default` recalculates | Implemented |
| P3 | Game 1.7, plan allows 1.5–1.6 | "for you to confirm" in "can it run" offers "add 1.7"; launch unblocked | Implemented |
| P4 | Export `archo`, import new plan | Allowed versions preserved | Implemented |
| C1 | Linked collection adds 3 (2 installed), removes 1; sync `add` | Enable 2 at suggested positions, link uninstalled 1, retain removed; one undoable edit; updated snapshot, next check unchanged | Implemented |
| C2 | C1 with `follow` | Also disable removed 1; no unsubscribe/deletion | Implemented |
| C3 | Player disables collection Mod, collection unchanged | No changes | Implemented |
| C4 | Unlink | Plan unchanged; association/snapshot deleted | Implemented |
