# Signed update feeds through Archo Monitor

> Research and unwired prototype, 2026-11-12 (America/Los_Angeles).
> Live observations were made on 2026-11-13 UTC. **[C]** means confirmed by a live HTTP or browser observation; maintainer-supplied browser evidence is attributed explicitly. Source inspection and design proposals are not [C].
> Basis: [17 §5 and §5.1](../specs/17-engineering.md#5-releases), [earlier Workshop research](2026-11-05_workshop-distribution.md).

## 1. Decisions and readiness

1. Use one Ed25519-signed file as the authority for a stable Windows update's version, exact archive name, byte length, SHA-256 and ordered download recipes. Workshop, GitHub and future object storage carry identical bytes. Transport metadata and netdisk filenames never establish authenticity.
2. Put it at **`.rimarcho/updates/stable.json` inside the new standalone Monitor item**. Keep the program ZIP off Workshop. The new item ID remains unassigned; this investigation publishes nothing.
3. Keep recipes declarative: HTTP GET / form POST, one-capture RE2 extraction, JSON pointers, exact filename selection, URL resolution, literal substitution and equality assertions. No JavaScript, shell, general expression evaluator, recursion or general loops. Fixed cookie handling and folder pagination are named native operations with bounded retries/pages.
4. Preserve existing ZIP naming and extraction/swap rules. A signed digest replaces an unsigned downloaded checksum as the authority for stable-feed downloads. The prototype returns a verified temporary archive and never installs it.
5. Respect signed source order in `auto`: **GitHub first, Lanzou second**, with OSS/COS possibly later. Keep GitHub's DNS/connect deadline near five seconds so unreachable GitHub falls through promptly. Contact Lanzou only after a player confirms installation and GitHub fails. Version checks read the signed feed from Monitor / GitHub and never contact Lanzou. Do not infer reachability from language. `github` must bypass Steam and netdisks. Prereleases and Linux remain on existing GitHub paths pending a separate integration decision.
6. Match RimCrow's Lanzou boundary: compute the fixed `acw_sc__v2` cookie that a browser computes, follow allowed redirects, and stop when a node returns non-challenge HTML or challenges the same host twice. The node's `down_r(el)` human-verification page is a **source failure**; never POST its `file`/`el`/`sign` form to `ajax.php`.
7. If Lanzou also fails, offer the share link/password for a browser download and let the player select the ZIP. Require the exact signed size and SHA-256 before the existing installation steps. The library check exists; application, CLI and UI integration are design only (§4.2).
8. **Lanzou cannot yet be the only China-accessible download source. [C]** This machine reaches network-verification HTML even in Chromium, whereas the maintainer downloaded directly in a desktop browser outside China. A final byte host, ZIP size and SHA-256 remain unconfirmed on this network. The standalone probe reports this failure explicitly.

The experiment lives in [internal/updatefeed](../../internal/updatefeed/feed.go), [cmd/lanzou-probe](../../cmd/lanzou-probe/main.go) and this research note/index. It remains separate from the self-update behavior specified by 17 §5.1; that specification describes the existing application path, not this prototype. No application, frontend, existing updater, specifications, changelog or release pipeline behavior changes. No production key, built-in public key, upload, push or publishing step is included.

## 2. Live Lanzou observations

### 2.1 Scope and evidence

The maintainer supplied `https://rimarcho.lanzn.com/b01i608qja`, password `ctot`, for live HTTP testing. Requests were serial, with fresh cookie jars in multiple independent attempts. No Steam credentials, browser profile, real game, Mods or RimArcho data directory were accessed. No concurrent downloads or rate-limit stress tests were attempted. Bodies were bounded; no ZIP was executed or installed. Bearer query values and cookies are omitted from this note.

| Evidence | Observation | Finding and next action |
| --- | --- | --- |
| E1 [C] | Fresh folder GET: HTTP 200, ESA headers, gzip body with `arg1` and `acw_sc__v2`; `X-Tengine-Error: denied by http_custom` | HTTP 200 is insufficient. Bound decompression, recognize the fixed challenge, retry once. |
| E2 [C] | Permutation/XOR cookie followed by another GET returned the password form and folder AJAX declaration | The fixed native strategy works for the observed folder. Do not evaluate its obfuscated script. |
| E3 [C] | Password POST returned `zt:1` and one file, `archo-1.0.10-windows-amd64.zip`, displayed size `33.9 M` | Select the signed exact filename; rounded size cannot authorize installation. |
| E4 [C] | The full file ID, including `webpage` query, led to `/fn?...` iframe and an external AJAX API | Retain query state. A bare file ID did not yield the same iframe in a fresh attempt. |
| E5 [C] | Download AJAX returned `zt:1`, `dom:https://developer4.lanrar.com` and an opaque query token | This is a resolver URL, not a confirmed file URL. |
| E6 [C] | Node GET returned another cookie challenge, then HTML network verification | Host-specific challenge state is needed. Reject HTML where a ZIP is required. |
| E8 [C] | Fresh Go session, consistent Chrome User-Agent, no download Referer: still network-verification HTML after the cookie challenge | Matching the anchor's `rel=noreferrer` behavior did not remove the blocker. |
| E9 [C], maintainer-supplied live evidence, 2026-11-13 | The maintainer's desktop browser outside China downloaded the file directly, without a verification page | The page is conditional; outside-China location alone does not imply failure. No size/hash or final byte host was supplied. |
| E10 [C], maintainer-supplied live evidence, 2026-11-13 | A real Chromium pane on this machine followed password → file page → iframe link and received the verification page from `developer4.lanrar.com`; the anchor had `rel=noreferrer`, `target=_blank`, and the page also loaded `/file/kdns.js` | Go is not the cause by itself. The behavior depends on network/browser reputation; the exact signal/trigger is unknown. Do not automate the human check. |
| E11 [C], follow-up standalone probe | Folder GET/cookie retry, listing pages 1 and 2 (empty), file page, iframe and download API succeeded; node GET/cookie retry returned human-verification HTML | Exit 1: `Lanzou requires human verification in a browser; this source failed`. Last host `developer4.lanrar.com`; ZIP bytes/hash unavailable. |

Reproduce E1 (cookies/tokens vary):

```powershell
curl.exe --compressed --max-time 25 --max-filesize 1048576 -sS -D - https://rimarcho.lanzn.com/b01i608qja
```

The standalone probe and opt-in test in §6 reproduce the implemented chain and current failure without logging bearer tokens. The supported path ends at the human-verification page. Exploratory verification submissions are excluded from the supported path and code.

### 2.2 Requests and extraction

| Stage | Request / fields | Result |
| --- | --- | --- |
| Folder | GET supplied URL, then one challenge retry | [C] Valid folder HTML. |
| Listing | POST `/filemoreajax.php?file=14238640`; `lx=2`, `fid=14238640`, `uid=5546482`, page-derived `puid`, `pg=1,2,...`, `rep=0`, page-derived `t` and `k`, `up=1`, `ls=1`, `pwd=ctot` | [C] Page 1 returned one ZIP; page 2 was empty in the follow-up probe. Larger folders are covered synthetically. |
| File | GET `/` plus selected entry's full `id` | [C] File page with `<iframe class="ifr2" ... src="/fn?...">`. |
| Iframe | GET iframe URL, Referer file page | [C] `domain1`, `domain2`, `ajaxdata`, `wp_sign` and a `downprocess` declaration. |
| Download API | POST `https://apifile.woozooo.com/ajaxfile.php?file=330523359`; `action=downprocess`, `websignkey=ajaxdata`, `signs=ajaxdata`, `sign=wp_sign`, `websign=2`, `kd=1`, `ves=1` | [C] JSON `dom` and `url`; combine as `dom + "/file/" + url`. |
| Resolver node | GET combined URL, one host-specific challenge retry | [C] Network-verification HTML on `developer4.lanrar.com`; no successful byte-serving redirect observed. |

[lanzou.go](../../internal/updatefeed/lanzou.go), `LanzouSource`, contains the exact extraction expressions shared by the standalone probe and [live_test.go](../../internal/updatefeed/live_test.go): literal folder fields, a ten-digit `t`, 32-hex-character `k`, iframe `src`, and literal `domain1`, `ajaxdata`, `wp_sign`. These are dated patterns, not a JavaScript parser. Folder variable names changed across requests; fixed variable names alone would be insufficient. A signed recipe update can change the patterns without an executable release.

[C] Returned iframe HTML also advertised `https://apifile.lanzouw.com` and a script-controlled fallback `https://developer2oss.lanzouc.com:661` when its DNS helper is absent. Neither alternative was requested or confirmed as a working route. Port 661 is rejected by the prototype; supporting it needs an explicit policy decision.

[C] Both short `Mozilla/5.0` and full Chrome User-Agents worked through metadata resolution. Successful AJAX requests included Referer, Origin and X-Requested-With; a separate download-API POST succeeded without a preexisting cookie jar. A complete header-omission matrix was not run, so necessity/optionality at every stage is unknown. The supplied password was accepted.

[C] ESA set `acw_tc` and `cdn_sec_tc` with `Max-Age=3600`. The returned challenge script specified one hour for its own cookie. Its fixed transformation permutes the 40 hex characters of `arg1`, then XORs the resulting 20 bytes with an embedded constant. `acwCookie` implements that bounded transformation independently from the observed page, without evaluating surrounding JavaScript. Cookie lifetimes do not establish download-token lifetime.

### 2.3 Stability, expiry and throttling

[C] Folder signing values, iframe parameters and download queries changed across fresh sessions. Resolve again on each download; do not persist bearer URLs. Reusing an earlier API signature later returned `zt:0`, but headers/state also differed: this is not a controlled TTL measurement.

The iframe contains a 1,800,000 ms timeout display. That is a UI timer, **not a confirmed server expiry guarantee**. No working direct URL was available for delayed reuse. Final-host stability, token lifetime, speed and range support remain unknown.

No HTTP 429 was observed. E9–E10 show that the network/browser reputation context affects the verification page, including in a real browser; the exact trigger is unknown. No rate ceiling is claimed. Fail a source once and use the next signed source inside the aggregate deadline. After Lanzou failure, offer the manual fallback in §4.2. Do not automate an interactive CAPTCHA or loop verification attempts.

### 2.4 RimCrow comparison (read only)

Read `../../../../RimModManager/backend/managers/mgr_update.py`, class `LanzouSource`, and delegated `backend/utils/lanzou_parser.py`; no code was copied.

RimCrow infers latest version from supported ZIP filenames and sets `file_hash=None`. Its direct session disables environment/application proxy routing. The parser uses a browser User-Agent, restricted JavaScript literal/alias/concatenation extraction, the known cookie challenge, folder pagination, iframe resolution and up to eight download redirects, rejecting unexpected HTML.

Here the signed feed supplies version, exact name, size and digest; the netdisk only supplies those bytes. Both clients paginate folders and compute the fixed cookie without running arbitrary page scripts. RimCrow bounds pagination at 100 pages and also stops on a short page; this prototype scans until an empty page/status 2, capped at 16 pages inside its shared request/body/deadline budgets. It selects the exact signed filename, never the netdisk's inferred latest release. The implementations were written independently.

RimCrow's `_resolve_download_url` stops with verification failure when HTML is not an acw challenge or the same host challenges twice. It **also stops at the `down_r` human-verification page**. The prototype keeps that same boundary. Reading RimCrow's code establishes this behavior, not a successful live transfer from this host.

## 3. Feed contract

### 3.1 Envelope, signature and freshness

Implemented types: `Envelope`, `Manifest`, `Artifact`, `Source`, `Step` and authenticated opaque `Feed` in [feed.go](../../internal/updatefeed/feed.go). The envelope has three fields:

| Field | Meaning |
| --- | --- |
| `key_id` | Identifier selecting an executable-trusted public key, never a URL or downloadable key. |
| `payload` | Padded standard base64 of exact UTF-8 manifest JSON bytes. |
| `signature` | Padded standard base64 of the 64-byte Ed25519 signature. |

The signed message is ASCII `RimArcho update feed v1\n`, followed by `key_id`, a newline, then decoded payload bytes. Here `\n` denotes a single newline byte. Signing binds the key selector and exact serialization; JSON canonicalization is unnecessary. Reformatting the payload requires signing again. Duplicate JSON keys (including case collisions), invalid UTF-8, unknown fields and trailing JSON are rejected. Verification happens before interpreting payload fields.

| Manifest field | Implemented contract |
| --- | --- |
| `format` | Integer 1. Unknown versions fail closed; incompatible schema/vocabulary changes require a new format and executable support. |
| `sequence` | Positive monotonically increasing unsigned integer, including same-version recipe repairs. `Parse` accepts a caller-supplied minimum; persistence is future integration. |
| `issued`, `expires` | RFC 3339 dates. Implemented validity cap: 90 days. Reject expired, nonpositive validity, zero issue date or issue more than five minutes ahead. Publication-policy recommendation: 90–180 days (§7); use 90 with this validator, and revise/test the cap explicitly before accepting 180. |
| `version` | Stable `X.Y.Z`, no leading-zero components, prefix, build metadata or prerelease. |
| `artifact` | `name`, positive integer `size` in bytes, 64 lowercase hex `sha256`. |
| `sources` | One to four uniquely named ordered entries. |

Future integration must atomically persist the highest accepted sequence **and payload digest** with authenticated bytes. Reject lower sequences and a different payload at an already accepted sequence. The prototype enforces only the supplied sequence floor; persistence and same-sequence equivocation detection are not implemented. Distribute a rotation public key in an executable before using its key ID; a feed cannot add trusted keys. Compromise recovery needs a new executable/distribution path and revocation procedure.

Signatures prevent tampering, not withholding. Dead items and stale mirrors can freeze a client until expiry. Cold installs have no high-water mark; expiry and a trustworthy clock are the remaining freeze controls. An expired cached feed may inform a “last known version” display but cannot authorize downloads. Re-parse/reverify cached bytes at installation time; never restore recipes from unsigned application state.

### 3.2 Archive identity

V1 targets Windows amd64: name must be exactly `archo-<version>-windows-amd64.zip`, matching `selfupdate.ZipName`. Signed size is mandatory and at most 200 MiB, measured in actual bytes. Content-Length, when present, must agree too. Every source must deliver the same size/hash; its own checksum cannot substitute a different artifact.

Keep publishing `<zip>.sha256` for legacy clients. `Feed.VerifyLocal(ctx, path)` constructs the sha256sum line from authenticated metadata and delegates to `selfupdate.VerifyLocal` with a positive signed size. An unsigned sidecar never replaces the signed digest. Recheck cached ZIP bytes against the signed size/hash.

Then keep `selfupdate.Extract` and existing ZIP-root, path, symlink, executable, per-entry and expansion-total checks, followed by lock/wait/journal/swap/startup verification/rollback from 17 §5.1. A digest is not a ZIP layout check. The prototype does not extract, execute, replace installed files or persist a release.

### 3.3 Recipe operations

`Source` has `name`, exact `hosts`, optional `user_agent`, optional `challenge:"acw-v2"`, ordered `steps`, and final `download` template. Names/destinations are identifiers. `${artifact}` is the signed archive name. Substitution reads only recipe variables, never environment, files or application state.

| `op` | Inputs and result |
| --- | --- |
| `request` | GET or POST `method`, `url`, optional allowlisted `headers` and `form`. Decoded body goes to `into`; response URL goes to `<into>_url`. Forms are URL-encoded after substitution. |
| `lanzou-list` | Fixed POST pagination operation, with the same URL/headers/form inputs and initial `pg=1`. Set `pg=1,2,...`; accept `zt=1` and the `text` array, stop at empty array or `zt=2`. Return the sole exact `name_all == artifact` entry's full `id` into `into`. Fail on malformed/failed pages, conflicting matches, no progress, or budget exhaustion. |
| `regex` | `input` names a variable; static RE2 `pattern` has exactly one capture, stored in `into`. No match fails. |
| `json` | JSON variable `input`, JSON pointer `path`; extract a scalar to `into`. |
| `select` | Array at `path`; exactly one object must have field `match` equal to expanded `equal`. Extract scalar `field`. Use the signed exact filename. |
| `url` | Resolve expanded `value` against expanded `base`; store in `into`. Recheck destination policy before requesting it. |
| `value` | Store expanded `value`, including bounded concatenation. |
| `assert` | Require expanded `input` equal expanded `equal`; store the matched value. |

For regex/json/select, `input` is a variable **name**, so large pages need not pass through small-value substitution. Other templates use `${name}`. Substitution is single-pass: extracted text cannot introduce recursive template evaluation. Undefined variables, unsupported operations/headers, malformed JSON and ambiguous selections fail the source.

The fixed `acw-v2` strategy handles one GET challenge retry, and the download stage allows one retry per host, inside shared budgets. It cannot solve arbitrary new challenges; a changed transformation requires an executable update. Folder pagination preserves query state in the selected ID and scans through the empty page, including after a match, to reject later ambiguity or incomplete listings. It never automates a node's human check. Folder contents never determine latest version.

### 3.4 Bounds and destination policy

Implementation: [network.go](../../internal/updatefeed/network.go), [recipe.go](../../internal/updatefeed/recipe.go), [download.go](../../internal/updatefeed/download.go).

| Resource | Limit |
| --- | --- |
| Envelope / decoded manifest | 96 KiB / 64 KiB |
| JSON nesting | 32 |
| Sources / hosts per source / steps per source | 4 / 8 / 48 |
| HTTP exchanges / redirects per source | 24 / 6, shared by recipe, challenges and download |
| Folder pagination | 16 pages including the terminating empty page, also inside shared budgets |
| Metadata body | 1 MiB wire-read cap and decoded cap; 4 MiB total decoded metadata |
| Extracted value / substitution / URL | 8 KiB |
| Regex / serialized step | 512 bytes / 16 KiB |
| Form fields / recipe headers | 16 / 6; encoded form at most 16 KiB |
| Request / response headers | 16 KiB each |
| Cookies | Fresh host-only state, 16 cookies and 8 KiB serialized state |
| Default User-Agent | 256 bytes |
| DNS plus connect / TLS / response headers | 5 s / 5 s / 10 s |
| Individual metadata request / full recipe | 20 s / 60 s, including body reads |
| Download / full source | 3 min / 4 min absolute deadlines |
| ZIP | Exact signed length, at most 200 MiB; read at most signed length + 1 |

Every initial URL, extracted target and redirect must use HTTPS port 443, have no userinfo/fragment, and match an exact signed hostname. No wildcards or arbitrary ports. Validate **all** DNS answers as public before dialing a checked literal IP, retaining original hostname for TLS. Reject private, loopback, link-local, CGNAT, unspecified, multicast, documentation/benchmark ranges and transition routes. Check DNS on connection establishment to prevent rebinding. POST redirects are rejected; cross-host GET redirects discard recipe headers and keep cookies host-local.

Only User-Agent, Referer, Origin, X-Requested-With, Accept and Accept-Language can be set. No inherited Authorization, caller cookie jar, arbitrary Cookie header, file URL or application secret. HTTP 429/5xx fails without automatic source retries. Absolute deadlines bound slow bodies and headers; bounded decompression handles gzip bombs.

The experiment clones only `*http.Transport`, overrides dialing, disables proxies and creates fresh cookies. Wrapped/custom RoundTrippers are rejected. **It does not yet interoperate with the application's installed `netcfg` wrapper.** The passed `http.Client` supplies a trusted standard transport/TLS configuration, not inherited routing/cookies/redirect/timeout policy.

A hostile recipe can still send small requests to allowed public services and consume its bounded request/time/disk budgets. A valid archive hash does not prevent those effects; a stolen signing key can also authorize malicious binaries. The hash is not a second independent trust anchor. Integration needs caller cancellation and an aggregate fallback deadline (proposed five minutes across sources). Tests use an unexported exact-origin exception for httptest; signed data and the public API cannot enable it.

## 4. Future integration decisions

### 4.1 Discovery and freshness

Search `env.WorkshopFolders` for the **new configured Monitor item ID**, then read only `.rimarcho/updates/stable.json`. Bound reads and reject symlinks/reparse escapes; do not search arbitrary Mods or trust PublishedFileId.txt as authorization. A copied Monitor can have a stale feed; verify its signature and dates too. Discovery/link checks are design only.

`internal/scan/fingerprint.go:skipDir` excludes `.rimarcho` entirely, avoiding snapshot/semantic churn. This directory has no About.xml and creates no separate Mod candidate. JSON outside the game's standard content directories is outside its loaders; VM validation of the published Monitor layout is still needed. Do not use semantic `About/` or overwrite `.rimarcho/origin.json`. RimArcho only reads published metadata, consistent with not writing markers into subscribed folders.

Proposed `auto` acquisition order: subscribed Monitor file, then an **already installed** anonymous SteamCMD using existing `steamcmd-self` and shared locks, then identical GitHub-hosted bytes. Invalid, missing or expired metadata falls through. Never use the configured named account, install SteamCMD here, or register a Mod source. Confirm both item result and expected feed; clean ACF/content together under the lock.

**Maintainer decision needed:** today's check never starts SteamCMD or downloads Workshop content (17 §5.1). Fetching even a small Monitor item changes that contract and may involve SteamCMD runtime updates. Proposed policy allows a bounded metadata refresh only with existing SteamCMD, as background maintenance or explicit check, after documenting disk/network effects. Do not block a UI thread. This investigation changes no current check behavior.

A stale subscribed copy must not indefinitely suppress network checks. Use it for immediate “last known” results, then refresh accessible metadata on the check interval, including a short-deadline GitHub-feed attempt. Compare authenticated sequences, retain the highest, and label expired metadata stale rather than “up to date.” This refines strict local-else-SteamCMD-else-GitHub: a valid local file is useful evidence, not proof Steam can still update it. Decide the periodic anonymous refresh policy when desktop Steam is offline.

### 4.2 Settings, channels and offline use

- `updates.source auto`: discovery above, then **after installation confirmation**, signed source order from §1: GitHub, Lanzou, possibly OSS/COS later. The existing five-second DNS/connect budget bounds an unreachable GitHub connection attempt; TLS and response deadlines are separate. Lanzou is never contacted by version checks. Locale alone does not reorder sources. Same-version recipe repairs increment sequence without a repeated new-version notification.
- `updates.source github`: bypass Monitor and SteamCMD; fetch stable signed metadata from a fixed GitHub URL and use only GitHub source(s). Also attach identical bytes to releases for exact-version recovery. A signature failure must never fall back to an unsigned sidecar for that stable update.
- A fallback retrieves the **same checked version/artifact**. A newer feed during installation requires a new check/confirmation, not silent substitution.
- Prereleases today use the GitHub release list. V1 accepts stable only; preserving the existing prerelease path retains its unsigned-sidecar trust, outside the stable-feed guarantee. Before claiming every update is signed, add a separately signed GitHub-only prerelease manifest/channel and tests. Linux stays GitHub notification-only under the current specification.
- Offline: retain last known version/notes/source and authenticated bytes; report unavailable/stale checks. Install only from a still-valid authenticated feed and exact size/hash-matching ZIP, after existing confirmation, and never downgrade the running version. No expired recipe execution. App comparison/persistence are not implemented here.

**Manual fallback (design; library check implemented, surfaces unwired):** after GitHub and Lanzou fail, show the public Lanzou share URL and password, with an action to open the share in the player's browser and an action to select the downloaded ZIP. A player can complete any human check there. Never display or persist a resolved bearer URL. The share link/password come from the authenticated source configuration; only the signed artifact identity authorizes the bytes.

Proposed CLI shape: `archo update --from <zip>`. It would use the checked, still-valid signed feed (re-authenticate cached bytes and enforce persisted sequence/version rules), rather than infer a version from the local name or trust a nearby sidecar. `--from` is **not implemented** and does not add a command/option to the public CLI in this round.

`Feed.VerifyLocal` checks the selected file through `selfupdate.VerifyLocal`, allowing a browser-renamed file only when its contents match the signed size/hash. Future installation acquires the existing update lock (17 §5.1 step 1), copies the input into private `download/` staging, verifies that staged copy again (the selected input can change), then follows steps 3–7: extraction/layout checks, process wait, journaled swap, startup verification/rollback and restart. The local input remains untouched. Missing/expired feed, wrong size/hash, cancellation or failed layout checks authorize no replacement; browsing or selecting a ZIP alone is not installation consent.

`netcfg` applies system/configured proxies and GitHub mirrors to normal HTTP. Its wrapper is deliberately rejected by this experiment. A future adapter must distinguish user-trusted proxy endpoints (possibly loopback) from recipe targets, validate logical destinations at every hop, and decide how to trust remote proxy DNS. Adding a mirror/proxy outside the guarded transport can bypass host/SSRF policy. An integrated release must not silently ignore player routing settings; direct-only is an experiment limit.

### 4.3 Retiring the old item

A later integration should replace `selfupdate.WorkshopItem=3835688284`, description/tag version parsing and GetPublishedFileDetails in the stable check with Monitor-feed discovery. Stop treating the removed item's ZIPs as an update authority. Keep generic Workshop-folder relocation and already-confirmed legacy cleanup for extracted programs; those protect existing players independently. Never automatically unsubscribe or delete their item folders.

Assign the new ID after a standalone Monitor item exists. Use a new Monitor-ID VM override rather than accidentally retaining the removed program ID; update `monitor/About/PublishedFileId.txt` in that publishing change. Old clients need a bridge executable from GitHub or manual download before they know the key/channel. Do not put a program ZIP in the new Monitor item to bridge them.

The supplied removal/result-9 background was not re-probed here. Code confirms `5af77ab9`: when Steam cannot confirm the item, a subscribed folder is compared with GitHub and the newer wins; offline the folder result remains. This matches current 17 §5.1. No discrepancy was found there. SteamCMD-on-check and signed authority are proposed changes, not current behavior.

### 4.4 Release signing and staging

Extend the release workflow later:

1. Produce the exact ZIP and existing sidecar normally. Its bundled Monitor must **not contain the feed naming that ZIP**: otherwise the ZIP hash depends on a file containing its hash.
2. Measure exact size/SHA-256 and validate ZIP layout. Publish immutable ZIP bytes to sources using maintainer-controlled steps; re-download mirrors and compare before advertising them.
3. Build a payload with new sequence, validity and tested recipes. A same-version recipe repair retains artifact name/size/hash and increments sequence. Review hosts/forms before signing.
4. Invoke a small offline Go Ed25519 helper. Read the private key from an explicitly supplied external/removable file or protected signing device, never the repo, command-line literal or CI log. Serialize once, sign §3.1's exact message, verify against the public key, then stage. Tests generate throwaway keys in memory.
5. Copy standalone Monitor to a **separate staging folder** and add `.rimarcho/updates/stable.json`, with no program ZIP/launcher. Stage identical feed bytes as GitHub release asset and fixed stable-feed object. Never mutate the finished ZIP.
6. Publish manually through the established maintainer-only path. `scripts/release.ps1` continues to stage, not push/publish. Report partial publication if Monitor upload fails after GitHub succeeds; never generate different bytes with the same sequence.

Signing/staging is design only; no scripts or real keys changed here. Store public keys/recipes in the repo, not private signing material. Monitor modVersion remains independent of launcher version/feed sequence.

## 5. Prototype coverage and limits

`Parse(reader, trustedKeys, now, minimumSequence)` returns a `Feed` with private manifest, preventing accidental mutation of verified recipe maps/slices. Accessors expose small values/copies. `Feed.Download(ctx, client, sourceIndex, quarantineDir)` runs one source and returns `Archive` only after signed size/hash verification. Read/Seek expose verified bytes; Close deletes the private temporary file. Failed transfers remove it without exposing an archive. Temporary names are random and created exclusively in caller-provided staging.

`Feed.VerifyLocal(ctx, path)` supplies the cheap manual-fallback verification in §4.2. `LanzouSource(shareURL, password)` builds the dated recipe shared by the probe and live test. `ProbeLanzou` runs it without a signed feed, downloads at most 200 MiB into an OS temporary directory, checks the ZIP prefix, measures size/SHA-256, closes/deletes the file and removes that directory before returning. Its observations never authorize installation. On failure it reports the last contacted host and leaves byte/hash values unavailable. Redirect destinations still require exact allowed hosts and public HTTPS; a tester can supply additional explicit hosts, never wildcards. No host is learned and automatically trusted from a response.

Callers own fallback, aggregate deadline, acquisition, key pinning, sequence persistence, current-version comparison, directory permissions and extraction. None of those app integrations is included. Standard library only. Stable-only schema, standard-transport restriction and fixed challenge transformation are experiment limits.

Synthetic tests use generated ZIPs/keys, httptest and t.TempDir. Coverage includes signature/key/freshness/sequence/schema/duplicate-key failures; exact artifact identity; form/regex/JSON/selection/relative URLs; cookies/challenge; redirect/request budgets; public-address policy; mixed DNS-answer rejection and literal-IP pinning; gzip limits; cancellation; size/hash mismatch; and quarantine cleanup. Follow-up tests cover later-page selection, empty/status-2 termination, later failures/ambiguity, no-progress/page caps, the shared Lanzou recipe through a redirected synthetic ZIP, local-file verification, human-page/repeated-challenge termination without verification POSTs, and probe output/argument redaction. No real game/Mod/app paths are used.

The opt-in live test first observes a real ZIP's size/hash, signs metadata with a throwaway key, reruns from a fresh state and independently hashes the verified result. This tests repeatability and implementation consistency, **not upstream authenticity**. `RIMARCHO_LIVE_LANZOU_SHA256` optionally supplies an independent expected digest. A self-observed SHA-256 cannot prove an official release. Here the probe failed before observing ZIP bytes, at network-verification HTML.

## 6. Validation and reproduction

```powershell
go build ./...
go vet ./internal/updatefeed/ ./cmd/lanzou-probe/
go test -count=1 ./internal/updatefeed/ ./cmd/lanzou-probe/
go run ./cmd/lanzou-probe -url https://rimarcho.lanzn.com/b01i608qja -password ctot -filename archo-1.0.10-windows-amd64.zip
$env:RIMARCHO_LIVE_LANZOU = '1'
go test -count=1 -run '^TestLiveLanzou$' -v ./internal/updatefeed/
Remove-Item Env:RIMARCHO_LIVE_LANZOU
node scripts/docs-refs/refs.cjs check
```

The fixture selects 1.0.10; use `RIMARCHO_LIVE_LANZOU_VERSION` for another exact version. It never auto-selects untrusted latest. Default tests skip external traffic. The standalone tool prints completed recipe stages, per-request HTTP status and hostname, fixed cookie retries, the download node and the observed final host/size/hash on success. Paths, query tokens, password and cookies are never printed. `-hosts host1,host2` allows a tester to add independently reviewed exact byte hosts if the resolver changes; an unknown host otherwise fails closed. The tool's exit codes are 0 for an observed ZIP, 1 for source/transfer/cleanup failure and 2 for invalid arguments. `go run` additionally prints `exit status 1` for failure. It never installs or executes downloads, and it is separate from `archo`'s command list.

Live downloads, if available, use OS temporary storage (the probe) or t.TempDir (the signed repeat), and never install. No VM game/Workshop validation ran: this package is unwired.

| Check | Result / output |
| --- | --- |
| `go build ./...` | Exit 0, no output. |
| `go vet ./internal/updatefeed/ ./cmd/lanzou-probe/` | Exit 0, no output. |
| `go test -count=1 ./internal/updatefeed/ ./cmd/lanzou-probe/` | Exit 0: updatefeed `0.933s`, lanzou-probe `0.277s`; live test skipped by default. |
| Standalone `lanzou-probe` [C] | Exit 1, expected on this network: folder pages 1/2, file, iframe and API succeeded; `developer4.lanrar.com` returned HTTP 200 cookie challenge, then HTTP 200 human-verification HTML. Output below. |
| Opt-in `TestLiveLanzou` (previous round) | Exit 1: resolved node `developer4.lanrar.com`, then network-verification HTML; no ZIP/hash assertion reached. The gate is retained and now shares the standalone probe's resolver. |
| `node scripts/docs-refs/refs.cjs check` | Exit 0: `all references resolve`. |

The live failure is a material research result, not a passing end-to-end test. A successful final transfer and token-lifetime/rate-limit measurements remain outstanding.

Follow-up live probe output after metadata resolution (no bearer values):

```text
stage=download-node host=developer4.lanrar.com
stage=http host=developer4.lanrar.com status=200
stage=acw-challenge host=developer4.lanrar.com status=200
stage=http host=developer4.lanrar.com status=200
stage=human-verification-required host=developer4.lanrar.com status=200
FAILED: Lanzou requires human verification in a browser; this source failed
last_host=developer4.lanrar.com bytes=unavailable sha256=unavailable
exit status 1
```

## 7. Maintainer questions before rollout

1. Can the intended mainland-China network download this share without the verification failure? Run the standalone probe there. A working trace or alternative mirror must establish the final byte host, TLS/port policy, size and independent digest. Which OSS/COS provider, region, domain, cost/bandwidth policy and release-staging workflow should supply an additional source? Keep GitHub first and Lanzou second pending that decision.
2. Is background anonymous SteamCMD metadata refresh acceptable despite the no-SteamCMD-on-check rule? How often should a possibly frozen subscribed item refresh?
3. Which new public Monitor ID and fixed GitHub stable-feed location will be used? Confirm publication accepts the JSON and continues updating subscribers; policy acceptance is not assumed.
4. Who holds the offline key, renewal calendar and backup/revocation procedure? Recommend **90–180 days** of validity rather than 30: releases and manual/offline signing may be infrequent, Workshop refresh can lag, and players may return after weeks offline. A monthly renewal can disable installation of an otherwise valid release without any new code being needed. Longer validity also extends the possible withholding/freeze window, so retain sequence high-water marks, refresh checks and planned renewal. Start at 90 days under the implemented cap; accepting 180 requires an explicit validator/test change. Missed renewal still disables feed-authorized installation, including manual ZIPs.
5. Should first integration include signed prereleases and trusted proxy/mirror support? Both are needed before extending the stable/direct guarantee to all update paths.
6. VM-check that `.rimarcho` data is ignored by RimWorld, survives publication, avoids scan identity churn and is staged without a circular ZIP/hash dependency. Test anonymous refresh/cleanup only there.
