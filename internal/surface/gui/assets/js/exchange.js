// The Import page (16 §2.18): every way into RimArcho in one place. Lists are compared with
// the player's list before anything changes — side by side, in order, what is added, removed
// and moved — and applied as one undo step; nothing here writes ModsConfig.xml (that stays save).
import { factText, S, $, el, api, T, Tf, errText, say } from "./state.js";
import { page, button, beginPageRead, area } from "./screens.js";
import { icon } from "./icons.js";
import * as pop from "./popups.js";
import { getChoices } from "./credit.js";
import { still } from "./motion.js";
import { splitBar } from "./membar.js";
import { archoSpot } from "./archo.js";
import { wireSeparator } from "./separator.js";
import { orderDiff, orderView, legend } from "./listdiff.js";

let ui = null;
export function initExchange(u) {
  ui = u;
  wireDrop();
}

let info = null;
let firstRun = false;
let session = null;
let previewSeq = 0;

// Import is a sub-page, including when entered from the first-run guide (16 §2.17).
export async function importFrom(k, src) {
  await exchange();
  if (S.screen === "plan-import") return preview(k, src);
}

export function cancel() {
  ++previewSeq;
  const from = session?.from || "plans";
  // on the first run, leaving the page ends the guide: the scan made the game's list the first one
  if (firstRun && from === "guide" && ui.afterImport) { const after = ui.afterImport; ui.afterImport = null; firstRun = false; return after(""); }
  ui.afterImport = null;
  return from === "guide" ? ui.resumeGuide() : ui.go(from);
}
export function key(e) {
  if (e.key === "Escape") { e.preventDefault(); return cancel(); }
}
// enter only means something while a list comparison is shown
export const hintList = () => [...($("#screen .xc-preview")?.dataset.mode === "list" ? [{ key: "enter", label: T("import") }] : []), { key: "esc", label: T("cancel") }, { key: "?", label: T("all keys") }];

// layout: the page's arrangement while the user compares the variants (design note
// 2026-11-20_import-page): "2col" (what to import | the comparison), "3col" (what to import |
// the comparison | a rail with Archo, the counts and the buttons) or "2col-new" (the rail
// laid across the top of the comparison). <html data-import-layout> or ?import-layout= picks it.
export function layout() {
  const v = document.documentElement?.dataset?.importLayout || new URLSearchParams(globalThis.location?.search || "").get("import-layout") || "";
  return v === "3col" || v === "2col-new" ? v : "2col";
}

const gameConfig = () => (S.board?.environmentRows || []).find(e => e.current)?.name || "";

// The Import page (16 §2.18): the sidebar's Import, `i`, a dropped file, Settings and the guide
// all open it. The left column says what can be imported, one row each with its button; the
// right shows what a read brings, compared with what the player has. Empty, the right is where
// a dropped file goes, with Archo in it. Moving in and adding a pack run as background tasks
// shown on the right (16 §2.16).
// first: the first-run guide hands over here after its scan (A1, 16 §2.15): the game's
// current list is the one filled button, and "skip" ends the guide.
export async function exchange(first = false) {
  const from = S.screen === "plan-import" ? session?.from : S.screen || "plan";
  if (S.screen !== "plan-import") firstRun = first && from === "guide";
  const target = from === "plans" ? ui.selectedList() : S.shell?.plan?.id || 0;
  const read = beginPageRead("plan-import", { replace: true });
  session = { from, target, name: S.shell?.plans?.find(p => p.id === target)?.name || "", epoch: S.screenSeq };
  ++previewSeq;
  try { info = await api.ExchangeInfo(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  const sc = $("#screen"); sc.innerHTML = "";
  const lay = layout();
  // the lead says the page's one gesture: drop it anywhere; the first run asks instead
  const w = page("plans pm-import xc-page xc-" + lay, firstRun ? T("Where does your first list come from?") : T("Drop files or folders anywhere on this window; RimArcho tells what they are."),
    firstRun ? button(T("skip"), "esc", cancel) : null);
  const grid = el("div", "xc-lay"); grid.dataset.layout = lay;
  const side = el("div", "xc-side");
  const card = el("section", "card settings xc-do");
  side.append(card);
  // the right: the comparison (or the tree, a task, a refusal) and, before any, the drop pane
  const main = el("div", "xc-main");
  const result = el("section", "card settings xc-preview xc-res"); result.hidden = true;
  // 2col-new: over the result, the counts and the buttons at the left and Archo at the right
  if (lay === "2col-new") main.append(dock("xc-bar"));
  // no label above the right card: it is as tall as the left one and says what it is itself
  main.append(result, emptyState());
  // the gap between the two cards is the card gap, and drags (16 §7.3 "Card spacing")
  const gap = el("div", "split xc-split");
  grid.append(side, gap, main);
  const setSide = (v) => { if (v) grid.style.setProperty("--xc-side", v + "px"); else grid.style.removeProperty("--xc-side"); };
  wireSeparator(gap, {
    key: "ui.gui.import.side", range: () => [300, Math.max(300, grid.clientWidth - 480)],
    get: () => side.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(300, +v) : 0),
  });
  // 3col: a rail at the right for Archo, the counts and the buttons; always there, so the
  // columns keep their widths when a result comes
  if (lay === "3col") grid.append(dock("xc-rail"));
  w.append(grid);
  sc.append(w);
  area(card, null); area(result, null);
  M = freshM();
  drawDo();
  placeArcho();
  read.finish(); ui.renderWhere(); ui.renderHints();
  if (!firstRun) survey();
}

const live = (active) => session === active && S.screen === "plan-import";

// dock: the rail (3col) or the bar (2col-new): Archo, the result's head and its buttons
function dock(cls) {
  const d = el(cls === "xc-rail" ? "aside" : "div", "xc-dock " + cls);
  d.append(el("div", "fig"), el("div", "rh"), el("div", "rf"));
  return d;
}

// Archo (16 §7.1 "Placements"): in the middle of the empty right pane, over the words that say
// a file can be dropped; with a result she stands under the left column (2col) or at the top
// of the rail (3col). She thinks while something is read or moved in, is pleased with a
// comparison, a thumbs-up when it is in, a shake at an error or a refusal.
const spot = () => archoSpot("import", { size: 176, calm: ["neutral", "ponder", "think", "smirk", "yum"] });
// nothing read or running is the calm state: her idle faces follow only there
const face = (e) => spot().set(e, e === "neutral");
function placeArcho(expr = "neutral") {
  const res = $("#screen .xc-preview"), showing = res && !res.hidden;
  const lay = $("#screen .xc-lay")?.dataset.layout;
  // 3col: always at the top of the rail; 2col-new: at the right of the bar over a result; 2col: with a
  // result she steps away, nothing stands beside the lists
  const host = lay === "3col" ? $("#screen .xc-rail .fig") : !showing ? $("#screen .xc-empty .fig") : $("#screen .xc-bar .fig");
  spot().stand(host, expr, expr === "neutral");
}

// row: one setting-style row; right holds its controls
function row(name, sub, ...right) {
  const r = el("div", "set"), l = el("div", "l"), rr = el("div", "r");
  l.append(el("div", "name", name));
  if (sub) l.append(el("div", "sub", sub));
  for (const x of right) if (x) rr.append(x);
  r.append(l, rr);
  return r;
}
function marked(text, ...bold) {
  // "your list %s, changed since" with the name in bold
  const parts = text.split("%s"), span = el("span");
  parts.forEach((p, i) => { span.append(p); if (i < bold.length) span.append(el("b", "", bold[i])); });
  return span;
}

// ---- the left column: what can be imported ----

// doRow: one way in: a name, what it takes, and its button
function doRow(id, ic, name, sub, btn, meta = "") {
  const r = el("div", "xc-dorow");
  r.dataset.do = id;
  const l = el("div", "l"), t = el("div", "name");
  t.append(el("span", "nt", name));
  if (meta) t.append(el("span", "meta", meta));
  l.append(t);
  if (sub) l.append(el("div", "sub", sub));
  r.append(l);
  if (btn) r.append(btn);
  return r;
}

// drawDo draws the left column: the game's current list (one click to the comparison), a Mod
// list and Mod subscriptions (their own forms), and another manager's data with the managers
// found (picked here, shown on the right).
function drawDo() {
  const card = $("#screen .xc-do");
  if (!card) return;
  card.textContent = "";
  const g = info?.game, cfg = gameConfig();
  const read = firstRun
    ? button(T("use this list"), "", () => { const after = ui.afterImport; ui.afterImport = null; firstRun = false; return after?.(T("use the game's list as it is")); }, "primary")
    : button(T("read"), "", () => g && preview("file", g.path, "", "game"));
  read.disabled = !g && !firstRun;
  const gr = doRow("game", "play", cfg ? Tf("The current list of %s", cfg) : T("The game's current list"),
    T("the list the game loads now; other managers save theirs here too"), read, g ? Tf("%d mods", g.mods) : "");
  if (g) gr.title = g.path;
  if (!g) gr.querySelector(".sub").textContent = T("the game has no ModsConfig.xml yet");
  const list = doRow("list", "plan", T("Import a Mod list"), T("a share code, a Steam collection link, a list file…"),
    button(T("import"), "", () => ui.newList("import")));
  const subs = doRow("subs", "workshop", T("Import Mod subscriptions"), T("Steam Workshop, LoversLab or GitHub links…"),
    button(T("import"), "", () => ui.go("subscribe")));
  // a pack is a list with its Mods' files; it has no other way in (10 §4)
  const pack = doRow("pack", "packs", T("Import a pack"), T("a .archopack file: a list with its Mods, checked file by file"),
    button(T("import"), "", async () => {
      let f = "";
      try { f = await api.PickPackFile(); } catch (e) { return say("fail", errText(e)); }
      if (f && S.screen === "plan-import") importPack(f);
    }));
  const db = el("div", "xc-db");
  db.append(doRow("db", "library", T("Import another manager's data"), T("Mod tags, colours, notes and sorting rules from RimSort, RimCrow or RimPy")));
  managerRows(db);
  card.append(gr, list, subs, pack, db);
  markShown();
  drawRows();
}

// markShown marks the row whose result the right shows
function markShown() {
  const res = $("#screen .xc-preview"), at = res && !res.hidden ? res.dataset.kind : "";
  for (const r of $("#screen")?.querySelectorAll?.(".xc-dorow") || []) r.classList.toggle("on", !!at && at !== "db" && r.dataset.do === at); // a picked manager shows on its own card
}

// emptyState: the right before anything is read: Archo in the middle, the drop under her
function emptyState() {
  const e = el("div", "xc-empty");
  // in 3col she stands in the rail, not here
  if (layout() !== "3col") e.append(el("div", "fig"));
  e.append(el("div", "dz-t", T("Drop the files or folders to import anywhere on this window")));
  const kinds = el("div", "dz-k");
  for (const [ic, tip] of [["plan", T("Mod lists: .xml, .rml, .json, .txt, a save (.rws)")], ["packs", T("packs: .archopack")], ["library", T("another manager's data: a folder, a .db file, userRules.json")]]) {
    const s = el("span", "ki"); s.innerHTML = icon[ic]; s.title = tip; s.setAttribute("aria-label", tip); kinds.append(s);
  }
  e.append(kinds);
  return e;
}

// ---- dropping files on the window (16 §2.18 "Dropping files") ----

// dropKind names what a dropped file or folder is by its name: "list", "pack", "manager" (a
// RimCrow or RimSort database, RimSort's userRules.json), "folder", or "" when the page cannot
// import it.
export function dropKind(name, dir = false) {
  if (dir) return "folder";
  const n = String(name || "").toLowerCase().replace(/^.*[\\/]/, "");
  if (n.endsWith(".archopack")) return "pack";
  if (n.endsWith(".db") || n === "userrules.json") return "manager";
  if (/\.(xml|rml|json|txt|rws)$/.test(n)) return "list";
  return "";
}

let dragDepth = 0;
function dropOverlay() {
  let o = document.querySelector(".xc-dropover");
  if (o) return o;
  o = el("div", "xc-dropover");
  o.setAttribute("aria-hidden", "true");
  const box = el("div", "box"), ic = el("span", "dz-i"); ic.innerHTML = icon.import;
  const kinds = el("span", "dz-k");
  for (const k of ["plan", "packs", "library"]) { const s = el("span", "ki"); s.innerHTML = icon[k]; kinds.append(s); }
  box.append(ic, el("span", "dz-t", T("drop to import")), kinds);
  o.append(box);
  document.body.append(o);
  return o;
}
export function showDropOver(on) {
  const o = on ? dropOverlay() : document.querySelector(".xc-dropover");
  if (o) o.classList.toggle("on", on);
}

// wireDrop: a file dragged over any page shows the overlay; dropped, it opens the Import page
// and goes where it belongs. A form that takes drops itself (the lists' new / import) handles
// them first. Under WebView2 composition hosting the native window receives no drops yet
// (16 §8.2); the browser harness does.
function wireDrop() {
  const files = e => e.dataTransfer?.types?.includes?.("Files");
  document.addEventListener("dragenter", e => { if (!files(e)) return; dragDepth++; showDropOver(true); });
  document.addEventListener("dragleave", e => { if (!files(e)) return; if (--dragDepth <= 0) { dragDepth = 0; showDropOver(false); } });
  document.addEventListener("dragover", e => { if (files(e)) e.preventDefault(); });
  document.addEventListener("drop", async e => {
    dragDepth = 0; showDropOver(false);
    if (e.defaultPrevented) return;
    const item = [...(e.dataTransfer?.items || [])].find(i => i.kind === "file");
    const f = item?.getAsFile?.() || e.dataTransfer?.files?.[0];
    if (!f) return;
    e.preventDefault();
    const dir = !!item?.webkitGetAsEntry?.()?.isDirectory;
    await dropped({ name: f.name, file: f, dir, more: (e.dataTransfer.files?.length || 1) - 1 });
  });
}

// dropped opens a dropped file or folder in the Import page. With a path (a native drop) it
// goes where a chosen file goes; without one (the browser) a list is read from its content
// and a pack, a database or a folder asks for the dialog.
export async function dropped(d) {
  const k = dropKind(d.path || d.name, d.dir);
  if (S.screen !== "plan-import") await exchange();
  if (S.screen !== "plan-import") return;
  if (d.more > 0) say("", Tf("one file at a time: reading %s", d.name));
  if (!k) return refuse(d.name);
  if (d.path) return k === "folder" ? addManagerSource(d.path) : openPicked(d.path);
  if (k === "list") return readFile(d.file);
  // the browser gives no path: the dialog does
  return refuse(d.name, k === "folder" || k === "manager" ? T("add it with “add…” under another manager's data") : T("choose it with “import” in Import a pack"), "warn");
}

// refuse says why a drop is not imported, on the right, and keeps the page as it was
function refuse(name, why = "", cls = "") {
  const res = showResult("refused", "", T("Not imported"));
  if (!res) return;
  const r = errorRow(name, why || T("RimArcho imports Mod lists (.xml, .rml, .json, .txt, .rws), packs (.archopack) and RimSort, RimCrow or RimPy data"));
  if (cls) r.classList.add(cls);
  // a refusal reads at a glance: the file's name after a stop mark (an ask for the dialog: a warning)
  const mk = el("span", "xc-refuse" + (cls ? " " + cls : "")); mk.innerHTML = cls ? icon.warn : icon.err;
  r.querySelector(".name").prepend(mk);
  res.append(r, footer("", button(T("close"), "", hideResult)));
  place();
  face("shake");
  say("fail", (why ? "" : T("not imported") + ": ") + (why || name));
}

// ---- everything from another manager ----

// M is the moving-in state of this page: what the survey found, the managers picked above,
// the parts ticked below and the run in progress.
let M = null;
function freshM() { return { managers: null, error: "", shown: new Set(), on: new Set(), mode: new Map(), open: new Set(), extra: M?.extra || [], run: null }; }

// managerRows: one card per manager found (drawRows fills them in), then "Not found? Add it
// yourself" with its button.
function managerRows(card) {
  const more = button(T("add…"), "", async () => {
    let f = "";
    try { f = await api.PickManagerImport(); } catch (e) { return say("fail", errText(e)); }
    if (!f || S.screen !== "plan-import") return;
    addManagerSource(f);
  });
  more.title = T("a data folder or file kept elsewhere; it is only read");
  const elsewhere = el("div", "xc-else");
  elsewhere.append(el("span", "", T("Not found? Add it yourself")), more);
  card.append(el("div", "xc-mrows"), elsewhere);
}

// addManagerSource adds a manager's folder or file kept elsewhere (the dialog, or a dropped one)
// and shows what it holds; a folder or file with no manager's data in it is refused on the right.
async function addManagerSource(f) {
  const active = session;
  const k = await api.ManagerFileKind(f).catch(() => "list");
  M.extra.push({ manager: k === "list" ? T("a file") : "", kind: k, path: f });
  await survey();
  if (!live(active)) return;
  const norm = x => String(x || "").replace(/\\/g, "/").toLowerCase();
  const m = M.managers?.find(m => m.groups.some(g => norm(g.path).startsWith(norm(f))));
  if (!m) return refuse(f.replace(/^.*[\\/]/, ""), T("no RimSort, RimCrow or RimPy data in it"));
  if (!M.shown.has(mKey(m))) showManager(m);
}

// survey asks what each manager holds (dry runs in their own session); rows fill in.
async function survey() {
  const active = session;
  M.managers = null; M.error = "";
  drawRows();
  try {
    const managers = await api.ImportSurvey(M.extra);
    if (!live(active)) return;
    M.managers = managers || [];
  } catch (e) { if (!live(active)) return; M.managers = []; M.error = errText(e); }
  drawRows(); drawTree();
}

const PART = () => ({ aliases: T("aliases"), notes: T("notes"), groups: T("groups"), tags: T("tags"), colours: T("colour marks"), ignored: T("ignored warnings"),
  rules: T("user rules"), "rules-off": T("rules switched off there"), interlocks: T("interlocks") });
const BECOMES = () => ({ aliases: T("%d aliases"), notes: T("%d notes"), groups: T("%d group tags on Mods"), tags: T("%d tags on Mods"), colours: T("%d coloured tags"),
  ignored: T("a tag on %d Mods"), rules: T("%d load-order rules"), "rules-off": T("%d load-order rules"), interlocks: T("%d attach rules") });
const GROUP = () => ({ configs: T("profiles"), player: T("notes and tags"), rules: T("rules"), lists: T("saved lists") });

const mKey = m => m.name;
const gKey = (m, g) => m.name + "|" + g.kind + "|" + (g.path || "");
const visible = g => g.parts.filter(p => p.state !== "empty");
const allParts = m => m.groups.flatMap(visible);
// usable: what counts towards "N of M"; toggleable: what a box above turns on (a list
// imported before only comes again when ticked on its own row)
const toggleable = ps => ps.filter(p => p.state === "new");
const usable = ps => ps.filter(p => p.state === "new" || (p.state === "listed" && M.on.has(p.id)));
const picked = ps => usable(ps).filter(p => M.on.has(p.id)).length;
const stateOf = ps => { const u = usable(ps), a = u.filter(p => M.on.has(p.id)).length; return a === 0 ? "" : a === u.length ? "checked" : "mixed"; };

function pickLine(m) {
  const ps = allParts(m), tog = toggleable(ps), tOn = tog.filter(p => M.on.has(p.id)).length;
  const again = ps.filter(p => p.state === "listed" && M.on.has(p.id)).length;
  if (!tog.length && !again) return T("nothing new");
  const first = tOn && tOn === tog.length ? (m.imported ? T("everything new") : T("everything")) : tOn ? Tf("%d of %d parts", tOn, tog.length) : "";
  return [first, again ? T("lists again") + " " + again : ""].filter(Boolean).join(" + ") || T("nothing");
}
// news: what changed since the manager was imported, for its row
function news(m) {
  const out = [], P = PART();
  let lists = 0, changed = 0;
  for (const g of m.groups) for (const p of visible(g)) {
    if (g.kind === "lists") { if (p.state === "new") lists++; else if (p.list?.changed) changed++; continue; }
    if (p.state !== "new" || !p.default) continue; // what is off by default is not news
    if (g.kind === "configs") out.push(p.name);
    else out.push(P[p.part] + " " + p.new);
  }
  if (lists) out.push(T("lists") + " " + lists);
  if (changed) out.push(T("changed lists") + " " + changed);
  return out;
}
const holds = m => m.groups.filter(g => visible(g).length).map(g => GROUP()[g.kind] + (g.kind === "lists" || g.kind === "configs" ? " " + visible(g).length : "")).join(", ");
const dayText = ms => { const d = new Date(ms), now = new Date(); return d.toDateString() === now.toDateString() ? T("today") : d.toLocaleDateString(undefined, { month: "numeric", day: "numeric" }); };
const MARK = { RimSort: ["RS", 205], RimCrow: ["RC", 18], RimPy: ["RP", 140] };
function mark(name, size = 32) {
  const [t, h] = MARK[name] || [name.slice(0, 2).toUpperCase(), 260];
  const s = el("span", "xc-mgr", t);
  s.style.cssText = `--h:${h};width:${size}px;height:${size}px;font-size:${Math.round(size * 0.36)}px`;
  return s;
}
const TICK = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3.2 8.4l3 3 6.6-6.8"/></svg>';
function box(state, tag = "span") {
  const b = el(tag, "xc-cb" + (state === "checked" ? " on" : state === "mixed" ? " part" : ""));
  b.innerHTML = TICK;
  if (tag === "button") { b.setAttribute("role", "checkbox"); b.setAttribute("aria-checked", state === "checked" ? "true" : state === "mixed" ? "mixed" : "false"); }
  return b;
}

// the rows above: one per manager, picked with a click
function drawRows() {
  const rows = $("#screen .xc-mrows");
  if (!rows) return;
  rows.textContent = "";
  if (!M.managers) { rows.append(el("div", "xc-hint", T("looking for RimSort, RimCrow and RimPy…"))); return; }
  if (M.error) rows.append(el("div", "xc-note warn", M.error));
  if (!M.managers.length) { rows.append(el("div", "xc-hint", T("No RimSort, RimCrow or RimPy data found."))); return; }
  const rank = m => !m.imported ? 1 : news(m).length ? 0 : 2;
  for (const m of [...M.managers].sort((a, b) => rank(a) - rank(b))) {
    const ps = allParts(m), st = stateOf(ps), on = M.shown.has(mKey(m)), nws = m.imported ? news(m) : [];
    const running = M.run && M.run.mgrs.includes(m.name);
    const b = el("button", "xc-mrow" + (on && !running ? " on" : "") + (m.imported && !nws.length && !on ? " seen" : ""));
    b.setAttribute("role", "checkbox");
    b.setAttribute("aria-checked", st === "checked" ? "true" : st === "mixed" ? "mixed" : "false");
    const t = el("span", "t", m.name);
    if (m.imported) t.append(el("span", "when", Tf("imported %s", dayText(m.imported))));
    const s = el("span", "s");
    if (running) s.append(runLine()); // quiet: the run panel below shows the progress
    else if (st) s.append(el("span", "pick", Tf("bringing over %s", pickLine(m))));
    else if (on) s.append(ps.some(p => p.state === "listed") ? T("nothing new to choose; tick a list below to import it again") : T("all of it is already in RimArcho"));
    else if (m.imported && nws.length) { const n = el("span", "nline"); n.title = nws.join(", "); n.append(el("span", "chip-n", T("new")), el("span", "nt", nws.join(", "))); s.append(n); }
    else if (m.imported) s.append(T("nothing new since then"));
    else s.append(holds(m));
    // the card itself shows it is picked (its accent edge): no box in front of it
    b.append(mark(m.name, 24), t, s);
    b.onclick = () => showManager(m);
    rows.append(b);
  }
}

function showManager(m) {
  if (M.run && M.run.state === "running") return say("", T("a move is running; stop it or wait"));
  if (M.run) M.run = null;
  ++previewSeq;
  const res = $("#screen .xc-preview");
  if (res) res.dataset.mode = "move";
  const k = mKey(m), ps = allParts(m);
  if (M.shown.has(k)) { ps.forEach(p => { M.on.delete(p.id); M.mode.delete(p.id); }); M.shown.delete(k); }
  else {
    // picking a manager turns on only what is new; what came in before stays off
    ps.forEach(p => { if (p.state === "new" && p.default) M.on.add(p.id); });
    M.shown.add(k); M.open.add(k);
    for (const g of m.groups) if (m.imported && visible(g).some(p => p.state === "new" || p.list?.changed)) M.open.add(gKey(m, g));
  }
  drawRows(); drawTree();
  if (M.shown.size) reveal();
}

function partLabel(g, p) {
  if (g.kind === "configs" || g.kind === "lists") return p.name;
  return PART()[p.part] || p.part;
}
function partBecomes(g, p) {
  if (g.kind === "configs") return T("a Game Config with its list");
  if (g.kind === "lists") {
    if (p.state === "listed") return M.mode.get(p.id) === "update" ? Tf("changes to your list %s; undoable", p.list.name) : T("another copy");
    return T("a new list");
  }
  return Tf(BECOMES()[p.part] || "%d", p.new);
}
// groupInstance: the RimSort instance a player-data group comes from (its folder name)
function groupInstance(g) {
  return g.kind === "player" && /aux_metadata\.db$/i.test(g.path || "") ? g.path.split(/[\\/]/).slice(-2, -1)[0] || "" : "";
}
function groupLabel(g) {
  const inst = groupInstance(g);
  return GROUP()[g.kind] + (inst ? " / " + inst : "");
}

function treeRow(lv, open, twKey, st, disabled, iconEl, label, meta, pills, right, onBox) {
  const r = el("div", `xc-tn lv${lv}` + (open ? " open" : "") + (st ? "" : " off"));
  r.style.setProperty("--lv", lv);
  if (twKey) {
    const tw = el("button", "tw"); tw.innerHTML = icon.chev;
    tw.setAttribute("aria-label", open ? T("fold") : T("unfold"));
    tw.onclick = () => { if (M.open.has(twKey)) M.open.delete(twKey); else M.open.add(twKey); drawTree(); };
    r.append(tw);
  } else r.append(el("span"));
  const b = box(st, "button"); b.disabled = disabled;
  b.onclick = onBox;
  r.append(b, iconEl || el("span"));
  const lb = el("span", "lb", label);
  if (meta) lb.append(el("span", "m", meta));
  for (const [cls, t] of pills) lb.append(el("span", "xc-pill " + cls, t));
  r.append(lb, right);
  return r;
}
function titled(e) { e.title = e.textContent; return e; }
// progBar is the GUI's progress bar (.prog); busy when the task has no count
function progBar(pct, busy = false) {
  const p = el("div", "prog"), b = el("div", "bar" + (busy ? " busy" : "")), i = el("i");
  i.style.width = pct + "%";
  b.append(i); p.append(b);
  return p;
}
function have(t) { const s = el("span", "bc have"); s.innerHTML = icon.ok; s.append(t); return s; }

function drawTree() {
  const res = $("#screen .xc-preview");
  if (!res || (res.dataset.mode && res.dataset.mode !== "move")) return;
  if (M.run) return drawRun();
  const list = (M.managers || []).filter(m => M.shown.has(mKey(m)));
  if (!list.length) { if (res.dataset.mode === "move") hideResult(); return; }
  showResult("move", "m", Tf("What comes over from %s", list.map(m => m.name).join(", ")));
  const tree = el("div", "xc-tree");
  for (const m of list) {
    const ps = allParts(m), k = mKey(m), st = stateOf(ps);
    tree.append(treeRow(0, M.open.has(k), k, st, !toggleable(ps).length && !ps.some(p => M.on.has(p.id)), mark(m.name, 22), m.name,
      m.imported ? Tf("imported %s", dayText(m.imported)) : "", [], el("span", "bc muted", pickLine(m)), () => toggle(ps, st)));
    if (!M.open.has(k)) continue;
    for (const g of m.groups) {
      const gp = visible(g);
      if (!gp.length && !g.error) continue;
      const gk = gKey(m, g), gst = stateOf(gp);
      const ic = el("span", "ti"); ic.innerHTML = icon[{ configs: "env", player: "plan", rules: "rescan", lists: "plan" }[g.kind]] || "";
      const allHad = gp.length && gp.every(p => p.state === "had" || p.state === "blocked") && gp.some(p => p.state === "had");
      const a = picked(gp), u = usable(gp).length;
      const right = g.error ? el("span", "why", g.error)
        : allHad ? have(T("already in RimArcho"))
        : gst === "mixed" ? el("span", "bc becomes", Tf("%d of %d chosen", a, u))
        : gst ? titled(el("span", "bc becomes", gp.filter(p => M.on.has(p.id)).map(p => g.kind === "lists" || g.kind === "configs" ? partLabel(g, p) : partBecomes(g, p)).join(", ")))
        : !toggleable(gp).length && gp.some(p => p.state === "listed") ? el("span", "bc", T("already your lists; tick one to import it again"))
        : el("span", "bc", T("not brought over"));
      const row = treeRow(1, M.open.has(gk), gp.length ? gk : "", gst, !toggleable(gp).length && !gp.some(p => M.on.has(p.id)), ic, GROUP()[g.kind], groupInstance(g), [], right, () => toggle(gp, gst));
      if (allHad) row.classList.add("had");
      tree.append(row);
      if (!M.open.has(gk)) continue;
      for (const p of gp) tree.append(partRow(m, g, p));
    }
  }
  const sum = el("div", "xc-tsum");
  const chosen = list.filter(m => picked(allParts(m)));
  if (!chosen.length) sum.append(el("span", "", T("nothing selected")));
  for (const m of chosen) { const s = el("span"); s.append(el("b", "", m.name), " " + pickLine(m)); sum.append(s); }
  const acts = el("div", "xc-acts");
  const any = list.some(m => picked(allParts(m)));
  acts.append(el("span", "note", (list.some(m => m.imported) ? T("What is already in RimArcho is greyed and stays as it is. A list comes in again only if you tick it.") : T("Only read from the other manager. Values you already have stay.")) + " " + T("Cannot be undone, except a list's content.")),
    button(T("cancel"), "", () => { M.on.clear(); M.mode.clear(); M.shown.clear(); drawRows(); drawTree(); }),
    button(T("bring over the selected"), "", startRun, "primary"));
  acts.lastChild.disabled = !any;
  res.append(tree, sum, acts);
  place();
}

function partRow(m, g, p) {
  const on = M.on.has(p.id), st = on ? "checked" : "";
  const P = [];
  let meta = "";
  if (g.kind === "lists" && p.list) meta = Tf("%d mods", p.list.mods) + (p.list.missing ? ", " + Tf("%d not installed", p.list.missing) : "");
  else if (g.kind === "configs") meta = p.meta || "";
  else meta = String(p.new + p.same);
  if (m.imported && p.state === "new" && g.kind !== "lists" && g.kind !== "configs" && p.same) P.push(["new", Tf("%d new", p.new)]);
  if (m.imported && p.state === "new" && (g.kind === "lists" || g.kind === "configs")) P.push(["new", T("new")]);
  if (p.list?.changed) P.push(["chg", T("changed")]);
  let right;
  if (p.state === "blocked") right = el("span", "why", p.why || "");
  else if (p.state === "had") right = have(g.kind === "configs" ? Tf("already the Game Config %s", p.why) : T("already in RimArcho"));
  else if (on) right = el("span", "bc becomes", partBecomes(g, p));
  else if (p.state === "listed") {
    right = el("span", "bc");
    const name = el("b", "", p.list.name);
    const lnk = (t, mode) => { const b = el("button", "xc-lnk", t); b.onclick = () => { M.on.add(p.id); M.mode.set(p.id, mode); drawRows(); drawTree(); }; return b; };
    if (p.list.changed) right.append(marked(T("your list %s, changed since"), p.list.name), lnk(T("update it"), "update"), lnk(T("another copy"), "copy"));
    else { right.append(T("already the list") + " ", name, lnk(T("import another copy"), "copy")); }
  } else right = el("span", "bc", T("not brought over"));
  const row = treeRow(2, false, "", st, p.state === "blocked" || p.state === "had", null, partLabel(g, p), meta, P, right, () => {
    if (on) { M.on.delete(p.id); M.mode.delete(p.id); }
    else { M.on.add(p.id); if (p.state === "listed") M.mode.set(p.id, p.list.changed ? "update" : "copy"); }
    drawRows(); drawTree();
  });
  if (p.state === "had") row.classList.add("had");
  if (p.state === "blocked") row.classList.add("dis");
  // view: what the part holds, read without bringing it over
  if (p.state !== "blocked") {
    const v = el("button", "xc-lnk xc-view", T("view"));
    v.type = "button";
    v.onclick = () => viewPart(m, g, p);
    row.append(v);
  }
  return row;
}

// viewPart shows what one part holds: a list or a Game Config as the order comparison, Mod
// data and rules value by value. "back" returns to the tree with the picks as they were.
async function viewPart(m, g, p) {
  const active = session, seq = ++previewSeq;
  say("busy", T("reading…"));
  let v;
  try { v = await api.ManagerPartView(p.id); } catch (e) { if (live(active)) say("fail", errText(e)); return; }
  if (!live(active) || seq !== previewSeq) return;
  say("", "");
  const back = () => { const res = $("#screen .xc-preview"); if (res) res.dataset.mode = "move"; ++previewSeq; drawTree(); };
  if (v.list) return preview("file", v.list, "", "db", { back, name: partLabel(g, p) });
  const res = showResult("view", "m", "");
  if (!res) return;
  const items = v.items || [], had = items.filter(i => i.had).length;
  const head = el("div", "xc-rhead xc-vhead");
  const t = el("span", "xc-vt");
  t.append(mark(m.name, 20), el("b", "", m.name), el("span", "", groupLabel(g) + " / " + partLabel(g, p)));
  const n = el("span", "xc-vn");
  n.append(el("span", "", Tf("%d new", items.length - had)));
  if (had) n.append(el("span", "had", Tf("%d already in RimArcho", had)));
  head.append(t, n);
  const list = el("div", "xc-items");
  const SHOW = 3000;
  for (const it of items.slice(0, SHOW)) {
    const r = el("div", "xc-item" + (it.had ? " had" : ""));
    r.append(el("span", "md", it.mod), el("span", "vl", itemValue(it)));
    r.title = it.mod + "\n" + itemValue(it) + (it.had ? "\n" + T("already in RimArcho") : "");
    list.append(r);
  }
  if (!items.length) list.append(el("div", "xc-hint", T("nothing in it")));
  if (items.length > SHOW || v.more) list.append(el("div", "xc-hint", Tf("only the first %d are shown", Math.min(SHOW, items.length))));
  res.append(head, list, footer("", button(T("back"), "", back)));
  place();
  face("smug");
}
const RULE = () => ({ after: T("loads after %s"), before: T("loads before %s"), incompatible: T("incompatible with %s"), attach: T("attached to %s"), top: T("loads at the top"), bottom: T("loads at the bottom") });
function itemValue(it) {
  if (it.part === "rules" || it.part === "rules-off" || it.part === "interlocks") return Tf(RULE()[it.value] || it.value, it.with || "");
  return it.value;
}

// a box turns its whole branch on (what is new), or off when all of it is on
function toggle(ps, st) {
  if (st === "checked") ps.forEach(p => { M.on.delete(p.id); M.mode.delete(p.id); });
  else toggleable(ps).forEach(p => M.on.add(p.id));
  drawRows(); drawTree();
}

// ---- the run: one background task, an item per part (16 §2.16) ----

const runPct = () => M.run?.task ? Math.round((M.run.task.done || 0) / Math.max(1, M.run.picks.length) * 100) : 0;
function runLine() {
  const r = M.run, n = r.task?.done || 0, all = r.picks.length;
  return r.state === "running" ? Tf("moving in… %d / %d", n, all) : r.state === "stopped" ? Tf("stopped at %d / %d", n, all) : T("moved in");
}

async function startRun(picks) {
  const active = session;
  if (!picks) {
    picks = [];
    for (const m of M.managers.filter(m => M.shown.has(mKey(m))))
      for (const g of m.groups) for (const p of visible(g))
        if (M.on.has(p.id)) picks.push({ id: p.id, mode: M.mode.get(p.id) || "", name: partLabel(g, p), mgr: m.name, group: groupLabel(g), kind: g.kind });
  }
  if (!picks.length) return;
  const mgrs = [...new Set(picks.map(p => p.mgr))];
  try {
    const id = await api.StartMigration(picks.map(p => ({ id: p.id, mode: p.mode, name: p.mgr + ": " + p.name })), mgrs.join(", "));
    if (!live(active)) return;
    M.run = { id, picks, mgrs, state: "running", task: null };
    say("", Tf("moving in from %s: a background task; you can leave this page", mgrs.join(", ")));
    poll(active);
  } catch (e) { say("fail", errText(e)); }
}

async function poll(active) {
  const r = M.run;
  if (!r || !live(active)) return;
  try { r.task = await api.TaskByID(r.id); } catch { /* keep the last record */ }
  if (!live(active) || M.run !== r) return;
  const st = r.task?.state;
  r.state = st === "run" || st === "queue" || !st ? "running" : st === "stop" ? "stopped" : "finished";
  drawRun(); drawRows();
  if (r.state === "running") setTimeout(() => poll(active), 400);
  else { await ui.reload(); say(st === "done" ? "ok" : "fail", r.task?.result || ""); }
}

function drawRun() {
  const r = M.run;
  if (!r) return;
  const res = showResult("move", "m", Tf("Moving in from %s", r.mgrs.join(", ")));
  if (!res) return;
  const items = r.task?.items || r.picks.map(p => ({ ref: p.id, state: "wait" }));
  const fails = items.filter(i => i.state === "fail");
  const done = items.filter(i => i.state === "ok").length;
  face(r.state === "running" ? "think" : r.state === "stopped" ? "neutral" : fails.length ? "skeptic" : "thumbsup");
  const head = el("div", "xc-run-h"), t = el("span");
  t.append(el("span", "t", r.mgrs.join(", ")), " ", el("span", "n",
    r.state === "running" ? runLine() : r.state === "stopped" ? Tf("stopped: %d of %d done", done, items.length)
      : Tf("done: %d of %d parts", done, items.length) + (fails.length ? ", " + Tf("%d failed", fails.length) : "")));
  head.append(t, el("span", "r n", r.state === "running" ? runPct() + "%" : ""), progBar(r.state === "running" ? runPct() : 100));
  const pg = el("div", "xc-pg");
  let lastM = "", lastG = "";
  items.forEach((it, i) => {
    const p = r.picks[i] || {};
    if (p.mgr !== lastM) { const h = el("div", "xc-pk"); h.style.setProperty("--lv", 0); h.append(mark(p.mgr, 20), el("b", "", p.mgr)); pg.append(h); lastM = p.mgr; lastG = ""; }
    if (p.group !== lastG) { const h = el("div", "xc-pk", p.group); h.style.setProperty("--lv", 1); pg.append(h); lastG = p.group; }
    const st = it.state === "ok" ? "done" : it.state === "skip" ? "cancel" : it.state || "wait";
    const row = el("div", "xc-pi " + st); row.style.setProperty("--lv", 2);
    const si = el("span", "xc-si " + st + (st === "run" ? " icon spin" : ""));
    if (st === "run") si.innerHTML = icon.rescan;
    else if (st === "done") si.innerHTML = TICK; else if (st === "fail") si.innerHTML = icon.warn; else if (st === "cancel") si.innerHTML = icon.close;
    const text = st === "wait" ? T("waiting") : st === "cancel" ? T("not done: stopped") : st === "run" ? "…" : st === "fail" ? it.why : it.note;
    row.append(si, el("span", "lb", p.name), factText(el("span", "rs"), text || ""));
    pg.append(row);
  });
  const f = el("div", "xc-acts");
  if (r.state === "running") {
    f.append(el("span", "note", T("Running as a background task. You can leave this page; it goes on.")),
      button(T("background tasks"), "", () => ui.go("tasks")),
      button(T("stop"), "", () => api.TaskAct("stop", [r.id], "").catch(e => say("fail", errText(e)))));
  } else if (r.state === "stopped") {
    const rest = r.picks.filter((_, i) => items[i]?.state !== "ok" && items[i]?.state !== "fail");
    f.append(el("span", "note", Tf("Finished parts are kept. %d not done.", rest.length)),
      button(T("choose again"), "", again), button(Tf("continue %d", rest.length), "", () => startRun(rest), "primary"));
  } else {
    const failed = r.picks.filter((_, i) => items[i]?.state === "fail");
    f.append(el("span", "note", (failed.length ? Tf("%d failed; the rest is in RimArcho.", failed.length) : T("Everything is in RimArcho.")) + " " + T("Notes and tags show in the Mod library; lists under lists; Game Configs in the switcher.")));
    if (failed.length) f.append(button(Tf("retry %d", failed.length), "", () => startRun(failed)));
    if (r.picks.some(p => p.kind === "lists")) f.append(button(T("open lists"), "", () => ui.go("plans")));
    f.append(button(T("done"), "", again, "primary"));
  }
  res.append(head, pg, f);
  place();
}

// again closes the run: the page asks the managers again, so what came in shows as imported
function again() {
  const after = session?.from === "guide" && M.run?.state === "finished" ? ui.afterImport : null;
  M.run = null; M.on.clear(); M.mode.clear(); M.shown.clear();
  hideResult();
  if (after) { ui.afterImport = null; return after(); }
  survey();
}

// openPicked sends a chosen file where it belongs: a pack is added to the packs, another
// manager's database or rules join the managers above, anything else gets the list preview.
async function openPicked(path) {
  const active = session;
  if (/\.archopack$/i.test(path)) return importPack(path);
  let kind = "list";
  try { kind = await api.ManagerFileKind(path); } catch { /* read it as a list */ }
  if (!live(active)) return;
  if (kind === "list") return preview("file", path);
  M.extra.push({ manager: "", kind, path });
  await survey();
  const m = M.managers?.find(m => m.groups.some(g => g.path === path));
  if (m && !M.shown.has(mKey(m))) showManager(m);
}

// importPack adds a pack as a background task and shows it in place. A newer revision of a
// pack the player has is confirmed first, with what it changes (10 §4).
async function importPack(path, update = null) {
  if (update === null) {
    let offer = null;
    try { offer = await api.PackUpdateOffer(path); } catch { /* the import reports it */ }
    if (offer) return pop.confirm(offer.title, offer.lines, T("update the pack"), () => importPack(path, true));
  }
  const active = session, seq = ++previewSeq;
  const name = path.replace(/^.*[\\/]/, "");
  const result = showResult("pack", "pack", T("Adding a pack"));
  if (!result) return;
  let id;
  try { id = await api.StartPackImport(path, !!update); }
  catch (e) { result.append(errorRow(name, errText(e)), footer("", button(T("close"), "", hideResult))); face("shake"); reveal(); return say("fail", errText(e)); }
  say("", T("adding the pack: a background task; you can leave this page"));
  let first = true;
  const tick = async () => {
    if (!live(active) || seq !== previewSeq) return;
    let t = null;
    try { t = await api.TaskByID(id); } catch { /* try again */ }
    if (!live(active) || seq !== previewSeq) return;
    const running = !t || t.state === "run" || t.state === "queue";
    face(running ? "think" : t.state === "done" ? "thumbsup" : "shake");
    result.textContent = "";
    const head = el("div", "xc-run-h"), h = el("span");
    h.append(el("span", "t", name), " ", el("span", "n", running ? T("adding the pack…") : t.state === "done" ? T("added to your packs") : T("not added")));
    head.append(h, el("span"), progBar(running ? 0 : 100, running));
    const pg = el("div", "xc-pg");
    if (!running && t.result) pg.append(el("pre", "lines xc-out" + (t.state === "done" ? "" : " err"), t.result));
    const f = el("div", "xc-acts");
    if (running) f.append(el("span", "note", T("Running as a background task. You can leave this page; it goes on.")), button(T("background tasks"), "", () => ui.go("tasks")));
    else if (t.state === "done") f.append(el("span", "note", T("The pack is ready to launch.")), button(T("open packs"), "enter", () => ui.go("packs"), "primary"));
    else f.append(el("span", "note", ""), button(T("close"), "", hideResult), button(T("choose another pack…"), "", async () => { const p = await api.PickPackFile().catch(() => ""); if (p) importPack(p); }));
    result.append(head, pg, f);
    place();
    if (first) { first = false; reveal(); }
    if (running) setTimeout(tick, 500);
    else { await ui.reload(); say(t.state === "done" ? "ok" : "fail", t.state === "done" ? T("pack added") : t.result); }
  };
  tick();
}

async function readFile(f) {
  const active = session;
  // a dropped or browser-picked file has no path: a pack or a database needs the dialog
  if (/\.(archopack|db)$/i.test(f.name)) return say("fail", T("choose this file with “choose a file…”"));
  try {
    const blob = /\.rws$/i.test(f.name) ? f.slice(0, 4 << 20) : f;
    const text = await blob.text();
    if (active === session && S.screen === "plan-import") preview("text", text, f.name);
  } catch (e) { say("fail", errText(e)); }
}

// ---- import, step 2: the order comparison ----
// A list read here (the game's, a dropped or chosen file) is shown next to the player's list,
// in order (16 §2.18 "Order comparison"): its source, the counts, the two lists side by side,
// and the buttons — apply it to the list, save it as a new list, or add only what the list lacks.

async function preview(kind, src, fileName, from = "file", view = null) {
  const body = $("#screen .xc-preview");
  const active = session, seq = ++previewSeq;
  const current = () => S.screen === "plan-import" && session === active && S.screenSeq === active.epoch && seq === previewSeq;
  if (!body) return;
  delete body.dataset.fieldError;
  const link = kind === "text" && /^\s*\S+\s*$/.test(src) && /steamcommunity|rentry|^\s*\d{6,}\s*$/.test(src);
  say("busy", link ? T("reading the link…") : T("reading the list…"));
  let p;
  showResult("list", from, T("Mod order comparison"));
  const wait = row(link ? T("reading the link…") : T("reading the list…"), "", progBar(0, true));
  wait.classList.add("xc-wait");
  body.append(wait);
  face("think");
  reveal();
  try { p = await api.ImportPreviewFor(active.target, kind, src); }
  catch (e) {
    if (!current()) return;
    body.textContent = "";
    body.dataset.fieldError = fileName || kind === "file" ? "file" : "text";
    body.append(errorRow(T("could not read it"), errText(e)), footer("", button(T("close"), "", hideResult)));
    place();
    face("shake");
    say("fail", errText(e)); reveal(); return;
  }
  if (!current()) return;
  say("", "");
  if (fileName) p.source = fileName;
  body.textContent = "";
  face("smug");

  const fromGame = kind === "file" && !!info?.game && src === info.game.path;
  const cfg = gameConfig();
  const theirs = fromGame ? (cfg ? Tf("%s (the game)", cfg) : T("the game's list")) : view?.name || p.name || (p.source || "").replace(/^.*[\\/]/, "") || p.format;
  const d = orderDiff(p.current || [], p.incoming || []);
  // where it comes from (format, file, game version) is the incoming list's header tooltip;
  // another game version is an amber mark there
  const tip = [p.format, p.source, p.game ? "RimWorld " + p.game : "", p.gameNote].filter(Boolean).join("\n");
  const ov = orderView(d, { left: active.name || T("your list"), right: theirs, rightTip: tip, rightNote: p.gameNote ? "RimWorld " + (p.game || "").split(".").slice(0, 2).join(".") : "", rightNoteTip: p.gameNote || "" });

  // the head is one line: the counts, what is not installed (subscribe them all), close
  const head = el("div", "xc-rhead");
  head.append(legend(d, active.name || T("your list"), theirs));
  if (p.missing.length) {
    const ids = workshopIDs(p.missing);
    const m = el("div", "xc-miss");
    const t = el("span", "mt", Tf("%d not installed", p.missing.length));
    t.title = p.missing.map(r => r.name).join(", ");
    m.append(t);
    if (ids.length) { const go = button(Tf("subscribe all %d", ids.length), "", () => getAll(ids, go), "small", "subscribe"); go.type = "button"; go.title = T("subscribed mods take their place in the list once downloaded"); m.append(go); }
    head.append(m);
  }
  body.append(head, ov.el);

  // a list viewed from another manager's tree: only a way back (bringing it over is the tree's)
  if (view) { body.append(footer("", button(T("back"), "", view.back))); place(); reveal(); return; }

  // the list already is this one: nothing to import (16 §2.18)
  if (p.replace?.same && !p.settings && !p.collection) {
    body.append(footer(fromGame ? Tf("%s already matches the game.", active.name) : Tf("%s already is this list.", active.name), button(T("close"), "", hideResult)));
    place(); reveal();
    return;
  }

  // the choices that ride along: follow a Workshop collection (07 §7.6), a list's Mod settings (07 §7.3)
  let follow = !!p.collection, settings = false;
  const opts = [];
  if (p.collection) opts.push(checkLine(T("follow this collection"), T("RimArcho tells you when it changes; catching up is one undoable step"), () => follow, v => { follow = v; }));
  if (p.settings) opts.push(checkLine(Tf("apply the Mod settings of %d mods", p.settings), T("after the import; settings files you have are kept"), () => settings, v => { settings = v; }));

  let busy = false;
  const name0 = fromGame && cfg ? cfg : p.name || (p.source || "").replace(/^.*[\\/]/, "").replace(/\.[^.]*$/, "");
  async function perform(dest, btn, listName = "") {
    if (busy || !current()) return;
    busy = true;
    for (const b of acts.querySelectorAll("button")) b.disabled = true;
    // the import runs: the button says so and Archo thinks until it is in
    btn.setAttribute("aria-busy", "true"); btn.classList.add("xc-working");
    const spin = el("span", "icon spin"); spin.innerHTML = icon.rescan; btn.prepend(spin);
    say("busy", T("importing…")); face("think");
    try {
      await api.ImportFollow(follow);
      const r = dest === "new" ? await api.ImportAsPlan(listName) : await api.ImportApplyTo(active.target, dest === "merge");
      if (!current()) return;
      let extra = "";
      if (settings) {
        try {
          const pl = await api.ImportSettings([], false, true);
          extra = settingsLine(await api.ImportSettings(pl.mods.filter(m => !m.missing).map(m => m.package), false, false));
        } catch (e) { extra = errText(e); }
      }
      for (const c of S.cols) { c.sel.clear(); c.query = ""; c.cursor = 0; }
      const after = active.from === "guide" ? ui.afterImport : null;
      ui.afterImport = null;
      await ui.reload();
      if (after) return after();
      const id = dest === "new" ? +r.page.slice(5) : active.target;
      await ui.manageLists(id);
      say("ok", (dest === "new" ? r.text : r) + (extra ? "  " + extra : ""), dest !== "new" ? async () => {
        try { await api.OpenPlan(id); const t = await api.Undo(); await ui.reload(); await ui.manageLists(id); say("ok", t); }
        catch (e) { say("fail", errText(e)); }
      } : false);
    } catch (e) {
      if (!current()) return;
      say("fail", errText(e)); busy = false; face("shake");
      btn.removeAttribute("aria-busy"); btn.classList.remove("xc-working"); spin.remove();
      for (const b of acts.querySelectorAll("button")) b.disabled = false;
    }
  }
  // a new list: its name is asked, starting from a free one (the game's list is named after its Game Config)
  const saveNew = async (btn) => {
    const free = await api.CheckPlanName(name0, "", 0).catch(() => ({ free: name0 }));
    if (!current()) return;
    pop.ask(T("new list name"), T("new list name"), async (v) => {
      const r = await api.CheckPlanName("", v, 0).catch(e => ({ problem: errText(e) }));
      if (r.problem) return say("fail", r.problem);
      perform("new", btn, v);
    }, free.free || name0);
  };
  const apply = button(Tf("apply to %s", active.name || T("your list")), "", () => perform("replace", apply), fromGame ? "primary" : "", "import-apply");
  apply.title = p.replace?.text || "";
  const merge = button(T("add only what it lacks"), "", () => perform("merge", merge));
  merge.title = p.merge?.text || "";
  merge.hidden = !!p.merge?.same;
  const asNew = button(T("save as a new list…"), "", () => saveNew(asNew), fromGame ? "" : "primary");
  asNew.title = Tf("%d mods; the current list stays as it is", p.total);
  const acts = footer("", button(T("close"), "", hideResult), merge, asNew, apply);
  acts.classList.add("xc-lacts");
  // the filled button last (16 §7.3): the game's list applies, any other list saves as new
  if (!fromGame) acts.append(asNew);
  if (opts.length) { const o = el("div", "xc-opts"); o.append(...opts); acts.prepend(o); acts.querySelector(".note")?.remove(); }
  body.append(acts);
  place();
  body.onkeydown = e => {
    if (e.key === "Enter" && !e.target.matches("button, a, select, textarea, input")) { e.preventDefault(); e.stopPropagation(); (fromGame ? apply : asNew).click(); }
  };
  reveal();
}

// checkLine: a ticked choice that rides along with the import
function checkLine(title, tip, get, set) {
  const b = el("button", "xc-ride");
  b.type = "button";
  const cb = box(get() ? "checked" : "");
  b.append(cb, el("span", "", title));
  b.title = tip;
  b.setAttribute("role", "checkbox");
  b.setAttribute("aria-checked", String(get()));
  b.onclick = () => { const v = !get(); set(v); cb.classList.toggle("on", v); b.setAttribute("aria-checked", String(v)); };
  return b;
}

// ---- the result area: a card of rows under the section that produced it ----

// showResult empties the result area for one mode, names it and marks the kind it came from
// (at: "m" for the managers, else the kind picked now).
function showResult(mode, at, label) {
  const res = $("#screen .xc-preview"), lab = $("#screen .xc-res-label");
  if (!res) return null;
  res.dataset.mode = mode; res.hidden = false; res.textContent = "";
  res.dataset.kind = at === "m" || at === "db" ? "db" : at;
  // one thing is shown at a time: a result of another row drops the managers' picks
  if (at !== "m" && at !== "db" && M && !M.run && M.shown.size) { M.shown.clear(); M.on.clear(); M.mode.clear(); drawRows(); }
  if (lab) { lab.hidden = false; labelText(lab, label); }
  const grid = $("#screen .xc-lay");
  if (grid) grid.dataset.showing = mode;
  for (const x of $("#screen")?.querySelectorAll?.(".xc-dock .rh, .xc-dock .rf") || []) x.textContent = "";
  markShown();
  placeArcho(spot().expr);
  ui?.renderHints?.();
  return res;
}
// labelText sets a section label's words and keeps its area number
function labelText(lab, text) {
  let lt = lab.querySelector(".lt");
  if (!lt) { lt = el("span", "lt"); lab.append(lt); }
  lt.textContent = text;
}
function hideResult() {
  ++previewSeq;
  const res = $("#screen .xc-preview"), lab = $("#screen .xc-res-label");
  if (res) { res.hidden = true; res.textContent = ""; delete res.dataset.mode; delete res.dataset.fieldError; delete res.dataset.kind; }
  if (lab) lab.hidden = true;
  const grid = $("#screen .xc-lay");
  if (grid) delete grid.dataset.showing;
  for (const x of $("#screen")?.querySelectorAll?.(".xc-dock .rh, .xc-dock .rf") || []) x.textContent = "";
  markShown();
  placeArcho("neutral");
  ui?.renderHints?.();
}
// place: in 3col and 2col-new the result's head (the counts) and foot (the buttons) go to the
// rail or the bar
function place() {
  const res = $("#screen .xc-preview"), rh = $("#screen .xc-dock .rh"), rf = $("#screen .xc-dock .rf");
  if (!res || !rh || !rf) return;
  const kids = [...res.children];
  for (const x of kids.filter(c => c.classList?.contains("xc-rhead"))) rh.append(x);
  for (const x of kids.filter(c => c.classList?.contains("xc-acts") || c.classList?.contains("xc-tsum"))) rf.append(x);
}
// reveal scrolls the result into view and keeps its label in view: a tall result shows from
// its top, a short one as far as needed.
function reveal() {
  const lab = $("#screen .xc-res-label"), res = $("#screen .xc-preview");
  if (!res || res.hidden) return;
  let sc = res.parentElement;
  while (sc && sc !== document.body && !(sc.scrollHeight > sc.clientHeight && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  if (!sc || sc === document.body) return;
  const view = sc.getBoundingClientRect(), top = (lab && !lab.hidden ? lab : res).getBoundingClientRect().top - 12, bottom = res.getBoundingClientRect().bottom + 12;
  let d = 0;
  if (top < view.top) d = top - view.top;
  else if (bottom > view.bottom) d = Math.min(bottom - view.bottom, top - view.top);
  if (d) sc.scrollBy({ top: d, behavior: still() ? "auto" : "smooth" });
}
// groupHead names a group of rows inside the result card
function groupHead(text) { return el("div", "group-note xc-gh", text); }
// footer: the result's actions at its foot, kept in view while the card scrolls past
function footer(note, ...acts) {
  const f = el("div", "xc-acts");
  f.append(el("span", "note", note || ""));
  for (const a of acts) if (a) f.append(a);
  return f;
}
function errorRow(name, why) {
  const r = row(name, "");
  const s = r.querySelector(".l"); s.append(el("div", "sub warn wrap", why));
  r.classList.add("xc-err");
  return r;
}
function settingsLine(pl) {
  let n = 0, failed = 0;
  for (const m of pl.mods) for (const f of m.files) { if (f.error) failed++; else if (f.write) n++; }
  if (n === 1) return failed ? Tf("Mod settings: %d file written, %d failed", n, failed) : Tf("Mod settings: %d file written", n);
  return failed ? Tf("Mod settings: %d files written, %d failed", n, failed) : Tf("Mod settings: %d files written", n);
}

// Shared by the Import page and the lists' new / import form: the source as the card's first
// row, its facts on one line, another game version in amber.
// sourceIsPath: the source is a file the player picked, so it copies on a click (16 §3.1 rule 12).
export function showImportSummary(body, p, sourceIsPath = !!p.sourcePath) {
  const top = el("div", "set xc-src"), l = el("div", "l");
  // the format, then installed / not installed as a bar named by the summary (V5)
  if (p.total && p.format) l.append(el("div", "name", p.format), splitBar(p.installed.length, p.missing.length, p.summary));
  else l.append(el("div", "name", p.summary));
  const facts = [sourceIsPath && p.source ? { text: p.source, paths: [p.source] } : p.source];
  if (p.name) facts.push(Tf("list: %s", p.name));
  if (p.game) facts.push("RimWorld " + p.game);
  if (p.allowed) facts.push(Tf("suggested game versions: %s", p.allowed));
  if (p.settings) facts.push(Tf("Mod settings of %d mods", p.settings));
  l.append(factText(el("div", "sub facts"), facts.filter(Boolean)));
  if (p.gameNote) l.append(el("div", "sub warn", p.gameNote));
  top.append(l);
  body.append(top);
}

// The Mods: those not installed first (with Workshop subscription), then the installed ones,
// folded unless some are in another version.
export function showImportContents(body, p) {
  if (!p.missing.length && !p.installed.length) return;
  body.append(groupHead(T("Mods")));
  if (p.missing.length) {
    const ids = workshopIDs(p.missing), rest = p.missing.length - ids.length;
    const note = ids.length
      ? T("subscribed mods take their place in the list once downloaded") + (rest ? "  " + Tf("%d have no Workshop page: look for them yourself", rest) : "")
      : T("none of them has a known Workshop page: look for them yourself");
    let go = null;
    if (ids.length) { go = button(Tf("subscribe all %d", ids.length), "", () => getAll(ids, go), "", "subscribe"); go.type = "button"; }
    const head = row(Tf("not installed: %d", p.missing.length), "", go);
    head.classList.add("xc-missing");
    head.querySelector(".l").append(factText(el("div", "sub wrap"), note));
    const list = el("div", "xc-list xc-mods");
    for (const r of p.missing) {
      const d = el("div", "xc-mod");
      const n = el("div", "n");
      n.append(el("span", "", r.name));
      if (r.package && r.package !== r.name) n.append(el("span", "dim", r.package));
      d.append(n);
      if (r.url) d.append(link(T("subscribe"), r.url, true));
      else d.append(el("span", "dim", T("not on the Workshop list")));
      if (r.instead) {
        const i = el("div", "xc-instead", Tf("replaced by %s", r.instead));
        if (r.insteadUrl) i.append(link(T("Workshop page"), r.insteadUrl));
        d.append(i);
      }
      list.append(d);
    }
    body.append(head, list);
  }
  if (p.installed.length) {
    // a RimArcho list names each mod's version: the ones installed in another version are
    // marked, and the list opens to show them
    const det = el("details", "xc-fold");
    const sum = el("summary", "set");
    const l = el("div", "l"), tw = el("span", "tw"); tw.innerHTML = icon.chev;
    l.append(el("div", "name", Tf("installed: %d", p.installed.length)));
    const facts = [Tf("%d enabled now", p.installed.filter((r) => r.enabled).length)];
    if (p.other) { facts.push(Tf("%d in another version", p.other)); det.open = true; }
    l.append(factText(el("div", "sub"), facts));
    const rr = el("div", "r"); rr.append(tw); sum.append(l, rr);
    det.append(sum);
    const list = el("div", "xc-list xc-mods");
    p.installed.forEach((r, i) => {
      const d = el("div", "xc-mod");
      d.append(el("span", "pos", String(i + 1)), el("span", "n" + (r.enabled ? "" : " accent"), r.name));
      if (r.other) d.append(el("span", "chip mark", T("another version")));
      if (!r.enabled) d.append(el("span", "dim", T("not enabled now")));
      list.append(d);
    });
    det.append(list);
    body.append(det);
  }
}

// workshopIDs: the Workshop items of rows that have a Workshop page.
export const workshopIDs = rows => [...new Set(rows.map(r => /[?&]id=(\d+)/.exec(r.url || "")?.[1]).filter(Boolean))];

// getAll asks how to get the Workshop items ids (Steam or SteamCMD), then gets them all as one
// background task (16 §2.16); btn stays disabled once it started. The import page's "subscribe
// all" and a followed collection's mods that are not installed share it.
export function getAll(ids, btn) {
  pop.choose(Tf("get %d mods", ids.length), getChoices(), async (i) => {
    if (btn) btn.disabled = true;
    try { say("ok", await api.Subscribe({ kind: "workshop", url: ids.join(" "), steamcmd: i === 1, batch: true })); }
    catch (e) { if (btn) btn.disabled = false; say("fail", errText(e)); }
  });
}

function link(text, url, button) {
  const a = el(button ? "button" : "a", button ? "btn small" : "xc-link", text);
  a.title = url;
  a.onclick = (e) => { e.preventDefault(); api.OpenURL(url).catch((x) => say("fail", errText(x))); };
  return a;
}
