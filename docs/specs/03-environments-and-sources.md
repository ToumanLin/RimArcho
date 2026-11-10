# 03 · Environments and sources

> Status: implemented except where marked "not implemented".
> Basis: 00, 00a §4, 02 §1.

This specification defines environments and sources: the terms, what an environment owns, environment fields and detection, source kinds, rebinding, and links and aliases. Scanning, reconciliation, Lineage and their storage are in 04. Design rationale: 00a §4.

## 1. Terms

| Term | Definition | UI name (16 §7.2) |
| --- | --- | --- |
| Environment | One game directory plus its save mode (user data directory). A player may have several, such as a licensed 1.7 and a 1.5 downloaded with SteamCMD | game |
| Source | A root directory that holds Mods. A game's `Data` and `Mods` belong to its environment; the others are **Mod folders**, shared by all environments | mod folders; source in filters |
| Instance | One Mod copy on disk, with an ID RimArcho assigns | this one |
| Snapshot | An Instance's metadata and content fingerprint from one scan | — |
| Lineage | A family of "the same Mod": the Workshop original, a locked version, the Mod before and after a rename | copies of the same mod |
| Tombstone | The kept record of an Instance that has disappeared | missing mod |
| Alias location | Another link location that points to the same real directory; it creates no other Instance | — |
| Identity question | An uncertain reconciliation match, "is this #xxxx returning?", for the player to answer | for you to confirm |

A plan (07) is a **list** in the interface; for other interface terms see 16 §7.2.

**IDs**: Instance, Lineage, environment and source IDs are random 128-bit values in lowercase base32 (alphabet `a–z2–7`, no padding, 26 characters). Output shows the shortest unique prefix among the Instances this environment sees (shared folders and its own), at least 4 characters; Instance and Lineage prefixes are computed separately. Reference syntax (`#a3f9`, `L#7c2e`, `pkg:`, `ws:`, `q:`, `src:`, `scan:`, `env:`) and ambiguity handling are in 13 §2. Databases and event details store full IDs; only output shortens them.

## 2. One environment per game directory

An environment owns one game directory. The Workshop directory, SteamCMD directories, extra folders, the Archo Monitor directory and the lock library are shared by all games: keeping them per environment would only create several records for the same Mod. What belongs where:

| Ownership | Contents |
| --- | --- |
| Environment (one game directory) | Its own `Data` (official) and `Mods` (local) sources, game version, launch method, launch arguments, current plan, launch records, `_Archo_` deployment links, save mode |
| Global (shared by all environments) | **Plans** (07 §1.7); **Mod folders**: the Workshop directory (found through Steam), SteamCMD directories, extra folders, the Archo Monitor directory, the lock library. Their Instances and Lineages, tags, notes, aliases, copy notes, history, user rules |
| By game version | Analysis slots and fact views; identical fact sets are shared across versions (05 §3) |
| By user data directory | ModsConfig.xml write records (§3.4, 07 §4.3) |

**Rules**:
1. **Shared sources**: `source.environment_id = ''`, and so is their Instances' `environment_id`; environment-owned official and local sources and their Instances keep their environment ID. All Lineages are global (`environment_id = ''`), so locked copies in `Mods/` and Workshop originals still share one Lineage.
2. **Labels**: shared source labels are globally unique (deployment link names contain them, 09 §3). Environment sources have only the labels official and local (plus retired names), which are also unique within "shared + this environment".
3. **Workshop directory**: at most one active directory. Adding another Workshop directory with `folder add` changes the existing one's path by rebinding (§4.2). When an environment is added whose game is in a Steam library and no Workshop directory exists yet, that library's Workshop directory is added automatically ("find through Steam").
   **A game's own Workshop directory is a SteamCMD directory.** A game downloaded with SteamCMD has `steamapps\workshop\content\294100` inside its game directory (`force_install_dir`). When it exists and is not the Steam client's Workshop directory, it is registered as a shared steamcmd source, shown in Settings "mod folders" like a folder the player added, and deployed through `_Archo_` links when enabled (09 §3). It is registered on `env add` (including "download a version"), on `env set` changing the game directory, and by a check across all environments before every scan (which also covers environments added earlier). It is **never** treated as a Workshop directory, so it never counts as "a second Workshop directory means changing the path"; `--workshop` pointing there also adds it as a SteamCMD directory. The pre-scan check does not add again a directory registered before, including one the player removed (retired): removed stays removed. **Explicitly adding the game** (`env add`, also re-adding a removed game) or pointing `env set` at it does restore the retired directory: the original source record comes back (same ID, label assigned again from steamcmd), so its returning Mods keep their Instances, lists, tags and notes. `folder add` of a retired directory (same path and kind) also restores the record instead of creating one. Test: `TestGameWorkshopIsSteamCMD`.
4. **One game directory belongs to only one environment**: `env add` refuses a game directory another environment uses.
5. **Saves**: `--saves shared` (the default: the game's default user data directory) or `--saves own` (RimArcho creates `<data-directory>/userdata/<environment-name>` and passes `-savedatafolder` on launch, 09 §2). There is no separate field; the mode is inferred from `user_data_root`: equal to the game's default means "shared default", under `<data-directory>/userdata/` means "own", anything else means "custom" (`--user-data` remains available). The data directory check (§3.1) allows own saves because RimArcho owns that directory.
6. **User rules are global, by Lineage**: rules describe relationships between Mods (load after, incompatible) that do not depend on the game being played. Global rules fit "one record per Mod", so the player states each rule once. A rule applies only when both Mods are enabled, so it has no effect in another game.
7. **Finding ignores are per environment**: a finding judges "this game, this version, these Mods together" (missing 1.7 support, a conflict with another Mod); accepting it in a 1.6 game does not make it safe in 1.7. Ignores already expire when the Mods change; sharing them across games would hide real findings. Lineage IDs in ignores are rewritten when Lineages merge (§2.1).
8. **Scan scope**: shared folders plus the current environment's `Data` and `Mods`; `scan --game-only` scans only the environment's own sources (04 §1.1).
9. **Global lock library**: one shared directory, `lock.dir` (default `%LOCALAPPDATA%\RimArcho\locked`), registered as the shared source `src:locked` (07 §5.1).
10. **Bundled Archo Monitor directory**: one shared source `src:monitor`. Monitor identity across sources: 19 §6.
11. **Mod library**: it covers the whole machine, including other games' own `Mods/` (07 §1.7), where each game's `Mods/` has the source value `local-<environment-name>` (`local-main`, `local-1.6`) and `source:local` matches all of them (`app.LocalValue`, `app.LocalMatches`).
12. **Commands**: `archo env add <name> --game <directory> [--saves shared|own] [--launch …]` and the group `archo folder list|add|remove` (MCP reads them with `status show=folders`; changing environments and settings stays with the player). `env add/set` `--workshop`, `--steamcmd` and `--extra` remain as aliases of `folder add`; `--remove-source` on a shared source equals `folder remove`. Full syntax: §3.1, 13 §4.
13. **Interface**: the "mod folders" settings group; the environment page shows only the game directory, version, launch method, saves and arguments; the first-run guide's step for adding an environment offers "a game you have" and "download a version" (SteamCMD, 11 §3).
14. **Links on environment switch**: `env use` (also the GUI and TUI switches) first creates missing `Mods/` links according to the target environment's ModsConfig.xml and current plan, then scans its `Data/` and `Mods/`, then makes it current. Shared sources such as Monitor use normal deployment links; the previous environment's local Mods get `_Archo_env_<Instance ID>` links to their real directories, which stay local Mods of the target environment and are not deleted on save or disable. If the target `Mods/` already contains the same package ID, that copy stays and no duplicate link is made; own saves without a list do not inherit one. Switching does not write ModsConfig.xml. It reports an error if links are needed while the target game runs; when creating links fails, the new links are rolled back and the original environment stays current. Tests: `TestEnvUse*`.

Creating and rolling back the links of item 14 writes the persistent ledger first; only healthy links are kept. Inbound links across all games are maintained before targets are moved or deleted; failure and restart recovery: §4.3, 09 §3.

### 2.1 Merged IDs

When two Instances or Lineages become one (a "same" answer, 04 §2.4; repairing the local records of shared-directory links, `Store.MergeLinkedInstances`; credit remapping, `Store.RemapCreditPlans`), one transaction rewrites every stored reference (`store/idremap.go`: `rewriteInstances`, `rewriteLineages`, `rewriteOrigins`, `mergePlayerData`, `mergeRules`, `mergeCopyNote`). The merged-away record is deleted. `instance_alias`, snapshots, `change_event` (the merged-away Instance's discovered event is deleted), `identity_question` (superseded once both sides are the same Instance), source use (05 §5.4) and stored origin markers follow the kept ID. List entries in the workspace, undo stack, versions and staging keep only the first duplicate, preserving order. Copy notes and `lineage_data` notes that differ are joined with a blank line (the kept alias wins), tags are united, user rules are rewritten and deduplicated, and finding ignores are rewritten (still per environment).

On-disk IDs are not changed: `id_alias(old_id, new_id, kind)` records each merged-away ID, and reading `pack.json` and `.rimarcho/origin.json` maps IDs through it first (`Store.IDAliases`).

### 2.2 Interface directory roles and aliases

In the interface, `source:` means the registered directory role: official, workshop, steamcmd, local, locked or other, independent of how the Mod was downloaded (Git or web). The lock library is recognized with `IsLockSource`. An extra directory has the role other plus a stable alias: its `Source.Label`, prefixed with `folder/` when it conflicts with a reserved word, or `folder/<Source.ID>` when it has none; the final directory name is displayed. Rebinding changes only the path, not the alias. This needs no table or migration. Paths remain in details, the CLI and the compatible `location:` queries, but are not GUI filter or completion candidates. Copy-row names are separate from filter roles: locked versions include their creation time; Git and web subscriptions may keep a description of the download method.

## 3. Environments

### 3.1 Fields

| Field | Description |
| --- | --- |
| `id` | Random ID |
| `name` | Chosen by the player; not empty, no spaces, `:` or `#` |
| `game_root` | Game installation directory. Valid if it contains `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldWin.exe`, `RimWorldLinux`, `RimWorldMac.app` or `Data/Core`. One game directory belongs to only one environment (`env add` refuses a directory another environment uses) |
| `user_data_root` | User data directory (containing `Config/ModsConfig.xml`), set by the save mode: **shared default** (`--saves shared`, the default: the game's default user data directory, which other environments may share, §3.4), **own** (`--saves own`: RimArcho creates `<data-directory>/userdata/<environment-name>` and passes `-savedatafolder`, 09 §2), or a **custom directory** from `--user-data`. Inferred from the path, with no separate field |
| `game_version` | The first line of `<game_root>/Version.txt` (UTF-8 BOM and surrounding whitespace stripped), **kept complete**, such as `1.6.4633 rev1261` or `1.7.4985 rev1269`. Comparisons use only major.minor (`1.6`; "Full game version" below). Reread on each scan and updated if changed |
| `launch` | direct or steam. Without an explicit choice at `env add`: steam for a game in a Steam library (`…/steamapps/common/RimWorld`), otherwise direct; specifying a Workshop directory does not change this default; the player may still choose the launch method explicitly |
| Launch arguments | Stored in settings (`env:<id>:launch_args`), changed with `env set --launch-args` or on the settings page; not in the environment table |

**Full game version**. The whole `Version.txt` line is always stored without truncation; "`1.6`" is computed when needed. Code: `env.ParseVersion(s) → env.Version{Full, Major, Minor, Build, Rev}`; `Short()` returns `1.6` (`env.MajorMinor` equals `ParseVersion(s).Short()`). Format: `major.minor.build revrevision`; `major.minor` alone is also accepted, with an unknown build.

- **Compare major.minor**: Mod `supportedVersions`, allowed plan versions (07 §1.6), the analysis cache's version key, the `--game-version` / MCP `game_version` override (which accepts only values like `1.6`), whether a pack can launch on this game (10 §3), the environment chip ("main  `1.6`"), the TUI status line.
- **Compare the full version**: only for the pack's "different build" notice (10 §5), and only when both sides have build numbers; when either has only `major.minor`, say "build not recorded" and do not count it as different.
- **Display the full version**: the launch page's game area (16 §2.3), the environment page and settings "this game", `archo env list/show`, pack pages and `pack list`. Narrow places (chips, the TUI status line) show only `Short()`.
- **Short values from outside**: RimArcho's own database stores whole lines (`environment.game_version` is reread on each scan; plan versions record the ModsConfig.xml version, which is the whole line). Only outside values may be short: lists from other tools, a save's `gameVersion`, a handwritten `pack.json`. They are kept unchanged and treated as "unknown build" under the rules above.

Environment commands (full syntax in 13 §4):
- `archo env list` (environments and shared Mod folders), `env detect` (detection suggestions only), `env add <name> --detected` or `env add <name> --game <directory> [--saves shared|own | --user-data <directory>] [--launch direct|steam]`. When the game is in a Steam library and no Workshop directory exists, `env add` also adds that library's Workshop directory as a Mod folder (§3.2 item 3).
- `archo env set <name> --game/--saves/--user-data/--launch/--launch-args`. Changing `--game` rebinds official and local by §4.2; if the new game is in another Steam library, the Workshop directory changes too.
- Alias syntax: `env add/set` `--workshop`, `--steamcmd` and `--extra` equal `archo folder add workshop|steamcmd|extra <directory>`; `--remove-source src:<label>` equals `archo folder remove` (§4.1).
- `archo env branches`, `archo env download <branch> --to <directory> [--account <account-name-or-anonymous>]`: download a game version with SteamCMD and add an environment for it (11 §3).
- `archo env use <name>` switches the current environment; the first environment added becomes current automatically; the global `--env <name>` switches for one command only.
- `archo env remove <name> --yes`: delete this environment's own database data (`Data` / `Mods` Instances and their history, launch records, analysis, finding ignores); keep the globally shared plans, rewriting entries that point to its Instances as `packageId` (07 §1.7); keep the Instances of shared Mod folders and the player's Mod data; delete Lineages left empty, with their player data and rules. Game and Mod files are not touched. Without `--yes` it refuses (exit code 2).
- `archo env rebind src:<label> yes|no`: see §4.2.

**Data directory check**: `env add`, `env set`, `folder add` and `scan` check that RimArcho's data directory, game directories, user data directories and all source roots **do not contain one another**, and refuse when one does. Reason: RimArcho reads these directories, and writes only to the locations listed in 02 §1; putting its data directory inside them would break that boundary. The only exception is own saves, `<data-directory>/userdata/<environment-name>`, which RimArcho owns.

### 3.2 Automatic detection

Try in order, stopping at the first match in each step:
1. **Game directory**: first find the Steam roots (Windows: the registry, then `C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`; macOS: `~/Library/Application Support/Steam`; Linux: `~/.steam/steam`, `~/.steam/root`, `~/.local/share/Steam`, Flatpak `~/.var/app/com.valvesoftware.Steam/.local/share/Steam` and `~/.var/app/com.valvesoftware.Steam/data/Steam`, Snap `~/snap/steam/common/.local/share/Steam`). Off Windows, resolve symlinks and discard duplicate roots and libraries. Read each root's `steamapps/libraryfolders.vdf` (or `config/libraryfolders.vdf`; old formats such as `"1" "D:\\SteamLibrary"` are supported) and search each library for `steamapps/common/RimWorld`. A match uses steam launch. A RimWorld executable or `Data/Core` identifies the game (`env.LooksLikeGameRoot`); no `appmanifest_294100.acf` is required, including for unmanaged Proton copies in an extra library.
   If none is found, try common manual installation locations, with direct launch:
   - Windows: `C:\Games\RimWorld`, `D:\Games\RimWorld`, `C:\GOG Games\RimWorld`, `D:\GOG Games\RimWorld`, `C:\Program Files\RimWorld`, `C:\Program Files (x86)\RimWorld`
   - macOS: `/Applications/RimWorld.app`, `/Applications/RimWorld`
   - Linux: `~/GOG Games/RimWorld`, `~/Games/RimWorld`
2. **User data directory**:
   - Windows: `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`
   - macOS: `~/Library/Application Support/RimWorld`
   - Linux native (`RimWorldLinux`): `~/.config/unity3d/Ludeon Studios/RimWorld by Ludeon Studios`. For a game in a Flatpak Steam root or one of its listed libraries, use `~/.var/app/com.valvesoftware.Steam/.config/unity3d/Ludeon Studios/RimWorld by Ludeon Studios` instead.
   - Linux Proton: a game with `RimWorldWin64.exe`, no `RimWorldLinux`, and `steamapps/compatdata/294100/pfx` in the same Steam library uses `<library>/steamapps/compatdata/294100/pfx/drive_c/users/steamuser/AppData/LocalLow/Ludeon Studios/RimWorld by Ludeon Studios`. This also applies to Flatpak Steam. The native executable takes precedence when both are present. The kind is derived from the layout, without a database field; changing Steam's compatibility setting does not require a schema upgrade. Launch restrictions: 09 §2.
3. **Workshop directory**: `steamapps/workshop/content/294100` in the game's Steam library, inferred from a game path shaped like `…/steamapps/common/RimWorld`. If settings have no Workshop directory yet, search the libraries in `libraryfolders.vdf` for `workshop/content/294100`, preferring the game's library, and offer a "use it" button.

Detection results are only suggestions: `env detect` prints the first match, then every additional game from `env.DetectAll` with its own user data and library paths and an explicit `env add --game` hint. With no game it prints the locations tried. Detection writes nothing; `env add --detected` adds the first match, and `env add --game` chooses another. In the GUI first-run guide, the player may select several detected environments and add them together (16 §2.15).

`env.Detect` and `env.DetectAll` select an existing user data directory for each game separately; `env.DefaultUserDataForGame` also returns its default before it exists, for `env add` and `--saves shared`. Detection options accept a synthetic home and Steam roots for tests. Without a game, the Linux user data search tries the native location, then Flatpak.

`env.SteamSignedIn` reads the active user and live process ID from the Windows registry on Windows or `~/.steam/registry.vdf` off Windows. It resolves the account name through the Steam root's `config/loginusers.vdf`, falling back to `AutoLoginUser`; a missing or invalid active user or a dead process means signed out.

### 3.3 Player.log location

- RimWorld 1.7 and later: `<user_data_root>/Logs/Player.log`
- Earlier versions: `<user_data_root>/Player.log`
- Unknown version: check both and take the newer mtime. The previous run's log is `Player-prev.log` in the same directory.
- Version components compare as numbers (`1.10` is later than `1.9`).

Which launch (environment, plan or pack) a log belongs to comes from launch records, not paths; see 06 §4.

### 3.4 Shared user data directories

Several environments may use one user data directory; commonly a licensed game and a test game share the game's default. This is allowed: `env add` / `env set` do not refuse it. Preconditions:
- Only one game may run at a time, detected across all environments (09 §4).
- ModsConfig.xml is written only on plan launch or "save" (09 §2, 07 §4.1), with the list for this play session.
- Outside-change detection records the last hash and the writer (which plan) by **user data directory**, not by environment (07 §4.3).
- Several environments write the same Player.log in turn; launch records decide which launch it belongs to (06 §4).

Packs do not share: each uses its own `-savedatafolder` (10 §3).

## 4. Sources

### 4.1 Source kinds

| `kind` | Root directory | Ownership | Description |
| --- | --- | --- | --- |
| `official` | `<game_root>/Data` | Environment | `Core` and DLC. Always first; `Core` cannot be disabled |
| `local` | `<game_root>/Mods` | Environment | The player's local Mods |
| `workshop` | `.../workshop/content/294100` | Shared | The subdirectory name is the Workshop ID. At most one active Workshop directory |
| `steamcmd` | Chosen by the player, or the `<game_root>/steamapps/workshop/content/294100` of a game downloaded with SteamCMD (§2 item 3) | Shared | Same structure as workshop; the game does not read it directly; enabled Mods are deployed through `_Archo_` links (09 §3) |
| `extra` | Chosen by the player | Shared | Other directories, enabled like steamcmd. RimArcho's Archo Monitor directory (`src:monitor`, 19 §6) and the lock library (`src:locked`, 07 §5.1) are also extra |

- **Mod folders** (shared sources, with an empty `source.environment_id`) are managed in one place: `archo folder list|add|remove` and Settings "mod folders" (16 §2.14); MCP reads them with `status show=folders`. `folder add workshop <directory>` when a Workshop directory exists changes its path, rebinding by §4.2. Mod folders must not contain one another or any game's `Data` or `Mods`. The Monitor directory and the lock library belong to RimArcho and cannot be removed.
- Each source has a **label** for `src:<label>` references. Shared labels are globally unique (deployment link names include them, 09 §3); environment-owned sources have only official and local (plus renamed retired labels). The default label is the kind; further sources of the same kind get `steamcmd2`, `steamcmd3` and so on.
- Source states:

| State | Meaning | During a scan |
| --- | --- | --- |
| `ok` | Normal | Scanned |
| `missing` | The root directory is absent | Not scanned; its Instances are neither matched nor tombstoned. Returns to ok when the root reappears on a later scan |
| `rebind-pending` | The path changed and too few samples matched (§4.2) | Not scanned; Instances unchanged until the player answers |
| `retired` | Removed, or replaced by a new source | Not scanned; its Instances are tombstoned on the next scan |

- missing means the whole source is temporarily absent (for example an unplugged external drive): it avoids tombstoning and then "reviving" the whole source, and lets rebinding compare against live Instances. Tombstones stand only for single directories that disappeared.
- Removing a Mod folder (`folder remove`) marks it retired and relabels it `<original-label>-removed` (with a number suffix on conflict); the next scan emits disappeared and tombstones its Instances.

### 4.2 Source identity and rebinding

Every source has its own `source_id`. Instances record only `source_id + relative path`, never absolute paths, so changing a game directory only requires changing source roots, and every Instance ID is preserved.

**When to evaluate**: `env set --game` changes the game directory (both official and local; the Workshop directory also changes by §3.2 item 3 if the new game is in another Steam library), `folder add workshop` changes the Workshop directory, or paths change on the environment or settings pages (16 §2.14).

**Evaluation**:
1. List the subdirectories with `About/About.xml` under the new root, sort them by name, and take up to 20 **evenly spaced** samples (not random, so reproducible).
2. Compute each sample's semantic fingerprint (04 §1.4) and find an Instance in the old source with the same relative path (case-insensitive); a similarity ≥ 0.9 counts as a match.
3. Match rate = matches / samples. At ≥ 80%, treat it as the same source and update only the root path. If the old source has no Instances, bind directly.
4. Below the threshold, do not bind automatically. Mark the source `rebind-pending`, record the new path in `pending_root`, and ask "is this the original source?" with the match rate and examples. The player answers with `archo env rebind src:<label> yes|no`:
   - yes: adopt the new root and return to ok.
   - no: retire the old source (retired), relabel it `<label>-old`, and tombstone its Instances on the next scan; create a new source at the new root with the original label.

If the player changes the game path and deletes Mods at the same time: rebind first, then turn the unmatched items into identity questions (04 §2.4).

**Writing a move**: the environment row and its official and local sources change in one transaction (`Store.MoveRecords`), so an error leaves the environment on its old game with its sources there; shared folders named in the same `env set` are separate changes after it. Setting or moving a managed directory (16 §2.14) writes the environment row, every source inside the directory and the folder settings in one transaction too. `MoveDirectory` moves the files first; if the records cannot be written, the directory is renamed back (across drives RimArcho's copy is deleted), so the incoming links are reattached to it (09 §3). Across drives the original goes to the recycle bin only after the records point to the copy. Removing a folder (`folder remove`) clears a pending re-binding with it; adding the folder back revives it as `ok` (or `missing`) without one.

### 4.3 Links and aliases

The lifecycle of links RimArcho creates is separate from scan aliases: `managed_link` persistently records the physical `Mods` path, the name and type, the Instance or pack identity, the original target, the new location and target, the move's source and destination, the managed directory's identity, the operation ID and the phase. Inspection prefers the ledger's ownership evidence; ordinary aliases still create no Instance. All known games share one enumeration of inbound links; see 09 §3.

- When listing subdirectories, follow junctions and symbolic links like the game's `DirectoryInfo.GetDirectories`, resolving real paths (Windows `GetFinalPathNameByHandle`). On Windows, comparisons ignore case and expand 8.3 short paths. Dangling links do not enter normal Mod lists, but the separate read-only link inspection must report them, never silently leave them out; non-directory entries create no Instance.
- **Aliases**: several locations that resolve to one real directory count as one Instance. The primary location is chosen in this order; the others are recorded as alias locations:
  1. An existing Instance's location (keeps R1 matching);
  2. A location that is not a link;
  3. Source reconciliation order (workshop → steamcmd → official → local → extra);
  4. Source order within the environment;
  5. Relative path.
- A link outside all sources has only one location; its real path is recorded (`links-to` in mod output), not as an alias. RimCrow's `Mods/_Link_<Workshop-ID>` works this way and is recognized as a normal local Mod; RimArcho does not manage or delete these links.
- **`_Archo_` links**: `Mods/_Archo_<source-label>_<directory-name>`, created when deploying Mods from steamcmd or extra sources (09 §3). They resolve to the real source directories and are only aliases of those Instances: no new Instance or event.
- **Pack links**: scans skip links whose names start with `!ArchoPack_` (case-insensitive). They exist only while a pack is played (10 §3), outside the environment. Pack copies carry `.rimarcho/origin.json` but are not scanned.
- **Work folders**: scans skip any folder whose name contains `.archo-` (case-insensitive): `.archo-*` and the `<name>.archo-tmp` / `.archo-old` folders a copy or replace leaves when cut short (`source.List`).
- Links **inside** Mod directories are not followed (02 §1). For a Mod whose root is a link, the target is fingerprinted.

## 5. Acceptance

| # | Scenario | Expected | Test | Status |
| --- | --- | --- | --- | --- |
| A4 | Move the whole game to another disk and change the environment's path | Rebinding succeeds; all Instance IDs unchanged | `TestA4GameFolderMoved` | Pass |
| A16 | Two environments (such as 1.6 and 1.7) share the default user data | `env add` allows it; outside changes are judged by user data directory, so switching from A to B after A writes reports nothing false; changes are attributed to the writer plan; logs exclude the other game's files by 06 §4 | `TestV8SharedUserData`, `TestForeignLogExcludedBeforeAnalysis` | Pass |
| A24 | Too few samples match when rebinding a path; the answer is no | `rebind-pending`; no retires the old source and relabels it `<label>-old`, its Instances are tombstoned on the next scan, and the new root becomes a new source | `TestA4RebindRejected` | Pass |
| A26 | Remove a steamcmd source, or an environment | Removing local is refused; the removed steamcmd source's Instances emit disappeared; `env remove` refuses without `--yes`, otherwise deletes all of the environment's rows | `TestEnvRemoveAndSourceRemove` | Pass |
| A29 | RimArcho's data directory inside a game or source root | `env add`, `env set` and scan refuse with an explanation | — | Pending validation |
| A30 | Save modes | `--saves own` creates `<data-directory>/userdata/<environment-name>` and launches with `-savedatafolder`; `--saves shared` restores the default; `--saves` together with `--user-data` is refused | `TestEnvModelSaves` | Pass |
| A31 | Add and remove Mod folders | Adding an existing directory changes nothing; containment and a game's `Mods` are refused; only shared folders can be removed, and their Instances disappear on the next scan; a second Workshop directory means rebinding, with Instances unchanged | `TestFolderCommands` | Pass |
| A32 | Linux Steam roots, external libraries, native / Proton / Flatpak games together, manual games and symlink aliases | Each game has its own default data and Player.log path; aliases produce one suggestion | `TestLinuxDetection` | Synthetic; Linux VM pending |
