# Log corpus P1 decisions and measurements (G1, G2)

Research design: `docs/research/00-日志语料顶层设计.md`. Measurements: `spikes/corpusrun/RESULT.txt`
(this file only has the conclusions). Raw logs: the private `RimArcho-corpus` repository.

## 1. Collection

| Decision | Detail |
|---|---|
| The GitHub API, not the web listing | With a token the API lists the whole history (704k gists back to 2016-11). The web listing only covers the newest ~1,000. A listing page takes ~10 s on GitHub's side, so `hugslogs api -list-only` requests 8 pages at a time; the 1.6 period (180k gists) took ~1.5 h |
| Sample whole days | One day per month (the 15th, `hugslogs sample`). Whole days keep a player's repeat uploads together (sessions, design §6.2). 6,093 logs, 0.49 GB packed in git (8× compression), within the corpus repository's 2 GB guard |
| Research mod library | The top 3,000 packages of the sample that are not installed locally: packageId → Workshop id via RimSort's `steamDB.json` (packageId, else a unique name), SteamCMD download, check against About.xml's own packageId (a dependency's `<packageId>` comes first in many files), textures and sounds stripped. 1,960 mods, 3.2 GB |
| 1.6 game data | The public 1.6 branch installed separately with SteamCMD (`D:\RimWorld16`), so 1.6 logs are simulated against 1.6 Core and DLCs, not the author's 1.7 |
| Research environment | A separate `RIMARCHO_HOME`. The downloaded mods sit in the 1.6 game's `Mods` folder, because a Loadout only takes mods the game can load (official, local, Workshop). Link deployment (09 §8) would now also allow a SteamCMD source |

## 2. Product changes made from the corpus

| Commit | Change | Effect |
|---|---|---|
| 818b348 | 1.7 Waylith log read natively; texture-missing, save-load, mod-report classes; engine status lines dropped; cluster templates | `other` 77,795 → 15,280 clusters on the P0 window |
| d4e074e | Schema v10: (instance_id, game_mm) indexes on the analysis fact tables | Analysis 74–133 s → 3.5–13 s on 3,500 instances |
| 621ce34 | Mono native frames continue the stack; XML Extensions error blocks are one entry; wanter rule for xref-missing and no Core/DLC text-match sources; paths matched by mod folder; texture "for def", xml Context defName, missing types, static constructors | Same 671 logs: with a source 22.5% → 26.1%; Core/DLC wrongly blamed 5,875 → 0 |
| da9d181 | Missing parent nodes (disabled provider = dependency), duplicate ThinkNode keys (think-tree patchers) | xml-load with a source 3.3% → 12.8% |
| (branch claude/g2-sorter-5f1419) | Sorter reports by constraint strength (04 §4.4). Per kind, violated vs kept pairs (same pair, same list-size band), observed/expected error sources: dependency 1.41, loadBefore 1.34 → warning; loadAfter 1.14, community 1.13 → info; a mod declaring it loads before Core/DLC 1.05 → info (harmless); a mod needing a DLC that loads after it 3–4.5 → part of `official.order`. 74 rules observed harmless and 7 harmful ship in `internal/order/observed.json` (aggregate counts; embedded: `data update` only fetches third-party datasets). A rule putting a mod before official content that other constraints keep after it, or a rule between DLCs against the official order, is not used, with the reason, instead of a cycle. `official.order` suggests the cheapest `archo move` (DLC back after its predecessor, or the few mods ahead of it down) | Sort-only rerun of 6,734 logs (`spikes/g2sev`): `order.violated` warnings 19,945 → 2,274 (16,677 info, 8,147 of them observed harmless); logs with such a warning 65.4% → 18.8%; 35 constraints not used (35 logs), logs with a cycle 746 → 716; `official.order` in 1,533 logs: a move suggested in 1,381 (DLC back 571, mods down 810; one move in 1,152), sort only 152 |
| d268a93, 0ad44ae, a7117bd | Rules for the RESULT gaps (below): missing textures and `GetNamed` names looked up in the enabled mods' XML and DLL string literals; a mod that defines and uses the missing def; reflection-only resolvers for `Cannot resolve dependency to assembly`; save holders (`curParent`); `absFilePath:` files; `<F>V</F>` field errors; Lunar/Components DLLs analysed; a library several mods ship is not the topmost frame | Same 671 logs: with a source 28.4% → 35.6%, attributed 59.0% → 63.6% |
| (branch claude/g9-harmony-5fddd6) | Manual Harmony targets from IL (`internal/harmony/manual.go`): an abstract interpretation of each method names the original and patch methods of `Harmony.Patch` calls (`ldtoken`, `AccessTools.Method`/`PropertyGetter`/`Constructor`/`TypeByName`, `Type.GetMethod`, `HarmonyMethod(Type, name)`, delegates, expressions, LINQ name searches), with locals/fields/arrays/lists, stack merges at joins, helpers re-evaluated per call site, `this.GetType()` in virtual overrides, and `(MethodBase, HarmonyMethod, …)` wrappers in other DLLs (Vehicle Framework's SmashTools). Unresolved originals are dynamic facts with a reason. `HugsLib.dll` is read in its own mod. Analyzer v6 | Same 671 logs, own homes (home-g9-base on master, home-g9): exact recall 62.9% → 80.9%, with the patch method 71.9% → 95.0%; per-log median 0.64 → 0.84; mods ≥ 90% 74.4% → 94.4%, nothing found 140 → 9. HAR 0 → 67.2% exact / 99.6% method, VF 0 → 92.2%, Performance Optimizer 4.4 → 97.8%, Character Editor, Facial Animation, HugsLib 0 → 100%, Pick Up And Haul 0 → 58.5% (rest: library version), VSE 0 → 94.1%, VEF core 71.7 → 77.6% / 89.8%. Static facts without a runtime counterpart 20.7% → 22.8% (+75k of +258k facts: conditional compat patches). `harmony.multi-transpiler` warnings 4,559 → 11,078 (G8 may want to look) |

| (branch claude/g8-severity-a2d627) | G8: severities from the G4 measurement (03 §4.3.1). `official.order` error → warning (mod subjects 7.4% / 3.6%, 2.1×; the overall 2.3% / 2.5% is not usable, half its subjects are DLCs and Core/DLC are never sources); `order.cycle` warning → info (6.8% / 6.1%; still listed, dimmed and uncounted, in the needs view); `version.unsupported` reversed: stopping at the previous minor 10.4% / 4.0% → warning, older only 0.8% / 3.2% → info. Kept: `dep.missing` error (the 1.6% / 3.3% "(lib)" rows are a research artefact: the player had the dependency, the library did not; real missing dependencies 9.9% / 4.4%), `patch.contested` info (6.3× overall, 3.2× when its mods have no `patch.no-match`, the same as `def.override`), `harmony.multi-transpiler` and `def.abstract-collision` warning (3.0× and 3.2× with Harmony-stack errors excluded, so not only circular). `archo launch` (and the TUI/GUI launch previews, which show its dry run) lists the problems of a code table (`app.LaunchChecks`: patch.no-match, def.parent-missing, incompat.active, dep.inactive, dep.missing), one row per code with why and fix, never blocking (04 §8) | Same 6,727 logs, mapping only. Error findings 41,710 → 40,081, logs with any 65.5% → 59.6%, subject is an error source 12.3% → 12.7% (chance 4.4%); without the `dep.missing (lib)` artefact 30,769 → 29,140, logs 52.6% → 41.5%, 16.1% → 16.9%. Warnings 188,614 → 190,607 (median 16 → 17 per log), 25.9% → 25.8%. Pre-launch list: shown in 60.6% of logs (median 1 problem, p90 34, grouped into ≤ 5 rows), 29.6% of its problems have their mod as an error source (chance 4.7%) |

### 2.1 G1 gap rules (d268a93 … a7117bd), measured

Compared on the same warmed research home (`home-g1`, every log's mods analysed once before;
a fresh home attributes less, because "an installed, disabled mod provides X" needs that mod's
facts). The baseline build (a328b49) on that home reproduces `results-v3` exactly. Mod lists:
the old `parsed.jsonl` on both sides (without `[v:]`/`[ov:]` entries such as HugsLib and CE).

First 671 logs, problem clusters, attributed / with a source:

| | before | after |
|---|---|---|
| all problem | 59.0% / 28.4% | 63.6% / 35.6% |
| texture-missing | 18.3% / 18.3% | 69.7% / 65.9% |
| — without a def (1,187) | 0% / 0% | 60.6% / 57.6% |
| xref-missing | 64.9% / 15.1% | 69.1% / 24.6% |
| — `Failed to find X named …` (738) | 21.8% / 0% | 67.8% / 24.3% |
| — cross-references (11,026) | 76.5% / 25.8% | 81.5% / 43.2% |
| — save `Could not load reference to … named` (8,321) | 53.4% / 2.4% | 52.8% / 0% |
| save-load | 14.7% / 0% | 14.7% / 14.5% |
| — `loadID … curParent=` (2,215) | 16.7% / 0% | 16.7% / 16.6% |
| xml-load | 45.5% / 12.8% | 48.0% / 16.6% |
| — `<F>V</F> doesn't correspond to any field` (1,755) | 46.6% / 21.9% | 53.8% / 39.8% |
| exception / harmony-stack | 59.7% / 53.6%, 97.5% / 73.6% | 61.9% / 54.6%, 99.5% / 74.6% |

The first 671 logs hardly have the assembly line or `_MainTex`, so 160 later logs that do were
measured too (same home, before → after, with a source): assembly line (79) 0% → 89.9%
(Prepatcher hooks `ReflectionOnlyAssemblyResolve`; Dubs Performance Analyzer, which only calls
`ReflectionOnlyLoadFrom`, is mentioned), Skyfaller SoundDefs (65) 7.7% → 78.5%, `GetNamed`
(368) 0% → 28.8%, `absFilePath` files (280) 0% → 23.2%, textures without a def (1,082) 0% → 6.2%.

Deliberately left without a source, with a Details line instead: save references to defs (the
save names the def, not its mod; the earlier text matches were guesses, 2.4% → 0%), think-node
keys, references held by game objects, and `_MainTex` shaders (they live in compressed asset
bundles; 548 logs are mostly one Ariandel Library shader and Odyssey's gravship shader).
Remaining misses: texture paths built at run time from a prefix, and mods shipping the Lunar
framework under another file name (5 clusters). Spot-check of 10 newly sourced clusters: 8
right, 2 plausible (a mod that defines and uses a def the player's game lacked).

## 3. Conclusions (final run v4, 2026-10-22)

Merged master (e993ff4: G1 rules, G2 grading, labelling, G3/G4 capture, parser fix), all 6,734
logs of 1.6 with full mod lists. Numbers in `spikes/corpusrun/RESULT.txt` "Final run".

1. **Attribution: 21.2% → 29.9% of problem clusters have a source** on the full sample (attributed
   to any mod 47.8% → 53.7%; certain 25k → 43k). Logs with ≥ 80% library coverage: 32.0%. Core and
   DLCs are never named as a source. The first accuracy pilot (32 labelled clusters) found every
   source RimArcho named correct; the gap is silence, not wrong blame — preliminary, 4 per class.
2. **The rest are rules, not mods.** Coverage barely matters (32.0% vs 29.9%). Largest remaining
   gaps: harmony-stack sources (32%: the Harmony lines name patchers, rarely the cause), texture
   paths built at run time, save references (deliberately not guessed), `_MainTex` shaders.
3. **A small sample overstates.** The first 671 logs run 6–7 points above the full sample; compare
   rule changes on the same logs, report rates on the full sample.
4. **Declared-constraint violations are common and mostly harmless.** Half of the logs break a
   dependency or load rule, 61% a community rule (now that HugsLib is in the lists, the most broken
   is the community rule "XML Extensions before HugsLib": 1,550 logs, 184 with either as a source).
   Above 100 mods, logs with and without violations have errors equally often. The sorter now
   grades by observed effect (04 §4.4): dependency and loadBefore warn, loadAfter and community
   rules inform; load-order warnings 19,945 → 2,274.
5. **Odyssey loaded late** is the main official-order violation (24% of logs): a DLC enabled after
   the list was built is appended to its end; 1,381 of 1,533 such logs get a specific move.
6. **Harmony static recall 60% (71% with the patch method found).** 82% of the misses are in
   assemblies that call `Harmony.Patch` directly; 96% are owned by mods flagged `harmony.manual`.
   Humanoid Alien Races, Vehicle Framework, Performance Optimizer, Character Editor and HugsLib (its
   DLL is skipped by the scanner) are 0–1%. The backlog item "IL scan for manual Harmony.Patch
   targets" is worth about 23 points (81.7% of the 28.7% missed).
7. **Problem severities, checked against the logs** (a subject mod is an error source: observed /
   chance): `patch.no-match` 37% / 4.5% (and 30% / 1.3% for patch-failed clusters) — the most
   predictive warning; `patch.contested` 42% / 6.6% (info, could be warning); `def.parent-missing`
   16% / 4.1% and `incompat.active` 26% / 7.3% (error, justified); `dep.inactive` 18% / 7.5%;
   `dep.missing (lib)` 1.6% / 3.3% and `official.order` 2.3% / 2.5% (error, at or below chance:
   too severe); `def.override` 25% / 8.1% (info, hidden by default). Harmony codes look 2–3×
   above chance, but attribution names Harmony patchers from the same stack lines, so that ratio is
   partly circular. G8 (§2) acted on this: two of the "too severe" readings did not hold up — the
   `dep.missing (lib)` rows are a research artefact, and `official.order`'s overall rate is blind to
   DLC subjects — and the error tier (16.9%) still predicts less than the warning tier (25.8%),
   because the most predictive problem, `patch.no-match`, is too common to block packs. Precision
   is what the pre-launch list (04 §8) selects on, not severity.

## 4. Next

- ~~Lower `dep.missing (lib)` and `official.order` from error; consider `patch.contested` as warning.~~
  Done in G8 (§2): `official.order` warning; `dep.missing` and `patch.contested` kept, with reasons.
- Pre-launch list (04 §8): add the G10 missing-dependency / known-bad-combination codes to
  `app.LaunchChecks` once measured; check the list's precision on new corpus runs.
- ~~IL scan for manual `Harmony.Patch` targets; scan HugsLib.dll~~ (G9, §2).
- `harmony.multi-transpiler` warnings rose from 4,559 to 11,078 clusters with G9's IL scan;
  review its severity the way G8 did.
- Feed the G5 patterns into MCP `Instructions` / `analyze_log` (draft:
  `docs/research/01-范式库接入草案.md`); 41% of clusters are in kinds the diagnosis workflow does
  not cover (save-load, missing resources, assembly dependencies, config checks).
- Labelling: tighten the fix and cause-type fields, label a larger stratified set (≥ 20 per class),
  then accuracy per class and the agent evaluation set (design §6.5).
- Sorter: a move action for `official.order` in the TUI/GUI needs list; refresh `observed.json`
  when the corpus grows.
