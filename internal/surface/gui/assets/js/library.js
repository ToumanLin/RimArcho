// Mod library > All (16 §2.9): what is on this machine. The same layout as the plan editor:
// the filter bar on top of a table, the details in a column on the right. The table is like
// a file manager's details view: one row per Lineage (opening to its copies), columns the
// player sorts (click a header, again reverses), moves (drag a header), sizes (drag its
// edge) and picks (right click the headers), kept in ui.gui.library.columns. The details
// follow the cursor; enter opens them over the page and closes them again. What it shows
// comes from view.BuildLibrary through API.Library; each copy carries only the codes of
// its ops (view.CopyOps), the labels and keys are the page's op table, sent once. The table
// draws only the rows in view, so 1350+ Lineages stay smooth. The Tags view (v) groups the
// rows under a header row per tag; on a header the details become the Tag manager.
import { S, $, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { on } from "./api.js";
import { icon } from "./icons.js";
import { detailFrame, noteEl, stale } from "./detail.js";
import * as pop from "./popups.js";
import { beginPageRead, pageLoading, openScreen, page, act, rescanButton, area, full } from "./screens.js";
import { updateFromForm } from "./subscribe.js";
import { createBar } from "./filterbar.js";
import * as ops from "./ops.js";
import { createTable, mergeColumns } from "./datatable.js";
import { wireSeparator } from "./separator.js";

const ROW = 30;      // every row has the same height: the list is drawn by position
const HEAD = 28;     // the column headers, sticky at the top of the table
const COLS_KEY = "ui.gui.library.columns";
const VIEW_KEY = "ui.gui.library.view"; // "mods" or "tags" (16 §2.9)
// a column's width when the player has not set one; the name column takes whatever is left
// (its width here is its least), so the table grows with the window
const WIDTH = { name: 200, size: 72, state: 116, source: 120, used: 150, updated: 84, version: 64, author: 130, tags: 160, played: 84 };
// columns whose natural order is A to Z (the arrow points up); the rest start biggest or newest
const ASC = new Set(["name", "source", "author", "tags"]);
const L = {
  page: null, sort: "name",
  bar: null,         // the filter bar (filterbar.js): its search and filters are the query
  open: new Set(),   // lineages shown open
  flat: [], cur: 0, anchor: -1,
  sel: new Set(),    // selected copies, by ref
  full: false,       // the details over the whole page (enter)
  focus: "rows",     // "rows" or "detail" (→ on a copy, ← back)
  cols: null,        // [{key, title, shown, w}], in the player's order
  view: null,        // "mods", or "tags": a header row per tag (ui.gui.library.view)
  tags: [],          // every tag (API.TagList): the groups' order, the Tag manager's list
  closed: new Set(), // tag groups shown closed, by lower-case tag ("" no tag)
  picked: new Set(), // tags picked in the Tag manager (ctrl+click on their chips), by lower-case tag
};
let ui = null;
let els = null;
let subscriptionPanel = null;

export function initLibrary(u) {
  ui = u;
  // A scan, or another process (the CLI, an agent), changes what is there.
  on("scan", () => { if (S.screen === "library") load(); });
  addEventListener("focus", () => { if (S.screen === "library") load(); });
  // a background task on the copies ended (16 §2.16): what is there changed
  document.addEventListener("task-ended", (e) => {
    if (S.screen !== "library") return;
    const ref = e.detail?.ref;
    if (ref) { const b = bar(); b.search = ""; b.filters = []; b.input.value = ""; L.sel = new Set([ref]); }
    load(false, ref);
  });
  addEventListener("resize", () => { if (S.screen === "library") drawRows(); });
}

// open shows the page (the sidebar's Mod library > All).
export async function open() {
  if (S.screen !== "library") { L.full = false; L.focus = "rows"; }
  const read = beginPageRead("library", { retry: open });
  await load(true, undefined, read);
}

// openSearch shows the page searching for q (a background task's mod, 16 §2.16).
export async function openSearch(q) {
  const b = bar();
  b.filters = [];
  b.search = q;
  b.input.value = q;
  L.cur = 0;
  await open();
}

// openInstance clears filters and selects the exact copy, expanding its Lineage if needed.
export async function openInstance(id) {
  const b = bar();
  b.search = ""; b.filters = []; b.input.value = "";
  L.sel.clear(); L.full = false; L.focus = "rows";
  const read = beginPageRead("library", { retry: () => openInstance(id) });
  await load(true, undefined, read, id);
}

// relang: the language changed; the page is drawn again in it, where it was (the column
// titles come again with the page data).
export async function relang() {
  const top = els ? els.rows.scrollTop : 0;
  L.cols = null;
  await load(true);
  if (els) { els.rows.scrollTop = top; els.rows.dataset.sig = ""; drawRows(); }
}

function bar() {
  if (!L.bar) {
    L.bar = createBar({
      cls: "lib-fbar",
      splitKey: "ui.gui.filter.split:library",
      column: () => els.side,
      data: () => L.page,
      change: () => { L.cur = 0; load(); },
      leave: () => els && els.rows.focus(),
      placeholder: () => T("search: words, tags:, source:, content:") + "  /",
    });
  }
  return L.bar;
}

async function load(fresh, focusRef, read = null, focusID = "") {
  if (!read && S.screen !== "library") return;
  read ||= beginPageRead("library", { retry: () => load(fresh, focusRef, null, focusID) });
  const b = bar();
  let p, saved, view, tags;
  try {
    [p, saved, view, tags] = await Promise.all([
      api.Library(b.search, b.filters, L.sort),
      L.cols ? null : api.Setting(COLS_KEY).catch(() => ""),
      L.view ? null : api.Setting(VIEW_KEY).catch(() => ""),
      L.view === "tags" ? tagList() : null,
    ]);
    // the first visit learns the view with the page; the Tags view then needs the tags too
    if (!L.view && view === "tags") tags = await tagList();
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  L.page = p;
  if (!L.view) L.view = view === "tags" ? "tags" : "mods";
  if (tags) L.tags = tags;
  if (focusID) {
    const copy = (p.rows || []).flatMap(r => r.insts).find(i => i.id === focusID);
    focusRef = copy?.ref;
    if (focusRef) L.sel = new Set([focusRef]);
    else say("fail", T("this copy is no longer in the Mod library"));
  }
  if (focusRef) for (const r of p.rows || []) if (r.insts.some((i) => i.ref === focusRef)) L.open.add(r.id);
  ops.setTable(p.opTable);
  if (!L.cols) L.cols = mergeColumns(columnDefs(), saved);
  flatten();
  if (focusRef) {
    const at = L.flat.findIndex((x) => x.t === "inst" && x.i.ref === focusRef);
    if (at >= 0) L.cur = at;
  }
  land(1);
  if (fresh || !els || S.screen !== "library" || !document.body.contains(els.root)) build();
  else draw();
  if (focusRef) { ensureVisible(); drawRows(); }
  read.finish();
}

// tagList reads every tag (API.TagList); a failure leaves the groups in row order.
const tagList = () => Promise.resolve().then(() => api.TagList()).catch(() => []);

// flatten lists the rows: a Lineage, and its copies when it is open (or when the filters
// show only some of them); a Lineage of one copy is that copy's row.
// In the Tags view each tag's rows follow its header row (gk: the group's key), so a mod
// with two tags is listed twice; untagged mods gather in No tag at the end.
function flatten() {
  const keep = L.flat[L.cur] && rowKey(L.flat[L.cur]);
  const out = [];
  const push = (r, gk) => {
    if (r.single) { out.push({ t: "inst", r, i: r.insts[0], lone: true, gk }); return; }
    out.push({ t: "lin", r, gk });
    if (L.open.has(r.id) || r.insts.length < r.copies) for (const i of r.insts) out.push({ t: "inst", r, i, gk });
  };
  if (L.view === "tags") {
    for (const g of groups()) {
      out.push({ t: "grp", g });
      if (!L.closed.has(g.key)) for (const r of g.rows) push(r, g.key);
    }
  } else for (const r of L.page.rows || []) push(r);
  L.flat = out;
  const at = keep ? out.findIndex((x) => rowKey(x) === keep) : -1;
  L.cur = at >= 0 ? at : Math.min(L.cur, Math.max(out.length - 1, 0));
  // a selected copy that is no longer listed is no longer selected
  const shown = new Set(out.filter((x) => x.t === "inst").map((x) => x.i.ref));
  for (const k of [...L.sel]) if (!shown.has(k) && !lineageHas(k)) L.sel.delete(k);
}
const rowKey = (x) => (x.t === "grp" ? "G|" + x.g.key : (x.gk != null ? "G" + x.gk + "|" : "") + (x.t === "lin" ? "L|" + x.r.id : "I|" + x.i.ref));

// groups: the Tags view's groups of the matching rows, in the tag list's order (pinned
// first, then the most used), case-insensitive; a tag the list does not have yet comes
// after, most rows first.
function groups() {
  const by = new Map(), none = [];
  for (const r of L.page.rows || []) {
    if (!(r.tags || []).length) { none.push(r); continue; }
    for (const t of r.tags) {
      const k = t.toLowerCase();
      if (!by.has(k)) by.set(k, { key: k, tag: t, info: null, rows: [] });
      const g = by.get(k);
      if (!g.rows.includes(r)) g.rows.push(r);
    }
  }
  const out = [];
  for (const info of L.tags) {
    const g = by.get(info.tag.toLowerCase());
    if (g) { g.tag = info.tag; g.info = info; out.push(g); by.delete(g.key); }
  }
  out.push(...[...by.values()].sort((a, b) => b.rows.length - a.rows.length));
  if (none.length) out.push({ key: "", tag: "", none: true, info: null, rows: none });
  return out;
}
const firstLine = (s) => String(s || "").split("\n")[0];
function lineageHas(ref) { return (L.page.rows || []).some((r) => r.insts.some((i) => i.ref === ref)); }

// ---- building ----

function build() {
  const sc = openScreen("library");
  const w = page("fill library", lead(), rescanButton(T("rescan")));
  const root = el("div", "lib" + (L.full ? " full" : ""));
  const main = el("section", "lib-main card");
  const head = el("div", "lib-head");
  const rows = el("div", "lib-rows");
  rows.tabIndex = -1;
  const th = el("div", "lib-th");
  const sizer = el("div", "lib-sizer");
  rows.append(th, sizer);
  main.append(head, rows);
  // the left column, as the editor's Not enabled: the filter bar on top, then the details (the
  // editor's detail card) filling the rest (16 §2.9)
  const detail = el("section", "area detail lib-detail");
  const dhead = el("div", "head");
  const scroll = el("div", "scroll");
  scroll.tabIndex = -1;
  detail.append(dhead, scroll);
  // the filter bar and the details are two cards, one above the other (16 §2.9)
  const side = el("div", "fcol lib-side");
  side.append(bar().el, detail);
  dhead.addEventListener("click", () => { if (L.bar.fills()) L.bar.toggle(false); });
  const split = el("div", "split lib-split");
  wireSplit(split, root);
  const selbar = el("div", "lib-bar");
  selbar.hidden = true;
  root.append(side, split, main);
  w.append(root, selbar);
  sc.append(w);
  els = { root, main, head, rows, th, sizer, side, detail, dhead, scroll, bar: selbar, lead: w.querySelector(".lead") };
  // 1 the filter, 2 the table, 3 the details (16 §3.5); the numbers sit in their headings
  const fb = L.bar.el;
  // the bar is kept across visits: mark it once
  const row = fb.querySelector(".fbar-row");
  if (!row.querySelector(":scope > .anum")) row.prepend(el("span", "anum", "1"));
  area(fb, null, () => L.bar.focus());
  area(main, null, () => { L.focus = "rows"; rows.focus(); markFocus(); hints(); });
  area(detail, null, () => { L.focus = "detail"; scroll.focus(); markFocus(); hints(); });
  rows.addEventListener("scroll", drawRows, { passive: true });
  rows.addEventListener("mousedown", () => { L.focus = "rows"; rows.focus(); markFocus(); });
  scroll.addEventListener("mousedown", () => { L.focus = "detail"; markFocus(); });
  rows.addEventListener("contextmenu", (e) => { if (!e.target.closest(".lib-row")) e.preventDefault(); });
  draw();
  rows.focus();
}

function updatePage(ref) {
  subscriptionPanel?.remove();
  subscriptionPanel = updateFromForm(els.main, ref, () => { subscriptionPanel?.remove(); subscriptionPanel = null; els.rows.focus(); });
}

// The left column's width: drag the split (kept in ui.gui.library.side), double click for
// the default.
const SIDE_KEY = "ui.gui.library.side.v2"; // v2: the default became the editor's 38%
let sideW = null;
function wireSplit(sp, root) {
  applySide(root);
  wireSeparator(sp, {
    key: SIDE_KEY, range: () => [240, root.clientWidth - 360],
    get: () => root.querySelector(".lib-side").getBoundingClientRect().width,
    set: (v) => { sideW = v; applySide(root); },
    reset: () => { sideW = 0; applySide(root); }, changed: drawRows,
    restore: sideW == null ? (v) => { sideW = +v || 0; applySide(root); } : null,
  });
}
function applySide(root) {
  if (sideW) root.style.setProperty("--lib-side", sideW + "px");
  else root.style.removeProperty("--lib-side");
}

function markFocus() {
  if (!els) return;
  els.detail.classList.toggle("focus", L.focus === "detail");
  els.main.classList.toggle("focus", L.focus === "rows");
}

function lead() {
  const p = L.page;
  if (!p) return "";
  return p.lead;
}

function draw() {
  if (!els) return;
  if (els.lead) els.lead.textContent = lead();
  els.root.classList.toggle("full", L.full);
  bar().draw();
  drawHead();
  els.sizer.style.height = HEAD + L.flat.length * ROW + "px";
  els.rows.dataset.sig = "";
  drawRows();
  follow(true);
  drawBar();
  markFocus();
  hints();
}

// ---- the columns ----

const columnDefs = () => (L.page?.columns || []).map((c) => ({
  ...c, cls: c.key, w: WIDTH[c.key] || 100, fixed: c.key === "name", dir: ASC.has(c.key) ? 1 : -1,
}));
const table = createTable({ key: COLS_KEY, columns: columnDefs,
  state: {
    get cols() { return L.cols; }, set cols(v) { L.cols = v; },
    get sort() { return L.sort.replace(/^-/, ""); },
    set sort(k) { L.sort = (L.sort.startsWith("-") ? "-" : "") + k; },
    get dir() { return L.sort.startsWith("-") ? -1 : 1; },
    set dir(v) { L.sort = (v < 0 ? "-" : "") + L.sort.replace(/^-/, ""); },
  },
  redraw: () => { drawHead(); els.rows.dataset.sig = ""; drawRows(); },
  sort: () => { L.cur = 0; load(); },
  focus: () => { L.focus = "rows"; els.rows.focus(); markFocus(); },
});
function shownCols() { return table.visible(); }
function drawHead() {
  const p = L.page, h = els.head;
  h.innerHTML = "";
  h.append(el("span", "anum", "2"));
  const n = p.shown === p.total ? Tf("%d mods", p.total) : Tf("%d of %d mods", p.shown, p.total);
  h.classList.add("word-facts");
  const title = el("span", "lib-count", n);
  title.id = "library-count";
  h.append(title, el("span", "faint", p.size));
  if (L.view === "tags") h.append(el("span", "faint", Tf("%d tags", L.tags.filter((t) => t.n).length)));
  // the view switch (v): every mod, or grouped by tag (16 §2.9)
  const seg = el("div", "seg sub lib-view");
  seg.setAttribute("role", "tablist");
  seg.setAttribute("aria-label", T("group by"));
  for (const [v, label] of [["mods", T("mods")], ["tags", T("tags")]]) {
    const b = el("button", L.view === v ? "on" : "", label);
    b.setAttribute("role", "tab");
    b.setAttribute("aria-selected", String(L.view === v));
    b.tabIndex = -1;
    b.onmousedown = (e) => e.preventDefault();
    b.onclick = () => setView(v);
    seg.append(b);
  }
  // collapse all / expand all: the groups in the Tags view, the Mods' copies in the Mods view
  const fold = el("div", "lib-fold");
  for (const [open, label, ic] of [[false, T("collapse all"), icon.fold], [true, T("expand all"), icon.unfold]]) {
    const b = el("button");
    b.innerHTML = ic;
    b.title = label;
    b.setAttribute("aria-label", label);
    b.tabIndex = -1;
    b.onmousedown = (e) => e.preventDefault();
    b.onclick = () => foldAll(open);
    fold.append(b);
  }
  h.append(fold, seg, keyCaps("v"));
  table.draw(els.rows, els.th, { role: "treegrid", labelledby: title.id, count: L.flat.length, multiselectable: true });
}

// ---- the rows ----

function drawRows() {
  if (!els || !els.root.isConnected) return;
  const box = els.rows;
  const top = Math.max(0, box.scrollTop - HEAD), h = box.clientHeight || 600;
  const a = Math.max(0, Math.floor(top / ROW) - 8), b = Math.min(L.flat.length, Math.ceil((top + h) / ROW) + 8);
  const shownRef = shownCopy() && shownCopy().ref;
  const sig = [a, b, L.cur, L.sel.size, L.flat.length, shownRef, [...L.sel].join(",")].join("|");
  if (box.dataset.sig === sig) return;
  box.dataset.sig = sig;
  table.count(L.flat.length);
  for (const n of [...box.querySelectorAll(".lib-row")]) n.remove();
  if (!L.flat.length) {
    const p = L.page;
    box.removeAttribute("aria-activedescendant");
    box.append(el("div", "lib-row empty", p.total ? T("no mod matches") : T("no mods yet (R scans the mod folders)")));
    return;
  }
  const frag = document.createDocumentFragment();
  for (let k = a; k < b; k++) frag.append(rowEl(L.flat[k], k));
  box.append(frag);
  const active = box.querySelector(".lib-row.cur");
  if (active) box.setAttribute("aria-activedescendant", active.id);
  else box.removeAttribute("aria-activedescendant");
}

function rowEl(x, k) {
  if (x.t === "grp") return groupEl(x, k);
  const r = el("div", "lib-row " + x.t + (x.lone ? " lone" : "") + (x.gk != null ? " in" : "") + (k === L.cur ? " cur" : ""));
  r.id = "library-row-" + encodeURIComponent(rowKey(x));
  r.setAttribute("aria-level", String((x.t === "lin" || x.lone ? 1 : 2) + (x.gk != null ? 1 : 0)));
  r.style.top = HEAD + k * ROW + "px";
  const selected = x.t === "inst" ? L.sel.has(x.i.ref) : x.r.insts.every((i) => L.sel.has(i.ref)) && x.r.insts.length;
  if (selected) r.classList.add("sel");
  r.setAttribute("aria-selected", String(!!selected));
  // the cells: a Lineage's from its row, a copy's from the copy (a lone copy is its Lineage)
  const cells = x.t === "lin" || x.lone ? x.r : x.i;
  for (const c of shownCols()) {
    if (c.key === "name") { r.append(nameCell(x)); continue; }
    const v = cells[c.key] || "";
    const cell = el("span", "word-facts c " + c.key + (c.key === "state" && cells.code ? " " + cells.code : ""), v);
    if (c.key === "size") cell.classList.add("num");
    if (c.key === "used" && cells.usedTitle) cell.title = cells.usedTitle;
    else if (v) cell.title = v;
    r.append(cell);
  }
  const name = r.querySelector(".name .t");
  if (name) {
    name.id = r.id + "-name";
    r.setAttribute("aria-labelledby", name.id);
  }
  table.row(r, k);
  r.onclick = (e) => click(k, e);
  r.ondblclick = () => { if (x.t === "lin") toggle(x.r); };
  r.oncontextmenu = (e) => {
    e.preventDefault();
    if (!selected) { L.cur = k; L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); follow(); drawBar(); }
    menuAt(e.clientX, e.clientY);
  };
  return r;
}

// groupEl: a tag's header row in the Tags view: the tag, how many of its mods are shown,
// pinned, and its note's first line; on it the details are the Tag manager.
function groupEl(x, k) {
  const g = x.g, open = !L.closed.has(g.key);
  const r = el("div", "lib-row grp" + (g.none ? " none" : "") + (k === L.cur ? " cur" : ""));
  r.id = "library-row-" + encodeURIComponent(rowKey(x));
  r.setAttribute("aria-level", "1");
  r.setAttribute("aria-expanded", String(open));
  r.setAttribute("aria-selected", "false");
  r.style.top = HEAD + k * ROW + "px";
  const cell = el("span", "c name");
  cell.append(el("span", "m"));
  const c = el("span", "chev" + (open ? " down" : ""));
  c.innerHTML = icon.chev;
  c.setAttribute("aria-hidden", "true");
  c.onclick = (e) => { e.stopPropagation(); toggleGroup(g.key); };
  const t = el("span", "t", g.none ? T("no tag") : g.tag);
  t.id = r.id + "-name";
  const n = g.info && g.info.n !== g.rows.length ? Tf("%d of %d mods", g.rows.length, g.info.n) : Tf("%d mods", g.rows.length);
  cell.append(c, t, el("span", "gn", n));
  if (g.info?.pinned) cell.append(el("span", "gn", T("on top")));
  if (g.info?.note) cell.append(el("span", "gnote", firstLine(g.info.note)));
  r.append(cell);
  r.setAttribute("aria-labelledby", t.id);
  table.row(r, k);
  r.onclick = (e) => click(k, e);
  r.ondblclick = () => toggleGroup(g.key);
  r.oncontextmenu = (e) => {
    e.preventDefault();
    L.cur = k; L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); follow(); drawBar(); hints();
    groupMenu(e.clientX, e.clientY);
  };
  return r;
}

// nameCell: only the name (16 §2.9): a Lineage's, or under an open Lineage the copy's folder.
function nameCell(x) {
  const left = el("span", "c name");
  left.append(el("span", "m"));
  if (x.t === "lin") {
    const c = el("span", "chev" + (L.open.has(x.r.id) || x.r.insts.length < x.r.copies ? " down" : ""));
    c.innerHTML = icon.chev;
    c.setAttribute("role", "button");
    c.tabIndex = -1;
    c.setAttribute("aria-expanded", String(isOpen(x.r)));
    c.setAttribute("aria-labelledby", "library-row-" + encodeURIComponent(rowKey(x)) + "-name");
    c.onclick = (e) => { e.stopPropagation(); toggle(x.r); };
    left.append(c, el("span", "t", x.r.name));
  } else if (x.lone) {
    left.append(el("span", "t", x.r.name));
  } else {
    const branch = el("span", "tree", "└");
    branch.setAttribute("aria-hidden", "true");
    left.append(branch, el("span", "t kind", x.i.label || x.i.name));
  }
  return left;
}

const isOpen = (r) => L.open.has(r.id) || r.insts.length < r.copies;

// toggle opens or closes a Lineage. The cursor never rests on a Lineage's own row: closing
// the one it is in moves it on to the next row (16 §2.9).
function toggle(r) {
  const x = L.flat[L.cur];
  const inside = x && x.r === r;
  if (L.open.has(r.id)) L.open.delete(r.id); else L.open.add(r.id);
  flatten();
  if (inside && !isOpen(r)) {
    const h = L.flat.findIndex((y) => y.t === "lin" && y.r.id === r.id && y.gk === x.gk);
    if (h >= 0) { L.cur = h + 1 < L.flat.length ? h + 1 : Math.max(0, h - 1); land(L.cur > h ? 1 : -1); }
  }
  resize();
  ensureVisible();
  follow();
  hints();
}

// toggleGroup opens or closes a tag's group; closing the one the cursor is in takes the
// cursor to its header.
function toggleGroup(key) {
  const x = L.flat[L.cur];
  const inside = x && x.t !== "grp" && x.gk === key;
  if (L.closed.has(key)) L.closed.delete(key); else L.closed.add(key);
  flatten();
  if (inside && L.closed.has(key)) {
    const h = L.flat.findIndex((y) => y.t === "grp" && y.g.key === key);
    if (h >= 0) L.cur = h;
  }
  resize();
  ensureVisible();
  follow();
  hints();
}

// foldAll closes (open false) or opens every tag group in the Tags view, every Mod's copies
// in the Mods view. The cursor stays on its row; closing every group takes it to its group's
// header, and closing every Mod leaves the cursor's open (it never rests on a Mod's own row).
function foldAll(open) {
  if (!els) return;
  const x = L.flat[L.cur];
  if (L.view === "tags") {
    if (open) L.closed.clear();
    else for (const y of L.flat) if (y.t === "grp") L.closed.add(y.g.key);
  } else {
    L.open.clear();
    if (open) { for (const r of L.page.rows || []) if (r.insts.length > 1) L.open.add(r.id); }
    else if (x && x.t === "inst" && !x.lone) L.open.add(x.r.id);
  }
  const key = x && rowKey(x), gk = x && (x.t === "grp" ? x.g.key : x.gk);
  flatten();
  let at = key ? L.flat.findIndex((y) => rowKey(y) === key) : -1;
  if (at < 0 && gk != null) at = L.flat.findIndex((y) => y.t === "grp" && y.g.key === gk);
  L.cur = Math.max(0, at);
  land(1);
  resize();
  ensureVisible();
  follow();
  hints();
}

// setView switches between every mod and the Tags view, staying on the same mod (on a
// tag's header: that group's first mod).
async function setView(v) {
  if (L.view === v || !els) return;
  const x = L.flat[L.cur];
  const r = x && (x.t === "grp" ? x.g.rows[0] : x.r);
  const ref = x && x.t === "inst" ? x.i.ref : "";
  L.view = v;
  api.SetSetting(VIEW_KEY, v).catch(() => {});
  if (v === "tags") L.tags = await tagList();
  flatten();
  if (r) {
    const at = L.flat.findIndex((y) => y.t !== "grp" && y.r.id === r.id && (!ref || y.t !== "inst" || y.i.ref === ref));
    L.cur = at >= 0 ? at : 0;
  }
  draw();
  ensureVisible();
  els.rows.dataset.sig = "";
  drawRows();
}

function resize() {
  els.sizer.style.height = HEAD + L.flat.length * ROW + "px";
  els.rows.dataset.sig = "";
  drawRows();
}

// land: the cursor came onto a Lineage's row going dir (1 down, -1 up). A closed Lineage
// opens and the cursor takes its first copy (going up: its last); going up onto the row of
// an open one, the cursor passes it.
function land(dir) {
  for (let guard = 0; guard < 4; guard++) {
    const x = L.flat[L.cur];
    if (!x || x.t !== "lin") return;
    const was = isOpen(x.r);
    if (!was) { L.open.add(x.r.id); flatten(); if (els) resize(); }
    if (dir >= 0 || !was) { L.cur = Math.min(L.flat.length - 1, L.cur + (dir >= 0 ? 1 : x.r.insts.length)); return; }
    if (L.cur === 0) { L.cur = 1; return; }
    L.cur -= 1;
  }
}

function click(k, e) {
  const x = L.flat[k];
  L.focus = "rows";
  if (e.shiftKey) {
    if (L.anchor < 0) L.anchor = L.cur;
    L.sel.clear();
    const [a, b] = [Math.min(L.anchor, k), Math.max(L.anchor, k)];
    for (let j = a; j <= b; j++) for (const i of shownCopies(L.flat[j])) L.sel.add(i.ref);
  } else if (e.ctrlKey || e.metaKey) {
    toggleSel(x);
    L.anchor = k;
  } else L.anchor = k;
  L.cur = k;
  // a click on a Lineage's row opens it and takes its first copy
  if (x.t === "lin") { const was = isOpen(x.r); land(1); if (!was) ensureVisible(); }
  els.rows.dataset.sig = "";
  drawRows();
  drawBar();
  follow();
  markFocus();
  hints();
}

// copiesOf: a row's copies; a tag's header row stands for its group's (ctrl+click selects them),
// but ranges and ctrl+a take only the rows they cover.
function copiesOf(x) { return x.t === "inst" ? [x.i] : x.t === "grp" ? x.g.rows.flatMap((r) => r.insts) : x.r.insts; }
const shownCopies = (x) => (x.t === "grp" ? [] : copiesOf(x));
function toggleSel(x) {
  const cs = copiesOf(x);
  const all = cs.every((i) => L.sel.has(i.ref));
  for (const i of cs) all ? L.sel.delete(i.ref) : L.sel.add(i.ref);
}

function ensureVisible() {
  const box = els.rows;
  const y = L.cur * ROW; // below the sticky headers
  if (y < box.scrollTop) box.scrollTop = y;
  else if (HEAD + y + ROW > box.scrollTop + box.clientHeight) box.scrollTop = HEAD + y + ROW - box.clientHeight;
}

function move(d, extend) {
  if (!L.flat.length) return;
  const from = L.cur;
  L.cur = Math.max(0, Math.min(L.flat.length - 1, L.cur + d));
  land(L.cur === 0 && d < 0 ? 1 : Math.sign(d) || 1);
  if (extend) {
    if (L.anchor < 0) L.anchor = from;
    L.sel.clear();
    const [a, b] = [Math.min(L.anchor, L.cur), Math.max(L.anchor, L.cur)];
    for (let j = a; j <= b; j++) for (const i of shownCopies(L.flat[j])) L.sel.add(i.ref);
  } else L.anchor = -1;
  ensureVisible();
  els.rows.dataset.sig = "";
  drawRows();
  drawBar();
  follow();
  hints();
}

function selectAll() {
  L.sel.clear();
  for (const x of L.flat) for (const i of shownCopies(x)) L.sel.add(i.ref);
  els.rows.dataset.sig = "";
  drawRows();
  drawBar();
  hints();
}

// ---- the details: the cursor's copy, in the right column (enter: over the page) ----

// cursorCopy: the copy the cursor stands for: its row's, or on a Lineage's row the copy in
// the current plan, else the first.
function cursorCopy() {
  const x = L.flat[L.cur];
  if (!x) return null;
  if (x.t === "grp") return null;
  if (x.t === "inst") return x.i;
  return x.r.insts.find((i) => i.inPlan) || x.r.insts.find((i) => i.id) || x.r.insts[0] || null;
}
const shownCopy = () => cursorCopy();

// follow draws the cursor's copy in the details at once (each step asks Go straight away;
// an answer for a row the cursor has left is dropped).
function follow() { return drawDetail(); }

function setFull(v) {
  L.full = v;
  els.root.classList.toggle("full", v);
  if (v) { L.focus = "detail"; els.scroll.focus(); } else { L.focus = "rows"; els.rows.focus(); }
  markFocus();
  ui.renderWhere();
  hints();
}

let detailSeq = 0, frame = null;
// drawDetail: the cursor's copy in the details, laid out as the editor's (detail.js
// detailFrame): its note, picture and name, where it comes from, Mod library's field list, its
// actions, the description and history (16 §2.9). A Workshop page not read yet is fetched
// straight away.
async function drawDetail() {
  if (!els) return;
  const x = L.flat[L.cur];
  if (L.view === "tags" && (!x || x.t === "grp")) return drawTagManager(x ? x.g : null);
  const i = cursorCopy() && findCopy(cursorCopy().ref);
  const box = els.scroll;
  const row = i && (L.page.rows || []).find((r) => r.insts.some((x) => x.ref === i.ref));
  els.dhead.innerHTML = "";
  els.dhead.append(el("span", "n", "3"), el("span", "t", T("details")));
  if (i) els.dhead.append(el("span", "q word-facts", (row ? row.name : "") + "  " + i.label));
  if (!i) { box.innerHTML = ""; box.dataset.ref = ""; frame = null; return; }
  const openFull = el("button", "editor-full-link", T("view full details") + "  Enter ↵");
  openFull.onmousedown = (e) => e.stopPropagation();
  openFull.onclick = (e) => { e.stopPropagation(); full("library", i.id || i.ref); };
  els.dhead.append(openFull);
  const same = frame && box.dataset.ref === i.ref && box.contains(frame.box);
  if (same && box.dataset.stamp === stampOf(i)) return;
  const n = ++detailSeq;
  const current = () => n === detailSeq && box.dataset.ref === i.ref;
  box.dataset.stamp = stampOf(i);
  if (!same) {
    // the copy shown stays, inert, until the next one's parts are in; then one swap (16 §4.1)
    box.dataset.ref = i.ref;
    // (the inner part: the scroll box itself takes the focus in details mode)
    stale(box.firstElementChild);
    const inner = el("div", "mod-detail");
    frame = detailFrame(inner, null, {
      ownFields: true,
      fieldActs: () => ({ folder: () => run("folder", [findCopy(i.ref) || i]), tags: () => run("tags", [findCopy(i.ref) || i]) }),
      run: (c, r) => run(c, [findCopy(r.ref) || r]),
      noteSaved: () => load(),
      noteLeave: () => els.rows.focus(),
      later: true,
      mount: () => { box.replaceChildren(inner); box.scrollTop = 0; },
    });
    frame.head(row ? row.name : i.label, "", i.id);
  }
  const f = frame;
  f.inst(i);
  const fields = api.LibraryFields(i.ref).then((fs) => { if (current()) f.fields(fs); }, (e) => { if (current()) f.error(errText(e)); });
  // a copy of another game's has no detail here: its fields say what there is
  const detail = !i.id ? null : api.Detail(i.id, false, false, false).then((d) => {
    if (!current()) return null;
    f.detail(d);
    return d;
  }, (e) => { if (current() && !/^no (mod|list)\b/.test(errText(e))) f.error(errText(e)); return null; });
  await f.settle([fields, detail]);
  if (!current()) return;
  f.show();
  const d = await detail;
  if (!current() || d?.state !== "not cached") return;
  f.fetching();
  const d2 = await api.Detail(i.id, false, true, false).catch(() => null);
  if (d2 && current()) f.detail(d2);
}
const stampOf = (i) => [i.note, i.auto, (i.ops || []).join(","), i.state, i.inPlan].join("|");

function findCopy(ref) {
  for (const r of L.page.rows || []) for (const i of r.insts) if (i.ref === ref) return i;
  return null;
}

// ---- the Tag manager: the details on a tag's header row in the Tags view (16 §2.9) ----

// drawTagManager lays the cursor's tag out as a copy's details are: its note, its name, how
// many mods have it, its fields (on top, similar tags), its actions (rename, merge into,
// delete), then every tag to go to. Tag changes are player data, outside the list's undo
// (16 §3.8), so merge and delete confirm first.
function drawTagManager(g) {
  const box = els.scroll;
  const ref = "tags:" + (g ? g.key : "-");
  const ae = document.activeElement;
  // a redraw (the window regaining focus, a scan) never takes text the player is typing
  if (box.dataset.ref === ref && box.contains(ae) && ae.matches("input, textarea")) return;
  // from one tag to another (a chip, the arrows) the panel keeps its place
  const same = (box.dataset.ref || "").startsWith("tags:"), top = box.scrollTop;
  frame = null;
  detailSeq++;
  box.dataset.ref = ref;
  box.dataset.stamp = "";
  const info = g && !g.none ? g.info || { tag: g.tag, n: g.rows.length, note: "", pinned: false } : null;
  els.dhead.innerHTML = "";
  els.dhead.append(el("span", "n", "3"), el("span", "t", T("tags")));
  if (g) els.dhead.append(el("span", "q word-facts", info ? info.tag : T("no tag")));
  if (info) {
    const only = el("button", "editor-full-link", T("filter to this tag"));
    only.onmousedown = (e) => e.stopPropagation();
    only.onclick = (e) => { e.stopPropagation(); bar().set("tags:" + info.tag, 1); };
    els.dhead.append(only);
  }
  const d = el("div", "mod-detail tag-detail");
  if (info) tagHead(d, g, info);
  else if (g) d.append(el("h1", "", T("no tag")), el("div", "facts", Tf("%d mods", g.rows.length)), el("p", "tm-hint", T("mods without tags: select them and press g to tag them")));
  else d.append(el("p", "tm-hint", T("no tags yet: select mods and press g to tag them")));
  allTags(d, g ? g.key : null);
  box.replaceChildren(d);
  box.scrollTop = same ? top : 0;
}

// tagHead: the tag's note, name, facts, fields and actions.
function tagHead(d, g, info) {
  d.append(noteEl({ id: info.tag, note: info.note || "" }, {
    placeholder: T("what this tag is for (shown in its group row)"),
    save: (text) => api.SetTagNote(info.tag, text),
    saved: async () => { await load(false); goToGroup(g.key); },
    leave: () => backToRows(),
  }));
  // the name, and keep on top (first in the tag bar and in this view) on its right
  const row = el("div", "tm-titlerow");
  const h = el("h1", "tm-name", info.tag);
  h.title = T("rename") + T("; ") + "F2";
  h.ondblclick = () => startRename();
  const pin = el("button", "fact", info.pinned ? T("stop keeping on top") : T("keep on top"));
  pin.onclick = () => tagAct(api.PinTag(info.tag, !info.pinned), g.key);
  row.append(h, pin);
  const facts = el("div", "facts");
  facts.append(el("span", "", Tf("%d mods", info.n)));
  if (info.n !== g.rows.length) facts.append(el("span", "", Tf("%d shown", g.rows.length)));
  if (info.pinned) facts.append(el("span", "", T("on top")));
  d.append(row, facts);
  // similar tags, drawn as fields.js draws a copy's field: a label, the value, its action
  const like = similarTags(info.tag);
  if (like.length) {
    const r = el("div", "frow");
    const v = el("span", "v");
    v.append(...like.map((t) => el("span", "chip", t)));
    const b = el("button", "fact", T("merge in"));
    b.onclick = () => confirmMerge(like, info.tag);
    r.append(el("span", "k", T("similar")), v, b);
    const fields = el("div", "fields");
    fields.append(r);
    d.append(fields);
  }
  const acts = el("div", "lib-acts");
  const btn = (label, key, run, cls = "") => {
    const b = el("button", "btn" + cls, label);
    b.append(keyCaps(key));
    b.onclick = run;
    acts.append(b);
  };
  btn(T("rename"), "F2", () => startRename());
  btn(T("merge into…"), "M", () => pickMerge(info.tag));
  btn(T("delete tag"), "Del", () => confirmDelete([info.tag]), " danger");
  d.append(acts);
}

// allTags: every tag with mods, as chips in the groups' order; a click goes to its group,
// ctrl+click picks several (the cursor's tag first, as a file list's selection would), and
// a bar under them merges, deletes or clears them. Tags kept on top that no mod has are
// named after them.
function allTags(d, curKey) {
  const used = L.tags.filter((t) => t.n), idle = L.tags.filter((t) => !t.n);
  if (!used.length && !idle.length) return;
  for (const k of L.picked) if (!used.some((t) => t.tag.toLowerCase() === k)) L.picked.delete(k);
  const sub = el("div", "sub", T("all tags"));
  sub.append(el("span", "cnt", String(used.length)));
  const cloud = el("div", "tm-cloud");
  for (const t of used) {
    const k = t.tag.toLowerCase();
    const c = el("button", "tm-chip" + (k === curKey ? " cur" : "") + (L.picked.has(k) ? " picked" : ""));
    c.append(el("span", "", t.tag), el("span", "n", String(t.n)));
    c.title = (t.note ? t.tag + "\n" + t.note : t.tag) + "\n" + T("ctrl+click picks several");
    c.setAttribute("aria-pressed", String(L.picked.has(k)));
    c.onclick = (e) => {
      if (e.ctrlKey || e.metaKey) {
        if (!L.picked.size && curKey && curKey !== k && used.some((u) => u.tag.toLowerCase() === curKey)) L.picked.add(curKey);
        if (L.picked.has(k)) L.picked.delete(k); else L.picked.add(k);
        return drawDetail();
      }
      L.picked.clear();
      goToGroup(k);
    };
    cloud.append(c);
  }
  d.append(sub, cloud);
  if (L.picked.size) {
    const tags = used.filter((t) => L.picked.has(t.tag.toLowerCase())).map((t) => t.tag);
    const b = el("div", "tm-picked");
    b.append(el("span", "cnt", Tf("%d tags picked", tags.length)));
    const btn = (label, run, cls = "") => { const x = el("button", "btn" + cls, label); x.onclick = run; b.append(x); };
    btn(T("merge into…"), () => pop.tagTarget(Tf("merge %s into", tags.join(", ")), used, (into) => confirmMerge(tags, into)));
    btn(T("delete tags"), () => confirmDelete(tags), " danger");
    btn(T("clear the selection"), () => { L.picked.clear(); drawDetail(); }, " quiet");
    d.append(b);
  }
  if (idle.length) d.append(el("p", "tm-idle", Tf("on top, but no mod has them: %s", idle.map((t) => t.tag).join(", "))));
}

// similarTags: tags spelled almost the same (case, spaces, dashes, a plural s), or one a
// longer form of the other: candidates for a merge.
function similarTags(tag) {
  const norm = (s) => s.toLowerCase().replace(/[\s_\-./]+/g, "").replace(/s$/, "");
  const n = norm(tag);
  return L.tags.filter((t) => {
    if (!t.n || t.tag.toLowerCase() === tag.toLowerCase()) return false;
    const m = norm(t.tag);
    if (m === n) return true;
    const [a, b] = m.length < n.length ? [m, n] : [n, m];
    return a.length >= 4 && b.startsWith(a);
  }).map((t) => t.tag);
}

// startRename turns the tag's name into a field: enter renames, esc (or leaving it) does not.
function startRename() {
  const h = els.scroll.querySelector(".tag-detail .tm-name");
  const g = L.flat[L.cur]?.g;
  if (!h || !g || g.none) return;
  const from = h.textContent;
  const inp = el("input", "tm-title");
  inp.value = from;
  inp.spellcheck = false;
  inp.setAttribute("aria-label", T("rename"));
  let over = false;
  const end = () => { if (over) return; over = true; inp.replaceWith(h); };
  inp.onkeydown = (e) => {
    if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); const to = inp.value; end(); renameTag(from, to); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); end(); backToRows(); }
  };
  inp.onblur = end;
  h.replaceWith(inp);
  L.focus = "detail";
  markFocus();
  inp.focus();
  inp.select();
}

// editTagNote opens the tag's note as a click on it does (n).
function editTagNote() {
  const n = els.scroll.querySelector(".tag-detail .lib-note");
  if (!n) return;
  L.focus = "detail";
  markFocus();
  n.click();
}

function pickMerge(tag) {
  const others = L.tags.filter((t) => t.n && t.tag.toLowerCase() !== tag.toLowerCase());
  pop.tagTarget(Tf("merge %s into", tag), others, (into) => confirmMerge([tag], into));
}

async function confirmMerge(from, into) {
  let p;
  try { p = await api.TagPreview(from, into); } catch (e) { return say("fail", errText(e)); }
  const lines = [
    p.from.join(", ") + "  →  " + p.into,
    p.both ? Tf("%d mods change to %s (%d already had it)", p.lineages, p.into, p.both) : Tf("%d mods change to %s", p.lineages, p.into),
    T("their notes are joined; tag changes cannot be undone"),
  ];
  pop.confirm(T("merge tags"), lines, T("merge"), () => tagAct(api.MergeTags(p.from, p.into), p.into.toLowerCase()), "");
}

async function confirmDelete(tags) {
  let p;
  try { p = await api.TagPreview(tags, ""); } catch (e) { return say("fail", errText(e)); }
  const lines = [
    Tf("remove %s from %d mods", p.from.join(", "), p.lineages),
    T("mod files and lists are not touched; tag changes cannot be undone"),
  ];
  pop.confirm(tags.length > 1 ? T("delete tags") : T("delete tag"), lines, T("delete"), () => tagAct(api.DeleteTags(p.from), null), "");
}

function renameTag(from, to) {
  to = to.trim();
  if (!to || to === from) return backToRows();
  const other = L.tags.find((t) => t.tag.toLowerCase() === to.toLowerCase() && t.tag.toLowerCase() !== from.toLowerCase());
  if (other) return confirmMerge([from], other.tag); // the name is taken: that is a merge
  tagAct(api.RenameTag(from, to), to.toLowerCase());
}

// tagAct runs a tag change, reads the page again and puts the cursor on the group of key
// (when there is one).
async function tagAct(p, key) {
  backToRows(); // first, so the redraw below is not held back by a field being typed in
  try { say("ok", await p); } catch (e) { say("fail", errText(e)); }
  L.picked.clear();
  await load(false);
  if (key != null) goToGroup(key);
}

function goToGroup(key) {
  const at = L.flat.findIndex((x) => x.t === "grp" && x.g.key === key);
  if (at < 0) return;
  L.cur = at;
  ensureVisible();
  els.rows.dataset.sig = "";
  drawRows();
  follow();
  hints();
}

function backToRows() {
  if (!els) return;
  L.focus = "rows";
  els.rows.focus();
  markFocus();
  hints();
}

// groupMenu: a tag header's menu (16 §8.5): what the Tag manager does, and every group
// open or closed.
function groupMenu(x, y, keys = false) {
  const g = L.flat[L.cur]?.g;
  if (!g) return;
  const acts = [];
  if (!g.none) {
    acts.push(
      { label: T("rename"), key: "F2", run: () => startRename() },
      { label: T("note"), key: "n", run: () => editTagNote() },
      { label: T("merge into…"), key: "M", run: () => pickMerge(g.tag) },
      { label: T("delete tag"), key: "Del", cls: "danger", run: () => confirmDelete([g.tag]) },
      { sep: true },
      { label: T("filter to this tag"), run: () => bar().set("tags:" + g.tag, 1) },
      { sep: true },
    );
  }
  acts.push(
    { label: T("collapse all"), run: () => foldAll(false) },
    { label: T("expand all"), run: () => foldAll(true) },
  );
  pop.menu(x, y, acts, keys, { name: g.none ? T("no tag") : g.tag, opener: els.rows });
}

// ---- ops: the selection's (the bar), else the cursor's ----

function selected() {
  if (L.sel.size) {
    const out = [];
    for (const r of L.page.rows || []) for (const i of r.insts) if (L.sel.has(i.ref)) out.push(i);
    return out;
  }
  const i = cursorCopy();
  return i ? [i] : [];
}

// opsOf: every op any of the copies has, each run on the copies that have it; the ops of
// one row at a time only for one copy.
function opsOf(copies) {
  const by = new Map();
  for (const i of copies) for (const c of i.ops || []) {
    if (!by.has(c)) by.set(c, []);
    by.get(c).push(i);
  }
  for (const [c] of by) if (copies.length > 1 && ops.op(c).one) by.delete(c);
  return by;
}

function drawBar() {
  const bar = els.bar;
  if (!L.sel.size) { bar.hidden = true; return; }
  bar.hidden = false;
  bar.innerHTML = "";
  bar.append(el("span", "accent", Tf("selected %d", L.sel.size)));
  const by = opsOf(selected());
  for (const c of ops.sorted([...by.keys()])) {
    const o = ops.op(c), copies = by.get(c);
    const b = el("button", "btn" + (o.danger ? " danger" : ""), o.label);
    if (copies.length < L.sel.size) b.append(el("span", "faint", " " + copies.length));
    if (o.group !== 3) b.dataset.envWrite = c;
    if (o.key) b.append(keyCaps(o.key));
    b.onclick = () => run(c, copies);
    bar.append(b);
  }
  const x = el("button", "btn quiet", T("clear the selection"));
  x.append(keyCaps("esc"));
  x.onclick = () => { L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); drawBar(); hints(); };
  bar.append(x);
}

// menuAt: the row menu (16 §8.5), the same groups as the editor's.
function menuAt(x, y, keys = false) {
  const by = opsOf(selected());
  ops.openMenu(x, y, [...by.keys()], (c) => run(c, by.get(c)), keys, { name: L.flat[L.cur]?.r?.name, opener: els.rows });
}

// menuAtCursor: the menu key or shift+F10 open the menu at the cursor's row.
function menuAtCursor() {
  const r = els.rows.querySelector(".lib-row.cur");
  const b = r ? r.getBoundingClientRect() : els.rows.getBoundingClientRect();
  if (L.flat[L.cur]?.t === "grp" && !L.sel.size) return groupMenu(b.left + 40, b.bottom, true);
  menuAt(b.left + 40, b.bottom, true);
}

// byKey runs the op key k stands for on the object, if it has one.
function byKey(k) {
  const by = opsOf(selected());
  const c = ops.keyed([...by.keys()], k);
  if (c) return run(c, by.get(c));
  const i = cursorCopy();
  if (k === "ctrl+w" || k === "ctrl+s") return say("", T("no Workshop ID: not a Workshop mod"));
  if (k === "ctrl+o") return say("", i && i.id ? T("the folder is not there") : T("RimArcho has not seen this copy: no folder"));
  say("", T("nothing to do with this row"));
}

async function run(code, copies) {
  const refs = copies.map((i) => i.ref);
  const i0 = copies[0];
  const o = ops.op(code);
  if (S.busy.env && o.group !== 3) return say("busy", Tf("switching to %s", S.busy.env));
  const done = async (p) => {
    try { const t = await p; say("ok", typeof t === "string" ? t : t && t.text); } catch (e) { say("fail", errText(e)); }
    L.sel.clear();
    await load(false, code === "separate" ? i0.ref : "");
    ui.reload();
  };
  switch (code) {
    case "subscription-update":
      if (selected().length === 1 && i0.subscriptionKind === "page") return updatePage(i0.ref);
      break;
    case "folder": return api.OpenFolder(i0.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
    case "steam": return api.OpenSteamWorkshop(i0.id || i0.ref).then((t) => say("ok", t), (e) => say("fail", errText(e)));
    case "workshop":
      if (i0.id) return api.OpenWorkshop(i0.id).then((t) => say("ok", t), (e) => say("", errText(e)));
      return api.OpenURL(i0.page).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
    case "tags": {
      const row = (L.page.rows || []).find((r) => r.insts.some((x) => x.ref === i0.ref));
      return pop.tags(copies.filter((i) => i.id).map((i) => ({ id: i.id, name: row ? row.name : i.label })), () => load());
    }
    case "note": {
      if (!L.full && innerWidth <= 760) setFull(true);
      const n = els.scroll.querySelector(".lib-note");
      if (n) n.click();
      return;
    }
  }
  if (!o.confirm) return done(api.LibraryDo(code, refs));
  if (code === "subscription-remove") return pop.libraryConfirm(code, refs, o.label, done);
  let p;
  try { p = await api.LibraryPreview(code, refs); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, o.label, p.apply ? () => done(api.LibraryDo(code, pop.previewOk(p, refs))) : null);
}

// ---- keys (16 §3.5): the plan editor's table; the arrows only move ----

export function key(e) {
  if (pageLoading()) return;
  const k = e.key;
  const ctrl = e.ctrlKey || e.metaKey;
  const x = L.flat[L.cur];
  const done = () => e.preventDefault();
  if (!els) return;
  if (k === "Escape" && subscriptionPanel?.isConnected) { done(); subscriptionPanel.remove(); subscriptionPanel = null; els.rows.focus(); return; }
  if (ops.menuKey(e)) { done(); return menuAtCursor(); }
  // the details hold the focus: the arrows scroll them, ← (or esc) goes back to the rows
  if (L.focus === "detail" && (k === "ArrowDown" || k === "ArrowUp" || k === "PageDown" || k === "PageUp" || k === "j" || k === "k")) {
    done();
    const d = k === "PageDown" || k === "PageUp" ? els.scroll.clientHeight * 0.9 : 40;
    els.scroll.scrollTop += (k === "ArrowDown" || k === "PageDown" || k === "j") ? d : -d;
    return;
  }
  if (L.focus === "detail" && (k === "ArrowLeft" || k === "h") && !L.full) { done(); L.focus = "rows"; els.rows.focus(); markFocus(); return hints(); }
  // a tag's header row (Tags view): ← → close and open it, → on an open one goes to the
  // Tag manager; F2, n, M, Del act on the tag (16 §2.9)
  if (x && x.t === "grp" && !ctrl && !e.altKey) {
    const g = x.g;
    if (k === "ArrowLeft" || k === "h") { done(); if (!L.closed.has(g.key)) toggleGroup(g.key); return; }
    if (k === "ArrowRight" || k === "l") {
      done();
      if (L.closed.has(g.key)) return toggleGroup(g.key);
      L.focus = "detail"; els.scroll.focus(); markFocus(); return hints();
    }
    if (k === "Enter") { done(); return toggleGroup(g.key); }
    if (!g.none && k === "F2") { done(); return startRename(); }
    if (!g.none && k === "n") { done(); return editTagNote(); }
    if (!g.none && k === "M") { done(); return pickMerge(g.tag); }
    if (!g.none && k === "Delete" && !L.sel.size) { done(); return confirmDelete([g.tag]); }
  }
  if (k === "ArrowDown" || k === "j") { done(); return move(1, e.shiftKey); }
  if (k === "ArrowUp" || k === "k") { done(); return move(-1, e.shiftKey); }
  const page = Math.max(1, Math.floor(els.rows.clientHeight / ROW) - 1);
  if (k === "PageDown") { done(); return move(page, e.shiftKey); }
  if (k === "PageUp") { done(); return move(-page, e.shiftKey); }
  if (k === "Home") { done(); return move(-1e9, e.shiftKey); }
  if (k === "End") { done(); return move(1e9, e.shiftKey); }
  if (k === "ArrowRight" || k === "l") {
    done();
    if (!x) return;
    if (x.t === "lin") { land(1); ensureVisible(); els.rows.dataset.sig = ""; drawRows(); return follow(); }
    L.focus = "detail"; els.scroll.focus(); markFocus(); return hints();
  }
  if (k === "ArrowLeft" || k === "h") {
    done();
    if (!x) return;
    // ← on a copy closes its Lineage (when it can close: the filters may show only some)
    if (x.t === "inst" && !x.lone && L.open.has(x.r.id)) return toggle(x.r);
    // in the Tags view, otherwise to the tag's header row
    if (x.gk != null) { const h = L.flat.findLastIndex((y, j) => j < L.cur && y.t === "grp"); if (h >= 0) goToGroup(L.flat[h].g.key); }
    return;
  }
  // enter: the full detail, the same page the editor opens (16 §4.2); in a narrow window
  // the details still lie over the table first (→)
  if (k === "Enter") {
    done();
    if (L.full) return setFull(false);
    const i = cursorCopy();
    if (i && (i.id || i.ref)) return full("library", i.id || i.ref);
    return;
  }
  if (k === "Escape") {
    done();
    if (L.full) return setFull(false);
    if (L.focus === "detail") { L.focus = "rows"; els.rows.focus(); markFocus(); return hints(); }
    if (L.sel.size) { L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); drawBar(); return hints(); }
    // the filter bar: one press clears it and closes it (G5)
    if (L.bar.active() || L.bar.open) { if (L.bar.active()) L.bar.clear(); if (L.bar.open) L.bar.toggle(false); }
    return;
  }
  if (ctrl && (k === " " || k === "Spacebar")) {
    done();
    if (x) { toggleSel(x); els.rows.dataset.sig = ""; drawRows(); drawBar(); hints(); }
    return;
  }
  if (ctrl && k.toLowerCase() === "a") { done(); return selectAll(); }
  // ctrl+o / ctrl+w: the cursor's copy's folder and Workshop page (16 §3.5); the Wails
  // window does not take them, preventDefault keeps them from the browser
  if (ctrl && !e.shiftKey && !e.altKey && ["o", "w", "s"].includes(k.toLowerCase())) { done(); return byKey("ctrl+" + k.toLowerCase()); }
  if (k === "/" || (ctrl && k.toLowerCase() === "f")) { done(); return L.bar.focus(); }
  if (k === "\\") { done(); return L.bar.toggle(); }
  if (ctrl || e.altKey) return;
  if (k === "v") { done(); return setView(L.view === "tags" ? "mods" : "tags"); }
  const keys = { g: "g", n: "n", Delete: "Del" };
  if (keys[k]) { done(); return byKey(keys[k]); }
  if (table.key(e)) return;
  if (k === "C") { done(); const i = cursorCopy(); if (i && i.id) document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: i.id, name: (L.flat[L.cur] || {}).r ? L.flat[L.cur].r.name : i.label } })); return; }
  if (k === "R") { done(); return ui.cmd("rescan"); }
}

// stepCopy is the full detail's ← →: the cursor moves one row and the page shows that row's
// copy (its id, or its ref when RimArcho never saw it).
export function stepCopy(d) {
  if (!L.page || !L.flat.length) return "";
  const from = L.cur;
  L.cur = Math.max(0, Math.min(L.flat.length - 1, L.cur + d));
  // a tag's header row has no copy to show: step past it (back where it was at an end)
  while (L.flat[L.cur]?.t === "grp") {
    const next = L.cur + (d < 0 ? -1 : 1);
    if (next < 0 || next >= L.flat.length) { L.cur = from; break; }
    L.cur = next;
  }
  land(d < 0 && L.cur > 0 ? -1 : 1);
  const i = cursorCopy();
  return i ? i.id || i.ref : "";
}

// where: the position line on this page (16 §3.2): the cursor's row of the rows shown.
export function where() {
  if (!L.page) return "";
  let s = `${T("Mod library")}    ${L.flat.length ? L.cur + 1 : 0} / ${L.flat.length}`;
  if (L.sel.size) s += "  " + Tf("%d selected", L.sel.size);
  if (L.full) s += " › " + T("details");
  return s;
}

// hintList: the hint line for the page (16 §2.2): what the object can do now, as the editor.
export function hintList() {
  const list = [];
  const x = L.flat[L.cur];
  const other = { key: "v", label: L.view === "tags" ? T("every mod") : T("group by tag") };
  if (x && x.t === "grp" && !L.sel.size) {
    list.push({ key: "← →", label: T("open / close") });
    if (!x.g.none) list.push({ key: "F2", label: T("rename") }, { key: "n", label: T("note") }, { key: "M", label: T("merge into…") }, { key: "Del", label: T("delete tag") });
    list.push(other, { key: "?", label: T("all keys") });
    return list;
  }
  const codes = [...opsOf(selected()).keys()];
  if (x && x.t === "inst" && !x.lone) list.push({ key: "←", label: T("open / close") });
  list.push({ key: "enter", label: L.full ? T("back") : T("details") });
  if (codes.includes("folder") || codes.includes("workshop")) list.push({ key: "ctrl+o", label: T("open the folder") }, { key: "ctrl+w", label: T("Workshop page") });
  if (codes.includes("steam")) list.push({ key: "ctrl+s", label: T("steam page") });
  list.push({ key: "/", label: T("search") }, { key: "\\", label: T("filter") }, other);
  list.push({ key: "?", label: T("all keys") });
  return list;
}
function hints() { ui.renderHints(); ui.renderWhere(); }
