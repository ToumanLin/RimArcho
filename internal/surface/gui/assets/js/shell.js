// The window's shell (06 §2.1, §2.2): the sidebar grouped by the player's questions, the
// top bar (collapse, breadcrumb, environment chip, the page's actions) and, at the
// sidebar's foot, the environment and the running game. What it lists comes from
// view.Nav and view.NavPath through API.Shell.
import { S, $, el, api, T, Tf } from "./state.js";
import { icon } from "./icons.js";
import { liveCount } from "./screens.js";
import { bump, slideNav } from "./motion.js";

let ui = null;

// Below this width the sidebar starts collapsed and opens over the page (06 §2.1).
export const NARROW = 760;
let collapsed = innerWidth < NARROW;
let narrow = innerWidth < NARROW;

export function initShell(u) {
  ui = u;
  $("#collapse").innerHTML = icon.sidebar;
  $("#collapse").onclick = () => setCollapsed(!collapsed);
  $("#scrim").onclick = () => setCollapsed(true);
  $("#env").onclick = () => ui.go("env");
  $("#nav-settings").onclick = () => ui.go("settings");
  $("#live").onclick = () => ui.go("needs");
  wireSideSplit();
  addEventListener("resize", () => {
    const n = innerWidth < NARROW;
    if (n !== narrow) { narrow = n; collapsed = n; }
    applyCollapse();
  });
  applyCollapse();
}

// The sidebar's width: dragged on its right edge, kept in the settings (ui.gui.side),
// double-click resets it (06 §2.1).
const SIDE_MIN = 168, SIDE_MAX = 360, SIDE_KEY = "ui.gui.side";

function setSideWidth(px) {
  const app = $("#app");
  if (px) app.style.setProperty("--side-w", Math.round(px) + "px");
  else app.style.removeProperty("--side-w");
  slideNav($("#nav"), true); // the highlights take the new width at once
}

function wireSideSplit() {
  const sp = $("#side-split");
  sp.title = T("drag to change the width; double-click to reset");
  api.Setting(SIDE_KEY).then((v) => { if (+v) setSideWidth(Math.min(SIDE_MAX, Math.max(SIDE_MIN, +v))); }).catch(() => {});
  sp.addEventListener("mousedown", (e) => {
    e.preventDefault();
    const x0 = e.clientX, w0 = $("#side").getBoundingClientRect().width;
    let w = w0;
    sp.classList.add("drag");
    document.body.classList.add("resizing");
    const move = (ev) => {
      w = Math.min(SIDE_MAX, Math.max(SIDE_MIN, w0 + ev.clientX - x0));
      setSideWidth(w);
    };
    const up = () => {
      sp.classList.remove("drag");
      document.body.classList.remove("resizing");
      removeEventListener("mousemove", move);
      removeEventListener("mouseup", up);
      if (w !== w0) api.SetSetting(SIDE_KEY, String(Math.round(w))).catch(() => {});
    };
    addEventListener("mousemove", move);
    addEventListener("mouseup", up);
  });
  sp.addEventListener("dblclick", () => {
    setSideWidth(0);
    api.SetSetting(SIDE_KEY, "").catch(() => {});
  });
}

function setCollapsed(v) {
  collapsed = v;
  applyCollapse();
}

function applyCollapse() {
  const app = $("#app");
  app.classList.toggle("collapsed", collapsed);
  app.classList.toggle("narrow", narrow);
  $("#scrim").hidden = collapsed || !narrow;
  $("#collapse").title = (collapsed ? T("show the sidebar") : T("hide the sidebar"));
}

// navigated: a narrow window's sidebar closes once a page is picked.
export function navigated() { if (narrow && !collapsed) setCollapsed(true); }

export async function loadShell() {
  try { S.shell = await api.Shell(); } catch { /* keep the last one */ }
  sync();
}

// current is the sidebar ID of what is shown: a page, the current plan (the editor and
// its full detail), or one pack.
export function current() {
  if (!S.screen || S.screen === "full") return (S.shell && S.shell.plan && S.shell.plan.page) || "plan";
  if (S.screen === "packs" && S.packFocus) return "pack:" + S.packFocus;
  // 卸载 RimArcho is under 设置
  if (S.screen === "uninstall") return "settings";
  // adding an environment belongs to the environment page
  if (S.screen === "guide" && S.guideFrom === "env") return "env";
  return S.screen;
}

// sync redraws the sidebar and the breadcrumb for the page shown.
export function sync() {
  renderSide();
  renderCrumbs();
}

// inGameMark: which plan the game's ModsConfig.xml holds (04 §1.3) is a small mark on that
// plan's sub-entry (the home card says it in words), only when there are several plans to
// tell apart; its tooltip says "游戏里现在是：<方案> v<n>". null when there is nothing to mark.
function inGameMark(isInGame) {
  const sh = S.shell;
  if (!isInGame || !sh || !sh.inGame || !sh.plans || sh.plans.length < 2) return null;
  const m = el("span", "ingame-dot");
  m.title = sh.inGame.text + " · " + T("switching plans writes no game file; writing to the game or launching does");
  return m;
}

const ICONS = { home: "home", needs: "needs", plan: "plan", packs: "packs", env: "env", library: "library", log: "log" };

// The entry whose sub-entries were open when the sidebar was last drawn: a redraw starts
// from it, so the sub-entries open and close with a transition (06 §7.1).
let openWas = "";
// Until the first page is shown the sidebar takes its state without moving (startup draws
// it before the home page is open).
let settled = false;
export function settle() { settled = true; }

function renderSide() {
  const nav = $("#nav");
  const sh = S.shell;
  if (!sh) return;
  const cur = current();
  const sig = JSON.stringify([sh.nav, cur, S.board && S.board.needs, liveCount()]);
  if (nav.dataset.sig !== sig) {
    nav.dataset.sig = sig;
    // the highlights are kept, so they slide from where they were
    const thumb = nav.querySelector(":scope > .nav-thumb");
    const hls = new Map([...nav.querySelectorAll(".kid-hl")].map((h) => [h.dataset.for, h]));
    nav.innerHTML = "";
    if (thumb) nav.append(thumb);
    let openNow = "";
    for (const g of sh.nav || []) {
      nav.append(el("div", "grp", g.title));
      for (const it of g.items || []) {
        const kids = it.kids || [];
        const kidOn = kids.some((k) => k.id === cur);
        const on = it.id === cur || kidOn;
        // only the open entry shows its sub-entries (06 §2.1)
        if (on && kids.length) openNow = it.id;
        const b = el("button", "nav-item" + (on ? " on" : "") + (kids.length ? " parent" : ""));
        b.dataset.id = it.id;
        b.title = it.key ? it.key.replace("shift", "Shift") : "";
        const ic = el("span", "ic");
        ic.innerHTML = icon[ICONS[it.id]] || "";
        b.append(ic, el("span", "lbl", it.label));
        if (kids.length) { const c = el("span", "chev"); c.innerHTML = icon.chev; b.append(c); }
        // Only 能不能跑 carries a number (06 §2.1): the board's count between two Shell
        // calls, plus this run's errors
        const badge = it.id === "needs" && S.board ? S.board.needs + liveCount() : it.badge;
        let n = null;
        if (badge) b.append(n = el("span", "badge-n num", String(badge)));
        bump("nav:" + it.id, n, badge || 0);
        b.onclick = () => ui.go(it.id);
        nav.append(b);
        if (kids.length) nav.append(kidList(it, kids, cur, hls.get(it.id)));
      }
    }
    // Draw the old state first, then switch to the new one: the sub-entries that close
    // and the ones that open move together, and nothing below them jumps. The first
    // drawing starts as it is.
    if (!thumb || !settled) openWas = openNow;
    for (const w of nav.querySelectorAll(".kids-wrap")) w.classList.toggle("open", w.dataset.for === openWas);
    void nav.offsetHeight;
    for (const w of nav.querySelectorAll(".kids-wrap")) w.classList.toggle("open", w.dataset.for === openNow);
    openWas = openNow;
  }
  slideNav(nav, !settled);
  const set = $("#nav-settings");
  set.classList.toggle("on", cur === "settings");
  if (!set.firstChild || !set.querySelector(".ic")) {
    const ic = el("span", "ic");
    ic.innerHTML = icon.settings;
    set.prepend(ic);
  }
  $(".lbl", set).textContent = T("settings");
  renderLive();
}

// kidList: an entry's sub-entries on a thin guide line, with their own highlight (hl, kept
// from the last drawing so it slides). The wrapper's row grows from 0fr to 1fr when the
// entry opens (style.css).
function kidList(it, kids, cur, hl) {
  const wrap = el("div", "kids-wrap");
  wrap.dataset.for = it.id;
  const clip = el("div", "kids");
  const box = el("div", "kids-in");
  if (!hl) { hl = el("span", "kid-hl"); hl.dataset.for = it.id; }
  box.append(hl);
  for (const k of kids) {
    const kb = el("button", "nav-kid" + (k.id === cur ? " on" : "") + (k.add ? " add" : ""));
    kb.dataset.id = k.id;
    kb.append(el("span", "lbl", k.label));
    const g = inGameMark(k.inGame);
    if (g) kb.append(g);
    // a plan's "有未载入的外部版本" (04 §4.3)
    if (k.mark) { const m = el("span", "mark"); m.title = k.mark; kb.append(m); kb.title = k.mark; }
    kb.onclick = () => ui.go(k.id);
    if (it.id === "plan" && !k.add) {
      kb.oncontextmenu = (e) => { e.preventDefault(); ui.planMenu(+k.id.slice(5), e.clientX, e.clientY); };
    }
    box.append(kb);
  }
  clip.append(box);
  wrap.append(clip);
  return wrap;
}

// renderLive: one line at the sidebar's foot, only while there is something to say (06
// §2.1): the game running ("游戏运行中 · 14:05 起 · 3 个错误") or, after it, a new Player.log
// to analyse (08 §2). The click opens 能不能跑. Otherwise nothing: the environment is the
// top bar's chip, and "the game is not running" is not news.
export function renderLive() {
  const box = $("#live");
  const l = S.live;
  const running = !!(l && l.running), hint = !!(l && !running && l.newLog);
  box.hidden = !running && !hint;
  box.classList.toggle("running", running);
  box.classList.toggle("newlog", hint);
  let text = "", errs = "";
  if (running) {
    text = l.pack ? Tf("pack %s running since %s", l.pack, l.since) : Tf("game running since %s", l.since);
    if (l.errors) errs = String(l.errors); // this run's errors: the amber count on the right edge
  } else if (hint) text = l.newLog;
  if (box.dataset.sig !== text + "|" + errs) {
    box.dataset.sig = text + "|" + errs;
    box.innerHTML = "";
    box.append(el("span", "t", text));
    if (errs) box.append(el("span", "badge-n num", errs));
  }
  const full = text + (errs ? " · " + Tf("%d errors", +errs) : "");
  box.title = full + " · " + (l && l.guess ? T("which list runs is taken from the launch records") : T("can it run"));
}

// crumbs: the breadcrumb of what is shown; every level opens its page (06 §2.2).
export function crumbs() {
  const sh = S.shell;
  const cur = current();
  let out = (sh && sh.crumbs && sh.crumbs[cur]) || [];
  if (S.screen === "guide") out = (S.guideFrom === "env" ? out : []).concat([{ label: S.guideFrom === "env" ? T("add an environment") : T("getting started"), id: "guide" }]);
  if (S.screen === "full" && S.detail) out = out.concat([{ label: S.detail.name, id: "full" }]);
  if (S.screen === "uninstall") out = out.concat([{ label: T("uninstall RimArcho"), id: "uninstall" }]);
  return out;
}

// renderCrumbs: the breadcrumb names the page (there is no other title), and the chip
// beside it says what the page is about: the environment, and on 能不能跑 the plan too.
// The home page spans every environment, and Mod 库存 (全部 and 变化) every copy on the
// machine: no chip there.
function renderCrumbs() {
  const env = $("#env");
  const sh = S.shell;
  let chip = (sh && sh.env) || "";
  if (S.screen === "needs" && sh && sh.plan) chip = sh.plan.name + " · " + chip;
  env.textContent = chip;
  env.hidden = !chip || S.screen === "home" || S.screen === "guide" || S.screen === "library" || S.screen === "changes" || S.screen === "uninstall";
  env.title = T("environment") + " · E";
  env.classList.toggle("on", S.screen === "env");
  const nav = $("#crumbs");
  const cs = crumbs();
  const sig = JSON.stringify(cs);
  if (nav.dataset.sig === sig) return;
  nav.dataset.sig = sig;
  nav.innerHTML = "";
  cs.forEach((c, i) => {
    if (i) nav.append(el("span", "sep", "/"));
    const b = el("button", "crumb" + (i === cs.length - 1 ? " last" : ""), c.label);
    b.onclick = () => { if (c.id !== "full" && c.id !== "guide") ui.go(c.id); };
    nav.append(b);
  });
}

// setActs puts the page's actions on the right of the top bar.
let actsKey = "";
export function setActs(key, ...nodes) {
  const box = $("#acts");
  if (key && key === actsKey) return;
  actsKey = key || "";
  box.innerHTML = "";
  for (const n of nodes) if (n) box.append(n);
}
