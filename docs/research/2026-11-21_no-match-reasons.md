# `patch.no-match` reasons against the log corpus

> Status: measured on 2026-11-21 (dev/1.1 at ded7d6ca, no product change). The recommendation is in 05 §4.4, "Player-facing use". Open: a full 6,734-log run, `exception` (9 findings), and a measurement of `target-missing` against players' real installs (§4).

## 1. Question

05 §4.4 gives each failed top-level patch operation one or more reasons. Two changes in progress need to know which reasons to show players, and at what severity: (A) a per-Mod row mark for reasons that do not fail, promoted to a Can it run item when the current log shows the same failure; (B) `target-inactive` computed from the Def index right after an edit. Rule for the interface: visuals over text, so a reason that seldom predicts an error must not become a warning.

## 2. Method

The method is the G4/G8 calibration of 05 §4.3.1 ([decisions-corpus.md](decisions-corpus.md) §2, §3), run on the current code over 1.6 logs:

- **Sample**: 1,995 of the 6,734 1.6 logs with full Mod lists (`-rawmods`), from four hash shards that were stopped at the time cap (each shard in file order, so the sample is effectively random by gist id). The research library now has 6,322 instances. Per-log coverage: p10 0.74, median 0.91, p90 0.99. v4 had a median of 0.82.
- **Per log**: the log's Mod list becomes the Loadout, followed by analysis (patch simulation) and `AnalyzeLogText`, both unchanged product code. Each failed `patch_fact` is one `patch.no-match` finding. Its reasons are taken from the fact, and its subject is the patching Mod.
- **Columns**:
  1. **Share**: the reason's share of all `patch.no-match` findings. A finding can have several reasons, so the shares add up to more than 100%.
  2. **Source**: the patching Mod is the source of a non-informational cluster in the same log, with an observed rate and a chance rate. Chance is the share of the log's resolved Mods that are a source of any cluster. This is the 05 §4.3.1 rule with one subject. Source attribution counts the log's own patch-failed lines, which makes it circular for an operation that the log reports, so a second figure leaves patch-failed clusters out (**other errors**).
  3. **Logged**: one of the log's own `Patch operation … failed` lines has the same FailKey, meaning the same description and the same Mod-relative file. The player's path ends with the simulated `file`.
- **Units**: rates are given per finding and per (log, Mod, reason). The second unit matches a row mark and keeps one Mod with 50 failing operations from dominating the result.
- **Intervals**: 95% bootstrap over logs (2,000 resamples), because findings cluster within logs and Mods.

The throwaway tool, its results and the full report are in `D:\RimArcho-test\nomatch-stats\` (`src\`, `res-*.jsonl`, `report.txt`). The results stay with the corpus and contain no raw log text.

## 3. Results

869 of 1,995 logs (43.6%) have a `patch.no-match`: 9,744 findings and 2,436 (log, Mod, reason) units. The logs contain 9,318 patch-failed lines in 773 logs. In G4 form over all subjects, the code as a whole is now 54.8% / 4.7% (11.6×), against 37.3% / 4.5% (8.3×) in v4.

Per (log, Mod, reason), with observed / chance (ratio [95%]):

| Reason | Units (share) | Findings (share) | Source | Other errors | Logged, same FailKey |
| --- | --- | --- | --- | --- | --- |
| all | 2,436 | 9,744 | 39.4% / 3.6% (11.0× [10.2–11.7]) | 13.0% / 2.8% (4.6× [4.2–5.1]) | 23.4% [21.5–25.3] |
| `node-absent` | 1,189 (48.8%) | 2,777 (28.5%) | 50.3% / 3.6% (14.2× [13.0–15.5]) | 4.8× [4.2–5.4] | 30.4% [27.7–33.2] |
| `wrong-version-route` | 525 (21.6%) | 3,368 (34.6%) | 29.1% / 3.3% (8.8× [7.8–9.9]) | 6.4× [5.2–7.6] | 7.8% [5.6–10.1]; per finding 1.8% [1.1–2.9] |
| `target-inactive`, all | 469 (19.3%) | 1,419 (14.6%) | 39.9% / 4.4% (9.0× [7.7–10.3]) | 4.3× [3.4–5.1] | 26.2% [21.4–30.6] |
| … with `unguarded-optional` | 139 (5.7%) | 873 (9.0%) | 58.3% / 5.4% (10.8× [8.5–13.4]) | 4.4× [3.1–5.8] | 47.5% [39.2–55.0] |
| … without it (guarded, or a hard dependency) | 333 (13.7%) | 546 (5.6%) | 32.7% / 4.1% (8.0× [6.7–9.4]) | 4.3× [3.3–5.4] | 17.7% [13.3–22.4] |
| `stale-name` | 322 (13.2%) | 682 (7.0%) | 25.2% / 2.8% (8.9× [7.0–10.9]) | 6.8× [5.2–8.6] | 0.3% [0.0–1.0] |
| `target-later` | 103 (4.2%) | 199 (2.0%) | 45.6% / 3.3% (13.8× [10.9–17.1]) | 4.5× [2.1–7.4] | 35.9% [26.9–44.8] |
| `target-missing` | 67 (2.8%) | 1,180 (12.1%) | 3.0% / 3.6% (0.8× [0.0–2.2]) | 1.0× [0.0–2.6] | 0.0% |
| `unclassified` | 53 (2.2%) | 115 (1.2%) | 56.6% / 4.7% (12.1× [9.1–15.9]) | 3.7× [1.7–5.8] | 43.4% [27.8–57.9] |
| `exception` | 9 (0.4%) | 9 (0.1%) | 0 / 9 | 0 / 9 | 0 / 9 |

Reason combinations: almost every finding has a single reason. The exceptions are `target-inactive + unguarded-optional` (873) and `node-absent + wrong-version-route` (5). Per finding, the logged rates are higher for the Mods with many operations: `target-inactive` 48.7% [27.8–69.0], `node-absent` 35.1%, `unclassified` 56.5%. The full per-finding table is in `report.txt`.

Reading:

1. **Whether the game logs the same failure separates the reasons; the source ratio does not.** Every reason except `target-missing` is 8–14× chance as a source and 4–7× for other errors. A Mod with any failing patch is an error-prone Mod, often outdated. The share that the log really reports varies 0–48% by reason, and that share is what "this patch fails in your game" means.
2. **A logged FailKey is the strongest signal of the code.** Units with a logged FailKey: source 24.5× chance, by construction. Units without one: 23.2% / 3.5% (6.6×), still above the 2× warning rule, but as "this Mod has errors" rather than "this patch fails". About 20% of the logged patch failures (1,844 of 9,318) are reproduced by the simulation. The rest come from library-version differences and from the 9% of entries that the library does not have.
3. **`wrong-version-route` and `stale-name` seldom fail in the game** (1.8% and 0.3% of findings). Their Mods have other errors at 6–7×, which makes them good "this Mod looks out of date" signals but poor patch warnings. A likely cause is that the library has the current Workshop version of the target or the patching Mod, while the player had an older matching one.
4. **The corpus cannot measure `target-missing`.** In the research environment it means that the library lacks the definer. The player usually had it, so nothing failed, which gives 1.0× and 0% logged. This is the artefact that `dep.missing (lib)` showed in G8. In a player's own install, the definer really is absent. A guarded operation then does not fail, and an unguarded one is the author's bug, which the player can do nothing about.
5. **`target-inactive` in the corpus** means that the library has the definer and the player's list does not. The player may not have installed it, which corresponds to `target-missing` in their game. The 47.5% logged rate with `unguarded-optional` is therefore a lower bound for "installed but disabled".
6. **`exception`**: 9 findings, none logged, none a source. Too few for a severity.

## 4. Limits and next

- 30% of the 1.6 logs. A full run takes about 6 h on 4 homes of about 300 MB each; the shards resume from their `res-*.jsonl`.
- The library has current versions, not the player's (see 3), and Mod-relative FailKeys can miss a player who renamed a local Mod folder. This does not affect the suffix match.
- Measuring `target-missing` and `target-inactive` against real installs needs logs with the player's installed set, which the HugsLib uploads lack. Archo Monitor's companion log (19) could provide it.
