# Cross-version analysis-fact sharing and cleanup measurements

2026-10-29. Synthetic data only; branch `codex/fact-share`, ran `git merge dev/0.9.0`, incorporating `aa5da8e` with v25. No VM, player game, Mods, or host RimArcho data directory used; no push. Specification contract: [05 §1–3](../specs/05-facts-and-findings.md). This report records measurements and tradeoffs.

## Results

4000 synthetic Mods, two game versions, 95 Defs and 20 Types per Mod. Both versions use identical folders; old structure: 760,000 Defs / 160,000 Types; now 380,000 / 80,000. In-use database size fell from v25's 215.7 MB to 66.6 MB (69.1% reduction); original branch base v24 was 250.9 MB. Median save for the second version: 0.457 seconds, versus v25's 5.50 seconds. In-place migration separately measured 4.33 seconds on an old-structure synthetic database with 760,000 Defs, retaining version views and fact fields without reanalyzing Mod files.

## Tables, keys, and reads

| Table | Ownership and key | Purpose |
| --- | --- | --- |
| `fact_set` | `id INTEGER PRIMARY KEY`, unique `(instance_id, kind, content_key)` | Input set for one Instance; `kind=def` or `dll` |
| `content_state` | Primary key `(instance_id, game_mm)`; references `def_set_id`, nullable `dll_set_id` | Current version's fact view; retains folders and parsing summary |
| `def_fact` | `set_id` foreign key | Original Def fields; Instance and version come through the view |
| `assembly_fact` / `type_fact` / `harmony_fact` / `harmony_id` | `set_id` foreign key | DLL facts; cascading cleanup when a set is deleted |
| `analysis` / `patch_fact` / `finding` | Original environment / version / enabled-set slot | Patch simulation and finding semantics preserved |

Def input key consists of snapshot hash, ordered effective-folder-list hash, and analyzer version. Folders are cleaned and path separators normalized first; priority and duplicates are retained. Version and condition text only select folders, without creating another copy of identical content. Sets remain scoped to one Instance; identities of different copies are not mixed, and Instance IDs were not converted to integers.

DLL input key consists of snapshot hash, effective DLL relative paths and whether each DLL skips Harmony parsing, and analyzer version. Different XML folders can share the same DLL set; package-identity-based HugsLib skipping and custom skip lists are reflected in the key. Enabling with no DLLs also creates an empty set, replacing the old DLL view.

`DefsOf`, `DefsNamed`, `DefCounts`, `DefCount`, assembly, Type / Harmony ID owner, and Harmony queries all read through `content_state` using `Store.MM`. Patches, why, logs, target-inactive, modsearch, and Name lookups continue using these interfaces or effective-folder resolution; the parent-template lookup fast path supports the new key. `CachedDefs` can reuse another version's set by Instance and content key.

Only indexes used for set deletion/enumeration, name lookup, and version-view joins remain. `def_fact_instance` and old version composite indexes disappear with old tables; `Name` uses partial index `def_fact_named(name,set_id) WHERE name<>''`. Type / Harmony ID queries always fetch facts by name first, then join version views, preventing SQLite from choosing per-Mod scans. Index-plan tests cover these paths.

## Cleanup and product decisions

`Store.PruneFacts(now time.Time) error` is an independent, transactional, repeatable function for periodic maintenance and the compact entry point. `SaveAnalysis`, environment removal, gc, and link identity merging already call cleanup in place.

1. Saving new analysis for an environment removes enabled-set slots unused by any plan across all that environment's versions; each slot retains only the latest run.
2. Slots for noncurrent game versions expire more than 30 days after their last completed analysis; current-version slots do not expire by time. Reads do not renew the last-analysis time. Unknown game versions are retained conservatively.
3. Remove `(instance, mm)` views when the Instance no longer exists, or no visible environment has either current game version mm or an mm analysis slot. Shared Instances are visible to all environments; private Instances consider only their owning environment. Unknown game versions conservatively retain visible instance views.
4. Delete sets unreferenced by any version view, cascading to their facts. Once the other log-ownership branch fixes misuse, wrong-version slots no longer updated can expire under the 30-day policy; replacing old-plan slots or removing environments can also clean them immediately.

30 days is the product suggestion adopted for this task: preserve the benefit of repeated recent cross-version log analysis while bounding misused slots. No player setting was added. Old sets are deleted only when unreferenced: if another valid version view still references an old set after one version updates, retain it until that view updates or is no longer needed.

**The conditional-folder limit for different plans on the same version remains.** Removing version from the content key eliminates duplicate cross-version data for identical folders, but `content_state` still has only one `(instance, mm)` view, not a per-plan view. The last analyzed plan on a version replaces the view; returning to another plan with an already-fresh slot may still show the previous plan's Defs. Sharing did not introduce or claim to solve this issue; a solution needs an analysis-slot-to-set mapping and read context.

## Migration

`schemaV26` / `migrateV26` rebuild fact tables in one transaction. Snapshot and analyzer versions are extracted from old keys and combined with stored ordered folders to merge Def sets. Old DLL records lack input skip policy, so sharing conservatively uses snapshot, analyzer version, and a full DLL-fact fingerprint; the next analysis replaces this with a normal DLL input key. Unrecognized old keys remain separate by version. Orphan facts without `content_state` are not migrated. Query-equivalence tests cover defName, Name, empty Def sets, nested Type names, Harmony ID case, manual Harmony patches, assembly flags/fields, and isolation of different snapshots/analyzer versions.

| Synthetic migration database measurement | Value |
| --- | ---: |
| Physical / in-use size before migration | 196,337,664 B |
| In-place migration | 4.3277 s |
| In-use size after migration | 63,344,640 B |
| Physical size after migration | 259,330,048 B |
| Reusable free pages within it | 195,985,408 B |
| Physical size after separate compaction | 60,465,152 B |

The migration sample was SQL-generated with shorter file paths and snapshot keys; it differs from the analysis benchmark database, so sizes cannot be directly subtracted. Migration temporarily grows disk usage; transaction failure rolls everything back. Startup migration does not run `VACUUM`; later maintenance shrinks physical size. Old DLL sets are replaced by their views on the next analysis and removed when unreferenced.

## Analysis and query measurements

Windows / Go 1.27.2, modernc SQLite, `internal/testkit` synthetic directories. Median of three independent groups per implementation; queries repeat 10 times per group (190 Name queries, DefCounts) or 100 times (others), with the median of group means shown. Disk pages measured after checkpoint. Includes actual XML parsing for 4000 Mods and saving additionally constructed DLL facts; existing synthetic-DLL regressions cover DLL metadata parsing performance. OS file cache and concurrent machine load were not fixed, so microsecond differences indicate scale only.

| Metric | v25 | v26 |
| --- | ---: | ---: |
| In-use database size | 215,703,552 B | 66,596,864 B |
| 1.6 analysis (before saving) | 1.471 s | 1.208 s |
| 1.6 save | 5.331 s | 3.258 s |
| 1.7 analysis (before saving) | 1.363 s | 1.111 s |
| 1.7 save | 5.502 s | 0.457 s |
| 1.6 analysis + save, median of same-group totals | 6.941 s | 4.531 s |
| 1.7 analysis + save, median of same-group totals | 6.865 s | 1.568 s |
| 190 distinct Name queries | 6.297 ms | 5.589 ms |
| One defName query | 30.47 µs | 25.17 µs |
| 95 Defs for one Instance | 172.49 µs | 172.33 µs |
| Whole-database DefCounts | 79.40 ms | 58.89 ms |
| TypeOwners | 30.76 µs | 25.42 µs |

Three-group medians at v24 base: 250,933,248 B; 1.6 save 5.758 s, 1.7 save 5.990 s; 190 Name queries 4.184 ms. v25 index reductions and this v26 set sharing are listed separately to avoid attributing v25 gains to this task.

No regression in key queries or total analysis cost was observed. About 5.6 ms for 190 Name queries cannot alone exhaust the 500 ms first-log budget; this validates storage reads only, not full-log/GUI end-to-end latency or native-window working-set VM acceptance. This task did not run the VM per its limits; the corresponding row was added to the prerelease checklist.

## Tests and reproduction

`go test ./...` passed; `node --test "scripts/vm/*.test.mjs"` passed 42 / 42.

Added/adjusted coverage: identical folders across two versions stored once; different folders/override order stored separately; old sets removed only after the final reference disappears following Mod updates; environment/plan-slot removal/30-day expiry; shared/private visibility; DLL sharing across XML folders, skip lists, empty-DLL replacement; migration field/query equivalence; index query plans. `TestAnalysisPerGameVersion`, Name lookups, and log regressions remain passing.

Before reproducing, point TEMP, TMP, APPDATA, LOCALAPPDATA, and USERPROFILE at the task's temporary directories; **do not set `RIMARCHO_HOME`**. Existing GUI `TestUseInstalledRealCopy` treats it as an explicit opt-in for real-backup acceptance. The first full run set it and was rejected by that test's marker check without accessing the backup; reran after unsetting it. Ordinary tests use their own testkit data directories.

```powershell
$base = 'D:\RimArcho-test\fact-share'
$env:TEMP = Join-Path $base 'tmp'
$env:TMP = $env:TEMP
$env:APPDATA = Join-Path $base 'appdata'
$env:LOCALAPPDATA = Join-Path $base 'localappdata'
$env:USERPROFILE = Join-Path $base 'profile'
foreach ($p in @($env:TEMP, $env:APPDATA, $env:LOCALAPPDATA, $env:USERPROFILE)) {
    New-Item -ItemType Directory -Force -Path $p | Out-Null
}
Remove-Item Env:RIMARCHO_HOME -ErrorAction SilentlyContinue
go test ./...
node --test "scripts/vm/*.test.mjs"
$env:RIMARCHO_FACT_BENCH = '1'
go test ./internal/store -run '^TestFactShareScale$' -v -count=3
go test ./internal/store -run '^TestMigrateV26' -v -count=1
Remove-Item Env:RIMARCHO_FACT_BENCH
```

For before-change measurements, use `git archive` to copy the corresponding commit under the same temporary root, then copy current `factshare_bench_test.go` into its `internal/store/`. v24=`de2fa73`, v25=`aa5da8e`. Benchmarks skip by default; only explicit `RIMARCHO_FACT_BENCH=1` generates scale databases, cleaned automatically afterward. archo.exe was not run; if run separately, all four user-specified isolation variables must be set.

## Main change entry points

| File / function | Change |
| --- | --- |
| `internal/content/content.go`: `Resolved.Key`, `FolderKey` | Effective-folder key |
| `internal/analyze/analyze.go`, `harmony.go`: `ContentState`, `AssemblyKey` | DLL input key and empty-input state |
| `internal/store/migrate26.go`, `store.go` | v26 structure, migration registration and conversion |
| `internal/store/savecontent.go`, `analysis.go` | Reused-set saving and all fact reads |
| `internal/store/factsets.go` | Cleanup, independent entry point, and historical migration-deletion compatibility |
| `envs.go`, `mutate.go`, `migrate14.go`, `linkedinstances.go` | Natural cleanup points |
| `internal/app/parentproblems.go` | Name-lookup cache-key validation |
| `internal/testkit/legacyfacts.go`, `testdata/store/facts-v25.sql` | Existing migration tests restore actual old table structures |
| `factsets_test.go`, `migrate26_test.go`, `factshare_bench_test.go`, existing migration/index/WAL tests | Behavior, query, and scale validation |

Specifications, architecture references, CHANGELOG, and VM checklist accompany code in the same commit.

## Conflicts between code and original documentation

Original 05 §2 stated `analyze.Version=4` and a version on every fact; actual code was version 8, and fact rows had no separate snapshot/analyzer fields, only old content keys and run records. Changed to references to `analyze.Version` and set associations.

Original 05 §3 stated both "only the latest run per `(environment, mm)`" and "multiple slots by enabled set". Actual code uses set-based slots; the multiple-slot contract now governs. Old-plan removal previously cleaned only the current mm; this explicitly cleans across all mm of that environment.

Original 17 §7.4 / 00a descriptions of facts physically partitioned by version now reference content sets and version views. Historical measurement tables remain; historical index-improvement figures were not rewritten as current structure.

## Evidence chain

Raw output is in the matching `fact-share-evidence/` directory; checksums in `sha256.json`.

| Evidence | Source and reproduction | Observation |
| --- | --- | --- |
| E-001 | `v24.log` / `v25.log` / `v26.log`; three `TestFactShareScale` groups | Row counts, sizes, save/query times for the same scenario |
| E-002 | `migration.log`; `TestMigrateV26Scale` | Time and space for 760,000-row in-place conversion; matching queries |
| E-003 | `fact-tests.log`; `go test ./internal/store -run 'Test(Fact|MigrateV26Fact)' -v` | Folder, update, visibility, slot, and DLL boundaries passed |
| E-004 | `node-test.log`; `node --test "scripts/vm/*.test.mjs"` | 42 passed, VM not run |

E-005: `go-test.log`, full `go test ./...` passed (real-backup acceptance not enabled).

Finding F-001: E-001 / E-002 support input-set sharing reducing duplicate data with fast conversion, status validated. Finding F-002: E-003 supports reference/visibility cleanup preserving version facts still needed, status validated. Call path P-001: `analyze.Run` generates input key → `SaveAnalysis` / `saveContent` finds or creates a set → `content_state` updates version reference → `PruneFacts` removes unneeded views → unreferenced sets cascade-delete. Boundaries are unmeasured VM end-to-end latency and the plan-view limitation above.
