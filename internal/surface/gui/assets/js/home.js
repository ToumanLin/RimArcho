// 首页 (06 §2.3), variant A: magpie's list pages — one card per plan or pack: a small icon
// tile (a list for a plan, a box for a pack), the name in bold, one muted line (mods · last
// played), the problem only when there is one, and ▶ 启动游戏 on the right. The card opens
// the plan's editor or the pack's page. The activity is the same kind of card. Data:
// API.Home (view.BuildHome).
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
  const w = page("home", "", rescanButton(), act(T("new plan / import"), "plus", () => newPlan()));
  sc.append(w);
  const rows = data.rows || [];
  const ctx = { running: !!(S.live && (S.live.running || S.live.blocked)), manyEnvs: new Set(rows.map((r) => r.env)).size > 1, several: rows.filter((r) => r.kind === "plan").length > 1 };
  const playing = el("section", "sec");
  playing.append(el("h2", "label", T("playing now")));
  const list = el("div", "ha-list");
  if (!rows.length) list.append(el("div", "card empty-state", T("no environment yet: add one on the environment page")));
  for (const r of rows) list.append(item(r, ctx));
  playing.append(list);
  w.append(playing);
  const actv = el("section", "sec");
  actv.append(el("h2", "label", T("activity")));
  const cards = el("div", "ha-list");
  cards.append(...activity());
  actv.append(cards);
  w.append(actv);
  sc.scrollTop = top;
}

// card: [tile] title / subline … right. go opens what the card is about.
function card(ic, title, sub, right, go, tip) {
  const d = el("div", "card ha-item");
  const t = el("span", "tile");
  t.innerHTML = icon[ic] || "";
  const l = el("div", "l");
  l.append(el("div", "n", title));
  if (sub) l.append(el("div", "s", sub));
  d.append(t, l);
  for (const x of right) if (x) d.append(x);
  if (go) { d.onclick = go; d.tabIndex = 0; d.onkeydown = (e) => { if (e.key === "Enter" && e.target === d) go(); }; }
  if (tip) d.title = tip;
  return d;
}

function item(r, ctx) {
  const tip = (r.kind === "plan" ? T("a plan is a list of mods that follows Workshop updates") : T("a pack locks every mod's version and keeps its own saves")) + " · " + (r.kind === "plan" ? T("edit") : T("details"));
  return card(r.kind === "plan" ? "plan" : "packs", r.name, subline(r, ctx), [problem(r), r.light ? null : launchButton(r, ctx)], () => open(r), tip);
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
  if (r.mark) return el("span", "problem", r.mark);
  return null;
}

function open(r) { ui.go(r.kind === "plan" ? r.page : "pack:" + r.name); }

function launchButton(r, ctx) {
  const b = el("button", "btn launch");
  const i = el("span", "ic");
  i.innerHTML = icon.play;
  b.append(i, el("span", "", T("launch the game")));
  b.onclick = (e) => { e.stopPropagation(); launch(r); };
  // one game at a time (08 §4)
  if (ctx.running) { b.disabled = true; b.title = T("a game is already running"); } else if (r.kind === "plan") b.title = "L";
  return b;
}

// activity: the last run and the last scan, as cards like the ones above.
function activity() {
  const link = (text) => el("span", "go", text);
  let run;
  if (S.live && S.live.running) run = card("log", T("last run"), T("the game is running") + " · " + (S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet")), [link(T("see this run →"))], () => ui.go("needs"));
  else if (!data.lastRun) run = card("log", T("last run"), T("no run read yet"), [link(T("open the game log →"))], () => ui.go("log"));
  else run = card("log", T("last run"), (data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors")) + " · " + data.lastRun,
    [link(data.lastRunErrors ? T("see which →") : T("open the game log →"))], () => ui.go(data.lastRunErrors ? "needs" : "log"));
  const scan = !data.scanWhen ? card("rescan", T("since the last scan"), T("no scan yet (R scans)"), [link(T("scan now →"))], () => ui.cmd("rescan"))
    : card("library", T("since the last scan"), (data.changes || T("nothing changed")) + " · " + Tf("scanned %s", data.scanWhen), [link(T("see what changed →"))], () => ui.go("changes"));
  return [run, scan];
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
