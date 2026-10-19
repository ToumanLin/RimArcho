# M3 decisions and measurements

Commits 5edf5a2..ce13e7f, local only. Scope: 03 §1–§5.2 and §7 (declaration, Def and Patch layers), the TUI
Problems view and Inspect, and the three M2 rulings (TUI memory, cycle severity, localisation). Not in M3:
Harmony, logs, MCP, ilspycmd.

## 1. TUI memory headroom (ruling 7)

Changes:
- `store.Mods` no longer selects `decl_blob`. `Mod.Decl()` loads it lazily, and `Store.LoadDecls` loads it in batches for
  sort and analysis.
- `app.Index()`, `app.Loadout()` and `app.Problems()` are cached. The cache key is `Store.ChangeKey()` (`PRAGMA data_version`
  + `total_changes()`) plus the environment id, plus the ModsConfig.xml size and mtime. Repeated TUI refreshes and the detail
  pane (which now shows the mod's problems) reuse them.
- `tui.TuneProcess`: a `debug.SetMemoryLimit(40 MiB)` soft cap, GOGC 50, and `PRAGMA cache_size=-256` on the TUI's single
  connection. The default 2 MB page cache stays resident in modernc's allocator.
- `spikes/tuimem`: drives the real TUI model headlessly. It scans through the archo binary and browses, then opens Problems
  (with the analysis child) and Inspect, and then idles.

Working set (WS) is what the M2 measurement and Task Manager report. Private bytes include about 45 MB that modernc's libc
commits at start, which is mostly not resident.

| real environment, 485 entries | WS idle | private idle |
|---|---|---|
| M2 end (teatest harness, list only) | 78.7 MB | — |
| M3 start (lazy decl, caches, soft cap) | 62–68 MB | 63–69 MB |
| **M3 end: scan + browsing + Problems view (child analysis) + Inspect, 2 min idle** | **30.8 MB** | **66.4 MB** |

Verdict: the working set has 49 MB of headroom below the 80 MB target. Private bytes have 13.6 MB, just short of the
15 MB target. The remaining overhead is the fixed libc arena (49 MB private right after `app.Open`, before any query).

**cgo SQLite option (written up, not taken).**
- Expected gain: `mattn/go-sqlite3` drops the transpiled-libc arena, so private bytes would be about 25–35 MB.
- Cost:
  - A C toolchain (mingw-w64) for every build, CI and release.
  - No trivial cross-compilation.
  - It contradicts 07 §1 ("pure Go, single exe").
- Not switching because:
  - The user-visible number (working set) is 31 MB.
  - No metric exceeds 80 MB.
  - The overhead does not grow over time.
- Revisit if M4 (log parsing in the TUI process) pushes private bytes above 75 MB.

## 2. `order.cycle` (ruling 2)

- `order.cycle` is a warning and never blocks save or launch. The sort output, the Problems view and `problems` show it.
- `order.Cycle.Suggest` ranks constraint kinds in this order:
  1. community rule;
  2. About.xml loadAfter/loadBefore (including force*);
  3. dependency;
  4. built-in official order.
- Within a rank, the edge that conflicts with the current order wins (From currently after To); after that, the first edge.
- User rules are never suggested: the fix for a user rule is `rule remove`.
- One key in the TUI: in Problems, select the cycle and press `a`. That runs its first suggestion, `archo rule add ignore
  <ref>`, after a y/n panel.

## 3. Localisation (ruling 3)

- TUI strings are looked up by their English text (`T`/`Tf`, `internal/surface/tui/i18n.go`). CLI, MCP and the command output
  shown in panels stay fixed English (05 §1). Row flags are translated at render time; the internal values stay English.
- Language selection: `ARCHO_LANG`, then the setting `ui.lang` (`auto|en|zh`), then the OS UI language
  (`GetUserPreferredUILanguages`; `LANG` on other OSes).
- New command `config get [<key>]` / `config set <key> <value>`. It is not in 05 §3, but 06 §1 needs somewhere to keep "跟随配置".
  Values are stored as meta `cfg:<key>`.

## 4. Analysis architecture (03 §3)

- **Packages:**
  - `internal/xdoc`: arena XML store and `xpath.NodeNavigator`.
  - `internal/content`: effective content.
  - `internal/analyze`: Def facts, Patch simulation, Def/Patch findings.
  - `internal/finding`: the shared Finding model.
  - `app/analysis.go` and `app/problems.go`: inputs, persistence, freshness, and the declaration layer.
- **`archo analyze`** is the child process. TUI, CLI and the commands that need content facts call `App.EnsureAnalysis`, which
  starts `<exe> analyze --env <name>` (`RIMARCHO_HOME` passed on) when the stored run is stale. Progress lines
  (`progress: …`) go to stderr; the TUI streams them into the status bar. Tests inject an in-process runner.
- **Freshness:** `AnalysisKey` hashes the analyzer version, the game major.minor, and every included instance's id, snapshot
  hash and position. User and community rules are not in the key, because they only affect the declaration layer, which is
  always computed live. The key is cheap: no declarations are loaded.
- **Included instances:** the enabled list plus every live, loadable, inactive instance. Inactive ones are indexed for Def
  facts only, as the definers behind `target-inactive`.
- **Arena store:**
  - Nodes are 44-byte structs in 64K-node chunks; the first chunk grows like a slice so small documents stay small.
  - Links are int32; names are interned in a `Names` table shared by the merged document and the patch documents.
  - Text lives in 1 MiB append-only pages. Texts of 64 KiB or more get their own allocation.
  - The first attempt used one growing text buffer: aliased strings (index keys) kept every old backing array alive, about
    226 MB live.
- **Parse pipeline:** at most 8 workers tokenise files with `encoding/xml` into events, with at most 2× workers files in
  memory. Documents are built sequentially in load order.
- **Child process limits:** `debug.SetMemoryLimit(300 MiB)`. Without it, peak WS is 435 MB; with it, 310 MB, costing +0.1 s.
- **Stored per environment:** the latest run only (`analysis`, `patch_fact`, `finding`). Def facts and the effective-content
  state are stored per instance (`def_fact`, `content_state`, keyed by content key) and survive across runs. Schema v3, plus
  v4 for the def `MayRequire` columns. Older runs are deleted on save. `env remove` and `gc` clean up.

## 5. Effective content (03 §1)

- **Version tag:** the exact version; else the highest tag not above it (numeric comparison); else `<default>`; else the
  lowest tag. The spike used string comparison and `default`; the spec says lowest. I combined both.
- **Without LoadFolders.xml:** the version folder (exact; else highest ≤ V; else lowest), then `Common`, then the root.
  Priority is in that order; assets are de-duplicated by relative path (as in the spike).
- **Conditions:** `IfDLCActive`/`IfDLCNotActive` are treated as `IfModActive`/`IfModNotActive`. The packageIds lose a
  `_steam` suffix.
- **Cache key:** `snapshot hash | mm | sorted condition outcomes | analyzer version`. A Loadout change only invalidates mods
  whose conditions flip (tested: `TestLoadFoldersConditions`).
- **Conditional folders:** `Resolved.Requires` maps each loaded conditional folder to the packages its condition names. A
  patch inside such a folder counts as guarded, and those packages are its intended targets (see §7).

## 6. Def facts (03 §2, §5.1)

- Fields stored: Type (element name), defName, Name, ParentName, Abstract, file (relative to the mod root), and line (of the
  start tag). Also stored: the def's own `MayRequire`/`MayRequireAnyOf`; the game skips such defs unless those mods are active.
- A def's line is its start tag. `archo def` prints `file:line` in load order and marks the last definer `← effective`.

## 7. Findings: judgment calls

**Declaration layer** (computed live; cached per database change):
- Codes added to 03 §4.3:
  - `entry.missing` (warning): a Loadout entry that matches no installed mod.
  - `mod.unloadable`: error when an unloadable / residue / duplicate-rejected instance is enabled; info for an installed
    about-disabled mod (per the M3 brief).
- `replacement.available` is not produced: there is no replacement data source until `data update` (M5).
- `dep.missing` suggestions:
  - `subscribe: <steamWorkshopUrl>` and `steam://url/CommunityFilePage/<id>`. These are not archo commands; the TUI shows
    them in a panel. (C1.)
  - A missing DLC gets its own claim text.
- `order.violated`: one finding per violated sort edge outside a cycle, with an ignore suggestion. Violations of the built-in
  official order are folded into one `official.order` error.
- `pkg.duplicate-active` (C2): the subjects are every enabled copy; the suggestion is to disable all but the first.
  `pkg.duplicate-installed`: info, with a `swap` suggestion.

**Def layer:**
- `def.override` (C3): skips abstract defs, duplicates within one mod, and defs whose MayRequire is not met.
- `def.abstract-collision`: the same `Name` from two or more mods, as a warning.
- `def.parent-missing` is checked **after the Patch simulation**, against Name attributes in the patched document.
  - This deviates from 03 §5.1 ("先收集全部 Name"). On the real environment, all 12 pre-patch hits were false positives: mods add
    `Name` to Core defs with AttributeAdd before inheritance is resolved, and Player.log has no parent errors.
  - Confidence: likely.

**Patch layer:**
- `patch.no-match` is one finding per failed top-level operation (what the game logs). The key is file + game description,
  so it matches Player.log. Reasons (03 §4.4), evaluated on the innermost failed operation:
  - `target-later`: the XPath matches once all patches have run. The creating mod is named from node allocation ranges.
    Defs of every mod are merged before any patch runs, so "defined by a later mod" cannot cause a no-match. I reinterpreted
    the reason as "created by a later patch".
  - `node-absent` (added): the def exists, but the rest of the path does not.
  - `target-inactive` / `target-missing`: from Def facts of all installed mods.
  - `wrong-version-route`: searches the non-effective XML of the patching mod and its intended targets.
  - `stale-name`: the intended target mod is active but has no such def. Up to three similar names (edit distance) from that
    mod are listed; on the real data it finds the M0 rename `sbzSmallDisplayShelfBase → sbzDisplayShelfBase`.
  - `unguarded-optional`: only together with `target-inactive`, when no guard applies and the definer is not a hard dependency.
  - `exception` (added): the game would throw, e.g. a tree operation on an attribute node.
  - `unclassified` (added): no def name in the XPath.
- Guards: FindMod (match branch), Conditional, a Sequence item with MayRequire, or a conditional load folder.
  - A top-level `MayRequire` is ignored, as the game does, and the detail says so.
  - Intended targets: the FindMod names (display names; DLCs use their folder name), MayRequire packages, conditional-folder
    packages, and hard dependencies other than Harmony and Ludeon.
- `patch.contested`: Replace/Remove from two or more mods on the same node. The node key is the def plus the element path.
- `patch.unknown-op`: one finding per mod, listing its custom classes. Those operations count as succeeding.

**Refs and ignores:**
- A problem ref is `p:<6 hex>` = sha1(code | key | subject lineage). It is stable across runs.
- Ignores are keyed by (lineage, code, key) and record the subject's snapshot. When the snapshot changes, the problem shows
  again with `[ignore expired: the mod changed]` (03 §4.2).
- Commands: `problems ignore|unignore <p:ref>`, `problems --ignored`; the TUI keys are `i` / `I`.

## 8. CLI (05 §3)

- **New commands:**
  - `analyze`;
  - `problems [--subject] [--code <code|prefix>] [--severity] [--all] [--detail] [--ignored] [--no-analyze]`;
  - `problems show|ignore|unignore <p:ref>`;
  - `def <Type/defName | Type@Name | defName>`, `def --mod <ref>`;
  - `patches [--mod] [--target <Type/defName | xpath part>] [--failed] [--all]`;
  - `why <A> <B>`;
  - `files search <pattern> [--mod] [--regex] [--all]`;
  - `config get|set`.
- `mod` gains `content:` and `problems:` lines (05 §2.2). `status` gains a `problems:` line.
- `problems`, `def` and `patches` rebuild a stale analysis through the child process. `mod`, `status` and `why` use what is
  stored, so they stay fast.
- The new golden scenario `testdata/golden/problems.txt` covers every new command.

## 9. TUI (06 §4, §6)

**Problems view:**
- Grouped by severity, then by subject mod (ref, name, position). Rows: `▸ code claim`.
- Keys:
  - `enter`/space expands evidence and the `[a] [b] …` suggestions; `enter` on an expanded problem opens it in Inspect.
  - `tab` picks a suggestion; `a` runs it. Read-only commands open in Inspect, `sort` shows its preview, anything else asks y/n.
  - `i` ignores (unignores in the ignored list); `I` shows the ignored list.
  - `s` cycles severity; `o` inspects the subject mod; `A` re-analyses.
- Opening the view starts the child analysis when it is stale.
- The status bar shows `problems e/w/i` (red with errors, `*` while stale).

**Inspect:**
- A line cursor. `enter` follows the first ref on the line: `#id`, `L#id`, `def:…` or `p:…`. Backspace walks back the history.
- Mod sub-tabs (`tab`/`shift+tab`): summary, Defs, Patches, problems, rules, history. A Harmony tab comes with M4.
- All Inspect text is the CLI command output.

## 10. Real-environment measurements (482 active mods, 1286 installed, game 1.7)

| item | result | target |
|---|---|---|
| full Def+Patch analysis, `archo analyze` wall time | 3.3–4.0 s first run (804 inactive mods parsed); 2.7 s with cache | ≤ 30 s |
| analysis child peak working set | **310 MB** (private 340 MB; 300 MiB soft cap) | ≤ 400 MB |
| without the soft cap | 435 MB peak | — |
| merged document | 2.68 M nodes, 35 MB text, 35 254 top-level defs, 5433 operations, 7044 XPath evaluations (6729 via the index fast path, 66 rebuilds) | — |
| Player.log replay (log from 1.6, analysed as 1.6) | 7/7 log failures simulated; 2 sim-only, both the post-log sbz rename; mismatch 0.037% | < 0.5% |
| TUI idle at end of M3 | WS 30.8 MB, private 66.4 MB | ≤ 80 MB with ≥ 15 MB headroom |
| real ModsConfig.xml | md5 680ab615… unchanged; no launch | — |

Problems on the real environment: 1 error, 322 warnings, 339 info.
- The error is `official.order` (RimCrow Companion before a DLC).
- 316 of the warnings are `version.unsupported`: the game is 1.7, and most mods list only up to 1.6.

## 11. Open questions

1. `version.unsupported` floods the list right after a game update (316 warnings). Should these be grouped into one summary
   problem per game version, or downgraded to info when the mod lists the previous minor version?
2. `def.override` (155) and `patch.contested` (104) are noisy info. Should they be hidden by default in the TUI, behind the
   `s` filter?
3. ParentName resolution is now post-patch, which deviates from 03 §5.1. Please confirm and update the spec text.
4. `target-later` was reinterpreted as "created by a later patch" (§7). Confirm, or define what the error-debugging guide meant.
5. TUI private bytes have 13.6 MB of headroom (§1). Accept working set as the T5 metric, or plan the cgo switch?
6. `config` is a new command (§3). Keep it, or fold the setting into `env set` / config.toml?
7. Analysis runs against the game's major.minor (1.7), while the user's ModsConfig.xml and Player.log are from 1.6.
   `RIMARCHO_LOG_VERSION` exists only for the replay test. Should `analyze --version` be exposed?

## 12. Main-session rulings (2026-10-19, after M3 review)

| # | Ruling | Where |
|---|---|---|
| 1 | `version.unsupported` severity becomes info when the mod lists the previous minor version (game 1.7, mod lists 1.6), warning otherwise. TUI and default CLI/MCP output fold these into one summary row per game version, which can be expanded. | 03 §4.3; implement in M4 |
| 2 | `def.override` and `patch.contested` are hidden by default in the TUI Problems view and in default `problems` output; they show under the `s` filter / `--all`, and always in Inspect and in `def` / `patches` output. MCP `list_problems` defaults to warning+ and takes a severity parameter. | 06 §4, 05 §3; M4 |
| 3 | ParentName resolution after patching is correct (the game resolves inheritance after applying patches). Spec text corrected. | 03 §5.1 |
| 4 | The `target-later` reinterpretation is correct (all defs are merged before patches run). `node-absent` is added to the spec. | 03 §4.4 |
| 5 | Working set is the T5 metric. Private bytes are recorded but not gated. Do not switch to cgo SQLite unless working set exceeds 65 MB. | 06 §8 |
| 6 | Keep `config`, and add it to 05 §3. | M4 |
| 7 | Expose `--game-version X.Y` on analysis commands (`analyze`, `problems`, `def`, `patches`, `why`, `log`) to override the environment version; it is needed for exactly this user's 1.6-list-on-1.7-game situation. Also make it an MCP tool parameter. | 05 §3; M4 |
