# Investigation of broken `_Archo_env_` links

2026-10-24; code baseline `7386f87`, line references refer to the workspace at investigation time. Read-only investigation; only this document was added. RimArcho, the game, tests, and repair commands were not run. No game files, player AppData, LocalLow, or existing repository files were changed; no commit. Times are UTC unless stated otherwise.

**Conclusion: a defect in RimArcho's link lifecycle.** Switching environments first created cross-game local links. Later, "move into the lock library" moved the target from drive D to C, updating only the Instance path in the database and leaving inbound links in other games untouched. Normal save unconditionally preserves `_Archo_env_`, while scanning and duplicate checks skip broken links. A normally completed operation is therefore enough to trigger this; no crash is needed. The move, Mod identity, and target location are confirmed. Without a per-operation audit log, the button clicked, exact CLI command, and worktree of the executing process cannot be determined.

## 1. Evidence and timeline

### E1: Actual link and target

Read-only commands executed:

```powershell
$link = 'D:\rimworld16\Mods\_Archo_env_yvae4hr7j2zwsjepwkqalnykwq'
Get-Item -Force -LiteralPath $link |
  Select-Object FullName,LinkType,Target,CreationTime,CreationTimeUtc,Attributes | Format-List
fsutil reparsepoint query $link
Test-Path -LiteralPath 'D:\SteamLibrary\steamapps\common\RimWorld\Mods\_3700765558_alt_decapitate'
```

Output excerpt:

```text
LinkType        : Junction
Target          : D:\SteamLibrary\steamapps\common\RimWorld\Mods\_3700765558_alt_decapitate
CreationTime    : 2026-10-22 16:32:48 (local PDT)
CreationTimeUtc : 2026-10-22 23:32:48
Attributes      : Directory, ReparsePoint
Reparse Tag Value : 0xa0000003 (Mount Point)
Substitute Name: \??\D:\SteamLibrary\steamapps\common\RimWorld\Mods\_3700765558_alt_decapitate
Test-Path: False
```

Also checked all prefixed entries in both known games with the following command (`@(...)[0]` ensures the full target is read, rather than the first character of a string):

```powershell
foreach ($root in @('D:\rimworld16','D:\SteamLibrary\steamapps\common\RimWorld')) {
  Get-ChildItem -Force -LiteralPath (Join-Path $root 'Mods') |
    Where-Object { $_.Name -like '_Archo_*' -or $_.Name -like '!ArchoPack_*' } |
    ForEach-Object {
      $targetPath = @($_.Target)[0]
      [pscustomobject]@{
        Path=$_.FullName; LinkType=$_.LinkType; Target=$targetPath
        TargetExists=$(if ($targetPath) { Test-Path -LiteralPath $targetPath })
        CreatedUTC=$_.CreationTimeUtc.ToString('o')
      }
    }
}
```

| Game | Result |
| --- | --- |
| `D:\rimworld16` | 7 junctions: 5 env, 1 locked, 1 monitor; only this document's env target is missing |
| Steam RimWorld | 4 env junctions, all targets exist |
| Total | 11 links, 1 broken, no `!ArchoPack_` |

1.6 also has a valid `_Archo_locked__3700765558_alt_decapitate`, created at `2026-10-23T20:58:49Z`, pointing to `%LOCALAPPDATA%\RimArcho\locked\_3700765558_alt_decapitate`. The old env link was not replaced. The lock-library directory and original Mod contents still exist.

### E2: The database proves a move, rather than cleanup of a temporary test directory

Read `%APPDATA%\RimArcho\archo.db` using the Python SQLite URI **`mode=ro&immutable=1`**, avoiding database, WAL, and SHM writes; no migration/checkpoint. Limitation: this reads only the checkpointed main database, excluding newer WAL transactions, so it cannot establish that recent operations did not occur. The following historical evidence is already in the main database.

Reproducible read-only queries:

```powershell
python -B -c 'import sqlite3,os; p=os.path.join(os.environ["APPDATA"],"RimArcho","archo.db"); c=sqlite3.connect("file:"+p.replace(chr(92),chr(47))+"?mode=ro&immutable=1",uri=True); mid="yvae4hr7j2zwsjepwkqalnykwq"; print(c.execute("SELECT id,real_path,source_id,status FROM instance WHERE id=?",(mid,)).fetchone()); print(c.execute("SELECT made FROM instance_note WHERE instance_id=?",(mid,)).fetchone()); print(c.execute("SELECT id,scan_id,package_id,name,origin FROM snapshot WHERE instance_id=? ORDER BY id",(mid,)).fetchall()); print(c.execute("SELECT id,name,game_root FROM environment").fetchall())'
Get-Content -LiteralPath "$env:LOCALAPPDATA\RimArcho\locked\_3700765558_alt_decapitate\.rimarcho\origin.json"
```

Key output:

```text
Instance: yvae4hr7j2zwsjepwkqalnykwq
package_id: touma.beheading.dolls
name: Beheading & Animatronic Dolls
status: live
real_path: C:\Users\Touman_Lin\AppData\Local\RimArcho\locked\_3700765558_alt_decapitate
source_id: qra4ztcixedbnoxtdb3acaflf4 (extra / locked, shared source)
instance_note.made:
  {"kind":"move","at":"2026-10-23T04:28:56.890403Z","plans":["Test"]}
snapshot 1254 / scan 1: origin=""
snapshot 7187 / scan 40: origin.copied_at="2026-10-23T04:28:53Z"
origin_instance: fdg5n42ixgrny76jo55mkk3osy (Workshop upstream, not the moved copy)
origin_lineage: r54h3wy43kvrof5dtuv4aduuhi
workshop_id: 3700765558
```

The on-disk `origin.json` matches the snapshot. Here, `copied_at` is when the marker was backfilled; alone, it cannot prove a pin. **`made.kind=move` explicitly identifies this move**, recorded by `internal/app/libraryact.go:151,165` after success. Also queried `scan WHERE id IN (34,40)`: 34 began at `10-22 23:32:48`, the same second the link was created; 40 was `10-23 04:28:53–56`, local folders fell from 79 to 78, and the locked source first appeared (1 Mod), with no events, consistent with an identity-preserving move.

Querying `instance_alias WHERE instance_id=…` finds only valid locked aliases for both games, not the old broken env link. Aliases are scan output, not a reliable creation ledger; the database has no dedicated link-intent/operation log table either.

| Time | Evidence and meaning |
| --- | --- |
| 10-22 23:32:27 | `%APPDATA%\RimArcho\logs\gui.log:51`: started build `107808d805b0-dirty` |
| 10-22 23:32:48 | Five env links for 1.6 created in one batch; scan 34 began |
| 10-23 04:28:53–56 | Copy moved to lock library, marker backfilled, rescanned, `kind=move` and plan `Test` recorded |
| 10-23 20:58:49 | Valid locked/monitor links created in 1.6; old env remained |
| 10-24 | `archo test run` found game scanning failed (E3) |

Existing environments are `main` (Steam game) and `1.6` (`D:\rimworld16`); the current `1.6` record was created at `10-23 20:57:27`, later than the old link. The old environment record is no longer in the current table. Do not use the new environment ID as the original creation-operation ID; locate by physical game path, Instance ID in the link, and timeline.

### E3: Evidence limits for game failure and test ownership

The existing [game test-run investigation](2026-10-24_game-test-runs-rimdevtools.md) records at `:48`: 1.6 `ModLister` throws `DirectoryNotFoundException` when scanning this link, followed by `ModsConfig`/`ModLister` type initialization failure and a `Root level exception` every frame, about 47k log lines. `:58` records that the test used a temporary `RIMARCHO_HOME` and game link counts did not change; `:63` records 6 exception clusters and exit code 1 even with Core alone. Although Knowledge.xml was written, menu initialization still failed; a `menu` flag alone cannot establish success.

The game was not rerun in this investigation. Exception evidence comes from the existing report; the original run log was not found for independent line-by-line verification. Roaming logs contain only `gui.log`, `pack-build.log`, and `wezterm-start.log`; searching `yvae4hr7|3700765558|decapitate` found no matches. GUI logs contain only startup version/timing. That run's Player.log/result.json was not found in the game-test worktree either. These records cannot establish that a particular worktree's process created the link.

## 2. All production creation entry points and cleanup contracts

Search commands:

```powershell
rg -n '_Archo_env_|_Archo_|!ArchoPack_|junction|symlink|mklink' internal
rg -n 'CreateLink\(|os\.Symlink\(|FSCTL_SET_REPARSE_POINT|applyLinks\(' internal -g '*.go' -g '!**/*_test.go'
git log --all --date=iso-strict --format='%h %ad %s' -- internal/app/envlinks.go internal/app/deploy.go internal/app/locklib.go internal/app/directories.go internal/app/packlinks.go
```

| Creation entry | When and target | Removal/preservation |
| --- | --- | --- |
| `internal/app/deploy.go:26,137,218`; `loadout.go:834` | Save/launch plan; `_Archo_<source-tag>_<rel>` → enabled extra/SteamCMD instance real directory, including locked, monitor | Remove unneeded/mismatched normal deployment links before creating; `:167` preserves all env links unconditionally, without health checks; no cleanup on exit |
| `internal/app/envlinks.go:28,140,148,201`; `envs.go:571,589` | `EnvUse → PrepareEnv → prepareEnvLinks`; fill local Mods from the previous environment according to target config/workspace; `_Archo_env_<Instance ID>` → real Mod directory in the original game; shared Mods use normal deployment names | Treated as installed library; retained across plan changes/disabling/saving; `:190` rolls back newly added links on batch creation or scan failure; cleaned on uninstall. No post-success tracking of target moves |
| `internal/app/pack.go:748,872,902,914` | Launch pack; `!ArchoPack_<sanitized-pack-name>_<directory>` → `<pack>/mods/<directory>` | Switching packs clears other/wrong pack links; writing a normal plan clears pack links; `pack unlink` clears the current game; retention after exit is existing design. `RemovePack:1085` clears only the current environment before recycling the pack, leaving a gap for other games |
| `internal/app/directories.go:435,495,508` | Cross-volume managed-directory move: detach its owned game Mods links, rebuild at the new location, rebase link locations and internal targets | Attempt to reconnect in place if copying fails; this path is not used by single-Mod moveToLibrary |
| `internal/app/directories.go:517,561,562` | After moving/relocating a managed directory, iterate environments, Readlink old targets, remove and recreate under the new root; try old targets on failure | Handles broken links, but repairs after the move, leaving an interruption window; not in the single-Mod move-to-lock-library call chain |

On Windows all creation reaches `internal/source/link_windows.go:69`: create an empty directory, then write a mount-point junction with `FSCTL_SET_REPARSE_POINT`; other systems use a symbolic link in `link_other.go:27`. `mklink /J` in `internal/testkit/link_windows.go:10` is a synthetic test helper, not another production entry.

Deletion uses `source.RemoveLink` (`link_windows.go:121`): verify a link, then a single `os.Remove`, without entering the target. Uninstall `uninstall.go:267` enumerates by prefix and preserves real directories; step `:44` handles links before locked/pack targets. This is more complete than single-Mod moves.

## 3. Root cause, exact sequence, and history

**F1 (E1, E2, high confidence): successful library moves omit cross-environment inbound-link maintenance.** Call path P1:

1. During an environment switch, `prepareEnvLinks` creates `_Archo_env_yvae… → Steam/Mods/_3700765558_alt_decapitate`; the suffix is the original Instance ID (E1; `envlinks.go:148`).
2. "Move to lock library" follows `MoveToLockedFor → moveToLibrary` (`libraryact.go:151`). D → C crosses volumes via `locklib.go:149,181`: copy and verify, then `ToTrash(from)`; the old path disappears. Same-volume Rename at `:146` also breaks links.
3. `locklib.go:157` calls `Store.RelocateInstance`; `internal/store/library.go:33` only UPDATEs the Instance source/path, then rescans, without detaching/redirecting other games' inbound links. `linkedBy` (`dupmove.go:129`) protects only external-manager `CopyPlayer` links in the current game, not these owned links.
4. The move succeeds and records `made.kind=move` / `Test` (E2). A locked link is created later; the old env link still points to the old location (E1). The sequence can complete without a crash.
5. `source.List` (`source.go:140–146`) skips resolution failures; `ModsCopies` (`modscopies.go:99–102`) also skips them, with a comment incorrectly claiming the game cannot see them; `planLinks` (`deploy.go:167`) preserves the broken link. Thus the database problems page misses it, while game scanning crashes (E3).

Merely changing to "record DB before creation" cannot fix a successful move; all inbound links must be maintained before the target disappears. Nor can the old env simply be redirected to the lock library: a valid locked link already exists locally, which would register the same Mod twice / duplicate its package ID.

Other candidates: `Trash` (`hidden.go:252`) recycles directly and then scans, with a similar risk; directory moves already redirect links, and uninstall already detaches first. Tests in `gametest.go:110–148` only prepare isolated data and launch, without creating Mods links; existing measurements also record unchanged link counts. Evidence supports **moving one Mod into the lock library**, with none supporting uninstall, temporary-test-directory cleanup, or a crash.

| Commit | Relationship |
| --- | --- |
| `3b070cf`, 10-20; `2499d59`, 10-20 | Introduced normal link deployment and packs respectively |
| `29eb24e`, 10-21; `af1b5bf`, 10-22 | Preserve Instance on move to lock library; extend to moving blocking copies |
| `c48a200` / `46e431b`, 10-22 | Add pack.links-left and duplicate pack-link handling, without this env lifecycle |
| `ca6d353`, 10-22 16:36:38 PDT | Introduce `_Archo_env_` and the normal-save preservation exception; local creation was about 4 minutes earlier, but the GUI was a dirty development build, so commit time cannot rule out running new code before commit |
| `451288a`, 10-23 | Repair links when moving/locating managed directories; later than this move at 10-22 21:28 PDT, and not wired into moveToLibrary |

No single-Mod cross-environment inbound-link maintenance fix was found through the current baseline.

## 4. Prevention design

- **Manage target lifecycles first.** Enumerate inbound links across all known games' Mods through a shared mechanism for single-Mod moves, managed-directory moves/relocation, trash, pack/subscription removal, and uninstall; deduplicate physical Mods directories rather than checking only the current environment or scan aliases. Report external-manager links only; refuse a move that would break them and explain why.
- **Detach before targets disappear.** Same volume: persist plan → detach owned inbound links → Rename → update Instance → rebuild necessary entries. Cross-volume: copy and verify → persist plan → detach → recycle old directory → update Instance → rebuild. Reconnect on failure only if the target exists, avoiding broken-link recreation by low-level CreateLink; previews include all affected games. Rebuilding preserves cross-environment installation semantics, leaving one valid entry per instance rather than both env and locked links.
- **Record creation/change intent first.** Add a small managed_link ledger: physical game path, link name/type, instance or pack identity, original target, data-directory identity, operation ID/phase; also from/to for moves. Commit intent before filesystem changes, complete the record on success; roll back in reverse on failure and retain failure state. DB transactions cannot make file operations atomic; phase records support recovery, rather than replacing filesystem coordination with an early DB path update.
- **Check interrupted state on startup; exit is only supplementary.** On restart, compare plans and actual links read-only and generate a recovery preview; recover inside an authorized write action. Do not rely on exit hooks or clear valid env/pack links on exit. An offline source drive does not mean a Mod was deleted.

## 5. Automatic detection and safe repair

### Detection

Add read-only InspectGameLinks: obtain known Mods directories from `Store.Envs` game roots, valid local sources, and historical ledger; normalize and deduplicate. Read only the first level with `ReadDir + Lstat`, verify owned prefixes and junction/symlink tags, then use `Readlink`/reparse data to obtain raw targets without requiring target existence. Do not claim coverage of unregistered games absent from the ledger; ask players to register them.

Do not reuse normal Mod lists (broken/pack links are filtered out); do not cache only Mods mtime (external target deletion need not change it). Probe target status on every check.

| Category | Criteria and handling |
| --- | --- |
| `mods.link-dangling` | Target definitely absent and parent drive/source accessible; error, affects the entire game regardless of plan enablement |
| `mods.link-foreign` | Target identity differs from ledger expectation, or prefixed link cannot be associated with a known instance/pack; show ownership evidence and raw target. Valid env targets are player directories, so "outside RimArcho's data directory" cannot establish foreign ownership |
| Unknown status | Access denied, offline drive/network, unknown reparse type; report check failure, preserve unchanged, never treat as missing |
| Prefixed real directory/file | Report a name collision, offer no deletion action |
| Valid but redundant | Duplicate entries go to the duplicate-package-ID rule; valid leftover packs to pack.links-left; retain valid env links |

Scanning and GUI/TUI startup check all known games; needs/can it run appends live results whenever fetching findings, supports game/path subjects without an Instance, and does not put them in the Def/Patch cache. Recheck the selected game before plan/pack launch and refuse if broken links remain; summarize other games without blocking a healthy game. `test run` checks before preparing data/starting a process; broken links in the tested game return structured failure (path, target, reason), without launching or changing Mods.

### Repair permissions and actions

`docs/specs/01-scope-and-status.md:72` (§4) requires previews, GUI confirmation or explicit CLI execution; `:81` allows management only of owned prefixed links. `docs/specs/08-launch-and-packs.md:332` (§7) and `trash.go:52` reject links in the recycle bin. Therefore:

1. **Detect automatically; repair with confirmation by default.** Proposed interfaces: `archo links check --all` (read-only), `archo links repair --all --dry-run`, and explicit repair execution. GUI lists full paths, targets, reasons, and delete/rebuild actions across games in one confirmation. These interfaces are proposed, not currently available.
2. Operate only on first-level known Mods entries with owned prefixes that are actual junctions/symlinks; immediately before execution revalidate parent path, type, file identity, and raw target, aborting if changed since preview. Delete a checked single link: no RemoveAll, no recursion, no resolving to the target before deleting, no target writes. Implementation must prevent concurrent replacement with a real directory using a fixed parent directory/handle or equivalent identity checks; serialize cross-process writes.
3. Detach broken/retargeted links whose ownership is established by the ledger; propose rebuilding only after validating the new target. Do not silently delete legacy/foreign links with only a prefix and no ledger: show evidence, and detach only after the player explicitly confirms RimArcho ownership; "ownership unknown" may remain. Never delete unowned prefixes, real directories, or unknown reparse types.
4. Detaching is deployment reversal under 08 §3/§5.3: safely delete the link itself, **without ToTrash or expanding the recycle-bin allowlist**. Real Mod deletion still uses the recycle bin under §7; do not recycle the target if inbound detachment fails.
5. Confirmed move/delete/save/launch previews may include necessary detachment and then finish automatically without per-link questions. Scanning, opening problems, app startup, and test run do not authorize deletion. Silent startup repair would first require §4 to define standing authorization; this design does not introduce it.
6. While an affected game runs, detect only; do not change Mods. Save pre-operation link information for rollback. Rebuild only if the target exists and no duplicates result; otherwise explain that a usable state cannot be restored.

**Proposed local repair: detach only old `_Archo_env_yvae4hr7j2zwsjepwkqalnykwq`, preserving the existing valid locked link and target.** The other ten links are valid; do not clear them because of this fault. Not performed in this investigation.

## 6. Pending code and spec changes

| Code scope | Change |
| --- | --- |
| source/link_windows.go, link_other.go | Read raw targets/link types, distinguish missing from inaccessible, safe single-link deletion |
| New app inspection/repair module; envlinks.go, deploy.go | Shared enumeration/planning; verify health before retaining env; ledger, rollback; env/locked deduplication |
| locklib.go, libraryact.go, dupmove.go, directories.go | Inbound-link maintenance across all games, detach/move ordering, phase recovery, previews |
| hidden.go, pack.go, subscribe/unsubscribe, uninstall.go | Clear inbound links in all games before target disappearance; RemovePack beyond current environment; retain links-first uninstall ordering |
| store | Link ledger/phase records, migrations and queries; scan aliases do not replace a ledger |
| scan.go, problems.go, launch.go, gametest.go, view/needs, CLI/MCP/GUI/TUI | Shared read-only checks, game-level findings, preview/confirmation actions, pre-test failure and structured reports; mark MCP repair as mutation |
| Synthetic regressions | Two-game cross-environment links followed by same-/cross-volume library moves, trash/pack removal; failure/interruption recovery at every phase; foreign prefixes, real directories, unknown tags, offline drives; concurrent directory replacement; env+locked deduplication; test run detects without launching or writing the game. Never use player paths in tests |

Spec ownership (listed only, not changed here):

- **01 §4/§4.1**: link repair, move/delete coordination, recovery timing and confirmation boundaries; uninstall ledger.
- **08 §3**: link ownership, retain only healthy env links, primary target lifecycle/recovery contract; **§5.3/§5.4** cross-game pack inbound-link cleanup; **§6/§7** detach before recycling targets, no recycle bin for detachment or allowlist expansion; **§8** acceptance; **§12** read-only test run precheck and failure report.
- **02 §0 item 14, §3.3, §4.1, §7/§8**: link lifecycle, scanning no longer silently drops exceptions, ledger/migrations/acceptance; normal aliases still do not create a new Instance.
- **04 §7.4** and **06/2.9-inventory-and-logs.md §2.9**: preview affected links on library moves, details reference 08 §3; **06/2.1-navigation-and-launch.md §2.3/§2.4**: launch blocking, cross-game findings, one-confirmation repair.
- **03 §4.3**: link finding codes/severity/path subjects; **05 §5**: inspection/repair tools, dry-run and write permissions; **07 §3/§4**: ledger location and synthetic verification without touching player data.
