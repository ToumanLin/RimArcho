// The plan editor's looks view (16 §2.5 Looks view, 21): which Mod's textures the game
// shows, for the list being edited. This file is the view's shell and its first layer: the
// editor | looks switch in the editor's head, the gallery (1), the Mods that supply textures
// (2), the editor's own load-order rules card (3) and the Preference card (4: RimArcho Looks
// Preference and RimArcho Styles Preference). A look
// opens the looks editor (lookseditor.js). Every winner, state, count and prediction comes
// from Go (internal/view/looks.go); nothing of them is worked out here.
import { S, $, el, esc, api, T, Tf, say, errText, cur, ROW_H } from "./state.js";
import { icon, sourceIcon } from "./icons.js";
import { wireSeparator } from "./separator.js";
import { copyablePath } from "./fields.js";
import { progress } from "./feedbacksemantics.js";
import * as pop from "./popups.js";
import { setRulesNumber, setRulesPlace, showRules } from "./rules.js";
import { createBar } from "./filterbar.js";
import { queryParts } from "./searchsuggest.js";
import { GRID, gridLayout, visibleRows, stepCard, scrollFor, texURL } from "./looksgrid.js";
import { applyGate, CONFIRMED_KEY, STYLES_CONFIRMED_KEY, applyLines, addLines } from "./looksconfirm.js";
import * as editor from "./lookseditor.js";

const VIEW_KEY = "ui.gui.plan.view", SCOPE_KEY = "ui.gui.looks.scope";
const FILTERS = ["overridden", "overriding", "mixed", "chosen", "nodef"];

const L = {
  ui: null,
  want: "editor", // the remembered view (ui.gui.plan.view), applied when the editor shows
  on: false,      // the looks view is shown instead of the board
  layer: 1,       // 1 the gallery, 2 the looks editor
  focus: 1,       // the numbered card of layer 1 that has the focus (16 §3.5)
  ov: null,       // view.LooksOverview
  gal: null,      // view.LooksGallery
  pick: null,     // what the gallery shows: { pos } a Mod, { chosen: true } the choices, with styles: true the looks with a style entry
  modCursor: 0,   // the Mods card's cursor (an index into mods())
  filter: "", search: "",
  bar: null,      // the filter bar (filterbar.js), a card of its own over the gallery
  barData: null,  // what the bar's panel lists: the look states, then the editor's groups (view.BarFilter)
  modQuery: "",   // the part of the query that filters Mods, as in the editor
  modSet: null,   // the plan positions that pass it; null: every Mod
  barSeq: 0,
  rows: [], layout: null, drawn: new Map(),
  sel: new Set(), cursor: -1, anchor: -1,
  seq: 0, galSeq: 0,
  building: null, // view LooksProgress while a read waits for the index
  error: "",
  warn: null,     // the dry run that found mixed results, until the player answers
  buildError: "", // why the last rebuild did not run (the choice is kept)
  stylesError: "", // the same for RimArcho Styles Preference after an add or a remove (the entry is kept)
  top: 0,
};

const root = () => $("#looks");
const scroller = () => $("#looks .lk-scroll");
const scopeWord = (s) => s === "all" ? T("all lists") : T("this list");
const num = (n) => Number(n || 0).toLocaleString("en-US");

export const active = () => L.on;
export const layer = () => (L.on ? L.layer : 0);

// ---- the shell: the switch, entering and leaving ----

export function initLooks(ui) {
  L.ui = ui;
  build();
  api.Setting(VIEW_KEY).then((v) => { L.want = v === "looks" ? "looks" : "editor"; sync(); }).catch(() => {});
}

let viewSwitch = null, scopeSwitch = null;
const segSwitch = editor.segSwitch;

function build() {
  const head = $("#planhead");
  // the switch is the first thing of the editor's second row, in both views and both layers
  viewSwitch = segSwitch("lk-switch", T("view"), [["editor", "editor"], ["looks", "looks"]], (v) => setView(v));
  head.prepend(viewSwitch);
  const way = el("span", "lk-way");
  const scope = el("span", "lk-scope");
  scopeSwitch = segSwitch("lk-scope-seg", T("set preference to"), [["plan", "current list"], ["all", "all"]], (v) => setScope(v));
  scope.append(el("span", "lk-lbl", T("set preference to")), scopeSwitch);
  $(".lead", head).after(way, scope);

  const r = root();
  r.innerHTML = "";
  const l1 = el("div", "lk-l1");
  const gal = el("section", "area lk-gal");
  gal.dataset.area = "looks-1";
  const refresh = el("button", "icon lk-refresh");
  refresh.type = "button";
  refresh.innerHTML = icon.rescan;
  refresh.onclick = () => load(true);
  // the editor's filter bar, a card of its own: the same query, plus the look states
  L.bar = createBar({
    cls: "lk-fbar", data: () => L.barData, change: () => barChanged(), splitKey: "ui.gui.filter.split:looks",
    column: () => $("#looks .lk-left"), resized: () => relayout(true), leave: () => focusCard(1),
    placeholder: () => T("search looks; filter Mods: tags:, source:, content:") + "  /",
  });
  const ghead = el("div", "lk-ghead");
  const gline = el("div", "lk-gline");
  gline.append(el("span", "anum", "1"), ghead, refresh);
  const sc = el("div", "lk-scroll");
  sc.tabIndex = -1;
  sc.setAttribute("role", "listbox");
  sc.setAttribute("aria-multiselectable", "true");
  sc.append(el("div", "lk-space"));
  gal.append(gline, sc, el("div", "lk-note"), el("div", "lk-act"));
  $(".lk-note", gal).hidden = true;
  $(".lk-act", gal).hidden = true;

  const split = el("div", "split lk-split");
  const side = el("div", "lk-side");
  const mods = el("section", "area lk-mods");
  mods.dataset.area = "looks-2";
  const mh = el("div", "head");
  const rows = el("div", "rows");
  rows.tabIndex = -1;
  rows.setAttribute("role", "listbox");
  mods.append(mh, rows);
  const pref = el("section", "area lk-pref");
  pref.dataset.area = "looks-4";
  pref.append(el("div", "head"), el("div", "lk-prefbody"));
  side.append(mods, pref);
  const left = el("div", "fcol lk-left");
  left.append(L.bar.el, gal);
  l1.append(left, split, side);
  const l2 = el("div", "lk-l2");
  l2.hidden = true;
  r.append(l1, l2);

  // the gap between the gallery and the right column is a splitter (16 §3.1 rule 10)
  wireSeparator(split, {
    key: "ui.gui.looks.side", direction: -1,
    get: () => side.getBoundingClientRect().width,
    range: () => [260, Math.max(260, l1.getBoundingClientRect().width - 320)],
    set: (v) => l1.style.setProperty("--lk-side", v + "px"),
    reset: () => l1.style.removeProperty("--lk-side"),
    restore: (v) => { if (+v) l1.style.setProperty("--lk-side", +v + "px"); },
    changed: () => relayout(true),
  });

  let frame = 0;
  sc.addEventListener("scroll", () => { if (!frame) frame = requestAnimationFrame(() => { frame = 0; drawRows(); }); }, { passive: true });
  let resized = 0;
  new ResizeObserver(() => { if (!resized) resized = requestAnimationFrame(() => { resized = 0; if (L.on && L.layer === 1) relayout(true); }); }).observe(sc);
  sc.addEventListener("mousedown", (e) => { if (e.shiftKey || e.ctrlKey || e.metaKey) e.preventDefault(); }); // a range click selects cards, not their text
  sc.addEventListener("click", galleryClick);
  // a texture the index no longer has (it is dropped after five idle minutes): read again once
  sc.addEventListener("error", (e) => { if (e.target.tagName === "IMG") revive(e.target); }, true);
  for (const [card, n] of [[gal, 1], [mods, 2], [pref, 4]]) card.addEventListener("mousedown", () => markFocus(n), true);
  rows.addEventListener("scroll", () => drawMods(), { passive: true });
  rows.addEventListener("click", (e) => {
    const row = e.target.closest(".row");
    if (row) showMod(+row.dataset.i);
  });
  $(".lk-act", gal).addEventListener("click", actClick);
  editor.initEditor({
    host: l2, way, ui: L.ui,
    scope: () => L.scope || "plan",
    overview: () => L.ov,
    setOverview: (ov) => { if (ov) { L.ov = ov; drawSide(); } },
    reading, gate, wrote, styleGate, wroteStyle, back: () => closeLook(), scopeWord, modAt,
  });
}

// sync runs with every drawing of the editor (main.js render): the switch follows the view,
// and the remembered view is applied the first time the editor shows.
export function sync() {
  if (!viewSwitch) return;
  if (!S.screen && S.board && !S.board.error && L.want === "looks" && !L.on) enter();
  $("#planhead").classList.toggle("looks-on", L.on);
  viewSwitch.paint(L.on ? "looks" : "editor");
  scopeSwitch.paint(L.scope || "plan");
  if (L.on) planChanged();
}

export async function setView(v) {
  v = v === "looks" ? "looks" : "editor";
  if (L.want !== v) { L.want = v; api.SetSetting(VIEW_KEY, v).catch(() => {}); }
  if (v === "looks" && !L.on) enter(); else if (v === "editor" && L.on) leave();
  L.ui.renderWhere();
  L.ui.renderHints();
}

let rulesHome = null;
function placeRules(here) {
  const card = $(".rulesarea");
  if (!card) return;
  if (here) {
    if (!rulesHome) rulesHome = { parent: card.parentElement, next: card.nextElementSibling };
    $("#looks .lk-side").insertBefore(card, $("#looks .lk-pref"));
    card.dataset.area = "looks-3";
    setRulesNumber("3");
    setRulesPlace(true);
  } else if (rulesHome) {
    rulesHome.parent.insertBefore(card, rulesHome.next);
    delete card.dataset.area;
    card.classList.remove("focus");
    setRulesNumber("");
    setRulesPlace(false);
    rulesHome = null;
  }
}

// enter shows the looks view in the board's place; the board keeps its rows (it is hidden,
// not built again when the player comes back).
function enter() {
  L.on = true;
  $("#board").hidden = true;
  root().hidden = false;
  placeRules(true);
  $("#planhead").classList.add("looks-on");
  viewSwitch.paint("looks");
  L.plan = planKey();
  if (L.layer === 2) editor.show(); else markFocus(L.focus);
  load(false);
}

function leave() {
  L.on = false;
  L.seq++; L.galSeq++;
  L.building = null;
  editor.hide();
  L.layer = 1;
  $("#looks .lk-l1").hidden = false;
  $("#looks .lk-l2").hidden = true;
  root().hidden = true;
  placeRules(false);
  $("#board").hidden = false;
  $("#planhead").classList.remove("looks-on");
  viewSwitch.paint("editor");
  clearPicked();
  drop();
  // the index is dropped in Go as the view is left (16 §10); the next visit builds it again
  api.LooksLeave().catch(() => {});
  L.ui.render();
  $("#board").focus({ preventScroll: true });
}

function drop() {
  L.ov = null; L.gal = null; L.rows = []; L.layout = null; L.pick = null; L.error = ""; L.warn = null;
  L.modSet = null; L.barEditor = null; L.modQuery = "";
  for (const e of L.drawn.values()) e.remove();
  L.drawn.clear();
}

const planKey = () => JSON.stringify([S.board?.env, S.shell?.plan?.id]);

// planChanged: the editor was drawn again. Another list or game: the view starts over. The
// same list: its looks are read again only when the list's Mods or their order moved.
let checking = false;
async function planChanged() {
  if (!L.on || checking) return;
  if (L.plan !== planKey()) {
    L.plan = planKey();
    if (L.layer === 2) closeLook(false);
    clearPicked(); drop();
    return load(false);
  }
  if (!L.ov || L.building) return;
  checking = true;
  try {
    const ov = await api.LooksOverview();
    if (!L.on || !L.ov) return;
    const moved = ov.stamp !== L.ov.stamp;
    setOverview(ov);
    drawSide();
    if (moved) { if (L.layer === 2) editor.reload(); else loadGallery(true); }
  } catch { /* the next read says why */ } finally { checking = false; }
}

// reading runs a looks read; while it waits for the index to be built it shows the build's
// progress (the cold build of a large list takes seconds).
let waits = 0;
async function reading(call) {
  let live = true, timer = 0;
  const poll = async () => {
    if (!live) return;
    try {
      const g = await api.LooksProgress();
      if (live) { L.building = g.building ? g : null; drawWait(); }
    } catch { /* the read itself reports */ }
    if (live) timer = setTimeout(poll, 300);
  };
  waits++;
  timer = setTimeout(poll, 250);
  try { return await call(); } finally {
    live = false;
    clearTimeout(timer);
    if (!--waits) { L.building = null; drawWait(); }
  }
}

async function load(fresh) {
  const n = ++L.seq;
  L.error = "";
  const b = $("#looks .lk-refresh");
  if (fresh) b.classList.add("spin");
  drawNote();
  let ov;
  try { ov = await reading(() => (fresh ? api.LooksRefresh() : api.LooksOverview())); } catch (e) {
    b.classList.remove("spin");
    if (n === L.seq && L.on) { L.error = errText(e); drawNote(); }
    return;
  }
  b.classList.remove("spin");
  if (n !== L.seq || !L.on) return;
  setOverview(ov);
  drawSide();
  if (L.layer === 2) return editor.reload();
  await loadGallery(fresh);
  barChanged(); // the panel's groups, and the Mods the query passes
}

function setOverview(ov) {
  L.ov = ov;
  L.scope = ov.scope === "all" ? "all" : "plan";
  scopeSwitch.paint(L.scope);
  const list = mods();
  if (L.pick && !L.pick.chosen && !list.some((m) => m.pos === L.pick.pos)) L.pick = null;
  if (!L.pick) {
    // the Mod under the editor's cursor, when it supplies textures; else the first one
    const id = S.detail?.id || cur(1)?.id;
    const at = Math.max(0, list.findIndex((m) => m.id === id));
    L.pick = list.length ? { pos: list[at].pos } : { chosen: true };
    L.modCursor = at;
  } else if (!L.pick.chosen) L.modCursor = Math.max(0, list.findIndex((m) => m.pos === L.pick.pos));
}

// the Mods the Mods card lists: load order, without RimArcho Looks Preference (it has its own card)
const mods = () => (L.ov ? L.ov.mods.filter((m) => !m.generated && (!L.modSet || L.modSet.has(m.pos))) : []);
const modAt = (pos) => (L.ov ? L.ov.mods.find((m) => m.pos === pos) : null) || null;
const pickedMod = () => (L.pick && !L.pick.chosen && !L.pick.none && !L.search ? modAt(L.pick.pos) : null);

async function setScope(v) {
  if (v === L.scope) return;
  L.scope = v;
  if (L.ov) L.ov.scope = v;
  scopeSwitch.paint(v);
  api.SetSetting(SCOPE_KEY, v).catch(() => {});
  L.warn = null;
  drawAct();
  if (L.layer === 2) editor.reload();
}

// ---- the gallery ----

// barQuery splits the bar's query (16 §2.5 Looks view): looks:<state> filters the looks, bare
// words search the looks of the whole list, and everything else filters the Mods as in the
// editor. One look state at a time: the last one stays in the query.
function barQuery() {
  const parts = queryParts(L.bar.search);
  const states = parts.filter((p) => p.key === "looks");
  const state = states.filter((p) => !p.negative && FILTERS.includes(p.value.toLowerCase())).pop();
  if (states.length > (state ? 1 : 0)) L.bar.setText(parts.filter((p) => p.key !== "looks" || p === state).map((p) => p.raw).join(" "));
  return {
    filter: state ? state.value.toLowerCase() : "",
    search: parts.filter((p) => !p.key && !p.negative).map((p) => p.value).join(" "),
    mods: parts.filter((p) => p.key !== "looks" && (p.key || p.negative)).map((p) => p.raw).join(" "),
  };
}

function barGroups(editor) {
  const counts = (L.gal && L.gal.counts) || {};
  const label = { overridden: T("overridden"), overriding: T("overriding"), mixed: T("mixed"), chosen: T("chosen"), nodef: T("no def") };
  const values = FILTERS.map((f) => ({ filter: "looks:" + f, label: label[f], count: counts[f] || 0, on: L.filter === f ? 1 : 0 }));
  return [{ key: "looks", title: T("looks"), values }, ...((editor && editor.groups) || [])];
}

// barChanged: the bar's query or panel changed. The Mods card keeps the Mods the query
// passes, the gallery the looks.
async function barChanged() {
  if (!L.on || !L.ov) return;
  const n = ++L.barSeq, q = barQuery();
  const fresh = q.filter !== L.filter || q.search !== L.search || q.mods !== L.modQuery;
  L.filter = q.filter; L.search = q.search; L.modQuery = q.mods;
  let d = null;
  try { d = await api.BarFilter(q.mods, [], true); } catch { d = null; }
  if (n !== L.barSeq || !L.on) return;
  L.barEditor = d;
  L.modSet = q.mods && d ? new Set((d.rows || [])[1] || []) : null;
  const list = mods();
  if (L.pick && !L.pick.chosen && !list.some((m) => m.pos === L.pick.pos)) {
    L.pick = list.length ? { pos: list[0].pos } : { none: true };
    L.modCursor = 0;
  } else if (L.pick?.none && list.length) { L.pick = { pos: list[0].pos }; L.modCursor = 0; }
  drawSide();
  if (fresh) { clearPicked(); await loadGallery(); }
  if (n !== L.barSeq) return;
  L.barData = { groups: barGroups(d), conds: (d && d.conds) || [], complete: d ? d.complete : "" };
  L.bar.draw();
}

async function loadGallery(keep = false) {
  if (!L.on || !L.ov) return;
  const n = ++L.galSeq, top = scroller().scrollTop, at = L.rows[L.cursor]?.id;
  if (L.pick?.none && !L.search) { // no Mod passes the bar's conditions
    L.galSeq++; L.error = ""; L.gal = { groups: [], counts: {}, rows: 0, stamp: L.ov.stamp }; L.rows = [];
    drawFilter(); drawHead(); relayout(false); drawNote(); drawAct(); L.ui.renderWhere();
    return;
  }
  const args = L.search ? ["", L.filter, L.search] : L.pick?.chosen ? [L.pick.styles ? "styles" : "chosen", L.filter, ""] : [String(L.pick ? L.pick.pos : ""), L.filter, ""];
  let g;
  try { g = await reading(() => api.LooksGallery(...args)); } catch (e) {
    if (n === L.galSeq && L.on) { L.error = errText(e); drawNote(); }
    return;
  }
  if (n !== L.galSeq || !L.on) return;
  L.error = "";
  L.gal = g;
  L.rows = (g.groups || []).flatMap((grp) => grp.rows || []);
  const alive = new Set(L.rows.map((r) => r.id));
  for (const id of [...L.sel]) if (!alive.has(id)) L.sel.delete(id);
  L.cursor = keep ? L.rows.findIndex((r) => r.id === at) : -1;
  L.anchor = -1;
  L.warn = null;
  drawFilter();
  drawHead();
  relayout(false);
  scroller().scrollTop = keep ? top : 0;
  drawRows();
  drawNote();
  drawAct();
  L.ui.renderWhere();
}

// relayout places the cards for the box's width; keep: the same look stays at the top.
function relayout(keep) {
  const sc = scroller();
  if (!sc || !L.gal) return;
  const width = sc.clientWidth;
  if (!width) return; // hidden (the looks editor is open): laid out when it shows again
  const top = sc.scrollTop;
  const groups = (L.gal.groups || []).map((grp) => ({ spans: (grp.rows || []).map((r) => (r.thumbs || []).length) }));
  L.layout = gridLayout(groups, width);
  $(".lk-space", sc).style.height = L.layout.total + "px";
  for (const e of L.drawn.values()) e.remove();
  L.drawn.clear();
  if (keep) sc.scrollTop = top;
  drawRows();
}

function groupOf(gi) { return (L.gal.groups || [])[gi]; }
// a row's card ({ x, w, n }) names the look by its place n in reading order
function rowOf(card) { const c = card.group === undefined ? L.layout.cards[card.n] : card; return groupOf(c.group).rows[c.item]; }

// drawRows keeps only the rows in view (and a small margin) in the page (16 §10).
function drawRows() {
  const sc = scroller(), lay = L.layout;
  if (!lay || !L.on) return;
  const [a, b] = visibleRows(lay, sc.scrollTop, sc.clientHeight);
  for (const [k, e] of L.drawn) if (k < a || k >= b) { e.remove(); L.drawn.delete(k); }
  const frag = document.createDocumentFragment();
  for (let r = a; r < b; r++) {
    if (L.drawn.has(r)) continue;
    const row = lay.rows[r], box = el("div", "lk-row");
    box.style.top = row.y + "px";
    box.style.height = row.h - GRID.gap + "px";
    const y = row.heads.length ? GRID.head : 0;
    box.innerHTML = row.heads.map((h) => groupHead(h)).join("") + row.cards.map((c) => card(c, y)).join("");
    L.drawn.set(r, box);
    frag.append(box);
  }
  $(".lk-space", sc).append(frag);
  const at = L.cursor >= 0 ? $(`#lk-card-${L.cursor}`, sc) : null;
  if (at) sc.setAttribute("aria-activedescendant", at.id); else sc.removeAttribute("aria-activedescendant");
}

function groupHead(h) {
  const grp = groupOf(h.group), rows = grp.rows || [];
  const all = rows.length > 0 && rows.every((r) => L.sel.has(r.id));
  return `<div class="lk-gh" style="left:${GRID.padX + h.x}px;width:${h.w}px" data-group="${h.group}">` +
    `<span class="gn" title="${esc(grp.category || "")}">${esc(grp.category || T("textures"))}</span><span class="gc">${num(rows.length)}</span>` +
    `<button type="button" tabindex="-1" class="lk-all${all ? " on" : ""}">${esc(all ? T("clear") : T("select all"))}</button></div>`;
}

// mark: the state of a look in words (16 §2.5): for the selected Mod, whether the game shows
// its textures or whose instead; a choice's state; mixed.
function mark(row) {
  const mod = !!pickedMod();
  if (row.choice === "broken") return { cls: "bad", text: Tf("choice broken: %s", row.from || "—") };
  if (row.choice === "asleep") return { cls: "dim", text: Tf("choice asleep: %s", row.from || "—") };
  if (mod && row.inGame) return { cls: "in", text: row.choice === "applied" ? T("in game, chosen") : T("in game") };
  if (mod && row.overriddenBy) return { cls: "ov", lead: T("overridden by"), text: row.overriddenBy };
  if (row.choice === "applied") return { cls: "ch", text: Tf("chosen: %s", row.from || "—") };
  if (row.choice === "not needed") return { cls: "ch", text: Tf("chosen, not needed: %s", row.from || "—") };
  if (row.mixed) return { cls: "mix", text: T("mixed") };
  return { cls: "", text: row.n === 1 ? T("1 Mod") : Tf("%d Mods", row.n) };
}

function thumbs(row) {
  const list = row.thumbs || [], stamp = L.gal.stamp, two = list.some((t) => t.own);
  return list.map((t, i) => {
    const pic = t.noPreview
      ? `<span class="lk-fmt">${esc(Tf("%s, no preview", t.noPreview))}</span>`
      : `<img src="${esc(texURL(row.id, t.p, t.s, 192, stamp))}" loading="lazy" decoding="async" alt="" data-p="${t.p}" data-s="${esc(t.s)}">`;
    const tag = two ? `<span class="lk-tag">${esc(t.own ? T("this Mod") : T("in game"))}</span>` : "";
    const more = i === list.length - 1 && row.more > 0 ? `<span class="lk-more">+${num(row.more)}</span>` : "";
    return `<div class="lk-tp${t.own ? " own" : ""}">${pic}${tag}${more}</div>`;
  }).join("");
}

function card(c, y) {
  const row = rowOf(c), m = mark(row);
  const sel = L.sel.has(row.id), at = c.n === L.cursor;
  const state = (m.lead ? m.lead + " " : "") + m.text + (row.mixed && m.cls !== "mix" ? ", " + T("mixed") : "");
  return `<div class="lk-card${sel ? " sel" : ""}${at ? " cur" : ""}" id="lk-card-${c.n}" role="option" aria-selected="${sel}" data-n="${c.n}" ` +
    `aria-label="${esc(row.title + ", " + state)}" style="left:${GRID.padX + c.x}px;top:${y}px;width:${c.w}px">` +
    `<div class="lk-th">${thumbs(row)}${row.kind === "worn" ? `<span class="lk-kind">${esc(T("worn"))}</span>` : ""}</div>` +
    `<span class="lk-ck">${icon.ok}</span>` +
    `<div class="lk-nm" title="${esc(row.id)}">${esc(row.title)}</div>` +
    `<div class="lk-mk ${m.cls}" title="${esc(state)}">${m.lead ? `<span class="w">${esc(m.lead)}</span>` : ""}<span>${esc(m.text)}</span>` +
    `${row.mixed && m.cls !== "mix" ? `<span class="x">${esc(T("mixed"))}</span>` : ""}</div></div>`;
}

// revive: a texture answered 404. The index is dropped when nothing read it for a while; one
// read brings it back, then the texture is asked for once more.
let reviving = null;
function revive(img) {
  if (img.dataset.again || !L.on) return;
  img.dataset.again = "1";
  const src = img.getAttribute("src");
  reviving = reviving || api.LooksOverview().finally(() => { setTimeout(() => { reviving = null; }, 2000); });
  reviving.then(() => { if (img.isConnected) img.src = src; }, () => {});
}

function drawFilter() {
  L.barData = { groups: barGroups(L.barEditor), conds: (L.barEditor && L.barEditor.conds) || [], complete: L.barEditor ? L.barEditor.complete : "" };
  L.bar.draw();
  const refresh = $("#looks .lk-refresh");
  refresh.title = T("refresh: read the Mod folders again");
  refresh.setAttribute("aria-label", refresh.title);
}

function drawHead() {
  const h = $("#looks .lk-ghead"), g = L.gal;
  if (!g) { h.replaceChildren(); return; }
  const shown = g.filter ? Tf("%s of %s looks", num(g.rows + (g.more || 0)), num(g.counts.all)) : (g.counts.all === 1 ? T("1 look") : Tf("%s looks", num(g.counts.all)));
  const m = pickedMod();
  h.replaceChildren();
  if (L.search) h.append(el("b", "mname", "“" + L.search + "”"));
  else if (m) {
    const src = el("span", "src");
    src.innerHTML = sourceIcon(m.kind);
    h.append(src, el("b", "mname", m.name), el("span", "pos", "#" + m.no));
    h.lastChild.previousSibling.title = m.name;
  } else h.append(el("b", "mname", genName()));
  h.append(el("span", "gcnt", shown));
  if (m && m.overridden) h.append(el("span", "gov", Tf("%s overridden", num(m.overridden))));
  if (g.more) h.append(el("span", "gov", Tf("the first %s are shown: search or filter to see the rest", num(g.rows))));
  $("#looks .lk-scroll").setAttribute("aria-label", (m ? m.name : L.search || genName()) + ", " + shown);
}

// drawNote: what the gallery says in the place of its cards: the index being built, a read
// that failed, or why nothing is listed.
function drawNote() {
  const box = $("#looks .lk-note");
  if (!box) return;
  let text = "", retry = false;
  if (L.error) { text = L.error; retry = true; }
  else if (L.building || !L.ov) return drawWait();
  else if (!L.ov.looks) text = T("no Mod of this list ships textures");
  else if (L.gal && !L.rows.length) {
    const m = pickedMod();
    if (L.search) text = Tf("no look matches “%s”", L.search);
    else if (L.pick?.none) text = T("no Mod with textures passes the filter");
    else if (L.filter && m) text = Tf("%s has no look under this filter", m.name);
    else if (L.filter) text = T("no look under this filter");
    else if (L.pick?.styles) text = T("no style or random variant yet: open a look, select a Mod and add");
    else if (L.pick?.chosen) text = T("no choice yet: open a look, select a Mod and apply");
    else text = T("this Mod has no textures");
  }
  box.hidden = !text;
  box.className = "lk-note" + (L.error ? " bad" : "");
  box.replaceChildren();
  if (!text) return;
  box.append(el("p", "", text));
  if (retry) {
    const b = el("button", "btn small", T("try again"));
    b.onclick = () => load(false);
    box.append(b);
  } else if (L.filter && L.ov.looks) {
    const b = el("button", "btn small", T("clear the filter"));
    b.onclick = () => L.bar.set("looks:" + L.filter, 0);
    box.append(b);
  }
}

// drawWait: the index is being built. The view stays usable: editor in the switch leaves it.
function drawWait() {
  const box = $("#looks .lk-note");
  if (!box || L.error) return;
  const g = L.building;
  if (!g && L.ov) { if (box.classList.contains("wait")) { box.hidden = true; box.className = "lk-note"; drawNote(); } return; }
  box.hidden = false;
  box.className = "lk-note wait";
  let p = $(".prog", box);
  if (!p) {
    box.replaceChildren();
    p = el("div", "prog");
    const bar = el("div", "bar");
    bar.append(el("i"));
    p.append(el("p", "ptitle", T("reading the textures of this list…")), bar, el("div", "ptext"));
    box.append(p);
  }
  const done = g ? g.done : 0, total = g ? g.total : 0;
  $("i", p).style.width = total ? Math.round(100 * done / total) + "%" : "0%";
  $(".ptext", p).textContent = total ? Tf("%s / %s Mods", num(done), num(total)) : "";
  progress($(".bar", p), $(".ptitle", p), total ? done : undefined, total || 100);
}

// ---- the right column ----

function drawSide() {
  drawModsHead();
  drawMods(true);
  drawPref();
}

function drawModsHead() {
  const list = mods(), h = $("#looks .lk-mods .head");
  h.innerHTML = `<span class="n">2</span><span class="t" id="lk-mods-t">${esc(T("Mods"))}</span><span class="num">${num(list.length)}</span>`;
  $("#looks .lk-mods .rows").setAttribute("aria-labelledby", "lk-mods-t");
}

// drawMods: the editor's rows, one line each, in load order. The share of a Mod's looks that
// a later Mod overrides is a tint across the row; the number of its looks is at the right.
function drawMods(reset = false) {
  const box = $("#looks .lk-mods .rows");
  if (!box) return;
  const list = mods();
  let pad = box.firstElementChild;
  if (!pad || !pad.classList.contains("pad") || reset) {
    box.replaceChildren();
    pad = el("div", "pad");
    box.append(pad);
    if (!list.length) box.append(el("div", "empty", L.modSet ? T("no Mod with textures passes the filter") : T("no Mod of this list ships textures")));
  }
  pad.style.height = list.length * ROW_H + "px";
  box.style.setProperty("--pw", Math.max(3, String(list.length ? list[list.length - 1].no : 0).length) + "ch");
  const first = Math.max(0, Math.floor(box.scrollTop / ROW_H) - 4);
  const last = Math.min(list.length, Math.ceil((box.scrollTop + (box.clientHeight || 400)) / ROW_H) + 5);
  const shown = pickedMod();
  const have = new Map([...box.querySelectorAll(".row")].map((r) => [+r.dataset.i, r]));
  for (const [i, r] of have) if (i < first || i >= last) r.remove();
  for (let i = first; i < last; i++) {
    const m = list[i];
    const sig = [m.pos, m.name, m.looks, m.overridden, shown && shown.pos === m.pos, i === L.modCursor].join("|");
    let r = have.get(i);
    if (r && r.dataset.sig === sig && r.isConnected) continue;
    if (!r || !r.isConnected) { r = el("div", "row"); box.append(r); }
    r.dataset.sig = sig;
    r.dataset.i = i;
    r.id = "lk-mod-" + m.pos;
    r.className = "row" + (shown && shown.pos === m.pos ? " sel" : "") + (i === L.modCursor ? " cur" : "");
    r.style.top = i * ROW_H + "px";
    r.setAttribute("role", "option");
    r.setAttribute("aria-selected", String(!!shown && shown.pos === m.pos));
    const share = m.looks ? Math.min(100, Math.max(m.overridden ? 2 : 0, 100 * m.overridden / m.looks)) : 0;
    const words = m.overridden ? Tf("%s of its %s looks are overridden by a later Mod", num(m.overridden), num(m.looks)) : Tf("%s looks, none overridden", num(m.looks));
    r.title = m.name + "\n" + words;
    r.setAttribute("aria-label", m.name + ", " + words);
    r.innerHTML = `<i class="lk-ovl" style="width:${share.toFixed(1)}%"></i><span class="m"></span><span class="p">${m.no}</span>` +
      `<span class="k">${sourceIcon(m.kind)}</span><span class="t">${esc(m.name)}</span><span class="lk-n">${num(m.looks)}</span>`;
  }
  const at = list[L.modCursor];
  if (at) box.setAttribute("aria-activedescendant", "lk-mod-" + at.pos); else box.removeAttribute("aria-activedescendant");
}

function modIntoView() {
  const box = $("#looks .lk-mods .rows"), y = L.modCursor * ROW_H;
  if (y < box.scrollTop) box.scrollTop = y;
  else if (y + ROW_H > box.scrollTop + box.clientHeight) box.scrollTop = y + ROW_H - box.clientHeight;
}

// showMod shows the looks of the Mod at index i of the Mods card (a click, or its cursor).
let following = 0;
function showMod(i, soon = false) {
  const list = mods();
  if (!list[i]) return;
  L.modCursor = i;
  L.pick = { pos: list[i].pos };
  dropWords();
  clearPicked();
  modIntoView();
  drawMods();
  drawPref();
  clearTimeout(following);
  if (soon) following = setTimeout(() => loadGallery(), 120); else loadGallery();
}

// showModByID: a Mod named elsewhere (a rule's Mod, the command panel); false when it
// supplies no texture.
export function showModByID(id) {
  const i = mods().findIndex((m) => m.id === id);
  if (!L.on || i < 0) return false;
  if (L.layer === 2) closeLook(false);
  showMod(i);
  focusCard(2);
  return true;
}

// ruleTarget: the Mod a new load-order rule is about (the rules card's add a rule, r).
// the generated Mod whose looks the gallery shows when no Mod is selected
const genName = () => (L.pick?.styles ? L.ov?.styles?.name || "RimArcho Styles Preference" : L.ov?.generated.name || "RimArcho Looks Preference");
export function ruleTarget() {
  const m = mods()[L.modCursor];
  return m ? { id: m.id, name: m.name } : null;
}

function showChosen(styles) {
  L.pick = styles ? { chosen: true, styles: true } : { chosen: true };
  dropWords();
  clearPicked();
  drawMods();
  drawPref();
  loadGallery();
}

// drawPref: the Preference card: RimArcho Looks Preference with its choices by state and
// RimArcho Styles Preference with its entries by state, what RimArcho wrote in each, and the
// folder that holds both.
function drawPref() {
  const card = $("#looks .lk-pref");
  if (!card) return;
  $(".head", card).innerHTML = `<span class="n">4</span><span class="t">${esc(T("preference"))}</span>`;
  const body = $(".lk-prefbody", card), ov = L.ov;
  body.replaceChildren();
  if (!ov) return;
  const g = ov.generated, st = ov.styles;
  const states = (parts, none) => {
    const box = el("span", "pv");
    const shown = parts.filter(([n]) => n > 0);
    if (!shown.length) box.append(el("span", "", none));
    for (const [n, f, cls] of shown) box.append(el("span", cls, Tf(f, num(n))));
    return box;
  };
  const facts = (x) => {
    const box = el("div", "lk-pfacts");
    const how = !x.files ? "" : x.copied && x.linked ? Tf("%s linked, %s copied", num(x.linked), num(x.copied)) : x.copied ? T("copied") : T("linked, not copied");
    box.append(el("span", "", x.files === 1 ? T("1 file") : Tf("%s files", num(x.files))));
    if (x.files) box.append(el("span", "", x.size), el("span", "", how));
    return box;
  };
  const row = el("button", "lk-prow lk-plooks" + (L.pick?.chosen && !L.pick.styles && !L.search ? " sel" : ""));
  row.type = "button";
  row.title = T("show the looks that have a choice");
  row.setAttribute("aria-pressed", String(!!L.pick?.chosen && !L.pick.styles && !L.search));
  row.onclick = () => showChosen();
  row.append(el("span", "pn", g.name), states([[g.applied, "%s applied", ""], [g.notNeeded, "%s not needed", ""], [g.asleep, "%s asleep", "dim"], [g.broken, "%s broken", "bad"]], T("no choice yet")));
  body.append(row, facts(g));
  if (st) {
    const srow = el("button", "lk-prow lk-pstyles" + (L.pick?.styles && !L.search ? " sel" : ""));
    srow.type = "button";
    srow.title = T("show the looks that have a style or random variant");
    srow.setAttribute("aria-pressed", String(!!L.pick?.styles && !L.search));
    srow.onclick = () => showChosen(true);
    srow.append(el("span", "pn", st.name), states([[st.active, "%s active", ""], [st.skipped, "%s skipped", "dim"], [st.asleep, "%s asleep", "dim"], [st.broken, "%s broken", "bad"]], T("no style or random variant yet")));
    const sfacts = facts(st);
    sfacts.classList.add("lk-psfacts");
    body.append(srow, sfacts);
  }
  const dir = el("div", "lk-pdir");
  copyablePath(dir, g.dir);
  body.append(dir);
  if (g.dirChosen) body.append(el("p", "lk-pnote", T("the first apply or add creates this folder")));
  const note = (text, cls, button) => {
    const p = el("p", "lk-pnote " + cls, text);
    if (button) p.append(button);
    body.append(p);
  };
  const rebuild = () => {
    const b = el("button", "btn small", T("rebuild"));
    b.dataset.envWrite = "looks";
    b.onclick = () => doRebuild();
    return b;
  };
  if (ov.gameRunning) note(T("a game is running: a choice is stored now, and RimArcho Looks Preference is rebuilt at the next write to the game"), "warn");
  if (L.buildError) note(Tf("RimArcho Looks Preference was not rebuilt: %s", L.buildError), "warn", ov.gameRunning ? null : rebuild());
  else if (!g.entry && g.applied > 0) note(T("RimArcho Looks Preference is not in this list, so its choices do not reach the game"), "warn", rebuild());
  if (L.stylesError) note(Tf("RimArcho Styles Preference was not rebuilt: %s", L.stylesError), "warn lk-pstyleserr");
  else if (st && !st.entry && st.active > 0) note(T("RimArcho Styles Preference is not in this list, so its styles and random variants do not reach the game; adding one in this list puts it in"), "warn");
  if (ov.warnings) note(Tf("%s texture files could not be read", num(ov.warnings)), "dim");
}

async function doRebuild() {
  try {
    const d = await api.LooksRebuild();
    wrote(d);
  } catch (e) { say("fail", errText(e)); }
}

// ---- selection and the action bar ----

function clearPicked() { L.sel.clear(); L.anchor = -1; L.warn = null; drawAct(); }

function paintPicked() {
  const sc = scroller();
  for (const c of sc.querySelectorAll(".lk-card")) {
    const row = L.rows[+c.dataset.n], sel = !!row && L.sel.has(row.id);
    c.classList.toggle("sel", sel);
    c.classList.toggle("cur", +c.dataset.n === L.cursor);
    c.setAttribute("aria-selected", String(sel));
  }
  for (const h of sc.querySelectorAll(".lk-gh")) {
    const rows = groupOf(+h.dataset.group).rows || [], all = rows.length > 0 && rows.every((r) => L.sel.has(r.id));
    const b = $(".lk-all", h);
    b.classList.toggle("on", all);
    b.textContent = all ? T("clear") : T("select all");
  }
  $("#looks .lk-gal").classList.toggle("picking", L.sel.size > 0);
  const at = L.cursor >= 0 ? $(`#lk-card-${L.cursor}`, sc) : null;
  if (at) sc.setAttribute("aria-activedescendant", at.id); else sc.removeAttribute("aria-activedescendant");
  L.warn = null;
  drawAct();
  L.ui.renderWhere();
  L.ui.renderHints();
}

function setCursor(n, reveal = true) {
  if (!L.rows.length) { L.cursor = -1; return; }
  L.cursor = Math.max(0, Math.min(L.rows.length - 1, n));
  if (reveal && L.layout) {
    const sc = scroller();
    sc.scrollTop = scrollFor(L.layout, L.cursor, sc.scrollTop, sc.clientHeight);
    drawRows();
  }
}

function toggle(n, on) {
  const row = L.rows[n];
  if (!row) return;
  if (on ?? !L.sel.has(row.id)) L.sel.add(row.id); else L.sel.delete(row.id);
}

function galleryClick(e) {
  const all = e.target.closest(".lk-all");
  if (all) {
    const rows = groupOf(+all.closest(".lk-gh").dataset.group).rows || [];
    const on = !rows.every((r) => L.sel.has(r.id));
    for (const r of rows) on ? L.sel.add(r.id) : L.sel.delete(r.id);
    return paintPicked();
  }
  const c = e.target.closest(".lk-card");
  if (!c) return;
  const n = +c.dataset.n;
  if (e.shiftKey && (L.anchor >= 0 || L.cursor >= 0)) {
    const from = L.anchor >= 0 ? L.anchor : L.cursor;
    for (let i = Math.min(from, n); i <= Math.max(from, n); i++) toggle(i, true);
    L.anchor = from;
  } else if (e.ctrlKey || e.metaKey || L.sel.size) { toggle(n); L.anchor = n; }
  else { setCursor(n, false); paintPicked(); return openLook(L.rows[n]); }
  setCursor(n, false);
  paintPicked();
}

const picked = () => L.rows.filter((r) => L.sel.has(r.id));

function drawAct() {
  const a = $("#looks .lk-act");
  if (!a) return;
  const n = L.sel.size;
  a.hidden = !n;
  a.className = "lk-act" + (L.warn ? " warning" : "");
  a.replaceChildren();
  if (!n) return;
  const btn = (label, act, cls = "") => {
    const b = el("button", "btn " + cls, label);
    b.type = "button";
    b.dataset.act = act;
    if (act !== "clear" && act !== "cancel") b.dataset.envWrite = "looks";
    return b;
  };
  if (L.warn) {
    const w = L.warn, whole = w.done.done - w.done.mixed;
    const text = el("span", "aw");
    text.innerHTML = icon.warn;
    text.append(el("span", "", Tf("%s of %s would be mixed: some of their textures stay with another Mod", num(w.done.mixed), num(w.done.done))));
    a.append(text);
    if (whole > 0) a.append(btn(Tf("only the %s not mixed", num(whole)), "whole", "primary"));
    a.append(btn(Tf("use for all %s", num(w.done.done)), "all", whole > 0 ? "" : "primary"), btn(T("cancel"), "cancel"));
    return;
  }
  const m = pickedMod(), chosen = picked().filter((r) => r.choice).length;
  a.append(el("span", "an", n === 1 ? T("1 selected") : Tf("%s selected", num(n))));
  if (m) {
    const use = btn(Tf("use %s for these", m.name), "use", "primary");
    use.title = use.textContent;
    a.append(use);
  }
  const undo = btn(T("undo"), "undo");
  undo.title = Tf("remove the choices of these looks for %s", scopeWord(L.scope));
  undo.disabled = !chosen;
  const clear = btn("", "clear", "lk-x");
  clear.innerHTML = icon.close;
  clear.title = T("clear the selection");
  clear.setAttribute("aria-label", clear.title);
  a.append(undo, clear);
}

function actClick(e) {
  const b = e.target.closest("[data-act]");
  if (!b || b.disabled) return;
  const act = b.dataset.act;
  if (act === "clear") { clearPicked(); return paintPicked(); }
  if (act === "cancel") { L.warn = null; return drawAct(); }
  if (act === "undo") return undoPicked();
  if (act === "use") return usePicked();
  const w = L.warn;
  if (!w) return;
  const mixed = new Set(w.done.items.filter((it) => it.mixed).map((it) => it.look));
  applyMany(act === "whole" ? w.ids.filter((id) => !mixed.has(id)) : w.ids, w.mod, act === "all" ? w.done : null);
}

// usePicked: use the selected Mod for the selected looks. The dry run says first how many
// would be mixed; the player then chooses.
async function usePicked() {
  const m = pickedMod(), ids = picked().map((r) => r.id);
  if (!m || !ids.length) return;
  let done;
  try { done = await api.LooksApplyPreview(ids, m.ref, [], L.scope); } catch (e) { return say("fail", errText(e)); }
  if (done.mixed > 0) { L.warn = { done, ids, mod: m }; return drawAct(); }
  applyMany(ids, m, done);
}

function applyMany(ids, m, done) {
  if (!ids.length) return;
  const scope = L.scope;
  gate({
    preview: () => (done ? Promise.resolve(done) : api.LooksApplyPreview(ids, m.ref, [], scope)),
    facts: (d) => [[T("looks"), num(d.done)], [T("from"), m.name], [T("for"), scopeWord(scope)]],
    title: ids.length === 1 ? T("apply this look in game?") : Tf("apply %s looks in game?", num(ids.length)),
    run: async () => {
      let d;
      try { d = await api.LooksApplyMany(ids, m.ref, scope); } catch (e) { return say("fail", errText(e)); }
      clearPicked();
      wrote(d);
    },
  });
}

async function undoPicked() {
  const ids = picked().filter((r) => r.choice).map((r) => r.id);
  if (!ids.length) return;
  let d;
  try { d = ids.length === 1 ? await api.LooksUndo(ids[0], L.scope) : await api.LooksUndoMany(ids, L.scope); } catch (e) { return say("fail", errText(e)); }
  clearPicked();
  wrote(d);
}

// what the confirmations' line builders (looksconfirm.js) word with
const WORDS = { T, Tf, num };
const fail = (e) => say("fail", errText(e));

// gate: an apply, behind the first apply's confirmation until the player turns it off.
function gate({ preview, facts, title, run }) {
  return applyGate({
    confirmed: () => !!(L.ov && L.ov.confirmed),
    preview,
    ask: (done, go) => confirmWrite(title, applyLines(facts(done), done, WORDS), T("apply"), go),
    remember: async () => { await api.SetSetting(CONFIRMED_KEY, "1"); if (L.ov) L.ov.confirmed = true; },
    fail,
  })(run);
}

// styleGate: an add of a style or random variant, behind the first add's confirmation; its
// "do not show this again" is a setting of its own (21 §6).
function styleGate({ preview, facts, kind, title, run }) {
  return applyGate({
    confirmed: () => !!(L.ov && L.ov.styles && L.ov.styles.confirmed),
    preview,
    ask: (done, go) => confirmWrite(title, addLines(facts(done), done, kind, WORDS), T("add"), go),
    remember: async () => { await api.SetSetting(STYLES_CONFIRMED_KEY, "1"); if (L.ov && L.ov.styles) L.ov.styles.confirmed = true; },
    fail,
  })(run);
}

// confirmWrite: what the apply or add writes (lines), that the feature is in test, and "do
// not show this again" (21 §6). A modal confirmation (popups.js panel); nothing is written
// before its yes.
function confirmWrite(title, lines, goLabel, go) {
  const body = el("div", "lk-ask");
  body.append(pop.confirmBody(lines));
  const test = el("div", "lk-test");
  const report = el("button", "btn small", T("feedback"));
  report.type = "button";
  report.onclick = () => { pop.close(); L.ui.go("feedback"); };
  test.append(el("b", "", T("This feature is still in test. If something goes wrong, please report it.")), report);
  const again = el("label", "check-row");
  const box = el("input");
  box.type = "checkbox";
  again.append(box, document.createTextNode(T("do not show this again")));
  body.append(test, again);
  pop.panel(title, body, goLabel, () => go(box.checked), "", "looks");
}

// wrote: an apply, undo or rebuild answered. Its sentence goes to the status line; a rebuild
// that did not run says why and stays on the RimArcho Looks Preference card (the choice is kept).
function wrote(d) {
  const failed = (d.items || []).filter((it) => it.error);
  L.buildError = d.buildError || "";
  say(d.buildError ? "fail" : "ok", [d.text, d.buildError, failed.length === 1 ? failed[0].title + ": " + failed[0].error : ""].filter(Boolean).join("  "));
  if (d.overview) setOverview(d.overview);
  drawSide();
  if (L.layer === 1) loadGallery(true);
  // the list may have gained its RimArcho Looks Preference entry: the editor's rows follow
  if (d.added) L.ui.reload();
}

// wroteStyle: an add or a remove of a style or random variant answered (view.LookStyleDone).
// The entry is kept even when the rebuild did not run; why stays on the Preference card.
function wroteStyle(d) {
  L.stylesError = d.buildError || "";
  say(d.buildError ? "fail" : "ok", [d.text, d.buildError || d.note].filter(Boolean).join("  "));
  if (d.overview) setOverview(d.overview);
  drawSide();
  // the list may have gained its RimArcho Styles Preference entry: the editor's rows follow
  if (d.added) L.ui.reload();
}

// ---- the two layers ----

function openLook(row) {
  if (!row) return;
  const sc = scroller();
  L.top = sc.scrollTop;
  L.layer = 2;
  $("#looks .lk-l1").hidden = true;
  $("#looks .lk-l2").hidden = false;
  const m = pickedMod();
  // the Mod selected in the gallery; else the one whose texture the card shows
  editor.open(row.id, m ? m.pos : (row.thumbs && row.thumbs[0] ? row.thumbs[0].p : -1));
  L.ui.renderWhere();
  L.ui.renderHints();
}

// closeLook goes back to the gallery: the same Mod, the same place, the cursor on the look.
function closeLook(focus = true) {
  if (L.layer !== 2) return;
  const changed = editor.hide();
  L.layer = 1;
  $("#looks .lk-l2").hidden = true;
  $("#looks .lk-l1").hidden = false;
  const sc = scroller();
  if (!L.layout || L.layout.inner !== Math.max(0, sc.clientWidth - GRID.padX * 2)) relayout(false);
  sc.scrollTop = L.top;
  drawRows();
  if (changed) loadGallery(true);
  if (focus) focusCard(1);
  L.ui.renderWhere();
  L.ui.renderHints();
}

// ---- focus and keys (16 §3.5): 1–4 focus the numbered cards; arrows only move ----

function markFocus(n) {
  L.focus = n;
  const cards = [$("#looks .lk-gal"), $("#looks .lk-mods"), $("#looks .lk-side .rulesarea"), $("#looks .lk-pref")];
  cards.forEach((c, i) => c && c.classList.toggle("focus", i + 1 === n));
}

function focusCard(n) {
  markFocus(n);
  if (n === 1) { scroller().focus({ preventScroll: true }); if (L.cursor < 0 && L.rows.length) { setCursor(0); paintPicked(); } }
  else if (n === 2) $("#looks .lk-mods .rows").focus({ preventScroll: true });
  else if (n === 3) showRules();
  else if (n === 4) $("#looks .lk-prow")?.focus({ preventScroll: true });
  L.ui.renderWhere();
  L.ui.renderHints();
}

// dropWords takes the search words out of the bar: a Mod was picked, its looks show
function dropWords() {
  if (!L.search) return;
  L.search = "";
  L.bar.setText(queryParts(L.bar.search).filter((p) => p.key || p.negative).map((p) => p.raw).join(" "));
}
function openFilter() { L.bar.toggle(); }
function openSearch() { L.bar.focus(); }

// key handles a key of the looks view; false leaves it to the editor's list-level keys
// (save, launch, the pages). The arrows only move; enter opens; space selects.
export function key(e) {
  if (!L.on) return false;
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  if (L.layer === 2) return editor.key(e);
  const take = () => { e.preventDefault(); return true; };
  if (!ctrl && !e.altKey && "1234".includes(k) && k.length === 1) { focusCard(+k); return take(); }
  if (!ctrl && !e.altKey && k === "/") { openSearch(); return take(); }
  if (ctrl && k.toLowerCase() === "f") { openSearch(); return take(); }
  if (!ctrl && !e.altKey && k === "\\") { openFilter(); return take(); }
  if (k === "Escape") {
    if (L.warn) { L.warn = null; drawAct(); }
    else if (L.sel.size) { clearPicked(); paintPicked(); }
    else if (L.filter || L.search) { L.bar.clear(); }
    return take();
  }
  if (L.focus === 2) {
    const d = { ArrowDown: 1, ArrowUp: -1, PageDown: 10, PageUp: -10, Home: -1e9, End: 1e9 }[k];
    if (d) { showMod(Math.max(0, Math.min(mods().length - 1, L.modCursor + d)), true); return take(); }
    if (k === "Enter") { showMod(L.modCursor); focusCard(1); return take(); }
    return k.startsWith("Arrow");
  }
  if (L.focus !== 1) return k.startsWith("Arrow") || k === "Home" || k === "End" || k.startsWith("Page");
  if (ctrl && k.toLowerCase() === "a") { L.rows.forEach((_, i) => toggle(i, true)); paintPicked(); return take(); }
  if (!L.rows.length || !L.layout) return k.startsWith("Arrow") || k === "Enter" || k === " ";
  const at = L.cursor;
  let n = null;
  if (k === "ArrowRight") n = at + 1;
  else if (k === "ArrowLeft") n = at - 1;
  else if (k === "ArrowDown") n = at < 0 ? 0 : stepCard(L.layout, at, 1);
  else if (k === "ArrowUp") n = at < 0 ? 0 : stepCard(L.layout, at, -1);
  else if (k === "PageDown" || k === "PageUp") {
    const rows = Math.max(1, Math.floor(scroller().clientHeight / (GRID.card + GRID.gap)) - 1);
    n = at < 0 ? 0 : at;
    for (let i = 0; i < rows; i++) n = stepCard(L.layout, n, k === "PageDown" ? 1 : -1);
  } else if (k === "Home") n = 0;
  else if (k === "End") n = L.rows.length - 1;
  else if (k === "Enter" && !ctrl) { if (at >= 0) openLook(L.rows[at]); return take(); }
  else if (k === " " || k === "Spacebar") { if (at >= 0) { toggle(at); L.anchor = at; paintPicked(); } return take(); }
  if (n == null) return false;
  setCursor(n);
  if (e.shiftKey && at >= 0) {
    const from = L.anchor >= 0 ? L.anchor : at;
    for (let i = Math.min(from, L.cursor); i <= Math.max(from, L.cursor); i++) toggle(i, true);
    L.anchor = from;
  }
  paintPicked();
  return take();
}

// hintList: the hint line's actions (16 §2.2): at most 7, ending with ? all keys.
export function hintList() {
  if (L.layer === 2) return editor.hintList();
  if (L.sel.size) return [
    { key: "space", label: T("select") }, { key: "shift+←↑↓→", label: T("select a range") }, { key: "ctrl+a", label: T("select everything shown") },
    { key: "enter", label: T("open the look") }, { key: "esc", label: T("clear the selection") }, { key: "?", label: T("all keys") },
  ];
  return [
    { key: "←↑↓→", label: T("move") }, { key: "enter", label: T("open the look") }, { key: "space", label: T("select") },
    { key: "1–4", label: T("cards") }, { key: "/", label: T("search") }, { key: "\\", label: T("filter") }, { key: "?", label: T("all keys") },
  ];
}

// where: the position line (16 §3.2).
export function where() {
  if (L.layer === 2) return editor.where();
  const m = pickedMod();
  let s = `${T("looks")}    ${L.search ? "“" + L.search + "”" : m ? m.name : genName()}    ${L.rows.length ? Math.max(0, L.cursor) + 1 : 0} / ${L.rows.length}`;
  if (L.sel.size) s += "  " + Tf("%d selected", L.sel.size);
  return s;
}

// help: ? in the looks view: its keys (the editor's ? lists the lists' verbs).
export function help() {
  const rows = L.layer === 2 ? editor.helpRows() : [
    ["1–4", T("focus a card: the gallery, Mods, load-order rules, preference")],
    ["←↑↓→  PgUp PgDn  Home End", T("move the cursor (nothing changes)")],
    ["enter", T("open the look")],
    ["space / ctrl+click", T("select or unselect one")],
    ["shift+←↑↓→ / shift+click", T("select a range")],
    ["ctrl+a", T("select everything shown")],
    ["/  ctrl+f", T("search")],
    ["\\", T("filter")],
    ["esc", T("back one level: the selection, the search and filter")],
    ["r", T("add a load-order rule for the Mod in the Mods card")],
    ["w  L  u", T("save, launch, undo the list's last edit")],
  ];
  pop.open("help", T("all keys"), (box) => {
    box.append(el("div", "ttl", T("the arrows only move; apply and undo are buttons, and undo takes a choice back")));
    const body = el("div", "body help");
    for (const [keys, what] of rows) body.append(el("b", "word-facts", keys), el("span", "word-facts", what));
    box.append(body);
  });
}
