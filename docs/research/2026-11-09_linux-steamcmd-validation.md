# Linux SteamCMD and release validation (2026-11-09)

Scope: Linux plan phase 1 steps 6, 7 and 10 on `codex/linux-steamcmd`. Contracts: 11 §3 and 17 §5 / §5.1. Real games, Mods, Steam and the real RimArcho home are outside this validation.

## Changes

| Step | Files | Result |
| --- | --- | --- |
| 6 | `internal/deps/deps.go`; `internal/app/gamedl.go`, `steamcmdarchive.go`, `steamcmderr.go`, `accounts.go`, `console_other.go`, `hide_other.go`, `gamebranches.go`, `toolupdates.go`; related synthetic fixtures | Platform launcher name, external Linux PATH preference, Valve Linux tar extraction, executable permissions, runtime dependency guidance and Linux package manifest. |
| 7 | `internal/app/selfupdate.go`, `updatenotice.go`; `internal/cmds/update.go`; `internal/view/selfupdate.go`, `autoupdate.go`; automatic-notice callers in CLI/TUI/GUI | GitHub-only Linux checks; package marker; version, release link and update instructions. Non-Windows install and cleanup cannot touch program files. TUI presentation uses the existing panel and view text. |
| 10 | `scripts/release.ps1`, `scripts/releasetar/` | Static Linux amd64 binary, tar.gz with executable mode, shared release documentation and Monitor assets, SHA-256. Default release builds both platforms; Linux-only and alternate output-directory options support local dry runs. |
| Documentation | Specs 11, 13, 17; Linux plan; release VM table | Contracts and a pending Linux VM session. CHANGELOG records the Windows-visible CLI help wording change; Windows installation behavior is unchanged. |

## Sources

The bootstrap URL is `https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz`, documented by [Valve's SteamCMD installation page](https://developer.valvesoftware.com/wiki/SteamCMD#Manually). Direct retrieval of Valve's wiki returned HTTP 403 during this session; distribution package names were checked against their own documentation:

- Debian/Ubuntu `lib32gcc-s1`: [Debian Bookworm package](https://packages.debian.org/bookworm/lib32gcc-s1) and [Ubuntu Jammy package](https://packages.ubuntu.com/jammy/lib32gcc-s1). Both identify the 32-bit GCC support library; Debian's dependencies include the 32-bit libc loader.
- Fedora `glibc.i686` and `libstdc++.i686`: [Fedora's 32-bit package instructions](https://fedoraproject.org/wiki/HOWTO_Setup_Android_Development#32_bit_packages); package roles are confirmed by the [glibc](https://packages.fedoraproject.org/pkgs/glibc/glibc/) and [libstdc++](https://packages.fedoraproject.org/pkgs/gcc/libstdc++/) catalogs.
- Arch `lib32-gcc-libs`: [official package](https://archlinux.org/packages/core/x86_64/lib32-gcc-libs/), described as 32-bit GCC runtime libraries, depending on `lib32-glibc`.

## Validation

- Windows: `go build ./...`, `go vet ./...` and `go test -count=1 ./...` passed on the final implementation before the deprecated tar constant was removed. After that Linux-only extraction correction, Windows build, vet and the affected `internal/app` suite passed again (62.322 seconds). No Windows self-update assertions were removed or relaxed.
- WSL Ubuntu 22.04, Go 1.27.2, `CGO_ENABLED=0`: all 59 packages selected by `go list -e ./... | grep -Ev '/(internal/surface/(gui|upgradewin)|spikes)(/|$)'` passed vet, pinned staticcheck 2026.2.2 and `go test -count=1 -timeout 20m`. The tarball build also passed.
- Focused Linux tests passed for the fake HTTP SteamCMD tarball, unsafe/incomplete archive rejection, executable bits, the actual temporary PATH lookup, missing-library diagnostics, and a fake GitHub feed with no asset downloads or Workshop requests. Package markers deb/rpm/aur, unknown and missing markers, and preservation of stale Windows swap files were checked.
- `TestInstallSteamCMD` and `TestInstallDepSteamCMDOwnDirectoryAndRemove` now attempt the installation and skip only when the unchanged `toolIdle` stub returns its Windows-process-detection error. Both still skip in this branch for that reason. Archive extraction is independently exercised rather than bypassing the process-safety guard.
- Windows replacement/Workshop self-update tests remain active on Windows and explicitly skip off Windows because D2 now disables those operations.
- Node VM/feedback-relay suites: 107 passed, no failures or skips. Documentation references and `git diff --check` passed.
- Local dry run: `scripts/release.ps1 -Version 1.1.0-linux-preview -GOOS linux -OutputDir D:/RimArcho-test/linux-release/final -SkipTests`, built from `efdfd1454670`. WSL `sha256sum -c` passed; extracted `archo` has mode 0755; `file` identifies a statically linked x86-64 ELF; `./archo --version` reports 1.1.0-linux-preview. The final smoke run used `/home/touman/rimarcho-steamcmd-smoke.EiWVle` as `RIMARCHO_HOME`.
- Isolation exception: the earlier WSL `--version` smoke runs used the default `/home/touman/.local/share/rimarcho`, because the command opens the database before printing its version. This was discovered by inspecting `internal/surface/cli/cli.go:92` and repeated correctly with a temporary home. That earlier access cannot be claimed as synthetic-home-only validation; the default directory was not deleted because its prior ownership/state was not recorded. Windows `%APPDATA%\RimArcho`, real games, Mods and Steam were not used.
- Logs: Windows `D:/RimArcho-test/linux-steamcmd-windows-tests.log`, `linux-steamcmd-windows-app-final.log`, `linux-steamcmd-node-tests.log`, `linux-steamcmd-release-final.log`; WSL `/home/touman/rimarcho-steamcmd-tests.log`, `rimarcho-steamcmd-focused.log`, `rimarcho-steamcmd-packages`.
- Build caches: `go clean -cache` completed on Windows and in WSL after validation. Existing SDK/module caches were retained. The WSL clone is 102 MB and extracted dry runs total 65 MB. Peak incremental growth of the shared Windows build cache was not measured.
- No release, tag, upload, merge or push was performed. Only this branch and its disposable WSL clone were changed.

## Remaining gaps and documentation differences

- `internal/app/toolbusy_other.go:7`: process safety is still the existing stub; the parallel Linux safety task owns it. It blocks end-to-end managed installation/removal. The two tests above retain only that skip.
- Real SteamCMD bootstrap execution, sign-in prompts, libraries and distribution wrappers remain unverified until the Linux desktop VM session in `UserGuide/release-vm-tests.md`. In particular, `internal/app/gamedl.go:524`, `internal/app/subscribeworkshop.go:429` and `internal/app/accounts.go:110` still read logs beside the selected executable; a distribution wrapper that relocates its log directory needs runtime validation.
- Packages do not yet install the marker; this is the defined interface for the later packaging phase (`internal/app/updatenotice.go:13`). No real .deb/.rpm/AUR upgrade or native Linux GUI was tested. The default combined Windows+Linux release invocation was not dry-run; its Linux-only path was.
- The original discovery documentation implied that a missing configured executable falls back to the managed copy. The code searches its siblings then PATH, skipping the managed copy when a setting is present. Specs 11 and 17 now follow the code. The original release specification said only dist was written, although resource and Monitor builds also use build intermediates and caches; 17 §5 now distinguishes release output from those intermediates. No new claim of complete Linux runtime support is made.

## Dry-run artifacts

Directory: `D:/RimArcho-test/linux-release/final`. Staging is retained beside the archive for inspection. Sizes are bytes; the SHA-256 sidecar uses `<hash>  <archive filename>` plus a newline.

| Artifact | Bytes |
| --- | ---: |
| `archo-1.1.0-linux-preview-linux-amd64.tar.gz` | 13,335,670 |
| `archo-1.1.0-linux-preview-linux-amd64.tar.gz.sha256` | 111 |

Archive member inventory:

| File | Bytes | Mode |
| --- | ---: | --- |
| `CHANGELOG.md` | 230,330 | `0644` |
| `LICENSE` | 1,067 | `0644` |
| `README.md` | 10,601 | `0644` |
| `README.zh-CN.md` | 10,015 | `0644` |
| `archo` | 32,694,432 | `0755` |
| `monitor/ArchoMonitor/1.5/Assemblies/ArchoMonitor.dll` | 53,248 | `0644` |
| `monitor/ArchoMonitor/1.6/Assemblies/ArchoMonitor.dll` | 53,248 | `0644` |
| `monitor/ArchoMonitor/1.7/Assemblies/ArchoMonitor.dll` | 52,736 | `0644` |
| `monitor/ArchoMonitor/About/About.xml` | 2,080 | `0644` |
| `monitor/ArchoMonitor/About/ModIcon.png` | 76,829 | `0644` |
| `monitor/ArchoMonitor/About/Preview.png` | 197,504 | `0644` |
| `monitor/ArchoMonitor/LICENSE` | 1,067 | `0644` |
| `monitor/ArchoMonitor/Languages/ChineseSimplified/Keyed/Settings.xml` | 488 | `0644` |
| `monitor/ArchoMonitor/Languages/English/Keyed/Settings.xml` | 503 | `0644` |
| `monitor/ArchoMonitor/LoadFolders.xml` | 312 | `0644` |
| `skills/rimworld-probe/SKILL.md` | 11,267 | `0644` |
| `skills/rimworld-probe/scripts/probe.mjs` | 12,946 | `0644` |
| `skills/rimworld-probe/template/About/About.xml` | 583 | `0644` |
| `skills/rimworld-probe/template/LoadFolders.xml` | 170 | `0644` |
| `skills/rimworld-probe/template/Probe.csproj` | 3,713 | `0644` |
| `skills/rimworld-probe/template/src/ProbeRunner.cs` | 17,165 | `0644` |
| `skills/rimworld-probe/template/src/Scenario.cs` | 1,604 | `0644` |

## Changed-file inventory by step

### 6 — SteamCMD

- `internal/app/accounts.go`
- `internal/app/accounts_test.go`
- `internal/app/autoupdate_test.go`
- `internal/app/console_other.go`
- `internal/app/credit_test.go`
- `internal/app/deps_test.go`
- `internal/app/directories_test.go`
- `internal/app/firstrun_test.go`
- `internal/app/gamebranches.go`
- `internal/app/gamebranches_integration_test.go`
- `internal/app/gamebranches_test.go`
- `internal/app/gamedl.go`
- `internal/app/gamedl_test.go`
- `internal/app/hide_other.go`
- `internal/app/steamcmdarchive.go`
- `internal/app/steamcmdarchive_test.go`
- `internal/app/steamcmderr.go`
- `internal/app/steamcmderr_linux_test.go`
- `internal/app/steamsignout_test.go`
- `internal/app/subscribe_test.go`
- `internal/app/subscribeworkshop_test.go`
- `internal/app/subscribeworkshoprun_test.go`
- `internal/app/toolupdates.go`
- `internal/app/uninstall_test.go`
- `internal/app/workshoppreflight_test.go`
- `internal/app/workshopprogress_test.go`
- `internal/cmds/anonymous_test.go`
- `internal/cmds/deps_test.go`
- `internal/cmds/env.go`
- `internal/cmds/uninstall_golden_test.go`
- `internal/deps/deps.go`
- `internal/deps/deps_test.go`
- `internal/deps/steamcmd_linux_test.go`
- `internal/surface/tui/esc_pages_test.go`
- `internal/surface/tui/feedback_test.go`
- `internal/surface/tui/uninstall_test.go`
- `internal/view/accounts_test.go`
- `internal/view/autoupdate_test.go`
- `internal/view/deps_test.go`
- `internal/view/subscriptions_test.go`

### 7 — Update notice

- `internal/app/selfupdate.go`
- `internal/app/selfupdate_test.go`
- `internal/app/updatenotice.go`
- `internal/app/updatenotice_linux_test.go`
- `internal/cmds/update.go`
- `internal/surface/cli/cli.go`
- `internal/surface/gui/autoupdate.go`
- `internal/surface/tui/update.go`
- `internal/view/autoupdate.go`
- `internal/view/selfupdate.go`

### 10 — Release

- `scripts/release.ps1`
- `scripts/releasetar/main.go`
- `scripts/releasetar/main_test.go`

### Documentation and output contract

- `CHANGELOG.md`
- `UserGuide/release-vm-tests.md`
- `docs/research/2026-11-09_linux-steamcmd-validation.md`
- `docs/research/2026-11-09_linux-support-plan.md`
- `docs/specs/11-downloads-and-accounts.md`
- `docs/specs/13-commands-and-cli.md`
- `docs/specs/17-engineering.md`
- `testdata/golden/basics.txt`
