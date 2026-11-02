# Community log corpus research · top-level design (draft v1)

> Status: draft; open questions in §10. v1 adds G5 (error pattern library for agent repair workflows). v2 records decisions, processing methods, and cleaning requirements (§10).
> Scope: use publicly shared HugsLib logs to test and improve RimArcho analysis and supply data-driven error patterns for agent diagnosis and repair over MCP. This document answers only "what are the goals, and what must we collect?", not implementation.

## 1. Why do this

RimArcho's judgments have so far been validated on only one real environment (the author's 485 Mods; see decisions-m2/m3):

- Whether sorting constraints are reasonable and cycles common;
- Whether finding catalog (03 §4.3) severities are appropriate;
- How many real errors log attribution (03 §6) covers.

One environment is too small a sample. HugsLib's "share logs" feature has accumulated extensive real-player data, each containing both **load order** and **run results**, filling exactly this gap.

Meanwhile, the agent diagnostic workflow (Diagnosis workflow in MCP `Instructions`, and the hotspot checklist attached to `analyze_log`) is handwritten from the old inspector skill's experience. It covers only a few error kinds (XML patches, cross-references, materials, `Pawn.Tick`). No data currently supports the distribution of real errors, common causes per kind, or effective fixes. The corpus can make this data-driven.

This is **offline research**, not a product feature: RimArcho itself collects no data online, consistent with "only `data update` accesses the network" (04 §6.3).

## 2. Data source: what a HugsLib log contains

According to `PrepareLogData()` in HugsLib `Source/Logs/LogPublisher.cs`, uploads contain, in order:

| Section | Content | Notes |
| --- | --- | --- |
| Timestamp | Upload time | |
| `Loaded mods:` | In **load order**: name, packageId, version (override / Manifest version), every assembly and version | Core data for sorting research |
| `Active Harmony patches:` | All runtime-patched methods and each patch's source; Harmony version | Ground truth for static Harmony analysis |
| `Platform information:` | CPU, OS, memory, hidden by default | Not needed; discard |
| Player.log | Sanitized: RimWorld paths, user directories, Steam IDs, connection information, GPU information, and useless lines removed | Truncated to 10,000 lines by default |

Upload method: by default creates a **public gist** through HugsLib's built-in account; players supplying their own token create private gists, which we cannot obtain.

Data characteristics (affecting interpretation):

- **Self-selected sample**: players share voluntarily, usually after a problem. Problem logs are far more common than in actual play.
- **HugsLib users only**: players without HugsLib are absent.
- **No causal labels**: we know "these Mods, this order, these errors", not the true cause.
- **Possible truncation**: logs exceeding 10,000 lines include only the beginning.
- **Mixed versions**: from 1.0 to the current version.
- **Duplicates**: one player may upload repeatedly with nearly identical Mod lists.

## 3. Goals

In priority order. Each goal states the question, the part of RimArcho to improve, and the metric.

### G1 Log attribution: coverage and regression corpus (primary)

- **Questions**: how many real errors can be attributed to specific Mods? Which kinds fail attribution? Are new error formats unrecognized?
- **Improve**: parsing, classification, and attribution evidence in 03 §6.
- **Metrics**:
  - Error-cluster classification coverage (fraction other than `other`);
  - Attribution rate (fraction with at least one `source` role);
  - Accuracy on the labeled subset (§6.4).
- **By-product**: a sanitized set of regression-test logs.

### G2 Sorting: research on real load orders (primary)

- **Questions**:
  - How often do players' actual orders violate declared constraints (loadBefore/After, dependencies)? Do violations coincide with errors?
  - How many Mods would RimArcho's sorter move on real lists? Are results stable?
  - Which Mod pairs often co-occur with `patch-failed` errors? Can these yield candidate community rules or inferred constraints?
- **Improve**: the sorter in 04 §5, plus `inferred` constraints and tiers discussed previously.
- **Metrics**: violation rate, number of moves, count of constraint pairs co-occurring with errors.

### G3 Static Harmony analysis recall (secondary)

- **Questions**: how many runtime-active Harmony patches does static scanning find? Are misses concentrated in manual `Harmony.Patch` (`harmony.manual`)?
- **Improve**: `HarmonyPatch` facts in 03 §2 and 1.x "IL scan for manual Patch targets" in 01 §1.
- **Metrics**: recall by method and Mod.
- **Prerequisite**: corresponding-version DLLs locally (§5 L2).

### G4 Finding catalog severity calibration (secondary)

- **Questions**: when static findings (`def.override`, `harmony.skip-prefix`, `patch.no-match`, etc.) appear, how often do corresponding errors actually occur in logs?
- **Improve**: severity in 03 §4.3 and `confidence` values.
- **Metrics**: fraction with a corresponding log error for each finding code.
- **Prerequisite**: corresponding Mod files locally (§5 L2).

### G5 Error pattern library: agent diagnosis and repair (secondary)

- **Questions**:
  - What common patterns occur in real errors? What fraction is each? How much does the existing workflow cover?
  - What usually causes each pattern? Which diagnostic route applies (which MCP tools first)?
  - Which fixes work: disabling a Mod, reordering, adding dependencies, updating Mods, repairing patches in a local copy?
- **Improve**: MCP `Instructions` diagnostic workflow, `analyze_log` checklist (03 §6.3), `suggestions` content.
- **Metrics**:
  - Pattern coverage: fraction of error clusters matching a known pattern (weighted by occurrence and by log separately);
  - Whether each pattern's "known fixes" have evidence (`Resolution` in §5 L1);
  - Agent evaluation: fraction of conclusions following the workflow that agree with labels (§6.5).
- **Two pattern tiers**:
  - **Generic patterns**: independent of specific Mods, durable. Examples: `Could not resolve cross-reference`, `MissingMethodException` (assembly mismatch with dependencies or game version), `TypeLoadException`, patch failures, `Exception ticking`, missing materials, duplicate XML definitions. Output: "matching rules + diagnostic route + common causes + fix kinds".
  - **Mod-specific patterns**: known issues in a particular Mod version, e.g. "A version 1.2 raises an error when enabled with B". Short-lived; must include Mod version and discovery time, automatically downgraded to "possibly fixed" after a Mod update.

### Non-goals

- No model training or "automatic diagnostic AI". Patterns are data and clues for agents; reasoning remains with the agent in the harness (D6). RimArcho has no built-in LLM.
- Pattern matching is not a conclusion. Matches are one piece of evidence, with source type `community` or `inferred`, still following 03 §4.2: "do not treat occurrence location as cause".
- No public database or republication of logs.
- No telemetry or automatic uploads in RimArcho.
- No causal claims. All conclusions describe "correlation", consistent with 03 §4.2.

## 4. Research questions and required data

| Required data | G1 attribution | G2 sorting | G3 Harmony | G4 calibration | G5 patterns |
| --- | :-: | :-: | :-: | :-: | :-: |
| Game version | ✓ | ✓ | ✓ | ✓ | ✓ |
| Mod list (order, packageId) | ✓ | ✓ | ✓ | ✓ | ✓ |
| Mod and assembly versions | | ✓ | ✓ | ✓ | ✓ (Mod-specific patterns) |
| Runtime Harmony patch list | ✓ | | ✓ | ✓ | ✓ |
| Log error clusters (with stack shapes) | ✓ | ✓ | | ✓ | ✓ |
| Consecutive uploads by one player | | | | | ✓ (fix evidence) |
| Resolution process in discussion threads | | | | | ✓ (fix evidence) |
| Local Mod files (About.xml) | Partial | ✓ | | ✓ | Partial |
| Local Mod files (Defs, Patches, DLL) | Partial | Partial | ✓ | ✓ | Partial |
| Manually labeled true causes | ✓ (accuracy) | | | | ✓ (evaluation dataset) |

Conclusion: **G1, G2, and G5 pattern discovery need only logs**, so can come first; G3, G4, and G5 fix validation depend on local Mod files or additional fix evidence and come later.

Logs themselves **contain no fixes**, only "what went wrong". Fix evidence can come from three sources, from least to most reliable:

1. **Consecutive uploads by one player**: if the Mod list changes and an error cluster disappears, record weak evidence that "this change may have fixed this error".
2. **Discussion threads**: replies in threads collected through §7 route B often contain fixes confirmed by the original poster.
3. **Manual labeling**: label both cause and fix in the §6.4 subset.

## 5. What to collect: four data layers

### L0 Raw text (local only)

- Preserve gist content unchanged for reparsing after parser upgrades.
- Key: gist ID; attach a content hash for deduplication.
- **Store only in the local data directory, never in the repository or shared externally.**

### L1 Structured records (local only)

Parse each log into a record:

| Entity | Fields |
| --- | --- |
| `LogRecord` | gist ID, upload time, full and major/minor game version, HugsLib version, Harmony version, truncation flag, total lines, Mod-list hash |
| `ModEntry` | Position, name, packageId, Mod version, assembly list (names, versions) |
| `HarmonyEntry` | Target method, patch kind, patch source (Harmony ID / Mod) |
| `ErrorCluster` | Normalized key (same as 03 §6.1), category, count, first line number, one sanitized sample, type frames in stack, Harmony patch lines in stack |
| `StackShape` | For G5: exception type, innermost game method (e.g. `Verse.Pawn.Tick`), assembly owning innermost Mod frame, fingerprint of the frame sequence with Mod parts removed. Groups similar errors from different Mods into one generic pattern |
| `Attribution` | RimArcho attribution for each cluster, evidence used, attribution engine version |
| `Session` | Consecutive uploads by one player (§6.2). Record `ListDiff` between neighbors: added, removed, moved Mods, and version changes |
| `Resolution` | Fix evidence: normalized cluster key, fix kind (disable / enable dependency / reorder / update / other), involved Mods, evidence source (session / thread / label), confidence |
| `Pattern` | G5 output: matching rules (normalized-key patterns and `StackShape` conditions), tier (generic / Mod-specific), occurrence and log counts, diagnostic route, common causes, fixes and `Resolution` evidence counts; Mod-specific patterns also record version range and discovery time |

`ErrorCluster` directly reuses `internal/logs` splitting and normalization so research and product share rules.

For `Resolution` from `Session`, exclude cases with too many simultaneous changes: if dozens of Mods changed, we cannot tell which change mattered. Determine rules in P1 from actual data.

### L2 Association with local Mod files (local only)

Logs contain Mod lists, not files. Declaration, Def, Patch, and Harmony analysis requires mapping lists to local files:

1. Count packageId frequency across logs. Expect a long tail: a few hundred Mods cover most entries.
2. Combine existing local Mods (author's environment) and high-frequency Mods downloaded through SteamCMD into a **research Mod library**.
3. Record **coverage** per log: how many listed Mods map to local files, and whether versions match.
4. Run G3 and G4 only on sufficiently covered logs, stating the threshold in conclusions.

Engineering prerequisite: RimArcho analysis currently uses "environment + Instance". Research needs a **virtual environment**: input a Mod list, resolve Instances from the research library, then analyze as a real Loadout. This is the only new core capability required for G3 and G4.

### L3 Aggregate results (may enter the repository)

- Statistics and conclusions in `spikes/<name>/RESULT.txt` and `docs/decisions-*.md`.
- Regression log samples require manual review and a second sanitization, in small numbers, stored separately.
- The **pattern library** (`Pattern`) can enter the repository and ship with the product. It contains only matching rules, statistics, diagnostic routes, and fixes, with no original log text. Form described in §8.

### Explicitly excluded

- Platform information such as CPU and memory (even when players make it public).
- gist comments and any account information beyond the uploading account.
- Personal information remaining after sanitization: user names, save names, local Mod folder names, etc.; sanitize again before ingestion.

## 6. Sample design

### 6.1 Filtering

- Game versions: mainly 1.6, track 1.7 separately (current version, growing sample). Do not collect earlier versions (P0 decision from version distribution, §7).
- Collect only logs with `Loaded mods:`; count and discard unparseable formats.

### 6.2 Deduplication

- Identical content: deduplicate by content hash.
- Consecutive uploads by one player: identical or nearly identical Mod-list hashes and similar error-cluster sets count as one "session". Use sessions as the statistical unit to prevent frequent uploaders dominating results.

### 6.3 Scale

| Phase | Scale | Purpose |
| --- | --- | --- |
| P0 feasibility | About 50 logs | Confirm bulk acquisition and understand formats |
| P1 initial version | About 500 logs | First G1/G2 metrics |
| P2 expansion | Thousands | Statistically stable conclusions, long-tail analysis |

### 6.4 Manually labeled subset

Attribution **accuracy** requires manual labels. Sample 50–100 P1 logs; a human identifies the true source Mod for each main error cluster and, where possible, its fix, as ground truth. This is the largest workload; confirm available time in advance.

Stratify by pattern: at least a few examples of every common generic pattern, avoiding dominance by the top one or two error kinds.

**Labeling method (2026-10-21 pilot, `spikes/corpuslabel`; figures in its `RESULT.txt`).** A small model (Haiku) labels; humans spot-check:

- **Evidence packet**: 1.5–7.5 KB of plain text per (log, cluster). Includes message, extra lines, stack and Harmony lines, nearby log lines, evidence-linked Mods (position, version, About.xml, missing dependencies, violated declared rules), and research-environment facts: `def`, `harmony --target`, `patches --failed`, `files search`, DLL assembly references, and corpus co-occurrence (Mods markedly overrepresented in other logs with the same error). Facts come from the same `cmds` commands as MCP tools, matching what agents see.
- **Why the labeling model does not call MCP directly**: the research environment can load only one log's Mod list at a time, so parallel labeling agents cannot share it. Pregenerated packets are reproducible, bounded, cheap, and individually reviewable. Agent diagnosis over MCP belongs to §6.5 evaluation, using the same labels as answers; keep the two tasks separate.
- **Labelers cannot see RimArcho attribution conclusions**, avoiding inflated agreement from anchoring.
- **Labels** are fixed-field JSON (cause Mod and role, failure phase, cause type, fix, confidence, verbatim quote, model name, prompt version), checked with `validate`: gist and cluster key must exist, packageId must occur in that log's list, quotes must match packet text exactly.
- Sampling: library coverage ≥ 80%, excluding `other` and `mod-report`; take the 2 templates occurring in most logs per problem class, 2 clusters per template, at most 1 cluster per log.

Pilot conclusion, 32 clusters: RimArcho **never named a wrong source** against reviewed labels (13/13), but named no source for 12/25 clusters with clear causes: all 4 xml-load cases missed, others spread across exception, harmony-stack, material, save-load. Thus the 21.2% coverage is mostly correct attribution; the gap is silence rather than wrong judgments (n = 32, 4 per class, preliminary only). Haiku errors were almost all "unknown", rather than blaming the wrong Mod. Across three iterations, disagreement with review fell from 44% to 31% to 22% (see RESULT). The gain came from better packets (co-occurrence, post-message lines, exact `files search`), not a larger model.

### 6.5 Agent evaluation dataset

Labeled logs with sufficiently high local Mod coverage (§5 L2) can form an agent evaluation dataset:

1. Build a virtual environment from the log's Mod list;
2. Let the agent diagnose only through MCP tools, following the scripted client in `docs/m4-mcp-transcripts.md`;
3. Compare agent conclusions (cause Mod, failure phase, fix) against labels.

Each workflow or pattern-library change can then be measured on the same evaluation dataset rather than a few manual records.

## 7. Acquisition routes (P0 validated, `spikes/hugslogs/RESULT.txt`; API probe 2026-10-19)

| Route | Description | P0 conclusion |
| --- | --- | --- |
| A. Public gists of the HugsLib account | Account `HugsLibRecordKeeper` | **GitHub API (with our own token) lists all history**: about 7,041 pages × 100 ≈ **704,000 logs**, back to 2016-11-17 (`hugslogs probe`). Download raw content from `gist.githubusercontent.com`. Web listing stops at 100 pages (about 1,000 recent logs, about 2.6 days); rapid paging triggers 429 rate limits. Fallback only without a token |
| B. gist links in Steam discussion threads | Mod bug-report threads usually request HugsLib logs | Thread pages can be scraped directly, with gist links (wrapped in `steamcommunity.com/linkfilter` redirects) and replies. Also the only source of older historical logs |

**Inference:** route A can **backfill by time range**, rather than only collect a rolling window. Volume is now the constraint: about 370–450 logs daily recently, estimated 170,000–200,000 since 1.6 released; about 600 KB each, roughly 100 GB total, too much for git. Thus **sample** by goal (§6.3); periodic incremental collection only keeps up with new logs, and missing a day loses no data.

**1.7 log format.** Current game version 1.7 changed runtime (header `=== Waylith log ===`, .NET 10); lines include timestamp, level, and category, e.g. `2026-10-19 15:17:41.976  INFO [Verse] RimWorld 1.7.4982 rev1280`. Multiline continuation lines may start at column zero; every message (including warnings) has a stack starting in logging itself (`Brutal.Logging.*`, `Verse.Log.*`). An unofficial publisher (`m00nl1ght.UnofficialUpdates.HugsLogPublisher`) uploads 1.7 logs without the `Log file contents:` separator.

**Version distribution.** Of 998 P0 logs, 936 were 1.6, only 39 were 1.7, and 16 were 1.5. Of the 39 for 1.7, 34 loaded official HugsLib, so the small sample reflects most players still using 1.6, rather than publisher issues. Scope: **mainly 1.6, track 1.7 separately**. 1.7 is current and the author's own environment; samples will grow over time.

Rules:

- Use our own GitHub token and respect rate limits.
- **Never use the token embedded in HugsLib source.**

## 8. Compliance and privacy

- Data is public but used only for research, without republication or individual profiling.
- L0–L2 stay local; only L3 enters the repository.
- Sanitize again before ingestion (§5 "Explicitly excluded").
- Conclusions must not identify players, e.g. by listing a complete rare Mod combination.
- Patterns record "how many logs", not which gists. Mod-specific patterns seen in fewer than a minimum number of logs are excluded.

## 9. How patterns enter the product (direction, not implemented in this phase)

The pattern library is the most direct product output of this research. Integration follows existing principles: RimArcho supplies facts; agents reason.

| Location | Use |
| --- | --- |
| `analyze_log` / `archo log` | On a cluster match, attach `pattern:<id>` as evidence (source type `community`), with frequency, common causes, suggested next commands |
| New referenceable entity `pattern:<id>` | As with other references, inspect further in Inspect, CLI, MCP: matching rules, diagnostic route, fixes and evidence counts |
| MCP `Instructions` | Rewrite workflow categories and checklists from actual pattern distribution, covering common patterns first |
| `suggestions` | Turn fixes into executable commands, e.g. `archo disable #a3f9`, `archo rule add after …` |
| Data distribution | As with community sorting rules, obtain through `archo data update`, recording dataset version (04 §6.3) |

## 10. Phases and open questions

### Phases

| Phase | Work | Deliverable |
| --- | --- | --- |
| P0 feasibility | Validate §7 route A; fetch about 50 logs, record actual formats of three sections; roughly inspect repeat-upload share | `spikes/hugslogs/RESULT.txt`: bulk acquisition feasibility, format notes, whether enough `Session` records exist |
| P1 corpus and G1/G2/G5 discovery | Fetch, parse, deduplicate, ingest locally; run existing attribution and sorter; cluster initial generic patterns by `StackShape`; extract `Resolution` from `Session`; finish labeled subset | G1/G2 metrics; initial pattern library and coverage; conclusions and improvements in `decisions-*.md` |
| P2 local Mod library and G3/G4/G5 evaluation | packageId frequency; download frequent Mods; implement virtual environment; compare static analysis with runtime; build agent evaluation dataset | G3/G4 metrics; agent evaluation baseline |
| P3 product integration | Integrate `analyze_log`, `pattern:` references, MCP `Instructions` per §9; compare before/after using evaluation dataset | New spec sections (03 §6, 05) |

At each phase end, record conclusions in `docs/decisions-*.md`, then decide whether to proceed.

**P0 status: complete** (2026-10-20), conclusions in `spikes/hugslogs/RESULT.txt`. Impact on P1:

- ~~Route A supports only rolling collection~~ (only web listing worked in P0). API probe 2026-10-19: all history is listable (about 704,000 logs, back to 2016-11); P1 can backfill by date. Storage volume becomes the constraint; sampling required.
- Repeat uploads by one player account for 40% of logs, a source of fix evidence.
- The Mod tail is longer than expected: top 1,000 packages cover only 57% of entries; top 3,000 cover 80%.
- Clean first: `other` contains many status lines; per-type messages need template normalization; 1.7 needs level-based distinctions.
- Thread text extraction must handle nested quotes and Mod-author identifiers before passing to a small model.

**P1 status: first G1/G2 round complete** (2026-10-20); conclusions in `docs/research/decisions-corpus.md`, figures in `spikes/corpusrun/RESULT.txt`.
- Sample: whole 15th day of each month in the 1.6 period, 6,734 logs for 1.6; research uses a separate 1.6 installation and 1,960 downloaded Mods, median per-log Mod coverage 82%.
- G1: corpus-driven parser/attribution changes raised clusters with a source from 22.5% to 28.4% on the same 671 logs, reducing Core/DLC falsely named sources from 5,875 to 0. Full sample: 21.2%; highly covered logs do no better, so remaining gaps are rules.
- G2: half of logs violate declared constraints, but above 100 Mods violation is unrelated to errors; the most violated constraints rarely involve attributed Mods.
- Still coverage only, not accuracy (open question 3).

**Second P1 round and first G3/G4/G5 round complete** (2026-10-22, final run v4, merged master, complete Mod lists):
- G1: problem clusters with a source on the full sample 21.2% → 29.9%; all sources named by RimArcho in the 32-cluster labeling pilot were correct; gaps were "not named", not "wrong".
- G2: sorter grades constraints by observed impact; load-order warnings 19,945 → 2,274.
- G3: static Harmony recall 60% (71% with patch method found); 82% of misses from assemblies directly calling `Harmony.Patch`.
- G4: `patch.no-match` is most predictive (about 8 times chance); error is too severe for `dep.missing (lib)` and `official.order`.
- G5: library v0 in `spikes/patterns/RESULT.txt`; integration draft in `docs/research/01-pattern-library-integration-draft.md`.

### Decided (v2)

| # | Question | Decision |
| --- | --- | --- |
| 1 | Primary goals | G1, G2. Keep G5 secondary; discover patterns alongside G1 in P1 |
| 2 | Download frequent Mods | Allowed. Research runs in the cloud; disk space is not a constraint |
| 4 | Corpus location | Separate **private** repository `ToumanLin/RimArcho-corpus`. RimArcho is public: placing raw logs there republishes them irreversibly; research runs in temporary cloud containers, so corpus must live outside the container |
| 5 | Discussion threads | Collect (§7 route B) |

**Processing (R1).** Text enrichment, such as determining whether a thread has a solution, extracting fixes, or initially classifying error clusters, uses parallel small-model subagents. Machine-determinable work (header parsing, cluster splitting, normalization, deduplication) always uses deterministic code, never models.

**Data cleaning (R2).** Cleaning is the first step in every phase, not an afterthought:
- Record cleaning outcomes for every raw log: accepted, discarded (with reason, e.g. old version, missing Mod list, unparseable format); aggregate discard reasons by count in RESULT.
- Model output must be structured (fixed fields), storing model name and prompt version; validate before corpus ingestion, including existence of referenced gist IDs and cluster keys.
- Manually spot-check each model-output batch; record sample size and failure rate in RESULT.

### Open questions

6. ~~Sampling and storage~~ Decided (2026-10-20): sample one whole day monthly (the 15th), keep raw logs in the corpus repository (git compresses about 8-fold; 6,093 logs pack to 0.49 GB; cap at 2 GB packed).
3. How much time is available for the labeled subset (§6.4)? This determines whether attribution **accuracy** can be measured (distinction below), and whether an agent evaluation dataset (§6.5) can be built.

> **Coverage vs. accuracy.** Coverage = fraction of all error clusters for which a source Mod is named, computable automatically. Accuracy = fraction of clusters with manually confirmed causes where RimArcho's source agrees with the human conclusion; requires labels. Coverage alone is risky: an attributor always "guessing a Mod" can reach 100% coverage while mostly wrong. Thread cases with confirmed fixes can supply some labels and reduce manual work.
