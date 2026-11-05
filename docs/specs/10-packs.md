# 10 · Packs

> Status: TODO(reorg)

A pack **builds** a plan into a fixed artifact. Each Mod is locked to its Instance and content snapshot at build time, with its Lineage retained. The pack contains only the content that this game version loads and uses its own user data directory. A list that works today may break after its Mods update; a pack gives the player an artifact that updates cannot change.

## 1. Data

**Location.** A pack lives at `<pack directory>/<pack name>/`. The pack directory is set by `pack.dir` (`archo config set pack.dir <path>`) and defaults to `<data directory>/packs`.

**Pack names** may contain letters, digits, spaces, `.`, `_` and `-`. The first character must be a letter or digit. The maximum length is 64 characters, and the name must not end in a dot or space.

**A pack's directory contains:**

| Path | Content |
| --- | --- |
| `pack.json` | The manifest described below |
| `mods/<directory name>/` | A copy of each bundled Mod; the original directory name with characters outside `[A-Za-z0-9._-]` replaced by `_`, and duplicates suffixed `-2`, `-3` |
| `mods/<directory name>/.rimarcho/origin.json` | The origin: original Instance, Lineage, Workshop ID, snapshot hash and copy time (the locked-version format, 07 §5) |
| `userdata/` | The pack's own user data directory (created on first launch, §3) |
| `state.json` | `{"last_played": time}`, the last played time |

**pack.json** currently uses `format` 2. Format 1 is still read; higher format versions are rejected.

| Field | Meaning |
| --- | --- |
| `format` | The format version |
| `name` | The pack name |
| `created` | The creation time (UTC) |
| `game` | The **full game version** at build time (the whole `Version.txt` line, for example `1.6.4633 rev1261`; see "full game version" in 03 §3.1). Packs that carry only `1.6` are still read, with the build treated as unknown |
| `env` | The source environment's name |
| `light` | A manifest-only RimArcho list with no files (07 §7.3); absent from the `pack.json` in a pack directory |
| `trimmed_bytes` | The total number of omitted bytes |
| `entries[]` | The entries in load order |

Each entry has these fields: `package_id`, `name`, `official` (Core or DLC from the game, not bundled), `lineage`, `instance`, `workshop_id`, `hash` (the semantic snapshot hash, 04 §1.4), `folder` (the directory under `mods/`), `bytes` (the copy size), `trimmed_bytes`, and `trimmed` (the omitted version directories). Format 2 adds the following fields (2026-10-24):

| Field | Meaning |
| --- | --- |
| `files` | The copy's file count, excluding `.rimarcho/` |
| `sha256` | The copy's content digest: one `<relative path>\0<bytes>\0<file SHA-256 hex>\n` line per file, excluding `.rimarcho/`, with `/` paths sorted bytewise; SHA-256 is then computed over the whole text. Computed at the end of the build, after texture conversion, and verified by import and `pack check` |
| `workshop_updated` | The Workshop update time (`timeupdated` from `appworkshop_294100.acf`, in Unix seconds), when available |
| `git_head` | The commit, when the Mod directory is a git repository |

The digest uses SHA-256 rather than the scan's xxh3 because packs may come from other people and must detect deliberate tampering.

**What is locked.** A pack locks each Mod's package ID, Lineage and Instance, Workshop ID, semantic hash and copy digest (and sometimes its Workshop update time or git commit). It also locks the load order (the order of `entries`), the full game version, and the DLC set. The `official` entries are the required Core and DLC; there is no separate field for them.

**Pack and list.** A pack always bundles Mod files and uses the `.archopack` extension (see the format below). To share only the list, export a RimArcho list (`.archolist.json`, 07 §7.3). This uses the same `pack.json` with `light` set to true and the file-related fields removed. Import compares local Instances by packageId and semantic hash.

**.archopack file** (format 2, 2026-10-24). The file is a tar stream compressed as a whole with zstd (klauspost/compress, `SpeedBetterCompression`, an 8 MB window, single-thread encoding, 17 §1). The first entry is `pack.json`, followed only by `mods/<directory name>/…`; each directory name must match a bundled entry's `folder`. Only regular files and directories are allowed, with no links. Format 1 ZIP files can still be imported. The reason for tar and zstd rather than ZIP is the result from ten real Workshop Mods (84.5 MB, mostly PNG): per-file ZIP deflate produced 42.4 MB and zstd 41.3 MB, almost the same. Whole-stream zstd, which shares a dictionary across similar XML files, produced 29.5 MB, 30% smaller at a similar speed. The cost is the lack of random access. Putting the manifest first lets it be read without extracting the whole archive. Any tool that reads `.tar.zst` can open it.

**Never bundle the game.** A pack contains only the compiled Mod copies, never the game, Core or DLC. The build skips `official` entries. Export writes only the manifest and bundled directories, excluding `userdata/`, `state.json` and all extras. Reject the entire import if it contains other paths (such as `RimWorldWin64_Data/`, `Data/Core/` or `Version.txt`), links, an `official` entry with a `folder`, or a bundled `package_id` starting with `ludeon.rimworld`. For comparison, a pack seen in October 2026 and advertised as "1.6 all DLC + Ratkin and four other races" (translated) was a WinRAR self-extracting exe (RAR5, 1.29 GB, 2.0 GB extracted, 44,670 files). It contained a full game (`RimWorldWin64_Data/`, `Data/Core` and DLC, with a replaced `steam_api64`) plus 38 Mod directories named by Workshop ID. It had no manifest, load order or hashes, and no way to verify it before extraction. RimArcho packs only the Mod part, with every entry verifiable.

Since 2026-10-22, there have been no "light packs". The former `pack build --light` created a manifest-only directory that could not launch and did not belong in the pack list. Players had to distinguish full and light packs when they only wanted to share a list with versions. Lists can now be exported from the current list (choose "RimArcho list", `archo loadout export --format archo`) or from a pack ("export the list", `archo pack export --list`). Old manifest-only directories are hidden and cannot launch; this feature was never released.

**Listing.** `archo pack list` shows the name, Mod count, size, game version and last played time. Size is the sum of the recorded `bytes`; listing does not traverse the copies unless the record is absent, in which case it walks `mods/`. An environment lists packs built there and packs whose source environment is absent locally but whose major and minor game versions match. It shows only the count of other packs.

## 2. Building

`archo pack build <pack name> [--dry-run]` builds a pack. In the UI, "create a pack" is a subpage of the plan (16 §2.6).

1. **Check.** The name must be valid, and no pack with that name may exist. Refuse and list any missing entries ("not installed: pkg:…") or error findings. **Sorting findings (`order.*`, `official.order`) do not block the build**, because the next step sorts the list. Exclude findings from logs. `version.unsupported` gives a warning with the Mod name and its declared versions, respecting the preferences in 05 §4.2, but does not block the build.
2. **Sort.** This is equivalent to `archo sort --dry-run`. Use the result only for the pack, leaving the plan unchanged, and record the number of moves.
3. **Select content (cleaning).** Keep only directories loaded by the current game version under the rules in 05 §1: use the matching LoadFolders.xml entries, or otherwise the shared directories and the highest version not above the current one. Omit unused version directories at the root and record them in `trimmed`. For example, a 1.6 environment keeps `1.6/` and the shared directories but drops `1.5/`. Exclude `.git` and `.rimarcho`. Count and skip links pointing outside the Mod directory, without following them. Do not copy Core or DLC.
4. **Measure.** Measure each copy's size, the omitted bytes and the free space on the pack volume. Refuse and list any unreadable Mod folders. Require the needed size plus 256 MB of free space.
5. **Copy.** Assemble the pack in `<pack name>.archo-tmp` inside the pack directory, writing each copy's `.rimarcho/origin.json`.
6. **Write pack.json**, then rename the temporary directory into place.

`--dry-run` stops after step 4 and lists the omitted version directories, sizes and warnings without writing to disk.

**The UI create button** is disabled when there are errors, with an explanation that opens can it run. "see the list first, write nothing" means `--dry-run` (16 §2.6, G14).

**Long tasks.** Copying takes minutes, so the GUI launches a child `archo pack build <pack name> [--env <environment>]` process. It uses the same data directory, runs detached from the window, and continues after the window closes. It writes progress to `<pack name>.archo-tmp/build.json`: the step (`check`, `measure`, `copy`, `write`, `done`), copied and total Mod counts, copied and total bytes, the current Mod, PID, start time and heartbeat. See 16 §2.16.1 P01–P02 and P04 for heartbeat and stale thresholds, write and report throttling, and the stop-check frequency. Write `build.json.part` and rename it so that readers cannot see a partial file.

- `archo pack stop <pack name>` creates a `stop` file in the build directory. The build checks every half-second, then stops and deletes the copied content when it finds the file.
- An interrupted build leaves its `.archo-tmp` directory. `archo pack list` lists these directories, and `archo pack remove <pack name> --unfinished` deletes them directly. They are RimArcho's unfinished output and do not go to the recycle bin. A running build requires `stop` first. Rebuilding with the same name replaces the old unfinished directory.

**Texture optimization.** There are two PNG-to-DDS modes (corrected 2026-10-22; previously allowed only in packs):

- **Ordinary optimization** ("convert textures to DDS" in the mod library, library › texture optimisation, `archo textures optimize`, MCP `manage_textures`) generates a same-name `.dds` beside each original PNG and retains the PNG. It processes all `Textures/` directories in the copy because Workshop directories are shared across game versions. A group with the same relative path is converted entirely or leaves no generated files. Before writing, record the batch as "in progress" in the database; afterward, mark it "complete". Failed or stopped groups remove the DDS files already written. Write and delete only DDS files that RimArcho generated and recorded. Skip groups with author-provided DDS files. Generated files are excluded from scan fingerprints. A Mod update produces `texture.stale`, offering regeneration or clearing. Refuse while the game runs.
- **Pack slimming** (the build page's texture options, `archo pack build --textures [--texture-scale N]`) adds a texture step after copying. It converts the pack copies, deletes successfully converted PNGs, and retains the PNGs in failed groups. The default is no scaling, called "pre-convert" in the UI; only the scaled mode is called "slim". Exclude RimArcho-generated DDS files in the original Mods from the pack copies. Measure the final converted size, but check free space against the peak when PNG and DDS coexist. `pack.json` records `textures`: the tool, scale, converted, retained and failed counts, and PNG and DDS bytes. The copy's `origin.json` also records `textures`; list export excludes that record.

Both modes use todds 0.4.1 (`-f BC1 -af BC7 -vf -fs`, with mipmaps): BC1 for opaque textures and BC7 for transparent textures. Full-size conversion costs disk space because DDS is larger than PNG, but saves loading time and halves video memory for opaque textures. The game encodes them as BC7 in 1.7 and DXT5 in 1.6, both at 8 bits per pixel; BC1 uses 4. Offer only scales that keep both dimensions divisible by 4 and the short side at least 128 pixels (20%–80%). Never scale masks (`_m`). Download todds from GitHub only after the player sees the source and size and agrees (library › texture optimisation, or `archo textures tool --install`). Alternatively, `textures.todds` can point to an existing copy, such as RimSort's. Conversion runs in a background archo child process and can be stopped by placing `textures.stop` in the data directory. See docs/research/dds-texture-optimization.md for the research, game loading rules and measurements.

## 3. Launching

Before moving a blocking copy, plan and maintain the incoming links from all games under 09 §3. The original confirmation lists the affected links. Environment and locked copies must not register twice.

`archo pack launch <pack name> [--dry-run] [--move-duplicates]` launches a pack. The UI provides a launch button for each pack (16 §2.7).

1. **Check.** A manifest-only directory cannot launch; suggest importing its `pack.json` with `archo loadout import`. Refuse if the pack's major or minor game version differs from the environment's, if `Mods/` is missing, or if any game is running (09 §1).
2. **Do not create same-package pairs in `Mods/`** (changed 2026-10-22). The former approach relied on `!` sorting first so that the pack copy won, but the game reported an error for every pair, as explained below. For each bundled copy, inspect the current same-package copies with `App.ModsCopies`:
   - If there is none, link the pack copy. It may coexist with a Workshop copy: the pack keeps the plain package ID, and the Workshop copy gets `_steam`.
   - If there is one player-owned folder or other-tool link (such as RimCrow `_Link_`) with the same content (its semantic hash equals the manifest's `hash`), **do not link the pack copy; use the existing `Mods/` copy**. Write its plain package ID in the list and show it under "from Mods/" in the preview and result. This is common for packs built from RimCrow `_<WorkshopID>_` copies.
   - If the content differs (it was updated after the build), or there are several copies, **refuse and offer to move them into the lock library** under the blocker rules in 09 §3. One confirmation moves the ordinary folders and then links the pack copy; their plans continue using them. Never move a link or a folder targeted by another tool's link. Refuse, explain the reason, and suggest moving it out of `Mods/` or rebuilding the current list. Never rename the player's folders.
   - If a plan's deployed `_Archo_` link has the same package ID, delete that owned link for this launch. A later plan save or launch recreates it.
   The pack list includes only bundled copies and the reused `Mods/` copies above.
3. **Create links.** Link `mods/<directory>` as `Mods/!ArchoPack_<pack name>_<directory>`, replacing special characters in the pack name with `_`. Use the deployment code and the link-type and deletion rules from 09 §3. Remove other packs' links, and retain this pack's existing links if their targets are correct. For a reused `Mods/` copy, create no link and remove any existing pack link. Refuse if a real directory occupies an intended link name.
4. **Copy settings.** If `userdata/Config/` is absent, copy the environment's `Config/` `.xml` files except ModsConfig.xml (graphics, keys and Mod settings); files keep their names. While `userdata/HugsLib/` is absent, copy the environment's `HugsLib/` files: HugsLib names settings by Mod, not by folder. Then, on every pack launch, apply 09 §3 "Settings follow the Mod" to the pack's `Config/` for the folders this run uses (a link name, or a reused `Mods/` copy's folder), reading the pack's own `Config/` first and the environment's otherwise; Archo Monitor's records of both data roots add folder names, such as the pack's links before a rename. The pack's files are never replaced, so its settings are independent after its first play. Other subdirectories are not copied.
5. **Write the pack list** to `<pack>/userdata/Config/ModsConfig.xml`. Use the environment's game version, falling back to the pack's, and record the installed official content as the known DLC. This is the pack's own file: overwrite it without a backup, leaving the environment's ModsConfig.xml untouched.
6. **Launch** with `-savedatafolder=<pack>/userdata`. Otherwise, follow 09 §2 for Steam or direct launch and extra arguments. The pack has its own list, saves and settings.
7. **Record** the last played time in `state.json` and a launch record with the method `pack:<pack name>`.

`--dry-run` shows the link changes (`+` / `-`), the pack's ModsConfig.xml path and entry count, the save directory, the settings to copy, and the command. It does not write to disk.

**Why the prefix is `!ArchoPack_`, and why it is no longer relied on.** The game registers `Mods/` entries by name, with `!` first. The original design let a pack copy win over a same-package local copy, which the game ignored as a duplicate. However, every pair produced an error. The Player.log from the player's 2026-10-21 "Test" pack launch had 6 such errors, including `_2888837973_` / `!ArchoPack____2888837973_`; `-savedatafolder` proves that RimArcho launched it rather than Steam. The player accepts none of these errors, so step 2 never creates such a pair. The prefix remains to identify which pack owns the link.

**Scanning skips pack links.** Enumeration skips links beginning with `!ArchoPack_`, so playing a pack creates no new Instances. The copy's `origin.json` is still written so that it can be matched to its original Lineage later if needed.

**`_Archo_` links.** Pack launch leaves the plan's deployment `_Archo_` links alone. They are absent from the pack list, so the game sees them but does not load them.

**Leaving a pack.** Launching or saving a plan removes all `!ArchoPack_` links (09 §2 step 4). They remain after the game exits so that relaunching the same pack does not need to recreate them.

**Direct Steam launch.** If the player launches through Steam without RimArcho, leftover pack links are read as ordinary Mods. The `!` prefix sorts first, and a same-package plan copy is ignored as a duplicate. The following behavior is implemented in `app.LeftPackLinks`:
- **Detection.** The current `Mods/` contains `!ArchoPack_` links, unless this environment's game is running that pack. This follows 09 §4 and `WhatRuns`: the command-line `-savedatafolder` matches the pack's userdata, or launch records are used as a fallback. Stopped games also count, because the next Steam launch would see the links. Check the directory on every findings read, outside the finding cache; inspect processes only if links exist.
- **Finding.** Report `pack.links-left` (warning, with the fix `archo pack unlink`) in `archo problems` and MCP `list_problems`. If the game is running this way, say "the game is running with the pack's copies". Same-package duplicates with `Mods/` upgrade the finding to error. List each pair in "breaks the game", since a Steam launch reports an error for each one. Pack and `Mods/` duplicates do not block a build: the build writes only the pack directory, and pack launch step 2 handles them.
- **UI.** Show one "for you to confirm" item with a one-click "remove the links" action and no confirmation, since relaunching the pack rebuilds them. Launching through RimArcho removes the links anyway, so this is not a "breaks the game" item. If there are no game-breaking findings, the launch-page notice shows this message and links to can it run (16 §2.3).
- **Removal.** `archo pack unlink [--dry-run]` and MCP `manage_packs action=unlink` remove only the links under 09 §3, leaving the pack unchanged. Refuse if any game is running.

**Pack Player.log (measurement pending).** The code assumes that 1.7 and later write logs to `Logs/Player.log` in the data directory, so a pack's log is at `<pack>/userdata/Logs/Player.log`. Earlier Unity versions use a fixed path shared with the environment. Whether the location actually follows `-savedatafolder` must be checked in the game, with the result recorded in 18. Log following and attribution use these locations (06 §4, §6).

## 4. Import, export and deletion

Before removing a pack, detach the owned links to it from all known games, not only the current environment. Incoming links from other tools block removal. The dry-run lists both directories and links.

- `archo pack export <pack name> <file> [--list]` writes the `.archopack` format 2 described above, containing only the manifest and bundled Mod directories. It excludes userdata and state, so saves and settings are not shared. First verify each copy's digest; for a format 1 pack, calculate and write back the digests and upgrade the manifest to 2. Refuse and list mismatches, explaining that the pack was modified and must be rebuilt. Write `<file>.part`, then rename it. `--list` writes a RimArcho list (`.archolist.json`, 07 §7.3), removing `folder`, sizes, trimmed directories and texture records. Refuse if the destination exists; never overwrite it. The UI offers "export" (`e`) and "export the list" (`E`).
- `archo pack import <file> [--as-plan]` handles two kinds of input:
  - For a pack (`.archopack`, with the header identifying zstd format 2 or ZIP format 1), read `pack.json` with a 64 MB cap. Validate the name and require that no same-name pack exists. Require free space of at least the sum of `bytes` plus 256 MB, and extract into `<pack name>.archo-tmp`. Reject the whole pack if a path contains `..`, is absolute, escapes the directory, or falls outside the "never bundle the game" allowlist. Each file has a 4 GB cap. **Verify** that every bundled directory exists, that format 2 digests and file counts match, and that `mods/` contains no unlisted directories. On any failure, delete the temporary directory and list the missing or different Mods. Rename it into place only after verification succeeds. A format 1 pack has no digests; report "content not verified (old format)". Then compare it with the current game (§5). The player can launch it with `archo pack launch`.
  - For a RimArcho list, compare live local Instances by packageId and semantic hash, grouping them as the same version, another version, or uninstalled (with Workshop links). `--as-plan` loads the list into the current plan reversibly. The player then obtains the missing Mods and builds a pack. UI lists use import preview (07 §7.2), which also shows "another version". The first-run "open" action extracts only `.archopack` files and routes other files to import preview.
- `archo pack check <pack name|file>` performs the comparison in §5. For a named pack, it also recalculates the copy digests. For an `.archopack` file, it verifies the stream without extracting to disk, using the same allowlist and digests as import. This allows inspection before import.
- `archo pack remove <pack name> [--dry-run]` removes the pack's `!ArchoPack_` links, then recycles its directory (02 §3). The preview shows the directory and every affected game's links, original targets and actions.
- `archo pack remove <pack name> --unfinished` and `archo pack stop <pack name>` follow §2.

## 5. Comparison with this game

Even a minor game upgrade can cause trouble, so a pack must state which build it was made for and warn on a mismatch. `app.CheckPack(pack, environment)` returns `PackCheck`, with three parts and a level for each item:

| Item | Test | Level |
| --- | --- | --- |
| Game | Different major or minor versions (`1.5` pack, `1.6` game) | error; refuse launch (§3 step 1) |
| | Same major and minor versions, but different builds or revisions (`1.6.4633 rev1261` vs `1.6.4871 rev590`) | warning: "created for 1.6.4633 rev1261; this game is 1.6.4871 rev590"; launch allowed |
| | Either build is absent | info: "build not recorded" |
| DLC | Required official entries are not installed | error; list the names and refuse launch (the game silently drops them, and dependent Mods fail) |
| | Installed DLC is unused by the pack | info; list it (not enabled, with no effect) |
| Mod | Pack: a copy is missing or its digest differs (calculated only for named `pack check` and import) | error; list it and refuse launch |
| | List (`.archolist.json`): the same version, another version, or uninstalled (§4) | warning for uninstalled Mods |

The comparison appears in `pack import` and `pack check` results, in the MCP `manage_packs check` action, and on the pack page (16 §2.7). `pack launch`, including `--dry-run`, shows warnings and info at the start and refuses if there are errors.

## 6. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| K1 | Three Mods, one with 1.5 and 1.6 directories, in a 1.6 environment | Include only 1.6 and shared content; record omitted directories and bytes in the manifest (`TestPackBuildAndLaunch`) | Pass |
| K2 | Build with nonsorting errors or missing entries | Refuse and list them (`TestPackBuildAndLaunch`); sorting-only findings permit the build | Pass |
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
| K23 | The environment has settings for a bundled Mod and `HugsLib/ModSettings.xml`; launch the pack twice, changing its settings between | The dry run lists the copy; the first launch writes `Mod__ArchoPack_<pack>_<folder>_<class>.xml` and the pack's HugsLib file, a reused `Mods/` copy keeps its file name; the second launch replaces nothing (`TestPackSettingsFollowTheMod`, `TestCarryModSettingsDirs`, `TestCopyHugsLib`) | Pass (synthetic); real game pending release VM tests |
| K17 | A leftover pack link duplicates a local copy; two player folders share a package ID; a folder appears beside `_Archo_` | can it run reports `pack.links-left` as an error with link removal, or `pkg.duplicate-mods` with guidance or use of the existing copy. Writes create no duplicate deployment (`TestPackLinksDuplicateReported`, `TestPlayerDuplicatesReported`, `TestDeployNoDuplicates`, `view.TestModsDuplicateNeed`) | Pass |
