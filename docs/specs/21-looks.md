# 21 · Looks

> Status: in test (the interface says so and asks for reports, §6). Implemented: the index (§2), choices (§3), `RimArcho Looks Preference` (§4), `RimArcho Styles Preference` with its store, functions, CLI, view models and window calls (§5; no XML it generates has run in a game yet, §5.4), the CLI (13 §4.2 `looks`), and the window's looks view with its first-apply confirmation (§6; 16 §2.5 Looks view; written and statically tested, not yet checked in a window). Not implemented: the page's "add" row for §5 and the MCP tool.
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
- **`RimArcho Looks Preference`**, **`RimArcho Styles Preference`**: the two generated Mods (§4, §5).

The interface and the CLI say look, texture and in game. They do not say picture, image or appearance (the game uses "appearance" for `Graphic_Appearances`).

## 2. The index

Package `internal/looks` (read-only; listed in `TestArchReadOnlyPackages`). Input: the plan's Mods in load order, each with its effective load folders for the game version (`content.Resolve`, 05 §1). Official content (Core, DLC) ships no loose textures and supplies none; a comparison with the game's own textures is not implemented.

### 2.1 Textures

For each Mod, walk `Textures/` of each effective load folder, highest priority first (the rules of `textures.Collect`: any letter case of `Textures`, regular files only). Inside one Mod, the highest-priority load folder that has a key supplies it; the files of that key in its lower-priority folders are kept with it (`File.Under`: path and size only), for a pack build (10 §2 step 6). Read width, height and format from the file header (PNG IHDR; DDS header: size and FourCC or DXGI format; JPEG SOF); never decode pixels for the index. Two providers' files are **identical** when extension, size and content hash match; only files that another provider also supplies with the same extension and size are hashed. An identical copy is recorded and never counts as an override.

### 2.2 Grouping keys into looks

In this order:

1. **Folder looks.** A folder that a Def names as the `texPath` of a `Graphic_Random`, `Graphic_Collection` (and subclasses), `Graphic_Cluster`, `Graphic_StackCount` or `Graphic_Appearances` graphic is one look of kind `folder`; its id is the folder path and each file is a slot named by its lower-case file name. Without Def data nothing is a folder look.
2. **Worn looks.** Cut a trailing `_m` (mask), then a trailing `_south`, `_east`, `_north` or `_west`. If the rest equals a `wornGraphicPath` value, or that value plus `_<bodytype>`, the key belongs to the look whose id is that path, kind `worn`; the slot is the facing, or `<bodytype>_<facing>`. A key equal to the path with no facing is the slot `single` of the same look.
3. **Plain looks.** Cut the mask and facing suffixes as above; the rest is the look id, kind `plain`; the slot is the facing, or `single`.

A look's category is the first three folders of its path. A missing `west` is `east` mirrored and is not a slot.

### 2.3 Defs

Read `Defs/**/*.xml` of every Mod in the plan (official content included) from its effective load folders. Resolve `ParentName` inheritance for the fields used here. Record, per look id, the Defs that reference it: Def type, defName, label, field (`graphicData/texPath`, `apparel/wornGraphicPath`, `uiIconPath`, `iconPath`), graphic class, and the Mod that defines the Def. Record `ThingStyleDef` (its `graphicData` and `wornGraphicPath`) and `StyleCategoryDef/thingDefStyles` (thing Def → style Def), so a look knows which thing Defs and style categories use it. Record, for every concrete `ThingDef` whose graphic or worn path leads to a look, its `graphicData` (path as written, graphic class and the other children as XML, with what it inherits) and its worn path (`Index.Things`): §5 writes style Defs from them. Patch operations are not applied: a texture path set only by a patch counts as "no Def".

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

Table `look_choice` (04 §4; `internal/store/looks.go`): scope (`all`, or a plan ID), look id, slot (`all` for the whole look, else a slot name), the chosen Mod's Lineage, and the time. One row per scope, look and slot. A choice names the Lineage (00a §4.3: player data attaches to Lineage), so it survives an update, a locked version or a rename; it applies when an Instance of that Lineage is in the plan (the first such entry in load order). Deleting a plan deletes its choices, since plan IDs are reused; the choices for all plans stay.

### 3.2 Resolution

For a slot: the plan's choice for that slot, else the plan's choice for `all`, else the same two for scope `all` (`app.LookState`). The first of these that exists decides the slot, and the others are **overruled** for it: when the deciding choice does not apply, the load-order result stands, not the next choice. It applies when the chosen Mod is in the plan and supplies the slot; otherwise the load-order result stands. A mask slot comes from the Mod that wins its texture when that Mod has the mask; otherwise from the last provider that has the mask. A second format (`.dds` beside `.png`) always follows its texture.

### 3.3 Operations

- **apply** (look, Mod, what, scope; `app.LooksApply`): `what` is the entire look or a list of slots. Writes choices only. Never changes the plan's order (except the `RimArcho Looks Preference` entry, §4). Warns, without refusing, when the result is mixed. Refused when the Mod is not in the current plan, has no texture of the look, or lacks a named slot, and for a mask slot. Choosing the entire look replaces that look's slot choices in the same scope; choosing slots leaves its other choices.
- **undo** (look, scope; `app.LooksUndo`): removes that look's choices in that scope only: never another look's, never the other scope's. Refused when there is none. The plan keeps its `RimArcho Looks Preference` entry.
- Both take a dry run that writes nothing and returns the choices, the states and the rebuild as they would be. Both are operation-log rows (`looks.choice`, 13 §1.3).
- The plan editor's rules card is the only place that changes load order.

### 3.4 States of a choice

Computed whenever the index is built; never stored:

| State | Meaning | What the game gets |
| --- | --- | --- |
| applied | The chosen Mod is in the plan and supplies the slot | The chosen file |
| not needed | The load order already gives the chosen file, or an identical one | Nothing is written for it; the choice is kept |
| asleep | No Instance of the chosen Lineage is in the plan | The load-order result; the choice wakes when the Mod returns |
| broken | The Mod is in the plan but no longer supplies that slot | The load-order result; shown with a count until the player undoes it or the texture returns |

The state of a slot is one of the four. The state of a choice (what the lists count): asleep when its Mod is not in the plan; else applied when any slot it decides is applied, else not needed when any is not needed, else broken. A choice of the whole look by a Mod that has only some of its slots is not broken: the other slots keep the load-order result. "Not needed" also requires that the mask the game would show is the same. A choice overruled for every slot it names (§3.2) has no state and is not counted.

A choice is never deleted automatically (except with its plan, §3.1).

## 4. `RimArcho Looks Preference`

A generated Mod (`internal/app/looksbuild.go`): `About/About.xml` (package ID `rimarcho.looks`, name `RimArcho Looks Preference` (what the game's Mod list and RimArcho show), supported versions 1.5, 1.6 and 1.7 as Archo Monitor's) and `Textures/…`. No Defs, no assemblies. Saves do not depend on it.

- **Where**: `<looks directory>/Looks/`: a shared extra source like Archo Monitor's (03 §2). The game reaches it through the link `_Archo_Looks` in `Mods/` (09 §3), made by the ordinary link deployment. The looks directory is `looks.dir` (config). When it is unset, the first apply chooses it and stores it there: on the volume of the current game installation, so that files can be hard links to Workshop and local Mods: `<local folder>/looks` (17 §3) when that is the game's volume, else `<volume root>/RimArcho/looks` (for example `D:\RimArcho\looks`); when that folder cannot be created, `<local folder>/looks`. The confirmation of the first apply shows the folder. It must not contain or be contained by the data directory or a source root. Once the source `src:looks` is registered, its folder is the looks directory whatever `looks.dir` says, as with the Monitor folder. The first apply creates the folder with the Mod's `About.xml`, registers the source and scans that source alone, so the plan can name the Instance. `app.DeployName` gives this Instance the fixed link name `_Archo_Looks`.
- **Contents**: for every slot whose state is "applied", the chosen file under its key's path (`Textures/<path below the Mod's Textures folder>`, with the Mod's own file name, letter case and extension), with its second format and, when the chosen Mod has one, its mask. Each file is a hard link to the Mod's file when both are on one volume, else a copy; a read-only file is always copied, because deleting a link to it would clear the attribute the Mod's file shares. RimArcho records every file it writes there (`look_file`: path, source path, size, modification time, linked or copied) and never writes through a link into a Mod: every destination is a new path in a new folder, and no existing file is opened for writing.
- **Folder looks**: a file can replace one the load order gives, not hide one. Applying a Mod to a folder look replaces the files both Mods name alike; files that only other Mods add stay. The interface says so.
- **Rebuild**: from the choices, never edited in place: build beside it (`Looks.archo-tmp`), then swap (the folder before goes through `Looks.archo-old`). It runs after apply or undo for the current plan, on `archo looks rebuild`, and before every write to the game (07 §4) of a plan that has the entry, for the plan being written, so a Mod update that replaced a linked file is picked up. A rebuild whose result equals what is on disk (the planned files' path, source, size and modification time equal `look_file`, and every file is there) writes nothing. Otherwise it is refused while a game runs in any environment (09 §4): after an apply or undo the choices are kept and the result says so. A rebuild that writes, is refused or fails is an operation-log row (`looks.build`, 13 §1.3).
- **In the plan**: an ordinary entry, always last. An apply makes it the current plan's last entry as one undoable edit (`app.LooksAddOp`); the choice itself is not in the plan's history and is undone with **undo** (§3.3). A sort keeps it last (08 §1.1). An empty `RimArcho Looks Preference` loads nothing. A plan that has applied choices for all plans but no entry does not show them in the game: the state reports the entry's position (`LooksState.Entry`, 0 when absent), `archo looks` says so, and an apply or `archo looks rebuild` in that plan adds the entry.
- **One folder for all plans and games**: the folder holds the result for the plan last written to a game. Two games that run at the same time on different plans would share it; the second write is refused while the first game runs.
- **Failure**: when a rebuild fails or is refused, the write to the game proceeds without the `RimArcho Looks Preference` entry in ModsConfig.xml, the result says why (`SaveResult.LooksError`; `looks:` in `archo save` and `archo launch`), and the choices and the plan's entry stay. The link in `Mods/` is left as it is, neither made nor removed: a link change is refused while that game runs (09 §3), and without the entry the game does not load the folder. The next write rebuilds.
- **Texture optimisation** (10 §2) never writes into the looks directory.
- **In a pack**: a pack never carries `RimArcho Looks Preference`; it carries the result. The build leaves the Mod out of the pack's list and, after copying, removes from the copies every texture the game would not show with the pack's list and the plan's choices, so that inside the pack the load order alone gives what the choices gave (10 §2 step 6). A choice made after the build needs a new revision of the pack.
- **Not a Mod of the player's**: the actions on a Mod's folder and identity are refused for it and not offered (16 §2.9 "Generated Mods").
- **Uninstall** deletes the generated Mod and the looks directory (02 §1 `looks`).

## 5. `RimArcho Styles Preference`

A second generated Mod (`internal/app/styles.go`, `stylesbuild.go`) that keeps a Mod's version of a look beside the one the game shows: `About/About.xml` (package ID `rimarcho.styles`, name `RimArcho Styles Preference`, supported versions as §4), `Textures/RimArchoStyles/…`, `Defs/RimArchoStyles.xml` and `Patches/RimArchoStyles.xml`. No assemblies. What it shares with §4 (the looks directory and its source, the link, the place at the end of the plan, the rebuild and swap, the failure handling) is one code path (`internal/app/generated.go`); `app.IsGenerated` is the one test for "a Mod RimArcho generates" and `app.IsLooks` / `app.IsStyles` name one of the two.

### 5.1 Entries

Table `look_style` (04 §4; `internal/store/styles.go`): an ID that is never reused, the kind (`style` or `variant`), the look id, the Mod's Lineage, the target, the random option and the time. One row per kind, look, Lineage and target. An entry applies to all plans: it has no scope, and a plan decides only whether `RimArcho Styles Preference` is enabled. An entry is never deleted automatically.

- **Style in a category** (kind `style`, target a `StyleCategoryDef`): for every concrete `ThingDef` that names the look as `graphicData/texPath` or `apparel/wornGraphicPath` (`looks.Index.Things`), one new `ThingStyleDef` named `RimArchoStyle_<entry ID>_<thing defName>`, and a patch that adds the row (thing Def, style Def) to that category's `thingDefStyles`. The Mod's textures of the look are placed under `Textures/RimArchoStyles/Style/<entry ID>/<name>…` with their facing, body-type and mask suffixes (a folder look: the folder with its files), so the style keeps showing that Mod's version whatever wins the look in the game. The style Def repeats the thing Def's `graphicData` (graphic class, draw size, shader and its other children, with what the Def inherits through `ParentName`, merged one level deep) with the new `texPath`; a path of the thing that is not this look (the item on the ground of a worn look, the worn path of a plain look) is written as the thing Def has it. A worn look carries `wornGraphicPath` and every body type the Mod supplies; a body type the Mod does not supply at all takes the file the game shows now, so that no pawn wears a missing texture. With the option "allow new items to receive this style at random" the style is also added to the thing Def's `randomStyle`, and a thing Def that writes no `randomStyleChance` gets `0.5`. When the category already has a style for that thing, the game uses one of them per ideoligion (`StyleCategoryDef.GetStyleForThingDef`), not per item: the choice is flagged (`Shared`) and the interfaces say so.
- **Random variant** (kind `variant`, target `default` or a `ThingStyleDef` the thing already has): a patch turns the target's `graphicData` (the thing Def's own, or that style's) into `Graphic_Random` over the folder `Textures/RimArchoStyles/Random/<key of the target>/`, which holds the texture the game shows there now as `v0` (resolved through the index and the plan's choices, §3.2) and each entry's Mod's version as `v1`, `v2`, … in the order the entries were added. A second variant of the same target extends the same folder. The game picks by the item's ID, so items already in a save may change look.

An add is refused, with a reason the interfaces show (`app.StyleReason`), when the entry would not be active in the current plan. A random variant is refused for:

| Reason (`StyleReason.Code`) | Case |
| --- | --- |
| `kind` | The look, or the target's look, is worn or a folder |
| `nothing` | No concrete `ThingDef` of the plan names the look as its `graphicData/texPath` (a look only patches or code use, an icon) |
| `style` | The target style is not a style of a thing that uses the look |
| `class` | The target's graphic class is neither `Graphic_Single` nor `Graphic_Multi` (stack counts, appearances, linked or Mod classes, no class) |
| `facings` | `Graphic_Multi` with a game before 1.7: only from 1.7 does the game group a folder's files by the name before the first `_`, so that a random set has facings |
| `official` | The texture the game shows for the target is no file of a Mod in the plan (official content ships no loose textures) |
| `shown` | The game already shows that Mod's version there (the same Mod or identical files) |
| `shape` | The Mod lacks the target's texture (`single`, or `south`) |

A style is refused when no thing Def uses the look (`nothing`) or the category is not in the plan (`category`); either kind when the same entry exists (`exists`) or the Mod has no texture of the look.

### 5.2 States

Computed with the index, never stored (`app.StylesState`, one resolver for the list, the add row's choices and the refusal of an add):

| State | Meaning | What the game gets |
| --- | --- | --- |
| active | The Mod is in the plan and supplies the look, and the thing Defs and the category or target are there | The Defs, patches and textures of the entry |
| skipped | The category, the target style or every thing Def is not in the plan, or a reason of §5.1's table holds for this plan | Nothing of the entry |
| broken | The Mod is in the plan but no longer supplies the look | Nothing; shown until the player removes it or the texture returns |
| asleep | No Instance of the Lineage is in the plan | Nothing; it wakes when the Mod returns |

### 5.3 The generated Mod

- **Where**: `<looks directory>/Styles/`, in the same source as `Looks/` (§4 "Where"); the link in `Mods/` is `_Archo_Styles`. The first add creates the looks directory when there is none, as the first apply does.
- **Contents**: only what the active entries of the plan being written need: a skipped, broken or sleeping entry writes no Def, patch or file. Textures are hard links or copies exactly as in §4, recorded in `style_file`; all lie under `Textures/RimArchoStyles/`, a path no other Mod uses (a `Graphic_Random` over a folder takes every Mod's files at that path).
- **Attachments do not break loading.** Each attachment is a `PatchOperationConditional` that tests for its Def first: the thing Def and the category for a style, the thing Def for `randomStyle`, the thing Def or style Def for a random variant; a Def that is absent is skipped and the rest load. `MayRequire` is not used: the game's own files put it on Defs and list items, not on patch operations, and it names a Mod where the test here is for the Def. A `ThingStyleDef` refers to nothing but textures of this Mod, so it loads without Ideology and other Mods can use it. The XML shapes and the official files each was checked against are listed at the top of `stylesbuild.go`.
- **Rebuild**: as §4 "Rebuild" (`Styles.archo-tmp`, `Styles.archo-old`; refused while a game runs; operation-log row `looks.stylebuild`). It runs after an add or a remove, on `archo looks rebuild`, and before every write to the game of a plan that has the entry: one pass (`app.generatedForWrite`) builds one index and rebuilds both generated Mods. A result whose files equal `style_file` and whose two XML files equal what is on disk writes nothing. A removed entry's Defs, patches and files go with the next rebuild.
- **In the plan**: both generated Mods come after every other Mod, `RimArcho Styles Preference` first and `RimArcho Looks Preference` last, so that the looks Mod's textures still win; the styles Mod's own textures and Defs have names nobody else uses and its patches apply to every Mod's Defs whatever its position. An add puts it there as one undoable edit (`app.StylesAddOp`); a remove leaves the entry in the plan; a sort keeps both at the end in that order (08 §1.1). A plan that has active entries but no entry of the Mod does not show them in the game: `StylesState.Entry` is 0, `archo looks style list` says so, and an add or `archo looks rebuild` in that plan adds it.
- **Failure**: a rebuild that fails or is refused leaves only that Mod out of the write (`SaveResult.StylesError`, `styles:` in `archo save` and `archo launch`); `RimArcho Looks Preference` is still written, and the reverse. The entries and the plan's entry stay.
- **Saves**: an item or building that received a style keeps its reference in the save. After the entry is removed, or without the Mod, the game logs the missing `ThingStyleDef` when the save loads and shows the thing in its default look. A random variant leaves nothing in a save.
- **Texture optimisation** (10 §2) and **uninstall** (02 §1 `looks`) treat it as §4's Mod; so do the refused actions (16 §2.9 "Generated Mods").
- **In a pack**: it has Defs, so it is an entry of the pack and copied like any Mod, as it is on disk at the build (10 §2 step 3).

### 5.4 Not checked in a game

The shapes follow the official files and the facts about the game's code this section names, and the tests check the XML and the files. None of it has run in a game. To check there (UserGuide/release-vm-tests.md): a nested `PatchOperationConditional` with only one branch is silent when its test fails; a thing Def becomes styleable by its row in `thingDefStyles` or `randomStyle` alone, without `CompProperties_Styleable`; a `ThingStyleDef` without `graphicData` (a thing Def with only a worn path) is accepted; `Graphic_Random` over a folder with masks (`v0_m`) in 1.6 and with facings (`v0_south`) in 1.7; a `randomStyleChance` inherited from a parent Def is not seen by the patch and is overridden with `0.5`.

## 6. Interface

CLI: 13 §4.2 `looks`; its `--dry-run` shows what the confirmation would.

The window: the plan editor's looks view (16 §2.5 Looks view): the `editor | looks` switch at the top left of the editor, a gallery of looks with the Mods, the load-order rules and the `RimArcho Looks Preference` card beside it, and the looks editor (which texture, compare, override chain, operation) for one look. The page shows no winner, state or prediction of its own: `internal/view/looks.go` builds the view models from `app.LooksState` (the index with the choices resolved): the overview, the gallery, one look's detail and the prediction of an apply (`LooksState.Would`: the look resolved with a hypothetical choice on top of the stored ones, nothing written). Writes go through `app.LooksApply`, `LooksUndo`, `LooksRebuild` and, for the gallery's selection, `app.LooksApplyMany` and `LooksUndoMany`: the looks are applied whole or undone with one index, one rebuild and, for an apply, one undoable plan edit; a look the Mod has no texture of, or without a choice to undo, is reported and left alone, and the call fails only when that holds for every look. The window keeps the plan's index in memory under `app.LooksKey` and hands it to these calls (`App.LooksCache`), so an apply does not read the Mod folders again. Texture pictures: 16 §4.4 (`app.LookTexture`, `<data directory>/cache/looktex/`).

The first apply in a home opens a confirmation that says what will be written (§4; the dry run gives the folder, the files as links or copies, and whether the plan gains the entry), that the feature is in test, and how to report a problem (a button to the feedback page); "do not show this again" is the setting `ui.gui.looks.confirmed`. The dialog is shown before every apply, from the looks editor and from the gallery's selection, until that box is ticked; cancel writes nothing (`looksconfirm.js`, tested in `testdata/looks.mjs`). 

Styles and random variants (§5). CLI: `archo looks style list | choices | add | remove` (13 §4.2). The view models carry what an "add" row needs: `LookDetail.Add` (`view.LookAdd`, `internal/view/styles.go`) has, for the selected Mod, the style categories (`styles`, each with `ok`, the reason `why` and its `code`, `shared` when the category already has a style for the thing, and the `note` to show), the random variant targets (`variants`, `default` first), `stylesNone` / `variantsNone` when nothing of a kind can be added and why (no Mod selected, no Def uses the look, an unsupported graphic class), the label of the random option, and the look's `entries` with their state; `LooksOverview.Styles` is the second Preference row (`RimArcho Styles Preference`: its place in the plan, entries per state, files, bytes, links and copies, the folder). The window's calls are `LookStyleAdd`, `LookStyleRemove` and `LookStyleAddPreview`, the dry run for a first-add confirmation whose "do not show this again" is the separate setting `ui.gui.styles.confirmed` (`view.StylesConfirmedKey`, in `LooksOverview.Styles.Confirmed`). The looks editor's operation card has the "add" row and the override chain lists a look's entries (16 §2.5 Looks view).

## 7. Acceptance

- `internal/looks` tests on synthetic Mods (`internal/testkit`): grouping of facings, masks, worn body types and folder looks; load-folder priority inside a Mod; identical copies; overridden, overriding and mixed; Def and style references through `ParentName`.
- `internal/app` tests (`looks_test.go`): choice resolution order (plan over all, slot over `all`; `TestLooksResolutionOrder`); the four states (`TestLooksStates`); a rebuild writes hard links on one volume and records them, an unchanged rebuild writes nothing, an updated Mod file is relinked, and files are copies without hard links (`TestLooksRebuildLinks`); undo touches one look and one scope (`TestLooksUndoOneLookOneScope`); the entry is added last in one undo step, a sort keeps it last, and the link is `_Archo_Looks` (`TestLooksEntryLastAndLinkName`); a refused rebuild leaves the game write without the entry (`TestLooksFailedRebuildLeavesEntryOut`); nothing is written into a Mod folder, uninstall included (`TestLooksWritesNothingIntoMods`); the looks directory on the game's volume and its fallback (`TestLooksDirOnGameVolume`); texture optimisation and `folder remove` leave it alone (`TestLooksModIsLeftAlone`). `internal/store`: `TestLookChoices`, `TestLookFiles`, `TestUpgradeLooksAndPlanDelete`. `internal/cmds`: `TestLooksCommands`. In a pack: 10 §6 K27. The generated Mods refuse a Mod's folder and identity actions, keep their folders, have their own library source, and a list that names them imports elsewhere as a list with Mods not installed (`TestGeneratedModsRefuseModActions`, `TestGeneratedModsMissingOnImport`; `internal/view` `TestGeneratedModOffersNoFolderOps`).
- `internal/looks` `TestDecodeDDS` (hand-built BC1, BC2, BC3 and uncompressed blocks; refusals) and `TestShown`. `internal/view` (`looks_test.go`): overview counts, gallery filters and search, detail axes of a plain, a worn and a folder look, predictions, and several looks applied as one undo step. `internal/surface/gui` (`looks_test.go`): the index is built once per plan state, kept across an apply and built again after a plan change (`TestLooksIndexIsBuiltOncePerPlanState`); the texture route serves only what the index names, makes and caches a thumbnail, decodes BC1 and BC3 and answers 404 for BC7 (`TestLookTexRoute`).
- §5, `internal/app` (`styles_test.go`): the generated Defs and patches as golden text and well-formed, with the textures of a style, a worn style and two random variant folders, and a second variant in the same folder (`TestStylesXMLAndFiles`); the refusals and the add row's choices (`TestStylesRefusals`); a variant with facings on game 1.7 (`TestStylesFacingsVariant`); active, skipped, asleep and broken (`TestStylesStates`); the dry run writes nothing, hard links are recorded, an unchanged rebuild writes nothing, copies without hard links, a removed entry's files and Defs go (`TestStylesRebuild`); both generated Mods stay the plan's last two entries in their order after a sort, and the link is `_Archo_Styles` (`TestStylesEntryPlaceAndLink`); a failed styles rebuild leaves only it out of the write (`TestStylesFailedRebuildLeavesOnlyItOut`); nothing is written into a Mod folder, uninstall and texture optimisation included (`TestStylesWriteNothingIntoMods`). `internal/store`: `TestLookStyles`. `internal/cmds`: `TestLooksStyleCommands`. `internal/view`: `TestLookAddRowAndStylesCard`. `internal/surface/gui`: `TestLookStyleCalls`.
- Real game, in the VM (UserGuide/release-vm-tests.md): apply a look from an earlier Mod, launch, see it in the game; undo, launch, see the load-order result. §5 (not run yet): add a style to a category and see it offered in an ideoligion's style and on a new building; add it with the random option and spawn items; add a random variant and see both textures on items; remove each, load the save; the checks of §5.4.
