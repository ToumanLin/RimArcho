# MVP acceptance (01 §2, §5)

Status at the end of M5 (2026-10-19), updated after the post-review polish (stale logs, cached log
attribution; `docs/decisions-m5.md` §7). Every claim below points to a test that runs in `go test ./...` or to a
measurement on the real environment, which was used read-only. Where something is not met or was not
verified, the row says so.

**Real environment**

| Item | Value |
|---|---|
| Game | RimWorld 1.7.4982 on Windows 11, SSD |
| Mods | 1354 mod folders: 6 official, 74 local, 1274 Workshop. 485 are enabled (ModsConfig.xml is a 1.6 list) |
| Player.log | 1.6 log, 5814 lines, 8.7 MB, written 2026-09-29 (before the 1.7 update). RimArcho treats it as a **stale log** (another game version): its 26 error and 86 warning clusters are folded under one row and not counted. `archo problems` on the real environment: 1 error, 46 warning |
| RimCrow DB | `D:\Game\RimModManager\data\mod_manager.db` |

**Test suite:** 15 packages, all passing (`go test ./...`).

## 1. User stories (01 §2)

| # | Story | Result | Evidence |
|---|---|---|---|
| U1 | Find game, user data and Workshop on first start; manual paths otherwise | **Pass** | `env.TestDetect`; `env add --game/--user-data/--workshop`. Real: `archo env detect` found all three paths, the 1.7.4982 version and the Steam launch method |
| U2 | One list of all mods: enabled, order, problems | **Pass** | TUI list view, `list --active`; `tui.TestListEnableMoveSortSaveUndo`, `TestSmallTerminalAndCJK`, `TestProblemsView` |
| U3 | Enable, disable, move, save to the game | **Pass** | `app/loadout_test.go` (edits, backup, atomic write, `_steam` tokens), `loadout.TestWriteFileBackupAtomic`, TUI test (space / J / w). M5 adds `loadout export/import` (`TestLoadoutExchange`) |
| U4 | One-key sort; show the conflicting constraints when it cannot sort | **Pass** | S1–S5 in `order/order_test.go` and the CLI golden `loadout`. A cycle names each About.xml; `rule add ignore` breaks it |
| U5 | See missing deps, incompatibilities, unsupported versions, duplicate packages, and how to fix them | **Pass** | C1–C4 in `app/problems_test.go` and `analyze_test.go`; golden `problems`. Each problem has evidence and fix commands (`problems show`) |
| U6 | After a game error, see which mod it is attributed to and the evidence | **Pass** | B1–B5: golden `harmonylog`, `app/m4_test.go`. Real log: 153 clusters, and patch failures map to file:line with a reason (`docs/m4-mcp-transcripts.md`). Log problems are grouped per log; a log from another game version or older than the Loadout's last change is stale, folded and not counted (`TestStaleLog`, `tui.TestProblemsViewLogs`, golden `harmonylog`) |
| U7 | Updates and author renames do not break lists and rules; I see what changed | **Pass** | A2 (update), A3 (packageId change keeps the Loadout and rules), A7 (rename) in `app/accept_test.go`. The M5 Changes view shows file-level diffs (`tui.TestChangesViewAndMouse`) |
| U8 | Undo the last edit, or go back to a version | **Pass** | `app/loadout_test.go` (undo/redo, versions, checkout, tag); TUI `u` / `ctrl+r` |
| U9 | Identities, rules and history survive a game path change | **Pass** | A4 `TestA4GameFolderMoved` (all instance IDs unchanged), `TestA4RebindRejected` |
| U10 | Copy a Workshop mod to a local copy for editing | **Pass** | `app/m5_test.go TestCopyLocal` (origin.json, git baseline with ignored files, no upstream `.git`, outside link skipped, Loadout and ModsConfig unchanged, same lineage, no overwrite, no `..` in `--name`); A6 confirmed membership; MCP `copy_local` |
| U11 | One-time import of RimCrow aliases, groups and user rules | **Pass** | `TestImportRimCrow`: path-first mapping, packageId fallback, ambiguous and missing reported, conflicts kept, idempotent, originals byte-identical, temp copy removed. Real dry run: 1105 of 1160 records mapped (95.3 %; the other 55 are not installed), 125 groups → 1128 tags, 5 user rules (switched off in RimCrow, so skipped by default), 64 about-disabled Workshop mods listed with guidance |
| U12 | Agent: query facts about a mod, Def, Patch, Harmony target, and who conflicts with whom | **Pass** | I1–I3, I5 in `surface/mcp/mcp_test.go`; 16 tools with annotations |
| U13 | Agent: given a stack, get the mod that owns each Harmony patch and the next step | **Pass** | B3 (three patchers, no source), `TestParsePastedStack`, I4 transcript §2 (God Hands + Yayo's Combat as patchers, then a source search) |
| U14 | Agent: decompile one mod on demand and search it | **Pass** | `TestSourceCache`, `TestSourceSearchScope`, `TestSourceWithoutILSpy`. Real: `read_source` of a nested patch class (0.7 s cold), search of three assemblies (1.7 s) |

### Acceptance cases from the specs

| Spec | Cases | Result | Where |
|---|---|---|---|
| 02 §9 | A1–A11 | Pass | `internal/app/accept_test.go` |
| 03 §6.4 | B1–B5 | Pass | golden `harmonylog`, `internal/app/m4_test.go` |
| 03 §7 | C1–C6 | Pass | `internal/app/problems_test.go`, `internal/analyze/analyze_test.go`, golden `harmonylog` |
| 04 §4.3 | S1–S5 | Pass | `internal/order/order_test.go`, golden `loadout` |
| 05 §6 | I1–I3, I5 | Pass | `internal/surface/mcp/mcp_test.go` |
| 05 §6 | I4 | Pass (self-check) | `docs/m4-mcp-transcripts.md`, a scripted client with no skill. Not yet tried in a real Claude Code / Codex session by the user |
| 06 §8 | T1 | Pass | §2 below |
| 06 §8 | T2, T3 | Pass | `TestSmallTerminalAndCJK` (80×24 hides the detail pane; CJK rows have equal display width) |
| 06 §8 | T4 | **Not verified** | A usability test with a new user (enable, sort, save, launch in 5 minutes without docs) needs a person |
| 06 §8 | T5 | Pass | §2 below |

### 01 §5.1: inspector replay on the real collection (≥ 300 mods)

| Diagnosis | RimArcho result | Compared with the inspector |
|---|---|---|
| XML patch fails (`[Node Research] Patch operation … failed`) | `log:E15` traces to `Progression production patch.xml:40` with reason `target-inactive`. The provider is VFE Tribals, which is installed but disabled. Confidence certain | The inspector's workflow ends at "find the patch file". RimArcho also names the reason and the missing provider: **more precise** |
| Missing cross-reference (`No Verse.SoundDef named Reload_Standard…`) | `log:E21`: the referrer is found at file:line, and the report says that no installed mod provides the def | The inspector would run an rg search; RimArcho does the search and the provider check: **same conclusion, fewer steps** |
| `Pawn.Tick` stack with Harmony lines | Every mod on a patch line is a `patcher`, never `source`. The topmost mod frame becomes `source`, the AllComps checklist is shown, and the source search covers only the cluster's candidates | Matches the inspector skill's "patcher ≠ cause" rule and its hot-spot checklist |

The inspector's own unit tests were replayed (B5: selector vs active state, Languages excluded from
rg, decompile cache surviving a cleaned cache dir, copy-local git baseline). **The old inspector itself was
not re-run on the real collection**, because it writes its cache and database under the RimCrow folder. The
comparison above is against its documented workflow and its tests.

## 2. Performance and resources (01 §5.2)

Measured on 2026-10-19 on the real environment. A fresh temporary data directory was used where "first"
matters.

| Metric | Target | Measured | Result |
|---|---|---|---|
| TUI idle memory (T5, working set) | ≤ 80 MB (≤ 65 MB per M3 ruling 5) | **47.3 MB** after 10 min idle (peak 57.1 MB, private 84.8 MB) | Pass |
| Cold start to interactive list | ≤ 2 s | 0.35 s for the first run after the scan, then 0.10 s (process start to first frame, 1354 rows); 50 ms in-process | Pass |
| First full scan | ≤ 60 s | 2.3 s with a warm OS file cache (M1 cold-cache measurement: 12.5 s) | Pass |
| Incremental scan, no change | ≤ 3 s | 0.61 s, peak working set 109 MB | Pass |
| Full Def + Patch (+ Harmony) analysis | ≤ 30 s, peak ≤ 400 MB, child process | 3.9 s wall (482 mods, 35 254 defs, 5433 operations, 2524 Harmony patches), peak working set 313 MB, child process | Pass |
| MCP single query (index built) | ≤ 500 ms | ≤ 3 ms for status, find_mods, get_mod, get_def, get_patches, get_harmony, list_problems, list_changes and analyze_log (median of 5). search_files takes ~310 ms. **First call of a session: 49–57 ms** (was 553 ms): the attributed log is read from the database. Only the first list_problems / analyze_log after the Player.log changes attributes it again: 568 ms, once, then stored | Pass. One exception: the first log call after a new Player.log (+68 ms) |
| T1: keypress response, 500+ mod list | ≤ 50 ms | Update + render: median < 0.1 ms, p95 0.5 ms, max 1.6 ms over 340 keypresses (scrolling, paging, typing a search) on 1354 rows | Pass |
| Sort, 500 mods (S5) | ≤ 1 s | 22 ms in-process (M2); 0.5 ms algorithm only | Pass |

T5 run (`spikes/tuimem`, real data directory): scan through the child process, browsing, the Problems view
with the child analysis, Inspect, `l` (log analysed in the TUI process), then 10 minutes idle.

| Phase | Working set | Private | Peak |
|---|---|---|---|
| list shown | 37.8 MB | 75.2 MB | 51.2 MB |
| after Problems + Inspect | 40.8 MB | 78.0 MB | 55.4 MB |
| log analysed | 42.4 MB | 80.0 MB | 57.1 MB |
| idle 5 min | 46.7 MB | 84.2 MB | 57.1 MB |
| **idle 10 min** | **47.3 MB** | 84.8 MB | 57.1 MB |

This run had a selected log, so the list-shown figure includes parsing the log at start. In a second run with
no selected log, the TUI parsed the log only when the Problems view opened (the M5 fix). Working set was
32.3 MB with the list shown, 43.6 MB with the Problems view open (log parsed), and 45.1 MB after 1 min idle.

## 3. Safety baseline (01 §5.3)

| Rule | Evidence |
|---|---|
| Nothing in the Workshop folder is modified | No code path writes under a source root except `copy-local`, which writes a new folder under `Mods/`, and `save`, which writes ModsConfig.xml. The about-disabled guidance tells the player to rename files; RimArcho never does it. All write tests use synthetic environments |
| ModsConfig.xml is backed up and written atomically | `loadout.TestWriteFileBackupAtomic`; `.rimarcho.bak` next to the file |
| No path traversal on copy; no following links outside the mod folder | `TestCopyLocal` (outside junction skipped, `--name ..\up` rejected, loops skipped) |
| No network port; MCP over stdio only | `archo mcp` uses `StdioTransport` only; no listener exists in the code |
| The real files were not touched during M1–M5 | ModsConfig.xml md5 `680ab615e9d194e11df6ca3cc28ea3d2` unchanged; RimCrow DB md5 unchanged across the dry run |

## 4. Not met or not verified

1. **T4 usability test** (a new user enables, sorts, saves and launches in 5 minutes without docs): not done. It
   needs a person.
2. **I4 with a real agent client**: verified with a scripted MCP client only. One session with Claude Code or
   Codex using the README config is still to do.
3. **Inspector side-by-side on the real collection**: the inspector was not run (it writes its cache under the
   RimCrow folder). The comparison is against its tests and documented workflow.
4. **First log call after a new Player.log**: 568 ms against the 500 ms target, once per new log (not per
   session). Most of it is the cross-reference referrer search (≈ 455 ms reading every enabled mod's Defs
   and Patches), not parsing (≈ 60 ms). The report is then stored, and status, get_mod and the other
   non-log tools never parse the log. The first call of a session now takes 49–57 ms.
5. **First full scan with a cold OS cache** was not re-measured in M5 (M1: 12.5 s; target 60 s).
6. **Packaging**: the release script builds an unsigned exe. There is no code signing and no Windows
   version resource (only `archo version` / `--version`).
