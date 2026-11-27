# 21 · Looks

> Status: in test (the interface says so and asks for reports, §6). §5 (styles and random variants) and the MCP tool are not implemented.
> Basis: 05 §1 (effective content), 07 §4 (writing to the game), 09 §3 (link deployment), 02 §1 (file writes).

When several Mods ship a texture at the same path, the game shows the file of the Mod that loads last. Looks lets the player see every such case in a plan and choose which Mod's textures the game shows, without changing the load order and without touching any Mod.

## 1. Terms

- **Texture**: one image file under a load folder's `Textures/` (`.png`, `.dds`, `.jpg`, `.jpeg`, `.psd`). Its **key** is the game's content path: relative to `Textures/`, lower case, forward slashes, no extension. A `.png` and a `.dds` with one key in one Mod are one texture in two formats.
- **Look**: the unit the player chooses: the textures that together draw one thing. Its **id** is the lower-case base path (§2.2). A Def is an index on top of a look: several Defs may use one look, one Def owns several looks, and a look may have no Def (interface icons, textures loaded by code).
- **Slot**: one texture position inside a look: a facing (`south`, `east`, `north`, `west`), `single` (no facing), a worn body type and facing (`female_south`), or one file of a folder look (the file name). A mask (`…_m`, or a facing followed by `m` as in `Dog_southm`; `…_mask` as slot name) is not chosen on its own: it follows its texture (§3.2).
- **Provider**: a Mod of the plan that supplies at least one slot of a look, with its position in the plan.
- **Override chain**: a slot's providers in load order. The last one is what the game shows.
- **In game**: what the game shows for a slot with this plan and game version, after the load order and the player's choices.
- **Choice**: the player's decision that a look, or some of its slots, comes from a named Mod. Scope: one plan, or all plans. A plan's choice wins over the one for all plans.
- **`_Archo_Looks`**, **`_Archo_Styles`**: the two generated Mods (§4, §5).

The interface and the CLI say look, texture and in game. They do not say picture, image or appearance (the game uses "appearance" for `Graphic_Appearances`).

## 2. The index

Package `internal/looks` (read-only; listed in `TestArchReadOnlyPackages`). Input: the plan's Mods in load order, each with its effective load folders for the game version (`content.Resolve`, 05 §1). Official content (Core, DLC) ships no loose textures and supplies none; a comparison with the game's own textures is not implemented.

### 2.1 Textures

For each Mod, walk `Textures/` of each effective load folder, highest priority first (the rules of `textures.Collect`: any letter case of `Textures`, regular files only). Inside one Mod, the highest-priority load folder that has a key supplies it. Read width, height and format from the file header (PNG IHDR; DDS header: size and FourCC or DXGI format; JPEG SOF); never decode pixels for the index. Two providers' files are **identical** when extension, size and content hash match; only files that another provider also supplies with the same extension and size are hashed. An identical copy is recorded and never counts as an override.

### 2.2 Grouping keys into looks

In this order:

1. **Folder looks.** A folder that a Def names as the `texPath` of a `Graphic_Random`, `Graphic_Collection` (and subclasses), `Graphic_Cluster`, `Graphic_StackCount` or `Graphic_Appearances` graphic is one look of kind `folder`; its id is the folder path and each file is a slot named by its lower-case file name. Without Def data nothing is a folder look.
2. **Worn looks.** Cut a trailing `_m` (mask), then a trailing `_south`, `_east`, `_north` or `_west`. If the rest equals a `wornGraphicPath` value, or that value plus `_<bodytype>`, the key belongs to the look whose id is that path, kind `worn`; the slot is the facing, or `<bodytype>_<facing>`. A key equal to the path with no facing is the slot `single` of the same look.
3. **Plain looks.** Cut the mask and facing suffixes as above; the rest is the look id, kind `plain`; the slot is the facing, or `single`.

A look's category is the first three folders of its path. A missing `west` is `east` mirrored and is not a slot.

### 2.3 Defs

Read `Defs/**/*.xml` of every Mod in the plan (official content included) from its effective load folders. Resolve `ParentName` inheritance for the fields used here. Record, per look id, the Defs that reference it: Def type, defName, label, field (`graphicData/texPath`, `apparel/wornGraphicPath`, `uiIconPath`, `iconPath`), graphic class, and the Mod that defines the Def. Record `ThingStyleDef` (its `graphicData` and `wornGraphicPath`) and `StyleCategoryDef/thingDefStyles` (thing Def → style Def), so a look knows which thing Defs and style categories use it. Patch operations are not applied: a texture path set only by a patch counts as "no Def".

### 2.4 Result per look

Id, kind, category, tags (`style`, `random`, `collection`, `cluster`, `stackcount`, `appearance`), Defs, styles, and its providers in load order, each with its slots: file path (relative to the Mod folder, original case), format, width, height, bytes, the second format of the same key if any, and "identical to provider N".

For a slot, the **load-order result** is the last provider that supplies it. For a look:

- **overridden** (of provider P): a later provider's different content wins at least one slot P supplies.
- **overriding** (of provider P): P wins a slot over an earlier provider's different content.
- **mixed**: the in-game result takes slots from more than one Mod. Folder looks are never called mixed: the game merges a folder's files across Mods by design.

### 2.5 Cost and cache

Targets and measurements: 18 §2. Header reads and hashes are cached in `<data directory>/cache/looks-v1.json`, keyed by Mod folder and validated per file by size and modification time (`fsscan.OpenCache`). The index of one plan is built on demand with bounded workers (`fsscan.Workers`, at most 8) and is not stored; `archo looks` and the interface read through the same function.

## 3. Choices

### 3.1 Storage

Table `look_choice` (04 §4): scope (`all`, or a plan ID), look id, slot (`all` for the whole look, else a slot name), the chosen Mod's Lineage, and the time. One row per scope, look and slot. A choice names the Lineage (00a §4.3: player data attaches to Lineage), so it survives an update, a locked version or a rename; it applies when an Instance of that Lineage is in the plan.

### 3.2 Resolution

For a slot: the plan's choice for that slot, else the plan's choice for `all`, else the same two for scope `all`. It applies when the chosen Mod is in the plan and supplies the slot; otherwise the load-order result stands. A mask slot comes from the Mod that wins its texture when that Mod has the mask; otherwise from the last provider that has the mask. A second format (`.dds` beside `.png`) always follows its texture.

### 3.3 Operations

- **apply** (look, Mod, what, scope): `what` is the entire look or a list of slots. Writes choices only. Never changes the plan's order. Warns, without refusing, when the result is mixed.
- **undo** (look, scope): removes that look's choices in that scope only: never another look's, never the other scope's.
- The plan editor's rules card is the only place that changes load order.

### 3.4 States of a choice

Computed whenever the index is built; never stored:

| State | Meaning | What the game gets |
| --- | --- | --- |
| applied | The chosen Mod is in the plan and supplies the slot | The chosen file |
| not needed | The load order already gives the chosen file, or an identical one | Nothing is written for it; the choice is kept |
| asleep | No Instance of the chosen Lineage is in the plan | The load-order result; the choice wakes when the Mod returns |
| broken | The Mod is in the plan but no longer supplies that slot | The load-order result; shown with a count until the player undoes it or the texture returns |

A choice is never deleted automatically.

## 4. `_Archo_Looks`

A generated Mod: `About/About.xml` (package ID `rimarcho.looks`, name `_Archo_Looks`, the game versions RimArcho supports) and `Textures/…`. No Defs, no assemblies. Saves do not depend on it.

- **Where**: `<looks directory>/Looks/`: a shared extra source like Archo Monitor's (03 §2). The game reaches it through the link `_Archo_Looks` in `Mods/` (09 §3), made by the ordinary link deployment. The looks directory is `looks.dir` (config). When it is unset, the first apply chooses it and stores it there: on the volume of the current game installation, so that files can be hard links to Workshop and local Mods: `<local folder>/looks` (17 §3) when that is the game's volume, else `<volume root>/RimArcho/looks` (for example `D:\RimArcho\looks`); when that folder cannot be created, `<local folder>/looks`. The confirmation of the first apply shows the folder. It must not contain or be contained by the data directory or a source root.
- **Contents**: for every slot whose state is "applied", the chosen file under its key's path, with its second format and its mask. Each file is a hard link to the Mod's file when both are on one volume, else a copy. RimArcho records every file it writes there (`look_file`: path, source path, size, modification time, linked or copied) and never writes through a link into a Mod.
- **Folder looks**: a file can replace one the load order gives, not hide one. Applying a Mod to a folder look replaces the files both Mods name alike; files that only other Mods add stay. The interface says so.
- **Rebuild**: from the choices, never edited in place: build beside it, then swap. It runs after apply or undo for the current plan, and before every write to the game (07 §4) for the plan being written, so a Mod update that replaced a linked file is picked up. A rebuild whose result equals what is on disk writes nothing. Refused while a game runs (09 §4).
- **In the plan**: an ordinary entry, always last. The first apply adds it to the plan as part of the same undoable edit; a sort keeps it last (08 §1.1). An empty `_Archo_Looks` loads nothing.
- **One folder for all plans and games**: the folder holds the result for the plan last written to a game. Two games that run at the same time on different plans would share it; the second write is refused while the first game runs.
- **Failure**: when a rebuild fails, the write to the game proceeds without the `_Archo_Looks` entry, the result says why, and the choices stay.
- **Texture optimisation** (10 §2) never writes into the looks directory.

## 5. `_Archo_Styles` (not implemented)

A generated Mod with Defs that keeps a Mod's version of a look beside the one in the game.

- **Style**: a new `ThingStyleDef` for the look's thing Def, added to an existing `StyleCategoryDef` by a patch that `MayRequire`s the category's Mod. The option "allow new items to receive this style at random" also adds it to the thing Def's `randomStyle`. When the category already has a style for that thing, the game uses one of them per ideoligion (`StyleCategoryDef.GetStyleForThingDef`), not per item: the interface says so. Without Ideology the style loads and other Mods can apply it.
- **Random variant** of the default look or of an existing style: a patch turns that `graphicData` into `Graphic_Random` over a folder inside `_Archo_Styles` that holds the original texture and the added one. Items already in a save may change look. Not offered for a look no Def uses.
- Styles apply to all plans; a plan decides only whether `_Archo_Styles` is enabled.
- Fallback: when the category's Mod or the thing Def is absent, that entry is skipped and the rest load.

## 6. Interface

Page design: 16 §2.5. CLI: 13 §4.2 `looks`. The first apply in a home opens a confirmation that says what will be written (§4), that the feature is in test, and how to report a problem; "do not show this again" is the setting `ui.gui.looks.confirmed`.

## 7. Acceptance

- `internal/looks` tests on synthetic Mods (`internal/testkit`): grouping of facings, masks, worn body types and folder looks; load-folder priority inside a Mod; identical copies; overridden, overriding and mixed; Def and style references through `ParentName`.
- `internal/app` tests: choice resolution order (plan over all, slot over `all`); the four states; a rebuild writes hard links on one volume and records them; an unchanged rebuild writes nothing; undo touches one look and one scope; a failed rebuild leaves the game write without the entry; nothing is written into a Mod folder.
- Real game, in the VM (UserGuide/release-vm-tests.md): apply a look from an earlier Mod, launch, see it in the game; undo, launch, see the load-order result.
