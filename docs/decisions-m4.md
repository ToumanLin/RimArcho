# M4 decisions and measurements

Commits 6b11e20..HEAD after 4704ee5, local only, not pushed. Scope: the M3 rulings 1, 2, 6 and 7, the Harmony layer
(03 §2, §5.3), log attribution (03 §6), on-demand ilspycmd (03 §5.3), the `harmony` / `log` / `source` commands, and the
MCP server `archo mcp` (05 §4). Not in M4: `copy-local` and the `copy_local` tool (M5). The old inspector
(`tools/ai_mod_decompile`) and the two `rimworld-mod-*` skills are untouched; removing them is the user's call.

## 1. M3 rulings

| Ruling | Implementation |
|---|---|
| 1 version.unsupported | Info when the mod lists `PrevMinor(game)`, warning otherwise. Folded into one row per game version in CLI, MCP and TUI (`p:version`, expandable). The row's severity is the highest among the folded rows. |
| 2 noisy info | `def.override` and `patch.contested` are hidden unless `--all`, `--code`, `--subject`, `--severity info` / `--min-severity info`; a `hidden: N def.override, M patch.contested` line says so. They always show in `def`, `patches`, Inspect. MCP `list_problems` passes `--min-severity warning` unless `severity`, `min_severity` or `all` is given. |
| 6 config | In 05 §3. |
| 7 --game-version | On analyze, problems, def, patches, harmony, why, log (validated `X.Y`, reset after the command); `game_version` on the matching MCP tools. The analysis child receives it. |

## 2. Harmony layer

- `internal/dotnet` is the spike decoder (spikes/dotnetmeta) promoted to a package, plus TypeDef/MethodDef/CustomAttribute
  helpers. `internal/harmony.Read` merges class- and method-level `[HarmonyPatch]` arguments (type, name, `MethodType`,
  `Type[]` argument lists, named arguments), takes the kind from `[HarmonyPrefix]`-style attributes or the method name,
  and marks a bool-returning prefix as `Skips` and `TargetMethod(s)` as `Dynamic`.
- **Manual patching** = a MemberRef to `HarmonyLib.Harmony::Patch` in the assembly. **Harmony ID candidates** are #US
  strings passed to `new Harmony(...)`-like call sites (in practice: user strings that look like IDs). They let a log
  line's owner be matched to a mod when it is not a packageId.
- **Skip list** (`DefaultSkip`, glob, case-insensitive, overridable): 0harmony, harmony, hugslib, 0multiplayerapi,
  0prepatcherapi, mono.cecil*, system.*, microsoft.*, newtonsoft.json, unityengine*, assembly-csharp*.
  (Corpus G9: `HugsLib.dll` is read in its own mod, `unlimitedhugs.hugslib`, whose patches are its own;
  it is skipped only as a copy inside another mod. `harmony.OwnLibrary`.)
- **Manual targets** (corpus G9, `internal/harmony/manual.go`): the IL of every method is interpreted
  abstractly to name the original and the patch methods of `Harmony.Patch` calls, and of calls to wrappers
  shaped `(MethodBase, HarmonyMethod, …)` in other assemblies. Unresolved originals are stored as dynamic facts
  `<manual: reason>` (patch method known); `harmony.manual` says how many calls were resolved and why not.
- Findings: `harmony.shared-target` (info, one per method ≥ 2 mods), `harmony.multi-transpiler` (warning, ≥ 2 mods),
  `harmony.skip-prefix` (warning, possible: a bool prefix on a method other mods also patch), `harmony.manual` (info).
- Facts go to schema v5 (`assembly_fact`, `type_fact`, `harmony_fact`, `harmony_id`); analyzer `Version = 3`.
- Fixtures: `testdata/dotnet` holds C# sources for a fake 0Harmony, a fake Assembly-CSharp and four mods, a build script,
  and the built DLLs (committed, 4–5 KB each, so tests need no .NET SDK).

## 3. Judgment calls

1. **`harmony.skip-prefix` is folded by default** (CLI, MCP, TUI) like version.unsupported. On the real environment it was
   134 of 180 warnings and buried order and patch problems. Rows show with `--code harmony.skip-prefix`, `--all`,
   `--subject`, and in `harmony --target`. Severity unchanged (warning). Folded rows are placed first in their severity, so
   the 30-row cut cannot hide them.
2. **Log problems join `problems` only for a selected log** (`log --select`, TUI `l`). A log is evidence about one past
   run; adding it silently would mix runs. The TUI shows "new log (l)" when Player.log is newer than both the last launch
   and the last log looked at (06 §7.3).
3. **Which Player.log**: `env.PlayerLog(userData, GameMM)` (1.7+: `Logs/Player.log`, older: the user data root); if absent,
   the other location. This environment's log is a 1.6 log in the legacy location although the game is 1.7, so the
   fallback is what finds it. The log header prints the log's own version.
4. **Log problem keys cut absolute paths** to their last three parts, so `p:` refs and ignores survive a moved library
   and are deterministic in tests.
5. **Cross-reference referrers** are found by one pass over the enabled mods' effective Defs/Patches looking for
   `>Name<` *and* `<Name>` (ThingDefCountClass lists such as `<costList><Steel>5</Steel>` and `products`). Without the
   second form the B2 fixture found no referrer.
6. **Roles** (03 §6.3): `[Mod]` prefix, file paths and patch-failure matches → source; inactive providers of a missing
   def → dependency; Harmony lines → patcher (matched by patch type, Harmony ID, then packageId); the topmost mod frame
   → source; lower mod frames and type names in the message → mentioned (source for "instantiating a mod of type").
   A patcher is never promoted to source without a frame or file of its own.
7. **Multi-line exception messages**: `Parameter name:`, `Actual value was`, `Object name:` continue the entry. The I4
   self-check found a pasted ArgumentOutOfRangeException split into two entries without this.
8. **files search skips `Languages/`** unless `--languages` (inspector behaviour; translations repeat every key).
9. **ilspycmd**: found via `ARCHO_ILSPYCMD`, PATH, `~/.dotnet/tools`; never installed — the error text gives
   `dotnet tool install --global ilspycmd`. Cache: `<home>/cache/source/<sha256[:20]>/` (whole project, `.done` marker,
   written to `.tmp` and renamed) and `<hash>.types/<Type>.cs` for single types (`-t`, used when the project is not
   cached). The `source_cache` table (schema v6) records each decompile; a deleted cache directory is rebuilt on demand.
   Reference dirs: the game's Managed folder (or game root on 1.7), the DLL's folder, the folder holding 0Harmony.dll.
10. **`source search` budget**: the CLI decompiles everything it needs; MCP `read_source` with `search` defaults to a
    10 s budget and says how many assemblies are pending ("still computing; run again").
11. **MCP = CLI**: every tool builds one CLI argv and returns `cmds.Execute`'s text unchanged (I1). `isError` is set for
    exit 1 and 2; exit 3 (ambiguous reference) is a normal result because the text is the candidate list (I3).
12. **MCP `env` parameter** on every tool (05 §4.1), applied per call.
13. **10 s analysis wait** (05 §4.1): `App.AnalyzeWait` (MCP: 10 s, CLI: unbounded). When the analysis child runs
    longer, the call returns declaration problems with "still computing in the background" and the child keeps going;
    the next call collects it. `def` / `patches` / `harmony` use the previous (stale) run in that window if there is
    one.
14. **`why` lists shared Harmony targets** (05 §3 共同补丁) with each side's patch kinds.
15. Tool input structs carry `game_version` / `env` as direct fields: the SDK's schema inference does not flatten
    embedded structs, so an embedded `GameVersion` was rejected as an unknown property.

## 4. Acceptance

| Test | Where | Result |
|---|---|---|
| B1 patch failure → file:line + §4.4 reason | golden `harmonylog`, `log --cluster E1` | `Patches/Compat.xml:2 … (patch.no-match: target-inactive, unguarded-optional)`, Lib as dependency, confidence certain |
| B2 xref → referrer + provider | golden, E2 | Refer `Defs/Recipes.xml:5` source; Lib dependency "defines … but is not enabled" |
| B3 Pawn.Tick, 3 mods' Harmony lines | golden, E3 | Mod A/B/C all `patcher`, none `source`; checklist + `harmony --target` / `source search "AllComps.Remove("` next |
| B4 500 repeats | golden, E3 | one cluster `x500` |
| B5 inspector replay | `internal/app/m4_test.go` | selector vs active state; files search excludes Languages and carries mod refs; decompile cache survives a deleted cache dir and is invalidated by a changed DLL; copy-local case deferred to M5 |
| C5 multi-transpiler | golden `problems --code harmony` | `harmony.multi-transpiler` for Mod B + Mod C on Verse.Pawn.Tick |
| C6 manual only | golden `harmony --mod` | `calls Harmony.Patch from code`; `harmony.manual` info; not "no patches" |
| I1 CLI = MCP | `mcp_test.TestCLIParity` | 19 calls byte-identical over in-memory transports |
| I2 refs reusable | `TestRefsRoundTrip` | every `#id`, `pkg:`, `p:`, `log:E#`, `def:` printed is accepted |
| I3 ambiguous pkg: | `TestAmbiguousPackage` | candidates listed, none picked |
| I4 agent without skills | `docs/m4-mcp-transcripts.md` (`spikes/mcpcheck`) | patch failure: E15 → `Progression production patch.xml:40` target-inactive → VFE Tribals disabled. Pawn.Tick: pasted stack → God Hands + Yayo's Combat as patchers, source search on both |
| I5 ≤ 40 lines + hints | `TestCLIParity`, `TestTruncation` | all default outputs ≤ 40 lines; cut lists name `--all` |

Also: annotations (`readOnlyHint` for the 12 read tools, `destructiveHint` true only for `save_loadout`), every
description contains an example call, write tools tested on a synthetic env only, `env` and still-computing tests.

## 5. Measurements (real environment, read-only)

| | |
|---|---|
| Environment | 1354 mods (485 enabled), game 1.7.4982, ModsConfig.xml for 1.6 |
| Harmony layer | 2524 patches in 451 assemblies, 42 240 types; all 1541 DLLs of all version folders read in 0.8 s, 0 errors |
| Full analysis with Harmony | 4.6 s, peak working set 311 MB (child process, under the 400 MB target) |
| Findings | harmony: 12 multi-transpiler, 134 skip-prefix, 296 shared-target, 97 manual |
| Player.log (1.6, 5814 lines, 8.7 MB) | 153 clusters (patch-failed 14, xref-missing 72, exception 13, xml-load 9, harmony-stack 4, other 41); `archo log` 0.57 s end to end |
| I4 self-check | 13 MCP calls, 3.0 s cold (includes one single-type decompile and decompiling 3 assemblies), 0.6 s warm |
| **TUI idle working set** (scan, browse, Problems + Inspect, `l` log analysis, 2 min idle) | **44.8 MB** (peak 53.3 MB, private 81.2 MB); gate ≤ 65 MB met. Before log parsing: 32.4 MB |

The log analysis adds about 12 MB to the TUI process (parsed clusters and the attribution context are kept for the
selected log). It stays well under the gate, so cgo SQLite is still not needed (ruling 5).

## 6. Open questions

1. Is folding `harmony.skip-prefix` right, or should it be info (it is "possible" by construction)?
2. Should a newer Player.log be selected automatically (log problems always in `problems`), instead of on `l` /
   `--select`?
3. When the log's game version differs from the environment's (here 1.6 log, 1.7 game), attribution still uses the
   current Loadout and 1.7 analysis. Warn in the header, or analyse with `--game-version` of the log automatically?
4. `source search --active` over every enabled mod decompiles ~450 assemblies (minutes, a few hundred MB of cache).
   Pre-decompile in the background after analysis, or keep it on demand with the MCP budget?
5. `harmony.manual` (97 mods) cannot say which methods are patched. A later step could scan IL for
   `ldtoken`/`GetMethod("X")` near `Harmony.Patch` calls. Worth it?

## 7. Main-session rulings (2026-10-19, after M4 review)

| # | Ruling |
|---|---|
| 1 | `harmony.skip-prefix` defaults to **info**. It escalates to warning only when the patched method shows up in a cluster of the selected log (stack frame or Harmony patch line). That gives 134 warnings → a handful with evidence. |
| 2 | Yes: `problems` (CLI/MCP) includes the newest Player.log automatically when it is newer than the last analysed one. The TUI shows the "new log" badge and parses it lazily when the Problems view is opened, to keep idle memory down. |
| 3 | When the log's game version differs from the environment's, attribute against the log's major.minor automatically (implicit `--game-version`), and say so in the log header: `analysed as 1.6 (log version); environment is 1.7`. |
| 4 | Keep decompiling on demand; no background pre-decompile (resource principle 00 §2.5). `source search --active` stays, but it first prints the estimated assembly count and cache size, and needs `--yes` in CLI. In MCP it is capped to the mods implicated by the current problem/log unless `scope: "active"` is passed explicitly. |
| 5 | IL scanning for manual Harmony targets goes to the post-MVP backlog (docs/backlog.md). |
