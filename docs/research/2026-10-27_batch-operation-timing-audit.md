# Batch-operation timing audit (2026-10-27)

Conclusion: **not all batch actions are slow; worst are file/subscription operations synchronously preflighting each item before showing confirmation, sometimes holding shared GUI locks.** Real-inventory unsubscribe confirmation took 11.14 seconds for 100 entries, 98.57 seconds for 1,293. Plan/task-record DB changes are usually fast, but full post-edit refresh can take seconds. "Update all" has another per-item preview chain: 135 entries, 39.58 seconds.

## 1. Scope, machine, safety boundaries

- Main checkout `D:\Game\RimModManager_Upgrade\RimArcho`, branch `dev/0.9.0`, initial HEAD `887b668`. Read `../AGENTS.md`, `docs/00a-architecture.md`, `docs/specs/06-ui/` overview/layers/interactions/GUI technology/relevant pages.
- Real Windows host, AMD Ryzen 9 7950X (32 logical processors); Chrome `C:/Program Files/Google/Chrome/Application/chrome.exe`, headless=new, 1600×1000, independent temporary profile, Node native WebSocket/CDP. Followed `scripts/check-accounts-page.cjs`, **no `/dev/pagecheck` offline data**.
- Copied `%APPDATA%\RimArcho\archo.db*`, `config.toml`, real `tasks.json` to dedicated system temp. Main DB 357,650,432 bytes, copy `PRAGMA integrity_check=ok`. Edits use separate SQLite backups of copy.
- Real inventory: 5,183 lineages, 6,573 instances; original current plan 715 entries, 570 installed unofficial entries for edit benchmarks; disabled Workshop sample pool 2,244. Task history initially 36, up to 40 after real retry/source checks, no fake 100 tasks.
- Built `./cmd/archo`, started real page with requested `go run ./spikes/guidev -home <copy> -exe <archo.exe>`. Then Go `-overlay` built same harness for timing; overlay/logs/diagnostic JS/binaries only temporary, business code unchanged.
- Actual Mod paths still reference local content for read-only directory/link/PNG/process checks. Unsubscribe/delete/resubscribe/lock/move/update/DDS conversion/cleanup **never confirmed**; only dry-runs/dialogs. No real Steam subscriptions/Mod files changed; no real execution using `I Dont Need It`. Mods actually operated: **none**; resubscribe/recycle restoration needed: **none**.
- Writes only temporary DB/task records/tags/plans/export files. Source checks accessed real public Steam queries, updated copied metadata only, no downloads. Temporary tags removed, most edits undone per round, whole copy discarded finally.
- Other workers advanced main to `d64c974` (consecutive-Space refresh fix). Main assets frozen at initial version; later harness builds read then-current Go files. Audit of starting implementation, **not post-fix acceptance of d64c974/background-nonblocking**; cannot assert old refresh issues remain latest. Preserved other workers' changes/reports.

## 2. Measurement definitions

- First feedback: next requestAnimationFrame after MutationObserver detects related DOM change following original button/key handler; next paint opportunity, not monitor presentation. "Loading" counts as feedback, **not edit completion**.
- Dangerous operations with dialogs: total is click→preview complete; confirmation→actual completion always not executed. Dry-run time is not download/copy/recycle/Steam execution time.
- Ordinary edit endpoint: original handler/refresh Promise return; standalone API time labeled. Some pages return Promise before next visible frame, so first feedback may slightly exceed handler time.
- First 10, first 100, all eligible actual entries per class. All counts differ, not equal-work rankings. No valid instance/queue state: N/A, no fake Mods. Single observations/few reruns, no strict OS cache isolation, not P95/controlled comparisons.
- Diagnostic JS only exposes original module entries, sets selection, invokes original handlers; no API/algorithm replacement. Source page lacks multiselect pending rows: 10/100 slice real pending then original `changesKey(U)`; 135 clicks original update all. Import/missing 10/100 use original APIs, labeled. Original movement/filtered-move APIs additionally tested; standalone API times not whole-page times.
- Response probes: arrow-to-next-frame, maximum 10ms timer interval, `API.Setting('ui.lang')`, actual task/inventory switching. **Movable JS with API waiting on lock is still blocked UI operation.** guidev HTTP/1 queues amplify some waits; native Wails bridge lacks per-origin 6-connection limit, so absolute switch times not extrapolated.
- Go overlay only entry time.Now/deferred elapsed, handler/dispatch/index/Loadout/link/process/source/network/SetTags spans. Nested times overlap, **never sum them as total cost**. Timing/log IO overhead not subtracted.

## 3. Batch-entry inventory and coverage

Sources: `assets/js/{library,lists,main,ops,planmanage,subscriptions,tasks,textures,exchange,screens,credit}.js`, `internal/view/ops.go` opTable, GUI→app/store chains.

| Entry/action | Frontend and Go API | Coverage/boundary |
|---|---|---|
| Multiselect enable/disable, restore staged | main.verb / Toggle | 10/100/570; disabled pool 10/100/2244, latter two rejected by real duplicate packageId |
| Enable/place at specified position, drag into enabled | main.place/drop / Place | Position dialog, in-list Place, enable from disabled 10/100/570; disable legal originals first, reenable, undo each round |
| Up/down, ±10, filtered movement, same-column drag | MoveBy / MoveShown / Place | Up/down/up 10/down 10/place 10/100/570; original MoveShown API 10/100/570 with alternating real-list positions |
| Multiselect stage/restore/drag out | Stage / Unstage / Toggle | Enabled 10/100/570, disabled 10/100/2244; actual copied-DB edits |
| Stage all / restore all | StageAll / RestoreAll | 570 stageable of 715; 3 reruns |
| Add/remove multiselect tags | pop.tags / TagCandidates / SetTag | Editor 10/100/570, inventory 10/100/6573; temporary tags added/removed |
| Remove from all plans | RunOp('remove') / RemoveFromPlans | 10/100/570; copied plans only, notice affects 3–5 plans |
| Sort whole plan | main.run / Run('sort') | Original 715-entry handler including refresh, not per-item transactions |
| Lock version | LibraryPreview/LibraryDo('pin') | dry-run 10/100/6559 |
| Update to Workshop | Same 'pin-update' | dry-run 10/100/149, distinct from source update |
| Unlock | Same 'unpin' | N/A: eligible=0, no real locked copies created |
| Move into lock library | Same 'move' | dry-run 10/100/149 |
| Unhide | Same 'unhide' | dry-run 10/13; 100 N/A |
| Unsubscribe/resubscribe | Same 'unsubscribe'/'resubscribe' | dry-run 10/100/1293 or 1294 |
| Recycle | Same 'delete' | dry-run 10/100/3760 |
| Remove subscription source | Same 'subscription-remove' | dry-run 10/100/1503; distinct from Steam unsubscribe |
| Multiselect source check/update | 'subscription-check'/'subscription-update' | Initially no eligible; checks yielded 135 pending; source-page previews 10/100/135; inventory entry not independently rerun |
| Source check all / one source | checkAll / CheckSubscriptionSource | Real 1294+1503; no separate 10/100 network subsets; individual durations from tasks/Go logs |
| Source update all | updateRows / WorkshopUpdatePreview or LibraryPreview | Real pending 10/100/all 135; all SteamCMD, no pending workshop branch sample |
| Import/collection missing/diagnostics subscribe all | exchange.getAll, screens.runAction / Subscribe(batch=true) | Original selector 10/100/570, no Steam/SteamCMD execution; diagnostics has 91 subscribable missing entries, not submitted |
| Diagnostics group enable/disable/remove missing | screens.doAction / DoAction | Real missing group remove10/100/143, API timing; no applicable dependency-enable group; shared edit layer |
| Diagnostics repair all deployment links / unlink packs | DoAction repair-links / unlink-packs | No applicable batch card; source trace only, no real Mods writes |
| Texture statistics (plan/library) | textures.count / Textures, TextureMeasure | Real targets8/98/570, library6573, actual PNG reads |
| DDS batch convert/clean | TexturesRun / TexturesClean; library textures-clean | Plan/library dialogs, not confirmed; inventory textures/clean eligible=0, no DDS fabricated. Conversion writes cannot be measured by statistics |
| Whole-plan 8-format export/save/copy | PlanExport / PlanExportSave / PlanExportCopy | Real10/100/715, original per-format buttons; temporary save API; clipboard source trace only |
| Import new/merge/replace | ImportPreviewFor / ImportAsPlanPreview / ImportApplyTo | Original exported slices10/100, preview/new API; real installed merge10/100/570, replace10/100/715, verify counts |
| Followed-collection sync additions/removals | PlanSync / PlanFollow | No suitable real pending collection, none fabricated; trace only |
| Task multiselect stop/cancel/retry/resume/up/down | tasks.run / TaskAct | Real forget/undo/clear, retry3 failed community-data tasks; no valid batch active stop/resume/up/down, no subscription retries |
| Task multiselect remove/clear completed/undo | TaskAct forget/clear/undo | 10/all 40, clear29; 100 N/A |
| SteamCMD batch Steam credit/replace | credit.js / CreditAnswer(batchIDs) | Identified entry, no safely confirmable due sample; review only |
| Full scan/analyze/build; whole-plan save/launch | Scan/Analyze/Build/Run | Scope tasks with real IO; scheduling/lock trace only, no extra game writes or execution-time coverage |

Notes, version/copy switch, unlink classification, open directory/web etc. have opTable `One=true`, **not multiselect batches**. Inventory has no plan enable/disable; editor handles them.

## 4. Measurements: preconfirmation preview

Seconds, **first feedback / preview return**. Actual local content per cell; dialogs closed, execution unmeasured. All means operation-eligible, not blindly all 6573.

| Operation | 10 | 100 | All (count) | UI while waiting |
|---|---:|---:|---:|---|
| Unsubscribe | 1.236 / 1.240 | 11.140 / 11.148 | 98.570 / 98.585 (1293) | Keys move; shared API lock queued |
| Resubscribe | 0.363 / 0.367 | 3.176 / 3.182 | 39.987 / 40.003 (1294) | Keys move; shared API lock queued |
| Lock version | 0.123 / 0.131 | 0.776 / 0.780 | 111.844 / 111.909 (6559) | Keys move; shared API lock queued |
| Update to Workshop | 1.308 / 1.311 | 11.565 / 11.570 | 22.722 / 22.727 (149) | Keys move; shared API lock queued |
| Move into lock library | 0.900 / 0.907 | 8.751 / 8.759 | 36.092 / 36.106 (149) | Keys move; shared API lock queued |
| Unhide | 0.131 / 0.150 | N/A | 0.127 / 0.133 (13) | Keys move; shared API lock queued |
| Remove subscription source | 4.325 / 4.330 | 14.114 / 14.118 | 121.034 / 121.045 (1503) | Keys move; Setting not persistently blocked |
| Recycle | 0.643 / 0.647 | 6.208 / 6.214 | 234.330 / 234.341 (3760) | Keys move; shared API lock queued |

Reruns: unsubscribe10 0.679/0.683s, resubscribe10 0.341/0.346s. Cache/sample sizes/directory shapes affect slope; do not linearly extrapolate10 to library.

Source-page update all separately:

| Pending count | First dialog feedback | Preview return | Setting response | Actual update |
|---:|---:|---:|---:|---|
| 10 | 3.127s | 3.133s | 1.7ms | Not confirmed |
| 100 | 32.107s | 32.114s | 1.4ms | Not confirmed |
| 135 | 39.583s | 39.590s | 2.5ms | Not confirmed |

### 4.1 UI-response evidence

During all-item unsubscribe/resubscribe/lock/Workshop update/move/recycle, Setting waited98.49/39.90/111.21/22.64/36.03/234.18s, nearly matching preflight. Cursor feedback mostly17–39ms, maximum timer usually<140ms: backend API lock, not continuous JS saturation.

During all-delete wait, task-page readiness120.56s; then library skeleton24ms/readiness331ms. Task reads do not hold `s.mu` throughout, so do not blame Tasks() from this number; Setting/Leave and harness HTTP queue contribute. **Direct lock evidence: LibraryPreview source and independent Setting probe.**

Source removal/update all already use detached readers: all-item Setting16.4ms/2.5ms, but no checking status, leaving blind121s/39.58s waits. Unlocking improves operability, not repeated work.

## 5. Measurements: plan edits, tags, diagnostics

### 5.1 Multiselect editing

Additional validation round (API success checked), **first feedback ms / handler+refresh seconds**. Initial rounds0.88–1.27s, validation1.84–4.36s: refresh/cache state matters, not stable per-Mod linear cost.

| Operation | 10 | 100 | All570 | Result/response |
|---|---:|---:|---:|---|
| Disable | 62.2 / 1.886 | 46.9 / 2.205 | 314.0 / 4.358 | Success, responsive, then refresh |
| Reenable just-disabled | 61.9 / 2.071 | 62.0 / 2.702 | 62.1 / 3.357 | Success, responsive, then refresh |
| Stage | 103.1 / 2.069 | 115.7 / 3.315 | 85.1 / 2.473 | Success, responsive, then refresh |
| Restore staged | 86.5 / 1.840 | 91.3 / 4.319 | 63.2 / 2.223 | Success, responsive, then refresh |

All-disable validation first 314ms exceeds200ms; Toggle22.4ms, later BarFilter2.056s. Whole-page 4.36s is not disable transaction time.

| Other operation | 10 first ms/total s | 100 first ms/total s | All first ms/total s | Notes |
|---|---:|---:|---:|---|
| Move down | 107.0/1.078 | 102.2/1.085 | 110.4/1.130 (570) | Initial, MoveBy |
| Open position dialog | 10.1/0.010 | 11.1/0.011 | 7.1/0.007 (570) | Local dialog |
| Apply position | 67.4/1.004 | 72.8/1.048 | 70.5/0.993 (570) | Place |
| Stage disabled | 46.9/1.288 | 44.3/2.904 | 57.7/1.231 (2244) | Stage |
| Drag staged back to disabled | 50.1/0.999 | 44.1/0.984 | **539.9/1.883 (2244)** | Full local update/render, timer gap533ms |
| Stage all | — | — | 100–106/0.835–0.925 | Actual570 of 715 |
| Restore all | — | — | 13–15/0.943–1.219 | 570 |
| Disable in all plans | 52.5/0.045 | 46.5/0.046 | 50.5/0.043 (570) | RunOp time, excludes full-page readiness |
| Whole-plan sort | — | — | Total1.471 (715) | Original Run+refresh, first feedback unrecorded |


Additional move/enable-position variants: subsets of original570 legal enabled entries, undo each round; first ms/handler+refresh s, MoveShown API labeled.

| Operation | 10 | 100 | All570 |
|---|---:|---:|---:|
| Move up1 | 21.9/1.133 | 13.5/1.150 | 17.2/1.270 |
| Move up10 | 12.4/1.126 | 13.5/2.088 | 12.1/1.092 |
| Move down10 | 12.3/1.304 | 10.8/1.277 | 12.2/1.050 |
| Enable legal disabled at20 | 30.0/1.122 | 30.9/1.098 | 56.8/1.102 |
| Filtered MoveShown (API only) | 43.2ms | 47.2ms | 46.3ms |

No errors, movement APIs returned expected counts/positions. API-only tests lack long navigation/input waits; tens of ms do not prove no blocking at larger scale.

Enable disabled pool: 10 succeeded, Toggle156.9ms, page 2.032s; 100/2244 rejected in98.2/100.4ms by `pkg.duplicate-installed` (another `[NL] Facial Animation - Experimentals` copy won). Rejected page completion0.942/0.968s, **not successful-enable performance**.

### 5.2 Tags

| Scope/count | Tag dialog first ms | Submit first ms | SetTags/SetTag | Submit→write done | Refresh observation |
|---|---:|---:|---:|---:|---|
| Editor 10 | 18.8 | 15.3 | Go SetTags1.0ms | Original handler+refresh1.208s | Responsive |
| Editor 100 | 11.6 | 26.0 | Go SetTags3.7ms | Original handler+refresh2.227s | Responsive |
| Editor 570 | 338.6 | **2206.0** | Go SetTags **2176.8ms** | Original handler+refresh3.840s | Setting~2146ms, lock wait |
| Inventory10 | 33.6 | 33.7 | SetTag API11.7ms | 46.4ms | Verified page ready1.622s |
| Inventory100 | 240.4 | 20.7 | SetTag API8.3ms | 33.2ms | Verified page ready1.497s |
| Inventory6573 | 214.6 | 296.5 | SetTag API281.9ms | 300.0ms | Verified page ready1.926s |

Inventory deduplicates by lineage, at most5183 families written. Rounds show state/SQLite differences; **tags are not always<300ms**. Removing all temporary tags Go SetTags1.721s exposes autocommit variance. Inventory verified readiness includes extra measurement `Library.open()`, potentially overlapping original `load()`, not strict product endpoint. One Library reread1.25–1.46s.

### 5.3 Diagnostics: remove missing entries in batch

Real group143. Original DoAction on real-ref10/100 slices+refresh: first 63.5/55.8ms, total2.120/1.981s. All143 original API return/result16.3ms ("removed143 mods from the list"), excludes full refresh. Undo each round. API/page times kept distinct; full card not expanded, no claim its button clicked.

## 6. Measurements: background statistics/network, tasks, export

### 6.1 Source checks and textures

| Operation | Actual count | First feedback ms | Total s | Response/result |
|---|---:|---:|---:|---|
| Check all sources | Workshop1294 + SteamCMD1503 | 17.7 (queue ack11.6) | **219.480** | Tasks page 52.4ms; both finished with 9/14 query failures |
| Texture statistics: small plan | 8 unofficial Mods | 14.3 | 5.041 | Setting2.2ms, timer gap<51ms |
| Texture statistics: medium | 98 unofficial Mods | 12.7 | 4.087 | Setting2.4ms, responsive |
| Texture statistics: original | 570 unofficial Mods | 12.2 | 39.015 | Setting14.3ms, responsive |
| Texture statistics: inventory | 6573 instances | 25.4 | **179.634** | Setting53.1ms, max gap902.9ms |
| Texture conversion dialog | Above8/98/570 targets | 19/27/48 | Dialog only | No DDS generated |
| DDS cleanup dialog | Same scope | 16/18/23 | Dialog only | No DDS removed |

The 10/100 texture samples retain 10/100 installed entries, including 2 official entries, excluded from texture targets: actual8/98, not10/100. Full library: 1870 PNG-result rows, 281462 PNGs, about 8.9GB, 241048 awaiting conversion; estimated DDS49.7GB makes actual conversion unreasonable and unauthorized for ordinary Mods. Copy lacked statistics cache; small→large warms OS cache; 98 faster than 8 does not mean larger work faster.

Two full-source tasks about 100/118 seconds; Go `fetchWorkshop`2797 actual calls, total216.416s; two `CheckSubscriptionSource`218.818s. Failures real, no fake responses. Checks yielded135 real pending SteamCMD updates for §4.

### 6.2 Tasks and subscribe-all selector

| Operation | Count | First/return/completion ms | Result |
|---|---:|---:|---|
| Multiselect task removal | 10 | 22.5 | Actually10 removed |
| Undo removal | 10 | 20.0 | 10 restored |
| Remove all tasks | 40 | 33.1 | 40 removed, Task API empty |
| Undo removal | 40 | 16.7 | 40 restored |
| Clear completed | 29 | 31.4 | Completed removed |
| Undo clear | 29 | 41.7 | Restored |
| Retry selected failed tasks | 3 data-update | 35.4 queue | **Only1 new task** |
| Subscribe all: method selector | 10/100/570 | 14.5/21.0/23.7 | Dialog only, no execution |

Removal/undo fast. `TaskAct` retry/resume/up/down only reads `ids[0]`, so fast multiselect retry omits selections, not high throughput. stop iterates ids, forget uses batch Forget, clear removes ended records. No100-task fabrication/real subscription retry.

Additional boundary: harness empty `rows:null` makes tasks.js drawList `X.page.rows.length` throw; original TaskAct(undo) restored40. Empty-state compatibility, separate from performance.

### 6.3 Import/export

Take original715-entry RimArcho JSON export's first 10/100; original ImportPreviewFor+ImportAsPlanPreview writes temporary plans; parse export entries to verify10/100, no fabricated Mods.715 uses original. Some original entries missing, supported by export.

| Count | Import preview API | New-plan API | Export first feedback | Export API/handler return | Format-switch completion range | Temporary save API |
|---:|---:|---:|---:|---:|---:|---:|
| 10 | 4.1ms | 1.9ms | 21.2ms | 51.4ms | 40.0–51.6ms | 4.4ms |
| 100 | 11.5ms | 2.7ms | 16.6ms | 41.4ms | 43.9–55.0ms | 3.8ms |
| 715 | — | — | 25.3ms | 6.1ms | 44.8–98.2ms | 7.4ms |

8 formats: RimArcho, RimSort, RML, RimCrow XML, share code, plain text, ModsConfig, Workshop links. Real `.fmt` clicks end at preview DOM replacement, excluding artificial100ms waits. First feedback may only be selected-button state; full replacement also reported.715 handler6.1ms, next frame25.3ms reflects endpoints, not negative overhead. Saves only dedicated temp, no game ModsConfig overwrite; API starts at submitted path, excludes picker time. Clipboard/native pickers/collection sync not separately end-to-end tested. Merge/replace below time original APIs, excluding forms/parsing/preview/refresh.


| Import mode | Input count | Entries before→after | Original API ms |
|---|---:|---:|---:|
| Merge real installed | 10 | 0→10 | 1.7 |
| Merge real installed | 100 | 0→100 | 1.5 |
| Merge real installed | 570 | 0→570 | 2.5 |
| Replace (keep real missing) | 10 | 2→10 | 1.6 |
| Replace (keep real missing) | 100 | 2→100 | 1.5 |
| Replace (keep real missing) | 715 | 2→715 | 3.0 |

Additional finding: merge skips uninstalled; the first 10/100 of the original 715 entries merged only 2 official entries, so invalidated/replaced with installed Workshop10/100/570, actual counts verified. Replace supports missing, retains real 10/100/715, target initially2 to avoid no-op. Temporary plans only, no game writes.

## 7. Slow-operation ranking

By completion at each real full scope, **not equal-count throughput**. Preview rows do not execute real deletion/unsubscribe.

| Rank | Operation/stage | Count | Completion | First feedback | Nature |
|---:|---|---:|---:|---:|---|
| 1 | Recycle preview | 3760 | 234.34s | 234.33s | No status, shared lock |
| 2 | Check all sources | 2797 | 219.48s | 17.7ms | Responsive background network, partial failures both sources |
| 3 | Inventory textures | 6573 | 179.63s | 25.4ms | Responsive background IO, large-table rendering pauses |
| 4 | Remove source preview | 1503 | 121.04s | 121.03s | No status, shared lock released |
| 5 | Lock preview | 6559 | 111.91s | 111.84s | No status, shared lock, real tree scans |
| 6 | Unsubscribe preview | 1293 | 98.59s | 98.57s | No status, shared lock |
| 7 | Resubscribe preview | 1294 | 40.00s | 39.99s | No status, repeated process checks |
| 8 | Update-all preview | 135 | 39.59s | 39.58s | Per-item frontend requests, no status |
| 9 | Move-to-lock preview | 149 | 36.11s | 36.09s | No status, shared lock |
| 10 | Workshop update preview | 149 | 22.73s | 22.72s | No status, shared lock |

100-item first feedback: update-all 32.11s > source-remove14.11s > Workshop-update11.57s > unsubscribe11.14s > move8.75s > delete6.21s > resubscribe3.18s > lock0.78s. Small batches already exceed200ms.

Nonpreview work: editor570 tags first 2.206s/SetTags2.177s; inventory-all tags first 297ms/removal Go 1.721s; unstage2244 first 540ms; all-disable validation314ms; full-texture timer gap903ms. Most edits show status quickly yet full refresh0.9–4.4s.

## 8. Go handler breakdown and causes

### 8.1 Repeating global preflight N times (main cause)

Chain `library.js:run` → `gui/library.go:LibraryPreview` → `view/library.go:LibPreview`. LibPreview loops refs, calls `cmds.Execute(... --dry-run)` then `usedWarning` per item, not real batch preflight.

| Measurement | Handler total ms | Repeated parts (count/total ms) |
|---|---:|---|
| Unsubscribe10 rerun | 663.5 | readIncomingLinks10/334.0; GameRunning 10/291.4; usedBy20/14.3; Execute10/650.0 |
| Resubscribe10 rerun | 324.8 | GameRunning 10/307.8 |
| Delete100 | 6191.2 | readIncomingLinks100/2952.8; planIncomingLinks100/2921.3; usedBy200/148.7 |
| Delete3760 | 234229.8 | readIncomingLinks3760/104689.9; planIncomingLinks3760/106642.8; usedBy7520/6810.6; Execute3760/224150.9 |
| Move100 | 8724.2 | readIncomingLinks100/3865.3; planIncomingLinks100/3418.8; usedBy100/137.5 |
| Workshop-update100 | 11544.6 | readIncomingLinks100/3126.5; planIncomingLinks100/2886.9; remainder includes read-only copyTree scans |
| Source-remove1503 | 120953.4 | readIncomingLinks1503/51025.8; usedBy3006/2475.7; Execute1503/118396.4; one loadIndex 209.2 |
| Lock6559 | 111281.5 | PinFor6559/91072.3 (max7851.1); Execute6559/95482.0 |

Locations (audit-time main-checkout lines):

- `internal/app/linklifecycle.go:51 readIncomingLinks`: InspectGameLinks, enumerate every game's Mods links, normalize paths each item, not this ref alone. `planIncomingLinks:177` recreates probe/rechecks game processes.
- `internal/app/gamerun.go:49 GameRunning`: probe cache within one closure only, not across Mods; Windows process enumeration~30ms. About 95% of resubscribe10 handler.
- `internal/app/workshopsub.go:Workshop`: GameRunning each preview; unsubscribe also checks uses/links. `libraryact.go:252 usedBy` → `library.go:614 libUses` rebuilds all plan/pack references; `usedWarning` asks again, twice per Mod.
- `internal/app/pin.go:119 PinFor` → `copylocal.go:copyLocal/copyTree`: dry-run enumerates real trees, highly uneven6559 directory sizes, not merely DB slowness.19677 confirmation lines, additional563ms handler→feedback includes bridge/transfer/text rendering, not all JS freeze.

Recommend one BatchPreviewContext for Index/Loadout/global uses/links/process snapshots; per Mod only joins/necessary own-tree scan, one final confirmation. **Revalidate processes/links/target identity before execution**, no permanent safety cache. About 211 seconds of delete-all repeated link/process work could mostly disappear by N→1; tree scans/real IO remain. Structural estimate, no implemented/accepted speed ratio.

### 8.2 Late dialog and GUI lock (direct background-branch overlap)

`assets/js/library.js:600 run` uses `await LibraryPreview` before pop.confirm, no visible preflight. `gui/library.go:117 LibraryPreview` only subscription-update/remove use independent ReadSession; others hold `s.mu` throughout, blocking other lock users.

Recommend immediate closeable "checking N items", disabled confirm, request identity against late results; independent read-only session/short snapshot lock. First-feedback target<200ms, but **dialog/unlock alone does not shorten preflight**, combine8.1.

### 8.3 Update all split into frontend single-item calls

`assets/js/subscriptions.js:459 updateRows`, line467 per-item await preview, line472 per-item WorkshopUpdate/LibraryDo after confirm. Each SteamCMD preview builds ReadSession → Loadout/loadIndex → `subscribe.go:271 selectSubscriptions` → all Subscriptions filtered to one ref.

100 measured entries made 100 LibraryPreview calls totaling 31.924s, loadIndex 100/17.293s; Loadout 22.978s includes index, not additive; Subscriptions 100/4.635s, selectSubscriptions 4.788s includes it; GameRunning 100/3.232s.135 handlers39.356s, index135/21.412s, selection135/6.005s, GameRunning 4.172s, matching page 39.59s.

Send all real refs once, preview in one read context, submit one batch preserving existing root/account grouping. Frontend single submissions defeat aggregation. Index/source filtering100/135→1 expected; network/copy still measured separately. No pending workshop-update sample, source inference only.

### 8.4 Metadata one request per Mod, duplicate across sources

`internal/app/subscriptionupdates.go:43 CheckSubscriptionSource` calls `describe.go:223 fetchWorkshop` each entry, public GetPublishedFileDetails `itemcount=1`.2797 calls216.4s; same ID repeated across Workshop/SteamCMD. checkAll queues two actually serial self tasks.

Use public multiitem parameters, reasonable groups e.g.100, deduplicate IDs across sources, bounded concurrency/item errors/retries/progress.2797 un-deduplicated requests become~28 batches: count estimate, not proportional219-second guarantee. No credential scraping/private APIs.

### 8.5 Per-tag autocommit, heavy refresh after light writes

`app/playerdata.go:15 SetTags` deduplicates lineages then loops; `store/playerdata.go:73 AddTag/:83 RemoveTag` independent DB.Exec, no batch transaction. Editor: 570 tags took 2.177s, all-remove1.721s, all-add282ms; write state changes cost.

Recommend short transaction/reused statement, retain dedup/item errors, invalidate once. `gui/api.go:SetTag` holds s.mu; shorter writes reduce lock waits. Acceptance tests actual adds/removals, not existing-tag no-ops.

Editor `main.js:edited/reload` → `lists.js:openEditor/refilter` full rereads and `gui/ops.go:15 BarFilter` new session/board rebuild add fixed cost. Validation Toggle/Stage9–218ms, Workspace2–20ms, BarFilter806–2095ms. Recommend local state updates/recompute affected columns/filters using real revisions, not stale state. d64c974 already changes refresh UX; remeasure there, not duplicate an unfixed claim.

`main.js moveLocal/list rendering` unstaging2244 causes533ms timer gap, frontend computation/render. Profile selection/rows/layout before Set/Map indexes/batched DOM; gap alone cannot identify one loop.

### 8.6 Static execution risks (not measurements)

`gui/tasks.go:371 taskBatch` already batches delete/unhide via batchFiles then one Scan; Steam unsubscribe/resubscribe via batchSteam, **50 per Steam call**, not launch per Mod. Still preview-loop/scan s.mu risks; pin/move via taskItem/LibDo, `pin.go:makeLock` scans each; `subscribe.go:494 RemoveSubscriptions` scans per item and runs `a.Scan(false)` afterward.

Recommend prepare batch→individual file changes→one scan/reconciliation, preserve interruption/failure progress. Real download/copy/recycle throughput needs recoverable authorized `I Dont Need It` acceptance separately; no execution seconds/postfix gains here.

### 8.7 Texture statistics and large result table

textures.js count already immediate/background/polling, not preconfirm silence.6573-instance PNG statistics~180s, 1870 nonvirtual result rows, final/incremental max gap903ms. Fingerprinted directory caches/batch reuse, chunked/virtual table; Performance trace needed before specific cause, timer gap not proof drawing alone.

### 8.8 Batch retry only first task

`gui/tasks.go:165 TaskAct` retry/resume/up/down `ids[0]`, frontend supplies all selections. Process all eligible in one batch/save, or explicitly single-select; define relative order for multiple moves. Functional omission needs own issue, not hidden by ms result. Normalize empty rows=null to array too.

## 9. Overlap with codex/background-nonblocking

Read-only review of `.claude/worktrees/background-nonblocking` uncommitted diff near end, still based887b668. No build/run/edits there: collaboration boundary, **not promise of final implementation/verified fixes**.

| Suggestion | Visible overlap | Acceptance focus |
|---|---|---|
| Immediate library preflight/close/late-result isolation | Direct: popups.libraryConfirm, library.js/fulldetail.js | 10/100/all first<200ms, no bypass during checks |
| Detached LibraryPreview, no GUI lock during file/Steam tasks | Direct: gui/library.go/tasks.go/readers.go/WorkspaceSnapshot/filesMu | Navigation/Setting do not wait whole task; execution safety recheck |
| Real batch source-removal/final scan/save | Direct: removebatch/batchRemoveSubscriptions/subscribe.go/store | Authorized tagged set only, restore, verify Scan count/item errors |
| Global link/process/use snapshots, final pin/move scan | Adjacent: diff not comprehensively covering per-item LibPreview CLI/all pin/move scans | Unlock is not total-time fix; remeasure readIncomingLinks/probes |
| One source updateRows batch preview/submit | No subscriptions.js change seen, extra work | For 100 entries, reduce Index builds from 100→1; preserve SteamCMD grouping |
| Batched metadata/cross-source dedup | No subscriptionupdates.go/describe.go batch change | Same real 2797, record request/failure counts |
| SetTags transaction/local refresh, ids[0], texture rendering | No complete fix visible; refresh separately d64c974 | Verify writes/refresh/multiselect separately, avoid enlarging worker scope |

Priority: merge/accept feedback/unlocking first (largest perceived benefit), then eliminate repeated global preflight/single-item update chains, then metadata/tag transactions/large tables. All expectations need same-real-data/same-eligible-count reruns.

## 10. Reproduction and cleanup

Dedicated temp home/profile, no checkout business edits. Simplified commands (replace with actual local paths):

```powershell
$auditRoot = Join-Path $env:TEMP ('RimArcho-batch-audit-' + [guid]::NewGuid())
# Create directories; copy APPDATA/RimArcho archo.db*, config.toml, tasks.json to $auditRoot/home
# SQLite integrity_check the copy; ensure read-only previews never confirm external actions
$exePath = Join-Path $auditRoot 'archo.exe'
go build -o $exePath ./cmd/archo
go run ./spikes/guidev -home (Join-Path $auditRoot 'home') -exe $exePath
# Separate headless Chrome profile; get page.webSocketDebuggerUrl via /json, CDP Runtime.evaluate
```

CDP measurement (original page API/modules, not mock):

```javascript
const t0 = performance.now();
let first;
const observer = new MutationObserver(() => {
  // Record next paint opportunity only when this operation's status/dialog/progress appears
  requestAnimationFrame(() => { first ??= performance.now() - t0; });
});
// Observe #popup / #status / current result; set original selection, click original button
// Also 10ms timer, arrow cursor, Setting('ui.lang') probe after 50ms
// Await original handler/final state, record total; close dialog without execution callback
```

Go breakdown rerun: time.Now/deferred elapsed at §8 entries, **system-temp overlay files only, compile `go build -overlay <json>`**, no checkout timing changes. Group by LibraryPreview completion, network logs separate, exclude Live/other reads, nested functions separate. Update100/135 stats exclude source-page initialization; initial10 had earlier reads, so only its handler plus clean100/135 details reported.

Corrected three invalid readings: rejected disabled enables not successes; missing await on task load left stale selections (initial all actually removed27), replaced by actual40 remove/undo; old export artificial100ms waits and mixed installed/missing counts discarded for entry-verified10/100/715. No conclusions from invalid numbers.

Research adds report only, no code commits. Temp harness/profile/DB/overlay/logs cleaned after evidence aggregation; do not stop other workers/roll back concurrent files. Existing 3 reports from Oct 26 and other worker's GUI/CLI report preserved.
