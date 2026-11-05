# Save page drafts

Three candidates for the save page under Play review (06 §2.11, 10 §4; ToDo "Save analysis"). Colors, sidebar and controls follow the current GUI (`style.css` tokens). Example data only (`COLONIES` in `shared.js`); buttons only show a toast and never read or write files.

## The problem

A save page has to show three kinds of things:

1. **Colony facts**: game version, environment, list, saves folder, storyteller.
2. **Technical analysis of one save**: is the file complete, which Mods own what in it, how its Mod list compares with the current list, whether it will load.
3. **Play facts of one save**: wealth, colonists, events.

The obvious layout is colony list → save list → save page. That puts (2) and (3), the most useful content, on the third level, and it repeats information: a colony is only an abstraction of its saves, so the colony page and the save page show the same version, list and Mods.

## Rule shared by all three

**A colony is not an object of its own.** It is the group of saves of one game (RimSaves `folder#§#name.rws` prefix first, then the world seed). Colony facts are read from its **latest save** and appear **once**. An older save shows only how it differs from the latest one ("+1 colonist, Ozzie still alive", "16 Mods added since"), never a second copy of the same facts.

Every save gets a one-line verdict answering the question players open the page with, "can I keep playing this?":

| Verdict | Meaning |
| --- | --- |
| Loads with the current list | Mods identical, or only added Mods |
| Loads, but something will be lost | A Mod that owns objects in the save is gone (14 sculptures vanish) |
| Will not load cleanly | File cut off, a `Class="…"` with no installed assembly, unresolved references, or the last load in the game log had errors |

All three drafts reuse the same blocks: verdict, checks (file, references, code, Mods vs list, last load), what the save uses (per Mod: what it owns, and what removing it mid-playthrough costs), and play facts with change since the previous save.

## Candidates

| | A: Colony page + timeline | B: Three columns | C: One list, verdict in the row |
| --- | --- | --- | --- |
| Levels | 2: colony list, colony page | 1: everything on one screen | 1: list of saves, details in a drawer |
| Where saves live | Dots on a wealth-over-days timeline (plus "List all saves" for file actions). The page always shows one save, the latest by default; ← → or a dot switches | Middle column, under the colony facts | Rows of a table grouped by colony headings; 3 rows per colony, "N older saves" expands |
| Deepest content | Is the colony page itself: no click for the latest save, one for an older save | Always visible in the right column, split into tabs (can it load / Mods it uses / colony) | Verdict and Mod difference (−1 +1) in every row; full analysis in a drawer |
| Colony facts | Header of the colony page | Top of the middle column | Group heading |
| Good for | Continuing a colony: what happened, can I load it | Comparing saves one after another (↑↓ with the analysis updating) | Cleanup and triage across all colonies: "needs attention" filter, hide old autosaves, cut-off files |
| Cost | Saves of the same colony close in time crowd on the timeline; file management is secondary | Right column is narrow (~50% of the page); tabs hide two of three sections; needs about 1280 px | Play facts and timeline only in the drawer; the drawer covers half of the list |

## B revised (current direction, 2026-11-04)

The player chose B as the closest fit to RimArcho's design, with a different split of the columns. `b.html` is the revised draft; the first version is kept as `b-first.html`.

| Column | Content |
| --- | --- |
| Left | One card: search on top (`/`), then colonies. Clicking a colony expands its saves below it as a tree (and selects the latest); clicking the open, current colony folds it. A colony row has only its name and save count; a save row only its name and in-game day. No status icons, descriptions or warnings here. Search matches colony names, save names and Mods in a colony's saves, and expands what it finds |
| Middle | Header: save name (no icon) and save actions, then only **game**, **list / pack** and **location** (folder + file). **Can it load?** contains only what needs attention: a quick-fix bar (one primary action, e.g. Fix and launch), then one row per problem or change against the current list, each with its consequence and its own fix: missing Mod (what is removed, broken references, missing classes → Subscribe and enable), cut-off file (→ Recycle, open the previous good save), last-load errors (→ game log), added Mods (safe → Disable to match), reordered Mods (→ Use the save's order). Passed checks are not listed. **Mod list**: the save's Mods in load order, not-installed ones marked, reordered ones marked ↕, Show all, Make a list from this save |
| Right | The playthrough: colony name and day, storyteller, difficulty, played time, biome; stats at this save with change since the previous save; colonists (dead struck through); events; wealth-over-days timeline (click a point to open that save) |

Keys: `/` search (`esc` clears), ↑↓ move through visible saves, ← → fold / unfold the current colony.

History: first B had tabs in the right column; second (same day) had separate colony and save cards in the left column, status everywhere and a "What the save uses" table. The player asked to remove repeated and unimportant information: status lives only in the middle column, colony facts are split between where the save belongs (middle) and the playthrough (right), and the per-Mod usage table became the plain Mod list (its consequences moved into the missing-Mod rows).

## First recommendation (before the choice)

**A as the base, with two ideas from C.** A removes the third level instead of making it easier to reach: a player opens a colony and is already looking at the latest save's analysis. Colony and save stop being two lists. From C take:

1. The verdict sentence on each colony card (already in A's left column) so problems are visible before opening.
2. The "needs attention" filter and cleanup hints ("1 autosave is cut off and can be recycled"), placed in A's "List all saves".

B is the best for stepping through many saves, but its narrow right column fights the content that matters most.

## Specifications

Written on 2026-11-04 from revised B: page layout 06 §2.11 (Saves moved to the Library group, 06 §2.1); read tiers, cache and background reading 10 §1.7; grouping, matching, can it load, actions, timeline and attribution 10 §4; commands, `sv:` references and MCP tools 05 §2, §4.2, §5.2; cache location 07 §3; decision 00 D18.

## Open

```bash
python -m http.server 8766 --directory docs/design/saves-drafts
```

Open `http://localhost:8766/a.html`; the top bar switches drafts.

- A: ↑↓ colony, ←→ older / newer save, `l` lists all saves; click a timeline dot.
- B (revised): `/` search, ↑↓ save, ←→ fold colony; click a timeline point.
- B first draft (`b-first.html`): `1` `2` `3` choose the column for ↑↓, `tab` changes the right column's tab.
- C: click a row or `enter` for the drawer, `esc` closes, `a` hides older autosaves, `!` needs attention.
