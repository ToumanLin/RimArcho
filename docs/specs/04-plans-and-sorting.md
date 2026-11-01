# 04 · Plans and sorting

> Status: 2026-10-21. Implemented: plan editing, multiple plans, history, write rules, writing on launch, sorting, rules, community data, locked versions (lock library), list exchange, player data and RimCrow import, and attributing outside changes to the writer. Added 2026-10-23: allowed game versions (§1.6), following Steam collections (§9.6), "Workshop links" export (§9.3); implemented (CLI, MCP, GUI). On 2026-10-25, plans became shared across all games (§1.1, §1.7); implemented (schema v19).
> Basis: 00, 00a, 01, 02 §5–§6, 03 §4, 05 §4, 06 §2.5, 07 §6–§7, 08 §2–§7.

A plan is an ordered enabled list. This document covers storage, editing, rollback, writing to the game's `ModsConfig.xml`, automatic sorting, locked versions, player data and list exchange. Launch flow: 08 §2; UI: 06 §2.5.

## 1. Plan model

### 1.1 What a plan is

- Plans are **shared by all games (environments)** (decided 2026-10-25, §1.7). Every game shows all plans; switching games does not make lists "disappear". The first plan is `default`. Names are case-insensitive.
- Content: an ordered list of entries, each an Instance or a **missing entry** (a packageId that cannot resolve to an Instance, preserved as-is, §4.2).
- Plans store Instances, not packageIds: the player chooses "this copy" (00a §4).
- All plans share the game's default saves and settings; plans have no own saves. Build a pack for own saves (08 §5).
- Edits **persist immediately** in RimArcho's database, surviving program closure; every step is undoable (§2.1). "Save" is separate, occurring only on plan launch (08 §2) or `archo save` (§4.1).
- Names are unique across all plans. A named list is a plan; naming a version no longer exists (`loadout tag` removed).
- Each environment remembers one **current plan** (any plan): the editor's plan and the default target of CLI/MCP commands. Two games may share a current plan.
- User rules (§6.1) and player data (aliases, tags, notes, §8.1) belong to Lineages and are shared across plans; plans decide only what is enabled and in what order.

### 1.2 Data owned by each plan

Workspace, undo stack, version history and staging belong to the plan (schema v11). Identity confirmation merging Mods moves these records to the retained ID under 02 §5.4.

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
| Launch records | Environment | `launch` records environment, time, method, command line, plus plan, list fingerprint and version number (plan empty for packs); filled on launch (08 §2) |
| User rules | Lineage (stored per environment) | `user_rule`, §6.1 |
| Player data | Lineage | `lineage_data`, `lineage_tag`, §8.1 |

### 1.3 Managing plans

| Action | Rules |
| --- | --- |
| Create | From an empty list, current-plan copy, historical version, imported list (§9.1), or save's Mod list (10 §4). First version source `import`, notes identify origin (copies: "copied from <plan> v<n>") |
| Switch | Changes only the environment's current plan, never game files. Current plan is used for launch, game writes and CLI/MCP plan commands. **Opening is not switching** (decided 2026-10-25): GUI sidebar, command panel and all lists, and TUI `P`, command panel or editor `tab` (since 2026-10-31), only view/edit the plan in this window (session's "open plan", `App.OpenPlan`), leaving current plan unchanged. Selecting it on launch, clicking "make current", saving from its editor or clicking launch makes it current. Newly created plans are only opened too. CLI `--open-plan <name>` targets another plan for one command without switching (used by GUI analysis/pack-build children). Actual game content follows the user-data-directory write record, displayed as "in the game now: <list> v<n>" (GUI: green dot on that sidebar child when multiple plans exist, 06 §2.5; TUI: status bar) |
| Rename | Name only; history unchanged |
| Delete | Player confirmation required; cannot delete the last plan. Delete its workspace, undo stack, versions and staging too; games using it as current switch to the oldest other plan |

GUI centralizes these actions on all lists (06 §2.17). Commands: `archo plan list`, `archo plan new <name> [--from current|v<n>|<file>]`, `archo plan use <name>`, `archo plan rename <old> <new>`, `archo plan remove <name>`. Other CLI/MCP plan commands (`enable`, `sort`, `loadout log`, etc.) target the current plan (05 §4).

### 1.4 First use and migration

- When an environment is first read without a current plan (`environment.current_plan = 0`):
  - If ModsConfig.xml still matches a plan's last write (hash equals write record), make that plan current; create none.
  - If ModsConfig.xml is absent (for example, a new game with own saves) and plans exist, make the most recently edited plan current; create none.
  - Otherwise read ModsConfig.xml and create a plan, v1 source `import`, note `ModsConfig.xml` (absent file: empty list, note `no ModsConfig.xml`). Name `default`; if taken, environment name, then numbered (`1.6 2`). First-run guide also offers "move over from RimSort or RimCrow" or "open a pack or a Steam collection" (06 §2.15).
- Existing database migration (v10 → v11): each environment's workspace, undo stack, versions and staging become its first `default` plan. Each `loadout_tag` name becomes a new plan with that version's list and a fresh `import` history entry; delete `loadout_tag`. Rewrite `modsconfig_state` by environment user data directory, writer set to its first plan; for shared directories retain the latest write. Rename a `default` tag to `default (tag)`; discard tags pointing to missing versions.

### 1.5 Archo Monitor added by default

Archo Monitor is RimArcho's bundled in-game helper Mod (03 §6.7; installation/updates: 08 §9). **On by default** means RimArcho adds it to plans; afterward it is an ordinary Mod.

- **Setting**: `monitor.default` (`on` / `off`, default `on`): `archo config set monitor.default off`, or settings "game log" group (06 §2.14). Controls automatic addition only, not updates (08 §9).
- **Plans affected**: new plans (all origins: empty, current-plan copy, history, imported list or save), and every existing plan in the environment when RimArcho first installs Monitor (upgrade). Check timing: "When to check" in 08 §9; new plans immediately after creation.
- **Once per plan**: record processed plans in meta `monitor.added:<plan ID>` (added, or already present). Players may then disable, move or stage normally; RimArcho does not re-add it except during sorting (next item).
- **Sorting adds it too** (2026-10-28): if enabled in settings, absent from plan and installed, every sort (`archo sort`, editor, can it run) places the latest version after Harmony in the same undo step. Re-add even if the player removed it; disable the setting to avoid this. If impossible (no Harmony in plan, `Mods/` cannot hold links), sort normally and give the reason (`SortResult.MonitorSkipped`, GUI notice and `archo sort`). Test: `TestSortAddsMonitor`.
- **Position**: immediately after Harmony (`brrainz.harmony`). Monitor About.xml declares `loadAfter brrainz.harmony`, `loadBefore Ludeon.RimWorld`; sorting (§5) keeps it after Harmony and before Core to observe Core and all Mod loading.
- **Neither add nor mark processed** (retry next check) if setting `off`, no Harmony (required), `Mods/` cannot hold links (FAT, exFAT, 08 §9), or no installed Monitor.
- **One reversible edit**, named `add Archo Monitor after Harmony (default; archo config set monitor.default off)`. Also applies to noncurrent plans: record in that plan's own undo stack; `archo undo` there reverses it. Edit result explanations (CLI output, UI notices) list affected plans. Tests: `TestMonitorInstallAndDefault`, `TestMonitorDefaultOff`.

### 1.6 Allowed game versions

Decided 2026-10-23, implemented. Each plan records **allowed game versions** (major/minor, such as 1.5, 1.6, 1.7), answering "which versions is this list intended for?" UI: all lists (06 §2.17).

- **Default**: intersect all Mods' declared `supportedVersions`, add the game major/minor at creation, then take the continuous range from lowest to highest. Example: intersection 1.5, 1.6, created under 1.7 → default 1.5–1.7.
  - Exclude official content (Core/DLC declare only their own version), missing entries and Mods without `supportedVersions` from the intersection.
  - Empty intersection: use only creation version.
  - Defaults are not stored; recalculate from the current list on every read, so enabling/disabling Mods changes them.
- **Player overrides**: store the chosen set, which need not be continuous. It no longer follows Mods until reset.
- **Storage**: add `created_game` (creation game major/minor) and `versions` (comma-separated selection, empty = default) to `plan`. On migration, take existing plans' `created_game` from their environment's game version then.
- **Copying plans** copies the player's selection. **Import** uses `versions` from RimArcho lists if present (§9.3), otherwise defaults; `created_game` is the current environment version.
- **Uses**:
  - Show plan versions on launch and all lists (06 §2.3, §2.17).
  - If current game is outside the allowed set, "for you to confirm" in "can it run" shows "<list> allows 1.5–1.6; this game is 1.7", with "add 1.7" and "go to all lists" actions. Does not block launch.
  - Exported RimArcho lists include `versions` (§9.3).
  - `version.unsupported` (03 §4.3) still checks current game version, independently.
- **Commands**: `archo plan versions [<name>]` shows allowed versions and the count of Mods not declaring each; `archo plan versions <name> <versions…>` sets them, e.g. `1.5 1.6 1.7` or `1.5-1.7`; `archo plan versions <name> default` resets. Version settings are not list edits and do not enter undo stacks; UI status offers "undo", which restores the previous value. MCP has corresponding tools.
- **Warning counts**: `PlanVersions.Missing` and version-option notices in all lists exclude ignored version warnings and list `Muted` separately (03 §4.2). If all ignored, say "no version warnings", never "every Mod supports it". `Common`, defaults, allowed versions and player selections still use actual declarations; ignoring warnings does not count as support.

### 1.7 Plans shared across all games

Decided 2026-10-25, implemented. Player feedback (translated): "I created three lists in 1.7, switched to 1.6 and saw only `default`; I thought the lists were lost." Plans are just "what is enabled and in what order", independent of game directories. Allowed game versions (§1.6) already describe intended versions, so plans no longer belong to environments.

- **List page, sidebar and launch page** show all plans without grouping by game. Show allowed versions normally; "can it run" warns if the current game is outside them (§1.6), rather than hiding plans.
- **Reading entries in another game**: plans store Instances. Shared-folder Instances (Workshop, SteamCMD, extra folders, lock library, Monitor) are global and recognized by all games. Another game's own Instances (Core/DLC in its `Data/`, its `Mods/`) read as missing packageId entries, then resolve to this game's copy under §4.2 (`app.localizer` + `Registry.Heal`); absent copies remain missing. Reading does not modify storage. Editing here stores this game's Instances; returning resolves back by packageId in the same way.
- **Links on game switch** (02 §0 item 14): Mods from the previous game's `Mods/` in the current plan still link into the target game as `_Archo_env_<Instance ID>`.
- **Other games' `Mods/` are usable in every game** (decided 2026-10-25): each game index also loads live Mods from other games' `Mods/` (excluding `_Archo_*` links), with in-memory source type `elsewhere` (`source.Elsewhere`, not stored; label `local-<environment name>`, `app.withElsewhere`). They appear in mod library (source `local-<environment name>`, location that game's `Mods/`, 02 §0 item 11) and editor Disabled. Once enabled, they deploy like SteamCMD/extra-folder Mods: save links them into this game's `Mods/` as `_Archo_env_<Instance ID>` (`DeployName`), retained on save/disable (same links as game-switch links). If this game already has an Instance of the same folder (a previously scanned link became a local Instance), show only this game's copy. If this game's `Mods/` has the same package ID, read the other game's plan entry as this game's copy (`Index.Elsewhere`, "retain target copy with same package ID" in 02 §0 item 14). Read-only Data/ is not shared across games. Test: `TestOtherGamesModsUsableAnywhere`.
- **Write records** also store the written game (`modsconfig_state.writer_env`). Resolve outside changes in that game (§4.3); "in the game now" includes its name when another game sharing the directory wrote it.
- **Deleting an environment** preserves plans: rewrite entries referencing its own Instances in workspaces, versions and undo steps to packageIds before deleting Instances (`store.keepLists`).
- **v19 migration**: add `writer_env` to `modsconfig_state`, using the writer plan's environment then. For duplicate names across environments, oldest retains its name; others become `<name> (<environment name>)`, numbered if still conflicting. Invalid current-plan references switch to that environment's oldest plan. Tests: `TestPlansSharedAcrossGames`, `TestPlanStore`, migration tests.

## 2. Editing

### 2.1 Operations

Every operation is a command shared by GUI, TUI, CLI and MCP (05 §1). Each operation changing the list or staging is one undo step; record none if neither changes.

| Operation | Command | Rules |
| --- | --- | --- |
| Enable | `archo enable <refs…> [--at <n>\|--before <ref>\|--after <ref>]` | Check loadability first (§2.3); explain refusals. Notify and skip already-enabled Mods. With an explicit position, insert multiple Mods consecutively in supplied order. Otherwise replace a missing entry with the same packageId in place; if none, place individually under sorting constraints (`order.Place`, same as editor suggestions `App.SuggestAt`, 06 §3): immediately after the last enabled Mod it must follow (not before official content); if only enabled Mods require following it, before the first of those (libraries, dependencies, DLC; later-enabled Odyssey thus returns to official order); unconstrained at end; `attach` immediately after target. Settled by corpus replay on 2026-10-26: remove DLC and 3 random Mods per log, then re-enable. This rule restores 95% of DLC and 17% of other Mods to original positions (median distance 31); "latest position constraints permit" gives 15% (median 35), always append gives 0% / 2% and other Mods violate 83,000 strong constraints (739 with this rule) |
| Disable | `archo disable <refs…>` | Accept `pkg:<packageId>` to remove missing entries (with or without `_steam`). Notify if already disabled |
| Move | `archo move <ref> --to <n>\|--before <ref>\|--after <ref>\|--up\|--down` | Require one position form; error for disabled entries, suggesting `archo enable` |
| Swap instance | `archo swap <ref> [<ref>]` | Replace an enabled Instance with another in the same Lineage, preserving position. References may be in either order. With one reference, use the sole other loadable Instance; if several, list choices. Different Lineages error, suggesting `archo lineage merge` or disable then enable |
| Sort | `archo sort [--dry-run]` | §5; `--dry-run` only displays results |
| Stage | `archo stage add/drop/restore/list` | §2.2 |
| Undo / redo | `archo undo [<n>]`, `archo redo [<n>]` | Up to 100 steps per stack, shared by all interfaces. New edits clear `redo`. `checkout` (§3.2) and import replacement (§9.1) are reversible edits too |
| History | `archo history [--all]` | Unsaved edits and versions (§3.3); latest 10 versions by default |

- Two processes (e.g. GUI and MCP) may share one RimArcho data directory (`%APPDATA%\RimArcho`, 07 §3). A database change key (`data_version` + `total_changes`) change makes the next read see the other process's changes; stale-view edits do not overwrite them. Test: `TestTwoProcessesShareTheWorkspace`.
- Undo/redo snapshots preserve the list's original stored Instance bindings (including Core, DLC and game-owned Mods), not current-game-resolved copies. Restore them unchanged, then resolve for display in the current game. Edit batches follow the same rule.
- Plan reads are cached until the database or ModsConfig.xml (size, modification time) changes.

**SteamCMD batch handoff**: after the player accepts credit replacement (02 §3.4), replace original Instances in lists with corresponding workshop Instances, preserving position, only while the game is stopped and every environment able to load the old copy can load the new one. Current content, historical versions, undo/redo, staged entries and position anchors follow in one transaction. Handoff creates no undo step that would restore the old copy; historical references alone do not retain it. If any environment fails the condition, retain the entire item and change no shared lists; corrupt staging likewise ends that item's handoff with retention. Fixed packs, game deployments not yet handed off and incoming links retain original identity/copy with reasons. Existing game projections reuse §4's writer and registry for `_steam`, replacing only this item and detaching only its owned deployment link, without committing other unsaved edits or switching the player's current list. Merge notes inside the remapping transaction. DDS ledgers do not migrate across physical directories; results suggest optimizing again. List edits and handoff share the launch lock; hold it from read through save. Late snapshots or long-lived edit batches conflicting with handoff must reread, never write old IDs back. Persist deployment obligations in SQLite first so work can resume after exit. Full flow and collection conditions: 02 §3.4.

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
| `archo stage restore <refs…> \| --all [--at\|--before\|--after]` | Restore; `--all` restores everything; refuse if neither references nor `--all` supplied (05 §3.3). One reversible edit |

Tests: `TestStageRestoreAllKeepsPositions`, `TestStageRestoreOneFollowsAnchor`, `TestStageRestoreAllRandomBatches`. Staging column and keys: 06 §2.5.

### 2.3 Loadability

An Instance admitted to a plan must be live, not residue or `unloadable`, and visible to the game or deployable by RimArcho:

- In `Data/`, `Mods/` or subscribed Workshop content; aliases there count (e.g. SteamCMD directory junction in `Mods/`). RimArcho-owned `_Archo_` links do not count here;
- Or **deployable**: source `steamcmd` or `extra`, linked into `Mods/` with `_Archo_` on game write (08 §3).

Refusal explanations for `enable`, `swap`, `stage restore`:

| Condition | Explanation |
| --- | --- |
| Tombstone | No longer on disk |
| `unloadable: about-disabled` | About.xml was renamed; the game cannot read it (handling: 08 §6) |
| Residue | Workshop folder is no longer subscribed; game ignores it |
| Source neither read by game nor deployable | Game does not read Mods from `src:<label>`; link or copy into `Mods/` |
| Another copy with same packageId at same location | `pkg.duplicate-installed`: first by folder name wins; game ignores this copy (03 §4.3) |
| Workshop and non-Workshop copies of same packageId | Cannot enable both (game retains only unsuffixed copy, §4.1 item 4); suggest `archo swap <enabled> <this copy>` |

## 3. History

### 3.1 Versions

Each plan version `v<n>` records time, source, complete list, game-version string and notes. Numbers increase within a plan.

| Source | Meaning |
| --- | --- |
| `import` | Initial list from outside: first ModsConfig.xml read (§1.4), or new plan from list file, share code, collection, save or copy (§1.3, §9.1). Notes identify origin |
| `external` | Outside changes: game Mod menu or another tool changed ModsConfig.xml; attribute to its last writer plan (§4.3). Note `ModsConfig.xml changed outside RimArcho`. Also serves as "startup snapshot": outside changes discovered on RimArcho startup use this source, not a separate one |
| `user` | List written to game by `archo save` (UI "save") |
| `undo` | Historical version checked out then written without further editing. Note `restored v<n>` |
| `launch` | List written on plan launch (08 §2). If identical to plan's latest version, create none; launch record points there. Launch after checkout without editing gives new version note `restored v<n>` |

Undo steps are not versions: versions are lists "written to or read from the game"; undo steps stay inside RimArcho.

A plan's **unsaved changes** are the difference between latest version and workspace. This is informational and blocks nothing; launch writes the current list (08 §2).

### 3.2 Commands

| Command | Meaning |
| --- | --- |
| `archo loadout log [--all]` | Current plan versions: number, time, source, entry count, notes; mark unloaded outside versions |
| `archo loadout diff [<v>] [<v>]` | Four categories: added / removed / moved / instance swaps. Default latest version versus workspace. Versions accept `v3` or `3`; workspace accepts `workspace`, `ws`, `current` |
| `archo loadout checkout <v>` | Load a version into workspace as one reversible edit; no game write. Immediate game write afterward uses source `undo` |

If the loaded version, resolved for the current game, exactly matches the workspace list, it is a no-op: no origin, conflict, staging or undo/redo changes. UI says only "no changes", without offering undo.

Diff algorithm: changed Instance within one Lineage is an "instance swap". Compute longest common subsequence of remaining shared entries; only entries outside it are "moved". Moving one Mod thus reports one move, not moves for entries whose positions were unaffected.

### 3.3 History area logic

UI: third column in plan editor (06 §2.5). Data and behavior:

- **Unsaved steps**: undo stack, one row per step, newest first. Cursor marks current state; `u` / `ctrl+r` moves it. Selecting and confirming an earlier step undoes through it (`StepTo`), stopping at stack bottom if insufficient. Clicking an unsaved step applies immediately; `u` / `ctrl+r` can still return.
- Titles derive from before/after lists (e.g. "enable Harmony · position 2"), not separately stored.
- **Versions**: one row per version with source. Selection shows diff against workspace; confirmation runs `loadout checkout`. Confirm before loading a version.
- Unloaded outside versions (§4.3) can also be diffed, loaded or ignored here.

## 4. Writing to the game

### 4.1 ModsConfig.xml write rules

Shared by **plan launch** (08 §2) and **`archo save`** (write without launching, for players launching via Steam etc.; UI "save").

1. **Synchronize first**: check outside changes before writing (§4.3), preserving existing outside content as a version rather than silently overwriting. **Refuse with unloaded outside versions.** If the target plan has one, or the file contains another plan's unloaded outside content, `save` and `launch` (including `--dry-run`) refuse without writes and give diff / checkout / ignore commands (05 I8). Writing would overwrite changes the player has not reviewed.
2. **Prechecks**: refuse environments without user data directory, suggesting `archo env set --user-data <directory>`; refuse unreadable existing files, asking the player to repair or move them. Write paths must pass safety-boundary checks (01 §4).
3. **Synchronize links**: before writing ModsConfig.xml, synchronize `_Archo_` links and remove pack `!ArchoPack_` links (08 §3).
4. **`activeMods`**: lowercase packageIds in plan order. When both Workshop and non-Workshop copies exist for a packageId, write `<packageId>_steam` if the plan chooses Workshop, otherwise unsuffixed `<packageId>`. Reason: game registers official → `Mods/` → subscribed Workshop, folder-name order within each group. Workshop duplicates of registered non-Workshop copies gain `_steam`; other duplicates are rejected (07 §7). Enabling both makes the game retain only the unsuffixed copy, so a plan cannot contain both Instances (§2.3).
5. Write **missing entries** unchanged. An Instance already in the plan but no longer registered by the game (deleted or disabled) is written as its packageId so it does not silently vanish. Automatically heal to an Instance when resolvable again (§4.2).
6. **`version`**: current environment game version, the full `Version.txt` string, as the game writes it; preserve original if environment has no game version. The game **discards the whole list** and resets if major/minor differs (07 §7), hence write the current version every time.
7. **Other content**: replace only `<version>` and `<activeMods>`, preserving `knownExpansions`, unknown elements, BOM and newline style; XML-escape text. If absent, create in game format, with installed official DLC in `knownExpansions` (live official Instances, excluding Core).
8. **Backup and atomic replacement**: back up original to `<user data directory>/Config/ModsConfig.xml.rimarcho.bak`, retaining only the latest (full history in RimArcho database). Write new content to a same-directory temporary file, flush, then rename over original.
9. **Record**: add plan version (`archo save`: `user` or `undo`; launch: `launch`, §3.1); record hash, writer (plan/version) and time against user data directory (§4.3); set workspace origin to this version and clear unloaded-outside-version marker.

Preview and execution:

- `archo save --dry-run` and command-panel "preview saving" show diff from latest version, version rewrite ("will change 1.6 to 1.7"), missing count, and links to create/delete. Preview is read-only, without "continue".
- UI "save" (`w`) executes directly like CLI `archo save`; status "saved as vN". Refusals in items 1/2 show reasons and commands without confirmation (06 §6). MCP allowance belongs to the host (05 §5).

If read `version` major/minor differs from the game, status bar and `archo status` warn: "This list was saved for version X; the game will reset it on launch. Launch or save once from RimArcho to save for the current version." Blocks nothing (launch always writes current version, 08 §2).

### 4.2 Instance resolution on reading

When reading ModsConfig.xml (and imported lists, §9), resolve each entry in game order (07 §7):

1. Match full ID first. `<pkg>_steam` maps to a Workshop Instance only if a same-named non-Workshop copy also exists.
2. Otherwise match **folder name** and rewrite the entry to that Mod's packageId.
3. If still unmatched and ending `_steam`, strip and retry. This covers a deleted local copy leaving only Workshop.
4. For several Instances matching a plain packageId, prefer non-Workshop. With same-source duplicates (e.g. two in `Mods/`), game accepts only the first by folder-name order, errors and ignores others. Both cases yield `pkg.duplicate-installed` (03 §4.3).

- Duplicate Instance entries retain the first position only (game disables the later one).
- Unresolved entries remain **missing entries**, visible with `dep.missing`-style findings (03 §4.3), never silently discarded.
- **Healing**: a later scan finding a resolvable Instance automatically turns the missing entry into an Instance, without manual repair (unless already in the list).
- **Installed deployable copy**: unbound packageIds do not automatically select copies outside the game registry (e.g. another game's `Mods/`). `App.UseInstalled(packageIDs)` binds copies marked `CanEnable` in `MissingMods` in place, one reversible batch edit; duplicates retain the first position. Recheck missing status, loadability and enabled same-package conflicts at execution; reject the batch if any request is unavailable. Binding creates no links; save/launch deploys under 08 §3. UI contract: 06 §8.7.

These rules are based only on decompilation; local/Workshop same-package coexistence and `_steam` entries have not been verified in-game (01 §5).

### 4.3 Outside changes

Several plans and environments share ModsConfig.xml (default user data directory, 02 §2.4). RimArcho records last-written hash, last-seen hash and writer **per user data directory** (game, plan, version; game recorded separately because plans are shared, §1.7).

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

## 5. Automatic sorting

### 5.1 Constraints

| Constraint | Source | Strength |
| --- | --- | --- |
| Core first, DLC immediately following in official order | Built-in | Hard |
| Dependencies before dependents | About.xml `modDependencies` (including `ByVersion`) | Hard |
| `loadBefore` / `loadAfter` (including `ByVersion`), `forceLoadBefore` / `forceLoadAfter` | About.xml | Hard |
| Community sorting rules | Community data (§6.3) | Hard |
| User rules | Player (§6.1) | Hard; may suppress any constraint above |
| Preserve current relative order where possible | Current list | Soft |

- Before sorting, add installed, loadable vanilla Core if absent; add absent Archo Monitor when `monitor.default` is on (§1.5); then graph the complete list. Preserve player DLC selections. Preview does not change workspace; additions and sort undo together. Name additions in results ("sorted: added Archo Monitor, 3 moved").
- Editor Disabled defaults to Core/DLC first in official order, then others by name; Enabled shows actual load order.
- `ByVersion` selects current game major/minor (keys such as `v1.6`).
- **Official order**: Core → Royalty → Ideology → Biotech → Anomaly → Odyssey; unknown `ludeon.rimworld.*` follow known DLC; all other Mods follow all official content.
- **Exemption**: Mods declaring `loadBefore` (including `ByVersion`) or `forceLoadBefore` an official package (e.g. Harmony) are exempt from "official first", matching game/Harmony design; otherwise almost every list would cycle.
- **Missing entries** are nodes too: no About.xml declarations, but "official first", other Mods' declarations targeting their packageIds and community rules still apply, largely preserving position.
- `incompatible` user rules affect findings only, not sorting (03 §4.3).
- **Unsatisfiable constraints** do not enter the graph; `archo sort` lists reasons under "not used" (corpus G2):

| Condition | Reason |
| --- | --- |
| Declaration or community rule between official packages contradicts official order | contradicts the official order |
| A rule putting a Mod before Core/DLC (About.xml loadBefore / forceLoadBefore, community rule), while other constraints require it after official content, cycles with built-in order | would put the mod before official content … |

  Noncyclic "before Core" (Harmony convention) is honored normally (exemption above).

### 5.2 Algorithm

1. Build a directed graph. Each edge records origin: Mod declaration, community rule or user rule, and suppressible reference (§6.2). Count duplicate edges once. Omit edges suppressed by `ignore`, reporting counts.
2. Find strongly connected components (Tarjan). If a cycle contains both built-in official edges and "before official content" rules, remove the latter (§5.1 unsatisfiable constraints) and retry. For remaining cycles, **suppress no constraints**: emit `order.cycle` (info, 03 §4.3.1; muted and uncounted in "can it run"), list each edge and origin, and sort the rest normally. Within a cycle: represent each Mod pair by its strongest constraint; retain pairs from strongest to weakest using the abandonment order below, dropping any pair that cycles with retained pairs. Sort members by retained constraints, preserving current order where possible. `archo sort` marks dropped constraints `[broken]`. (Changed 2026-10-26: previously kept the whole cycle's current order, also preserving dependency violations; corpus replay showed about 5% more strong-constraint violations than RimCrow as a result.)
3. Suggest one **edge to suppress** per cycle: weakest dropped constraint (if none suppressible, weakest edge in the cycle). **Abandonment order**, weak to strong:
   1. Constraint strength (§5.3, including per-rule `observed.json` judgments): harmless, weak, strong;
   2. Within equal strength: community rule, `loadAfter`, `loadBefore` / `force*`, dependency, built-in official order, user rule;
   3. Ties: first abandon the edge already violated by the current list (From after To).

   Never suggest user rules; remove them with `archo rule remove`. Keep suppressible references for CLI. UI adopts sorter choices automatically, explaining dropped requirements without counting them in "can it run" (merged into automatic sorting, 06 §5), without requiring manual suppression or an error dialog. Choices apply only to this sort, not stored ignore rules.
4. Topologically sort components (Kahn). When several are available, compare in order: attached to the just-emitted node first (step 5); earlier tier first (top, normal, bottom, §5.5); earliest first member in current list first. Without top/bottom tiers this minimizes differences from the original.
5. `attach`: prioritize a Mod attached after the just-emitted node, provided hard constraints permit.
6. Output new order and diff. UI writes directly to the plan (one reversible edit), not the game. `archo sort --dry-run` only displays. Results include missing required dependencies, unresolved entries, recommended replacements and automatic cycle resolution; API contract: 06 §6.1.

Why cycles do not block: the game does not sort and only warns on declaration violations, so cyclic lists usually run. Example: Ferny's Progression Production declares `loadAfter` Furniture, while Better Architect Mods mutually declare `loadAfter` and dependencies, forming a real 3-Mod cycle. RimArcho reports `order.cycle`, evidence and suppressible references, abandoning the weakest constraint while sorting. `order.cycle` blocks neither game writes, launch nor pack builds (08 §5.2).

### 5.3 Severity of declaration violations

Corpus conclusions (numbers in 07 §6):

- **Declaration violations are common and mostly harmless.** About half of logs violate dependencies or load-order declarations, nearly half community rules. Above 100 Mods, error proportions are similar with/without violations. Most frequently violated constraints seldom involve Mods attributed as error sources. One (`modoptionssort` before Core) is impossible. Thus `order.violated` is at most warning, not error (03 §4.3); sort results and findings distinguish "causes errors" from "declared but not game-enforced".

**Constraint strength** (`internal/order/strength.go`, corpus P1: 6,734 HugsLib 1.6 logs, docs/research/decisions-corpus.md). For each violated constraint, compare logs violating versus preserving it (both Mods enabled, same list-size tier), measuring the proportion where either Mod is attributed as a finding source:

| Constraint | Violations | Source ratio observed/expected | Strength | Severity |
| --- | --- | --- | --- | --- |
| Mod needs Core/DLC first (dependency, loadAfter, forceLoadAfter targeting official package) | 192 | 3–4.5 | Strong | Merge into `official.order` |
| Dependency (between Mods) | 2,903 | 1.41 | Strong | warning |
| loadBefore (between Mods) | 395 | 1.34 | Strong | warning |
| forceLoad*, user rules | Few | — | Strong | warning |
| loadAfter (between Mods) | 5,430 | 1.14 | Weak | info |
| Community rules (between Mods) | 6,541 | 1.13 | Weak | info |
| Declared before Core/DLC | 451 | 1.05 | Harmless | info |

- Above 1.3 is strong. Weak/harmless violations appear only in technical information, excluded from "can it run" counts (06 §5). Plan editor › sorting explanations and order findings state strong/weak, strong first (06 §2.5).
- Per-rule judgments with enough corpus violations ship in `internal/order/observed.json` (aggregate counts only, no log IDs). "Harmless": ≥ 30 violations with source proportion no higher than preserved logs (if fewer than 30 preserved logs, ≥ 100 violations and proportion ≤ 2%). "Harmful": ≥ 20 violations, proportion ≥ 20%, more than twice preserved proportion, z ≥ 3. Use table entries where present, otherwise type defaults. Harmless-violation explanations show corpus counts and suggest `ignore` first.
- `archo data update` fetches third-party datasets only (§6.3); RimArcho has no data repository, so this table is embedded and updated with releases.
- **Odyssey too late** is the commonest official-order violation (23% of corpus logs): DLC enabled after list creation is appended. `official.order` (warning) offers the cheapest moves: move official packages after Mods back after their preceding official package, or move the few Mods before official content after the last official package, whichever uses fewer steps (at most 3; otherwise suggest sorting). Do not separately report constraints from official packages to Mods; list strong ones (Mod requires this DLC) as `official.order` evidence. Automatic sort also restores official order (03 §4.3).
- Mods without declared `loadBefore` official packages go after all DLC, even if originally before Core (e.g. RimCrow Companion).

### 5.4 Other details

- Community rules read only `<data directory>/cache/communityRules.json` (§6.3).
- Diffs do not mark unchanged positions as moved (§3.2).
- Target: sort 500 Mods in ≤ 1 second (S5); measurements: 07 §6.

### 5.5 Top and bottom tiers

Added 2026-10-26. Sources: community `loadTop` / `loadBottom` (`{"value": true}`), user `top` / `bottom` (§6.1), RimCrow import (§8.2). User rules win if both exist for one Mod.

- **Tiers are preferences, not constraints**: compare only when topological sort has multiple available components (§5.2 step 4), never violate constraints, never enter finding analysis (no "not last" finding). Official content is unaffected and stays first (§5.1). Top Mods without loadBefore official packages go immediately after all DLC.
- **Bring related Mods**: top Mods pull all required predecessors along constraint paths to top; bottom Mods pull all dependents along constraint paths to bottom. Otherwise top Mods wait for dependencies emitted at original positions. Official order is hard, so pulling never puts Mods before DLC (RimCrow propagates position weights similarly but lacks official constraints; replay put Mods before DLC in 4.8% of lists).
- Mods pulled both ways (e.g. requiring a bottom Mod while required by a top Mod) stay normal; explicitly top/bottom Mods keep their own tier. Constraints still apply.
- Preserve current order within tiers.
- Suppress community tiers with `archo rule add ignore community:<pkg>:loadtop` (or `loadbottom`); user rules with `archo rule remove`.
- `archo sort` lists each top/bottom Mod, source, suppression method and pulled count under "ends"; mark those pulled both ways `[also pulled to the other end]`.
- No RimCrow-style position weights, alphabetical tie-breaking or guessed Mod-type positions (translations, textures): no corpus evidence, and these cause excessive moves on every sort.

## 6. Rules and community data

### 6.1 User rules

Global on Lineages, shared by all games/plans (02 §0 item 6; `store.Rules` ignores environment), so original/locked versions share rules and package-ID changes do not lose them. (Corrected 2026-10-30: previously said "stored per environment" though code was already global.)

| Rule | Meaning |
| --- | --- |
| `after A B` | A after B |
| `before A B` | A before B |
| `incompatible A B` | Cannot enable A and B together (finding analysis only, not sorting) |
| `attach A B` | A immediately after B (translations, patches) |
| `top A` / `bottom A` | A as early/late as constraints permit (§5.5; preference, not constraint) |
| `ignore <constraint ref>` | Suppress a declaration, community rule or built-in official order |

| Command | Description |
| --- | --- |
| `archo rule add after\|before\|incompatible\|attach <A> <B> [--note <text>]` | References resolve to Lineages; reject same-Lineage pair |
| `archo rule add top\|bottom <A> [--note <text>]` | One reference |
| `archo rule add ignore <constraint ref> [--note <text>]` | Use constraint syntax printed by `archo sort` |
| `archo rule edit <r:n> <kind> <A> [<B>] \| ignore <constraint ref> [--note <text>]` | Replace rule kind, targets and note (2026-10-30); retain `r:<n>` and creation time (`App.UpdateRule`) |
| `archo rule list` | Rules: `r:<n>`, kind, targets, note, creation time |
| `archo rule remove <r:n>` | Delete one |

- Every rule supports a note and records creation time.
- IDs increase monotonically (`rule_seq` in `meta`), never reused after deletion; `r:<n>` always means the same rule in notes/scripts. GUI deletion status "undo" restores it unchanged, retaining `r:<n>` (`Store.RestoreRule`).
- UI: GUI "load-order rules" in editor left column (06 §2.5, default only rules involving this list); TUI editor `r` and details "rules" technical page. Rules are not list edits and do not enter plan undo stacks. MCP `manage_rules` lacks `edit` (TODO, 05 §5.2).

### 6.2 Suppressible constraint references

| Printed form | Refers to | Stored form |
| --- | --- | --- |
| `decl:<#instance>:<field>:<pkg>` | Mod About.xml declaration; lowercase field `loadafter`, `loadbefore`, `forceloadafter`, `forceloadbefore`, `dependency` | `decl:<Lineage>:<field>:<pkg>` |
| `official:<#instance>` | Built-in official-order constraint on this Mod | `official:<Lineage>` |
| `community:<pkg>:<field>:<pkg>` | Community rule, field `loadafter` or `loadbefore` | Same |
| `community:<pkg>:loadtop`, `community:<pkg>:loadbottom` | Mod's community top/bottom tier (§5.5) | Extra trailing `:` |

Print the Lineage's primary Instance reference; store the Lineage so suppression survives copy/package-ID changes. Printed references work unchanged in `rule add ignore` (`TestPrintedIgnoreRefsRoundTrip`).

### 6.3 Community data

| Dataset | File (`<data directory>/cache/`) | Purpose | Default source |
| --- | --- | --- | --- |
| Community sorting rules `rules` | `communityRules.json` | Supplement loadBefore/After (§5.1) | `https://raw.githubusercontent.com/RimSort/Community-Rules-Database/main/communityRules.json` |
| Workshop metadata `steamdb` | `steamDB.json` | packageId/Workshop-ID mapping: links for missing entries; dependency completion (when author omits `modDependencies`, unimplemented) | `https://raw.githubusercontent.com/RimSort/Steam-Workshop-Database/main/steamDB.json` |
| Replacement rules `replacements` | `replacements.json.gz` | "Use another Mod instead": `mod.replaced` (03 §4.3); import preview also suggests replacements for uninstalled entries | `https://raw.githubusercontent.com/emipa606/UseThisInstead/main/replacements.json.gz` |

- Update online via `archo data update`, settings "update now" or automatic checks (06 §2.16); ordinary page reads are offline.
- `archo data update [<datasets>…] [--dry-run]`:
  - Download sequentially (all three if unnamed); `--dry-run` lists sources only.
  - Stage temporary files, validate JSON (`rules` for rules/replacements, `database` for steamDB), record dataset version/timestamp and entry count.
  - **Replace only when all ready**: stage data, source records and compact steamDB index, checking write/close errors. Any download, validation or staging failure preserves existing files. Stage old files during replacement; rename or database-generation write failures restore the whole set. First-install failure removes partial new files. If restoration also fails, preserve backup directory and include location in error. Implementation: `internal/app/datareplace.go`; no cross-file atomicity promise for process kill/power loss.
  - Unchanged source with local file: send previous ETag / Last-Modified as `If-None-Match` / `If-Modified-Since`; skip unchanged downloads. Per-file limit 512 MB, timeout 10 minutes (steamDB about 50 MB). Auto-decompress gzip and strip UTF-8 BOM.
  - Write `<file>.source.json` beside each dataset, replaced/rolled back together: dataset, source, ETag, Last-Modified, version, download time, check time, entries, bytes. 304 advances only check time; older sidecars without it use download time.
  - Compress downloaded steamDB to `steamDB.index.json` (packageId → [Workshop ID, name]; smallest Workshop ID for duplicate packageIds; skip `appid` entries), avoiding repeated parsing of the 50 MB original.
- `archo data status`: source, version, download/check times, entry count per dataset; "old" after 30 days; automatic-check status: 05 §4.2.
- Manual/automatic updates share a lock; download/validation failure preserves files. Success advances database `community:generation` to invalidate other connections' compact indexes; GUI/TUI discard page analysis caches. Automatic failure notifications: 06 §2.16.
- Sources configurable: `config set data.rules.url`, `data.steamdb.url`, `data.replacements.url`; local paths allowed.
- Community data stores packageIds, resolved to Lineages on use.

## 7. Locked versions

A locked version fixes **one Mod** to specific content, unaffected by Workshop updates. It is not a new identity concept, just another Instance in the same Lineage (02 §6). Player-facing wording is "locked version"; `pin` creates it, `copy-local` remains an alias.

### 7.1 Two forms

| Form | Shape | Detection |
| --- | --- | --- |
| Created by RimArcho | Copy directory in **lock library**, with `.rimarcho/origin.json`; Workshop unchanged. Setting `lock.dir`, default `%LOCALAPPDATA%\RimArcho\locked`, shared by all environments (02 §0), registered as shared `extra` source (`src:locked`). Copies, like upstream, are global records usable by any game's plan. Pre-0.9.0 per-environment `<lock.dir>/<environment>` subdirectories migrate to shared folders `src:locked-<environment>`, files unmoved, copies still locked versions. Game sees them only through `_Archo_` links (08 §3) | Scan reads `origin.json`, merges into same Lineage under R5, marks "confirmed" (02 §5.1). `origin_instance` is upstream. Older `Mods/` copies with `origin.json` still count |
| RimCrow style | Workshop `About/About.xml` renamed to `About.xml.disabled` (invisible to game), local copy in `Mods/` (often `_<WorkshopID>_`) | Same Lineage has an about-disabled Workshop Instance and another live, loadable non-Workshop Instance (RimCrow copy, `Mods/` copy or deployed copy all count; prefer enabled if several). Workshop is upstream |

- Locked versions produce `mod.pinned` (info, 03 §4.3), **no longer** `mod.unloadable`. List shows "locked": copy is effective, Workshop is "upstream".
- Recompute classification during analysis, not stored. About-disabled Workshop directories without copies are "hidden Mods" (08 §6), not locked versions.
- Pack copies also have `origin.json`, but scanning skips pack links, so they are not locked versions (08 §5).

**Why lock library rather than `Mods/`.** For duplicate packageIds in `Mods/`, game registers only first folder-name order (07 §7). Two locked versions cannot coexist there; updating requires moving the old copy or renaming About.xml. Lock-library storage links only the current plan's selected copy into `Mods/`: old/new copies coexist, each plan chooses its own, without About.xml changes or manual work. Cost: if `Mods/` volume lacks junctions (FAT, exFAT), plans using locked versions cannot write to game; refuse with explanation under 08 §3.

### 7.2 Creating

`archo pin <instance> [--name <directory name>] [--git] [--dry-run]` (`copy-local` alias).

1. On first use, register lock-library source (check it neither contains nor is contained by data directory/source roots, 01 §4). Copy Instance there, default `<original directory>_local`; suffix if occupied (`_local2`, `_local3` …), **never overwrite**. `--name` must be a plain directory name (no separators, `.`, `..`, `<>:"|?*`).
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
6. Swap original for copy **only in current plan** (`swap`, one reversible edit; no plan change if original disabled). Other plans keep Workshop. On launch/save, copy links into `Mods/` through `_Archo_`; Workshop uses `<packageId>_steam` (§4.1 item 4). If `Mods/` already has another non-Workshop copy with the same package ID, do not create the link (08 §3: game errors on same-source duplicates). If an ordinary folder, offer one confirmation to move it into lock library too (`--move-duplicates`; its plans keep using it through links), then link this copy. If a link or targeted by another tool's link, do not move; refuse with explanation, suggesting using that copy or moving it manually. Never rename About.xml.
7. Output copy reference, path, file count, bytes and skipped links.

### 7.3 Upstream updates

- **RimCrow style**: Workshop snapshot differs from copy (both snapshot ID and semantic hash), and ACF `timeupdated` is later than copy's first appearance.
- **RimArcho-created**: upstream's current snapshot hash differs from `origin_snapshot` in `origin.json`.

Then upgrade `mod.pinned` to warning, saying "upstream has an update", with "see the changes" and "update the locked version":

`archo mod <instance> --diff <instance>` lists differing semantic files, `+ only in latter`, `~ changed`, `- only in former`, sorted by path. Compare about-disabled `About/About.xml.disabled` as `About/About.xml`. Write nothing.

### 7.4 Updating a locked version

Original move-to-library confirmation / CLI dry-run also lists every affected game, link path, original target and detach/rebuild destination. Detach all games' owned incoming links before physical movement; reconnect after updating Instance. Keep one effective entry per instance per game, never both env and locked entries. Refuse foreign incoming links and running games; failure/recovery contract: 08 §3.

`archo pin --update <instance> [--name <directory name>] [--dry-run]`: reference copy or upstream. Show diff first; create a new lock-library copy (new directory) under §7.2 and swap into current plan. Retain old copy (may contain player edits; this is why `--git` baseline exists).

**Old copies in `Mods/`.** Before lock library, RimArcho created copies there. Move such copies (only with `origin.json`) into lock library on first `pin --update` or `archo pin <copy> --move-to-library [--dry-run]`. Same volume: rename; cross-volume: copy (including `.git`, `.rimarcho/`), verify each file, recycle original. Refuse links pointing outside the copy. This command never moves directories without `origin.json` (including RimCrow copies); other backup copies in `Mods/` use mod library "move into the lock library" (`archo library move-to-locked`, 06 §2.9). Both are the same move: point Instance to new location, then scan, preserving ID and all plan references (previously replaced with a new Instance only in current plan). CLI executes without `--dry-run`; UI previews then confirms.

### 7.5 Unpinning

`archo unpin <instance> [--dry-run]`: reference copy or upstream.

- **RimCrow style**: rename Workshop `About/About.xml.disabled` back to `About/About.xml` (refuse without changes if About.xml exists), rescan immediately. Swap copy in current plan to Workshop if present. Leave copy where it is.
- **RimArcho-created**: swap current-plan copy back to upstream if present; retain copy in lock library.
- Current plan only; others unchanged.
- CLI executes without `--dry-run` (like `save`); UI previews then confirms.

Recycling Mod folders: 08 §6–§7.

## 8. Player data and RimCrow import

### 8.1 Player data

| Data | Owner | Description | Command |
| --- | --- | --- | --- |
| Alias | Lineage | Display name in all interfaces; `find`/search match it. Log attribution still matches About.xml name because the game prints `[Name]`, not alias | `archo alias <ref> [<name>] [--clear]` |
| Tags | Lineage | Multiple, case-insensitive; RimCrow groups import as tags | `archo tag add\|remove <ref> <tags…> [--mod <ref>]…`, `archo tag list`; filter `list --filter tag:<tag>` |
| Note | Lineage | Plain text | `archo note <ref> [<text>] [--clear]` |

Storage: `lineage_data` (environment, Lineage, alias, note), `lineage_tag` (environment, Lineage, tag). Lineage-owned, shared by original/locked versions and all plans.

### 8.2 One-time RimCrow import

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
   | `user_rules.json` loadTop / loadBottom (`value` true) | `top` / `bottom` (§5.5) |
   | sign_color, user_mod_type, ignored_issues | No equivalent; count only |

5. **Disabled** RimCrow user rules are excluded unless `--include-disabled-rules`; silently importing would alter sorting.
6. Report mapped counts (path/packageId), success, existing, conflicts; list every unmapped/conflicting item, RimCrow-disabled resource count. Count About.xml.disabled-only directories by source, with handling guidance (rename back or re-enable in RimCrow then `archo scan`; alternatively unsubscribe/delete; RimArcho handling: 08 §6).
7. Repeatable with identical results: skip identical values, deduplicate rules by kind + Lineage. No log-table import needed.
8. `--dry-run` reports only, writes nothing. Default 6 details per category; `--all` shows all.

One local import has run; results: 07 §6.

## 9. List exchange

### 9.1 Import destinations

Every imported list (file, pasted text, share code, Rentry link, Steam collection, save Mod list) is previewed, then offers three choices:

- **import as a new list**: new plan with imported list, v1 source `import`, origin note (filename, `share code`, `collection ws:<ID>`, `rentry <ID>`). Default name from list (share-code name, collection title), otherwise filename. Command `loadout import --new` (05 §4.2), MCP `manage_plans action=import`.
- **replace the whole list**: replace whole list as one reversible edit; no game write.
- **add the ones you lack** (merge): add imported Mods absent from current plan at constraint-suggested positions, preserving existing entries; one reversible edit.

None writes ModsConfig.xml.

Steam collection import adds a checked-by-default "follow this collection from now on" checkbox below preview, linking the collection to the new/current plan afterward (§9.6).

Command: `archo loadout import <file|share code> [--new [<name>]]`; without `--new`, replace current plan.

Preview: format, source, list name, game version (warn "the list is for RimWorld X, this game runs Y" if major/minor differs), total, installed/uninstalled counts. Uninstalled entries get Workshop links (supplied IDs or steamDB lookup) and replacement suggestions (§6.3). Show added/removed/moved/swapped counts for replacing current plan. RimArcho lists (§9.3) include per-Mod versions: installed entries without a local matching version show "another version"; append "N in another version" to total; expand installed group by default.

Resolution: lowercase packageIds; retain `_steam` tokens and resolve under §4.2; unresolved become missing entries. Workshop-ID-only entries map to installed Instances' packageIds (prefer loadable); absent local copies do not become missing entries (no packageId), but show Workshop links in preview/results. If a list contains no official content (Core/DLC), replacement and new-plan import retain current plan's enabled Core/DLC at the front; if none, add installed Core (2026-10-30). Applies because Steam collections, Workshop-link exports (§9.3) and ID-only text cannot contain official content, and handwritten packageId lists often omit it. Preview totals and installed/uninstalled count only list entries; retained Core/DLC count only in replacement changes. If Core or any DLC appears in the list, follow it exactly. Names/Workshop IDs serve missing-entry display and links only. RimArcho lists also include semantic hashes: if packageId resolves to different content but another live/loadable Instance has the exact hash, swap to it (unless already in list).

### 9.2 Import formats

Detect by content, not extension (except `.rws`, read as save).

| Format | Detection | Read |
| --- | --- | --- |
| RimArcho list (`.archolist.json`) | Starts `{`, has `entries` array and numeric `format` | `pack.json` format (08 §5.1): `name` is list name, `game`, `entries[]` fields `package_id`, `name`, `workshop_id`, `hash`; reject newer `format`. Read pack-directory `pack.json` the same way |
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
| RimCrow share code | See §9.4 | |

- JSON entries may be strings or objects with `packageId` / `package_id` / `packageid` / `id`, `name`, `workshopId` / `workshop_id` / `publishedFileId`.
- Retain first occurrence of duplicate packageIds. Detect UTF-16 / UTF-8 BOM and normalize to UTF-8; read files under 16 MB whole.
- If no Mods found, error: "no mods found in it: a list from RimSort, RimCrow or RimWorld, a share code, packageIds or workshop links".
- Pasting one line in UI: existing file path reads file; Rentry link fetches `https://rentry.co/<id>/raw` (4 MB cap; on failure suggest opening page, copying list and pasting); Workshop link or ≥ 6-digit number reads Steam collection (§9.5). Only the latter two use networking.
- Pack (`.archopack`) import: 08 §5.4. Its exported list is a RimArcho list imported here.

### 9.3 Export

`archo loadout export [--format <format>] [--out <file>]` exports current plan. Without `--out`, stdout; existing `--out` files are refused, never overwritten.

| Format | Content |
| --- | --- |
| `archo` | RimArcho list (`.archolist.json`): `pack.json` format (08 §5.1), `light` true, `name` plan name. Entries include `lineage`, `instance`, `workshop_id`, semantic `hash` (Core/DLC only `official`); missing entries only `package_id`. Importer sees which versions match (§9.1). First UI export option, "RimArcho list" |
| `text` (default) | Two `#` header lines (environment, Mod count, time; import instructions), then `<packageId>  # <name>` per line; missing names `missing` |
| `modsconfig` / `rimsort` | ModsConfig XML (`ModsConfigData`: `version`, `activeMods/li`, `knownExpansions/li`); same-package Workshop copy uses `_steam`. RimSort exports this format; `rimsort` is an alias |
| `rml` | RimWorld `savedModList`: `meta` contains `gameVersion`, `modIds`, `modSteamIds` (`0` if absent), `modNames`; `modList` contains `ids`, `names`; load directly in game's Mod menu |
| `rimcrow` | Share code, one line (§9.4) |
| `rimcrow-xml` | RimCrow `ModList.xml`: `modIds`, `modNames`, `modSteamWorkshopIds` (`0` if absent) |
| `workshop` | Workshop links (added 2026-10-23): one `#` header (list name, Mod count, Workshop count), then `- [<name>](https://steamcommunity.com/sharedfiles/filedetails/?id=<Workshop ID>)` per line in load order; final comment lists non-Workshop entries (Core, DLC, local Mods, missing entries). Lets recipients subscribe; imports as Workshop ID list (§9.2) |

`archo` adds `versions`: plan allowed game versions (§1.6), e.g. `["1.5","1.6","1.7"]`. Older RimArcho ignores it.

UI export is a list subpage (06 §2.17): preview each format, then save to file or copy; share codes are copy-only.

Entry serialization matches game writing (§4.1 items 4/5); game version from environment, falling back to ModsConfig.xml.

### 9.4 RimCrow share codes

- Format: `RC-<8 uppercase CRC32 digits>-<unpadded URL-safe base64>`. Also accept old `RMM1-`; strip `share://RC/` prefix.
- Base64 decodes to zlib-compressed JSON: `[1, list name, game version, entries]`. Entries: `"pkg"`, `["pkg","WorkshopID"]` or `["pkg","WorkshopID","name"]`.
- CRC32 covers compressed bytes; reject mismatch ("share code is damaged or incomplete"); reject first element other than 1 (unsupported version).
- Export at maximum compression; named entries as triples, Workshop-ID-only as pairs, otherwise strings. List name is plan name.

### 9.5 Steam collections
- API: `POST https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/`, form `collectioncount=1&publishedfileids[0]=<ID>`, no API key. Sort `children[]` by `sortorder` (gaps allowed); `filetype` 0 = Mod, 2 = subcollection. Noncollection/private IDs have `result` other than 1 and no `children`; report "this is not a collection".
- Fill collection/item titles through Workshop details API, 100 per request; title failures do not prevent comparison.
- Collection URL/ID: take `?id=` or leading ≥ 6 digits.
- `archo collection <URL or ID>` reads and compares locally in three groups: installed (matching Instance, prefer loadable for duplicate Workshop IDs), uninstalled (Workshop links), subcollections (listed, no recursion). Online only on execution.
- `archo collection <URL or ID> --import` imports installed Mods in collection order, destination under §9.1 (currently replace current plan), prefixed with current Core/DLC (§9.1). Uninstalled entries show links, not missing entries.
- Missing items in "can it run" offer "subscribe in the Workshop" links; one-click subscribe/unsubscribe planned for 1.2 (01 §1).

### 9.6 Following Steam collections

Decided 2026-10-23, implemented (checks when opening all lists, due conditions in 06 §2.16.1 A14; scheduled background checks TODO). Plans may **link** one or more Steam collections. When a collection changes, RimArcho tells the player what changed; syncing is the player's choice, never automatic plan mutation.

- **Data**: new `plan_collection` table: plan, Workshop ID, title, sync mode (`add` / `follow`), last check time, **snapshot** (collection entries/order when last linked or synced).
- **Link**: paste collection URL or check "follow this collection from now on" during import (§9.1). Fetch once and snapshot current content, so newly linked collections have no changes. Link each collection once per plan.
- **Check**: online only when opening all lists (interval: 06 §2.16.1 A14) or `archo plan sync --check`, same API as §9.5. No launch check or background polling.
- **Changes**: compare current collection against snapshot: added (not in snapshot), removed (not in current). A player-disabled collection Mod is not a change; comparison is against snapshot, not plan.
- **Sync**: one reversible edit `sync collection <title>`, then update snapshot to current collection.
  - `add` (default, UI "add new only"): enable installed additions at constraint-suggested positions, like adding missing Mods (§9.1). Retain collection-removed Mods in plan.
  - `follow` (UI "also disable dropped"): also disable collection-removed Mods still in plan. Disable only; no unsubscribe or deletion.
  - Uninstalled additions show Workshop links, not missing entries (§9.5).
- **Uninstalled count**: collection entries without any local copy, with "subscribe in the Workshop" links.
- **Unlink**: delete association/snapshot, preserve plan. Deleting a plan also deletes associations.
- **Commands**: `archo plan follow <name> <collection URL or ID> [--mode add|follow]`, `archo plan unfollow <name> <collection>`, `archo plan sync <name> [<collection>] [--check|--dry-run]`. `--check` checks/lists changes only; `--dry-run` shows proposed enables/disables. MCP has corresponding tools; `sync` without `--check`/`--dry-run` is marked as mutating a plan.

## 10. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| S1 | Sort an already sorted list | Order unchanged | Pass (`TestS1SortedIsStable`) |
| S2 | Two Mods mutually declare loadAfter | `order.cycle`, identifying each declaration's About.xml | Pass (`TestS2CycleAndS3Ignore`, `TestSortCycleIgnoreAttach`) |
| S3 | User rule suppresses one declaration | Sort succeeds, satisfying remaining constraints | Pass (same tests) |
| S4 | Translation `attach` to original Mod | Translation immediately follows original | Pass (`TestS4Attach`) |
| S5 | Sort 500 Mods | ≤ 1 second (measurements: 07 §6) | Pass (`TestS5FiveHundred`) |
| S6 | Cycle includes community rule and About.xml declaration | Suggest suppressing community rule | Pass (`TestCycleSuggestPrefersCommunity`) |
| S6b | Cycle has dependency and loadAfter; current order violates dependency | Sort satisfies dependency, drops/suggests suppressing loadAfter | Pass (`TestCycleBreaksWeakestEdge`) |
| S7 | Odyssey last, other official content first | Sort puts Odyssey after Anomaly; first `official.order` suggestion `archo move <Odyssey> --after <previous official package>` clears finding | Pass (`TestOrderSeverityAndDLCMove`, `TestOfficialMoves`) |
| S9 | Community requires Royalty after Biotech | No cycle; rule listed "not used"; official order unchanged | Pass (`TestDropContradictsOfficialOrder`) |
| S10 | Mod declares loadBefore Core and depends on a Mod after Core | No `order.cycle`; loadBefore "not used" | Pass (`TestDropBeforeOfficialInCycle`, `TestSortDropsUnsatisfiable`) |
| S11 | One violation each of dependency order, loadAfter, `superniquito.modoptionssort` loadBefore Core | Dependency warning; loadAfter info; modoptionssort info, explaining no extra corpus errors when violated | Pass (`TestAssess`, `TestOrderSeverityAndDLCMove`) |
| S13 | Community bottom rule, another Mod user-top, top Mod needs library | Top and library first after DLC, bottom and dependents last, others preserve order; ignoring community bottom returns normal tier; repeat sort stable | Pass (`TestTiers`, `TestSortTopBottom`) |
| V1 | Start RimArcho after outside ModsConfig.xml edit | New `external` version; none if unchanged | Pass (`TestExternalChangeAndConflict`) |
| V2 | Select first history step after three edits | Workspace returns to after first step; `ctrl+r` advances | Pass (`TestHistorySteps`) |
| V3 | Select v1 in history | Show diff; after confirmation workspace equals v1 | Pass |
| V4 | Game changes only ModsConfig.xml formatting (indentation) | No new version | Pass (`TestExternalChangeAndConflict`) |
| V5 | Outside edit with unsaved plan changes | Record `external`, do not load, flag conflict; checkout clears flag | Pass (`TestExternalChangeAndConflict`, per environment) |
| V6 | Plans A/B; write A, edit in game Mod menu, current B | Load outside content as A `external`; B unchanged | Pass |
| V7 | V6 but A edited again after write | A records `external` and conflict; diff/load/ignore available | Pass |
| V8 | Two environments share user data; environment 1 writes, open 2 | No outside-change report | Pass |
| V9 | Create (empty/current/v<n> copy), switch, rename, delete plans | Independent workspaces/undo/versions/staging; cannot delete last; switching writes no game files | Pass |
| V10 | Migrate old database with `loadout_tag` | One plan/environment retains history; each tag becomes new plan | Pass |
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
| D1 | Local HTTP fixture serves three datasets | Download, parse, `data status` entry counts | Pass (`TestCommunityData`) |
| D2 | Second dataset invalid JSON | Entire operation fails, old files unchanged | Pass (`TestCommunityData`) |
| D3 | Enabled Mod in replacements | "can it run" shows "use X instead" | Pass (`TestCommunityData`) |
| X7 | `import rimcrow` twice | Second all "already exists"; disabled rules excluded unless `--include-disabled-rules` | Pass (`TestImportRimCrow`) |
| S8 | Printed ignore reference | Usable unchanged in `rule add ignore` | Pass (`TestPrintedIgnoreRefsRoundTrip`) |
| P1 | supportedVersions intersection 1.5/1.6, created under 1.7 | Default 1.5–1.7; empty intersection only 1.7 | Implemented |
| P2 | Player sets 1.6/1.7 then enables 1.4-only Mod | Allowed versions unchanged; `plan versions <name> default` recalculates | Implemented |
| P3 | Game 1.7, plan allows 1.5–1.6 | "for you to confirm" in "can it run" offers "add 1.7"; launch unblocked | Implemented |
| P4 | Export `archo`, import new plan | Allowed versions preserved | Implemented |
| C1 | Linked collection adds 3 (2 installed), removes 1; sync `add` | Enable 2 at suggested positions, link uninstalled 1, retain removed; one undoable edit; updated snapshot, next check unchanged | Implemented |
| C2 | C1 with `follow` | Also disable removed 1; no unsubscribe/deletion | Implemented |
| C3 | Player disables collection Mod, collection unchanged | No changes | Implemented |
| C4 | Unlink | Plan unchanged; association/snapshot deleted | Implemented |
