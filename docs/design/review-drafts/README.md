# Play review drafts: how the page is organised

Six candidates for Saves › Play review (16 §2.11, 12 §3). They decide the page's organisation only; how a Mod's content is shown (names, labels, pictures) is decided after, and every draft uses the same plain detail for now. Colours, sidebar and controls follow the GUI (`style.css` tokens). Example data only (`shared.js`); buttons show a toast and read or write nothing.

Open `e.html`, `f.html`, `a.html` … `d.html` (launch configuration `review-drafts`, port 8768). The bar at the top switches drafts and the theme.

## The problem with the first page

One flat list of Mods in four groups. With 500 Mods the "never seen" and "partly seen" groups are hundreds of rows with no way in but a name search, and opening a Mod pours out hundreds of defNames. The page answers "how much of this Mod did I see" but not "what have I not tried", which is asked by kind of content or by what the Mod is for.

## Candidates

| Draft | Organised by | The question it answers first | Many Mods |
| --- | --- | --- | --- |
| A By kind of content | The kinds of 12 §2.2 in a rail (buildings, weapons, apparel…), each with its total seen; the list is the Mods adding that kind | "What buildings have I never built?" | A kind holds tens of Mods, not hundreds; a Mod is repeated under each kind it adds |
| B By your tags | The player's tags from the Mod library as folding bands, each with its total and how many Mods were never seen; can switch to grouping by how much is seen | "How much of my furniture Mods did I use?" | Bands fold; depends on the player having tagged Mods (untagged Mods form one band) |
| C Table and detail | One row a Mod, as the Mod library's table: sort by any column, filter by state, kind and tag; detail on the right | "Show me the big Mods I barely touched" | Made for it: 26 px rows, sorting and filters; least guidance |
| D Map | A square a Mod, sized by how much it adds, coloured by the share seen; grouped by tag, main kind or not at all; detail on the right | "Where are the big untouched areas?" | Hundreds of Mods fit one screen; small Mods are only dots, names need a hover |

They combine: A's rail or B's bands can sit left of C's table as its filter, and D can be the overview above any of them.

## Second round: pictures instead of text

A to D were found too wordy: rows of names, fractions and category lists ask the player to read a report. E and F show the same facts as pictures, with the words in hover text.

| Draft | Organised by | What carries the meaning |
| --- | --- | --- |
| E Shelf and album | A shelf of Mod covers, the least tried first; a cover opens the Mod's album, its things as tiles under a tab per kind | Cover in colour or grey (never touched), a ring for the share seen; a tile in colour (seen) or as an outline (not seen). The only text is the Mod's name |
| F Highlights | The page picks: three big Mods never touched, three barely used, then the kinds of content; each leads to E | The same covers, rings and tiles; two headings |

The covers and tiles here are stand-ins (a gradient with initials, a glyph of the kind). The real page would use the Mod's `About/Preview.png` and each thing's texture and in-game label, none of which the analysis keeps today: Def facts hold neither `label` nor `graphicData/texPath` (05 §5.1), so E and F need them added (Def content version +1, a background re-analysis), and things without a picture of their own (research, events, quests, genes) would keep a glyph.

## G: one colony, three questions

Written after reading one real colony (48 saves over 379 days, 485 Mods in the save); its numbers are measured, not example data. What the data showed:

- 300 of the 485 Mods add nothing a save can show, and of the 183 others, 121 are things the player picks from (floors, furniture, hair, clothes, backstories). Ranking by share seen puts exactly those first (1 of 405 floors), which says nothing.
- The share means something only for what a game brings the player to: quests, research, events, creatures, factions. There a Mod with nothing seen is a storyline not started.
- The player comes for a colony, not for the library.

So the page is one colony at a time and answers, in order: what is still ahead (one square per quest, project, event or creature), what the colony was built on (the Mods with the most in the saves), and what was installed and never showed up. Pick-from Mods and functional Mods are one closing line.

## H: three panes

The player's layout: Saves becomes a sidebar group of its own (all saves, play review); the page has three panes as All saves does. Left: find box and one card a colony, "All colonies" first. Middle: draft D's map, grouped by default into "the game brings you to" (colour is the share seen of quests, research, events, creatures, factions) and "you pick from" (two states: used, or an amber outline for never used). Right: the Mod's card, then its progress: one square per quest or project, and a plain count for what is picked from. `real.js` holds one real colony's numbers (Mod names are public); its colony names are examples.
