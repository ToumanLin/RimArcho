// RimArcho GUI (08). The page only lays out and maps input: what it shows comes from
// internal/view through the Go service, and the 06 §3 rules hold here too:
//   1. the arrows only move (the cursor, the focus, the scroll), never change data;
//   2. verbs act at once on the object (the selection, else the cursor's row), u undoes;
//   3. the position line always says where you are;
//   4. the hint line lists what you can do with the object now;
//   5. few popups, each closed with esc, never nested.
import { api, on, setText } from "./api.js";
import "./window-theme.js";
import "./refnums.js";
import "./fulltitle.js";
import { applyAccent } from "./accent.js";
import { factText, S, $, $$, el, esc, T, Tf, errText, say, clearSay, undoStatus, col, cur, selection, key, idsOf, refsOf, named, AREAS } from "./state.js";
import { openEditor, setItems, refilter, renderLists, renderHeads, renderRows, ensureVisible, pageDelta, wireLists, setOnHistory, stagingOpen, setStagingOpen, showGhost, acceptGhost, barOn } from "./lists.js";
import { createBar } from "./filterbar.js";
import { showDetail, scrollDetail } from "./detail.js";
import * as pop from "./popups.js";
import * as scr from "./screens.js";
import * as shell from "./shell.js";
import * as homePage from "./home.js";
import * as fullPage from "./fulldetail.js";
import * as guide from "./guide.js";
import * as plans from "./plans.js";
import * as library from "./library.js";
import { updateFromForm } from "./subscribe.js";
import { initExchange, exchange, key as importKey, hintList as importHints } from "./exchange.js";
import { icon } from "./icons.js";
import * as motion from "./motion.js";
import * as ops from "./ops.js";
import * as uninst from "./uninstall.js";
import * as packsPage from "./packs.js";
import * as planManage from "./planmanage.js";
import * as tex from "./textures.js";
import * as tasksPage from "./tasks.js";
import { refreshBackgrounds } from "./bg.js";
import { environmentReason, withEnvironmentSwitch } from "./environment.js";
import { wireSeparator } from "./separator.js";
import { go, openPage, initNavigation, pageEntering, pageEntered, wireHistory } from "./navigation.js";
import { checkCredit, getChoices } from "./credit.js";

// ---- loading ----

let reloadSeq = 0;
// retryRead reads the workspace again after a failed read: 1, 2, 4, 8 s, then every 10 s
// for a minute; a successful read resets it.
let readRetries = 0, readRetryTimer = null;
function retryRead() {
  if (readRetryTimer || readRetries >= 10) return;
  const wait = Math.min(1000 * 2 ** readRetries, 10000);
  readRetries++;
  readRetryTimer = setTimeout(() => { readRetryTimer = null; reload(); }, wait);
}
async function reload(snapshot = null, read = null) {
  if (S.busy.env && !snapshot && !read) return;
  // edits on their way to Go: the lists here are ahead of Go's; the last edit reads them
  if (editsPending && !snapshot && !read) { reloadWanted = true; return; }
  if (!read && !S.screen) return openEditor((ticket) => reload(snapshot, ticket), false, () => { render(); detailFollow(true); });
  const seq = ++reloadSeq, gameRequest = S.envSeq, epoch = editEpoch;
  // an edit made while this read was on its way is newer than what it brings
  const current = () => seq === reloadSeq && gameRequest === S.envSeq && (read ? read.current() : epoch === editEpoch);
  let b, sh, history;
  try {
    const result = snapshot || await api.Workspace();
    if (!current() || result.stale) return;
    b = result.board; sh = result.shell; history = result.history;
  } catch (e) { if (read) throw e; if (current()) { say("fail", errText(e)); retryRead(); } return; }
  if (!current()) return;
  // A board that could not be read (a background job holding the data) has an error and no
  // columns: it must not replace the lists on screen, the sidebar's lists or the games.
  if (b && b.error && S.board && !S.board.error) {
    say("fail", b.error + "  " + T("trying again…"));
    retryRead();
    if (read) read.finish();
    return;
  }
  readRetries = 0;
  const apply = () => { S.board = b; S.shell = sh; S.history = history; };
  if (read) read.commit(apply); else apply();
  S.busy.scan = b.scanning;
  S.busy.analyze = b.analyzing;
  if (b.error && b.envs && b.envs.length) say("fail", b.error);
  const cols = b.cols || [[], [], []];
  const keep = [0, 1, 2].map((k) => setItems(k, cols[k] || []));
  await Promise.all([0, 1, 2].map((k) => refilter(k, keep[k])));
  if (!current()) return;
  if (!read) { render(); detailFollow(); }
  await scr.logEnvironmentChanged();
  return true;
}

let detailKey = "";
function detailFollow(force) {
  const k = Math.min(S.focus, 2) === S.focus ? S.focus : lastList;
  const it = cur(k);
  const id = it ? key(it) + "|" + (it.pos || 0) + "|" + S.board.stamp : "";
  if (!force && id === detailKey) return;
  detailKey = id;
  showDetail(it, k === 2);
}

// relang: the language changed (06 §2.14). The text comes again from Go (the page data and
// the T() table); the same page opens again in the new language, with its scroll, the
// lists' cursors and selections and the open details kept: no reload, no jump home.
async function relang() {
  const s = await api.Init();
  archoTheme.apply(s.theme);
  applyAccent(s.accent);
  S.start = s;
  setText(s.text);
  document.documentElement.lang = s.lang;
  const screen = S.screen;
  const top = $("#screen").scrollTop;
  await reload();
  detailFollow(true);
  if (screen === "full") { await step(0); await scr.full(); }
  else if (screen === "library") await library.relang();
  else if (screen && screen !== "guide") await openPage(screen);
  if (screen) $("#screen").scrollTop = top;
  render();
}

async function reaccent() {
  const s = await api.Init();
  S.start.accent = s.accent;
  S.start.theme = s.theme;
  archoTheme.apply(s.theme);
  applyAccent(s.accent);
}

// ---- drawing ----

function render() {
  renderTop();
  renderLists();
  renderWhere();
  renderHints();
}

// renderTop: the plan editor's description line, like every page's (06 §2.2, §2.5): how many
// mods the plan enables; its actions sit at the right end of the line (renderWhere).
function renderTop() {
  if (!S.board) return;
  $("#planhead .lead").textContent = Tf("%d enabled, in the order the game loads them", col(1).items.length);
}

// goLaunch is the editor's 启动游戏 (and its L): the game starts from 启动, so it opens that
// page with this list applied — launching it from its editor makes it current — and the
// focus on its launch button (06 §2.5).
// exportCurrent is the editor's e: the current list's export sub-page (06 §2.5, §2.17).
function exportCurrent() {
  const p = S.shell && S.shell.plan;
  if (!p) return go("plans");
  return planManage.exportOf(p.id, "editor");
}

async function goLaunch() {
  if (environmentReason()) return say("busy", environmentReason());
  const open = S.shell && S.shell.plan;
  if (open && !open.current && !(await plans.usePlan(open.id))) return;
  await go("home");
  await homePage.focusLaunch();
}

// planActions: the list's actions in the top bar (06 §2.5): its "有未载入的外部版本" mark,
// how many entries are not saved to the game yet, rename and delete, import / export (the
// list's page in 全部列表), save, and set current or launch. Undo sits beside sort in the
// enabled list's head.
function planActions() {
  const u = (S.board && S.board.unsaved) || 0;
  const btn = (label, k, run, cls = "", envWrite = "") => {
    const b = el("button", "btn " + cls, label);
    if (envWrite) b.dataset.envWrite = envWrite;
    if (k) { b.append(el("kbd", "", k)); b.title = k; }
    b.onclick = (e) => { e.stopPropagation(); run(); };
    return b;
  };
  const out = [];
  const plan = S.shell && S.shell.plan;
  if (plan && plan.conflict) {
    const m = el("button", "chip mark");
    m.append(el("span", "dot"), el("span", "t", plan.conflictText));
    m.title = plan.conflictText + T("; ") + T("can it run › needs your answer");
    m.onclick = (e) => { e.stopPropagation(); go("needs"); };
    out.push(m);
  }
  if (u) {
    const n = el("span", "unwritten", Tf("%d unsaved", u));
    n.title = T("different from the last saved list; launching or saving saves these changes to the game");
    motion.bump("unwritten", n, u);
    out.push(n);
  } else motion.bump("unwritten", null, 0);
  out.push(scr.rescanButton(T("rescan")));
  if (plan) {
    out.push(btn(T("rename"), "", () => plans.renamePlan(plan.id), "", "rename-plan"),
      btn(T("delete"), "", () => plans.deletePlan(plan.id), "danger", "remove-plan"));
  }
  const io = btn(T("import/export"), "", manageCurrent);
  io.title = T("the list's page in all lists: import, export, share");
  const save = btn(u ? T("save changes") : T("no changes"), "w", () => run("save"), u ? "hot" : "", "save");
  save.disabled = !u;
  out.push(io, save);
  // another list is the one launched (04 §1.3): this one can be made current first
  if (plan && S.shell.current) {
    const c = btn(T("set current"), "", () => plans.usePlan(plan.id), "primary", "use-plan");
    c.title = Tf("the game is launched with %s; this list is only open here", S.shell.current.name) + T("; ") + T("no game file is written");
    out.push(c);
  } else {
    // 启动 opens 启动, where the game starts (06 §2.5)
    const l = btn(T("launch"), "L", goLaunch, "primary", "launch");
    l.title = T("opens the launch page, where the game starts") + T("; ") + "L";
    out.push(l);
  }
  return out;
}

// manageCurrent opens 全部列表 with the open list selected: its import, export and sharing.
function manageCurrent() {
  const p = S.shell && S.shell.plan;
  if (p) planManage.select(p.id);
  return go("plans");
}

let lastList = 1;

// The sidebar's entries in shift+1–8 order, 在玩 then 库 (06 §3.5; view.Nav).
// "plan" is 列表: like a click on it, the current list (06 §2.1).
const NAV = ["home", "needs", "log", "library", "plan", "packs", "textures", "env"];
const navTarget = (id) => id !== "plan" ? id : ((S.shell && S.shell.plans) || []).find((p) => p.current)?.page || "plans";

// renderWhere: the position line (06 §3.2), the sidebar and the breadcrumb.
function renderWhere() {
  shell.sync();
  $("#planhead").hidden = !!S.screen;
  if (!S.screen) shell.setActs("plan|" + ((S.board && S.board.unsaved) || 0) + "|" + !!S.busy.scan + "|" + T("undo") + "|" + JSON.stringify(S.shell && S.shell.plan) + "|" + JSON.stringify(S.shell && S.shell.current), ...planActions());
  // A page's place is its breadcrumb; the position line says more only on the plan (where
  // the cursor is in which list) and in the full detail.
  let s = "";
  if (S.screen === "library") s = library.where(); // a table like the plan's lists
  else if (S.screen === "tasks") s = tasksPage.where();
  else if (S.screen && S.screen !== "full") s = ""; // a page: the breadcrumb is enough
  else if (S.screen === "full") {
    s = fullPage.fullName();
    const sec = scr.fullSection();
    if (sec) s += " › " + sec;
  } else if (S.focus === 3) {
    const sc = $(".detail .scroll");
    const pct = sc.scrollHeight > sc.clientHeight ? Math.round(100 * sc.scrollTop / (sc.scrollHeight - sc.clientHeight)) : 100;
    s = `${T("details")}    ${S.detail ? S.detail.name : "—"}    ${pct}%`;
  } else {
    const c = col();
    s = `${T(AREAS[S.focus])}    ${c.view.length ? c.cursor + 1 : 0} / ${c.view.length}`;
    if (c.sel.size) s += "  " + Tf("%d selected", c.sel.size);
  }
  if (S.popup) s += " › " + S.popup.title;
  factText($("#where"), s);
}

// actions: what can be done with the object now (the hint line and the context menu).
function actions() {
  if (S.focus > 2) return [
    { key: "PgDn", label: T("scroll") }, { key: "enter", label: T("full details"), run: () => verb("full") },
    { key: "ctrl+o", label: T("open the folder"), run: () => verb("folder") }, { key: "ctrl+w", label: T("Workshop page"), run: () => verb("workshop-id") },
    { key: "ctrl+s", label: T("steam page"), run: () => verb("steam") },
    { key: "g", label: T("tags"), run: () => verb("tags") },
  ];
  const sel = selection();
  const k = S.focus;
  if (!sel.length) return [{ key: "/", label: T("search"), run: () => startSearch() }, { key: "ctrl+k", label: T("commands"), run: () => pop.palette() }];
  const out = [];
  if (k === 0) out.push({ key: "space", label: T("enable"), run: () => verb("toggle") });
  if (k === 1) out.push({ key: "space", label: T("disable"), run: () => verb("toggle") });
  if (k === 2) out.push({ key: "space", label: T("put back"), run: () => verb("toggle") });
  out.push({ key: "t", label: k === 2 ? T("put back") : T("stage"), run: () => verb("stage") });
  out.push({ key: "m", label: k === 1 ? T("move to…") : T("enable at…"), run: () => verb("move") });
  // ctrl+o / ctrl+w: the folder and the Workshop page of the cursor's mod (06 §3.5)
  if (sel.length === 1 && sel[0].id) out.push({ key: "ctrl+o", label: T("open the folder"), run: () => verb("folder") }, { key: "ctrl+w", label: T("Workshop page"), run: () => verb("workshop-id") });
  if (sel.length === 1 && sel[0].id) out.push({ key: "ctrl+s", label: T("steam page"), run: () => verb("steam") });
  if (k === 1) out.push({ key: "J K", label: T("down / up"), run: null });
  out.push({ key: "g", label: T("tags"), run: () => verb("tags") });
  if (sel.length === 1) {
    out.push({ key: "enter", label: T("full details"), run: () => verb("full") });
    if (k === 1 && sel[0].id) out.push({ key: "x", label: T("switch version…"), run: () => verb("swap") });
  }
  return out;
}

function renderHints() {
  const h = $("#hints");
  h.innerHTML = "";
  let list = actions();
  if (S.screen === "full") list = fullPage.fullHints();
  else if (S.screen === "library") list = library.hintList();
  else if (S.screen === "tasks") list = tasksPage.hintList();
  else if (S.screen === "changes") list = scr.changesHints();
  else if (S.screen === "subscribe") list = scr.addHints();
  else if (S.screen === "home") list = homePage.hintList();
  else if (S.screen === "packs" || S.screen === "build") list = packsPage.hintList();
  else if (S.screen === "plan-import") list = importHints();
  else if (S.screen === "plans" || S.screen === "plan-export") list = planManage.hintList();
  else if (S.screen) list = [{ key: "shift+1–8", label: T("pages") }, { key: "ctrl+k", label: T("commands") }, { key: "?", label: T("all keys") }];
  if (scr.changesFiltered()) list = [{ key: "esc", label: T("every mod") }, ...list];
  list = foldShortcuts(list);
  const c = S.focus < 3 ? col() : null;
  if (c && c.sel.size && !S.screen) h.append(el("span", "accent", Tf("%d selected", c.sel.size)));
  list.slice(0, 7).forEach((a) => {
    const group = el("span", "hint-group");
    group.append(el("b", "", a.key), " " + a.label);
    h.append(group);
  });
  if (c && c.sel.size && !S.screen) {
    const group = el("span", "hint-group");
    group.append(el("b", "", "esc"), " " + T("clear the selection"));
    h.append(group);
  }
}

// foldShortcuts: ctrl+o and ctrl+w share one place in the hint line.
export function foldShortcuts(list) {
  const i = list.findIndex((a) => a.key === "ctrl+o");
  if (i < 0 || !list.some((a) => a.key === "ctrl+w")) return list;
  const out = list.filter((a) => a.key !== "ctrl+w");
  out[i] = { key: "ctrl+o ctrl+w", label: T("folder / Workshop page") };
  return out;
}

// ---- moving (navigation only: rule 1) ----

function focusArea(k) {
  if(k===3 && !$('.area[data-area="3"]').getClientRects().length)return verb("full");
  if (k === S.focus) return;
  if (S.focus < 3 && k < 3 && col().sel.size) {
    col().sel.clear(); // switching to another list clears the selection (06 §3.4)
  }
  S.focus = k;
  if (k < 3) lastList = k;
  S.anchor = -1;
  // 暂存 is folded under 详情: going to it opens it
  if (k === 2 && !stagingOpen()) setStagingOpen(true);
  renderLists();
  renderWhere();
  renderHints();
  if (k < 3) { ensureVisible(k); detailFollow(); }
}

function moveCursor(d, extend) {
  const c = col();
  if (!c.view.length) return;
  const from = c.cursor;
  c.cursor = Math.max(0, Math.min(c.view.length - 1, c.cursor + d));
  if (extend) {
    if (S.anchor < 0) S.anchor = from;
    c.sel.clear();
    const [a, b] = [Math.min(S.anchor, c.cursor), Math.max(S.anchor, c.cursor)];
    for (let i = a; i <= b; i++) c.sel.add(key(c.items[c.view[i]]));
  } else S.anchor = -1;
  ensureVisible();
  renderRows(S.focus);
  renderHeads();
  renderWhere();
  renderHints();
  detailFollow();
}

// step moves the cursor of the last list by d (the full detail's ←→).
async function step(d) {
  const c = col(lastList);
  c.cursor = Math.max(0, Math.min(c.view.length - 1, c.cursor + d));
  ensureVisible(lastList);
  renderRows(lastList);
  detailKey = "";
  await showDetail(cur(lastList), lastList === 2);
}

function click(k, i, e) {
  const c = S.cols[k];
  if (S.focus !== k) focusArea(k);
  if (e.shiftKey) {
    if (S.anchor < 0) S.anchor = c.cursor;
    c.sel.clear();
    const [a, b] = [Math.min(S.anchor, i), Math.max(S.anchor, i)];
    for (let j = a; j <= b; j++) c.sel.add(key(c.items[c.view[j]]));
  } else if (e.ctrlKey || e.metaKey) {
    const kk = key(c.items[c.view[i]]);
    c.sel.has(kk) ? c.sel.delete(kk) : c.sel.add(kk);
    S.anchor = i;
  } else S.anchor = i;
  c.cursor = i;
  renderRows(k);
  renderHeads();
  renderWhere();
  renderHints();
  detailFollow();
}

// ---- verbs (rule 2): each acts at once and is one undo step ----

// edited runs a verb, says what it did and redraws; the visible rows slide to their new
// places and the ones it changed (ids; else what moved) flash (06 §7.1). call is a function
// that starts the verb: the verbs go to Go one after another, in the order given, and the
// page reads the lists again once, after the last (a key held down does not wait on each
// step). local, when given, changes the lists here at once, before Go answers.
let editChain = Promise.resolve(), editsPending = 0, reloadWanted = false, editEpoch = 0;
async function edited(call, undoable = true, ids = null, local = null) {
  const before = motion.snapshot();
  editEpoch++;
  editsPending++;
  S.editing = editsPending;
  if (local) {
    for (const c of S.cols) c.sel.clear();
    if (local() !== false) { render(); motion.play(before, ids); detailFollow(); }
  }
  const mine = editChain.then(async () => {
    try { return { r: await (typeof call === "function" ? call() : call) }; } catch (e) { return { e }; }
  });
  editChain = mine;
  const { r, e } = await mine;
  editsPending--;
  S.editing = editsPending;
  if (e) say("fail", errText(e));
  else {
    const text = typeof r === "string" ? r : r && r.text;
    say("ok", text, undoable);
    S.placed = r && r.placed && r.placed.length ? r.placed : null;
  }
  if (editsPending) { reloadWanted = true; return e ? null : r; }
  reloadWanted = false;
  if (!local) for (const c of S.cols) c.sel.clear();
  const again = motion.snapshot();
  await reload();
  motion.play(local ? again : before, S.placed || ids);
  return e ? null : r;
}

// ---- the lists changed here at once (edited's local) ----

// take removes the rows with these keys from column k, in the column's order.
function take(k, keys) {
  const c = S.cols[k], want = new Set(keys), out = [], rest = [];
  for (const it of c.items) (want.has(key(it)) ? out : rest).push(it);
  if (!out.length) return out;
  const curKey = cur(k) && key(cur(k));
  const shown = new Set(c.view.map((i) => key(c.items[i])));
  c.items = rest;
  relist(k, shown, curKey && !want.has(curKey) ? curKey : null, c.cursor);
  return out;
}

// put adds rows to column k: 已启用 at a 1-based place (0: the end), 未启用 in its order
// (official first, then by name), 暂存 at the end. The cursor goes to the first.
function put(k, its, at = 0) {
  if (!its.length) return;
  const c = S.cols[k];
  const shown = new Set(c.view.map((i) => key(c.items[i])));
  for (const it of its) shown.add(key(it));
  if (k === 1) {
    const i = at > 0 ? Math.min(at - 1, c.items.length) : c.items.length;
    c.items = [...c.items.slice(0, i), ...its.map((it) => ({ ...it, unsaved: true })), ...c.items.slice(i)];
  } else if (k === 0) {
    const rank = (it) => [it.kind === "official" ? 0 : 1, (it.name || "").toLowerCase()];
    const less = (a, b) => { const x = rank(a), y = rank(b); return x[0] - y[0] || (x[1] < y[1] ? -1 : x[1] > y[1] ? 1 : 0); };
    const out = [...c.items];
    for (const it of its) {
      const v = { ...it, pos: 0 };
      const i = out.findIndex((o) => less(v, o) < 0);
      i < 0 ? out.push(v) : out.splice(i, 0, v);
    }
    c.items = out;
  } else c.items = [...c.items, ...its.map((it) => ({ ...it, pos: 0 }))];
  relist(k, shown, key(its[0]));
}

// relist renumbers 已启用 and rebuilds a column's view: every row, or while it is filtered
// the rows it showed and the ones that came in (Go filters again after the reload).
function relist(k, shown, keepKey, keepAt = 0) {
  const c = S.cols[k];
  if (k === 1) c.items = c.items.map((it, i) => (it.pos === i + 1 ? it : { ...it, pos: i + 1 }));
  const filtered = k < 2 ? barOn() : !!c.query.trim();
  c.view = [];
  c.items.forEach((it, i) => { if (!filtered || shown.has(key(it))) c.view.push(i); });
  let at = keepKey ? c.view.findIndex((i) => key(c.items[i]) === keepKey) : -1;
  if (at < 0) at = keepAt;
  c.cursor = Math.max(0, Math.min(at, c.view.length - 1));
}

// moveLocal: the selection leaves column from for column to (at: 已启用's place). The focus
// follows the rows only within 已启用.
function moveLocal(from, to, keys, at = 0) {
  if (!S.cols[from] || !keys.length) return false;
  const its = take(from, keys);
  if (!its.length) return false;
  put(to, its, at);
  return true;
}

// toggleLocal is space: 已启用 → 未启用 (an entry with no mod only leaves), else into
// 已启用 at its end (Go then puts it where the rules say; the reload slides it there).
function toggleLocal(k, its) {
  const mods = its.filter((it) => it.id).map(key);
  if (k !== 1) return moveLocal(k, 1, mods);
  const gone = take(1, its.filter((it) => !it.id).map(key)).length > 0;
  return moveLocal(1, 0, mods) || gone;
}

// blockAt: where MoveBy puts the block (view.MoveBy): its first row d places on, in the
// list without it.
function blockAt(keys, d) {
  const c = S.cols[1], want = new Set(keys);
  const first = c.items.findIndex((it) => want.has(key(it)));
  if (first < 0) return 0;
  const rest = c.items.length - want.size;
  return Math.max(1, Math.min(first + 1 + d, rest + 1));
}

// stepAt is view.StepAt: J / K while the bar shows only some rows.
function stepAt(visible, moving, d) {
  if (!moving.length || !d) return 0;
  const mv = new Set(moving), first = Math.min(...moving), last = Math.max(...moving);
  const others = visible.filter((p) => !mv.has(p));
  const before = (p) => moving.filter((q) => q < p).length;
  if (d > 0) {
    const after = others.filter((p) => p > last);
    if (!after.length) return 0;
    const p = after[Math.min(d, after.length) - 1];
    return p - before(p) + 1;
  }
  const ahead = others.filter((p) => p < first);
  if (!ahead.length) return 0;
  const p = ahead[Math.max(ahead.length + d, 0)];
  return p - before(p);
}

// Browsing another copy in the detail must also retarget its keys and menu.
function actionContext() {
  if (S.focus === 3 && S.detail?.id) {
    for (let k = 0; k < 3; k++) {
      const item = S.cols[k].items.find((it) => it.id === S.detail.id);
      if (item) return { k, sel: [item] };
    }
  }
  const k = S.focus < 3 ? S.focus : lastList;
  return { k, sel: S.focus < 3 ? selection() : (cur(k) ? [cur(k)] : []) };
}

async function verb(name) {
  if (environmentReason() && !["full", "workshop", "folder", "workshop-id", "steam", "needs"].includes(name)) return say("busy", environmentReason());
  const { k, sel } = actionContext();
  const ids = idsOf(sel);
  switch (name) {
    case "toggle":
      if (!sel.length) return;
      return edited(() => api.Toggle(k, ids, refsOf(sel)), true, ids, () => toggleLocal(k, sel));
    case "stage":
      if (!ids.length) return;
      return edited(() => api.Stage(k, ids), true, ids, () => moveLocal(k, k === 2 ? 1 : 2, sel.filter((it) => it.id).map(key)));
    case "move": {
      const placed = S.placed;
      if (placed) return pop.position({ ids: placed, what: placed.length === 1 ? nameOf(placed[0]) : named(placed.map((id) => ({ name: nameOf(id) }))) }, 1);
      if (!ids.length) return;
      return pop.position({ ids, what: named(sel) }, k);
    }
    case "down": case "up": case "down10": case "up10": {
      if (k !== 1 || !ids.length) return say("", T("only enabled mods can be moved"));
      const d = { down: 1, up: -1, down10: 10, up10: -10 }[name];
      // the bar shows only some rows: past the next row shown (view.StepAt, 06 §3.5)
      const keys = sel.map(key);
      if (barOn()) {
        const c = col(1), shown = c.view.map((i) => c.items[i].pos);
        const at = stepAt(shown, sel.map((it) => it.pos).filter(Boolean), d);
        if (!at) return say("", T("already at the edge of the rows shown"));
        return edited(() => api.MoveShown(ids, d, shown), true, ids, () => moveLocal(1, 1, keys, at));
      }
      const at = blockAt(keys, d);
      return edited(() => api.MoveBy(ids, d), true, ids, () => moveLocal(1, 1, keys, at));
    }
    case "tags":
      return pop.tags(sel);
    case "full":
      if (S.focus===3 && S.detail?.id) return fullPage.full("plan",S.detail.id);
      if (sel.length && sel[0].id) { lastList = k; return scr.full(); }
      return;
    case "workshop":
      if (S.detail && S.detail.url) return api.OpenURL(S.detail.url).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
      return say("", T("not a workshop mod"));
    // ctrl+o / ctrl+w: the cursor's mod, not the details (they may still be loading)
    case "folder": case "workshop-id": case "steam": {
      const it = S.focus < 3 ? cur() : sel[0];
      if (!it || !it.id) return say("", name === "folder" ? T("RimArcho has not seen this copy: no folder") : T("no Workshop ID: not a Workshop mod"));
      if (name === "folder") return api.OpenFolder(it.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
      if (name === "steam") return api.OpenSteamWorkshop(it.id).then((t) => say("ok", t), (e) => say("", errText(e)));
      return api.OpenWorkshop(it.id).then((t) => say("ok", t), (e) => say("", errText(e)));
    }
    case "swap":
      if (sel.length === 1 && sel[0].id) return edited(() => api.Swap(sel[0].id), true, ids);
      return;
    case "note": {
      // the mod's own note (06 §3.5): one line, from what is there; empty clears it
      const it = sel.length === 1 ? sel[0] : null;
      if (!it || !it.id) return say("", T("one mod at a time"));
      const now = await api.ModNote(it.id).catch(() => "");
      return pop.ask(Tf("note on %s", it.name), T("a note on this mod…"), async (text) => {
        try { say("ok", await api.SetModNote(it.id, text)); } catch (err) { say("fail", errText(err)); }
        await reload();
      }, now);
    }
    case "undo": return edited(() => api.Undo(), false);
    case "redo": return edited(() => api.Redo(), false);
    case "restore-all":
      if (!S.cols[2].items.length) return say("", T("staging is empty"));
      return edited(() => api.RestoreAll());
    case "stage-all": return edited(() => api.StageAll());
    case "needs": return scr.needs();
  }
}

function nameOf(id) {
  for (const c of S.cols) { const it = c.items.find((x) => x.id === id); if (it) return it.name; }
  return id;
}

async function place(from, obj, at) {
  const keys = S.cols[from] ? S.cols[from].items.filter((it) => obj.ids.includes(it.id)).map(key) : [];
  await edited(() => api.Place(from, obj.ids, at), true, obj.ids, () => moveLocal(from, 1, keys, at));
}

// drop: dragging is another way to run a verb (06 §8.5).
async function drop(from, to, items, at) {
  const ids = idsOf(items);
  const keys = items.map(key), withID = items.filter((it) => it.id).map(key);
  if (to === 1) return edited(() => api.Place(from, ids, at), true, ids, () => moveLocal(from, 1, from === 1 ? keys : withID, at));
  if (from === to) return;
  if (to === 2) return edited(() => api.Stage(from, ids), true, ids, () => moveLocal(from, 2, withID));
  if (to === 0 && from === 1) return edited(() => api.Toggle(1, ids, refsOf(items)), true, ids, () => toggleLocal(1, items));
  if (to === 0 && from === 2) return edited(() => api.Unstage(ids), true, ids, () => moveLocal(2, 0, withID));
}

// run: sort, save and launch act at once and say what they did (06 §6); sort
// offers 撤销. A refusal says why in the status line and shows the commands that resolve it.
async function run(what) {
  const before = motion.snapshot();
  let o;
  try { o = await api.Run(what); } catch (e) { return say("fail", errText(e)); }
  // another version in the way that can move into the lock library: one confirmation, then on
  if (o.moves) return confirmMoves(o, () => api.RunMoving(what), async (o2) => { say(o2.refused ? "fail" : "ok", o2.text); await reload(); motion.play(before, null); });
  if (o.refused) {
    say("fail", o.text);
    if (o.refused.length > 1) pop.text({ sort: T("sort"), save: T("save"), launch: T("launch") }[what], o.refused);
  } else say("ok", o.text, !!o.undo);
  await reload();
  motion.play(before, null);
  if (what === "sort" && o.missingDependencies?.length) offerDependencies(o.missingDependencies);
}

// offerDependencies: a sort found enabled mods whose required mods are not enabled (04).
// Installed ones are enabled in one undo step and the list sorted again; Workshop ones are
// subscribed after the usual Steam / SteamCMD choice; the rest can only be named.
function offerDependencies(missing) {
  const by = (k) => missing.filter((m) => m.kind === k);
  const installed = by("installed"), workshop = by("workshop"), unknown = by("unknown");
  const line = (m) => Tf("%s, needed by %s", m.name || m.packageId, (m.requiredBy || []).slice(0, 3).join(T(", ")) + ((m.requiredBy || []).length > 3 ? " …" : ""));
  const lines = [];
  if (installed.length) lines.push(Tf("installed, not enabled (%d):", installed.length), ...installed.map(line));
  if (workshop.length) lines.push(Tf("not installed, on the Workshop (%d):", workshop.length), ...workshop.map(line));
  if (unknown.length) lines.push(Tf("not installed, no known source (%d):", unknown.length), ...unknown.map(line));
  const subscribe = (ids) => pop.choose(Tf("get %d mods", ids.length), getChoices(), async (i) => {
    try { say("ok", await api.Subscribe({ kind: "workshop", url: ids.join(" "), steamcmd: i === 1, batch: true })); } catch (e) { say("fail", errText(e)); }
  });
  if (!installed.length && !workshop.length) return pop.text(T("some required mods are missing"), lines);
  const go = installed.length
    ? (workshop.length ? Tf("enable %d and subscribe %d", installed.length, workshop.length) : Tf("enable %d and sort again", installed.length))
    : Tf("subscribe %d", workshop.length);
  pop.confirm(T("some required mods are missing"), lines, go, async () => {
    let ids = workshop.map((m) => m.workshopId);
    if (installed.length) {
      const r = await edited(() => api.CompleteDependencies(), true);
      if (r?.workshopIds?.length) ids = r.workshopIds;
    }
    if (ids.length) subscribe([...new Set(ids)]);
  });
}

// confirmMoves asks once about moving folders into the lock library (08 §3), then runs again.
export function confirmMoves(o, again, then) {
  say("", "");
  pop.confirm(T("move the other versions into the lock library"), o.moves, T("move and go on"), async () => {
    say("busy", T("moving into the lock library…"));
    let o2;
    try { o2 = await again(); } catch (e) { return say("fail", errText(e)); }
    if (o2.refused && o2.refused.length > 1) pop.text(T("launch"), o2.refused);
    then(o2);
  });
}

// preview: the dry run of sort, save or launch, to read (the palette's 预览…); nothing to confirm.
async function preview(what) {
  try { const p = await api.Preview(what); pop.text(p.title, p.lines); } catch (e) { say("fail", errText(e)); }
}

// cmd runs a palette command (view.Commands).
async function cmd(id) {
  if (environmentReason() && ["launch", "sort", "save", "undo", "redo", "restore-all", "stage-all", "rescan", "analyze"].includes(id)) return say("busy", environmentReason());
  // A page replaces the page shown once its data is in: the mod list never shows between.
  const pages = { home: "home", needs: "needs", logs: "log", changes: "changes", subscribe: "subscribe", library: "library", log: "log", settings: "settings", packs: "packs", build: "build", "env-page": "env", uninstall: "uninstall", textures: "textures", tasks: "tasks" };
  if (pages[id]) return go(pages[id]);
  // the list's own actions show the list; the rest work on any page
  if (["filter", "details", "tech", "tags"].includes(id) && scr.closeScreen()) renderWhere();
  switch (id) {
    // from the editor, 启动 opens the launch page, as L does there (06 §2.5)
    case "launch": return S.screen ? run(id) : goLaunch();
    case "sort": case "save": return run(id);
    case "preview-sort": case "preview-save": case "preview-launch": return preview(id.slice(8));
    case "undo": case "redo": case "restore-all": case "stage-all": return verb(id);
    case "rescan":
      if (await api.Scan()) { S.busy.scan = true; say("busy", T("scanning…")); scr.busyChanged(); }
      return;
    case "analyze":
      if (await api.Analyze()) { S.busy.analyze = true; say("busy", T("checking the mods' files…")); }
      return;
    case "env": return chooseEnv();
    case "plans": return plans.chooser();
    case "new-plan": return plans.newPlan();
    case "filter": return startSearch();
    case "details": return verb("full");
    case "tech": return verb("full");
    case "tags": return verb("tags");
    case "cli": return commandLine();
    case "import": return S.screen === "plans" ? planManage.newList("import") : exchange(0);
    case "export": return exportCurrent();
    case "help": return pop.help(S.screen || "plan");
    case "quit": return window.close();
  }
}

async function chooseEnv() {
  const envs = (S.board && S.board.envs) || [];
  if (!envs.length) return say("", T("no games yet (archo env add)"));
  pop.choose(T("switch game"), envs, (i) => useEnv(envs[i]),
    Math.max(0, envs.indexOf(S.board.env)), { key: "Delete", label: T("remove the game"), envWrite: "remove-env", run: (i) => scr.removeEnv(envs[i]) });
}

async function useEnv(name) {
  try {
    const switched = await withEnvironmentSwitch(name, async (current) => {
      const result = await api.UseEnv(name);
      if (!current() || result.stale) return;
      for (const c of S.cols) { c.query = ""; c.cursor = 0; c.sel.clear(); }
      await reload(result);
      if (current()) await refreshEnvironment();
      if (current()) say("ok", result.text);
    });
    return switched;
  } catch (e) {
    say("fail", errText(e));
    return false;
  }
}

// Refresh whichever page is visible now; completing a switch never returns to its caller.
async function refreshEnvironment() {
  S.screenSeq++;
  if (!S.screen) return openPage("plan");
  if (S.screen && S.screen !== "guide") await openPage(S.screen);
}

function commandLine() {
  pop.ask(T("run an archo command"), "list --filter tag:ui", async (line) => {
    const out = await api.RunCLI(line);
    pop.text("archo " + line, out);
    reload();
  });
}

async function paletteChoice(o) {
  if (o.kind === 0) return cmd(o.id);
  if (o.kind === 3) return go(o.id);
  if (o.kind === 2) {
    return useEnv(o.id);
  }
  // a mod: go to its list and row; a filter that hides it is cleared (06 §3.5)
  for (const k of [1, 0, 2]) {
    const c = S.cols[k];
    let i = c.items.findIndex((x) => x.id === o.id);
    if (i < 0) continue;
    let v = c.view.indexOf(i);
    if (v < 0 && k < 2) { S.bar.clear(); await barChanged(); v = c.view.indexOf(i); }
    if (v < 0) { c.query = ""; $(`.area[data-area="${k}"] .search input`).value = ""; await refilter(k, o.id); v = c.view.indexOf(i); }
    focusArea(k);
    c.cursor = v;
    ensureVisible(k);
    renderLists();
    renderWhere();
    detailFollow();
    return;
  }
}

// ---- search (06 §3.7) ----

function startSearch(k = S.focus < 3 ? S.focus : lastList) {
  focusArea(k);
  if (k < 2) return S.bar.focus();
  const c = col(k);
  c.searching = true;
  renderHeads();
  const inp = $(`.area[data-area="${k}"] .search input`);
  inp.value = c.query;
  inp.focus();
  inp.select();
}

let searchSeq = 0;
async function search(k, q) {
  const c = S.cols[k];
  c.query = q;
  const n = ++searchSeq;
  const keep = cur(k) ? key(cur(k)) : "";
  c.cursor = 0;
  await refilter(k, "");
  if (n !== searchSeq) return;
  showGhost(k);
  renderHeads();
  renderRows(k);
  renderWhere();
  detailFollow();
}

function searchKey(k, e, inp) {
  if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); inp.value = ""; search(k, ""); col(k).searching = false; inp.blur(); renderHeads(); return; }
  if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); col(k).searching = false; inp.blur(); renderHeads(); return; }
  // tab or → at the end takes the grey completion (06 §3.7)
  if ((e.key === "Tab" && !e.shiftKey) || e.key === "ArrowRight") {
    if (acceptGhost(k, inp)) { e.preventDefault(); e.stopPropagation(); return; }
  }
  if (e.key === "ArrowLeft" || e.key === "Home" || e.key === "End") setTimeout(() => showGhost(k));
  // 未启用 and 已启用 share the filter bar: tab only takes the completion here (06 §3.7)
  if (e.key === "Tab") { e.preventDefault(); e.stopPropagation(); return; }
  if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); moveCursor(e.key === "ArrowDown" ? 1 : -1, false); }
  e.stopPropagation();
}

// barChanged: the bar's search or filters changed (or it opened): both lists again, the
// cursors kept on their mods.
let barSeq = 0;
async function barChanged() {
  const n = ++barSeq;
  const keep = [0, 1].map((k) => (cur(k) ? key(cur(k)) : ""));
  await Promise.all([0, 1].map((k) => refilter(k, keep[k])));
  if (n !== barSeq) return;
  S.bar.draw();
  for (const k of [0, 1]) ensureVisible(k);
  renderLists();
  renderWhere();
  renderHints();
  detailFollow();
}

// historyChooser: the history's steps and versions as a list (H): the keyboard's way to what
// a click on the history fold does (06 §2.5, §3.1 rule 6).
function historyChooser() {
  const hs = (S.history || []).filter((h) => h.kind !== "now");
  if (!hs.length) return say("", T("no history yet"));
  pop.choose(T("history"), hs.map((h) => (h.kind === "version" ? "◆ " : "") + h.text), (i) => onHistoryPick(hs[i]));
}
let onHistoryPick = () => {};

// changesOfCursor: the 变化 page with only the cursor's mod (C; the detail's link).
function changesOfCursor() {
  const it = actionContext().sel[0];
  if (it && it.id) document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: it.id, name: it.name } }));
}

// widthKey: < > make the focused column narrower or wider, = gives the default widths back
// (the splits' drag and double click, 06 §2.5).
function widthKey(d) {
  const board = $("#board");
  const k = S.focus === 3 ? 0 : S.focus === 1 ? 1 : 2;
  if (!d) { [0, 1, 2].forEach((j) => board.style.removeProperty(`--c${j}`)); api.SetSetting("ui.gui.columns.v2:" + ((S.board && S.board.env) || ""), "").catch(() => {}); return; }
  const ws = COLS().map((a) => a.getBoundingClientRect().width);
  const other = k === 2 ? 1 : k + 1;
  const step = 24 * d;
  if (ws[k] + step < 160 || ws[other] - step < 160) return;
  ws[k] += step; ws[other] -= step;
  ws.forEach((v, j) => board.style.setProperty(`--c${j}`, v + "fr"));
  api.SetSetting("ui.gui.columns.v2:" + ((S.board && S.board.env) || ""), [0, 1, 2].map((j) => board.style.getPropertyValue(`--c${j}`)).join(",")).catch(() => {});
  for (const j of [0, 1, 2]) renderRows(j);
}

// subPage: tab / shift+tab go to the next / previous page under the open sidebar entry.
function subPage(d) {
  const cur = shell.current();
  for (const g of (S.shell && S.shell.nav) || []) for (const it of g.items || []) {
    const kids = (it.kids || []).filter((x) => !x.add);
    const i = kids.findIndex((x) => x.id === cur);
    if (i >= 0 && kids.length > 1) return go(kids[(i + d + kids.length) % kids.length].id);
    // the entry's own page that is not one of its sub-entries (整合包's list): to the first
    // or the last of them
    if (i < 0 && it.id === cur && kids.length) return go(kids[d > 0 ? 0 : kids.length - 1].id);
  }
}

// ---- the row menu (06 §8.5): the same groups and order as Mod 库存's, from view.RowOps ----

async function openRowMenu(x, y, keys) {
  const { k, sel } = actionContext();
  const it = sel[0];
  if (!it) return;
  let codes = [];
  // a list entry with no mod: only out of the list
  if (!it.id) codes = k === 1 ? ["disable"] : [];
  else try { codes = await api.RowOps(k, it.id); } catch { codes = []; }
  const n = S.focus < 3 ? selection().length : 1;
  if (n > 1) codes = codes.filter((c) => !ops.op(c).one);
  ops.openMenu(x, y, codes, (c) => rowOp(c), keys);
}

function menuAtCursor() {
  const k = S.focus < 3 ? S.focus : lastList;
  const r = $(`.area[data-area="${k}"] .row.cur`) || $(`.area[data-area="${k}"] .rows`);
  const b = r.getBoundingClientRect();
  openRowMenu(b.left + 40, b.bottom, true);
}

// rowOp runs an op of the menu on the object: the plan's own as their verbs, the version
// and disk ops as Mod 库存 runs them (a dry run first when they change files).
async function rowOp(code, only = null) {
  const verbs = { enable: "toggle", disable: "toggle", "put-back": "toggle", "enable-at": "move", "move-to": "move", stage: "stage", swap: "swap", tags: "tags", note: "note", folder: "folder", workshop: "workshop-id", steam: "steam" };
  if (verbs[code] && !only) return verb(verbs[code]);
  const ids = only || idsOf(actionContext().sel);
  if (!ids.length) return;
  const o = ops.op(code);
  if (code === "subscription-update" && ids.length === 1) {
    const copies = await api.ModCopies(ids[0]);
    const copy = copies?.row?.insts?.find((c) => c.id === ids[0]);
    if (copy?.subscriptionKind === "page") {
      const host = document.querySelector(".detail .scroll") || document.querySelector("#screen");
      let panel;
      panel = updateFromForm(host, copy.ref, () => panel.remove());
      return;
    }
  }
  const run = () => edited(() => api.RunOp(code, ids), code === "remove", ids);
  if (!o.confirm) return run();
  let p;
  try { p = await api.OpPreview(code, ids); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, o.label, p.apply ? run : null);
}

// delKey: Del unsubscribes the Workshop copy or deletes one of the player's own (asks first).
async function delKey() {
  const { k, sel } = actionContext();
  const it = sel[0];
  if (!it || !it.id) return;
  const codes = await api.RowOps(k, it.id).catch(() => []);
  const c = ops.keyed(codes, "Del");
  if (!c) return say("", T("nothing to do with this row"));
  return rowOp(c);
}

// ---- the keyboard ----

function onKey(e) {
  if (e.isComposing || e.defaultPrevented) return;
  const k = e.key;
  // a menu opened with the mouse does not hold the focus: esc still closes it first (G5)
  if (k === "Escape" && !$("#menu").hidden) { e.preventDefault(); pop.hideMenu(); return; }
  // ctrl+b moves the navigation between the side and the top, on every page (06 §2.1)
  if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && k.toLowerCase() === "b") { e.preventDefault(); shell.toggleNav(); return; }
  // ctrl+j: 后台任务 from any page; again goes back (06 §2.16)
  if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && k.toLowerCase() === "j" && !S.popup) { e.preventDefault(); shell.navigated(); tasksPage.toggle(); renderWhere(); return; }
  // Native tab order reaches paths, including the first one; activation stays local.
  if (k === "Tab" && (S.popup ? $("#popup") : S.screen ? $("#screen") : $("#detail"))?.querySelector(".path-copy")) return;
  if (pop.popupKey(e)) return;
  if (k === "Escape" && S.screen === "plan-import") return importKey(e);
  if (k === "Escape" && S.screen === "plans") return planManage.key(e);
  if (document.activeElement && ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement.tagName)) return;
  if ((k === "u" || ((e.ctrlKey || e.metaKey) && k === "z")) && !S.busy.env && undoStatus()) { e.preventDefault(); return; }
  if (k !== "Shift" && k !== "Control" && k !== "Alt") clearSay();
  // shift+1–8: the sidebar's entries. By the physical key, so it works on any layout
  // (shift+1 is "!" on a US keyboard, shift+3 is "#").
  const dig = /^Digit([1-9])$/.exec(e.code || "");
  if (dig && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
    e.preventDefault();
    if (NAV[+dig[1] - 1]) go(navTarget(NAV[+dig[1] - 1]));
    return;
  }
  // tab / shift+tab: the next / previous page under the same sidebar entry (Mod 库存's 全部 and
  // 变化, the plans); the full detail keeps tab for its sub-sections (06 §3.5)
  // (on the pages with forms, settings and the like, tab keeps moving the focus)
  // Pages with adjustable cards keep native tab order to reach their gaps (06 §3.1).
  if (e.target.closest?.('[role="separator"]') && k === "Tab") return;
  if (k === "Tab" && !e.ctrlKey && !e.altKey && S.screen === "packs") { e.preventDefault(); return subPage(e.shiftKey ? -1 : 1); }
  if (!S.screen && scr.pageLoading()) {
    if ((e.ctrlKey || e.metaKey) && k === "k") { e.preventDefault(); pop.palette(); }
    return;
  }
  if (S.screen) {
    // esc (and enter) leave the full detail for the plan; the sidebar's pages are side by
    // side, so esc does not go from one to another (06 §3.1)
    if (S.screen === "full" && (k === "Escape" || k === "Backspace")) { e.preventDefault(); return fullPage.back(); }
    if (e.ctrlKey && k === "k") { e.preventDefault(); return pop.palette(); }
    if (k === "?" && S.screen !== "full") { e.preventDefault(); return pop.help(S.screen); }
    // Navigation stays available while a new page is loading. Its object shortcuts
    // must wait for data, including after an initial read failed.
    if (scr.pageLoading()) {
      if (S.screen === "build" && k === "Escape") { e.preventDefault(); return go(S.buildFrom || "plan"); }
      return;
    }
    // 1–9 focus the page's numbered areas (06 §3.5)
    if (S.screen === "tasks") return tasksPage.key(e);
    if (S.screen !== "home" && scr.areaKey(e)) return;
    if (S.screen === "home") return homePage.homeKey(e);
    if (S.screen === "packs" || S.screen === "build") return packsPage.packsKey(e);
    if (S.screen === "plan-import") return importKey(e);
    if (S.screen === "plans" || S.screen === "plan-export") return planManage.key(e);
    if (S.screen === "needs") return scr.needsKey(e);
    if (S.screen === "library") return library.key(e);
    if (S.screen === "textures") return tex.key(e);
    if (S.screen === "log" || S.screen === "changes") return scr.logsKey(e);
    // 添加订阅: the form takes its keys; esc outside it goes back to 订阅与更新
    if (S.screen === "subscribe") { if (k === "Escape") { e.preventDefault(); go("changes"); } return; }
    if (S.screen === "full") return scr.fullKey(e);
    return;
  }
  const ctrl = e.ctrlKey || e.metaKey;
  const handled = () => e.preventDefault();
  // navigation
  if (k === "ArrowDown" || k === "ArrowUp") {
    handled();
    if (S.focus === 3) return scrollDetail(k === "ArrowDown" ? 1 : -1, false), renderWhere();
    return moveCursor(k === "ArrowDown" ? 1 : -1, e.shiftKey);
  }
  if (k === "ArrowLeft" || k === "ArrowRight") {
    handled();const areas=[3,1,0],at=areas.indexOf(S.focus===2?0:S.focus);
    return focusArea(areas[Math.max(0,Math.min(2,at+(k==="ArrowRight"?1:-1)))]);
  }
  // PgUp / PgDn move the cursor a page (06 §3.3); in the details they scroll them
  if (k === "PageDown" || k === "PageUp") {
    handled();
    if (S.focus === 3) { scrollDetail(k === "PageDown" ? 1 : -1, true); return renderWhere(); }
    const box = $(`.area[data-area="${S.focus}"] .rows`);
    return moveCursor(pageDelta(S.focus,k === "PageDown" ? 1 : -1), e.shiftKey);
  }
  if (k === "Home" || k === "End") { handled(); if (S.focus < 3) moveCursor(k === "End" ? 1e9 : -1e9, e.shiftKey); return; }
  if (!ctrl && "1234".includes(k) && k.length === 1) { handled(); return focusArea(+k - 1); }
  if (k === "Escape") {
    handled();
    if (S.focus < 3 && col().sel.size) { col().sel.clear(); renderLists(); renderWhere(); renderHints(); }
    // the filter bar: one press clears it and closes it (G5: back to the base in at most 3)
    else if (S.focus < 2 && (barOn() || S.bar.open)) { if (barOn()) S.bar.clear(); if (S.bar.open) S.bar.toggle(false); }
    else if (S.focus === 2 && col().query) { col().query = ""; $(`.area[data-area="${S.focus}"] .search input`).value = ""; search(S.focus, ""); }
    return;
  }
  if (ops.menuKey(e)) { handled(); return menuAtCursor(); }
  if (k === "Delete") { handled(); return delKey(); }
  if (ctrl && k.toLowerCase() === "a" && S.focus < 3) {
    handled();
    const c = col();
    for (const i of c.view) c.sel.add(key(c.items[i]));
    renderRows(S.focus); renderWhere(); renderHints();
    return;
  }
  if (ctrl && (k === " " || k === "Spacebar")) {
    handled();
    if (S.focus > 2) return;
    const it = cur();
    if (it) { const c = col(); c.sel.has(key(it)) ? c.sel.delete(key(it)) : c.sel.add(key(it)); renderRows(S.focus); renderWhere(); renderHints(); }
    return;
  }
  // commands
  if (ctrl) {
    // ctrl+w: the Wails window has no accelerator for it (no app menu, WebView2's
    // accelerator keys off), so preventDefault keeps it ours
    const m = { k: () => pop.palette(), f: () => startSearch(), z: () => verb("undo"), y: () => verb("redo"), r: () => verb("redo"),
      o: () => verb("folder"), w: () => verb("workshop-id"), s: () => verb("steam") }[k.toLowerCase()];
    if (m) { handled(); m(); }
    return;
  }
  if (e.altKey && (k === "J" || k === "j" || k === "K" || k === "k")) { handled(); return verb(k.toLowerCase() === "j" ? "down10" : "up10"); }
  const map = {
    " ": () => verb("toggle"), t: () => verb("stage"), m: () => verb("move"), J: () => verb("down"), K: () => verb("up"),
    g: () => verb("tags"), n: () => verb("note"), Enter: () => verb("full"), x: () => verb("swap"),
    S: () => run("sort"), w: () => run("save"), L: () => goLaunch(), u: () => verb("undo"), T: () => verb(S.focus === 2 ? "restore-all" : "stage-all"),
    "/": () => startSearch(), "\\": () => S.bar.toggle(), c: () => go("changes"), l: () => go("log"), R: () => cmd("rescan"), A: () => cmd("analyze"),
    H: () => historyChooser(), C: () => changesOfCursor(), "<": () => widthKey(-1), ">": () => widthKey(1), "=": () => widthKey(0),
    E: () => go("env"), P: () => plans.chooser(), "?": () => pop.help("plan"), ":": () => commandLine(), i: () => exchange(0), e: () => exportCurrent(),
  };
  // shift+letter is the capital letter even where the key reports it in lower case
  const kk = e.shiftKey && /^[a-z]$/.test(k) ? k.toUpperCase() : k;
  if (map[kk]) { handled(); map[kk](); }
}

// ---- the column widths (06 §7) ----

// Three spatial columns: detail/history, enabled, inactive/staging.
const COLS = () => [$("#board > .fcol"), $('.area[data-area="1"]'), $("#board .editor-right")];

function wireSplits() {
  const board = $("#board");
  const key = () => "ui.gui.columns.v2:" + ((S.board && S.board.env) || "");
  $$("#board > .split").forEach((sp) => {
    const lo = +sp.dataset.split, hi = lo + 1;
    wireSeparator(sp, {
      key, get: () => COLS()[lo].getBoundingClientRect().width,
      range: () => [160, COLS()[lo].getBoundingClientRect().width + COLS()[hi].getBoundingClientRect().width - 160],
      set: (value) => {
        const widths = COLS().map((a) => a.getBoundingClientRect().width);
        widths[hi] -= value - widths[lo]; widths[lo] = value;
        // Hidden columns keep a usable width for when they show again (06 §2.5).
        widths.forEach((v, j) => board.style.setProperty(`--c${j}`, (v || 220) + "fr"));
      },
      reset: () => [0, 1, 2].forEach((j) => board.style.removeProperty(`--c${j}`)),
      serialize: () => [0, 1, 2].map((j) => board.style.getPropertyValue(`--c${j}`)).join(","),
      changed: () => { for (const k of [0, 1, 2]) renderRows(k); },
    });
  });
  // Each fold shares its own parent column (06 §3.1, 06 §2.5).
  board.querySelectorAll(".stack > .fold").forEach((card) => {
    const stack = card.parentElement;
    const gap = el("div", "hsplit stack-split");
    stack.insertBefore(gap, card);
    const set = (v) => {
      card.classList.toggle("stack-sized", !!v);
      if (v) card.style.setProperty("--stack-size", v + "px");
      else card.style.removeProperty("--stack-size");
    };
    wireSeparator(gap, {
      axis: "y", direction: -1, key: "ui.gui.plan." + (card.dataset.area === "2" ? "stage" : "history"),
      enabled: () => card.classList.contains("open") && !card.hidden,
      range: () => [72, stack.clientHeight - 148 - [...stack.querySelectorAll(".fold")]
        .filter((other) => other !== card).reduce((h, other) => h + other.getBoundingClientRect().height, 0)],
      get: () => card.getBoundingClientRect().height, set, reset: () => set(0), restore: (v) => set(+v || 0),
      changed: () => renderRows(2),
    });
  });
}

async function loadWidths() {
  const v = await api.Setting("ui.gui.columns.v2:" + ((S.board && S.board.env) || ""));
  const board = $("#board");
  let ws = (v || "").split(",");
  ws.slice(0, 3).forEach((w, j) => { if (w) board.style.setProperty(`--c${j}`, w); });
}

// ---- start ----

const ui = {
  reload, refreshEnvironment, relang, reaccent, render, pageEntering, pageEntered, renderWhere, renderHints, renderHeads, click, verb, drop, place, run, preview, cmd, step, search, searchKey, go, useEnv,
  palette: paletteChoice, selectionItems: selection, planMenu: (id, x, y) => plans.menuAt(id, x, y),
  menu: (x, y) => openRowMenu(x, y, false),
  detailFollow: (f) => detailFollow(f),
  newList: (from) => planManage.newList(from),
  duplicateList: (id) => planManage.duplicate(id),
  selectedList: () => planManage.selected(),
  resumeGuide: () => guide.resume(),
  manageLists: (id = 0) => { planManage.select(id); return go("plans"); },
  afterImport: null, // the first-run guide's next step once an import is applied
};

async function start() {
  const s = await api.Init();
  archoTheme.apply(s.theme);
  applyAccent(s.accent);
  S.start = s;
  setText(s.text);
  document.documentElement.lang = s.lang;
  initNavigation(ui);
  wireHistory();
  pop.initPopups(ui);
  scr.initScreens(ui);
  shell.initShell(ui);
  homePage.initHome(ui);
  fullPage.initFull(ui);
  packsPage.initPacks(ui);
  planManage.initPlanManage(ui);
  guide.initGuide(ui);
  initExchange(ui);
  plans.initPlans(ui);
  library.initLibrary(ui);
  tasksPage.initTasks(ui);
  S.bar = createBar({ data: () => S.barData, change: () => barChanged(), splitKey: "ui.gui.filter.split:plan",
    column: () => $("#board > .fcol"), resized: () => renderRows(0), leave: () => focusArea(lastList < 2 ? lastList : 1),
    placeholder: T("search: words, tags:, source:, content:") + "  /" });
  $("#board .barhost").replaceWith(S.bar.el);
  $('.area[data-area="3"] > .head').addEventListener("click", () => { if (S.bar.fills()) S.bar.toggle(false); });
  wireLists(ui);
  wireSplits();
  document.addEventListener("keydown", onKey);
  document.addEventListener("verb", (e) => { const v = e.detail; v === "needs" ? go("needs") : verb(v); });
  document.addEventListener("editor-full",()=>fullPage.full("plan",S.detail?.id));
  document.addEventListener("editor-use-copy", async (e) => {
    if(environmentReason())return say("busy",environmentReason());
    const result = await edited(() => api.RunOp("use",[e.detail]),true,[e.detail]);
    if (!result) return;
    const c = S.cols[1], at = c.view.findIndex((i) => c.items[i].id === e.detail);
    if (at >= 0) { c.cursor = at; lastList = 1; ensureVisible(1); renderLists(); }
    const item = c.items.find((it) => it.id === e.detail);
    if (item) await showDetail(item, false);
    renderWhere(); renderHints();
  });
  // the details' buttons act on the copy shown, whatever the lists' selection
  document.addEventListener("detail-op", (e) => {
    if (environmentReason() && ops.op(e.detail.code).group !== 3) return say("busy", environmentReason());
    rowOp(e.detail.code, [e.detail.id]);
  });
  // The detail button always acts on the shown instance, independently of list selection.
  document.addEventListener("separate-mod", async (e) => {
    await edited(() => api.RunOp("separate", [e.detail]), false, [e.detail]);
    detailFollow(true);
  });
  // a detail's 在变化记录查看全部: 变化 with only that mod's Lineage (06 §2.9)
  document.addEventListener("changes-of", async (e) => { shell.navigated(); await scr.changes(e.detail); renderWhere(); renderHints(); });
  // the enabled list's head: import / export
  $("#board").addEventListener("click", (e) => {
    const b = e.target.closest(".head [data-cmd]");
    if (b) { e.stopPropagation(); b.dataset.cmd === "exchange" ? exchange(0) : run(b.dataset.cmd); }
  });
  $(".detail .scroll").addEventListener("scroll", () => { if (S.focus === 3) renderWhere(); }, { passive: true });
  $(".detail").addEventListener("mousedown", () => focusArea(3));
  on("scan", async (ev) => {
    S.busy.scan = false;
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    await reload();
    scr.busyChanged();
    if (S.screen === "home") homePage.home();
  });
  on("analyze", async (ev) => {
    if (ev.line) { S.busy.analyze = true; say("busy", T("checking the mods' files…") + " " + ev.line); return; }
    S.busy.analyze = false;
    say(ev.ok ? "ok" : "fail", ev.ok ? T("analysis done") : ev.error);
    await reload();
    scr.analyzeEvent(ev);
    if (ev.done && S.screen === "home") homePage.home();
  });
  on("pack", (ev) => packsPage.packEvent(ev));
  on("uninstall", (ev) => uninst.event(ev));
  on("textures", (ev) => tex.event(ev));
  on("tasks", (ev) => tasksPage.event(ev));
  document.addEventListener("task-ended", (e) => {
    if (!S.screen && ["subscribe", "subscription-from", "subscription-check", "subscription-update", "subscription-remove"].includes(e.detail?.kind)) reload();
  });
  // Menu art is prepared after the first draw, outside the task page (06 §2.14).
  on("backgrounds", () => {
    if (S.screen === "home") homePage.home({ keep: true });
    if (S.screen === "settings") refreshBackgrounds();
  });
  // a task's o on a mod: Mod 库存 searching for it (06 §2.16)
  document.addEventListener("library-search", async (e) => { shell.navigated(); await library.openSearch(e.detail); renderWhere(); });
  tasksPage.refreshFoot();
  on("guide", (ev) => guide.guideEvent(ev));
  on("download", (ev) => guide.downloadEvent(ev));
  // launching waits (up to ten seconds) for a background Steam credit switch to finish
  on("launch-wait", (ev) => say("busy", ev.text));
  // Another process (the TUI, the CLI, an agent) may have changed the data: look again
  // when the window gets the focus (G8).
  addEventListener("focus", async () => {
    try { if (S.board && (await api.Stamp()) !== S.board.stamp) await reload(); } catch { /* keep the page */ }
    checkCredit();
  });
  await reload();
  // No environment yet: the first-run guide (06 §2.15); else the home page (06 §2.3).
  const first = !S.shell || !S.shell.envs;
  if (first) await guide.start("first");
  else await go("home");
  shell.settle();
  requestAnimationFrame(() => api.Ready().catch(() => {}));
  pollLive();
  await loadWidths();
  if (first) return;
  await scr.checkDirectories();
  if (S.board && S.board.neverScanned) cmd("rescan");
  checkCredit();
}

// A history row (06 §2.5): a click goes there at once. Loading a version is one step: the
// list it replaces stays in the history and 撤销 brings it back.
onHistoryPick = (h) => goHistory(h);
setOnHistory((h) => goHistory(h));
async function goHistory(h) {
  try { say("ok", await api.HistoryGo(h.steps || 0, h.version || 0), h.kind === "version"); } catch (e) { say("fail", errText(e)); }
  await reload();
}

// pollLive follows the running game every two seconds (06 §2.1): the sidebar's foot says
// it is running, its errors lead 能不能跑, and launching waits until it exits (08 §4).
async function pollLive() {
  try {
    const l = await api.Live();
    const before = JSON.stringify(S.live && S.live.items);
    const was = !!(S.live && S.live.running);
    S.live = l;
    shell.renderLive();
    if (JSON.stringify(l.items) !== before) { scr.liveChanged(); shell.sync(); }
    if (l.running) scr.logLive();
    if (was !== !!l.running) homePage.liveChanged();
    // the game exited: a session that used SteamCMD mods may make a credit question due
    if (was && !l.running) checkCredit();
  } catch { /* the next poll tries again */ }
  setTimeout(pollLive, 2000);
}

start().catch((e) => say("fail", errText(e)));
