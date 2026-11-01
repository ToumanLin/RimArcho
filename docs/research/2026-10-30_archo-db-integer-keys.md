# Replacing text IDs and paths with integers in archo.db: research

2026-10-30. Answers five questions in `ToDo.md`, "research: replace text IDs and paths with integers in archo.db". Research/measurement only, **no production schema/migration changes**; specifications wait for review of conclusions. Branch `claude/db-intkey-research-77d962`, base `64e7182` (dev/1.0, schema v26, `codex/fact-share` and `codex/db-maintenance` merged). Synthetic data only; no `%APPDATA%\RimArcho`, real game/Mod access.

## Conclusions

On a synthetic database matching ToDo real row counts (245.5 MB):

| Question | Recommendation | Key figures |
| --- | --- | --- |
| 1 Where surrogate keys belong | **No Instance integer key.** v26 `fact_set` already maps fact tables; fact rows carry only integer `set_id` | Integer keys save0.99 MB (0.4%); remaining Instance text totals0.85 MB |
| 2 Path-table granularity | **No global/per-Mod path table.** Fact-file paths use one list per fact set (`fact_set.files` + row index); snapshot manifests per question 3. No separate old-path cleanup: lists die with sets, manifests follow02 §7.1 | Set lists alone−38.9 MB (−15.8%); with question 3−92.9 MB (−37.8%). Global table saves only 4.5 MB more, adds0.43-second maintenance scan, doubles manifest-write time |
| 3 Compressed nonsemantic file blocks | **Yes.** One zstd block/snapshot (prefix-compressed paths+sizes, no mtime, DLLs remain rows); snapshot `kinds`/`langs` computed during scan | −51.4 MB (−20.9%); first inventory content-kind+PNG queries **263+22 → about 3ms**; texture-provider logs+40–50ms, DLL lookup~3× faster |
| 4 Name-only disabled Mods | **No.** `archo def`, two log explanations, detail Defs need disabled-Mod file: line; missing-parent providers need MayRequire; only parent/abstract unread | Alone−25.3 MB, but indexed paths leave only−2.4 MB (1.6%); parent/abstract alone−2.7 MB |
| 5 Query/migration cost | Recommended design: name lookups, cached Defs, incremental scans within noise; pages faster | Conversion8.4–11.3s (one transaction, no VACUUM), later VACUUM1.6–1.9s; same real row counts imply comparable scale, show startup progress |

Three other code/document conflicts or independent fixes (§1, §7.2): `def_fact` already lacks `instance_id`; nonsemantic mtime unread; `Store.InstanceFiles` misses Workshop Mods since v14; `Store.DefsNamed` query plan makes name lookup~30× slower.

## 1. Correcting ToDo assumptions (code/document conflicts)

ToDo numbers came from a read-only measurement of a 10-day real database (443 MB) on 2026-10-29. Current source comparison:

1. **`def_fact` no longer has `instance_id`.** v26 (`store.schemaV26`) gives facts integer `set_id`, with Instance in `fact_set(id, instance_id, kind, content_key)`. ToDo's "19.8 MB instance_id duplicated in two indexes" predates v26. Instance text left across facts/snapshots/analysis/events totals about **0.85 MB** (§3).
2. **No code reads nonsemantic-file mtime.** ToDo says72% of rows serve incremental mtime comparison. Actually only semantic hashes reuse: `scan.ComputeSkip` compares size+mtime only if `IsSemantic(rel)` and prior hash (`internal/scan/fingerprint.go:115-117`); `Store.CurrentFiles` only `hash IS NOT NULL`. Nonsemantic texture/audio/Languages mtime only written/copied; real readers query paths (§5.1).
3. **`Store.InstanceFiles` queries environment-private Mods only.** `i.environment_id=?` (`internal/store/files.go:9`), but since v14 (2026-10-22) shared-folder Workshop/SteamCMD Instances have empty `environment_id`. Function written10-21 before v14, unchanged/untested. Log attribution "who supplies this texture/DLL" (`logrefs.go:410`, `logrefs.go:517`, `logcallers.go:294`) misses Workshop Mods. Apparent v14 regression, outside scope/unfixed. Benchmarks use intended `environment_id IN ('', ?)` (more results/slower).
4. **07 §6.2's190 distinct Name queries5.589ms no longer holds**, §7.2.

## 2. Method

### 2.1 Synthetic database

Measurement `TestIntKeyResearch` across `internal/store/intkey_research_test.go` (synthetic DB), `intkey_convert_test.go` (prototype conversion), `intkey_query_test.go` (queries/writes), `intkey_main_test.go` (main flow), opt-in `RIMARCHO_INTKEY_BENCH=1`, outside production paths. `testkit.New` creates actual v26 database with migrations/indexes/triggers, then bulk-fills real-database shape:

| Item | ToDo real DB | Synthetic DB |
| --- | ---: | ---: |
| Instance | 1354 (07 §6 real: 6 official, 74 local, 1274 Workshop) | 1354, same distribution, 482 enabled |
| `snapshot_file` rows | 980,000 | 973,764 (4,864 snapshots: 1354 current+history) |
| `rel_path` total/mean | 59.6 MB / about 61 B | 56.9 MB /58.4 B, 271,102 distinct paths |
| Nonsemantic share | 72% | 71.7% (697,889 rows) |
| `def_fact` rows | 760,000 | 771,016 (1,603 Def sets), enabled Mods 40.4% |
| `def_fact.file` total/mean | 33.6 MB /44 B | 34.8 MB /45.1 B, 49,941 distinct files (15.4 Def/file average) |
| `type_fact`, `harmony_*` | About 200,000 | 207,073+30,462+1,275 |
| Other | — | `patch_fact`16,299 (3 slots), `finding`2,100, `content_state`2,708 |

Paths model real Mods, e.g. `1.6/Defs/ThingDefs_Buildings/XYZ_Buildings_Kamelvan.xml`, `Textures/Things/Building/Furniture/XYZ_Torix/XYZ_Torix_north.png`, `Languages/ChineseSimplified (Simplified Chinese)/DefInjected/ThingDef/…`.45% have version folders, 40% of those `1.7/`, with separate1.7 sets; others share across versions (~82% shared, assumption without real proportion). Historical snapshots change5% semantic files, remove1%, add0.5%.

**Synthetic first `ContentKinds`236–294ms, real 246ms (07 §6.2)**, suggesting similar manifest shape.

### 2.2 Prototype structures

Each prototype `VACUUM INTO` copies baseline, converts in one timed transaction (migration), checkpoints/`VACUUM`s, then `dbstat` tables/indexes. Queries follow existing `internal/store` SQL adapted to layout, checksummed against baseline; all match except deliberately removed fields (`*`).

| Name | Change |
| --- | --- |
| `v26` | Baseline/current structure |
| `v26-copy` | Unchanged copy opened like prototypes, noise control |
| `ik` | Integer Instance surrogate: `instance_key(n INTEGER PRIMARY KEY, id TEXT UNIQUE)`; integers in `fact_set`, `content_state`, `patch_fact` |
| `path-global` | Global `path(id INTEGER PRIMARY KEY, rel TEXT UNIQUE)`; integer fact `file`/`snapshot_file.rel_path` |
| `path-hash` | Same, 64-bit hash/index replaces text-unique index: `path(id, h, rel)` + `path_h(h)` |
| `ik+path-mod` | Per-Mod `mod_path(inst, id, rel, PRIMARY KEY(inst,id)) WITHOUT ROWID` (requires `ik`) |
| `setfiles` | Per-set `fact_set.files` JSON array, row indexes; read list once/set, restore paths in Go |
| `blob` | Nonsemantic except DLL rows become zstd/snapshot: prefix paths+sizes, no mtime; snapshot `kinds` bits/`langs` computed at conversion/write |
| `blob-dedup` | Same, identical blocks stored once (`file_blob`, xxh3-128) |
| `slim-names` | Disabled-Mod Def type/defName/Name only; clear `file`, `line`, `parent`, `abstract`, `may_require*` |
| `slim-parent` | Clear only disabled `parent`/`abstract`, unread fields (§6) |
| `combo-global` | `blob-dedup` + `path-global` + `slim-parent` |
| `combo-hash` | `blob-dedup` + `path-hash` + `slim-parent` |
| `combo-ik-mod` | `blob-dedup` + `ik+path-mod` + `slim-parent` |
| `combo-names` | `blob-dedup` + `path-hash` + `slim-names` |
| `combo-setfiles` | `blob-dedup` + `setfiles` + `slim-parent` |

**Definitions**: Windows 11/Ryzen 9 7950X/63 GB/Go 1.27.2/modernc SQLite, default 2 MB page cache, warm OS cache. One warm/checksum query then median 3 rounds; fast queries repeat≥30ms then average (Windows clock~0.5ms). Writes transactionally rolled back. Four runs1–4, ranges shown. VM and other build/test sessions active: **identical `v26`/`v26-copy` differ20–40%, some queries nearly2× across runs**, so count only multiplicative differences. Post-VACUUM sizes identical each run, unaffected by noise.

## 3. Question1: surrogate-key placement

**Recommend no additional Instance integer surrogate.** Proposed fact mapping already exists as v26 `fact_set`: fact rows only `set_id` (1–2-byte integer), set records Instance.

| Structure | DB size | Change | Migration |
| --- | ---: | ---: | ---: |
| `v26` | 245.47 MB | — | — |
| `ik` | 244.48 MB | **−0.99 MB (−0.4%)** | 0.07–0.09s |

Queries within noise (§7.1 `ik`). Remaining Instance text0.85 MB across `fact_set`1,895 rows, `content_state`2,708, `snapshot`4,864, `patch_fact`16,299, `finding`, `change_event`. Largest patch_fact scoped to analysis slots, deleted with them.

Integer keys/identity rules (review `internal/store/mutate.go`, `migrate14.go`, `linkedinstances.go`, `identityrefs.go`):

- **same answer** retains old candidate/tombstone row/text ID, removes new Instance; rewrites the `instance_id` in `snapshot`, `change_event`, `instance_alias` to the candidate, plan (`workspace`, `workspace_undo`, `loadout_version`)/launch fingerprints via `rewriteInstanceLists` (`identityrefs.go`). The new Instance's `fact_set`/`content_state` are not rewritten, left for `PruneFacts` to delete and rebuilt on the next analysis (AnalysisKey includes Instance ID/snapshot hash). Integer UPDATE changes column, no simplification.
- **v14 merge, `MergeLinkedInstances`** likewise rewrite text IDs, retain earliest live copy, delete merged copy's facts entirely.
- **Lineage split/merge** changes only `instance.lineage_id`, not Instance ID, unrelated to surrogate.
- **External references text**: plan JSON, `id_alias`, `log_cache` reports, pack manifests, `#a3f9` short refs (05 §2). Integer adds internal second identity requiring synchronization on every rewrite path.
- **Migration renumbering** safe for internal-only integers, as `fact_set.id` already renumbered by v26. Integer caches (`ContentKinds` key `(snapshot_id, scan_id)`) live one process only.

Only per-Mod path tables (§4) require integer Instance keys: a 26-byte text ID prefix in `mod_path` costs about 7 MB across 270,000 paths. Section 4 does not recommend that design.

## 4. Question2: path-table granularity

### 4.1 Path tables alone (uncompressed manifests)

| Structure | Size | Change | `def_fact` | DLL facts | `snapshot_file` | Path table/list | Conversion |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `v26` | 245.47 | — | 114.67 | 33.75 | 84.04 | — | — |
| `path-global` | 187.81 | −57.7 (−23.5%) | 81.04 | 27.45 | 28.49 | 37.81 | 7.0–7.2s |
| `path-hash` | 176.03 | −69.4 (−28.3%) | 81.04 | 27.45 | 28.49 | 26.03 | 10.1–10.6s |
| `ik+path-mod` | 165.81 | −79.7 (−32.5%) | 79.94 | 27.08 | 27.24 | 19.53 | 10.3–12.2s |
| `setfiles` | 206.56 | −38.9 (−15.8%) | 79.81 | 26.89 | 84.04 | +2.8 (sets+views1.09→3.90) | 3.4–4.5s |

MB. Global unique text index `sqlite_autoindex_path_1` alone18.9 MB, duplicating paths; hash index4.6 MB. Per-Mod smallest, no text lookup index, loads whole Mod path list for writes. `setfiles` changes only facts, comparable fact savings (79.8 vs81.0 MB), manifests unchanged.

**Cost**: integer paths require joining path table for pattern queries.

| Query ms | `v26` | `path-global` | `path-hash` | `ik+path-mod` |
| --- | ---: | ---: | ---: | ---: |
| `ContentKinds` all/uncached (first inventory open) | 236–294 | 398–443 | 524–618 | 626–777 |
| `SnapshotsWithPNG` all | 21–31 | 47–52 | 67–89 | 31 |
| `InstanceFiles("%Textures/%")` (log texture lookup) | 115–209 | 295–481 | 328–357 | 314–338 |
| `InstanceFiles("%x.dll")` ×20 (log DLL lookup) | 1,435–1,812 | 4,599–4,759 | 4,950–8,050 | 1,500–1,728 |
| Write 30 snapshot manifests | 20–32 | 75–86 | 197–214 | 26–39 |

Thus **do not ship manifest path tables alone**: first inventory adds0.1–0.5s, farther from 06 §10's200ms. Combine §5 derived columns/blocks; then only semantic/DLL rows remain11 MB, table savings few MB (§8).

### 4.2 Choosing granularity

- **Per snapshot** is existing `snapshot_file`, duplicated paths each snapshot: 970,000 rows, 270,000 distinct (historical snapshots mostly match). Does not save manifests; pointing `def_fact.file` to snapshot-file rows ties fact lifetime to manifests, whose old manifests are pruned by 02 §7.1 while another game version's `content_state` may still reference the old set. **Rejected**, no separate prototype. Nonsemantic block/snapshot (§5) is compressed version of this granularity, adopted.
- **Per Mod** smallest but needs integer Instance (§3); same/v14/link merges move snapshots with original path IDs, requiring renumbering. `CachedDefs` (all disabled cached Defs each analysis)494–613→1,039–1,114ms, `DefsOf` slower due per-row `(inst,id)` lookup. **Rejected**.
- **Global** needs no identity-merge rewrite, paths once across Mods/snapshots; per-path write lookup makes manifests2–4× slower, maintenance scans all references (§4.3).
- **Per fact-set file list** (`setfiles`, additional option): content-keyed immutable sets own facts; list/facts share lifetime: **no cleanup, merge renumbering, write lookup**. Read once/set, cache by immutable `set_id`.

**Recommend per-set fact lists and per-snapshot manifest blocks (§5), no global/per-Mod tables.** Combined comparison:

| | `combo-setfiles` | `combo-global` |
| --- | ---: | ---: |
| DB size | **152.58 MB** (−92.9,−37.8%) | 148.12 MB (−97.4,−39.7%) |
| `snapshot_file` | 21.77 (semantic/DLL text) | 11.03 (path IDs) |
| Path table/list | 2.8 (in `fact_set`) | 7.52 |
| Old-path maintenance | None | 0.42–0.68s each |
| Write 30 snapshot manifests ms | 16.9–22.7 | 44.1–50.0 |
| Conversion | 8.4–11.3s | 7.9–8.6s (busy run 4:14.3) |

Global saves another4.5 MB (1.8%) at cost of global table/maintenance step/write lookup, not worthwhile.

### 4.3 Old-path cleanup timing

Recommended approach needs no separate rule:

- File list lives in `fact_set` row, deleted by `PruneFacts` with set (03 §3: immediately when no version references). After update, delete old set when all version views move on.
- Nonsemantic blocks follow snapshot manifests, 02 §7.1 retention (current, unanswered questions, before/after last3 updates per Instance), deleted in `Store.Maintain` step 1.

If global chosen, clean after `Store.Maintain` (02 §7.2) steps1/2 in same transaction: `DELETE FROM path WHERE id NOT IN (SELECT path_id FROM snapshot_file UNION ALL SELECT file_id FROM def_fact … )`. Even no deletions scans refs: uncompressed0.83–0.92s, with blocks0.42–0.68s. Not during save/scan; table grows between maintenance.

## 5. Question3: nonsemantic files in compressed snapshot blocks

### 5.1 Identify path-query readers first

| Reader | Needed rows | When | Scope | After blocks |
| --- | --- | --- | --- | --- |
| `Store.CurrentFiles` (incremental scan) | Semantic only | Scan | All environment current snapshots | Unchanged |
| `semanticSet`, `SourceSemantics` (reconcile/compare/rebind) | Semantic only | Scan/player operations | One snapshot/source | Unchanged |
| `Store.ContentKinds` (`content.go`, inventory/`type:`/`has:`) | Any DLL, Defs/Patches XML, Languages | Page load, process snapshot cache | All current | Read precomputed snapshot `kinds` |
| `Store.SnapshotsWithPNG` / `SnapshotsWithPNGFor` (`textures.go`, `content.go`, inventory/textures) | `%textures/%.png` | Page load, `SnapshotsKey` cache | All/specified | Read PNG bit in `kinds` |
| `Store.Languages` (`library.go`, details) | `%languages/%` | Open details | One snapshot | Read `langs` |
| `Store.InstanceFiles` (`files.go`, log texture/DLL providers) | Pattern | Analyze log, cached report | All live current | DLLs remain rows; textures decode all current blocks |
| `Store.SnapshotFiles` (`changes.go`, file differences) | All paths/sizes/hashes, no mtime | Open changes/row | Two snapshots | Rows+two decoded blocks |
| `persist.go` (same-content external-state copy/mtime rewrite) | All | Scan | One snapshot | Copy block (reference only if dedup) |
| `maintenance.go`, `mutate.go`, `envs.go` | Delete | Maintenance/gc/remove env | — | Delete blocks too |

`files search`/MCP `search_files` (`internal/modsearch`) reads disk, not `snapshot_file`. Nonsemantic `size` used only changes-page size differences, `mtime` unread (§1).

### 5.2 Figures

| Structure | Size | Change | `snapshot_file` | Blocks | Conversion |
| --- | ---: | ---: | ---: | ---: | ---: |
| `v26` | 245.47 | — | 84.04 | — | — |
| `blob` | 195.09 | −50.4 (−20.5%) | 21.77 | 11.89 | 4.0–5.6s |
| `blob-dedup` | 194.11 | −51.4 (−20.9%) | 21.77 | 10.91 | 4.2–4.3s |

698,000 nonsemantic non-DLL rows: 45.7 MB path text→19.4 MB prefix+size→zstd **10.1 MB (~14.5 bytes/row)**. Including mtime gives15.2 MB, 50% more, omitted. About 1/3 of 4,864 blocks identical (3,312 unique). Synthetic historical snapshots change1.5% paths; real XML/DLL-only updates may dedup more, but here only 1 MB saved.

| Query ms | `v26` | `blob-dedup` | `combo-setfiles` |
| --- | ---: | ---: | ---: |
| `ContentKinds` all/uncached | 236–294 | **4.5–5.8** | **1.9–2.7** |
| `SnapshotsWithPNG` all | 21–31 | 3.9–4.6 | 0.9–1.1 |
| `Languages` × 100 | 11.6–21.1 | 0.57–0.65 | 0.82–0.87 |
| `InstanceFiles("%Textures/%")` | 115–209 | 142–166 | 159–211 |
| `InstanceFiles("%x.dll")` × 20 | 1,435–1,812 | 436–512 | 424–607 |
| `SnapshotFiles` × 100(changes page) | 17–28 | 12.7–14.7 | 15.2–18.3 |
| `CurrentFiles`(incremental scan) | 95–137 | 79–91 | 75–122 |
| Write 30 snapshot manifests | 20–32 | 14.0–14.5 | 16.9–22.7 |

**Recommend blocks+derived columns, exclude DLL/mtime, dedup optional.** Main page gain derived columns: first inventory content-kind~260ms→few ms (real 246, 07 §6.2). Independent of blocks, can ship first: snapshot `kinds`/`langs`, compute on write/backfill migration. Only texture-provider logs slower+40–50ms, once/log then cached. DLL lookup occurs per missing assembly; putting DLLs in blocks would decode>20,000 for 20 queries. Remaining rows fewer, DLL queries~3× faster.

## 6. Question4: name-only disabled-Mod Defs

### 6.1 Readers/fields for disabled Mods (source review)

| Feature | Fields |
| --- | --- |
| `target-inactive` (`analyze/facts.go` `knowledge`/`reasons`) | Type/defName/Name. Patch simulation/parent expansion enabled only; disabled feed disposable draft |
| `archo def X`/MCP `get_def` "also-in (not enabled)" | **file: line** (`cmds/content.go:141`) |
| Log xref-missing "defines … at file: line but is not enabled" (`app/logs.go:758-771`) | Type, **file, line** |
| Log missing parent "provides the abstract def … (file: line)" (`app/logs.go:811-818`) | **file, line** |
| Detail Defs (`view/detail.go:333` calls `archo def --mod`, any Mod) | Type/defName/Name, **file, line** |
| Missing-parent provider (`app/parentproblems.go:147`) | **MayRequire/MayRequireAny** (`Loaded`) |
| `why`, `explain_relation` | Type, defName, Name |
| `Parent`, `Abstract` | **No disabled-Mod readers**: displayed/`defFindings` enabled only |

No code follows ParentName→Name inheritance chains in fact tables.

### 6.2 Figures

| Structure | Size | Change | `def_fact` |
| --- | ---: | ---: | ---: |
| `v26` | 245.47 | — | 114.67 |
| `slim-names` | 220.19 | −25.3(−10.3%) | 89.39 |
| `slim-parent` | 242.81 | −2.7(−1.1%) | 112.01 |
| `combo-hash` (blocks+hash paths+`slim-parent`) | 146.26 | — | 78.33 |
| `combo-names` (same, `slim-names`) | 143.88 | Another2.4 below previous | 75.95 |

**Reject.** Name-only alone saves25 MB mostly `file` text; indexed paths leave2.4 MB (1.6%), losing file: line in4 visible outputs and misjudging parent providers without MayRequire. Sets share `(Instance,kind,content key)` (`ensureFactSet` `ON CONFLICT DO NOTHING`): same Mod enabled/disabled in different plans has same key, so name-only set may serve enabled analysis. Requires separate kind/two copies, larger. Parent/abstract-only changes no output but saves2.7 MB, likewise needs separate kind, not worthwhile.

## 7. Question5: queries, page loading, migration

### 7.1 All queries

Milliseconds, run 1–3 ranges (`*` deliberately removed fields/results differ; `defName`/`Name` current SQL, `CROSS JOIN` §7.2):

| Milliseconds | `v26` | `v26-copy` | `ik` | `path-global` | `ik+path-mod` | `blob-dedup` | `combo-global` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| defName×100 | 85.8–92.8 | 84.1–93.4 | 93.1–93.7 | 86.5–91.5 | 127–157 | 120–135 | 88.0–92.4* |
| Name×190 | 153–185 | 155–163 | 165–174 | 156–169 | 206–229 | 197–208 | 149–160* |
| defName×100(CROSS JOIN) | 2.1–2.7 | 2.2–2.3 | 2.6 | 2.6 | 4.5 | 2.7 | 2.4–2.8* |
| Name×190(CROSS JOIN) | 4.7–6.5 | 5.0–5.4 | 5.2 | 6.3 | 9.2 | 5.8 | 5.2–6.0* |
| DefsOf×100 | 79.7–89.5 | 77.8–78.6 | 77.4–81.1 | 88.1–89.8 | 135–161 | 92.7–99.4 | 78.2–80.1* |
| DefCounts(whole table) | 65.8–70.1 | 62.2–64.1 | 52.9–56.4 | 67.8–89.3 | 57.0–72.4 | 75.8–87.1 | 62.0–64.5 |
| CachedDefs(all disabled) | 536–613 | 494–515 | 541–555 | 594–1030 | 1039–1114 | 585–662 | 518–530* |
| TypeOwners×100 | 2.1–2.9 | 1.9–2.0 | 2.4–3.0 | 2.6–2.9 | 3.2–3.8 | 2.5–2.6 | 2.3–2.7 |
| HarmonyPatches(all) | 58.7–108 | 57.5–58.1 | 59.4–84.0 | 63.4–116 | 77.5–112 | 67.2–79.8 | 61.8–66.6 |
| CurrentFiles(incremental scan) | 94.5–137 | 96.1–107 | 95.2–109 | 115–142 | 192–197 | 79.1–91.1 | 110–115 |
| SemanticSet×100 | 8.2–14.8 | 6.8–7.9 | 7.4–7.6 | 10.9–15.1 | 17.0–18.5 | 6.3–7.1 | 8.3–10.7 |
| SnapshotFiles×100(changes page) | 17.4–28.0 | 16.4–17.8 | 16.0–16.3 | 34.7–35.2 | 45.9–53.9 | 12.7–14.7 | 15.4–17.2 |
| InstanceFiles(%Textures/%) | 115–209 | 121–130 | 120–125 | 295–481 | 314–338 | 142–166 | 164–168 |
| InstanceFiles(%x.dll)×20 | 1435–1812 | 1323–1499 | 1394–1407 | 4599–4759 | 1500–1728 | 436–512 | 787–1149 |
| ContentKinds(all, uncached) | 236–294 | 235–266 | 245–256 | 398–443 | 626–777 | 4.5–5.8 | 5.7–7.5 |
| SnapshotsWithPNG(all) | 20.9–31.0 | 20.6–32.7 | 21.9–23.4 | 46.5–52.2 | 30.8–30.9 | 3.9–4.6 | 4.6–5.7 |
| Languages×100(details) | 11.6–21.1 | 10.7–14.1 | 11.0–11.1 | 25.4–27.0 | 16.3–17.8 | 0.57–0.65 | 0.73–0.95 |
| write 30 snapshot manifests | 20.2–32.1 | 20.4–31.2 | 22.0–22.7 | 74.6–85.5 | 26.4–38.5 | 14.0–14.5 | 44.1–49.8 |
| write 30 Def sets | 56.9–112 | 57.6–98.4 | 56.5–58.5 | 59.4–71.4 | 60.0–61.5 | 56.9–58.2 | 72.1–88.0 |

`setfiles` prototypes separately run 4 (read list once/set/call, restore paths in Go, no cross-call cache; run 3 SQL per-row `json_extract` made `DefsOf`499–607ms, prototype flaw). Run 4 was busier; identical structures were 20–40% slower than runs 1–3; compare same-run `v26-copy` only:

| Milliseconds (run 4) | `v26-copy` | `setfiles` | `combo-setfiles` | `combo-global` |
| --- | ---: | ---: | ---: | ---: |
| defName×100 | 86.5 | 90.2 | 125* | 153* |
| Name×190 | 153 | 162 | 183* | 250* |
| defName×100(CROSS JOIN) | 2.1 | 9.0 | 12.9* | 5.2* |
| Name×190(CROSS JOIN) | 5.6 | 19.7 | 30.9* | 10.3* |
| DefsOf×100 | 109 | 87.6 | 98.6* | 130* |
| CachedDefs(all disabled) | 724 | 574 | 925* | 927* |
| HarmonyPatches(all) | 73.4 | 65.3 | 116 | 109 |
| CurrentFiles(incremental scan) | 127 | 93.4 | 122 | 195 |
| InstanceFiles(%Textures/%) | 183 | 119 | 211 | 275 |
| InstanceFiles(%x.dll)×20 | 1693 | 2111 | 607 | 1245 |
| ContentKinds(all, uncached) | 261 | 401 | 2.7 | 7.7 |
| write 30 snapshot manifests | 27.9 | 39.5 | 22.7 | 50.0 |
| write 30 Def sets | 54.4 | 95.3 | 89.2 | 110 |

After `CROSS JOIN`, name lookup reads one extra set list/call: `setfiles`~70µs, `combo-setfiles`~110–130µs; immutable `set_id` list cache removes it. Prototype `HarmonyPatches` still `json_extract`, an upper bound.

### 7.2 Existing slow query (independent, affects first log)

`Store.DefsNamed` (`def`/log attribution/missing parent) SQL: `… FROM def_fact f JOIN content_state c ON c.def_set_id=f.set_id WHERE f.name=? AND c.game_mm=? ORDER BY c.instance_id, f.rowid`. `EXPLAIN QUERY PLAN`:

```
JOIN:       SEARCH c USING INDEX content_state_mm (game_mm=?)
            SEARCH f USING INDEX def_fact_named (name=? AND set_id=?)
CROSS JOIN: SEARCH f USING INDEX def_fact_named (name=?)
            SEARCH c USING INDEX content_state_def (def_set_id=? AND game_mm=?)
```

To avoid sorting, SQLite traverses all 1,354 version `content_state` rows then probes Name index each. `CROSS JOIN` (already used by `TypeOwners`) preserves results: 190 Name **153–185→4.7–6.5ms**, 100 defName **84–93→2.1–2.7ms**.10-29 sharing report 5.6ms predates B1 fix `6c67849` adding ORDER BY instance_id.190 missing-parent log clusters alone consume1/3 first-log 500ms budget (07 §6 first MCP `list_problems`/`analyze_log` after new log). Independent of integers, fix separately.

### 7.3 Against06 §10/07 §6 thresholds

Recommended `combo-setfiles` effects, §7.1 figures:

| Path | Threshold | `v26` | Recommended | Verdict |
| --- | --- | ---: | ---: | --- |
| First inventory content-kind+PNG | Page≤200ms | 257–325 | 2.8–3.8 | Faster; current component alone exceeds budget |
| Detail language, one snapshot | Interaction≤50ms | 0.12–0.21 | 0.01 | Unchanged |
| Changes two-snapshot files | Page≤200ms | 0.35–0.56 | 0.30–0.37 | Unchanged |
| First log texture+20 DLL+190 Name | ≤500ms (07 §6) | 1,700–2,200 | 770–1,000 | Faster but over; DLL/Name dominate (§7.2), independent. Synthetic DLL lookup covers all 1354 environment Mods, more than current source (§1 item 3) |
| Analysis all-disabled cached Defs | No separate threshold (indexed7.8s, 07 §6) | 494–724 | 702–925 | Prototype per-row string split; production direct indexes closer to baseline, worst+0.2–0.4s |
| Incremental semantic-row read | ≤3s (07 §6) | 95–137 | 75–122 | Unchanged |
| Write 30 manifests | — | 20–32 | 16.9–22.7 | Same/slightly faster |
| Save 30 Def sets | — | 54–112 | 71.8–89.2 | Within noise |

Memory: prototype adds no resident cache. Derived columns allow removing `ContentKinds` process cache; set-list cache needs cap/IndexKey invalidation (06 §10 avoid long-term memory).

### 7.4 Migration duration

245 MB synthetic DB, one startup-upgrade transaction, no VACUUM like v26:

| Conversion | Duration | File before maintenance | In-use | Later VACUUM |
| --- | ---: | ---: | ---: | ---: |
| `ik` | 0.07–0.09s | 248.9 MB | 244.5 MB | 1.9–2.0s |
| `setfiles` | 3.4–4.5s | 294.1 MB | 206.6 MB | 1.6–1.7s |
| `blob-dedup` | 4.2–4.3s | 256.4 MB | 201.6 MB | 2.0–2.3s |
| `combo-global` | 7.9–8.6s (run 4:14.3) | 256.6 MB | 152.7 MB | 1.2–2.5s |
| **`combo-setfiles` (recommended)** | **8.4–11.3s** | 256.4 MB | 162.7 MB | 1.6–1.9s |
| `combo-ik-mod` | 8.8–9.2s | 255.4 MB | 146.6 MB | 1.2–1.3s |

Time scales with affected rows, not total size; ToDo real same rows implies~10s. v26 synthetic 760,000 rows 4.33s (07 §6), VM54 MB real first-open+upgrade1.7s. Longer step needs startup progress ("upgrading database…"), not frozen-looking window. File grows first (~256 MB before maintenance, WAL extra), later reclaimed; space rule02 §7.2 twice DB+64 MiB. `D:\RimArcho-test\dbint` peak~500 MB (baseline+converting copy+WAL).

Migration constraint: `Store.Open` `foreign_keys(1)` cannot turn off in transaction. Parent `fact_set` **must not be dropped/rebuilt**, cascading all facts; `ALTER TABLE ADD COLUMN` only. Children `def_fact`/etc can create/copy/drop/rename. Prototype disabled FKs; production must respect constraint. Recommendation only adds fact_set column.

## 8. Recommended design

| Structure | Size | Relative v26 |
| --- | ---: | ---: |
| `v26` | 245.47 MB | — |
| Derived columns only (`kinds`,`langs`) | About 245 MB (two extra snapshot columns, not separately measured) | All page-loading gain |
| `blob-dedup` (blocks+derived) | 194.11 MB | −20.9% |
| **`combo-setfiles`** | **152.58 MB** | **−37.8%** |
| `combo-global` (global paths too) | 148.12 MB | −39.7% |

1. Snapshot `kinds` (DLL, Defs/Patches XML,`Languages`, texture PNG four bits), `langs` (existing Languages directory rules), computed at write/backfilled migration. `ContentKinds`/`SnapshotsWithPNG*`/Languages read them.
2. Nonsemantic non-DLL `snapshot_file` rows to sorted `rel_path`/prefix/size/zstd per-snapshot block, no mtime. New `snapshot_list(snapshot_id INTEGER PRIMARY KEY, n, data)`, optional hash-dedup `file_blob`. Update `InstanceFiles` texture branch, `SnapshotFiles`, persist copy/rewrite, 02 §7.1 cleanup, gc, remove env.
3. Add `fact_set`.`files` JSON/newline list; replace `file TEXT` in `def_fact`/`type_fact`/`harmony_fact`/`harmony_id`/`assembly_fact` with indexes. Build list/set on save, read once/set.
4. No Instance integers/name-only/global paths.
5. Independent first fixes: `Store.DefsNamed` `CROSS JOIN` (§7.2), InstanceFiles `environment_id IN ('', ?)` (§1 item 3, tests needed).

## 9. Proposed implementation plan for 01 §5

(2026-10-31: added01 §5.1 step 24 per §10. Step 1, derived columns, was completed first as schema v27; blocks+set lists together deferred to v28 after §12 VM measurement, no dedup.)

Add step to §5.1 implementation table (number at that time):

| # | Step | Dependencies | Spec |
| --- | --- | --- | --- |
| 24 | **archo.db size (schema v27)**: snapshot `kinds`/`langs` computed scan/backfill; content/PNG/language read them; nonsemantic non-DLL rows to per-snapshot zstd `snapshot_list` (prefix paths+sizes, no mtime), keep semantic/DLL rows; `fact_set`.`files`, indexed fact `file`; one transaction/progress/no VACUUM/no fact_set rebuild. No Instance integers/name-only. Synthetic245→153 MB, first inventory content260→3ms (research2026-10-30_archo-db-integer-keys) | — | 02 §4.3,§7,§7.1,§7.2; 03 §2,§3; 07 §6 |

Implement in order, independently mergeable:

1. **Derived columns** (independent, largest gain): two `snapshot` columns, `insertSnapshot` computes, migration backfills all; `ContentKinds`/`SnapshotsWithPNG`/`SnapshotsWithPNGFor`/`Languages` read; remove process cache. Keep content/texture/language tests, add equivalence for `About.xml.disabled`-only/versioned Languages/any-location DLL.
2. **Blocks**: `snapshot_list` (`file_blob` if dedup), `insertFiles` splits rows/block, `insertSnapshot` copies previous block, mtime refresh semantic only, `InstanceFiles` texture decodes current blocks, `SnapshotFiles` merges, 02 §7.1/`gc`/`DeleteEnv` delete blocks,`Maintain --dry-run` counts blocks. Tests byte-identical `modchanges`, SnapshotFiles migration equivalence, protected manifests intact after maintenance.
3. **Set lists**: `saveContent`/`saveHarmony` list/set; `DefsOf`/`DefsNamed`/`CachedDefs`/`HarmonyPatches`/`Assemblies`/`TypeOwners` restore indexed paths, bounded immutable `set_id` cache. Rebuild child tables like v26 preserving rowid order. Reuse `TestMigrateV26*` equivalence plus `TestDefsNamedOrderIndependentOfWrites`.
4. **Specs/baselines**: 02 §4.3/§7 schema and `snapshot_file.mtime` incremental-only description updated; 02 §7.1 blocks follow manifests; 03 §3 set lists; 07 §6.2 synthetic benchmarks;`docs/release-vm-tests.md` row for real-VM upgrade time/size/first inventory.
5. **Benchmark**: retain `TestIntKeyResearch` reference, replace `combo-setfiles` with production or separate opt-in production-only benchmark.

## 10. User decisions (2026-10-31)

Questions/answers at the time:

1. Recommended or global path table? **Recommended**: set lists+snapshot blocks, no global paths/Instance integers/name-only.
2. Dedup blocks? **No.**
3. Derived columns separately first? **Yes**, done schema v27 (`8b38732`, synthetic 07 §6.2).
4. Fix `DefsNamed` plan/`InstanceFiles` scope now? **These two first**, before derived columns; separate branches already fixed/merged dev/1.0.
5. Measure real VM before deciding schema? **Yes**, before table changes (§12).

**Correction to §8**: `combo-setfiles`152.58 MB uses dedup `blob-dedup` plus `slim-parent` (clears disabled parent/abstract). Decisions reject both, adding~1.0 MB dedup (§5.2)+2.7 MB (§6.2), final synthetic~156 MB (−36%). §12 measures exactly `blob`+`setfiles`, no dedup/field removal.

## 11. Reproduction

```powershell
$base = 'D:\RimArcho-test\dbint'
$env:GOCACHE = (go env GOCACHE)          # Save first; changing LOCALAPPDATA below changes Go cache location
$env:TEMP = "$base\tmp"; $env:TMP = $env:TEMP
$env:APPDATA = "$base\appdata"; $env:LOCALAPPDATA = "$base\localappdata"; $env:USERPROFILE = "$base\profile"
Remove-Item Env:RIMARCHO_HOME -ErrorAction SilentlyContinue
$env:RIMARCHO_INTKEY_BENCH = '1'
$env:RIMARCHO_INTKEY_DISK_ROOT = $base    # Fail above RIMARCHO_INTKEY_DISK_MB (default 1400)
go test ./internal/store -run '^TestIntKeyResearch$' -v -count=1 -timeout 60m
# Subset: $env:RIMARCHO_INTKEY_LAYOUTS = 'v26-copy, combo-setfiles'; small trial: $env:RIMARCHO_INTKEY_SCALE = '0.05'
```

Full run 6–9 minutes. `t.TempDir()` removes all DBs afterward. Four raw runs in [`2026-10-30_archo-db-integer-keys-evidence/`](2026-10-30_archo-db-integer-keys-evidence/) (`run1.log`–`run4.log`, `run4` `setfiles` only).

**Limits**: synthetic paths/Defs per file/history-change proportions calibrated to totals, not real distribution; real prefix similarity/dedup affects block sizes. Backend only, not GUI/native/VM. Load noise §2.2.

## 12. Real VM database measurements (2026-10-30)

Test VM Win11-Player-Test, 4 vCPU/8 GB, snapshot `RimArcho-Real-Vulkan`, restored afterward. Real DB touched only VM: `sqlite3 -readonly .backup` copy, all upgrades/conversions on copy; `APPDATA`/`LOCALAPPDATA`/`TEMP` point to guest work directory; real `archo.db`/ModsConfig.xml MD5 unchanged afterward. Host retains figures only (`2026-10-30_archo-db-integer-keys-evidence/vm-*.txt`, no Mod names). Same-directory `vm-measure.ps1`, conversion `TestIntKeyConvertFile` with `RIMARCHO_INTKEY_DB` copy.

**Database**: snapshot schema18, 53.6 MB; 1271 current/1285 total snapshots, 239,350 manifest rows (78% nonsemantic destined for blocks), 80,355 Defs. No DLL facts (`type_fact`/`assembly_fact`0), much smaller than host443 MB.

| Item | Figures |
| --- | --- |
| v26 build dev/1.0 `295cb45` first open18→26 | 825ms (whole `archo db status` process), 53.6→62.2 MB including WAL, no v26 VACUUM |
| v27 `8b38732` first open26→27 | **317ms**, same measurement, process startup~33ms; unchanged size |
| `archo library` (inventory view model, new process each, 5 runs) | v26 **315–373ms**→v27 **89–98ms**, line-identical output |
| Content kinds (1271 current, best of 5) | Old query **206.8→2.0ms**, convertible textures18.5→1.1ms |
| v27 compacted | 37.2 MB: `snapshot_file`20.6,`def_fact`12.1, snapshots2.3, sets/views1.0 |
| Prototype (nondedup blocks+set lists,§10) | **1.09s** one transaction, later VACUUM0.18s |
| Converted size | **19.6 MB (−47%)**: `snapshot_file`20.6→4.2, blocks1.65 (1285 blocks, 12.1 MB paths→3.7 prefix→1.4 zstd),`def_fact`12.1→8.6, sets/views1.0→1.6 |
| Check | 187,775 rows compressed, 187,775 decoded |

Versus synthetic: 1285 snapshots vs4864 with more history, larger proportional savings−47% vs~−36%; no DLL facts, less fact savings. Time scales by rows: 240,000→1.1s, 970,000→8–11s, host estimated~10s; v28 still needs progress.

Untested: host443 MB with hundreds enabled/DLL facts (rules prohibit copying to host); full GUI inventory frame (here same view-model CLI process).

## 13. Production implementation (2026-10-31, schema v28)

§10 design implemented via §9 steps2/3 (branch `claude/db-v28-6b0a03`), specs02 §7,§7.1–§7.2, 03 §3, synthetic 07 §6.2 (`TestV28Formal`: same synthetic v27 245.5→v28 156.2 MB, matches corrected~156 MB, production migration4.7s). Block selection (no hash/non-`.dll`) and list creation match prototype; reads differ: once/set, restore/cache in Go, not per-row SQL `json_extract`. Prototype Def checksums differ from v26 (`DIFF`) in harness, also on v27 baseline: prototype issue. Production equivalence `TestMigrateV28`/`TestFileListsMatchRows`. Harness `ikPlanLibrary` never finishes at `RIMARCHO_INTKEY_SCALE=0.02` (too few candidates); use0.05.
