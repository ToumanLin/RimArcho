// 启动 (06 §2.3), on a Swiss grid: what is applied is one large headline that fits its
// width (its mods and last play below), the last game and the changes since it was last
// played in the top right, and under a rule three steps read left to right — 01 the game,
// 02 that game's lists and packs, 03 ▶ 启动游戏 with the applied list's problem under it
// in one line. Picking a game or a row applies it at once
// (its environment and plan become current; a pack becomes the featured item); it writes
// no game file. Picking the applied row again opens it. The problem can be hidden; it
// comes back when the row, its game, its mods or its problem change.
// Data: API.Home (view.BuildHome, view.AddOtherEnvs, view.PickFeatured) and
// API.HomeBackground (the image, its dimming, blur and focus; 06 §2.14).
import { S, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, changesPlan, beginPageRead } from "./screens.js";
import { launchPack } from "./packs.js";
import { withEnvironmentSwitch } from "./environment.js";
import { places, slideFrom } from "./motion.js";

let ui = null;
export function initHome(u) {
  ui = u;
  // Back from a game: the page reads the new Player.log (API.Home reads it when it is newer
  // than the last one read).
  addEventListener("focus", () => { if (S.screen === "home") home(); });
  addEventListener("resize", () => { if (S.screen === "home") { markY.clear(); placeMarks(); fitHeadline(); } });
}

// the hidden problem's fingerprint (a GUI setting, so it stays hidden across restarts)
const HIDDEN_KEY = "ui.gui.home.hidden";
let data = null;
let bg = null;
let hidden = null;
let pendingRow = null; // latest pick while the previous page data remains visible
let launching = 0;     // until when ▶ says it is starting (the live state takes over)
let arrived = false;   // the last apply just finished: ▶ shows the new pick coming in
const picks = new Map(); // game → the row last applied in it this session
// what was shown last, so a redraw animates only what moved
const shown = { game: null, row: null, note: false, drawn: false };
const markY = new Map();

let homeRetries = 0, homeRetryTimer = null;
export async function home({ keep = false } = {}) {
  clearTimeout(homeRetryTimer);
  const read = beginPageRead("home", { keep, retry: () => home() });
  let next, backdrop, h;
  try {
    [next, backdrop, h] = await Promise.all([api.Home(), api.HomeBackground(), hidden == null ? api.Setting(HIDDEN_KEY).catch(() => "") : hidden]);
  } catch (e) { read.fail(e); retryHome(); return; }
  if (!read.current()) return;
  // An empty page while games exist is a read that failed (a background job holding the
  // data), not "no game yet": keep what was shown, say so, and read again.
  const games = (S.shell?.envs || 0) > 0 || (S.board?.envs?.length || 0) > 0;
  if (next?.error || (games && !(next?.rows?.length))) {
    const why = next?.error || T("could not read the games right now (a background task may be busy); trying again…");
    if (rows().length) { draw(); read.finish(); say("busy", why); } else read.fail(new Error(why));
    retryHome();
    return;
  }
  homeRetries = 0;
  data = next; bg = backdrop; hidden = h || "";
  const f = applied();
  if (f) picks.set(f.envName, key(f));
  draw();
  read.finish();
}

// retryHome reads the page again after 1, 2, 4, 8 s, then every 10 s, for about a minute
function retryHome() {
  if (homeRetries >= 10) return;
  const wait = Math.min(1000 * 2 ** homeRetries, 10000);
  homeRetries++;
  homeRetryTimer = setTimeout(() => { if (S.screen === "home") home({ keep: true }); }, wait);
}

const rows = () => (data && data.rows) || [];
const applied = () => (data && data.featured >= 0 ? rows()[data.featured] : null);
const running = () => !!(S.live && (S.live.running || S.live.blocked));
const key = (r) => [r.kind, r.envName, r.plan || "", r.kind === "pack" ? r.name : ""].join("|");
// a game's rows: its lists, then its packs, each by the last time played (view.SortHome)
const rowsOf = (env) => rows().filter((r) => r.envName === env).sort((a, b) => (a.kind === b.kind ? 0 : a.kind === "plan" ? -1 : 1));
// shownFirst puts the applied one at the top of its column; ↑↓ and ←→ still step through
// the columns' own order, so the keys do not bounce between the top two.
const shownFirst = (list, on) => { const i = list.findIndex(on); return i > 0 ? [list[i], ...list.slice(0, i), ...list.slice(i + 1)] : list; };
// the games with something on the page, by name; short is the chip's version ("1.7"),
// full the build ("1.7.4990 rev1069")
function games() {
  const seen = new Map();
  for (const r of rows()) {
    if (seen.has(r.envName)) continue;
    const p = r.envName + "  ";
    const chip = r.env && r.env.startsWith(p) ? r.env.slice(p.length) : "";
    const full = r.gameVersion || chip;
    seen.set(r.envName, { name: r.envName, version: full, short: chip || (/^\d+\.\d+/.exec(full) || [""])[0] });
  }
  return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
}
const gameOf = (r) => games().find((g) => g.name === r.envName) || { name: r.envName, version: "" };

// Numbered columns are focus keys (06 §2.3, 06 §3.5): ↑↓ in 01 changes games;
// elsewhere it applies lists and packs. Enter on 03 launches, elsewhere it opens the row.
function focusedStep() { return document.activeElement?.closest(".hm-step")?.dataset.step; }

function focusStep(n) {
  const s = document.querySelector(`.page.home .hm-s${n}`);
  if (!s) return;
  const target = s.querySelector(".hm-pick.on") || s.querySelector(".hm-pick") || (!S.busy.env && s.querySelector(".hm-go:not(:disabled)")) || s;
  target.focus();
}

export function homeKey(e) {
  const k = e.key, f = pendingRow || applied();
  if (/^[1-3]$/.test(k) && !e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) { e.preventDefault(); focusStep(k); return; }
  if (k === "Enter" && document.activeElement?.matches(".hm-go, .hm-s3")) { e.preventDefault(); document.querySelector(".hm-go")?.click(); return; }
  if (!f) return;
  const vertical = k === "ArrowDown" || k === "ArrowUp";
  if (vertical && focusedStep() !== "1") {
    e.preventDefault();
    const list = rowsOf(f.envName), i = list.findIndex((r) => key(r) === key(f));
    if (list.length > 1) apply(list[(i + (k === "ArrowDown" ? 1 : list.length - 1)) % list.length]);
    return;
  }
  if (vertical || k === "ArrowLeft" || k === "ArrowRight") {
    e.preventDefault();
    const gs = games(), i = gs.findIndex((g) => g.name === f.envName);
    if (gs.length > 1) pickGame(gs[(i + (k === "ArrowRight" || k === "ArrowDown" ? 1 : gs.length - 1)) % gs.length].name);
    return;
  }
  if (k === "Enter") { e.preventDefault(); edit(f); return; }
  if (k !== "L" && !(k === "l" && e.shiftKey)) return;
  if (running()) return;
  e.preventDefault();
  launch(f);
}

export function hintList() {
  return [{ key: "1–3", label: T("column") }, { key: "↑↓", label: T("list") }, { key: "←→", label: T("game") }, { key: "enter", label: T("edit") }, { key: "L", label: T("launch") },
    { key: "shift+1–8", label: T("pages") }, { key: "?", label: T("all keys") }];
}

// focusLaunch is the editor's 启动游戏 (06 §2.5): the current plan becomes the applied row
// (it usually is; a pack applied here gives way) and the focus goes to ▶ 启动游戏, so L or
// enter starts the game.
export async function focusLaunch() {
  if (!data) return;
  const f = applied();
  if (!f || f.kind !== "plan" || !f.current) {
    const r = rows().find((x) => x.kind === "plan" && x.current);
    if (r) await apply(r);
  }
  focusStep(3);
}

// liveChanged redraws the launch button when the game starts or stops.
export function liveChanged() { if (S.screen === "home" && data) { launching = 0; draw(); } }

function draw() {
  const focus = focusedStep();
  const sc = S.screen === "home" ? document.querySelector("#screen") : openScreen("home");
  // where the rows were, so a row that moves to the top slides there
  const before = places(sc.querySelector(".page.home"), ".hm-pick");
  sc.innerHTML = "";
  setBackdrop(bg);
  // no page actions: rescanning is Mod 库's, a new plan is 方案's
  const w = page("home", "");
  sc.append(w);
  const f = applied();
  if (!rows().length) {
    const e = el("div", "hm-empty");
    const add = el("button", "btn", T("add a game"));
    add.onclick = () => ui.go("env");
    e.append(el("h1", "", T("no game yet: add one on the games page")), add);
    w.append(e);
    return;
  }
  const env = f ? f.envName : games()[0].name;
  const rowMoved = !f || shown.row !== key(f), gameMoved = shown.game !== env;
  // the floor goes first so everything paints over it: over a picture it gives the three
  // steps a flat ground (06 §2.3)
  w.append(el("div", "hm-floor"), activity(), hero(f, rowMoved));
  w.append(el("div", "hm-rule" + (shown.drawn ? "" : " in")));
  w.append(gameStep(env), rowStep(f, env, gameMoved), launchStep(f));
  w.querySelectorAll(".hm-picks").forEach((l) => l.classList.toggle("scrolls", l.querySelectorAll(".hm-pick").length > 6));
  slideFrom(w, ".hm-pick", before);
  fitHeadline();
  placeMarks(gameMoved);
  if (pendingRow && S.busy.env) showSwitch(pendingRow);
  if (focus) focusStep(focus);
  shown.game = env; shown.row = f ? key(f) : null; shown.drawn = true;
  shown.note = !!w.querySelector(".hm-note");
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

// splitTags takes the trailing "[…]" groups off a name ("The Progression Modpack [UPDATED
// 1.7]"): Workshop tags, which go on the line under the headline. A name that is only
// tags stays whole.
function splitTags(name) {
  const tags = [];
  let stem = name, m;
  while ((m = /^(.*\S)\s*\[([^[\]]+)\]\s*$/.exec(stem))) { tags.unshift(m[2].trim()); stem = m[1]; }
  return [stem, tags];
}

// hero: the applied row as the page's one headline.
function hero(f, moved) {
  const box = el("section", "hm-hero");
  if (!f) {
    const head = el("div", "hm-headline");
    head.append(el("h1", "", T("pick one and play.")));
    box.append(head);
    return box;
  }
  const [stem, tags] = splitTags(f.name);
  const head = el("div", "hm-headline");
  const h1 = el("h1", moved ? "in" : "", stem);
  h1.title = f.name;
  head.append(h1);
  const facts = el("div", "hm-facts num" + (moved ? " in" : ""));
  for (const t of tags) facts.append(el("span", "", t));
  facts.append(el("span", "word-facts", f.facts || Tf("%d mods", f.mods)));
  if (f.played) facts.append(el("span", "", f.played));
  box.append(head, facts);
  return box;
}

// fitHeadline sets the headline as large as its width allows (06 §2.3): one line, from the
// largest size down to 64px; still too long, two lines, from 64px down to 48px. Only a
// name longer than that ends in an ellipsis (the whole name is on hover).
function fitHeadline() {
  const h1 = document.querySelector(".page.home .hm-headline h1");
  if (!h1) return;
  h1.classList.remove("two");
  h1.style.fontSize = "";
  const max = parseFloat(getComputedStyle(h1).fontSize), min = Math.min(max, 64);
  let size = max;
  for (let i = 0; i < 4 && h1.scrollWidth > h1.clientWidth + 1 && size > min; i++) {
    size = Math.max(min, Math.floor(size * h1.clientWidth / h1.scrollWidth));
    h1.style.fontSize = size + "px";
  }
  if (h1.scrollWidth <= h1.clientWidth + 1) return;
  h1.classList.add("two");
  // a third line adds about a line's height; glyphs below the tight line height add less
  for (let i = 0; i < 4 && h1.scrollHeight > h1.clientHeight + size * .4 && size > 48; i++) {
    size = Math.max(48, size - 4);
    h1.style.fontSize = size + "px";
  }
}

function step(n, cls, title) {
  const s = el("section", "hm-step " + cls);
  s.dataset.step = String(Number(n));
  s.tabIndex = -1; // Disabled launch controls leave their column reachable (06 §2.3).
  const h = el("h3");
  h.append(el("b", "num", n), title);
  s.append(h);
  return s;
}

// 01: the games, the applied one first; picking one applies its last row (this session's
// pick, else its most recently played). Each says its short version unless its name
// already does ("1.6"); the full build is on hover.
function gameStep(env) {
  const s = step("01", "hm-s1", T("game"));
  const list = el("div", "hm-picks");
  list.append(el("span", "hm-mark"));
  for (const g of shownFirst(games(), (g) => g.name === env)) {
    const b = el("button", "hm-pick" + (g.name === env ? " on" : ""));
    b.dataset.game = g.name;
    b.dataset.id = "game|" + g.name;
    b.title = g.version;
    b.append(el("span", "nm", g.name), el("small", "num", g.short && !g.name.includes(g.short) ? g.short : ""));
    b.onclick = () => pickGame(g.name);
    list.append(b);
  }
  s.append(list);
  return s;
}

// 02: the game's lists and packs, the applied one first; picking one applies it, picking
// the applied one opens it.
function rowStep(f, env, moved) {
  const s = step("02", "hm-s2", T("list or pack"));
  const list = el("div", "hm-picks" + (moved ? " enter" : ""));
  list.append(el("span", "hm-mark"));
  shownFirst(rowsOf(env), (r) => f && key(r) === key(f)).forEach((r, i) => {
    const on = f && key(r) === key(f);
    const b = el("button", "hm-pick" + (on ? " on" : ""));
    b.dataset.key = key(r);
    b.dataset.id = "row|" + key(r);
    b.style.setProperty("--d", i);
    const nm = el("span", "nm", r.name);
    // Only the applied row is marked, and 03 says what is wrong; another row's state is
    // on hover (06 §2.3).
    if (on && r.level === "bad") nm.append(el("span", "flag bad", "▲"));
    else if (on && r.level !== "ok") nm.append(el("span", "flag", "○"));
    // the count alone for a list; a pack says it is one
    b.append(nm, el("small", "num word-facts", r.kind === "pack" ? T("pack") + "  " + r.mods : String(r.mods)));
    const use = on ? T("pick again to open it") : r.otherEnv ? Tf("switch to %s and use this; no game file is written", r.envName) : T("use this; no game file is written");
    b.title = !on && r.level !== "ok" && r.state ? r.state + "\n" + use : use;
    b.onclick = () => (on ? edit(r) : apply(r));
    list.append(b);
  });
  s.append(list);
  return s;
}

// 03: ▶ 启动游戏 (the one block of colour on the page) and the problem under it.
function launchStep(f) {
  const s = step("03", "hm-s3", T("launch"));
  const live = running(), starting = !live && launching > Date.now();
  const b = el("button", "hm-go" + (live ? " running" : "") + (starting ? " starting" : "") + (arrived ? " arrived" : ""));
  b.dataset.envWrite = "launch";
  arrived = false;
  const top = el("div", "top");
  const title = el("span", "title", live ? T("the game is running") : starting ? T("starting…") : T("launch the game"));
  if (live) title.prepend(el("span", "dot"));
  top.append(title);
  if (!live) top.append(el("kbd", "", "L"));
  // 01 and 02 already say what starts; the block adds only the edits that launching saves
  const sub = live ? (S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet")) : f && f.unsaved ? Tf("%d unsaved", f.unsaved) : "";
  const bottom = el("div", "bottom");
  bottom.append(el("span", "sub num", sub));
  if (!live) bottom.append(el("span", "arrow", "→"));
  b.append(top, bottom);
  // one game at a time (08 §4)
  b.disabled = live || !f;
  if (live) b.title = T("a game is already running");
  else if (f && f.unsaved) b.title = T("different from the last saved list; launching or saving saves these changes to the game");
  b.onclick = () => f && launch(f);
  s.append(b);
  const n = f && note(f);
  if (n) s.append(n);
  return s;
}

// problemOf: what is wrong with the applied row; null when all is well. The current plan's
// most likely trouble of a launch (view.LaunchHint: the most predictive problem in the
// shared logs, with its reason), else problems that break the game, else a pack's links
// left in Mods/ (08 §5.3), else an outside version not loaded. It never blocks the launch.
function problemOf(r) {
  const h = r.current && data.launch;
  if (h) return { what: h.text, why: h.why, bad: true, go: true, id: h.code + ":" + h.count };
  if (r.level === "bad") return { what: r.state, bad: true, go: r.current };
  if (r.links) return { what: r.links, go: r.current };
  if (r.mark) return { what: r.mark };
  return null;
}
// fingerprint: the row, its game, its mods and its problem; any change shows the problem again
const fingerprint = (r, p) => [key(r), r.env, r.facts, r.versions, r.state, p.what, p.id || "", (data.changes && data.changes.text) || ""].join("␟");

function note(r) {
  const p = problemOf(r);
  if (!p) return null;
  const fp = fingerprint(r, p);
  if (hidden === fp) return null;
  const wrap = el("div", "hm-note" + (p.bad ? " bad" : "") + (shown.note ? "" : " enter"));
  const inner = el("div"), box = el("div", "box");
  // one line; the reason and its log numbers are on hover and on 能不能跑
  const what = el(p.go ? "button" : "div", "what", (p.bad ? "▲  " : "") + p.what);
  what.title = [p.what, p.why].filter(Boolean).join("\n");
  if (p.go) what.onclick = () => ui.go("needs");
  box.append(what);
  const x = el("button", "x", "×");
  x.title = T("hide until the list, the game or a mod changes");
  x.setAttribute("aria-label", T("hide"));
  x.onclick = async () => {
    wrap.classList.add("leaving");
    hidden = fp;
    api.SetSetting(HIDDEN_KEY, fp).catch(() => {});
    setTimeout(() => { if (S.screen === "home") draw(); }, 300);
    say("", T("hidden; it comes back when the list, the game or a mod changes"));
  };
  box.append(x);
  inner.append(box);
  wrap.append(inner);
  return wrap;
}

// placeMarks moves each step's square to its picked row, sliding from where it was; a
// game's new rows start with the square in place.
function placeMarks(gameMoved) {
  document.querySelectorAll(".page.home .hm-picks").forEach((list, i) => {
    const m = list.querySelector(".hm-mark"), on = list.querySelector(".hm-pick.on");
    if (!m) return;
    m.hidden = !on;
    if (!on) return;
    const y = on.offsetTop + on.offsetHeight / 2 - 4;
    const from = markY.has(i) && !(i === 1 && gameMoved) ? markY.get(i) : y;
    m.style.transition = "none";
    m.style.transform = `translateY(${from}px)`;
    m.getBoundingClientRect();
    m.style.transition = "";
    m.style.transform = `translateY(${y}px)`;
    markY.set(i, y);
  });
}

function edit(r) { ui.go(r.kind === "plan" ? r.page : "pack:" + r.name); }

function pickGame(env) {
  const f = applied();
  if (!S.busy.env && f && f.envName === env) return;
  const want = picks.get(env);
  const r = rows().find((x) => key(x) === want) || rows().find((x) => x.envName === env);
  if (r) apply(r);
}

// apply: 应用 (06 §2.3) — the row becomes the applied one. The page stays as it is: the
// squares move to the pick at once, and ▶ shows the switch (another game: "switching to
// main…", which can take a while; another row: "applying X…") until the new page is read,
// then the new pick comes in on it.
async function apply(r) {
  pendingRow = r;
  showSwitch(r);
  try {
    const switched = await withEnvironmentSwitch(r.envName, async (current) => {
      const result = await api.HomeApply(r.kind, r.envName, r.plan || 0, r.kind === "pack" ? r.name : "");
      if (!current() || result.stale) return;
      picks.set(r.envName, key(r));
      await ui.reload(result);
      if (!current()) return;
      pendingRow = null;
      arrived = true;
      await ui.refreshEnvironment();
      if (current()) say("ok", result.text);
    });
    if (switched) {
      pendingRow = null;
      if (focusedStep() === "3") focusStep(3);
    }
  } catch (e) {
    pendingRow = null;
    arrived = false;
    if (S.screen === "home" && data) draw();
    say("fail", errText(e));
  }
}

// showSwitch marks the pick in 01 and 02 and turns ▶ into the switch's progress.
function showSwitch(r) {
  const root = document.querySelector(".page.home");
  if (!root) return;
  const gameMoved = !applied() || applied().envName !== r.envName;
  root.querySelectorAll(".hm-s1 .hm-pick").forEach((b) => b.classList.toggle("on", b.dataset.game === r.envName));
  if (!gameMoved) root.querySelectorAll(".hm-s2 .hm-pick").forEach((b) => b.classList.toggle("on", b.dataset.key === key(r)));
  else root.querySelectorAll(".hm-s2 .hm-pick").forEach((b) => b.classList.add("leaving"));
  placeMarks(false);
  const b = root.querySelector(".hm-go");
  if (!b) return;
  // Keep 03 focused while its launch control is disabled (06 §2.3).
  if (document.activeElement === b) b.closest(".hm-step").focus();
  b.className = "hm-go switching";
  b.disabled = true;
  const top = el("div", "top"), bottom = el("div", "bottom");
  top.append(el("span", "title", gameMoved ? Tf("switching to %s…", r.envName) : Tf("applying %s…", r.name)));
  bottom.append(el("span", "sub num", gameMoved ? T("loading the game, please wait…") : ""));
  const bar = el("span", "bar");
  bar.setAttribute("role", "progressbar");
  bar.setAttribute("aria-label", top.textContent);
  b.replaceChildren(top, bottom, bar);
  b.setAttribute("aria-busy", "true");
}

// activity: the last game (only once there was one) and the current plan's changes since
// it was last played, each a line that is a link (06 §2.3).
function activity() {
  const box = el("div", "hm-act");
  const live = S.live && S.live.running;
  if (live || data.lastRun) {
    const runText = live ? T("the game is running") + "  " + (S.live.errors ? Tf("%d errors", S.live.errors) : T("no errors yet"))
      : (data.lastRunDay || data.lastRun) + "  " + (data.lastRunErrors ? Tf("%d errors", data.lastRunErrors) : T("no errors"));
    const runGo = live || data.lastRunErrors ? "needs" : "log";
    box.append(actLine(T("last game"), runText, () => ui.go(runGo)));
  }
  // "nothing changed" tells nothing: the line shows only with changes or before a scan
  const c = data.changes;
  if (c && (c.count || c.noScan)) {
    const go = c.noScan ? () => ui.cmd("rescan") : () => { changesPlan(); ui.go("changes"); };
    box.append(actLine(c.title, c.text, go));
  }
  return box;
}

function actLine(title, text, go) {
  const b = el("button", "hm-actline");
  b.append(el("span", "t", title), el("span", "x word-facts", text), el("span", "go", "→"));
  b.title = text;
  b.onclick = go;
  return b;
}

// launch is the applied row's L (08 §2, §5.3): the plan launches at once, as the pack
// does on the packs page. ▶ says it is starting until the game's live state arrives.
async function launch(r) {
  if (S.busy.env) return say("busy", Tf("switching to %s…", S.busy.env));
  launching = Date.now() + 4000;
  draw();
  setTimeout(() => { if (S.screen === "home" && launching && launching <= Date.now()) { launching = 0; draw(); } }, 4100);
  try {
    if (r.kind !== "plan") await launchPack(r.name);
    else if (await ui.run("launch") === "asked") { launching = 0; draw(); }
  } catch (e) { launching = 0; draw(); say("fail", errText(e)); }
}
