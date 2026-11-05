# SteamCMD parallel-download research

> Research date: 2026-10-26, measurements about 19:13–19:26 (PDT, UTC−07:00). Source/public-reference review and local anonymous-download experiments; recommendations below not implemented. No business-code changes, commits, real accounts, Steam subscription API calls, or writes to real game/Mods directories.

## 1. Conclusions and decisions

**Parallelism is conditionally feasible: anonymous downloads use independent SteamCMD runtimes, Workshop libraries, and caches, one process per instance; publish completed content serially to the final source. Do not simply add goroutines or remove existing instance locks.**

Three rounds with four small public Mods: four fully isolated processes took median **10.006 seconds**, single-process batching **15.610 seconds** (about 1.56×, 35.9% less time), both 12/12 successful. Two processes took 18.374 seconds, slower than batching. Supports optional anonymous parallelism, not guaranteed gains for every network/Mod count.

**Prioritize batch updates.** New Workshop subscriptions already download multiple entries per launch; updates launch/scan per entry. Per-item median 28.169 seconds; single-process batching cuts download-stage time about 44.6%, simpler than multiple runtimes. Single-process `+runscript` median 14.163 seconds is comparable to batch command lines; it addresses long commands/batch organization, not item parallelism.

| Method | Verdict | Basis and conditions |
| --- | --- | --- |
| One process, repeated `+workshop_download_item` or multiple script commands | Works, entries serial | Logs request next only after previous success; individual downloads may use multiple HTTP connections |
| Multiple processes, shared installation/library | **Do not adopt** | Three rounds: 5/12 successes; final ACF installed records 1, 0, 0 |
| Multiple processes, shared installation, separate `+force_install_dir` only | Insufficient as supported design | 12/12 succeeded, but config/depotcache/logs/self-update files shared; no concurrent-write guarantee |
| Separate installations, shared `force_install_dir` | Do not adopt | Runtime isolated, ACF/staging/commit directories still shared; library contention remains; not separately tested |
| Fully isolated installations + separate libraries + anonymous | **Conditionally feasible** | Local 2/4 concurrency succeeded; no shared writable caches, directory links, final publication |
| Multiple SteamCMD sessions for one real account | No parallelism in first version | Session replacement/elsewhere-login failures exist; runtime copies do not solve server conflicts; login untested under task limits |

## 2. Current implementation: batch additions, per-item updates

Code snapshot: RimArcho `dev/0.9.0`, HEAD `81017d85f73f504ed59d4701c94ccd907363d8f6`. Read [workspace rules](../../../AGENTS.md), [architecture](../00a-architecture.md), [session research](2026-10-24_login-sessions-loverslab-steamcmd.md), [subscription spec §3.4](../specs/11-downloads-and-accounts.md#1-subscriptions-git-download-pages-steamcmd). Earlier recommendations do not replace source: account actions/locks already exist.

### 2.1 Entry points, processes, result parsing

| File / symbol | Actual behavior | Acceleration considerations |
| --- | --- | --- |
| [subscribeworkshop.go](../../internal/app/subscribeworkshop.go) `workshopIDs`, `SubscribeWorkshop` | Validate/deduplicate IDs; default Steam client; SteamCMD fallback only for `ErrSteamUnavailable`, or explicit `--steamcmd`. Item failure does not arbitrarily reroute | Preserve routing; account permissions must not trigger parallelism |
| Same file `downloadWorkshop` | Builds `+force_install_dir <base> +login <account> +workshop_download_item 294100 <ID> … +quit`; all IDs in one process, no batch cap/`runscript` | Additions amortize startup; many IDs risk command length/long lifetimes |
| Same file `runWorkshopSteamCMD`; [gamedl.go](../../internal/app/gamedl.go) `runSteamCMD` | Runs after `accounts.Lock(exe)`; anonymous window hidden, `CombinedOutput()` returns all output after exit; cwd exe directory | No Workshop live bytes/percentage callback, context, no-progress timeout, automatic retry |
| Nonanonymous `runWorkshopSteamCMD`; [console_windows.go](../../internal/app/console_windows.go) `startConsole` | Native authentication console; record `logs/console_log.txt` offset, read appended text after exit | Shared-instance concurrency mixes logs/ownership; do not multiply authentication windows |
| `workshopSuccess`, `workshopFailure` | Globally match `Success. Downloaded item <ID>`, `ERROR! Download item <ID> failed (...)`; failures override success. Unconfirmed entries fail with Workshop page; process errors included | Preserve item granularity. Other formats, e.g. `ERROR! Timeout downloading item …`, may remain unconfirmed; complete classification |
| [gamedl.go](../../internal/app/gamedl.go) `errorKind`, `DownloadError.Why` | Classifies ownership/disk/branch/network/other; Workshop reasons retain mapped explanation | Not retry classification: generic `Failure` does not establish network fault |
| [subscribe.go](../../internal/app/subscribe.go) `UpdateSubscriptions` | Per selected SteamCMD Mod calls `downloadWorkshop` with one item (`[]string{ID}`, original source root, false); errors stop following updates | **One launch per Mod**; each success registers source/scans/gets Index, repeating overhead |
| Same file `CheckSubscriptions`; [record.go](../../internal/subscribe/record.go) `Record` | No remote Workshop-version check; SteamCMD `Installed` is download time, not manifest; rerun lets SteamCMD determine increments | `InstalledAt`/About.xml time cannot establish current version |

Game-download progress is not Workshop support: [gamedl.go](../../internal/app/gamedl.go) `FetchGame`, `steamResult.read` poll logs/parse `Update state … progress: … (done / total)`; `downloadWorkshop` bypasses that loop.

### 2.2 Sessions, tool discovery, exclusion

`findTool` defaults to `SteamCMDPath()` → [deps.go](../../internal/deps/deps.go) `Finder.Status`: configured exe/directory, managed `%LOCALAPPDATA%\RimArcho\steamcmd`, PATH. Default path alone cannot identify effective instance.

`SteamCMDAccount`: invocation override → `steamcmd.account` → anonymous. Workshop supplies empty override, hence configured account or anonymous. Only account name passed; SteamCMD handles password/Guard; `GameBranches` explicitly anonymous.

[lock.go](../../internal/accounts/lock.go) `accounts.Lock` normalizes absolute/real paths, using in-process mutex; Windows [lock_windows.go](../../internal/accounts/lock_windows.go) adds path-hashed `Local\RimArcho-account-…` named mutex across same-session RimArcho processes. [accounts.go](../../internal/app/accounts.go) `SteamAccountAction`, `ForgetAccounts`, `FetchGame`, tool installation/branch queries share it. Non-Windows `platformLock` empty: in-process only.

Locks constrain cooperating RimArcho calls, **not manually/other-manager-launched SteamCMD**; key is not library path. Four exe paths writing one library evade existing protection against ACF contention.

### 2.3 Download directories and game visibility

Normal flow:

```text
SteamCMD exe directory: self-update, config, depotcache, logs
    ↓ +force_install_dir <steamcmd-mods.dir>
<base>/steamapps/workshop/content/294100/<ID>
    ↓ success confirmation + About/About.xml check + write .rimarcho/source.json
FolderAdd(source.SteamCMD, content root) → Scan(false) → Index
    ↓ player enables/deploys plan
<game>/Mods/_Archo_<source label>_<directory name> → same source Mod
```

Default `base`: `%LOCALAPPDATA%\RimArcho\steamcmd-mods`, configurable via `steamcmd-mods.dir`. Standard existing sources update in original library; nonstandard roots use managed `.archo-tmp-*`, then `replaceWorkshopImport`: temporary sibling import, inbound-link handling, recycle old directory, rename new, clean staging on exit.

**Normal downloads do not move physical Mods into game Mods.** [deploy.go](../../internal/app/deploy.go) `DeployName`, `planLinks`, `applyLinks` deploy `_Archo_` links. Sources shared across games; do not duplicate per-environment sources for parallelism. Retain [linklifecycle.go](../../internal/app/linklifecycle.go) `PlanIncomingLinks` / begin / finish / abort and running-game update refusals.

## 3. Local measurements

### 3.1 Method and boundaries

- Windows 11 Pro build 26200; Ryzen 9 7950X (16 cores/32 threads), Python 3.13.5; Wi-Fi link 1.2 Gbps, not available internet bandwidth; no speed test/packet capture.
- Managed SteamCMD existed, but experiment downloaded fresh [official Valve package](https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip), **without copying/reading account caches**. Zip 774,825 bytes/about 0.195 seconds; first update/start about 9.225 seconds, bootstrap 7 then update/restart output; actual download clients all `1792617903`, exits 0.
- Exe SHA-256: `7baf77927639793e11b2a02ec2fd6d9114c95432be2f23a3225e274732255cf4`, identifies this version, not future zip versions.
- All installs/scripts/logs/content/ACF in unique system-temp `RimArcho-SteamCMD-research-` GUID directory; no real Steam-library links. All `+login anonymous`, `force_install_dir` before login, no `validate`/automatic retry.
- Copied clean self-updated seed before formal timing; new runtime/empty library per mode × round. Seed copying/self-update/hashes excluded; multiple-runtime cold deployment time/space excluded from gains.
- `time.perf_counter()` from first command start to all subprocess exits/basic directory checks; concurrent wall time, not summed process seconds. Stride split for 2/4 processes (2/1 entries each). Hidden windows, closed stdin, 120-second limit; no formal timeout.
- Three rotated orders: ①empty login, per-item, batch, 2, 4, script; ②4, batch, per-item, script, 2, empty login; ③script, 2, 4, empty login, batch, per-item. No other simultaneous research timing groups; OS/CDN caches uncleared, other machine activity possible: small observational sample.

Samples checked with key/login-free `ISteamRemoteStorage/GetPublishedFileDetails/v1/` POST: `result=1`, `consumer_app_id=294100`, `visibility=0`, `banned=0`, then anonymous downloads verified. Preliminary Wall Light `1423699208` downloaded anonymously but public details `result=9`; replaced and timing discarded, excluded below. Hidden Steam-page template notices do not establish public visibility.

| Workshop item | ID | Downloaded files | Content bytes | SteamPipe logged download bytes |
| --- | --- | ---: | ---: | ---: |
| [CM Color Coded Mood Bar](https://steamcommunity.com/sharedfiles/filedetails/?id=2006605356) | 2006605356 | 47 | 5,402,600 | 800,816 |
| [Allow Tool](https://steamcommunity.com/sharedfiles/filedetails/?id=761421485) | 761421485 | 138 | 1,558,281 | 744,544 |
| [Replace Stuff](https://steamcommunity.com/sharedfiles/filedetails/?id=1372003680) | 1372003680 | 87 | 1,348,299 | 929,984 |
| [Common Sense](https://steamcommunity.com/sharedfiles/filedetails/?id=1561769193) | 1561769193 | 90 | 1,038,445 | 400,416 |
| Total | — | 362 | 9,347,625 | 2,875,760 |

Final column: `content_log.txt` `update started : download 0/N`, planned content bytes, **not received network bytes including handshake/manifest/protocol overhead**.

### 3.2 Formal timing: cold content, initialized runtime

Per-item models current update process calls, excluding RimArcho DB/scans/links/import. Single-process batch models new subscriptions.

| Mode | Round 1 / s | Round 2 / s | Round 3 / s | Median / s | Item success |
| --- | ---: | ---: | ---: | ---: | ---: |
| Serial per-item launch (4 processes) | 42.966 | 28.169 | 25.478 | **28.169** | 12/12 |
| One process, 4 repeated `+workshop_download_item` | 22.362 | 15.610 | 14.065 | **15.610** | 12/12 |
| One `+runscript`, same 4 commands | 12.011 | 14.163 | 14.386 | **14.163** | 12/12 |
| 2 fully isolated processes, 2 items each | 17.932 | 18.374 | 30.567 | **18.374** | 12/12 |
| 4 fully isolated processes, 1 item each | 10.166 | 8.836 | 10.006 | **10.006** | 12/12 |
| Control: start, anonymous connect, quit, no download | 6.706 | 7.934 | 5.710 | **6.706** | N/A; 3/3 normal exits |

Success requires corresponding `Success. Downloaded item` and About.xml. Full-tree hashes of all 60 contents (5 modes × 3 rounds) matched each Mod's 15 copies. Formal processes all exited 0 without item failures; not evidence of zero long-term failure rate.

Versus single-process batch, 2 processes **increase median time 17.7%**, 4 reduce 35.9%; versus script batch, 4 reduce about 29.4%. Rotation cannot attribute script/command-line median gap about 1.45 seconds to `runscript`; both log in once, scripts do not save four more launches.

### 3.3 Shared-directory control: failed processes still exit 0

After formal timing, 4 anonymous processes started together on same public items, fresh temp directories for three rounds. Deliberate race test, not recommendation. Final completeness uses tree hashes after all exit; ACF counts target IDs only in `WorkshopItemsInstalled`, not `WorkshopItemDetails`.

| Runtime / library | Round | Total s | Console successes | Final complete content | ACF installed total |
| --- | ---: | ---: | ---: | ---: | ---: |
| Same exe directory, 4 independent `force_install_dir` | 1 | 13.321 | 4/4 | 4/4 | 4/4 (1 per library) |
| Same | 2 | 10.698 | 4/4 | 4/4 | 4/4 |
| Same | 3 | 22.781 | 4/4 | 4/4 | 4/4 |
| Same exe directory, same `force_install_dir` | 1 | 45.729 | 3/4 | 3/4 | **1/4** |
| Same | 2 | 9.542 | 1/4 | 1/4 | **0/4** |
| Same | 3 | 9.943 | 1/4 | 1/4 | **0/4** |

**All exited 0, including processes with 7 failed downloads.** Round 1 Allow Tool: `ERROR! Download item 761421485 failed (Failure).`; final `WorkshopItemsInstalled` only Replace Stuff despite two other complete Mods. Rounds 2/3: Mood Bar complete/console success, ACF installed empty. Exit code/directory existence alone insufficient.

Experiment proves this version does not safely queue shared writes, not the precise causal file writes, so do not blame ACF alone. Shared `workshop/downloads/294100`, temp/commit directories and per-App state also contend.

### 3.4 Incremental reruns and bottlenecks

Same four commands rerun in three batch libraries: **4.329 / 4.662 / 4.719 seconds, median 4.662, 12/12 success**; hashes/ACF complete. Logs `has no changes`, commit `0 updated`. SteamCMD avoids unchanged retransfers but still pays startup/connection/remote/local checks.

Seconds from each request to `result : OK` in formal batch logs:

| Round | Mood Bar | Allow Tool | Replace Stuff | Common Sense |
| --- | ---: | ---: | ---: | ---: |
| 1 | 4 s | 2 s | 1 s | 2 s |
| 2 | 3 s | 1 s | 1 s | 2 s |
| 3 | 3 s | 2 s | 1 s | 1 s |

Intervals include info/manifest/download/decompression/validation/writes, not pure transfer. About 2.88 MB content, empty-login median 6.706 seconds, batching better than per-item launches: key opportunities **startup/session overhead and fixed item latency**. Single-process logs already show multiple HTTP/2 CDN connections/chunks; one command is not one TCP connection.

**Maximum single-large-Mod throughput unmeasured; no claim one download cannot saturate bandwidth.** No isolated traffic/large-file test; 9.35 MB decompressed size divided by total time is not speed. Large Mods may be bandwidth/CDN/disk/CPU/validation limited; multiprocessing may not help. Four small Mods/one Windows/three rounds do not establish optimal hundred-item concurrency.

## 4. What safe parallelism must isolate

| Shared resource | Evidence / references | Requirement |
| --- | --- | --- |
| Installation, self-updating exe/dll/package | First launch updates/restarts; shared-directory multiprocess maintenance not guaranteed | Exclusive installation/update; independent initialized runtimes, no hardlinks/junctions sharing writable files |
| `config/config.vdf`, libraryfolders | Per-instance config generated; Valve says login may update config.vdf | Content-root isolation does not isolate config; real-account instance stays exclusive, never cloned into anonymous workers |
| `ssfn`/sentry/credentials | [Prior session research §3](2026-10-24_login-sessions-loverslab-steamcmd.md#3-steamcmd); real caches unread, sessions untested | Never copy/merge/parse; anonymous workers from official install/credential-free seed; absent ssfn does not prove no login cache |
| `depotcache` | Generated `294100_<manifest>.manifest` | Independent per worker, retained for reuse, not cleared while running; no assumed multiwriter protocol |
| `steamapps/workshop/appworkshop_294100.acf` | Shared-library tests lost records | Isolate whole library, not just `<ID>`; no concatenating/overwriting ACF after parallel execution |
| `workshop/downloads/294100`, staging/final content | Shared-library failures may exit 0 | Independent base, verify successes, one publisher to final source |
| `logs/console_log.txt`, workshop/content logs | Nonanonymous parsing reads offset tails; same-directory processes share writes | Separate worker logs/offsets; associate request ID/worker/batch events |
| Lock files/named locks | No dependable persistent `.lock` found; 4 shared-path processes ran/damaged state | No visible file does not prove no locks, but internal protection insufficient here; retain instance lock, add final-source publishing lock |

This Windows version's `force_install_dir` **does place Workshop content/appworkshop ACF under specified base**, verified. Not all runtime state moves; historical platforms/versions have [path issues](https://github.com/ValveSoftware/steam-for-linux/issues/11662). Verify actual writes/success on new platforms, not treating argument as complete sandbox. Linux may use user Steam config; RimSort supplies separate home. Linux/macOS untested.

[Valve login-cache docs](https://partner.steamgames.com/doc/sdk/uploading#Using_SteamPipe_In_A_CI_CD_Environment) confirm token/config.vdf writes; [official EResult](https://partner.steamgames.com/doc/api/steam_api#EResult) includes `LoggedInElsewhere`, `LogonSessionReplaced`, `PasswordRequiredToKickSession`. Conflicts exist, **not proof every version/session immediately evicts another**. No real login tested; conservative rule serializes same-account download/login/verify/forget despite separate directories. Four anonymous sessions coexisted, still subject to same-IP/CDN/server limits, thresholds untested.

Community [SCMDAutomated](https://github.com/Risonna/SCMDAutomated) author describes anonymous multi-instance/logged-in single-instance, acknowledging failure risks. Corroboration, not isolation-test substitute/production proof. Valve Developer Wiki page returned 403; unread text/old-mirror one-login claim not treated as current guarantee.

## 5. Two local reference projects

Local checkout implementations, not arbitrary online versions.

| Project / snapshot | Organization | Results, retry, isolation |
| --- | --- | --- |
| RimSort `4964bf8900aa31f946591cd1c711be58d0ef2873` | [wrapper.py](../../../RimSort/app/utils/steam/steamcmd/wrapper.py) `SteamcmdInterface.download_mods`: `STEAMCMD_BATCH_SIZE=25`; `_build_download_script`: force_install_dir, login anonymous, one ID per line, optional validate, quit; one process/batch | [runner_panel.py](../../../RimSort/app/windows/runner_panel.py) `finished` → `_start_next_steamcmd_batch` serially; no worker pool. Per-item counters, prompt retry remaining afterward |
| Same runtime | Tool `<prefix>/steamcmd`, content `<prefix>/steam`, depotcache in tool directory | `_get_process_environment` separate Linux home under prefix protects desktop library config; `_build_download_script` uses fixed `steamcmd_script.txt` unsuitable unchanged for parallelism |
| RimModManager `90740602c47d4fa62136237b253346716832dced` | [mgr_steam.py](../../../RimModManager/backend/managers/mgr_steam.py) `download_workshop_items` background thread, `_run_steamcmd_process` batches 25 under `FileManager.storage_lock`; `_run_single_steamcmd_batch` one process/unique temporary anonymous script | Serial loop, not multiprocess parallel; `_consume_steamcmd_log_progress` reads logs, `_retry_steamcmd_batch` shrinks failures 10/1 with depth limit; single-item early return, not infinite retries |
| Same runner | `cwd=self.steamcmd_dir`; normal scripts lack force_install_dir, use tool library | 180-second no-progress timeout/process-tree cancel; `last_progress_at` mainly completion/failure/batch-error, so unfinished large downloads may falsely time out. `_steamcmd_lock` protects task map; storage_lock serializes download loop |

RimModManager [mgr_download.py](../../../RimModManager/backend/managers/mgr_download.py) `ThreadPoolExecutor(max_workers=5)` is generic, not SteamCMD concurrency 5. Separate Workshop-repair flow writes Steam ACF/files; not called, should not accompany normal acceleration.

RimSort 25-item cap defensively avoids vprof thread-table exhaustion, source/original [issue #1411](https://github.com/RimSort/RimSort/issues/1411) support it; not a Valve protocol limit of "at most 25 Mods per download". [User guide](https://rimsort.github.io/RimSort/user-guide/steamcmd-browser) confirms client-independent RimWorld Mod downloads, consistent with anonymous samples.

## 6. Recommended implementation (no code changes here)

### 6.1 Keep single instance first, batch updates

Default concurrency **1**; deduplicate same-source/account IDs, initially **25** per batch, restart between batches. 25 empirical from references, adjustable, not protocol guarantee. Unique temporary `runscript` avoids Windows long commands/fixed-name races; preserve other successes after one failure.

Separate execution/parsing from validation/publication/scanning; aggregate link lifecycle per source, write origins/scan/index successes together, report partial failures. Collecting IDs in `UpdateSubscriptions` must retain returned items/original positions/distinct roots.

| File / function | Suggested change |
| --- | --- |
| [subscribe.go](../../internal/app/subscribe.go) `UpdateSubscriptions` | Group SteamCMD updates by real root/effective account; preserve other types' order/errors, avoid process/scan per Mod |
| [subscribeworkshop.go](../../internal/app/subscribeworkshop.go) `downloadWorkshop` | Separate execution/result merge/publication, cap 25, coordinate links, one final Scan/Index |
| Same `runWorkshopSteamCMD`; [gamedl.go](../../internal/app/gamedl.go) runner | Unique-script cleanup, incremental events, cancel/no-progress checks; keep `accounts.Lock(exe)` |
| `workshopSuccess`, `workshopFailure`, `errorKind` | Add timeout/connection/auth forms; exit 0 still requires item/About.xml checks; retry eligible failures only |
| [subscribeworkshop_test.go](../../internal/app/subscribeworkshop_test.go), [subscribe_test.go](../../internal/app/subscribe_test.go) | Future grouping/batching/mixed outcomes/exit-0 failure/cancel/link tests; existing tests do not prove real concurrency safety |

### 6.2 Optional isolated anonymous pool: 1/2/4, never "2 is faster"

Second phase: explicitly disableable anonymous acceleration, **cap 4**; existing/external instances/real accounts single-instance. Four measured useful locally for similar small-Mod queues; 2 only resource-limited/diagnostic, not recommended speed tier. Default 4 requires broader networks/queues/SSD/HDD/large-Mod evidence; default stays 1.

Each worker owns installation/config/appcache/depotcache/logs/base/steamapps; stop for initialization/upgrades, source only credential-free dedicated seed. Never clone user's SteamCMD or pretend different cwd for same exe isolates state.

Prefer stable final source; workers download privately, coordinator publishes under target-source lock via `replaceWorkshopImport`/link lifecycle, one final scan. **Worker goroutines must not call entire `downloadWorkshop`**: `subscriptionCache`, source registration, DB/Index, links/recycle are not inherently concurrent.

Pool adds cache copies/publication state: define storage first. Retain worker ACF/cache, route same ID back when possible, persist final-source-applied manifest. Do not discard worker manifest after moving/splice ACF. Download success/publication failure retains retryable content/error, not completion. Publication-only final root needs old-ACF ownership migration and disabling old direct writes designed together; do not enable pool beforehand.

Alternative worker roots registered separately avoid copies but alter source UX/reassignment, not a spec-free shortcut.

Future locations: planning/results/publication `subscribeworkshop.go`; startup `gamedl.go`/worker runner; retain `accounts/lock.go` instance locks/add source locks; `deps/deps.go` defines worker/account relation; `accounts.go`/uninstall maintain new directories. Sync directories/settings/state with [01 write inventory](../specs/01-scope-and-status.md), 11 §1, 11 §3, settings specs. No passwords/tokens/ssfn in business parameters/reports.

### 6.3 Other gains and fallback

| Technique | Suggested policy | Risks/fallback |
| --- | --- | --- |
| Skip current items | Initially retain ACF/depotcache for increments; later player-requested batched public-version signals compared with reliable applied manifest/time; unknown/missing/damaged content goes to SteamCMD | Local download time not remote version; latest-at-install cannot ensure latest offline; metadata failure uses existing checks |
| Retry failures | Failed IDs only, temporary connection/timeouts max 2 extra attempts, about 5/15-second jittered backoff; batch errors 25→10→1. Classify Failure; no blind permission/private/unlicensed retries; stop disk-full | Allowed fields only, no full auth console; rate-limit pauses/reduces concurrency, respects service wait |
| Timeout/cancel | Independent process/item/trustworthy byte/disk progress; initial 180 seconds without effective progress, separate auth-wait; terminate owned tree only | Incomplete item alone not no-progress; tolerate ongoing large downloads; never global taskkill steamcmd.exe |
| Mixed sizes | Balance remaining size, start large early/fill others small, 25-item plus byte budgets avoids long batch blocking | Unknown size normal queue; saturated network/disk drops to 1–2; do not extrapolate the measured 4 concurrent small-Mod workers to large packages |
| Cache/validation | Retain manifests/caches, no per-batch depotcache purge/default validate; retain About.xml/publication validation | Targeted validation/redownload for damage; cache purge repair, not routine speedup |
| Rollout/fallback | Batch updates first, pool separately optional; version anomalies/repeated failures/rate limits/inconsistency finish processes then remaining items single-worker/instance | Never shared-original-library multiprocess fallback; no redownload of published successes; accounts original exclusive console; single-worker publisher respects ACF ownership |

## 7. Evidence, limits, reviewability

| Evidence | Finding | Recommended path |
| --- | --- | --- |
| E1: §2 local source/HEAD; `rg -n 'downloadWorkshop|runWorkshopSteamCMD|accounts.Lock' internal/app internal/accounts` | F1: additions batch, updates per-item; required publication/account-lock contracts | P1: aggregate updates, batches, shared publish/scan |
| E2: §3.1 metadata, §3.2 three timings/60 trees; reproduce §8 | F2: four isolated anonymous processes useful locally, two not guaranteed | P2: default 1, optional isolated 4-worker/resource-limited 2-worker tiers, more samples before default |
| E3: §3.3 three shared-library controls/final ACF; failures exit 0 | F3: shared-library failures, files/ACF diverge | P3: full isolation/item checks/single publisher/app locks |
| E4: §5 local symbols/original issue | F4: references serial small batches; 25 empirical | P4: borrow batching, do not confuse threads/HTTP pools with SteamCMD concurrency |
| E5: §4 original Valve docs; no real login | F5: isolate runtime, account multisession safety unverified | P5: serialize accounts, no credential cloning/no eviction guarantees |

Only this Windows client/four public items observed. Untested real accounts/private items/hundreds-item queues/large-Mod maximum throughput/concurrent bootstrap/self-update/power-loss recovery/cross-platform. Download does not require complete dependencies; no game launch verified functionality/compatibility.

Dedicated temp directory removed after experiments; raw logs/scripts not permanent project files. Actual scripts/raw timings/key excerpts/tree fingerprints below support independent reruns. Fingerprint: sorted relative POSIX paths, concatenate per-file `UTF8(relative path) + NUL + SHA256(file contents)`, then SHA-256.

| ID | Matching tree fingerprint for 15 formal downloads per Mod |
| --- | --- |
| 2006605356 | `18d072af6d6f71cf0f4c9f246f45e6438beb9b8d3bf9ba04148e196d0e3a040e` |
| 761421485 | `849d55095e0c78afebb054fd590caf3e050138cd23cdc5a9cd298791be002e57` |
| 1372003680 | `cc5ced34522840882d18476234486bbe4b32bedc094bd9613481b7cbdd4c7058` |
| 1561769193 | `2f31a951a3e0dd3fc8464659f53780913904e06c407ec883ab84923a3e4b4716` |

## 8. Rerun steps and actual scripts

Requires Windows/PowerShell/Python stdlib/Steam-CDN network. New temporary directories only; never existing Steam/game paths. Verify public samples independently (POST values change), prepare unlogged seed:

```powershell
$publicIds = @('2006605356', '761421485', '1372003680', '1561769193')
$publicBody = @{itemcount=$publicIds.Count}
for ($i=0; $i -lt $publicIds.Count; $i++) { $publicBody["publishedfileids[$i]"]=$publicIds[$i] }
$publicDetails = Invoke-RestMethod -Uri 'https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/' -Method Post -Body $publicBody
$publicDetails.response.publishedfiledetails | Select-Object publishedfileid,result,consumer_app_id,visibility,banned,file_size
if (@($publicDetails.response.publishedfiledetails | Where-Object { $_.result -ne 1 -or $_.consumer_app_id -ne 294100 -or $_.visibility -ne 0 -or $_.banned -ne 0 }).Count) { throw 'Samples are no longer all public; stop rerun' }
$benchRoot = Join-Path ([IO.Path]::GetTempPath()) ('RimArcho-SteamCMD-research-' + [guid]::NewGuid().ToString('N'))
$benchSeed = Join-Path $benchRoot 'seed'
New-Item -ItemType Directory -Path $benchSeed | Out-Null
Invoke-WebRequest -Uri 'https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip' -OutFile (Join-Path $benchRoot 'steamcmd.zip')
Expand-Archive -LiteralPath (Join-Path $benchRoot 'steamcmd.zip') -DestinationPath $benchSeed
# Complete self-update; rerun +quit if necessary, confirm no more updates before timing.
Start-Process -FilePath (Join-Path $benchSeed 'steamcmd.exe') -ArgumentList '+quit' -WorkingDirectory $benchSeed -WindowStyle Hidden -Wait
Start-Process -FilePath (Join-Path $benchSeed 'steamcmd.exe') -ArgumentList '+quit' -WorkingDirectory $benchSeed -WindowStyle Hidden -Wait
```

Save formal benchmark below as `$benchRoot\bench.py`, run `python (Join-Path $benchRoot 'bench.py') $benchRoot`. Round directories must not exist; no clearing/overwriting old rounds, use new benchRoot.


```python
import pathlib, shutil, subprocess, time, json, re, sys, concurrent.futures, statistics
root=pathlib.Path(sys.argv[1]).resolve()
ids=['2006605356','761421485','1372003680','1561769193']
results=[]
def launch(worker,base,items,label,script=False):
    args=[str(worker/'steamcmd.exe'),'+force_install_dir',str(base),'+login','anonymous']
    for item in items: args += ['+workshop_download_item','294100',item]
    args+=['+quit']
    if script:
        scriptfile=worker/(label+'.txt')
        scriptfile.write_text('force_install_dir "'+str(base)+'"\nlogin anonymous\n'+''.join('workshop_download_item 294100 '+i+'\n' for i in items)+'quit\n',encoding='utf-8')
        args=[str(worker/'steamcmd.exe'),'+runscript',str(scriptfile)]
    log=root/(label+'.log'); started=time.perf_counter()
    with log.open('wb') as f:
        p=subprocess.Popen(args,cwd=worker,stdin=subprocess.DEVNULL,stdout=f,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW)
        try: code=p.wait(timeout=120)
        except subprocess.TimeoutExpired:
            subprocess.run(['taskkill','/PID',str(p.pid),'/T','/F'],capture_output=True,creationflags=subprocess.CREATE_NO_WINDOW); code='timeout'
    elapsed=time.perf_counter()-started
    s=log.read_text(errors='replace')
    success=re.findall(r'Success\. Downloaded item (\d+)',s)
    files={}
    for i in items:
        folder=base/'steamapps'/'workshop'/'content'/'294100'/i
        about=folder/'About'/'About.xml'
        files[i]={'about':about.is_file(),'bytes':sum(p.stat().st_size for p in folder.rglob('*') if p.is_file()) if folder.exists() else 0}
    return {'label':label,'seconds':round(elapsed,3),'exit':code,'requested':items,'success':success,'failures':re.findall(r'ERROR![^\r\n]*',s),'files':files,'acf':(base/'steamapps'/'workshop'/'appworkshop_294100.acf').is_file()}
def bench(mode,repeat):
    home=root/(mode+'-'+str(repeat)); home.mkdir()
    count=1 if mode in ['peritem','batch','runscript','login'] else int(mode[-1])
    workers=[]
    for i in range(count):
        w=home/('worker'+str(i));shutil.copytree(root/'seed',w,ignore=shutil.ignore_patterns('logs','*.old','*.delete'));workers.append(w)
    t=time.perf_counter()
    if mode=='peritem':
        runs=[launch(workers[0],home/'content0',[item],mode+'-'+str(repeat)+'-'+str(i)) for i,item in enumerate(ids)]
    elif mode in ['batch','runscript','login']:
        runs=[launch(workers[0],home/'content0',[] if mode=='login' else ids,mode+'-'+str(repeat),mode=='runscript')]
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=count) as pool:
            jobs=[pool.submit(launch,workers[i],home/('content'+str(i)),ids[i::count],mode+'-'+str(repeat)+'-'+str(i)) for i in range(count)]
            runs=[j.result() for j in jobs]
    result={'mode':mode,'repeat':repeat,'seconds':round(time.perf_counter()-t,3),'ok':sum(1 for r in runs for i in r['requested'] if i in r['success'] and r['files'][i]['about']),'requested':sum(len(r['requested']) for r in runs),'runs':runs}
    results.append(result);(root/'results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2),encoding='utf-8')
    print(json.dumps({k:v for k,v in result.items() if k!='runs'},ensure_ascii=False),flush=True)
for repeat,modes in enumerate([['login','peritem','batch','parallel2','parallel4','runscript'],['parallel4','batch','peritem','runscript','parallel2','login'],['runscript','parallel2','parallel4','login','batch','peritem']],1):
    for mode in modes: bench(mode,repeat)
print('BENCHMARK_COMPLETE',flush=True)
```

After formal benchmark exits, save next as `extras.py` in same temp directory; run `python (Join-Path $benchRoot 'extras.py') $benchRoot`. Reuses bench.py launch, three shared-runtime/library controls then existing batch reruns. Not concurrent with formal timing.

```python
import pathlib, sys
root=pathlib.Path(sys.argv[1]).resolve()
source=(root/'bench.py').read_text(encoding='utf-8')
exec(source.split('for repeat,modes in enumerate(')[0])
extra=[]
for repeat in range(1,4):
    for shared_output in [False,True]:
        mode='shared-all' if shared_output else 'shared-runtime'
        home=root/(mode+'-'+str(repeat));home.mkdir()
        worker=home/'worker';shutil.copytree(root/'seed',worker,ignore=shutil.ignore_patterns('logs','*.old','*.delete'))
        started=time.perf_counter()
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            jobs=[pool.submit(launch,worker,home/('content0' if shared_output else 'content'+str(i)),[item],mode+'-'+str(repeat)+'-'+str(i)) for i,item in enumerate(ids)]
            runs=[j.result() for j in jobs]
        acfs={}
        for acf in home.glob('content*/steamapps/workshop/appworkshop_294100.acf'):
            text=acf.read_text(errors='replace')
            sections=re.split(r'"WorkshopItemsInstalled"\s*\{',text)
            installed=sections[1].split('"WorkshopItemDetails"')[0] if len(sections)>1 else ''
            acfs[str(acf.relative_to(home))]=[i for i in ids if '"'+i+'"' in installed]
        result={'mode':mode,'repeat':repeat,'seconds':round(time.perf_counter()-started,3),'ok':sum(1 for r in runs for i in r['requested'] if i in r['success'] and r['files'][i]['about']),'runs':runs,'acf_ids':acfs}
        extra.append(result);(root/'extra-results.json').write_text(json.dumps(extra,ensure_ascii=False,indent=2),encoding='utf-8')
        print(json.dumps({k:v for k,v in result.items() if k!='runs'},ensure_ascii=False),flush=True)
for repeat in range(1,4):
    home=root/('batch-'+str(repeat))
    run=launch(home/'worker0',home/'content0',ids,'warm-'+str(repeat))
    extra.append({'mode':'warm','repeat':repeat,'seconds':run['seconds'],'runs':[run]})
    (root/'extra-results.json').write_text(json.dumps(extra,ensure_ascii=False,indent=2),encoding='utf-8')
    print(json.dumps({'mode':'warm','repeat':repeat,'seconds':run['seconds'],'ok':len(run['success'])}),flush=True)
print('EXTRA_COMPLETE',flush=True)
```

Result `ok` is preliminary per-process-exit check; shared directories require whole-group recheck. Hash all formal trees, extras final contents against baseline; ACF only WorkshopItemsInstalled. Final verification produced §3.3, not transient exit snapshots.

Confirm no experiment children remain; cleanup in same PowerShell session. Checks restrict target to direct system-temp child matching GUID pattern, never user directory:

```powershell
$benchResolved = [IO.Path]::GetFullPath($benchRoot).TrimEnd('\')
$benchTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
if ((Split-Path $benchResolved -Parent) -ne $benchTemp -or (Split-Path $benchResolved -Leaf) -notmatch '^RimArcho-SteamCMD-research-[0-9a-f]{32}$') { throw 'Cleanup target is outside dedicated temporary directory' }
$benchRunning = Get-CimInstance Win32_Process -Filter "Name='steamcmd.exe'" | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($benchResolved + '\', [StringComparison]::OrdinalIgnoreCase) }
if ($benchRunning) { throw 'Experiment SteamCMD still running; wait for exit' }
Remove-Item -LiteralPath $benchResolved -Recurse -Force
```
