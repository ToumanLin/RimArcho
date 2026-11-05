# MCP transcript review, round 2: h1, h2, h5 with Pi and dsh

Source: the 18 runs of round 2 on 2026-11-04 (cases h1, h2, h5; A = Pi 1.4, B = DeepSeek Harness (dsh) 0.2.1; both with `archo mcp`, default `core` group, built from 5ae03605, which includes P1–P4 of the [round-1 review](2026-11-04_mcp-transcript-review.md); model `apikeyfan/deepseek-flash`; real RimWorld 1.6.4871 in the VM; 20 min limit). Aggregate results are in [mcp-eval-design.md](mcp-eval-design.md), "Round 2". Raw data (read-only): `D:\RimArcho-test\mcp-eval\results\<case>-<group>-<run>\`. For B, the full session log (`session.v4.jsonl.zstd`) was read, not the 8 KiB-capped `transcript.jsonl`.

A reference such as `h1-A-1 t=11` means 11 s after the run's first event (Pi: assistant message timestamps; dsh: session event times), in a timeline extracted from the transcript. Times are approximate to the turn. No code was changed and archo was not run for this review.

## Summary

Results: A 7/9, B 9/9. The two failures were not caused by Archo output. One was partly caused by the absence of an Archo fact (§4).

What changed since round 1:

- **P1 works.** `test_run` set `isError` on 3 of 17 calls (round 1: 22 of 22). Its `player_log` comparison (`"fixed": 57`, `"clusters": []`) was read and trusted ("Test run shows zero clusters, all 57 previous errors fixed", `h1-A-2 t=429`). Two of the three errors are a new problem: a pinned copy was not visible to the test (§2.4).
- **P2 works where read.** Agents dismissed the material and translation clusters by Archo's label ("Archo considers translation error count as known noise", `h2-A-2 t=136`). Pi runs that went to the shell before `analyze_log` still chased the 19 translation errors (`h2-A-2`, `h5-A-2`).
- **P3 works.** `get_mod ref="khamenman.outfitstandsplus"` resolved (`h5-B-1 t=15`). No bare-packageId error in 204 Archo calls.
- **P4 half works.** Attribution now runs on the case logs (`h2` clusters name Outfit Stands Plus as the owner of the missing types), but `status` and `list_problems` still say "stale log (source unknown …): not counted" and report `0 error, 0 warning` (§2.5).

New findings, by cost:

1. **Archo has no notion of two Mods shipping an assembly with the same name** (h2). All six runs found the stale `OutfitStandsPlus.dll` in the Odyssey Patch with `ls`, `md5sum` and `ilspycmd -l c` (12–37 s). Archo had every fact needed: `analyze_log` said Outfit Stands Plus "provides type … and is enabled (a version mismatch?)". It never looked for another enabled copy of that assembly (§2.2). → P19.
2. **`pin_mod` then `test_run` does not test the pinned copy.** The Instructions say "pin_mod first, then edit the copy". dsh did that in 3 runs, and `test_run` then loaded the original and listed the copy as `missing` (09 §7: "A test run creates no links"). One run then edited the Workshop file in place, one swapped `Mods` junctions by hand, and one avoided testing and spent about 440 s building its own static checker (§2.4). Every one of the 15 runs that verified a fix with `test_run` changed the live Mod folders first. → P20.
3. **The h1 cascade is attributed to the wrong Mods.** The root cluster (`Could not find type named VanillaFurnitureEC.StatParts_Bin`) says "(no mod identified)". The 1,000-line cross-reference cluster names five Mods that merely use `Beauty`, BondageFurniture first. 52 `defines the stat base more than once` clusters fill the list. `search_files StatParts_Bin` found the patch line in 0.45 s, so the culprit came from Archo anyway, through the agent's own query (§2.1). → P5 (round 1, still open), P21.
4. **Pre-existing, unrelated texture noise is neither attributed nor marked as unrelated** (h2). The Odyssey Patch's missing art is in the reference log too. Its path is built as `"OdysseyPatch/OutfitStands/OutfitStand_" + defName`, and Archo's string-literal attribution matches whole strings only. 4 of the 5 solving runs drew placeholder PNGs and handed them in, and `test_run` then listed the texture clusters as `fixed`. B spent 45–130 s per run on it, and h2-A-2 timed out on a Workshop-wide grep for the texture (§2.3, §4). → P22.
5. **Pi without Instructions starts in the shell.** In 4 of 9 Pi runs the first `analyze_log` came after 16–35 shell calls (81–126 s), including the failed h2-A-2. dsh called it within 6 s in all 9 runs (§5). → P10 (round 1), now with evidence.

## 1. Results by case

| Case | Group | Solved | s per run | Archo calls per run | Culprit file first seen (s, per run) |
| --- | --- | --- | --- | --- | --- |
| h1 cascade | A | 3/3 | 277, 443, 161 | 18, 5, 10 | 11 (`search_files`), 243 (shell grep), 53 |
| h1 cascade | B | 3/3 | 256, 325, 360 | 15, 14, 18 | 16 (`search_files`), 46 (`get_patches`), 17 (`search_files`) |
| h2 same-name DLL | A | 2/3 | 336, 1,200 (timeout), 323 | 4, 4, 7 | 15, 12, 12 (all by `ls` of `Assemblies`) |
| h2 same-name DLL | B | 3/3 | 327, 488, 328 | 6, 9, 6 | 17, 23, 19 (all by `ls` / `Get-ChildItem`) |
| h5 two faults | A | 2/3 | 250, 301 (wrong folder), 272 | 4, 8, 11 | c1 9–12 (shell); c5 54, 109 (shell), 27 (`analyze_log`) |
| h5 two faults | B | 3/3 | 711, 406, 169 | 19, 29, 17 | c5 13 (`analyze_log`), 40, 56 (`search_files`); c1 22–155 |

Archo against shell, all runs:

| Group | Archo calls | Other calls | Archo share of result text | `test_run` calls (`isError`) | `list_problems` results over 15 KB |
| --- | --- | --- | --- | --- | --- |
| A Pi | 71 | 318 | 22% (199 K of 898 K chars) | 8 (1) | 3 |
| B dsh | 133 | 284 | 25% (330 K of 1.33 M chars) | 9 (2) | 5 |

Calls per tool, A / B: `analyze_log` 27 / 35, `status` 8 / 10, `test_run` 8 / 9, `get_def` 5 / 10, `get_mod` 5 / 9, `list_problems` 5 / 8, `search_files` 3 / 14, `get_patches` 3 / 10, `list_changes` 3 / 4, `read_source` 1 / 3, `guide` 1 / 4, `pin_mod` 0 / 8, `find_mods` 0 / 5, `unpin_mod` 0 / 2, `edit_loadout` 0 / 1, `plan_history` 0 / 1, `scan` 1 / 0. Pi once called a tool that does not exist (`mcp__archo__read`, `h1-A-1 t=14`).

## 2. Per case: what helped, what misled, what was skipped

### 2.1 h1: a misspelled class in Vanilla Furniture Expanded's patch

**Did `analyze_log` attribute the cascade to the patch? No.** The default list (`h1-A-1 t=6`, identical in all six runs, 5,084 chars):

```
log:E3 xml-load x2  Exception loading def from file Stats_Basics_General.xml: … Could not fin...  (common, unattributed)  [pattern:g:xml.type-missing]
log:E4 xref-missing x1000  Could not resolve cross-reference: No RimWorld.StatDef named Beauty …  → #n3az (mentioned, +4)  [pattern:g:xref.def]
log:E5 xref-missing x119  … StatDef named BeautyOutdoors …  (common, unattributed)
log:E8 xml-load x4  Config error in Filth_Dirt: defines the stat base  more than once.  (common, unattributed)
… E9–E32 the same for 24 more defs …
… 27 more (archo log --all)
```

- `analyze_log cluster=E3` (the root cause): "attribution: (no mod identified)", "note: common, unattributed: this kind is in 34.5% of public 1.6 logs". Causes: "the framework mod that defines the class is not active or loads later; the class was renamed in an update". A typo is not among them, and the patch that adds the class is not searched. Round 1's P5 is not done.
- `analyze_log cluster=E4`: "providers: #lnrj (active)" (Core; round 1's "none among installed mods" is fixed), but attribution lists five `mentioned` Mods, BondageFurniture first ("references Beauty at …DDJY_Buildings_bed.xml:72"), Vanilla Furniture Expanded third, because its own Defs use `Beauty`. Nothing links E4 to E3, although E3's two lines are in E4's `nearby` block.
- `get_def StatDef/Beauty` (all six runs): "defined-by 1. #lnrj Core Defs/Stats/Stats_Basics_General.xml:210 ← effective", patched by VEF, VFE ("hit 3") and Beheading. It does not say the def failed to load.
- The 52 `Config error in X: defines the stat base  more than once` clusters (the stat name is empty because `Beauty` is null) are one cluster per def. Agents asked about them: `read_source search="defines the stat base"` (`h1-A-1 t=30`), `analyze_log cluster=E8` (`h1-A-3 t=7`, `h1-B-1 t=49`).
- **What found the culprit:** `search_files pattern=StatParts_Bin` → "29 #l4uq Vanilla Furniture Expanded … 1.6/Patches/Core.xml 12 in Defs/StatDef[defName="Beauty"]/parts <li Class="VanillaFurnitureEC.StatParts_Bin"/>" (453 ms; `h1-A-1 t=8`, `h1-B-1 t=16`, `h1-B-3 t=17`). `get_patches target=StatDef/Beauty` listed VFE's operation too (`h1-B-3 t=18`: "#d2xs Vanilla Furniture Expanded 1.6/Patches/Core.xml:3 Verse.PatchOperationFindMod(Core) hit 3").
- **`get_patches` with a bare name misses it.** `get_patches target=Beauty all=true` (`h1-B-2 t=46`) answered "patches: 2 operations, 0 failed": VEF and Beheading, without VFE's `FindMod` → `Sequence` → `Add`. The agent: "VFE patch Core.xml not shown in get_patches target Beauty because maybe FindMod operation not simulated/listed?" (`t=50`). The full form `StatDef/Beauty` lists 3. → P26.
- **What the agents still did by hand:** confirming the right name `StatPart_Bin` (decompiling or `strings` on `VanillaFurnitureEC.dll`, or comparing with the `1.5/Patches/Core.xml` shortcut): `h1-A-1 t=93`, `h1-B-1 t=218`, `h1-B-3 t=205`, `h1-A-2 t=191–243`. h1-A-2 did not use `search_files` and lost 90 s on one `grep -Ril StatParts_Bin` over `Mods\` (`t=28` → `t=118`) after plain `grep -rl` returned nothing (it does not follow the junctions).
- `test_run` after the fix: `"fixed": 57`, `"clusters": []`. The `fixed[]` list repeats the five `mentioned` Mods for the `Beauty` clusters and is 7.6 KB.
- `list_problems all=true` (4 calls in h1): 20–25 KB, 137 info rows, nothing about the patch. Round 1's P12 is not done.

### 2.2 h2: the Odyssey Patch ships a stale `OutfitStandsPlus.dll`

**What Archo said.** The first list (`h2-A-1 t=9`, identical in all runs):

```
log:E3 xml-load x1  Could not find a type named OutfitStandsPlus.Things.MechanizedOutfitStand  → #lzcq (mentioned)
… E4–E6 the same for three more types …
log:E7–E14 Config error in OutfitStandsPlus_… has null thingClass / … is not a ThingWithComps  (common, unattributed)
log:E17–E20 texture-missing  Failed to find any textures at OdysseyPatch/OutfitStands/OutfitStand_Female …  (common, unattributed)
```

`analyze_log cluster=E3`: "#lzcq Outfit Stands Plus mentioned evidence: provides type OutfitStandsPlus.Things.MechanizedOutfitStand and is enabled (a version mismatch?)", "confidence: possible (no source identified)". `list_problems code=log detail=true` (`h2-A-3 t=138`) repeats it four times. `get_mod` of both Mods (`h2-A-3 t=127`) says "content: defs 2, patches 5, assemblies 2, harmony patches 65" without naming the assemblies. `list_problems` default: "problems: 0 error, 0 warning, 0 info" (`h2-B-2 t=9`).

**What the agents did.** Every run listed `Assemblies` folders by shell within 12–23 s, saw a second `OutfitStandsPlus.dll` (31,744 bytes against 33,280), hashed both (`md5sum`, `Get-FileHash`), and ran `ilspycmd -l c` on both to see that the copy in the Odyssey Patch lacks the four types (confirmed at 25–37 s). One `test_run` per run then confirmed the fix ("fixed": 16 or 12).

**Does Archo's analysis know about duplicate assembly names?** No.

- There is no `asm.*` finding code (05 §4.3; `internal/finding`, `internal/app`).
- The missing-type rule (`internal/app/logs.go:915`) knows the type is in an enabled Mod's assembly (`Store.TypeOwners`) and stops at "(a version mismatch?)".
- The only same-name logic is `attrCtx.library` (`logs.go:630`). It treats a DLL name that several enabled Mods ship (`LunarFramework.dll`) as a shared library and avoids blaming any of them. That is right for identical library copies and wrong for h2, where the copies differ.
- `Store.InstanceFiles(env, "%<name>.dll")`, used for `Cannot resolve dependency to assembly` (`logrefs.go:494`), can already find every installed copy of a DLL by name.

So `analyze_log` could have said: "#lais 1trickPwnyta's Odyssey Patch (list position 24) ships `1.6/Assemblies/OutfitStandsPlus.dll` (31,744 B, without this type) and loads before #lzcq (25), whose copy defines it" → source. `get_mod` could list each assembly with its name, version and size, and `list_problems` could flag differing same-name assemblies statically, before any log exists. → P19.

**Where else time went in h2:** the texture clusters (§2.3, §4), and in Pi runs the translation count before `analyze_log` (§5).

### 2.3 h2: the texture clusters

- The list row says only "(common, unattributed) [pattern:g:asset.texture-missing]". No run opened a texture cluster's detail. `h2-A-2 t=136` asked for the pattern instead: "causes: the art comes from a mod or DLC that is not active; the mod ships the file under another path or case; route: analyze_log cluster=E<n>, search_files <path>".
- `search_files pattern="OdysseyPatch/OutfitStands" kind=all all=true` → "0 matches in 0 files of 0 mods" (`h2-B-3 t=79`). The string exists only inside `OdysseyPatch.dll`, as the prefix of `"OdysseyPatch/OutfitStands/OutfitStand_" + defName`. `search_files` matches asset paths, not assembly string literals. The attribution rule for string literals in code (06 §2; `logrefs.go:257`) compares whole strings (`lit[s]`), so a prefix never matches.
- Agents then decompiled `OdysseyPatch.dll` (all B runs, h2-A-2), read `Verse.Graphic_Multi` with `read_source` (`h2-B-2 t=214–262`, `h2-B-3 t=98`), and drew placeholder textures. 4 of the 5 solving runs (h2-A-1, h2-B-1, h2-B-2, h2-B-3) handed in 12–16 PNG files under `odysseypatch.1trickPwnyta/1.6/Textures/OdysseyPatch/OutfitStands/`; only h2-A-3 handed in just `delete.json`. The texture lines are not part of the fault: they are the Odyssey Patch's own pre-existing defect, present in the case's reference log (the log with the correct fix) and in c2. The grader accepted the PNGs because it checks only for errors beyond the reference.
- **How `test_run` presented them.** All four PNG runs wrote the textures before their only `test_run` (`h2-B-1` at `t=114`, test at `t=166`; `h2-A-1` 187/214; `h2-B-3` 177/196; `h2-B-2` 311/346). Each report then listed the four texture clusters under `fixed` (`"fixed": 16`, `"clusters": []`), with `"mods": []`. So `test_run` presented silencing unrelated noise as part of the fix. h2-A-3 tested without PNGs and got them back as remaining clusters: `"before": "present"`, `"mods": []`, `"note": "common, unattributed: this kind is in 27.5% of public 1.6 logs"`, and `isError` although 12 clusters were fixed. `before: present` was the one signal that they predate the change; nothing said they are unrelated to it, and the error flag said the opposite.
- **How `analyze_log` presented them.** Every list row (all 6 runs, 7 calls) reads `log:E17 texture-missing x1 Failed to find any textures at OdysseyPatch/OutfitStands/OutfitStand_Female … (common, unattributed) [pattern:g:asset.texture-missing]`. These rows look exactly like the row of the h1/h5 root cause (§2.1, P23). Unlike the material and translation lines (P2), they are not in the `hidden:` known-noise line.

### 2.4 h5: two faults; and `pin_mod` + `test_run`

- **c5 fault:** `analyze_log cluster=E4` named it at once: "#shg3 10x Carrying Capacity (Continued) source evidence: references OutfitStandsPlus_MechanizedOutfitStandResearch at Patches/Stats/Stats_Pawns_General.xml:14", "confidence: likely" (`h5-A-3 t=23`, `h5-B-1 t=13`). The decoy (Outfit Stands Plus) was not named. This is the round-1 c5 result again.
- **c1 fault:** `analyze_log cluster=E3`: "Could not find a type named OutfitStandsPlus.Thing.MendingOutfitStand", "attribution: (no mod identified)", "note: common, unattributed" (`h5-A-3 t=12`). The next lines (`after:58 Config error in OutfitStandsPlus_MendingOutfitStand: has null thingClass`) name the def, and `search_files` for the class finds `Defs/ThingDefs/Building_MendingOutfitStand.xml` (`h5-B-2 t=33`). Agents found the file by `grep` or by reading the Mod's Defs (9–155 s), then read the Mod's `Source/` or decompiled its DLL to learn `OutfitStandsPlus.Things`. This is round 1's P5 again.
- **`pin_mod` → edit → `test_run` fails.** dsh followed the Instruction "Never edit Workshop files in place: pin_mod first, then edit the copy" in three runs:
  - `h1-B-3 t=92`: `pin_mod #d2xs git=true` → "plan: #fdj6 replaces #d2xs in the current plan …; the game gets it through an _Archo_ link on save or launch". It edited the copy, then `test_run` (`t=105`) returned `isError`, `"missing": [{"package_id": "vanillaexpanded.vfecore"}]`, and the original error with `"before": "present"`. The agent: "The test run still has old errors and reports missing … the pinned copy is in locked dir, not linked into Mods until save" (`t=204`). It then **edited the Workshop file in place** (`…\workshop\content\294100\1718190143\1.6\Patches\Core.xml`, with a `.bak_eval` copy) and tested again.
  - `h5-B-2 t=124–138`: pinned both Mods, edited the copies, `test_run` → `missing` two Mods, old errors. "The test run loaded the original Workshop copies (pinned copies weren't visible to it), so that result is not meaningful. I'll … test by temporarily swapping the Mods junctions" (`t=268`). It read `guide topic=test_run` ("Mod folders the game cannot see are reported as missing; no links are created"), swapped the junctions by hand and ran `test_run` again (`t=308`).
  - `h5-B-1`: decided "To test fixed files, need plan use modified copies … test optional" (`t=193`) and instead spent about 440 s (`t=205–647`) writing a PowerShell checker that compares every `thingClass` / `Class=` in all Defs and Patches with `ilspycmd -l c` output for every DLL. It pinned both Mods at `t=654`, reverted the plan and finished without a test run (711 s, the slowest run of the round).

  09 §7 decides this on purpose ("The game can find plan Mods only in its Data/, Mods/ … A test run creates no links"). The Instruction and the test tool do not fit together: the only safe workflow the server recommends cannot be tested. → P20.

### 2.5 All cases: status and the "not counted" log

`status` in every run: "log: Player.log … stale log (source unknown: no launch and no ModsConfig.xml write by RimArcho before it): not analysed yet, so its errors are not in the counts". `list_problems` default: "problems: 0 error, 0 warning, 0 info" followed by "12 error, 4 warning, not counted" (`h2-B-2 t=9`, `h5-B-2 t=14`). P4's attribution part is in, but its "count such a log's findings in `list_problems` instead of not counted" part is not. In the eval this is an artifact (nothing launched through RimArcho). For a player who starts the game from Steam, the same headline says "0 error" while the log has errors.

## 3. Shell fallbacks for something Archo does or should do

Time per category is the gap to the next call, split across parallel calls, classified by command text (approximate, ±20%):

| Fallback | Runs | Approx. time | Archo tool that does it / should |
| --- | --- | --- | --- |
| Listing `Assemblies` folders, hashing DLLs, `ilspycmd -l c` to compare type lists (h2) | 6 of 6 h2 | 50–80 s per run | none; P19 (`get_mod` assembly list, duplicate-name finding) |
| Texture paths: decompiling `OdysseyPatch.dll`, `Graphic_Multi`, drawing PNGs, grepping the Workshop folder (h2) | 6 of 6 h2 | 45–130 s per B run; h2-A-2 lost its last ~940 s | none; P22 |
| `grep -r` / `findstr /s` over `Mods\` for a class or def name (junctions not followed by `grep -r`) | 11 of 18 | 10–90 s per call; h1-A-2 one call 90 s, h5-A-2 136 s total | `search_files` (0.45 s); used in 9 B runs, 3 A runs |
| Confirming the right class name: `ilspycmd -l c`, `strings`, diffing `1.5/`, reading the Mod's `Source/` (h1, h5 c1) | most h1/h5 runs | 20–60 s per run; h5-B-1 a 440 s home-made checker | P5 (did-you-mean types); P19's type lists |
| Re-reading the test run's `Player.log` after a clean `test_run` | 11 of 15 runs that ran `test_run` | 5–20 s per run | `test_run` (P24) |
| Making `test_run` see a fix: editing files in the live Mod folders (Workshop folders behind the junctions in h1 and h5), moving the stale DLL out (h2), backups to `/tmp` | all 15 runs that verified a fix with `test_run`; h5-B-2 re-pointed the junctions at pinned copies instead | 10–70 s | `pin_mod` + `test_run` (P20) |
| Reading `Languages\` for the 19 translation errors | h2-A-2, h5-A-2, h5-A-3, h1-B-1 | 20–30 s each | already known noise (P2); reached only through `analyze_log` |
| Harness snooping (`C:\Eval\lib`, `task.md`, `git log`, timestamps) | 13 of 18 | 5–35 s per run | eval artifact |

Archo produced 22% (A) and 25% (B) of the text the agents read, about the same as round 1 (17% for A).

## 4. The two failures

**h2-A-2 (timeout).** The culprit was in hand at `t=25` ("Confirmed: odysseypatch.1trickPwnyta bundles an older/stripped copy of OutfitStandsPlus.dll … which shadows the real one"). After that:

| t (s) | Activity |
| --- | --- |
| 0–25 | 12 shell calls; found and confirmed the duplicate DLL |
| 25–52 | decompiled `OdysseyPatch.dll` to see why `OdysseyPatch/OutfitStands/*` textures are missing |
| 52–125 | `git log` of `C:\Eval\lib\*`, then the 19 translation errors (`Languages\`, `DefInjected`) |
| 125–139 | first `analyze_log`: translation and material clusters are known noise; texture pattern: "the art comes from a mod or DLC that is not active" |
| 139–255 | `C:\Eval\out`, `task.md`, the transcript, Odyssey asset bundles, `Verse.ModAssemblyHandler` |
| 255–259 | asked whether the Workshop copy ships the texture: `grep -rl "OutfitStand_Female" --binary-files=text` over `steamapps\workshop\content\294100` (1,296 folders, about 15 GB). The call never returned; the run hit the 20 min limit at 1,200 s with nothing submitted |

Could Archo output have prevented it?

- **Partly yes.** The question behind the last call was "does any copy of any installed Mod ship this texture?". Archo knows that in milliseconds (06 §2, "File providers"), and with P22 it would have said, in the list row, that the path is built in the Odyssey Patch's own code and no installed Mod ships it. The texture pattern's causes ("a mod or DLC that is not active") pointed the other way: towards finding a provider somewhere.
- **The 100 s on translations** would have been 0 s if Pi had called `analyze_log` first, as dsh did (§5).
- **Not Archo:** the harness gives `bash` no time limit, so one unbounded grep ended the run.

**h5-A-2 (wrong submit folder).** Pi fixed both faults by `t=173`, wrote them with `cd "C:/Eval/work" && mkdir -p submit/…` and later `answer.json` to `C:/Eval/work/submit/`, then reported `C:\Eval\submit`. No Archo call is involved and none could have prevented it. Its Archo use was otherwise normal (`analyze_log` E3/E4 named the c5 culprit; `test_run` "fixed": 4).

## 5. Instructions: Pi (dropped) against dsh (kept)

dsh's system prompt contains the whole server `Instructions` ("Start with status. For a game error or crash, read guide topic=diagnose before concluding … Never edit Workshop files in place: pin_mod first, then edit the copy."). Pi's recorded prompt has none of it, as in round 1.

| Behaviour | A Pi | B dsh |
| --- | --- | --- |
| `status` as the first call | 3 of 9 (6 of 9 within 5 s) | 8 of 9 (9 of 9 within 4 s) |
| First `analyze_log` | 0–5 s in 5 runs; 81, 82, 125, 126 s in 4 runs after 16–35 shell calls | 2–6 s in all 9 |
| `guide` calls | 1 (`topic=test_run`) | 4 (`diagnose` once, `test_run` 3 times) |
| `pin_mod` before editing | 0 runs | 3 runs (8 calls) |
| Changed the live Mod folders before `test_run` | 8 of 8 runs with a test | 7 of 7 runs with a test (h1-B-3 after the pinned test failed; h5-B-2 by re-pointing junctions) |
| Archo calls per run | 7.9 | 14.8 |
| `search_files` / `get_patches` / `get_def` calls | 3 / 3 / 5 | 14 / 10 / 10 |

The 15 vs 8 difference comes mostly from dsh's use of the lookup tools (`search_files`, `get_patches`, `get_def`, `find_mods`) and `pin_mod`. The Instructions show in three places:

1. **Order.** dsh always started with `status` then `analyze_log`. The four Pi runs that started in the shell lost 81–126 s first, and one of them (h2-A-2) is a failure. Pi's tool list does say `status` comes first ("call first"), and 6 of 9 runs did call it early, but nothing in a description says that a game error starts at `analyze_log`.
2. **Safety rule.** Only dsh pinned before editing, and `test_run` then could not see the pinned copies (§2.4). In both groups, every run that tested a fix changed the Mod folders the game loads: in h1 and h5 those are Workshop folders behind junctions. In a real player's library that overwrites Workshop files, so the rule the Instructions carry is not followed in practice by either harness until P20 exists.
3. **`guide topic=diagnose`** was read once in 18 runs, by dsh. "Read before concluding" is not followed even when delivered.

The extra dsh calls did not cost accuracy or much time (B 374 s mean, A 396 s), but B used 40% more tokens. Most extra B time was in h5-B-1's checker, which a working P20 removes.

## 6. Earlier proposals: status after round 2

| # | Status | Round-2 evidence |
| --- | --- | --- |
| P1 `test_run` vs player log | done, works | `isError` 3/17 (was 22/22); "fixed" read and trusted in every run with a test |
| P2 known noise | done, works | dismissed by label; but "common, unattributed" now also marks the causal clusters (P23) |
| P3 bare packageIds | done, works | no reference error in 204 calls |
| P4 logs of unknown owner | attribution done; counting not | §2.5 |
| P5 type-missing: source + did-you-mean | open; raise to top 3 | h1 E3, h5 E3 "(no mod identified)"; most h1/h5 runs confirmed the right name by hand; h5-B-1 440 s |
| P8 cascade grouping | open; extended by P21 | h1 |
| P9 compact `test_run` | open | `fixed[]` of 57 clusters is 7.6 KB with `args`, `run_dir`, `log_path` |
| P10 rules in descriptions | open; raise | §5: 4 Pi runs started in the shell |
| P12 collapse info rows | open | 8 `list_problems` results of 20–25 KB, 137 info rows each, never used |
| P13 dump lines in `nearby` | open | every cluster detail in all 18 runs starts with 4 `AdjustParms - ldfld …` lines |
| P11, P14–P18 | open | no new evidence |

## 7. New proposals, ranked (with open earlier ones placed by priority)

| Rank | # | Change | Tool / text | Evidence runs | Spec |
| --- | --- | --- | --- | --- | --- |
| 1 | P19 | **Same-name assemblies.** (a) New static finding `asm.duplicate-name`: two or more enabled Mods ship a DLL with the same assembly name in their effective `Assemblies` folders, and the copies differ (hash, assembly version, or type set). Name the copy the game loads first, by list position, and the types only the other copy has. Warning when the shadowed copy defines types that XML or patches name; info when the copies are identical (shared libraries such as `LunarFramework.dll`, which `attrCtx.library` already treats as benign). Verify in the VM which copy 1.6 keeps when names collide, and state it in the finding. (b) In `analyze_log`, when a missing type is defined by an enabled Mod, look for an earlier same-name copy without it (`Store.InstanceFiles` + `TypeOwners`). If found, make that Mod the source: `#lais ships 1.6/Assemblies/OutfitStandsPlus.dll (31,744 B) without this type and loads before #lzcq (24 < 25)`. Otherwise keep "a version mismatch?". (c) `get_mod` lists assemblies: file, assembly name, version, size, type count, `same name as #x`. | `list_problems`, `analyze_log`, `get_mod` | h2-A-1..3, h2-B-1..3 (shell DLL compare 12–37 s in each; `get_mod` "assemblies 2"; `list_problems` "0 error") | 05 §4.3 (new assembly rows), 05 §5.3, 06 §2 "Missing types", 14 §2 (`get_mod`) |
| 2 | P20 | **Make `pin_mod` → edit → `test_run` work.** `test_run` makes game-invisible plan Mods visible for the test only: create `_Archo_` links before launch and remove them after, under 09 §3's ledger and link rules, or use an equivalent test-only `Mods` view. Until then: when `missing` is not empty, do not launch. Return `status: plan-not-visible` with the copies and `next: save_loadout` (dry run first), because the report would describe another list. Change the Instructions sentence to the workflow that works, and `guide topic=test_run` with it. | `test_run`, `Instructions`, `guide test_run` | h1-B-3 (`t=92–204`, then edited Workshop in place), h5-B-2 (`t=124–308`, swapped junctions), h5-B-1 (`t=193–654`, no test; 440 s checker) | 09 §7 "Game-invisible Mods", 09 §3, 14 §3 |
| 3 | P5 | (round 1, unchanged text) For `Could not find type named X` / `Could not find a type named X`: source = the Mod whose effective Defs **or patch values** contain X (the `search_files` query agents ran by hand), with file:line; list the nearest types from analysed and game assemblies (`VanillaFurnitureEC.StatPart_Bin` in #l4uq `VanillaFurnitureEC.dll`; `OutfitStandsPlus.Things.MendingOutfitStand`). Add "a typo or wrong namespace in this Mod's XML" to the pattern's causes. | `analyze_log`, pattern `g:xml.type-missing` | h1 all 6, h5 all 6 | 06 §2, 06 §8 |
| 4 | P21 | **Def file that failed to load.** When a cluster says `Exception loading def from file F`, treat clusters about defs that F defines (from the `Def` facts: Core's `Stats_Basics_General.xml` defines `Beauty`, `BeautyOutdoors`) as consequences: `xref-missing` for those defs, and `Config error in X: defines the stat base  more than once` with an empty stat name. Show them collapsed under the root in the list (`log:E3 … → 1,123 consequent lines in 56 clusters`) with role `consequence of log:E3`, not `mentioned` referrers. `get_def` of such a def adds `log: not loaded (log:E3: exception loading Stats_Basics_General.xml)`. `test_run` `fixed[]` keeps the grouping. Extends P8. | `analyze_log`, `get_def`, `test_run` | h1 all 6 (64 clusters, BondageFurniture named first for the 1,000-line cluster) | 06 §1 (clustering), 06 §2 (`defName` evidence), 06 §3, 09 §7 |
| 5 | P22 | **Mark unrelated pre-existing noise, so agents neither chase it nor change Mods to silence it.** One proposal with four parts. (a) *`test_run`*: a cluster with `before: present` that shares no Mod, def, type or file with any `fixed` cluster is labelled `pre-existing, unrelated to what this run fixed`. It does not set `isError` when at least one cluster is fixed and nothing new appeared (P1's `isError` suggestion, not yet adopted: h2-A-3 failed with 12 fixed). (b) *`test_run`*: when a pre-existing, unrelated cluster disappears, report it apart from `fixed` (`silenced: 4 texture-missing (#lais Odyssey Patch's own art; unrelated to the other fixed clusters)`), so adding files to an uninvolved Mod is not shown as part of the fix. (c) *`analyze_log`*: attribute the clusters so the relation is visible without a test run. Match assembly string literals as prefixes of a missing texture path, when the literal contains `/` and ends in `/`, `_` or `-` (at least 8 characters): `"OdysseyPatch/OutfitStands/OutfitStand_"` → #lais source, likely. Put it on the list row: `→ #lais (own code asks for art it does not ship; no installed mod has it; rendering only)`. Rank such clusters after the load errors and name them in the `hidden:`/noise line when they share nothing with the error clusters. Where an earlier log exists (`Player-prev.log`, earlier test runs of the plan), say `also in <log>`. Add the cause "the mod's own code asks for art it does not ship" to `g:asset.texture-missing`. (d) *`search_files`*: `kind=assemblies` (or `strings=true`) also matches string literals, so the agent's own query finds the code. | `test_run`, `analyze_log`, `search_files`, pattern text | h2 all 6: 4 of 5 solving runs handed in 12–16 invented PNGs and `test_run` listed the textures as `fixed`; h2-A-3 `isError` with `before: present`; B 45–130 s per run; h2-A-2 timeout; `h2-B-3 t=79` 0 matches | 09 §7 (comparison, exit code), 06 §2 (string-literal referrers), 06 §1 (known noise), 05 §5.5, 06 §8, 14 §2 |
| 6 | P10 | (round 1) Put the order rule into descriptions that every client shows: `analyze_log` "For any game error, start here, before reading Player.log"; `status` keeps "call first". Keep the pin rule out of `Instructions` until P20 lands, or word it as P20 says. | `descLog`, `descStatus`, `Instructions` | h2-A-2, h2-A-3, h5-A-1, h5-A-2 (first `analyze_log` at 81–126 s) | 14 §3 |
| 7 | P23 | **"common, unattributed" on causal clusters.** The label (P2) also marks the root cluster in h1 and h5 and the eight h2 config errors, and `guide` reads it as "often background rather than the fault being looked for". Apply it only to clusters that are present before the change being tested (the player-log comparison in `test_run`) or that share no def or type with an attributed cluster. Otherwise say `source not identified (kind seen in 34.5% of public logs)`. | `analyze_log`, `test_run`, `guide` patterns text | h1 E3, h5 E3, h2 E7–E14, `test_run` notes in h1-B-3, h5-B-2 | 06 §8, 09 §7 |
| 8 | P24 | **`test_run` informational clusters.** Instead of only `counts: {"mod-report": 1, "other": 4, "noise": 2}`, list them, one line each, with `before: present/absent`, so the agent need not reread the log. With P9: drop `args`, compact `fixed[]` to class, count and 100 characters of message. | `test_run` | 11 of 15 runs with `test_run` re-read the run log (5 by `analyze_log file=`, others by grep) | 09 §7 |
| 9 | P25 | (P4 remainder) Count a log of unknown owner in `list_problems` and `status` when attribution ran on it; do not print `0 error, 0 warning` as the headline while the log has 12 errors. | `status`, `list_problems` | every run; `h2-B-2 t=9`, `h5-B-2 t=14` | 05 §4.5, 06 §4 |
| 10 | P12 | (round 1) Collapse `list_problems all=true` info rows by code. | `list_problems` | 8 results of 20–25 KB | 05 §4.5, 13 §3.2 |
| 11 | P13 | (round 1) Drop IL dump lines from `nearby:`. | `analyze_log` | every cluster detail | 06 §3 |
| 12 | P26 | `get_patches target=<bare name>` must find the same operations as `target=<Type>/<name>`, including operations nested in `FindMod` / `Sequence` (or say that a bare name matched only top-level XPaths and suggest the full form). | `get_patches` | h1-B-2 `t=46` (2 operations instead of 3; the missing one was the culprit's) | 05 §5.2, 13 §2 |

Harness items (not Archo): give the agent's shell a per-command time limit (h2-A-2); check the submit folder after the run and keep the run's output out of the guest (`C:\Eval\out`, read in h2-A-2 `t=156`); remove the `1.5/Patches/Core.xml` shortcut from h1 or accept that it makes the did-you-mean step easier.

## 8. What could not be determined

- Which copy RimWorld 1.6 keeps when two Mods ship the same assembly name, in general. h2's fault and its accepted fixes (remove the Odyssey copy, or load Outfit Stands Plus first) imply the first in load order wins; P19 should verify that in the VM before the finding states it.
- What the texture cluster's detail would have said: no run opened E17–E20. From 06 §2 it should say "providers: none among installed mods". That was not visible on the list row, where the agents decided.
- Exact token cost per result: dsh records usage per step, Pi per request; sizes here are characters.
- Time attribution in §3 is per call, by command text. A call that mixes categories is counted under the first match, so the numbers are approximate in both directions.
- With three runs per cell, 9/9 against 7/9 does not separate the harnesses. §5 compares behaviour, not accuracy.
