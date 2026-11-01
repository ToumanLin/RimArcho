# 08 · Launch and packs

> Status: this specification was rewritten on 2026-10-21 for the new launch model. Packs v1, link deployment, hidden Mods, the recycle bin, writing on launch, the limit of one running game at a time, and launch records that identify the plan are implemented.
> Basis: 00, 00a, 01 §4, 02 §2.4 / §3.3 / §4.2, 03 §6.4 / §6.7, 04 §1.5 / §4 / §7, 06 §2, 10 §4. Archo Monitor was added on 2026-10-21 (§9).

This document describes how RimArcho hands a plan or pack to the game: which files it writes, which links it creates, how it launches the game, and how it detects a running game. It also covers hidden Mods and the recycle bin, whose operations directly change Mod folders, and Archo Monitor, the in-game helper Mod bundled with RimArcho (§9).

Every action that writes to disk must offer a preview (CLI `--dry-run`, or "preview …" in the UI command panel), be reversible or explain why it cannot be reversed, and report what it did. Launching and writing to the game execute in one step in the CLI, TUI and GUI, without a preview-confirmation prompt (06 §6). A refusal gives the reason and commands that can resolve it. RimArcho cannot undo hiding Mod folders or sending them to the recycle bin, so the UI still asks for confirmation first (§6, §7).

## 1. Launch model

**One game at a time.** Before launching, RimArcho checks for game processes across all environments (§4). If any game is running, it refuses the launch and identifies the environment and the plan or pack being played.
The reason is that environments and plans share ModsConfig.xml and the save directory. Two games running at once would overwrite one another's files.

**Write on launch.** ModsConfig.xml is a projection of a plan, not the source of truth (00a §1). Plan edits are saved immediately in RimArcho's database (04 §1). They reach the game only when:

- The player launches a plan (§2).
- The player clicks "save" (`archo save`), for use when launching through Steam or another route.

**Sharing the default user data directory is fine.** For example, the Steam and SteamCMD installations of RimWorld may both use `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`. Only one game runs at a time, and each launch writes the plan intended for that run, so the previous writer does not affect the next launch. RimArcho records the last written hash and the writer (the plan) for each user data directory to detect outside changes (04 §4.3). See 02 §2.4 for the other consequences of sharing the directory.

**Pack exception.** A pack uses its own user data directory (`-savedatafolder`, §5.3) and leaves the environment's ModsConfig.xml and saves untouched.

**Launch records.** Every plan or pack launch records the environment, the plan or pack, the time, the launch method, the full command line and the enabled-list fingerprint. Logs are matched to the latest launch by their write time (03 §6.4). Save management uses these records to identify the plan (10 §4).
The current table is `launch(environment_id, started_at, method, command, plan_id, fingerprint, version_n)`. Its `method` is `steam`, `direct` or `pack:<pack name>`; `plan_id`, `fingerprint` and `version_n` were added in schema v11. A plan launch records the plan, its enabled-list fingerprint (`ListFingerprint`, also used for stale detection) and the version played. A pack launch has no plan and uses the fingerprint of the pack's list.

## 2. Launching a plan

`archo launch [--dry-run]` launches the current plan. See 05 §4 for arguments that specify a plan and for other reference forms. The UI entry points are the top-bar launch control, "launch the game" on the home card, and `L` (06 §2.1, §2.3). One click executes the launch, and the status line reports the result. If step 1 or 2 below refuses the launch, the status line gives the reason and commands that can resolve it (06 §6). The command panel's "preview the launch" is read-only.

**Steps** (if any step fails, stop without executing the later steps):

1. **Check.** No game may be running (§1, §4). The environment must have a user data directory; otherwise, suggest `archo env set --user-data <dir>`. If an existing ModsConfig.xml cannot be read, refuse and ask the player to repair or delete it first.
2. **Handle outside changes.** If the file differs from the last write, first attribute the change to the last plan that wrote it, under 04 §4.3. Load the change if possible, then continue. If it creates a conflict (an outside version has not been loaded), refuse and offer commands to view the diff, load the change or ignore it (04 §4.1 item 1). This prevents the write from silently overwriting edits made in the game's Mod menu.
3. **Synchronize `_Archo_` links** (§3): create any missing links that the plan needs, and remove extra links or links with the wrong targets.
4. **Remove all `!ArchoPack_` links** (§5.3): the game must not see pack copies when playing a plan. Steps 3 and 4 now use a single operation plan that removes links before creating them.
5. **Write ModsConfig.xml** under 04 §4.1: back it up as `ModsConfig.xml.rimarcho.bak`, write a temporary file and replace it atomically, write the current game `version`, and use the `_steam` tokens. Record the file's hash and writer afterward.
6. **Record the launch** (§1). The implementation records it only after the game process starts successfully. A failed launch produces no record, so logs cannot be matched to a launch that never happened.
7. **Record a plan version with the source `launch`** (04 §3). Do not record a duplicate if it is identical to the plan's latest version.
8. **Start the game process**, without waiting for it to exit.

The links are handled before ModsConfig.xml is written. If a link operation fails (for example, because `Mods/` is on exFAT), the file remains unchanged instead of referring to Mods that are unavailable.

**Two launch methods** (the environment's `launch` field, set with `archo env set --launch direct|steam`):

| Method | Run | Arguments |
| --- | --- | --- |
| `steam` | The Steam client (`steam.exe`, `steam.sh` or `steam`, found in the detected Steam directory) | `-applaunch 294100`, followed by the extra arguments |
| `direct` | The first executable found in the game directory: `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldLinux`, `RimWorldMac.app` | The extra arguments |

- Both methods append `-savedatafolder=<path>` when the user data directory differs from the local default, using normalized paths for the comparison.
- The player enters extra arguments in the "launch" group in settings (06 §2.14). They are stored as `env:<environmentID>:launch_args`, split on whitespace and appended last.
- The working directory is the game directory. Arguments containing spaces or tabs are displayed in double quotes.
- If Steam is missing, refuse and suggest `archo env set --launch direct`. If the game executable is missing, refuse and name the directory that was searched.

**`--dry-run` shows** the `_Archo_` and `!ArchoPack_` links to create or remove (`+ name` / `- name`), the ModsConfig.xml path, whether `version` will be rewritten from X to Y, the numbers of entries to write and missing entries, and the full launch command. It does not write to disk or launch the game. `save --dry-run` shows the same preview without the launch command.

**Prelaunch notices** (corpus G8): below the launch command, `archo launch` (including `--dry-run`) lists the current plan's findings that are most often associated with game errors. These notices **do not block launch**.
- The `LaunchChecks` table in `internal/app/launchcheck.go` selects the finding codes in descending order of the corpus observed vs. chance measure (03 §4.3.1): `patch.no-match`, `def.parent-missing`, `incompat.active`, `dep.inactive`, `dep.missing`, and the corpus-layer codes `dep.observed-missing` and `combo.known-bad` (warnings only). To add a code, add one row containing the code, a brief reason with the corpus numbers, and a summary sentence for multiple findings.
- Each kind gets one line. A single finding shows its conclusion and first suggestion. Multiple findings show a summary ("5 patches miss in 2 Mods: #a A, #b B") and their common first suggestion, or `archo problems --code <code>` if they have none. Separate `why:` and `fix:` lines follow; `why:` gives the reason and corpus numbers.
- Ignored findings and findings derived from logs are excluded. Launch does not wait for analysis: Def and Patch checks use the stored analysis, with an additional `checks:` line if it has never run or is stale.
- In the UI, the problem strip below the launch button shows the kind most likely to cause trouble and a brief reason. It links to "can it run" (06 §2.3).
- The UI has an additional confirmation: if the current plan has `dep.missing` or `dep.inactive` findings (missing hard dependencies), clicking launch first shows "missing required mods" and lists each missing dependency. The player must choose "launch anyway" to continue. The game will not load these Mods and may reset the list to vanilla and DLC (VM scenario 9, B8). If the launch would already be refused because of an outside conflict, a running game or another reason, show that refusal first and do not prompt. The CLI and TUI do not ask.

**Writing to the game (`archo save`)** executes steps 1–5 and 7 above, with the version source recorded under 04 §3, but creates no launch record and starts no process. Writing is allowed while the game runs; the result says "the game reads it on its next start", because the game does not reread this file while running. `save --dry-run` shows the same preview. The UI calls this "save" and places it at the top right of the plan page (06 §2.5). The main rules are in 04 §4.

**After the game exits.** When an exit is detected (§4), or when RimArcho next opens, suggest "analyse this run's log" if the current Player.log is newer than both the latest launch and the last log the player viewed. Launch attribution follows 03 §6.4. The signal is `view.Live.NewLog`: the TUI displays it on the right of the status line and dismisses it when the player opens the logs. The GUI shows no notice; it reads the log once when the signal appears (`API.ReadNewLog`, 2026-10-30, 06 §2.1), so the can it run badge includes this run.

**Removed rules** (these earlier rules no longer apply):

- A plan with "unsaved changes" blocked launch, and `launch --force` launched the saved list. Plans now have no "unsaved" launch state: launching writes the list intended for that run. `--force` has been removed. (The code check and argument still await deletion.)
- A mismatch between the major and minor version in ModsConfig.xml and the game blocked launch and required `save` first. Launch now rewrites the file with the current game version, so this check is unnecessary. (Its deletion is pending.) The notice shown when reading a mismatched version remains; see 04 §4.1 for its wording.
- The player was asked whether to save before launching.

## 3. Link deployment

**Target lifecycle (shared).** Enumerate incoming links at the first level across all games, covering environments, local sources and physical Mods directories known to the ledger. Deduplicate them by physical path. Refuse if the operation would break links belonging to other tools, or if an inspection result is unknown. First preview the operation and persist its intent, then detach the links safely, move the target or send it to the recycle bin, update the database, and rebuild the required entries. Each instance may have at most one effective entry per game. Refuse if an affected game is running. On failure, reconnect a link only if its original target exists. The phase ledger is independent of aliases. Startup reports `mods.link-recovery` without making changes; recovery requires a confirmed repair. Do not clear healthy environment or pack links on exit.

**Link health and repair.** Each call to `InspectGameLinks` rereads the game roots of all registered environments and the `Mods/` directories of nonretired local sources, deduplicating them by physical path. It inspects only first-level `_Archo_*` and `!ArchoPack_*` entries. Read the junction or symbolic link's reparse type and raw target first, then distinguish a target that exists, one that is definitively missing, and one whose status is unknown or inaccessible. Do not rely on ordinary Mod enumeration, aliases or the directory-mtime cache. Unregistered games are outside the inspection's coverage.

Broken links, ownership mismatches, inspection failures and name collisions with real directories produce the game and path findings defined in 03 §4.3. Both scanning and can it run display them. A broken link or failed inspection in the selected game blocks plan, pack and test-run operations before they write or launch; findings from other games are shown only as summaries. Always retain environment links whose targets exist. Extra valid pack links still follow §5.3.

Preview a repair first, and execute it only after explicit GUI or TUI confirmation or CLI `links repair --yes`. Detach only first-level junctions or symbolic links in a known `Mods/` directory that have an owned prefix and are proven broken or aimed at the wrong target. Show the ownership evidence from the known instance or pack and the expected target. If a prefixed link's existing physical target matches a live instance's real_path after normalization and case-insensitive comparison, use "target is known instance X" as the ownership evidence, even if the old ID in the link name has been deleted. Retain such a link if it is healthy, without reporting a finding or renaming it. Enumerate these incoming links when moving or recycling that instance too. Continue reporting duplicate entries under the existing rules. An old link with only a prefix and no target matching a known instance or pack requires the player to include it with `--include <full path>` or explicitly in the UI; never delete it silently. Never delete real files or directories, entries without an owned prefix, unknown tags or inaccessible items. Serialize repairs across processes by physical game directory, and refuse repairs for running games. Before execution, revalidate the preview's parent directory, file identity, type and raw target; refuse if any has changed. On Windows, pin the parent and link handles and detach the link by handle. On other systems, pin the parent, revalidate, and unlink each entry individually, without recursion or deletion through the target. Undoing deployment removes only the links; it never recycles targets or uses the recycle bin. The result lists the detached links. On failure, preserve the remaining items. Do not propose rebuilding a link to an unverified new target.


Mods may live in directories the game does not read: SteamCMD download directories, personal Mod repositories, the RimArcho lock library, and the Archo Monitor directory. These are `steamcmd` and `extra` sources (02 §3.1, 04 §7, §9). Before launch, directory links make them available in `Mods/`.

**Deployable.** An Instance is deployable if its source is `steamcmd` or `extra`, it is live, it is neither residue nor `unloadable`, and it is not already visible to the game through a non-RimArcho alias in official `Data/`, `Mods/` or Workshop directories (02 §3.3). A deployable Instance can be enabled directly.

**Link names.** A link is named `Mods/_Archo_<source label>_<directory name>`. Replace `/`, `\` and `:` in the label and directory name with `_`. Deployable sources are shared Mod folders with globally unique labels (02 §3.1), so the same Instance has the same link name in every game. If the v14 migration renamed a label, the next write to the game replaces the old link name under the rules below. The `_Archo_` prefix means "managed by RimArcho" and is matched case-insensitively.

**Deployment timing.** When launching a plan or writing to the game (§2 step 3), create any missing links for the plan's deployable Instances. Delete `_Archo_` links that are absent from the plan or point to the wrong recorded Instance. Remove links before creating them so that old links with the same name can be replaced. `--dry-run` only lists the changes. Editing a plan never touches `Mods/`.

**Environment switching.** Before scanning or resolving the list, fill in the target list's missing shared-Mod links (such as Monitor) and local-Mod links from the previous environment; see 02 §0 item 14. Local Mods use `_Archo_env_<Instance ID>`. These links represent installed local Mods. An ordinary save retains healthy links with known types and existing targets. Anomalies require inspection and confirmed repair. Uninstalling still removes these links under the `_Archo_` rules.

**Pre-enable checks** (perform these while editing the plan, rather than waiting for a write):

- Deployable Instances in the plan and Instances targeted by existing `_Archo_` links register as "in `Mods/`, folder name = link name" (04 §4.2). Exclude an existing link's target if it is absent from the plan and the plan contains another Instance with the same package ID (`deploySet`). The next write removes that link, so the two copies will not coexist in `Mods/`. Otherwise, after an Archo Monitor update but before the next save, the old link sorts first by folder name and falsely makes the new version `mod.unloadable` (2026-10-28 VM).
- Refuse to enable a copy that conflicts with a Mod of the same package ID already in `Mods/` (`pkg.duplicate-installed`). The game recognizes only the first registered copy.
- **Recheck on write** (added 2026-10-22). Two copies with the same packageId in `Mods/` cause "Tried loading mod with the same packageId multiple times", and the game ignores the later copy (`Verse.ModLister.TryAddMod`, the same in 1.6 and 1.7). Only a Workshop copy can coexist with a same-package copy in `Mods/`, as `_steam`. If a same-package folder appears after editing (a manual copy or a RimCrow `_Link_`), save and launch do not create the `_Archo_` link (`deployDuplicates`). Instead, they offer to **move that folder into the lock library** (added 2026-10-22, below), or the player can use `archo swap` to select the existing copy. The can it run page reports pairs that already exist (at the end of §3).
- **Another version blocks the way** (added 2026-10-22; shared by `_Archo_` deployment and pack launch, `app/dupmove.go`). If a requested copy has the same package ID but different content from an ordinary folder in `Mods/`, the refusal returns `DuplicatesError`. It lists each blocking folder, who needs the requested copy (`#ref name` or "Y from pack X"), its destination in the lock library, and the links to create after the space is cleared.
  - **One confirmation.** The UI (save, plan launch, the pack's launch button, and the pack on the launch page) asks once. It lists "`Mods/X → <lock library>/X`, then link `Mods/Y` for …". Choosing "move and go on" moves the folder and resumes the save or launch without another prompt. CLI and MCP refusals provide the same command with `--move-duplicates` (`archo save|launch --move-duplicates`, `archo pack launch <pack> --move-duplicates`; MCP `save_loadout` / `launch_game` / `manage_packs action=launch` with `move_duplicates: true`), or individual `archo library move-to-locked <ref>` commands. `--dry-run` lists "would move into the lock library first" without writing to disk.
  - **The move** is the lock-library move from 04 §7 (`moveToLibrary`): rename on the same volume; otherwise copy, verify and recycle the original. The Instance remains unchanged, so **its plans still use it**. The next write of one of those plans links it back through `_Archo_`. After moving, reread the state before saving or launching; only the requested copy remains in `Mods/`.
  - **Never move** a link, whether owned by RimArcho or another tool (such as RimCrow `_Link_`), a folder targeted by another tool's link (moving it would break the link), an unknown folder (scan first), or a folder containing outward links (04 §7). If any blocker cannot be moved, offer no repair command. Refuse and give the reason for each one.
  - The tests are `TestPackLaunchMovesOtherVersion` (the pack launches after the move, and writing the copy's plan links exactly that version), `TestPackLaunchNeverMovesLinks`, `TestSaveMovesOtherVersion`, `TestDuplicateFixMovesToLocked` and `gui.TestPackLaunchMovesConfirm`.
- A same-package Workshop copy permits only `swap`, just like an ordinary local copy. The deployed copy writes the unsuffixed package ID; the Workshop copy writes `<package ID>_steam`.

**Link types.** Windows uses directory junctions, which need no administrator privileges. Other systems use symbolic links. Refuse if the volume holding `Mods/` does not support junctions (FAT or exFAT), and explain the reason without falling back to copying; copies are only for packs. If deployment is required but the environment has no `Mods/`, refuse and suggest `archo env set --game <dir>`.

**Deletion.** Remove only the link itself (on Windows, `RemoveDirectory` removes the junction), and never enter its target. First verify that it is a link: a reparse point or symbolic link. Never touch a nonlink directory whose name begins with `_Archo_`; report an error asking the player to rename or move it. Report the same error if a real directory occupies the intended name of a new link.

**Identity.** Scanning resolves links to real paths (on Windows, using `GetFinalPathNameByHandle`). A link is only an alias of the Instance; it creates neither a new Instance nor an event (02 §3.3).

**Data directory protection.** Before writing to `Mods/`, refuse if it contains the RimArcho data directory or is contained by it.

**No takeover of others' links.** RimCrow `_Link_<WorkshopID>` links are recognized as ordinary local Mods (02 §8 A11). RimArcho never manages or deletes them.

**Duplicate packageIds in `Mods/`** (`pkg.duplicate-mods`, error, "breaks the game" in can it run). Check the actual contents of `Mods/`, regardless of who put them there: ordinary folders, other tools' links, and `_Archo_` or `!ArchoPack_` links. `App.ModsCopies` reads the packageId from each folder or link and caches the result while `Mods/` remains unchanged. One-click fixes affect only items owned by RimArcho. If a pair includes an `_Archo_` link, use the existing `Mods/` copy (an undoable `swap` when it is in the same Lineage), or remove the linked copy from the plan so that the next write removes its link. Duplicates caused only by pack links are merged into `pack.links-left` (§5.3), with a one-click "remove the links" action. If an ordinary folder can be moved under the blocker rules above, the first action is **move into lock library** (`M`, pressed again to confirm; `archo library move-to-locked <ref>`). Prefer the folder beside an `_Archo_` link; otherwise, move the copy that no plan uses and the game ignores. Its plans continue using it after the move. If none can be moved, explain that the player must manually move the extra copy out of `Mods/`.

## 4. Running-game detection

**Discovery.** On Windows, `CreateToolhelp32Snapshot` lists processes whose executable names start with `rimworld`, and `QueryFullProcessImageName` obtains their full paths. A path under an environment's game directory identifies that environment's game. Also read:

- The start time (`GetProcessTimes`).
- The command line (`NtQueryInformationProcess` with `ProcessCommandLineInformation`; since Windows 8.1, only `PROCESS_QUERY_LIMITED_INFORMATION` is needed). Extract the value of `-savedatafolder=`; either the value or the whole argument may be quoted.

Detection is not implemented on other systems, where games are always considered stopped.

**Across environments.** The prelaunch check (§1) inspects every environment. A process whose executable is under any registered game directory counts as a running game. The refusal identifies the environment and uses the launch records to name the plan or pack. Read the process list once and compare it with each registered root using path boundaries (`RimWorld2` is not within `RimWorld`). Both the refusal and `archo status` name the plan (for example, `environment main is running plan:default`) or pack. For a game launched outside RimArcho, display `plan:X (last written to the game)`.

**What is being played.** After finding a process, determine whether it is playing the environment's list or a pack:

- If the command line is available, no `-savedatafolder` argument, or a value equal to the environment's user data directory, means the environment's list. A value equal to `<pack directory>/<pack name>/userdata` means that pack. For any other directory, record it as an unknown user data directory, preserving the path.
- If the command line is unavailable, take the latest launch record before the process start time plus 1 minute. If its method is `pack:<pack name>` and it is within 10 minutes of the process start (Steam takes time to launch a game), infer that pack and label it "from the launch records". Otherwise, treat it as the environment's list.

**Frequency.** See [06 §2.16.1](06-ui/2.16-scheduled-behavior.md) G01 and L01 for GUI and TUI reads of game status and new logs, and for CLI `archo log --follow` checks of the log and game exit. There is no resident background process.

**Display.** Show "not running" or "running", with the PID and start time when running.

- `archo status` displays `game: running the list for 12m (pid 1234; archo log --follow)`. For a pack, it uses `pack <pack name>`; an inferred pack adds `(from the launch records)`.
- In the GUI, the disabled launch-page block says "the game is running" and shows the error count for this run. The top-right "last game" control does the same. Errors enter can it run and its badge (06 §2.1, §2.3). Since 2026-10-30, the sidebar no longer has a running-game line at the bottom.
- The TUI displays the same information in its status bar (06 §9).
- On exit, stop following the log, perform a full analysis (03 §6.6), and suggest log analysis under §2.

## 5. Packs

A pack **builds** a plan into a fixed artifact. Each Mod is locked to its Instance and content snapshot at build time, with its Lineage retained. The pack contains only the content that this game version loads and uses its own user data directory. A list that works today may break after its Mods update; a pack gives the player an artifact that updates cannot change.

### 5.1 Data

**Location.** A pack lives at `<pack directory>/<pack name>/`. The pack directory is set by `pack.dir` (`archo config set pack.dir <path>`) and defaults to `<data directory>/packs`.

**Pack names** may contain letters, digits, spaces, `.`, `_` and `-`. The first character must be a letter or digit. The maximum length is 64 characters, and the name must not end in a dot or space.

**A pack's directory contains:**

| Path | Content |
| --- | --- |
| `pack.json` | The manifest described below |
| `mods/<directory name>/` | A copy of each bundled Mod; the original directory name with characters outside `[A-Za-z0-9._-]` replaced by `_`, and duplicates suffixed `-2`, `-3` |
| `mods/<directory name>/.rimarcho/origin.json` | The origin: original Instance, Lineage, Workshop ID, snapshot hash and copy time (the locked-version format, 04 §7) |
| `userdata/` | The pack's own user data directory (created on first launch, §5.3) |
| `state.json` | `{"last_played": time}`, the last played time |

**pack.json** currently uses `format` 2. Format 1 is still read; higher format versions are rejected.

| Field | Meaning |
| --- | --- |
| `format` | The format version |
| `name` | The pack name |
| `created` | The creation time (UTC) |
| `game` | The **full game version** at build time (the whole `Version.txt` line, for example `1.6.4633 rev1261`; see "full game version" in 02 §2.1). Packs that carry only `1.6` are still read, with the build treated as unknown |
| `env` | The source environment's name |
| `light` | A manifest-only RimArcho list with no files (04 §9.3); absent from the `pack.json` in a pack directory |
| `trimmed_bytes` | The total number of omitted bytes |
| `entries[]` | The entries in load order |

Each entry has these fields: `package_id`, `name`, `official` (Core or DLC from the game, not bundled), `lineage`, `instance`, `workshop_id`, `hash` (the semantic snapshot hash, 02 §4.4), `folder` (the directory under `mods/`), `bytes` (the copy size), `trimmed_bytes`, and `trimmed` (the omitted version directories). Format 2 adds the following fields (2026-10-24):

| Field | Meaning |
| --- | --- |
| `files` | The copy's file count, excluding `.rimarcho/` |
| `sha256` | The copy's content digest: one `<relative path>\0<bytes>\0<file SHA-256 hex>\n` line per file, excluding `.rimarcho/`, with `/` paths sorted bytewise; SHA-256 is then computed over the whole text. Computed at the end of the build, after texture conversion, and verified by import and `pack check` |
| `workshop_updated` | The Workshop update time (`timeupdated` from `appworkshop_294100.acf`, in Unix seconds), when available |
| `git_head` | The commit, when the Mod directory is a git repository |

The digest uses SHA-256 rather than the scan's xxh3 because packs may come from other people and must detect deliberate tampering.

**What is locked.** A pack locks each Mod's package ID, Lineage and Instance, Workshop ID, semantic hash and copy digest (and sometimes its Workshop update time or git commit). It also locks the load order (the order of `entries`), the full game version, and the DLC set. The `official` entries are the required Core and DLC; there is no separate field for them.

**Pack and list.** A pack always bundles Mod files and uses the `.archopack` extension (see the format below). To share only the list, export a RimArcho list (`.archolist.json`, 04 §9.3). This uses the same `pack.json` with `light` set to true and the file-related fields removed. Import compares local Instances by packageId and semantic hash.

**.archopack file** (format 2, 2026-10-24). The file is a tar stream compressed as a whole with zstd (klauspost/compress, `SpeedBetterCompression`, an 8 MB window, single-thread encoding, 07 §1). The first entry is `pack.json`, followed only by `mods/<directory name>/…`; each directory name must match a bundled entry's `folder`. Only regular files and directories are allowed, with no links. Format 1 ZIP files can still be imported. The reason for tar and zstd rather than ZIP is the result from ten real Workshop Mods (84.5 MB, mostly PNG): per-file ZIP deflate produced 42.4 MB and zstd 41.3 MB, almost the same. Whole-stream zstd, which shares a dictionary across similar XML files, produced 29.5 MB, 30% smaller at a similar speed. The cost is the lack of random access. Putting the manifest first lets it be read without extracting the whole archive. Any tool that reads `.tar.zst` can open it.

**Never bundle the game.** A pack contains only the compiled Mod copies, never the game, Core or DLC. The build skips `official` entries. Export writes only the manifest and bundled directories, excluding `userdata/`, `state.json` and all extras. Reject the entire import if it contains other paths (such as `RimWorldWin64_Data/`, `Data/Core/` or `Version.txt`), links, an `official` entry with a `folder`, or a bundled `package_id` starting with `ludeon.rimworld`. For comparison, a pack seen in October 2026 and advertised as "1.6 all DLC + Ratkin and four other races" (translated) was a WinRAR self-extracting exe (RAR5, 1.29 GB, 2.0 GB extracted, 44,670 files). It contained a full game (`RimWorldWin64_Data/`, `Data/Core` and DLC, with a replaced `steam_api64`) plus 38 Mod directories named by Workshop ID. It had no manifest, load order or hashes, and no way to verify it before extraction. RimArcho packs only the Mod part, with every entry verifiable.

Since 2026-10-22, there have been no "light packs". The former `pack build --light` created a manifest-only directory that could not launch and did not belong in the pack list. Players had to distinguish full and light packs when they only wanted to share a list with versions. Lists can now be exported from the current list (choose "RimArcho list", `archo loadout export --format archo`) or from a pack ("export the list", `archo pack export --list`). Old manifest-only directories are hidden and cannot launch; this feature was never released.

**Listing.** `archo pack list` shows the name, Mod count, size, game version and last played time. Size is the sum of the recorded `bytes`; listing does not traverse the copies unless the record is absent, in which case it walks `mods/`. An environment lists packs built there and packs whose source environment is absent locally but whose major and minor game versions match. It shows only the count of other packs.

### 5.2 Building

`archo pack build <pack name> [--dry-run]` builds a pack. In the UI, "create a pack" is a subpage of the plan (06 §2.6).

1. **Check.** The name must be valid, and no pack with that name may exist. Refuse and list any missing entries ("not installed: pkg:…") or error findings. **Sorting findings (`order.*`, `official.order`) do not block the build**, because the next step sorts the list. Exclude findings from logs. `version.unsupported` gives a warning with the Mod name and its declared versions, respecting the preferences in 03 §4.2, but does not block the build.
2. **Sort.** This is equivalent to `archo sort --dry-run`. Use the result only for the pack, leaving the plan unchanged, and record the number of moves.
3. **Select content (cleaning).** Keep only directories loaded by the current game version under the rules in 03 §1: use the matching LoadFolders.xml entries, or otherwise the shared directories and the highest version not above the current one. Omit unused version directories at the root and record them in `trimmed`. For example, a 1.6 environment keeps `1.6/` and the shared directories but drops `1.5/`. Exclude `.git` and `.rimarcho`. Count and skip links pointing outside the Mod directory, without following them. Do not copy Core or DLC.
4. **Measure.** Measure each copy's size, the omitted bytes and the free space on the pack volume. Refuse and list any unreadable Mod folders. Require the needed size plus 256 MB of free space.
5. **Copy.** Assemble the pack in `<pack name>.archo-tmp` inside the pack directory, writing each copy's `.rimarcho/origin.json`.
6. **Write pack.json**, then rename the temporary directory into place.

`--dry-run` stops after step 4 and lists the omitted version directories, sizes and warnings without writing to disk.

**The UI create button** is disabled when there are errors, with an explanation that opens can it run. "see the list first, write nothing" means `--dry-run` (06 §2.6, G14).

**Long tasks.** Copying takes minutes, so the GUI launches a child `archo pack build <pack name> [--env <environment>]` process. It uses the same data directory, runs detached from the window, and continues after the window closes. It writes progress to `<pack name>.archo-tmp/build.json`: the step (`check`, `measure`, `copy`, `write`, `done`), copied and total Mod counts, copied and total bytes, the current Mod, PID, start time and heartbeat. See 06 §2.16.1 P01–P02 and P04 for heartbeat and stale thresholds, write and report throttling, and the stop-check frequency. Write `build.json.part` and rename it so that readers cannot see a partial file.

- `archo pack stop <pack name>` creates a `stop` file in the build directory. The build checks every half-second, then stops and deletes the copied content when it finds the file.
- An interrupted build leaves its `.archo-tmp` directory. `archo pack list` lists these directories, and `archo pack remove <pack name> --unfinished` deletes them directly. They are RimArcho's unfinished output and do not go to the recycle bin. A running build requires `stop` first. Rebuilding with the same name replaces the old unfinished directory.

**Texture optimization.** There are two PNG-to-DDS modes (corrected 2026-10-22; previously allowed only in packs):

- **Ordinary optimization** ("convert textures to DDS" in the mod library, library › texture optimisation, `archo textures optimize`, MCP `manage_textures`) generates a same-name `.dds` beside each original PNG and retains the PNG. It processes all `Textures/` directories in the copy because Workshop directories are shared across game versions. A group with the same relative path is converted entirely or leaves no generated files. Before writing, record the batch as "in progress" in the database; afterward, mark it "complete". Failed or stopped groups remove the DDS files already written. Write and delete only DDS files that RimArcho generated and recorded. Skip groups with author-provided DDS files. Generated files are excluded from scan fingerprints. A Mod update produces `texture.stale`, offering regeneration or clearing. Refuse while the game runs.
- **Pack slimming** (the build page's texture options, `archo pack build --textures [--texture-scale N]`) adds a texture step after copying. It converts the pack copies, deletes successfully converted PNGs, and retains the PNGs in failed groups. The default is no scaling, called "pre-convert" in the UI; only the scaled mode is called "slim". Exclude RimArcho-generated DDS files in the original Mods from the pack copies. Measure the final converted size, but check free space against the peak when PNG and DDS coexist. `pack.json` records `textures`: the tool, scale, converted, retained and failed counts, and PNG and DDS bytes. The copy's `origin.json` also records `textures`; list export excludes that record.

Both modes use todds 0.4.1 (`-f BC1 -af BC7 -vf -fs`, with mipmaps): BC1 for opaque textures and BC7 for transparent textures. Full-size conversion costs disk space because DDS is larger than PNG, but saves loading time and halves video memory for opaque textures. The game encodes them as BC7 in 1.7 and DXT5 in 1.6, both at 8 bits per pixel; BC1 uses 4. Offer only scales that keep both dimensions divisible by 4 and the short side at least 128 pixels (20%–80%). Never scale masks (`_m`). Download todds from GitHub only after the player sees the source and size and agrees (library › texture optimisation, or `archo textures tool --install`). Alternatively, `textures.todds` can point to an existing copy, such as RimSort's. Conversion runs in a background archo child process and can be stopped by placing `textures.stop` in the data directory. See docs/research/dds-texture-optimization.md for the research, game loading rules and measurements.

### 5.3 Launching

Before moving a blocking copy, plan and maintain the incoming links from all games under §3. The original confirmation lists the affected links. Environment and locked copies must not register twice.

`archo pack launch <pack name> [--dry-run] [--move-duplicates]` launches a pack. The UI provides a launch button for each pack (06 §2.7).

1. **Check.** A manifest-only directory cannot launch; suggest importing its `pack.json` with `archo loadout import`. Refuse if the pack's major or minor game version differs from the environment's, if `Mods/` is missing, or if any game is running (§1).
2. **Do not create same-package pairs in `Mods/`** (changed 2026-10-22). The former approach relied on `!` sorting first so that the pack copy won, but the game reported an error for every pair, as explained below. For each bundled copy, inspect the current same-package copies with `App.ModsCopies`:
   - If there is none, link the pack copy. It may coexist with a Workshop copy: the pack keeps the plain package ID, and the Workshop copy gets `_steam`.
   - If there is one player-owned folder or other-tool link (such as RimCrow `_Link_`) with the same content (its semantic hash equals the manifest's `hash`), **do not link the pack copy; use the existing `Mods/` copy**. Write its plain package ID in the list and show it under "from Mods/" in the preview and result. This is common for packs built from RimCrow `_<WorkshopID>_` copies.
   - If the content differs (it was updated after the build), or there are several copies, **refuse and offer to move them into the lock library** under the blocker rules in §3. One confirmation moves the ordinary folders and then links the pack copy; their plans continue using them. Never move a link or a folder targeted by another tool's link. Refuse, explain the reason, and suggest moving it out of `Mods/` or rebuilding the current list. Never rename the player's folders.
   - If a plan's deployed `_Archo_` link has the same package ID, delete that owned link for this launch. A later plan save or launch recreates it.
   The pack list includes only bundled copies and the reused `Mods/` copies above.
3. **Create links.** Link `mods/<directory>` as `Mods/!ArchoPack_<pack name>_<directory>`, replacing special characters in the pack name with `_`. Use the deployment code and the link-type and deletion rules from §3. Remove other packs' links, and retain this pack's existing links if their targets are correct. For a reused `Mods/` copy, create no link and remove any existing pack link. Refuse if a real directory occupies an intended link name.
4. **Copy first-play settings.** If `userdata/Config/` is absent, copy the environment's `Config/` `.xml` files except ModsConfig.xml (graphics, keys and Mod settings). The settings are independent afterward.
5. **Write the pack list** to `<pack>/userdata/Config/ModsConfig.xml`. Use the environment's game version, falling back to the pack's, and record the installed official content as the known DLC. This is the pack's own file: overwrite it without a backup, leaving the environment's ModsConfig.xml untouched.
6. **Launch** with `-savedatafolder=<pack>/userdata`. Otherwise, follow §2 for Steam or direct launch and extra arguments. The pack has its own list, saves and settings.
7. **Record** the last played time in `state.json` and a launch record with the method `pack:<pack name>`.

`--dry-run` shows the link changes (`+` / `-`), the pack's ModsConfig.xml path and entry count, the save directory, the settings to copy, and the command. It does not write to disk.

**Why the prefix is `!ArchoPack_`, and why it is no longer relied on.** The game registers `Mods/` entries by name, with `!` first. The original design let a pack copy win over a same-package local copy, which the game ignored as a duplicate. However, every pair produced an error. The Player.log from the player's 2026-10-21 "Test" pack launch had 6 such errors, including `_2888837973_` / `!ArchoPack____2888837973_`; `-savedatafolder` proves that RimArcho launched it rather than Steam. The player accepts none of these errors, so step 2 never creates such a pair. The prefix remains to identify which pack owns the link.

**Scanning skips pack links.** Enumeration skips links beginning with `!ArchoPack_`, so playing a pack creates no new Instances. The copy's `origin.json` is still written so that it can be matched to its original Lineage later if needed.

**`_Archo_` links.** Pack launch leaves the plan's deployment `_Archo_` links alone. They are absent from the pack list, so the game sees them but does not load them.

**Leaving a pack.** Launching or saving a plan removes all `!ArchoPack_` links (§2 step 4). They remain after the game exits so that relaunching the same pack does not need to recreate them.

**Direct Steam launch.** If the player launches through Steam without RimArcho, leftover pack links are read as ordinary Mods. The `!` prefix sorts first, and a same-package plan copy is ignored as a duplicate. The following behavior is implemented in `app.LeftPackLinks`:
- **Detection.** The current `Mods/` contains `!ArchoPack_` links, unless this environment's game is running that pack. This follows §4 and `WhatRuns`: the command-line `-savedatafolder` matches the pack's userdata, or launch records are used as a fallback. Stopped games also count, because the next Steam launch would see the links. Check the directory on every findings read, outside the finding cache; inspect processes only if links exist.
- **Finding.** Report `pack.links-left` (warning, with the fix `archo pack unlink`) in `archo problems` and MCP `list_problems`. If the game is running this way, say "the game is running with the pack's copies". Same-package duplicates with `Mods/` upgrade the finding to error. List each pair in "breaks the game", since a Steam launch reports an error for each one. Pack and `Mods/` duplicates do not block a build: the build writes only the pack directory, and pack launch step 2 handles them.
- **UI.** Show one "for you to confirm" item with a one-click "remove the links" action and no confirmation, since relaunching the pack rebuilds them. Launching through RimArcho removes the links anyway, so this is not a "breaks the game" item. If there are no game-breaking findings, the launch-page notice shows this message and links to can it run (06 §2.3).
- **Removal.** `archo pack unlink [--dry-run]` and MCP `manage_packs action=unlink` remove only the links under §3, leaving the pack unchanged. Refuse if any game is running.

**Pack Player.log (measurement pending).** The code assumes that 1.7 and later write logs to `Logs/Player.log` in the data directory, so a pack's log is at `<pack>/userdata/Logs/Player.log`. Earlier Unity versions use a fixed path shared with the environment. Whether the location actually follows `-savedatafolder` must be checked in the game, with the result recorded in 07 §6. Log following and attribution use these locations (03 §6.4, §6.6).

### 5.4 Import, export and deletion

Before removing a pack, detach the owned links to it from all known games, not only the current environment. Incoming links from other tools block removal. The dry-run lists both directories and links.

- `archo pack export <pack name> <file> [--list]` writes the `.archopack` format 2 described above, containing only the manifest and bundled Mod directories. It excludes userdata and state, so saves and settings are not shared. First verify each copy's digest; for a format 1 pack, calculate and write back the digests and upgrade the manifest to 2. Refuse and list mismatches, explaining that the pack was modified and must be rebuilt. Write `<file>.part`, then rename it. `--list` writes a RimArcho list (`.archolist.json`, 04 §9.3), removing `folder`, sizes, trimmed directories and texture records. Refuse if the destination exists; never overwrite it. The UI offers "export" (`e`) and "export the list" (`E`).
- `archo pack import <file> [--as-plan]` handles two kinds of input:
  - For a pack (`.archopack`, with the header identifying zstd format 2 or ZIP format 1), read `pack.json` with a 64 MB cap. Validate the name and require that no same-name pack exists. Require free space of at least the sum of `bytes` plus 256 MB, and extract into `<pack name>.archo-tmp`. Reject the whole pack if a path contains `..`, is absolute, escapes the directory, or falls outside the "never bundle the game" allowlist. Each file has a 4 GB cap. **Verify** that every bundled directory exists, that format 2 digests and file counts match, and that `mods/` contains no unlisted directories. On any failure, delete the temporary directory and list the missing or different Mods. Rename it into place only after verification succeeds. A format 1 pack has no digests; report "content not verified (old format)". Then compare it with the current game (§5.5). The player can launch it with `archo pack launch`.
  - For a RimArcho list, compare live local Instances by packageId and semantic hash, grouping them as the same version, another version, or uninstalled (with Workshop links). `--as-plan` loads the list into the current plan reversibly. The player then obtains the missing Mods and builds a pack. UI lists use import preview (04 §9.2), which also shows "another version". The first-run "open" action extracts only `.archopack` files and routes other files to import preview.
- `archo pack check <pack name|file>` performs the comparison in §5.5. For a named pack, it also recalculates the copy digests. For an `.archopack` file, it verifies the stream without extracting to disk, using the same allowlist and digests as import. This allows inspection before import.
- `archo pack remove <pack name> [--dry-run]` removes the pack's `!ArchoPack_` links, then recycles its directory (§7). The preview shows the directory and every affected game's links, original targets and actions.
- `archo pack remove <pack name> --unfinished` and `archo pack stop <pack name>` follow §5.2.

### 5.5 Comparison with this game

Even a minor game upgrade can cause trouble, so a pack must state which build it was made for and warn on a mismatch. `app.CheckPack(pack, environment)` returns `PackCheck`, with three parts and a level for each item:

| Item | Test | Level |
| --- | --- | --- |
| Game | Different major or minor versions (`1.5` pack, `1.6` game) | error; refuse launch (§5.3 step 1) |
| | Same major and minor versions, but different builds or revisions (`1.6.4633 rev1261` vs `1.6.4871 rev590`) | warning: "created for 1.6.4633 rev1261; this game is 1.6.4871 rev590"; launch allowed |
| | Either build is absent | info: "build not recorded" |
| DLC | Required official entries are not installed | error; list the names and refuse launch (the game silently drops them, and dependent Mods fail) |
| | Installed DLC is unused by the pack | info; list it (not enabled, with no effect) |
| Mod | Pack: a copy is missing or its digest differs (calculated only for named `pack check` and import) | error; list it and refuse launch |
| | List (`.archolist.json`): the same version, another version, or uninstalled (§5.4) | warning for uninstalled Mods |

The comparison appears in `pack import` and `pack check` results, in the MCP `manage_packs check` action, and on the pack page (06 §2.7). `pack launch`, including `--dry-run`, shows warnings and info at the start and refuses if there are errors.

## 6. Hidden Mods

Before recycling the target of a hidden, locked, residual or subscribed Mod, execute the incoming-link plan for all games under §3. Download-page replacement, SteamCMD updates and unsubscription also detach links before replacing the target. On failure, reconnect only targets that still exist.

Other managers, such as RimCrow, rename `About/About.xml` to `About.xml.disabled` to hide a Mod copy from the game. Scanning creates an Instance with `unloadable: about-disabled` for a directory that contains only the disabled file (02 §4.2).

**Classification** is computed during each analysis and is not stored. Findings are reported only for disabled Instances of this kind.

| Category | Condition | Finding |
| --- | --- | --- |
| Locked version | Another live, loadable, nonresidual, non-Workshop Instance in the same Lineage is visible to the game or deployable. RimCrow `_<WorkshopID>_` copies, locked copies and copies deployed in `Mods/` all count; prefer an enabled copy if several exist | `mod.pinned`, info |
| Unsubscribed residue | A Workshop source absent from the ACF subscription list | `mod.hidden-residue`, warning |
| Individually hidden | All other cases | `mod.hidden`, warning |

- Locked versions **do not report** `mod.unloadable`: the copy is effective, and the Workshop original is the "upstream". The product rules for creation, swapping only in the current plan, and upstream notices are in 04 §7. This section covers only the hidden folders.
- **Upstream update.** If the Workshop snapshot differs from the copy and the ACF `timeupdated` is later than the copy's first appearance, upgrade `mod.pinned` to warning and offer "see the changes". Compare semantic files, treating the upstream About.xml.disabled as About.xml: `archo mod <copy> --diff <Workshop copy>`. The output uses `+ only in latter`, `~ changed` and `- only in former`.
- See 07 §6 for the locally measured category counts.

**Actions:**

| Category | Action | Command | Writes |
| --- | --- | --- | --- |
| Locked version | See differences | `archo mod <instance> --diff <instance>` | None |
| Locked version | Unlock | `archo unpin <instance> [--dry-run]` | Rename the Workshop About.xml.disabled to About.xml; swap the copy to the Workshop version in the current plan if present; retain the copy |
| Unsubscribed residue | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle the whole folder |
| Unsubscribed residue | Resubscribe | `archo workshop subscribe <instance> [--dry-run]` | No file writes; subscribe through the Steam client, which downloads the Mod (§10) |
| Individually hidden | Keep hidden | `archo problems ignore <p:ref>` | Write only to the RimArcho database (the existing ignore mechanism expires on Mod changes, 03 §4.2) |
| Individually hidden | Restore | `archo unhide <instance> [--dry-run]` | Rename About.xml.disabled to About.xml |
| Individually hidden | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle the whole folder |

**Rename rules (unhide, unpin):**

- Accept only live about-disabled Instances. An `unpin` reference may identify the copy or upstream original. Refuse unless there is a pair consisting of a hidden Workshop original and a loadable copy.
- Refuse without changing files if About.xml already exists in `About/` (case-insensitive), or if About.xml.disabled is absent.
- Before renaming, check that the data directory and the folder do not contain one another.
- Rescan immediately after the rename.

**`trash` conditions** (also see the §7 allowlist). The Instance must be live, from a Workshop or SteamCMD source, and either about-disabled or unsubscribed residue. Unsubscribe subscribed, loadable Mods through Steam instead. The real path must be inside the source root and must not equal it. Enabled Mods and Mods used by plans do not block the operation: list the plans in `warning:` and proceed. Rescan after moving the folder. The Instance becomes a tombstone, and its plan entries appear as missing.

**UI.** The status and action columns in Mod inventory › all provide the one-click actions above (06 §2.9), with a preview and confirmation first. Since 2026-10-29, they are no longer on can it run; its bottom line links to the inventory instead (06 §2.4).

## 7. Recycle bin

Before recycling a target, detach all owned incoming links using the ledger phases and recovery from §3. Detaching links does not use the recycle bin or widen the allowlist.

Removing a deployment link is undeployment under §3. Safely delete only the link itself, without creating a recycle-bin entry or expanding this section's allowlist.

RimArcho does not permanently delete player content. Always move it to the system recycle bin, and refuse if the recycle bin is unavailable.

**Allowlist.** Accept only these four categories; reject all other paths:

| Category | Conditions | Callers |
| --- | --- | --- |
| Mod folder | An Instance's real directory inside, but not equal to, its source root: a hidden or unsubscribed Workshop or SteamCMD copy (Steam redownloads subscribed ones), or the player's own `Mods/` or lock-library copy (refuse if the copy itself is a link). Enabled Mods and those used by plans only warn | `trash` (§6), mod library "delete" (06 §2.9), 1.2 unsubscribe |
| Pack directory | `<pack name>/` under the pack directory | `pack remove` (§5.4) |
| Save file | A save under the environment's user data `Saves/` (product rules in 10 §4) | Save management (not implemented) |
| Player content during uninstall | Lock-library copies (selected by default), packs, own-saves directories, `<data directory>/backups/saves/` and SteamCMD-downloaded games, only when the player chooses deletion. RimArcho-owned folders (data, Monitor and downloaded SteamCMD) are deleted directly rather than recycled | Uninstall (01 §4.1) |

The single entry point `App.ToTrash` (`internal/app/trash.go`) enforces the path allowlist regardless of the caller. Caller-specific rules for enabled Mods, plan use and subscription are checked beforehand. The path allowlist is as follows; resolve links first, but reject a path that is itself a link:
- A path inside, but not equal to, any source root (Workshop, SteamCMD, `Mods/`, the lock library or other Mod folders, in any environment), or inside any `<game directory>/Mods/`.
- A direct child `<pack name>/` under the pack directory, excluding the root itself and folders inside packs.
- A path inside any environment's user data `Saves/`, an own-saves directory `<data directory>/userdata/<environment name>`, or the save backups `<data directory>/backups/saves` and its descendants.
- An environment's game directory itself, only if it was downloaded by SteamCMD (`steamapps/appmanifest_294100.acf` exists).

Refuse other paths (`ErrNotInTrashList`, `TestTrashAllowList`).

**Platforms:**

- **Windows** uses `SHFileOperationW`, `FO_DELETE`, and the flags `FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_SILENT | FOF_NOERRORUI | FOF_WANTNUKEWARNING`. The last flag causes an error on a volume without a recycle bin, such as a network drive, instead of permanent deletion. Report an error with the shell code if the return value is nonzero or the operation is cancelled.
- **macOS** renames the item into `~/.Trash`, adding `.2`, `.3` on name collisions.
- **Other systems** follow freedesktop: move the item to `$XDG_DATA_HOME/Trash/files/` (default `~/.local/share/Trash`) and write `info/<name>.trashinfo` with `Path` and `DeletionDate`. Add `.2`, `.3` on name collisions.
- A cross-volume rename fails outside Windows. Refuse without falling back to copying and deleting.

## 8. Acceptance

### Launch and link deployment

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| J1 | `launch --dry-run` with Steam and nondefault user data | The command includes `steam.exe`, `-applaunch 294100` and `-savedatafolder=`; no launch (`TestLaunchPlans`) | Pass |
| J2 | Launch after `--launch direct` | Run `RimWorldWin64.exe` and create one launch record | Pass |
| J3 | Enable a SteamCMD Mod, run `save --dry-run`, then `save` | The preview lists the link without creating it; save creates `Mods/_Archo_…` (`TestLinkDeployment`) | Pass |
| J4 | A real `_Archo_` directory exists in `Mods/` | Refuse with "not a link" and preserve the directory (`TestLinkDeploymentRefusesRealFolder`) | Pass |
| J5 | A SteamCMD Mod has the same package ID as a `Mods/` Mod, or as an enabled Workshop copy | Refuse to enable the first (`pkg.duplicate-installed`); suggest `swap` for the second, after which the deployed copy writes the unsuffixed ID (`TestLinkDeploymentSamePackage`) | Pass |
| J6 | Launch while another environment's game runs | Refuse and name the running environment and plan | Pass |
| J7 | Launch directly after edits (formerly "unsaved") | Write ModsConfig.xml, then launch; `--force` is neither needed nor accepted | Pass |
| J8 | Launch with an old ModsConfig.xml `version` | Write the current game version and launch without blocking | Pass |
| J9 | Two environments share the default user data directory; launch their plans in sequence | Each launch writes its intended plan; the second write attributes the file to the second plan | Implemented (writer per user data directory, `TestV8SharedUserData`); real sequential launches await release VM tests |
| J10 | Edit the list in the game menu, exit, then launch another plan in RimArcho | First attribute the outside changes to the last writer under 04 §4.3 (load an external version or report a conflict), then write the target plan | Pass (loadable changes are adopted before writing; conflicts refuse) |
| J11 | Launch a plan | Record the environment, plan, time and fingerprint; add a `launch` version unless it matches the latest | Pass |
| J12 | `archo save` | Write ModsConfig.xml and links without starting a process or creating a launch record | Pass |
| J13 | After exit, Player.log is newer than the launch | The TUI suggests analysis; the GUI reads the log and includes the run in the can it run badge (`TestReadNewLogCountsTheRun`) | TUI pass; GUI changed 2026-10-30, synthetic tests pass, VM pending |
| J15 | Run `launch --dry-run` with `patch.no-match`, `dep.missing` or similar findings | Show `before you play: …`, one line per kind with `why:` and `fix:`, without blocking launch; add `checks:` if never analyzed (golden `loadout`, `problems`) | Pass |
| J14 | A pack process has an unreadable command line (`TestWhatRuns`); `-savedatafolder` is quoted (`TestSaveDataArg`) | Infer the pack from launch records and label the inference; extract the correct path | Pass |

### Packs

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| K1 | Three Mods, one with 1.5 and 1.6 directories, in a 1.6 environment | Include only 1.6 and shared content; record omitted directories and bytes in the manifest (`TestPackBuildAndLaunch`) | Pass |
| K2 | Build with nonsorting errors or missing entries | Refuse and list them (`TestPackBuildBlocked`); sorting-only findings permit the build | Pass |
| K3 | `pack launch --dry-run` | List `+ !ArchoPack_…`, the ModsConfig path and arguments; no writes or launch | Pass |
| K4 | After `pack launch` | The environment's ModsConfig is unchanged; the pack's `userdata/Config/ModsConfig.xml` contains its list; the command includes `-savedatafolder=<pack>/userdata`; each bundled Mod has a link | Pass |
| K5 | Import a pack list into another environment | Correctly group the same version, another version and uninstalled Mods (`TestPackExportImport`) | Pass |
| K6 | Scan after playing a pack | Create no new Instances; skip pack links | Pass |
| K7 | Save or launch a plan after playing a pack | Remove all pack links | Pass |
| K8 | Export, delete and reimport a pack | The round trip matches; export excludes userdata and state | Pass |
| K9 | Run `pack stop` during a build; try building with insufficient space | Stop and delete copied content; refuse insufficient space with the needed size (`TestPackBuildStopAndSpace`) | Pass |
| K10 | An interrupted build leaves `.archo-tmp` | List it and allow removal with `--unfinished`; do not allow deletion of a running build (`TestPackUnfinished`) | Pass |
| K11 | Observe build progress | Steps advance in `build.json`; a 15-second heartbeat loss marks interruption (`TestPackBuildProgress`) | Pass |
| K12 | Launch directly through Steam with leftover links | can it run offers one-click removal (`TestPackLinksLeft` covers stopped, running as the pack and Steam-launched games; removal deletes only links and refuses while running) | Pass |
| K13 | Play a pack on 1.7 | Write the log to `<pack>/userdata/Logs/Player.log` and follow it live | Verification pending |
| K14 | Run `pack launch` while a game runs | Refuse | Pass |
| K15 | A same-content `Mods/` copy exists (RimCrow `_<ID>_` or `_Link_`), then the local copy updates | Reuse the existing copy without a duplicate link; after the update, refuse and list the folder (`TestPackLaunchNoDuplicates`, reproduction of the player's 10-21 log) | Pass |
| K16 | A plan's `_Archo_` link has the same package ID as a bundled copy | Remove the link before pack launch, leaving no duplicates (`TestPackDropsDuplicateDeploy`) | Pass |
| K18 | An ordinary folder of another version blocks a pack or locked copy | The refusal gives `--move-duplicates` and `archo library move-to-locked`; confirmation moves the folder and links the intended copy. Its plans later link exactly the original version; never move a blocker that is a link (`TestPackLaunchMovesOtherVersion`, `TestPackLaunchNeverMovesLinks`, `TestSaveMovesOtherVersion`, `TestDuplicateFixMovesToLocked`, `gui.TestPackLaunchMovesConfirm`) | Pass |
| K19 | Export and reimport format 2 | Use tar and zstd with the manifest first and matching digests; still accept format 1 ZIP (`TestPackArchiveRoundTrip`, `TestPackImportFormat1`) | Pass |
| K20 | A pack includes game files (`RimWorldWin64_Data/…`, `Data/Core/…`, `Version.txt`), links, bundled `ludeon.rimworld*` entries or unlisted directories | Reject the entire pack without temporary residue (`TestPackImportRejectsGameFiles`) | Pass |
| K21 | A bundled file is modified or missing | Import refuses and names the Mod; named `pack check` reports it; export refuses (`TestPackImportVerifies`) | Pass |
| K22 | The pack targets `1.6.4633 rev1261` and the game is `1.6.4871 rev590`; the pack records only `1.6`; required Biotech is missing | Report a build-mismatch warning, unknown-build info and a missing-DLC error respectively; only errors block launch (`TestCheckPack`) | Pass |
| K17 | A leftover pack link duplicates a local copy; two player folders share a package ID; a folder appears beside `_Archo_` | can it run reports `pack.links-left` as an error with link removal, or `pkg.duplicate-mods` with guidance or use of the existing copy. Writes create no duplicate deployment (`TestPackLinksDuplicateReported`, `TestPlayerDuplicatesReported`, `TestDeployNoDuplicates`, `view.TestModsDuplicateNeed`) | Pass |

### Hidden Mods and recycle bin

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| H1 | A RimCrow Workshop copy is about-disabled and `Mods/_1234_` is enabled | Report `mod.pinned` info, with no `mod.unloadable` (`TestHiddenPinned`) | Pass |
| H2 | The Workshop content and ACF time update after H1 | Report `mod.pinned` warning "upstream has an update" and show the changed files | Pass |
| H3 | An unsubscribed Workshop folder is about-disabled | Report `mod.hidden-residue`; the trash preview lists the directory; recycling and rescanning create a tombstone (`TestHiddenResidueTrash`) | Pass |
| H4 | A Workshop Mod is subscribed, with no local copy | Report `mod.hidden`; `unhide` makes it loadable (`TestHiddenAlone`) | Pass |
| H5 | About.xml already exists during unhide | Refuse without writing files | Pass |
| H6 | Trash targets a local `Mods/` directory | Refuse | Pass |
| H7 | Unpin a locked copy used in the current plan | Restore the Workshop About.xml and swap only the current plan; retain the copy and leave other plans unchanged | Verification pending (current plan only, 04 §7.5) |
| H8 | The recycle entry point receives a path outside the allowlist | Refuse (`TestTrashAllowList`: arbitrary directory, game directory, Mods root, Workshop root, Core, user data, data, pack root, a folder inside a pack, or a Mods link) | Pass |
| H9 | Trash targets a volume without a recycle bin, such as a network drive | Report an error and preserve the files without permanent deletion | Verification pending |

### Archo Monitor

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| M1 | First run with the bundled Monitor | Copy to `<monitor.dir>/ArchoMonitor-<version>/`, register `src:monitor`, and scan one Instance (`TestMonitorInstallAndDefault`) | Pass |
| M2 | Run again | Do not recopy or register a duplicate | Pass |
| M3 | A new version is bundled | Create a new directory and Instance in the same Lineage; swap every old-version plan with one undoable edit per plan (`TestMonitorUpdate`) | Pass |
| M4 | Launch a plan containing Monitor | Create the `Mods/_Archo_monitor_ArchoMonitor-<version>` deployment link and place Monitor immediately after Harmony in ModsConfig.xml (`TestMonitorInstallAndDefault`; real game pending) | Pass |
| M5 | `Mods/` is on exFAT | Skip automatic addition and explain why; after manual enable, save and launch refuse, naming Monitor and `archo disable` (`TestMonitorExFAT`, simulated exFAT) | Pass |
| M6 | Build a plan containing Monitor | Include a Monitor copy in the pack and write its log under `<pack>/userdata/` (`TestMonitorInPack`; game verification needed) | Pass (copying) |
| M7 | A development `RIMARCHO_MONITOR` build has `-dev` and changed content | Replace it when no game runs; defer while a game runs (`TestMonitorDevReplace`, `TestMonitorDevPath`) | Pass |
| M8 | No Monitor is bundled (`go run`, no environment override) | Do not install or add it; `archo monitor` reports the searched locations (`TestMonitorNotBundled`) | Pass |
| M9 | A check finishes after launch and swaps 0.9.0-dev to 0.9.1 while the game runs | Count the run's log after exit; the old link does not make the new version unloadable; the next launch updates the link; later changes of other kinds still make the log stale (`TestMonitorUpdateAfterLaunch`; real game in the release VM table) | Pass |

## 9. Archo Monitor

RimArcho bundles Archo Monitor as an in-game helper Mod. See 03 §6.7 for what it captures, its log format and how RimArcho reads it; see 04 §1.5 for default addition to plans. This section describes its path from the release package to the game.

**Bundled Monitor.** The release ZIP includes `monitor/ArchoMonitor/`, a complete Mod folder with `About/`, `LoadFolders.xml`, and assemblies for `1.5/`, `1.6/` and `1.7/` (07 §5). RimArcho searches in this order:

1. Check `RIMARCHO_MONITOR`, which points to a Mod folder for development and tests.
2. Check `monitor/ArchoMonitor/` beside the executable.
3. For development builds (`buildinfo.Version` is `dev`), search upward from the executable directory and working directory for `monitor/out/ArchoMonitor/` (the `dotnet build` output, 07 §2).

If none is found, treat Monitor as absent: do not install it or add it to a plan. `archo monitor` lists the locations searched.

**Build version.** `monitor/VERSION` is the source of the bundled Monitor version, independent of the launcher version. Increment it when Monitor behavior or its format changes. `monitor/build.ps1`, direct `dotnet build`, and the release script all use this value by default. Developers can explicitly override it with `build.ps1 -Version` (MSBuild `MonitorVersion`); the generated About.xml `modVersion` matches the assembly version.

**Installation.** Install to `<Monitor directory>/ArchoMonitor-<version>/`. The Monitor directory is set by `monitor.dir`, defaulting to `%LOCALAPPDATA%\RimArcho\monitor`. Like the lock library, it is outside the data directory (01 §4). Take the version from About.xml `modVersion`, allowing only `[0-9A-Za-z.+-]`. Each version has one Mod folder directly under the Monitor root. Sources inspect only one level below their root (02 §3.1), so do not use a two-level `<version>/ArchoMonitor` layout.

**Check timing.** Check when the GUI or TUI opens, on `archo scan` or `archo monitor install`, and when creating a plan (04 §1.5). The steps are:

1. Read the bundled Monitor's version and content fingerprint (each file's relative path, size and SHA-256).
2. If the target directory is absent, assemble `<target>.archo-tmp` and rename it into place. If it exists with the same fingerprint, leave it unchanged. If its content differs, do not overwrite a stable version (one without `-`); explain "same version, different content". Replace a prerelease version, such as `0.9.0-dev`, only when no game is running: assemble the new directory, rename the old one to `<target>.archo-old`, then delete that old directory, which is RimArcho's own artifact. Otherwise, defer the replacement until next time.
3. If there is no Monitor source, register a shared `extra` source (02 §0) with the label `monitor` and the Monitor directory as its root. First check that it and the data directory or any source root do not contain one another. All environments use this single source. Before 0.9.0, each environment had its own; migration v14 merges them by root.
4. Scan once if files were copied or a source was registered.
5. Update old versions in existing plans as described below, then add Monitor by default under 04 §1.5.

**Into the game.** Like the lock library, Monitor enters the game only through an `_Archo_` link (§3): `Mods/_Archo_monitor_ArchoMonitor-<version>`. The deployability checks, prelaunch synchronization and deletion rules are the same as in §3.

**Updates.** A new version is a new directory and a new Instance. Every live Instance under the Monitor source with packageId `toumanlin.archomonitor` counts as Monitor. The highest version, compared as a semantic About.xml `modVersion`, is the latest. If the Instances have different Lineages, merge them into the latest version's Lineage (02 §6, `lineage merge`). In every plan in the environment that contains an old Monitor version, swap it in place for the latest. Each plan gets one reversible edit named `update Archo Monitor <old> → <new>`. This does not depend on `monitor.default`, which controls only whether Monitor is added. Swap even while a game runs; do not wait for exit or repeat the swap at launch (00 §6 D21). This is a plan edit: it reaches `Mods/` only on the next launch or write to the game, when the old link is removed. Until then, the old link does not compete with the plan's new version (§3 pre-enable checks). A Monitor-only version update does not make this run's log stale (03 §6.4). Keep the old version directories, each less than 100 KB. A version that no plan uses is not linked into the game.

**FAT and exFAT.** If the volume holding `Mods/` does not support junctions, skip default addition and explain "`Mods/` is on exFAT, which cannot hold the link Archo Monitor needs". On Windows, identify this by the `FAT`, `FAT32` or `exFAT` filesystem name returned by `GetVolumeInformationW`. If the player enables Monitor manually, plan launch and writing to the game refuse under §3, naming Archo Monitor and giving `archo disable <ref>`. Do not fall back to copying.

**Packs.** Copy Monitor into a pack like any other Mod (§5.2, source `extra`). The Monitor in the pack writes to `<pack>/userdata/ArchoMonitor.log` (03 §6.7.3).

**Commands.**

- `archo monitor` shows the bundled version and location (or the searched locations), installed versions, the source, whether and where it appears in the current plan, and `monitor.default`.
- `archo monitor install [--dry-run]` executes the steps above. With `--dry-run`, it only lists what would happen.
- `archo status` adds a line: `monitor: ArchoMonitor 0.9.0 in the plan (after Harmony)`, `monitor: not in the plan (archo enable …)` or `monitor: not installed`.
- MCP `status` includes the same line, and `status show=settings` lists `monitor.default`. Enabling, disabling and moving use the existing `edit_loadout` tool.

## 10. Steam client calls (subscribe and unsubscribe)

This is implemented for unsubscribe and resubscribe in the mod library (06 §2.9). Like RimCrow, RimArcho subscribes to and unsubscribes from Workshop items through the player's running Steam client:

- **Bounded child process.** RimArcho launches `archo steam-worker subscribe|unsubscribe|state|download|watch <Workshop IDs…>`, which is not listed in help. Its working directory is `%LOCALAPPDATA%\RimArcho\steam`. This contains `steam_api64.dll` copied from the game installation and `steam_appid.txt` containing `294100`. The local 1.7 DLL is in the game root; also search the Unity layout at `RimWorldWin64_Data\Plugins\x86_64`. Recopy the DLL if the game's copy changes. Never write to the game directory. The child exits after its calls, but `watch` stays alive while waiting for downloads. Steam then shows "playing RimWorld", so downloads must be requested explicitly rather than left to Steam's background updates. A DLL failure cannot crash the window.
- **Calls.** Use `syscall`, without cgo, to invoke the flat Steamworks C API. Call `SteamAPI_InitFlat` (SDK 1.58 and later), falling back to `SteamAPI_Init` if it is absent. Select the highest available versions of `SteamAPI_SteamUGC_v0NN` and `SteamAPI_SteamUtils_v0NN` in the DLL. Issue `SteamAPI_ISteamUGC_SubscribeItem` or `UnsubscribeItem` for each item, then loop `SteamAPI_RunCallbacks`. Read each call's `EResult` through `ISteamUtils_IsAPICallCompleted` and `GetAPICallResult` until all calls have results or time out. For unanswered calls, use the subscribed bit from `GetItemState`. Before the operation, process the subscription-list refresh triggered by initialization. Afterward, continue processing callbacks and watch for confirmed-success items whose state reverts; retry each reverted item once. See 06 §2.16.1 S05 for the initialization, reply and compensation-observation intervals and deadlines. Finally, call `SteamAPI_Shutdown` and exit. The local game's 1.7.4982 DLL provides `SteamAPI_Init`, `SteamUGC_v016` and `SteamUtils_v010`.
- **Output** is line-oriented text parsed by the parent (`steamworks.Parse`): `ITEM <ID> ok|failed:<EResult>|timeout <state bits>`, `STATE <ID> <state bits>`, and `DONE`. If Steam is unavailable, the only line is `ERROR <reason>`. `watch` adds `PROGRESS <ID> <downloaded bytes> <total bytes> <state bits>`, emitted only when a sample changes. The parent consumes each line through `App.runSteamStreaming` and calls `BatchCalls.Progress` or `Done`, without waiting for the child to exit. A batch has at most 50 items. Ordinary calls time out after 40 seconds; download waits use the limits below.
- **Steam unavailable.** Steam not running, not being logged in, a missing or unloadable DLL, or a timeout produces `app.ErrSteamUnavailable`. The window opens the items' Workshop pages and explains the reason; the CLI error includes their page links. A Steam failure (`failed:<EResult>`) is reported as returned, without opening pages. The wording is in `app.WorkshopResultText`. Result 3 (no connection) says "the Steam client is offline", and task details suggest restarting Steam. A common cause is SteamCMD logging into the same account and displacing the client (06 §2.9).
- **Unsubscription lets Steam delete Workshop folders**, so it is destructive like recycling. The UI first shows `--dry-run` with the folder to be deleted, then asks for confirmation; MCP `manage_library` marks it as destructive. An enabled Mod or one used by plans in any environment only warns, listing the plans in `warning:`, and unsubscription proceeds. Those plan entries become missing. Refuse if a game is running (§4). Unsubscribed residue cannot be unsubscribed again (use `trash`), and an already subscribed item cannot be subscribed again.
- **Resubscribing subscribed Workshop originals.** `App.Workshop("resubscribe", refs, dryRun)` previews and then confirms that RimArcho will recycle the folder before Steam downloads it again, losing any local changes. Accept only Workshop originals with numeric Workshop IDs that are not themselves links and carry no locked marker. Enabled Mods and Mods used by any list are allowed; refuse if a game runs. First unsubscribe through the existing worker. Only for items whose unsubscription Steam confirms, recycle the entire original `…/workshop/content/294100/<Workshop ID>` directory, including extra files and `.git`, through the existing recycle backend and path allowlist. Never follow the copy's own symbolic link or junction to clean its target, and never delete permanently. Subscribe only after `Lstat` confirms that the original path is absent. If Steam has concurrently deleted it, count that as success. Briefly retry transient failures such as sharing conflicts under 06 §2.16.1 S10. If a directory still exists after the deadline, or its absence cannot be confirmed, report that item's cleanup failure and do not subscribe it; other successful items continue. Do not clean up or subscribe items whose unsubscription was not confirmed. Unsubscribed residue skips unsubscription and cleanup and subscribes directly. The GUI uses the existing Steam background-task queue, with at most 50 items per batch; the CLI and MCP execute in the foreground. In between, do not remove Instances, change lists or detach game links to the original path. Scan rule R1 (same source and relative path, 02 §5.1) reuses the original ID and Lineage when the folder returns, even if a scan marked it missing during the gap. Confirmed subscriptions, including residue, go to `watch`. For each item, call `ISteamUGC::DownloadItem(id, true)`, pump `SteamAPI_RunCallbacks` at the intervals in 06 §2.16.1 S08, read `GetItemState` and the byte counts from `GetItemDownloadInfo`, and obtain the install path through `GetItemInstallInfo`. An item succeeds only when its directory exists and is nonempty, Installed is set, and neither Downloading nor DownloadPending is set. Installed alone may refer to an old manifest and does not prove completion. See 06 §2.16.1 S08 for the per-item no-growth stall threshold and total wait limit. On expiry, report "Steam did not download; please check Steam" or a download timeout. `BatchCalls.Stopped` cancels the operation: the parent closes the input pipe, and the worker exits and calls Shutdown. If the DLL hangs, force-terminate and reap the child. Parent exit also causes pipe EOF, preventing orphan workers. See 06 §2.16 for task-page phases, per-item text and JSON. The CLI reports progress about every 10% and at each item's completion; the command returns only after the download completes or fails.
- **Anonymous local state.** If Steam is unavailable, `workshop state` and MCP `steam_state` return whether the local folder exists, explicitly marked `Local` with the reason retained. Subscription, update and download progress are unknown; state bits 0 must not be interpreted as "unsubscribed". Subscription and unsubscription still require an account, and failures must not be disguised as success.
- **Commands.** Use `archo workshop unsubscribe <refs…> [--dry-run]`, `archo workshop subscribe <refs…> [--dry-run]`, `archo workshop resubscribe <refs…> [--dry-run]`, and `archo workshop state <refs…>` (subscribed, installed, needs update, downloading). The MCP `manage_library` actions are `unsubscribe`, `subscribe`, `resubscribe` and `steam_state`. Do not rescan immediately after a call. Resubscription waits for the download to complete; ordinary subscription and unsubscription leave Steam to download or delete in the background, and the next scan sees the result.
- **Tests** use fake child processes (`App.SteamWorker` for ordinary calls and `App.SteamWorkerStream` for streaming waits), never real Steam. Real calls occur only after player confirmation. Manual development checks stop at the preview.

## 11. Downloading game versions with SteamCMD

**Account management.** Settings › accounts offers only login, relogin and logout. The one-line availability status is defined in 06 §2.14. After the player enters an account name, open the native SteamCMD console with `+login <name>`; the player enters the password and Guard response there and quits. After the window closes, run `+login <name> +licenses_print +quit` hidden to verify automatically with cached authentication, without passing a password. Confirm login only if both the login steps and subsequent user info are OK; support step and result messages on separate lines. RimWorld ownership requires app 294100 in a license's Apps/AppID list. Store only the ownership boolean and check time, not the license list. Anonymous users can download Workshop Mods but cannot own or download the game. Do not claim credentials have expired on a network failure, or reuse records from another tool path or account.

Logout runs `+logout +quit`. If it fails, is unconfirmed or is offline, automatically clear the managed copy's known local authentication cache (01 §4). Explain the scope beforehand in the confirmation text. Do not read tokens, back up authentication files or log out other devices. Copies at external paths or on `PATH` manage their own login; they do not get managed-account buttons or cleanup.

Workshop fallback, game downloads and account operations share the resolution of the actual tool instance and account. The instance lock covers the process lifetime. Public branch lists always use anonymous access; reading branches does not establish a game license. SteamCMD owns its console output. RimArcho adds no prompts or window titles containing untranslated text. Externally reported progress and errors contain only the permitted phases, counts and reasons, never raw authentication or license logs.

Game-version download is implemented (`archo env branches`, `archo env download`, and "download a version" in first-run step 1, 06 §2.15). It serves a player who owns RimWorld and wants an older version, such as 1.5, as another environment.

- **SteamCMD location.** Search in order: `steamcmd.path`, `%LOCALAPPDATA%\RimArcho\steamcmd\steamcmd.exe`, then `PATH`. If none is found, ask the player first, showing the source and size. Obtain `Content-Length` with a HEAD request to `https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip`. After consent (CLI `--install-steamcmd`), download and extract it to `%LOCALAPPDATA%\RimArcho\steamcmd`. SteamCMD updates itself on first run, downloading about 30 MB.
- **Branches.** Run `steamcmd +login anonymous +app_info_update 1 +app_info_print 294100 +quit`. Anonymous access can read RimWorld's app information without ownership. Read `depots/branches`, omit password-required branches (`pwdrequired`), and sort as follows: `public`, `unstable`, `version-*` from newest to oldest, then the others (`alpha*`, `beta*`). Never guess branch names. The list measured on 2026-10-22 (`testdata/steamcmd/app_info_print_294100.txt`) was `public`, `unstable` (Public testing), `version-1.6.4633`, `version-1.5`, `version-1.4.3901`, `version-1.4.3563`, `version-1.4.3542`, `version-1.3-latest`, `version-1.3.3287`, `version-1.2.3005`, `version-1.2`, `version-1.1`, `version-1.0`, `beta19`, `beta18`, `alpha17`…`alpha13`, `alpha4`. There is no `version-1.6`; the version of `public` is known only after downloading and reading Version.txt.
- **Branch cache.** Cache public branches in-process by appID and SteamCMD tool identity (normalized real path, file size and modification time). See 06 §2.16.1 C02 (`internal/app/gamebranches.go`) for validity and retry gates after failure. Concurrent queries for the same key run only once. When the cache expires, show the old list immediately, refresh anonymously in the background, and update the UI on completion. On failure, retain the old list and show the error; allow retry after a short backoff. The actual query still holds `accounts.Lock`, sharing instance exclusion with downloads and account operations. Reread tool identity after success to support SteamCMD self-updates. Cancel branch queries when the window closes. The UI shows a lightweight refresh notice. See 06 §2.16.1 G05 for status-polling backoff and the stop deadline; reentering the page allows another read. Download-log polling and waits for descendant processes follow S01 and S12.
- **Destination.** The directory must neither be inside nor contain the RimArcho data directory. It must not contain or be contained by any source, including any game's `Data` or `Mods` and any Mod folder, and must not be an existing environment's game directory. Create it if it does not exist.
- **Login.** Use the account in settings by default. Anonymous access can download only Workshop Mods; a game download asks the player to log into an account that owns RimWorld. The `steamcmd.account` setting may be empty (anonymous), `anonymous`, or an account name. The download form and CLI `--account` override it only for this run, with precedence: explicit account → settings → anonymous. Explicit `anonymous` overrides a saved account; clearing the setting restores anonymous access. Public branch lists always use anonymous access. RimArcho stores only the account name and does not receive passwords or Steam tokens. The player enters them in SteamCMD's **own console** (`CREATE_NEW_CONSOLE`) or the CLI's current terminal.
- **Download.** Run `+force_install_dir <directory> +login <account> +app_update 294100 [-beta <branch>] validate +quit`, omitting `-beta` for `public`. RimArcho does not read the SteamCMD console. It follows the tool's `<steamcmd>/logs/console_log.txt`, starting at the prelaunch file length and restarting from the beginning if the file becomes shorter. Convert `Update state (0x..) downloading|verifying update, progress: P (done / total)` into progress; `Success! App '294100' fully installed.` indicates success. Then confirm that the directory contains a game and register it through the existing-game flow. Name it after the version, such as `1.5`, use `direct` launch, and default to shared saves (CLI may use `--saves own`). Scan only its `Data` and `Mods`.
- **Plain-language failures.** Explain login, Guard, network, license, disk and branch failures using the permitted reasons in `DownloadError.Why`. For an unrecognized result, say that completion was not confirmed. Do not append raw SteamCMD output to external messages. A final `fully installed` overrides earlier errors because SteamCMD reconnects itself: `steamcmd has been disconnected from steam with result 3 (No Connection)` may be followed by `Retrying...`. In a 2026-10-28 VM test, a 16 GB download disconnected once but still completed. Previously, this was reported as Steam being unavailable and no environment was added (`TestSteamDownloadRecoversFromDisconnect`).
- **Unverified items.** Development did not use real login or download. A 2026-10-28 VM test used cached login to download `unstable` (16.4 GB, 37 minutes), verifying the progress-line format and disconnect text. Still unverified are the real login window and prompts, the wording of failures (only `No subscription` and the login `FAILED (...)` formats are certain), whether a nonexistent branch produces an error or installs `public`, and the completed directory's contents. Tests use a fake SteamCMD (`App.SteamCMDStart` / `SteamCMDRun`) that writes log lines. In development, a fake `steamcmd.exe` exercised the progress bar and environment addition in the browser.

## 12. Test runs

Before preparing isolated data or starting a process, perform the read-only link inspection from §3. If the tested game's links are broken or inspection fails, return a structured report with `status: links-blocked`, paths, targets, reasons and evidence. Do not create test data, change Mods or launch the game. The low-level `internal/gametest.Run` also performs a read-only precheck.

Test runs are implemented (2026-10-24, `internal/gametest`, `App.TestRun`, `archo test run`, MCP `test_run`; see `docs/research/2026-10-24_game-test-runs-rimdevtools.md` for research and measurements). Launch a plan in an environment's game, check whether it reaches the main menu or test map and which errors occur, then close the game and report. Do not touch the player's saves, configuration or logs, or write to the game directory.

- **Isolated user data.** Use `<data directory>/tests/<environment ID>/userdata`, passed to the game through `-savedatafolder` as `-savedatafolder=`. Reuse it across runs; 1.7 stores shader and texture caches here, so reuse makes a substantial difference. Before each run, write `Config/ModsConfig.xml` (`loadout.Render`, with known DLC as in §5.3). Copy the environment's other `Config/` `*.xml` files to preserve Mod settings, except `ModsConfig.xml`, `Knowledge.xml`, `Prefs.xml` and `KeyPrefs.xml`. Delete `Config/Knowledge.xml`. Also remove old Prefs and KeyPrefs from the isolated directory on each run, letting the target game generate default language, window and key preferences to avoid incompatibilities from sharing player configuration across versions. Retain the caches. The directory cannot be inside the game directory; `gametest` checks this.
- **Logs.** Each run gets `<data directory>/tests/<environment ID>/runs/<UTC time>/`, using `20060102-150405`. If a directory for that second exists, advance to the next second. It contains `Player.log`, merged stdout and stderr in `stdout.log`, the process result in `result.json`, and the attribution report in `report.json`. Keep only the latest 10 run directories per environment, retaining the `userdata` cache. For Unity 1.6 and earlier, add `-logfile <run directory>/Player.log`. From 1.7 onward, identified by `Waylith.dll` in the game directory, the file-log location is hardcoded to the player's LocalLow. Add `-NoFileLog` instead, and let RimArcho capture stdout and stderr. As it reads the console format (`HH:MM:SS  LEVEL message`), convert it to the file format in `Player.log` so that `internal/logs` can parse it normally. Launch without a console (`DETACHED_PROCESS`); terminal Ctrl+C reaches only RimArcho, which kills the game. A known limitation is that the 1.7 console uses the system ANSI code page. Characters it cannot represent, such as Chinese Mod names on an English system, become `?` in the log. Package and type names are unaffected, so attribution still works (measured 2026-10-24).
- **Launch.** Always start the executable directly, with the game directory as the working directory. Do not use Steam, which cannot provide a process handle. For Unity versions, also add `-popupwindow`. Refuse if a game is already running (`checkNoGame`).
- **Decision.** At the main menu, `MainMenuDrawer.Init()` writes `Config/Knowledge.xml`. Its appearance after launch marks arrival at the menu; then wait for `--settle` to collect late errors. For a test map (`--map`), add `quicktest`. After `Initializing new game with mods` appears, count arrival when the log has been quiet for the required threshold. See 06 §2.16.1 V01–V04 for sampling intervals, the settle wait and default time limits. An early game exit is `exited`; reaching the `--timeout` deadline is `timeout`. Kill the process at the end.
- **Game-invisible Mods.** The game can find plan Mods only in its `Data/`, `Mods/` (including existing links), or the environment's Workshop directory. A test run creates no links. List invisible Mods in the report's `missing` field and launch anyway.
- **Report.** Include the status (`menu` / `map` / `timeout` / `exited`), loading seconds from launch to the marker, total seconds, peak memory (the process's `PeakWorkingSetSize`), the requested Mod count and `missing`, the log's error clusters (`App.AnalyzeLog`, attributed against the tested plan), and counts by category. Text output is for people; `--json` is for MCP and scripts. Exit with 0 only if the target was reached without error clusters; otherwise, exit with 1.

XML errors in PrefsData or KeyPrefs are classified as `settings` (environment configuration), without listing suspected Mods. They still count as errors and produce a failing exit code.

### Acceptance

- `go test ./internal/gametest/` uses fake processes to cover the main menu, map, timeout and early exit. After console-log conversion, `logs.Parse` must still recognize the Waylith format.
- On a real machine, run `archo test run` for a small plan on 1.6 or 1.7. The modification times of `Logs/` and `Config/` under the player's LocalLow must remain unchanged, the game directory must be unchanged, and the report status must be `menu`.
