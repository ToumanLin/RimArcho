# 10 · Unplayed and worth playing

> Status: question two, "what haven't I played?", is a design specification for 1.1; §5 is for 1.2; §6 is a long-term direction. Implemented: reading the Mod list at the start of a save (§1.3), the save head (§1.7 L0), the full read's core (§1.7 L1), grouping and matching (§4.1, §4.2), can it load except the last load's errors (§4.3), what a Mod owns in a save (§4.6), the actions (§4.4) and the save commands and tools (§1.6). Everything else is unimplemented. The Saves page, its read tiers and its tools were decided on 2026-11-04 (§1.6, §1.7, §4; 06 §2.11).
> Basis: 00 (§2, §4), 00a, 03, 04, 06, 08, 09.

Question two asks: **which installed Mods and content have gone unused across multiple playthroughs?** Read all saves without modifying them and map the Defs they reference back to their providing Mods. This analysis is deterministic and needs no AI; Archo participates only in interpreting results and making suggestions (09 §4).

The answer to question two feeds question three: first establish what has gone unplayed, then choose what is worth playing.

## 1. Save reading

### 1.1 Read-only

- Save analysis is **read-only**: open saves for reading only, never write, rename or move them. Rename, copy, backup and recycle bin moves are separate save-management actions started by the player (§4.4).
- Results stay local and are not uploaded. Only statistics are sent to the model, after consent (09 §3.2).

### 1.2 Save locations

| Location | Belongs to |
| --- | --- |
| `<user data directory>/Saves/*.rws` (shared, own or custom user data directory: `app.Saves`, 02 §2.1) | Environments using this user data directory, and plans launched in those environments. When environments share the game's default user data directory (02 §2.4), deduplicate by real path and read only once |
| `<data directory>/packs/<pack name>/userdata/Saves/*.rws` | This pack (packs use their own `-savedatafolder`, 08 §5.3) |

Read autosaves (`Autosave-*.rws`) like manual saves; label them "autosave" in the UI.

### 1.3 Format and `<meta>` (implemented)

Saves are XML with a `<savegame>` root, starting with `<meta>`, followed by `<game>`:

```xml
<savegame><meta><gameVersion>1.6.4633 rev1261</gameVersion>
<modIds><li>ludeon.rimworld</li><li>tester.gamma</li>…</modIds>
<modSteamIds><li>0</li><li>0</li><li>3000000003</li>…</modSteamIds>
<modNames><li>Core</li><li>Gamma</li>…</modNames></meta>
<game>…</game></savegame>
```

- `<meta><modIds>` contains the packageIds enabled when saving, in load order; `<modNames>` corresponds one-to-one.
- Reading rules (`readSaveMeta` in `internal/app/exchange.go`): stream parsing, non-strict mode, ignore the declared character set. Stop at `</meta>` without reading the tens of MB in `<game>`. Lowercase all packageIds, record `gameVersion`, and use `modNames` to name "not installed" entries. Missing `<meta><modIds>` returns `no <meta><modIds> in the save`.
- Detection: treat a file as a save if its extension is `.rws` or its first 4096 bytes contain `<savegame`, regardless of any other extension (`ReadLoadoutFile`; `ParseList` in `listformats.go` also detects `<savegame`).
- Current use is list import: `archo loadout import <save>` puts the save's Mod list into a plan as one reversible edit, without writing game files (04 §9). Test: `TestLoadoutExchange`.
- `<modSteamIds>` is not read yet. The save head (§1.7 L0) will read it: a nonzero ID lets a not-installed entry offer Subscribe (§4.3); zero means a local or unknown Mod.

### 1.4 Extracting Def references

- Stream the entire save, collect text from all `<def>` elements and deduplicate it into "the set of defNames referenced by this save".
- Whether to include other reference forms (research-name keys in research progress, `<kindDef>` and other fields not named `<def>`), and their exact element paths, will be decided after measurement (undecided).
- Extraction is part of the full read (§1.7 L1), cached and run in the background as described there.

### 1.5 Mapping Defs back to Mods

- Candidates: **Mods in this save's `<modIds>`**, resolved to currently installed Instances under 04 §4.2. This save does not count toward Mods outside that set.
- Use Def facts (03 §5.1) in those Mods' effective content (03 §1) to find the defining Mod by defName. When several Mods define (override) the same defName, attribute it to the last in the save's load order, matching the game. If several Def types share a name, count all those types as present (minimal rule; element context may distinguish them later, undecided).
- Exclude abstract Defs (`Abstract="True"`).
- A defName in a save but not defined by any installed Mod means a Mod update removed it, or the Mod was uninstalled. Count it as "in the save, source not found", without an error.
- Interpretation: a Def reference means only **"was present"**, not depth of play.

### 1.6 Tools for Archo and external agents

Save reading has read-only tools in the tool layer (05 §1.2, §4.2, §5.2): `saves list` (`list_saves`), `saves show` (`get_save`) and `unplayed` (`list_unplayed`, not implemented). Save-management actions are `manage_saves` (§4.4). A save reference is `sv:<hex>`, a short hash of the save's real path (05 §2); a file name or `<colony>/<save>` also resolves. Every fix in `saves show` names the command or tool call that performs it, so agents act through the existing tools (§4.3).

### 1.7 Read tiers, cache and background reading

A save is read at three costs; each tier is computed and cached separately. L0 is implemented (`savefile.ReadHead`, cached in memory per App by path, size and mtime; `app.ListSaves`). L1 is implemented in the core (`internal/app/saveread.go`): the cache, `archo saves index`, `Save.Summary` on every listed save with a valid cache row, `App.SaveFacts` for one save, and grouping by world (§4.1). Can it load uses the cached facts for its malformed and class rows and for what a missing Mod owns (§4.3, §4.6); the CLI, MCP and page do not show play facts yet.

| Tier | What | Cost | Used for |
| --- | --- | --- | --- |
| **L0 head** | File path, size and modification time; `<meta>` (§1.3, plus `<modSteamIds>`); the last 64 bytes, checking for `</savegame>` | A few ms per save | Grouping by RimSaves prefix or file name (§4.1), list matching (§4.2), the Mod list and its differences, cut-off files (§4.3) |
| **L1 full read** | One streaming pass over the whole file (`savefile.Scan` → `savefile.Facts`, versioned by `savefile.ParserVersion`): whether the XML is well formed and where reading stopped; colony identity (world name, seed and persistent random value; player faction and every faction; scenario; maps with their settlement); ticks, start tick, in-game day and date, storyteller and difficulty, real play time, permadeath; the player's humanlike pawns (name, race, kind, dead, slave, where) and counts of animals, mechanoids and prisoners; raid and colonist statistics, research progress, the game's history series (wealth and colonists in full, the rest as last value); letters from the archive; every `<def>` value and `Class="…"` value with counts (§1.4); references of six kinds checked against the objects the save defines. Element paths and their sources: [save format fields](../research/2026-11-04_save-format-fields.md), from the game's code and unverified on real saves; biome is not read | Synthetic 27 MB save: 0.08 s, 4 MB peak heap (07 §6 baseline: 0.11 s for `<def>` extraction) | Grouping by world, consequences of missing Mods (§4.6), play facts and timeline (§4.5), unused content (§3) |
| **L2 derived** | L1 joined with the current state: matched list, differences, Def and class attribution (§1.5, §4.6), last-load evidence from the game log (03 §6.7) | Milliseconds | Can it load (§4.3) |

- L1 depends only on the file, so it is cached in the database (`save_cache`, 07 §3) by path, size, modification time and parser version, as gzip JSON like `log_cache`; it is not reread while all four are unchanged. Beside the facts the row keeps what lists need (ticks, world identity, world and player faction names, malformed), so listing reads no facts; `App.SaveFacts` loads one save's facts. A file whose size or modification time changes while it is read is being written: not stored. L2 depends on the current list and index, so it is not cached and is recomputed when either changes. L0 is read on demand.
- L1 runs in a child process, `archo saves index` (05 §4.2), as background task kind `saves` (06 §2.16, §8.4): progress lines reach the status line and the task record, and each finished file is written to the cache at once, so a stopped run keeps what it read. The selected save is read first, then newest first, with 2–4 readers. Reading streams with constant memory; the child process keeps the GUI within its memory budget (06 §10).
- A file modified in the last 5 seconds, or whose size changes between two reads, is **being written**: it gets no verdict and is read again later. The page rereads L0 when it opens and when a running game exits (08 §4).
- CLI and MCP do not wait for the background run: `saves show` reads L1 for its one save inline; `saves list` reports saves without L1 as `meta only` and names `saves index`. Until L1 exists, every save is `meta only`, `saves show` says play facts are not read, and neither names `saves index`.

## 2. Content catalog

### 2.1 Purpose

Players do not know `ThingDef` or `IncidentDef`. The content catalog groups each Mod's Defs into categories players understand, to:

- Say "this Mod adds 12 weapons, 3 factions and 5 events" in Mod details;
- List unused content by category (§3);
- Identify functional Mods (§3.3).

### 2.2 Mapping Def types to categories (draft)

| Category | Def type | Notes |
| --- | --- | --- |
| Factions | `FactionDef` | Whether hidden factions (`hidden`) count is undecided |
| Races | `ThingDef` with `race` and `humanlike` | |
| Animals | `ThingDef` with `race`, neither humanlike nor mechanoid | |
| Mechanoids | `ThingDef` with `race`, a mechanoid | |
| Buildings | `ThingDef` with `category` of `Building` | Includes furniture, production benches and defenses |
| Weapons | `ThingDef`, a weapon (has `verbs` or belongs to a weapon item category) | |
| Apparel | `ThingDef` with `apparel` | Includes armor |
| Plants | `ThingDef` with `plant` | |
| Items | Other `ThingDef` with `category` of `Item` | Food, medicine, materials, etc. |
| Floors | `TerrainDef` | |
| Research | `ResearchProjectDef` | |
| Events | `IncidentDef`, `GameConditionDef` | |
| Quests | `QuestScriptDef` | |
| Xenotypes and genes | `XenotypeDef`, `GeneDef` | Biotech |
| Culture | `MemeDef`, `PreceptDef`, `RitualPatternDef` | Ideology |
| Abilities | `AbilityDef` | Includes psycasts |
| Implants and conditions | `HediffDef` | |
| Traits | `TraitDef` | |
| Biomes | `BiomeDef` | |
| Storytellers | `StorytellerDef` | |
| Scenarios | `ScenarioDef` | |
| Not content | Other types, such as `KeyBindingDef`, `MainButtonDef`, `StatDef`, `JobDef`, `WorkGiverDef`, `ThinkTreeDef`, `SoundDef`, `DesignationCategoryDef`, `LetterDef` | Mechanics and UI; players do not "play" these |

- This table is a draft. Subdividing `ThingDef` requires reading its fields (`race`, `category`, `apparel`, `plant`, `verbs`). Whether Def facts in 03 §5.1 currently carry these fields needs checking; add them if absent (unimplemented).
- Inheritance: fields may come from a parent Def (`ParentName`); classify using inherited values.
- Each Def belongs to one category: use the first matching row, top to bottom.
- `PawnKindDef` is not a separate category: it forms part of factions and races.
- Label categories that may leave no trace in saves (events, quests, scenarios, storytellers) "may be underestimated" in the UI.

## 3. Unused content

Under "not played yet › playthrough review › unused content" (06 §2.11). Consider only saves where the player actually played with the Mod enabled: compare a Mod only against saves whose `<modIds>` includes it.

### 3.1 Three groups

| Group | Criterion | UI wording |
| --- | --- | --- |
| Never appeared | The Mod has playable content (categories other than "not content" in §2.2), none of which appears in any save with it enabled | "Enabled across 4 colonies, never encountered", followed by what it adds, by category |
| Only a small part played | The share of playable content encountered is below a threshold (draft: 20%, configurable in settings, undecided) | "Adds 40 weapons, only 3 used", listing unused content by category |
| Functional, excluded | See §3.3 | Separate collapsed group: "These Mods leave no save data; saves cannot tell whether they were used" |

- Count **colonies** (§4.1): union all saves of the same colony so that saving a colony 30 times does not count as 30 playthroughs.
- Count only Mods still installed. Current enabled status does not affect the result, but show "enabled / not enabled in the current list" in the UI.
- A Mod enabled in only one short colony (few saves) has a low-confidence conclusion; label it "small sample" (threshold undecided).
- Each item expands to show evidence: which saves enabled it and which defNames appeared (00 §4 item 2).

### 3.2 Interpretation

- Def references mean only "was present": one wall and a hundred walls count alike; researching without building counts only as the research being present.
- Mapping uses the currently installed version. If a Mod update changes defNames, references in old saves lose their source (§1.5), biasing conclusions toward "unused". Explain this in the UI.
- Defs added by a Patch (effective content in 03 §1) belong to the Mod adding the Patch.

### 3.3 Functional Mods

UI, performance, bug-fix and pure mechanics-adjustment Mods do not add Defs to saves; saves cannot tell whether they were used. Criterion (draft):

1. A Mod with **no** Def in a playable content category in its effective content (only Patches, assemblies, textures, language files, or Defs solely in "not content") is functional;
2. The player may mark a Mod "functional" or "not functional", overriding detection. This is player data on its Lineage (04 §8); storage is undecided (possibly a reserved tag).

Functional Mods do not enter the first two groups. Framework Mods (such as Harmony and HugsLib) naturally fall under rule 1.

### 3.4 Relationship with Archo

Results need no AI. Archo may "interpret" this page: pick unused content worth trying and explain why (09 §4.2, after 1.1). Send statistics, not saves, with consent under 09 §3.2.

## 4. Saves

The Saves page is in the Library group (06 §2.11; moved out of Play review on 2026-11-04). A colony is not stored as an object of its own: it is the group of saves of one game. Facts about the colony are read from its **latest save** and shown once; an older save shows only how it differs.

### 4.1 Group by colony

Evaluate in order:

1. A RimSaves file name `<folder>#§#<save>.rws` puts the save in colony `<folder>`; the page shows `<save>` as its name.
2. With L1 facts (§1.7), saves sharing world identity form one colony: world seed, persistent random value and world name (a world generated again from the same seed gets another persistent random value). The player faction's name is left out because players rename it during play; the colony takes the name of its newest read save's faction. A save not read in full yet joins the one world colony in its folder whose saves share its file name (rule 3). Unverified on real saves.
3. Otherwise, group by file name after removing trailing numbers and separators. An autosave (`Autosave-<n>`) names no colony: it joins the colony in the same folder whose save has the same Mod list and is nearest in time, else an "Autosaves" group. Regroup when L1 facts arrive.

- Name a colony after its RimSaves folder, else the player faction, else the world. Order colonies by their latest save, and saves newest first.
- Saves in a pack's folder belong to that pack (§4.2 rule 1). Colonies whose latest save matches no list form a separate "matches no list" group.

### 4.2 Matching a plan

Implemented (`app.ListSaves`, 2026-11-04). Evaluate in order and stop at the first match:

1. Save is in a pack's save directory → that pack.
2. Save modification time falls after a launch record (08 §1) and before the next launch, and that launch used this user data directory → that launch's plan. A pack's launch does not match a save in a game folder (packs save in their own folder).
3. Save `<modIds>` (enabled set resolved under 04 §4.2) exactly matches a version (04 §3) of a plan (`TokenFingerprint`) → that plan; if several match, choose the most recently played.
4. No match → "matches no list".

Approximate matching for rule 3 (an enabled set differing by one or two entries) is undecided; omit it initially to avoid false matches. Rules 1 and 2 may match a save whose Mods differ from the list; §4.3 lists those differences.

### 4.3 Selecting a save

**Where it belongs**: game version and environment, the matched list or pack with the rule that matched it, and the location (folder and file).

**Can it load?** compares the save with the matched list as it is now and lists only what needs attention. Passed checks are not listed. `app.CheckSave` implements every row but the last load's: the rows needing only the head (cut off, Mods, order, game version, no list) always, and those needing the full read (malformed XML, classes, what a Mod owns) when the save's L1 facts are cached (§1.7; `CheckSave` never reads a save in full). Without them its rows and levels are those of the head alone.

| Item | Level | Consequence shown | Fix |
| --- | --- | --- | --- |
| Cut off: no `</savegame>` (L0) and not being written | Error | Can never load | Recycle; open the previous readable save |
| Malformed XML (L1). A file that only ends early is the cut-off row, not this one | Error | Where reading stopped (line and byte) | Recycle; open the previous readable save (complete and, when read, well formed) |
| A Mod in the save is not installed, whether the list lacks it or has it only as a missing entry (as after a fix, until Steam's download is scanned) | Error when it owns pawns, factions or classes in the save (L1, §4.6); otherwise, and without the full read, warning | What it owns (§4.6), its classes among them; broken references are not implemented | Subscribe and enable (needs `<modSteamIds>`; only Subscribe when the list has it already), else Make a list from this save |
| A Mod in the save is installed but not enabled | As above | As above | Enable in the list, positioned in save order |
| `Class="…"` with no type in the list's loaded Mods or the game (L1), and no Mod with a row of its own explaining it: a class whose Mod has a row is counted in that row (§4.6) | Error | The types, and the installed Mod defining them when one does (one row per such Mod, one for types no installed Mod defines) | None: such a Mod is not in the save's Mods |
| The last load of this save had errors (a game log phase whose save name matches, 03 §6.7) | Error | Count and first message | Open the game log at that load |
| A Mod in the list is not in the save | Information | Usually safe: it starts fresh in an existing save | Disable to match |
| Same Mods in a different order | Information | Usually safe; patch order differs | Use the save's order |
| Save made for another game version than the environment's | Warning | The game converts it on load | — |

- Compare packageIds case-insensitively. For Workshop entries written as `<package ID>_steam`, strip the suffix before comparing (04 §4.1).
- The verdict takes the highest level present: **loads with the list**, **loads, but content will be lost** (warnings) or **will not load cleanly** (errors). The quick-fix bar has at most one primary action: **Fix and launch** applies every Mod fix as one reversible plan edit, starts any subscriptions, previews, then launches (08 §2); for a cut-off save it is Recycle; for a save matching no list it is Make a list from this save.
- `saves show` prints each item with its consequence and the command that performs the fix (`fix: <label> (archo <command>)`), then the quick fix. With the full read, a missing Mod's consequence says what loading without it loses (§4.6), marked inferred where it is; without it, the consequence is generic: what it added to the save is lost on load.
- `saves fix --launch` does not launch a malformed save, as for a cut-off one.

**Mod list**: the save's Mods in load order, named from `<modNames>`; not-installed entries and reordered entries marked; the list's Mods missing from the save named after it.

**Play facts** (L1): day and date, storyteller and difficulty, real play time, biome, wealth, colonists (with deaths), animals, research progress and raids survived, each with the change since the colony's previous save.

### 4.4 Actions

The actions are implemented in `internal/app/saveactions.go` (`FixSave`, `RenameSave`, `CopySave`, `BackupSaves`, `TrashSaves`), with their commands and MCP tool (`internal/cmds/saves.go`, `manage_saves`; 05 §4.2, §5.2) and the GUI page (06 §2.11).

| Action | What it does | Confirmation |
| --- | --- | --- |
| Fix | The Mod fixes of §4.3 as one reversible plan edit (04 §2.1): installed Mods enabled and Workshop Mods added as missing entries, each right after the nearest Mod before it in the save; optionally the list's Mods the save lacks disabled, and the shared Mods put in the save's order. The caller subscribes to the Workshop Mods; `--launch` launches afterwards | Not required; preview first (`--dry-run`) |
| Create a list from this save's Mods | Create a plan containing the imported save list (same as `loadout import --new`, §1.3); retain and mark uninstalled entries, as with ModsConfig.xml import | Not required; the new plan's first version has source `import` |
| Rename | Rename the file in place, inline as in 06 §2.17. Keep a RimSaves prefix; refuse an existing name | Not required |
| Copy | Copy to `<name> (copy).rws` (then `(copy 2)`…) in the same folder, never overwriting, keeping the modification time so the copy sorts beside the original | Not required |
| Back up | Copy to `<data directory>/backups/saves/<time>/`, preserving filenames. Allow multiple selections; one backup shares one time directory. Copy only; leave originals in place | Player confirmation required |
| Move to the recycle bin | Use the recycle bin rules in 08 §7: save files are one of the three accepted categories. No permanent deletion | Player confirmation required |

- `<time>` is local time, `YYYYMMDD-HHMMSS` like database backups (07 §3).
- Both actions requiring confirmation follow 06 §6: list affected files and sizes first; execute only after the player clicks "confirm". Archo can request but cannot confirm for the player (09 §2.3). MCP previews with `dry_run=true` and executes with `confirm=true` (05 §5.2).
- While the game runs (08 §4), refuse rename and recycle bin moves and explain "the game is running". Backups and copies work normally (read-only source access).
- A colony with a cut-off save gets a cleanup hint offering to recycle it.
- `saves fix --launch` launches only when the save can load and none of its Mods is still a missing entry of the list: a Workshop Mod the fix just subscribed is downloaded by Steam in the background, so the player scans and runs the fix again.

### 4.5 Timeline and events

From the L1 facts of a colony's saves (and the game's own history records inside each save, if measurement confirms they hold a series): wealth and colonists over in-game days, one point per save, with deaths marked; events such as deaths, raids repelled and marriages, newest first. Fields await measurement. Counting occurrences instead of only presence is optional.

### 4.6 What a Mod owns in a save

Implemented for can it load (`internal/app/saveowners.go`, 2026-11-04; synthetic tests only, unverified on real saves). To state the consequence of a missing Mod, attribute the save's L1 facts (§1.7) to the Mods with a row of their own (not installed, or installed but not in the list). A Mod **owns** what the save needs and nothing the list loads provides: an object that a Mod the list loads, or the game, also provides is not lost.

- **Defs**: each `<def>` value by Def facts (03 §2; batched, `store.DefDefiners`), as in §1.5: non-abstract definitions; of several defining Mods with rows, the last in the save's order. A Def the game generates from another at startup has no Def file and resolves by the Def it is made from: prefixes `Corpse_`, `Meat_`, `Blueprint_`, `Blueprint_Install_`, `Frame_`, `Techprint_`, `Psytrainer_`, `Neurotrainer_`, `Make_`, `Administer_`, suffixes `_Rough`, `_RoughHewn`, `_Smooth` (`DefGenerator`, 1.6). Counted per Def type with the number of `<def>` elements naming them, about one per object ("3 things, 1 hediff").
- **Pawns** (the player's humanlike pawns, alive and dead) by their race Def; **factions** by their FactionDef, named by the faction's name. The facts count animals, mechanoids and prisoners without their races, so those are not attributed.
- **Classes**: each `Class="…"` value; a generic one by its definition and each argument. The game's own types by `store.GameClassOwners` (03 §5.3); `System.` and `UnityEngine.` types count as present (not indexed; the runtime has them). Mod types by `store.TypeDefiners` in every form the game tries (07 §7 item 20): the value, and the value prefixed by each of `store.SaveNamespaces`; a simple name found nowhere is looked up as a nested type's inner name (`store.NestedTypeDefiners`). World and game components are among the classes: the facts do not say where a `Class` appeared, so they are not reported apart (not implemented; needs a scanner field).
- **Which facts exist** (03 §3), and so what is certain: Def facts for every loadable installed Mod once an analysis of the game version has run; type facts only for Mods enabled when an analysis ran (a disabled Mod keeps them while its content is unchanged); a Mod removed from disk keeps its facts while its tombstone lasts (02 §5.5; inferred from the code and seen in synthetic tests: scan and analysis keep a tombstone's version view). Facts of any copy of the packageId count, in any environment of that game version (`store.PackageFactViews`). A Mod without facts of a kind (never installed, or disabled without type facts) is given, **inferred**, what nothing installed provides of that kind: the Defs no installed Mod defines and the classes no installed assembly nor the game defines. When several Mods lack facts, each is given all of it and says it may belong to the others. A Def no installed Mod defines may also be one a Mod update removed (§1.5), so a Mod whose inferred share is only Defs stays a warning.
- **Unchecked**: when a Mod the list loads has no Def view, the Defs, pawns and factions are not checked; when one has no DLL view, or the game's types are not indexed (`GameTypesIndexed`), a class found nowhere is not concluded on (no class-missing row, nothing inferred), since it may be in the unindexed assembly. The row says what was not checked; with neither checked it is as without the full read.
- Counts per kind ("3 colonists of race Kurin_Race (1 dead), 1 faction (Kurin Clan), 1 class (ModA.CompBroken), 3 things, 1 hediff") are `app.SaveOwned` on the row's `SaveIssue.Owns` (JSON `owns`); the text is `consequence()` in `internal/cmds/saves.go` (CLI and MCP) and `issueView()` in `internal/view/saves.go` (the page).
- Speed: `saves list` checks every save, so lookups are batched and kept per App while the database is unchanged (`saveOwnerMemo`); Defs are looked up only for saves with a Mod row. Measured in 07 §6.
- Not implemented: whether a Mod can be removed mid-playthrough (`saves list --mod`), which the same attribution answers.

## 5. Playthrough themes and content conflicts (1.2)

Some Mods have no technical conflict but undermine one another in play. For example, Milira and Deadman's Switch do not technically conflict, but their power levels differ so much that the latter becomes pointless when played together.

- In 1.2, Archo provides suggestions first. In the plan editor (06 §2.5) or playthrough review, the player clicks "Ask Archo about this list's theme". Archo reads the plan's Mod list, content catalog statistics (§2) and unused content (§3), identifies combinations that may undermine one another and explains why. Label conclusions "inferred".
- Send Mod names, descriptions and statistics with consent under 09 §3.2.
- Keep this out of "can it run" and block no actions: these are gameplay suggestions, not errors.
- Turn accumulated community data into rules later (community data channel in 04 §6); rule form is undecided.

## 6. Worth playing (long term)

Two pages under "worth playing" in the sidebar: play goals and shared packs (06 §2.12). This records direction only; **details will be decided later**.

### 6.1 Player-shared packs

- Share a **RimArcho list** (the list exported from a pack, 04 §9.3): a manifest only (each Mod's lineage, semantic hash and source Workshop ID, 08 §5.1), without Mod files.
- Download every Mod from Steam Workshop; the sharing site hosts no Mod files.
- On import, match local Instances by packageId and hash (04 §9.1, 08 §5.4), and list Workshop links for missing entries.
- The sharing site is a separate project outside the RimArcho repository. RimArcho only exports RimArcho lists, imports them and browses them on the "shared packs" page (interface undecided).

### 6.2 Recommendations that understand the player

- Input is the answer to question two: unused content (§3), colonies the player has played and frequently used categories.
- Archo uses this to recommend content and play goals ("worth playing › play goals"). This is "from being directed to understanding the player" in 00 §2.
- The player profile **stays local**; the player can view and clear it. Obtain consent under 09 §3.2 before sending it to the model.
- Recommendation data requirements, profile structure and relationship with the sharing site will be decided later.

## 7. Acceptance

| # | Scenario | Expected | Status |
| --- | --- | --- | --- |
| SV1 | `loadout import` a save (`TestLoadoutExchange`) | Stop after `<meta>`; import 4 entries, 1 uninstalled with a name; no ModsConfig.xml writes; `undo` restores | Pass |
| SV2 | Read all saves to extract Defs | Every save's size and modification time remain unchanged | Unimplemented |
| SV3 | Second analysis with no new saves | Use the cache without rereading saves | Pass for the full read (synthetic: `TestSavesFullRead`); unused content (§3) is unimplemented |
| SV4 | A Mod with only Patches and assemblies | Goes into "functional, excluded", not "never appeared" | Unimplemented |
| SV5 | A Mod "appears" only in saves without it enabled | Does not count as having appeared | Unimplemented |
| SV6 | 30 saves from one colony | Count as 1 colony | Unimplemented |
| SV7 | Matching a save to a plan | Follow §4.2 order; unmatched saves enter "saves without a matching list" | Partial: launch and same-Mods matches pass (synthetic: `TestSavesListGroupAndMatch`), pack saves in golden `saves`; unmatched saves untested |
| SV8 | Back up a save without player confirmation | No files written; after confirmation, appears in `<data directory>/backups/saves/<time>/`, original unchanged | Pass (synthetic: `TestSaveFileActions`, `TestManageSavesActions`) |
| SV9 | Move to the recycle bin | Confirm first; enter system recycle bin; refuse while the game runs | Partial: confirmation passes (synthetic: `TestSaveFileActions`, `TestManageSavesActions`, golden `saves`); the real recycle bin and a running game are untested |
| SV10 | Fix a save whose list lacks an installed Mod and a Workshop Mod | One reversible edit: the installed Mod enabled, the Workshop Mod added as a missing entry, each in save order. Until the download is scanned the save still reads "content will be lost", and fixing again only subscribes again (no edit) | Pass (synthetic: `TestFixSave`, golden `saves`) |
| SV11 | Open Saves with no L1 cache | Left card, middle header and Mod list appear from L0 at once; the background task reads the selected save first | Unimplemented |
| SV12 | A save written during a crash (no `</savegame>`) | Cut off, error, Recycle offered; a file still being written gets no verdict | Partial: cut off passes (synthetic: `TestCheckSave`); a file being written is untested |
| SV13 | RimSaves name `Ashfall#§#Landing.rws` | Grouped under colony `Ashfall`, shown as `Landing` | Pass (synthetic: `TestSavesListGroupAndMatch`) |
| SV14 | A save whose Mod with pawns is not installed | Error naming what is lost; Fix subscribes and enables it as one reversible edit | Pass (synthetic: `TestCheckSaveOwnership`, a removed and two never-installed Mods; the fix as SV10); a real save untested (release VM row) |
| SV15 | `get_save` on MCP | Same text as `saves show`; each fix names its command or tool call | Pass (synthetic: `TestSaveToolsCLIParity`, golden `saves`) |
