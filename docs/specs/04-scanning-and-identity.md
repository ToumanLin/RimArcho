# 04 · Scanning and identity

> Status: TODO(reorg)

## 1. Scanning

### 1.1 Flow

```
Check source states → list source subdirectories → group (merge identical real paths) → read observations → reconcile → write snapshots → emit change events → mark stale facts
```

- Command: `archo scan [--full] [--game-only]`. `--full` ignores cached hashes and rereads all semantic files. TUI and GUI run `archo scan` in subprocesses; the main process does not scan, avoiding retention of peak scan memory in the interface process.
- **Scope**: shared Mod folders and current environment's own `Data`, `Mods`, once per directory. `--game-only` scans only the current environment's two sources: after adding an environment (guide step 2), only the new game needs reading. Active Instances in unscanned shared sources neither match nor tombstone but remain R4 "original" candidates, so `Mods/` copies still join Workshop originals' Lineages. If shared folders have never been read, guide step 2 omits `--game-only`.
- Before starting, check data directory per 03 §3.1 and refresh each source state per 03 §4.1; skip missing, `rebind-pending`, retired.
- When reading origin markers (`.rimarcho/origin.json`), first map merged-away IDs through `id_alias` (03 §2.1).
- A subdirectory is a Mod if it has `About/About.xml` or only `About/About.xml.disabled` (§1.2). Filenames are case-insensitive. Other subdirectories are counted without Instances.
- **Concurrency**: one goroutine per source, up to `NumCPU` workers within each. Assign IDs in deterministic reconciliation order, independent of concurrency.
- **Incremental**: reuse previous hash if relative path, size, and mtime are unchanged. mtime only skips hashing; it does not determine identity.
- **Memory**: incremental scans load only hashed semantic file rows, not the full list. Release lists after reading if directories are unchanged and no rehash occurred; if only external state changed (such as ACF manifest), copy old snapshot file rows to the new snapshot with SQL. Measured memory/timing: 18.
- Scans update only identity and metadata, plus the type index of the scanned environment's own game assemblies (05 §5.3). Analysis rebuilds Def, Patch, Harmony facts on demand (05 §3).
- Per-source scan output lists subdirectory count, Mod count, residue count, about-disabled count, non-Mod directory count, count-only residue directories, alias count; Workshop sources without ACF show `no ACF`.

### 1.2 Residue and about-disabled

**Subscriptions and residue** (workshop sources only; game does not directly load SteamCMD directories):
- Game loads only **subscribed** Workshop items (17 §6). Subscribed means present in `appworkshop_294100.acf` `WorkshopItemsInstalled` or `WorkshopItemDetails`.
- Without ACF, treat all as subscribed.
- Folders absent from subscription records are residue: still create Instances, but cannot add to plans; can it run suggests cleanup (05 §4.3, 02 §3).
- Residue folders without `About.xml` or `.disabled` are counted only, without Instances.

**about-disabled**:
- If `About/About.xml` is absent but `About/About.xml.disabled` exists (RimCrow's disable method), read it as declaration, create Instance, mark `unloadable: about-disabled`. Such Instances cannot join plans; filter with `list --filter unloadable`.
- Workshop residue folders containing only `About.xml.disabled` are both residue and about-disabled.
- Scan does not restore filenames. Analysis classifies them below, computed per analysis without persistence:

| Category | Condition | Finding code | Description |
| --- | --- | --- | --- |
| Locked version | Same Lineage has another live, loadable, non-Workshop Instance (RimCrow `_<Workshop-ID>_` copies, `copy-local` copies, and copies deployed in `Mods/` count) | `mod.pinned` (info; warning if upstream updated) | Do not report `mod.unloadable`; the copy is effective, Workshop copy is upstream. Product rules: 07 §5 |
| Unsubscribed residue | Workshop source, absent from ACF subscription list | `mod.hidden-residue` (warning) | May recycle (02 §3) |
| Independently hidden | Everything else | `mod.hidden` (warning) | Keep hidden, restore, or recycle (09 §5) |

### 1.3 Observations

Record for each Mod directory:

| Category | Contents |
| --- | --- |
| Location | `source_id`, relative path, real path, link flag, alias locations |
| Declarations | `About.xml`: `packageId`, name, author/authors, `modVersion`, `supportedVersions`, description, `modDependencies`` and ``modDependenciesByVersion`, `loadBefore`, `loadAfter`, `forceLoadBefore`, `forceLoadAfter`, `incompatibleWith`, `loadBeforeByVersion`, `loadAfterByVersion`, `incompatibleWithByVersion`, `descriptionsByVersion`; `About/PublishedFileId.txt` (strip BOM/whitespace); original `LoadFolders.xml` |
| Workshop ID | Workshop-type sources (workshop, steamcmd): directory name; others: `PublishedFileId.txt` |
| External state | Workshop-type sources: ACF manifest ID, timeupdated, size, subscription status; with `.git`: `HEAD` commit (read `HEAD`, then ref file or `packed-refs`) |
| Content fingerprint | See §1.4 |
| Origin marker | `.rimarcho/origin.json` (if present and `origin_instance` nonempty) |
| State | residue, unloadable |

**Tolerant `About.xml` parsing**:
- Encoding: detect UTF-16 LE/BE and UTF-8 BOM, convert to UTF-8; without BOM but with XML `encoding="utf-16"`, read UTF-16, unless actual content is UTF-8, then follow actual encoding. Applies to `LoadFolders.xml` and Defs/Patches too.
- Tolerate tag case, mixed author/authors, whitespace in li, package ID case.
- Split author like game `ModMetaData.Authors`: on ` and `, `&`, `,`, trimming whitespace.
- Dependency li may contain `packageId` child or plain text.
- Malformed `About.xml` still creates Instance; record error in declaration without interrupting scan.

Always store `packageId` twice: original `package_id_raw`, lowercase `package_id`. Derive missing `packageId` per §1.5.

### 1.4 Fingerprints and Similarity

| Layer | Scope | Calculation |
| --- | --- | --- |
| Structural list | All files | Relative path + size (+ mtime, incremental only) |
| Semantic hash | All files under `About/`, `LoadFolders.xml`, XML under any `Defs/` or `Patches/` (including version directories like `1.6/Defs/`), `.dll` under any `Assemblies/` | xxh3-128 per file |
| Snapshot hash | Both layers above | Sort by lowercase path; hash each "lowercase path + size + semantic hash (if any)" to quickly detect changes. Lowercase paths mean case-only renames on Windows do not count as updates |

- Large files such as textures/audio compare size only, without content reads.
- Exclude `.git`, `.svn`, `.rimarcho`, so locked versions and originals have identical fingerprints.
- Do not follow links or device files within `Mods`; skip unreadable entries.

**Similarity**: over two snapshots' semantic file sets, take the larger of two Jaccard coefficients:
- `J(paths)`: compare lowercase relative paths only. Recognizes "move + update": contents changed, structure unchanged.
- `J(hashes)`: compare content hashes without paths, as a **multiset** (count each identical-content file separately). Recognizes "reorganization": files move into subdirectories like `1.6/` without content changes.

With no semantic files on either side, similarity is 0. R3/R4 additionally require matching declarations; R6 does not equate same paths with same content, using content evidence in §2.1. Adding files to small `Mods` can lower similarity below 0.85 (for example adding one to 4 semantic files gives 0.8); R6 then asks for confirmation, an acceptable conservative outcome.

### 1.5 Derived packageId

Game permits very old `Mods` without `<packageId>`. RimArcho derives it like `ModMetaDataInternal.TryParsePackageId` (confirmed by decompiling 1.7.4982, 17 §6), marking "inferred" (`package_id_derived`):

```
ConvertToASCII(author + hash3(description)) + "." + ConvertToASCII(name)
```

- Default author: `Anonymous`.
- `hash3`: empty description gives none; otherwise compute `GenText.StableStringHash` (initial 23, UTF-16 code units `n = n*31 + c`, int32 wraparound), decimal, remove minus sign, take first 3 digits.
- `ConvertToASCII`: per UTF-16 code unit, retain letters/digits, replace other c with `c % 25 + 65`.
- Lowercase result into `package_id`.
- Official content (official source) missing `packageId`: `Core` uses `ludeon.rimworld`, others `ludeon.rimworld.<lowercase-directory-name>`.

## 2. Reconciliation

Input: all current scan observations; database Instances (including tombstones); unscanned source list. Output: Instance ID per observation, change events, identity questions, Instances disappearing this scan. Reconciliation is pure logic (`internal/identity`); caller handles database I/O.

### 2.1 Rules R1–R7

Matched observations and Instances do not participate in later rules.

| Rule | Condition | Result | Event |
| --- | --- | --- | --- |
| R1 | Same `source_id` + relative path (case-insensitive) | Same Instance | See §2.3 |
| R2 | Observation from Workshop-type source (workshop, steamcmd); another Instance from a Workshop-type source, same directory name (same Workshop ID), not seen this scan | Same Instance (for example Workshop content moved Steam libraries) | moved |
| R5 | Observation has `.rimarcho/origin.json` | New Instance, join recorded Lineage, mark "confirmed"; exceptions below | copied |
| R3 | A live Instance not seen this scan has similarity ≥ 0.85 to observation and matching `packageId` or Workshop ID declaration | Same Instance | moved |
| R4 | An Instance already seen this scan has similarity ≥ 0.6 to observation and matching `packageId` or Workshop ID declaration | New Instance, join its Lineage, mark "inferred" | copied |
| R6 | A tombstone or currently disappearing Instance passes `identity.QuestionScore` content check, with unique candidates in both directions (below) | New Instance, new Lineage, and identity question "is this #xxxx returning?" | discovered |
| R7 | Everything else | New Instance, new Lineage | discovered |

- **Matching declarations**: same nonempty `packageId` (case-insensitive), or valid Workshop ID; blank, `0`, and nonnumeric Workshop IDs do not match. R2 directory names must also be valid Workshop IDs.
- **R5 before R3/R4**: origin markers are authoritative even after major edits make similarity unrecognizable.
- **R5 rename exception**: if a live Instance not seen this scan has the same origin marker in its latest snapshot (same `origin_instance`) and similarity ≥ 0.85, treat as "copy renamed": same Instance, emit moved, no new copy. Otherwise renaming a locked version's directory would create another Instance.
- **R5 Lineage**: prefer marker `origin_lineage` if it exists in this environment; otherwise current Lineage of `origin_instance`; if neither exists, create Lineage (still emit copied).
- For multiple candidates in R2–R5, choose highest similarity.
- **R6 content evidence**: only content hashes of Defs / Patches XML and Assemblies DLL; exclude About files, previews, `LoadFolders.xml`, empty content; count duplicates once; path-independent. Content-set Jaccard ≥ 0.6; matching declarations require at least one shared content, otherwise at least three distinct shared contents. Names, authors, folder names, and common texture paths alone cannot trigger a question. Implementation: `identity.QuestionScore`, `identity.CompareFiles`.
- **R6 ambiguity guard**: if a new observation has multiple eligible candidates or multiple observations choose one candidate, record independently through R7 without questions; do not pick highest score or ask per pair.
- Store "inferred"/"confirmed" in Instance `lineage_basis` (§3).

### 2.2 Execution Order and Candidate Scope

**Two phases**:
1. Run R1, R2, R5, R3 rule by rule over all observations, matching only known database Instances.
2. Run remaining observations individually through R4 → R6 → R7.

This allows Instances just created by R7 in this scan to be R4 originals. Otherwise the first scan would assign unrelated Lineages to Workshop originals and `Mods/` copies, or the same Workshop ID in Steam and SteamCMD.

**Observation order**: workshop → steamcmd → official → local → extra; same-kind sources follow environment order, then lowercase relative path. Establish originals before copies.

**Candidate scope**:

| Rule | Candidates |
| --- | --- |
| R2 | Workshop-type Instances not seen this scan, **including tombstones**; prefer live. Matching tombstones emit reappeared, plus moved if location changed |
| R5 rename exception, R3 | Instances live before scan and not seen this time; no tombstones (reserved for R6) |
| R4 | Instances already seen this scan, including newly created |
| R6 | Tombstones plus currently disappearing Instances |

Instances in unscanned sources (missing, `rebind-pending`) are excluded from all candidates and do not become tombstones.

**Direction**: R4 "original" is whichever copy was processed first. If a RimCrow-disabled Workshop original becomes an Instance after a deployed copy (for example copy scanned before about-disabled support), events record the Workshop copy as an "inferred copy" of the deployed one. Lineage is unaffected; primary Instance still follows §3 priority.

### 2.3 Events

**Identity events**:

| Event | When | detail |
| --- | --- | --- |
| `discovered` | R6, R7 creation | `src:<label> <relative-path>` |
| `copied` | R4, R5 creation | R4: `inferred copy of #<original>`; R5: `origin #<source-Instance> (.rimarcho/origin.json)` |
| `moved` | R2, R3, R5 rename exception | `src:<old-label> <old-path> → src:<new-label> <new-path>` |
| `reappeared` | R1/R2 matches a tombstone, or player answers same to identity question | `src:<label> <relative-path>` |
| `disappeared` | Live Instance still unseen after scan (its source was scanned) | `src:<label> <relative-path>`; then tombstone |
| `updated` | Existing Instance matched and snapshot hash changed (including R2/R3 move + update) | — |

**Field events**: only when snapshot hash changes, compare old/new declarations and emit separately:

| Event | Comparison | detail |
| --- | --- | --- |
| `package-id-changed` | Lowercase `packageId` | `pkg:<old> → pkg:<new>` |
| `renamed` | name | `"<old>" → "<new>"` |
| `author-changed` | Author list | `<old, …> → <new, …>` |
| `version-support-changed` | `supportedVersions` | `<old> → <new>` (space-separated) |
| `dependencies-changed` | All dependency packageIds (including versioned, lowercased, deduplicated, sorted) | `+pkg:<x> -pkg:<y>` |
| `load-rules-changed` | `loadBefore`/`loadAfter`/`forceLoad*`/`incompatibleWith` and versioned forms (lowercase, sort, compare) | — |

- One scan may emit several events for one Instance. All share a scan ID (`scan:<n>`).
- Each event references before/after snapshots (`before_snapshot`, `after_snapshot`), allowing file-level diffs. Change history (16 §2.9) combines a Mod's events over a period into one row: compare declarations/file lists of earliest `before_snapshot` and latest `after_snapshot`; detailed `load-rules-changed` differences come from those declarations too.
- `_Archo_` link appearance/disappearance changes alias locations only, without events (03 §4.3).
- View events in GUI "mod library › changes" (16 §2.9), `archo changes [<reference>] [--since …] [--all]` (latest scan by default, 13 §4), and MCP.

**Snapshot write conditions**: write a new snapshot only when snapshot hash, ACF manifest, git `HEAD`, or origin marker changes. If only mtime changes, update file mtimes in current snapshot directly. ACF-manifest-only changes create a snapshot without updated.

### 2.4 Identity Questions

- R6 records the observation's new Instance, candidate Instance, content overlap, and a small comparison summary (shared content count, each side's content count, declaration differences). Store summary with question; page construction reads question records only, not file lists.
- Every successful scan rechecks visible unanswered questions in the same `Store.ApplyScan` transaction: apply R6 content checks to current snapshots; delete unanswered questions with insufficient evidence, missing references, or multiple eligible questions involving the same Mod. Use stored snapshots only, so check unscanned sources too (partial scans, other games' `Mods/`) (since 2026-10-28; previously retained, leaving 67 incorrect questions about one VRE patch on the host). Exclude answered questions, including player same / copy / unrelated decisions. Do not modify `Mods`, lists, history, or grouping.
- View: `archo identity [list] [--all]` (no subcommand also lists; `--all` includes processed), `archo identity why q:<n>`; GUI "can it run › needs your answer" and editor problems panel (16 §2.4, §5). CLI/MCP list, inspect, and answer include the same brief comparison.
- **Load comparison evidence on demand**: `Store.QuestionEvidence` reads declarations/file records from stored observation snapshot and candidate snapshot at question scan. Folders and directory-derived Workshop IDs use current records; explicitly say comparison unavailable if records were cleaned. `cmds.CompareIdentity` / `IdentityReasonLines` share comparison: omit fields absent on both sides, give one content overlap conclusion, up to five shared files plus remaining count; no matching-method explanation. UI structure: [16 §5](16-interface/4-details-and-feedback.md#5-can-it-run-wording). No Mod file scan or network reads.
- Answer: `archo identity resolve q:<n> same|copy|unrelated`; UI "same", "a copy", "unrelated".
  - same: merge new Instance into candidate. Candidate keeps ID, Lineage, history, takes new location, snapshots, events, aliases; rewrite current lists, undo/redo, historical versions, staging entries/anchors across all plans in one transaction; retain first duplicate position and unchanged missing entries; rewrite launch list fingerprints too. `id_alias` maps merged ID and emptied Lineage; pack manifests/origin markers resolve to survivor without modifying files; delete new Instance's discovered, add reappeared under question's scan; mark other questions involving both Instances superseded. Refuse same if R1/R2 already matched candidate back meanwhile.
  - copy: join candidate Lineage, mark "confirmed".
  - unrelated: close question only.

### 2.5 Tombstones and gc

- Disappeared Instances become tombstones with `tombstone_at`, retaining Lineage, rules, aliases, historical snapshots.
- Default retention 90 days, configurable with nonnegative `tombstone_days` in `config.toml`.
- `archo gc [--dry-run]` (settings "clean tombstones") removes:
  - Expired tombstones and events, except those referenced by unanswered questions;
  - Old snapshots neither current nor referenced by events/questions;
  - Lineages made empty.
- `--dry-run` counts only.
- Players run tombstone gc manually. Periodic database maintenance does not change tombstone/identity behavior; it only cleans old file lists per §4.1.

## 3. Lineage (Including Locked Versions)

- Every new Instance creates its own Lineage by default (empty `lineage_basis` means "founding member").
- Ways to join existing Lineages and markers:

| Method | `lineage_basis` |
| --- | --- |
| R4 inferred copy | inferred |
| R5 origin marker, copy answer | confirmed |
| Manual split/merge | manual |

- Manual split/merge: `archo lineage split <reference>` (move Instance into its own new Lineage), `archo lineage merge <reference> <reference>` (merge second Lineage into first).
- Splitting copies original alias, Mod notes, tags into new group, then edits are independent; copy notes stay with Instance. User rules/finding ignores stay with original group. Change identity and copy player data in one transaction; Instance ID, path, snapshots, plan entries stay unchanged. Rescans preserve manual grouping; origin markers/upstream update relationships remain.
- **Primary Instance**: default Lineage display name; priority: live → Workshop-type source → earliest `first_seen` → ID.
- **Lineage attachments**: user rules and player data (aliases, tags, notes) belong to Lineage, shared by originals/locked versions and survive renames; plan entries belong to Instances (07 §1, 07 §6; rationale 00a §4.3).
- **Known hard cases**: author forks a new Mod with different Workshop ID but same `packageId` → separate Lineages, analysis reports duplicate package ID; same Workshop ID in Steam Workshop and SteamCMD → two Instances, one Lineage.

**Locked version identity** (product rules 07 §5):
- Locked version is a new R5-confirmed Instance in the same Lineage. Creation (`copy-local` is low-level command) copies Mod to local source (default `<original-directory-name>_local`) and writes `.rimarcho/origin.json`:

```json
{
  "origin_instance": "<full source Instance ID>",
  "origin_lineage": "<full Lineage ID>",
  "workshop_id": "<Workshop ID, may be empty>",
  "origin_snapshot": "<snapshot hash at copy time, hexadecimal>",
  "copied_at": "2026-10-21T08:00:00Z"
}
```

- Next scan R5 rejoins original Lineage, marks "confirmed", emits copied. Later directory rename follows §2.1 exception as moved, retaining locked version identity.
- RimArcho never writes Workshop marker files, because Steam verification/updates delete them.
- Manual player copies have no marker; R4 marks "inferred".
- RimCrow style (Workshop `About.xml.disabled`, copy in `Mods/`): R4 joins both to one Lineage; analysis recognizes locked version per §1.2.
- unpin swaps plan copy back to Workshop version without changing identity data; copy stays in place (07 §5).

## 4. Storage

**Schema and upgrades**: new databases are created from `internal/store/schema.sql` (embedded), then `meta.schema_version = store.SchemaVersion`. The first public release (1.0.0) shipped schema 29 (`store.FirstReleaseSchema`). A schema change edits `schema.sql`, bumps `SchemaVersion` and appends one step to `store.upgrades` (30: game type index, 05 §5.3; 31: save full-read cache, 12 §1.7); opening an older database runs the missing steps in order, each in one transaction, which may report row progress. A database with schema 1–28 (a pre-1.0 development build) is refused with `store.ErrPreRelease` and left unmodified: "database from a pre-1.0 development build (schema N); move <path> away to start with a new database". A schema newer than `SchemaVersion` is refused as "unsupported schema version". Test: `TestPreReleaseDatabaseRefused`.

**Startup upgrade progress** (steps 30 and 31 only create empty tables, so they finish well inside the 300 ms below): display progress when opening runs upgrade steps (`store.OpenUpgrading`, `app.OpenWith`, `cli.upgradeReport`); hide if complete within 300 ms. `RimArcho.exe` has no console; show a small native window (`internal/surface/upgradewin`: RimArcho title, one text line/progress bar, cannot close, system DPI/interface language), closed when the upgrade completes; TUI writes the same line to stderr before taking the terminal (in-place percentage refresh, interface language); CLI/MCP write one English stderr line "Upgrading the RimArcho database (schema N → M)…", then elapsed time. stdout unchanged; MCP protocol stream gets no extra content. Steps that report rows show a percentage; others show only "upgrading". If opening fails, `RimArcho.exe` (no console) also shows a modal error box in the interface language (`cli.upgradeReport.openFailed`, `upgradewin.ShowError`): a pre-1.0 database gets its own text naming `archo.db` and saying to move or rename it; other errors show the error text. OK exits; the CLI, TUI and MCP only write stderr.

**Managed links** (`managed_link`): link creation records, with phase/path indexes; existing Instances/aliases are never converted into proven creation records. Commit intent before link file operations, then detached, `records-updated`, complete; safe rollback records `rolled-back`. Startup checks unfinished rows read-only; problems page/scan report `mods.link-recovery`; recover only through existing confirmed `links repair`, never silent rebuilding.

**Identity question summary**: `identity_question.summary` stores §2.4's small comparison summary; `identity_question_open(status, environment_id)` indexes unanswered questions.

**Fact cache structure and cleanup**: set structure and reference cleanup: 05 §3; `Store.PruneFacts(now)` is an independent maintenance entry point. Environment removal/tombstone cleanup also prune in the same transaction; visible environments and analysis slots retain shared Instances.

SQLite (modernc, WAL + `synchronous=NORMAL` + `journal_size_limit` 16 MB). After a large write, next reset truncates WAL to 16 MB instead of retaining peak size while connections remain open; after analysis writes or scans creating snapshots, also attempt nonwaiting `wal_checkpoint(TRUNCATE)` (`Store.TruncateWAL`). Measured: about 380,000 analysis rows leave about 100 MB WAL while GUI is open. Identity tables (empty `environment_id` = shared across all environments):

```
environment(id, name UNIQUE, game_root, user_data_root, game_version, launch, created_at, current_plan)
source(id, environment_id, kind, label, root, status[ok|missing|rebind-pending|retired], pending_root,
       UNIQUE(environment_id, label))                       -- shared folders: environment_id=''
lineage(id, environment_id, primary_instance_id, created_at) -- all environment_id=''
instance(id, environment_id, lineage_id, lineage_basis[''|inferred|confirmed|manual], source_id, rel_path,
         real_path, status[live|tombstone], residue, unloadable, current_snapshot_id,
         first_seen, last_seen, tombstone_at)
instance_alias(instance_id, source_id, rel_path, real_path)
snapshot(id, instance_id, scan_id, snapshot_hash, package_id, package_id_raw, name, authors,
         workshop_id_declared, supported_versions, decl_blob, acf_manifest, acf_time_updated, acf_size,
         git_head, origin, file_count, total_size, semantic_count, kinds, langs)
snapshot_file(snapshot_id, rel_path, size, mtime, hash NULL)   -- semantic files (only these have hashes) and DLL
snapshot_list(snapshot_id PRIMARY KEY, n, data)               -- remaining files: one compressed block per snapshot
scan(id, environment_id, started_at, finished_at, full, stats)
change_event(id, scan_id, instance_id, kind, before_snapshot, after_snapshot, detail)
identity_question(id, environment_id, scan_id, instance_id, observation_snapshot_id, candidate_instance_id,
                  score, status[open|resolved], resolution[same|copy|unrelated|superseded], resolved_at, summary)
meta(key, value)   -- schema_version, current environment, etc.
id_alias(old_id PRIMARY KEY, new_id, kind[instance|lineage])  -- merged-away IDs (03 §2.1, 04 §2.4)
```

- `decl_blob` stores normalized `About.xml` (JSON) for analysis.
- **Snapshot file lists have two locations** (`store/filelist.go`): hashed semantic files and `.dll` (any location, ASCII case folding only) are `snapshot_file` rows; remaining files (textures, sounds, Languages, `.meta`, etc.) form one `snapshot_list` block: sort by `rel_path`; each entry stores shared prefix length with previous entry, suffix, size (no mtime: incremental scan alone needs it and reads only hashed rows, `Store.CurrentFiles`); zstd compress entire block; n is count. Blocks **are not deduplicated**: copying lists for identical content/new external state copies whole block; refreshing same-snapshot mtime rewrites whole block. Snapshots without non-semantic files have no block. Readers: `Store.SnapshotFiles` (change-page file diffs) merges both and sorts by path; `Store.InstanceFiles` (log texture / DLL provider lookup) queries rows first, skips blocks for `.dll`-ending patterns, otherwise decompresses current blocks and matches SQLite `LIKE` rules in Go (`%`, `_`, ASCII case folding only). DLL remain rows because DLL-name Mod lookup (`%x.dll`) is the commonest query and must not decompress every block.
- `snapshot.kinds`, `snapshot.langs` are derived from file lists at snapshot write (`store.deriveContent`). kinds bits: `.dll` anywhere; XML under `Defs/` or `Patches/` at any loading-folder depth; files under `Languages/`; PNG under directories whose names end with textures (`%textures/%.png`, convertible textures). langs: language names from `Languages/<name>/…` at root or next-level loading folder, in path order, first case-insensitive duplicate only, joined by `/`. ASCII folding matches previous SQLite `LIKE`. Snapshot paths never change (different path/size means different snapshot hash), so columns stay valid even after §4.1 removes files; identical-content/new-external-state list copies copy both columns. Mod inventory content types (`Store.ContentKinds`), convertible textures (`Store.SnapshotsWithPNG*`), detail languages (`Store.Languages`) read columns by primary key instead of scanning `snapshot_file`.
- `instance.environment_id` follows `source:` empty for shared folders, owning environment for `Data`/`Mods`. Visible Instances have `environment_id IN ('', <environment>)`.
- Global: Lineage, player data (`lineage_data`, `lineage_tag`), copy notes, user rules, change events (by Instance ownership), plans (07 §1.7). Per environment: current plan, launch records, analysis runs, finding ignores, scan records. Per user data directory: ModsConfig.xml write records.
- Schema creation and upgrades: "Schema and upgrades" above; pre-1.0 databases are refused, not upgraded.
- Version reminder preferences are global database records; storage in 05 §4.2.
- Plan/rule/player-data tables: 04; fact/analysis tables: 05 §3; launch records: 09 §1.

### 4.1 Snapshot File List Retention

`Store.Maintain` cleans `snapshot_file`` and ``snapshot_list` together per snapshot, retaining snapshot rows (declarations, hash, file count, total size) and events. Keep full lists for the union of:

- Every Instance's current snapshot, including tombstones, for incremental scans and identity matching.
- Unanswered questions' `observation_snapshot_id`, plus candidate snapshots selected by `questionevidence.go`: candidate Instance snapshot at or before question scan, excluding observation snapshot, latest by `scan_id DESC, id DESC`. Both current snapshots are also protected above, preserving semantic sets for scan rechecks.
- Before/after snapshots of each Instance's latest 3 updated events, ordered by `scan_id DESC, id DESC`. No age limit.

Delete entire file lists for other snapshots, not only unhashed rows. modchanges still shows declarations/events; if lists cannot be compared, add only "(file lists no longer stored)", without misreporting the remaining side as additions/deletions.

### 4.2 Database Maintenance and Space Reclamation

Entry: `App.Maintain` → `Store.Maintain`; settings "compact", `archo db vacuum [--dry-run]`, MCP `maintenance action=vacuum`, and GUI automatic maintenance share it. Fixed order:

1. Clean old file lists per §4.1 in one transaction.
2. In that transaction, clean version views no game needs, expired non-current analysis slots, unreferenced fact sets per 05 §3 (`pruneFactsOn`, same as analysis-save cleanup).
3. After commit, reclaim space: manual always VACUUM; automatic only if free pages total at least 16 MiB, or at least 1 MiB and 20% of database pages. Keep existing auto_vacuum mode, no schema change.
4. `wal_checkpoint(TRUNCATE)`, record `meta[maintenance:last_completed]`, truncate completion-marker WAL too. Completion says released/current size ("database compacted: freed 183 MB, now 260 MB").

Before VACUUM, require free space on database volume at least twice database page size total plus 64 MiB for temporary database/full WAL; skip reclamation if insufficient/unknown, with manual required-space message ("cannot compact database: it needs 1.2 GB of free disk space"). Already-committed file-list cleanup needs no restore; retry later. Private maintenance connection has zero busy wait; SQLite busy / locked or busy checkpoint aborts immediately, silently for automatic maintenance. On interruption/failure, attempt WAL truncation; otherwise retry later. `meta[maintenance:lease]` prevents multiple windows entering between VACUUM transactions; expiry in 16 §2.16.1 A10.

GUI alone schedules automatic maintenance after first paint; initial delay, attempt period, successful interval, and recheck frequency in [16 §2.16.1](16-interface/2.16.1-scheduled-behavior.md) A08, A10; completion time in meta key above. Skip while any registered game's process, scan, analysis, running/queued background task, or page background file read exists; recheck continuously, cancel on foreground work/game start or window close. Use independent connection, no page connection or page mutex; retry later if busy. CLI, MCP, TUI do not schedule this.

Manual maintenance ignores period/idle restrictions but still refuses while a game runs, asking to close it. `--dry-run` counts file records to delete only (rows plus block entries), without cleanup, compression, or completion-time writes. Tombstone gc: §2.5.

## 5. Acceptance

Synthetic temporary games verify same-volume/simulated cross-volume lock-library moves, recycling, and cross-game pack removal after linking into two games; foreign inbound links/running games block; read-only reports after interruption at each phase and confirmed recovery. All use real junctions/platform symlinks, never player games or RimArcho AppData.

| # | Scenario | Expected | Test | Status |
| --- | --- | --- | --- | --- |
| A1 | First scan of over 300 `Mods` | One Instance per directory; discovered count equals Mod count | `TestA1FirstScanManyMods` | Pass |
| A2 | Workshop updates a Mod | Same Instance, updated | `TestA2WorkshopUpdate` | Pass |
| A3 | Author changes `packageId` in update | Same Instance, `package-id-changed`; unchanged Instance/Lineage IDs preserve plans/user rules | `TestA3PackageIDChange` | Pass |
| A5 | Manually copy Workshop Mod to `Mods/` | New Instance, original Lineage, "inferred" | `TestA5ManualCopy` | Pass |
| A6 | Copy with `.rimarcho/origin.json`, heavily edited | New Instance, same Lineage, "confirmed", copied; later directory rename emits only moved | `TestA6OriginMarker` | Pass |
| A7 | Rename local Mod directory | Same Instance, moved; rename + edit: moved + updated; rename + add 1 file to 4-semantic-file Mod → discovered + disappeared + 1 question | `TestA7LocalRename` | Pass |
| A8 | Resubscribe within 30 days | Same Instance, reappeared | `TestA8Resubscribe` | Pass |
| A9 | Same Workshop ID in Steam and SteamCMD | Two Instances, one Lineage | `TestA9SteamAndSteamCMD` | Pass |
| A10 | Unrelated `Mods` share `packageId` | Two Instances, two Lineages; `pkg:` lists all candidates (exit 3); analysis reports duplicate package ID | `TestA10SamePackageIDUnrelated` | Pass |
| A11 | Directory appears in two sources through junction | One Instance, one alias location | `TestA11Junction` | Pass |
| A12 | `copy-local` creates locked version | New Instance in `Mods/`, `origin.json`, same Lineage, "confirmed"; no `.git` copy or following links outside Mod | `TestCopyLocal` | Pass |
| A13 | RimCrow style: Workshop about-disabled + `Mods/_Link_<id>` junction to RimCrow repository copy | Two Instances, one Lineage; Workshop `unloadable: about-disabled`, `packageId` read normally; `list --filter unloadable` matches; analysis recognizes locked version (`mod.pinned`, 07 §8) | `TestAboutDisabled`, `TestHiddenPinned` | Pass |
| A14 | Enable SteamCMD Mod, write to game, `_Archo_` link in `Mods/`, rescan | Link is alias; no new Instance/events; removing link and rescanning also emits none | `TestLinkDeployment` | Pass |
| A15 | Scan with pack `!ArchoPack_` links in `Mods/` | Skip links; Instance count unchanged | `TestPackBuildAndLaunch` | Pass |
| A17 | Two environments share Workshop | Same Mod has same Instance/Lineage, tags/notes in both; `Mods/` copies join Workshop Lineage even with `--game-only`; plans belong to their respective environments; "used in" lists both sets; environment removal retains shared `Mods`/tags; cannot add same game directory twice | `TestEnvModelSharedWorkshop` | Pass |
| A18 | Unsubscribed Mod and non-Mod folders in Workshop | Former is residue Instance; latter counted only (`ResidueNoAbout`); subscribed not marked residue | `TestResidue` | Pass |
| A19 | Unsubscribed Workshop folder has only `About.xml.disabled` | Both residue/about-disabled | — | Pending validation |
| A20a | 67 small `Mods` share only About/preview/texture paths with tombstone, or content forms 1:67 / 67:1 | No false questions; no questions for bidirectional ambiguity; next scan removes old false questions, retains answers | `TestIdentityQuestionsCommonPathsFanOut`, `TestIdentityQuestionsAmbiguousContent`, `TestIdentityQuestionsRecheckStored` | Pass |
| A20b | Newly discovered Mod referenced by current list, old versions, undo/redo, staging anchors, packs; answer same/copy/unrelated | same remaps references transactionally to original, historical checkout retains entries; copy/unrelated retain references; failure rolls back all | `TestIdentityAnswersPreserveListReferences`, `TestResolveSameReferenceRewriteRollback` | Pass |
| A20 | Disappeared Mod appears elsewhere with new `packageId` (R6), answer same | Restore original Instance ID, add reappeared, delete new Instance discovered | `TestQuestionResolveSame` | Pass |
| A21 | `lineage split` / `lineage merge` | Membership follows command, manual | `TestLineageSplitMerge` | Pass |
| A22 | gc after tombstone expiry | Remove expired tombstones/events, retain unanswered-question references | `TestTombstoneGC` | Pass |
| A23 | Only Workshop ACF manifest changes | New snapshot, no updated | `TestManifestOnlyChange` | Pass |
| A25 | Move all files into `1.6/` unchanged; change contents with unchanged paths | Both have sufficient similarity, same Instance | `TestSimilarityRestructureAndContentOnly` | Pass |
| A27 | `About.xml` UTF-16 LE/BE BOM, UTF-8 BOM, declares utf-16 but UTF-8 | Read declarations correctly | `TestReadDeclUTF16` | Pass |
| A28 | Old Mod lacks `<packageId>` | Derive per §1.5, matching game, "inferred" | `TestDerivePackageID` | Pass |
