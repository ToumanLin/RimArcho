# VM: archo.db size, migration, and log ownership (2026-10-29)

Completed main A–F checks, found 1 upgrade-output regression; G stopped at main menu because the game lacks an autosave-off UI, so full flow untested. Testing only, no App or test-tool fixes. Branch `codex/vm-db-round`, target commit `6e062a999941b3c0e18ec19d82793dee361dc0fa`. Scope/stop conditions from `D:\RimArcho-test\vm-db-round\BRIEF.md`. Evidence in same directory; no real database copied to host.

## 1. Environment and method

- VM: `Win11-Player-Test`, user restored `RimArcho-Real` and started it; this round did not operate snapshots. Player logged in, Steam running.
- Real home: guest `C:\Users\Player\AppData\Roaming\RimArcho`, database `archo.db`. Environment `Steam`, game `C:\Program Files (x86)\Steam\steamapps\common\RimWorld`, shared default user data directory.
- Old build: `a201344a0d37`, schema **18**. Registered game `1.6.4871 rev590`, actual Player.log/game `1.6.4871 rev591`; not assumed schema 24, recorded as measured.
- New build: `0.0.0-vm (6e062a999941)`, debug information retained; CLI SHA-256 `04865BB6E60B26EF147FD242D84DE19EDE0C191A49DEEE7A4D4E3BC40B9C40B1`, GUI SHA-256 `903776543125CF42E81CAC3180D236855DD04272780E40500737041BD327BE36`. Bundled fonts/Monitor from existing main-repository `dist/archo-0.9.0-dev-windows-amd64`, Monitor 0.9.1.
- To keep host artifacts under the evidence directory, builds went to its `build/`, using `vm.ps1 build` compiler arguments. Ran `push`, CLI migration, then `launch`, avoiding `go` which opens GUI first. vm.ps1 unchanged.
- `sqlite3` absent from guest PATH; used existing `C:\Users\Public\Documents\VM-Test\phase1a\node\sqlite3.exe -readonly`. Checked database existence first each time; retrieved UTF-8 text only.
- GUI used real mouse input through `cdp.mjs`; game input inside guest through `SetCursorPos` / `mouse_event` / `keybd_event`, screenshots through VMware wrapper. Console helpers hid their windows and checked RimWorld was foreground before every input.
- Database-size tables use bytes, MiB = 1,048,576 bytes; App notices retain original MB.

## 2. A: old-version baseline

Expected: preserve real facts, caches, and CLI output before replacing build; validate migration with a Def/DLL test plan.

Initially found 80,355 Defs with existing analysis facts, so no analysis rerun needed for initial 6-Mod plan. With old CLI, created/switched to `dbtest` from current, added Harmony, HugsLib, Vanilla Expanded Framework, Vanilla Furniture Expanded, existing Archo Monitor, sorted/saved. Total 11 Mods: those 5 plus Core and 5 DLCs. Final old analysis 1.278 seconds; saved baseline below.

| Metric | A: before upgrade | B: migration only | B: first new analysis | C: after adding 1.7 analysis |
| --- | ---: | ---: | ---: | ---: |
| Database bytes | 58,089,472 | 64,237,568 | 64,237,568 | 64,237,568 |
| WAL bytes | 0 (file absent) | 0 (file absent) | 0 (file absent) | 0 (file absent) |
| page_count | 14,182 | 15,683 | 15,683 | 15,683 |
| freelist_count | 662 | 5,657 | 4,966 | 4,142 |
| schema | 18 | 26 | 26 | 26 |
| fact_set | Not applicable | 1,275 | 1,282 | 1,531 |
| def_fact | 80,380 | 80,380 | 80,468 | 97,851 |
| type_fact | 4,114 | 4,114 | 4,114 | 7,977 |
| snapshot_file | 239,350 | 239,350 | 239,350 | 239,350 |
| log_cache | 1 | 1 | 1 | 1 |
| sum(length(report)) | 2,738,332 | 218,162 | 218,319 | 218,319 |
| First two report bytes | `7B22` | `1F8B` | `1F8B` | `1F8B` |

Before plan changes, database was 53,633,024 bytes, page_count 13,094, freelist_count 0, snapshot_file 239,350, log_cache 1, report 2,737,033 bytes. Do not directly compare these with migration after enabling 5 Mods.

Evidence: `a-inventory.txt`, `a-prepare.txt`, `a-baseline.txt`, `a-{problems,def,patches,harmony,why,log}.txt`.

## 3. B: migration and output equivalence

Expected: first new-CLI open upgrades to schema 26, compresses old reports, removes old indexes, creates fact sets, preserves query content/order.

First `db status` open took **1,708 ms**, exit 0. After CLI closed, database 64,237,568 bytes. Command printed `database 75.7 MB` while running; not the main-file size after closing connections. v26 does not auto-VACUUM (05 §3), so main file temporarily grew 6,148,096 bytes, leaving 5,657 free pages, later reclaimed by automatic maintenance (§5). Test-table "database smaller after upgrade" cannot mean immediate physical shrink on migration-transaction completion.

`fact_set` exists; `def_fact_name`, `type_fact_name`, `type_fact_instance`, `harmony_fact_instance`, `harmony_fact_target` absent. Final Name index is v26 `def_fact_named`; intermediate v25 `def_fact_mm_named` is superseded by v26 and need not remain. gzip compresses original report to **7.97%** (about 92.0% reduction).

| Comparison command (actual syntax) | Actual result |
| --- | --- |
| `problems --all` | Main analysis still 0 error / 0 warning / 8 info; old log still 347 error / 296 warning, uncounted; first query rebuilt analyzer 7 → 8 (20,162 ms, analyzer internal 18.3 seconds). Log finding order and two IDs changed, see B1 |
| `def Steel` | Byte-identical file, new version 69 ms |
| `patches --mod pkg:vanillaexpanded.vfecore` | Byte-identical file: 32 operations / 0 failed, new version 62 ms |
| `harmony` | Byte-identical file: 500 patches / 14 assemblies, new version 68 ms |
| `why pkg:vanillaexpanded.vfecore pkg:brrainz.harmony` | Byte-identical file, new version 65 ms |
| `log` | 647 clusters, same categories/counts; E6 summary's first provider changed, see B1; new version 108 ms |

BRIEF's `patches <Mod>` and `why <Mod>` abbreviate syntax; actual CLI needs `--mod` and two Mod arguments. Both versions used actual syntax above; early invalid-argument output was not comparison baseline. Windows PowerShell 5.1's `NativeCommandError` location lines wrapping native stderr are evidence-wrapper output; reanalysis itself exits 0, so wrappers are not App errors.

### B1: upgrade changes primary attribution, finding IDs, and order for missing-parent-template logs

Status: **fixed, awaiting VM retest** (`6c67849`: `Store.DefsNamed` orders by Instance ID then file, no longer row number; synthetic `TestDefsNamedOrderIndependentOfWrites`). Original record: observed output regression. Cause cannot yet be attributed solely to v26 migration, because first query also rebuilt analyzer-7 cache for analyzer 8; this qualification remains, without claiming an isolated schema-only repro.

Repro: from this round's old snapshot, use old version to create/save the 11-Mod plan above and save `problems --all`/`log`. Without changing Mod files/plan, replace build, run `db status`, repeat both queries.

Expected: identical log and fact-query content yield identical candidate presentation and finding references.

Actual:

1. First dependency for `log:E6` (missing `Bomb`) changed from `#ecpc Vanilla Arsenal` to `#rcm2 Impact Weaponry - Reloaded`; both still say `(dependency, +1)`, candidate count unchanged.
2. E6 finding ID changed `p:92d9ba` → `p:719e84`; E237 (missing `Plant_Bush`) changed `p:727121` → `p:d4d4a8`.
3. Start of `problems --all` log section changed **E36 → E238 → E6 → E237** to **E237 → E6 → E36 → E238**; other counts unchanged.
4. Reading retained original log afterward on new version put Vanilla Arsenal first again in detailed E6 attribution, with both candidates still present. Output did not preserve BRIEF-required order/content consistency.

Read-only code review: `Store.DefsNamed` orders fact `rowid` (`internal/store/analysis.go`); missing-parent attribution adds dependencies in that order, using stable role sorting (`internal/app/logs.go`). Candidate display may therefore depend on write layout; a causal lead, not an isolated root-cause conclusion.

Evidence: `a-log.txt` / `b-log.txt`, `diff-log.patch`, `a-problems.txt` / `b-problems.txt`, `diff-problems.patch`, `b-detail-and-prefs.txt`, `e-history-and-provider.txt`. No code changed this round; the two ID changes were not counted as separate bugs.

## 4. C, D: cross-version sharing and WAL

C expected: 1.7 adds only content-different sets, with normal queries.

`analyze --game-version 1.7` **4,895 ms**, exit 0; `problems --game-version 1.7` **309 ms**, exit 0. Each version has 1,271 Def views: **1,025 point to the same sets, 246 differ**. fact_set +249 (Def +246, DLL +3), def_fact +17,383, type_fact +3,863; main database growth **0 bytes**, reusing free pages, 4,966 → 4,142. No sets unreferenced by content_state.

D expected: keep GUI open; WAL ≤ 16 MiB after analysis triggered by real controls.

Clicked "check files again" on can it run, waited for "analysis complete". With window open, `archo.db-wal` was **0 bytes**. First real GUI scan and Monitor 0.9.0-dev → 0.9.1 update changed enabled set `f6caa3d9c88929da6af477d7` → `cec0ffbb691cc5da1b48af9b`; old 1.7 slot was subsequently cleaned as unreferenced. This is an actual plan-reference change, not lost cross-version sharing. C comparison preceded first GUI scan.

After scan: `snapshot_file` 266,878; Def 80,711, Type 4,137. Online Steam Workshop changes and Monitor update were excluded from B upgrade comparisons.

Evidence: `c-second-version.txt`, `c-analyze17.txt`, `c-problems17.txt`, `d-wal.txt`, `e-before-idle.txt`.

## 5. E: automatic maintenance and compact entry

Expected: idle maintenance after GUI startup, completion timestamp, migration-page reclamation; real "compact" action reports reclaimed/current size; refuses while game runs.

| Point | Database bytes | WAL bytes | Free pages | Completion time (UTC) |
| --- | ---: | ---: | ---: | --- |
| GUI analysis complete, before maintenance | 64,237,568 | 0 | 4,168 | None |
| After first automatic maintenance | 40,882,176 | 0 | 0 | `2026-10-29T08:15:36Z` |
| After manual "compact" | 40,882,176 | 0 | 0 | `2026-10-29T08:16:45Z` |

Automatic maintenance freed **23,355,392 bytes (22.27 MiB)**. Actual manual notice: `database compacted: freed 0 B, now 39.0 MB` (translated). With game process 6152 running, button remained clickable, then reported `cannot compact database: game Steam is running (process 6152); close it first` (translated), without maintenance.

There were 76 old update events, but **0** updated events whose old before-manifest was cleaned, so no real historical sample showing "(file list cleaned up)". This branch cannot count as passed; no real Mod was damaged to create a sample.

Evidence: `d-wal.txt`, `e-before-idle.txt`, `e-after-manual.txt`, `e-vacuum-completed.png`, `e-vacuum-ui.txt`, `e-running-refusal.png`, `e-running-refusal-ui.txt`, `e-history-and-provider.txt`, `f-prepare.txt`.

## 6. F: log ownership with shared user data directory

Expected: 1.6 excludes logs whose paths clearly belong to another registered environment; explicit path/version cannot bypass; own prev fallback; new environment accepts own log; removing environment cleans its facts.

Created guest test `fake-game17` with `Version.txt` (1.7.4990 rev1069), Data/Core/About.xml, and one DamageDef XML from real Core, solely for registration/fact cleanup, never launching fake game. Registered `dbforeign17`, sharing real user data with Steam environment. Backed up original Player.log and existing Player-prev.log (if any) in guest, then changed real log Mono path to fake directory and `RimWorld …` version line to 1.7.

| Check | Actual result |
| --- | --- |
| 1.6 `problems --all` | Other-environment log excluded from log section/counts |
| Default 1.6 `log` / GUI log page without own prev | CLI reports no usable Player.log; GUI "Player.log not found"; can it run eventually "nothing needs attention here" |
| 1.6 explicit `log --file <current log>` | Exit 1: `log belongs to environment dbforeign17` |
| Same plus `--game-version 1.7` | Same refusal, exit 1 |
| 1.6 analysis slots | Still only two 1.6 slots; no 1.7 slot created |
| Add Steam-owned Player-prev.log | Default CLI/GUI fall back to prev, 647 clusters; current 1.7 file still excluded |
| Switch GUI to dbforeign17 | Shows Player.log, 1.7.4990 rev1069, 1435 lines; CLI accepts too, 647 clusters |
| `env remove dbforeign17 --yes` | Private instances 1 → 0, fact sets 2 → 0, Defs 14 → 0; version views and analysis slots 0, no orphan sets |

Initial version-line construction replaced lines starting "1.6", failing to match actual `RimWorld 1.6...`; corrected after checking and reran whole group. Resulting initial 1.6 slot in fake environment was a fixture mistake, not App bug, cleaned on environment removal. Final judgments use `f-corrected.txt` and `f-cli/*-corrected.txt`.

Restored original logs afterward; fake environment removed, current environment back to Steam. No Steam subscriptions, unsubscriptions, or account logins changed.

Evidence: `f-corrected.txt`, `f-cli/`, `f-steam-no-own-log.png`, `f-steam-needs.png`, `f-steam-prev.png`, `f-foreign17-log.png`, `f-remove-and-restore.txt`.
## 7. G: full game round blocked, not counted as passing

Launched real 1.6 from GUI to main menu with Monitor 0.9.1 loaded, and completed running-game compact refusal (§5).

BRIEF explicitly requires "disable autosaves in game settings first" because VM Saves sync to real Steam cloud. Actual General options have only intervals 0.5 / 1 / 3 / 7 / 14 days and Autosaves count minimum 1. Developer mode adds shorter debug intervals only; no off option found in Dev/Debug Settings either. Thus no colony entered and no saves created/overwritten; this round cannot count as full game-log-flow acceptance.

Proposed alternatives: with user confirmation, set guest Prefs.xml interval to 999999 days and restart, or confirm 14 days with bounded play time. Without confirmation, followed BRIEF's "environment blocked, user intervention needed" stop condition; elapsed time was not treated as consent.

While searching, temporarily enabled developer mode and changed autosaves count to 1, then restored in game UI to **developer mode off, count 5, interval 1 day**. Developer mode's `Dev quicktest` shifted main-menu positions; while restoring settings, mistakenly opened an old-save mismatch confirmation. Did not click Load anyway / Load mod list; chose Go back and closed the load window. No old save actually loaded/saved. Test-operation mistake, not App bug.

Evidence: `g-taskbar-active.png`, `g-autosave-menu.png`, `g-autosave-min.png`, `g-autosave-devmenu.png`, `g-dev-settings.png`, `g-debug-search-result.png`, `g-options-restored-corrected.png`. `g-autosave-off.png` records an initial ineffective slider click, not successful disabling.

**Still needed**: new colony → several minutes of play (pause, speed up, open windows) → save under a new name → load it → normal exit → compare Player.log / ArchoMonitor.log and "errors from the last run"; then recheck gzip/WAL. Main-menu log checks below are partial coverage only.

### 7.1 Partial checks after normal main-menu exit

Confirmed `Quit to OS` from current screenshot and clicked; game exited normally at **08:54 UTC**, no RimWorldWin64 process remained. Player.log **14,372 bytes / 309 lines**, CLI **3 informational clusters, no errors/warnings**; GUI Player.log listed these three loading-phase entries. ArchoMonitor.log **864 bytes**, same session `37d3c50a27a4` records session → menu → exit, `context.entries=0`; GUI Monitor tab "no errors in this run". Can it run "dbtest can run; nothing needs attention", without invented play/save/load phases. `problems --all`: 0 error / 0 warning / 9 info; extra info is two installed Monitors, not a game-run error.

With GUI open, final database **46,596,096 bytes**, WAL **6,208,872 bytes (5.92 MiB, below 16 MiB)**, page_count 11,376, freelist_count 937. Three log_cache reports start `1F8B`, lengths 1,475 / 217,659 / 217,742 bytes. This follows F environment creation/removal and real scans, not the same point as B migration-only size. Read-only Prefs.xml check: 1 day, count 5, devMode=False; Steam only registered environment.

Evidence: `g-menu-final.txt`, `g-menu-only-Player.log`, `g-menu-only-ArchoMonitor.log`, `g-menu-only-cli-log.txt`, `g-menu-only-problems.txt`, `g-menu-log.png` (Monitor), `g-menu-player.png`, `g-menu-needs.png`, and matching UI text.

## 8. Tested scope moved from VM checklist

| Original test item | Tested this round | Remaining |
| --- | --- | --- |
| v26 cross-version fact sharing, old-fact/slot cleanup | Old DB migration, six CLI comparisons, 1.6/1.7 shared/different sets, old-slot cleanup after real Monitor update, private-fact cleanup on fake-environment removal | B1 output regression retest after fix; same-version folder-condition/priority isolation, old-plan removal, current-version retention/other-version expiry/rebuild, first-log/page performance not isolated |
| Schema 25 compression and WAL | Actual schema 18 → 26, gzip, indexes, matching CLI cache-read counts, WAL = 0 with completed GUI analysis/window open | Game logs/WAL/gzip after full G flow |
| Shared user-data log ownership | Fake 1.7 header + real 1.6 environment; CLI/GUI exclusion, explicit-path/forced-version refusal, prev fallback, no log, new environment accepts own log | Two real games alternating full launch/exit, MCP, old logs after same-directory game upgrade, live overwrite, each count/notice entry |
| Periodic maintenance and compaction | Idle automatic maintenance, completion marker, physical reclamation, manual GUI notice, refusal while game runs | Isolated 7-day cycle, skip during task/page reads and cancel in progress, MCP busy/quiet retry, CLI/MCP entries, insufficient space, cleaned old-history manifest display, retained-set integrity |

Checklist retains only right-column scope; passing results here do not replace remaining acceptance.

## 9. Automated checks

- `node --test "scripts/vm/*.test.mjs"`: **42 / 42 passed**.
- `go test ./...`: **failed**, sole failure `internal/surface/gui.TestSubscriptionReadDoesNotBlockEditsAndRechecksAfterOvertake`, `subscriptions_test.go:221: editor waits for a slow read`; other packages passed. No code changed.
- Targeted rerun `go test ./internal/surface/gui -run '^TestSubscriptionReadDoesNotBlockEditsAndRechecksAfterOvertake$' -count=1`: **passed** (2.233 seconds). Does not rewrite the first full-suite failure as passing; recorded as concurrency/timing to investigate, not another confirmed VM App bug.
- Cleared host test-process RIMARCHO_* variables first, using default synthetic repository fixtures only; no host App opened real home.

Evidence: `go-test.txt`, `go-test-recheck.txt`, `node-test.txt`. Specifications/CHANGELOG unchanged because product behavior unchanged; commit includes this report and VM checklist only.
## 10. Cleanup state

Ended under BRIEF environment-blocked condition: main A–F checks complete, B1 unfixed, G full flow untested; no colony entered or saves written. Game exited normally from menu; guest Windows shut down normally with `shutdown /s /t 0`; host `vmrun list` confirmed **Total running VMs: 0**. No snapshots restored/created/deleted. Test plan `dbtest`, new build, and guest artifacts remain; user handles later restoration.

Host evidence `D:\RimArcho-test\vm-db-round\`: cleanup count 244 files, 263,737,362 bytes (about 251.5 MiB, below 1 GB; excludes tiny later cleanup notes). No real DB copied. Commit contains this report and four checklist rows' remaining scope only; no push/merge.
