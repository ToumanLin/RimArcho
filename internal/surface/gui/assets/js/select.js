// pickClick applies a click on a multi-select list's row to its selection (16 §3.4), the
// same in the editor's lists, the Mod library and background tasks:
//   click             only this row: the selection is cleared (actions take the cursor row)
//   ctrl+click        toggles this row; with nothing selected the cursor row joins it first,
//                     since it was what the actions took
//   shift+click       the range from the anchor (or the cursor), replacing the selection
//   ctrl+shift+click  adds that range
// o.at, o.cursor and o.anchor are view positions (anchor -1: none); o.one(j) is what a
// ctrl+click on j toggles (a tag header: its rows), o.range(j) what a range or the cursor
// takes at j (a header: nothing); o.seed false when the cursor was not the actions' target
// (another list was focused). It returns the new anchor.
export function pickClick(sel, e, o) {
  const ctrl = e.ctrlKey || e.metaKey;
  if (e.shiftKey) {
    const anchor = o.anchor < 0 ? o.cursor : o.anchor;
    if (!ctrl) sel.clear();
    const [a, b] = [Math.min(anchor, o.at), Math.max(anchor, o.at)];
    for (let j = a; j <= b; j++) for (const x of o.range(j)) sel.add(x);
    return anchor;
  }
  if (ctrl) {
    if (!sel.size && o.seed !== false && o.cursor >= 0 && o.cursor !== o.at) for (const x of o.range(o.cursor)) sel.add(x);
    const ks = o.one(o.at);
    const all = ks.length && ks.every((x) => sel.has(x));
    for (const x of ks) all ? sel.delete(x) : sel.add(x);
    return o.at;
  }
  sel.clear();
  return o.at;
}
