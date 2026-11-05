# Login session management research: LoversLab and SteamCMD

> Date: 2026-10-24. Type: design research; all "recommendations" below are unimplemented. Scope: local specs, code, public sources; no services logged into, no credentials entered or sent, no SteamCMD login or download run.

**A shared settings entry is feasible, but maturity differs: SteamCMD can come first; LoversLab is technically feasible, while site permission for automated downloads and actual compatibility remain unconfirmed.** No design may let RimArcho business code receive, read, save, or forward passwords, verification codes, or 2FA secrets. Session cookies are credentials too and need separate boundaries.

Ship SteamCMD management under "settings → accounts" first. Keep LoversLab on "open download page → player downloads → install from zip", then experiment with isolated WebView2 login after checking site terms and obtaining support. Do not prefer browser-cookie import or promise "log in once, download automatically forever".

## 1. Current implementation and integration points

| Evidence | Current behavior | Design impact |
| --- | --- | --- |
| [11 §1](../specs/11-downloads-and-accounts.md#1-subscriptions-git-download-pages-steamcmd-2026-10-24); [page.go: ReadPage](../../internal/subscribe/page.go) | One HTTPS GET; no cookie jar, no redirects; parses only JSON-LD / Open Graph and repository links | Keep public metadata reading; authenticated download needs a separate entry rather than silently changing reader boundaries |
| [subscribepage.go: SubscribePage](../../internal/app/subscribepage.go) | Git when a repository exists; player supplies release archives with --from, then installs in subscription directory | LoversLab files without a repository still need manual download |
| [11 §3](../specs/09-launch.md); [gamedl.go: SteamCMDAccount, startSteamCMD](../../internal/app/gamedl.go) | Per-call account → configured account → anonymous; passes only account name; separate Windows console; reads progress from SteamCMD's own logs | Reuse account selection and console launch, adding management actions without a password field |
| [subscribeworkshop.go: SubscribeWorkshop, runWorkshopSteamCMD](../../internal/app/subscribeworkshop.go); [deps.go: Finder.Status](../../internal/deps/deps.go) | Workshop tries Steam client first, falls back to SteamCMD (also explicitly selectable); dependency discovery chooses actual executable | Workshop and game downloads must bind to the same actual SteamCMD instance/cache; settings must not find a different copy |
| [02 §1, §2](../specs/01-scope-and-status.md); [uninstall.go: UninstallPlan](../../internal/app/uninstall.go) | SteamCMD may write its own login cache; external tools or directories containing Mods may be retained | Existing uninstall cannot guarantee forgotten sessions; add credential cleanup independent of tool deletion |

Windows GUI already uses WebView2. The main window in [gui.go: Run, BrowserArgs](../../internal/surface/gui/gui.go) has Wails service bindings, a shared user-data directory, and process-saving arguments. Do not navigate it directly to LoversLab or reuse its bindings and launch arguments.

## 2. LoversLab

### 2.1 Login mechanism and facts confirmed here

LoversLab [robots.txt](https://www.loverslab.com/robots.txt) identifies Invision Community. Invision typically maintains identity with session and persistent-login cookies, e.g. ips4_IPSSessionFront, ips4_member_id, ips4_login_key; device cookies may also participate. **These are platform examples, not a verified current LoversLab cookie list; one cookie may not suffice.** Login forms and some actions use session-paired CSRF tokens; an old csrfKey is not a fixed download parameter. Sources: [Invision cookies](https://invisioncommunity.com/cookies/), [template CSRF parameters](https://invisioncommunity.com/4guides/themes-and-customizations/template-syntax/template-plugins-r143/).

Ordinary visits to the public home, terms, cookie information, and file categories returned 403 here; the home response contained Cloudflare "Just a moment" and "Enable JavaScript and cookies". This confirms a challenge in this visit, **not that every 403 means unauthenticated**. Players must handle CAPTCHA/challenges; no automated solving, retry loops, or bypasses. WebView2 compatibility needs testing. Forwarding forum cookies to Go HTTP may fail because challenge state and browser context differ. Sources: [Cloudflare browser/WebView support](https://developers.cloudflare.com/cloudflare-challenges/reference/supported-browsers/), [clearance](https://developers.cloudflare.com/cloudflare-challenges/concepts/clearance/).

Invision supports optional/group-required 2FA, including authenticators. Whether LoversLab currently enables it, and for which accounts, was not confirmed. Let players complete the entire flow on the original site; do not handle verification codes, recovery codes, or TOTP secrets. Source: [Invision account security](https://invisioncommunity.com/4guides/security-and-rules/account-security-two-factor-auth-r316/).

### 2.2 Download requirements

Expected flow: **file page → site's download button → possible declaration / "I agree" and file selection → actual file request / redirect**. Page ID is not a version-specific attachment ID; one page may contain multiple versions/packages. Historical IPS4 integrations include do=download, attachment IDs, confirmation parameters, etc., but do not guarantee today's interface stability. No authenticated flow was entered here, nor was "I agree" verified for every file. Source: [historical Wabbajack IPS4 flow](https://github.com/wabbajack-tools/wabbajack/wiki/Home/1aa44fef735e1996b924ae2c329b225805db1931).

If integration is later approved:

1. Read actual current links from the player-selected file page; open login when needed, without simulating username/password submission.
2. Show original declarations, licenses, or "I agree" for players to click; do not skip with prefilled confirm parameters. Let players select among multiple files rather than assume the first is correct.
3. Request files with that session/current CSRF state. Allow necessary HTTPS redirects but send credentials according to domain, path, Secure, etc.; never forward Cookie/Authorization unchanged to external mirrors or arbitrary domains. Keep temporary signed URLs out of logs, source.json, and share manifests.
4. Download to a managed temporary directory, verify a file response rather than login/challenge HTML, then reuse archive/About.xml validation and installation.

Sites may set per-group counts, waits, concurrency, size, or bandwidth limits; **no authoritative current LoversLab values were found, so do not hardcode "N daily downloads"**. Recommend serial work per service, honoring countdowns and Retry-After, pausing on 429 with retry time. Distinguish permission denial, challenges, and quotas. Never evade limits by changing accounts, proxies, or logging in again.

Existing projects do not prove reliability: [current Wabbajack docs](https://wiki.wabbajack.org/modlist_author_documentation/Meta%20Files.html#manualurl) explicitly support only LoversLab manualURL due to API-download issues; [current AIPS4Source](https://github.com/wabbajack-tools/wabbajack/blob/main/Wabbajack.Downloaders.ManualSources/AIPS4Source.cs) also requires browser download. Historical OAuth/API code is not evidence that RimArcho has API authorization.

### 2.3 Site rules, legality, and adult-site interface

robots.txt was readable: User-Agent: * disallows /login/, /register/, /lostpassword/, /search/, etc., and some do=add/do=submit queries; no blanket /files/ disallow. Avoid programmatic crawling of disallowed paths; distinguish players logging in themselves from automated crawler login. **Not disallowing /files/ does not authorize automated downloads**; robots is neither access authorization nor a license agreement. Sources: [site robots.txt](https://www.loverslab.com/robots.txt), [RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html#section-3).

[Terms](https://www.loverslab.com/terms/) and [guidelines](https://www.loverslab.com/guidelines/) both returned 403, so prohibitions on automated scraping/downloads could not be confirmed. Before shipping authenticated downloads, check current player-visible terms and individual file licenses, asking the site where needed. If bots/download managers are prohibited or a dedicated API required, cookie replay cannot circumvent this. **The conclusion is "permission unconfirmed", neither "confirmed violation" nor "legality guaranteed".**

An acceptable design requires players to use their own accounts, download only authorized content, and follow site rules and author licenses. Login grants no redistribution rights; do not share accounts/cookies, scrape restricted content, or bypass payment/age restrictions. Laws and adult-content access requirements vary by region; evidence here cannot establish worldwide legality.

Disable the entry by default; players opt in. Use neutral names "LoversLab session", "open site login"; first enablement explains adult content and requires meeting the site's access conditions. No adult images or recommendations on the account page; no automatic registration or age confirmation.

### 2.4 Options and password boundary

| Option | Benefits | Drawbacks and choice |
| --- | --- | --- |
| Isolated WebView2; player logs into original site; save necessary session cookies | Login, CAPTCHA, 2FA stay on site; native cookie API can obtain HttpOnly cookies | No Cloudflare compatibility guarantee; isolate web content from local privileges; persistence may involve more than one cookie. Conditional later experiment |
| Player explicitly imports site cookie file | Reuses system-browser login without entering passwords in RimArcho | File equals account credentials; export leaks easily, expires, may not transfer challenge state. Consider only explicitly selected site cookies, no browser-database scanning |
| System-browser login/download, install zip through RimArcho | Passwords and sessions remain in browser; lowest challenge/site-change maintenance | Extra manual import; cannot provide automatic background downloads. Recommended current fallback |

"Never save passwords" is a hard requirement for all three, not a fourth password-login option. No RimArcho password field, autofill, form scraping, keyboard monitoring, login POST-body reading, or password credential storage.

If implemented, use a separate WebView2 environment/profile; disable password autosave and general form autofill, host objects, Wails services, page-to-host messaging, DevTools/remote debugging, recording, and request-body diagnostics. Allow only required site navigation/challenge resources; expose no file/command interfaces. Read necessary site-scoped cookies with native CookieManager, not passwords through document.cookie/DOM scripts. Sources: [WebView2 security](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/security), [password autosave setting](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2settings4), [CookieManager](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2cookiemanager).

**A host can technically run scripts/inspect WebViews: "business code never reads passwords" is enforceable and auditable, but an embedded WebView cannot guarantee "the host has no ability to see passwords".** If absolute invisibility requires that isolation, choose system-browser manual download. If the site later offers supported authorization, prefer system-browser authorization without scraping OAuth login passwords or tokens.

No Chrome/Edge cookie-database decryption or elevated import. Windows Chrome cookies also use App-Bound Encryption; direct reading is fragile and broadens privileges. Do not bypass it even with player consent. Source: [Chrome announcement](https://security.googleblog.com/2024/07/improving-security-of-chrome-cookies-on.html). If manual cookie files are supported later, remind players to delete plaintext originals after import and explain RimArcho cannot erase backups.

### 2.5 Windows session storage, expiry, and logout

**Recommend a current-Windows-user-scoped DPAPI file**, e.g. %LOCALAPPDATA%\RimArcho\accounts\loverslab.session: only necessary cookies and domain, path, Secure, HttpOnly, SameSite, Expires attributes; no passwords. Use user-scoped CryptProtectData, without CRYPTPROTECT_LOCAL_MACHINE, and restrict ACLs. Suitable for cookie collections and easier to exclude from backups than secrets in ordinary config/SQLite/source.json. Source: [DPAPI](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata).

Windows Credential Manager CRED_TYPE_GENERIC also works with a RimArcho-specific target and local-machine persistence, but a single blob is limited to 2560 bytes; a cookie set may exceed it. It does not infer site expiry and requires explicit CredDelete. Choose one store, avoiding duplication. Sources: [CREDENTIALW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/ns-wincred-credentialw), [CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew).

WebView2 also stores cookies, cache, etc. in its user-data directory by default; encrypting one exported copy does not eliminate other copies. To "persist only necessary cookies", use a temporary isolated profile, close browser processes after export, and remove the whole profile; restore only approved cookies next time. Test effects on challenge/device state; fall back to browser download on failure. Source: [WebView2 user-data folders](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder).

Expiry handling:

- Show known local cookie expiry with "the server may invalidate it earlier"; assess relationships between multiple credentials by actual use. Without Expires, show "session / unknown"; invent no fixed lifetime or default permanent cross-launch persistence.
- Settings reads local state offline; validate only when players click "verify" or download, recording last verification time. Saved ≠ currently valid.
- Login redirect / explicit unauthenticated result → "login required", pause service tasks, open player login; challenge → "browser verification required"; 429 → "limit reached". File permission denial does not directly erase the session. Players retry after login; no endless refresh.
- "Log out and forget" disables the session, clears memory, then removes DPAPI file/credential and WebView cookies, storage, cache. If online and supported, log out the current server session; local forgetting does not revoke other devices or copied cookies. Forgetting is irreversible; no recoverable secret backup.

DPAPI/Credential Manager protect at-rest data and user isolation, not malicious programs or agents running as the same user with arbitrary file/process access.

## 3. SteamCMD

### 3.1 Cache, Steam Guard, and "remember password"

Valve states that players enter password and Steam Guard on first SteamCMD login; after exit, account-name-only login can reuse the token in config/config.vdf, which may be updated. Thus "remember password" means **SteamCMD itself stores reusable login credentials**, not RimArcho saving passwords; do not invent a general --remember-password flag. Source: [Steamworks CI/CD login cache](https://partner.steamgames.com/doc/sdk/uploading#Using_SteamPipe_In_A_CI_CD_Environment).

sentry/ssfn are related device-trust files; older workflows commonly retained both them and config.vdf. Device trust is distinct from a password-replacing login token. New versions may not generate ssfn; names/layout are not stable APIs: **absence of ssfn does not prove no cache, and presence does not prove logged in**. Sources: [Codemagic sentry notes](https://docs.codemagic.io/yaml-publishing/steam/), [integration report of ssfn no longer generated](https://github.com/game-ci/steam-deploy/issues/56); the latter shows version differences, not a Valve format guarantee.

RimArcho does not parse/copy token or ssfn contents or migrate them into its credential store. SteamCMD runs as the current user and manages its cache; RimArcho restricts directory permissions, without claiming third-party files are DPAPI-encrypted by RimArcho. Passwords, Guard codes, and phone confirmations occur only in SteamCMD's own window; no RimArcho TOTP/recovery-code handling.

### 3.2 What can be shown and how to log in

Show selected account, actual SteamCMD path/source, cache clues, last success, current task state, with honest evidence strength:

| Status | Evidence | Does not establish |
| --- | --- | --- |
| Anonymous | Resolved effective account is anonymous | Anonymous can download every app/item |
| Account configured, unverified | steamcmd.account or per-call override | Account is logged in |
| Possible cache detected | Managed instance has config/cache files and local success record; no secret contents read | Valid token for this account exists |
| This call logged in as X | Current SteamCMD success matches requested account | Still online after exit; next call needs no Guard |
| Last login succeeded; verify when needed | Previous success record | Cache remains valid forever |
| Login required / waiting for player / network error | Explicit auth failure, interaction wait, or network failure in this call | Network failure means invalid credentials |

RimArcho has no reliable offline "current login status API"; file existence is at most a clue. Do not silently launch SteamCMD to test login from the page: it accesses the network, changes cache, and may prompt for passwords. Expiry is generally unavailable; show "managed by Steam, unknown" and last verification time.

"Log in / log in again" reuses the actual dependency path and working directory, opening a separate native console with only +login <account-name>. Players enter password/Guard directly, then exit when done. The visible Windows window is the purpose of this action. Business code does not take stdin, record a pseudoterminal, or capture screens; convert only allowlisted phases/results into state, without retaining full info/console dumps. Valve permits players to check connection with info in the window; do not send full info to GUI, task history, or MCP.

Workshop and game downloads share "per-call override → settings → anonymous" account resolution. Bind cache identity to actual SteamCMD instance; changing tool path resets "last success" inference. Login, verification, downloads, and cleanup on one instance are mutually exclusive, preventing cleanup of active cache.

### 3.3 Logout and forgetting: two meanings

SteamCMD logout/logoff disconnects and revokes saved login information. Historical "logout does not clear cache" issues exist, but Valve tracking shows successful retesting and closure in 2025; do not present the old fault as inevitable today. Current Windows builds still require testing. Sources: [Valve #7120](https://github.com/ValveSoftware/steam-for-linux/issues/7120), [current command list](https://github.com/dgibbs64/SteamCMD-Commands-List/blob/main/steamcmd_commands.txt).

Recommend "log out and forget this SteamCMD login":

- Have SteamCMD execute logout on a logged-in managed instance, then exit; quit alone or clearing steamcmd.account does not clear cache.
- Offline or after logout failure, allow explicitly labeled "forget locally": stop managed processes, then clean verified authentication-store locations for this instance without reading tokens or backing up auth files. Include config/config.vdf, legacy ssfn/sentry in version acceptance; unknown layouts cannot be claimed cleaned. If needed, reset a dedicated managed runtime directory while retaining separately downloaded games/Mods.
- First version manages one dedicated SteamCMD account; if cleanup affects multiple cached accounts, say so in the preview. Do not guess VDF fields for single-account edits/deletion.
- Local cleanup cannot guarantee revocation of leaked credentials or other devices; direct players to Steam account security / Steam Guard device management for remote revocation.

Externally specified/PATH SteamCMD may be shared; never automatically delete config, ssfn, or userdata. Recommend persistent account management only for a RimArcho-dedicated instance; label external instances "external cache, unmanaged" and offer no button that creates managed login state there. This permits complete forgetting on uninstall without harming other tools.

### 3.4 What anonymous can download

Anonymous can read public RimWorld app/branch information. Ordinary public downloadable **RimWorld Workshop items (AppID 294100) generally support anonymous too**, without Steam client or Steam-owned game. RimSort official docs confirm this usage. This is not universal across games and does not guarantee private, friends-only, delisted, banned, or otherwise restricted items. Use Valve's actual response; do not hardcode "all public Workshop supports anonymous". Source: [RimSort SteamCMD guide](https://rimsort.github.io/RimSort/user-guide/steamcmd-browser).

Branch reading and game download have different permissions. RimWorld itself/old versions require a license; anonymous No subscription should prompt for a licensed account or existing game directory, without bypassing ownership checks. Read actual branch names from Steam; preserve anonymous defaults rather than assuming all anonymous downloads fail. Valve Developer Wiki returned 403, so its unread body was not treated as evidence.

## 4. Shared "settings → accounts"

Accounts are global download-service configuration, not copied per game environment; one selected identity per service suffices initially.

| Service | Name | State and time | Actions |
| --- | --- | --- | --- |
| SteamCMD | Selected account or "anonymous", plus actual tool source | Distinguish configured, cache clues, current success, login needed per §3.2; unknown expiry, last verification | Change account, log in / log in again, explicit verification, switch anonymous, log out and forget |
| LoversLab (after player opt-in) | Site-confirmed display name; "unknown" if unavailable, not member ID as username | Disconnected / saved unverified / usable / login needed / browser verification / quota; known cookie expiry and last verification | Enable, open site login, explicit verification, log out and forget, open download page; cookie import later |
| LoversLab (current phase) | — | "manual browser download" | Open download page, install zip; no unimplemented "connected" state |

Actions follow existing rules:

- **Writes and networking**: [02 §1](../specs/01-scope-and-status.md) requires scoped writes, previews, player initiation, reversibility explanation. List new session files, isolated WebView profiles, credential target names; previews show only service, location category, scope, irreversibility, never cookies/tokens. First enablement explains saved sessions; verification may refresh cache and is not a side-effect-free read. Ordinary settings reads stay offline.
- **Passwords and configuration**: ordinary config/archo.db store only nonsecret account choice/derived state. No secrets in DB backups, exports, packs, source.json, CLI arguments, environment variables, task JSON, errors, or diagnostic bundles. Narrow "include SteamCMD verbatim" rules to allowlisted fields; do not hand raw logs to agents.
- **MCP/agents**: expose only sanitized service/coarse state/player-action information. No tools to read/export/import credentials, enter passwords/Guard, operate login pages, or proxy arbitrary authenticated HTTP. Login, forgetting, site declarations belong to player controls. Authorized fixed download actions may use sessions locally, while agents supply only file-page/item references and receive results, never credentials or arbitrary credential-bearing requests; validate domains/download scope. Keep login windows outside agent DOM/screenshot/debug channels.
- **Isolation limits**: external agents with arbitrary same-user shell/file access are not application-isolated by DPAPI. "Never give credentials to agents" also needs harness restrictions on secret directories, browser profiles, processes, debugging. MCP withholding credentials is necessary, not protection from arbitrary local code.
- **Uninstall**: add a separate "forget account sessions" step before deleting tools/data; do not skip because SteamCMD contains retained Mods. Clean all managed session files, Credential Manager entries, temporary profiles, managed SteamCMD auth caches; retain Mods, games, system-browser login, external-tool caches. Report cleanup failure with retryable state, never "all forgotten". Future session-directory moves must retain one authoritative copy; uninstall covers registered old locations too.

## 5. Implementation order and spec changes

1. **Define boundaries and uninstall before SteamCMD management.** Reuse the dedicated instance found by deps; initially only account name, login window, honest state, logout/local forgetting. Share between Workshop fallback and game downloads; no token reading or multi-account vault. Players must perform actual Windows login in later acceptance.
2. **Keep LoversLab manual for now.** Group entry points in accounts/subscription flow, without saving sessions for unimplemented automatic downloads. First verify current rules, automated-download permission, supported API/authorization. No player credentials are needed for this check.
3. **After permission is clear, test isolated WebView2.** Player login/file selection/consent; verify minimum cookie set, challenge compatibility, encrypted storage, expiry recovery, uninstall. Prefer downloads in the same browser context; consider HTTP handoff only if feasible and permitted. If password isolation or site rules cannot be met, retain manual download.
4. **Consider browser-cookie file import last.** Only explicitly needed, permitted, site-scoped manual files; no browser-database scans/decryption.

Proposed spec changes only; this report does not edit specs:

| Spec | Proposed change |
| --- | --- |
| 02 §1, §2 | Add session/profile/credential-store writes and separate forgetting; extend password ban to all services; Steam tokens remain SteamCMD-only, RimArcho may perform scoped cleanup without reading contents; cover player-initiated auth/verification networking |
| 11 §1 | Keep credential-free ReadPage contract; if LoversLab downloads are approved later, separately define authenticated download, file selection, declarations, limits, expiry, manual fallback |
| 11 §3 | Add login/verification/logout/forgetting, instance-cache binding, state evidence, anonymous/license distinction; replace raw-log return, add real Windows acceptance |
| 16 §2.14 ([current section](../specs/16-interface/2.11-play-review-and-saves.md)) | Merge SteamCMD account into "accounts"; add actions, state, last verification, unknown expiry; neutral opt-in LoversLab; update settings action allowlist |
| 05, 09 | Credential management is player-only; sanitized MCP/built-in Archo state and fixed downloads; forbid credential parameters, login-page observation, general authenticated proxy |
| 17 §2.1, storage/GUI rules | deps still only discovers tools; add session storage, ACLs, backup exclusions, separate WebView environment, debug isolation, cleanup |

Later acceptance focus: first/cached login, Guard/cancel/offline/expiry; anonymous Workshop and unlicensed game failure; tool-path changes do not mix states; old managed cache unusable after logout; sessions forgotten even when SteamCMD is retained for Mods; no secrets in export/diagnostics/MCP. LoversLab additionally: multiple files, declarations, challenges, 429, cookie rotation, temporary-profile cleanup. No authenticated acceptance performed here.

## 6. Evidence-to-conclusion mapping

| Evidence | Conclusion | Proposed path |
| --- | --- | --- |
| E1: local ReadPage, SteamCMDAccount, launcher, uninstall (§1 links) | F1: existing password boundaries reusable; authenticated downloads/uninstall cleanup add work | P1: specs and separate forgetting before account actions |
| E2: robots.txt 200, public-page Cloudflare 403, unreadable terms (§2) | F2: automatic-download permission and authenticated flow unconfirmed | P2: manual browser download until rules verified, then session experiments |
| E3: official Valve cache guidance, #7120 fix record (§3) | F3: account-name-only possible; cache is not online status; cleanup needs version acceptance | P3: native login → derived state → SteamCMD logout / managed local forgetting |
| E4: Microsoft DPAPI, Credential Manager, WebView2 docs (§2.4–2.5) | F4: user-scoped encryption protects persistence, not arbitrary same-user processes | P4: one secret store → profile cleanup → no agent observation/credential access |

Observation date matches report date. E1 is reviewable through linked files/symbols; E2 through ordinary cookie-free GET, noting time/network-dependent responses and that 403 is not terms-body evidence; E3/E4 through source links. This report records only sanitized observations, without saving network responses, credentials, or case files.
