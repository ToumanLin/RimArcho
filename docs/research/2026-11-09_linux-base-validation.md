# Linux base validation (2026-11-09)

Scope: [Linux plan](2026-11-09_linux-support-plan.md) phase 1, steps 1, 2 and 9, on `codex/linux-base`. This establishes build and synthetic-test coverage; it does not establish Linux runtime safety or release support.

## Changes

| Files | Result |
| --- | --- |
| `internal/surface/cli/cli.go`, `gui_enabled.go`, `gui_disabled.go`, `gui_other_test.go` | Windows selects the existing GUI startup; other builds return the localized no-window message. The build expressions can later include Linux with cgo without renaming these implementation files. |
| `internal/l10ndata/locale/zh/cli.json` | Chinese translation of the no-window message; English uses the source text through `view.T`. |
| `internal/app/console_other.go`, `console_other_test.go` | Start Workshop SteamCMD through `os/exec` in a separate process group; `KillTree` kills that group. Interactive calls inherit stdin, stdout and stderr. One focused synthetic test checks descendant cancellation. Windows process creation is unchanged. |
| `.github/workflows/ci.yml` | Add an Ubuntu job with cgo disabled: gofmt, static build, vet, staticcheck at the existing pin, and uncached tests. Exclude GUI, upgrade-window and spike packages. The Windows job is unchanged. `go list -e` allows discovery past excluded Windows-only spikes; vet and tests still check every selected package. |
| `internal/app/describe_test.go`, `m5_test.go`; `internal/view/saves_test.go`; `internal/source/link_health_test.go`; `internal/testkit/gitrepo.go` | Use native path separators, check save collisions with the filesystem's case rules, keep the previous link inode allocated while testing replacement, and preserve the existing Git fixture hashes across operating systems. No golden files were regenerated. |
| Windows test files and Linux skips below | Preserve Windows assertions while making platform dependencies explicit. |
| `internal/accounts/session.go`, `browser_windows.go`; `internal/csproj/csproj.go`; `internal/app/selfupdate.go`; `internal/view/settings.go` | Move the Windows-only cookie-capture method with its caller; remove an unused map and a discarded directory-read result; fix a pre-existing gofmt mismatch. These are lint fixes with no intended behavior change. |
| Specifications 11, 13, 16 and 17; the Linux plan; `UserGuide/release-vm-tests.md` | Document the build boundary, cancellation, CI coverage and future VM check. No CHANGELOG entry: this base is not shipped and changes no supported Windows behavior. |

## Newly excluded tests

Tests moved behind a `windows` build tag retain their assertions:

| Test | File | Reason |
| --- | --- | --- |
| `TestILSpyExeFromLauncher` | `internal/app/ilspy_windows_test.go` | Windows `.cmd` launcher syntax and Windows executable layout. `TestCleanILSpy` remains cross-platform. |
| `TestAutomaticGitAndAuthenticatedPageChecksNeverInstall` | `internal/app/autoupdate_windows_test.go` | Its authenticated page fixture persists a Windows DPAPI session. Other automatic-update tests remain cross-platform. |
| `TestLoversLabArchiveInstallUpdateWithBlockedPublicPage` | `internal/app/subscribepage_windows_test.go` | Windows DPAPI session and browser-authenticated download flow. Manual/public page tests remain cross-platform. |
| `TestLoversLabFileAgreementDownloadAndFallback` | `internal/subscribe/loverslab_test.go` | Windows DPAPI session persistence. |
| `TestLoversLabRotationsPersistAfterEveryAuthenticatedRequest` | `internal/subscribe/loverslab_test.go` | Windows DPAPI session persistence. |
| `TestLoversLabSelectorFileMetadata` | `internal/subscribe/loverslab_test.go` | Windows DPAPI session persistence. |
| `TestLoversLabGuestPageKeepsLoginState` | `internal/subscribe/loverslab_test.go` | Windows DPAPI session persistence. |
| `TestWorkshopCopyCleanupKeepsItem` | `internal/selfupdate/relocate_windows_test.go` | Relocation of the two Windows release executables. Linux self-update is excluded by decision D2. |
| `TestWorkshopCleanupKeepsItemDocs` | `internal/selfupdate/relocate_windows_test.go` | Cleanup of the extracted Windows release. `TestInWorkshopFolder` remains cross-platform. |

The following tests call `t.Skip` only on Linux. Their Windows assertions are unchanged:

| Test | File | Deferred behavior |
| --- | --- | --- |
| `TestRemoveSteamCMDProtectsContent` | `internal/app/deps_test.go` | Plan step 3: `toolIdle` refuses replacement before content checks can run. |
| `TestRemoveDepStaysInsideManagedDirectory` | `internal/app/deps_test.go` | Plan step 3: tool-process detection. |
| `TestInstallDepSteamCMDOwnDirectoryAndRemove` | `internal/app/deps_test.go` | Plan steps 3 and 6: tool-process detection and SteamCMD installation. |
| `TestInstallDepToddsAndRemove` | `internal/app/deps_test.go` | Plan step 3: tool-process detection. |
| `TestInstallSteamCMD` | `internal/app/gamedl_test.go` | Plan steps 3 and 6: tool-process detection and SteamCMD installation. |
| `TestInstallTodds` | `internal/app/textures_test.go` | Plan step 3: tool-process detection. |
| `TestGoldenDeps` | `internal/cmds/deps_test.go` | Plan step 3: the successful removal transcript requires tool-process detection. |
| `TestExternalToolsSettings` | `internal/view/deps_test.go` | Plan step 3: removal is hidden while process detection is unavailable. |
| `TestGoldenHarmonyLog` | `internal/cmds/m4_golden_test.go` | Additional discovered gap outside the original deferred list: native absolute log paths are not attributed or normalized. Explicitly skipped to keep this task narrow; the native fixture and Windows golden remain unchanged. |

Existing Windows-only files and pre-existing skips, including WebView2 preflight, were retained.

## Linux issues left unchanged

| Location | Finding |
| --- | --- |
| `internal/app/gamerun_other.go:6` | Running-game detection returns no processes (plan step 3). Passing synthetic tests does not make Linux write operations safe. |
| `internal/app/toolbusy_other.go:7` | Tool replacement always fails because Linux process detection is absent (plan step 3); this caused eight skips above. |
| `internal/app/logs.go:793`, `:890`, `:1295` | Native Linux absolute paths do not match `winPathRe`. Log-file attribution is omitted and finding references retain the temporary root instead of being stable. This caused the additional golden-test skip. |
| `internal/source/link_health.go:140` | `os.SameFile` can accept a recreated same-target symlink if the filesystem immediately reuses its inode. Observed in the original replacement fixture. The portable fixture retains the old inode to test replacement deterministically; inode-reuse hardening remains open. |
| `internal/app/directories.go:459`, `internal/app/locklib.go:133` | Same-volume checks still use `filepath.VolumeName` (plan step 4); the tests here use one filesystem and do not establish cross-mount safety. |
| `internal/app/saveactions.go:317` | The existing case-insensitive target comparison still needs the plan step 4 fix. The preview fixture now checks an exact-case collision on Linux. |
| `internal/app/uninstall.go:181` | `LocalDir` still falls back to the cache directory (plan step 4). Synthetic fixtures supply their own base. |
| `internal/app/gamedl.go:104`, `internal/app/accounts.go:183` | SteamCMD instance paths still name `steamcmd.exe` (plan step 6). No real installer or Steam interaction was tested. |

## Verification

- WSL: Ubuntu 22.04, Linux amd64, Go version from `go.mod` downloaded into `~/sdk`, `CGO_ENABLED=0`; clone in `~/rimarcho-linux`, module/build/test caches in `~/go`. Commits were made in the Windows worktree and pulled into WSL.
- Linux static `archo` build: passed; `file` reports a statically linked x86-64 ELF.
- Linux gofmt, vet, pinned staticcheck: passed on the CI package selection.
- Linux `go test -count=1 -timeout 20m`: passed for all 58 selected packages with the explicit exclusions above.
- Windows `go build ./...` and `go vet ./...`: passed. Final `go test -count=1 ./...`: passed with `GOMAXPROCS=4`; test code and assertions were unchanged. Earlier full runs also passed at default concurrency. Two final default-concurrency runs intermittently failed to rename a synthetic Monitor install directory (`Access is denied`), first in `TestSortAddsMonitor`, then in `TestUninstallRechecksSourceLibraryBeforeDeletion/link`. The first run overlapped temporary-directory inspection; the second did not. Lower concurrency passed; the cause is not established and no Monitor behavior was changed.
- Node VM/feedback-relay tests: 107 passed, none failed or skipped.
- Documentation references and `git diff --check`: passed.
- No native Linux GUI, real games, real Mods, Steam, or real RimArcho home were used. No merge or push was performed.

Resource exception: simultaneous cold build caches briefly exceeded the 3 GB combined disk limit (approximately 3.4 GB measured). The Windows cache was removed immediately after the measurement; build caches are cleared between the final Linux and Windows runs. The time limit was not approached.

Test logs are retained in WSL under `~/go/linux-tests.log`, `~/go/windows-tests.log` and `~/go/windows-tests-retry-*.log`; the selected Linux package list is `~/go/linux-packages.txt`. Build caches were cleared after verification; the toolchain, module cache, binary and clone remain in the authorized directories.

## Documentation discrepancies

17 §5 claimed Linux and macOS compiled without runtime verification. It now distinguishes the static Linux non-GUI build and synthetic CI coverage from unfinished runtime support; macOS is unverified. 16 §8.2 now scopes its no-cgo GUI statement to Windows. The plan's initial survey table describes the pre-change findings; its phase 1 steps 1, 2 and 9 record completion. No remaining runtime-support claim was inferred from a successful build.
