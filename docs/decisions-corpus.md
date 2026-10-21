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
   at. `modoptionssort` before Core (651 logs) can be satisfied (Harmony does it; 12 logs keep it) but
   breaking it is harmless (1 log with an error source). The sorter's output should
   distinguish "will break" from "declared but unenforced".
4. **Odyssey loaded late** is the main official-order violation (23% of logs): a DLC enabled after
   the list was built is appended to its end.
5. **Coverage, not accuracy.** No labelled causes yet; the author defers labelling until RimArcho
   has the scaffolding for an agent to do it (design §10 item 3).

## 4. Next

- Rules for the remaining gaps in RESULT, measured on the full sample before and after.
- Sorter: a move action for `official.order` in the TUI/GUI needs list (today: sort only); refresh
  `observed.json` when the corpus grows.
- Labelling scaffolding, then accuracy (G1) and the agent evaluation set (design §6.5).
- G5 patterns: templates and sessions are ready (40% of logs are repeat uploads; 327 of 410
  small-change pairs lose an error cluster).
