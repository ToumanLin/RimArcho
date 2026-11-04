# MCP transcript review: 2026-11-04 A/B matrix

Source: the 30 runs of the overnight matrix on 2026-11-04 (Pi 1.4, model `magpie/apikeyfan/deepseek-flash`, real RimWorld 1.6.4871 in the VM, build `1.0.1-dev+490fa4fc`, `archo mcp` with the default `core` group). Case design and expected answers: [mcp-eval-design.md](mcp-eval-design.md). Raw data (read-only): `D:\RimArcho-test\mcp-eval\results\<case>-<group>-<run>\`. A reference such as `c3-A-1 L115` means line 115 of that run's `transcript.jsonl`. No code was changed for this review, and archo was not run.

## Summary

Group A solved 15 of 15. Most of its remaining time went to three things, none of them the injected fault:

1. **Baseline noise.** The `SolidColorMat-Custom/Solid Color` material cluster is in every case's log, in the reference log of every correct fix, and in every `test_run` report. Agents chased it in 11 of 15 A runs, for about 1,190 s in total (80 s per run, 15% of A's wall time). The biggest cases are c3-A-1 (about 340 s) and c5-A-1 (about 250 s, two extra `test_run`s spent disabling innocent mods). Archo itself pointed them there: the cluster's pattern route is `search_files uiIconColor` (§2.2).
2. **`test_run` never says "your fix worked".** All 22 `test_run` calls returned `isError` because of that noise cluster. The first run of a plan has no baseline, so it has no `gone` list either. 15 of the 22 results were re-checked by hand right away (11 bash greps of the run's `Player.log`, 4 `analyze_log file=`).
3. **Harness snooping.** Agents looked for "what was injected" through timestamps, `C:\Eval\lib`, `git diff` and `agent.ps1` (about 60 s per A run). This is an eval artifact. A small part is a gap: no tool says which files of a mod differ from its upstream copy.

Two eval artifacts weaken A in c1 and c3. The staged case logs had no `Mono path` line (`scripts/mcp-eval/package.mjs` removed it), and nothing in the eval launched the game or wrote the list through RimArcho. So Archo could not tell which game wrote the player's `Player.log` and never ran the attribution analysis. (Correction, 2026-11-04: this review first said the logs named a host path; the P4 work found they named none.) As a result, the c1 type-missing cluster and the c3 Harmony cluster show `(no mod identified)` (§2.4). Rerun c1 and c3 after P4 (and with `package.mjs` now keeping a rewritten `Mono path[0]`) before drawing conclusions about attribution.

Top proposals (details in §7): compare `test_run` with the player's log and call pre-existing clusters what they are (P1); mark the solid-colour material cluster as known noise and drop its misleading route (P2); accept a bare packageId wherever a mod reference is expected (P3); attribute logs whose game path is unknown instead of skipping analysis (P4); add "did you mean" types for `xml.type-missing` (P5); add a static `harmony.target-missing` finding with the real overloads (P6).

## 1. Results by case

| Case | A: s per run (avg) | A: Archo calls (avg) | A: Mtok | B: s per run (avg) | B: Mtok | A solved | B solved |
| --- | --- | --- | --- | --- | --- | --- | --- |
| c1 type typo | 553, 419, 304 (425) | 8 | 1.32 | 900, 832, 695 (809) | 3.02 | 3 | 3 |
| c2 patch vs order | 390, 652, 624 (555) | 12 | 2.93 | 385, 900, 814 (699) | 1.59 | 3 | 2 |
| c3 Harmony / DLL | 805, 503, 295 (534) | 6 | 2.31 | 629, 900, 669 (732) | 3.29 | 3 | 2 |
| c4 dependency off | 487, 568, 415 (490) | 18 | 2.42 | 900, 900, 842 (880) | 4.38 | 3 | 1 |
| c5 wrong blame | 783, 790, 297 (623) | 18 | 2.31 | 900, 419, 837 (718) | 2.61 | 3 | 2 |

Where A's time went. The numbers come from the time between an assistant turn and the next turn, assigned by what the turn's calls touch (script: `noise.py` in the review scratchpad; approximate):

| Run | Total s | Noise (`SolidColor`, `uiIconColor`, `_MainTex`) | Harness snooping | Waiting for `test_run` | Notes |
| --- | --- | --- | --- | --- | --- |
| c1-A-1 | 553 | 148 | 15 | 143 | Fix verified at 347 s; then decompiled Melee Animation, HAT, Assembly-CSharp for `SolidColorMat` |
| c1-A-2 | 419 | 0 | 0 | 60 | 39–300 s: built a list of every XML class name and every type in every DLL by hand to find the typo (§2.5) |
| c2-A-3 | 624 | 90 | 174 | 65 | Downloaded the Odyssey Patch repository from GitHub; translations; `SolidColorMat` |
| c3-A-1 | 805 | 340 | 4 | 63 | Fix verified at 399 s; the other 400 s were spent on the noise cluster (decompiled the whole Assembly-CSharp, VEF, Facial Animation) |
| c3-A-2 | 503 | 156 | 57 | 79 | |
| c5-A-1 | 783 | 246 | 130 | 250 | Culprit known at 31 s (`search_files`); disabled God Hands and Melee Animation and ran `test_run` twice to bisect the noise |
| c5-A-2 | 790 | 46 | 88 | 134 | One bash `stat` loop over 13,764 files took 335 s (L1679 → L2080), looking for "injected" files |
| All 15 A runs | 7,887 | 1,194 | 891 | 1,581 | |

**What would have shortened A.** In c5, `analyze_log cluster=E2` and `search_files` named 10x Carrying Capacity as the source within 12–31 s in all three runs (`c5-A-2 L315`: "#m35t 10x Carrying Capacity (Continued) source evidence: references OutfitStandsPlus_MechanizedOutfitStandResearch at Patches/Stats/Stats_Pawns_General.xml:14"). The 623 s average is almost all noise, snooping and the extra test runs. P1 and P2 address it. c3-A-1's 805 s is the same story after the fix. In c1-A-2 and c3, the missing attribution (eval artifact, P4) and the missing nearest-type and overload facts (P5, P6) would have saved 100–260 s.

## 2. Friction, with evidence

### 2.1 `test_run`: every result is an error; the first one has no comparison

- All 22 calls set `isError`, including runs that reached the menu or map with only the baseline noise left (`c1-A-1 L4699`: `"status": "menu"`, one cluster `SolidColorMat…`, `"mods": []`, `"confidence": "possible"`). 08 §12 decides this on purpose: the exit code counts every error cluster, and the previous run is not a known-good reference.
- The baseline is the previous **test run** of the same plan, so the first `test_run` in a session has no `baseline` or `gone` at all. A map run (`map=true`) needs an earlier map run as its baseline. Only 5 of the 22 reports had a `baseline`. The agent's real question is "are the errors of the player's log gone?", and no tool answers it directly. When there was a baseline, `gone` did the job (`c2-A-2 L12153`: `"gone": 6`, including the `[Outfit Stands Plus] Patch operation … failed` cluster). Even then, `gone[]` entries lose their attribution (`"mods": [], "confidence": ""`).
- After 15 of 22 results the agent re-read the run's log itself (`c1-A-1 L5029`: `grep -n -iE "error|exception|failed|could not|config error" …/runs/…/Player.log`), or called `analyze_log file=<run log>` (`c1-A-3 L3529`).
- The JSON is pretty-printed and repeats `args`, `run_dir` and `log_path` every time. With 15 `patch-failed` clusters of one mod it was 7.5 KB (`c5-A-1 L8355`).
- A mono runtime crash at exit produced a cluster whose message is a separator line: `"class": "exception", "message": "================================================================="` (`c2-A-2 L3854`, also `L4041` as `log:E5`).

### 2.2 The solid-colour material cluster leads agents astray

- `analyze_log cluster=E5` (`c1-A-3 L998`) says "usually harmless unless something renders wrong" but also gives `causes: a buildable def with uiIconColor but no icon path` and `route (tools): search_files uiIconColor`. 7 of the 12 `search_files` calls in the matrix searched `uiIconColor` and found only Core and Anomaly (`c1-A-3 L2167`, `c2-A-3 L5347`, `c3-A-1 L14774`, `c4-A-2 L7703`, `c5-A-1 L5197`, `c5-A-2 L3278`, `c5-A-3 L3542`).
- The message `Material 'SolidColorMat-Custom/Solid Color-RGBA(0.000, 0.000, 0.000, 0.000)'` names a material made in code (`SolidColorMaterials`), not a def icon. The pattern evidence itself says it is common: "seen in 2,696 of 40,326 public 1.6 logs".
- `test_run` lists it as the only cluster with `"mods": []`. Agents read the remaining error as theirs to fix.
- The `Translation data for language English has 19 errors` line, also baseline, drew a further ~30–300 s per run in both groups.

### 2.3 Bare packageIds are rejected

All 7 non-`test_run` errors are the same: a packageId without `pkg:`.

```
c2-A-2 L12027  edit_loadout refs=["khamenman.outfitstandsplus"]
  error: khamenman.outfitstandsplus matches nothing (not a reference (#id, L#id, pkg:<packageId>, ws:<workshopId>); search with archo find khamenman.outfitstandsplus)
```

The same error occurs in `get_mod` (`c1-A-1 L6316`), `edit_loadout` (`c2-A-3 L10689`, `c4-A-3 L4725` with the declared case `OskarPotocki.VanillaFactionsExpanded.Core`, `c5-A-1 L8108`), `list_changes ref=` (`c5-A-1 L3740`) and `get_patches mod=` (`c5-A-3 L4182`). Each cost one or two round trips (find_mods, then retry, 10–25 s). The error text points to the CLI (`archo find`) instead of `find_mods`. Agents never tried the CLI: no bash call ran `archo`.

### 2.4 Attribution skipped for a log of unknown owner (eval artifact, real gap)

- `EnsureAttributionAnalysis` (`internal/app/logowner.go`) analyses only when `logOwner` says the log belongs to this environment. The staged case logs have no `Mono path` line at all (`package.mjs` removed it), and there was no RimArcho launch or `ModsConfig.xml` write before them, so `logOwner` found no owner. So the eval never analysed before the first `analyze_log`. A log whose header names an unregistered game folder gets the same result.
- Effect in c3 (`c3-A-1 L115`): "detail: failing type SmartFirefight.Startup: no enabled mod's analysed assemblies define it (search_files / find_mods for its namespace)" and "attribution: (no mod identified)". The message reads as "no mod defines this type". In fact nothing had been analysed.
- Effect in c1 (`c1-A-1 L358`): `Could not find a type named OutfitStandsPlus.Thing.MendingOutfitStand`, "attribution: (no mod identified)". The next log line names the def `OutfitStandsPlus_MendingOutfitStand`.
- `list_problems` hid the log findings for the same reason: "stale log (source unknown: no launch and no ModsConfig.xml write by RimArcho before it): 6 warning, not counted" (`c2-A-1 L3604`). The agent needed a second call with `code=log`.
- The same happens to a real player whose game folder moved, or who copies a log from another machine.

### 2.5 `xml.type-missing` gives no candidate type (c1)

The pattern's causes are "the framework mod that defines the class is not active or loads later; the class was renamed in an update" (`c1-A-1 L358`). A typo in the XML is not among them, and no tool lists the similar types that do exist. c1-A-2 spent 39–300 s building that list by hand: it grepped every `<thingClass>`, `<compClass>`, `<workerClass>` and `Class=` in all XML, ran `ilspycmd -l c` on every game and mod DLL, and diffed the two (L1611–L3318). All three c1 A runs decompiled `OutfitStandsPlus.dll` to see `OutfitStandsPlus.Things.MendingOutfitStand`.

### 2.6 `get_patches failed=true` contradicts the log (c2)

- The log says `[Outfit Stands Plus] Patch operation … (thingClass="Building_OutfitStand")/comps) failed`. `get_patches failed=true` answered "patches: 0 failed of 146 operations simulated, 2 not simulated" (`c2-A-1 L5786`, `c2-A-3 L4942`, and after the fix `L10833`).
- The 2 operations it could not simulate are the cause: the Odyssey Patch's `OdysseyPatch.PatchOperationReplaceIf` rewrites `thingClass` first. `list_problems code=patch` mentions them only as info (`c2-A-3 L10835`: "info patch.unknown-op 2 operations use mod-defined classes that cannot be simulated").
- `list_problems` gave a misleading explanation (`c2-A-1 L3655`): "inferred simulation: this operation does not fail now (the mod or the Loadout changed since the log)". Nothing had changed; the simulation was incomplete.
- `get_def ThingDef/Building_OutfitStand` showed the OSP patch as `hit 1` (`c2-A-1 L8938`), which the agent had to reconcile with the log by hand.

### 2.7 Many clusters, one cause (c4)

- `analyze_log` listed 132 clusters for one disabled dependency (`c4-A-2 L212`, 4.8 KB; `all=true` 19 KB at L253). None of them says "dependency".
- The `ReflectionTypeLoadException … VanillaFurnitureEC` cluster names Vanilla Furniture Expanded as the source with pattern `asm.version-mismatch` ("fixes: update, disable"; `c4-A-1 L4423`). The neighbouring line names the missing assembly `'VEF, Version=1.1.7.0'`, shipped by the installed but disabled `#ssne`.
- `xref-missing` for `StatDef/Beauty` says "providers: none among installed mods" (`c4-A-2 L408`), but `get_def StatDef/Beauty` shows Core defines it (`L466`). The def is missing because `Stats_Basics_General.xml` failed to load (cluster E6, `Could not find type named VanillaFurnitureEC.StatPart_Bin`).
- `list_problems` was the tool that answered c4 at once: "error dep.inactive #dmop Vanilla Furniture Expanded requires pkg:oskarpotocki.vanillafactionsexpanded.core; #ssne is installed but not enabled" (`c4-A-1 L4421`). Runs that called it early finished fastest. c4-A-1 first read the 1,795-line log in bash for 130 s.

### 2.8 Results that were too large

| Call | Size | Use |
| --- | --- | --- |
| `list_problems all=true` / `min_severity=info` (4 calls) | 19.9–20.6 KB; Pi truncated one ("Warning: truncated output (original token count: 6167)", `c2-A-3 L4940`) | 137–145 info findings (`def.override`, `harmony.shared-target` …), none relevant |
| `analyze_log all=true` in c4 | 19.1 KB | the same 132 clusters |
| `test_run` with many clusters | 7.5 KB | pretty JSON |

Every cluster's `nearby:` block starts with 4 lines of a Harmony transpiler dump from AB's Head Apparel Tweaker (`before:55 AdjustParms - ldfld System.Collections.Generic.List`1[RimWorld.RenderSkipFlagDef] renderSkipFlags [0],`). That is about 500 characters per cluster call, in 49 of 62 `analyze_log` calls.

### 2.9 CLI commands in MCP output

`next:` and `fix:` lines are CLI commands, as 05 §3.1 rule 11 requires: `next: archo log --cluster E2`, `hidden: 4 informational clusters (archo log --all)`, `fix: archo enable ssne`, `next: archo save` after every `edit_loadout`, `next: archo mod 4bdk` after `find_mods`. In this run no agent tried to execute them, so the cost is tokens and a small translation step, not errors.

## 3. Usage

### 3.1 Calls per tool (group A, 15 runs)

| Tool | Calls | Runs | Errors | Result chars: median / max | Used? |
| --- | --- | --- | --- | --- | --- |
| `analyze_log` | 62 | 15 | 0 | 1,643 / 19,087 | Yes: first clusters in every run; c5 attribution named the culprit |
| `test_run` | 22 | 15 | 22 (`isError`) | 1,201 / 7,546 | Partly: status used, cluster list re-checked by hand (§2.1) |
| `list_problems` | 17 | 10 | 0 | 1,248 / 20,626 | Yes in c4 (`dep.inactive`); `all=true` calls unused |
| `status` | 15 | 15 | 0 | 658 / 658 | Called first in every run, as its description says; content rarely used |
| `find_mods` | 15 | 10 | 0 | 152 / 3,335 | Mostly to recover from §2.3 |
| `edit_loadout` | 13 | 6 | 4 | 137 / 195 | Yes (c2 move, c4 enable) |
| `search_files` | 12 | 7 | 0 | 517 / 643 | Yes in c5 (culprit in 30 s); 7 calls were the `uiIconColor` route |
| `get_mod` | 7 | 5 | 1 | 1,369 / 1,450 | Little |
| `get_patches` | 7 | 4 | 1 | 82 / 199 | Misleading in c2 (§2.6) |
| `list_changes` | 5 | 4 | 1 | 73 / 161 | No ("nothing changed"; the case has no history) |
| `save_loadout` | 5 | 3 | 0 | 267 / 382 | Yes (c4 dry run, then save) |
| `get_def` | 4 | 3 | 0 | 367 / 367 | Yes (c2, c4) |
| `list_library` | 3 | 3 | 0 | 2,423 / 2,509 | No |
| `plan_history` | 3 | 2 | 0 | 168 / 179 | No |
| `scan` | 1 | 1 | 0 | 221 | No |
| `read_source` | 1 | 1 | 0 | 163 | No (mod without assemblies) |

**Never called (10 of 26):** `guide`, `get_harmony`, `explain_relation`, `get_description`, `ignore_problem`, `launch_game`, `manage_plans`, `manage_rules`, `pin_mod`, `unpin_mod`. Notes:

- `get_harmony` and `read_source` went unused even in c3. All six c3 runs (A and B) decompiled `Verse.GenSpawn` with `ilspycmd`, which the task prompt names. `read_source game=true type=Verse.GenSpawn` would have done it.
- `guide` was never called. The server Instructions, which say "read guide topic=diagnose before concluding", do not appear in the system prompt Pi recorded (`sections` has only preamble, tools, rules, docs and cwd; no transcript contains "Start with status"). Pi 1.4 probably does not pass MCP `instructions` to the model. Agents still called `status` first in all 15 runs, which its description asks for.
- `pin_mod` was never called although agents edited mod files in place. The task asked for files under `C:\Eval\submit`, and the mods were local, not Workshop.

### 3.2 Archo vs bash in group A

Group A made 192 Archo calls and 688 bash calls. Archo results were 324 K characters; bash results were 1.63 M. So bash still produced 83% of what A read. Bash was used to read mod files (`cat Defs/…`), to decompile and build (`ilspycmd`, `dotnet build`), to copy files to `C:\Eval\submit`, to grep test-run logs, and to snoop on the harness.

## 4. Missing tools: what group B did by hand

B found the right culprit within about 110 s in 11 of 15 runs (for example `c1-B-1 L827`, t=10: "missing 's' in Things"). It then spent 300–800 s without a way to tell which remaining errors were the injected ones. Time per category over all 15 B runs (per-turn split by command text, about ±15%):

| Category | Seconds | Share | Archo tool that covers it |
| --- | --- | --- | --- |
| Locating mods, defs, packageIds; `find`/`grep` not following the `Mods\` junctions (11 of 15 runs, 10–70 s each, e.g. `c5-B-2 L625–L1116`) | 2,442 | 21% | `find_mods`, `get_mod`, `search_files`, `get_def` |
| Provenance hunting: `git diff`, mtimes, md5, GitHub clones, harness files | 2,230 | 19% | none (P15) |
| Starting the game by hand: about 26 launches with Start-Process + sleep + kill; `-batchmode` wrote no log (`c4-B-1 L8249`); ModsConfig backups | 2,016 | 18% | `test_run`, `edit_loadout` + `save_loadout` |
| Decompiling and metadata: in c4, home-made System.Reflection.Metadata tools comparing TypeRefs (`c4-B-3`: 38 calls, 338 s) | 1,781 | 16% | `read_source`; `list_problems` makes the c4 work unnecessary |
| Noise: the 19 translation errors, `SolidColorMat`, "Mod Quest Maker dependency" warnings (12 of 15 runs) | 1,286 | 11% | partly `analyze_log`; see below |
| Reading and deduplicating `Player.log` (c4: 1,795 lines; `c4-B-1 L773`) | 642 | 6% | `analyze_log` |
| Building DLLs (`c3-B-1 L7210`: mscorlib vs netstandard reference conflicts) | 234 | 2% | none |

Timeouts and what would have saved them:

- c1-B-1: about 330 s on the `SolidColor` warning, then 270 s enumerating every type in every DLL. P2, plus `test_run` to confirm.
- c2-B-2: one `grep -Ril "Quest Maker"` over 1,296 Workshop folders ran from t=259 to the cutoff (L4939). `get_patches` and `get_def`, and `search_files`, which stays within enabled mods.
- c3-B-2: about 450 s on the 19 translation errors, including a home-made diagnostic mod and a second game start (`L13828`); it never wrote `answer.json`.
- c4-B-1: it saw "Vanilla Expanded Framework … inactive!" at t=97 (`L1818`) but doubted it for 500 s. c4-B-2 confirmed the fix but submitted no answer. `list_problems dep.inactive` gives the authoritative answer.
- c5-B-1: it blamed the decoy (Outfit Stands Plus) at t=69, then built a `TranslationReport` mod. `search_files def=true` shows that only 10x Carrying Capacity's patch carries the misspelled name.

Manual work that no current tool covers, which becomes candidate tools:

1. **Translation-error attribution.** About 1,100 s in B (three runs built diagnostic mods for it) and some A time. Report per mod the duplicate Keyed keys, DefInjected folders that are not def types, and injections whose target def is missing (P16).
2. **Baseline / pre-existing marking** (P1, P2). It would have shortened 9 B runs and most A runs.
3. **Provenance: files that differ from the mod's upstream copy** (P15).
4. **Harmony target resolution with the real overloads** (P6).
5. **Bisection.** c5-B-3 removed mods one at a time over about 5 launches (422 s) with a shell script; c5-A-1 did the same through `edit_loadout` + `test_run` (two runs, about 130 s). It could become a `test_run` mode that halves a set of candidates until a cluster disappears (P17, low priority: the cases did not need it once attribution worked).
6. **Rebuild from shipped source** against the game's own assemblies. Every c3 run hand-wrote a csproj and found the Mono `mscorlib` and `0Harmony` references by trial (P18, low priority).

Eval validity notes from the B transcripts: B read `C:\Eval\agent.ps1` and `out\transcript.jsonl` (`c4-B-1 L11404`: "Group A has archo MCP"). B cloned from GitHub (c2). Something in the model path replaced a type name with `{{SECRET}}` in tool results, 107 times in `c4-B-3` (`VEF.Buildings.{{SECRET}}`). That made VEF.dll look tampered with and fed a wrong turn ("So VEF.dll is broken", `c4-B-3 L6067`).

## 5. Output size and schema cost

- **Schema per turn.** The first request of an A run has 8,661 input tokens, against 1,948 for B: the Archo tool list adds **about 6,700 tokens to every request**. Pi's recorded `toolsAdded` for the 26 Archo tools is 25.8 KB of JSON (05 §5.1 measures 27,435 bytes for core `tools/list`). Over A's 714 turns that is 4.8 M of 33.8 M tokens (14%). With a 96–97% cache hit rate it is cheap in money, but it is also context the model carries in every turn. The largest schemas: `search_files` 2.3 KB, `manage_plans` 1.7 KB, `list_library` 1.5 KB, `list_problems` 1.5 KB, `analyze_log` 1.45 KB, `read_source` 1.4 KB, `pin_mod` 1.4 KB. The 10 tools never called account for about 9.4 KB (about 2,300 tokens).
- **Per result.** In characters (tokens ≈ characters / 3.5 for this mixed text): `analyze_log` median 1.6 K (126 K total), `list_problems` median 1.2 K but 104 K total because of four 20 K calls, `test_run` median 1.2 K (43 K total). Everything else is under 1.5 K. A tool result was rarely the problem; the bash `cat` of whole files and logs was (`read` of `Player.log` 48 K in B).

## 6. Descriptions and Instructions

- **Instructions not delivered (likely).** See §3.1. Whatever must reach every client cannot live only in `Instructions`. Two rules matter most here: "read guide topic=diagnose" and "a patcher or a mentioned mod is not the cause".
- **`asset.maintex` route** (§2.2): the pattern library's route and cause misled 7 calls and much bash work. This is the most costly piece of wording in the run.
- **`xml.type-missing` causes** (§2.5) leave out "a typo in this mod's XML", the most common local cause.
- **`asm.version-mismatch` for a disabled dependency** (§2.7): "fixes: update, disable" is wrong when the expected assembly is installed but not enabled.
- **"no enabled mod's analysed assemblies define it"** (§2.4) reads as a fact about mods when it is a fact about the analysis state.
- **`get_patches` "0 failed"** while operations are not simulated (§2.6) reads as "patches are fine".
- **`test_run` description** says "Compares with the previous run". Agents expected a comparison with the player's log and did not get one on the first call.
- **`list_problems`** description says "Warning and above by default". Agents added `all=true` or `min_severity=info` 6 times hoping for more and got 137 info rows.
- Nothing in the descriptions made agents call a wrong tool. `status` first, `analyze_log` then `cluster=`, `search_files` for a defName and `list_problems` for dependencies were all picked correctly.

## 7. Proposed changes, by priority

| # | Change | Tool / text affected | Expected effect | Spec |
| --- | --- | --- | --- | --- |
| P1 | `test_run` also compares with the player's problem log, the log `analyze_log` reads by default, whenever it exists (always, so the first run of a plan and the first map run get one too). Each cluster gets `before: present / absent`, and a `fixed:` list names the player-log clusters that are gone, with their attribution kept (also for `gone[]`). Consider `isError=false` when every remaining cluster was already in the player's log and at least one is gone. That keeps the "unchanged faulty list fails" rule, because an unchanged list removes nothing. | `test_run`, `descTestRun` | Removes most hand re-checks (15 of 22) and covers the 17 of 22 reports that had no baseline and much of the noise chase (about 80 s per run) | 08 §12 (comparison, exit code), 05 §5.2 |
| P2 | Specialise the material pattern: `SolidColorMat-…` with a code-made shader is "known noise: a material made in code; no def or file causes it" with no `uiIconColor` route; class it informational (hidden by default, like the `Fallback handler` clusters). Do the same for `Translation data for language … has N errors`. Generally, a cluster with no attribution whose pattern is in more than 5% of public logs is shown as `common, unattributed`. | `analyze_log`, `test_run`, pattern library | Ends the single biggest time sink in A (c3-A-1 −340 s, c5-A-1 −250 s) | 03 §6.8, 03 §6.1 |
| P3 | Accept a bare packageId (contains a dot, matches case-insensitively) as `pkg:<id>` wherever a mod reference is expected; on no match, the MCP error names `find_mods query=…`, not `archo find`. | reference resolver; all tools with `ref`, `refs`, `mod`, `before`/`after` | 7 errors and about 15 round trips in 15 runs gone | 05 §2, 05 §3.3 |
| P4 | When the log's game path matches no known environment, attribute against the current environment anyway (it is the only candidate), and say `log game path D:\… is not this environment's game; attributed against env:main`. Never say "no enabled mod's analysed assemblies define it" when nothing was analysed: say `assemblies not analysed yet` and analyse. Count such a log's findings in `list_problems` instead of "not counted". Harness: rewrite `Mono path[0]` when staging case logs, then rerun c1 and c3. | `analyze_log`, `list_problems`, `logOwner`, harness `guest/overlay.ps1` | c1 and c3 name the culprit in the first `analyze_log` | 03 §6.4, 03 §4.5 |
| P5 | `xml.type-missing`: name the mod whose XML holds the class (search the defs; `source` role) and list the nearest existing types from analysed assemblies (same simple name, small edit distance): `did you mean OutfitStandsPlus.Things.MendingOutfitStand (OutfitStandsPlus.dll, #oa3s)`. Add "a typo or wrong namespace in this mod's XML" to the pattern's causes. | `analyze_log` cluster detail, pattern text | c1 solved from one call; c1-A-2 −260 s | 03 §6.2, 03 §6.8 |
| P6 | New static finding `harmony.target-missing` (error when the method is in a log cluster): an attribute patch whose target type, method name or parameter list does not resolve in the game or dependency assemblies. List the existing overloads, for example `GenSpawn.Spawn has (Thing, IntVec3, Map, Rot4, WipeMode, bool, bool); the patch names 6 parameters`. Link it from `asm.harmony-target` clusters. | `list_problems`, `get_harmony`, `analyze_log` | c3 diagnosis without decompiling Assembly-CSharp (100–150 s in c3-A-1 and c3-A-3) | 03 §4.3, 03 §5.3 |
| P7 | When a logged `patch-failed` operation simulates as passing, say why it may differ. List the unsimulated operations that run earlier on the same def (`OdysseyPatch.PatchOperationReplaceIf ×2 in #aflv, loads before, on ThingDef/Building_OutfitStand`). Replace "the mod or the Loadout changed since the log" with that when nothing changed. `get_patches failed=true` prints the `not simulated` operations next to "0 failed". | `get_patches`, `list_problems` log evidence, `analyze_log` | c2 cause found without decompiling the Odyssey Patch | 03 §5.2, 03 §4.4 |
| P8 | Group clusters by root cause. When many `xml-load` / `xref-missing` / `ReflectionTypeLoadException` clusters name types of an assembly that an installed but disabled mod ships, lead the list with one line: `85 clusters: types in VEF.*, VanillaFurnitureEC.*; VEF.dll is shipped by #ssne Vanilla Expanded Framework (installed, not enabled) → list_problems p:eceb27`. Give such clusters the `dependency` role and a `fixes: enable-dependency` pattern. Fix `providers: none among installed mods` when a def's file failed to load: name the file and cluster. | `analyze_log`, `list_problems` cross-links | c4 answered by the first `analyze_log` | 03 §6.2, 03 §6.3 |
| P9 | `test_run` output: drop `args`, compact the JSON (no indentation), and collapse clusters of one mod and one class into one row with a count. | `test_run` | `test_run` results half their size | 08 §12 |
| P10 | Put the two key diagnosis rules into the `analyze_log` description (within the 320-character budget) and keep them in `Instructions`. Some clients (Pi 1.4 here) do not show `Instructions`. | `descLog`, `Instructions` | The rules reach Pi-like clients | 05 §5.3 |
| P11 | In MCP results, render `next:` / `fix:` / `(archo … --all)` hints as tool calls (`analyze_log cluster=E2`, `edit_loadout action=enable refs=[#ssne]`), or omit them. The CLI keeps its commands. This is an exception to I1 for hint lines only. | render layer (`ShellSafe` / `Doc.String`), all tools | Fewer tokens; no translation step | 05 §3.1 rule 11, 05 §1.1 |
| P12 | `list_problems all=true` with more than 30 info findings: collapse info by code with counts (as `harmony.skip-prefix` already is). The full list stays behind `code=`. | `list_problems` | Removes the 20 KB results | 03 §4.5, 05 §3.2 |
| P13 | Drop known dump lines (IL listings such as `AdjustParms - ldfld …`, transpiler traces) from `nearby:`. | `analyze_log` | About 500 characters per cluster call | 03 §6.1 |
| P14 | Schema: shorten parameter descriptions of the largest schemas (`search_files`, `manage_plans`, `list_library`, `pin_mod`). Measure a `diagnose` group (status, find_mods, get_mod, analyze_log, list_problems, get_def, get_patches, get_harmony, search_files, read_source, edit_loadout, save_loadout, test_run, guide) against `core` in the next matrix. | `Tools()` schemas, `--tools` | Up to 2,300 tokens per request | 05 §5.1, 05 §5.3 |
| P15 | A "what differs from upstream" view: `list_changes ref=<mod>` or `get_mod` reports files that differ from the mod's Workshop or Git origin, where one is known. | `list_changes` | Would replace the timestamp hunting, which is partly an eval artifact | 02, 05 §4.2 |
| P16 | Translation-error attribution: for `Translation data for language X has N errors`, report per mod the duplicate Keyed keys, DefInjected folders that are not def types, and injections whose target def is missing. It is static, from the language files. | new finding family under `list_problems`; `analyze_log` detail | About 1,100 s of B work and some A work; also a real player question | 03 §4.3, 03 §6.8 |
| P17 | Bisection mode for `test_run`: given candidate mods and a cluster, halve the set over test runs until the cluster disappears, then report the mod. | `test_run` | Low priority: needed only when attribution fails | 08 §12 |
| P18 | Build help for a mod's shipped source: list the reference assemblies the game loads (Mono `mscorlib`, the active `0Harmony`, game `Managed\`) as csproj-ready paths. | `read_source` or `guide` | Low priority: about 30–60 s per c3 run | 03 §5.4 |

## 8. What could not be determined

- Whether Pi 1.4 sends MCP `instructions` to the model somewhere the transcript does not record. The recorded system prompt has no trace of it, and Pi is not installed on the host to check its source.
- Exact token counts per tool result: Pi records usage per request, not per result. Sizes above are characters.
- How much of A's c1 and c3 time would remain with the log ownership fixed (P4). That needs a rerun.
- Whether P1's `isError` change is acceptable under 08 §12's decision; the proposal keeps the "unchanged list fails" property, but the owner of 08 §12 should decide.
- Time attribution in §1 is per turn, by keyword. A turn that mixes noise work with real work is counted fully as noise, so the numbers are approximate in both directions.
- Archo was not run against synthetic data for this review: every size is taken from the transcripts, which reflect the real eval build.
