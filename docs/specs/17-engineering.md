# 17 · Technology and engineering

> Status: implemented and in use in 1.0. Technology, code structure, data locations, tests and releases (with self-update) are as described; §6 records game behavior confirmed by decompilation, §7 engineering notes.
> Basis: 00, 00a; 02 §1 (write allowlist); 16 §8–§10 (GUI and TUI technology); 18 (measured baselines).

## 1. Technology

| Purpose | Choice | Reason / notes |
| --- | --- | --- |
| Language | Go 1.27.2, `CGO_ENABLED=0` | Single exe, controlled memory use, simple concurrency; no C toolchain, direct cross-compilation |
| TUI | Bubble Tea v1.3.10 + Lip Gloss (v1.1.1 pseudo-version) + termenv | Rendering lists of over 500 rows and Chinese character widths verified (18); no Bubbles |
| GUI | Wails v3.0.0-beta.28 + WebView2; plain HTML / CSS / native JS (ES modules), embedded in the exe with `go:embed` | No framework, npm or build step; no `wails3` binding generation; only `go build` needed (16 §8) |
| Storage | SQLite, pure Go modernc.org/sqlite v1.60.1; WAL + `synchronous=NORMAL`, default cache; schema from `internal/store/schema.sql` (04 §4) | No cgo; multiple processes (archo.exe, RimArcho.exe, child processes) share one database |
| XML reading | Standard library `encoding/xml` streaming parser | Def layer extracts fields without building a DOM |
| XPath simulation | antchfx/xpath v1.3.9 + custom compact node store `internal/xdoc` (implements `xpath.NodeNavigator`) | No measured difference from .NET XPath 1.0 (18). antchfx/xmlquery v1.5.2 is used only in `spikes/` |
| .NET metadata | saferwall/pe v1.6.5 reads tables and heaps + custom CustomAttribute blob decoder (about 450 lines, `internal/dotnet`) | No need to develop an ECMA-335 reader |
| MCP | Official Go SDK modelcontextprotocol/go-sdk v1.8.0, stdio only | Supports tool annotations |
| KeyValues | Custom `internal/vdf` | Reads `libraryfolders.vdf` and `appworkshop_*.acf` |
| Hash | xxh3-128 (zeebo/xxh3 v1.1.0) | Fast |
| Pack files | `.archopack` format 2: standard library `archive/tar` + klauspost/compress v1.20.3 `zstd` (`SpeedBetterCompression`, 8 MB window, single-thread encoding; decoding uses `WithDecoderLowmem`, one thread, 64 MB window limit), copy digests SHA-256 (standard library); standard library reads format 1 ZIP | Rationale and measurements in 10 §1 |
| Game menu backgrounds | `internal/menubg`: klauspost/compress v1.20.3 (1.7 ZIP zstd method 93) + custom Unity SerializedFile / ResourceManager / Texture2D range reads (no ZIP in 1.6) + woozymasta/bcn v0.7.0 (BC1 / BC3 / BC7, MIT); standard library for RGBA32 | Pure Go, no external runtime tools; decode and resize by row; keep the official preview if extraction fails. Resource selection, support boundaries and cache rules: [16 §2.14](16-interface/2.14-settings.md#214-settings) |
| Character width | mattn/go-runewidth v0.0.31 (`EastAsianWidth` disabled) | Chinese alignment; see §7.2 |
| System interfaces | golang.org/x/sys | Path resolution, process table, recycle bin, hidden child-process windows |
| Git (subscriptions, 11 §1) | go-git v5 (github.com/go-git/go-git/v5, pure Go) | Players need not install Git; `CGO_ENABLED=0` unchanged. Only anonymous HTTPS / local fetch, shallow clone, ls-remote and checkout. System `git` rejected: most players do not have it; discovery and output parsing vary by version. Tests create bare repositories in temporary directories with go-git, without networking |
| Decompilation | ilspycmd (external, optional, not distributed) | Called on demand (05 §5.4). Search `ARCHO_ILSPYCMD` → PATH → `~/.dotnet/tools`; never install automatically; if missing, suggest `dotnet tool install --global ilspycmd` |

Do not introduce Web frameworks, frontend frameworks or build tools, ORMs, or cgo SQLite (thresholds in 18 §1). Archo model invocation: 15 §3.

## 2. Code structure

```
cmd/archo/            Entry: no arguments opens TUI; arguments use CLI; `mcp` uses MCP; `gui` opens a window
internal/
  env/                Environments, path detection, Version.txt, Player.log location, read-only ModsConfig parsing, data directory
  source/             Source types, junction / symbolic link / 8.3 short-path resolution, child directory enumeration, rebinding, recycle bin
  scan/               Tolerant About.xml parsing, PublishedFileId, LoadFolders, ACF, git HEAD, origin.json, layered fingerprints, similarity
  identity/           Reconciliation R1–R7, Lineage membership, change events, identity questions (pure logic)
  store/              SQLite access, schema and upgrades, gc
  ids/                Internal IDs and short references
  vdf/                Valve KeyValues
  textenc/            Mod XML encoding normalization (including UTF-16)
  content/            Effective content (LoadFolders, version directories, conditions)
  modsearch/          Search Mod files in the game's loading scope (`files search` / `search_files`, 05 §5.5)
  xdoc/               Compact XML node store and NodeNavigator for Patch simulation
  analyze/            Def facts, Patch simulation, Def / Patch findings
  dotnet/             .NET metadata reading
  harmony/            Read Harmony facts from assemblies
  logs/               Player.log parsing and clustering
  finding/            Finding model
  loadout/            Plan lists, ModsConfig.xml reading and writing (backup, atomic replacement)
  order/              Constraint graph and sorting
  rules/              Community rule data
  bbcode/             Workshop description BBCode parsing
  app/                Operations behind commands: environments, scan pipeline, reference resolution, plans, analysis, logs, packs, launch, running-game detection
  l10ndata/           Embedded feature JSON translations under locale/<language>/; add a locale directory to bundle another language (16 §1)
  view/               Surface-independent view models; language selection and lookup shared by TUI and GUI (16 §1)
  cmds/               Single command table: argument parsing, execution, outline text (13 §1)
  render/             Outline text rendering (13 §3)
  surface/cli|mcp|tui|gui/   Surfaces
  deps/               External dependency discovery and versions (todds, SteamCMD, §2.1)
  feedback/           Feedback: issue body, prefilled links, relay submission (16 §2.14)
  selfupdate/         RimArcho updates: release information, version comparison, download and verification, extraction, replacement and rollback (§5.1)
  icon/, winres/      Icon rendering (portrait.svg) and Windows resource objects
  buildinfo/          Release version (-ldflags -X)
  procmem/            Process memory readings
  testkit/            Synthetic test environments
scripts/release.ps1   Release script; scripts/winres/ writes .syso; scripts/monitoricon/ holds Archo Monitor icons
feedback-relay/       Feedback relay (Cloudflare Worker, JavaScript, 16 §2.14): worker.mjs, worker.test.mjs, wrangler.toml; deployment in README.md; excluded from releases, deployed by repository owner
monitor/              Archo Monitor (in-game helper Mod, C#, 19): src/ source, About/ and LoadFolders.xml, build.ps1; outputs monitor/out/ArchoMonitor/ (not committed)
spikes/               Technology experiments and measurement programs (same module, included by `go vet ./...`; excluded from releases)
testdata/             Synthetic Mod samples (mods/), golden files (golden/), .NET fixtures (dotnet/)
```

- CLI and MCP depend only on `cmds`; TUI and GUI depend on `view` and `app`. Business packages never depend on surfaces.
- Business rules belong only in `app` or `view`. A surface-layer condition such as "if this is a Workshop Mod …" is a defect (16 §1).
- Heavy work runs in short-lived child processes. TUI, GUI and commands needing content facts launch scanning (`archo scan`) and analysis (`archo analyze --env <name>`) as children, passing `RIMARCHO_HOME`. Progress lines (`progress: …`) go to stderr; the UI routes them to its status bar. Tests inject an in-process runner.

### 2.1 External dependencies (`internal/deps`)

RimArcho uses two programs that are not compiled into the exe. Only `deps` discovers them. Other features (texture optimization, game downloads, Mod sources, settings, uninstall) ask it instead of searching themselves.

| Name | Purpose | Search order | Version | RimArcho-owned directory |
| --- | --- | --- | --- | --- |
| `todds` | DDS encoder (10 §2) | `textures.todds` → own copy → `PATH` | Run `--version`; anything other than 0.4.1 is unusable | `%LOCALAPPDATA%\RimArcho\todds` |
| `steamcmd` | Download game versions (11 §3) and Mods | `steamcmd.path` (search same directory if file absent) → own copy → `PATH` | Do not run it (it self-updates); leave version empty | `%LOCALAPPDATA%\RimArcho\steamcmd` |

Interface (kept small for other features):

```go
f := app.Deps()                          // *deps.Finder; reads current settings
path, version, ok := f.Locate(deps.SteamCMD) // ok: found and usable
info := f.Status(deps.Todds)             // Path, Version, Source (config/own/path), OK, Own, Err, ConfigKey, Dir, Installable
all := f.All()                           // todds, then steamcmd
app.InstallDep(name, progress)           // Download to own directory (call only after player consent); returns executable path
app.RemoveDep(name)                      // Delete only own copy; see 02 §1
```

- Discovery does not use networking, download or modify files. Cache version probes by path, size and modification time so refreshing settings does not run todds each time. A failed probe is not cached (a scanner holding the file, a timeout): the next lookup probes again. Probes run outside the cache lock.
- Installation does not change settings: if the player specified another copy, it remains selected after installation (the settings explanation shows the source).
- `app.CheckToolUpdates` checks only the currently used managed SteamCMD and todds: Valve's Windows SteamCMD package manifest and todds' latest stable GitHub release. It downloads no executables. Results appear in settings rows and `archo deps`; scheduling: 16 §2.16. Read SteamCMD's version from the package manifest; manual updates run its own update process, never overwrite an installed tool with the bootstrap ZIP. Install only the todds version supported by the caller; merely notify about incompatible newer releases.
- Check processes before updating or removing; refuse while a same-named tool is in use. In-app use and replacement of SteamCMD and todds share locks. Refuse replacement if Windows process inspection fails; do not infer whether a tool is busy from a single failed write.
- CLI: `archo deps` (status, versions, paths, sources), `archo deps install <name>`, `archo deps remove <name>`. Settings buttons run the same commands; downloads enter background tasks (16 §2.16).
- Tests: `internal/deps` uses temporary directories and fake `LookPath` / `Version` to check discovery order, version caching and `Own`; `internal/app/deps_test.go` checks that `RemoveDep` refuses copies it does not own.

### 2.2 Network: proxy and GitHub mirror (`internal/netcfg`)

Some of RimArcho's hosts are often unreachable in mainland China: `raw.githubusercontent.com` (community data), `api.github.com` and `github.com` releases (self-update, todds), and at times `steamcommunity.com` and `api.steampowered.com`. Players there usually run a proxy tool (Clash, v2rayN) in system-proxy mode, which sets the Windows per-user proxy. WebView2 follows that proxy, but Go's default transport does not, so without `netcfg` the window's pages would load while RimArcho's own downloads fail.

`netcfg.Install` (called once in `cli.Run`, so in `archo.exe`, `RimArcho.exe` and their child processes) replaces `http.DefaultTransport` with a wrapper. Every client without a transport of its own goes through it; test clients (`testkit`, `App.HTTP`) keep their own. `app.OpenWith` and `config set network.*` pass the settings to `netcfg.Configure`, so a change applies at once in the running process.

| Setting | Values | Behavior |
| --- | --- | --- |
| `network.proxy` | `system` (default), `off`, or a proxy URL (`http://`, `https://`, `socks5://`; a bare `host:port` is HTTP) | `system`: `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` first, then the Windows proxy (`HKCU\…\Internet Settings`: `ProxyEnable`, `ProxyServer` including per-scheme `http=…;https=…;socks=…`, `ProxyOverride` with `*` wildcards and `<local>`), read at most every 5 seconds. PAC scripts (`AutoConfigURL`) are not read. Loopback never goes through a proxy. |
| `network.github_mirror` | empty (default) or a prefix (`https://gh-proxy.org/`) or a template with `{url}` | GET and HEAD to `github.com`, `api.github.com`, `codeload.github.com`, `raw.githubusercontent.com`, `objects.githubusercontent.com`, `release-assets.githubusercontent.com`, `gist.githubusercontent.com` go to the mirror first. A mirror that fails to connect, gives no headers within 15 seconds, or answers ≥ 400 is followed by the direct request. `Authorization` and `Cookie` headers are not sent to the mirror. |

- A failed request to a non-loopback host adds what to set: the proxy, and for GitHub hosts the mirror. The error still unwraps to its cause and keeps `Timeout()`.
- A mirror sees and serves the files it relays. todds' final release zip is pinned by SHA-256 (`textures.ToddsZipSHA256`), so a mirror cannot swap it. Self-update checks the zip against the release's checksum file, but both come through the mirror, so the player trusts the mirror for updates; the settings description says to use a trusted one. Community data is parsed and rejected when malformed (08 §2).
- Not covered: SteamCMD (its own networking), the login windows (WebView2, system proxy), and the feedback relay's host. RimCrow's hosts-file overrides and automatic mirror catalogs are not adopted: the proxy setting covers most players, and mirror lists need upkeep.
- Tests: `internal/netcfg` (mirror first, fallback on 502 and on a closed port, POST and other hosts untouched, system-proxy parsing and bypass, error hint) dial only loopback; `TestNetworkSettings` checks `config set` validation and the settings rows.

## 3. Data locations

| Platform | Directory |
| --- | --- |
| Windows | `%APPDATA%\RimArcho\` |
| macOS | `~/Library/Application Support/RimArcho/` |
| Linux | `$XDG_DATA_HOME/rimarcho/`, otherwise `~/.local/share/rimarcho/` |

The `RIMARCHO_HOME` environment variable overrides this table (used by tests and child processes).

| Content | Description |
| --- | --- |
| `config.toml` | Reads only `tombstone_days` (default 90); writes defaults on first launch. Other settings live in the database meta table (`cfg:<key>`, 13 §4.2) |
| `archo.db` (and WAL) | All facts, identities, plans, history, findings, log attribution cache, and the save full-read cache (`save_cache`, 12 §1.7: the facts as gzip JSON plus summary columns for lists) |
| `cache/` | `previews/` (thumbnails), `workshop/<Workshop ID>.json` (Workshop descriptions), `workshop-img/` (description images), community data (`communityRules.json`, `steamDB.json` compressed into `steamDB.index.json`, `replacements.json.gz`); `loadcost-v1.json` (per-Mod list loading measurements, keyed by Mod directory, stamped by the loading-directory list fingerprint). `ilscan/<SHA-256>.json` (comp-list edits of one DLL for log attribution, 06 §2, §5). `cache clear` clears supported categories; they are rebuilt on demand |
| `source/` | Default managed Source library; `folder set source` / `folder move source` persist another root. Mod projects: `mods/<lowercase filesystem-safe packageId>/<DLL stem> <first 6 hash hex>/`; a prefix collision extends the suffix without moving existing code. Game projects: `game/<build>/Assembly-CSharp/`. Type-only files live in the same DLL folder under `.types/<type-key>/`. Identity, publication and retention: 05 §5.4. Library-relative records survive relocation; contents are never exported or attached |
| `source-library.lock` | Coordinates source readers/decompilers with relocation and cleanup across processes; artifact locks live in the library's `.locks/` |
| `logs/` | `gui.log`, `pack-build.log` (`wezterm-start.log` may remain from development builds before 1.0) |
| `backups/` | Database backups `archo-YYYYMMDD-HHMMSS.db` (local time); save backups `saves/YYYYMMDD-HHMMSS/` (12 §4.4, same time format) |
| `packs/` | Packs (10) |
| `webview2/` | WebView2 user data (not in `%APPDATA%\RimArcho.exe`) |
| `refnums/` | `<prefix>.txt` per page, written while GUI reference numbers are on (16 §3.5); only for looking numbers up, never read back |
| `wezterm/`, `wezterm.lua` | A portable WezTerm and its configuration, left by development builds before 1.0; RimArcho neither uses, writes nor deletes them (16 §9.2) |
| `rimcrow-*` | Temporary RimCrow database copies during `import rimcrow`; deleted afterward |

- `app.LocalDir()` resolves the separate managed root: `cfg:local.dir`, otherwise `%LOCALAPPDATA%\RimArcho` on Windows (the existing `LockBase` test override replaces Local AppData). The first-run choice (16 §2.15), or `archo config set local.dir <absolute-folder>`, may select an empty root before it holds content; it neither moves nor creates files. The root must be outside the data directory, game/source roots and Workshop. Existing per-folder settings keep precedence. This root supplies defaults for locked Mods, Archo Monitor, subscriptions, SteamCMD Mod downloads, SteamCMD, todds, account sessions and the Steam runtime. Packs, source files and caches retain their data-directory defaults; downloaded games retain their individually chosen destinations. Once the root holds content, relocate folders with the existing folder controls (03 §4.1). Uninstall resolves the same root (02 §2).
- The data directory, game directories, user data directories and source roots must not contain one another (03 §3.1).
- Writes outside the data directory are limited to the allowlist in 02 §1.
- The game's ModsConfig.xml backup is `ModsConfig.xml.rimarcho.bak` in the same directory (07 §4.1).

## 4. Tests

| Layer | Method |
| --- | --- |
| Unit | `_test.go` in each package: About.xml (BOM, tag case, `authors`, derived packageId), semantic file classification, similarity, incremental hashing, ACF, VDF, IDs, detection, Player.log, ModsConfig, etc. |
| Identity | `internal/app/accept_test.go` builds A1–A11 from 04 §5 with `internal/testkit`: fake clock, fixed-seed IDs, `mklink /J` (symbolic links on macOS / Linux), handwritten Mod templates |
| Output | Golden tests (`internal/cmds/golden_test.go`, `m4_golden_test.go`): `basics`, `lifecycle`, `rebind`, `loadout`, `problems`, `harmonylog`; snapshots in `testdata/golden/*.txt`; stable across runs |
| CLI = MCP | `internal/surface/mcp/mcp_test.go`: `TestToolsAndAnnotations`, `TestCLIParity`, `TestRefsRoundTrip`, `TestAmbiguousPackage`, `TestTruncation`, `TestWriteTools`, `TestEnvParameter`, `TestStillComputing` (13 §6) |
| Analysis | B series (06 §9) and C series (05 §6); Harmony fixtures in `testdata/dotnet`: fake 0Harmony, fake Assembly-CSharp and C# source for four Mods, build script and compiled DLLs (4–5 KB each, committed; tests need no .NET SDK) |
| Plans and sorting | `internal/order/order_test.go` (S1–S5), `app/loadout_test.go`, `loadout.TestWriteFileBackupAtomic` |
| TUI | teatest interaction snapshots (`TestEnableMoveSortSaveUndo`, `TestBreakpointsAndCJK`, `TestNeedsView`, `TestChangesView`, `TestMouseClicks`, `TestNeedsLogs`, `TestT7Wrapping`, `TestFullSections`, etc.); manual usability test T4 (16 §11) |
| GUI | `gui.TestSameViewModels`, `gui.TestVerbsAndUndo`; debug in a browser using `spikes/guidev` (listens only on 127.0.0.1, excluded from releases) |
| Feedback relay | `node --test "feedback-relay/*.test.mjs"`: fake GitHub, Map as KV, the real `SendGate` Durable Object over Map storage; no real GitHub or Cloudflare. Covers issue creation, JWT signing, token reuse, three limits, refusals, excluding IP and contact details from issues, private key format. Client uses `httptest` as relay: `feedback.TestSend`, `gui.TestFeedbackSend`, `tui.TestFeedbackSendTUI`; point `RIMARCHO_FEEDBACK_RELAY` to it. Clear `feedback.Relay` too when testing "no relay"; tests never contact the real relay. One real submission belongs in VM testing |
| Scale | Opt-in with `RIMARCHO_SCALE_TESTS=1` (about a minute each, so not in the default run): `tui.TestT1KeyLatency1354` (16 §11 T1, 1,354 synthetic Mods, slowest key ≤ 50 ms; run it alone, as other packages' CPU load breaks the threshold) and `vmstate.TestScaleFixture` (the 4,000-Mod VM fixture) |
| Concurrency | `app/concurrent_test.go`: two connections to one data directory; external changes update `ChangeKey`, edits from a stale view do not overwrite them (G8) |
| Real corpus | Optional tests run only with environment variables: `RIMARCHO_REAL_HOME` (data directory) + `RIMARCHO_EXE` (archo for children) for TUI memory; `RIMARCHO_IDLE_SECONDS` changes idle duration; `RIMARCHO_NO_BROWSE` skips browsing; `RIMARCHO_LOG_VERSION` specifies log replay major/minor version; `RIMARCHO_REAL_COPY` (data directory **copy**) for T1; `RIMARCHO_WORKSHOP_CACHE` replays real Workshop descriptions. Measurement programs in `spikes/` (`tuimem`, `tuistart`, `mcplat`, `mcpcheck`, `analyzereal`, `logreal`, `logfirst`, etc.) |

Test speed: `testkit.New` copies a schema-only database built once per test process instead of creating the schema for each fixture, and replaces the SteamCMD retry backoff (5 s / 15 s) with no wait. Run the suite with `go test -count=1 ./...`: on Windows, recording the test cache's file accesses makes an uncached run about 40% slower (32 threads: 4.5 minutes, against 3.2 with `-count=1`).

`l10ndata.TestSourceLanguage` scans tracked source files and falls back to a repository walk when Git is unavailable. Han characters and fullwidth punctuation fail the test outside the centralized, documented exceptions in `internal/l10ndata/source_language_test.go`; a file whose name contains `zh-CN` is exempt without being listed.

**Archo Monitor**: its own tests, the manual in-game checklist and its build are in 19 §7.

In the test VM, `scripts/vm/round.ps1` automates a whole game round (new colony, play, save, load, return to main menu, exit) and checks phase order, save name, and whether the game uses RimArcho's bundled Monitor. Test-only Archo Drive controls the game (`scripts/vm/drive`, excluded from releases), without screenshots; see `scripts/vm/README.md`. Like `release.ps1`, `vm.ps1 build` builds Monitor from `monitor/`. Before a round, run `archo monitor install`, then place the driver: any content change replaces the entire development Monitor folder, removing the driver too.

**Test safety rules:**

1. Automated tests use only synthetic environments and temporary data directories (`RIMARCHO_HOME` points to a temporary directory). All write-operation tests (save, copy, links, recycle bin, packs) run only on synthetic environments.
2. With real data, **game, Workshop, `Mods/`, ModsConfig.xml and RimCrow databases are read-only**. Record and compare ModsConfig.xml md5 before and after measurements; use marker files to check mtime if needed.
3. Real measurements needing RimArcho database writes use a data directory **copy** (`RIMARCHO_REAL_COPY`), never `%APPDATA%\RimArcho`. Restoring after a measurement is unreliable: an interrupted measurement, or test tools opening the real Can it run page, can leave settings such as `log.selected` / `log.seen` in the real `archo.db` pointing at the real Player.log.
4. Measurement scripts must leave no files in user directories; record files third-party programs write themselves.
5. VM game saves sync to the player's real Steam Cloud. Therefore rounds run by `round.ps1` do not autosave and delete their own saves before exit. Place the test driver only in the Monitor directory written by `archo monitor`, never a guessed default. When rehearsing guest scripts on the host, point `USERPROFILE`, `APPDATA` and `LOCALAPPDATA` to temporary directories.

## 5. Releases

- Primary platform: Windows amd64. Link the same source twice (16 §8.1):
  - `archo.exe`: console application, CLI, `archo mcp`, `archo tui`, `archo gui`.
  - `RimArcho.exe`: `-H windowsgui`; double-click opens the GUI without a console window; `buildinfo.GUI=1`. Scan and analysis children use `CREATE_NO_WINDOW`.
  - Both executables share a data directory and database and can run together.
- `scripts/release.ps1 -Version X.Y.Z[-rc1] [-SkipTests]`:
  1. Validate the version format (semantic versioning), then test: `go test -count=1 ./internal/... ./cmd/... ./scripts/... ./spikes/hugslogs ./spikes/corpuslabel` (product code, build helpers and maintained corpus tools); after scan or TUI speed changes, also set `RIMARCHO_SCALE_TESTS=1` (the Scale row in §4);
  2. `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X buildinfo.Version/Commit/Date"`, marking dirty commits with `-dirty`;
  3. Before each build, `go run ./scripts/winres` writes `cmd/archo/rsrc_windows_amd64.syso` (icons and version information, 16 §8.6) and deletes it afterward;
  4. Build Archo Monitor (`monitor/build.ps1`, version rules in 19 §6), place in `monitor/ArchoMonitor/`;
  5. Package `dist/archo-<version>-windows-amd64.zip`: both executables, README.md, CHANGELOG.md, LICENSE, `monitor/ArchoMonitor/`; from 1.0.6 on also README.zh-CN.md and the git-tracked files of `skills/` (agent skills such as `rimworld-probe`, 14 §3). A new top-level entry goes into `selfupdate.Files` one release before the zip carries it, because older versions refuse a zip with entries they do not know (§5.1 step 3); 1.0.5 added these two;
  6. Write `dist/archo-<version>-windows-amd64.zip.sha256` (sha256sum format: `<64 lowercase hexadecimal digits>  <zip name>`). Upload both the ZIP and the checksum to GitHub Releases; self-update refuses to install without the checksum (§5.1).
  The script writes only `dist/`; it never pushes or publishes.
- The repository is public; do not commit binaries. `internal/icon` generates icons from `portrait.svg` at runtime or release time (16 §8.6).
- **Building Archo Monitor**: 19 §7.
- The feedback relay (`feedback-relay/`) is excluded from releases. The repository owner deploys it to Cloudflare following its README and sets its address as `Relay` in `internal/feedback/relay.go` (deployed and set). A build with an empty address offers only "open GitHub".
- Publish through GitHub Releases, tag `v<version>`; mark prereleases (`-rc1`, etc.) as prereleases on GitHub. Self-update: §5.1 (files change only after player confirmation, never unattended). Executables are unsigned (explained in README).
- No installer: extract and run. Uninstall with `archo uninstall` or Settings › "uninstall RimArcho…" (02 §2); after RimArcho closes, a temporary cmd script deletes the ZIP's files. If an installer is adopted later, its uninstaller becomes the last step and also handles updates (§5.1).
- Record text output format (13 §5) and MCP tool changes in CHANGELOG.
- Before release, run the entire [release VM tests](../../UserGuide/release-vm-tests.md) table in the VM (real data, `scripts/vm/`); fix findings or record TODOs before tagging.
- macOS and Linux: `GOOS=linux` and `GOOS=darwin` compile, without runtime testing; official support in 1.x (00 §5).

### 5.1 RimArcho self-update

Checks are automatic; installation needs player confirmation (02 §1). Code: `internal/selfupdate` (independent of other packages), `internal/app/selfupdate.go`; UI: 16 §2.3, §2.14, §2.16, §9.1; CLI `archo update [--check]` (13 §4).

- **Workshop startup**: `selfupdate.InWorkshopFolder(appDir)` recognizes a RimWorld item under any library's `steamapps/workshop/content/294100/<digits>/`, including an extracted child directory; Windows path matching ignores case. The GUI's relocation dialog and first-run choices are specified in 16 §2.15. `app.RelocateWorkshop` copies `selfupdate.Files`, starts the installed GUI and exits the old one. The new GUI receives the old folder through `RIMARCHO_WORKSHOP_CLEANUP`, waits for old-folder RimArcho processes to close (up to 30 seconds), and completes the confirmed release-only cleanup (`selfupdate.CleanWorkshopRelease`): at the item root it removes the release entries except `selfupdate.ItemFiles` (`README.md`, `README.zh-CN.md`, `LICENSE`, `skills/`), which the published item carries itself (see **Upload** below) and the extraction only overwrote with the same files; in a folder of its own below the item root it removes every release entry. `UserGuide/` is never a release entry. Failure is reported, without deleting item content. `app.InstallSelfUpdate` refuses installation from a Workshop folder, even if relocation was declined. TUI/CLI startup prints one warning to stderr suggesting `RimArcho.exe` for relocation; machine-readable stdout is unchanged. Distribution basis: [Workshop research §1.5–§1.6](../research/2026-11-05_workshop-distribution.md#16-first-start-from-the-workshop-folder). File boundaries and uninstall: 02 §1, §2.
- **Check** (`app.CheckSelfUpdate`): automatic maintenance category `self` (16 §2.16.1 A15), Settings "check for updates", TUI "update RimArcho", and `archo update [--check]` (`App.RunAutomaticNow`, recorded like automatic checks). Source selection is described under **Workshop source** below. For GitHub, `selfupdate.Client.Latest` reads Releases (`api.github.com/repos/ToumanLin/RimArcho`; metadata ≤ 1 MB, 15 seconds; `RIMARCHO_UPDATE_API` substitutes a test release service for VM tests without publishing to the production repository). **The running version determines the channel**: stable requests `/releases/latest` (GitHub excludes drafts and prereleases); prereleases (such as `1.1.0-rc1`) use GitHub only, request `/releases?per_page=30` and select the highest non-draft version, including prereleases, so prerelease users also receive stable releases. Compare semantic versions, `rc2 < rc10` (`selfupdate.Compare`). Development builds (non-semantic versions such as `dev`) neither check nor install. Store results in meta `self-update` (source, check fallback reason, version, notes, release page, two assets, and the Workshop item when applicable) and `automatic:self` (`latest` ensures one notification per version, even across sources). Recalculate update availability against the running version each time (`App.SelfUpdatePending`), so a record of the installed version or an older one does not count. Keep the last results and their source for offline use.
- **Assets**: `archo-<version>-windows-amd64.zip` and `<zip>.sha256` (§5 step 6). If either is absent, only notify and direct the player to the release page.
- **Install** (`app.InstallSelfUpdate`):
  1. Create `.archo-update/` in the application folder (on write failure, report an error and link the release page). Open its `lock` exclusively (Windows non-shared handle owned by the process). Refuse a second installation in that folder ("another RimArcho is already installing the update"); startup cleanup also skips it.
  2. Download into `download/`: checksum first (≤ 64 KB), ZIP into `<zip>.part`, computing SHA-256 while writing. Rename to ZIP only if its byte count matches the release asset size and its hash matches the checksum. On interruption, size/hash mismatch or stop, delete `.part` and change nothing. Reuse a downloaded ZIP with a correct hash. Limit 200 MB.
  3. Extract into `new-<version>/` (`selfupdate.Extract`): allow only `selfupdate.Files` at ZIP root (`archo.exe`, `RimArcho.exe`, `README.md`, `README.zh-CN.md`, `CHANGELOG.md`, `LICENSE`, `monitor/`, `skills/`; 1.0.0 knows only the first six, so a 1.0.0 install cannot update itself to a zip with the last two). Reject absolute paths, drive letters, `..`, symbolic links, files over 200 MB and totals over 500 MB; require both executables. Treat `\` and `/` as separators (Windows PowerShell 5 Compress-Archive writes `\`).
  4. Wait for other RimArcho processes in the application folder (GUI, TUI, `archo mcp`, their scan children) to exit (16 §2.16.1 W09): the new version may upgrade the database, making continued use by old versions unsafe. Allow stopping while waiting: change no files, retain the download for next time.
  5. Replace (`selfupdate.Apply`): rename each old entry into `old-<old version>-<time>/`, then rename the new one into place. Windows permits renaming running executables, not deleting them, so move old files aside. All files remain on the same volume in the application folder; only renames occur. On any failure, reverse completed steps in reverse order. Leave entries absent from the ZIP untouched.
  6. Verify: run the new `archo.exe version` with an empty temporary data directory (`selfupdate.VersionOf`, up to 20 seconds, not interrupted by stop). Require the new version; otherwise roll back (`Swap.Rollback`) and report "the new program could not start; restored RimArcho <old version>". From step 5 onward, window closing or TUI exit waits for completion or rollback (`app.WaitSelfSwap`, 16 §2.16.1 W04).
  7. The surface restarts: GUI launches the new `RimArcho.exe` with the same arguments (`RIMARCHO_UPDATED_FROM=<old process pid>`), records task results, then closes. TUI exits, closes the database, runs the new `archo.exe` in the same terminal and waits (`cli.restartTUI`). CLI only replaces files and asks the player to reopen.
- **Cleanup**: on GUI and TUI startup, without blocking the first screen, `App.CleanSelfUpdate` waits up to 10 seconds for `RIMARCHO_UPDATED_FROM` to exit, then deletes `old-*`, `new-*` and `download/` in `.archo-update/`. Keep files still in use (the old TUI process waiting for the new one) until next time. Uninstall deletes all `.archo-update/` (02 §2).
- **Workshop source** (VM round 2026-11-06 with unlisted test item `3834567511`; rationale, risks and live tests: [research/2026-11-05_workshop-distribution.md](../research/2026-11-05_workshop-distribution.md)). The Workshop supplies checks and step 2's input; steps 3 to 7 remain the same. GitHub stays for the first download, for prereleases and as the fallback. Code: `internal/selfupdate/workshop.go`, `internal/app/selfupdateworkshop.go`.
  - **Selection**: `updates.source` is `auto` (default) or `github` (16 §2.14). Automatic prefers Workshop and falls back to GitHub on failure; GitHub skips Workshop. There is no Workshop-only choice: it would differ from automatic only by failing where the fallback works. The running prerelease channel overrides the setting.
  - **Item**: `selfupdate.WorkshopItem` is the RimArcho item, 3835688284 (created 2026-11-07, first carried by 1.0.5), and `monitor/About/PublishedFileId.txt` holds the same ID; an empty value skips Workshop. `RIMARCHO_UPDATE_WORKSHOP_ITEM` overrides it for VM tests, including an empty override. The item named RimArcho must be **public**: the anonymous Web API answers `result 9` for an unlisted item (VM 2026-11-06), so only subscribers could see its version. It contains the Archo Monitor Mod folder (19 §6) plus the Windows amd64 release assets of §5 step 6 unchanged. The item carries stable releases only. Its tags are the game's: `Mod` and each game version in Monitor's About.xml `supportedVersions` (`1.5`, `1.6`, `1.7`), as RimWorld's own uploader sets them. Its version is the description's stamp, a last line `RimArcho v<X.Y.Z>` that every publish writes (`selfupdate.Stamp`); `selfupdate.DescriptionVersion` reads the last such line and ignores prerelease and malformed ones. The version cannot be a tag, because Steam shows every tag on the item page (players read the `v1.0.5` of 1.0.5 as a misplaced tag), nor key-value tags or metadata, because the anonymous details request does not return them (`IPublishedFileService/GetDetails` needs a Web API key; checked 2026-11-07). An item published by 1.0.5 has a `v<semver>` tag and no stamp; `WorkshopVersion` reads that tag when the description has no stamp, ignoring unrelated, invalid and prerelease tags and picking the highest stable one. The zip name must match the checked version. Anonymous SteamCMD downloads an unlisted item (VM 2026-11-06).
  - **Older clients**: RimArcho 1.0.5 reads only the `v<semver>` tag; 1.0.0 and earlier read only GitHub. Without the tag, 1.0.5's Web API step finds no version and it falls back as described under Check: a subscribed folder's zip names, then GitHub. So 1.0.5 keeps updating; only an install that is not subscribed and cannot reach GitHub stops seeing releases until updated by hand. The `v<semver>` tag is therefore not kept beside the game tags.
  - **Check**: one `GetPublishedFileDetails` POST (no key; `postWorkshopDetails`, 15-second deadline) reads the description's stamp, else a `v<semver>` tag; `selfupdate.Newer` compares the version with the running one. The check never starts SteamCMD or downloads. When the Web API gives no version (offline, result other than 1, neither stamp nor stable tag), a subscribed item folder in any Steam library supplies it from its zip names, each with its checksum (`selfupdate.SubscribedVersion`); that also shows the zip size. Otherwise an empty or invalid item ID, a missing item or no version falls back to GitHub. CLI check output names the resulting source and fallback reason. `time_updated` is not used (Mod checks compare times, 16 §2.16.1 A03).
  - **Download**: search all Steam libraries with `env.WorkshopFolders` for `steamapps/workshop/content/294100/<id>/`. Copy the exact checked zip and checksum into `download/` using `selfupdate.CopyLocal`; verify with the same digest and size logic as GitHub (`VerifyLocal`, checksum ≤ 64 KB, zip ≤ 200 MB). GitHub requires its declared asset size; local Workshop files use the checksum as authority. A missing, stale or invalid subscribed copy proceeds to SteamCMD. Always call `SteamCMDAccount("anonymous")`, never the configured account, because a named account can sign desktop Steam out. Use `%LOCALAPPDATA%\RimArcho\steamcmd-self`, with one ID through `runWorkshopBatches` and the shared SteamCMD instance lock; do not register or scan a Mod source. Success requires `Success. Downloaded item <id>` and the exact zip and checksum, since SteamCMD exits 0 on failure. Progress is indeterminate. If SteamCMD is absent or fails, resolve `/releases/tags/v<checked version>` on GitHub and download that version, or report both failures; never substitute a newer version. The updater does not install managed SteamCMD. Delete the whole private base folder, including its ACF, after a successful swap and on cleanup.
  - **Upload** (maintainer only): `archo workshop publish` (13 §4.4), a separate step after the GitHub release; `scripts/release.ps1` still never publishes (§5). `internal/workshoppublish.Assemble` reads the matching Windows amd64 zip and checksum from `dist/`, validates the checksum with `selfupdate.ParseSum`, checks the zip's SHA-256 and 200 MB limit, and copies them beside a built Monitor into a fresh `dist/workshop-<version>-<suffix>/`. It prefers the release's `dist/archo-<version>-windows-amd64/monitor/ArchoMonitor/`, then `monitor/ArchoMonitor/`, then `monitor/out/ArchoMonitor/`. Require About.xml, LoadFolders.xml, all three game-version assemblies and Languages. It also copies `README.md`, `README.zh-CN.md`, `UserGuide/user-guide.md`, `UserGuide/user-guide.zh-CN.md` and `skills/` (without `bin/`, `obj/` and `node_modules/`) from the repository root into the item at the same paths; each is required. They sit beside the zip, never in it: the zip stays the GitHub release byte for byte, because updaters before 1.0.5 reject top-level entries they do not know and no updater accepts `UserGuide/`. RimWorld ignores them, since LoadFolders.xml loads only the Mod folders. About.xml stays Monitor's (the game lists it as Archo Monitor); only the item title is RimArcho. An optional About/Preview.png must be smaller than 1 MB. Use the explicit item ID, otherwise `monitor/About/PublishedFileId.txt`, otherwise the built Monitor's ID file; missing or empty means create. The worker uses `CreateItem(294100, Community)` when needed, saves the new ID to the source and staged About folders before updating, then sets absolute content, optional preview, title RimArcho, description and the tags before `SubmitItemUpdate`. The tags replace all others: `Mod` and Monitor's game versions, read from the staged About.xml by `workshoppublish.Tags`, which refuses a list without a game version or with one that is not `N.N`. The description is always set, so its stamp names the release being published: the command line's file, else the repository's `UserGuide/release-post.steam.txt`, stamped by the command (a stamp already at the file's end is replaced) and at most 8,000 bytes with the stamp; with neither file, publish is refused before Steam. The Simplified Chinese description is `--description-zh`, else `UserGuide/release-post.zh-CN.steam.txt`, stamped the same way; with neither, it is left alone. Visibility is left alone unless supplied to the worker. A translation (the Simplified Chinese description) is a second update after the main one, because an update handle holds one language: `SetItemUpdateLanguage(schinese)`, title RimArcho and that description, with an empty change note; the worker reports `TRANSLATED <id> <language>`. The item page's preview is the command line's, else `docs/assets/release-post/workshop-preview.gif` (smaller than 1 MB), else Monitor's `About/Preview.png`; the GIF stays outside the item folder, so the game still shows Preview.png. The same Steam runtime as subscriptions supplies the DLL; Steam must be running and signed in, and RimWorld is never started. `GetItemUpdateProgress` feeds stderr progress; call results report account restrictions and whether the Workshop legal agreement must be accepted on the item page (the item stays hidden until accepted). A failure after creation retains the ID for retries. `--dry-run` assembles and validates without opening the player database or contacting Steam. Staging folders remain for inspection; failed assembly removes its partial folder. Code: `internal/app/workshoppublish.go`, `internal/steamworks/publish*.go`.
- **Untouched**: data directory, game and Mods. Under `%LOCALAPPDATA%\RimArcho`, only the private self-update base and the existing SteamCMD runner's scripts, logs and own runtime state are written. First opening the new version upgrades the database if its schema changed (04 §4); old versions refuse newer databases (`unsupported schema version`). To revert, back up the database first (Settings, "database" › "back up", or `archo db backup`).
- **Tests**: `internal/selfupdate` uses `httptest` for release reading and channels, version comparison, Workshop description stamps and tags, local staging, checksum format, download interruption, hash/size mismatch, ZIP paths, replacement and rollback (Nth rename failure), renaming running executables on Windows, and `VersionOf`. `internal/app/selfupdate_test.go` uses fake GitHub and Workshop metadata, synthetic Steam libraries and a fake SteamCMD runner for source selection, fallback, anonymous login, success confirmation, cleanup and one notification across sources; it also covers waiting for other RimArcho processes, verification failure, interrupted download, rollback when the new program cannot start, stopping while waiting, and single installation. Actual executable replacement is tried only on self-built copies on the host; old-to-new upgrades are tested in the VM (release VM test table).

## 6. Game behavior notes (facts confirmed by decompilation)

Basis: ilspycmd decompilation of 1.7.4982 `Assembly-CSharp.dll`, covering `Verse.ModsConfig`, `ModLister`, `ModMetaData`, `ModMetaDataInternal`, `ModContentPack`, `LoadedModManager`, `PatchOperation*`, `DirectXmlLoader`. Items marked "measured" were verified in a real environment.

**Mod registration and ModsConfig.xml**

1. `ModLister.RebuildModList` calls `TryAddMod` in order: official `Data/` → `Mods/` → **subscribed** Workshop entries. Within `Mods/`, registration follows folder-name order, so `!` folders come first (pack links use `!ArchoPack_`, 09 §3).
2. The game enumerates only **subscribed** Workshop entries; folders left after unsubscribing are not loaded (measured: several exist locally). RimArcho considers an entry subscribed if it appears in ACF `WorkshopItemsInstalled` or `WorkshopItemDetails` (04 §1.2).
3. **`_steam` rule**: if packageId duplicates an existing entry from another source and the new entry is a Workshop copy, set `appendPackageIdSteamPostfix`, making its ID `<pkg>_steam`; save that suffixed ID. Duplicates from the same source produce an error and are ignored. Reading: match full ID, then folder name; if still unmatched and suffixed `_steam`, strip it and retry. If one unsuffixed ID is enabled twice, disable the later entry. (In-game verification pending: coexistence of local and Workshop copies with the same package ID.)
4. If `ModsConfig.version` major/minor differs from the game, **reset the whole enabled list** (measured: player's 1.6 list with 1.7 game). RimArcho therefore always writes the current game version (07 §4.1).
5. Derived packageId (when absent in About.xml, `ModMetaDataInternal.TryParsePackageId`): `ConvertToASCII(author + first 3 characters of description hash) + "." + ConvertToASCII(name)`; default `author` is `Anonymous`; description hash uses `GenText.StableStringHash` (UTF-16, wrapping int32 overflow). Official content without packageId uses `ludeon.rimworld` for Core, `ludeon.rimworld.<folder>` otherwise (04 §1.5).
6. Official DLC About.xml has no `<name>`; display name comes from the ExpansionDef label, the folder name (for example, "Royalty").
7. Log Mod prefix `[Name]` uses About.xml's name, not RimArcho's alias.
8. Player.log: `<user data>/Logs/Player.log` in 1.7+, user data root in earlier versions (03 §3.3).

**Def and Patch (behavior XPath simulation must honor, 05 §5.2)**

9. Ignore comments and whitespace while parsing XML (`IgnoreComments` / `IgnoreWhitespace`).
10. Merge all Mods' Defs first, then execute Patches in load order. Thus "defined by a later Mod" does not cause a miss; `target-later` means "created by a later Patch".
11. Resolve ParentName inheritance after Patches (Mods use AttributeAdd to add `Name` to Core Defs).
12. A Def with `MayRequire` / `MayRequireAnyOf` is skipped if the relevant Mod is not enabled.
13. Only `li` in a `PatchOperationSequence` list honors `MayRequire`; it has no effect on top-level Operation.
14. `PatchOperationFindMod` matches Mod **display names** (DLC: item 6).
15. Tree operations on a selected attribute node throw an exception; count that operation as failed.
16. Failure descriptions in logs are the operation's `ToString()`, such as `Verse.PatchOperationAdd(xpath)`, `...Sequence(count=N, lastFailedOperation=...)`; compare by "file path + description", one entry per failed top-level operation.
17. Treat LoadFolders `IfDLCActive` / `IfDLCNotActive` as `IfModActive` / `IfModNotActive`, stripping `_steam` from package IDs (05 §1).

**Other**

18. Steam verification or updates overwrite Workshop files, so RimArcho never writes marker files there. Create a locked version before modifying a Mod (07 §5).
19. Game argument `-savedatafolder` gives packs their own save and configuration directories (10 §3); whether Player.log follows awaits measurement.
20. **Save class names** (1.6.4633 `Verse.GenTypes`, `Scribe_Deep`): a save writes `Class="…"` when an `IExposable` object's runtime type differs from the declared one, using `GetTypeNameWithoutIgnoredNamespaces`: a generic type as `Type.ToString()` (`Verse.Pair`2[Verse.Thing,System.Int32]`), a type in an ignored namespace (`RimWorld`, `Verse`, `LudeonTK`, `Verse.AI`, `Verse.AI.Group`, `Verse.Sound`, `Verse.Grammar`, `RimWorld.Planet`, `RimWorld.BaseGen`, `RimWorld.QuestGen`, `RimWorld.SketchGen`, `System`; `store.SaveNamespaces`) by its simple name, a nested one by its own name only, and any other by its full name with `+` for nesting. Loading (`GetTypeInAnyAssembly`) tries, in order: the simple name among all types of the global or an ignored namespace (case-sensitive); the name as given (case-insensitive) in `Assembly-CSharp`, then each loaded Mod assembly, then `Type.GetType` (the calling assembly and mscorlib); each ignored namespace prefixed; a generic built from its parts. No other game DLL is searched, `Assembly-CSharp-firstpass` included. Used by 05 §5.3 and 12 §4.6.

## 7. Engineering notes

### 7.1 Dependency pinning

- **Wails pinned to v3.0.0-beta.28** (v3 is still beta). Verify upgrades separately, including minimized memory targets (§7.3). This reads Wails private fields by name; layout changes cause a harmless no-op, but memory savings cease.
- **charmbracelet/x/ansi** is at v0.11.4 and lipgloss at a v1.1.1 pseudo-version, both raised by Wails through minimal version selection. v0.11.9 is incompatible with bubbletea v1.3.10 cellbuf; do not use it. `clipperhouse/displaywidth` is at v0.11.0, which compiles with uax29 v2.7.0.
- **saferwall/pe**: returns raw coded indexes; decode tag bits yourself. Heaps map into the file; copy before `Close`. Treat all Harmony enums as int32 (they are all int32).
- **MCP SDK**: tool argument structures require direct fields; embedded structures are not flattened (14 §3).
- **go-git**: fetching local paths (`file://`) uses an in-process implementation when `git-upload-pack` is absent; tests need no system Git. It does not support recursive submodule updates or LFS (11 §1 item 10).
- **SteamCMD location**: subscriptions request paths only through `App.FindTool(name) (path, ok)` (11 §1 item 2), supplied by dependency management in settings; tests replace it with a fake.

### 7.2 Terminal and Windows pitfalls

- **East Asian ambiguous width**: `–`, `…`, `·` occupy 1 cell in Windows Terminal. If locale or `RUNEWIDTH_EASTASIAN` enables runewidth's `EastAsianWidth`, alignment breaks; disable it. Nerd Font private-use code points also have ambiguous width; count them as 1 cell.
- With **piped input**, bubbletea decodes UTF-8 byte by byte, so end-to-end tests inject Chinese keystrokes using `p.Send`; real-console Chinese input requires manual validation.
- **GUI**: the `[hidden]` stylesheet rule and the service method names are in 16 §8.2; the CSP and the absence of TCP ports in 16 §8.3.

### 7.3 Memory measures

- **TUI process** (`tui.TuneProcess`, first line of `tui.Run`): `debug.SetMemoryLimit(40 MiB)` soft limit, GOGC 50, sole TUI connection `PRAGMA cache_size=-256` (256 KiB page cache). All scans run in children, never in the main process.
- **On-demand loading**: `store.Mods` omits `decl_blob`; `Mod.Decl()` reads it on demand; sorting and analysis batch-load through `Store.LoadDecls`.
- **Caching**: `app.Loadout()` and `app.Problems()` cache by `Store.ChangeKey()` (`PRAGMA data_version` + `total_changes()`) + environment ID + ModsConfig.xml size/mtime. Concurrent executables detect each other's changes through `ChangeKey`. `app.Index()` and link checks cache by `Store.IndexKey()`; the index also includes environment ID. `store.Open` installs the shared `archo_index_gen` counter table and persistent triggers, advancing generation only on environment, source, lineage, instance, snapshot, lineage_data and lineage_tag changes. List edits do not invalidate other connections' indexes. `OpenSession` installs no triggers and writes nothing. Shared indexes (`ReadIndex`) retain their source generation, avoiding marking stale data current after a concurrent commit. Link checks read all environments once, caching only IDs, source labels, relative paths, real paths and status; shared Instances are not loaded twice.
- **Lazy log parsing**: `App.LazyLog` makes TUI startup status read only the stored attribution report (`App.LogCacheOnly`); opening findings parses new logs. `status` and `mod` (MCP `status`, `get_mod`) never parse logs.
- **Scanning**: one goroutine per source, up to NumCPU workers per source. IDs are assigned deterministically during reconciliation, independent of concurrency. Incremental scans load only hashed semantic-file rows. If a folder is unchanged and not rehashed, release its manifest after reading. If only external state (such as ACF manifest) changes, copy file rows from the old snapshot to the new one with SQL.
- `PRAGMA shrink_memory` and closing idle connections bring no benefit. Idle `FreeOSMemory` is kept (timing: 16 §2.16.1 M01; about 5 MB less early in idle).
- **GUI process** (`gui/memory.go`): `debug.SetMemoryLimit(256 MiB)`, `PRAGMA cache_size=-256` per connection. The soft limit leaves room for concurrent read allocations: with 5183 real Mod library rows and reused WAL readers the resident heap is about 100 MiB, so 128 MiB still causes frequent GC during page rebuilds, and 32 MiB keeps the heap at the limit during a refresh (one list edit triggers tens to hundreds of GCs and runs 2–3 times slower). Go does not return memory by itself while the window idles, so `FreeOSMemory` runs, debounced (16 §2.16.1 M01), after `Board`, `Library`, `Needs`, log and texture reads, scans and leaving a page. `Library` is kept, invalidated by the database ChangeKey, so that returning does not rebuild it, until its reader expires (below). Leaving Can it run or Game log drops the main App's log reports and findings; independent log sessions keep their own change-aware cache. Leaving Launch drops each plan's findings. Leaving Texture optimization cancels unfinished statistics. Pages call `Leave` in `openScreen` / `closeScreen`. Measured: 18 §2.
- **Loaded state and slow reads**: `s.board.St`, `s.lib.St` and the main App cache share the index, copying only enabled lists and workspaces. Logs use an independent App / SQLite connection and mutex, preserving pending cross-version analysis. Texture reads also use an independent connection, but capture targets from the main App with only names, references and directories, without reloading the full index. Hold the shared API lock only while capturing inputs and publishing results. Texture statistics use at most 8 workers (2 on an HDD, `fsscan.Workers`), each keeping per-Mod summaries. Cache PNG headers in `cache/textures/` by path, size and mtime; page summaries in `summaries-v1.json`, reused by texture metadata fingerprint, options and DDS records, at most one atomic write per page. Still enumerate directories and check sizes/mtime to detect unscanned external changes; conversion and restoration always revalidate files.
- **Page reader expiry** (16 §2.16.1 M07): each page reader keeps its own connection, list state, problems and caches; index contents, list registries and game links are shared (below). The Mod library, library copy details, All lists, Mod details, cold-index, texture and uninstall readers close after 5 unused minutes; the library reader also drops `Library`. The editor/sidebar reader and the log reader stay: `s.board` and saved needs are their objects. Never close a reader whose objects are published. Index Mods do not depend on the reader that built them: their lazy `decl_blob` reads use the declaration connection (shared index contents, below).
- **Shared read-only caches** (`app/shared.go`): sessions share list registries by a 128-bit fingerprint of their candidates and game-link instances by database path and shared index generation (`i<token>.<n>`; a connection's own fallback key is not shared). Both are never changed after construction; `Registry.Given` is capacity-clipped, so appending copies. Entries are weak pointers: they live only while a session uses them. A moved managed directory, which the index generation does not count, calls `forgetLinks`.
- **Shared index contents** (`App.ReadIndex`): a page session takes a warm index as its own `Index` (environment copy, session for foreign-list lookups) over the source's Mods, maps, lineages and sources, rather than a copy of about 8 MB per session at 6,574 copies. Nothing changes them after `readIndex` (checked with a type-aware scan of every field write; only constructors and the store's scans write); `readIndex` clips the slices so an append copies. The one lazily filled field is the declaration: `Mod.lazy` and `Mod.store` are atomic, and `ReadIndex` binds the Mods to the declaration connection, one `OpenSession` per opened data directory, shared by its read sessions and closed by the App that opened it (`declReads`). Snapshot rows never change `decl_blob`, so any connection reads the same declaration. Declarations loaded by any session stay with the shared Mods until the index generation changes. An App not from `Open` has no declaration connection; a reader then loads its own index. Copying a `store.Mod` by value is a `go vet` error (copylocks).
- **Page DOM cache** (`JS/screens.js:openScreen`): only warm pages (`WARM`, Can it run) keep their last drawing; other pages show their skeleton on return, including during a game switch. The Mod library and Search files keep their own element references.
- **Launch-page memory refresh** (16 §2.16.1 G19): `HomeMemory` reads on the Launch page's log reader with the current list (`OpenPlan = 0`, as `Home`), so the 30-second refresh hits the cached list state. A new read session per call would rebuild the whole index and list each time and keep the idle window's Go memory rising (18 §2).
- **Slow reads on other pages**: Launch and Can it run reuse the independent log session; the sidebar and the list editor share one read session; the Mod library and All lists keep their own connections and caches. An independent reader coalesces cold-index queries; page sessions share its index contents (shared index contents, above) but own their list state, problems and caches. Pin a short-lived WAL read view during page construction to avoid repeated state rebuilding under consecutive list commits; release before publication. Launch reads list state directly without constructing all editor library rows. Main App stays in cache-only mode; visiting these pages does not enable synchronous log parsing for later requests. Changes, technical details, settings, environments, pack list/details/build preview, game detection and uninstall preview use short-lived independent connections, closed after constructing views; mutations remain serialized by main App. Environment pages read configuration only, not Mod indexes. Background discovery captures small inputs (official/background Mod names, directories) from existing indexes, accesses files outside the lock, then validates the environment on publication. Do not simply remove locks on one App or retain a second full-library cache for a small settings page.
- **Package-name validation** avoids regex: `^[\p{L}\p{N}][\p{L}\p{N} ._\-]{0,63}$` copies two Unicode classes 63 times, retaining about 1.4 MB.
- **WebView2 switches** (`gui.BrowserArgs`, through `application.WindowsOptions.AdditionalBrowserArgs`; `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` is ineffective under Wails): always `--enable-features=NetworkServiceInProcess2 --renderer-process-limit=1`, merging network service into the browser and removing a roughly 7 MB process (page is offline, 16 §8.2; only one page, so renderer cap changes nothing except preventing extras). With GPU enabled, idle savings about 6 MB without visible differences. `ui.gpu off` (16 §2.14) adds `--disable-gpu --disable-gpu-compositing --in-process-gpu`: removes the GPU process, saving another 15 MB or so idle and 20 MB in the Mod library; the CPU draws the page.
- **Minimization and background** (`gui/winmem_policy.go`, `gui/winmem_windows.go`): `ICoreWebView2_19::put_MemoryUsageTargetLevel` sets `LOW` and `NORMAL` at the times given in 16 §2.16.1 M08; `js/activity.js` reports pointer, wheel and key input through `PageInput`. Low keeps a visible window drawing: on the host a visible inactive window under low still showed its page, and opening the Mod library drew in full (page process 102 → 15 MB idle, 83 MB after opening the library). Restore counts as active because Windows activates a window while it is still minimized and Wails reports no `WindowActive` for that; wrongly assuming active only keeps normal memory. A quick switch away and back changes nothing. With a WebView2 runtime that lacks the interface (before 1.0.2210) the call is skipped. Wails beta.28 does not expose it: its internal `ICoreWebView2_19` binding lists only its own two methods after IUnknown, so calling through it would call `get_Settings`. RimArcho calls the vtable slot directly (IUnknown 3 + ICoreWebView2 58 + _2…_18 together 58, + 1 = 120) and reads the WebView2 pointer from Wails' private fields by name (`WebviewWindow.impl.chromium.webview`). This saves about 45 MB (WebView2 129 → 81 MB). `TrySuspend` would also need a COM callback; not implemented.

### 7.4 Analysis implementation

- Packages: `internal/xdoc` (arena XML store and NodeNavigator), `internal/content` (effective content), `internal/analyze` (Def facts, Patch simulation, findings), `internal/finding`, `app/analysis.go` and `app/problems.go` (inputs, storage, freshness, declaration layer).
- **Arena**: 44-byte node structures, allocated in blocks of 64K (first grows like a slice to keep small documents small); int32 links; names interned in a `Names` table shared by merged and Patch documents; text in 1 MiB append-only pages, separate allocations above 64 KiB. A growing text buffer would keep every old backing array alive through strings that index keys reference (about 226 MB).
- **Parsing pipeline**: up to 8 workers turn files into events with `encoding/xml`, retaining at most twice the worker count in files; build documents sequentially in load order.
- **defName/@Name index fast path is mandatory** (otherwise the 30-second target is unreachable); invalidation rules: `spikes/xpathcompat/index.go`.
- **Child process**: `debug.SetMemoryLimit(300 MiB)`.
- **Freshness**: `AnalysisKey` hashes analyzer version, game major/minor, each included Instance's ID, snapshot hash and position. User/community rules do not enter the key (they affect only the dynamically computed declaration layer). Included Instances = enabled list + all live, loadable, disabled Instances (the latter build only Def facts as `target-inactive` providers).
- **Storage**: analysis slots, content sets and reference cleanup: 05 §3 (`store.SaveAnalysis`, `store.PruneFacts`); log attribution reports and file reference indexes: 06 §5.
- **Per-plan analysis caching**: results are cached by the fingerprint of the enabled set, several at a time; switching plans does not evict the others (05 §3).

### 7.5 Other

- GUI performs scheduled database maintenance in an independent session after the first screen. Retention, collection and scheduling: 04 §4.1–§4.2. Synthetic regressions cover identity evidence, history degradation, insufficient disk, busy, cancellation, background tasks and running-game protection. Manual compaction uses the same flow.

- When several locations resolve to one real directory, choose primary by "already an Instance location → not a link → source reconciliation order → source order"; record others as aliases. Do not follow links inside Mod directories; exclude `.git`, `.svn`, `.rimarcho` from fingerprints (03 §4.3, 04 §1.4).
- Never insert external Workshop description content as HTML; serve images only from local cache through `/img/` (16 §8.3).
- `ARCHO_MEMSTATS=1` prints process memory before exit (measurement).
- `app.ChildAnalyze` refuses to launch a child inside a Go test binary; otherwise tests would relaunch themselves recursively.
- File search (05 §5.5) does not invoke `rg.exe`: pure Go. Literal search first uses ASCII-case-folded `bytes.Index` on whole files, calculating line numbers only on hits; regex uses RE2 with literal-prefix prefiltering. About 5–6 thousand files and 150 ms per search, within 500 ms budget, so no trigram or defName index (snapshot invalidation maintenance outweighs benefit). Def-definition and Patch relationships have separate analysis indexes (`get_def`). Directory and file reads use `modsearch.FS`, allowing a cached walker later.
- UI screenshots use Win32 `PrintWindow`, without bringing the player's window forward or capturing other windows.
