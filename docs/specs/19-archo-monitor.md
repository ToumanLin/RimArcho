# 19 · Archo Monitor

> Status: implemented. A few checks need real 1.6 and 1.7 games (§8; 06 §9 L14, L16).
> Basis: 03, 04 §3, 06, 07 §1.5, 09 §3, 10 §2, 17

Archo Monitor is the in-game helper Mod that RimArcho bundles. This document describes what it records in the game (§2), what that costs (§3), the companion log it writes (§4), how it differs between game versions (§5), how RimArcho installs and updates it (§6), and how it is built and tested (§7). How RimArcho reads its log is in 06 §7; adding it to plans is in 07 §1.5.

## 1. Purpose

Only the game knows what `Player.log` cannot say: which file or `Def` was loading when an error occurred, which Mod owns an assembly or Harmony patch in the stack, and how many errors occurred after the logging limit. Archo Monitor (`packageId` `toumanlin.archomonitor`) records these in-game as JSON lines in a companion log. Its behavior follows RimCrow Companion 0.3.12; the decompiled code was read only to understand behavior and format, and the implementation was written independently. No code was borrowed from the projects it credits: BetterModSort (Apache-2.0), LogAfterDefError (MIT), BetterLog (MPL-2.0), or Mod Error Checker (no public license). About.xml credits them all.

Archo Monitor settings use RimWorld Keyed translations and follow the game language. Package the `Languages` directory with the Mod; building and testing are in §7.

## 2. What is captured

| Hook | Action |
| --- | --- |
| `Prefix` on `Verse.Log.Error(string)`, `Verse.Log.Warning(string)` | Record every error and warning. The `Prefix` runs before the `StopLoggingAtMessageCount` check, so errors are counted even after the game stops writing `Player.log`. |
| `Prefix` on `Verse.Log.Message(string)` | Off by default because informational entries are the most numerous and this is a hot path. Turning on "Record info messages (Log.Message)" in the Mod settings attaches it on the next startup. |
| `Application.logMessageReceivedThreaded` | Capture `Exception`, Error, and Assert messages outside `Verse.Log`, with no `Verse.Log` frame in the stack. |
| `Prefix` + finalizer on `DirectXmlLoader.DefFromNode`, `DirectXmlToObjectNew.DefFromNodeNew` (`1.6`+) | Keep a thread-local stack of the loading `LoadableXmlAsset` and `Def` node. Also map `defName`, or `Name` if absent, to the file, owning Mod, and `ParentName`. |
| `Prefix` + finalizer on `LoadedModManager.ApplyPatches` | Mark XML patching. On the first entry, collect all enabled `Mods`' operations: map XPath `defName` and `Name` values to the Mod and operation type, including `Sequence`, `FindMod`, and `Conditional` children. |
| `Postfix` on `DirectXmlToObject.ValidateListNode` | Map a referenced `Def` name to the loading `Def`, file, and list field from the stack above, without another XML traversal. Use this for missing cross-references. |
| `Postfix` on `Verse.UIRoot_Entry.Init()` | Menu initialization is complete and `MainMenuDrawer.Init` has already run. Enter menu, recording it again when returning to the menu from the game. |
| The same postfix, first time per session (Monitor ≥ 0.10.0, `SettingsRecord`) | Write the **settings record**: for each running Mod class (`LoadedModManager.ModHandles`) with a settings window (`SettingsCategory()` not empty) or settings read through `GetSettings` (`Mod.modSettings` set), its packageId, `Content.FolderName`, simple class name, the file name `LoadedModManager.GetSettingsFilename` builds (falling back to `GenText.SanitizeFilename`), the window title, and whether settings were read. Monitor calls only what the game calls itself: `SettingsCategory` when listing Mod options, and the file name `GetSettings` uses. RimArcho uses it for 09 §3. |
| The same postfix, first time per session, once the queued start-up work is done (`LongEventHandler.ExecuteWhenFinished`; Monitor ≥ 0.11.0, `HugsLibRecord`) | When HugsLib is loaded, write the **HugsLib record**: each settings pack (`HugsLibController.SettingsManager.ModSettingsPacks`) with its identifier (`ModId`, its element in `HugsLib/ModSettings.xml`), dialog entry (`EntryName`), and owning Mod's packageId and folder (the child Mod whose `SettingsIdentifier` is the pack's, from `HugsLibController.Instance.childMods`; for HugsLib's own pack `HugsLib`, `OwnContentPack`; empty when not found); and per setting (at most 300, `full` false beyond) its name, title, description, value and default (`ToString()` of `Value` / `DefaultValue`), the value type's simple name, `hidden` (`NeverVisible`) and, for an enum type, `choices` (`Enum.GetNames`, at most 50). Everything is found by reflection, and only property getters run (never `StringValue`, whose getter can reset a value). Texts are cut at 300 characters. Without HugsLib nothing is written. |
| `Prefix` + finalizer on `RimWorld.Dialog_ModSettings.DoWindowContents(Rect)` (Monitor ≥ 0.11.0, `SettingsWindow`) | Write a **settings window record** for the first frame of each opening of a Mod's settings window (a new dialog instance): the Mod's settings file as above and what the window drew, in order, at most 300 entries (`full` false beyond): headings (`label`), and each control's kind (`checkbox`, `radio`, `slider`, `text`, `number`, `button`), label, current value and a slider's `min` / `max`. The first opening patches `Widgets.Label(Rect, string\|TaggedString)`, `CheckboxLabeled`, `Checkbox`, `RadioButtonLabeled`, `ButtonText`, `TextField`, `TextArea`, `HorizontalSlider` (both forms) and `IntEntry` with prefixes and postfixes that record only while that frame draws and otherwise just check a flag (no finalizer on these hot paths). A control drawn inside another (a checkbox's label) is not recorded again; a field drawn right after a heading takes it as its label (Listing_Standard's labeled sliders and entries). Rich-text tags are removed, texts are cut at 200 characters, and the dialog's own title is dropped. What a Mod draws by hand or in its own dialog (HugsLib: see the HugsLib record) is not seen. |
| `Prefix` on `Verse.SavedGameLoaderNow.LoadGameFromSaveFileNow(string)` | Begin loading a save and enter save. Keep the filename without its directory. |
| `Prefix` on `RimWorld.Planet.WorldGenerator.GenerateWorld`, `Verse.Game.InitNewGame()` | Begin world generation or new-colony initialization and enter new. Record consecutive entries into the same phase only once. |
| `Postfix` on `Verse.Game.FinalizeInit()` | New-colony or save initialization is complete. Enter play. |

- **Stack → Mod**: map the frame's declaring-type assembly to the Mod loading that assembly, cached by assembly. For Harmony patches on the frame's method, use the patch method's assembly, falling back to matching the owner against `packageId`, cached by method. Exclude base assemblies (mscorlib, `System`, netstandard, `Microsoft`, `Mono`, `MonoMod`, `Unity`, `UnityEngine`, `Assembly-CSharp`, `Assembly-CSharp-firstpass`, `0Harmony`, `HarmonyLib`, `Waylith`, `Brutal`, `ICSharpCode`, `Newtonsoft`, `NAudio`, `NVorbis`, `ISharpZipLib`, `com.rlabrecque.steamworks.net`, and Monitor itself). Include base `Mods` (Harmony, HugsLib, XML Extensions, Prepatcher, Performance Optimizer, RocketMan, and Mod Error Checker) only if no other Mod is found. Stop at the first non-base Mod frame. For textual stacks supplied in messages or `Unity` callbacks, parse `Type.Method` line by line and use the same mapping.
- **Extra context**: use the first rule that yields a result. For `Config error in X:` or `Exception in ConfigErrors() of X:`, collect X's file, the parent chain's files (up to 10 levels), and patch operations on those `Defs`. For `Could not resolve cross-reference … named X`, collect the referencing `Def`, file, and field, plus patches adding X to those `Defs`. For `… loadID X`, record only `loadID:X`. While loading a `Def`, collect the current file, `Def`, and Mod. Also map message packageIds such as `[a.b]` and `assembly name: X` to `Mods`.
- **`inferredType`** uses the first match: `DefConfigError` (the Config error above), `CrossRefError` (cross-reference or loadID), `PatchError` (during XML patching or a `Patch operation` message), `XMLSyntaxError` (while loading a `Def`; the RimCrow name means an error during XML loading), `AssemblyConflict` (`MissingMethodException`, `TypeLoadException`, `ReflectionTypeLoadException`, `MissingFieldException`), `TickException` (`Exception ticking`), `DrawException` (`Exception drawing`), `NullReference`, `OutOfMemory`, or `MissingTexture` (`Failed to find any texture`, `Could not load Texture2D`, `Could not load UnityEngine.Texture2D`). Otherwise, it is empty. A Monitor failure writes `ObserverFailure` without affecting the game.
- **Stack levels**: in 1.5 and `1.6` (`Unity`), Monitor raises Error and `Exception` stack levels below `ScriptOnly` to `ScriptOnly`, leaving `Full` unchanged. It does not change them if other diagnostic `Mods` (Visual Exceptions or Mod Error Checker) are loaded. In 1.7, `Waylith` has no such setting and already writes full stacks.
- **Unchanged messages**: RimCrow adds a `[Mod name, …] ` prefix. Monitor preserves the `Player.log` message exactly so that RimArcho can match it.

## 3. Overhead

- **Per log entry (hot path)**: look up the level and original message in a hash table once. A hit only increments the count. A miss normalizes the message and analyzes its stack and context once per distinct message.
- **Loading**: each `Def` requires one stack push and pop and one dictionary write. Each `Def` list item requires one dictionary write, without constructing a node path. Collect patches in one pass. The tables are capped at 300,000 `Defs` and 500,000 references; recording stops when they are full.
- **File writes**: use a 64 KiB buffer. Flush the first occurrence of an `ERROR` immediately because the game may crash next. A background timer writes other first occurrences and count changes at most once per second. Flush on `Application.quitting` and `AppDomain.ProcessExit`.
- **Phase changes**: handle them only when a hook fires, without polling or work per frame. Under the same Writer lock, call `FlushDirty`, then write and flush the phase boundary. Do not add stacks, reflection, or file writes to the per-error hot path.

## 4. Files and format

- **Location**: write `ArchoMonitor.log` under the save-data root (`GenFilePaths.SaveDataFolderPath`, which follows `-savedatafolder`, so packs use `<pack>/userdata/`). If unavailable, use the command-line `-savedatafolder`, then `Application.persistentDataPath`. On startup, rename the existing log to `ArchoMonitor-prev.log`, replacing the previous file, and create a new current log.
- Use UTF-8 without a BOM, with one JSON object per line ending in `\n`. Truncate message content above 4 KiB and details above 16 KiB, adding … . At 32 MiB, write `kind:"full"` and stop writing new entries. Phase changes and exit still write the remaining counts and boundaries.
- **Deduplication**: the fingerprint combines the level, inferred type, and normalized lowercase message (hex addresses → `<hex>`, numbers → `<num>`, and whitespace collapsed to one space). The exact-message hot table is capped at 8000 entries, and the fingerprint table at 4000. When full, flush pending counts before clearing them. Later occurrences of the same messages are then treated as first occurrences again.

**Format (schema 1)** is a superset of the RimCrow format, so one reader handles both.

| Field | RimCrow Companion | Archo Monitor |
| --- | --- | --- |
| `timestamp` | `yyyy-MM-dd HH:mm:ss.fff`, the local time when this line was written | Same |
| `level` | `ERROR`, `WARNING`, `INFO` | Same |
| `message` | The text before the stack, prefixed with [names] if `Mods` were found | The original text before the stack |
| `details` | The stack text on every line | The stack appears only on the entry's first line; later count lines use an empty string to avoid repeating it. An `ERROR` or `WARNING` without its own stack gets the stack of the code calling `Verse.Log`, excluding Monitor and logging frames. |
| `count` | Occurrences represented by this line: 1 at first, then the increase since the previous line for the same entry | Same |
| `context.relatedModIds` | The lowercase, player-visible `packageId` | Same |
| `context.relatedFiles` | A mixture of Mod-relative and absolute paths, `patch:<Def>:<operation>`, `xref:<name>:<node-path>`, `def:<name>`, `loadID:<ID>`, and `<root:<node>>` | Only absolute file paths; other references go in `context.refs` |
| `context.inferredType` | The names above, except `PatchError` | The names above |
| `source` | — | `"archo-monitor"` |
| `schema` | — | `1` |
| `id` | — | The entry's 8-hex fingerprint, identical on every line for that entry |
| `total` | — | The cumulative count through this line |
| `kind` | — | Present only on special lines: session (first), exit (last on normal exit), full, phase, settings, settings-window, and hugslib |
| `context.phase` | — | The phase on a phase row: menu, save, new, or play; loading applies before the first phase row |
| `context.save` | — | The save filename on a save phase row, if available, without its directory |
| `context.settings` | — | On the settings row (message `[Archo Monitor] settings <n>`): `[{packageId, folder, class, file, title, stored}]`, one element per Mod class (§2) |
| `context.roles` | — | `{"<packageId>": "xml" \| "def" \| "patch" \| "stack" \| "text"}` records how the Mod was found: file owner, definer in the `Def` chain, patcher, stack or patch owner, or message mention. |
| `context.refs` | — | `patch:<Def>:<operation-type>@<packageId>`, `xref:<name>:<Def>/<field>`, `def:<name>`, `loadID:<ID>` |
| `context.def` | — | The `Def` loading when the error occurred |

**Session line**. The first line has level `INFO`, message `[Archo Monitor] session <id>`, and kind session. Its context contains session (12 random hexadecimal digits), version (the Monitor version), game (`VersionControl.CurrentVersionStringWithRev`), `dataRoot`, and `playerLog` (`Application.consoleLogPath`, empty if unavailable). Monitor also writes `[Archo Monitor] <version> session <id>: <full-ArchoMonitor.log-path>` to `Player.log` through `Log.Message`. This lets RimArcho pair the files: a `1.6` pack's `Player.log` stays in `Unity`'s fixed location while the companion is in the pack's data root, so the explicit path avoids guessing. **Exit line**. Its kind is exit, and `context.entries` is the number of distinct entries.

**Phase line**. Its level is `INFO`, kind is phase, message is `[Archo Monitor] phase <phase>`, and `context.phase` is one of the values above. The other envelope fields follow schema 1. Flush pending count rows before the change, so that each count row belongs to one phase; total remains cumulative for the whole run. Write the same phase message to `Player.log` through `Log.Message`, regardless of the informational logging setting. Read boundaries independently in each file's order. Phase messages themselves never form error clusters.

## 5. Version differences

| | 1.5, `1.6` | 1.7 |
| --- | --- | --- |
| Runtime | `Mono`, with assemblies compiled for net472 | .NET 10 (`Waylith`), with assemblies compiled for net10.0 |
| Harmony | 2.x bundled with the Harmony Mod | 3.x |
| `Application` | `UnityEngine` | `UnityStubs`: has `logMessageReceivedThreaded`, quitting, and `consoleLogPath`, but no `SetStackTraceLogType` |
| `DefFromNodeNew` | Present in `1.6`, absent in 1.5; find it by reflection and skip the hook if absent | Present |
| Phase hooks (§2) | All exist in the reference assemblies. `GenerateWorld` lacks `LandmarkDensity` in 1.5 and has it in `1.6`; find it by name through reflection. | All exist; `GenerateWorld` has the same parameters as in `1.6`. |

`LoadFolders.xml` selects directories by game version: `1.5/` and `1.6/` contain the net472 assemblies, and `1.7/` contains net10.0 assemblies. Find hook targets through reflection. Skip missing targets and list them in the session line's `context.skipped`.

The phase hook signatures are checked for each version against the Krafs reference assemblies selected by `monitor/ArchoMonitor.csproj`. These references contain only method signatures. The actual timing of menu entry, world generation, save loading, and playable state must be tested in full game rounds in both `1.6` and 1.7 (the pre-release VM checklist).

## 6. Installation and updates

This section describes Monitor's path from the release package to the game.

**Bundled Monitor.** The release ZIP includes `monitor/ArchoMonitor/`, a complete Mod folder with `About/`, `LoadFolders.xml`, and assemblies for `1.5/`, `1.6/` and `1.7/` (17 §5). RimArcho searches in this order:

1. Check `RIMARCHO_MONITOR`, which points to a Mod folder for development and tests.
2. Check `monitor/ArchoMonitor/` beside the executable.
3. For development builds (`buildinfo.Version` is `dev`), search upward from the executable directory and working directory for `monitor/out/ArchoMonitor/` (the `dotnet build` output, 17 §2).

If none is found, do not install a bundled copy; an already scanned Monitor from any source remains available for plans. `archo monitor` lists the locations searched.

**Build version.** `monitor/VERSION` is the source of the bundled Monitor version, independent of the launcher version. Increment it when Monitor behavior or its format changes. `monitor/build.ps1`, direct `dotnet build`, and the release script all use this value by default. Developers can explicitly override it with `build.ps1 -Version` (MSBuild `MonitorVersion`); the generated About.xml `modVersion` matches the assembly version.

**Installation.** Install to `<Monitor directory>/ArchoMonitor-<version>/`. The Monitor directory is set by `monitor.dir`, defaulting to `%LOCALAPPDATA%\RimArcho\monitor`. Like the lock library, it is outside the data directory (02 §1). Take the version from About.xml `modVersion`, allowing only `[0-9A-Za-z.+-]`. Each version has one Mod folder directly under the Monitor root. Sources inspect only one level below their root (03 §4.1), so do not use a two-level `<version>/ArchoMonitor` layout.

**Check timing.** Check when the GUI or TUI opens, on `archo scan` or `archo monitor install`, and when creating a plan (07 §1.5). The steps are:

1. Read the bundled Monitor's version and content fingerprint (each file's relative path, size and SHA-256).
2. If the target directory is absent, assemble `<target>.archo-tmp` and rename it into place. If it exists with the same fingerprint, leave it unchanged. If its content differs, do not overwrite a stable version (one without `-` or `+`); explain "same version, different content". Replace a prerelease version, such as `0.9.0-dev`, only when no game is running: assemble the new directory, rename the old one to `<target>.archo-old`, then delete that old directory, which is RimArcho's own artifact. Otherwise, defer the replacement until next time.
3. If there is no Monitor source, register a shared `extra` source (03 §2) with the label `monitor` and the Monitor directory as its root. First check that it and the data directory or any source root do not contain one another. All environments use this single source.
4. Scan once if files were copied or a source was registered.
5. Reconcile Monitor selection in existing plans as described below, then add Monitor by default under 07 §1.5.

**Identity and selection.** Every Instance with packageId `toumanlin.archomonitor` counts as Monitor, regardless of source (Workshop, `Mods/`, SteamCMD or an extra folder). Select among live, subscribed, loadable Instances by About.xml `modVersion`, using `CompareVersions`; the highest version wins. On a version tie, prefer the Workshop copy. Multiple installed Monitor copies do not produce `pkg.duplicate-installed`; genuine duplicate folders in `Mods/` still follow 09 §3.

**Into the game.** The selected Workshop or `Mods/` copy is already visible to the game and needs no deployment link. A copy from the Monitor source uses `Mods/_Archo_monitor_ArchoMonitor-<version>`; other sources follow the usual deployment rules (09 §3). Launching or writing the plan removes stale managed Monitor links when selecting a copy that needs none. Only the selected Monitor is active in ModsConfig.xml; a Workshop copy beside a higher-version linked copy stays inactive (no second `_steam` entry).

**Updates.** A new bundled version is a new directory and a new Instance. If the eligible Monitor Instances have different Lineages, merge them into the selected copy's Lineage (04 §3, `lineage merge`). In every plan containing Monitor, replace its Monitor entries with the selected copy at the first Monitor position, including when selection changes source at the same version. Each changed plan gets one reversible edit named `update Archo Monitor <old> → <new>`. This also collapses an imported plan containing both local and Workshop copies to one active Instance. This does not depend on `monitor.default`, which controls only whether Monitor is added. Swap even while a game runs; do not wait for exit or repeat the swap at launch (00 §6 D21). This is a plan edit: it reaches `Mods/` only on the next launch or write to the game, when the old link is removed. Until then, the old link does not compete with the plan's new version (09 §3 pre-enable checks). A Monitor-only version update does not make this run's log stale (06 §4). Keep the old version directories. A version that no plan uses is not linked into the game.

**FAT and exFAT.** When the selected Monitor needs a deployment link and the volume holding `Mods/` does not support junctions, skip default addition and explain "`Mods/` is on exFAT, which cannot hold the link Archo Monitor needs". A selected Workshop or `Mods/` copy needs no link, so default addition, sorting and explicit addition work on these filesystems. On Windows, identify this by the `FAT`, `FAT32` or `exFAT` filesystem name returned by `GetVolumeInformationW`. If the player manually enables a copy that needs a link, plan launch and writing to the game refuse under 09 §3, naming Archo Monitor and giving `archo disable <ref>`. Do not fall back to copying.

**Packs.** Copy the selected Monitor into a pack like any other Mod (10 §2). The Monitor in the pack writes to `<pack>/userdata/ArchoMonitor.log` (§4).

**Commands.**

- `archo monitor` shows the bundled version and location (or the searched locations), installed versions, the source, whether and where it appears in the current plan, and `monitor.default`.
- `archo monitor install [--dry-run]` executes the steps above. With `--dry-run`, it only lists what would happen.
- `archo status` adds a line: `monitor: ArchoMonitor 0.9.0 in the plan (after Harmony)`, `monitor: not in the plan (archo enable …)` or `monitor: not installed`.
- MCP `status` includes the same line, and `status show=settings` lists `monitor.default`. Enabling, disabling and moving use the existing `edit_loadout` tool.

## 7. Building and testing

**Building** (`monitor/`):

- SDK-style csproj, `dotnet build`; requires .NET SDK 8+ (`net10.0` requires SDK 10). Compile the same source twice: `net472` (for `1.5/`, `1.6/`, Mono + Harmony 2.x) and `net10.0` (for `1.7/`, Waylith + Harmony 3.x). Handle differences with constants (`RW_UNITY`, `RW_WAYLITH`) and reflection (§5).
- Do not commit game assemblies. Default references use NuGet `Krafs.Rimworld.Ref` (`1.5.4409` for 1.5, `1.6.4871` for 1.6, `1.7.4984-unstable` for 1.7). Setting `RimWorldDir=<game directory>` references local game assemblies instead (`RimWorldWin64_Data/Managed`, or game root for 1.7) to build against the local version. Harmony uses `Lib.Harmony` (2.3.x for net472, 3.0.x for net10.0), `ExcludeAssets=runtime`: exclude assemblies from output; use Harmony Mod's at runtime.
- `build.ps1 [-Version <version>] [-RimWorldDir <directory>]`: build for 1.5, 1.6 and 1.7, assemble `monitor/out/ArchoMonitor/` (write version to About.xml `modVersion`; source in §6). `go run ./scripts/monitoricon` generates `About/Preview.png` and `About/ModIcon.png` (same source as application icon; do not commit PNGs).
- During development, `RIMARCHO_MONITOR=<repository>/monitor/out/ArchoMonitor` selects this build for local RimArcho (§6).

**Tests**. In-game behavior requires game testing. Game-independent tests live in `monitor/test` (`dotnet run -c Release -p:GameVersion=1.7`): message splitting, fingerprints, stack-frame parsing, JSON writing, deduplication and count lines, rotation. RimArcho-side installation: §8; reading, pairing and evidence: L5–L16 in 06 §9. Package the `Languages` directory with the Mod and check that the settings labels and tooltips follow the game language (English and Simplified Chinese) and that the info-message setting is kept (§2).

Manual checklist (repeat on 1.6 and 1.7; use a test plan and save, not regular playthroughs):

1. Build: `pwsh monitor/build.ps1 -RimWorldDir <game directory>`, then set `RIMARCHO_MONITOR=<repository>\monitor\out\ArchoMonitor` and run `archo monitor install`. Expect `installed`, `registered src:monitor`, `plan:<name>: added after Harmony`.
2. Prepare known XML errors: in a local test Mod's `Defs/`, place a `ThingDef` missing `label` (`Config error in <defName>`) and a list item referencing a nonexistent Def (`<costList><NoSuchThing>1</NoSuchThing></costList>`, missing cross-reference). Enable this Mod.
3. `archo launch` (or launch in the UI). Confirm `Mods/_Archo_monitor_ArchoMonitor-<version>` is a link and `toumanlin.archomonitor` immediately follows `brrainz.harmony` in ModsConfig.xml.
4. After the game reaches the main menu: `ArchoMonitor.log` exists in the save data directory; its first line is a session line (`version`, `game`, empty `skipped`). Player.log contains `[Archo Monitor] <version> session <id>: <path>`.
5. Two errors in `ArchoMonitor.log`: Config error has the test Mod's XML in `relatedFiles`, the Mod in `relatedModIds`, and `inferredType` of `DefConfigError`; the cross-reference error has `inferredType` of `CrossRefError` and `xref:NoSuchThing:…` in `refs`.
6. While the game runs, open Can it run, "this run": both errors appear, with evidence "log · ArchoMonitor.log · Defs/…"; the same item in `archo log --follow --once` is followed by the test Mod (source).
7. Exit the game: last line of `ArchoMonitor.log` is `kind:"exit"`. Launch again: the previous file becomes `ArchoMonitor-prev.log`; evidence in "errors from the last run" still names the file.
8. Play once with a pack on 1.6 (`archo pack build`, `archo pack launch`): log goes to `<pack>/userdata/ArchoMonitor.log`; the marker in Player.log points there.
9. Overhead: compare load time with Monitor enabled and disabled in a large plan (hundreds of Mods), using a stopwatch or Player.log timestamps (`gui.log` does not cover this). Difference should be within a few seconds; many repeats of one in-game error should not cause stutter.
10. Turn on "Record info messages (Log.Message)" in the Monitor's Mod settings, restart, and check for INFO entries in `ArchoMonitor.log`; turn it off and check that they are gone.

## 8. Acceptance

RimArcho's reading of the companion log is accepted in 06 §9 (L5–L16).

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| M1 | First run with the bundled Monitor | Copy to `<monitor.dir>/ArchoMonitor-<version>/`, register `src:monitor`, and scan one Instance (`TestMonitorInstallAndDefault`) | Pass |
| M2 | Run again | Do not recopy or register a duplicate | Pass |
| M3 | A new version is bundled | Create a new directory and Instance in the same Lineage; swap every old-version plan with one undoable edit per plan (`TestMonitorUpdate`) | Pass |
| M4 | Launch a plan containing the bundled Monitor copy | Create the `Mods/_Archo_monitor_ArchoMonitor-<version>` deployment link and place Monitor immediately after Harmony in ModsConfig.xml (`TestMonitorInstallAndDefault`; real game pending) | Pass |
| M5 | `Mods/` is on exFAT | A copy needing a link skips automatic addition; manual enable followed by save or launch refuses, naming Monitor and `archo disable`. A selected Workshop copy works without a link (`TestMonitorExFAT`, `TestMonitorWorkshopOnlyExFAT`, simulated exFAT) | Pass |
| M6 | Build a plan containing Monitor | Include a Monitor copy in the pack and write its log under `<pack>/userdata/` (`TestMonitorInPack`; game verification needed) | Pass (copying) |
| M7 | A development `RIMARCHO_MONITOR` build has `-dev` and changed content | Replace it when no game runs; defer while a game runs (`TestMonitorDevReplace`, `TestMonitorDevPath`) | Pass |
| M8 | No Monitor is bundled (`go run`, no environment override) | Do not install a bundled copy; `archo monitor` reports the searched locations. An installed Workshop copy can still be added (`TestMonitorNotBundled`, `TestMonitorWorkshopOnlyExFAT`) | Pass |
| M9 | A check finishes after launch and swaps 0.9.0-dev to 0.9.1 while the game runs | Count the run's log after exit; the old link does not make the new version unloadable; the next launch updates the link; later changes of other kinds still make the log stale (`TestMonitorUpdateAfterLaunch`; real game in the release VM table) | Pass |
| M10 | Subscribed Workshop copy beside the bundled Monitor's link | Highest modVersion wins, Workshop wins a tie and the next save removes the old link; no installed-duplicate finding, one Monitor active in the plan and ModsConfig.xml (`TestMonitorWorkshopSelection`) | Pass (synthetic) |
