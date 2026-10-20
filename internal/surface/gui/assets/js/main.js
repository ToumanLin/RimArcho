// RimArcho GUI (08). The page only lays out and maps input: what it shows comes from
// internal/view through the Go service, and the 06 §13 rules hold here too:
//   1. the arrows only move (the cursor, the focus, the scroll), never change data;
//   2. verbs act at once on the object (the selection, else the cursor's row), u undoes;
//   3. the position line always says where you are;
//   4. the hint line lists what you can do with the object now;
//   5. few popups, each closed with esc, never nested.
import { api, on, setText } from "./api.js";
import { S, $, $$, el, esc, T, Tf, errText, say, clearSay, col, cur, selection, key, idsOf, refsOf, named, AREAS } from "./state.js";
import { setItems, refilter, renderLists, renderHeads, renderRows, ensureVisible, wireLists } from "./lists.js";
import { showDetail, scrollDetail } from "./detail.js";
import * as pop from "./popups.js";
import * as scr from "./screens.js";

// ---- loading ----

async function reload() {
  let b;
  try { b = await api.Board(); } catch (e) { say("fail", errText(e)); return; }
  S.board = b;
  S.busy.scan = b.scanning;
  S.busy.analyze = b.analyzing;
  if (b.error) say("fail", b.error);
  const cols = b.cols || [[], [], []];
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

// ---- drawing ----

function render() {
  renderTop();
  renderLists();
  renderWhere();
  renderHints();
}

function renderTop() {
  const b = S.board;
  if (!b) return;
  $("#env").textContent = b.env ? `${b.env} · ${mm(b.gameVersion)}` : T("no environment");
  const n = $("#needs");
  n.textContent = b.needs ? Tf("%d to handle", b.needs) : T("nothing to handle");
  n.classList.toggle("none", !b.needs);
  $("#unsaved").textContent = b.unsaved ? Tf("%d not saved", b.unsaved) : "";
  $("#btn-sort").textContent = T("sort");
  $("#btn-save").textContent = T("save");
  $("#btn-launch").textContent = T("launch") + " ▶";
}

function mm(v) { const p = String(v || "").split("."); return p.length >= 2 ? p[0] + "." + p[1] : v || ""; }

let lastList = 1;

// renderWhere: the position line (06 §13.3).
function renderWhere() {
  let s = "";
  if (S.screen === "needs") s = "RimArcho › " + T("needs attention");
  else if (S.screen === "changes") s = "RimArcho › " + T("recent changes");
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
    { key: "o", label: T("Workshop page"), run: () => verb("workshop") }, { key: "#", label: T("tags"), run: () => verb("tags") },
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
  if (k === 1) out.push({ key: "J K", label: T("down / up"), run: null });
  out.push({ key: "#", label: T("tags"), run: () => verb("tags") });
  if (sel.length === 1) {
    out.push({ key: "enter", label: T("full details"), run: () => verb("full") });
    if (sel[0].id && S.detail && S.detail.url) out.push({ key: "o", label: T("Workshop page"), run: () => verb("workshop") });
    if (k === 1 && sel[0].id) out.push({ key: "x", label: T("swap for another copy"), run: () => verb("swap") });
  }
  return out;
}

function renderHints() {
  const h = $("#hints");
  h.innerHTML = "";
  let list = actions();
  if (S.screen) list = [{ key: "esc", label: T("back") }];
  const c = S.focus < 3 ? col() : null;
  if (c && c.sel.size && !S.screen) h.append(el("span", "accent", Tf("%d selected", c.sel.size)), el("span", "sep", "·"));
  list.slice(0, 7).forEach((a, i) => {
    if (i) h.append(el("span", "sep", "·"));
    h.append(el("b", "", a.key), " " + a.label);
  });
  if (c && c.sel.size && !S.screen) h.append(el("span", "sep", "·"), el("b", "", "esc"), " " + T("clear the selection"));
}

// ---- moving (navigation only: rule 1) ----

function focusArea(k) {
  if (k === S.focus) return;
  if (S.focus < 3 && k < 3 && col().sel.size) {
    col().sel.clear(); // switching to another list clears the selection (06 §13.5)
  }
  S.focus = k;
  if (k < 3) lastList = k;
  S.anchor = -1;
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

async function edited(p, undoable = true) {
  let r;
  try { r = await p; } catch (e) { say("fail", errText(e)); await reload(); return null; }
  const text = typeof r === "string" ? r : r && r.text;
  say("ok", text, undoable);
  S.placed = r && r.placed && r.placed.length ? r.placed : null;
  for (const c of S.cols) c.sel.clear();
  await reload();
  return r;
}

async function verb(name) {
  const k = S.focus < 3 ? S.focus : lastList;
  const sel = S.focus < 3 ? selection() : (cur(lastList) ? [cur(lastList)] : []);
  const ids = idsOf(sel);
  switch (name) {
    case "toggle":
      if (!sel.length) return;
      return edited(api.Toggle(k, ids, refsOf(sel)));
    case "stage":
      if (!ids.length) return;
      return edited(api.Stage(k, ids));
    case "move": {
      const placed = S.placed;
      if (placed) return pop.position({ ids: placed, what: placed.length === 1 ? nameOf(placed[0]) : named(placed.map((id) => ({ name: nameOf(id) }))) }, 1);
      if (!ids.length) return;
      return pop.position({ ids, what: named(sel) }, k);
    }
    case "down": case "up": case "down10": case "up10": {
      if (k !== 1 || !ids.length) return say("", T("only enabled mods can be moved"));
      const d = { down: 1, up: -1, down10: 10, up10: -10 }[name];
      return edited(api.MoveBy(ids, d));
    }
    case "tags":
      return pop.tags(sel);
    case "full":
      if (sel.length && sel[0].id) { lastList = k; return scr.full(); }
      return;
    case "workshop":
      if (S.detail && S.detail.url) return api.OpenURL(S.detail.url).then(() => say("ok", T("opened the Workshop page")), (e) => say("fail", errText(e)));
      return say("", T("not a workshop mod"));
    case "swap":
      if (sel.length === 1 && sel[0].id) return edited(api.Swap(sel[0].id));
      return;
    case "undo": return edited(api.Undo(), false);
    case "redo": return edited(api.Redo(), false);
    case "restore-all":
      if (!S.cols[2].items.length) return say("", T("staging is empty"));
      return edited(api.RestoreAll());
    case "needs": return scr.needs();
  }
}

function nameOf(id) {
  for (const c of S.cols) { const it = c.items.find((x) => x.id === id); if (it) return it.name; }
  return id;
}

async function place(from, obj, at) {
  await edited(api.Place(from, obj.ids, at));
}

// drop: dragging is another way to run a verb (08 §6).
async function drop(from, to, items, at) {
  const ids = idsOf(items);
  if (to === 1) return edited(api.Place(from, ids, at));
  if (from === to) return;
  if (to === 2) return edited(api.Stage(from, ids));
  if (to === 0 && from === 1) return edited(api.Toggle(1, ids, refsOf(items)));
  if (to === 0 && from === 2) return edited(api.Unstage(ids));
}

// preview: sort, save and launch show what they will do first (06 §9).
async function preview(what) {
  let p;
  try { p = await api.Preview(what); } catch (e) { return say("fail", errText(e)); }
  pop.confirm(p.title, p.lines, { sort: T("sort"), save: T("save to the game"), launch: T("launch") }[what], p.apply ? async () => {
    try { say("ok", await api.Apply(what), what === "sort"); } catch (e) { say("fail", errText(e)); }
    await reload();
  } : null);
}

// cmd runs a palette command (view.Commands).
async function cmd(id) {
  scr.closeScreen();
  switch (id) {
    case "sort": case "save": case "launch": return preview(id);
    case "undo": case "redo": case "restore-all": return verb(id);
    case "needs": return scr.needs();
    case "changes": return scr.changes();
    case "rescan":
      if (await api.Scan()) { S.busy.scan = true; say("busy", T("scanning…")); }
      return;
    case "analyze":
      if (await api.Analyze()) { S.busy.analyze = true; say("busy", T("checking the mods' files…")); }
      return;
    case "log": return scr.chooseLog();
    case "env": return chooseEnv();
    case "filter": return startSearch();
    case "details": return verb("full");
    case "tech": return verb("full");
    case "tags": return verb("tags");
    case "cli": return commandLine();
    case "help": return pop.help();
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
  if (o.kind === 2) {
    try { say("ok", await api.UseEnv(o.id)); } catch (e) { say("fail", errText(e)); }
    return reload();
  }
  // a mod: go to its list and row; a filter that hides it is cleared (06 §8)
  for (const k of [1, 0, 2]) {
    const c = S.cols[k];
    let i = c.items.findIndex((x) => x.id === o.id);
    if (i < 0) continue;
    let v = c.view.indexOf(i);
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

// ---- search (06 §13.8) ----

function startSearch(k = S.focus < 3 ? S.focus : lastList) {
  focusArea(k);
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
  renderHeads();
  renderRows(k);
  renderWhere();
  detailFollow();
}

function searchKey(k, e, inp) {
  if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); inp.value = ""; search(k, ""); col(k).searching = false; inp.blur(); renderHeads(); return; }
  if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); col(k).searching = false; inp.blur(); renderHeads(); return; }
  if (e.key === "Tab") {
    // tab goes to the next list with matches, carrying the query
    e.preventDefault();
    e.stopPropagation();
    const counts = col(k).others || [0, 0, 0];
    for (let d = 1; d <= 2; d++) {
      const j = (k + d) % 3;
      if (counts[j] > 0) {
        col(j).query = inp.value;
        inp.blur();
        col(k).searching = false;
        startSearch(j);
        search(j, inp.value);
        return;
      }
    }
    return;
  }
  if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); moveCursor(e.key === "ArrowDown" ? 1 : -1, false); }
  e.stopPropagation();
}

// ---- the keyboard ----

function onKey(e) {
  if (e.isComposing) return;
  const k = e.key;
  if (pop.popupKey(e)) return;
  if (document.activeElement && document.activeElement.tagName === "INPUT") return;
  if (k !== "Shift" && k !== "Control" && k !== "Alt") clearSay();
  if (S.screen) {
    if (k === "Escape" || k === "Backspace") { e.preventDefault(); scr.closeScreen(); detailFollow(true); return; }
    if (e.ctrlKey && k === "k") { e.preventDefault(); return pop.palette(); }
    if (S.screen === "needs") return scr.needsKey(e);
    if (S.screen === "changes") return scr.changesKey(e);
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
  if (k === "PageDown" || k === "PageUp") { handled(); scrollDetail(k === "PageDown" ? 1 : -1, true); return renderWhere(); }
  if (k === "Home" || k === "End") { handled(); if (S.focus < 3) moveCursor(k === "End" ? 1e9 : -1e9, e.shiftKey); return; }
  if (!ctrl && "1234".includes(k) && k.length === 1) { handled(); return focusArea(+k - 1); }
  if (k === "Tab") { handled(); return focusArea((S.focus + (e.shiftKey ? 3 : 1)) % 4); }
  if (k === "Escape") {
    handled();
    if (S.focus < 3 && col().sel.size) { col().sel.clear(); renderLists(); renderWhere(); renderHints(); }
    else if (S.focus < 3 && col().query) { col().query = ""; $(`.area[data-area="${S.focus}"] .search input`).value = ""; search(S.focus, ""); }
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
    const m = { k: () => pop.palette(), f: () => startSearch(), z: () => verb("undo"), y: () => verb("redo"), r: () => verb("redo") }[k.toLowerCase()];
    if (m) { handled(); m(); }
    return;
  }
  if (e.altKey && (k === "J" || k === "j" || k === "K" || k === "k")) { handled(); return verb(k.toLowerCase() === "j" ? "down10" : "up10"); }
  const map = {
    " ": () => verb("toggle"), t: () => verb("stage"), m: () => verb("move"), J: () => verb("down"), K: () => verb("up"),
    "#": () => verb("tags"), Enter: () => verb("full"), o: () => verb("workshop"), x: () => verb("swap"),
    S: () => preview("sort"), w: () => preview("save"), L: () => preview("launch"), u: () => verb("undo"), T: () => verb("restore-all"),
    "/": () => startSearch(), "!": () => scr.needs(), c: () => scr.changes(), R: () => cmd("rescan"), A: () => cmd("analyze"),
    E: () => chooseEnv(), "?": () => pop.help(), ":": () => commandLine(),
  };
  if (map[k]) { handled(); map[k](); }
}

// ---- the column widths (08 §4) ----

function wireSplits() {
  const board = $("#board");
  const key = () => "ui.gui.columns:" + ((S.board && S.board.env) || "");
  $$(".split").forEach((sp) => {
    sp.addEventListener("mousedown", (e) => {
      e.preventDefault();
      const areas = $$("#board .area");
      const widths = areas.map((a) => a.getBoundingClientRect().width);
      const i = +sp.dataset.split;
      const x0 = e.clientX;
      sp.classList.add("drag");
      const move = (ev) => {
        const dx = ev.clientX - x0;
        const w = widths.slice();
        w[i] = Math.max(160, widths[i] + dx);
        w[i + 1] = Math.max(160, widths[i + 1] - dx);
        w.forEach((v, j) => board.style.setProperty(`--c${j}`, v + "fr"));
      };
      const up = () => {
        sp.classList.remove("drag");
        removeEventListener("mousemove", move);
        removeEventListener("mouseup", up);
        const vals = [0, 1, 2, 3].map((j) => board.style.getPropertyValue(`--c${j}`)).join(",");
        api.SetSetting(key(), vals).catch(() => {});
        for (const k of [0, 1, 2]) renderRows(k);
      };
      addEventListener("mousemove", move);
      addEventListener("mouseup", up);
    });
    sp.addEventListener("dblclick", () => {
      [0, 1, 2, 3].forEach((j) => board.style.removeProperty(`--c${j}`));
      api.SetSetting(key(), "").catch(() => {});
    });
  });
}

async function loadWidths() {
  const v = await api.Setting("ui.gui.columns:" + ((S.board && S.board.env) || ""));
  const board = $("#board");
  (v || "").split(",").forEach((w, j) => { if (w) board.style.setProperty(`--c${j}`, w); });
}

// ---- start ----

const ui = {
  reload, renderWhere, renderHints, renderHeads, click, verb, drop, place, preview, cmd, step, search, searchKey,
  palette: paletteChoice, selectionItems: selection,
  menu: (x, y) => pop.menu(x, y, actions().filter((a) => a.run)),
};

async function start() {
  const s = await api.Init();
  S.start = s;
  setText(s.text);
  document.documentElement.lang = s.lang;
  pop.initPopups(ui);
  scr.initScreens(ui);
  wireLists(ui);
  wireSplits();
  document.addEventListener("keydown", onKey);
  document.addEventListener("verb", (e) => { const v = e.detail; v === "needs" ? scr.needs() : verb(v); });
  $("#needs").onclick = () => scr.needs();
  $("#env").onclick = () => chooseEnv();
  for (const b of $$("#top .tb")) b.onclick = () => preview(b.dataset.cmd);
  $(".detail .scroll").addEventListener("scroll", () => { if (S.focus === 3) renderWhere(); }, { passive: true });
  $(".detail").addEventListener("mousedown", () => focusArea(3));
  on("scan", async (ev) => {
    S.busy.scan = false;
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    await reload();
  });
  on("analyze", async (ev) => {
    if (ev.line) { S.busy.analyze = true; say("busy", T("checking the mods' files…") + " " + ev.line); return; }
    S.busy.analyze = false;
    say(ev.ok ? "ok" : "fail", ev.ok ? T("analysis done") : ev.error);
    await reload();
    scr.analyzeEvent(ev);
  });
  // Another process (the TUI, the CLI, an agent) may have changed the data: look again
  // when the window gets the focus (G8).
  addEventListener("focus", async () => {
    try { if (S.board && (await api.Stamp()) !== S.board.stamp) await reload(); } catch { /* keep the page */ }
  });
  await reload();
  await loadWidths();
  if (S.board && S.board.neverScanned) cmd("rescan");
}

start().catch((e) => say("fail", errText(e)));
