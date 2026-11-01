# SteamCMD batch downloads followed by subscription: research

> 2026-10-26, local Windows research; initial measurements about 19:53–20:15 PDT; §10 adds tests later that evening (UTC−07:00). Code baseline: main checkout, `dev/0.9.0`, HEAD `81017d85f73f504ed59d4701c94ccd907363d8f6`. Only this report added; no business-code edits or commits. Author credit is the primary constraint.

## 1. Decision summary

**For "author credit + ultimately maintaining one content copy", recommend durable obligations for all real subscriptions, with Steam client exclusively maintaining Workshop content; CMD pre-seeding does not guarantee no redownload or immediate playability.** Every selected ID creates a subscription obligation, even on download failure. If the client is absent, the app exits, or calls partly fail, persist pending work and resume in the original account context.

§10 fills the missing condition: after clearing all six experimental Mods' content directories, `WorkshopItemsInstalled`, and `WorkshopItemDetails`, download the same versions anonymously with CMD, pre-seed, then `SubscribeItem`. The client still retransmits content and replaces seeded files; identical final hashes do not mean files remained unchanged during processing. The earlier "zero content transfer" in §4.3 applies only with retained installation records, not first installation.

**Asynchronous does not mean it is safe to read the same files being committed.** Valve requires waiting for download completion callbacks before accessing content. Background preparation need not block UI or stop an ongoing game, but new Mods become launch-ready only after client processing finishes. Do not promise use during redownload/commit. Immediate use of CMD content while the client processes requires another stable copy, conflicting with strictly "one copy at all times".

Author credit continues through existing `ISteamUGC::SubscribeItem`, which establishes a real account subscription. Initial observation showed public Current Subscribers fall by 1 after unsubscribe; resubscribe succeeded, but the page then rate-limited, preventing an unconfounded "exactly +1" observation. Anonymous or authenticated CMD download alone never replaces subscription; same-account resubscription adds no new unique subscriber.

Speed is not the decision criterion for this goal. Initial four-item active-client totals: 21.275 / 22.567 seconds; independent CMD source followed by subscription and client completion: 32.065 / 29.615 seconds. §3–§7 retain measurements/tradeoffs; current recommendation follows this section, §8, §10. Research only; no business-code edits or commits.

## 2. Scope, current state, and prior research

Read [workspace rules](../../../AGENTS.md), [architecture](../00a-architecture.md), [02 §3.4](../specs/02-environments-identity-and-scanning.md#34-subscriptions-git-download-pages-steamcmd-2026-10-24), [login session research](2026-10-24_login-sessions-loverslab-steamcmd.md), [parallel download research](2026-10-26_steamcmd-parallel-downloads.md). Do not repeat its process parallelism, shared-library races, or 25-item batching findings. Work on `codex/steamcmd-batch` is outside this report's implementation scope; proposals should build on its batch executor.

The workspace initially contained two untracked reports: parallel downloads and page-load time/optimization; both retained. The user clarified that other tasks' changes in `.claude/worktrees/` are outside this task.

The authorized tag was initially written "I dont need this"; the database actually uses **`I Dont Need It`**, explicitly corrected and authorized by the user. Read-only SQLite confirmed 37 tagged Lineages. One Lineage can have workshop, steamcmd, and local copies, so this is not deletion permission for 37 separate directories. Real experiments used only the four client Workshop items below, leaving other subscriptions, game Mods, and other sources untouched.

| Mod | Workshop ID | Content bytes | Initial/final subscription and installation | Actions |
| --- | --- | ---: | --- | --- |
| [Disco!](https://steamcommunity.com/sharedfiles/filedetails/?id=2436747646) | 2436747646 | 44,352,603 | Both subscribed, installed | Recycle old directory, unsubscribe/resubscribe, download, pre-seed |
| [Dubs Break Mod](https://steamcommunity.com/sharedfiles/filedetails/?id=1722398508) | 1722398508 | 12,976,724 | Both subscribed, installed | Same |
| [Vanilla Recycling Expanded](https://steamcommunity.com/sharedfiles/filedetails/?id=3155781848) | 3155781848 | 6,143,491 | Both subscribed, installed | Same |
| [No Time Change Sounds](https://steamcommunity.com/sharedfiles/filedetails/?id=2572368517) | 2572368517 | 36,838 | Both subscribed, installed | Same; also public counts |
| Total | 4 items | **63,509,656** | Final Steam state **5** for all four | 1=Subscribed, 4=Installed |

### 2.1 Existing call paths

| File / symbol | Findings |
| --- | --- |
| [subscribeworkshop.go](../../internal/app/subscribeworkshop.go) `SubscribeWorkshop` | Validates/deduplicates IDs; defaults to `runSteam(Subscribe, ids)`; SteamCMD only on `ErrSteamUnavailable` or explicit `--steamcmd`. Per-item Steam failures do not switch downloader. Successful subscription returns **without waiting for content download** |
| Same file `downloadWorkshop` | One SteamCMD launch downloads multiple IDs; effective account, default anonymous; after download and About.xml validation, writes source record, registers SteamCMD source, scans/indexes. Does not call `SubscribeItem` |
| Same file `runWorkshopSteamCMD`; [accounts/lock.go](../../internal/accounts/lock.go) | Locks actual instance; nonanonymous uses SteamCMD's cache and console. Windows named mutex is isolated by actual exe path, not a client content-library write lock |
| [subscribe.go](../../internal/app/subscribe.go) `subscriptionRecord`, `UpdateSubscriptions`, `RemoveSubscriptions` | SteamCMD is a local managed source; updates remain SteamCMD incremental downloads. `Installed` is download time, not remote manifest. Local source removal is not account unsubscribe |
| [workshopsub.go](../../internal/app/workshopsub.go) `runSteam` | Starts a worker per **50** IDs: RimArcho implementation value, not Valve's subscription limit |
| [worker_windows.go](../../internal/steamworks/worker_windows.go) `Worker` | One `SubscribeItem` per item, collects async handles; no atomic "subscribe all" API. Waits 2–15 seconds, then 500 ms and pumps callbacks; per-item output; parent ordinary call timeout 40 seconds |
| [watch.go](../../internal/steamworks/watch.go) `watchDownloads`; [workshopwatch.go](../../internal/app/workshopwatch.go) | Existing `DownloadItem(id,true)` plus download observation reusable for resubscribe; 2-minute no-byte-progress and 6-hour total limits. Not part of default post-subscription waiting contract |
| [tasks.go](../../internal/app/tasks.go) `OpenTasks` | `tasks.json` retains history, but queued/running tasks reopen as stopped; manual retry/resume is not automatic cross-exit subscription recovery |

Show "subscription request time" separately from "Mod content availability"; do not compare the former to full SteamCMD download time.

## 3. Method and speed

### 3.1 Method, timing, authentication boundary

- Steam client uses its existing session and the game's `steam_api64.dll`; DLL, `steam_appid.txt`, external Python ctypes observer in a system temporary research directory. SDK calls only, no game launch or RimArcho scan/deployment, no real business DB writes. API tools return only allowed item results/states; no auth input collection.
- Anonymous SteamCMD initialized from Valve's official package in a separate temporary instance, without copying managed config, tokens, or device-trust files. exe SHA-256 `7baf77927639793e11b2a02ec2fd6d9114c95432be2f23a3225e274732255cf4`. Initial self-update/tool preparation excluded.
- Each anonymous round uses empty `force_install_dir`; four commands serial in one process. Runtime, depotcache, OS/CDN cache may be reused: **not a fully cold network/cache start**. Client depotcache not cleared; chunk reuse differs by downloader.
- Authenticated group only lets the managed instance try its own cache. No account configured in RimArcho; experiment uses client's existing nonsecret account name in memory only. stdin closed, password prompts prohibited; terminate on auth requirement, retain only phase result. No credential/token/ssfn reads; no password/Guard input or storage. Lock matches `internal/accounts` path normalization/Windows mutex; checked no other SteamCMD first, no concurrent account sessions.
- Before real unsubscribe, move the four old client directories to Windows recycle bin so Steam cannot directly delete originals; stop on recycle failure. Back up ACF and field snapshots per group. Never manually overwrite client ACF; pre-seed copies only into those four empty content directories.
- Monotonic clock. Total starts before subscription call or SteamCMD launch, ends when client content installed. Excludes setup, unsubscribe, recycling, subscription-list convergence. Active-client group calls `DownloadItem(true)` after subscription and watches `GetItemDownloadInfo`, flags, About.xml; accepts only Installed without NeedsUpdate/Downloading/Pending. Re-read final states and separately verify file-tree hashes.
- Early helper callback waits were shorter than Go worker and read stale subscription bits; later added callback waits before/after, always re-read final confirmation. Raw wall times below are **not exact RimArcho Go performance benchmarks**.

### 3.2 Measurements for the same Mods

| Method | Round 1 / seconds | Round 2 / seconds | Round 3 / seconds | Result/limits |
| --- | ---: | ---: | ---: | --- |
| SteamCMD anonymous, independent empty library, stop at download completion | 13.950 | 9.889 | 11.267 | 12/12 succeeded, median **11.267**; no subscription/client phase |
| SteamCMD cached account, independent empty library | Stopped after 1.413 | Not repeated | Not repeated | Auth required, 0/4 downloads started; **not account download time** |
| Subscription only, no active DownloadItem: first missing-content recovery | >120 | — | — | Four requests succeeded, directories not restored; excluded from throughput comparison |
| Same subscription-only conditions, manual download at 30 seconds | 49.808 | — | — | Manual wait 30.033, active download 19.157; illustrates stalled queue, not speedup baseline |
| Subscribe + immediate active client DownloadItem, wait for completion | **21.275** | **22.567** | — | Active segment 20.403 / 19.513; both 4/4, cold directories, old install records may remain |
| CMD independent download → subscribe all → active client completion | **32.065** | **29.615** | — | CMD 8.464 / 8.375; active client 22.763 / 18.165; extra content transfer both rounds |
| CMD download → copy four directories to client library → subscribe/client check | **41.486** | — | — | CMD 18.612, download+copy 19.001, check 19.373; client content transfer of 0, retained install records |

Active-client median total **21.921 seconds**; independent-library then subscribe median **30.840 seconds**, about **40.7%** longer to fully materialize content here. Rounds were not randomized; one machine/four Mods, not generalizable to other networks or large queues.

In independent-library round 1, both client record sections were confirmed absent after unsubscribe before temporary download; client then fetched content anyway. Direct control showing "independent library is not automatically reused". In round 2 and some baseline rounds, records did not vanish within 15 seconds; retained as observed, no forced ACF edits to manufacture a cold start.

### 3.3 Serial processing, limits, queues

Local active-client logs processed No Time Change Sounds, Recycling, Dubs Break, Disco in sequence; other items sampled as pending until the current item finished. SteamCMD commands likewise serial. Supports "this queue serializes Workshop items", **not that every Steam version/scenario is serial or uses one HTTP connection**.

Client messages included:

```text
Delaying next workshop updated 5 minutes (last update installed nothing)
AppID 294100 has no changes
... commit ... 0 updated, 0 moved, 0 deleted files
```

When directories were recycled but ACF still claimed installed, resubscription alone did not restore content in 120 seconds; same after 30 seconds in a second observation. Explicit DownloadItem restored it. These artificial missing-directory/convergence conditions are not ordinary new-subscription averages, but show Subscribed/Installed bits or ACF existence are insufficient completion evidence.

Valve [DownloadItem docs](https://partner.steamgames.com/doc/api/ISteamUGC#DownloadItem): `bHighPriority=true` pauses other ongoing Steam downloads to prioritize the item. An accelerator must not unconditionally elevate every item and interfere with games; offer an explicit action and reuse the observer.

No Steam config potentially containing auth state was read for bandwidth settings; no user settings changed, network isolation, or peak-bandwidth test. Cannot infer fixed client caps, unlimited SteamCMD, or internet throughput from 63.51 MB unpacked divided by wall time.

## 4. Subscription, duplicate downloads, ACF

### 4.1 Independent directories are not automatically adopted

Content plans for independent-library round 1, from GetItemDownloadInfo/client content_log; excludes handshake, manifest, metadata overhead.

| ID | CMD independent-library planned bytes | Client planned bytes after subscription |
| --- | ---: | ---: |
| 2436747646 | 28,902,592 | 28,343,568 |
| 1722398508 | 3,138,032 | 53,552 |
| 3155781848 | 3,509,056 | 3,508,320 |
| 2572368517 | 9,136 | 9,136 |
| Total | **35,558,816** | **31,914,576** |

Client transfer is not a byte-for-byte full redownload: Dubs logged about 12.86 MB reuse, and chunk selection may differ. Still, all four transferred content. Same-version copies in existing `D:/rimworld16` SteamCMD sources remained throughout and did not prevent client downloads.

### 4.2 ACF before/after

Manifest/size below are shared by initial state, independent CMD result, and completed client state, not versions inferred from download times.

| ID | manifest | timeupdated | size |
| --- | --- | --- | ---: |
| 2436747646 | 8150265425327708779 | 1753700863 | 44,352,603 |
| 1722398508 | 4817418585505826202 | 1791923023 | 12,976,724 |
| 3155781848 | 4359570922284147934 | 1791981472 | 6,143,491 |
| 2572368517 | 2566161780937107763 | 1754513851 | 36,838 |

| Phase | Client WorkshopItemsInstalled | Client WorkshopItemDetails | Client SizeOnDisk | Temporary CMD ACF |
| --- | --- | --- | ---: | --- |
| Initial | All four versions above | Versions, latest versions, timetouched, subscribedby, etc. | 16,353,807,970 | Absent |
| After three independent anonymous timings | Exactly initial | Exactly initial | Initial | Four installed each round; entire client ACF unchanged |
| Independent-library round 1 after unsubscribe convergence | All four absent | All four absent | 16,290,298,314 | About to create |
| After CMD success, subscription/client completion | Versions restored | Rebuilt by client; subscribedby etc. may fill asynchronously | 16,353,807,970 | Four installed, same client versions |
| Before/after pre-seed copy | Old four install records remain | Four details temporarily empty | 16,353,807,970 | Four installed |
| After pre-seed client check | Versions unchanged | Client fills account/latest fields | Initial | Independent copy retained |
| Final | All install fields match initial | Normal client time/download-state changes allowed | Initial | Recycled after experiment |

Nontarget portions: excluding four IDs from both dictionaries, sorted-JSON SHA-256 before/after `33378f8be466169692fe838c6f5d944b68bff9a282a8b62a769d329bbef6e0ce`; **1,289** entries per section, all identical. Top-level runtime fields change during normal API use; whole-file byte identity is inappropriate. No full old-backup restoration, which would roll back normal client state.

### 4.3 One copy without retransmission?

| Option | Feasibility | Evidence, risks, handling |
| --- | --- | --- |
| Independent CMD source + native client subscription | Feasible, accept two copies by default | Duplicate transfer measured; independent updates, simple, no shared-library write race |
| Pre-seed CMD content into client item directory, then client validation | **Conditional byte reuse** | Four local items showed `has no changes`, `0 updated,0 moved,0 deleted`, API content total of 0, but install records remained. Empty-record tests in §10 retransmitted/replaced files; no first-install zero-redownload guarantee. Check still 19.373 seconds, no speed guarantee |
| Point CMD `force_install_dir` directly at client Steam library | Not default online path | Two programs write AppWorkshop, staging/commit, content; instance lock excludes no Steam client. Prior research shows shared-library distortions. **Not run against the real library here**; pre-seeding is not same-library validation |
| Fully exit Steam, merge only target ACF/content, restart | Possible explicit repair, separate acceptance pending | Needs backups, strict parsing, target-ID-only changes, preservation of account/unknown fields, atomic publish, per-item recovery, client revalidation. ACF is not server subscription API; merge cannot replace SubscribeItem |
| Junction/hardlink client item to managed CMD source | Shared dual writers discouraged | Unsubscribe, replacement, validation, CMD update may damage targets or race; identity deduplication is not filesystem lifecycle safety |

[Valve GetItemInstallInfo](https://partner.steamgames.com/doc/api/ISteamUGC#GetItemInstallInfo) reads the actual install directory; it does not register arbitrary local paths. `BInitWorkshopForGameServer` targets game servers, not ordinary client imports.

Reference [RimModManager mgr_steam.py](../../../RimModManager/backend/managers/mgr_steam.py) `repair_workshop_items_via_steamcmd` explicitly requires Steam fully exited; `_apply_steamcmd_workshop_repair` backs up and merges specified items. Offline repair reference, not Valve's ACF/adoption guarantee; not called here.

## 5. Author credit: real subscriptions, statistics, experiment

### 5.1 Which actions count

Valve [ISteamUGC](https://partner.steamgames.com/doc/api/ISteamUGC#EItemStatistic) exposes NumSubscriptions, NumUniqueSubscriptions, UniqueWebsiteViews, Favorites, Playtime separately. [SubscribeItem](https://partner.steamgames.com/doc/api/ISteamUGC#SubscribeItem) creates a subscription; the [Workshop guide](https://partner.steamgames.com/doc/features/workshop/implementation) lists it for in-app subscription management, without excluding programmatic subscriptions from credit.

| Action | Current subscriptions | Unique/historical subscribers | Other author statistics |
| --- | --- | --- | --- |
| Web/client button or successful Steamworks SubscribeItem | Same kind of account subscription; at most one current relationship per account/item | Only first subscription contributes a unique user; repeated requests do not create users | Not automatically votes, favorites, web views, playtime |
| Same-account unsubscribe/resubscribe | Removes then restores relationship | **Not a new lifetime subscriber each time**; this previously subscribed account cannot test first-subscription increment | Do not inflate counts by cycling |
| Anonymous CMD `workshop_download_item` | **No subscription or subscription credit** | No named unique subscriber | No view/vote/favorite/playtime action; internal CDN/download telemetry invisible, so cannot claim all backend statistics are zero |
| Authenticated CMD `workshop_download_item` | **Download alone still creates no subscription credit** | Identity is not subscription | Same; auth group required login and never downloaded, so interface-semantic conclusion, not successful measurement |
| Official Web API SubscribePublishedFile | Real subscription on authorized success; counts untested here | Do not assume deduplication bypass | Publisher key required, not launcher login-free alternative |

[ISteamRemoteStorage Web API](https://partner.steamgames.com/doc/webapi/ISteamRemoteStorage#SubscribePublishedFile) `SubscribePublishedFile` requires a **Steamworks publisher authentication key**, secure-server-only, never in clients. Ordinary user API keys, Steam OpenID, or existing client sessions cannot replace publisher authority. RimArcho must not collect keys, browser cookies, tokens to bypass client dependency; keyless `GetPublishedFileDetails` is public read-only metadata only.

Official semantics and public count changes below support "same real subscription as buttons". **No author private dashboard access or verification of Valve aggregation/anti-abuse internals.** Interpret `lifetime_subscriptions` and SDK "unique subscriptions" as historical unique subscribers; one resubscription or cross-user aggregate changes cannot prove exact billing/recommendation weights.

### 5.2 Public page/API observations

Times PDT. Cookie-free ordinary GET, no random cache-busting parameters. Current Subscribers is parsed from page; Unique Visitors means visitors, **not unique subscribers**.

| No Time Change Sounds: time/phase | Page Current Subscribers | API subscriptions | API lifetime_subscriptions | Observation |
| --- | ---: | ---: | ---: | --- |
| 20:03:25, before unsubscribe | **58,276** | 58,276 | 136,616 | Existing subscription |
| 20:03:38, four unsubscribes succeeded | **58,275** | 58,276 | 136,616 | Page −1; API stale, refresh/cache difference |
| 20:05:39, resubscribed | HTTP 429 | 58,276 | 136,616 | Call succeeded, page recovery unobserved; stopped continuous polling |
| 20:09:15, independent-library unsubscribe | Not requested | 58,275 | 136,616 | Global aggregate, not attributable to this account alone |
| 20:09:24, anonymous download done, before subscribe | Not requested | **58,275** | **136,616** | Download added no visible subscriptions; server subscription still pending |
| 20:09:48, after subscription | Not requested | 58,275 | 136,616 | Success need not instantly refresh aggregate |
| 20:15:19, final after cooldown | HTTP 429 | 58,274 | 136,618 | Final account state=5; other-user changes confound totals; no further web retry |

Other three API readings for review; current / lifetime:

| Phase | Disco! | Dubs Break Mod | Vanilla Recycling Expanded |
| --- | --- | --- | --- |
| 20:03:25 before unsubscribe | 40,577 / 112,364 | 240,995 / 569,374 | 272,141 / 438,885 |
| 20:03:38 after unsubscribe | 40,577 / 112,364 | 240,995 / 569,374 | 272,141 / 438,885 |
| 20:05:39 after resubscribe | 40,577 / 112,364 | 240,997 / 569,375 | 272,141 / 438,885 |
| 20:09:15 after unsubscribe | 40,577 / 112,364 | 240,996 / 569,376 | 272,142 / 438,885 |
| 20:09:24 CMD done, unsubscribed | 40,577 / 112,364 | 240,996 / 569,376 | 272,142 / 438,885 |
| 20:15:19 final | 40,579 / 112,368 | 240,991 / 569,378 | 272,144 / 438,887 |

`Cache-Control: no-cache` appeared, but does not guarantee real-time backend aggregation/internal caches; changing API Date does not prove fresh data. Other users were not isolated; no public-count refresh SLA. Strongest empirical conclusion: **Steamworks subscriptions affect page current subscribers; download does not replace subscription; unchanged public counts do not establish failure.** No clean resubscribe +1 page or author lifetime evidence; later review needs no 429, low background traffic, first-subscription account. Do not fabricate missing evidence.

### 5.3 Completing every subscription when credit comes first

Absolute guarantees depend on Steam, account state, and reopening RimArcho. Guarantee **persist intent first, never forget unconfirmed work, resume after recovery, never silently report all complete**:

1. Before any download, atomically persist all deduplicated IDs with AppID, nonsecret target-account identity, creation time, per-item status, retry time, error class. Independent of scanned Instances or `source.json`; failed/no-directory downloads still need records.
2. Separate download and subscription states. If subscribing "last", process **the entire original selection after all attempts finish (success/failure/cancel)**, not `okIDs`. Hung downloads need cancel/timeout; persist intent on exit, never rely only on in-process defer cleanup.
3. If client absent, show "waiting for Steam; author subscription incomplete"; no false success, anonymous substitute, or web-session scraping. Retry at startup/foreground/Steam recovery; low-frequency probes while running, not worker-per-second. Without a resident background process, no progress while app closed.
4. Confirm target account through local session before submission. On a different account, await user confirmation rather than reassign old obligations. Atomically mark confirmed only after success and refreshed subscription bit; callback/network ambiguity stays unknown, query before resending. **Never Unsubscribe before retrying.**
5. Crash after remote success/before local confirmation: query or idempotently repeat SubscribeItem. At-least-once attempts, no cross-service exactly-once promise. Do not redownload successful items; separate business failure from public-stat cache.
6. Backoff/jitter transient errors; persist `next_attempt_at`, e.g. exponential 5 seconds to 30 minutes. After per-round attempt limit, defer/show pending rather than delete obligations or tight-loop. Permission/deletion/ban failures remain "user action needed" with Workshop page; no blind retry for nonnetwork errors.
7. Remove pending intent only on explicit cancellation; download cancellation does not cancel credit by default. Local deletion/recycling never auto-unsubscribes. Task-history cleanup must retain unconfirmed semantic work.

**Batches are not transactions; no dependable official fixed rate.** Existing 50/worker can cap requests; start with smaller windows, waits, adaptive results. Policy values only. Valve EResult 84=temporary RateLimitExceeded, 25=LimitExceeded, 24=InsufficientPrivilege, 26=Revoked. Web HTTP 429 differs from SubscribeItem EResult 84; only web 429 measured, no account-limit stress test. CMD 25-item batches are not subscription API limits.

Also found [workshopresult.go](../../internal/app/workshopresult.go) `WorkshopResultText` labels 24/25 disagree with official EResult; correct classification during implementation to avoid treating permission as rate limiting and limits as revocation. Retain raw numbers for diagnosis; no code fix here.

## 6. Which copy RimWorld reads and how to manage two

RimWorld reads game `Data/`, `Mods/`; Steam builds discover Workshop item directories through Steam, not arbitrary RimArcho CMD sources. Local client root `D:/SteamLibrary/steamapps/workshop/content/294100`; also registered `D:/rimworld16/steamapps/workshop/content/294100` and managed CMD sources.

[deploy.go](../../internal/app/deploy.go) `deployable`, `DeployName`, `planLinks` expose selected CMD Instances through `Mods/_Archo_<source-tag>_<directory-name>`. Plans reference Instances; client and CMD copies of one Workshop ID are **two Instances, one Lineage**. Lineage shares tags/rules, not bytes, and never switches plans to "last downloaded" automatically.

[loadout.go](../../internal/loadout/loadout.go) `NewRegistry` registers Data, Mods, workshop in order. When local/linked content occupies packageId, Workshop may become `<packageId>_steam`. No newest-by-time choice or automatic synchronization. New Workshop candidates after subscription affect token allocation; use registry to write ModsConfig, never handwritten packageId or silent instance swaps.

| Mode | Authoritative plan copy | Updater | Deduplication/recycling |
| --- | --- | --- | --- |
| Client default | Client workshop Instance | Steam client | No new CMD copy needed |
| Optional permanent CMD acceleration | Player-selected linked CMD Instance | RimArcho starts CMD updates; client independently updates its own copy | Accept two copies, no dual-write links; report versions, no auto-sync |
| Temporary CMD acceleration then client handoff | Temporary CMD first, explicit switch to workshop Instance | Client after handoff | Wait for client verification/game exit, handle plan references/inbound links, recycle unused CMD; retain copies referenced by any plan/environment |
| Explicit offline repair/pre-seed | Client workshop Instance | Steam only after publication | Separate publication/rollback contract; no dual source ownership for one real directory |

Thus "download then subscribe" preserves credit but does not naturally yield one copy/updater. Recommend accepting independent copies in the first version rather than silently adding ACF merge and identity/link migration to acceleration.

## 7. Consistency, fallback, actual player waits

| State | Presentation and continuation |
| --- | --- |
| CMD succeeds, subscription fails/unknown | "Content available; author subscription pending"; retain source/download and separate pending subscription; retry only subscription, no redownload/rollback |
| CMD fails, subscription succeeds | Keep real subscription; client may download; offer failed IDs priority client download/page/error. **Subscribe failed downloads too** |
| Both fail | Retain separate errors; downloader changes do not fix permission. Existing files are not proof of success |
| Public count stale, subscription confirmed | Use account API state; explain delayed public statistics; no unsubscribe/resubscribe verification loops |
| Anonymous denied/invisible/age/ownership restriction | Preserve all intent; use authorized client session. Configured managed CMD account may serially retry failed items via cache; pause for native authentication if required, user retries afterward |
| Private/friends-only/delisted/banned/missing license | Login does not guarantee permission; no visibility/age/ownership bypass. Explain unavailable, open original page; no endless retry of permanent errors |
| App exit/crash | Persist intent; resume under original account next launch. Stopped GUI history must not become permanent omission |

Strict "subscribe last" waiting model:

- Wait for both: `T_CMD + T_subscription + T_client-check/retransfer + publication-overhead`; usually adds stages, as locally measured; no overall speed claim.
- Release when CMD is playable: `T_CMD + T_scan/link + T_subscription-confirmation`; client sync off foreground path. CMD segment shorter here, but actual launch untested; needs integrated acceptance.
- Fast download with indefinitely pending subscription: credit incomplete; retain prominent state, never speed through missed subscriptions.
- Already-fast/current client, saturated network/disk: CMD-first may only add startup/transfer. Keep default and client fallback; do not force all content through CMD.

For credit, **real subscription first, client download remains the safest default**. Optional acceleration can use independent CMD sources, with obligations created when selection is confirmed. If network action must remain last, use §5.3 persistence/failure cleanup; "last" must not depend on download success.

## 8. Recommended changes, specs, acceptance

Future implementation proposals, **not implemented here**. For credit + one copy, default: "persist obligations → subscribe every selection → asynchronous client download/validation → usable next launch". No CMD stage needed for this goal; pre-seeding is a fallible content hint, never correctness prerequisite.

| Location | Recommendation |
| --- | --- |
| `internal/app/subscribeworkshop.go` `SubscribeWorkshop` | Persist all IDs before network calls; report subscription confirmation separately from client readiness. Real subscription by default, async preparation, retain failed obligations |
| Same file `downloadWorkshop`, `runWorkshopSteamCMD`; `internal/app/subscribe.go` | Keep independent sources for non-Steam environments. Explicit CMD-to-client import reuses other branch's 25-item executor, downloads to independent same-volume staging outside client library; never client `force_install_dir`, no second permanent source |
| App persistent subscription module; `internal/store` | Unique nonsecret account + AppID + WorkshopID; atomic pending/unknown/confirmed/needs-action, retry/error. Confirm account subscription for completion. Failed/canceled downloads retain intent until explicit cancellation |
| `internal/app/workshopsub.go`, `internal/steamworks/worker_windows.go` | Reuse per-item SubscribeItem and commit partial success; query before retry after timeout. 50/batch is implementation, no official fixed QPS; bounded windows, rate/network backoff/jitter, no atomic-all claim |
| `internal/app/workshopwatch.go`, `internal/steamworks/watch.go` | Background observation; explicit `DownloadItem(false)` if needed, no default preemption. Await target AppID/ID completion callback and verify state/directory/version; Installed/About alone insufficient. Existing completion also needs NeedsUpdate exclusion, callbacks, errors |
| `internal/app/tasks.go`, startup/task entry | Restore persistent obligations independently of stopped history; pending without Steam, resume with original session. Exit pauses work, no hidden resident process |
| `internal/app/deploy.go`, `linklifecycle.go`, `internal/loadout`, launch | New unready workshop Instances cannot enter launchable plans; do not swap running plans' source/links. Prepare other content in background but do not request updates to targets used by current game; app locks cannot protect against external Steam updates |
| `internal/accounts`, result text | Keep instance locks/cache/native auth. Not a Steam-client-library lock. Anonymous failure may use authorized client or serial cached CMD; pause on auth requirement without reading auth files. Retain EResult copy fix proposal |

If CMD pre-seeding is implemented: client root is never CMD target. CMD's AppWorkshop ACF serves staging only; read manifest/size for verification, **never copy/merge into client ACF**. Prepare complete directories only when new items are unsubscribed, client directory/both record sections empty, game not using them. Only a fully exited client provides an enforceable single-writer handoff. Same-volume atomic rename, restart client, subscribe all, let client rebuild/redownload. While client runs, flags/ACF checks are not exclusive locks; account lock cannot stop client writes. Fall back to direct subscription/client installation without overwriting directories.

After adoption, workshop Instance is authoritative, client alone updates; no CMD `source.json` or incremental CMD updates inside client directory. Existing permanent CMD sources remain separate Instances; shared Lineage is not deletion permission. Recycle old CMD only after game exit, all plan/environment inbound references handled, client confirmed complete. Unhandled shared references prevent a machine-wide single-copy claim. "One copy" means final active source; recycle bin and download/commit may still use space.

Spec changes for review:

- **02 §3.4, 08 §10/§11**: separate subscription/readiness, all-ID credit, account binding, subscribe failures, cross-exit recovery. Optional pre-seed falls back to client, no no-redownload promise.
- **01 §4 write list, 07 storage/lifecycle**: durable pending work, independent staging, same-volume publish, client-exit boundary, recycling/crash recovery; default needs no ACF write permission.
- **06 inventory/subscriptions/tasks, 05 CLI/MCP**: distinguish "subscribed", "waiting for Steam", "downloading/validating", "launch-ready"; nonblocking background tasks with explicit waits for new unavailable Mods.
- **04 plans, 08 §3 links/launch**: no automatic source switch by Workshop ID; scan/deploy only fully ready content, no running directory/link swaps. Explain conflict between strict single copy and immediate reads during client processing.

Acceptance: all failed downloads still subscribe every ID; exit/crash across confirmation window; partial batches/rate backoff; Steam absent, original-account recovery/account switching; temporarily unreadable ACF; empty-ACF pre-seed retransmission/replacement; launch readiness/no running-plan source swaps; staging recycling after completion; refuse auto-recycling CMD still referenced by plans. Failure never unsubscribes, restores whole ACF, or deletes used sources.

## 9. Review method, evidence chain, final state

### 9.1 Actual steps and retest boundaries

Sequence: read-only source/DB/initial state → three anonymous rounds → managed-cache attempt stopped at auth → missing-content subscription-only observation → active client → independent CMD then subscribe → pre-seed → second active-client/independent rounds → pump callbacks and verify subscription/files/ACF → recycle temporary directories.

Public metadata retest, without cookies/passwords/keys; counts change over time, avoid tight polling, stop at 429:

```powershell
$researchIDs = @('2436747646','1722398508','3155781848','2572368517')
$researchBody = @{itemcount=$researchIDs.Count}
for ($i=0; $i -lt $researchIDs.Count; $i++) { $researchBody["publishedfileids[$i]"]=$researchIDs[$i] }
(Invoke-RestMethod -Uri 'https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/' -Method Post -Body $researchBody).response.publishedfiledetails |
    Select-Object publishedfileid,result,subscriptions,lifetime_subscriptions
```

Local queries used Python stdlib `sqlite3.connect('file:<absolute-database-path>?mode=ro', uri=True)`. Authorized-set SQL below. Retests must recheck tags, real paths, game exit, user permission; historical lists are not permanent authorization:

```sql
SELECT i.id, i.rel_path, i.real_path, i.status, s.kind, s.root, n.name
FROM instance i
JOIN lineage_tag t ON t.lineage_id = i.lineage_id
JOIN source s ON s.id = i.source_id
LEFT JOIN snapshot n ON n.id = i.current_snapshot_id
WHERE t.tag = 'I Dont Need It' AND s.kind = 'workshop';
```

Real retests may build current `cmd/archo` into a new system temporary directory with game DLL copy and `steam_appid.txt` containing only `294100`, using existing `steam-worker state|unsubscribe|subscribe|watch <ID...>` protocol. `watch` actively requests downloads, **not side-effect-free observation**. Each API worker exits independently; wait for convergence. Client group recycles targets, unsubscribes, awaits target ACF/directory changes, then times subscription through watch success; independent group runs same CMD batch before subscribing. Check per-item results, actual About.xml, flags, file trees, not only process exit.

CMD command shape: `steamcmd.exe +@NoPromptForPassword 1 +force_install_dir <new-temporary-base-for-this-round> +login anonymous +workshop_download_item 294100 2436747646 +workshop_download_item 294100 1722398508 +workshop_download_item 294100 3155781848 +workshop_download_item 294100 2572368517 +quit`; placeholder must be a newly created directory each round, never a real Steam library. Account attempts must reuse cache in the managed instance's original directory, never copy auth directories. Without valid cache, stop at auth; do not turn public experiment scripts into password collectors.

Pre-seed additionally backs up client ACF before copying; copy only verified four-ID content into corresponding empty directories, no ACF merge or `.rimarcho/source.json`. Installation records remained in the measured pre-seed group: record this condition on reproduction. Recycle old and experimental directories only, no `Remove-Item -Recurse` or hard deletion.

### 9.2 Evidence → Finding → Path

| Evidence | Reviewable source / observation | Finding | Path |
| --- | --- | --- | --- |
| E1: code baseline | §2.1 files/symbols; `rg -n 'SubscribeWorkshop|runWorkshopSteamCMD|runSteam|OpenTasks' internal/app` | Default success means subscription only; CMD source differs from subscription; tasks do not auto-resume credit | P1: persist all intents, separate states |
| E2: three anonymous/two dual-stage rounds | §3.2 raw times, §4.1 bytes, About/flags/hashes | CMD faster locally, but independent CMD+subscription duplicates transfer and takes longer | P2: client default, opt-in CMD, explicit foreground completion criterion |
| E3: client ACF/log/pre-seed | §3.3/§4.2; `has no changes`, Installed with missing directory, zero seeded transfer | Manifest is not disk completion proof; reuse still incurs checking | P3: active download/content validation, no online ACF dual writes |
| E4: official UGC/stats/Web API, public counts | §5 URLs/times/values/HTTP | Real SubscribeItem, download not credit, delayed/confounded aggregates, ordinary key not Web subscription alternative | P4: subscribe all selections, confirm account state, page supplementary only |
| E5: cache attempt | Auth needed at 1.413 seconds, no download, no secret collection | Cache unusable here, account speed unmeasured | P5: user authenticates in native SteamCMD then retest, no bypass |
| E6: final state | §9.3 state=5; target records/nontarget ACF identical, trees restored | No missing subscriptions/content left | P6: retain recovery evidence, recycle artifacts, no business changes |

Call path: select IDs → **durable intent (E1/E4)** → optional CMD download/verify (E2/E5) → SubscribeItem for entire set (E4) → per-item confirmation/persistent failure wait → selected Instance through registry/links (§6) → independent client sync or explicit handoff (E3). Download failure never cuts off credit.

### 9.3 Final restoration and limits

All four final `GetItemState=5`, About.xml present, manifest/timeupdated/size match initial. Client directory counts/bytes/full tree hashes restored: Disco 126 files, Dubs 29, Recycling 191, No Time 12. Dubs original contained one extra zero-byte file; restored, no experimental difference retained. Tree algorithm: sort relative POSIX paths, accumulate `UTF8(path) + NUL + SHA256(file bytes)`, then SHA-256; exclude `.rimarcho`.

No changes to existing CMD sources, plans, deployment links, game files, other subscriptions. Both nontarget ACF sections' 1,289 values identical. SteamCMD handled auth; no secrets read/printed/saved, no multiple authenticated CMD sessions. Dedicated instance, scripts, copies, ACF backups, sanitized observations all recycled after verification; original directories also remain recoverable from recycle bin.

Unverified with reasons: authenticated group required login; web 429 prevented clean resubscribe +1; first-account lifetime increment, offline ACF merge, direct shared-library download, long-term limits/failure rates, larger Mods, actual game launch untested. Truly empty install records and client-closed pre-seeding tested in §10, updating initial limits. Others do not invalidate "independent libraries retransmit, full process did not accelerate, credit needs separate durable guarantees".

## 10. Empty-client pre-seeding: one copy and runtime reads

### 10.1 Conditions and steps

Fills §4.3's key gap: **first-install local conditions require absent target directories and IDs absent from both ACF sections**, not merely unsubscribe or empty details. Both rounds use same existing client account, only authorized `I Dont Need It` set. Original four plus Ruthless Faction Pursuit and BioReactor: six, 75,396,063 bytes, 486 official package files. Largest authorized item Disco! (44.35 MB); no out-of-tag Mods added for size.

1. Each round rechecks tags, client library, game exit, no other CMD sessions read-only. Back up whole ACF, target/nontarget dictionaries, original trees. Recycle old experiment directories, then six `UnsubscribeItem` calls, all successful callbacks.
2. No ACF edits. Wait until six directories and both sections empty, save `empty.acf`: **308.266 seconds** round 1, **307.829** round 2, both logging five-minute checks. Normal client exit/restart in round 2 did not instantly clear installed entries; still await convergence. Restart cannot replace condition checks.
3. Only then independent anonymous CMD downloads latest six to temporary `force_install_dir`: **12.112 / 11.045 seconds**, both 6/6, no auth prompts. No client target/config/auth copying; depotcache/OS/CDN retained, not fully cold.
4. Copy three larger items C → D client directories; copy other three first to independent D staging, then same-volume rename. Verify all file hashes; no CMD ACF or `source.json`. Both client sections remain empty before/after publication; directories alone do not create install records.
5. Round 1 seeds while client running then subscribes. Round 2 seeds with client fully exited, restarts original session (observer accessible after about **5.655 seconds**), then subscribes. Every callback EResult=1. Observer SDK makes Steam consider app running, sometimes `Suspended`; at about 30 seconds explicitly `DownloadItem(false)`, all accepted, no global pause/high-priority preemption.
6. Round 2 observes Windows directory events, about 30 ms target metadata polling, about 0.5-second full hashes, and one persistent read-only handle per item allowing READ/WRITE/DELETE sharing. Handles do not block replacement. Cross-check UGC progress, completion callbacks, ACF, Workshop/content logs, staging, NIC totals; independent short-lived SDK final confirmation.

Round 1 observer hit ACF sharing denial at about 35.237 seconds and stopped; later evidence only client logs/final files, **not full read-safety validation**. Round 2 retries short ACF locks, covering all six plus about three seconds after completion. Event observer waited to exit and wrote final results only at **221.789 seconds**: tool cleanup, not validation duration. Wakeup sets experimental About.xml to its original timestamp, leaving content/mtime/file ID unchanged, outside valid observation window.

### 10.2 Download, publication, completion

Client bytes below are round 2 maximum GetItemDownloadInfo got/total, equal finally and matching per-item `content_log` plans; excludes manifest/handshake overhead. Completion is cumulative from explicit low-priority requests, items serial, not per-item pure CPU validation.

| Mod / ID | Content bytes | Publication | Round 1/2 milliseconds | Client retransfer bytes | Round 2 callback / seconds | File IDs changed |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| Disco! / 2436747646 | 44,352,603 | Cross-drive copy | 61.498 / 172.460 | 28,343,568 | 6.097 | 126/126 |
| Dubs Break Mod / 1722398508 | 12,976,724 | Cross-drive copy | 23.352 / 18.769 | 53,552 | 10.972 | 28/28 |
| Ruthless Faction Pursuit / 3621784437 | 9,867,879 | Cross-drive copy | 27.020 / 23.108 | 1,820,160 | 15.639 | 28/28 |
| Vanilla Recycling Expanded / 3155781848 | 6,143,491 | Same-drive rename | 0.873 / 0.727 | 3,508,320 | 20.751 | 191/191 |
| BioReactor (Continued) / 3307031939 | 2,018,528 | Same-drive rename | 0.711 / 0.608 | 1,322,480 | 25.455 | 101/101 |
| No Time Change Sounds / 2572368517 | 36,838 | Same-drive rename | 0.426 / 0.359 | 9,136 | 30.004 | 12/12 |
| Total | **75,396,063** | 3 copies + 3 renames | — | **35,057,216** | **30.004** | **486/486** |

Publication milliseconds cover only copy/rename calls, excluding staging copies, hashing, SDK startup. Shows rename can publish, not full performance comparison; neither prevented retransfer. Round 1 first three logged 28,343,568, 53,552, 1,820,160 bytes; last three combined 4,839,936, same **35,057,216** total.

Round 2 all callbacks EResult=1; last at **60.036 seconds** from observation start; all state=5 and ACF complete at **60.174**. Each showed `9 → 41 → 57 → 5`: subscribed/NeedsUpdate → queued → downloading → subscribed/installed. Seeded About present while state 9 does not establish playability. Includes startup/wait/explicit requests, not natural post-subscription queue timing or speed comparison.

**Actual retransfer, not just validation:** `workshop/downloads/294100/<ID>` appeared while downloading and vanished after commit; two other observed `downloading` paths had no targets. `content_log` logged nonzero download, staging, committing. Some chunks reused, e.g. Disco 2,219,008, Dubs 12,857,986 bytes; still not zero retransfer. Two depotcache locations under client program directory/D library had unchanged filenames/sizes/mtime; unchanged cache does not mean no download.

Wi-Fi totals from before round 2 through observer exit increased **44,777,246 received**, **6,421,776 sent bytes**. Includes Steam startup/background/tool cleanup; **not Mod-specific traffic or validation speed**. Evidence for retransfer is per-item API/content-log **35,057,216** bytes, not NIC totals.

### 10.3 ACF and content versions

| Snapshot | WorkshopItemsInstalled | WorkshopItemDetails | SizeOnDisk / bytes |
| --- | --- | --- | ---: |
| Round 1 before.acf | All six present | All six present | 16,353,807,970 |
| empty.acf | All six absent | All six absent | 16,278,411,907 |
| published.acf (both rounds) | Six still absent | Six still absent | 16,278,411,907 |
| after-preseed.acf / final.acf | Six created by client | Six created by client | 16,353,807,970 |

Final installation fields below match anonymous CMD ACF and initial versions. Client creates manifest/timeupdated, latest_manifest/latest_timeupdated, timetouched, subscriber fields. Temporary time/download fields may be cleaned later; final round 2 details omit BytesDownloaded/BytesToDownload, which cannot imply no download.

| ID | manifest | timeupdated | size |
| --- | --- | --- | ---: |
| 2436747646 | 8150265425327708779 | 1753700863 | 44,352,603 |
| 1722398508 | 4817418585505826202 | 1791923023 | 12,976,724 |
| 3621784437 | 8140011528139760774 | 1768734973 | 9,867,879 |
| 3155781848 | 4359570922284147934 | 1791981472 | 6,143,491 |
| 3307031939 | 6497344820961569741 | 1786218403 | 2,018,528 |
| 2572368517 | 2566161780937107763 | 1754513851 | 36,838 |

Whole-file SHA-256 for original, round 1 empty, final ACF: `51949dfe73b93e3426f9ed698c0091ce84d9bd0c3a2c4065a10167609e95123d`, `6fde8d280314a067474a3a1deba13bbc3682783f404c7b608b761892ec4b9a33`, `17aaa9b26b16363d6fcb8fba05c1043611a5da0e1e4877c997f790012482a650`. Header/target details may change, so equality not required. Excluding six IDs, **1,287** entries per section, initial/final hash `5aaf89a8ffe5cd49ce407de315868f3a91562beb258e98da06d785b84dd2fae3`, values identical. No full ACF restoration; snapshots saved before related content changes, no manual ACF edits throughout.

Final six official file-tree hashes equal CMD seeds, **while all 486 NTFS file IDs changed**: direct evidence of client replacement with identical bytes. Equal hashes do not prove read-only validation. Tree algorithm §9.3, excluding `.rimarcho`; restore Dubs extra empty file at final check, separately from official-package comparison.

### 10.4 Playing during client replacement?

Round 2 covered six items with **773 metadata rounds**, **119 full-tree hash attempts**, **714 persistent-handle reads**; maximum metadata gap **0.331 seconds**, hash gap **0.576**. No event overflow/errors. No root-directory delete/rename events, but files inside were not stable.

Events show per-file commit: create `0Harmony.dll~RF….TMP`, rename old `0Harmony.dll` temporarily, add new original-path file, remove temporary backup. All 486 original files have old/new name event pairs. Example client log:

```text
21:24:01 update started : download 0/28343568, reuse 0/2219008, stage 0/44352603
21:24:02 starting commit ... workshop/downloads/294100 ... workshop/content/294100 : 148 updated, 0 moved, 0 deleted files
```

"0 deleted" does not mean old files never disappear: commit replaces files individually, not an atomic whole-Mod snapshot. Different versions may mix old/new content; this same-version test proves replacement, not specific cross-version failures. Metadata repeatedly recorded changed IDs; one Disco tree-hash traversal hit **PermissionError at 35.646 seconds**. No exact failed path retained; could involve original or transient file. Does not prove game read failure or crash. Difference details cap at 1,000, mostly repeated ID changes; count is not independent corruption count, nor does truncation prove every persistent-handle read succeeded.

[Valve DownloadItem docs](https://partner.steamgames.com/doc/api/ISteamUGC#DownloadItem) prohibit disk access before DownloadItemResult_t after successful request, also with NeedsUpdate. Existing delete-sharing handles may still point to old files, without guaranteeing later path opens. No RimWorld run or real DLL/resource-loader lock test. **Evidence does not support guaranteed launch/use of the same Mod while the client processes it.**

Background work can avoid blocking GUI or stopping a current game to prepare unused new Mods; current game should keep stable existing content, new Mods wait for next launch. Do not actively request replacement of an item in use; Steam updates are outside RimArcho instance locks. Immediate CMD use during commit needs a separate stable copy or deferred handoff until exit; cannot promise one copy at all times, immediate adoption, and safe reads together.

### 10.5 Feasibility, fallback, boundaries

| Option / condition | Additional finding | Product handling |
| --- | --- | --- |
| Both sections truly empty, complete latest content, then subscribe | Both rounds retransfer/replace, including client-closed seed; no zero-redownload guarantee | Fall back to normal client install, retain subscription, launch only after completion |
| Same-version seed with install records | Initial §4.3 zero transfer; different conditions | Optimization opportunity, not correctness/safety contract |
| Final one active source | Client adoption feasible | Resolve references before recycling staging/old copies; transfer/recycle bin still uses space |
| Immediate play while same content processed | No guarantee, per-file replacement observed | Block launch path; await readiness or accept second stable copy |
| CMD shares running client's library/ACF | Untested, dual-write risk remains | Reject; account lock cannot coordinate client |

If explicit pre-seeding remains, §8 details it: CMD always downloads to independent same-volume staging outside client library, no ACF import; rename only with client fully exited and target without records/references/active tasks. Restart, subscribe all, let client check/redownload; scan as workshop only after confirmation, client-only future updates. **Simpler recommendation remains direct real subscription and background waiting, omitting CMD/handoff.**

Obligations do not depend on publication: persist every selection, finalize subscriptions after success/failure/canceled downloads; keep original-account pending work through missing client, exit, rate limits, resume next launch; confirm successes only, backoff rest. Anonymous failure does not cancel credit: authorized client first, cached CMD serial retry if available, native player authentication when needed. No failure auto-unsubscribes, overwrites ACF, or rolls back others.

Untested: subscribe-then-immediately-seed race, old-manifest content. No existing verifiable old version, so no fabricated manifests or damaged packages presented as old versions. Redownload of complete same-version content already refutes unconditional no-redownload. No extra authenticated downloads, public stats, or larger unauthorized Mods; credit evidence/auth boundaries remain §5.

### 10.6 Evidence and restoration

| Evidence | Finding | Path |
| --- | --- | --- |
| E7: two empty/published ACF sets, directories, anonymous versions/hashes | Empty-record conditions satisfied; seeds do not self-register | Client-only ACF writes, no zero-redownload guarantee |
| E8: six nonzero progress values, transfer/commit logs, callbacks | Client successfully retransferred 35,057,216 bytes, not an import error | Accept normal install fallback, await readiness asynchronously |
| E9: complete events, IDs, hash-read exception | Identical final hashes despite per-file replacement; directory not read-safety boundary | Processing target is not guaranteed playable |
| E10: independent final state, original hashes, nontarget ACF | All six state=5, target fields original, nontarget dictionaries identical | Retain subscriptions, recycle temporary work, no whole-ACF rollback |

Final Mods: **Disco!, Dubs Break Mod, Ruthless Faction Pursuit, Vanilla Recycling Expanded, BioReactor (Continued), No Time Change Sounds**, all **subscribed, installed, About readable, state=5**, trees restored to pre-round state. Original Dubs zero-byte `.git/gk/config` restored. No plan/deployment/real Mods/existing CMD-source changes. Other sources already contained copies and were retained by scope; no machine-wide deduplication claim.

Six old directories, anonymous instance, downloads, D staging, monitoring scripts, ACF backups, raw records all moved to Windows recycle bin, no hard deletion. Report retains key logs/counts/versions/check summaries; raw snapshots recoverable with recycled directories. Only this report updated in repository; other tasks' untracked reports retained, no commit.
