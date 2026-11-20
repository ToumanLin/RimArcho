# Can it run: where Archo sits (2026-11-20)

Status: comparison for the user to choose from. Nothing in the specs or CHANGELOG changes yet.
The variants are real CSS/JS behind a temporary switch; a follow-up keeps only the chosen one.

## The switch (temporary)

`data-archo-spot="a".."g"` on `<html>` (or `?archo-spot=` in the dev harness URL) picks the
variant; anything else is A, the shipped placement. Code: `needsArcho` / `needsSpot` in
`internal/surface/gui/assets/js/screens.js`, CSS under "TEMPORARY (2026-11-20)" in `style.css`.
`archo.js` is unchanged (each variant uses `archoSpot(...).stand`).

## Rules the variants are judged by

Never the visual centre; use existing blank space; change the layout as little as possible;
alive; head and hair never cut; shown head-to-chest.

## Evidence

Contact sheets in `D:\RimArcho-test\archo-spot-variants\`: `sheet-2000.png`, `sheet-1280.png`,
`sheet-860.png`. Columns A–F; rows: light en and dark zh, each with a short right column (the
list can run, one log card) and a long one (blocked, eight log cards in four blocks). At 2000
and 1280 a "scrolled to her" row shows where she is when the first view does not include her;
at 860 (one column) every cell where she is below the first view is scrolled to her (marked).
Single shots: `shots/<en-light|zh-dark>/<variant>-<short|long>-<width>[-seen].png`.
Synthetic data only (canitrun `states.js`), dev harness. A–F with reduced motion (her still); G
animated, plus two 16 s GIFs from opening the page: `g-en-light-1280-short.gif`,
`g-zh-dark-1280-long.gif`.

## Variants

| | Where | Layout change | Short column | Long column | One column (860) |
|---|---|---|---|---|---|
| A | Left end of the right column's first card heading (shipped V2) | that heading grows to ~110 px | in view | in view | in view, mid-page |
| B | Behind the top edge of the right column's first card, left end; head in the page-heading row | page heading grows ~44 px (lead and actions move to its bottom line) | in view | in view | gap above the right column grows to her height |
| C | Below the right column's last card, left end, chest fades | none | in view, in the blank under the card | below the fold until the column is scrolled to its end | at the page's end |
| D | Small (96 px) in the page heading, left of check again / game log | page heading grows ~37 px | in view | in view | in view at the top |
| E | Her own card first in the right column: face + the verdict's icons (✓, or ▲ n ✕ n), chest behind its bottom edge | one ~75 px card added; logs move down | in view | in view | in view, mid-page |
| F | Bottom right of the right column's blank space, rising from its bottom; when the cards reach her, after the last card, right-aligned | none | in view, far corner | below the fold until scrolled to the end | at the page's end, right |
| G | Right of the last-save card (the left column's first card), shortened by her width; chest fades at the card's bottom line (added at the user's request) | last-save card ~150 px narrower; no height changes | in view | in view | in view at the top |

### Pros and cons

- **A** (baseline): always in view and near the run's errors. The heading is ~110 px tall with a
  blank area to her right; the user already finds it not good.
- **B**: the classic peek: body hidden by the card, head over its edge, in the blank part of the
  heading row; always in view; one figure-height of new space, shared by the whole heading row.
  **Collides with "fix N automatically"** at 1280 when that button is shown (long state: she
  covers its left end). Shipping B needs a rule for that (hide her, or move her right of the
  actions' start). At 2000 the row has room.
- **C**: no layout change at all; with a short column she fills exactly the blank that is there.
  With a long column she is not seen until the player scrolls the column to its end; her
  position wanders with the column's length.
- **D**: in view in every state and width; small enough that the heading grows only a little.
  At 96 px her face reads less; she sits among buttons, the busiest part of the page, and drifts
  left when "fix N automatically" appears.
- **E**: the only one where her face and the verdict read as one unit (her face already follows
  pass / warnings / blocked). It adds a card, so it is the largest layout change, and the icons
  repeat the counts already in the page lead (`default ▲4 ✕8`).
- **F**: no layout change; the page's largest blank area (bottom right) at 2000 and 1280 with a
  short column, so she is far from the centre and from every control. With a long column she
  falls back to after the last card (seen only at the column's end), like C.

- **G**: in view in every state and width, at the top, and nothing grows: the last-save card's
  content is short (a name, a verdict, one button), so its right part was blank anyway. She sits
  between the two columns rather than inside either one's content, not in the centre. Without a
  last save (no save loaded yet) there is no card to stand beside; G falls back to F. With a
  long chip list (missing DLC/Mods) the chips wrap one item earlier.

## Recommendation

**G** (added after the first round). It is the only variant that is in view in every state and
width without growing anything: the space beside the short last-save card was already blank. F
is its fallback when there is no last save.

Before G, the recommendation was F, with B as the alternative if she must be in view in every
state:

F is the only variant that uses blank space that already exists in the common case (a short
right column) without moving anything, and her rising from the bottom edge gives head-to-chest
without a card edge. Its weakness is the long column, where she is out of view at first; that is
also when the page is busiest, so her absence costs least there. If the user wants her always on
screen, B is the cleaner always-visible option than A or D, but it needs the collision rule with
"fix N automatically" before it ships.
