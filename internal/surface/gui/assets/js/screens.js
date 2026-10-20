// The pages over the board (the top bar's tabs): 需要处理 (06 §7), 日志 (the game log, mod
// changes 06 §4.1, the manager's actions 08 §14), the full detail (§6.3), packs and
// settings. esc returns to the mod list; the tab shows where you are.
import { S, $, el, api, T, Tf, errText, say, cur } from "./state.js";
import { statusIcon } from "./icons.js";
import { onHistory } from "./lists.js";
import * as pop from "./popups.js";

let ui = null;
export function initScreens(u) { ui = u; }

const GROUPS = ["this list", "missing dependencies", "conflicts", "last run", "needs your answer", "hidden mods", "this run"];

function openScreen(name) {
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

// page builds a page: a head (title, then what sits right of it) and a body.
function page(cls, title, ...right) {
  const w = el("div", "page " + cls);
  const head = el("div", "page-head");
  head.append(el("h2", "", title));
  for (const r of right) if (r) head.append(r);
  w.append(head);
  return w;
}

function button(label, keyName, run, cls = "") {
  const b = el("button", "btn " + cls, label);
  if (keyName) b.append(el("kbd", "", keyName));
  b.onclick = (e) => { e.stopPropagation(); run(); };
  return b;
}

// seg is a small view switch (magpie's): one button per choice, the chosen one raised.
function seg(names, on, pick, keys) {
  const s = el("div", "seg sub");
  names.forEach((n, i) => {
    const b = el("button", i === on ? "on" : "", n);
    if (keys) b.title = keys[i];
    b.onclick = () => pick(i);
    s.append(b);
  });
  return s;
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

// ---- 需要处理: three columns ----
// The list itself (its order, missing dependencies, conflicts), the game (this run, the
// last run's log) and what needs your call (identity questions, hidden mods).
const NCOLS = [
  { title: "list", groups: [0, 1, 2] },
  { title: "the game", groups: [6, 3] },
  { title: "your call", groups: [4, 5] },
];
const N = { data: null, cols: [[], [], []], col: 0, cur: [0, 0, 0], open: new Set(), els: [[], [], []] };

export function liveChanged() {
  if (S.screen === "needs" && N.data) { splitNeeds(); drawNeeds(); }
}

function splitNeeds() {
  const items = ((S.live && S.live.items) || []).concat(N.data.items || []);
  N.cols = NCOLS.map((c) => items.filter((it) => c.groups.includes(it.group)).sort((a, b) => c.groups.indexOf(a.group) - c.groups.indexOf(b.group)));
  N.cur = N.cur.map((x, i) => Math.min(x, Math.max(N.cols[i].length - 1, 0)));
  if (!N.cols[N.col].length) { const f = N.cols.findIndex((c) => c.length); if (f >= 0) N.col = f; }
}

export async function needs(keepCursor) {
  let data;
  try { data = await api.Needs(); } catch (e) { say("fail", errText(e)); return; }
  N.data = data;
  if (!keepCursor) { N.cur = [0, 0, 0]; N.col = 0; }
  splitNeeds();
  drawNeeds();
}

function drawNeeds() {
  const sc = S.screen === "needs" ? $("#screen") : openScreen("needs");
  const tops = [...sc.querySelectorAll(".col .scroll")].map((x) => x.scrollTop);
  sc.innerHTML = "";
  const d = N.data;
  let state = "";
  if (S.busy.analyze) state = T("checking the mods' files…");
  else if (d.never) state = T("the mods' files are not checked yet");
  else if (d.stale) state = T("the files changed since the last check");
  const w = page("needs fill", d.count ? Tf("%d to handle", d.count) : T("nothing to handle"),
    state ? el("span", "muted", state) : null, el("span", "grow"),
    button(T("check the files again"), "A", () => ui.cmd("analyze")),
    button(T("game log"), "l", () => logs(0)));
  const grid = el("div", "cols three");
  N.els = [[], [], []];
  NCOLS.forEach((c, ci) => {
    const card = el("section", "card col" + (ci === N.col ? " focus" : ""));
    const head = el("div", "card-head");
    head.append(el("span", "label", T(c.title)), el("span", "n", String(N.cols[ci].filter((x) => x.counted).length || "")));
    const scroll = el("div", "scroll");
    if (!N.cols[ci].length) scroll.append(el("div", "empty-state", "✓ " + T("nothing here")));
    let g = -1;
    N.cols[ci].forEach((it, i) => {
      if (it.group !== g && c.groups.length > 1) {
        g = it.group;
        scroll.append(el("div", "group", T(GROUPS[g])));
      }
      const row = needRow(it, ci, i);
      N.els[ci][i] = row;
      scroll.append(row);
    });
    card.append(head, scroll);
    card.onmousedown = () => { if (N.col !== ci) { N.col = ci; markNeeds(); } };
    grid.append(card);
  });
  w.append(grid);
  sc.append(w);
  [...sc.querySelectorAll(".col .scroll")].forEach((x, i) => { x.scrollTop = tops[i] || 0; });
  markNeeds(true);
}

function needRow(it, ci, i) {
  const row = el("div", "need" + (it.dim ? " dimmed" : ""));
  const ic = el("span", "ic");
  ic.innerHTML = it.dim || !it.status ? "" : statusIcon(it.status);
  const body = el("div", "body");
  body.append(el("div", "text", it.text));
  if (it.fold && N.open.has(it.text)) {
    const parts = el("div", "parts");
    for (const p of it.parts || []) parts.append(el("div", "", "· " + p));
    body.append(parts);
  }
  const acts = el("div", "acts");
  for (const act of it.actions || []) {
    acts.append(button(act.label, act.key, () => { N.col = ci; N.cur[ci] = i; markNeeds(); runAction(it, act); }, "small"));
  }
  body.append(acts);
  row.append(ic, body);
  row.onclick = () => { N.col = ci; N.cur[ci] = i; markNeeds(); };
  return row;
}

// markNeeds moves the cursor's mark without building the page again (the keys stay quick).
function markNeeds(initial) {
  $$cols().forEach((c, ci) => c.classList.toggle("focus", ci === N.col));
  N.els.forEach((rows, ci) => rows.forEach((r, i) => r.classList.toggle("cur", i === N.cur[ci] && ci === N.col)));
  const on = N.els[N.col][N.cur[N.col]];
  if (on) on.scrollIntoView({ block: initial ? "nearest" : "nearest" });
}
const $$cols = () => [...document.querySelectorAll("#screen .cols .col")];

async function runAction(it, act) {
  switch (act.kind) {
    case "sort": case "save":
      return ui.preview(act.kind);
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
    // Not undone with u: ask first (06 §9, 08 §8).
    return pop.confirm(act.label, [act.warn || act.label], act.label, () => doAction(act, 0));
  }
  return doAction(act, 0);
}

async function doAction(act, option) {
  try { say("ok", await api.DoAction(act, option), act.kind === "enable" || act.kind === "disable" || act.kind === "choose-disable"); } catch (e) { say("fail", errText(e)); }
  await ui.reload();
  if (S.screen === "needs") needs(true);
}

export function needsKey(e) {
  const k = e.key;
  if (k === "ArrowLeft" || k === "ArrowRight") {
    e.preventDefault();
    const d = k === "ArrowRight" ? 1 : -1;
    for (let c = N.col + d; c >= 0 && c < 3; c += d) if (N.cols[c].length) { N.col = c; break; }
    return markNeeds();
  }
  const n = N.cols[N.col].length;
  const m = n ? moveIn(N.cur[N.col], n, k) : null;
  if (m != null) { e.preventDefault(); N.cur[N.col] = m; return markNeeds(); }
  if (k === "A") return ui.cmd("analyze");
  if (k === "l") return logs(0);
  const it = N.cols[N.col][N.cur[N.col]];
  const name = k === " " ? "space" : k === "Enter" ? "enter" : k;
  const act = it && (it.actions || []).find((a) => a.key === name);
  if (act) { e.preventDefault(); runAction(it, act); }
}

export function analyzeEvent(ev) {
  if (S.screen === "needs" && ev.done) needs(true);
}

// chooseLog is the palette's "choose a log": the game log page.
export function chooseLog() { return logs(0); }

// ---- 日志: the game log, mod changes, the manager's actions ----
const SUBS = ["game log", "mod changes", "manager actions"];
const L = { sub: 0, path: "", page: null, loading: false, pending: "", cur: 0, detail: new Map(), seq: 0, els: [] };
const C = { scans: [], cur: 0, rows: [], open: new Map(), els: [] };
const H = { items: [], cur: -1, els: [] };

export function logsWhere() { return S.screen === "logs" ? T("logs") + " › " + T(SUBS[L.sub]) : ""; }

export async function logs(sub = L.sub) {
  L.sub = sub;
  if (sub === 0) return gameLog();
  if (sub === 1) return changes();
  return actions();
}
// changes is the palette's and c's "recent changes".
export function changesPage() { return logs(1); }

function logsPage(...right) {
  const sc = S.screen === "logs" ? $("#screen") : openScreen("logs");
  sc.innerHTML = "";
  const w = page("logs fill", T("logs"), seg(SUBS.map((s) => T(s)), L.sub, (i) => logs(i), ["1", "2", "3"]), el("span", "grow"), ...right);
  sc.append(w);
  ui.renderWhere();
  return w;
}

async function gameLog(path = L.path) {
  const n = ++L.seq;
  // The page shows at once; the log is read behind it (a new log takes a moment).
  if (!L.page || L.page.path !== path || !path) {
    L.loading = true;
    drawGameLog();
  }
  let p;
  try { p = await api.LogPage(path || ""); } catch (e) { L.loading = false; if (n === L.seq) { say("fail", errText(e)); drawGameLog(); } return; }
  if (n !== L.seq) return;
  if (p.pending) {
    // the mods are being analysed for the log's game version: look again shortly
    L.pending = p.pending;
    L.loading = false;
    L.page = { files: p.files, path: "" };
    L.path = p.path;
    if (S.screen === "logs" && L.sub === 0) drawGameLog();
    setTimeout(() => { if (S.screen === "logs" && L.sub === 0 && n === L.seq) gameLog(p.path); }, 1500);
    return;
  }
  L.pending = "";
  if (!L.page || L.page.path !== p.path) L.cur = 0;
  L.page = p;
  L.path = p.path;
  L.loading = false;
  if (S.screen === "logs" && L.sub === 0) drawGameLog();
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
  const w = logsPage(...right);
  const files = (p && p.files) || [];
  const bar = el("div", "subbar");
  if (files.length) {
    bar.append(seg(files.map((f) => f.label + (f.when ? " · " + f.when : "")), Math.max(0, files.findIndex((f) => f.path === L.path)), (i) => { L.path = files[i].path; L.page = null; gameLog(files[i].path); }));
  }
  if (p && p.facts && !L.loading) bar.append(el("span", "muted", p.facts));
  w.append(bar);
  if (L.pending) { w.append(el("div", "card empty-state", Tf("the mods are analysed for %s first (once per game version)…", L.pending))); return; }
  if (L.loading) { w.append(el("div", "card empty-state", T("reading the log…"))); return; }
  if (!p || !p.path) { w.append(el("div", "card empty-state", T("no Player.log found"))); return; }
  const grid = el("div", "cols two");
  const list = el("section", "card col focus");
  const scroll = el("div", "scroll");
  L.els = [];
  if (!(p.rows || []).length) scroll.append(el("div", "empty-state", "✓ " + T("no errors in this log")));
  (p.rows || []).forEach((r, i) => {
    const row = el("div", "lrow" + (r.info ? " dimmed" : ""));
    const ic = el("span", "ic");
    ic.innerHTML = statusIcon(r.status);
    const body = el("div", "body");
    const top = el("div", "top");
    top.append(el("span", "chip", r.class), el("span", "count", "×" + r.count));
    if (r.mods) top.append(el("span", "mods", r.mods));
    body.append(top, el("div", "text", r.text));
    row.append(ic, body);
    row.onclick = () => { L.cur = i; markLog(); };
    L.els.push(row);
    scroll.append(row);
  });
  list.append(scroll);
  const det = el("section", "card col detail-card");
  det.append(el("div", "scroll", ""));
  grid.append(list, det);
  w.append(grid);
  markLog();
}

let detailTimer = 0;
function markLog() {
  L.els.forEach((r, i) => r.classList.toggle("cur", i === L.cur));
  const on = L.els[L.cur];
  if (on) on.scrollIntoView({ block: "nearest" });
  const r = L.page && L.page.rows && L.page.rows[L.cur];
  const box = $("#screen .detail-card .scroll");
  if (!box) return;
  if (!r) { box.innerHTML = ""; box.append(el("div", "empty-state", T("pick an error on the left"))); return; }
  const k = L.page.path + "|" + r.cluster;
  if (L.detail.has(k)) return showLines(box, L.detail.get(k));
  box.innerHTML = "";
  box.append(el("div", "empty-state", T("loading…")));
  clearTimeout(detailTimer);
  detailTimer = setTimeout(async () => {
    let lines;
    try { lines = await api.LogCluster(L.page.path, r.cluster); } catch (e) { lines = [errText(e)]; }
    L.detail.set(k, lines);
    const now = L.page && L.page.rows && L.page.rows[L.cur];
    if (now && L.page.path + "|" + now.cluster === k) showLines($("#screen .detail-card .scroll"), lines);
  }, 90);
}

function showLines(box, lines) {
  if (!box) return;
  box.innerHTML = "";
  const pre = el("pre", "lines");
  pre.textContent = (lines || []).join("\n");
  box.append(pre);
}

async function changes() {
  try { C.scans = (await api.Changes()) || []; } catch (e) { say("fail", errText(e)); return; }
  C.cur = Math.min(C.cur, 1e9);
  drawChanges();
}

function drawChanges() {
  const w = logsPage();
  const scroll = el("div", "scroll page-scroll");
  w.append(scroll);
  if (!C.scans.length) { scroll.append(el("div", "card empty-state", T("no scan yet (R scans)"))); return; }
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

async function actions() {
  try { H.items = (await api.History()) || []; } catch (e) { say("fail", errText(e)); return; }
  const w = logsPage();
  const scroll = el("div", "scroll page-scroll");
  w.append(scroll);
  const card = el("section", "card");
  const head = el("div", "card-head");
  head.append(el("span", "label", T("manager actions")), el("span", "muted", T("unsaved steps and saved versions; click one to go there")));
  card.append(head);
  H.els = [];
  if (H.items.length <= 1) card.append(el("div", "empty-state", T("no actions yet")));
  let versions = false;
  H.items.forEach((h, i) => {
    if (h.kind === "version" && !versions) { versions = true; card.append(el("div", "group", T("saved versions"))); }
    const r = el("div", "ev h " + h.kind, h.text);
    if (h.kind === "now") r.prepend(el("span", "dot"));
    r.onclick = () => { H.cur = i; markRows(H); if (h.kind !== "now") goHistory(h); };
    H.els.push(r);
    card.append(r);
  });
  scroll.append(card);
  if (H.cur < 0) H.cur = Math.max(0, H.items.findIndex((h) => h.kind === "now"));
  H.cur = Math.max(0, Math.min(H.cur, H.items.length - 1));
  markRows(H);
}

async function goHistory(h) {
  await onHistory(h);
  if (S.screen === "logs" && L.sub === 2) actions();
}

export function logsKey(e) {
  const k = e.key;
  if (k === "Tab") { e.preventDefault(); return logs((L.sub + (e.shiftKey ? 2 : 1)) % 3); }
  if (!e.shiftKey && "123".includes(k) && k.length === 1) { e.preventDefault(); return logs(+k - 1); }
  if (L.sub === 0) {
    const n = (L.page && L.page.rows && L.page.rows.length) || 0;
    const m = n ? moveIn(L.cur, n, k) : null;
    if (m != null) { e.preventDefault(); L.cur = m; return markLog(); }
    return;
  }
  const X = L.sub === 1 ? C : H;
  const n = L.sub === 1 ? C.rows.length : H.items.length;
  const m = n ? moveIn(X.cur, n, k) : null;
  if (m != null) { e.preventDefault(); X.cur = m; return markRows(X); }
  if (k === "Enter") {
    e.preventDefault();
    if (L.sub === 1 && C.rows[C.cur]) return toggleFiles(C.rows[C.cur]);
    const h = H.items[H.cur];
    if (L.sub === 2 && h && h.kind !== "now") return goHistory(h);
  }
}

// ---- the full detail (06 §6.3): ←→ the previous / next mod, tab the sub-sections ----
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
  const w = page("full", d ? d.name : "", el("span", "muted", T("← → the previous / next mod · tab the sections")));
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
  } else card.append(el("p", "muted", T("loading…")));
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

// ---- settings (10 §7) ----
// Every row is a value and the command that changes it (view.SettingsPage); the page runs
// only those commands (API.ApplySetting).

export async function settings() {
  let groups;
  try { groups = (await api.Settings()) || []; } catch (e) { say("fail", errText(e)); return; }
  const sc = S.screen === "settings" ? $("#screen") : openScreen("settings");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("settings", T("settings"));
  sc.append(w);
  const run = async (argv, value, after) => {
    try {
      const out = await api.ApplySetting(argv, value || "");
      say("ok", (out && out[0]) || T("done"));
    } catch (e) { say("fail", errText(e)); }
    if (after) return after();
    await ui.reload();
    settings();
  };
  for (const g of groups) {
    w.append(el("div", "label section", g.title));
    const card = el("section", "card");
    for (const it of g.items || []) {
      const row = el("div", "set");
      const left = el("div", "l");
      left.append(el("div", "name", it.label));
      if (it.note) left.append(el("div", "sub", it.note));
      const right = el("div", "r");
      if (it.edit === "choice") {
        const s = el("select");
        for (const c of it.choices) {
          const o = el("option", "", c);
          o.value = c;
          if (c === it.value) o.selected = true;
          s.append(o);
        }
        // A new language needs the page's text again: reload after it is set.
        s.onchange = () => run(it.argv, s.value, it.argv[2] === "ui.lang" ? () => location.reload() : null);
        right.append(s);
      } else if (it.edit) {
        const inp = el("input", "field");
        inp.value = it.value || "";
        inp.spellcheck = false;
        const ok = button(T("apply"), "", () => run(it.argv, inp.value.trim()));
        inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); ok.click(); } };
        right.append(inp, ok);
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
  sc.scrollTop = top;
}

// ---- packs (10 §8) ----
// The packs like versions in a game launcher: play one, or build one from the current list.

export async function packs() {
  let rows;
  try { rows = (await api.Packs()) || []; } catch (e) { say("fail", errText(e)); return; }
  const sc = S.screen === "packs" ? $("#screen") : openScreen("packs");
  sc.innerHTML = "";
  const name = el("input", "field");
  name.placeholder = T("name of a new pack");
  name.spellcheck = false;
  const build = button(T("build from the current list"), "", async () => {
    const n = name.value.trim();
    if (!n) { name.focus(); return; }
    let pv;
    try { pv = await api.PackPreview(n); } catch (e) { return say("fail", errText(e)); }
    pop.confirm(Tf("build pack %s", n), pv.lines, T("build"), pv.ok ? async () => {
      say("busy", T("copying the mods…"));
      try { say("ok", await api.PackBuild(n)); } catch (e) { say("fail", errText(e)); }
      packs();
    } : null);
  });
  name.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); build.click(); } };
  const w = page("packs", T("packs"), el("span", "grow"), name, build);
  sc.append(w);
  const card = el("section", "card");
  if (!rows.length) {
    card.append(el("div", "empty-state", T("no packs yet: a pack fixes the current list — every mod as it is now, without the files of other game versions — and plays with its own saves")));
  }
  for (const r of rows) {
    const row = el("div", "pack");
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
}
