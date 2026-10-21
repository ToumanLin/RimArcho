// The first-run guide (06 §2.15): three steps, each with a way back.
//   1. find the game: what detection finds (tick several), or a folder typed in;
//   2. read the mods: a scan per environment in a child process, with its progress;
//   3. where to start: the game's own list, a list from RimSort / RimCrow (previewed, then
//      applied: view.ReadImport → PreviewImport → ApplyImport), or a pack / a collection.
// The environment page's "add an environment" starts at step 1.
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { openScreen, page, button } from "./screens.js";
import { exchange, importFrom } from "./exchange.js";

let ui = null;
export function initGuide(u) { ui = u; }

const STEPS = ["find the game", "read the mods", "where to start"];
const G = { step: 0, games: [], picked: new Set(), added: [], scan: null, typed: "", error: "" };

// start opens the guide at step 1; from is "first" (no environment yet) or "env".
export async function start(from = "first") {
  S.guideFrom = from;
  G.step = 0;
  G.added = [];
  G.scan = null;
  G.error = "";
  G.picked.clear();
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
  // a folder typed in when detection finds nothing (or not this one)
  const typed = el("div", "typed");
  const inp = el("input", "field");
  inp.placeholder = T("the game folder (the one with RimWorldWin64.exe)");
  inp.spellcheck = false;
  inp.value = G.typed;
  inp.oninput = () => { G.typed = inp.value; };
  const add = button(T("add this folder"), "", async () => {
    let g;
    try { g = await api.GuideGame(inp.value); } catch (e) { G.error = errText(e); return draw(); }
    G.typed = "";
    G.error = "";
    if (!G.games.some((x) => x.game.toLowerCase() === g.game.toLowerCase())) {
      G.games.push(g);
      if (!g.env) G.picked.add(G.games.length - 1);
    }
    draw();
  });
  inp.onkeydown = (e) => { if (e.key === "Enter") { e.stopPropagation(); add.click(); } };
  typed.append(inp, add);
  card.append(typed);
  const picked = [...G.picked].filter((i) => G.games[i] && !G.games[i].env);
  const next = button(picked.length > 1 ? Tf("add %d environments", picked.length) : T("add and continue"), "", async () => {
    next.disabled = true;
    let names;
    try { names = await api.GuideAdd(picked.map((i) => G.games[i])); } catch (e) { G.error = errText(e); return draw(); }
    G.added = names || [];
    G.games = (await api.GuideFind()) || G.games;
    G.picked.clear();
    await ui.reload();
    scanAll();
  }, "primary");
  next.disabled = !picked.length;
  footer(card, G.added.length ? button(T("next"), "", () => go(1)) : null, next);
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
