// Mod 库存 › 全部 (06 §2.9): what is on this machine. The same layout as the plan editor:
// the filter bar on top of a table, the details in a column on the right. The table is like
// a file manager's details view: one row per Lineage (opening to its copies), columns the
// player sorts (click a header, again reverses), moves (drag a header), sizes (drag its
// edge) and picks (right click the headers), kept in ui.gui.library.columns. The details
// follow the cursor; enter opens them over the page and closes them again. What it shows
// comes from view.BuildLibrary through API.Library; each copy carries only the codes of
// its ops (view.CopyOps), the labels and keys are the page's op table, sent once. The table
// draws only the rows in view, so 1350+ Lineages stay smooth.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { on } from "./api.js";
import { icon } from "./icons.js";
import { detailFrame } from "./detail.js";
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
};
let ui = null;
let els = null;
let subscriptionPanel = null;

export function initLibrary(u) {
  ui = u;
  // A scan, or another process (the CLI, an agent), changes what is there.
  on("scan", () => { if (S.screen === "library") load(); });
  addEventListener("focus", () => { if (S.screen === "library") load(); });
  // a background task on the copies ended (06 §2.16): what is there changed
  document.addEventListener("task-ended", (e) => {
    if (S.screen !== "library") return;
    const ref = e.detail?.ref;
    if (ref) { const b = bar(); b.search = ""; b.filters = []; b.input.value = ""; L.sel = new Set([ref]); }
    load(false, ref);
  });
  addEventListener("resize", () => { if (S.screen === "library") drawRows(); });
}

// open shows the page (the sidebar's Mod 库存 › 全部).
export async function open() {
  if (S.screen !== "library") { L.full = false; L.focus = "rows"; }
  const read = beginPageRead("library", { retry: open });
  await load(true, undefined, read);
}

// openSearch shows the page searching for q (a background task's mod, 06 §2.16).
export async function openSearch(q) {
  const b = bar();
  b.filters = [];
  b.search = q;
  b.input.value = q;
  L.cur = 0;
  await open();
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
      placeholder: T("search: words, tags:, source:, content:") + "  /",
    });
  }
  return L.bar;
}

async function load(fresh, focusRef, read = null) {
  if (!read && S.screen !== "library") return;
  read ||= beginPageRead("library", { retry: () => load(fresh, focusRef) });
  const b = bar();
  let p, saved;
  try {
    [p, saved] = await Promise.all([api.Library(b.search, b.filters, L.sort), L.cols ? null : api.Setting(COLS_KEY).catch(() => "")]);
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  L.page = p;
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

// flatten lists the rows: a Lineage, and its copies when it is open (or when the filters
// show only some of them); a Lineage of one copy is that copy's row.
function flatten() {
  const keep = L.flat[L.cur] && rowKey(L.flat[L.cur]);
  const out = [];
  for (const r of L.page.rows || []) {
    if (r.single) { out.push({ t: "inst", r, i: r.insts[0], lone: true }); continue; }
    out.push({ t: "lin", r });
    if (L.open.has(r.id) || r.insts.length < r.copies) for (const i of r.insts) out.push({ t: "inst", r, i });
  }
  L.flat = out;
  const at = keep ? out.findIndex((x) => rowKey(x) === keep) : -1;
  L.cur = at >= 0 ? at : Math.min(L.cur, Math.max(out.length - 1, 0));
  // a selected copy that is no longer listed is no longer selected
  const shown = new Set(out.filter((x) => x.t === "inst").map((x) => x.i.ref));
  for (const k of [...L.sel]) if (!shown.has(k) && !lineageHas(k)) L.sel.delete(k);
}
const rowKey = (x) => (x.t === "lin" ? "L|" + x.r.id : "I|" + x.i.ref);
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
  // the left column, as the editor's 未启用: the filter bar on top, then the details (the
  // editor's detail card) filling the rest (06 §2.9)
  const detail = el("section", "area detail lib-detail");
  const dhead = el("div", "head");
  const scroll = el("div", "scroll");
  scroll.tabIndex = -1;
  detail.append(dhead, scroll);
  // the filter bar and the details are two cards, one above the other (06 §2.9)
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
  // 1 the filter, 2 the table, 3 the details (06 §3.5); the numbers sit in their headings
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
  h.append(el("span", "lib-count", n), el("span", "faint", p.size));
  table.draw(els.rows, els.th);
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
  for (const n of [...box.querySelectorAll(".lib-row")]) n.remove();
  if (!L.flat.length) {
    const p = L.page;
    box.append(el("div", "lib-row empty", p.total ? T("no mod matches") : T("no mods yet (R scans the mod folders)")));
    return;
  }
  const frag = document.createDocumentFragment();
  for (let k = a; k < b; k++) frag.append(rowEl(L.flat[k], k));
  box.append(frag);
}

function rowEl(x, k) {
  const r = el("div", "lib-row " + x.t + (x.lone ? " lone" : "") + (k === L.cur ? " cur" : ""));
  r.style.top = HEAD + k * ROW + "px";
  const selected = x.t === "inst" ? L.sel.has(x.i.ref) : x.r.insts.every((i) => L.sel.has(i.ref)) && x.r.insts.length;
  if (selected) r.classList.add("sel");
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
  r.onclick = (e) => click(k, e);
  r.ondblclick = () => { if (x.t === "lin") toggle(x.r); };
  r.oncontextmenu = (e) => {
    e.preventDefault();
    if (!selected) { L.cur = k; L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); follow(); drawBar(); }
    menuAt(e.clientX, e.clientY);
  };
  return r;
}

// nameCell: only the name (06 §2.9): a Lineage's, or under an open Lineage the copy's folder.
function nameCell(x) {
  const left = el("span", "c name");
  left.append(el("span", "m"));
  if (x.t === "lin") {
    const c = el("span", "chev" + (L.open.has(x.r.id) || x.r.insts.length < x.r.copies ? " down" : ""));
    c.innerHTML = icon.chev;
    c.onclick = (e) => { e.stopPropagation(); toggle(x.r); };
    left.append(c, el("span", "t", x.r.name));
  } else if (x.lone) {
    left.append(el("span", "t", x.r.name));
  } else {
    left.append(el("span", "tree", "└"), el("span", "t kind", x.i.label || x.i.name));
  }
  return left;
}

const isOpen = (r) => L.open.has(r.id) || r.insts.length < r.copies;

// toggle opens or closes a Lineage. The cursor never rests on a Lineage's own row: closing
// the one it is in moves it on to the next row (06 §2.9).
function toggle(r) {
  const x = L.flat[L.cur];
  const inside = x && x.r === r;
  if (L.open.has(r.id)) L.open.delete(r.id); else L.open.add(r.id);
  flatten();
  if (inside && !isOpen(r)) {
    const h = L.flat.findIndex((y) => y.t === "lin" && y.r.id === r.id);
    if (h >= 0) { L.cur = h + 1 < L.flat.length ? h + 1 : Math.max(0, h - 1); land(L.cur > h ? 1 : -1); }
  }
  resize();
  ensureVisible();
  follow();
  hints();
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
    for (let j = a; j <= b; j++) for (const i of copiesOf(L.flat[j])) L.sel.add(i.ref);
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

function copiesOf(x) { return x.t === "inst" ? [x.i] : x.r.insts; }
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
    for (let j = a; j <= b; j++) for (const i of copiesOf(L.flat[j])) L.sel.add(i.ref);
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
  for (const x of L.flat) for (const i of copiesOf(x)) L.sel.add(i.ref);
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
// detailFrame): its note, picture and name, where it comes from, Mod 库存's field list, its
// actions, the description and history (06 §2.9). A Workshop page not read yet is fetched
// straight away.
async function drawDetail() {
  if (!els) return;
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
    box.dataset.ref = i.ref;
    const inner = el("div", "mod-detail");
    box.replaceChildren(inner);
    box.scrollTop = 0;
    frame = detailFrame(inner, null, {
      ownFields: true,
      fieldActs: () => ({ folder: () => run("folder", [findCopy(i.ref) || i]), tags: () => run("tags", [findCopy(i.ref) || i]) }),
      run: (c, r) => run(c, [findCopy(r.ref) || r]),
      noteSaved: () => load(),
      noteLeave: () => els.rows.focus(),
    });
    frame.head(row ? row.name : i.label, "", i.id);
  }
  const f = frame;
  f.inst(i);
  api.LibraryFields(i.ref).then((fs) => { if (current()) f.fields(fs); }, (e) => { if (current()) f.error(errText(e)); });
  if (!i.id) return;
  try {
    const d = await api.Detail(i.id, false, false, false);
    if (!current()) return;
    f.detail(d);
    if (d.state === "not cached") {
      f.fetching();
      const d2 = await api.Detail(i.id, false, true, false).catch(() => null);
      if (d2 && current()) f.detail(d2);
    }
  } catch (e) {
    // a copy of another game's has no detail here: its fields say what there is
    if (current() && !/^no (mod|list)\b/.test(errText(e))) f.error(errText(e));
  }
}
const stampOf = (i) => [i.note, i.auto, (i.ops || []).join(","), i.state, i.inPlan].join("|");

function findCopy(ref) {
  for (const r of L.page.rows || []) for (const i of r.insts) if (i.ref === ref) return i;
  return null;
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
    if (o.key) b.append(el("kbd", "", o.key));
    b.onclick = () => run(c, copies);
    bar.append(b);
  }
  const x = el("button", "btn quiet", T("clear the selection"));
  x.append(el("kbd", "", "esc"));
  x.onclick = () => { L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); drawBar(); hints(); };
  bar.append(x);
}

// menuAt: the row menu (06 §8.5), the same groups as the editor's.
function menuAt(x, y, keys = false) {
  const by = opsOf(selected());
  ops.openMenu(x, y, [...by.keys()], (c) => run(c, by.get(c)), keys);
}

// menuAtCursor: the menu key or shift+F10 open the menu at the cursor's row.
function menuAtCursor() {
  const r = els.rows.querySelector(".lib-row.cur");
  const b = r ? r.getBoundingClientRect() : els.rows.getBoundingClientRect();
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
  pop.confirm(p.title, p.lines, o.label, p.apply ? () => done(api.LibraryDo(code, refs)) : null);
}

// ---- keys (06 §3.5): the plan editor's table; the arrows only move ----

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
    return;
  }
  // enter: the full detail, the same page the editor opens (06 §4.2); in a narrow window
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
  // ctrl+o / ctrl+w: the cursor's copy's folder and Workshop page (06 §3.5); the Wails
  // window does not take them, preventDefault keeps them from the browser
  if (ctrl && !e.shiftKey && !e.altKey && ["o", "w", "s"].includes(k.toLowerCase())) { done(); return byKey("ctrl+" + k.toLowerCase()); }
  if (k === "/" || (ctrl && k.toLowerCase() === "f")) { done(); return L.bar.focus(); }
  if (k === "\\") { done(); return L.bar.toggle(); }
  if (ctrl || e.altKey) return;
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
  L.cur = Math.max(0, Math.min(L.flat.length - 1, L.cur + d));
  land(d < 0 && L.cur > 0 ? -1 : 1);
  const i = cursorCopy();
  return i ? i.id || i.ref : "";
}

// where: the position line on this page (06 §3.2): the cursor's row of the rows shown.
export function where() {
  if (!L.page) return "";
  let s = `${T("Mod library")}    ${L.flat.length ? L.cur + 1 : 0} / ${L.flat.length}`;
  if (L.sel.size) s += "  " + Tf("%d selected", L.sel.size);
  if (L.full) s += " › " + T("details");
  return s;
}

// hintList: the hint line for the page (06 §2.2): what the object can do now, as the editor.
export function hintList() {
  const list = [];
  const x = L.flat[L.cur];
  const codes = [...opsOf(selected()).keys()];
  if (x && x.t === "inst" && !x.lone) list.push({ key: "←", label: T("open / close") });
  list.push({ key: "enter", label: L.full ? T("back") : T("details") });
  if (codes.includes("folder") || codes.includes("workshop")) list.push({ key: "ctrl+o", label: T("open the folder") }, { key: "ctrl+w", label: T("Workshop page") });
  if (codes.includes("steam")) list.push({ key: "ctrl+s", label: T("steam page") });
  list.push({ key: "/", label: T("search") }, { key: "\\", label: T("filter") });
  list.push({ key: "?", label: T("all keys") });
  return list;
}
function hints() { ui.renderHints(); ui.renderWhere(); }
