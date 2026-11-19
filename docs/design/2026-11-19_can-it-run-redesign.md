# Can it run: redesign (2026-11-19)

Status: option A implemented 2026-11-19 (16 §2.4 "GUI layout", 16 §7.1 "Placements"). The player's card: the page's headings ("before you start", "your last save", "worth knowing", "when you last played", "errors from the last run") used different fonts, colours and indents; the save card's format clashed with the rest; Archo floated alone in the right column's empty space.

## What was wrong

Measured on the page as it was (`D:\RimArcho-test\canitrun\before\`):

| Element | Size / weight | Colour | Left edge |
| --- | --- | --- | --- |
| Column caption ("before you start") | 13 px, 600 | accent when focused, else `--fg` | column + 4 px |
| "your last save" | 12 px, 600, date beside it | `--muted` | column + 2 px, no number |
| Section ("1 breaks the game 4") | 12 px, 600, number before it | red / accent / amber / log | column + 2 px, then the number |
| Run block ("errors when … 2the list Nov 18") | 12 px, 600 | log colour | as sections; count and run run together |
| Inside the save card | ✕ / ! / ✓ as a text glyph, name and verdict both coloured, a bulleted list, a small button | red / amber / green | card + 14 px |
| Inside every other card | status icon (SVG), bold title, grey explanation, buttons at the foot | grey with an icon in colour | card + 16 px, text at + 42 px |

Three heading levels (column, save, section) in four colour schemes, two item layouts, and two left edges.

## Proposal

**One heading style, inside the card.** The columns lose their visible captions; each section is one card whose first row is its heading, the app's `.card-head` (as in the editor's areas, Background tasks and Settings): the area number, a 14 px icon in the section's colour, the name in the muted heading colour (12 px, 600), the count in faint figures, and on the right what the card is about: when the save was loaded, the run's identity, or the green dot of a running game. Only the icon is coloured, so every heading has the same font, colour and left edge, and the colour still tells the section at a glance (✕ red breaks the game, ⓘ accent worth knowing, ▲ amber for you to confirm, the log colour for run errors). Headings stay at the top of the column while a long card scrolls.

**One item layout.** Your last save becomes an item like the others: the verdict's status icon, the save's name in bold with the verdict beside it in grey, the rows that need attention as chips with +N for the rest, and its buttons at the foot. Open in Saves is primary when the save will lose content or not load cleanly, as an item's main action is.

**Nothing is lost.** The column names (before you start, when you last played, this run and the last) are the columns' accessible names, and where the items come from is their accessible description and the hover of each heading. "and N more" is the +N chip's tooltip and accessible name. The CLI, TUI and MCP are unchanged.

**Archo.** She looks over the right column's first card: right-aligned, 140 px, and the card's top edge hides her last 12 px, below the chin (the "peek" of 16 §7.1, built from `stand` in a band before the card, so `archo.js` is unchanged). The right column is the last run's: the errors she will one day explain through the "ask Archo" hook. The band sits beside the left column's first card, which is empty space on every window size, and she never covers a heading or a button. The right column always has a card now, so she always has one to look over.

| State | Left column | Right column | Archo |
| --- | --- | --- | --- |
| 0 problems | your last save ✓, worth knowing (RAM / VRAM) | errors from the last run: ✓ no errors in the last run | thumbsup, then calm cycling |
| 1 problem | your last save, breaks the game 1 | as above | surprised |
| Many | one card per section, the column scrolls; headings stay on top | one card per run block (started / loaded / played) | surprised (breaks) / skeptic (to confirm) / ponder (run errors only) |
| No last save | the first section's card is first | unchanged | unchanged |
| No last-run log | unchanged | one card: the missing-log sentence with the path to copy | unchanged |
| Memory estimate stale / missing | the RAM / VRAM item as before: stale totals while it re-estimates; missing draws in use and free only | — | — |
| Under 900 px | the columns stack, left first; headings no longer stick | after the left column, Archo above its first card | hidden when she would be cut (the existing `fits`) |
| Nothing at all | before you start: ✓ nothing here | as 0 problems | thumbsup |

`1`–`9` now count the save card (and an empty column's card) too, so every heading has its number and the same left edge; such a card focuses its first button.

## Options considered

Mockups from the real CSS on synthetic data (`D:\RimArcho-test\canitrun\mock\`):

| | Option | Image |
| --- | --- | --- |
| A | **Recommended, implemented.** Cards with their headings inside; Archo over the right column's first card. | `mock\a-cards-peek.png` |
| B | The same cards; Archo in the page heading, between the list's summary and the page actions. The heading row must grow by about 50 px for her, which pushes both columns down, and she lands near the middle of the page, between its summary and its main action, auto-fix (rule 1: never the visual centre). | `mock\b-cards-head.png` |
| C | Headings outside the cards, as before but in one style; Archo standing at the right column's foot. She stays disconnected from the content, and on a long right column she ends below the fold. | `mock\c-before.png` (the page before, for comparison) |

## Not changed

The page heading (the list's name, ▲ N, ✕ M), the item rows, buttons, evidence marks, done lines, confirmations, the draggable gap, keys other than `1`–`9`, the TUI, and Archo's expressions and cycling.
