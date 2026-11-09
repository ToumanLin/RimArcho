// Layout stays independent of instance selection: an inactive group heading shares
// its first copy's row, so every cursor, drag and verb still names a real instance. A tag
// group's header (a negative entry, disabledgroups.js) is a row of its own; copy headings
// start again under it.
export function listGeometry(items, view, column, rowHeight = 28) {
  const tops = [], heights = [], heads = [];
  let total = 0, previous = null;
  for (const index of view) {
    if (index < 0) { tops.push(total); heights.push(rowHeight); heads.push(false); total += rowHeight; previous = null; continue; }
    const item = items[index];
    const lineage = item.lineage || item.id;
    const heading = column === 0 && item.copyCount > 1 && lineage !== previous;
    const height = rowHeight + (heading ? rowHeight : 0);
    tops.push(total); heights.push(height); heads.push(heading); total += height; previous = lineage;
  }
  return { tops, heights, heads, total };
}
export function rowAt(layout, y) {
  let lo = 0, hi = layout.tops.length;
  while (lo < hi) { const mid = (lo + hi) >> 1; if (layout.tops[mid] + layout.heights[mid] <= y) lo = mid + 1; else hi = mid; }
  return lo;
}
