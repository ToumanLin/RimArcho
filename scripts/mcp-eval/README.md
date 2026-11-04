# MCP A/B evaluation harness

Design and case list: [docs/research/mcp-eval-design.md](../../docs/research/mcp-eval-design.md). Everything real stays under `D:\RimArcho-test\mcp-eval\` (never in the repo).

| Script | Where | What |
| --- | --- | --- |
| `cases.mjs` | host | Writes `cases/<id>/` (overlay, answer key, correct-fix layer) from the pristine Mods. c3 rebuilds a DLL with `dotnet`. |
| `sandbox.mjs <case> [--run] [--fixed] [--layer dir]` | host | Junctions the real 1.6 game and the case's Mods into `run/<name>/`, runs the game with `-savedatafolder`/`-logFile`, kills it after ~40 s of log silence. `--layer` applies a submission. |
| `package.mjs <case>...` | host | Packs the case for the VM into `pkg/<case>/` (manifest, ModsConfig, Player.log with VM paths, overlay). The log keeps its `Mono path[0]` line, rewritten to `C:\Eval\RimWorld`, so Archo knows the log is the guest game's (03 §6.4); packs made before 2026-11-04 dropped it. Rerun `package.mjs` before the next matrix. |
| `guest/stage.ps1` | VM, once | Builds `C:\Eval` (lib of non-Workshop Mods, game junctions, `archo.exe`). Already in snapshot `RimArcho-Eval-Staged`. |
| `run-case.ps1 -Case c1 -Group A -Run 1` | host | One run: revert to `RimArcho-Eval-Staged3` (live, `新玩家` signed in), push the case, `guest/overlay.ps1` (admin: faulty files written into the Mod's real folder, original timestamps), `guest/case.ps1` (Mods junctions, config, log; group A also scans with Archo), delete the staging package, `guest/agent.ps1` (Pi, 15 min limit), collect `results/<case>-<group>-<run>/`, stop the VM. Group B has no `archo.exe` on disk and runs Pi with `--no-mcp`. A mutex keeps it to one run at a time. `-Archo <exe>` replaces the snapshot's Archo with that build (a copy stays in the result folder); `-StageOnly` stops after staging and leaves the VM running. Pi starts with `-interactive` (the signed-in desktop session, which must be `新玩家`): in session 0 the game cannot open a window and hangs. |
| `guest/probe-testrun.mjs` | VM, after `-StageOnly` | Calls `test_run` through Pi's own MCP client (60 s request timeout, progress requested), no model; writes `C:\Eval\out\probe.txt`. |
| `summarize.mjs <resultDir>` | host | Tool calls per name, turns, tokens, final text -> `summary.json`. |
| `grade.mjs <resultDir>` | host | Culprit from `answer.json` (list, else explanation); applies the submitted files, starts the game, compares the log with `reference.Player.log`; the fix also has to keep the culprit enabled and change it where the fault is -> `grade.json`. `GRADE_REUSE_LOG=1` re-scores without starting the game. |

Both groups get the same `guest/task.md`. Run in the background (a run takes 5 to 8 minutes); one VM, so one run at a time.
