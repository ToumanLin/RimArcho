// 首页 (06 §2.3): what is applied now as one large card with ▶ 启动游戏, then every other
// plan and pack of every environment as one line — the game versions it runs on, the
// name, its kind, its mods, and 应用 — then the last game and the current plan's changes
// since it was last played, one line each.
// 应用 makes a row the applied one (its environment and plan become current; a pack becomes
// the featured item); it writes no game file. Data: API.Home (view.BuildHome,
// view.AddOtherEnvs, view.PickFeatured).
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, changesPlan } from "./screens.js";
import { icon } from "./icons.js";

let ui = null;
export function initHome(u) {
  ui = u;
  // Back from a game: the page reads the new Player.log (API.Home reads it when it is newer
  // than the last one read).
  addEventListener("focus", () => { if (S.screen === "home") home(); });
}

let data = null;

export async function home() {
  try { data = await api.Home(); } catch (e) { say("fail", errText(e)); return; }
  draw();
}

// homeKey: L launches the featured card, as its button does (one game at a time).
export function homeKey(e) {
  if (e.key !== "L" && !(e.key === "l" && e.shiftKey)) return;
  const r = data && data.featured >= 0 && (data.rows || [])[data.featured];
  if (!r || r.light || (S.live && (S.live.running || S.live.blocked))) return;
  e.preventDefault();
  launch(r);
}

// liveChanged redraws the launch button when the game starts or stops.
export function liveChanged() { if (S.screen === "home" && data) draw(); }

function draw() {
  const sc = S.screen === "home" ? document.querySelector("#screen") : openScreen("home");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  // no page actions: rescanning is Mod 库's, a new plan is 方案's
  const w = page("home", "");
  sc.append(w);
  const rows = data.rows || [];
  if (!rows.length) { w.append(el("div", "card empty-state", T("no environment yet: add one on the environment page"))); return; }
  const running = !!(S.live && (S.live.running || S.live.blocked));
  if (data.featured >= 0) w.append(hero(rows[data.featured], running));
  const rest = rows.filter((_, i) => i !== data.featured);
  if (rest.length) {
    const sec = el("section", "sec");
    sec.append(el("h2", "label", T("the others")));
    const list = el("div", "card hb-list");
    for (const r of rest) list.append(line(r));
    sec.append(list);
    w.append(sec);
  }
  w.append(activity());
  sc.scrollTop = top;
}

const kindTag = (r) => el("span", "kind", r.kind === "plan" ? T("plan") : T("pack"));

// problem: the state only when something is wrong; else nothing.
function problem(r) {
  if (r.level === "bad") {
    const p = el(r.current ? "button" : "span", "problem");
    p.innerHTML = icon.warn;
    p.append(el("span", "", r.state));
    if (r.current) { p.title = T("can it run"); p.onclick = (e) => { e.stopPropagation(); ui.go("needs"); }; }
    return p;
  }
  if (r.mark) return el("span", "problem", r.mark);
  return null;
}

// hero: the applied plan or pack — its environment, the name and kind, mods · last played,
// the problem, and the big ▶ 启动游戏. The card opens the plan's editor or the pack's page.
function hero(r, running) {
  const d = el("div", "card hb-hero");
  d.onclick = () => ui.go(r.kind === "plan" ? r.page : "pack:" + r.name);
  d.title = r.kind === "plan" ? T("edit") : T("details");
  const l = el("div", "l");
  const name = el("div", "nm");
  name.append(el("span", "n", r.name), kindTag(r));
  l.append(el("div", "k", r.env), name, el("div", "s", [Tf("%d mods", r.mods), r.played].join(" · ")));
  const p = problem(r);
  if (p) l.append(p);
  d.append(l);
  if (!r.light) {
    const b = el("button", "btn launch primary big");
    const i = el("span", "ic");
    i.innerHTML = icon.play;
    b.append(i, el("span", "", T("launch the game")));
    b.onclick = (e) => { e.stopPropagation(); launch(r); };
    // one game at a time (08 §4)
    if (running) { b.disabled = true; b.title = T("a game is already running"); } else if (r.kind === "plan") b.title = "L";
    d.append(b);
  }
  return d;
}

// line: versions | name [kind] | N 个 Mod | 应用 — on one grid, so the columns line up.
function line(r) {
  const d = el("div", "hb-line");
  const v = el("span", "v num", r.versions || "—");
  v.title = r.env;
  const b = el("button", "btn apply", T("apply"));
  b.title = r.otherEnv ? Tf("switch to %s and use this; no game file is written", r.envName) : T("use this; no game file is written");
  b.onclick = () => apply(r);
  d.append(v, el("span", "n", r.name), kindTag(r), el("span", "m num", Tf("%d mods", r.mods)), b);
  return d;
}

// apply: 应用 (06 §2.3) — the row becomes the featured card.
async function apply(r) {
  try { say("ok", await api.HomeApply(r.kind, r.envName, r.plan || 0, r.kind === "pack" ? r.name : "")); } catch (e) { return say("fail", errText(e)); }
  await ui.reload();
  await home();
}

// activity: the last game (only once there was one) and the current plan's changes since
// it was last played, each a line that is a link (06 §2.3).
function activity() {
  const box = el("div", "hb-act");
  const running = S.live && S.live.running;
  if (running || data.lastRun) {
    const runText = running ? T("the game is running") + " · " + (S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet"))
      : (data.lastRunDay || data.lastRun) + " · " + (data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"));
    const runGo = running || data.lastRunErrors ? "needs" : "log";
    box.append(actLine(T("last game"), runText, () => ui.go(runGo)));
  }
  const c = data.changes;
  if (c) {
    const go = c.noScan ? () => ui.cmd("rescan") : () => { changesPlan(); ui.go("changes"); };
    box.append(actLine(c.title, c.text, go, true));
  }
  return box;
}

function actLine(title, text, go, wide) {
  const b = el("button", "hb-actline" + (wide ? " wide" : ""));
  b.append(el("span", "t", title), el("span", "x", text), el("span", "go", "→"));
  b.onclick = go;
  return b;
}

// launch is the featured card's L (08 §2, §5.3): the plan launches at once, as the pack
// does on the packs page.
async function launch(r) {
  if (r.kind === "plan") return ui.run("launch");
  try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
}
