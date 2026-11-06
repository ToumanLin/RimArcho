# VM: real rounds with Archo Monitor phase markers (afternoon of 2026-10-28)

Branch `dev/0.9.0`. Guest builds successively `4d031c0`, `876e0cb`, `4cf327c`, `3ccb442`, and the B8 fix commit; Monitor 0.9.1. VM `Win11-Player-Test`, real home (state after snapshot `RimArcho-Real`), Chinese UI. Evidence under `D:\RimArcho-test\vm-phase` (screenshots, `phases16.txt`, `dl17*.txt`, `sz.txt`, `cats.txt`, `crash17-evidence.txt`, `sv3–7.txt`, `pl.txt`, B8's `b8live.txt`, `b8pl.txt`).

Game input injected inside guest using `SetCursorPos` / `mouse_event` (`vm.ps1 exec -Interactive`): host computer-use clicks do not reach inside VMware. RimDevTools automated testing was not used: it edits ModsConfig.xml, adds its tool Mod, uses developer quicktest instead of "new colony", and has no save/load steps, so would test a different flow.

## 1. 1.6 (1.6.4871, list phase-test, 18 Mods)

Completed a full round: main menu → new colony (Crashlanded, Cassandra, peaceful, random location, no ideology) → 3× speed until day 2, hour 22 → save "New Arrivals5" → load it → play to day 3 → main menu → quit to desktop.

- **Both logs have correct phase order.** ArchoMonitor.log: `session` (15:31:25) → `menu` (15:32:10, immediately after loading) → `new` (15:36:36) → `play` (15:38:12) → `save` (15:42:19, `context.save` = "New Arrivals5") → `play` (15:42:23) → `menu` (15:44:33) → `exit` (15:45:03, entries 5). Corresponding `[Archo Monitor] phase …` lines in Player.log are at 51, 54, 83,100,120,134.
- **Errors only during loading**, consistent with this 1.6 list: two kinds of [SSR] XML errors (3 occurrences), one MVCF cross-reference.
- `archo problems`: each log cluster carries `[loading xN]`.
- Can it run: "errors from the last run" card has only the "loading the game" group; empty phase groups are hidden.
- Game log: Archo Monitor tab has a "loading the game" separator, all 5 entries below it; right-side card proportions 62/38 and 40% are correct.

## 2. 1.7: download works, VM initially could not launch it

- **Download**: `archo env download unstable --to C:\Games\RimWorld-unstable`, cached login, 16.4 GB, about 37 minutes.
  - SteamCMD disconnected once: `steamcmd has been disconnected from steam with result 3 (No Connection)`, followed by `Retrying...`.
  - SteamCMD finally wrote `Success! App '294100' fully installed.`, but RimArcho reported "cannot reach Steam", exit 1, without adding environment. **B1, fixed** (`876e0cb`).
  - Rerun in same directory after fix: validation only, `env:1.7 downloaded and added`, 1.7.4990 rev1069.
  - SteamCMD login signed out the guest Steam client (known); recovered by `steam.exe -shutdown` and reopening.
- **Download also fetched all subscribed Workshop Mods (B2, unfixed, decision needed)**:
  - Game itself about 1 GB; `steamapps\workshop\content\294100` contains 1293 entries, 15.6 GB: all account subscriptions, recorded in `appworkshop_294100.acf`.
  - Per 03 §2 item 3, RimArcho registers this as shared `steamcmd` source. Every Mod gains a copy: 2595 instances, 1294 Lineages with multiple instances, over a thousand `pkg.duplicate-installed` findings. Visible in 1.6 too because the source is shared.
  - Related minor issue: missing-content providers appear twice (SteamCMD and Workshop copies); `archo log` next steps first recommend enabling the SteamCMD copy, while the list originally used Workshop.
  - 11 §3 previously marked "directory contents after download" unverified; now known.
- **1.7 crashes on launch without Vulkan**: "No Vulkan support was found on this computer" (`VK_ERROR_INCOMPATIBLE_DRIVER`). VMware virtual GPU has no Vulkan. Game tried three Vulkan configurations (stage 1–3), all crashed, generating `Crashes\crash-…165213/165354/165529.txt`.
- **RimArcho does not reveal the crash (B3, unfixed, decision needed)**:
  - Log lines resemble `ERROR [Vulkan] [General] [Loader Message] vkCreateInstance: Found no drivers!`. Removing category `[Vulkan]` leaves a message starting `[General]`, classified as "Mod self-report" (`modReportRe`). `Vulkan fallback stage N` is "other". Both are `Informational` and hidden.
  - Result: can it run lacks "errors from the last run"; launch page says "last game … no errors"; game log says "its errors are included in needs attention", contradictory. `archo log`: `list: (none)`, `hidden: 4 informational clusters`.
  - Actual cause appears only in `Crashes\crash-*.txt`, a directory RimArcho does not read.
  - After installing software Vulkan, several such `[General] [Loader Message]` lines still appear per launch and are listed as "Mod self-report" on game log.

## 3. 1.7 rounds with software Vulkan

With player permission, installed Mesa lavapipe in guest: pal1000/mesa-dist-win `mesa3d-26.2.4-release-msvc.7z` (67 MB, SHA256 verified), extracted to `C:\Mesa\x64`, registered ICD in `HKLM\SOFTWARE\Khronos\Vulkan\Drivers`. Host copy under `D:\RimArcho-test\mesa`. Game runs at very low frame rate; injected clicks must be held 700 ms to register.

- **Round 1 (17:13–17:58), full flow**: new colony → play → save "New Arrivals9" (17:45) → load (`save` 17:49:26, `play` 17:49:28) → play about 2 minutes → main menu (17:56:21) → quit to desktop (`exit` 17:58:09). Phase order correct as in 1.6. Errors only during loading ([SSR], MVCF), same as 1.6.
- **Round 2 (18:02–18:13), save-load errors**: disabled 5 hair Mods in phase-test (AFU men/women, Won men/women, Lolidrop), loaded "New Arrivals9". Game showed Mods mismatch listing exactly these 5; chose Load anyway. Loading produced 23 `Could not load reference to RimWorld.HairDef named …` occurrences (8 clusters, `[save xN]`).
  - During run and after exit, can it run grouped "loading the game / loading a save"; game log had "loading New Arrivals9 18:06" separator. Correct phase grouping.
  - `4cf327c` verified in a real run: "open game log" from "this run" selects the same cluster.
- **Found and fixed this round**:
  - **B4 wrong attribution**: all 23 were assigned to Vanilla Expanded Framework, saying "source not yet known". It only patches `Pawn.ExposeData`, traversed while loading. RimArcho had found installed but disabled hair providers (`archo log --cluster` `providers: … (not enabled)`), but placed them after patchers. Now disabled providers for `xref-missing` precede patchers, confidence likely. Can it run says "AFU Women's Hair (translated) — 11 warnings / referenced content does not exist; it is not in the list", one row per four hair Mods. Attribution cache version incremented; old logs reanalyzed.
  - **B5 page does not refresh**: while can it run was open, background-check completion updated only sidebar; game exit also did not reread. After round 1 exit it showed "17:49" (save loading); round 2 "18:06", changing to exit time only after leaving/returning. Both now redraw. Retest: game exited 18:54:11, next poll showed 18:54 without page switching (minimized window slows browser's 2-second poll to about once a minute).
  - **B6 repeated wording**: each live entry started "this run," though section already says "this run"; evidence repeated "this run, N times". Now "Won hair_men: 3× Could not load …", evidence shows log filename only.
- **B7 same errors listed twice during run (fixed, §4, §4a)**: can it run's "last run" read Player.log while game was writing it, duplicating current errors/counts and phase subheadings. First added "last run  time" subheading, then changed the root behavior per player decision: actively written logs do not count as problem logs.
- Rounds 3, 4 (18:35–18:54) retested installed fixes, only reaching save loading or main menu.

## 4. Other fixes this round (all on `dev/0.9.0`)

- `7235e14`: can it run evidence used `ArchoMonitor.log  Defs/…/Apparel.xml  <time>`; window collapsed spaces, making it look like one path. Now "file Defs/…  ArchoMonitor.log  time"; window splits at double spaces with 12px gaps (16 §7.3).
- `49a7e1e` + `4cf327c`: while game runs, "this run" entries also use "open game log", locating corresponding cluster by first line (live cluster IDs are batch-based and differ from log page).
- `876e0cb`:B1.
- `69964c2`: B4, B5, B6, B7 subheading.
- `3ccb442`: B7. While game runs, its actively written current-environment log (`App.LiveLog`) is not a problem log; last run reads Player-prev.log instead, including read-only sidebar-count caches. Game log says "the game is still writing it: included in needs attention after exit".
- `a0c3682`: B8. If Monitor arrives first in live tracking, wait for Player.log to finish that entry, merging late arrivals (§4c).
- Next commit: live "is it in Player.log?" checks also include noise lines (§4c).

## 4a. Real B7 validation (1.6, after restoring `RimArcho-Real`)

Default list plus Harmony, Archo Monitor, [SSR], and its three dependencies (12 Mods); [SSR] logs 3 errors, 1 warning during loading.

- First run (19:22–19:24) main menu then exit, establishing "last run".
- Second run (19:26–19:43) full flow: new colony (Crashlanded, Cassandra peaceful, reload anytime, ideology off) → play → save "New Arrivals10" → load → play → main menu → exit.
  - While running: `archo problems` log line "Player-prev.log … 19:24, current (from the launch … 19:22)". Can it run lists current run then "last run  list  Oct 28 19:24", without duplicating current run.
  - Sidebar count previously included only current run, missing last run (read-only-cache path could not read reports during play, predating changes). Reading Player-prev.log too yields sidebar 5 = section 5 (last run 1 + current 4).
  - After exit, without page switching, can it run updates to "list Oct 28 19:43", sidebar 1 = section 1; `archo problems` returns to Player.log.
  - Launch again to main menu: game log marks Player.log "the game is still writing it: included in needs attention after exit", no include button.
- **B8 live tracking splits one error into two (fixed, §4c)**: in second run, "this run" showed [SSR] `Could not resolve cross-reference … Reload_Standard` twice, evidence Player.log and ArchoMonitor.log. Restarting RimArcho and reading the whole log left one. Apparently the companion entry arrives before its Player.log line, forming a Monitor-only cluster, never merged when that line arrives. Predates changes.
- Minor: live "this run" included unattributed `Translation data for language English has 9 errors` (evidence ArchoMonitor.log), excluded by full analysis after exit.

## 4b. Test saves entered the real Steam cloud

This VM uses the player's real Steam account; RimWorld `Saves/*.rws` uses Steam Auto-Cloud (guest Steam `userdata/1026722804/294100/remotecache.vdf`, 205 .rws entries). Therefore:

- Today's test saves "New Arrivals5" (1.6), "New Arrivals9" (1.7), "New Arrivals10" (1.6) are in the cloud, synchronized back after snapshot restoration, and will appear on the player's own computer.
- During second run, game's autosaves wrote Autosave-4 (19:35), Autosave-5 (19:37), overwriting those cloud slots' original contents irrecoverably.
- Player decides whether to delete test saves; none were touched. Future rounds should disable autosaving in game options first and save only under new names.

## 4c. B8 fix and real validation (1.6, after restoring `RimArcho-Real`)

Cause: in this 1.6 VM, Player.log has no stacks; each error is one line and parser waits for the next line to know it ended. After [SSR] `Could not resolve cross-reference … Reload_Standard`, the next line (`Translation data …`) arrived about 19 seconds later. Monitor was already read; after 5 seconds without a completed matching Player.log entry, it became Monitor-only. Player.log completion then grouped under another key, making two.

Fix (`internal/app/livelog.go`):
- Continue waiting if it is Player.log's actively written final entry, or Player.log still has unread content beyond one read.
- If Player.log arrives after waiting, merge into the Monitor-only entry: use Player.log as first sample, reattribute, and change evidence back to Player.log.
- Recompute count from Player.log each time, then take the larger of it and Monitor count. Previously without phase markers counts accumulated: Monitor's 3 plus Player.log's later 4 showed 7. Synthetic `TestLiveLogMonitorFirst` fails on pre-change code.

Real validation: same 12-Mod list, launch to main menu and exit, read "this run" (`API.Live`) every 3 seconds during loading. Game debug log places this error at 20:32:36, next line 20:32:55, exactly B8 timing. "This run" had one entry throughout, evidence Player.log, count 1; no Monitor-only entry appeared between 20:32:41–20:32:52. After exit `archo log` also had one cluster (E6 `xref-missing x1`).

Two other observations in the same run:
- Fixed (commit after B8): `Translation data for language English has 9 errors` still appeared live with ArchoMonitor.log evidence. Player.log has it too, but live checks considered only errors; this line is noise in Player.log, whereas Monitor's stack let it pass filtering. Full analysis considers every Player.log entry, so excludes it after exit. Live tracking now also records nonerror Player.log lines (hashes only, up to 50,000), suppressing the same Monitor entry (`TestLiveLogMonitorNoise`, not retested in VM).
- Fixed (`a91adc0`, real VM validated): launched immediately after restore; RimArcho's first scan (1302 Mods, 27 seconds) finished after launch, then replaced Archo Monitor 0.9.0-dev with bundled 0.9.1. List changed after launch, making its log stale after exit (`stale log … not counted`); can it run showed only "older log". Two installed Monitors also triggered `mod.unloadable` in `archo problems`: game recognizes only one. B7's run launched after scan, avoiding this. Established causes:
  - Not test setup: build `monitor/ArchoMonitor` copied from `D:\RimArcho-test\vm-phase\ArchoMonitor`, byte-identical; one Mod folder, packageId `toumanlin.archomonitor`, version 0.9.1 (`monitor/VERSION` also 0.9.1); snapshot had 0.9.0-dev. Players upgrading RimArcho with old Monitor follow the same path.
  - Staleness: window-open Monitor check (`monitor install` subprocess) copies 0.9.1, full scan (27 seconds), then swaps Monitor in plans. Game launches earlier with a fingerprint containing 0.9.0-dev; after swap it differs, counted by 06 §4 item 3 as "plan changed after launch". Fix: Monitor-only version swaps do not count (`App.monitorSwapOnly`). Not deferred until game exit (that swap would still stale this run) or moved to launch (0.9.1 not scanned yet); see 00 §6 D21.
  - Two installed copies are intentional (19 §6 retains old-version directories). Game loaded 0.9.0-dev: only `_Archo_monitor_ArchoMonitor-0.9.0-dev` was linked at launch. `mod.unloadable` was false: "game-visible" set included plan 0.9.1 (linked on next write) plus old 0.9.0-dev link still in `Mods/`; folder `…0.9.0-dev` sorts before `…0.9.1`, making new version ignored. Next write actually removes old link before adding new, so both never coexist in `Mods/`. Old links outside the plan are now excluded when the plan contains another copy with that package ID (`deploySet`, 09 §3).
  - Synthetic `TestMonitorUpdateAfterLaunch` follows the observed order: install 0.9.0-dev → launch → update to 0.9.1 while running → write log/exit → launch again. Pre-change code fails both checks (`Changed: true`; `mod.unloadable`).
  - Real validation (`a91adc0`, restore `RimArcho-Real`, same 12 Mods, evidence `D:\RimArcho-test\vm-monitor-swap`): open window 21:16:30, click home launch 21:16:36 (v4, 0.9.0-dev), history `update Archo Monitor 0.9.0-dev → 0.9.1` at 21:17:07, matching original timing. After main-menu exit, `archo problems` says `log Player.log … current (from the launch of plan:default at 2026-10-28 21:16): 2 error, 1 warning`, no `mod.unloadable`; can it run's "errors from the last run" is this run (21:21, [SSR] 3 errors, 1 warning). `save --dry-run` lists `- _Archo_monitor_ArchoMonitor-0.9.0-dev`, `+ _Archo_monitor_ArchoMonitor-0.9.1`. Next launch: only 0.9.1 link in `Mods/`, Player.log `[Archo Monitor] 0.9.1`, `phase menu`; log still current after exit. Both reached main menu only, no saves. Restored `RimArcho-Real` afterward, VM off.
  - Setup note: `vm.ps1 build` copies Monitor from latest `dist/archo-*` (now 0.9.0-dev). To test bundled new version, replace `dist/vm/RimArcho/monitor/ArchoMonitor` after build before packaging; both rounds did so.
  - Remaining: `archo problems` info `pkg.duplicate-installed  2 copies of pkg:toumanlin.archomonitor are installed`. Happens after every Monitor update (old directories retained per 19 §6), unrelated to timing; only technical info/CLI, not can it run.

## 5. Open decisions

- B2: handling Workshop directory after game download.
- B3: displaying 1.7 engine ERROR lines and crash reports.
- Launch before scan finishes makes Monitor update stale the run: fixed, verified in real 1.6 VM (§4c). Whether to suppress old-Monitor `pkg.duplicate-installed` info remains open.
- Whether to delete Steam-cloud test saves (§4b).
- Untested: grouping actual play-phase errors (this list has none while playing), TUI, English UI.
- Restoring `RimArcho-Real` removes 1.7, lavapipe, and these saves; retesting 1.7 needs lavapipe reinstallation (host `D:\RimArcho-test\mesa`) and 1.7 redownload (about 16 GB, 15.6 GB B2 Workshop content).
