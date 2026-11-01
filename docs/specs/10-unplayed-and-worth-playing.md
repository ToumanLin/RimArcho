# 10 · Unplayed and worth playing

> Status: question two, "what haven't I played?", is a design specification for 1.1; §5 is for 1.2; §6 is a long-term direction. Only reading the Mod list at the start of a save (§1.3) is implemented; everything else is unimplemented.
> Basis: 00 (§2, §4), 00a, 03, 04, 06, 08, 09.

Question two asks: **which installed Mods and content have gone unused across multiple playthroughs?** Read all saves without modifying them and map the Defs they reference back to their providing Mods. This analysis is deterministic and needs no AI; Archo participates only in interpreting results and making suggestions (09 §4).

The answer to question two feeds question three: first establish what has gone unplayed, then choose what is worth playing.

## 1. Save reading

### 1.1 Read-only

- Save analysis is **read-only**: open saves for reading only, never write, rename or move them. Backups and recycle bin moves are separate save-management actions requiring player confirmation (§4.4, §4.5).
- Results stay local and are not uploaded. Only statistics are sent to the model, after consent (09 §3.2).

### 1.2 Save locations

| Location | Belongs to |
| --- | --- |
| `<user data directory>/Saves/*.rws` | Environments using this user data directory, and plans launched in those environments. When environments share the game's default user data directory (02 §2.4), deduplicate by real path and read only once |
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
- `<modSteamIds>` is not read yet (undecided: whether save management should use it to locate Workshop entries for "what is missing now").

### 1.4 Extracting Def references

- Stream the entire save, collect text from all `<def>` elements and deduplicate it into "the set of defNames referenced by this save".
- Whether to include other reference forms (research-name keys in research progress, `<kindDef>` and other fields not named `<def>`), and their exact element paths, will be decided after measurement (undecided).
- Cache each save's results locally by (path, size, modification time); do not reread when all three are unchanged. Cache location follows 07 §3 (undecided).
- Measured baseline in 07 §6: 202 local saves, 15–28 MB each; the latest has 1382 distinct `<def>` values, extracted in 0.11 seconds. At this rate, the first full read takes twenty-odd seconds; run it in the background with progress (06 §8.4). Thereafter read only new and changed saves.

### 1.5 Mapping Defs back to Mods

- Candidates: **Mods in this save's `<modIds>`**, resolved to currently installed Instances under 04 §4.2. This save does not count toward Mods outside that set.
- Use Def facts (03 §5.1) in those Mods' effective content (03 §1) to find the defining Mod by defName. When several Mods define (override) the same defName, attribute it to the last in the save's load order, matching the game. If several Def types share a name, count all those types as present (minimal rule; element context may distinguish them later, undecided).
- Exclude abstract Defs (`Abstract="True"`).
- A defName in a save but not defined by any installed Mod means a Mod update removed it, or the Mod was uninstalled. Count it as "in the save, source not found", without an error.
- Interpretation: a Def reference means only **"was present"**, not depth of play.

### 1.6 Tools for Archo and external agents

Save reading must have corresponding read-only tools in the tool layer from 05, exposed in both CLI and MCP (unimplemented; command names undecided, to be settled when added to 05 §4). Suggested form:

| Suggested command | Purpose |
| --- | --- |
| `saves list [--colony <name>]` | List saves: colony, time, game version, enabled count, matching plan (§4.2) |
| `saves show <save>` | A save's Mod list, matching plan, missing / extra Mods (§4.3) |
| `unplayed [--group never\|partial\|functional] [--mod <ref>]` | Unused content (§3) |

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

## 4. Save management

Under "not played yet › playthrough review › save management" (06 §2.11).

### 4.1 Group by colony

- Group saves from the same colony. Derive colony identity from world and player-faction information in the save (for example, world seed, world name, player faction name); exact fields await measurement (undecided). If unavailable, group by filename after removing trailing numbers and the `Autosave-` prefix.
- Name groups after the player faction or world; order saves newest first.
- Show the matching plan and environment for each group (§4.2). Older saves without any matching plan form a separate "saves without a matching list" group.
- Pack saves (§1.2) belong to that pack and show its name.

### 4.2 Matching a plan

Evaluate in order and stop at the first match:

1. Save is in a pack's save directory → that pack.
2. Save modification time falls after a launch record (08 §1) and before the next launch, and that launch used this user data directory → that launch's plan.
3. Save `<modIds>` (enabled set resolved under 04 §4.2) exactly matches a version (04 §3) of a plan → that plan; if several match, choose the most recently played.
4. No match → "saves without a matching list".

Approximate matching (for example, an enabled set differing by one or two entries) is undecided; omit it initially to avoid false matches.

### 4.3 Selecting a save

Show:

- Save time, game version (`<gameVersion>`), enabled count then;
- Matching plan and environment (or pack), and the basis for the match (which rule in §4.2);
- **What is missing now**: entries in the save's `<modIds>` not currently enabled in the matching plan. Divide into "installed but not enabled" and "not installed" (names from `<modNames>`);
- **What is extra now**: entries currently enabled in the matching plan but absent from the save.

Compare packageIds case-insensitively. For Workshop entries written as `<package ID>_steam`, strip the suffix before comparing (04 §4.1).

### 4.4 Actions

| Action | What it does | Confirmation |
| --- | --- | --- |
| Re-enable missing Mods in the list | Add "installed but not enabled" entries back to the matching plan, positioned in save order; list uninstalled entries without acting on them | Not required; one reversible plan edit (04 §2.1) |
| Create a list from this save's Mods | Create a plan containing the imported save list (same as `loadout import`, §1.3); retain and mark uninstalled entries, as with ModsConfig.xml import | Not required; the new plan's first version has source `import` |
| back up | Copy to `<data directory>/backups/saves/<time>/`, preserving filenames. Allow multiple selections; one backup shares one time directory. Copy only; leave originals in place | Player confirmation required |
| Move to the recycle bin | Use the recycle bin rules in 08 §7: save files are one of the three accepted categories. No permanent deletion | Player confirmation required |

- `<time>` is local time, formatted consistently with other backup directories (07 §3, undecided).
- Both actions requiring confirmation follow 06 §6: list affected files and sizes first; execute only after the player clicks "confirm". Archo can request but cannot confirm for the player (09 §2.3).
- While the game runs (08 §4), refuse recycle bin moves and explain "the game is running". Backups work normally (read-only source access).

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
| SV3 | Second analysis with no new saves | Use the cache without rereading saves | Unimplemented |
| SV4 | A Mod with only Patches and assemblies | Goes into "functional, excluded", not "never appeared" | Unimplemented |
| SV5 | A Mod "appears" only in saves without it enabled | Does not count as having appeared | Unimplemented |
| SV6 | 30 saves from one colony | Count as 1 colony | Unimplemented |
| SV7 | Matching a save to a plan | Follow §4.2 order; unmatched saves enter "saves without a matching list" | Unimplemented |
| SV8 | Back up a save without player confirmation | No files written; after confirmation, appears in `<data directory>/backups/saves/<time>/`, original unchanged | Unimplemented |
| SV9 | Move to the recycle bin | Confirm first; enter system recycle bin; refuse while the game runs | Unimplemented |
| SV10 | "Re-enable missing Mods in the list" | One reversible edit; only list uninstalled entries | Unimplemented |
