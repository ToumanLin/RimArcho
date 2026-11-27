# Linux, module by module (2026-11-22)

The [2026-11-09 Linux plan](2026-11-09_linux-support-plan.md) finished its phase 1: since 1.0.9 a static `archo` (CLI, TUI, MCP) ships as an experimental tarball (17 §5). This report takes stock on `dev/1.1` at `5482ea14`, lists every platform-specific piece and what Linux does with it, records a first build of the GUI on Linux, and orders the remaining work into modules so that 1.2 can ship an experimental Linux GUI. 1.1 ships no Linux GUI (decided). **[C]** marks facts checked in the code or by a build or run on 2026-11-22, **[I]** inferences still to confirm.

## Summary

- **The non-GUI side compiles and is tested only on synthetic data.** The 68 packages Linux CI selects build and vet with `GOOS=linux CGO_ENABLED=0` **[C]**. None of the twelve S12 rows in the [release VM tests](../../UserGuide/release-vm-tests.md) has been run against a real game; that, not code, is the largest gap in what already ships.
- **The GUI builds and runs on Linux.** On Ubuntu 24.04 (WebKitGTK 2.52.6), `CGO_ENABLED=1 go build -tags gtk3 ./cmd/archo`, with only the two build-tag lines in `surface/cli/gui_*.go` changed, produced a window that showed the first-run guide, the Launch page, the Mod library and Mod details correctly, with Chinese text and the animated Archo **[C]**. No RimArcho Go code failed to compile.
- **Five runtime problems block a preview:** a blank page under the DMA-BUF renderer (in the VM), the window larger than the screen, a WebKit crash when the process gets SIGTERM, no Linux progress or error window (`upgradewin` is a no-op, so a GUI started from a desktop entry fails silently), and a 65 MB heap spike from drawing the 256 px window icon at every start (§1, §4.2).
- **Two decisions come first:** GTK 3 (`-tags gtk3`, WebKitGTK 4.1) or Wails' default GTK 4 (WebKitGTK 6.0); and allowing cgo for one Linux GUI binary while every other build keeps `CGO_ENABLED=0` (17 §1 and `TestArchNoCgo` forbid it today) (§6).
- **Recommended 1.2 milestone (§5):** a separate `rimarcho` GUI binary in the Linux tarball, built by a Linux CI job, with the five blockers fixed and the S12 rows run once. Packages, Steam-client subscriptions, account logins and Archo Agent stay out.

## 1. What was checked

### Cross-compiling on the Windows host

| Command (`GOOS=linux GOARCH=amd64`) | Result |
| --- | --- |
| `CGO_ENABLED=0 go build ./...` | Fails only at `spikes/analyzereal`: `spikes/internal/procmem` has no Linux file. Product code builds **[C]** |
| `CGO_ENABLED=0 go build ./cmd/...`; vet on CI's 68 packages | Pass **[C]** |
| `CGO_ENABLED=0 go vet ./internal/surface/upgradewin`; `go test -c` of it | Pass: it has `_other.go` stubs, so CI's exclusion of it is no longer needed **[C]** |
| `CGO_ENABLED=0 go vet ./internal/surface/gui` | Fails in Wails: `menu_linux.go:7:12: undefined: pointer` (its Linux code needs cgo) **[C]** |
| `CGO_ENABLED=1 go vet ./internal/surface/gui` | Fails: no C cross-toolchain on the host. The GUI has to be built on Linux **[C]** |
| Importers of Wails | Only `internal/surface/gui` (and `spikes/gui-v0`); `cmd/archo` without the GUI build tag links no Wails **[C]** |

### Building the GUI in the Linux-Test VM

Snapshot `Linux-Clean-Steam`, synthetic data only (an empty `HOME`, `XDG_*` and `RIMARCHO_HOME` under `/tmp/rh`; no Steam, no SteamCMD, no real game). Added in the guest: Go 1.27.2, `gcc`, `pkg-config`, `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`. The VM was reverted afterwards.

| Step | Result |
| --- | --- |
| `CGO_ENABLED=1 go vet ./internal/surface/gui` | Fails at `pkg-config`: Wails beta.28 asks for `gtk4` and `webkitgtk-6.0` by default **[C]** |
| Same with `-tags gtk3` | Passes, including `upgradewin` (1 min, warm cache) **[C]** |
| Build tags `windows \|\| (linux && cgo)` in `gui_enabled.go` and the negation in `gui_disabled.go` (guest copy only), then `go build -tags gtk3 -trimpath -ldflags "-s -w" ./cmd/archo` | Builds in 2 m 27 s cold. Binary 59.7 MB (the static `archo` is 32.7 MB). Links `libwebkit2gtk-4.1.so.0`, `libgtk-3.so.0`, `libsoup-3.0.so.0`, `libjavascriptcoregtk-4.1.so.0`; highest glibc symbol `GLIBC_2.34` **[C]** |
| `archo gui`, empty home | A window with GNOME's title bar opens; Wails logs `Webkit2Gtk=v2.52.6`; all page files load; no JavaScript error (an injected `error`/`unhandledrejection` reporter stayed silent). **The page stays blank** **[C]** |
| Same with `WEBKIT_DISABLE_DMABUF_RENDERER=1` | The first-run guide draws: Steam detection finds no game in the empty home, Archo's animated figure plays **[C]** |
| Synthetic game (`Version.txt`, empty `RimWorldLinux`, `testdata/mods` Core and three Mods), `archo env add synth --game … --launch direct`, `archo scan`, then the GUI | The Launch page, Mod library (table, column header, size bars), Mod details (copy buttons, links, tags) and the sidebar's subpages draw as on Windows; `Versioned Mod 版本` renders. Super+Up maximized the window to the 1280×800 screen **[C]** |
| Window size | Wails opens 1440×880 on a 1280×800 screen, partly off-screen **[C]** |
| `kill <pid>` (SIGTERM) on the GUI | The `WebKitWebProcess` child died with SIGTRAP; apport saved a crash report and showed GNOME's "Report a problem" dialog **[C]** |

**Memory**, synthetic 3-Mod home, llvmpipe software GL (no GPU in the VM), after the Launch page and the Mod library; `/proc/<pid>/status`:

| Process | VmRSS | RssAnon |
| --- | --- | --- |
| RimArcho (Go + GTK + the WebKit UI side, one process) | 196–211 MB | 40–50 MB |
| `WebKitWebProcess` (page) | 193 MB | 49 MB |
| `WebKitNetworkProcess` | 50 MB | 8 MB |

- `GODEBUG=gctrace=1`: Go's live heap is 4 MB at rest. The first forced GC at 0.6 s shows `65->65->3 MB`: on non-Windows `windowIcon()` draws `icon.PNG(256)` with 4× supersampling on every start (16 §8.6 draws 32 px on Windows) **[C]**.
- One earlier run of the empty home showed RssAnon 227 MB in the Go process, nearly all in one 208 MB anonymous mapping outside Go's heap; three later starts did not repeat it. Cause unknown (Mesa llvmpipe or JavaScriptCore are candidates) **[I]**.
- VmRSS counts shared library pages (`libLLVM`, `libwebkit2gtk`), so it is not Task Manager's private working set (18 §1). RssAnon is the nearer measure; Pss would split shared pages fairly. With a real GPU the numbers will differ; these are not baselines.

## 2. Inventory of platform-specific pieces

Status on Linux today: **works** (implemented and tested on synthetic data), **real** (also seen working on a real Linux system), **stub** (compiles, does nothing or reports unsupported), **message** (says it is not available), **missing** (no Linux behaviour; the feature silently does less).

### Non-GUI

| Piece | Code | Windows | Linux today |
| --- | --- | --- | --- |
| Data directory | `env.AppHome` | `%APPDATA%\RimArcho` | Works: XDG (17 §3) |
| Managed root (`LocalDir`) | `app/uninstall.go` | `%LOCALAPPDATA%\RimArcho` | Works: `$XDG_DATA_HOME/rimarcho-local` |
| Steam roots, libraries, native / Proton / Flatpak / Snap games, user data | `env/detect.go`, `layout.go` | Registry + libraries | Works (03 §3.2); S12 rows not run |
| Steam signed-in user | `env/steamuser.go`, `path_other.go` | Registry | Works: `.steam/registry.vdf` |
| Launch | `app/launch.go` | Direct or Steam | Works: native direct or Steam; Proton only through Steam; own saves under Proton refused (09 §2) |
| Running game, busy tools | `gamerun_linux.go`, `toolbusy_linux.go` | Process snapshot | Works: `/proc`, Wine path mapping |
| Other RimArcho processes (uninstall, self-update wait) | `uninstall_other.go` `archoProcesses` | Process snapshot | **Stub**: returns none |
| Process start time (PID reuse check for tasks) | `taskfile_other.go` `processStart` | `GetProcessTimes` | **Stub**: time 0, liveness by `kill 0` only |
| Database writer liveness | `store/process_other.go` | `OpenProcess` | Works: `kill 0` |
| Same-volume moves, `EXDEV` | `source/samevolume_other.go` | Volume names | Works: `st_dev` |
| Recycle bin | `source/trash_other.go` | Shell `IFileOperation` | Works: freedesktop trash, home and per-volume |
| Links | `source/link_other.go` | Junctions | Works: symlinks |
| Game-link lock, source-library lock, task-file lock | `linklock_other.go`, `sourcelock_other.go`, `taskfile_other.go` | Windows locks | Works: `flock` |
| Self-update lock | `selfupdate_other.go` `lockUpdate` | Exclusive handle | **Stub**: in-process only; harmless because Linux never installs (D2) |
| Account-session lock | `accounts/lock_other.go` | Named mutex | **Stub**: no lock; harmless while sessions are unsupported |
| Self-update | `app/selfupdate.go`, `updatenotice.go` | Download, swap, verify | Works as designed: update notice only (D2) |
| Uninstall | `uninstall_other.go` `startAfterExit` | cmd script after exit | Works: deletes at once (a running file can be unlinked) |
| SteamCMD | `deps`, `gamedl.go`, `steamcmdarchive.go`, `console_other.go` | `steamcmd.exe` | Works: Valve tarball, own `HOME`, process groups (11 §3); real run is an S12 row |
| Steam-client subscribe, Workshop publish | `steamworks/*_windows.go` (`steam_api64.dll`) | Steamworks worker | **Message**: SteamCMD only (plan phase 4) |
| todds | `textures/todds_other.go` | Managed download | Works: native `todds` from settings or `PATH`; never downloads; instructions to build |
| ilspycmd | `deps` | `dotnet tool` | Works [I]: same discovery, `.cmd` launcher test is Windows-only |
| Saved secrets (Archo Agent key) | `secret/secret_other.go` | Credential Manager | **Stub**: `ErrUnsupported` |
| Account sessions (LoversLab, browser login) | `accounts/protect_other.go`, `browser_other.go` | DPAPI + WebView2 login window | **Message**: `ErrSession`; manual zip as decided |
| Archo Agent | `app/agent_other.go` | Console window with Pi | **Message**: "only built for Windows" |
| System proxy | `netcfg/system_other.go` | WinINet settings | Works partly: `HTTPS_PROXY`/`HTTP_PROXY`; no desktop (GNOME/KDE) proxy |
| System language | `l10ndata/lang_other.go` | UI language | Works: `LC_ALL`, `LC_MESSAGES`, `LANG` |
| RAM, process memory | `machine/memory_linux.go`, `procmem_linux.go` | `GlobalMemoryStatusEx`, PSAPI | Works: `/proc`; VRAM unknown |
| Disk kind (HDD → fewer workers) | `fsscan/workers_other.go` | `IOCTL_STORAGE_QUERY_PROPERTY` | **Stub**: unknown, always SSD settings |
| Log creation time | `logtime_other.go` | File creation time | **Stub**: zero (Linux has `statx` birth time) |
| Other managers' folders (import from RimSort, RimPy) | `managerdiscovery_other.go` | Running processes and shortcuts | **Missing**: none found; RimSort's own Linux paths are not looked at |
| Documents folder | `uninstall_other.go` `documentsDir` | Known folder | Works partly: `~/Documents`, not `xdg-user-dir DOCUMENTS` |
| Hidden child windows | `hide_other.go` (app, deps, selfupdate) | `CREATE_NO_WINDOW` | Works: nothing to hide |
| Detached pack builds | `packbuild_other.go` | Detached process | Works: `Setsid` |
| Open URL / folder (TUI) | `tui/detail.go`, `shortcuts.go` | `rundll32`, `explorer.exe` | Works: `xdg-open` |
| Archo Monitor (in-game Mod) | `monitor/` | — | Platform-neutral C#, no P/Invoke **[C]**; Linux game run is an S12 row |
| Release | `scripts/release.ps1`, `releasetar` | ZIP | Works: static tarball, built on Windows |
| CI | `.github/workflows/ci.yml` | Full | Works: `ubuntu-latest`, cgo off, excludes `gui`, `upgradewin`, `spikes` |

### GUI (`internal/surface/gui`, `upgradewin`, `surface/cli`)

| Piece | Code | Windows | Linux today |
| --- | --- | --- | --- |
| GUI startup | `cli/gui_enabled.go`, `gui_disabled.go` | Wails + WebView2 | **Message** ("this build has no window"); compiles and runs with cgo + `gtk3` (§1) |
| Two executables | 16 §8.1 | `archo.exe` console, `RimArcho.exe` `-H windowsgui` | **Missing**: only `archo`; `archo gui` hands over to a sibling only on Windows |
| WebView2 preflight, Wine message | `accounts.WebView2Installed`, `machine.Wine` | Message box | `WebView2Installed` returns true; no WebKitGTK check |
| Database-upgrade progress and startup error boxes | `upgradewin` | Win32 window, message box | **Stub**: `Open` returns nil, `ShowError` does nothing |
| Frameless window, custom title buttons, Snap Layouts, composition hosting | `gui.go`, `window_theme_windows.go`, `window-chrome.js` | Yes | Not used: GTK title bar, as 16 §8.2 says for other platforms **[C]** |
| Window theme (title bar colours) | `window_theme_other.go` | DWM attributes | Stub: follows the GTK theme |
| Window icon | `winicon_other.go` | Resource group 3 | Works, but draws 256 px at every start (65 MB heap spike, §1) |
| Memory target on minimise / background | `winmem_windows.go` | `ICoreWebView2_19` | **Stub**: none (WebKitGTK has memory-pressure settings, not exposed by Wails) |
| WebView2 switches, `ui.gpu off` | `BrowserArgs` | `AdditionalBrowserArgs` | **Missing**: no effect; Wails' Linux `WebviewGpuPolicy` could serve `ui.gpu off` |
| Web data folder | `WebviewUserDataPath` | `<data>/webview2` | **Missing**: WebKitGTK picks its own XDG folder **[I]** |
| Files and links dropped on the window | `drop_windows.go` (OLE) | Yes | **Stub**: `wireNativeDrop` does nothing; Wails Linux has `EnableFileDrop` |
| Session end during uninstall | `uninstall_windows.go` | `WM_QUERYENDSESSION` block | Stub |
| Open URL, open folder | `gui.go:213`, `library.go:275` | `rundll32`, `explorer.exe` | Works [I]: `xdg-open`; no "reveal and select" |
| Volume label in Game Config | `shell.go:581` | `filepath.VolumeName` | **Missing**: empty on Linux (the old plan's remaining GUI call site) |
| SteamCMD login from the GUI | `console_other.go` | New console window | **Missing**: no terminal to type into (needs a terminal emulator or the `archo` command shown) |
| Context menu, mouse back/forward | `nativemenu.js`, `navigation.js` | Tested | Untested [I] |
| File dialogs (import/export) | `exchange.go` via Wails | Win32 dialogs | Untested [I]: Wails uses GTK dialogs |
| Clipboard copy buttons | page script | Tested | Untested [I] |

## 3. Modules in dependency order

Each module can merge on its own. Effort: **S** up to two days, **M** up to a week, **L** more. "VM" means the Linux-Test VM (Ubuntu 24.04, native game and a Proton copy; see the handoff note), real data only there (17 §4).

### L0 · Build and CI foundation (S)

- **Gaps:** CI excludes `upgradewin` though it builds; no job builds the GUI; `TestArchNoCgo` and 17 §1 forbid cgo everywhere; `spikes/internal/procmem` breaks `GOOS=linux go build ./...`.
- **Work:** drop `upgradewin` from the exclusion; add a `linux-gui` job on `ubuntu-22.04` (oldest glibc and WebKitGTK I target) with `CGO_ENABLED=1 -tags gtk3`, `apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev`, vet and build of `cmd/archo` plus the GUI package's Go tests; give the spike a `_other.go` or a build tag. Scope the cgo rule: allowed only behind the Linux GUI build tag (decision D7, §6).
- **Tests:** the new CI job; `TestArch*` with the scoped exception.
- **Player gets:** nothing visible.

### L1 · Real-game validation of what ships (M)

- **Gaps:** none of the S12 rows has run: native and Proton detection and launch, Flatpak, process guards, cross-mount moves, case-only saves, trash, log attribution, SteamCMD in its own home, todds guidance, memory probes, the tarball.
- **Work:** run S12 in the VM with `round`-style real games (launch, play, save, load, exit, log check); fix what it finds. Needs Linux versions of `scripts/vm/round.ps1` pieces (ssh instead of PowerShell Direct; Archo Drive is platform-neutral C#).
- **Tests:** VM only; each fix gets a synthetic regression test in CI.
- **Player gets:** the experimental CLI/TUI becomes trustworthy; release notes can say which rows passed.

### L2 · Process and file-system parity (S)

- **Gaps:** `archoProcesses` (uninstall and update waits), `processStart` time, `logCreationTime`, `storageKind`, `documentsDir`, Linux volume label in Game Config.
- **Work:** reuse the `/proc` reader of `gamerun_linux.go` for RimArcho processes and `/proc/<pid>/stat` start time; `unix.Statx` birth time with a fallback to mtime; `/sys/block/<dev>/queue/rotational` for disk kind; `xdg-user-dir DOCUMENTS` or `user-dirs.dirs`; mount point or label instead of `VolumeName`.
- **Tests:** synthetic `/proc` and `/sys` trees in CI (the pattern of `gamerun_linux_test.go`); one VM check of uninstall while a second `archo` runs.
- **Player gets:** uninstall and texture scans behave as on Windows; HDD libraries are not thrashed.

### L3 · Desktop integration (S)

- **Gaps:** RimSort and RimPy folders on Linux not found for import; GNOME/KDE proxy ignored; no "show in folder" that selects the file.
- **Work:** look in RimSort's Linux config folders and `.desktop` files; read the proxy from `gsettings`/`kreadconfig` only if players ask (environment variables already work); reveal through the `org.freedesktop.FileManager1.ShowItems` D-Bus call with `xdg-open <dir>` as fallback (pure-Go D-Bus, no cgo).
- **Tests:** synthetic config folders in CI; VM check with a real RimSort install.
- **Player gets:** import from RimSort finds its lists; "location" opens the folder with the Mod selected.

### L4 · Secrets and Archo Agent (M)

- **Gaps:** `secret` unsupported; Archo Agent refuses; account sessions (DPAPI) unsupported.
- **Work:** Secret Service over D-Bus (pure Go) for the Agent key, with a clear message when no keyring runs (SteamOS game mode, headless). Start Pi in the current terminal for `archo agent`, or in a terminal emulator from the GUI (shared with L7's SteamCMD login). Account sessions stay manual (decided); revisit only if the browser login moves to the WebKitGTK window.
- **Tests:** fake Secret Service in CI; VM run of the Agent with the model gateway.
- **Player gets:** Archo Agent on Linux.

### L5 · Native todds download (S, optional)

- **Gap:** players must build todds; the official Linux build needs `GLIBC_2.29`, `GLIBCXX_3.4.32` (GCC 13), so 22.04 cannot run it (old plan §3 item 12).
- **Work:** offer the official zip only when the system's `libstdc++` provides `GLIBCXX_3.4.32`; otherwise keep the build instructions.
- **Tests:** synthetic version probe in CI; VM conversion of a small Mod.
- **Player gets:** texture optimisation without building a tool, on newer distributions.

### L6 · Steam-client subscriptions (L, optional, old plan phase 4)

- **Gap:** `steamworks` loads `steam_api64.dll` with Windows calls.
- **Work:** load the game's `libsteam_api.so` with purego (no cgo) in `archo steam-worker`; publish stays Windows-only (maintainer).
- **Tests:** VM only (needs a signed-in Steam client; never SteamCMD with the same account).
- **Player gets:** subscribe and unsubscribe through Steam instead of SteamCMD.

### GUI modules (G1–G4, §4) and L7 · Packages (M)

L7 comes after G3: `.deb` and `.rpm` with nfpm, an AUR `PKGBUILD`, each writing `archo-install-kind` (17 §5.1). Tests: install, run, upgrade and remove in the VM (deb), a Fedora VM or container (rpm), an Arch container (AUR). Player gets: a package manager install and updates.

Dependency order: L0 → (L1, L2, L3 in any order) → G1 → G2 → G3 → (L4, L7) → G4; L5 and L6 any time after L1.

## 4. The GUI track

### 4.1 Build (G1, M)

- **GTK 3 or 4.** Wails beta.28 defaults to GTK 4 with WebKitGTK 6.0 and keeps GTK 3 with WebKitGTK 4.1 behind `-tags gtk3` **[C]**. GTK 3 is what built and ran here, and what Ubuntu 22.04 and Debian 12 certainly carry **[C for 24.04, I for the others]**. Recommend `gtk3` for 1.2 and a test build with GTK 4 once a newer Wails release is pinned (17 §7.1): GTK 4 is where Wails is going and where WebKitGTK's newest renderer work lands **[I]**.
- **Two binaries, as on Windows.** Keep the static `archo` (cgo off; SteamOS, headless and MCP users need no GTK) and add `rimarcho`, the GUI build, linked dynamically. `archo gui` hands over to a sibling `rimarcho` like it does to `RimArcho.exe`; `rimarcho` without a display prints the `archo` hint. Scan and analysis children keep running the GUI's own executable with command arguments, as `RimArcho.exe` does (16 §8.1).
- **Where it is built.** `release.ps1` runs on Windows with cgo off; the GUI must be built on Linux. Build `rimarcho` in a GitHub Actions job (L0) on `ubuntu-22.04` and download the artifact into the release, or build it in the Linux VM; the publish script (17 §5 step 7) gains one asset either way.
- **cgo exception.** 17 §1 says `CGO_ENABLED=0` everywhere and `TestArchNoCgo` checks no `import "C"`. My code needs no `import "C"`; Wails' Linux files do. The rule changes to: no cgo in RimArcho's packages, and only the Linux GUI binary is built with `CGO_ENABLED=1` (D7).

### 4.2 Runtime blockers (G2, M)

| Problem | Cause | Fix |
| --- | --- | --- |
| Blank page | WebKitGTK's DMA-BUF renderer fails in the VM's SVGA driver; Wails turns it off only for NVIDIA (`application_linux.go:35`) **[C]** | Set `WEBKIT_DISABLE_DMABUF_RENDERER=1` before Wails starts when no hardware renderer is available, or always while experimental; tie it to `ui.gpu off` with `WebviewGpuPolicyNever` |
| Window larger than the screen | Fixed 1440×880 | Clamp to the monitor's work area (Wails screen API), as Windows does through the system |
| SIGTERM crashes the page process | No signal handling; WebKit's child sees its parent vanish | Handle SIGTERM/SIGINT/SIGHUP as a window close (`Window.Close()`, the same `WindowClosing` path) |
| Silent failures | `upgradewin` is a stub | A small GTK window through Wails before the main window, or `zenity`/`kdialog` when present, else a desktop notification and `logs/gui.log` |
| Startup heap spike | `icon.PNG(256)` on every start | Ship PNG icons in the package (hicolor theme) and pass a small pre-drawn PNG; draw 256 px only at release time |

Also in G2: the WebKitGTK version check (log it, refuse below the floor in D4 with a message), the web data folder under `<data>/webkit`, `wireNativeDrop` through Wails' `EnableFileDrop`, the Game Config volume label (L2), and SteamCMD login from the GUI through a terminal emulator (`x-terminal-emulator`, `gnome-terminal`, `konsole`, `kgx`) or the `archo` command to copy.

### 4.3 WebKitGTK and my page

The frontend is plain HTML, CSS and ES modules (16 §8.2). A grep of `style.css`, `index.html` and `js/` for features whose support differs between Chromium (WebView2) and WebKit:

| Feature | Uses | WebKitGTK |
| --- | --- | --- |
| `color-mix()` | 74 | Supported since about 2.40 (Safari 16.2) [I] |
| `:has()` | 21 | Since about 2.42 (Safari 15.4 engine) [I] |
| `oklch()` / `oklab()` | 8 | Since about 2.40 [I] |
| `::-webkit-scrollbar` | 7 | Supported (it is WebKit's own) |
| `scrollbar-gutter`, `scrollbar-width`/`-color` | 6, 3 | Newer: around 2.46–2.48 [I]; degrade to default scrollbars |
| `startViewTransition` | 6 | Guarded by a feature test in `shell.js` **[C]**; 2.48+ [I] |
| `@container` | 2 | Since about 2.40 [I] |
| `content-visibility`, `text-wrap` | 1 each | Newer [I]; harmless when ignored |
| HTML `popover` API, `<dialog>` | 0 | — (my popovers are custom elements) **[C]** |
| Mouse buttons 3/4 (`navigation.js`), `auxclick` | 3 | X11 buttons 8/9 should arrive as 3/4 [I]: test |
| `contextmenu` cancel (`nativemenu.js`) | — | Needed: without the `production` tag Wails is in debug mode and WebKit's menu includes "Inspect Element" [I]: test |
| Named Windows fonts (Segoe UI, Microsoft YaHei, Consolas, Cascadia) | 5 | Fall back to the system font (Ubuntu, Cantarell, Noto); Chinese needs Noto CJK, which most desktops install. Widths differ: a visual pass over the static UI rules (16 §3.1 "Checks") is needed **[C: the fallback rendered acceptably on the pages above]** |

- Everything above rendered on 2.52.6. The question is the oldest WebKitGTK players have. Ubuntu and Debian ship WebKitGTK security updates to their stable releases, so 22.04 and Debian 12 should carry 2.4x–2.5x now [I: check `apt-cache policy` on each]. Set the floor at 2.44 (D4) and check it at startup.
- **No CDP.** `spikes/guidev`'s page checks, the memprobe harness and the jumpiness probe drive Chromium; they keep testing the shared frontend on any OS. Native Linux numbers need `WebKitWebDriver` or the Web Inspector's remote protocol (`WEBKIT_INSPECTOR_HTTP_SERVER`); decision D5 (manual at release) stands for 1.2.
- **Memory baselines.** 16 §10 is written in Windows terms: Go private working set ≤ 80 MB and Task Manager's total ≤ 200 MB. On Linux the GTK and WebKit UI side lives in RimArcho's own process, so "Go memory" cannot be separated from the toolkit. Proposal: on Linux measure the process tree's summed Pss (`/proc/<pid>/smaps_rollup`) after 10 idle minutes, report RssAnon of the main process beside it, and record them in 18 as Linux rows that do not gate releases while the GUI is experimental. First data point (§1): RssAnon ≈ 50 + 49 + 8 MB, with software GL. The 10-minute idle and 1,354-Mod numbers are G4 work.
- **Minimised and background memory** (M08): WebKitGTK offers `WebKitMemoryPressureSettings`; Wails does not expose it, so it needs cgo in my code or a Wails change. Leave M08 Windows-only in 1.2.

### 4.4 Packaging

| Format | 1.2 | Notes |
| --- | --- | --- |
| `.tar.gz` | Yes | `archo` (static), `rimarcho` (GUI), `rimarcho.desktop`, hicolor PNG icons, Monitor, docs, and an `install.sh` that copies to `~/.local/bin` and `~/.local/share/applications`. The README names the runtime packages: `libwebkit2gtk-4.1-0` (Debian/Ubuntu), `webkit2gtk4.1` (Fedora), `webkit2gtk-4.1` (Arch) [I] |
| `.deb`, `.rpm`, AUR | After 1.2 (L7) | nfpm; declare the WebKitGTK dependency; write `archo-install-kind` |
| AppImage | No | Bundling WebKitGTK means carrying its helper processes at fixed `libexec` paths; large and fragile (unchanged from D3) |
| Flatpak | No | Sandbox blocks Steam libraries, symlinks and `/proc` (unchanged) |

### 4.5 What "experimental Linux GUI" needs

1. G1 and G2 done; the GUI's Go tests pass in the Linux GUI CI job.
2. One VM session (a new S13 in the release VM tests): open every page on the synthetic 1,354-Mod home and on the VM's real native game; the list editor's drag, context menu, mouse side buttons, file dialogs, clipboard, drop of a `.archopack`; scan and analysis children; launch the native game from the Launch page; close during a scan; uninstall.
3. Release notes and README: experimental, which distributions were tried, the runtime package to install, the DMA-BUF note, how to report problems (the Feedback page works without the GUI too).
4. No promise of memory or speed targets on Linux; record the first numbers in 18.

## 5. Recommended first milestone for 1.2

**"Linux GUI preview"**: L0, G1, G2, L2, and L1's S12 run, then the tarball with `rimarcho`.

| Step | Effort | Exit check |
| --- | --- | --- |
| L0: CI job `linux-gui` on `ubuntu-22.04` with `-tags gtk3`; `upgradewin` back in Linux CI; scoped cgo rule (D7) | S | Job green on `dev/1.2` |
| G1: build tags `windows \|\| (linux && cgo)`, `rimarcho` binary, `archo gui` hand-over, release script takes the CI artifact | M | `rimarcho` from CI opens on the VM |
| G2: the five blockers in §4.2 plus the WebKitGTK version check | M | Window opens on the VM with default settings, sized to the screen; `kill` closes it cleanly; a database-upgrade error is visible |
| L2: process and file-system parity | S | CI synthetic tests; uninstall waits for a running `archo` in the VM |
| L1: S12 rows and the new S13 GUI session | M | Rows pass or have recorded TODOs |
| Docs: 16 §8.1–§8.2, 17 §1 and §5, 01 §1, CHANGELOG, user guide Linux section | S | `refs.cjs check` clean |

About three weeks of work. Not in this milestone: packages (L7), Steam-client subscriptions (L6), Archo Agent and secrets (L4), native memory targets (M08), GTK 4, Wayland-specific checks beyond what GNOME's default session shows.

## 6. Decisions, and discrepancies

D4, D7, D8 and D9 were decided by the maintainer on 2026-11-22, as recommended. D10 is still open. 17 §1, §5 and 16 §8.1 change when the code that follows these decisions lands, not before.

| # | Question | Decision |
| --- | --- | --- |
| D4 (revisited, decided) | Oldest distributions | Ubuntu 22.04, Debian 12, Fedora 40; WebKitGTK at least 2.44, checked at startup; the glibc floor is set by building on 22.04 |
| D7 (decided) | cgo for the Linux GUI | `CGO_ENABLED=1` only for the Linux GUI binary (WebKitGTK is a C library, and Wails' Linux code calls it through cgo); no `import "C"` in RimArcho's packages; `TestArchNoCgo` stays; `archo` stays pure Go and static |
| D8 (decided) | GTK 3 or GTK 4 | GTK 3 (`-tags gtk3`, WebKitGTK 4.1) for 1.2, because it is measured and on all D4 distributions; build once with GTK 4 (WebKitGTK 6.0) at the next pinned Wails, and switch when the D4 distributions carry it |
| D9 (decided) | One Linux binary or two | Two: the static `archo` (CLI, TUI, MCP; no GTK needed: SteamOS game mode, servers) and the dynamic `rimarcho` GUI, as `archo.exe` and `RimArcho.exe` on Windows |
| D10 (open) | Linux memory measure | Recommended: summed Pss of the process tree, recorded, not gating, while experimental |
| WebKitGTK data folder (decided 2026-11-22) | Where the window's web data lives | Stays where WebKitGTK puts it, in the data directory; Wails beta.28 cannot move it (16 §8.2) |

Discrepancies between docs and code found while checking:

- The 2026-11-09 plan §4 item 1 and §5 say GTK 3 and WebKitGTK 4.1, which is right, but Wails needs `-tags gtk3` to use them: its default is GTK 4 and WebKitGTK 6.0.
- `.github/workflows/ci.yml` and 17 §5 exclude `internal/surface/upgradewin` from Linux CI; it builds, vets and compiles its tests with cgo off.
- 16 §8.6's icon rule ("a development build without `.syso` renders one icon at the system large-icon size") covers Windows; off Windows every start renders 256 px (`winicon_other.go`).
- 16 §8.1 ("GUI startup is compiled only on Windows") still describes the code. The GUI package itself compiles and runs on Linux with cgo and the two tag changes; those changes were made only in the VM copy and not committed.
