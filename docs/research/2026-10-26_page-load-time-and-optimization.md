# Page Load Times and Optimization Research

Research date: 2026-10-26 (local PDT). Code: `dev/0.9.0`, `81017d8`. This report records observations of the current implementation and optimization recommendations. No optimizations were implemented or code committed.

## 1. Conclusions and How to Read the Results

The clearest waits come from manually measuring all Mod textures (first run **78.9 seconds**, repeat **2.5 seconds**), uninstall preview (real directories **4.5 seconds**), and game download version information (anonymous SteamCMD **2.8–2.9 seconds**). Viewing the shared 1.7 logs in the 1.6 environment also requires cross-version analysis first: direct analysis in the isolated database took **30.3 seconds**. The existing analysis on the 1.6 "needs attention" page was stale; the measured interval from initial display through automatic analysis to fresh results was **35.2 seconds**. The immediate return time of an asynchronous API cannot stand in for final data readiness.

Among frequent pages, subscriptions and updates still takes **0.56–0.74 seconds** with a warm cache; add subscription follows the same heavy path. The first whole-library request in a new process takes **1.45 seconds**. The Mod library takes about **0.19–0.20 seconds + 22–26 ms JSON encoding** with a warm cache and transfers about **5.4 MB** at once; the first request in a new process takes about **1.46 seconds**. Home takes about **0.75 seconds** on first entry and **0.17–0.20 seconds** on repeat entry. The list management main panel now takes tens to just over a hundred milliseconds; the old, pre-optimization judgment of seconds no longer applies.

Recommended priorities: (1) make add subscription lightweight and reuse subscription-page inventory reads; (2) reuse the uninstall preview size cache; (3) reduce repeated construction of full Mod library DTOs and filter bars; (4) maintain texture and resource-usage statistics incrementally by snapshot; (5) reuse analysis results from the same read in Home/Workspace. Specific gains are research targets in section 7, with no modified-code comparison yet.

**All figures measure backend data readiness, not end-to-end GUI time.** No GUI was launched, screenshots taken, or DOM/paint timings invented. All purely local tabs, collapsible areas, and menus are mapped separately. "Incremental backend 0" means no new request, not zero screen rendering time. Nodes without real data, or requiring login or writes, are marked unmeasured with a measurement path.

## 2. Data, Isolation, and Method

### 2.1 Actual Data

| Item | Sample in this study |
| --- | --- |
| Machine | Ryzen 9 7950X, 16 cores / 32 logical threads; about 63 GiB RAM; Windows 11 Pro 10.0.26200 |
| Tools | Go 1.27.2 windows/amd64; temporary Go tests in the current repository's GUI package, calling real APIs directly; images via `PageHandler` + `httptest` |
| Data directory | `%APPDATA%\RimArcho`, locally expanded to `C:\Users\Touman_Lin\AppData\Roaming\RimArcho` |
| Database | 335,196,160 B; 6,523 instance, 6,791 snapshot, 950,168 snapshot_file, 601,193 def_fact, 1,727 finding, 9,455 change_event |
| Visible Mods | Both environments' Index contains 6,506; not the full table row count (tombstones/environment ownership and other filters) |
| Game main | `D:\SteamLibrary\steamapps\common\RimWorld`, 1.7.4990 rev1069; current plan 1; workspace list has 481 entries |
| Game 1.6 | `D:\rimworld16`, 1.6.4871 rev590; current plan 4; workspace list has 56 entries |
| Plans | 1 default; 2 Test; 3 default (1.6); 4 Multiplayer Modpack 1.6 (Stable); 5 The Progression Modpack [UPDATED 1.7]. Shared globally; both environments see all 5 |
| Other | 4 plan_collection; 2 log_cache; texture_file is 0; real packs directory has no packs |

Read the [top-level design](../00-top-level-design.md), [architecture](../00a-architecture.md), [engineering spec](../specs/17-engineering.md), and [UI spec](../specs/16-interface/README.md) first, then enumerated actual routes and calls. The GUI is Wails v3 + native ES modules; thin API → `internal/view` → `internal/app` → SQLite/disk. Direct calls here bypass the Wails bridge, so Go duration is not called screen load time.

### 2.2 Definitions of Cold, First Entry, and Warm

- **C (session cold)**: new `app.Open` / `newAPI`, one call without reading Workspace first. Database opening is outside the timer; existing persistent caches remain; the same test process may reuse process-level content caches. This is not process cold, OS cold, or first installation.
- **E (first entry)**: another new API completes Workspace, then opens the target page for the first time. Usually closer to clicking the sidebar after opening the app. Workspace completion is not counted again in the target page.
- **H (warm)**: median of the next 5 calls on the same API. Each table has 1 C/E call and 5 H calls, insufficient for p95. Cross-page order and disk caches can make C lower than E; that alone does not show a regression.
- **P (process cold)**: compile the test program separately; each run starts a new process and directly calls the target API without pre-reading Index/Loadout. Each main item uses 3 independent processes, reporting median and range; OS file caches remain.
- **Asynchronous final**: wait for Workspace's `environment` completion event or the final Build, PlanLoadCost, Textures result. This test polls every 1 ms; the frontend actually polls Build every 50 ms and textures/usage every 100 ms, adding about 0–50 / 0–100 ms before display, plus bridge and rendering time.
- JSON time is separate: `json.Marshal` is measured after API return, excluding JS decoding, DOM, and image decoding. Small Windows timings reported as 0 are recorded as `<1 ms`.

Measurements ran serially; profiling was separate from main timing. Phase samples ran 3 times and report medians. Phases change cache temperature, and cumulative calls may overlap, so they cannot be summed directly as page totals. Windows file caches were not cleared, nor were real caches deleted to manufacture "cold" conditions.

### 2.3 Isolation and Side-Effect Boundaries

Created `.cache/page-load-20261026/home/archo.db` using `backup()` through a read-only SQLite connection, and copied config.toml, tasks.json, and the real cache. Games, Mods, and logs were still read from real paths. The original backup SHA-256 is `6a9abaf587854829581a3b4adfb2aa2595f6ff50adcced7e6fc80e86ca70fa1b`. All normal API database/cache writes went to the copy. Account sessions/cookie directories were not copied, so Settings account state does not represent retained real login sessions. If production settings coincides with the 4-hour automatic site verification, it may add up to 10 seconds of network waiting; this was not covered. Host directories such as WebView/WezTerm were not copied either, so directory sizes were additionally measured using the copied Store + real `Home`, directly calling `PlanUninstall` after reviewing it as read-only. `Uninstall` / `UninstallRun` was not called.

The main round disabled HTTP, SteamWorker, and automatic AnalyzeRunner to keep background scanning/analysis out of timing. It therefore represents opening pages with existing scan results. **SteamCMDRun and SteamWorker are separate channels**: `GuideDownloadInfo` still actually ran anonymous `app_info_update` / `app_info_print 294100`, returning 21 public branches; this accounts for the roughly 3 seconds in the table. No real Steam account login, subscription, unsubscription, deletion, tool installation, deployment, or game launch occurred. A few unauthenticated HTTP reads of Harmony's public description followed; see section 5. Cross-version log analysis was explicitly run separately on the copy; errors from disabled analysis were not counted as readiness.

`%APPDATA%\RimArcho\refnums` did not exist, and the DB had no `ui.gui.refnums` record, so specific numbers such as `hom22` could not be verified. Only real prefixes from [refnums.js](../../internal/surface/gui/assets/js/refnums.js) are cited below; no numbers are invented.

## 3. Current Page Tree and Measurement Mapping by Node

Entry points come from `openPage/go` in [navigation.js](../../internal/surface/gui/assets/js/navigation.js), Nav in [shell.go](../../internal/surface/gui/shell.go), and each page's `api.*` calls. `L` in the tree means the parent payload already supplies the data: entering the child needs no new backend request (incremental C/H both 0; DOM unmeasured). `U` marks an unmeasured path explained in section 6. Arbitrary search terms, every data row, and every button state are not counted as infinitely many pages; their entry types and representative measurements are listed.

```text
RimArcho GUI
├─ Global shell / sidebar / breadcrumbs / game and list selection → Init, Shell, Workspace (table A)
│  ├─ Switch environments / open another list → Workspace; no actual switch against real config
│  ├─ Command palette → Palette; help / shortcut dialog → Keys (table C)
│  ├─ Selection / input / generic confirmation / text dialog / context menu → L; dynamic content uses the relevant page API
│  └─ Table column visibility / order / menus / draggable dividers → L; saved Setting reads are tiny; no saves performed
├─ Home [hom] → Home + HomeBackground concurrently (table A)
│  ├─ Game selection, that game's list/pack cards, last run, status and changes → parent payload
│  ├─ Hide/expand problems, keyboard focus, switch cards within the same data → L
│  └─ Background JPEG → /bg (table F); selecting another game/list writes selection, so target-environment sessions simulated reads here
├─ Can it run / needs attention [run] → Needs (table A)
│  ├─ Four sections: save-breaking/unrunnable, last-run errors, needs confirmation, hidden Mods → L
│  ├─ More actions, choose disable targets, identity confirmation choices, evidence text → parent data or Inspect (table G)
│  └─ Log / Mod details navigation → independent nodes; recheck is an Analyze operation (U)
├─ Mod library
│  ├─ All [lib] → Library (table A)
│  │  ├─ Search harmony / workshop source filter / sort → Library; representative values measured
│  │  ├─ Expand families, copy rows, column/facet menus → L; local virtual scrolling does not reduce transferred data
│  │  ├─ Right details → Detail + LibraryFields (tables C / D), Workshop details fetched on demand (table F)
│  │  └─ Copy operation preview/confirmation → LibraryPreview / OpPreview (table G, single Mod; other scopes U); tags/notes/position use shared dialogs
│  ├─ Subscriptions and updates [chg] → SubscriptionsPage (table A)
│  │  ├─ Current list / all Mods / single Mod family scope → all three independent requests measured
│  │  ├─ Pending / completed updates, date collapse, source status, selected-item details → L
│  │  ├─ Expand file changes → ChangeFiles, real before/after snapshots measured (table F)
│  │  └─ Source checks / updates / redownload / unsubscribe preview and confirmation → table G single-Mod previews; task execution U
│  └─ Add subscription [sub] → SubscriptionsPage(false, "") (table A)
│     ├─ Auto-detect Workshop / Git / web / LoversLab, Steam/SteamCMD method → local form L
│     ├─ Workshop confirmation preview → SubscribePreview (table F, --dry-run only)
│     ├─ Web metadata / Git branch/tag/commit / LoversLab files, protocol and authentication pages → U / public web failure sample in table F
│     └─ Choose download directory/zip, authentication file / native site login window → U
├─ Lists
│  ├─ Current/specified list editor [edt] → Workspace (table A)
│  │  ├─ Inactive / active / staged columns, selections, sort menus → parent data L
│  │  ├─ Search within column → Filter; top cross-column filters and facets → BarFilter (table D)
│  │  ├─ Right details → Detail + LibraryCopy (tables C / D)
│  │  ├─ History drawer → History; copy selection → ModCopies; row actions → RowOps
│  │  ├─ Edit tags → TagCandidates; move to position → PosOptions; note input → ModNote
│  │  └─ Sort / write to game / launch preview → Preview(sort/save/launch) (table C), no actions applied
│  ├─ All lists [lst] → PlanManage, all 5 plans measured separately (table B)
│  │  ├─ Name / game-version chips / followed collections / follow mode / differences and missing entries → parent payload L
│  │  ├─ Resource-usage statistics panel and Top details → PlanLoadCost → PlanLoadCostMeasure (tables B / D)
│  │  ├─ Automatic followed-collection check (after 10 min) → PlanCheck (U; network branch excluded from main panel)
│  │  ├─ New/copy/draft → CheckPlanName, PlanSources / PlanFroms (table C)
│  │  │  ├─ Empty / existing list, game version / save sources → PlanSources response; option switching L
│  │  │  └─ Import file/text/collection preview → ExchangeInfo, ImportPreviewFor (table C; collection U)
│  │  └─ Rename / delete confirmation → CheckPlanName, RemovePlanWarn (tables C / D), not applied
│  ├─ Import [imp] → ExchangeInfo; choose format/target/paste/preview → ImportPreviewFor
│  │  └─ Merge/replace/new, missing-subscription choices → preview data L; apply/subscribe/native picker U
│  ├─ Export [exp] → PlanExport + ExchangeInfo (tables C / D)
│  │  ├─ archo / rimsort / rml / rimcrow-xml / rimcrow / text / modsconfig / workshop → all 8 formats measured
│  │  └─ Output text, copy/save results and overwrite confirmation → L / U (no clipboard or export-file writes)
│  └─ Create a pack [bld] → BuildPage → BuildPageResult (tables A / D)
│     ├─ Name check → PackNameCheck; destination, steps, blockers → parent payload
│     ├─ No conversion / 100 / 80 / 75 / 60 / 50 / 40 / 25% → option switching is L; only clicking "count the textures" requests again
│     ├─ See the list first → PackDryRun (table F)
│     └─ Build progress / errors / resume unfinished pack / completed pack → U (no actual build)
├─ Packs [pak] → Packs (table A, actually empty)
│  ├─ Individual pack pack:<name> / Mod table / description / saves and other details → Pack (U, no real sample)
│  ├─ Unfinished build entries / progress → U, no real sample
│  └─ Launch checks, export preview / delete confirmation → PackLaunch / PackExport / PackRemovePreview (U)
├─ Convert textures to DDS [tex] → TexturesInfo(plan/all) (table A)
│  ├─ Current list / all scope, scale and encoder choices → TexturesInfo or existing local options
│  ├─ Manual "measure" → Textures → TextureMeasure, including first whole-library 78.9s (tables A / D)
│  ├─ Statistics row details / expansion / progress → returned payload L
│  └─ Tool install confirmation / conversion / cleanup / stop and results → U (not executed)
├─ Games [env] → Environment (table A)
│  ├─ Edit path / version / launch method / saves / arguments → existing group data L; native path selection U
│  ├─ Remove game confirmation → EnvRemovePreview (table C), no removal
│  └─ Add game → guide [gui]
├─ Settings [set] → Settings + Backgrounds + Tasks concurrently (table A)
│  ├─ Appearance: theme, accent → same Settings response; open options L (actual changes write config, unmeasured)
│  ├─ Home background → Backgrounds(initial/complete)
│  │  ├─ Game, each background Mod, custom-image group tabs → L; thumbnail/full image /bg → table F
│  │  └─ Dimming, blur, focus, favorites, slideshow options → existing data; no image selection/deletion/saves
│  ├─ RimArcho management directories → Settings; directory-check dialog → Directories
│  │  └─ Change/move location confirmation → DirectoryMovePreview (U), path picker U
│  ├─ Mod folders → Settings; add/remove confirmation L, not applied
│  ├─ Accounts → status in Settings; login/forget/web login/Steam verification dialogs → U
│  ├─ External tools → status in Settings; ToolInstallOffer install-information confirmation → U
│  ├─ Interface / community data / database / cache / game logs → 6 groups in the same Settings request, incremental L
│  │  └─ Cache cleanup / database cleanup / reread logs and other confirmations → existing text L; execution and results U
│  └─ Uninstall [del] → UninstallPlan (tables A / F)
│     ├─ Keep copies/packs/saves/games, destination directory, backup/restore options → UninstallPlan again for each answer change
│     └─ Final confirmation → L; execution progress / results / exit → U (never executed)
├─ Logs [log] → LogPage (tables A / F)
│  ├─ Player.log / Player-prev.log / ArchoMonitor.log and accompanying monitor area → all 3 files measured additionally
│  ├─ Error-cluster list, filters/sort, selection → existing Rows L
│  ├─ Original evidence + community pattern card → LogCluster + LogPattern (serial reader, table F)
│  └─ Live monitor area → LogMonitor / Live; no running game in this study (tables C / D, empty-data note)
├─ Tasks [tsk] → Tasks (table A)
│  ├─ Status / queue / time / search filters, column menus → L
│  └─ Task row details / children / failure-text expansion → existing payload L; retry/cancel/cleanup results U
├─ Guide [gui] → Tasks + GuideFind (table A)
│  ├─ First run/add game, detected games and manual-path confirmation → GuideFind / GuideGame (table F)
│  ├─ Download game: public branches/directory/account choice → GuideDownloadInfo; dropdown options L
│  │  └─ SteamCMD installation / download progress / login confirmation → U
│  ├─ Read Mods progress → GuideScan (U: actual scan operation, not a read-only page)
│  └─ Where to start: game lists/list import/pack or collection → L; import reuses imp, pack extraction U
└─ Full Mod details [ful] (independent entry) → Detail + ModCopies + Sections concurrently (table D)
   ├─ Six cards: Mod description / copies / used by / fields / history / technical information → same result
   ├─ Technical Def / Patch / Harmony / rules / history tabs → Sections returns all at once; switching L
   ├─ Choose copy/description cache or remote fetch → corresponding Detail / ModCopies (tables C / F)
   └─ Tags/position/notes/operation previews, all-changes navigation → reuse corresponding nodes
```

The 11 basic Settings groups come from one `Settings()` response, not 11 backend routes; backgrounds are independent. Future planned pages such as recap/save management/worth playing/Archo, which are not registered yet, are excluded from current page counts. TUI uses the same `internal/view`; this report covers the GUI pages above, and does not present TUI drawing or terminal startup as GUI data.

## 4. Page and Sub-Entry Timing Tables

All units are ms; each environment cell is **C / E / H**. H is the median of 5 calls. Supplementary tables explicitly use **E₀ / H₃** (first after warming Workspace / median of the next 3 calls). Attribution mainly comes from call chains and phase evidence in section 5, not guesses based solely on timing.

### Table A: Main Pages

| Page / backend call | main: C / E / H | 1.6: C / E / H | Main source, readiness boundary |
| --- | --- | --- | --- |
| Global initialization Init | <1 / <1 / <1 | <1 / <1 / <1 | Static descriptions/settings; about 0.5ms additional JSON |
| Editor/environment workspace Workspace | 419.5 / 284.6 / 281.6 | 376.8 / 263.0 / 260.5 | Completion event; Index, Board, Needs, Shell; about 1.33MB |
| Home hom: Home | 1003.0 / 753.3 / 169.5 | 803.6 / 754.0 / 198.1 | Independent log reader, current plan Board/Needs, other plans and environments |
| Home background HomeBackground | 125.7 / 2.5 / <1 | 129.1 / 2.0 / <1 | Initial Index/background settings; JPEG separate |
| Sidebar Shell | 279.2 / 105.1 / 6.5 | 246.5 / 79.7 / 7.5 | boardState, Needs, plan/pack rows; reusable when warm |
| Can it run run: Needs | 605.2 / 273.1 / 120.2 | 508.9 / 237.4 / 78.6 | Load analysis/logs, disk copy/link checks, BuildNeeds |
| Mod library lib: all Library | 1600.6 / 687.2 / 200.1 | 869.8 / 686.2 / 191.6 | Inventory construction/full view; about 5.4MB JSON measured separately |
| lib: search harmony | 591.1 / 534.6 / 68.8 | 621.5 / 496.7 / 57.8 | Same inventory; still constructs matching view/full facets |
| lib: workshop source filter | 669.0 / 516.7 / 103.4 | 639.2 / 545.3 / 95.0 | Same; not just frontend filtering |
| Updates chg: current plan | 649.2 / 618.3 / 595.5 | 564.4 / 541.2 / 560.1 | SubscriptionsPage; new reader, BuildChanges, full Library |
| Updates chg: whole library | 697.5 / 759.0 / 735.4 | 633.1 / 656.6 / 636.3 | Same; do not substitute old Changes API figures |
| Updates chg: single Mod | 547.4 / 559.4 / 562.6 | 534.7 / 568.9 / 672.9 | Same; smaller scope still builds the full library |
| Add subscription sub: accounts/recent items | 612.1 / 579.6 / 605.5 | 572.7 / 562.3 / 656.9 | Same chain as SubscriptionsPage, not pure form overhead |
| Packs pak: list | 5.8 / 1.5 / 1.1 | 5.9 / 1.5 / 1.5 | Actually 0 packs; represents empty list only |
| Create pack bld: final without texture conversion | 358.7 / 278.4 / 271.3 | 464.8 / 396.7 / 393.2 | Asynchronous BuildPage, problem checks, space/manifest |
| bld: final 100% texture scan | 310.2 / 278.6 / 274.8 | 17336.2 / 780.8 / 779.6 | Initial 1.6 run fills texture information; changing scale alone does not trigger this |
| All lists lst: default main panel | 185.6 / 37.0 / 35.3 | 262.6 / 118.4 / 100.7 | Scoped deployment preview; statistics panel separate |
| lst: final default plan resource-usage statistics | 426.4 / 316.7 / 311.1 | 586.3 / 197.5 / 172.7 | Directory stamp traversal, cache validation, resource counting |
| Export exp: default format | 177.8 / 2.0 / 2.5 | 138.8 / 1.0 / 1.1 | Plan serialization; 8 formats in table D |
| Import imp: page information | <1 / <1 / <1 | <1 / <1 / <1 | Format/import capability descriptions |
| imp: text preview | 161.7 / 2.5 / 2.0 | 168.1 / 1.0 / 1.0 | 2 packageId samples; actual import not applied |
| Games env | 4.8 / 1.0 / 1.5 | 5.8 / <1 / 1.1 | Environment settings groups; no full Board rebuild |
| Settings set: Settings body | 225.6 / 194.2 / 183.7 | 151.4 / 191.3 / 160.0 | detachedReader each time; Index/tools/directories etc.; concurrent backgrounds and Tasks separate |
| set: initial background options | 131.6 / 2.0 / 1.0 | 107.0 / 2.0 / 1.5 | Existing cache; initial call may synchronously read Index; no wait for background extraction |
| set: all background preparation complete | 301.4 / 30.8 / 33.1 | 134.6 / 32.9 / 32.2 | Wait for workers; persistent image cache retained, not first decode without image cache |
| Textures tex: initial plan-page information | 151.1 / 2.5 / 1.5 | 148.8 / 1.5 / 1.0 | Targets/tools only, no texture contents read |
| tex: initial whole-library page information | 170.5 / 3.1 / 2.0 | 137.6 / 5.7 / 5.6 | Same; statistics separate |
| tex: final plan statistics | 826.4 / 203.0 / 233.2 | 215.8 / 77.9 / 79.8 | CollectCached, directory/PNG header cache, MakePlan |
| Tasks tsk | 1.3 / <1 / <1 | <1 / 1.4 / <1 | Current history/queue snapshot; no real task execution in this study |
| Guide gui: existing game detection | 6.8 / 2.5 / 1.5 | 5.6 / 1.5 / 2.2 | Detect paths and version files |
| gui: download version information | 2946.7 / 2830.7 / 2868.5 | 3057.6 / 2720.5 / 2795.9 | Real anonymous SteamCMD public branch query |
| Logs log: default file | 355.0 / 213.7 / 4.1 | Not ready (analysis disabled; supplement in table F) | main valid; original 1.6 round disabled analysis, see table F; error return is not readiness |
| log: second candidate file | 618.1 / 196.6 / 6.1 | Not ready (analysis disabled; supplement in table F) | API uses second LogFiles item, locally Player-prev; same 1.6 limitation |
| Uninstall del: preview | 7663.8 / 4470.2 / 4508.3 | 4152.9 / 4572.8 / 4219.3 | Copied Home; real Home supplement in table F; per-directory sizes and link checks |

### Table B: Main Panels and Resource-Usage Statistics for All Real Plans

`PlanManage(id)` uses C/E/H. Resource-usage statistics directly use `App.PlanLoadCost`, median of three phase measurements, with real directories and persistent caches retained. This is separate diagnosis, not end-to-end timing. Different environments resolve different versions of LoadFolders; statistics counts may differ from workspace entries (DLC/deduplication/available-resource rules).

| Plan | main panel C/E/H | 1.6 panel C/E/H | main resource-usage statistics | 1.6 resource-usage statistics |
| --- | --- | --- | --- | --- |
| 1 default | 275.1 / 33.7 / 39.1 | 232.1 / 91.3 / 102.6 | 244.6 | 280.5 |
| 2 Test | 256.7 / 46.5 / 39.7 | 308.5 / 101.5 / 104.2 | 74.9 | 70.9 |
| 3 default (1.6) | 222.3 / 43.6 / 34.4 | 208.9 / 85.9 / 107.2 | 22.6 | 22.9 |
| 4 Multiplayer Modpack 1.6 (Stable) | 226.8 / 46.2 / 45.4 | 287.2 / 160.8 / 153.4 | 145.7 | 143.1 |
| 5 The Progression Modpack [UPDATED 1.7] | 259.8 / 67.8 / 68.4 | 307.9 / 151.2 / 151.4 | 325.1 | 364.0 |

Resource-usage cache hits are not zero work. For example, the third main plan 1 run still had 430 hits/44 remeasurements, plan 5 had 527/43; 1.6 plan 5 had 454 hits/259 remeasurements initially and 671/42 in the next two runs. Total time cannot all be attributed to cache misses, nor was a full-hit comparison achieved here.

### Table C: Details, Dialogs, Editor, and Settings Subrequests

| Sub-entry / API | main: C / E / H | 1.6: C / E / H | Method / notes |
| --- | --- | --- | --- |
| Cached Mod details Detail | 301.3 / 166.3 / 6.7 | 259.1 / 141.0 / 7.2 | Valid Harmony data; About/cached Workshop parsing; network fetch in table F |
| Copy selection ModCopies | 204.8 / 40.4 / <1 | 133.8 / 34.1 / <1 | Real Harmony copies |
| Editor copy fields LibraryCopy | 192.8 / 39.8 / <1 | 140.3 / 32.2 / <1 | Concurrent composition with Detail in table D |
| Library right-side fields LibraryFields | 176.8 / 41.2 / <1 | 144.7 / 38.3 / <1 | Same |
| Full-detail technical Sections | 203.6 / 172.8 / 176.3 | 149.5 / 137.7 / 143.0 | Harmony; new reader, all tab text and Humanize |
| Tag candidates TagCandidates | 171.4 / 1.0 / <1 | 125.0 / <1 / 1.0 | Single Mod; empty candidates also valid |
| Move position PosOptions | 186.7 / 7.6 / 4.5 | 128.5 / 1.5 / 1.2 | Single Mod, empty search, before=false |
| Row menu RowOps | 161.2 / 37.3 / <1 | 144.6 / 32.6 / <1 | Harmony in active column |
| Command palette Palette | 164.9 / 3.4 / 1.5 | 126.2 / 3.1 / 2.5 | Empty query |
| Shortcut help Keys | <1 / <1 / <1 | <1 / 1.0 / <1 | plan page; static table varies by page |
| History History | N/A / 19.4 / 17.3 | N/A / <1 / <1 | C has no board; null is not completion; E/H valid only |
| Sort preview Preview(sort) | 194.5 / 6.8 / 7.0 | 141.2 / 1.5 / 1.0 | No actual sorting |
| Write preview Preview(save) | 203.7 / 37.2 / 28.9 | 174.8 / 47.7 / 27.7 | No ModsConfig writes |
| Launch preview Preview(launch) | 339.6 / 158.2 / 39.7 | 239.0 / 136.8 / 42.6 | No game launch |
| New-list sources PlanSources | 173.0 / 1.0 / 1.5 | 123.3 / 1.5 / 1.0 | Current plan source/version/save options |
| New-list name validation CheckPlanName | 152.6 / 1.0 / 1.0 | 136.5 / 1.5 / 1.5 | Valid new list, no writes |
| Old new-list sources PlanFroms | 153.3 / <1 / <1 | 134.3 / <1 / <1 | Compatibility data for existing entry point |
| Remove game confirmation EnvRemovePreview | <1 / 1.0 / <1 | 1.0 / <1 / 1.0 | Current environment, read-only |
| Directory check Directories | 1.9 / 1.0 / <1 | <1 / <1 / <1 | Read-only directory summary |
| Live monitor LogMonitor | 175.7 / <1 / <1 | 132.9 / <1 / <1 | No running game: empty return does not represent populated monitoring |

### Table D: Combined Entries, Options, and Supplementary Whole-Library Statistics

E₀/H₃ are first after Workspace warmup / median of 3 subsequent calls. No cold duration is invented for combinations without independent C samples; table C has their component requests' cold costs. Concurrent combinations were actually invoked; individual durations cannot simply be added.

| Node | main E₀ / H₃ | 1.6 E₀ / H₃ | Description |
| --- | --- | --- | --- |
| Full/Harmony | 236.1 / 189.3 | 164.5 / 166.5 | Full-detail Promise.all, shared by 6 cards/all technical tabs |
| Panel/editor | 158.0 / 7.5 | 138.9 / 12.5 | Detail + LibraryCopy concurrently |
| Panel/library | 159.9 / 6.0 | 127.3 / 10.1 | Detail + LibraryFields concurrently |
| Full/largeDefs | 222.7 / 196.8 | Failed: no mod | main samples instance with most def_fact rows; absent from this environment's 1.6 Index; failure is not completion |
| Sections/largeDefs | 193.8 / 215.2 | Failed: no mod | Same; main output about 13.6KB; not all large Def sets cost the same |
| Export/archo | 3.0 / 2.5 | 2.5 / 1.2 | Export text |
| Export/rimsort | 1.5 / 1.5 | 1.5 / 1.0 | Export text |
| Export/rml | 3.4 / 1.6 | 1.2 / 1.0 | Export text |
| Export/rimcrow-xml | 2.5 / 1.0 | 2.0 / <1 | Export text |
| Export/rimcrow | 3.1 / 2.8 | <1 / 1.5 | Export text |
| Export/text | 2.5 / 1.5 | 1.5 / 1.0 | Export text |
| Export/modsconfig | 1.5 / 1.6 | 1.6 / 1.6 | Export text |
| Export/workshop | 2.0 / 1.8 | <1 / 1.0 | Export text |
| Build/80 | 277.0 / 295.9 | 784.0 / 775.7 | Explicit "count the textures"; scale selection itself needs no request |
| Build/75 | 283.9 / 306.0 | 800.7 / 767.3 | Explicit "count the textures"; scale selection itself needs no request |
| Build/60 | 283.2 / 273.2 | 883.8 / 819.3 | Explicit "count the textures"; scale selection itself needs no request |
| Build/50 | 288.1 / 298.4 | 847.3 / 825.8 | Explicit "count the textures"; scale selection itself needs no request |
| Build/40 | 307.3 / 277.1 | 793.5 / 760.4 | Explicit "count the textures"; scale selection itself needs no request |
| Build/25 | 301.6 / 278.6 | 884.4 / 830.3 | Explicit "count the textures"; scale selection itself needs no request |
| Textures/all | 78861.8 / 2553.9 | 2280.1 / 2481.1 | Both environments share texture files/cache; main first run 78.9s; main already warmed 1.6 |
| Filter/inactive | 107.5 / 6.1 | 91.9 / 8.5 | harmony search in inactive column; primary filter result does not reload full Board |
| BarFilter | 338.4 / 398.1 | 330.7 / 311.9 | Cross-column filtering + facets, about 1.08MB result; worth optimizing |
| RemovePlanWarn | 1.5 / 1.0 | <1 / 1.0 | Read-only warning for removing current plan |
| Live | 33.3 / 35.0 | 23.7 / 24.0 | Probe without running game; represents idle state only |

### Table E: First Request in Independent Processes (main)

P is the median [minimum, maximum] of 3 new processes; process/database opening is outside API timing, and OS caches were not cleared. JSON also reports the median of 3 calls.

| Target | P ms [range] | JSON ms |
| --- | --- | --- |
| Workspace | 424.2 [414.1, 429.1] | 5.6 |
| Home | 843.6 [842.7, 855.0] | <1 |
| Library | 1459.0 [1454.4, 1506.3] | 21.3 |
| SubscriptionsPage | 1452.3 [1451.1, 1464.1] | 2.0 |
| Settings | 213.6 [197.6, 284.2] | <1 |

## 5. Source Breakdown, Validity Checks, and Slow-Page Ranking

### Table F: Supplementary Real Files, Logs, and External Reads

| Node | First / subsequent ms | Data and boundary |
| --- | --- | --- |
| ChangeFiles 11009 → 14785 | 7.6 / 5.1 | 65 file differences; median of next 3 calls; real change_event |
| ChangeFiles 11007 → 14781 | <1 / <1 | 8 differences |
| ChangeFiles 1427 → 14780 | 7.1 / 6.5 | 138 differences |
| ChangeFiles 1043 → 14777 | <1 / <1 | 16 differences; repeated pairs in latest 5 events listed once |
| Real Home uninstall plan | 4512.9 / 4446.3 / 4555.2 | Three independent readers, 10 steps; median 4512.9; no uninstall execution |
| Harmony Preview HTTP | 68.8 / 0.5 | 200, 41,807 B; existing disk thumbnail cache, not generation without cache |
| Background thumb HTTP | 52.7 / <1 | 200, 23,479 B; existing JPEG cache |
| Background full HTTP | <1 / <1 | 200, 255,479 B; same image; browser decoding unmeasured |
| Forced Harmony Workshop details refresh | 412.2 | Unauthenticated, 24 Workshop text blocks; subsequent cached reads 134.0, 139.6, all error-free |
| SubscriptionPage public web page | 253.3 / 270.7 | Request reached parsing but "no version or modification date"; error readiness, not successful web-preview baseline |
| Workshop subscription dry-run preview | main 151.9 / 147.3; 1.6 140.3 / 151.1 | Returns 1 subscription plan; apply is only an executability flag; Subscribe not called |
| PackNameCheck | main <1 / <1; 1.6 <1 / <1 | New valid name, no pack created |
| PackDryRun | main 104.2 / 5.2; 1.6 298.0 / 221.4 | main returns blockers for missing dependencies; 1.6 returns real dry-run manifest; workloads differ |
| ModNote | Both environments <1 / <1 | Current note empty; valid empty string |
| GuideGame real path | main <1 / <1; 1.6 5.6 / <1 | Successfully returns 1.7 and 1.6 version/path information respectively |
| main Player.log | 333.2 / 3.1 | 9 clusters; repeat entry on same reader; independent from table A, not a replacement C |
| main Player-prev.log | 11.9 / 5.1 | 26 clusters; Player.log read before, reader already warm |
| main ArchoMonitor.log | 4.6 / 5.2 | 26 lines; same |
| Complete 1.7 log analysis for 1.6 | 30296.0 | Explicit App.Analyze on copy; excludes production subprocess startup/GUI polling; additional prerequisite for full readiness |
| 1.6 Player.log (after analysis) | 339.2 / 5.1 | 9 clusters; original 4.6ms with analysis disabled is not completion |
| 1.6 Player-prev.log (after analysis) | 177.1 / 7.2 | 26 clusters |
| 1.6 ArchoMonitor.log (after analysis) | 5.9 / 8.7 | 26 lines |
| Evidence + pattern card, main's three logs | 53.4 / 3.6 / 222.2 | Actual first clusters E4, E17, E17 respectively; 18/13/18 lines; latter two community cards exist |
| Evidence + pattern card, 1.6's three logs | 40.2 / 41.9 / 190.7 | 19/14/21 lines; LogCluster and LogPattern share logRead lock, so concurrent frontend calls still serialize |

The original round's empty `ChangeFiles` differences using the same snapshot, unsupported-operation error from `LibraryPreview("lock")`, and missing 1.6 "large Def instance" failure are excluded from successful performance data. Original `LogPattern/LogCluster("E1")` is also not an evidence-content baseline; actual returned clusters were checked above.

### Table G: Concurrent Page Dependencies, Final Analysis Readiness, and Operation Dialog Supplements

The following were measured after the main round. The copy had completed cross-version log analysis and analysis of the current 1.6 plan; these figures cannot replace the original Stale scenario. Page combinations actually used goroutines concurrently. C/E each have 1 sample; H is the median of the next 3; JSON encoding excluded. The Settings combination waits only for initial Backgrounds options; complete background image preparation is in table A.

| Page combination | main C / E / H ms | 1.6 C / E / H ms |
| --- | --- | --- |
| hom: Home + HomeBackground + Setting(hidden) | 833.8 / 648.3 / 162.5 | 762.1 / 641.2 / 196.9 |
| set: Settings + Backgrounds + Tasks | 299.7 / 226.3 / 214.7 | 158.1 / 167.3 / 165.3 |

The supplementary 1.6 Needs measurement followed the real `Needs → startAnalyze → analyze event → Needs` path. Only the runner was replaced with an independent App directly running Analyze on the copy (avoiding GUI/subprocess startup): at 207.0ms it first returned 62 items with `stale=true`; at 34771.3ms analysis success arrived; at 35191.6ms it returned 62 items with `stale=false/never=false`. The next three calls took 74.9/70.9/76.5ms, median 74.9ms. **Even with equal item counts, old results and final fresh analysis must be distinguished.** Production subprocess startup, IPC, and frontend refresh scheduling remain excluded. This sample shows actual backend work; it does not establish a full production end-to-end 35.2 seconds.

The following uses the main Workspace and real Harmony `#nvxs`, a single copy, first call / median of next 3 calls. Only `LibraryPreview` was called, never `LibraryDo`. `apply=false` means actual rejection/nothing to do in the preview, measuring only that branch, not successful execution preparation. Editor `OpPreview` converts IDs to refs and calls the same LibraryPreview directly, so these dependency times can be reused, but conversion overhead was not separately timed.

| LibraryPreview operation | First / H₃ ms | apply / rows | Explanation |
| --- | --- | --- | --- |
| pin | 8.9 / <1 | true / 3 | Dry-run only, plan displayable; not executed |
| pin-update | <1 / <1 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| unpin | <1 / <1 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| unhide | <1 / <1 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| move | <1 / <1 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| delete | 304.0 / 253.7 | true / 2 | Dry-run only, plan displayable; not executed |
| unsubscribe | 1.0 / 1.0 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| resubscribe | <1 / <1 | false / 2 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| textures | 29.9 / 28.6 | false / 4 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| textures-clean | 25.2 / 27.1 | false / 1 | Rejected or no work with current Workshop copy/Steam tools disabled; not a successful baseline |
| subscription-remove | 502.0 / 480.6 | true / 3 | Dry-run only, plan displayable; not executed |
| subscription-update | 238.0 / 216.7 | true / 4 | Dry-run only, plan displayable; not executed |

Warm subscription-removal previews at 480.6ms and deletion previews at 253.7ms are themselves slow dialogs above 200ms. Their dry-run and "which plans are affected" statistics should narrow reads to the current ref/plan set (`view/library.go LibPreview/usedWarning`), avoiding repeated Loadout/PlansUsing for each selection. Initial optimizations P1/P4 can share the read-only inventory and reverse plan index here; query multiple selections in batches, without caching final deletion permission.

| Inspect evidence command | First / H₃ ms | Text lines |
| --- | --- | --- |
| `mod/#nvxs` | 101.8 / 7.1 | 23 |
| `patches/--mod/#nvxs` | 2.6 / 1.6 | 3 |
| `def/--mod/#nvxs` | 2.9 / 1.0 | 1 |
| `harmony/--mod/#nvxs` | 13.0 / 11.8 | 3 |
| `problems` | 173.5 / 9.9 | 37 |
| `folder/check` | 1.0 / 1.0 | 11 |

Evidence covers only these real command results. Def/Patch lines may be empty or summary-only; they do not establish large-output performance for all Mods.

### 5.1 Phase Timing Evidence

| Independent diagnostic phase (median of three) | main ms | 1.6 ms | Meaning for total duration |
| --- | --- | --- | --- |
| Index/cold | 152.3 | 115.7 | DB loading, field/index assembly; not full disk rescan |
| Loadout/after-index | 32.0 | 22.8 | Resolve plan, dependencies/LoadFolders etc. |
| ReadSession | 5.7 | 6.6 | New SQLite connection overhead itself far below Index rebuild |
| ReadIndex/copy | 3.0 | 3.0 | Immutable index snapshot copying already implemented; useful pattern for other slow pages |
| Loadout/after-copy | 32.8 | 22.4 | Copying Index still requires current Loadout construction |
| Library/build-warm-index | 491.7 | 398.1 | First inventory aggregation after obtaining Index; not BuildLibrary view |
| Library/rebuild | 247.6 | 225.5 | Rebuilding inventory on same reader is still nonzero |
| Library/render-all | 192.1 | 176.7 | Full BuildLibrary DTO/ops/facets, excluding JSON |
| Library/render-filter | 74.2 | 56.1 | View construction for harmony filter |
| Library/JSON | 26.7 | 24.7 | Phase samples encode in about 24–27ms |
| Changes/after-index | 47.8 | 14.2 | Old BuildChanges, used only as internal Subscriptions phase evidence |
| Subscriptions/after-index | 271.2 | 238.7 | Same warmed reader; differs from fresh API on every call |
| ModsCopies/disk | 6.6 | 31.3 | Independent disk copy checks; 1.6 directories differ |
| GameLinks/disk | 25.4 | 24.7 | Link checks; warm Problems branch may not rerun this full chain |
| Problems/first | 96.7 | 39.7 | Initial problem aggregation, including cache and disk branches |
| Problems/hot-with-disk | 4.7 | 6.7 | Disk checks remain after Problems cache exists |
| Needs/view | 2.5 | 1.2 | BuildNeeds itself tiny; most waiting prepares its inputs |
| Board/view | 142.8 | 162.1 | Board generation after same-session warmup |
| Settings/Dependencies | 16.2 | 13.4 | Tool location/version reads, existing version cache |
| Settings/Caches | 2.5 | 2.7 | Cache directory WalkDir statistics not the main Settings bottleneck with this data |
| Settings/Folders | 3.0 | 3.2 | Folder group |
| Settings/All | 19.6 | 19.9 | Complete SettingsPage after same App has Index and other state |
| Home/first | 698.8 | 589.4 | Initial Home after phases above; not same temperature as table A |
| Home/hot | 172.3 | 222.9 | Repeated Home generation still does work |

In the main round, warm Library/all encoding was about 22.2 ms for main and 26.2 ms for 1.6; payloads were 5,355,970 B and 5,335,776 B. Search still transfers about 1.65 MB, source filtering about 2.60 MB. Workspace is about 1.33 MB with about 4.6 ms encoding. In contrast, whole-library subscriptions is only about 0.37 MB with about 1.5 ms encoding, so prioritize reducing backend reconstruction on subscriptions, and also reduce transfer/object construction on the library page.

### 5.2 Where Disk, XML, Network, Repeated Computation, and Locks Occur

**Repeated inventory/subscription computation.** [SubscriptionsPage](../../internal/surface/gui/subscriptions.go) runs `detachedReader → Loadout → BuildSubscriptions` every time. [BuildSubscriptions](../../internal/view/subscriptions.go) builds changes, then calls `App.Library()` for the full inventory. Viewing one Mod does not skip the whole library. Add subscription needs only accounts/recent entries but constructs the same data first. New connections at 5–7ms are not the whole problem; Index at 116–152ms, first inventory at 398–492ms, and repeated inventory at 225–248ms deserve more attention. [readers.go](../../internal/surface/gui/readers.go) and [readsession.go](../../internal/app/readsession.go) already have the `ReadIndex/readInventorySession` pattern; sharing a mutable App is unnecessary to save this cost.

**CPU and lock evidence.** A separate CPU/mutex profile of 12 consecutive main `SubscriptionsPage(true, "")` calls, including one Workspace preparation, took 9.93s wall time and 14.15 CPU seconds. Cumulative samples: `BuildSubscriptions` 7.21 CPU seconds, `App.Library` 5.28, SQLite VDBE 4.18, `Loadout` 2.03, `Index/loadIndex` 1.57, `ContentKinds` 1.03. These nested cumulative samples cannot be added and do not represent wall-time percentages of page phases. GC drain accumulated 3.77 CPU seconds, supporting fewer temporary whole-library objects. The mutex profile's 1.86 seconds of aggregate waiting is mainly runtime/GC locks. **There is no evidence attributing the current serial subscription-page slowness to the main API mutex.** Code proves some concurrent requests serialize, but this single-page profile cannot establish real concurrent GUI p95.

**XML/About.** Pages use decl/analysis stored in the DB by scanning, rather than recursively parsing all disk XML on every request. Lazy declaration reads in `Index` and Loadout still involve parsing/assembly costs, but `encoding/xml` was not separately instrumented here; an "XML share" cannot be invented. The 30.3-second cross-version analysis is the prerequisite that actually reads Def/Patch/assemblies and simulates them; distinguish it from page display with existing analysis. The initial 78.9-second texture run and 17.3-second Build100 run relate to persistent PNG header/dimension caches and different target scopes, **not XML parsing time**. There was no per-file syscall tracing; exact breakdowns still require focused profiling.

**Texture and resource-usage statistics.** `readTextureMod → CollectCached → MakePlan` in [app/textures.go](../../internal/app/textures.go) traverses texture directories and computes statistics. Cache hits save PNG header reads, not all directory traversal. [Collect](../../internal/textures/collect.go) reads the Textures tree and each file's information. [loadcost.go](../../internal/app/loadcost.go) traverses Textures/Sounds/Defs/Patches/Languages/Assemblies through `loadCostStamp/fsscan.Walk` **before** `cache.Get`. Up to 8 Mod workers already exist; do not increase parallelism without evidence. Different plans still have invalidated entries; cache keys/invalidation sources merit focused investigation.

**Home/Workspace.** [shell.go Home](../../internal/surface/gui/shell.go) builds Board, readNeeds, plans/packs through an independent `logRead` reader, then AddOtherEnvs; some data is evaluated repeatedly. [problems.go](../../internal/app/problems.go) checks copies/links/legacy packs even with a problem cache; the Needs view itself takes only 1–3ms. `shellLocked(nil)` also obtains Needs again. These reads account for external Mod changes, so optimize by reuse within one operation or short caches with invalidation.

**Settings/details.** [api.go Sections/Settings](../../internal/surface/gui/api.go) uses new readers. SettingsPage on a warmed App is about 20ms, while warm GUI API calls remain 160–184ms; the difference includes rebuilding new state. Settings cache directory statistics take only about 3ms and should not be optimized first. Full details [fulldetail.js](../../internal/surface/gui/assets/js/fulldetail.js) finishes only after `Promise.all(Detail, ModCopies, Sections)`; all technical-page commands and Humanize run on entry, making otherwise fast details wait for Sections at 143–176ms.

**Uninstall.** [uninstall.go sizeOf](../../internal/app/uninstall.go) already caches sizes once per App. However, GUI [UninstallPlan](../../internal/surface/gui/uninstall.go) creates a detached reader each time, discarding that cache after the call and rescanning sizes when checkboxes change. Three real-Home runs still take 4.4–4.6 seconds; the copy's first 7.7-second run is not a stable disk-cold baseline.

**Network.** [gamedl.go GameBranches](../../internal/app/gamedl.go) makes anonymous public queries via SteamCMD and holds `accounts.Lock(exe)`. Its 3 seconds includes process startup/SteamCMD/network, not separately timed. A successful forced details refresh takes 412ms, while cached fetches still take 134–140ms, showing the cost of details' short-lived reader reconstruction. General web parsing success conditions differ from the Workshop description API. The public-page metadata sample failed and cannot be treated as the same path.

### 5.3 Slow-Node Ranking

The following ranks major observed complete-readiness waits, without combining different cache temperatures into one uniform ranking. H in frequent-page rows is the main/1.6 warm median. Asynchronous results need frontend polling time added to approach user-perceived time.

| Order | Node | Observed wait | Type |
| --- | --- | --- | --- |
| 1 | Manual whole-library texture statistics | First 78.862s; H 2.554 / 2.481s | Very slow initial run + warm traversal; not initial texture page |
| 2 | Stale Needs analysis / cross-version log prerequisite | Final Needs 35.192s; direct log analysis 30.296s + page construction | In-process runner on copy; production subprocess startup overhead unmeasured |
| 3 | Create pack 100% texture statistics | 1.6 C 17.336s; H 0.275 / 0.780s | Initial target texture cache; manual scan statistics |
| 4 | Uninstall preview | Real Home median 4.513s | Reproducible on each answer change |
| 5 | Game download version information | H 2.869 / 2.796s | SteamCMD public branch query |
| 6 | Whole-library subscriptions and updates | P 1.452s; H 0.735 / 0.636s | Frequent pure reads, clear gains |
| 7 | Add subscription / current / single-Mod updates | H 0.56–0.67s | Small pages also rebuild whole library |
| 8 | Initial full Mod library | P 1.459s; E 0.687s | Inventory + DTO; H still about 0.2s plus encoding |
| 9 | First Home entry | E 0.753 / 0.754s; H 0.170 / 0.198s | Independent reader, cross-plan/environment |
| 10 | Subscription removal dry-run dialog | main H 0.481s | Single Harmony preview, no real unsubscription |
| 11 | Create pack without conversion | H 0.271 / 0.393s | Prechecks, excluding actual build |
| 12 | Editor top filter bar | H 0.398 / 0.312s | Rebuild facets/about 1.08MB objects |
| 13 | Workspace | H 0.282 / 0.261s | Combined Board/Needs/Shell snapshot |
| 14 | Plan resource-usage statistics | main current H 0.311s; largest plan diagnosis 0.325/0.364s | Stamp scan, remeasure invalidated entries |
| 15 | Delete dry-run dialog | main H 0.254s | Single Harmony, deletion not called |
| 16 | Plan texture statistics | H 0.233 / 0.080s | Depends on plan target texture count |
| 17 | Full details/technical, settings | Usually 0.15–0.20s | Near 200ms; large content/concurrency still need GUI validation |

## 6. Nodes Without Successful Figures and How to Estimate/Measure Them

| Node | Why unmeasured | Executable method / entry-to-readiness model |
| --- | --- | --- |
| All L subtabs/options/collapsible areas/pure confirmation dialogs | No independent handler; incremental backend C/H both 0; GUI not launched | Add `performance.mark` around existing JS `beginPageRead/read.finish` or click callbacks, then measure paint with two requestAnimationFrame calls; retain parent payload preparation time and report DOM separately. No unmeasured "guaranteed 16ms" promises |
| Native file/directory/program pickers, account windows, Steam codes/web login | Require GUI/human interaction; real account interaction should not be triggered automatically | Measure only open→first usable control; human thinking/input separate. UI automation or Wails/native callback timing requires specific authorization to launch windows; not done here |
| Successful ordinary web/Git/LoversLab branch and file pages | Cost depends on URL/auth/remote content; only public metadata failure sample here | Use unauthenticated/authorized read-only APIs for actual user sources; cold means no persistent metadata, warm retains cache. Time HTTP DNS/TLS/TTFB/body and parsing separately; do not replace authenticated file lists with public errors. Table F explicitly identifies failure |
| Followed-collection refresh PlanCheck, Workshop collection import | Main panel ready before network checks; network disabled this round to control background variables; no actual remote results | Read the 4 real plan_collection entries through read-only public requests; record first PlanManage screen and complete collection readiness separately, storing cache in isolated Store. Separate collection sizes; do not infer from a 2ms local form |
| Pack details/unfinished packs/pack launch checks, export and removal previews | Real packs empty; synthetic packs are not real user samples | Once packs exist, measure `Pack/PackRemovePreview`, PackExport preview, and safe launch checks in order; time manifest decoding, Mod table, directory sizes separately. First screen ≈ manifest + view; final ≈ first screen + detail scans; no numeric range here |
| DirectoryMovePreview, other LibraryPreview/OpPreview and Needs Inspect input combinations | Depends on path/action/selection count/evidence type; table G covers single-Mod branches for all 12 LibPreview types and 6 Inspect types, not arbitrary paths/multiselection/all evidence instances | Use copied config, valid target paths, single Mod/100 Mods/select all, cold/hot, dry-run only. List rejections separately too; never execute Apply/Run. Earlier erroneous lock sample excluded |
| Rescan, first pack import, installation/build/texture conversion, task execution/retry/cancel/uninstall results | Post-operation state pages require writes; not read-only entry loading | Tasks/events already supply backend data for progress shells; complete result time depends on operation duration. Run focused throughput tests only in isolated output directories; button enqueue return is not completion |
| Current real-game live monitoring | Game not running during test; LogMonitor empty | Passively poll an already running user game; separate probe, incremental tail parsing, attribution/display; do not launch game for measurement here |
| First network image/img download, first Unity decode without persistent background cache | Real cache retained; image HTTP measured valid cache hits only | Move only target cache entry aside in copy; leave real resources untouched. Measure download/decode-resize/JPEG encode/disk write, then HTTP and browser decode; many images require concurrency limits/cancellation statistics |
| Truly OS-cold disk / first scan after installation | System caches not cleared; existing 335MB index database; would substantially change baseline | Sample fixed order after reboot or use independent cloned VM; record disk bytes/queue/antivirus effects. This report's C is not load time after clearing all caches |

These limits do not zero unmeasured items as "fast." Successful read entries appear in the tables. Every finite UI node has a data dependency or supplementary measurement path above; branches without successful samples do not establish full end-to-end GUI coverage.

## 7. Optimizations (Ordered by Benefit / Cost)

Benefit ranges are removable work and targets based on current measured phases, **not implemented speedups**. Correct invalidation after real external file changes is required; deletion/deployment authorization results must not be cached.

| Priority | Evidence → concrete approach | Expected benefit / acceptance target | Change scope (files / functions relative to internal/) | Cost and risk |
| --- | --- | --- | --- | --- |
| P1 | sub needs only accounts/recent items but calls full BuildSubscriptions; add lightweight AddSubscriptionInfo; chg uses readInventorySession/ReadIndex and reuses read-only inventory by environment, snapshot, scope, change key | sub warm about 0.6s→target within 30–50ms; chg removes 116–152ms Index rebuild + 225–248ms repeated inventory; warm target 150–300ms, needs measurement | `surface/gui/subscriptions.go SubscriptionsPage`, `assets/js/subscriptions.js openAdd`, `view/subscriptions.go BuildSubscriptions`, `gui/readers.go` / `app/readsession.go` | Small to medium; no shared mutable App/DB iterators; invalidate after scan/task/environment/plan/tag/note changes; lightweight sub can ship independently first |
| P2 | Existing App.sizeOf uninstall cache discarded by each detachedReader; retain dedicated preview reader or per-preview size map; option changes only reassemble plan; sizes may appear progressively in background | Repeated option changes save most 4.5s directory traversal; warm target <100–300ms, initial path summary first | `gui/uninstall.go UninstallPlan`, `app/uninstall.go PlanUninstall/sizeOf` | Small to medium; cache byte estimates only, not deletion decisions; fully revalidate paths/processes/ownership before execution; mark external size changes as estimates and allow refresh |
| P3 | Public branches start SteamCMD each time; 5–10min TTL + singleflight by appID/tool identity; show old data before refresh | Warm branch info 2.8s→local <10–30ms; initial still about 3s in background | `app/gamedl.go GameBranches`, `gui/shell.go GuideDownloadInfo`, `assets/js/guide.js` | Small; briefly delayed new branches; failure must not overwrite old data; retain accounts.Lock, never bypass SteamCMD serialization lock |
| P4 | Warm Library BuildLibrary about 177–192ms, 5.4MB; cache static rows/ops, read selected details on demand, aggregate facets once; BarFilter reuses counts/query index | Warm library backend target within 80–120ms; payload target down 50–80% (verify with field audit first); BarFilter target within 50–100ms | `view/library.go BuildLibrary/libInst`, `gui/library.go library/Library`, `view/columns.go` / `view/bar.go`, `gui/ops.go BarFilter`, `assets/js/library.js/lists.js` | Medium; row action availability changes with current plan/tasks/tags, so snapshot-only caching insufficient; retain accessible fields; existing DOM virtualization is not transfer optimization |
| P5 | Whole-library textures first 78.9s, warm 2.5s; loadcost still Walks before hit; maintain summaries by snapshot+LoadFolders+game MM+active package set; rescan only changed Mods; show last statistics, validate in background | Same-input reentry target within 50–150ms; initial whole-library still requires I/O, progressively visible; if 90% of snapshots unchanged, target about 90% less remeasurement work, no guaranteed proportional wall-time reduction | `app/textures.go TextureSummaryProgress/readTextureMod`, `textures/cache.go CollectCached`, `app/loadcost.go PlanLoadCost/loadCostStamp`, `gui/textures.go`, `gui/planmanage.go` | Medium to large; root directory mtime does not represent child-file changes; need scan events/file watching or short TTL + recursive background validation, tool/algorithm version keys; external DDS ownership still needs safety checks |
| P6 | Home rebuilds Board/Needs and repeatedly reads across plans/environments; Workspace Shell may repeat needs; memoize within one read context, pass existing Needs to shell, reuse Board/PlanProblems by input key | Initial removes one or more 116–162ms phases; target E<350–500ms, H<80–120ms; Workspace saves repeated aggregation, profile actual removable work | `gui/shell.go Home/shellLocked`, `gui/api.go needsLocked`, `gui/environment.go`, `app/problems.go`, `view/shell.go BuildHome/AddOtherEnvs` | Medium; current/open plans differ, never reuse wrong plan; log updates/external link changes must invalidate; long TTL hides new problems |
| P7 | Full-detail Promise.all waits for all Sections; send Detail/Copies first, lazy-load/cache Sections by technical tab; Settings keeps independent reader but reuses Index | ful shows main description/copies first in about 10–50ms (existing reader), avoiding 143–176ms wait; warm Settings approaches 20–50ms from 160–184ms | `assets/js/fulldetail.js full`, `gui/api.go Sections/Settings`, `view/detail.go Sections`, `gui/readers.go` | Small to medium; progressive sections need accurate loading states; first screen is not all ready; section cache key includes snapshot, gameMM, analysis version, language; readIndex copy must remain consistent |
| P8 | Cross-version logs require 30.3s analysis; cache by environment + plan-set fingerprint + log MM; queue in background on version conflict, show original text/clusters before progressive attribution; combine LogCluster/Pattern to share one report | Initial 30s work not guaranteed to shrink; avoid repeating it on second read; evidence path at 190–222ms targets <20–50ms; profile repeated parsing/attribution | `app/logs.go EnsureLogAnalysis`, `app/analysis.go EnsureAnalysis`, `gui/api.go LogPage/LogCluster/LogPattern`, `view/logpage.go` | Medium to large; pending/persistent caches already exist, fill missing keys/reuse rather than adding duplicate cache layer; background analysis must respect 128/subprocess memory limits and cancellation |

By **benefit/cost**, P3 on the infrequent download page can land quickly. By **waits users encounter most**, recommend these 5 near-term changes:

1. **Lightweight add subscription + chg read-only inventory reuse (P1)**: frequent and well evidenced; split the lightweight interface first, then compare old/new subscription pages.
2. **Retain uninstall preview size cache (P2)**: continue the existing cache design; consecutive selections should not each wait 4.5 seconds.
3. **Reduce library-list DTO/filter-bar weight (P4)**: improve Go construction, the 5.4MB bridge transfer, and WebView object handling together.
4. **Incremental texture/usage statistics by Mod snapshot (P5)**: address the longest 78.9-second wait; deliver last results + background validation before incremental correctness.
5. **Reuse within one Home/Workspace read (P6)**: improve daily startup/navigation; establish repetition with phase/CPU profiles before merging work.

P7 can fit between these as a small patch; P8 first needs focused observation of cross-version keys and repeated analysis. No recommendation for a global cache on every page or blindly adding goroutines: that would reduce external Mod-change visibility and amplify memory/disk contention.

### 7.1 Relationship to Recent Existing Optimizations

- `bfc8d93` ("only inspect copies with the same packageId, instead of rebuilding the registry for all Mods", translated): `app/deploy.go regWith` already narrows candidates. Current warm PlanManage panels take 35–153ms; table B is the post-optimization baseline. Do not propose the completed whole-library registry fix again. The useful pattern is scoped queries, directly applicable to single-Mod/current-plan Subscriptions branches.
- `93b5639`: `app/library.go` already builds Mods/duplicate maps by folder, and `store/content.go ContentKinds` already caches by snapshot/scans. New Apps/readers, warm view reconstruction, and payload costs remain. P1/P4 should reuse existing caches and complete invalidation boundaries, not reimplement the folder map or duplicate ContentKinds caching.
- `ReadIndex`, inventory readers, PNG header cache, asynchronous measurement/cancellation, and up to 8 loadcost workers already exist. Connect these existing paths first; this baseline does not justify replacing the SQLite driver, introducing ORM/frontend frameworks, or adding dependencies.

## 8. Reproduction and Research Artifacts

Original observations remain locally in git-ignored `.cache/page-load-20261026/`: `results.jsonl` (910 records, 65 entry types × two environments × C/E/H), `run.log`, `detail.log`, `process.log`, `supplement.log`, `logpreviews.log`, `public.log`, `sub.cpu`, `sub.mutex`, `needsfinal.log`, `groups.log`. This report includes summaries and can be read without those caches. Raw files contain local paths and are not uploaded to external services.

The temporary measurement file was `internal/surface/gui/research_pageload_test.go`, deleted on completion. Only this report was added; no business code changed and no commit was made. Another `2026-10-26_steamcmd-parallel-downloads.md` appeared in the workspace during cleanup, was not produced by this study, and was neither changed nor deleted. No research test source remained. Full measurement source follows for reproduction on the same version, preserving an auditable measurement contract after restoring the tests.

Reproduction steps:

1. Confirm branch/commit, close background tasks that change Mods/logs, and record hardware, game versions, plan/index counts, and existing cache state.
2. Create a new isolated Home with a read-only SQLite backup; copy config, tasks, cache, and empty packs. Do not point research Home directly at real `%APPDATA%\RimArcho`. Example backup code follows; `dst` must be a new directory.

```python
from pathlib import Path
import os, sqlite3, shutil
src = Path(os.environ["APPDATA"]) / "RimArcho"
dst = Path(".cache/page-load-repeat/home")
dst.mkdir(parents=True, exist_ok=False)
with sqlite3.connect((src / "archo.db").as_uri()+"?mode=ro", uri=True) as a:
    with sqlite3.connect(dst / "archo.db") as b:
        a.backup(b)
for name in ("config.toml", "tasks.json"):
    if (src / name).exists(): shutil.copy2(src / name, dst / name)
shutil.copytree(src / "cache", dst / "cache")
(dst / "packs").mkdir()
```

3. Temporarily save the source below to the Go test path above. Running the main measurement unchanged includes anonymous SteamCMD branch queries; `TestResearchPublicAndAssets` performs only public HTTP; `TestResearchLogAndPreviews` runs one actual analysis on the copy. Adjust fixed Harmony ID/real environment names before supplementary runs; changed actual samples cannot reuse this report's snapshot IDs directly.

```powershell
$env:RIMARCHO_RESEARCH_HOME = (Resolve-Path .cache/page-load-repeat/home).Path
$env:RIMARCHO_RESEARCH_OUTPUT = Join-Path (Resolve-Path .cache/page-load-repeat).Path 'results.jsonl'
go test ./internal/surface/gui -run '^TestResearchPageLoads$' -count=1 -timeout 20m -v
go test ./internal/surface/gui -run '^TestResearch(Breakdown|Extras|Validate)$' -count=1 -timeout 20m -v
# Run supplementary tests serially to avoid CPU/I/O interference
go test ./internal/surface/gui -run '^TestResearch(Supplement|LogAndPreviews|PublicAndAssets|NeedsFinal|PageGroups)$' -count=1 -timeout 20m -v
# True process cold: start 3 new processes per method, without PageLoads pre-read preparation
go test -c -o .cache/page-load-repeat/gui.test.exe ./internal/surface/gui
foreach ($method in @('Workspace','Home','Library','SubscriptionsPage','Settings')) {
    $env:RIMARCHO_RESEARCH_ENV = 'main'
    $env:RIMARCHO_RESEARCH_PROFILE = $method
    foreach ($run in 1..3) {
        & .cache/page-load-repeat/gui.test.exe '-test.run=^TestResearchIndependent$' '-test.v'
    }
}
$env:RIMARCHO_RESEARCH_PROFILE = 'SubscriptionsPage'
& .cache/page-load-repeat/gui.test.exe '-test.run=^TestResearchProfile$' '-test.cpuprofile=.cache/page-load-repeat/sub.cpu' '-test.mutexprofile=.cache/page-load-repeat/sub.mutex'
go tool pprof -top -cum .cache/page-load-repeat/gui.test.exe .cache/page-load-repeat/sub.cpu
# Remove only temporary research source added by this study; keep the report
git diff --check
git status --short
```

Disabling `AnalyzeRunner/SteamWorker`, enabling public HTTP separately, timing JSON separately, and polling asynchronous final results in the appendix are part of the measurement contract. This is not a test for submission and must not be added to regular CI with automatic network access.

<details>
<summary>Full temporary measurement source (removed from business directories after research)</summary>

```go

package gui

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "reflect"
 "runtime"
 "runtime/debug"
 "sort"
 "strings"
 "testing"
 "time"

 "github.com/ToumanLin/RimArcho/internal/app"
 "github.com/ToumanLin/RimArcho/internal/view"
 "github.com/ToumanLin/RimArcho/internal/modchanges"
)

type researchTransport struct{}
func (researchTransport) RoundTrip(*http.Request)(*http.Response,error){return nil,errors.New("research: network disabled")}
type researchCase struct {name, method string; args []any; async string}
type researchRow struct {Env,Name,Mode string; Run int; HandlerMS,JSONMS float64; Bytes int; Error string `json:",omitempty"`}
func researchInvoke(s *API, c researchCase)(any,error){
 if c.async=="full" || c.async=="editor-panel" || c.async=="library-panel" {
  id:=c.args[0].(string)
  calls:=[]researchCase{{method:"Detail",args:[]any{id,false,false,false}}}
  switch c.async{case "full":calls=append(calls,researchCase{method:"ModCopies",args:[]any{id}},researchCase{method:"Sections",args:[]any{id}});case "editor-panel":calls=append(calls,researchCase{method:"LibraryCopy",args:[]any{id}});case "library-panel":calls=append(calls,researchCase{method:"LibraryFields",args:[]any{id}})}
  type res struct{v any;e error};ch:=make(chan res,len(calls))
  for _,call:=range calls{go func(){v,e:=researchInvoke(s,call);ch<-res{v,e}}()};vs:=[]any{};var err error
  for range calls{v:=<-ch;vs=append(vs,v.v);if v.e!=nil{err=v.e}};return vs,err
 }
 if c.async=="workspace" {
  ch:=make(chan *EnvironmentResult,1)
  s.emit=func(n string,v any){if n=="environment"{ch<-v.(*EnvironmentResult)}}
  s.environment.mu.Lock();seq:=s.environment.readSeq+1;s.environment.mu.Unlock()
  if !s.Workspace(seq){return nil,errors.New("workspace rejected")}
  select{case v:=<-ch: if v.Error!=""{return v,errors.New(v.Error)};return v,nil;case <-time.After(2*time.Minute):return nil,errors.New("workspace timeout")}
 }
 if c.async=="build" {id,e:=s.BuildPage(c.args[0].(int));if e!=nil{return nil,e};for {p,e:=s.BuildPageResult(id);if e!=nil||p!=nil{return p,e};time.Sleep(time.Millisecond)}}
 if c.async=="cost" {p,e:=s.PlanLoadCost(c.args[0].(int64));if e!=nil{return nil,e};for {p=s.PlanLoadCostMeasure(p.Request);if p==nil{return nil,errors.New("lost cost")};if !p.Loading {if p.Error!=""{return p,errors.New(p.Error)};return p,nil};time.Sleep(time.Millisecond)}}
 if c.async=="texture" {p,e:=s.Textures(c.args[0].(string),100);if e!=nil{return nil,e};for {p=s.TextureMeasure(p.Request);if p==nil{return nil,errors.New("lost texture")};if !p.Loading {if p.Error!=""{return p,errors.New(p.Error)};return p,nil};time.Sleep(time.Millisecond)}}
 if c.async=="background" {s.Backgrounds();s.bgRead.workers.Wait();s.mu.Lock();out:=append([]app.Background(nil),s.bgs...);s.mu.Unlock();return out,nil}
 m:=reflect.ValueOf(s).MethodByName(c.method);if !m.IsValid(){return nil,fmt.Errorf("no method %s",c.method)}
 ins:=make([]reflect.Value,len(c.args));for i,v:=range c.args{if v==nil {ins[i]=reflect.Zero(m.Type().In(i))}else{ins[i]=reflect.ValueOf(v)}}
 vals:=m.Call(ins);var out any
 for _,v:=range vals{if v.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()){if !v.IsNil(){return out,v.Interface().(error)}}else{out=v.Interface()}}
 return out,nil
}
func researchOpen(t *testing.T, env string)(*API,func()){
 a,e:=app.Open(os.Getenv("RIMARCHO_RESEARCH_HOME"));if e!=nil{t.Fatal(e)}
 a.EnvName=env;a.LazyLog=true;a.LogCacheOnly=true
 a.HTTP=&http.Client{Transport:researchTransport{},Timeout:5*time.Second}
 a.AnalyzeRunner=func(string,func(string))error{return errors.New("research: analysis launch disabled")}
 a.SteamWorker=func(string,[]string)(string,error){return "",errors.New("research: Steam disabled")}
 tuneProcess(a)
 s:=newAPI(a);s.busy["analyze"]=true
 return s,func(){s.Close();a.Close()}
}
func TestResearchPageLoads(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip("research only")}
 view.SetLang("zh");debug.SetMemoryLimit(MemoryLimit)
 out,e:=os.OpenFile(os.Getenv("RIMARCHO_RESEARCH_OUTPUT"),os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600);if e!=nil{t.Fatal(e)};defer out.Close();enc:=json.NewEncoder(out)
 envs:=[]string{"main","1.6"};if x:=os.Getenv("RIMARCHO_RESEARCH_ENV");x!=""{envs=[]string{x}}
 for _,env:=range envs{
 s,done:=researchOpen(t,env);st,e:=s.a.Loadout();if e!=nil{t.Fatal(e)}
 plan:=st.Plan.ID
 ids:=[]string{};for _,m:=range st.Ix.Mods{if m.WorkshopID()!=""&&m.SnapID>0{ids=append(ids,m.ID)}};sort.Strings(ids)
 id:=ids[0];for _,m:=range st.Ix.Mods{if strings.Contains(strings.ToLower(m.Name),"harmony"){id=m.ID;break}}
 ref:=st.Ix.Ref(id);snap:=st.Ix.ByID[id].SnapID
 plans,_:=s.a.Plans();logs:=view.LogFiles(s.a,st)
 fmt.Printf("DATA env=%s plan=%d mods=%d representative=%s snapshot=%d logs=%d cpus=%d\n",env,plan,len(st.Ix.Mods),ref,snap,len(logs),runtime.NumCPU())
 logpath:="";if len(logs)>0{logpath=logs[0].Path}
 done()
 c:=func(n,m string,args ...any)researchCase{return researchCase{name:n,method:m,args:args}}
 cases:=[]researchCase{
 c("Init","Init"), {name:"Workspace",async:"workspace"},c("Home","Home"),c("HomeBackground","HomeBackground"),c("Shell","Shell"),c("Needs","Needs"),
 c("Library/all","Library","",[]string{},""),c("Library/search","Library","harmony",[]string{},""),c("Library/filter","Library","",[]string{"source:workshop"},""),
 c("Subscriptions/plan","SubscriptionsPage",false,""),c("Subscriptions/library","SubscriptionsPage",true,""),c("Subscriptions/mod","SubscriptionsPage",false,id),
 c("Changes/plan","Changes",false,""),c("Changes/library","Changes",true,""),c("Changes/mod","Changes",false,id),c("ChangeFiles","ChangeFiles",snap,snap),
 c("AddSubscription/data","SubscriptionsPage",false,""),c("Packs","Packs"),
 {name:"Build/off",async:"build",args:[]any{0}},{name:"Build/100",async:"build",args:[]any{100}},
 c("PlanManage","PlanManage",int64(0)),c("PlanSources","PlanSources",plan),c("CheckPlanName","CheckPlanName","new list","new list",int64(0)),c("PlanFroms","PlanFroms"),
 {name:"PlanLoadCost",async:"cost",args:[]any{plan}},
 c("PlanExport/default","PlanExport",plan,""),c("ExchangeInfo","ExchangeInfo"),c("ImportPreview/text","ImportPreviewFor",plan,"text","ludeon.rimworld\nbrrainz.harmony"),
 c("Environment","Environment"),c("EnvRemovePreview","EnvRemovePreview",env),c("Settings","Settings"),c("Directories","Directories"),c("Backgrounds/initial","Backgrounds"),{name:"Backgrounds/complete",async:"background"},
 c("TexturesInfo/plan","TexturesInfo","plan",100),c("TexturesInfo/all","TexturesInfo","all",100),
 {name:"Textures/plan",async:"texture",args:[]any{"plan"}},
 c("Tasks","Tasks"),c("GuideFind","GuideFind"),c("GuideDownloadInfo","GuideDownloadInfo"),
 c("LogPage","LogPage",""),c("LogMonitor","LogMonitor",logpath),c("LogPattern","LogPattern",logpath,"E1"),c("LogCluster","LogCluster",logpath,"E1"),
 c("Detail/cached","Detail",id,false,false,false),c("ModCopies","ModCopies",id),c("LibraryCopy","LibraryCopy",id),c("LibraryFields","LibraryFields",id),c("Sections","Sections",id),
 c("TagCandidates","TagCandidates",[]string{id}),c("PosOptions","PosOptions",[]string{id},"",false),c("RowOps","RowOps",1,id),c("Palette","Palette",""),c("Keys","Keys","plan"),c("History","History"),
 c("Preview/sort","Preview","sort"),c("Preview/save","Preview","save"),c("Preview/launch","Preview","launch"),
 c("UninstallPlan","UninstallPlan",view.UninstallChoice{KeepLocked:true}),
 }
 for _,p:=range plans{cases=append(cases,c(fmt.Sprintf("PlanManage/id%d",p.ID),"PlanManage",p.ID))}
 if len(logs)>1{cases=append(cases,c("LogPage/previous","LogPage",logs[1].Path))}
 for _,x:=range cases{
  if filter:=os.Getenv("RIMARCHO_RESEARCH_MATCH");filter!=""&&x.name!=filter{continue}
  for _,mode:=range []string{"cold","entry"}{ if os.Getenv("RIMARCHO_RESEARCH_COLDONLY")=="1" && mode!="cold"{continue}
   s,close:=researchOpen(t,env)
   if mode=="entry"{if _,e:=researchInvoke(s,researchCase{async:"workspace"});e!=nil{t.Fatal(e)}}
   n:=1;if mode=="entry"{n=6}
   for i:=0;i<n;i++{start:=time.Now();v,e:=researchInvoke(s,x);elapsed:=time.Since(start);jt:=time.Now();b,je:=json.Marshal(v);jms:=float64(time.Since(jt).Nanoseconds())/1e6
    label:=mode;if i>0{label="hot"}
    row:=researchRow{Env:env,Name:x.name,Mode:label,Run:i,HandlerMS:float64(elapsed.Nanoseconds())/1e6,JSONMS:jms,Bytes:len(b)};if e!=nil{row.Error=e.Error()};if je!=nil{row.Error=je.Error()}
    enc.Encode(row)
    if i==0{fmt.Printf("RESULT %s %s %s %.3f ms json=%.3f bytes=%d err=%s\n",env,x.name,label,row.HandlerMS,row.JSONMS,row.Bytes,row.Error)}
   };close()
  }
 }
 }
}

// Detailed phase timings over an independent session, without frontend polling.
func TestResearchPhases(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()}
 _=context.Background();_=filepath.Separator
}
func TestResearchBreakdown(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh")
 for _,env:=range []string{"main","1.6"}{
 for run:=0;run<3;run++{
  s,close:=researchOpen(t,env);a:=s.a
  phase:=func(name string,f func()){start:=time.Now();f();fmt.Printf("PHASE %s %d %s %.3f\n",env,run,name,float64(time.Since(start).Nanoseconds())/1e6)}
  phase("Index/cold",func(){_,_=a.Index()})
  phase("Loadout/after-index",func(){_,_=a.Loadout()})
  st,_:=a.Loadout()
  phase("ModsCopies/disk",func(){_=a.ModsCopies(st.Ix)})
  phase("GameLinks/disk",func(){_,_=a.InspectGameLinks()})
  phase("Problems/first",func(){_,_=a.Problems(false,nil)})
  ps,_:=a.Problems(false,nil)
  phase("Needs/view",func(){_=view.BuildNeeds(a,st,ps)})
  phase("Problems/hot-with-disk",func(){_,_=a.Problems(false,nil)})
  phase("Board/view",func(){_,_=view.BuildBoard(a)})
  phase("Shell/hot",func(){_=s.Shell()})
  var r *app.App
  phase("ReadSession",func(){r,_=a.ReadSession()})
  r.EnvName=env
  phase("ReadIndex/copy",func(){r.ReadIndex(a)})
  phase("Loadout/after-copy",func(){_,_=r.Loadout()})
  var lib *app.Library
  phase("Library/build-warm-index",func(){lib,_=r.Library()})
  var page *view.LibPage
  phase("Library/render-all",func(){page,_=view.BuildLibrary(lib,"",nil,"")})
  phase("Library/JSON",func(){_,_=json.Marshal(page)})
  phase("Library/render-filter",func(){_,_=view.BuildLibrary(lib,"harmony",nil,"")})
  phase("Library/rebuild",func(){_,_=r.Library()})
  phase("Changes/after-index",func(){_,_=view.BuildChanges(r,st,modchanges.Opts{})})
  phase("Subscriptions/after-index",func(){_,_=view.BuildSubscriptions(r,st,modchanges.Opts{})})
  phase("Settings/Caches",func(){_=a.Caches()})
  phase("Settings/DBInfo",func(){_,_=a.DBInfo()})
  phase("Settings/Dependencies",func(){_=view.DependencyGroup(a)})
  phase("Settings/Directories",func(){_=view.DirectoryGroup(a)})
  phase("Settings/Folders",func(){_=view.FolderGroup(a)})
  phase("Settings/Accounts",func(){_=view.AccountGroup(a)})
  phase("Settings/Monitor",func(){_,_=a.MonitorState()})
  phase("Settings/All",func(){_=view.SettingsPage(a,nil)})
  phase("Home/first",func(){_=s.Home()})
  phase("Home/hot",func(){_=s.Home()})
  for _,p:=range mustPlans(a){phase(fmt.Sprintf("PlanLoadCost/id%d",p),func(){cost,e:=a.PlanLoadCost(context.Background(),p,nil);if e==nil{fmt.Printf("COST env=%s id=%d mods=%d cached=%d measured=%d\n",env,p,cost.Mods,cost.Cached,cost.Measured)}else{fmt.Printf("COSTERROR %v\n",e)}})}
  r.Close();close()
 }
 }
}
func mustPlans(a *app.App)[]int64{ps,_:=a.Plans();out:=[]int64{};for _,p:=range ps{out=append(out,p.ID)};return out}
func TestResearchProfile(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()}
 view.SetLang("zh");s,close:=researchOpen(t,"main");defer close();_,_=researchInvoke(s,researchCase{async:"workspace"})
 method:=os.Getenv("RIMARCHO_RESEARCH_PROFILE");args:=[]any{}
 switch method {case "Library":args=[]any{"",[]string{},""};case "SubscriptionsPage":args=[]any{true,""};case "PlanManage":args=[]any{int64(0)}}
 for i:=0;i<12;i++{_,e:=researchInvoke(s,researchCase{method:method,args:args});if e!=nil{t.Fatal(e)}}
}
func TestResearchExtras(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh")
 for _,env:=range []string{"main","1.6"}{
  s,close:=researchOpen(t,env);st,_:=s.a.Loadout();plan:=st.Plan.ID;id:=""
  for _,m:=range st.Ix.Mods{if m.Name=="Harmony"{id=m.ID;break}}
  var big string;s.a.Store.DB.QueryRow("select instance_id from def_fact where game_mm=? group by instance_id order by count(*) desc limit 1",st.GameMM()).Scan(&big)
  cases:=[]researchCase{{name:"Full/Harmony",async:"full",args:[]any{id}},{name:"Panel/editor",async:"editor-panel",args:[]any{id}},{name:"Panel/library",async:"library-panel",args:[]any{id}}}
  if big!=""{cases=append(cases,researchCase{name:"Full/largeDefs",async:"full",args:[]any{big}},researchCase{name:"Sections/largeDefs",method:"Sections",args:[]any{big}})}
  for _,f:=range view.ExportFormats(s.a){cases=append(cases,researchCase{name:"Export/"+f.ID,method:"PlanExport",args:[]any{plan,f.ID}})}
  for _,tex:=range []int{80,75,60,50,40,25}{cases=append(cases,researchCase{name:fmt.Sprintf("Build/%d",tex),async:"build",args:[]any{tex}})}
  cases=append(cases,researchCase{name:"Textures/all",async:"texture",args:[]any{"all"}},researchCase{name:"Filter/inactive",method:"Filter",args:[]any{0,"harmony"}},researchCase{name:"BarFilter",method:"BarFilter",args:[]any{"",[]string{},true}},researchCase{name:"RemovePlanWarn",method:"RemovePlanWarn",args:[]any{plan}},researchCase{name:"Live",method:"Live"})
  close()
  for _,c:=range cases{
   s,close:=researchOpen(t,env);_,_=researchInvoke(s,researchCase{async:"workspace"})
   for i:=0;i<4;i++{start:=time.Now();v,e:=researchInvoke(s,c);ms:=float64(time.Since(start).Nanoseconds())/1e6;j:=time.Now();b,_:=json.Marshal(v);jms:=float64(time.Since(j).Nanoseconds())/1e6;fmt.Printf("EXTRA %s %s %d %.3f json=%.3f bytes=%d err=%v\n",env,c.name,i,ms,jms,len(b),e)};close()
  }
 }
}

func TestResearchValidate(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()}
 view.SetLang("zh")
 for _,env:=range []string{"main","1.6"}{
 s,close:=researchOpen(t,env);_,_=researchInvoke(s,researchCase{async:"workspace"});st,_:=s.a.Loadout()
 n:=s.Needs();fmt.Printf("VALID %s active=%d mods=%d NeedsNever=%v NeedsStale=%v needsItems=%d\n",env,len(st.List),len(st.Ix.Mods),n.Never,n.Stale,len(n.Items))
 for _,f:=range view.LogFiles(s.a,st){p,e:=s.LogPage(f.Path);if p!=nil{fmt.Printf("VALIDLOG %s %s rows=%d pending=%s err=%v\n",env,filepath.Base(f.Path),len(p.Rows),p.Pending,e)}else{fmt.Printf("VALIDLOG %s nil %v\n",env,e)}}
 for _,g:=range s.Settings(){fmt.Printf("GROUP %s %s rows=%d\n",env,g.Title,len(g.Items))}
 d:=s.GuideDownloadInfo();fmt.Printf("VALIDGUIDE %s branches=%d error=%s\n",env,len(d.Branches),d.Error)
 close()
 }
}

func TestResearchIndependent(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh");debug.SetMemoryLimit(MemoryLimit)
 env:=os.Getenv("RIMARCHO_RESEARCH_ENV");if env==""{env="main"}
 method:=os.Getenv("RIMARCHO_RESEARCH_PROFILE");c:=researchCase{name:method,method:method}
 switch method{case "Workspace":c.async="workspace";case "Library":c.args=[]any{"",[]string{},""};case "SubscriptionsPage":c.args=[]any{true,""};case "UninstallPlan":c.args=[]any{view.UninstallChoice{KeepLocked:true}}}
 s,close:=researchOpen(t,env);defer close()
 start:=time.Now();v,e:=researchInvoke(s,c);ms:=float64(time.Since(start).Nanoseconds())/1e6;j:=time.Now();b,_:=json.Marshal(v)
 fmt.Printf("PROCESS %s %s %.3f json=%.3f bytes=%d err=%v\n",env,method,ms,float64(time.Since(j).Nanoseconds())/1e6,len(b),e)
}
func TestResearchSupplement(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh")
 s,close:=researchOpen(t,"main");defer close();_,_=researchInvoke(s,researchCase{async:"workspace"})
 rows,e:=s.a.Store.DB.Query(`select before_snapshot,after_snapshot from change_event where before_snapshot>0 and after_snapshot>0 and before_snapshot<>after_snapshot order by id desc limit 5`);if e!=nil{t.Fatal(e)}
 type pair struct{b,a int64};pairs:=[]pair{};for rows.Next(){var p pair;rows.Scan(&p.b,&p.a);pairs=append(pairs,p)};rows.Close()
 for _,p:=range pairs{for i:=0;i<4;i++{start:=time.Now();files:=s.ChangeFiles(p.b,p.a);fmt.Printf("DIFF %d %d %d %.3f files=%d\n",p.b,p.a,i,float64(time.Since(start).Nanoseconds())/1e6,len(files))}}
 for i:=0;i<3;i++{r,e:=s.a.ReadSession();if e!=nil{t.Fatal(e)};r.Home=filepath.Join(os.Getenv("APPDATA"),"RimArcho");start:=time.Now();p,e:=r.PlanUninstall(app.UninstallOptions{KeepLocked:true});fmt.Printf("REALHOME uninstall %d %.3f steps=%d err=%v\n",i,float64(time.Since(start).Nanoseconds())/1e6,len(p.Steps),e);r.Close()}
 st,_:=s.a.Loadout();id:="";for _,m:=range st.Ix.Mods{if m.Name=="Harmony"{id=m.ID;break}}
 h,_:=PageHandler(s)
 for _,path:=range []string{"/preview/"+id}{for i:=0;i<4;i++{rr:=httptest.NewRecorder();start:=time.Now();h.ServeHTTP(rr,httptest.NewRequest("GET",path,nil));fmt.Printf("ASSET %s %d %.3f status=%d bytes=%d\n",path,i,float64(time.Since(start).Nanoseconds())/1e6,rr.Code,rr.Body.Len())}}
}

func TestResearchLogAndPreviews(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh")
 for _,env:=range []string{"main","1.6"}{
 s,close:=researchOpen(t,env);_,_=researchInvoke(s,researchCase{async:"workspace"});st,_:=s.a.Loadout()
 id:="";for _,m:=range st.Ix.Mods{if m.Name=="Harmony"{id=m.ID;break}}
 cases:=[]researchCase{{name:"PackNameCheck",method:"PackNameCheck",args:[]any{"research-preview"}},{name:"PackDryRun",method:"PackDryRun",args:[]any{"research-preview",0}},{name:"ModNote",method:"ModNote",args:[]any{id}},{name:"GuideGame",method:"GuideGame",args:[]any{st.Ix.Env.GameRoot}},{name:"SubscribePreview/workshop",method:"SubscribePreview",args:[]any{SubscriptionRequest{Kind:"workshop",URL:st.Ix.ByID[id].WorkshopID()}}},{name:"LibraryPreview/lock",method:"LibraryPreview",args:[]any{"lock",[]string{st.Ix.Ref(id)}}}}
 for _,c:=range cases{for i:=0;i<4;i++{start:=time.Now();v,e:=researchInvoke(s,c);b,_:=json.Marshal(v);fmt.Printf("SAFE %s %s %d %.3f bytes=%d err=%v payload=%s\n",env,c.name,i,float64(time.Since(start).Nanoseconds())/1e6,len(b),e,string(b[:min(120,len(b))]))}}
 logs:=view.LogFiles(s.a,st)
 if env=="1.6" && len(logs)>0{r,_:=s.logSession();r.GameVersion=app.LogMM(app.LogVersionOf(logs[0].Path));start:=time.Now();_,e:=r.Analyze(nil);fmt.Printf("LOGANALYSIS %s %.3f mm=%s err=%v\n",env,float64(time.Since(start).Nanoseconds())/1e6,r.GameVersion,e);r.GameVersion="";r.Store.MM=st.GameMM()}
 for _,f:=range logs{for i:=0;i<4;i++{start:=time.Now();p,e:=s.LogPage(f.Path);if e!=nil{fmt.Printf("LOGFINAL %s %s error=%v\n",env,filepath.Base(f.Path),e);break};fmt.Printf("LOGFINAL %s %s %d %.3f rows=%d pending=%s\n",env,filepath.Base(f.Path),i,float64(time.Since(start).Nanoseconds())/1e6,len(p.Rows),p.Pending)
 if i==0 && len(p.Rows)>0{cid:=p.Rows[0].ID;start=time.Now();lines,e:=s.LogCluster(f.Path,cid);card:=s.LogPattern(f.Path,cid);fmt.Printf("LOGEVIDENCE %s %s %.3f cluster=%s lines=%d card=%v err=%v\n",env,filepath.Base(f.Path),float64(time.Since(start).Nanoseconds())/1e6,cid,len(lines),card!=nil,e)}}}
 close()
 }
}
func TestResearchPublicAndAssets(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh");s,close:=researchOpen(t,"main");defer close();_,_=researchInvoke(s,researchCase{async:"workspace"})
 s.a.HTTP=&http.Client{Timeout:12*time.Second};id:="#nvxs"
 for i:=0;i<3;i++{start:=time.Now();p,e:=s.Detail(id,false,true,i==0);n:=0;note:="";if p!=nil{n=len(p.Workshop);note=p.Note};fmt.Printf("PUBLIC Detail %d %.3f blocks=%d note=%s err=%v\n",i,float64(time.Since(start).Nanoseconds())/1e6,n,note,e)}
 for i:=0;i<3;i++{start:=time.Now();p,e:=s.SubscriptionPage("https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077");n:="";if p!=nil{n=p.Name};fmt.Printf("PUBLIC Page %d %.3f name=%s err=%v\n",i,float64(time.Since(start).Nanoseconds())/1e6,n,e)}
 s.Backgrounds();s.bgRead.workers.Wait();p:=s.Backgrounds();h,_:=PageHandler(s)
 for _,g:=range p.Groups{if len(g.Items)==0{continue};b:=g.Items[0];for _,path:=range []string{b.Thumb,b.Full}{if path==""{continue};for i:=0;i<3;i++{rr:=httptest.NewRecorder();start:=time.Now();h.ServeHTTP(rr,httptest.NewRequest("GET",path,nil));fmt.Printf("BGASSET %d %.3f status=%d bytes=%d\n",i,float64(time.Since(start).Nanoseconds())/1e6,rr.Code,rr.Body.Len())}};break}
}
func TestResearchNeedsFinal(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh");s,close:=researchOpen(t,"1.6");defer close();_,_=researchInvoke(s,researchCase{async:"workspace"})
 ch:=make(chan map[string]any,1);s.emit=func(n string,v any){if n=="analyze"{if m,ok:=v.(map[string]any);ok&&m["done"]==true{ch<-m}}}
 s.a.AnalyzeRunner=func(env string,progress func(string))error{a,e:=app.Open(os.Getenv("RIMARCHO_RESEARCH_HOME"));if e!=nil{return e};defer a.Close();a.EnvName=env;_,e=a.Analyze(progress);return e};s.busy["analyze"]=false
 start:=time.Now();n:=s.Needs();fmt.Printf("NEEDSFINAL initial %.3f stale=%v never=%v items=%d\n",float64(time.Since(start).Nanoseconds())/1e6,n.Stale,n.Never,len(n.Items))
 if n.Stale||n.Never{select{case e:=<-ch:fmt.Printf("NEEDSFINAL event %.3f value=%v\n",float64(time.Since(start).Nanoseconds())/1e6,e);case <-time.After(3*time.Minute):t.Fatal("analysis timeout")}}
 n=s.Needs();fmt.Printf("NEEDSFINAL complete %.3f stale=%v never=%v items=%d\n",float64(time.Since(start).Nanoseconds())/1e6,n.Stale,n.Never,len(n.Items))
 for i:=0;i<3;i++{start=time.Now();n=s.Needs();fmt.Printf("NEEDSFINAL hot %d %.3f stale=%v items=%d\n",i,float64(time.Since(start).Nanoseconds())/1e6,n.Stale,len(n.Items))}
}
func TestResearchPageGroups(t *testing.T){
 if os.Getenv("RIMARCHO_RESEARCH_HOME")==""{t.Skip()};view.SetLang("zh")
 for _,env:=range []string{"main","1.6"}{for _,page:=range []string{"HomeGroup","SettingsGroup"}{for _,mode:=range []string{"cold","entry"}{
 s,close:=researchOpen(t,env);if mode=="entry"{_,_=researchInvoke(s,researchCase{async:"workspace"})};n:=1;if mode=="entry"{n=4}
 for i:=0;i<n;i++{cs:=[]researchCase{};if page=="HomeGroup"{cs=[]researchCase{{method:"Home"},{method:"HomeBackground"},{method:"Setting",args:[]any{"ui.gui.home.hidden"}}}}else{cs=[]researchCase{{method:"Settings"},{method:"Backgrounds"},{method:"Tasks"}}};start:=time.Now();ch:=make(chan error,len(cs));for _,c:=range cs{go func(){_,e:=researchInvoke(s,c);ch<-e}()};for range cs{if e:=<-ch;e!=nil{t.Fatal(e)}};fmt.Printf("PAGEGROUP %s %s %s %d %.3f\n",env,page,mode,i,float64(time.Since(start).Nanoseconds())/1e6)};close()
 }}}
 s,close:=researchOpen(t,"main");defer close();_,_=researchInvoke(s,researchCase{async:"workspace"});id:="#nvxs";ref:="#nvxs"
 for _,kind:=range []string{"pin","pin-update","unpin","unhide","move","delete","unsubscribe","resubscribe","textures","textures-clean","subscription-remove","subscription-update"}{for i:=0;i<4;i++{start:=time.Now();p,e:=s.LibraryPreview(kind,[]string{ref});apply:=false;n:=0;if p!=nil{apply=p.Apply;n=len(p.Lines)};fmt.Printf("ACTION %s %d %.3f apply=%v lines=%d err=%v\n",kind,i,float64(time.Since(start).Nanoseconds())/1e6,apply,n,e)}}
 for _,argv:=range [][]string{{"mod",id},{"patches","--mod",ref},{"def","--mod",ref},{"harmony","--mod",ref},{"problems"},{"folder","check"}}{for i:=0;i<4;i++{start:=time.Now();lines,e:=s.Inspect(argv);fmt.Printf("INSPECT %s %d %.3f lines=%d err=%v\n",strings.Join(argv,"/"),i,float64(time.Since(start).Nanoseconds())/1e6,len(lines),e)}}
}


```

</details>
