// 首页 (06 §2.3), variant B "continue": what you played last as one large card with ▶ 启动游戏,
// then every other plan and pack as one compact line each, then the activity as two lines of
// text. Per item only what decides what to play: the name, one line (mods · last played), the
// problem when there is one, and launch. Data: API.Home (view.BuildHome).
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, act, rescanButton } from "./screens.js";
import { icon } from "./icons.js";
import { openPlan, newPlan } from "./plans.js";

let ui = null;
export function initHome(u) { ui = u; }

let data = null;

export async function home() {
  try { data = await api.Home(); } catch (e) { say("fail", errText(e)); return; }
  draw();
}

// liveChanged redraws the launch buttons when the game starts or stops.
export function liveChanged() { if (S.screen === "home" && data) draw(); }

function draw() {
  const sc = S.screen === "home" ? document.querySelector("#screen") : openScreen("home");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("home hb", "", rescanButton(), act(T("new plan / import"), "plus", () => newPlan()));
  sc.append(w);
  const rows = data.rows || [];
  if (!rows.length) { w.append(el("div", "card empty-state", T("no environment yet: add one on the environment page"))); return; }
  const ctx = { running: !!(S.live && (S.live.running || S.live.blocked)), manyEnvs: new Set(rows.map((r) => r.env)).size > 1, several: rows.filter((r) => r.kind === "plan").length > 1 };
  let lead = rows.findIndex((r) => !r.light);
  if (lead < 0) lead = 0;
  w.append(hero(rows[lead], ctx));
  const rest = rows.filter((_, i) => i !== lead);
  if (rest.length) {
    const sec = el("section", "sec");
    sec.append(el("h2", "label", T("the others")));
    const list = el("div", "card hb-list");
    for (const r of rest) list.append(line(r, ctx));
    sec.append(list);
    w.append(sec);
  }
  w.append(activity());
  sc.scrollTop = top;
}

// subline: "485 个 Mod · 10 月 20 日玩过" — with the environment when there are several, and
// "游戏里是它" on the plan the game has when there are several plans.
function subline(r, ctx) {
  return [ctx.manyEnvs ? r.env : "", r.facts, r.played, r.inGame && ctx.several ? T("in the game") : ""].filter(Boolean).join(" · ");
}

// problem: the state only when something is wrong; else nothing.
function problem(r) {
  if (r.level === "bad") {
    const p = el(r.current ? "button" : "span", "problem");
    p.innerHTML = icon.warn;
    p.append(el("span", "", r.state));
    if (r.current) { p.title = T("can it run"); p.onclick = (e) => { e.stopPropagation(); ui.go("needs"); }; }
    return p;
  }
  if (r.mark) { const p = el("span", "problem", r.mark); return p; }
  return null;
}

function open(r) { ui.go(r.kind === "plan" ? r.page : "pack:" + r.name); }

function launchButton(r, ctx, cls) {
  const b = el("button", "btn launch " + cls);
  const i = el("span", "ic");
  i.innerHTML = icon.play;
  b.append(i, el("span", "", T("launch the game")));
  b.onclick = (e) => { e.stopPropagation(); launch(r); };
  // one game at a time (08 §4)
  if (ctx.running) { b.disabled = true; b.title = T("a game is already running"); } else if (r.kind === "plan") b.title = "L";
  return b;
}

function hero(r, ctx) {
  const d = el("div", "card hb-hero");
  d.onclick = () => open(r);
  d.title = r.kind === "plan" ? T("edit") : T("details");
  const l = el("div", "l");
  l.append(el("div", "k", r.kind === "plan" ? T("plan") : T("pack")), el("div", "n", r.name), el("div", "s", subline(r, ctx)));
  const p = problem(r);
  if (p) l.append(p);
  d.append(l);
  if (!r.light) d.append(launchButton(r, ctx, "primary big"));
  return d;
}

function line(r, ctx) {
  const d = el("div", "hb-line");
  d.onclick = () => open(r);
  d.title = r.kind === "plan" ? T("edit") : T("details");
  d.append(el("span", "n", r.name), el("span", "s", subline(r, ctx)));
  const p = problem(r);
  d.append(p || el("span", "grow"));
  if (!r.light) d.append(launchButton(r, ctx, "quiet"));
  return d;
}

// activity: two lines of text, each a link.
function activity() {
  const box = el("div", "hb-act");
  const runText = S.live && S.live.running ? T("the game is running") + " · " + (S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet"))
    : !data.lastRun ? T("no run read yet") : data.lastRun + " · " + (data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"));
  const runGo = (S.live && S.live.running) || data.lastRunErrors ? "needs" : "log";
  box.append(actLine(T("last run"), runText, () => ui.go(runGo)));
  const scanText = !data.scanWhen ? T("no scan yet (R scans)") : (data.changes || T("nothing changed")) + " · " + Tf("scanned %s", data.scanWhen);
  box.append(actLine(T("since the last scan"), scanText, () => (data.scanWhen ? ui.go("changes") : ui.cmd("rescan"))));
  return box;
}

function actLine(title, text, go) {
  const b = el("button", "hb-actline");
  b.append(el("span", "t", title), el("span", "x", text), el("span", "go", "→"));
  b.onclick = go;
  return b;
}

// launch is the row's L (08 §2, §5.3): that plan becomes current, then its preview; a pack
// plays at once, as on the packs page.
async function launch(r) {
  if (r.kind === "plan") {
    if (!(await openPlan(r.plan))) return;
    await home();
    return ui.preview("launch");
  }
  try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
}
