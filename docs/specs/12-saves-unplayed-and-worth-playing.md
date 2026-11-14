# 12 · Saves, unplayed and worth playing

> Status: save reading (§1) and Saves (§4) are implemented in the CLI, MCP and GUI, except the items marked "not implemented" there. Selected-save content attribution (§2) and the cross-save Play review of unused content (§3) are implemented with the stated limits: in the GUI, as `saves review` and as `list_saves` with `review`. The separate `unplayed` command and `list_unplayed` tool (13 §4.2, 14 §2) are not implemented, and neither are the items marked "not implemented" in §3. §5 is for 1.2; §6 is a long-term direction.
> Basis: 00 (§2, §4), 00a, 02 §3, 05, 06, 07, 08, 09, 10, 15, 16.

Question two asks: **which installed Mods and content have gone unused across multiple playthroughs?** Read all saves without modifying them and map the Defs they reference back to their providing Mods. This analysis is deterministic and needs no AI; Archo participates only in interpreting results and making suggestions (15 §4).

The answer to question two feeds question three: first establish what has gone unplayed, then choose what is worth playing.

## 1. Save reading

### 1.1 Read-only

- Save analysis is **read-only**: open saves for reading only, never write, rename or move them. Rename, copy, backup and recycle bin moves are separate save-management actions started by the player (§4.4).
- Results stay local and are not uploaded. Only statistics are sent to the model, after consent (15 §3.2).

### 1.2 Save locations

| Location | Belongs to |
| --- | --- |
| `<user data directory>/Saves/*.rws` (shared, own or custom user data directory: `app.Saves`, 03 §3.1) | Environments using this user data directory, and plans launched in those environments. When environments share the game's default user data directory (03 §3.4), deduplicate by real path and read only once |
| `<data directory>/packs/<pack name>/userdata/Saves/*.rws` | This pack (packs use their own `-savedatafolder`, 10 §3) |

Read autosaves (`Autosave-*.rws`) like manual saves; label them "autosave" in the UI.

### 1.3 Format and `<meta>`

Saves are XML with a `<savegame>` root, starting with `<meta>`, followed by `<game>`:

```xml
<savegame><meta><gameVersion>1.6.4633 rev1261</gameVersion>
<modIds><li>ludeon.rimworld</li><li>tester.gamma</li>…</modIds>
<modSteamIds><li>0</li><li>0</li><li>3000000003</li>…</modSteamIds>
<modNames><li>Core</li><li>Gamma</li>…</modNames></meta>
<game>…</game></savegame>
```

- `<meta><modIds>` contains the packageIds enabled when saving, in load order; `<modNames>` corresponds one-to-one.
- Reading rules (`savefile.ReadMeta`, used by the save head and by `readSaveMeta` in `internal/app/exchange.go`): stream parsing, non-strict mode, ignore the declared character set. Stop at `</meta>` without reading the tens of MB in `<game>`. Lowercase all packageIds, record `gameVersion`, and use `modNames` to name "not installed" entries. A save without `<meta><modIds>` gives `no <meta><modIds> in the save`.
- Detection: a file is a save if its extension is `.rws` or its first 4096 bytes contain `<savegame`, whatever its extension (`ReadLoadoutFile`; `ParseList` in `listformats.go` also detects `<savegame`).
- List import: `archo loadout import <save>` puts the save's Mod list into a plan as one reversible edit, without writing game files (07 §7). Test: `TestLoadoutExchange`.
- `<modSteamIds>` is read by the save head (§1.7 L0), not by list import: a nonzero ID lets a not-installed entry offer Subscribe (§4.3); zero means a local or unknown Mod.

### 1.4 Extracting Def references

`savefile.Scan` streams the whole save once. `Facts.Defs` retains every `<def>` value and its occurrence count for existing load checks (§4.6). `Facts.DefRefs` separately records content references by Def type and name; an empty type means unknown. Parser changes invalidate L1 by `savefile.ParserVersion` (§1.7).

- Bare `<def>` values remain untyped unless context identifies factions, storyteller, pawn genes, precepts, traits, hediffs, abilities, active conditions or queued incidents.
- Named references: `kindDef` → `PawnKindDef`; `stuff` → `ThingDef`; research manager `currentProj` and `progress/techprints/knowledge/keys/li` → `ResearchProjectDef`; pawn `genes/xenotype` → `XenotypeDef`; `geneSet/genes/li` → `GeneDef`; ideology `memes/li` → `MemeDef`; precept `sourcePattern` → `RitualPatternDef`; quest `root` → `QuestScriptDef`; scenario `playerFaction/factionDef` → `FactionDef`; bill `recipe` → `RecipeDef`.
- Appearance under pawn `story`: `childhood`, `adulthood`, `hairDef`, `bodyType`, `headType`, `furDef`; under `style`: `beardDef`, `faceTattoo`, `bodyTattoo`. Each uses its corresponding Def type; both tattoos use `TattooDef`.
- Styles (parser 8): a thing's `styleDef` → `ThingStyleDef`; an ideoligion's `thingStyleCategories/li/category` → `StyleCategoryDef`.
- Surviving history: tale `surroundings/biome` → `BiomeDef`. This means only that its record survives.
- What the storyteller fired (`StoryState`, on the world and on each map; parser 6): `storyState/lastFireTicks/keys/li` → `IncidentDef`, the tick every incident last fired there, which the game keeps for good; `storyState/recentRandomIncidents/li` → `IncidentDef` and `recentRandomQuests/li`, `recentRandomDecrees/li` → `QuestScriptDef`, the last five of each. It is the only trace of an event that is over. An abandoned map's record goes with the map.
- What a quest or the scenario fires: `parts/li/incident` under `questManager` or `scenario` → `IncidentDef`, and `parts/li/questScript` under `questManager` → `QuestScriptDef`. These name what a part would fire, not what happened.
- A map's weather: `weatherManager/curWeather` and `lastWeather` → `WeatherDef` (§4.7).
- Grids of Defs by short hash (`Facts.GridDefs`, parser 4; compressed things parser 5): the maps' terrain (`maps/li/terrainGrid/` `topGrid`, `underGrid`, `foundationGrid`, `tempGrid`) → `TerrainDef`, the maps' compressed things (`maps/li/compressedThingMap`: rock, ore and chunks, one a cell) → `ThingDef`, and the world's tiles (`world/grid/layers/values/li/tileBiome`; `world/grid/tileBiome` before 1.6) → `BiomeDef`. Each is what `DataExposeUtility.LookByteArray` wrote: base64, raw-deflated when the element's name ends in `Deflate`, of little-endian uint16s, one a cell, 0 for none. The scanner keeps the cells per hash. A hash is named when the content is attributed (`App.gridRefs`): the game gives the Defs of one type their hashes in defName order, each `StableStringHash(defName) % 65535` (a signed 32-bit 23·31ⁿ polynomial, cast to 16 bits) or, when that is 0 or taken, the next free one (`ShortHashGiver`, decompiled from 1.6.4871); the same is done over the `TerrainDef` and `BiomeDef` facts of the save's Mods that load with them, together with the terrain the game generates at load (`App.impliedTerrain`): rough, rough-hewn and smooth stone of every natural rock that is not a resource rock (`<rock>_Rough`, `_RoughHewn`, `_Smooth`; the rock's `building/isNaturalRock` and `isResourceRock`, own or inherited), attributed through the rock's ThingDef (§2.2), and a carpet of every `TerrainTemplateDef` in every structure colour (the template's defName followed by the ColorDef's without `Structure_`), attributed to the template under terrain. A colour's type is not in the facts, so the structure colours are taken to be the ColorDefs named `Structure_…`, as the game's own are. A named hash counts as a typed reference with its cells as uses. A hash no Def has (a Def a Patch or code added, a Mod not installed or not analysed, or a collision resolved differently: the game sorts with the culture's comparer, this by code unit) is counted in `SaveContentResult.GridUnnamed` by type. Compressed things are named less surely. The game hashes every ThingDef, those it generates included (blueprints, frames, corpses, meat…), and these are not all known: the Def files' ThingDefs of the save's Mods are hashed as above, and a hash they give to no compressible Def is tried as a compressible Def pushed up to three places past its own hash, when exactly one fits. Compressible is taken to be natural rock (`building/isNaturalRock`, own or inherited, ore included) and what derives from `ChunkBase` or `ChunkRockBase`, where the game sets `saveCompressible`; that field itself is not in the Def facts. A copy whose Def is found in any of these grids is marked (`SaveNeed.Grid`).
- References that only set something up or name what may come go to `Facts.ConfigRefs` (same shape; parser 7), not to `DefRefs`: a bill's `recipe`, the `incident` and `questScript` of quest and scenario parts, and a research project whose value in `progress`, `techprints` or `knowledge` is 0 or missing (keys are paired with the values that follow; `currentProj` stays in `DefRefs`). The selected-save attribution (§2.3) and the removal verdicts (§4.7) count both, since the save names the Def either way; unused content (§3) counts `DefRefs` and the grids only.
- Exclude configuration/help links: ThingFilter lists (`allowedDefs`, `disallowedDefs` and filter subtrees), letter `hyperlinkThingDefs` and `hyperlinkHediffDefs`. Arbitrary list items, object-reference roots and cosmetic fields outside their context are not Def references.
- No compressed terrain/world grids or indexed `DefMap` values are decoded. Measurements: 18 §2. Paths and evidence: [save-content research](../research/2026-11-09_save-content-attribution.md).

### 1.5 Mapping Defs back to Mods

- Candidates are this save's `<modIds>`, resolved to currently installed Instances under 07 §4.2, including Core and DLC. Duplicate resolved Instances keep their first save position, independently of the current plan's enabled list.
- Use persisted raw Def facts from effective folders (05 §1). Evaluate retained top-level and nested `MayRequire` / `MayRequireAnyOf` against the save's packages. Patches are not replayed.
- Exclude abstract Defs. Choose the provider last in the save's load order **within each type**. An untyped name counts every eligible `(type,name)` candidate, potentially in several Mods/categories. A typed name matches that type; when no Def of the name has it, a Def of a subclass matches instead, because the game writes a reference by the field's type, not the Def's own class (a `VFETribals.TribalResearchProjectDef` is referenced as a research project). No class hierarchy is stored for Def types: a Mod's own Def class (its type has a namespace) is taken to derive from the referenced type, and the game's own subclasses are listed (`gameDefSubtypes`: `CreepJoinerFormKindDef` under `PawnKindDef`). Candidate totals may exceed distinct-name counts.
- Wrappers/generated Defs follow §2.2. Each attributed identity counts once; occurrence totals are references, not objects made or content completed.
- A reference with no eligible installed definition or supported generated-base match enters `unknown` (source not found), not a load error. Installed save Mods without the new content-field version appear separately in `unchecked` (a Mod that is not installed is not unchecked: its references are source not found); an older Def view is insufficient readiness. A known provider awaiting re-extraction contributes no card totals and is not treated as an unmatched source: what it adds is left out until an analysis that includes it, which for a save of another environment or game version is a list made from the save and analysed (the card offers it). A pack's save uses the pack's own copies, or the library's copy of the packageId while the pack's has no content facts. A Def whose stored content fields do not decode is unclassified; it fails no reader of Def facts.
- Events on the same library: the storyteller's record raises a long colony's events from 5–7 Defs of 2–4 Mods to 37–80 of 9–13 (Vanilla Events Expanded, Real Ruins, Go Explore! and others appear), with no new name without a source; a Mod with nothing but past events moves from safe to care.
- Compressed things on the same library: every hash named in all eight saves (1–11 Defs a save), all of the game's own rock, ore and chunks; with exact hashes alone 2 of 12 kinds in one save were unnamed.
- Grids on the same library (eight saves): 12–30 terrain kinds a save, all named in seven saves and one left unnamed in the eighth; every biome named (11–12 of Alpha Biomes in each). Without the generated terrain 3–6 kinds a save were unnamed, all carpets and stone.
- Measured on a real library (2026-11-10, eight saves of 1,250–2,360 names): 17–38 names per save have no source. Of the 72 distinct ones, most are Defs made in code (bulk recipes, diagonal walls), Defs of Mods no longer installed, and Defs that have since moved to a Mod the save does not list; 2 are Patch-added.
- Sources use **currently installed Mod versions**. References mean only **was present**, not depth of play, contact, completion, or evidence of absence elsewhere.

### 1.6 Tools for Archo and external agents

Save reading has read-only tools in the tool layer (13 §1.2, §4.2, 14 §2): `saves list` (`list_saves`), `saves show` (`get_save`) and `saves review` (`list_saves` with `review`, §3). Save-management actions are `manage_saves` (§4.4). A save reference is `sv:<hex>`, a short hash of the save's real path (13 §2); a file name or `<colony>/<save>` also resolves. Every fix in `saves show` names the command or tool call that performs it, so agents act through the existing tools (§4.3).

### 1.7 Read tiers, cache and background reading

A save is read at three costs; each tier is computed and cached separately. L0: `savefile.ReadHead`, cached in memory per App by path, size and mtime (`app.ListSaves`). L1: `internal/app/saveread.go`, with the cache, `archo saves index`, `Save.Summary` on every listed save with a valid cache row, `App.SaveFacts` for one save, and grouping by world (§4.1). Can it load uses the cached facts for its malformed and class rows and for what a missing Mod owns (§4.3, §4.6). The CLI and MCP show L1 facts (`saves list` the in-game day, `saves show` the play facts; 13 §4.2), and so does the page (16 §2.11): the in-game day on rows, and the play facts and timeline in the right card.

| Tier | What | Cost | Used for |
| --- | --- | --- | --- |
| **L0 head** | File path, size and modification time; `<meta>` (§1.3, plus `<modSteamIds>`); the last 64 bytes, checking for `</savegame>` | A few ms per save | Grouping by RimSaves prefix or file name (§4.1), list matching (§4.2), the Mod list and its differences, cut-off files (§4.3) |
| **L1 full read** | One streaming pass over the whole file (`savefile.Scan` → `savefile.Facts`, versioned by `savefile.ParserVersion`): whether the XML is well formed and where reading stopped; colony identity (world name, seed and persistent random value; player faction and every faction; scenario; maps with their settlement); ticks, start tick, in-game day and date, storyteller and difficulty, real play time, permadeath; the player's humanlike pawns (name, race, kind, dead, slave, where) and counts of animals, mechanoids and prisoners; raid and colonist statistics, research progress, the game's history series (wealth and colonists in full, the rest as last value); letters from the archive; every `<def>` value and `Class="…"` value with counts, plus typed content references (§1.4); references of six kinds checked against the objects the save defines. Element paths and their sources: [save format fields](../research/2026-11-04_save-format-fields.md), from the game's code and unverified on real saves; only named tale biomes are read | Streaming scan and memory measurements: 18 §2 | Grouping by world, consequences of missing Mods (§4.6), play facts and timeline (§4.5), unused content (§3) |
| **L2 derived** | L1 joined with the current state: matched list, differences, selected-save content categories (§2), Def and class attribution (§1.5, §4.6), last-load evidence from the game log (06 §7) | Milliseconds | Can it load (§4.3); content referenced in the selected save (§2) |

- L1 depends only on the file, so it is cached in the database (`save_cache`, 17 §3) by path, size, modification time and parser version, as gzip JSON like `log_cache`; it is not reread while all four are unchanged. A row from `savefile.MinParserVersion` up to `ParserVersion` is valid for lists, grouping, play facts and Can it load (a parser change that only adds facts keeps their meaning: 2 added `DefRefs`), so a new version does not unread every save; raise `MinParserVersion` when an existing field those readers use changes meaning. `DefRefs`, `ConfigRefs` and `GridDefs` changed meaning without raising it: their readers (the content attribution of §2.3, §3.4 and §4.7) require a row of exactly `ParserVersion` and read an older one again, and a new reader of them must do the same. A reader of a newer field reads that save again on demand: `SaveFacts` with read rereads an older row (the CLI's one save, the page's selected one), and `SaveContent` reports an older row as `Stale` (`TestSaveContentOlderParserRow`). Beside the facts the row keeps what lists need (ticks, world identity, world and player faction names, malformed), so listing reads no facts; `App.SaveFacts` loads one save's facts. A file whose size or modification time changes while it is read is being written: not stored. A read that fails is stored too, by path, size and modification time with the current parser version negated and the reason (`store.PutSaveFailure`), unless the file is gone or another program holds it (Windows sharing or lock violation): such a save is `Save.ReadFailed`, `saves index` and the page do not read it again until the file changes or the parser version does (`Save.NeedsRead`), `App.SaveReadFailure` gives the reason, and `saves show` reads it again. L2 depends on the current list and index, so it is not cached and is recomputed when either changes. L0 is read on demand.
- L1 runs in a child process, `archo saves index` (13 §4.2), as background task kind `saves` (16 §2.16, §8.4): progress lines reach the status line and the task record, and each finished file is written to the cache at once, so a stopped run keeps what it read. A save it cannot read gets a line of its own (`progress: saves-failed <ref> <reason>`, `app.ParseSavesFailLine`) and the child exits with an error; the GUI's run then ends done, "read N saves; M could not be read: <reason>", and a child that fails for another reason, even after such a save, ends the run failed with its own error line, not its closing `next:` hint (`app.SavesIndexError.Fatal`). Without `--only`, the run also removes the cache rows of saves that are gone (`PruneSaveCache`), the named first saves notwithstanding. The selected save is read first, then newest first, with 2–4 readers. Reading streams with constant memory; the child process keeps the GUI within its memory budget (16 §10). So that selecting never waits for the run, the GUI reads a selected save that is not read yet itself, one save at a time on its own session (16 §2.11); a save read by both is stored twice, the same row.
- A file modified in the last 5 seconds, or whose size changes between two reads, is **being written**: it gets no verdict and is read again later. The page rereads L0 when it opens and when a running game exits (09 §4).
- CLI and MCP do not wait for the background run: `saves show` reads L1 for its one save inline, before its load check, then lists the saves again when that read was new so the save sits in its world's colony; the previous save it compares with (§4.3) is used only from the cache. `saves list` never reads: it shows the day of saves with L1, `meta only` for the rest (a note counts them and names `saves index`) and `malformed` for saves whose read stopped early. A save being written is not read; `saves show` says so. Measured (synthetic 25.8 MB save, Windows): the inline read takes about 0.1 s, plus about 0.14 s inside the first `open` of the freshly written file (most likely the antivirus scanning it); `saves show` from the cache takes about 2 ms, `saves list` 1 ms.

## 2. Content catalog

### 2.1 Purpose

Players do not know `ThingDef` or `IncidentDef`. The content catalog groups each Mod's Defs into categories players understand, to:

- Say "this Mod adds 12 weapons, 3 factions and 5 events" in Mod details;
- List unused content by category (§3);
- Identify functional Mods (§3.3).

### 2.2 Mapping Def types to categories

The selected-save catalog (`App.SaveContent`) uses these ordered categories. Each candidate has one category; `other` is last. Groupings use the game's predicates, with inference stated where class ancestry is unavailable.

| Category key | Def type / predicate | Rules |
| --- | --- | --- |
| `factions` | `FactionDef` | Hidden factions count, with a hidden count and translated name suffix |
| `races` | Race ThingDef, intelligence ≥ Humanlike | Humanlike races |
| `animals` | Race: not ToolUser, organic flesh, not an Anomaly entity | FleshTypeDef `isOrganic` defaults true; omitted flesh defaults Normal |
| `mechanoids` | Race flesh type is Mechanoid | Humanlike takes precedence |
| `creatures` | Other races | Entities, drones, other ToolUser/nonorganic races. A PawnKindDef is not content of its own: it counts as its race (the race ThingDef it names, through its parents), attributed to the race's Mod; a kind whose race is not found stays itself under `other` |
| `buildings` | Artificial building | category Building, excluding natural/resource rock |
| `weapons` | Item with verbs or tools, without apparel | Tools-only weapons count; inherited lists and resets matter. This is `ThingDef.IsWeapon`: wood, beer and tusks have tools and `equipmentType` Primary in 1.6, so they count as weapons |
| `apparel` | Nonnull apparel block | Apparel with verbs remains apparel |
| `plants` | Plant thingClass family | Built-in Plant/Plant_* recognized; custom subclasses inferred from retained class/plant evidence and marked inferred |
| `items` | Remaining category Item | Food, medicine, materials, etc. |
| `terrain` | `TerrainDef`, `TerrainTemplateDef` | All terrain, natural included; read from the maps' grids and named references (§1.4) |
| `research` | `ResearchProjectDef` | Zero-progress keys still count |
| `events` | `IncidentDef`, `GameConditionDef` | Queued/active content and what the storyteller fired (§1.4); not proof of completion |
| `quests` | `QuestScriptDef` | Includes surviving offered/expired quests |
| `xenotypes` | `XenotypeDef`, `GeneDef` | Named xenotypes and genes |
| `culture` | `MemeDef`, `PreceptDef`, `RitualPatternDef`, `CultureDef`, `StyleCategoryDef`, `ThingStyleDef` | Supported saved reference forms only |
| `abilities` | `AbilityDef` | Includes psycasts |
| `hediffs` | `HediffDef` | Implants, conditions, diseases, injuries |
| `traits` | `TraitDef` | |
| `recipes` | `RecipeDef` | Includes bills and generated recipes |
| `appearance` | `HairDef`, `BeardDef`, `TattooDef`, `BodyTypeDef`, `HeadTypeDef`, `FurDef`, `BackstoryDef` | Appearance and backstories |
| `biomes` | `BiomeDef` | The world's tile grid and tale evidence (§1.4) |
| `storytellers` | `StorytellerDef` | Current and any other surviving supported references |
| `scenarios` | `ScenarioDef` | Usually no original identity survives |
| `other` | Remaining types or unknown classification | Mechanics/UI references remain visible; incomplete classification marks names classification unknown |
| `unknown` | No eligible source | Separate source-not-found group; outside Mod/category totals |

**Inheritance.** Persist bounded unresolved fields (05 §5.1), resolving only referenced nodes and dependencies for the selected save. XML `Name` is the parent key across Def types. Choose the greatest Mod order not after the child; ties keep the first node. Abstract parents may omit `defName`. Memoization is local to this query, with cycle and 64-level depth guards. Lists append; empty children preserve inherited lists; `Inherit="False"` resets; null blocks clear inherited presence. Missing, cyclic, over-depth or truncated inputs produce unknown classification, without default predicates.

**Generated Defs.** `Corpse_`, `Blueprint_`, `Blueprint_Install_`, `Frame_` point to their base: it counts once in its own category/provider without a separate wrapper entry. Minified things contribute their contained serialized Def, not a wrapper item. Other generated forms use their base provider but their own identity/category: `Techprint_`, `Meat_`, `Psytrainer_`, `Neurotrainer_` are items; `Make_`, `Administer_` are recipes; `_Rough`, `_RoughHewn`, `_Smooth` are terrain. Construction references mean planned/under construction, not necessarily built.

### 2.3 Selected-save result and limits

`App.SaveContent` uses cached L1 facts only and never reads the save. Missing/invalid cache returns `Read=false` with a note; a row of an older parser returns `Stale` (the GUI then reads the save on the page's own session, as an unread selected one, and asks again on its `saves-read` event; a read that failed is not retried while the page is open); malformed cached saves yield partial results with a note. GUI `SaveContent(ref)` returns `view.SaveContent` (`internal/view/saves_content.go`): distinct names, totals without names, Mods in save order, unknown sources, unchecked Mods and coverage limits. Mod Def counts are distinct `(type,name)` candidates; category names sort by references then name, capped at 100. Empty categories/Mods are omitted. Quests, scenarios and storytellers carry `Under=true` (`contentUnder`): the save keeps little or nothing of the ones already over. Events, biomes and terrain do not, since they are read from the storyteller's record and the grids (§1.4); the limits below still say what those miss.

Compute only for the selected save, never during listing. Queries batch names/candidate Instances, including parent/race/flesh dependencies; no Def XML is opened and L2 is not persisted. `saves show --content` and `get_save content=true` share text/counts (13 §4.2, 14 §2).

Coverage limits accompany every read result:

- Patch-added Defs and Patch changes to existing Defs are not reflected; no post-Patch catalog.
- Terrain, biomes, rock, ore and chunks are read from the grids (§1.4), the last three less surely.
- Past events are read from the storyteller's record (§1.4). Finished quests the game has dropped and past storytellers leave nothing; the history events manager's indexed maps are not decoded.
- Scenario names/parts normally do not recover the original ScenarioDef.

The same result carries what the save needs of each Mod (`Needs`, §4.7).

This result lists only what the save references. What each Mod adds in full, and what a colony's saves together lack of it, is the Play review (§3). Absence from this result cannot establish never played.

## 3. Unused content

Under Saves › Play review (16 §2.11). Consider only saves where the player actually played with the Mod enabled: compare a Mod only against saves whose `<modIds>` includes it.

### 3.1 Coverage and presentation

For each Mod, the review counts the content added and the content seen, by category, and lists the names not seen. Functional Mods (§3.3) have no counted content. The GUI presents these results as a map with two content groups and a functional-Mod count at its foot; the groups, square sizes, colour steps, grouping switch and selected Mod's details are specified in 16 §2.11. CLI and MCP list never-seen and partly-seen Mods and report counts of all-seen and functional Mods.

- Count **colonies** (§4.1): union all saves of the same colony, not only its latest (what was used up or sold is only in older saves), so that saving a colony 30 times does not count as 30 playthroughs. Saves are read in the background (§1.7); until all are read the result says how many it covers.
- Count only Mods still installed and analysed (§3.4). Current enabled status does not affect the result.
- The GUI shows how many colonies and saves had each Mod enabled, plus its category totals and the names not seen (16 §2.11). A current-list enabled mark, a small-sample warning and individual save evidence are not implemented.

### 3.2 Interpretation

- Evidence is of three kinds. Seen: the Def is in `DefRefs` or in a grid (§1.4): a thing, pawn, terrain or biome that is there, research with progress, an incident the storyteller fired. Set up only: it is only in `ConfigRefs` or in what §1.4 excludes (filters, help links); this does not count as seen. Absent: say "not seen in the saves", never "never encountered"; categories a save undercounts (§2.3) carry that mark here too.
- Recipes are never seen: a save names a recipe only in bills. The review leaves recipes out of what it counts rather than report them all as not seen.
- Measured on eight saves of a real library: research the save lists with no progress is 82–299 projects a save (most of the list in a young colony, a quarter in an old one), bills name 0–56 recipes, quest and scenario parts 0–2 incidents; against 1,053–2,443 names seen.
- Def references mean only "was present": one wall and a hundred walls count alike; researching without building counts only as the research being present.
- Mapping uses the currently installed version. If a Mod update changes defNames, references in old saves lose their source (§1.5), biasing conclusions toward "unused". Explain this in the UI.
- Defs added by a Patch (effective content in 05 §1) belong to the Mod adding the Patch.

### 3.3 Functional Mods

UI, performance, bug-fix and pure mechanics-adjustment Mods do not add Defs to saves; saves cannot tell whether they were used. Criterion (draft):

1. A Mod with **no** Def in a playable content category in its effective content (only Patches, assemblies, textures, language files, or mechanics/UI Defs outside the playable groups in §2.2) is functional;
2. The player may mark a Mod "functional" or "not functional", overriding detection. This is player data on its Lineage (07 §6); storage is undecided (possibly a reserved tag).

Functional Mods are excluded from the GUI map and counted in its foot line (16 §2.11). Framework Mods (such as Harmony and HugsLib) naturally fall under rule 1.

### 3.4 Computation and tools

`App.SaveReview` (CLI `saves review`, MCP `list_saves` with `review`, and the page Saves › Play review, 16 §2.11).

- Each save is attributed as in §2.3 with `ConfigRefs` left out. A Mod's catalog, what it adds by category, is every Def it provides when loaded with the Mods of the first counted save that has it, categorised as in §2.2; a pawn kind is its race. Seen is the catalog's names that some counted save has, so generated names (meat, techprints, smoothed stone) count for neither side.
- Counted categories are those of §2.2 without `recipes`, `scenarios` and `other`. A Mod with nothing in them is functional (§3.3 rule 1; the player's mark of rule 2 is not built).
- Mods: installed and analysed copies only. The game's own (Core, DLC) are included.
- Application result order: Mods with something to see first, by the share seen, lowest first, then by how much they add; functional ones last. CLI and MCP use coverage groups; the GUI orders its content groups by amount added (16 §2.11).
- A save counts when it has a current full read (`savefile.ParserVersion`; the index keeps reads of older parsers, §1.7). The others are counted as left out, or read first on request (`--read`).
- Per-save attribution is kept in the store's `save_review_cache` as gzip JSON: seen names by copy and category, plus a catalog reference. Enabled-copy metadata and categorized catalog names are stored once in `save_review_catalog`, content-addressed by SHA-256 of canonical JSON, with a foreign-key reference from each save. Saves with the same resolved copies, order and catalog share the stored and decoded object. Per-save storage grows with seen evidence, not the library's Def count. A save result is valid only for the same normalized path, file size, nanosecond mtime, exact `savefile.ParserVersion`, `App.ReviewInputKey` and the save's own context. The input key covers what every save rests on: the parser, Def-extractor and review versions, the installed-copy index generation, the active Def sets' identity, content keys and readiness, instance aliases and pack manifest stamps (`ReviewInputKey` in `internal/app/savereview_cache.go` lists the inputs). Content extraction version alone cannot detect changed Mod Defs. The save's context (`reviewContext`) is the Game Config and game version it is attributed in (its own, else the current one), the list whose copies are deployed there (the set of copies, not their order) and its pack, so switching Game Config or editing a list the save is not attributed under keeps its row. The window keeps pages per context (`ReviewContextKey`: current Game Config, version and open list), for the two most recently used contexts; a kept page is used only while the contexts of every listed save are unchanged (`ReviewSavesContextKey`), so editing a list some save is attributed under, current or not, makes it stale. Changing a cache row does not invalidate other entries.
- A hit merges only kept evidence and its shared catalog, without loading full facts, rerunning attribution or rebuilding the catalog. A miss attributes one save at a time and builds the catalog once per enabled-copy order while that set remains in the run's two-entry catalog LRU. Different orders and active packages have separate lookup keys; identical content hashes share the catalog. The run retains the union of seen names, first-counted per-copy catalog references and, only when needed, installed Defs (`defSource`); it never retains all saves' full facts or one catalog per save. The LRU is released with the run. Colonies are processed by last played and saves newest first.
- Each save has one replaceable row, deleted with its full-read row when pruning confirms the file is gone. Catalog publication and the save reference commit atomically. Replacing a reference or deleting saves prunes catalogs with no remaining references; obsolete review-format rows and orphan catalogs are dropped before a run's first write, so a review that only merges kept rows writes nothing. A review does not prune `save_cache`; `saves index` does (§1.7). Schema 37 adds the catalog table and reference; the first review drops the older embedded-catalog format while keeping reusable full-read facts. Deleted SQLite pages are reusable; file shrinking follows normal database compaction. CLI and MCP use the same cache without changing their review output or `--read` behavior.
- On the synthetic 500-Mod / 100,000-Def / 200-save measurement, rows are 17,250 bytes/save plus one shared catalog, versus 1,301,898 bytes/save with embedded catalogs. Combined payload falls from 260.4 MB to 4.74 MB; restart review is 216 ms versus 4.52 s. Full methodology, first-run times, heap measurements and the comparison with rebuilding catalogs are in 18 §2.
- Things' styles and ideoligions' style categories are read since parser 8 (§1.4); without them every style Mod was never seen.

### 3.5 Relationship with Archo

Results need no AI. Archo may "interpret" this page: pick unused content worth trying and explain why (15 §4.2, after 1.1). Send statistics, not saves, with consent under 15 §3.2.

## 4. Saves

The Saves page is in the Saves group of the sidebar, between Playing and Library (16 §2.1, §2.11). A colony is not stored as an object of its own: it is the group of saves of one game. Facts about the colony are read from its **latest save** and shown once; an older save shows only how it differs.

### 4.1 Group by colony

Evaluate in order:

1. A RimSaves file name `<folder>#§#<save>.rws` puts the save in colony `<folder>`; the page shows `<save>` as its name.
2. With L1 facts (§1.7), saves sharing world identity form one colony: world seed, persistent random value and world name (a world generated again from the same seed gets another persistent random value). The player faction's name is left out because players rename it during play; the colony takes the name of its newest read save's faction. A save not read in full yet joins the one world colony in its folder whose saves share its file name (rule 3). Unverified on real saves.
3. Otherwise, group by file name after removing trailing numbers and separators. An autosave (`Autosave-<n>`) names no colony: it joins the colony in the same folder whose save has the same Mod list and is nearest in time, else an "Autosaves" group. Regroup when L1 facts arrive.

- Name a colony after its RimSaves folder, else the player faction, else the world. Order colonies by their latest save, and saves newest first.
- **The player's name.** The player may rename a colony in RimArcho (`App.RenameColony`, `saves rename-colony`, 16 §2.11): the name is stored by the colony's key (`colony_name`, schema 33) and shown instead of the saves' name, which stays beside it (`Colony.GameName`) and still finds the colony. A colony grouped by world identity also takes a name set while its saves were grouped by file name (before their full read); such a file-name key belongs to at most one world colony, the one with the oldest manual save of that name, so a later world saved under the same name does not inherit it. `App.SettleColonyNames` (run by `saves index` without `--only`) settles against a listing taken after its read; it moves such a name onto the world key and removes names no colony answers to, in folders that were read and whose saves all have a full read (a missing drive keeps its names, and so does a folder with a save changed since its read, which groups by file name until it is read again). An empty name, or the saves' own, clears it under every key the colony answers to. Save files, the faction and the world keep their names.
- Saves in a pack's folder belong to that pack (§4.2 rule 1). Colonies whose latest save matches no list form a separate "matches no list" group.

### 4.2 Matching a plan

`app.ListSaves` evaluates these in order and stops at the first match:

1. Save is in a pack's save directory → that pack.
2. Save modification time falls after a launch record (09 §1) and before the next launch, and that launch used this user data directory (checked against its captured root when available, even after an environment is edited) → that launch's plan. A pack's launch does not match a save in a game folder (packs save in their own folder).
3. Save `<modIds>` (enabled set resolved under 07 §4.2) exactly matches a version (07 §3) of a plan (`TokenFingerprint`) → that plan; if several match, choose the most recently played.
4. No match → "matches no list".

Approximate matching for rule 3 (an enabled set differing by one or two entries) is undecided and not implemented, to avoid false matches. Rules 1 and 2 may match a save whose Mods differ from the list; §4.3 lists those differences.

### 4.3 Selecting a save

**Where it belongs**: game version and environment, the matched list or pack with the rule that matched it, the location (folder and file), and whose Saves folder that is (§4.8).

**Can it load?** compares the save with the matched list as it is now and lists only what needs attention. The Saves page checks it against what the Launch page applied instead, when that is another game, list or pack (16 §2.11; `App.SaveAgainstCurrent`, `SameTarget`); `saves show --current` and `saves fix --current` do so against the current game's current list, and the fix commands they print keep `--current`. Passed checks are not listed. `app.CheckSave` implements every row: the rows needing only the head (cut off, Mods, order, game version, no list) always, those needing the full read (malformed XML, classes, what a Mod owns) when the save's L1 facts are cached (§1.7; `CheckSave` never reads a save in full), and the last load's errors and warnings when a game log's report is stored (below). Without the full read, the rows and levels are those of the head alone.

| Item | Level | Consequence shown | Fix |
| --- | --- | --- | --- |
| Cut off: no `</savegame>` (L0) and not being written | Error | Can never load | Recycle; open the previous readable save |
| Malformed XML (L1). A file that only ends early is the cut-off row, not this one | Error | Where reading stopped (line and byte) | Recycle; open the previous readable save (complete and, when read, well formed) |
| A Mod in the save is not installed, whether the list lacks it or has it only as a missing entry (as after a fix, until Steam's download is scanned) | Error when it owns pawns, factions or classes in the save (L1, §4.6); otherwise, and without the full read, warning | What it owns (§4.6), its classes among them; broken references are not implemented | Subscribe and enable (needs `<modSteamIds>`; only Subscribe when the list has it already), else Make a list from this save |
| A Mod in the save is installed but not enabled | As above | As above | Enable in the list, positioned in save order |
| `Class="…"` with no type in the list's loaded Mods or the game (L1), and no Mod with a row of its own explaining it: a class whose Mod has a row is counted in that row (§4.6) | Error | The types, and the installed Mod defining them when one does (one row per such Mod, one for types no installed Mod defines) | None: such a Mod is not in the save's Mods |
| The last load of this save had errors or warnings (a game log phase whose save name matches, 06 §7) | Error if any errors, otherwise warning | Separate error and warning counts, kind count and first message | Open the game log at that cluster |
| A Mod in the list is not in the save | Information | Usually safe: it starts fresh in an existing save | Disable to match |
| Same Mods in a different order | Information | Usually safe; patch order differs | Use the save's order |
| Save made for another game version than the environment's | Warning | The game converts it on load | — |

- **The last load** (`app.lastLoadOf`, `internal/app/saveload.go`): Archo Monitor names the save it loads on the "save" phase mark (19 §4). Occurrences in that phase are counted separately as errors and warnings using `logSeverity` in `internal/app/logs.go`; informational clusters do not count. The environment's `Player.log`, then `Player-prev.log`, are read from their stored reports only (`log_cache`, 06 §5), never parsed here, so a log not analysed yet says nothing; each log's loads are kept per App while the log and its report are unchanged. Names match without `.rws` and case. A run that loaded several different saves counts its "save" phase across them, so it attributes nothing to any of them (no row; not concluded). A pack's save gets no row. A load whose run RimArcho launched (or wrote the list for) with another list than the one checked gives no row either: its errors come from that list (a save loaded with Mods missing is not broken with the list it was made with); the run's list is its `RunSource.Fingerprint` against `ListFingerprint` of the checked list, and a run of unknown source still counts. The row (`SaveIssue.Load`: errors, warnings, total kinds, when, and first message) is an error when any errors were logged, otherwise a warning; zero errors and warnings gives no row. Its first cluster prefers errors over warnings, then a cluster that began during the load, in order of first occurrence. CLI and MCP text includes both nonzero counts with singular or plural nouns, the kind count, log and time. The row has one fix: open the game log at that cluster.
- Compare packageIds case-insensitively. For Workshop entries written as `<package ID>_steam`, strip the suffix before comparing (07 §4.1).
- The verdict takes the highest level present: **loads with the list**, **loads, but content will be lost** (warnings) or **will not load cleanly** (errors). The quick fix (`SaveCheck.Quick`, `saves show`) is at most one action: **Fix and launch** applies every Mod fix as one reversible plan edit, starts any subscriptions, previews, then launches (09 §2); for a cut-off save it is Recycle; for a save matching no list it is Make a list from this save. The Saves page words and chooses its bar's action itself (16 §2.11): warnings read "content will be lost", what could not be checked is said there, its action while something could not be checked is to make a list from the save and analyse it, and it never offers Fix and launch.
- `saves show` prints each item with its consequence and the command that performs the fix (`fix: <label> (archo <command>)`), then the quick fix. With the full read, a missing Mod's consequence says what loading without it loses (§4.6), marked inferred where it is; without it, the consequence is generic: what it added to the save is lost on load.
- `saves fix --launch` does not launch a malformed save, as for a cut-off one.

**Mod list**: the save's Mods in load order, named from `<modNames>`; not-installed entries and reordered entries marked; the list's Mods missing from the save named after it (`saves show --mods`). **Changes against the list**: the save's Mods the list or pack it is checked against lacks (`SaveMod.InList` false) and the list's Mods the save lacks (`SaveCheck.ListOnly`, in the list's order), by packageId with case and the `_steam` suffix ignored, load order ignored; a save with no list or pack has none. Each Mod, the list's own included, carries the installed copy this environment resolves it to (`SaveMod.Library`), for a link to the Mod library.

**Play facts** (L1): day and date, storyteller and difficulty, real play time, biome, wealth, colonists (with deaths), animals, research progress and raids survived, each with the change since the colony's previous save. `saves show` (13 §4.2) shows them without biome (not read, §1.7) and with research as the current project and how many projects have progress above 0 (the game saves a key for every project; whether one is finished needs its cost from the Def, L2). Living pawns of the player's faction among the world pawns are counted apart as world pawns, not colonists: the game keeps the starting pawns the player did not choose there. The previous save is the next older save of the colony; the change is computed when its L1 read is cached and well formed and lists only what changed; with `--play` each colonist is marked `new` (absent from the previous save) or `died` (alive in it), matched by pawn ID. The GUI shows only the day and date, storyteller, difficulty and play time (16 §2.11, `view.SavePlayOf`), and the dead in Over time's legend; the numbers, colonists and events are in `saves show`.

### 4.4 Actions

The actions are implemented in `internal/app/saveactions.go` (`FixSave`, `RenameSave`, `CopySave`, `BackupSaves`, `TrashSaves`), with their commands and MCP tool (`internal/cmds/saves.go`, `manage_saves`; 13 §4.2, 14 §2) and the GUI page (16 §2.11).

| Action | What it does | Confirmation |
| --- | --- | --- |
| Fix | The Mod fixes of §4.3 as one reversible plan edit (07 §2.1): installed Mods enabled and Workshop Mods added as missing entries, each right after the nearest Mod before it in the save; optionally the list's Mods the save lacks disabled, and the shared Mods put in the save's order. The caller subscribes to the Workshop Mods; `--launch` launches afterwards | Not required; preview first (`--dry-run`) |
| Create a list from this save's Mods | Create a plan containing the imported save list (same as `loadout import --new`, §1.3); retain and mark uninstalled entries, as with ModsConfig.xml import | Not required; the new plan's first version has source `import` |
| Rename | Rename the file in place, inline as in 16 §2.17. Keep a RimSaves prefix. On every platform, compare the complete destination filename against all directory entries without case: refuse a match belonging to another file, even on a case-sensitive filesystem. Allow a case-only rename of the save itself, identified by filesystem metadata (`os.SameFile`). Never replace a destination that appears meanwhile (`MoveFileEx` without replacing on Windows; `RENAME_NOREPLACE` on Linux) | Not required |
| Copy | Copy to `<name> (copy).rws` (then `(copy 2)`…) in the same folder, never overwriting, keeping the modification time so the copy sorts beside the original | Not required |
| Copy to the current Game Config | Copy only the file into the current Game Config's Saves folder (§4.8), under its own name when free, else as Copy names it; refused for a save already in that folder | Not required |
| Back up | Copy to `<data directory>/backups/saves/<time>/`, preserving filenames. Allow multiple selections; one backup shares one time directory. Copy only; leave originals in place | Player confirmation required |
| Move to the recycle bin | Use the recycle bin rules in 02 §3: save files are one of its accepted categories. No permanent deletion | Player confirmation required |

- `<time>` is local time, `YYYYMMDD-HHMMSS` like database backups (17 §3): the preview's time, so the folder the preview names is the one written. The folder is made new; when a backup already has that name it gets `-2`, `-3`…. A second file of the same name (saves from different folders) goes into a subfolder named after its pack or game (numbered when that is taken).
- **The preview's token** (`SaveFilesResult.Token`): a hash of each previewed save's reference, size and modification time (and, for a backup, the preview's time). Confirming with it re-reads the files and refuses the whole action when any changed (`ErrPreviewChanged`: "the saves changed since the preview; preview again"), so an autosave rotated onto the same name since the preview is not recycled. The GUI always passes it; the CLI prints it in its confirm command (`--token`); MCP refuses trash with `confirm=true` and no token. A confirmation without a token (a person typing `--yes`) is not checked.
- Both actions requiring confirmation follow 16 §6: list affected files and sizes first; execute only after the player clicks "confirm". Archo can request but cannot confirm for the player (15 §2.3). MCP previews with `dry_run=true` and executes with `confirm=true` (14 §2).
- While the game runs (09 §4), refuse rename and recycle bin moves (the recycle preview too) and explain "the game is running". Backups and copies work normally (read-only source access).
- Backup and recycle previews list the requested saves and total bytes. Completed results list and total only successful files, with failures separately and a nonzero CLI/MCP exit status. If none succeeds, say nothing was copied or moved; never print a success headline for failed files (`SaveFilesResult`, `saveFilesDoc`).
- A colony with a cut-off save gets a cleanup hint offering to recycle it.
- `saves fix --launch` launches only when the save can load and none of its Mods is still a missing entry of the list: a Workshop Mod the fix just subscribed is downloaded by Steam in the background, so the player scans and runs the fix again. A Mod that cannot be downloaded (a missing entry with no Workshop ID, or a Workshop item installed under another packageId: `SaveFixResult.Stuck`, `app.StuckWhy`) or whose subscription failed for good (removed from the Workshop) is not waited for: the launch goes ahead and says which Mods it is without. A fix asked for Mods the list already loads (`--mod`, MCP `mods`) is already done: no edit, no error.

### 4.5 Timeline and events

From the L1 facts of a colony's saves and the game's own history records inside each save: wealth and colonists over in-game days, one point per save, with deaths marked; events such as deaths, raids repelled and marriages, newest first. Counting occurrences instead of only presence is optional (undecided).

In `saves show` (13 §4.2): the events are the letters of the game's archive (`Facts.Events`, the newest 200), shown newest first with their in-game day, label and LetterDef: 3 on one line by default, 6 with `--play`, all with `--play --all`; the change since the previous save is §4.3's.

In the GUI (16 §2.11): the lines are the selected save's own `Wealth_Total` and `FreeColonists` records, spread evenly from day 0 to its day; each read save of the colony is a point at its in-game day (from its summary), on the selected save's wealth line where that reaches, so a save of another branch of play is placed on this save's history; death letters are marked; the events are the newest letters (any letter: raids, joiners, deaths), not counted. Only the selected save's facts are loaded, never every save's. The records are read from the game's code and unverified on real saves.

### 4.6 What a Mod owns in a save

Implemented for can it load (`internal/app/saveowners.go`; tested on synthetic saves only, unverified on real saves). To state the consequence of a missing Mod, attribute the save's L1 facts (§1.7) to the Mods with a row of their own (not installed, or installed but not in the list). A Mod **owns** what the save needs and nothing the list loads provides: an object that a Mod the list loads, or the game, also provides is not lost.

- **Defs**: each `<def>` value by Def facts (05 §2; batched, `store.DefDefiners`), as in §1.5: non-abstract definitions; of several defining Mods with rows, the last in the save's order. A Def the game generates from another at startup has no Def file and resolves by the Def it is made from: prefixes `Corpse_`, `Meat_`, `Blueprint_`, `Blueprint_Install_`, `Frame_`, `Techprint_`, `Psytrainer_`, `Neurotrainer_`, `Make_`, `Administer_`, suffixes `_Rough`, `_RoughHewn`, `_Smooth` (`DefGenerator`, 1.6). Counted per Def type with the number of `<def>` elements naming them, about one per object ("3 things, 1 hediff").
- **Pawns** (the player's humanlike pawns, alive and dead) by their race Def; **factions** by their FactionDef, named by the faction's name. The facts count animals, mechanoids and prisoners without their races, so those are not attributed.
- **Classes**: each `Class="…"` value; a generic one by its definition and each argument. The game's own types by `store.GameClassOwners` (05 §5.3); `System.` and `UnityEngine.` types count as present (not indexed; the runtime has them). Mod types by `store.TypeDefiners` in every form the game tries (17 §6 item 20): the value, and the value prefixed by each of `store.SaveNamespaces`; a simple name found nowhere is looked up as a nested type's inner name (`store.NestedTypeDefiners`). World and game components are among the classes: the facts do not say where a `Class` appeared, so they are not reported apart (not implemented; needs a scanner field).
- **Which facts exist** (05 §3), and so what is certain: Def facts for every loadable installed Mod once an analysis of the game version has run; type facts only for Mods enabled when an analysis ran (a disabled Mod keeps them while its content is unchanged); a Mod removed from disk keeps its facts while its tombstone lasts (04 §2.5; inferred from the code and seen in synthetic tests: scan and analysis keep a tombstone's version view). A list entry the game cannot see (a Mod removed while still in the list) does not count as loaded, so what it owns is lost. Facts of any copy of the packageId count, in any environment of that game version (`store.PackageFactViews`). A Mod without facts of a kind (never installed, or disabled without type facts) is given, **inferred**, what nothing installed provides of that kind: the Defs no installed Mod defines and the classes no installed assembly nor the game defines. When several Mods lack facts, each is given all of it and says it may belong to the others. A Def no installed Mod defines may also be one a Mod update removed (§1.5), so a Mod whose inferred share is only Defs stays a warning.
- **Unchecked**: when a Mod of the save that the list loads has no Def view, the Defs, pawns and factions are not checked; when one has no DLL view, or the game's types are not indexed (`GameTypesIndexed`), a class found nowhere is not concluded on (no class-missing row, nothing inferred), since it may be in the unindexed assembly. A Mod the list loads and the save does not have stored nothing in it, so its facts are not needed (a pack's copies all are: its Mods resolve by packageId). The row says what was not checked; with neither checked it is as without the full read. So analysing a list made from the save (16 §2.11) is enough to check it.
- **Together**: the Mods that are not installed are one row on the page, so `SaveCheck.GoneOwns` is what they own together: what each owns by its own facts, plus what nothing installed provides once rather than once per Mod (`Among` counts only Mods outside the group). `SaveOwned.NoFacts` marks a Mod the database has nothing on (no Def view and no DLL view), which the page says.
- **Names**: each `OwnedDefs` keeps the names of its Defs, most used first, at most 100 (`OwnedDefNames`), for the page's View.
- Counts per kind ("3 colonists of race Kurin_Race (1 dead), 1 faction (Kurin Clan), 1 class (ModA.CompBroken), 3 things, 1 hediff") are `app.SaveOwned` on the row's `SaveIssue.Owns` (JSON `owns`); the text is `consequence()` in `internal/cmds/saves.go` (CLI and MCP) and `issueView()` / `goneView()` in `internal/view/saves.go` (the page).
- Speed: `saves list` checks every save, so lookups are batched and kept per App while the database is unchanged (`saveOwnerMemo`); Defs are looked up only for saves with a Mod row. Measured in 18.
- Whether a Mod can be removed mid-playthrough is §4.7. `saves list --mod` lists the saves whose Mod list has the Mod but does not say what removing it loses.

### 4.7 What the save needs of each Mod

Implemented (`internal/app/saveneeds.go`, `SaveContentResult.Needs`; tested on synthetic saves only, unverified on real saves). §4.6 says what the list the save is checked against costs it; this says, whatever that list is, what removing each one of the save's Mods would cost, so a player knows which Mods must stay before changing a list. Every Mod of the save gets one verdict:

| Verdict | Criterion | Meaning |
| --- | --- | --- |
| `breaks` | It provides the race of a player's humanlike pawn (alive or dead), a FactionDef the save names, a `Class="…"` value, or a map's current or last weather | The save may not load without it. These are what make a missing Mod's row an error in §4.3, plus the weather |
| `loses` | Otherwise, it provides a Def in a category of things in the world: races, animals, mechanoids, other creatures, buildings, weapons, apparel, plants, items, terrain, biomes, or culture (§2.2), or a Def of it is in a map's or the world's grids (§1.4: rock and ore are in no such category) | The save loads; those things are gone. A precept or a material whose Def is gone leaves errors behind rather than a clean reset, so culture counts here |
| `care` | Otherwise, it provides any other Def the save references (research, recipes, appearance, traits, health conditions, quests…) | They reset and the colony goes on |
| `safe` | It is installed and analysed, nothing of it is in the save and no other Mod of the save requires it | Safe as far as the save can tell: a save cannot show what a Mod's code or patches do without leaving a Def or class |
| `unchecked` | It is not installed, has no content facts for the game version, has no type facts and nothing known breaks the save, or would be `care` or `safe`, defines terrain, biomes, rock or chunks, and the save has some of that kind nobody could name | Nothing can be said; analysing a list made from the save (16 §2.11) checks the first three |

- **Sources.** Defs, categories and the providing Mod are the selected-save content attribution (§2, §1.5: of several of the save's Mods defining a Def, the last in the save's order; a Mod's own Def class counts as a subclass of the type a reference names, as in §1.5, `defSubtype`). Pawns are `Facts.Colonists` by their race's ThingDef; the player's living world pawns (unchosen starting pawns) count as world pawns (`SaveNeed.World`), not colonists, and still make the race's Mod `breaks`; factions are `Facts.Factions` by FactionDef, named by the faction's name; weather is the `WeatherDef` references (§1.4).
- **Classes.** A `Class="…"` value belongs to a Mod when, among the save's Mods, only that Mod's assemblies define a part of it (lookups as in §4.6: `store.TypeDefiners`, `SaveNamespaces`, nested types); a class the game, the runtime or another of the save's Mods also defines is not lost with it. A Mod without type facts (05 §3) may hold classes nobody can see, so unless something else already makes it `breaks` it is `unchecked` ("its code is not analysed").
- **Terrain, biomes and rock.** They are read from the save's grids (§1.4), so a Mod whose terrain is on a map, whose biome is on a world tile, or whose rock, ore or chunks are on a map is `loses`: on loading without it the game puts soil where the terrain was (with an error per kind), the layer's default biome on the tile, and nothing where the rock was (with an error per kind), so the map keeps working but is not what it was. When the grids hold a hash of one kind that no Def of the save's Mods has (`GridUnnamed`), a copy it may belong to (`gridResult.may`) that would otherwise be `care` or `safe` is `unchecked` ("the save has terrain, biomes or rock that could not be named"): for terrain and biomes a copy with TerrainDef, TerrainTemplateDef or BiomeDef files (a Def under another name, or one a Patch added); for compressed things a copy with a compressible Def whose own hash is at most 16 places before it. Natural rock alone does not make a copy one for terrain: its stone floors are named from the rock.
- **Requirements.** A Mod that another of the save's Mods requires (About.xml `modDependencies` for the save's game version, 08) takes the heaviest verdict among the Mods requiring it, through chains of requirements: they do not run without it, so what they hold in the save goes with it (Harmony, required by a race Mod with a colonist, is `breaks`). `NeededBy` names the Mods requiring it directly. A requirement that several of the save's Mods can meet (alternatives) binds none of them. A Mod required only by `safe` Mods is `care`, since removing it alone leaves them broken; one that is otherwise `safe` and required by an `unchecked` Mod is `unchecked` ("a Mod that needs it could not be checked"). Only declared requirements count: a Mod that uses another's code without declaring it is not seen.
- **The game.** Core and the DLC (`ludeon.rimworld…`) get a verdict like any Mod and are marked `Game`: the interfaces name them apart and leave them out of the groups and counts.
- **Limits.** As §2.3 and §3.2: a reference means present, not how much. Animals, mechanoids and prisoners are not attributed as pawns (§4.6), so a Mod whose only pawns are those is `loses`. A Def that an earlier Mod of the save also defines is counted for the last one, although removing that one would fall back to the earlier definition. Fixes the game or other Mods offer for a removed Mod are not considered.
- Interfaces: the Saves page's right card (16 §2.11), `saves show --content` and `get_save content=true`, and for one Mod `saves show --mod <ref>` and `get_save mod=<ref>` (13 §4.2).

### 4.8 Saves across Game Configs

Implemented (`internal/app/savechoice.go`; tested on synthetic saves only). A Game Config (03 §6) is one option the player switches, so what RimArcho shows of the saves does not depend on which one is current: switching away and back never makes saves disappear. A game started with a Game Config lists only its own config's Saves folder (its user data, 03 §3.1), so that is what the interfaces say about a save in another folder.

- **Ownership is by folder** (§1.2): a save belongs to the Saves folder it is in, and so to the Game Configs using that folder (several when they share one, as two installations on the game's default folder do: shown "A, C", never one of them picked) or to a pack. The list a save matched (§4.2) is a separate fact; it does not decide where the save belongs.
- **Every save is listed** (`ListSaves`, 16 §2.11), whichever Game Config is current. With saves in two or more folders, each colony says whose folder it is in (`SaveColonyRow.Owner`, `view.FolderOwner`; `saves list` adds `[saves of …]`) and the Saves page can filter by it (default: all). Colonies never span folders (§4.1 keys include the folder), so a colony copied into another config's folder is a second colony: the two are separate saves that go on separately.
- **Can it load? still compares with what the Launch page applied** (§4.3): it answers "if I launch now, can I load this". When the save is in a folder that the game started with that config (or the applied pack) does not list (`Save.SeenBy`), the check says so first ("in the Saves folder of B: the game started with A does not list it"), then shows the comparison, which answers "would it load if it were there". Its actions are Switch and launch (the save's own Game Config and list, when it matched one) and **Copy to the saves of A** (`App.CopySaveTo`, `saves copy --to-current`, `manage_saves action=copy_to_current`): only the file is copied into the current config's Saves folder, under its own name when free, else `<name> (copy)` and on as Copy does (§4.4); no Mod, list or setting moves. Nothing moves saves on its own, and no save is ever launched with a config whose game would not list it.
- **What this save needs** (§4.7) does not depend on any config: the same for a save whichever is current.
- **Steam Cloud** syncs the game's default user data folder only: a folder that only own or custom saves configs use, or a pack's, says "Steam Cloud does not sync this folder" (`SaveDetail.NoCloud`; `saves show` "saves of").
- **Which save Can it run checks** (16 §2.4): by default the save this Game Config's newest run loaded (`LastLoadedSave`, which already looks only at this config's runs and folders). The player may choose instead **this save** (that file) or **this colony** (its newest save, whichever it is when checked) on the Saves page, with `saves choose <save> [--colony]` or `manage_saves action=choose|choose_colony`; `saves choose --clear` / `unchoose` goes back to the last loaded. The choice is kept per Game Config (`meta` key `saves.check.<config ID>`: kind, file or colony key, folder and name), so each config keeps its own after switching. It may name a save in another config's folder; Can it run then says the game started with this config does not list it. A chosen colony keeps its file-name keys (`SaveChoice.Also`); Can it run takes the colony with the newest save among those answering to the key, so a colony chosen before its full read is found as its world's colony (the choice moves to the world key), and a world colony saved again is found before the new saves are read. A chosen file or colony that is gone says so ("not in its Saves folder any more") instead of falling back silently. Only the chosen file, or the chosen colony's folder, is read; the Can it run cache key includes every save file's size and time in that folder (`SaveChoiceStamp`), so a save written over in place is seen. Removing a Game Config removes its choice.
- **Play review** (§3.4) covers every folder's colonies, as the Saves page does.

## 5. Playthrough themes and content conflicts (1.2)

Some Mods have no technical conflict but undermine one another in play. For example, Milira and Deadman's Switch do not technically conflict, but their power levels differ so much that the latter becomes pointless when played together.

- In 1.2, Archo provides suggestions first. In the plan editor (16 §2.5) or playthrough review, the player clicks "Ask Archo about this list's theme". Archo reads the plan's Mod list, content catalog statistics (§2) and unused content (§3), identifies combinations that may undermine one another and explains why. Label conclusions "inferred".
- Send Mod names, descriptions and statistics with consent under 15 §3.2.
- Keep this out of "can it run" and block no actions: these are gameplay suggestions, not errors.
- Turn accumulated community data into rules later (community data channel in 08 §2); rule form is undecided.

## 6. Worth playing (long term)

Two pages under "worth playing" in the sidebar: play goals and shared packs (16 §2.12). This records direction only; **details will be decided later**.

### 6.1 Player-shared packs

- Share a **RimArcho list** (the list exported from a pack, 07 §7.3): a manifest only (each Mod's lineage, semantic hash and source Workshop ID, 10 §1), without Mod files.
- Download every Mod from Steam Workshop; the sharing site hosts no Mod files.
- On import, match local Instances by packageId and hash (07 §7.1, 10 §4), and list Workshop links for missing entries.
- The sharing site is a separate project outside the RimArcho repository. RimArcho only exports RimArcho lists, imports them and browses them on the "shared packs" page (interface undecided).

### 6.2 Recommendations that understand the player

- Input is the answer to question two: unused content (§3), colonies the player has played and frequently used categories.
- Archo uses this to recommend content and play goals ("worth playing › play goals"). This is "from being directed to understanding the player" in 00 §2.
- The player profile **stays local**; the player can view and clear it. Obtain consent under 15 §3.2 before sending it to the model.
- Recommendation data requirements, profile structure and relationship with the sharing site will be decided later.

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| SV1 | `loadout import` a save (`TestLoadoutExchange`) | Stop after `<meta>`; import 4 entries, 1 uninstalled with a name; no ModsConfig.xml writes; `undo` restores | Pass |
| SV2 | Read all saves to extract Defs | Every save's size and modification time remain unchanged | Untested (the full read, §1.7 L1, reads every save; no test checks the files afterwards) |
| SV3 | Second analysis with no new saves | Use the cache without rereading saves | Pass for the full read (synthetic: `TestSavesFullRead`) and for the review (synthetic: `TestSaveReviewCacheInvalidation`: kept per-save results are reused, and a changed file, parser version, Mod content or a removed save invalidates them) |
| SV4 | A Mod with only Patches and assemblies | Goes into "functional, excluded", not "never appeared" | Pass for a Mod with no counted content (synthetic: `TestSaveReview`: a Mod with only a mechanics Def); one with only Patches or assemblies untested |
| SV5 | A Mod "appears" only in saves without it enabled | Does not count as having appeared | Pass (synthetic: `TestSaveReview`: a colony without the Mod is not counted for it) |
| SV6 | 30 saves from one colony | Count as 1 colony | Pass (synthetic: `TestSaveReview`: two saves of one colony count as one colony and two saves) |
| SV7 | Matching a save to a plan | Follow §4.2 order; unmatched saves enter "matches no list" | Partial: launch and same-Mods matches pass (synthetic: `TestSavesListGroupAndMatch`), pack saves in golden `saves`; unmatched saves untested |
| SV8 | Back up a save without player confirmation | No files written; after confirmation, appears in `<data directory>/backups/saves/<time>/`, original unchanged | Pass (synthetic: `TestSaveFileActions`, `TestManageSavesActions`) |
| SV9 | Move to the recycle bin | Confirm first; enter system recycle bin; refuse while the game runs | Partial: confirmation passes (synthetic: `TestSaveFileActions`, `TestManageSavesActions`, golden `saves`); the real recycle bin and a running game are untested |
| SV10 | Fix a save whose list lacks an installed Mod and a Workshop Mod | One reversible edit: the installed Mod enabled, the Workshop Mod added as a missing entry, each in save order. Until the download is scanned the save still reads "content will be lost", and fixing again only subscribes again (no edit) | Pass (synthetic: `TestFixSave`, golden `saves`) |
| SV11 | Open Saves with no L1 cache | Left card, middle header and Mod list appear from L0 at once; the background task reads the selected save first | Pass (synthetic: `gui.TestSavesFullReadPage`; guidev on a synthetic home of 163 saves, 660 MB: listing 53 ms, the selected 29 MB save's play facts after about 110 ms, the run done in 0.34 s from the OS file cache); real saves untested |
| SV12 | A save written during a crash (no `</savegame>`) | Cut off, error, Recycle offered; a file still being written gets no verdict | Pass (synthetic: `TestCheckSave`; being written, not read and no verdict: golden `saves_play`) |
| SV13 | RimSaves name `Ashfall#§#Landing.rws` | Grouped under colony `Ashfall`, shown as `Landing` | Pass (synthetic: `TestSavesListGroupAndMatch`) |
| SV14 | A save whose Mod with pawns is not installed | Error naming what is lost; Fix subscribes and enables it as one reversible edit | Pass (synthetic: `TestCheckSaveOwnership`, a removed and two never-installed Mods; the fix as SV10); a real save untested (release VM row) |
| SV15 | `get_save` on MCP | Same text as `saves show`; each fix names its command or tool call | Pass (synthetic: `TestSaveToolsCLIParity` with `play`, golden `saves`) |
| SV16 | `saves show` on a save not read in full, then `saves index` and `saves list` | The save is read inline and joins its world's colony; play facts as §4.3; the previous save is compared only once it is read; `saves list` shows `day N`, `meta only` with a note naming `saves index`, `malformed`; every call within 40 lines without `--all` | Pass (synthetic: golden `saves_play`, `TestSavePlayText`); real saves untested |
| SV17 | Typed/untyped names and same-name overrides | Every untyped type candidate counts; typed references restrict type; last provider per type follows save order; abstracts excluded | Pass (synthetic: `TestSaveContentCandidates`) |
| SV18 | A/B/C parents, empty/reset lists, predicates and custom plants | Child-relative Name lookup; correct categories and inferred/unknown markers | Pass (synthetic: `TestSaveContentPredicatesAndInheritance`) |
| SV19 | Wrappers, generated Defs, cached save file unavailable | Wrappers collapse; generated identities keep their categories with base ownership; L2 reads no save | Pass (synthetic: `TestSaveContentGeneratedAndCache`) |
| SV20 | Schema 34 facts, per-Mod readiness, MayRequire | Def/DLL views survive schema 35; unchecked until new fields; save packages determine conditions | Pass (synthetic: `TestUpgradeDefContentKeepsFacts`, `TestSaveContentUnchecked`, predicate test) |
| SV21 | GUI JSON and CLI/MCP content with many Mods | Exact contract, bounded names, totals without names, limits, ≤40 text lines without all | Pass (synthetic: `TestSaveContentContract`, `TestSaveContentAPI`, golden `saves_content`, `TestSaveContentTextBudget`, `TestSaveToolsCLIParity`); real saves untested |
