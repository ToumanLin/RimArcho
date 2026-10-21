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
| d268a93, 0ad44ae, a7117bd | Rules for the RESULT gaps (below): missing textures and `GetNamed` names looked up in the enabled mods' XML and DLL string literals; a mod that defines and uses the missing def; reflection-only resolvers for `Cannot resolve dependency to assembly`; save holders (`curParent`); `absFilePath:` files; `<F>V</F>` field errors; Lunar/Components DLLs analysed; a library several mods ship is not the topmost frame | Same 671 logs: with a source 28.4% → 35.6%, attributed 59.0% → 63.6% |

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

## 3. Conclusions

1. **Attribution is limited by rules, not by missing mods.** Logs with ≥ 80% of their mods in the
   library have the same source rate as all logs (21.6% vs 21.2%). More downloads will not help;
   the gaps listed in RESULT (textures without a def, missing assemblies, save-load references,
   `_MainTex`, `Failed to find … named`) need rules.
2. **A small sample overstates.** The first 671 logs gave 28.4% with a source, the full 6,734
   logs 21.2%. Rule changes are compared on the same logs; rates are reported on the full sample.
3. **Declared-constraint violations are common and mostly harmless.** Half of the logs violate a
   dependency or load rule, 45% a community rule. Above 100 mods, logs with and without violations
   have errors equally often. The most violated constraints rarely involve the mods the errors point
   at, and one (`modoptionssort` before Core) cannot be satisfied. The sorter's output should
   distinguish "will break" from "declared but unenforced".
4. **Odyssey loaded late** is the main official-order violation (23% of logs): a DLC enabled after
   the list was built is appended to its end.
5. **Coverage, not accuracy.** No labelled causes yet; the author defers labelling until RimArcho
   has the scaffolding for an agent to do it (design §10 item 3).

## 4. Next

- Rules for the remaining gaps in RESULT, measured on the full sample before and after.
- Sorter: severity per constraint kind, using how often a violation coincides with errors attributed
  to either mod (G2 pair data in `results-v3.jsonl`).
- Labelling scaffolding, then accuracy (G1) and the agent evaluation set (design §6.5).
- G5 patterns: templates and sessions are ready (40% of logs are repeat uploads; 327 of 410
  small-change pairs lose an error cluster).
