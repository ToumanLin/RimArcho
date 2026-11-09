# Save content and attribution research, 2026-11-09

Status: **research and proposal; no implementation**. Repository evidence is from `feat/save-content`, HEAD `d0e7e412`. Measurements use synthetic data only. Real-save coverage, historical attribution accuracy, and compatibility with the exact 1.6.4871 build remain **unverified**.

The right card can report **content references found in this save, matched against the currently installed versions of its Mods**. It cannot measure how much content the player experienced. Adding a bounded set of named Def fields is inexpensive; recovering compressed terrain and world data is a separate problem. The existing attribution primitives are useful, but neither the missing-Mod ownership routine nor `def_fact` is a complete content catalog. In particular, Patch-added Defs are absent from persisted Def facts, and inheritance needs the child's load position rather than the last parent anywhere in the list.

## Evidence and scope

Read with 12 §1.4, §1.5, §1.7, §2, §3 and §4.6; 05 §1 and §5.1; [the earlier save-format research](2026-11-04_save-format-fields.md). Those specifications remain unchanged.

The earlier research's source was a temporary decompilation of 1.6.4871 rev590, explicitly deleted afterwards (`2026-11-04_save-format-fields.md:5`). That source is unavailable. No installed game, installed Mod, real save, or player RimArcho data directory was opened. A public source snapshot was fetched into a new external scratch directory:

- Source: [IlyaChichkov/RimworldDecompiled, pinned commit d2d70e57a8042c08c3f7ac41659b4988d63e2491](https://github.com/IlyaChichkov/RimworldDecompiled/tree/d2d70e57a8042c08c3f7ac41659b4988d63e2491).
- Assembly version `1.6.9438.37837` (`Properties/AssemblyInfo.cs:18`); `RimWorld/VersionControl.cs:41` converts it to **1.6.4633 rev1261**. It is evidence for that build, not independent verification of 1.6.4871.
- Below, **`G:path:line`** means a file and its original line in that pinned source tree. Example: [G:Verse/ThingDef.cs:954](https://github.com/IlyaChichkov/RimworldDecompiled/blob/d2d70e57a8042c08c3f7ac41659b4988d63e2491/Verse/ThingDef.cs#L954). Abbreviated game filenames in the same evidence cell refer to this source too; `internal/` and `docs/` paths refer to RimArcho.
- Probes and copied repository code live under `%TEMP%\rimarcho-save-content-20261109`; this is an isolated scratch copy, not a worktree or a copy of player data. `testkit.New` creates synthetic Core, DLC and Mods (`internal/testkit/testkit.go:71`).

Evidence-to-finding-to-design trace:

| Evidence | Finding | Consequence |
| --- | --- | --- |
| Game ExposeData and Scribe methods (A) | Def references include named scalars, lists and dictionary keys | Use context rules and retain reference kind |
| Persisted facts and synthetic Patch probe (B) | Effective folder selection is not persisted post-Patch content | Add analysis-scoped catalog output for Patch provenance |
| Game predicates and XML inheritance (C) | Local flags and a global last-parent rule cannot reproduce classification | Store unresolved fields; resolve in the save's installed scope |
| Synthetic timings (A, B, C) | Scalar extraction and batched lookup are affordable at the tested scale | Keep L1 streaming and L2 selected-save-only |
| Persistence and retention semantics (D) | Presence can be incidental; absence can reflect lost evidence | Avoid claims of use, completion or never encountered |

## A. Which reference forms to read

### Paths and serialization

Paths are relative to `savegame/game/`. `P` is any serialized pawn, including map things, corpses/containers, caravans and world pawns. `T` is a serialized thing. `I` is `world/ideoManager/ideos/li`; `Q` is `questManager/quests/li`; `L` is a letter in `history/archive/archivables/li` (active-stack letters have similar objects). Deep-object paths are examples of the named classes' layout, not a promise that every subclass uses them.

`Scribe_Defs.Look` writes the selected label with the Def's defName, omitting null (`G:Verse/Scribe_Defs.cs:5`). `Scribe_Collections` with `LookMode.Def` writes `li` children using that method (`G:Verse/Scribe_Collections.cs:159`); dictionaries write paired `keys/li` and `values/li`. Never treat arbitrary `li`, `root`, `source`, `kind` or `ideo` text as a Def without context.

| Reference and path | Serialization and evidence | What `<def>` alone loses |
| --- | --- | --- |
| Thing/race/item `T/def` | `Thing.ExposeData`, `Scribe_Defs.Look(ref def, "def")`; `G:Verse/Thing.cs:1221` | Nothing for full serialized things; compressed things are separate below |
| Pawn kind `P/kindDef` | `Look(ref kindDef, "kindDef")`; `G:Verse/Pawn.cs:4552` | PawnKindDef even when the race ThingDef is present. Also written by `PawnKindCount.cs:14` and `MechClusterSketch.cs:22` |
| Material `T/stuff` | `Look(ref stuffInt, "stuff")`; `G:Verse/Thing.cs:1249` | Material ThingDefs referenced only as stuff, not as separate stacks |
| Research `researchManager/currentProj`; `{progress,techprints,knowledge}/keys/li` | Def scalar and Def-key/value dictionaries; `G:RimWorld/ResearchManager.cs:70` | ResearchProjectDefs. `currentKnowledgeProjects/li/{category,project}` uses deep records with two Def scalars (`ResearchManager.cs:17`); category is not a project |
| Xenotype `P/genes/xenotype` | `Look(ref xenotype, "xenotype")`; `G:RimWorld/Pawn_GeneTracker.cs:1321` | XenotypeDef; a custom xenotype name is a string, not proof of a named Def |
| Genes `P/genes/{endogenes,xenogenes}/li/def` | Deep lists (`Pawn_GeneTracker.cs:1319`), then `Gene` writes `def` (`G:Verse/Gene.cs:167`) | Standard pawn genes are already counted |
| Gene packs/xenogerms `T/geneSet/genes/li` | Deep `geneSet` (`G:RimWorld/GeneSetHolderBase.cs:120`); Def list `genes` (`GeneSet.cs:270`) | GeneDefs in packs, independently of genes in pawns |
| Ideology `P/ideo/ideo`; `I/...` | Pawn tracker uses `Scribe_References.Look(ref ideo, "ideo")`, an `Ideo_N` object reference (`G:RimWorld/Pawn_IdeoTracker.cs:402`); manager deep list (`IdeoManager.cs:292`) | Ideo is not a Def; do not send `Ideo_N` to DefDefiners |
| Memes `I/memes/li`; precepts `I/precepts/li/def` | Def list `memes`, deep list `precepts` (`G:RimWorld/Ideo.cs:519`); Precept Def scalar (`Precept.cs:823`) | Memes missed; standard PreceptDefs already counted |
| Ritual pattern `I/precepts/li/sourcePattern` | `Look(ref sourcePattern, "sourcePattern")`; `G:RimWorld/Precept_Ritual.cs:922` | RitualPatternDef; other ritual scalars include `attachableOutcomeEffect` at line 928 |
| Ideology extras `I/{culture,iconDef,colorDef}` | Def scalars (`G:RimWorld/Ideo.cs:515`) | CultureDef, IdeoIconDef, ColorDef, which the draft culture row omits |
| Storyteller `storyteller/def`; difficulty `storyteller/difficulty` | Two Def scalars (`G:RimWorld/Storyteller.cs:154`) | Storyteller already counted; DifficultyDef missed, and is not another storyteller |
| Scenario `scenario/{name,parts/li/def}` | Name/text and deep parts (`G:RimWorld/Scenario.cs:109`); ScenPartDef (`ScenPart.cs:29`) | **No ScenarioDef reference in this serializer.** Parts and a localized name cannot reliably recover it |
| Player faction `scenario/playerFaction/factionDef`; factions `world/factionManager/allFactions/li/def` | Def scalars (`G:RimWorld/ScenPart_PlayerFaction.cs:15`, `Faction.cs:272`) | Scenario faction hint missed; instantiated factions normally supply the FactionDef already. `Faction_3` is an object reference |
| Abilities `P/abilities/abilities/li/def`; hediff abilities `P/healthTracker/hediffSet/hediffs/li/abilities/li/def` | Deep lists (`G:RimWorld/Pawn_AbilityTracker.cs:189`, `Verse/Hediff.cs:417`), Ability Def scalar (`RimWorld/Ability.cs:1272`) | Standard saved AbilityDefs already counted, including psycasts |
| Hediffs `P/healthTracker/hediffSet/hediffs/li/def`; traits `P/story/traits/allTraits/li/def` | Def scalars (`G:Verse/Hediff.cs:409`, `RimWorld/Trait.cs:72`) | Standard HediffDefs and TraitDefs already counted; hediff `source` and `sourceHediffDef` add source references (Hediff.cs:410) |
| Quest script `Q/root` | `Look(ref root, "root")`; `G:RimWorld/Quest.cs:522` | QuestScriptDef. Further references: `Q/parts/li/questScript` (`QuestPart_AddGiverQuest.cs:40`), incident parameters' `questScriptDef` (`IncidentParms.cs:175`) |
| Active conditions `{world,maps/li}/gameConditionManager/activeConditions/li/def` | Deep list (`G:RimWorld/GameConditionManager.cs:198`), Def scalar (`GameCondition.cs:198`) | Active GameConditionDefs already counted; expired conditions need not remain |
| Queued incidents `storyteller/incidentQueue/queuedIncidents/li/firingInc/def` | Deep queue (`G:RimWorld/Storyteller.cs:156`, `IncidentQueue.cs:42`, `QueuedIncident.cs:38`), IncidentDef (`FiringIncident.cs:31`) | Already counted; queue membership does not prove the incident fired |
| Incident settings `scenario/parts/li/incident`; `Q/parts/li/incident` | Def scalars (`G:RimWorld/ScenPart_IncidentBase.cs:19`, `QuestPart_Incident.cs:97`); also `SignalAction_Incident.cs:22` | IncidentDefs in settings/quest parts, potentially naming future events |
| History and letters `L/def`; `history/historyEventsManager/...` | Letter writes **LetterDef** (`G:Verse/Letter.cs:72`). History uses `DefMap<HistoryEventDef,...>` (`RimWorld/HistoryEventsManager.cs:264`); DefMap writes indexed `vals`, without Def-name keys (`Verse/DefMap.cs:54`) | A threat letter does not identify IncidentDef. HistoryEventDef is another type; its indexed map cannot be decoded by collecting text names |
| Letter links `L/{hyperlinkThingDefs,hyperlinkHediffDefs}/li` | Def lists (`G:Verse/ChoiceLetter.cs:151`) | Help links, not proof an object existed |
| Appearance/backstory `P/story/{childhood,adulthood,hairDef,bodyType,headType,furDef}`; `P/style/{beardDef,faceTattoo,bodyTattoo}` | Def scalars (`G:RimWorld/Pawn_StoryTracker.cs:323`, `:416`; `Pawn_StyleTracker.cs:235`) | Cosmetic/backstory content; draft has no category. Style also saves queued next-style Defs |
| Bills `.../bills/li/recipe`; filters `.../allowedDefs/li` | Bill scalar (`G:RimWorld/Bill.cs:208`); filter Def list (`Verse/ThingFilter.cs:293`) | RecipeDefs and allowed ThingDefs; allowance does not prove production/possession |
| Tales `taleManager/tales/li/surroundings/biome`; `.../pawnData/{kind,peq,app}` | Def scalars (`G:RimWorld/Tale.cs:111`, `TaleData_Surroundings.cs:37`, `TaleData_Pawn.cs:47`, `:59`) | Cheap named BiomeDef and historical kind/equipment/apparel references; depends on surviving tales |
| Pocket tile `maps/li/pocketTileInfo/biome` | Deep tile (`G:Verse/Map.cs:633`), Def scalar (`RimWorld.Planet/Tile.cs:361`) | Some cheap biome evidence; not ordinary world-tile coverage |
| Terrain `maps/li/terrainGrid/{topGrid,underGrid,foundationGrid,tempGrid}` and `...Deflate`; things `maps/li/compressedThingMap[Deflate]` | Short-hash grids (`G:Verse/TerrainGrid.cs:570`, `:671`; `MapFileCompressor.cs:21`, `:43`) | Built terrain and compressible rocks/chunks can leave **no named reference** |
| World biome `world/grid/layers/values/li/tileBiome[Deflate]` | Layer dictionary (`G:RimWorld.Planet/WorldGrid.cs:682`), byte array of biome hashes (`PlanetLayer.cs:1060`) | Ordinary tile biomes. This source uses layers; the earlier report's unlayered `world/grid/tileBiomeDeflate` is not this snapshot's shape |

This inventory covers the requested categories and important misleading forms; it is not an allowlist for all game/Mod serializers. Mod code can choose any label and collection shape. Adding every field ending in `Def` still misses stuff, root, recipe, lists and dictionaries.

### Cost of extra forms in `savefile.Scan`

The lexer already sees every element and retains at most 512 bytes for a short leaf (`internal/savefile/scan_lex.go:22`). Interning and extraction are in `scan_rules.go:347` and `:523`; the absolute-path rule trie starts at `:101`. Extra scalar/list rules need no second pass or DOM. Memory is **O(new distinct references)** plus existing depth/ID state, not constant with reference cardinality.

Every addition to persisted L1 coverage or meaning requires a **ParserVersion bump** (`internal/savefile/scan.go:8`), currently 1. One combined rollout can use one bump. Reclassifying/reattributing existing references in L2 alone does not require rereading saves.

| Extra form | Processing and memory cost | Measurement or remaining work |
| --- | --- | --- |
| kindDef | Intern one value per occurrence; no need to retain every pawn | Dense benchmark median 88.74 ms vs 88.16 ms with probe disabled |
| stuff | Same; materials usually repeat heavily | 89.11 ms on that fixture |
| Research current/progress keys | Reuse existing path actions; progress names already retained separately | 88.39 ms. L2 could union current Research fields immediately, but must report mixed coverage explicitly |
| Xenotype, quest root, sourcePattern, factionDef, recipe, appearance | Small context dispatch and interning; proportional to new names | Combined scalar prototype 90.49 ms, including available kind/stuff/research/appearance fields. No realistic xenotype/quest/ritual population in fixture: individual deltas **unmeasured** |
| Meme, GeneSet, letter-link Def lists | Match parent path/class; collect only li leaves; O(unique new names), no pairing | Same scalar cost model; these list populations are **unmeasured**, not in combined figure |
| Other research dictionaries | Intern only Def-key side; retain positive values separately for stronger conclusions | O(keys); techprints/knowledge not separately measured |
| Terrain/compressed things/world biome; indexed DefMap | Decode base64/deflate, map hashes/indexes to exact loaded Def universe; bound streaming inflate and output | **Unverified/unmeasured.** Ushort grids use approximately 2 decoded B/cell/layer; 250×250 is 125 KB/grid before bookkeeping. Reproducing hashes/indexes/back-compatibility is the main correctness cost. Do not guess from installed XML |
| ScenarioDef | No saved source identity in Scenario.ExposeData | No scalar rule fixes it; retain scenario name as metadata |

Measurements: Windows/amd64, Ryzen 9 7950X, Go 1.27.2, warm filesystem cache. Unmodified `BenchmarkScan27MB`: **104.14 ms**, 25.77 MiB (27.0 decimal MB), **3.657 MiB peak heap growth**, 7,327,104 B allocated. Prototype: 3 repetitions × 20 scans/mode; table shows medians. Baseline ranged 87.25–94.48 ms, so sub-millisecond differences are within noise. Allocations stayed around 7.3–7.4 MB/scan. The scalar name switch is a cost probe, not proposed context validation.

A separate 15,000-distinct-kindDef probe took 2.12 ms without extraction, 5.03 ms with it. Additional allocation **4,490,400 B**; retained heap increased about **1.11 MB**, roughly 74 B/new 16-character name. This includes final-map construction. It is a single-run cardinality estimate, not a peak-memory guarantee. Typed evidence/samples add storage beyond this untyped probe.

## B. Attribution with today's facts

### What can be reused

`store.DefDefiners` is an indexed, chunked **candidate lookup**, not a winner selector. It returns distinct `(instance, Def type)` per defName, excluding abstract rows, in 500-name batches (`internal/store/saveowners.go:10`, `:15`, `:34`). It joins Store.MM views from **all environments**. It does not filter save candidates, evaluate MayRequire, check view freshness or return classification fields. Query-plan test: `internal/store/saveowners_test.go:10`.

Resolve save packageIds to installed Instances; preserve save order; filter candidates before selecting winners. Include Core/DLC. `source.Derive` adds official Data (`internal/source/source.go:104`); official folders derive ludeon.rimworld.* identifiers (`internal/scan/about.go:137`); analysisMods does not exclude them (`internal/app/analysis.go:47`). Synthetic Core/Royalty below confirm their Defs reach def_fact; actual installed official counts remain unverified.

`App.attributeSave` serves **loss consequences**, not a full inventory (`internal/app/saveowners.go:456`, `:548`):

- It collects Def ownership only with missing/disabled issue rows and required views. Content provided by the loaded list is deliberately not reported as lost.
- It chooses among Mods with issue rows and can distribute unexplained references across unindexed Mods by elimination. That inference must not become definite catalog ownership.
- Its memo (`ownerMemo`, `saveowners.go:440`) and generated-name batching (`loadDefs`, `:850`) are reusable patterns; SaveOwned and the lost predicate are not the card model.
- `inSaveList` changes environment/selected plan (`internal/app/saves.go:684`); it **does not recreate the save's original enabled set**, reevaluate folders or rerun analysis.

### Rules and limitations

| Issue | Evidence and conclusion | Required behavior |
| --- | --- | --- |
| Same name in several types | Scribe resolves generic Def type (`G:Verse/Scribe_Defs.cs:5`); databases are type-scoped (`DefDatabase.cs:8`); synthetic SharedName returns ThingDef and TraitDef | Last provider **per (type,name)**, not per string. Known context narrows type; unknown def keeps multiple candidates explicitly ambiguous. Raw-name and typed-candidate totals may differ |
| Load order | DefDatabase orders OverwritePriority, then Mod order; duplicate replaces earlier (`G:Verse/DefDatabase.cs:22`, `:54`). Core priority 0, others 1 (`ModContentPack.cs:80`) | Normal valid lists give last-in-save-order within type. Same-Mod duplicate definitions skip after first; Patch-created Defs enter afterwards. A universal last-string rule is insufficient |
| Abstract | SQL excludes abstract=1; named abstract parents are needed for classification | Do not count abstract parents; retain for inheritance. Known abstract-only names should not be called removed content without qualification |
| Conditions/folders | DefFact.Loaded exists (`internal/analyze/analyze.go:54`) but raw lookup does not call it; folders depend on enabled set (`internal/content/content.go:64`) | Evaluate save packages. Current-plan views may be wrong; missing/mismatched sets mean unchecked, not absent source |
| Generated Defs | impliedPrefixes/impliedSuffixes and recursive loadDefs resolve a base (`internal/app/saveowners.go:236`, `:850`) | Prefer explicit eligible definition, then bounded/versioned generated rule. Keep original name, role and base identity; mark inferred |
| Generated role/type | Corpse provider comes from race; techprint from research; frame/blueprint from building **or terrain**. Memo copies base definers including Type | Base Type is not generated Type/category. Do not turn techprint into research or corpse into living race in totals. Wrappers are not independent playable content |
| Incomplete generators | Source has UnnaturalCorpse_ (`G:RimWorld/ThingDefGenerator_Corpses.cs:74`), absent from prefix list | Some generated content remains unknown. Mod generators need explicit knowledge; patterns are not proof. Real explicit eligible definitions win |
| Patches | NewDefs captured before simulation (`internal/analyze/analyze.go:197`, `:216`, simulation `:265`); store writes only NewDefs (`internal/store/savecontent.go:29`) | Patch-added Defs **absent from def_fact**. Removals/renames/field changes do not revise raw rows. Effective content in 05 §1 selects files, not post-Patch facts |
| Patch ownership | 12 §3.2 assigns newly added content to Patch author; simulator has provenance clues (`internal/analyze/facts.go:564`) but no persisted resolved catalog | Record creator of new nodes; keep original owner for modified existing Def and track modifiers separately. Custom operations leave partial results |
| Game Patch loading | Assetless nodes go to patchedDefs (`G:Verse/LoadedModManager.cs:433`), then databases after ordinary Defs (`DefDatabase.cs:41`) | Patch-author attribution is RimArcho provenance policy, not necessarily game Def.modContentPack |
| Historical identity | Save stores packages, not snapshots/Instance IDs | Direct match means provided now, not proof of historical file/version |

**Documentation discrepancy:** 05 §5.1 describes inheritance resolution after patches. Code checks parent-name existence after simulation (`internal/analyze/facts.go:515`); it does not resolve inherited classification fields. Neither that wording nor effective-folder selection establishes an existing post-Patch catalog.

### Synthetic resolution share and latency

Fixture built with internal/testkit: Core with 6,000 referenced Defs plus 300 race bases; Royalty with 1,000 referenced Defs; 20 Mods × 350 referenced Defs; one Mod successfully adding 300 Defs via PatchOperationAdd; 300 Corpse_Race_* references; 400 undefined names. Total **15,000 distinct referenced names**. No real files supplied fixture content.

| Group | Names | Direct lookup | With corpse-base fallback |
| --- | ---: | ---: | ---: |
| Core, DLC, ordinary Mods | 14,000 | 14,000 | 14,000 |
| Patch-added | 300 | 0 | 0 |
| Generated corpses | 300 | 0 | 300 |
| Deliberately missing | 400 | 0 | 0 |
| Total | 15,000 | **93.33%** | **95.33%** |

A separate minimal Patch probe reports `result=ok`, one hit on `/Defs`, and no failure reason. Separate checks confirm abstract exclusion, a Def guarded by an absent package still returned by the raw query, and a shared name in two types returning two rows. Percentages describe **controlled test composition**, not real content coverage.

The save was **26,999,451 B**, with 15,000 names and a large skipped grid-like leaf. Scan from an in-memory reader: **5.38 ms**, 4.76 MB allocated. Its sparse XML is not equivalent to the dense 27 MB benchmark. Five warm DB runs querying names, looking up bases, filtering save candidates, selecting per-type winners and forming count groups took **28.49 / 29.15 / 32.56 / 31.62 / 29.55 ms** (median **29.55 ms**). Direct lookup: 26.03–29.03 ms. Total allocation: 6.83–7.12 MB/run. No memo; DB pages warmed by setup/validation.

That is about **35 ms for this sparse file plus basic attribution**. Combining separate dense 88–104 ms scan measurements with lookup suggests **120–140 ms**, an **estimate**, not an end-to-end card measurement. Excluded: cache gunzip/JSON, inheritance, Patch replay, labels, pagination, IPC, antivirus first-open and cold disk. The 18 entries distinguish a 27 MB scan baseline from a **29 MB / 15,000-Def** page fixture (`docs/specs/18-performance.md:100`, `:102`); they are not one workload.

Real resolution share and selected-card latency are **unverified**; the VM procedure below measures them rather than extrapolating artificial percentages.

## C. Catalog rules and storage

### Correcting every draft category

This table combines classification with save visibility. Partial means some references, not all. Type mappings are RimArcho product choices; the game does not define this one-category catalog.

| Draft row | Game rule / deciding XML | `<def>`-only coverage and correction |
| --- | --- | --- |
| Factions | FactionDef; hidden separate from isPlayer, humanlikeFaction and displayInFactionSelection (`G:RimWorld/FactionDef.cs:11`, `:24`, `:148`) | Instantiated factions captured; scenario-only hint missed. Hidden factions can exist; label hidden/system instead of ignoring evidence |
| Races | ThingDef.race nonnull and race.Humanlike: intelligence ≥ Humanlike (`G:Verse/RaceProperties.cs:261`); XML race/intelligence | Serialized pawn races captured; kind-only/history missed. No race/humanlike XML field corresponding to draft wording |
| Animals | race.Animal: !ToolUser, IsFlesh, !IsAnomalyEntity (`RaceProperties.cs:265`); intelligence, fleshType and referenced FleshTypeDef.isOrganic (`G:RimWorld/FleshTypeDef.cs:182`) | Serialized races captured. Draft neither-humanlike-nor-mech includes entities, drones and other ToolUser races; use real predicate or other-creatures group |
| Mechanoids | Resolved FleshType equals Mechanoid; missing flesh defaults Normal (`RaceProperties.cs:336`) | Serialized races captured; kind-only missed. Odyssey drones have a separate predicate (`:358`), not automatically mechanoids |
| Buildings | category=Building; IsBuildingArtificial also excludes natural/resource rock and recognizes frames (`G:Verse/ThingDef.cs:1183`) | Full things captured, compressed rocks missed. Draft includes natural rock; prefer artificial buildings for furniture/production/defense and separate construction wrappers |
| Weapons | category=Item, nonempty verbs **or tools**, and !IsApparel (`ThingDef.cs:954`); category, verbs, tools, apparel | Full things captured; tale equipment missed. Primary equipment and Weapons* category names are not the predicate. Draft misses tools-only melee weapons |
| Apparel | IsApparel: apparel nonnull (`ThingDef.cs:876`) | Full things captured, tale/link evidence missed. Apparel with verbs stays apparel |
| Plants | IsPlant: Plant assignable from thingClass (`ThingDef.cs:1315`); thingClass and ancestry | Full things captured. A plant block alone is a hint. Unknown Mod subclasses require type-family evidence |
| Items | Remaining category=Item | Full things captured, stuff-only materials missed. Exclude corpses/minified/construction wrappers from independent playable counts |
| Floors | TerrainDef.IsFloor checks Floor tag (`G:Verse/TerrainDef.cs:336`); natural, layerable and water/soil/rock tags distinguish terrain | Mostly missed by name due to grids. Not all TerrainDefs are floors: rename to terrain or restrict tags/li=Floor |
| Research | ResearchProjectDef; costs determine completion, not presence | Normal manager references missed. Add current/progress/techprints/knowledge project forms. Zero progress is still a reference, not completion |
| Events | Product union IncidentDef and GameConditionDef | Active conditions/queued incidents captured; quest/scenario incident fields missed. Completed incidents often leave no IncidentDef; LetterDef/HistoryEventDef cannot substitute |
| Quests | QuestScriptDef | Normal Q/root missed; add validated script labels. Offered/rejected/expired quest references also count; not completion |
| Xenotypes and genes | XenotypeDef, GeneDef | Pawn genes captured; xenotype and GeneSet lists missed. Custom xenotype display name is not a Def identity |
| Culture | Draft MemeDef, PreceptDef, RitualPatternDef | Precepts captured; memes/patterns missed. CultureDef, StyleCategoryDef/ThingStyleDef and other Ideology types need scope decision |
| Abilities | AbilityDef including psycasts; type mapping, no universal psycast XML flag | Standard ability objects captured. Uninstantiated grants from other Defs do not prove experience |
| Implants and conditions | HediffDef | Standard objects captured; source/help-link fields missed. Includes injury/disease and temporary conditions, not just implants |
| Traits | TraitDef | Saved traits captured; a gene's possible forced trait is not proof a pawn had it |
| Biomes | BiomeDef | Normal grids missed; tales and pocket tiles give cheap partial evidence. Referenced/generated does not mean visited |
| Storytellers | StorytellerDef | Active captured; previous storytellers need not remain. DifficultyDef separate |
| Scenarios | ScenarioDef catalog type; saved Scenario is a deep object | Original ScenarioDef normally absent. Keep name as metadata and source identity unknown |
| Not content | Product exclusion for remaining types, not a game rule | Many jobs/needs/skills/world objects captured. UI should distinguish known excluded references from unknown classification |

Other gaps: PawnKindDef may be a Mod's only reference for a variant of a vanilla race; assigning it to a faction needs association evidence. RecipeDef is referenced by bills (including surgery) and can be playable content without a new ThingDef. Cosmetic/backstory Defs, WeatherDef, RoyalTitleDef, PsychicRitualDef, Odyssey planet-layer/world-generation types and Anomaly entities/rituals lack adequate draft categories. Keep explicit other/unsupported groups; otherwise content Mods could be falsely labeled functional.

ThingCategory also includes Projectile, Filth, Gas, Attachment, Mote, Ethereal and PsychicEmitter (`G:Verse/ThingCategory.cs:3`); these are not all inventory items. Corpses use class (`ThingDef.cs:892`), blueprints entityDefToBuild plus Ethereal (`:896`), frames generated runtime state (`:894`), minifiability minifiedDef (`:541`). A Core minified wrapper can contain a Mod building: preserve contained evidence without counting another playable item. Blueprints/frames mean **planned/under construction**, not built.

### Inheritance across Mods

Raw facts store Name, ParentName, Abstract and file/line (`internal/analyze/analyze.go:364`; `internal/store/schema.sql:173`). **Name is the inheritance key, not defName.** Abstract nodes without defName are valid parents. Classification fields and inherited values are not stored.

The game registers patched XML then resolves inheritance (`G:Verse/LoadedModManager.cs:354`, `:373`). GetBestParentFor picks the named parent with greatest Mod order **not after the child**, with special handling for nodes without a Mod (`G:Verse/XmlInheritance.cs:208`). It is not the last parent in the whole list. Name indexing is not the partial patch's `(Def type, Name)` assumption.

Example: A defines Name=Base as Item; B inherits Base; C redefines Base as Building. B uses A, not C. A Patch-created parent can have null original-Mod provenance and take a different branch. Missing/cyclic/ambiguous inheritance must yield incomplete classification, not silently default to items/other.

Resolution clones parent and recursively overlays child (`XmlInheritance.cs:260`, `:278`). Lists append; Inherit=False resets contents. Empty child elements can leave inherited elements intact (`:349`); attributes are overwritten. A boolean has-verbs without presence/reset semantics is insufficient. Preserve absent/explicit values, null semantics, block resets and list emptiness. Nested MayRequire and custom deserialization/class behavior need validation before claiming exact runtime equivalence.

Memoize by **node identity and scope**, traverse parents with cycle/depth guards, apply overlays, then classify. Exact IsPlant/IsCorpse/custom-subclass predicates need class hierarchy. type_fact stores names only (`internal/store/schema.sql:182`), although the .NET reader consumes Extends for other analyses (`internal/dotnet/compscan.go:141`). Until exposed, recognize known built-in families and label custom-family XML classification inferred/unknown.

### Storage choices and rebuilding existing databases

| Option | Advantages | Costs and correctness constraints |
| --- | --- | --- |
| Final category column(s) in def_fact | Small; fast aggregates | Wrong as immutable per-Mod fact when another Mod changes parent/Patch/dependency. Suitable as local hint or separately keyed resolved result |
| Compact flags / hint string | Few bytes, one column, cheap lookup | Needs unset/false/null/reset and list semantics. Proposed 32-bit hint omits tools, exact plant classes, terrain tags, organic flesh and runtime kinds. Category string alone cannot resolve inheritance |
| Structured raw fields in def_fact | Retains evidence for resolution; shares fact sets/file indexes | More bytes/decoding than flags. Bound selected subtrees/values, not all XML; preserve presence/reset and version encoding |
| Separate local-fact table | Omit irrelevant Def rows; independent catalog version | Needs stable row identity, batch joins/pruning/migration. (set_id,type,defName) is not unique for abstract/duplicate definitions |
| Resolved catalog per analysis scope | Correct home for post-Patch fields, creation provenance and categories | Disk/lifecycle costs; complete ordered-scope key; custom operations remain partial |
| Compute from Def files on demand | No persistent classification fields | Repeated small-file opens, parent closure, Patch replay, version selection and changing files during page load. Background indexing fallback only |

**Recommendation:** bounded structured unresolved fields beside existing Def facts; memoized selected-reference inheritance. Add analysis-scoped resolved catalog rows for post-Patch correctness. Never store patched results under existing per-Mod fact keys. An integer encoding is acceptable if it preserves tested semantics; compactness alone cannot justify discarding required fields.

Cache boundaries:

1. FolderKey hashes snapshot plus ordered normalized folders (`internal/content/content.go:42`); Run appends analyzer version (`internal/analyze/analyze.go:162`). Other enabled Mods/order can change without changing a particular Mod's key.
2. CachedDefs reads exact instance/kind/key (`internal/store/analysis.go:39`). ensureFactSet inserts only a fresh key (`internal/store/factsets.go:97`); saveContent does not rewrite old facts (`savecontent.go:29`). Adding a column alone **cannot backfill**, even if analysis reruns with the same key.
3. Bump analyzer/local-catalog fact version for extraction. AnalysisKey includes analyzer version, included Instances, snapshots, positions and game major/minor (`internal/app/analysis.go:76`). Next analysis creates sets and updates views atomically. Readiness must check required fields/version/key, not just FactViews.Defs (view existence).
4. Resolved catalog keys include resolved installed Instance order, snapshot/folder keys, enabled-package conditions, game version and catalog/analyzer version; include game/type metadata identity where class ancestry matters. content_state keeps one view per instance/game_mm (`schema.sql:164`), not arbitrary historical scopes. Replacing it per save would churn views; reuse immutable sets and add scope-to-set mapping for multiple save scopes.
5. Extraction meaning changes bump ParserVersion; save rows invalidate by size/mtime/version (`internal/store/savecache.go:29`), without schema rewrite or save mutation. Def fields/category-only changes leave L1 valid.
6. Recompute L2 on candidate/order, relevant facts, rules or reference-cache changes. Bound memo/selected results; never materialize every Def for every save during listing.

Current schema is 34 (`internal/store/store.go:19`). A future column/table needs schema increment, fresh DDL and upgrade step. Synthetic `ALTER TABLE def_fact ADD COLUMN research_fields TEXT NOT NULL DEFAULT ''` took **1.52 ms**; page count stayed 371. This measures additive DDL only, not backfill/WAL or rebuilding player libraries. Re-extraction is the real cost; use the analysis child/background model. Keep old Def/DLL facts usable until replacement; mark catalog fields not indexed instead of treating a migration default as valid empty content.

### Assessment of the stopped partial implementation

Reviewed only: codex-partial-impl.patch at the supplied scratchpad path; not applied.

Useful choices: selected-save L2, batched queries, installed-save candidates, presence bits, analyzer version change. Material limits:

- Reads only Facts.Defs; misses named/list forms in A.
- Chooses one winning Mod per **string**, then one category, dropping another type's valid provider; conflicts with 12 §1.5's multiple-type rule.
- Weapon predicate adds Primary/Weapons categories but omits tools/apparel exclusion; plant/animal rules are approximations.
- Last parent in whole order is not child-relative selection; flags omit XML merge cases.
- No persisted Patch-created catalog; uses last-analyzed scope views.
- Generated broad categories lose target/role distinctions, including terrain blueprints.
- Migration deletes **all content_state rows** and Def sets, making DLL views unavailable until rebuilding too. Catalog additions should preserve unrelated view availability.

## D. What the page can honestly claim

Title: **Content referenced in this save**. Basis: **Sources use your installed Mod versions.** Count unique referenced identities, not objects produced, quests completed or hours played. Occurrence totals, if shown, are **references**: objects can be referenced repeatedly, including in filters, bills and history.

| State | Player-facing meaning |
| --- | --- |
| Typed reference/current eligible definition | Source matched in installed content; historical version unknown |
| Generated/base match | Source inferred from base content; expansion shows base and role |
| Unknown type/several same-name types | Multiple candidates; not independently confirmed encounters |
| Missing or wrong-scope/version facts | Not fully indexed; offer indexing, separate from unmatched |
| No eligible source after supported lookup | **Source not found in installed content. The Mod may have changed or been removed, or this reference may be generated or not supported yet.** Not a load error by itself |
| Malformed/truncated save | Partial result before parse failure |
| Unsupported category/serializer | Coverage/classification unknown; not evidence the Mod is functional |

Severe underestimation: terrain, biomes, completed incidents, quest history, past storytellers and original scenarios. Current def map systematically misses research, xenotypes, memes and ritual patterns; named rules repair that specific gap, not historical retention. Compressed rocks/chunks and stuff-only materials need their extra forms. Every category can lose traces when objects disappear/history is pruned; none supports never-played claims from absence alone.

There is overstatement too: generated factions need not have been contacted, world pawns need not be encountered, filters/bills can express preferences, queued incidents are future content, letters link help Defs. Separate **object/state**, **history** and **configuration/link** evidence. The broad reference set fits this card; future 12 §3 unused comparisons need a stronger policy. Unioning colony saves improves retention but cannot prove encounters or absence.

Use confidence classes (direct-current, generated-inferred, ambiguous, unchecked, unmatched), not invented percentages. Do not distribute unknown names among missing Mods as definite sources; 12 §4.6's inferred consequence display is a different purpose.

## E. Proposed design and decisions

### Data flow and API

1. **L1:** retain existing untyped def map for compatibility. Add bounded reference collection keyed by expected Def type (when known), name and evidence kind; occurrence count, rule ID and small sample path. Add high-value scalar/Def-list forms in one pass. No identity inference from localized names, arbitrary leaves or object IDs. One ParserVersion bump.
2. **Local facts:** add unresolved fields from C with presence/merge information and field version. Reuse file/line provenance and immutable sets. Batch query returns fields, conditions, parent identity and readiness. Include Core/DLC; rebuild in analysis child.
3. **Scope:** resolve save packages with Instance/pack rules; explicitly compute installed enabled set/order, folders and conditions. Queue missing indexing; query returns partial/unchecked cached results. Switching to matched plan is insufficient.
4. **L2:** match typed candidates, retain ambiguity for unknown types; effective provider per type/name. Try eligible generated rules with bounds; preserve base/role. Resolve child-relative parent closures and classify. Keep wrappers, configuration-only evidence and unsupported types apart from playable totals.
5. **Post-Patch catalog:** emit surviving/new nodes, inherited classification and creation provenance from supported simulation operations. Persist under analysis/scope key; creator separate from modifiers. Custom operations/runtime generators leave partial flags. Never replay full list in selected-save handler.
6. **Surfaces:** summary for selected save only; bounded Mod/category counts and paged expanded references. Listing stays L0/cached summaries. CLI/MCP share application result; eventual output changes require normal spec/changelog updates during implementation.

Illustrative shape, not implemented:

```json
{
  "state": "partial",
  "basis": "installed-versions",
  "coverage": {
    "referenceRules": 2,
    "catalogRules": 1,
    "compressedGrids": false,
    "patchCatalog": false,
    "saveComplete": true,
    "uncheckedMods": ["example.not-indexed"]
  },
  "counts": {
    "rawNames": 15000,
    "resolvedNames": 14300,
    "unmatchedNames": 700
  },
  "mods": [{
    "packageId": "example.content",
    "instance": "instance-reference",
    "categories": [{"key": "weapons", "unique": 12}]
  }],
  "details": {"items": [], "nextCursor": null}
}
```

Numbers illustrate the fixture, not expected coverage. Detail items carry name, expected/resolved type, count, evidence kind, category, match kind, Instance and file/line; generated items add base/role, ambiguous items alternatives. Raw names count strings; category counts use typed identities under the chosen ambiguity policy and need not sum to that count. Filter detail query by Mod/category, bound pages (e.g. 100), sort deterministically. Do not ship 15,000 names eagerly or construct all their DOM rows.

Suggested phases: **1**, conservative card with named forms, inherited classification and explicit Patch/grid limits; **2**, post-Patch provenance and exact type-family support; **3**, grids/unused-content only after VM measurements and evidence-policy agreement. Phase 1 cannot claim full 12 §2 / §3 support.

### Decisions needed

| Decision | Recommendation |
| --- | --- |
| Present in card vs used in later analysis | Card includes supported references with evidence kinds; unused analysis excludes configuration/help links by default, avoids never-encountered wording |
| Partial Patch/grid support first? | Allow clearly partial card; post-Patch effective catalog is explicit separate milestone |
| Several types sharing untyped name | Retain per-type providers/ambiguity; decide whether ambiguous entries enter category totals or separate bucket |
| Category boundaries | Game predicates; add other creatures, recipes/cosmetics; terrain or Floor tag restriction; hidden factions visible; wrappers do not inflate playable totals |
| Scenario and weak history | Scenario metadata unless original identity independently recorded; coverage labels for events/quests/biomes/past storytellers |
| Storage/backfill | Structured unresolved fields, lazy background rebuild, scope catalog for Patches; retain unrelated facts/L1 |

## Reproduction and remaining VM verification

Commands ran in isolated scratch copy; fixture writes are temporary. Original benchmark:

```powershell
go test -count=1 ./internal/savefile -run '^$' -bench '^BenchmarkScan27MB$' -benchtime=5x -benchmem
```

Retained research probes: `%TEMP%\rimarcho-save-content-20261109\repo`:

```powershell
go test -count=3 ./internal/savefile -run '^$' -bench '^BenchmarkResearchExtraForms$' -benchtime=20x -benchmem
go test -count=1 ./internal/savefile ./internal/app -run '^TestResearch(Distinct|Content|Patch)$' -v
```

Scratch copy contains go.mod, go.sum, internal/ and testdata/ from the cited revision. Dense generator: `internal/savefile/scan_bench_test.go:117`; extra fixture composition in B. Extra-form prototype optionally calls existing extract.def from extract.leaf; modes: disabled, kindDef, stuff, research actions, combined scalar-label switch. It measures collection cost, not final rule correctness. All probes passed. SHA-256 identifies retained inputs:

| Scratch file | SHA-256 |
| --- | --- |
| internal/app/research_patch_test.go | `24701E1B5423CD3252139FE7D9B554E4CC442A6CC76751C1D3552F54F1744037` |
| internal/app/research_content_test.go | `032F5030875173712C730215388AFCA540719890DA9B277E2F408761ED9AE44E` |
| internal/savefile/research_extra_test.go | `FF8D9A35D8AC16FBC04173B7A762644092F4C446BE2FBA8593C5F5EDFD3F2835` |
| internal/savefile/research_distinct_test.go | `D642E3FA9AABC3C78EEDCE38535CF96767BAF12018D9951194D0FBDC5D75DB6A` |
| Probe copy of internal/savefile/scan_rules.go | `5F3C91698792B16CA4C0FC50579C7D5C6AE599AF848CAABB3D76290B1CA43CB5` |

Scratch paths are disposable session artifacts, not product dependencies. No probe code is in the worktree. Full Go/frontend suites were not run: deliverable changes documentation only. `node scripts/docs-refs/refs.cjs check` and `git diff --check` passed with the document/index present.

These checks require the **VM**, not host access. None ran here:

| Unverified question | Exact VM check and output to retain |
| --- | --- |
| Exact build and paths | Decompile VM 1.6.4871 with earlier report's ilspycmd -p method into VM scratch, recording assembly hash/build. Compare cited ExposeData, XmlInheritance, DefDatabase and predicates. Inspect existing saves for each path, including layers/pocketTileInfo/tales; record absent as well as present |
| Gain per form | On the read-only 18 save corpus, run existing extraction and each context-rule family independently. Per save: bytes, element count, distinct type/name additions, occurrences and overlap with existing def values. Report category gains without summing overlapping gains |
| Real resolution share | On copied RimArcho data in VM, index matching save scopes. Count direct/generated/ambiguous/excluded/unchecked/unmatched names; separate official content, Patch-created references and unavailable versions. Inspect reproducible samples from each bucket against effective sources/game-resolved Defs |
| End-to-end latency/memory | Dense 27–29 MB save, 15,000-name synthetic save and largest real save: 1 cold, 10 warm runs. Time open/Scan/cache encode-decode/query/inheritance/catalog-read/API separately; median/p95, peak heap/private bytes, allocations, JSON size and mounted rows. Separate cold DB/antivirus costs |
| Classification/inheritance | Synthetic VM Mods: tools-only weapons, apparel with verbs, custom Plant/Corpse subclasses, humanlike/animal/mech/entity/drone races, duplicate parent Names before/after child, list append/empty/Inherit=False, Patch add/rename/remove. Compare to game-resolved Defs/providers; include Floor tags and terrain blueprints |
| Grids/history limits | Prove ushort hashes and DefMap indexes against exact scope, back-compatibility and layers; measure bounded inflate separately. Compare active/finished incidents and offered/completed/deleted quests across saves; check what letters/history actually retain |

Follow 17 §4 and the stricter task boundary: all real-data work stays in the VM, inspected saves/game/Mods stay read-only, database writes use a copy. These checks delimit evidence; they are not implementation work in this worktree.
