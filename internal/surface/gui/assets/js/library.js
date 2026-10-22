// Mod 库存 › 全部 (06 §2.9): what is on this machine. The filters, then a table like a file
// manager's details view: one row per Lineage (opening to its copies), columns the player
// sorts (click a header, again reverses), moves (drag a header), sizes (drag its edge) and
// picks (right click the headers), kept in ui.gui.library.columns. A copy's details open
// over the right of the table (esc goes back). What it shows comes from view.BuildLibrary
// through API.Library; the actions are the copies' view.LibAction data. The table draws
// only the rows in view, so 1350+ Lineages stay smooth.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { on } from "./api.js";
import { icon } from "./icons.js";
import { renderDetail } from "./detail.js";
import { fieldList } from "./fields.js";
import * as pop from "./popups.js";
import { openScreen, page, act, rescanButton } from "./screens.js";

const FILTER = '<svg class="i" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z"/></svg>';
const ROW = 30;      // every row has the same height: the list is drawn by position
const HEAD = 28;     // the column headers, sticky at the top of the table
const SHOW = 6;      // filter values shown before "more"
const COLS_KEY = "ui.gui.library.columns";
// a column's width when the player has not set one; the name column also takes what is left
const WIDTH = { name: 260, size: 72, state: 116, source: 120, used: 150, updated: 84, version: 64, author: 130, tags: 160, played: 84 };
// columns whose natural order is A to Z (the arrow points up); the rest start biggest or newest
const ASC = new Set(["name", "source", "author", "tags"]);
const L = {
  page: null, search: "", filters: [], sort: "name",
  open: new Set(),   // lineages shown open
  more: new Set(),   // filter groups shown in full
  flat: [], cur: 0, anchor: -1,
  sel: new Set(),    // selected copies, by ref
  detail: null,      // the copy whose details replace the right column
  drawer: false,     // a narrow window: the filters over the list
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
  if (S.screen !== "library") {
    L.detail = null;
    L.drawer = false;
  }
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

async function load(fresh) {
  const n = ++L.seq;
  let p, saved;
  try {
    [p, saved] = await Promise.all([api.Library(L.search, L.filters, L.sort), L.cols ? null : api.Setting(COLS_KEY).catch(() => "")]);
  } catch (e) { say("fail", errText(e)); return; }
  if (n !== L.seq) return;
  L.page = p;
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
  // a narrow window: the filters open over the list (06 §2.9)
  const filterBtn = act(T("filters"), "", () => setDrawer(!L.drawer), "f");
  filterBtn.classList.add("lib-drawer-btn");
  filterBtn.querySelector(".ic").innerHTML = FILTER;
  const w = page("fill library", lead(), filterBtn, rescanButton(T("rescan")));
  const root = el("div", "lib" + (L.drawer ? " drawer-open" : ""));
  const side = el("aside", "lib-filter card");
  const scrim = el("div", "lib-scrim");
  scrim.onclick = () => setDrawer(false);
  const main = el("section", "lib-main card");
  const head = el("div", "lib-head");
  const rows = el("div", "lib-rows");
  rows.tabIndex = -1;
  const th = el("div", "lib-th");
  const sizer = el("div", "lib-sizer");
  rows.append(th, sizer);
  const detail = el("section", "lib-detail");
  detail.hidden = true;
  main.append(head, rows, detail);
  const bar = el("div", "lib-bar");
  bar.hidden = true;
  root.append(side, scrim, main);
  w.append(root, bar);
  sc.append(w);
  els = { root, side, main, head, rows, th, sizer, detail, bar, lead: w.querySelector(".lead") };
  rows.addEventListener("scroll", drawRows, { passive: true });
  rows.addEventListener("mousedown", () => rows.focus());
  draw();
}

function lead() {
  const p = L.page;
  if (!p) return "";
  return p.lead;
}

function draw() {
  if (!els) return;
  if (els.lead) els.lead.textContent = lead();
  els.root.classList.toggle("drawer-open", L.drawer);
  drawFilters();
  drawHead();
  els.sizer.style.height = HEAD + L.flat.length * ROW + "px";
  els.rows.dataset.sig = "";
  drawRows();
  drawDetail();
  drawBar();
  hints();
}

// ---- the filter column ----

function drawFilters() {
  const p = L.page;
  const box = els.side;
  const focused = document.activeElement && document.activeElement.classList.contains("lib-search");
  const caret = focused ? document.activeElement.selectionStart : null;
  box.innerHTML = "";
  const s = el("div", "lib-search-box");
  const inp = el("input", "lib-search");
  inp.placeholder = T("search: words, tag:, state:, source:, used:, -not") + "  /";
  inp.spellcheck = false;
  inp.value = L.search;
  // the grey completion after the last word (06 §3.7, view.CompleteLibrary): the typed text
  // invisibly, then the rest, as in the plan's search boxes
  const ghost = el("span", "lib-ghost");
  ghost.append(el("span", "typed"), el("span", "sfx"));
  const showGhost = () => {
    const sfx = p.complete && inp.value === L.search ? p.complete : "";
    const atEnd = document.activeElement === inp && inp.selectionStart === inp.value.length && inp.selectionEnd === inp.value.length;
    ghost.hidden = !(sfx && atEnd && inp.scrollLeft === 0);
    if (!ghost.hidden) { ghost.firstChild.textContent = inp.value; ghost.lastChild.textContent = sfx; }
  };
  let timer = 0;
  inp.oninput = () => { ghost.hidden = true; clearTimeout(timer); timer = setTimeout(() => { L.search = inp.value; L.cur = 0; load(); }, 120); };
  inp.onblur = inp.onscroll = showGhost;
  inp.onkeydown = (e) => {
    if ((e.key === "Tab" || e.key === "ArrowRight") && !ghost.hidden && p.complete) {
      e.preventDefault(); e.stopPropagation();
      inp.value += p.complete;
      L.search = inp.value;
      L.cur = 0;
      load();
      return;
    }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); if (inp.value) { inp.value = ""; L.search = ""; load(); } inp.blur(); els.rows.focus(); }
    else if (e.key === "Enter" || e.key === "ArrowDown") { e.preventDefault(); e.stopPropagation(); inp.blur(); els.rows.focus(); }
    else e.stopPropagation();
  };
  s.append(inp, ghost);
  box.append(s);
  // what is in force, each removable, and clear-all
  if ((p.conds || []).length || L.search) {
    const c = el("div", "lib-conds");
    for (const x of p.conds || []) {
      const b = el("button", "lib-cond" + (x.exclude ? " ex" : ""));
      b.append(el("span", "", x.label), el("span", "x", "×"));
      b.title = x.exclude ? T("left out") : "";
      b.onclick = () => setFilter(x.filter.replace(/^-/, ""), 0);
      c.append(b);
    }
    const all = el("button", "lib-clear", T("clear all"));
    all.onclick = () => { L.filters = []; L.search = ""; L.cur = 0; load(); };
    c.append(all);
    box.append(c);
  }
  const groups = el("div", "lib-groups");
  for (const g of p.groups || []) {
    const vals = (g.values || []).filter((v) => v.count > 0 || v.on);
    if (!vals.length) continue;
    const sec = el("div", "lib-group");
    sec.append(el("div", "lib-gtitle", g.title));
    const full = L.more.has(g.key);
    vals.forEach((v, i) => {
      if (!full && i >= SHOW && !v.on) return;
      const b = el("button", "lib-val" + (v.on === 1 ? " on" : v.on === -1 ? " ex" : "") + (v.cur ? " cur" : ""));
      b.append(el("span", "lbl", v.label));
      if (v.cur) b.append(el("span", "curv", T("this game")));
      b.append(el("span", "n num", String(v.count)));
      b.title = T("left click: only these · right click: leave these out");
      b.onclick = () => setFilter(v.filter, v.on === 1 ? 0 : 1);
      b.oncontextmenu = (e) => { e.preventDefault(); setFilter(v.filter, v.on === -1 ? 0 : -1); };
      sec.append(b);
    });
    if (vals.length > SHOW) {
      const m = el("button", "lib-more", full ? T("fewer") : Tf("%d more", vals.length - SHOW));
      m.onclick = () => { full ? L.more.delete(g.key) : L.more.add(g.key); drawFilters(); };
      sec.append(m);
    }
    groups.append(sec);
  }
  box.append(groups);
  if (focused) { inp.focus(); if (caret != null) inp.setSelectionRange(caret, caret); }
  showGhost();
}

// setFilter: on 1 keeps only copies with the value, -1 leaves them out, 0 drops the condition.
function setFilter(f, on) {
  L.filters = L.filters.filter((x) => x !== f && x !== "-" + f);
  if (on === 1) L.filters.push(f);
  if (on === -1) L.filters.push("-" + f);
  L.cur = 0;
  load();
}

function setDrawer(v) {
  L.drawer = v;
  if (els) els.root.classList.toggle("drawer-open", v);
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

// ---- the rows ----

function drawRows() {
  if (!els) return;
  const box = els.rows;
  const top = Math.max(0, box.scrollTop - HEAD), h = box.clientHeight || 600;
  const a = Math.max(0, Math.floor(top / ROW) - 8), b = Math.min(L.flat.length, Math.ceil((top + h) / ROW) + 8);
  const sig = [a, b, L.cur, L.sel.size, L.flat.length, L.detail && L.detail.ref, [...L.sel].join(",")].join("|");
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
  if (L.detail && x.t === "inst" && L.detail.ref === x.i.ref) r.classList.add("open");
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
  r.oncontextmenu = (e) => { e.preventDefault(); if (!(selected)) { L.cur = k; drawRows(); } menuAt(e.clientX, e.clientY); };
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
    // a copy opens its details in the right column; again (or esc) goes back (06 §2.9)
    if (x.t === "inst") {
      L.cur = k;
      if (L.detail && L.detail.ref === x.i.ref) closeDetail(); else openDetail(x.i);
      return;
    }
    if (x.t === "lin" && L.cur === k) toggle(x.r);
  }
  L.cur = k;
  els.rows.dataset.sig = "";
  drawRows();
  drawBar();
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
  // the details follow the cursor while they are open
  const x = L.flat[L.cur];
  if (L.detail && x.t === "inst" && x.i.ref !== L.detail.ref) openDetail(x.i);
  hints();
}

// ---- a copy's details: they replace the right column ----

async function openDetail(i) {
  L.detail = i;
  els.main.classList.add("detail-open");
  els.rows.dataset.sig = "";
  drawRows();
  drawDetail();
  hints();
}

function closeDetail() {
  L.detail = null;
  els.main.classList.remove("detail-open");
  els.rows.dataset.sig = "";
  drawRows();
  drawDetail();
  hints();
}

let detailSeq = 0;
// drawDetail: the copy's note at the top, then its actions (only those that apply), then
// one field list, then the mod's picture, name and description (06 §2.9).
async function drawDetail() {
  const box = els.detail;
  const i = L.detail && findCopy(L.detail.ref);
  if (!i) { box.hidden = true; box.dataset.ref = ""; els.main.classList.remove("detail-open"); L.detail = null; return; }
  const same = box.dataset.ref === i.ref && !box.hidden;
  L.detail = i;
  box.hidden = false;
  if (same && box.querySelector(".lib-note.editing")) return; // keep a note being written
  const n = ++detailSeq;
  const old = box.querySelector(".scroll");
  const keep = same && old ? old.scrollTop : 0;
  box.innerHTML = "";
  box.dataset.ref = i.ref;
  const top = el("div", "lib-dhead");
  const back = el("button", "btn small", "← " + T("back"));
  back.append(el("kbd", "", "esc"));
  back.onclick = closeDetail;
  const row = (L.page.rows || []).find((r) => r.insts.some((x) => x.ref === i.ref));
  top.append(back, el("span", "t", row ? row.name : i.label), el("span", "faint", i.label));
  const scroll = el("div", "scroll");
  box.append(top, scroll);
  scroll.append(noteEl(i));
  const acts = el("div", "lib-acts");
  for (const a of (i.actions || []).filter((a) => a.detail)) {
    const b = el("button", "btn" + (a.danger ? " danger" : ""), a.label);
    b.onclick = () => run(a, [i]);
    acts.append(b);
  }
  if (acts.childNodes.length) scroll.append(acts);
  const fields = el("div", "lib-fields");
  scroll.append(fields);
  const d = el("div", "lib-desc");
  d.id = "lib-detail";
  scroll.append(d);
  try {
    const fs = await api.LibraryFields(i.ref);
    if (n !== detailSeq) return;
    fields.append(fieldList(fs, {
      folder: () => api.OpenFolder(i.id).catch((e) => say("fail", errText(e))),
      tags: () => pop.tags([{ id: i.id, name: row ? row.name : i.label }], () => load()),
    }));
  } catch (e) { if (n === detailSeq) fields.append(el("p", "err", errText(e))); }
  scroll.scrollTop = keep;
  if (!i.id) return;
  try {
    const det = await api.Detail(i.id, false, false, false);
    if (n !== detailSeq) return;
    renderDetail(det, d, scroll, true, { page: true });
  } catch (e) { if (n === detailSeq) d.append(el("p", "err", errText(e))); }
}

// noteEl: the copy's own note (06 §2.9). The player's text, else what RimArcho wrote (grey),
// else a place to write one; a click edits it, enter saves, shift+enter breaks the line,
// esc leaves it as it was.
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
  box.title = T("click to edit");
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
      if (text === start.trim()) return show();
      try { say("ok", await api.LibrarySetNote(i.ref, text)); } catch (e) { say("fail", errText(e)); }
      box.classList.remove("editing");
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

// ---- actions: the selection's (the bar), else the cursor's ----

function selected() {
  if (L.sel.size) {
    const out = [];
    for (const r of L.page.rows || []) for (const i of r.insts) if (L.sel.has(i.ref)) out.push(i);
    return out;
  }
  const x = L.flat[L.cur];
  if (!x) return [];
  return x.t === "inst" ? [x.i] : x.r.single ? [x.r.insts[0]] : [];
}

// actionsOf: every action any of the copies has, each run on the copies that have it.
function actionsOf(copies) {
  const order = [], by = new Map();
  for (const i of copies) for (const a of i.actions || []) {
    if (!by.has(a.kind)) { by.set(a.kind, { a, copies: [] }); order.push(a.kind); }
    by.get(a.kind).copies.push(i);
  }
  return order.map((k) => by.get(k));
}

function drawBar() {
  const bar = els.bar;
  if (!L.sel.size) { bar.hidden = true; return; }
  bar.hidden = false;
  bar.innerHTML = "";
  bar.append(el("span", "accent", Tf("selected %d", L.sel.size)));
  for (const { a, copies } of actionsOf(selected())) {
    if (a.kind === "folder" || a.kind === "workshop") continue; // one copy at a time
    const b = el("button", "btn" + (a.danger ? " danger" : ""), a.label);
    if (copies.length < L.sel.size) b.append(el("span", "faint", " " + copies.length));
    b.onclick = () => run(a, copies);
    bar.append(b);
  }
  const x = el("button", "btn quiet", T("clear the selection"));
  x.append(el("kbd", "", "esc"));
  x.onclick = () => { L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); drawBar(); hints(); };
  bar.append(x);
}

function menuAt(x, y) {
  const items = actionsOf(selected()).map(({ a, copies }) => ({ label: a.label, run: () => run(a, copies) }));
  if (items.length) pop.menu(x, y, items);
}

async function run(a, copies) {
  const refs = copies.map((i) => i.ref);
  const i0 = copies[0];
  if (a.kind === "folder") return api.OpenFolder(i0.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
  if (a.kind === "workshop") return api.OpenURL(i0.url).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
  const go = async () => {
    try { say("ok", await api.LibraryDo(a.kind, refs), a.kind === "enable" || a.kind === "remove"); } catch (e) { say("fail", errText(e)); }
    L.sel.clear();
    await load();
    ui.reload();
  };
  if (!a.confirm) return go();
  let p;
  try { p = await api.LibraryPreview(a.kind, refs); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, a.label, p.apply ? go : null);
}

// ---- keys (06 §3): the arrows only move; enter opens; verbs act on the object ----

export function key(e) {
  const k = e.key;
  const ctrl = e.ctrlKey || e.metaKey;
  const x = L.flat[L.cur];
  const done = () => e.preventDefault();
  if (k === "ArrowDown" || k === "j") { done(); return move(1, e.shiftKey); }
  if (k === "ArrowUp" || k === "k") { done(); return move(-1, e.shiftKey); }
  if (k === "PageDown") { done(); return move(Math.floor(els.rows.clientHeight / ROW) - 1, e.shiftKey); }
  if (k === "PageUp") { done(); return move(-Math.floor(els.rows.clientHeight / ROW) + 1, e.shiftKey); }
  if (k === "Home") { done(); return move(-1e9, e.shiftKey); }
  if (k === "End") { done(); return move(1e9, e.shiftKey); }
  if (k === "ArrowRight" || k === "l") {
    done();
    if (!x) return;
    if (x.t === "lin" && !(L.open.has(x.r.id) || x.r.insts.length < x.r.copies)) return toggle(x.r);
    if (x.t === "lin") return move(1);
    return openDetail(x.i);
  }
  if (k === "ArrowLeft" || k === "h") {
    done();
    if (L.detail) return closeDetail();
    if (!x) return;
    if (x.t === "inst" && !x.lone) { L.cur = L.flat.findIndex((y) => y.t === "lin" && y.r.id === x.r.id); ensureVisible(); els.rows.dataset.sig = ""; return drawRows(); }
    if (x.t === "lin" && L.open.has(x.r.id)) return toggle(x.r);
    return;
  }
  if (k === "Enter") {
    done();
    if (!x) return;
    if (x.t === "lin") return toggle(x.r);
    return L.detail && L.detail.ref === x.i.ref ? closeDetail() : openDetail(x.i);
  }
  if (k === "Escape") {
    done();
    if (L.drawer) return setDrawer(false);
    if (L.detail) return closeDetail();
    if (L.sel.size) { L.sel.clear(); els.rows.dataset.sig = ""; drawRows(); drawBar(); return hints(); }
    if (L.search || L.filters.length) { L.search = ""; L.filters = []; L.cur = 0; return load(); }
    return;
  }
  if (ctrl && (k === " " || k === "Spacebar")) {
    done();
    if (x) { toggleSel(x); els.rows.dataset.sig = ""; drawRows(); drawBar(); hints(); }
    return;
  }
  // ctrl+o / ctrl+w: the cursor's copy's folder and Workshop page (06 §3.5); the Wails
  // window does not take them, preventDefault keeps them from the browser
  if (ctrl && !e.shiftKey && !e.altKey && (k.toLowerCase() === "o" || k.toLowerCase() === "w")) {
    done();
    return shortcut(k.toLowerCase() === "o" ? "folder" : "workshop");
  }
  if (k === "/" || (ctrl && k === "f")) { done(); const s = els.side.querySelector(".lib-search"); if (innerWidth < 760) setDrawer(true); s.focus(); s.select(); return; }
  if (k === "f" && !ctrl) { done(); return setDrawer(!L.drawer); }
  if (k === "u" && !ctrl) { done(); return ui.verb("undo").then(() => load()); }
  const verbs = { " ": "enable", o: "workshop", d: "folder" };
  if (verbs[k] && !ctrl) {
    done();
    const found = actionsOf(selected()).find(({ a }) => a.kind === verbs[k]);
    if (found) return run(found.a, found.copies);
    return;
  }
  if (k === "R") { done(); return ui.cmd("rescan"); }
}

// cursorCopy: the copy the cursor stands for: its row's, the open details', or on a
// Lineage's row the copy in the current plan, else the first.
function cursorCopy() {
  const x = L.flat[L.cur];
  if (!x) return null;
  if (x.t === "inst") return x.i;
  return x.r.insts.find((i) => i.inPlan) || x.r.insts.find((i) => i.id) || x.r.insts[0] || null;
}

// shortcut runs ctrl+o (folder) or ctrl+w (workshop) on the cursor's copy.
function shortcut(kind) {
  const i = L.detail || cursorCopy();
  if (!i) return;
  const a = (i.actions || []).find((a) => a.kind === kind);
  if (a) return run(a, [i]);
  say("", kind === "workshop" ? T("no Workshop ID: not a Workshop mod") : i.id ? T("the folder is not there") : T("RimArcho has not seen this copy: no folder"));
}

// hints: the hint line for the page (06 §2.2).
export function hintList() {
  const list = [{ key: "↑↓", label: T("move") }];
  const x = L.flat[L.cur];
  if (x && x.t === "lin") list.push({ key: "→ ←", label: T("open / close") });
  if (x && x.t === "inst") list.push({ key: "enter", label: L.detail ? T("back") : T("details") });
  const acts = actionsOf(selected()).map(({ a }) => a.kind);
  if (acts.includes("enable")) list.push({ key: "space", label: T("add to the current plan") });
  if (x) list.push({ key: "ctrl+o ctrl+w", label: T("folder / Workshop page") });
  list.push({ key: "/", label: T("search") }, { key: "ctrl+space", label: T("select") });
  list.push({ key: "?", label: T("all keys") });
  return list;
}
function hints() { ui.renderHints(); }
