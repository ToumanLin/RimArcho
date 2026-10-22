// The first-run guide (06 §2.15): three steps, each with a way back.
//   1. find the game: 已有的游戏 — what detection finds (tick several), a folder typed in or
//      chosen in the folder dialog; or 下载一个版本 — a branch, a folder and the Steam account
//      name, downloaded by SteamCMD in its own window (08 §10);
//   2. read the mods: a scan per environment in a child process, with its progress (a game
//      added once the shared mod folders were read: only its own Data/ and Mods/);
//   3. where to start: the game's own list, a list from RimSort / RimCrow (previewed, then
//      applied: view.ReadImport → PreviewImport → ApplyImport), or a pack / a collection.
// The environment page's "add an environment" starts at step 1.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button, seg, pickFolder } from "./screens.js";
import { exchange, importFrom } from "./exchange.js";

let ui = null;
export function initGuide(u) { ui = u; }

const STEPS = ["find the game", "read the mods", "where to start"];
const G = { step: 0, way: 0, games: [], picked: new Set(), added: [], scan: null, typed: "", error: "" };
// The download way: SteamCMD's info (DownloadInfo), the choices, and a run's progress.
const D = { info: null, loading: false, branch: "", dir: "", dirTyped: false, account: "", run: null };

// start opens the guide at step 1; from is "first" (no environment yet) or "env".
export async function start(from = "first") {
  S.guideFrom = from;
  G.step = 0;
  G.added = [];
  G.scan = null;
  G.error = "";
  G.picked.clear();
  if (D.run && D.run.end) D.run = null;
  G.way = D.run ? 1 : 0;
  openScreen("guide");
  await find();
}

async function find() {
  try { G.games = (await api.GuideFind()) || []; } catch (e) { G.games = []; say("fail", errText(e)); }
  G.games.forEach((g, i) => { if (!g.env) G.picked.add(i); });
  draw();
}

function draw() {
  if (S.screen !== "guide") return;
  const sc = $("#screen");
  sc.innerHTML = "";
  const lead = S.guideFrom === "env" ? T("find the install, read its mods once, and it is ready") : T("find the game, read the mods once, then pick where your first plan comes from");
  const w = page("guide", lead);
  sc.append(w);
  // the steps: where you are, where you can go back to
  const bar = el("div", "steps");
  STEPS.forEach((s, i) => {
    if (i) bar.append(el("span", "line" + (i <= G.step ? " on" : "")));
    const b = el("button", "st" + (i === G.step ? " cur" : i < G.step ? " done" : ""));
    b.append(el("span", "n", i < G.step ? "✓" : String(i + 1)), el("span", "t", T(s)));
    b.disabled = i >= G.step || (i === 1 && !G.added.length);
    b.onclick = () => go(i);
    bar.append(b);
  });
  w.append(bar);
  const card = el("section", "card guide-card");
  w.append(card);
  [step1, step2, step3][G.step](card);
  ui.renderWhere();
}

function go(i) {
  G.step = i;
  G.error = "";
  draw();
}

function footer(card, ...btns) {
  const f = el("div", "guide-foot");
  if (G.error) f.append(el("span", "err grow", G.error)); else f.append(el("span", "grow"));
  if (G.step > 0) f.append(button(T("back"), "", () => go(G.step - 1)));
  for (const b of btns) if (b) f.append(b);
  card.append(f);
}

// ---- 1. find the game ----

function step1(card) {
  card.append(el("div", "guide-q", T("which RimWorld installs should RimArcho manage?")));
  const sw = seg([T("a game you have"), T("download a version")], G.way, (i) => {
    G.way = i;
    G.error = "";
    draw();
    if (i === 1 && !D.info && !D.loading) loadDownload();
  });
  sw.classList.add("guide-way");
  card.append(sw);
  if (G.way === 1) return download(card);
  const list = el("div", "games");
  if (!G.games.length) list.append(el("div", "empty-state", T("no RimWorld found in the Steam libraries or the usual folders; type the game folder below")));
  G.games.forEach((g, i) => {
    const r = el("label", "game" + (g.env ? " used" : ""));
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = !g.env && G.picked.has(i);
    cb.disabled = !!g.env;
    cb.onchange = () => { cb.checked ? G.picked.add(i) : G.picked.delete(i); draw(); };
    const body = el("div", "b");
    const top = el("div", "top");
    top.append(el("span", "v", "RimWorld " + (g.version || "—")), el("span", "chip", g.launch === "steam" ? "Steam" : T("started directly")));
    if (g.env) top.append(el("span", "chip cur", Tf("already environment %s", g.env)));
    body.append(top, el("div", "path", g.game));
    if (!g.env) {
      const name = el("input", "field small");
      name.value = g.name;
      name.spellcheck = false;
      name.title = T("the environment's name");
      name.onclick = (e) => e.preventDefault();
      name.oninput = () => { g.name = name.value.trim(); };
      const nm = el("div", "nm");
      nm.append(el("span", "muted", T("name")), name);
      body.append(nm);
    }
    r.append(cb, body);
    list.append(r);
  });
  card.append(list);
  // a folder typed in or chosen in the folder dialog (detection found nothing, or not this one)
  const typed = el("div", "typed");
  const inp = el("input", "field");
  inp.placeholder = T("the game folder (the one with RimWorldWin64.exe)");
  inp.spellcheck = false;
  inp.value = G.typed;
  inp.oninput = () => { G.typed = inp.value; };
  const addPath = async (path) => {
    let g;
    try { g = await api.GuideGame(path); } catch (e) { G.typed = path; G.error = errText(e); return draw(); }
    G.typed = "";
    G.error = "";
    if (!G.games.some((x) => x.game.toLowerCase() === g.game.toLowerCase())) {
      G.games.push(g);
      if (!g.env) G.picked.add(G.games.length - 1);
    }
    draw();
  };
  const add = button(T("add this folder"), "", () => addPath(inp.value));
  const choose = button(T("choose…"), "", () => pickFolder(inp.value, addPath));
  inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); add.click(); } };
  typed.append(inp, choose, add);
  card.append(typed);
  const picked = [...G.picked].filter((i) => G.games[i] && !G.games[i].env);
  const next = button(picked.length > 1 ? Tf("add %d environments", picked.length) : T("add and continue"), "", async () => {
    next.disabled = true;
    let names;
    try { names = await api.GuideAdd(picked.map((i) => G.games[i])); } catch (e) { G.error = errText(e); return draw(); }
    added(names || []);
  }, "primary");
  next.disabled = !picked.length;
  footer(card, G.added.length ? button(T("next"), "", () => go(1)) : null, next);
}

// added: environments were added (found, typed or downloaded); read their mods.
async function added(names) {
  G.added = names;
  G.games = (await api.GuideFind()) || G.games;
  G.picked.clear();
  await ui.reload();
  scanAll();
}

// ---- 1b. download a version (SteamCMD, 08 §10) ----

async function loadDownload() {
  D.loading = true;
  draw();
  try { D.info = await api.GuideDownloadInfo(); } catch (e) { D.info = { error: errText(e) }; }
  D.loading = false;
  const bs = (D.info && D.info.branches) || [];
  if (bs.length && !bs.some((b) => b.name === D.branch)) D.branch = bs[0].name;
  if (!D.dirTyped) D.dir = suggestDir();
  draw();
}

function branchLabel(b) {
  if (b.name === "public") return T("current version");
  if (b.name === "unstable") return T("test version (unstable)");
  if (b.version) return "RimWorld " + b.name.replace(/^version-/, "");
  return b.name;
}

// suggestDir is <drive>\Games\RimWorld <version> for the chosen branch.
function suggestDir() {
  const b = ((D.info && D.info.branches) || []).find((x) => x.name === D.branch);
  const games = (D.info && D.info.games) || "";
  const v = b ? (b.version || (b.name === "public" ? "" : b.name)) : "";
  return games ? games + "\\RimWorld" + (v ? " " + v : "") : "";
}

function download(card) {
  if (D.run) return downloading(card, D.run);
  if (D.loading || !D.info) {
    card.append(el("div", "dl-note", T("asking Steam which versions RimWorld has… (SteamCMD may update itself first)")));
    const prog = el("div", "prog");
    const b = el("div", "bar busy");
    b.append(el("i"));
    prog.append(b);
    card.append(prog);
    return footer(card);
  }
  const info = D.info;
  if (!info.steamcmd) {
    const o = info.offer || {};
    const size = o.bytes > 0 ? sizeText(o.bytes) : T("size unknown");
    card.append(el("div", "dl-note", Tf("RimArcho downloads game versions with Valve's SteamCMD, which is not on this computer. Download it from Valve (%s) into %s?", size, o.dir || "")));
    const get = button(T("download SteamCMD"), "", async () => {
      get.disabled = true;
      try { await api.GuideInstallSteamCMD(); } catch (e) { G.error = errText(e); return draw(); }
      G.error = "";
      D.info = null;
      loadDownload();
    }, "primary");
    return footer(card, get);
  }
  if (info.error && !(info.branches || []).length) {
    card.append(el("div", "dl-note err", info.error));
    return footer(card, button(T("ask again"), "", () => { D.info = null; loadDownload(); }));
  }
  const list = el("div", "branches");
  for (const b of info.branches || []) {
    const row = el("button", "branch" + (b.name === D.branch ? " on" : ""));
    row.append(el("span", "v", branchLabel(b)), el("span", "muted", b.description || b.name));
    if (b.updated && !b.updated.startsWith("0001")) row.append(el("span", "when", b.updated.slice(0, 10)));
    row.onclick = () => { D.branch = b.name; if (!D.dirTyped) D.dir = suggestDir(); draw(); };
    list.append(row);
  }
  card.append(list);
  const where = el("div", "typed");
  const dir = el("input", "field");
  dir.placeholder = T("the folder for the game");
  dir.spellcheck = false;
  dir.value = D.dir;
  dir.oninput = () => { D.dir = dir.value; D.dirTyped = true; };
  where.append(el("span", "muted lbl", T("game folder")), dir, button(T("choose…"), "", () => pickFolder(dir.value, (p) => { D.dir = p; D.dirTyped = true; draw(); })));
  card.append(where);
  const who = el("div", "typed");
  const acc = el("input", "field");
  acc.placeholder = T("your Steam account name");
  acc.spellcheck = false;
  acc.autocomplete = "username";
  acc.value = D.account;
  acc.oninput = () => { D.account = acc.value.trim(); };
  who.append(el("span", "muted lbl", T("Steam account")), acc);
  card.append(who);
  card.append(el("div", "dl-note", T("SteamCMD opens a window of its own: type the password and the Steam Guard code there. RimArcho never sees them; it only gives SteamCMD the account name.")));
  const start1 = button(T("download and add"), "", async () => {
    if (!D.account) { G.error = T("type your Steam account name"); return draw(); }
    let ok;
    try { ok = await api.GuideDownload(D.branch, D.dir, D.account, ""); } catch (e) { G.error = errText(e); return draw(); }
    if (!ok) { G.error = T("a download is already running"); return draw(); }
    G.error = "";
    D.run = { state: "starting", percent: 0, line: "", end: false };
    draw();
  }, "primary");
  start1.disabled = !D.branch || !D.dir;
  acc.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); start1.click(); } };
  footer(card, start1);
}

const DL_STATES = { starting: "starting SteamCMD…", login: "logging in (in the SteamCMD window)…", downloading: "downloading", verifying: "checking the files", done: "done" };

function downloading(card, r) {
  const t = r.end ? (r.ok ? Tf("downloaded; environment %s added", r.env) : T("the download did not finish")) : T(DL_STATES[r.state] || r.state);
  const head = el("div", "dl-state");
  head.append(el("span", "t", t));
  if (!r.end && r.total > 0) head.append(el("span", "muted", Tf("%s of %s", sizeText(r.done), sizeText(r.total))));
  card.append(head);
  const prog = el("div", "prog");
  const bar = el("div", "bar" + (r.end || r.total > 0 ? "" : " busy"));
  const fill = el("i");
  fill.style.width = (r.end && r.ok ? 100 : Math.round(r.percent || 0)) + "%";
  bar.append(fill);
  prog.append(bar);
  card.append(prog);
  if (r.error) card.append(el("div", "dl-note err", r.error));
  else if (r.line) card.append(el("div", "dl-line", r.line));
  if (r.end && !r.ok) return footer(card, button(T("try again"), "", () => { D.run = null; draw(); }));
  footer(card);
}

// downloadEvent is a "download" event: SteamCMD's progress, then the end.
export function downloadEvent(ev) {
  if (!D.run) return;
  if (ev.end) {
    Object.assign(D.run, { end: true, ok: ev.ok, error: ev.error || "", env: ev.env || "" });
    draw();
    if (ev.ok) added([ev.env]);
    return;
  }
  Object.assign(D.run, ev);
  draw();
}

function sizeText(n) {
  if (!(n > 0)) return "0";
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i ? n.toFixed(1) : String(n)) + " " + u[i];
}

// ---- 2. read the mods ----

async function scanAll() {
  G.step = 1;
  G.scan = { rows: G.added.map((n) => ({ env: n, text: T("waiting"), state: "wait" })), done: false, ok: true };
  draw();
  if (!(await api.GuideScan(G.added))) {
    G.scan.done = true;
    G.error = T("a scan is already running; wait for it to finish");
    draw();
  }
}

// guideEvent is a "guide" event: an environment started, ended, or all are done.
export function guideEvent(ev) {
  if (!G.scan) return;
  if (ev.done) {
    G.scan.done = true;
    G.scan.ok = ev.ok;
    ui.reload();
    return draw();
  }
  const r = G.scan.rows[ev.i];
  if (!r) return;
  if (ev.step) { r.state = ev.ok ? "ok" : "bad"; r.text = ev.ok ? ev.text : ev.error; } else { r.state = "cur"; r.text = ev.text; }
  draw();
}

function step2(card) {
  card.append(el("div", "guide-q", T("RimArcho reads every mod folder once; later scans only read what changed")));
  const s = G.scan || { rows: [], done: true };
  const list = el("div", "scanlist");
  for (const r of s.rows) {
    const d = el("div", "scanrow " + r.state);
    d.append(el("span", "n", r.state === "ok" ? "✓" : r.state === "bad" ? "✗" : r.state === "cur" ? "…" : "·"), el("span", "env", r.env), el("span", "t", r.text));
    list.append(d);
  }
  card.append(list);
  const doneN = s.rows.filter((r) => r.state === "ok" || r.state === "bad").length;
  const prog = el("div", "prog");
  const bar = el("div", "bar" + (s.done ? "" : " busy"));
  const fill = el("i");
  fill.style.width = (s.rows.length ? Math.round(100 * doneN / s.rows.length) : 100) + "%";
  bar.append(fill);
  prog.append(bar);
  card.append(prog);
  const again = s.done && !s.ok ? button(T("scan again"), "", () => scanAll()) : null;
  const next = button(T("next"), "", () => go(2), "primary");
  next.disabled = !s.done;
  footer(card, again, next);
}

// ---- 3. where to start ----

function step3(card) {
  card.append(el("div", "guide-q", T("where does your first plan come from?")));
  const ways = el("div", "ways");
  const n = S.board && S.board.cols ? (S.board.cols[1] || []).length : 0;
  ways.append(way(T("the game's current list"), Tf("ModsConfig.xml enables %d mods; it becomes your first plan", n),
    button(T("use this list"), "", () => finish(T("the game's list is your first plan")), "primary")));
  const imp = button(T("import a list…"), "", () => { ui.afterImport = () => finish(""); exchange(0); });
  ways.append(way(T("move over from RimSort or RimCrow"), T("a list file, a share code or pasted text; you see what changes before it is applied"), imp));
  const src = el("input", "field");
  src.placeholder = T("a pack file, or a Steam Workshop collection link");
  src.spellcheck = false;
  const open = button(T("open"), "", async () => {
    const v = src.value.trim();
    if (!v) return src.focus();
    if (/steamcommunity\.com|^\s*\d{6,}\s*$/.test(v)) { ui.afterImport = () => finish(""); return importFrom("text", v); }
    try { finish(await api.PackImport(v)); } catch (e) { G.error = errText(e); draw(); }
  });
  src.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); open.click(); } };
  const row = el("div", "typed");
  row.append(src, open);
  ways.append(way(T("open a pack or a Steam collection"), T("a pack someone shared (.archopack or pack.json), or a Workshop collection"), row));
  card.append(ways);
  footer(card, button(T("skip"), "", () => finish("")));
}

function way(title, text, action) {
  const d = el("div", "way");
  d.append(el("div", "t", title), el("div", "muted", text), action);
  return d;
}

async function finish(text) {
  ui.afterImport = null;
  G.scan = null;
  if (text) say("ok", text);
  await ui.reload();
  ui.go("home");
}
