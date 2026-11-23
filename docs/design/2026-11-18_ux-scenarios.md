# UX by scenario: where the entries are today and how to regroup them

> Status: proposal for review (2026-11-18). Documents only; nothing here is implemented. Decisions for the player are in §5.
> Inputs: player feedback 2026-11-18 (card `cbad43cb7dac0`, points 2–3), card imp04 remainder, card `cmwlyiaefkuzs`. Sources: specs 16 §2.1–2.18, 07 §7, 10 §4, `guide.js`; refnums `hom`, `sea`, `edt`, `sav` (`%APPDATA%\RimArcho\refnums`). The `hom` file was taken before the Saves group and the Import entry existed, so those have no ref number yet.

## 1. The problem in one paragraph

The player's complaint is not that a feature is missing: it is that the next step of a task sits on another page. The clearest case is theirs: after adding a new game they wanted "the order the game already has", and the only place that offers it is step 3 of the *first-run* guide. Add Game Config's "Mod list" choice holds only existing lists and "New empty list" (`guide.js:388`), so on a second game the player must go to Import › A Mod list › "choose…", find `<user data>\Config\ModsConfig.xml` in a file dialog, and choose "import as a new list". Import itself is the last sidebar entry, has no `shift+` key, and the guide, Settings, the editor and All lists each keep their own door to it. Players asked for RimPy/RimSort list compatibility that already exists (07 §7.2) because they did not find it.

## 2. Scenarios today

Clicks count from the Launch page, mouse only, and are estimated from the specs (not measured in the app). "Far" marks a step whose entry is on a different page or group from the step before it.

### S1. First run, or coming from another manager

| # | Step | Entry today | Clicks | Far? |
| --- | --- | --- | --- | --- |
| 1 | Choose the managed folder | guide › "use this folder" | 1 | |
| 2 | Find the game | guide step 1 › tick › "add" | 2 | |
| 3 | Read the Mods | guide step 2 (automatic) › "next" | 1 | |
| 4a | Use the game's list | guide step 3 › "use this list" | 1 | |
| 4b | …or move over from RimSort/RimCrow/RimPy | guide step 3 › "import…" → **Import page** › tick manager › "bring over the selected" → back to guide | 3–5 | far (leaves the guide) |
| 4c | …or a pack / collection | guide step 3 › "choose a pack…" / paste link | 2 | |
| 5 | Later: import one more list from RimSort | sidebar › Library › Import (last entry, no ref yet) | 2+ | far |

Total: 5–10. The guide and Import overlap: step 3 is a smaller Import page with different wording ("where does your first list come from?" vs "What do you want to bring in?"), and pack/collection appear in both.

### S2. A new game install or version (e.g. 1.7 breaks Mods)

| # | Step | Entry today | Clicks | Far? |
| --- | --- | --- | --- | --- |
| 1 | Back up the working game first | Game Config page (`hom13`) › "copy the game folder…" › "copy and add" | 3 | far: under Game Configs, not where the update hurts (Launch / Can it run) |
| 2 | Add the new version | `hom13` › "Add Game Config" › a game you have / download a version › … | 4–8 | |
| 3 | **Start from the order that game already has** | not offered; Import › A Mod list › "choose…" › navigate to `Config\ModsConfig.xml` › "import as a new list" | 6–9 + a path to know | **far, and hidden** |
| 4 | Let the old list run on 1.7 | Can it run (`hom5`) › for you to confirm › "Add 1.7" | 2 | |
| 5 | See which Mods have 1.7 updates | Launch top-right "N Mods updated" or Mod library › Updates (`sea13`) | 2–3 | far |
| 6 | Fix what breaks | Can it run (`hom5`) › Auto-fix | 2 | |

Total: 19–27, and step 3 needs knowledge most players lack. Card `cmwlyiaefkuzs` (one-click backup) is done, but the entry is a button beside "Add Game Config" on a page the player opens rarely; nothing on Launch or Updates points at it before a big update.

### S3. Edit a list and test compatibility repeatedly

| # | Step | Entry today | Clicks | Far? |
| --- | --- | --- | --- | --- |
| 1 | Open the list | Lists (`hom8`) | 1 | |
| 2 | Enable/disable, sort `S` | editor | n | |
| 3 | Check | editor head "problems" → Can it run (`hom5`) | 1 | far (page change) |
| 4 | Launch | Launch (`hom33`) or `L`; list-page launch button is coming in 2B | 2 | far today |
| 5 | Read the result | Launch top-right "Last game" → Can it run / Game log (`hom6`) | 1 | |
| 6 | Back to the list | sidebar › Lists | 1 | far |

Per loop: 6+ page changes. 2B (launch from the list page) removes step 4's hop; steps 3 and 5 still leave the editor.

### S4. Fix problems

Launch problem line → Can it run (1) › action (1). Elsewhere items go to Mod library or Texture optimisation (muted links). This flow is short; keep it.

### S5. Update Mods

Launch "N Mods updated" → Updates (1–2), or Mod library › Updates (`sea13`, 2). Locked-version "upstream updated" lives in All mods' states, a third place. Acceptable; the backup entry (S2 step 1) is what is missing here.

### S6. Review saves

Saves › All saves (2) › select › "Check this save" / "Make a list" (1–2). Play review is a separate entry. Short; "Make a list" is a list import that does not appear on the Import page.

### S7. Share or import a list or pack

| Direction | Entry today | Clicks |
| --- | --- | --- |
| Export a list | editor `e` or All lists › "export…" → export subpage | 2–3 |
| Build/share a pack | editor/All lists › "create a pack…" (`B`) → Packs › export `e` | 4–6 |
| Import a list | Import page, editor `i`, All lists' own "new / import" form, drop a file, command palette | 2–3 |
| Import a pack | Import page › "Choose a pack…", or guide step 3 | 2 |
| Follow a collection | All lists › "follows Steam collections" paste, or Import › paste link › "follow" | 2–3 |

Import has five doors and two forms (Import page and All lists' "new / import", 16 §2.17); export has none on the Import page.

## 3. Inventory: every import and export

| # | Kind | Where | Produces |
| --- | --- | --- | --- |
| I1 | Game's own ModsConfig.xml | first-run guide step 3 only | first list |
| I2 | List file (12 formats, 07 §7.2: RimArcho, ModsConfig/RimSort/RimPy XML, RimSort JSON, RimCrow JSON/XML, `.rml`, text, Workshop IDs, Rentry, clipboard) | Import › A Mod list; editor `i`; All lists "new / import"; drop; command palette | new list / replace / add missing |
| I3 | Pasted text: share code, Rentry link, Steam collection link | Import › A Mod list (paste); guide step 3 (collection) | same as I2; collection can be followed |
| I4 | Save's Mod list | Import (a `.rws` file); Saves › "Make a list" | list |
| I5 | Pack `.archopack` | Import › A pack; guide step 3 | pack (background task) |
| I6 | Another manager (RimSort, RimCrow, RimPy): profiles, lists, notes/tags, rules | Import › Everything from another manager; guide "import…"; Settings row (§2.14) | lists, player data, rules, Game Configs |
| I7 | RimCrow profiles as Game Configs | inside I6 | Game Configs |
| I8 | A game folder | guide/Add Game Config › a game you have | Game Config |
| I9 | A game version from Steam | guide/Add Game Config › download a version | game + Game Config |
| I10 | Copy of a game folder (backup) | Game Config › "copy the game folder…" / Add Game Config third way | game copy + Game Config |
| I11 | Settings of another Game Config | Add Game Config › "start with" | Config files incl. Mod settings |
| I12 | Steam subscription | Mod library › Add a subscription | Mod |
| E1 | List in 7 formats (archo, text, modsconfig/rimsort, rml, rimcrow code, rimcrow-xml, workshop) | list › Export subpage; `archo loadout export` | file / clipboard |
| E2 | Pack `.archopack` (light or full) | Packs › export `e` / export list `E` | file |
| E3 | Mod settings | none (open the Config folder only) | — |
| E4 | Player data / database | none (16 §2.14: no database export) | — |

Count: **12 kinds of import in 6 places, 4 of export in 2**. Players see only the Import page's three cards, which cover I2–I7.

## 4. Proposal

### 4.1 Principles

1. **One "bring in" place**, reached from everything that brings something in, and the guide is that place's first-run view, not a second implementation.
2. **Offer the next step where the current one ends**: a new game ends with "which list?", so the game's own list is offered there; an update starts with "back up?".
3. **Fewer sidebar entries**; actions that belong to an object live on the object (a list's export on the list, a game's copy on the game).

### 4.2 Merge the guide and Import (decision A)

- **A1 — Import absorbs the guide's step 3.** The guide keeps its first two steps (folder, game, scan), then *opens the Import page* in a "first list" mode: a fourth card "the game's current list (N Mods)" comes first and is preselected; the other three cards are unchanged. Guide step 3 is deleted. One page, one wording.
- **A2 — The guide becomes Import's "start" tab.** Import gets a step bar: game › Mods › lists & data. On a first run it starts at game; later it starts at lists & data. Add Game Config is the same page started at game. Most consistent; largest change to two specced pages.
- **A3 — Keep both, share components.** Guide step 3 reuses the Import cards in place (no page jump), and Import gains the "game's current list" card. Least change; the duplication stays.

**Recommendation: A1.** It removes a step and a duplicate form while leaving the guide's game steps (which have no Import analogue) alone.

### 4.3 The new-game flow (S2) — no option needed

- Add Game Config's "Mod list" choice gains **"the game's current list (N Mods)"** as the first option when the chosen game's user data has a ModsConfig.xml, and it is the default for a game already played. This fixes the player's exact complaint in one place (I1 everywhere, not only on a first run).
- After a new version is added, the done screen offers in one line: "let <list> run on 1.7 too" (07 §1.6) and "see which Mods have 1.7 updates".
- The Import page's A Mod list card gets a chip row of **ModsConfig.xml of each Game Config** and of found RimSort/RimPy saved lists, so the common files need no file dialog.

### 4.4 Backup before an update (decision B)

- **B1** — "copy the game folder…" also appears on the Launch page's update line when the game's build changed or Steam has an update pending, as "back up <game> first".
- **B2** — Only on Updates (Mod library) as a header action.
- **B3** — Leave it on Game Configs; mention it in the release post.

**Recommendation: B1**; the Launch page is where the player notices an update, and the line already exists.

### 4.5 Fewer entries (decision C)

| Today | Proposed |
| --- | --- |
| Sidebar Library: Lists, Packs, Mod library (5 sub), Texture optimisation, Game Configs, Import | Lists, Packs, Mod library, **Game Configs**, **Import & export**; Texture optimisation moves under Mod library (C1) |
| All lists' own "new / import" form | removed; "import…" opens Import with All lists' selection as destination |
| Settings' "import from RimSort…" row | removed (Import is visible) |
| Export only on the list | Import page gains an "export" side: the current list (E1) and pack export, so "share" and "bring in" sit together (C2) |

- **C1** — Import becomes "Import & export", moves up to the top of Library (or gets `shift+` key); Texture optimisation folds into Mod library.
- **C2** — Keep Import as is, only move it up; export stays on lists.
- **C3** — Import moves to the Playing group's top as "Add" (games, lists, packs, Mods) — a single "+" for everything in §3's I-rows.

**Recommendation: C1.** C3 is the clearest model but rearranges three groups at once; revisit after A1 ships.

### 4.6 The test loop (S3)

With 2B's launch button on the list page, add a compact **"last run: N errors →"** line and the problem count in the editor head that opens Can it run *in the editor's details pane* (read-only summary, "open Can it run" for actions). The loop then stays on one page. Option for later, not decided here.

### 4.7 Removals that shrink the UI

- Guide step 3 (A1). All lists' import mode. Settings' import row. Duplicate collection input (keep All lists' "follow" for an existing list; Import's paste covers new ones).
- Not removed: Can it run's Elsewhere links, Saves' "Make a list" (the save is the context).

## 5. Comparison

| Flow | RimSort | RimCrow | RimArcho today | Proposed |
| --- | --- | --- | --- | --- |
| Layout | One main window (active/inactive), menu bar File/Edit/Download/Instances/Textures | One main screen, header icon buttons; "load order" menu (import file/clipboard/code, export) | Sidebar of pages | Same, fewer entries |
| Import a list | File › Open / Import (Rentry, collection) | header "load order" menu | Import page + 4 doors | Import & export page, all doors land there |
| New game / instance | Instances menu: new, clone | profiles; game install manager | Game Configs page | same + "game's current list" |
| Backup a game | Instances › clone instance (RimSort's paths and data, not a copy of the game) | data bundle (RimCrow data, not the game) | copy a game folder | same, surfaced at update time |
| Mod settings | Troubleshooting › delete Mod configs | Mod settings manager modal; residue cleaner | Mod library › Mod settings | see `docs/research/2026-11-18_mod-config.md` |

Both reference managers keep import/export in one menu next to the list; neither has a first-run guide that imports. Their weakness is discoverability of everything else (menus and unlabeled icons); RimArcho should keep labeled pages but match their "one place for lists in and out".

## 6. Decisions for the player

1. Guide ↔ Import: **A1** (Import absorbs step 3), A2, or A3.
2. Backup entry: **B1** (Launch update line), B2, or B3.
3. Sidebar: **C1** (Import & export at top of Library, textures under Mod library), C2, or C3.
4. Whether §4.3 (game's current list in Add Game Config) can go ahead now without waiting for 1–3 (recommended: yes, it is small and independent).
