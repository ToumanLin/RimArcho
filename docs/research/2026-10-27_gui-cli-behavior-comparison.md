# GUI and CLI behavior comparison (2026-10-27, dev/0.9.0)

Purpose: establish a basis for "CLI automation / semiautomatic scripts covering GUI operations". Trace each GUI API (`*API` methods in `internal/surface/gui`) to `internal/app` / `internal/view` functions, then find `archo` commands reaching the same functions.

**Review update, 2026-10-27**: baseline `0b30d42` from `codex/automation-research` / `dev/0.9.0`. Static enumeration now finds 184 exported `*API` methods and 132 command registrations (including aliases and entry points), replacing the draft estimate of "about 180 / 80". `codex/background-nonblocking@bd8d5d8` fixes D1 but is not merged into this baseline; a branch fix cannot be marked complete here. D1–D10 numbering is retained; items marked "updated/new" below are changes from this review.

The full action inventory, per-item priorities, three driver layers, first 10 scenarios, and VM plan are in the [VM automation test plan](2026-10-27_vm-automation-test-plan.md). This page records behavioral differences; the plan owns implementation details.

## Conclusions

- **Mostly consistent.** The shared-core design in 13 §1.1 largely holds: most domain writes call the same `app.*` functions; per-plan undo stacks live in the database and work across processes (`internal/store/loadout.go:57`); launch locks, link-health checks, ModsConfig backup/atomic replacement live in app. Permission while a game runs depends on the action, not a blanket "reject while running": saving can write configuration for the next launch (`internal/cmds/loadout.go:381`), while some file changes are rejected. Many GUI actions build commands and call `cmds.Execute` (previews, most settings, inspection, uninstall). **Updated: new Workshop subscription tasks call SubscribeWorkshopBatch directly, so adding subscriptions cannot all be described as the same command** (`internal/surface/gui/subscribe.go:128`). Scanning and analysis still launch `archo scan` / `archo analyze` subprocesses.
- **Not fully consistent.** Some GUI paths do extra work in gui/view or take a different path; see "Differences to fix" below. Several GUI actions also lack CLI commands.

## Differences to fix (by importance)

| # | Difference | GUI | CLI | Impact |
|---|---|---|---|---|
| D1 | **Batch trash / unhide bypass link lifecycle (updated: fixed in flight, still present in baseline)** | Baseline `gui/tasks.go:411` `batchFiles` still uses `TrashPlan` then `TrashPath` (`app/batchfiles.go:31`) or `os.Rename`, ending with one `Scan(false)` | `trash` / `unhide` → individual actions in `app/hidden.go:120`, `:203` and `scanAfterFileOp` (`:270`) | `codex/background-nonblocking@bd8d5d8:internal/surface/gui/tasks.go:386` now uses a detached session calling `a.Trash` / `a.Unhide` per item, sharing link lifecycle. After merge, rerun cross-environment link, cancellation, and continue-after-failure scenarios; do not reimplement |
| D2 | **Different position algorithm for Space enabling** | `view/verbs.go:103` calls `SuggestAt(whole group)` then `Enable(At)`, inserting a block | `enable` without position appends, then places individually with `order.Place` (`app/loadout.go:598`); `attach` rules handled separately | Multiselection with attach rules may place differently. CLI has no suggestion-only command, so scripts cannot reproduce GUI |
| D3 | **Toggle restores missing entries differently in place (updated)** | `view/verbs.go:105` supplies `At`, bypassing positionless replacement in `app/loadout.go:578` | Without position, replacement can occur in place | New UseInstalled (`app/useinstalled.go:13`) preserves position, revalidates batches, and has one undo step; CLI has `loadout use-installed`. It does not change Toggle, so D3 is not entirely gone. Still code-derived, not reproduced on a real machine |
| D4 | **Different "currently edited plan"** | Sidebar-opened plan exists only in GUI process memory (`app/plan.go:369`); edits, undo, save, pack build, and analysis act on it; subprocesses get `--open-plan` | Defaults to current plan; `--open-plan` exists (`surface/cli/cli.go:194`), but `archo help` lists only `--env` | Same command in GUI `:` panel and terminal may edit different plans. Scripts must explicitly set `--open-plan` |
| D5 | **Granularity of existing subscription updates (wording updated)** | Executes each ref within one task (`gui/tasks.go:84`), continuing after failures; page updates skipped at submission (`:118`) | Whole-group `subscribe update` → UpdateSubscriptions (`cmds/subscribe.go:73`); page requires --from | Different result order, skips, and failure handling. This is not the new Workshop subscription task; that redesign is below |
| D6 | **Unsubscribe / resubscribe fault tolerance** | Per-item dry run; a bad ref fails only that item; Steam batches of 50; opens Workshop page if Steam absent | One bad ref fails the entire command | Scripts must filter individually |
| D7 | **Concurrent pack builds** | Rejects if any pack is building (`api.go:343`); detached subprocess continues after window closes | Rejects only same-name pack (`app/pack.go:415`), synchronous in-process | CLI can build two packs concurrently |
| D8 | **Needs-page "convert textures" (location updated)** | `gui/api.go:1053` holds lock while calling synchronous OptimizeTextures in `view/needs.go:1017`, bypassing texture-page subprocess/stop entry | CLI has textures optimize, no textures stop | GUI's two entries schedule differently; needs should use unified task entry; CLI stop should call StopTextures |
| D9 | Switch environment after game download | Switches (`shell.go:460`) | Does not switch | Script must add `env use` |
| D10 | Minor differences | Trim notes; collection preview defaults to follow; collection checks throttled 10 minutes; stale `problems` launches analysis subprocess | Notes unchanged; `collection --import` defaults not to follow; `plan sync --check` always connects; `problems` analyzes in-process | Usually harmless if scripts account for them |

### New capability review (this update)

| Capability | GUI / app evidence | Current CLI equivalent |
|---|---|---|
| needs / ListProblems cache and publication | `internal/surface/gui/needscache.go:39`, `:105`, `:139`; ListProblems uses the same Needs, background publication keyed by context and version | `problems` shares problem core; no sequence, updating, list-problems events, or GUI cache contract. Compare final facts; test cache/frontend consumption separately |
| Four sort-notice categories | `internal/view/preview.go:47`, `:78` return missingDependencies / missingMods / replacements / cycles | `sort [--dry-run] [--json]` shares `app.SortNotices` with GUI; text/JSON include installed/remote/unknown dependencies, copies for missing entries, recommended replacements, cycles |
| CompleteDependencies | `internal/surface/gui/ops.go:128` → `internal/app/dependencies.go:164`, enables installed dependency closure and sorts, one undo | `loadout complete-dependencies`, same app operation and single undo; remote missing dependencies reported only |
| ReplaceRecommended | `internal/surface/gui/ops.go:137` → `internal/app/sortnotices.go:206`, whole-batch revalidation/replacement | `loadout replace-recommended <ref…>`, same app batch revalidation and single undo |
| UseInstalled | `internal/surface/gui/ops.go:145` → `internal/app/useinstalled.go:13`; also triggered by needs | `loadout use-installed <packageId…>`, same app in-place batch repair and single undo |
| SteamCMD batch credit back to Steam | `internal/surface/gui/subscribe.go:89`, `:146` → SubscribeWorkshopBatch; `internal/app/credit.go:19`, `:97`, `:127`; `internal/surface/gui/credit.go:35` | CLI subscribe workshop calls only SubscribeWorkshop (`internal/cmds/subscribe.go:67`); no public credit-batch / prompt / answer / run. Internal GUI arguments are not registered CLI flags. Credit is a domain gap, not "intentionally omitted" alongside account login |
| Subscribe task redesign | `internal/surface/gui/subscribe.go:128`, `tasks.go:165`: per-item progress, permanent failures, cancellation, remaining-only continuation, same-row recovery | Some core subscription logic shared; CLI lacks GUI task ID / same-row recovery / task controls. Share per-item results; test window cancellation/feedback separately |
| Game-loading smoke test | `internal/cmds/test.go:15` test run, `internal/app/gametest.go:80` isolates configuration/logs | Existing `archo test run --json`, dedicated CLI-only entry; usable for game-load validation, but assert empty Missing and actual loaded list, not just process or exit code |

## GUI actions without CLI equivalents

**CLI additions needed for automation:**

| GUI action | GUI entry | CLI currently can only |
|---|---|---|
| Structured problem publication (new) | `ListProblems` | `problems` shares core; no GUI sequence/updating/event contract |
| Steam credit batch/answer/advance (new) | `Subscribe(Batch)` / `CreditPrompt` / `CreditAnswer` | No public equivalent; explicit policy/state entries needed |
| Enable at suggested position / show suggestion only | `Toggle` (`SuggestAt`) | Cannot reproduce, see D2 |
| Move multiselection as a block or by filter results | `MoveBy`, `Place`, `MoveShown` | `move` accepts 1 ref; scripts calculate `--to` individually |
| Stage all | `StageAll` | List all non-Core/DLC entries, then `stage add` |
| Place multiple staged entries at a specified position | `Place` | Split into `enable --at` + `stage drop` |
| Merge import | `ImportApply(merge=true)` | None |
| Import pasted text / Rentry link | `view.ReadImport` (`view/exchange.go:55`) | Files and share codes only |
| Import collection as a new followed plan | `ImportAsPlanPreview(follow)` | `collection --import` only replaces workspace |
| Update to Workshop version (overwrite locked copy in place) | `LibraryDo(LibPinUpdate)` → `UpdateToWorkshop` | `pin --update` is another function (rebuild in a new folder) |
| Ask Steam client to update a Workshop Mod | `WorkshopUpdate` → `UpdateWorkshopClient` | `workshop resubscribe` approximates it but deletes/redownloads |
| Check updates for an entire source | `CheckSubscriptionSource` | `subscribe check` individually |
| Stop texture conversion | `TexturesStop` | Kill process or write stop file |
| Add Archo Monitor (after Harmony) | `AddMonitor` | `monitor install` uses `EnsureMonitor`, a different function |
| Open a newly created plan | `NewPlan` | After `plan new`, explicitly `plan use` or `--open-plan` |
| Batch `trash` / `pin` / `unpin` / `unhide` / `library move-to-locked` | Multiselection | These commands accept 1 ref; script loops |
| Background tasks: queue, stop, retry, continue | `Tasks`, `TaskAct` | Synchronous CLI, Ctrl-C interruption |

**Account/authentication interactions are currently GUI-only:** SteamCMD / LoversLab login, verification, logout, forget (`gui/accounts.go:18`), LoversLab file browsing/installation. Authentication dialogs and human takeover in automation will be handled separately later. **Updated: Steam credit moves to the domain gap, no longer considered "intentionally unavailable in CLI" with login.** VM may use the user's real Steam account within accepted scope; client subscribe/unsubscribe still changes global account state.

**UI-only, no CLI needed:** open folder/web page, file/folder pickers, clipboard, background image, `ui.gui.*` layout preferences, home-selected pack, directory validation and live package-ID checks in onboarding, reference-number mode.

## CLI capabilities beyond GUI

`swap <ref> <ref>` selects a copy (GUI reports ambiguity with more than 2 copies); `pin --name/--git/--reason`, `library move-to-locked --reason`; `textures optimize --force/--no-mipmaps`; `pack import x.archolist.json --as-plan`; `plan sync --dry-run`; `rule add/list/remove`; commands' `--dry-run`. **New: `test run [plan] --json [--map]` is an existing game smoke entry** (`internal/cmds/test.go:15`); no dedicated GUI button, but callable via RunCLI.

## Comparison table (GUI action → CLI)

✅ Same main domain function; state assertions still required, not acceptance of GUI scheduling/presentation. ≈ Different arguments, composition, or presentation. ⚠ Differences above. ❌ No CLI. **New verification boundary**: staging meta is written separately (`internal/app/stage.go:57`, `:126`); UndoRedo restores only workspace entries (`internal/app/loadout.go:839`). Agreement on both sides therefore does not prove full "list + staging" undo; VM plan makes it an independent oracle.

### Plan editor

| GUI action | Key / API | CLI | |
|---|---|---|---|
| Enable | space / `Toggle` | `enable <ref…> [--at n]` | ⚠ D2 D3 |
| Disable | space / `Toggle` | `disable <ref…>` | ✅ |
| Stage / put back / put all back | `t` `T` / `Stage` `Toggle` `RestoreAll` | `stage add` / `stage restore [<ref>]` | ✅ |
| Remove from staging | Drag / `Unstage` | `stage drop` | ✅ |
| Move up/down 1 or 10 positions | `J` `K` `alt+J` `alt+K` / `MoveBy` | `move <ref> --up/--down/--to` | ≈ Single entry only |
| Move to… | `m` / `Place` | `move --to/--before/--after`, `enable --at`, `stage restore --at` | ≈ Single entry only |
| Switch copy | `x` / `Swap` | `swap <ref> [<ref>]` | ✅ CLI more capable |
| Sort / save / launch | `S` `w` `L` / `Run` `RunMoving` | `sort` / `save` / `launch [--move-duplicates]` | ✅ |
| Sort notices and completion (new) | `Run("sort")`, `CompleteDependencies`, `ReplaceRecommended`, `UseInstalled` | `sort [--dry-run] [--json]` shares `app.SortNotices`; three repair commands above | ✅ Same domain paths/single undo, window scheduling tested separately |
| List problems and needs cache (new) | `Needs`, `ListProblems`, `list-problems` | `problems` | ≈ Shared problem core, cache/events not equivalent |
| Preview | `Preview` | `sort/save/launch --dry-run` | ✅ Same command |
| Undo / redo / history | `u` `ctrl+r` `H` | `undo [n]` / `redo [n]` / `history` / `loadout checkout v<n>` | ✅ Shared across processes |
| Tags / notes | `g` `n` | `tag add/remove` / `note [--clear]` | ✅ (note trimming: D10) |
| Plan: create, switch, rename, remove, game versions | `NewPlan` `UsePlan` `RenamePlan` `RemovePlan` `PlanVersionsSet` | `plan new/use/rename/remove --yes/versions` | ✅ (open after creation: D4) |
| Open without making current | `OpenPlan` | Global `--open-plan <name>` | ⚠ D4 |
| Follow collection / unfollow / sync / check | `PlanFollow*` `PlanSync` `PlanCheck` | `plan follow/unfollow/sync [--check]` | ✅ |
| RAM / video memory estimate | `PlanLoadCost*` | `plan cost` | ✅ |
| Export | `ExportSave` `PlanExport*` | `loadout export --format --out` | ≈ No clipboard or overwrite of existing files |
| Import (replace / new plan) | `ImportApply` `ImportAsPlan` | `loadout import <file\|code> [--new]` | ≈ See missing actions |
| Command line | `:` / `RunCLI` | Any command | ⚠ Acts on GUI-opened plan |

### Mod inventory and subscriptions

| GUI action | API | CLI | |
|---|---|---|---|
| Browse, filter | `Library` | `library [--search --include --exclude --sort]` | ≈ |
| Add to current plan | `LibraryDo(enable)` | `enable` | ⚠ D2 |
| Remove from all plans | `LibraryDo(remove)` | `library remove-from-plans` | ✅ |
| Lock / unlock | `LibraryDo(pin/unpin)` | `pin` / `unpin` | ✅ (GUI background task) |
| Update to Workshop version | `LibraryDo(pinupdate)` | — | ❌ |
| Move into lock library | `LibraryDo(move)` | `library move-to-locked` | ✅ |
| Trash / unhide | `LibraryDo(delete/unhide)` | `trash` / `unhide` | ⚠ D1 |
| Unsubscribe / resubscribe | `LibraryDo(unsub/resub)` | `workshop unsubscribe/resubscribe` | ⚠ D6 |
| Copy note | `LibrarySetNote` | `note --copy` | ✅ |
| Details / inspect | `Detail` `Inspect` | `mod` `describe` `problems` `patches` `def` `harmony` | ✅ |
| Changes | `Changes` | `changes [--library]` | ≈ |
| Directory relocation | `DirectoryMovePreview`, etc. | `folder move <id> <dir> [--dry-run]` | ✅ |
| Add subscription (git / web / Workshop) (updated) | `SubscribePreview` `Subscribe` | `subscribe git/page/workshop [--dry-run]` | ≈ git/page mostly use same command; Workshop tasks use SubscribeWorkshopBatch; cancellation/continuation/credit in new review |
| Update using downloaded files | `SubscriptionFrom` | `subscribe update <ref> --from <zip>` | ✅ |
| Subscription check / update / remove | `LibraryDo(sub*)` | `subscribe check/update/remove` | ⚠ D5 |
| Workshop-client update, source-wide check | `WorkshopUpdate` `CheckSubscriptionSource` | — | ❌ |
| Accounts, LoversLab (updated) | `AccountAction` `Lover*` | — | GUI-only; special interactions handled later |
| Steam credit batch (new category) | `CreditPrompt` `CreditAnswer` | — | ❌ Domain gap, not an account-interaction exemption |

### Other pages

| GUI action | API | CLI | |
|---|---|---|---|
| Packs: list, manifest, create, stop, remove, export, import, launch | `Packs` `PackDryRun` `PackBuild` `PackStop` `PackRemove` `PackExport` `PackImport` `PackLaunch` | `pack list/build [--dry-run]/stop/remove [--unfinished]/export/import/launch` | ✅ (build: D7, D4) |
| Textures: statistics, install todds, convert, clean | `Textures*` | `textures status/tool --install/optimize/clean` | ≈ (statistics use different functions; matching counts unconfirmed) |
| Stop texture conversion | `TexturesStop` | — | ❌ |
| Environments: list, switch, remove | `Environment` `UseEnv` `RemoveEnv` | `env list/use/remove --yes` | ✅ |
| Settings | `ApplySetting` | `env set/rebind`, `folder *`, `config set`, `db *`, `cache clear`, `data update`, `deps *`, `gc` | ✅ Same commands |
| Onboarding: find, add, first scan | `GuideFind` `GuideAdd` `GuideScan` | `env detect` → `env add` → `env use` → `scan` → `analyze` | ≈ One action split into commands |
| Onboarding: install SteamCMD, download game | `GuideInstallSteamCMD` `GuideDownload` | `deps install steamcmd`, `env download <branch> --to` | ⚠ D9 |
| Launch, home ▶ | `Run("launch")` | `launch` | ✅ |
| Needs attention: one-click actions (updated) | `DoAction` | `enable` `disable` `rule add ignore` `loadout checkout` `identity resolve` `unhide` `unpin` `trash` `problems ignore` `pack unlink` `links repair` `swap` `textures *` `plan versions` … | Mostly ✅; UseInstalled has `loadout use-installed`; texture conversion scheduling: D8 (`internal/view/needs.go:965`) |
| Scan / analyze | `Scan` `Analyze` | `scan` / `analyze` | ✅ GUI launches these subprocesses |
| Logs: select log, cluster, pattern | `SelectLog` `LogCluster` `LogPattern` | `log --file --select`, `log --cluster`, `pattern` | ✅ |
| Add Archo Monitor | `AddMonitor` | — | ❌ |
| Uninstall RimArcho | `UninstallPlan` `UninstallRun` | `uninstall [--dry-run] --yes …` | ✅ |

## Script conventions

1. Supply `--env <name>` and `--open-plan <plan name>` on every command; never rely on "current".
2. Preview writes with `--dry-run` where available (not all commands, such as enable, provide it). Interpret exit codes: 0 success, 1 error, 2 usage error, 3 ambiguous reference (candidates printed). 0 cannot replace state assertions; an asynchronous GUI ack is even less proof of completion.
3. Use `trash` for deletion; baseline D1 still differs, so revalidate after the in-flight fix merges. Missing multiselect commands may temporarily use loops, without claiming identical GUI failure policy/history-step counts.
4. **New**: rapid keys, page/list switching, confirmation/cancellation, Esc, status text, and event consumption require a real Wails window; CLI cannot cover them. Reuse existing `RIMARCHO_DEVTOOLS_PORT` (`internal/surface/gui/gui.go:79`) and `scripts/vm/cdp.mjs:19`; authentication interactions will be listed separately later.
