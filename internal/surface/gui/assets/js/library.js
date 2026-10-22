// Mod 库存 › 全部 (06 §2.9): what is on this machine. Three columns: the filters, one row per
// Lineage (opening to its copies), and the right column's size · state · used-in, row by
// row; a copy's details replace the right column (esc goes back). What it shows comes from
// view.BuildLibrary through API.Library; the actions are the copies' view.LibAction data.
// The list draws only the rows in view, so 1350+ Lineages stay smooth.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { on } from "./api.js";
import { icon } from "./icons.js";
import { renderDetail } from "./detail.js";
import * as pop from "./popups.js";
import { openScreen, page, act, rescanButton } from "./screens.js";

const FILTER = '<svg class="i" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z"/></svg>';
const ROW = 30;      // every row has the same height: the list is drawn by position
const SHOW = 6;      // filter values shown before "more"
const L = {
  page: null, search: "", filters: [], sort: "name",
  open: new Set(),   // lineages shown open
  more: new Set(),   // filter groups shown in full
  flat: [], cur: 0, anchor: -1,
  sel: new Set(),    // selected copies, by ref
  detail: null,      // the copy whose details replace the right column
  drawer: false,     // a narrow window: the filters over the list
  seq: 0,
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

async function load(fresh) {
  const n = ++L.seq;
  let p;
  try { p = await api.Library(L.search, L.filters, L.sort); } catch (e) { say("fail", errText(e)); return; }
  if (n !== L.seq) return;
  L.page = p;
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
  const w = page("fill library", lead(), filterBtn, rescanButton());
  const root = el("div", "lib" + (L.drawer ? " drawer-open" : ""));
  const side = el("aside", "lib-filter card");
  const scrim = el("div", "lib-scrim");
  scrim.onclick = () => setDrawer(false);
  const main = el("section", "lib-main card");
  const head = el("div", "lib-head");
  const rows = el("div", "lib-rows");
  rows.tabIndex = -1;
  const sizer = el("div", "lib-sizer");
  rows.append(sizer);
  const detail = el("section", "lib-detail");
  detail.hidden = true;
  main.append(head, rows, detail);
  const bar = el("div", "lib-bar");
  bar.hidden = true;
  root.append(side, scrim, main);
  w.append(root, bar);
  sc.append(w);
  els = { root, side, main, head, rows, sizer, detail, bar, lead: w.querySelector(".lead") };
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
  els.sizer.style.height = L.flat.length * ROW + "px";
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
      const b = el("button", "lib-val" + (v.on === 1 ? " on" : v.on === -1 ? " ex" : ""));
      b.append(el("span", "lbl", v.label), el("span", "n num", String(v.count)));
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
  const so = el("div", "lib-sort");
  so.append(el("div", "lib-gtitle", T("sort")));
  const seg = el("div", "lib-sorts");
  for (const x of p.sorts || []) {
    const b = el("button", x.filter === p.sort ? "on" : "", x.label);
    b.onclick = () => { L.sort = x.filter; load(); };
    seg.append(b);
  }
  so.append(seg);
  box.append(so);
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

// ---- the list ----

function drawHead() {
  const p = L.page;
  const h = els.head;
  h.innerHTML = "";
  const n = p.shown === p.total ? Tf("%d mods", p.total) : Tf("%d of %d mods", p.shown, p.total);
  h.append(el("span", "lib-count", n), el("span", "faint", " · " + p.size));
  const r = el("div", "lib-rh");
  r.append(el("span", "", T("size")), el("span", "", T("state")), el("span", "", T("used in")));
  h.append(r);
}

function drawRows() {
  if (!els) return;
  const box = els.rows;
  const top = box.scrollTop, h = box.clientHeight || 600;
  const a = Math.max(0, Math.floor(top / ROW) - 8), b = Math.min(L.flat.length, Math.ceil((top + h) / ROW) + 8);
  const sig = [a, b, L.cur, L.sel.size, L.flat.length, L.detail && L.detail.ref, [...L.sel].join(",")].join("|");
  if (box.dataset.sig === sig) return;
  box.dataset.sig = sig;
  for (const n of [...box.querySelectorAll(".lib-row")]) n.remove();
  if (!L.flat.length) {
    const p = L.page;
    const e = el("div", "lib-row empty", p.total ? T("no mod matches") : T("no mods yet (R scans the mod folders)"));
    e.style.top = "0px";
    box.append(e);
    return;
  }
  const frag = document.createDocumentFragment();
  for (let k = a; k < b; k++) frag.append(rowEl(L.flat[k], k));
  box.append(frag);
}

function rowEl(x, k) {
  const r = el("div", "lib-row " + x.t + (x.lone ? " lone" : "") + (k === L.cur ? " cur" : ""));
  r.style.top = k * ROW + "px";
  const selected = x.t === "inst" ? L.sel.has(x.i.ref) : x.r.insts.every((i) => L.sel.has(i.ref)) && x.r.insts.length;
  if (selected) r.classList.add("sel");
  if (L.detail && x.t === "inst" && L.detail.ref === x.i.ref) r.classList.add("open");
  const left = el("div", "l");
  left.append(el("span", "m"));
  let size, state, code, used, usedAll;
  if (x.t === "lin") {
    const c = el("span", "chev" + (L.open.has(x.r.id) || x.r.insts.length < x.r.copies ? " down" : ""));
    c.innerHTML = icon.chev;
    c.onclick = (e) => { e.stopPropagation(); toggle(x.r); };
    left.append(c, el("span", "t", x.r.name));
    left.append(el("span", "g", Tf("%d copies", x.r.copies)));
    if (x.r.tags && x.r.tags.length) left.append(el("span", "g", x.r.tags.join(" ")));
    ({ size, state, code, used } = x.r);
    usedAll = x.r.usedTitle;
  } else {
    const i = x.i;
    if (x.lone) {
      left.append(el("span", "t", x.r.name), el("span", "g", i.label));
      if (x.r.tags && x.r.tags.length) left.append(el("span", "g", x.r.tags.join(" ")));
    } else {
      left.append(el("span", "tree", "└"), el("span", "t kind", i.label));
    }
    if (i.note) left.append(el("span", "note", i.note));
    if (i.inPlan) left.append(el("span", "here", "●"));
    ({ size, state, code, used } = i);
    usedAll = i.usedTitle;
  }
  const right = el("div", "rc");
  right.append(el("span", "sz num", size || ""));
  const st = el("span", "st " + (code || ""), state || "");
  right.append(st);
  const u = el("span", "us", used || "");
  if (usedAll) u.title = usedAll;
  right.append(u);
  r.append(left, right);
  r.onclick = (e) => click(k, e);
  r.ondblclick = () => { if (x.t === "lin") toggle(x.r); };
  r.oncontextmenu = (e) => { e.preventDefault(); if (!(selected)) { L.cur = k; drawRows(); } menuAt(e.clientX, e.clientY); };
  return r;
}

function toggle(r) {
  if (L.open.has(r.id)) L.open.delete(r.id); else L.open.add(r.id);
  flatten();
  els.sizer.style.height = L.flat.length * ROW + "px";
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
  const y = L.cur * ROW;
  if (y < box.scrollTop) box.scrollTop = y;
  else if (y + ROW > box.scrollTop + box.clientHeight) box.scrollTop = y + ROW - box.clientHeight;
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
async function drawDetail() {
  const box = els.detail;
  const i = L.detail && findCopy(L.detail.ref);
  if (!i) { box.hidden = true; els.main.classList.remove("detail-open"); L.detail = null; return; }
  L.detail = i;
  box.hidden = false;
  const n = ++detailSeq;
  const scroll = box.querySelector(".scroll") || el("div", "scroll");
  box.innerHTML = "";
  const top = el("div", "lib-dhead");
  const back = el("button", "btn small", "← " + T("back"));
  back.append(el("kbd", "", "esc"));
  back.onclick = closeDetail;
  top.append(back, el("span", "t", i.label));
  box.append(top, scroll);
  scroll.innerHTML = "";
  const facts = el("div", "lib-facts");
  const kv = (label, value) => {
    if (!value) return;
    const r = el("div", "kv");
    r.append(el("span", "k", label), el("span", "v", value));
    facts.append(r);
  };
  kv(T("path"), i.path);
  kv(T("states"), (i.states || []).join(" · "));
  kv(T("used in"), (i.usedIn || []).join(" · "));
  kv(T("versions"), i.versions);
  kv(T("played"), i.played);
  kv(T("size"), i.size);
  const acts = el("div", "lib-acts");
  for (const a of i.actions || []) {
    const b = el("button", "btn" + (a.danger ? " danger" : ""), a.label);
    b.onclick = () => run(a, [i]);
    acts.append(b);
  }
  scroll.append(acts, facts);
  if (!i.id) return;
  const d = el("div", "lib-desc");
  d.id = "lib-detail";
  scroll.append(d);
  try {
    const det = await api.Detail(i.id, false, false, false);
    if (n !== detailSeq) return;
    renderDetail(det, d, scroll, true, { page: true });
  } catch (e) { if (n === detailSeq) d.append(el("p", "err", errText(e))); }
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
  if (a.kind === "folder") return api.OpenFolder(i0.id).catch((e) => say("fail", errText(e)));
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

// hints: the hint line for the page (06 §2.2).
export function hintList() {
  const list = [{ key: "↑↓", label: T("move") }];
  const x = L.flat[L.cur];
  if (x && x.t === "lin") list.push({ key: "→ ←", label: T("open / close") });
  if (x && x.t === "inst") list.push({ key: "enter", label: L.detail ? T("back") : T("details") });
  const acts = actionsOf(selected()).map(({ a }) => a.kind);
  if (acts.includes("enable")) list.push({ key: "space", label: T("add to the current plan") });
  if (acts.includes("workshop")) list.push({ key: "o", label: T("open the Workshop page") });
  list.push({ key: "/", label: T("search") }, { key: "ctrl+space", label: T("select") });
  list.push({ key: "?", label: T("all keys") });
  return list;
}
function hints() { ui.renderHints(); }
