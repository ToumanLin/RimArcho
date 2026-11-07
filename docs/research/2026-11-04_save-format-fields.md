# Save format: the fields of a full read (L1), 2026-11-04

Status: **from source, unverified on real saves.** Every path below was read from the game's `ExposeData` methods; none has been checked against a real `.rws` yet (no real saves were read on the host). A VM measurement against real saves is pending; until then treat each path as the expected shape, not a measured one. Implementation: `internal/savefile` (`Scan`, `Facts`, `ParserVersion`); tier definition: 12 §1.7 L1.

Source: RimWorld 1.6.4871 rev590, `RimWorldWin64_Data/Managed/Assembly-CSharp.dll`, decompiled with `ilspycmd` 10.1 (`-p`, references the `Managed` folder) into a scratch directory outside the repository and deleted afterwards. `archo source` was not used: it skips the game's own assembly (see 2026-11-01_decompile-storage.md).

## How Scribe writes a save

These rules explain the shape of every path below.

| Rule | Source | Consequence for reading |
| --- | --- | --- |
| The file is `<savegame>` with `<meta>` (12 §1.3) then `<game>` | `GameDataSaveLoader.SaveGame` (`SafeSaver.Save(…, "savegame", …)`, `Scribe_Deep.Look(ref target, "game")`) | Absolute paths start `savegame/game/` |
| A value equal to its default is **not written** | `Scribe_Values.Look` (`!forceSave && value.Equals(defaultValue)` → return) | A missing element means the default: `greatestPopulation` 3, `Faction.loadID` 0, `Ideo.id` 0, `Quest.id` 0, `Map.uniqueID` −1, `WorldObject.ID` −1, `healthState` Mobile, `guestStatus` Guest |
| A null def is not written either | `Scribe_Defs.Look` (writes `"null"` with default `"null"`) | `<def>null</def>` does not occur; the scanner still ignores it |
| A null deep object is `<label IsNull="True" />` | `Scribe_Deep.Look` | Animals have `<story IsNull="True" />` |
| A deep object of another type than declared gets `Class="…"` | `Scribe_Deep.Look`, `GenTypes.GetTypeNameWithoutIgnoredNamespaces` | Types in `RimWorld`, `Verse`, `Verse.AI`, `Verse.AI.Group`, `Verse.Sound`, `Verse.Grammar`, `RimWorld.Planet`, `RimWorld.BaseGen`, `RimWorld.QuestGen`, `RimWorld.SketchGen`, `LudeonTK`, `System` are written without namespace; others (all Mod types) with their full name; generic types as `Type.ToString()` |
| A reference is the target's `GetUniqueLoadID()` as text, or `null` | `Scribe_References.Look`; a destroyed thing or world object is written as `null` unless `saveDestroyedThings` | References are plain leaf text such as `Thing_Human123`, `Faction_3` |
| A list is `<li>` children; a dictionary is `<keys><li>…</li></keys><values><li>…</li></values>` | `Scribe_Collections.Look` | Research progress pairs keys and values by position |
| A byte array is base64 with a line break every 100 characters (and one before and after), deflated when that is smaller (`<label>Deflate`) | `DataExposeUtility.LookByteArray`, `AddLineBreaksToLongString`, `CompressUtility.Compress` (Ionic `DeflateStream`: raw deflate, RFC 1951) | Strip whitespace, base64-decode, inflate with `compress/flate` |

## Paths

All under `savegame/game/` unless they start with `…/` (relative to the element named in the row before) or are marked "any".

| Facts field | Path | Source method | Notes |
| --- | --- | --- | --- |
| `RealPlayTime` | `info/realPlayTimeInteracting` | `GameInfo.ExposeData` | Seconds (float) |
| `Permadeath` | `info/permadeathMode` | `GameInfo.ExposeData` | `True` or absent |
| `Scenario` | `scenario/name` | `Scenario.ExposeData` | |
| player faction def | `scenario/playerFaction/factionDef` | `Scenario.ExposeData` → `ScenPart_PlayerFaction.ExposeData` | The only place naming the player's faction: `FactionDef.isPlayer` is in the Def, not the save |
| `TicksGame` | `tickManager/ticksGame` | `TickManager.ExposeData` | `Day()` = ticksGame / 60000 |
| `GameStartAbsTick` | `tickManager/gameStartAbsTick` | `TickManager.ExposeData` | `TicksAbs()` = start + ticksGame; `Date()` per `GenDate.Year`/`DayOfYear` (year 5500 + abs / 3 600 000; quadrum = day of year / 15, Aprimay first) |
| `Stats.*` | `storyWatcher/statsRecord/{numRaidsEnemy, numThreatsQueued, colonistsKilled, colonistsLaunched, greatestPopulation}` | `StoryWatcher.ExposeData` → `StatsRecord.ExposeData` | `numThreatsQueued` holds the field `numThreatBigs`. `greatestPopulation` defaults to 3 |
| `Research.Current` | `researchManager/currentProj` | `ResearchManager.ExposeData` | |
| `Research.Progress` | `researchManager/progress/{keys,values}/li` | `ResearchManager.ExposeData` | Dictionary ResearchProjectDef → float. Finished means progress ≥ the Def's cost, so L1 cannot say which are finished. Not extracted: `techprints`, `knowledge` (Anomaly), `currentKnowledgeProjects` |
| `Storyteller`, `Difficulty` | `storyteller/def`, `storyteller/difficulty` | `Storyteller.ExposeData` | A custom difficulty adds `customDifficulty` (not read) |
| `History` | `history/autoRecorderGroups/li/def` (group), `…/recorders/li/def`, `…/recorders/li/records` or `recordsDeflate` | `History.ExposeData` → `HistoryAutoRecorderGroup.ExposeData` → `HistoryAutoRecorder.ExposeData` (`RecordsToBytes`) | Little-endian float32 series. One value every `recordTicksFrequency` of the HistoryAutoRecorderDef (Core `HistoryAutoRecorders.xml`: 30000 for Wealth_Total, Wealth_Items, Wealth_Buildings, Wealth_Pawns, FreeColonists, Prisoners, ColonistMood, ThreatPoints; 60000 for Adaptation, PopAdaptation, PopIntent), first value when the recorder is created (`HistoryAutoRecorder.Tick`). Wealth_Total sums the player's home maps (`HistoryAutoRecorderWorker_WealthTotal`). Whole series kept for `KeptSeries` only |
| `Events` | `history/archive/archivables/li` with `Class`, `…/def`, `…/label`, `…/arrivalTick` | `Archive.ExposeData` → `Letter.ExposeData` | Archivables mix letters, `Message` (`text`, `startingTick`) and `ArchivedDialog` (`title`, `createdTick`); only letters have `label`, so only those are kept. `arrivalTick` is TicksGame at arrival (`LetterStack.ReceiveLetter`). The archive keeps at most 200 unpinned archivables (`Archive.MaxNonPinnedArchivables`), so early events of a long game are gone. The label is in the language played at the time |
| `World` | `world/info/{name, seedString, persistentRandomValue, planetCoverage}` | `World.ExposeData` → `WorldInfo.ExposeData` | |
| `Factions` | `world/factionManager/allFactions/li/{def, name, loadID}` | `World.ExposeComponents` → `FactionManager.ExposeData` → `Faction.ExposeData` | `Player` = the faction whose def is the scenario's player faction def; if none, the first whose def contains `Player` (heuristic) |
| `Maps` | `maps/li/uniqueID`, `…/mapInfo/parent`, `…/mapInfo/isPocketMap` | `Game.ExposeData` → `Map.ExposeData` → `MapInfo.ExposeData` | `parent` is `WorldObject_N`; its name and faction come from the world object (next row). `Home` = the parent's faction is the player's |
| map parents | `world/worldObjects/worldObjects/li` and `world/pocketMaps/li`: `…/ID`, `…/faction`, `…/nameInt` | `WorldObjectsHolder.ExposeData` → `WorldObject.ExposeData`, `Settlement.ExposeData` | `Class="Caravan"` marks a caravan (`Caravan.ExposeData`: pawns in `pawns/innerList/li`) |
| pawns | any element with direct `<def>`, `<id>` and then `<kindDef>` | `Thing.ExposeData` (def, id, faction) → `Pawn.ExposeData` (kindDef first after the base) | Found on maps (`maps/li/things/thing`, inside corpses `innerContainer/innerList/li`, carried pawns `carryTracker/innerContainer/innerList/li`), in `world/worldPawns/{pawnsAlive, pawnsMothballed, pawnsDead}/li`, in caravans and elsewhere. Only `MechClusterSketch` and `PawnKindCount` also write `kindDef`, and they have no `def` + `id` |
| `Pawn.Race`, `.ID`, faction | the pawn's `def`, `id`, `faction` | `Thing.ExposeData` | `faction` is written as a value (`Faction_N` or `null`), resolved by search, not through cross-references |
| `Pawn.Kind` | `…/kindDef` | `Pawn.ExposeData` | |
| `Pawn.First/Nick/Last`, `Pawn.Name` | `…/name` with `Class="NameTriple"`: `first`, `nick`, `last`; `Class="NameSingle"`: `name` | `NameTriple.ExposeData`, `NameSingle.ExposeData` | `Display()` follows `ToStringShort` (the nick) |
| humanlike | `…/story` present and not `IsNull` | `PawnComponentsUtility.CreateInitialComponents` (story only for `RaceProps.Humanlike`) | `Colonists` = humanlike pawns of the player faction |
| animal | `…/training` present and not `IsNull` | `PawnComponentsUtility.AddAndRemoveDynamicComponents` (training when intelligence ≤ Animal, has a faction, not a mechanoid) | `Animals`; other living player pawns count as `Mechs` |
| `Pawn.Dead` | `…/healthTracker/healthState` = `Dead` | `Pawn_HealthTracker.ExposeData` | Default Mobile is omitted |
| `Pawn.Slave`, `Prisoners` | `…/guest/guestStatus` (`Slave`, `Prisoner`), `…/guest/hostFaction` | `Pawn_GuestTracker.ExposeData` | A prisoner keeps its own faction; `hostFaction` is the player's |
| `Defs` | any `<def>` | `Scribe_Defs.Look(ref def, "def")` in `Thing`, `Hediff`, `Gene`, `Tale`, `Letter`, `WorldObject`, `ScenPart`, … | Counts every occurrence (12 §1.4). See "Not extracted" for Defs written under other names |
| `Classes` | any `Class="…"` | `Scribe_Deep.Look` | |
| `Refs` | see below | | |

## References (`Refs`)

`GetUniqueLoadID` has 51 implementations in the game alone (`Thing_`, `Faction_`, `Hediff_`, `Bill_<recipe>_`, `Verb_`, `Zone_`, `Area_<id>_Home`, `<GameCondition class>_`, `<policy key>_<label>_`, …) and Mods add their own, so the scanner checks only the kinds whose defining object it can find exactly:

| Kind | Defined by | Default when absent |
| --- | --- | --- |
| `Thing_<ThingID>` | an element with a direct `<def>` followed by `<id>` (`Thing.ExposeData`; ThingID = defName + number) | — |
| `Faction_<loadID>` | `world/factionManager/allFactions/li/loadID` | 0 |
| `Map_<uniqueID>` | `maps/li/uniqueID` | −1 |
| `WorldObject_<ID>` | `world/worldObjects/worldObjects/li/ID`, `world/pocketMaps/li/ID` | −1 |
| `Ideo_<id>` | `world/ideoManager/ideos/li/id` | 0 |
| `Quest_<id>` | `questManager/quests/li/id` | 0 |

- A reference is any leaf text starting with a known kind and `_`, except inside `<def>` and inside the ID fields `id`, `loadID`, `ID`, `uniqueID`, `uniqueId`: a verb's own `loadID` is built from its owner (`Verb.CalculateUniqueLoadID`, e.g. `Thing_Human10_0_Smash`) and would otherwise read as a reference to a missing thing.
- References of the other known kinds are counted in `Unchecked`; unknown kinds (Mods) are not counted at all.
- Things the map compressor stores in the compressed grid instead of `<things>` (`CompressibilityDeciderUtility.IsSaveCompressible`: rocks and chunks at full health, unreferenced) have no `<id>`. The compressor skips things that the `CompressibilityDecider` sees referenced, but it does not see references from Mod code, so such a reference would read as dangling.
- A truncated or malformed save leaves `Refs` empty: forward references would all look dangling.

## Not extracted (open questions for the VM measurement)

1. **Biome.** A map's biome comes from its tile in the world grid (`world/grid`, compressed per-tile arrays such as `tileBiomeDeflate`, indexed by def short hash). Decoding needs the grid layout and the short-hash table, so it is skipped. A cheaper source may exist on real saves; check.
2. **Defs not written as `<def>`.** Terrain (`topGridDeflate` and friends) and compressed things are short hashes, so floors and rocks of a Mod never appear in `Defs`. Pawn fields also hold Defs under their own names: `kindDef`, `stuff`, `childhood`/`adulthood` (backstories), `hairDef`, `beardDef`, `bodyType`, `headType`, `xenotype`, research keys. Which to add is the open question of 12 §1.4.
3. **Colony identity.** Whether world name + seed + persistent random value + player faction name is enough (12 §4.1) needs real saves: a world regenerated with the same seed shares the seed but not `persistentRandomValue`.
4. **Player faction of Mod scenarios.** A scenario with no `playerFaction` part, or a Mod that replaces `ScenPart_PlayerFaction`, falls back to the `Player` name heuristic.
5. **Colonist semantics.** `Colonists` is "humanlike, player faction": it includes slaves (flagged), babies and the dead, and excludes colonists whose faction changed (kidnapped). The game's `IsColonist` adds slave and quest-lodger rules; compare counts on real saves with the game's colonist bar.
6. **Element density and timing.** The synthetic benchmark save has about 0.9 million elements in 27 MB. Real saves may be denser; measure time and peak memory on the 202 saves of 18.
7. **Older saves.** A 1.5 save loaded and resaved in 1.6 has 1.6 shapes; an unconverted 1.5 save may lack newer elements (`BackCompatibility` fills them on load). Facts then hold the defaults.

## Performance

`BenchmarkScan27MB` (synthetic 27.0 MB save, 1 419 distinct defs, 58 000 defined things, Ryzen 9 7950X, file in the OS cache): **0.084 s per scan, 323 MB/s, 3.9 MB peak heap growth, 7.3 MB allocated per scan.** For comparison, `encoding/xml` on the same bytes in memory takes 0.29 s (`RawToken`) and 0.33 s (`Token`) just to tokenize, before any extraction; the scanner therefore tokenizes bytes itself (`scan_lex.go`) and keeps only the text of the element opened last, up to 512 bytes (64 MB for the history series), so compressed grids are never buffered.
