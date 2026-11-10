# Linux safety validation (2026-11-09)

Scope: [Linux plan](2026-11-09_linux-support-plan.md) phase 1 steps 3 and 4, plus the native log-path gap from [base validation](2026-11-09_linux-base-validation.md). Implementation commits: `46da959b` and `b08d6d36`, on `codex/linux-safety`. This establishes synthetic safety coverage for non-GUI `archo`; real-game Linux release acceptance remains pending.

## Files changed by requested step

Paths in this table are relative to the repository root. Repeated files have responsibilities in more than one step.

| Step | Files | Result |
| --- | --- | --- |
| 1. Running games | `internal/app/gamerun_linux.go`, `gamerun_other.go`, `gamerun.go`, `gamerun_linux_test.go` | Read native and Wine/Proton process paths and NUL-separated arguments; fill PID, executable, start time and save-data information where available. Resolve Wine drive mappings and filesystem aliases before matching environment boundaries. Existing guards consume these runs. Other unsupported operating systems retain their stub. |
| 2. Busy tools | `internal/app/toolbusy_linux.go`, `toolbusy_other.go`, `toolbusy_windows.go`, `deps.go`, `textures.go`, `gamedl.go`, `toolbusy_linux_test.go`; `internal/app/deps_test.go`, `textures_test.go`; `internal/cmds/deps_test.go`; `internal/view/deps_test.go` | Pass the managed folder to process checks. Linux detects executables anywhere below it, including deleted running executables. Protected matching same-user processes block replacement; an unrelated protected `sd-pam` does not block every tool. Windows keeps its existing name-based check. Six deferred tests run again. The only installer-area change is the folder argument at `gamedl.go:106`. |
| 3. Volumes | `internal/source/samevolume_windows.go`, `samevolume_other.go`, `samevolume_test.go`; `internal/app/directories.go`, `locklib.go`, `uninstall.go`, `app.go`, `moves_linux_test.go` | One helper compares Windows volume names or Unix device IDs, using existing ancestors for new destinations. All three non-GUI move paths use it. Unknown devices select checked copying; directory and lock-library renames also fall back on `EXDEV`, including bind-mount cases. Copy verification and original recycling retain their existing transaction/link handling. |
| 4. Case-only save rename | `internal/app/saveactions.go`, `rename_linux.go`, `rename_other.go`, `saves_test.go` | Check filesystem identity rather than case-folded paths. Refuse another save whose name differs only by case; allow changing the original's case. Linux uses `RENAME_NOREPLACE` to protect against a newly appearing target. |
| 5. Persistent managed root | `internal/app/uninstall.go`, `localdir_other_test.go` | Off Windows, default to absolute `XDG_DATA_HOME/rimarcho-local`, otherwise `~/.local/share/rimarcho-local` (review fix: originally `rimarcho`, the data directory itself); preserve explicit configuration and the synthetic `LockBase` override. Do not use the cache or `LOCALAPPDATA` there. The overlap check stays enforced. |
| 6. Trash | `internal/source/trash_other.go`, `trash_linux_test.go` | Select home-volume or private volume trash; check ownership, permissions and link type. Reserve metadata exclusively, escape paths, use a mount-relative path for volume trash, and finish metadata before moving. Failures leave the original in place. macOS retains its existing trash branch. |
| 7. Native log paths | `internal/app/logs.go`; `internal/cmds/m4_golden_test.go` | Attribute absolute Linux paths and normalize their finding keys using the existing final-three-segment rule. Invalidate old attribution caches. Enable the Harmony-log golden on Linux without changing any golden file. |
| 8. Documentation | Specifications `02`, `06`, `09`, `12`, `17`; the Linux plan; `UserGuide/release-vm-tests.md`; `CHANGELOG.md`; this report | Update the owning contracts, mark the non-GUI plan steps complete and add three Linux VM sessions. The changelog includes only behavior also visible on Windows: shared Linux-log attribution and game-folder aliases. |

Specification 08 has no independent running-game refusal rule: it describes editing/sorting plans. Its disk-write and launch contracts are in 07/09, so no new refusal was added to sorting.

## Tests enabled again

| Test | File |
| --- | --- |
| `TestRemoveSteamCMDProtectsContent` | `internal/app/deps_test.go` |
| `TestRemoveDepStaysInsideManagedDirectory` | `internal/app/deps_test.go` |
| `TestInstallDepToddsAndRemove` | `internal/app/deps_test.go` |
| `TestInstallTodds` | `internal/app/textures_test.go` |
| `TestGoldenDeps` | `internal/cmds/deps_test.go` |
| `TestExternalToolsSettings` | `internal/view/deps_test.go` |
| `TestGoldenHarmonyLog` | `internal/cmds/m4_golden_test.go` |

`TestInstallDepSteamCMDOwnDirectoryAndRemove` and `TestInstallSteamCMD` remain skipped on Linux for the SteamCMD task. All other exclusions recorded by the base task remain in place. No golden was regenerated and no test framework or dependency was added.

## Verification

- Windows: `go build ./...`, `go vet ./...`, and `go test -count=1 ./...` passed. The full test run used `GOMAXPROCS=4`, as in the base task's final validation. Subsequent implementation changes were confined to Linux-tagged code/tests; the final Windows-selected source is the tested source.
- WSL Ubuntu 22.04, Go 1.27.2 from `~/sdk/go`, `CGO_ENABLED=0`, `GOMAXPROCS=4`: static `archo` build, `go vet`, CI-pinned `staticcheck@2026.2.2`, and `go test -count=1 -timeout 20m` passed on all 58 CI-selected packages. Selection: `go list -e ./... | grep -Ev '/(internal/surface/(gui|upgradewin)|spikes)(/|$)'`. `file` identified a statically linked x86-64 ELF.
- Documentation references and `git diff --check` passed. The Node VM/feedback-relay suites passed all 107 tests, with no skips.
- The first focused Linux run exposed the protected same-user `sd-pam` process: refusing every unreadable same-user executable would leave all tool removals blocked. The second implementation compares protected process names with the tool tree and handles Linux's truncated `comm`; focused tests and the full suite passed afterward.
- Two log-display wrappers failed after successful Go commands because the final shell line received a carriage return. A separate directly executed WSL script confirmed pinned staticcheck returned zero. These wrapper failures did not indicate test failures.

### Real Linux mechanisms exercised with synthetic data

`TestLinuxGameProcesses` runs harmless copies of `/bin/sleep` named `RimWorldLinux`, `RimWorldWin64.exe`, `wine64-preloader` and `wine`. The Wine-named processes receive a Wine-format game path in argv. Real procfs discovery verifies PID, host game path, start time, readable arguments, game-folder aliases and sibling-folder exclusion. These are harmless ELF programs, not actual RimWorld or Wine.

`TestLinuxToolIdle` runs an arbitrarily named ELF from a nested tool folder, checks the sibling boundary, then unlinks the executable while it continues running. Both busy states are refused. An unavailable process table is also refused. A separate focused test covers protected-process name matching and the 15-byte kernel limit; the WSL session's actual protected `sd-pam` exercises the unrelated-process fallback.

Save tests run on Windows' case-insensitive filesystem and Linux's case-sensitive filesystem. The Linux test creates two distinct case variants, verifies that neither is overwritten, then removes the collision and changes the original's case successfully. XDG tests isolate HOME, data, cache and Windows environment variables.

`/dev/shm` is an existing writable tmpfs on a different device from the WSL clone/temp directories (observed device IDs 65 and 2096). No sudo, loop device or new mount was needed:

- `TestMoveDirAcrossLinuxMounts` performs a real cross-device uninstall-style move. Its synthetic recycle callback verifies the destination's content before removing the original.
- `TestFreedesktopTrash/other-volume` performs an actual same-volume move into `/dev/shm/.Trash-1000`, created exclusively for that test. It checks private permissions, retained content, removal of the original and mount-relative escaped metadata. Cleanup removes only that fresh synthetic trash. The test skips rather than touching a pre-existing volume trash.
- Home-trash collisions and a symlink in place of the metadata directory are tested; the failure preserves the original file.

### Fixtures and unverified cases

- Save-data argument parsing, unreadable native arguments, Wine `C:`/`Z:` conversion and prefix drive mappings use a synthetic proc tree or prefix. An actual Wine/Proton installation was not used.
- Existing launch, texture, save, link-lifecycle, directory-move and uninstall refusal tests use injected `GameProbe` runs. Live procfs discovery is tested separately. No actual game was launched.
- `MoveDirectory` and lock-library cross-volume transaction/link tests retain their `SameVolume` fixtures. The real second-filesystem move test exercises `moveDir`; the shared device helper and trash are exercised on the real second device. No real bind mount or `EXDEV` from equal-device bind mounts was exercised.
- The kernel no-replace save path was exercised on Linux's case-sensitive filesystem. A Linux case-insensitive mount was not available; Windows exercises the corresponding filesystem-identity behavior through its own rename implementation.
- GUI, real games, real Mods, Steam, system account stores and real RimArcho data were not used. The Linux VM sessions in the release table remain pending.

## Remaining gaps and documentation discrepancies

| Location | Remaining work |
| --- | --- |
| `internal/app/uninstall.go:177` | Resolved in review: the managed root first defaulted to the data directory itself, which the overlap check refuses for managed Mod libraries; it is now the sibling `rimarcho-local` (17 §3). |
| `internal/source/link_health.go:140` | The base task's inode-reuse gap remains: a recreated same-target link can pass `os.SameFile` if its inode is immediately reused. The deterministic fixture retains the previous inode; hardening is separate work. |
| `internal/surface/gui/shell.go:547` | GUI volume labeling still uses `filepath.VolumeName`; GUI Linux support remains phase 2. |
| `internal/env/env.go:113`, `internal/menubg/unity_cache.go:13`, `internal/menubg/catalog.go:179` | Native game-layout and Steam/Proton detection belong to `codex/linux-detect`; no changes made here. |
| `internal/app/gamedl.go:104`, `internal/app/accounts.go:183`, `:219` | SteamCMD executable/lock paths still name `steamcmd.exe`; installer and related platform work belong to `codex/linux-steamcmd`. Only the necessary tool-directory argument was changed in the installer. |
| `internal/app/workshopsub.go:449` | Steam-client integration still looks for the Windows DLL; Linux client subscriptions remain outside phase 1. |
| `internal/app/gamerun_other.go:6`, `internal/app/toolbusy_other.go:7` | Platforms other than Windows/Linux retain unsupported process detection; macOS was not validated. |

The specifications previously described Windows-only game detection and refusal of all cross-volume Unix trash. They now describe this implementation. The Linux plan's initial survey table remains a dated account of the original findings; its phase-1 checklist records completion. The base validation report remains historical rather than being rewritten.

## Resources and handoff

WSL work used `~/rimarcho-safety`, the existing `~/sdk/go` and module cache in `~/go/pkg`, a task-specific `~/go/cache-safety`, and `~/go/tmp-safety`. At the end of validation, the clone, task build cache and static binary measured approximately 103, 722 and 44 MiB; temporary build usage was separately observed at 166 MiB during execution. This stayed below the 1.5 GB allowance for the WSL artifacts. Windows reused its pre-existing shared `D:\go-build` cache; no other task's shared cache was globally cleared.

The task-specific Linux build cache and static binary are removed after validation; the clone and small logs remain. Linux logs are `~/go/safety-linux-tests.log`, `safety-focused-app.log`, `safety-focused-source.log` and `safety-staticcheck.log`; selected packages are in `~/go/safety-packages.txt`. Windows logs are `%TEMP%\rimarcho-safety-windows-tests.log` and `%TEMP%\rimarcho-safety-node-tests.log`.

No merge, push or edits to another task's branch/worktree were performed.
