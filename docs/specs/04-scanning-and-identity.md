# 04 · Scanning and identity

> Status: implemented.
> Basis: 00a §4 (identity rationale), 03 (environments and sources), 02 §1.

This specification defines scanning, how scanned directories are reconciled with known Instances, Lineage, change events, identity questions, and their storage.

## 1. Scanning

### 1.1 Flow

```
Check source states → list source subdirectories → group (merge identical real paths) → read observations → reconcile → write snapshots → emit change events → mark stale facts
```

- Command: `archo scan [--full] [--game-only]`. `--full` ignores cached hashes and rereads all semantic files. The TUI and GUI run `archo scan` in a subprocess; the main process never scans, so the interface process does not keep the scan's peak memory.
- **Scope**: the shared Mod folders and the current environment's own `Data` and `Mods`, each directory once. `--game-only` scans only the current environment's two sources: after an environment is added (guide step 2), only the new game needs reading. Live Instances in unscanned shared sources are neither matched nor tombstoned, but remain R4 "original" candidates, so copies in `Mods/` still join the Workshop originals' Lineages. If the shared folders have never been read, guide step 2 leaves out `--game-only`.
- Before starting, check the data directory (03 §3.1) and refresh each source's state (03 §4.1); skip missing, `rebind-pending` and retired sources.
- When reading origin markers (`.rimarcho/origin.json`), first map merged-away IDs through `id_alias` (03 §2.1).
- A subdirectory is a Mod if it has `About/About.xml`, or only `About/About.xml.disabled` (§1.2). File names are case-insensitive. Other subdirectories are counted, without Instances.
- **Concurrency**: one goroutine per source, with up to `NumCPU` workers in each. IDs are assigned in the deterministic reconciliation order, independent of concurrency.
- **Incremental**: reuse the previous hash when the relative path, size and mtime are unchanged. mtime only skips hashing; it never decides identity.
- **Memory**: incremental scans load only the hashed semantic file rows, not the full list. When a directory is unchanged and nothing was rehashed, its list is released after reading; when only external state changed (such as the ACF manifest), the old snapshot's file rows are copied to the new snapshot in SQL. Measured memory and timing: 18.
- Scans update only identity and metadata, plus the type index of the scanned environment's own game assemblies (05 §5.3). Analysis rebuilds Def, Patch and Harmony facts on demand (05 §3).
- Scan output lists per source the subdirectory count, Mod count, residue count, about-disabled count, non-Mod directory count, count-only residue directories and alias count; Workshop sources without an ACF show `no ACF`.

### 1.2 Residue and about-disabled

**Subscriptions and residue** (workshop sources only; the game does not load SteamCMD directories directly):
- The game loads only **subscribed** Workshop items (17 §6). Subscribed means listed in `WorkshopItemsInstalled` or `WorkshopItemDetails` of `appworkshop_294100.acf`.
- Without an ACF, everything counts as subscribed.
- Folders missing from the subscription records are residue: they still get Instances but cannot be added to plans, and can it run suggests cleaning them up (05 §4.3, 02 §3).
- Residue folders with neither `About.xml` nor `About.xml.disabled` are only counted, without Instances.

**about-disabled**:
- If `About/About.xml` is absent but `About/About.xml.disabled` exists (RimCrow's way of disabling), read it as the declaration, create an Instance and mark it `unloadable: about-disabled`. Such Instances cannot join plans; filter them with `list --filter unloadable`.
- A Workshop residue folder containing only `About.xml.disabled` is both residue and about-disabled.
- Scans never rename files back. Analysis classifies these Instances as below, computed on each analysis and not stored:

| Category | Condition | Finding code | Description |
| --- | --- | --- | --- |
| Locked version | The same Lineage has another live, loadable, non-Workshop Instance (RimCrow `_<Workshop-ID>_` copies, `copy-local` copies and copies deployed in `Mods/` count) | `mod.pinned` (info; warning if upstream updated) | No `mod.unloadable`: the copy is the effective one and the Workshop copy is upstream. Product rules: 07 §5 |
| Unsubscribed residue | Workshop source, missing from the ACF subscription list | `mod.hidden-residue` (warning) | May be recycled (02 §3) |
| Hidden on its own | Everything else | `mod.hidden` (warning) | Keep hidden, restore, or recycle (09 §5) |

### 1.3 Observations

Recorded for each Mod directory:

| Category | Contents |
| --- | --- |
| Location | `source_id`, relative path, real path, link flag, alias locations |
| Declarations | `About.xml`: `packageId`, name, author/authors, `modVersion`, `supportedVersions`, description, `modDependencies` and `modDependenciesByVersion`, `loadBefore`, `loadAfter`, `forceLoadBefore`, `forceLoadAfter`, `incompatibleWith`, `loadBeforeByVersion`, `loadAfterByVersion`, `incompatibleWithByVersion`, `descriptionsByVersion`; `About/PublishedFileId.txt` (BOM and whitespace stripped); the original `LoadFolders.xml` |
| Workshop ID | Workshop-type sources (workshop, steamcmd): the directory name; others: `PublishedFileId.txt` |
| External state | Workshop-type sources: ACF manifest ID, timeupdated, size, subscription status; with `.git`: the `HEAD` commit (read `HEAD`, then the ref file or `packed-refs`) |
| Content fingerprint | See §1.4 |
| Origin marker | `.rimarcho/origin.json` (if present and `origin_instance` is not empty) |
| State | residue, unloadable |

**Tolerant `About.xml` parsing**:
- Encoding: detect UTF-16 LE/BE and UTF-8 BOMs and convert to UTF-8; without a BOM but with XML `encoding="utf-16"`, read UTF-16, unless the content is actually UTF-8, in which case follow the content. The same applies to `LoadFolders.xml`, Defs and Patches.
- Tolerate tag case, mixed author/authors, whitespace in `li`, and package ID case.
- Split authors like the game's `ModMetaData.Authors`: on ` and `, `&` and `,`, trimming whitespace.
- A dependency `li` may contain a `packageId` child or plain text.
- A malformed `About.xml` still creates an Instance; the error is recorded in the declaration and the scan goes on.

`packageId` is always stored twice: the original `package_id_raw` and the lowercase `package_id`. A missing `packageId` is derived by §1.5.

### 1.4 Fingerprints and similarity

| Layer | Scope | Calculation |
| --- | --- | --- |
| Structural list | All files | Relative path + size (+ mtime, incremental only) |
| Semantic hash | All files under `About/`, `LoadFolders.xml`, XML under any `Defs/` or `Patches/` (including version directories like `1.6/Defs/`), `.dll` under any `Assemblies/` | xxh3-128 per file |
| Snapshot hash | Both layers above | Sort by lowercase path and hash each "lowercase path + size + semantic hash (if any)", to detect changes quickly. Because paths are lowercased, a case-only rename on Windows is not an update |

- Large files such as textures and audio are compared by size only, without reading their content.
- `.git`, `.svn` and `.rimarcho` are excluded, so locked versions and their originals have identical fingerprints.
- Links and device files inside Mod directories are not followed; unreadable entries are skipped.

**Similarity**: over the semantic file sets of two snapshots, take the larger of two Jaccard coefficients:
- `J(paths)`: compares lowercase relative paths only. Recognizes "move + update": contents changed, structure unchanged.
- `J(hashes)`: compares content hashes without paths, as a **multiset** (each file with identical content counts separately). Recognizes "reorganization": files moved into subdirectories like `1.6/` without content changes.

When neither side has semantic files, similarity is 0. R3 and R4 also require matching declarations; R6 does not take the same paths as the same content, and uses the content evidence in §2.1 instead. Adding files to a small Mod can lower similarity below 0.85 (adding one file to 4 semantic files gives 0.8); R6 then asks for confirmation, which is an acceptable, conservative outcome.

### 1.5 Derived packageId

The game accepts very old Mods without `<packageId>`. RimArcho derives the ID as `ModMetaDataInternal.TryParsePackageId` does (confirmed by decompiling 1.7.4982, 17 §6) and marks it "inferred" (`package_id_derived`):

```
ConvertToASCII(author + hash3(description)) + "." + ConvertToASCII(name)
```

- Default author: `Anonymous`.
- `hash3`: nothing for an empty description; otherwise compute `GenText.StableStringHash` (initial 23, over UTF-16 code units `n = n*31 + c`, with int32 wraparound), in decimal, drop the minus sign and take the first 3 digits.
- `ConvertToASCII`: per UTF-16 code unit, keep letters and digits and replace any other `c` with `c % 25 + 65`.
- The result is lowercased into `package_id`.
- Official content (official source) without a `packageId`: `Core` uses `ludeon.rimworld`, the others `ludeon.rimworld.<lowercase-directory-name>`.

## 2. Reconciliation

Input: all observations of the current scan; the database's Instances (including tombstones); the list of unscanned sources. Output: an Instance ID per observation, change events, identity questions, and the Instances that disappear in this scan. Reconciliation is pure logic (`internal/identity`); the caller does the database I/O.

### 2.1 Rules R1–R7

Observations and Instances that a rule matched take no part in later rules.

| Rule | Condition | Result | Event |
| --- | --- | --- | --- |
| R1 | Same `source_id` + relative path (case-insensitive) | Same Instance | See §2.3 |
| R2 | The observation is from a Workshop-type source (workshop, steamcmd); another Instance from a Workshop-type source has the same directory name (the same Workshop ID) and was not seen in this scan | Same Instance (for example Workshop content that moved to another Steam library) | moved |
| R5 | The observation has `.rimarcho/origin.json` | New Instance that joins the recorded Lineage, marked "confirmed"; exceptions below | copied |
| R3 | A live Instance not seen in this scan has similarity ≥ 0.85 to the observation and a matching `packageId` or Workshop ID declaration | Same Instance | moved |
| R4 | An Instance already seen in this scan has similarity ≥ 0.6 to the observation and a matching `packageId` or Workshop ID declaration | New Instance that joins its Lineage, marked "inferred" | copied |
| R6 | A tombstone or an Instance disappearing now passes the `identity.QuestionScore` content check, with a unique candidate in both directions (below) | New Instance, new Lineage, and the identity question "is this #xxxx returning?" | discovered |
| R7 | Everything else | New Instance, new Lineage | discovered |

- **Matching declarations**: the same nonempty `packageId` (case-insensitive), or a valid Workshop ID; blank, `0` and nonnumeric Workshop IDs never match. R2 directory names must also be valid Workshop IDs.
- **R5 before R3 and R4**: origin markers are authoritative, even after edits so large that similarity no longer recognizes the copy.
- **R5 rename exception**: if a live Instance not seen in this scan has the same origin marker in its latest snapshot (the same `origin_instance`) and similarity ≥ 0.85, it is a "renamed copy": the same Instance, a moved event, and no new copy. Otherwise renaming a locked version's directory would create another Instance.
- **R5 Lineage**: the marker's `origin_lineage` if it exists in this environment; otherwise the current Lineage of `origin_instance`; if neither exists, a new Lineage (copied is still emitted).
- When R2–R5 have several candidates, the one with the highest similarity wins.
- **R6 content evidence**: only the content hashes of Defs and Patches XML and Assemblies DLLs count; About files, previews, `LoadFolders.xml` and empty content are excluded; duplicates count once; paths are ignored. The content-set Jaccard must be ≥ 0.6; with matching declarations at least one shared content is required, otherwise at least three distinct shared contents. Names, authors, folder names and common texture paths alone never raise a question. Implementation: `identity.QuestionScore`, `identity.CompareFiles`.
- **R6 ambiguity guard**: if a new observation has several eligible candidates, or several observations would choose one candidate, each is recorded through R7 without questions; RimArcho neither picks the highest score nor asks per pair.
- "inferred" and "confirmed" are stored in the Instance's `lineage_basis` (§3).

### 2.2 Execution order and candidate scope

**Two phases**:
1. Run R1, R2, R5 and R3, rule by rule, over all observations, matching only Instances already in the database.
2. Run each remaining observation through R4 → R6 → R7.

This lets Instances that R7 created earlier in the same scan serve as R4 originals. Otherwise the first scan would give unrelated Lineages to Workshop originals and their `Mods/` copies, or to the same Workshop ID in Steam and SteamCMD.

**Observation order**: workshop → steamcmd → official → local → extra; sources of the same kind in environment order, then by lowercase relative path. Originals are established before copies.

**Candidate scope**:

| Rule | Candidates |
| --- | --- |
| R2 | Workshop-type Instances not seen in this scan, **including tombstones**; live ones preferred. A matched tombstone emits reappeared, plus moved if its location changed |
| R5 rename exception, R3 | Instances live before the scan and not seen this time; no tombstones (they are reserved for R6) |
| R4 | Instances already seen in this scan, including ones just created |
| R6 | Tombstones and Instances disappearing now |

Instances in unscanned sources (missing, `rebind-pending`) are excluded from all candidates and do not become tombstones.

**Direction**: the R4 "original" is whichever copy was processed first. If a Workshop original disabled by RimCrow became an Instance after a deployed copy did (for example, the copy was scanned before about-disabled support existed), events record the Workshop copy as an "inferred copy" of the deployed one. The Lineage is unaffected; the primary Instance still follows the priority in §3.

### 2.3 Events

**Identity events**:

| Event | When | detail |
| --- | --- | --- |
| `discovered` | Created by R6 or R7 | `src:<label> <relative-path>` |
| `copied` | Created by R4 or R5 | R4: `inferred copy of #<original>`; R5: `origin #<source-Instance> (.rimarcho/origin.json)` |
| `moved` | R2, R3, R5 rename exception | `src:<old-label> <old-path> → src:<new-label> <new-path>` |
| `reappeared` | R1 or R2 matches a tombstone, or the player answers same to an identity question | `src:<label> <relative-path>` |
| `disappeared` | A live Instance is still unseen after the scan (and its source was scanned) | `src:<label> <relative-path>`; it then becomes a tombstone |
| `updated` | An existing Instance matched and its snapshot hash changed (including R2/R3 move + update) | — |

**Field events**: only when the snapshot hash changes, the old and new declarations are compared and each difference is emitted separately:

| Event | Comparison | detail |
| --- | --- | --- |
| `package-id-changed` | Lowercase `packageId` | `pkg:<old> → pkg:<new>` |
| `renamed` | name | `"<old>" → "<new>"` |
| `author-changed` | Author list | `<old, …> → <new, …>` |
| `version-support-changed` | `supportedVersions` | `<old> → <new>` (space-separated) |
| `dependencies-changed` | All dependency packageIds (including versioned ones; lowercased, deduplicated, sorted) | `+pkg:<x> -pkg:<y>` |
| `load-rules-changed` | `loadBefore`/`loadAfter`/`forceLoad*`/`incompatibleWith` and their versioned forms (lowercased, sorted, compared) | — |

- One scan may emit several events for one Instance. All share the scan ID (`scan:<n>`).
- Each event references its before and after snapshots (`before_snapshot`, `after_snapshot`), so file-level diffs are possible. Change history (16 §2.9) combines a Mod's events over a period into one row by comparing the declarations and file lists of the earliest `before_snapshot` and the latest `after_snapshot`; the details of `load-rules-changed` also come from those declarations.
- An `_Archo_` link appearing or disappearing changes only alias locations, without events (03 §4.3).
- Events are shown in the GUI's Mod library › updates (16 §2.9), by `archo changes [<reference>] [--since …] [--all]` (latest scan by default, 13 §4), and in MCP.

**When a snapshot is written**: a new snapshot is written only when the snapshot hash, the ACF manifest, the git `HEAD` or the origin marker changes. If only mtimes change, the file mtimes of the current snapshot are updated in place. A change of the ACF manifest alone creates a snapshot without an updated event.

### 2.4 Identity questions

- R6 records the observation's new Instance, the candidate Instance, the content overlap, and a small comparison summary (the shared content count, each side's content count, declaration differences). The summary is stored with the question, so building pages reads question records only, never file lists.
- Every successful scan rechecks the visible unanswered questions in the same `Store.ApplyScan` transaction: it applies the R6 content checks to the current snapshots, and deletes unanswered questions whose evidence is insufficient, whose references are missing, or which are one of several eligible questions involving the same Mod. It uses only stored snapshots, so it also checks unscanned sources (partial scans, other games' `Mods/`). Answered questions, including the player's same / copy / unrelated decisions, are excluded. It does not change Mods, lists, history or grouping.
- Viewing: `archo identity [list] [--all]` (with no subcommand it also lists; `--all` includes answered ones), `archo identity why q:<n>`; in the GUI, can it run › "needs your answer" and the editor's problems panel (16 §2.4, §5). Listing, inspecting and answering in the CLI and MCP include the same short comparison.
- **Comparison evidence is loaded on demand**: `Store.QuestionEvidence` reads declarations and file records from the stored observation snapshot and from the candidate's snapshot at the question's scan. Folders and Workshop IDs derived from directory names use the current records; if the records were cleaned up, it says explicitly that the comparison is unavailable. `cmds.CompareIdentity` / `IdentityReasonLines` share the comparison: fields absent on both sides are left out, there is one conclusion about the content overlap, and at most five shared files plus the remaining count are listed; there is no explanation of the matching method. Interface structure: [16 §5](16-interface/4-details-and-feedback.md#5-can-it-run-wording). No Mod files are scanned and nothing is read from the network.
- Answering: `archo identity resolve q:<n> same|copy|unrelated`; in the interface "same", "a copy", "unrelated".
  - same: merge the new Instance into the candidate. The candidate keeps its ID, Lineage and history, and takes the new location, snapshots, events and aliases. One transaction rewrites the current lists, undo/redo, historical versions, and staging entries and anchors of all plans, keeping the first duplicate's position and missing entries unchanged; launch list fingerprints are rewritten too. `id_alias` maps the merged ID and the emptied Lineage, so pack manifests and origin markers resolve to the survivor without changing files. The new Instance's discovered event is deleted and a reappeared event is added under the question's scan; other questions involving both Instances are marked superseded. same is refused if R1 or R2 has meanwhile matched the candidate back.
  - copy: join the candidate's Lineage, marked "confirmed".
  - unrelated: close the question only.

### 2.5 Tombstones and gc

- A disappeared Instance becomes a tombstone with `tombstone_at`, keeping its Lineage, rules, aliases and historical snapshots.
- The default retention is 90 days, set by a nonnegative `tombstone_days` in `config.toml`.
- `archo gc [--dry-run]` (Settings "remove old tombstones") removes:
  - Expired tombstones and their events, except those referenced by unanswered questions;
  - Old snapshots that are neither current nor referenced by events or questions;
  - Lineages left empty.
- `--dry-run` only counts.
- The player runs tombstone gc by hand. Periodic database maintenance does not change tombstones or identity; it only cleans old file lists by §4.1.

## 3. Lineage (including locked versions)

- Every new Instance creates its own Lineage by default (an empty `lineage_basis` means "founding member").
- Ways to join an existing Lineage, with their markers:

| Method | `lineage_basis` |
| --- | --- |
| R4 inferred copy | inferred |
| R5 origin marker, copy answer | confirmed |
| Manual split or merge | manual |

- Manual split and merge: `archo lineage split <reference>` (moves the Instance into a new Lineage of its own), `archo lineage merge <reference> <reference>` (merges the second Lineage into the first).
- Splitting copies the original's alias, Mod notes and tags into the new group; after that, edits are independent. Copy notes stay with the Instance. User rules and finding ignores stay with the original group. Identity changes and the copying of player data happen in one transaction; the Instance ID, path, snapshots and plan entries stay unchanged. Rescans keep manual grouping; origin markers and upstream update relationships remain.
- **Primary Instance**: gives the Lineage its default display name; priority: live → Workshop-type source → earliest `first_seen` → ID.
- **What belongs to a Lineage**: user rules and player data (aliases, tags, notes) belong to the Lineage, are shared by originals and locked versions, and survive renames; plan entries belong to Instances (07 §1, §6; rationale 00a §4.3).
- **Known hard cases**: an author forks a new Mod with a different Workshop ID but the same `packageId` → separate Lineages, and analysis reports the duplicate package ID; the same Workshop ID in the Steam Workshop and SteamCMD → two Instances, one Lineage.

**Locked version identity** (product rules: 07 §5):
- A locked version is a new, R5-confirmed Instance in the same Lineage. Creating one (`copy-local` is the low-level command) copies the Mod into a local source (default `<original-directory-name>_local`) and writes `.rimarcho/origin.json`:

```json
{
  "origin_instance": "<full source Instance ID>",
  "origin_lineage": "<full Lineage ID>",
  "workshop_id": "<Workshop ID, may be empty>",
  "origin_snapshot": "<snapshot hash at copy time, hexadecimal>",
  "copied_at": "2026-10-21T08:00:00Z"
}
```

- The next scan's R5 joins it to the original Lineage, marks it "confirmed" and emits copied. A later directory rename is a moved event by the §2.1 exception, so the locked version keeps its identity.
- RimArcho never writes marker files into Workshop folders, because Steam verification and updates delete them.
- Copies the player makes by hand have no marker; R4 marks them "inferred".
- RimCrow style (Workshop `About.xml.disabled`, a copy in `Mods/`): R4 joins both into one Lineage; analysis recognizes the locked version by §1.2.
- unpin swaps the plan's copy back to the Workshop version without changing identity data; the copy stays where it is (07 §5).

## 4. Storage

**Schema and upgrades**: a new database is created from `internal/store/schema.sql` (embedded), then `meta.schema_version = store.SchemaVersion`. The first public release (1.0.0) shipped schema 29 (`store.FirstReleaseSchema`). A schema change edits `schema.sql`, bumps `SchemaVersion` and appends one step to `store.upgrades` (30: game type index, 05 §5.3; 31: save full-read cache, 12 §1.7; 32: tag notes, 07 §6.1). Opening an older database runs the missing steps in order, each in one transaction, which may report row progress. A database with schema 1–28 (from a pre-1.0 development build) is refused with `store.ErrPreRelease` and left unchanged: "database from a pre-1.0 development build (schema N); move <path> away to start with a new database". A schema newer than `SchemaVersion` is refused as "unsupported schema version". Test: `TestPreReleaseDatabaseRefused`.

**Upgrade progress at startup** (steps 30 and 31 only create empty tables, so they finish well within the 300 ms below): progress is shown when opening runs upgrade steps (`store.OpenUpgrading`, `app.OpenWith`, `cli.upgradeReport`), and hidden if the upgrade completes within 300 ms. `RimArcho.exe` has no console, so it shows a small native window (`internal/surface/upgradewin`: RimArcho title, one line of text or a progress bar, cannot be closed, system DPI, interface language) that closes when the upgrade completes. The TUI writes the same line to stderr before taking over the terminal (percentage refreshed in place, interface language). The CLI and MCP write one English stderr line, "Upgrading the RimArcho database (schema N → M)…", then the elapsed time. stdout is unchanged, and the MCP protocol stream gets nothing extra. Steps that report rows show a percentage; others show only "upgrading". If opening fails, `RimArcho.exe` also shows a modal error box in the interface language (`cli.upgradeReport.openFailed`, `upgradewin.ShowError`): a pre-1.0 database gets its own text naming `archo.db` and saying to move or rename it; other errors show the error text. OK exits. The CLI, TUI and MCP only write to stderr.

**Managed links** (`managed_link`): records of the links RimArcho created, with phase and path indexes; existing Instances and aliases are never turned into proven creation records. The intent is committed before any link file operation, then the phases detached, `records-updated` and complete follow; a safe rollback records `rolled-back`. Startup checks unfinished rows read-only; the problems page and scans report `mods.link-recovery`; recovery happens only through the existing confirmed `links repair`, never by silent rebuilding.

**Identity question summary**: `identity_question.summary` stores the small comparison summary of §2.4; `identity_question_open(status, environment_id)` indexes unanswered questions.

**Fact cache structure and cleanup**: the set structure and reference cleanup are in 05 §3; `Store.PruneFacts(now)` is a separate maintenance entry point. Removing an environment and cleaning up tombstones also prune facts in the same transaction; visible environments and analysis slots keep shared Instances.

SQLite (modernc, WAL + `synchronous=NORMAL` + `journal_size_limit` 16 MB). After a large write, the next reset truncates the WAL to 16 MB instead of keeping its peak size while connections stay open; after analysis writes, or scans that create snapshots, a nonwaiting `wal_checkpoint(TRUNCATE)` is also attempted (`Store.TruncateWAL`). Without this, about 380,000 analysis rows leave about 100 MB of WAL while the GUI is open. Identity tables (an empty `environment_id` means shared by all environments):

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

- `decl_blob` stores the normalized `About.xml` (JSON) for analysis.
- **Snapshot file lists are kept in two places** (`store/filelist.go`): hashed semantic files and `.dll` files (anywhere, ASCII case folding only) are `snapshot_file` rows; the remaining files (textures, sounds, Languages, `.meta` and so on) form one `snapshot_list` block. The block is sorted by `rel_path`; each entry stores the length of the prefix it shares with the previous entry, the suffix and the size (no mtime: only incremental scans need it, and they read only the hashed rows, `Store.CurrentFiles`); the whole block is zstd-compressed, and `n` is the entry count. Blocks **are not deduplicated**: copying the list for identical content or new external state copies the whole block, and refreshing mtimes in the same snapshot rewrites the whole block. Snapshots without non-semantic files have no block. Readers: `Store.SnapshotFiles` (file diffs on the changes page) merges both and sorts by path; `Store.InstanceFiles` (looking up which Mod provides a texture or DLL named in the log) queries the rows first, skips blocks for patterns ending in `.dll`, and otherwise decompresses current blocks and matches SQLite `LIKE` rules in Go (`%`, `_`, ASCII case folding only). DLLs stay rows because looking up a Mod by DLL name (`%x.dll`) is the commonest query and must not decompress every block.
- `snapshot.kinds` and `snapshot.langs` are derived from the file lists when a snapshot is written (`store.deriveContent`). kinds bits: `.dll` anywhere; XML under `Defs/` or `Patches/` at any loading-folder depth; files under `Languages/`; PNG under directories whose names end in textures (`%textures/%.png`, convertible textures). langs: the language names from `Languages/<name>/…` at the root or in the next level of loading folders, in path order, keeping only the first of case-insensitive duplicates, joined by `/`. Case folding is ASCII only, as SQLite `LIKE` does. Snapshot paths never change (a different path or size means a different snapshot hash), so the columns stay valid even after §4.1 removes files; copying a list for identical content or new external state copies both columns. The Mod library's content types (`Store.ContentKinds`), convertible textures (`Store.SnapshotsWithPNG*`) and detail languages (`Store.Languages`) read these columns by primary key instead of scanning `snapshot_file`.
- `instance.environment_id` follows its source: empty for shared folders, the owning environment for `Data` and `Mods`. Visible Instances have `environment_id IN ('', <environment>)`.
- Which records are global, per environment, per game version or per user data directory: 03 §2. Change events follow their Instance's ownership; analysis runs and scan records are per environment.
- Version reminder preferences are global database records; storage in 05 §4.2.
- Plan, rule and player-data tables: 07, 08; fact and analysis tables: 05 §3; launch records: 09 §1.

### 4.1 Snapshot file list retention

`Store.Maintain` cleans `snapshot_file` and `snapshot_list` together, per snapshot, keeping the snapshot rows (declarations, hash, file count, total size) and events. Full lists are kept for the union of:

- Every Instance's current snapshot, including tombstones', for incremental scans and identity matching.
- The `observation_snapshot_id` of unanswered questions, plus the candidate snapshots `questionevidence.go` selects: the candidate Instance's latest snapshot at or before the question's scan, excluding the observation snapshot, ordered by `scan_id DESC, id DESC`. Both current snapshots are also protected by the item above, which keeps the semantic sets that scan rechecks need.
- The before and after snapshots of each Instance's latest 3 updated events, ordered by `scan_id DESC, id DESC`. No age limit.

Other snapshots lose their whole file list, not only the unhashed rows. modchanges still shows declarations and events; when lists cannot be compared, it adds only "(file lists no longer stored)" and does not report the remaining side as additions or deletions.

### 4.2 Database maintenance and space reclamation

Entry point: `App.Maintain` → `Store.Maintain`; Settings "compact", `archo db vacuum [--dry-run]`, MCP `maintenance action=vacuum` and the GUI's automatic maintenance share it. Fixed order:

1. Clean old file lists by §4.1 in one transaction.
2. In the same transaction, clean version views no game needs, expired non-current analysis slots, and fact sets nothing references, by 05 §3 (`pruneFactsOn`, as analysis saves do).
3. After the commit, reclaim space: manual maintenance always runs VACUUM; automatic maintenance only if free pages total at least 16 MiB, or at least 1 MiB and 20% of the database's pages. The existing auto_vacuum mode is kept, with no schema change.
4. Run `wal_checkpoint(TRUNCATE)`, record `meta[maintenance:last_completed]` and truncate the WAL of that completion marker too. Completion reports the space freed and the current size ("database compacted: freed 183 MB, now 260 MB").

Before VACUUM, the database's volume must have free space of at least twice the total database page size plus 64 MiB, for the temporary database and a full WAL; if there is less, or it is unknown, reclamation is skipped, and manual maintenance says how much space it needs ("cannot compact database: it needs 1.2 GB of free disk space"). File-list cleanup already committed needs no restore; retry later. The private maintenance connection waits zero time when busy: SQLite busy / locked or a busy checkpoint aborts at once, silently for automatic maintenance. On interruption or failure, WAL truncation is attempted; otherwise it retries later. `meta[maintenance:lease]` keeps several windows from entering between VACUUM transactions; expiry in 16 §2.16.1 A10.

Only the GUI schedules automatic maintenance, after the first paint. The initial delay, attempt period, interval after success and recheck frequency are in [16 §2.16.1](16-interface/2.16.1-scheduled-behavior.md) A08 and A10; the completion time is in the meta key above. It is skipped while any registered game's process, a scan, an analysis, a running or queued background task, or a page's background file read exists; it keeps rechecking, and is cancelled by foreground work, a game starting, or the window closing. It uses its own connection, no page connection or page mutex, and retries later when busy. The CLI, MCP and TUI do not schedule it.

Manual maintenance ignores the period and idle conditions but still refuses while a game runs, asking the player to close it. `--dry-run` only counts the file records it would delete (rows plus block entries), without cleaning, compacting or writing the completion time. Tombstone gc: §2.5.

## 5. Acceptance

| # | Scenario | Expected | Test | Status |
| --- | --- | --- | --- | --- |
| A1 | First scan of over 300 Mods | One Instance per directory; the discovered count equals the Mod count | `TestA1FirstScanManyMods` | Pass |
| A2 | The Workshop updates a Mod | Same Instance, updated | `TestA2WorkshopUpdate` | Pass |
| A3 | The author changes `packageId` in an update | Same Instance, `package-id-changed`; unchanged Instance and Lineage IDs preserve plans and user rules | `TestA3PackageIDChange` | Pass |
| A5 | Copy a Workshop Mod into `Mods/` by hand | New Instance, original Lineage, "inferred" | `TestA5ManualCopy` | Pass |
| A6 | A copy with `.rimarcho/origin.json`, heavily edited | New Instance, same Lineage, "confirmed", copied; a later directory rename emits only moved | `TestA6OriginMarker` | Pass |
| A7 | Rename a local Mod directory | Same Instance, moved; rename + edit: moved + updated; rename + adding 1 file to a Mod with 4 semantic files → discovered + disappeared + 1 question | `TestA7LocalRename` | Pass |
| A8 | Resubscribe within 30 days | Same Instance, reappeared | `TestA8Resubscribe` | Pass |
| A9 | The same Workshop ID in Steam and SteamCMD | Two Instances, one Lineage | `TestA9SteamAndSteamCMD` | Pass |
| A10 | Unrelated Mods share a `packageId` | Two Instances, two Lineages; `pkg:` lists all candidates (exit 3); analysis reports the duplicate package ID | `TestA10SamePackageIDUnrelated` | Pass |
| A11 | A directory appears in two sources through a junction | One Instance, one alias location | `TestA11Junction` | Pass |
| A12 | `copy-local` creates a locked version | New Instance in `Mods/`, `origin.json`, same Lineage, "confirmed"; no `.git` copied and no links followed out of the Mod | `TestCopyLocal` | Pass |
| A13 | RimCrow style: Workshop copy about-disabled + `Mods/_Link_<id>` junction to the copy in the RimCrow repository | Two Instances, one Lineage; the Workshop copy is `unloadable: about-disabled` and its `packageId` is read normally; `list --filter unloadable` matches; analysis recognizes the locked version (`mod.pinned`, 07 §8) | `TestAboutDisabled`, `TestHiddenPinned` | Pass |
| A14 | Enable a SteamCMD Mod, write to the game (an `_Archo_` link in `Mods/`), rescan | The link is an alias; no new Instance or events; removing the link and rescanning also emits none | `TestLinkDeployment` | Pass |
| A15 | Scan with pack `!ArchoPack_` links in `Mods/` | The links are skipped; the Instance count is unchanged | `TestPackBuildAndLaunch` | Pass |
| A17 | Two environments share the Workshop | The same Mod has the same Instance and Lineage, with its tags and notes, in both; `Mods/` copies join the Workshop Lineage even with `--game-only`; "used in" names each plan once, whichever game uses it; removing an environment keeps the shared Mods and tags; the same game directory cannot be added twice | `TestEnvModelSharedWorkshop` | Pass |
| A18 | An unsubscribed Mod and non-Mod folders in the Workshop | The former is a residue Instance; the latter are only counted (`ResidueNoAbout`); subscribed Mods are not marked residue | `TestResidue` | Pass |
| A19 | An unsubscribed Workshop folder has only `About.xml.disabled` | Both residue and about-disabled (`mod.hidden-residue`) | `TestHiddenResidueTrash` | Pass |
| A20a | 67 small Mods share only About, preview or texture paths with a tombstone, or the content forms 1:67 / 67:1 | No false questions; no questions for two-way ambiguity; the next scan removes old false questions and keeps answers | `TestIdentityQuestionsCommonPathsFanOut`, `TestIdentityQuestionsAmbiguousContent`, `TestIdentityQuestionsRecheckStored` | Pass |
| A20b | A newly discovered Mod referenced by the current list, old versions, undo/redo, staging anchors and packs; answer same / copy / unrelated | same remaps the references to the original in one transaction, and checking out history keeps its entries; copy and unrelated keep the references; a failure rolls everything back | `TestIdentityAnswersPreserveListReferences`, `TestResolveSameReferenceRewriteRollback` | Pass |
| A20 | A disappeared Mod appears elsewhere with a new `packageId` (R6); answer same | The original Instance ID is restored, reappeared is added, and the new Instance's discovered event is deleted | `TestQuestionResolveSame` | Pass |
| A21 | `lineage split` / `lineage merge` | Membership follows the command, manual | `TestLineageSplitMerge` | Pass |
| A22 | gc after tombstones expire | Expired tombstones and events are removed; references of unanswered questions are kept | `TestTombstoneGC` | Pass |
| A23 | Only the Workshop ACF manifest changes | New snapshot, no updated | `TestManifestOnlyChange` | Pass |
| A25 | Move all files into `1.6/` unchanged; change contents with paths unchanged | Both keep enough similarity: same Instance | `TestSimilarityRestructureAndContentOnly` | Pass |
| A27 | `About.xml` with a UTF-16 LE/BE BOM, a UTF-8 BOM, or declaring utf-16 while being UTF-8 | Declarations are read correctly | `TestReadDeclUTF16` | Pass |
| A28 | An old Mod without `<packageId>` | Derived by §1.5, matching the game, "inferred" | `TestDerivePackageID` | Pass |
