// Saves › Play review (16 §2.11, 12 §3): what the installed Mods add and how much of it
// the saves show, all saves of a colony together. The page fills the window with four
// numbered cards whose borders drag (16 §3.1 rule 10): 1 the colonies (a find box, then
// one card each), 2 a map of the colony's Mods (one square a Mod, in two groups), 3 the
// details of the Mod picked, drawn as Mod library draws them (detail.js detailFrame), and
// 4 what the saves show of it: each category folded, the names not seen inside. The review runs in the background and arrives as "review"
// events: progress lines, then the page (16 §8.4).
import { S, $, el, api, T, Tf, say } from "./state.js";
import { on } from "./api.js";
import { beginPageRead, page, area, button } from "./screens.js";
import { icon } from "./icons.js";
import { detailFrame } from "./detail.js";
import { wireSeparator } from "./separator.js";
import { choiceControl } from "./choices.js";
import { openLibraryInstance } from "./librarylink.js";
import { tagColors, tagCSS } from "./tagcolor.js";

// page: the review shown; colony: the colony asked for ("" all); q: the find box; by: how
// the map is grouped (kind, tags, main); sel: the Mod picked (its package ID); names: the
// categories (Mod|label) whose names show in full.
const kept = (k, d) => { try { return localStorage.getItem("review." + k) || d; } catch { return d; } };
const keep = (k, v) => { try { localStorage.setItem("review." + k, v); } catch { /* storage can be off */ } };
const X = { page: null, colony: "", q: "", by: kept("by", "progress"), sort: kept("sort", "amount"), view: kept("view", "covers"), sel: "", names: new Set(), cats: new Set(), seq: 0, detail: 0 };
let find = null, cols = null, map = null, ctl = null, dinfo = null, dbody = null, pinfo = null, pwrap = null, shown = "";
const waiting = new Map(); // request → its resolve
const NAMES = 40; // names not seen shown in a category before "N more"
const lc = (s) => String(s || "").toLowerCase();

on("review", (ev) => {
  if (!ev) return;
  if (ev.line) { if (S.screen === "review" && ev.request === X.seq) say("busy", ev.line); return; }
  const w = waiting.get(ev.request);
  if (!w) return;
  if (ev.partial && ev.page) { w.partial(ev.page); return; }
  waiting.delete(ev.request);
  w.done(ev);
});

// read asks for the review and waits for its last event; a newer request makes it stale.
// The page comes in parts (16 §3.1 rule 9): at once the colonies, then the Mods of the
// saves reviewed so far, newest first; partial is called with each.
function read(partial) {
  const n = X.seq = Math.max(X.seq + 1, Date.now() * 1000);
  return new Promise((resolve, reject) => {
    waiting.set(n, {
      partial: (p) => { if (n === X.seq) partial(p); },
      done: (ev) => {
        if (n !== X.seq) return resolve(null);
        if (ev.error) return reject(new Error(ev.error));
        resolve(ev.page);
      },
    });
    api.SaveReviewRead(n, X.colony).then((ok) => { if (!ok) { waiting.delete(n); reject(new Error(T("the window is closing"))); } },
      (e) => { waiting.delete(n); reject(e); });
  });
}

// progress says how far a running review is; nothing once it is whole.
function progress(p, partial) {
  if (partial && p.total > 0) say("busy", Tf("reviewing the saves %d/%d", p.done || 0, p.total));
  else if ($("#status")?.dataset.kind === "busy") say("", "");
}

export async function open() {
  const r = beginPageRead("review", { retry: open, label: "reviewing the saves…" });
  let built = false;
  const show = (p, partial) => {
    if (!r.current()) return;
    X.page = p;
    X.colony = p.colony || "";
    if (!built) { built = true; build(); r.finish(); } else { drawColonies(); drawMap(); drawRight(); }
    progress(p, partial);
  };
  let p = null;
  while (!p) {
    try { p = await read((part) => show(part, true)); } catch (e) { if (built) say("fail", String(e?.message || e)); else r.fail(e); return; }
    if (!r.current()) return;
  }
  show(p, false);
}

// pick reviews another colony behind the page shown.
async function pick(key) {
  if (key === X.colony) return;
  X.colony = key;
  X.names.clear(); X.cats.clear();
  drawColonies();
  say("busy", T("reviewing the saves…"));
  const show = (p, partial) => {
    if (S.screen !== "review") return;
    X.page = p;
    X.colony = p.colony || "";
    drawColonies(); drawMap(); drawRight();
    progress(p, partial);
  };
  let p;
  try { p = await read((part) => show(part, true)); } catch (e) { say("fail", String(e?.message || e)); return; }
  if (p) show(p, false);
}

const mods = () => (X.page?.groups || []).flatMap((g) => g.mods.map((m) => ({ m, mode: g.key })));
const picked = () => mods().find((x) => x.m.packageId === X.sel) || mods()[0] || null;

function build() {
  const p = X.page;
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("fill review", p.lead);
  const grid = el("div", "sv rv");
  const leftCol = el("div", "sv-leftcol");
  const search = el("label", "card sv-find");
  find = el("input", "sv-q");
  find.type = "search";
  find.placeholder = T("find a colony");
  find.setAttribute("aria-label", find.placeholder);
  find.dataset.key = "review-find";
  find.value = X.q;
  find.autocomplete = "off"; find.spellcheck = false;
  find.oninput = () => { X.q = find.value; drawColonies(); };
  find.onkeydown = (e) => {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); if (X.q) { X.q = find.value = ""; drawColonies(); } else cols.focus({ preventScroll: true }); }
    else if (e.key === "ArrowDown" || e.key === "Enter") { e.preventDefault(); e.stopPropagation(); cols.focus({ preventScroll: true }); }
    else e.stopPropagation(); // typing is not a page key
  };
  search.append(find, el("kbd", "", "/"));
  const leftCard = el("section", "card sv-left");
  cols = el("div", "sv-tree rv-cols");
  cols.tabIndex = 0;
  cols.setAttribute("role", "listbox");
  cols.setAttribute("aria-label", T("colonies"));
  cols.onkeydown = (e) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    const keys = [...cols.querySelectorAll("[data-colony]")].map((b) => b.dataset.colony);
    const i = keys.indexOf(X.colony), j = Math.max(0, Math.min(keys.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)));
    e.preventDefault();
    if (keys[j] !== undefined) pick(keys[j]);
  };
  const colHead = el("div", "sv-cardhead");
  colHead.append(el("span", "", T("colonies")));
  leftCard.append(colHead, cols);
  leftCol.append(search, leftCard);
  const mid = el("section", "card sv-mid rv-mid");
  const midHead = el("div", "sv-cardhead rv-bar");
  // how the Mods are grouped, how each group is sorted, and pictures or a list
  ctl = el("div", "rv-ctl");
  const by = choiceControl(T("group by"), [["progress", T("by progress")], ["kind", T("by kind of content")], ["tags", T("by your tags")], ["main", T("by largest category")]].map(([value, label]) => ({ value, label })), X.by);
  by.dataset.key = "review-by";
  by.onchange = () => { X.by = by.value; keep("by", X.by); drawMap(); };
  const sort = choiceControl(T("sort"), [["progress", T("least seen first")], ["amount", T("most content first")], ["name", T("A-Z")]].map(([value, label]) => ({ value, label })), X.sort);
  sort.dataset.key = "review-sort";
  sort.onchange = () => { X.sort = sort.value; keep("sort", X.sort); drawMap(); };
  const view = el("div", "rv-seg");
  view.setAttribute("role", "group");
  view.setAttribute("aria-label", T("show the Mods as"));
  for (const [k, label] of [["covers", T("pictures")], ["list", T("list")]]) {
    const b = el("button", "", label);
    b.type = "button";
    b.dataset.view = k;
    b.dataset.key = "review-view-" + k;
    b.onclick = () => { X.view = k; keep("view", k); drawMap(); b.focus({ preventScroll: true }); };
    view.append(b);
  }
  ctl.append(by, sort, view);
  midHead.append(el("span", "", T("Mods")), ctl);
  map = el("div", "sv-body rv-map");
  map.tabIndex = 0;
  map.setAttribute("aria-label", T("the Mods, one tile each with its picture and name; the bar is how much of it the saves show"));
  map.onkeydown = mapKey;
  mid.append(midHead);
  if (p.note) mid.append(el("p", "sv-note warn", p.note));
  mid.append(map);
  // right: the Mod's details as Mod library draws them, above what the saves show of it
  const rightCol = el("div", "rv-right");
  const dcard = el("section", "card rv-detail");
  const dhead = el("div", "sv-cardhead");
  dinfo = el("span", "rv-info");
  dhead.append(el("span", "", T("details")), dinfo);
  dbody = el("div", "sv-body scroll");
  dbody.tabIndex = 0;
  dcard.append(dhead, dbody);
  const vgap = el("div", "hsplit rv-vsplit");
  const prog = el("section", "card rv-prog");
  const phead = el("div", "sv-cardhead rv-phead");
  pinfo = el("span", "rv-info");
  phead.append(el("span", "", T("in the saves")), pinfo);
  pwrap = el("div", "rv-pwrap");
  prog.append(phead, pwrap);
  rightCol.append(dcard, vgap, prog);
  const gap1 = el("div", "split sv-split"), gap2 = el("div", "split sv-split");
  grid.append(leftCol, gap1, mid, gap2, rightCol);
  w.append(grid);
  sc.append(w);
  const setVar = (box, name) => (v) => { if (v) box.style.setProperty(name, v + "px"); else box.style.removeProperty(name); };
  const leftRange = () => [200, Math.max(200, Math.min(480, grid.clientWidth - 640))];
  wireSeparator(gap1, { key: "ui.gui.review.left", range: leftRange,
    get: () => leftCol.getBoundingClientRect().width, set: setVar(grid, "--sv-left"), reset: () => setVar(grid, "--sv-left")(0),
    restore: (v) => setVar(grid, "--sv-left")(+v ? Math.max(leftRange()[0], Math.min(leftRange()[1], +v)) : 0) });
  const rightRange = () => [280, Math.max(280, grid.clientWidth - leftCol.getBoundingClientRect().width - 380)];
  wireSeparator(gap2, { key: "ui.gui.review.right", direction: -1, range: rightRange,
    get: () => rightCol.getBoundingClientRect().width, set: setVar(grid, "--sv-right"), reset: () => setVar(grid, "--sv-right")(0),
    restore: (v) => setVar(grid, "--sv-right")(+v ? Math.max(rightRange()[0], Math.min(rightRange()[1], +v)) : 0) });
  const topRange = () => [120, Math.max(120, rightCol.clientHeight - 160)];
  wireSeparator(vgap, { axis: "y", key: "ui.gui.review.detail", range: topRange,
    get: () => dcard.getBoundingClientRect().height, set: setVar(rightCol, "--rv-top"), reset: () => setVar(rightCol, "--rv-top")(0),
    restore: (v) => setVar(rightCol, "--rv-top")(+v ? Math.max(topRange()[0], Math.min(topRange()[1], +v)) : 0) });
  area(leftCard, colHead, () => cols.focus({ preventScroll: true }));
  area(mid, midHead, () => map.focus({ preventScroll: true }));
  area(dcard, dhead, () => dbody.focus({ preventScroll: true }));
  area(prog, phead, () => pwrap.querySelector(".sv-body")?.focus({ preventScroll: true }));
  shown = "";
  drawColonies(); drawMap(); drawRight();
}

function drawColonies() {
  if (!cols || !X.page) return;
  const q = lc(X.q.trim());
  cols.replaceChildren();
  const card = (key, name, line) => {
    const b = el("button", "rv-col" + (key === X.colony ? " on" : ""));
    b.type = "button";
    b.tabIndex = -1;
    b.dataset.colony = key;
    b.dataset.key = "review-colony-" + (key === "*" ? "all" : key);
    b.setAttribute("role", "option");
    b.setAttribute("aria-selected", String(key === X.colony));
    b.append(el("b", "", name), el("span", "", line));
    b.onclick = () => pick(key);
    cols.append(b);
  };
  if (!q) card("*", T("all colonies"), X.page.all);
  for (const c of X.page.colonies || []) if (!q || lc(c.name).includes(q)) card(c.key, c.name, c.line);
  if (!cols.childElementCount) cols.append(el("p", "sv-none", T("no colony matches")));
}

// A Mod's tile: its picture and name, so it is known without picking it, in a grid of one
// size, the Mods that add most first. The bar under the picture is how much the saves
// show, in steps that can be told apart: nothing (amber), some, most, all (green).
const initials = (name) => (String(name).match(/[\p{L}\p{N}]+/gu) || []).slice(0, 2).map((w) => [...w][0].toUpperCase()).join("");
function shade(m, mode) {
  if (mode !== "meet") return m.part === 0 ? "unused" : "used";
  if (m.part === 0) return "none";
  return m.part >= m.size ? "whole" : m.part * 2 >= m.size ? "most" : "some";
}

// groupsBy: the map's groups under the grouping chosen. A Mod keeps the colouring of its
// own group whatever it is grouped by.
function groupsBy() {
  const gs = X.page.groups || [];
  let list;
  if (X.by === "kind") list = gs.map((g) => ({ label: g.label, hint: g.hint, items: g.mods.map((m) => ({ m, mode: g.key })) }));
  else if (X.by === "progress") {
    // what the player asks first: what was never touched, then how far the rest went
    const order = [["none", T("nothing in the saves")], ["some", T("a little seen")], ["most", T("mostly seen")], ["whole", T("all seen")], ["used", T("picked from, some in use")]];
    const by = new Map(order.map(([k]) => [k, []]));
    for (const x of mods()) by.get(tier(x.m, x.mode)).push(x);
    list = order.map(([k, label]) => ({ label, hint: k === "used" ? T("floors, furniture, clothes, looks: how many are used says little") : "", items: by.get(k) }));
  } else {
    const out = new Map();
    const add = (k, x) => { if (!out.has(k)) out.set(k, []); out.get(k).push(x); };
    for (const x of mods()) {
      if (X.by === "main") add(x.m.main, x);
      else if ((x.m.tags || []).length) for (const t of x.m.tags) add(t, x);
      else add("", x);
    }
    list = [...out].map(([k, items]) => ({ label: k || T("no tag"), tag: X.by === "tags" ? k : "", last: !k, hint: "", items }));
    list.sort((a, b) => a.last - b.last || b.items.length - a.items.length);
  }
  const share = (x) => x.m.size ? x.m.part / x.m.size : 0;
  const cmp = {
    progress: (a, b) => share(a) - share(b) || b.m.adds - a.m.adds,
    amount: (a, b) => b.m.adds - a.m.adds,
    name: (a, b) => a.m.name.localeCompare(b.m.name, undefined, { sensitivity: "base", numeric: true }),
  }[X.sort] || (() => 0);
  for (const g of list) g.items.sort(cmp);
  return list.filter((g) => g.items.length);
}

// tier is how far the saves went with a Mod: for what the game brings you to, nothing,
// some (under half), most, all; for what is picked from, nothing or used.
function tier(m, mode) {
  if (m.part === 0) return "none";
  if (mode !== "meet") return "used";
  return m.part >= m.size ? "whole" : m.part * 2 >= m.size ? "most" : "some";
}

function drawMap() {
  if (!map || !X.page) return;
  for (const b of ctl.querySelectorAll("[data-view]")) { b.classList.toggle("on", b.dataset.view === X.view); b.setAttribute("aria-pressed", String(b.dataset.view === X.view)); }
  const top = map.scrollTop;
  map.replaceChildren();
  const groups = groupsBy();
  if (!groups.length) map.append(el("p", "empty-state", T("no save here has been read yet, or none has an installed Mod with content")));
  const sel = picked();
  for (const g of groups) {
    const sec = el("section", "rv-grp");
    const h = el("h3");
    const name = el("span", "l", g.label);
    if (g.tag) { const c = tagCSS(tagColors()[lc(g.tag)]); if (c) { name.classList.add("tagged"); name.style.setProperty("--c", c); } }
    h.append(name, el("span", "k", [Tf("%d Mods", g.items.length), g.hint].filter(Boolean).join(T(", "))));
    const tiles = el("div", "rv-tiles " + X.view);
    for (const { m, mode } of g.items) {
      const st = shade(m, mode);
      const b = el("button", "rv-tile " + st + (sel && sel.m === m ? " on" : ""));
      b.type = "button";
      b.tabIndex = -1;
      b.dataset.mod = m.packageId;
      b.dataset.key = "review-mod-" + m.packageId;
      b.title = m.name + "\n" + m.tip;
      b.setAttribute("aria-label", m.name + T(": ") + m.tip);
      const pic = el("span", "pic");
      pic.append(el("span", "ini", initials(m.name)));
      if (m.library) {
        const img = el("img");
        img.alt = "";
        img.loading = "lazy";
        img.decoding = "async";
        img.onerror = () => img.remove();
        img.src = "/preview/" + encodeURIComponent(m.library) + "?w=thumb";
        pic.append(img);
      }
      const bar = el("span", "bar");
      const fill = el("i");
      fill.style.width = (m.part ? Math.max(6, Math.round(100 * m.part / m.size)) : 0) + "%";
      bar.append(fill);
      b.append(pic, bar, el("span", "t", m.name));
      if (X.view === "list") b.append(el("span", "n", m.size ? Tf("%d / %d", m.part, m.size) : ""));
      b.onclick = () => { choose(m.packageId); map.focus({ preventScroll: true }); };
      tiles.append(b);
    }
    sec.append(h, tiles);
    map.append(sec);
  }
  if (X.page.foot) map.append(el("p", "rv-foot", X.page.foot));
  map.scrollTop = top;
}

// choose picks a Mod's square: the mark moves, and the right cards show it.
function choose(id) {
  X.sel = id;
  for (const b of map.querySelectorAll(".rv-tile")) b.classList.toggle("on", b.dataset.mod === id);
  map.querySelector(".rv-tile.on")?.scrollIntoView({ block: "nearest" });
  drawRight();
}

// mapKey: the arrows move among the squares (they only navigate, 16 §3.1 rule 1): ←→ the
// one before or after, ↑↓ the nearest in the row above or below; Home and End the ends.
function mapKey(e) {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const tiles = [...map.querySelectorAll(".rv-tile")];
  if (!tiles.length) return;
  let i = tiles.findIndex((b) => b.classList.contains("on")), j = -1;
  if (i < 0) i = 0;
  if (e.key === "ArrowRight") j = Math.min(tiles.length - 1, i + 1);
  else if (e.key === "ArrowLeft") j = Math.max(0, i - 1);
  else if (e.key === "Home") j = 0;
  else if (e.key === "End") j = tiles.length - 1;
  else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    const down = e.key === "ArrowDown", r = tiles[i].getBoundingClientRect(), x = r.left + r.width / 2;
    let best = Infinity, row = null;
    for (let k = 0; k < tiles.length; k++) {
      const q = tiles[k].getBoundingClientRect();
      if (down ? q.top < r.bottom - 1 : q.bottom > r.top + 1) continue;
      const edge = down ? q.top : -q.bottom;
      if (row === null || edge < row - 1) { row = edge; best = Infinity; j = -1; }
      if (edge > row + q.height) continue;
      const d = Math.abs(q.left + q.width / 2 - x);
      if (d < best) { best = d; j = k; }
    }
  } else return;
  e.preventDefault(); e.stopPropagation();
  if (j >= 0 && j !== i || !X.sel) choose(tiles[Math.max(0, j)].dataset.mod);
}

// drawRight: the Mod picked. Card 3 is its details as Mod library draws them (the picture,
// name, authors, facts, fields, description, history); card 4 what the saves show of it.
function drawRight() {
  if (!dbody) return;
  const x = picked();
  dinfo.replaceChildren();
  pinfo.replaceChildren();
  pwrap.replaceChildren();
  if (!x) { dbody.replaceChildren(); shown = ""; return; }
  const m = x.m;
  dinfo.append(el("span", "q word-facts", m.name));
  if (m.library) {
    const go = el("button", "editor-full-link", T("show in the mod library"));
    go.type = "button";
    go.dataset.key = "review-lib-" + m.packageId;
    go.onclick = () => openLibraryInstance(m.library);
    dinfo.append(go);
  }
  if (shown !== m.packageId) {
    shown = m.packageId;
    const n = ++X.detail;
    const inner = el("div", "mod-detail");
    // the tags and the folder are edited and opened in Mod library: their links go to the copy there
    const lib = () => openLibraryInstance(m.library);
    const frame = detailFrame(inner, dbody, { later: true, mount: () => dbody.replaceChildren(inner), fieldActs: () => ({ tags: lib, folder: lib }) });
    frame.head(m.name, "", m.library || "");
    const detail = !m.library ? null : api.Detail(m.library, false, false, false).then((d) => { if (n === X.detail) frame.detail(d); }, () => {});
    frame.settle([detail]).then(() => { if (n === X.detail && dbody.isConnected) frame.show(); });
  }

  pinfo.append(meter(m.seen, m.adds, "big"), el("b", "", m.sum), el("span", "muted", m.where));
  // why the numbers are not exact, in a line; the rest on request
  const lim = el("details", "sv-limits rv-why");
  lim.append(el("summary", "", T("not exact: a save shows only what is in it now")));
  lim.append(el("p", "", T("what was used up, sold or lost before the oldest save leaves no trace; bills, filters and research not begun do not count; recipes are not counted; a Mod updated since may have renamed things")));
  const body = el("div", "sv-body rv-cats");
  body.tabIndex = 0;
  body.setAttribute("aria-label", T("what it adds, by category, with the names not seen"));
  for (const c of m.cats || []) {
    const key = m.packageId + "|" + c.label, open = X.cats.has(key);
    const names = c.names || [];
    const sec = el("section", "rv-cat");
    const h = el("button", "rv-cath");
    h.type = "button";
    h.dataset.key = "review-cat-" + m.packageId;
    h.dataset.cat = c.label;
    h.setAttribute("aria-expanded", String(open));
    const chv = el("span", "chv" + (open ? " open" : "")); chv.innerHTML = icon.chev;
    h.append(chv, el("span", "l", c.label), meter(c.seen, c.adds), el("span", "k", Tf("%d / %d", c.seen, c.adds) + (c.under ? " " + T("(saves undercount these)") : "")));
    h.onclick = () => {
      if (open) X.cats.delete(key); else X.cats.add(key);
      const top = body.scrollTop;
      drawRight();
      const nb = pwrap.querySelector(".rv-cats");
      if (nb) nb.scrollTop = top;
      [...pwrap.querySelectorAll(".rv-cath")].find((b) => b.dataset.cat === c.label)?.focus({ preventScroll: true });
    };
    sec.append(h);
    if (open) {
      if (!names.length) sec.append(el("p", "rv-names muted", T("all seen")));
      else {
        // a long category shows its first names; the rest on request
        const whole = X.names.has(key) || names.length <= NAMES;
        const p = el("p", "rv-names");
        for (const n of whole ? names : names.slice(0, NAMES)) p.append(el("span", "", n));
        if (!whole) {
          const all = button(Tf("%d more", names.length - NAMES + (c.more || 0)), "", () => { X.names.add(key); drawRight(); }, "link sv-fold");
          all.dataset.key = "review-names-" + m.packageId;
          p.append(all);
        } else if (c.more) p.append(el("span", "more", Tf("and %d more", c.more)));
        sec.append(p);
      }
    }
    body.append(sec);
  }
  pwrap.append(lim, body);
}

function meter(seen, adds, cls = "") {
  const mt = el("span", "rv-meter " + cls + (seen === 0 ? " none" : seen >= adds ? " whole" : ""));
  mt.setAttribute("aria-hidden", "true");
  const i = el("i");
  i.style.width = (seen ? Math.max(4, Math.round(100 * seen / adds)) : 0) + "%";
  mt.append(i);
  return mt;
}

// key: the page's keys (16 §2.11). true when taken. In the map the arrows move among the
// squares (mapKey); in the two right cards they scroll; elsewhere ↑↓ pick a colony.
export function key(e) {
  if (e.ctrlKey || e.metaKey || e.altKey) return false;
  if (e.key === "/" && document.activeElement !== find && find) {
    e.preventDefault();
    find.focus({ preventScroll: true });
    find.select();
    return true;
  }
  const at = document.activeElement;
  if (at?.closest?.(".rv-mid, .rv-right")) return false;
  if ((e.key === "ArrowDown" || e.key === "ArrowUp") && cols && at !== cols && at !== find) {
    cols.focus({ preventScroll: true });
    cols.dispatchEvent(new KeyboardEvent("keydown", { key: e.key }));
    e.preventDefault();
    return true;
  }
  return false;
}

export function hintList() {
  return [{ key: "/", label: T("find a colony") }, { key: "↑↓", label: T("colony") }, { key: "←→↑↓", label: T("Mod, in the map") }, { key: "enter", label: T("fold a category") }, { key: "1–4", label: T("the cards") }, { key: "?", label: T("all keys") }];
}
