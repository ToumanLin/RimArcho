# Linux detection and launch validation (2026-11-09)

Scope: [Linux support plan](2026-11-09_linux-support-plan.md) phase 1 step 5, branch `codex/linux-detect`. Contracts: 03 §3.2, 09 §2 and 20 §2. Tests use synthetic homes, games, Steam roots and prefixes; no real game, Mod, Steam account or RimArcho home was used.

## Changes by step

| Step | Files | Result |
| --- | --- | --- |
| 1: Steam roots and libraries | `internal/env/detect.go`, `linux_detect_test.go` | Add `.steam/root`, Flatpak `data/Steam` and Snap alongside the existing Linux roots. Resolve Steam library symlinks off Windows, preserving order and discarding duplicates. Retain modern and old VDF formats, `config/` fallback and manual locations. |
| 2: native and Proton data | `internal/env/layout.go`, `detect.go`; `internal/app/envs.go`; `internal/view/shell.go` | Derive Proton from the executable and existing library prefix; native takes precedence. Resolve data per game, including external Flatpak libraries, when detecting, adding, selecting shared saves and classifying save mode. No schema or stored field changes. |
| 3: launch | `internal/env/detect.go`; `internal/app/launch.go`, `envs.go`, `gametest.go`, `linux_launch_test.go`; `internal/gametest/paths.go`, `paths_test.go` | Find host Steam on PATH, a root script or Flatpak with its argument prefix. Refuse direct Proton launch, own saves at add/set, redirected saves at launch (including extra arguments and pack launches), and isolated test runs before data preparation. Native direct launch and test runs select `RimWorldLinux`. |
| 4: layout | `internal/env/env.go`, `layout.go`; `internal/menubg/catalog.go`, `unity_cache.go`, `unity_test.go`; `internal/gametest/paths.go`, `paths_test.go`; `internal/cmds/guide_rebuild.go` | Add native Managed assemblies, Unity background extraction and source stamps, and native test-run executable paths. The rebuild guide names both Managed layouts. |
| 5: Steam sign-in | `internal/env/path_other.go`, `steamuser.go`, `steamuser_other_test.go` | Read the active user and process from `.steam/registry.vdf`; resolve loginusers across the Linux roots, with AutoLoginUser fallback. Dead processes and invalid registries are signed out. Windows registry code stays unchanged. |
| 6: docs | Specifications 03, 05, 09, 20; Linux plan; `UserGuide/release-vm-tests.md`; `CHANGELOG.md`; this report | Document current behavior and add separate native, Proton and Flatpak Linux VM sessions. The changelog records only the rebuild-guide wording visible on Windows. |

No changes to `internal/app/workshopsub.go`: Linux Steamworks loading remains phase 4 of the plan; adding a native library path would not make that Windows-only loader work.

## Synthetic detection table

All Steam game paths below end in `steamapps/common/RimWorld`. All data directories include `Ludeon Studios/RimWorld by Ludeon Studios` after the stated parent. Tests check `Detect`, `DetectAll`, the game-specific default, launch method, Workshop, version and Player.log. Both ordinary and Flatpak native data directories exist in the fixtures, so search order cannot mask a wrong selection.

| Input layout under synthetic HOME | Detected environment |
| --- | --- |
| `.steam/steam`, `.steam/root`, or `.local/share/Steam`; `RimWorldLinux` | Native, Steam launch; data under `~/.config/unity3d` |
| `snap/steam/common/.local/share/Steam`; `RimWorldLinux` | Native, Steam launch; ordinary native data |
| `.var/app/com.valvesoftware.Steam/.local/share/Steam`; native game | Native, Steam launch; Flatpak `.config/unity3d` data |
| `.var/app/com.valvesoftware.Steam/data/Steam`; native game in an external VDF library | Native, Steam launch; Flatpak data even outside its root |
| Standard Steam root with external library, `RimWorldWin64.exe` and `compatdata/294100/pfx` | Proton, Steam launch; data under the same library's `pfx/drive_c/users/steamuser/AppData/LocalLow` |
| Flatpak root, Windows executable and prefix | Proton, Steam launch; prefix data, independent of Flatpak native data |
| Windows executable without prefix | Ordinary Steam suggestion; not classified as Proton |
| Native and Windows executables with prefix | Native takes precedence; ordinary native data |
| `Games/RimWorld` or `GOG Games/RimWorld`; native executable | Native, direct launch; ordinary native data |
| Native + external Proton + Flatpak native games together | Three suggestions, each with its own data root |
| `.steam/steam` and `.steam/root` symlinked to `.local/share/Steam` | One canonical library and one game suggestion (Linux check) |

## Verification

- Focused Windows synthetic tests: passed for detection, Steam command selection, launch restrictions, native Managed paths, Unity background extraction and test-run executable selection. Linux-only assertions await WSL below.
- Full Windows build, vet and uncached Go tests: pending.
- WSL Ubuntu 22.04 static build, CI package selection, vet and uncached Go tests: pending.
- Node VM/feedback-relay tests, documentation references and diff checks: pending.
- Linux VM native/Proton/Flatpak sessions: not run; rows remain in the release VM checklist. No real launch or sign-in was attempted.

## Remaining boundaries and documentation discrepancies

- Proton own/custom saves, packs and isolated runs remain unavailable until phase 2 implements Windows-path conversion. The refusal is intentional (09 §2).
- Classification requires an existing prefix and specifically `RimWorldWin64.exe`; a Windows game before Steam has created its prefix is not classified as Proton. This follows the detection contract in 03 §3.2.
- Actual Steam, Flatpak sandbox access, Snap confinement, Steam Deck and entering the game are unverified. The executable and registry fixtures are never executed or connected to Steam.
- Linux runtime safety, SteamCMD, updates and packaging belong to the parallel work and are not established by this branch's tests.
- The base validation report and the plan's initial survey describe pre-change gaps; they are historical findings. The initial 03 §3.2 listed Flatpak as a generic fallback, while `DetectAll` reused one data directory for every game. It now specifies per-game data selection. The Player.log version rule in 03 §3.3 remains authoritative: 1.6 uses the data root, 1.7+ uses `Logs/`; the plan's initial Proton description omitted that version distinction.
