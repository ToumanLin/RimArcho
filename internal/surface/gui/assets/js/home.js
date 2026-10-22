// 启动 (06 §2.3), laid out like the game's main menu over a background image: every plan
// and pack of every environment down the left — the applied one first, raised, with
// 编辑 ↵; any other with 应用 ↵ on the cursor or under the mouse — the last run and the last
// changes since it was last played below them (each line only once there is
// something to say), and ▶ 启动游戏 in the bottom right with the applied list's problem
// tucked under it. 应用 makes a row the applied one (its environment and plan become
// current; a pack becomes the featured item); it writes no game file.
// Data: API.Home (view.BuildHome, view.AddOtherEnvs, view.PickFeatured) and
// API.HomeBackground (the image, its dimming, blur and focus; 06 §2.14).
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
let bg = null;
let rows = []; // the applied row first, then the others in the home order
let cur = 0;   // the keyboard's row

export async function home() {
  try { [data, bg] = await Promise.all([api.Home(), api.HomeBackground()]); } catch (e) { say("fail", errText(e)); return; }
  const all = data.rows || [];
  rows = data.featured >= 0 ? [all[data.featured], ...all.filter((_, i) => i !== data.featured)] : all;
  cur = Math.min(cur, Math.max(rows.length - 1, 0));
  draw();
}

const featured = () => (data && data.featured >= 0 ? rows[0] : null);
const running = () => !!(S.live && (S.live.running || S.live.blocked));

// homeKey: ↑↓ move, enter edits the applied row or applies another, L launches.
export function homeKey(e) {
  const k = e.key;
  if (k === "ArrowDown" || k === "ArrowUp") {
    e.preventDefault();
    if (rows.length) setCur((cur + (k === "ArrowDown" ? 1 : rows.length - 1)) % rows.length);
    return;
  }
  if (k === "Home" || k === "End") { e.preventDefault(); if (rows.length) setCur(k === "Home" ? 0 : rows.length - 1); return; }
  // enter on the focused launch button (focusLaunch) launches, as clicking it does
  if (k === "Enter" && document.activeElement && document.activeElement.matches(".hm-launch .btn.launch")) { e.preventDefault(); document.activeElement.click(); return; }
  if (k === "Enter") { e.preventDefault(); if (rows[cur]) act(rows[cur]); return; }
  if (k !== "L" && !(k === "l" && e.shiftKey)) return;
  const r = featured();
  if (!r || r.light || running()) return;
  e.preventDefault();
  launch(r);
}

export function hintList() {
  return [{ key: "↑↓", label: T("move") }, { key: "enter", label: T("apply / edit") }, { key: "L", label: T("launch") },
    { key: "shift+1–7", label: T("pages") }, { key: "?", label: T("all keys") }];
}

// focusLaunch is the editor's 启动游戏 (06 §2.5): the current plan becomes the applied row
// (it usually is; a pack applied here gives way) and the focus goes to ▶ 启动游戏, so L or
// enter starts the game.
export async function focusLaunch() {
  if (!data) return;
  const f = featured();
  if (!f || f.kind !== "plan" || !f.current) {
    const r = rows.find((x) => x.kind === "plan" && x.current);
    if (r) await apply(r);
  }
  const b = document.querySelector(".hm-launch .btn.launch");
  if (b) b.focus();
}

// liveChanged redraws the launch button when the game starts or stops.
export function liveChanged() { if (S.screen === "home" && data) draw(); }

function draw() {
  const sc = S.screen === "home" ? document.querySelector("#screen") : openScreen("home");
  sc.innerHTML = "";
  setBackdrop(bg);
  // no page actions: rescanning is Mod 库's, a new plan is 方案's
  const w = page("home", "");
  sc.append(w);
  if (!rows.length) { w.append(el("div", "card empty-state", T("no environment yet: add one on the environment page"))); return; }
  const menu = el("div", "hm-menu");
  const list = el("div", "hm-list");
  rows.forEach((r, i) => list.append(line(r, i)));
  menu.append(list, el("div", "grow"), activity());
  w.append(menu);
  const f = featured();
  if (f) w.append(launcher(f));
}

// setBackdrop puts the image behind the top bar and the page (#main's first child), or
// takes it away: off the launch page nothing holds the decoded picture (06 §10).
export function setBackdrop(b) {
  const app = document.querySelector("#app");
  let d = document.querySelector("#backdrop");
  if (!b || !b.url) {
    if (d) d.remove();
    app.classList.remove("photo");
    return;
  }
  if (!d) {
    d = el("div", "");
    d.id = "backdrop";
    d.append(el("div", "img"), el("div", "shade"));
    document.querySelector("#main").prepend(d);
  }
  const img = d.firstChild;
  const u = `url("${b.url}")`;
  if (img.style.backgroundImage !== u) img.style.backgroundImage = u;
  img.style.backgroundPosition = b.focus === "left" ? "left center" : b.focus === "right" ? "right center" : "center";
  img.style.filter = b.blur ? `blur(${b.blur}px)` : "";
  img.style.inset = b.blur ? `-${b.blur * 2}px` : "0";
  d.style.setProperty("--dim", b.dim / 100);
  app.classList.add("photo");
}

function setCur(i) {
  cur = i;
  document.querySelectorAll(".hm-row").forEach((d, j) => d.classList.toggle("cur", j === i));
  const d = document.querySelectorAll(".hm-row")[i];
  if (d) d.scrollIntoView({ block: "nearest" });
}

const kindTag = (r) => el("span", "kind", r.kind === "plan" ? T("plan") : T("pack"));

// line: [kind] name / versions · mods · played, and 编辑 ↵ or 应用 ↵ on the right.
function line(r, i) {
  const applied = i === 0 && featured() === r;
  const d = el("div", "hm-row" + (applied ? " on" : "") + (i === cur ? " cur" : ""));
  const n = el("div", "n");
  n.append(kindTag(r), el("span", "t", r.name));
  const sub = [r.versions || "", Tf("%d mods", r.mods), r.played].filter(Boolean).join(" · ");
  const s = el("div", "s num", sub);
  s.title = r.env;
  const b = el("button", "act", (applied ? T("edit") : T("apply")) + " ↵");
  b.title = applied ? "" : r.otherEnv ? Tf("switch to %s and use this; no game file is written", r.envName) : T("use this; no game file is written");
  b.onclick = (e) => { e.stopPropagation(); act(r); };
  d.append(n, s, b);
  d.onclick = () => setCur(i);
  d.ondblclick = () => act(r);
  return d;
}

function act(r) {
  if (featured() === r) return ui.go(r.kind === "plan" ? r.page : "pack:" + r.name);
  apply(r);
}

// launcher: where the applied list lives, ▶ 启动游戏, and its problem under the button.
function launcher(r) {
  const box = el("div", "hm-launch");
  const where = el("div", "where");
  where.append(r.env + " · ", el("b", "", r.name));
  box.append(where);
  if (r.light) return box;
  const stack = el("div", "stack");
  const b = el("button", "btn launch primary");
  const i = el("span", "ic");
  i.innerHTML = icon.play;
  b.append(i, el("span", "", T("launch the game")), el("kbd", "", "L"));
  b.onclick = () => launch(r);
  // one game at a time (08 §4)
  if (running()) { b.disabled = true; b.title = T("a game is already running"); }
  stack.append(b);
  const p = problem(r);
  if (p) stack.append(p);
  box.append(stack);
  return box;
}

// problem: what is wrong with the applied list, as a tab under the button; nothing when
// all is well. The current plan's goes to 能不能跑: problems that break the game, else a
// pack's links left in Mods/ (08 §5.3), else an outside version not loaded.
function problem(r) {
  if (r.level !== "bad" && !r.mark && !r.links) return null;
  const links = r.level !== "bad" && r.links;
  const p = el(r.current && (r.level === "bad" || links) ? "button" : "div", "issue" + (r.level === "bad" ? " bad" : ""));
  if (r.level === "bad") {
    const i = el("span", "ic");
    i.innerHTML = icon.warn;
    p.append(i);
  }
  p.append(el("span", "t", r.level === "bad" ? r.state : links || r.mark));
  if (p.tagName === "BUTTON") {
    p.append(el("span", "go", "→"));
    p.title = T("can it run");
    p.onclick = () => ui.go("needs");
  }
  return p;
}

// apply: 应用 (06 §2.3) — the row becomes the applied one, at the top.
async function apply(r) {
  try { say("ok", await api.HomeApply(r.kind, r.envName, r.plan || 0, r.kind === "pack" ? r.name : "")); } catch (e) { return say("fail", errText(e)); }
  cur = 0;
  await ui.reload();
  await home();
}

// activity: the last game (only once there was one) and the current plan's changes since
// it was last played, each a line that is a link (06 §2.3).
function activity() {
  const box = el("div", "hm-act");
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
  const b = el("button", "hm-actline" + (wide ? " wide" : ""));
  b.append(el("span", "t", title), el("span", "x", text), el("span", "go", "→"));
  b.onclick = go;
  return b;
}

// launch is the applied row's L (08 §2, §5.3): the plan launches at once, as the pack
// does on the packs page.
async function launch(r) {
  if (r.kind === "plan") return ui.run("launch");
  try { say("ok", await api.PackLaunch(r.name)); } catch (e) { say("fail", errText(e)); }
}
