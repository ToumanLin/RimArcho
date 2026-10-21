// 首页 (06 §2.3): what you play — each plan and pack with its state, edit or details, and
// launch — then the activity cards: the last run, what the last scan changed, and the slot
// 没玩到 will fill (10 §3). Data: API.Home (view.BuildHome).
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
  const w = page("home", T("pick one and play. A plan is a list of mods that follows Workshop updates; a pack locks every mod's version and keeps its own saves."),
    rescanButton(), act(T("new plan / import"), "plus", () => newPlan()));
  sc.append(w);
  const running = !!(S.live && (S.live.running || S.live.blocked));

  const playing = el("section", "sec");
  playing.append(el("h2", "label", T("playing now")));
  const list = el("div", "card home-list");
  for (const r of data.rows || []) list.append(row(r, running));
  if (!(data.rows || []).length) list.append(el("div", "empty-state", T("no environment yet: add one on the environment page")));
  playing.append(list);
  if ((data.rows || []).length > 1) playing.append(el("div", "note muted", T("most recently played first")));
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

// row: [environment] name [方案 | 整合包] / its facts — state, edit or details, launch.
function row(r, running) {
  const d = el("div", "home-row");
  d.append(el("span", "envtag", r.env));
  const l = el("div", "l");
  const name = el("div", "name");
  name.append(el("span", "n", r.name), el("span", "kind " + r.kind, r.kind === "plan" ? T("plan") : T("pack")));
  // the game's ModsConfig.xml holds this plan now; a plan's unloaded external version
  const g = inGameMark(r.inGame);
  if (g) name.append(g);
  if (r.mark) name.append(el("span", "chip mark", r.mark));
  const meta = [r.facts, r.mode, r.played].filter(Boolean).join(" · ");
  l.append(name, el("div", "sub", meta));
  d.append(l);

  const right = el("div", "r");
  right.append(stateChip(r));
  if (r.kind === "plan") right.append(button(T("edit"), "", () => ui.go(r.page)));
  else right.append(button(T("details"), "", () => ui.go("pack:" + r.name)));
  if (!r.light) {
    const b = button(T("launch the game"), "", () => launch(r), "primary launch");
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

function lastRun() {
  if (S.live && S.live.running) {
    return actCard(T("last run"), S.live.pack ? Tf("pack %s", S.live.pack) + " · " + T("the game is running") : T("the game is running"),
      S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet"), T("see this run →"), () => ui.go("needs"));
  }
  if (!data.lastRun) return actCard(T("last run"), T("no run read yet"), T("launch once and its errors show here"), T("open the game log →"), () => ui.go("log"));
  return actCard(T("last run"), data.lastRun, data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"),
    data.lastRunErrors ? T("see which →") : T("open the game log →"), () => ui.go(data.lastRunErrors ? "needs" : "log"));
}

function changes() {
  if (!data.scanWhen) return actCard(T("mod library · since the last scan"), T("no scan yet (R scans)"), "", T("scan now →"), () => ui.cmd("rescan"));
  return actCard(T("mod library · since the last scan"), Tf("scanned %s", data.scanWhen), data.changes || T("nothing changed"), T("see what changed →"), () => ui.go("changes"));
}

// actCard: a heading, a muted line, a line, and a "… →" link (the whole card opens it).
function actCard(title, first, second, link, go) {
  const d = el(go ? "button" : "div", "act-card");
  d.append(el("span", "t", title));
  if (first) d.append(el("span", "a", first));
  if (second) d.append(el("span", "b", second));
  if (link) d.append(el("span", "go", link));
  if (go) d.onclick = go;
  return d;
}
