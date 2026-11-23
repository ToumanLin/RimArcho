# Import page drafts

Four candidates for Library › Import (16 §2.18). Colours, sidebar and controls follow the GUI (`style.css` tokens). Example data only (`shared.js`); buttons show a toast and read or write nothing.

Open `a.html` … `d.html` (launch configuration `import-drafts`, port 8769). The bar at the top switches drafts and the theme.

## The problem with the current page

One "found on this computer" list, one paste box and one "choose a file…" button sit together, and they bring in three different things:

1. **A Mod list**: a new list or changes to one of the player's (from a file, a save, a link, a share code or pasted text).
2. **A pack**: an `.archopack` that becomes a pack.
3. **Another manager's data**: lists, notes, tags, rules and, from RimCrow, Game Configs.

Before choosing, the player cannot tell which of these will happen. The rows of found data name a file path and a kind ("player data"), not what the player gets. "choose a file…" sits inside the "paste" section. The long line listing formats has to be read before the player knows whether their file is accepted.

## Candidates

| Draft | Organised by | First thing the player sees | Fits best when |
| --- | --- | --- | --- |
| A By what you get | Three cards: a Mod list, a pack, everything from another manager. Each says what you get, which sources it accepts, and has its own input | "What do you want to bring in?" | The player knows what they have in hand |
| B Moving in first | Found managers as large tiles; the selected one lists each kind of data with a checkbox and what it becomes, then one "bring over" button. A slim "bring in one thing" row below | "Moving in from another manager?" | Most visits are migrations (first weeks of use) |
| C Drop anything | One large input for a file, a drop or pasted text. RimArcho says what it recognised and what it will become, then the player confirms. Below: what was found, and a table of what can come in | A drop area | The player has a file and does not care what kind it is |
| D Inbox | Two panes, as All lists: everything that can be imported (other managers, files and links used before), each marked new, imported or changed; the detail maps each kind of data to what it becomes | The list of sources and their state | Imports are repeated (a manager still in use, a followed collection) |

They share three rules the current page breaks:

- **Say what it becomes** before anything is chosen (`→ 3 new lists`, `→ 41 notes, 63 tags …`), in words, not as a file kind.
- **Recognise, then confirm**: after a file or a paste, a card names what it is ("a Mod list from RimSort", "the Mods of a save", "a pack") with its facts, then the destinations.
- **Name what does not come over**, so the player is not left guessing (C's "not here" line, D's "not imported" rows).

## Chosen: A, with a selection tree (2026-11-14)

The player found A the clearest. Moving in must not be all or nothing, and nothing is chosen for the player:

- **Above, the player picks managers.** The found managers in the third card are rows with a check box; nothing is ticked when the page opens. Ticking one adds it to the tree below and turns on its usable parts; unticking it removes it.
- **Below, the player refines.** The tree shows only the managers picked above: each manager, its kinds of data and, where a kind has parts, the parts (each list, each profile; aliases, notes, groups, tags, colour marks, ignored warnings; user rules, interlocks, switched-off rules). A box turns its whole branch on, or off when all of it is on; a parent shows a bar when partly on. Each row says what it becomes, or "not brought over".
- What cannot come over (a profile whose game is not here) is greyed with the reason; RimCrow's switched-off rules stay off when RimCrow is picked. The card row says how much is chosen ("bringing over 15 of 17 parts"). "cancel" clears the choice.
- **Check boxes.** RimArcho has no check box of its own (the GUI uses native ones). The draft's box uses the GUI's tokens and icon stroke: 16px with a 4px radius, a 1.5px `--faint` outline, `--accent` fill with the `ok` icon's tick in `--accent-fg` when on, a short bar when partly on. If adopted, it replaces the native boxes elsewhere (`.check-row`, the guide's game list, the background settings).

## Imported before (draft A)

A source the player has imported stays on the page (they may keep using that manager and come back for what is new), but its rows say what happened. The switch "example: imported before / first visit" in the review bar shows both. Example: RimCrow imported 11-09 and changed since; RimSort imported 11-12 with nothing new; RimPy never imported.

| Where | Never imported | Imported, something new | Imported, nothing new |
| --- | --- | --- | --- |
| Card row | what it holds ("4 saved lists") | a faint "imported 11-09" after the name; below, a small "new" tag and one muted line "4 notes · 2 tags · 1 rule · 1 list, 1 changed" (cut with an ellipsis, the whole in the tooltip); sorted first | "imported 11-12" after the name, "nothing new since then" below; the mark faded; sorted last |
| Picking the row | turns on everything usable | turns on only what is new; kinds with something new open | turns nothing on; the tree opens so a list can be brought in again |

In the tree:

- **A part already in RimArcho** (aliases, groups, an imported profile) is grey with a disabled box and "✓ already in RimArcho" or "✓ already the Game Config Vanilla+". A kind whose parts are all there says the same and stays folded.
- **A part with something new** shows its total and a "4 new" pill, and becomes only the new part ("4 new notes").
- **A list imported before** is not ticked by picking the manager or its kind: "already the list Main colony · import another copy". If the manager's file changed since, it has a "changed" pill and "your list Latest_ModList, changed since: 3 added, 1 removed · update it · another copy". Updating changes the player's list (undoable); a copy becomes "Main colony 2". A list never imported has a "new" pill.
- The manager row counts the two apart: "4 of 5 new parts + 1 list again".
- When the run is closed, what finished is recorded: the row then says "imported today · nothing new", and a failed part stays new.

This needs an import history (16 §2.18 has none yet): per source, its path, the signature at import (`managerSourceSignature`) and the date; per list file, the list it became and that file's signature, to tell "changed" from "same"; and per part, what is already in RimArcho, which the dry run that counts new parts already knows (N new, M already present).

## Progress (draft A)

Following 16 §3.1 (work the player starts, that takes time and whose result is inspected, is a background task; incidental reads show progress where they happen):

| Step | Shown as |
| --- | --- |
| Reading what was found, recognising a file or a paste | In place: the row or the recognition card says "reading…"; not a task |
| Importing a list | Immediate and undoable: a status line with undo; not a task. Subscribing its missing Mods stays the existing task |
| Moving in from another manager | **A background task**, one per "bring over", an item per chosen part |
| Adding a pack | **A background task**: copy (by bytes), check every file, compare with the game |

Moving in, in the draft: "bring over the selected" turns the tree into the run in place. A bar and "moving in… 5 / 16" head it; each part shows waiting (a dot), running (a spinner, with a sub-count such as "340 / 812 Mods" when a Game Config's game is read), done (a tick and the result, "117 notes, 3 kept yours", "a new list Latest_ModList") or failed (the reason, "the file changed while it was read; try again"). The card row says "moving in… 5 / 16" with a thin bar. The footer says the player can leave; "background tasks" opens the same task there. "stop" keeps finished parts and marks the rest "not done: stopped", then "continue 7"; at the end, "retry N" for failed parts, "open lists", "done". No page-wide loading bar.

The pack, in the draft: "add the pack" turns the recognition card into its run: copy the Mods (GB), check every file (count), compare with your game; the same footer and stop; at the end "open the pack".

Today `ApplyManagerImport` holds the API lock for the whole import (the window waits) and `PackImport` runs synchronously with no progress and no stop; both become tasks in the RimArcho queue (16 §2.16).

## Notes for building

- A (imported before) and D need a record of earlier imports (source path, signature, date, and the list each list file became) to mark rows imported or changed; nothing keeps that today. The migration preview's source signature (`managerSourceSignature`) could be stored with the date. Without it, importing a list again creates a duplicate ("Main colony 2") with no warning.
- The tree (A) and B's per-kind checkboxes need the migration to apply parts separately: `app.ImportRimCrow` and `ImportRimSortAux` take a selection (kinds, list files, profiles), and the found lists import as lists one by one. Today a RimCrow import applies everything except unticked profiles, and each found list is a separate preview.
- Counts per kind (lists, notes, tags, rules) for found data would come from a dry run of each source when the page opens; discovery today only lists paths. On a slow disk this belongs in the background, with the rows filling in.
