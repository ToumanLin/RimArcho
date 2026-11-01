# 03 · Facts and Analysis

> Status: implemented (declaration, `Def`, Patch, Harmony layers, log attribution, live tracking), log ownership by launch records (§6.4), multiple cached analyses per plan (§3).
> Basis: 00, 00a, 02, 04, 07 §6, 07 §7, 08 §4

This document explains how RimArcho collects facts from Mod files/game logs and derives findings. Identity/scanning: 02; plans/sorting: 04; commands/MCP tools: 05; interface presentation: 06 (can it run page wording in 06 §5). Performance targets/measurements live in 07 §6 and are only referenced here.

"Analyzed plan": analysis always uses one plan's (04 §1) enabled list and order. In the interface, the open plan; CLI/MCP default to the environment's current plan.

## 1. Effective Content

The game does not load every file in a Mod directory. All content analysis (`Def`, Patch, assemblies, log reference lookup) uses only effective content, following `ModContentPack.InitLoadFolders` and `DirectXmlLoader.XmlAssetsInModFolder` (07 §7).

**Version directory selection**. Given game major/minor `V` (e.g. `1.6`):

- **With `LoadFolders.xml`**: exact `V` tag first; otherwise highest tag not above `V`; otherwise `<default>`; otherwise lowest tag. Within selected tag:
  - All unconditional `<li>` apply;
  - Evaluate `IfModActive`, `IfModNotActive`, `IfModActiveAll` against analyzed plan; legacy `IfDLCActive` / `IfDLCNotActive` mean `IfModActive` / `IfModNotActive`. Strip `_steam` from conditional packageIds before comparison.
- **Without `LoadFolders.xml`**: version directory (exact, otherwise highest not above `V`, otherwise lowest), `Common`, root, in descending priority. Version directory names must match `^[vV]?\d+\.\d+$`. For identical relative resource paths, use only highest-priority copy.
- Compare numeric tuples: `1.10 > 1.6`.

**Encoding**. Detect UTF-16 LE/BE and UTF-8 BOM for About.xml, `LoadFolders.xml`, `Defs`/`Patches`; convert to UTF-8 before parsing. Without BOM but XML `encoding="utf-16"`, also read UTF-16 (`internal/textenc`, shared by scan, xdoc, content).

**`Conditional` directories**. Record named condition packages for each loaded conditional directory (`Resolved.Requires`). `Patches` there are guarded; those packages are expected targets (§4.4).

**Cache key**. `Conditional` `<li>` makes effective content plan-dependent, so its key is:

`snapshot hash | hash of ordered effective directory list | analyzer version`

Preserve directory priority order and duplicates; clean paths/normalize separators first (`content.FolderKey`); changing order can change same-name resource overrides. Game version and condition evaluation already affect directory selection; identical directories do not need extra sets for different versions/condition text. Key excludes whole plan; plan edits invalidate only `Mods` whose effective directories change (`TestLoadFoldersConditions`).

`Assemblies` use `analyze.AssemblyKey`: snapshot hash, relative effective DLL paths, each DLL's Harmony-skip flag, analyzer version. Different XML directories can share DLL facts if effective DLLs and skip policy match.

## 2. Fact Types

| Fact | Fields | Source |
| --- | --- | --- |
| `ModDecl` | Dependencies (including versioned), loadBefore/After, forceLoad*, incompatibleWith, `supportedVersions`, original `LoadFolders.xml` | Snapshot `decl_blob` (02 §4.3) |
| `Def` | Type (element name), `defName`, `Name`, `ParentName`, `Abstract`, file relative to Mod root, line (opening tag), owning Instance, `Def`'s own `MayRequire` / `MayRequireAnyOf` | Effective `Defs/**/*.xml` |
| `PatchOp` | Operation class, XPath, child-operation structure, value summary, file/line, ordinal, simulation result/reason | Effective `Patches/**/*.xml` |
| `TypeDef` | Assembly, namespace, full type name | Effective DLL .NET metadata |
| `HarmonyPatch` | Target type/method/parameters, `MethodType`, patch kind (prefix / postfix / transpiler / finalizer), patch method, possible original-method skip, dynamic target flag | Same as above |
| `HarmonyId` | Suspected Harmony ID strings (heuristic) | Same as above |
| `Rule` | Subject, relation, object, source (user / community / replacement), scope | 04 §6 |

- Fact sets associate each fact with Instance, snapshot, analyzer version (`analyze.Version`); reads use §3 version views for current `Store.MM`. Rebuild facts when analyzer version differs.
- Game loads `Defs` with `MayRequire` only when required `Mods` are enabled; analysis follows this (`DefFact.Loaded`).
- `archo def` lists definitions' `file:line` in load order; last marked `← effective`.

## 3. Execution and Storage

**Layers**. Declaration (milliseconds, computed on demand) → `Def` → Patch (depends on `Def`); Harmony independent. Higher-layer failures do not affect lower-layer results; Harmony failure affects only its facts.

**On-demand declarations**. No subprocess or persistence. Cache results with plan reads in-process, keyed by `Store.ChangeKey()` (`PRAGMA data_version` + `total_changes()`), environment ID, ModsConfig.xml size/mtime; frequent UI refreshes/details reuse it.

**GUI list snapshots**. Editor and can it run share in-memory finding snapshots. `List` edits synchronously update declarations, retain previous content results marked updating, and complete logs/analysis in background; keys, invalidation, events in [06 §8.7](06-ui/8-gui-technology.md#87-%E5%88%97%E8%A1%A8%E9%97%AE%E9%A2%98%E4%B8%8E-needs-%E7%BC%93%E5%AD%98-api).

**On demand**. Check/rebuild stale parts only when can it run opens, editor details need findings, or CLI/MCP request content information. problems, def, patches, harmony rebuild stale analysis through subprocesses; mod, status, why stay fast using stored results only (`--no-analyze` also prevents problems rebuilding).

**Subprocess**. `Def`, Patch, Harmony analysis runs in `archo analyze`: caller (`App.EnsureAnalysis`) starts `<exe> analyze --env <name>` when stored results are stale, passing `RIMARCHO_HOME` and `--game-version`. Subprocess writes facts and exits without affecting main-process memory.

- `progress: …` lines go to stderr; UI status/progress bar, CLI text.
- Soft limit `debug.SetMemoryLimit(300 MiB)` (`AnalyzeMemoryLimit`); peak target in 07 §6.
- Tests inject in-process runner (`InProcessAnalyze`).
- **Wait limit**: per-entry default `App.AnalyzeWait` in 06 §2.16.1 W01; CLI unlimited. On timeout return declaration findings with "still computing in the background"; subprocess continues; next call gets results. Meanwhile def / patches / harmony use previous stale results.

**AnalysisKey (freshness)**. Hash analyzer version, game major/minor, each included Instance ID, snapshot hash, position in plan (first 24 hexadecimal characters). User/community rules excluded because they affect only on-demand declarations. Cheap to compute without loading declarations.

**Included Instances**. Plan enabled list plus all other live, loadable disabled Instances; loadable includes game-visible `Mods` and SteamCMD / other-folder `Mods` deployable through links on save (since 2026-10-28; previously disabled copies had no `Def` facts and missing-parent providers required per-file fallback reads). Disabled Instances yield only `Def` facts for `target-inactive` (§4.4) and missing-`Def` provider lookup in logs. Harmony reads enabled Instances only.

**`--game-version X.Y`**. Accepted by analyze, problems, def, patches, harmony, why, log (validate `X.Y`, restore after command), overriding environment version; MCP tools expose `game_version`. Use when game is 1.7 but list/logs remain `1.6`.

**Storage** (`store.SaveAnalysis`, `store.saveContent`, `store.PruneFacts`).

- Analysis slot: `(environment, game_mm, enabled-set fingerprint)` (`analysis.set_key`), latest run per slot. Runs, patch facts, findings remain isolated by environment/version; AnalysisKey detects order changes. Plans sharing enabled set share a slot. Saving analysis removes enabled-set slots unused by any plan across all this environment's game versions.
- `fact_set(id, instance_id, kind, content_key, files)` unique key `(Instance, fact kind, content key)`; `kind=def` stores `Defs`, `kind=dll` stores Assembly / Type / Harmony / Harmony ID. Fact rows carry only `set_id`; set determines Instance, whose ID format is unchanged.
- **Set file lists** (v28, `store/setfiles.go`): `fact_set.files` is a JSON array of used Mod-relative paths, in first-use row order; `def_fact`, `type_fact`, `harmony_fact`, `harmony_id`, `assembly_fact` store zero-based `file_ix` instead of paths. Build list once per set on save (`saveContent`, `saveHarmony`); immutable after set write, deleted with set. Readers (`DefsOf`, `DefsNamed`, `HarmonyPatches`, `Assemblies`, `TypeOwners`) read list once per set after rows, mapping indexes to paths; `Store` caches by `set_id`, clearing on `ChangeKey` because deleted IDs may be reused; randomly discard half above 65,536 paths. `CachedDefs` (all disabled Mod `Defs` per analysis) reads one set/list together, bypassing cache to avoid evicting others. Migration/result equivalence: 02 §7 v28.
- `content_state` retains `(instance_id, game_mm)` primary key; `def_set_id` / `dll_set_id` point to sets, retaining effective directories/parse summaries. Every fact read joins this version view by `Store.MM`; identical content for same Instance across environments/game versions shares sets. Disabled Instances build `Def` only; retain stored DLL view only if content unchanged; enabling writes an empty set even with no effective DLL to clear old views.
- Index sets by `set_id`; global name queries start at name index then join version view. `def_fact_named(name, set_id) WHERE name<>''` preserves fast parent `Name` log lookup; Type / Harmony ID also look up sets by name before per-Mod scans. DDL: `store.schemaV26`; five `file_ix` fact tables: `store.schemaV28`.
- **`Def` lookup order by name** (`Store.DefsNamed`, used by def, log attribution, missing-parent fallback): Instance ID first, file order within Mod. Not table row order: cache rebuild/migration changes rows, changing first missing-parent provider and log finding references (2026-10-29 VM: analyzer 7→8 rebuild changed `log:E6` first provider and `p:`). Sorting does not change join order: name index first, then version view (`CROSS JOIN`, like `TypeOwners`); ordinary `JOIN` lets SQLite avoid sort by scanning whole version `content_state` and querying name index per row, dozens of times slower, worsening with Mod count (07 §6.2; `TestDefsNamedPlanStartsFromName` checks plan).
- **Plan limitation remains**: version views are not plan views. Last analyzed directories for `(instance, mm)` replace its view; if two plans select opposite LoadFolders conditions/different folders, returning to a plan with a fresh slot may still show the other's content in def. Directory-based shared sets do not change this existing limitation.
- **Slot retention**: slots matching environment's current major/minor do not expire by age; others expire after `store.ForeignAnalysisRetention` since last completed analysis, deleting run patches/findings too. Default in 06 §2.16.1 A11. Unknown game version suspends age expiry.
- **Version view cleanup**: delete `(instance, mm)` if Instance no longer exists, or no visible environment currently uses mm or has an mm analysis slot. Visibility per 02 §7, no Mod file access; conservatively retain for visible environments with unknown version. Shared Instances check all environments, private ones only owner. Same rule after environment/slot deletion.
- **Set cleanup**: immediately delete sets without `content_state` references; cascade fact rows. `SaveAnalysis`, `env remove`, gc, link identity merges clean within their transactions; independent `Store.PruneFacts(now time.Time) error` is repeatable. Cleanup changes only fact cache/analysis slots; maintenance shrinks physical database.
- **Migration**: v26 converts old content-key snapshot/analyzer versions and stored directories into `Def` sets, without rereading Mod files. Old DLL cache lacked skip policy; conservatively share by snapshot, analyzer version, fingerprint of all DLL facts; next analysis replaces with normal input key. Retain unrecognized old keys per version, without guessing sharing. Do not migrate facts without old version-view references; transaction failure rolls back all. No automatic `VACUUM`.
- Decompilation `source_cache` (§5.4) and log attribution `log_cache` (§6.5) below.

## 4. Findings

### 4.1 Structure

| Field | Description |
| --- | --- |
| `code` | Finding code, §4.3 |
| `severity` | error / warning / info |
| `subjects` | Entity references; first is main subject (Instance, `pkg:<id>`, or `log:E<n>`) |
| `claim` | One-sentence conclusion |
| `evidence` | `List` of "reference + explanation + source type": declared (Mod declaration) / inferred / log / community / user (player rule) |
| `confidence` | certain / likely / possible |
| `suggestions` | Executable commands such as `archo enable #b21c`. `Prefix` non-archo suggestions: `subscribe: <Workshop-link>`, `steam://url/CommunityFilePage/<id>`, displayed as links |
| `key` | Identifies finding across runs within code (references/ignores) |
| `layer` | decl / def / patch / harmony / log |

UI evidence chips are evidence source types (06 §5).

**Principle: appearance is not causation.** A Mod patch appearing in a stack is only related (patcher), not a cause without its own frames/files as evidence (§6.3).

### 4.2 Ignores and References

- **Finding reference** `p:<6-hex-digits>` = first 3 bytes of sha1(`code | key | subject Lineage`), stable across runs. Log keys lowercase absolute paths reduced to final three segments, preserving references/ignores after Mod library moves or in tests.
- **Ignore** records `(Lineage, code, key)` on Lineage per environment, with subject snapshot at ignore time. Snapshot changes (Mod update, visible in mod library › changes) expire ignore; finding returns with `[ignore expired: the mod changed]`.
- Commands: `problems ignore|unignore <p:ref>`, `problems --ignored`, `problems show <p:ref>`; default list ends with `ignored: N (archo problems --ignored)`. UI keys: 06.

**Mute/restore version reminders** is independent of snapshot ignores (`app.SetVersionMute`, `store.VersionMutes`). Key: normalized `packageId` (trim, lowercase, strip game's `_steam`) + game major/minor; shared by all plans/environments/same-package copies, surviving Mod updates, snapshot changes, Lineage merges/splits; another major/minor reminds again. Changed package ID means another Mod. Schema v23 adds `version_mute(package_id, game_mm)` composite primary key, not environment/snapshot/Instance-bound; migration leaves old `finding_ignore` unchanged.

- `ProblemSet.Muted` retains muted current-plan `version.unsupported` outside `List`; `All()`/individual details still expose them. Default CLI/MCP/summaries report muted count (CLI `ignored-version-warnings`); `--all`, subject/code filters do not automatically show them; explicit `--include-muted` / `include_muted=true` includes marked items. Commands/tools: 05 §4.2, §5.2.
- Exclude them from finding counts, Mod row icons, pre-launch prompts, pack build `Mismatch` warnings; allowed-version undeclared counts also exclude them, recording muted count separately (04 §`1.6`). Declaration facts, allowed-version intersection/default, pack version intersection, pack/game major/minor restrictions, file integrity checks are unaffected.
- Restore individual Mod mute directly without confirmation; batch mute of summaries/manifest groups requires confirmation. UI entry points/individual wording: 06 §5.

### 4.3 Finding Catalog

**Declaration layer** (computed on demand)

Link findings query `InspectGameLinks` each time, independent of declaration/`Def`/Patch caches; player-directory targets are not inherently foreign. Inspect disk: first level of each `Mods/` and link targets; Instance names/registered real paths are cached with Mod index using `Store.IndexKey` (07 §8.3). Compare link target with scanned Instance real path and confirm Instance still resolves there; only on mismatch re-resolve every Instance path.

| `code` | Severity | Detection and suggestions |
| --- | --- | --- |
| `entry.missing` | warning | Plan entry matches no installed Mod (retained in ModsConfig.xml). Subject `pkg:<id>`. Suggest `archo find <id>`, `archo disable pkg:<id>`; add `subscribe: <link>` if community data knows Workshop item |
| `mods.link-dangling` | error | Owned-prefix link at first level of known game's `Mods/`, target confirmed missing and source accessible; subject `path:<full-path>`, with `game:<game-path>`, no Instance required. Block selected game's launch/test run; summarize others only. Repair: 08 §3 |
| `mods.link-foreign` | warning | Target mismatches known Instance/pack identity or cannot be mapped; show original target/ownership evidence. Retain valid env links; prefix alone never authorizes deletion |
| `mods.link-check-failed` | error | Access denied, offline disk, unknown reparse tag, etc. prevent judgment; report failure/retain; complete inspection before selected-game launch |
| `mods.link-occupied` | warning | Real directory/file occupies owned-prefix name; report only, no deletion action |
| `mods.link-recovery` | error | Persistent ledger has unfinished operation; show actual state/recovery action read-only, complete after confirmed repair, no automatic startup disk changes |
| `mod.unloadable` | error / info | **error**: enabled Instance game will not load (unloadable, residue, displaced by same-package copy); suggest disable. **info**: installed, disabled, unloadable for reasons other than about-disabled (02 §4.2) |
| `mod.pinned` | info / warning | Locked version (always "locked version" in player text, retain code name). Current implementation: Workshop `About.xml.disabled` with another live, loadable non-Workshop Instance in same Lineage (RimCrow `_<Workshop-ID>_`, `copy-local`, `Mods/` deployed copies count). **info**: copy effective, Workshop upstream; suggest `archo unpin`. **warning "upstream has an update"**: Workshop semantic hash differs from copy, ACF timeupdated later than copy first seen; suggest `archo mod <copy> --diff <Workshop>`, `archo unpin`. RimArcho-made locked versions with unhidden Workshop original (lock library, 04 §7): warn when upstream snapshot differs from `origin.json` `origin_snapshot`, suggest `archo pin --update <copy>`. No `mod.unloadable` for locked versions |
| `mod.hidden-residue` | warning | Workshop source, renamed About.xml, absent from ACF subscriptions: unsubscribed residue. Suggest `archo trash` |
| `mod.hidden` | warning | Other rename-hidden `Mods` (usually another manager). Suggest `archo unhide`, `archo trash`, or ignore to keep hidden. Recompute classification per analysis without persistence; actions: 08 §6 |
| `mod.replaced` | warning | Community Use This Instead (`replacements.json`, `archo data update`, 04 §6) knows maintained replacement for current game version. If installed, suggest disable old/enable new; otherwise Workshop link |
| `dep.missing` | error | Required package (including current-version `modDependenciesByVersion`) absent from environment. Separate wording for DLC. Suggest `subscribe: <steamWorkshopUrl>` and `steam://url/CommunityFilePage/<id>` |
| `dep.inactive` | error | Dependency installed but disabled. Suggest `archo enable` per candidate |
| `order.violated` | warning / info | Order violates a noncyclic constraint (loadBefore/After, dependency, community/user). One finding per edge, key `<before-package>><after-package>`; declared/community/user evidence by kind. Severity follows strength (04 §5.3): dependency, loadBefore, forceLoad*, user (strong) warning; loadAfter, community (weak), corpus-harmless violations info. Suggest `archo sort --dry-run`, `archo move <A> --after <B>`, `archo rule add ignore <ref>` |
| `official.order` | warning | Core/DLC not first or out of release order. Combine all built-in official-order violations. `Common` corpus case: DLC enabled after list creation, appended last (Odyssey too late). First suggest minimal `archo move` (new DLC after previous official package, 04 §5.3), then `archo sort --dry-run` |
| `order.cycle` | info | Constraint cycle, listing each edge/source. Game warns without sorting, so cycles do not block write/launch. Break suggestions/ranking: 04 §5. Info basis: §4.3.1; can it run still lists it (muted, uncounted), because only player decides which rule to suppress |
| `incompat.active` | error | Two declared-incompatible `Mods` enabled. Suggest disabling one, `archo why A B` |
| `version.unsupported` | warning / info | `supportedVersions` omits current major/minor (exclude official/no-field `Mods`). Previous minor (game 1.7, Mod through `1.6`) warning; only older versions info (corpus, §4.3.1; opposite old rule) |
| `pkg.duplicate-active` | error | Multiple Instances with same `packageId` enabled. Subjects all enabled copies; suggest retaining first only |
| `pkg.duplicate-installed` | info | Multiple Instances with same `packageId`, at most one enabled; evidence marks game-ignored copy. Suggest `archo swap` |

> Declaration violations are common and mostly harmless: half of corpus logs violate dependency/order declarations, 45% community rules; above 100 `Mods`, error frequency is equal with/without violations; most-violated constraints rarely involve error-attributed `Mods`, some (modoptionssort before Core) are impossible. Distinguish errors from non-enforced declarations by constraint-kind severity (04 §5.3). Numbers: 07 §6.

Old `replacement.available` no longer emitted; replaced by `mod.replaced`.

**Log corpus layer** (part of declarations, computed for current enabled list; `internal/known/known.json`, generated by `spikes/knowndeps` from 40,326 player-shared `1.6` logs, packageIds/counts/error templates only, at least 5 distinct players per entry; community evidence. Embedded with program: `archo data update` fetches third-party data only; RimArcho has no own data repository, like `internal/order/observed.json`)

| `code` | Severity | Detection |
| --- | --- | --- |
| `dep.observed-missing` | warning / info | A enabled, B disabled, A About.xml does not declare B; at least 25% of such corpus logs report missing type/parent `Def`/`Def`/texture supplied by B. Suggest enabling installed B or Workshop link. Warning if at least half and not merely missing sound/texture, otherwise info. Evidence counts, missing items, typical error, whether next upload by same player loses error (sessions after enabling B) |
| `combo.known-bad` | warning / info | A-attributed errors mainly with B enabled (≥ 30% together, less than one-quarter with A alone), or mainly one A version (About.xml `modVersion` / `About/Manifest.xml` version, exactly what HugsLib records). Warning `at `≥ 50% and not missing sound/texture, otherwise info |
| `mod.known-error` | info | Same error in at least half of shared logs enabling A, independent of rest of list: known error in Mod itself |

Severity basis: with declared dependency disabled (`dep.inactive`), subject Mod is an error source 18% vs 7.5% chance (decisions-corpus §3.7); this layer's entries have errors in at least 25–30% of corresponding logs by construction, but observations are not declarations and authors may intend optionality, so maximum warning.

**`Def` layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `def.override` | info | Multiple `Mods` define same type/`defName`; later load wins, identified. Skip abstract `Defs`, same-Mod duplicates, unmet `MayRequire` |
| `def.parent-missing` | error | `ParentName` has no matching `Name`. Check patched documents **after** Patch simulation (§5.1). Executed custom classes (`patch.unknown-op`) count as success; their XML text values count as added `Name`, suppressing false reports (Chimera Creation (Continued) adds Anomaly `Name` via custom class). confidence likely |
| `def.abstract-collision` | warning | Two or more `Mods` define abstract parents with same `Name` |

**Missing parent template help** (`app.ParentHelp` / `FindParentHelp`). `def.parent-missing` prevents affected content loading; aggregate types/counts by subject Mod. Query stored `Name` index (`Store.DefsNamed`) first for providers: offer enable commands for installed, loadable disabled definers when analysis/content key are fresh, enabling does not change conditional directories, and `Def` conditions are met.

Only when players open details or CLI/MCP request one finding, search effective `Defs` of installed/loadable/disabled `Mods` for `Names` without cached providers (`modsearch.FindNames`); never during page load. Fresh facts index every loadable Mod's `Name` (including link-deployed `Mods`); fallback reads only `Mods` whose facts may differ after enablement: stale content key or changed conditional directories. Without such `Mods`, read no files and conclude no installed Mod (including disabled) provides it. Apply current game version, LoadFolders, `MayRequire` after enabling that Mod; match top-level `Def` `Name` only, not `defName` or nested properties; no network or analysis-cache writes. Search has deadline and limits on `Mods`, directory entries, files, single-file/total bytes (implementation), supports caller cancellation; limits/unreadable files/invalid XML retain results and state incomplete search, never equate not found with absent. GUI uses independent read session without navigation/edit/log read locks; cancellation: 06 §5.

Uninstalled candidates come from subject dependencies (including current version), loadAfter / forceLoadAfter (including current version), cached `Def` `MayRequire` / `MayRequireAnyOf`. If declared URL or existing community Steam DB package index identifies Workshop ID, offer subscribe/page; otherwise Workshop search; candidates do not claim confirmed template provision. Omit candidates if every missing `Name` already has an installed provider. `Always` offer subject Workshop requirements (name search without Workshop ID). Only when existing, attributed, current-list-matching logs contain no error/warning involving that Mod, explain potentially harmless optional content and offer group ignore; use §4.2's per-finding update expiry, no new ignore format. UI/command output: 06 §5, 05 §3.2.

**Patch layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `patch.no-match` | warning | `Top`-level simulation failure (the layer game logs); one per failed top-level operation, with reason (§4.4) |
| `patch.contested` | info | Two or more `Mods` `Replace`/`Remove` same node. Node key = `Def` + element path |
| `patch.unknown-op` | info | Unsimulatable custom PatchOperation; one per Mod listing custom classes; treat operations as successful |

**Harmony layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `harmony.shared-target` | info | Two or more `Mods` patch same method; one per method |
| `harmony.skip-prefix` | info, may become warning | A bool-returning prefix may skip original method, also patched by other `Mods`. confidence possible. Escalation: §4.5 |
| `harmony.multi-transpiler` | info, may become warning | Two or more `Mods`' transpilers on same method. Severity basis §4.3.1; escalation same as skip-prefix (§4.5) |
| `harmony.manual` | info | Assembly calls `Harmony.Patch`; statically resolved IL targets become normal patches (§5.3); explains resolved calls and unresolved reasons, never misreports no patches |

**Log layer**: one `log.<category>` per error cluster (categories §6.1).

| `code` | Severity |
| --- | --- |
| `log.exception`, `log.harmony-stack`, `log.xml-load` | error |
| `log.patch-failed`, `log.xref-missing`, `log.material`, `log.texture-missing`, `log.save-load` | warning |
| `log.mod-report`, `log.other` | info, excluded from finding list |

#### 4.3.1 Severity Basis (Log Corpus, G4/G8)

Severity measures how strongly a finding predicts game log errors, tested with 6,734 public RimWorld `1.6` logs (docs/research/decisions-corpus.md §3, §2 G8). For each finding, compare whether involved `Mods` are attributed as error sources in the same log against randomly choosing as many `Mods` (observed / chance).

Rules:

- **error**: game will definitely have a problem (unmet declared dependencies, declared incompatibility, missing parent `Def`), observed clearly above chance (about 2× or more). Errors block pack building (08 §5.2), so exclude merely possible problems.
- **warning**: observed about 2× or more, actionable for players.
- **info**: near/below chance, or numerous normal coexistence patterns (overrides, contention) without individual action value.

| `code` | Severity | Observed / chance | Reason |
| --- | --- | --- | --- |
| `patch.no-match` | warning | 37.3% / 4.5% (8.3×); `patch-failed` errors 29.7% / 0.9% | Strongest error predictor. Not error: 70% have no matching logged patch failure (e.g. optional targets); escalation would prevent half of lists building packs |
| `def.parent-missing` | error | 16.3% / 4.1% (3.9×) | Retain |
| `incompat.active` | error | 26.4% / 7.3% (3.6×) | Retain |
| `def.abstract-collision` | warning | 25.4% / 8.3% (3.0×); 3.2× excluding Harmony stack errors | Retain |
| `harmony.multi-transpiler` | info (formerly warning), warning when in log stack | Before G9 20.3% / 7.0% (2.9×, full); same 669 logs after G9 20.7% / 8.9% (2.3×); 7,291 G9 additions 2.2×; excluding Harmony stack errors 2.3× | Resolving manual patches increased 4,559 to 11,078 (17/log), dropping ratio below same-layer info `harmony.shared-target` (2.6×), `harmony.skip-prefix` (2.5×); patched-method error-stack rate (0.5%) matches shared targets (0.6%), indicating patch-heavy `Mods` rather than transpiler conflicts. Lower to info, exclude pre-launch prompts; promote like skip-prefix when patched method occurs in current error cluster (§4.5) |
| `dep.inactive` | error | 18.1% / 7.5% (2.4×) | Retain |
| `dep.missing` | error | 9.9% / 4.4% (2.3×, 242 genuinely missing player dependencies) | Retain. Another 10,941 entries reflect missing research-library dependencies players actually had (`1.6`% / 3.3%), an artifact unsuitable for severity; below-chance ratio confirms unrelated findings are `not counted` related |
| `version.unsupported` (previous version only) | warning (formerly info) | 10.4% / 4.0% (2.6×) | Code compiled for previous game version may fail after API changes |
| `version.unsupported` (older versions only) | info (formerly warning) | 0.8% / 3.2% (0.3×) | Old `Mods` still used survived several versions; broken ones were abandoned |
| `official.order` | warning (formerly error) | 7.4% / 3.6% (2.1×) when `Mods` involved; paired comparison in 04 §5.3 1.8× | Similar to dependency-order violations (1.4×, warning), not certain game errors. Overall 2.3% / 2.5% unusable: half subjects are DLC, and Core/DLC never attributed as sources |
| `order.cycle` | info (formerly warning) | 6.8% / 6.1% (1.1×) | Cyclic lists run normally, error rate matches chance; drop weakest cyclic constraint during sort (04 §5.2) |
| `patch.contested` | info | 41.9% / 6.6% (6.3×); without subject `patch.no-match`, 21.3% / 6.6% (3.2×) | Retain info. High ratio mainly same `Mods`' no-match findings already reported; remaining 3.2× resembles `def.override` (3.0×, info): shared edits are normal coexistence, about 70,000 entries, 18/log, individually unactionable |
| `def.override` | info | 24.6% / 8.1% (3.0×) | Retain, same reason (700,000 entries) |
| `harmony.shared-target`, `harmony.skip-prefix` | info | 2.7×, 2.6× | Retain: most numerous (1.39 million, 870,000), individually unactionable; log-backed skip-prefix promotes to warning (§4.5) |
| `harmony.manual`, `patch.unknown-op` | info | `1.6`×, 2.5× | Retain: describe incomplete analysis, not problems themselves |

Pre-launch findings (`LaunchChecks`, 08 §2) are selected by this observed/chance table, not severity.

### 4.4 `patch.no-match` Reasons

`Classify` at innermost failed operation; multiple reasons allowed.

| Reason | Detection |
| --- | --- |
| `target-inactive` | Target `Def` defined by installed disabled Mod (query all installed `Def` facts) |
| `target-missing` | No installed Mod defines target |
| `target-later` | XPath matches after all patches: target created by a later Mod's **patch**. Creator identified by node allocation ranges. Game merges all `Defs` before patches, so definition by later Mod alone never causes failure |
| `node-absent` | `Def` exists, remaining XPath does not |
| `wrong-version-route` | Target on disk outside effective content: search inactive XML in patching Mod and expected targets |
| `stale-name` | Expected target Mod enabled, lacks `defName`; list up to 3 similar names by edit distance |
| `unguarded-optional` | Only with `target-inactive`: no guard and definer not hard dependency |
| `exception` | Game would throw, e.g. tree operations on attribute nodes |
| `unclassified` | No `Def` name in XPath |

**Guards**: `PatchOperationFindMod` match branch, `Conditional`, `Sequence` children with `MayRequire`, conditional loading folders (§1). Ignore top-level `MayRequire` like game, explaining in details.

**Expected targets**: `FindMod` names (display names, DLC directory names), `MayRequire` packages, conditional-folder packages, hard dependencies other than Harmony/Ludeon.

### 4.5 Default Display

Entry points show different defaults from the same finding set. Usually `--all` or filters show full list; muted version reminders need explicit `--include-muted` (§4.2).

- **Hide `def.override`, `patch.contested` by default** from problems and UI finding lists; end with `hidden: N def.override, M patch.contested (archo problems --all)`. Show with `--all`, `--code`, `--subject`, `--severity info`, `--min-severity info`; always visible in def, patches, details.
- **Collapse `version.unsupported`**: combine same-game-version unmuted reminders (§4.2), highest severity: `<sev> version.unsupported  N enabled mods do not list 1.7 (a list 1.6, b older only)  (archo problems --code version.unsupported)`. Same CLI/MCP/UI summaries; Mod row tooltips/details only state its declared versions (06 §5).
- **`harmony.multi-transpiler` defaults to info**, same escalation as skip-prefix below (not collapsed).
- **`harmony.skip-prefix` defaults to info, collapsed**: one summary (`… N bool prefixes from M mods can skip methods other mods also patch; none is in a log stack (archo problems --code harmony.skip-prefix)`). Promote only if patched method (without overload) appears in an error cluster of **current, non-stale problem log** (stack frame or owning frame of Harmony patch line): add cluster as log evidence, put `archo log --cluster E<n>` first. `List` escalated findings individually. `Full` list via `--code harmony.skip-prefix`, `--all`, `--subject`, `harmony --target`.
- Summaries precede individual findings within severity, avoiding default 30-row truncation.
- **MCP `list_problems`** defaults `--min-severity warning` unless severity, `min_severity`, all, `include_muted` supplied.
- **Log findings** group by log (§6.4). Current log first 5 by default; rest via `archo problems --code log`; stale logs collapse to uncounted title, expanded via `archo problems --code log --all`. Exclude `log.mod-report`, `log.other`.
- **Ignored findings** excluded by default (§4.2).
- **Can it run page** includes only actionable errors/warnings, leaving info in technical details; codes, grouping, wording: 06 §5.

## 5. Analyzers

### 5.1 Def Layer

- Stream XML, extract §2 fields only, no full DOM.
- Resolve inheritance after all patches, like game (patches can change `Name`/`ParentName`, e.g. `AttributeAdd` names Core `Defs`). Collect all patched-document `Name`, then resolve `ParentName`; later same-name abstract nodes override. Pre-patch checks create false positives (real measurements: 07 §6).

### 5.2 Patch Simulation

1. Merge effective Mod `Defs` into one in-memory document (`<Defs>` root), plan order.
2. Apply PatchOperations in same order, recording node match counts.
3. Support `Add`, `Insert`, `Remove`, `Replace`, `AttributeAdd`, `AttributeSet`, `AttributeRemove`, `SetName`, `AddModExtension`, `Sequence`, `Conditional`, `Test`, `FindMod`. Other classes are `patch.unknown-op`, treated as success.
4. Interpret success (`Always`, `Normal`, `Invert`, `Never`) like game.
5. Failure key = file + game's operation description (`FailKey`), matching `Player.log` failure messages (§6.2).

**XPath**: engine must match .NET XPath 1.0. Compatibility-test all XPath in real Mod collection, replay `Player.log` (simulate every logged failure), mismatch below 0.5%, documented differences; measurements: 07 §6. `Common` `Defs/XDef[defName="…"]` uses indexed fast path.

**Memory and parsing** (merged document exists only in subprocess):

- Nodes (`internal/xdoc`): 44-byte structs in 64K-node blocks; first block grows like slice to keep small documents small. int32 links; element names in shared `Names` table for merged/patch documents.
- Text in 1 MiB append-only pages; ≥ 64 KiB text allocated separately. No single growing text buffer: index-key references would retain old backing arrays.
- Parsing pipeline: up to 8 `encoding/xml` workers tokenize files, at most twice worker count files in memory; build documents sequentially in load order.

### 5.3 Assemblies and Harmony

Read .NET metadata natively in Go, no .NET runtime (`internal/dotnet`, `internal/harmony`). Read `Assemblies/*.dll` in effective directories of enabled Instances.

- Read `TypeDef`, `MethodDef`, `MemberRef`, `TypeRef`, `CustomAttribute`, `#Strings`, `#US`.
- **Targets**: merge class/method `[HarmonyPatch]` arguments (target type, method, `MethodType`, `Type[]` parameters, named arguments). `TargetMethod(s)` patches are dynamic.
- **Patch kinds**: `[HarmonyPrefix]` / `[HarmonyPostfix]` / `[HarmonyTranspiler]` / `[HarmonyFinalizer]`; otherwise infer method names `Prefix` / `Postfix` / `Transpiler` / `Finalizer`.
- **May skip original**: bool-returning prefix.
- **Manual patches** (corpus G9, `internal/harmony/manual.go`, `internal/dotnet/il.go`): if assembly has `MemberRef` to `HarmonyLib.Harmony::Patch`, abstract-interpret each method IL to resolve original/patch methods; wrappers in other assemblies with `(MethodBase, HarmonyMethod, …)` also count. Resolved targets become ordinary facts (target, kind, patch method), participating in shared-target/multi-transpiler like attribute patches; unresolved originals create dynamic `<manual: reason>` facts (known patch method). `harmony.manual` explains resolved call count/unresolved reasons. Still unresolved: runtime-selected originals (whole-game scans such as HAR `BodyReferenceTranspiler`), helper-built collections, `Assembly.GetType`, base methods referenced through subclasses (headings use declaring type).
- **HugsLib**: read `HugsLib.dll` normally in own Mod (`unlimitedhugs.hugslib`), attributing patches to it; skip only copies in other `Mods` (`harmony.OwnLibrary`).
- **Harmony ID candidates**: `#US` strings passed to calls like `new Harmony(...)` (in practice ID-like reverse-domain `a.b.c`), heuristic. Maps log patch owners to `Mods` when owner is not `packageId`.
- **Skip list** (`harmony.DefaultSkip`, case-insensitive glob): `0harmony.dll`, `harmony.dll`, `hugslib.dll`, `0multiplayerapi.dll`, `0prepatcherapi.dll`, `mono.cecil*.dll`, `system.*.dll`, `microsoft.*.dll`, `newtonsoft.json.dll`, `unityengine*.dll`, `assembly-csharp*.dll`. Skip Harmony facts but index `TypeDef` (frames there are libraries). Analysis input can replace (`Input.HarmonySkip`); no player setting yet.
- Fixtures: `testdata/dotnet` has fake `0Harmony`/`Assembly-CSharp` and four `Mods`' C# sources, build scripts, committed DLLs (4–5 KB each); tests need no .NET SDK.

### 5.4 On-demand Decompilation

Use ilspycmd only on demand, no background pre-decompilation (no resident process; exits after heavy work).

- **Locate**: `ARCHO_ILSPYCMD`, PATH, `~/.dotnet/tools`. If missing, error with `dotnet tool install --global ilspycmd`; never install automatically.
- **Reference directories**: game's Managed directory (game root in 1.7), DLL directory, `0Harmony`.dll directory.
- **Cache**: entire project in `<data-directory>/cache/source/<sha256[:20]>/` (write `.tmp`, rename, `.done` on completion); without cached project, decompile individual type with `-t` into `<hash>.types/<Type>.cs`. `source_cache` records each decompilation; rebuild deleted caches on demand; DLL changes invalidate.
- `archo source <instance> [--type <type>]`: only requested assembly/type.
- `archo source search <pattern>` requires scope: `--in <ref>...`, `--implicated` (latest log cluster candidate `Mods`; Harmony warning subjects without logs), or `--active`. `--active` prints estimate first (assembly count, DLL size, cache about 3× DLL, about 1.5 seconds/assembly), CLI requires `--yes`. Cluster next steps search only own candidates (`--in …`).
- MCP `read_source` with search defaults `--implicated`; only `scope: "active"` expands to all enabled `Mods`; default 10-second budget, report remaining assemblies on expiry ("still computing; run again").
- `archo files search` excludes `Languages/` by default (translations repeat every key), includes with `--languages`; traverse per Mod, 8 file-read workers, preserve traversal order. Commands: 05 §4; scope: §5.5.
- GUI: mod library › search files "C# code" (06 §2.9) searches named `Mods` only, no whole-list `--active`; up to 60 seconds decompilation per search (`view.CodeSearchBudget`), state remaining count, next search resumes; stop/new search cancels decompilation (`App.SourceSearchContext`).

### 5.5 Search Files Within Loaded Scope (2026-10-24)

`files search` / `search_files` searches only game-loaded content, ordered by load order (`internal/modsearch`):

- **`Mods`**: current plan's enabled entries by default, exact listed copies (Workshop/local/locked separate), with positions; repeatable `--mod` names others, choosing listed copy on package duplicates; include official Core/DLC by default, `--game without|only` excludes/searches only them.
- **Folders**: resolve per §1 (LoadFolders version/conditions; otherwise version/`Common`/root); traverse only game content subdirectories `Defs`, `Patches`, `Languages`, `Textures`, `Sounds`, `Assemblies` (plus `Lunar/Components`); `About/` at root only, no other root folders. Identical relative paths use highest priority only. `--all-versions` searches all version directories/every LoadFolders folder, ignoring conditions and deduplication, marking folder per result. Silently skip nonexistent LoadFolders targets; count unreadable files and show first error.
- **Kinds**: default `defs,patches,about`; select with `--kind`, add translations only with `--languages` / `--lang <language-folder>`. `Textures`/`Sounds`/`Assemblies` match relative paths, not binary contents.
- **Matching**: literal, case-insensitive default; `--regex` (RE2), `--case`, `--word`. One hit per line.
- **Results**: grouped Mod (position/reference/name/package/copy/loading folder) → file → line; `Defs`/`Patches` hits include containing top-level `Def` (`ThingDef/name`, abstract `ThingDef@Name`) or operation XPath. Default 5 lines/file, 40/page (`--per-file`, `--limit`, `--page`), totals always complete; `--mode files|count` lists files or per-Mod counts only. If default scope finds nothing, explain excluded scope.
- **`Def` mode** (`--def`): pattern is `defName`, whole-word case-sensitive, default `Defs`/`Patches` only. Roles: def (`<defName>`), abstract (`Name="…"`), child (`ParentName="…"`), patch (containing XPath names it), `patch-value`, translation, ref; summary first: definers (later effective if multiple), parent/children, patch operation/Mod counts, referencing Mod count. Text evidence only; `get_def` / `get_patches` simulation determines actual matches.

## 6. Log Attribution

### 6.1 Parsing

**Two formats**:

- `1.6` and earlier (`Unity`): no message level. Read `RimWorld X.Y.Z revN` in first 50 lines.
- 1.7 (`Waylith`, `=== Waylith log ===` within first 50 lines): `<date> <time> <level> [<category>] <message>`, then exception stack (if any) and call stack. Read `Version: X.Y.Z.N`. Collect only `WARN`, `WARNING`, `ERROR`, `FATAL`, `CRITICAL`; exclude `INFO`, `DEBUG`, `TRACE` from clusters. Discard logging-system call frames (`Brutal.Logging.*`, `Verse.Log`), mark others caller frames separately from exception frames. Call-stack-only messages are not exception. Rewrite `- PREFIX owner: Target.Method <- Ret Type:Method(…)` into `Unity` format; strip leading dot and `_PatchN` from .NET 10 `.Ns.Type.Method_PatchN(…)`, marking patched methods.

**Entries**: message line plus following continuation lines.

- Continuations: whitespace-prefixed; `[Ref `, `(wrapper `, `Rethrow as `, `--- End of`, `file: `, `Subnode:` (and next line); multiline .NET `Parameter name: `, `Actual value was `, `Object name: `; `Mono` native `<0x… + 0x…> <unknown method>`; bare frames without `at ` (inner exception stack).
- `(Filename: ` or blank lines end entry. Truncate lines above 4 KiB.
- `[Ref X] Duplicate stacktrace` refers to earlier original with same Ref.
- Combine XML Extensions blocks (`[Mod - Start of stack trace]` … `[End of stack trace]` … `Source file: <path>`) into `[Mod] <first-failure-line>`; rest continuations, path as `file:`.
- Entry location frame (`Top`): first exception frame, otherwise first caller frame (logging code).

**Excluded from error clusters**: no exception stack and engine/framework status lines: shader fallback (`Shader '…': fallback shader '…' not found`), translation totals (`Translation data for language … has N errors`), XML Extensions statistics (`[XML Extensions] N total patch operations run`), `Curl error N: Callback aborted`, `Mod … is missing supported versions list in About.xml`. Rules from corpus. Other `Unity` entries count only with exception stack, noninformational category, or error / exception / failed / could not / couldn't / invalid / missing / not found.

**Categories** (in order):

| Category | Detection |
| --- | --- |
| `patch-failed` | Starts `] Patch operation ` / `Patch operation `, `Verse.PatchOperation…): Failed`, `Verse.PatchOperationSequence: Error in the operation`, XML Extensions `Verse.PatchOperation…` failures |
| `xref-missing` | `Could not resolve cross-reference`, `Could not load reference to`, `Failed to find <X>Def named …` |
| `material` | `_MainTex` or `ResolveIcon` in stack |
| `xml-load` | XML errors, `Could not find parent node`, `Duplicate def`, `Could not find a type named`, `Config error in`, etc. |
| `texture-missing` | `Could not load Texture2D at`, `Failed to find any textures at`, `Could not load Texture\|Material\|Shader\|AudioClip\|SoundDef` |
| `save-load` | Unresolved save references: `Could not resolve reference to object with loadID`, `… is not deep-saved`, `Error in PostExposeData`, `Could not find think node with key`, `Exception loading … from save`, etc. |
| `harmony-stack` | Harmony patch lines in stack |
| `exception` | `Exception` stack or message containing `Exception` |
| `mod-report` | Mod self-report with `[name]`, `<color=…>[name]`, `name :: `, `MOD: `, etc. (not level words like `ERROR:`) |
| `other` | Everything else |

`mod-report` / other are informational, outside finding list.

**Category is not runtime phase**: names like `save-load` describe content only; phases use explicit Archo Monitor markers (§6.7.3–4), never category/stack/vanilla text such as "Loading game…". `logs.Entry.Phase` is latest preceding marker; `logs.Cluster.Phase` retains first phase, `Phases` counts each phase separately; keys/numbers do not change with phase. Old marker-free logs remain unphased.

**Clustering**:

- Normalize first 300 message characters: paths → `<path>`, addresses/long hexadecimal IDs → `<hex>`, numbers → `#`.
- Key = category | normalized message | (for `patch-failed`, continuations to distinguish files) | location frame `Type.Method`. Merge into `log:E<n>` by first appearance, count and first 5 line numbers.
- **Template** = category | further replace color tags, `defName=…`, quoted contents, names after `named / loadID / Type / field / at / for def / key`, `Def` in "found to give to X", parenthesized types, `packageId` with placeholders. Templates count problem kinds (each missing texture has distinct cluster, same template). Keys retain names needed for attribution.

### 6.2 Evidence

Attribute against facts: effective content, `Defs`, Patch simulation, `TypeDef`, `HarmonyId`. Match Mod names from About.xml (game prints `[Name]`, not aliases). Core/DLC (`ludeon.rimworld` prefix) never become sources from text matches or `Def` ownership.

Strongest first:

| Evidence | `Rule` |
| --- | --- |
| `[Mod name]` prefix | Self-reported issue → source. XML Extensions uses full names (`[[FSF] Vanilla Bionics Expansion] Verse.PatchOperation…`) |
| File path | `.xml/.dll/.png/.txt` in message/continuations inside Instance directory → source. Other-machine logs match Mod directories: `…\workshop\content\294100\<id>\…`, `…\Mods\<directory>\…` (including HugsLib-redacted `[Rimworld_dir]\Mods\…`) |
| Patch failure vs simulation | Match `patch-failed` by `[Mod]\|description` or `file:` → source, with `file:line`, §4.4 reasons; disabled target provider → dependency + `archo enable`. XML Extensions matches XPath. If no longer failing, state "the mod or the Loadout changed since the log" |
| `defName` (`xref-missing`) | Record missing `def:<Type>/<name>`; `Def` facts list enabled/disabled providers; disabled → dependency + `archo enable`; none: "providers: none among installed mods". `Could not load reference` states save-reading context. **Wanter**: enabled nonofficial definer of `Def` in "found to give to `<XDef>` `<name>`" → source. **Referrers**: search enabled effective `Defs`/`Patches` once for `>Name<` and `<Name>` (ThingDefCountClass lists like `costList`/products); skip Core/DLC and `Mods` defining name themselves; if over 3 `Mods` match or wanter found source, only mentioned |
| `for def 'X'` in `texture-missing` | X definer → source |
| File providers | For missing textures without enabled-Mod file reference, find installed nonofficial `Textures/<path>.*`; for nongame `Cannot resolve dependency to assembly 'X'`, find `X.dll`. Disabled → dependency + enable; enabled → mentioned (game not reading that loading folder?); none: "providers: none among installed mods". If assembly has no types and no enabled loader, details list installed DLL copies. Scope: live visible Instances, shared folders (Workshop, SteamCMD, extra, Monitor, locked) plus own `Data`/`Mods` (02 §0, `Store.InstanceFiles`), not other games' `Mods` |
| `xml-load` | `Could not find parent node named "X"`: installed disabled provider → dependency. `Two ThinkNodes have the same unique save key`: enabled patchers of `ThinkTreeDef/<key>` → patcher. Definers of `<defName>` / `Name="…"` in `Context` / `Full node` → source |
| `material` | Checklist (buildable `Def` with `uiIconColor` but no `uiIconPath` / `uiIconPathsStuff` / `graphicData/texPath`, including abstract bases), `archo files search "uiIconColor"`, `"uiIconPath"` |
| Missing types | `Could not find type named NS.T` / `expected class 'NS.T'`: disabled provider → dependency; enabled → mentioned (version mismatch?) |
| Harmony patch lines | `- PREFIX <owner>: …` → patcher; match owner by patch type, Harmony ID, `packageId` in order; otherwise "no installed mod matches" |
| Stack frame types | Non-game namespaces vs `TypeDef`: topmost Mod frame → source, lower → mentioned. Game namespaces: `Verse.`, `RimWorld.`, `System.`, `UnityEngine.`, `HarmonyLib.`, `Mono.`, `Unity.`, `MonoMod.`, `Ionic.`, `LudeonTK.`, `Steamworks.`, `Waylith.`, `Brutal.`, `RimWorldWaylith.` |
| Message type names | mentioned; "instantiating a mod of type T" / "static constructor of T" → source |

### 6.3 Roles and Output

**Roles**: source (produced error), patcher (only patched related method), dependency (missing disabled dependency), mentioned (only mentioned), candidates in this order. `Never` promote patcher without own frames/files. `Exception`: installed disabled `xref-missing` providers rank dependency before patcher. Missing `Def` belongs to provider; save/load patchers merely touched code (VM 2026-10-28: missing hair in save previously blamed Vanilla Expanded Framework patching `Pawn.ExposeData`).

**Confidence**: first source with file/patch evidence → certain; source with other evidence → likely; exception-first dependency → likely ("a mod that defines it is installed but not enabled", later source evidence restores source conclusion); candidates without source → possible ("no source identified"); no candidates → possible ("no mod identified"). Cached reports carry `logCacheVersion`; increment on attribution rule changes to reattribute old logs.

**Hot paths**: stacks through `Verse.Pawn.Tick`, `Verse.ThingWithComps.Tick`, `Verse.Thing.DoTick`, `RimWorld.Planet.WorldPawns.WorldPawnsTick`, `Verse.Pawn.TickRare`, `Verse.ThingWithComps.TickRare`, `Verse.TickList.Tick`, `Verse.ThingWithComps.CompTick` include tick-crash checklist (`AllComps` mutation during iteration, runtime comp addition without `InitializeComps()`, patch lines only show last method touch, also inspect ExposeData/SpawnSetup/DeSpawn/Destroy), `archo source search "AllComps.Remove(" --in …`, `"AllComps.Add("`.

**One `log.<category>` finding per cluster**:

- subjects: candidate Instances, or `log:E<n>` without candidates.
- claim: `log:E<n> x<count>: <message>`.
- evidence: candidate `<role>: <evidence>` (log), plus category details (reasons/providers, inferred).
- suggestions: first `archo log --cluster E<n>`, then next steps such as `archo patches --mod <ref> --failed`, `archo harmony --target Verse.Pawn.Tick`, `archo source <ref> --type <type>`, `archo def <Type>/<name>`, `archo enable <ref>`.
- key: cluster key, absolute paths reduced to final three segments (§4.2).

**Cluster details** (`archo log --cluster E<n>`, MCP, UI): complete message/continuations, messages around first occurrence (nearby: 4 before, 3 after, ≤30 lines, 200 characters/message), avoiding raw-log lookup (corpus G7).

**Error patterns** (`pattern:<id>`, §6.8): matching clusters show `[pattern:g:<id>]` at row end and `pattern:` in details. Community background knowledge only; no attribution/role/confidence changes or standalone conclusions.

**Pasted text** (`AnalyzeLogText`, e.g. pasted stacks) uses identical parsing/attribution, labeled `<text>`.

### 6.4 Which Launch Owns a Log

**First identify writing game**: `App.CheckLogOwner` checks before selection, cache reads, attribution, log-version analysis, marking seen. Prefer explicit startup-header game path against `game_root`: currently `Unity` `1.6` `Mono path[0] = '<game_root>/…_Data/Managed'`; absolute paths only, not Mod errors/stack source paths. Repository `Waylith` 1.7 sample has version but no verifiable game path; do not assume `Mono` lines. Without definite path, use time-matched launch or ModsConfig write records below; path/record conflict uses path for game, unknown plan. **Version mismatch alone does not mean another game**: directory may have upgraded; another environment matching log version is insufficient. Without evidence, retain unknown/stale behavior, do not guess another environment.

**Other games' logs do not belong here**: exclude before attribution/cache/`EnsureLogAnalysis`/seen writes; absent from finding lists (even stale groups), sidebar counts, new-log notices, last-run errors. Game log page, CLI problems/log/status, MCP share rules. Recheck selections overwritten by another game; live tracking checks each read/end, discards clusters after overwrite. **Explicit `log --file` / `--game-version` never bypass ownership**: refuse only when definitely written by another registered environment (`log belongs to environment <name>`); explicit own/unknown files remain viewable, unknown stale without analysis. Header path outside all registered environments (shared/corpus logs, same game before move) is unknown, not foreign. `--game-version` changes analysis version, not identity.

**Which file**: 1.7+ prefer `<user-data>/Logs/Player.log`, earlier user-data root (02 §2.3); find available `Player.log`/`Player-prev.log` at both, excluding foreign files. If current unavailable, finding list/game log/default `archo log` use available previous; none means no log. Pack 1.7 uses own userdata; `1.6` uses environment log (08 §5.3).

**Problem log**: player-selected (`archo log --select` or game-log page, stored `log.selected:<environment>`), or current `Player.log` if newer. **While game runs**, current environment file (`App.LiveLog`, same as `RunLog`) is this run, not last run: never use as problem log, even selected; `Player.log` yields to `Player-prev.log` (moved on startup), including cache-only `LazyLog` sidebar counts; restore after exit. 1.7 pack own logs unaffected. `archo log` still defaults live `Player.log` (2026-10-28: rechecking while running previously read live as last run, duplicating findings/counts). CLI problems/status/MCP are not lazy but choose by same rule. For idle memory, UI parses only when can it run opens; before then cache-only (`App.LazyLog`, `App.LogCacheOnly`); opening marks seen (`log.seen:<environment>`). If available `Player.log` is newer than last launch/seen log, TUI shows "new log"; GUI reads once in background without waiting for page (`API.ReadNewLog`, 2026-10-30, 06 §2.1), then releases normally.

**Analyze by log version**: confirmed own-game log with different major/minor and no `--game-version` first refreshes that version's analysis (`EnsureLogFileAnalysis` → `EnsureLogAnalysis`), attributes by log version, heading `analysed: as 1.6 (log version); environment is 1.7`. Same directory upgraded `1.6`→1.7 can analyze its `1.6` logs. Unknown files remain stale, uncounted, viewable; no automatic other-version analysis, but stored facts may attribute.

**Ownership and staleness**:

- **`Rule`**: match each log by write time to **latest launch record** starting no later than mtime (including other environments sharing userdata; 1.7 `Logs/` excludes pack launches because packs write their own) (environment/plan or pack/time/list fingerprint in 08 §2). Stale if any:
  1. Log major/minor differs from analyzed version;
  2. Matched launch is not analyzed plan (another own plan/pack; foreign environments already excluded);
  3. Launch list fingerprint differs from current enabled list (edited after launch). Replacing only same-position Archo Monitor version (08 §9) does not count: retrieve list at recorded launch/write version, verify fingerprint, compare Monitors as same item (`App.monitorSwapOnly`; VM 2026-10-28: post-launch check updated Monitor, incorrectly staling just-finished run and leaving only older logs).
  
  Without launch record (Steam/external launch), use latest ModsConfig write before log time (04 §4.3, writer plan); no match means unknown source, stale with heading explanation. Do not use ModsConfig.xml mtime anymore.
- Stale findings excluded from problems/status/UI counts and skip-prefix escalation; collapse to title by default; `--code`, `--subject`, `--all` show individual entries; `problems show` still works, naming log.

**Heading**: one per log: filename, declared game version, write time, current/stale with reason (e.g. `written by 1.6, the game is 1.7`), severity counts; stale adds `not counted` and expand command. Clusters below heading.

### 6.5 Caching

- Parsing/clustering is fast; referrer searches dominate (07 §6), so cache **whole attribution report**, one `log_cache` row per path, plus in-memory copy.
- Key = path + size + mtime + input fingerprint: `logCacheVersion`, `--game-version`, game major/minor, analysis run, enabled list, each installed Mod ID/snapshot/name/alias/`packageId`/directory. Any change reparses.
- status, mod (MCP status/`get_mod`) only read cache, never parse. Missing cache marks Pending; status explains errors uncounted.
- gzip JSON (since schema v25, old reports compressed in migration): reports often exceed logs (4 MB `Player.log` about 9 MB report), compressed to about one-twentieth. Invalid compressed report is cache miss/reparse.
- Report covers file size at read time (game may append), with CRC-32C of bytes for cursor (§6.6).

**First query for new log** (cache miss, target ≤500 ms, 07 §6):

- **Indexed `Name` lookup** (`def_fact_named`, only ~5% of rows with `Name`, filtered by `content_state` version; §3): missing-parent clusters query `Name="…"`; formerly full-version `Def` scan per lookup (83,000 rows ~70 ms), 190 clusters ~14 seconds. One query per distinct name/log.
- **Reference index** (`defName` referrers): per enabled set/game version, hashes names appearing as references in effective `Defs`/`Patches` (`>X<`, `<X>`, no whitespace). Analysis subprocess builds afterward (485 `Mods`, 5750 files, 77 MB: ~0.3–0.4 seconds, 4.8 MB); build on query if absent. `<data-directory>/cache/refindex/<key>.idx`, key = game major/minor + enabled Instance/snapshot/folders, order-independent; latest 8 only. Search only index-selected files, verify original forms taking earliest line; formerly reread every enabled file per missing name. Identical results across 4 corpus logs/4113 clusters; reference evidence now file-sorted.
- **Clustering**: classify each entry (level/message/continuations/frames) once; `Classify` once/entry; `noiseRe`, `textureRe`, `modReportRe` use one leading `^` (branch-local anchors made Go retry each position of long messages).
- Measurements in 07 §6: former 568 ms logs now 130–330 ms.

### 6.6 Live Tracking

Game detection: 08 §4. While running:

- **Incremental read** (`app.LiveLog`): remember `Player.log` path/offset; check each second, up to 8 MiB/read, remainder next time. Shrink means new file (restart): reset offset/clusters. Split lines, retain incomplete last line.
- Parser state across batches; entry ends at next noncontinuation or game exit (`logs.Parser`).
- Phase markers persist (§6.7.4). Old cluster repeated in new phase updates that phase count and this-run entry without new cluster. First marker assigns preceding entries to loading; no markers keeps flat entries.
- **Memory**: clusters only, no raw log; first 3 samples/count each; beyond 2000 discard oldest informational clusters. Memoize category by message + location + continuations (regex ~70 µs/entry, repeated once); clear at 10000 entries.
- Attribute each new cluster's first sample fully (§6.2), show in this-run part of can it run last-run errors, first-appearance order (06 §2.4). Update counts without dialogs; game-running/current-error count on GUI launch page/TUI status line (since 2026-10-30, no GUI sidebar-bottom text).
- On exit stop tracking, perform full analysis like `archo log`, replacing this-run section.

**Companion logs**: in-game Mod JSON (Archo Monitor `ArchoMonitor.log`, RimCrow `RimCrow_Realtime.log`) read incrementally with `Player.log`, adding timestamps/levels/counts/files/related `Mods`. Selection/fields: §6.7.

**Interface**: CLI `archo log --follow` emits new clusters until game exit/Ctrl+C; `--json` one cluster/line, `--once` stops after current contents. Other log flags (`--cluster`, `--file`, `--prev`, `--text`, `--all`, `--select`): 05 §4.

**since cursor** (MCP `analyze_log` since, CLI `archo log --since`, agent polling; implemented 2026-10-22):

- Cluster lists include `cursor: log1.<size>.<checksum>[.<companion-size>.<checksum>]`: `Player.log` and paired companion (§6.7.4) byte counts in base 36 plus CRC-32C. No on-disk cursor state; identical CLI/MCP.
- With since, analyze current log normally (cached if possible), recluster first size bytes without attribution for old counts; show only new clusters (new) and count increases (`+N`), heading added/total counts, new cursor. Companion prefix included, so companion-only entries after game stops `Player.log` and increased counts also count; changed companion session means no knowledge at cursor.
- **Expired**: shorter/different-checksum prefix means restart/new log. State expired cursor and list all current clusters. If current `Player.log` mismatches but `Player-prev.log` prefix matches, say cursor's run is now `Player-prev.log`, suggest `archo log --prev --since <cursor>` (MCP `prev=true`); mismatching previous means replaced by a newer run (another restart).
- Half-written final cursor line: completed entry counts new (prefer extra report over omission).
- `Full`-analysis polling, not `LiveLog`; analyze size at each call (§6.5 first-query speed).

### 6.7 Archo Monitor and Companion Logs

> Status: added 2026-10-21. Decision: RimArcho bundles in-game helper Archo Monitor, enabled in plans by default (04 §1.5), installed with releases (08 §9).

Only the game knows what `Player.log` cannot say: currently loading file/`Def`, Mod owners of stack assemblies/Harmony patches, errors after logging limit. Archo Monitor (`packageId` `toumanlin.archomonitor`) records these as JSON lines. Behavior follows RimCrow Companion 0.3.12 (decompiled code read only for behavior/format; implementation written independently); no code borrowed from its credited BetterModSort (Apache-2.0), LogAfterDefError (MIT), BetterLog (MPL-2.0), Mod Error Checker (no public license); About.xml credits all.

#### 6.7.1 What Is Captured

| Hook | Action |
| --- | --- |
| `Prefix` on `Verse.Log.Error(string)`, `Verse.Log.Warning(string)` | Record every error/warning. `Prefix` precedes `StopLoggingAtMessageCount` check, so count errors even after `Player.log` stops |
| `Prefix` on `Verse.Log.Message(string)` | Off by default (most entries, hot path); enabling "record informational messages" in Mod settings attaches on next startup |
| `Application.logMessageReceivedThreaded` | `Exception`/Error/Assert outside `Verse.Log` (no `Verse.Log` stack frame) |
| `Prefix` + finalizer on `DirectXmlLoader.DefFromNode`, `DirectXmlToObjectNew.DefFromNodeNew` (`1.6`+) | Thread-local stack: loading `LoadableXmlAsset`/`Def` node; map `defName` (otherwise `Name`) → file, owning Mod, `ParentName` |
| `Prefix` + finalizer on `LoadedModManager.ApplyPatches` | Mark XML patching; first entry collects enabled `Mods`' operations: XPath `defName`/`Name` → Mod/operation type, including `Sequence`/`FindMod`/`Conditional` children |
| `Postfix` on `DirectXmlToObject.ValidateListNode` | Referenced `Def` name → loading `Def`/file/list field from stack above, no extra XML traversal, for missing cross-references |
| `Postfix` on `Verse.UIRoot_Entry.Init()` | Menu initialization complete (`MainMenuDrawer.Init` already ran), enter menu; record again on returning to menu |
| `Prefix` on `Verse.SavedGameLoaderNow.LoadGameFromSaveFileNow(string)` | Begin loading save, enter save, retain filename without directory |
| `Prefix` on `RimWorld.Planet.WorldGenerator.GenerateWorld`, `Verse.Game.InitNewGame()` | Begin world/new colony initialization, enter new; consecutive same phase recorded once |
| `Postfix` on `Verse.Game.FinalizeInit()` | New colony/save initialization complete, enter play |

- **Stack → Mod**: declaring type assembly → loading Mod (assembly cache); Harmony patches on frame method → patch method assembly, fallback owner vs `packageId` (method cache). Exclude base assemblies (mscorlib, `System`, netstandard, `Microsoft`, `Mono`, `MonoMod`, `Unity`, `UnityEngine`, `Assembly-CSharp`, `Assembly-CSharp-firstpass`, `0Harmony`, `HarmonyLib`, `Waylith`, `Brutal`, `ICSharpCode`, `Newtonsoft`, `NAudio`, `NVorbis`, `ISharpZipLib`, `com.rlabrecque.steamworks.net`, Monitor itself); base `Mods` (Harmony, HugsLib, XML Extensions, Prepatcher, Performance Optimizer, RocketMan, Mod Error Checker) only without others. Stop at first non-base Mod frame. Text stacks (message-supplied/`Unity` callbacks) parse `Type.Method` per line through same mapping.
- **Extra context**, first successful rule: `Config error in X:` / `Exception in ConfigErrors() of X:` → X file, parent chain files (up to 10), patch operations on these `Defs`; `Could not resolve cross-reference … named X` → referencing `Def`/file/field and patches adding X; `… loadID X` → `loadID:X` only; while loading `Def` → current file/`Def`/Mod. Also map message `[a.b]` packageIds and `assembly name: X`.
- **`inferredType`**, first match: `DefConfigError` (above), `CrossRefError` (cross-reference/loadID), `PatchError` (during XML patching or `Patch operation` message), `XMLSyntaxError` (while loading `Def`; RimCrow name meaning error during XML loading), `AssemblyConflict` (`MissingMethodException`, `TypeLoadException`, `ReflectionTypeLoadException`, `MissingFieldException`), `TickException` (`Exception ticking`), `DrawException` (`Exception drawing`), `NullReference`, `OutOfMemory`, `MissingTexture` (`Failed to find any texture`, `Could not load Texture2D`, `Could not load UnityEngine.Texture2D`), otherwise empty. Monitor failures write `ObserverFailure` without affecting game.
- **Stack levels**: 1.5/`1.6` `Unity` raises Error/`Exception` below `ScriptOnly` to `ScriptOnly` (`Full` unchanged), unless other diagnostic `Mods` (Visual Exceptions, Mod Error Checker) are loaded. 1.7 `Waylith` has no setting and already writes full stacks.
- **Unchanged messages**: unlike RimCrow's `[Mod name, …] ` prefix, Monitor preserves `Player.log` message exactly for matching.

#### 6.7.2 Overhead

- **Per log (hot path)**: one level + original message hash lookup; hit increments count only. Miss normalizes/analyzes stack/context once per distinct message.
- **Loading**: one stack push/pop and dictionary write per `Def`; one write per `Def` list item (no node-path construction); one patch collection pass. Table caps: 300,000 `Defs`, 500,000 references; stop recording when full.
- **File writes**: 64 KiB buffer. First `ERROR` flushes immediately (crash may follow); other first occurrences/count changes written by background timer at most once/second; flush on `Application.quitting`/`AppDomain.ProcessExit`.
- **Phase changes**: only hook-driven, no polling/frame work. Under Writer lock, `FlushDirty` then write/flush boundary; no added stacks/reflection/file writes on per-error hot path.

#### 6.7.3 Files

- **Location**: `ArchoMonitor.log` under save-data root (`GenFilePaths.SaveDataFolderPath`, follows `-savedatafolder`, packs `<pack>/userdata/`; fallback command-line `-savedatafolder` then `Application.persistentDataPath`). On startup rename existing to `ArchoMonitor-prev.log`, replacing previous, create new current.
- UTF-8 without BOM, JSON object/line ending `\n`. Truncate message above 4 KiB/details above 16 KiB with … . At 32 MiB write `kind:"full"`, stop new entries; phase/exit still write remaining counts/boundaries.
- **Deduplication**: level | inferred type | normalized lowercase message (hex addresses → `<hex>`, numbers → `<num>`, whitespace collapsed). Exact-message hot table cap 8000, fingerprint table 4000; flush pending counts then clear when full; subsequent same messages treated as first.

**Format (schema 1)**: superset of RimCrow format, one reader handles both.

| Field | RimCrow Companion | Archo Monitor |
| --- | --- | --- |
| `timestamp` | `yyyy-MM-dd HH:mm:ss.fff`, local write time | Same |
| `level` | `ERROR`, `WARNING`, `INFO` | Same |
| `message` | Pre-stack text, prefixed [names] if `Mods` found | Original pre-stack text |
| `details` | Stack on every line | First entry line only; later count lines empty to avoid repeated stacks. `ERROR`/`WARNING` without own stack get calling `Verse.Log` code stack, excluding Monitor/logging frames |
| `count` | Occurrences represented: first 1, then increment since previous line | Same |
| `context.relatedModIds` | Lowercase player-visible `packageId` | Same |
| `context.relatedFiles` | Mixed Mod-relative/absolute paths, `patch:`<`Def`>:`<operation>`, `xref:``<name>`:`<node-path>`, `def:``<name>`, `loadID:<ID>`, `<root:``<node>`> | Absolute file paths only; others in `context.refs` |
| `context.inferredType` | `Names` above except `PatchError` | `Names` above |
| `source` | — | `"archo-monitor"` |
| `schema` | — | `1` |
| `id` | — | 8-hex fingerprint, identical per entry line |
| `total` | — | Cumulative count through this line |
| `kind` | — | Special lines only: session (first), exit (last on normal exit), full, phase |
| `context.phase` | — | Phase row: menu, save, new, play; loading before first phase |
| `context.save` | — | Save filename on save phase, if available, no directory |
| `context.roles` | — | `{"<packageId>": "xml" \| "def" \| "patch" \| "stack" \| "text"}`: file owner, `Def` chain definer, patcher, stack/patch owner, message mention |
| `context.refs` | — | `patch:`<`Def`>:`<operation-type>`@<`packageId`>, `xref:``<name>`:<`Def`>/`<field>`, `def:``<name>`, `loadID:<ID>` |
| `context.def` | — | `Def` loading at error time |

**Session line**, first: level `INFO`, message `[Archo Monitor] session <id>`, kind session; context: session (12 random hex), version (Monitor), game (`VersionControl.CurrentVersionStringWithRev`), `dataRoot`, `playerLog` (`Application.consoleLogPath`, empty if unavailable). Also `Log.Message` into `Player.log`: `[Archo Monitor] <version> session <id>: <full-ArchoMonitor.log-path>`. This pairs files: `1.6` pack `Player.log` stays `Unity`-fixed while companion is in pack root, avoiding guesses. **Exit line**: kind exit, `context.entries` distinct entry count.

**Phase line**: `INFO`, kind phase, message `[Archo Monitor] phase <phase>`, `context.phase` above; other envelope fields schema 1. Flush pending count first, so count rows belong to one phase; total remains whole-run cumulative. Write same phase message via `Log.Message` to `Player.log` regardless of informational logging setting. Read boundaries independently in file order; phase messages never form clusters.

#### 6.7.4 How RimArcho Reads Them

**Selection** (read-only, no rename/deletion):

1. Companion is in **this run's save-data root**: environment userdata for plans, `<pack>/userdata` for packs (08 §5.3). Analyzing `Player.log` uses its containing directory, parent of Logs in 1.7.
2. With `Player.log` session marker, try marker path, adjacent `ArchoMonitor-prev.log` (renamed by next startup), then both in data root; require matching session-line ID.
3. Without marker, try same-generation `ArchoMonitor.log`/prev with valid session, then RimCrow; marker with no session match falls back only to RimCrow. Generation pairing: `Player.log` ↔ `RimCrow_Realtime.log` (fallback `RMM_Realtime.log`), `Player-prev.log` ↔ same names with `-prev`. Candidates without session match must be nonempty regular files, mtime at/after run start and at/before `Player.log` mtime +1 minute; no confident ownership means no pair. Start: `Started:` in 1.7 (timezone), otherwise Windows creation time; absent or later than mtime falls back conservatively to mtime. Current-run lower bound also exceeds `Player-prev.log` mtime, avoiding stale creation times on `Unity` truncation/reuse.
4. Same companion (hardlinks/path aliases included) cannot pair with both Player.logs in directory: session evidence first, current generation on ties. `app.CompanionFor`/full analysis/log page/live tracking share rules; recheck on companion append/rotation even if `Player.log` unchanged.
5. Live (§6.6): select by above in run data root, Monitor before RimCrow; recheck session on marker arrival, including rotated files. Shrink resets like `Player.log`. Read companion first for classification hints; reattribute already-attributed clusters when companion arrives. Wait for companion-only pairing per 06 §2.16.1 L03 (near-simultaneous writes), no wait after game exit; keep waiting if unfinished `Player.log` last entry is the match or unread backlog remains. Late `Player.log` entries merge into companion-only cluster, become first sample/reapply attribution; max counts, no extra cluster. As full analysis, presence checks include all `Player.log` entries, even noise/info (normalized first-line hashes only, cap 50,000 then clear): companion stack passing noise filter does not turn same `Player.log` noise into error.

**Field usage**:

| Field | Use |
| --- | --- |
| `message` | First line normalized per §6.1 matches clusters. RimCrow with `relatedModIds` and leading [ strips through first ] space |
| `id` | Merge Monitor by id; RimCrow by level + normalized message |
| `timestamp` | First line time = cluster first occurrence, shown in live/full details |
| `level` | Cluster level (absent in `1.6` `Player.log`); `ERROR` → error, others warning |
| count, total | Sum counts, or max total if available. No phases: max of `Player.log`/companion; phases: merge below. Only companion knows counts after game logging limit |
| `kind:phase` | Assign messages/count rows latest preceding phase. Max per phase across sources, then cluster phase totals, no double addition; unlocatable total-restored counts remain unknown (UI "phase not recorded"). If only companion has markers and `Player.log` entry lacks companion, unknown, never assumed loading |
| `details` | Monitor-only `ERROR`/`WARNING` (usually after game logging limit): message newline details through §6.1 parser, ordinary cluster/attribution, details "only in `ArchoMonitor.log`". Not for RimCrow (prefixed messages, often guessed stacks) |
| `relatedFiles` | Absolute path in **enabled** Instance → source, evidence `ArchoMonitor.log`: `<Mod-relative-path>`, same strength as §6.2 file paths (certain possible). RimCrow relative paths combine each `relatedModIds` root; pseudo-paths `patch:`/`xref:`/`def:`/`loadID:`/`<root:` treated as refs |
| `relatedModIds` + roles | xml/def with same-Instance file evidence → source; patch → patcher; stack/text/no role (RimCrow) → mentioned. `Add` evidence, never downgrade stronger roles |
| `refs` | Evidence text on corresponding Instance (`patch:…@<packageId>` on patcher) |
| `inferredType` | If own category other/exception and hint more specific, use hint: `DefConfigError`/`XMLSyntaxError` → `xml-load`; `CrossRefError` → `xref-missing` or `save-load` for loadID; `PatchError` → `patch-failed`; `MissingTexture` → `texture-missing`. Changes key, identical live/full rules; other hints details only |
| `context.def` | Evidence `while loading <Def>` |

- **UI**: can it run file-backed cluster evidence chip kind log ("log"), source `file <Mod-relative-path>  ArchoMonitor.log  <time>` (`…  RimCrow_Realtime.log` for RimCrow); file first with explicit label, spacing per 06 §7.3, window splits on two spaces (VM 2026-10-28: old `ArchoMonitor.log` path spacing collapsed into apparent single path). Multiple same-Mod locations show first plus N more.
- **Game log page** (06 §2.10): companion section "Archo Monitor · this run", row per entry (time/level/short `inferredType`/first line/count/`Mods`/files), expand details, jump matching cluster; header session version/game and session/exit times. File selector lists companion below `Player.log`. Same reader (also session game/exit time/full), cached path/size/mtime; running uses live-loaded data, no extra read.
- **`Phases`/display**: with phases, deduplicate entries per boundary, retain first file order; same id can reappear after save load to show boundary. Global attribution still merges by id. Can it run/log separators: 06 §2.4/§2.10; CLI/MCP: 05 §4. Older Monitor/RimCrow/no Monitor without markers retains old count/display rules.
- **Cache** (§6.5): add paired companion path/size/mtime.
- Skip/count invalid lines (live `BadLines`, full heading); companion entry table and UI phase-entry table each cap 20000; update existing entries only when full.

#### 6.7.5 Version Differences

| | 1.5, `1.6` | 1.7 |
| --- | --- | --- |
| Runtime | `Mono`, net472 assemblies | .NET 10 (`Waylith`), net10.0 |
| Harmony | Mod-bundled 2.x | 3.x |
| `Application` | `UnityEngine` | `UnityStubs`: `logMessageReceivedThreaded`/quitting/`consoleLogPath`, no `SetStackTraceLogType` |
| `DefFromNodeNew` | `1.6` yes, 1.5 no (reflection, skip if missing) | Yes |
| Phase hooks (§6.7.1) | All in reference assemblies; `GenerateWorld` lacks `LandmarkDensity` in 1.5, has it in `1.6`; reflect by name | All present; `GenerateWorld` parameters same as `1.6` |

`LoadFolders.xml` chooses version folders: net472 in `1.5/` and `1.6/`, net10.0 in `1.7/`; reflect hook targets, skip missing and list in session `context.skipped`.

Phase hook signatures checked per version against Krafs references selected by `monitor/ArchoMonitor.csproj`; references have signatures only; actual menu/world/load/play timing needs full rounds in `1.6`/1.7 (pre-release VM checklist).

### 6.8 Error Pattern Library

Public HugsLib corpus patterns (`docs/research/00-log-corpus-design.md` §3 G5, §9; integration `docs/research/01-pattern-library-integration-draft.md`) embedded as `internal/patterns/patterns.json`, like `internal/order/observed.json`, templates/packageIds/counts only. Generate with `go run ./spikes/patterns -export internal/patterns/patterns.json`; `-verify <corpusrun-result>` checks product/research classifications per cluster (v0: 610,910 clusters, 0 differences).

| Layer | Reference | Recognition | Cluster display |
| --- | --- | --- | --- |
| Generic | `pattern:g:<id>`, e.g. `pattern:g:xml.type-missing` | Ordered regex on message part of template (§6.1, category\|message), optional category restriction; first match wins | One-sentence explanation; `evidence: community: seen in N of M public 1.6 logs (x% of error clusters)`; common causes; diagnostic route (MCP tools, omit current step, replace `E<n>`); fix kind; error disappearance after 1–3 list edits on same-player repeat uploads vs unchanged control (hide below 5 pairs) |
| Fallback | Same, fallback | Bare `NullReferenceException`, other exception/XML/patch/save errors: category only | Hidden |
| Mod-specific | `pattern:m:<packageId>:<hash>` | Exact template, Mod attributed source | `known-error:` log count, first/latest months, observed versions, "newer Mod versions may have fixed this" |

- Patterns describe typical error kind, not culprit in this list; appearance is not cause. UI labels "community background", only on match (06 §2.10).
- Mod-specific templates need ≥10 logs/5 player sessions; discard path/user-data-looking templates (docs/research/00 §8).
- `archo pattern [<reference>]`: rule, phase, counts, causes, route, fixes, repeat-upload evidence; no argument lists generic patterns by cluster share; `--mod <packageId>` lists known errors. MCP `analyze_log pattern=``<reference>` (05 §4).
- Checklist (MCP `guide topic=errors`, CLI `archo guide errors`, 05 §5.3), corpus-frequency order, covers gaps in `guide topic=diagnose`: save loading, missing assets, config checks, assembly loads, noise; diagnose step 3 links it.
- Distribution: v0 bundled; later updates via `archo data update` like community sort rules (04 §6.3).

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| B1 | Patch-failed log | Specific Patch file/line, §4.4 reasons (golden harmonylog: `Patches/Compat.xml:2 … (patch.no-match: target-inactive, unguarded-optional)`, Lib dependency, certain) | Pass |
| B2 | Missing cross-reference | Referrer Mod/file, whether provider exists | Pass |
| B3 | `Pawn.Tick` exception, 3 `Mods`' Harmony lines | All patcher, no source; checklist, `harmony --target`/`source search` `AllComps`.`Remove`( | Pass |
| B4 | `Exception` repeated 500 times | One cluster `x500` | Pass |
| B5 | Old inspector replay (`internal/app/m4_test.go`) | Same/better conclusions: selectors vs enabled state, `files search` excludes `Languages` with Mod refs, decompilation rebuilds deleted cache/invalidates changed DLL | Pass |
| B6 | Own `1.6` log after game upgrade to 1.7 | Auto analyze as `1.6`, heading notes (`TestLogOwnVersion`, `TestOwnLogBeforeGameUpgrade`) | Pass |
| B7 | Own old-version log or before plan edit | Stale, uncounted, collapsed (`TestStaleLog`, `tui.TestProblemsViewLogs`) | Pass (current rules) |
| B8 | New process rereads same log | `log_cache`, no parse; status cache-only (`TestLogCache`) | Pass |
| B9 | Log after plan A launch, analyze B | Stale, other-plan launch reason | Pass |
| B10 | Edit A after A launch | Stale, edited-after-launch; restoring original list makes current | Pass |
| B11 | Steam launch without record, previous save of A | Belongs to A | Pass |
| B12 | Known pattern template (§6.8) | pattern ref, pattern/`analyze_log` follow-up; no fallback display; Mod-specific only source (golden harmonylog) | Pass |
| B13 | Corpus `dep.observed-missing`/`combo.known-bad`/`mod.known-error` | `known.json` findings for enabled set, community evidence (`internal/app/known_test.go`) | Pass |
| B14 | Shared userdata, another game writes log | Exclude before analysis/attribution/cache; no seen/new notice/findings/counts; own previous fallback; CLI/MCP parity (`TestForeignLogExcludedBeforeAnalysis`, `TestLogOwnerEvidence`, `TestForeignLogCLIParity`) | Synthetic pass; VM pending |
| C1 | Enable Mod with uninstalled dependency | `dep.missing`, Workshop suggestion | Pass |
| C2 | Same-package Workshop/local both enabled | `pkg.duplicate-active` | Pass |
| C3 | Two `Mods` define `ThingDef` | `def.override`, effective definer | Pass |
| C4 | Disabled patch target, no `FindMod` | `patch.no-match`: `target-inactive`, `unguarded-optional` | Pass |
| C5 | Two transpilers same method | info; warning if current stack (golden harmonylog Mod B/C `Verse.Pawn.Tick`, current warning/stale info) | Pass |
| C6 | Manual code-only patching | `harmony.manual`, no false no-patches | Pass |
| C7 | Flip one Mod LoadFolders condition | Only that Mod effective content invalidated (`TestLoadFoldersConditions`) | Pass |
| C8 | Alternate 1.7/`1.6` analysis | Both retained (`TestAnalysisPerGameVersion`) | Pass |
| C9 | bool prefix target in current error cluster | warning, cluster evidence/log command; stale no escalation (`TestSkipPrefixEscalation`) | Pass |
| C10 | `source search` no scope; active without yes | Usage error; estimate only/no decompile (`TestSourceSearchScope`) | Pass |
| C11 | Switch between different enabled-set plans | No rerun on second return | Pass |
| C12 | RimArcho locked copy, Workshop update | `mod.pinned` warning upstream update, diff available | Pass |
| L1 | Append fixture exception | New cluster within 1 second, same attribution | Pass |
| L2 | Entry split across batches | One entry | Pass |
| L3 | Truncate/rewrite | Reset offset/clusters | Pass |
| L4 | Append 500,000 identical errors | One cluster/count 500,000, resident growth <20 MB | Pass |
| L5 | `RimCrow_Realtime.log` JSON/bad lines | Timestamps, bad-line count | Pass |
| L6 | Only `RMM_Realtime.log` | Read without rename (`TestCompanionFor`) | Pass |
| L7 | MCP since cursor | Only new/increased clusters including companion counts; restart expiry/Player-prev explanation (`TestLogSince`, `TestLogSinceMonitor`, `mcp.TestCLIParity`) | Pass |
| L8 | Matching session, `relatedFiles` enabled `Defs/X.xml` | source, `ArchoMonitor.log: Defs/X.xml` evidence, can it run file chip (`TestLogAnalysisWithMonitor`, `view.TestCompanionChip`) | Pass |
| L9 | Matching prev sessions, current different | Pair prev with prev, not current with old (`TestCompanionFor`) | Pass |
| L10 | Monitor-only `ERROR` after game logging stops | Cluster, companion total, live waits per L03 (`TestLogAnalysisWithMonitor`, `TestLiveLogMonitor`) | Pass |
| L11 | Bad lines, incomplete last, RimCrow prefix | Count bad, wait incomplete, strip prefix to match (`logs.TestCompanionMonitor`, `logs.TestCompanionRimCrow`) | Pass |
| L12 | Pack `-savedatafolder`=`<pack>`/userdata | Read pack Monitor, not environment (`TestCompanionFor`, `TestLiveLogMonitor`) | Pass |
| L14 | Real `1.6`/1.7 known XML error with Monitor | Entry/file in Monitor, can it run file evidence | Pending (manual checklist 07 §4) |
| L13 | Count lines 1/5/3, total 9 | Count 9, first timestamp (`logs.TestCompanionMonitor`, `TestLogAnalysisWithMonitor`) | Pass |
| L15 | Companion first, `Player.log` ends/appears after 5 seconds | One cluster, `Player.log` evidence; later errors counted (`TestLiveLogMonitorFirst`; split in VM 2026-10-28). `Player.log` noise (e.g. `Translation data … errors`) with companion stack also excluded live (`TestLiveLogMonitorNoise`) | Pass |

Fact set/cleanup acceptance: `TestFactSetsShareVersionsAndUpdates`, `TestFactSetsDifferentFoldersAndPriority`, `TestFactSetsHarmonyViews`, `TestFactPruneVisibilitySlotsAndEnvRemoval`, `TestFactSetsDifferentAssemblyInputs`, `TestMigrateV26FactViews`; scale/migration measurements: 07 §6.
