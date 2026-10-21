// 首页 (06 §2.3): what you play — each plan and pack with its state, edit or details, and
// launch — then the activity cards: the last run, what the last scan changed, and the slot
// 没玩到 will fill (10 §3). Data: API.Home (view.BuildHome).
//
// One card of rows on one grid (the columns line up from row to row): the name and one
// quiet line, one state chip, then a quiet 编辑 / 详情 and ▶ 启动游戏. Only the first row
// you can launch (the one played last) has the solid launch button; the others are tinted
// until the row is hovered.
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, act, rescanButton } from "./screens.js";
import { icon } from "./icons.js";
import { openPlan, newPlan } from "./plans.js";
import { inGameMark } from "./shell.js";

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
  const w = page("home", T("pick one and play."), rescanButton(), act(T("new plan / import"), "plus", () => newPlan()));
  sc.append(w);
  const running = !!(S.live && (S.live.running || S.live.blocked));
  const rows = data.rows || [];
  // the environment is said only when the rows span more than one
  const manyEnvs = new Set(rows.map((r) => r.env)).size > 1;
  const lead = rows.findIndex((r) => !r.light);

  const playing = el("section", "sec");
  playing.append(el("h2", "label", T("playing now")));
  if (!rows.length) playing.append(el("div", "card empty-state", T("no environment yet: add one on the environment page")));
  else {
    const list = el("div", "card home-list");
    rows.forEach((r, i) => list.append(row(r, running, manyEnvs, i === lead)));
    playing.append(list);
  }
  w.append(playing);

  const actv = el("section", "sec");
  actv.append(el("h2", "label", T("activity")));
  const cards = el("div", "act-cards");
  cards.append(lastRun(), changes());
  // 没玩到 (10 §3) comes in 1.1: its card is here, hidden until it has something to say.
  const unplayed = actCard(T("not played yet"), "", "", "", null);
  unplayed.classList.add("hl");
  unplayed.hidden = true;
  cards.append(unplayed);
  actv.append(cards);
  w.append(actv);
  sc.scrollTop = top;
}

// row: name [方案 | 整合包] (marks) / one quiet line — the state chip — 编辑 / 详情, launch.
function row(r, running, manyEnvs, lead) {
  const d = el("div", "home-row");
  const l = el("div", "l");
  const name = el("div", "name");
  name.append(el("span", "n", r.name));
  const kind = el("span", "kind", r.kind === "plan" ? T("plan") : T("pack"));
  kind.title = r.kind === "plan" ? T("a plan is a list of mods that follows Workshop updates") : T("a pack locks every mod's version and keeps its own saves");
  name.append(kind);
  // the plan the game has (when there are several), a plan's unloaded external version
  const g = inGameMark(r.inGame);
  if (g) name.append(g);
  if (r.mark) { const m = el("span", "mark-dot"); m.title = r.mark; name.append(m); }
  // a plan always follows the Workshop (its tag says so); a pack says how it is locked
  const meta = [manyEnvs ? r.env : "", r.facts, r.kind === "pack" ? r.mode : "", r.played].filter(Boolean).join(" · ");
  l.append(name, el("div", "sub", meta));
  d.append(l);

  const st = el("div", "st");
  st.append(stateChip(r));
  d.append(st);

  const right = el("div", "r");
  if (r.kind === "plan") right.append(button(T("edit"), "", () => ui.go(r.page), "ghost"));
  else right.append(button(T("details"), "", () => ui.go("pack:" + r.name), "ghost"));
  if (!r.light) {
    const b = button(T("launch the game"), "", () => launch(r), "launch " + (lead ? "primary" : "soft"));
    const i = el("span", "ic");
    i.innerHTML = icon.play;
    b.prepend(i);
    // one game at a time (08 §4)
    if (running) { b.disabled = true; b.title = T("a game is already running"); }
    const wrap = el("span", "launch-wrap");
    wrap.title = running ? T("a game is already running") : (r.kind === "plan" ? "L" : "");
    wrap.append(b);
    right.append(wrap);
  }
  d.append(right);
  return d;
}

// stateChip: ⚠ "N 个问题会出错" (opens 能不能跑 for the current plan), ✓ "能跑", or a
// muted "还没检查".
function stateChip(r) {
  const lv = r.level === "bad" ? "bad" : r.level === "ok" ? "ok" : "unknown";
  const c = el(lv === "bad" && r.current ? "button" : "span", "state " + lv);
  if (lv !== "unknown") {
    const i = el("span", "ic");
    i.innerHTML = lv === "bad" ? icon.warn : icon.ok;
    c.append(i);
  }
  c.append(el("span", "", r.state));
  if (c.tagName === "BUTTON") { c.title = T("can it run"); c.onclick = () => ui.go("needs"); }
  return c;
}

// launch is the row's L (08 §2, §5.3): that plan becomes current, then its preview; a pack
// plays at once, as on the packs page.
async function launch(r) {
  if (r.kind === "plan") {
    if (!(await openPlan(r.plan))) return;
    await home(); // the row it launches from is now the current plan's
    return ui.preview("launch");
  }
  try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
}

// lastRun: what the last run left — its errors first, which list and when under it.
function lastRun() {
  if (S.live && S.live.running) {
    return actCard(T("last run"), S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet"),
      S.live.pack ? Tf("pack %s", S.live.pack) + " · " + T("the game is running") : T("the game is running"), T("see this run →"), () => ui.go("needs"));
  }
  if (!data.lastRun) return actCard(T("last run"), T("no run read yet"), "", T("open the game log →"), () => ui.go("log"));
  return actCard(T("last run"), data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"), data.lastRun,
    data.lastRunErrors ? T("see which →") : T("open the game log →"), () => ui.go(data.lastRunErrors ? "needs" : "log"));
}

// changes: what the last scan found, then when it ran.
function changes() {
  if (!data.scanWhen) return actCard(T("since the last scan"), T("no scan yet (R scans)"), "", T("scan now →"), () => ui.cmd("rescan"));
  return actCard(T("since the last scan"), data.changes || T("nothing changed"), Tf("scanned %s", data.scanWhen), T("see what changed →"), () => ui.go("changes"));
}

// actCard: a small title, the news, a muted line, and a "… →" link (the whole card opens it).
function actCard(title, main, sub, link, go) {
  const d = el(go ? "button" : "div", "act-card");
  d.append(el("span", "t", title));
  if (main) d.append(el("span", "a", main));
  if (sub) d.append(el("span", "b", sub));
  if (link) d.append(el("span", "go", link));
  if (go) d.onclick = go;
  return d;
}
