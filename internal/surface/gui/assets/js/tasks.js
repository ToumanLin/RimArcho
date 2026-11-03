// Background tasks (06 §2.16): what RimArcho does in the background, what waits and what each item
// came to. Like Mod library: the filter bar and the tasks down the left; on the right the task's
// summary, then a table of every item it touched (06 §3.9: the headers sort, move, size and
// pick columns). Not a popup: the work goes on while the player is on other pages, and the
// sidebar's foot line and ctrl+j bring them back here. "tasks" events say the record
// changed; a lane task that ended also brings its result for the status line.
import { progress } from "./feedbacksemantics.js";
import { pathContent } from "./fields.js";
import { factText, S, $, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { beginPageRead, page, act as pageAct, area } from "./screens.js";
import { createBar } from "./filterbar.js";
import * as pop from "./popups.js";
import { icon } from "./icons.js";
import { still } from "./motion.js";
import { createTable, mergeColumns } from "./datatable.js";
import { wireSeparator } from "./separator.js";

const COLS_KEY = "ui.gui.tasks.items.columns";
const SIDE_KEY = "ui.gui.tasks.side";
const SPLIT_KEY = "ui.gui.tasks.split";
// the items table's columns: key, title, width (name takes what is left), shown by default,
// the natural direction (1: A to Z, -1: biggest first) and how to compare
const COLS = [
  { key: "name", title: "mod", w: 150, def: true, fixed: true, dir: 1, cmp: (a, b) => a.name.localeCompare(b.name) },
  { key: "state", title: "result", w: 160, def: true, dir: 1, cmp: (a, b) => a.rank - b.rank },
  { key: "why", title: "reason", w: 220, def: true, dir: 1, cmp: (a, b) => (b.why ? 1 : 0) - (a.why ? 1 : 0) || a.why.localeCompare(b.why) },
  { key: "size", title: "size", w: 76, def: true, dir: -1, num: true, cmp: (a, b) => a.bytes - b.bytes },
  { key: "ref", title: "location", w: 200, def: false, dir: 1, cmp: (a, b) => a.ref.localeCompare(b.ref) },
];
const GROUPS = ["run", "queue", "today", "yesterday", "earlier"];
const GROUP_TITLE = { run: "running", queue: "queued", today: "today", yesterday: "yesterday", earlier: "earlier" };

const X = {
  page: null,
  cur: null,          // the task under the cursor (its id)
  sel: new Set(),     // selected tasks
  anchor: null,
  focus: "list",      // "list" or "items"
  icur: 0,            // the cursor in the items table
  cols: null,         // [{key, shown, w}] in the player's order
  sort: "state", dir: 1,
  bar: null,
  from: null,         // the page ctrl+j came from (again goes back)
};
let ui = null;
let els = null;

export function initTasks(u) { ui = u; }

// ---- the sidebar's foot line (06 §2.1): always there, where it is; it says more while
// something runs, waits or failed ----
export function foot(p) {
  const b = $("#tasks-line");
  if (!b) return;
  b.hidden = false;
  b.classList.toggle("on", S.screen === "tasks");
  const busy = !!(p && (p.running || p.queued));
  const sig = p ? [p.foot, p.pct, p.unseen, p.running, p.queued].join("|") : "";
  if (b.dataset.sig === sig && b.firstChild) return;
  b.dataset.sig = sig;
  b.innerHTML = "";
  const ic = el("span", "ic");
  if (busy) {
    const f = (p.pct / 100) * 34.6;
    ic.innerHTML = `<svg class="ring" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.5"/><circle class="v" cx="8" cy="8" r="5.5" style="stroke-dasharray:${f.toFixed(1)} 40"/></svg>`;
  } else ic.innerHTML = icon.tasks;
  b.append(ic, el("span", "lbl word-facts", busy ? p.foot : T("background tasks")));
  if (p && p.running) b.append(el("span", "cnt num", p.pct + "%"));
  if (p && p.unseen) b.append(el("span", "badge-n red num", String(p.unseen)));
  b.title = T("background tasks") + T("; ") + "ctrl+j";
}

export async function refreshFoot() {
  try { const p = await api.Tasks(); foot(p); return p; } catch { return null; }
}

// event: the record changed; a lane task that ended says so on the status line.
export async function event(ev) {
  if (ev && ev.ended) {
    say(ev.ended.ok ? "ok" : "fail", ev.ended.text + (ev.ended.ok || S.screen === "tasks" ? "" : ", ctrl+j"));
    document.dispatchEvent(new CustomEvent("task-ended", { detail: ev.ended }));
  }
  if (S.screen === "tasks") return load();
  refreshFoot();
}

// toggle: ctrl+j opens the page, and from it goes back where it came from.
export function toggle() {
  if (S.screen === "tasks" && X.from) return ui.go(X.from);
  X.from = S.screen ? (S.screen === "full" ? "plan" : S.screen) : "plan";
  return open();
}

export async function open() {
  if (S.screen !== "tasks") X.focus = "list";
  const read = beginPageRead("tasks", { retry: open });
  await load(read);
  api.TasksSeen().then(refreshFoot).catch(() => {});
}

async function load(read = null) {
  if (!read && S.screen !== "tasks") return;
  read ||= beginPageRead("tasks", { retry: () => load() });
  let p, saved;
  try {
    [p, saved] = await Promise.all([api.Tasks(), X.cols ? null : api.Setting(COLS_KEY).catch(() => "")]);
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  foot(p);
  p.rows ||= [];
  X.page = p;
  if (!X.cols) X.cols = mergeCols(saved);
  if (!els || !document.body.contains(els.root)) build();
  draw();
  read.finish();
}

const table = createTable({ key: COLS_KEY, columns: COLS.map((d) => ({ ...d, shown: d.def })), state: X,
  redraw: () => drawItems(), focus: () => focusItems(),
});
function mergeCols(saved) { return mergeColumns(COLS.map((d) => ({ ...d, shown: d.def })), saved); }
const colDef = (k) => COLS.find((c) => c.key === k);

// ---- the filter bar: state, queue, time; the search matches names, results and items ----
function bar() {
  if (!X.bar) {
    X.bar = createBar({
      cls: "lib-fbar",
      splitKey: "ui.gui.filter.split:tasks",
      column: () => els.side,
      data: () => barData(),
      change: () => draw(),
      leave: () => focusList(),
      placeholder: T("search tasks and mod names") + "  /",
    });
  }
  return X.bar;
}

const FACETS = [
  { key: "state", title: "state", of: (r) => r.state, label: (r) => r.stateText },
  { key: "lane", title: "queue", of: (r) => r.lane, label: (r) => r.laneText },
  { key: "time", title: "time", of: (r) => (r.group === "run" || r.group === "queue" ? "now" : r.group), label: (r) => T(r.group === "run" || r.group === "queue" ? "now" : r.group) },
];
function barData() {
  const rows = (X.page && X.page.rows) || [];
  const b = X.bar;
  if (!b) return {}; // the bar draws once while it is being made
  const on =(f) => (b.filters.includes(f) ? 1 : b.filters.includes("-" + f) ? -1 : 0);
  const groups = FACETS.map((g) => {
    const vals = new Map();
    for (const r of rows) {
      const v = g.of(r);
      if (!vals.has(v)) vals.set(v, { label: g.label(r), filter: g.key + ":" + v, count: 0 });
      vals.get(v).count++;
    }
    return { key: g.key, title: T(g.title), values: [...vals.values()].map((v) => ({ ...v, on: on(v.filter) })) };
  });
  const conds = b.filters.map((f) => {
    const ex = f.startsWith("-"), [k, v] = f.replace(/^-/, "").split(":");
    const g = FACETS.find((x) => x.key === k);
    const r = rows.find((x) => g && g.of(x) === v);
    return { filter: f, exclude: ex, label: r ? g.label(r) : v };
  });
  return { groups, conds };
}
function shown() {
  const rows = (X.page && X.page.rows) || [];
  if (!X.bar) return rows;
  const q = X.bar.search.trim().toLowerCase();
  const keep = {}, drop = {};
  for (const f of X.bar.filters) {
    const ex = f.startsWith("-"), [k, v] = f.replace(/^-/, "").split(":");
    (ex ? drop : keep)[k] = (ex ? drop : keep)[k] || new Set();
    (ex ? drop : keep)[k].add(v);
  }
  return rows.filter((r) => {
    if (q && !q.split(/\s+/).every((w) => r.search.includes(w))) return false;
    for (const g of FACETS) {
      const v = g.of(r);
      if (keep[g.key] && !keep[g.key].has(v)) return false;
      if (drop[g.key] && drop[g.key].has(v)) return false;
    }
    return true;
  });
}

// ---- the page ----
function build() {
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("fill tasks", "",
    pageAct(T("take the done ones off the record"), "ok", () => run("clear", []), "C"));
  const root = el("div", "tk");
  const side = el("div", "fcol tk-side");
  const list = el("section", "area tk-list");
  const lhead = el("div", "head");
  const rows = el("div", "tk-rows");
  rows.tabIndex = -1;
  list.append(lhead, rows);
  side.append(bar().el, list);
  lhead.addEventListener("click", () => { if (X.bar.fills()) X.bar.toggle(false); });
  const split = el("div", "split tk-split");
  const right = el("div", "tk-right");
  const sum = el("section", "area tk-sum");
  const hsplit = el("div", "hsplit tk-hsplit");
  const items = el("section", "area tk-items");
  const ihead = el("div", "head");
  const iscroll = el("div", "lib-rows tk-iscroll");
  iscroll.tabIndex = -1;
  const th = el("div", "lib-th tk-th");
  const ibody = el("div", "tk-ibody");
  iscroll.append(th, ibody);
  const none = el("div", "tk-none");
  items.append(ihead, iscroll, none);
  right.append(sum, hsplit, items);
  root.append(side, split, right);
  w.append(root);
  sc.append(w);
  els = { root, side, list, lhead, rows, right, sum, items, ihead, iscroll, th, ibody, none };
  const row = X.bar.el.querySelector(".fbar-row");
  if (!row.querySelector(":scope > .anum")) row.prepend(el("span", "anum", "1"));
  area(X.bar.el, null, () => X.bar.focus());
  area(list, null, () => focusList());
  area(items, null, () => focusItems());
  wireGaps(root, split, right, hsplit);
  wireList(rows);
  wireItems();
  rows.addEventListener("mousedown", () => focusList(true));
  iscroll.addEventListener("mousedown", () => focusItems(true));
  sum.addEventListener("click", (e) => { const b = e.target.closest("[data-act]"); if (b) run(b.dataset.act, targets()); });
}

function focusList(quiet) {
  X.focus = "list";
  if (!quiet && els) els.rows.focus();
  mark();
}
function focusItems(quiet) {
  X.focus = "items";
  if (!quiet && els) els.iscroll.focus();
  mark();
}
function mark() {
  if (!els) return;
  els.list.classList.toggle("focus", X.focus === "list");
  els.items.classList.toggle("focus", X.focus === "items");
  ui.renderHints();
  ui.renderWhere();
}

// ---- the gaps (06 §2.16): left | right, and the summary over the items ----
function wireGaps(root, split, right, hsplit) {
  const setSide = (v) => { if (v) root.style.setProperty("--tk-side", v + "px"); else root.style.removeProperty("--tk-side"); };
  const setTop = (v) => { if (v) right.style.setProperty("--tk-top", v + "px"); else right.style.removeProperty("--tk-top"); };
  wireSeparator(split, {
    key: SIDE_KEY, range: () => [220, root.clientWidth - 420],
    get: () => els.side.getBoundingClientRect().width, set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v || 0),
  });
  wireSeparator(hsplit, {
    axis: "y", key: SPLIT_KEY, range: () => [90, right.clientHeight - 140],
    get: () => els.sum.getBoundingClientRect().height, set: setTop, reset: () => setTop(0),
    restore: (v) => setTop(+v || 0),
  });
}

// ---- drawing ----
function draw() {
  if (!els || !X.page) return;
  X.bar.draw();
  const list = shown();
  if (!list.find((r) => r.id === X.cur)) X.cur = list[0] ? list[0].id : null;
  for (const id of [...X.sel]) if (!list.find((r) => r.id === id)) X.sel.delete(id);
  drawList(list);
  drawSum();
  drawItems();
  ui.renderHints();
  ui.renderWhere();
}

function drawList(list) {
  els.lhead.innerHTML = "";
  els.lhead.append(el("span", "anum", "2"), el("span", "t", T("tasks")), el("span", "num", String(list.length)));
  if (X.bar.active()) els.lhead.append(el("span", "q", T("filtered")));
  const before = still() ? null : rects();
  const box = els.rows;
  const keep = new Map([...box.querySelectorAll(".tk-row")].map((e) => [e.dataset.id, e]));
  const frag = document.createDocumentFragment();
  let last = "", groupLabel;
  for (const r of list) {
    if (r.group !== last) {
      const n = list.filter((x) => x.group === r.group).length;
      const g = el("div", "group tk-group");
      groupLabel = el("span", "", T(GROUP_TITLE[r.group]));
      groupLabel.id = "task-group-" + r.group;
      g.append(groupLabel, el("span", "n", String(n)));
      frag.append(g);
      last = r.group;
    }
    let e = keep.get(r.id);
    if (!e) { e = el("div", "tk-row"); e.dataset.id = r.id; }
    e.className = "tk-row st-" + r.state + (r.id === X.cur ? " cur" : "") + (X.sel.has(r.id) ? " sel" : "");
    const sig = JSON.stringify([r.title, r.obj, r.time, r.line, r.count, r.pct, r.state]);
    if (e.dataset.sig !== sig) {
      e.dataset.sig = sig;
      e.innerHTML = "";
      const l1 = el("div", "l1");
      l1.id = "task-title-" + r.id;
      l1.append(el("span", "t", r.title), el("span", "o", r.obj));
      e.setAttribute("role", "group");
      for (const [i, text] of [...l1.children].entries()) text.id = l1.id + "-" + i;
      e.setAttribute("aria-labelledby", [...l1.children].map(n => n.id).join(" "));
      const l2 = el("div", "l2");
      if (r.group === "run") {
        l2.append(mini(r, l1), el("span", "pn num", r.count || r.pct + "%"));
      } else if (r.group !== "queue") {
        const state = stateWord(r); state.id = "task-state-" + r.id;
        l2.append(state);
      }
      l2.append(el("span", "txt word-facts", r.line || ""));
      e.append(l1, el("span", "tm num", r.time || ""), l2);
    }
    e.setAttribute("aria-describedby", r.group === "run" || r.group === "queue" ? groupLabel.id : "task-state-" + r.id);
    frag.append(e);
  }
  if (!list.length) frag.append(el("div", "empty-state", X.page.rows.length ? T("no task matches the filter, esc clears it") : T("nothing has run in the background yet")));
  box.innerHTML = "";
  box.append(frag);
  if (before) slide(before);
  const c = box.querySelector(".tk-row.cur");
  if (c) c.scrollIntoView({ block: "nearest" });
}
function rects() {
  const m = new Map();
  for (const e of els.rows.querySelectorAll(".tk-row")) m.set(e.dataset.id, e.getBoundingClientRect().top);
  return m;
}
// a task that changed groups slides to its new place (FLIP, transform only; 06 §7.1)
function slide(before) {
  for (const e of els.rows.querySelectorAll(".tk-row")) {
    const y = before.get(e.dataset.id);
    if (y == null) continue;
    const d = y - e.getBoundingClientRect().top;
    if (Math.abs(d) < 2) continue;
    e.animate([{ transform: `translateY(${d}px)` }, { transform: "none" }], { duration: 260, easing: "cubic-bezier(.2,.8,.2,1)" });
  }
}
function taskProgress(node, r, label) {
  const count = /^(\d+)\s*\/\s*(\d+)$/.exec(r.count || "");
  return progress(node, [...label.children], count ? +count[1] : r.pct > 0 || r.state === "done" ? r.pct : undefined, count ? +count[2] : 100, r.count || "");
}
function mini(r, label) {
  const m = el("span", "mini s-" + r.state);
  const i = el("i");
  i.style.width = r.pct + "%";
  m.append(i);
  return taskProgress(m, r, label);
}
function stateWord(r) {
  const s = el("span", "stw s-" + r.state);
  const mk = { done: icon.ok, fail: icon.err, part: icon.warn }[r.state];
  if (mk) { const k = el("span", "mk"); k.innerHTML = mk; s.append(k); }
  s.append(r.stateText);
  return s;
}
const current = () => (X.page ? X.page.rows.find((r) => r.id === X.cur) : null);
function targets() {
  const rows = (X.page && X.page.rows) || [];
  if (X.sel.size) return rows.filter((r) => X.sel.has(r.id));
  const c = current();
  return c ? [c] : [];
}

function drawSum() {
  const r = current(), box = els.sum;
  const sig = r ? JSON.stringify([r.id, r.title, r.obj, r.state, r.say, r.pct, r.count, r.dur, r.steps, r.acts, r.time]) : "";
  if (box.dataset.sig === sig) return;
  box.dataset.sig = sig;
  box.innerHTML = "";
  if (!r) { box.append(el("div", "empty-state", T("pick a task"))); return; }
  const top = el("div", "top");
  const h = el("h3");
  h.append(el("span", "t", r.title));
  h.append(el("span", "o", r.obj));
  top.append(h);
  const summary = el("p", "say" + (r.kind === "subscribe" ? "" : " word-facts") + " s-" + r.state, r.say);
  if (r.kind === "subscribe") summary.style.whiteSpace = "pre-line";
  box.append(top, summary);
  if (r.bar) {
    const p = el("div", "prog");
    const b = el("div", "bar" + (r.state === "fail" ? " bad" : "") + (r.state === "run" ? " busy" : ""));
    const i = el("i");
    i.style.width = r.pct + "%";
    b.append(i);
    taskProgress(b, r, h);
    p.append(b);
    box.append(p);
  }
  if (r.steps && r.steps.length) {
    const st = el("div", "tsteps");
    r.steps.forEach((s, i) => {
      const x = el("span", "tstep " + s.state);
      x.append(el("span", "n", s.state === "ok" ? "✓" : s.state === "fail" ? "!" : String(i + 1)), s.label);
      st.append(x);
    });
    box.append(st);
  }
  const acts = el("div", "acts");
  for (const a of r.acts) {
    const b = el("button", "btn" + (a.primary ? " primary" : "") + (a.danger ? " danger" : ""), a.label);
    b.dataset.act = a.id;
    b.append(keyCaps(a.key));
    acts.append(b);
  }
  box.append(acts);
  const facts = el("div", "facts");
  const fact = (k, v) => { if (!v) return; const s = el("span", "", k); s.append(el("b", " word-facts", v)); facts.append(s); };
  fact(T("queue"), r.laneText + "  " + r.laneNote);
  fact(T("started"), r.time);
  fact(T("took"), r.dur);
  box.append(facts);
}

// ---- the items table (06 §3.9) ----
function itemRows() {
  const r = current();
  const list = [...((r && r.items) || [])];
  const d = colDef(X.sort);
  if (d) list.sort((a, b) => d.cmp(a, b) * X.dir * d.dir || a.name.localeCompare(b.name));
  return list;
}
function drawItems() {
  const r = current();
  const has = !!(r && r.items && r.items.length);
  els.ihead.innerHTML = "";
  els.ihead.append(el("span", "anum", "3"), el("span", "t", T("every item")), el("span", "num", has ? String(r.items.length) : ""));
  if (has) els.ihead.append(el("span", "r word-facts", r.counts));
  els.iscroll.hidden = !has;
  els.none.hidden = has;
  if (!has) {
    factText(els.none, r ? [r.noItems, r.line] : []);
    if (r) {
      const g = r.acts.find((a) => a.id === "goto");
      if (g) { const b = el("button", "btn", g.label); b.append(keyCaps(g.key)); b.onclick = () => run("goto", [r]); els.none.append(el("br"), b); }
    }
    return;
  }
  const vis = X.cols.filter((c) => c.shown);
  els.ihead.querySelector(".t").id = "task-items-heading";
  table.draw(els.iscroll, els.th, { role: "grid", labelledby: "task-items-heading", count: r.items.length });
  const rows = itemRows();
  X.icur = Math.max(0, Math.min(X.icur, rows.length - 1));
  els.ibody.innerHTML = "";
  rows.forEach((it, i) => {
    const e = el("div", "tk-irow st-" + it.state + (i === X.icur ? " cur" : ""));
    e.dataset.i = i;
    for (const c of vis) {
      const d = colDef(c.key);
      const cell = el("div", "c c-" + c.key + (d.num ? " r" : ""));
      if (c.key === "name") cell.textContent = it.name;
      else if (c.key === "state") cell.append(itemState(it));
      else if (c.key === "why") { pathContent(cell, it.why || "—"); cell.title = it.why || ""; }
      else if (c.key === "size") cell.textContent = it.size || "—";
      else pathContent(cell, it.ref);
      e.append(cell);
    }
    for (const bar of e.querySelectorAll(".tk-dl")) progress(bar, e.querySelector(".c-name") || bar, it.got || 0, it.downloadSize, bar.title);
    table.row(e, i);
    els.ibody.append(e);
  });
}
function itemState(it) {
  const map = { ok: "done", fail: "fail", run: "run", wait: "queue", skip: "stop" };
  const s = el("span", "stw s-" + map[it.state]);
  const mk = { ok: icon.ok, fail: icon.err }[it.state];
  if (mk) { const k = el("span", "mk"); k.innerHTML = mk; s.append(k); }
  s.append(it.stateText);
  // a Steam download in progress: its bytes as a bar (got / downloadSize)
  if (it.state === "run" && it.downloadSize > 0) {
    const pct = Math.max(0, Math.min(100, (100 * (it.got || 0)) / it.downloadSize));
    const bar = el("span", "prog tk-dl");
    const b = el("span", "bar"), fill = el("i");
    fill.style.width = pct.toFixed(1) + "%";
    b.append(fill);
    bar.append(b);
    bar.title = `${fmtBytes(it.got || 0)} / ${fmtBytes(it.downloadSize)}`;
    s.append(bar);
  }
  return s;
}
function fmtBytes(n) {
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i ? n.toFixed(1) : String(n)) + " " + u[i];
}
function wireItems() {
  els.ibody.addEventListener("mousedown", (e) => {
    const r = e.target.closest(".tk-irow");
    if (!r) return;
    X.icur = +r.dataset.i;
    drawItems();
  });
  els.ibody.addEventListener("contextmenu", (e) => {
    const r = e.target.closest(".tk-irow");
    if (!r) return;
    e.preventDefault();
    X.icur = +r.dataset.i;
    drawItems();
    itemMenu(e.clientX, e.clientY);
  });
}
const curItem = () => itemRows()[X.icur];
function itemMenu(x, y, keys) {
  const it = curItem(), t = current();
  if (!it || !t) return;
  const acts = [
    { label: T("see it in the mod library"), key: "o", run: () => toLibrary(it.name) },
    { label: T("open the folder"), key: "ctrl+o", run: () => run("folder", [t], it.ref) },
  ];
  if (it.workshopURL) acts.push({ label: T("open the Workshop page"), run: () => api.OpenURL(it.workshopURL).catch((e) => say("fail", errText(e))) });
  if (it.state === "fail" && !it.permanent) acts.push({ sep: true }, { label: T("try only this one again"), key: "r", run: () => run("retry", [t], it.ref) });
  acts.push({ label: T("copy the name"), key: "ctrl+c", run: () => copy(it.name) });
  pop.menu(x, y, acts, keys);
}
function copy(text) {
  navigator.clipboard.writeText(text).then(() => say("ok", Tf("copied %s", text))).catch(() => {});
}
function toLibrary(name) {
  document.dispatchEvent(new CustomEvent("library-search", { detail: name || "" }));
}

// ---- the task list: click, shift+click, ctrl+click, right click, double click ----
function wireList(rows) {
  rows.addEventListener("mousedown", (e) => {
    const r = e.target.closest(".tk-row");
    if (!r || e.button !== 0) return;
    const id = r.dataset.id;
    if (e.ctrlKey) X.sel.has(id) ? X.sel.delete(id) : X.sel.add(id);
    else if (e.shiftKey && X.cur) {
      const ids = shown().map((x) => x.id), a = ids.indexOf(X.cur), b = ids.indexOf(id);
      ids.slice(Math.min(a, b), Math.max(a, b) + 1).forEach((x) => X.sel.add(x));
    }
    moveTo(id);
  });
  rows.addEventListener("dblclick", (e) => { if (e.target.closest(".tk-row")) focusItems(); });
  rows.addEventListener("contextmenu", (e) => {
    const r = e.target.closest(".tk-row");
    e.preventDefault();
    if (!r) return;
    if (!X.sel.has(r.dataset.id)) X.sel.clear();
    moveTo(r.dataset.id);
    taskMenu(e.clientX, e.clientY);
  });
}
function moveTo(id) {
  if (id !== X.cur) X.icur = 0;
  X.cur = id;
  draw();
}
function taskMenu(x, y, keys) {
  const r = current();
  if (!r) return;
  const acts = [];
  for (const a of r.acts) {
    if (a.id === "forget") acts.push({ sep: true });
    acts.push({ label: a.label, key: a.key, cls: a.danger ? "danger" : "", run: () => run(a.id, targets()) });
  }
  if (r.items && r.items.length) acts.unshift({ label: T("every item"), key: "enter", run: () => focusItems() }, { sep: true });
  pop.menu(x, y, acts, keys);
}

// run: a task action (the buttons, the menus and the keys all come here).
async function run(act, rows, ref = "") {
  if (act === "goto") {
    const r = rows[0];
    if (!r) return;
    if (r.target === "library") return toLibrary(r.items && r.items.length === 1 ? r.items[0].name : "");
    return ui.go(r.target);
  }
  try {
    const text = await api.TaskAct(act, rows.map((r) => r.id), ref);
    if (act === "forget" || act === "clear") {
      say("ok", text);
      const s = $("#status"), a = el("a", "", T("undo"));
      a.onclick = () => run("undo", []);
      s.append(a);
    } else if (text) say("ok", text);
    if (act === "forget") X.sel.clear();
  } catch (e) { say("fail", errText(e)); }
  load();
}

// ---- keys (06 §2.16): the page's own; the rest are the shell's ----
export function key(e) {
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  if (!X.page || !els) return;
  const done = () => e.preventDefault();
  if (k === "/" || (ctrl && k === "f")) { done(); return X.bar.focus(); }
  if (k === "\\") { done(); return X.bar.toggle(); }
  if (k === "1") { done(); return X.bar.focus(); }
  if (k === "2") { done(); return focusList(); }
  if (k === "3") { done(); return focusItems(); }
  if (k === "Escape") {
    done();
    if (X.bar.open) { X.bar.toggle(false); return focusList(); }
    if (X.focus === "items") return focusList();
    if (X.sel.size) { X.sel.clear(); return draw(); }
    if (X.bar.active()) return X.bar.clear();
    return;
  }
  if (k === "C") { done(); return run("clear", []); }
  if (ctrl && k.toLowerCase() === "z") { done(); return run("undo", []); }
  const menuKey = k === "ContextMenu" || (k === "F10" && e.shiftKey);
  if (X.focus === "items") return itemsKey(e, done, menuKey);
  const list = shown(), ids = list.map((r) => r.id), i = ids.indexOf(X.cur);
  const page = Math.max(1, Math.floor(els.rows.clientHeight / 46) - 1);
  const go = (j) => {
    j = Math.max(0, Math.min(ids.length - 1, j));
    if (e.shiftKey && X.cur) { X.sel.add(X.cur); X.sel.add(ids[j]); }
    done();
    moveTo(ids[j]);
  };
  if (k === "ArrowDown") return go(i + 1);
  if (k === "ArrowUp") return go(i - 1);
  if (k === "PageDown") return go(i + page);
  if (k === "PageUp") return go(i - page);
  if (k === "Home") return go(0);
  if (k === "End") return go(ids.length - 1);
  if (k === "Enter" || k === "ArrowRight") { done(); return focusItems(); }
  if (ctrl && (k === " " || k === "Spacebar")) { done(); if (X.cur) X.sel.has(X.cur) ? X.sel.delete(X.cur) : X.sel.add(X.cur); return draw(); }
  if (ctrl && k.toLowerCase() === "a") { done(); ids.forEach((x) => X.sel.add(x)); return draw(); }
  if (menuKey) { done(); const r = els.rows.querySelector(".tk-row.cur"); if (r) { const b = r.getBoundingClientRect(); taskMenu(b.left + 30, b.bottom, true); } return; }
  const r = current();
  if (!r) return;
  const hit = r.acts.find((a) => a.key === (ctrl ? "ctrl+" + k.toLowerCase() : k === "Delete" ? "Del" : k));
  if (hit) { done(); return run(hit.id, targets()); }
  if ("xrJKo".includes(k) && k.length === 1) { done(); say("", Tf("%s cannot do that now", r.title)); }
}
function itemsKey(e, done, menuKey) {
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  const rows = itemRows();
  const page = Math.max(1, Math.floor(els.iscroll.clientHeight / 30) - 2);
  const go = (j) => { done(); X.icur = Math.max(0, Math.min(rows.length - 1, j)); drawItems(); const c = els.ibody.querySelector(".tk-irow.cur"); if (c) c.scrollIntoView({ block: "nearest" }); ui.renderWhere(); };
  if (k === "ArrowDown") return go(X.icur + 1);
  if (k === "ArrowUp") return go(X.icur - 1);
  if (k === "PageDown") return go(X.icur + page);
  if (k === "PageUp") return go(X.icur - page);
  if (k === "Home") return go(0);
  if (k === "End") return go(rows.length - 1);
  if (k === "ArrowLeft") { done(); return focusList(); }
  if (table.key(e)) return;
  if (menuKey) { done(); const r = els.ibody.querySelector(".tk-irow.cur"); if (r) { const b = r.getBoundingClientRect(); itemMenu(b.left + 30, b.bottom, true); } return; }
  const it = rows[X.icur], t = current();
  if (!it) return;
  if (k === "o") { done(); return toLibrary(it.name); }
  if (ctrl && k.toLowerCase() === "o") { done(); return run("folder", [t], it.ref); }
  if (ctrl && k.toLowerCase() === "c") { done(); return copy(it.name); }
  if (k === "r" && it.state === "fail" && !it.permanent) { done(); return run("retry", [t], it.ref); }
}

// ---- the shell's lines ----
export function where() {
  if (!X.page) return "";
  const list = shown(), i = list.findIndex((r) => r.id === X.cur);
  if (X.focus === "items") {
    const r = current();
    return `${r ? r.title : ""} › ${T("every item")}    ${X.icur + 1} / ${(r && r.items && r.items.length) || 0}`;
  }
  let s = `${T("tasks")}    ${i + 1} / ${list.length}`;
  if (X.sel.size) s += "  " + Tf("%d selected", X.sel.size);
  if (X.bar && X.bar.active()) s += "  " + T("filtered");
  return s;
}
export function hintList() {
  if (X.bar && X.bar.open) return [{ key: "←→↑↓", label: T("pick a value") }, { key: "enter", label: T("only these") }, { key: "x", label: T("leave out") }, { key: "esc", label: T("close") }];
  if (X.focus === "items") return [{ key: "↑↓", label: T("move") }, { key: "←", label: T("back to the tasks") }, { key: "o", label: T("see it in the mod library") }, { key: "s", label: T("sort and columns") }, { key: "?", label: T("all keys") }];
  const r = current();
  const out = [{ key: "↑↓", label: T("move") }];
  if (r && r.items && r.items.length) out.push({ key: "enter", label: T("every item") });
  for (const a of (r && r.acts) || []) if (out.length < 6) out.push({ key: a.key, label: a.label });
  out.push({ key: "?", label: T("all keys") });
  return out;
}
