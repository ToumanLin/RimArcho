# 08 · Launch and packs

> Status: rewritten 2026-10-21 for the new launch model. Implemented: packs v1, link deployment, hidden Mods, recycle bin, writing on launch, one running game at a time, launch records with plans.
> Basis: 00, 00a, 01 §4, 02 §2.4 / §3.3 / §4.2, 03 §6.4 / §6.7, 04 §1.5 / §4 / §7, 06 §2, 10 §4. Archo Monitor added 2026-10-21 (§9).

This document covers handing plans/packs to the game: file writes, links, launch, running-game detection, hidden-Mod/recycle-bin operations that directly change Mod folders, and bundled in-game helper Archo Monitor (§9).

Every disk-writing action must offer preview (CLI `--dry-run`, command-panel "preview …"), be reversible or explain why not, and report what it did. Launch and game writes execute directly in CLI/TUI/GUI, without preview-confirmation prompts (06 §6); refusals give reasons and resolving commands. RimArcho cannot undo hiding/recycling Mod folders, so UI still confirms first (§6, §7).

## 1. Launch model

**One game at a time.** Before launch, detect game processes across all environments (§4). Refuse if any runs, identifying environment and plan/pack.
Reason: environments/plans share ModsConfig.xml and saves; simultaneous games would overwrite one another.

**Write on launch.** ModsConfig.xml projects a plan; it is not the source of truth (00a §1). Edits persist immediately in RimArcho's database (04 §1), reaching the game only when:

- Launching a plan (§2);
- Player clicks "save" (`archo save`), for launching through Steam/other paths.

**Sharing the default user data directory is fine.** Steam and SteamCMD RimWorld may both use `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`. Only one game runs and each launch writes its intended plan, so the previous writer does not affect this launch. RimArcho records last hash/writer (plan) per user data directory for outside-change detection (04 §4.3). Other consequences: 02 §2.4.

**Pack exception.** Packs use own user data (`-savedatafolder`, §5.3), leaving environment ModsConfig.xml/saves untouched.

**Launch records.** Record every plan/pack launch: environment, plan/pack, time, method, full command line, enabled-list fingerprint. Logs map to latest launch by write time (03 §6.4); save management uses records to identify plans (10 §4).
Current table: `launch(environment_id, started_at, method, command, plan_id, fingerprint, version_n)`, `method` is `steam`, `direct` or `pack:<pack name>`; `plan_id`, `fingerprint`, `version_n` added in schema v11. Plan launches record plan, enabled fingerprint (`ListFingerprint`, shared with stale detection) and played version; pack launches have no plan, using pack-list fingerprint.

## 2. Launching a plan

`archo launch [--dry-run]` launches current plan; explicit plan arguments/references: 05 §4. UI: top-bar launch, home-card "launch the game", `L` (06 §2.1, §2.3). One click executes, status reports result. Steps 1/2 refusals show reasons/resolving commands (06 §6). Command-panel "preview the launch" is read-only.

**Steps** (stop on any failure):

1. **Check.** No game running (§1, §4). Require user data directory (otherwise suggest `archo env set --user-data <dir>`). Refuse unreadable ModsConfig.xml; ask player to repair/delete it.
2. **Handle outside changes.** If file differs from last write, attribute to last writer under 04 §4.3. Load if possible and continue; refuse conflicts (unloaded outside version), offering diff/load/ignore commands (04 §4.1 item 1), avoiding silent overwrite of in-game edits.
3. **Synchronize `_Archo_` links** (§3): create missing required links, remove extras/wrong targets.
4. **Remove all `!ArchoPack_` links** (§5.3): plan play must not see pack copies. Steps 3/4 now share one remove-then-create plan.
5. **Write ModsConfig.xml** under 04 §4.1 (backup `ModsConfig.xml.rimarcho.bak`, temporary write/atomic replacement, current game `version`, `_steam` tokens). Record hash/writer afterward.
6. **Record launch** (§1). Implementation records only after successful process start; failed launches produce no record, so logs cannot map to a nonexistent launch.
7. **Record plan version, source `launch`** (04 §3), omitting duplicates of latest version.
8. **Start game process**, without waiting for exit.

Links precede ModsConfig.xml so link failure (e.g. exFAT `Mods/`) leaves the file unchanged, never a list referring to unavailable Mods.

**Two methods** (environment `launch`, `archo env set --launch direct|steam`):

| Method | Run | Arguments |
| --- | --- | --- |
| `steam` | Steam client (`steam.exe`, `steam.sh`, `steam`, found in detected Steam directory) | `-applaunch 294100`, then extra arguments |
| `direct` | First existing in game directory: `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldLinux`, `RimWorldMac.app` | Extra arguments |

- Both append `-savedatafolder=<path>` when user data differs from local default (normalized-path comparison).
- Extra arguments are entered in settings "launch" (06 §2.14), stored as `env:<environmentID>:launch_args`, split on whitespace and appended last.
- Working directory is game directory. Display arguments containing spaces/tabs in double quotes.
- Missing Steam: refuse, suggest `archo env set --launch direct`. Missing executable: refuse, naming searched directory.

**`--dry-run` shows** `_Archo_` / `!ArchoPack_` links to create/remove (`+ name` / `- name`), ModsConfig.xml path, version rewrite X → Y, entry/missing counts, full launch command. No writes/launch. `save --dry-run` shares preview without launch command.

**Prelaunch notices** (corpus G8): `archo launch` (including `--dry-run`) lists current-plan findings most associated with game errors below the command, **without blocking launch**.
- `LaunchChecks` in `internal/app/launchcheck.go` selects codes by descending corpus observed/chance (03 §4.3.1): `patch.no-match`, `def.parent-missing`, `incompat.active`, `dep.inactive`, `dep.missing`, corpus-layer `dep.observed-missing`, `combo.known-bad` (warning only). New codes need one row: code, brief reason with corpus numbers, aggregate sentence.
- One line per kind: single finding conclusion + first suggestion; several use summary ("5 patches miss in 2 Mods: #a A, #b B") and common first suggestion, otherwise `archo problems --code <code>`. Follow with separate `why:` (reason/numbers) and `fix:` lines.
- Exclude ignored/log-derived findings. Do not wait for analysis; Def/Patch uses stored analysis, adding `checks:` if never analyzed/stale.
- UI: problem strip below launch button shows most likely kind and brief reason, linking to "can it run" (06 §2.3).
- UI has another confirmation: current-plan `dep.missing` / `dep.inactive` (missing hard dependencies) prompts "missing required mods", listing each, before "launch anyway". Game will not load these Mods and may reset list to vanilla/DLC (VM scenario 9, B8). If launch already refused (outside conflict, running game, etc.), show refusal first, no prompt. CLI/TUI do not ask.

**Game write (`archo save`)** = steps 1–5 and 7 (version source per 04 §3), without launch record/process. Allowed while game runs; result says "the game reads it on its next start" (no live reread). `save --dry-run` shares preview. UI "save" at plan-page top right (06 §2.5). Primary rules: 04 §4.

**After game exit.** On detected exit (§4) or next RimArcho opening, if current Player.log is newer than latest launch and last viewed log, suggest "analyse this run's log". Launch attribution: 03 §6.4. Signal `view.Live.NewLog`: TUI status right, dismissed on opening logs. GUI shows no notice, instead reading the log once when signaled (`API.ReadNewLog`, 2026-10-30, 06 §2.1), including this run in can it run badge.

**Removed rules** (no longer applicable):

- Blocking launch on "unsaved changes", `launch --force` using saved list. Plans now have no "unsaved" launch state; launch writes the intended list. `--force` removed. (Code check/argument deletion pending.)
- Blocking mismatched ModsConfig.xml version and requiring `save`. Launch itself rewrites current version, so check is pointless. (Deletion pending.) Retain mismatch read notice, wording in 04 §4.1.
- Asking whether to save before launch.

## 3. Link deployment

**Target lifecycle (shared)**: enumerate first-level incoming links across all games, covering environments, local sources and ledger-known physical Mods, deduplicated by physical path. Refuse if foreign links would break or inspection is unknown. Preview/persist intent, detach safely, move/recycle target, update database, rebuild needed entries; at most one effective entry per instance/game. Refuse affected running games; on failure reconnect only if original target exists. Phase ledger is independent of aliases. Startup reports `mods.link-recovery` read-only; recovery requires confirmed repair. Do not clear healthy env/pack links on exit.

**Link health and repair.** `InspectGameLinks` rereads game roots of all registered environments and `Mods/` of nonretired local sources each time, deduplicating physical paths; inspect only first-level `_Archo_*`, `!ArchoPack_*`. Read junction/symlink reparse type and raw target first, then distinguish exists, definitively missing, unknown/inaccessible. Do not depend on ordinary Mod enumeration, aliases or directory-mtime cache. Unregistered games are outside coverage.

Broken links, ownership mismatch, inspection failure and real-directory name collisions produce game/path findings from 03 §4.3, shown in scan and can it run. Broken/failed inspections in selected game block plan, pack and test-run writes/launches; other games are summaries only. Retain env links with existing targets; extra valid pack links follow §5.3.

Preview repairs first; execute only after GUI/TUI explicit confirmation or CLI `links repair --yes`. Detach only first-level known-`Mods/` owned-prefix junctions/symlinks proven broken/wrong-target; show ownership evidence from known instance/pack and expected target. If an existing prefixed link's physical target (normalized, case-insensitive) matches a live instance real_path, even with deleted old ID in its name, use "target is known instance X" as ownership evidence: retain if healthy, without finding/rename. Enumerate these incoming links when moving/recycling that instance too. Report duplicate entries normally. Old prefix-only links with no known instance/pack target need player `--include <full path>` or explicit UI inclusion, never silent deletion. Never delete real files/directories, nonprefix entries, unknown tags or inaccessible items. Serialize cross-process repairs per physical game directory; refuse running games. Before execution, revalidate preview parent directory, file identity, type and raw target; refuse changes. Windows pins parent/link handles and detaches by handle; other systems pin parent, revalidate and unlink individually, never recursively or through targets. Undoing deployment removes links only, never recycles targets or uses recycle bin. Results list detached links; preserve remaining items on failure. Do not propose rebuilding unverified new targets.


Mods may live outside game-readable directories (SteamCMD downloads, personal repositories, RimArcho lock library, Archo Monitor directory: `steamcmd`/`extra` sources, 02 §3.1, 04 §7, §9), linked into `Mods/` before launch.

**Deployable.** Instance source `steamcmd`/`extra`, live, not residue/`unloadable`, not already game-visible through non-RimArcho aliases in official `Data/`, `Mods/` or Workshop (02 §3.3). Deployable Instances can be enabled directly.

**Link names.** `Mods/_Archo_<source label>_<directory name>`, replacing `/`, `\`, `:` with `_` in labels/names. Deployable sources are shared folders with globally unique labels (02 §3.1), so the same Instance uses the same link name in every game. If v14 migration renamed labels, next game write replaces old link names under rules below. `_Archo_` means RimArcho-managed, matched case-insensitively.

**Deployment timing.** Plan launch/game save (§2 step 3): create missing deployable-plan links; delete `_Archo_` links absent from plan or targeting wrong Instance. Remove before create, replacing same-name old links. `--dry-run` only lists. Plan editing never touches `Mods/`.

**Environment switching.** Fill missing shared links (Monitor etc.) and previous environment's local-Mod links from target list before scanning/resolving; details: 02 §0 item 14. Local Mods use `_Archo_env_<Instance ID>`, representing installed local Mods. Ordinary save retains healthy known-type links with existing targets; inspect/confirm repair for anomalies. Uninstall still removes them as `_Archo_` links.

**Pre-enable checks** (during editing, not deferred to writing):

- Deployable plan Instances and Instances targeted by existing `_Archo_` links register as "in `Mods/`, folder name = link name" (04 §4.2). Exclude existing link target if absent from plan and another same-package Instance is present (`deploySet`): next write deletes it, so both will not coexist. Otherwise after Monitor updates but before saving, older link sorts first by name and falsely makes new version `mod.unloadable` (2026-10-28 VM).
- Refuse enabling conflicts with same-package Mods already in `Mods/` (`pkg.duplicate-installed`); game recognizes only first registered copy.
- **Recheck on write** (added 2026-10-22): duplicate packageIds in `Mods/` cause "Tried loading mod with the same packageId multiple times", later copy ignored (`Verse.ModLister.TryAddMod`, same in 1.6/1.7; only Workshop can coexist as `_steam`). If a same-package folder appears after editing (manual copy, RimCrow `_Link_`), save/launch does not create `_Archo_` (`deployDuplicates`), instead offers **move that folder into lock library** (added 2026-10-22, below), or `archo swap` to existing copy. Existing pairs are findings (§3 end).
- **Another version blocks the way** (added 2026-10-22; shared by `_Archo_` deployment and pack launch, `app/dupmove.go`). A requested copy with same package ID but different content from an ordinary `Mods/` folder yields `DuplicatesError`: list blockers, who needs the copy (`#ref name` or "Y from pack X"), lock-library destinations, and links created after clearing space.
  - **One confirmation.** UI (save, plan launch, pack launch button, launch-page pack) confirms once, listing "`Mods/X → <lock library>/X`, then link `Mods/Y` for …". "move and go on" moves and resumes save/launch. CLI/MCP refusal gives same-command `--move-duplicates` (`archo save|launch --move-duplicates`, `archo pack launch <pack> --move-duplicates`; MCP `save_loadout` / `launch_game` / `manage_packs action=launch` with `move_duplicates: true`), or individual `archo library move-to-locked <ref>`. `--dry-run` lists "would move into the lock library first", no writes.
  - **Move** is 04 §7 lock-library move (`moveToLibrary`): same-volume rename, otherwise copy/verify/recycle original. Instance unchanged, so **its plans still use it**, linked back through `_Archo_` on next write. Reread state before save/launch; only requested copy remains in `Mods/`.
  - **Never move** links (owned or other tools' RimCrow `_Link_`), folders targeted by other tools' links (would break them), unknown folders (scan first), folders with outward links (04 §7). If any cannot move, offer no repair command; refuse with each reason.
  - Tests: `TestPackLaunchMovesOtherVersion` (pack launches after move; writing the copy's plan links exactly that version), `TestPackLaunchNeverMovesLinks`, `TestSaveMovesOtherVersion`, `TestDuplicateFixMovesToLocked`, `gui.TestPackLaunchMovesConfirm`.
- Same-package Workshop copy permits only `swap`, as with ordinary local copies; deployed copy writes unsuffixed package ID, Workshop `<package ID>_steam`.

**Link types.** Windows junctions need no admin; elsewhere symbolic links. Refuse unsupported `Mods/` volumes (FAT/exFAT), explaining without fallback copying (copies only for packs). Missing `Mods/` when deployment required: refuse, suggest `archo env set --game <dir>`.

**Deletion.** Remove link itself only (Windows `RemoveDirectory` removes junction), never enter target. Verify link type (reparse point/symlink) first. Never touch nonlink `_Archo_` directories; error asking player to rename/move. Same for real-directory collision at intended link name.

**Identity.** Scan resolves links to real paths (Windows `GetFinalPathNameByHandle`); link is an Instance alias, no new Instance/event (02 §3.3).

**Data directory protection.** Before writing `Mods/`, refuse if it and RimArcho data directory contain one another.

**No takeover of others' links.** Recognize RimCrow `_Link_<WorkshopID>` as ordinary local Mods (02 §8 A11), never manage/delete them.

**Duplicate packageIds in `Mods/`** (`pkg.duplicate-mods`, error, "breaks the game" in can it run). Inspect actual contents (`App.ModsCopies`, reads packageId from every folder/link, cached while `Mods/` unchanged), regardless of creator: ordinary folders, other tools' links, `_Archo_` / `!ArchoPack_`. One-click fixes affect only RimArcho-owned items: with `_Archo_`, use existing `Mods/` copy (same-Lineage `swap`, undoable) or remove from plan so next write removes link. Pack-only duplicates merge into `pack.links-left` (§5.3), one-click "remove the links". With movable ordinary folders (same rules as blockers), first action **move into lock library** (`M`, press again to confirm; `archo library move-to-locked <ref>`): prefer folder beside `_Archo_`, otherwise unused/game-ignored copy. Its plans retain use after moving. If nothing movable, explain manual removal from `Mods/`.

## 4. Running-game detection

**Discovery.** Windows `CreateToolhelp32Snapshot` lists processes whose executable names start `rimworld`; `QueryFullProcessImageName` gets full paths. Under environment game directory means that game. Also read:

- Start time (`GetProcessTimes`);
- Command line (`NtQueryInformationProcess` `ProcessCommandLineInformation`, only `PROCESS_QUERY_LIMITED_INFORMATION` needed since Windows 8.1), extracting `-savedatafolder=` (value or whole argument may be quoted).

Unimplemented on other systems; games always considered stopped.

**Across environments.** Prelaunch checks (§1) inspect every environment: executable under any registered game directory means running. Refusal identifies environment and plan/pack from launch records. Read process list once, compare with each root using path boundaries (`RimWorld2` is not within `RimWorld`). Refusal and `archo status` name plan (e.g. `environment main is running plan:default`) or pack. Non-RimArcho launches say `plan:X (last written to the game)`.

**What is being played.** After finding process:

- Command line available: no `-savedatafolder` or equals environment user data → environment list; equals `<pack directory>/<pack name>/userdata` → pack; another directory → record unknown user data directory as-is.
- Command line unavailable: latest launch before process start + 1 minute; if `pack:<pack name>` and within 10 minutes of process start (Steam takes time), infer pack and label "from the launch records"; otherwise environment list.

**Frequency.** GUI/TUI game status/new-log reads, CLI `archo log --follow` log and exit checks: [06 §2.16.1](06-ui/2.16-scheduled-behavior.md) G01, L01. No resident background process.

**Display.** "not running" or "running" with PID/start time.

- `archo status`: `game: running the list for 12m (pid 1234; archo log --follow)`; packs `pack <pack name>`, inferred adds `(from the launch records)`.
- GUI: running launch-page block says "the game is running" and this-run error count (disabled); top-right "last game" likewise. Errors enter can it run/badge (06 §2.1, §2.3). Since 2026-10-30 no sidebar-bottom running-game line.
- TUI: same status bar (06 §9).
- On exit stop following, perform full analysis (03 §6.6), and suggest log analysis under §2.

## 5. Packs

A pack **builds** a plan into a fixed artifact: lock each Mod to its Instance/content snapshot, retain Lineage, copy only content loaded by this game version, play with own user data. A list working today may break after Mod updates; packs preserve an artifact unaffected by updates.

### 5.1 Data

**Location.** `<pack directory>/<pack name>/`. Setting `pack.dir` (`archo config set pack.dir <path>`), default `<data directory>/packs`.

**Pack names**: letters, digits, spaces, `.`, `_`, `-`; first character letter/digit, max 64 characters, no trailing dot/space.

**Pack directory:**

| Path | Content |
| --- | --- |
| `pack.json` | Manifest below |
| `mods/<directory name>/` | Bundled Mod copies; original directory name with non-`[A-Za-z0-9._-]` replaced by `_`, duplicates suffixed `-2`, `-3` |
| `mods/<directory name>/.rimarcho/origin.json` | Origin: original Instance, Lineage, Workshop ID, snapshot hash, copy time (locked-version format, 04 §7) |
| `userdata/` | Own user data (created on first launch, §5.3) |
| `state.json` | `{"last_played": time}`, last played |

**pack.json** (`format` currently 2; still reads 1; rejects higher versions):

| Field | Meaning |
| --- | --- |
| `format` | Format version |
| `name` | Pack name |
| `created` | Creation time (UTC) |
| `game` | **Full game version** at build (whole `Version.txt` line, e.g. `1.6.4633 rev1261`, "full game version" in 02 §2.1). Read packs carrying only `1.6`, treating build as unknown |
| `env` | Source environment name |
| `light` | Manifest only, no files: RimArcho list (04 §9.3). Absent from pack-directory `pack.json` |
| `trimmed_bytes` | Total omitted bytes |
| `entries[]` | Entries in load order |

Entry fields: `package_id`, `name`, `official` (Core/DLC from game, not bundled), `lineage`, `instance`, `workshop_id`, `hash` (semantic snapshot hash, 02 §4.4), `folder` (under `mods/`), `bytes` (copy size), `trimmed_bytes`, `trimmed` (omitted version directories). Format 2 adds (2026-10-24):

| Field | Meaning |
| --- | --- |
| `files` | Copy file count (excluding `.rimarcho/`) |
| `sha256` | Copy digest: one `<relative path>\0<bytes>\0<file SHA-256 hex>\n` line per file (excluding `.rimarcho/`), `/` paths, bytewise sorted, then SHA-256 of the whole. Compute last in build (after textures); import and `pack check` verify |
| `workshop_updated` | Workshop update time (`timeupdated` from `appworkshop_294100.acf`, Unix seconds), if available |
| `git_head` | Commit if Mod directory is a git repository, if available |

Use SHA-256 rather than scan xxh3 because packs come from others and must detect deliberate tampering.

**What is locked.** Per-Mod package ID, Lineage/Instance, Workshop ID, semantic hash/copy digest (sometimes Workshop timestamp/git commit), load order (`entries`), full game version and DLC set (`official` entries are required Core/DLC; no separate field).

**Pack and list.** Packs always bundle files, extension `.archopack` (below). For list-only sharing export RimArcho list (`.archolist.json`, 04 §9.3): same `pack.json`, `light` true, file-related fields removed; import compares local Instances by packageId/semantic hash.

**.archopack file** (format 2, 2026-10-24): tar stream compressed as a whole with zstd (klauspost/compress, `SpeedBetterCompression`, 8 MB window, single-thread encoding, 07 §1). First entry `pack.json`, followed only by `mods/<directory name>/…`, each name matching a bundled entry's `folder`. Regular files/directories only, no links. Still import format 1 ZIP. Why tar+zstd rather than ZIP: ten real Workshop Mods (84.5 MB, mostly PNG), per-file ZIP deflate 42.4 MB, zstd 41.3 MB, nearly equal; whole-stream zstd (shared dictionary across similar XML) 29.5 MB, 30% smaller, similar speed. No random access; putting manifest first avoids full extraction to read it. Any `.tar.zst` tool opens it.

**Never bundle the game.** Only compiled Mod copies, never game/Core/DLC. Build skips `official`; export writes only manifest and bundled directories (no `userdata/`, `state.json` or extras). Reject entire imports with other paths (`RimWorldWin64_Data/`, `Data/Core/`, `Version.txt`), links, `official` with `folder`, or bundled `package_id` starting `ludeon.rimworld`. Comparison: an October 2026 pack advertised as "1.6 all DLC + Ratkin and four other races" (translated) was a WinRAR self-extracting exe (RAR5, 1.29 GB, 2.0 GB extracted, 44,670 files), containing a full game (`RimWorldWin64_Data/`, `Data/Core`/DLC, replaced `steam_api64`) plus 38 Workshop-ID Mod directories, no manifest/order/hashes and no pre-extraction verification. RimArcho packs only the Mod part, with every entry verifiable.

Since 2026-10-22 no "light packs": former `pack build --light` made manifest-only directories that could not launch and should not appear in packs. Players had to distinguish full/light when they only wanted versioned lists. Now export lists from current list ("RimArcho list", `archo loadout export --format archo`) or pack ("export the list", `archo pack export --list`). Old manifest-only directories are hidden/unlaunchable (never released).

**Listing.** `archo pack list`: name, Mod count, size, game version, last played. Sum recorded `bytes` without traversing copies (walk `mods/` only if absent). Environment shows packs built there plus packs whose source environment is absent locally but major/minor matches; show only count of others.

### 5.2 Building

`archo pack build <pack name> [--dry-run]`; UI "create a pack" subpage of plan (06 §2.6).

1. **Check.** Valid name, no existing pack. Refuse/list missing entries ("not installed: pkg:…") or error findings. **Sorting findings (`order.*`, `official.order`) do not block**: next step sorts. Exclude log findings. `version.unsupported` warns (Mod name/declared versions), respecting 03 §4.2 preferences, without blocking.
2. **Sort.** Equivalent to `archo sort --dry-run`, for pack only, leaving plan unchanged. Record move count.
3. **Select content (cleaning).** Keep only directories loaded by current game (03 §1: matching LoadFolders.xml entries, otherwise shared directories + highest version ≤ current). Omit unused root version directories, recording `trimmed`, e.g. 1.6 keeps `1.6/` and shared, drops `1.5/`. Exclude `.git`, `.rimarcho`; count/skip outward links without following. No Core/DLC copies.
4. **Measure.** Per-copy size, omitted bytes, pack-volume free space. Refuse/list unreadable Mod folders; require needed size + 256 MB free.
5. **Copy.** Assemble `<pack name>.archo-tmp` in pack directory, writing each `.rimarcho/origin.json`.
6. **Write pack.json**, rename temporary directory into place.

`--dry-run` stops after step 4, listing omitted versions, sizes, warnings, no writes.

**UI create button** is disabled with explanation on errors; explanation opens can it run. "see the list first, write nothing" means `--dry-run` (06 §2.6, G14).

**Long tasks.** Copying takes minutes; GUI launches child `archo pack build <pack name> [--env <environment>]` (same data directory, detached, continues after closing window). Progress in `<pack name>.archo-tmp/build.json`: step (`check`, `measure`, `copy`, `write`, `done`), copied/total Mods and bytes, current Mod, PID, start, heartbeat. Heartbeat/stale thresholds, write/report throttling, stop frequency: 06 §2.16.1 P01–P02, P04. Write `build.json.part`, rename, avoiding partial reads.

- `archo pack stop <pack name>` creates `stop` in build directory; check every half-second, then stop/delete copied content.
- Interrupted builds leave `.archo-tmp`; `archo pack list` lists them, `archo pack remove <pack name> --unfinished` deletes directly (RimArcho unfinished output, not recycle bin). Running builds require `stop` first. Rebuilding same name replaces old unfinished directory.

**Texture optimization.** Two PNG→DDS modes (corrected 2026-10-22; previously pack-only):

- **Ordinary optimization** (mod library "convert textures to DDS", library › texture optimisation, `archo textures optimize`, MCP `manage_textures`): generate same-name `.dds` beside original PNGs, retaining PNGs. Process all `Textures/` directories in this copy (Workshop shared by game versions); same-relative-path group converts all or none. Record batch "in progress" in database before writing, "complete" afterward; failed/stopped groups remove written DDS. Write/delete only own recorded DDS; skip groups with author-provided DDS. Generated files excluded from scan fingerprints. Mod update produces `texture.stale`, offering regenerate/clear. Refuse while game runs.
- **Pack slimming** (build-page texture options, `archo pack build --textures [--texture-scale N]`): add texture step after copying, converting pack copies, deleting successful PNGs, retaining PNGs for failed groups. Default no scaling, UI "pre-convert"; only scaled mode is "slim". Exclude original Mods' RimArcho DDS from pack copies. Measure final converted size, but free-space checks use peak with PNG/DDS together. `pack.json` records `textures` (tool, scale, converted/retained/failed counts, PNG/DDS bytes); copy `origin.json` records `textures`; list export excludes it.

Both use todds 0.4.1 (`-f BC1 -af BC7 -vf -fs`, mipmaps): opaque BC1, transparent BC7. Full-size conversion costs disk (DDS larger than PNG) but saves loading time and half opaque video memory (1.7 encodes BC7, 1.6 DXT5, both 8 bits/pixel; BC1 4). Offer only scales keeping both dimensions divisible by 4 and short side ≥ 128 pixels (20%–80%); never scale masks (`_m`). Download todds from GitHub only after player sees source/size and agrees (library › texture optimisation, or `archo textures tool --install`), or configure existing tool through `textures.todds` (e.g. RimSort's). Conversion runs in background archo child, stoppable via `textures.stop` in data directory. Research, game loading rules, measurements: docs/research/dds-texture-optimization.md.

### 5.3 Launching

Before moving blockers, plan/maintain all game incoming links under §3. Original confirmation lists affected links; env/locked never double-register.

`archo pack launch <pack name> [--dry-run] [--move-duplicates]`; UI launch button per pack (06 §2.7).

1. **Check.** Manifest-only directories cannot launch (suggest `archo loadout import` of `pack.json`). Refuse major/minor mismatch, missing `Mods/`, any running game (§1).
2. **No same-package pairs in `Mods/`** (changed 2026-10-22; former `!`-first approach made pack win but game errored per pair, below). For every bundled copy inspect current same-package copies (`App.ModsCopies`):
   - None: link pack copy. Workshop coexistence: pack keeps plain ID, Workshop `_steam`.
   - One player-owned folder or other-tool link (RimCrow `_Link_`), same content (semantic hash equals manifest `hash`): **do not link pack copy; use existing `Mods/` copy**, plain package ID in list; preview/results under "from Mods/". Common for packs built from RimCrow `_<WorkshopID>_` copies.
   - Different content (updated after build), or several copies: **refuse and offer lock-library move** (§3 blocker rules). One confirmation moves ordinary folders, then links pack; their plans retain use. Never move links or folders targeted by others' links; refuse, explain and suggest moving out of `Mods/` or rebuilding current list. Never rename player folders.
   - Same-package plan `_Archo_` deployment: delete owned link for this launch; plan save/launch recreates later.
   Pack list includes bundled and reused `Mods/` copies only.
3. **Links.** Link `mods/<directory>` as `Mods/!ArchoPack_<pack name>_<directory>` (sanitize special pack-name characters to `_`) using deployment code (§3 types/deletion rules). Remove other packs' links; retain correct current ones; do not create (remove existing) for reused `Mods/` copies. Refuse real-directory name collisions.
4. **First-play settings.** If `userdata/Config/` absent, copy environment `.xml` except ModsConfig.xml (graphics, keys, Mod settings); independent afterward.
5. **Write pack list** to `<pack>/userdata/Config/ModsConfig.xml`: environment game version (fallback pack), installed official content as known DLC. Own file, overwrite without backup; environment ModsConfig.xml untouched.
6. **Launch** with `-savedatafolder=<pack>/userdata`, otherwise §2 (Steam/direct, extra arguments). Own list, saves, settings.
7. **Record** `state.json` last played and launch record `pack:<pack name>`.

`--dry-run` shows link changes (`+`/`-`), pack ModsConfig.xml path/count, saves, settings to copy, command, no writes.

**Why `!ArchoPack_`, and why no longer rely on it.** Game registers `Mods/` by name; `!` comes first. Originally pack copies won against same-package local copies, which game ignored as duplicates. But every pair emitted an error (player's 2026-10-21 "Test" pack launch Player.log had 6, e.g. `_2888837973_` / `!ArchoPack____2888837973_`; `-savedatafolder` proves RimArcho launch, not Steam). Player accepts no such errors, so step 2 never creates pairs. Keep prefix to identify pack ownership.

**Scan skips pack links.** Skip `!ArchoPack_` links during enumeration; pack play creates no new Instances. Still write copy `origin.json` for future original-Lineage matching.

**`_Archo_` links.** Pack launch leaves plan-deployment `_Archo_` links alone; absent from pack list, so game sees but does not load them.

**Leaving a pack.** Plan launch/save removes all `!ArchoPack_` (§2 step 4). Keep after game exit so relaunching same pack need not recreate.

**Direct Steam launch.** Without RimArcho, leftover pack links are read as ordinary Mods (`!` first, same-package plan copy ignored as duplicate). Implemented (`app.LeftPackLinks`):
- **Detection**: current `Mods/` has `!ArchoPack_`, except when this environment's game runs that pack (§4/`WhatRuns`: command-line `-savedatafolder` matches pack userdata, fallback launch records). Also counts when stopped: next Steam launch would see links. Check directory on every findings read (processes only if links), outside finding cache.
- **Finding**: `pack.links-left` (warning, fix `archo pack unlink`), in `archo problems` / MCP `list_problems`. If running this way, say "the game is running with the pack's copies". Duplicate packageIds with `Mods/` upgrade to error, list pairs in "breaks the game" (Steam launch errors for each). Pack/Mods duplicates do not block builds (build writes only pack; launch step 2 handles).
- **UI**: one "for you to confirm" item (RimArcho launch removes links, so not "breaks the game"), one-click "remove the links", no confirmation (relaunch rebuilds). Launch-page notice shows it if no game-breaking findings, linking can it run (06 §2.3).
- **Removal**: `archo pack unlink [--dry-run]`, MCP `manage_packs action=unlink`: links only (§3), pack unchanged; refuse any running game.

**Pack Player.log (measurement pending).** Code assumes 1.7+ logs at data `Logs/Player.log`, hence `<pack>/userdata/Logs/Player.log`; earlier Unity fixed path shared with environment. Verify `-savedatafolder` behavior in game (record in 07 §6). Follow/attribution uses these locations (03 §6.4, §6.6).

### 5.4 Import, export and deletion

Before removal detach all known games' owned links to pack, not just current environment; foreign incoming links block. Dry-run lists directories and links.

- `archo pack export <pack name> <file> [--list]`: `.archopack` format 2 above, manifest and bundled Mod directories only, no userdata/state (no shared saves/settings). Verify each copy digest first (format 1 packs calculate/write digests and upgrade manifest to 2); refuse/list mismatches (modified pack, rebuild). Write `<file>.part`, then rename. `--list` writes RimArcho list (`.archolist.json`, 04 §9.3), removing `folder`, sizes, trimmed directories and texture records. Refuse existing destination, never overwrite. UI "export" (`e`), "export the list" (`E`).
- `archo pack import <file> [--as-plan]`:
  - Pack (`.archopack`, header detects zstd format 2 or ZIP format 1): read `pack.json` (64 MB cap), validate name/nonexistence; require sum `bytes` + 256 MB free; extract into `<pack name>.archo-tmp`. Reject whole pack for `..`, absolute/outside paths or entries outside "never bundle game" allowlist; per-file cap 4 GB. **Verify** every bundled directory exists, format 2 digest/count match, no unlisted `mods/` directories. Any failure deletes temporary directory and lists missing/different Mods. Rename into place only after verification. Format 1 lacks digests; report "content not verified (old format)". Then compare current game (§5.5); launch with `archo pack launch`.
  - RimArcho list: compare live local Instances by packageId/semantic hash in three groups: same version, another version, uninstalled (links). `--as-plan` loads current plan reversibly. Player obtains missing Mods, then builds. UI lists use import preview (04 §9.2), also showing "another version"; first-run "open" extracts only `.archopack`, routing others to import preview.
- `archo pack check <pack name|file>`: §5.5 comparison. Named packs also recalculate copy digests; `.archopack` streams verification without disk extraction (same allowlist/digests), for inspection before import.
- `archo pack remove <pack name> [--dry-run]`: remove its `!ArchoPack_` links, recycle pack directory (§7). Preview directory and all affected games' links, original targets, actions.
- `archo pack remove <pack name> --unfinished`, `archo pack stop <pack name>`: §5.2.

### 5.5 Comparison with this game

Even minor game upgrades can cause trouble; packs must state their build and warn on mismatch. `app.CheckPack(pack, environment)` → `PackCheck`, three parts with levels:

| Item | Test | Level |
| --- | --- | --- |
| Game | Different major/minor (`1.5` pack, `1.6` game) | error, refuse launch (§5.3 step 1) |
| | Same major/minor, different build/revision (`1.6.4633 rev1261` vs `1.6.4871 rev590`) | warning: "created for 1.6.4633 rev1261; this game is 1.6.4871 rev590"; launch allowed |
| | Either build absent | info: "build not recorded" |
| DLC | Required official entries not installed | error, list names, refuse launch (game silently drops them; dependent Mods fail) |
| | Installed DLC unused by pack | info, list (not enabled, no effect) |
| Mod | Pack: copy missing/digest differs (calculated only for named `pack check` and import) | error, list, refuse launch |
| | List (`.archolist.json`): same version / another version / uninstalled (§5.4) | uninstalled warning |

Shown in `pack import` / `pack check` results; at start of `pack launch` (including `--dry-run`), listing warnings/info and refusing errors; MCP `manage_packs check`; pack page (06 §2.7).

## 6. Hidden Mods

Before recycling hidden, locked, residual or subscribed Mod targets, execute §3's all-game incoming-link plan. Download-page replacement, SteamCMD updates and unsubscribe also detach before replacement; failure reconnects only surviving targets.

Other managers (RimCrow etc.) rename `About/About.xml` to `About.xml.disabled`, hiding copies from game. Scan creates `unloadable: about-disabled` Instances for directories with only the disabled file (02 §4.2).

**Classification**: compute per analysis, not stored; report only disabled Instances of this kind.

| Category | Condition | Finding |
| --- | --- | --- |
| Locked version | Same Lineage has another live, loadable, nonresidual, non-Workshop Instance, game-visible or deployable (RimCrow `_<WorkshopID>_`, locked copy, deployed `Mods/` copy all count); prefer enabled if several | `mod.pinned`, info |
| Unsubscribed residue | Workshop source, absent from ACF subscription list | `mod.hidden-residue`, warning |
| Individually hidden | Others | `mod.hidden`, warning |

- Locked versions **do not report** `mod.unloadable`: copy is effective, Workshop "upstream". Product rules (creation, current-plan-only swap, upstream notices): 04 §7; only hidden-folder handling here.
- **Upstream update**: Workshop snapshot differs from copy and ACF `timeupdated` is later than copy's first appearance. Upgrade `mod.pinned` to warning, action "see the changes". Compare semantic files, treating upstream About.xml.disabled as About.xml: `archo mod <copy> --diff <Workshop copy>`, output `+ only in latter`, `~ changed`, `- only in former`.
- Local measured category counts: 07 §6.

**Actions:**

| Category | Action | Command | Writes |
| --- | --- | --- | --- |
| Locked version | See differences | `archo mod <instance> --diff <instance>` | None |
| Locked version | Unlock | `archo unpin <instance> [--dry-run]` | Rename Workshop About.xml.disabled to About.xml; swap current-plan copy to Workshop if present; retain copy |
| Unsubscribed residue | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle whole folder |
| Unsubscribed residue | Resubscribe | `archo workshop subscribe <instance> [--dry-run]` | No file writes; subscribe through Steam client, Steam downloads (§10) |
| Individually hidden | Keep hidden | `archo problems ignore <p:ref>` | RimArcho database only (existing ignore mechanism, expires on Mod change, 03 §4.2) |
| Individually hidden | Restore | `archo unhide <instance> [--dry-run]` | Rename About.xml.disabled to About.xml |
| Individually hidden | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle whole folder |

**Rename rules (unhide, unpin):**

- Live about-disabled Instances only. `unpin` accepts copy/upstream reference; refuse unless hidden Workshop + loadable copy pair exists.
- Refuse if About.xml already exists (case-insensitive) or About.xml.disabled absent; no changes.
- Check data directory and folder do not contain one another before rename.
- Rescan immediately after rename.

**`trash` conditions** (also §7 allowlist): live; Workshop/SteamCMD source; about-disabled or unsubscribed residue (unsubscribe subscribed loadable Mods through Steam); real path inside but not equal to source root. Enabled/in-use plans do not block: list plans in `warning:` and proceed. Rescan after moving; Instance becomes tombstone, its plan entries missing.

**UI.** Mod inventory › all status/actions (06 §2.9), one-click actions per table, preview/confirm first. Since 2026-10-29 not on can it run; bottom line links there (06 §2.4).

## 7. Recycle bin

Before recycling targets detach all owned incoming links, reusing §3 ledger/recovery; link detachment does not use recycle bin or widen allowlist.

Deployment-link removal is §3 undeployment: safely delete link itself, without recycle-bin entry or expanding this allowlist.

No permanent deletion of player content: always use system recycle bin; refuse if unavailable.

**Allowlist.** Only four categories; reject everything else:

| Category | Conditions | Callers |
| --- | --- | --- |
| Mod folder | Instance real directory inside, not equal to source root: hidden/unsubscribed Workshop/SteamCMD copy (Steam redownloads subscribed ones); player's own `Mods/`/lock-library copy (refuse if itself a link); enabled/in-use only warns | `trash` (§6), mod library "delete" (06 §2.9), 1.2 unsubscribe |
| Pack directory | `<pack name>/` under pack directory | `pack remove` (§5.4) |
| Save file | Under environment user data `Saves/` (product rules: 10 §4) | Save management (unimplemented) |
| Player content during uninstall | Lock-library copies (default), packs, own-saves directories, `<data directory>/backups/saves/`, SteamCMD-downloaded games, only when player chooses deletion (default for locked copies). RimArcho-owned folders (data, Monitor, downloaded SteamCMD) deleted directly, not recycled | Uninstall (01 §4.1) |

Single entry point `App.ToTrash` (`internal/app/trash.go`) enforces paths regardless of caller. Caller-specific rules (enabled, plan use, subscribed) checked beforehand. Path allowlist (resolve links first, reject paths that are themselves links):
- Inside, not equal to, any source root (Workshop, SteamCMD, `Mods/`, lock library, other Mod folders, any environment); inside every `<game directory>/Mods/`;
- Direct child `<pack name>/` under pack directory (not root or folders inside packs);
- Inside any environment's user data `Saves/`, own saves `<data directory>/userdata/<environment name>`, save backups `<data directory>/backups/saves` and descendants;
- An environment game directory itself only if SteamCMD-downloaded (`steamapps/appmanifest_294100.acf` exists).

Refuse others (`ErrNotInTrashList`, `TestTrashAllowList`).

**Platforms:**

- **Windows**: `SHFileOperationW`, `FO_DELETE`, flags `FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_SILENT | FOF_NOERRORUI | FOF_WANTNUKEWARNING`. Last flag errors instead of permanently deleting on volumes without recycle bins (e.g. network drives). Nonzero return/cancellation errors include shell code.
- **macOS**: rename into `~/.Trash`, suffix collisions `.2`, `.3`.
- **Other**: freedesktop, move to `$XDG_DATA_HOME/Trash/files/` (default `~/.local/share/Trash`), write `info/<name>.trashinfo` (`Path`, `DeletionDate`); suffix collisions `.2`, `.3`.
- Cross-volume renames fail outside Windows; refuse without copy/delete fallback.

## 8. Acceptance

### Launch and link deployment

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| J1 | `launch --dry-run` (Steam, nondefault userdata) | Command has `steam.exe`, `-applaunch 294100`, `-savedatafolder=`; no launch (`TestLaunchPlans`) | Pass |
| J2 | Launch after `--launch direct` | Run `RimWorldWin64.exe`; one launch record | Pass |
| J3 | Enable SteamCMD Mod, `save --dry-run`, then `save` | Preview lists but creates no link; save creates `Mods/_Archo_…` (`TestLinkDeployment`) | Pass |
| J4 | Real `_Archo_` directory in `Mods/` | Refuse "not a link", preserve directory (`TestLinkDeploymentRefusesRealFolder`) | Pass |
| J5 | SteamCMD Mod same package as `Mods/` Mod; same as enabled Workshop | First refuses enable (`pkg.duplicate-installed`); second suggests `swap`, deployed copy writes unsuffixed ID (`TestLinkDeploymentSamePackage`) | Pass |
| J6 | Launch while another environment game runs | Refuse, name environment/plan | Pass |
| J7 | Launch directly after edits (formerly "unsaved") | Write ModsConfig.xml then launch; no `--force` needed or accepted | Pass |
| J8 | Launch with old ModsConfig.xml `version` | Write current version and launch, no block | Pass |
| J9 | Two environments share default userdata, launch their plans in sequence | Each writes intended plan; second write attributes file to second plan | Implemented (writer per userdata, `TestV8SharedUserData`); real sequential launches await release VM tests |
| J10 | Edit in game menu, exit, launch another plan in RimArcho | Attribute outside changes to last writer first (load external or conflict), then write target | Pass (loadable changes adopted then written; conflicts refuse) |
| J11 | Launch plan | Record environment/plan/time/fingerprint; new `launch` version unless same as latest | Pass |
| J12 | `archo save` | Write ModsConfig.xml/links, no process or launch record | Pass |
| J13 | After exit Player.log newer than launch | TUI suggests analyzing; GUI reads and includes run in can it run badge (`TestReadNewLogCountsTheRun`) | TUI pass; GUI changed 2026-10-30, synthetic pass, VM pending |
| J15 | `patch.no-match`, `dep.missing` etc., `launch --dry-run` | `before you play: …`, one line/kind plus `why:`/`fix:`; launch unblocked; `checks:` if never analyzed (golden `loadout`, `problems`) | Pass |
| J14 | Pack process unreadable command line (`TestWhatRuns`); quoted `-savedatafolder` (`TestSaveDataArg`) | Infer pack from records, label inferred; extract correct path | Pass |

### Packs

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| K1 | Three Mods, one with 1.5/1.6 dirs, environment 1.6 | Only 1.6/shared content; manifest omitted directories/bytes (`TestPackBuildAndLaunch`) | Pass |
| K2 | Build with non-sorting errors or missing entries | Refuse/list (`TestPackBuildBlocked`); sorting-only findings permit build | Pass |
| K3 | `pack launch --dry-run` | List `+ !ArchoPack_…`, ModsConfig path, arguments; no writes/launch | Pass |
| K4 | After `pack launch` | Environment ModsConfig unchanged; pack userdata contains list; `-savedatafolder=<pack>/userdata`; link per bundled Mod | Pass |
| K5 | Import pack list into another environment | Correct same/another/uninstalled groups (`TestPackExportImport`) | Pass |
| K6 | Scan after pack play | No new Instances (skip pack links) | Pass |
| K7 | Save/launch plan after pack play | All pack links removed | Pass |
| K8 | Export, delete, reimport pack | Round trip matches; no userdata/state export | Pass |
| K9 | `pack stop` during build; insufficient space | Stop/delete copied content; refuse with needed size (`TestPackBuildStopAndSpace`) | Pass |
| K10 | Interrupted `.archo-tmp` | Listed, removable `--unfinished`; running not deletable (`TestPackUnfinished`) | Pass |
| K11 | Build progress | Steps advance in `build.json`; interrupted after 15-second heartbeat loss (`TestPackBuildProgress`) | Pass |
| K12 | Direct Steam launch with leftover links | can it run notices, one-click removal (`TestPackLinksLeft`: stopped, running as pack, Steam launch; links only removed, running refuses) | Pass |
| K13 | Play pack on 1.7 | Log `<pack>/userdata/Logs/Player.log`, followed live | Verification pending |
| K14 | `pack launch` while game runs | Refuse | Pass |
| K15 | Same-content `Mods/` copy (RimCrow `_<ID>_`, `_Link_`), then local update | Reuse existing without duplicate link; after update refuse/list folder (`TestPackLaunchNoDuplicates`, player's 10-21 log reproduction) | Pass |
| K16 | Plan `_Archo_` same package as bundled copy | Remove link before pack launch, no duplicates (`TestPackDropsDuplicateDeploy`) | Pass |
| K18 | Different-version ordinary folder blocks pack/locked copy | Refusal gives `--move-duplicates`/`archo library move-to-locked`; confirmation moves, links intended copy; its plans later link exactly original version; never move blockers that are links (`TestPackLaunchMovesOtherVersion`, `TestPackLaunchNeverMovesLinks`, `TestSaveMovesOtherVersion`, `TestDuplicateFixMovesToLocked`, `gui.TestPackLaunchMovesConfirm`) | Pass |
| K19 | Format 2 round trip | tar+zstd, first manifest, matching digests; format 1 ZIP accepted (`TestPackArchiveRoundTrip`, `TestPackImportFormat1`) | Pass |
| K20 | Game files (`RimWorldWin64_Data/…`, `Data/Core/…`, `Version.txt`), links, bundled `ludeon.rimworld*`, unlisted directories | Reject entire pack, no temporary residue (`TestPackImportRejectsGameFiles`) | Pass |
| K21 | Modified/missing bundled file | Import refuses/names Mod; named `pack check` reports; export refuses (`TestPackImportVerifies`) | Pass |
| K22 | Pack `1.6.4633 rev1261`, game `1.6.4871 rev590`; pack only `1.6`; missing required Biotech | Build-mismatch warning, unknown-build info, missing-DLC error; only error blocks (`TestCheckPack`) | Pass |
| K17 | Leftover pack link duplicates local; two player folders; folder appears beside `_Archo_` | can it run reports `pack.links-left` error (remove links), `pkg.duplicate-mods` (guidance/use existing); writes create no duplicate deployment (`TestPackLinksDuplicateReported`, `TestPlayerDuplicatesReported`, `TestDeployNoDuplicates`, `view.TestModsDuplicateNeed`) | Pass |

### Hidden Mods and recycle bin

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| H1 | RimCrow Workshop about-disabled + enabled `Mods/_1234_` | `mod.pinned` info, no `mod.unloadable` (`TestHiddenPinned`) | Pass |
| H2 | Workshop content/ACF update after H1 | `mod.pinned` warning "upstream has an update", file diff | Pass |
| H3 | Unsubscribed about-disabled Workshop folder | `mod.hidden-residue`; trash preview lists directory; recycle then tombstone (`TestHiddenResidueTrash`) | Pass |
| H4 | Subscribed, no copy | `mod.hidden`; unhide makes loadable (`TestHiddenAlone`) | Pass |
| H5 | About.xml exists during unhide | Refuse, no writes | Pass |
| H6 | Trash local `Mods/` directory | Refuse | Pass |
| H7 | Unpin locked copy in current plan | Restore Workshop About.xml, swap current only, retain copy/other plans | Verification pending (current only, 04 §7.5) |
| H8 | Recycle entry receives nonallowlisted path | Refuse (`TestTrashAllowList`: arbitrary/game/Mods-root/Workshop-root/Core/userdata/data/pack-root/inside-pack/Mods-link) | Pass |
| H9 | Trash on no-recycle-bin volume (network drive) | Error, preserve files, no permanent deletion | Verification pending |

### Archo Monitor

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| M1 | First bundled-Monitor run | Copy `<monitor.dir>/ArchoMonitor-<version>/`, register `src:monitor`, scan one Instance (`TestMonitorInstallAndDefault`) | Pass |
| M2 | Run again | No recopy/duplicate registration | Pass |
| M3 | New bundled version | New directory/Instance, same Lineage; swap each old-version plan, one undoable edit each (`TestMonitorUpdate`) | Pass |
| M4 | Launch Monitor plan | Deployment link, immediately after Harmony in ModsConfig (`TestMonitorInstallAndDefault`; real game pending) | Pass |
| M5 | exFAT `Mods/` | No auto-add, explain; manual enable then save/launch refuses naming Monitor and `archo disable` (`TestMonitorExFAT`, simulated exFAT) | Pass |
| M6 | Build Monitor plan | Pack copy, logs under pack userdata (`TestMonitorInPack`; game verification needed) | Pass (copying) |
| M7 | Development `RIMARCHO_MONITOR`, `-dev`, changed content | Replace when stopped, defer while running (`TestMonitorDevReplace`, `TestMonitorDevPath`) | Pass |
| M8 | No bundled Monitor (`go run`, no env override) | No install/add; `archo monitor` reports searched locations (`TestMonitorNotBundled`) | Pass |
| M9 | Check finishes after launch, swaps 0.9.0-dev to 0.9.1 while game runs | Run log counts after exit; old link does not make new version unloadable; next launch updates link; later other changes still stale log (`TestMonitorUpdateAfterLaunch`; real game in release VM table) | Pass |

## 9. Archo Monitor

Bundled in-game helper (capture, log format, reading: 03 §6.7; default addition: 04 §1.5). This section follows it from release package to game.

**Bundled Monitor.** Release ZIP includes `monitor/ArchoMonitor/`: complete Mod (`About/`, `LoadFolders.xml`, assemblies in `1.5/`, `1.6/`, `1.7/`, 07 §5). Discovery order:

1. `RIMARCHO_MONITOR` (Mod folder, development/tests);
2. `monitor/ArchoMonitor/` beside executable;
3. Development builds (`buildinfo.Version` = `dev`): search upward from executable directory and working directory for `monitor/out/ArchoMonitor/` (`dotnet build` output, 07 §2).

If absent, treat as no Monitor: no install/add; `archo monitor` lists searched locations.

**Build version.** `monitor/VERSION` owns bundled Monitor version, independent of launcher. Increment for Monitor behavior/format changes; `monitor/build.ps1`, direct `dotnet build`, release script default to this value. Explicit `build.ps1 -Version` (MSBuild `MonitorVersion`) overrides; generated About.xml `modVersion` matches assembly version.

**Installation.** `<Monitor directory>/ArchoMonitor-<version>/`. Setting `monitor.dir`, default `%LOCALAPPDATA%\RimArcho\monitor`, outside data directory like lock library (01 §4). Version from About.xml `modVersion`, only `[0-9A-Za-z.+-]`. One directory/version directly under root, since sources inspect one level only (02 §3.1); not `<version>/ArchoMonitor`.

**Check timing.** GUI/TUI opening, `archo scan`, `archo monitor install`, new plan (04 §1.5):

1. Read bundled version/content fingerprint (relative path, size, SHA-256 per file).
2. Target absent: assemble `<target>.archo-tmp`, rename. Existing identical: no change. Different content: stable version (no `-`) not overwritten, explain "same version, different content"; prerelease (e.g. `0.9.0-dev`) replaced only while no game runs (stage new, rename old to `<target>.archo-old`, delete own artifact), otherwise defer.
3. Register absent Monitor source: shared `extra` (02 §0), label `monitor`, root Monitor directory; check mutual noncontainment with data/source roots first. One source for all environments (pre-0.9.0 per-environment sources merged by root in v14).
4. Scan if files copied or source registered.
5. Update old versions in existing plans (below), then default-add under 04 §1.5.

**Into the game.** Like lock library, only via `_Archo_` (§3): `Mods/_Archo_monitor_ArchoMonitor-<version>`, same deployability, prelaunch sync and deletion rules.

**Updates.** New version = new directory/Instance. All live `toumanlin.archomonitor` Instances under Monitor source count, highest About.xml semantic `modVersion` is latest. Merge differing Lineages into latest's (02 §6, `lineage merge`). Swap old versions in every environment plan in place, one reversible edit each, named `update Archo Monitor <old> → <new>`. Independent of `monitor.default` (addition only). Swap even while game runs; do not wait or repeat at launch (00 §6 D21): plan edits reach `Mods/` only next launch/save, then remove old link; until then it does not compete with new version (§3 pre-enable checks). Monitor-only updates do not stale this run's log (03 §6.4). Keep old directories (each < 100 KB); unused versions are not linked.

**FAT/exFAT.** If `Mods/` volume lacks junctions (Windows `GetVolumeInformationW` filesystem `FAT`, `FAT32`, `exFAT`), skip default-add, explaining "`Mods/` is on exFAT, which cannot hold the link Archo Monitor needs". Manual enable then save/launch refuses under §3, naming Monitor and `archo disable <ref>`. No copy fallback.

**Packs.** Copy like any Mod (§5.2, source `extra`); bundled Monitor logs `<pack>/userdata/ArchoMonitor.log` (03 §6.7.3).

**Commands.**

- `archo monitor`: bundled version/location (or searched locations), installed versions/source, current-plan presence/position, `monitor.default`.
- `archo monitor install [--dry-run]`: steps above; preview only with flag.
- `archo status` adds `monitor: ArchoMonitor 0.9.0 in the plan (after Harmony)`, `monitor: not in the plan (archo enable …)` or `monitor: not installed`.
- MCP `status` same line; `status show=settings` lists `monitor.default`; use existing `edit_loadout` for enable/disable/move.

## 10. Steam client calls (subscribe and unsubscribe)

Implemented (mod library unsubscribe/resubscribe, 06 §2.9). Like RimCrow, RimArcho subscribes/unsubscribes through the player's running Steam client:

- **Bounded child process.** RimArcho launches hidden-from-help `archo steam-worker subscribe|unsubscribe|state|download|watch <Workshop IDs…>`, working directory `%LOCALAPPDATA%\RimArcho\steam`, holding game-copied `steam_api64.dll` (local 1.7 game root; also search Unity `RimWorldWin64_Data\Plugins\x86_64`; recopy if game DLL changes) and `steam_appid.txt` containing `294100`. Never write game directory. Child exits after calls; `watch` stays during download waits. Steam shows "playing RimWorld", requiring explicit downloads rather than waiting for background updates. DLL failures cannot crash the window.
- **Calls.** No cgo; `syscall` invokes flat Steamworks C API: `SteamAPI_InitFlat` (SDK 1.58+), fallback `SteamAPI_Init`; highest available `SteamAPI_SteamUGC_v0NN`, `SteamAPI_SteamUtils_v0NN`; issue `SteamAPI_ISteamUGC_SubscribeItem` / `UnsubscribeItem` individually, loop `SteamAPI_RunCallbacks`, read `EResult` through `ISteamUtils_IsAPICallCompleted` / `GetAPICallResult` until all answered or timed out; unanswered use `GetItemState` subscribed bit. Process initialization subscription-list refresh first; afterward continue callbacks and observe confirmed-success items reverting, retry once for reverted items. Initialization/reply/compensation intervals/deadlines: 06 §2.16.1 S05. Finally `SteamAPI_Shutdown` and exit. Local 1.7.4982 DLL has `SteamAPI_Init`, `SteamUGC_v016`, `SteamUtils_v010`.
- **Output** parsed by parent (`steamworks.Parse`): `ITEM <ID> ok|failed:<EResult>|timeout <state bits>`, `STATE <ID> <state bits>`, `DONE`; unavailable Steam yields only `ERROR <reason>`. `watch` adds `PROGRESS <ID> <downloaded bytes> <total bytes> <state bits>` only on sample changes. Parent `App.runSteamStreaming` consumes lines, calls `BatchCalls.Progress` / `Done`, without waiting for exit. Up to 50 items; ordinary timeout 40 seconds; download waits below.
- **Steam unavailable** (not running/logged in, missing/unloadable DLL, timeout) is `app.ErrSteamUnavailable`: window opens item Workshop pages with reason; CLI error links pages. Steam `failed:<EResult>` reports actual error without opening pages; wording `app.WorkshopResultText`. Result 3 says "the Steam client is offline"; task details suggest restarting Steam. Common cause: SteamCMD login with same account displaces client (06 §2.9).
- **Unsubscribe lets Steam delete Workshop folders**, destructive like recycling: UI preview (`--dry-run`, affected folder), then confirm; MCP `manage_library` destructive. Enabled/in-use in any environment only warns with plan list, then proceeds; those entries become missing. Refuse running games (§4). Residue cannot unsubscribe (trash instead); subscribed cannot subscribe again.
- **Resubscribe subscribed Workshop originals.** `App.Workshop("resubscribe", refs, dryRun)` previews/confirms recycling folder then Steam redownload, losing local changes. Accept Workshop source, numeric ID, original not itself linked or locked. Allow enabled/in-use lists, refuse running game. Existing worker unsubscribes first; only confirmed-success items have entire original `…/workshop/content/294100/<Workshop ID>` recycled via existing backend/path allowlist (including extras/`.git`), never following the copy's own symlink/junction or permanently deleting. Subscribe only after `Lstat` confirms absence; concurrent Steam deletion counts as success; transient sharing failures retry briefly under 06 §2.16.1 S10. If still present/unknown after deadline, report cleanup failure and do not subscribe that item; successful others continue. Unconfirmed unsubscribe: no cleanup/subscribe. Residue skips unsubscribe/cleanup and subscribes directly. GUI reuses Steam background queue, ≤ 50 per batch; CLI/MCP foreground. Do not remove Instances, change lists or detach game links to original path in between. Scan R1 (same source/relative path, 02 §5.1) reuses ID/Lineage when folder returns, even if gap scan tombstoned it. Confirmed subscriptions (including residue) go to `watch`: `ISteamUGC::DownloadItem(id, true)` per item, pump callbacks at 06 §2.16.1 S08 intervals, read `GetItemState`, `GetItemDownloadInfo` bytes, `GetItemInstallInfo` path. Success only if directory exists/nonempty, Installed set, Downloading/DownloadPending unset; Installed alone may be old manifest, not completion proof. Per-item no-growth stall/total limits: 06 §2.16.1 S08; report "Steam did not download; please check Steam" or timeout. `BatchCalls.Stopped` cancels; parent closes input, worker exits/Shutdown; hung DLL force-kills/reaps child. Parent exit also causes pipe EOF, avoiding orphan workers. Task phases/text/JSON: 06 §2.16. CLI emits progress about every 10% and per-item completion, returning only after download success/failure.
- **Anonymous local state.** `workshop state` / MCP `steam_state` returns local folder existence if Steam unavailable, explicitly `Local` with reason. Subscription/update/download progress unknown; state bits 0 cannot mean unsubscribed. Subscribe/unsubscribe still require account; never disguise failures as success.
- **Commands**: `archo workshop unsubscribe <refs…> [--dry-run]`, `archo workshop subscribe <refs…> [--dry-run]`, `archo workshop resubscribe <refs…> [--dry-run]`, `archo workshop state <refs…>` (subscribed, installed, needs update, downloading); MCP `manage_library` actions `unsubscribe`, `subscribe`, `resubscribe`, `steam_state`. No immediate rescan: resubscribe waits for download; ordinary subscribe/unsubscribe leave Steam downloading/deleting in background, observed next scan.
- **Tests** fake children (`App.SteamWorker` ordinary, `App.SteamWorkerStream` streaming), never real Steam. Real calls only after player confirmation; development manual checks stop at preview.

## 11. Downloading game versions with SteamCMD

**Account management**: settings › accounts offers login/relogin and logout only; one-line availability defined by 06 §2.14. After account name, open native SteamCMD console `+login <name>`; player enters password/Guard and quits. Then hidden `+login <name> +licenses_print +quit` verifies cached authentication, no password passed. Confirm only if login steps and subsequent user info are OK; support separate step/result lines. Ownership requires 294100 in a license Apps/AppID list; store only boolean/check time, not license list. Anonymous downloads Workshop Mods, never owns/downloads game. Network failure does not imply expired credentials; never reuse records across tool/account changes.

Logout runs `+logout +quit`; on failure/unconfirmed/offline automatically clear known local auth cache of managed copy (01 §4), scope disclosed beforehand in confirmation. Never read tokens, back up auth files or log out other devices. External/PATH copies manage login themselves, without managed account buttons/cleanup.

Workshop fallback, game downloads and account operations share actual instance/account resolution and process-lifetime exclusion. Public branches always anonymous; reading branches does not establish ownership. SteamCMD controls its console output; RimArcho adds no prompts or untranslated window titles. External progress/errors include only permitted phases/counts/reasons, never raw auth/license logs.

Implemented (`archo env branches`, `archo env download`, first-run step 1 "download a version", 06 §2.15). Player owns RimWorld and wants an older version (e.g. 1.5) as another environment.

- **SteamCMD location.** Search `steamcmd.path`, `%LOCALAPPDATA%\RimArcho\steamcmd\steamcmd.exe`, `PATH`. If absent, ask showing source/size (HEAD `https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip`, `Content-Length`); after consent (CLI `--install-steamcmd`) download/extract into `%LOCALAPPDATA%\RimArcho\steamcmd`. First run self-updates (about 30 MB).
- **Branches.** `steamcmd +login anonymous +app_info_update 1 +app_info_print 294100 +quit`: anonymous reads app info, no ownership required. Read `depots/branches`, omit `pwdrequired`, sort `public`, `unstable`, newest-first `version-*`, others (`alpha*`, `beta*`). Never guess names. Measured 2026-10-22 (`testdata/steamcmd/app_info_print_294100.txt`): `public`, `unstable` (Public testing), `version-1.6.4633`, `version-1.5`, `version-1.4.3901`, `version-1.4.3563`, `version-1.4.3542`, `version-1.3-latest`, `version-1.3.3287`, `version-1.2.3005`, `version-1.2`, `version-1.1`, `version-1.0`, `beta19`, `beta18`, `alpha17`…`alpha13`, `alpha4`. No `version-1.6`; determine public version only from downloaded Version.txt.
- **Branch cache.** In-process by appID/tool identity (normalized real path, size, mtime); validity/retry gates: 06 §2.16.1 C02 (`internal/app/gamebranches.go`); coalesce same-key concurrent queries. Show stale list immediately, anonymous refresh in background, update UI on completion; retain old/error on failure, retry after short backoff. Query holds `accounts.Lock`, sharing instance exclusion with downloads/accounts. Reread identity after success for SteamCMD self-updates; cancel queries on closing window. Lightweight refresh notice; polling backoff/deadline: 06 §2.16.1 G05; reentry can reread. Download-log polling/descendant waits: S01, S12.
- **Destination.** Neither inside nor containing RimArcho data; no mutual containment with any source (any game's Data/Mods, Mod folder); not an existing environment game directory. Create if absent.
- **Login.** Default settings account; anonymous can download only Workshop Mods, game download asks to log in with ownership. `steamcmd.account` empty/`anonymous` or account name. Form/CLI `--account` overrides only this run: explicit → settings → anonymous; explicit anonymous overrides saved account, clearing setting restores anonymous. Public branches always anonymous. Store account name only; never receive passwords/tokens. Player enters them in SteamCMD's **own console** (`CREATE_NEW_CONSOLE`) or CLI current terminal.
- **Download.** `+force_install_dir <directory> +login <account> +app_update 294100 [-beta <branch>] validate +quit` (no `-beta` for `public`). Read no console; follow tool's `<steamcmd>/logs/console_log.txt` from prelaunch length (restart from beginning if truncated). `Update state (0x..) downloading|verifying update, progress: P (done / total)` becomes progress; `Success! App '294100' fully installed.` means success. Verify game directory, register as existing game (version name e.g. `1.5`, direct launch, shared saves default, CLI `--saves own`), scan only Data/Mods.
- **Plain-language failures.** Explain login/Guard/network/license/disk/branch through allowed `DownloadError.Why`; unknown outcomes say completion unconfirmed. Never attach raw SteamCMD text externally. Final `fully installed` overrides earlier errors: SteamCMD reconnects (`steamcmd has been disconnected from steam with result 3 (No Connection)` then `Retrying...`). A 2026-10-28 VM 16 GB download disconnected but completed; previously falsely reported unavailable/no environment (`TestSteamDownloadRecoversFromDisconnect`).
- **Unverified** (no real development login/download; 2026-10-28 VM cached login downloaded unstable, 16.4 GB, 37 minutes, validating progress format/disconnect text): real login window/prompts, failure text (only `No subscription` and login `FAILED (...)` formats certain), nonexistent branch behavior (error or public fallback), completed directory content. Fake SteamCMD (`App.SteamCMDStart` / `SteamCMDRun`) writes test log lines; development browser fake `steamcmd.exe` verified progress/environment addition.

## 12. Test runs

Before isolated data/process setup, perform §3 read-only link inspection. Broken/failed inspected game links return structured `status: links-blocked`, paths/targets/reasons/evidence, without test data, Mod changes or launch. Low-level `internal/gametest.Run` also prechecks read-only.

Implemented (2026-10-24, `internal/gametest`, `App.TestRun`, `archo test run`, MCP `test_run`; research/measurements: `docs/research/2026-10-24_game-test-runs-rimdevtools.md`). Actually launch a plan in an environment, check main-menu/test-map arrival and errors, terminate, report. Do not touch player saves/config/logs or write game directory.

- **Isolated user data.** `<data directory>/tests/<environment ID>/userdata`, passed through `-savedatafolder`, reused (1.7 shader/texture caches greatly speed reuse). Before each run write `Config/ModsConfig.xml` (`loadout.Render`, known DLC as §5.3), copy other environment `Config/` `*.xml` to retain Mod settings except `ModsConfig.xml`, `Knowledge.xml`, `Prefs.xml`, `KeyPrefs.xml`; delete `Config/Knowledge.xml`. Remove old isolated Prefs/KeyPrefs each run; target game generates defaults (language/window/keys), avoiding cross-version incompatibility; retain caches. Directory cannot be inside game (`gametest` checks).
- **Logs.** Per-run `<data directory>/tests/<environment ID>/runs/<UTC time>/` (`20060102-150405`; advance one second on collision): `Player.log`, merged stdout/stderr `stdout.log`, process `result.json`, attributed `report.json`. Keep latest 10 runs/environment, retain `userdata` cache. Unity ≤1.6 gets `-logfile <run directory>/Player.log`; ≥1.7 (`Waylith.dll` in game root) hardcodes file logs to player LocalLow, so pass `-NoFileLog`, capture stdout/stderr, stream console `HH:MM:SS  LEVEL message` into file-format Player.log for ordinary `internal/logs` parsing. Launch detached (`DETACHED_PROCESS`), so terminal Ctrl+C reaches RimArcho only, which kills game. Known limit: 1.7 console uses system ANSI code page; unrepresentable characters (Chinese Mod names on English systems) become `?`, but package/type names and attribution unaffected (measured 2026-10-24).
- **Launch.** Always direct executable (Steam cannot supply process handle), game working directory; Unity also `-popupwindow`. Refuse running games (`checkNoGame`).
- **Decision.** Main menu: `MainMenuDrawer.Init()` writes `Config/Knowledge.xml`; its postlaunch appearance marks arrival, then `--settle` waits for late errors. Map (`--map`): add `quicktest`; after `Initializing new game with mods`, quiet-log threshold marks arrival. Sampling/settle/default limits: 06 §2.16.1 V01–V04. Early game exit `exited`, `--timeout` deadline `timeout`. Kill process at end.
- **Game-invisible Mods.** Only Data/Mods (including existing links)/environment Workshop are visible. Test runs create no links; list invisible Mods in report `missing`, launch anyway.
- **Report.** Status (`menu`/`map`/`timeout`/`exited`), loading seconds (start to marker), total seconds, peak process `PeakWorkingSetSize`, requested Mod count/`missing`, log clusters (`App.AnalyzeLog`, attribution against tested plan), category counts. Human text, `--json` for MCP/scripts. Exit 0 if target reached without error clusters; otherwise 1.

PrefsData / KeyPrefs XML errors are `settings` (environment configuration), without suspected Mods; still count as errors and failing exit status.

### Acceptance

- `go test ./internal/gametest/`: fake processes cover menu/map/timeout/early exit; console-log conversion remains recognizable as Waylith by `logs.Parse`.
- Real machine: run small-plan `archo test run` on 1.6/1.7; player LocalLow `Logs/` / `Config/` mtimes and game directory unchanged, report `menu`.
