# Game test runs: RimDevTools research and measurements (2026-10-24)

Purpose: let RimArcho "launch the game with a given plan, reach the main menu, collect errors, close the game, and produce a report" (08 §12). First inspect how `D:\Game\RimModManager_Upgrade\RimDevTools` works (read-only), then test on two local game copies.

## 1. How RimDevTools works

License: MIT (`LICENSE`, 2026 Platinum trigger (translated)). Its code may be reused, but it is C# and we use Go. We borrowed ideas only; no code was copied.

| Step | RimDevTools approach | Code location |
| --- | --- | --- |
| Launch | `ProcessStartInfo` starts `RimWorldWin64.exe` directly with `-popupwindow`; auto/xml modes also add `quicktest` (enter a test map directly). Can run on an isolated desktop (`CreateDesktop`) without taking the foreground | `Infrastructure/GameProcessLauncher.cs`, `Commands/TestCommand.cs` |
| Isolate configuration/saves | **No isolation.** It changes the player's real `ModsConfig.xml` and its own tool Mod settings. `ConfigTransaction` backs them up first and restores them afterward; after interruption, `TransactionRecovery` restores them on the next launch. `EnvironmentLock` prevents concurrent runs | `Infrastructure/ConfigTransaction.cs` |
| Set plan | Read an external ModsConfig or project configuration, resolve dependencies, render and write the real `ModsConfig.xml` | `ModListParser` (inside `TestCommand.cs`) |
| Detect success | Its bundled **tool Mod** (LogSyncMod) logs `[LogSyncMod] RUNTIME_READY_TO_CLOSE` 10 seconds after `LongEventHandler.ExecuteWhenFinished`; a Harmony patch on `Root_Entry/Root_Play.Update` logs a `HEARTBEAT runId=… seq=…` every 5 seconds. Quick mode requires "another heartbeat after the ready signal" for success; 60 seconds without a heartbeat means `GAME_HUNG`; total timeout is configurable | `source/LogSyncMod/LogSyncMod.cs`, `RuntimeHeartbeat.cs`, `IncrementalLogSignalScanner.cs` |
| Read logs | LogSyncMod filters logs by PackageId/keywords into its own sync file. The CLI reads it incrementally (handling truncation and rereading) | `IncrementalLogReader` |
| Stop | `Process.Kill(true)` kills the process tree; if that fails, leave the configuration transaction unrestored and report `GAME_TERMINATION_FAILED` | `DeployCommand.cs` `ProcessUtil.TerminateAsync` |
| Result | One JSON result (success/failure code, runId, Mod list, dependency warnings); auto mode also has game-written `*_Result.json`/`*_Evidence.json` | `CliResult` |
| Preflight | Steam must be running; wait for any existing RimWorld process to exit first | `SteamSessionGuard` |

Ideas worth borrowing: heartbeats/hang detection, conservative cleanup that does not restore configuration if termination fails, one JSON result, and waiting for an existing game to exit.

Not adopted: changing the real `ModsConfig.xml` and restoring it (we isolate everything with `-savedatafolder`, leaving player files untouched and needing no transaction recovery); requiring a tool Mod to detect readiness (it cannot be installed when the game directory is read-only; see the alternative in §2.3).

## 2. Local measurements

Two copies: `D:\rimworld16` (1.6.4871 rev590, Unity) and `D:\SteamLibrary\steamapps\common\RimWorld` (1.7.4985 rev1269, Waylith engine). Both were launched read-only, with data directories under temporary directories.

### 2.1 Where the logs go

- **1.6**: `-savedatafolder=<dir>` moves only Config/Saves; Unity's Player.log does not follow. Add `-logfile <path>` (two separate arguments; do not quote the path again, or the file will be empty) to write to the specified location.
- **1.7**: the log location is hardcoded. Decompiled `RimWorld.dll`: `FileLog.SetDirectory(Bootstrap.ResolveEarlyLogRoot())`; the latter returns only `%USERPROFILE%\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`, **ignoring `-savedatafolder`**. At startup, it rotates `Logs\Player.log` to `Player-prev.log`. Running 1.7 with `-savedatafolder` therefore overwrites the player's real logs (this happened in the first probe: the player's original `Player-prev.log` was replaced).
  - Solution: add `-NoFileLog` (`FileLog.Disabled`; `SetDirectory` returns immediately without touching any log files). Logs also go to the console (`Microsoft.Extensions.Logging` console logger), so we capture stdout/stderr. `WindowsConsole.OpenIfRequested` leaves an already valid stdout handle alone, allowing the pipe to receive output.
  - Console and file formats differ: `HH:MM:SS  LEVEL message`, continuation lines `HH:MM:SS <original text>`, without date or category. Convert to file format (`=== Waylith log ===` header, `Version:` line, `date time  LEVEL message`) for normal parsing by `internal/logs`.
  - This also answers the unverified item in 01 §5.2 / 08 §5.3: **when a pack runs 1.7 with `-savedatafolder`, logs go to the environment's shared location, not `<pack>/userdata/Logs/Player.log`**. The existing pack code's assumption is wrong (recorded separately as follow-up work).

### 2.2 Is isolation complete?

The data directory contained `Config/{ModsConfig,Prefs,KeyPrefs,Knowledge,LastPlayedVersion}`, `HardwareProfile/`, `ShaderCache/`, and `TextureCache/` (about 27 MB of texture bundles on the first 1.7 run). Nothing was written to the player's LocalLow directory except the 1.7 logs (resolved with `-NoFileLog`). Nothing was written to the game directory. Unity's resolution and other PlayerPrefs are in the registry; these were not checked.

### 2.3 Detecting the main menu

Vanilla has no "loading complete" log line. Decompiling `Assembly-CSharp.dll` in 1.6 and 1.7 showed `UIRoot_Entry.Init()` → `MainMenuDrawer.Init()` → first statement `PlayerKnowledgeDatabase.Save()`, writing `<data directory>/Config/Knowledge.xml`. Delete it before launch; its reappearance marks arrival at the main menu, without any tool Mod. With Core only in 1.7, it appeared about 16 seconds after launch.

Test map: `quicktest` runs `Root_Play.SetupForQuickTestPlay`; `Game.InitNewGame` logs `Initializing new game with mods:`. Treat a subsequent quiet period in the log as map readiness (heuristic).

### 2.4 Other findings

- `D:\rimworld16\Mods` contains a dangling link made by RimArcho, `_Archo_env_yvae4hr7j2zwsjepwkqalnykwq` (to the deleted `…\_3700765558_alt_decapitate`). In 1.6, `ModLister` throws `DirectoryNotFoundException` when scanning it; `ModsConfig`/`ModLister` type initialization fails, followed by `Root level exception` every frame, producing 47k log lines. This copy currently **cannot reach the main menu even with Core only**. A test run can report this as failure (it appears in the error clusters), but the root cause is a link-maintenance bug, recorded separately.
- The 1.7 `ModsConfig` static constructor only renames missing Mods rather than deleting them. The game's rewritten ModsConfig therefore cannot tell us which Mods failed to load. Check "which Mods the game can see" before launch ourselves.
- Memory: 1.7 with Core only peaked at about 1.9 GB working set and settled at 1.4 GB; 1.6 used about 0.36 GB (failed loading). Reading process `PeakWorkingSetSize` is cheap and can be included in the report.

## 3. Conclusion (design in 08 §12)

Implement our own flow without a tool Mod: isolated data directory + version-specific log arguments (1.6 `-logfile`; 1.7 `-NoFileLog` + stdout capture) + `Knowledge.xml` main-menu marker + timeout/early-exit detection + process termination + existing `internal/logs` and `App.AnalyzeLog` for error clusters and attribution. Never write to the game directory or player data, so Mods invisible to the game (requiring links) are only reported; no links are created.

## 4. Real-machine run records (`archo test run`, 2026-10-24)

RimArcho used temporary `RIMARCHO_HOME` and user data directories (leaving the player's RimArcho data and game configuration untouched). Modification times of the player's LocalLow `Logs/*`, `Config/ModsConfig.xml`, and `Config/Prefs.xml` were compared before and after each run: all unchanged. The number of `_Archo_*` links in the game directory was unchanged (no links created).

| Game | Plan | Result | Loading / total time | Peak memory | Error clusters | Exit code |
| --- | --- | --- | --- | --- | --- | --- |
| 1.7.4985 rev1269 (Steam copy) | Harmony, Core, 5 DLCs, JobsLimitsTweak (8 total) | `menu` | 31.5 s / 37.0 s (first run, building caches); 13.5 s / 19.3 s (reusing caches) | 2188 / 2011 MB | 0 (only mod-report 3, other 6) | 0 |
| 1.6.4871 rev590 (`D:\rimworld16`) | Core (1 total) | `menu` (marker written, but main-menu initialization had failed) | 11.5 s / 17.1 s | 362 MB | 6 exception clusters: `ModsConfig`/`ModLister` type initialization failed (dangling link, §2.4), then `Root level exception` every frame | 1 |

Note: 1.6 reached the "main-menu marker" with fatal errors, so exit code 1 comes from the error clusters, not the status (08 §12). Chinese Mod names appear as `?` in the 1.7 console log (system ANSI code page, known limitation in 08 §12).

## 5. Follow-up

- Dangling `_Archo_env_*` link in `D:\rimworld16\Mods`: link maintenance should clean up or skip dangling links (08 §3).
- Incorrect 1.7 pack log-location assumption (§2.1): pack launches on 1.7 should also add `-NoFileLog` and capture output, or read the shared location in reports (08 §5.3).
- The player's 1.6 `Config/Prefs.xml` modification time changed once from 03:26, during the first 1.6 probe (whose `-logFile` argument had extra quotes). The second 1.6 probe and both formal runs left it unchanged. Cause unknown (possibly another concurrent session).
- MCP `test_run` holds the MCP server lock while running; other tool calls must wait for it to finish.
- GUI entry point not implemented.
