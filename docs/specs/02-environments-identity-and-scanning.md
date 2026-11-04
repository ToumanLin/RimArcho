# 02 · Environments, Identity and Scanning

> Status: implemented except where marked "not implemented".
> Basis: 00, 00a §4, 01 §4.

This document defines environments and sources, scanning, Instance reconciliation, Lineage, change events, and their storage. See 00a §4 for design rationale.

## 0. One Environment = One Game Directory (0.9.0)

> Implemented (branch `claude/env-model-b161dc`). The decisions below guided the rework; §1–§3 and §7 have been rewritten accordingly.

The player found it wrong for each environment to attach Workshop, Monitor, SteamCMD, and extra folders: these folders are shared by all games, and storing them per environment only creates multiple records for the same Mod (known limitation in §7). Approved model:

| Ownership | Contents |
| --- | --- |
| Environment (one game directory) | Its own `Data` (official)` and ``Mods` (local) sources, game version, launch method, launch arguments, current plan, launch records, `_Archo_` deployment links, save mode |
| Global (shared by all environments) | **Plans** (since 2026-10-25, 04 §1.7); **Mod folders**: Workshop directory (found through Steam), SteamCMD directories, extra folders, Archo Monitor directory, lock library. Their Instances and Lineages, tags, notes, aliases, copy notes, history, user rules |
| By game version | Analysis slots and fact views; identical fact sets are shared across versions (03 §3) |
| By user data directory | ModsConfig.xml write records (already implemented; "not implemented" in §2.4 is outdated) |

**Decisions**:
1. **Shared sources**: `source.environment_id = ''`. Their Instances also have `environment_id = ''`; environment-owned official / local sources and Instances retain their environment ID. All Lineages are global (`environment_id = ''`), so locked copies in `Mods/` and Workshop originals still share one Lineage.
2. **Labels**: shared source labels are globally unique (deployment link names contain labels, 08 §3); environment sources have only official, local (and retired names), also unique within "shared + this environment".
3. **Workshop directory**: at most one active directory. Adding a new Workshop directory with `folder add` means changing its path through §3.2 rebinding. When adding an environment, if the game is in a Steam library and no Workshop directory exists, add it automatically ("find through Steam").
   **Workshop directories bundled with games are SteamCMD directories** (2026-10-22, branch `claude/env-followups`): SteamCMD-downloaded games place `steamapps\workshop\content\294100` inside the game directory (`force_install_dir`). If present and not the Steam client's Workshop directory, register it as a shared steamcmd source; show it like player-added folders in settings "mod folders", and deploy it through `_Archo_` links when enabled (08 §3). Timing: `env add` (including "download a version"), `env set` changing the game directory, and checks across all environments before every scan (backfills environments added earlier). It is **never** treated as a Workshop directory and never triggers "a second Workshop directory means changing paths"; `--workshop` pointing there also adds it as a SteamCMD directory. Pre-scan checks do not re-add previously registered directories (including retired ones removed by the player): removal stays removed. But **explicitly adding this game** (`env add`, including re-adding a removed game) or pointing `env set` here restores the retired directory (2026-10-25; player reported that after removing/re-adding `1.6`, its SteamCMD directory stayed "removed"). Restore the original source record (same ID, label reassigned from steamcmd), so returning `Mods` retain their original Instances, lists, tags, and notes. `folder add` for a retired directory (same path and kind) likewise restores the record instead of creating one. Test: `TestGameWorkshopIsSteamCMD`.
4. **One game directory belongs to only one environment**: `env add` refuses game directories used by another environment. Two environments that already shared a directory before this rule remain unchanged (Mod inventory "used in" still matches them by real path).
5. **Saves**: `--saves shared` (default, game's default user data directory) or `--saves own` (RimArcho creates `<data-directory>/userdata/<environment-name>`, passes `-savedatafolder` on launch; 08 §2 already supports non-default directories). No separate field; infer from `user_data_root`: equal to game default means "shared default", under `<data-directory>/userdata/` means "own", otherwise "custom" (`--user-data` remains available). `Data` directory checks permit own saves because RimArcho owns that directory.
6. **Global user rules by Lineage**: rules describe relationships between `Mods` (load after, incompatible), independent of which game is played; global rules fit "one record per Mod", so players state them once. Rules apply only when both `Mods` are enabled, preventing effects in another game.
7. **Finding ignores by environment**: a finding judges "this game, this version, these `Mods` together" (missing 1.7 support, conflict with another Mod); accepting it in `1.6` does not imply safety in 1.7. Ignores already expire after Mod changes; sharing them across games would hide real findings. Lineage IDs in ignores are rewritten when Lineages merge (§0.1).
8. **Scan scope**: `archo scan` scans "shared folders + current environment's `Data` / `Mods`", once per shared folder in a scan. `scan --game-only` scans only current environment sources; used for adding environments and guide step 2. Active Instances under unscanned shared sources do not become tombstones but remain R4 "original" candidates (copies in `Mods/` still join the Workshop original's Lineage).
9. **Global lock library**: root is `lock.dir` (default `%LOCALAPPDATA%\RimArcho\locked`), one shared source `src:locked`, no per-environment subdirectories. Pre-0.9.0 per-environment `<lock.dir>/<environment-name>` sources are shared folders `locked-<environment-name>`; their copies still count as locked versions (04 §7.1).
10. **Archo Monitor**: one shared source `src:monitor`.
11. **Mod inventory**: list Instances from "shared folders + current environment sources", plus **other games' own `Mods/`** (decided on 2026-10-25, 04 §1.7: the Mod library covers the whole machine; players switching to 1.7 must still see `Mods` in `1.6` `Mods/`). Each game's `Mods/` has source value `local-<environment-name>` (`local-main`, `local-1.6`; UI says "`local-main`"); `source:local` still matches every game's `Mods/` (`app.LocalValue`, `app.LocalMatches`); "used in" lists all plans (shared by all environments, 04 §1.7; the same Mod now has the same Instance ID, without approximating by real path).
12. **Commands**: `archo env add <name> --game <directory> [--saves shared|own] [--launch …]`; new group `archo folder list|add|remove` (MCP `status show=folders`; environment/settings changes remain player-only). `env add/set` `--workshop`, `--steamcmd`, `--extra` remain compatibility aliases for `folder add`; `--remove-source` on shared sources equals `folder remove`.
13. **Interface**: add "mod folders" settings group; environment page shows only game directory, version, launch method, saves, and arguments; first guide step for adding an environment splits into "a game you have"` and `"download a version" (SteamCMD, 01 §3, 08 §11).
14. **Fill links on environment switch**: `env use` (including GUI/TUI switches) first fills missing `Mods/` links according to target environment ModsConfig.xml and current plan, then scans its `Data/Mods/`, then sets current environment. Shared sources such as Monitor use normal deployment links; previous environment's local `Mods` use `_Archo_env_<Instance ID>` pointing to real directories and remain as local `Mods` of the target environment, not deleted on save/disable. If target `Mods/` already contains the same package ID, retain it without duplicate links; own saves without a list do not inherit one. Switching does not write ModsConfig.xml; report an error if links are needed while target game runs; on creation failure roll back new links and keep original environment. Tests: `TestEnvUse*`.

Link creation/rollback in item 14 writes the persistent ledger first; only healthy links are retained. Maintain inbound links across all games before moving/deleting targets; failure and restart recovery are in §3.3 and 08 §3.

### 0.1 Merged IDs

When two Instances or Lineages become one (a "same" answer, §5.4; repairing local records of shared directory links, `Store.MergeLinkedInstances`; credit remapping, `Store.RemapCreditPlans`), one transaction rewrites every stored reference (`store/idremap.go`: `rewriteInstances`, `rewriteLineages`, `rewriteOrigins`, `mergePlayerData`, `mergeRules`, `mergeCopyNote`): the merged-away record is deleted; `instance_alias`, snapshots, `change_event` (merged-away discovered deleted), `identity_question` (superseded once both sides are the same Instance), source use (03 §5.4) and stored origin markers follow the kept ID; list entries in workspace, undo stack, versions and staging keep only the first duplicate, preserving order; copy notes and `lineage_data` notes are joined with a blank line when they differ (kept alias wins), tags are united, user rules are rewritten and deduplicated, finding ignores are rewritten (still per environment).

On-disk IDs are not changed: `id_alias(old_id, new_id, kind)` records each merged-away ID, and reading `pack.json` and `.rimarcho/origin.json` maps IDs through it first (`Store.IDAliases`).

### 0.2 Interface Directory Roles and Aliases (2026-10-25)

The UI meaning of `source:` is registered directory role: official, workshop, steamcmd, local, locked, other, independent of Git / web download method. Lock library recognition still uses `IsLockSource`. Extra directories have both other and a stable alias: use existing `Source.Label`, prefix reserved-word conflicts with `folder/`, and use `folder/<Source.ID>` if missing; display the final directory name. Rebinding changes only path, not alias. No new table or migration; paths remain in details, CLI, and compatible `location:` queries, but not GUI filter/completion candidates. Abstract copy-row names are separate from filter roles: locked versions include creation time; Git / web subscriptions may retain download-method descriptions.


## 1. Terms

| Term | Definition | UI name (06 §7.2) |
| --- | --- | --- |
| Environment | One game directory plus its save mode (user data directory). Players may have several, such as a licensed 1.7 and SteamCMD-downloaded 1.5 | game |
| Source | A root directory storing `Mods`. A game's `Data`` and ``Mods` belong to its environment; others are **Mod folders** shared by all environments | mod folders; source in filters |
| Instance | One Mod copy on disk, with a RimArcho-assigned ID | this one |
| Snapshot | An Instance's metadata and content fingerprint from one scan | — |
| Lineage | A family of "the same Mod", such as Workshop original, locked version, before/after rename | copies of the same mod |
| Tombstone | A retained Instance record that has disappeared | missing mod |
| Alias location | Other link locations pointing to the same real directory, without creating another Instance | — |
| Identity question | An uncertain reconciliation match, "is this #xxxx returning?", for the player to answer | for you to confirm |

A plan (04) is a **list** in the UI; see 06 §7.2 for other UI terms.

**IDs**: Instance, Lineage, environment, and source IDs are random 128-bit values, lowercase base32 (alphabet `a–z2–7`, no padding, 26 characters). Display the shortest unique prefix among Instances visible to this environment (shared folders and its own), at least 4 characters; compute Instance and Lineage prefixes separately. Reference syntax (`#a3f9`, `L#7c2e`, `pkg:`, `ws:`, `q:`, `src:`, `scan:`, `env:`) and ambiguity handling are in 05 §2. Store full IDs in databases and event detail; shorten only at output.

## 2. Environments

### 2.1 Fields

| Field | Description |
| --- | --- |
| `id` | Random ID |
| `name` | Player-chosen name; nonempty, no spaces, `:`, or `#` |
| `game_root` | Game installation directory. Valid if it contains `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldWin.exe`, `RimWorldLinux`, `RimWorldMac.app`, or `Data/Core`. One game directory belongs to only one environment (`env add` refuses a directory already used by another environment) |
| `user_data_root` | User data directory (containing `Config/ModsConfig.xml`), determined by save mode: **shared default** (`--saves shared`, game's default user data directory, default, may be shared with other environments, §2.4), **own** (`--saves own`, RimArcho creates `<data-directory>/userdata/<environment-name>`, passes `-savedatafolder`, 08 §2), or **custom directory** from `--user-data`. Inferred from path, without a separate field |
| `game_version` | Read first line of `<game_root>/Version.txt` (strip UTF-8 BOM and surrounding whitespace), **preserve the complete version**, such as `1.6.4633 rev1261`, `1.7.4985 rev1269`. Compare only major/minor `1.6` ("Full game version" below). Reread each scan and update if changed |
| `launch` | direct or steam. Without explicit `env add` choice: use steam for games in a Steam library (`…/steamapps/common/RimWorld`), otherwise direct; specifying a Workshop directory does not change this default; players may still explicitly choose launch method |
| Launch arguments | Stored in settings (`env:<id>:launch_args`), changed by `env set --launch-args` or settings page, not in environment table |

**Full game version** (decided on 2026-10-24). Always store the entire `Version.txt` line without truncation; compute "`1.6`" when needed. Code: `env.ParseVersion(s) → env.Version{Full, Major, Minor, Build, Rev}`; `Short()` returns `1.6` (`env.MajorMinor` equals `ParseVersion(s).Short()`). Format: `major.minor.build revrevision`; `major.minor` is also accepted with unknown build.

- **Compare major/minor**: Mod `supportedVersions`, allowed plan versions (04 §`1.6`), analysis cache version key, `--game-version` / MCP `game_version` override (already only accepts `1.6`), whether a pack can launch on this game (08 §5.3), environment chip ("main  `1.6`"), TUI status line.
- **Compare full version**: only pack "different build" notice (08 §5.5). Compare only if both have build numbers; if either has only `major.minor`, say "build not recorded", without counting as different.
- **Display full version**: launch page 01 game area (06 §2.3), environment page and settings "this game", `archo env list/show`, pack pages and `pack list`. Narrow spaces (chips, TUI status line) show only `Short()`.
- **Old data**: RimArcho's own database never stored truncated values (`environment.game_version` rereads each scan; plan versions record ModsConfig.xml version, already the whole line). Only outside values may be short: old lists or lists from other tools, save `gameVersion`, handwritten `pack.json`. Preserve them unchanged and treat as "unknown build" per the rules above.

Environment commands (full syntax in 05 §4):
- `archo env list` (environments and shared Mod folders), `env detect` (detection suggestions only), `env add <name> --detected` or `env add <name> --game <directory> [--saves shared|own | --user-data <directory>] [--launch direct|steam]`. When the game is in a Steam library and no Workshop directory exists, `env add` also adds that library's Workshop directory as a Mod folder (§2.2 item 3).
- `archo env set <name> --game/--saves/--user-data/--launch/--launch-args`. Changing `--game` rebinds official` and `local per §3.2; if the new game is in another Steam library, change the Workshop directory too.
- Compatibility syntax: `env add/set` `--workshop`, `--steamcmd`, `--extra` equal `archo folder add workshop|steamcmd|extra <directory>`; `--remove-source src:<label>` equals `archo folder remove` (§3.1).
- `archo env branches`, `archo env download <branch> --to <directory> [--account <account-name-or-anonymous>]`: download a game version with SteamCMD and add an environment (08 §11).
- `archo env use <name>` switches current environment; the first added environment becomes current automatically; global `--env <name>` switches for one command only.
- `archo env remove <name> --yes`: delete this environment's own database data (`Data` / `Mods` Instances and history, launch records, analysis, finding ignores); retain globally shared plans, rewriting entries pointing to its Instances as `packageId` (04 §1.7); retain shared Mod folder Instances and player-authored Mod data; delete Lineages made empty and their player data/rules. Do not touch game or Mod files. Refuse without `--yes` (exit code 2).
- `archo env rebind src:<label> yes|no`, see §3.2.

**`Data` directory check**: `env add`, `env set`, `folder add`,` and `scan check that RimArcho's data directory, game directories, user data directories, and all source roots **do not contain one another**; refuse containment. Reason: RimArcho reads these directories or writes only under 01 §4's list; putting its data directory inside them would break that boundary. The only exception is own saves `<data-directory>/userdata/<environment-name>`, already owned by RimArcho.

### 2.2 Automatic Detection

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

Detection results are suggestions only: `env detect` outputs suggestions and attempted locations; only `env add --detected` writes. In GUI first-run guide, players may select multiple detected environments to add together (06 §2.15).

### 2.3 Player.log Location

- RimWorld 1.7 and later: `<user_data_root>/Logs/Player.log`
- Earlier versions: `<user_data_root>/Player.log`
- Unknown version: check both, choose newest mtime. Previous run log is `Player-prev.log` in the same directory.
- Compare numeric version components (`1.10` exceeds `1.9`).

Launch ownership of logs (environment, plan, or pack) comes from launch records, not paths; see 03 §6.4.

### 2.4 Shared User Data Directories

Multiple environments may use one user data directory, commonly licensed and test games sharing the game default. This is allowed; `env add` / `env set` do not refuse. Preconditions:
- Only one game may run at a time, detected across all environments (08 §4).
- Write ModsConfig.xml only on plan launch or "save" (08 §2, 04 §4.1), with the list for this play session.
- Outside-change detection records last hash and writer (which plan) by **user data directory**, not environment (04 §4.3).
- Multiple environments write the same Player.log in turn; launch records determine ownership (03 §6.4).

Packs do not share: each uses its own `-savedatafolder` (08 §5.3).

## 3. Sources

### 3.1 Source Kinds

| `kind` | Root directory | Description |
| --- | --- | --- |
| `kind` | Root directory | Ownership | Description |
| --- | --- | --- | --- |
| `official` | `<game_root>/Data` | Environment | `Core` and DLC. Always first; `Core` cannot be disabled |
| `local` | `<game_root>/Mods` | Environment | Player's local `Mods` |
| `workshop` | `.../workshop/content/294100` | Shared | Subdirectory name is Workshop ID. At most one active Workshop directory |
| `steamcmd` | Player-specified, or `<game_root>/steamapps/workshop/content/294100` bundled with a SteamCMD game (§0 item 3) | Shared | Same structure as workshop; not read directly by the game; deploy through `_Archo_` links when enabled (08 §3) |
| `extra` | Player-specified | Shared | Other directories; enabled like steamcmd. RimArcho's Archo Monitor directory (`src:monitor`, 08 §9) and lock library (`src:locked`, 04 §7.1) are also extra |

- Manage **Mod folders** (shared sources with empty `source.environment_id`) in one place: `archo folder list|add|remove`, settings "mod folders" (06 §2.14); MCP reads with `status show=folders`. `folder add workshop <directory>` with an existing Workshop directory changes its path, rebinding per §3.2; Mod folders must not contain one another or any game's `Data`/`Mods`; Monitor directory and lock library belong to RimArcho and cannot be removed.
- Each source has a **label** for `src:<label>` references. Shared labels are globally unique (included in deployment link names, 08 §3); environment-owned sources have only official, local (and renamed retired labels). Default label is kind; subsequent same-kind sources use `steamcmd2`, `steamcmd3`, etc.
- Source states:

| State | Meaning | During scan |
| --- | --- | --- |
| `ok` | Normal | Scan |
| `missing` | Root directory absent | Do not scan; neither match nor tombstone its Instances. Return to ok when root reappears on a later scan |
| `rebind-pending` | Path changed, insufficient sample match rate (§3.2) | Do not scan; leave Instances unchanged pending player answer |
| `retired` | Removed or replaced by new source | Do not scan; tombstone its Instances on next scan |

- missing means the entire source is temporarily absent (for example an unplugged external drive): avoid tombstoning and "reviving" the whole source, and allow rebinding against live Instances. Tombstones represent individual directory disappearance only.
- Removing a Mod folder (`folder remove`): mark retired, relabel `<original-label>-removed` (number suffix on conflict); next scan emits disappeared and tombstones its Instances.

### 3.2 Source Identity and Rebinding

Every source has its own `source_id`. Instances record only `source_id + relative path`, not absolute paths. Thus changing a game directory only requires changing source roots to preserve all Instance IDs.

**When to evaluate**: `env set --game` changes game directory (both official` and `local; Workshop directory also changes per §2.2 item 3 if the new game is in another Steam library), `folder add workshop` changes Workshop directory, or paths change in environment/settings pages (06 §2.14).

**Evaluation**:
1. List subdirectories with `About/About.xml` under the new root, sort by name, and take up to 20 **evenly spaced** samples (not random; reproducible).
2. Compute each sample's semantic fingerprint (§4.4), find an Instance in the old source with the same relative path (case-insensitive); similarity ≥ 0.9 counts as a match.
3. Match rate = matches / samples. At ≥ 80%, treat as the same source and update only root path. Bind directly if old source has no Instances.
4. Below threshold, do not bind automatically. Mark source `rebind-pending`, record new path in `pending_root`, and ask "is this the original source?" with match rate and examples. Player answers with `archo env rebind src:<label> yes|no`:
   - yes: adopt new root, return to ok.
   - no: retire old source (retired), relabel `<label>-old`, tombstone its Instances on next scan; create a new source at new root with original label.

If the player changes game path and deletes `Mods` together: rebind first, then put unmatched items into identity questions (§5.4).

### 3.3 Links and Aliases

Link lifecycle is independent of scan aliases: `managed_link` persistently records physical `Mods` path, name/type, Instance or pack identity, original target, new location/target, move from/to, managed directory identity, operation ID, and phase. Inspection prioritizes ledger ownership evidence; normal aliases still create no Instance. All known games share inbound-link enumeration; see 08 §3.

- When listing subdirectories, follow junctions and symbolic links like the game's `DirectoryInfo.GetDirectories`, resolving real paths (Windows `GetFinalPathNameByHandle`). Windows comparisons ignore case and expand 8.3 short paths. Dangling links do not enter normal Mod lists but must be reported by separate read-only link inspection, never silently omitted; non-directory entries create no Instance.
- **Aliases**: multiple locations resolving to one real directory count as one Instance. Choose primary location in this priority order; record the others as alias locations:
  1. An existing Instance's location (keeps R1 matching);
  2. A non-link location;
  3. Source reconciliation order (workshop → steamcmd → official → local → extra);
  4. Source order within environment;
  5. Relative path.
- A link outside all sources has only one location; record its real path (`links-to` in mod output), not an alias. RimCrow's `Mods/_Link_<Workshop-ID>` works this way and is recognized as a normal local Mod; RimArcho does not manage or delete these links.
- **`_Archo_` links**: `Mods/_Archo_<source-label>_<directory-name>` created when deploying `Mods` from steamcmd / extra (08 §3). They resolve to real source directories and are merely aliases of those Instances: no new Instance or event.
- **Pack links**: scan skips links with names starting `!ArchoPack_` (case-insensitive). They exist only while playing packs (08 §5.3), outside the environment. Pack copies carry `.rimarcho/origin.json` but are not scanned now.
- Do not follow links **inside** Mod directories (01 §4). For `Mods` whose root is a link, fingerprint the target.

### 3.4 Subscriptions: Git, Download Pages, SteamCMD (2026-10-24)

"Subscribe" = install a Mod from an origin, then check/pull updates and remove it later. Steam client still handles Workshop subscriptions (08 §10); add three origins here:

| Origin | Install location | Tracks | Update check | Update |
| --- | --- | --- | --- | --- |
| git: Git repository URL (https, `file://`, or local path) | Subscription directory `<subscribed.dir>/<repository-name>`, entire working tree including `.git` | One of branch (remote default by default), tag, or commit | Read remote refs (ls-remote); branch/tag commit differs from installed = update; tracked commits never have updates | Fetch and check out new commit; refuse dirty working tree (preserve player edits) |
| page: public Mod download page (LoversLab `https://www.loverslab.com/files/file/<id>-<name>/`), login required for download | Subscription directory `<subscribed.dir>/<page-name>` | Page | Read public page version/update time (JSON-LD), changed = update | Player downloads in browser, then updates from zip/extracted folder; RimArcho does not download |
| steamcmd: Workshop ID, explicitly chosen or Steam client unavailable | SteamCMD download directory `<steamcmd-mods.dir>/steamapps/workshop/content/294100/<Workshop-ID>`, registered as shared steamcmd source | Workshop item | Cannot determine offline; record last download time only | Rerun `workshop_download_item` (SteamCMD handles incremental download) |

**Subscribe all choice and later credit** (GUI backend contract: [Steam credit API](../api/steam-credit.md)):

- Before subscribe all on import, following collections, or batch subscribe on needs attention, players choose "subscribe through Steam (slow, but credits authors through subscriptions, managed by Steam)" or "download through SteamCMD (fast, but gives authors no credit, managed by RimArcho)". Client path creates no credit batch. Old calls without `batch:true` keep their semantics.
- Record a batch only for actual execution with explicit `kind=workshop, steamcmd=true, batch=true`; include only items confirmed successful this time, with `About.xml` present and scanned as SteamCMD Instances. Partial failure retains successes; total failure creates no batch. This batch scopes later prompts and differs from SteamCMD runner download batches. Downloads do not create Steam account subscriptions.
- After a normal plan successfully launches and records a Launch, match batches against actually enabled Instances still in live SteamCMD sources; count each Launch once per batch, not games that do not use those `Mods`. Pack runs use fixed copies and do not count as SteamCMD Instance use. After three sessions, change counting to prompt.
- Return `CreditPrompt` only when no registered game runs and the Steam worker is available; merge all due batches, deduplicate count by Workshop ID, and give up to three names. Queries do not consume prompts; frontend must coalesce concurrent queries and open only one dialog at a time; only a persisted answer consumes a prompt. Defer if Steam is unavailable or a game runs. "dismiss" permanently enters dismissed; never ask again for that batch.
- After replacement confirmation, persist replacing before task `steam-credit-replace`. Perform real SubscribeItem calls per recorded Instance; overlapping batches reuse persistently confirmed subscriptions, never unsubscribe/resubscribe to retry; if an earlier batch already replaced the original, later batch records done (already replaced). Persist transient failure counts/backoff; wait for Steam recovery; permanent errors such as explicit invisibility, no access, or removal mark item failed, retain CMD content, original result code and Workshop page, without automatic retry.
- Handover requires client state Subscribed and Installed, without NeedsUpdate, Downloading, or DownloadPending, readable `About.xml`, and scanned workshop Instance with the same Workshop ID. Pause all handover while a game runs. Lists are shared across games: check every environment registry; wherever the old SteamCMD Instance is loadable (including after deployment), the new workshop Instance must also be loadable; otherwise mark whole item retained, change no list/deployment, and name the environment; do not use the list's creation EnvID. Corrupt staging JSON also marks the entire item retained, names the list, and ends without infinite backoff; update existing deployed game projections through registry, game writes, and link lifecycle, changing only this item while preserving other unsaved edits and current plan selection. Persistent deployment obligations ensure completion after interruption.
- After deployed projections hand over, remap current contents, historical versions, undo/redo, staging entries and anchors of all lists to the workshop Instance in one transaction; merge both `instance_note` values (deduplicate text, prefer existing origin); increment `meta.credit.remap` in that transaction to invalidate long-open edit batches. DDS records bind physical directories, file state, and content snapshots; do not migrate texture_mod/texture_file directly or copy DDS; if records exist, prompt to optimize the workshop copy again. The same Mod in one Lineage counts as one item, without an undo step restoring old copy. Do not rewrite fixed packs; beyond cross-environment loadability or corrupt staging above, mark retained with reason if a fixed pack, unfinished other-game deployment, or known inbound link (including actual links in `managed_link` ledger directories) still uses original CMD copy. Only unreferenced copies without inbound links pass allowlist and link lifecycle into recycle bin, never hard deletion. Once all items are done / retained / failed, batch becomes done; per-item reasons remain in task results. done means processing ended, not that all subscriptions succeeded or all copies were recycled.
- Waiting for scans, file tasks, or client scan results uses fixed short waits without increasing failure backoff. RimArcho list read synchronization, save, undo/redo, staging, normal/pack launch, and handover share a cross-process lock; list edits lock before reading, stale snapshot saves reject conflicts, and long-lived TUI edit batches must reload after remapping. Scans run outside the lock; lock only handover, deployment, and recycling; launch wait limit is 06 §2.16.1 W02, with Go supplying "waiting for background replacement" while waiting, error only on timeout. Recheck game processes before writes/recycling; Steam and other external launches ignore the lock, leaving a process-check race window. Session-count failure after successful launch logs only; do not report a launched game as a failed launch. Stopping a task cancels current Steam worker and stops at a handover item boundary; window-close runner wait limit is 06 §2.16.1 W04, retaining completed subscriptions and unfinished obligations. App exit leaves no resident process; next GUI startup resumes from SQLite, or resume on background tasks page. Task history cleanup does not delete obligations. Prompt/replacement authorization covers only recorded batch Instances, without automatic unsubscribe, copying into client directories, or changing client ACF.

**Rules**:
1. **Prefer Steam client for Workshop items**: `subscribe workshop <Workshop-ID>` subscribes through Steam client first (same worker as 08 §10); fall back to SteamCMD only on "Steam unavailable" (`ErrSteamUnavailable`: Steam not running, no `steam_api64.dll` in game directory, API startup fails). Players may explicitly request SteamCMD with `--steamcmd`. If neither path works, report error and Workshop page.

   Before actual subscription, both paths preflight using keyless public `ISteamRemoteStorage/GetPublishedFileDetails` POST, up to 100 IDs/request and three seconds/request; HTTP, decoding, or connection failure stops later preflight requests, leaving unknown items to normal download paths. Explicit `result != 1` or `consumer_app_id != 294100` marks "removed from the Workshop (the author deleted it or made it private)", without Steam / SteamCMD calls, retries, or credit inclusion; missing items remain unknown. Use title as task name and `file_size` as progress weight if present. Previews do not preflight over network.
2. **Settings dependency management locates/installs SteamCMD**; subscriptions request its path through one lookup (`App.FindTool("steamcmd") (path, ok)`, default `SteamCMDPath()`); if absent, say "install SteamCMD in settings"; subscriptions do not download it. Accounts follow 08 §11 (`SteamCMDAccount`): anonymous runs hidden; named accounts log in through SteamCMD's own window; RimArcho never touches passwords.
3. **Subscription directory** (`subscribed.dir`, default `%LOCALAPPDATA%\RimArcho\subscribed`) belongs to RimArcho, registered as shared extra source `src:subscribed`, like Monitor (08 §9): deploy via `_Archo_` links (08 §3), include in managed directories (move, relocate), prohibit `folder remove`. Register SteamCMD download directory (`steamcmd-mods.dir`, default `%LOCALAPPDATA%\RimArcho\steamcmd-mods`) as steamcmd source on first download. Neither may contain/be contained by data directory or source roots.
4. **Origin lives in the Mod folder**: `.rimarcho/source.json` (fingerprints already exclude `.rimarcho`, `.git`, §4.4). No separate table for subscription origin (credit batches persist separately): inventory/details read this file in subscription source folders (mtime-cached), preserving origin after database rebuild or folder moves. Fields:

   ```json
   {"kind": "git|page|steamcmd", "url": "repository URL or download page", "repo": "Git repository linked by download page",
    "workshop_id": "steamcmd Workshop ID", "ref_type": "branch|tag|commit", "ref": "main",
    "installed": "installed commit or page update time at installation", "installed_at": "RFC3339",
    "latest": "commit / page update time from last check", "`checked_at`": "RFC3339", "`check_error`": "last check failure reason"}
   ```
   SteamCMD items without this file (previous player downloads) use `kind=steamcmd` and directory name as Workshop ID.
5. **Install**: install into same-volume temporary `<subscribed.dir>/.archo-tmp-*` first (scan skips `.archo-` directories), require root `About/About.xml`, write `source.json`, then rename to final name; suffix occupied names with `-2`, `-3`, etc.; never overwrite. If repository root is not a Mod (Mod in subdirectory), refuse and list subdirectories with `About/About.xml`. Refuse duplicate URL + tracking target. Scan immediately after install; reconcile new Instance normally (§5), joining same Workshop Mod's Lineage through R4.
6. **Check updates** connects only on player request (`subscribe check`, inventory "check updates"); write latest, `checked_at`, `check_error`; "update available" means nonempty latest differs from installed. Offline inventory still shows last result/time.
7. **Update**: `--dry-run` first explains old/new commits; refuse while a game runs (loaded files would change). Git updates in place (same Instance, updated event); download page updates in rule 11.

   - SteamCMD additions and updates share batched downloads: group consecutive updates by real source root and effective account, deduplicate Workshop IDs, limit items per batch with `workshopBatchSize` in `internal/app/subscribeworkshoprun.go`; start one process per batch, create unique temporary script beside SteamCMD exe, pass only ASCII filename to `+runscript`, clean up after process exit. Instances remain mutually exclusive; nonanonymous accounts keep native console.
   - Each item requires success confirmation and `About/About.xml`; exit code 0 or existing directory is insufficient. Report invalid IDs individually before batching, without blocking others; local preparation failures such as script creation retain path/system error. Individual failures retain successes; later batches/source groups continue per next rule; after each group download, verify, publish, write origin; scan/index once at end of subscription update. Update existing sources in their original library; arbitrary layouts still replace via staging, old-directory recycling, and inbound-link lifecycle, preserving identity.
   - After all initial downloads finish, requeue IDs with timeouts, connection failures, abnormal SteamCMD exit, or unconfirmed completion (including bare `Failure` without permanent reason); retry only remaining retryable items, at most two extra rounds, with backoff. Do not automatically retry permissions, invisibility, ownership, authentication, disk-full, or explicit unknown errors. `File Not Found` / `FileNotFound` means "removed from the Workshop", without account advice; suggest checking account only for login, ownership, or access-denied issues. Nonexistent, private, unauthorized, unowned, or disk-write-failed items carry nonretryable markers. If no item in a batch is confirmed and password, Guard, login, or anonymous login fails, stop that source group and mark unrun items with the same failure. Report each failure reason and Workshop page without raw authentication logs.
   - One subscribe-all operation is one task; internal batches/retry rounds create no task rows or steps. Within the existing account lock, SteamCMD records starting `console_log.txt` offset and reads this run's output incrementally; each Success / ERROR updates task, but final per-item parsing and `About.xml` validation remain authoritative. Progress/cancellation UI contract: 06 §2.16.
   - Cancellation terminates only this task's SteamCMD process tree (Windows Job Object, assigned before process resumes), removes temporary script, releases account lock; confirmed successes still validate, register source, scan into database, and enter credit batch; unfinished items become canceled. Resume keeps original task/progress, submitting only remaining items. Steam client path stops later calls after current call returns, retaining subscriptions already made. Closing window also cancels current task runner.
   - Only Git items split SteamCMD groups; skipped download-page items during update-all do not split groups, and returned items retain selection order. Git failure still stops later operations, scanning earlier successes. SteamCMD failures aggregate at operation end, continuing later Git updates and preserving their results.

8. **Remove** (`subscribe remove`) = recycle folder (08 §7 allowlist includes subscription/download directory folders with `source.json` or in SteamCMD sources), never permanently delete; warn only if plans use it (same as 06 §2.9 deletion). SteamCMD items have no account subscription to cancel; deletion is removal.
9. **Commands**: `archo subscribe git <URL> [--branch <name> | --tag <name> | --commit <sha>] [--dry-run]`, `archo subscribe page <download-page> [--git | --from <zip-or-folder>] [--dry-run]`, `archo subscribe workshop <Workshop-ID…> [--steamcmd] [--dry-run]`, `archo subscribe list`, `archo subscribe check [<reference>…]`, `archo subscribe update <references…> | --all [--dry-run]`, `archo subscribe remove <references…> [--dry-run]`. check without references checks all subscriptions; update requires references or `--all`, refusing neither (05 §3.3).
10. **Pure-Go Git** (go-git, 07 §1), no installed Git or system git calls; only repositories requiring no login (public https, local paths); no submodules or LFS (pointer files are downloaded, warn after install). Shallow clone branches/tags (depth 1); full fetch then checkout for tracked commits.
11. **Download pages and LoversLab**: `subscribe.ReadPage` fetches public metadata with one HTTPS GET, no cookies, redirects, or download-button reading. Public pages may also challenge; 403 does not mean not logged in. Authenticated LoversLab download is a separate player entry point:
   - Read JSON-LD (schema.org `SoftwareApplication`: name, `softwareVersion`, `dateModified`; fall back to `og:title`, `og:updated_time`), and body links to Git repositories (gitgud.io, GitHub, GitLab root URLs, stripping `/-/tree/…`, `/-/blob/…`, etc.).
   - If page links one repository (for example RimJobWorld to gitgud.io), `subscribe page <page>` suggests a Git subscription (with several repositories, choose the one most linked into files via `/-/tree/…`, `/-/blob/…`, `/tree/…`, `/blob/…`; homepage/release-only links are usually dependencies, such as Harmony on RimJobWorld's page; if ambiguous, ask player to choose using `subscribe git`), recording page in `source.json` url (kind remains git, repo is repository); `--git` does this directly. gitgud.io is GitLab, anonymous https clone with or without `.git`.
   - Without a repository, or if player wants page releases: `subscribe page <page> --from <zip-or-folder>`. Player downloads in browser first (UI "open download page"); RimArcho installs zip (standard `archive/zip`; extract 7z/rar first) or folder into subscription directory: Mod at archive root or sole first-level subdirectory; refuse without `About/About.xml`. installed records page version/update time (`1.4.1 @ 2026-10-23T15:16:59Z`). Examples: RimJobWorld links a gitgud.io repository (Git); RJW Industrial Milk Management has only a download button (zip).
   - Update check rereads page; changed version or newer update time means update; update = `subscribe update <reference> --from <new-zip-or-folder>`, install in temporary directory, recycle original, rename into same location (R1, same Instance).
   - Page read failures (Cloudflare block, changed page layout) only record `check_error`, without retry or bypass.
   - After first login in settings › accounts, accepting site notice and logging in personally in isolated window, players can read downloadable files on subscribed pages without Git repositories; list multiple files by actual filename, size, date, falling back to button text only without metadata; player selection required. Authenticated `subscribe.LoversLab` does not change public reader contract. Updates use the same file selection flow.
   - Download using actual links/forms from this request, never guessed attachment IDs or CSRF parameters. Show site declarations verbatim and submit only after player agreement; hidden fields and temporary links stay in memory, never `source.json` or tasks. Download managed temporary zip, then reuse `subscribe.Import`, `About.xml` validation, install preview, and old-directory recycling; if public metadata is blocked, local installation records `player download`.
   - Missing/expired sessions, login redirects, 403, Cloudflare/captcha, or unrecognized pages explain the reason and return to "open download page → choose zip". No automatic challenge solving, login loops, or retries.
   - Save session encrypted with current Windows user DPAPI; cookies and non-secret status only, no passwords; do not invent persistent expiry for session cookies. Preserve site's Remember me in login window. Show expiry of longest-lived authentication cookie (member/login/device key, etc.), not earliest short session cookie; session-only means N/A. Merge Set-Cookie (rotation, expiry, deletion) from every HTTPS response in validation, file reading, download, and authenticated update checks into encrypted storage, surviving restarts. Public `ReadPage` stays cookie-free; logged-in GUI update checks reuse authenticated client.
   - Serialize per service; request spacing and 429 Retry-After cooldown in 06 §2.16.1 S11; refuse retries during long cooldowns. HTTPS only, exact `loverslab.com` / `www.loverslab.com` file flows; unregistered external CDNs not observed in practice fall back to manual download; never forward site cookies to CDNs.
   - No general "Mod index" `source:` no existing index to connect.

## 4. Scanning

### 4.1 Flow

```
Check source states → list source subdirectories → group (merge identical real paths) → read observations → reconcile → write snapshots → emit change events → mark stale facts
```

- Command: `archo scan [--full] [--game-only]`. `--full` ignores cached hashes and rereads all semantic files. TUI and GUI run `archo scan` in subprocesses; the main process does not scan, avoiding retention of peak scan memory in the interface process.
- **Scope**: shared Mod folders and current environment's own `Data`, `Mods`, once per directory. `--game-only` scans only the current environment's two sources: after adding an environment (guide step 2), only the new game needs reading. Active Instances in unscanned shared sources neither match nor tombstone but remain R4 "original" candidates, so `Mods/` copies still join Workshop originals' Lineages. If shared folders have never been read, guide step 2 omits `--game-only`.
- Before starting, check data directory per §2.1 and refresh each source state per §3.1; skip missing, `rebind-pending`, retired.
- When reading origin markers (`.rimarcho/origin.json`), first map merged-away IDs through `id_alias` (§0.1).
- A subdirectory is a Mod if it has `About/About.xml` or only `About/About.xml.disabled` (§4.2). Filenames are case-insensitive. Other subdirectories are counted without Instances.
- **Concurrency**: one goroutine per source, up to `NumCPU` workers within each. Assign IDs in deterministic reconciliation order, independent of concurrency.
- **Incremental**: reuse previous hash if relative path, size, and mtime are unchanged. mtime only skips hashing; it does not determine identity.
- **Memory**: incremental scans load only hashed semantic file rows, not the full list. Release lists after reading if directories are unchanged and no rehash occurred; if only external state changed (such as ACF manifest), copy old snapshot file rows to the new snapshot with SQL. Measured memory/timing: 07 §6.
- Scans update only identity and metadata. Analysis rebuilds Def, Patch, Harmony facts on demand (03 §3).
- Per-source scan output lists subdirectory count, Mod count, residue count, about-disabled count, non-Mod directory count, count-only residue directories, alias count; Workshop sources without ACF show `no ACF`.

### 4.2 Residue and about-disabled

**Subscriptions and residue** (workshop sources only; game does not directly load SteamCMD directories):
- Game loads only **subscribed** Workshop items (07 §7). Subscribed means present in `appworkshop_294100.acf` `WorkshopItemsInstalled` or `WorkshopItemDetails`.
- Without ACF, treat all as subscribed.
- Folders absent from subscription records are residue: still create Instances, but cannot add to plans; can it run suggests cleanup (03 §4.3, 08 §7).
- Residue folders without `About.xml` or `.disabled` are counted only, without Instances.

**about-disabled**:
- If `About/About.xml` is absent but `About/About.xml.disabled` exists (RimCrow's disable method), read it as declaration, create Instance, mark `unloadable: about-disabled`. Such Instances cannot join plans; filter with `list --filter unloadable`.
- Workshop residue folders containing only `About.xml.disabled` are both residue and about-disabled.
- Scan does not restore filenames. Analysis classifies them below, computed per analysis without persistence:

| Category | Condition | Finding code | Description |
| --- | --- | --- | --- |
| Locked version | Same Lineage has another live, loadable, non-Workshop Instance (RimCrow `_<Workshop-ID>_` copies, `copy-local` copies, and copies deployed in `Mods/` count) | `mod.pinned` (info; warning if upstream updated) | Do not report `mod.unloadable`; the copy is effective, Workshop copy is upstream. Product rules: 04 §7 |
| Unsubscribed residue | Workshop source, absent from ACF subscription list | `mod.hidden-residue` (warning) | May recycle (08 §7) |
| Independently hidden | Everything else | `mod.hidden` (warning) | Keep hidden, restore, or recycle (08 §6) |

### 4.3 Observations

Record for each Mod directory:

| Category | Contents |
| --- | --- |
| Location | `source_id`, relative path, real path, link flag, alias locations |
| Declarations | `About.xml`: `packageId`, name, author/authors, `modVersion`, `supportedVersions`, description, `modDependencies`` and ``modDependenciesByVersion`, `loadBefore`, `loadAfter`, `forceLoadBefore`, `forceLoadAfter`, `incompatibleWith`, `loadBeforeByVersion`, `loadAfterByVersion`, `incompatibleWithByVersion`, `descriptionsByVersion`; `About/PublishedFileId.txt` (strip BOM/whitespace); original `LoadFolders.xml` |
| Workshop ID | Workshop-type sources (workshop, steamcmd): directory name; others: `PublishedFileId.txt` |
| External state | Workshop-type sources: ACF manifest ID, timeupdated, size, subscription status; with `.git`: `HEAD` commit (read `HEAD`, then ref file or `packed-refs`) |
| Content fingerprint | See §4.4 |
| Origin marker | `.rimarcho/origin.json` (if present and `origin_instance` nonempty) |
| State | residue, unloadable |

**Tolerant `About.xml` parsing**:
- Encoding: detect UTF-16 LE/BE and UTF-8 BOM, convert to UTF-8; without BOM but with XML `encoding="utf-16"`, read UTF-16, unless actual content is UTF-8, then follow actual encoding. Applies to `LoadFolders.xml` and Defs/Patches too.
- Tolerate tag case, mixed author/authors, whitespace in li, package ID case.
- Split author like game `ModMetaData.Authors`: on ` and `, `&`, `,`, trimming whitespace.
- Dependency li may contain `packageId` child or plain text.
- Malformed `About.xml` still creates Instance; record error in declaration without interrupting scan.

Always store `packageId` twice: original `package_id_raw`, lowercase `package_id`. Derive missing `packageId` per §4.5.

### 4.4 Fingerprints and Similarity

| Layer | Scope | Calculation |
| --- | --- | --- |
| Structural list | All files | Relative path + size (+ mtime, incremental only) |
| Semantic hash | All files under `About/`, `LoadFolders.xml`, XML under any `Defs/` or `Patches/` (including version directories like `1.6/Defs/`), `.dll` under any `Assemblies/` | xxh3-128 per file |
| Snapshot hash | Both layers above | Sort by lowercase path; hash each "lowercase path + size + semantic hash (if any)" to quickly detect changes. Lowercase paths mean case-only renames on Windows do not count as updates |

- Large files such as textures/audio compare size only, without content reads.
- Exclude `.git`, `.svn`, `.rimarcho`, so locked versions and originals have identical fingerprints.
- Do not follow links or device files within `Mods`; skip unreadable entries.

**Similarity**: over two snapshots' semantic file sets, take the larger of two Jaccard coefficients:
- `J(paths)`: compare lowercase relative paths only. Recognizes "move + update": contents changed, structure unchanged.
- `J(hashes)`: compare content hashes without paths, as a **multiset** (count each identical-content file separately). Recognizes "reorganization": files move into subdirectories like `1.6/` without content changes.

With no semantic files on either side, similarity is 0. R3/R4 additionally require matching declarations; R6 does not equate same paths with same content, using content evidence in §5.1. Adding files to small `Mods` can lower similarity below 0.85 (for example adding one to 4 semantic files gives 0.8); R6 then asks for confirmation, an acceptable conservative outcome.

### 4.5 Derived packageId

Game permits very old `Mods` without `<packageId>`. RimArcho derives it like `ModMetaDataInternal.TryParsePackageId` (confirmed by decompiling 1.7.4982, 07 §7), marking "inferred" (`package_id_derived`):

```
ConvertToASCII(author + hash3(description)) + "." + ConvertToASCII(name)
```

- Default author: `Anonymous`.
- `hash3`: empty description gives none; otherwise compute `GenText.StableStringHash` (initial 23, UTF-16 code units `n = n*31 + c`, int32 wraparound), decimal, remove minus sign, take first 3 digits.
- `ConvertToASCII`: per UTF-16 code unit, retain letters/digits, replace other c with `c % 25 + 65`.
- Lowercase result into `package_id`.
- Official content (official source) missing `packageId`: `Core` uses `ludeon.rimworld`, others `ludeon.rimworld.<lowercase-directory-name>`.

## 5. Reconciliation

Input: all current scan observations; database Instances (including tombstones); unscanned source list. Output: Instance ID per observation, change events, identity questions, Instances disappearing this scan. Reconciliation is pure logic (`internal/identity`); caller handles database I/O.

### 5.1 Rules R1–R7

Matched observations and Instances do not participate in later rules.

| Rule | Condition | Result | Event |
| --- | --- | --- | --- |
| R1 | Same `source_id` + relative path (case-insensitive) | Same Instance | See §5.3 |
| R2 | Observation from Workshop-type source (workshop, steamcmd); another Instance from a Workshop-type source, same directory name (same Workshop ID), not seen this scan | Same Instance (for example Workshop content moved Steam libraries) | moved |
| R5 | Observation has `.rimarcho/origin.json` | New Instance, join recorded Lineage, mark "confirmed"; exceptions below | copied |
| R3 | A live Instance not seen this scan has similarity ≥ 0.85 to observation and matching `packageId` or Workshop ID declaration | Same Instance | moved |
| R4 | An Instance already seen this scan has similarity ≥ 0.6 to observation and matching `packageId` or Workshop ID declaration | New Instance, join its Lineage, mark "inferred" | copied |
| R6 | A tombstone or currently disappearing Instance passes `identity.QuestionScore` content check, with unique candidates in both directions (below) | New Instance, new Lineage, and identity question "is this #xxxx returning?" | discovered |
| R7 | Everything else | New Instance, new Lineage | discovered |

- **Matching declarations**: same nonempty `packageId` (case-insensitive), or valid Workshop ID; blank, `0`, and nonnumeric Workshop IDs do not match. R2 directory names must also be valid Workshop IDs.
- **R5 before R3/R4**: origin markers are authoritative even after major edits make similarity unrecognizable.
- **R5 rename exception**: if a live Instance not seen this scan has the same origin marker in its latest snapshot (same `origin_instance`) and similarity ≥ 0.85, treat as "copy renamed": same Instance, emit moved, no new copy. Otherwise renaming a locked version's directory would create another Instance.
- **R5 Lineage**: prefer marker `origin_lineage` if it exists in this environment; otherwise current Lineage of `origin_instance`; if neither exists, create Lineage (still emit copied).
- For multiple candidates in R2–R5, choose highest similarity.
- **R6 content evidence**: only content hashes of Defs / Patches XML and Assemblies DLL; exclude About files, previews, `LoadFolders.xml`, empty content; count duplicates once; path-independent. Content-set Jaccard ≥ 0.6; matching declarations require at least one shared content, otherwise at least three distinct shared contents. Names, authors, folder names, and common texture paths alone cannot trigger a question. Implementation: `identity.QuestionScore`, `identity.CompareFiles`.
- **R6 ambiguity guard**: if a new observation has multiple eligible candidates or multiple observations choose one candidate, record independently through R7 without questions; do not pick highest score or ask per pair.
- Store "inferred"/"confirmed" in Instance `lineage_basis` (§6).

### 5.2 Execution Order and Candidate Scope

**Two phases**:
1. Run R1, R2, R5, R3 rule by rule over all observations, matching only known database Instances.
2. Run remaining observations individually through R4 → R6 → R7.

This allows Instances just created by R7 in this scan to be R4 originals. Otherwise the first scan would assign unrelated Lineages to Workshop originals and `Mods/` copies, or the same Workshop ID in Steam and SteamCMD.

**Observation order**: workshop → steamcmd → official → local → extra; same-kind sources follow environment order, then lowercase relative path. Establish originals before copies.

**Candidate scope**:

| Rule | Candidates |
| --- | --- |
| R2 | Workshop-type Instances not seen this scan, **including tombstones**; prefer live. Matching tombstones emit reappeared, plus moved if location changed |
| R5 rename exception, R3 | Instances live before scan and not seen this time; no tombstones (reserved for R6) |
| R4 | Instances already seen this scan, including newly created |
| R6 | Tombstones plus currently disappearing Instances |

Instances in unscanned sources (missing, `rebind-pending`) are excluded from all candidates and do not become tombstones.

**Direction**: R4 "original" is whichever copy was processed first. If a RimCrow-disabled Workshop original becomes an Instance after a deployed copy (for example copy scanned before about-disabled support), events record the Workshop copy as an "inferred copy" of the deployed one. Lineage is unaffected; primary Instance still follows §6 priority.

### 5.3 Events

**Identity events**:

| Event | When | detail |
| --- | --- | --- |
| `discovered` | R6, R7 creation | `src:<label> <relative-path>` |
| `copied` | R4, R5 creation | R4: `inferred copy of #<original>`; R5: `origin #<source-Instance> (.rimarcho/origin.json)` |
| `moved` | R2, R3, R5 rename exception | `src:<old-label> <old-path> → src:<new-label> <new-path>` |
| `reappeared` | R1/R2 matches a tombstone, or player answers same to identity question | `src:<label> <relative-path>` |
| `disappeared` | Live Instance still unseen after scan (its source was scanned) | `src:<label> <relative-path>`; then tombstone |
| `updated` | Existing Instance matched and snapshot hash changed (including R2/R3 move + update) | — |

**Field events**: only when snapshot hash changes, compare old/new declarations and emit separately:

| Event | Comparison | detail |
| --- | --- | --- |
| `package-id-changed` | Lowercase `packageId` | `pkg:<old> → pkg:<new>` |
| `renamed` | name | `"<old>" → "<new>"` |
| `author-changed` | Author list | `<old, …> → <new, …>` |
| `version-support-changed` | `supportedVersions` | `<old> → <new>` (space-separated) |
| `dependencies-changed` | All dependency packageIds (including versioned, lowercased, deduplicated, sorted) | `+pkg:<x> -pkg:<y>` |
| `load-rules-changed` | `loadBefore`/`loadAfter`/`forceLoad*`/`incompatibleWith` and versioned forms (lowercase, sort, compare) | — |

- One scan may emit several events for one Instance. All share a scan ID (`scan:<n>`).
- Each event references before/after snapshots (`before_snapshot`, `after_snapshot`), allowing file-level diffs. Change history (06 §2.9) combines a Mod's events over a period into one row: compare declarations/file lists of earliest `before_snapshot` and latest `after_snapshot`; detailed `load-rules-changed` differences come from those declarations too.
- `_Archo_` link appearance/disappearance changes alias locations only, without events (§3.3).
- View events in GUI "mod library › changes" (06 §2.9), `archo changes [<reference>] [--since …] [--all]` (latest scan by default, 05 §4), and MCP.

**Snapshot write conditions**: write a new snapshot only when snapshot hash, ACF manifest, git `HEAD`, or origin marker changes. If only mtime changes, update file mtimes in current snapshot directly. ACF-manifest-only changes create a snapshot without updated.

### 5.4 Identity Questions

- R6 records the observation's new Instance, candidate Instance, content overlap, and a small comparison summary (shared content count, each side's content count, declaration differences). Store summary with question; page construction reads question records only, not file lists.
- Every successful scan rechecks visible unanswered questions in the same `Store.ApplyScan` transaction: apply R6 content checks to current snapshots; delete unanswered questions with insufficient evidence, missing references, or multiple eligible questions involving the same Mod. Use stored snapshots only, so check unscanned sources too (partial scans, other games' `Mods/`) (since 2026-10-28; previously retained, leaving 67 incorrect questions about one VRE patch on the host). Exclude answered questions, including player same / copy / unrelated decisions. Do not modify `Mods`, lists, history, or grouping.
- View: `archo identity [list] [--all]` (no subcommand also lists; `--all` includes processed), `archo identity why q:<n>`; GUI "can it run › needs your answer" and editor problems panel (06 §2.4, §5). CLI/MCP list, inspect, and answer include the same brief comparison.
- **Load comparison evidence on demand**: `Store.QuestionEvidence` reads declarations/file records from stored observation snapshot and candidate snapshot at question scan. Folders and directory-derived Workshop IDs use current records; explicitly say comparison unavailable if records were cleaned. `cmds.CompareIdentity` / `IdentityReasonLines` share comparison: omit fields absent on both sides, give one content overlap conclusion, up to five shared files plus remaining count; no matching-method explanation. UI structure: [06 §5](06-ui/4-details-and-feedback.md#5-can-it-run-wording). No Mod file scan or network reads.
- Answer: `archo identity resolve q:<n> same|copy|unrelated`; UI "same", "a copy", "unrelated".
  - same: merge new Instance into candidate. Candidate keeps ID, Lineage, history, takes new location, snapshots, events, aliases; rewrite current lists, undo/redo, historical versions, staging entries/anchors across all plans in one transaction; retain first duplicate position and unchanged missing entries; rewrite launch list fingerprints too. `id_alias` maps merged ID and emptied Lineage; pack manifests/origin markers resolve to survivor without modifying files; delete new Instance's discovered, add reappeared under question's scan; mark other questions involving both Instances superseded. Refuse same if R1/R2 already matched candidate back meanwhile.
  - copy: join candidate Lineage, mark "confirmed".
  - unrelated: close question only.

### 5.5 Tombstones and gc

- Disappeared Instances become tombstones with `tombstone_at`, retaining Lineage, rules, aliases, historical snapshots.
- Default retention 90 days, configurable with nonnegative `tombstone_days` in `config.toml`.
- `archo gc [--dry-run]` (settings "clean tombstones") removes:
  - Expired tombstones and events, except those referenced by unanswered questions;
  - Old snapshots neither current nor referenced by events/questions;
  - Lineages made empty.
- `--dry-run` counts only.
- Players run tombstone gc manually. Periodic database maintenance does not change tombstone/identity behavior; it only cleans old file lists per §7.1.

## 6. Lineage (Including Locked Versions)

- Every new Instance creates its own Lineage by default (empty `lineage_basis` means "founding member").
- Ways to join existing Lineages and markers:

| Method | `lineage_basis` |
| --- | --- |
| R4 inferred copy | inferred |
| R5 origin marker, copy answer | confirmed |
| Manual split/merge | manual |

- Manual split/merge: `archo lineage split <reference>` (move Instance into its own new Lineage), `archo lineage merge <reference> <reference>` (merge second Lineage into first).
- Splitting copies original alias, Mod notes, tags into new group, then edits are independent; copy notes stay with Instance. User rules/finding ignores stay with original group. Change identity and copy player data in one transaction; Instance ID, path, snapshots, plan entries stay unchanged. Rescans preserve manual grouping; origin markers/upstream update relationships remain.
- **Primary Instance**: default Lineage display name; priority: live → Workshop-type source → earliest `first_seen` → ID.
- **Lineage attachments**: user rules and player data (aliases, tags, notes) belong to Lineage, shared by originals/locked versions and survive renames; plan entries belong to Instances (04 §1, 04 §8; rationale 00a §4.3).
- **Known hard cases**: author forks a new Mod with different Workshop ID but same `packageId` → separate Lineages, analysis reports duplicate package ID; same Workshop ID in Steam Workshop and SteamCMD → two Instances, one Lineage.

**Locked version identity** (product rules 04 §7):
- Locked version is a new R5-confirmed Instance in the same Lineage. Creation (`copy-local` is low-level command) copies Mod to local source (default `<original-directory-name>_local`) and writes `.rimarcho/origin.json`:

```json
{
  "origin_instance": "<full source Instance ID>",
  "origin_lineage": "<full Lineage ID>",
  "workshop_id": "<Workshop ID, may be empty>",
  "origin_snapshot": "<snapshot hash at copy time, hexadecimal>",
  "copied_at": "2026-10-21T08:00:00Z"
}
```

- Next scan R5 rejoins original Lineage, marks "confirmed", emits copied. Later directory rename follows §5.1 exception as moved, retaining locked version identity.
- RimArcho never writes Workshop marker files, because Steam verification/updates delete them.
- Manual player copies have no marker; R4 marks "inferred".
- RimCrow style (Workshop `About.xml.disabled`, copy in `Mods/`): R4 joins both to one Lineage; analysis recognizes locked version per §4.2.
- unpin swaps plan copy back to Workshop version without changing identity data; copy stays in place (04 §7).

## 7. Storage

**Schema and upgrades**: new databases are created from `internal/store/schema.sql` (embedded), then `meta.schema_version = store.SchemaVersion`. The first public release (1.0.0) shipped schema 29 (`store.FirstReleaseSchema`). A schema change edits `schema.sql`, bumps `SchemaVersion` and appends one step to `store.upgrades`; opening an older database runs the missing steps in order, each in one transaction, which may report row progress. A database with schema 1–28 (a pre-1.0 development build) is refused with `store.ErrPreRelease` and left unmodified: "database from a pre-1.0 development build (schema N); move <path> away to start with a new database". A schema newer than `SchemaVersion` is refused as "unsupported schema version". Test: `TestPreReleaseDatabaseRefused`.

**Startup upgrade progress** (no upgrade step exists yet, so it never shows): display progress when opening runs upgrade steps (`store.OpenUpgrading`, `app.OpenWith`, `cli.upgradeReport`); hide if complete within 300 ms. `RimArcho.exe` has no console; show a small native window (`internal/surface/upgradewin`: RimArcho title, one text line/progress bar, cannot close, system DPI/interface language), closed when the upgrade completes; TUI writes the same line to stderr before taking the terminal (in-place percentage refresh, interface language); CLI/MCP write one English stderr line "Upgrading the RimArcho database (schema N → M)…", then elapsed time. stdout unchanged; MCP protocol stream gets no extra content. Steps that report rows show a percentage; others show only "upgrading". If opening fails, `RimArcho.exe` (no console) also shows a modal error box in the interface language (`cli.upgradeReport.openFailed`, `upgradewin.ShowError`): a pre-1.0 database gets its own text naming `archo.db` and saying to move or rename it; other errors show the error text. OK exits; the CLI, TUI and MCP only write stderr.

**Managed links** (`managed_link`): link creation records, with phase/path indexes; existing Instances/aliases are never converted into proven creation records. Commit intent before link file operations, then detached, `records-updated`, complete; safe rollback records `rolled-back`. Startup checks unfinished rows read-only; problems page/scan report `mods.link-recovery`; recover only through existing confirmed `links repair`, never silent rebuilding.

**Identity question summary**: `identity_question.summary` stores §5.4's small comparison summary; `identity_question_open(status, environment_id)` indexes unanswered questions.

**Fact cache structure and cleanup**: set structure and reference cleanup: 03 §3; `Store.PruneFacts(now)` is an independent maintenance entry point. Environment removal/tombstone cleanup also prune in the same transaction; visible environments and analysis slots retain shared Instances.

SQLite (modernc, WAL + `synchronous=NORMAL` + `journal_size_limit` 16 MB). After a large write, next reset truncates WAL to 16 MB instead of retaining peak size while connections remain open; after analysis writes or scans creating snapshots, also attempt nonwaiting `wal_checkpoint(TRUNCATE)` (`Store.TruncateWAL`). Measured: about 380,000 analysis rows leave about 100 MB WAL while GUI is open. Identity tables (empty `environment_id` = shared across all environments):

```
environment(id, name UNIQUE, game_root, user_data_root, game_version, launch, created_at, current_plan)
source(id, environment_id, kind, label, root, status[ok|missing|rebind-pending|retired], pending_root,
       UNIQUE(environment_id, label))                       -- shared folders: environment_id=''
lineage(id, environment_id, primary_instance_id, created_at) -- all environment_id=''
instance(id, environment_id, lineage_id, lineage_basis[''|inferred|confirmed|manual], source_id, rel_path,
         real_path, status[live|tombstone], residue, unloadable, current_snapshot_id,
         first_seen, last_seen, tombstone_at)
instance_alias(instance_id, source_id, rel_path, real_path)
snapshot(id, instance_id, scan_id, snapshot_hash, package_id, package_id_raw, name, authors,
         workshop_id_declared, supported_versions, decl_blob, acf_manifest, acf_time_updated, acf_size,
         git_head, origin, file_count, total_size, semantic_count, kinds, langs)
snapshot_file(snapshot_id, rel_path, size, mtime, hash NULL)   -- semantic files (only these have hashes) and DLL
snapshot_list(snapshot_id PRIMARY KEY, n, data)               -- remaining files: one compressed block per snapshot
scan(id, environment_id, started_at, finished_at, full, stats)
change_event(id, scan_id, instance_id, kind, before_snapshot, after_snapshot, detail)
identity_question(id, environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id,
                  score, status[open|resolved], resolution[same|copy|unrelated|superseded], resolved_at, summary)
meta(key, value)   -- schema_version, current environment, etc.
id_alias(old_id PRIMARY KEY, new_id, kind[instance|lineage])  -- merged-away IDs (§0.1, §5.4)
```

- `decl_blob` stores normalized `About.xml` (JSON) for analysis.
- **Snapshot file lists have two locations** (`store/filelist.go`): hashed semantic files and `.dll` (any location, ASCII case folding only) are `snapshot_file` rows; remaining files (textures, sounds, Languages, `.meta`, etc.) form one `snapshot_list` block: sort by `rel_path`; each entry stores shared prefix length with previous entry, suffix, size (no mtime: incremental scan alone needs it and reads only hashed rows, `Store.CurrentFiles`); zstd compress entire block; n is count. Blocks **are not deduplicated**: copying lists for identical content/new external state copies whole block; refreshing same-snapshot mtime rewrites whole block. Snapshots without non-semantic files have no block. Readers: `Store.SnapshotFiles` (change-page file diffs) merges both and sorts by path; `Store.InstanceFiles` (log texture / DLL provider lookup) queries rows first, skips blocks for `.dll`-ending patterns, otherwise decompresses current blocks and matches SQLite `LIKE` rules in Go (`%`, `_`, ASCII case folding only). DLL remain rows because DLL-name Mod lookup (`%x.dll`) is the commonest query and must not decompress every block.
- `snapshot.kinds`, `snapshot.langs` are derived from file lists at snapshot write (`store.deriveContent`). kinds bits: `.dll` anywhere; XML under `Defs/` or `Patches/` at any loading-folder depth; files under `Languages/`; PNG under directories whose names end with textures (`%textures/%.png`, convertible textures). langs: language names from `Languages/<name>/…` at root or next-level loading folder, in path order, first case-insensitive duplicate only, joined by `/`. ASCII folding matches previous SQLite `LIKE`. Snapshot paths never change (different path/size means different snapshot hash), so columns stay valid even after §7.1 removes files; identical-content/new-external-state list copies copy both columns. Mod inventory content types (`Store.ContentKinds`), convertible textures (`Store.SnapshotsWithPNG*`), detail languages (`Store.Languages`) read columns by primary key instead of scanning `snapshot_file`.
- `instance.environment_id` follows `source:` empty for shared folders, owning environment for `Data`/`Mods`. Visible Instances have `environment_id IN ('', <environment>)`.
- Global: Lineage, player data (`lineage_data`, `lineage_tag`), copy notes, user rules, change events (by Instance ownership), plans (04 §1.7). Per environment: current plan, launch records, analysis runs, finding ignores, scan records. Per user data directory: ModsConfig.xml write records.
- Schema creation and upgrades: "Schema and upgrades" above; pre-1.0 databases are refused, not upgraded.
- Version reminder preferences are global database records; storage in 03 §4.2.
- Plan/rule/player-data tables: 04; fact/analysis tables: 03 §3; launch records: 08 §1.

### 7.1 Snapshot File List Retention

`Store.Maintain` cleans `snapshot_file`` and ``snapshot_list` together per snapshot, retaining snapshot rows (declarations, hash, file count, total size) and events. Keep full lists for the union of:

- Every Instance's current snapshot, including tombstones, for incremental scans and identity matching.
- Unanswered questions' `observation_snapshot_id`, plus candidate snapshots selected by `questionevidence.go`: candidate Instance snapshot at or before question scan, excluding observation snapshot, latest by `scan_id DESC, id DESC`. Both current snapshots are also protected above, preserving semantic sets for scan rechecks.
- Before/after snapshots of each Instance's latest 3 updated events, ordered by `scan_id DESC, id DESC`. No age limit.

Delete entire file lists for other snapshots, not only unhashed rows. modchanges still shows declarations/events; if lists cannot be compared, add only "(file lists no longer stored)", without misreporting the remaining side as additions/deletions.

### 7.2 Database Maintenance and Space Reclamation

Entry: `App.Maintain` → `Store.Maintain`; settings "compact", `archo db vacuum [--dry-run]`, MCP `maintenance action=vacuum`, and GUI automatic maintenance share it. Fixed order:

1. Clean old file lists per §7.1 in one transaction.
2. In that transaction, clean version views no game needs, expired non-current analysis slots, unreferenced fact sets per 03 §3 (`pruneFactsOn`, same as analysis-save cleanup).
3. After commit, reclaim space: manual always VACUUM; automatic only if free pages total at least 16 MiB, or at least 1 MiB and 20% of database pages. Keep existing auto_vacuum mode, no schema change.
4. `wal_checkpoint(TRUNCATE)`, record `meta[maintenance:last_completed]`, truncate completion-marker WAL too. Completion says released/current size ("database compacted: freed 183 MB, now 260 MB").

Before VACUUM, require free space on database volume at least twice database page size total plus 64 MiB for temporary database/full WAL; skip reclamation if insufficient/unknown, with manual required-space message ("cannot compact database: it needs 1.2 GB of free disk space"). Already-committed file-list cleanup needs no restore; retry later. Private maintenance connection has zero busy wait; SQLite busy / locked or busy checkpoint aborts immediately, silently for automatic maintenance. On interruption/failure, attempt WAL truncation; otherwise retry later. `meta[maintenance:lease]` prevents multiple windows entering between VACUUM transactions; expiry in 06 §2.16.1 A10.

GUI alone schedules automatic maintenance after first paint; initial delay, attempt period, successful interval, and recheck frequency in [06 §2.16.1](06-ui/2.16-scheduled-behavior.md) A08, A10; completion time in meta key above. Skip while any registered game's process, scan, analysis, running/queued background task, or page background file read exists; recheck continuously, cancel on foreground work/game start or window close. Use independent connection, no page connection or page mutex; retry later if busy. CLI, MCP, TUI do not schedule this.

Manual maintenance ignores period/idle restrictions but still refuses while a game runs, asking to close it. `--dry-run` counts file records to delete only (rows plus block entries), without cleanup, compression, or completion-time writes. Tombstone gc: §5.5.

## 8. Acceptance

Synthetic temporary games verify same-volume/simulated cross-volume lock-library moves, recycling, and cross-game pack removal after linking into two games; foreign inbound links/running games block; read-only reports after interruption at each phase and confirmed recovery. All use real junctions/platform symlinks, never player games or RimArcho AppData.

| # | Scenario | Expected | Test | Status |
| --- | --- | --- | --- | --- |
| A1 | First scan of over 300 `Mods` | One Instance per directory; discovered count equals Mod count | `TestA1FirstScanManyMods` | Pass |
| A2 | Workshop updates a Mod | Same Instance, updated | `TestA2WorkshopUpdate` | Pass |
| A3 | Author changes `packageId` in update | Same Instance, `package-id-changed`; unchanged Instance/Lineage IDs preserve plans/user rules | `TestA3PackageIDChange` | Pass |
| A4 | Move entire game to another disk, change environment path | Rebind succeeds; all Instance IDs unchanged | `TestA4GameFolderMoved` | Pass |
| A5 | Manually copy Workshop Mod to `Mods/` | New Instance, original Lineage, "inferred" | `TestA5ManualCopy` | Pass |
| A6 | Copy with `.rimarcho/origin.json`, heavily edited | New Instance, same Lineage, "confirmed", copied; later directory rename emits only moved | `TestA6OriginMarker` | Pass |
| A7 | Rename local Mod directory | Same Instance, moved; rename + edit: moved + updated; rename + add 1 file to 4-semantic-file Mod → discovered + disappeared + 1 question | `TestA7LocalRename` | Pass |
| A8 | Resubscribe within 30 days | Same Instance, reappeared | `TestA8Resubscribe` | Pass |
| A9 | Same Workshop ID in Steam and SteamCMD | Two Instances, one Lineage | `TestA9SteamAndSteamCMD` | Pass |
| A10 | Unrelated `Mods` share `packageId` | Two Instances, two Lineages; `pkg:` lists all candidates (exit 3); analysis reports duplicate package ID | `TestA10SamePackageIDUnrelated` | Pass |
| A11 | Directory appears in two sources through junction | One Instance, one alias location | `TestA11Junction` | Pass |
| A12 | `copy-local` creates locked version | New Instance in `Mods/`, `origin.json`, same Lineage, "confirmed"; no `.git` copy or following links outside Mod | `TestCopyLocal` | Pass |
| A13 | RimCrow style: Workshop about-disabled + `Mods/_Link_<id>` junction to RimCrow repository copy | Two Instances, one Lineage; Workshop `unloadable: about-disabled`, `packageId` read normally; `list --filter unloadable` matches; analysis recognizes locked version (`mod.pinned`, 04 §10) | `TestAboutDisabled`, `TestHiddenPinned` | Pass |
| A14 | Enable SteamCMD Mod, write to game, `_Archo_` link in `Mods/`, rescan | Link is alias; no new Instance/events; removing link and rescanning also emits none | `TestLinkDeployment` | Pass |
| A15 | Scan with pack `!ArchoPack_` links in `Mods/` | Skip links; Instance count unchanged | `TestPackBuildAndLaunch` | Pass |
| A16 | Two environments (e.g. `1.6`/1.7) share default user data | `env add` permits; outside changes judged by user data directory, no false report switching A→B after A writes; attribute to writer plan; logs exclude other game's files per 03 §6.4 | `TestV8SharedUserData`, `TestForeignLogExcludedBeforeAnalysis` | Pass |
| A17 | Two environments share Workshop | Same Mod has same Instance/Lineage, tags/notes in both; `Mods/` copies join Workshop Lineage even with `--game-only`; plans belong to their respective environments; "used in" lists both sets; environment removal retains shared `Mods`/tags; cannot add same game directory twice | `TestEnvModelSharedWorkshop` | Pass |
| A18 | Unsubscribed Mod and non-Mod folders in Workshop | Former is residue Instance; latter counted only (`ResidueNoAbout`); subscribed not marked residue | `TestResidue` | Pass |
| A19 | Unsubscribed Workshop folder has only `About.xml.disabled` | Both residue/about-disabled | — | Pending validation |
| A20a | 67 small `Mods` share only About/preview/texture paths with tombstone, or content forms 1:67 / 67:1 | No false questions; no questions for bidirectional ambiguity; next scan removes old false questions, retains answers | `TestIdentityQuestionsCommonPathsFanOut`, `TestIdentityQuestionsAmbiguousContent`, `TestIdentityQuestionsRecheckStored` | Pass |
| A20b | Newly discovered Mod referenced by current list, old versions, undo/redo, staging anchors, packs; answer same/copy/unrelated | same remaps references transactionally to original, historical checkout retains entries; copy/unrelated retain references; failure rolls back all | `TestIdentityAnswersPreserveListReferences`, `TestResolveSameReferenceRewriteRollback` | Pass |
| A20 | Disappeared Mod appears elsewhere with new `packageId` (R6), answer same | Restore original Instance ID, add reappeared, delete new Instance discovered | `TestQuestionResolveSame` | Pass |
| A21 | `lineage split` / `lineage merge` | Membership follows command, manual | `TestLineageSplitMerge` | Pass |
| A22 | gc after tombstone expiry | Remove expired tombstones/events, retain unanswered-question references | `TestTombstoneGC` | Pass |
| A23 | Only Workshop ACF manifest changes | New snapshot, no updated | `TestManifestOnlyChange` | Pass |
| A24 | Insufficient path-rebind samples; answer no | `rebind-pending`; no retires/relabels old source `<label>-old`, tombstones Instances next scan, new root becomes new source | `TestA4RebindRejected` | Pass |
| A25 | Move all files into `1.6/` unchanged; change contents with unchanged paths | Both have sufficient similarity, same Instance | `TestSimilarityRestructureAndContentOnly` | Pass |
| A26 | Remove steamcmd source/environment | Refuse removing local; removed steamcmd Instances emit disappeared; `env remove` refuses without `--yes`, otherwise deletes all environment rows | `TestEnvRemoveAndSourceRemove` | Pass |
| A27 | `About.xml` UTF-16 LE/BE BOM, UTF-8 BOM, declares utf-16 but UTF-8 | Read declarations correctly | `TestReadDeclUTF16` | Pass |
| A28 | Old Mod lacks `<packageId>` | Derive per §4.5, matching game, "inferred" | `TestDerivePackageID` | Pass |
| A29 | `Data` directory inside game/source root | `env add`, `env set`, scan refuse with explanation | — | Pending validation |
| A30 | Save modes | `--saves own` creates `<data-directory>/userdata/<environment-name>`, launches with `-savedatafolder`; `--saves shared` restores default; refuse `--saves` + `--user-data` | `TestEnvModelSaves` | Pass |
| A31 | Add/remove Mod folders | Adding existing directory unchanged; refuse containment/game `Mods`; remove shared folders only, Instances disappeared next scan; second Workshop directory means rebind with unchanged Instances | `TestFolderCommands` | Pass |
| A33 | Subscribe to local bare Git repository (branch/tag/commit) | Subscription folder has `.git`/`.rimarcho/source.json`; one scanned Instance; refuse duplicate URL/target; refuse non-Mod root and list subdirectories | `TestSubscribeGit` | Pass |
| A34 | Bare repository branch gains commit | `subscribe check` marks update; `update --dry-run` no writes; update advances commit, same Instance; refuse dirty tree | `TestSubscribeGitUpdate` | Pass |
| A35 | Workshop subscribe with Steam available/unavailable | Available: Steam client only; unavailable: located SteamCMD (`workshop_download_item 294100 <ID>`), download directory registered as steamcmd; missing SteamCMD reports error/page | `TestSubscribeWorkshopFallback` | Pass |
| A36 | `subscribe remove` | Recycle folder (fake bin in test), warn if plan uses it; refuse outside subscription directories/SteamCMD sources | `TestSubscribeRemove` | Pass |
| A37 | Download page (local test HTTP server, fake neutral file page) | Read title/update/repository; `--git` subscribes repository and records page; `--from` zip installs; newer page time makes check report update; `update --from` preserves Instance ID | `TestSubscribePage` | Pass |
