// The pages over the board (the sidebar's entries, 06 §2.1): 能不能跑 (06 §2.4, §5), the
// game log (§2.10), Mod 库 › 变化 (§2.9), the full detail (§4.2), packs, the environment
// and settings. The pages are side by side: esc does not go from one to another (06 §3.1);
// esc leaves only the full detail, back to the plan. A page's actions sit in the top bar.
import { S, $, el, api, T, Tf, errText, say, cur, slideThumb } from "./state.js";
import { icon } from "./icons.js";
import { statusIcon } from "./icons.js";
import { bump, shimmer } from "./motion.js";
import * as pop from "./popups.js";
import { bgSection } from "./bg.js";
import { virtualRows } from "./virtualrows.js";

let ui = null;
export function initScreens(u) { ui = u; }

// fresh: the next page built is a new page (not a redraw of the one shown); only a new page
// fades in, so a redraw after an action or a sub-view switch does not flicker.
let fresh = false;

// left tells Go the player left a page, which forgets what only that page used (07 §8.3).
function left(next) {
  if (S.screen && S.screen !== next) {
    if (S.screen === "log") { L.list?.destroy(); L.list = null; L.want = ""; clearTimeout(detailTimer); }
    api.Leave(S.screen).catch(() => {});
  }
}

export function openScreen(name) {
  fresh = S.screen !== name;
  if (fresh) S.screenSeq++;
  if (fresh) onArea = 0;
  left(name);
  S.screen = name;
  if (name !== "home") dropBackdrop();
  const sc = $("#screen");
  sc.hidden = false;
  sc.innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return sc;
}

export function closeScreen() {
  if (!S.screen) return false;
  left(null);
  S.screenSeq++;
  S.screen = null;
  dropBackdrop();
  $("#screen").hidden = true;
  $("#screen").innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return true;
}

// dropBackdrop takes the launch page's image away with the page (home.js setBackdrop).
function dropBackdrop() {
  const d = $("#backdrop");
  if (d) d.remove();
  $("#app").classList.remove("photo");
}

// page builds a page: one muted sentence that says what the page is for (the breadcrumb
// names it, 06 §2.2), then the body. The buttons and fields among right are the page's
// actions: they sit at the right end of that line, quiet unless primary; anything else
// joins the lead. The top bar has no buttons.
export function page(cls, lead, ...right) {
  const w = el("div", "page " + cls + (fresh ? " enter" : ""));
  fresh = false;
  resetAreas();
  const head = el("div", "page-head");
  if (lead) head.append(el("p", "lead", lead));
  const acts = [];
  for (const r of right) {
    if (!r) continue;
    if (r.matches("button, input, select")) {
      if (r.matches("button.btn") && !r.classList.contains("primary")) r.classList.add("quiet");
      acts.push(r);
    } else head.append(r);
  }
  if (acts.length) {
    const box = el("div", "page-acts");
    for (const a of acts) box.append(a);
    head.append(box);
  }
  if (head.childNodes.length) w.append(head);
  return w;
}

// ---- numbered areas (06 §3.5): on every page 1–9 focus its areas, as 1–4 do in the editor;
// each area's heading shows its number ("1 会让游戏出错") ----
let areas = [];
let onArea = 0; // the area last picked on this page: a redraw keeps its mark
export function resetAreas() { areas = []; }

// area numbers box (its heading head gets the number) and says what focusing it does; the
// default scrolls it into view and focuses its first control, so tab goes on from there.
export function area(box, head, focus) {
  const n = areas.length + 1;
  if (n > 9 || !box) return;
  areas.push({ box, focus: focus || (() => { const f = box.querySelector("button, input, select, [tabindex]"); if (f) f.focus({ preventScroll: true }); }) });
  box.dataset.area = String(n);
  box.classList.toggle("area-on", n === onArea);
  if (!head) return;
  // a heading kept across visits keeps one number
  const had = head.querySelector(":scope > .anum");
  if (had) had.textContent = String(n);
  else head.prepend(el("span", "anum", String(n)));
}

// areaKey takes 1–9 on a page: the area is marked, shown and focused. true when taken.
export function areaKey(e) {
  if (e.ctrlKey || e.metaKey || e.altKey || !/^[1-9]$/.test(e.key)) return false;
  const a = areas[+e.key - 1];
  if (!a || !a.box.isConnected) return false;
  e.preventDefault();
  onArea = +e.key;
  for (const x of areas) x.box.classList.toggle("area-on", x === a);
  a.box.scrollIntoView({ block: "nearest" });
  a.focus();
  return true;
}

export function button(label, keyName, run, cls = "") {
  const b = el("button", "btn " + cls, label);
  if (keyName) b.append(el("kbd", "", keyName));
  b.onclick = (e) => { e.stopPropagation(); run(); };
  return b;
}

// act is a page action with an icon (the top bar's quiet buttons, 06 §2.2).
export function act(label, ic, run, keyName) {
  const b = button(label, keyName, run);
  const i = el("span", "ic");
  i.innerHTML = icon[ic] || "";
  b.prepend(i);
  if (keyName) b.title = keyName;
  return b;
}

// seg is a small view switch (magpie's): one button per choice, the chosen one raised.
export function seg(names, on, pick, keys) {
  const s = el("div", "seg sub");
  s.append(el("span", "thumb"));
  names.forEach((n, i) => {
    const b = el("button", i === on ? "on" : "", n);
    if (keys) b.title = keys[i];
    b.onclick = () => pick(i);
    s.append(b);
  });
  requestAnimationFrame(() => setSeg(s, on));
  return s;
}

// setSeg opens choice i of a switch already on the page: its thumb slides there.
export function setSeg(s, i) {
  if (!s) return;
  const bs = [...s.querySelectorAll(":scope > button")];
  bs.forEach((b, j) => b.classList.toggle("on", j === i));
  slideThumb(s, bs[i]);
}

// moveIn moves a cursor kept as an index in [0, n).
function moveIn(curIdx, n, k) {
  if (k === "ArrowDown" || k === "j") return Math.min(curIdx + 1, n - 1);
  if (k === "ArrowUp" || k === "k") return Math.max(curIdx - 1, 0);
  if (k === "PageDown") return Math.min(curIdx + 10, n - 1);
  if (k === "PageUp") return Math.max(curIdx - 10, 0);
  if (k === "Home") return 0;
  if (k === "End") return n - 1;
  return null;
}

// ---- 能不能跑 (06 §2.4, §5): one column of sections, by consequence ----
// 会让游戏出错, 上次运行的报错 (this run first while the game runs), 要你确认, 隐藏的 Mod.
// Each item is a card: a bold line, a plain sentence, where the claim comes from (the
// evidence chip), one primary action and the rest under ⋯. The arrows move a cursor
// through the items; an action's key runs it (06 §3).
const SECTIONS = ["breaks the game", "errors from the last run", "for you to confirm", "hidden mods"];
const SECTION_CLS = ["break", "run", "confirm", "hidden"];
const N = { data: null, items: [], cur: 0, open: new Set(), els: [] };

// archo is the hook for the Archo panel (06 §2.13, 1.1): when set, an item can be handed
// to it ("Archo 在看"). Nothing sets it yet.
export const archo = { ask: null };

export function liveChanged() {
  if (S.screen === "needs" && N.data) { orderNeeds(); drawNeeds(); }
}

// liveCount: this run's counted errors (the sidebar's badge adds them, 06 §2.1).
export function liveCount() {
  return ((S.live && S.live.items) || []).filter((it) => it.counted).length;
}

function orderNeeds() {
  const all = ((S.live && S.live.items) || []).concat(N.data.items || []);
  // stable: within a section the view's order (this run before the last run)
  N.items = all.map((it, i) => [it, i]).sort((a, b) => (a[0].section - b[0].section) || (a[1] - b[1])).map((x) => x[0]);
  N.cur = Math.min(N.cur, Math.max(N.items.length - 1, 0));
}

export async function needs(keepCursor) {
  let data;
  try { data = await api.Needs(); } catch (e) { say("fail", errText(e)); return; }
  N.data = data;
  if (!keepCursor) N.cur = 0;
  orderNeeds();
  drawNeeds();
}

// lead: what the page says about the plan in one sentence.
function needsLead(d) {
  const plan = (S.shell && S.shell.plan && S.shell.plan.name) || T("the list");
  const breaks = N.items.filter((x) => x.counted && x.section === 0).length;
  const count = N.items.filter((x) => x.counted).length;
  if (breaks) return Tf("%s is not safe to play yet: fix the %d problems below; every step can be undone.", plan, breaks);
  if (count) return Tf("%s can run; %d things below are worth a look.", plan, count);
  if (d.never) return Tf("%s: the mods' files are not checked yet.", plan);
  return Tf("%s can run: nothing to handle.", plan);
}

function drawNeeds() {
  const sc = S.screen === "needs" ? $("#screen") : openScreen("needs");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const d = N.data;
  let state = "";
  if (S.busy.analyze) state = T("checking the mods' files…");
  else if (d.never) state = T("the mods' files are not checked yet");
  else if (d.stale) state = T("the files changed since the last check");
  const w = page("needs", needsLead(d),
    state ? el("span", "state-note", state) : null,
    act(T("check the files again"), "rescan", () => ui.cmd("analyze"), "A"),
    act(T("game log"), "log", () => ui.go("log"), "l"));
  N.els = [];
  if (!N.items.length) {
    w.append(el("section", "card empty-state", "✓ " + T("nothing here")));
  }
  let i = 0;
  SECTIONS.forEach((title, si) => {
    const its = N.items.filter((x) => x.section === si);
    if (!its.length) return;
    const sec = el("section", "sec need-sec " + SECTION_CLS[si]);
    const n = its.filter((x) => x.counted).length;
    const h = el("h2", "label", T(title));
    const first = i;
    area(sec, h, () => { N.cur = first; markNeeds(); });
    if (n) {
      const c = el("span", "n num", String(n));
      h.append(el("span", "", " · "), c);
      bump("needs:" + si, c, n);
    }
    // which run the log is from: the list, or a pack (08 §5.3)
    if (si === 1 && S.live && S.live.running) h.append(el("span", "muted", " · " + (S.live.pack ? Tf("pack %s", S.live.pack) : T("this run"))));
    else if (si === 1 && d.lastRun) h.append(el("span", "muted", " · " + d.lastRun));
    sec.append(h);
    const card = el("div", "card need-list");
    for (const it of its) {
      const row = needCard(it, i);
      N.els[i] = row;
      card.append(row);
      i++;
    }
    sec.append(card);
    w.append(sec);
  });
  sc.append(w);
  sc.scrollTop = top;
  markNeeds();
}

function needCard(it, i) {
  const row = el("div", "need" + (it.dim ? " dimmed" : ""));
  const top = el("div", "top");
  const body = el("div", "body");
  if (it.dim) {
    body.append(el("div", "why", it.text));
    if (it.why) body.append(el("div", "why sub", it.why));
  } else {
    body.append(el("div", "title", it.title || it.text));
    if (it.why) body.append(el("div", "why", it.why));
  }
  top.append(body);
  const acts = it.actions || [];
  const right = el("div", "acts");
  if (archo.ask && it.section === 1) right.append(button(T("ask Archo"), "", () => archo.ask(it), "small"));
  if (acts.length) {
    const a = acts[0];
    right.append(button(a.label, "", () => { N.cur = i; markNeeds(); runAction(it, a); }, it.dim ? "" : "primary"));
    if (acts.length > 1) {
      const more = button("⋯", "", () => {
        N.cur = i; markNeeds();
        const r = more.getBoundingClientRect();
        pop.menu(r.left, r.bottom + 4, acts.slice(1).map((x) => ({ label: x.label, key: x.key, run: () => runAction(it, x) })));
      }, "more");
      more.title = acts.slice(1).map((x) => x.label).join(" · ");
      right.append(more);
    }
  }
  top.append(right);
  row.append(top);
  if (it.fold && N.open.has(it.text)) {
    const parts = el("div", "parts");
    for (const p of it.parts || []) parts.append(el("div", "", "· " + p));
    row.append(parts);
  }
  if (it.evidence) {
    const ev = el("div", "ev-line");
    ev.append(el("span", "ev-chip " + it.evidence.kind, it.evidence.label));
    if (it.evidence.source) ev.append(el("span", "src", it.evidence.source));
    row.append(ev);
  }
  row.onclick = () => { N.cur = i; markNeeds(); };
  return row;
}

// markNeeds moves the cursor's mark without building the page again (the keys stay quick).
function markNeeds() {
  N.els.forEach((r, i) => r && r.classList.toggle("cur", i === N.cur));
  const on = N.els[N.cur];
  if (on) on.scrollIntoView({ block: "nearest" });
}

async function runAction(it, act) {
  switch (act.kind) {
    case "sort": case "save":
      return ui.run(act.kind);
    case "open-url":
      try { await api.OpenURL(act.url); say("ok", act.done); } catch (e) { say("fail", errText(e)); }
      return;
    case "view":
      try { pop.text(act.label, await api.Inspect(act.argv)); } catch (e) { say("fail", errText(e)); }
      return;
    case "list":
      return pop.text(act.title, act.lines);
    case "fold":
      N.open.has(it.text) ? N.open.delete(it.text) : N.open.add(it.text);
      return drawNeeds();
    case "choose-disable":
      return pop.choose(act.title, act.options.map((o) => o.label), (i) => doAction(act, i));
  }
  if (act.confirm) {
    // Not undone with u: ask first (06 §6).
    return pop.confirm(act.label, [act.warn || act.label], act.label, () => doAction(act, 0));
  }
  return doAction(act, 0);
}

async function doAction(act, option) {
  try { say("ok", await api.DoAction(act, option), act.undo || act.kind === "enable" || act.kind === "disable" || act.kind === "choose-disable"); } catch (e) { say("fail", errText(e)); }
  await ui.reload();
  if (S.screen === "needs") needs(true);
}

// needsKey: ↑↓ (j k, PageUp/PageDown, Home/End) move through the items, ← → jump to the
// previous / next section; an item's action keys run its actions.
export function needsKey(e) {
  const k = e.key;
  const n = N.items.length;
  if ((k === "ArrowLeft" || k === "ArrowRight") && n) {
    e.preventDefault();
    const sec = N.items[N.cur].section;
    const idx = k === "ArrowRight" ? N.items.findIndex((x) => x.section > sec) : N.items.findIndex((x) => x.section === Math.max(...N.items.filter((y) => y.section < sec).map((y) => y.section)));
    if (idx >= 0) { N.cur = idx; markNeeds(); }
    return;
  }
  const m = n ? moveIn(N.cur, n, k) : null;
  if (m != null) { e.preventDefault(); N.cur = m; return markNeeds(); }
  if (k === "A") return ui.cmd("analyze");
  if (k === "l") return ui.go("log");
  const it = N.items[N.cur];
  const name = k === " " ? "space" : k === "Enter" ? "enter" : k;
  const act = it && (it.actions || []).find((a) => a.key === name);
  if (act) { e.preventDefault(); runAction(it, act); }
}

export function analyzeEvent(ev) {
  if (S.screen === "needs" && ev.done) needs(true);
}

// ---- 游戏日志 (06 §2.10) and Mod 库 › 变化 (§2.9) ----
// tab: 0 the Player.log clusters, 1 the companion log's entries (mcur, open: their cursor
// and the entries whose details are open).
const L = { path: "", page: null, loading: false, pending: "", error: "", cur: 0, detail: new Map(), seq: 0, els: [], tab: 0, mcur: 0, open: new Set(), want: "", list: null };
// of: only this mod's Lineage ({id, name}), from a detail's history; null every mod.
const C = { page: null, cur: 0, items: [], els: [], open: new Set(), raw: new Map(), of: null, library: false };

// logPage keeps the page's head and gives a fresh body below it, and its actions.
function logPage(name, title, ...right) {
  let w = S.screen === name ? $("#screen .page.logs") : null;
  if (!w) {
    const sc = openScreen(name);
    w = page("logs fill", title, ...right);
    sc.append(w);
  } else {
    while (w.children.length > 1) w.lastChild.remove();
    let box = $(".page-head .page-acts", w);
    if (!box) {
      let head = $(".page-head", w);
      if (!head) { head = el("div", "page-head"); w.prepend(head); }
      box = el("div", "page-acts");
      head.append(box);
    }
    box.innerHTML = "";
    for (const r of right) if (r && r.matches("button, input, select")) { if (r.matches("button.btn") && !r.classList.contains("primary")) r.classList.add("quiet"); box.append(r); }
  }
  ui.renderWhere();
  return w;
}

// gameLog shows the page immediately; late responses cannot reopen it after navigation.
export async function gameLog(path = L.path, open = false) {
  const n = ++L.seq;
  if (!open && S.screen !== "log") return;
  L.error = "";
  L.pending = "";
  L.loading = !(L.page && L.page.path && (L.page.path === path || !path));
  drawGameLog(); // switch the page and breadcrumb before waiting for any I/O
  const epoch = S.screenSeq;
  const current = () => n === L.seq && S.screen === "log" && S.screenSeq === epoch;
  let p;
  try { p = await api.LogPage(path || ""); } catch (e) {
    if (current()) { L.loading = false; L.error = errText(e); say("fail", L.error); drawGameLog(); }
    return;
  }
  if (!current()) return;
  if (p.pending) {
    // the mods are being analysed for the log's game version: look again shortly
    L.pending = p.pending;
    L.loading = false;
    L.page = { files: p.files, path: "" };
    L.path = p.path;
    drawGameLog();
    setTimeout(() => { if (current()) gameLog(p.path); }, 1500);
    return;
  }
  L.pending = "";
  if (!L.page || L.page.path !== p.path) L.cur = 0;
  L.page = p;
  L.path = p.path;
  L.loading = false;
  drawGameLog();
}

function drawGameLog() {
  L.list?.destroy(); L.list = null;
  clearTimeout(detailTimer); L.want = "";
  const p = L.page;
  const right = [];
  if (p && p.path && !L.loading) {
    if (p.selected) right.push(el("span", "muted", T("its errors are in needs attention")));
    else right.push(button(T("add its errors to needs attention"), "", async () => {
      try { say("ok", await api.SelectLog(p.path)); } catch (e) { say("fail", errText(e)); }
      await ui.reload();
      gameLog(p.path);
    }));
  }
  const keep = $("#screen .page.logs .col.focus .scroll");
  const was = keep ? keep.scrollTop : 0;
  const w = logPage("log", T("each run's errors, grouped by mod; pick one to read the log"), ...right);
  const files = (p && p.files) || [];
  const bar = el("div", "subbar");
  if (files.length) bar.append(filePicker(files));
  if (p && p.facts && !L.loading) bar.append(el("span", "muted", p.facts));
  w.append(bar);
  if (L.error) { w.append(el("div", "card empty-state err", L.error)); return; }
  if (p && p.noMonitor && !L.loading) w.append(noMonitor(p));
  if (L.pending) { w.append(el("div", "card empty-state", Tf("the mods are analysed for %s first (once per game version)…", L.pending))); return; }
  if (L.loading) { const c = el("div", "card pad"); c.append(shimmer(T("reading the log…"), 6)); w.append(c); return; }
  if (!p || !p.path) { w.append(el("div", "card empty-state", T("no Player.log found"))); return; }
  const mon = p.monitor;
  if (!mon) L.tab = 0;
  const grid = el("div", "cols two logcols");
  const list = el("section", "card col focus");
  if (mon) {
    const tabs = el("div", "ltabs");
    tabs.append(seg([Tf("Player.log · %d", (p.rows || []).length), mon.title + " · " + mon.entries.length], L.tab, (i) => { L.tab = i; drawGameLog(); }, ["←", "→"]));
    list.append(tabs);
  }
  const scroll = el("div", "scroll");
  L.els = [];
  if (L.tab === 1) monitorRows(scroll, mon);
  else {
    if (!(p.rows || []).length) scroll.append(el("div", "empty-state", "✓ " + T("no errors in this log")));
    const rows = p.rows || [];
    L.cur = Math.min(L.cur, Math.max(0, rows.length - 1));
    L.list = virtualRows(scroll, rows.length, (i) => {
      const r = rows[i];
      const row = el("div", "lrow" + (r.info ? " dimmed" : ""));
      const ic = el("span", "ic");
      ic.innerHTML = statusIcon(r.status);
      const body = el("div", "body");
      const top = el("div", "top");
      top.append(el("span", "chip", r.class), el("span", "count", "×" + r.count));
      if (r.mods) top.append(el("span", "mods", r.mods));
      body.append(top, logText(r));
      row.append(ic, body);
      row.onclick = () => { L.cur = i; markLog(); };
      return row;
    });
  }
  list.append(scroll);
  const det = el("section", "card col detail-card");
  const dscroll = el("div", "scroll", "");
  dscroll.tabIndex = -1;
  det.append(dscroll);
  // Archo 日志分析: what the shared logs say about the picked error, its own card under the
  // log text (06 §2.10); hidden while nothing matches
  const ana = el("section", "card col ana-card");
  ana.hidden = true;
  ana.append(el("div", "col-head", ""), el("div", "scroll"));
  ana.firstChild.append(el("span", "anum", "3"), T("Archo log analysis"));
  // the analysis above the log text it explains; the gaps between the three cards drag
  const side = el("div", "logside");
  const hs = el("div", "hsplit");
  hs.hidden = true;
  side.append(ana, hs, det);
  const vs = el("div", "split logsplit");
  grid.append(list, vs, side);
  logSplits(grid, list, vs, ana, hs);
  w.append(grid);
  // 1 the runs (or the monitor's entries), 2 the log text of the one picked
  resetAreas();
  area(list, list.querySelector(".ltabs") || null, () => { det.classList.remove("focus"); list.classList.add("focus"); });
  area(det, null, () => { list.classList.remove("focus"); det.classList.add("focus"); det.querySelector(".scroll").focus(); });
  area(ana, null, () => { if (!ana.hidden) ana.querySelector(".scroll").focus(); });
  if (!list.querySelector(".ltabs")) { list.prepend(el("div", "col-head", "")); list.firstChild.append(el("span", "anum", "1"), T("the errors")); }
  det.prepend(el("div", "col-head", "")); det.firstChild.append(el("span", "anum", "2"), T("the log text"));
  scroll.scrollTop = was;
  markLog(true);
}

// logText draws a log line in the game's colours: the level colours the line, a mod's
// <color>, <b>, <i> (view.LogSpans) style its spans. Text only ever goes in as text; a
// colour is used only when it is a plain #rrggbb.
const HEX = /^#[0-9a-f]{6}$/;
function logText(r) {
  const d = el("div", "text lv-" + (r.level || "info"));
  for (const sp of r.spans || [{ t: r.text }]) {
    const styled = (sp.c && HEX.test(sp.c)) || sp.b || sp.i;
    if (!styled) { d.append(document.createTextNode(sp.t)); continue; }
    const s = el("span", (sp.c && HEX.test(sp.c) ? "rc" : "") + (sp.b ? " rb" : "") + (sp.i ? " ri" : ""));
    s.textContent = sp.t;
    if (sp.c && HEX.test(sp.c)) { s.style.setProperty("--c", sp.c); s.style.setProperty("--l", HEX.test(sp.l || "") ? sp.l : sp.c); }
    d.append(s);
  }
  return d;
}

// filePicker: the log shown, and a menu of the logs — each Player.log with the companion
// log of its run under it (03 §6.7.4); picking a companion shows its section.
function filePicker(files) {
  const shown = files.find((f) => f.path === L.path) || files[0];
  const b = el("button", "btn quiet picker");
  b.append(el("span", "", shown.label), el("span", "muted num", shown.when || ""), el("span", "chev", "⌄"));
  b.onclick = (e) => {
    e.stopPropagation();
    const r = b.getBoundingClientRect();
    pop.menu(r.left, r.bottom + 4, files.map((f) => ({
      label: f.label, key: f.when, cls: (f.of ? "sub" : "") + ((f.of ? f.of === L.path && L.tab === 1 : f.path === L.path && L.tab === 0) ? " on" : ""),
      run: () => {
        L.tab = f.of ? 1 : 0;
        if ((f.of || f.path) === L.path) return drawGameLog();
        L.path = f.of || f.path; L.page = null; L.cur = 0; L.mcur = 0;
        gameLog(L.path);
      },
    })));
  };
  return b;
}

// noMonitor: one line when the current plan has no Archo Monitor, with the default edit.
function noMonitor(p) {
  const n = el("div", "lognote");
  n.append(el("span", "", p.noMonitor));
  if (p.addMonitor) n.append(button(T("add Archo Monitor"), "", async () => {
    try { say("ok", await api.AddMonitor(), true); } catch (e) { say("fail", errText(e)); }
    await ui.reload();
    gameLog(L.path);
  }, "small"));
  return n;
}

// monitorRows: the companion log of the run — time, level, kind, ×count, related mods and
// files; a click opens its details and the Player.log cluster it matches.
function monitorRows(scroll, mon) {
  const head = el("div", "mhead");
  if (mon.running) head.append(el("span", "live-dot"));
  head.append(el("span", "", mon.facts));
  scroll.append(head);
  if (!mon.entries.length) { scroll.append(el("div", "empty-state", "✓ " + T("nothing reported in this run"))); return; }
  L.mcur = Math.min(L.mcur, Math.max(0, mon.entries.length - 1));
  L.list = virtualRows(scroll, mon.entries.length, (i) => {
    const m = mon.entries[i];
    const open = L.open.has(m.key);
    const row = el("div", "lrow mrow" + (open ? " open" : ""));
    const ic = el("span", "ic");
    ic.innerHTML = statusIcon(m.status);
    const body = el("div", "body");
    const top = el("div", "top");
    top.append(el("span", "time num", m.time || ""));
    if (m.kind) top.append(el("span", "chip", m.kind));
    if (m.count > 1) top.append(el("span", "count", "×" + m.count));
    if (m.mods && m.mods.length) { const s = el("span", "mods", m.mods.join(" · ")); s.title = m.mods.join("\n"); top.append(s); }
    body.append(top, logText(m));
    if (m.files && m.files.length) body.append(el("div", "files", m.files.join(" · ")));
    if (open) {
      if (m.details) { const pre = el("pre", "details"); pre.textContent = m.details; pre.onclick = (e) => e.stopPropagation(); body.append(pre); }
      if (m.cluster) {
        const go = el("button", "go", Tf("%s in Player.log", m.cluster) + " →");
        go.onclick = (e) => { e.stopPropagation(); toCluster(m.cluster); };
        body.append(go);
      } else if (m.only) body.append(el("div", "files", Tf("only in %s: the game had stopped writing Player.log", mon.label)));
    }
    row.append(ic, body);
    row.onclick = () => {
      L.mcur = i;
      L.open.has(m.key) ? L.open.delete(m.key) : L.open.add(m.key);
      drawGameLog();
    };
    return row;
  });
}

// toCluster shows a Player.log cluster in the errors tab.
function toCluster(id) {
  const i = ((L.page && L.page.rows) || []).findIndex((r) => r.cluster === id);
  L.tab = 0;
  if (i >= 0) L.cur = i;
  drawGameLog();
  markLog();
}

// logLive refreshes the companion section from the live tracker (pollLive's 2 s tick,
// while the game runs).
export async function logLive() {
  if (S.screen !== "log" || !L.page || !L.page.path) return;
  const path = L.page.path, epoch = S.screenSeq;
  let m;
  try { m = await api.LogMonitor(path); } catch { return; }
  if (!m || S.screen !== "log" || S.screenSeq !== epoch || L.page?.path !== path) return;
  if (JSON.stringify(m) === JSON.stringify(L.page.monitor)) return;
  L.page.monitor = m;
  drawGameLog();
}

let detailTimer = 0;
function markLog(keepScroll) {
  clearTimeout(detailTimer); L.want = "";
  const at = L.tab === 1 ? L.mcur : L.cur;
  L.list?.select(at, !keepScroll);
  const box = $("#screen .detail-card .scroll");
  if (!box) return;
  setAna(null);
  let id = "";
  if (L.tab === 1) {
    const mon = L.page && L.page.monitor;
    const m = mon && mon.entries[at];
    if (!m) { box.innerHTML = ""; box.append(el("div", "empty-state", T("pick an entry on the left"))); return; }
    if (!m.cluster) {
      const why = m.only ? Tf("only in %s: the game had stopped writing Player.log", mon.label) : T("no matching error in Player.log");
      showLines(box, [why, "", m.text, ...(m.details ? ["", m.details] : [])]);
      return;
    }
    id = m.cluster;
  } else {
    const r = L.page && L.page.rows && L.page.rows[at];
    if (!r) { box.innerHTML = ""; box.append(el("div", "empty-state", T("pick an error on the left"))); return; }
    id = r.cluster;
  }
  const k = L.page.path + "|" + id;
  if (L.detail.has(k)) { const d = L.detail.get(k); return showLines(box, d.lines, d.card); }
  box.innerHTML = "";
  box.append(shimmer(T("loading…"), 8));
  clearTimeout(detailTimer);
  L.want = k;
  detailTimer = setTimeout(async () => {
    const path = L.page.path;
    let lines, card = null;
    try {
      [lines, card] = await Promise.all([api.LogCluster(path, id), api.LogPattern(path, id).catch(() => null)]);
    } catch (e) { lines = [errText(e)]; }
    L.detail.set(k, { lines, card });
    if (S.screen === "log" && L.want === k) showLines($("#screen .detail-card .scroll"), lines, card);
  }, 90);
}

// showLines: a cluster's evidence (the CLI text: the whole message, its extra lines, the
// messages around it), then the pattern card when the cluster matches a pattern.
function showLines(box, lines, card) {
  if (!box) return;
  box.innerHTML = "";
  const pre = el("pre", "lines");
  pre.textContent = (lines || []).join("\n");
  box.append(pre);
  setAna(card);
}

// logSplits: the gap between the list and the right side drags the list's width; the gap
// under Archo 日志分析 drags its height. Kept in ui.gui.log.left / ui.gui.log.ana;
// double-click goes back to the default.
const LOG_LEFT = "ui.gui.log.left", LOG_ANA = "ui.gui.log.ana";
const logSize = { left: null, ana: null };
function applyLogSize(grid) {
  if (logSize.left) grid.style.setProperty("--log-left", logSize.left + "px");
  else grid.style.removeProperty("--log-left");
  if (logSize.ana) grid.style.setProperty("--log-ana", logSize.ana + "px");
  else grid.style.removeProperty("--log-ana");
  grid.classList.toggle("ana-sized", !!logSize.ana);
}
function logSplits(grid, list, vs, ana, hs) {
  if (logSize.left == null) {
    Promise.all([api.Setting(LOG_LEFT), api.Setting(LOG_ANA)]).then(([l, a]) => {
      logSize.left = +l || 0; logSize.ana = +a || 0; applyLogSize(grid);
    }).catch(() => { logSize.left = 0; logSize.ana = 0; });
  } else applyLogSize(grid);
  const drag = (sp, key, axis, start, clamp) => {
    sp.addEventListener("mousedown", (e) => {
      e.preventDefault();
      const p0 = axis === "x" ? e.clientX : e.clientY, v0 = start();
      sp.classList.add("drag");
      document.body.classList.add(axis === "x" ? "resizing" : "resizing-y");
      const move = (m) => { logSize[key] = clamp(Math.round(v0 + (axis === "x" ? m.clientX : m.clientY) - p0)); applyLogSize(grid); };
      const up = () => {
        sp.classList.remove("drag");
        document.body.classList.remove("resizing", "resizing-y");
        removeEventListener("mousemove", move);
        removeEventListener("mouseup", up);
        api.SetSetting(key === "left" ? LOG_LEFT : LOG_ANA, String(logSize[key])).catch(() => {});
      };
      addEventListener("mousemove", move);
      addEventListener("mouseup", up);
    });
    sp.addEventListener("dblclick", () => { logSize[key] = 0; applyLogSize(grid); api.SetSetting(key === "left" ? LOG_LEFT : LOG_ANA, "").catch(() => {}); });
  };
  drag(vs, "left", "x", () => list.getBoundingClientRect().width, (v) => Math.max(260, Math.min(grid.clientWidth - 320, v)));
  drag(hs, "ana", "y", () => ana.getBoundingClientRect().height, (v) => Math.max(80, Math.min(grid.clientHeight - 140, v)));
}

// setAna fills the Archo 日志分析 card with a pattern card, or hides it (null).
function setAna(card) {
  const ana = $("#screen .ana-card");
  if (!ana) return;
  const sc = ana.querySelector(".scroll");
  sc.innerHTML = "";
  ana.hidden = !card;
  const hs = ana.nextElementSibling;
  if (hs && hs.classList.contains("hsplit")) hs.hidden = !card;
  if (card) sc.append(patternCard(card));
}

// patternCard (06 §2.10): what the shared logs say this kind of error usually is. It is
// marked as community background, apart from the evidence above: never a conclusion about
// this list.
function patternCard(c) {
  const d = el("div", "pcard");
  // first what it does to the game and how much that matters, then what it usually is and
  // what to try; the causes, the other fixes and the numbers are quieter, and the source
  // (shared logs, not a verdict on this list) closes the card
  if (c.levelText) {
    const h = el("div", "psev");
    h.append(el("span", "sev " + c.level, c.levelText), el("span", "impact", c.impact));
    d.append(h);
  }
  if (c.summary) d.append(el("div", "ps", c.summary));
  if (c.next) d.append(el("div", "pnext", c.next));
  if (c.known) d.append(el("div", "ps known", c.known));
  const more = el("div", "pmore");
  const row = (k, v) => { if (!v) return; const r = el("div", "pr"); r.append(el("span", "k", k), el("span", "v", v)); more.append(r); };
  if (c.causes && c.causes.length) row(T("common causes"), c.causes.join(T("; ")));
  if (c.fixes && c.fixes.length) row(T("kinds of fix"), c.fixes.join(" · "));
  if (more.childNodes.length) d.append(more);
  const facts = [c.seen, c.gone].filter(Boolean).join(" · ");
  if (facts) d.append(el("div", "pm", facts));
  d.append(el("div", "pm note", T("from shared logs, not a conclusion about your list")));
  return d;
}

// ---- Mod 库存 › 变化 (06 §2.9): one row per mod, what its changes mean, by tier ----
// The current plan's mods since that plan was last played (a: the whole library); new and
// gone mods are one summary row; older changes follow by day. enter opens a row (the
// grouped summary, then every fact, then its actions), i its raw file list. ctrl+w opens
// the Workshop change notes, ctrl+o the folder, S sorts, ! goes to 能不能跑, p locks this
// version. tab / shift+tab go to the other sub-page (全部). Data: API.Changes
// (view.BuildChanges → modchanges.Build).

// changesFiltered: 变化 shows one mod's Lineage (the hint line offers esc).
export function changesFiltered() { return S.screen === "changes" && !!C.of; }

// changesPlan: the next 变化 shows the current plan since it was last played (the home
// page's line opens it so).
export function changesPlan() { C.library = false; C.of = null; C.cur = 0; }

// changes shows the page; of: {id, name} narrows it to that mod's Lineage, null shows every
// mod, undefined keeps what it shows.
export async function changes(of) {
  if (of !== undefined && (of && of.id) !== (C.of && C.of.id)) { C.of = of; C.cur = 0; C.open.clear(); }
  let p;
  try { p = await api.Changes(C.library, C.of ? C.of.id : ""); } catch (e) { say("fail", errText(e)); return; }
  C.page = p;
  drawChanges();
}

export function rescanButton(label) {
  const b = button(S.busy.scan ? T("scanning…") : label || T("rescan the mod folders"), "R", () => ui.cmd("rescan"));
  b.classList.add("rescan");
  if (label) b.dataset.label = label;
  b.prepend(el("span", "ic" + (S.busy.scan ? " spin" : "")));
  b.firstChild.innerHTML = icon.rescan;
  b.disabled = !!S.busy.scan;
  return b;
}

// busyChanged redraws what shows the scan's state (the rescan buttons) and, after a scan,
// the pages that show what it found.
export function busyChanged() {
  if (S.screen === "env") return environment();
  if (S.screen === "changes") return changes();
  const b = document.querySelector(".page .rescan");
  if (b) b.replaceWith(rescanButton(b.dataset.label));
}

const rowKey = (r) => r.id + "|" + (r.before || 0) + "|" + (r.after || 0);
const TIER = { 1: "launch", 2: "watch", 3: "content" };

function drawChanges() {
  const top = ($("#screen .page-scroll") || {}).scrollTop || 0;
  const w = logPage("changes", T("what changed in your mods"), rescanButton());
  const scroll = el("div", "scroll page-scroll chg");
  w.append(scroll);
  const p = C.page || {};
  // the scope and time, or the one condition (removable as on Mod 库存)
  const bar = el("div", "chg-bar");
  if (C.of) {
    const b = el("button", "lib-cond");
    b.append(el("span", "", Tf("only %s", C.of.name)), el("span", "x", "×"));
    b.title = T("esc shows every mod");
    b.onclick = () => changes(null).then(() => ui.renderHints());
    bar.append(b);
  } else {
    const seg = el("div", "chg-scope");
    for (const [lib, label] of [[false, T("the current plan")], [true, T("the whole library")]]) {
      const b = el("button", "pill" + (C.library === lib ? " on" : ""), label);
      b.title = "a";
      b.onclick = () => { if (C.library !== lib) toggleScope(); };
      seg.append(b);
    }
    bar.append(seg);
    if (p.since) bar.append(el("span", "muted", p.since));
  }
  scroll.append(bar);
  C.items = [];
  C.els = [];
  if (p.noScan) { scroll.append(el("div", "card empty-state", T("no scan yet (R scans)"))); return finish(); }
  if (!C.of) {
    const card = el("section", "card chg-card");
    for (const r of p.rows || []) modRow(card, r);
    if (p.arrivals) group(card, p.arrivals, "now");
    if (!card.children.length) card.append(el("div", "empty-state", T("nothing changed")));
    scroll.append(card);
  }
  const older = p.older || [];
  if (older.length && !C.of) scroll.append(el("h2", "label chg-older", T("earlier")));
  for (const d of older) {
    const k = "day:" + d.day;
    const open = !!C.of || C.open.has(k);
    const card = el("section", "card chg-card");
    const n = (d.rows || []).length + ((d.arrivals && d.arrivals.rows) || []).length;
    const head = el("div", "chg-row chg-day");
    head.append(el("span", "mod", d.day), el("span", "muted", Tf("%d mods", n)), el("span", "chev", open ? "▾" : "▸"));
    item(head, { t: "day", key: k });
    card.append(head);
    if (open) {
      for (const r of d.rows || []) modRow(card, r);
      if (d.arrivals) group(card, d.arrivals, d.day);
    }
    scroll.append(card);
  }
  finish();
  function finish() {
    C.cur = Math.min(C.cur, Math.max(C.items.length - 1, 0));
    markRows(C);
    const sc = $("#screen .page-scroll");
    if (sc) sc.scrollTop = top;
    ui.renderHints();
  }
}

// item registers a selectable row: a click selects it and opens or closes it.
function item(node, it) {
  const i = C.items.length;
  C.items.push(it);
  C.els.push(node);
  node.onclick = (e) => { if (e.target.closest("button")) return; C.cur = i; toggle(it); };
}

// modRow: [tier] name · sentence ▸, opened: the summary, every fact, the files, the actions.
function modRow(card, r) {
  const k = rowKey(r);
  const open = C.open.has(k);
  const row = el("div", "chg-row");
  row.append(el("span", "tier " + (TIER[r.tier] || ""), r.tierText), el("span", "mod", r.mod), el("span", "txt", r.text));
  if ((r.details && r.details.length) || r.actions || hasFiles(r)) row.append(el("span", "chev", open ? "▾" : "▸"));
  item(row, { t: "mod", key: k, r });
  card.append(row);
  if (!open) return;
  const box = el("div", "chg-open");
  for (const d of r.details || []) box.append(el("div", "line", d));
  if (hasFiles(r)) {
    const f = el("button", "chg-files", (C.raw.has(k) ? "▾ " : "▸ ") + T("files"));
    f.title = "i";
    f.onclick = () => rawFiles(r);
    box.append(f);
    if (C.raw.has(k)) {
      const pre = el("pre", "lines files");
      pre.textContent = C.raw.get(k).join("\n");
      box.append(pre);
    }
  }
  if (r.actions && r.actions.length) {
    const acts = el("div", "chg-acts");
    for (const a of r.actions) {
      const b = el("button", "btn", a.label);
      b.append(el("kbd", "", a.key));
      b.onclick = () => changeAct(r, a.kind);
      acts.append(b);
    }
    box.append(acts);
  }
  card.append(box);
}

const hasFiles = (r) => r.before && r.after && r.before !== r.after;

// group: the one row of new and gone mods; opened, each of them.
function group(card, g, where) {
  const k = "group:" + where;
  const open = C.open.has(k);
  const row = el("div", "chg-row chg-group");
  row.append(el("span", "tier content", T("new and gone")), el("span", "txt", g.text), el("span", "chev", open ? "▾" : "▸"));
  item(row, { t: "group", key: k });
  card.append(row);
  if (open) for (const r of g.rows || []) modRow(card, r);
}

function toggle(it) {
  C.open.has(it.key) ? C.open.delete(it.key) : C.open.add(it.key);
  drawChanges();
}

async function rawFiles(r) {
  const k = rowKey(r);
  if (C.raw.has(k)) C.raw.delete(k);
  else {
    try { C.raw.set(k, (await api.ChangeFiles(r.before, r.after)) || []); } catch (e) { return say("fail", errText(e)); }
    C.open.add(k);
  }
  drawChanges();
}

function toggleScope() {
  if (C.of) return;
  C.library = !C.library;
  C.cur = 0;
  changes();
}

// changeAct runs a row's action (each also has its key on the page).
async function changeAct(r, kind) {
  switch (kind) {
    case "workshop":
      try { say("ok", await api.OpenChangeNotes(r.workshopId)); } catch (e) { say("fail", errText(e)); }
      return;
    case "deps": return ui.go("needs");
    case "sort": return ui.run("sort");
    case "lock": {
      // a copy in the lock library: the dry run first, as on Mod 库存 (06 §6)
      let p;
      try { p = await api.LibraryPreview("pin", [r.ref]); } catch (e) { return say("fail", errText(e)); }
      pop.confirm(p.title, p.lines, T("lock this one"), p.apply ? async () => {
        try { say("ok", await api.LibraryDo("pin", [r.ref])); } catch (e) { say("fail", errText(e)); }
        await ui.reload();
        changes();
      } : null);
    }
  }
}

// changesHints: the hint line on 变化 — the moves, then what the row under the cursor offers.
export function changesHints() {
  const it = C.items[C.cur];
  const out = [{ key: "↑↓", label: T("move") }, { key: "enter", label: T("open") }];
  if (it && it.r) {
    if (hasFiles(it.r)) out.push({ key: "i", label: T("files") });
    // ctrl+o first: the hint line folds it with ctrl+w into one place
    out.push({ key: "ctrl+o", label: T("open the folder") });
    for (const a of it.r.actions || []) out.push({ key: a.key, label: a.label });
  }
  if (!C.of) out.push({ key: "a", label: C.library ? T("the current plan") : T("the whole library") });
  out.push({ key: "tab", label: T("all mods") });
  return out;
}

function markRows(X) {
  X.els.forEach((r, i) => r.classList.toggle("cur", i === X.cur));
  const on = X.els[X.cur];
  if (on) on.scrollIntoView({ block: "nearest" });
}

// logsKey: the arrows move in the game log's errors or in the changes; enter opens a
// change's files.
export function logsKey(e) {
  const k = e.key;
  // focus in the log text (2): the arrows and page keys scroll it
  if (S.screen === "log" && document.activeElement && document.activeElement.closest(".detail-card") && /^(Arrow(Up|Down)|Page(Up|Down)|Home|End)$/.test(k)) return;
  if (S.screen === "log") {
    const mon = L.page && L.page.monitor;
    if (mon && (k === "ArrowLeft" || k === "ArrowRight")) { e.preventDefault(); L.tab = k === "ArrowRight" ? 1 : 0; return drawGameLog(); }
    if (L.tab === 1 && mon) {
      const n = mon.entries.length;
      const m = n ? moveIn(L.mcur, n, k) : null;
      if (m != null) { e.preventDefault(); L.mcur = m; return markLog(); }
      const it = mon.entries[L.mcur];
      if (k === "Enter" && it) { e.preventDefault(); L.open.has(it.key) ? L.open.delete(it.key) : L.open.add(it.key); return drawGameLog(); }
      return;
    }
    const n = (L.page && L.page.rows && L.page.rows.length) || 0;
    const m = n ? moveIn(L.cur, n, k) : null;
    if (m != null) { e.preventDefault(); L.cur = m; return markLog(); }
    return;
  }
  return changesKey(e);
}

// changesKey: the keys of 变化 (every action on the page has one).
function changesKey(e) {
  const k = e.key;
  const ctrl = e.ctrlKey || e.metaKey;
  const done = () => e.preventDefault();
  if (k === "Escape" && C.of) { done(); return changes(null).then(() => ui.renderHints()); }
  if (k === "Tab") { done(); return ui.go("library"); }
  const m = C.items.length ? moveIn(C.cur, C.items.length, k) : null;
  if (m != null) { done(); C.cur = m; markRows(C); return ui.renderHints(); }
  const it = C.items[C.cur];
  const r = it && it.r;
  const has = (kind) => r && (r.actions || []).some((a) => a.kind === kind);
  if (ctrl) {
    const lk = k.toLowerCase();
    if (lk === "o" && r) { done(); return api.OpenFolder(r.id).then(() => say("ok", T("opened the folder")), (x) => say("fail", errText(x))); }
    if (lk === "w" && r) { done(); return changeAct(r, "workshop"); }
    return;
  }
  if (k === "Enter" && it) { done(); return toggle(it); }
  if (k === "i" && r && hasFiles(r)) { done(); return rawFiles(r); }
  if (k === "a" && !C.of) { done(); return toggleScope(); }
  if (k === "R") { done(); return ui.cmd("rescan"); }
  if (k === "S" && has("sort")) { done(); return changeAct(r, "sort"); }
  if (k === "D" && has("deps")) { done(); return changeAct(r, "deps"); }
  if (k === "p" && has("lock")) { done(); return changeAct(r, "lock"); }
}

// ---- the full detail (06 §4.2): one page for the editor and Mod 库存 (fulldetail.js) ----
export { full, fullKey, fullSection } from "./fulldetail.js";

// ---- settings (06 §2.14) ----
// Every row is a value and the command that changes it (view.SettingsPage); the page runs
// only those commands (API.ApplySetting).

export async function settings() {
  let groups, bg;
  try { [groups, bg] = await Promise.all([api.Settings(), bgSection()]); } catch (e) { say("fail", errText(e)); return; }
  groups = groups || [];
  const sc = S.screen === "settings" ? $("#screen") : openScreen("settings");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("settings", T("how RimArcho looks and works; each row shows the command that changes it"));
  sc.append(w);
  w.append(bg);
  renderGroups(w, groups, settings);
  sc.scrollTop = top;
}

// settingRunner runs a setting's command, then redraws the page with again.
function settingRunner(again) {
  return async (argv, value, after) => {
    try {
      const out = await api.ApplySetting(argv, value || "");
      say("ok", (out && out[0]) || T("done"));
    } catch (e) { say("fail", errText(e)); }
    if (after) return after();
    await ui.reload();
    again();
  };
}

// renderGroups draws setting groups (view.SettingGroup) as labelled cards.
function renderGroups(w, groups, again) {
  const run = settingRunner(again);
  for (const g of groups) {
    const lab = el("div", "label section", g.title);
    w.append(lab);
    const card = el("section", "card settings");
    area(card, lab);
    for (const it of g.items || []) {
      const row = el("div", "set");
      const left = el("div", "l");
      left.append(el("div", "name", it.label));
      if (it.note) left.append(el("div", "sub", it.note));
      const right = el("div", "r");
      if (it.edit === "choice") {
        const s = el("select");
        for (const [i, c] of it.choices.entries()) {
          const o = el("option", "", (it.choiceLabels && it.choiceLabels[i]) || c);
          o.value = c;
          if (c === it.value) o.selected = true;
          s.append(o);
        }
        // A new language: the page's text again, in place (main.js relang).
        s.onchange = () => run(it.argv, s.value, it.argv[2] === "ui.lang" ? () => ui.relang() : null);
        right.append(s);
      } else if (it.edit) {
        const inp = el("input", "field");
        inp.value = it.value || "";
        inp.spellcheck = false;
        const ok = button(T("apply"), "", () => run(it.argv, inp.value.trim()));
        inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); ok.click(); } };
        right.append(inp);
        // A folder: the window's folder dialog; the chosen folder is applied at once.
        if (it.edit === "path") right.append(button(T("choose…"), "", () => pickFolder(inp.value, (p) => run(it.argv, p))));
        right.append(ok);
      } else if (it.value) {
        right.append(el("span", "muted", it.value));
      }
      for (const a of it.actions || []) {
        // 卸载 RimArcho is its own page with the plan; every other action is a command
        if (a.argv[0] === "uninstall") { right.append(button(a.label, "", () => ui.go(a.argv[0]))); continue; }
        right.append(button(a.label, "", () => a.confirm ? pop.confirm(a.label, [a.confirm], a.label, () => run(a.argv)) : run(a.argv)));
      }
      row.append(left, right);
      card.append(row);
    }
    w.append(card);
  }
}

// pickFolder opens the window's folder dialog at start and calls then with the folder
// chosen (nothing when cancelled). Without a window (the development harness) it says so:
// every path can be typed.
export async function pickFolder(start, then) {
  let p;
  try { p = await api.PickFolder(start || ""); } catch (e) { say("fail", errText(e)); return; }
  if (p) then(p);
}

// ---- the environment (the pill on the left of the top bar) ----
// Everything here belongs to one game folder (02 §0): which one is in use, the folder, its
// version, how it is started and its saves. The mod folders every game shares are on the
// settings page.

export async function environment() {
  let p;
  try { p = await api.Environment(); } catch (e) { say("fail", errText(e)); return; }
  const sc = S.screen === "env" ? $("#screen") : openScreen("env");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("env", T("an environment is one game folder: its version, how it starts, its saves"), act(T("add an environment"), "plus", () => ui.go("guide")), rescanButton());
  sc.append(w);
  const envLab = el("div", "label section", T("every environment"));
  w.append(envLab);
  const card = el("section", "card");
  area(card, envLab);
  for (const e of p.envs || []) {
    const row = el("div", "envrow" + (e.current ? " on" : ""));
    const l = el("div", "l");
    l.append(el("div", "name", e.name), el("div", "sub", e.game));
    row.append(l);
    if (e.current) row.append(el("span", "chip cur", T("in use")));
    else {
      row.append(button(T("use this one"), "", async () => {
        try { say("ok", await api.UseEnv(e.name)); } catch (err) { say("fail", errText(err)); }
        for (const c of S.cols) { c.query = ""; c.cursor = 0; c.sel.clear(); }
        await ui.reload();
        environment();
      }));
    }
    row.append(button(T("remove environment"), "", () => removeEnv(e.name), "quiet"));
    card.append(row);
  }
  w.append(card);
  renderGroups(w, p.groups || [], environment);
  sc.scrollTop = top;
}

// removeEnv: the dry run says what RimArcho forgets and what stays, one confirmation removes
// it (06 §2.14). Another environment takes over when it was in use; with none left, the
// guide opens.
export async function removeEnv(name) {
  let p;
  try { p = await api.EnvRemovePreview(name); } catch (e) { say("fail", errText(e)); return; }
  pop.confirm(p.title, p.lines, T("remove environment"), async () => {
    try { say("ok", await api.RemoveEnv(name)); } catch (e) { say("fail", errText(e)); return; }
    for (const c of S.cols) { c.query = ""; c.cursor = 0; c.sel.clear(); }
    await ui.reload();
    if (!(S.board && S.board.envs && S.board.envs.length)) return ui.go("guide");
    if (S.screen === "env") environment();
  });
}
