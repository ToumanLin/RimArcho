// The pages over the board (the sidebar's entries, 06 §2.1): 能不能跑 (06 §2.4, §5), the
// game log (§2.10), Mod 库 › 变化 (§2.9), the full detail (§4.2), packs, the environment
// and settings. The pages are side by side: esc does not go from one to another (06 §3.1);
// esc leaves only the full detail, back to the plan. A page's actions sit in the top bar.
import { choiceControl } from "./choices.js";
import { linkRows } from "./linkrows.js";
import { copyablePath, editablePath, pathContent } from "./fields.js";
import { factText, S, $, el, api, T, Tf, errText, say, cur, slideThumb } from "./state.js";
import { icon, bowMark } from "./icons.js";
import { statusIcon } from "./icons.js";
import { bump, shimmer } from "./motion.js";
import * as pop from "./popups.js";
import { bgSection } from "./bg.js";
import { appearanceControl } from "./appearance.js";
import { virtualRows } from "./virtualrows.js";
import { on } from "./api.js";
import { wireSeparator } from "./separator.js";
import { changes, changesKey, initSubscriptions } from "./subscriptions.js";
import { setNeedRunner } from "./badgepop.js";
import { initNeedText, needText, needParts, needEvidence } from "./needtext.js";
import { getChoices } from "./credit.js";
import { setVersionRunner } from "./versionwarnings.js";
import { openVersionList } from "./versionpanel.js";

let ui = null;
// hintsChanged redraws the hint line after a page's own state changed what it lists.
export const hintsChanged = () => ui && ui.renderHints();
export function initScreens(u) {
  ui = u;
  initNeedText(u);
  initSubscriptions(u, { beginPageRead, page, area, button, act, seg });
  on("automatic-update", ev => {
    // A lapsed LoversLab login needs the player; update counts are plain news.
    if (ev?.notice) say(ev.kind === "session" ? "warn" : "ok", ev.notice);
    if (S.screen === "settings") refreshSettingTasks(true);
    if (S.screen === "changes" && ev?.kind === "mods") changes();
    // a new RimArcho puts its line on the launch page (06 §2.3); found rarely, so redraw
    if (S.screen === "home" && ev?.kind === "self" && ev?.notice) ui.reload();
  });
  on("tasks", (ev) => {
    if (S.screen === "settings") refreshSettingTasks(!!ev?.ended);
    if (S.screen === "changes" && ev?.ended) changes();
  });
}

// fresh: the next page built is a new page (not a redraw of the one shown); only a new page
// fades in, so a redraw after an action or a sub-view switch does not flicker.
let fresh = false;
const pageCache = new Map();

// left tells Go the player left a page, which forgets what only that page used (07 §8.3).
function left(next) {
  if (S.screen && S.screen !== next) {
    settlePageRead();
    if (S.screen === "log") { L.list?.destroy(); L.list = null; L.want = ""; clearTimeout(detailTimer); }
    api.Leave(S.screen).catch(() => {});
  }
}

export function openScreen(name) {
  fresh = S.screen !== name;
  if (fresh) S.screenSeq++;
  if (fresh) onArea = 0;
  const previous = $("#screen");
  if (fresh && S.screen && previous.children.length && !pageLoading()) {
    pageCache.set(S.screen, { nodes: [...previous.childNodes], top: previous.scrollTop });
  }
  left(name);
  S.screen = name;
  if (name !== "home") dropBackdrop();
  const sc = $("#screen");
  sc.hidden = false;
  sc.removeAttribute("aria-busy");
  sc.replaceChildren();
  // While the game switches, or when coming back to a page kept warm (06 §2.4: 能不能跑,
  // the editor's other half) for the same game and list, the last drawing shows at once
  // and the page reads again behind it.
  const sameContext = WARM.has(name) && pageContexts.get(name) === JSON.stringify([S.board?.env, S.shell?.plan?.id]);
  const cached = (S.busy.env || sameContext) && pageCache.get(name);
  if (cached) { sc.append(...cached.nodes); sc.scrollTop = cached.top; sc.dataset.warm = sameContext ? "1" : ""; }
  else delete sc.dataset.warm;
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
  $("#screen").removeAttribute("aria-busy");
  $("#screen").innerHTML = "";
  ui.renderWhere();
  ui.renderHints();
  return true;
}

const pageReads = new Map();
// pages kept warm: shown at once from their last drawing when the game and list are the same
const WARM = new Set(["needs"]);
const pageContexts = new Map();
// notePageContext records that a page's content was drawn for the current game and list
// without a page read (the editor behind the start page), so opening it is warm.
export function notePageContext(name) { pageContexts.set(name, JSON.stringify([S.board?.env, S.shell?.plan?.id])); }
let activeRead = null;
export const pendingPageRead = () => activeRead?.done;
function settlePageRead() { activeRead?.resolve(); activeRead = null; }
export const pageLoading = () => !!$("#screen .page-loading, #screen .page-read-error");
// Every page starts navigation before I/O and owns a ticket for its reply. Refreshes
// keep the visible content; a new page/subview shows a placeholder immediately.
// keep: a page that is already drawn stays as it is while it reads, even when the game or
// list changed (the launch page shows that change itself).
export function beginPageRead(name, { replace = false, keep = false, label = "loading…", retry } = {}) {
  ui.pageEntering?.(name);
  const same = S.screen === name;
  const sc = same ? $("#screen") : openScreen(name);
  settlePageRead();
  let resolve;
  const done = new Promise(r => { resolve = r; });
  activeRead = { done, resolve };
  const n = (pageReads.get(name) || 0) + 1;
  pageReads.set(name, n);
  const epoch = S.screenSeq, gameRequest = S.envSeq;
  let env = S.board?.env, plan = S.shell?.plan?.id;
  const context = JSON.stringify([env, plan]);
  const changed = pageContexts.get(name) !== context;
  pageContexts.set(name, context);
  const current = () => S.envSeq === gameRequest && S.screen === name && S.screenSeq === epoch && S.board?.env === env && S.shell?.plan?.id === plan && pageReads.get(name) === n;
  const editor = name === null, board = editor && $("#board");
  // Old object controls cannot remain usable across a game/list switch (06 §8.2).
  const objectPage = editor || name === "library";
  const hasContent = sc.children.length || (editor && same && sc.hidden && S.board);
  // Coming back to the editor of the same list: its columns are still in #board, so show
  // them at once and read again behind them (no placeholder, no wait).
  const warmEditor = editor && !same && !changed && !replace && S.board && !S.board.error && board && board.querySelector(".row");
  const warmPage = !editor && !same && !changed && !replace && sc.dataset.warm === "1" && sc.children.length;
  if (warmEditor) {
    sc.replaceChildren();
    sc.hidden = true;
  } else if (warmPage) {
    // the kept drawing stays; the read below replaces it
  } else if ((!same || (changed && (!S.busy.env || objectPage)) || replace || !hasContent || sc.querySelector(".page-read-error")) && !((keep && same || S.busy.env && !objectPage) && hasContent && !pageLoading())) {
    sc.hidden = false;
    sc.innerHTML = "";
    const w = page(name || "plan", "");
    const card = el("section", "card pad page-loading");
    card.append(shimmer(T(label), 6));
    w.append(card); sc.append(w);
  }
  sc.setAttribute("aria-busy", "true");
  if (board) board.inert = !sc.hidden;
  if (editor && $("#planhead")) $("#planhead").inert = !sc.hidden;
  // A read behind the editor already on screen (after sort, an edit, undo) keeps the status
  // line: it says what the action did, which "loading…" would wipe.
  if (editor && sc.hidden && !warmEditor && !same) say("busy", T(label));
  return {
    current,
    // A workspace reply can itself change the current game/list (06 §8.2).
    commit(apply) {
      if (!current()) return false;
      apply();
      env = S.board?.env; plan = S.shell?.plan?.id;
      pageContexts.set(name, JSON.stringify([env, plan]));
      return true;
    },
    finish() {
      if (!current()) return;
      sc.removeAttribute("aria-busy");
      if (editor && $("#status")?.textContent === T(label)) say("", "");
      ui.pageEntered?.();
      settlePageRead();
    },
    fail(error) {
      if (!current()) return;
      sc.removeAttribute("aria-busy");
      const message = errText(error);
      const placeholder = sc.querySelector(".page-loading");
      if (placeholder) {
        placeholder.className = "card pad err page-read-error"; placeholder.textContent = message;
        if (retry) placeholder.append(button(T("try again"), "", retry));
      }
      say("fail", message);
      settlePageRead();
    },
  };
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
  if (lead) head.append(el("p", "lead word-facts", lead));
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

// archoMark is the small bow in front of Archo's name where the portrait would be too small
// to read (06 §7).
export function archoMark() {
  const m = el("span", "archo-mark");
  m.innerHTML = bowMark(16);
  return m;
}

export function button(label, keyName, run, cls = "", envWrite = "") {
  const b = el("button", "btn " + cls, label);
  if (envWrite) b.dataset.envWrite = envWrite;
  if (keyName) b.append(el("kbd", "", keyName));
  b.onclick = (e) => { e.stopPropagation(); run(); };
  return b;
}

// act is a page action with an icon (the top bar's quiet buttons, 06 §2.2).
export function act(label, ic, run, keyName, envWrite = "") {
  const b = button(label, keyName, run, "", envWrite);
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
    const b = el("button", i === on ? "on word-facts" : "word-facts", n);
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

// ---- 能不能跑 (06 §2.4, §5): two columns ----
// Left (开始之前): 会让游戏出错, 可以留意, 要你确认, what checking the mods' files and the
// shared logs found. Right (上次玩的时候): the run's errors, this run first while the game
// runs. Each item is a card: a bold line, a plain sentence, what belongs to it (link rows,
// its parts, Also, links to the other column), then all its actions as buttons with the
// evidence on the right of that row; no ⋯ menu. An action that needs a confirmation asks in
// the item; the items an action resolved stay in place as one line with 撤销.
const SECTIONS = ["breaks the game", "worth knowing", "for you to confirm", "errors from the last run"];
const SECTION_CLS = ["break", "worth", "confirm", "run"];
const COL_SECS = [[0, 1, 2], [3]]; // view.Section*: the left column's, then the right's
// cols: each column's rows (items and done lines); cur / at: each column's cursor (index,
// and the id it is on, so a new read keeps it); col: the column the keys act on.
// done: the done lines, oldest first; before: the rows as they were when an action started
// here, and ready once that action has finished (a refresh in between leaves it alone);
// result: what that action said and whether u undoes it; ask: the item asking for a
// confirmation in place.
const N = { data: null, cols: [[], []], cur: [0, 0], at: ["", ""], col: 0, open: new Set(), els: [[], []],
  hintsShut: false, done: [], batch: 0, before: null, ready: false, result: null, ask: null, others: false, env: "" };

// archo is the hook for the Archo panel (06 §2.13, 1.1): when set, an item can be handed
// to it ("Archo 在看"). Nothing sets it yet.
export const archo = { ask: null };

export function liveChanged() {
  if (S.screen === "needs" && N.data && !pageLoading()) { orderNeeds(); drawNeeds(); }
}

// liveCount: this run's counted errors (the sidebar's badge adds them, 06 §2.1).
export function liveCount() {
  return ((S.live && S.live.items) || []).filter((it) => it.counted).length;
}

// orderNeeds sorts the items into the two columns and puts the done lines back where their
// items were. Items shown inside another (into) and those other pages handle are left out.
function orderNeeds() {
  const all = ((S.live && S.live.items) || []).concat(N.data.items || []).filter((it) => !it.into && it.section < 4);
  // stable: within a section the view's order (this run before the last run)
  const sorted = all.map((it, i) => [it, i]).sort((a, b) => (a[0].section - b[0].section) || (a[1] - b[1])).map((x) => x[0]);
  const cols = [sorted.filter((it) => it.section !== 3 && !(N.hintsShut && it.section === 1)), sorted.filter((it) => it.section === 3)];
  for (const d of N.done) {
    const c = cols[d.col];
    let at = d.after ? c.findIndex((x) => x.id === d.after) + 1 : 0;
    if (at <= 0) at = c.findIndex((x) => d.after ? x.section > d.section : x.section >= d.section);
    if (at < 0) at = c.length;
    c.splice(at, 0, d);
  }
  N.cols = cols;
  for (const ci of [0, 1]) {
    const i = cols[ci].findIndex((x) => x.id === N.at[ci]);
    N.cur[ci] = i >= 0 ? i : Math.min(N.cur[ci], Math.max(cols[ci].length - 1, 0));
    N.at[ci] = cols[ci][N.cur[ci]]?.id || "";
  }
  if (!cols[N.col].length && cols[1 - N.col].length) N.col = 1 - N.col;
}

export async function needs(keepCursor) {
  const read = beginPageRead("needs");
  let data;
  try { data = await api.Needs(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  // done lines belong to this visit and this game (06 §5)
  const env = (S.board && S.board.env) || "";
  if (!keepCursor || env !== N.env) { N.done = []; N.ask = null; N.before = null; N.ready = false; N.result = null; N.others = false; }
  N.env = env;
  N.data = data;
  // back on a kept page: the cursor stays where it was
  if (!keepCursor && $("#screen").dataset.warm !== "1") { N.cur = [0, 0]; N.at = ["", ""]; N.col = 0; }
  if (N.before && N.ready) settleDone(data);
  orderNeeds();
  drawNeeds();
  read.finish();
  if (!data.memory?.estimated || data.memory?.stale) refreshMemoryEstimate(read);
}

let memoryRead = 0;
async function refreshMemoryEstimate(read) {
  const token = ++memoryRead;
  const env = N.data.environment, plan = N.data.plan, list = N.data.listVersion;
  const current = () => token === memoryRead && S.screen === "needs" && N.data.environment === env && N.data.plan === plan && N.data.listVersion === list && (!read || read.current());
  try {
    const first = await api.PlanLoadCost(plan || 0);
    if (!first || !current()) return;
    let result = first;
    while (result?.loading && current()) {
      await new Promise(resolve => setTimeout(resolve, 250));
      if (!current()) return;
      result = await api.PlanLoadCostMeasure(first.request);
    }
    if (!result || !current()) return;
    if (result.error) { say("fail", result.error); return; }
    needs(true);
  } catch (e) { if (current()) say("fail", errText(e)); }
}

// snapshot: the rows as they are before an action that changes something.
function snapshot() {
  N.ready = false;
  N.result = null;
  N.before = N.cols.flatMap((c, ci) => c.map((x, i) => ({ id: x.id, done: !!x.done, group: x.group, section: x.section,
    kind: x.kind || "", dim: !!x.dim, col: ci, after: i ? c[i - 1].id : "", title: x.done ? x.title : (x.title || x.text) })));
}

// statusNow: what the status line says, and whether it offers 撤销.
function statusNow() {
  const st = $("#status");
  return { text: st ? [...st.childNodes].filter((n) => n.nodeName !== "A").map((n) => n.textContent).join("").trim() : "",
    undo: !!(st && st.querySelector("a")) };
}

// actionDone: the action started after snapshot() has finished; result is what it said
// (else the status line now, before a background task's message replaces it).
function actionDone(result) {
  if (!N.before) return;
  N.result = result || statusNow();
  N.ready = true;
}

// settleDone: the items the action resolved stay as done lines: gone from this read, with
// no new item of the same kind in their place ("7 mods are out of order" becoming "6" is
// the same item, changed). The line says what the action said.
function settleDone(data) {
  const before = N.before;
  const { text, undo } = N.result || statusNow();
  N.before = null;
  N.ready = false;
  N.result = null;
  const had = new Set(before.map((r) => r.id));
  const now = new Set((data.items || []).map((it) => it.id));
  const fresh = new Set((data.items || []).filter((it) => !had.has(it.id) && it.kind).map((it) => it.kind));
  const batch = ++N.batch;
  for (const r of before) {
    if (r.done || now.has(r.id) || r.group === 6 || (r.dim && r.section === 3) || (r.kind && fresh.has(r.kind))) continue;
    N.done.push({ done: true, id: r.id, section: r.section, col: r.col, after: r.after, title: r.title, text: text || T("done"), batch, undo });
  }
}

// undoBatch: the done lines that 撤销 brings back (the last action that u can undo).
function undoBatch() {
  for (let i = N.done.length - 1; i >= 0; i--) if (N.done[i].undo) return N.done[i].batch;
  return 0;
}

async function undoDone() {
  const batch = undoBatch();
  if (!batch) return false;
  await ui.edited(() => api.Undo(), false);
  N.done = N.done.filter((d) => d.batch !== batch);
  if (S.screen === "needs") needs(true);
  return true;
}

// lead: what the page says about the plan in one sentence.
function needsLead(d) {
  const plan = (S.shell && S.shell.plan && S.shell.plan.name) || T("the list");
  const left = N.cols[0].filter((x) => !x.done && x.counted).length;
  const runs = N.cols[1].filter((x) => !x.done && x.counted).length;
  if (left) return Tf("%s is not safe to play yet: %d to handle before you start, %d error groups in the last run", plan, left, runs);
  if (runs) return Tf("%s: %d error groups in the last run", plan, runs);
  if (d.never) return Tf("%s: the mods' files are not checked yet.", plan);
  if (N.cols[0].some((x) => x.status === -1)) return Tf("%s can run; suggestions below may help if you see errors.", plan);
  return Tf("%s can run: nothing to handle.", plan);
}

function drawNeeds() {
  const sc = S.screen === "needs" ? $("#screen") : openScreen("needs");
  const keep = [...sc.querySelectorAll(".needcol > .scroll")].map((s) => s.scrollTop);
  const focused = document.activeElement && sc.contains(document.activeElement) ? document.activeElement.closest(".need")?.dataset.id : "";
  sc.innerHTML = "";
  const d = N.data;
  let state = "";
  if (S.busy.analyze) state = T("checking the mods' files…");
  else if (d.never) state = T("the mods' files are not checked yet");
  else if (d.stale) state = T("the files changed since the last check");
  const w = page("needs fill", needsLead(d),
    state ? el("span", "state-note", state) : null,
    autoFix(),
    act(T("check the files again"), "rescan", () => ui.cmd("analyze"), "A", "analyze"),
    act(T("game log"), "log", () => ui.go("log"), "l"));
  N.els = [[], []];
  const live = N.cols[1].some((x) => x.group === 6);
  const column = (ci, title, sub) => {
    const c = el("section", "needcol" + (N.col === ci ? " focus" : ""));
    const cap = el("div", "needcap");
    cap.append(el("b", "", T(title)), el("span", "", T(sub)));
    const box = el("div", "scroll");
    c.append(cap, box);
    return [c, box];
  };
  const [left, lbox] = column(0, "before you start", "found by checking the mods' files and shared logs");
  const [right, rbox] = column(1, live ? "this run and the last" : "when you last played", "seen in the game's log");
  const split = el("div", "split needsplit");
  const grid = el("div", "cols needcols");
  grid.append(left, split, right);
  needSplit(grid, left, split);
  for (const [ci, box] of [[0, lbox], [1, rbox]]) {
    for (const si of COL_SECS[ci]) {
      const rows = N.cols[ci].filter((x) => x.section === si);
      const shut = si === 1 && N.hintsShut && (d.items || []).some((x) => x.section === 1 && !x.into);
      if (!rows.length && !shut) continue;
      box.append(needSection(si, rows, ci, shut, live));
    }
  }
  if (!N.cols[0].length) lbox.append(el("section", "card empty-state", "✓ " + T("nothing here")));
  if (!N.cols[1].length) rbox.append(el("div", "need-empty", T("no errors in the last run")));
  // what other pages handle: one line each (06 §2.4)
  for (const e of d.elsewhere || []) {
    const b = el("button", "need-away", "→ " + e.text);
    b.onclick = () => e.page === "library" ? document.dispatchEvent(new CustomEvent("library-search", { detail: e.query || "" })) : ui.go(e.page);
    lbox.append(b);
  }
  w.append(grid);
  sc.append(w);
  sc.querySelectorAll(".needcol > .scroll").forEach((s, i) => { s.scrollTop = keep[i] || 0; });
  // 1–9: the sections in the order shown, the left column's first (06 §3.5)
  resetAreas();
  sc.querySelectorAll(".need-sec").forEach((sec) => area(sec, sec.querySelector("h2"), () => {
    const ci = +sec.dataset.col, i = N.cols[ci].findIndex((x) => x.section === +sec.dataset.section);
    if (i >= 0) { N.col = ci; setCur(ci, i); }
  }));
  markNeeds(false);
  if (focused) sc.querySelector(`.need[data-id="${CSS.escape(focused)}"] button`)?.focus({ preventScroll: true });
  ui.renderHints();
}

// needSection: a section's heading and its card. The run's errors say which run; while the
// game runs this run's come first, then a quiet line and the last run's.
function needSection(si, rows, ci, shut, live) {
  const sec = el("section", "sec need-sec " + SECTION_CLS[si]);
  sec.dataset.col = String(ci);
  sec.dataset.section = String(si);
  const title = si === 3 && live ? "errors this run" : SECTIONS[si];
  const h = el("h2", "label", T(title));
  const n = rows.filter((x) => !x.done && x.counted).length;
  if (si === 1) {
    // 可以留意 folds and opens by its heading (06 §2.4)
    const all = (N.data.items || []).filter((x) => x.section === 1 && !x.into).length;
    h.prepend(el("span", "fold-chev" + (shut ? "" : " open"), "›"));
    h.append(el("span", "n num muted", String(all)));
    h.onclick = () => { N.hintsShut = !N.hintsShut; orderNeeds(); drawNeeds(); };
    h.title = T(shut ? "show" : "hide");
  } else if (n) {
    const c = el("span", "n num", String(n));
    h.classList.add("word-facts");
    h.append(c);
    bump("needs:" + si, c, n);
  }
  if (si === 3) {
    if (live) {
      const run = el("span", "muted need-live");
      run.append(el("span", "dot"), document.createTextNode(S.live.pack ? Tf("pack %s", S.live.pack) : T("game running")));
      h.append(run);
    } else if (N.data.lastRun) h.append(el("span", "muted word-facts", N.data.lastRun));
  }
  sec.append(h);
  if (shut) return sec;
  const card = el("div", "card need-list");
  let phase = "", group = -1;
  for (const it of rows) {
    const i = N.cols[ci].indexOf(it);
    if (!it.done) {
      // while the game runs, the last run's items follow this run's: say which run they are
      // from, or the same phase headings repeat over the same errors (view.GroupThisRun 6,
      // GroupLastRun 3)
      if (it.group === 3 && group === 6) card.append(el("div", "need-phase need-run word-facts", T("last run") + (N.data.lastRun ? "  " + N.data.lastRun : "")));
      if (it.phaseHeading && (it.phaseHeading !== phase || it.group !== group)) card.append(el("div", "need-phase", it.phaseHeading));
      phase = it.phaseHeading || ""; group = it.group;
    }
    const row = needCard(it, ci, i);
    N.els[ci][i] = row;
    card.append(row);
  }
  sec.append(card);
  return sec;
}

// autoFix: one click for what needs no question: the required mods that are installed
// but not enabled are enabled and the list sorted again (one undo step). The button counts
// the items it resolves (06 §5).
function autoFix() {
  const items = (N.data.items || []).filter((it) => it.counted && !it.into);
  const enable = items.filter((it) => it.target && (it.actions || []).some((a) => a.kind === "enable")).length;
  if (!enable) return null;
  const n = enable + items.filter((it) => (it.actions || []).some((a) => a.kind === "sort")).length;
  const b = act(Tf("fix %d automatically", n), "ok", async () => {
    snapshot();
    const r = await ui.edited(() => api.CompleteDependencies(), true);
    if (r == null) N.before = null;
    else actionDone({ text: typeof r === "string" ? r : r.text || "", undo: true });
    if (S.screen === "needs") needs(true);
  }, "F", "enable");
  b.classList.add("primary");
  b.title = T("enables the required mods that are installed and sorts the list; u undoes it");
  return b;
}

// needCard: one item, or a done line. The buttons are all its actions: the first is the
// main one; the answers of an identity question and the options of a choice are equal.
function needCard(it, ci, i) {
  if (it.done) return doneLine(it, ci, i);
  const quiet = it.dim && it.section !== 1;
  const row = el("div", "need" + (quiet ? " dimmed" : "") + (it.kind === "memory.capacity" ? " memory-capacity" : ""));
  row.dataset.id = it.id || "";
  const top = el("div", "top");
  const body = el("div", "body");
  const ic = el("span", "need-ic");
  ic.innerHTML = statusIcon(it.status);
  top.append(ic);
  if (quiet) {
    body.append(needText("div", "why word-facts", it.text, it.content?.text));
    if (it.why) body.append(needText("div", "why sub", it.why, it.content?.why));
  } else {
    body.append(needText("div", "title word-facts", it.title || it.text, it.title ? it.content?.title : it.content?.text));
    if (it.why && !it.linkGroups?.length) body.append(needText("div", "why", it.why, it.content?.why));
  }
  top.append(body);
  row.append(top);
  const acts = it.actions || [];
  const pick = (a) => () => { setCur(ci, i); runAction(it, a, true); };
  // what belongs to it: the parts behind a fold, link rows, merged lines, the other column
  const x = el("div", "need-x");
  const fold = acts.find((a) => a.kind === "fold");
  if (fold && it.parts?.length) {
    const on = N.open.has(it.text);
    const b = el("button", "need-disc" + (on ? " open" : ""), Tf("show %d", it.parts.length));
    b.onclick = (e) => { e.stopPropagation(); setCur(ci, i); runAction(it, fold, true); };
    x.append(b);
    if (on) x.append(needParts(it));
  }
  if (it.linkGroups?.length) x.append(linkBlock(it));
  for (const a of it.also || []) x.append(el("div", "need-also", a));
  for (const r of it.related || []) {
    const b = el("button", "need-rel", (ci ? "← " : "→ ") + r.text);
    b.onclick = (e) => { e.stopPropagation(); jumpTo(r.id); };
    x.append(b);
  }
  const foot = el("div", "need-foot");
  const btns = el("div", "acts");
  if (archo.ask && it.section === 3) {
    const b = button(T("ask Archo"), "", () => archo.ask(it), "small");
    b.prepend(archoMark());
    btns.append(b);
  }
  let main = true;
  for (const a of acts) {
    if (a.kind === "fold") continue;
    if (a.kind === "view" && a.argv?.[0] === "identity" && a.argv?.[1] === "why") {
      // 为什么这样问: a quiet link under the sentence, apart from the three answers
      const b = button(a.label, "", pick(a), "identity-why", actionWrite(a));
      if (a.title) b.title = a.title;
      body.append(b);
      continue;
    }
    if (a.kind === "choose-disable") {
      // a choice: each option is a button, none the main one (06 §5)
      a.options.forEach((o, oi) => {
        const b = button(o.label, oi ? "" : a.key, () => { setCur(ci, i); if (N.ask) { N.ask = null; } snapshot(); doAction(a, oi); }, "option", actionWrite(a));
        btns.append(b);
      });
      main = false;
      continue;
    }
    const answer = a.kind === "answer";
    const b = button(a.label, a.key, pick(a), answer ? "" : main ? "primary" : "", actionWrite(a));
    if (a.title) b.title = a.title;
    if (!answer) main = false;
    btns.append(b);
  }
  if (acts.some((a) => a.kind === "answer")) btns.classList.add("identity-answers");
  if (x.children.length) row.append(x);
  if (N.ask && N.ask.id === it.id) row.append(confirmRow(it, N.ask.act));
  else {
    foot.append(btns);
    if (it.evidence) foot.append(needEvidence(it.evidence));
    if (btns.children.length || it.evidence) row.append(foot);
  }
  row.onclick = () => setCur(ci, i);
  return row;
}

// markNeeds moves the cursor's mark without building the page again (the keys stay quick).
function markNeeds(scroll = true) {
  N.els.forEach((col, ci) => col.forEach((r, i) => r && r.classList.toggle("cur", ci === N.col && i === N.cur[ci])));
  document.querySelectorAll("#screen .needcol").forEach((c, ci) => c.classList.toggle("focus", ci === N.col));
  const on = N.els[N.col]?.[N.cur[N.col]];
  if (on && scroll) on.scrollIntoView({ block: "nearest" });
}

function setCur(ci, i) {
  N.col = ci;
  N.cur[ci] = i;
  N.at[ci] = N.cols[ci][i]?.id || "";
  markNeeds();
}

// jumpTo: the item an other column's link names; it lights up for a moment (06 §5).
function jumpTo(id) {
  for (const ci of [0, 1]) {
    const i = N.cols[ci].findIndex((x) => x.id === id);
    if (i < 0) continue;
    setCur(ci, i);
    const r = N.els[ci][i];
    if (r) { r.classList.remove("flash"); void r.offsetWidth; r.classList.add("flash"); }
    return;
  }
}

// doneLine: an item the last actions resolved (06 §5): ✓, what the status line said, its
// title; the latest that u can undo has 撤销.
function doneLine(d, ci, i) {
  const row = el("div", "need done");
  row.dataset.id = d.id;
  const ic = el("span", "need-ic");
  ic.innerHTML = icon.ok || "";
  const body = el("div", "body");
  body.append(el("span", "done-what", d.text), el("span", "done-title", d.title));
  row.append(ic, body);
  if (d.undo && d.batch === undoBatch()) row.append(button(T("undo"), "u", () => undoDone(), "small", "undo"));
  row.onclick = () => setCur(ci, i);
  return row;
}

// confirmRow: the question an action asks before it runs, in place of the buttons (06 §5).
function confirmRow(it, a) {
  const box = el("div", "need-confirm");
  box.append(el("span", "t", a.kind === "repair-links" ? T("apply the repair shown above?") : (a.warn || a.label)));
  const yes = button(a.label, "enter", () => confirmYes(), "primary", actionWrite(a));
  const no = button(T("cancel"), "esc", () => { N.ask = null; drawNeeds(); });
  box.append(yes, no);
  return box;
}

function confirmYes() {
  const a = N.ask && N.ask.act;
  N.ask = null;
  if (!a) return;
  snapshot();
  doAction(a, 0);
}

// linkBlock: the current game's broken links; the other games' fold into one line (the
// repair still covers them all, 06 §2.4).
function linkBlock(it) {
  const gs = it.linkGroups, cur = gs.filter((g) => g.current);
  if (!cur.length || cur.length === gs.length) return linkRows(gs);
  const others = gs.filter((g) => !g.current);
  const box = el("div", "need-links");
  box.append(linkRows(cur));
  const n = others.reduce((s, g) => s + (g.rows || []).length, 0);
  const b = el("button", "need-disc" + (N.others ? " open" : ""), Tf("also %s: %d links to handle", others.map((g) => g.game).join(T("; ")), n));
  b.onclick = (e) => { e.stopPropagation(); N.others = !N.others; drawNeeds(); };
  box.append(b);
  if (N.others) box.append(linkRows(others));
  return box;
}

// needSplit: the gap between the columns drags the left one's width, kept in
// ui.gui.needs.left; double-click goes back to the default (06 §3.1 rule 10).
const NEEDS_LEFT = "ui.gui.needs.left";
let needsLeft = 0;
function needSplit(grid, left, split) {
  const apply = () => needsLeft ? grid.style.setProperty("--needs-left", needsLeft + "px") : grid.style.removeProperty("--needs-left");
  apply();
  const set = (v) => { needsLeft = v; apply(); };
  wireSeparator(split, {
    axis: "x", key: NEEDS_LEFT, get: () => left.getBoundingClientRect().width, range: () => [320, grid.clientWidth - 300],
    set, reset: () => set(0), restore: (v) => set(+v || 0), serialize: () => String(needsLeft),
    enabled: () => !matchMedia("(max-width: 900px)").matches,
  });
}

// Read-only needs actions stay available during a game switch (06 §3.1).
const actionWrite = (a) => ["open-url", "problem", "view", "game-log", "list", "version-list", "manage-lists", "fold"].includes(a.kind) ? "" : a.kind;
// The pinned badge popover in the editor offers the same actions (06 §4.2).
setNeedRunner((it, a) => runAction(it, a), actionWrite);
setVersionRunner((it, a) => runAction(it, a));
// The editor's problems panel (problems.js) runs them too.
export const runNeedAction = (it, a) => runAction(it, a);
export const needActionWrite = actionWrite;

// runAction runs an item's action. here: from 能不能跑 itself, where a confirmation is
// asked in the item, a choice is its buttons, and what the action resolves stays as done
// lines (06 §5); the editor's panel and popover ask in a dialog.
async function runAction(it, act, here = false) {
  switch (act.kind) {
    case "sort": case "save":
      if (here) snapshot();
      await ui.run(act.kind);
      if (here) actionDone();
      if (here && S.screen === "needs") needs(true);
      return;
    case "open-url":
      try { await api.OpenURL(act.url); say("ok", act.done); } catch (e) { say("fail", errText(e)); }
      return;
    case "locate-directory":
      return directoryPrompt({ id: act.refs[0], label: it.title || act.label, path: act.lines?.[0] || "" }, false, async () => { await ui.reload(); needs(true); });
    case "problem":
      return pop.problem(act.problem, (fix) => runAction(it, fix), actionWrite);
    case "game-log":
      syncLogEnvironment();
      L.target = act.cluster || "";
      L.targetLine = act.line || "";
      L.tab = 0;
      L.tabPick = 0;
      return gameLog(act.logPath || "", true);
    case "view":
      if (act.argv?.[0] === "plan" && act.argv?.[1] === "cost") return refreshMemoryEstimate();
      if (act.argv?.[0] === "identity" && act.argv?.[1] === "why") {
        return pop.identity(act.question, (answer) => runAction(it, answer, here));
      }
      try { pop.text(act.label, await api.Inspect(act.argv)); } catch (e) { say("fail", errText(e)); }
      return;
    case "list":
      return pop.text(act.title, act.lines);
    case "version-list":
      return openVersionList(act, runAction, async (id) => { await ui.go("plan"); await ui.palette({ kind: 1, id }); });
    case "manage-lists":
      return ui.manageLists(0);
    case "fold":
      N.open.has(it.text) ? N.open.delete(it.text) : N.open.add(it.text);
      return drawNeeds();
    case "choose-disable":
      // here the options are the item's buttons: the key puts the focus on the first
      if (here) return focusButtons(it, ".btn.option");
      return pop.choose(act.title, act.options.map((o) => o.label), (i) => doAction(act, i));
    case "subscribe":
      // a background task (06 §2.16): the mods take their place in the list once downloaded
      return pop.choose(act.label, getChoices(), async (i) => {
        try { say("ok", await api.Subscribe({ kind: "workshop", url: act.refs.join(" "), steamcmd: i === 1, batch: true })); } catch (e) { say("fail", errText(e)); }
      });
  }
  if (act.confirm) {
    // Not undone with u: ask first (06 §6); on this page in the item itself
    if (here && S.screen === "needs") {
      N.ask = { id: it.id, act };
      drawNeeds();
      return focusButtons(it, ".need-confirm .btn.primary");
    }
    if (act.kind === "repair-links" && act.linkGroups?.length) {
      return pop.panel(act.label, linkRows(act.linkGroups), act.label, () => doAction(act, 0), "link-repair", act.kind);
    }
    return pop.confirm(act.label, [act.warn || act.label], act.label, () => doAction(act, 0));
  }
  if (here) snapshot();
  return doAction(act, 0);
}

function focusButtons(it, sel) {
  const row = $(`#screen .need[data-id="${CSS.escape(it.id || "")}"]`);
  row?.querySelector(sel)?.focus();
}

async function doAction(act, option) {
  let result = null;
  try {
    const text = await api.DoAction(act, option);
    const undo = !!(act.undo || act.kind === "enable" || act.kind === "disable" || act.kind === "choose-disable");
    say("ok", text, undo);
    result = { text, undo };
  } catch (e) { say("fail", errText(e)); N.before = null; }
  await ui.reload();
  actionDone(result);
  if (act.kind === "version-mute" || act.kind === "version-unmute") document.dispatchEvent(new CustomEvent("version-mutes-changed"));
  if (S.screen === "needs") needs(true);
}

// needsKey: ↑↓ (j k, PageUp/PageDown, Home/End) move in the column, ← → change the column;
// an item's action keys run its actions. With the focus on an item's buttons ← → move
// between them and esc gives the keys back to the list; a question asked in place takes
// enter and esc.
export function needsKey(e) {
  // shift+letter is the capital letter even where the key reports it in lower case
  const k = e.shiftKey && /^[a-z]$/.test(e.key) ? e.key.toUpperCase() : e.key;
  if (N.ask) {
    if (k === "Escape") { e.preventDefault(); N.ask = null; return drawNeeds(); }
    if (k === "Enter" && !e.target.closest?.("button")) { e.preventDefault(); return confirmYes(); }
  }
  const onBtn = e.target.closest?.("#screen .need button");
  if (onBtn) {
    if (k === "ArrowLeft" || k === "ArrowRight") {
      e.preventDefault();
      const bs = [...onBtn.closest(".need").querySelectorAll("button")];
      bs[(bs.indexOf(onBtn) + (k === "ArrowRight" ? 1 : bs.length - 1)) % bs.length].focus();
      return;
    }
    if (k === "Escape") { e.preventDefault(); onBtn.blur(); return; }
    if (k === "Enter" || k === " ") return; // the button itself
  }
  if (k === "ArrowLeft" || k === "ArrowRight") {
    e.preventDefault();
    const ci = k === "ArrowLeft" ? 0 : 1;
    if (N.cols[ci].length) { N.col = ci; markNeeds(); }
    return;
  }
  const rows = N.cols[N.col];
  const m = rows.length ? moveIn(N.cur[N.col], rows.length, k) : null;
  if (m != null) { e.preventDefault(); return setCur(N.col, m); }
  if (k === "A") return ui.cmd("analyze");
  if (k === "F") { const b = $("#screen .page-acts .primary"); if (b) { e.preventDefault(); b.click(); } return; }
  if (k === "l") return ui.go("log");
  const it = rows[N.cur[N.col]];
  const name = k === " " ? "space" : k === "Enter" ? "enter" : k;
  const act = it && !it.done && (it.actions || []).find((a) => a.key === name);
  if (act) { e.preventDefault(); return runAction(it, act, true); }
  if (k === "u" && undoBatch()) { e.preventDefault(); undoDone(); }
}

export function analyzeEvent(ev) {
  if (S.screen === "needs" && ev.done) needs(true);
}

// ---- 游戏日志 (06 §2.10) and Mod 库 › 变化 (§2.9) ----
// tab: 0 the Player.log clusters, 1 the companion log's entries (mcur, open: their cursor
// and the entries whose details are open).
const L = { env: null, path: "", page: null, loading: false, pending: "", error: "", cur: 0, target: "", targetLine: "", detail: new Map(), seq: 0, els: [], tab: 0, tabPick: null, mcur: 0, open: new Set(), want: "", list: null, views: [], view: null, viewPath: "" };

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

// Called by every board reload, including home-page applies and external switches.
// A shared file path can contain different attribution in a different environment.
function syncLogEnvironment() {
  const env = S.board?.env || "";
  if (L.env === env) return false;
  L.env = env;
  L.seq++;
  L.list?.destroy(); L.list = null;
  clearTimeout(detailTimer);
  L.path = ""; L.page = null; L.loading = false; L.pending = ""; L.error = "";
  L.cur = 0; L.mcur = 0; L.tab = 0; L.want = ""; L.els = [];
  L.target = ""; L.targetLine = "";
  L.detail.clear(); L.open.clear();
  L.views = []; L.view = null; L.viewPath = "";
  return true;
}

export async function logEnvironmentChanged() {
  if (syncLogEnvironment() && S.screen === "log") await gameLog("", true);
}

// gameLog shows the page immediately; late responses cannot reopen it after navigation.
export async function gameLog(path, open = false) {
  const changed = syncLogEnvironment();
  path = changed ? "" : path ?? L.path;
  if (!open && S.screen !== "log") return;
  ++L.seq;
  L.error = "";
  L.pending = "";
  L.loading = !(L.page && L.page.path && (L.page.path === path || !path));
  const read = beginPageRead("log", { replace: L.loading, label: "reading the log…", retry: () => gameLog(path, true) });
  const current = read.current;
  let p;
  try { p = await api.LogPage(path || ""); } catch (e) {
    if (current()) { L.loading = false; L.error = errText(e); read.fail(e); }
    return;
  }
  if (!current()) return;
  read.finish();
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
  if (!L.page || L.page.path !== p.path) {
    // a newly shown run opens on Archo Monitor's log when it has one (its errors say which
    // file or def was loading); a tab picked in the file menu wins
    L.cur = 0;
    L.tab = L.tabPick ?? (isArcho(p.monitor) ? 1 : 0);
  }
  L.tabPick = null;
  L.page = p;
  L.path = p.path;
  L.loading = false;
  if (L.target || L.targetLine) {
    // the running game's items carry the cluster's first line: their ids are not the page's
    const rows = p.rows || [];
    let i = L.target ? rows.findIndex(r => r.cluster === L.target) : -1;
    if (i < 0 && L.targetLine) i = rows.findIndex(r => r.line === L.targetLine);
    if (i >= 0) L.cur = i;
    L.tab = 0;
    L.target = ""; L.targetLine = "";
    drawGameLog();
    markLog();
    return;
  }
  drawGameLog();
}

function drawGameLog() {
  if (L.list) {
    L.views[L.view] = L.list.snapshot();
    L.list.destroy(); L.list = null;
  }
  if (L.viewPath !== L.page?.path) {
    L.views = []; L.open.clear(); L.mcur = 0;
    L.viewPath = L.page?.path || "";
  }
  L.view = L.tab;
  clearTimeout(detailTimer); L.want = "";
  const p = L.page;
  const right = [];
  if (p && p.path && !L.loading) {
    if (p.selected) right.push(el("span", "muted", T("its errors are in needs attention")));
    // the game still writes it: listed as this run, as the last run once the game exits
    else if (p.live) right.push(el("span", "muted", T("the game is writing it: its errors join needs attention when it exits")));
    else right.push(button(T("add its errors to needs attention"), "", async () => {
      try { say("ok", await api.SelectLog(p.path)); } catch (e) { say("fail", errText(e)); }
      await ui.reload();
      gameLog(p.path);
    }));
  }
  const w = logPage("log", "", ...right);
  const files = (p && p.files) || [];
  const bar = el("div", "subbar");
  if (files.length) bar.append(filePicker(files));
  if (p && p.facts && !L.loading) bar.append(el("span", "muted word-facts", p.facts));
  w.append(bar);
  if (L.error) { w.append(el("div", "card empty-state err", L.error)); return; }
  if (p && p.noMonitor && !L.loading) w.append(noMonitor(p));
  if (L.pending) { w.append(el("div", "card empty-state", Tf("the mods are analysed for %s first (once per game version)…", L.pending))); return; }
  if (L.loading) { const c = el("div", "card pad"); c.append(shimmer(T("reading the log…"), 6)); w.append(c); return; }
  if (!p || !p.path) { w.append(el("div", "card empty-state", T("no Player.log found"))); return; }
  const mon = p.monitor;
  if (!mon) L.tab = 0;
  L.view = L.tab;
  const grid = el("div", "cols two logcols");
  const list = el("section", "card col focus");
  if (mon) {
    const tabs = el("div", "ltabs");
    tabs.append(seg([Tf("Player.log  %d", (p.rows || []).length), mon.title + "  " + mon.entries.length], L.tab, (i) => { L.tab = i; drawGameLog(); }, ["←", "→"]));
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
      row.onclick = () => { L.cur = i; markLog(true); };
      return phaseRow(row, r.phaseStart);
    }, L.views[0]);
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
  ana.firstChild.append(el("span", "anum", "3"), archoMark(), T("Archo log analysis"));
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
  scroll.scrollTop = L.views[L.tab]?.scrollTop || 0;
  markLog(true);
  ui.renderHints();
}

// logHints: the game log's keys for the hint line (the tab decides what enter does).
export function logHints() {
  const out = [{ key: "↑↓", label: T("move") }];
  if (L.page?.monitor) out.push({ key: "←→", label: T("section") });
  if (L.tab === 1 && L.page?.monitor) out.push({ key: "enter", label: T("expand / close") });
  out.push({ key: "2", label: T("the log text") }, { key: "ctrl+k", label: T("commands") }, { key: "?", label: T("all keys") });
  return out;
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
  const shown = (L.tab === 1 && files.find((f) => f.of === L.path)) || files.find((f) => f.path === L.path) || files[0];
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
        L.path = f.of || f.path; L.page = null; L.cur = 0; L.mcur = 0; L.tabPick = L.tab;
        gameLog(L.path);
      },
    })));
  };
  return b;
}

const isArcho = (mon) => !!mon && /^ArchoMonitor/i.test(mon.label || "");

// noMonitor: one line when the current plan has no Archo Monitor, with the default edit.
function noMonitor(p) {
  const n = el("div", "lognote");
  n.append(el("span", "", p.noMonitor));
  if (p.addMonitor) n.append(button(T("add Archo Monitor"), "", async () => {
    try { say("ok", await api.AddMonitor(), true); } catch (e) { say("fail", errText(e)); }
    await ui.reload();
    gameLog(L.path);
  }, "small", "add-monitor"));
  return n;
}

// monitorRows: the companion log of the run — time, level, kind, ×count, related mods and
// files; selection shows its evidence, disclosure opens inline details.
function monitorRows(scroll, mon) {
  const head = el("div", "mhead");
  if (mon.running) head.append(el("span", "live-dot"));
  head.append(el("span", "word-facts", mon.facts));
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
    if (m.mods && m.mods.length) { const s = el("span", "mods word-facts", m.mods.join("  ")); s.title = m.mods.join("\n"); top.append(s); }
    body.append(top, logText(m));
    if (m.files && m.files.length) body.append(factText(el("div", "files"), m.files.map((text, i) => ({ text, paths: [{ text, path: m.filePaths?.[i] || text }] }))));
    const extra = el("div", "monitor-details");
    extra.hidden = !open;
    if (canOpenMonitor(m)) {
      if (m.details) { const pre = el("pre", "details"); pathContent(pre, m.details); extra.append(pre); }
      if (m.cluster) {
        const go = el("button", "go", Tf("%s in Player.log", m.cluster) + " →");
        go.onclick = (e) => { e.stopPropagation(); toCluster(m.cluster); };
        extra.append(go);
      } else if (m.only) extra.append(el("div", "files", Tf("only in %s: the game had stopped writing Player.log", mon.label)));
    }
    body.append(extra);
    const disclosure = el(canOpenMonitor(m) ? "button" : "span", "log-disclosure");
    if (canOpenMonitor(m)) {
      const chev = el("span", "fold-chev");
      chev.innerHTML = icon.chev;
      disclosure.append(chev);
      disclosure.title = T(open ? "hide details" : "show details");
      disclosure.setAttribute("aria-label", disclosure.title);
      disclosure.setAttribute("aria-expanded", String(open));
      disclosure.onclick = (e) => { e.stopPropagation(); toggleMonitor(i); };
    }
    row.append(disclosure, ic, body);
    row.onclick = () => { L.mcur = i; markLog(true); };
    return phaseRow(row, m.phaseStart);
  }, L.views[1]);
}

// The separator shares its data row's measured wrapper, so it has no cursor index
// or count of its own and participates in the existing scroll anchoring.
function phaseRow(row, label) {
  if (!label) return row;
  const wrap = el("div", "log-phase-row");
  wrap.append(el("div", "log-phase", label), row);
  return wrap;
}

const canOpenMonitor = (m) => !!(m.details || m.cluster || m.only);
function toggleMonitor(i) {
  const m = L.page?.monitor?.entries[i];
  if (!m || !canOpenMonitor(m)) return;
  L.mcur = i;
  const open = !L.open.has(m.key);
  if (open) L.open.add(m.key); else L.open.delete(m.key);
  L.list?.update(i, (row) => {
    if (row.classList.contains("log-phase-row")) row = row.querySelector(".lrow");
    row.classList.toggle("open", open);
    row.querySelector(".monitor-details").hidden = !open;
    const b = row.querySelector(".log-disclosure");
    b.title = T(open ? "hide details" : "show details");
    b.setAttribute("aria-label", b.title);
    b.setAttribute("aria-expanded", String(open));
  });
  markLog(true);
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
  const path = L.page.path, epoch = S.screenSeq, seq = L.seq;
  let m;
  try { m = await api.LogMonitor(path); } catch { return; }
  if (!m || S.screen !== "log" || S.screenSeq !== epoch || L.seq !== seq || L.page?.path !== path) return;
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
  // a new run rewrites the same file with other clusters under the same ids
  const k = L.page.path + "|" + id + "|" + ((L.page.rows || []).find(r => r.cluster === id)?.line || "");
  if (L.detail.has(k)) { const d = L.detail.get(k); return showLines(box, d.lines, d.card); }
  box.innerHTML = "";
  box.append(shimmer(T("loading…"), 8));
  clearTimeout(detailTimer);
  L.want = k;
  const env = L.env, seq = L.seq;
  detailTimer = setTimeout(async () => {
    const path = L.page.path;
    let lines, card = null;
    try {
      [lines, card] = await Promise.all([api.LogCluster(path, id), api.LogPattern(path, id).catch(() => null)]);
    } catch (e) { lines = [errText(e)]; }
    if (L.env !== env || L.seq !== seq) return;
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
  pathContent(pre, (lines || []).join("\n"));
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
  applyLogSize(grid);
  const wire = (sp, name, axis, get, range) => {
    const set = (v) => { logSize[name] = v; applyLogSize(grid); };
    wireSeparator(sp, {
      axis, key: name === "left" ? LOG_LEFT : LOG_ANA, get, range, set, reset: () => set(0),
      restore: (v) => set(+v || 0), serialize: () => String(logSize[name]),
      enabled: () => !matchMedia("(max-width: 900px)").matches,
    });
  };
  wire(vs, "left", "x", () => list.getBoundingClientRect().width, () => [260, grid.clientWidth - 320]);
  wire(hs, "ana", "y", () => ana.getBoundingClientRect().height, () => [80, grid.clientHeight - 140]);
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
  const row = (k, v) => { if (!v) return; const r = el("div", "pr"); r.append(el("span", "k", k), el("span", "v word-facts", v)); more.append(r); };
  if (c.causes && c.causes.length) row(T("common causes"), c.causes.join(T("; ")));
  if (c.fixes && c.fixes.length) row(T("kinds of fix"), c.fixes.join("  "));
  if (more.childNodes.length) d.append(more);
  const facts = [c.seen, c.gone].filter(Boolean).join("  ");
  if (facts) d.append(el("div", "pm word-facts", facts));
  d.append(el("div", "pm note", T("from shared logs, not a conclusion about your list")));
  return d;
}

// ---- Mod 库存 › 订阅与更新 and 添加订阅 (06 §2.9): subscriptions.js ----
export { changes, changesHints, changesFiltered, changesPlan, openAdd, addHints } from "./subscriptions.js";

export function rescanButton(label) {
  const b = button(S.busy.scan ? T("scanning…") : label || T("rescan the mod folders"), "R", () => ui.cmd("rescan"), "", "rescan");
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
  const b = document.querySelector(".page .rescan, #planhead .rescan");
  if (b) b.replaceWith(rescanButton(b.dataset.label));
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
      if (k === "Enter" && it && (!e.target.closest?.("button, .path-copy") || e.target.closest?.(".log-disclosure"))) { e.preventDefault(); return toggleMonitor(L.mcur); }
      return;
    }
    const n = (L.page && L.page.rows && L.page.rows.length) || 0;
    const m = n ? moveIn(L.cur, n, k) : null;
    if (m != null) { e.preventDefault(); L.cur = m; return markLog(); }
    return;
  }
  return changesKey(e);
}

// ---- the full detail (06 §4.2): one page for the editor and Mod 库存 (fulldetail.js) ----
export { full, fullKey, fullSection } from "./fulldetail.js";

// ---- settings (06 §2.14) ----
// Every row is a value and the command that changes it (view.SettingsPage); the page runs
// only those commands (API.ApplySetting).

export async function settings() {
  const read = beginPageRead("settings");
  const taskVersion = settingTaskRead;
  let groups, bg, tasks;
  try { [groups, bg, tasks] = await Promise.all([api.Settings(), bgSection(read.current), api.Tasks()]); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  groups = groups || [];
  const sc = S.screen === "settings" ? $("#screen") : openScreen("settings");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("settings", "");
  sc.append(w);
  // Groups in view.SettingsPage's order (the TUI's too); 启动页背景 closes the card that
  // holds the accent colour.
  for (const g of groups) {
    renderGroups(w, [g], settings);
    if (g.items?.some(it => it.edit === "swatches")) w.lastElementChild.append(bg);
  }
  markSettingTasks(tasks);
  sc.scrollTop = top;
  read.finish();
  if (taskVersion !== settingTaskRead) refreshSettingTasks(true);
}

// Task events update the affected values without replacing edits or focus (06 §2.14).
let settingTaskRead = 0;
let settingValueRead = 0;
async function refreshSettingTasks(values = false) {
  const n = ++settingTaskRead, epoch = S.screenSeq;
  const v = values ? ++settingValueRead : 0;
  try {
    const [tasks, groups] = await Promise.all([api.Tasks(), values ? api.Settings() : null]);
    if (S.screen !== "settings" || S.screenSeq !== epoch) return;
    if (n === settingTaskRead) markSettingTasks(tasks);
    if (!groups || v !== settingValueRead) return;
    const rows = [...$("#screen").querySelectorAll("[data-setting-row]")];
    for (const g of groups) for (const it of g.items || []) {
      const row = rows.find(r => r.dataset.settingRow === g.title + "/" + it.label);
      if (!row) continue;
      const note = row.querySelector(".setting-note"), value = row.querySelector(".setting-value");
      if (note) { factText(note, { text: it.note || "", paths: it.notePaths }); note.hidden = !it.note; }
      if (value) factText(value, { text: it.value || "", paths: it.valuePath || it.edit === "url" ? [it.value] : [] });
      settingPathPicker(row, it, settings);
      const valueNote = row.querySelector(".setting-value-note");
      if (valueNote) { valueNote.textContent = it.valueNote || ""; valueNote.hidden = !it.valueNote; }
      const folded = row.nextElementSibling?.classList.contains("setting-details") ? row.nextElementSibling : null;
      if (JSON.stringify(it.details || null) !== (folded?.dataset.details || "null")) {
        const open = !!folded?.open;
        folded?.remove();
        if (it.details) { const d = settingDetails(it.details); d.open = open; row.after(d); }
      }
      const actions = JSON.stringify(it.actions || []);
      if (row.dataset.settingActions !== actions) {
        row.querySelectorAll("[data-setting-action]").forEach(b => b.remove());
        renderSettingActions(row.querySelector(".r"), it.actions, settingRunner(settings), it);
        row.dataset.settingActions = actions;
      }
    }
    if (n === settingTaskRead) markSettingTasks(tasks);
  } catch (e) { if (S.screen === "settings" && S.screenSeq === epoch) say("fail", errText(e)); }
}

function markSettingTasks(tasks) {
  const active = new Set((tasks?.rows || []).filter(t => t.state === "queue" || t.state === "run").map(t => t.kind));
  for (const b of $("#screen").querySelectorAll("button[data-setting-task]")) {
    b.disabled = active.has(b.dataset.settingTask);
    b.textContent = b.disabled ? b.dataset.pending : b.dataset.label;
  }
}

// Each startup checks the disk, including environments other than the selected one.
export async function checkDirectories() {
  let dirs;
  try { dirs = await api.Directories(); } catch (e) { say("fail", errText(e)); return; }
  const missing = (dirs || []).filter(d => d.status !== "ok" && d.status !== "unused");
  if (!missing.length) return;
  const body = el("div");
  body.append(el("p", "sub", T("directories are checked each time RimArcho starts; you can skip this and locate them in settings later")));
  for (const d of missing) {
    const row = el("div", "set");
    const left = el("div", "l");
    const choose = () => directoryPrompt(d, false, async () => { await ui.reload(); checkDirectories(); });
    left.append(el("div", "name", d.label), editablePath(copyablePath(el("div", "sub", d.path), d.path), choose));
    row.append(left, button(T("locate"), "", choose));
    body.append(row);
  }
  pop.panel(T("some directories need their new location"), body, T("skip for now"), () => {});
}

function directoryPrompt(d, moving, after) {
  const popup = S.popup;
  return chooseLocalPath(d.path, async selected => {
    if (!moving) {
      try {
        const lines = await api.ApplySetting(["folder", "set", d.id, "{}"], selected);
        if (S.popup === popup) pop.close();
        say("ok", lines?.[0] || T("done"));
        if (after) await after();
      } catch (e) { say("fail", errText(e)); }
      return;
    }
    // Moving chooses the parent folder, then keeps the original preview/confirmation.
    const path = selected.replace(/[\\/]$/, "") + "/" + d.path.replace(/[\\/]$/, "").split(/[\\/]/).pop();
    const screen = S.screen, epoch = S.screenSeq;
    try {
      const links = await api.DirectoryMovePreview(d.id, path);
      if (S.screen !== screen || S.screenSeq !== epoch) return;
      pop.confirm(d.label, [T("move this directory and its files to the new path?"), d.path + " → " + path, ...(links || [])], T("confirm move"), async () => {
        try {
          const lines = await api.ApplySetting(["folder", "move", d.id, "{}"], path);
          say("ok", lines?.[0] || T("done"));
          if (after) await after();
        } catch (e) { say("fail", errText(e)); }
      });
    } catch (e) { say("fail", errText(e)); }
  });
}

// Reattach picker actions when background task results replace address text.
function settingPathPicker(row, it, again) {
  if (it.edit !== "directory" && it.edit !== "path") return;
  const choose = it.edit === "directory"
    ? () => directoryPrompt({ id: it.directory, label: it.label, path: it.value }, false, async () => { await ui.reload(); again(); })
    : () => chooseLocalPath(it.editValue ?? it.value ?? "", value => settingRunner(again)(it.argv, value), { file: !!it.file });
  editablePath(row.querySelector(".setting-value"), choose, !!it.file);
  if (it.file) editablePath(row.querySelector(".setting-note"), choose, true);
}

// settingRunner runs a setting's command, then redraws the page with again.
function settingRunner(again) {
  return async (argv, value, after, background = false) => {
    const screen = S.screen, epoch = S.screenSeq;
    try {
      const out = argv[0] === "account" ? (argv[1] === "steamcmd" && argv[2] === "login" ? await api.SteamLogin(value) : await api.AccountAction(argv[1], argv[2])) : await api.ApplySetting(argv, value || "");
      say("ok", (out && out[0]) || T("done"));
    } catch (e) { if (S.screen === screen && S.screenSeq === epoch) say("fail", errText(e)); return; }
    if (S.screen !== screen || S.screenSeq !== epoch) return;
    if (background) return refreshSettingTasks(true);
    if (after) return after();
    await ui.reload();
    if (S.screen === screen && S.screenSeq === epoch) again();
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
    if (g.note) card.append(el("div", "group-note", g.note));
    for (const it of g.items || []) {
      const row = el("div", "set");
      row.dataset.settingRow = g.title + "/" + it.label;
      row.dataset.settingActions = JSON.stringify(it.actions || []);
      row.title = it.hover || "";
      const left = el("div", "l");
      left.append(el("div", "name", it.label));
      const note = factText(el("div", "sub setting-note"), { text: it.note || "", paths: it.notePaths });
      note.hidden = !it.note;
      left.append(note);
      const right = el("div", "r");
      if (it.edit === "swatches" || it.edit === "segments") {
        row.classList.add("appearance-row");
        right.append(appearanceControl(it, () => ui.reaccent()));
      } else if (it.edit === "choice") {
        const s = choiceControl(it.label, it.choices.map((c, i) => ({ value: c, label: it.choiceLabels?.[i] || c })), it.value);
        if (it.note) {
          note.id = s.id + "-note";
          s.setAttribute("aria-describedby", note.id);
        }
        // A new language: the page's text again, in place (main.js relang).
        s.onchange = () => run(it.argv, s.value,
          it.argv[2] === "ui.lang" ? () => ui.relang() : it.argv[2] === "ui.accent" ? () => ui.reaccent() : null);
        right.append(s);
      } else if (it.edit === "directory") {
        const value = factText(el("span", "muted setting-value"), { text: it.value, paths: [it.value] });
        const d = { id: it.directory, label: it.label, path: it.value };
        const after = async () => { await ui.reload(); again(); };
        right.append(value, button(T("locate"), "", () => directoryPrompt(d, false, after), "", "apply-setting"),
          button(T("move to…"), "", () => directoryPrompt(d, true, after), "", "apply-setting"));
      } else if (it.edit === "path") {
        right.append(factText(el("span", "muted setting-value"), { text: it.value || "", paths: it.valuePath ? [it.value] : [] }));
        const adding = it.argv[0] === "folder" && it.argv[1] === "add";
        right.append(button(T(adding ? "add…" : "change…"), "", () => chooseLocalPath(it.editValue ?? it.value ?? "",
          value => run(it.argv, value), { file: !!it.file }), "", "apply-setting"));
      } else if (it.edit === "url") {
        right.append(factText(el("span", "muted setting-value"), { text: it.value, paths: [it.value] }),
          button(T("change…"), "", () => urlPrompt(it.label, it.value, value => run(it.argv, value)), "", "apply-setting"));
      } else if (it.links?.length) {
        for (const link of it.links) right.append(button(link.label, "", () => api.OpenURL(link.url).catch(e => say("fail", errText(e))), "community-link"));
      } else if (it.edit) {
        const inp = el("input", "field");
        inp.value = it.editValue ?? it.value ?? "";
        inp.spellcheck = false;
        const ok = button(T("apply"), "", () => run(it.argv, inp.value.trim()), "", "apply-setting");
        inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); ok.click(); } };
        if (it.editValue !== undefined) right.append(factText(el("span", "muted setting-value"), { text: it.value || "", paths: it.valuePath ? [it.value] : [] }));
        right.append(inp);
        right.append(ok);
      } else if (it.url) {
        // 社区: a web page, opened in the browser (06 §2.14)
        const value = document.createElement("span"); value.textContent = it.value || it.url;
        const b = button("↗", "", () => api.OpenURL(it.url).catch(e => say("fail", errText(e))), "link");
        b.title = T("open the web page"); b.setAttribute("aria-label", T("open the web page"));
        right.append(copyablePath(value, it.url), b);
      } else {
        right.append(factText(el("span", "muted setting-value"), { text: it.value || "", paths: it.valuePath ? [it.value] : [] }));
      }
      const valueNote = el("span", "muted word-facts setting-value-note", it.valueNote || "");
      valueNote.hidden = !it.valueNote;
      right.append(valueNote);
      renderSettingActions(right, it.actions, run, it);
      row.append(left, right);
      settingPathPicker(row, it, again);
      card.append(row);
      if (it.details) card.append(settingDetails(it.details));
    }
    w.append(card);
  }
}

// settingDetails folds text under its row (06 §3.1 rule 7): RimArcho's release notes.
function settingDetails(d) {
  const box = el("details", "setting-details");
  box.dataset.details = JSON.stringify(d);
  box.append(el("summary", "", d.summary));
  if (d.text) box.append(el("pre", "sub", d.text.replace(/\r\n/g, "\n")));
  if (d.link) {
    const row = el("div", "setting-release-link");
    const a = copyablePath(el("span", "sub", d.link), d.link);
    const open = button("↗", "", () => api.OpenURL(d.link).catch(e => say("fail", errText(e))), "link");
    open.title = T("open the web page"); open.setAttribute("aria-label", open.title);
    row.append(a, open); box.append(row);
  }
  return box;
}

function renderSettingActions(right, actions, run, item) {
  for (const a of actions || []) {
    // 卸载 RimArcho and 反馈 are their own pages; every other action is a command
    if (a.argv[0] === "uninstall" || a.argv[0] === "feedback") {
      const b = button(a.label, "", () => ui.go(a.argv[0]));
      b.dataset.settingAction = a.argv.join(" ");
      right.append(b); continue;
    }
    const submit = (value = "") => {
      if (a.argv[0] === "account") {
        b.disabled = true; b.textContent = a.label + "…"; say("busy", a.label + "…");
        return run(a.argv, value).finally(() => { b.disabled = false; b.textContent = a.label; });
      }
      if (a.task) { b.disabled = true; b.textContent = a.pending; }
      return run(a.argv, "", null, !!a.task);
    };
    const b = button(a.label, "", async () => {
      if (a.input !== undefined) {
        if (right.querySelector(".account-login")) { right.querySelector(".account-login input").focus(); return; }
        const form = el("form", "account-login subscribe-row"), input = el("input", "field small");
        input.value = a.input; input.spellcheck = false; input.autocomplete = "username";
        input.setAttribute("aria-label", T("Steam account name"));
        input.placeholder = T("Steam account name");
        const hint = el("span", "muted", T("enter to log in, esc to cancel"));
        const close = () => { form.remove(); b.hidden = false; b.focus(); };
        form.append(input, hint); b.hidden = true; right.append(form); input.focus(); input.select();
        form.onsubmit = e => {
          e.preventDefault(); e.stopPropagation();
          const name = input.value.trim();
          if (!name || /^(anonymous|匿名)$/i.test(name)) { say("fail", T("enter a Steam account name first")); input.focus(); return; }
          close(); submit(name);
        };
        form.onkeydown = e => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); } else if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); form.requestSubmit(); } };
        return;
      }
      if (a.argv[0] === "deps" && a.argv[1] === "install") {
        b.disabled = true;
        try {
          const offer = await api.ToolInstallOffer(a.argv[2]);
          pop.confirm(a.label, [a.confirm, Tf("source: %s", offer.url), Tf("target folder: %s", offer.dir),
            offer.bytes > 0 ? Tf("size: %s bytes", offer.bytes) : T("size unknown")], a.label, () => submit());
        } catch (e) { say("fail", errText(e)); }
        finally { b.disabled = false; }
        return;
      }
      if (a.confirm) pop.confirm(a.label, [a.confirm], a.label, () => submit()); else submit();
    }, "", a.argv.join(" "));
    b.dataset.settingAction = a.argv.join(" ");
    if (a.task) {
      b.dataset.settingTask = a.task;
      b.dataset.label = a.label;
      b.dataset.pending = a.pending;
    }
    right.append(b);
  }
}

// Native selection is the only GUI local-path editor. Ignore cancelled or late results.
let choosingPath = false;
export async function chooseLocalPath(value, done, { file = false } = {}) {
  if (choosingPath) return;
  choosingPath = true;
  const screen = S.screen, epoch = S.screenSeq;
  let path;
  try { path = file ? await api.PickProgram() : await api.PickFolder(value || ""); }
  catch (e) { if (S.screen === screen && S.screenSeq === epoch) say("fail", errText(e)); }
  finally { choosingPath = false; }
  if (path && S.screen === screen && S.screenSeq === epoch) return done(path);
}

export function urlPrompt(title, value, done) {
  const focused = document.activeElement;
  pop.open("url", title, (box, p) => {
    box.classList.add("url-editor");
    box.setAttribute("role", "dialog"); box.setAttribute("aria-modal", "true"); box.setAttribute("aria-label", title);
    box.append(el("div", "ttl", title));
    const input = el("input", "field");
    input.value = value; input.type = "url"; input.spellcheck = false;
    input.setAttribute("aria-label", title);
    const error = el("div", "err"); error.setAttribute("role", "alert");
    const apply = () => {
      const next = input.value.trim();
      try {
        const url = new URL(next);
        if (!/^https?:\/\//i.test(next) || !["http:", "https:"].includes(url.protocol) || !url.hostname || /\s/.test(next)) throw new Error();
      } catch {
        error.textContent = T("enter a complete HTTP or HTTPS URL");
        input.setAttribute("aria-invalid", "true"); input.focus(); return;
      }
      pop.close(); done(next);
    };
    input.oninput = () => { error.textContent = ""; input.removeAttribute("aria-invalid"); };
    const body = el("div", "body"); body.append(input, error);
    const actions = el("div", "btns");
    actions.append(button(T("cancel"), "", () => pop.close()), button(T("OK"), "", apply, "primary", "apply-setting"));
    box.append(body, actions);
    p.keys = e => {
      if (e.key === "Enter" && e.target === input) { e.preventDefault(); apply(); }
      return true;
    };
    p.onclose = () => focused?.isConnected && focused.focus({ preventScroll: true });
    requestAnimationFrame(() => { if (S.popup === p) input.focus(); });
  });
}

export async function pickFolder(start, then) {
  return chooseLocalPath(start, then);
}

// ---- the environment (the pill on the left of the top bar) ----
// Everything here belongs to one game folder (02 §0): which one is in use, the folder, its
// version, how it is started and its saves. The mod folders every game shares are on the
// settings page.

export async function environment() {
  const read = beginPageRead("env");
  let p;
  try { p = await api.Environment(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  const sc = S.screen === "env" ? $("#screen") : openScreen("env");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("env", "", act(T("add a game"), "plus", () => ui.go("guide")), rescanButton());
  sc.append(w);
  const envLab = el("div", "label section", T("every game"));
  w.append(envLab);
  const card = el("section", "card");
  area(card, envLab);
  for (const e of p.envs || []) {
    const row = el("div", "envrow" + (e.current ? " on" : ""));
    const l = el("div", "l");
    l.append(el("div", "name", e.name), factText(el("div", "sub"), { text: e.game, paths: [e.path] }));
    row.append(l);
    if (e.current) {
      const current = el("button", "chip cur", T("in use"));
      current.onclick = () => { if (S.busy.env && S.busy.env !== e.name) ui.useEnv(e.name); };
      row.append(current);
    } else {
      row.append(button(T("use this one"), "", () => ui.useEnv(e.name)));
    }
    row.append(button(T("remove the game"), "", () => removeEnv(e.name), "quiet danger", "remove-env"));
    card.append(row);
  }
  w.append(card);
  renderGroups(w, p.groups || [], environment);
  sc.scrollTop = top;
  read.finish();
}

// removeEnv: the dry run says what RimArcho forgets and what stays, one confirmation removes
// it (06 §2.14). Another environment takes over when it was in use; with none left, the
// guide opens.
export async function removeEnv(name) {
  let p;
  try { p = await api.EnvRemovePreview(name); } catch (e) { say("fail", errText(e)); return; }
  pop.confirm(p.title, p.lines, T("remove the game"), async () => {
    try { say("ok", await api.RemoveEnv(name)); } catch (e) { say("fail", errText(e)); return; }
    for (const c of S.cols) { c.query = ""; c.cursor = 0; c.sel.clear(); }
    await ui.reload();
    if (!(S.board && S.board.envs && S.board.envs.length)) return ui.go("guide");
    if (S.screen === "env") environment();
  });
}
