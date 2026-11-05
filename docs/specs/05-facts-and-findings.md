# 05 · Facts and findings

> Status: the declaration, `Def`, Patch, and Harmony layers, log attribution, and live tracking are implemented. Logs are assigned using launch records (06 §4), and multiple analyses are cached per plan (§3).
> Basis: 00, 00a, 02, 04, 18, 17 §6, 09 §4

This document explains how RimArcho collects facts from Mod files and game logs, then derives findings from those facts. See 02 for identity and scanning, 04 for plans and sorting, 05 for commands and MCP tools, and 06 for their presentation in the interface (the wording for the can it run page is in 16 §5). Performance targets and measurements belong in 18; this document only references them.

The "analyzed plan" is the plan whose enabled list and order the analysis uses (07 §1). In the interface, this is the open plan. The CLI and MCP default to the environment's current plan.

## 1. Effective Content

The game does not load every file in a Mod directory. All content analysis (`Def`, Patch, assemblies, and log reference lookup) uses only the effective content. It follows the game's `ModContentPack.InitLoadFolders` and `DirectXmlLoader.XmlAssetsInModFolder` semantics (17 §6).

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
| `ModDecl` | Dependencies (including version-specific dependencies), loadBefore/After, forceLoad*, incompatibleWith, `supportedVersions`, and the original `LoadFolders.xml` | The snapshot's `decl_blob` (04 §1.3) |
| `Def` | Type (element name), `defName`, `Name`, `ParentName`, `Abstract`, file relative to the Mod root, line of the opening tag, owning Instance, and the `Def`'s own `MayRequire` and `MayRequireAnyOf` | Effective `Defs/**/*.xml` |
| `PatchOp` | Operation class, XPath, child-operation structure, value summary, file and line, ordinal, and simulation result and reason | Effective `Patches/**/*.xml` |
| `TypeDef` | Assembly, namespace, and full type name | .NET metadata in effective DLLs |
| Game type | Game assembly file, game version, full type name, namespace, simple name | .NET metadata in the game's own `Assembly-CSharp.dll` and `Assembly-CSharp-firstpass.dll`, read at scan (§5.3) |
| `HarmonyPatch` | Target type, method, and parameters; `MethodType`; patch kind (prefix / postfix / transpiler / finalizer); patch method; whether it may skip the original method; and whether the target is dynamic | Same as above |
| `HarmonyId` | Suspected Harmony ID strings, identified heuristically | Same as above |
| `Rule` | Subject, relation, object, source (user / community / replacement), and scope | 08 §2 |

- Each fact is associated with an Instance, a snapshot, and the analyzer version (`analyze.Version`) through its fact set. Reads use the version views in §3 for the current `Store.MM`. Facts are rebuilt when the analyzer version differs.
- The game loads `Defs` with `MayRequire` only when the required `Mods` are enabled. Analysis follows the same rule (`DefFact.Loaded`).
- `archo def` lists the `file:line` of each definition in load order and marks the last one `← effective`.

## 3. Execution and Storage

**Layers**. The declaration layer takes milliseconds and is computed on demand. The `Def` layer follows it, then the Patch layer, which depends on the `Def` layer. Harmony is independent. A failure in a higher layer does not affect the results of lower layers; a Harmony failure affects only its own facts.

**On-demand declarations**. The declaration layer uses no subprocess and stores no results in the database. Its results are cached in-process together with plan reads. The key combines `Store.ChangeKey()` (`PRAGMA data_version` + `total_changes()`), the environment ID, and the size and modification time of ModsConfig.xml. Frequent interface refreshes and detail panels reuse this cache.

**GUI list snapshots**. The editor and can it run share finding snapshots in memory. List edits update the declaration layer synchronously. The content layers retain their previous results, marked as updating, while logs and analysis are completed in the background. See [16 §8.7](16-interface/8-gui-technology.md#87-list-findings-and-needs-cache-api) for the keys, invalidation conditions, and event contract.

**On demand**. Stale parts are checked and rebuilt only when can it run opens, the plan editor's details need findings, or the CLI or MCP requests content information. The problems, def, patches, and harmony commands rebuild stale analysis through subprocesses. The mod, status, and why commands stay fast by using only stored results (`--no-analyze` also prevents problems from rebuilding).

**Subprocess**. The `Def`, Patch, and Harmony analyses run in an `archo analyze` subprocess. When the stored results are stale, the caller (`App.EnsureAnalysis`) starts `<exe> analyze --env <name>` and passes `RIMARCHO_HOME` and `--game-version`. The subprocess writes the facts and exits without affecting the main process's memory.

- The subprocess writes `progress: …` lines to stderr. The interface displays them in the status bar or progress bar, and the CLI displays them as text.
- The subprocess has a soft limit of `debug.SetMemoryLimit(300 MiB)` (`AnalyzeMemoryLimit`). See 18 for the peak target.
- Tests inject an in-process runner (`InProcessAnalyze`).
- **Wait limit**: the default `App.AnalyzeWait` for each entry point is in 16 §2.16.1 W01; the CLI has no limit. On timeout, return the declaration findings with "still computing in the background". The subprocess continues, and the next call retrieves its results. Meanwhile, def, patches, and harmony use the previous, stale results.

**AnalysisKey (freshness)**. Hash the analyzer version, the game major.minor version, and each included Instance's ID, snapshot hash, and position in the plan. Use the first 24 hexadecimal characters. User and community rules are excluded because they affect only the on-demand declarations. Computing the key is cheap and does not require loading the declarations.

**Included Instances**. Include the analyzed plan's enabled list and all other live, loadable, disabled Instances. Loadable Instances include `Mods` already visible to the game and SteamCMD or other-folder `Mods` that can be deployed through links on save (since 2026-10-28; previously, these disabled copies had no `Def` facts, so finding providers of missing parent templates required per-file fallback reads). Extract only `Def` facts from disabled Instances, for `target-inactive` (§4.4) and log queries about who provides a missing `Def`. Harmony reads only enabled Instances.

**`--game-version X.Y`**. The analyze, problems, def, patches, harmony, why, and log commands accept this option. They validate the `X.Y` format, override the environment version, and restore it after the command. The corresponding MCP tools expose `game_version`. Use this when the game is 1.7 but the list and logs are still `1.6`.

**Storage** (`store.SaveAnalysis`, `store.saveContent`, `store.PruneFacts`).

- An analysis slot is `(environment, game_mm, enabled-set fingerprint)` (`analysis.set_key`), with the latest run retained per slot. Runs, patch facts, and findings remain isolated by environment and version. AnalysisKey detects order changes. Plans with the same enabled set share a slot. Saving an analysis removes enabled-set slots that no plan uses, across all game versions in that environment.
- The unique key of `fact_set(id, instance_id, kind, content_key, files)` is `(Instance, fact kind, content key)`. `kind=def` stores `Defs`; `kind=dll` stores Assembly, Type, Harmony, and Harmony ID facts. Fact rows carry only `set_id`. The set determines the Instance, whose ID format remains unchanged.
- **Set file lists** (`store/setfiles.go`): `fact_set.files` is a JSON array of the Mod-relative paths used by the set, ordered by their first use in a row. The `def_fact`, `type_fact`, `harmony_fact`, `harmony_id`, and `assembly_fact` tables store a zero-based `file_ix` instead of a path. Build the list once per set on save (`saveContent`, `saveHarmony`). It is immutable after the set is written and is deleted with the set. After reading the rows, readers (`DefsOf`, `DefsNamed`, `HarmonyPatches`, `Assemblies`, `TypeOwners`) read the list once per set and map the indexes back to paths. `Store` caches the list by `set_id` and clears it on `ChangeKey`, because a deleted set's ID may be reused. When the total exceeds 65,536 paths, randomly discard half. `CachedDefs`, which reads all disabled Mod `Defs` for each analysis, reads one set and its list together. It bypasses the cache to avoid evicting other lists.
- `content_state` retains the primary key `(instance_id, game_mm)`. Its `def_set_id` and `dll_set_id` point to the sets, and it retains the effective directories and parse summaries. Every fact read joins this version view by `Store.MM`. The same Instance shares sets across environments and game versions when its content is identical. Disabled Instances build only `Def` facts and retain a stored DLL view only if the content is unchanged. Once enabled, an Instance writes an empty set even if it has no effective DLL, clearing the old DLL view.
- Sets are indexed by `set_id`. Global name queries use the name index first, then join the version view. `def_fact_named(name, set_id) WHERE name<>''` preserves the fast path for parent `Name` lookup in logs. Type and Harmony ID queries also look up sets by name before scanning individual `Mods`. The DDL is in `internal/store/schema.sql`.
- **`Def` lookup order by name** (`Store.DefsNamed`, used by def, log attribution, and missing-parent fallback): order by Instance ID first, then by file order within a Mod. Do not use table row order. Cache rebuilds change row numbers, which can change the first provider of a missing parent template and the log finding references (2026-10-29 VM: the analyzer 7→8 rebuild changed the first provider for `log:E6` and its `p:` reference). Sorting does not change the join order: use the name index first, then join the version view (`CROSS JOIN`, as in `TypeOwners`). With an ordinary `JOIN`, SQLite avoids sorting by scanning the entire version's `content_state` and querying the name index for each row. This is dozens of times slower, with the gap growing as the Mod count increases (18 §2; `TestDefsNamedPlanStartsFromName` checks the query plan).
- **Batch lookups for save attribution** (`store/saveowners.go`, 12 §4.6): `DefDefiners` and `TypeDefiners` take up to 500 names per query and return the defining instances without file paths, name index first as above (`TestSaveOwnerLookupsStartFromName`). `NestedTypeDefiners` finds nested types by their inner name in one pass over the version's nested type facts. `FactViews` says which instances have a Def and a DLL view for `Store.MM`; `PackageFactViews` the same by packageId over every instance of it, in any environment and including tombstones.
- **Plan limitation remains**: version views are not plan views. The last directories analyzed for `(instance, mm)` replace that version view. If two plans select opposite LoadFolders conditions and load different folders, returning to a plan with a fresh slot may still show the other plan's content in def. Sharing sets by directory does not change this existing limitation.
- **Slot retention**: slots matching the environment's current major.minor version do not expire by age. Other slots expire when `store.ForeignAnalysisRetention` has elapsed since the last completed analysis; their run patches and findings are deleted too. The default is in 16 §2.16.1 A11. An unknown game version suspends age expiry.
- **Version view cleanup**: delete `(instance, mm)` if the Instance no longer exists, or if no environment that can see it currently uses mm or has an mm analysis slot. Determine visibility using 04 §4, without accessing Mod files. Conservatively retain views for visible environments with an unknown game version. Check all environments for shared Instances and only the owner for private ones. Apply the same rule after deleting an environment or slot.
- **Set cleanup**: immediately delete sets with no `content_state` references and cascade the deletion to their fact rows. `SaveAnalysis`, `env remove`, gc, and link identity merges perform cleanup within their own transactions. The independent maintenance call `Store.PruneFacts(now time.Time) error` can be repeated. Cleanup changes only the fact cache and analysis slots; maintenance handles shrinking the physical database.
- See below for the Source library and `source_use` (§5.4) and log attribution `log_cache` (06 §5).

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

The evidence chips in the interface represent evidence source types (16 §5).

**Principle: appearance is not causation.** A Mod patch appearing in a stack is only related (patcher). It is not a source unless its own frames or files provide evidence (06 §3).

### 4.2 Ignores and References

- A **finding reference**, `p:<6-hex-digits>`, is the first 3 bytes of sha1(`code | key | subject Lineage`) and is stable across runs. Log finding keys reduce absolute paths to their final three segments and lowercase them, preserving references and ignores after the Mod library moves or in tests.
- An **ignore** records `(Lineage, code, key)` on the Lineage, per environment, together with the subject's snapshot at the time it was ignored. A snapshot change (a Mod update, visible in mod library › changes) expires the ignore. The finding returns with `[ignore expired: the mod changed]`.
- The commands are `problems ignore|unignore <p:ref>`, `problems --ignored`, and `problems show <p:ref>`. The default list ends with `ignored: N (archo problems --ignored)`. See 06 for the interface keys.

**Muting and restoring version reminders** are independent of snapshot-based ignores (`app.SetVersionMute`, `store.VersionMutes`). The key combines the normalized `packageId` (trim whitespace, lowercase, and strip the game's `_steam` suffix) and the game major.minor version. All plans, environments, and copies of the same package share it. Mod updates, snapshot changes, and Lineage merges or splits do not clear it; another major.minor version triggers a reminder again. A changed package ID identifies another Mod. `version_mute(package_id, game_mm)` has a composite primary key, independent of the environment, snapshot, or Instance.

- `ProblemSet.Muted` retains the current plan's muted `version.unsupported` findings outside `List`; `All()` and individual details still expose them. By default, the CLI, MCP, and summaries report the muted count (CLI `ignored-version-warnings`). Neither `--all` nor subject or code filters automatically show these findings. An explicit `--include-muted` or `include_muted=true` includes them with a marker. See 13 §4.2 and 14 §2 for the commands and tools.
- Exclude muted reminders from finding counts, Mod row icons, pre-launch prompts, and pack build `Mismatch` warnings. Also exclude them from the counts of undeclared versions in a plan's allowed-version summary, and record the muted count separately (04 §`1.6`). Declaration facts, the allowed-version intersection and default, the pack version intersection, pack and game major.minor restrictions, and file integrity checks are unaffected.
- Restore an individual Mod's mute directly, without confirmation. Batch muting through summaries or manifest groups requires confirmation. See 16 §5 for the interface entry points and individual wording.

### 4.3 Finding Catalog

**Declaration layer** (computed on demand)

Link findings query `InspectGameLinks` each time, independently of the declaration, `Def`, and Patch caches. A target is not considered foreign just because it is in a player directory. Inspect the first level of each `Mods/` directory and each link target on disk. Instance names and registered real paths are cached with the Mod index using `Store.IndexKey` (17 §7.3). First compare the link target with the Instance's scanned real path and confirm on disk that the Instance still resolves there. Re-resolve every Instance path only if they do not match.

| `code` | Severity | Detection and suggestions |
| --- | --- | --- |
| `entry.missing` | warning | A plan entry matches no installed Mod but is retained in ModsConfig.xml. The subject is `pkg:<id>`. Suggest `archo find <id>` and `archo disable pkg:<id>`; add `subscribe: <link>` if community data identifies the Workshop item. |
| `mods.link-dangling` | error | A link with the owned prefix is at the first level of a known game's `Mods/`. Its target is confirmed missing, and its source is accessible. The subject is `path:<full-path>`, with `game:<game-path>`; no Instance is required. Block the selected game's launch and test run, but only summarize findings for other games. See 09 §3 for repair. |
| `mods.link-foreign` | warning | The target does not match a known Instance or pack identity, or cannot be mapped. Show the original target and ownership evidence. Retain valid env links; the prefix alone never authorizes deletion. |
| `mods.link-check-failed` | error | Access denied, an offline disk, an unknown reparse tag, or another failure prevents a judgment. Report the failure and retain the link. Complete the inspection before launching the selected game. |
| `mods.link-occupied` | warning | A real directory or file occupies a name with the owned prefix. Report it without offering a deletion action. |
| `mods.link-recovery` | error | The persistent ledger has an unfinished operation. Show the actual state and recovery action read-only, and complete the operation after confirmed link repair. Do not change the disk automatically on startup. |
| `mod.unloadable` | error / info | **error**: an enabled Instance will not load in the game because it is unloadable, residue, or displaced by another copy of the same package. Suggest disable. **info**: an installed, disabled Instance is unloadable for a reason other than about-disabled (04 §1.2). |
| `mod.pinned` | info / warning | A locked version (always "locked version" in player text; retain the code name). The current implementation recognizes a Workshop copy with `About.xml.disabled` and another live, loadable, non-Workshop Instance in the same Lineage. RimCrow `_<Workshop-ID>_` copies, `copy-local` copies, and copies deployed into `Mods/` count. **info**: the copy is effective, and the Workshop copy is upstream; suggest `archo unpin`. **warning "upstream has an update"**: the Workshop semantic hash differs from the copy, and ACF timeupdated is later than the copy's first appearance; suggest `archo mod <copy> --diff <Workshop>` and `archo unpin`. For RimArcho-made locked versions whose Workshop original is not hidden (the lock library, 07 §5), warn when the upstream snapshot differs from `origin.json` `origin_snapshot` and suggest `archo pin --update <copy>`. Do not also report `mod.unloadable` for locked versions. |
| `mod.hidden-residue` | warning | A Workshop source has a renamed About.xml and is absent from the ACF subscriptions: it is unsubscribed residue. Suggest `archo trash`. |
| `mod.hidden` | warning | Another Mod is hidden by renaming, usually by another manager. Suggest `archo unhide`, `archo trash`, or ignoring the finding to keep it hidden. Recompute the classification for each analysis without persistence. See 09 §5 for actions. |
| `mod.replaced` | warning | Community Use This Instead data (`replacements.json`, obtained through `archo data update`, 08 §2) identifies a maintained replacement for the current game version. If it is installed, suggest disabling the old Mod and enabling the new one; otherwise, provide a Workshop link. |
| `dep.missing` | error | The required package, including dependencies in `modDependenciesByVersion` for the current version, is absent from this environment. Use separate wording for missing DLC. Suggest `subscribe: <steamWorkshopUrl>` and `steam://url/CommunityFilePage/<id>`. |
| `dep.inactive` | error | The required package is installed but disabled. Suggest `archo enable` for each candidate. |
| `order.violated` | warning / info | The current order violates a constraint (loadBefore/After, a dependency, a community rule, or a user rule) that is not in a cycle. Emit one finding per edge, with key `<before-package>><after-package>` and declared, community, or user evidence according to the constraint kind. Severity follows constraint strength (08 §1.3): dependencies, loadBefore, forceLoad*, and strong user rules are warning; loadAfter, weak community rules, and constraints whose violation is harmless in the corpus are info. Suggest `archo sort --dry-run`, `archo move <A> --after <B>`, and `archo rule add ignore <ref>`. |
| `official.order` | warning | Core and DLC are not at the front of the list or not in release order. Combine all edges violating the built-in official order into one finding. The common corpus case is DLC enabled after the list was built and appended to the end (Odyssey is too late). Suggest the minimal `archo move` first, returning the newly enabled DLC to after the preceding official package (08 §1.3), then `archo sort --dry-run`. |
| `order.cycle` | info | The sorting constraints form a cycle. List each edge and its source. The game warns rather than sorting, so a cycle does not block writing or launch. See 08 §1 for proposed ways to break it and their ranking. The info basis is in §4.3.1. The can it run page still lists it, muted and uncounted, because only the player can decide which constraint to suppress. |
| `incompat.active` | error | Two `Mods` declared incompatible are both enabled. Suggest disabling one and `archo why A B`. |
| `version.unsupported` | warning / info | `supportedVersions` omits the current major.minor version. Do not check official content or `Mods` without this field. A Mod that stops at the previous minor version (game 1.7, Mod through `1.6`) is warning; one listing only earlier versions is info (log corpus, §4.3.1; the reverse of the old rule). |
| `pkg.duplicate-active` | error | Multiple Instances with the same `packageId` are enabled. All enabled copies are subjects. Suggest retaining only the first. |
| `pkg.duplicate-installed` | info | Multiple Instances have the same `packageId`, with at most one enabled. Mark a copy ignored by the game in the evidence only when another copy is one the game sees. Suggest `archo swap` only when one copy is enabled (`swap` needs an enabled copy); with none enabled, no fix. |

> Declaration violations are common and mostly harmless: half of the corpus logs violate dependency or load-order declarations, and 45% violate community rules. Above 100 `Mods`, logs with and without violations have the same error frequency. The most frequently violated constraints rarely involve the `Mods` identified as error sources; some constraints (modoptionssort before Core) cannot be satisfied at all. Sorting must therefore distinguish declarations that predict errors from declarations that are not enforced, with severity determined by the constraint kind (08 §1.3). See 18 for the numbers.

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

Only when the player opens the finding's details, or the CLI or MCP requests an individual finding, search the effective `Defs` of installed, loadable, disabled `Mods` for `Names` with no cached provider (`modsearch.FindNames`). Do not run this search during page load. Fresh facts already index the `Name` of every loadable Mod, including those deployed through links. The fallback reads only `Mods` whose facts may differ after enablement: those with a stale content key or conditional directories that would change. If there are no such `Mods`, read no files and conclude that no installed Mod, including disabled ones, provides the template. Apply the current game version, LoadFolders, and `MayRequire` conditions as they would be after enabling that Mod. Match only the `Name` of a top-level `Def`, not a `defName` or nested property. Do not access the network or write to the analysis cache. The search has a deadline and limits on the number of `Mods`, directory entries, files, and single-file and total bytes (see the implementation). It supports caller cancellation. If a limit is reached, a file is unreadable, or XML is invalid, retain the results and state that the search is incomplete; do not treat "not found" as "absent". The GUI uses an independent read session without holding navigation, edit, or log read locks. See 16 §5 for cancellation.

Uninstalled candidates come from the subject's dependency declarations (including those for the current game version), loadAfter and forceLoadAfter (including those for the current version), and cached `Def` `MayRequire` and `MayRequireAnyOf` values. If a declared URL or the existing community Steam DB package index identifies the Workshop ID, offer a subscription and page link; otherwise, offer a Workshop search. These are candidates, not confirmed providers of a template. Omit the candidates if every missing `Name` already has an installed provider. Always offer the subject's Workshop requirements link, falling back to a name search if there is no Workshop ID. Explain that the finding may concern harmless optional content, and offer a group ignore, only when existing, attributed logs matching the current list contain no error or warning involving that Mod. Continue to apply §4.2's update-expiry rule to each finding, without introducing a new ignore format. See 16 §5 and 13 §3.2 for the interface and command output.

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
| `asm.duplicate-name` | warning or info | Two or more enabled `Mods` ship a DLL with the same file name in their effective folders, with different type sets (`analyze/asmdup.go`, 2026-11-05). The game keeps the first copy in load order: `ModAssemblyHandler` loads each `Mod`'s DLLs with `Assembly.LoadFrom`, and an assembly name already loaded is not loaded again (MCP eval h2, real 1.6: a stale `OutfitStandsPlus.dll` in a patch `Mod` listed first hid four types of Outfit Stands Plus; listing Outfit Stands Plus first, or removing the stale copy, fixed it). Warning when a later copy defines types the loaded copy lacks; the claim names the loaded copy's `Mod`, the other copies and up to 4 lost types, and the evidence lists each copy's file, type count and list position and up to 20 lost types. Info when the copies differ but the loaded one has every type of the others. Identical type sets (a shared library such as `LunarFramework.dll`) are no finding. Types under `System.` and `Microsoft.` (what the compiler or a polyfill package adds, such as `Microsoft.CodeAnalysis.EmbeddedAttribute` and `System.Runtime.CompilerServices.RefSafetyRulesAttribute`) never count as lost: no XML or patch names them (VM round 2026-11-05: h2 stayed a warning on these two types after the fix). Matching is by file name, not by the assembly name in the metadata; they are the same in every case seen so far. Skipped libraries (`harmony.DefaultSkip`: `0Harmony.dll`, `HugsLib.dll` …) are left out: many `Mods` ship their own copy of another version, and those libraries manage it. One copy per `Mod` (the first of its folders). |

**Log layer**. Emit one `log.<category>` finding per error cluster (see 06 §1 for the categories).

| `code` | Severity |
| --- | --- |
| `log.exception`, `log.harmony-stack`, `log.xml-load` | error |
| `log.patch-failed`, `log.xref-missing`, `log.material`, `log.texture-missing`, `log.save-load`, `log.game-error` | warning |
| `log.mod-report`, `log.other`, `log.noise` | info, excluded from finding list |

#### 4.3.1 Severity Basis (Log Corpus, G4/G8)

Severity measures how strongly a finding predicts errors in the game log. It was tested with 6,734 public RimWorld `1.6` logs (docs/research/decisions-corpus.md §3, §2 G8). For each finding, check whether the involved `Mods` are attributed as error sources in the same log. Compare that rate with the rate for randomly choosing the same number of `Mods` (observed versus chance).

The rules are:

- **error**: the game will definitely have a problem (unmet declared dependencies, a declared incompatibility, or a missing parent `Def`), and the observed rate is clearly above chance (about 2× or more). Errors block pack building (10 §2), so this severity excludes problems that are merely possible.
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
| `official.order` | warning (formerly error) | 7.4% / 3.6% (2.1×) when the involved `Mods` are considered; paired comparison in 08 §1.3: 1.8× | This is comparable to dependency-order violations (1.4×, warning), rather than a certain game error. The overall 2.3% / 2.5% is unusable: half of the subjects are DLC, and Core and DLC are never attributed as sources. |
| `order.cycle` | info (formerly warning) | 6.8% / 6.1% (1.1×) | Cyclic lists run normally, with an error rate matching chance. Sorting drops the weakest constraint in the cycle (08 §1.2). |
| `patch.contested` | info | 41.9% / 6.6% (6.3×); without `patch.no-match` on the subject: 21.3% / 6.6% (3.2×) | Retain info. The high ratio mainly comes from no-match findings in the same `Mods`, already reported by `patch.no-match`. The remaining 3.2× resembles `def.override` (3.0×, info): shared edits are normal coexistence, with about 70,000 entries and 18 per log, and no useful action for an individual finding. |
| `def.override` | info | 24.6% / 8.1% (3.0×) | Retain the severity for the same reason as above (700,000 entries). |
| `harmony.shared-target`, `harmony.skip-prefix` | info | 2.7×, 2.6× | Retain the severity: these are the most numerous findings (1.39 million and 870,000), with no useful action for an individual finding. Log evidence promotes skip-prefix to warning (§4.5). |
| `harmony.manual`, `patch.unknown-op` | info | 1.6×, 2.5× | Retain the severity: these describe incomplete analysis, rather than problems themselves. |

Pre-launch findings (`LaunchChecks`, 09 §2) are selected using the observed and chance values in this table, rather than severity.

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
- **Collapse `version.unsupported`**: combine unmuted reminders for the same game version (§4.2) into one line, using the highest severity: `<sev> version.unsupported  N enabled mods do not list 1.7 (a list 1.6, b older only)  (archo problems --code version.unsupported)`. The CLI, MCP, and interface summaries are the same. Mod row tooltips and details state only the versions that the Mod declares (16 §5).
- **`harmony.multi-transpiler` defaults to info**, with the same escalation rule as skip-prefix below. It is not collapsed.
- **`harmony.skip-prefix` defaults to info and is collapsed**: combine its info findings into one summary (`… N bool prefixes from M mods can skip methods other mods also patch; none is in a log stack (archo problems --code harmony.skip-prefix)`). Promote a finding to warning only if the patched method, without its overload portion, appears in an error cluster of the **current, non-stale problem log**, either as a stack frame or as the owning frame of a Harmony patch line. Add the cluster as log evidence and put `archo log --cluster E<n>` first in the suggestions. List escalated findings individually. Show the full list with `--code harmony.skip-prefix`, `--all`, `--subject`, or `harmony --target`.
- Put summaries first within each severity so that the default 30-row truncation does not hide them.
- **MCP `list_problems`** defaults to `--min-severity warning` unless severity, `min_severity`, all, or `include_muted` is supplied.
- **`--all` folds info rows above 30** (MCP review round 2 P12): when `--all` without a code or severity filter would list more than 30 info findings, list info as one row per code (`info <code>  N problems  (archo problems --code <code> --all; MCP: list_problems code=<code> all=true)`) after the warnings. `--code`, `--subject` and `--severity info` list every row; a truncated list's `… N more (…)` command keeps those filters and adds `--all` (VM round 2026-11-05: `--code` pointed at a bare `--all`, which folded the code again). MCP eval h1: 8 `list_problems all=true` results of 20–25 KB, 137 info rows each, never used.
- **Log findings** are grouped by log (06 §4). Show the first 5 findings from the current log by default; show the rest with `archo problems --code log`. Collapse stale logs to an uncounted title, expanded with `archo problems --code log --all`; the title names the MCP form too (`MCP: list_problems code=log`), since MCP output is the CLI text. Exclude `log.mod-report`, `log.other` and `log.noise` from the finding list.
- **Ignored findings** are excluded from the default list (§4.2).
- **The can it run page** includes only errors and warnings the player can act on, leaving info findings in the technical details. See 16 §5 for the codes, grouping, and wording.

## 5. Analyzers

### 5.1 Def Layer

- Parse the XML as a stream, extracting only the fields in §2 without retaining a full DOM.
- Resolve inheritance after all patches have been applied, as the game does. Patches can change `Name` and `ParentName`; for example, `AttributeAdd` can name Core `Defs`. First collect all `Name` values in the patched documents, then resolve `ParentName`. For abstract nodes with the same name, the later node in load order overrides the earlier one. Checking before patches produces false positives (see 18 for measurements in a real environment).

### 5.2 Patch Simulation

1. Merge each Mod's effective `Defs` into one in-memory document with a `<Defs>` root, in plan order.
2. Apply each Mod's PatchOperations in the same order, recording how many nodes each operation matches.
3. Support `Add`, `Insert`, `Remove`, `Replace`, `AttributeAdd`, `AttributeSet`, `AttributeRemove`, `SetName`, `AddModExtension`, `Sequence`, `Conditional`, `Test`, and `FindMod`. Record other classes as `patch.unknown-op` and treat them as successful.
4. Interpret the success attribute (`Always`, `Normal`, `Invert`, `Never`) using the game's semantics.
5. The failure key combines the file and the game's description of the operation (`FailKey`), matching the failure messages in `Player.log` (06 §2).

**XPath**. The engine must match .NET XPath 1.0 behavior. Test compatibility with all XPath expressions in the real Mod collection and replay `Player.log`, simulating every logged failure. The mismatch rate must be below 0.5%, with differences documented. See 18 for the measurements. The common `Defs/XDef[defName="…"]` form uses an indexed fast path.

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
- **Game assemblies** (`app.indexGameTypes`, `store/gametypes.go`): every scan (04 §1.1) reads the type names (`dotnet.Assembly.TypeNames`) of `Assembly-CSharp.dll` and `Assembly-CSharp-firstpass.dll` in the scanned environment's Managed folder (`env.ManagedDir`: `RimWorldWin64_Data/Managed`, else the game folder in 1.7). Only these two: a save's `Class="…"` names a `Verse.IExposable` type, which only `Assembly-CSharp` and Mod assemblies can define, and the game resolves it only there and in mscorlib (17 §6 item 20); the Unity, Steamworks, audio and zip libraries beside them reference no game type. firstpass holds only Unity image effects in 1.6 and is indexed for completeness; the game does not search it for unqualified names.
  - **Storage**: `game_assembly` has one row per environment and file, with the game major.minor from `Version.txt`, the path, size, modification time and reader version (`analyze.Version`); `game_type` holds its type names. A scan rereads a file only when one of these changed, and removes the row when the file is gone. An unreadable file keeps its row with the error and no types, so it is not retried until it changes. An unknown game version leaves the rows as they are. `env remove` deletes the environment's rows. There are no fact sets: an environment has one game folder. Two environments of the same major.minor (different builds) answer together.
  - **Lookups** for `Store.MM` (`store/gametypes.go`): `GameTypeOwners(name)` returns the files defining a full name, with `TypeOwners`' nested-name fallback but ignoring case, like the game. `GameClassOwners(class)` resolves a save's `Class` value in the game's order (17 §6 item 20): the simple name of a type in `store.SaveNamespaces` or the global namespace (case-sensitive), else the full name, else the name prefixed by each of those namespaces; a generic value resolves by its definition. Mod types are not included: callers ask `TypeOwners` too. `GameTypesIndexed()` says whether a game of that version has its `Assembly-CSharp.dll` indexed; until then an empty answer says nothing (for example before the first scan after the upgrade to schema 30).
  - Measured cost and type counts: 18 §2 (game type index).
- The fixtures in `testdata/dotnet` contain fake `0Harmony` and `Assembly-CSharp` libraries, C# sources for four `Mods`, build scripts, and committed DLLs (4–5 KB each). Tests need no .NET SDK.

### 5.4 On-demand Decompilation

Use ilspycmd only when requested. The player may explicitly queue decompilation of the current list's Mods as a background task (16 §2.16); nothing decompiles automatically. ilspycmd exits when its work finishes.

- **Discovery**: look in `ARCHO_ILSPYCMD`, PATH, and `~/.dotnet/tools`. If ilspycmd is not found, report an error and provide `dotnet tool install --global ilspycmd`. Never install it automatically.
- **Reference directories**: use the game's Managed directory (the game root in 1.7), the DLL's directory, and the directory containing 0Harmony.dll.
- **Source library**: generated code is durable, regenerable, local-only data in the managed `source` directory (layout: 17 §3; relocation and cleanup: 16 §2.14). Cache clearing never touches it. On first opening, discard any old `<home>/cache/source` once.
- **Identity**: `source_library` maps the full DLL SHA-256 to a library-relative folder, kind, package ID, DLL name, game build, ilspycmd version and creation time, with stored size accounting. Game identities also include the build, so equal bytes from different releases can retain distinct build folders. `source_use` keeps past attempts, recording every `(hash, instance)` use: project/type attempts and successful shared hits. Upserts retain the latest status, mode and time per pair. History does not assert that output still exists.
- **Publication**: decompile into a unique temporary folder, write the completion marker there, then publish by rename. Type-only output lives under the DLL folder's `.types/<type-key>/`; once the project exists, use its files and discard type-only output. Concurrent requests share an artifact lock; moving and removal require exclusive library access. A changed DLL gets a new identity; old code remains until explicitly removed.
- **Game code**: on demand, read the configured environment's `Assembly-CSharp.dll` from its Managed folder (or game root in 1.7), with that directory as a reference. Store it per configured game build; a historical Mod-analysis version override does not select a different game installation. Packs, feedback attachments and list exports never include library contents.
- **Search**: group requested copies by DLL identity before dispatch and search each generated folder once. CLI/MCP return one set of matches with library paths and “same code in: …” attribution naming sharing copies. GUI/TUI attach the shared project matches to each matching copy in their streaming tree. A CLI/MCP dispatch budget stops new work only; already dispatched assemblies finish. Explicit cancellation stops ilspycmd and further file reads. Defaults and command shapes belong to 13 §4.2 and 14 §1.
- **Unused**: a Mod artifact is unused when no installed or locked copy contains its DLL hash, including inactive copies and other version folders. A game artifact is unused when no configured environment has its build. Check current DLL bytes across registered sources when cleanup is requested; abort if an installed source cannot be checked. Preview folder count and bytes, then remove only on explicit confirmation; never sweep automatically.
- MCP `read_source` with `search` defaults to `--implicated`. Only `scope: "active"` extends it to all enabled `Mods`. The default budget is 10 seconds. If it is exceeded, state how many assemblies still need decompilation ("still computing; run again").
- `archo files search` skips `Languages/` by default because translation files repeat every key; include them only with `--languages`. Traverse by Mod and use 8 workers to read files, preserving traversal order. See 13 §4 for command details and §5.5 for scope rules.
- **GUI/TUI**: Mod library › search files uses `App.SourceProjects` and `view.FileSearchRun` without a dispatch budget. Cached identities are visited first; missing projects are decompiled sequentially and retained. Stop, replacement and leaving cancel the context. Core in scope selects the game assembly (`FileSearchReq.Game` remains available to backend callers). The page contract, limits and polling are in 16 §2.9 and §2.16.1 G17.

### 5.5 Search Files Within Loaded Scope (2026-10-24)

The `files search` and `search_files` commands search only what the game actually loads and return results in load order (`internal/modsearch`):

- **Mods**: by default, search the current plan's enabled entries, using the exact copy in the list (Workshop, local, and locked versions are distinct), with its list position. Use `--mod` (repeatable) to name other copies; if several copies share a package ID, use the one in the list. Official content (Core and DLC) is included by default; `--game without|only` excludes it or searches only it.
- **Folders**: resolve each Mod's loading folders using §1: the version tags and conditions in `LoadFolders.xml`, or otherwise the version directory, `Common`, and the root. Within each loading folder, traverse only the subdirectories the game reads: `Defs`, `Patches`, `Languages`, `Textures`, `Sounds`, and `Assemblies`, plus `Lunar/Components`. Read `About/` only from the root. Do not search other folders under the root. For the same relative path, search only the copy with the highest priority. With `--all-versions`, search all version directories and every folder in `LoadFolders.xml`, ignoring conditions and retaining duplicates; mark each result with its folder. Silently skip nonexistent folders named by `LoadFolders.xml`. Count unreadable files and report the first error.
- **Kinds**: the default is `defs,patches,about`. Select kinds with `--kind`; add translations only with `--languages` or `--lang <language-folder>`. Match `Textures`, `Sounds`, and `Assemblies` by relative path without reading their binary content.
- **Matching**: the default is a case-insensitive literal match. Support `--regex` (RE2), `--case`, and `--word`. Count each line as one hit.
- **Results**: group by Mod (position, reference, name, package ID, copy, and loading folder), then file, then line. Hits in `Defs` and `Patches` include the containing top-level `Def` (`ThingDef/name`, or `ThingDef@Name` for abstract definitions) or the containing patch operation's XPath. Retain 5 lines per file and 40 lines per page by default (`--per-file`, `--limit`, `--page`); totals always cover all hits. `--mode files|count` lists only files or the count per Mod. If the default scope has no hits, explain which scopes were excluded.
- **Def mode** (`--def`): the pattern is a defName, matched as a case-sensitive whole word. By default, search only `Defs` and `Patches`. Each hit has a role: `def` (`<defName>`), `abstract` (`Name="…"`), `child` (`ParentName="…"`), `patch` (the containing operation's XPath includes the name), `patch-value` (the name is a value in a patch), `translation`, or `ref`. Precede the hits with a summary: definers (identify the later, effective definition if there are several), parents and children, patch operation and Mod counts, and the number of referencing `Mods`. This is textual evidence; use the `get_def` or `get_patches` simulation to determine whether a patch actually matches.

## 6. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
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

The acceptance tests for fact sets and cleanup are `TestFactSetsShareVersionsAndUpdates`, `TestFactSetsDifferentFoldersAndPriority`, `TestFactSetsHarmonyViews`, `TestFactPruneVisibilitySlotsAndEnvRemoval`, `TestFactSetsDifferentAssemblyInputs`; see 18 for scale measurements.
