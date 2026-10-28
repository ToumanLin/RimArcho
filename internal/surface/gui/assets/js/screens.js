// The pages over the board (the sidebar's entries, 06 §2.1): 能不能跑 (06 §2.4, §5), the
// game log (§2.10), Mod 库 › 变化 (§2.9), the full detail (§4.2), packs, the environment
// and settings. The pages are side by side: esc does not go from one to another (06 §3.1);
// esc leaves only the full detail, back to the plan. A page's actions sit in the top bar.
import { linkRows } from "./linkrows.js";
import { copyablePath, pathContent } from "./fields.js";
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
import { getChoices } from "./credit.js";

let ui = null;
export function initScreens(u) {
  ui = u;
  initSubscriptions(u, { beginPageRead, page, area, button, act, seg });
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
  if (S.screen === "needs" && N.data && !pageLoading()) { orderNeeds(); drawNeeds(); }
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
  const read = beginPageRead("needs");
  let data;
  try { data = await api.Needs(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  N.data = data;
  // back on a kept page: the cursor stays where it was
  if (!keepCursor && $("#screen").dataset.warm !== "1") N.cur = 0;
  orderNeeds();
  drawNeeds();
  read.finish();
}

// lead: what the page says about the plan in one sentence.
function needsLead(d) {
  const plan = (S.shell && S.shell.plan && S.shell.plan.name) || T("the list");
  const breaks = N.items.filter((x) => x.counted && x.section === 0).length;
  const count = N.items.filter((x) => x.counted).length;
  if (breaks) return Tf("%s is not safe to play yet: fix the %d problems below.", plan, breaks);
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
    autoFix(),
    act(T("check the files again"), "rescan", () => ui.cmd("analyze"), "A", "analyze"),
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
      h.classList.add("word-facts");
      h.append(c);
      bump("needs:" + si, c, n);
    }
    // which run the log is from: the list, or a pack (08 §5.3)
    if (si === 1 && S.live && S.live.running) h.append(el("span", "muted", (S.live.pack ? Tf("pack %s", S.live.pack) : T("this run"))));
    else if (si === 1 && d.lastRun) h.append(el("span", "muted word-facts", d.lastRun));
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

// autoFix: one click for what needs no question: the required mods that are installed
// but not enabled are enabled and the list sorted again (one undo step).
function autoFix() {
  const n = N.items.filter((it) => it.counted && it.target && (it.actions || []).some((a) => a.kind === "enable")).length;
  if (!n) return null;
  const b = act(Tf("fix %d automatically", n), "ok", async () => {
    await ui.edited(() => api.CompleteDependencies(), true);
    if (S.screen === "needs") needs(true);
  }, "F", "enable");
  b.classList.add("primary");
  b.title = T("enables the required mods that are installed and sorts the list; u undoes it");
  return b;
}

function needCard(it, i) {
  const row = el("div", "need" + (it.dim ? " dimmed" : ""));
  const top = el("div", "top");
  const body = el("div", "body");
  if (it.dim) {
    body.append(el("div", "why word-facts", it.text));
    if (it.why) body.append(el("div", "why sub", it.why));
  } else {
    body.append(el("div", "title word-facts", it.title || it.text));
    if (it.why && !it.linkGroups?.length) body.append(el("div", "why", it.why));
  }
  top.append(body);
  const acts = it.actions || [];
  const right = el("div", "acts");
  if (archo.ask && it.section === 1) {
    const b = button(T("ask Archo"), "", () => archo.ask(it), "small");
    b.prepend(archoMark());
    right.append(b);
  }
  if (acts.length) {
    const a = acts[0];
    right.append(button(a.label, "", () => { N.cur = i; markNeeds(); runAction(it, a); }, it.dim ? "" : "primary", actionWrite(a)));
    if (acts.length > 1) {
      const more = button("⋯", "", () => {
        N.cur = i; markNeeds();
        const r = more.getBoundingClientRect();
        pop.menu(r.left, r.bottom + 4, acts.slice(1).map((x) => ({ label: x.label, key: x.key, envWrite: actionWrite(x), run: () => runAction(it, x) })));
      }, "more");
      more.title = acts.slice(1).map((x) => x.label).join(T("; "));
      right.append(more);
    }
  }
  top.append(right);
  row.append(top);
  if (it.linkGroups?.length) row.append(linkRows(it.linkGroups));
  if (it.fold && N.open.has(it.text)) {
    const parts = el("div", "parts");
    for (const p of it.parts || []) parts.append(el("div", "", ", " + p));
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

// Read-only needs actions stay available during a game switch (06 §3.1).
const actionWrite = (a) => ["open-url", "view", "list", "manage-lists", "fold"].includes(a.kind) ? "" : a.kind;
// The pinned badge popover in the editor offers the same actions (06 §4.2).
setNeedRunner((it, a) => runAction(it, a), actionWrite);
// The editor's problems panel (problems.js) runs them too.
export const runNeedAction = (it, a) => runAction(it, a);
export const needActionWrite = actionWrite;

async function runAction(it, act) {
  switch (act.kind) {
    case "sort": case "save":
      return ui.run(act.kind);
    case "open-url":
      try { await api.OpenURL(act.url); say("ok", act.done); } catch (e) { say("fail", errText(e)); }
      return;
    case "locate-directory":
      return directoryPrompt({ id: act.refs[0], label: it.title || act.label, path: act.lines?.[0] || "" }, false, async () => { await ui.reload(); needs(true); });
    case "view":
      try { pop.text(act.label, await api.Inspect(act.argv)); } catch (e) { say("fail", errText(e)); }
      return;
    case "list":
      return pop.text(act.title, act.lines);
    case "manage-lists":
      return ui.manageLists(0);
    case "fold":
      N.open.has(it.text) ? N.open.delete(it.text) : N.open.add(it.text);
      return drawNeeds();
    case "choose-disable":
      return pop.choose(act.title, act.options.map((o) => o.label), (i) => doAction(act, i));
    case "subscribe":
      // a background task (06 §2.16): the mods take their place in the list once downloaded
      return pop.choose(act.label, getChoices(), async (i) => {
        try { say("ok", await api.Subscribe({ kind: "workshop", url: act.refs.join(" "), steamcmd: i === 1, batch: true })); } catch (e) { say("fail", errText(e)); }
      });
  }
  if (act.confirm) {
    // Not undone with u: ask first (06 §6).
    if (act.kind === "repair-links" && act.linkGroups?.length) {
      return pop.panel(act.label, linkRows(act.linkGroups), act.label, () => doAction(act, 0), "link-repair", act.kind);
    }
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
  // shift+letter is the capital letter even where the key reports it in lower case
  const k = e.shiftKey && /^[a-z]$/.test(e.key) ? e.key.toUpperCase() : e.key;
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
  if (k === "F") { const b = $("#screen .page-acts .primary"); if (b) { e.preventDefault(); b.click(); } return; }
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
const L = { env: null, path: "", page: null, loading: false, pending: "", error: "", cur: 0, detail: new Map(), seq: 0, els: [], tab: 0, tabPick: null, mcur: 0, open: new Set(), want: "", list: null };

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
  L.detail.clear(); L.open.clear();
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
// files; a click opens its details and the Player.log cluster it matches.
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
    if (open) {
      if (m.details) { const pre = el("pre", "details"); pathContent(pre, m.details); pre.onclick = (e) => e.stopPropagation(); body.append(pre); }
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
  const k = L.page.path + "|" + id;
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
  const appearance = groups.filter(g => g.items?.some(it => it.edit === "swatches"));
  renderGroups(w, appearance, settings);
  w.querySelector(".settings")?.append(bg);
  renderGroups(w, groups.filter(g => !appearance.includes(g)), settings);
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
      if (value) factText(value, { text: it.value || "", paths: it.valuePath ? [it.value] : [] });
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
    left.append(el("div", "name", d.label), copyablePath(el("div", "sub", d.path), d.path));
    row.append(left, button(T("locate"), "", () => directoryPrompt(d, false, async () => { await ui.reload(); checkDirectories(); })));
    body.append(row);
  }
  pop.panel(T("some directories need their new location"), body, T("skip for now"), () => {});
}

function directoryPrompt(d, moving, after) {
  pop.open("directory", d.label, (box, p) => {
    box.append(el("div", "ttl", d.label), copyablePath(el("p", "sub", d.path), d.path));
    const input = el("input", "field");
    input.spellcheck = false;
    input.placeholder = T("new directory path");
    const error = el("div", "err");
    const actions = el("div", "actions");
    let busy = false;
    let previewedPath = "";
    const affected = el("pre", "sub");
    const apply = async () => {
      const path = input.value.trim();
      if (!path || busy) return;
      busy = true;
      error.textContent = "";
      try {
        if (moving && previewedPath !== path) {
          const links = await api.DirectoryMovePreview(d.id, path);
          affected.textContent = [d.path + " → " + path, ...(links || [])].join("\n");
          previewedPath = path;
          applyButton.textContent = T("confirm move");
          busy = false;
          return;
        }
        const lines = await api.ApplySetting(["folder", moving ? "move" : "set", d.id, "{}"], path);
        pop.close();
        say("ok", lines?.[0] || T("done"));
        if (after) await after();
      } catch (e) { error.textContent = errText(e); busy = false; }
    };
    if (moving) box.append(el("p", "sub", T("move this directory and its files to the new path?")));
    const applyButton = button(T(moving ? "preview move" : "locate"), "", apply, "primary", "apply-setting");
    input.addEventListener("input", () => {
      previewedPath = "";
      affected.textContent = "";
      applyButton.textContent = T(moving ? "preview move" : "locate");
    });
    actions.append(button(T(moving ? "choose parent folder…" : "choose…"), "", () => pickFolder(d.path, path => {
      input.value = moving ? path.replace(/[\\/]$/, "") + "/" + d.path.split(/[\\/]/).pop() : path;
    })),
      applyButton, button(T("skip for now"), "", () => pop.close()));
    box.append(input, affected, error, actions);
    p.keys = e => { if (e.key === "Enter") { e.preventDefault(); apply(); return true; } return false; };
    setTimeout(() => input.focus(), 0);
  });
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
        const s = el("select");
        for (const [i, c] of it.choices.entries()) {
          const o = el("option", "", (it.choiceLabels && it.choiceLabels[i]) || c);
          o.value = c;
          if (c === it.value) o.selected = true;
          s.append(o);
        }
        // A new language: the page's text again, in place (main.js relang).
        s.onchange = () => run(it.argv, s.value,
          it.argv[2] === "ui.lang" ? () => ui.relang() : it.argv[2] === "ui.accent" ? () => ui.reaccent() : null);
        right.append(s);
        if (it.valueNote) right.append(el("span", "muted word-facts", it.valueNote));
      } else if (it.edit === "directory") {
        const value = factText(el("span", "muted setting-value"), { text: it.value, paths: [it.value] });
        const d = { id: it.directory, label: it.label, path: it.value };
        const after = async () => { await ui.reload(); again(); };
        right.append(value, button(T("locate"), "", () => directoryPrompt(d, false, after), "", "apply-setting"),
          button(T("move to…"), "", () => directoryPrompt(d, true, after), "", "apply-setting"));
      } else if (it.edit) {
        const inp = el("input", "field");
        inp.value = it.editValue ?? it.value ?? "";
        inp.spellcheck = false;
        const ok = button(T("apply"), "", () => run(it.argv, inp.value.trim()), "", "apply-setting");
        inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); ok.click(); } };
        if (it.editValue !== undefined) right.append(factText(el("span", "muted setting-value"), { text: it.value || "", paths: it.valuePath ? [it.value] : [] }));
        right.append(inp);
        // Apply the file or folder chosen through the window's dialog.
        if (it.edit === "path") right.append(button(T("choose…"), "", async () => {
          if (!it.file) return pickFolder(inp.value, (p) => run(it.argv, p));
          try { const p = await api.PickProgram(); if (p) run(it.argv, p); } catch (e) { say("fail", errText(e)); }
        }, "", "apply-setting"));
        right.append(ok);
      } else {
        right.append(factText(el("span", "muted setting-value"), { text: it.value || "", paths: it.valuePath ? [it.value] : [] }));
      }
      renderSettingActions(right, it.actions, run, it);
      row.append(left, right);
      card.append(row);
    }
    w.append(card);
  }
}

function renderSettingActions(right, actions, run, item) {
  for (const a of actions || []) {
    // 卸载 RimArcho is its own page with the plan; every other action is a command
    if (a.argv[0] === "uninstall") {
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
