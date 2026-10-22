// The pages over the board (the sidebar's entries, 06 §2.1): 能不能跑 (06 §2.4, §5), the
// game log (§2.10), Mod 库 › 变化 (§2.9), the full detail (§4.2), packs, the environment
// and settings. The pages are side by side: esc does not go from one to another (06 §3.1);
// esc leaves only the full detail, back to the plan. A page's actions sit in the top bar.
import { S, $, el, api, T, Tf, errText, say, cur, slideThumb } from "./state.js";
import { icon } from "./icons.js";
import { statusIcon } from "./icons.js";
import { setActs } from "./shell.js";
import { bump, shimmer } from "./motion.js";
import * as pop from "./popups.js";

let ui = null;
export function initScreens(u) { ui = u; }

// fresh: the next page built is a new page (not a redraw of the one shown); only a new page
// fades in, so a redraw after an action or a sub-view switch does not flicker.
let fresh = false;

export function openScreen(name) {
  fresh = S.screen !== name;
  S.screen = name;
  const sc = $("#screen");
  sc.hidden = false;
  sc.innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return sc;
}

export function closeScreen() {
  if (!S.screen) return false;
  S.screen = null;
  $("#screen").hidden = true;
  $("#screen").innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return true;
}

// page builds a page: one muted sentence that says what the page is for (the breadcrumb
// names it, 06 §2.2), then the body. The buttons and fields among right are the page's
// actions: they go to the top bar, quiet unless primary; anything else joins the lead.
export function page(cls, lead, ...right) {
  const w = el("div", "page " + cls + (fresh ? " enter" : ""));
  fresh = false;
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
  setActs("", ...acts);
  if (head.childNodes.length) w.append(head);
  return w;
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
const L = { path: "", page: null, loading: false, pending: "", cur: 0, detail: new Map(), seq: 0, els: [], tab: 0, mcur: 0, open: new Set(), want: "" };
// of: only this mod's Lineage ({id, name}), from a detail's history; null every mod.
const C = { scans: [], cur: 0, rows: [], open: new Map(), els: [], of: null };

// logPage keeps the page's head and gives a fresh body below it; the actions go to the top
// bar.
function logPage(name, title, ...right) {
  let w = S.screen === name ? $("#screen .page.logs") : null;
  if (!w) {
    const sc = openScreen(name);
    w = page("logs fill", title, ...right);
    sc.append(w);
  } else {
    while (w.children.length > 1) w.lastChild.remove();
    setActs("", ...right.filter((r) => r && r.matches("button, input, select")));
  }
  ui.renderWhere();
  return w;
}

// gameLog reads the log page; open: it was navigated to, so it is drawn even when the read
// comes back before the loading line would show (unless another page opened meanwhile).
export async function gameLog(path = L.path, open = false) {
  const n = ++L.seq;
  const from = S.screen;
  const show = () => S.screen === "log" || (open && S.screen === from);
  // What was read before shows at once and is replaced when the new read is in; a log not
  // read yet says so only if it takes a moment (no flash of a loading line).
  let slow = 0;
  if (L.page && L.page.path && (L.page.path === path || !path)) drawGameLog();
  else slow = setTimeout(() => { if (n === L.seq) { L.loading = true; drawGameLog(); } }, 150);
  let p;
  try { p = await api.LogPage(path || ""); } catch (e) { L.loading = false; if (n === L.seq) { say("fail", errText(e)); drawGameLog(); } return; }
  clearTimeout(slow);
  if (n !== L.seq) return;
  if (p.pending) {
    // the mods are being analysed for the log's game version: look again shortly
    L.pending = p.pending;
    L.loading = false;
    L.page = { files: p.files, path: "" };
    L.path = p.path;
    if (show()) drawGameLog();
    setTimeout(() => { if (S.screen === "log" && n === L.seq) gameLog(p.path); }, 1500);
    return;
  }
  L.pending = "";
  const same = L.page && !L.loading && JSON.stringify(L.page) === JSON.stringify(p);
  if (!L.page || L.page.path !== p.path) L.cur = 0;
  L.page = p;
  if (same) return;
  L.path = p.path;
  L.loading = false;
  if (show()) drawGameLog();
}

function drawGameLog() {
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
  if (p && p.noMonitor && !L.loading) w.append(noMonitor(p));
  if (L.pending) { w.append(el("div", "card empty-state", Tf("the mods are analysed for %s first (once per game version)…", L.pending))); return; }
  if (L.loading) { const c = el("div", "card pad"); c.append(shimmer(T("reading the log…"), 6)); w.append(c); return; }
  if (!p || !p.path) { w.append(el("div", "card empty-state", T("no Player.log found"))); return; }
  const mon = p.monitor;
  if (!mon) L.tab = 0;
  const grid = el("div", "cols two");
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
    (p.rows || []).forEach((r, i) => {
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
      L.els.push(row);
      scroll.append(row);
    });
  }
  list.append(scroll);
  const det = el("section", "card col detail-card");
  det.append(el("div", "scroll", ""));
  grid.append(list, det);
  w.append(grid);
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
  mon.entries.forEach((m, i) => {
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
    L.els.push(row);
    scroll.append(row);
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
  let m;
  try { m = await api.LogMonitor(L.page.path); } catch { return; }
  if (!m || S.screen !== "log" || !L.page) return;
  if (JSON.stringify(m) === JSON.stringify(L.page.monitor)) return;
  L.page.monitor = m;
  drawGameLog();
}

let detailTimer = 0;
function markLog(keepScroll) {
  const at = L.tab === 1 ? L.mcur : L.cur;
  L.els.forEach((r, i) => r.classList.toggle("cur", i === at));
  const on = L.els[at];
  if (on && !keepScroll) on.scrollIntoView({ block: "nearest" });
  const box = $("#screen .detail-card .scroll");
  if (!box) return;
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
  if (L.detail.has(k)) return showLines(box, L.detail.get(k));
  box.innerHTML = "";
  box.append(shimmer(T("loading…"), 8));
  clearTimeout(detailTimer);
  L.want = k;
  detailTimer = setTimeout(async () => {
    let lines;
    try { lines = await api.LogCluster(L.page.path, id); } catch (e) { lines = [errText(e)]; }
    L.detail.set(k, lines);
    if (L.want === k) showLines($("#screen .detail-card .scroll"), lines);
  }, 90);
}

function showLines(box, lines) {
  if (!box) return;
  box.innerHTML = "";
  const pre = el("pre", "lines");
  pre.textContent = (lines || []).join("\n");
  box.append(pre);
}

// changesFiltered: 变化 shows one mod's Lineage (the hint line offers esc).
export function changesFiltered() { return S.screen === "changes" && !!C.of; }

// changes shows the page; of: {id, name} narrows it to that mod's Lineage, null shows every
// mod, undefined keeps what it shows.
export async function changes(of) {
  if (of !== undefined && (of && of.id) !== (C.of && C.of.id)) { C.of = of; C.cur = 0; }
  try { C.scans = (await api.Changes(C.of ? C.of.id : "")) || []; } catch (e) { say("fail", errText(e)); return; }
  C.cur = Math.min(C.cur, 1e9);
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
  const b = document.querySelector("#acts .rescan, .page .rescan");
  if (b) b.replaceWith(rescanButton(b.dataset.label));
}

function drawChanges() {
  const w = logPage("changes", T("what each scan found: mods added, updated and gone"), rescanButton());
  const scroll = el("div", "scroll page-scroll");
  w.append(scroll);
  if (C.of) {
    // the one condition, removable as on Mod 库存
    const bar = el("div", "chg-of");
    const b = el("button", "lib-cond");
    b.append(el("span", "", Tf("only %s", C.of.name)), el("span", "x", "×"));
    b.title = T("esc shows every mod");
    b.onclick = () => changes(null).then(() => ui.renderHints());
    bar.append(b);
    scroll.append(bar);
  }
  if (!C.scans.length) { scroll.append(el("div", "card empty-state", C.of ? T("no change recorded for this mod") : T("no scan yet (R scans)"))); return; }
  C.rows = [];
  C.els = [];
  for (const s of C.scans) {
    const card = el("section", "card");
    const head = el("div", "card-head");
    head.append(el("span", "label", Tf("scan %s", s.time)), el("span", "muted", s.summary));
    card.append(head);
    for (const e of s.events || []) {
      const i = C.rows.length;
      C.rows.push(e);
      const r = el("div", "ev");
      r.append(el("span", "mod", e.mod), el("span", "muted", e.text));
      if (e.files) r.append(el("span", "faint", C.open.has(e.id) ? "▾" : "▸"));
      r.onclick = () => { C.cur = i; toggleFiles(e); };
      C.els.push(r);
      card.append(r);
      if (C.open.has(e.id)) card.append(el("pre", "lines files", C.open.get(e.id).join("\n")));
    }
    scroll.append(card);
  }
  C.cur = Math.min(C.cur, Math.max(C.rows.length - 1, 0));
  markRows(C);
}

async function toggleFiles(e) {
  const top = ($("#screen .page-scroll") || {}).scrollTop || 0;
  if (C.open.has(e.id)) C.open.delete(e.id);
  else if (e.files) C.open.set(e.id, (await api.FileDiff(e.id)) || []);
  drawChanges();
  const sc = $("#screen .page-scroll");
  if (sc) sc.scrollTop = top;
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
  if (k === "Escape" && C.of) { e.preventDefault(); return changes(null).then(() => ui.renderHints()); }
  const m = C.rows.length ? moveIn(C.cur, C.rows.length, k) : null;
  if (m != null) { e.preventDefault(); C.cur = m; return markRows(C); }
  if (k === "Enter" && C.rows[C.cur]) { e.preventDefault(); return toggleFiles(C.rows[C.cur]); }
}

// ---- the full detail (06 §4.2): ←→ the previous / next mod, tab the sub-sections ----
const F = { tab: 0, secs: null, id: "" };

export async function full() {
  const it = cur();
  if (!it || !it.id) return;
  if (F.id !== it.id) { F.tab = 0; F.secs = null; F.id = it.id; }
  openScreen("full");
  drawFull();
  try { F.secs = await api.Sections(it.id); } catch (e) { F.secs = []; say("fail", errText(e)); }
  if (S.screen === "full" && F.id === it.id) drawFull();
}

function drawFull() {
  const sc = $("#screen");
  sc.innerHTML = "";
  const d = S.detail;
  const names = [T("summary"), ...(F.secs || []).map((s) => s.title + (s.count >= 0 ? " · " + s.count : ""))];
  const w = page("full", T("← → the previous / next mod · tab the sections"));
  const bar = el("div", "subbar");
  bar.append(seg(names, F.tab, (i) => { F.tab = i; drawFull(); }));
  w.append(bar);
  const card = el("section", "card pad");
  if (F.tab === 0) {
    const src = $("#detail");
    card.append(src.cloneNode(true));
    // re-bind the links of the copy through the original's handlers
    card.querySelectorAll(".link").forEach((a, i) => { const o = src.querySelectorAll(".link")[i]; if (o) a.onclick = o.onclick; });
  } else if (F.secs) {
    const s = F.secs[F.tab - 1];
    const pre = el("pre", "lines");
    pre.textContent = (s.lines || []).join("\n");
    card.append(pre);
  } else card.append(shimmer(T("loading…"), 8));
  w.append(card);
  sc.append(w);
  ui.renderWhere();
}

export function fullKey(e) {
  const n = 1 + (F.secs ? F.secs.length : 0);
  if (e.key === "Tab") { F.tab = (F.tab + (e.shiftKey ? n - 1 : 1)) % n; drawFull(); e.preventDefault(); return; }
  if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
    e.preventDefault();
    ui.step(e.key === "ArrowRight" ? 1 : -1).then(() => full());
    return;
  }
  const sc = $("#screen");
  if (e.key === "ArrowDown") sc.scrollTop += 40;
  else if (e.key === "ArrowUp") sc.scrollTop -= 40;
  else if (e.key === "PageDown" || e.key === " ") sc.scrollTop += sc.clientHeight * 0.9;
  else if (e.key === "PageUp") sc.scrollTop -= sc.clientHeight * 0.9;
  else return;
  e.preventDefault();
}

export function fullSection() {
  if (S.screen !== "full" || !F.secs || F.tab === 0) return "";
  return F.secs[F.tab - 1].title;
}

// ---- settings (06 §2.14) ----
// Every row is a value and the command that changes it (view.SettingsPage); the page runs
// only those commands (API.ApplySetting).

export async function settings() {
  let groups;
  try { groups = (await api.Settings()) || []; } catch (e) { say("fail", errText(e)); return; }
  const sc = S.screen === "settings" ? $("#screen") : openScreen("settings");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("settings", T("how RimArcho looks and works; each row shows the command that changes it"));
  sc.append(w);
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
    w.append(el("div", "label section", g.title));
    const card = el("section", "card settings");
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
  const card = el("section", "card");
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
    card.append(row);
  }
  w.append(card);
  renderGroups(w, p.groups || [], environment);
  sc.scrollTop = top;
}

// ---- packs (08 §5) ----
// The packs like versions in a game launcher: play one, or build one from the current list.
// A build runs in the background (an `archo pack build` child): the page shows its steps
// and progress from the "pack" events, and a build another window started, or one cut
// short, from the list (Packs().unfinished).

const PACK_STEPS = ["check the list", "sort for the pack", "copy the mods", "write pack.json", "ready to play"];
const builds = {};      // name → the preview's steps, for the build card
let packPoll = 0;

// flow draws the build's steps as boxes joined by arrows. steps: the preview's (with
// facts); at: the step a build is at (-1: a preview); done: the build ended.
function flow(steps, at = -1, done = false) {
  const f = el("div", "flow");
  steps.forEach((s, i) => {
    if (i) f.append(el("div", "arrow" + (at > i - 1 || done ? " on" : "")));
    let cls = "step";
    if (s.level) cls += " " + s.level;
    if (at >= 0) cls += done || i < at ? " done" : i === at ? " cur" : " wait";
    const b = el("div", cls);
    const h = el("div", "h");
    h.append(el("span", "n", done || (at >= 0 && i < at) ? "✓" : String(i + 1)), el("span", "t", s.title));
    b.append(h);
    for (const x of s.facts || []) b.append(el("div", "f", x));
    f.append(b);
  });
  return f;
}

function plainSteps() { return PACK_STEPS.map((t) => ({ title: T(t), facts: [] })); }

// attention lists what the player must look at: the steps marked amber or red, with their
// details (the blockers, the mods that cannot be copied, the version mismatches).
function attention(steps) {
  const box = el("div", "attn");
  for (const s of steps) {
    if (s.level !== "warn" && s.level !== "bad") continue;
    const g = el("div", "a " + s.level);
    g.append(el("div", "h", [s.title, ...(s.facts || [])].join(" · ")));
    if (s.more && s.more.length) {
      const ul = el("ul");
      for (const m of s.more) ul.append(el("li", "", m));
      g.append(ul);
    }
    box.append(g);
  }
  return box.childNodes.length ? box : null;
}

function progressBox(p) {
  const w = el("div", "prog");
  const bar = el("div", "bar" + (p.error ? " bad" : ""));
  const fill = el("i");
  fill.style.width = Math.round((p.frac || 0) * 100) + "%";
  bar.append(fill);
  w.append(bar, el("div", "ptext" + (p.error ? " bad" : ""), p.error || p.text || ""));
  return w;
}

// buildCard is a build on the page: running (here or in another window) or cut short.
function buildCard(u) {
  const card = el("section", "card build" + (u.running ? "" : " unfinished"));
  card.dataset.build = u.name;
  const head = el("div", "pack");
  const left = el("div", "l");
  left.append(el("div", "name", u.running ? Tf("building pack %s", u.name) : u.name), el("div", "sub", u.facts || ""));
  const right = el("div", "r");
  if (u.running) {
    right.append(button(T("stop"), "", async () => {
      try { say("ok", await api.PackStop(u.name)); } catch (e) { say("fail", errText(e)); }
    }));
  } else {
    right.append(button(T("build again"), "", () => previewBuild(u.name), "primary"));
    right.append(button(T("delete what it copied"), "", () => pop.confirm(T("delete what it copied"), [Tf("the unfinished build of %s and what it copied (%s) are deleted; your mods are not touched", u.name, u.copied || "?")], T("delete what it copied"), async () => {
      say("busy", T("deleting what it copied…"));
      try { say("ok", await api.PackRemoveUnfinished(u.name)); } catch (e) { say("fail", errText(e)); }
      packs();
    }), "danger"));
  }
  head.append(left, right);
  card.append(head);
  if (u.running) {
    const p = u.progress || { step: 0, frac: 0, text: "" };
    const body = el("div", "flowwrap");
    body.append(flow(builds[u.name] || plainSteps(), p.step, p.done), progressBox(p));
    card.append(body);
  }
  return card;
}

// packEvent is a "pack" event: the progress of the build this window started, and its end.
export function packEvent(ev) {
  S.packBuild = ev.done ? null : ev;
  if (ev.done) {
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    if (S.screen === "packs") packs();
    return;
  }
  if (S.screen !== "packs") { say("busy", Tf("building pack %s", ev.name) + " · " + ev.text); return; }
  const card = document.querySelector(`.card.build[data-build="${CSS.escape(ev.name)}"]`);
  if (!card) { packs(); return; }
  const body = card.querySelector(".flowwrap");
  body.innerHTML = "";
  body.append(flow(builds[ev.name] || plainSteps(), ev.step, false), progressBox(ev));
  card.querySelector(".sub").textContent = T("building") + " · " + ev.text;
}

async function previewBuild(n) {
  let pv;
  say("busy", T("checking the list…"));
  try { pv = await api.PackPreview(n); } catch (e) { return say("fail", errText(e)); }
  say("", "");
  const body = el("div", "packpv");
  body.append(flow(pv.steps));
  const a = attention(pv.steps);
  if (a) body.append(a);
  if (pv.dir) body.append(el("div", "muted dir", T("folder") + ": " + pv.dir));
  pop.panel(Tf("build pack %s", n), body, T("build"), pv.ok ? async () => {
    builds[n] = pv.steps;
    try { say("ok", await api.PackBuild(n)); } catch (e) { say("fail", errText(e)); }
    packs();
  } : null, "wide");
}

export async function packs() {
  let rows, others = 0, un = [];
  try {
    const l = await api.Packs();
    rows = (l && l.rows) || []; others = (l && l.others) || 0; un = (l && l.unfinished) || [];
    // This window's build before its child wrote its first state.
    if (l && l.building && !un.some((u) => u.name === l.building)) un.unshift({ name: l.building, running: true, facts: T("checking the list…") });
  } catch (e) { say("fail", errText(e)); return; }
  const sc = S.screen === "packs" ? $("#screen") : openScreen("packs");
  const typed = sc.querySelector(".page.packs input.field");
  const keep = typed ? typed.value : "";
  sc.innerHTML = "";
  const name = el("input", "field");
  name.placeholder = T("name of a new pack");
  name.spellcheck = false;
  name.value = keep;
  const build = button(T("build from the current list"), "", () => {
    const n = name.value.trim();
    if (!n) { name.focus(); return; }
    previewBuild(n);
  });
  name.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); build.click(); } };
  const w = page("packs", T("a pack copies every mod of the list as it is now and plays with its own saves"), name, build);
  sc.append(w);
  for (const u of un) {
    if (u.running && S.packBuild && S.packBuild.name === u.name) u.progress = S.packBuild;
    w.append(buildCard(u));
  }
  const card = el("section", "card");
  if (!rows.length) {
    card.append(el("div", "empty-state", T("no packs yet: a pack fixes the current list — every mod as it is now, without the files of other game versions — and plays with its own saves")));
  }
  let focus = null;
  for (const r of rows) {
    const row = el("div", "pack" + (S.packFocus === r.name ? " hl" : ""));
    if (S.packFocus === r.name) focus = row;
    const left = el("div", "l");
    left.append(el("div", "name", r.name), el("div", "sub", r.facts + " · " + r.played));
    const right = el("div", "r");
    if (!r.light) {
      right.append(button(T("play") + " ▶", "", async () => {
        try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
        packs();
      }, "primary"));
    }
    right.append(button(T("move to the recycle bin"), "", () => pop.confirm(T("move to the recycle bin"), [Tf("pack %s goes to the recycle bin; you can restore it from there", r.name)], T("move to the recycle bin"), async () => {
      try { say("ok", await api.PackRemove(r.name)); } catch (e) { say("fail", errText(e)); }
      packs();
    }), "danger"));
    row.append(left, right);
    card.append(row);
  }
  w.append(card);
  if (focus) requestAnimationFrame(() => focus.scrollIntoView({ block: "nearest" }));
  if (others) w.append(el("div", "muted note", Tf("%d packs belong to other environments; switch the environment to see them", others)));
  // A build this window does not watch (started before a restart, or elsewhere): look
  // again while it runs.
  clearTimeout(packPoll);
  if (un.some((u) => u.running && !(S.packBuild && S.packBuild.name === u.name))) {
    packPoll = setTimeout(() => { if (S.screen === "packs" && !S.popup) packs(); else if (S.screen === "packs") packPoll = setTimeout(packs, 1500); }, 1500);
  }
}
