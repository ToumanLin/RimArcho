// The looks gallery's grid (16 §2.5 Looks view): geometry only, no DOM, so the node suite
// (testdata/looks.mjs) checks it. Cards have one height; a look with several textures takes
// one cell per texture; a category smaller than a row shares its row with the next small
// categories, each under its own heading, so a Mod with many one-look folders leaves no
// blank rows. The page draws only the rows visibleRows names (16 §10).

export const GRID = { min: 124, card: 152, head: 26, gap: 3, group: 14, padX: 8, padY: 4, foot: 70, overscan: 320 };

// gridColumns: how many cells fit in width, and one cell's width.
export function gridColumns(width, g = GRID) {
  const inner = Math.max(0, width - g.padX * 2);
  const cols = Math.max(1, Math.floor((inner + g.gap) / (g.min + g.gap)));
  return { cols, cell: Math.max(1, Math.floor((inner - g.gap * (cols - 1)) / cols)), inner };
}

// spanOf: the cells a look takes: one per texture it draws, at most a whole row.
export const spanOf = (thumbs, cols) => Math.min(cols, Math.max(1, thumbs || 1));

// gridLayout places groups ([{ spans: [n, …] }], one span per look) in a box of the given
// width. rows: [{ y, h, heads: [{ x, w, group }], cards: [{ x, w, n }] }]; cards (by n, the
// look's place in reading order): [{ row, x, w, group, item }]; total: the content's height.
export function gridLayout(groups, width, g = GRID) {
  const { cols, cell, inner } = gridColumns(width, g);
  const step = cell + g.gap;
  const rows = [], cards = [];
  let y = g.padY, band = null;
  const widthOf = (span) => spanOf(span, cols) * step - g.gap;
  const newRow = (head) => {
    const row = { y, h: (head ? g.head : 0) + g.card + g.gap, heads: [], cards: [], end: 0 };
    rows.push(row);
    y += row.h;
    return row;
  };
  const put = (row, x, group, item, w) => {
    row.cards.push({ x, w, n: cards.length });
    cards.push({ row: rows.length - 1, x, w, group, item });
  };
  groups.forEach((grp, gi) => {
    const spans = grp.spans || [];
    if (!spans.length) return;
    const all = spans.reduce((a, s) => a + widthOf(s) + g.gap, -g.gap);
    const firstTwo = widthOf(spans[0]) + (spans.length > 1 ? widthOf(spans[1]) + g.gap : 0);
    let x = band ? band.end + g.group : 0, i = 0;
    // the rest of the current row takes the group when all of it, or its first two cards, fit
    if (!band || x + Math.min(all, firstTwo) > inner + 1) { band = newRow(true); x = 0; }
    const head = { x, w: 0, group: gi };
    band.heads.push(head);
    for (; i < spans.length && x + widthOf(spans[i]) <= inner + 1; i++) {
      put(band, x, gi, i, widthOf(spans[i]));
      x += widthOf(spans[i]) + g.gap;
    }
    head.w = Math.max(60, x - g.gap - head.x);
    band.end = x - g.gap;
    while (i < spans.length) { // the rest of a large group: full rows without a heading
      const row = newRow(false);
      x = 0;
      for (; i < spans.length && (x === 0 || x + widthOf(spans[i]) <= inner + 1); i++) {
        put(row, x, gi, i, widthOf(spans[i]));
        x += widthOf(spans[i]) + g.gap;
      }
      row.end = x - g.gap;
      band = row;
    }
    if (band.end + g.group + step > inner) band = null;
  });
  return { cols, cell, inner, rows, cards, total: rows.length ? y + g.foot : 0 };
}

// visibleRows: the rows [first, end) that touch the box scrolled to top, with the overscan.
export function visibleRows(layout, top, height, overscan = GRID.overscan) {
  const rows = layout.rows, from = top - overscan, to = top + height + overscan;
  let lo = 0, hi = rows.length;
  while (lo < hi) { const mid = (lo + hi) >> 1; if (rows[mid].y + rows[mid].h < from) lo = mid + 1; else hi = mid; }
  let end = lo;
  while (end < rows.length && rows[end].y < to) end++;
  return [lo, end];
}

// cardTop: where card n's box starts in the content, under its row's heading if it has one.
export function cardTop(layout, n, g = GRID) {
  const c = layout.cards[n];
  if (!c) return 0;
  const row = layout.rows[c.row];
  return row.y + (row.heads.length ? g.head : 0);
}

// stepCard: the card d rows below (d < 0: above) card n, the nearest one by its left edge;
// n itself where there is no such row.
export function stepCard(layout, n, d) {
  const c = layout.cards[n];
  if (!c) return layout.cards.length ? 0 : -1;
  const row = layout.rows[c.row + d];
  if (!row || !row.cards.length) return n;
  let best = row.cards[0];
  for (const o of row.cards) if (Math.abs(o.x - c.x) < Math.abs(best.x - c.x)) best = o;
  return best.n;
}

// scrollFor: the scroll position that shows card n whole in a box of the given height
// (top unchanged when it already shows).
export function scrollFor(layout, n, top, height, g = GRID) {
  const y = cardTop(layout, n, g);
  if (y - g.head < top) return Math.max(0, y - g.head - g.padY);
  if (y + g.card > top + height - g.foot) return y + g.card - height + g.foot;
  return top;
}

// texURL: the address of one texture of a look (16 §4.4): the provider's position, the slot
// (a mask: "<slot>_mask"), the box it must fit (0: the original) and the views' stamp.
export function texURL(id, pos, slot, w, stamp) {
  return `/looktex/${String(id).split("/").map(encodeURIComponent).join("/")}?p=${pos}&s=${encodeURIComponent(slot)}&w=${w}&v=${encodeURIComponent(stamp || "")}`;
}
