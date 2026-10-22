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
import { renderDetail } from "./detail.js";
import { fieldList } from "./fields.js";
import * as pop from "./popups.js";
import { openScreen, page, rescanButton } from "./screens.js";
import { createBar } from "./filterbar.js";
import * as ops from "./ops.js";

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
  seq: 0,
  cols: null,        // [{key, title, shown, w}], in the player's order
};
let ui = null;
let els = null;

export function initLibrary(u) {
  ui = u;
  // A scan, or another process (the CLI, an agent), changes what is there.
  on("scan", () => { if (S.screen === "library") load(); });
  addEventListener("focus", () => { if (S.screen === "library") load(); });
  addEventListener("resize", () => { if (S.screen === "library") drawRows(); });
}

// open shows the page (the sidebar's Mod 库存 › 全部).
export async function open() {
  if (S.screen !== "library") { L.full = false; L.focus = "rows"; }
  await load(true);
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
      placeholder: T("search: words, tag:, state:, source:, used:, -not") + "  /",
    });
  }
  return L.bar;
}

async function load(fresh) {
  const n = ++L.seq;
  const b = bar();
  let p, saved;
  try {
    [p, saved] = await Promise.all([api.Library(b.search, b.filters, L.sort), L.cols ? null : api.Setting(COLS_KEY).catch(() => "")]);
  } catch (e) { say("fail", errText(e)); return; }
  if (n !== L.seq) return;
  L.page = p;
  ops.setTable(p.opTable);
  if (!L.cols) L.cols = mergeCols(p.columns || [], saved);
  flatten();
  if (fresh || !els || S.screen !== "library" || !document.body.contains(els.root)) build();
  else draw();
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
  rows.addEventListener("scroll", drawRows, { passive: true });
  rows.addEventListener("mousedown", () => { L.focus = "rows"; rows.focus(); markFocus(); });
  scroll.addEventListener("mousedown", () => { L.focus = "detail"; markFocus(); });
  rows.addEventListener("contextmenu", (e) => { if (!e.target.closest(".lib-row")) e.preventDefault(); });
  draw();
  rows.focus();
}

// The left column's width: drag the split (kept in ui.gui.library.side), double click for
// the default.
const SIDE_KEY = "ui.gui.library.side";
let sideW = null;
function wireSplit(sp, root) {
  if (sideW == null) api.Setting(SIDE_KEY).then((v) => { sideW = +v || 0; applySide(root); }).catch(() => { sideW = 0; });
  else applySide(root);
  sp.addEventListener("mousedown", (e) => {
    e.preventDefault();
    const x0 = e.clientX, w0 = els.detail.getBoundingClientRect().width;
    sp.classList.add("drag");
    const move = (m) => { sideW = Math.max(240, Math.min(root.clientWidth - 360, Math.round(w0 + m.clientX - x0))); applySide(root); };
    const up = () => {
      sp.classList.remove("drag");
      removeEventListener("mousemove", move);
      removeEventListener("mouseup", up);
      api.SetSetting(SIDE_KEY, String(sideW)).catch(() => {});
      drawRows();
    };
    addEventListener("mousemove", move);
    addEventListener("mouseup", up);
  });
  sp.addEventListener("dblclick", () => { sideW = 0; applySide(root); api.SetSetting(SIDE_KEY, "").catch(() => {}); });
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

// mergeCols: the page's columns in the saved order with the saved widths; a column the
// saved list does not know (a new one) keeps its default place at the end.
function mergeCols(page, saved) {
  let keep = [];
  try { keep = JSON.parse(saved || "[]") || []; } catch { keep = []; }
  const by = new Map(page.map((c) => [c.key, c]));
  const out = [];
  for (const k of keep) {
    const c = by.get(k.k);
    if (!c) continue;
    by.delete(k.k);
    out.push({ key: c.key, title: c.title, shown: c.key === "name" || !k.h, w: +k.w || WIDTH[c.key] || 100 });
  }
  for (const c of by.values()) out.push({ key: c.key, title: c.title, shown: c.shown, w: WIDTH[c.key] || 100 });
  return out;
}

function saveCols() {
  const v = JSON.stringify(L.cols.map((c) => (c.shown ? { k: c.key, w: c.w } : { k: c.key, w: c.w, h: 1 })));
  api.SetSetting(COLS_KEY, v).catch(() => {});
}

function shownCols() { return L.cols.filter((c) => c.shown); }

// layout sets the grid for the headers and every row; the table scrolls sideways when the
// columns are wider than the card.
function layout() {
  const cs = shownCols();
  const tpl = cs.map((c) => (c.key === "name" ? `minmax(${c.w}px, 1fr)` : c.w + "px")).join(" ");
  const min = cs.reduce((n, c) => n + c.w, 0) + 10 * (cs.length - 1) + 24;
  els.rows.style.setProperty("--lib-cols", tpl);
  els.rows.style.setProperty("--lib-minw", min + "px");
}

function sortKey() { return L.sort.replace(/^-/, ""); }

function drawHead() {
  const p = L.page;
  const h = els.head;
  h.innerHTML = "";
  const n = p.shown === p.total ? Tf("%d mods", p.total) : Tf("%d of %d mods", p.shown, p.total);
  h.append(el("span", "lib-count", n), el("span", "faint", " · " + p.size));
  layout();
  const th = els.th;
  th.innerHTML = "";
  th.title = T("drag to move · right click: columns");
  for (const c of shownCols()) {
    const cell = el("div", "th " + c.key + (sortKey() === c.key ? " on" : ""));
    cell.append(el("span", "lbl", c.title));
    if (sortKey() === c.key) {
      const up = ASC.has(c.key) !== L.sort.startsWith("-");
      cell.append(el("span", "arrow", up ? "▲" : "▼"));
    }
    const rz = el("span", "rz");
    rz.onpointerdown = (e) => resize(e, c);
    cell.append(rz);
    cell.onpointerdown = (e) => { if (e.button === 0 && e.target !== rz) drag(e, c, cell); };
    th.append(cell);
  }
  th.oncontextmenu = (e) => { e.preventDefault(); colMenu(e.clientX, e.clientY); };
}

// sortBy: a click on a header sorts by it; again reverses.
function sortBy(key) {
  L.sort = L.sort === key ? "-" + key : key;
  L.cur = 0;
  load();
}

function resize(e, c) {
  e.preventDefault();
  e.stopPropagation();
  const x0 = e.clientX, w0 = c.w;
  const t = e.target;
  t.setPointerCapture(e.pointerId);
  document.body.classList.add("col-resizing");
  t.onpointermove = (m) => { c.w = Math.max(c.key === "name" ? 120 : 40, Math.round(w0 + m.clientX - x0)); layout(); };
  t.onpointerup = () => {
    t.onpointermove = t.onpointerup = null;
    document.body.classList.remove("col-resizing");
    saveCols();
  };
}

// drag: a header moved sideways more than a few pixels moves the column; otherwise the
// press was a click, and sorts.
function drag(e, c, cell) {
  const x0 = e.clientX;
  let moving = false, to = -1;
  cell.setPointerCapture(e.pointerId);
  const cells = () => [...els.th.querySelectorAll(".th")];
  cell.onpointermove = (m) => {
    if (!moving && Math.abs(m.clientX - x0) < 5) return;
    moving = true;
    cell.classList.add("dragging");
    const cs = cells();
    to = cs.length;
    for (let i = 0; i < cs.length; i++) {
      const r = cs[i].getBoundingClientRect();
      if (m.clientX < r.left + r.width / 2) { to = i; break; }
    }
    for (const x of cs) x.classList.remove("drop-before", "drop-after");
    if (to < cs.length) cs[to].classList.add("drop-before"); else cs[cs.length - 1].classList.add("drop-after");
  };
  cell.onpointerup = () => {
    cell.onpointermove = cell.onpointerup = null;
    if (!moving) return sortBy(c.key);
    const shown = shownCols();
    const target = to < shown.length ? shown[to] : null;
    if (target !== c) {
      L.cols.splice(L.cols.indexOf(c), 1);
      const at = target ? L.cols.indexOf(target) : L.cols.length;
      L.cols.splice(at, 0, c);
      saveCols();
    }
    drawHead();
    els.rows.dataset.sig = "";
    drawRows();
  };
}

// colMenu: right click on the headers shows or hides a column (the name always shows).
function colMenu(x, y) {
  const redraw = () => { saveCols(); drawHead(); els.rows.dataset.sig = ""; drawRows(); };
  const items = L.cols.filter((c) => c.key !== "name").map((c) => ({
    label: c.title, key: c.shown ? "✓" : "", run: () => { c.shown = !c.shown; redraw(); },
  }));
  items.push({ label: T("reset the columns"), cls: "sep", run: () => { L.cols = mergeCols(L.page.columns || [], ""); redraw(); } });
  pop.menu(x, y, items);
}

// tableMenu (s): what the headers do with the mouse, from the keyboard (06 §3.1 rule 6):
// sort by a column (again reverses), move or size the sorted column, show or hide columns.
function tableMenu() {
  const redraw = () => { saveCols(); drawHead(); els.rows.dataset.sig = ""; drawRows(); };
  const sc = L.cols.find((c) => c.key === sortKey());
  const shift = (d) => {
    const vis = shownCols(), i = vis.indexOf(sc), to = vis[i + d];
    if (!sc || !to) return;
    L.cols.splice(L.cols.indexOf(sc), 1);
    L.cols.splice(L.cols.indexOf(to) + (d > 0 ? 1 : 0), 0, sc);
    redraw();
  };
  const items = shownCols().map((c) => ({
    label: Tf("sort by %s", c.title), key: sortKey() === c.key ? (ASC.has(c.key) !== L.sort.startsWith("-") ? "▲" : "▼") : "", run: () => sortBy(c.key),
  }));
  if (sc) items.push({ sep: true },
    { label: Tf("move %s left", sc.title), run: () => shift(-1) }, { label: Tf("move %s right", sc.title), run: () => shift(1) },
    { label: Tf("widen %s", sc.title), run: () => { sc.w += 24; layout(); saveCols(); } },
    { label: Tf("narrow %s", sc.title), run: () => { sc.w = Math.max(sc.key === "name" ? 120 : 40, sc.w - 24); layout(); saveCols(); } });
  items.push({ sep: true });
  for (const c of L.cols.filter((c) => c.key !== "name")) items.push({ label: c.title, key: c.shown ? "✓" : "", run: () => { c.shown = !c.shown; redraw(); } });
  items.push({ sep: true }, { label: T("reset the columns"), run: () => { L.cols = mergeCols(L.page.columns || [], ""); redraw(); } });
  const b = els.th.getBoundingClientRect();
  pop.menu(b.left + 20, b.bottom, items, true);
}

// ---- the rows ----

function drawRows() {
  if (!els) return;
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
    const cell = el("span", "c " + c.key + (c.key === "state" && cells.code ? " " + cells.code : ""), v);
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
    left.append(el("span", "tree", "└"), el("span", "t kind", x.i.name || x.i.label));
  }
  if (x.t === "inst" && x.i.inPlan) left.append(el("span", "here", "●"));
  return left;
}

function toggle(r) {
  if (L.open.has(r.id)) L.open.delete(r.id); else L.open.add(r.id);
  flatten();
  els.sizer.style.height = HEAD + L.flat.length * ROW + "px";
  els.rows.dataset.sig = "";
  drawRows();
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
  } else {
    L.anchor = k;
    // a click on a Lineage's row again opens or closes it; the details follow the cursor
    if (x.t === "lin" && L.cur === k) toggle(x.r);
  }
  L.cur = k;
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

let followTimer = 0;
// follow draws the cursor's copy in the details, at once when asked, else once the cursor
// rests a moment (the arrows held down do not load every copy on the way).
function follow(now) {
  clearTimeout(followTimer);
  if (now) return drawDetail();
  followTimer = setTimeout(drawDetail, 90);
}

function setFull(v) {
  L.full = v;
  els.root.classList.toggle("full", v);
  if (v) { L.focus = "detail"; els.scroll.focus(); } else { L.focus = "rows"; els.rows.focus(); }
  markFocus();
  ui.renderWhere();
  hints();
}

let detailSeq = 0;
// drawDetail: the copy's note at the top, then its buttons (the detail's ops, only those
// that apply), then one field list, then the mod's picture, name and description (06 §2.9).
async function drawDetail() {
  if (!els) return;
  const i = cursorCopy() && findCopy(cursorCopy().ref);
  const box = els.scroll;
  const row = i && (L.page.rows || []).find((r) => r.insts.some((x) => x.ref === i.ref));
  els.dhead.innerHTML = "";
  els.dhead.append(el("span", "t", T("details")));
  if (i) els.dhead.append(el("span", "q", (row ? row.name : "") + " · " + i.label));
  if (!i) { box.innerHTML = ""; box.dataset.ref = ""; return; }
  const same = box.dataset.ref === i.ref && box.dataset.stamp === stampOf(i);
  if (same) return;
  if (box.dataset.ref === i.ref && box.querySelector(".lib-note.editing")) return; // keep a note being written
  const n = ++detailSeq;
  const keep = box.dataset.ref === i.ref ? box.scrollTop : 0;
  box.innerHTML = "";
  box.dataset.ref = i.ref;
  box.dataset.stamp = stampOf(i);
  box.append(noteEl(i));
  const acts = el("div", "lib-acts");
  for (const c of ops.sorted((i.ops || []).filter((c) => ops.op(c).detail))) {
    const o = ops.op(c);
    const b = el("button", "btn" + (o.danger ? " danger" : ""), o.label);
    if (o.key) b.append(el("kbd", "", o.key));
    b.onclick = () => run(c, [i]);
    acts.append(b);
  }
  if (acts.childNodes.length) box.append(acts);
  const fields = el("div", "lib-fields");
  box.append(fields);
  const d = el("div", "lib-desc");
  box.append(d);
  try {
    const fs = await api.LibraryFields(i.ref);
    if (n !== detailSeq) return;
    fields.append(fieldList(fs, {
      folder: () => run("folder", [i]),
      tags: () => run("tags", [i]),
    }));
  } catch (e) { if (n === detailSeq) fields.append(el("p", "err", errText(e))); }
  box.scrollTop = keep;
  if (!i.id) return;
  try {
    const det = await api.Detail(i.id, false, false, false);
    if (n !== detailSeq) return;
    renderDetail(det, d, box, true, { page: true });
  } catch (e) { if (n === detailSeq) d.append(el("p", "err", errText(e))); }
}
const stampOf = (i) => [i.note, i.auto, (i.ops || []).join(","), i.state, i.inPlan].join("|");

// noteEl: the copy's own note (06 §2.9). The player's text, else what RimArcho wrote (grey),
// else a place to write one; a click (or n) edits it, enter saves, shift+enter breaks the
// line, esc leaves it as it was.
function noteEl(i) {
  const box = el("div", "lib-note" + (i.note ? "" : i.auto ? " auto" : " empty"));
  const show = () => {
    box.innerHTML = "";
    box.classList.remove("editing");
    box.append(el("div", "txt", i.note || i.auto || T("write a note")));
    if (!i.note && i.auto) box.append(el("div", "by", T("by RimArcho · click to edit")));
  };
  show();
  if (!i.id) return box;
  box.title = T("click to edit") + " · n";
  box.onclick = () => {
    if (box.classList.contains("editing")) return;
    box.classList.add("editing");
    box.innerHTML = "";
    const start = i.note || i.auto || "";
    const ta = el("textarea", "lib-note-in");
    ta.value = start;
    ta.placeholder = T("a note on this copy…");
    ta.rows = Math.min(6, Math.max(2, start.split("\n").length + 1));
    box.append(ta, el("div", "by", T("enter saves · esc cancels")));
    ta.focus();
    ta.setSelectionRange(ta.value.length, ta.value.length);
    let done = false;
    const save = async () => {
      if (done) return;
      done = true;
      const text = ta.value.trim();
      if (text === start.trim()) { show(); els.rows.focus(); return; }
      try { say("ok", await api.LibrarySetNote(i.ref, text)); } catch (e) { say("fail", errText(e)); }
      box.classList.remove("editing");
      els.rows.focus();
      await load();
    };
    ta.onkeydown = (e) => {
      e.stopPropagation();
      if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); save(); }
      else if (e.key === "Escape") { e.preventDefault(); done = true; show(); els.rows.focus(); }
    };
    ta.onblur = save;
  };
  return box;
}

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
  if (k === "ctrl+w") return say("", T("no Workshop ID: not a Workshop mod"));
  if (k === "ctrl+o") return say("", i && i.id ? T("the folder is not there") : T("RimArcho has not seen this copy: no folder"));
  say("", T("nothing to do with this row"));
}

const PLAN = new Set(["add", "take-out", "put-back", "stage", "use", "swap", "remove"]);

async function run(code, copies) {
  const refs = copies.map((i) => i.ref);
  const i0 = copies[0];
  const o = ops.op(code);
  const done = async (p, undo) => {
    try { const t = await p; say("ok", typeof t === "string" ? t : t && t.text, undo); } catch (e) { say("fail", errText(e)); }
    L.sel.clear();
    await load();
    ui.reload();
  };
  switch (code) {
    case "folder": return api.OpenFolder(i0.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
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
    case "enable-at": case "move-to": {
      const ids = copies.filter((i) => i.id).map((i) => i.id);
      const row = (L.page.rows || []).find((r) => r.insts.some((x) => x.ref === i0.ref));
      const what = ids.length === 1 ? (row ? row.name : i0.label) : Tf("%d mods", ids.length);
      const from = code === "move-to" ? 1 : i0.ops.includes("put-back") ? 2 : 0;
      return pop.position({ ids, what }, from, (at) => done(api.Place(from, ids, at), true));
    }
  }
  if (PLAN.has(code)) return done(api.LibraryDo(code, refs), true);
  if (!o.confirm) return done(api.LibraryDo(code, refs), false);
  let p;
  try { p = await api.LibraryPreview(code, refs); } catch (e) { return say("fail", errText(e)); }
  // 优化贴图 takes minutes: it runs in the background ("textures" events), like the page's
  const run = code === "textures" ? () => done(api.TexturesRun("", 100, refs), false) : () => done(api.LibraryDo(code, refs), false);
  pop.confirm(p.title, p.lines, o.label, p.apply ? run : null);
}

// ---- keys (06 §3.5): the plan editor's table; the arrows only move ----

export function key(e) {
  const k = e.key;
  const ctrl = e.ctrlKey || e.metaKey;
  const x = L.flat[L.cur];
  const done = () => e.preventDefault();
  if (!els) return;
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
    if (x.t === "lin" && !(L.open.has(x.r.id) || x.r.insts.length < x.r.copies)) return toggle(x.r);
    if (x.t === "lin") return move(1);
    L.focus = "detail"; els.scroll.focus(); markFocus(); return hints();
  }
  if (k === "ArrowLeft" || k === "h") {
    done();
    if (!x) return;
    if (x.t === "inst" && !x.lone) { L.cur = L.flat.findIndex((y) => y.t === "lin" && y.r.id === x.r.id); ensureVisible(); els.rows.dataset.sig = ""; drawRows(); return follow(); }
    if (x.t === "lin" && L.open.has(x.r.id)) return toggle(x.r);
    return;
  }
  if (k === "Enter") { done(); return setFull(!L.full); }
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
  if (ctrl && !e.shiftKey && !e.altKey && (k.toLowerCase() === "o" || k.toLowerCase() === "w")) { done(); return byKey("ctrl+" + k.toLowerCase()); }
  if (k === "/" || (ctrl && k.toLowerCase() === "f")) { done(); return L.bar.focus(); }
  if (k === "\\") { done(); return L.bar.toggle(); }
  // undo / redo: the plan edits (space, m, t, x) are the current plan's steps
  if ((k === "u" && !ctrl) || (ctrl && k.toLowerCase() === "z")) { done(); return ui.verb("undo").then(() => load()); }
  if (ctrl && (k.toLowerCase() === "y" || k.toLowerCase() === "r")) { done(); return ui.verb("redo").then(() => load()); }
  if (ctrl || e.altKey) return;
  const keys = { " ": "space", m: "m", t: "t", x: "x", g: "g", n: "n", Delete: "Del" };
  if (keys[k]) { done(); return byKey(keys[k]); }
  if (k === "s") { done(); return tableMenu(); }
  if (k === "C") { done(); const i = cursorCopy(); if (i && i.id) document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: i.id, name: (L.flat[L.cur] || {}).r ? L.flat[L.cur].r.name : i.label } })); return; }
  if (k === "R") { done(); return ui.cmd("rescan"); }
}

// where: the position line on this page (06 §3.2): the cursor's row of the rows shown.
export function where() {
  if (!L.page) return "";
  let s = `${T("Mod library")} · ${L.flat.length ? L.cur + 1 : 0} / ${L.flat.length}`;
  if (L.sel.size) s += " · " + Tf("%d selected", L.sel.size);
  if (L.full) s += " › " + T("details");
  return s;
}

// hintList: the hint line for the page (06 §2.2): what the object can do now, as the editor.
export function hintList() {
  const list = [];
  const x = L.flat[L.cur];
  const codes = [...opsOf(selected()).keys()];
  const sp = ops.keyed(codes, "space");
  if (sp) list.push({ key: "space", label: ops.op(sp).label });
  for (const k of ["m", "t"]) { const c = ops.keyed(codes, k); if (c) list.push({ key: k, label: ops.op(c).label }); }
  if (x && x.t === "lin") list.push({ key: "→ ←", label: T("open / close") });
  list.push({ key: "enter", label: L.full ? T("back") : T("details") });
  if (codes.includes("folder") || codes.includes("workshop")) list.push({ key: "ctrl+o", label: T("open the folder") }, { key: "ctrl+w", label: T("Workshop page") });
  list.push({ key: "/", label: T("search") }, { key: "\\", label: T("filter") });
  list.push({ key: "?", label: T("all keys") });
  return list;
}
function hints() { ui.renderHints(); ui.renderWhere(); }
