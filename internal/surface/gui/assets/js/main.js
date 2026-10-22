// RimArcho GUI (08). The page only lays out and maps input: what it shows comes from
// internal/view through the Go service, and the 06 §3 rules hold here too:
//   1. the arrows only move (the cursor, the focus, the scroll), never change data;
//   2. verbs act at once on the object (the selection, else the cursor's row), u undoes;
//   3. the position line always says where you are;
//   4. the hint line lists what you can do with the object now;
//   5. few popups, each closed with esc, never nested.
import { api, on, setText } from "./api.js";
import { S, $, $$, el, esc, T, Tf, errText, say, clearSay, col, cur, selection, key, idsOf, refsOf, named, AREAS } from "./state.js";
import { setItems, refilter, renderLists, renderHeads, renderRows, ensureVisible, wireLists, setOnHistory, stagingOpen, setStagingOpen, showGhost, acceptGhost, barOn } from "./lists.js";
import { createBar } from "./filterbar.js";
import { showDetail, scrollDetail } from "./detail.js";
import * as pop from "./popups.js";
import * as scr from "./screens.js";
import * as shell from "./shell.js";
import * as homePage from "./home.js";
import * as guide from "./guide.js";
import * as plans from "./plans.js";
import * as library from "./library.js";
import { initExchange, exchange } from "./exchange.js";
import { icon } from "./icons.js";
import * as motion from "./motion.js";
import * as ops from "./ops.js";
import * as uninst from "./uninstall.js";

// ---- loading ----

async function reload() {
  let b;
  try { b = await api.Board(); } catch (e) { say("fail", errText(e)); return; }
  S.board = b;
  S.busy.scan = b.scanning;
  S.busy.analyze = b.analyzing;
  // no environment yet: the first-run guide says so, not an error
  if (b.error && b.envs && b.envs.length) say("fail", b.error);
  const cols = b.cols || [[], [], []];
  const sh = shell.loadShell();
  try { S.history = await api.History(); } catch { S.history = null; }
  await sh;
  const keep = [0, 1, 2].map((k) => setItems(k, cols[k] || []));
  await Promise.all([0, 1, 2].map((k) => refilter(k, keep[k])));
  render();
  detailFollow();
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

function renderTop() {
  if (!S.board) return;
  $("#btn-launch").textContent = T("launch") + " ▶";
  $("#btn-launch").title = T("write the list to the game and start it") + " · L";
  syncLaunch();
}

// syncLaunch: launch sits in the top bar on the plan only, and not while a game runs.
function syncLaunch() {
  const l = S.live || {};
  $("#btn-launch").hidden = !!S.screen || !!l.running;
  // another environment's game runs: one game at a time (08 §1)
  $("#btn-launch").disabled = !!l.blocked;
  $("#btn-launch").title = (l.blocked || T("write the list to the game and start it")) + " · L";
}

// planActions: the plan's actions in the top bar (06 §2.5): undo, sort, build a pack, write
// to the game, and how many entries are not written to the game yet (06 §2.2); the plan's
// own menu (rename, copy, delete) and its "有未载入的外部版本" mark (04 §4.3).
function planActions() {
  const u = (S.board && S.board.unsaved) || 0;
  const btn = (label, k, run, cls = "") => {
    const b = el("button", "btn " + cls, label);
    if (k) { b.append(el("kbd", "", k)); b.title = k; }
    b.onclick = (e) => { e.stopPropagation(); run(); };
    return b;
  };
  const out = [];
  const plan = S.shell && S.shell.plan;
  if (plan && plan.conflict) {
    const m = el("button", "chip mark");
    m.append(el("span", "dot"), el("span", "t", plan.conflictText));
    m.title = plan.conflictText + " · " + T("can it run › needs your answer");
    m.onclick = (e) => { e.stopPropagation(); go("needs"); };
    out.push(m);
  }
  if (u) {
    const n = el("span", "unwritten", Tf("%d not written", u));
    n.title = T("different from the list last written to the game; launching or writing to the game writes them");
    motion.bump("unwritten", n, u);
    out.push(n);
  } else motion.bump("unwritten", null, 0);
  out.push(btn(T("undo"), "u", () => verb("undo"), "wide-only"), btn(T("import / export"), "i", () => exchange(0)),
    btn(T("build a pack"), "", () => go("packs"), "wide-only"), btn(T("write to the game"), "w", () => run("save"), u ? "hot" : ""));
  if (plan) {
    const more = btn("⋯", "", () => { const r = more.getBoundingClientRect(); plans.menuAt(plan.id, r.left, r.bottom + 4); }, "icon-btn");
    more.title = T("the plan: rename, copy, delete") + " · P";
    out.push(more);
  }
  return out;
}

let lastList = 1;

// The sidebar's entries in shift+1–7 order, 在玩 then 库 (06 §3.5; view.Nav).
const NAV = ["home", "needs", "log", "library", "plan", "packs", "env"];

// go opens a sidebar page; "plan" is the current plan's editor, "plan:<id>" a plan (it
// becomes current: no game file is written), "plan:new" the new-plan dialog, "pack:<name>"
// a pack on the packs page.
async function go(id) {
  if (uninst.locked()) return; // an uninstall runs, or has run (06 §2.14)
  shell.navigated();
  if (id === "plan:new") return plans.newPlan();
  if (id.startsWith("plan:")) {
    if (!(await plans.openPlan(+id.slice(5)))) return;
    id = "plan";
  }
  S.packFocus = id.startsWith("pack:") ? id.slice(5) : null;
  await openPage(id);
  // a page already shown is redrawn in place: its sub-entry and breadcrumb follow too
  renderWhere();
}

async function openPage(id) {
  if (S.packFocus) return scr.packs();
  switch (id) {
    case "plan":
      if (scr.closeScreen()) detailFollow(true);
      return renderWhere();
    case "home": return homePage.home();
    case "needs": return scr.needs();
    case "packs": return scr.packs();
    case "env": return scr.environment();
    case "changes": return scr.changes(S.screen === "changes" ? undefined : null);
    case "library": return library.open();
    case "log": return scr.gameLog(undefined, true);
    case "settings": return scr.settings();
    case "uninstall": return uninst.open();
    case "guide": return guide.start(S.board && S.board.envs && S.board.envs.length ? "env" : "first");
  }
}

// renderWhere: the position line (06 §3.2), the sidebar and the breadcrumb.
function renderWhere() {
  shell.sync();
  syncLaunch();
  if (!S.screen) shell.setActs("plan|" + ((S.board && S.board.unsaved) || 0) + "|" + T("undo") + "|" + JSON.stringify(S.shell && S.shell.plan), ...planActions());
  // A page's place is its breadcrumb; the position line says more only on the plan (where
  // the cursor is in which list) and in the full detail.
  let s = "";
  if (S.screen === "library") s = library.where(); // a table like the plan's lists
  else if (S.screen && S.screen !== "full") s = ""; // a page: the breadcrumb is enough
  else if (S.screen === "full") {
    const it = cur(lastList);
    s = `${T(AREAS[lastList])} › ${S.detail ? S.detail.name : ""}`;
    if (it) s += `（${col(lastList).cursor + 1} / ${col(lastList).view.length}）`;
    const sec = scr.fullSection();
    if (sec) s += " › " + sec;
  } else if (S.focus === 3) {
    const sc = $(".detail .scroll");
    const pct = sc.scrollHeight > sc.clientHeight ? Math.round(100 * sc.scrollTop / (sc.scrollHeight - sc.clientHeight)) : 100;
    s = `${T("details")} · ${S.detail ? S.detail.name : "—"} · ${pct}%`;
  } else {
    const c = col();
    s = `${T(AREAS[S.focus])} · ${c.view.length ? c.cursor + 1 : 0} / ${c.view.length}`;
    if (c.sel.size) s += " · " + Tf("%d selected", c.sel.size);
  }
  if (S.popup) s += " › " + S.popup.title;
  $("#where").textContent = s;
}

// actions: what can be done with the object now (the hint line and the context menu).
function actions() {
  if (S.focus > 2) return [
    { key: "PgDn", label: T("scroll") }, { key: "enter", label: T("full details"), run: () => verb("full") },
    { key: "ctrl+o", label: T("open the folder"), run: () => verb("folder") }, { key: "ctrl+w", label: T("Workshop page"), run: () => verb("workshop-id") },
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
  if (k === 1) out.push({ key: "J K", label: T("down / up"), run: null });
  out.push({ key: "g", label: T("tags"), run: () => verb("tags") });
  if (sel.length === 1) {
    out.push({ key: "enter", label: T("full details"), run: () => verb("full") });
    if (k === 1 && sel[0].id) out.push({ key: "x", label: T("swap for another copy"), run: () => verb("swap") });
  }
  return out;
}

function renderHints() {
  const h = $("#hints");
  h.innerHTML = "";
  let list = actions();
  if (S.screen === "full") list = [{ key: "esc", label: T("back") }];
  else if (S.screen === "library") list = library.hintList();
  else if (S.screen === "changes") list = scr.changesHints();
  else if (S.screen === "home") list = homePage.hintList();
  else if (S.screen) list = [{ key: "shift+1–7", label: T("pages") }, { key: "ctrl+k", label: T("commands") }, { key: "?", label: T("all keys") }];
  if (scr.changesFiltered()) list = [{ key: "esc", label: T("every mod") }, ...list];
  list = foldShortcuts(list);
  const c = S.focus < 3 ? col() : null;
  if (c && c.sel.size && !S.screen) h.append(el("span", "accent", Tf("%d selected", c.sel.size)), el("span", "sep", "·"));
  list.slice(0, 7).forEach((a, i) => {
    if (i) h.append(el("span", "sep", "·"));
    h.append(el("b", "", a.key), " " + a.label);
  });
  if (c && c.sel.size && !S.screen) h.append(el("span", "sep", "·"), el("b", "", "esc"), " " + T("clear the selection"));
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
    case "folder": case "workshop-id": {
      const it = S.focus < 3 ? cur() : cur(lastList);
      if (!it || !it.id) return say("", name === "folder" ? T("RimArcho has not seen this copy: no folder") : T("no Workshop ID: not a Workshop mod"));
      if (name === "folder") return api.OpenFolder(it.id).then(() => say("ok", T("opened the folder")), (e) => say("fail", errText(e)));
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
  if (o.refused) {
    say("fail", o.text);
    if (o.refused.length > 1) pop.text({ sort: T("sort"), save: T("write to the game"), launch: T("launch") }[what], o.refused);
  } else say("ok", o.text, !!o.undo);
  await reload();
  motion.play(before, null);
}

// preview: the dry run of sort, save or launch, to read (the palette's 预览…); nothing to confirm.
async function preview(what) {
  try { const p = await api.Preview(what); pop.text(p.title, p.lines); } catch (e) { say("fail", errText(e)); }
}

// cmd runs a palette command (view.Commands).
async function cmd(id) {
  // A page replaces the page shown once its data is in: the mod list never shows between.
  const pages = { home: "home", needs: "needs", logs: "log", changes: "changes", library: "library", log: "log", settings: "settings", packs: "packs", "env-page": "env", uninstall: "uninstall" };
  if (pages[id]) return go(pages[id]);
  // the list's own actions show the list; the rest work on any page
  if (["filter", "details", "tech", "tags"].includes(id) && scr.closeScreen()) renderWhere();
  switch (id) {
    case "sort": case "save": case "launch": return run(id);
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
    case "export": return exchange(1);
    case "help": return pop.help(S.screen || "plan");
    case "quit": return window.close();
  }
}

async function chooseEnv() {
  const envs = (S.board && S.board.envs) || [];
  if (!envs.length) return say("", T("no environments (archo env add)"));
  pop.choose(T("switch environment"), envs, async (i) => {
    try { say("ok", await api.UseEnv(envs[i])); } catch (e) { say("fail", errText(e)); }
    for (const c of S.cols) { c.query = ""; c.cursor = 0; c.sel.clear(); }
    await reload();
  }, Math.max(0, envs.indexOf(S.board.env)));
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
    try { say("ok", await api.UseEnv(o.id)); } catch (e) { say("fail", errText(e)); }
    return reload();
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
  const verbs = { enable: "toggle", disable: "toggle", "put-back": "toggle", "enable-at": "move", "move-to": "move", stage: "stage", swap: "swap", tags: "tags", note: "note", folder: "folder", workshop: "workshop-id" };
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
  if (e.isComposing) return;
  const k = e.key;
  // a menu opened with the mouse does not hold the focus: esc still closes it first (G5)
  if (k === "Escape" && !$("#menu").hidden) { e.preventDefault(); pop.hideMenu(); return; }
  if (pop.popupKey(e)) return;
  if (document.activeElement && document.activeElement.tagName === "INPUT") return;
  if (k !== "Shift" && k !== "Control" && k !== "Alt") clearSay();
  // shift+1–7: the sidebar's entries. By the physical key, so it works on any layout
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
  if (k === "Tab" && !e.ctrlKey && !e.altKey && (!S.screen || S.screen === "library" || S.screen === "changes")) { e.preventDefault(); return subPage(e.shiftKey ? -1 : 1); }
  if (S.screen) {
    // esc (and enter) leave the full detail for the plan; the sidebar's pages are side by
    // side, so esc does not go from one to another (06 §3.1)
    if (S.screen === "full" && (k === "Escape" || k === "Backspace" || k === "Enter")) { e.preventDefault(); scr.closeScreen(); detailFollow(true); return; }
    if (e.ctrlKey && k === "k") { e.preventDefault(); return pop.palette(); }
    if (k === "?" && S.screen !== "full") { e.preventDefault(); return pop.help(S.screen); }
    if (S.screen === "home") return homePage.homeKey(e);
    if (S.screen === "needs") return scr.needsKey(e);
    if (S.screen === "library") return library.key(e);
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
      o: () => verb("folder"), w: () => verb("workshop-id") }[k.toLowerCase()];
    if (m) { handled(); m(); }
    return;
  }
  if (e.altKey && (k === "J" || k === "j" || k === "K" || k === "k")) { handled(); return verb(k.toLowerCase() === "j" ? "down10" : "up10"); }
  const map = {
    " ": () => verb("toggle"), t: () => verb("stage"), m: () => verb("move"), J: () => verb("down"), K: () => verb("up"),
    g: () => verb("tags"), n: () => verb("note"), Enter: () => verb("full"), x: () => verb("swap"),
    S: () => run("sort"), w: () => run("save"), L: () => run("launch"), u: () => verb("undo"), T: () => verb(S.focus === 2 ? "restore-all" : "stage-all"),
    "/": () => startSearch(), "\\": () => S.bar.toggle(), c: () => go("changes"), l: () => go("log"), R: () => cmd("rescan"), A: () => cmd("analyze"),
    H: () => historyChooser(), C: () => changesOfCursor(), "<": () => widthKey(-1), ">": () => widthKey(1), "=": () => widthKey(0),
    E: () => go("env"), P: () => plans.chooser(), "?": () => pop.help("plan"), ":": () => commandLine(), i: () => exchange(0), e: () => exchange(1),
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
  $$(".split").forEach((sp) => {
    sp.addEventListener("mousedown", (e) => {
      e.preventDefault();
      const widths = COLS().map((a) => a.getBoundingClientRect().width);
      const lo = +sp.dataset.split, hi = lo + 1;
      const x0 = e.clientX;
      sp.classList.add("drag");
      const move = (ev) => {
        const dx = ev.clientX - x0;
        const w = widths.slice();
        w[lo] = Math.max(160, widths[lo] + dx);
        w[hi] = Math.max(160, widths[hi] - dx);
        // a hidden column (a narrow window) keeps a usable width for when it shows again
        w.forEach((v, j) => board.style.setProperty(`--c${j}`, (v || 220) + "fr"));
      };
      const up = () => {
        sp.classList.remove("drag");
        removeEventListener("mousemove", move);
        removeEventListener("mouseup", up);
        const vals = [0, 1, 2].map((j) => board.style.getPropertyValue(`--c${j}`)).join(",");
        api.SetSetting(key(), vals).catch(() => {});
        for (const k of [0, 1, 2]) renderRows(k);
      };
      addEventListener("mousemove", move);
      addEventListener("mouseup", up);
    });
    sp.addEventListener("dblclick", () => {
      [0, 1, 2].forEach((j) => board.style.removeProperty(`--c${j}`));
      api.SetSetting(key(), "").catch(() => {});
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
  reload, relang, renderWhere, renderHints, renderHeads, click, verb, drop, place, run, preview, cmd, step, search, searchKey, go,
  palette: paletteChoice, selectionItems: selection, planMenu: (id, x, y) => plans.menuAt(id, x, y),
  menu: (x, y) => openRowMenu(x, y, false),
  afterImport: null, // the first-run guide's next step once an import is applied
};

async function start() {
  const s = await api.Init();
  S.start = s;
  setText(s.text);
  document.documentElement.lang = s.lang;
  pop.initPopups(ui);
  scr.initScreens(ui);
  shell.initShell(ui);
  homePage.initHome(ui);
  guide.initGuide(ui);
  initExchange(ui);
  plans.initPlans(ui);
  library.initLibrary(ui);
  S.bar = createBar({ data: () => S.barData, change: () => barChanged(), splitKey: "ui.gui.filter.split:plan",
    column: () => $("#board > .fcol"), resized: () => renderRows(0), leave: () => focusArea(S.focus < 2 ? S.focus : 0),
    placeholder: T("search: words, tag:, author:, type:, has:, version:, -not") + "  /" });
  $("#board .barhost").replaceWith(S.bar.el);
  $('.area[data-area="0"] > .head').addEventListener("click", () => { if (S.bar.fills()) S.bar.toggle(false); });
  wireLists(ui);
  wireSplits();
  document.addEventListener("keydown", onKey);
  document.addEventListener("verb", (e) => { const v = e.detail; v === "needs" ? go("needs") : verb(v); });
  // a detail's 在变化页查看全部: 变化 with only that mod's Lineage (06 §2.9)
  document.addEventListener("changes-of", async (e) => { shell.navigated(); await scr.changes(e.detail); renderWhere(); renderHints(); });
  $("#btn-launch").onclick = () => run("launch");
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
  on("pack", (ev) => scr.packEvent(ev));
  on("uninstall", (ev) => uninst.event(ev));
  on("guide", (ev) => guide.guideEvent(ev));
  on("download", (ev) => guide.downloadEvent(ev));
  // Another process (the TUI, the CLI, an agent) may have changed the data: look again
  // when the window gets the focus (G8).
  addEventListener("focus", async () => {
    try { if (S.board && (await api.Stamp()) !== S.board.stamp) await reload(); } catch { /* keep the page */ }
  });
  await reload();
  requestAnimationFrame(() => api.Ready().catch(() => {}));
  pollLive();
  await loadWidths();
  // No environment yet: the first-run guide (06 §2.15); else the home page (06 §2.3).
  if (!S.shell || !S.shell.envs) { await guide.start("first"); shell.settle(); return; }
  await go("home");
  shell.settle();
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
    syncLaunch();
    if (JSON.stringify(l.items) !== before) { scr.liveChanged(); shell.sync(); }
    if (l.running) scr.logLive();
    if (was !== !!l.running) homePage.liveChanged();
  } catch { /* the next poll tries again */ }
  setTimeout(pollLive, 2000);
}

start().catch((e) => say("fail", errText(e)));
