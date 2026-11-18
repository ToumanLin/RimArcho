import { fieldError, progress } from "./feedbacksemantics.js";
import { editablePath, copyablePath, pathContent } from "./fields.js";
// The first-run guide (16 §2.15): three steps, each with a way back.
//   1. find the game: existing games — what detection finds (tick several), or a folder
//      chosen in the native dialog; or download a version — a branch, a folder and an optional
//      account override, downloaded by SteamCMD in its own window (11 §3);
//   2. read the mods: a scan per environment in a child process, with its progress (a game
//      added once the shared mod folders were read: only its own Data/ and Mods/);
//   3. where to start: the game's own list, a list from RimSort / RimCrow (previewed, then
//      applied: view.ReadImport → PreviewImport → ApplyImport), or a pack / a collection.
// The environment page's "add a game" starts at step 1.
// Settings' "preview" shows the first run as a new player sees it, managed-folder step
// included, and writes nothing: adding, downloading, scanning and the starting list are
// played out on the page only (16 §2.15).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { page, button, seg, chooseLocalPath, beginPageRead } from "./screens.js";
import { exchange, importFrom } from "./exchange.js";
import { withEnvironmentSwitch } from "./environment.js";
import { on } from "./api.js";
import { archoFigure, archoMode, bootDone } from "./archo.js";
import { managersWay } from "./importways.js";

let ui = null;
export function initGuide(u) {
  ui = u;
  on("tasks", () => updateSteamCMD());
  on("copy", (ev) => copyEvent(ev));
}

let session = 0, activeSession = null;
export const historySession = () => activeSession;
export const previewing = () => S.guideFrom === "preview";

const STEPS = ["find the game", "read the mods", "where to start"];
// G.folder: the managed folder (FirstRunFolder) while the first run still has to choose it;
// it is then step -1, before "find the game" (16 §2.15).
const G = { folder: null, folderBusy: false, step: 0, way: 0, games: [], picked: new Set(), added: [], scan: null, typed: "", error: "", errorFrom: null };
// The download way: SteamCMD's info (DownloadInfo), the choices, and a run's progress.
const D = { info: null, loading: false, branch: "", dir: "", dirTyped: false, account: "", run: null };
const C = { id: "", row: null, submitting: false };
// The copy way (03 §3.5): which game, where, its name, what comes along, the plan the
// backend checked (GuideCopyPlan) and a run's progress.
const K = { from: "", dir: "", dirTyped: false, name: "", mods: false, own: false, plan: null, planning: false, run: null, seq: 0 };
const addedGames = () => (S.board && S.board.envs) || [];

// The guide follows the same durable task as the background page (16 §2.15, 16 §2.16).
async function updateSteamCMD() {
  if (!C.id) return;
  const id = C.id;
  let tasks;
  try { tasks = await api.Tasks(); } catch (e) { G.error = errText(e); draw(); return; }
  if (id !== C.id) return;
  const row = tasks?.rows?.find(t => t.id === id);
  if (!row) return;
  C.row = row;
  if (row.state === "queue" || row.state === "run") return draw();
  C.id = "";
  if (row.state !== "done") { G.error = row.line || row.stateText; draw(); return; }
  G.error = "";
  D.info = null;
  loadDownload();
}

// start opens the guide at step 1; from is "first" (no environment yet), "env" (adding a
// game) or "preview" (the first run, shown without writing anything).
export async function start(from = "first") {
  activeSession = ++session;
  S.guideFrom = from;
  G.step = 0;
  G.added = [];
  G.scan = null;
  G.error = "";
  G.picked.clear();
  if (D.run && D.run.end) D.run = null;
  if (K.run && K.run.end) { K.run = null; K.plan = null; }
  G.way = D.run ? 1 : K.run && !K.run.end ? 2 : 0;
  // the Import page's and the Game page's "copy a game" open this way (16 §2.18)
  if (S.guideWay != null) { G.way = S.guideWay; S.guideWay = null; }
  G.folder = null;
  P.timer = null;
  await find(from !== "env");
  bootDone();
}

// find reads the games detection finds and, on a first run, whether the managed folder is
// still to be chosen: then the guide starts there.
async function find(first = false) {
  const read = beginPageRead("guide", { replace: true });
  let games, folder = null;
  try {
    [games, folder] = await Promise.all([api.GuideFind(), first ? api.ManagedFolder() : null]);
    games ||= [];
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  // a preview shows the folder step even when the folder was chosen long ago
  if (folder && (!folder.done || previewing())) { G.folder = { ...folder, done: false }; G.step = -1; }
  // a preview shows the games already added as a new player would see them: not added yet
  G.games = previewing() ? games.map((g) => ({ ...g, env: "" })) : games;
  G.games.forEach((g, i) => { if (!g.env) G.picked.add(i); });
  draw();
  read.finish();
}

export function resume() {
  const read = beginPageRead("guide"); draw(); read.finish();
  if (G.step === 0 && G.way === 1 && !D.run && !D.loading) loadDownload();
  if (G.step === 0 && G.way === 2 && !K.run && !K.plan) loadCopyPlan();
}

function draw() {
  if (S.screen !== "guide") return;
  const sc = $("#screen");
  sc.innerHTML = "";
  const lead = S.guideFrom === "env" ? T("find the install, read its mods once, and it is ready")
    : G.folder ? T("choose RimArcho's folder, find the game, read the mods once, then pick where your first list comes from")
    : T("find the game, read the mods once, then pick where your first list comes from");
  const w = page("guide");
  sc.append(w);
  // the steps: where you are, where you can go back to; the managed folder is step -1
  const bar = el("div", "steps");
  const steps = G.folder ? ["managed folders", ...STEPS] : STEPS, first = G.folder ? -1 : 0;
  steps.forEach((s, k) => {
    const i = first + k;
    if (k) bar.append(el("span", "line" + (i <= G.step ? " on" : "")));
    const b = el("button", "st" + (i === G.step ? " cur" : i < G.step ? " done" : ""));
    b.append(el("span", "n", i < G.step ? "✓" : String(k + 1)), el("span", "t", T(s)));
    b.disabled = i >= G.step || (i === 1 && !G.added.length);
    b.onclick = () => go(i);
    bar.append(b);
  });
  // the lead and the steps on the left, Archo on the right: one row, as tall as she is
  const top = el("div", "guide-top"), left = el("div", "guide-lead"), head = el("div", "page-head");
  head.append(el("p", "lead word-facts", lead));
  if (previewing()) head.append(el("p", "guide-preview-note muted", T("a preview of the first run: nothing you choose here is saved")));
  left.append(head, bar);
  top.append(left);
  // Archo on the steps' row, behind the card (16 §2.15): one slot kept across redraws, so
  // the live figure keeps moving while progress redraws the card
  if (archoMode() !== "off") {
    if (!figure || figure.ended || figure.session !== activeSession) {
      figure?.end();
      figure = archoFigure(el("div", "guide-archo"), guideExpression());
      figure.session = activeSession;
    } else figure.set(guideExpression());
    top.append(figure.slot);
  }
  const card = el("section", "card guide-card");
  w.append(top, card);
  [folderStep, step1, step2, step3][G.step + 1](card);
  ui.renderWhere();
}

// Archo's face follows the guide: a wink to start, thinking while something runs, a salute
// when a scan or a download is done, surprise at an error.
let figure = null;
function guideExpression() {
  if (G.error) return "surprised";
  if (G.step === -1) return "wink";
  if (G.step === 0) {
    if (D.run) return !D.run.end ? "think" : D.run.ok ? "salute" : "surprised";
    return G.way === 1 ? "ponder" : G.folder ? "smile" : "wink";
  }
  if (G.step === 1) return !G.scan || !G.scan.done ? "think" : G.scan.ok ? "salute" : "surprised";
  return "smile";
}

function go(i) {
  G.step = i;
  G.error = "";
  draw();
}

function footer(card, ...btns) {
  const f = el("div", "guide-foot");
  if (G.error) f.append(el("span", "err grow", G.error)); else f.append(el("span", "grow"));
  if (previewing()) f.append(button(T("end the preview"), "", () => finish("")));
  if (G.step > (G.folder ? -1 : 0)) f.append(button(T("back"), "", () => go(G.step - 1)));
  for (const b of btns) if (b) f.append(b);
  card.append(f);
}

// ---- 0. the managed folder (first run only) ----
// Where SteamCMD, downloaded Mods and tools go. The default is filled in; the player keeps
// it or picks another folder, and confirming it is the way on (17 §3).

function folderStep(card) {
  const info = G.folder;
  card.append(el("div", "guide-q", T("Where should RimArcho keep SteamCMD, downloaded Mods and tools?")));
  const choose = () => chooseLocalPath(info.path, async (path) => {
    G.folderBusy = true; draw();
    try { G.folder = { ...(await api.FolderSpace(path)), path }; G.error = ""; }
    catch (e) { G.error = errText(e); }
    finally { G.folderBusy = false; draw(); }
  });
  const row = el("div", "typed folder-row");
  row.append(editablePath(copyablePath(el("span", "", info.path), info.path), choose), button(T("change…"), "", choose));
  card.append(row);
  const space = el("div", "folder-space muted");
  space.append(el("span", "", Tf("free: %s", freeText(info.free))));
  if (info.low) space.append(el("span", "warn", T("Less than 10 GB is free on this drive.")));
  card.append(space);
  if (info.error) card.append(el("div", "dl-note err", info.error));
  const use = button(T("use this folder"), "", async () => {
    G.folderBusy = true; draw();
    try {
      if (!previewing()) await api.ChooseManagedFolder(info.path);
      G.folder = { ...info, done: true };
      G.error = "";
      G.step = 0;
    } catch (e) { G.error = errText(e); }
    finally { G.folderBusy = false; draw(); }
  }, "primary");
  use.disabled = G.folderBusy || !!info.error;
  footer(card, use);
}

const freeText = (n) => n < 0 ? T("free space unknown") : sizeText(n);

// ---- 1. find the game ----

function step1(card) {
  card.append(el("div", "guide-q", T("which RimWorld installs should RimArcho manage?")));
  // copying needs a game already added (a preview has none of its own to copy, it shows the way)
  const canCopy = addedGames().length > 0;
  if (G.way === 2 && !canCopy) G.way = 0;
  const ways = [T("a game you have"), T("download a version")];
  if (canCopy) ways.push(T("copy a game you added"));
  const sw = seg(ways, G.way, (i) => {
    G.way = i;
    G.error = "";
    draw();
    if (i === 1 && !D.run && !D.loading) loadDownload();
    if (i === 2 && !K.run) loadCopyPlan();
  });
  sw.classList.add("guide-way");
  card.append(sw);
  if (G.way === 1) return download(card);
  if (G.way === 2) return copyGame(card);
  const list = el("div", "games");
  if (!G.games.length) list.append(el("div", "empty-state", T("no RimWorld found in the Steam libraries or the usual folders; add its folder below")));
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
    if (g.env) top.append(el("span", "chip cur", Tf("added as %s", g.env)));
    body.append(top, copyablePath(el("div", "path", g.game), g.game));
    if (!g.env) {
      const name = el("input", "field small");
      name.value = g.name;
      name.spellcheck = false;
      name.title = T("the game's name");
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
  const typed = el("div", "typed");
  const addPath = async (path) => {
    let g;
    // the names already on the page count as taken: two games of one version get 1.6-2, 1.6-3
    const pending = G.games.filter((x) => !x.env && x.name).map((x) => x.name);
    try { g = await api.GuideGame(path, pending); } catch (e) { G.typed = path; G.error = errText(e); return draw(); }
    if (previewing()) g = { ...g, env: "" };
    G.typed = "";
    G.error = "";
    if (!G.games.some((x) => x.game.toLowerCase() === g.game.toLowerCase())) {
      G.games.push(g);
      if (!g.env) G.picked.add(G.games.length - 1);
    }
    draw();
  };
  typed.append(button(T("add this folder"), "", () => chooseLocalPath(G.typed, addPath)));
  card.append(typed);
  const picked = [...G.picked].filter((i) => G.games[i] && !G.games[i].env);
  const next = button(picked.length > 1 ? Tf("add %d games", picked.length) : T("add and continue"), "", async () => {
    next.disabled = true;
    let names;
    try {
      const games = picked.map((i) => G.games[i]);
      if (previewing()) return await added(games.map((g) => g.name));
      const accepted = await withEnvironmentSwitch(games[0].name, async () => {
        names = await api.GuideAdd(games);
      });
      if (accepted) await added(names || []);
    } catch (e) { G.error = errText(e); draw(); }
  }, "primary");
  next.disabled = !picked.length;
  footer(card, G.added.length ? button(T("next"), "", () => go(1)) : null, next);
}

// added: environments were added (found, typed or downloaded); read their mods.
async function added(names) {
  G.added = names;
  if (!previewing()) G.games = (await api.GuideFind()) || G.games;
  G.picked.clear();
  await scanAll();
}

// ---- 1b. download a version (SteamCMD, 11 §3) ----

async function loadDownload() {
  D.loading = true;
  draw();
  try { D.info = await api.GuideDownloadInfo(); } catch (e) { D.info = { error: errText(e) }; }
  D.loading = false;
  applyDownloadInfo();
  if (D.info?.refreshing) refreshDownload(D.info);
}

function applyDownloadInfo() {
  const bs = (D.info && D.info.branches) || [];
  if (bs.length && !bs.some((b) => b.name === D.branch)) D.branch = bs[0].name;
  if (!D.dirTyped) D.dir = suggestDir();
  draw();
}

async function refreshDownload(info) {
  const deadline = Date.now() + 120000;
  let delay = 250;
  while (D.info === info && info.refreshing && S.screen === "guide" && G.step === 0 && G.way === 1 && !D.run && Date.now() < deadline) {
    await new Promise(resolve => setTimeout(resolve, Math.min(delay, deadline - Date.now())));
    delay = Math.min(delay * 2, 5000);
    if (D.info !== info || S.screen !== "guide" || G.step !== 0 || G.way !== 1 || D.run || Date.now() >= deadline) return;
    let next;
    try { next = await api.GuideDownloadInfo(); } catch { return; }
    if (D.info !== info || S.screen !== "guide" || G.step !== 0 || G.way !== 1 || D.run || Date.now() >= deadline) return;
    D.info = info = next;
    if (!info.refreshing) applyDownloadInfo();
  }
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
  if (C.id || C.submitting) {
    const row = C.row;
    card.append(el("div", "dl-state", row ? row.say : T("downloading…")));
    const prog = el("div", "prog");
    const bar = el("div", "bar" + (row?.pct > 0 ? "" : " busy"));
    const fill = el("i");
    fill.style.width = (row?.pct || 0) + "%";
    bar.append(fill); progress(bar, card.querySelector(".dl-state"), row?.count || row?.pct > 0 ? row.pct : undefined); prog.append(bar); card.append(prog);
    return footer(card);
  }
  if (D.run) return downloading(card, D.run);
  if (!D.info) {
    card.append(el("div", "dl-note", T("asking Steam which versions RimWorld has… (SteamCMD may update itself first)")));
    const prog = el("div", "prog");
    const b = el("div", "bar busy");
    progress(b, card.querySelector(".dl-note"));
    b.append(el("i"));
    prog.append(b);
    card.append(prog);
    return footer(card);
  }
  const info = D.info;
  if (!info.steamcmd) {
    const o = info.offer || {};
    const size = o.bytes > 0 ? sizeText(o.bytes) : T("size unknown");
    card.append(pathContent(el("div", "dl-note"), Tf("RimArcho downloads game versions with Valve's SteamCMD, which is not on this computer. Download it from Valve (%s) into %s?", size, o.dir || ""), [o.dir]));
    const get = button(T("download SteamCMD"), "", async () => {
      if (previewing()) { D.info = { ...info, steamcmd: "preview", games: info.games || "C:\\Games", branches: PREVIEW_BRANCHES }; return applyDownloadInfo(); }
      get.disabled = true;
      C.submitting = true; C.row = null;
      draw();
      try {
        C.id = await api.GuideInstallSteamCMD();
        say("ok", Tf("%s, %s: in the %s queue", T("download SteamCMD"), "RimArcho", "RimArcho"));
      } catch (e) { G.error = errText(e); }
      finally { C.submitting = false; }
      if (!C.id) return draw();
      G.error = "";
      updateSteamCMD();
    }, "primary");
    return footer(card, get);
  }
  if (info.error && !(info.branches || []).length) {
    card.append(el("div", "dl-note err", info.error));
    return footer(card, button(T("ask again"), "", () => { D.info = null; loadDownload(); }));
  }
  if (info.refreshing) card.append(el("div", "dl-note muted", T("refreshing branch list…")));
  if (info.error) card.append(el("div", "dl-note err", info.error));
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
  const choose = () => chooseLocalPath(D.dir, path => { D.dir = path; D.dirTyped = true; draw(); });
  where.append(el("span", "muted lbl", T("game folder")), editablePath(copyablePath(el("span", "", D.dir), D.dir), choose),
    button(T("change…"), "", choose));
  card.append(where);
  const who = el("div", "typed");
  const acc = el("input", "field");
  acc.placeholder = Tf("empty: %s (from settings)", info.account || "anonymous");
  acc.spellcheck = false;
  acc.autocomplete = "username";
  acc.value = D.account;
  acc.oninput = () => { D.account = acc.value.trim(); };
  who.append(el("span", "muted lbl", T("SteamCMD account (optional)")), acc);
  card.append(who);
  card.append(el("div", "dl-note", T("Anonymous by default; you can choose an account in settings or here. Enter anonymous to override a saved account. Password and Steam Guard are entered only in SteamCMD.")));
  const start1 = button(T("download and add"), "", async () => {
    if (previewing()) return previewDownload();
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
  const t = r.end ? (r.ok ? Tf("downloaded; game %s added", r.env) : T("the download did not finish")) : T(DL_STATES[r.state] || r.state);
  const head = el("div", "dl-state");
  head.append(el("span", "t", t));
  if (!r.end && r.total > 0) head.append(el("span", "muted", Tf("%s of %s", sizeText(r.done), sizeText(r.total))));
  card.append(head);
  const prog = el("div", "prog");
  const bar = el("div", "bar" + (r.end || r.total > 0 ? "" : " busy"));
  const fill = el("i");
  fill.style.width = (r.end && r.ok ? 100 : Math.round(r.percent || 0)) + "%";
  bar.append(fill);
  progress(bar, head, r.end && r.ok ? 100 : r.total > 0 ? r.percent : undefined);
  prog.append(bar);
  card.append(prog);
  if (r.error) card.append(el("div", "dl-note err", r.error));
  else if (r.line) card.append(el("div", "dl-line", r.line));
  if (r.end && !r.ok) return footer(card, button(T("try again"), "", () => { D.run = null; draw(); }));
  footer(card);
}

// downloadEvent is a "download" event: SteamCMD's progress, then the end.
export function downloadEvent(ev) {
  if (!D.run || D.run.end || D.run.preview) return;
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
  if (previewing()) return previewScan();
  // A scan that did not start gives the flag back as it was (another scan may still run).
  const was = S.busy.scan;
  S.busy.scan = true;
  let started = false;
  try {
    started = await api.GuideScan(G.added);
    if (!started) G.error = T("a scan is already running; wait for it to finish");
  } catch (e) {
    G.error = errText(e);
    G.scan.ok = false;
  } finally {
    if (!started) {
      S.busy.scan = was;
      G.scan.done = true;
      draw();
    }
  }
}

// guideEvent is a "guide" event: an environment started, ended, or all are done.
export function guideEvent(ev) {
  if (!G.scan || G.scan.done || previewing()) return;
  if (ev.done) {
    G.scan.done = true;
    S.busy.scan = false;
    G.scan.ok = ev.ok;
    ui.reload();
    return draw();
  }
  const r = G.scan.rows[ev.i];
  if (!r) return;
  if (ev.step) { r.state = ev.ok ? "ok" : "bad"; r.text = ev.ok ? ev.text : ev.error; r.frac = 1; } else { r.state = "cur"; r.text = ev.text; r.frac = ev.frac || 0; }
  draw();
}

function step2(card) {
  card.append(el("div", "guide-q", T("RimArcho reads every mod folder once; later scans only read what changed")));
  const s = G.scan || { rows: [], done: true };
  const list = el("div", "scanlist");
  for (const r of s.rows) {
    const d = el("div", "scanrow " + r.state);
    d.append(el("span", "n", r.state === "ok" ? "✓" : r.state === "bad" ? "✗" : r.state === "cur" ? "…" : "—"), el("span", "env", r.env), el("span", "t", r.text));
    list.append(d);
  }
  card.append(list);
  // each environment counts as one; the one being read counts how far it is
  const doneN = s.rows.reduce((n, r) => n + (r.state === "ok" || r.state === "bad" ? 1 : r.state === "cur" ? r.frac || 0 : 0), 0);
  const prog = el("div", "prog");
  const bar = el("div", "bar" + (s.done ? "" : " busy"));
  const fill = el("i");
  fill.style.width = (s.rows.length ? Math.round(100 * doneN / s.rows.length) : 100) + "%";
  bar.append(fill);
  progress(bar, card.querySelector(".guide-q"), s.rows.length ? Math.round(100 * doneN / s.rows.length) : undefined, 100);
  prog.append(bar);
  card.append(prog);
  const again = s.done && !s.ok ? button(T("scan again"), "", () => scanAll()) : null;
  const next = button(T("next"), "", () => go(2), "primary");
  next.disabled = !s.done;
  footer(card, again, next);
}

// ---- 3. where to start ----

function step3(card) {
  card.append(el("div", "guide-q", T("where does your first list come from?")));
  if (previewing()) return previewStep3(card);
  const ways = el("div", "ways");
  const n = S.board && S.board.cols ? (S.board.cols[1] || []).length : 0;
  ways.append(way(T("the game's current list"), Tf("ModsConfig.xml enables %d mods; that becomes your first list", n),
    button(T("use this list"), "", () => finish(T("use the game's list as it is")), "primary")));
  // the same box as the Import page's (16 §2.18): it says what discovery found
  ways.append(managersWay(() => { ui.afterImport = () => finish(""); }));
  const src = el("input", "field");
  src.placeholder = T("a Steam Workshop collection link");
  src.type = "url"; src.spellcheck = false;
  const open = button(T("open"), "", async () => {
    const v = src.value.trim();
    if (!v) return src.focus();
    if (!/^https?:\/\//i.test(v)) { G.error = T("enter a complete HTTP or HTTPS URL"); G.errorFrom = ["url", G.error]; return draw(); }
    ui.afterImport = () => finish(""); return importFrom("text", v);
  });
  src.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); open.click(); } };
  const row = el("div", "typed");
  const pick = button(T("choose a pack…"), "", async () => {
    const session = activeSession;
    try {
      const path = await api.PickPackFile();
      if (path && S.screen === "guide" && activeSession === session) await finish(await api.PackImport(path));
    } catch (e) { if (S.screen === "guide" && activeSession === session) { G.error = errText(e); G.errorFrom = ["pack", G.error]; draw(); } }
  });
  // the link field above, the pack button on the buttons' line with the other two ways
  row.append(src, open);
  ways.append(way(T("open a pack or a Steam collection"), T("choose a pack (.archopack), or paste a Steam Workshop collection link"), pick, row));
  card.append(ways);
  footer(card, button(T("skip"), "", () => finish("")));
  const error = card.querySelector(".guide-foot .err");
  // the footer's error belongs to the field that caused it, while it is still that error
  const from = (k) => () => G.errorFrom?.[0] === k && G.errorFrom[1] === G.error;
  if (error) { fieldError(src, error, from("url")); fieldError(pick, error, from("pack")); }
}

// way is one choice: its title, what it does, then its button at the bottom, level with the
// other choices' buttons (extra goes between the text and the button)
function way(title, text, action, extra) {
  const d = el("div", "way");
  d.append(el("div", "t", title), el("div", "muted", text));
  if (extra) d.append(extra);
  d.append(action);
  return d;
}

async function finish(text) {
  activeSession = null;
  ui.afterImport = null;
  G.scan = null;
  if (previewing()) {
    P.timer = null;
    if (D.run?.preview) D.run = null;
    if (K.run?.preview) K.run = null;
    say("ok", T("preview ended; nothing was saved"));
    return ui.go("settings");
  }
  if (text) say("ok", text);
  await ui.reload();
  ui.go("home");
}

// ---- the preview (16 §2.15) ----
// The same cards with made-up progress: a download and a scan play out on timers, and the
// last step's ways end the preview. A timer belongs to one preview session; leaving the page
// or ending the preview stops it.

const P = { timer: null };
const PREVIEW_BRANCHES = [
  { name: "public", version: "" },
  { name: "unstable", version: "" },
  { name: "version-1.5", version: "1.5" },
];

function previewTick(steps, ms = 180) {
  const own = activeSession, timer = {};
  P.timer = timer;
  let i = 0;
  const next = () => {
    if (P.timer !== timer || activeSession !== own || !previewing()) return;
    steps[i++]();
    if (S.screen === "guide") draw();
    if (i < steps.length) setTimeout(next, ms);
  };
  setTimeout(next, ms);
}

function previewDownload() {
  G.error = "";
  const total = 2.6e9, env = "1.6";
  D.run = { state: "starting", percent: 0, line: "", end: false, preview: true };
  const steps = [() => { D.run.state = "login"; }];
  for (let k = 1; k <= 10; k++) steps.push(() => Object.assign(D.run, { state: "downloading", percent: k * 10, done: total * k / 10, total }));
  steps.push(() => Object.assign(D.run, { state: "verifying", total: 0 }));
  steps.push(() => { Object.assign(D.run, { end: true, ok: true, env }); added([env]); });
  draw();
  previewTick(steps, 250);
}

function previewScan() {
  const rows = G.scan.rows, total = 240, steps = [];
  rows.forEach((r) => {
    for (let k = 1; k <= 9; k++) steps.push(() => Object.assign(r, { state: "cur", frac: k / 10, text: Tf("reading the mods of %s: %d of %d", r.env, Math.round(total * k / 9), total) }));
    steps.push(() => Object.assign(r, { frac: 0.95, text: Tf("checking the files of %s…", r.env) }));
    steps.push(() => Object.assign(r, { state: "ok", frac: 1, text: Tf("%d mods", total) }));
  });
  steps.push(() => { G.scan.done = true; });
  previewTick(steps);
}

function previewStep3(card) {
  const ways = el("div", "ways");
  const end = (label) => button(label, "", () => finish(""));
  const n = S.board && S.board.cols ? (S.board.cols[1] || []).length : 0;
  ways.append(way(T("the game's current list"), Tf("ModsConfig.xml enables %d mods; that becomes your first list", n), end(T("use this list"))));
  ways.append(way(T("move over from RimSort or RimCrow"), T("lists and player data, previewed before anything is imported"), end(T("import…"))));
  ways.append(way(T("open a pack or a Steam collection"), T("choose a pack (.archopack), or paste a Steam Workshop collection link"), end(T("choose a pack…"))));
  card.append(ways);
  footer(card, end(T("skip")));
}

// ---- 1c. copy a game you added (03 §3.5) ----
// A backup before the game updates, or a second game to try something in: the game's
// folder, never RimArcho's links in Mods/, copied as a new game started directly.

async function loadCopyPlan() {
  const games = addedGames();
  if (!games.includes(K.from)) K.from = games[0] || "";
  if (!K.from) return;
  const seq = ++K.seq;
  K.planning = true;
  draw();
  let plan = null, error = "";
  try { plan = await api.GuideCopyPlan(K.from, K.dirTyped ? K.dir : "", K.name, K.mods); } catch (e) { error = errText(e); }
  if (seq !== K.seq) return;
  K.planning = false;
  K.plan = plan;
  if (plan && !K.dirTyped) K.dir = plan.dir;
  G.error = error;
  draw();
}

function copyGame(card) {
  if (K.run) return copying(card, K.run);
  if (!K.from) { loadCopyPlan(); return footer(card); }
  const p = K.plan;
  const from = el("div", "typed");
  const pick = el("select", "field");
  pick.setAttribute("aria-label", T("copy which game"));
  for (const g of addedGames()) {
    const o = el("option", "", g);
    o.value = g;
    o.selected = g === K.from;
    pick.append(o);
  }
  pick.onchange = () => { K.from = pick.value; K.name = ""; if (!K.dirTyped) K.dir = ""; loadCopyPlan(); };
  from.append(el("span", "muted lbl", T("copy which game")), pick);
  card.append(from);
  const where = el("div", "typed");
  const choose = () => chooseLocalPath(K.dir, (path) => { K.dir = path; K.dirTyped = true; loadCopyPlan(); });
  where.append(el("span", "muted lbl", T("copy to")), editablePath(copyablePath(el("span", "", K.dir), K.dir), choose), button(T("change…"), "", choose));
  card.append(where);
  const named = el("div", "typed");
  const name = el("input", "field small");
  name.value = K.name;
  name.placeholder = p?.name || "";
  name.spellcheck = false;
  name.onchange = () => { K.name = name.value.trim(); loadCopyPlan(); };
  named.append(el("span", "muted lbl", T("the copy's name")), name);
  card.append(named);
  const box = (text, on, set) => {
    const l = el("label", "copy-opt");
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = on;
    cb.onchange = () => set(cb.checked);
    l.append(cb, el("span", "", text));
    return l;
  };
  card.append(box(Tf("the game's Mods folder too (%s)", p ? sizeText(p.modBytes) : "…"), K.mods, (v) => { K.mods = v; loadCopyPlan(); }),
    box(T("keep its saves and settings apart"), K.own, (v) => { K.own = v; draw(); }));
  const space = el("div", "folder-space muted");
  if (p) {
    space.append(el("span", "", Tf("copies %s; free: %s", sizeText(p.bytes), freeText(p.free))));
    if (p.low) space.append(el("span", "warn", T("Not enough room on this drive for the copy.")));
  } else if (K.planning) space.append(el("span", "", T("checking…")));
  card.append(space);
  card.append(el("div", "dl-note", T("The copy is started with the game's own exe, so Steam never updates it. Launching it makes RimArcho's Mod links again.")));
  const go = button(T("copy and add"), "", async () => {
    go.disabled = true;
    if (previewing()) return previewCopy();
    let ok;
    try { ok = await api.GuideCopy(K.from, K.dir, K.name, K.mods, K.own); } catch (e) { G.error = errText(e); return draw(); }
    if (!ok) { G.error = T("a copy is already running"); return draw(); }
    G.error = "";
    K.run = { bytes: 0, total: p.bytes, end: false };
    draw();
  }, "primary");
  go.disabled = !p || p.low || K.planning;
  footer(card, go);
}

function copying(card, r) {
  const t = r.end ? (r.ok ? Tf("copied; game %s added", r.env) : T("the copy did not finish")) : T("copying the game…");
  const head = el("div", "dl-state");
  head.append(el("span", "t", t));
  if (!r.end && r.total > 0) head.append(el("span", "muted", Tf("%s of %s", sizeText(r.bytes), sizeText(r.total))));
  card.append(head);
  const pct = r.end && r.ok ? 100 : r.total > 0 ? Math.round(100 * r.bytes / r.total) : 0;
  const prog = el("div", "prog");
  const bar = el("div", "bar");
  const fill = el("i");
  fill.style.width = pct + "%";
  bar.append(fill);
  progress(bar, head, pct);
  prog.append(bar);
  card.append(prog);
  if (r.error) card.append(el("div", "dl-note err", r.error));
  if (r.end && !r.ok) return footer(card, button(T("try again"), "", () => { K.run = null; K.plan = null; loadCopyPlan(); }));
  footer(card);
}

// copyEvent is a "copy" event: the bytes copied, then the end.
function copyEvent(ev) {
  if (!K.run || K.run.end || K.run.preview) return;
  if (ev.end) {
    Object.assign(K.run, { end: true, ok: ev.ok, error: ev.error || "", env: ev.env || "" });
    draw();
    if (ev.ok) { K.plan = null; added([ev.env]); }
    return;
  }
  Object.assign(K.run, ev);
  draw();
}

function previewCopy() {
  G.error = "";
  const total = K.plan?.bytes || 1.3e9, env = K.name || K.plan?.name || K.from + "-copy";
  K.run = { bytes: 0, total, end: false, preview: true };
  const steps = [];
  for (let k = 1; k <= 10; k++) steps.push(() => { K.run.bytes = total * k / 10; });
  steps.push(() => { Object.assign(K.run, { end: true, ok: true, env }); added([env]); });
  draw();
  previewTick(steps, 200);
}
