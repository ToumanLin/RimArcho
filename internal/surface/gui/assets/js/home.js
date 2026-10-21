// 首页 (06 §2.3): what you play — each plan and pack with its state, edit or details, and
// launch — then the activity card: the last run, what the last scan changed, and the slot
// 没玩到 will fill (10 §3). Data: API.Home (view.BuildHome).
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button } from "./screens.js";
import { icon } from "./icons.js";

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
  const w = page("home", T("home"));
  sc.append(w);
  const running = !!(S.live && (S.live.running || S.live.blocked));

  w.append(el("div", "label section", T("playing now")));
  const list = el("section", "card");
  for (const r of data.rows || []) list.append(row(r, running));
  if (!(data.rows || []).length) list.append(el("div", "empty-state", T("no environment yet: add one on the environment page")));
  w.append(list);

  w.append(el("div", "label section", T("activity")));
  const act = el("section", "card activity");
  act.append(activity(icon.log, T("last run"), lastRun(), () => ui.go("needs")));
  act.append(activity(icon.rescan, T("mod library changes"), changes(), () => ui.go("changes")));
  // 没玩到 (10 §3) comes in 1.1: its slot is here, hidden until it has something to say.
  const unplayed = activity(icon.library, T("not played yet"), "", null);
  unplayed.hidden = true;
  act.append(unplayed);
  w.append(act);
  sc.scrollTop = top;
}

function row(r, running) {
  const d = el("div", "home-row");
  const l = el("div", "l");
  const name = el("div", "name");
  name.append(el("span", "chip env", r.env), el("span", "n", r.name), el("span", "kind", r.kind === "plan" ? T("plan") : T("pack")));
  const state = el("span", "state " + (r.level || "unknown"), r.state);
  const sub = el("div", "sub");
  sub.append(state, el("span", "sep", "·"), el("span", "", r.facts), el("span", "sep", "·"), el("span", "", r.played));
  l.append(name, sub);
  const right = el("div", "r");
  if (r.kind === "plan") right.append(button(T("edit"), "", () => ui.go("plan")));
  else right.append(button(T("details"), "", () => ui.go("pack:" + r.name)));
  if (!r.light) {
    const b = button(T("launch") + " ▶", "", () => launch(r), "primary");
    // one game at a time (08 §4)
    if (running) { b.disabled = true; b.title = T("a game is already running"); }
    const wrap = el("span", "launch");
    wrap.title = running ? T("a game is already running") : (r.kind === "plan" ? "L" : "");
    wrap.append(b);
    right.append(wrap);
  }
  d.append(l, right);
  return d;
}

// launch is the row's L (08 §2, §5.3): the plan's preview first; a pack plays at once, as
// on the packs page.
async function launch(r) {
  if (r.kind === "plan") return ui.preview("launch");
  try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
}

function lastRun() {
  if (S.live && S.live.running) {
    return (S.live.pack ? T("now") + " · " + S.live.pack : T("now")) + " · " + T("the game is running");
  }
  if (!data.lastRun) return T("no run read yet");
  return data.lastRun + " · " + (data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"));
}

function changes() {
  if (!data.scanWhen) return T("no scan yet (R scans)");
  return data.scanWhen + " · " + (data.changes || T("nothing changed"));
}


function activity(ic, title, text, go) {
  const d = el("button", "act-row");
  const i = el("span", "ic");
  i.innerHTML = ic;
  d.append(i, el("span", "t", title), el("span", "v", text));
  if (go) { d.append(el("span", "go", "›")); d.onclick = go; } else d.disabled = true;
  return d;
}
