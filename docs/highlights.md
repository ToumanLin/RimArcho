# RimArcho highlights

English | [&#31616;&#20307;&#20013;&#25991;](highlights.zh-CN.md)

Release copy for RimArcho 1.0.5: a tagline, blurbs of three lengths, and the selling points behind them. Every claim here is implemented; numbers carry their conditions. Details are in the [user guide](user-guide.md).

## Tagline

**RimArcho — the RimWorld launcher that knows why your game broke.**

Alternatives:
- A one-stop RimWorld launcher that understands its players.
- Pick, sort, check, launch, and find out why. All in one place.

## One sentence

RimArcho is a free RimWorld launcher and Mod manager for players with hundreds of Mods: it sorts your list, checks that it will run, launches the game, and traces errors back to the Mods that caused them, with the evidence.

## Short blurb (about 80 words)

RimArcho brings everything a heavily modded RimWorld needs into one window. Edit lists with full undo, sort with explanations, and see before you launch what will break: missing dependencies, conflicts, failing XML patches and clashing Harmony patches. After you play, it reads the log, groups repeated errors and names the Mod responsible, with the evidence. Lock Mod versions, freeze modpacks with their own saves, check whether old saves still load, and let AI agents help through MCP.

## Long blurb (about 200 words)

RimWorld with 300 Mods is a different game, and keeping it running is a job of its own. RimArcho is built for that job.

**Before you play**, it checks your list the way the game reads it: dependencies, incompatibilities, load order, game versions, duplicate copies, and the content itself. It merges every Mod's Defs and simulates their XML patches in order, so a failing patch comes with its file, line and a reason such as "the target Mod is disabled". It reads Harmony patches straight from the Mods' code. Most fixes are one key, and every edit can be undone.

**After you play**, it reads Player.log while the game runs, folds hundreds of repeats into one line, and tells apart the Mod that caused an error from the Mods that merely appear in its stack. The bundled Archo Monitor adds what the log leaves out: which file was loading, and in which phase.

**Around that**: lock a Mod version against Workshop updates, freeze a whole list into a pack with its own saves, check what an old save loses without a missing Mod, run several game versions side by side, and subscribe from the Workshop, SteamCMD, Git or download pages. Moving from RimSort or RimCrow takes one import.

## Selling points

### It explains, it does not guess
- **Every conclusion shows its evidence** and where it comes from: the Mod's own declaration, inference, community data, the game log, or your rules.
- **XML patch failures with a reason**: `target-inactive`, `target-missing`, `stale-name`, `node-absent`, `wrong-version-route`… with file and line, from simulating the patches the way the game applies them.
- **Harmony analysis without starting the game**: shared targets, prefixes that can skip the original, multiple transpilers, same-named DLLs that hide each other.
- **Error attribution with roles**: source, patcher, dependency, mentioned. A Harmony patcher in a stack trace is not automatically the culprit.
- **Follows the game's own rules**: version folders, `LoadFolders.xml`, Def inheritance after patching. Reports cover only files the game will load.

### Learns from the community
- **Data from 40,326 player-shared logs**: dependencies Mods forget to declare, combinations that tend to fail, and 3,947 known errors of specific Mods, built into the program.
- **Severities measured, not assumed**: how strongly each kind of problem predicts real errors was measured on 6,734 public logs, so harmless rule breaks stay quiet.
- **Stand-ins count**: `alternativePackageIds` and "Use This Instead" replacements satisfy dependencies, so a translation for an abandoned Mod works with its continuation.

### Safe by design
- **List edits never reach the game until you save or launch.** They change a working copy with 100 steps of undo and a full version history.
- **Backed-up, atomic writes** of `ModsConfig.xml`; changes made in the game's Mod menu or another manager are detected and kept, never silently overwritten.
- **Your Mods, saves and packs go to the recycle bin** when removed, other Mods enter the game through links instead of copies, Mod files change only when you ask (un-hide, texture conversion), and uninstall previews every step.
- **Staging**: set a batch of suspects aside and put them all back at their old positions with one key.

### Built for big libraries
- **Every copy tracked**: Workshop, local, SteamCMD, extra folders, locked versions. Updates, moves and copies between scans are recorded; a missing Mod stays in your list and reconnects when it returns.
- **Lock a version** against Workshop updates, compare when an update arrives, and update when you are ready.
- **Packs**: freeze a list into exact Mod files with their own saves; export as `.archopack`, optionally with slimmed textures.
- **Tags view and Tag manager**: group, rename, merge and annotate tags across thousands of Mods.
- **Updates labelled by impact**: Affects launch, Worth a look, Content only. Update all in one batch.
- **Texture optimisation**: convert PNG to DDS for faster loading, keeping the originals and a one-click undo.
- **Search inside every Mod**: Defs, patches and decompiled C#, streamed as results are found.
- **Fuzzy command panel** (`ctrl+k`): `lib hug` finds HugsLib, `vanilka expnded` finds Vanilla Expanded.

### Beyond the Mod list
- **Can it load?** for every save: "loading without it loses 3 colonists of race Kurin_Race, 1 faction, 3 things", with a one-step fix.
- **Colony history** from your saves: wealth, colonists, events, on a timeline.
- **Several game versions side by side**, each with its own saves if you like; download another version with SteamCMD.
- **Live log while you play**, and a memory check before launch that warns when your RAM looks short.
- **Subscriptions from anywhere**: Steam client, SteamCMD (no Steam client needed), Git branches, tags or commits, and download pages including LoversLab. SteamCMD batches can later move to real Steam subscriptions that credit the authors.
- **Works without a Steam account** for managing local games and Mods.

### One program, four ways in
- **Desktop window**, **terminal UI**, **scriptable CLI** and **MCP server**, on the same code with the same facts; the window and terminal share most keys.
- **Screen-reader friendly**: pages, controls, rows and states are named, and progress and errors are announced.
- **AI agents as first-class users**: connect Claude Code, Codex or any MCP client with one line. The agent sees the same evidence, its edits go into your undo history, and read-only tools are marked as such.
- **Test runs** (CLI and agents): start the real game with throwaway data, reach the menu or a test map, and compare errors with your last game.
- **English and Simplified Chinese**, switchable without a restart.

### Light on your computer
Measured on Windows 11 with an SSD, 1,354 Mod folders and a 485-Mod list unless noted:
- The terminal UI shows its first screen in 0.10 s (0.35 s on the first start after a scan) and idles at about 47 MB.
- With a synthetic 6,574-copy library and 601 Mods enabled, the window uses about 180 MB at start and 289 MB after visiting every page (whole process tree, one run), and 0.2–1.1 % of one CPU core while resting on the start, Mod library, can it run and settings pages.
- Search files returns its first match in 34 ms across 1,354 Mods (synthetic library).
- No background service: RimArcho uses nothing once you close it.

## Moving from another manager

RimArcho finds RimSort lists and user rules, and RimCrow databases and saved lists, previews them, and imports lists, aliases, notes, groups as tags and rules. Your old manager's files are left untouched. It also reads RimSort, RimCrow and RimWorld list formats, share codes, `.rml`, saves, Rentry pages and Steam collections.

## Coming next

Say "planned", not "available", for these:
- **Archo**, the built-in AI assistant (1.1). Outside agents can connect through MCP today.
- **What haven't I played yet?** (1.1): finding Mods and content you never touched across your colonies.
- **One-click subscription to a whole Workshop collection** and playthrough themes (1.2).
- **What's worth playing?** (later): shared packs and suggestions based on what you have not played.

## Requirements

- Windows 10 or 11 (64-bit). The window uses Microsoft Edge WebView2, already part of Windows.
- Made for RimWorld 1.6 and 1.7; older installs can be managed too.
- Optional: ilspycmd (needs the .NET SDK) to read Mods' C# code.
- Free and open source under the MIT licence. The programs are not code-signed, so Windows SmartScreen may ask once.
