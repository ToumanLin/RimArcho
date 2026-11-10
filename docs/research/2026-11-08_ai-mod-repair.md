# AI Mod repair: what RimArcho must provide (proposal, 2026-11-08)

Status: proposal, not decided. Question: RimArcho is infrastructure whose first user is an AI agent (00 §3.2, D12). The agent diagnoses and fixes Mod errors and conflicts for a player who does not know Mod internals. What must RimArcho provide so that this is simple, usable and robust *for the player*, and where does fixing C# assemblies fit?

Status 2026-11-09: deferred. R6 is done: rounds 4 and 5 ([mcp-eval-design.md](mcp-eval-design.md), archo e2fa7445, deepseek-flash) re-ran h4, h7 and h8. Attribution now works: h4 5/6 (dsh 3/3, Pi 2/3), h7 6/6 once a fix in either Mod counts (the grading was unfair: vanilla's loop tolerates the list change), h8 culprit named in 8 of 9 runs (Pi 2/3, dsh 3/3, Pi with Archo's instructions 3/3), fixed in 5. What still fails is execution, not diagnosis: a shell command that never returns (`find /`), and h8 DLLs compiled with `csc` that the game refuses (`FieldAccessException`); be8ba34e makes Archo flag those (`built: !!`, `access: !!`, `shipped:`). Pi given Archo's server instructions solved 6/9 against plain Pi 7/9 and dsh 8/9, so the instructions do not explain dsh's lead. h9 was not re-run. Order when this resumes: the player-side promises first (R5, R4, R1); R3 only as facts in output ("nothing to fix"), not a longer prescribed workflow; R2 last or not at all: the failures it targets are now detected, and a tool that does the steps for the agent may limit stronger models.

Evidence: the three MCP evaluation rounds ([mcp-eval-design.md](mcp-eval-design.md), [round 1](2026-11-04_mcp-transcript-review.md), [round 2](2026-11-05_mcp-transcript-review-round2.md)), CHANGELOG 1.0.9 (unreleased), and checks of ilspycmd 11 and dnSpyEx made on 2026-11-08 (Appendix A).

## 1. The player's side

The player never sees the agent's tools. They see four things, and each is a promise RimArcho has to keep whatever the agent does:

1. **Nothing to install.** No .NET SDK, no command line. A tool RimArcho needs is fetched once, after the player agrees.
2. **Nothing breaks.** The agent works on copies. The player's Mods, Workshop folders and saves change only when the player applies a fix, and every fix can be undone.
3. **Proof, not a claim.** "Before: 23 errors. After: 0, and your save loads" comes from a test run, not from the agent's opinion.
4. **Honesty.** "Nothing needs fixing" is a valid answer. A fix made for one version of a Mod says so when the Mod updates.

## 2. What the evaluations show

**Most errors need no code change, and agents with RimArcho fix them.** Round 1 (c1–c5): 15/15 with Archo, 10/15 without. Round 2 (h1, h2, h5): Pi 7/9, dsh 9/9. These were list, XML and stale-DLL problems (h2's answer is deleting a stale copy or reordering).

**Code-level runtime errors are where agents fail, and compiling is rarely why.** Round 3 (archo 1.0.7):

| Case | Solved | Why the others failed |
| --- | --- | --- |
| h4, null in another Mod's postfix | 2/6 | all 4 failures null-checked the Mod at the top of the stack: the symptom, not the cause |
| h7, comp list changed mid-tick | 0/6 | all 6 rewrote the profiler loop that threw; 5 of 6 found SSR's `AllComps.Remove` and judged it allowed |
| h8, two Mods' lord jobs clash | 3/6 | all 5 runs that submitted named the right Mod; two rebuilds without the publicizer threw `FieldAccessException`, one run timed out after 73 bash calls |

c3 (a Mod that ships its source, clear cause) was 3/3 in round 1. Building cost time (every c3 run hand-wrote a csproj and found the Mono `mscorlib` and `0Harmony` references by trial, 30–60 s), but it failed only when references were wrong (h8). The hard part is choosing the right Mod.

**Agents always change something.** h9 has no fault. 0 of 6 runs said so: 4 false positives, 2 timeouts.

**Agents change live files.** In round 2, every run that verified a fix with `test_run` first changed the live Mod folders (8/8 A, 7/7 B). One run edited a Workshop file in place. Only dsh used `pin_mod`, and `test_run` then loaded the original.

**Noise costs more than code.** Round 1 A spent about 1,190 s (15% of its time) on pre-existing errors; in h2, 4 of 5 solving runs drew placeholder PNGs for unrelated missing textures.

For the player, the risks are a wrong fix, an unneeded fix and a change to real files. A missing SDK is a smaller problem, and it matters only for the few fixes that need code.

## 3. What already exists (1.0.9)

| Need | What RimArcho has | Status |
| --- | --- | --- |
| Work on a copy | `pin_mod` (07 §5); `test_run` links pinned copies for the run (P20) | Shipped 1.0.5 |
| Proof | `test_run` compares with the player's log (`fixed`, `before`) and the previous run (20 §2) | Shipped 1.0.5 |
| Proof for runtime and save errors | `test_run` `save` loads a save in the run's data, `ticks` plays N ticks (20 §2) | 1.0.9 |
| Right culprit, null errors (h4) | `trace:` follows the null to the frame that passes it; `source callers` | 1.0.9; round 4: 5/6 |
| Right culprit, list changed elsewhere (h7) | list modifiers from an IL scan of every enabled Mod (06 §2) | 1.0.9; round 4: 6/6 with either Mod accepted |
| How to rebuild | `build:` block in `get_mod`; `guide topic=rebuild` (pin first, prefer a patch Mod, publicizer, references, `test_run` proves it) | 1.0.9 |
| What a rebuild changed | `source diff` (`read_source mode=diff`): changed methods, non-public members used without a publicizer | 1.0.9 |
| Read code | `read_source` with ilspycmd, found on PATH or `~/.dotnet/tools` (05 §5.4) | Needs the SDK to install |
| Build code | the agent runs `dotnet restore` and `dotnet build`; the guide says: without the SDK "nothing builds; tell the player to install it" | Needs the SDK and nuget.org |

mcp-eval-design still lists "nothing tells an agent that a DLL needs publicized references" as open; the 1.0.9 `build:` block and rebuild guide cover it.

The safety and proof pieces exist. What is missing is mostly on the player's side: zero setup, honest answers, a record of fixes, and approval.

## 4. Gaps and proposals

### R1. Read code with nothing installed

ilspycmd 11's Windows x64 build is a Native AOT executable that runs without .NET (Appendix A). Make it a third external dependency in `internal/deps` (17 §2.1), next to todds and SteamCMD, which already have what it needs: `archo deps install`, a download only after the player agrees, a RimArcho-owned folder, a pinned hash, and the settings rows.

- Source: the NuGet package `ilspycmd.win-x64` at a pinned version, checked by SHA-256 like todds' zip. nuget.org is not covered by the GitHub mirror (17 §2.2) and can be slow from mainland China, so a copy attached to a RimArcho GitHub release is the second source; the pinned hash makes the mirror safe.
- Search order like the others: `ARCHO_ILSPYCMD` → own copy → PATH → `~/.dotnet/tools`.
- Not in the release zip: self-update accepts only known top-level entries (`selfupdate.Files`: the two exes, READMEs, CHANGELOG, LICENSE, `monitor`, `skills`), so 1.0.8 and older would reject a zip with `tools/`. Downloading also keeps 21 MB out of every update.
- When it is missing, `read_source` and the guide say "ask the player to install the decompiler in Settings" instead of "install the .NET SDK".
- Spec change: 05 §5.4 and 17 §1 say ilspycmd is "never installed automatically" and "not distributed". A download the player confirms keeps the intent; the wording must change.

### R2. Build code with nothing installed

A build tool (CLI `archo build`, MCP `build_mod`) that takes a source folder and does the parts agents get wrong:

- **Target**: framework from the game (net472 for 1.5/1.6, net10.0 for 1.7, 19 §5), Harmony 2.x or 3.x from the plan.
- **References**: the game's `Managed` folder (the game root on 1.7), the plan's own Harmony, and the DLLs of the Mods the source depends on, taken from the current plan rather than from a relative HintPath.
- **Publicizer**: when the `build:` block says the code needs one, publicize the game references itself (h8).
- **Output**: only into a pinned copy or a new patch Mod, never a Workshop folder.
- **Result**: compiler errors in a short form, then `source diff` against the old DLL, then the advice to run `test_run`.

The compiler would be a helper fetched on demand like R1, as another `deps` entry: Roslyn in a self-contained .NET executable. Not measured: its size, and whether it can be Native AOT. Windows' own `csc.exe` (C# 5, .NET Framework 4.8) is on every Windows 10/11 and needs no download, but cannot build for 1.7 or modern C#, and agents write modern C#. It is a fallback at most.

R2 also lets `skills/rimworld-probe` build without the SDK (20 §4).

### R3. Least change first, and "nothing to fix"

- **A fix ladder** in `guide topic=diagnose` and in what tools suggest: list change (enable, disable, move, swap copy) → Mod settings → XML patch → small Harmony patch Mod → rebuilt Mod. Each step up needs more evidence and a passing `test_run`.
- **"Nothing to fix" is an answer.** When the player's log has no error attributed to an enabled Mod, `status` and `analyze_log` should say it in those words (h9: 0/6). The task text stays as it is, so the evaluation keeps measuring this.
- **Hiding is not fixing.** `source diff` can flag a change that only adds a `try`/`catch` or a null check in the method that threw, when the trace names another Mod as the source (h4). Related: P22(b).

### R4. A fix is a record

Each applied code or file fix becomes a record the player can see: which Mod and version, what changed (the pinned copy's git baseline, 07 §5.2), why (the cluster and evidence), and the test result.

Where the record lives (the owner's requirement, 2026-11-09): in two places, written by the agent as part of every fix.

- **A file in the pinned copy** (or the patch Mod): what changed, file by file; why (the cluster and its evidence); the original Mod's version or Workshop id; the `test_run` result before and after; the date. It travels with the copy (packs, folder moves) and is readable without RimArcho. File name and format to decide; the game ignores unknown files at a Mod's root.
- **A summary in the copy's RimArcho note** (Mod library, the copy note of 16 §2.9): one or two lines naming the fix and the file. The copy note was made for this.
- Gaps today: `pin_mod reason` writes the copy note only when the copy is made, before any fix. Writing it later needs `set_player_data action=copy_note`, which is served only with `--tools all`, and replaces the text, so it would erase the pin reason. A fix needs a core-group way to append to the copy note. Archo can also check that the record is there: a pinned copy whose files differ from its git baseline but has no record file is flagged in `get_mod` and the Mod library.

- **When the Mod updates** (07 §5.3 already warns about the newer Workshop version): re-run the test with the new version. If the error is gone, suggest dropping the fix. If the fix is still needed, say so. Today `pin --update` makes a new copy without the edits (07 §5.4); a patch Mod keeps working, which is one more reason R3 prefers it.
- **A report for the author**: the cluster, the cause and the change, as text the player can post. The author's fix is the one that lasts.
- **Remove all**: one action that takes every AI fix out of the current plan.

### R5. Approval in Archo

15 §4.2 says that for changes to a Mod's code Archo gives "only the explanation and the location, and never modif[ies] Mod files"; ToDo ("Archo Agent") wants an agent that acts. Proposed rule: a code or file fix is allowed only on a pinned copy or a new patch Mod, after a `test_run` that removes the error and adds none, behind a confirmation card (15 §2.2) that shows what changes and the before/after counts.

### R6. Measure before building R2

The h4 and h7 attribution fixes are in 1.0.9 but have not been run. Re-run h4, h7, h8 and h9 on 1.0.9 first: if agents now pick the right Mod, the remaining failures are about building, and they show whether R2 is the next step. Grading inside the VM and an h8 save check are open items in mcp-eval-design.

## 5. Order

1. R6, re-measure on 1.0.9.
2. R3, ladder and "nothing to fix" (guide text and small output changes).
3. R1, ilspycmd download.
4. R2, build tool.
5. R4, fix records.
6. R5, with the built-in Archo (1.1).

## 6. Not pursued

- **dnSpyEx.** GPLv3, a WPF GUI, and its editing is not reachable from a command line. Appendix B describes how its method editing works.
- **Editing one method and merging it into the original DLL** (dnSpy's approach, possible with dnlib). It exists for people who want to change one method without handling a whole project. An agent can rebuild the project or write a patch Mod instead, so the hardest part of dnSpy, the merge, is not needed.
- **An IL writer in Go.** `internal/dotnet` reads metadata and IL; writing is a different project.

## Appendix A. Facts checked on 2026-11-08

- `dotnet tool install` of ilspycmd 11.2.0.10050 leaves a `.cmd` launcher and `…\ilspycmd.win-x64\11.2.0.10050\tools\any\win-x64\ilspycmd.exe`: one file of 21,473,280 bytes with no DLLs beside it and no `hostfxr` reference (a framework-dependent apphost has one). Copied to a temporary folder and run with `PATH` set to `System32` only and `DOTNET_ROOT` pointing to a missing folder, `-t System.Xml.Linq.XName` decompiled in 0.3 s. Its nuspec says MIT. Not tested: a Windows install with no .NET at all.
- `ilspycmd -lv` selects the C# language version (CSharp1 and later).
- `C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe` is version 4.8.9221.0, "for C# 5".
- dnlib and ILSpy are MIT; dnSpyEx is GPLv3.

## Appendix B. Why recompiling is hard, and how dnSpyEx does it

Recompiling decompiled code is hard on three levels:

1. **Merging one edited method back** needs the whole type as context, brings compiler-generated types (closures, iterator and async state machines), and needs every reference remapped to the original assembly.
2. **Rebuilding a whole decompiled project** often does not compile (unrecovered generated names, IL that C# cannot express). It needs every reference at the right version. It can change behavior (inlined `const` values and default arguments). It changes compiler-generated names and IL sequences, which other Mods' Harmony patches and transpilers may target.
3. **Any rebuild** proves only that the code compiles: Mono (1.6) and .NET 10 (1.7) differ in what they accept, Workshop updates overwrite the change, and there are two target frameworks.

dnSpyEx (`Extensions/dnSpy.AsmEditor/Compiler`) solves level 1:

- It decompiles the whole type, showing only the edited method and passing the rest as hidden context.
- It patches an in-memory copy of the original module before handing it to Roslyn as a reference (`MDEditorPatcher`): it renames the edited type to a GUID, makes it non-public, points references to it at the temporary assembly, and adds `InternalsVisibleTo` for that assembly.
- After compiling, `ModuleImporter` (about 2,300 lines) imports the new compiler-generated types, renaming them on name clashes, merges members and remaps references. It also handles edge cases its comments name: security attributes the compiler always adds, embedded attributes and VB core types.

Its errors go to a person, who edits the code or falls back to IL editing. Level 2 problems (inlined constants, changed generated names) are left to the person.
