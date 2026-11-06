# 09 · Launch

> Status: implemented: writing on launch, one running game at a time, launch records that name the plan, link deployment and repair, running-game detection (Windows only), and hidden Mods. Test runs are in 20; Archo Monitor is in 19.
> Basis: 00, 00a, 02 §1 / §3, 03 §3.4 / §4.3, 04 §1.2, 06 §4 / §7, 07 §1.5 / §4 / §5, 12 §4, 16 §2.

This specification describes how RimArcho hands a plan to the game: which files it writes, which links it creates, how it launches the game, and how it detects a running game. It also covers hidden Mods, whose actions change Mod folders directly (§5); test runs are in 20, and Archo Monitor, the bundled in-game helper Mod, is in 19. Packs are in 10; Steam client calls and SteamCMD downloads are in 11.

Every action that writes to disk follows 02 §1: RimArcho writes only to the locations listed there, and each write offers a preview (CLI `--dry-run`, or "preview …" in the UI command panel) and is reversible or explains why it cannot be. Each action reports what it did. Launching and writing to the game execute in one step in the CLI, TUI and GUI, without a confirmation prompt (16 §6). A refusal gives the reason and commands that can resolve it. RimArcho cannot undo hiding Mod folders or sending them to the recycle bin, so the UI asks for confirmation first (§5, 02 §3).

## 1. Launch model

**One game at a time.** Before launching, RimArcho checks for game processes across all environments (§4). If any game is running, it refuses the launch and names the environment and the plan or pack being played. Environments and plans share ModsConfig.xml and the save directory, so two games running at once would overwrite each other's files.

**Write on launch.** ModsConfig.xml is a projection of a plan, not the source of truth (00a §1). Plan edits are saved in RimArcho's database at once (07 §1). They reach the game only when:

- the player launches a plan (§2);
- the player clicks "save" (`archo save`), for launching through Steam or another route.

**Sharing the default user data directory is fine.** For example, the Steam and SteamCMD installations of RimWorld may both use `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`. Only one game runs at a time, and each launch writes the plan intended for that run, so the previous writer does not affect the next launch. For each user data directory, RimArcho records the hash it last wrote and the writer (the plan), to detect outside changes (07 §4.3). See 03 §3.4 for the other consequences of sharing the directory.

**Pack exception.** A pack uses its own user data directory (`-savedatafolder`, 10 §3) and leaves the environment's ModsConfig.xml and saves untouched.

**Launch records.** Every plan or pack launch records the environment, the plan or pack, the time, the launch method, the full command line and the fingerprint of the enabled list. Logs are matched to the latest launch by their write time (06 §4). Save management uses these records to identify the plan (12 §4).
The table is `launch(environment_id, started_at, method, command, plan_id, fingerprint, version_n)`. `method` is `steam`, `direct` or `pack:<pack name>`. A plan launch records the plan, the fingerprint of its enabled list (`ListFingerprint`, also used for stale detection) and the version played. A pack launch has no plan and records the fingerprint of the pack's list.

## 2. Launching a plan

`archo launch [--dry-run]` launches the current plan. See 13 §4 for arguments that name a plan and for other reference forms. The UI entry points are the launch control in the top bar, "launch the game" on the Launch page, and `L` (16 §2.1, §2.3). One click launches, and the status line reports the result. If step 1 or 2 below refuses the launch, the status line gives the reason and commands that can resolve it (16 §6). The command panel's "preview the launch" is read-only.

**Steps** (if any step fails, stop without executing the later steps):

1. **Check.** No game may be running (§1, §4). The environment must have a user data directory; otherwise, suggest `archo env set --user-data <dir>`. If an existing ModsConfig.xml cannot be read, refuse and ask the player to repair or delete it first.
2. **Handle outside changes.** If the file differs from the last write, first attribute the change to the last plan that wrote it, under 07 §4.3. Load the change if possible, then continue. If it creates a conflict (an outside version has not been loaded), refuse and offer commands to view the diff, load the change or ignore it (07 §4.1 item 1). This prevents the write from silently overwriting edits made in the game's Mod menu.
3. **Synchronize `_Archo_` links** (§3): create any missing links that the plan needs, and remove extra links or links with the wrong targets.
4. **Remove all `!ArchoPack_` links** (10 §3): the game must not see pack copies when playing a plan. Steps 3 and 4 run as one operation plan that removes links before creating them.
5. **Write ModsConfig.xml** under 07 §4.1: back it up as `ModsConfig.xml.rimarcho.bak`, write a temporary file and replace it atomically, write the current game `version`, and use the `_steam` tokens. Record the file's hash and writer afterward. Then give Mods seen under a new folder their settings (§3 "Settings follow the Mod").
6. **Record the launch** (§1). It is recorded only after the game process has started (step 8). A failed launch leaves no record, so no log is matched to a launch that never happened.
7. **Record a plan version with the source `launch`** (07 §3). Do not record a duplicate if it is identical to the plan's latest version.
8. **Start the game process**, without waiting for it to exit.

The links are handled before ModsConfig.xml is written. If a link operation fails (for example, because `Mods/` is on exFAT), the file remains unchanged instead of referring to Mods that are unavailable.

**Two launch methods** (the environment's `launch` field, set with `archo env set --launch direct|steam`):

| Method | Run | Arguments |
| --- | --- | --- |
| `steam` | The Steam client (`steam.exe`, `steam.sh` or `steam`, found in the detected Steam directory) | `-applaunch 294100`, followed by the extra arguments |
| `direct` | The first executable found in the game directory: `RimWorldWin64.exe`, `RimWorld.exe`, `RimWorldLinux`, `RimWorldMac.app` | The extra arguments |

- Both methods append `-savedatafolder=<path>` when the user data directory differs from the local default, using normalized paths for the comparison.
- The player enters "extra arguments" in the "this game" group in settings (16 §2.14). They are stored as `env:<environmentID>:launch_args`, split on whitespace and appended last.
- The working directory is the game directory. Arguments containing spaces or tabs are displayed in double quotes.
- If Steam is missing, refuse and suggest `archo env set --launch direct`. If the game executable is missing, refuse and name the directory that was searched.

**`--dry-run` shows** the `_Archo_` and `!ArchoPack_` links to create or remove (`+ name` / `- name`), the ModsConfig.xml path, whether `version` will be rewritten from X to Y, the numbers of entries to write and missing entries, and the full launch command. It does not write to disk or launch the game. `save --dry-run` shows the same preview without the launch command.

**Prelaunch notices** (corpus G8): below the launch command, `archo launch` (including `--dry-run`) lists the current plan's findings that are most often associated with game errors. These notices **do not block launch**.
- The `LaunchChecks` table in `internal/app/launchcheck.go` selects the finding codes in descending order of the corpus observed vs. chance measure (05 §4.3.1): `patch.no-match`, `def.parent-missing`, `incompat.active`, `dep.inactive`, `dep.missing`, and the corpus-layer codes `dep.observed-missing` and `combo.known-bad` (warnings only). To add a code, add one row containing the code, a brief reason with the corpus numbers, and a summary sentence for multiple findings.
- Each kind gets one line. A single finding shows its conclusion and first suggestion. Multiple findings show a summary ("5 patches miss in 2 Mods: #a A, #b B") and their common first suggestion, or `archo problems --code <code>` if they have none. Separate `why:` and `fix:` lines follow; `why:` gives the reason and corpus numbers.
- Ignored findings and findings derived from logs are excluded. Launch does not wait for analysis: Def and Patch checks use the stored analysis, with an additional `checks:` line if it has never run or is stale.
- In the UI, the problem strip below the launch button shows the kind most likely to cause trouble and a brief reason. It links to Can it run (16 §2.3).
- The UI asks once more: if the current plan has `dep.missing` or `dep.inactive` findings (missing hard dependencies), clicking launch first shows "required mods are missing" and lists each missing dependency. The player must choose "launch anyway" to continue. The game will not load these Mods and may reset the list to vanilla and DLC (VM scenario 9, B8). If the launch would be refused anyway (an outside conflict, a running game or another reason), the refusal comes first and there is no prompt. The CLI and TUI do not ask.

**Writing to the game (`archo save`)** executes steps 1–5 and 7 above, with the version source recorded under 07 §3, but creates no launch record and starts no process. Writing is allowed while the game runs; the result says "the game reads it on its next start", because the game does not reread this file while running. `save --dry-run` shows the same preview. The UI calls this "save" and places it at the top right of the list editor (16 §2.5). The main rules are in 07 §4.

**After the game exits.** When an exit is detected (§4), or when RimArcho next opens, suggest "analyse this run's log" if the current Player.log is newer than both the latest launch and the last log the player viewed. Launch attribution follows 06 §4. The signal is `view.Live.NewLog`: the TUI displays it on the right of the status line and dismisses it when the player opens the logs. The GUI shows no notice; it reads the log once when the signal appears (`API.ReadNewLog`, 16 §2.1), so the Can it run badge includes this run.

**What does not block a launch.** A plan has no "unsaved" state that blocks launch, because launching writes the list intended for that run; there is no `launch --force`. A ModsConfig.xml whose major and minor `version` differ from the game's does not block launch either: step 5 rewrites it. The notice shown when reading such a file remains (07 §4.1). RimArcho does not ask whether to save before launching.

## 3. Link deployment

**Target lifecycle (shared).** Enumerate incoming links at the first level across all games, covering environments, local sources and physical Mods directories known to the ledger. Deduplicate them by physical path. Refuse if the operation would break links belonging to other tools, or if an inspection result is unknown. First preview the operation and persist its intent, then detach the links safely, move the target or send it to the recycle bin, update the database, and rebuild the required entries. Each Instance may have at most one effective entry per game. Refuse if an affected game is running. On failure, reconnect a link only if its original target exists. The phase ledger is independent of aliases. Startup reports `mods.link-recovery` without making changes; recovery requires a confirmed repair. Do not clear healthy environment or pack links on exit.

**Link health and repair.** Each call to `InspectGameLinks` rereads the game roots of all registered environments and the `Mods/` directories of nonretired local sources, deduplicating them by physical path. It inspects only first-level `_Archo_*` and `!ArchoPack_*` entries. Read the junction or symbolic link's reparse type and raw target first, then distinguish a target that exists, one that is definitively missing, and one whose status is unknown or inaccessible. Do not rely on ordinary Mod enumeration, aliases or the directory-mtime cache. Unregistered games are outside the inspection's coverage.

Broken links, ownership mismatches, inspection failures and name collisions with real directories produce the game and path findings defined in 05 §4.3. Both scanning and Can it run display them. A broken link or failed inspection in the selected game blocks plan, pack and test-run operations before they write or launch; findings from other games are shown only as summaries. Always retain environment links whose targets exist. Extra valid pack links still follow 10 §3.

Preview a repair first, and execute it only after explicit GUI or TUI confirmation or CLI `links repair --yes`. Detach only first-level junctions or symbolic links in a known `Mods/` directory that have an owned prefix and are proven broken or aimed at the wrong target. Show the ownership evidence from the known Instance or pack and the expected target. If a prefixed link's existing physical target matches a live Instance's real_path after normalization and case-insensitive comparison, use "target is known instance X" as the ownership evidence, even if the ID in the link name has been deleted. Retain such a link if it is healthy, without reporting a finding or renaming it. Enumerate these incoming links when moving or recycling that Instance too. Duplicate entries are still reported under the existing rules. A link with only a prefix and no target matching a known Instance or pack requires the player to include it with `--include <full path>` or explicitly in the UI; never delete it silently. Never delete real files or directories, entries without an owned prefix, unknown tags or inaccessible items. Serialize repairs across processes by physical game directory, and refuse repairs for running games. Before execution, revalidate the preview's parent directory, file identity, type and raw target; refuse if any has changed. On Windows, pin the parent and link handles and detach the link by handle. On other systems, pin the parent, revalidate, and unlink each entry individually, without recursion or deletion through the target. Undoing deployment removes only the links; it never recycles targets or uses the recycle bin. The result lists the detached links. On failure, preserve the remaining items. Do not propose rebuilding a link to an unverified new target.

Mods may live in directories the game does not read: SteamCMD download directories, personal Mod repositories, the RimArcho lock library (07 §5), and the Archo Monitor directory (19 §6). These are `steamcmd` and `extra` sources (03 §4.1). Before launch, directory links make them available in `Mods/`.

**Deployable.** An Instance is deployable if its source is `steamcmd` or `extra`, it is live, it is neither residue nor `unloadable`, and it is not already visible to the game through a non-RimArcho alias in official `Data/`, `Mods/` or Workshop directories (03 §4.3). A deployable Instance can be enabled directly.

**Link names.** A link is named `Mods/_Archo_<source label>_<directory name>`. Replace `/`, `\` and `:` in the label and directory name with `_`. Deployable sources are shared Mod folders with globally unique labels (03 §4.1), so the same Instance has the same link name in every game. If a label changes, the next write to the game replaces the old link name under the rules below. The `_Archo_` prefix means "managed by RimArcho" and is matched case-insensitively.

**Deployment timing.** When launching a plan or writing to the game (§2 step 3), create any missing links for the plan's deployable Instances. Delete `_Archo_` links that are absent from the plan or point to the wrong recorded Instance. Remove links before creating them so that old links with the same name can be replaced. `--dry-run` only lists the changes. Editing a plan never touches `Mods/`.

**Settings follow the Mod** (`app/modsettings.go`). The game names a Mod's settings file after the folder it sees the Mod under: `Config/Mod_<folder>_<Mod class>.xml` (`Verse.LoadedModManager.GetSettingsFilename`, identical in 1.6 and 1.7). The whole name goes through `GenText.SanitizeFilename`, which turns each run of `"<>|:*?\/{}!@#$%^&` or control characters into one `_`, so a pack link `!ArchoPack_x_…` reads `Mod__ArchoPack_x_….xml` (`app.SettingsFile`). A Mod that moves between its Workshop folder, a `Mods/` copy, an `_Archo_` link and a pack link would otherwise read a new file and start with its defaults. So each write to the game (save and launch, after ModsConfig.xml) checks every Mod the registry loads: for each Mod class that has a settings file under another folder name of the same package (lower case, without `_steam`) but none under the current one, copy the newest such file to the current name. A package's folder names are those of its installed and former copies: Workshop and `Mods/` folders, other tools' links (aliases), another game's `Mods/` folder, and `_Archo_` deployment names, plus every folder Archo Monitor recorded the package under in that save-data root (19 §2, 06 §7 `kind:settings`), which also covers copies since deleted or renamed. Both parts of a file name may contain underscores, so a file belongs to the longest known folder name it starts with. Never overwrite a file, change or delete the original, or ask. Copies keep the source's modification time. A failed copy is listed with its reason and does not stop the write; `--dry-run` lists what would be copied. Settings stored elsewhere are untouched: HugsLib keys its file by Mod, Mod Settings Framework uses a fixed file, and XML Extensions keeps its client Mods' settings in its own standard file, which follows XML Extensions. Known limit: a player who deletes the current file to reset a Mod gets the old one back on the next write while that file exists. Packs follow 10 §3 step 4, test runs §7.

**Environment switching.** Before scanning or resolving the list, fill in the target list's missing shared-Mod links (such as Monitor) and local-Mod links from the previous environment; see 03 §2 item 14. Local Mods use `_Archo_env_<Instance ID>`. These links represent installed local Mods. An ordinary save retains healthy links with known types and existing targets. Anomalies require inspection and confirmed repair. Uninstalling still removes these links under the `_Archo_` rules.

**Pre-enable checks** (perform these while editing the plan, rather than waiting for a write):

- Deployable Instances in the plan and Instances targeted by existing `_Archo_` links register as "in `Mods/`, folder name = link name" (07 §4.2). Exclude an existing link's target if it is absent from the plan and the plan contains another Instance with the same package ID (`deploySet`). The next write removes that link, so the two copies will not coexist in `Mods/`. Otherwise, after an Archo Monitor update but before the next save, the old link would sort first by folder name and falsely make the new version `mod.unloadable`.
- Refuse to enable a copy that conflicts with a Mod of the same package ID already in `Mods/` (`pkg.duplicate-installed`). The game recognizes only the first registered copy.
- **Recheck on write.** Two copies with the same packageId in `Mods/` cause "Tried loading mod with the same packageId multiple times", and the game ignores the later copy (`Verse.ModLister.TryAddMod`, the same in 1.6 and 1.7). Only a Workshop copy can coexist with a same-package copy in `Mods/`, as `_steam`. If a same-package folder appears after editing (a manual copy or a RimCrow `_Link_`), save and launch do not create the `_Archo_` link (`deployDuplicates`). Instead, they offer to **move that folder into the lock library** (below), or the player can use `archo swap` to select the existing copy. Can it run reports pairs that already exist (end of this section).
- **Another version blocks the way** (shared by `_Archo_` deployment and pack launch, `app/dupmove.go`). If a requested copy has the same package ID as an ordinary folder in `Mods/` but different content, the refusal returns `DuplicatesError`. It lists each blocking folder, who needs the requested copy (`#ref name` or "Y from pack X"), the folder's destination in the lock library, and the links to create once the way is clear.
  - **One confirmation.** The UI (save, plan launch, the pack's launch button, and the pack on the Launch page) asks once. It lists "`Mods/X → <lock library>/X`, then link `Mods/Y` for …". Choosing "move and go on" moves the folder and resumes the save or launch without another prompt. CLI and MCP refusals provide the same command with `--move-duplicates` (`archo save|launch --move-duplicates`, `archo pack launch <pack> --move-duplicates`; MCP `save_loadout` / `launch_game` / `manage_packs action=launch` with `move_duplicates: true`), or individual `archo library move-to-locked <ref>` commands. `--dry-run` lists "would move into the lock library first" without writing to disk.
  - **The move** is the lock-library move from 07 §5 (`moveToLibrary`): rename on the same volume; otherwise copy, verify and recycle the original. The Instance remains unchanged, so **its plans still use it**. The next write of one of those plans links it back through `_Archo_`. After moving, reread the state before saving or launching; only the requested copy remains in `Mods/`.
  - **Never move** a link, whether owned by RimArcho or another tool (such as RimCrow `_Link_`), a folder targeted by another tool's link (moving it would break the link), an unknown folder (scan first), or a folder containing outward links (07 §5). If any blocker cannot be moved, offer no repair command. Refuse and give the reason for each one.
  - The tests are `TestPackLaunchMovesOtherVersion` (the pack launches after the move, and writing the copy's plan links exactly that version), `TestPackLaunchNeverMovesLinks`, `TestSaveMovesOtherVersion`, `TestDuplicateFixMovesToLocked` and `gui.TestPackLaunchMovesConfirm`.
- A same-package Workshop copy permits only `swap`, just like an ordinary local copy. The deployed copy writes the unsuffixed package ID; the Workshop copy writes `<package ID>_steam`.

**Link types.** Windows uses directory junctions, which need no administrator privileges. Other systems use symbolic links. Refuse if the volume holding `Mods/` does not support junctions (FAT or exFAT), and explain the reason without falling back to copying; copies are only for packs. If deployment is required but the environment has no `Mods/`, refuse and suggest `archo env set --game <dir>`.

**Deletion.** Remove only the link itself (on Windows, `RemoveDirectory` removes the junction), and never enter its target. First verify that it is a link: a reparse point or symbolic link. Never touch a nonlink directory whose name begins with `_Archo_`; report an error asking the player to rename or move it. Report the same error if a real directory occupies the intended name of a new link.

**Identity.** Scanning resolves links to real paths (on Windows, using `GetFinalPathNameByHandle`). A link is only an alias of the Instance; it creates neither a new Instance nor an event (03 §4.3).

**Data directory protection.** Before writing to `Mods/`, refuse if it contains the RimArcho data directory or is contained by it.

**No takeover of others' links.** RimCrow `_Link_<WorkshopID>` links are recognized as ordinary local Mods (04 §5 A11). RimArcho never manages or deletes them.

**Duplicate packageIds in `Mods/`** (`pkg.duplicate-mods`, error, "breaks the game" in Can it run). Check the actual contents of `Mods/`, regardless of who put them there: ordinary folders, other tools' links, and `_Archo_` or `!ArchoPack_` links. `App.ModsCopies` reads the packageId from each folder or link and caches the result while `Mods/` remains unchanged. One-click fixes affect only items owned by RimArcho. If a pair includes an `_Archo_` link, use the existing `Mods/` copy (an undoable `swap` when it is in the same Lineage), or remove the linked copy from the plan so that the next write removes its link. Duplicates caused only by pack links are merged into `pack.links-left` (10 §3), with a one-click "remove the links" action. If an ordinary folder can be moved under the blocker rules above, the first action is **"move into the lock library"** (`M`, pressed again to confirm; `archo library move-to-locked <ref>`). Prefer the folder beside an `_Archo_` link; otherwise, move the copy that no plan uses and the game ignores. Its plans continue using it after the move. If none can be moved, explain that the player must manually move the extra copy out of `Mods/`.

## 4. Running-game detection

**Discovery.** On Windows, `CreateToolhelp32Snapshot` lists processes whose executable names start with `rimworld`, and `QueryFullProcessImageName` obtains their full paths. A path under an environment's game directory identifies that environment's game. Also read:

- The start time (`GetProcessTimes`).
- The command line (`NtQueryInformationProcess` with `ProcessCommandLineInformation`; since Windows 8.1, only `PROCESS_QUERY_LIMITED_INFORMATION` is needed). Extract the value of `-savedatafolder=`; either the value or the whole argument may be quoted.

Detection is not implemented on other systems, where games are always considered stopped.

**Across environments.** The prelaunch check (§1) inspects every environment. A process whose executable is under any registered game directory counts as a running game. The refusal identifies the environment and uses the launch records to name the plan or pack. Read the process list once and compare it with each registered root using path boundaries (`RimWorld2` is not within `RimWorld`). Both the refusal and `archo status` name the plan (for example, `environment main is running plan:default`) or pack. For a game launched outside RimArcho, display `plan:X (last written to the game)`.

**What is being played.** After finding a process, determine whether it is playing the environment's list or a pack:

- If the command line is available, no `-savedatafolder` argument, or a value equal to the environment's user data directory, means the environment's list. A value equal to `<pack directory>/<pack name>/userdata` means that pack. For any other directory, record it as an unknown user data directory, preserving the path.
- If the command line is unavailable, take the latest launch record before the process start time plus 1 minute. If its method is `pack:<pack name>` and it is within 10 minutes of the process start (Steam takes time to launch a game), infer that pack and label it "from the launch records". Otherwise, treat it as the environment's list.

**Frequency.** See [16 §2.16.1](16-interface/2.16.1-scheduled-behavior.md) G01 and L01 for GUI and TUI reads of game status and new logs, and for CLI `archo log --follow` checks of the log and game exit. There is no resident background process.

**Display.** Show "not running" or "running", with the PID and start time when running.

- `archo status` displays `game: running the list for 12m (pid 1234; archo log --follow)`. For a pack, it uses `pack <pack name>`; an inferred pack adds `(from the launch records)`.
- In the GUI, the disabled block on the Launch page says "the game is running" and shows the error count for this run. The "last game" control at the top right does the same. Errors enter Can it run and its badge (16 §2.1, §2.3).
- The TUI displays the same information in its status bar (16 §9).
- On exit, stop following the log, perform a full analysis (06 §6), and suggest log analysis under §2.

## 5. Hidden Mods

Before recycling the target of a hidden, locked, residual or subscribed Mod, execute the incoming-link plan for all games under §3. Download-page replacement, SteamCMD updates and unsubscription also detach links before replacing the target. On failure, reconnect only targets that still exist.

Other managers, such as RimCrow, rename `About/About.xml` to `About.xml.disabled` to hide a Mod copy from the game. Scanning creates an Instance with `unloadable: about-disabled` for a directory that contains only the disabled file (04 §1.2).

**Classification** is computed during each analysis and is not stored. Findings are reported only for disabled Instances of this kind.

| Category | Condition | Finding |
| --- | --- | --- |
| Locked version | Another live, loadable, nonresidual, non-Workshop Instance in the same Lineage is visible to the game or deployable. RimCrow `_<WorkshopID>_` copies, locked copies and copies deployed in `Mods/` all count; prefer an enabled copy if several exist | `mod.pinned`, info |
| Unsubscribed residue | A Workshop source absent from the ACF subscription list | `mod.hidden-residue`, warning |
| Individually hidden | All other cases | `mod.hidden`, warning |

- Locked versions **do not report** `mod.unloadable`: the copy is effective, and the Workshop original is the "upstream". The product rules for creation, swapping only in the current plan, and upstream notices are in 07 §5. This section covers only the hidden folders.
- **Upstream update.** If the Workshop snapshot differs from the copy and the ACF `timeupdated` is later than the copy's first appearance, upgrade `mod.pinned` to warning and offer "see the changes". Compare semantic files, treating the upstream About.xml.disabled as About.xml: `archo mod <copy> --diff <Workshop copy>`. The output uses `+ only in latter`, `~ changed` and `- only in former`.
- Measured category counts: 18 §3.

**Actions:**

| Category | Action | Command | Writes |
| --- | --- | --- | --- |
| Locked version | See differences | `archo mod <instance> --diff <instance>` | None |
| Locked version | Unlock | `archo unpin <instance> [--dry-run]` | Rename the Workshop About.xml.disabled to About.xml; swap the copy to the Workshop version in the current plan if present; retain the copy |
| Unsubscribed residue | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle the whole folder |
| Unsubscribed residue | Resubscribe | `archo workshop subscribe <instance> [--dry-run]` | No file writes; subscribe through the Steam client, which downloads the Mod (11 §2) |
| Individually hidden | Keep hidden | `archo problems ignore <p:ref>` | Write only to the RimArcho database (the existing ignore mechanism expires on Mod changes, 05 §4.2) |
| Individually hidden | Restore | `archo unhide <instance> [--dry-run]` | Rename About.xml.disabled to About.xml |
| Individually hidden | Move to recycle bin | `archo trash <instance> [--dry-run]` | Recycle the whole folder |

**Rename rules (unhide, unpin):**

- Accept only live about-disabled Instances. An `unpin` reference may identify the copy or upstream original. Refuse unless there is a pair consisting of a hidden Workshop original and a loadable copy.
- Refuse without changing files if About.xml already exists in `About/` (case-insensitive), or if About.xml.disabled is absent.
- Before renaming, check that the data directory and the folder do not contain one another.
- Rescan immediately after the rename.

**`trash` conditions** (also see the 02 §3 allowlist). The Instance must be live, from a Workshop or SteamCMD source, and either about-disabled or unsubscribed residue. Unsubscribe subscribed, loadable Mods through Steam instead. The real path must be inside the source root and must not equal it. Enabled Mods and Mods used by plans do not block the operation: list the plans in `warning:` and proceed. Rescan after moving the folder. The Instance becomes a tombstone, and its plan entries appear as missing.

**UI.** The status and action columns in Mod library › "all mods" provide the one-click actions above (16 §2.9), with a preview and confirmation first. Can it run does not offer these actions; its bottom line links to the Mod library (16 §2.4).

## 6. Archo Monitor

RimArcho bundles the in-game helper Archo Monitor and deploys it through an `_Archo_` link like the lock library (§3). Bundling, installation, updates and its commands are in 19 §6.

## 7. Test runs

Test runs and probe runs launch a plan with isolated data and report its errors; they are in 20.

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
| J7 | Launch directly after edits | Write ModsConfig.xml, then launch; `--force` is neither needed nor accepted | Pass |
| J8 | Launch with an old ModsConfig.xml `version` | Write the current game version and launch without blocking | Pass |
| J9 | Two environments share the default user data directory; launch their plans in sequence | Each launch writes its intended plan; the second write attributes the file to the second plan | Implemented (writer per user data directory, `TestV8SharedUserData`); real sequential launches await release VM tests |
| J10 | Edit the list in the game menu, exit, then launch another plan in RimArcho | First attribute the outside changes to the last writer under 07 §4.3 (load an external version or report a conflict), then write the target plan | Pass (loadable changes are adopted before writing; conflicts refuse) |
| J11 | Launch a plan | Record the environment, plan, time and fingerprint; add a `launch` version unless it matches the latest | Pass |
| J12 | `archo save` | Write ModsConfig.xml and links without starting a process or creating a launch record | Pass |
| J13 | After exit, Player.log is newer than the launch | The TUI suggests analysis; the GUI reads the log and includes the run in the Can it run badge (`TestReadNewLogCountsTheRun`) | TUI pass; GUI synthetic tests pass, VM pending |
| J15 | Run `launch --dry-run` with `patch.no-match`, `dep.missing` or similar findings | Show `before you play: …`, one line per kind with `why:` and `fix:`, without blocking launch; add `checks:` if never analyzed (golden `loadout`, `problems`) | Pass |
| J16 | A Mod's settings file exists under its Workshop folder; swap to a SteamCMD copy (an `_Archo_` link), `save --dry-run`, `save`, change the copy's settings, swap back and save | The preview lists "would copy" and writes nothing; save copies `Mod_<ID>_<class>.xml` to `Mod__Archo_steamcmd_<ID>_<class>.xml` and keeps the original; swapping back copies nothing and changes neither file (`TestSettingsFollowDeployedCopy`, `TestCarryModSettings`, `TestSettingsFileMatchesGame`) | Pass (synthetic); real game pending release VM tests |
| J14 | A pack process has an unreadable command line (`TestWhatRuns`); `-savedatafolder` is quoted (`TestSaveDataArg`) | Infer the pack from launch records and label the inference; extract the correct path | Pass |

### Hidden Mods

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| H1 | A RimCrow Workshop copy is about-disabled and `Mods/_1234_` is enabled | Report `mod.pinned` info, with no `mod.unloadable` (`TestHiddenPinned`) | Pass |
| H2 | The Workshop content and ACF time update after H1 | Report `mod.pinned` warning "upstream has an update" and show the changed files | Pass |
| H3 | An unsubscribed Workshop folder is about-disabled | Report `mod.hidden-residue`; the trash preview lists the directory; recycling and rescanning create a tombstone (`TestHiddenResidueTrash`) | Pass |
| H4 | A Workshop Mod is subscribed, with no local copy | Report `mod.hidden`; `unhide` makes it loadable (`TestHiddenAlone`) | Pass |
| H5 | About.xml already exists during unhide | Refuse without writing files | Pass |
| H7 | Unpin a locked copy used in the current plan | Restore the Workshop About.xml and swap only the current plan; retain the copy and leave other plans unchanged | Verification pending (current plan only, 07 §5.5) |
