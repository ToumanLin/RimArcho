# Linux support plan (2026-11-09)

Players asked for Linux (2026-11-07, again 2026-11-09). One asked for a `.deb`, an `.rpm` and something for Arch. Another ran the Windows build under Proton: `archo.exe` started but could not find the game paths by itself, and `RimArcho.exe` showed no window (WebView2 does not run under Wine; since `1425a31d` it says so). This plan turns the survey in ToDo "Linux version" into phases. macOS stays out of scope (ToDo). **[C]** marks facts checked in the code or by a build on 2026-11-09, **[I]** inferences still to confirm.

## Summary

- **The non-GUI code is close to compiling.** With `GOOS=linux CGO_ENABLED=0`, every package under `internal/` except `surface/gui`, `surface/cli` and `surface/upgradewin` passes `go vet` once `startWorkshopCMD` has a non-Windows version; that is the only missing symbol. **[C]** Most of the work is runtime behavior, not compiling.
- **Ship in four phases**, each releasable on its own: 0) tell Proton users how to set paths by hand; 1) a native `archo` (CLI, TUI, MCP) that is safe to use; 2) the native GUI; 3) packages (`.tar.gz`, `.deb`, `.rpm`, AUR). Steam-client subscribe (phase 4) is optional.
- **Proton installs count as Linux installs.** Many Linux players run the Windows game through Proton; phase 1 detects both the native game and a Proton game, so a native `archo` serves the player who tested under Proton.
- **Testing needs a Linux VM** with Steam, the native game and a Proton copy (§7). No real-data tests on the host (17 §4).

## 1. What was checked

| Fact | Where |
| --- | --- |
| Only `startWorkshopCMD` is missing outside the GUI **[C]** | `internal/app/subscribeworkshop.go:443`; Windows version in `console_windows.go` |
| Wails v3 (beta.28) on Linux needs cgo; with cgo off it fails at `undefined: pointer` **[C]**; it also needs GTK 3 and WebKitGTK 4.1 headers, and has to be built on Linux **[I]** | `go vet` with `GOOS=linux` |
| `surface/cli` imports `surface/gui`, so `cmd/archo` pulls in Wails **[C]** | `internal/surface/cli/cli.go:28` |
| `gameProcesses` returns nil and `toolIdle` always fails off Windows **[C]** | `internal/app/gamerun_other.go`, `toolbusy_other.go` |
| Same-volume checks use `filepath.VolumeName`, which is always `""` on Linux **[C]** | `app/directories.go:459`, `app/locklib.go:133`, `app/uninstall.go:937`, `gui/shell.go:547` |
| Game layout assumes `RimWorldWin64_Data` and `steam_api64.dll` **[C]** | `env/env.go:113`, `menubg/unity_cache.go`, `menubg/catalog.go`, `app/workshopsub.go:449` |
| SteamCMD is `steamcmd.exe` everywhere **[C]** | `app/accounts.go`, the SteamCMD installer |
| CI runs only on `windows-latest` **[C]** | `.github/workflows/ci.yml` |
| 17 §5 says `GOOS=linux` compiles: wrong **[C]** | `docs/specs/17-engineering.md` §5 |
| Already right for Linux: user data paths incl. Flatpak Steam (03 §3), `RimWorldLinux` as a game root (03 §3) and for direct launch (09 §2), data directory under XDG (17 §3) **[C]** | specs |

## 2. Phase 0: answer the Proton tester (no code)

- Reply to the tester: the Windows build is not the way to go on Linux. Until phase 1 ships, `wine archo.exe` in a Linux terminal works if the paths are given by hand: game `Z:\home\<user>\.local\share\Steam\steamapps\common\RimWorld`, Workshop `…\steamapps\workshop\content\294100`, user data `Z:\home\<user>\.config\unity3d\Ludeon Studios\RimWorld by Ludeon Studios` (native game) or `…\steamapps\compatdata\294100\pfx\drive_c\users\steamuser\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios` (game run through Proton). Launching from Wine is not expected to work.
- Ask both players which distribution they use, and whether the game is native or Proton. Their answers set the priorities of §5.

## 3. Phase 1: native `archo` (CLI, TUI, MCP)

Goal: a static `archo` for `linux/amd64` (`CGO_ENABLED=0`) that is as safe on Linux as on Windows. Size: medium.

1. **Build split — done.** GUI startup is selected by a `windows` build tag (later `windows || (linux && cgo)`), so `cmd/archo` builds without Wails. On a build without the GUI, `archo gui` says the window is not in this build and points to the text interface. Contract: 13 §4.1, 16 §8.1.
2. **`startWorkshopCMD` off Windows — done.** `os/exec` with its own process group (`Setpgid`); cancel kills the group, matching the Windows job object. `console=true` inherits the current terminal's streams (CLI/TUI) instead of opening a new window. Contract: 11 §1.
3. **Safety stubs — done.** Linux reads `/proc` for native and Wine/Proton games and for busy tools. Contracts: 09 §4 and 17 §2.1. Synthetic processes exercise discovery; guarded operations share the existing running-game checks.
4. **Linux-only bugs — done for non-GUI `archo`:**
   - Same-volume checks compare `st_dev` through one helper at the three non-GUI call sites in §1, so moves across mounts copy instead of failing with `EXDEV`. The GUI shell call site remains phase 2 work.
   - Renaming a save to a name that differs only by case must not overwrite the other save (`saveactions.go`): on a case-sensitive file system they are two files.
   - `LocalDir` uses the XDG data directory, not `~/.cache` (it holds the lock library).
   - `source.Trash` (`trash_other.go`) works across volumes: freedesktop trash in `$XDG_DATA_HOME/Trash`, or `<mount>/.Trash-<uid>` for another volume.
   - The base task's additional log-path gap is fixed: native absolute Linux paths participate in attribution and stable finding keys (06 §2–§3); the Harmony-log golden runs unchanged on both systems.
   - Verification and remaining limitations, [safety validation](2026-11-09_linux-safety-validation.md).
5. **Detection** (03 §3):
   - Steam roots: `~/.steam/steam`, `~/.local/share/Steam`, Flatpak `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`, Snap `~/snap/steam/common/.local/share/Steam`; then `libraryfolders.vdf` as on Windows.
   - A game folder with `RimWorldWin64.exe` beside `steamapps/compatdata/294100` is a **Proton game**: user data in `compatdata/294100/pfx/drive_c/users/steamuser/AppData/LocalLow/Ludeon Studios/RimWorld by Ludeon Studios`, Player.log in the same folder. It launches only through Steam (09 §2); own-saves mode is phase 2 (`-savedatafolder` needs a `Z:` path).
   - Game layout: `RimWorldLinux_Data` beside `RimWorldWin64_Data` (Managed folder, menu backgrounds, `gametest` paths).
   - `SteamSignedIn` reads `~/.steam/registry.vdf`.
6. **SteamCMD.** Install `steamcmd_linux.tar.gz` (runs `linux32/steamcmd`, needs 32-bit libraries); prefer a `steamcmd` already on `PATH` (distribution packages exist). When it fails to start, say which package is missing (`lib32gcc-s1` on Debian/Ubuntu, `glibc.i686` on Fedora) **[I: names to confirm]**. Login works in the terminal that runs `archo`.
7. **Update notice instead of self-update (D2).** Run the same check as `CheckSelfUpdate` (GitHub Releases); when a newer version exists, say so with the release link and how to update: through the package manager for a `.deb`, `.rpm` or AUR install, by downloading the new tarball otherwise. Nothing is downloaded or replaced.
8. **Not in phase 1, with a clear message:** subscribing through the Steam client (phase 4: SteamCMD only); LoversLab login (manual zip, as decided).
9. **CI — done.** `ubuntu-latest` checks the static `archo` build, gofmt, `go vet`, pinned staticcheck and `go test -count=1` on every package except `surface/gui`, `surface/upgradewin` and `spikes/...`. Windows checks are unchanged. WSL verification, portable fixtures and explicit Linux test exclusions are recorded in [the base validation report](2026-11-09_linux-base-validation.md); runtime safety remains unfinished.
10. **Release.** `archo-<version>-linux-amd64.tar.gz` with execute bits, made with `tar` or Go's `archive/tar`, not `Compress-Archive` (`scripts/release.ps1`); add a `.sha256`.
11. **Docs.** 17 §5 (what compiles and what is tested), 01 §1 (Linux CLI in 1.x), 03 §3 (Steam roots, Proton), 09 §2 (Proton launches through Steam), the user guide (Linux section), CHANGELOG, a release VM table for Linux.

## 4. Phase 2: native GUI

Size: large; most of it is testing.

1. **Build `RimArcho` on Linux** with cgo, `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`, in CI on the oldest supported distribution (glibc is linked dynamically, so build on the oldest).
2. **WebKitGTK differences.** `:has()` and `color-mix()` need WebKitGTK ≥ ~2.42; check the versions in the oldest supported distributions (D4) and either require that version or add fallbacks. The WebGL Archo may fall back to stills; accept it.
3. **Windows-only GUI code:** the WebView2 memory trim when minimized (17 §7.3) becomes a no-op; check native-window keys and the context-menu rule (`333409ca`) in WebKitGTK; the first-run guide offers the Linux paths.
4. **SteamCMD login from the GUI** has no console: open the player's terminal (`x-terminal-emulator`, `gnome-terminal`, `konsole`), or show the `archo` command to run.
5. **Proton own-saves mode:** pass `-savedatafolder` as a `Z:` path.
6. **Measurement tools.** WebKitGTK has no CDP, so the VM harness, memprobe, jumpiness probe and the 16 §10 / 18 baselines do not run as they are. Decide (D5): measure Linux by hand at release, or port the harness to WebKit's inspector or WebDriver (`WebKitWebDriver`).

## 5. Phase 3: packages

Build every format from one build with [nfpm](https://nfpm.goreleaser.com/) (`.deb`, `.rpm`) and plain `tar`. Size: small once phase 2 builds.

| Format | For | Notes |
| --- | --- | --- |
| `.tar.gz` | Everyone, incl. SteamOS and other immutable systems | Both programs, a `.desktop` file, the icon, a short install note |
| `.deb` | Debian, Ubuntu, Mint, Pop!_OS | Depends on `libgtk-3-0`, `libwebkit2gtk-4.1-0` **[I]** |
| `.rpm` | Fedora, RHEL family, Nobara | Depends on `gtk3`, `webkit2gtk4.1` **[I]**; openSUSE names differ |
| AUR `rimarcho-bin` | Arch, Manjaro, EndeavourOS, CachyOS | A PKGBUILD that downloads the tarball; can be handed to a community maintainer |
| Flatpak | — | Not planned: the sandbox blocks writes to Steam libraries, symlinks and `/proc` (ToDo) |
| AppImage | — | Not planned at first: bundling WebKitGTK is large and fragile; reconsider if SteamOS players need it |

Linux has no self-update (D2): package managers update `.deb`, `.rpm` and AUR installs, and the update notice tells tarball users to download the new version.

## 6. Phase 4 (optional): subscribing through the Steam client

`steamworks` loads `steam_api64.dll` with Windows calls. On Linux, load `libsteam_api.so` from the game folder with [purego](https://github.com/ebitengine/purego) (no cgo), and run `archo steam-worker` the same way. Until then, Linux subscribes with SteamCMD.

## 7. Test environment

- **New Linux VM** (VMware, like the Windows VMs): Ubuntu 24.04 desktop, Steam signed in, RimWorld 1.6 native, a second library with the game forced to Proton, the same 6 active Mods as the Windows real home; snapshot it. Real-data rules as for the Windows VMs (17 §4).
- **Smoke tests only:** a Fedora VM for the `.rpm`, an Arch container for the PKGBUILD.
- **Steam Deck:** ask a player; no hardware here.
- Every phase ends with a VM round that enters the game: launch, play, save, load, exit, then check the game log.

## 8. Decisions (2026-11-09)

| # | Question | Decision |
| --- | --- | --- |
| D1 | Detect Proton installs in phase 1? | Yes: the player who tested has one, and detection is cheap; Proton launches only through Steam |
| D2 | Self-update on Linux | No self-update; an update notice instead (§3 step 7): the new version, its release link, and how to update for the way RimArcho was installed |
| D3 | Which packages | `.tar.gz`, `.deb`, `.rpm`, AUR; no Flatpak or AppImage at first |
| D4 | Oldest supported distributions | Ubuntu 22.04, Debian 12, Fedora 40 (sets the WebKitGTK and glibc floor; versions to confirm) |
| D5 | GUI measurements on Linux | Manual at release for now; port the harness only if Linux players report slow pages |
| D6 | When | After 1.1; phase 1 can ship as a "Linux preview" without the GUI |
