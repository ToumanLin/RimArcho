# 07 · Plans

> Status: implemented in the CLI, MCP and GUI: plans shared across all games (§1), editing and staging (§2), history (§3), writing to the game and outside changes (§4), locked versions (§5), player data and RimCrow import (§6), and list exchange with followed Steam collections (§7). Not implemented: background checks of followed collections (§7.6).
> Basis: 00, 00a, 01, 02, 04 §2–§3, 05 §4, 09 §2–§5, 10, 13 §4, 16 §2.5, 17 §6, 18.

A plan is an ordered list of enabled Mods. This document covers storage, editing, undo, writing to the game's `ModsConfig.xml`, locked versions, player data and list exchange. Sorting and rules: 08. Launch flow: 09 §2. UI: 16 §2.5.

## 1. Plan model

### 1.1 What a plan is

- Plans are **shared by all games (environments)** (§1.7): every game shows every plan. The first plan is `default`. Names are case-insensitive and unique across all plans.
- Content: an ordered list of entries, each an Instance or a **missing entry** (a packageId that does not resolve to an Instance, kept as it is, §4.2).
- Plans store Instances, not packageIds: the player chooses "this copy" (00a §4).
- Plans have no saves of their own: all plans use the environment's saves and settings. Build a pack for separate saves (10).
- Edits **persist immediately** in RimArcho's database and survive closing the program; every step is undoable (§2.1). Saving is separate: it happens only on plan launch (09 §2) or `archo save` (§4.1).
- Each environment remembers one **current plan** (any plan): the editor's plan and the default target of CLI/MCP commands. Two games may share a current plan.
- User rules (08 §2.1) and player data (aliases, tags, notes, §6.1) belong to Lineages and are shared by all plans; a plan decides only what is enabled and in what order.

### 1.2 Data owned by each plan

Each plan owns its workspace, undo stack, version history and staging. When confirming an identity merges two Mods, these records move to the retained ID (04 §2.4).

| Data | Content | Storage |
| --- | --- | --- |
| Plan | Name, creation environment, created time, last modified, last launched | `plan` table (`id`, `environment_id`, `name`, `created_at`, `updated_at`, `created_game`, `versions`). `environment_id` is only the game where the plan was created, not where it can be used; the application keeps names unique across all plans |
| Workspace | Current list; originating version (`origin_version`, `origin_kind`: `import`, `external`, `checkout`, `edited`, `save`); unloaded outside version (`conflict_version`, §4.3) | `workspace`, one row per plan |
| Undo stack | `undo` and `redo`, at most 100 steps each; each step stores the complete list before the edit, the staging (entries, order, original positions and anchors) and the operation name | `workspace_undo`, per plan |
| Version history | `v<n>`: time, source (§3.1), complete list, game-version string, notes | `loadout_version`, numbered per plan |
| Staging | Staged Mods and original positions (§2.2) | `stage:<plan ID>` in `meta` |
| Current plan | One per environment | Field on the environment |

Data not owned by plans:

| Data | Owner | Notes |
| --- | --- | --- |
| ModsConfig.xml write record | **User data directory** | Last written hash, last seen hash, writer (plan and version), write time (§4.3). `modsconfig_state` has one row per user data directory. The writer is "what the file contains now": the written version after saving, or the `external` version after loading or recording an outside change (otherwise the next outside change would be wrongly classified as a conflict) |
| Launch records | Environment | `launch` records environment, time, method and command line, plus plan, list fingerprint and version number (no plan for packs); filled on launch (09 §2) |
| User rules | Lineage (global) | `user_rule`, 08 §2.1 |
| Player data | Lineage | `lineage_data`, `lineage_tag`, §6.1 |

### 1.3 Managing plans

| Action | Rules |
| --- | --- |
| Create | From an empty list, a copy of the current plan, a historical version, an imported list (§7.1) or a save's Mod list (12 §4). The first version has source `import`; its note names the origin (copies: "copied from <plan> v<n>") |
| Switch | Changes only the environment's current plan, never game files. The current plan is used for launch, game writes and CLI/MCP plan commands. **Opening is not switching**: the GUI sidebar, command panel and all lists, and in the TUI `P`, the command panel or the editor's `tab`, only open the plan in this window for viewing and editing (the session's open plan, `App.OpenPlan`); the current plan stays as it is. A plan becomes current when it is chosen on launch, when the player clicks "make current", or when it is saved or launched from its editor. A new plan is only opened. CLI `--open-plan <name>` targets another plan for one command without switching (used by the GUI's analysis and pack-build child processes). What the game contains follows the user data directory's write record, shown as "in the game now: <list> v<n>" (GUI: a green dot on that sidebar child when there are several plans, 16 §2.5; TUI: status bar) |
| Rename | Name only; history unchanged |
| Delete | Needs player confirmation; the last plan cannot be deleted. Its workspace, undo stack, versions and staging are deleted too; games using it as current plan switch to the oldest other plan |

The GUI gathers these actions on all lists (16 §2.17). Commands: `archo plan list`, `archo plan new <name> [--from current|v<n>|<file>]`, `archo plan use <name>`, `archo plan rename <old> <new>`, `archo plan remove <name>`. Other CLI/MCP plan commands (`enable`, `sort`, `loadout log` and so on) target the current plan (13 §4).

### 1.4 First use

- When an environment is read and has no current plan (`environment.current_plan = 0`):
  - If ModsConfig.xml still matches a plan's last write (its hash equals the write record), make that plan current; create none.
  - If ModsConfig.xml is absent (for example, a new game with its own saves) and plans exist, make the most recently edited plan current; create none.
  - Otherwise read ModsConfig.xml and create a plan: v1 has source `import` and note `ModsConfig.xml` (absent file: an empty list with note `no ModsConfig.xml`). It is named `default`; if that is taken, the environment's name, then numbered (`1.6 2`). The first-run guide also offers "move over from RimSort or RimCrow" and "open a pack or a Steam collection" (16 §2.15).

### 1.5 Archo Monitor added by default

Archo Monitor is RimArcho's bundled in-game helper Mod (19; installation and updates: 19 §6). **On by default** means RimArcho adds it to plans; after that it is an ordinary Mod.

- **Setting**: `monitor.default` (`on` / `off`, default `on`): `archo config set monitor.default off`, or the settings "game log" group (16 §2.14). It controls only automatic addition, not updates (19 §6).
- **Plans affected**: new plans (from any origin: empty, copy of the current plan, history, imported list or save), and every existing plan in the environment when Monitor first becomes available. When to check: "Check timing" in 19 §6; new plans right after creation.
- **Once per plan**: processed plans are recorded in meta `monitor.added:<plan ID>` (added, or already present). The player may then disable, move or stage it as usual; RimArcho adds it again only when sorting (next item).
- **Sorting adds it too**: if the setting is on and Monitor is installed but not in the plan, every sort (`archo sort`, the editor, can it run) places the selected copy (19 §6) after Harmony in the same undo step, even if the player removed it; turn the setting off to avoid this. If that is impossible (no Harmony in the plan, or the selected copy needs a link and `Mods/` cannot hold links), the sort runs normally and gives the reason (`SortResult.MonitorSkipped`, a GUI notice and `archo sort`). Test: `TestSortAddsMonitor`.
- **Position**: right after Harmony (`brrainz.harmony`). Monitor's About.xml declares `loadAfter brrainz.harmony` and `loadBefore Ludeon.RimWorld`; sorting (08 §1) keeps it after Harmony and before Core, so it observes the loading of Core and every Mod.
- **Neither add nor mark processed** (retry at the next check) if the setting is `off`, Harmony (required) is absent, the selected copy needs a link that `Mods/` cannot hold (FAT, exFAT, 19 §6), or Monitor is not installed.
- **One reversible edit**, named `add Archo Monitor after Harmony (default; archo config set monitor.default off)`. For a plan that is not current, the edit goes into that plan's own undo stack; `archo undo` there reverses it. The result (CLI output, UI notices) lists the plans affected. Tests: `TestMonitorInstallAndDefault`, `TestMonitorDefaultOff`.

### 1.6 Allowed game versions

Each plan records **allowed game versions** (major.minor, such as 1.5, 1.6, 1.7): which versions the list is meant for. UI: all lists (16 §2.17).

- **Default**: intersect the `supportedVersions` declared by all Mods, add the game's major.minor at creation, then take the continuous range from lowest to highest. Example: intersection 1.5, 1.6, created under 1.7 → default 1.5–1.7.
  - Official content (Core/DLC declare only their own version), missing entries and Mods without `supportedVersions` are left out of the intersection.
  - Empty intersection: only the creation version.
  - Defaults are not stored. They are recalculated from the current list on every read, so enabling or disabling Mods changes them.
- **Player override**: the chosen set is stored and need not be continuous. It stops following the Mods until reset.
- **Storage**: `plan.created_game` (the game's major.minor at creation) and `plan.versions` (comma-separated selection; empty means default).
- **Copying a plan** copies the player's selection. **Import** uses `versions` from a RimArcho list if present (§7.3), otherwise defaults; `created_game` is the current environment's version.
- **Uses**:
  - Plan versions show on Launch and all lists (16 §2.3, §2.17).
  - If the current game is outside the allowed set, "for you to confirm" in "can it run" shows "<list> allows 1.5–1.6; the game now is 1.7", with the actions "add 1.7" and "go to list management". It does not block launch.
  - Exported RimArcho lists include `versions` (§7.3).
  - `version.unsupported` (05 §4.3) still checks the current game version, independently.
- **Commands**: `archo plan versions [<name>]` shows the allowed versions and, for each, how many Mods do not declare it; `archo plan versions <name> <versions…>` sets them, for example `1.5 1.6 1.7` or `1.5-1.7`; `archo plan versions <name> default` resets them. Version settings are not list edits and do not enter undo stacks; the UI status offers "undo", which restores the previous value. MCP has corresponding tools.
- **Warning counts**: `PlanVersions.Missing` and the version notices in all lists leave out ignored version warnings and list them separately as `Muted` (05 §4.2). If all are ignored, say "no version warnings", never "every Mod supports it". `Common`, defaults, allowed versions and player selections still use the actual declarations: ignoring a warning does not count as support.

### 1.7 Plans shared across all games

A plan is only "what is enabled and in what order", independent of game directories, and allowed game versions (§1.6) say which versions it is meant for. So plans do not belong to environments.

- **All lists, the sidebar and the Launch page** show every plan, not grouped by game, with its allowed versions; "can it run" warns when the current game is outside them (§1.6) instead of hiding plans.
- **Reading entries in another game**: plans store Instances. Instances in shared folders (Workshop, SteamCMD, extra folders, lock library, Monitor) are global and recognized by every game. Another game's own Instances (Core/DLC in its `Data/`, its `Mods/`) read as missing packageId entries, then resolve to this game's copy under §4.2 (`app.localizer` + `Registry.Heal`); entries without a copy here stay missing. Reading does not change storage. Editing here stores this game's Instances; going back resolves them by packageId in the same way.
- **Links on game switch** (03 §2 item 14): Mods of the current plan from the previous game's `Mods/` are still linked into the target game as `_Archo_env_<Instance ID>`.
- **Other games' `Mods/` are usable in every game**: each game's index also loads the live Mods in other games' `Mods/` (except `_Archo_*` links), with the in-memory source type `elsewhere` (`source.Elsewhere`, not stored; label `local-<environment name>`, `app.withElsewhere`). They appear in the Mod library (source `local-<environment name>`, location that game's `Mods/`, 03 §2 item 11) and in the editor's Disabled. Once enabled, they deploy like SteamCMD and extra-folder Mods: saving links them into this game's `Mods/` as `_Archo_env_<Instance ID>` (`DeployName`); the links stay on save and on disable (they are the same links as those made on game switch). If this game already has an Instance of the same folder (a link scanned earlier became a local Instance), only this game's copy is shown. If this game's `Mods/` has the same package ID, the other game's plan entry reads as this game's copy (`Index.Elsewhere`; "retain target copy with same package ID" in 03 §2 item 14). The read-only `Data/` is not shared between games. Test: `TestOtherGamesModsUsableAnywhere`.
- **Write records** also store the game that wrote (`modsconfig_state.writer_env`). Outside changes are resolved in that game (§4.3); "in the game now" includes its name when another game sharing the directory wrote it.
- **Deleting an environment** keeps the plans: before its Instances are deleted, entries referencing them in workspaces, versions and undo steps are rewritten to packageIds (`store.keepLists`).
- Tests: `TestPlansSharedAcrossGames`, `TestPlanStore`.

## 2. Editing

### 2.1 Operations

Every operation is a command shared by the GUI, TUI, CLI and MCP (13 §1). Each operation that changes the list or the staging is one undo step; an operation that changes neither records none.

| Operation | Command | Rules |
| --- | --- | --- |
| Enable | `archo enable <refs…> [--at <n>\|--before <ref>\|--after <ref>]` | Check loadability first (§2.3) and explain refusals. Skip Mods already enabled, with a notice. With an explicit position, insert several Mods one after another in the order given. Otherwise replace a missing entry with the same packageId in place; if there is none, place each Mod by the sorting constraints (`order.Place`, the same as the editor's suggestions, `App.SuggestAt`, 16 §3): right after the last enabled Mod it must follow (never before official content); if it only has enabled Mods that must follow it, before the first of those (libraries, dependencies, DLC, so a later-enabled Odyssey returns to official order); with no constraint, at the end; with `attach`, right after its target. Corpus replay (remove the DLC and 3 random Mods from each log's list, then enable them again): this rule puts 95% of DLC and 17% of other Mods back in their original positions (median distance 31); "the latest position the constraints permit" gives 15% (median 35); always appending gives 0% / 2%, and the other Mods then violate 83,000 strong constraints (739 with this rule) |
| Disable | `archo disable <refs…>` | Accepts `pkg:<packageId>` to remove a missing entry (with or without `_steam`). Notice if already disabled |
| Move | `archo move <ref> --to <n>\|--before <ref>\|--after <ref>\|--up\|--down` | Needs exactly one position form; a disabled entry is an error that suggests `archo enable` |
| Swap instance | `archo swap <ref> [<ref>]` | Replace an enabled Instance with another of the same Lineage, keeping its position. The references may come in either order. With one reference, use the only other loadable Instance; if there are several, list them. Different Lineages are an error that suggests `archo lineage merge`, or disabling one and enabling the other |
| Sort | `archo sort [--dry-run]` | 08 §1; `--dry-run` only shows the result |
| Stage | `archo stage add/drop/restore/list` | §2.2 |
| Undo / redo | `archo undo [<n>]`, `archo redo [<n>]` | Up to 100 steps per stack, shared by all interfaces. A new edit clears `redo`. `checkout` (§3.2) and replacing the list by import (§7.1) are reversible edits too |
| History | `archo history [--all]` | Unsaved edits and versions (§3.3); the latest 10 versions by default |

- Two processes (for example the GUI and MCP) may share one RimArcho data directory (`%APPDATA%\RimArcho`, 17 §3). When the database change key (`data_version` + `total_changes`) changes, the next read sees the other process's changes; an edit made from a stale view does not overwrite them. Test: `TestTwoProcessesShareTheWorkspace`.
- Undo and redo snapshots keep the Instance bindings the list stored (including Core, DLC and Mods owned by the game), not the copies resolved for the current game. They are restored unchanged, then resolved for display in the current game. Edit batches follow the same rule.
- Plan reads are cached until the database or ModsConfig.xml (size, modification time) changes.

**SteamCMD batch handoff**: after the player accepts credit replacement (11 §1), Instances in lists are replaced by the corresponding Workshop Instances, keeping their positions. This happens only while the game is stopped and only if every environment that can load the old copy can load the new one. Current content, historical versions, undo/redo, staged entries and position anchors change in one transaction. The handoff creates no undo step that would restore the old copy; references in history alone do not keep it. If any environment fails the condition, the whole item is kept and no shared list changes; corrupt staging likewise ends that item's handoff and keeps it. Fixed packs, game deployments not yet handed off and incoming links keep their original identity and copy, with reasons. Existing game projections reuse §4's writer and registry for `_steam`: they replace only this item and detach only its own deployment link, without committing other unsaved edits or switching the player's current list. Notes are merged inside the remapping transaction. DDS ledgers do not move across physical directories; the result suggests optimizing again. List edits and the handoff share the launch lock, held from the read through the save. A late snapshot or long-lived edit batch that conflicts with the handoff must read again and never write old IDs back. Deployment obligations are first stored in SQLite so the work can resume after exit. Full flow and collection conditions: 11 §1.

### 2.2 Staging

Staging is one persistent list per plan that is **never written to the game**. It has two uses:

- **Preparation**: hold newly subscribed Mods until they are ready to enable.
- **Troubleshooting**: move a batch from Enabled into staging, then put them back at their original positions in one action.

Rules:

- Staging holds only **disabled** Mods. Enabling a staged Mod again (enable, undo, checkout or import) removes it from staging for good, in the same transaction; disabling it later does not bring it back.
- A Mod staged from Enabled remembers its original 1-based position and the Instance before it (its anchor; empty if it was first or followed a missing entry, in which case the position is used). If the anchor itself was staged earlier from between entries, the anchor moves to that Mod, so restoring keeps their original order.
- Mods staged from Disabled have no original position (0).
- A Mod is restored after its anchor; if the anchor is staged, follow the chain of anchors; if none is found, use the original position. When restoring a batch, Mods whose anchors are in the batch come back with them, the others by original position; entries without a position are appended in staging order. Anchor traversal is bounded so that edits cannot make it cycle.
- With a position argument (`--at`, `--before`, `--after`), only one Mod is restored, at that position.
- Loadability is checked before restoring (§2.3).
- Each staging add, drop or restore is one reversible edit, including staging from Disabled and removing without enabling. Each step records the full entries and staging state (entries, order, original positions, anchors), and undo or redo restores both in one transaction. Other list edits also record the staging, so undo can bring back items removed by enable, checkout or import. Combined batch edits follow the same rule.
- Old undo steps without a staging snapshot restore only the entries, keep unrelated staged items, and remove for good the staged items enabled in the current or target list. Old positions and anchors are not guessed; the reverse steps recorded from then on store full snapshots.

| Command | Meaning |
| --- | --- |
| `archo stage list` | Staged Mods and original positions |
| `archo stage add <refs…>` | Stage; enabled Mods leave the list and remember position. One reversible edit |
| `archo stage drop <refs…>` | Remove from staging without enabling. One reversible edit |
| `archo stage restore <refs…> \| --all [--at\|--before\|--after]` | Restore; `--all` restores everything; refuse if neither references nor `--all` supplied (13 §3.3). One reversible edit |

Tests: `TestStageRestoreAllKeepsPositions`, `TestStageRestoreOneFollowsAnchor`, `TestStageRestoreAllRandomBatches`. Staging column and keys: 16 §2.5.

### 2.3 Loadability

An Instance admitted to a plan must be live (not residue, not `unloadable`) and either visible to the game or deployable by RimArcho:

- In `Data/`, `Mods/` or subscribed Workshop content; aliases there count (for example a SteamCMD directory junction in `Mods/`), RimArcho's own `_Archo_` links do not;
- Or **deployable**: source `steamcmd` or `extra`, linked into `Mods/` as `_Archo_` when writing to the game (09 §3).

Refusal explanations for `enable`, `swap` and `stage restore`:

| Condition | Explanation |
| --- | --- |
| Tombstone | No longer on disk |
| `unloadable: about-disabled` | About.xml was renamed; the game cannot read it (handling: 09 §5) |
| Residue | The Workshop folder is no longer subscribed; the game ignores it |
| Source neither read by the game nor deployable | The game does not read Mods from `src:<label>`; link or copy it into `Mods/` |
| Another copy with the same packageId in the same location | `pkg.duplicate-installed`: the first by folder name wins; the game ignores this copy (05 §4.3) |
| Workshop and non-Workshop copies of the same packageId | Both cannot be enabled (the game keeps only the unsuffixed copy, §4.1 item 4); suggest `archo swap <enabled> <this copy>` |

## 3. History

### 3.1 Versions

Each plan version `v<n>` records time, source, complete list, game-version string and notes. Numbers increase within a plan.

| Source | Meaning |
| --- | --- |
| `import` | First list from outside: the first ModsConfig.xml read (§1.4), or a new plan from a list file, share code, collection, save or copy (§1.3, §7.1). The note names the origin |
| `external` | Outside change: the game's Mod menu or another tool changed ModsConfig.xml; attributed to the plan that last wrote it (§4.3). Note `ModsConfig.xml changed outside RimArcho`. This is also the startup snapshot: outside changes found when RimArcho starts use this source, not a separate one |
| `user` | List written to the game by `archo save` (UI "save") |
| `undo` | A historical version checked out, then written without further edits. Note `restored v<n>` |
| `launch` | List written on plan launch (09 §2). If it equals the plan's latest version, no version is created and the launch record points to that one. Launching after a checkout without edits gives a new version with note `restored v<n>` |

Undo steps are not versions: versions are lists written to or read from the game; undo steps stay inside RimArcho.

A plan's **unsaved changes** are the difference between its latest version and the workspace. This is for information and blocks nothing; launch writes the current list (09 §2).

### 3.2 Commands

| Command | Meaning |
| --- | --- |
| `archo loadout log [--all]` | The current plan's versions: number, time, source, entry count, notes; unloaded outside versions are marked |
| `archo loadout diff [<v>] [<v>]` | Four categories: added / removed / moved / instance swaps. Default: latest version against the workspace. Versions accept `v3` or `3`; the workspace accepts `workspace`, `ws`, `current` |
| `archo loadout checkout <v>` | Load a version into the workspace as one reversible edit; nothing is written to the game. A game write right after it uses source `undo` |

If the loaded version, resolved for the current game, equals the workspace list, checkout does nothing: origin, conflict, staging and undo/redo stay unchanged. The UI says only "no changes" and offers no undo.

Diff algorithm: a changed Instance within one Lineage is an "instance swap". Compute the longest common subsequence of the remaining shared entries; only entries outside it are "moved". Moving one Mod therefore reports one move, not a move for every entry whose position it shifted.

### 3.3 History area logic

UI: the plan editor's third column (16 §2.5). Data and behavior:

- **Unsaved steps**: the undo stack, one row per step, newest first. A cursor marks the current state; `u` / `ctrl+r` moves it. Selecting and confirming an earlier step undoes back to it (`StepTo`), stopping at the bottom of the stack. Clicking an unsaved step applies it at once; `u` / `ctrl+r` can still go back.
- Step titles are derived from the lists before and after (for example "enable Harmony · position 2"), not stored separately.
- **Versions**: one row per version with its source. Selecting one shows its diff against the workspace; confirming runs `loadout checkout`. Loading a version asks for confirmation first.
- Unloaded outside versions (§4.3) can also be compared, loaded or ignored here.

## 4. Writing to the game

### 4.1 ModsConfig.xml write rules

Shared by **plan launch** (09 §2) and **`archo save`** (write without launching, for players who launch through Steam or elsewhere; UI "save").

1. **Synchronize first**: check for outside changes before writing (§4.3), so outside content is kept as a version rather than silently overwritten. **Refuse while there are unloaded outside versions.** If the target plan has one, or the file holds another plan's unloaded outside content, `save` and `launch` (including `--dry-run`) refuse without writing and give the diff / checkout / ignore commands (13 I8): writing would overwrite changes the player has not reviewed.
2. **Prechecks**: refuse an environment without a user data directory, suggesting `archo env set --user-data <directory>`; refuse an existing file that cannot be read, asking the player to repair or move it. Write paths must pass the safety-boundary checks (02 §1).
3. **Synchronize links**: before writing ModsConfig.xml, synchronize `_Archo_` links and remove pack `!ArchoPack_` links (09 §3).
4. **`activeMods`**: lowercase packageIds in plan order. When a packageId has both Workshop and non-Workshop copies, write `<packageId>_steam` if the plan chooses the Workshop copy, otherwise the unsuffixed `<packageId>`. Reason: the game registers official content, then `Mods/`, then subscribed Workshop content, in folder-name order within each group. A Workshop duplicate of a registered non-Workshop copy gets `_steam`; other duplicates are rejected (17 §6). With both enabled the game keeps only the unsuffixed copy, so a plan cannot contain both Instances (§2.3).
5. **Missing entries** are written unchanged. An Instance in the plan that the game no longer registers (deleted or disabled) is written as its packageId, so it does not silently vanish; it heals back to an Instance when it resolves again (§4.2).
6. **`version`**: the current environment's game version, the full `Version.txt` string, as the game writes it; keep the original if the environment has no game version. The game **discards the whole list** and resets it if major.minor differs (17 §6), so the current version is written every time.
7. **Other content**: replace only `<version>` and `<activeMods>`, keeping `knownExpansions`, unknown elements, BOM and newline style; XML-escape text. If the file is absent, create it in the game's format, with the installed official DLC in `knownExpansions` (live official Instances other than Core).
8. **Backup and atomic replacement**: back up the original to `<user data directory>/Config/ModsConfig.xml.rimarcho.bak`, keeping only the latest (the full history is in RimArcho's database). Write the new content to a temporary file in the same directory, flush it, then rename it over the original.
9. **Record**: add a plan version (`archo save`: `user` or `undo`; launch: `launch`, §3.1); record the hash, writer (plan and version) and time for the user data directory (§4.3); set the workspace's origin to this version and clear the unloaded-outside-version marker.

Preview and execution:

- `archo save --dry-run` and the command panel's "preview saving" show the diff from the latest version, the version rewrite ("will change 1.6 to 1.7"), the missing count, and the links to create and delete. The preview is read-only and has no "continue".
- UI "save" (`w`) runs at once, like CLI `archo save`; the status reads "saved as vN". Refusals from items 1 and 2 show the reasons and commands without asking for confirmation (16 §6). Whether MCP may save is up to the host (14).

If the `version` read has a different major.minor than the game, the status bar and `archo status` warn: "the list was saved for X; the game resets it when it starts — launching from RimArcho or saving once saves it for the current version". This blocks nothing (launch always writes the current version, 09 §2).

### 4.2 Instance resolution on reading

When reading ModsConfig.xml (and imported lists, §7), resolve each entry in the game's order (17 §6):

1. Match the full ID first. `<pkg>_steam` maps to a Workshop Instance only if a non-Workshop copy with the same name also exists.
2. Otherwise match the **folder name** and rewrite the entry to that Mod's packageId.
3. If still unmatched and the entry ends in `_steam`, strip the suffix and try again. This covers a deleted local copy that leaves only the Workshop copy.
4. When several Instances match a plain packageId, prefer the non-Workshop one. With duplicates in the same source (for example two in `Mods/`), the game accepts only the first in folder-name order, logs an error and ignores the others. Both cases give `pkg.duplicate-installed` (05 §4.3).

- When an Instance appears more than once, only its first position is kept (the game disables the later one).
- Unresolved entries stay as **missing entries**, shown with `dep.missing`-style findings (05 §4.3), never silently dropped.
- **Healing**: when a later scan finds an Instance the entry resolves to, the missing entry becomes that Instance automatically (unless it is already in the list).
- **Installed deployable copy**: an unbound packageId does not automatically pick a copy outside the game's registry (for example in another game's `Mods/`). `App.UseInstalled(packageIDs)` binds the copies marked `CanEnable` in `MissingMods` in place, as one reversible batch edit; duplicates keep their first position. Missing status, loadability and conflicts with an enabled copy of the same package are checked again when it runs; the batch is rejected if any request is unavailable. Binding creates no links; save and launch deploy them under 09 §3. UI contract: 16 §8.7.

These rules come only from decompilation; local and Workshop copies of the same package side by side, and `_steam` entries, have not been verified in the game (01 §4.2).

### 4.3 Outside changes

Several plans and environments can share one ModsConfig.xml (the default user data directory, 03 §3.4). RimArcho records the last written hash, the last seen hash and the writer **per user data directory** (game, plan, version; the game is recorded separately because plans are shared, §1.7).

**When to check**: every scan, GUI/TUI startup and command that reads a plan.

**Decision**:

1. The hash equals the last seen hash: unchanged.
2. It equals RimArcho's last write: update "seen" only.
3. Neither, but the parsed list and `version` equal the writer's recorded version (a format-only change, for example the game rewrote the file): not an outside change; update "seen" only.
4. Otherwise it is an **outside change** (for example the player edited the in-game Mod menu), attributed to the **plan that last wrote** the file (entries resolved in its environment, §4.2):
   - Plan **not edited** since the write (the workspace still equals the written version): load the file as its `external` version and into the workspace; undoable. Notice: "ModsConfig.xml changed outside RimArcho: loaded as <list> v<n>". If Mods were removed (the game resets to vanilla and DLC when Mod loading fails), add "; N mods left the list (v<m> has them)". A "for you to confirm" card in "can it run" ("ModsConfig.xml was changed outside RimArcho: N mods left the list") names the removed Mods and offers "see the difference" (`loadout diff v<m> v<n>`), "put back v<m>" (one undoable step) and "keep the change". It goes away when the player answers, edits the workspace or checks out another version.
   - Plan **edited** since the write: record only an `external` version without loading it; mark it as an "unloaded outside version" and report the conflict. The player can:
     - **Diff**: `archo loadout diff v<n> workspace`;
     - **Load**: `archo loadout checkout v<n>` (reversible edit), which clears the marker;
     - **Ignore**: `archo loadout ignore-external`, which clears the marker but keeps the history. (The marker blocks game writes, §4.1 item 1, so no write ever clears it.)
   - The marker shows in `archo status`, `archo loadout log`, the status bar and the plan page.

- Without a write record (a new user data directory or a lost record), the change is attributed to the current environment's current plan and handled as in the two cases above; that plan becomes the writer. The same applies if the writer plan was deleted.
- The first read with no plan imports under §1.4; it is not an outside change. Import does not mark the file "seen", so if a file written by another environment's plan is then changed, the change is still attributed to that plan.

Implementation: `internal/app/external.go`, called during synchronization on every plan read. Markers appear in `archo status`, `loadout log`, "can it run" (with a diff action), the sidebar and the plan page; markers on other plans are listed too, with `plan use <name>` or `--env <name>`. Tests: `TestExternalChangeAndConflict`, V6–V8.

## 5. Locked versions

A locked version fixes **one Mod** to specific content that Workshop updates do not change. It is not a new identity concept, just another Instance in the same Lineage (04 §3). Players see the words "locked version"; `pin` creates one, and `copy-local` is an alias.

### 5.1 Two forms

| Form | Shape | Detection |
| --- | --- | --- |
| Created by RimArcho | A copy directory in the **lock library**, with `.rimarcho/origin.json`; the Workshop copy is unchanged. Setting `lock.dir`, default `%LOCALAPPDATA%\RimArcho\locked`, shared by all environments (03 §2) and registered as the shared `extra` source `src:locked`. Copies, like their upstream, are global records any game's plan can use. Per-environment subdirectories `<lock.dir>/<environment>` left by earlier versions are shared folders `src:locked-<environment>`; their copies are still locked versions. The game sees the copies only through `_Archo_` links (09 §3) | The scan reads `origin.json`, merges the copy into the same Lineage under R5 and marks it "confirmed" (04 §2.1). `origin_instance` is the upstream. Copies with `origin.json` in `Mods/` (made by earlier versions) still count |
| RimCrow style | The Workshop copy's `About/About.xml` renamed to `About.xml.disabled` (invisible to the game), with a local copy in `Mods/` (often `_<WorkshopID>_`) | The same Lineage has an about-disabled Workshop Instance and another live, loadable non-Workshop Instance (a RimCrow copy, a `Mods/` copy or a deployed copy; if several, prefer the enabled one). The Workshop copy is the upstream |

- A locked version produces `mod.pinned` (info, 05 §4.3), not `mod.unloadable`. The list shows "locked": the copy is in effect, the Workshop copy is "upstream".
- The classification is recomputed during analysis, not stored. About-disabled Workshop directories without a copy are "hidden Mods" (09 §5), not locked versions.
- Pack copies also have `origin.json`, but scanning skips pack links, so they are not locked versions (10).

**Why a lock library rather than `Mods/`.** For duplicate packageIds in `Mods/`, the game registers only the first in folder-name order (17 §6). Two locked versions cannot coexist there, and updating would mean moving the old copy or renaming About.xml. In the lock library, only the copy the current plan chooses is linked into `Mods/`: old and new copies coexist and each plan chooses its own, with no About.xml changes or manual work. The cost: if the `Mods/` volume has no junctions (FAT, exFAT), plans using locked versions cannot be written to the game; RimArcho refuses with an explanation under 09 §3.

### 5.2 Creating

`archo pin <instance> [--name <directory name>] [--git] [--dry-run]` (`copy-local` alias).

1. On first use, register the lock-library source (check that it neither contains nor is inside the data directory or a source root, 02 §1). Copy the Instance there, by default as `<original directory>_local`; if that is taken, add a number (`_local2`, `_local3` …); **never overwrite**. `--name` must be a plain directory name (no separators, `.`, `..`, `<>:"|?*`).
2. Copy details:
   - Copy from the source's real path (links resolved first).
   - Copy links inside the Mod by content; skip links pointing outside the Mod and cyclic links, and list them in the output.
   - Leave out the upstream's `.git` and any existing `.rimarcho/`.
   - Keep file modification times.
   - Assemble in `<target>.archo-tmp`, then rename it into place. On failure, delete the temporary directory, leaving nothing behind.
3. Write `.rimarcho/origin.json`:

   ```json
   {"origin_instance": "<ID>", "origin_lineage": "<ID>", "workshop_id": "<ID>", "origin_snapshot": "<hash>", "copied_at": "<ISO time>"}
   ```
   `origin_snapshot` is the source snapshot hash at copy time (hexadecimal); `copied_at` is UTC.
4. With `--git`: run `git init` and `git add --all --force` in the copy (ignored files included, for a complete baseline), and commit with a fixed identity (`RimArcho` / `rimarcho@localhost`), `core.autocrlf=false`, `commit.gpgsign=false`, `--no-verify` and the message `Baseline: copy of <name> (#<instance ID>)`. Without git the copy is still made; a git failure is reported.
5. Rescan, which merges the copy into the original Lineage.
6. Swap the original for the copy **only in the current plan** (`swap`, one reversible edit; the plan does not change if the original is disabled). Other plans keep the Workshop copy. On launch or save, the copy is linked into `Mods/` through `_Archo_`, and the Workshop copy is written as `<packageId>_steam` (§4.1 item 4). If `Mods/` already has another non-Workshop copy with the same package ID, the link is not created (09 §3: the game reports an error for duplicates in the same source). If that copy is an ordinary folder, offer, with one confirmation, to move it into the lock library too (`--move-duplicates`; its plans keep using it through links), then link this copy. If it is a link, or another tool's link points at it, do not move it: refuse with an explanation, suggesting to use that copy or move it by hand. Never rename About.xml.
7. Output the copy's reference, path, file count, bytes and skipped links.

### 5.3 Upstream updates

- **RimCrow style**: the Workshop snapshot differs from the copy's (both snapshot ID and semantic hash), and the ACF `timeupdated` is later than the copy's first appearance.
- **Created by RimArcho**: the upstream's current snapshot hash differs from `origin_snapshot` in `origin.json`.

Then `mod.pinned` becomes a warning: "can it run" shows "<Mod>: the Workshop has a newer version", with "see what changed" and "use the workshop version" (§5.5).

`archo mod <instance> --diff <instance>` lists the semantic files that differ, as `+ only in latter`, `~ changed` and `- only in former`, sorted by path. An about-disabled `About/About.xml.disabled` is compared as `About/About.xml`. It writes nothing.

### 5.4 Updating a locked version

`archo pin --update <instance> [--name <directory name>] [--dry-run]`: reference the copy or the upstream. Show the diff first, then create a new lock-library copy (a new directory) under §5.2 and swap it into the current plan. The old copy is kept (it may contain the player's edits; this is why the `--git` baseline exists).

**Copies in `Mods/`.** Copies with `origin.json` in `Mods/` (made by earlier versions) move into the lock library on the first `pin --update`, or with `archo pin <copy> --move-to-library [--dry-run]`. On the same volume they are renamed; across volumes they are copied (including `.git` and `.rimarcho/`), each file is verified, and the original is recycled. Links pointing outside the copy are refused. This command never moves directories without `origin.json` (RimCrow copies among them); other backup copies in `Mods/` use the Mod library's "move into the lock library" (`archo library move-to-locked`, 16 §2.9). Both are the same move: the Instance is pointed to the new location, then scanned, keeping its ID and every plan reference. The CLI runs it without `--dry-run`; the UI previews, then confirms.

The move's confirmation (and the CLI dry run) also lists every affected game, link path, original target and where each link is detached and rebuilt. All games' own incoming links are detached before the physical move and reconnected after the Instance is updated. Each game keeps one effective entry per Instance, never both an env and a locked entry. Foreign incoming links and running games are refused; failure and recovery contract: 09 §3.

### 5.5 Unpinning

`archo unpin <instance> [--dry-run]`: reference the copy or the upstream.

- **RimCrow style**: rename the Workshop copy's `About/About.xml.disabled` back to `About/About.xml` (refuse, changing nothing, if About.xml exists) and rescan at once. Swap the copy in the current plan for the Workshop copy, if present. The copy stays where it is.
- **Created by RimArcho**: swap the current plan's copy back to the upstream, if present; the copy stays in the lock library.
- Only the current plan changes; other plans stay as they are.
- The CLI runs it without `--dry-run` (like `save`); the UI previews, then confirms.

Recycling Mod folders: 09 §5, 02 §3.

## 6. Player data and RimCrow import

### 6.1 Player data

| Data | Owner | Description | Command |
| --- | --- | --- | --- |
| Alias | Lineage | Display name in all interfaces; `find` and search match it. Log attribution still matches the About.xml name, because the game prints `[Name]`, not the alias | `archo alias <ref> [<name>] [--clear]` |
| Tags | Lineage | Several, case-insensitive; RimCrow groups import as tags | `archo tag add\|remove <ref> <tags…> [--mod <ref>]…`, `archo tag list`; filter `list --filter tag:<tag>` |
| Note | Lineage | Plain text | `archo note <ref> [<text>] [--clear]` |

Storage: `lineage_data` (environment, Lineage, alias, note), `lineage_tag` (environment, Lineage, tag). The data belongs to the Lineage and is shared by original and locked versions and by all plans.

### 6.2 One-time RimCrow import

`archo import rimcrow --db <path> [--rules <path>] [--dry-run] [--include-disabled-rules] [--all]`

1. `--db` is required: RimCrow keeps its database at `<installation directory>/data/mod_manager.db`, which cannot be found through the registry or a configuration file. `--rules` defaults to `data\rules\user_rules.json` beside the database.
2. Copy the database and WAL into a temporary `rimcrow-*` directory under RimArcho's data, open the copy read-only, and delete it afterwards. RimCrow's originals are never opened for writing.
3. RimCrow's player data uses packageIds. First map resource paths (absolute → source + relative path) to Instances, then fall back to the packageId. If a packageId maps to several Lineages, do not import it; report it.
4. Mapping:

   | RimCrow | RimArcho |
   | --- | --- |
   | Alias, note | Alias, note; do not overwrite differing existing values, report conflicts |
   | Groups, tags | Tags |
   | Interlock | `attach` rule |
   | `user_rules.json` loadAfter / loadBefore / incompatibleWith | `after` / `before` / `incompatible` |
   | `user_rules.json` loadTop / loadBottom (`value` true) | `top` / `bottom` (08 §1.5) |
   | sign_color, user_mod_type, ignored_issues | No equivalent; count only |

5. **Disabled** RimCrow user rules are left out unless `--include-disabled-rules` is given; importing them silently would change sorting.
6. Report the mapped counts (by path or packageId), imported, already existing and conflicting; list every unmapped or conflicting item, and the count of resources disabled in RimCrow. Count the directories that have only About.xml.disabled, by source, with what to do (rename them back, or enable them again in RimCrow, then `archo scan`; or unsubscribe or delete them; RimArcho's handling: 09 §5).
7. Running it again gives the same result: identical values are skipped, and rules are deduplicated by kind + Lineage. RimCrow's log tables are not imported.
8. `--dry-run` only reports and writes nothing. By default 6 details per category; `--all` shows all.

Measured on real data: 18 §3.

## 7. List exchange

### 7.1 Import destinations

Every imported list (file, pasted text, share code, Rentry link, Steam collection, a save's Mod list) is previewed, then offers three choices:

- **import as a new list**: a new plan with the imported list, v1 source `import`, with an origin note (file name, `share code`, `collection ws:<ID>`, `rentry <ID>`). The default name comes from the list (share-code name, collection title), otherwise the file name. Command `loadout import --new` (13 §4.2), MCP `manage_plans action=import`.
- **replace the whole list**: replace the whole list as one reversible edit; nothing is written to the game.
- **add the ones you lack** (merge): add the imported Mods the current plan lacks, at the positions the constraints suggest, keeping the existing entries; one reversible edit.

None of them writes ModsConfig.xml.

A Steam collection import adds a "follow this collection from now on" checkbox below the preview, checked by default, which links the collection to the new or current plan afterwards (§7.6).

Command: `archo loadout import <file|share code> [--new [<name>]]`; without `--new` it replaces the current plan.

Preview: format, source, list name, game version (warning "the list is for RimWorld X, this game runs Y" if major.minor differs), total, and installed and not-installed counts. Entries not installed get Workshop links (from the IDs given or a steamDB lookup) and replacement suggestions (08 §2.3). When replacing the current plan, the preview shows the added, removed, moved and swapped counts. RimArcho lists (§7.3) include each Mod's version: installed entries without a matching local version show "another version", the total gets "N in another version", and the installed group is expanded by default.

Resolution: packageIds are lowercased; `_steam` tokens are kept and resolved under §4.2; unresolved entries become missing entries. Entries with only a Workshop ID map to the packageId of an installed Instance (loadable ones first); without a local copy they do not become missing entries (there is no packageId), but the preview and the result show their Workshop links. If a list has no official content (Core/DLC), replacing and importing as a new plan keep the current plan's enabled Core and DLC at the front; if it has none, the installed Core is added. This is because Steam collections, Workshop-link exports (§7.3) and ID-only text cannot contain official content, and handwritten packageId lists often leave it out. Preview totals and the installed / not-installed counts count only the list's entries; the kept Core and DLC count only in the replacement changes. If Core or any DLC is in the list, the list is followed exactly. Names and Workshop IDs serve only to display missing entries and to link them. RimArcho lists also include semantic hashes: if a packageId resolves to different content but another live, loadable Instance has the exact hash, that Instance is used instead (unless it is already in the list).

### 7.2 Import formats

Formats are detected by content, not by extension (except `.rws`, read as a save).

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
- Of duplicate packageIds, the first is kept. UTF-16 and UTF-8 BOMs are detected and the text is normalized to UTF-8; files under 16 MB are read whole.
- If no Mods are found, the error is "no mods found in it: a list from RimSort, RimCrow or RimWorld, a share code, packageIds or workshop links".
- Pasting one line in the UI: an existing file path reads the file; a Rentry link fetches `https://rentry.co/<id>/raw` (at most 4 MB; on failure, suggest opening the page, copying the list and pasting it); a Workshop link or a number of 6 or more digits reads a Steam collection (§7.5). Only the last two use the network.
- Pack (`.archopack`) import: 10 §4. A pack's exported list is a RimArcho list, imported here.

### 7.3 Export

`archo loadout export [--format <format>] [--out <file>]` exports the current plan. Without `--out` it writes to stdout; an existing `--out` file is refused, never overwritten.

| Format | Content |
| --- | --- |
| `archo` | RimArcho list (`.archolist.json`): `pack.json` format (10 §1), `light` true, `name` the plan's name. Entries include `lineage`, `instance`, `workshop_id` and the semantic `hash` (Core/DLC only `official`); missing entries only `package_id`. The importer can see which versions match (§7.1). The UI's first export option, "RimArcho list" |
| `text` (default) | Two `#` header lines (environment, Mod count, time; how to import), then one `<packageId>  # <name>` per line; missing entries are named `missing` |
| `modsconfig` / `rimsort` | ModsConfig XML (`ModsConfigData`: `version`, `activeMods/li`, `knownExpansions/li`); a Workshop copy of a package that also has another copy uses `_steam`. RimSort exports this format; `rimsort` is an alias |
| `rml` | RimWorld `savedModList`: `meta` holds `gameVersion`, `modIds`, `modSteamIds` (`0` if none), `modNames`; `modList` holds `ids`, `names`; loads directly in the game's Mod menu |
| `rimcrow` | Share code, one line (§7.4) |
| `rimcrow-xml` | RimCrow `ModList.xml`: `modIds`, `modNames`, `modSteamWorkshopIds` (`0` if none) |
| `workshop` | Workshop links: one `#` header (list name, Mod count, Workshop count), then one `- [<name>](https://steamcommunity.com/sharedfiles/filedetails/?id=<Workshop ID>)` per line in load order; a final comment lists the entries not on the Workshop (Core, DLC, local Mods, missing entries). Recipients can subscribe from it; it imports as a Workshop ID list (§7.2) |

`archo` also has `versions`: the plan's allowed game versions (§1.6), for example `["1.5","1.6","1.7"]`. Older RimArcho versions ignore it.

In the UI, export is a subpage of the list (16 §2.17): preview each format, then save it to a file or copy it; share codes can only be copied.

Entries are serialized as when writing to the game (§4.1 items 4 and 5); the game version comes from the environment, falling back to ModsConfig.xml.

### 7.4 RimCrow share codes

- Format: `RC-<8 uppercase CRC32 hex digits>-<unpadded URL-safe base64>`. The old `RMM1-` prefix is also accepted, and a `share://RC/` prefix is stripped.
- The base64 decodes to zlib-compressed JSON: `[1, list name, game version, entries]`. Entries: `"pkg"`, `["pkg","WorkshopID"]` or `["pkg","WorkshopID","name"]`.
- The CRC32 covers the compressed bytes; a mismatch is rejected ("share code is damaged or incomplete"), and so is a first element other than 1 (unsupported version).
- Export uses maximum compression: entries with a name as triples, entries with only a Workshop ID as pairs, the rest as strings. The list name is the plan's name.

### 7.5 Steam collections

- API: `POST https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/`, form `collectioncount=1&publishedfileids[0]=<ID>`, no API key. `children[]` is sorted by `sortorder` (gaps allowed); `filetype` 0 is a Mod, 2 a subcollection. An ID that is not a collection, or is private, has `result` other than 1 and no `children`; report "this is not a collection".
- Collection and item titles are filled in through the Workshop details API, 100 per request; failing to get titles does not prevent the comparison.
- Collection URL or ID: take `?id=` or the leading 6 or more digits.
- `archo collection <URL or ID>` reads the collection and compares it locally in three groups: installed (the matching Instance; loadable ones first when a Workshop ID has duplicates), not installed (Workshop links), subcollections (listed, not followed). It goes online only when run.
- `archo collection <URL or ID> --import` imports the installed Mods in collection order to a destination under §7.1 (for now, replacing the current plan), preceded by the current Core and DLC (§7.1). Entries not installed show links, not missing entries.
- Missing Mods in "can it run" offer subscribing (16 §2.4). Subscribing to or unsubscribing from a whole collection in one action is planned for 1.2 (01 §1).

### 7.6 Following Steam collections

A plan may **link** one or more Steam collections. When a collection changes, RimArcho tells the player what changed; syncing is the player's choice and never changes a plan automatically. Collections are checked when all lists opens (when due: 16 §2.16.1 A14); checking on a schedule in background tasks is not implemented.

- **Data**: the `plan_collection` table: plan, Workshop ID, title, sync mode (`add` / `follow`), last check time, and a **snapshot** (the collection's entries and order when last linked or synced).
- **Link**: paste a collection URL, or check "follow this collection from now on" when importing (§7.1). The collection is fetched once and its current content snapshotted, so a newly linked collection has no changes. A collection is linked at most once per plan.
- **Check**: online only when all lists opens (interval: 16 §2.16.1 A14) or with `archo plan sync --check`, using the same API as §7.5. There is no check on launch and no background polling.
- **Changes**: the current collection is compared with the snapshot: added (not in the snapshot), removed (no longer in the collection). A collection Mod the player disabled is not a change, since the comparison is with the snapshot, not the plan.
- **Sync**: one reversible edit `sync collection <title>`, after which the snapshot becomes the current collection.
  - `add` (default, UI "add new only"): enable the installed additions at the positions the constraints suggest, as when adding missing Mods (§7.1). Mods removed from the collection stay in the plan.
  - `follow` (UI "also disable dropped"): also disable Mods removed from the collection that are still in the plan. It only disables; it never unsubscribes or deletes.
  - Additions not installed show Workshop links, not missing entries (§7.5).
- **Not-installed count**: collection entries without any local copy, with "subscribe in the Workshop" links.
- **Unlink**: deletes the link and its snapshot and keeps the plan. Deleting a plan also deletes its links.
- **Commands**: `archo plan follow <name> <collection URL or ID> [--mode add|follow]`, `archo plan unfollow <name> <collection>`, `archo plan sync <name> [<collection>] [--check|--dry-run]`. `--check` only checks and lists changes; `--dry-run` shows the enables and disables it would make. MCP has corresponding tools; `sync` without `--check` or `--dry-run` is marked as changing a plan.

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
| H11 | Workshop update after H10 | `mod.pinned` becomes a warning; `mod --diff` lists the changes; `pin --update` creates and swaps in a new copy, keeps the old one | Pass |
| H12 | `unpin` H10 copy | Current plan returns to Workshop, copy retained | Pass |
| X1 | Import RimSort XML, JSON, clipboard | Original order, missing entries retained | Pass (`TestParseRimSortFormats`) |
| X2 | Export RimSort then reimport | Round trip matches | Pass (`TestExportImportRoundTrip`) |
| X3 | Import `.rml`, ModList.xml, share code; export/reimport rml, rimcrow-xml, code | Round trip matches; reject one-digit CRC error | Pass (`TestRimCrowAndRMLFormats`, `TestExportImportRoundTrip`) |
| X4 | Collection with 1 subcollection, 1 uninstalled entry | Three groups correct; `--import` lists link for uninstalled item, not missing entry; Core stays first | Pass (`TestCollection`) |
| X4a | Plan enables Core/one DLC, another installed but disabled; export/import Workshop links (replace/new), then text containing Core | First two retain Core/enabled DLC/list Mods, exclude disabled DLC; explicit Core follows input | Pass (`TestImportWithoutOfficialKeepsCoreAndDLC`, `TestExportWorkshopLinks`) |
| X5 | Import as new plan | New plan, v1 `import`, current unchanged | Pass |
| X6 | Replace current plan then undo | One undo restores original; no ModsConfig.xml write | Pass (`TestImportApplyIsOneStep`, `TestLoadoutExchange`) |
| X7 | `import rimcrow` twice | Second all "already exists"; disabled rules excluded unless `--include-disabled-rules` | Pass (`TestImportRimCrow`) |
| P1 | supportedVersions intersection 1.5/1.6, created under 1.7 | Default 1.5–1.7; empty intersection only 1.7 | Pass (`TestPlanVersionsDefault`) |
| P2 | Player sets 1.6/1.7 then enables 1.4-only Mod | Allowed versions unchanged; `plan versions <name> default` recalculates | Pass (`TestPlanVersionsChosen`) |
| P3 | Game 1.7, plan allows 1.5–1.6 | "for you to confirm" in "can it run" offers "add 1.7"; launch unblocked | Pass (`TestGameNotAllowedNeed`) |
| P4 | Export `archo`, import new plan | Allowed versions preserved | Pass (`TestPlanVersionsTravelInRimArchoList`) |
| C1 | Linked collection adds 3 (2 installed), removes 1; sync `add` | Enable 2 at suggested positions, link uninstalled 1, retain removed; one undoable edit; updated snapshot, next check unchanged | Pass (`TestFollowCollection`) |
| C2 | C1 with `follow` | Also disable removed 1; no unsubscribe/deletion | Pass (`TestFollowCollection`) |
| C3 | Player disables collection Mod, collection unchanged | No changes | Pass (`TestFollowCollection`) |
| C4 | Unlink | Plan unchanged; association/snapshot deleted | Pass (`TestFollowCollection`) |
