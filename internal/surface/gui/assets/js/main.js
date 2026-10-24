// RimArcho GUI (08). The page only lays out and maps input: what it shows comes from
// internal/view through the Go service, and the 06 §3 rules hold here too:
//   1. the arrows only move (the cursor, the focus, the scroll), never change data;
//   2. verbs act at once on the object (the selection, else the cursor's row), u undoes;
//   3. the position line always says where you are;
//   4. the hint line lists what you can do with the object now;
//   5. few popups, each closed with esc, never nested.
import { api, on, setText } from "./api.js";
import { factText, S, $, $$, el, esc, T, Tf, errText, say, clearSay, undoStatus, col, cur, selection, key, idsOf, refsOf, named, AREAS } from "./state.js";
import { openEditor, setItems, refilter, renderLists, renderHeads, renderRows, ensureVisible, wireLists, setOnHistory, stagingOpen, setStagingOpen, showGhost, acceptGhost, barOn } from "./lists.js";
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
import { initExchange, exchange, key as importKey, hintList as importHints } from "./exchange.js";
import { icon } from "./icons.js";
import * as motion from "./motion.js";
import * as ops from "./ops.js";
import * as uninst from "./uninstall.js";
import * as packsPage from "./packs.js";
import * as planManage from "./planmanage.js";
import * as tex from "./textures.js";
import * as tasksPage from "./tasks.js";
import { environmentReason, withEnvironmentSwitch } from "./environment.js";
import { wireSeparator } from "./separator.js";
import { go, openPage, initNavigation, pageEntering, pageEntered, wireHistory } from "./navigation.js";

// ---- loading ----

let reloadSeq = 0;
async function reload(snapshot = null, read = null) {
  if (S.busy.env && !snapshot && !read) return;
  if (!read && !S.screen) return openEditor((ticket) => reload(snapshot, ticket), false, () => { render(); detailFollow(true); });
  const seq = ++reloadSeq, gameRequest = S.envSeq;
  const current = () => seq === reloadSeq && gameRequest === S.envSeq && (!read || read.current());
  let b, sh, history;
  try {
    const result = snapshot || await api.Workspace();
    if (!current() || result.stale) return;
    b = result.board; sh = result.shell; history = result.history;
  } catch (e) { if (read) throw e; if (current()) say("fail", errText(e)); return; }
  if (!current()) return;
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
// page with the current plan applied and the focus on its launch button (06 §2.5).
// exportCurrent is the editor's e: the current list's export sub-page (06 §2.5, §2.17).
function exportCurrent() {
  const p = S.shell && S.shell.plan;
  if (!p) return go("plans");
  return planManage.exportOf(p.id, "editor");
}

async function goLaunch() {
  if (environmentReason()) return say("busy", environmentReason());
  await go("home");
  await homePage.focusLaunch();
}

// planActions: the plan's actions in the top bar (06 §2.5): undo, sort, build a pack, write
// to the game, and how many entries are not written to the game yet (06 §2.2); the plan's
// own menu (rename, copy, delete) and its "有未载入的外部版本" mark (04 §4.3).
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
    const n = el("span", "unwritten", Tf("%d not written", u));
    n.title = T("different from the list last written to the game; launching or writing to the game writes them");
    motion.bump("unwritten", n, u);
    out.push(n);
  } else motion.bump("unwritten", null, 0);
  out.push(btn(T("undo"), "u", () => verb("undo"), "wide-only", "undo"), btn(T("import"), "i", () => exchange()), btn(T("export"), "e", exportCurrent),
    btn(T("create a pack"), "", () => go("build"), "wide-only"), btn(T("write to the game"), "w", () => run("save"), u ? "hot" : "", "save"));
  if (plan) {
    const more = btn("⋯", "", () => { const r = more.getBoundingClientRect(); plans.menuAt(plan.id, r.left, r.bottom + 4); }, "icon-btn");
    more.title = T("the list: rename, copy, delete") + T("; ") + "P";
    out.push(more);
  }
  // 启动游戏 opens 启动, where the game starts (06 §2.5)
  const l = btn(T("launch the game"), "L", goLaunch, "primary", "launch");
  l.title = T("opens the launch page, where the game starts") + T("; ") + "L";
  out.push(l);
  return out;
}

let lastList = 1;

// The sidebar's entries in shift+1–8 order, 在玩 then 库 (06 §3.5; view.Nav).
const NAV = ["home", "needs", "log", "library", "plans", "packs", "textures", "env"];

// renderWhere: the position line (06 §3.2), the sidebar and the breadcrumb.
function renderWhere() {
  shell.sync();
  $("#planhead").hidden = !!S.screen;
  if (!S.screen) shell.setActs("plan|" + ((S.board && S.board.unsaved) || 0) + "|" + T("undo") + "|" + JSON.stringify(S.shell && S.shell.plan), ...planActions());
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
  else if (S.screen === "home") list = homePage.hintList();
  else if (S.screen === "packs" || S.screen === "build") list = packsPage.hintList();
  else if (S.screen === "plan-import") list = importHints();
  else if (S.screen === "plans" || S.screen === "plan-export") list = planManage.hintList();
  else if (S.screen) list = [{ key: "alt+←/→", label: T("page back / forward") }, { key: "shift+1–8", label: T("pages") }, { key: "ctrl+k", label: T("commands") }, { key: "?", label: T("all keys") }];
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

// edited waits for a verb, says what it did and redraws; the visible rows slide to their
// new places and the ones it changed (ids; else what moved) flash (06 §7.1).
async function edited(p, undoable = true, ids = null) {
  const before = motion.snapshot();
  let r;
  try { r = await p; } catch (e) { say("fail", errText(e)); await reload(); return null; }
  const text = typeof r === "string" ? r : r && r.text;
  say("ok", text, undoable);
  S.placed = r && r.placed && r.placed.length ? r.placed : null;
  for (const c of S.cols) c.sel.clear();
  await reload();
  motion.play(before, S.placed || ids);
  return r;
}

async function verb(name) {
  if (environmentReason() && !["full", "workshop", "folder", "workshop-id", "steam", "needs"].includes(name)) return say("busy", environmentReason());
  const k = S.focus < 3 ? S.focus : lastList;
  const sel = S.focus < 3 ? selection() : (cur(lastList) ? [cur(lastList)] : []);
  const ids = idsOf(sel);
  switch (name) {
    case "toggle":
      if (!sel.length) return;
      return edited(api.Toggle(k, ids, refsOf(sel)), true, ids);
    case "stage":
      if (!ids.length) return;
      return edited(api.Stage(k, ids), true, ids);
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
      if (barOn()) { const c = col(1); return edited(api.MoveShown(ids, d, c.view.map((i) => c.items[i].pos)), true, ids); }
      return edited(api.MoveBy(ids, d), true, ids);
    }
    case "tags":
      return pop.tags(sel);
    case "full":
      if (sel.length && sel[0].id) { lastList = k; return scr.full(); }
      return;
    case "workshop":
      if (S.detail && S.detail.url) return api.OpenURL(S.detail.url).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
      return say("", T("not a workshop mod"));
    // ctrl+o / ctrl+w: the cursor's mod, not the details (they may still be loading)
    case "folder": case "workshop-id": case "steam": {
      const it = S.focus < 3 ? cur() : cur(lastList);
      if (!it || !it.id) return say("", name === "folder" ? T("RimArcho has not seen this copy: no folder") : T("no Workshop ID: not a Workshop mod"));
      if (name === "folder") return api.OpenFolder(it.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
      if (name === "steam") return api.OpenSteamWorkshop(it.id).then((t) => say("ok", t), (e) => say("", errText(e)));
      return api.OpenWorkshop(it.id).then((t) => say("ok", t), (e) => say("", errText(e)));
    }
    case "swap":
      if (sel.length === 1 && sel[0].id) return edited(api.Swap(sel[0].id), true, ids);
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
    case "undo": return edited(api.Undo(), false);
    case "redo": return edited(api.Redo(), false);
    case "restore-all":
      if (!S.cols[2].items.length) return say("", T("staging is empty"));
      return edited(api.RestoreAll());
    case "stage-all": return edited(api.StageAll());
    case "needs": return scr.needs();
  }
}

function nameOf(id) {
  for (const c of S.cols) { const it = c.items.find((x) => x.id === id); if (it) return it.name; }
  return id;
}

async function place(from, obj, at) {
  await edited(api.Place(from, obj.ids, at), true, obj.ids);
}

// drop: dragging is another way to run a verb (06 §8.5).
async function drop(from, to, items, at) {
  const ids = idsOf(items);
  if (to === 1) return edited(api.Place(from, ids, at), true, ids);
  if (from === to) return;
  if (to === 2) return edited(api.Stage(from, ids), true, ids);
  if (to === 0 && from === 1) return edited(api.Toggle(1, ids, refsOf(items)), true, ids);
  if (to === 0 && from === 2) return edited(api.Unstage(ids), true, ids);
}

// run: sort, write to the game and launch act at once and say what they did (06 §6); sort
// offers 撤销. A refusal says why in the status line and shows the commands that resolve it.
async function run(what) {
  const before = motion.snapshot();
  let o;
  try { o = await api.Run(what); } catch (e) { return say("fail", errText(e)); }
  // another version in the way that can move into the lock library: one confirmation, then on
  if (o.moves) return confirmMoves(o, () => api.RunMoving(what), async (o2) => { say(o2.refused ? "fail" : "ok", o2.text); await reload(); motion.play(before, null); });
  if (o.refused) {
    say("fail", o.text);
    if (o.refused.length > 1) pop.text({ sort: T("sort"), save: T("write to the game"), launch: T("launch") }[what], o.refused);
  } else say("ok", o.text, !!o.undo);
  await reload();
  motion.play(before, null);
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
  const pages = { home: "home", needs: "needs", logs: "log", changes: "changes", library: "library", log: "log", settings: "settings", packs: "packs", build: "build", "env-page": "env", uninstall: "uninstall", textures: "textures", tasks: "tasks" };
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
    case "import": return exchange(0);
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
  const it = cur(S.focus < 3 ? S.focus : lastList);
  if (it && it.id) document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: it.id, name: it.name } }));
}

// widthKey: < > make the focused column narrower or wider, = gives the default widths back
// (the splits' drag and double click, 06 §2.5).
function widthKey(d) {
  const board = $("#board");
  const k = S.focus < 2 ? S.focus : 2;
  if (!d) { [0, 1, 2].forEach((j) => board.style.removeProperty(`--c${j}`)); api.SetSetting("ui.gui.columns:" + ((S.board && S.board.env) || ""), "").catch(() => {}); return; }
  const ws = COLS().map((a) => a.getBoundingClientRect().width);
  const other = k === 2 ? 1 : k + 1;
  const step = 24 * d;
  if (ws[k] + step < 160 || ws[other] - step < 160) return;
  ws[k] += step; ws[other] -= step;
  ws.forEach((v, j) => board.style.setProperty(`--c${j}`, v + "fr"));
  api.SetSetting("ui.gui.columns:" + ((S.board && S.board.env) || ""), [0, 1, 2].map((j) => board.style.getPropertyValue(`--c${j}`)).join(",")).catch(() => {});
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
  const k = S.focus < 3 ? S.focus : lastList;
  const it = cur(k);
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
async function rowOp(code) {
  const verbs = { enable: "toggle", disable: "toggle", "put-back": "toggle", "enable-at": "move", "move-to": "move", stage: "stage", swap: "swap", tags: "tags", note: "note", folder: "folder", workshop: "workshop-id", steam: "steam" };
  if (verbs[code]) return verb(verbs[code]);
  const sel = S.focus < 3 ? selection() : (cur(lastList) ? [cur(lastList)] : []);
  const ids = idsOf(sel);
  if (!ids.length) return;
  const o = ops.op(code);
  const run = () => edited(api.RunOp(code, ids), code === "remove", ids);
  if (!o.confirm) return run();
  let p;
  try { p = await api.OpPreview(code, ids); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, o.label, p.apply ? run : null);
}

// delKey: Del unsubscribes the Workshop copy or deletes one of the player's own (asks first).
async function delKey() {
  const k = S.focus < 3 ? S.focus : lastList;
  const it = cur(k);
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
    if (NAV[+dig[1] - 1]) go(NAV[+dig[1] - 1]);
    return;
  }
  // tab / shift+tab: the next / previous page under the same sidebar entry (Mod 库存's 全部 and
  // 变化, the plans); the full detail keeps tab for its sub-sections (06 §3.5)
  // (on the pages with forms, settings and the like, tab keeps moving the focus)
  // Pages with adjustable cards keep native tab order to reach their gaps (06 §3.1).
  if (e.target.closest?.('[role="separator"]') && k === "Tab") return;
  if (k === "Tab" && !e.ctrlKey && !e.altKey && (S.screen === "changes" || S.screen === "packs")) { e.preventDefault(); return subPage(e.shiftKey ? -1 : 1); }
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
  if (k === "ArrowLeft" || k === "ArrowRight") { handled(); return focusArea(Math.max(0, Math.min(3, S.focus + (k === "ArrowRight" ? 1 : -1)))); }
  // PgUp / PgDn move the cursor a page (06 §3.3); in the details they scroll them
  if (k === "PageDown" || k === "PageUp") {
    handled();
    if (S.focus === 3) { scrollDetail(k === "PageDown" ? 1 : -1, true); return renderWhere(); }
    const box = $(`.area[data-area="${S.focus}"] .rows`);
    const page = Math.max(1, Math.floor(box.clientHeight / 28) - 1);
    return moveCursor(k === "PageDown" ? page : -page, e.shiftKey);
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

// Three columns: 未启用, 已启用, and 详情 with 暂存 and 历史 under it. split 0 is between the
// first two, split 1 before the third. Widths are fr values per environment.
const COLS = () => [$("#board > .fcol"), $('.area[data-area="1"]'), $("#board .stack")];

function wireSplits() {
  const board = $("#board");
  const key = () => "ui.gui.columns:" + ((S.board && S.board.env) || "");
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
  // Expanded staging/history share the detail column (06 §3.1, 06 §2.5).
  const stack = board.querySelector(".stack");
  stack.querySelectorAll(".fold").forEach((card, i) => {
    const gap = el("div", "hsplit stack-split");
    stack.insertBefore(gap, card);
    const set = (v) => {
      card.classList.toggle("stack-sized", !!v);
      if (v) card.style.setProperty("--stack-size", v + "px");
      else card.style.removeProperty("--stack-size");
    };
    wireSeparator(gap, {
      axis: "y", direction: -1, key: "ui.gui.plan." + (i ? "history" : "stage"),
      enabled: () => card.classList.contains("open") && !card.hidden,
      range: () => [72, stack.clientHeight - 148 - [...stack.querySelectorAll(".fold")]
        .filter((other) => other !== card).reduce((h, other) => h + other.getBoundingClientRect().height, 0)],
      get: () => card.getBoundingClientRect().height, set, reset: () => set(0), restore: (v) => set(+v || 0),
      changed: () => renderRows(2),
    });
  });
}

async function loadWidths() {
  const v = await api.Setting("ui.gui.columns:" + ((S.board && S.board.env) || ""));
  const board = $("#board");
  let ws = (v || "").split(",");
  // saved by the four-column board: 未启用, 已启用, 暂存, 详情 — 详情's is the third column's now
  if (ws.length === 4) ws = [ws[0], ws[1], ws[3]];
  ws.slice(0, 3).forEach((w, j) => { if (w) board.style.setProperty(`--c${j}`, w); });
}

// ---- start ----

const ui = {
  reload, refreshEnvironment, relang, render, pageEntering, pageEntered, renderWhere, renderHints, renderHeads, click, verb, drop, place, run, preview, cmd, step, search, searchKey, go, useEnv,
  palette: paletteChoice, selectionItems: selection, planMenu: (id, x, y) => plans.menuAt(id, x, y),
  menu: (x, y) => openRowMenu(x, y, false),
  detailFollow: (f) => detailFollow(f),
  newList: (from) => planManage.newList(null, from),
  selectedList: () => planManage.selected(),
  resumeGuide: () => guide.resume(),
  manageLists: (id = 0) => { planManage.select(id); return go("plans"); },
  afterImport: null, // the first-run guide's next step once an import is applied
};

async function start() {
  const s = await api.Init();
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
    column: () => $("#board > .fcol"), resized: () => renderRows(0), leave: () => focusArea(S.focus < 2 ? S.focus : 0),
    placeholder: T("search: words, tag:, author:, type:, has:, version:, -not") + "  /" });
  $("#board .barhost").replaceWith(S.bar.el);
  $('.area[data-area="0"] > .head').addEventListener("click", () => { if (S.bar.fills()) S.bar.toggle(false); });
  wireLists(ui);
  wireSplits();
  document.addEventListener("keydown", onKey);
  document.addEventListener("verb", (e) => { const v = e.detail; v === "needs" ? go("needs") : verb(v); });
  // The detail button always acts on the shown instance, independently of list selection.
  document.addEventListener("separate-mod", async (e) => {
    await edited(api.RunOp("separate", [e.detail]), false, [e.detail]);
    detailFollow(true);
  });
  // a detail's 在变化页查看全部: 变化 with only that mod's Lineage (06 §2.9)
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
  // a task's o on a mod: Mod 库存 searching for it (06 §2.16)
  document.addEventListener("library-search", async (e) => { shell.navigated(); await library.openSearch(e.detail); renderWhere(); });
  tasksPage.refreshFoot();
  on("guide", (ev) => guide.guideEvent(ev));
  on("download", (ev) => guide.downloadEvent(ev));
  // Another process (the TUI, the CLI, an agent) may have changed the data: look again
  // when the window gets the focus (G8).
  addEventListener("focus", async () => {
    try { if (S.board && (await api.Stamp()) !== S.board.stamp) await reload(); } catch { /* keep the page */ }
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
  } catch { /* the next poll tries again */ }
  setTimeout(pollLive, 2000);
}

start().catch((e) => say("fail", errText(e)));
