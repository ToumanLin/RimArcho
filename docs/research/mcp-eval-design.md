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
| Group N (Group B before 2026-11-04) | Pi without Archo MCP and without the `archo` CLI. Gets `Player.log` path, the Mod folders and the game folder. The c1 to c5 matrix (A 15/15, N 10/15) settled this comparison; the 2026-11-04 round compares harnesses on h1, h2 and h5 only |
| Prompt | "The game logs errors / misbehaves with this Mod list. Find the Mod that causes it, then fix it. Hand in the rewritten XML or the recompiled DLL." No hint about the fault class |
| Repeats | at least 3 runs per task per group (DeepSeek is stochastic) |

## Isolation

Each run starts from a clean image: the VM snapshot (preferred, no evidence can leak between runs) or, on the host, a fresh copy of the case directory plus an empty `%APPDATA%\RimArcho` and empty `Docs\`. Never run B after A in the same writable state: A leaves `archo.db`, Source-library decompilations and notes.

The VM image has .NET SDK 8, `ilspycmd`, Node 26, Pi, dsh and Git for Windows (snapshot `RimArcho-Eval-Staged4` = Staged3 plus Node 26.13.0 and dsh in `C:\Tools\npm`, 2026-11-04; see Harness status).

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

## Grading

`scripts/mcp-eval/logdiff.mjs` compares a run's error lines (paths and addresses normalised) with the case's `reference.Player.log`. A submission is applied as a layer over the case (`sandbox.mjs <case> --layer <dir>`; files at `<packageId>/<mod-relative path>`, `enable.json` for re-enabled Mods), the game starts, and the log must have no error line the reference fix lacks. Culprit scoring uses `answer.json`: every `culprit` must be named; naming only a `decoy` fails.

## Scoring

1. **Culprit**: correct Mod named (0/1; c2 needs the Mod whose patch fails, naming the cause is a bonus; c5 fails if only the decoy is named).
2. **Fix**: the submitted files applied as above, then one game start; no error line beyond the reference (0/1).
3. Cost: turns, tool calls, tokens, wall clock.
4. Group A only: tools used, calls that returned nothing useful, and where the agent fell back to bash. This drives the MCP changes.

## Harness status (2026-11-02)

- VM snapshot `RimArcho-Eval-Staged3` (UID 11, live, `NewPlayer(Chinese)` signed in on the desktop; Staged2, UID 10, without autologon, is its parent): game 1.6.4871 junctioned into `C:\Eval\RimWorld`, non-Workshop Mods in `C:\Eval\lib`, Archo build in `C:\Eval\tools`, Git for Windows (the shell Pi's `bash` tool needs), `ilspycmd` on the machine PATH. Each run reverts to it; see `scripts/mcp-eval/README.md`.
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

Pilot with both fixes, c3 group A (one run, `results/c3-A-1`): 508 s, 35 turns, 54 tool calls (bash 49, `status` 1, `test_run` 2), 1.33 M tokens; culprit yes, fix yes (rebuilt DLL, log clean). After copying its rebuilt DLL into the Mod, the agent verified with `test_run` (menu, 89 s) and `test_run map=true` (map, 85 s); both returned full reports. Both reports set `isError`, because the base noise errors stay (expected; 09 §7 counts every error cluster), so the agent still grepped the run's Player.log to see that its own error was gone.

MCP lead from that report: the cluster lists no Mod (`mods: []`, `possible`) although `SmartFirefight.Startup` is a type in Smart Firefight's own assembly; the missing `Designator_ExtinguishFires` texture is also unattributed. Attributing a type named in the message to the Mod whose assembly defines it would name the culprit directly.

## Open items

- Redo the pilots: none before defect 6 was fixed had a working game (results moved to `results-session0`); c3 group A is redone above.
- MCP leads so far: attribute a type named in an exception to the Mod whose assembly defines it; let `test_run` compare against a baseline run (or the previous run) so a fix shows as "error gone" instead of a failing exit caused by unrelated noise.
- Full matrix: 5 cases x 2 groups x at least 3 runs (7 to 15 minutes each plus 2 minutes of grading, one VM, so about 8 hours).
- Grading starts the real game on the host (a window, about 2 minutes per run); consider grading inside the VM.
- Review whether the 15-minute limit is right: two of three B runs hit it.
