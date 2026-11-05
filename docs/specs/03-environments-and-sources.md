# 03 · Environments and sources

> Status: implemented except where marked "not implemented".
> Basis: 00, 00a §4, 02 §1.

This document defines environments and sources, scanning, Instance reconciliation, Lineage, change events, and their storage. See 00a §4 for design rationale.

## 1. Terms

| Term | Definition | UI name (16 §7.2) |
| --- | --- | --- |
| Environment | One game directory plus its save mode (user data directory). Players may have several, such as a licensed 1.7 and SteamCMD-downloaded 1.5 | game |
| Source | A root directory storing `Mods`. A game's `Data`` and ``Mods` belong to its environment; others are **Mod folders** shared by all environments | mod folders; source in filters |
| Instance | One Mod copy on disk, with a RimArcho-assigned ID | this one |
| Snapshot | An Instance's metadata and content fingerprint from one scan | — |
| Lineage | A family of "the same Mod", such as Workshop original, locked version, before/after rename | copies of the same mod |
| Tombstone | A retained Instance record that has disappeared | missing mod |
| Alias location | Other link locations pointing to the same real directory, without creating another Instance | — |
| Identity question | An uncertain reconciliation match, "is this #xxxx returning?", for the player to answer | for you to confirm |

A plan (04) is a **list** in the UI; see 16 §7.2 for other UI terms.

**IDs**: Instance, Lineage, environment, and source IDs are random 128-bit values, lowercase base32 (alphabet `a–z2–7`, no padding, 26 characters). Display the shortest unique prefix among Instances visible to this environment (shared folders and its own), at least 4 characters; compute Instance and Lineage prefixes separately. Reference syntax (`#a3f9`, `L#7c2e`, `pkg:`, `ws:`, `q:`, `src:`, `scan:`, `env:`) and ambiguity handling are in 13 §2. Store full IDs in databases and event detail; shorten only at output.

## 2. One Environment = One Game Directory (0.9.0)

> Implemented (branch `claude/env-model-b161dc`). The decisions below guided the rework; §1–§4 and 04 §4 have been rewritten accordingly.

The player found it wrong for each environment to attach Workshop, Monitor, SteamCMD, and extra folders: these folders are shared by all games, and storing them per environment only creates multiple records for the same Mod (known limitation in 04 §4). Approved model:

| Ownership | Contents |
| --- | --- |
| Environment (one game directory) | Its own `Data` (official)` and ``Mods` (local) sources, game version, launch method, launch arguments, current plan, launch records, `_Archo_` deployment links, save mode |
| Global (shared by all environments) | **Plans** (since 2026-10-25, 07 §1.7); **Mod folders**: Workshop directory (found through Steam), SteamCMD directories, extra folders, Archo Monitor directory, lock library. Their Instances and Lineages, tags, notes, aliases, copy notes, history, user rules |
| By game version | Analysis slots and fact views; identical fact sets are shared across versions (05 §3) |
| By user data directory | ModsConfig.xml write records (already implemented; "not implemented" in §3.4 is outdated) |

**Decisions**:
1. **Shared sources**: `source.environment_id = ''`. Their Instances also have `environment_id = ''`; environment-owned official / local sources and Instances retain their environment ID. All Lineages are global (`environment_id = ''`), so locked copies in `Mods/` and Workshop originals still share one Lineage.
2. **Labels**: shared source labels are globally unique (deployment link names contain labels, 09 §3); environment sources have only official, local (and retired names), also unique within "shared + this environment".
3. **Workshop directory**: at most one active directory. Adding a new Workshop directory with `folder add` means changing its path through §4.2 rebinding. When adding an environment, if the game is in a Steam library and no Workshop directory exists, add it automatically ("find through Steam").
   **Workshop directories bundled with games are SteamCMD directories** (2026-10-22, branch `claude/env-followups`): SteamCMD-downloaded games place `steamapps\workshop\content\294100` inside the game directory (`force_install_dir`). If present and not the Steam client's Workshop directory, register it as a shared steamcmd source; show it like player-added folders in settings "mod folders", and deploy it through `_Archo_` links when enabled (09 §3). Timing: `env add` (including "download a version"), `env set` changing the game directory, and checks across all environments before every scan (backfills environments added earlier). It is **never** treated as a Workshop directory and never triggers "a second Workshop directory means changing paths"; `--workshop` pointing there also adds it as a SteamCMD directory. Pre-scan checks do not re-add previously registered directories (including retired ones removed by the player): removal stays removed. But **explicitly adding this game** (`env add`, including re-adding a removed game) or pointing `env set` here restores the retired directory (2026-10-25; player reported that after removing/re-adding `1.6`, its SteamCMD directory stayed "removed"). Restore the original source record (same ID, label reassigned from steamcmd), so returning `Mods` retain their original Instances, lists, tags, and notes. `folder add` for a retired directory (same path and kind) likewise restores the record instead of creating one. Test: `TestGameWorkshopIsSteamCMD`.
4. **One game directory belongs to only one environment**: `env add` refuses game directories used by another environment. Two environments that already shared a directory before this rule remain unchanged (Mod inventory "used in" still matches them by real path).
5. **Saves**: `--saves shared` (default, game's default user data directory) or `--saves own` (RimArcho creates `<data-directory>/userdata/<environment-name>`, passes `-savedatafolder` on launch; 09 §2 already supports non-default directories). No separate field; infer from `user_data_root`: equal to game default means "shared default", under `<data-directory>/userdata/` means "own", otherwise "custom" (`--user-data` remains available). `Data` directory checks permit own saves because RimArcho owns that directory.
6. **Global user rules by Lineage**: rules describe relationships between `Mods` (load after, incompatible), independent of which game is played; global rules fit "one record per Mod", so players state them once. Rules apply only when both `Mods` are enabled, preventing effects in another game.
7. **Finding ignores by environment**: a finding judges "this game, this version, these `Mods` together" (missing 1.7 support, conflict with another Mod); accepting it in `1.6` does not imply safety in 1.7. Ignores already expire after Mod changes; sharing them across games would hide real findings. Lineage IDs in ignores are rewritten when Lineages merge (§2.1).
8. **Scan scope**: `archo scan` scans "shared folders + current environment's `Data` / `Mods`", once per shared folder in a scan. `scan --game-only` scans only current environment sources; used for adding environments and guide step 2. Active Instances under unscanned shared sources do not become tombstones but remain R4 "original" candidates (copies in `Mods/` still join the Workshop original's Lineage).
9. **Global lock library**: root is `lock.dir` (default `%LOCALAPPDATA%\RimArcho\locked`), one shared source `src:locked`, no per-environment subdirectories. Pre-0.9.0 per-environment `<lock.dir>/<environment-name>` sources are shared folders `locked-<environment-name>`; their copies still count as locked versions (07 §5.1).
10. **Archo Monitor**: one shared source `src:monitor`.
11. **Mod inventory**: list Instances from "shared folders + current environment sources", plus **other games' own `Mods/`** (decided on 2026-10-25, 07 §1.7: the Mod library covers the whole machine; players switching to 1.7 must still see `Mods` in `1.6` `Mods/`). Each game's `Mods/` has source value `local-<environment-name>` (`local-main`, `local-1.6`; UI says "`local-main`"); `source:local` still matches every game's `Mods/` (`app.LocalValue`, `app.LocalMatches`); "used in" lists all plans (shared by all environments, 07 §1.7; the same Mod now has the same Instance ID, without approximating by real path).
12. **Commands**: `archo env add <name> --game <directory> [--saves shared|own] [--launch …]`; new group `archo folder list|add|remove` (MCP `status show=folders`; environment/settings changes remain player-only). `env add/set` `--workshop`, `--steamcmd`, `--extra` remain compatibility aliases for `folder add`; `--remove-source` on shared sources equals `folder remove`.
13. **Interface**: add "mod folders" settings group; environment page shows only game directory, version, launch method, saves, and arguments; first guide step for adding an environment splits into "a game you have"` and `"download a version" (SteamCMD, 01 §3, 11 §3).
14. **Fill links on environment switch**: `env use` (including GUI/TUI switches) first fills missing `Mods/` links according to target environment ModsConfig.xml and current plan, then scans its `Data/Mods/`, then sets current environment. Shared sources such as Monitor use normal deployment links; previous environment's local `Mods` use `_Archo_env_<Instance ID>` pointing to real directories and remain as local `Mods` of the target environment, not deleted on save/disable. If target `Mods/` already contains the same package ID, retain it without duplicate links; own saves without a list do not inherit one. Switching does not write ModsConfig.xml; report an error if links are needed while target game runs; on creation failure roll back new links and keep original environment. Tests: `TestEnvUse*`.

Link creation/rollback in item 14 writes the persistent ledger first; only healthy links are retained. Maintain inbound links across all games before moving/deleting targets; failure and restart recovery are in §4.3 and 09 §3.

### 2.1 Merged IDs

When two Instances or Lineages become one (a "same" answer, 04 §2.4; repairing local records of shared directory links, `Store.MergeLinkedInstances`; credit remapping, `Store.RemapCreditPlans`), one transaction rewrites every stored reference (`store/idremap.go`: `rewriteInstances`, `rewriteLineages`, `rewriteOrigins`, `mergePlayerData`, `mergeRules`, `mergeCopyNote`): the merged-away record is deleted; `instance_alias`, snapshots, `change_event` (merged-away discovered deleted), `identity_question` (superseded once both sides are the same Instance), source use (05 §5.4) and stored origin markers follow the kept ID; list entries in workspace, undo stack, versions and staging keep only the first duplicate, preserving order; copy notes and `lineage_data` notes are joined with a blank line when they differ (kept alias wins), tags are united, user rules are rewritten and deduplicated, finding ignores are rewritten (still per environment).

On-disk IDs are not changed: `id_alias(old_id, new_id, kind)` records each merged-away ID, and reading `pack.json` and `.rimarcho/origin.json` maps IDs through it first (`Store.IDAliases`).

### 2.2 Interface Directory Roles and Aliases (2026-10-25)

The UI meaning of `source:` is registered directory role: official, workshop, steamcmd, local, locked, other, independent of Git / web download method. Lock library recognition still uses `IsLockSource`. Extra directories have both other and a stable alias: use existing `Source.Label`, prefix reserved-word conflicts with `folder/`, and use `folder/<Source.ID>` if missing; display the final directory name. Rebinding changes only path, not alias. No new table or migration; paths remain in details, CLI, and compatible `location:` queries, but not GUI filter/completion candidates. Abstract copy-row names are separate from filter roles: locked versions include creation time; Git / web subscriptions may retain download-method descriptions.

## 3. Environments

### 3.1 Fields

| Field | Description |
| --- | --- |
| `id` | Random ID |
| `name` | Player-chosen name; nonempty, no spaces, `:`, or `#` |
| `game_root` | Game installation directory. Valid if it contains `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldWin.exe`, `RimWorldLinux`, `RimWorldMac.app`, or `Data/Core`. One game directory belongs to only one environment (`env add` refuses a directory already used by another environment) |
| `user_data_root` | User data directory (containing `Config/ModsConfig.xml`), determined by save mode: **shared default** (`--saves shared`, game's default user data directory, default, may be shared with other environments, §3.4), **own** (`--saves own`, RimArcho creates `<data-directory>/userdata/<environment-name>`, passes `-savedatafolder`, 09 §2), or **custom directory** from `--user-data`. Inferred from path, without a separate field |
| `game_version` | Read first line of `<game_root>/Version.txt` (strip UTF-8 BOM and surrounding whitespace), **preserve the complete version**, such as `1.6.4633 rev1261`, `1.7.4985 rev1269`. Compare only major/minor `1.6` ("Full game version" below). Reread each scan and update if changed |
| `launch` | direct or steam. Without explicit `env add` choice: use steam for games in a Steam library (`…/steamapps/common/RimWorld`), otherwise direct; specifying a Workshop directory does not change this default; players may still explicitly choose launch method |
| Launch arguments | Stored in settings (`env:<id>:launch_args`), changed by `env set --launch-args` or settings page, not in environment table |

**Full game version** (decided on 2026-10-24). Always store the entire `Version.txt` line without truncation; compute "`1.6`" when needed. Code: `env.ParseVersion(s) → env.Version{Full, Major, Minor, Build, Rev}`; `Short()` returns `1.6` (`env.MajorMinor` equals `ParseVersion(s).Short()`). Format: `major.minor.build revrevision`; `major.minor` is also accepted with unknown build.

- **Compare major/minor**: Mod `supportedVersions`, allowed plan versions (04 §`1.6`), analysis cache version key, `--game-version` / MCP `game_version` override (already only accepts `1.6`), whether a pack can launch on this game (10 §3), environment chip ("main  `1.6`"), TUI status line.
- **Compare full version**: only pack "different build" notice (10 §5). Compare only if both have build numbers; if either has only `major.minor`, say "build not recorded", without counting as different.
- **Display full version**: launch page 01 game area (16 §2.3), environment page and settings "this game", `archo env list/show`, pack pages and `pack list`. Narrow spaces (chips, TUI status line) show only `Short()`.
- **Old data**: RimArcho's own database never stored truncated values (`environment.game_version` rereads each scan; plan versions record ModsConfig.xml version, already the whole line). Only outside values may be short: old lists or lists from other tools, save `gameVersion`, handwritten `pack.json`. Preserve them unchanged and treat as "unknown build" per the rules above.

Environment commands (full syntax in 13 §4):
- `archo env list` (environments and shared Mod folders), `env detect` (detection suggestions only), `env add <name> --detected` or `env add <name> --game <directory> [--saves shared|own | --user-data <directory>] [--launch direct|steam]`. When the game is in a Steam library and no Workshop directory exists, `env add` also adds that library's Workshop directory as a Mod folder (§3.2 item 3).
- `archo env set <name> --game/--saves/--user-data/--launch/--launch-args`. Changing `--game` rebinds official` and `local per §4.2; if the new game is in another Steam library, change the Workshop directory too.
- Compatibility syntax: `env add/set` `--workshop`, `--steamcmd`, `--extra` equal `archo folder add workshop|steamcmd|extra <directory>`; `--remove-source src:<label>` equals `archo folder remove` (§4.1).
- `archo env branches`, `archo env download <branch> --to <directory> [--account <account-name-or-anonymous>]`: download a game version with SteamCMD and add an environment (11 §3).
- `archo env use <name>` switches current environment; the first added environment becomes current automatically; global `--env <name>` switches for one command only.
- `archo env remove <name> --yes`: delete this environment's own database data (`Data` / `Mods` Instances and history, launch records, analysis, finding ignores); retain globally shared plans, rewriting entries pointing to its Instances as `packageId` (07 §1.7); retain shared Mod folder Instances and player-authored Mod data; delete Lineages made empty and their player data/rules. Do not touch game or Mod files. Refuse without `--yes` (exit code 2).
- `archo env rebind src:<label> yes|no`, see §4.2.

**`Data` directory check**: `env add`, `env set`, `folder add`,` and `scan check that RimArcho's data directory, game directories, user data directories, and all source roots **do not contain one another**; refuse containment. Reason: RimArcho reads these directories or writes only under 02 §1's list; putting its data directory inside them would break that boundary. The only exception is own saves `<data-directory>/userdata/<environment-name>`, already owned by RimArcho.

### 3.2 Automatic Detection

Try in order, stopping at the first match in each step:
1. **Game directory**: first find Steam roots (Windows: registry, then `C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`; macOS: `~/Library/Application Support/Steam`; Linux: `~/.steam/steam`, `~/.local/share/Steam`, Flatpak `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`). Read each root's `steamapps/libraryfolders.vdf` (or `config/libraryfolders.vdf`; support old formats such as `"1" "D:\\SteamLibrary"`), search each library for `steamapps/common/RimWorld`. Matches use steam launch.
   If not found, try common manual installation locations, using direct launch:
   - Windows: `C:\Games\RimWorld`, `D:\Games\RimWorld`, `C:\GOG Games\RimWorld`, `D:\GOG Games\RimWorld`, `C:\Program Files\RimWorld`, `C:\Program Files (x86)\RimWorld`
   - macOS: `/Applications/RimWorld.app`, `/Applications/RimWorld`
   - Linux: `~/GOG Games/RimWorld`, `~/Games/RimWorld`
2. **User data directory**:
   - Windows: `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`
   - macOS: `~/Library/Application Support/RimWorld`
   - Linux: `~/.config/unity3d/Ludeon Studios/RimWorld by Ludeon Studios`, plus Flatpak `~/.var/app/com.valvesoftware.Steam/.config/unity3d/Ludeon Studios/RimWorld by Ludeon Studios`
3. **Workshop directory**: `steamapps/workshop/content/294100` in the game's Steam library. Infer it from game paths shaped like `…/steamapps/common/RimWorld`. If settings have no Workshop directory yet, search libraries from `libraryfolders.vdf` for `workshop/content/294100`, preferring the game's library, and offer a "use it" button.

Detection results are suggestions only: `env detect` outputs suggestions and attempted locations; only `env add --detected` writes. In GUI first-run guide, players may select multiple detected environments to add together (16 §2.15).

### 3.3 Player.log Location

- RimWorld 1.7 and later: `<user_data_root>/Logs/Player.log`
- Earlier versions: `<user_data_root>/Player.log`
- Unknown version: check both, choose newest mtime. Previous run log is `Player-prev.log` in the same directory.
- Compare numeric version components (`1.10` exceeds `1.9`).

Launch ownership of logs (environment, plan, or pack) comes from launch records, not paths; see 06 §4.

### 3.4 Shared User Data Directories

Multiple environments may use one user data directory, commonly licensed and test games sharing the game default. This is allowed; `env add` / `env set` do not refuse. Preconditions:
- Only one game may run at a time, detected across all environments (09 §4).
- Write ModsConfig.xml only on plan launch or "save" (09 §2, 07 §4.1), with the list for this play session.
- Outside-change detection records last hash and writer (which plan) by **user data directory**, not environment (07 §4.3).
- Multiple environments write the same Player.log in turn; launch records determine ownership (06 §4).

Packs do not share: each uses its own `-savedatafolder` (10 §3).

## 4. Sources

### 4.1 Source Kinds

| `kind` | Root directory | Description |
| --- | --- | --- |
| `kind` | Root directory | Ownership | Description |
| --- | --- | --- | --- |
| `official` | `<game_root>/Data` | Environment | `Core` and DLC. Always first; `Core` cannot be disabled |
| `local` | `<game_root>/Mods` | Environment | Player's local `Mods` |
| `workshop` | `.../workshop/content/294100` | Shared | Subdirectory name is Workshop ID. At most one active Workshop directory |
| `steamcmd` | Player-specified, or `<game_root>/steamapps/workshop/content/294100` bundled with a SteamCMD game (§2 item 3) | Shared | Same structure as workshop; not read directly by the game; deploy through `_Archo_` links when enabled (09 §3) |
| `extra` | Player-specified | Shared | Other directories; enabled like steamcmd. RimArcho's Archo Monitor directory (`src:monitor`, 09 §6) and lock library (`src:locked`, 07 §5.1) are also extra |

- Manage **Mod folders** (shared sources with empty `source.environment_id`) in one place: `archo folder list|add|remove`, settings "mod folders" (16 §2.14); MCP reads with `status show=folders`. `folder add workshop <directory>` with an existing Workshop directory changes its path, rebinding per §4.2; Mod folders must not contain one another or any game's `Data`/`Mods`; Monitor directory and lock library belong to RimArcho and cannot be removed.
- Each source has a **label** for `src:<label>` references. Shared labels are globally unique (included in deployment link names, 09 §3); environment-owned sources have only official, local (and renamed retired labels). Default label is kind; subsequent same-kind sources use `steamcmd2`, `steamcmd3`, etc.
- Source states:

| State | Meaning | During scan |
| --- | --- | --- |
| `ok` | Normal | Scan |
| `missing` | Root directory absent | Do not scan; neither match nor tombstone its Instances. Return to ok when root reappears on a later scan |
| `rebind-pending` | Path changed, insufficient sample match rate (§4.2) | Do not scan; leave Instances unchanged pending player answer |
| `retired` | Removed or replaced by new source | Do not scan; tombstone its Instances on next scan |

- missing means the entire source is temporarily absent (for example an unplugged external drive): avoid tombstoning and "reviving" the whole source, and allow rebinding against live Instances. Tombstones represent individual directory disappearance only.
- Removing a Mod folder (`folder remove`): mark retired, relabel `<original-label>-removed` (number suffix on conflict); next scan emits disappeared and tombstones its Instances.

### 4.2 Source Identity and Rebinding

Every source has its own `source_id`. Instances record only `source_id + relative path`, not absolute paths. Thus changing a game directory only requires changing source roots to preserve all Instance IDs.

**When to evaluate**: `env set --game` changes game directory (both official` and `local; Workshop directory also changes per §3.2 item 3 if the new game is in another Steam library), `folder add workshop` changes Workshop directory, or paths change in environment/settings pages (16 §2.14).

**Evaluation**:
1. List subdirectories with `About/About.xml` under the new root, sort by name, and take up to 20 **evenly spaced** samples (not random; reproducible).
2. Compute each sample's semantic fingerprint (04 §1.4), find an Instance in the old source with the same relative path (case-insensitive); similarity ≥ 0.9 counts as a match.
3. Match rate = matches / samples. At ≥ 80%, treat as the same source and update only root path. Bind directly if old source has no Instances.
4. Below threshold, do not bind automatically. Mark source `rebind-pending`, record new path in `pending_root`, and ask "is this the original source?" with match rate and examples. Player answers with `archo env rebind src:<label> yes|no`:
   - yes: adopt new root, return to ok.
   - no: retire old source (retired), relabel `<label>-old`, tombstone its Instances on next scan; create a new source at new root with original label.

If the player changes game path and deletes `Mods` together: rebind first, then put unmatched items into identity questions (04 §2.4).

### 4.3 Links and Aliases

Link lifecycle is independent of scan aliases: `managed_link` persistently records physical `Mods` path, name/type, Instance or pack identity, original target, new location/target, move from/to, managed directory identity, operation ID, and phase. Inspection prioritizes ledger ownership evidence; normal aliases still create no Instance. All known games share inbound-link enumeration; see 09 §3.

- When listing subdirectories, follow junctions and symbolic links like the game's `DirectoryInfo.GetDirectories`, resolving real paths (Windows `GetFinalPathNameByHandle`). Windows comparisons ignore case and expand 8.3 short paths. Dangling links do not enter normal Mod lists but must be reported by separate read-only link inspection, never silently omitted; non-directory entries create no Instance.
- **Aliases**: multiple locations resolving to one real directory count as one Instance. Choose primary location in this priority order; record the others as alias locations:
  1. An existing Instance's location (keeps R1 matching);
  2. A non-link location;
  3. Source reconciliation order (workshop → steamcmd → official → local → extra);
  4. Source order within environment;
  5. Relative path.
- A link outside all sources has only one location; record its real path (`links-to` in mod output), not an alias. RimCrow's `Mods/_Link_<Workshop-ID>` works this way and is recognized as a normal local Mod; RimArcho does not manage or delete these links.
- **`_Archo_` links**: `Mods/_Archo_<source-label>_<directory-name>` created when deploying `Mods` from steamcmd / extra (09 §3). They resolve to real source directories and are merely aliases of those Instances: no new Instance or event.
- **Pack links**: scan skips links with names starting `!ArchoPack_` (case-insensitive). They exist only while playing packs (10 §3), outside the environment. Pack copies carry `.rimarcho/origin.json` but are not scanned now.
- Do not follow links **inside** Mod directories (02 §1). For `Mods` whose root is a link, fingerprint the target.

## 5. Acceptance

| # | Scenario | Expected | Test | Status |
| --- | --- | --- | --- | --- |
| A4 | Move entire game to another disk, change environment path | Rebind succeeds; all Instance IDs unchanged | `TestA4GameFolderMoved` | Pass |
| A16 | Two environments (e.g. `1.6`/1.7) share default user data | `env add` permits; outside changes judged by user data directory, no false report switching A→B after A writes; attribute to writer plan; logs exclude other game's files per 06 §4 | `TestV8SharedUserData`, `TestForeignLogExcludedBeforeAnalysis` | Pass |
| A24 | Insufficient path-rebind samples; answer no | `rebind-pending`; no retires/relabels old source `<label>-old`, tombstones Instances next scan, new root becomes new source | `TestA4RebindRejected` | Pass |
| A26 | Remove steamcmd source/environment | Refuse removing local; removed steamcmd Instances emit disappeared; `env remove` refuses without `--yes`, otherwise deletes all environment rows | `TestEnvRemoveAndSourceRemove` | Pass |
| A29 | `Data` directory inside game/source root | `env add`, `env set`, scan refuse with explanation | — | Pending validation |
| A30 | Save modes | `--saves own` creates `<data-directory>/userdata/<environment-name>`, launches with `-savedatafolder`; `--saves shared` restores default; refuse `--saves` + `--user-data` | `TestEnvModelSaves` | Pass |
| A31 | Add/remove Mod folders | Adding existing directory unchanged; refuse containment/game `Mods`; remove shared folders only, Instances disappeared next scan; second Workshop directory means rebind with unchanged Instances | `TestFolderCommands` | Pass |
