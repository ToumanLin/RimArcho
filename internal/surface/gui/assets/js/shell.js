// The window's shell (06 §2.1, §2.2): the sidebar grouped by the player's questions, the
// top bar (collapse, breadcrumb, environment chip, the page's actions) and, at the
// sidebar's foot, the environment and the running game. What it lists comes from
// view.Nav and view.NavPath through API.Shell.
import { factText, S, $, el, api, T, Tf } from "./state.js";
import { icon } from "./icons.js";
import { liveCount, seg, setSeg } from "./screens.js";
import { bump, slideNav, places, slideFrom } from "./motion.js";
import * as pop from "./popups.js";
import { wireSeparator } from "./separator.js";

let ui = null;

// Where the navigation sits (06 §2.1): at the side (the sidebar, groups 在玩 / 库) or at the
// top (the same entries as one strip over the page: the groups are separators, an entry's
// sub-entries a dropdown). The toggle heads the sidebar, or ends the strip; ctrl+b. The
// choice is kept in ui.gui.nav ("top", or empty for the side).
const NAV_KEY = "ui.gui.nav";
let layout = "side";

export const navLayout = () => layout;

export function initShell(u) {
  ui = u;
  const t = $("#navtoggle");
  t.innerHTML = icon.sidebar;
  t.onclick = () => toggleNav();
  $("#env").onclick = () => ui.go("env");
  $("#nav-settings").onclick = () => ui.go("settings");
  $("#live").onclick = () => ui.go("needs");
  $("#tasks-line").onclick = () => ui.go("tasks");
  $("#side").addEventListener("keydown", navKey);
  wireSideSplit();
  api.Setting(NAV_KEY).then((v) => { if (v === "top") { layout = "top"; applyNav(); } }).catch(() => {});
  addEventListener("resize", () => slideNav($("#nav"), true));
  applyNav();
}

// toggleNav moves the navigation between the side and the top, and remembers it. The change
// is a view transition (the entries move to their new places) unless motion is reduced.
export function toggleNav() {
  layout = layout === "side" ? "top" : "side";
  api.SetSetting(NAV_KEY, layout === "top" ? "top" : "").catch(() => {});
  const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (document.startViewTransition && !still) document.startViewTransition(() => applyNav());
  else applyNav();
}

function applyNav() {
  const app = $("#app");
  app.classList.toggle("nav-top", layout === "top");
  app.classList.toggle("nav-side", layout === "side");
  const t = $("#navtoggle");
  t.title = (layout === "side" ? T("navigation at the top") : T("navigation at the side")) + T("; ") + "ctrl+b";
  slideNav($("#nav"), true);
  renderCrumbs();
}

// navigated: nothing to close any more (the navigation never lies over the page).
export function navigated() { pop.hideMenu(); }

// navKey: the keyboard in the navigation itself. Tab reaches its buttons; ← → (↑ ↓ at the
// side) move between them, enter opens one, and ↓ on a top entry with sub-entries opens
// its dropdown (06 §2.1, rule 6 of §3.1).
function navKey(e) {
  const b = e.target.closest("button");
  if (!b) return;
  const all = [...$("#side").querySelectorAll("button")].filter((x) => x.offsetParent);
  const i = all.indexOf(b);
  const next = layout === "top" ? "ArrowRight" : "ArrowDown", prev = layout === "top" ? "ArrowLeft" : "ArrowUp";
  if (e.key === next || e.key === prev) {
    if (layout === "top" && e.key === "ArrowDown") return;
    e.preventDefault(); e.stopPropagation();
    all[(i + (e.key === next ? 1 : all.length - 1)) % all.length].focus();
    return;
  }
  if (layout === "top" && e.key === "ArrowDown" && b.classList.contains("parent")) {
    e.preventDefault(); e.stopPropagation();
    kidMenu(b, true);
    return;
  }
  if (e.key === "Enter" || e.key === " ") { e.preventDefault(); e.stopPropagation(); b.click(); return; }
  if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); b.blur(); }
}

// kidMenu: an entry's sub-entries as a dropdown under it (the top layout).
function kidMenu(b, keys) {
  const it = navItem(b.dataset.id);
  if (!it || !it.kids) return;
  const r = b.getBoundingClientRect();
  pop.menu(r.left, r.bottom + 4, it.kids.map((k) => ({ label: k.label, cls: k.id === current() ? "on" : "", run: () => ui.go(k.id) })), keys);
}

// kidOwner: the entry whose sub-pages include the page shown (or that is the page shown).
function kidOwner(id) {
  for (const g of (S.shell && S.shell.nav) || []) {
    for (const it of g.items || []) {
      if (it.kids && it.kids.length && (it.id === id || it.kids.some((k) => k.id === id))) return it;
    }
  }
  return null;
}

function navItem(id) {
  for (const g of (S.shell && S.shell.nav) || []) for (const it of g.items || []) if (it.id === id) return it;
  return null;
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
  wireSeparator(sp, {
    key: SIDE_KEY, range: () => [SIDE_MIN, SIDE_MAX],
    get: () => $("#side").getBoundingClientRect().width,
    set: setSideWidth, reset: () => setSideWidth(0),
    restore: (v) => { if (+v) setSideWidth(Math.min(SIDE_MAX, Math.max(SIDE_MIN, +v))); },
  });
}

export async function loadShell() {
  try { S.shell = await api.Shell(); } catch { /* keep the last one */ }
  sync();
}

// current is the sidebar ID of what is shown: a page, the open plan (the editor and its
// full detail), or one pack.
export function current() {
  // 编译成整合包 is the current plan's sub-page (06 §2.1)
  // the full detail belongs to the list it was opened from (06 §4.2)
  if (S.screen === "full" && S.fullFrom === "library") return "library";
  if (!S.screen || S.screen === "full" || S.screen === "build") return (S.shell && S.shell.plan && S.shell.plan.page) || "plan";
  if (S.screen === "packs" && S.packFocus) return "pack:" + S.packFocus;
  // a list's export: under 列表管理, or under the list when the editor opened it (06 §2.17)
  if (S.screen === "plan-export" && S.planExport) return S.planExport.from === "editor" ? "plan:" + S.planExport.id : "plans";
  if (S.screen === "plan-import") return "plans"; // 06 §2.17
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
  m.title = sh.inGame.text + T("; ") + T("switching lists does not save to the game; saving or launching does");
  return m;
}

const ICONS = { home: "home", needs: "needs", plan: "plan", packs: "packs", textures: "textures", env: "env", library: "library", log: "log" };

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
    // the open entry's sub-entries slide when their order changes (the current list first)
    const was = settled ? places(nav, ".kids-wrap.open .nav-kid") : null;
    nav.innerHTML = "";
    if (thumb) nav.append(thumb);
    let openNow = "";
    for (const g of sh.nav || []) {
      nav.append(el("div", "grp", g.title));
      for (const it of g.items || []) {
        const kids = it.kids || [];
        // a pack left out of the capped sub-entries still keeps 整合包 open
        const kidOn = kids.some((k) => k.id === cur) || (it.id === "packs" && String(cur).startsWith("pack:"));
        const on = it.id === cur || kidOn;
        // only the open entry shows its sub-entries (06 §2.1)
        if (on && kids.length) openNow = it.id;
        const b = el("button", "nav-item" + (on ? " on" : "") + (kids.length ? " parent" : ""));
        b.dataset.id = it.id;
        // the label is the tooltip in the compact rail (magpie's); the key follows it
        b.title = it.label + (it.key ? T("; ") + it.key.replace("shift", "Shift") : "");
        const ic = el("span", "ic");
        ic.innerHTML = icon[ICONS[it.id]] || "";
        b.append(ic, el("span", "lbl", it.label));
        if (kids.length) {
          const c = el("span", "chev");
          c.innerHTML = icon.chev;
          // at the top, the chevron opens the sub-entries as a dropdown
          c.onclick = (e) => { if (layout === "top") { e.stopPropagation(); kidMenu(b, false); } };
          b.append(c);
        }
        // Only 能不能跑 carries a number (06 §2.1): the board's count between two Shell
        // calls, plus this run's errors
        const badge = it.id === "needs" && S.board ? S.board.needs + liveCount() : it.badge;
        let n = null;
        if (badge) b.append(n = el("span", "badge-n num", String(badge)));
        bump("nav:" + it.id, n, badge || 0);
        // 列表 opens its first sub-entry, the current list (06 §2.1)
        b.onclick = () => ui.go(it.id === "plan" ? (kids.find((k) => k.current) || kids.find((k) => k.id !== "plans") || { id: "plans" }).id : it.id);
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
    slideFrom(nav, ".kids-wrap.open .nav-kid", was);
  }
  slideNav(nav, !settled);
  const set = $("#nav-settings");
  set.classList.toggle("on", cur === "settings");
  const tl = $("#tasks-line");
  if (tl) tl.classList.toggle("on", cur === "tasks");
  if (!set.firstChild || !set.querySelector(".ic")) {
    const ic = el("span", "ic");
    ic.innerHTML = icon.settings;
    set.prepend(ic);
  }
  $(".lbl", set).textContent = T("settings");
  set.title = T("settings");
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
    kb.title = k.label;
    const g = inGameMark(k.inGame);
    if (g) kb.append(g);
    // a plan's "有未载入的外部版本" (04 §4.3)
    if (k.mark) { const m = el("span", "mark"); m.title = k.mark; kb.append(m); kb.title = k.label + T("; ") + k.mark; }
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
    box.append(el("span", "t word-facts", text));
    if (errs) box.append(el("span", "badge-n num", errs));
  }
  const full = text + (errs ? "  " + Tf("%d errors", +errs) : "");
  box.title = full + T("; ") + (l && l.guess ? T("which list runs is taken from the launch records") : T("can it run"));
}

// crumbs: the breadcrumb of what is shown; every level opens its page (06 §2.2).
export function crumbs() {
  const sh = S.shell;
  const cur = current();
  let out = (sh && sh.crumbs && sh.crumbs[cur]) || [];
  if (S.screen === "guide") out = (S.guideFrom === "env" ? out : []).concat([{ label: S.guideFrom === "env" ? T("add a game") : T("getting started"), id: "guide" }]);
  if (S.screen === "full" && S.fullName) out = out.concat([{ label: S.fullName, id: "full" }]);
  if (S.screen === "uninstall") out = out.concat([{ label: T("uninstall RimArcho"), id: "uninstall" }]);
  if (S.screen === "build") out = out.concat([{ label: T("create a pack"), id: "build" }]);
  if (S.screen === "plan-export" && S.planExport) out = ((sh && sh.crumbs && sh.crumbs["plan:" + S.planExport.id]) || []).concat([{ label: T("export"), id: "plan-export" }]);
  if (S.screen === "plan-import") out = out.slice(0, -1).concat([{ label: T("import"), id: "plan-import" }]);
  if (S.screen === "plans" && S.listCreating) out = out.concat([{ label: T("new list"), id: "plan:new" }]);
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
  if (S.screen === "needs" && sh && sh.plan) chip = sh.plan.name + "  " + chip;
  factText(env, chip);
  env.hidden = !chip || !S.screen || S.screen === "home" || S.screen === "guide" || S.screen === "library" || S.screen === "changes" || S.screen === "subscribe" || S.screen === "uninstall" || S.screen === "textures" || S.screen === "tasks";
  env.title = T("game") + T("; ") + "E";
  env.classList.toggle("on", S.screen === "env");
  const nav = $("#crumbs");
  let cs = crumbs();
  // At the top the strip already names the entry: an entry with sub-pages shows them here
  // as a switch (magpie's), then only what lies below the sub-page (the detail, the build).
  const cur = current();
  const owner = layout === "top" ? kidOwner(cur) : null;
  let tabs = null;
  if (owner) {
    tabs = owner.kids;
    cs = cs.slice(((sh && sh.crumbs && sh.crumbs[cur]) || []).length);
  } else if (layout === "top" && ((sh && sh.crumbs && sh.crumbs[cur]) || []).length > 1) {
    cs = cs.slice(1); // the group (在玩, 库) is not shown at the top
  }
  const sig = JSON.stringify([tabs && tabs.map((k) => k.id + k.label), cs]);
  const on = tabs ? tabs.findIndex((k) => k.id === cur) : -1;
  if (nav.dataset.sig === sig) {
    if (tabs) setSeg(nav.querySelector(".seg"), on);
    return;
  }
  nav.dataset.sig = sig;
  nav.innerHTML = "";
  if (tabs) {
    const s = seg(tabs.map((k) => k.label), on, (i) => ui.go(tabs[i].id));
    s.classList.add("navtabs");
    // a plan's tab has the plan's menu, as its sidebar entry does
    if (owner.id === "plan") {
      [...s.querySelectorAll(":scope > button")].forEach((b, i) => {
        const k = tabs[i];
        if (!k.add) b.oncontextmenu = (e) => { e.preventDefault(); ui.planMenu(+k.id.slice(5), e.clientX, e.clientY); };
      });
    }
    nav.append(s);
    if (cs.length) nav.append(el("span", "sep", "/"));
  }
  cs.forEach((c, i) => {
    if (i) nav.append(el("span", "sep", "/"));
    const b = el("button", "crumb" + (i === cs.length - 1 ? " last" : ""), c.label);
    b.onclick = () => { if (c.id !== "full" && c.id !== "guide") ui.go(c.id); };
    nav.append(b);
  });
}

// setActs puts the page's actions at the right end of its description line (06 §2.2): the
// page's own .page-head, or the plan editor's #planhead. The top bar has no buttons.
let actsKey = "";
export function setActs(key, ...nodes) {
  const box = S.screen ? $("#screen .page-head .page-acts") : $("#planhead .page-acts");
  if (!box) return;
  if (key && key === actsKey && box.dataset.filled) return;
  actsKey = key || "";
  box.dataset.filled = "1";
  box.innerHTML = "";
  for (const n of nodes) if (n) box.append(n);
}
