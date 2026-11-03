# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## MCP测试
基于日志语料的真实mod保存日志和测试题

## Decompilation

Decompilation serves two purposes: investigating Mod conflicts and creating Mods. Reconsider storing decompiled assemblies in the cache and propose an approach.

2026-11-01: decided in [research/2026-11-01_decompile-storage.md](docs/research/2026-11-01_decompile-storage.md): a durable source library including the game's own code; being implemented. Mod projects are deferred (New Mod Project below).

### Decompilation management
管理反编译完成的mod instance

## New Mod Project
Allow player to initialize a new mod project

## Research displaying, editing, and managing Mod configuration

Mod configuration determines how a Mod's content is played. In the game it has an interface; outside the game it is plain XML.

2026-11-02: researched in [research/2026-11-02_mod-settings.md](docs/research/2026-11-02_mod-settings.md). First finding: settings files are named after the folder the game sees, so `_Archo_` and `!ArchoPack_` links fork a Mod's settings (01 §5.3). Direction (simplified 2026-11-02, no tricks, low player load):

- Settings follow the Mod: when a deployment name changes, RimArcho copies the settings to the new name before launch, without asking and without overwriting. Implemented 2026-11-03 (08 §3, §5.3 step 4, §12); real-game check in the release VM tests.
- Archo Monitor records which settings file each Mod uses and what its settings window shows when the player opens it. The file record is implemented 2026-11-03 (Monitor 0.10.0, 03 §6.7.1); the window record is not.
- RimArcho reads simple files (on/off, numbers, text, a choice) itself; it can change only those, only with the game closed, with a backup. Everything else is read-only: change it in the game.
- One Mod settings page for the current list. Implemented 2026-11-03, read only, as Mod library › Mod settings (06 §2.9), with `archo mod-settings` and MCP `get_mod_settings`.

Not now: profiles, exports, cleanup.

## Accessibility

Accessibility is intended to let AI agents operate without screenshots. Define acceptance criteria for it.

2026-11-02: criteria defined as 06 §11 A1–A18, with a baseline in [research/2026-11-02_gui-accessibility.md](docs/research/2026-11-02_gui-accessibility.md). Nothing passes in full. Desktop agents already read the page through COM UIA (A17 partial; the baseline's "only the window" came from the .NET client). Next: hide the editor behind secondary pages, give lists and tables row semantics, and give dialogs a focus boundary.

2026-11-02 (later): those three are done (A1, A6, A8, A9, A15 pass). Next: name the remaining fields and icons (A2, A10), tell same-name controls apart (A5), menu and command-panel semantics (A7), progress and messages as status (A11, A12).

2026-11-03: progress and messages done (A11 passes, A12 partial: some messages don't name their subject, and outcomes other than tasks are gone after navigation). Next: A2, A10, A5, A7.

## Save analysis

### Save management

Preview the colony name, in-game date, storyteller and difficulty, play time, colonist count, save size, and Mod list.

Group saves by colony using RimSaves' folder logic. RimSaves stores the folder name in the filename as folder#§#save-name.rws. Recognize that prefix so its users' saves are not grouped incorrectly.

Support renaming and copying saves, and offer cleanup suggestions.

### Mods used by a save

Explain the effect of the save's Mod list. List objects belonging to Mod X, for example 3 colonists of its race, 12 buildings, 1 faction, and 1 WorldComponent.

Close the gap in 10 §3.3: after extracting Class, functional Mods with Comp or Component classes leave traces in saves, so their usage can be determined at least partially.

Answer the common community question of whether a Mod can be removed mid-playthrough. Compare the current Mod list with the save's Mod list.

### Detect save corruption

At the file level, detect truncation (no </savegame>, often saved during a crash), empty files, and malformed XML. Do not report a save as corrupt while the game is writing it; check running-game detection in 08 §4 first.

At the reference level, collect defined and referenced loadIDs statically and find references to nonexistent objects. This can reveal Could not resolve reference before loading.

At the source level, Class="..." pointing to a type absent from installed assemblies will fail on load and is the most severe case. An unresolved Def source (already recorded in 10 §1.5) should become a risk warning.

Connect the checks to logs: Monitor already records context.save, so attach save-load errors from the last attempt to the save card, showing static checks alongside actual loading evidence.

Undecided: whether to generate a repaired copy by removing invalid references and objects belonging to missing Mods.

### Playthrough review (data for the third question)

Join multiple saves of a colony into a timeline: in-game days, real play time, population, wealth, and deaths.

Include research progress (the undecided item in 10 §1.4) and DLC in content not yet played.

Optionally count occurrences instead of only recording that something appeared. This costs little and can show how deeply the content was played.

## Archo Agent

Archo should be a Mod development agent that can take action, not just translate or tag. Tentatively use a modified Pi agent; the approach needs research.

# Deferred

## TUI

The TUI lacks a navigation bar and feels slow during use.

## Loading animation
Maybe add a UI loading animation--low priority for now

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Deferred (2026-11-01): the estimate is good enough for now, so no real-hardware run is planned. If false warnings are ever reported, run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).