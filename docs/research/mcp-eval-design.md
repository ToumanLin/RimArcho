# Archo MCP A/B evaluation: design (draft 2026-11-02)

Goal: measure how much `archo mcp` helps an agent find and fix a broken Mod, and find which tools to improve.

## Fixed conditions

| Item | Value |
| --- | --- |
| Game | RimWorld 1.6.4871 (`D:\rimworld16`; host env `env:1.6`) |
| Player environment | plan `Test` of env `1.6`: 35 active Mods (Harmony, Archo Monitor, 5 DLCs, VEF, VFE, character/animation/hair Mods). Real Mod library, nothing cleaned: multi-version folders (1.2 to 1.6) stay |
| Baseline noise (keep it) | `Player.log` already has 10 errors / 2 warnings from Mods that are not part of any task (ReGrowthCore, VTE, ReBuild... `Could not find class ... Trying to use ModSettingsFramework.PatchOperationWorker`). Agents must not blame these |
| Agent | model `apikeyfan/deepseek-flash` through the magpie gateway, reasoning effort `high`, same task text and wall clock for every group |
| Group A | Pi 1.4.0: bash + file read/write + `dotnet` + `ilspycmd` + **Archo MCP** (`archo mcp`, exposure `direct`) |
| Group B (from 2026-11-04) | dsh 0.2.1-rc.5 (DeepSeek Harness, `--profile headless --json`, patch `scripts/mcp-eval/guest/dsh-eval.yml`): its own PowerShell, file and search tools, subagents, todo; the same **Archo MCP** (per-call timeout 15 min; dsh's default 60 s would cut `test_run` off). Web search and fetch, the session-title model call and telemetry are off; no approval prompts (`danger-full-access`, Pi has none either). dsh puts Archo's MCP server instructions into the system prompt; Pi 1.4 drops them (review P10), so the groups do not see the same Archo guidance |
| Group P (2026-11-09) | Pi as in group A, plus Archo's MCP server instructions appended to the system prompt (`--append-system-prompt`), so it sees the same Archo guidance as dsh |
| Group N (Group B before 2026-11-04) | Pi without Archo MCP and without the `archo` CLI. Gets `Player.log` path, the Mod folders and the game folder. The c1 to c5 matrix (A 15/15, N 10/15) settled this comparison; the 2026-11-04 round compares harnesses on h1, h2 and h5 only |
| Prompt | "The game logs errors / misbehaves with this Mod list. Find the Mod that causes it, then fix it. Hand in the rewritten XML or the recompiled DLL." No hint about the fault class |
| Repeats | at least 3 runs per task per group (DeepSeek is stochastic) |

## Isolation

Each run starts from a clean image: the VM snapshot (preferred, no evidence can leak between runs) or, on the host, a fresh copy of the case directory plus an empty `%APPDATA%\RimArcho` and empty `Docs\`. Never run B after A in the same writable state: A leaves `archo.db`, Source-library decompilations and notes.

The VM image has .NET SDK 8, `ilspycmd` 11.2.0.10050, Node 26, Pi, dsh and Git for Windows (snapshot `RimArcho-Eval-Staged5` = Staged4 plus ilspycmd 11.2, 2026-11-08; Staged4 = Staged3 plus Node 26.13.0 and dsh in `C:\Tools\npm`, 2026-11-04, with ilspycmd 9.1.0.7988; see Harness status).

## Cases (built 2026-11-02)

Base environment: 34 of the 35 `Test` plan Mods (Archo Monitor removed for both groups: its `ArchoMonitor.log` is an Archo analysis product). Every case adds or changes only the files named below, and has a real 1.6.4871 `Player.log` written by the real game (host sandbox, `scripts/mcp-eval/`). `node scripts/mcp-eval/cases.mjs` generates `D:\RimArcho-test\mcp-eval\cases\<id>\`: `case.json`, `overlay/`, `Player.log`, `answer.json`, `answer/` (a correct fix) and `reference.Player.log` (the log with that fix).

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| c1 | XML, wrong class (easy) | Outfit Stands Plus: `thingClass` of the first Mending stand reads `OutfitStandsPlus.Thing.MendingOutfitStand` | `Could not find a type named ...`, `Config error in OutfitStandsPlus_MendingOutfitStand` | Outfit Stands Plus | `Defs/ThingDefs/Building_MendingOutfitStand.xml` line 37 restored |
| c2 | Patch vs load order (2 Mods) | Added real Mod "1trickPwnyta's Odyssey Patch" listed before Outfit Stands Plus; Outfit Stands Plus' first patch matches the stand by `thingClass`, which the Odyssey Patch rewrites | `[Outfit Stands Plus] Patch operation ...(thingClass="Building_OutfitStand")/comps failed`; the Odyssey Patch also logs unrelated texture noise (decoy) | Outfit Stands Plus (failing patch); the Odyssey Patch is the cause | match by `defName` again, or make Outfit Stands Plus load first |
| c3 | C#, Harmony target gone (DLL) | Added real Mod Smart Firefight; its `GenSpawn.Spawn` patch lists 6 parameter types, 1.6 has 7. The Mod ships its source; the DLL is rebuilt from the edited source | `Error in static constructor of SmartFirefight.Startup ... Patching exception ... Undefined target method` | Smart Firefight | rebuilt `1.6/Assemblies/SmartFirefight.dll` with the 7-parameter overload (`answer/`) |
| c4 | Missing dependency | Vanilla Expanded Framework installed but not enabled | about 1,700 log lines: VEF types not found, `ReflectionTypeLoadException` in Vanilla Furniture Expanded, many `Exception loading def` | Vanilla Expanded Framework (disabled) | `ModsConfig.xml` with VEF enabled |
| c5 | Wrong blame | 10x Carrying Capacity gets a "compat" patch that rewrites the research prerequisite of Outfit Stands Plus' stands with a misspelled name | `Could not resolve cross-reference to ResearchProjectDef named OutfitStandsPlus_MechanizedOutfitStandResearch` (looks like an Outfit Stands Plus problem) | 10x Carrying Capacity (decoy: Outfit Stands Plus) | remove or fix the Operation (`...StandsResearch`) |

The multi-version layout of the real Mods stays as it is; c3 and c5 need the agent to edit the folder the game really loads.

### Harder cases (built 2026-11-04)

c1 to c5 all name the culprit in the error line, or one hop from it; group A solved 15 of 15 (2026-11-04 matrix). The harder cases follow fault classes that the public 1.6 logs (40,326, private corpus; aggregate figures only) rarely attribute to their source Mod. Built with `node scripts/mcp-eval/cases.mjs h1 h2 h5`; each was reproduced in the real 1.6.4871 game (host sandbox), and its reference fix removes every injected error line.

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| h1 | Cascade from one patch | Vanilla Furniture Expanded's `1.6/Patches/Core.xml` adds `VanillaFurnitureEC.StatParts_Bin` (should be `StatPart_Bin`) to the vanilla `Beauty` StatDef. The type does not resolve, so 1.6 drops the whole StatDef (same shape as the real ReGrowth logs, `Exception loading def from file Biomes_*.xml`) | 1,618 lines: the first error names the vanilla file `Stats_Basics_General.xml` and no Mod; then 1,119 `Could not resolve cross-reference` lines, `Failed to find RimWorld.StatDef named Beauty` and dozens of `Config error in <vanilla Def>: defines the stat base more than once` | Vanilla Furniture Expanded (decoy: Vanilla Expanded Framework) | the class name restored, or the operation removed |
| h2 | Same-name assembly | The Odyssey Patch (listed before Outfit Stands Plus) ships a stale `1.6/Assemblies/OutfitStandsPlus.dll`, built from Outfit Stands Plus' own source minus the Mending and Mechanized stands (a referenced DLL copied on build). Mono's `Assembly.LoadFrom` gives Outfit Stands Plus the already loaded assembly of that name | `Could not find a type named OutfitStandsPlus.Things.MendingOutfitStand` (and 3 more stands) plus 16 `Config error` lines, although Outfit Stands Plus' own DLL has those types; the Odyssey Patch's texture noise (as in c2) | the Odyssey Patch (decoy: Outfit Stands Plus) | the stale DLL deleted (`delete.json`) or replaced, or Outfit Stands Plus listed first |
| h5 | Two faults at once | c1 and c5 together | both sets of errors; c5's errors name Outfit Stands Plus, which also has c1's real fault | Outfit Stands Plus and 10x Carrying Capacity | both fixes, each in its own Mod |

Findings made while building them:

- 1.6 looks up XML class names ignoring case (`GenTypes.GetTypeInAnyAssemblyRaw` calls `GetType(name, false, ignoreCase: true)`), so a case-only typo such as `VanillaFurnitureEc` loads cleanly. The ReGrowth logs' `RegrowthCore.BiomesKitControl` failure is therefore not the case difference itself.
- A missing class in a list item fails the whole Def in 1.6 (`DirectXmlToObjectNew.ResolveTypeForNode` throws), not just the item as the older `DirectXmlToObject` path would.

Pilot `h1-B-0` (dsh + Archo, 20 min limit, Staged4): culprit and fix right in 456 s, 49 model calls, 71 tool calls (Archo 23, `pwsh` 35, no `test_run`), 3.15 M tokens (96% cache reads). It confirmed the typo against the Mod's own `1.5/Patches/Core.xml`, which still has the right name: the multi-version layout leaves a reference copy in h1.

### Round 2: Pi vs dsh, both with Archo (2026-11-04)

18 runs (h1, h2, h5 × A = Pi, B = dsh × 3), 20 min limit, three VMs in parallel (`matrix.ps1`, 81 min), archo built from 5ae03605. Cost at CNY 0.5 / 1.0 / 0.01 per Mtok (input / output / cache read).

| Group | Solved (culprit and fix) | Mean s (median) | Model calls | Tool calls (Archo) | Tokens | Cache hit | Cost per run |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A Pi | 7/9 | 396 (301) | 32.7 | 43.0 (7.9) | 1.17 M | 95.6% | ¥0.047 |
| B dsh | 9/9 | 374 (328) | 32.0 | 46.3 (14.8) | 1.66 M | 95.8% | ¥0.066 |

| Case | A Pi | B dsh |
| --- | --- | --- |
| h1 cascade | 3/3, 294 s, Archo 11.0 | 3/3, 314 s, Archo 15.7 |
| h2 same-name DLL | 2/3, 620 s (one timeout), Archo 5.0 | 3/3, 381 s, Archo 7.0 |
| h5 two faults | 2/3, 274 s, Archo 7.7 | 3/3, 429 s, Archo 21.7 |

The two failures are both Pi's:

- **h2-A-2:** Pi followed the Odyssey Patch's texture noise (the decoy) and ran `grep -r` and `du` over the guest's whole Workshop folder (about 15 GB of subscribed Mods). The call outlived the 20 min limit.
- **h5-A-2:** Pi fixed both faults but wrote them to `C:\Eval\work\submit` (relative to its working folder), then reported `C:\Eval\submit`, so nothing was handed in.

dsh calls Archo about twice as often (14.8 against 7.9 per run) and uses 40% more tokens. Both groups ran `test_run` about once per run.

With 3 runs per cell, the 9/9 against 7/9 difference is within chance. Accuracy does not separate the harnesses yet; the integration differences listed under Fixed conditions (server instructions, sandbox, runtime) weigh more.

Fixes and findings from the round:

- **Grader, BOM:** dsh writes JSON through Windows PowerShell 5, which adds a BOM and makes a one-item list a bare string. `grade.mjs` and `sandbox.mjs` now accept both. h1-B-1 was re-scored from its log; h2-B-3 was graded again after a crash.
- **Leak:** agents can read their own transcript at `C:\Eval\out\transcript.jsonl` (h2-A-2 grepped it). No result depends on it; move the output out of the guest's reach before the next round.
- **h1 shortcut:** its `1.5/Patches/Core.xml` keeps the right class name, which some runs used to confirm the typo.

The submission format gained `delete.json` (files to remove, `<packageId>/<path>`), which h2's root fix needs; `task.md` describes it for every case. The time limit in `task.md` now follows `run-case.ps1 -Minutes`. Grading rules: h2 accepts a change under the Odyssey Patch's `1.6/Assemblies/` (a deletion counts) or `ModsConfig.xml`, with both Mods still enabled; h5 needs a change in each of the two Mods (`touchAll`).

### Runtime case h7 (built 2026-11-06)

h1 to h5 are all startup errors. h7 is the first error that appears only while the game runs, and its stack names no culprit. It comes from a real fix the user made in June 2026 (repro notes outside the repo: `D:\RimArcho-test\ssr-body-repro`). The real June trigger was never found, so the case injects one.

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| h7 | Runtime, a comp changes its own list mid-tick (no culprit frame) | [SSR]Beautiful Body (`ssr.body`, in the base list; the case uses the Workshop copy turned back into the June 2026 version, see VM below) gets its original `1.6/Assemblies/FS_SSR_Body.dll` back (`FS_SSR_Body.pre-codex-backup.dll`). Its comp `TC_ForceBodyShape` on `Human` swaps itself to the end of `AllComps` after 300 ticks and then removes itself during `CompTick`. Added trigger: Pawn Comp Profiler (`tallowbyte.pawncompprofiler`, a small Mod of ours with source, `scripts/mcp-eval/mods/pawncompprofiler`), which ticks every pawn's comps with `foreach` and times each one. Vanilla's `ThingWithComps.Tick` indexes with a cached count, so SSR alone logs nothing | On a map only (about 200 ticks in, in the repro): 23 × `Exception ticking <pawn> (at (x, 0, z)): System.InvalidOperationException: Collection was modified` (2 per colonist, 1 per world pawn, `Suppressing further errors`). Every stack goes through `PawnCompProfiler.ThingWithComps_Tick_Patch.Prefix`, and through a second, unrelated `Pawn.Tick` prefix from the base list (`Palpha.godhands`). No SSR frame; SSR appears only in the `Initializing new game with mods` list | [SSR]Beautiful Body or Pawn Comp Profiler (two right answers since 2026-11-08, see Round 4) | `FS_SSR_Body.dll` rebuilt so the comp never changes `parent.AllComps` (reference: the user's fixed DLL in `answer/`), or `PawnCompProfiler.dll` rebuilt with vanilla's index loop over a cached count. Either Mod turned off does not count |

Harness: `case.json` has `runtime: {ticks: 3000}`. `sandbox.mjs` then sets the sandbox Prefs' `autosaveIntervalDays` to `ticks / 60000`, starts the game with `-quicktest` (a new map at normal speed), and stops it once `Saves/Autosave-1.rws` is written: a vanilla signal that the map ran 3000 ticks, without a helper Mod in the log or the Mod list. A buggy or fixed run takes about 100 s. `grade.mjs` fails a runtime submission whose map never reached the autosave, and the h7 rule requires a change under `ssr.body/1.6/Assemblies/` or `tallowbyte.pawncompprofiler/1.6/Assemblies/` with both Mods still enabled (until 2026-11-08 only SSR's). The case overlay leaves the backup DLL and the 26 MB `_2985408740_.zip` out of SSR's folder (overlay `omit`). Since 2026-11-06, `sandbox.mjs` also links the unchanged folders of a changed Mod as junctions instead of copying the Mod; SSR has about 270 MB of textures.

Checked on the host (real 1.6.4871, 2026-11-06). The buggy runs gave 23 tick errors twice, with different random pawns each time. The reference fix gave none, with no other new error line either: the quicktest map's noise was the same in both runs. Grading results: the reference answer through `grade.mjs` gets culprit and fix. The buggy DLL with only the decoy named gets neither (`decoyNamedOnly`). A change in the profiler alone, on a clean log, is "workaround elsewhere".

VM (2026-11-08). The guest has only the Workshop copy of SSR, the 2026-09 rewrite without the comp. So h7 now starts from the Workshop copy on the host too (`add` with `replace: true` swaps the base entry's folder), and the overlay turns it into the June version: `About.xml`, `Patches/Patches.xml` (the comp) and the 1.4 to 1.6 DLLs are the only files that differ apart from textures, and the June code names no texture. The logs were recorded again this way: the same 23 errors buggy, none fixed, and no texture error from the Workshop textures. `package.mjs` packs Mods the guest's library lacks (`cases/_mods/*`, Cyanobot's Genes) into `pkg/<case>/lib`; `guest/overlay.ps1` copies them into `C:\Eval\lib`. `logdiff.mjs` now skips `SteamAPI.Init() failed`, which comes and goes with the host's Steam client and would fail a correct fix.

`test_run map=true` (archo 1.0.7, a3a53af7, Eval-3, through Pi's MCP client): status `map` after 126 s, and the report has the h7 errors as two `harmony-stack` clusters (3 colonists, 15 world pawns), attributed to Pawn Comp Profiler and PA's God Hands (`likely`), the two Mods in the stack. So an agent can reproduce h7 with Archo; Archo names only the decoys.

### Runtime case h4 (built 2026-11-08)

From the real 1.6 logs: EcoDialogues builds an interaction log entry, and Cyanobot's Genes' postfix on that constructor throws. The stack's top frame is Cyanobot's Genes; the cause is the caller. EcoDialogues is not on disk, so the caller is our own small Mod of the same shape.

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| h4 | Runtime NRE in one Mod's postfix, caused by another Mod's call (wrong blame) | Added: Cyanobot's Genes (`cyanobot.CyanobotsGenes`, library copy 2889844371, unchanged) and Colonist Mutterings (`mossbrook.colonistmutterings`, ours, with source: `scripts/mcp-eval/mods/colonistmutterings`). About once an in-game hour each colonist "mutters"; the line is a `PlayLogEntry_Mutter : PlayLogEntry_Interaction` built with `recipient = null`. Cyanobot's postfix on the `PlayLogEntry_Interaction` constructor reads `recipient.genes`. Vanilla does not allow a null pawn in that entry (`PlayLogEntry_Interaction has a null pawn reference` when it is shown) | On a map: `[Colonist Mutterings] Error creating mutter: System.NullReferenceException`, once per colonist (3 in the recorded log, one identical line). Top frame `CyanobotsGenes.PlayLogEntry_Interaction_Patch.Postfix`, then the vanilla constructor with Cyanobot's `POSTFIX` line, then `ColonistMutterings.PlayLogEntry_Mutter..ctor`. Cyanobot's Genes also logs 4 missing-texture lines (in the reference too) | Colonist Mutterings (decoy: Cyanobot's Genes) | `PlayLogEntry_Mutter` derived from `LogEntry` (or given a real recipient), `1.6/Assemblies/ColonistMutterings.dll` rebuilt from the shipped source (reference in `answer/`). A null check in Cyanobot's postfix only hides it |

Checked on the host (real 1.6.4871): buggy run with the errors, reference fix with none. Grading results: reference answer through `grade.mjs` gets culprit and fix; Cyanobot's Genes named alone gets neither (`decoyNamedOnly`); a change in Cyanobot's Genes alone, on a clean log, is "workaround elsewhere". Rule: `mossbrook.colonistmutterings/1.6/Assemblies/` must change, both Mods stay enabled.

### Round 3, h4 and h7: Pi vs dsh (2026-11-08)

12 runs (h4, h7 × A = Pi, B = dsh × 3) on Eval-3, 20 min limit, archo 1.0.7 (dev/1.1 a3a53af7, `tools\archo-1.0.7.exe`). Cost as in round 2. The first attempt ran while the model gateway was down (0 tool calls; moved to `results-gateway-down/`).

| Cell | Solved | Mean s | Tool calls (Archo) | Tokens | Cost per run |
| --- | --- | --- | --- | --- | --- |
| h4 A Pi | 1/3 | 539 | 68 (9.7) | 2.55 M | ¥0.080 |
| h4 B dsh | 1/3 | 755 | 91 (18.7) | 5.66 M | ¥0.155 |
| h7 A Pi | 0/3 | 413 | 52 (9.7) | 1.85 M | ¥0.059 |
| h7 B dsh | 0/3 | 505 | 54 (17.0) | 2.24 M | ¥0.087 |

Every run called `test_run` with `map=true`, and got status `map`: Archo's test run reproduces both faults.

Why the 10 failures failed:

- **h4 (4 failures):** all patched Cyanobot's Genes' postfix with a null check (the stack's top frame), and named only Cyanobot's Genes. The logs were clean, so the grade is "workaround elsewhere". Both solved runs fixed Colonist Mutterings' entry; h4-B-1 also patched Cyanobot's Genes and named both Mods.
- **h7 (6 failures):** all rewrote the profiler's `foreach` as an index loop. 5 of 6 found SSR's `TC_ForceBodyShape` and its `AllComps.Remove` / `GenList.Swap`, but judged it allowed: vanilla's cached-count loop tolerates it, so the profiler is the one that departs from vanilla (h7-B-1: "Comps are allowed to modify that list while they tick"). 5 of 6 logs were clean. h7-A-1's log had one extra line, `Object with load ID Thing_Human… is referenced … but is not deep-saved`, from the autosave that ends the map run: a grader flake from the harness, not from the submission.

Regraded 2026-11-08 with both answers accepted (Round 4): h7 is 5/6, Pi 2/3 and dsh 3/3; h7-A-1 still fails on the autosave line.

Case design: the h7 verdict is debatable. That SSR is the culprit rests on "a comp must not change `AllComps` inside `CompTick`", but vanilla's loop is written to survive exactly that, and the agents' reading is reasonable. Either accept a profiler fix as a second right answer (then h7 would be 5/6 on the fix), or make the fault one that vanilla does not survive. h4's verdict has vanilla's own error (`has a null pawn reference`) behind it, and two runs found it.

Archo gaps found (no app code changed):

1. Attribution names only the Mods in the stack. h7: both clusters list Pawn Comp Profiler and PA's God Hands (`likely`), nothing about a third Mod's comp changing the list; Archo cannot know which comps the pawn has. h4: Archo lists Colonist Mutterings and Cyanobot's Genes, both `likely`, without saying which frame is the caller.
2. Harness (not Archo): an autosave-based map run can add a save-time error that the reference lacks; `logdiff.mjs` does not filter it.

### Transpiler conflict h3 and save error h6 (built 2026-11-08)

Both follow the commonest unattributed classes in the public 1.6 logs (private corpus, aggregate figures only). Built with `node scripts/mcp-eval/cases.mjs h3 h6`; each was reproduced in the real 1.6.4871 game (host sandbox), and its reference fix removes every injected error line with no new one.

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| h3 | Bad IL from two transpilers on one method (no Mod named) | Added Humanoid Alien Races (HAR, early in the list) and ReSplice: Charmweavers (last). Charmweavers' DLL, decompiled and rebuilt, gets a transpiler on `Pawn_RelationsTracker.SecondaryLovinChanceFactor`, patched from its `Mod` constructor (so before every static constructor), that routes vanilla's `pawn.def != otherPawn.def` through a helper `RacesDiffer(ThingDef, ThingDef)`. Alone it is valid. HAR's own transpiler, applied after it, turns each `ldfld Thing::def` into `RaceProps.Humanlike` (a bool), so the helper gets two bools and Mono's JIT rejects the method | one error, as in 544 of 6,734 public 1.6 logs, nearly to the byte: `Error during patching RimWorld.Pawn_RelationsTracker :: Single SecondaryLovinChanceFactor(Verse.Pawn) with: pre \| post: ... SecondaryLovinChanceFactorPostfix ... \| trans: ... SecondaryLovinChanceFactorTranspiler`, then `InvalidProgramException: Invalid IL code in ... SecondaryLovinChanceFactor_Patch2 ...: IL_0021: call 0x0000000b`. HAR's `AlienHarmony.Patch` wrapper catches the exception and names only its own patch methods; no Mod, no assembly, no stack beyond Harmony | Charmweavers (decoy: HAR) | Charmweavers' DLL without the transpiler (reference: the Workshop DLL), or a transpiler that does not assume a ThingDef on the stack. A change under `ReSplice.XOTR.Charmweavers/1.6/Assemblies/` with both Mods enabled; changing HAR only hides it |
| h6 | Save-load error, a Def removed by another Mod's patch | Vanilla Hairs and Beards Retextured (base list) gets `Patches/Core_Beards_Patch.xml`, which removes the vanilla beards BushyStyled, SideWhiskers and Seer ("they clip through the retextured heads"). The Mod's own way to hide a style is `styleTags` `IgnoreMe` (its `Anomaly_Hairs_Patch.xml`). New games start cleanly; the player's save (a quicktest colony, 3000 ticks, pristine base list) has pawns with those beards | startup is clean; after `Loading game from file ...` four `Could not load reference to RimWorld.BeardDef named SideWhiskers / Seer / BushyStyled` (one per pawn), naming only vanilla Defs. In the corpus, 6,156 of 40,326 1.6 logs have `Could not load reference to <Def> named`; 29,134 load a save. The usual reading, "the save used a Mod that is gone", is wrong here: the beards are Core's | the retexture Mod (decoy, a natural wrong guess: AFU Men's Hairstyles) | hide the beards instead of removing them (reference: `styleTags` `IgnoreMe`), or drop the operation or the file (`delete.json`). A change under `hourai.vanillahairsretex.1.6/` with the Mod enabled |

Harness for h6: `case.json` `load: {save}` makes `sandbox.mjs` copy the save to `Saves/autostart.rws`. With dev mode on (the pinned Prefs have it), vanilla loads that file at startup (`SaveGameFilesUtility.GetAutostartSaveFile`), so grading loads the save without a helper Mod; the run ends on log silence as usual (about 70 s, the map loaded). The VM gets the log only, no save: archo's `test_run` uses isolated test data and never loads a save, and the guest's Steam account is the player's real one, whose Steam Cloud syncs `Saves/*.rws` (an `autostart.rws` there would load on the player's own PC in dev mode). So in h6 an agent cannot reproduce the error; it has to reason from the log and the Mod files.

Found while building them:

- The real HAR + Charmweavers pair loads cleanly in 1.6.4871: Charmweavers' patch on the method is a plain postfix. Whatever made the corpus logs fail (Charmweavers is in 57% of them and in none of the clean ones) is not in today's versions, so h3 injects the second transpiler.
- Charmweavers patches from a `[StaticConstructorOnStartup]`, as HAR does, and HAR is listed first; a transpiler there would have failed in Charmweavers' own `PatchAll` and named it (`Error in static constructor of ReSpliceCharmweavers.Core`). Patching from the `Mod` constructor is common practice and puts the failure in HAR's call.
- Rebuilding Charmweavers from `ilspycmd` output needs the references at decompile time (`-r`; without them every struct cast comes out as `((T)(ref x))`), a publicized Assembly-CSharp (Krafs.Publicizer; one accessor then has to be fixed by hand), the Harmony Mod's own `0Harmony.dll` (2.4 API; `Lib.Harmony.Ref` 2.3.6 lacks `CodeMatcher.Advance()`), and `net48`. A plain rebuild of the decompiled source grades as a fix (checked), so an agent's decompile-and-rebuild route works.
- The vanilla save compressor (`CompressibilityDecider`) keeps every Thing that a job, reservation, designation or projectile references, so "Was it compressed away" (the commonest load warning) needs a Mod-held reference and a Thing that changes within the test window; not reproducible in a 3000-tick quicktest, so h6 uses the second class.

Grading checks (host, real game): h3 reference answer, and a plain rebuild of the decompiled Charmweavers, get culprit and fix; HAR named with a HAR file changed gets neither (`decoyNamedOnly`). h6 reference answer, and `delete.json` removing the patch, get culprit and fix; AFU Men's Hairstyles named and disabled gets neither.

#### Results: h3 and h6, Pi vs dsh, both with Archo (2026-11-08)

12 runs (h3, h6 × A = Pi, B = dsh × 3), 20 min limit, one VM (Eval-2, `matrix.ps1 -Cases h3,h6`, 03:51 to 06:57, 186 min), archo `tools/archo-1.0.7.exe` built from dev/1.1 a3a53af7 (sha256 `a8380b7b26a049f0…`). Cost at CNY 0.5 / 1.0 / 0.01 per Mtok (input / output / cache read). A first attempt from 03:09 is void: the host port proxy to the model gateway had stopped listening, so every run on every VM ended at its first model call with 0 tool calls (`results-gateway-down/`).

| Group | Solved (culprit and fix) | Mean s (median) | Model calls | Tool calls (Archo) | Tokens | Cache hit | Cost per run |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A Pi | 5/6 | 536 (528) | 42.8 | 50.8 (15.2) | 2.03 M | 96.4% | ¥0.076 |
| B dsh | 6/6 | 518 (517) | 40.5 | 56.3 (23.2) | 2.48 M | 96.4% | ¥0.094 |

| Case | A Pi | B dsh |
| --- | --- | --- |
| h3 transpiler conflict | 2/3, 720 s, Archo 11.7 | 3/3, 586 s, Archo 26.0 |
| h6 save error | 3/3, 352 s, Archo 18.7 | 3/3, 450 s, Archo 20.3 |

All 12 runs named the right culprit. The one failure:

- **h3-A-1:** Pi named Charmweavers and handed in a rebuilt DLL, but also took Charmweavers out of `ModsConfig.xml` ("with Charmweavers out of the active list the start is clean (verified by a test run)"). The grader's rule caught it ("log is clean only because the broken Mod was disabled"). Graded alone (ModsConfig left out), its DLL still logs the same Invalid IL error: the disabled Mod was the only reason the log was clean.

Observations:

- **h3 is the slowest case so far** (653 s mean over both groups; round 2's cases took 294 to 620 s). Every run took the same path: `analyze_log` attributes the Invalid IL cluster to no Mod (`attribution: (no mod identified)`), then `get_harmony` on `Pawn_RelationsTracker.SecondaryLovinChanceFactor` lists the four patches from both Mods, including `..._RaceCheck_Patch.Transpiler`. That one call turns "no Mod named" into two candidates. The rest of the time goes into decompiling and rebuilding Charmweavers (the reference assembly and publicizer issues listed above).
- **h6 is easy with Archo:** `get_def` / `get_patches` on the BeardDef find the `PatchOperationRemove` within a few calls. All six runs fixed it by dropping the operation (one by `delete.json`), none with `IgnoreMe`; all are accepted fixes. Several noticed that the retexture Mod ships textures for exactly those three beards, which contradicts the injected "they clip" comment: a shortcut, like h1's `1.5` copy. No run could reproduce the error (no save in the guest), and none needed to.
- dsh again calls Archo more (23.2 against 15.2 per run) and uses more tokens (2.48 M against 2.03 M). With 3 runs per cell, 6/6 against 5/6 is within chance.
- Archo leads (not bugs): `analyze_log` could attribute an `Error during patching <method> with: ... | trans: <patch method>` line through its own Harmony index (the patch method names map to HAR, and the method's patches name both Mods), which would put Charmweavers in front of the agent without a second call. `test_run` still reports `isError` for the base noise (known, see the c3 pilot). No Archo bug was seen in the 12 runs.

### Runtime lord conflict h8 and no-fault control h9 (built 2026-11-08)

| # | Fault class | Injected fault (real Mods) | What the log shows | Right answer: culprit | Right answer: fix |
| --- | --- | --- | --- | --- | --- |
| h8 | Runtime, wrong blame (two Mods' lord jobs) | No file is changed. Progression: Education (`ferny.ProgressionEducation`, Workshop release of 2026-09-07) lacks a `GetLord()` check where `LordToil_AttendClass` re-adds an "orphaned" student to the class lord (the user's PR fixes it). The case adds it, Intimacy - Friends n' Lovers and Intimacy - Socio Butterfly to the base list and ships a prepared save: a classroom in session (board, two desks, bell, a Social class with 2 students) while the students are in a Socio Butterfly `LordJob_Birthday` | 298 identical lines in 4000 ticks: `Tried to add pawn <name> to lord with lord jobProgressionEducation.LordJob_AttendClass but this pawn is already a member of lord with lord jobRecreationalSexWithEuterpe.LordJob_Birthday. Pawns can't be members of more than one lord at the same time.` No stack in Player.log. Both Mods are named | Progression: Education (decoy: Socio Butterfly, whose lord it is; Friends n' Lovers) | `LordToil_AttendClass.LordToilTick` skips a student with `GetLord() != null`; rebuild `1.6/Assemblies/ProgressionEducation.dll` (it ships its source in `1.6/Source`). Reference build in `answer/` |
| h9 | No fault | Nothing: the base list (34 Mods) | The base list's normal noise: no error line; "Translation data for language English has 19 errors" and the game's own "red text" joke | none | answer.json with an empty culprit list and no files |

h8 is the first case whose trigger is a game state, not a Mod: `case.json` `runtime: {ticks: 4000, save: 'save/autostart.rws'}`. `sandbox.mjs` copies the save to the sandbox as `Saves/autostart.rws`; the pinned Prefs have dev mode on, and the game loads that file at start (`SaveGameFilesUtility.GetAutostartSaveFile`) instead of a `-quicktest` map. It stops at the first autosave, 4000 ticks later (about 100 s). `make-pe-save.mjs` builds the save once from a throw-away helper Mod (`mods/pe-setup`): it spawns the lesson stone, desks and bell, creates the class through PE's own classes, starts a `LordJob_Birthday` with the two students, saves and quits. The helper is removed from the save's Mod list and component list afterwards, so the game loads it without a warning. Findings while building it:

- PE's Workshop DLL and Workshop source agree (both lack the guard); a Skill class needs workbenches unless its skill is taught at the plain school desks (Social works, Crafting did not validate). The board opens a rename dialog that pauses the game: the helper closes it.
- The class lord needs no extra push: the teacher rings the bell, then every tick the attend toil re-adds each idle student who is not in the class lord, so the error repeats. The first run reproduced the real log line exactly.
- The line has none of `logdiff.mjs`'s old error words, so the grader would have called both logs clean: `Tried to add pawn` is now in its pattern.
- In the VM the guest's Workshop copies of the three Mods are the same versions as the host's (`ProgressionEducation.dll` 222,720 bytes), so the case has no overlay. The agent has no save to load in the VM (`test_run` starts a new map, where no class exists): h8 can only be solved by reading, which PE's shipped source allows.

Grading. h8 (`RULES`): both PE and Socio Butterfly stay enabled and the change lands in `ferny.ProgressionEducation/1.6/Assemblies/`; the run must reach the autosave. h9 (`control`): culprit = answer.json present with an empty list; fix = no file, deletion or list change. Naming any Mod or changing anything is `falsePositive`; a missing answer.json is a miss (`answered: false`). `task.md` is unchanged, so it does not say that a case can be fault-free. Checked on the host: the h8 reference answer gets culprit and fix (clean log, autosave reached); only the decoy named gets neither (`decoyNamedOnly`); an empty h9 answer gets both; an h9 answer with a file and a named Mod gets neither and `falsePositive`.

#### Results: h8 and h9, Pi vs dsh, both with Archo (2026-11-08)

12 cells (h8, h9 × A = Pi, B = dsh × 3), 20 min limit, Player-Test only (`matrix.ps1`, about 4 h 45 min), archo 1.0.7 (`tools/archo-1.0.7.exe`, built from dev/1.1 a3a53af7, sha256 `a8380b7b…ffa90`). Cost at CNY 0.5 / 1.0 / 0.01 per Mtok (input / output / cache read). h9 counts as solved only with an `answer.json` that has an empty culprit list and no changed file.

| Group | Solved | Mean s | Turns | Tool calls (Archo) | Tokens | Cost per run |
| --- | --- | --- | --- | --- | --- | --- |
| A Pi | 2/6 | 1018 | 76.0 | 85.0 (13.3) | 5.23 M | ¥0.141 |
| B dsh | 1/6 | 1031 | 75.3 | 92.2 (27.5) | 7.76 M | ¥0.194 |

| Case | A Pi | B dsh |
| --- | --- | --- |
| h8 PE lord conflict | 2/3, 1037 s, Archo 11.3, ¥0.113 | 1/3, 948 s, Archo 28.7, ¥0.200 |
| h9 no fault | 0/3 (2 false positives, 1 timeout), 999 s, Archo 15.3, ¥0.169 | 0/3 (2 false positives, 1 timeout), 1113 s, Archo 26.3, ¥0.188 |

h8 per run (culprit named / fix): A1 no/no (1200 s timeout, nothing submitted); A2 yes/yes (1010 s); A3 yes/yes (902 s); B1 yes/yes (1200 s limit reached, submission graded); B2 yes/no (953 s); B3 yes/no (692 s).

- **Culprit.** All 5 agents that submitted named Progression: Education; none was fooled by the decoys (Socio Butterfly, Friends n' Lovers). Pi A2 also named Socio Butterfly. The log line names both lord jobs and PE ships its source, so reading was enough.
- **Why the fix failed (B2, B3).** Both rebuilt `ProgressionEducation.dll` from `1.6/Source` without Krafs.Publicizer; the game then logs `FieldAccessException: Field RimWorld.StatWorker:stat is inaccessible` and the map never reaches the autosave. A3 rebuilt it correctly; A2 shipped a small separate DLL instead; B1 shipped a patch DLL (`EducationLordFix.dll`). Archo's `read_source` does not tell an agent how a Mod's DLL is built.
- **A1** spent 73 `bash` calls rebuilding and diffing the DLL (a Debug build against the Workshop one) and ran out of time without writing a submission.
- **No agent could load the save.** `list_saves` was called in every run, but `test_run` only starts a new map, where no class exists. `test_run` was used 1 to 2 times per h8 run and could not show whether the fix works.

h9 per run: A1 timeout, no answer; A2 changed `Palpha.godhands` translation files; A3 changed translation files of two Mods; B1 disabled Beheading Dolls in `ModsConfig.xml`; B2 timeout, no answer; B3 changed translation files of two Mods. No agent answered "nothing to fix".

- Every agent that answered chased noise present in the base list: the six Unity `_MainTex` material errors and the 19 "Translation data for language English" errors. The task prompt says there are errors to fix; that is the pressure. All four answers broke nothing, but each is a false positive by the control's rule.
- The control shows an agent will not volunteer "no fault" without a hint. A task line such as "the log may be fine" would change the case, so it was not added. Whether to count translation-key cleanups as harm is the owner's call (they cost a run's time, not the user's game).

Void cell: h9-A-2 first ended at 489 s on four `521` errors from the model provider (gateway origin down), so it was moved to `results-gateway-down/` and run again (775 s, above). An earlier attempt of all cells had been voided by the gateway outage (0 tool calls, "Request timed out"); those results are also in `results-gateway-down/`.

Archo findings:

- **No Archo defect was hit in these 12 runs.** One tool error appears: `analyze_log` answered "no Player.log found for 1.6 under C:\Users\…\LocalLow\…" in h9-A-1 (5 times) on a guest where the game had not yet been run. The message is the CLI's (`archo log --file <path>`); check that an MCP agent can act on it.
- **Gap, not a bug:** no tool starts a given save, so a runtime fault that needs a game state (h8) cannot be reproduced or verified through Archo. Likewise nothing tells an agent a Mod's DLL needs publicized references.
- dsh calls Archo about twice as often (27.5 against 13.3 per run) and uses about 50% more tokens, as in round 2; neither group is more accurate here (3 runs per cell).

### Round 4, h4, h7 and h8 rerun with the improved tools (2026-11-08)

18 runs (h4, h7, h8 × A = Pi, B = dsh × 3) on Player-Test, Eval-2 and Eval-3 (snapshot `RimArcho-Eval-Staged5`, ilspycmd 11.2), 20 min limit, archo from dev/1.1 e2fa7445: comp-list modifiers and `listed` attribution, TargetMethod() targets, the null trace, source search coverage and member view, `test_run` with saves, and the other changes from the round 3 transcript reviews. Round 3 figures in brackets (h8: the round 2 h8 cells).

h7 grading changed in this round: the profiler's `foreach` departs from vanilla's cached-count loop, which tolerates a comp that removes itself, so a profiler fix is a right answer too (`answer.json` `requiredAny`, `grade.mjs` h7 rule). Both rounds' h7 runs were regraded from their saved logs (`GRADE_REUSE_LOG=1`).

| Cell | Solved | Mean s | Archo calls | Tokens |
| --- | --- | --- | --- | --- |
| h4 A Pi | 2/3 (1/3) | 523 (539) | 16.0 (9.7) | 2.75 M (2.55 M) |
| h4 B dsh | 3/3 (1/3) | 677 (755) | 21.0 (18.7) | 2.90 M (5.66 M) |
| h7 A Pi | 3/3 (2/3 regraded) | 267 (413) | 10.7 (9.7) | 0.62 M (1.85 M) |
| h7 B dsh | 3/3 (3/3 regraded) | 283 (505) | 13.7 (17.0) | 0.89 M (2.24 M) |
| h8 A Pi | 2/3 (2/3) | 925 (1037) | 10.0 (11.3) | 3.97 M (4.00 M) |
| h8 B dsh | 2/3 (1/3) | 545 (948) | 28.7 (28.7) | 5.40 M (8.45 M) |

Failures:

- **h4-A-2:** named and patched Cyanobot's Genes (the decoy at the stack top), as the round 3 failures did.
- **h8-B-2:** named both PE and Recreation With Euterpe; its rebuilt PE DLL left `Exception in Verse.AI.ThinkNode_Priority TryIssueJobPackage: System.NullReferenceException` in the log.
- **h8-A-3:** reached the 1200 s limit with nothing submitted.
- **h7:** all six named Pawn Comp Profiler and rewrote its loop as an index loop over the comps, rebuilt from its source; every log was clean. The list-modifier output named SSR's comp, and the agents judged the profiler the one that departs from vanilla.

h7 and dsh's h4 and h8 runs are much cheaper than in round 3 (h7 about a third of the tokens and two thirds of the time).

### Round 5, group P: Pi with Archo's server instructions (2026-11-09)

Question: is dsh ahead because it passes Archo's MCP server instructions to the model, which Pi 1.4 drops? Group P is Pi as in group A plus those instructions in the system prompt: `guest/agent.ps1` asks the run's `archo.exe` for them (`initialize`) and passes the file with `--append-system-prompt` (saved per run as `archo-instructions.md`, 1,949 characters). 9 runs (h4, h7, h8 × 3) on the three VMs, 20 min limit, the same build as round 4 (`tools/archo-e2fa7445.exe`), graded with the same rules (h7 accepts either Mod). Columns A and B are round 4's.

| Case | A Pi | P Pi + instructions | B dsh |
| --- | --- | --- | --- |
| h4 | 2/3, 523 s, Archo 16.0, 2.75 M | 2/3, 740 s, Archo 12.7, 1.08 M | 3/3, 677 s, Archo 21.0, 2.90 M |
| h7 | 3/3, 267 s, Archo 10.7, 0.62 M | 3/3, 384 s, Archo 14.7, 0.81 M | 3/3, 283 s, Archo 13.7, 0.89 M |
| h8 | 2/3, 925 s, Archo 10.0, 3.97 M | 1/3, 440 s, Archo 17.7, 3.14 M | 2/3, 545 s, Archo 28.7, 5.40 M |
| All | 7/9 | 6/9 (culprit 8/9) | 8/9 |

Failures:

- **h4-P-3:** 20 turns, then `find / -name "Mono.Cecil*.dll" -maxdepth 6` over the guest's disk, which ran past the time limit (no answer). Pi's bash tool has no default timeout; round 2's h2-A-2 ended the same way.
- **h8-P-2, h8-P-3:** named PE and rebuilt `ProgressionEducation.dll` with Krafs.Publicizer (both found `PublicizedAssemblies/`), but the game logs `FieldAccessException: Field RimWorld.StatWorker:stat is inaccessible from method ProgressionEducation.StatWorker_IsDisabledFor_Patch:Postfix` at startup and the test map fails ("Error while generating a map"). The Workshop DLL, built from the same csproj, does not throw; why the agents' builds do is not yet known. No P run called `guide topic=rebuild`.

Reading: the instructions do not close the gap. P solves no more than A (6/9 against 7/9, within chance) and names the culprit as often as dsh (8/9 against 9/9). Its failures are a hung shell command and two DLL builds, not diagnosis. They cut Pi's tokens on h4 and h8 (1.08 M against 2.75 M; 3.14 M against 3.97 M) and raise its Archo calls on h7 and h8. Over rounds 2 to 5 dsh is ahead mainly on execution: bounded commands and a fix that runs. For an Archo Agent that points at dsh as the base (MIT, so it can ship with RimArcho), with Archo's instructions given to whichever harness is used.

## Grading

`scripts/mcp-eval/logdiff.mjs` compares a run's error lines (paths and addresses normalised) with the case's `reference.Player.log`. A submission is applied as a layer over the case (`sandbox.mjs <case> --layer <dir>`; files at `<packageId>/<mod-relative path>`, `enable.json` for re-enabled Mods), the game starts, and the log must have no error line the reference fix lacks. Culprit scoring uses `answer.json`: every Mod in `required` must be named, or, where the key has `requiredAny` (h7), every Mod of one of its lists; naming only a `decoy` fails.

## Scoring

1. **Culprit**: correct Mod named (0/1; c2 needs the Mod whose patch fails, naming the cause is a bonus; c5 fails if only the decoy is named).
2. **Fix**: the submitted files applied as above, then one game start; no error line beyond the reference (0/1).
3. Cost: turns, tool calls, tokens, wall clock.
4. Group A only: tools used, calls that returned nothing useful, and where the agent fell back to bash. This drives the MCP changes.

## Harness status (2026-11-02)

- VM snapshot `RimArcho-Eval-Staged3` (UID 11, live, `NewPlayer(Chinese)` signed in on the desktop; Staged2, UID 10, without autologon, is its parent): game 1.6.4871 junctioned into `C:\Eval\RimWorld`, non-Workshop Mods in `C:\Eval\lib`, Archo build in `C:\Eval\tools`, Git for Windows (the shell Pi's `bash` tool needs), `ilspycmd` on the machine PATH. Each run reverts to it; see `scripts/mcp-eval/README.md`.
- Current snapshot `RimArcho-Eval-Staged5` (2026-11-08, on Player-Test, Eval-2 and Eval-3; `run-case.ps1` reverts to it): Staged4 (Node 26.13.0 and dsh) with `ilspycmd` 11.2.0.10050 instead of 9.1.0.7988. The 11.2 build is the self-contained `win-x64` single exe (22 MB, runs without .NET), so `C:\Tools\ilspycmd` holds that one file; `scripts/mcp-eval/setup-ilspy-vm.ps1` built it from Staged4. Runs before 2026-11-08 used 9.1 and later runs use 11.2, so decompiler output may differ between rounds. `Verse.IntVec3` from the guest's `Assembly-CSharp.dll` was decompiled to stdout and to a file as the administrator and as the standard user, in session 0 and on the desktop. Setup scripts wait for the guest with a probe command: `vmrun checkToolsState` says "installed" for these encrypted VMs even with the tools running.
- Pi reaches the model through a host port proxy 192.168.86.1:3425 (netsh portproxy + firewall rule "RimArcho-eval-VM-to-model-gateway"); remove both after the evaluation.
- Pi 1.4.0 has native MCP: group A registers `archo mcp` with exposure `direct`; group B runs with `--no-mcp` and has no `archo.exe` on disk.
- Staging hides the answer: `guest/overlay.ps1` writes the faulty files into the Mod's real folder with the replaced file's timestamps, every Mod in `Mods\` is a junction, and the staging package and scripts are deleted before Pi starts. `run-case.ps1` holds a mutex so two runs never share the VM.

Harness defects found by the pilots and fixed (earlier pilot results are void and kept only under `D:\RimArcho-test\mcp-eval\results-*`):

1. Pi's `bash` tool found no shell in the VM, so no run could use `dotnet` or `ilspycmd` (the c1 pilot included). Git for Windows is now in the image.
2. The fix check accepted "disable the broken Mod" and workarounds in another Mod. `grade.mjs` now also requires the culprit to stay enabled and the change to land where the fault is (c3: a rebuilt DLL); `fixClean` keeps the plain log result.
3. Culprit scoring read only the `culprit` list of `answer.json`. A right explanation now counts too (`culpritVia: explanation`, checked by hand).
4. The agent could read the staging package (`C:\Eval\pkg\<case>\overlay`, `manifest.json`) and see that the broken Mod was the only folder that was not a junction. Three runs did. Fixed as described above.
5. Two pilots once ran at the same time and reverted the VM under each other. Fixed by the mutex.
6. Pi ran in session 0 (vmrun without `-interactive`), so any game the agent started, through `test_run` or by hand, had no display and hung at `Screen: DX11 could not switch resolution` (peak 128 MB, timeout after 600 s). The same game in the desktop session reaches the main menu in 42 s (2.4 GB). `run-case.ps1` now starts Pi with `-interactive`, which needs `NewPlayer(Chinese)` to be the signed-in user. Snapshot `RimArcho-Eval-Staged3` (UID 11, live) adds autologon for `NewPlayer(Chinese)` and was taken with it signed in. Every pilot before this fix (both groups) ran without a working game.

### Pilot, case c1 (before fixes 1 to 4; to redo)

Both groups found Outfit Stands Plus and fixed the file; neither had a working shell.

| Group | Time | Tool calls | Tokens (incl. cache reads) | Culprit | Fix |
| --- | --- | --- | --- | --- | --- |
| A (Archo MCP) | 225 s | 44 (Archo tools 27: search_files 13, analyze_log 4, read_source 3, status 2, ...) | 1.15 M | yes | yes |
| B | 337 s | 56 (read 47, bash 4, write 4, edit 1) | 0.83 M | yes | yes |

First MCP findings from the pilot: `test_run` hit Pi's 60 s MCP request limit ("MCP request timed out after 60000ms"), so a long tool needs progress or an async start/poll shape; after that call `analyze_log` answered "no Player.log found" for the same environment; `search_files` was called 13 times.

### Pilot, cases c3 to c5, leak-free (one run per group, 2026-11-02)

| Case | Group | Time | Turns | Tool calls | Tokens (incl. cache) | Culprit | Fix |
| --- | --- | --- | --- | --- | --- | --- | --- |
| c3 | A | 884 s | 47 | 56 (bash 47, Archo 8) | 2.20 M | yes | yes (rebuilt DLL) |
| c3 | B | 694 s | 55 | 68 (bash 66) | 2.72 M | yes | yes (rebuilt DLL) |
| c4 | A | 657 s | 65 | 83 (bash 72, Archo 10) | 4.10 M | yes | yes |
| c4 | B | 900 s (limit) | 108 | 116 (bash 116) | 10.80 M | no (nothing submitted) | no |
| c5 | A | 407 s | 37 | 53 (bash 44, Archo 7) | 1.85 M | yes | yes |
| c5 | B | 900 s (limit) | 74 | 103 (bash 102) | 3.47 M | no (`answer.json` never written) | yes |

Observations (one run per cell, so only leads):

- Group A solved all three; group B ran out of time in c4 and c5. In c4 (about 1,700 log lines) B spent 10.8 M tokens reading the log with bash and submitted nothing; A started from `analyze_log` / `list_problems`.
- With a working shell, group A uses the MCP lightly: 7 to 10 Archo calls per run, mostly `status`, `analyze_log`, `list_problems`, then bash for everything else (reading files, `ilspycmd`, `dotnet build`).
- `test_run` hit Pi's 60 s MCP request limit in every A run (9, 5 and 5 timeout messages), so the agent never saw a game-start result from it. On timeout Pi cancels the request and the cancel closes the game, so every retry started over. See "test_run fix" below.
- The c1 pilot above predates fixes 1 to 4 and must be redone.

### test_run fix (2026-11-02)

Two causes, both needed for a working `test_run` in the eval:

1. **Archo (fixed, branch `claude/test-run-progress`).** Pi 1.4 asks for progress on every MCP call and restarts its 60 s timer on each progress notification. `archo mcp` now sends one every 10 s while a call runs, with the current step (14 §1). Checked in the VM with Pi's own MCP client (`guest/probe-testrun.mjs`, no model): the call stayed open for 600 s with 60 notifications and returned the report.
2. **Harness (fixed).** The game hung in session 0 (defect 6). `run-case.ps1 -Archo <exe>` now pushes the evaluated build and starts Pi with `-interactive` in `NewPlayer(Chinese)`'s desktop (snapshot Staged3).

End to end with both fixes (c3 staged, Pi's MCP client, no model): `test_run` returned `menu` after 93 s (load 84 s, peak 4.3 GB, 9 progress notifications) with the injected fault as an `exception` cluster: `Error in static constructor of SmartFirefight.Startup … Undefined target method`.

Pilot with both fixes, c3 group A (one run, `results/c3-A-1`): 508 s, 35 turns, 54 tool calls (bash 49, `status` 1, `test_run` 2), 1.33 M tokens; culprit yes, fix yes (rebuilt DLL, log clean). After copying its rebuilt DLL into the Mod, the agent verified with `test_run` (menu, 89 s) and `test_run map=true` (map, 85 s); both returned full reports. Both reports set `isError`, because the base noise errors stay (expected; 20 §2 counts every error cluster), so the agent still grepped the run's Player.log to see that its own error was gone.

MCP lead from that report: the cluster lists no Mod (`mods: []`, `possible`) although `SmartFirefight.Startup` is a type in Smart Firefight's own assembly; the missing `Designator_ExtinguishFires` texture is also unattributed. Attributing a type named in the message to the Mod whose assembly defines it would name the culprit directly.

## Open items

- Redo the pilots: none before defect 6 was fixed had a working game (results moved to `results-session0`); c3 group A is redone above.
- MCP leads so far: attribute a type named in an exception to the Mod whose assembly defines it; let `test_run` compare against a baseline run (or the previous run) so a fix shows as "error gone" instead of a failing exit caused by unrelated noise.
- Full matrix: 5 cases x 2 groups x at least 3 runs (7 to 15 minutes each plus 2 minutes of grading, one VM, so about 8 hours).
- Grading starts the real game on the host (a window, about 2 minutes per run); consider grading inside the VM.
- Review whether the 15-minute limit is right: two of three B runs hit it.
