# Common Mod authoring error patterns (v1)

> Status: v1, 2026-10-22. For Mod authors: the most common **loading-phase** errors in real player logs, what authors got wrong, and how to write them correctly.
> Basis: 6,734 public RimWorld 1.6 HugsLib logs (design: `00-log-corpus-design.md`), G5 error library `spikes/patterns/patterns-v0.json` (branch `claude/corpus-g345-f84a49`), full results-v4, G6 labels, and individual checks of 120 real clusters (opening Mod XML, decompiling DLLs, comparing 1.6.4871 `Assembly-CSharp.dll` and Core/DLC Defs).
> Scope: v1 covers only loading patterns with strong data and verification. Runtime NREs, Harmony-stack errors, and save errors need separate code research; listed in §5.

## 0. How to read this

**What the numbers mean.** Three counts per pattern:

- **Logs**: logs containing the pattern (6,734 total). Share = logs / 6,734.
- **Sessions**: consecutive uploads within 48 hours by one player with nearly identical lists count as one session (about 5,000 total). Closer to "how many players encountered it" than log counts.
- **Clusters**: same-template errors in one log grouped together. Far more clusters than logs means many errors per log.

Figures come from patterns-v0 (v3 attribution); template log counts from results-v4. Definitions agree, with differences within a few percentage points.

**Sampling bias.** Players **voluntarily shared** these logs, mostly after problems; rates far exceed the general population. **Relative** pattern frequencies are reliable; absolute claims such as "55% of players encounter this" are not. Mostly 1.6 (Unity) logs; 1.7 unrepresented.

**Ordered by frequency, not severity.** Each has its own consequences. The two most common (cross-references, sounds) usually lose a minor feature; rarer assembly/static-constructor errors may disable an entire Mod.

**Players rarely fix these themselves.** Between consecutive uploads, loading errors rarely disappear after changing only 1–3 Mods: `xref.def` 3%, `xref.sound` 1%, patch xpath 10%, `xml.no-field` 9%. Players often change unrelated Mods. These generally need author fixes.

**Verification levels.** Checked 3–11 real clusters per pattern:

- **Confirmed**: cause visible in Mod files/decompiled code and consistent with log;
- **Likely**: mechanism confirmed, but logged version absent locally (library mostly newer), or trigger unseen;
- **Not reproduced**: cause absent from current files, or attribution wrong (player environment/another Mod).

"Still present" refers to local library versions downloaded between 2025-06 and 2026-09. packageIds appear only for confirmed cases where the author is clearly responsible.

## 1. Overview

| # | Pattern | Typical message | Logs | Share | Sessions | Clusters | Consequence | Checks (confirmed/likely/not reproduced) |
|---|---|---|---|---|---|---|---|---|
| 1 | `xref.def` | Could not resolve cross-reference … | 3,706 | 55% | 2,730 | 94,223 | Reference discarded, occasionally cascades | 4 / 0 / 4 |
| 2 | `xref.sound` | No Verse.SoundDef named X … (using undefined sound) | 3,532 | 52% | 2,632 | 21,909 | No sound (warning) | 6 / 1 / 2 |
| 3 | `patch.op-failed` + `patch.xpath` | Patch operation … failed / Failed to find a node with the given xpath | 2,501 / 1,790 | 37% / 27% | 1,899 / 1,322 | 27,947 / 20,705 | Patch ineffective; Sequence stops subsequent operations | 7 / 5 / 1 |
| 4 | `xml.type-missing` | Could not find type named X | 2,248 | 33% | 1,733 | 45,124 | Whole Def fails, may cascade | 3 / 1 / 0 |
| 5 | `xml.no-field` | <x> doesn't correspond to any field in type T | 2,044 | 30% | 1,540 | 19,603 | Field ignored | 3 / 2 / 0 |
| 6 | `xml.config-error` | Config error in X: … | 2,004 | 30% | 1,421 | 46,760 | Mostly harmless, some gameplay impact | 5 / 1 / 1 |
| 7 | `asm.version-mismatch` | MissingMethodException / TypeLoadException … | 1,950 | 29% | 1,505 | 17,513 | Feature or whole assembly fails | 4 / 6 / 0 |
| 8 | `asm.harmony-target` | Undefined target method for patch method … | 1,867 | 28% | 1,425 | 3,690 | Patch fails; PatchAll may disable whole Mod | 5 / 4 / 0 |
| 9 | `asset.texture-missing` | Could not load Texture2D at 'X' … | 1,821 | 27% | 1,411 | 68,115 | Pink/blank textures, no sound | 3 / 1 / 5 |
| 10 | `xref.getnamed` | Failed to find T named X. There are N defs … | 1,561 | 23% | 1,197 | 12,098 | Code gets null, often followed by NRE | 2 / 2 / 7 |
| 11 | `patch.findmod` | Patch operation PatchOperationFindMod(X) failed | 1,548 | 23% | 1,165 | 3,429 | Compatibility patch ineffective | 3 / 2 / 3 |
| 12 | `asm.static-ctor` | Error in static constructor of X / Error while instantiating a mod | 1,098 | 16% | 858 | 3,398 | Often whole Mod fails | 4 / 2 / 1 |
| 13 | `asset.maintex` | Material 'X' with Shader 'Y' doesn't have … '_MainTex' | 537 | 8% | 414 | 562 | Missing icon (warning) | 1 / 1 / 1 |
| 14 | `xml.asset-load` | Exception loading UnityEngine.Texture2D from file. | 450 | 7% | 338 | 461 | Pink textures / no sound | 5 / 1 / 5 |
| 15 | `xml.def-exception` | Exception loading def from file X.xml: … | 413 | 6% | 314 | 1,806 | Whole Def (sometimes file) lost | 2 / 1 / 2 |

Also `asm.dependency-missing` (774 logs, "Cannot resolve dependency to assembly 'UnityEngine.InputLegacyModule' … ReflectionOnly"): confirmed noise from Prepatcher's mechanism; 776/776 logs with this message include Prepatcher. **Not other Mod authors' fault**, so omitted here.

120 cases checked: 58 confirmed, 30 likely, 32 not reproduced (plus 1 unavailable target Mod).

## 2. Five root causes

The author-side causes behind 15 patterns fall into five groups. Many patterns express the same root cause differently.

### 2.1 Loading 1.5 content on 1.6

Largest group, spanning `xml.no-field`, `xml.type-missing`, `asm.version-mismatch`, `asm.harmony-target`, `asm.static-ctor`, `xml.def-exception`, `xref.sound`.

Game behavior (confirmed by decompiling 1.6.4871 `ModContentPack.InitLoadFolders`):

- Without **a `<v1.6>` entry** in `LoadFolders.xml`, the game selects **the highest version not exceeding 1.6**. Mods ending at `<v1.5>` load 1.5 Defs/DLLs on 1.6.
- Similar version-folder fallback without `LoadFolders.xml`.
- `<li>/</li>` loads the root too. Root `Assemblies/` DLLs are overridden only by **same-name** files in version folders; different names load both (§3.7).

Recurring corpus changes from 1.5 → 1.6:

| 1.5 | 1.6 | Appears in |
|---|---|---|
| `<race><wildness>` | `<statBases><Wildness>` | xml.no-field (about 400 logs) |
| `<placingDraggableDimensions>` | `<drawStyleCategory>` | xml.no-field(261) |
| `TerrainDef.holdSnow` / `hideAtSnowDepth` | `holdSnowOrSand` / `hideAtSnowOrSandDepth` | xml.no-field |
| Building `<building><soundAmbient>` | `CompProperties_AmbientSound` | xml.no-field |
| SoundDef `Pawn_Melee_Punch_HitBuilding` | `…_Generic` / `_Metal` / `_Stone` / `_Wood` | xref.sound (about 2,000) |
| `LongEventHandler.QueueLongEvent` (6 parameters) | Added `bool forceHideUI` | asm.version-mismatch (about 250) |
| `void GenText.AppendInNewLine` | Returns `StringBuilder` | asm.version-mismatch |
| `int Map.Tile`,`GenerateThings(int tile)` | `PlanetTile` | asm.version-mismatch, xml.def-exception(113) |
| `Pawn_EquipmentTracker.MakeRoomFor(eq)` | Added overload `MakeRoomFor(eq, out dropped)` | asm.static-ctor (41) |
| `PawnRenderTree.ProcessApparel` | Moved to `DynamicPawnRenderNodeSetup_Apparel` | asm.harmony-target |
| `Log.Error(string, bool)` | Only `Log.Error(string)` remains | asm.version-mismatch (bundled old libraries) |

Note: **a return-type-only change also breaks binary compatibility** (`AppendInNewLine`). "Source needs no edits" does not mean "old DLL works"; recompile for every game version.

### 2.2 Unguarded optional DLC / Mod content

Most common author mistake in `xref.def`, `xref.sound`, `patch.xpath`: referencing/patching DLC/Mod content without skipping when absent.

- Cross-references: missing `xref.def` names include 9% DLC Defs, 52% other Mods; `xref.sound` 11% DLC sounds, 14% other Mods.
- Patches: Rabbie, Grimstone: Beasts, MoeLotl patch Royalty/Odyssey Defs without guards, reporting errors every launch without DLC.
- Common partial fixes: adjacent lines have `MayRequire`, one omitted; or guard the **wrong** DLC (Ideology sound under Royalty condition).

### 2.3 Dependency framework or target Mod changed, author did not follow

- **VEF 1.6 renamed namespaces**: `VFECore.*` → `VEF.*`, `VanillaGenesExpanded.*` → `VEF.Genes.*`, `VanillaFurnitureExpanded.*` → `VEF.Buildings.*`. Old XML names caused widespread `xml.type-missing` early in 1.6, plus `xml.no-field` (missing class falls back to base, each child reported as no field).
- Targets move content into conditional folders (Vanilla Animals Expanded animals loaded only without Odyssey) or remove nodes (Vanilla Genetics Expanded hybrids lost `<verbs>`), breaking patches.
- `MayRequire` guards absence, not "Mod installed but class renamed" (§3.4).

### 2.4 Fragile code

Works until another update or earlier Mod change:

- Positional xpath (`li[5]`), mutable-value selection (`li[clipFolderPath="Weapon/Revolver"]`), full C# class-name selection;
- `PatchOperationSequence` + `PatchOperationTest` for "skip if absent", without `<success>Always</success>`, so guard itself errors;
- Bare `harmony.PatchAll()` in static/`Mod` constructors: one missing target prevents **all remaining patches in the Mod**;
- `AccessTools.Method(type, "Name")` without parameter types becomes ambiguous after overload additions; passing null to `Harmony.Patch` unchecked;
- Hardcoded transpiler IL shapes (local indices, closure names) break on minor hotfixes;
- `DefDatabase<T>.GetNamed` / `ThingDef.Named` for optional Defs without null checks.

### 2.5 Asset problems

Texture path/filename mismatch, missing bundled art, dimensions not divisible by 4, unsupported WAV encoding, custom shaders without `_MainTex`. Individually uncommon, all detectable before release.

## 3. Individual patterns

### 3.1 `xref.def`: reference to a nonexistent Def

- **Frequency**: 3,706 logs (55%), 2,730 sessions, 94,223 clusters. Common templates:
  - `Could not resolve cross-reference: No RimWorld.StatDef named X found to give to RimWorld.StatModifier (null stat)` (810 logs)
  - `Could not resolve cross-reference to Verse.ThingDef named X (wanter=thingDefs)`(663)
  - `Could not resolve cross-reference to Verse.ResearchProjectDef named X (wanter=researchPrerequisites)`(484)
  - `… ThingDef named X (wanter=recipeUsers)`(399),`… (wanter=prerequisites)`(339)
- **Consequence**: reference discarded (stat absent, research prerequisite lost, recipe not added to bench). Usually minor lost functionality; occasionally runtime nulls.
- **Missing-name origins** (64,212 deduplicated (log, name) pairs): disabled other-Mod Defs 52%; absent everywhere (typo, removed, unavailable library Mod) 31%; uninstalled DLC 9%; cascades from vanilla Def load failures 5%; DLC shown enabled but Def absent 3% (suspected player installation).
- **Author mistakes**:
  1. Unguarded DLC Def: Royalty `JumpRange` in armor `statBases`; adjacent lines use `MayRequire`, but this one was missed (`com.zoshsgahdnkc.wrmegacorpedition`, still present).
  2. Whole Mod is DLC content, but DLC only in `loadAfter`, not `modDependencies` (`kearril.fishingspotsandanglerkits`, Odyssey fishing, still present).
  3. Copied DLC component references DLC-only effects (`kindseal.ufpc`, Anomaly incinerator effects, still present).
  4. **Cascade**: patch adds misspelled/unloaded C# ModExtension to **vanilla** Def, causing its load failure and dozens of xrefs blamed on others. Old ReGrowth patch used `RegrowthCore`, DLL `ReGrowthCore`, losing all vanilla BiomeDefs (25/25 related logs have matching load exceptions). Fixed in current 1.6.
  5. Redefining vanilla abstract parent (`<ThingDef Name="BuildingBase" Abstract="True">`) with old values propagates invalid references to vanilla/Mod buildings. Errors name innocent children (VFE Spacer/Tavern cases; true source absent from library).
- **Correct approach**:

  Individual fields/list items can carry `MayRequire` (game permits missing reference silently):

  ```xml
  <statBases>
    <ArmorRating_Sharp>0.8</ArmorRating_Sharp>
    <JumpRange MayRequire="Ludeon.RimWorld.Royalty">20</JumpRange>
  </statBases>
  <researchPrerequisites>
    <li MayRequire="Ludeon.RimWorld.Odyssey">Fishing</li>
  </researchPrerequisites>
  <comps>
    <li Class="CompProperties_AbilityBurner" MayRequire="Ludeon.RimWorld.Anomaly">
      <!-- References Anomaly mote / effecter -->
    </li>
  </comps>
  ```

  If the whole Def needs DLC, put `MayRequire` on the Def or use a conditional folder (§3.2). If the whole Mod needs it, declare `modDependencies`:

  ```xml
  <modDependencies>
    <li>
      <packageId>Ludeon.RimWorld.Odyssey</packageId>
      <displayName>Odyssey</displayName>
    </li>
  </modDependencies>
  ```

  Do not redefine vanilla abstract parents; patch values instead.
- **Confidence**: 4 confirmed (including cascade), 4 not reproduced (1 fixed currently, 3 missing dependencies/other Mods changing parents).

### 3.2 `xref.sound`: reference to a nonexistent sound

- **Frequency**: 3,532 logs (52%), 2,632 sessions, 21,909 clusters. Template `Could not resolve cross-reference: No Verse.SoundDef named X found to give to <Type> … (using undefined sound instead)`, recipients: `RaceProperties` (2,085 logs), `ThingDef` (831), `VerbProperties` (699), `Tool`, `SkyfallerProperties` (626 each), `IngestibleProperties` (568).
- **Consequence**: warning, not error; affected action is silent.
- **Missing-name origins** (16,463 pairs): absent everywhere 73%, including at least 13% renamed vanilla 1.6 `Pawn_Melee_Punch_HitBuilding`, about 15% audio paths in SoundDef fields, remainder typos/dangling third-party names; other-Mod sounds 14%, DLC 11%.
- **Engine detail**: field-level `MayRequire` **does not work for sound**. Other missing Def types skip silently; missing SoundDef inserts undefined sound and **unconditionally** warns (decompiled `DirectXmlCrossRefLoader.WantedRefForObject.TryResolve`). Guard DLC sounds at Def level.
- **Author mistakes** (6 confirmed, 7 Mods, all still present):
  1. Copy 1.5 race XML to 1.6 unchanged, retaining `Pawn_Melee_Punch_HitBuilding` (`xmb.breadmech`, `w.zhp`).
  2. DLC sounds in unconditional Defs: Anomaly security door (`ods.simpledoors`), Odyssey shield (`kyzy.shipwallenhanced`).
  3. Wrong DLC guard: copied Ideology role ability with `CombatCommand_Warmup`, child guarded by `MayRequire="Ludeon.RimWorld.Royalty"` (`kalospacer.dragonianmix`).
  4. Third-party sound never defined in any Mod/version (`aoba.fortress.industrial.citadel`, `VFE_Shot_InfernoSpewer`, 715 logs).
  5. Typo `Reload_Standard`, vanilla `Standard_Reload` (`aoba.rimthunder.mobileinfantry`, 167 logs; 17 local files copied typo).
  6. `clipPath` in a SoundDef-name field (`sustainStartSound`, etc.), likely (changed currently).
- **Correct approach**:

  ```xml
  <!-- Vanilla 1.6 name -->
  <soundMeleeHitBuilding>Pawn_Melee_Punch_HitBuilding_Generic</soundMeleeHitBuilding>

  <!-- DLC sound: MayRequire on the whole Def -->
  <ThingDef ParentName="DoorBase" MayRequire="Ludeon.RimWorld.Anomaly">
    <defName>MyMod_SecurityDoor</defName>
    <building>
      <soundDoorOpenManual>SecurityDoor_Open</soundDoorOpenManual>
    </building>
  </ThingDef>
  ```

  If only the sound depends on DLC, replace with a guarded patch (`MayRequire` on Sequence `<li>`, §3.3):

  ```xml
  <Operation Class="PatchOperationSequence">
    <operations>
      <li Class="PatchOperationReplace" MayRequire="Ludeon.RimWorld.Anomaly">
        <xpath>Defs/ThingDef[defName="MyMod_Door"]/building/soundDoorOpenManual</xpath>
        <value><soundDoorOpenManual>SecurityDoor_Open</soundDoorOpenManual></value>
      </li>
    </operations>
  </Operation>
  ```

  Or a conditional folder:

  ```xml
  <loadFolders>
    <v1.6>
      <li>/</li>
      <li>1.6</li>
      <li IfModActive="Ludeon.RimWorld.Anomaly">1.6/Mods/Anomaly</li>
    </v1.6>
  </loadFolders>
  ```

  SoundDef fields take `defName`; audio paths belong only inside SoundDef `<grains><li Class="AudioGrain_Clip"><clipPath>`. When upgrading folders, compare your Def fields with new Core fields.
- **Confidence**: 6 confirmed, 1 likely, 2 not reproduced (1 other-Mod vanilla parent change; 1 missing declared dependency).

### 3.3 `patch.op-failed` / `patch.xpath`: XML patch target not found

- **Frequency**: a failure usually emits two lines: summary `[Mod] Patch operation Verse.PatchOperationX(xpath) failed` plus `file:` (`patch.op-failed`, 2,501 logs, 37%, 27,947 clusters), detail `[Mod] Verse.PatchOperationX(xpath="…"): Failed to find a node with the given xpath` (`patch.xpath`, 1,790 logs, 27%, 20,705 clusters). Common sources: Combat Extended compatibility (327 logs), Dead Man's Switch – Synthetic (151), Better Architect Menu (97), Soundscape Enhanced (79), Layers: Capes and Armor (73).
- **Consequence**: operation ineffective; in `PatchOperationSequence`, all later operations stop.
- **Author mistakes**:
  1. **Unguarded DLC patches** (3 confirmed, still present): add enemies to Royalty `Empire` (`runnelatki.rabbieracemod`); wildlife to five Odyssey biomes, even commented "Odyssey" (`abrolo.grimstone.beasts`); stats to Royalty `Throne` inside `success=Normal` Sequence (`fxz.moelotlzombie.update`).
  2. **Not following target updates** (2 confirmed, fixed): CE patches VAE animals moved in 1.6 to without-Odyssey folders; CE replaces hybrid `<verbs>` removed in VGE 1.6.
  3. **Move content out of load folders but leave patches** (confirmed, fixed): Def in `1.6/to be updated/`, patch still active in `1.6/Patches/`.
  4. **Guard itself errors** (confirmed, still present): "change only if present" via `PatchOperationSequence` + `PatchOperationTest` without `<success>Always</success>`. Popular tweaks remove vanilla node, producing errors every launch.
  5. **Positional selection** (likely): `root/children/li[5]` points wrongly after child-list changes.
  6. **Mutable-value selection** (likely): `li[clipFolderPath="Weapon/Revolver"]` stops matching after another sound Mod changes it.
  7. Existing G6 labels: PatchOperationAdd fails on `li[label="neurotic"]` (PF05); target removed `HelixienGasBoiler` fields (PF15).
- **Correct approach**:

  Three DLC guards, in robustness order: `IfModActive` folder (§3.2), `PatchOperationFindMod` (display name, §3.11), or `MayRequire` on Sequence `<li>`.

  **Note**: 1.6.4871 checks `MayRequire` only on **list items** (skipping `<li>` in Sequence `<operations>`), **not top-level `<Operation MayRequire="…">`** (decompiled `ModContentPack.LoadPatches`, `DirectXmlToObject.ObjectFromXml`). Some corpus "fixes" add it at top level and cannot work according to game code.

  ```xml
  <Operation Class="PatchOperationSequence">
    <operations>
      <li Class="PatchOperationAdd" MayRequire="Ludeon.RimWorld.Royalty">
        <xpath>Defs/FactionDef[defName="Empire"]/permanentEnemyToEveryoneExcept</xpath>
        <value><li>MyMod_Faction</li></value>
      </li>
    </operations>
  </Operation>
  ```

  For possibly absent nodes (removed by others or only in parent Def), use `PatchOperationConditional`, not Sequence + Test.

  ```xml
  <Operation Class="PatchOperationConditional">
    <xpath>Defs/MeditationFocusDef[defName="Natural"]/requiredBackstoriesAny</xpath>
    <match Class="PatchOperationAdd">
      <xpath>Defs/MeditationFocusDef[defName="Natural"]/requiredBackstoriesAny</xpath>
      <value><li><categoryName>MyMod_Tribal</categoryName></li></value>
    </match>
    <!-- No nomatch: if another Mod removed the restriction, do nothing -->
  </Operation>
  ```

  Select by stable key, not position:

  ```xml
  <!-- Avoid: root/children/li[5] -->
  <xpath>Defs/PawnRenderTreeDef[defName="MyRace"]/root/children/li[debugLabel="Head"]/drawData</xpath>
  ```

  Also: no optional targets in `success=Normal` Sequences; move patches with unloaded Defs; rerun patches after major target updates (developer-mode startup logs suffice).
- **Confidence**: 7 confirmed, 5 likely (4 old versions unavailable), 1 not reproduced; also reused 2 reviewed G6 cases.

### 3.4 `xml.type-missing`: XML class not found

- **Frequency**: 2,248 logs (33%), 1,733 sessions, 45,124 clusters. Common: `Could not find class Configurable_Deconstruct_Percentage.DeconstructSettings while resolving node ModSettings` (107 logs), NewRatkin `Could not find a type named NewRatkin.*` (about 80 each), `Exception loading def from file X.xml: … Could not find type named VFECore.* / VFESecurity.*`. RimArcho source attribution nearly absent (0%); identified sources through Mod overrepresentation in error logs, then checked files.
- **Consequence**: `Class=` in components/extensions causes **whole Def failure**, cascading for patched vanilla Defs (§3.1). In `<Operation Class=…>`, falls back to base class: ineffective patch and `xml.no-field` for each child.
- **Author mistakes**:
  1. **Framework renames** (2 confirmed): VRE Android GeneDef still `VanillaGenesExpanded.GeneExtension`, renamed `VEF.Genes.GeneExtension` in VEF 1.6 (55 logs, all in the 1.6 release week, fixed); VPE Biotech Integration `VFECore.PatchOperationToggableSequence` (fixed).
  2. **Removed optional class, not covered by `MayRequire`** (confirmed, still present): Grimworld – Hammer of the Imperium (`grimworld.astramilitarum`) trench `<li Class="VFESecurity.CompProperties_TerrainSetter" MayRequire="VanillaExpanded.VFESecurity">`, class absent from VFE Security 1.6. Guard works only when disabled; enabled but missing class loses whole trench Def (62/64 logs with both Mods error, 0/118 without VFE Security).
  3. **Own 1.5 content loaded on 1.6** (confirmed, early VFE Security 1.6, 74 logs): §2.1.
  4. **XML/DLL out of sync** (likely): `workerClass` points to class no longer in DLL.
  5. Old player config retains renamed settings class (Configurable Deconstruct): game automatically uses new class, **harmless**, ignore.
- **Correct approach**:

  - After framework upgrades, search globally for `Class="OldNamespace.` and confirm no `Could not find type` in developer mode.
  - Place optional components in target-specific conditional folders, not only `MayRequire`, allowing at least version-specific folders.
  - Release XML and DLL from the same build.
- **Confidence**: 3 confirmed, 1 likely.

### 3.5 `xml.no-field`: XML field absent from type

- **Frequency**: 2,044 logs (30%), 1,540 sessions, 19,603 clusters. Common: `<placingDraggableDimensions>…` on `ThingDef` (261 logs), `<wildness>…` in `<race>` (250 and 198), `<soundAmbient>MechTurretBig_Call</soundAmbient>` in `<building>` (about 150).
- **Consequence**: field ignored. Harmless alone, but almost always means **the whole XML targets an older version**; other fields probably wrong too.
- **Author mistakes**:
  1. **Removed/renamed 1.6 fields, still loading 1.5 XML** (2 confirmed, fixed): NewRatkin animals (`solaris.ratkinracemod`) `<wildness>` in a 1.5 file (about 300 logs), which 1.6 loaded at the time; Gloomy Furniture walls (`solaris.furniturebase`) `<placingDraggableDimensions>` (52 logs). Same fix visible in Epochs Pottery, Uncle Boris, SimpleStairs fork, VFE Security (74 logs): old fields in 1.5, `drawStyleCategory` in 1.6. Full table §2.1.
  2. **False missing fields from missing class** (confirmed): §3.4. Fix class name, not fields.
  3. **Framework version mismatch**: field from newer/older framework than player's version, e.g. `PipeSystem.ProcessDef.maxOutputCount` (G6 XL14).
- **Not a Mod fault**: mech turret `soundAmbient` and Ideology `LayoutRoomDef.selectionWeight` groups (about 150 logs) match vanilla 1.5 byte-for-byte, no overrepresented Mod, many missing vanilla textures. Mixed old game files; advise "verify integrity of game files", not author blame.
- **Correct approach**:

  ```xml
  <!-- 1.5 -->
  <race>
    <wildness>0.55</wildness>
  </race>
  <placingDraggableDimensions>1</placingDraggableDimensions>

  <!-- 1.6 -->
  <statBases>
    <Wildness>0.55</Wildness>
  </statBases>
  <drawStyleCategory>Walls</drawStyleCategory>
  ```

  Above all, **provide a real 1.6 folder for 1.6** and a `<v1.6>` entry in `LoadFolders.xml` (§2.1); otherwise 1.6 players fall back to 1.5 content.
- **Confidence**: 3 confirmed, 2 likely (1 confirmed not Mod fault).

### 3.6 `xml.config-error`: game configuration self-check

- **Frequency**: 2,004 logs (30%), 1,421 sessions, 46,760 clusters. Common: `Config error in FT_ENIAC: description has trailing whitespace` (93 logs), `Multiple gene categories share the same displayPriorityInXenotype` (84), `Vent_Over: is airtight but Fillage is not Full` (60), `apparel has no layers` (60).
- **Consequences**: three tiers.

  | Tier | Message | Actual impact |
  |---|---|---|
  | Harmless | Description whitespace; duplicate gene `displayPriorityInXenotype`; duplicate research coordinates; Beauty with `category None` | Log noise |
  | Minor | `has costStuffCount but no stuffCategories`; `tradeability doesn't allow traders to sell` | Value silently ignored |
  | Real impact | `weaponMoney` below cheapest weapon; `apparel has no layers`; null `thingClass` / `workerClass`; comps on non-`ThingWithComps`; `Two ThinkNodes have the same unique save key` | Unarmed pawns, unwearable apparel, discarded comps, save/load problems |
- **Author mistakes** (5 confirmed):
  1. Common integer gene display priority collides with another Mod (VRE Android `VREA_Hardware`, harmless, still present).
  2. **New inherited parent fields**: vanilla 1.6 adds `Vent.isAirtight`, child sets `fillPercent=0.6`, contradictory (`uuugggg.replacestuff` original still present; Continued 1.6 folder fixed but old root file remains).
  3. Copied `costStuffCount` without `stuffCategories` (SimpleStairs fork, `rw.mod.simplestairs`, still present).
  4. PawnKind `weaponMoney` minimum below only available weapon price, possibly unarmed tanks (`dms.proton.jointoperations`, still present).
  5. Parent patch leaks into new DLC child (CE `Gun_ChargeLance.destroyOnDrop` inherited by Odyssey `Gun_ChargeLance_Unique`, fixed currently).
- **Correct approach**:

  - Developer-mode startup: clear own `Config error` logs, or at least confirm harmless tier.
  - Check new parent fields each game version.
  - Before patching parents, search new descendants (1.6 Odyssey adds `_Unique` children to vanilla weapons).
  - `weaponMoney` minimum above cheapest `weaponTags` weapon market value.
  - Never replace shared-parent `<apparel>` / `<layers>` wholesale, stripping children (likely cause of 60-log "apparel has no layers" group naming innocent children).
- **Confidence**: 5 confirmed, 1 likely, 1 not reproduced (most frequent whitespace absent currently).

### 3.7 `asm.version-mismatch`: DLL calls removed code

- **Frequency**: 1,950 logs (29%), 1,505 sessions, 17,513 clusters. Common: `ReflectionTypeLoadException` (407), `TypeLoadException: Could not resolve type … from typeref (expected class … in assembly …)` (342 + 305), `MissingMethodException: Method not found: … QueueLongEvent(…)` (about 250).
- **Consequence**: caller feature fails; static constructor may disable whole Mod. `TypeLoadException` during `GetTypes()` makes game **discard whole assembly**.
- **Author mistakes**:
  1. **Only 1.5 DLL/LoadFolders through v1.5** (2 confirmed, still present): original Replace Stuff (`uuugggg.replacestuff`) 6-parameter `QueueLongEvent`; RocketMan (`krkr.rocketman`) `void AppendInNewLine`. Both fall back to 1.5 DLLs on 1.6 (§2.1). Continued Replace Stuff recompiled for 1.6.
  2. **Bundled old shared library** (confirmed): Mods ship `0MultiplayerAPI.dll`; name binding means **first loaded serves all Mods**. 0.1/0.2 call `Log.Error(string, bool)`, removed in 1.6. All 33 error logs loaded 0.1/0.2, never Rimefeller's 0.5; 19 from old Achtung. Victim Mods did nothing wrong. Current Achtung ships 0.6.
  3. **Old root `Assemblies/` DLL leaks in** (likely): `<v1.6>` includes `<li>/</li>`, renamed main DLL lets 1.0-era root DLL load alongside new one (Harmony 1 type references, 75 logs).
  4. **"1.6" built against 1.5 references** (likely, fixed).
  5. **Wrong target framework** (likely): `System.Collections, Version=10.0.0.0` from .NET 10 references absent in game Mono.
  6. **Hard reference to optional Mod**: good example Fortified Features Framework isolates CE types in DLL loaded only through `IfModActive="CETeam.CombatExtended"` (G6 AS03/AS04 failures violate declared `loadAfter`, player fault).
- **Correct approach**:

  ```xml
  <!-- LoadFolders.xml: one entry per supported version, versioned DLL folders -->
  <loadFolders>
    <v1.5>
      <li>1.5</li>
    </v1.5>
    <v1.6>
      <li>1.6</li>
      <li IfModActive="CETeam.CombatExtended">1.6/CE</li> <!-- Separate DLL for CE type references -->
    </v1.6>
  </loadFolders>
  ```

  ```xml
  <!-- csproj: target net472, reference game DLLs or matching reference package -->
  <TargetFramework>net472</TargetFramework>
  <PackageReference Include="Krafs.Rimworld.Ref" Version="1.6.*" />
  ```

  - If not maintaining 1.6, omit it from `supportedVersions` so players see incompatibility.
  - Shared libraries (0Harmony, 0MultiplayerAPI): depend on provider Mod (`brrainz.harmony` for Harmony), or bundle latest only.
  - No root `Assemblies/` DLLs; put old DLLs in corresponding version folders.
- **Confidence**: 4 confirmed, 6 likely (mostly fixed, old DLL unavailable), 0 not reproduced.

### 3.8 `asm.harmony-target`: Harmony target method absent

- **Frequency**: 1,867 logs (28%), 1,425 sessions, 3,690 clusters. Common: `HarmonyException: Patching exception in method null ---> ArgumentException: Undefined target method for patch method …` (142 + 68), `Error in static constructor of LWM.DeepStorage.ModInit … Patching exception` (99), Big & Small `Failed to patch VFEInsectoids 2's Creep with additional genes.` (405 logs, Message, not error).
- **Consequence**: depends on patch location. Bare `PatchAll()` in `[StaticConstructorOnStartup]` / `Mod` constructor: **one failed target stops all remaining patches**. Per-class catches (VEF, LunarFramework) lose only that patch.
- **Author mistakes**:
  1. **Other Mod removed patched method** (confirmed): Engine Industries (`aoba.motorization.engine`) targets `VehiclePawn.GetExtraFloatMenuOptionsFor`, absent in two VF 1.6 versions. Static-constructor `PatchAll` loses all Motorization patches (52 logs, fixed).
  2. **Patching another Mod's patch class** (confirmed, fixed): Map Preview compatibility targets `VEE.FertilityGrid_Patch` by string, removed in VEE 1.6. Module catch limits loss (146 logs); new version checks target existence, correctly.
  3. **Hardcoded transpiler IL** (confirmed, still present): While You're Up 1.6 patch (`zsbk.patch16.whileyoureup`) fails 24/24 logs on 1.6.4850 / 4871, 0/118 earlier, from hotfix-recompiled closure. `Mod` constructor PatchAll disables all. LWM Deep Storage 1.5 hardcoded `Ldloc_1` produces invalid IL on 1.6 (fixed).
  4. **Targets added only in hotfix** (likely): two VEF patches fail only early 1.6; `TargetMethods()` fails to filter null.
  5. **Vanilla method moved in 1.6** (likely, game confirmed): `PawnRenderTree.ProcessApparel` → `DynamicPawnRenderNodeSetup_Apparel`; `DoWindowContents` declared on `Dialog_Search<T>`, not `Dialog_WorldSearch`.
  6. **Good example**: Big & Small null-checks another Mod's private field before reflection and skips if absent; only issue is error-like message every launch.
- **Correct approach**:

  Patch per class so failures remain isolated:

  ```csharp
  [StaticConstructorOnStartup]
  public static class MyModStartup
  {
      static MyModStartup()
      {
          var harmony = new Harmony("author.mymod");
          foreach (var type in AccessTools.GetTypesFromAssembly(typeof(MyModStartup).Assembly))
          {
              try { harmony.CreateClassProcessor(type).Patch(); }
              catch (Exception e) { Log.Warning($"[MyMod] {type.Name} not applied: {e.Message}"); }
          }
      }
  }
  ```

  Use `Prepare()` to check cross-Mod/optional targets:

  ```csharp
  [HarmonyPatch]
  public static class Patch_VehicleFloatMenu
  {
      static MethodBase target;

      static bool Prepare()
      {
          target ??= AccessTools.Method("Vehicles.VehiclePawn:GetExtraFloatMenuOptionsFor",
                                        new[] { typeof(IntVec3) });
          return target != null; // false: Harmony skips this patch class without error
      }

      static MethodBase TargetMethod() => target;

      static void Postfix(ref IEnumerable<FloatMenuOption> __result) { /* … */ }
  }
  ```

  Filter null from `TargetMethods()`:

  ```csharp
  static IEnumerable<MethodBase> TargetMethods() =>
      new[] { AccessTools.DeclaredMethod(typeof(A), "X"), AccessTools.DeclaredMethod(typeof(B), "Y") }
      .Where(m => m != null);
  ```

  If a transpiler anchor is absent, return unchanged instructions and warn rather than throw:

  ```csharp
  static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
  {
      var codes = instructions.ToList();
      int i = codes.FindIndex(c => c.Calls(FindResourcesMethod));
      if (i < 0)
      {
          Log.Warning("[MyMod] transpiler anchor not found; feature disabled for this game build");
          return codes;
      }
      // … Insert code
      return codes;
  }
  ```

  Prefer Prefix/Postfix over Transpiler; find local/closure fields by name through reflection with null checks, not hardcoded indices.
- **Confidence**: 5 confirmed, 4 likely.

### 3.9 `asset.texture-missing`: texture/audio path has no file

- **Frequency**: 1,821 logs (27%), 1,411 sessions, 68,115 clusters. Templates: `Could not load Texture2D at 'X' in any active mod or in base resources.` (1,187), `… for def 'Y' …` (636), `Could not load AudioClip at 'X' …` (291), `Failed to find any textures at X while constructing Multi(initPath=…)` (about 30 per group).
- **Consequence**: pink/blank textures, no sound; functionality unaffected.
- **Many are not author faults.** 5/9 checked groups were player environments: hundreds of missing textures in one log (damaged install), whole Mod textures missing in one month, missing vanilla paths uncorrelated with Mods. If **nearly every** log with the Mod errors, suspect Mod; about 1% suggests player environment.
- **Author mistakes** (3 confirmed):
  1. **Def without art**: apparel style points to `Things/ApparelStyles/…Rustic/…`, only another style bundled (`lanyv.bunnyarmourfromhoomancute`, 86/116 logs, still present).
  2. **Other Mod assets with dependency commented out**: `Solaris.FurnitureBase` in About.xml under "temporarily removed" comment (translated) (`gloomy.furniture.fix`, all 19 errors without Gloomy Furniture, 0/319 with it, still present).
  3. **`clipPath`/filename mismatch**: `Kill` versus `Kill_.ogg` (`hirokazu.akiyama.music.pack`, 136/138 before fix, fixed 2026-10).
  4. Likely: add-on hardcodes parent texture path later moved by parent.
- **Game details**:
  - `Graphic_Multi` errors only if **all** `_north/_east/_south/_west` and unsuffixed files are absent; one direction suffices.
  - `for def 'Y'` names the Def whose graphic is being built, possibly vanilla modified by another Mod, not necessarily culprit's own Def.
- **Correct approach**:

  - Before release, developer mode: spawn each item/building/apparel and check `Could not load` logs.
  - Declare asset providers in `modDependencies`; bundle your own if avoiding hard dependency.
  - `clipPath` / `texPath` are relative to `Sounds/` / `Textures/`, no extension, matching case (Linux/Mac case-sensitive).
- **Confidence**: 3 confirmed, 1 likely, 5 groups not reproduced (player environments). G6 TX13 (Dead Man's Switch "unbundled texture") needs review: library has it; all 25 logs fall in 2026-10 whole-Mod texture-loss window.

### 3.10 `xref.getnamed`: C# Def lookup by name fails

- **Frequency**: 1,561 logs (23%), 1,197 sessions, 12,098 clusters. `Failed to find <Type> named X. There are N defs of this type loaded.` Types: ThingDef (487), TerrainDef (328), JobDef (304), ThoughtDef (267), HediffDef (214).
- **Consequence**: null returned; without check, immediate NullReferenceException; in static constructor, whole class initialization fails.
- **Engine detail**: `DefDatabase<T>.GetNamed(name)`, `ThingDef.Named(name)`, `HediffDef.Named(name)`, `[DefOf]` fields without `[MayRequire]` all emit this when absent; `GetNamedSilentFail(name)` does not. **"There are 0 defs"** means execution before Def loading finished.
- **Author mistakes**:
  1. **Unchecked `Named` on removable/renamable Defs** (confirmed): stew Mod (`badoaks.campfirestews`) uses `ThingDef.Named` to look up vanilla and other Mods' stoves in its static constructor, then null `.recipes` crashes constructor (still present).
  2. **Too early** (confirmed): Maru (`vamv.maruracemod`) RecipeWorker static constructor calls `HediffDef.Named(...)` before Defs load; all 58 logs "There are 0 defs". Matching `[DefOf]` class already in assembly (still present).
  3. Likely: XML Def has DLC condition, code lacks it (fixed).
- **RimArcho attribution is weak here**: 10 sampled "source Mods" were not real callers (DLL strings for comparisons/dictionary keys or already SilentFail). Explains most 7 not-reproduced cases; another group has enabled DLC but missing Defs (Odyssey `AncientHeavyBridge`, etc., 310 logs).
- **Correct approach**:

  ```csharp
  // Optional Def: SilentFail + null check
  var stove = DefDatabase<ThingDef>.GetNamedSilentFail("MyOtherMod_Stove");
  if (stove != null) stove.recipes.Add(MyDefOf.MyMod_Stew);

  // Fixed references use [DefOf]; optional DLC/Mod fields use [MayRequire]
  [DefOf]
  public static class MyDefOf
  {
      public static RecipeDef MyMod_Stew;
      [MayRequire("Ludeon.RimWorld.Odyssey")] public static ThingDef AncientHeavyBridge;

      static MyDefOf() => DefOfHelper.EnsureInitializedInCtor(typeof(MyDefOf));
  }
  ```

  Initialize Def-dependent work in `[StaticConstructorOnStartup]` (after Def load) or lazily; not static constructors of Worker/Comp types triggered during loading.
- **Confidence**: 2 confirmed, 2 likely, 7 not reproduced (bad attribution/player environment).

### 3.11 `patch.findmod`: optional-Mod compatibility patch fails

- **Frequency**: 1,548 logs (23%), 1,165 sessions, 3,429 clusters. `[Mod] Patch operation Verse.PatchOperationFindMod(X) failed`, or XmlExtensions `FindMod` / `OptionalPatch failed`. Common: VEF (207), Cinders of the Embergarden (74), Majestic Foliage Framework (71), Misc. Training Medieval Retexture (64).
- **Read the message correctly**: `FindMod(X) failed` **does not mean X absent**; an operation inside `match` or `nomatch` failed. Parentheses list guarded Mods regardless of branch. `PatchOperationFindMod` matches **display name** exactly, not packageId: "Humanoid Alien Races" differs from "Humanoid Alien Races ~ Dev" (decompiled `PatchOperationFindMod.ApplyWorker` / `ModLister.HasActiveModWithName`).
- **Author mistakes** (3 confirmed):
  1. **Correct guard, old class-name xpath**: `li[@Class="GraphicCustomization.CompProperties_GraphicCustomization"]`, renamed `VEF.Graphics.…` in VEF 1.6 (`oracle.miscellania`, fixed).
  2. **Wrong Mod guarded**: Medieval Overhaul guard, both branches patch Misc. Training, neither dependency nor guarded; always fails without it (`serek.misctrainingmedievalretexture`, still present).
  3. **Same node as another Mod**: VFE Security turret fuel component removed earlier by CE (`reel.turretpipeline`; CE incompatibility declared, mostly player mix).
  4. Likely: VEF unconditional vanilla `Column` requirement replacement fails if others change class (Royalty now conditional, Ideology not); LoadFolders/FindMod conditions differ.
- **Correct approach**:

  ```xml
  <!-- Guard the actual target Mod; name must exactly match About.xml <name> -->
  <Operation Class="PatchOperationFindMod">
    <mods>
      <li>Target Mod Name</li>
    </mods>
    <match Class="PatchOperationConditional">
      <xpath>Defs/ThingDef[defName="ArcheryTarget"]/graphicData</xpath>
      <match Class="PatchOperationReplace"> … </match>
    </match>
  </Operation>
  ```

  - Prefer packageId: `LoadFolders.xml` `IfModActive="packageId"`, Sequence `<li MayRequire="packageId">` avoid display-name changes. Target-specific conditional folder is safest.
  - Avoid full C# class-name xpath keys; select structurally or `contains(@Class,"CompProperties_GraphicCustomization")`.
  - Wrap shared mutable nodes in `PatchOperationConditional`.
- **Confidence**: 3 confirmed, 2 likely, 3 not reproduced, 1 uncheckable (target absent locally).

### 3.12 `asm.static-ctor`: startup code throws

- **Frequency**: 1,098 logs (16%), 858 sessions, 3,398 clusters. `Error in static constructor of X: System.TypeInitializationException …`, `Error while instantiating a mod of type X: TargetInvocationException …`, `[Harmony: failed to render frame: TypeInitializationException]` (278 logs, same failure echoed in stack rendering).
- **Consequence**: names **where**, not **why**, code failed. A throwing static constructor makes every later type access throw `TypeInitializationException`, causing repeated errors throughout play. Throwing `Mod` constructor skips settings/initialization.
- **Author mistakes**:
  1. Static-constructor `PatchAll()` stops on one failure (§3.8, most common).
  2. Static constructor calls changed 1.6 API (RocketMan, Replace Stuff, §3.7).
  3. **Name-only `AccessTools.Method`** (confirmed): PawnShields (JecsTools 1.5) targets `Pawn_EquipmentTracker.MakeRoomFor`; new 1.6 `out` overload makes name ambiguous (41 logs). Unofficial 1.6 now specifies parameter types.
  4. **Unchecked null from `AccessTools.Method` to `Harmony.Patch`** (likely): `Null method for <harmonyId>`. [LTO] Colony Groups makes about 45 such calls in one static constructor; any missing target stops all.
  5. **Bundled shared-library static constructor fails** (confirmed, 0MultiplayerAPI §3.7).
  6. **Not this Mod's fault**: access denied to `harmony.log.txt`. **Another** Mod/environment variable enabled Harmony debugging, player desktop unwritable; later patchers blamed. Lesson: **no `Harmony.DEBUG` / `[HarmonyDebug]` in releases**.
  7. Unchecked DefDatabase iteration/mutation in static constructor (specific trigger not reproduced, inherently fragile).
- **Correct approach**:

  ```csharp
  // Always specify parameter types
  var m = AccessTools.Method(typeof(Pawn_EquipmentTracker), "MakeRoomFor", new[] { typeof(ThingWithComps) });

  // Null-check before manual Patch
  if (m == null) Log.Warning("[MyMod] MakeRoomFor not found; shield auto-drop disabled");
  else harmony.Patch(m, postfix: new HarmonyMethod(typeof(MyPatches), nameof(MyPatches.MakeRoomFor_Postfix)));
  ```

  Minimize static-constructor work; catch per Def when iterating/mutating so one bad Def does not break all.
- **Confidence**: 4 confirmed, 2 likely, 1 not reproduced.

### 3.13 `asset.maintex`: custom shader lacks `_MainTex`

- **Frequency**: 537 logs (8%), 414 sessions, 562 clusters. `Material 'X' with Shader 'Y' doesn't have a texture property '_MainTex'`. Never names Mod, so RimArcho cannot attribute.
- **Consequence**: warning, blank UI icon.
- **Mechanism**: non-mote ThingDef without `uiIconPath` uses material `mainTexture` (`BuildableDef.ResolveIcon`); missing shader `_MainTex` makes Unity warn. More precise than original G5 "uiIconColor but no icon path".
- **Author mistakes** (1 confirmed): non-mote ability-effect Def uses lightning shader, no `uiIconPath` (`ariandel.ariandellibrary`, present in 410/411 error logs, still present). Likely second: VE Gravship chroma-key shader (121/121, unavailable Mod).
- **Correct approach**: provide `uiIconPath`, or hidden shader `_MainTex`:

  ```
  Properties {
      [HideInInspector] _MainTex ("", 2D) = "white" {}
      // … Custom properties
  }
  ```
- **Confidence**: 1 confirmed, 1 likely, 1 not reproduced.

### 3.14 `xml.asset-load`: texture/audio file unreadable

- **Frequency**: 450 logs (7%), 338 sessions, 461 clusters. `Exception loading UnityEngine.Texture2D from file.` (381 logs), `… AudioClip from file.` (78). Next `absFilePath:` gives file.
- **Consequence**: pink texture / no sound.
- **Largest group is not author packaging**: 18,374 messages (374 logs) have block-compressed DDS dimensions not divisible by 4. Named Mods **bundle no DDS**, only PNG, each errors in 1–2% of logs. Player optimization tools generated DDS locally; 1.6 rejects block-compressed texture dimensions not divisible by 4. Author-side PNG sizes: 4266×2400 (Shella Backgrounds), 42×42 (Charmweavers), 400×322 (Alpha Prefabs), 274×232 and 279×278 (Better Info Card).
- **Author mistakes** (5 confirmed):
  1. PNG dimensions not multiples of 4 (4 cases above, still present). Common optimization tools make aligned source sizes helpful in avoiding player errors.
  2. **Unsupported WAV**: 8 `WAVE_FORMAT_EXTENSIBLE`, stereo, 96 kHz (`plaxen.hamster`, errors in 43/53 logs, still present). Actual error masked as `ArgumentNullException: key`, difficult diagnosis.
  3. Likely: non-ASCII audio names (`Khazad-Dûm.ogg`) produce 404.
  4. Old Better Info Card folders retain unused `Snipaste_*.png` screenshots loaded as textures.
- **Game detail**: 1.6 DDS loader accepts DXT1, DXT5, BC7 (DX10), several uncompressed formats; same-name `.dds` shadows `.png` (see `dds-texture-optimization.md`).
- **Correct approach**:

  - Dimensions multiples of 4, preferably powers of 2.
  - Ogg Vorbis or ordinary PCM WAV (format tag 1), 44.1 / 48 kHz.
  - ASCII asset filenames only.
  - No unused files under `Textures/`.
- **Confidence**: 5 confirmed, 1 likely, 5 not reproduced (GPU support, zeroed files, threaded loading, player environments).

### 3.15 `xml.def-exception`: exception loading whole Def

- **Frequency**: 413 logs (6%), 314 sessions, 1,806 clusters. `Exception loading def from file X.xml: <exception>`. Largest NewRatkin `TraderKinds_Caravan_Ratkin.xml: TypeLoadException: … invalid vtable method slot` (113 logs).
- **Consequence**: Def lost, sometimes all Defs in file.
- **Author mistakes**:
  1. **Old DLL overrides changed abstract signature** (confirmed): old `NewRatkin.dll` `GenerateThings(int forTile, …)` vs 1.6 `GenerateThings(PlanetTile forTile, …)` fails type load, losing caravan TraderKindDef. Multiple reuploads have **same DLL name and assembly version**; game loads one, possibly old. Current 1.6 copies use `PlanetTile`, same-name risk remains.
  2. **Empty `<li></li>` XML** (confirmed, fixed): three empty `<workDisables>` items; empty string invalid `WorkTags`, entire backstory lost.
  3. Likely: .NET 10 references (§3.7) prevent all types with `List<T>` fields and dependent Defs loading.
  4. Other: enum typos (`Midworld`, `ture`), framework XML/version mismatch.
- **Correct approach**:

  - Recompile against current `Assembly-CSharp.dll`; increment `AssemblyVersion` each release.
  - Do not reupload others' Mods under identical assembly names.
  - Delete empty `<li></li>`; for none, remove whole node or use `None`.
- **Confidence**: 2 confirmed, 1 likely, 2 not reproduced (apparently framework or unidentified triggering patch).

## 4. Pre-release checklist

Based on these data, the following covers most author-side loading errors:

1. **Version folders**: each supported version has its own folder and `<v1.x>` LoadFolders entry; omit unsupported versions from `supportedVersions`. No root `Assemblies/` DLLs. (§2.1)
2. **Developer-mode startup, clear own errors/warnings**: `Could not resolve cross-reference`, `doesn't correspond to any field`, `Could not find type`, `Config error`, `Patch operation … failed`.
3. **Start with Core only** (no DLC/optional Mods): references/patches should silently skip, catching most §3.1–§3.3 issues.
4. **Guard DLC sounds at Def level**, not field `MayRequire`. (§3.2)
5. **Patches**: optional targets use `IfModActive` folders, `PatchOperationFindMod`, Sequence `<li MayRequire>` (top-level `<Operation>` ineffective), or `PatchOperationConditional`. Stable xpath keys, not positions/mutable values; FindMod actual target's display name. (§3.3, §3.11)
6. **Harmony**: per-class patches/catches, `Prepare()` for cross-Mod targets, typed/null-checked `AccessTools.Method`, unchanged output on transpiler mismatch, debugging off in releases. (§3.8, §3.12)
7. **C# Def lookup**: optional `GetNamedSilentFail`/null checks, fixed `[DefOf]` with `[MayRequire]` for optional fields; not during Def loading. (§3.10)
8. **Build**: `net472`, current game references; no shared libraries or latest only. (§3.7)
9. **Assets**: file for every texPath/clipPath; dimensions divisible by 4; Ogg/PCM WAV; ASCII names; `uiIconPath` for non-mote custom shaders. (§3.9, §3.13, §3.14)
10. **After major framework/target updates**: search old class names, rerun patches against new version. (§2.3)

## 5. Not yet covered

These common patterns need specific stack/code study, not messages/XML alone; omitted from v1. Frequencies from patterns-v0 (6,734 logs).

| Pattern | Phase | Logs | Sessions | Clusters | Why omitted |
|---|---|---|---|---|---|
| `rt.nre` bare NullReferenceException | Runtime | 2,502 | 1,929 | 27,838 | No information in message; group by innermost game method + Mod assembly, inspect code |
| `rt.other` other runtime exceptions | Runtime | 2,148 | 1,653 | 14,245 | Same |
| `rt.post-load` after save load / long event | Save | 1,905 | 1,489 | 7,517 | Often old save data |
| `save.loadid` missing referenced object | Save | 1,704 | 1,324 | 20,718 | Often mid-save Mod changes, not all author errors |
| `rt.ui` UI drawing exceptions | Runtime | 1,258 | 1,014 | 1,892 | Need UI patch code |
| `save.defref` missing referenced Def | Save | 1,010 | 767 | 90,952 | Often removed Mods in old save |
| `rt.tick` Tick exception | Runtime | 827 | 674 | 4,618 | Need code |
| `save.other` | Save | 676 | 510 | 8,012 | |
| `save.thinknode` changed think-tree node | Save | 562 | 454 | 883 | |
| `rt.ai-job` pawn AI/job | Runtime | 547 | 463 | 2,969 | Need code |
| `save.not-deep-saved` | Save | 511 | 411 | 2,142 | Author-side (saves reference not object), worth later article |
| `save.abstract-component` | Save | 438 | 350 | 819 | Often Mod removal |
| `rt.resolve-references` | Loading | 398 | 326 | 5,815 | Def ResolveReferences throws, need code |
| `asset.texture-create` invalid parameters | Loading | 389 | 305 | 1,957 | Often texture replacement/scaling Mods |
| `rt.mapgen` map generation | Runtime | 338 | 275 | 437 | |
| `rt.translation` injection failure | Loading | 311 | 250 | 359 | Translation/target version mismatch |
| `rt.pawn-generation` | Runtime | 293 | 231 | 390 | |
| `rt.collection-modified` mutation during iteration | Runtime | 229 | 189 | 244 | Typical author error (add/remove comps in Tick), worth later article |
| `rt.comp-init` failed component instantiation | Runtime | 226 | 168 | 235 | |
| `asm.invalid-il` invalid IL | Runtime | 105 | 69 | 113 | Multiple transpiler conflicts |
| `xml.other` / `patch.other` unclassified | Loading | 1,543 / 513 | 1,243 / 409 | 27,885 / 1,468 | G5 rules not subdivided |

results-v4 `harmony-stack` exceptions (38,760 clusters) mostly fit runtime patterns above and also need code research.

## 6. Method and reproduction

- **Patterns/frequencies**: 46 G5 rules in `spikes/patterns/main.go` (branch `claude/corpus-g345-f84a49`, `RESULT.txt`, `patterns-v0.json`). Template log counts reclassified from `corpus-kb/results-v4.jsonl` using same rules.
- **Case selection**: results-v4 clusters with exactly one source in local research library (`D:\RimWorld16\Mods`, Workshop 294100), selected by template frequency/distinct subcauses. Unattributed patterns (`xml.type-missing`, `xml.config-error`, `asset.maintex`) use overrepresented Mods in error logs, then file checks. Reuses reviewed G6 `corpus-kb/labels/v2/tasks.jsonl` cases (label IDs in text).
- **Verification**: open actual 1.6 load files resolved by LoadFolders; evaluate each xpath against merged 1.6.4871 Core/DLC Defs and targets with scripts; ilspycmd DLL decompilation, compare 1.6.4871 `Assembly-CSharp.dll` signatures; monthly with-Mod/error-log comparisons where needed.
- **Limitations**:
  - Local versions mostly **newer than player logs**; many fixed cases infer historical files from old folders/logs, marked likely.
  - `D:\RimWorld16\Mods` lacks `Textures/`/`Sounds/`; assets checked only in Workshop copies. Local regenerated `.dds` does not represent author packages.
  - Attribution from RimArcho v3/v4 (about 78% source accuracy), especially weak for `xref.getnamed`.
  - 1.6 logs only; 1.7 changes (Waylith) unrepresented.
- **Verification records**: per-case paths/code locations in session temporary directory, not committed; reproducible by this method. Counts in §1.
