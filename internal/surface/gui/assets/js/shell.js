// The window's shell (06 §2.1, §2.2): the sidebar grouped by the player's questions, the
// top bar (collapse, breadcrumb, environment chip, the page's actions) and, at the
// sidebar's foot, the environment and the running game. What it lists comes from
// view.Nav and view.NavPath through API.Shell.
import { S, $, el, api, T, Tf } from "./state.js";
import { icon } from "./icons.js";

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
  addEventListener("resize", () => {
    const n = innerWidth < NARROW;
    if (n !== narrow) { narrow = n; collapsed = n; }
    applyCollapse();
  });
  applyCollapse();
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

// current is the sidebar ID of what is shown: a page, the plan (the editor and its full
// detail), or one pack.
export function current() {
  if (!S.screen || S.screen === "full") return "plan";
  if (S.screen === "packs" && S.packFocus) return "pack:" + S.packFocus;
  // adding an environment belongs to the environment page
  if (S.screen === "guide" && S.guideFrom === "env") return "env";
  return S.screen;
}

// sync redraws the sidebar and the breadcrumb for the page shown.
export function sync() {
  renderSide();
  renderCrumbs();
}

const ICONS = { home: "home", needs: "needs", plan: "plan", packs: "packs", env: "env", library: "library", log: "log" };

function renderSide() {
  const nav = $("#nav");
  const sh = S.shell;
  if (!sh) return;
  const cur = current();
  const sig = JSON.stringify([sh.nav, cur, S.board && S.board.needs]);
  if (nav.dataset.sig !== sig) {
    nav.dataset.sig = sig;
    nav.innerHTML = "";
    for (const g of sh.nav || []) {
      nav.append(el("div", "grp", g.title));
      for (const it of g.items || []) {
        const kids = it.kids || [];
        const open = it.id === cur || kids.some((k) => k.id === cur) || (it.id === "library" && cur === "changes");
        const b = el("button", "nav-item" + (open ? " on" : ""));
        b.title = it.key ? it.key.replace("shift", "Shift") : "";
        const ic = el("span", "ic");
        ic.innerHTML = icon[ICONS[it.id]] || "";
        b.append(ic, el("span", "lbl", it.label));
        // 能不能跑's badge follows the board's count between two Shell calls
        const badge = it.id === "needs" && S.board ? S.board.needs : it.badge;
        if (badge) b.append(el("span", "count", String(badge)));
        b.onclick = () => ui.go(it.id === "library" ? "changes" : it.id);
        nav.append(b);
        // only the open entry shows its sub-entries (06 §2.1)
        if (open && kids.length) {
          const box = el("div", "kids");
          for (const k of kids) {
            const kb = el("button", "nav-kid" + (k.id === cur ? " on" : ""), k.label);
            kb.onclick = () => ui.go(k.id);
            box.append(kb);
          }
          nav.append(box);
        }
      }
    }
  }
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

// renderLive: the environment and the running game at the sidebar's foot (06 §2.1).
export function renderLive() {
  const box = $("#live");
  const l = S.live;
  const env = (S.shell && S.shell.env) || "";
  $(".env", box).textContent = env;
  const run = $(".run", box);
  box.classList.toggle("running", !!(l && l.running));
  if (l && l.running) {
    run.textContent = (l.pack ? Tf("pack %s running since %s", l.pack, l.since) : Tf("game running since %s", l.since)) + (l.errors ? " · " + Tf("%d errors", l.errors) : "");
  } else run.textContent = (l && l.newLog) || T("the game is not running"); // 08 §2: a new Player.log; the click opens 能不能跑
  box.classList.toggle("newlog", !!(l && !l.running && l.newLog));
  box.title = l && l.guess ? T("which list runs is taken from the launch records") : T("can it run");
}

// crumbs: the breadcrumb of what is shown; every level opens its page (06 §2.2).
export function crumbs() {
  const sh = S.shell;
  const cur = current();
  let out = (sh && sh.crumbs && sh.crumbs[cur]) || [];
  if (S.screen === "guide") out = (S.guideFrom === "env" ? out : []).concat([{ label: S.guideFrom === "env" ? T("add an environment") : T("getting started"), id: "guide" }]);
  if (S.screen === "full" && S.detail) out = out.concat([{ label: S.detail.name, id: "full" }]);
  return out;
}

function renderCrumbs() {
  const env = $("#env");
  env.textContent = (S.shell && S.shell.env) || "";
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
