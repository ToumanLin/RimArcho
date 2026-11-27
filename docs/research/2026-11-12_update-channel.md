# Parallel update checks and GitHub/Lanzou release pairs

> **Status (2026-11-13):** integrated; the behaviour is specified in [17 §5.1](../specs/17-engineering.md#51-rimarcho-self-update). The maintainer then dropped the `.sha256` requirement on Lanzou: the folder holds only the zip, which is checked against GitHub's checksum when that can be fetched, otherwise by its CRC-32, the extraction checks and the start-up test. Manual install takes the zip alone (`archo update --from <zip>`). Where §1, §3 and §7 below require a pair, the specification wins.

> Research and unwired prototype, 2026-11-12 (America/Los_Angeles), with live observations on 2026-11-13 UTC.
> **[C]** means confirmed by a live HTTP/browser observation. Maintainer-supplied browser evidence is attributed explicitly; source inspection and proposals are not [C].
> Basis: [17 §5.1](../specs/17-engineering.md#51-rimarcho-self-update), [earlier Workshop research](2026-11-05_workshop-distribution.md).

## 1. Decisions

1. **Keep the release layout exactly as today:** `archo-<X.Y.Z>-windows-amd64.zip` plus `<zip>.sha256`, in the existing sha256sum format. Publish the same pair to GitHub Releases and `https://rimarcho.lanzn.com/b01i608qja` (password `ctot`). ZIP roots, bundled files, extraction and swap/rollback stay under 17 §5.1. This design adds no package entries or update-file format requiring a stepping-stone updater release.
2. **Check three metadata sources concurrently, with one overall deadline, and choose the highest stable version:** GitHub Releases latest; the standalone Archo Monitor item's description stamp via anonymous `GetPublishedFileDetails`; and the Lanzou folder's exact stable ZIP filenames. Preserve each source's result. A failed/timed-out source never discards another source's successful result. Checking order is separate from download priority.
3. The new Workshop item carries **only Archo Monitor**. Its description ends with `RimArcho v<X.Y.Z>`, read by `selfupdate.DescriptionVersion`. No item-file reads, subscriptions, SteamCMD, tags or feed are involved in checks. Its ID is unassigned: `updatechannel.MonitorWorkshopItem` remains empty, with `RIMARCHO_UPDATE_MONITOR_ITEM` as the future VM/test override. Never reuse the retired program item **3835688284**.
4. **Download precisely the checked version: GitHub pair first, Lanzou pair second, manual browser pair last.** A version known only from Lanzou is eligible for Lanzou download. Both automatic sources require the exact ZIP's `.sha256`; missing/invalid checksum fails that source while the version can still support a notice. Never silently substitute another version.
5. Lanzou checks stop after folder GET/password POST/pagination. File details, iframe/download API and download node are installation-time work after player confirmation. The resolver computes the fixed `acw_sc__v2` cookie and follows allowed redirects, like RimCrow. Non-challenge HTML or a repeated challenge on the same node host fails the source. **Never automate the `down_r(el)` human-verification page or its POST.**
6. Manual fallback opens the public share in the player's browser. The player can complete human verification and pick the ZIP and `.sha256`; RimArcho can find the sidecar beside the ZIP or resolve that exact sidecar from the listing. Verify using `selfupdate.VerifyLocal` unchanged. Missing/mismatching checksum never authorizes installation.
7. `updates.source github` uses GitHub only. Prereleases stay GitHub-only. This prototype does not change Linux's existing notification path. Object storage (OSS/COS) is **not planned**.

Implementation is limited to [internal/lanzou](../../internal/lanzou/client.go), [internal/updatechannel](../../internal/updatechannel/check.go), [cmd/lanzou-probe](../../cmd/lanzou-probe/main.go), and research documentation. Application, public CLI, UI, actual specifications, CHANGELOG and release scripts are untouched. Per-source reporting and browser selection are library/design work, not newly available `archo` controls. No upload, publication or game/application data access occurs.

## 2. Checks and source results

`updatechannel.Check` starts at most three tasks concurrently under one deadline (default 30 seconds, maximum 60). It uses `selfupdate.Client.Latest`, an anonymous single-item POST equivalent to `app.postWorkshopDetails`, and `lanzou.Client.List`. Standard transports get a five-second connect timeout; TLS and response-header deadlines are separate. Successful results survive other source failures or the overall deadline. Result order stays GitHub, Monitor, Lanzou, while the maximum semantic version wins independently of latency.

Each `SourceResult` records source, status (`ok`, `failed`, `timeout`, `unconfigured`), version, elapsed time, safe failure reason and whether a checksum asset was advertised. This is input for a later `archo update --check` display; the CLI is not wired. Monitor supplies a version only. GitHub/Lanzou checksum presence means an asset/filename was advertised, not that its contents were downloaded or validated. Verification remains mandatory at download time.

Proposed display example, not an implemented CLI contract:

```text
github   failed       GitHub release lookup failed
monitor  unconfigured standalone Monitor item ID is not assigned
lanzou   ok           version=1.0.10 checksum_present=false
latest=1.0.10
```

The Web API request uses `itemcount=1`, `publishedfileids[0]=<Monitor ID>`, with no credentials. Require HTTP 200, exactly that returned ID, result 1 and a stable description stamp. Missing item/stamp, result 9, malformed/oversized response or invalid ID fails just Monitor. Tags do not substitute for the stamp. No named account, SteamCMD install/run, local Workshop scan or item-file download occurs. The application-layer `postWorkshopDetails` remains unchanged; the unwired adapter duplicates its small request rather than importing app.

A stamped Monitor version can precede binary mirror publication: report it, then show download failures if its pair is unavailable. A stale/dead source cannot suppress a newer result. If all sources fail, do not claim the player is up to date. There is no cryptographic freshness/rollback protection; future integration still compares against the running version and never downgrades.

### RimCrow comparison (read only)

Inspected `../../../../RimModManager/backend/managers/mgr_update.py`, `_check_sources`: a four-worker pool, one overall timeout, per-source results, maximum version wins. Its same-version downloads are sorted by latency; RimArcho retains the fixed order in §1. Its Lanzou source infers versions from supported ZIP filenames and supplies `file_hash=None`: the inspected download path has **no checksum verification**.

**Local-code discrepancy:** `LanzouSource.check()` calls `LanzouParser.get_all_files()`, which also calls `get_file()` after selecting a filename and resolves download details. Thus the inspected local code is not strictly listing-only and may fail during a check at the node. Folder metadata alone can reveal the version without that chain. My `Client.List` deliberately stops before file details, and synthetic tests reject any file/node request during checks.

RimCrow's `_resolve_download_url` stops on non-acw HTML or a second challenge on the same host, including the human-verification page. Its folder parser bounds pagination at 100 pages and also stops on a short page. Mine scans through an empty page/status 2, bounded at 16 pages and shared budgets, never returning an incomplete successful listing. RimCrow code was inspected read-only and not copied.

## 3. Download, verification and manual fallback

`updatechannel.Download(ctx, options, checkedVersion, stagingDir)` looks up that exact stable GitHub version through `selfupdate.Client.Version`, then uses the existing `Client.Download` for its ZIP/checksum pair. The GitHub-only prerelease prototype requires the lookup still to name the checked version; a changed result fails safely. Later integration can retain the checked release snapshot. Downloads and fallback share a five-minute absolute deadline.

If GitHub fails, `lanzou.Client.Download` lists afresh, requires exactly the checked ZIP and `.sha256`, resolves the checksum first, then the ZIP, and delegates verification to `selfupdate.VerifyLocal`. A higher filename appearing during download does not change the selection. Missing/ambiguous files, failed pagination, changed HTML, human verification, cancellation, limits or checksum mismatch fail the source and remove staging. Neither package extracts, installs or executes anything.

GitHub supplies its asset size and sidecar. Lanzou's rounded listing size is not an authoritative byte count: the transfer enforces 200 MiB, measures actual bytes and checks the staged ZIP's digest against the sidecar. It checks the staged copy again with that measured size. Manual verification uses a positive known size when available; zero retains `VerifyLocal`'s existing checksum-only size convention. No size manifest or new checksum syntax is introduced.

`updatechannel.VerifyLocal` accepts an explicit sidecar or looks for `<selected zip>.sha256`, then the exact release sidecar in the same directory. `VerifyWithChecksum` also accepts bounded bytes from `lanzou.Client.Checksum`. Both call unchanged `selfupdate.VerifyLocal` with the checked release filename. Browser-renamed ZIPs are allowed if their contents match. A filename-bearing checksum must identify the checked ZIP; the existing parser also accepts a bare hash.

Future manual installation retains the lock (17 §5.1 step 1), copies selected inputs to private `download/` staging, re-verifies the staged ZIP (the selected input can change), then follows steps 3–7: archive-root/path/symlink/executable/expansion checks, process wait, journaled swap, startup verification/rollback and restart. Selected files remain untouched. Confirmation, locks, progress, browser and picker controls are not wired here. Proposed CLI shape `archo update --from <zip> [--checksum <sidecar>]` is design only.

### Trust boundary

A ZIP plus a `.sha256` from the **same source detects corruption, incomplete transfers and accidental mismatches**. It does not authenticate a compromised source: control of the GitHub repository or Lanzou folder can allow replacing both ZIP and checksum with a matching malicious pair. Explicitly selecting a malicious pair has the same limitation. This is the same trust model as today's GitHub updater, with integrity checking absent from RimCrow's inspected Lanzou path.

A compromised Steam/Monitor description can advertise a misleading version or deny service, but the stamp cannot supply a binary, hash, mirror URL or executable instructions. Installation still needs a matching pair from the configured GitHub/Lanzou location or explicit manual selection. This is not a guarantee that any single compromised distribution account is harmless: an account controlling **both ZIP and sidecar** defeats same-source checksums. This design has neither signatures nor independently pinned expected hashes.

## 4. Fixed Go resolver and bounds

The download chain is folder GET, fixed cookie retry, password/pagination, exact full file ID, file page, iframe, download API, node redirects/cookie retry, bytes. IDs retain query state. There is no recipe interpreter, arbitrary script execution or downloadable parser. Lanzou page/API changes can require a normal RimArcho release; that maintenance cost is accepted. Cookies and bearer URLs remain session-local and are never logged or persisted.

| Resource | Bound |
| --- | --- |
| Destinations | HTTPS port 443; exact configured hosts; no userinfo/fragments/wildcards |
| Default hosts | Share host, `apifile.woozooo.com`, `developer4.lanrar.com` |
| DNS/connect | Every answer must be public; dial a checked literal IP with original TLS hostname; five seconds including DNS |
| TLS / response headers | Five / ten seconds; request/response headers at most 16 KiB |
| Requests / redirects | 24 / 6 per session, shared across listing, resolution, cookies and transfers |
| Pagination | 16 pages including terminating empty page; failed/no-progress/incomplete listings fail |
| Metadata | 1 MiB wire/decoded body, 4 MiB aggregate; bounded gzip; JSON depth 32, no duplicate/case-colliding keys |
| Values / forms | 8 KiB values/URLs; 16 fields / 16 KiB encoded form |
| Cookies | Fresh host-local state, 16 cookies / 8 KiB |
| Time | 20 seconds per metadata request; 60 seconds per listing/resolution; three minutes per transfer, four per Lanzou pair |
| Files | ZIP at most 200 MiB; checksum at most 64 KiB; read one extra byte to detect overflow |

Budgets apply together: requests/time can end a large listing/pair before its page limit. POST redirects fail; cross-host GET redirects discard request-specific headers and keep cookies host-local. Reject private/loopback/link-local/CGNAT/reserved DNS answers, including mixed public/private responses. Do not retry HTTP 429/5xx or human checks automatically. Synthetic tests alone can inject one exact httptest origin; callers cannot enable that exception.

Lanzou uses direct connections, clones standard `http.Transport`, and rejects custom wrappers. GitHub/Steam standard transports retain routing/TLS settings with short connects. Application `netcfg` wrapper/proxy integration is deferred until wiring. Unknown redirect hosts fail closed; the diagnostic's `-hosts` permits explicitly reviewed exact extra hosts while retaining HTTPS/public-IP enforcement.

## 5. Live evidence

Serial requests used fresh cookie state against the maintainer-provided share/password, with bounded bodies and no game, Mods, Steam account or RimArcho data access. Nothing downloaded was installed/executed. Bearer queries and cookies are omitted.

| Evidence | Observation | Consequence |
| --- | --- | --- |
| E1 [C] | Folder GET returned HTTP 200 containing `arg1` / `acw_sc__v2`; fixed permutation/XOR cookie retry returned folder HTML | HTTP 200 alone is insufficient; the fixed strategy handles this observed page. |
| E2 [C] | Password POST returned `archo-1.0.10-windows-amd64.zip`; follow-up pagination reached empty page 2 | Folder metadata alone reveals the stable version. |
| E3 [C], replacement-design listing test | PASS: `version=1.0.10 checksum_present=false`; folder/listing requests only, on `rimarcho.lanzn.com` | A version notice works. The sidecar was absent at observation time, so this Lanzou release cannot yet pass pair verification. |
| E4 [C] | Full file ID/query led to `/fn?...` iframe and a download API resolver under `developer4.lanrar.com` | Preserve query state; resolver host is not a confirmed final byte host. |
| E5 [C], replacement-design ZIP probe | Node returned a cookie challenge, retry returned HTTP 200 human-verification HTML; exit 1 | Clear source failure; ZIP byte count/hash unavailable. |
| E6 [C], maintainer's desktop browser, 2026-11-13 | Outside China, downloaded directly without verification | Browser/network context can permit a direct download; no final host/size/hash was supplied. |
| E7 [C], maintainer-supplied real Chromium pane on this machine, 2026-11-13 | Password, file page and iframe link also reached human verification; anchor had `rel=noreferrer`, `target=_blank`; page also loaded `/file/kdns.js` | Go alone is not the cause. Network/browser reputation affects the page; exact trigger is unknown. |

[C] A fresh Go session with Chrome User-Agent and no download Referer still reached verification. No final successful transfer, speed/range measurement, token TTL or rate ceiling was established. No HTTP 429 was observed. UI timers/cookie lifetimes do not establish server token lifetime. The page advertised alternative API/domain routes; successful use was not confirmed, and port 661 remains rejected. Verification submissions and arbitrary page scripts are outside the supported chain.

## 6. Validation and reproduction

Default tests use only synthetic ZIP/checksum bytes, httptest and t.TempDir. Coverage includes concurrent start/max-version selection/deadline isolation; GitHub-only/prerelease routing; Monitor ID/stamp/results; listing-only pagination/exact names; failed/ambiguous/missing pages; exact-version pair selection; checksum mismatch/limits; human/repeated challenges without verification POSTs; DNS/destination/resource limits; local pair checks; cleanup and probe output redaction. Tests never run SteamCMD or touch real game/Mod/application state.

```powershell
go build ./...
go vet ./internal/lanzou/ ./internal/updatechannel/ ./cmd/lanzou-probe/
go test -count=1 ./internal/lanzou/ ./internal/updatechannel/ ./cmd/lanzou-probe/
$env:RIMARCHO_LIVE_LANZOU_LIST = '1'
go test -count=1 -run '^TestLiveLanzouListing$' -v ./internal/lanzou/
Remove-Item Env:RIMARCHO_LIVE_LANZOU_LIST
go run ./cmd/lanzou-probe -url https://rimarcho.lanzn.com/b01i608qja -password ctot -filename archo-1.0.10-windows-amd64.zip
node scripts/docs-refs/refs.cjs check
```

`RIMARCHO_LIVE_LANZOU=1` retains the opt-in ZIP test too. Both skip by default. The standalone tool prints stages, HTTP statuses and hostnames, then final byte count/hash on success. Its temporary download is deleted before return; it never installs or executes it. Exit 0 means observed ZIP, 1 source/transfer/cleanup failure, 2 invalid arguments. An observation never substitutes for checksum verification. The `archo` command list is unchanged.

| Check | Result |
| --- | --- |
| `go build ./...` | Exit 0, no output. |
| `go vet ./internal/lanzou/ ./internal/updatechannel/ ./cmd/lanzou-probe/` | Exit 0, no output. |
| `go test -count=1 ./internal/lanzou/ ./internal/updatechannel/ ./cmd/lanzou-probe/` | Exit 0: Lanzou `1.023s`, updatechannel `0.969s`, probe `0.284s`; live tests skipped by default. |
| `node scripts/docs-refs/refs.cjs check` | Exit 0: `all references resolve`. |
| Env-gated listing-only test [C] | PASS: `version=1.0.10 checksum_present=false`; folder pages 1/2 only. |
| Single standalone ZIP probe [C] | Expected exit 1 at `developer4.lanrar.com` human verification; output below. |

```text
stage=http host=apifile.woozooo.com status=200
stage=download-node host=developer4.lanrar.com
stage=http host=developer4.lanrar.com status=200
stage=acw-challenge host=developer4.lanrar.com status=200
stage=http host=developer4.lanrar.com status=200
stage=human-verification-required host=developer4.lanrar.com status=200
FAILED: Lanzou requires human verification in a browser; this source failed
last_host=developer4.lanrar.com bytes=unavailable sha256=unavailable
exit status 1
```

Mainland-China end-to-end transfer remains to be tested. Publishing the exact `.sha256` beside the observed share's ZIP is still needed. VM validation of Monitor publication/stamp and integrated browser/staging/rollback behavior has not run.

## 7. Proposed spec text for 17 §5.1 (review only)

Proposed text only; neither the specification nor application is changed:

> **Check (stable Windows, automatic source):** concurrently query GitHub Releases latest, the standalone Archo Monitor item's description stamp through anonymous `GetPublishedFileDetails`, and Lanzou's paginated stable ZIP filenames under one deadline. Record per-source results; choose the highest stable version. Missing/unreachable sources do not suppress other successes. Lanzou checks never request file details/download nodes. Monitor checks never start SteamCMD or read item files. `updates.source github` and prereleases use GitHub only.
>
> **Sources and assets:** configure the standalone Monitor ID separately from retired program item 3835688284. Its description stamp is `RimArcho v<X.Y.Z>`. Assets stay `archo-<version>-windows-amd64.zip` plus `<zip>.sha256`, identical on GitHub Releases and the configured Lanzou folder. Monitor carries no update data file or program ZIP.
>
> **Install (player confirmation required):** obtain the exact checked version's ZIP/checksum pair from GitHub, else Lanzou, else let the player download both in a browser and select them. Never substitute another version. Verify through `selfupdate.VerifyLocal` and preserve steps 1–7, including staging, archive checks and rollback. Missing/invalid checksum, network verification or mismatch fails that source without replacing files. Same-source checksums detect corruption, not a compromised publisher.

The real spec/application retain the legacy Workshop/GitHub flow and prohibit SteamCMD during checks. This proposal likewise uses no SteamCMD during checks, but replaces legacy item/file discovery with description-only Monitor metadata plus concurrent Lanzou listing. Keep Workshop-folder relocation and explicitly confirmed legacy cleanup; do not automatically unsubscribe or delete old item directories. Retire 3835688284 from future update selection, and assign the new Monitor ID only with the actual publication/integration change.

## 8. Considered and dropped

**Removed:** Ed25519, signing keys/key IDs, envelopes, signed manifests, sequence/expiry/freshness machinery, the `.rimarcho/updates/stable.json` proposal and declarative download recipes. No signing/renewal/key-custody recommendation remains. `internal/updatefeed` is removed. Pushed format or package-layout changes couple publication to installed updaters and can force stepping-stone updater releases. The maintainer chose the existing release pair and fixed Go parsing for simplicity; Lanzou parser changes may require a normal RimArcho release.

Remaining integration questions are the public Monitor ID/stamp publication, report presentation, player proxy/mirror routing, mainland-China transfers and VM browser/staging/rollback tests. Update-file publication and cryptographic key custody are not rollout tasks.
