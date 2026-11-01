# Decompiled code: where it lives (proposal, 2026-11-01)

Status: proposal, waiting for the owner's decision. Facts below come from a code survey of `dev/1.0` (Codex task `task_62077f853a`, report kept outside the repository).

Decompiling serves two purposes: finding out why mods conflict, and making mods. The current design serves neither well.

## What is wrong today

1. **It is a cache.** Output goes to `<home>/cache/source/<first 20 hex of the DLL's SHA-256>/` (`internal/app/source.go`). Settings › clear cache deletes it; folder names mean nothing to a player, an IDE or an agent; a path handed out by `archo source` or MCP `read_source` can vanish.
2. **The game's own code is missing.** `assembly-csharp*.dll` is skipped (`harmony.SkippedFor`) and no command decompiles the game. Yet the game method a Harmony patch targets is the first thing both purposes need.
3. **Identity is loose.** Output is keyed by DLL bytes only. Two copies with the same DLL share a folder but search reports each hit twice, once per copy. Old hash folders stay forever after a mod updates: no size cap, no sweep.
4. **It is not a project.** References passed to ilspycmd are the game's `Managed` folder, the DLL's own folder and one Harmony folder; nothing builds it. Editing a cached file would be lost on the next clear.
5. Small spec drift: 03 §5.4 says every decompile is recorded and the budget is 60 s; type-only decompiles and shared hits are not recorded, and the budget only stops new work (in-flight work finishes).

## Proposal: two places with different lifetimes

### A. The source library (read-only, durable, regenerable)

A managed directory like the others (07 §3): default `<home>/source`, movable in Settings (for example to `D:\RimWorld Source`). Clear cache does not touch it.

```
source\
  game\1.6.4633\Assembly-CSharp\Verse\Pawn_HealthTracker.cs
  mods\<packageId>\<DLL name> <short hash>\<namespaces>\*.cs
```

- One folder per DLL content, named so a person can read it. A table maps each installed copy (instance) to its folders, so search and MCP show "same code as copy X" instead of duplicate hits, and a path stays valid while any copy uses it.
- The game's code is decompiled once per game build, on first need (C# search with "RimWorld's own code" ticked, or opening the original method from a Harmony patch). Local only: never in packs, feedback or exports.
- Still on demand (03 §5.4), plus one button to decompile the current list's mods ahead of time for people who search C# often.
- Settings shows its size and "remove unused": folders no installed or locked copy uses any more.
- An optional workspace file (`RimWorld Source.code-workspace`) lists the game and the current list's mods, so VS Code or Rider can jump between a patch and the game method.

Migration: none needed. The old `cache/source` is dropped; folders are rebuilt on demand.

### B. Conflict research uses the library

- Harmony facts already say who patches which method. The library adds the code: the original method from `game\…`, then each prefix, postfix and transpiler from `mods\…`, in load order. This is the strip in the Search files mockup's C# view.
- Search files › C# code searches chosen copies (each labelled with its source and date) plus, optionally, the game.
- `archo source` / MCP `read_source` return library paths, which stay valid.

### C. Making mods: Mod projects (editable, owned by the player)

Never inside the cache or the library.

- "New Mod project" asks for a folder (default `Documents\RimWorld Mods\<name>`) and writes About/About.xml, `1.6/Defs`, `1.6/Patches`, `Source/<name>.csproj` (.NET Framework 4.8, HintPath references to the game's `Managed` DLLs and Harmony with `Private=false`, output to `1.6/Assemblies`), like RimDevTools' `ProjectTemplate`.
- "Patch this method" (from a Harmony conflict or the C# view) adds a Harmony patch skeleton for that method, and a reference to the target mod's DLL when it patches a mod. Archo (09) fills in the code.
- The project is linked into the Mods folder (a junction), so a list can enable it; build, launch and read the log is Archo's loop.
- "Copy into a Mod project" copies one decompiled file as a starting point; the library itself stays read-only.

## Decisions needed

1. Library location: `<home>/source` by default and movable (proposed), or ask on first use.
2. Unused folders: removed only by the Settings button (proposed), or also automatically after 30 days.
3. Mod projects: part of 1.0, or together with Archo later.

Specifications to change once decided: 03 §5.4 (decompile), 07 §3 (data directory layout), 06 §2.9 (Search files), 09 (Archo's tools).
