// The Disabled column's two views (16 §2.5): every copy in name order ("individual"), or the
// copies grouped under a header per tag ("tags"). A Mod sits under its first tag only, so each
// row is still one copy and the cursor, selection, drag and verbs work on unique rows.
//
// A column's view is a list of entries: an index into its items, or a header, written
// -(g+1) for c.groups[g]. c.flat keeps the filter's answer before grouping (every matching
// row, also those of closed groups), so a group can open again without asking Go.
import { S, key, api } from "./state.js";

export const VIEW_KEY = "ui.gui.plan.disabled"; // "individual" or "tags"
const CLOSED_KEY = "archo.disabled.closed";     // closed groups, by lower-case tag ("" no tag)

export const G = { view: "individual", order: [], orderStamp: undefined, closed: new Set() };
try { const v = JSON.parse(localStorage.getItem(CLOSED_KEY) || "[]"); if (Array.isArray(v)) G.closed = new Set(v); } catch { /* none closed */ }
const keepClosed = () => { try { localStorage.setItem(CLOSED_KEY, JSON.stringify([...G.closed])); } catch { /* not kept */ } };

export const grouped = () => G.view === "tags";
export const headOf = (c, vi) => (vi < 0 && c.groups ? c.groups[-vi - 1] : null);
// entryKey names a view entry across regroupings: a copy by its key, a header by its tag.
export const entryKey = (c, vi) => (vi == null ? "" : vi < 0 ? "\u0000" + (headOf(c, vi)?.key ?? "") : key(c.items[vi]));
export const curKey = (k) => { const c = S.cols[k]; return c.view.length ? entryKey(c, c.view[c.cursor]) : ""; };
// shownItems: every row the filter shows, closed groups included (the take / put / relist sets).
export const shownItems = (c) => c.flat || c.view;
// matched: how many rows the filter shows (the column head's count).
export const matched = (c) => shownItems(c).length;

// shape turns a column's plain view (item indices) into its displayed view.
export function shape(k) {
  const c = S.cols[k];
  c.flat = c.view;
  if (k !== 0 || !grouped()) { c.groups = null; return; }
  const groups = groupRows(c.items, c.view, G.order);
  if (c.view.length !== c.items.length) {
    const all = new Map(groupRows(c.items, c.items.map((_, i) => i), G.order).map((g) => [g.key, g.rows.length]));
    for (const g of groups) g.total = all.get(g.key) || g.rows.length;
  }
  const view = [];
  groups.forEach((g, gi) => {
    g.open = !G.closed.has(g.key);
    g.at = view.length;
    view.push(-(gi + 1));
    if (g.open) for (const i of g.rows) view.push(i);
  });
  c.groups = groups;
  c.view = view;
}

// groupRows: the rows under their first tag, in the Tag manager's order (pinned first, then
// the most used: API.TagList); a tag it does not list yet comes after, most rows first, and
// the rows with no tag last. Rows keep the column's order within a group.
export function groupRows(items, view, order) {
  const by = new Map(), none = [];
  for (const i of view) {
    const t = (items[i].tags || [])[0];
    if (!t) { none.push(i); continue; }
    const k = t.toLowerCase();
    let g = by.get(k);
    if (!g) by.set(k, g = { key: k, tag: t, rows: [] });
    g.rows.push(i);
  }
  const out = [];
  for (const t of order) {
    const g = by.get(t.toLowerCase());
    if (g) { g.tag = t; out.push(g); by.delete(g.key); }
  }
  out.push(...[...by.values()].sort((a, b) => b.rows.length - a.rows.length));
  if (none.length) out.push({ key: "", tag: "", none: true, rows: none });
  return out;
}

// findEntry: where keepKey is in the view; a copy in a closed group is found at its header.
export function findEntry(c, keepKey) {
  if (!keepKey) return -1;
  const at = c.view.findIndex((vi) => entryKey(c, vi) === keepKey);
  if (at >= 0 || !c.groups || keepKey[0] === "\u0000") return at;
  const g = c.groups.find((g) => !g.open && g.rows.some((i) => key(c.items[i]) === keepKey));
  return g ? g.at : -1;
}

// regroup draws the column's groups again (a group opened or closed, the view or the tags'
// order changed), keeping the cursor on its row; a row that went into a closed group leaves
// the cursor on that group's header.
export function regroup(keepKey = curKey(0)) {
  const c = S.cols[0];
  c.view = shownItems(c);
  shape(0);
  const at = findEntry(c, keepKey);
  c.cursor = Math.max(0, Math.min(at >= 0 ? at : c.cursor, c.view.length - 1));
}

export function setGroupOpen(gkey, open) {
  open ? G.closed.delete(gkey) : G.closed.add(gkey);
  keepClosed();
  regroup();
}

// foldAll opens or closes every group shown; closing takes the cursor to its group's header.
export function foldAll(open) {
  const c = S.cols[0];
  if (open) G.closed.clear();
  else for (const g of c.groups || []) G.closed.add(g.key);
  keepClosed();
  regroup();
}

// reveal opens the group that holds item i (a jump to a Mod, 16 §3.5).
export function reveal(i) {
  const c = S.cols[0];
  const g = c.groups && c.groups.find((g) => !g.open && g.rows.includes(i));
  if (g) setGroupOpen(g.key, true);
}

// setView switches the view, staying on the same Mod (from a header: its group's first).
export async function setView(v) {
  if (G.view === v) return false;
  const c = S.cols[0], vi = c.view[c.cursor], h = headOf(c, vi);
  const keep = h ? (h.rows.length ? key(c.items[h.rows[0]]) : "") : entryKey(c, vi);
  api.SetSetting(VIEW_KEY, v).catch(() => {});
  if (v === "tags") await refreshOrder();
  G.view = v;
  regroup(keep);
  return true;
}

// refreshOrder reads the tags' order again when the data changed; true when it changed.
export async function refreshOrder() {
  const stamp = S.board ? S.board.stamp : null;
  if (G.orderStamp === stamp) return false;
  let tags;
  try { tags = await api.TagList(); } catch { return false; }
  const order = (tags || []).map((t) => t.tag);
  G.orderStamp = stamp;
  if (order.join("\n") === G.order.join("\n")) return false;
  G.order = order;
  return true;
}

// loadView learns the view the player chose last (once, at start).
export async function loadView() {
  let v = "";
  try { v = await api.Setting(VIEW_KEY); } catch { /* the default */ }
  if (v !== "tags") return false;
  await refreshOrder();
  G.view = "tags";
  return true;
}
