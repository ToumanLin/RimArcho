# Environments sharing one game install, 2026-11-11

Status: **decided 2026-11-11 (§10, specified in 03 §6); not implemented**. The owner has chosen an environment to mean **one game install plus one user data folder**. Several environments may share an install; plans remain global. This note evaluates that model against the working tree, including existing uncommitted work. References below are repository-relative `path:line` plus a symbol; line numbers describe this checkout.

**Recommendation:** introduce a game record and preserve existing IDs. Share physical inventory, scans and static analysis by game; retain current plan, launch configuration, finding ignores and launch history by environment. Keep file-derived state keyed by its actual path. Merely removing the duplicate-game check would create duplicate inventories and misattribute running games.

No game, GUI, player database, installed Mod, save or host RimArcho data was opened. The sibling RimCrow repository was read as source only. No download, build, test or runtime experiment was needed for this source investigation. Runtime log redirection remains unverified (§4). This is a proposal, not a replacement for the specifications.

## 1. Model and findings

Read with 03 §2–§4, 04 §4, 05 §3, 06 §4, 07 §1.7 and §6.2, 09 §2–§4, 10 §3, 12 §1 and §4, 19 §2 and §4, and 20 §2. The background request is ToDo.md, “Feedback”, first item.

| Evidence | Finding | Design consequence |
| --- | --- | --- |
| `internal/app/envs.go:183` `EnvAdd`, `:265` `envOfGame`; `internal/store/envs.go:96` `Sources` | The application rejects a second environment for a game; inventory visibility uses its ID | Introduce GameID before allowing the second environment |
| `internal/store/persist.go:107` `ApplyScan`, `:136` Lineage insert | Instances inherit source ownership; Lineages are already global | One official/local source pair and inventory per game; no per-environment copies |
| `internal/app/refs.go:50` `Index`, `:81` `readIndex` | The cached index combines physical inventory with an Environment pointer | Reuse inventory across sibling environments, but rebind environment-specific context |
| `internal/app/gamerun.go:169` `RunningGame`, `:244` `WhatRuns` | Executable path selects an environment first; redirected data currently distinguishes packs and tests, not sibling environments | Identify game first, then environment by effective save-data root |
| `internal/app/creditreplace.go:134` deployment capture, `:187` replay | Credit replacement visits user-data roots and calls the normal save/deploy operation for each | Separate each Config rewrite from shared Mods-directory reconciliation |
| `internal/app/logowner.go:52` `logOwner` | A matching startup game path is accepted before checking environment launch identity | Matching the install is insufficient to prove which sibling environment wrote a log |

Proposed ownership:

- **Game:** ID, canonical directory key, game_root, full game_version, created_at; official/local sources and their Instances; scans, identity questions, static analysis slots, game assembly/type index, physical link and directory operations. A stable display name is useful for other-game source labels.
- **Environment:** existing ID and name, game_id, user_data_root, launch method, current_plan, created_at; `env:<id>:launch_args`, log selection/seen preferences and launch records.
- **User-data root:** ModsConfig write state, actual Config/HugsLib files, saves and Monitor observations. A root is a path identity, not another mandatory database entity.
- **Global:** plans and all their editing/history state, shared Mod folders and their Instances, Lineages/player annotations/rules, content-addressed facts and caches.

Use a joined Environment view in Go to retain convenient GameRoot/GameVersion reads while adding explicit GameID. Write game fields through game operations. Do not use an Environment with its ID silently overwritten to mean a game: launch and settings callers would then corrupt the wrong scope.

**Uniqueness:** one game per canonical install directory; unique environment name; no two environments of the **same game** with the same effective user-data root. Preserve existing cross-game sharing of the default user-data directory (03 §3.4). A global unique user-data-root constraint would break valid existing users.

A sibling switch changes selected environment/current list and reads that root's Config; it creates no source, performs no scan, and reconciles no Mods links. The next explicit save/launch deploys its selected list under the existing global running-game guard. Environments isolate saves/settings, **not the physical Mods directory**.

## 2. Store ownership, table by table

All fifteen tables containing `environment_id` in `internal/store/schema.sql` are accounted for below. “Keep” describes semantics, not a requirement to preserve a misleading column name forever.

| Table / schema line | What is stored today; store evidence | Proposed scope and migration |
| --- | --- | --- |
| environment, 6 | Game path/version, user root, launch and current_plan in one row; `envs.go:15` InsertEnv, `:22` UpdateEnv | Split game fields out; preserve environment ID, name, root, launch and current_plan |
| source.environment_id, 108 | Owner ID for official/local, empty string for shared folders; `envs.go:96` Sources and `:127` InsertSource | **Game/global**; rename to game_id. Keep shared sentinel and UNIQUE(game_id,label) |
| instance.environment_id, 12 | Derived from source owner, not simply the scanning environment; `persist.go:107–155` ApplyScan | **Game/global**; rename to game_id, preserve every Instance ID |
| lineage.environment_id, 10 | **Empty string** on creation; `persist.go:136`, `mutate.go:146`; `store.go:44` Shared | **Global**; keep empty, or remove obsolete column in a separate cleanup |
| scan.environment_id, 34 | Initiating environment; scan can include global sources; `persist.go:18` BeginScan | **Game** execution/history scope. Rename to game_id; shared-only history with empty owner remains valid |
| identity_question.environment_id, 42 | Initiating reconciliation scope; `persist.go:220` ApplyScan | **Game**, including questions about shared-source observations. Do not duplicate questions for sibling environments |
| user_rule.environment_id, 46 | **Empty string**, even though AddRule accepts envID; `loadout.go:263–310` AddRule/Rules/RestoreRule | **Global**. Rules reads all rows; do not backfill with GameID |
| launch.environment_id, 49 | Actual launching environment; `launch.go:114` AddLaunchRecord; `launches.go:23` LaunchRecordBefore | **Environment**. Keep. Add historical game/root snapshots if launch attribution is strengthened (§4) |
| analysis.environment_id, 51 | Static run slot by owner, game_mm and set_key; `analysis.go:193` SaveAnalysis, `:264` LatestAnalysis | **Game**, plus version and enabled-set key. Rename to game_id; findings/patch facts follow run. Do not share merely because game_mm matches |
| finding_ignore.environment_id, 64 | Actual environment accepted by SetIgnore/Ignores; `analysis.go:404–434` | **Environment** recommended: accepting a warning in one play context must not accept it in another. No GameID rewrite or automatic copying |
| lineage_data.environment_id, 71 | **Empty string** in setData regardless of envID argument; reads all rows; `playerdata.go:16,66` | **Global** alias/note. Keep sentinel or remove later |
| lineage_tag.environment_id, 75 | **Empty string** in AddTag; globally read/ordered by lineage; `playerdata.go:35,73` | **Global** tags. Keep sentinel or remove later |
| plan.environment_id, 86 | **Real creating environment ID**, not empty. EnvID is provenance, not ownership; `plan.go:16`, `:53` Plans, `:103` CreatePlan | **Global plan with environment provenance**. Keep nullable/logical origin; no cascade when environment disappears. created_game captures version. Do not rename this blindly to game_id |
| credit_session.environment_id, 148 | Copied from launch record by CountCreditSession; key includes batch, env, launch, start; `credit.go:98–119` | **Environment/history**. Keep even after environment/launch deletion; counted sessions must survive |
| credit_deployment.environment_id, 152 | Writer environment for a pending Config handoff, with plan_id; `credit.go:162–183`, `app/creditreplace.go:140–177` | Semantically **user-data-root obligation**, currently carried by environment ID. Retain environment for context; add captured root/key so removal or root edits cannot lose/misdirect it |
| game_assembly.environment_id, 204 | Physical game DLL metadata; `gametypes.go:49,68,99` | **Game**; rename to game_id, retain assembly IDs and associated game_type rows |

The remaining schema tables have no `environment_id`; their indirect ownership matters just as much:

| Tables / schema lines | Current key and proposed treatment |
| --- | --- |
| instance_alias (19), snapshot (22), snapshot_file (31), snapshot_list (172), change_event (37), instance_note (104) | Source/Instance/snapshot/scan children. Follow physical inventory, therefore game or global source; preserve IDs and history |
| patch_fact (55), finding (60) | Children of static analysis run: game + version + enabled set. Apply the selected environment's ignores at read time; do not embed ignores in shared static results |
| fact_set (161), content_state (164), def_fact (173), assembly_fact (178), type_fact (182), harmony_fact (183), harmony_id (188) | Instance/content/version facts, already shared. No new environment dimension. Prune only when no remaining inventory/analysis reference needs them (05 §3) |
| game_type (208) | Child of game_assembly, follows GameID indirectly. Some type lookup readers currently query by Store.MM across installs (`gametypes.go:168–176`), not strictly the selected environment: clarify this existing behavior instead of promising strict install isolation without changing queries |
| workspace (89), workspace_undo (92), loadout_version (94), plan_collection (124) | Global plan children. Keep all on environment removal. Retain history, staging meta, following collections and undo independently of current_plan |
| modsconfig_state (99) | **Already user-data-root scoped**, normalized root primary key, writer_env is real environment provenance; `launch.go:29,41,58` UserDataKey/ModsConfigStateAt/SetModsConfigStateAt. Keep path key and writer_env. Deprecated env-ID wrappers only resolve that environment's root |
| save_cache (215) | **Absolute save-file path** plus size, mtime, parser; raw facts rather than environment ownership (`savecache.go`). Naturally follows each user-data root; no GameID or EnvironmentID column needed |
| log_cache (83) | **Log path** plus size/mtime/version/input_key (`logcache.go`, `app/logs.go:272` logInputKey). Keep path key; ensure input and in-memory keys distinguish context-sensitive reports after sibling switching |
| credit_batch (138), credit_item (142) | Global download/credit task and Workshop item. Keep; deployment is the special root-scoped part, sessions the historical part |
| managed_link (129) | Physical mods_path/name/target and journal operation, not an env ID. Keep physical ownership; enumerate each game's Mods path once |
| texture_file (112), texture_mod (120) | Path/Instance-based conversion records; follow physical copies, not play environment |
| source_use (68), source_library (189) | DLL hash/Instance or DLL hash/game build. Keep shared content identity |
| tag_note (78), tag_color (80), colony_name (82), id_alias (107), version_mute (157) | Global tag/colony/identity/version data. Do not duplicate by environment or game |
| meta (5) | Mixed scopes: schema/current_env/global cfg; env launch args and log preferences; plan staging/Monitor-added; root-scoped Monitor history. Audit prefixes individually |
| archo_index_gen (installed separately) | `indexkey.go:16,31` lists index tables and installs triggers on open. Add game to invalidation; preserve a valid generation counter through migration |

There is no need for a generic “user data” table just to re-key save_cache/log_cache. The **new root-scoped requirement** is durable handoff identity and, preferably, historical launch/root attribution, not duplicating these caches.

The plan table's UNIQUE(environment_id,name) is weaker than the global naming behavior: PlanByName handles names across all plans (`plan.go:81`). Preserve the existing semantics during this split; enforce global uniqueness only after checking legacy ambiguity, as a separately specified change.

## 3. Code impact and approximate size

These are **review estimates of production files**, not additive totals or measured implementation timings. Shared files occur in several rows. As a search bound, `rg -l 'e\.ID|Env\.ID' internal/app -g '*.go' -g '!*test.go'` matched 40 files in this working tree; not every hit should change.

| Area | Concrete assumptions / entry points | Required change; estimated files |
| --- | --- | --- |
| Domain/store and construction | `env/env.go:15` Environment; `app/app.go:223` OpenWith and `:327` Env; `store/envs.go:15–97` | Join selected environment to game; distinguish inventory selectors from context selectors. **6–10** |
| Scans/reconciliation | `app/scan.go:102–250` scanEnvironment uses e.ID for Sources/Known/CurrentFiles/CurrentHashes/BeginScan/ApplyScan; `:284–314` counts, official tags, then Loadout | Pass GameID to physical scope; version update once per game; post-scan ModsConfig sync only for selected root, other roots lazy on use. Child scans keep --env as context. **6–9** |
| Index/static analysis | `app/refs.go:50–120`; `store/indexkey.go:16`; `app/readsession.go:59,110`; `app/analysis.go:134,164,275`; `app/gametypes.go:22` | Cache inventory by game + store generation, while retaining env context for Loadout. Never reuse sibling's Index.Env/current plan. Share analysis lock/run slots by GameID and retain needed sets. **8–12** |
| env use / links | `app/envs.go` EnvUse/PrepareEnv; `app/envlinks.go:26` prepareEnvLinks | Add same-game fast path with **no call** to prepareEnvLinks. Keep cross-game behavior: inspect destination Config/workspace, prefer destination local copy, create missing links, scan only when needed. **3–5** |
| Other games' Mods | `store/planglobal.go:95,123` ElsewhereMods/ForeignInstances; `app/elsewhere.go:16–43`; `app/sourcefilter.go:75` LocalValue | Exclude current GameID, not env ID; sibling local Mods are “here”. Enumerate foreign sources once. Replace local-<environment name> with stable local-<game name> (or stable source label) while preserving legacy labels initially. **4–7** |
| Physical directory/link operations | `app/directories.go:34–49,304–336`; `app/uninstall.go:689–712`; deploy/copylocal/packlinks/linkhealth | One managed game directory and link ledger view per GameID. Moving an install updates every sibling's joined path/version; choosing a different install for one environment is a different action. **8–12** |
| Running game and logs | `app/gamerun.go:169,244`; `app/logowner.go:52,87`; `app/logsource.go:32,54,99,121`; `app/logs.go:113,272,1372` | Resolve game then data root; preserve env-level launch/source/fingerprint rules, log selection and seen state. Fix same-root executable attribution and unknown arguments. **8–12** |
| Saves / Monitor | `app/saves.go:132,662,725,751`; `app/monitorsettings.go:61,69`; `monitor/src/MonitorMod.cs:103` | SaveFolders already deduplicates real save paths and associates environments. Preserve per-root Monitor state, select environment by root/launch, not first install match. **4–7 Go files**; no Monitor format change required |
| SteamCMD / credit | `app/envs.go` adoptGameWorkshop call; `app/creditreplace.go:98–235`; `store/credit.go:98,162` | Adopt game-contained Workshop once per game, preserve shared source identity. Validate client copy per game; rewrite every affected user root but reconcile shared Mods once. **5–8** |
| Packs | `app/pack.go:41–46,655,800,963`, SaveFolders pack mapping at `saves.go:166` | Keep manifest env as origin environment name for compatibility; decide list visibility separately from compatible game selection. Physical links use game; userdata remains pack-owned. **3–5** |
| Test runs | `app/gametest.go:144,203,332`; `app/testrunlinks.go:91,153` | Keep test userdata/runs/baselines per environment; guard/recover shared Mods mutations by physical game. Check interrupted journals from siblings. **4–6** |
| CLI/MCP | `cmds/env.go:14–35`; `surface/cli/cli.go:246`; `surface/mcp/server.go:83`; `cmds/guide.go:120` | Keep --env/tool env selecting environment; scan resolves its game. Update env commands/help/status/guide. MCP currently leaves setup to player: do not invent env mutation tools unless chosen explicitly. **5–9** |
| GUI/TUI | `view/settings.go:150,165`; `view/shell.go:62,429,538`; `surface/gui/environment.go:43,101`; `surface/tui/palette.go:270`, `update.go:676` | Group environment switcher by game, add sibling creation, split settings groups; detection must no longer mark an install unavailable merely because an environment uses it. Preserve async/stale-request guarantees. **10–16** plus locale entries |
| RimCrow import | `app/managerdiscovery.go:22–66`; `app/rimcrow.go` RimCrow import/openCopy; `view/managerimport.go` | Read profiles, preview mapping, import their lists/settings references; current importer has no profile registration. **5–8** plus fixtures |

### Switching and deployment details

03 §2 item 14 describes links between **different installs**. The literal `_Archo_env_<Instance ID>` prefix (`app/envlinks.go:20`) can stay for backward compatibility: its suffix is an Instance ID, not an environment ID. Do not rename existing links just to match the new vocabulary. New sibling environments need no such cross-game link.

prepareEnvLinks currently reads the destination's ModsConfig and current workspace, can borrow local candidates from the previous environment, and checks LastScan(e.ID). Leaving that code unchanged means a sibling can be treated as a new unscanned game, or attempt self-links. Change the physical lookups to GameID, and bypass switch-time deployment for same-game transitions.

A normal launch can still change shared links: two environments can select different locked versions of a package. Only one game may run at once, so deploy on explicit save/launch with the existing launch lock and game-running check. Switching environment while another sibling runs must not alter its links or Config and must not make the UI label that running process as the newly selected sibling.

MoveDirectory/SetDirectory and env set --game must distinguish:

1. **Relocate this installed game:** preserve GameID, rebind both sources, update all environments together, and show every affected environment.
2. **Use another installed game in this environment:** change only game_id after validation; do not rebind the original game's sources for its remaining environments.

The current env set path mutation combines these operations. A shared install makes that ambiguity unsafe.

## 4. Running games, launches, logs and user data

### Resolve ownership in two steps

RunningGame currently iterates Envs with the current one first, probes the executable under GameRoot, and immediately returns that environment. WhatRuns treats **no savedatafolder as the passed environment's own root**, even when that environment has custom data. Those assumptions fail for sibling environments.

Proposed resolution:

1. Probe each canonical **game** once; keep the install-wide safety result even if attribution is unknown.
2. With readable arguments, resolve savedatafolder; absence means the game's actual **default** data root, never the selected environment's custom root.
3. Match that root to a sibling environment, a pack userdata directory, or a test directory. Otherwise show the explicit unknown root.
4. With unreadable arguments, consult launches of **all environments of that game** around process start, retain existing slack rules, and label inference. If evidence is ambiguous, report the game and “environment unknown”; do not choose the current sibling.
5. Return both GameID and optional EnvID/root in running state. GameRunning may still mean “this install is busy”; UI LiveLog must require the right environment/run, rather than interpreting non-nil as “this environment is running”.

Keep launch.environment_id, PlanID, fingerprint and version_n (`app/launch.go:106`); pack launches still record the selected environment and method pack:<name>. Add captured game_id and effective user_data_root to **new** launch records. Environment paths can be changed later, so joining historical records only to today's root can misattribute history. Backfill old records conservatively: today's environment root for ordinary launches, pack root when reconstructible; mark unknown historical roots rather than claiming certainty.

Log ownership has two questions: “which install supplied these assemblies?” and “which environment/list/settings produced this run?”. A Mono startup header answers only the first. Update logOwner so a same-install match does not short-circuit root/session/launch attribution. Keep judge's current environment, plan, version and fingerprint checks; siblings sharing one plan still have different settings and runs. Preserve sharedEnvs for **cross-game shared roots**, not all environments of one game.

### Player.log: known assumptions and unresolved runtime behavior

- 03 §3.3 says <user root>/Player.log before 1.7, and <user root>/Logs/Player.log from 1.7.
- 10 §3 explicitly says pack log redirection is **not verified in game**, describes earlier Unity logs as fixed/shared, and leaves savedatafolder behavior to validation.
- `app/gamerun.go:302` RunLog's comment likewise describes older Unity's fixed log. Its implementation tries a run's redirected root if present, then falls back to LogPaths for the selected environment.
- `app/logs.go:113` LogPaths searches **both layouts, in version-preferred order**, independently for current and previous files. It does not choose the newer mtime for unknown versions as 03 §3.3 says. `env.PlayerLog` is a separate helper; callers must not assume its selection behavior equals LogPaths.
- 20 §2 and `internal/gametest` deliberately redirect older Unity with -logfile, and handle 1.7 Waylith with -NoFileLog/stdout because its normal file path is described as hardcoded LocalLow. This is additional evidence that save-root isolation alone is insufficient to promise log isolation, including on 1.7.

**Conclusion:** savedatafolder redirects saves/Config and Monitor's own path in the intended model; this research does **not** establish where regular 1.6 or 1.7 Player.log actually lands. Do not ship log isolation as a verified claim. In the VM, launch two environments of one install sequentially with distinct marker settings, inspect default and custom log paths, and repeat for direct/Steam and supported versions/platforms. If older Unity needs explicit -logfile, add it for newly isolated environments with a specified compatibility policy; do not silently relocate existing users' logs during schema migration. Decide 1.7 separately after observation.

Launch argument parsing also needs attention: `app/launch.go:155` uses strings.Fields on LaunchArgs, so a quoted path with spaces is split. RimCrow stores run_commands as an argv list (`models.py:59`; `mgr_profile.py:616` extends args). Joining that list with spaces is **not** a lossless import. Use an argument parser/serialization contract that round-trips tokens; never execute imported text through a shell. Refuse or explicitly resolve imported -savedatafolder conflicting with the environment root and log-path flags that undermine attribution. Show effective argv in preview.

### Saves, Monitor and caches

SaveFolders already enumerates all environment Saves roots and all pack roots, deduplicating real paths (`app/saves.go:132–176`). newSaveMatcher/match use launch records from the folder's environments, then saved Mod lists. Preserve this root-first approach; no duplicate scan/cache of sibling saves. Filtering --env means **that environment's saves**, not all saves of its shared install. SameTarget remains environment + pack identity (`saves.go:792`), which is appropriate.

Monitor writes ArchoMonitor.log and its previous file under GenFilePaths.SaveDataFolderPath (`monitor/src/MonitorMod.cs:103`, `Writer.cs:52`). Its recorded Mod settings history is already retained per root as meta `mod-settings:<PathKey(root)>` (`app/monitorsettings.go:16,61`), not by environment ID. Keep it that way; use the selected environment's root for normal lists and the pack/test root for those runs. Do not copy Monitor log/history into a new environment.

Raw save_cache can remain context-independent. Log reports include attributed/contextual data: audit logInputKey and the cache read/rejudge path so the same physical log and same plan cannot carry another environment's “current run” classification. Add environment/root/run identity where needed, or cache raw attribution and compute ownership separately. Retain per-environment `log.selected:<id>` and `log.seen:<id>`.

### Credit handoff and isolated tests

credit_session is historical counting and must survive environment removal (`TestCreditSessionsSurviveRemovingGame`, `credit_test.go:682`). credit_deployment is a pending obligation to rewrite the particular Config that used the SteamCMD copy. Re-keying it to GameID would collapse two sibling roots and leave one config stale.

Capture root/key and writer at handoff creation. Rewrite all affected roots, honoring outside edits/backups and token resolution; evaluate required physical links across the resulting projections before recycling the old copy. Repeated writeGame calls for siblings can remove each other's links because deployment is plan-oriented: one physical reconciliation or a deliberate retained-copy outcome is necessary when projections cannot coexist. Never recycle while another sibling's Config or interrupted journal still needs the old copy. Missing/deleted environments need an explicit durable resolution, not endless “deployment environment no longer exists”.

TestRun uses tests/<environment ID>/userdata and runs (`app/gametest.go:203`); preserve that separation for settings and baseline comparison. The temporary link/hide journal mutates the **shared game**. A test in sibling B must detect/recover sibling A's interrupted links.json before changing Mods, while still obeying ownership checks. Removal must not discard the only recovery record. Proton isolation remains refused.

## 5. Concrete migration and rollback

The checkout creates schema **35** (`store/store.go:19`); upgrades start at public schema 29 and each step has its own transaction (`:142–198`). 04 §4's explanatory list stops at 33, so use the code's upgrade chain (34 tag order, 35 Def content). The next unused schema version, rather than a hardcoded number in a long-lived plan, owns this change.

Recommended single atomic structural upgrade:

1. Back up the database using the existing supported backup mechanism before upgrading; close older RimArcho processes. WAL mode is enabled and foreign_keys=1 is connection-wide (`store.go:103–118`); do not copy a live bare .db as a purported complete backup.
2. Create game(id PRIMARY KEY, name, game_root, game_root_key, game_version, created_at). For each environment, insert a game with the **same ID**, old name/path/version/time.
3. Create the replacement environment shape with existing id/name, game_id=id, user_data_root, launch, created_at and **unchanged current_plan**. Preserve meta current_env and all environment-keyed settings.
4. Rename game-scoped columns/index definitions in source, instance, scan, identity_question, analysis and game_assembly. IDs stay unchanged; game_type and analysis children need no data rewrite. Leave global sentinel columns and environment/provenance columns alone.
5. Add root identity to credit_deployment and any new launch snapshots. Backfill from writer environment where possible, report unresolved pending obligations; do not discard counted credit_session rows. Handle pack launch roots distinctly.
6. Install constraints/indexes for game lookup and (game_id,user_data_key) uniqueness; path policy below must be settled first. Add game to index-generation triggers. Recreate stale triggers after table replacement and invalidate derived in-memory state.
7. Check row counts, references, root keys, current selection, shared sentinel values, and duplicate groups before committing the new schema_version in the same transaction. Fresh schema.sql and upgrade output must agree. Report progress through OpenUpgrading when needed.

The ID-preserving idea is sound **only for the six game-scoped environment_id columns above**. It is not a reason to reinterpret every old environment ID as a game ID. The split needs very little bulk rewriting: keep snapshot/file/fact blobs intact, no filesystem scans, no new userdata, no links, no ModsConfig writes, no renamed environments/plans/packs.

**Zero visible change for normal existing users:** one old environment becomes one game plus one environment; all paths, names, IDs/references, launch args/method, current list, history, accepted findings and packs remain the same. The UI can continue showing the existing name on its chip. It reads the same files and launches the same argv. Different games sharing default userdata remain allowed.

**Path collision risk:** EnvAdd checks PathKey, whereas process matching and save enumeration sometimes resolve real paths. Old rows can therefore represent aliases of one physical install despite the nominal one-environment rule. Do not silently merge them while claiming zero change. Preflight canonical collisions and offer an explicit repair/merge path; a conservative upgrade can preserve legacy alias groups without enforcing a stronger unique canonical key until repaired, while new additions use canonical matching. Same-game same-root conflicts after merging require deduplication/selection, not a new empty root.

Use the same canonical path policy for uniqueness, import, process matching and root obligations: clean/absolute, resolve existing links and Windows short paths, apply platform-correct case behavior; for nonexistent own roots resolve the nearest existing ancestor. A stored root key must be refreshed on a confirmed relocation. UserDataKey currently cleans and folds Windows case but does not resolve links (`store/launch.go:29`); it is not enough alone for duplicate prevention.

**Rollback:** an error within the new upgrade step rolls back DDL/data/version together; previous committed upgrade steps remain committed by the existing framework. Test injected failures before/after environment replacement and trigger installation. After a successful upgrade there is no safe automatic downgrade once sibling environments exist. Close all RimArcho processes and restore the consistent pre-upgrade backup to roll back; newer user changes are then lost. Old executables reject a newer schema on normal Open, but already-open sessions do not re-run migration checks (OpenSession is not the upgrader); avoid mixed-version processes during this upgrade.

Do not add strict FKs to historically dangling provenance columns as part of the split. plan origin, credit_session, and old ModsConfig writer references can legitimately outlive the environment. Resolve that retention policy explicitly.

## 6. Adding, changing and removing environments

### CLI and initial state

Keep the existing shape and permit a taken game directory when userdata differs:

```text
archo env add colony-b --game <existing-game-directory> --saves own
archo env add colony-b --game <existing-game-directory> --user-data <existing-profile-root>
archo env add colony-b --like colony-a --saves own
```

The third command is proposed convenience, not present today. --like selects the source environment's game, launch method/args and initialization source; refuse --like with a contradictory --game. The first two reuse the game row/source pair and need no scan. Bare --game still defaults to shared/default for compatibility, but if that game/default pair already exists, refuse with the existing environment name and suggest --saves own/--user-data. Do not silently switch save mode.

Recommended explicit initialization:

| Creation kind | Config/HugsLib | Current plan |
| --- | --- | --- |
| Bare --game ... --saves own | Empty user root; game creates defaults. No implicit dependence on whichever environment happens to be current | Existing first-use behavior (07 §1.4), with own per-env selection; no copied selection |
| --like source ... --saves own | Seed Config/*.xml except ModsConfig.xml and HugsLib using the pack step-4 rules | **Copy source's current workspace into a new globally named plan**, select it for the new environment; do not copy history, undo or finding ignores |
| --user-data existing-root / RimCrow import | Adopt in place; copy nothing and overwrite nothing | Import that root's ModsConfig.xml to a new global plan and select it, with normal missing-token handling |

A --like plan copy avoids two apparently separate play contexts accidentally editing one global workspace. The player may later explicitly select the same global plan in both environments. A clone operation must state that it clones the current workspace (including unsaved list edits), not the last written ModsConfig. Plan duplication should use existing app editing/import primitives and unique naming.

Extract initialization from `app/pack.go:963–988` and `app/modsettings.go:321` copyHugsLib; make errors visible rather than inheriting the pack code's best-effort Config copying. Seed only a new/empty destination, never replace existing files. No saves, logs, caches, Monitor history, crash reports or screenshots. Do not copy ModsConfig.xml: write it on explicit save/launch through normal version/token logic. Thereafter each root's settings are independent. Copy only permitted files and respect the existing no-follow-inside-Mod boundary.

Refuse duplicate name, invalid game, same canonical (game,userdata) pair, overlapping/unsafe roots, contradictory flags, destination collisions during seeding, or unsupported launch redirection. Guard against a running source/destination while cloning Config and against concurrent add races using the database uniqueness constraint. Roll back only newly created files/rows; never clean an adopted external root on failure.

For Proton on Linux, EnvAdd/EnvSet reject --saves own (`app/envs.go:208`); gamePlan also rejects **custom** nondefault roots and savedatafolder args (`app/launch.go:145–161`). Close that registration/launch gap: reject both own/custom sibling isolation until redirection works. With one allowed default root, a second runnable Proton environment of that install is impossible under the new uniqueness rule. Offer the reason, not an environment that can never launch.

### Removal semantics

Split Store.DeleteEnv (`store/envs.go:186`) into environment cleanup and last-game cleanup within one transaction.

- **Remove one of several environments:** delete its environment row, launch records and finding ignores, clear env-scoped preferences, and choose another current environment if necessary. Retain game sources, all Instances/history, identity questions, scans, game types and static analyses. Keep global plans and their editing/history state. Keep root-derived records that remain useful or are referenced; retain counted credit sessions.
- **Remove the last environment:** also forget the game record and its official/local inventory, analysis and assembly rows. Run the existing keepLists conversion of disappearing Instance IDs to packageIds across workspace/undo/versions (`planglobal.go:25`), then prune empty Lineages/player data/rules and unreferenced facts. Preserve shared-source Instances/history. The existing deletion preserves scans with shared change events by clearing their scope (`envs.go:207–208`); keep that behavior.
- **Remove a game explicitly, if offered:** preview all its environments and perform the two operations above for all of them. A separate command is optional; last-environment removal can be sufficient for phase 1.
- **Files:** env remove remains “forget records”, never delete the install, Mods, own or external userdata, saves, packs, or physical links. Data left under RimArcho's own userdata can be surfaced separately for cleanup, following 02. Root caches and ModsConfig write history are not blindly deleted while another game can share them.
- **Pending obligations:** resolve/reassign root-based credit deployments and interrupted test/link journals before discarding their owner; do not delete the only recovery information. Block removal while the install runs, as today (`app/envs.go` EnvRemove), even if a different sibling is running.

Existing DeleteEnv comments say it deletes “its plans”, but the implementation calls keepLists and deliberately keeps global lists. Follow the code. Its current deletion loop also does not clean env launch-arg/log meta or pending credit deployments; do not copy these omissions into the new contract.

## 7. RimCrow profile import

### What exists in the reference

`../RimModManager/backend/database/models.py:51–69` GameProfile stores id/name/game_install_path/user_data_path, run_commands, prefer_steam_launch, source-selection flags and metadata. `backend/managers/mgr_profile.py:159` _ensure_default_profile creates **empty game and userdata paths**, not an automatically usable install. `:296–343` create_profile allocates an absolute data/profiles/<UUID> root, can inherit current data, and saves one row. `:590–617` get_launch_args adds savedatafolder for a nondefault root and extends argv with run_commands.

Do not infer migration semantics from the clone docstring alone: create_profile calls _clone_user_data when requested; this proposal intentionally seeds only the pack-style settings subset and **no saves** for new RimArcho clones. Adopting an existing RimCrow root naturally exposes its existing saves.

RimArcho today imports annotations/groups/rules, not GameProfile. DiscoverManagerImports (`app/managerdiscovery.go:22–66`) offers database candidates and backup lists (including bounded backups/profile enumeration), not each profile's live Config. Extend that flow; it is not already an environment importer.

### Mapping and import procedure

| RimCrow value | Proposed mapping / conflict rule |
| --- | --- |
| game_install_path | Normalize/canonicalize and deduplicate to one game row. Scan each new install once, share its inventory with all imported profiles. Prefer Version.txt over the profile's cached game_version |
| id, name | Allocate a RimArcho environment ID; retain (source database identity, profile ID) in an import mapping for repeatability. Normalize invalid CLI names and show collision resolution in preview; never replace an existing environment by name alone |
| user_data_path nonempty | Use the actual normalized root **in place**, commonly RimCrow/data/profiles/<id>; no copy. Do not assume every profile is at that convention when an explicit path exists |
| default / empty user_data_path | Resolve the selected game's normal default root, with per-game native/Proton rules. Empty game_install_path is **uninitialized** in the reference code; require a detected/chosen install, or mark skipped/unresolved. Never invent a runnable default from an empty path |
| prefer_steam_launch | steam when true, direct otherwise, validated against the actual platform/install. The reference normalizes runtime flags; report impossible/stale combinations, especially Proton |
| run_commands | Preserve argv tokens with round-trip parsing/serialization (§4), preview effective args; do not append a second savedatafolder that overrides the chosen root |
| <root>/Config/ModsConfig.xml | Create one uniquely named global plan per imported profile and set that environment's current_plan. Use existing registry/token resolution; retain missing packages. No write back to the source Config |
| use_workshop_mods / use_self_mods | Do **not** create per-environment visibility flags. Import actual enabled tokens and register necessary shared folders; report any unresolved copy-selection difference (§below) |
| inactive/temp order, description, last_played_time | No automatic fake launch/history. Report unsupported fields; optionally preserve description as import provenance if a supported field is added |

Data containment permits a normal external RimCrow profile root: `app/envs.go:100` checkRoots only tests overlap with **RimArcho's own App.Home**, and `:136` checkUserData permits RimArcho-owned userdata specially. RimCrow's data directory is not RimArcho's data directory merely because both are called data. A profile in the sibling manager's data/profiles is therefore allowed when the two applications' data homes are disjoint. If RimArcho's configured home contains that profile root, or vice versa, refuse; do not bypass the safety boundary for import.

**Spec/code discrepancy:** 03 §3.1 describes roots as mutually non-containing. checkRoots does not enforce arbitrary game-vs-userdata or userdata-vs-userdata overlap; folder checks enforce some additional source constraints separately. State the actual checks in the import preview, and add explicit validation for new environment roots (including nested userdata roots) rather than claiming today's code already provides it.

Source flags are mostly policy that RimArcho does not need: its library intentionally includes shared Workshop/extra folders, and enabled **plan entries** choose what loads. The reference's `backend/database/dao.py:265–276` uses flags to select candidate roots; local/DLC roots enter independently. Thus ignoring the booleans is reasonable **only after resolving the actual profile list/copies**. A RimCrow “self” folder may need registration as an extra source; same-package local/Workshop ambiguity, existing _Link_ links and About.xml.disabled need explicit reporting. A flag being false is not a reason to uninstall/hide a globally shared Mod.

Use the existing copy-DB/WAL/read-only pattern (`app/rimcrow.go:61` openCopy), with no writes to RimCrow DB/rules/profile metadata during import. Adopting the user root means later explicit launches **will** write that root's Config, settings and saves through the game. Explain this in confirmation: import itself is read-only with respect to the external root, continued use shares it with RimCrow. Warn that removing that profile through RimCrow can remove the adopted data; do not take ownership of its directory.

Preview groups profiles by game and marks existing pair, name/path conflict, missing game, unsupported Proton isolation, missing Config and unresolved tokens. Map an already known game/root pair to its existing environment by explicit choice; do not create duplicates or silently replace its current plan. Repeat imports should recognize profile mapping and preserve player edits; updating a mapped profile must be an explicit operation. Dry run may use disposable DB copies but must not persist environment, plan, source, mapping or profile-root writes.

## 8. UI names and settings

The existing glossary maps environment to UI “game” (`docs/glossary.md`, Environments row; 16 §7.2). `view/shell.go:197` labels the environment page “game”; `view/settings.go:180` titles EnvGroups “this game”. Locale examples include `locale/ja/settings.json:146` and `locale/zh/navigation.json:17`. Those labels cannot distinguish the two layers.

Proposed terms; the Chinese and Japanese are quoted in code spans and belong in the locale files.

| Concept | English UI | Simplified Chinese locale text | Japanese UI |
| --- | --- | --- | --- |
| Physical install | Game installation; “game folder” for its path | `游戏本体`; `游戏目录` | `ゲーム本体`; `ゲームフォルダー` |
| Play context | Environment | `环境` | `環境` |
| Selected context group | This environment | `当前环境` | `この環境` |
| Physical settings group | Game installation | `游戏本体` | `ゲーム本体` |
| Add sibling | Add environment | `添加环境` | `環境を追加` |

Keep “list”/plan and “pack” distinct; neither becomes a save owner. Use environment names in the chip/switcher and CLI/MCP env references; put install name/version/path in group headings or secondary text. For a single legacy environment, retain its familiar name and avoid forcing a new installation-naming wizard.

- Switcher: group environments by installation, select one environment; “add environment” can reuse that installation and pick own/custom userdata. Show effective data-root path in details, especially default vs isolated.
- Environment page/settings (16 §2.8, §2.14): **This environment** holds name, user-data root, current list, launch method/args and removal. **Game installation** holds game folder/version and official/local sources; moving it previews all affected environments.
- Shared Mod folders stay global. Library labels local-<game> name the physical installation, not whichever sibling is current; give game an initial stable display name from its migrated environment name.
- Home rows (view.AddOtherEnvs) currently repeat global plans for every environment. Do not multiply identical install inventory reads or present indistinguishable list rows. Show environment as the launch destination, optionally group repeated plans, and keep applied/open/current list state per selected environment.
- Settings “Folders”, install move/uninstall previews, first-run detection and remove dialogs must enumerate one physical install but all affected environments/roots. TUI E/game picker and command palette become environment pickers with install context.
- Update glossary, English humanization, zh/ja locale keys, empty states, help text, guide wording and machine output field descriptions. Keep stable protocol/JSON keys unless a specified versioned change is needed.

Pack visibility is a separate product choice: `PacksOf` currently includes a pack only when manifest env matches **environment name**, or when that environment name is unknown and game major.minor matches (`pack.go:655–683`). Same-game siblings will not automatically list each other's packs. Recommend keep env as origin metadata and list all **game-compatible** packs with origin shown; do not rewrite exported manifests. If origin-only listing is retained initially, state it explicitly. Packs retain their own userdata and settings even when launched from another sibling.

## 9. Specifications, phases and tests

### Contract updates for implementation

This research changes no specifications. The implementation must update the responsible sections, CHANGELOG's top unreleased entry and UserGuide/release-vm-tests.md together.

| Sections | Required change |
| --- | --- |
| 03 §1, §2 (especially items 1, 4, 5, 7, 8, 14), §3.1, §3.2, §3.3, §3.4, §4.1–§4.3, §5 | Game/environment distinction, uniqueness, initialization/removal/moves, detection and log-path evidence; preserve cross-game root sharing |
| 04 §1.1, §2.2–§2.4, §4, §5 | Game scan/history/question scope, migration/indexes, shared-event retention, acceptance |
| 05 §3, §4.2, §5.3, §5.4 | Game/set static slots versus environment ignores; assembly queries and decompilation install selection |
| 06 §4, §5, §6, §7 | Install evidence vs environment/run identity, root snapshots, cache context, live and companion-log isolation |
| 07 §1.1–§1.4, §1.7, §4.1–§4.3, §6.2, §7.1 | Global plans, current selection per environment, cloning/import, shared install projection and external edits |
| 09 §1–§4, §5, §8 | Shared physical deployment, two-step process identity, Proton and safe running-state handling |
| 10 §1, §3, §5, §6 | Manifest origin vs visibility/compatibility, settings seeding helper, shared links, validated log location |
| 11 §1, §3 | Credit deployment/root obligations and downloaded install registration once per game |
| 12 §1.2, §1.7, §4.2–§4.4 | Save roots/filtering/matching, environment selection on load, cache and removal behavior |
| 13 §2, §4.1–§4.3; 14 §1–§3 | References, CLI shape/help, env selector/tool descriptions and guide |
| 16 §2.1, §2.3, §2.8, §2.11, §2.14, §2.15, §2.16.1, §3.1, §7.2, §8.4, §9.4 | Naming/switching, create/import/remove forms, scheduled scan deduplication, async snapshot/context safety |
| 19 §2, §4, §6; 20 §2, §5 | Per-root settings records, shared installation Monitor inventory; isolated test roots vs physical journals |
| 02 §1–§2; 17 §3–§4; 18 | New writes/cleanup boundaries, VM acceptance; measure switch/scan/memory regressions rather than claiming performance |
| 00a §1 and §4.3; 00 §6 D4/D15/D16; docs/glossary.md | Domain ownership and rationale/naming. D15's plan save-sharing statement must refer to environment userdata; D16 still limits simultaneous runs |

### Phased delivery

Sizes are engineering estimates including focused synthetic tests and contract updates, not promises. Shared install correctness should ship before profile import is enabled.

| Phase | Scope / exit condition | Rough size |
| --- | --- | --- |
| 1. Separate identities without new siblings | Store/domain split, joined reads, inventory/analysis GameID selectors, deletion split, path/move/uninstall deduplication, migration compatibility. Existing users see same behavior | **Large, 4–7 working days; ~20–30 production files** |
| 2. Enable sibling environments safely | Add/validate/clone, same-game switch fast path, distinct current plans/ignores/settings; process/log ownership, root-aware credit obligations, test-journal recovery; CLI/MCP context tests | **Large, 5–8 days; ~20–30 production files**, overlapping phase 1 |
| 3. Player UI and RimCrow import | Names/switcher/settings/create/remove/import preview, per-profile plans, argv round-trip, idempotent mappings and compatible pack visibility decision | **Medium–large, 3–6 days; ~12–20 files plus locales** |
| 4. VM acceptance and performance | Real version/platform log-path matrix, launch A/B/packs/tests, Steam/default/manual launch, credit interruption/removal, switching while busy, upgrade backup/restore | **Medium, 2–4 days**, dependent on VM availability |

Overall roughly **2–5 engineering weeks**, depending on log behavior and handoff policy; file ranges overlap and must not be summed. No need for a new frontend framework, per-environment copies of inventories or an eager scan of every user-data root.

### Existing tests and extensions

| Existing evidence | What changes / new coverage |
| --- | --- |
| TestEnvModelSharedWorkshop (`envmodel_test.go:35`) | Its line-46 “second game directory refused” assertion becomes a same-root refusal; add same-game own/custom root success, one source pair/Instance identity, shared Workshop unchanged |
| TestEnvModelSaves (`envmodel_test.go:120`), TestLinuxLaunch (`linux_launch_test.go:16`) | Default pair conflict, own/custom creation, symlink/short-path/case aliases, containment, Proton registration and launch refusals |
| TestEnvUse* (`envlinks_test.go:33–260`) | Retain cross-game behavior and rollback; add same-game switch asserts no scan rows, new Instances, link writes or ModsConfig writes, including while sibling runs. Empty own saves must not inherit a list implicitly |
| TestPlansSharedAcrossGames (`planglobal_test.go:17`), TestV8SharedUserData (`external_test.go:175`) | Add sibling selections, shared vs cloned plan workspaces, global rules/annotations, per-env ignores; preserve cross-game shared-root external-change attribution |
| TestEnvRemoveCurrent (`envmodel_test.go:271`), TestEnvRemoveAndSourceRemove (`accept_test.go:670`) | Remove nonlast sibling preserves inventory/analysis/plans; last removal converts history references and preserves shared events; roots/files untouched, pending journals/credit handled |
| TestEnvSetGameMoveIsOneWrite (`directories_test.go:499`) | Move shared game atomically updates every sibling's joined view, one source pair, failure rollback; reattach one env to another install leaves siblings intact |
| TestWhatRuns (`gamerun_test.go:29`) | Two siblings with current one deliberately wrong; redirected/default/no args, ArgsKnown=false, unknown root, pack/test, stale launch fallback. Safety must still block unknown environment |
| TestForeignLogExcludedBeforeAnalysis (`logowner_test.go:36`), TestLogBelongsToLaunch (`loglaunch_test.go:88`), TestLogWriteRecordAndUnknown (`:165`) | Same executable root must not prove sibling ownership; same global plan/different root; captured old root after env set; current/previous selection and unknown source |
| TestLogCache (`logcache_test.go:70`), TestLogReportRebuiltAfterReanalysis (`logcachekey_test.go:15`) | Same log path/same plan across contexts cannot retain stale ownership/ignore counts; cached raw attribution may still be reused |
| TestSavesListGroupAndMatch (`saves_test.go:44`), TestSavesAgainstCurrentList (`:458`) | Both roots list once, --env filters one, launch matching selects correct sibling; cross-game shared root remains deduplicated; pack saves keep identity |
| TestCreditDeploymentsAcrossGamesResume (`credit_test.go:518`), TestCreditSessionsSurviveRemovingGame (`:682`), TestCreditCrossEnvironmentCandidateRequired (`:795`) | Two roots/one Mods path, pending resume after root edit/remove, conflicting link projections, no premature recycle; counted sessions retained |
| TestPackBuildAndLaunch (`pack_test.go:35`), TestMonitorInPack | Keep pack userdata and origin; verify visibility choice, settings seed from selected sibling, no extra inventory and no sibling log attribution |
| TestTestRunPlanConfigVisibilityAndReport (`gametest_test.go:98`), TestTestRunLinksBlockedAndLeftover (`:380`), TestTestRunComparisonBaselineSelection (`testrun_compare_test.go:149`) | Per-env settings and baselines, shared-game mutual exclusion/recovery, sibling interrupted journal, player root/log untouched |
| TestDiscoverManagerImports (`firstrun_test.go:16`), TestImportRimCrow (`m5_test.go:113`) | Synthetic gameprofile rows including empty default, aliases/conflicts/missing Config, two profiles/one game, custom roots with spaces, token/args import, source flags, dry-run and repeated imports preserve edits |
| TestIndexKey / TestOpenRestoresIndexTriggers (`store/indexkey_test.go:11,76`), TestUpgradeGameTypes (`gametypes_test.go:130`), TestPreReleaseDatabaseRefused (`plan_test.go:61`) | Fresh vs upgraded schema equivalence, populated migration all scopes, FK/index/trigger checks, failure injection, zero blob/ID/root changes, old/new schema refusal |
| TestEnvParameter (`surface/mcp/mcp_test.go:407`), CLI globalFlags tests | --env/env resolves sibling context without persisting selection; same command/tool output; setup remains player-only unless explicitly expanded |

Add GUI worker stale-switch/snapshot and TUI selection tests with two environments sharing GameID; inventory reuse must never retain the wrong Environment pointer. Assert one scan/analysis for the same game/set and unchanged switch-time filesystem state using synthetic fixtures. Extend VM rows for each new control and changed behavior.

No tests ran for this research-only file. Implementation validation should run targeted store/app/view/surface tests first, then the required `go test -count=1 ./...` and Node suites under AGENTS.md, using only internal/testkit data. Actual game/log-path acceptance belongs solely in the VM.

## 10. Owner decisions and remaining risks

### Decided 2026-11-11

One rule decides each question: to the player, **an environment is a separate copy of the game**. What they see or change in one environment applies to that environment only; where sharing would make "I changed it in A, why did B change?" possible, the two are kept apart. The decisions are now in 03 §6 (not implemented).

| # | Decision |
| --- | --- |
| 1 | Finding ignores are per environment |
| 2 | Adding an environment offers "copy this environment" (the default: Config settings, HugsLib and the current list **as the editor shows it**, unsaved edits included; no saves, logs or history) or "empty" (game defaults, Core and DLC). The dialog lists what is and is not copied. The copied list is a **new** list, "<environment>'s list"; the player may later choose to share a list between environments |
| 3 | Each environment's log page shows only its own runs. If the VM shows that environments of one game write one Player.log, new environments get their own `-logfile`; existing environments keep their log where it is |
| 4 | RimCrow profiles are adopted in place (no copy); the preview says the folder is shared with RimCrow and that removing the profile there removes its saves |
| 5 | The game installation is named automatically ("RimWorld 1.6", plus the folder when versions repeat). Moving it lists every environment that follows; pointing one environment at another installation changes only that environment |
| 6 | Packs show in every environment of a compatible game version, labelled with the environment they were built in; `pack.json` `env` stays origin metadata |
| 7 | Proton: "add environment" for that game is disabled with the reason |
| 8 | Launch records keep the environment name of the time; a SteamCMD handoff recycles the old copy only after every environment that uses it has moved |
| 9 | Legacy path aliases: environments that turn out to share one physical installation stay as they were after the upgrade; RimArcho offers to merge them, never merges silently |
| 10 | Switcher grouped by installation; settings split into "this environment" (user data, list, launch method and arguments) and "game installation" (folder, version) |

2026-11-12 owner, decision 2: no list copy; the config selects a list.

### Questions as researched

1. **Finding ignores:** this note recommends keeping them per environment. Sharing by game is possible, but changes acceptance behavior across play contexts; choose deliberately.
2. **Initialization:** approve explicit --like settings + new-plan cloning, with bare own mode empty, or choose an alternative default. Define whether a clone includes unsaved workspace edits; the recommendation is yes, without history/staging.
3. **Game naming and moving:** choose a stable installation label and distinguish relocation from reassigning one environment. Preserve old local-<name> filters or provide an explicit compatibility alias.
4. **Log location:** resolve the documented 1.6/1.7 disagreement in VM before promising isolated logs. Decide whether new environments get explicit logfile flags and how ordinary Steam launches remain attributable.
5. **Historical and pending roots:** approve captured launch/credit roots and removal behavior. Shared-install credit projections can conflict physically even though Config files are isolated; retaining the old copy is preferable to losing a still-needed Mod.
6. **Path aliases and duplicates:** settle legacy alias repair and canonical key policy. Do not let migration silently merge two old inventories or create new paths to satisfy a constraint.
7. **RimCrow root adoption:** make shared ownership consequences explicit. Default rows with empty paths need a choice; duplicate profiles mapping to the same pair cannot both become environments. Import should not claim perfect copy selection merely because packageIds match.
8. **Pack listing:** game-compatible visibility with origin shown is recommended; manifest env remains origin metadata for compatibility.
9. **Proton:** keep a clear unsupported-isolation refusal until path translation/launch support is implemented; it cannot be fixed by the database split.
10. **Performance and concurrency:** share inventory/analysis, not mutable environment context; deduplicate scheduled work and physical directory walks. Preserve cross-process launch/handoff locks and GUI stale-result guards. A fast sibling switch requires tests for *absence* of scan/link work, not just a quick page render.

The structural split is justified and the same-ID migration avoids expensive inventory rewrites. The hard boundaries are root identity, physical shared deployment, and run attribution. Keeping those boundaries explicit lets existing users migrate unchanged and lets RimCrow profiles become ordinary environments instead of a special launch-only feature.
