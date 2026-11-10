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

Validation results and release sizes are recorded after the local checks complete.
