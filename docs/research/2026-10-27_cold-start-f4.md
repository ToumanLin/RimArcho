# Cold-start F4: host synthetic-scale measurements (2026-10-27)

Three reproducible causes are fixed: duplicate Board building, queued stale detail reads during cursor movement, and status animations forcing synchronous layout. Five-round incremental comparisons give cold-ready **2701.7 → 2514.8 → 2554.0 → 2538.7 ms**; subsequent alternating baseline/final runs, five each, give **3279.8 → 3103.7 ms**. Median per-round maximum sampling interval during full interaction: **917.7 → 284.4 → 232.3 ms**, with no >500 ms intervals in the final five rounds.

**The VM's 5.2–5.5 s has not been reproduced on the host; real-data F4 acceptance still awaits phase 2.** The synthetic DB is only 3.88 MiB and lacks real analysis facts, history, caches, and disk layout. No real RimArcho AppData DB was read; no real game/Workshop/inventory/AppData copied; VM untouched; no Steam/SteamCMD clients, subscription changes, or game launches.

Boundary mistake: early synthetic Workshop entries triggered public metadata network reads. That round was stopped on discovery; 5c820df fixes scale-fixture workshop.fetch to off and adds a test. All main tables and diagnostics here were rerun with the same offline seed; early results were discarded. No real user data was accessed, but those early reads violated this task's offline boundary.

Branch `codex/f4-coldstart`, worktree `.claude/worktrees/f4-coldstart`, baseline `3f7ecd1`. Functional fixes: af0633c, c13a991, bad5094; measurement completion: 8f66388. Baseline also enables main API/render instrumentation with pre-fix functional code; window runtime-ready and theme spans were added later, so absolute window milestones list only final results.

## Definitions and reproduction

Based on [Phase 1a F4 and timing table](2026-10-27_vm-automation-phase1a-results.md), [page cost §5–7/P6](2026-10-26_page-load-time-and-optimization.md), and [batch-operation audit](2026-10-27_batch-operation-timing-audit.md), checked against Shell/Home, workspace open, and list-render specifications.

- Reuses `scripts/vm/run.mjs --scenario 1 --driver W`: new process → Esc closes startup directory notice → open default list → original stable. `--cold-only true` skips later editing only, without changing stability counts, polling, or verdicts.
- Host Ryzen 9 7950X, 32 logical cores, 64 GiB, Windows 11, WebView2 155.0.4283.62, Node 22.17.0, Go 1.27.2.
- `D:/RimArcho-test/f4/seed`: 4000 live instances, 481 bound enabled entries, 3 lists, 2 game versions 1.6/1.7. Mods contain only About.xml/empty Defs; empty game exe is never executed; nonexistent packs directory triggers the Esc notice.
- Each round makes a home only by SQLite backup of the synthetic seed, shares synthetic Mod folders, and uses a new WebView profile. Runner's historical result.fixture label still says real-copy (meaning only --baseline supplied); actual provenance is confirmed by the synthetic seed path/scale above. OS file cache was not cleared: process-cold startup, not machine-cold startup.
- All outputs under `D:/RimArcho-test/f4/`; per-round case/profile removed, traces/checkpoints compressed, duplicate evidence deleted. Final bytes at end, strictly below 2,000,000,000.
- All figures are **five-round medians [minimum–maximum], ms**, no P95. Nested/concurrent spans cannot be added; row medians do not sum to a total median.

```powershell
# New seed path, no real AppData reads; scale disables Workshop fetch by default
D:/RimArcho-test/f4/tools/vmstate.exe --mode fixture --scale --root D:/RimArcho-test/f4/new-seed
node scripts/vm/run.mjs --scenario 1 --driver W --baseline D:/RimArcho-test/f4/new-seed --tools D:/RimArcho-test/f4/tools --out D:/RimArcho-test/f4/new-runs --perf true --cold-only true --archive true --cleanup true
# Remove --cold-only true for full interaction; use --profile true or --timeline true for independent CPU/timeline diagnostics
```

## Incremental cold-start comparisons

Original runner starts timing before port detection, about 34 ms before actual spawn; both are reported for Phase 1a comparability. Stable oracle repeatedly requires 5 equal state samples at 50 ms polls. editor() first clicks parent "lists" and stabilizes, then default and stabilizes; the final roughly 0.85 s includes two waits, not 0.85 s of list drawing.

| Adjacent stages | Original | After Board | After details | After status animation |
| --- | ---: | ---: | ---: | ---: |
| spawn → DevTools connectable | 257.5 [256.9–826.1] | 267.4 [255.1–811.0] | 271.4 [253.3–801.6] | 261.2 [251.7–824.9] |
| DevTools → Shell current list | 417.0 [373.3–433.4] | 415.1 [375.8–452.5] | 425.6 [370.2–464.5] | 411.8 [381.7–447.6] |
| Shell → Home/startup notice stable | 705.2 [692.1–707.5] | 556.1 [473.4–581.7] | 553.9 [471.0–643.1] | 540.9 [477.9–550.0] |
| Home stable → Esc sent | 23.2 [22.9–23.6] | 23.0 [21.6–23.7] | 22.4 [21.7–25.8] | 21.9 [20.2–25.1] |
| Esc → notice gone and stable | 393.6 [391.9–395.9] | 393.6 [392.3–395.0] | 394.7 [389.2–397.7] | 396.8 [392.2–402.3] |
| Notice gone → editor stable | 875.8 [845.3–884.1] | 863.9 [849.1–872.6] | 848.3 [844.3–859.0] | 856.0 [837.5–870.7] |
| **Actual spawn → stable** | 2667.4 [2599.7–3243.7] | 2480.1 [2447.7–3093.5] | 2520.2 [2453.5–2993.8] | 2504.9 [2447.5–3022.8] |
| **Original runner cold-ready** | 2701.7 [2633.9–3278.5] | 2514.8 [2481.6–3129.5] | 2554.0 [2487.5–3027.0] | 2538.7 [2479.9–3057.9] |

BarFilter is the reliably attributable cold-start gain; details/animation target interaction. Tens of milliseconds between cold-start groups are not independently classified as gains/regressions. About 0.55 s extra waiting occurred before the first Go/package marker, without slower backend DB. Five independent cold-ready rounds after adding SetWindowTheme spans gave **2736.3 [2627.7–3363.2]**, also retained, without picking the fastest group.

Then alternated baseline/final executables, five runs each (both had about 0.55 s before the first Go marker each round; loading/cache/security-software source undetermined):

| Adjacent stages | Alternating baseline | Alternating final |
| --- | ---: | ---: |
| spawn → DevTools connectable | 827.6 [781.7–876.7] | 825.4 [802.9–863.7] |
| DevTools → Shell current list | 436.0 [363.1–590.9] | 417.4 [375.8–599.5] |
| Shell → Home/startup notice stable | 692.5 [648.6–710.4] | 550.6 [485.8–556.7] |
| Home stable → Esc sent | 26.0 [22.9–28.5] | 23.1 [22.6–26.7] |
| Esc → notice gone and stable | 396.0 [389.5–421.5] | 396.9 [393.2–409.5] |
| Notice gone → editor stable | 881.6 [863.2–898.9] | 863.3 [848.4–881.0] |
| **Actual spawn → stable** | 3241.6 [3152.3–3456.6] | 3068.1 [3023.7–3214.5] |
| **Original runner cold-ready** | 3279.8 [3184.9–3489.4] | 3103.7 [3055.8–3248.6] |

Alternating-group total medians differ by 176.1 ms (about 5.4%), consistent with directly removed duplicate computation. Absolute values from different groups must not be mixed; all first-run/loading fluctuations are retained.

| Backend/synchronous drawing (overlapping) | Original | After Board | After details | After status animation |
| --- | ---: | ---: | ---: | ---: |
| App.Open (including DB/recovery) | 2.9 [2.0–10.0] | 2.7 [2.1–3.2] | 2.7 [2.6–3.5] | 2.2 [1.9–6.9] |
| DB open + migrations | 2.0 [1.1–5.4] | 1.5 [1.1–1.6] | 1.7 [1.6–2.1] | 1.6 [1.1–1.8] |
| Incoming-link recovery | 0.0 [0.0–0.5] | 0.0 [0.0–0.5] | 0.0 [0.0–0.0] | 0.0 [0.0–0.0] |
| window icon | 26.2 [25.5–27.4] | 26.3 [26.1–27.2] | 25.8 [25.2–27.2] | 25.6 [25.1–26.7] |
| First Workspace | 207.9 [202.5–222.3] | 203.9 [196.6–216.5] | 201.3 [182.2–236.1] | 204.5 [192.2–231.8] |
| Of which Board-build | 111.3 [108.7–113.6] | 108.5 [106.5–117.5] | 107.8 [101.8–116.7] | 109.6 [104.5–114.0] |
| First Home | 135.0 [133.7–149.5] | 133.2 [127.7–138.9] | 133.3 [71.2–163.4] | 132.8 [119.7–160.7] |
| Home-rows | 28.5 [26.3–32.1] | 27.8 [26.0–28.7] | 27.2 [26.8–29.5] | 27.4 [25.3–29.5] |
| Home-other-envs | 10.9 [8.9–12.1] | 12.6 [10.3–17.2] | 10.6 [10.0–13.1] | 12.1 [9.6–13.4] |
| **First BarFilter backend** | 188.6 [186.6–199.1] | 25.2 [23.8–26.7] | 26.0 [23.2–28.6] | 24.8 [24.6–26.7] |
| Hidden editor-render JS | 28.7 [27.2–29.7] | 28.2 [26.5–29.5] | 27.6 [27.2–32.9] | 28.0 [25.7–28.5] |
| First visible editor-render JS | 8.1 [7.5–8.3] | 8.0 [7.5–9.5] | 7.6 [7.4–8.3] | 7.9 [7.2–8.2] |
| First BarFilter JS total | 198.9 [196.1–211.2] | 36.3 [32.6–39.2] | 35.3 [33.5–37.8] | 33.8 [33.0–35.9] |

Original first-round second Board-build: **165.2 [162.8–171.8]**, recomputed immediately after Workspace. Previous P6's Home BuildHomeFromState and Workspace passing Needs to Shell were already in baseline; this round removes duplicate Board work in BarFilter.

| Alternating final absolute milestone (from spawn) | ms |
| --- | ---: |
| First Go package init marker | 569.1 [558.1–623.8] |
| GUI Run entry | 587.6 [578.2–645.8] |
| Call Wails Run | 619.3 [609.2–681.8] |
| First JS module | 932.3 [916.4–1111.2] |
| Wails WindowRuntimeReady | 936.8 [920.8–1115.4] |
| Final stable | 3068.1 [3023.7–3214.5] |

Before the first Go marker includes process loading and dependency-package initialization, not attributable to DB. This Wails version did not fire Common.WindowShow; window stages use the received runtime-ready, not exact first screen presentation. Phase 2 should add independent Win32 visibility observation.

First-round APIs (five alternating-final rounds): backend spans **include lock waiting**; JS totals also include encoding/decoding/bridge/scheduling. Payload is UTF-8 size after JSON.stringify again; extra sizeMs is about 1–2 ms for large payloads and excluded from API total. Workspace includes the asynchronous environment event, not just enqueueing.

| Call | Backend ms | JS total ms | payload bytes |
| --- | ---: | ---: | ---: |
| SetWindowTheme | 0.0 [0.0–0.0] | 15.0 [13.1–22.3] | 2 |
| Init | 0.5 [0.0–0.8] | 13.5 [12.0–21.1] | 12549 |
| Setting | 0.5 [0.0–1.0] | 30.5 [11.3–48.8] | 2 |
| Tasks | 0.0 [0.0–1.0] | 28.8 [8.8–46.7] | 65 |
| HomeBackground | 50.0 [38.1–58.6] | 58.0 [49.6–69.1] | 45 |
| CreditPrompt | 86.3 [63.5–90.5] | 92.2 [75.3–100.6] | 4 |
| Home | 139.9 [121.4–143.2] | 148.6 [133.9–150.9] | 1786 |
| Workspace | 201.6 [191.5–208.4] | 232.3 [210.1–255.7] | 639522 |
| BarFilter | 24.9 [24.8–25.6] | 37.8 [33.9–46.1] | 479005 |
| Live | 21.2 [20.1–24.5] | 25.1 [23.0–27.7] | 41 |
| Directories | 0.5 [0.0–0.5] | 2.6 [2.1–4.1] | 1871 |
| Ready | 1.1 [0.0–5.6] | 6.7 [2.9–8.1] | 2 |
| LibraryCopy | 13.8 [11.8–16.7] | 16.8 [14.1–30.7] | 4 |
| Detail | 34.9 [32.2–35.8] | 38.3 [34.4–41.6] | 698 |
| Leave | 0.0 [0.0–1.5] | 22.4 [21.8–25.6] | 2 |

Shell arrives with the environment payload, without an independent JS call; nested Shell-build **0.5 [0.0–0.6]**. Home/Workspace already run concurrently; index loader coalesces serialized index reads. Potential work remains in Home refresh, hidden-editor prebuilding, and cross-plan/environment summaries, but deferring them may not reduce total time to opening a list. This round did not expand concurrent whole-library reads or add TTL.

## Pause causes, fixes, and correctness

1. **af0633c: BarFilter reuses workspaceReader's immutable Board.** workspaceReader mutex protects cache; API.mu only snapshots session/options. Each call still refreshPageIndex, pinPageRead, and validates LoadoutKey. Edits and external inventory/alias changes must rebuild; stale data is not treated as fresh. Backend **188.6 → 25.2 ms**; alternating validation **185.2 → 24.9 ms**.
2. **c13a991: read only the final row's details after 75 ms cursor dwell.** Old seq rejected stale responses but could not cancel queued Detail/LibraryCopy or already computed detailFrame. With the serialized reader lock, old Detail took up to about 1.6–2.0 s. New flow immediately updates row name, clears old details/actions, and advances seq; explicit detail opening still reads immediately, with page/env guards and forced post-edit refresh retained.
3. **bad5094: restart status animation with Web Animations.** Old statusIn removed class → read offsetWidth → added class each time, forcing layout just after list changes. Now cancel prior animation and restart with the same opacity/translateY and CSS duration/easing, preserving reduced-motion/empty-text behavior. getComputedStyle may still update styles, but geometry is no longer forced. Profiled statusIn self time fell from about 3.5 s/full round to about 22 ms.

Five complete W scenario 1 rounds: 1/10/100 rows, 20/50/100 ms cadence, repeated/held Space, undo/staging/filtering. "Interval" below is the actual execution interval of an armed 25 ms timer, **not individual task duration**.

| Per-round metric | Board only, old details | Plus detail coalescing | Plus status-animation fix |
| --- | ---: | ---: | ---: |
| Detail requests | 584.0 [583.0–585.0] | 63.0 [62.0–79.0] | 73.0 [69.0–79.0] |
| LibraryCopy requests | 376.0 [375.0–377.0] | 57.0 [55.0–73.0] | 68.0 [64.0–74.0] |
| Maximum sampler interval | 917.7 [866.3–1078.6] | 284.4 [276.3–495.4] | 232.3 [215.8–266.9] |
| Number of >500 ms intervals | 5.0 [4.0–8.0] | 0.0 [0.0–0.0] | 0.0 [0.0–0.0] |
| Slowest Detail JS total | 1662.7 [1637.6–2041.8] | 73.8 [63.9–132.3] | 55.1 [54.0–70.8] |
| statusIn sampled self time/full round | 3943.1 [3665.7–4022.7] | 3497.7 [3230.9–4061.0] | 21.9 [15.7–28.4] |
| renderRows sampled self time/full round | 3677.2 [3440.3–3755.4] | 3602.2 [3465.4–3742.3] | 3190.2 [3175.0–3429.2] |
| detailFrame sampled self time/full round | 1257.7 [1163.5–1412.0] | 186.8 [178.8–230.7] | 191.4 [175.9–204.4] |

All five Board-only rounds passed functionality and failed performance as expected; all five detail-coalescing and five final rounds passed both, with no blank pages/lost sidebar/script errors. CPU figures are profiler sampling attribution, not precise hardware CPU timing; detail counts depend on cursor dwell/scheduling, and 75 ms does not coalesce every intermediate row at 100 ms intervals.

Independent timeline diagnostics (separate from five cold rounds, f4-trace-sync aligns renderer tasks and sampler):

| Version | >500 ms intervals | Maximum interval ms | Longest top-level renderer task ms |
|---|---:|---:|---:|
| Board only, old details | 8 | 1207.9 | 43.2 |
| Plus detail coalescing | 1 | 534.9 | 65.7 |
| Final status-animation fix | 0 | 218.5 | 33.0 |

The 534.9 ms interval occurred during consecutive Space input, overlapping multiple roughly 36–42 ms onKey tasks; larger intervals also contained many shorter tasks. Evidence supports **detail queues, dense input/short tasks, and synchronous layout jointly starving the event loop**, not a single 0.7 s function or large JSON parse. Backend mutex wait and actual computation are not yet separately instrumented, so no total lock-contention share is given; causal coalescing comparisons reducing queues/layout are complete.

In independent timelines, total Layout during first visible editor-render → stable was 11.5/14.5/15.8 ms for old details/detail fix/final, with longest individual Layout 1.1/2.4/2.3 ms; first Paint occurred 17.6/14.4/14.7 ms after render start. These single-round diagnostics do not replace five-round medians or exact monitor-presentation timing. Full-interaction Layout counts were 6681 → 5703 → 4724, total time 5485.0 → 5246.0 → 3599.6 ms, supporting repeated layout as cumulative interaction cost, not initial list layout as the main part of the roughly 0.85 s wait.

Long Tasks observer was supported but returned no entries; that does not prove no blocking. The roughly 2.2 s interval exporting all __vm evidence occurred after the armed sampler stopped and is test serialization, excluded from application pauses. All groups used the same runner; heavy traces have observation cost.

## Review of seven feedback items at 07:14

This table reviews existing implementation, commits, and regressions; old-audit real-data timings are not passed off as new measurements. No actual subscription removal was confirmed in this round.

| Feedback | Status | Commit/implementation | Test coverage |
|---|---|---|---|
| Several seconds before bulk-unsubscribe dialog appears | fixed (first feedback); preflight may still be long | b3edbbd, merged via 9f30c27; libraryConfirm opens checking first, previews asynchronously, rejects late results for old dialog | Existing check-background-nonblocking.cjs interaction check; no dedicated Node unit test |
| Slow bulk deletion; want fast irreversible option | partly: option implemented, full-scale speed unaccepted | b3edbbd / 9f30c27; subscription-remove-fast, explicitly irreversible, second confirmation; up to 4 workers without inbound links, serial with rescan when linked | TestParallelRemovalKeepsValidItemsAndFastSkipsRecycler; cancellation/inbound-link tests |
| Are all batches slow? | partly: not all slow, not all fixed | Audit separates DB edits from per-item files/global checks; bd8d5d8 improves retained reader; this round's three fixes reduce refresh cost; no "all fixed" commit | Edit/task/file tests exist; no all-action timing-budget coverage |
| Cannot do anything during background deletion | fixed (main lock path); large-library cold CPU reads may remain slow | b3edbbd / 9f30c27; independent background App/SQLite session, short API.mu, WAL reader | TestRemovalIOKeepsPagesAndEditorAvailable, TestColdPagesReadDuringWALWriter |
| List does not load, home falsely says no game | fixed (treating failure data as real) | d64c974 frontend retain/retry; b3edbbd / 9f30c27 explicit backend failures; bb5f64d preserves final results | TestReadFailuresAreExplicit, W scenario 1 sidebar/no-game oracle |
| Four rapid Space presses blank the page and lose sidebar | fixed (two known paths) | d64c974; b3edbbd valid setItems indices; eb4fb78 / 752f18a deferred ResizeObserver redraw; this round reduces detail queue | editorfilter Node, lists.test.mjs, complete W1 burst/undo; four new detail-follow tests |
| Sorting should automatically suggest dependencies | fixed (GUI flow connected) | 14b0684 offerAfterSort; 990b8c5 CLI/API repairs/notices; bb5f64d prevents loading overwriting results | TestSortDependencyAPI, TestReplaceRecommendedAPI, loadout_notices; no dedicated auto-dialog unit test |

## Validation, evidence, and phase 2

`go test ./... -p 2 -parallel 4` passed; full GUI suite passed again after status-animation changes; 7 Node runner tests passed; TestScaleFixture checks scale and workshop.fetch=off. Tests cleared real-data opt-ins and isolated AppData/temp under F4; no relaxed budgets/timeouts. Final GUI build and git diff --check passed.

New TestBarFilterReusesWorkspaceRowsAndInvalidates covers reuse, Toggle, alias/index invalidation; 4 detail-follow tests cover one timer/final read after 100 cursor changes, immediate clearing of old actions, page/environment changes, and stale in-flight replies; 2 status-motion tests cover time units, frames/easing, cancellation, empty text, reduced motion, and prohibit offsetWidth reads.

Evidence under F4: offline-baseline/filter/final and motion-cold each have five incremental cold rounds; complete-cold has five extra-instrumentation reruns; crossover-baseline/complete alternate five each; offline-before/after and motion-after each have five full-interaction rounds; offline-timeline-before/after and motion-timeline each one diagnostic. Root retains perf/result/window-trace/CPU/timeline archives by case ID; case directories retain process.log/checkpoint; result records executable SHA-256. EBUSY cleanup now retries without losing result; final offline groups are complete. Statistics scripts and summary.json stay in F4, outside git.

Total host output: 1,427,681,561 bytes (about 1.33 GiB). No data/temp files/screenshots in git.

After notification that VM is free, phase 2 should measure:

1. Same VM, same isolated real baseline state, five rounds each of baseline/three fix variants, using original W scenario 1 stable. Record DB/instance/entry/analysis-fact scale, hashes, WebView/cache state; do not mix groups.
2. Align spawn, first Go mark, DB migrations/recovery, Win32 visibility/runtime-ready, JS modules, each API backend/bridge/payload, first visible list render/layout, and stable. Distinguish roughly 0.55 s pre-marker fluctuations from extra VM seconds; inspect real facts/history, disk probes, Home summaries, heap/GC; segment mutex waits if needed.
3. Five rounds each of Space/move/filter bursts, reporting >500 ms intervals, unfinished Detail queues, feedback latency, correctness. Separate CPU/timeline diagnostics to validate statusIn gains with real layout.
4. Within existing authorization, validate external inventory/alias invalidation, plan/environment switching, background/edit interleaving, post-sort dependencies. Actual deletion/subscription changes require explicit authorization and are not done here.

## Real VM data review (before merge)

2026-10-27, Win11-Player-Test, snapshot `RimArcho-Real`: Steam 1.6.4871, 1293 Workshop Mods, default list 6 entries. Both release packages have identical assets (fonts and Archo Monitor), alternating 5 runs each.

- dev:`d20121f`
- F4: `0de4ab7` (dev merged)

| Metric | dev median [range] | F4 median [range] |
|---|---:|---:|
| `gui.log` ready (process start to startup-page drawing complete) | 892 ms [882–906] | 850 ms [846–893] |
| Longest main-thread interval while holding ↓ for 80 presses (33 ms interval) in editor disabled column | 55 ms [44–126] | 40 ms [36–41] |

Neither build had intervals above 200 ms; cursor ended on the same Mod. This VM list is small and cannot reproduce the host's 5.2 s cold start with 481 entries. The issue remains unexplained and needs a real-scale list. VM snapshot restored after testing.

## Real-scale VM list (483 entries)

List `big` built via CLI in VM: take 470 installed 1.6 Workshop Mods, `complete-dependencies`, sort/save, yielding 483 entries (17 missing), set current. Build dev `351c3ee`, `RIMARCHO_PERF=1`. Times start at Go initialization.

| Run | List | ready | WebView ready | Workspace | BarFilter | Home start |
|---|---|---:|---:|---:|---:|---:|
| First after list creation | big | 1576 ms | 691 | starts 778, lasts 154 ms | starts 950, lasts 436 ms | 1472 |
| Second, third | big | 1010 / 1035 ms | — | — | — | — |
| First, second | default (6 entries) | 968 / 998 ms | 455 | starts 495, lasts 100 ms | starts 608, lasts 240 ms | 910 |

Critical-path causes:

1. **Startup page waits for the editor.** `main.js start()` first `await reload()`, sequentially doing Workspace, BarFilter inside `refilter`, and editor rendering before `go("home")`. Home needs only `S.shell` from `Workspace`, yet waits another roughly 350–600 ms for BarFilter/hidden-editor rendering, 35–40% of startup-page time.
2. **With the filter bar open, first BarFilter reads every Mod's content index.** With groups true, `BarNeedsContent` holds and `reader.content.Get` reads content facts for all installed Mods. Real Mods have many Defs, taking 240–436 ms. Synthetic fixture Mods are nearly empty, so host measured only 25 ms. Second call hits cache, 16 ms.
3. List size itself matters little: with warm caches, 483 versus 6 entries differs by about 40 ms. Extra first-start cost (1576 ms immediately after list creation) comes from rebuilding invalidated caches.

The host runner's 5.2 s always used newly copied home and Mods, keeping caches cold; cold-ready also includes Esc, opening editor, and two stability waits. Thus 5.2 s likely largely reflects measurement method, not users' ordinary startup time. Rules prohibit copying real host data, so this was not directly verified.

### Fix: startup page no longer waits for editor

`main.js start()` opens the startup page immediately after Workspace returns shell and board. Editor filter results (BarFilter) and rows finish in the background, render behind the startup page, and `notePageContext(null)` records current game/list. Later editor opening uses the existing warm path: show these rows first, then reread in the background.

VM, same 483-entry list, one warm-up then 5 alternating rounds. Before: `1fac4bb`; after: this fix:

| Metric | before median [range] | after median [range] |
|---|---:|---:|
| `gui.log` ready | 986 ms [960–992] | 702 ms [673–722] |
| Subsequent sidebar editor opening | 19 ms | 10 ms |

Every round on both builds showed 483 editor entries, without read errors or loading placeholders.
