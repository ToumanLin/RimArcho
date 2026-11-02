# 03 · Facts and Analysis

> Status: the declaration, `Def`, Patch, and Harmony layers, log attribution, and live tracking are implemented. Logs are assigned using launch records (§6.4), and multiple analyses are cached per plan (§3).
> Basis: 00, 00a, 02, 04, 07 §6, 07 §7, 08 §4

This document explains how RimArcho collects facts from Mod files and game logs, then derives findings from those facts. See 02 for identity and scanning, 04 for plans and sorting, 05 for commands and MCP tools, and 06 for their presentation in the interface (the wording for the can it run page is in 06 §5). Performance targets and measurements belong in 07 §6; this document only references them.

The "analyzed plan" is the plan whose enabled list and order the analysis uses (04 §1). In the interface, this is the open plan. The CLI and MCP default to the environment's current plan.

## 1. Effective Content

The game does not load every file in a Mod directory. All content analysis (`Def`, Patch, assemblies, and log reference lookup) uses only the effective content. It follows the game's `ModContentPack.InitLoadFolders` and `DirectXmlLoader.XmlAssetsInModFolder` semantics (07 §7).

**Version directory selection**. For a game major.minor version `V` (e.g. `1.6`), select the directories as follows:

- **With `LoadFolders.xml`**: use the exact `V` tag if present. Otherwise, use the highest tag not above `V`, then `<default>`, then the lowest tag. Within the selected tag:
  - All unconditional `<li>` entries apply.
  - Evaluate `IfModActive`, `IfModNotActive`, and `IfModActiveAll` against the analyzed plan. Treat the legacy `IfDLCActive` and `IfDLCNotActive` as `IfModActive` and `IfModNotActive`, respectively. Strip the `_steam` suffix from conditional packageIds before comparing them.
- **Without `LoadFolders.xml`**: use the version directory, `Common`, and the root, in descending priority. For the version directory, use the exact version if present, otherwise the highest version not above `V`, otherwise the lowest version. Version directory names must match `^[vV]?\d+\.\d+$`. For resources with the same relative path, use only the copy with the highest priority.
- Compare versions as numeric tuples: `1.10 > 1.6`.

**Encoding**. For About.xml, `LoadFolders.xml`, `Defs`, and `Patches`, detect UTF-16 LE and BE and UTF-8 from the BOM, then convert to UTF-8 before parsing. Also read a file as UTF-16 if it has no BOM but its XML declaration specifies `encoding="utf-16"` (`internal/textenc`, shared by scan, xdoc, and content).

**Conditional directories**. For each loaded conditional directory, record the packages named in its conditions (`Resolved.Requires`). Treat the `Patches` in that directory as guarded, with those packages as their expected targets (§4.4).

**Cache key**. Conditional `<li>` entries make the effective content depend on the plan, so the effective-content cache key is:

`snapshot hash | hash of ordered effective directory list | analyzer version`

Preserve the directory priority order and duplicates. Clean the paths and normalize their separators first (`content.FolderKey`); changing the order can change which same-name resource overrides another. The game version and condition evaluation already affect directory selection, so identical directories do not need separate sets for different versions or condition text. The key excludes the whole plan. A plan edit invalidates only the `Mods` whose effective directories change (`TestLoadFoldersConditions`).

Assemblies use a separate key, `analyze.AssemblyKey`: the snapshot hash, the relative paths of the effective DLLs, each DLL's Harmony-skip flag, and the analyzer version. Different XML directories can share DLL facts when the effective DLLs and skip policy match.

## 2. Fact Types

| Fact | Fields | Source |
| --- | --- | --- |
| `ModDecl` | Dependencies (including version-specific dependencies), loadBefore/After, forceLoad*, incompatibleWith, `supportedVersions`, and the original `LoadFolders.xml` | The snapshot's `decl_blob` (02 §4.3) |
| `Def` | Type (element name), `defName`, `Name`, `ParentName`, `Abstract`, file relative to the Mod root, line of the opening tag, owning Instance, and the `Def`'s own `MayRequire` and `MayRequireAnyOf` | Effective `Defs/**/*.xml` |
| `PatchOp` | Operation class, XPath, child-operation structure, value summary, file and line, ordinal, and simulation result and reason | Effective `Patches/**/*.xml` |
| `TypeDef` | Assembly, namespace, and full type name | .NET metadata in effective DLLs |
| `HarmonyPatch` | Target type, method, and parameters; `MethodType`; patch kind (prefix / postfix / transpiler / finalizer); patch method; whether it may skip the original method; and whether the target is dynamic | Same as above |
| `HarmonyId` | Suspected Harmony ID strings, identified heuristically | Same as above |
| `Rule` | Subject, relation, object, source (user / community / replacement), and scope | 04 §6 |

- Each fact is associated with an Instance, a snapshot, and the analyzer version (`analyze.Version`) through its fact set. Reads use the version views in §3 for the current `Store.MM`. Facts are rebuilt when the analyzer version differs.
- The game loads `Defs` with `MayRequire` only when the required `Mods` are enabled. Analysis follows the same rule (`DefFact.Loaded`).
- `archo def` lists the `file:line` of each definition in load order and marks the last one `← effective`.

## 3. Execution and Storage

**Layers**. The declaration layer takes milliseconds and is computed on demand. The `Def` layer follows it, then the Patch layer, which depends on the `Def` layer. Harmony is independent. A failure in a higher layer does not affect the results of lower layers; a Harmony failure affects only its own facts.

**On-demand declarations**. The declaration layer uses no subprocess and stores no results in the database. Its results are cached in-process together with plan reads. The key combines `Store.ChangeKey()` (`PRAGMA data_version` + `total_changes()`), the environment ID, and the size and modification time of ModsConfig.xml. Frequent interface refreshes and detail panels reuse this cache.

**GUI list snapshots**. The editor and can it run share finding snapshots in memory. List edits update the declaration layer synchronously. The content layers retain their previous results, marked as updating, while logs and analysis are completed in the background. See [06 §8.7](06-ui/8-gui-technology.md#87-list-findings-and-needs-cache-api) for the keys, invalidation conditions, and event contract.

**On demand**. Stale parts are checked and rebuilt only when can it run opens, the plan editor's details need findings, or the CLI or MCP requests content information. The problems, def, patches, and harmony commands rebuild stale analysis through subprocesses. The mod, status, and why commands stay fast by using only stored results (`--no-analyze` also prevents problems from rebuilding).

**Subprocess**. The `Def`, Patch, and Harmony analyses run in an `archo analyze` subprocess. When the stored results are stale, the caller (`App.EnsureAnalysis`) starts `<exe> analyze --env <name>` and passes `RIMARCHO_HOME` and `--game-version`. The subprocess writes the facts and exits without affecting the main process's memory.

- The subprocess writes `progress: …` lines to stderr. The interface displays them in the status bar or progress bar, and the CLI displays them as text.
- The subprocess has a soft limit of `debug.SetMemoryLimit(300 MiB)` (`AnalyzeMemoryLimit`). See 07 §6 for the peak target.
- Tests inject an in-process runner (`InProcessAnalyze`).
- **Wait limit**: the default `App.AnalyzeWait` for each entry point is in 06 §2.16.1 W01; the CLI has no limit. On timeout, return the declaration findings with "still computing in the background". The subprocess continues, and the next call retrieves its results. Meanwhile, def, patches, and harmony use the previous, stale results.

**AnalysisKey (freshness)**. Hash the analyzer version, the game major.minor version, and each included Instance's ID, snapshot hash, and position in the plan. Use the first 24 hexadecimal characters. User and community rules are excluded because they affect only the on-demand declarations. Computing the key is cheap and does not require loading the declarations.

**Included Instances**. Include the analyzed plan's enabled list and all other live, loadable, disabled Instances. Loadable Instances include `Mods` already visible to the game and SteamCMD or other-folder `Mods` that can be deployed through links on save (since 2026-10-28; previously, these disabled copies had no `Def` facts, so finding providers of missing parent templates required per-file fallback reads). Extract only `Def` facts from disabled Instances, for `target-inactive` (§4.4) and log queries about who provides a missing `Def`. Harmony reads only enabled Instances.

**`--game-version X.Y`**. The analyze, problems, def, patches, harmony, why, and log commands accept this option. They validate the `X.Y` format, override the environment version, and restore it after the command. The corresponding MCP tools expose `game_version`. Use this when the game is 1.7 but the list and logs are still `1.6`.

**Storage** (`store.SaveAnalysis`, `store.saveContent`, `store.PruneFacts`).

- An analysis slot is `(environment, game_mm, enabled-set fingerprint)` (`analysis.set_key`), with the latest run retained per slot. Runs, patch facts, and findings remain isolated by environment and version. AnalysisKey detects order changes. Plans with the same enabled set share a slot. Saving an analysis removes enabled-set slots that no plan uses, across all game versions in that environment.
- The unique key of `fact_set(id, instance_id, kind, content_key, files)` is `(Instance, fact kind, content key)`. `kind=def` stores `Defs`; `kind=dll` stores Assembly, Type, Harmony, and Harmony ID facts. Fact rows carry only `set_id`. The set determines the Instance, whose ID format remains unchanged.
- **Set file lists** (v28, `store/setfiles.go`): `fact_set.files` is a JSON array of the Mod-relative paths used by the set, ordered by their first use in a row. The `def_fact`, `type_fact`, `harmony_fact`, `harmony_id`, and `assembly_fact` tables store a zero-based `file_ix` instead of a path. Build the list once per set on save (`saveContent`, `saveHarmony`). It is immutable after the set is written and is deleted with the set. After reading the rows, readers (`DefsOf`, `DefsNamed`, `HarmonyPatches`, `Assemblies`, `TypeOwners`) read the list once per set and map the indexes back to paths. `Store` caches the list by `set_id` and clears it on `ChangeKey`, because a deleted set's ID may be reused. When the total exceeds 65,536 paths, randomly discard half. `CachedDefs`, which reads all disabled Mod `Defs` for each analysis, reads one set and its list together. It bypasses the cache to avoid evicting other lists. See 02 §7 v28 for migration and result equivalence.
- `content_state` retains the primary key `(instance_id, game_mm)`. Its `def_set_id` and `dll_set_id` point to the sets, and it retains the effective directories and parse summaries. Every fact read joins this version view by `Store.MM`. The same Instance shares sets across environments and game versions when its content is identical. Disabled Instances build only `Def` facts and retain a stored DLL view only if the content is unchanged. Once enabled, an Instance writes an empty set even if it has no effective DLL, clearing the old DLL view.
- Sets are indexed by `set_id`. Global name queries use the name index first, then join the version view. `def_fact_named(name, set_id) WHERE name<>''` preserves the fast path for parent `Name` lookup in logs. Type and Harmony ID queries also look up sets by name before scanning individual `Mods`. The DDL is in `store.schemaV26`; the five fact tables that use `file_ix` are in `store.schemaV28`.
- **`Def` lookup order by name** (`Store.DefsNamed`, used by def, log attribution, and missing-parent fallback): order by Instance ID first, then by file order within a Mod. Do not use table row order. Cache rebuilds and migrations change row numbers, which can change the first provider of a missing parent template and the log finding references (2026-10-29 VM: the analyzer 7→8 rebuild changed the first provider for `log:E6` and its `p:` reference). Sorting does not change the join order: use the name index first, then join the version view (`CROSS JOIN`, as in `TypeOwners`). With an ordinary `JOIN`, SQLite avoids sorting by scanning the entire version's `content_state` and querying the name index for each row. This is dozens of times slower, with the gap growing as the Mod count increases (07 §6.2; `TestDefsNamedPlanStartsFromName` checks the query plan).
- **Plan limitation remains**: version views are not plan views. The last directories analyzed for `(instance, mm)` replace that version view. If two plans select opposite LoadFolders conditions and load different folders, returning to a plan with a fresh slot may still show the other plan's content in def. Sharing sets by directory does not change this existing limitation.
- **Slot retention**: slots matching the environment's current major.minor version do not expire by age. Other slots expire when `store.ForeignAnalysisRetention` has elapsed since the last completed analysis; their run patches and findings are deleted too. The default is in 06 §2.16.1 A11. An unknown game version suspends age expiry.
- **Version view cleanup**: delete `(instance, mm)` if the Instance no longer exists, or if no environment that can see it currently uses mm or has an mm analysis slot. Determine visibility using 02 §7, without accessing Mod files. Conservatively retain views for visible environments with an unknown game version. Check all environments for shared Instances and only the owner for private ones. Apply the same rule after deleting an environment or slot.
- **Set cleanup**: immediately delete sets with no `content_state` references and cascade the deletion to their fact rows. `SaveAnalysis`, `env remove`, gc, and link identity merges perform cleanup within their own transactions. The independent maintenance call `Store.PruneFacts(now time.Time) error` can be repeated. Cleanup changes only the fact cache and analysis slots; maintenance handles shrinking the physical database.
- **Migration**: v26 converts the snapshot and analyzer version in old content keys, together with the stored directories, into `Def` sets without rereading Mod files. The old DLL cache did not record the skip policy, so migration conservatively shares it by snapshot, analyzer version, and a fingerprint of all DLL facts. The next analysis replaces this with the normal input key. Retain unrecognized old keys per version without guessing whether they can be shared. Do not migrate facts with no old version-view references. A transaction failure rolls back the entire migration. Migration does not run `VACUUM` automatically.
- See below for the Source library and `source_use` (§5.4) and log attribution `log_cache` (§6.5).

## 4. Findings

### 4.1 Structure

| Field | Description |
| --- | --- |
| `code` | The finding code (§4.3) |
| `severity` | error / warning / info |
| `subjects` | Entity references; the first is the main subject (Instance, `pkg:<id>`, or `log:E<n>`) |
| `claim` | A one-sentence conclusion |
| `evidence` | A list of "reference + explanation + source type" entries. The source types are declared (the Mod's declaration), inferred, log, community, and user (a player rule). |
| `confidence` | certain / likely / possible |
| `suggestions` | Executable commands such as `archo enable #b21c`. Prefix suggestions that are not archo commands: `subscribe: <Workshop-link>` or `steam://url/CommunityFilePage/<id>`. Display them as links. |
| `key` | The key that identifies this finding across runs within its code, for references and ignores |
| `layer` | decl / def / patch / harmony / log |

The evidence chips in the interface represent evidence source types (06 §5).

**Principle: appearance is not causation.** A Mod patch appearing in a stack is only related (patcher). It is not a source unless its own frames or files provide evidence (§6.3).

### 4.2 Ignores and References

- A **finding reference**, `p:<6-hex-digits>`, is the first 3 bytes of sha1(`code | key | subject Lineage`) and is stable across runs. Log finding keys reduce absolute paths to their final three segments and lowercase them, preserving references and ignores after the Mod library moves or in tests.
- An **ignore** records `(Lineage, code, key)` on the Lineage, per environment, together with the subject's snapshot at the time it was ignored. A snapshot change (a Mod update, visible in mod library › changes) expires the ignore. The finding returns with `[ignore expired: the mod changed]`.
- The commands are `problems ignore|unignore <p:ref>`, `problems --ignored`, and `problems show <p:ref>`. The default list ends with `ignored: N (archo problems --ignored)`. See 06 for the interface keys.

**Muting and restoring version reminders** are independent of snapshot-based ignores (`app.SetVersionMute`, `store.VersionMutes`). The key combines the normalized `packageId` (trim whitespace, lowercase, and strip the game's `_steam` suffix) and the game major.minor version. All plans, environments, and copies of the same package share it. Mod updates, snapshot changes, and Lineage merges or splits do not clear it; another major.minor version triggers a reminder again. A changed package ID identifies another Mod. Schema v23 adds `version_mute(package_id, game_mm)` with a composite primary key, independent of the environment, snapshot, or Instance. Migration leaves the old `finding_ignore` unchanged.

- `ProblemSet.Muted` retains the current plan's muted `version.unsupported` findings outside `List`; `All()` and individual details still expose them. By default, the CLI, MCP, and summaries report the muted count (CLI `ignored-version-warnings`). Neither `--all` nor subject or code filters automatically show these findings. An explicit `--include-muted` or `include_muted=true` includes them with a marker. See 05 §4.2 and §5.2 for the commands and tools.
- Exclude muted reminders from finding counts, Mod row icons, pre-launch prompts, and pack build `Mismatch` warnings. Also exclude them from the counts of undeclared versions in a plan's allowed-version summary, and record the muted count separately (04 §`1.6`). Declaration facts, the allowed-version intersection and default, the pack version intersection, pack and game major.minor restrictions, and file integrity checks are unaffected.
- Restore an individual Mod's mute directly, without confirmation. Batch muting through summaries or manifest groups requires confirmation. See 06 §5 for the interface entry points and individual wording.

### 4.3 Finding Catalog

**Declaration layer** (computed on demand)

Link findings query `InspectGameLinks` each time, independently of the declaration, `Def`, and Patch caches. A target is not considered foreign just because it is in a player directory. Inspect the first level of each `Mods/` directory and each link target on disk. Instance names and registered real paths are cached with the Mod index using `Store.IndexKey` (07 §8.3). First compare the link target with the Instance's scanned real path and confirm on disk that the Instance still resolves there. Re-resolve every Instance path only if they do not match.

| `code` | Severity | Detection and suggestions |
| --- | --- | --- |
| `entry.missing` | warning | A plan entry matches no installed Mod but is retained in ModsConfig.xml. The subject is `pkg:<id>`. Suggest `archo find <id>` and `archo disable pkg:<id>`; add `subscribe: <link>` if community data identifies the Workshop item. |
| `mods.link-dangling` | error | A link with the owned prefix is at the first level of a known game's `Mods/`. Its target is confirmed missing, and its source is accessible. The subject is `path:<full-path>`, with `game:<game-path>`; no Instance is required. Block the selected game's launch and test run, but only summarize findings for other games. See 08 §3 for repair. |
| `mods.link-foreign` | warning | The target does not match a known Instance or pack identity, or cannot be mapped. Show the original target and ownership evidence. Retain valid env links; the prefix alone never authorizes deletion. |
| `mods.link-check-failed` | error | Access denied, an offline disk, an unknown reparse tag, or another failure prevents a judgment. Report the failure and retain the link. Complete the inspection before launching the selected game. |
| `mods.link-occupied` | warning | A real directory or file occupies a name with the owned prefix. Report it without offering a deletion action. |
| `mods.link-recovery` | error | The persistent ledger has an unfinished operation. Show the actual state and recovery action read-only, and complete the operation after confirmed link repair. Do not change the disk automatically on startup. |
| `mod.unloadable` | error / info | **error**: an enabled Instance will not load in the game because it is unloadable, residue, or displaced by another copy of the same package. Suggest disable. **info**: an installed, disabled Instance is unloadable for a reason other than about-disabled (02 §4.2). |
| `mod.pinned` | info / warning | A locked version (always "locked version" in player text; retain the code name). The current implementation recognizes a Workshop copy with `About.xml.disabled` and another live, loadable, non-Workshop Instance in the same Lineage. RimCrow `_<Workshop-ID>_` copies, `copy-local` copies, and copies deployed into `Mods/` count. **info**: the copy is effective, and the Workshop copy is upstream; suggest `archo unpin`. **warning "upstream has an update"**: the Workshop semantic hash differs from the copy, and ACF timeupdated is later than the copy's first appearance; suggest `archo mod <copy> --diff <Workshop>` and `archo unpin`. For RimArcho-made locked versions whose Workshop original is not hidden (the lock library, 04 §7), warn when the upstream snapshot differs from `origin.json` `origin_snapshot` and suggest `archo pin --update <copy>`. Do not also report `mod.unloadable` for locked versions. |
| `mod.hidden-residue` | warning | A Workshop source has a renamed About.xml and is absent from the ACF subscriptions: it is unsubscribed residue. Suggest `archo trash`. |
| `mod.hidden` | warning | Another Mod is hidden by renaming, usually by another manager. Suggest `archo unhide`, `archo trash`, or ignoring the finding to keep it hidden. Recompute the classification for each analysis without persistence. See 08 §6 for actions. |
| `mod.replaced` | warning | Community Use This Instead data (`replacements.json`, obtained through `archo data update`, 04 §6) identifies a maintained replacement for the current game version. If it is installed, suggest disabling the old Mod and enabling the new one; otherwise, provide a Workshop link. |
| `dep.missing` | error | The required package, including dependencies in `modDependenciesByVersion` for the current version, is absent from this environment. Use separate wording for missing DLC. Suggest `subscribe: <steamWorkshopUrl>` and `steam://url/CommunityFilePage/<id>`. |
| `dep.inactive` | error | The required package is installed but disabled. Suggest `archo enable` for each candidate. |
| `order.violated` | warning / info | The current order violates a constraint (loadBefore/After, a dependency, a community rule, or a user rule) that is not in a cycle. Emit one finding per edge, with key `<before-package>><after-package>` and declared, community, or user evidence according to the constraint kind. Severity follows constraint strength (04 §5.3): dependencies, loadBefore, forceLoad*, and strong user rules are warning; loadAfter, weak community rules, and constraints whose violation is harmless in the corpus are info. Suggest `archo sort --dry-run`, `archo move <A> --after <B>`, and `archo rule add ignore <ref>`. |
| `official.order` | warning | Core and DLC are not at the front of the list or not in release order. Combine all edges violating the built-in official order into one finding. The common corpus case is DLC enabled after the list was built and appended to the end (Odyssey is too late). Suggest the minimal `archo move` first, returning the newly enabled DLC to after the preceding official package (04 §5.3), then `archo sort --dry-run`. |
| `order.cycle` | info | The sorting constraints form a cycle. List each edge and its source. The game warns rather than sorting, so a cycle does not block writing or launch. See 04 §5 for proposed ways to break it and their ranking. The info basis is in §4.3.1. The can it run page still lists it, muted and uncounted, because only the player can decide which constraint to suppress. |
| `incompat.active` | error | Two `Mods` declared incompatible are both enabled. Suggest disabling one and `archo why A B`. |
| `version.unsupported` | warning / info | `supportedVersions` omits the current major.minor version. Do not check official content or `Mods` without this field. A Mod that stops at the previous minor version (game 1.7, Mod through `1.6`) is warning; one listing only earlier versions is info (log corpus, §4.3.1; the reverse of the old rule). |
| `pkg.duplicate-active` | error | Multiple Instances with the same `packageId` are enabled. All enabled copies are subjects. Suggest retaining only the first. |
| `pkg.duplicate-installed` | info | Multiple Instances have the same `packageId`, with at most one enabled. Mark the copy ignored by the game in the evidence. Suggest `archo swap`. |

> Declaration violations are common and mostly harmless: half of the corpus logs violate dependency or load-order declarations, and 45% violate community rules. Above 100 `Mods`, logs with and without violations have the same error frequency. The most frequently violated constraints rarely involve the `Mods` identified as error sources; some constraints (modoptionssort before Core) cannot be satisfied at all. Sorting must therefore distinguish declarations that predict errors from declarations that are not enforced, with severity determined by the constraint kind (04 §5.3). See 07 §6 for the numbers.

The old `replacement.available` finding is no longer emitted; `mod.replaced` replaces it.

**Log corpus layer**. This is part of the declaration layer and is computed for the current enabled list. Its data is in `internal/known/known.json`, generated by `spikes/knowndeps` from 40,326 player-shared `1.6` logs. It contains only packageIds, counts, and error templates, with at least 5 distinct players per entry. The evidence source is community. The data is embedded with the program, like `internal/order/observed.json`: `archo data update` fetches only third-party data, and RimArcho does not yet have its own data repository.

| `code` | Severity | Detection |
| --- | --- | --- |
| `dep.observed-missing` | warning / info | A is enabled, B is disabled, and A's About.xml does not declare B. At least 25% of corpus logs with A enabled and B disabled report a missing type, parent `Def`, `Def`, or texture supplied by B. Suggest enabling B if installed, or provide its Workshop link. Use warning if at least half of the logs report the error and it is not merely a missing sound or texture; otherwise, use info. Evidence includes counts, missing items, a typical error, and whether the error disappears in the same player's subsequent uploads after enabling B. |
| `combo.known-bad` | warning / info | Errors attributed to A mainly occur with B enabled (≥ 30% of logs with both enabled, less than one-quarter of that rate with A alone), or mainly with one version of A (About.xml `modVersion` or the `About/Manifest.xml` version, exactly what HugsLib records). Use warning at ≥ 50% unless the error is a missing sound or texture; otherwise, use info. |
| `mod.known-error` | info | At least half of the shared logs enabling A report the same error, independently of the rest of the list. This indicates a known error in the Mod itself. |

Severity basis: when a declared dependency is disabled (`dep.inactive`), the subject Mod is an error source in 18% of cases, versus a 7.5% chance baseline (decisions-corpus §3.7). By construction, this layer's entries have their errors in at least 25–30% of the corresponding logs. However, these are observations rather than declarations, and the author may intend the dependency to be optional, so the maximum severity is warning.

**`Def` layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `def.override` | info | Multiple `Mods` define the same type and `defName`. The later definition in load order wins; identify it. Skip abstract `Defs`, duplicates within one Mod, and `Defs` with unmet `MayRequire` conditions. |
| `def.parent-missing` | error | `ParentName` has no matching `Name`. Check patched documents **after** Patch simulation (§5.1). Treat executed custom classes (`patch.unknown-op`) as successful and their XML text values as added `Name` values, suppressing false reports (Chimera Creation (Continued) adds Anomaly `Name` values through a custom class). The confidence is likely. |
| `def.abstract-collision` | warning | Two or more `Mods` define abstract parents with the same `Name`. |

**Missing parent template help** (`app.ParentHelp` / `FindParentHelp`). A `def.parent-missing` finding means the affected content cannot load. Aggregate the affected types and counts by subject Mod. To find providers, first query the stored `Name` index (`Store.DefsNamed`). Offer enable commands for installed, loadable, disabled definers when the analysis and content key are fresh, enabling the Mod does not change its conditional directories, and the definition's conditions are met.

Only when the player opens the finding's details, or the CLI or MCP requests an individual finding, search the effective `Defs` of installed, loadable, disabled `Mods` for `Names` with no cached provider (`modsearch.FindNames`). Do not run this search during page load. Fresh facts already index the `Name` of every loadable Mod, including those deployed through links. The fallback reads only `Mods` whose facts may differ after enablement: those with a stale content key or conditional directories that would change. If there are no such `Mods`, read no files and conclude that no installed Mod, including disabled ones, provides the template. Apply the current game version, LoadFolders, and `MayRequire` conditions as they would be after enabling that Mod. Match only the `Name` of a top-level `Def`, not a `defName` or nested property. Do not access the network or write to the analysis cache. The search has a deadline and limits on the number of `Mods`, directory entries, files, and single-file and total bytes (see the implementation). It supports caller cancellation. If a limit is reached, a file is unreadable, or XML is invalid, retain the results and state that the search is incomplete; do not treat "not found" as "absent". The GUI uses an independent read session without holding navigation, edit, or log read locks. See 06 §5 for cancellation.

Uninstalled candidates come from the subject's dependency declarations (including those for the current game version), loadAfter and forceLoadAfter (including those for the current version), and cached `Def` `MayRequire` and `MayRequireAnyOf` values. If a declared URL or the existing community Steam DB package index identifies the Workshop ID, offer a subscription and page link; otherwise, offer a Workshop search. These are candidates, not confirmed providers of a template. Omit the candidates if every missing `Name` already has an installed provider. Always offer the subject's Workshop requirements link, falling back to a name search if there is no Workshop ID. Explain that the finding may concern harmless optional content, and offer a group ignore, only when existing, attributed logs matching the current list contain no error or warning involving that Mod. Continue to apply §4.2's update-expiry rule to each finding, without introducing a new ignore format. See 06 §5 and 05 §3.2 for the interface and command output.

**Patch layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `patch.no-match` | warning | A top-level operation fails in simulation, at the layer the game logs. Emit one finding per failed top-level operation, with reasons (§4.4). |
| `patch.contested` | info | Two or more `Mods` use `Replace` or `Remove` on the same node. The node key combines the `Def` and element path. |
| `patch.unknown-op` | info | A custom PatchOperation cannot be simulated. Emit one finding per Mod listing its custom classes, and treat these operations as successful. |

**Harmony layer**

| `code` | Severity | Detection |
| --- | --- | --- |
| `harmony.shared-target` | info | Two or more `Mods` patch the same method. Emit one finding per method. |
| `harmony.skip-prefix` | info, may become warning | A bool-returning prefix may skip an original method that other `Mods` also patch. The confidence is possible. See §4.5 for escalation. |
| `harmony.multi-transpiler` | info, may become warning | Two or more `Mods` have transpilers on the same method. See §4.3.1 for the severity basis and §4.5 for escalation, which follows skip-prefix. |
| `harmony.manual` | info | An assembly calls `Harmony.Patch`. Targets resolved statically from IL become ordinary patch facts (§5.3). Explain how many calls were resolved and why others were not; do not misreport that there are no patches. |

**Log layer**. Emit one `log.<category>` finding per error cluster (see §6.1 for the categories).

| `code` | Severity |
| --- | --- |
| `log.exception`, `log.harmony-stack`, `log.xml-load` | error |
| `log.patch-failed`, `log.xref-missing`, `log.material`, `log.texture-missing`, `log.save-load` | warning |
| `log.mod-report`, `log.other` | info, excluded from finding list |

#### 4.3.1 Severity Basis (Log Corpus, G4/G8)

Severity measures how strongly a finding predicts errors in the game log. It was tested with 6,734 public RimWorld `1.6` logs (docs/research/decisions-corpus.md §3, §2 G8). For each finding, check whether the involved `Mods` are attributed as error sources in the same log. Compare that rate with the rate for randomly choosing the same number of `Mods` (observed versus chance).

The rules are:

- **error**: the game will definitely have a problem (unmet declared dependencies, a declared incompatibility, or a missing parent `Def`), and the observed rate is clearly above chance (about 2× or more). Errors block pack building (08 §5.2), so this severity excludes problems that are merely possible.
- **warning**: the observed rate is about 2× or more, and the player can act on the finding.
- **info**: the rate is near or below chance, or the findings are numerous, normal forms of coexistence (overrides and contention) with no useful action for an individual finding.

| `code` | Severity | Observed / chance | Reason |
| --- | --- | --- | --- |
| `patch.no-match` | warning | 37.3% / 4.5% (8.3×); `patch-failed` errors 29.7% / 0.9% | This is the strongest error predictor. Keep it below error: 70% have no matching logged patch failure (e.g. optional targets), and escalation would prevent half of the lists from building packs. |
| `def.parent-missing` | error | 16.3% / 4.1% (3.9×) | Retain the severity. |
| `incompat.active` | error | 26.4% / 7.3% (3.6×) | Retain the severity. |
| `def.abstract-collision` | warning | 25.4% / 8.3% (3.0×); 3.2× excluding Harmony stack errors | Retain the severity. |
| `harmony.multi-transpiler` | info (formerly warning), promoted to warning when in a log stack | Before G9: 20.3% / 7.0% (2.9×, full corpus); the same 669 logs after G9: 20.7% / 8.9% (2.3×); 7,291 G9 additions: 2.2×; excluding Harmony stack errors: 2.3× | Resolving manual patches increased the count from 4,559 to 11,078 (17 per log), dropping the ratio below the same-layer info findings `harmony.shared-target` (2.6×) and `harmony.skip-prefix` (2.5×). The rate at which patched methods appear in error stacks (0.5%) matches shared targets (0.6%), indicating that these `Mods` have many patches rather than transpiler conflicts. Lower the severity to info and exclude it from pre-launch prompts. As with skip-prefix, promote it when the patched method occurs in a current error cluster (§4.5). |
| `dep.inactive` | error | 18.1% / 7.5% (2.4×) | Retain the severity. |
| `dep.missing` | error | 9.9% / 4.4% (2.3×, 242 dependencies genuinely absent from player lists) | Retain the severity. Another 10,941 entries reflect dependencies absent from the research library that players actually had (1.6% / 3.3%). That research artifact is unsuitable for setting severity; its below-chance ratio confirms that the method does not count unrelated findings as related. |
| `version.unsupported` (previous version only) | warning (formerly info) | 10.4% / 4.0% (2.6×) | Code compiled for the previous game version can fail when the new version changes an API. |
| `version.unsupported` (older versions only) | info (formerly warning) | 0.8% / 3.2% (0.3×) | Old `Mods` still in use are those that survived several versions; broken ones were abandoned. |
| `official.order` | warning (formerly error) | 7.4% / 3.6% (2.1×) when the involved `Mods` are considered; paired comparison in 04 §5.3: 1.8× | This is comparable to dependency-order violations (1.4×, warning), rather than a certain game error. The overall 2.3% / 2.5% is unusable: half of the subjects are DLC, and Core and DLC are never attributed as sources. |
| `order.cycle` | info (formerly warning) | 6.8% / 6.1% (1.1×) | Cyclic lists run normally, with an error rate matching chance. Sorting drops the weakest constraint in the cycle (04 §5.2). |
| `patch.contested` | info | 41.9% / 6.6% (6.3×); without `patch.no-match` on the subject: 21.3% / 6.6% (3.2×) | Retain info. The high ratio mainly comes from no-match findings in the same `Mods`, already reported by `patch.no-match`. The remaining 3.2× resembles `def.override` (3.0×, info): shared edits are normal coexistence, with about 70,000 entries and 18 per log, and no useful action for an individual finding. |
| `def.override` | info | 24.6% / 8.1% (3.0×) | Retain the severity for the same reason as above (700,000 entries). |
| `harmony.shared-target`, `harmony.skip-prefix` | info | 2.7×, 2.6× | Retain the severity: these are the most numerous findings (1.39 million and 870,000), with no useful action for an individual finding. Log evidence promotes skip-prefix to warning (§4.5). |
| `harmony.manual`, `patch.unknown-op` | info | 1.6×, 2.5× | Retain the severity: these describe incomplete analysis, rather than problems themselves. |

Pre-launch findings (`LaunchChecks`, 08 §2) are selected using the observed and chance values in this table, rather than severity.

### 4.4 `patch.no-match` Reasons

Classify the reasons at the innermost failed operation. An operation may have multiple reasons.

| Reason | Detection |
| --- | --- |
| `target-inactive` | An installed, disabled Mod defines the target `Def`. Query all installed `Def` facts. |
| `target-missing` | No installed Mod defines the target. |
| `target-later` | The XPath matches after all patches have run: a later Mod's **patch** creates the target. Identify the creator by node allocation ranges. The game merges all `Defs` before applying patches, so a definition in a later Mod alone never causes a failure. |
| `node-absent` | The `Def` exists, but the rest of the XPath does not. |
| `wrong-version-route` | The target is on disk but outside the effective content. Search inactive XML in the patching Mod and its expected targets. |
| `stale-name` | The expected target Mod is enabled but lacks the `defName`. List up to 3 similar names from that Mod by edit distance. |
| `unguarded-optional` | Only occurs with `target-inactive`: there is no guard, and the definer is not a hard dependency. |
| `exception` | The game would throw an exception, e.g. for a tree operation on an attribute node. |
| `unclassified` | The XPath contains no `Def` name to classify. |

**Guards** include the match branch of `PatchOperationFindMod`, `Conditional`, `Sequence` children with `MayRequire`, and conditional loading folders (§1). Ignore top-level `MayRequire`, as the game does, and explain this in the details.

**Expected targets** are the names in `FindMod` (display names, or directory names for DLC), `MayRequire` packages, conditional-folder packages, and hard dependencies other than Harmony and Ludeon.

### 4.5 Default Display

Each entry point shows a different default subset of the same findings. Usually, `--all` or filters show the full list; muted version reminders require an explicit `--include-muted` (§4.2).

- **Hide `def.override` and `patch.contested` by default** in problems output and interface finding lists. End the list with `hidden: N def.override, M patch.contested (archo problems --all)`. Show these findings with `--all`, `--code`, `--subject`, `--severity info`, or `--min-severity info`. They are always visible in def and patches output and in the details.
- **Collapse `version.unsupported`**: combine unmuted reminders for the same game version (§4.2) into one line, using the highest severity: `<sev> version.unsupported  N enabled mods do not list 1.7 (a list 1.6, b older only)  (archo problems --code version.unsupported)`. The CLI, MCP, and interface summaries are the same. Mod row tooltips and details state only the versions that the Mod declares (06 §5).
- **`harmony.multi-transpiler` defaults to info**, with the same escalation rule as skip-prefix below. It is not collapsed.
- **`harmony.skip-prefix` defaults to info and is collapsed**: combine its info findings into one summary (`… N bool prefixes from M mods can skip methods other mods also patch; none is in a log stack (archo problems --code harmony.skip-prefix)`). Promote a finding to warning only if the patched method, without its overload portion, appears in an error cluster of the **current, non-stale problem log**, either as a stack frame or as the owning frame of a Harmony patch line. Add the cluster as log evidence and put `archo log --cluster E<n>` first in the suggestions. List escalated findings individually. Show the full list with `--code harmony.skip-prefix`, `--all`, `--subject`, or `harmony --target`.
- Put summaries first within each severity so that the default 30-row truncation does not hide them.
- **MCP `list_problems`** defaults to `--min-severity warning` unless severity, `min_severity`, all, or `include_muted` is supplied.
- **Log findings** are grouped by log (§6.4). Show the first 5 findings from the current log by default; show the rest with `archo problems --code log`. Collapse stale logs to an uncounted title, expanded with `archo problems --code log --all`. Exclude `log.mod-report` and `log.other` from the finding list.
- **Ignored findings** are excluded from the default list (§4.2).
- **The can it run page** includes only errors and warnings the player can act on, leaving info findings in the technical details. See 06 §5 for the codes, grouping, and wording.

## 5. Analyzers

### 5.1 Def Layer

- Parse the XML as a stream, extracting only the fields in §2 without retaining a full DOM.
- Resolve inheritance after all patches have been applied, as the game does. Patches can change `Name` and `ParentName`; for example, `AttributeAdd` can name Core `Defs`. First collect all `Name` values in the patched documents, then resolve `ParentName`. For abstract nodes with the same name, the later node in load order overrides the earlier one. Checking before patches produces false positives (see 07 §6 for measurements in a real environment).

### 5.2 Patch Simulation

1. Merge each Mod's effective `Defs` into one in-memory document with a `<Defs>` root, in plan order.
2. Apply each Mod's PatchOperations in the same order, recording how many nodes each operation matches.
3. Support `Add`, `Insert`, `Remove`, `Replace`, `AttributeAdd`, `AttributeSet`, `AttributeRemove`, `SetName`, `AddModExtension`, `Sequence`, `Conditional`, `Test`, and `FindMod`. Record other classes as `patch.unknown-op` and treat them as successful.
4. Interpret the success attribute (`Always`, `Normal`, `Invert`, `Never`) using the game's semantics.
5. The failure key combines the file and the game's description of the operation (`FailKey`), matching the failure messages in `Player.log` (§6.2).

**XPath**. The engine must match .NET XPath 1.0 behavior. Test compatibility with all XPath expressions in the real Mod collection and replay `Player.log`, simulating every logged failure. The mismatch rate must be below 0.5%, with differences documented. See 07 §6 for the measurements. The common `Defs/XDef[defName="…"]` form uses an indexed fast path.

**Memory and parsing**. The merged document exists only in the subprocess.

- Node storage (`internal/xdoc`) uses 44-byte structs in 64K-node blocks. The first block grows like a slice so that small documents stay small. Links use int32, and element names go in a `Names` table shared by the merged and patch documents.
- Text goes in 1 MiB append-only pages; text of ≥ 64 KiB is allocated separately. Do not use a single growing text buffer: references from index keys would retain its old backing arrays.
- The parsing pipeline uses up to 8 `encoding/xml` workers to tokenize files, with at most twice the worker count of files in memory at once. Documents are built sequentially in load order.

### 5.3 Assemblies and Harmony

Read .NET metadata natively in Go, without a .NET runtime (`internal/dotnet`, `internal/harmony`). Read `Assemblies/*.dll` in the effective directories of enabled Instances.

- Read the `TypeDef`, `MethodDef`, `MemberRef`, `TypeRef`, and `CustomAttribute` tables and the `#Strings` and `#US` heaps.
- **Targets**: merge the class-level and method-level `[HarmonyPatch]` arguments: the target type, method, `MethodType`, `Type[]` parameters, and named arguments. Mark `TargetMethod(s)` patches as dynamic targets.
- **Patch kinds**: use the `[HarmonyPrefix]`, `[HarmonyPostfix]`, `[HarmonyTranspiler]`, or `[HarmonyFinalizer]` attribute. If none is present, infer the kind from the method name `Prefix`, `Postfix`, `Transpiler`, or `Finalizer`.
- **Possible original-method skip**: a prefix that returns bool may skip the original method.
- **Manual patches** (corpus G9, `internal/harmony/manual.go`, `internal/dotnet/il.go`): when an assembly has a `MemberRef` to `HarmonyLib.Harmony::Patch`, abstract-interpret each method's IL to resolve the original and patch methods in `Harmony.Patch` calls. Wrappers defined in other assemblies with parameters like `(MethodBase, HarmonyMethod, …)` also count. Resolved targets become ordinary patch facts (target, kind, and patch method), participating in shared-target and multi-transpiler findings just like attribute patches. If the original method cannot be resolved, record a dynamic `<manual: reason>` fact with the known patch method. `harmony.manual` explains how many calls were resolved and why the others were not. The following remain unresolved: originals selected at runtime (whole-game scans such as HAR `BodyReferenceTranspiler`), collections built by helpers, `Assembly.GetType`, and base methods referenced through subclasses (the headings use the declaring type).
- **HugsLib**: read `HugsLib.dll` normally in its own Mod (`unlimitedhugs.hugslib`) and attribute its patches to that Mod. Skip only the copies in other `Mods` (`harmony.OwnLibrary`).
- **Harmony ID candidates**: heuristically identify `#US` strings passed to calls such as `new Harmony(...)`; in practice, these are ID-like reverse-domain strings such as `a.b.c`. Use them to map log patch owners to `Mods` when the owner is not a `packageId`.
- **Skip list** (`harmony.DefaultSkip`, a case-insensitive glob): `0harmony.dll`, `harmony.dll`, `hugslib.dll`, `0multiplayerapi.dll`, `0prepatcherapi.dll`, `mono.cecil*.dll`, `system.*.dll`, `microsoft.*.dll`, `newtonsoft.json.dll`, `unityengine*.dll`, `assembly-csharp*.dll`. Skip Harmony facts for these libraries but still index `TypeDef`; frames in them are library frames. The analysis input can replace the list (`Input.HarmonySkip`); there is no player setting yet.
- The fixtures in `testdata/dotnet` contain fake `0Harmony` and `Assembly-CSharp` libraries, C# sources for four `Mods`, build scripts, and committed DLLs (4–5 KB each). Tests need no .NET SDK.

### 5.4 On-demand Decompilation

Use ilspycmd only when requested. The player may explicitly queue decompilation of the current list's Mods as a background task (06 §2.16); nothing decompiles automatically. ilspycmd exits when its work finishes.

- **Discovery**: look in `ARCHO_ILSPYCMD`, PATH, and `~/.dotnet/tools`. If ilspycmd is not found, report an error and provide `dotnet tool install --global ilspycmd`. Never install it automatically.
- **Reference directories**: use the game's Managed directory (the game root in 1.7), the DLL's directory, and the directory containing 0Harmony.dll.
- **Source library**: generated code is durable, regenerable, local-only data in the managed `source` directory (layout: 07 §3; relocation and cleanup: 06 §2.14). Cache clearing never touches it. On first opening after the upgrade, discard the old `<home>/cache/source` once; retain its history through the store migration.
- **Identity**: `source_library` maps the full DLL SHA-256 to a library-relative folder, kind, package ID, DLL name, game build, ilspycmd version and creation time, with stored size accounting. Game identities also include the build, so equal bytes from different releases can retain distinct build folders. `source_use` replaces `source_cache`, preserving historical attempts and recording every `(hash, instance)` use: project/type attempts and successful shared hits. Upserts retain the latest status, mode and time per pair. History does not assert that output still exists.
- **Publication**: decompile into a unique temporary folder, write the completion marker there, then publish by rename. Type-only output lives under the DLL folder's `.types/<type-key>/`; once the project exists, use its files and discard type-only output. Concurrent requests share an artifact lock; moving and removal require exclusive library access. A changed DLL gets a new identity; old code remains until explicitly removed.
- **Game code**: on demand, read the configured environment's `Assembly-CSharp.dll` from its Managed folder (or game root in 1.7), with that directory as a reference. Store it per configured game build; a historical Mod-analysis version override does not select a different game installation. Packs, feedback attachments and list exports never include library contents.
- **Search**: group requested copies by DLL identity before dispatch and search each generated folder once. CLI/MCP return one set of matches with library paths and “same code in: …” attribution naming sharing copies. GUI/TUI attach the shared project matches to each matching copy in their streaming tree. A CLI/MCP dispatch budget stops new work only; already dispatched assemblies finish. Explicit cancellation stops ilspycmd and further file reads. Defaults and command shapes belong to 05 §4.2 and §5.1.
- **Unused**: a Mod artifact is unused when no installed or locked copy contains its DLL hash, including inactive copies and other version folders. A game artifact is unused when no configured environment has its build. Check current DLL bytes across registered sources when cleanup is requested; abort if an installed source cannot be checked. Preview folder count and bytes, then remove only on explicit confirmation; never sweep automatically.
- MCP `read_source` with `search` defaults to `--implicated`. Only `scope: "active"` extends it to all enabled `Mods`. The default budget is 10 seconds. If it is exceeded, state how many assemblies still need decompilation ("still computing; run again").
- `archo files search` skips `Languages/` by default because translation files repeat every key; include them only with `--languages`. Traverse by Mod and use 8 workers to read files, preserving traversal order. See 05 §4 for command details and §5.5 for scope rules.
- **GUI/TUI**: Mod library › search files uses `App.SourceProjects` and `view.FileSearchRun` without a dispatch budget. Cached identities are visited first; missing projects are decompiled sequentially and retained. Stop, replacement and leaving cancel the context. Core in scope selects the game assembly (`FileSearchReq.Game` remains available to backend callers). The page contract, limits and polling are in 06 §2.9 and §2.16.1 G17.

### 5.5 Search Files Within Loaded Scope (2026-10-24)

The `files search` and `search_files` commands search only what the game actually loads and return results in load order (`internal/modsearch`):

- **Mods**: by default, search the current plan's enabled entries, using the exact copy in the list (Workshop, local, and locked versions are distinct), with its list position. Use `--mod` (repeatable) to name other copies; if several copies share a package ID, use the one in the list. Official content (Core and DLC) is included by default; `--game without|only` excludes it or searches only it.
- **Folders**: resolve each Mod's loading folders using §1: the version tags and conditions in `LoadFolders.xml`, or otherwise the version directory, `Common`, and the root. Within each loading folder, traverse only the subdirectories the game reads: `Defs`, `Patches`, `Languages`, `Textures`, `Sounds`, and `Assemblies`, plus `Lunar/Components`. Read `About/` only from the root. Do not search other folders under the root. For the same relative path, search only the copy with the highest priority. With `--all-versions`, search all version directories and every folder in `LoadFolders.xml`, ignoring conditions and retaining duplicates; mark each result with its folder. Silently skip nonexistent folders named by `LoadFolders.xml`. Count unreadable files and report the first error.
- **Kinds**: the default is `defs,patches,about`. Select kinds with `--kind`; add translations only with `--languages` or `--lang <language-folder>`. Match `Textures`, `Sounds`, and `Assemblies` by relative path without reading their binary content.
- **Matching**: the default is a case-insensitive literal match. Support `--regex` (RE2), `--case`, and `--word`. Count each line as one hit.
- **Results**: group by Mod (position, reference, name, package ID, copy, and loading folder), then file, then line. Hits in `Defs` and `Patches` include the containing top-level `Def` (`ThingDef/name`, or `ThingDef@Name` for abstract definitions) or the containing patch operation's XPath. Retain 5 lines per file and 40 lines per page by default (`--per-file`, `--limit`, `--page`); totals always cover all hits. `--mode files|count` lists only files or the count per Mod. If the default scope has no hits, explain which scopes were excluded.
- **Def mode** (`--def`): the pattern is a defName, matched as a case-sensitive whole word. By default, search only `Defs` and `Patches`. Each hit has a role: `def` (`<defName>`), `abstract` (`Name="…"`), `child` (`ParentName="…"`), `patch` (the containing operation's XPath includes the name), `patch-value` (the name is a value in a patch), `translation`, or `ref`. Precede the hits with a summary: definers (identify the later, effective definition if there are several), parents and children, patch operation and Mod counts, and the number of referencing `Mods`. This is textual evidence; use the `get_def` or `get_patches` simulation to determine whether a patch actually matches.

## 6. Log Attribution

### 6.1 Parsing

**Two formats** are supported:

- In `1.6` and earlier (`Unity`), messages have no level. Read the version from `RimWorld X.Y.Z revN` in the first 50 lines.
- In 1.7 (`Waylith`, identified by `=== Waylith log ===` within the first 50 lines), each message has the form `<date> <time> <level> [<category>] <message>`, followed by an exception stack, if any, and a call stack. Read the version from `Version: X.Y.Z.N`. Collect only `WARN`, `WARNING`, `ERROR`, `FATAL`, and `CRITICAL`; exclude `INFO`, `DEBUG`, and `TRACE` from error clusters. Discard the logging system's call frames (`Brutal.Logging.*`, `Verse.Log`) and mark the others as caller frames, separate from exception frames. A message with only a call stack is not an exception. Rewrite `- PREFIX owner: Target.Method <- Ret Type:Method(…)` into `Unity` format. Strip the leading dot and `_PatchN` from .NET 10 `.Ns.Type.Method_PatchN(…)` frames and mark them as patched methods.

**Entries** consist of a message line and its following continuation lines.

- Continuations include lines beginning with whitespace; lines beginning with `[Ref `, `(wrapper `, `Rethrow as `, `--- End of`, `file: `, or `Subnode:` (and its next line); multiline .NET exception messages beginning with `Parameter name: `, `Actual value was `, or `Object name: `; `Mono` native frames such as `<0x… + 0x…> <unknown method>`; and bare frames without `at ` from inner exception stacks.
- A `(Filename: ` line or a blank line ends the current entry. Truncate lines longer than 4 KiB.
- `[Ref X] Duplicate stacktrace` refers to the earlier original stack with the same Ref.
- Combine an XML Extensions block (`[Mod - Start of stack trace]` … `[End of stack trace]` … `Source file: <path>`) into one entry: `[Mod] <first-failure-line>`. Treat the rest as continuations and the path as `file:`.
- The entry's location frame (`Top`) is the first exception frame, or the first caller frame (the code writing the log) if there is no exception.

**Exclude engine or framework status lines from error clusters** when they have no exception stack. These include shader fallback messages (`Shader '…': fallback shader '…' not found`), translation totals (`Translation data for language … has N errors`), XML Extensions statistics (`[XML Extensions] N total patch operations run`), `Curl error N: Callback aborted`, and `Mod … is missing supported versions list in About.xml`. These rules come from the corpus. Other `Unity` entries count as problems only if they have an exception stack, a noninformational category, or one of these terms: error, exception, failed, could not, couldn't, invalid, missing, or not found.

**Categories** are tested in this order:

| Category | Detection |
| --- | --- |
| `patch-failed` | The message starts with `] Patch operation ` or `Patch operation `, contains `Verse.PatchOperation…): Failed` or `Verse.PatchOperationSequence: Error in the operation`, or reports a `Verse.PatchOperation…` failure in an XML Extensions block. |
| `xref-missing` | `Could not resolve cross-reference`, `Could not load reference to`, or `Failed to find <X>Def named …` |
| `material` | The message contains `_MainTex`, or the stack contains `ResolveIcon`. |
| `xml-load` | XML errors, `Could not find parent node`, `Duplicate def`, `Could not find a type named`, `Config error in`, etc. |
| `texture-missing` | `Could not load Texture2D at`, `Failed to find any textures at`, or `Could not load Texture\|Material\|Shader\|AudioClip\|SoundDef` |
| `save-load` | An unresolved save reference: `Could not resolve reference to object with loadID`, `… is not deep-saved`, `Error in PostExposeData`, `Could not find think node with key`, `Exception loading … from save`, etc. |
| `harmony-stack` | The stack contains Harmony patch lines. |
| `exception` | There is an `Exception` stack, or the message contains `Exception`. |
| `mod-report` | The Mod reports on itself with a prefix such as `[name]`, `<color=…>[name]`, `name :: `, or `MOD: `, excluding level words such as `ERROR:`. |
| `other` | Everything else |

The `mod-report` and other categories are informational and do not appear in the finding list.

**Category is not runtime phase**. Names such as `save-load` describe message content only. Determine phases only from explicit Archo Monitor markers (§6.7.3–4), never from a category, stack, or vanilla text such as "Loading game…". `logs.Entry.Phase` is the latest marker before the entry. `logs.Cluster.Phase` retains the phase of the first occurrence, and `Phases` counts occurrences in each phase separately. Cluster keys and numbers do not change with the phase. Old logs without markers remain unphased.

**Clustering** follows these rules:

- Normalize the first 300 message characters: replace paths with `<path>`, addresses and long hexadecimal IDs with `<hex>`, and numbers with `#`.
- The key combines the category, normalized message, continuations for `patch-failed` (to distinguish files), and the location frame's `Type.Method`. Merge entries with the same key into `log:E<n>`, numbered by first appearance. Record the count and the first 5 line numbers.
- A **template** combines the category with a further-normalized message. Replace color tags, `defName=…`, quoted contents, names after `named / loadID / Type / field / at / for def / key`, the `Def` in "found to give to X", parenthesized type names, and `packageId` with placeholders. Templates count kinds of problems: each missing texture has its own cluster but the same template. Cluster keys retain these names because attribution needs them.

### 6.2 Evidence

Attribution uses the fact store: effective content, `Defs`, Patch simulation results, `TypeDef`, and `HarmonyId`. Match Mod names from About.xml; the game prints `[Name]`, not the player's aliases. Core and DLC (the `ludeon.rimworld` prefix) never become sources through text matches or `Def` ownership.

Use the following evidence, strongest first:

| Evidence | `Rule` |
| --- | --- |
| `[Mod name]` prefix | A self-reported issue identifies the Mod as source. XML Extensions uses full names (`[[FSF] Vanilla Bionics Expansion] Verse.PatchOperation…`). |
| File path | A `.xml/.dll/.png/.txt` path in the message or continuations inside an Instance directory identifies it as source. For logs from other machines, match Mod directories: `…\workshop\content\294100\<id>\…` or `…\Mods\<directory>\…`, including HugsLib-redacted `[Rimworld_dir]\Mods\…` paths. |
| Patch failure versus simulation | Match `patch-failed` to the simulated failure by `[Mod]\|description` or `file:` and mark the Mod as source, with `file:line` and §4.4 reasons. A disabled target provider is dependency, with `archo enable`. Match XML Extensions failures by XPath. If the operation no longer fails, state "the mod or the Loadout changed since the log". |
| `defName` (`xref-missing`) | Record the missing `def:<Type>/<name>` and list its enabled and disabled providers from the `Def` facts. A disabled provider is dependency, with `archo enable`; if none exist, state "providers: none among installed mods". For `Could not load reference`, explain that the error occurred while reading a save. **Wanter**: the enabled, nonofficial definer of the `Def` in "found to give to `<XDef>` `<name>`" is source. **Referrers**: search enabled effective `Defs` and `Patches` once for `>Name<` and `<Name>` (ThingDefCountClass lists such as `costList` and products). Skip Core and DLC and any `Mods` that define the name themselves. If over 3 `Mods` match, or the wanter rule already found a source, mark referrers only as mentioned. |
| `for def 'X'` in `texture-missing` | The definer of X is source. |
| File providers | For a missing texture with no enabled-Mod file reference, find installed nonofficial `Mods` with `Textures/<path>.*`. For nongame `Cannot resolve dependency to assembly 'X'`, find `X.dll`. A disabled provider is dependency, with an enable suggestion; an enabled provider is mentioned (perhaps the game does not read that loading folder). If none exist, state "providers: none among installed mods". If the assembly has no types and no enabled Mod loads it, list the installed DLL copies in the details. The scope is live Instances visible to the environment: shared folders (Workshop, SteamCMD, extra folders, Monitor, and the lock library), plus its own `Data` and `Mods` (02 §0, `Store.InstanceFiles`). Exclude other games' `Mods`. |
| `xml-load` | For `Could not find parent node named "X"`, an installed, disabled provider is dependency. For `Two ThinkNodes have the same unique save key`, enabled patchers of `ThinkTreeDef/<key>` are patcher. The definers of `<defName>` or `Name="…"` in `Context` or `Full node` are source. |
| `material` | Provide a checklist for a buildable `Def` with `uiIconColor` but no `uiIconPath`, `uiIconPathsStuff`, or `graphicData/texPath`, including abstract bases. Include `archo files search "uiIconColor"` and `"uiIconPath"`. |
| Missing types | For `Could not find type named NS.T` or `expected class 'NS.T'`, a disabled provider is dependency; an enabled provider is mentioned (a possible version mismatch). |
| Harmony patch lines | A line such as `- PREFIX <owner>: …` is patcher evidence. Match the owner by patch type, Harmony ID, then `packageId`. If no match exists, state "no installed mod matches". |
| Stack frame types | Match frames in nongame namespaces against `TypeDef`. The topmost Mod frame is source, and lower frames are mentioned. Game namespaces are `Verse.`, `RimWorld.`, `System.`, `UnityEngine.`, `HarmonyLib.`, `Mono.`, `Unity.`, `MonoMod.`, `Ionic.`, `LudeonTK.`, `Steamworks.`, `Waylith.`, `Brutal.`, and `RimWorldWaylith.`. |
| Message type names | A type name is mentioned evidence. In "instantiating a mod of type T" or "static constructor of T", it identifies the source. |

### 6.3 Roles and Output

**Roles** are source (produced the error), patcher (only patched a related method), dependency (a missing, disabled dependency), and mentioned (only mentioned). Rank candidates in that order. Never promote a patcher to source without its own frames or files. There is one exception to the ranking: for an installed, disabled `xref-missing` provider, dependency ranks before patcher. The missing `Def` belongs to the provider; `Mods` patching save or loading code have merely touched that code (VM 2026-10-28: a missing hairstyle in a save was previously blamed on Vanilla Expanded Framework, which patched `Pawn.ExposeData`).

**Confidence** depends on the first candidate. A source with file or patch evidence is certain; a source with other evidence is likely. A dependency ranked first under the exception above is likely ("a mod that defines it is installed but not enabled"); if later rules find source evidence, restore the source conclusion. Candidates without a source give possible ("no source identified"), and no candidates gives possible ("no mod identified"). Cached attribution reports carry `logCacheVersion`. Increment it when attribution rules change so that old logs are attributed again.

**Hot paths**. For stacks through `Verse.Pawn.Tick`, `Verse.ThingWithComps.Tick`, `Verse.Thing.DoTick`, `RimWorld.Planet.WorldPawns.WorldPawnsTick`, `Verse.Pawn.TickRare`, `Verse.ThingWithComps.TickRare`, `Verse.TickList.Tick`, or `Verse.ThingWithComps.CompTick`, include the tick-crash checklist: check for `AllComps` mutation during iteration and runtime comp addition without `InitializeComps()`; remember that patch lines show only who last touched the method; also inspect ExposeData, SpawnSetup, DeSpawn, and Destroy. Include `archo source search "AllComps.Remove(" --in …` and `"AllComps.Add("`.

**Each cluster produces one `log.<category>` finding**:

- The subjects are the candidate Instances, or `log:E<n>` if there are no candidates.
- The claim is `log:E<n> x<count>: <message>`.
- The evidence includes each candidate's `<role>: <evidence>` (log), plus category details such as reasons and providers (inferred).
- The first suggestion is `archo log --cluster E<n>`, followed by next steps such as `archo patches --mod <ref> --failed`, `archo harmony --target Verse.Pawn.Tick`, `archo source <ref> --type <type>`, `archo def <Type>/<name>`, or `archo enable <ref>`.
- The key is the cluster key, with absolute paths reduced to their final three segments (§4.2).

**Cluster details** (`archo log --cluster E<n>`, MCP, and the interface) include the complete message and continuations, plus messages around the first occurrence (nearby: 4 before and 3 after, ≤30 lines, truncated to 200 characters per message). This avoids another lookup in the raw log (corpus G7).

**Error patterns** (`pattern:<id>`, §6.8) appear on matching clusters as `[pattern:g:<id>]` at the end of the row and as a `pattern:` section in the details. Patterns provide background knowledge with community as the evidence source. They do not change attribution, roles, or confidence, or form standalone conclusions.

**Pasted text** (`AnalyzeLogText`, e.g. pasted stacks) uses the same parsing and attribution and is labeled `<text>`.

### 6.4 Which Launch Owns a Log

**First identify the game that wrote the log**. `App.CheckLogOwner` checks ownership before log selection, attribution cache reads, attribution, log-version analysis, or marking the log as seen. Prefer an explicit game path in the startup header and compare it with the environment's `game_root`. Currently, this recognizes the `Unity` `1.6` line `Mono path[0] = '<game_root>/…_Data/Managed'`. Accept only absolute paths; do not infer ownership from Mod errors or source paths in stacks. The repository's `Waylith` 1.7 sample has a version but no verifiable game path, so do not assume it has `Mono` lines. Without a definite path, determine the environment from the time-matched launch or ModsConfig write records below. If an explicit path conflicts with a record, use the path to identify the game and mark the plan as unknown. **A version mismatch alone does not identify another game**: the same game directory may have been upgraded. Another environment matching the log version is insufficient evidence to exclude the log. Without enough evidence, retain the existing unknown or stale handling; do not guess that another environment owns it.

**Other games' logs do not belong to this environment**. Exclude them before attribution, attribution cache reads, `EnsureLogAnalysis`, or seen-state writes. They must not appear in finding lists (including stale groups), sidebar counts, new-log notices, or errors from the last run. The game log page, CLI problems, log, and status commands, and corresponding MCP tools share these rules. Recheck an existing selection if another game has overwritten the file. Live tracking checks on each read and at the end, discarding clusters after an overwrite. **Explicit `log --file` and `--game-version` do not bypass ownership checks**. Refuse a file only when it was definitely written by another registered environment (`log belongs to environment <name>`). Explicit files belonging to this environment or with unknown ownership remain viewable; unknown files remain stale and do not trigger analysis. A header path outside all registered environments (shared logs, corpus logs, or this game before a move) means unknown ownership, not a foreign environment. `--game-version` changes the analysis version, not the environment identity.

**File selection**. For 1.7+, prefer `<user-data>/Logs/Player.log`; for earlier versions, prefer the user-data root (02 §2.3). Look for available `Player.log` and `Player-prev.log` files in both locations and exclude foreign files. If the current file is unavailable, the finding list, game log page, and default `archo log` use the available previous file. If neither is available, treat the log as absent. Packs use their own userdata in 1.7 and the environment's log in `1.6` (08 §5.3).

**Problem log**. The log used for findings is the player's selection (`archo log --select` or the game log page, stored in `log.selected:<environment>`), or the current `Player.log` if it is newer. **While the game runs**, the file being written by the current environment (`App.LiveLog`, the same as `RunLog`) belongs to this run, rather than the last run. Never use it as the problem log, even if selected. `Player.log` yields to `Player-prev.log`, where the game moves the previous log on startup. Apply this rule to cache-only `LazyLog` sidebar counts too, and restore normal selection after exit. A 1.7 pack writes its own logs and is unaffected. `archo log` still defaults to the live `Player.log` (2026-10-28: rechecking while the game ran previously read the live log as the last run, listing the same errors twice and doubling counts). The CLI problems and status commands and corresponding MCP tools are not lazy, but select the problem log by the same rule. To reduce idle memory, the interface normally parses the log only when can it run opens and reads only the cache beforehand (`App.LazyLog`, `App.LogCacheOnly`). Opening the page marks the log as seen (`log.seen:<environment>`). If an available `Player.log` is newer than both the last launch and the last seen log, the TUI shows "new log". The GUI reads it once in the background without waiting for the page to open (`API.ReadNewLog`, 2026-10-30, 06 §2.1), then releases it normally.

**Analyze using the log's own version**. If a log is confirmed to belong to this environment, its major.minor version differs from the environment's, and no `--game-version` was supplied, first refresh the analysis for that version (`EnsureLogFileAnalysis` → `EnsureLogAnalysis`). Attribute using the log version and show `analysed: as 1.6 (log version); environment is 1.7` in the heading. The same game directory can therefore analyze its own `1.6` logs after an upgrade from `1.6` to 1.7. Unknown files remain viewable, stale, and uncounted. They do not automatically trigger analysis for another version, though attribution may use stored facts.

**Ownership and staleness** follow these rules:

- Match each log by its write time to the **latest launch record** whose start time is no later than the log's mtime. Include other environments sharing userdata. For logs under 1.7 `Logs/`, exclude pack launches because packs write their own logs. Launch records contain the environment, plan or pack, time, and list fingerprint (08 §2). A log is stale if any of the following applies:
  1. Its major.minor version differs from the analyzed version.
  2. The matched launch is not for the analyzed plan (another plan or pack in this environment; foreign logs were already excluded).
  3. The launch list fingerprint differs from the current enabled list because the plan was edited after launch. Replacing only the Archo Monitor version in the same position (08 §9) does not count as an edit. Retrieve the list at the recorded launch or write version, verify its fingerprint, then compare each Monitor as the same item (`App.monitorSwapOnly`; VM 2026-10-28: a post-launch check updated Monitor, incorrectly marking the just-finished run stale and leaving only older logs).

  Without a matching launch record (for a Steam or external launch), use the latest ModsConfig write record before the log time, which identifies the writer plan (04 §4.3). If no record matches, mark the source as unknown and the log as stale, and explain this in the heading. Do not use the ModsConfig.xml mtime anymore.
- Exclude stale findings from problems, status, and interface counts and from skip-prefix escalation. Collapse them to a title by default. The `--code`, `--subject`, and `--all` filters show individual entries; `problems show` still works and names the log.

**Heading**. Show one heading per log with the filename, declared game version, write time, current or stale status with a reason (e.g. `written by 1.6, the game is 1.7`), and severity counts. Add `not counted` and an expand command for a stale log. List its clusters below the heading.

### 6.5 Caching

- Parsing and clustering are fast; referrer searches take most of the time (07 §6). Cache the **whole attribution report**, with one `log_cache` row per path and a copy in memory.
- The key combines the path, size, mtime, and an input fingerprint: `logCacheVersion`, `--game-version`, game major.minor version, analysis run, enabled list, and each installed Mod's ID, snapshot, name, alias, `packageId`, and directory. Reparse if any of these changes.
- The status and mod commands (MCP status and `get_mod`) read only the cache and never parse the log. A missing cache marks the log Pending; status explains that its errors are not counted.
- Store the report as gzip-compressed JSON (since schema v25; migration compresses old reports). Reports often exceed the logs themselves: a 4 MB `Player.log` produces about a 9 MB report. Compression reduces this to about one-twentieth. If a cached report cannot be decompressed, treat it as a cache miss and reparse.
- The report covers only the file size at read time, since the game may still append to it. Record the CRC-32C of those bytes for the cursor (§6.6).

**The first query for a new log** has a target of ≤500 ms on a cache miss (07 §6):

- **Indexed `Name` lookup** (`def_fact_named`, containing only the ~5% of rows with a `Name`, then filtered by the `content_state` version; §3): missing-parent clusters query `Name="…"` for a provider. Previously, each lookup scanned all `Def` facts for that game version (83,000 rows, ~70 ms), taking ~14 seconds for 190 clusters. Query each distinct name only once per log.
- **Reference index** (`defName` referrers): keep one index per enabled set and game version. It records hashes of names appearing as references in each effective `Defs` or `Patches` file (`>X<` and `<X>`, without whitespace). The analysis subprocess builds it after analysis (485 `Mods`, 5750 files, 77 MB: ~0.3–0.4 seconds and 4.8 MB); a query builds it if absent. Store it at `<data-directory>/cache/refindex/<key>.idx`. The key combines the game major.minor version and each enabled Instance, snapshot, and folder, independently of order. Retain only the latest 8 indexes. Search only the files selected by the index, verify the original forms, and take the earliest matching line. Previously, every missing name required rereading every enabled file. Results are identical across 4 corpus logs and 4113 clusters; reference evidence is now sorted by file.
- **Clustering**: classify each entry (level, message, continuations, and frames) only once, calling `Classify` once per entry. The `noiseRe`, `textureRe`, and `modReportRe` expressions use one leading `^`; branch-local anchors made Go retry at each position in a long message.
- See 07 §6 for the measurements: logs that formerly took 568 ms now take 130–330 ms.

### 6.6 Live Tracking

See 08 §4 for game detection. While the game runs:

- **Incremental read** (`app.LiveLog`): remember the `Player.log` path and read offset. Check each second and read up to 8 MiB, leaving the remainder for the next read. A smaller file means a new file (a game restart): reset the offset and clear the old clusters. Split new content into lines and retain an incomplete last line until the next read.
- Preserve parser state across batches. An entry ends at the next noncontinuation line or at game exit (`logs.Parser`).
- Preserve phase markers across batches too (§6.7.4). If an old cluster repeats in another phase, update that phase's count and the this-run entry without requiring a new cluster. The first marker assigns preceding entries to loading. Without any markers, retain the original flat entries.
- **Memory**: retain clusters, not the raw log. Keep the first 3 sample entries and the count per cluster. Above 2000 clusters, discard the oldest informational clusters. Memoize the category by message, location frame, and continuations (regex costs ~70 µs per entry, so repeated messages are classified once). Clear the table at 10000 entries.
- Fully attribute the first sample of each new cluster (§6.2). Show the result in the this-run part of errors from the last run on can it run, in first-appearance order (06 §2.4). Update counts without dialogs. Show the game-running status and this run's error count on the GUI launch page and TUI status line (since 2026-10-30; the GUI sidebar no longer has this text at the bottom).
- On exit, stop tracking and perform a full analysis, as with `archo log`. Replace the this-run section with the result.

**Companion logs** are JSON logs written by in-game helper `Mods`: Archo Monitor's `ArchoMonitor.log` or RimCrow's `RimCrow_Realtime.log`. Read them incrementally alongside `Player.log` to add timestamps, levels, counts, files, and related `Mods`. See §6.7 for file selection and field usage.

**Interface**. CLI `archo log --follow` emits new clusters until game exit or Ctrl+C. With `--json`, emit one cluster per line; with `--once`, stop after reading the current contents. See 05 §4 for the other log flags (`--cluster`, `--file`, `--prev`, `--text`, `--all`, `--select`).

**The since cursor** supports agent polling (MCP `analyze_log` since, CLI `archo log --since`; implemented 2026-10-22):

- The cluster lists include `cursor: log1.<size>.<checksum>[.<companion-size>.<checksum>]`: the number of bytes read from `Player.log` and its paired companion (§6.7.4), in base 36, plus their CRC-32C checksums. The cursor stores no state on disk. The CLI and MCP results are identical.
- With since, analyze the current log normally, using the cache if possible. Recluster the first size bytes without attribution to obtain the counts at the cursor. Show only new clusters (new) and clusters with increased counts (`+N`), with the added and total counts in the heading and a new cursor. Also read the companion prefix, so companion-only entries after the game stops writing `Player.log` and increased companion counts are included. A changed companion session means nothing was known about it at the cursor.
- **Expiry**: if the prefix is shorter or its checksum differs, the game has restarted and written a new log. State that the cursor expired and list all current clusters. If the current `Player.log` does not match but the `Player-prev.log` prefix does, explain that the cursor's run is now `Player-prev.log` and suggest `archo log --prev --since <cursor>` (MCP `prev=true`). If the previous log itself does not match, explain that a newer run replaced it after another restart.
- The last line at the cursor may have been half-written. Count the entry as new when it is completed, preferring an extra report to an omission.
- This polls full analysis, rather than `LiveLog`: analyze the file size at each call. See §6.5 for first-query speed.

### 6.7 Archo Monitor and Companion Logs

> Status: added 2026-10-21. Decision: RimArcho bundles the in-game helper Archo Monitor, enabled in plans by default (04 §1.5) and installed with releases (08 §9).

Only the game knows what `Player.log` cannot say: which file or `Def` was loading when an error occurred, which Mod owns an assembly or Harmony patch in the stack, and how many errors occurred after the logging limit. Archo Monitor (`packageId` `toumanlin.archomonitor`) records these in-game as JSON lines in a companion log. Its behavior follows RimCrow Companion 0.3.12; the decompiled code was read only to understand behavior and format, and the implementation was written independently. No code was borrowed from the projects it credits: BetterModSort (Apache-2.0), LogAfterDefError (MIT), BetterLog (MPL-2.0), or Mod Error Checker (no public license). About.xml credits them all.

Archo Monitor settings use RimWorld Keyed translations and follow the game language. Package the `Languages` directory with the mod; build and test requirements are in 07 §4–§5.

#### 6.7.1 What Is Captured

| Hook | Action |
| --- | --- |
| `Prefix` on `Verse.Log.Error(string)`, `Verse.Log.Warning(string)` | Record every error and warning. The `Prefix` runs before the `StopLoggingAtMessageCount` check, so errors are counted even after the game stops writing `Player.log`. |
| `Prefix` on `Verse.Log.Message(string)` | Leave this hook off by default because informational entries are the most numerous and this is a hot path. Enabling "record informational messages" in the Mod settings attaches it on the next startup. |
| `Application.logMessageReceivedThreaded` | Capture `Exception`, Error, and Assert messages outside `Verse.Log`, with no `Verse.Log` frame in the stack. |
| `Prefix` + finalizer on `DirectXmlLoader.DefFromNode`, `DirectXmlToObjectNew.DefFromNodeNew` (`1.6`+) | Keep a thread-local stack of the loading `LoadableXmlAsset` and `Def` node. Also map `defName`, or `Name` if absent, to the file, owning Mod, and `ParentName`. |
| `Prefix` + finalizer on `LoadedModManager.ApplyPatches` | Mark XML patching. On the first entry, collect all enabled `Mods`' operations: map XPath `defName` and `Name` values to the Mod and operation type, including `Sequence`, `FindMod`, and `Conditional` children. |
| `Postfix` on `DirectXmlToObject.ValidateListNode` | Map a referenced `Def` name to the loading `Def`, file, and list field from the stack above, without another XML traversal. Use this for missing cross-references. |
| `Postfix` on `Verse.UIRoot_Entry.Init()` | Menu initialization is complete and `MainMenuDrawer.Init` has already run. Enter menu, recording it again when returning to the menu from the game. |
| `Prefix` on `Verse.SavedGameLoaderNow.LoadGameFromSaveFileNow(string)` | Begin loading a save and enter save. Retain the filename without its directory. |
| `Prefix` on `RimWorld.Planet.WorldGenerator.GenerateWorld`, `Verse.Game.InitNewGame()` | Begin world generation or new-colony initialization and enter new. Record consecutive entries into the same phase only once. |
| `Postfix` on `Verse.Game.FinalizeInit()` | New-colony or save initialization is complete. Enter play. |

- **Stack → Mod**: map the frame's declaring-type assembly to the Mod loading that assembly, cached by assembly. For Harmony patches on the frame's method, use the patch method's assembly, falling back to matching the owner against `packageId`, cached by method. Exclude base assemblies (mscorlib, `System`, netstandard, `Microsoft`, `Mono`, `MonoMod`, `Unity`, `UnityEngine`, `Assembly-CSharp`, `Assembly-CSharp-firstpass`, `0Harmony`, `HarmonyLib`, `Waylith`, `Brutal`, `ICSharpCode`, `Newtonsoft`, `NAudio`, `NVorbis`, `ISharpZipLib`, `com.rlabrecque.steamworks.net`, and Monitor itself). Include base `Mods` (Harmony, HugsLib, XML Extensions, Prepatcher, Performance Optimizer, RocketMan, and Mod Error Checker) only if no other Mod is found. Stop at the first non-base Mod frame. For textual stacks supplied in messages or `Unity` callbacks, parse `Type.Method` line by line and use the same mapping.
- **Extra context**: use the first rule that yields a result. For `Config error in X:` or `Exception in ConfigErrors() of X:`, collect X's file, the parent chain's files (up to 10 levels), and patch operations on those `Defs`. For `Could not resolve cross-reference … named X`, collect the referencing `Def`, file, and field, plus patches adding X to those `Defs`. For `… loadID X`, record only `loadID:X`. While loading a `Def`, collect the current file, `Def`, and Mod. Also map message packageIds such as `[a.b]` and `assembly name: X` to `Mods`.
- **`inferredType`** uses the first match: `DefConfigError` (the Config error above), `CrossRefError` (cross-reference or loadID), `PatchError` (during XML patching or a `Patch operation` message), `XMLSyntaxError` (while loading a `Def`; the RimCrow name means an error during XML loading), `AssemblyConflict` (`MissingMethodException`, `TypeLoadException`, `ReflectionTypeLoadException`, `MissingFieldException`), `TickException` (`Exception ticking`), `DrawException` (`Exception drawing`), `NullReference`, `OutOfMemory`, or `MissingTexture` (`Failed to find any texture`, `Could not load Texture2D`, `Could not load UnityEngine.Texture2D`). Otherwise, it is empty. A Monitor failure writes `ObserverFailure` without affecting the game.
- **Stack levels**: in 1.5 and `1.6` (`Unity`), Monitor raises Error and `Exception` stack levels below `ScriptOnly` to `ScriptOnly`, leaving `Full` unchanged. Do not change them if other diagnostic `Mods` (Visual Exceptions or Mod Error Checker) are loaded. In 1.7, `Waylith` has no such setting and already writes full stacks.
- **Unchanged messages**: RimCrow adds a `[Mod name, …] ` prefix. Monitor preserves the `Player.log` message exactly so that RimArcho can match it.

#### 6.7.2 Overhead

- **Per log entry (hot path)**: look up the level and original message in a hash table once. A hit only increments the count. A miss normalizes the message and analyzes its stack and context once per distinct message.
- **Loading**: each `Def` requires one stack push and pop and one dictionary write. Each `Def` list item requires one dictionary write, without constructing a node path. Collect patches in one pass. The tables are capped at 300,000 `Defs` and 500,000 references; stop recording when full.
- **File writes**: use a 64 KiB buffer. Flush the first occurrence of an `ERROR` immediately because the game may crash next. A background timer writes other first occurrences and count changes at most once per second. Flush on `Application.quitting` and `AppDomain.ProcessExit`.
- **Phase changes**: handle them only when a hook fires, without polling or work per frame. Under the same Writer lock, call `FlushDirty`, then write and flush the phase boundary. Do not add stacks, reflection, or file writes to the per-error hot path.

#### 6.7.3 Files

- **Location**: write `ArchoMonitor.log` under the save-data root (`GenFilePaths.SaveDataFolderPath`, which follows `-savedatafolder`, so packs use `<pack>/userdata/`). If unavailable, use the command-line `-savedatafolder`, then `Application.persistentDataPath`. On startup, rename the existing log to `ArchoMonitor-prev.log`, replacing the previous file, and create a new current log.
- Use UTF-8 without a BOM, with one JSON object per line ending in `\n`. Truncate message content above 4 KiB and details above 16 KiB, adding … . At 32 MiB, write `kind:"full"` and stop writing new entries. Phase changes and exit still write the remaining counts and boundaries.
- **Deduplication**: the fingerprint combines the level, inferred type, and normalized lowercase message (hex addresses → `<hex>`, numbers → `<num>`, and whitespace collapsed to one space). The exact-message hot table is capped at 8000 entries, and the fingerprint table at 4000. When full, flush pending counts before clearing them. Subsequent occurrences of the same messages are treated as first occurrences again.

**Format (schema 1)** is a superset of the RimCrow format, so one reader handles both.

| Field | RimCrow Companion | Archo Monitor |
| --- | --- | --- |
| `timestamp` | `yyyy-MM-dd HH:mm:ss.fff`, the local time when this line was written | Same |
| `level` | `ERROR`, `WARNING`, `INFO` | Same |
| `message` | The text before the stack, prefixed with [names] if `Mods` were found | The original text before the stack |
| `details` | The stack text on every line | The stack appears only on the entry's first line; later count lines use an empty string to avoid repeating it. An `ERROR` or `WARNING` without its own stack gets the stack of the code calling `Verse.Log`, excluding Monitor and logging frames. |
| `count` | Occurrences represented by this line: 1 at first, then the increase since the previous line for the same entry | Same |
| `context.relatedModIds` | The lowercase, player-visible `packageId` | Same |
| `context.relatedFiles` | A mixture of Mod-relative and absolute paths, `patch:<Def>:<operation>`, `xref:<name>:<node-path>`, `def:<name>`, `loadID:<ID>`, and `<root:<node>>` | Only absolute file paths; other references go in `context.refs` |
| `context.inferredType` | The names above, except `PatchError` | The names above |
| `source` | — | `"archo-monitor"` |
| `schema` | — | `1` |
| `id` | — | The entry's 8-hex fingerprint, identical on every line for that entry |
| `total` | — | The cumulative count through this line |
| `kind` | — | Present only on special lines: session (first), exit (last on normal exit), full, and phase |
| `context.phase` | — | The phase on a phase row: menu, save, new, or play; loading applies before the first phase row |
| `context.save` | — | The save filename on a save phase row, if available, without its directory |
| `context.roles` | — | `{"<packageId>": "xml" \| "def" \| "patch" \| "stack" \| "text"}` records how the Mod was found: file owner, definer in the `Def` chain, patcher, stack or patch owner, or message mention. |
| `context.refs` | — | `patch:<Def>:<operation-type>@<packageId>`, `xref:<name>:<Def>/<field>`, `def:<name>`, `loadID:<ID>` |
| `context.def` | — | The `Def` loading when the error occurred |

**Session line**. The first line has level `INFO`, message `[Archo Monitor] session <id>`, and kind session. Its context contains session (12 random hexadecimal digits), version (the Monitor version), game (`VersionControl.CurrentVersionStringWithRev`), `dataRoot`, and `playerLog` (`Application.consoleLogPath`, empty if unavailable). Also write `[Archo Monitor] <version> session <id>: <full-ArchoMonitor.log-path>` to `Player.log` through `Log.Message`. This lets RimArcho pair the files: a `1.6` pack's `Player.log` stays in `Unity`'s fixed location while the companion is in the pack's data root, so the explicit path avoids guessing. **Exit line**. Its kind is exit, and `context.entries` is the number of distinct entries.

**Phase line**. Its level is `INFO`, kind is phase, message is `[Archo Monitor] phase <phase>`, and `context.phase` is one of the values above. The other envelope fields follow schema 1. Flush pending count rows before the change, so that each count row belongs to one phase; total remains cumulative for the whole run. Write the same phase message to `Player.log` through `Log.Message`, regardless of the informational logging setting. Read boundaries independently in each file's order. Phase messages themselves never form error clusters.

#### 6.7.4 How RimArcho Reads Them

**Selection** is read-only; do not rename or delete the files.

1. The companion is in **this run's save-data root**: the environment's userdata for a plan, or `<pack>/userdata` for a pack (08 §5.3). When analyzing a `Player.log`, use its containing directory as the data root, or the parent of Logs for 1.7 `Logs/Player.log`.
2. If `Player.log` has a session marker, try the marker's path, the adjacent `ArchoMonitor-prev.log` (renamed on the next startup), then both files in the data root. Select the file with the same ID in its session line.
3. Without a marker, try the same generation's `ArchoMonitor.log` or `ArchoMonitor-prev.log`, which must have a valid session line, then RimCrow. If there is a marker but no matching session, fall back only to RimCrow. Pair RimCrow by generation: `Player.log` with `RimCrow_Realtime.log` (or `RMM_Realtime.log` if unavailable), and `Player-prev.log` with the same names containing `-prev`. Every candidate without a session match must be a nonempty regular file whose mtime is at or after the run's start and at or before `Player.log` mtime +1 minute. Do not pair files if ownership cannot be confirmed. Use the `Started:` time in `Player.log` (1.7, with a timezone), otherwise the Windows file creation time. If the start time is unavailable or later than the log's mtime, conservatively use the mtime. The current run's lower bound must also be later than `Player-prev.log` mtime, avoiding old creation times retained when `Unity` truncates and reuses a file.
4. The same companion, including a hardlink or path alias, cannot be paired with both Player.logs in the same directory. Prefer session-marker evidence; on equal evidence, assign it to the current generation. `app.CompanionFor`, full analysis, the log page, and live tracking share these rules. Recheck the pairing after a companion append or rotation even if `Player.log` has not changed.
5. For live tracking (§6.6), select the companion in the run's data root using these rules, preferring Monitor to RimCrow. Recheck the session when a marker arrives, including rotated files. A smaller companion restarts the read, as with `Player.log`. Read the companion before `Player.log` so classification hints are ready. If a cluster was attributed before its companion entry arrived, attribute it again on arrival. Wait for a companion-only entry to pair according to 06 §2.16.1 L03, since `Player.log` and Monitor write almost simultaneously. Stop waiting at game exit, but otherwise continue if the matching `Player.log` entry is the unfinished last entry or if an unread backlog remains. If the `Player.log` entry appears after the wait, merge it into the companion-only cluster. Make it the first sample and reattribute; use the larger of the two counts without creating another cluster. As in full analysis, presence checks include all `Player.log` entries, even noise and informational lines. Store only normalized first-line hashes, capped at 50,000 and cleared when full. If a Monitor entry has a stack and passes the noise filter but the same line in `Player.log` is noise, it is still not an error.

**Field usage**:

| Field | Use |
| --- | --- |
| `message` | Normalize the first line using §6.1 and match it to a `Player.log` cluster. For a RimCrow line with nonempty `relatedModIds` and a message beginning with `[`, first strip the prefix through the first `] `. |
| `id` | Merge Archo Monitor lines by entry id, and RimCrow lines by level plus normalized message. |
| `timestamp` | The time on the entry's first line is the cluster's first-occurrence time. Show it in the live this-run section and full-analysis cluster details. |
| `level` | Use it as the cluster level, which `1.6` `Player.log` lacks. Map `ERROR` to error and other levels to warning. |
| count, total | Sum the counts across lines for an entry, or take the maximum total if available. Without phase markers, use the larger of the `Player.log` and companion counts; with markers, merge them as described below. Only the companion knows the count after the game stops writing `Player.log`. |
| `kind:phase` | Assign each `Player.log` message and companion count row to its latest preceding phase. Take the larger count from the two sources per phase, then combine the cluster's phase totals without double-counting. Counts restored by total that cannot be assigned to a phase remain unknown (the interface says "phase not recorded"). If only the companion has markers and a `Player.log` entry has no companion record, keep its phase unknown; do not assume loading. |
| `details` | For Monitor `ERROR` and `WARNING` entries absent from `Player.log`, usually after its logging limit, pass message + newline + details to the §6.1 parser. They become ordinary clusters and receive normal attribution, with "only in `ArchoMonitor.log`" in the details. Do not do this for RimCrow, whose messages have prefixes and whose stacks are often guessed. |
| `relatedFiles` | An absolute path inside an **enabled** Instance identifies it as source, with `ArchoMonitor.log: <Mod-relative-path>` as evidence. This has the same strength as §6.2 file-path evidence, so confidence can reach certain. Combine RimCrow relative paths with each Mod directory in `relatedModIds` and handle them the same way. Treat pseudo-paths (`patch:`, `xref:`, `def:`, `loadID:`, `<root:`) as refs. |
| `relatedModIds` + roles | xml or def with file evidence for the same Instance gives source; patch gives patcher; stack, text, or no role (RimCrow) gives mentioned. Only add evidence if the Instance already has a stronger role; never downgrade it. |
| `refs` | Attach the text as evidence to the corresponding Instance (`patch:…@<packageId>` goes on the patcher). |
| `inferredType` | If RimArcho classifies the cluster as other or exception and the hint maps to a more specific category, use that category: `DefConfigError` and `XMLSyntaxError` → `xml-load`; `CrossRefError` → `xref-missing`, or `save-load` for loadID; `PatchError` → `patch-failed`; `MissingTexture` → `texture-missing`. This changes the cluster key, so live tracking and full analysis use the same rule. Put other hints only in the details. |
| `context.def` | Add `while loading <Def>` to the evidence text. |

- **Interface**: on can it run, a cluster with file evidence has an evidence chip of kind log ("log"). Its source is `file <Mod-relative-path>  ArchoMonitor.log  <time>` (or `…  RimCrow_Realtime.log` for RimCrow). Put the file first with the explicit "file" label and use the spacing from 06 §7.3. The window splits the text on two spaces (VM 2026-10-28: the old `ArchoMonitor.log  <path>` spacing collapsed in the window, making it look like one path). For several locations in the same Mod, show the first plus N more.
- **Game log page** (06 §2.10): give the companion its own "Archo Monitor · this run" section. Show one row per entry with the time, level, short `inferredType`, first line, count, related `Mods`, and files. Expand to show details, and allow a jump to a matching cluster. The header contains the session's version and game and the session and exit times. List the companion below its `Player.log` in the file selector. Use the same reader, also retaining the session game, exit time, and full line. Cache by file path, size, and mtime. While the game runs, use the data already read by live tracking without another read.
- **Phases and display**: when phase information exists, deduplicate companion entries separately at each phase boundary and retain their first-appearance order in the file. The same id can appear again after another save load to show the boundary. Global attribution still merges by id. See 06 §2.4 and §2.10 for can it run grouping and log-page separators, and 05 §4 for the CLI and MCP. Logs from an older Monitor, RimCrow, or no Monitor retain the old count and display rules if they have no phase markers.
- **Cache** (§6.5): add the paired companion's path, size, and mtime to the key.
- Skip and count invalid lines (live `BadLines`, or the full-analysis heading). Each companion entry table and interface phase-entry table is capped at 20000 entries. Once full, update only existing entries.

#### 6.7.5 Version Differences

| | 1.5, `1.6` | 1.7 |
| --- | --- | --- |
| Runtime | `Mono`, with assemblies compiled for net472 | .NET 10 (`Waylith`), with assemblies compiled for net10.0 |
| Harmony | 2.x bundled with the Harmony Mod | 3.x |
| `Application` | `UnityEngine` | `UnityStubs`: has `logMessageReceivedThreaded`, quitting, and `consoleLogPath`, but no `SetStackTraceLogType` |
| `DefFromNodeNew` | Present in `1.6`, absent in 1.5; find it by reflection and skip the hook if absent | Present |
| Phase hooks (§6.7.1) | All exist in the reference assemblies. `GenerateWorld` lacks `LandmarkDensity` in 1.5 and has it in `1.6`; find it by name through reflection. | All exist; `GenerateWorld` has the same parameters as in `1.6`. |

`LoadFolders.xml` selects directories by game version: `1.5/` and `1.6/` contain the net472 assemblies, and `1.7/` contains net10.0 assemblies. Find hook targets through reflection. Skip missing targets and list them in the session line's `context.skipped`.

The phase hook signatures have been checked for each version against the Krafs reference assemblies selected by `monitor/ArchoMonitor.csproj`. These references contain only method signatures. The actual timing of menu entry, world generation, save loading, and playable state must be tested in full game rounds in both `1.6` and 1.7 (the pre-release VM checklist).

### 6.8 Error Pattern Library

The pattern library comes from the public HugsLib log corpus (`docs/research/00-log-corpus-design.md` §3 G5 and §9; integration plan in `docs/research/01-pattern-library-integration-draft.md`). It ships as embedded data (`internal/patterns/patterns.json`, like `internal/order/observed.json`) and contains only templates, packageIds, and counts. Generate the data from the research dataset with `go run ./spikes/patterns -export internal/patterns/patterns.json`. The `-verify <corpusrun-result>` option checks that the product matcher and research classifications agree for every cluster (v0: 610,910 clusters, 0 differences).

| Layer | Reference | Recognition | Cluster display |
| --- | --- | --- | --- |
| Generic | `pattern:g:<id>`, e.g. `pattern:g:xml.type-missing` | Match regex rules in order against the message part of the cluster template (§6.1, category\|message), optionally restricted by category. The first match wins. | Show a one-sentence explanation; `evidence: community: seen in N of M public 1.6 logs (x% of error clusters)`; common causes; a diagnostic route using MCP tools, omitting the current step and replacing `E<n>` with this cluster; fix kinds; and the error-disappearance rate after 1–3 list edits in the same player's repeated uploads, compared with an unchanged-list control. Hide the comparison with fewer than 5 pairs. |
| Fallback | Same, fallback | Bare `NullReferenceException` and other exception, XML, patch, or save-loading errors: describe only the category. | Hidden |
| Mod-specific | `pattern:m:<packageId>:<hash>` | The template matches exactly, and the Mod is attributed as source. | Show a `known-error:` line with the public-log count, first and latest months of occurrence, observed Mod versions, and "newer Mod versions may have fixed this". |

- Patterns describe what this kind of error usually means, rather than identifying the culprit in this list. The principle that appearance is not causation still applies. The interface labels the pattern card "community background" and shows it only on a match (06 §2.10).
- Retain Mod-specific templates only if they occur in at least 10 logs and 5 player sessions. Discard any template that appears to contain paths or user data (docs/research/00 §8).
- `archo pattern [<reference>]` shows a pattern's recognition rules, phase, occurrence counts, common causes, diagnostic route, fix kinds, and repeat-upload evidence. With no argument, it lists all generic patterns by cluster share. `--mod <packageId>` lists a Mod's known errors. MCP provides this through `analyze_log pattern=<reference>` (05 §4).
- A checklist (MCP `guide topic=errors`, CLI `archo guide errors`, 05 §5.3) follows the corpus frequency order. It covers categories not addressed by `guide topic=diagnose`: save loading, missing assets, configuration checks, assembly loading, and noise. Step 3 of the diagnostic route links to it.
- Distribution: v0 ships with the program. Later versions will be obtained through `archo data update`, like the community sorting rules (04 §6.3).

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| B1 | A log reports a patch failure. | Attribute it to a specific Patch file and line, with §4.4 reasons (golden harmonylog: `Patches/Compat.xml:2 … (patch.no-match: target-inactive, unguarded-optional)`, Lib is dependency, confidence is certain). | Pass |
| B2 | A log reports a missing cross-reference. | Identify the referring Mod and file, and state whether a provider exists. | Pass |
| B3 | A `Pawn.Tick` exception has Harmony patch lines from 3 `Mods`. | All 3 `Mods` are patcher, with no source. Provide the checklist and `harmony --target` and `source search "AllComps.Remove("`. | Pass |
| B4 | The same exception repeats 500 times. | Show one cluster with `x500`. | Pass |
| B5 | Replay the old inspector's cases (`internal/app/m4_test.go`). | Conclusions are the same or more accurate: compare selectors with the enabled state; `files search` excludes `Languages` and includes Mod references; decompilation rebuilds a deleted cache directory and invalidates the cache when a DLL changes. | Pass |
| B6 | Read the same game's old `1.6` log after it upgrades to 1.7. | Automatically analyze it as `1.6` and note this in the heading (`TestLogOwnVersion`, `TestOwnLogBeforeGameUpgrade`). | Pass |
| B7 | Read this environment's old-version log or a log written before a plan edit. | Mark it stale, uncounted, and collapsed (`TestStaleLog`, `tui.TestProblemsViewLogs`). | Pass (current rules) |
| B8 | Read the same log again in a new process. | Use `log_cache` without reparsing; status reads only the cache (`TestLogCache`). | Pass |
| B9 | A log was written after plan A launched; analyze plan B. | Mark it stale because it belongs to another plan's launch. | Pass |
| B10 | Edit plan A after launching it, then read the log. | Mark it stale because the plan changed after launch. Restoring the original list makes the log current again. | Pass |
| B11 | Launch through Steam without a launch record, after writing plan A with save. | Assign the log to plan A. | Pass |
| B12 | A cluster template matches a known pattern (§6.8). | Attach `pattern:<id>` and allow follow-up with `archo pattern` or `analyze_log pattern=`. Hide fallback patterns, and show a Mod-specific pattern only when that Mod is source (golden harmonylog). | Pass |
| B13 | Evaluate the corpus layer (`dep.observed-missing`, `combo.known-bad`, `mod.known-error`). | Report `known.json` findings for the current enabled list with community evidence (`internal/app/known_test.go`). | Pass |
| B14 | Two environments share userdata, and another game writes the log. | Exclude it before analysis, attribution, or cache reads. Do not mark it seen or include it in new-log notices, findings, or counts. Fall back to this game's previous log. The CLI and MCP agree (`TestForeignLogExcludedBeforeAnalysis`, `TestLogOwnerEvidence`, `TestForeignLogCLIParity`). | Synthetic pass; VM pending |
| C1 | Enable a Mod whose dependency is not installed. | Report `dep.missing` with a Workshop link suggestion. | Pass |
| C2 | Enable both Workshop and local copies with the same package ID. | Report `pkg.duplicate-active`. | Pass |
| C3 | Two `Mods` define the same `ThingDef`. | Report `def.override` and identify the effective definer. | Pass |
| C4 | A patch's target Mod is disabled, with no `FindMod` guard. | Report `patch.no-match` with `target-inactive` and `unguarded-optional`. | Pass |
| C5 | Two `Mods` use transpilers on the same method. | Report `harmony.multi-transpiler` as info, promoted to warning if the method appears in the current log's stack (golden harmonylog: Mod B and Mod C at `Verse.Pawn.Tick`; warning for a current log, info once stale). | Pass |
| C6 | A Mod patches only through manual code calls. | Report `harmony.manual` without falsely claiming there are no patches. | Pass |
| C7 | A plan edit flips only one Mod's LoadFolders condition. | Invalidate only that Mod's effective content (`TestLoadFoldersConditions`). | Pass |
| C8 | Alternate between 1.7 and `1.6` analysis. | Retain both analyses without evicting either (`TestAnalysisPerGameVersion`). | Pass |
| C9 | A bool prefix's target appears in a current log error cluster. | Promote `harmony.skip-prefix` to warning with cluster evidence and `archo log --cluster`. Do not escalate for a stale log (`TestSkipPrefixEscalation`). | Pass |
| C10 | Run `source search` without a scope, then with `--active` but no `--yes`. | The first returns a usage error. The second prints only the estimate, without decompiling (`TestSourceSearchScope`). | Pass |
| C11 | Switch between two plans with different enabled sets and view findings. | Do not rerun analysis on the second return to a plan. | Pass |
| C12 | The Workshop upstream of a RimArcho-made locked version updates. | Report `mod.pinned` as warning with "upstream has an update" and allow viewing the diff. | Pass |
| L1 | Append an exception to the fixture Player.log. | Show one new cluster within 1 second, with the same attribution as full analysis. | Pass |
| L2 | Write an entry in two batches, with continuations in the second. | Combine them into one entry. | Pass |
| L3 | Truncate and rewrite the file on a game restart. | Reset the offset and clear the old clusters. | Pass |
| L4 | Append 500,000 lines of the same error. | Keep one cluster with count 500,000 and resident memory growth <20 MB. | Pass |
| L5 | `RimCrow_Realtime.log` contains JSON and bad lines. | Add timestamps and count bad lines. | Pass |
| L6 | Only the old name `RMM_Realtime.log` exists. | Read it in the same way without renaming it (`TestCompanionFor`). | Pass |
| L7 | Call MCP `analyze_log` with the previous since cursor. | Return only new clusters or increased counts, including companion increases. After a restart, explain that the cursor expired and its run is now Player-prev.log (`TestLogSince`, `TestLogSinceMonitor`, `mcp.TestCLIParity`). | Pass |
| L8 | Player.log has a session marker; `ArchoMonitor.log` has the same id and a `relatedFiles` path to an enabled Mod's `Defs/X.xml`. | Mark the Mod as source with `ArchoMonitor.log: Defs/X.xml` evidence and show the file on the can it run evidence chip (`TestLogAnalysisWithMonitor`, `view.TestCompanionChip`). | Pass |
| L9 | After two runs, `Player-prev.log` and `ArchoMonitor-prev.log` share a session, while `ArchoMonitor.log` belongs to another run. | Pair prev with prev. Do not pair the current Player.log with the old companion (`TestCompanionFor`). | Pass |
| L10 | The companion has an `ERROR` absent from Player.log after game logging stops. | Create a cluster using the companion total; live tracking waits according to 06 §2.16.1 L03 (`TestLogAnalysisWithMonitor`, `TestLiveLogMonitor`). | Pass |
| L11 | Read bad lines, an incomplete last line, and a RimCrow message with a `[name] ` prefix. | Count bad lines, leave the last line for the next read, and strip the prefix to match the cluster (`logs.TestCompanionMonitor`, `logs.TestCompanionRimCrow`). | Pass |
| L12 | Run a pack with `-savedatafolder=<pack>/userdata`. | Read `<pack>/userdata/ArchoMonitor.log`, rather than the environment's companion (`TestCompanionFor`, `TestLiveLogMonitor`). | Pass |
| L14 | Enable Monitor and create a known XML error in real `1.6` and 1.7 games. | `ArchoMonitor.log` contains the entry and file, and can it run shows file evidence. | Pending (manual checklist 07 §4) |
| L13 | Write one entry across lines with counts 1, 5, and 3 and total 9. | The cluster count is 9, with the timestamp from the first line (`logs.TestCompanionMonitor`, `TestLogAnalysisWithMonitor`). | Pass |
| L15 | During live tracking, the companion entry arrives first, and the Player.log entry ends or appears after 5 seconds. | Keep one cluster with Player.log as evidence and count later occurrences of the same error in it (`TestLiveLogMonitorFirst`; VM 2026-10-28 split it into two). If the Player.log line is noise, e.g. `Translation data … errors`, and the companion entry has a stack, exclude it from live tracking too (`TestLiveLogMonitorNoise`). | Pass |

The acceptance tests for fact sets and cleanup are `TestFactSetsShareVersionsAndUpdates`, `TestFactSetsDifferentFoldersAndPriority`, `TestFactSetsHarmonyViews`, `TestFactPruneVisibilitySlotsAndEnvRemoval`, `TestFactSetsDifferentAssemblyInputs`, `TestMigrateV26FactViews`; see 07 §6 for scale and migration measurements.
