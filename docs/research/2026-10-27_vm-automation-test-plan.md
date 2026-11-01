# VM automation test plan (2026-10-27)

## 1. Recommendation and verification boundaries

Phase 1 primarily uses **WebView2 CDP against the real Wails window**, covering ordinary fast input, navigation, cancellation, import/export, and adding/removing Mods. CLI runs core scenarios and state comparisons; guidev/direct `*API` verifies GUI services, events, and caches. All three layers use the same scenario semantics and snapshots; CLI success cannot replace window acceptance. Reuse the existing **VMware Workstation / Win11-Player-Test**, with a new test clone and data baseline.

Static code research and implementation proposal only: no VM restore, real subscribe/unsubscribe, deletion, or game launch. Old performance research is not current-build acceptance. No new PoC: extend the existing real-window driver.

Baseline: this `codex/automation-research` worktree, `0b30d42ab85ffc94eecc4c56df316bb3ca459047`; local `dev/0.9.0` pointed to it during review. Also checked `codex/background-nonblocking` `bd8d5d8` (includes `b3edbbd`), **not in this baseline**. Unqualified file:line references mean `0b30d42`; use symbols to recheck during implementation.

Current top-level Go files contain **184 exported `*API` methods and 132 `register(&Command{Name: ...})` entries**; the latter includes aliases/entry commands, not 132 independent player capabilities. Old "about 180 / 80" underestimated registrations. Coverage groups reads/writes/previews/polls/dialogs by player action. Multiple buttons for one action are one domain capability, but crawler must click each.

Reproducible read-only commands (repository root, PowerShell):

```powershell
git rev-parse HEAD
git log -2 --oneline codex/background-nonblocking
rg -n '^func \([^)]*\*API\) [A-Z]' internal/surface/gui -g '!**/*_test.go'
rg -n 'register\(&Command\{Name:' internal/cmds -g '!**/*_test.go'
git diff 0b30d42...codex/background-nonblocking -- internal/surface/gui/tasks.go
```

Scope follows actual GUI routes/APIs, checked against `docs/specs/06-ui/`: navigation/launch §2.1–2.4, lists/packs/environments §2.5–2.8, inventory/logs §2.9–2.10, settings/guide/tasks/all lists §2.14–2.17, and shortcuts/selection/details/confirmation in §3–6. Parent `inventory/ui-pages.md:37`, `:110`, `:198`, `:239`, `inventory/api-surface.md:29` describe RimCrow requirements, not all implemented RimArcho GUI features.

## 2. Current differences and fix order

### 2.1 Recent changes and D1–D10 review

| Item | Current evidence / conclusion | CLI equivalence and handling |
| --- | --- | --- |
| D1 bulk delete/unhide | Baseline `internal/surface/gui/tasks.go:411` still directly `TrashPath`/`os.Rename`, then one Scan; single CLI `internal/app/hidden.go:120`, `:203`, `:270` handles links/affected environments | **Different baseline paths**. `codex/background-nonblocking@bd8d5d8:internal/surface/gui/tasks.go:386` calls `a.Trash`/`a.Unhide` per item; after merge domain shared, task wrappers differ. Accept this fix first, no second bulk-delete implementation |
| D2 enable at suggested position | `internal/view/verbs.go:105` Toggle calls SuggestAt then Enable(At); `internal/cmds/loadout.go:146` directly Enable; without position `internal/app/loadout.go:555` appends each then positions | **Different paths**; move suggested-block placement+enable into app; add CLI `enable --suggested` and read-only placement preview, retain `--at` |
| D3 restore missing item in place | `internal/app/loadout.go:578` replaces missing entries only without explicit position; Toggle passes At. New `internal/app/useinstalled.go:13` is a separate repair | UseInstalled does not resolve D3. Add `loadout use-installed <packageId…>` to same function; test Toggle/explicit-position contract |
| D4 current vs open plan | `internal/app/plan.go:335`, `:354`; `internal/surface/cli/cli.go:195` supports `--open-plan`, `internal/cmds/registry.go:281` help only shows `--env` | Explicit env/open-plan aligns; fix global help. GUI opens NewPlan, CLI new is not use; do not change persistent current plan for symmetry |
| D5 update subscriptions | `internal/surface/gui/tasks.go:84`, `:118` per-item, skips page updates; `internal/cmds/subscribe.go:73` sends whole batch to UpdateSubscriptions | **Different failure paths/policy**. Shared batch runner with per-item results, CLI continue-on-error/page-skipped output; shell loops do not reproduce transaction/history |
| D6 unsubscribe/resubscribe | `internal/surface/gui/tasks.go:487` per-item precheck, batches 50, browser fallback if Steam unavailable; CLI `internal/cmds/workshop.go:16` → whole Workshop batch | Domain function shareable, tolerance/feedback differ; shared precheck/chunks and CLI item status; window tests browser fallback/task feedback |
| D7 build pack | `internal/surface/gui/api.go:348` global GUI busy; `internal/app/pack.go:415` same-name core collision | **Different concurrency rules**; decide different-name parallel builds, move gate into shared app. CLI sync/GUI subprocess need not be identical |
| D8 needs texture conversion | `internal/surface/gui/api.go:1053` → `internal/view/needs.go:1017` synchronous OptimizeTextures; texture page stoppable subprocess `internal/surface/gui/textures.go:171` | GUI scheduling difference; unify needs task entry; CLI optimize exists, stop absent |
| D9 switch after download | `internal/surface/gui/shell.go:426` switches environment; CLI `env download` only downloads/registers | Explicit env use in scenario; if atomic download/open desired add shared `env download --use` |
| D10 details | `internal/surface/gui/ops.go:115`/view notes; `internal/view/exchange.go:396` collection follow; `internal/view/planmanage.go:334` ten-minute checks; CLI note/collection/plan sync | Shared trim; explicit collection follow values; throttling/background analysis are scheduling, compare facts/freshness/errors, not calls |
| needs/ListProblems cache | `internal/surface/gui/needscache.go:39` key includes env ID/plan/version/list/log/links/directories; `:105` ListProblems from Needs; `:139` checks input version on publish | Same app findings as CLI `problems`, but no GUI cache/sequence/updating/`list-problems` event contract. **Compare results; cache GUI-only**. Add structured read-only findings, no CLI cache copy |
| New sort notices | `internal/view/preview.go:47`, `:78` missingDependencies/missingMods/replacements/cycles; `internal/cmds/loadout.go:250` sortCmd still changes/constraints/old cycle text | **Partial equivalence**: same App.Sort, cycles old output, others incomplete. Shared SortNotices DTO, all four in CLI text/JSON including dry-run |
| CompleteDependencies | `internal/surface/gui/ops.go:128` → `internal/view/verbs.go:298` → `internal/app/dependencies.go:164` | **No CLI**; add `loadout complete-dependencies`, one undo; report remote missing, separate subscribe, no silent download |
| ReplaceRecommended | `internal/surface/gui/ops.go:137` → `internal/app/sortnotices.go:206`, revalidates on execution | **No CLI**; add `loadout replace-recommended <ref…>`; per-item swap not equivalent to batch atomic validation/one undo |
| UseInstalled | `internal/surface/gui/ops.go:145`, needs DoAction → `internal/app/useinstalled.go:13` | **No CLI**; add `loadout use-installed <packageId…>`, preserve position, batch revalidate, one undo |
| Steam credit | `internal/surface/gui/subscribe.go:89` internal CMD batch marker; `:146` SubscribeWorkshopBatch; `internal/app/credit.go:19` records, `:97` answers, `:127` advances; `internal/surface/gui/credit.go:35` task | CLI `subscribe workshop` only SubscribeWorkshop (`internal/cmds/subscribe.go:67`), no public credit-batch or prompt/answer/step. Internal marker not CLI flag. Add explicit policy, credit status/answer/run with SQLite checkpoint; never choose answers for users |
| Subscription task rework | `internal/surface/gui/subscribe.go:128` SubscribeWorkshopBatch+WorkshopCalls; `internal/surface/gui/tasks.go:165` cancel/continue/retry, permanent item failures | Some core retry shared; GUI task IDs/item display/same-row continuation/permanent-failure buttons **not CLI equivalent**. Structure batch results first, later resumable CLI jobs; Phase 1 tests cancellation/continuation in window |

### 2.2 Fixes ranked by player automation coverage

Prioritize common actions/differential validation unlocked, not API counts. Fast input, page state, and cancellation feedback in 3a cannot be unlocked by CLI additions.

| Order | Proposed change (unimplemented names) | Common scenarios unlocked | Estimate |
| --- | --- | --- | --- |
| P0-1 | Thin CompleteDependencies/ReplaceRecommended/UseInstalled CLI, shared SortNotices, structured sort | Sort→dependencies→missing/recommended→immediate undo; concentrated new GUI features | 1–2 person-days |
| P0-2 | Shared D2/D3 EnableSuggested; app MoveBlock/staging placement; multi-ref move, stage all | Multiselect Space, placement, drag, stage all/history, especially equal lists with unequal undo counts | 2–3 person-days |
| P0-3 | Shared ReadImport/ApplyImport; CLI `--text`/`--merge`/collection `--new`, explicit follow; shared export overwrite | File/share-code/paste/collection replace/merge/new list, export/import | 1–2 person-days |
| P0-4 | Merge/accept D1; shared D5/D6 per-item results/failure policy; bulk trash/unhide/pin/unpin | Select batch→delete/unsubscribe/update→cancel, cross-game links; depends on in-flight D1 | 2–4 person-days including validation; not automatically concurrent with P0-1 |
| P1-1 | `pin --update-to-workshop` → UpdateToWorkshop; `workshop update` → UpdateWorkshopClient; `subscribe check --source` → CheckSubscriptionSource | Normal upgrades/source checks rather than delete/redownload resubscribe | 1–2 person-days |
| P1-2 | Explicit credit policy/commands/state using CreditStep; later shared query/cancel/continue | CMD→Workshop replacement, duplicates/failure recovery; window cancel testable now | 2–4 person-days |
| P1-3 | `textures stop` → StopTextures; needs task entry; `monitor add` → AddMonitor | Stop conversion, retain completed items, monitor position | 0.5–1 person-day |
| P2 | D4 help, D7 gate, D9 `--use`, D10 trim/follow | Reduce script mistakes, uncommon combinations; no block to window acceptance | 1–2 person-days |

Initial read-only canonical JSON export can live in test code reading app/store directly. No production snapshot framework for research; no parsing truncated/localized CLI text as DB truth.

## 3. Complete player-action mapping

"Same path" means main app capability shared, not GUI scheduling/prechecks/confirmation/DTO/presentation tested. "Different path" includes composition/parameters/failure policies. "GUI-only" has no dedicated CLI equivalent (RunCLI does not make every command a GUI button). Pure UI actions belong here and use real-window tests, not artificial CLI commands. Unchanged rows: preserve sharing and include scenarios. APIs cover actions plus related reads/previews/polls.

### 3.1 Lists, diagnosis, import/export

| Player action / API | CLI equivalent | Status | Location / completion |
| --- | --- | --- | --- |
| Initialize/open editor: Init, Board, Workspace, Shell, Stamp | status, list, plan list | Different | gui/api.go:183, :208; gui/environment.go:179; async composition, compare final content, reject late reads |
| Search/filter/facets: Filter, BarFilter | list, find, library filters | Different | gui/api.go:604, gui/ops.go:15; shared facts, test three columns/positions separately |
| Space enable/disable/put back: Toggle | enable/disable/stage restore | Different | view/verbs.go:105; D2/D3 |
| Stage/remove: Stage, Unstage, RestoreAll | stage add/drop/restore | Same | gui/api.go:801; view/verbs.go:138; compare placement/undo |
| StageAll | No direct command | GUI-only | view/verbs.go:277; add stage all, not N history steps |
| Block placement/move: MoveBy, Place, MoveShown, PosOptions | single-ref move; enable/stage restore position | Different | view/verbs.go:161, :178, gui/ops.go:41; app composition/bulk entry; shown position is UI input |
| Swap copy: Swap; LibraryDo use/swap | swap A [B] | Same | gui/api.go:881; view/library.go:914; explicit B in CLI, GUI ambiguity window test |
| Sort/save/launch: Run, RunMoving, Preview | sort/save/launch, --dry-run, --move-duplicates | Different | view/preview.go:72, :144; shared domain, notice/refusal/reentry differ |
| Undo/redo/history jump: Undo, Redo, History, HistoryGo | undo/redo/history/loadout checkout | Same | gui/api.go:558, :569, :853; app/history.go:29; compare stacks, not only list |
| Dependency closure: CompleteDependencies | None | GUI-only | app/dependencies.go:164; P0-1 |
| Recommended replacement: ReplaceRecommended | None (swap not equivalent) | GUI-only | app/sortnotices.go:206; P0-1 |
| Installed copy for missing: UseInstalled | None (enable not equivalent) | GUI-only | app/useinstalled.go:13; P0-1 |
| Tag candidates/edit: TagCandidates, SetTag | tag list/add/remove, repeatable --mod | Same | gui/api.go:930, :947; lineage tags, fast-confirm separate |
| Mod/copy notes: ModNote, SetModNote, LibrarySetNote | note, note --copy | Different | gui/ops.go:104, :115; gui/library.go:292; shared trim |
| New/copy/from version/save: PlanFroms, PlanSources, CreatePlan, NewPlan, CheckPlanName | plan new --from | Different | gui/planforms.go:22, :93; gui/shell.go:156; separate post-create open state, shared app validation |
| Current vs open: UsePlan, OpenPlan | plan use/--open-plan | Same | gui/shell.go:195, :204; D4, snapshot process-local open state |
| All lists/delete warning/rename/delete: PlanManage, RemovePlanWarn, RenamePlan, RemovePlan | plan list/rename/remove --yes | Same | gui/planmanage.go:122; gui/shell.go:212; window warning/cancel/last-plan protection |
| Game versions: PlanVersionsSet | plan versions | Same | gui/planmanage.go:137 → view/planmanage.go:258 |
| Follow/mode/unfollow/check/sync: PlanFollow, PlanFollowMode, PlanUnfollow, PlanCheck, PlanSync | plan follow --mode/unfollow/sync --check | Different | gui/planmanage.go:130; view/planmanage.go:334 throttle; explicit follow/mode |
| Memory/video estimates: PlanLoadCost, PlanLoadCostMeasure | plan cost | Same | gui/planmanage.go:30, :77; async/late results separately |
| Import info/preview: ExchangeInfo, ImportPreview, ImportPreviewFor, ImportFollow | loadout import; collection --import --follow | Different | gui/exchange.go:40, :132; view/exchange.go:49; CLI lacks preview DTO/text/merge |
| Replace/merge/target plan: ImportApply, ImportApplyTo | loadout import replace only | Different | gui/exchange.go:153, gui/planforms.go:178; add --merge, app.ApplyImport (view/exchange.go:367) |
| Import new/follow collection: ImportAsPlan, ImportAsPlanPreview | loadout import --new; collection current workspace only | Different | gui/shell.go:167, gui/planforms.go:160; collection --new, explicit opening |
| Export text/save/copy: PlanExport, PlanExportSave, PlanExportCopy, ExportSave, ExportCopy | loadout export --format --out | Different | gui/planmanage.go:173; view/exchange.go:468; 8 formats, CLI refuses existing file, GUI confirms overwrite; explicit --overwrite |
| Needs/editor problems: Needs, ListProblems | problems/show/ignore/unignore | Different | gui/needscache.go:105; GUI cache/events, comparable core snapshots |
| Needs one-click fixes: DoAction | move, sort, enable, disable, rule add ignore, loadout checkout/ignore-external, identity resolve, unhide/unpin/trash, problems ignore, pack unlink, links repair, swap, library move-to-locked, textures optimize/clean, plan versions | Mostly same, two exceptions | view/needs.go:965 cases; no use-installed CLI, synchronous textures D8; frontend navigation/collapse/web |
| Row/menu/preview: RowOps, RunOp, OpPreview | Inventory/list commands here | Different | gui/ops.go:60; view/ops.go:74 single table for buttons/menu/keys; preview permission still revalidated on execution |
| Palette/CLI/help: Palette, RunCLI, Keys | help/all cmds | Same + UI | gui/api.go:1403, :1444; RunCLI inherits open plan; completion/focus/key routing window-only |

Here and below `gui/…` means `internal/surface/gui/…`; `app/…`, `view/…`, `cmds/…` under `internal/`.

### 3.2 Inventory, files, subscriptions, accounts

| Player action / API | CLI equivalent | Status | Location / completion |
| --- | --- | --- | --- |
| Browse/filter/copies/fields: Library, ModCopies, LibraryCopy, LibraryFields | library --lineage/--facets/--include/--exclude/--sort/--all, mod | Same | gui/library.go:107, :242; view/library.go:417; expansion/virtual scroll window |
| Inventory enable/add/disable/take-out/stage/use/swap via LibraryDo | enable/disable/stage add/swap | Different | view/library.go:914 shares Toggle; D2/D3 |
| Remove from all lists: LibraryDo remove | library remove-from-plans | Same | view/library.go:869; one undo per affected list |
| Separate copy: LibraryDo separate | lineage split | Same | view/library.go:831; CLI merge too, test GUI applicability |
| Lock/unlock/move: LibraryPreview, LibraryDo pin/unpin/move | pin/unpin/library move-to-locked | Different bulk wrapper | view/library.go:884; same single-item, CLI single-ref, GUI continues failures; bulk results needed |
| Update to Workshop: LibraryDo pin-update | pin --update creates new directory, not equivalent | GUI-only | view/library.go:888 → UpdateToWorkshop; P1-1 |
| Delete/unhide: LibraryDo delete/unhide | trash/unhide | Different | gui/tasks.go:411; accept D1 cross-env links after merge |
| Unsubscribe/resubscribe: LibraryDo unsubscribe/resubscribe | workshop unsubscribe/resubscribe | Different | gui/tasks.go:487; D6 |
| Convert/clean DDS: LibraryDo textures/textures-clean | textures optimize/clean <ref…> | Same, scheduling differs | view/library.go:854; only generated DDS deleted, PNG unchanged |
| Change history/file diff: Changes, ChangeFiles | changes [--library] | Different | gui/api.go:1097, :1121; separate GUI DTO, shared snapshot diff as read-only output |
| Subscriptions/add page: SubscriptionsPage, AddSubscriptionInfo | subscribe list, changes, workshop state | Different | gui/subscriptions.go:13, :50; GUI source/task/inventory aggregate; same source facts |
| Add git/page/Workshop: SubscribePreview, Subscribe, SubscriptionPage | subscribe git/page/workshop --dry-run | Different | gui/subscribe.go:27, :79, :128; git/page often cmds; Workshop tasks/progress/credit |
| Replace from file: SubscriptionFromPreview, SubscriptionFrom | subscribe update <ref> --from <zip-or-directory> | Same | gui/subscribe.go:109, :120, task runs cmds |
| Check/update/remove existing: LibraryDo subscription-check/update/remove | subscribe check/update/remove | Different | gui/tasks.go:84, :118; D5 page skip/item failure |
| Whole source check: CheckSubscriptionSource | subscribe check only recorded subscriptions, not all sources | GUI-only | gui/subscriptions.go:62, gui/settingtasks.go:69; source entry needed |
| Steam updates existing: WorkshopUpdatePreview, WorkshopUpdate | resubscribe deletes/redownloads, not equivalent | GUI-only | gui/subscriptions.go:73, :99 → UpdateWorkshopClient |
| CMD credit prompt/accept/defer: CreditPrompt, CreditAnswer | None | GUI-only | gui/credit.go:18, :35; app/credit.go:19, :127; P1-2 |
| Tasks view/seen/stop/cancel/continue/retry/requeue/clear/remove/record undo: Tasks, TasksSeen, TaskAct | No unified tasks | GUI-only | gui/tasks.go:155, :165; GUI records/worker not shell processes; window first, shared job control later |
| Steam/LoversLab login/verify/logout/forget: AccountAction, AccountStatus, SteamLogin | None | GUI-only, retain player authentication | gui/accounts.go:18, :261, :266; later, not Phase 1 |
| LoversLab select/follow/install/preview: LoverFiles, LoverFollow, LoverInstall, LoverInstallPreview, SubscriptionFilePage | page --from partial approximation | GUI-only | gui/accounts.go:127, :155, :192, :218, :242; auth later, ordinary zip covered Phase 1 |
| Open folder/Workshop/client/source/notes: OpenFolder, OpenWorkshop, OpenSteamWorkshop, OpenSubscriptionSource, OpenModsFolder, OpenChangeNotes, OpenURL | No matching UI command | GUI-only UI | gui/library.go:158, :181, :199; gui/subscriptions.go:111, :146; gui/api.go:1128, :1466; target check, external windows later |

### 3.3 Other pages and shell

| Player action / API | CLI equivalent | Status | Location / completion |
| --- | --- | --- | --- |
| Home selection/launch: Home, HomeApply, HomeBackground | env use + plan use/pack launch/launch | Different | gui/shell.go:86, gui/environment.go:44; selection vs persistent current; no transient false no-game |
| Packs/details/name/estimate: Packs, Pack, PackNameCheck, BuildPage, BuildPageResult | pack list/check/build --dry-run | Different | gui/api.go:249, :267, :304; gui/buildpage.go:26; GUI estimate DTO, shared facts/numbers |
| Preview/build/stop: PackDryRun, PackBuild, PackStop | pack build [--dry-run], pack stop | Different | gui/api.go:284, :348, :433; D7; active stop cleans copied content, crash remnants removed/rebuilt via unfinished (app/packbuild.go:251, app/pack.go:426), no resume promise |
| Remove finished/unfinished/preview: PackRemove, PackRemoveUnfinished, PackRemovePreview | pack remove [--unfinished] | Same | gui/api.go:444, :467; gui/linklifecycle.go:15 |
| Import/export/launch pack: PackImport, PackExport, PackLaunch | pack import/export/launch | Same | gui/shell.go:348; gui/api.go:317, :460; CLI list-as-plan separate |
| Texture info/stats/poll: TexturesInfo, Textures, TextureMeasure | textures status | Different | gui/textures.go:48, :69, :88; same scope/content/scale; async ack not completion |
| Install/convert/stop/clean: TexturesInstall, TexturesRun, TexturesStop, TexturesClean | textures tool --install/optimize/no stop/clean | Different; stop GUI-only | gui/textures.go:163, :171, :278, :291; add CLI stop, unify needs |
| Environments/switch/remove preview/remove: Environment, UseEnv, EnvRemovePreview, RemoveEnv | env list/use/remove --yes | Same, async wrapper | gui/api.go:502, :1147, :1159; gui/environment.go:39; late results/write gate |
| Guide find/validate/add/scan: GuideFind, GuideGame, GuideAdd, GuideScan | env detect/add/use, scan, analyze | Different composition | gui/shell.go:237, :247, :255; gui/environment.go:49; no CLI for live UI validation |
| Branches/install CMD/download game: GuideDownloadInfo, GuideInstallSteamCMD, GuideDownload | env branches/deps install/env download | Different | gui/shell.go:380, :416, :426; D9, directory/account state separate |
| Settings/tool offer/directories/move preview: Settings, ToolInstallOffer, Directories, DirectoryMovePreview | config get, deps, folder check/move --dry-run | Same | gui/api.go:478, :489, :518; gui/linklifecycle.go:5; errors/cancel no writes |
| Settings/env paths/directories/tools/DB/cache/data/gc: ApplySetting | config set, env set/rebind, folder set/move/add/remove, deps install/remove, db backup/vacuum, cache clear, data update, gc | Same | gui/api.go:528, gui/settingtasks.go:140; extra async task tests |
| Layout/preferences: Setting, SetSetting | Some config get/set; no per-key ui.gui.* command | GUI-only UI | gui/api.go:1498, :1508; column visibility/width/order, nav, language/theme persistence |
| Background browse/set/add/remove: Backgrounds, SetBackground, AddBackgroundFile, RemoveBackgroundFile | None | GUI-only UI | gui/background.go:160, :197, :220, :251; crawler controls, filesystem snapshot |
| Logs/select/cluster/pattern: Logs, LogPage, SelectLog, LogCluster, LogPattern | log --file/--select/--cluster, pattern | Same | gui/api.go:1169, :1187, :1240, :1265, :1281; compare file/clusters, not formatting |
| Live/companion monitor: Live, LogMonitor | monitor, log | Different aggregate | gui/api.go:587, :1217; correct run/session, no stale results |
| AddMonitor | monitor install only EnsureMonitor, not equivalent | GUI-only | view/monitorlog.go:225; cmds/monitor.go:72; monitor add after Harmony |
| Scan/analyze: Scan, Analyze | scan/analyze | Same | gui/api.go:1293, :1334; subprocess GUI, window reentry/tasks/events |
| Details/technical cards/evidence: Detail, Sections, Inspect | mod, describe, def, patches, harmony, problems | Same aggregate | gui/api.go:676, :737, :1077; crawler tabs/network/images/copy |
| Native pickers/clipboard: PickFolder, PickProgram, PickImportFile, PickExportFile, PickSubscriptionZip, Clipboard | Explicit paths/files, no OS dialogs | GUI-only UI | gui/exchange.go:68, :88, :96, :104, :122; gui/subscribe.go:102; CDP before/after, UIA later |
| Reference numbers: SaveRefNums, ctrl+backslash | None | GUI-only test aid | gui/api.go:1477; assets/js/refnums.js:1; discovery/location, not full inventory |
| Uninstall preview/run: UninstallPlan, UninstallRun | uninstall --dry-run/--yes/options | Same | gui/uninstall.go:16, :33; final separate clone, not mid-crawler navigation |
| Ready/leave/close/quit/frame theme: Ready, Leave, Close, Quit, SetWindowTheme | Process exit, no matching UI | GUI-only lifecycle | gui/api.go:132, gui/memory.go:55, gui/readers.go:161, gui/uninstall.go:70, gui/window_theme.go:7; exit waits/cache release/task stopped |
| Click/double/right/drag/multiselect/suggestions/table menus/expand/scroll/Tab/Esc/back/forward/shortcuts | None | GUI-only input/presentation | assets/js/main.js:983, lists.js:300, navigation.js:186, popups.js:170; real window/crawler, no CLI |

### 3.4 CLI-only and reference requirements

| Capability | Status/evidence | Automation |
| --- | --- | --- |
| swap A B; pin --name/--git/--reason; move-to-locked --reason; textures --force/--no-mipmaps | Extra CLI choices; cmds/loadout.go:25, copylocal.go:13, library.go:28, textures.go:40 | No GUI input is not mismatch; core parameter tests, UI only if product needs |
| pack import --as-plan, plan sync --dry-run, loadout ignore-external, workshop subscribe/state | CLI entries/modes; cmds/pack.go, planfollow.go:18, loadout.go:39, workshop.go:18 | Some indirect needs GUI; declare scenario applicability, no invented buttons |
| alias, full rule editing, lineage merge, import rimcrow, source/search, files search, why, loadout log/diff, data/cache/db status, version | No complete dedicated GUI; cmds/playerdata.go:14, loadout.go:40, identity.go, rimcrow.go, source.go, content.go:23, settings.go, version.go | RunCLI available; CLI core coverage, GUI read subsets as above |
| archo test run [plan] --json [--map] [--timeout] [--settle] | CLI-only; cmds/test.go:15; app/gametest.go:80 | Reuse game-loading smoke, separate from GUI launch acceptance |
| mcp/gui, copy-local aliases, dry-run modes | CLI transport/launch/modes; cmds/registry.go:109, copylocal.go:18 | Not missing buttons; retain CLI regressions |
| RimCrow AI diagnosis/translation/language packs/Workshop publishing/custom themes/data migration | inventory/ui-pages.md:259, :266, :322, :344, :351; api-surface.md:285; no current matching RimArcho API/routes | Reference requirements, not implemented GUI, not GUI-only or Phase 1 denominator |
| Retrospective/save management, worth playing, Archo | Future specs/06-ui/2.11-other-pages.md:5, :11, :15 | Crawler when routes open; no fabricated coverage |

## 4. Three driver layers and real-window entry points

| Layer | Detects | Cannot detect / cost |
| --- | --- | --- |
| CLI processes + core snapshots | Parameters, exit codes, validation, order, bulk failures, history, files/links/ModsConfig, swaps, cross-process undo | No loading overwrite/blank DOM/key values/frontend event filtering. Small synthetic fixtures seconds; full scan/network extra. Offline core scenarios each PR |
| guidev `/call` or direct `*API` + events; optional Chrome frontend | DTOs, request/stale, invalidation, tasks/events, needs/editor common source, swallowed errors; Chrome also DOM/text | API skips JS; Chrome is not Wails/WebView2, proves no native bridge/input/concurrency. Reuse HTTP harness, moderate upkeep, seconds–minutes |
| Real RimArcho.exe + WebView2 CDP, limited UIA | Fast-input races, late navigation results, focus/shortcuts, confirm/Esc, cancel feedback, loading/blank pages, actual event consumption, WebView2 rendering | Requires a logged-in desktop, WebView2, VM/display; native file/Steam dialogs are outside DOM. Run 10 high-value scenarios per PR; crawler, whole-library, online, and game cases nightly. Cost: minutes to hours |

Required observation points for four recent bugs:

| Bug | Minimum meaningful verification | Current protection |
| --- | --- | --- |
| Loading overwrites result text | Continuously sample `#status` across sort/edit/undo refresh; result persists until next action, not just API text | assets/js/screens.js:138 retains editor status; main.js:426 publishes then reloads |
| Rapid Space gives blank/stale reads overwrite edit | High-frequency real-window Input, record frames/final list; do not await business completion between keys | main.js:54 reloadSeq/editEpoch; :407 editChain/editsPending; serial scripts rarely trigger |
| Environment ID/name mismatch drops events | Run frontend with display name deliberately different from ID; cross env/plan, delayed events; accept new/reject old | needscache.go:80 publishes Env.ID; assets/js/problems.js:206 compares board.problems.environment |
| Shift+letter still lowercase | Real Wails `key='s', shift=true` and `key='S'`; S sort/J K move/T put back reach correct branch | main.js:1111 normalization; scripts/vm/cdp.mjs:36 defaults lowercase+Shift |

**Exact guidev serialization boundary:** do not use it for Wails concurrency acceptance. But baseline `spikes/guidev/main.go:30` shim fetches independently, `:145` handler reflects directly, no global serial queue; `:53` mutex belongs to event hub. Serial scripts, API locks, HTTP/1 connection limits, frontend editChain alter interleaving. Do not claim guidev necessarily serializes all calls, or HTTP concurrency proves Wails races fixed.

Use **existing CDP**, no new bridge: `internal/surface/gui/gui.go:79` BrowserArgs reads `RIMARCHO_DEVTOOLS_PORT`, passing `--remote-debugging-port` to Wails AdditionalBrowserArgs; WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS alone is ineffective here. Build/launch: `scripts/vm/vm.ps1:55`, `:124`; release-style GUI uses GUI=1/windowsgui. Equivalent guest context (RimArcho deployed there):

```powershell
$env:RIMARCHO_DEVTOOLS_PORT = '9222'
$env:RIMARCHO_HOME = 'C:\VM-Test\case\home'
Start-Process -FilePath 'C:\Users\Public\Documents\VM-Test\RimArcho\RimArcho.exe' -WorkingDirectory 'C:\Users\Public\Documents\VM-Test\RimArcho'
```

Reuse connect/evaluate/key/mouse in `scripts/vm/cdp.mjs:19`; `smoke.mjs:43` already checks wails.localhost, navigation, side buttons, script errors. Runtime.evaluate **only observes/discovers/sets fixtures**. Player actions use Input.dispatchKeyEvent/Input.dispatchMouseEvent, not api.Toggle or DOM `.click()` substitutes. Send rapid input at fixed 20/50/100ms intervals, await CDP acknowledgement only, not business Promises. Record sent/received key/time/repeat; serial `await edited()` must not erase races. Test autoRepeat/hold/keyUp separately; double/triple clicks use clickCount plus independent rapid same-position clicks.

Fill three driver gaps first: `cdp.mjs:36` lacks explicit Space/Backslash mapping; add `Space → key=" " / VK=32`, backslash/VK=220. Mouse helper handles side buttons only; use coordinate Input.dispatchMouseEvent for left/right. `cdp.call` (`:30`) needs timeout, protocol error checks, disconnect rejection of pending Promises, preventing false input success/hangs. Reuse smoke.mjs:15 page error/unhandledrejection capture, keep Runtime.enable off by default (comment notes altered rapid navigation); diagnostics separately. Screen off/not logged in → driver-not-ready, not app blank page. UIA for a few native dialogs/OS-input cross-checks. CDP does not replace Windows hotkey/IME acceptance; add hooks only later if observations truly missing.

## 5. One scenario, multiple drivers, comparable state

### 5.1 Scenario contract

JSON or YAML works; existing Node/PowerShell read JSON, so prefer JSON. YAML below illustrates proposed fields/semantics, **not implemented runner input**. Separate business actions from input bursts: business actions comparable, window-only input explicitly inapplicable to CLI, never silently skipped as passed.

```yaml
schema: 1
name: enable-sort-undo-save
fixture: small-two-envs
seed: 271027
context: {env: test-16, plan: colony-a}
drivers: [cli, gui-api, wails]
steps:
  - checkpoint: before
  - action: enable
    refs: [mod-a, mod-b]
    placement: suggested-block
  - action: sort
  - checkpoint: sorted
  - action: undo
    expect: {listEquals: after-enable}
  - action: redo
    expect: {stateEquals: sorted, scope: list}
  - action: save
  - await: {idle: true, problemsFresh: true}
  - assert: [no-dangling-links, config-matches-saved-list, counts-consistent]
budgets: {feedbackMs: 200, localReadyMs: 5000, analysisMs: 120000}
```

Enable automatically creates `after-enable`; every action records before/after. Until CLI supports suggested-block, return **unsupported: D2**, not default enable. Adapters declare exact/composite/unsupported per action; compare history even for composites, never hide extra undo steps. Subscribe/trash semantic actions explicitly declare transport/batch policy/refs/cancel boundary/partial outcomes.

Each step has stepId, requestId, timeout, acceptable refusals, prestate, completion conditions. GUI API connects `/events` first; UseEnv/Workspace await matching request's environment completion, bool true is ack only. Tasks await terminal and persisted item results, not merely a Tasks event. Real window waits for interactive DOM/correct context, independently verifies state. Random stress saves seed/input trace/checkpoints.

### 5.2 Separate runs from identical initial state

1. Freeze fixture: build, DB backup, game version/DLC, Mod manifests, community/replacement data, ACF, settings, tasks.json; all guest paths. Use SQLite backup API or complete stopped backup, never live DB copy without WAL.
2. Run CLI, GUI API, Wails sequentially from same VM baseline. Restore entire mutable data/differencing disk each run, verify fixture hashes. No CLI mutation followed by GUI continuation, no simultaneous DB writers.
3. Snapshot after every semantic action/stable burst; compare earliest divergence, inputs/errors/task items/files, not only final list. Continuously sample UI during bursts; static end state cannot rule out transient blank pages.
4. Offline fixtures fix network responses/content. Online Workshop separate; changed remote content/subscriptions → inconclusive, not relaxed invariants. Snapshots do not roll back Steam server state; record starting/ending sets and clean per scenario.

### 5.3 Snapshots and comparison

| Scope | Required data/comparison | Normalization and never-ignore rules |
| --- | --- | --- |
| Context | env ID→fixture name, game MM/DLC, persistent current plan, GUI open plan, latest request | Map random IDs to fixture keys; name is not ID; wrong-plan edits never ignored |
| DB lists | All plan names/version limits, ordered workspace.entries, origin kind/version, conflict, staging entries/original positions | `internal/store/plan.go:11`, :31, loadout.go:10; map instance by source-relative path+packageId+fingerprint, never merge copies by packageId |
| DB history | Per-plan ordered undo/redo entries+op, saved versions entries/source, collection mode/members, tags/notes, lock/source metadata | store/loadout.go:57, :89, :132; map row IDs, **never ignore stack size/order, relative version numbers, operation kinds**; extra GUI step is difference |
| Installation/tasks | live/tombstone, lineage mapping, subscription records, credit batch/item checkpoints, task/item state/reasons/retryability/completed set | Relational ID mapping; UI tasks surface-only extension, CLI inapplicable does not hide domain installation differences |
| ModsConfig.xml | Raw SHA-256/bytes plus activeMods order, knownExpansions, version | Semantics first, byte differences reported; normalize explicit root/time fields only; no activeMods sorting or dropping _steam suffix |
| Mods/ links | Relative names/reparse types/targets/existence/readability/ownership for all affected game/pack Mods | Guest roots → GAME_A/LIBRARY tokens; no recursive traversal into external targets; not current env only |
| Disk | Full mutable manifest: relative path/type/length/SHA-256, About.xml(.disabled), markers, ZIP/extraction, DDS, locks, recycle results | Build immutable thousands-Mod manifest once; hash touched set/check no unplanned changes per step. Recycle random names not literal equality; verify source removed/content recoverable |
| GUI findings/visuals | Needs/ListProblems/Board env/plan/listVersion/revision/sequence/freshness/problemId ownership, page/selection/focus/aria-busy/status/console | Sequence monotonic/late-drop within run, not numeric equality across drivers; same-state counts/content equal. Blank/error/zero list never dismissed as loading |

Ignore only actual wall-clock creation/check/duration fields, random UUID/autoincrement values themselves, guest absolute roots, normal cache/WebView profile/log rotation differences. Check relationships/causal order/expiry/retry semantics; fixed clocks can compare time branches too. Never ignore all meta/history/credit/file attributes to erase diffs. DB bytes/WAL layout are not equivalence criteria.

## 6. Pass criteria and budgets

### 6.1 Shared criteria

- **Lists never disappear**: fixtures with environments never falsely show "no game yet"; successful reads cannot turn nonempty lists into blank DOM. Initial placeholders allowed with recognizable feedback/navigation and eventual requested page/plan. Cancel/Esc closes only correct layer, no accidental env switch/list clearing.
- **Latest action owns state**: success/refusal/partial results survive background loading; old navigation replies cannot overwrite new page; accept correct env ID+plan+listVersion, nondecreasing sequence. Task results remain inspectable after leaving.
- **Lists/history**: ordered entries/staging positions/metadata correct; one edit+undo restores original bytes of that action's payload (e.g. entries encoding), not whole SQLite. Undo moves stacks; tags/disk deletion must not falsely claim undo. **Known shared-core boundary**: app.Stage/Unstage separately write staging meta (stage.go:57, :126), UndoRedo restores only workspace entries (loadout.go:839), view.Place does Enable+Unstage (verbs.go:178). Whole list+staging byte restoration is a target to verify/converge, not an existing guarantee. Both drivers can be equally wrong, requiring independent oracle.
- **Deployment/disk**: no dangling/cyclic/wrong-target links; saved ModsConfig equals saved list; unsaved edits never write game config. Explain external conflicts. No unplanned loss, DDS cleanup preserves PNG, real-directory collisions never silently overwritten.
- **Copies/credit**: target environment has expected winning loadable package copy. After Workshop switch, list/staging/recoverable history/deployment no longer points wrongly to recycled CMD. Repeats create no extra copies. Protected originals may remain with retained reason; never require physical count=1 universally. app/credit.go:127, app/credit_test.go:676.
- **Cancellation boundaries**: completed downloads/deletions retained, unstarted items not executed. Current item may finish at safe boundary; cancellation cannot revoke Steam requests. Continue only remaining/retryable, permanent failures explicit, no reinstallation of completed items.
- **Finding consistency**: after updating=false/stale=false, needs/editor global counts, per-Mod problemIds, cards agree. While waiting, old results allowed with updating. Four sort notices match source; match API errors/refusals/CLI exits explicitly.

### 6.2 Feedback, readiness, actual completion separately

Historical single/few observations are not current P95: bulk audit starts `887b668`, page study `81017d8`, needs post-fix separate. Their methods inform budgets; **new thresholds below are proposed and unverified**.

| Scenario / historical source | Old observation | Phase 1 threshold / timeout |
| --- | --- | --- |
| Single input / skeleton / loading / cancel feedback | Bulk audit §5: initial feedback mostly 13–115ms, also 314ms; §5 stage-all painting about 540ms | Normal interactions follow the specs §10 target of 50ms; stress-test feedback/navigation has a soft 200ms threshold; continuous main-thread stalls >500ms fail performance. Unready data is not completion |
| Local editor/home/library/needs | Page §1: home first 0.75s; library new 1.46s, warm 0.19–0.20s+JSON 22–26ms; subscriptions warm 0.56–0.74s, new 1.45s | Small ready ≤2s; thousands library ≤5s, warm nav ≤2s; hard 15s/step. Cold 1.5s spec separately; VM not host benchmark |
| needs cache/background analysis | Large 1.6 before/after: warm 10.8–12.2ms, ListProblems 22.8–46.3ms, post-edit publish 11.658s; old first analysis 35.2s | Same-version warm API ≤100ms target; window counts ≤200ms; fresh ≤120s retaining content/updating; separate baseline on slow hardware |
| Unsubscribe confirmation precheck 10/100/1293 | Bulk §4 1.24/11.148/98.585s; Setting locked 98.49s | Precheck feedback ≤200ms; local confirm 10≤2s,100≤5s,all≤15s optimization targets; unrelated Setting/nav ≤200ms; timeout/lock fails even if JS moves |
| Delete/source update/all precheck | Delete 100 6.214s,3760 234.341s; update 135 39.590s; all pin 111.909s | Same progress vs confirm distinction; 100/all 5s/15s targets retest after D1/nonblocking; log defects rather than raise limits |
| Post-confirmation editing and refresh | Bulk audit §5: disabling 570 entries took 4.358s, Toggle only 22.4ms; sorting 1.471s | Small fixture ≤2s, thousand-entry list ≤5s; input/navigation remains usable, last action retained; stable within 10s after continuous input ends |
| Download/file batch/cancel | No destructive actions in audit, **no measured completion baseline** | Queue≤200ms,progress≤1s; cancel intent≤200ms,safe stop≤5s target or explain external wait. Local hard10min,online20min,item stall60s diagnostic; size/network baselines separate |
| Texture stats/conversion | First full78.9s,retest2.5s | Feedback≤200ms,stats≤120s; fixture-size conversion budget, initial hard10min; stop before all files finish |
| Game loading | Not launched in this study; cmds/test.go:15 defaults test run to at most 10min | Small valid list reaches menu within 120s target; real large lists initially have a hard 10min timeout with load-time/memory records. Menu/map + assertions on current-run logs required |

For every budget, record host/guest CPU, RAM, disk, GPU, build hash, content scale, and cold/warm category. Time from actual input receipt; record first-feedback, confirm-ready, API-ack, data-ready, task-terminal, and stable-snapshot separately. Run at least 5 rounds and retain all samples; report meaningful quantiles only after 20 or more. Separate performance failures from functional failures; old single runs are not P95.

Original detailed records: [bulk timing audit](2026-10-27_batch-operation-timing-audit.md), [page-load research](2026-10-26_page-load-time-and-optimization.md), [needs cache results](2026-10-27_needs-cache-and-list-problems-api-results.md).

## 7. Ten Concrete Phase 1 Scenarios

Drivers C=CLI,A=guidev/direct API,W=real Wails. A proves service results only; DOM/input assertions require W. Budgets reference §6; unsupported C actions explicit, W/A first.

| Order/scenario | Steps | Drivers | Expected invariants | Budget |
| --- | --- | --- | --- | --- |
| 1 Rapid Space, repeats, and Shift | Select 1/10/100 valid Mods in the inactive column; press Space 20 times at 20/50/100ms intervals; repeat in active/staged columns. Hold Space, Shift+J/K, Shift+S/T; test lowercase key + Shift and uppercase variants; settle→undo/redo | W required; A/C can run corresponding sequential actions, but C suggested-block awaits P0-2 | No blank view; conservation across three columns; no duplicates/lost actions. Calculate final state from actually received legal input, without assuming every key reaches the browser. History can step back; state retained, stable counts agree | Per-key feedback ≤200ms; no >500ms UI freeze; stable ≤10s after burst, analysis ≤120s |
| 2 Navigate/switch before ready | Open nonempty A/B20 times at50–200ms,editor→needs→library→home, next navigation while read pending;env ID≠name,two versions;return A | W;A request/event stress,C core context only | Last page/list wins,no old clearing/no false no-game;A/B/current intact,correct events;write controls gated during switch,restored after | Nav≤200ms,last local≤5s/hard15s,analysis≤120s |
| 3 Repeat sort/double/triple click | S/sort10 times;double/triple enable/import/subscribe;repeat while pending;immediate undo after sort,redo,navigate back | W;A/C deterministic order/history;notice repairs P0-1 | Idempotent sort,no duplicate plan/download/entry;controls may disable/coalesce/refuse/legally repeat by contract,not universal policy;status survives loading,undo correct predecessor | Ack/refusal≤200ms,local≤2s/large5s,burst stable10s |
| 4 Mistakes, confirmation cancellation, Esc, and immediate undo | Open delete/unsubscribe/export-overwrite/move-directory confirmations; test cancel, Esc, and close separately. Open menu/position/tag/import dialogs and Esc one layer at a time. Immediately undo enable/move/stage/sort; repeat mistakes and cancellation | W; A verifies preview does not write, C verifies dry-run; cancellation input requires W | Domain snapshots equal before/after cancel; no lost list, task started, or accidental main-window exit; reasonable focus return. Undoable actions restore payload; previews write no config/files | Local dialog open ≤200ms, close ≤200ms; precheck for 10/100/all: 2/5/15s targets; undo stable ≤5s |
| 5 Add→cancel download→continue→enable | Single/bulk Workshop incl permanent/retryable failures,cancel midtask,navigate,continue same task,repeat continue;CMD/Steam rounds,scan→enable→sort | W/A;C subscription core,GUI continuation/credit unsupported | Completed retained,remaining canceled,no reinstall,permanent reasons/no invalid retry,total/task rows agree,no duplicate copies/wrong-list edits | Ack≤200ms,progress≤1s,cancel≤200ms/stop5s target,online20min,stall60s diagnostic |
| 6 Remove:disable/unsubscribe/delete/cancel batch | Shared source group;disable→undo→redo,unsubscribe subset,recycle hidden/residue,cancel batch incl badref;check A/B links,scan,reopen | W/A/C;baseline D1 should differ,retest after merge | Disable undoable,unsubscribe/recycle not falsely undoable;completed retained,unstarted untouched,no cross-env dangling,missing honest,partial failure keeps later results | Feedback≤200ms,precheck2/5/15s,local10min,cancel as above |
| 7 Four imports/eight export roundtrips | File/RC code/pasted text/Workshop collection preview→cancel→reopen→replace;merge→undo,new plan;8 exports,cancel/confirm overwrite;reimport representable data | W/A;C file/code,text-as-file composite,merge/collection-new P0-3 | Canceled previews no writes,single-step replace/merge undo,naming/double-click preserves originals,order/missing intact;lossy text compares projection,not instance hash;no accidental ModsConfig | Local preview/apply≤2s/all5s,feedback≤200ms,collection hard30s,failed original intact |
| 8 Notices→complete/use-installed/recommended | Fixture installed/remote missing dependencies,missing placeholder with available copy,recommended alternative,cycle;sort,handle each,mutate during repair for revalidation,immediate undo,needs/editor counts | W/A;C partial notices/three repairs pending P0-1 | Four notices,no unloadable copies/lost position,atomic revalidation,no partial mutation,one undo each,fresh counts agree,correct env events | Local≤2s/large5s,counts200ms,analysis120s |
| 9 Save→real launch/load | Valid save,check ModsConfig/all links,GUI launch normal path;after stop separately archo test run --json (1.6 LogSync/AutoTest later);missing dep/external config/running-game refusal | W/A/C;guidev disables Launcher,real launch W/C | Correct projection/backup/no dangling,current menu/map,actual loaded list,diagnosable failure not mere live process;isolated test config not GUI save/launch substitute | Save≤5s,launch feedback200ms,small menu120s/large hard10min,settle after exit |
| 10 Discover every control/try each | §8 crawler all pages/cards/menus/tabs/representative rows;known fixture per edge,cancel before confirm,double-click/Esc/leave;unreachable reasons | W primary,A state,C mapped differential | No script errors/blank/false no-game/lost list;state oracle per write edge,clicked is not pass;counts/history/files/copies follow action contract | Local edge ready5s/hard15s,feedback200ms,long-task budgets;initial crawler hard60min,report uncovered |

Each scenario starts small/controlled, then realistic scale (about5k lineages/6k instances, actual runtime counts). Define repeat policies for3/5/10 by control; if repeated new allows two plans, require clear names/no overwrite, not an invented universal defect. Stress completion is after last input and related requests settle.

## 8. "Try every button": automatic control crawler

Use **runtime DOM discovery + refnums location + command/operation-table checks**, not another handwritten button list.

Existing CONTROL selector `assets/js/refnums.js:26` includes button,a,input/select/textarea,summary,roles,list rows; IDENT (`:30`) includes data-cmd,data-env-write. `window.refnum(id)` returns current DOM. Numbers also cover text/images, historical "not shown now" elements, capped3000/page (`:23`); **refnums line count is not button denominator**, IDs not eternally stable across fixtures. SaveRefNums writes `a.Home/refnums` (gui/api.go:1477), default `%APPDATA%\RimArcho\refnums`; with isolated RIMARCHO_HOME read guest test home, never host player profile.

Implementation:

1. CDP real window, enable ctrl+backslash. Discover through CONTROL, clickable rows, semantic data,onclick/tabindex,Accessibility tree,cursor:pointer; hit-test/actual events remove decoration. Filter disabled/inert/unhittable/occluded while recording reasons/preconditions, not silently ignoring. refnums pick treats whole rows as controls and may skip child buttons: recursively discover/deduplicate actionable descendants. Delegated events make addEventListener enumeration insufficient.
2. Control key: page + dialog/menu state + tag/role + data-cmd/data-env-write/data-id/name + normalized label. Include ancestor path, refnum, bounds, current plan/env, visible/enabled, and triggered API/events. Group repeated rows by action type only when keys match but object instances differ; retain representative states for 1/multiple selections, missing entries, duplicate copies, and hidden entries. Content text is not a stable selector.
3. Click nav,tabs,expanders,header/right-click/shift+F10 menus,more actions,detail tabs; rescan after virtual scroll. MutationObserver discovers dynamic controls; maintain pending edges in page-state graph. Include narrow overlays,empty/problem lists,running tasks. Add keyboard-only Palette/Keys/view.OpTable (`view/ops.go:74`), not DOM-only coverage.
4. `data-env-write` is a **write-gate hint** (`environment.js:21`), not full registry: navigation/cancel/input/click may lack it, same value may mean different objects. Palette cannot enumerate native dialogs/drag/dynamic task buttons either. Compare three sources; new unclassified entries → coverage-new.
5. Reset state per edge; coordinate Input at hittable center, repeated same-position clicks separate stress variant. Cancel/Esc first and compare unchanged domain; restore fixture, confirm/submit with shared invariants/action oracle. Fill minimal valid forms, then invalid/duplicate/empty; context preconditions through discovered semantics/small **action-fixture recipes**, not per-page lists.
6. Do not blindly click through polluted state. Move directories/delete packs/uninstall/exit change root; separate clone branches, export evidence then restore. External edges record target/blocked/external, UIA later. Unreachable never passes.
7. Automatic ledger: discovered,eligible,attempted,asserted,passed,failed,blocked,disabled-by-precondition. Primary **asserted / eligible action edges**, not clicked/refnums. New clickable/command/OpTable without coverage fails; API exports change alert.100% means reachable edges in defined fixture space, not all combinations.

Write semantics still need action-specific assertions. Generic crawler discovers/executes/catches general damage but cannot infer correct sorting/downloads from labels. Small semantic operation table connects §5 snapshots: automatic discovery, explicit expectations. Performance rounds disable refnums/extra MutationObserver to avoid altering races; normal coverage retains numbers for diagnosis.

## 9. VM, real data, game loading

### 9.1 Platform choice

| Option | Reset/provisioning | GPU/game | Cost/tradeoff |
| --- | --- | --- | --- |
| Windows Sandbox | Discard on close;.wsb mappings/startup,no persistent layered snapshots | vGPU available,no full RimWorld/Mod compatibility guarantee | Light first-run/install smoke; repeated thousands-Mod/Steam copies costly,not primary |
| Hyper-V | Checkpoints/differencing VHDX/PowerShell | No assumed sufficient guest3D;GPU-P/DDA depend hardware/host/guest,validate separately | Windows Pro/RAM/disk;good offline CLI/API,little benefit rebuilding GUI/GPU stack |
| **Existing VMware Workstation** | vmrun wrapper,Tools copies,test VM,snapshots,CDP forwarding | Virtual3D for initial launch,not physical GPU performance/DDS quality | Lowest reuse cost;4vCPU/8GB/120GB only new-player base;thousands-Mod suggest8vCPU/16–32GB,separate large data disk,measure RAM |
| Other VM / physical fallback | VirtualBox supports snapshots; a dedicated Windows machine can use a system-disk image | Validate 3D compatibility; recheck GPU-sensitive issues on physical hardware | Do not migrate drivers for Phase 1; no VM pass automatically proves physical game performance |

Existing basis: `scripts/vm/README.md:1`, `vm.ps1:16` Win11-Player-Test; host `D:\VMs\PLAYER-TEST-AI-HANDOFF.md` existing-environment/test-reset rules: test VM only, export evidence before restore, do not alter Clean-Player. New clones/scenario snapshots designed here; **no existing VM operation this round**. Restore game,Mod library,RimArcho home,recycle bin,user data,Steam local state,tasks,not DB alone.

### 9.2 Provisioning/reset

Create three fixtures in user's own VM: small (20–50 controlled Mods/minimal XML/Def dependencies), realistic (thousands of real Mods,few valid launch lists), online (small Workshop sets/network failures). Copy owned game/DLC `steamapps/common/RimWorld`, `workshop/content/294100`, required manifests/ACF,local/locked/CMD sources from host; inventory/hash/size first, all writes guest disk. Read-only source transfer, copy to guest-writable before tests; no delete-through-host writable shares. Existing game/DLC authorization applies; anonymous CMD only for supported Workshop, not guaranteed paid game/DLC provisioning.

Real DB copies stay in the user's own private VM, never committed, uploaded, or used as public CI fixtures. After backup import, rebind every environment's game/userdata, shared sources, lock library, and packs paths; prefer the same drive letters and relative layout. Migrate through product `folder set`/`env rebind`, then scan to verify identity and history; do not corrupt ID/path relationships with whole-database string replacements. `internal/env/env.go:167` supports RIMARCHO_HOME; also isolate APPDATA/LOCALAPPDATA, logs, dependency caches, and WebView2 profile. Changing home alone does not isolate absolute paths in the DB.

Normal shutdown after provisioning; OS+tools and fixture+build baselines. Each scenario differencing disk/clone, each driver starts same fixture. Export sanitized diffs,input traces,results,needed logs privately before reset; verify prior processes exited, restore/hash. Record reset/copy times to consider independent cases/shared read-only batches; slow resets do not justify chained write contamination.

Steam subscriptions are account-global; real account may be used within user-accepted scope. VM snapshots never undo server subscriptions.

### 9.3 "Actually loaded in game"

Reuse `archo test run <plan> --json`: `internal/cmds/test.go:15` has `--map`/`--timeout`/`--settle`; `internal/app/gametest.go:80` isolated userdata,direct launch,logs/memory/report; `:112` link check,`:179` test config. Does not create missing GUI save links: save/verify in VM first. `Success()` (`:66`) requires menu/map/no non-info clusters but **does not directly fail nonempty Missing**; wrapper asserts missing empty,requested vs actual loaded match,no Core-only fallback. Exit/process alone never passes.

RimDevTools second: local repository `D:\Game\RimModManager_Upgrade\RimDevTools`; `RimDevTools/Commands/TestCommand.cs:154` quick/manual/auto/xml;`:339` quick waits RUNTIME_READY_TO_CLOSE/main-thread heartbeat,auto runId COMPLETED;`:825` executed≥planned. `source/AutoTestRunner/AutoTestReportWriter.cs:51` Result/Manifest/Evidence; `source/LogSyncMod/LogSyncMod.cs:29` startup signal. Current runId/log offset prevents stale results; add limited map/Def/window/recipe assertions.

Tools currently1.6 (`RimDevTools/README.md:1`), not promised1.7. Install LogSync in1.6,quick+few deterministic auto/xml;1.7 uses local test run until compatibility validated. It transactionally writes/restores ModsConfig/LogSync settings and may use active-mods-file (TestCommand.cs:586); when testing RimArcho-saved lists, retain original hash/check no list substitution,verify restore afterward. Passing samples never proves all gameplay across thousands of Mods.

## 10. Later special cases

Recording/replay/manual handoff later. guidev `/call`/frontend Call.ByName possible observation points,no complete recorder now; future records sanitized semantic actions,not raw account parameters. Steam Guard/client dialogs/native pickers/player-judgment game checks blocked/manual initially,UIA/manual continuation later. No Phase1 auth/replay protocol.

## 11. Phases, effort, evidence

Net person-days for experienced developer,excluding download/load waits,platform faults,major performance fixes. Deliver resettable10 scenarios with failure diagnosis first; do not await full CLI parity.

| Phase | Deliverable/exit | Estimate |
| --- | --- | --- |
| Phase1a shared base/high-frequency stress | Reuse VM/CDP,timeouts,small fixtures,canonical snapshots,DOM/state oracles;reproducible1–4,input interleaving,crawl discovery |3–4 person-days|
| Phase1b ordinary management loop |5–9,file/code/text/collection+8 formats,cancel/continue,disable/unsubscribe/delete,sort repairs,save/launch/test run;P0-1 first,D1 differential after merge;online/offline separate|4–6 person-days|
| Phase1c controls/real scale |10 state graph,crawler/write oracles/reachability;private real DB,thousands-scale budgets,reproducible traces/snapshots,explicit gaps|3–5 person-days|
| Phase2 differential completion |P0-2/3/4,P1 CLI,full cross-env links,credit recovery,textures/packs interruption,migration/uninstall separate clones,periodic game assertions|6–10 person-days,depends shared batches|
| Phase3 special interactions |§10,physical GPU/compatibility matrix,limited manual checks|Estimate later|

Phase 1 takes about 10–15 person-days; existing drivers reduce platform rebuilding. CLI P0-1 takes about 1–2 person-days and can fit alongside; schedule other P0 items by actual dependencies. Deliver 10 scenario results + an action-reachability ledger + earliest-divergence diagnosis, not "the script finished without crashing." Start CI with synthetic C/A fixtures; run W serially in the private VM, online/heavy game cases nightly. Results default outside the repository; Git receives scenarios/runner/docs only, never DBs, screenshots, tmp, or browser profiles.

Evidence→finding→implementation (static findings validated;thresholds/new runner proposed,acceptance not executed):

| Evidence | Finding | Path/acceptance |
| --- | --- | --- |
|E01:gui/tasks.go:411;background-nonblocking@bd8d5d8:386;§1 commands|F01:D1 exists in baseline,in-flight fix shares app|P01:merge then scenario6,two games' links/canceled item results|
|E02:gui/ops.go:128;cmds/loadout.go:250;app/dependencies.go:164,sortnotices.go:206,useinstalled.go:13|F02:three GUI repairs no CLI,incomplete sort notices|P02:P0-1,scenario8 three drivers,one undo/four notices|
|E03:view/verbs.go:105,:178;app/loadout.go:555;store/loadout.go:57|F03:suggested placement/in-place restore/composite history not command-loop equivalent|P03:P0-2,1/4/8 entries/staging/history|
|E04:spikes/guidev/main.go:30,:145;gui/gui.go:79;scripts/vm/cdp.mjs:19|F04:API/real driver exist,different concurrency|P04:Phase1a,1–4 require W,C/A domain only|
|E05:assets/js/screens.js:138;main.js:407,:1111;problems.js:206;needscache.go:139|F05:status/blank/keys/event filtering outside CLI|P05:continuous UI/input/late events,not final DB only|
|E06:assets/js/refnums.js:26,:81,:186;view/ops.go:74;environment.js:21|F06:refnums assists,not complete denominator|P06:Phase1c runtime crawl/oracles,blocked/new entries|
|E07:cmds/test.go:15;app/gametest.go:66;external TestCommand.cs:339,:825|F07:load/game validation exists,default success insufficient|P07:scenario9 missing/actual loaded/runId/manifest|

Reproduce source findings through §1 rg/git show;external RimDevTools/VM handoff local to user. No external-project credentials,private DB contents,or runtime artifacts embedded.
