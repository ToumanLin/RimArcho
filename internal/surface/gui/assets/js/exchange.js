// Import and export of the mod list (07 §7): to and from RimSort, RimCrow, RimWorld's
// own .rml, share codes, Rentry and Workshop collections. An import is previewed first —
// what is installed, what is missing and where to get it, what the enabled list becomes —
// and applied as one undo step; nothing here writes ModsConfig.xml (that stays save).
import { fieldError } from "./feedbacksemantics.js";
import { factText, S, $, el, api, T, Tf, errText, say } from "./state.js";
import { page, button, beginPageRead, area } from "./screens.js";
import { icon } from "./icons.js";
import * as pop from "./popups.js";
import { getChoices } from "./credit.js";
import { choiceControl } from "./choices.js";
import { copyablePath, editablePath } from "./fields.js";

let ui = null;
export function initExchange(u) {
  ui = u;
  // File drops anywhere in the window enter the same import flow (16 §2.17).
  document.addEventListener("dragover", e => { if (e.dataTransfer?.types.includes("Files")) e.preventDefault(); });
  document.addEventListener("drop", async e => {
    if (e.defaultPrevented) return;
    const f = e.dataTransfer?.files?.[0];
    if (!f) return;
    e.preventDefault();
    if (S.screen !== "plan-import") await exchange();
    readFile(f);
  });
}

let info = null;
let firstRun = false;
let pasted = "";
let session = null;
let previewSeq = 0;

// Import is a sub-page, including when entered from the first-run guide (16 §2.17).
export async function importFrom(kind, src) {
  await exchange();
  if (S.screen === "plan-import") return preview(kind, src);
}

export function cancel() {
  ++previewSeq;
  const from = session?.from || "plans";
  // on the first run, leaving the page ends the guide: the scan made the game's list the first one
  if (firstRun && from === "guide" && ui.afterImport) { const after = ui.afterImport; ui.afterImport = null; firstRun = false; return after(""); }
  ui.afterImport = null;
  return from === "guide" ? ui.resumeGuide() : ui.go(from);
}
export function key(e) { if (e.key === "Escape") { e.preventDefault(); cancel(); } }
export const hintList = () => [{ key: "tab", label: T("import destination") }, { key: "enter", label: T("import") }, { key: "esc", label: T("cancel") }, { key: "?", label: T("all keys") }];

// The Import page (16 §2.18): the sidebar's Import, `i`, a dropped file, Settings and the guide
// all open it. It is drawn like Game Config and Settings: numbered sections, each a card of
// rows with a name and one line on the left and its controls on the right. "Bring in" has a
// row per kind of source (the game's list, a list file, pasted text, a pack); "From another
// manager" has a row per manager found, picked with a click, and the tree below to choose its
// parts; "Share" exports. Moving in and adding a pack run as background tasks shown in place
// (16 §2.16).
// first: the first-run guide hands over here after its scan (A1, 16 §2.15): the game's
// current list comes first and is picked, and "skip" ends the guide.
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
  // esc leaves (the hint line says so); only the first run names its way out, "skip"
  const w = page("plans pm-import", firstRun ? T("Where does your first list come from?") : T("What do you want to bring in?"),
    firstRun ? button(T("skip"), "esc", cancel) : null);
  const result = el("section", "card pad xc-preview"); result.hidden = true;
  const [inLab, inCard] = sectionCard(T("Bring in"));
  const [mLab, mCard] = sectionCard(T("From another manager"));
  w.append(inLab, inCard, mLab, mCard, result);
  area(inCard, inLab); area(mCard, mLab);
  // the export side (C1): share what you have, below bringing things in
  if (!firstRun) { const [sLab, sCard] = sectionCard(T("Share")); shareRows(sCard); w.append(sLab, sCard); area(sCard, sLab); }
  sc.append(w);
  // the rows are filled in on the page: the file row's error watches the result area
  inCard.append(gameRow(), ...listRows(), packRow());
  managerRows(mCard);
  read.finish(); ui.renderWhere(); ui.renderHints();
  survey();
}

const live = (active) => session === active && S.screen === "plan-import";

// section: a numbered label over a card of rows, as on Game Config and Settings
function sectionCard(title) {
  return [el("div", "label section", title), el("section", "card settings xc-sec")];
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

// The game's current list: its ModsConfig.xml in the current Game Config. On the first run
// it is the list the scan already made, so "use this list" ends the guide; later it is
// previewed like any other list.
function gameRow() {
  const g = info?.game;
  const use = firstRun
    ? button(T("use this list"), "", () => { const after = ui.afterImport; ui.afterImport = null; firstRun = false; return after?.(T("use the game's list as it is")); }, "primary")
    : button(T("read it"), "", () => g && preview("file", g.path));
  if (g) use.title = g.path;
  use.disabled = !g && !firstRun;
  const r = row(T("The game's current list"), g ? Tf("its ModsConfig.xml enables %d Mods", g.mods) : T("the game has no ModsConfig.xml yet"), use);
  if (firstRun) r.classList.add("on");
  return r;
}

// A Mod list: a file (or one dropped anywhere on the window), or pasted text: a share code,
// a Rentry or collection link, a copied list. What the file input accepts is its tooltip.
function listRows() {
  const fileInput = el("input"); fileInput.type = "file"; fileInput.hidden = true;
  fileInput.accept = ".xml,.rml,.json,.txt,.rws";
  fileInput.onchange = () => { if (fileInput.files[0]) readFile(fileInput.files[0]); };
  const pick = button(T("choose a file…"), "", async () => {
    let f = "";
    try { f = await api.PickAnyImport(); } catch (e) { return say("fail", errText(e)); }
    if (f && S.screen === "plan-import") openPicked(f);
    else if (!info.dialogs) fileInput.click();
  });
  pick.title = T("RimSort, RimCrow and RimPy lists, RimWorld .rml, ModsConfig.xml or a save (.rws)") + ". " + T("a file can also be dropped anywhere on the window");
  const result = () => $("#screen .xc-preview");
  fieldError(pick, result(), () => result()?.dataset.fieldError === "file");
  // one line tall; a copied list keeps its lines (it is a textarea)
  const ta = el("textarea", "field xc-paste");
  ta.spellcheck = false; ta.rows = 1;
  ta.placeholder = T("a link, a share code or a list");
  ta.value = pasted;
  ta.oninput = () => { pasted = ta.value; };
  ta.onkeydown = e => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); e.stopPropagation(); go.click(); } };
  const clip = button(T("paste"), "", async () => {
    let t = "";
    try { t = await api.Clipboard(); } catch { /* the page asks the browser */ }
    if (!t && navigator.clipboard) { try { t = await navigator.clipboard.readText(); } catch { /* not allowed */ } }
    if (!t) return say("", T("the clipboard has no text"));
    ta.value = pasted = t;
  });
  clip.title = T("paste from the clipboard");
  const go = button(T("read"), "", () => ta.value.trim() && preview("text", ta.value));
  go.title = T("links are read from Steam or Rentry when you press read");
  const file = row(T("A Mod list"), T("a new list, or changes to one of yours; previewed first"), pick, fileInput);
  const text = row(T("Pasted text"), T("a Steam collection or Rentry link, a share code, or a copied list"), ta, clip, go);
  text.classList.add("xc-textrow");
  return [file, text];
}

// A pack: an .archopack file, added as a background task.
function packRow() {
  const b = button(T("choose a pack…"), "", async () => {
    let f = "";
    try { f = await api.PickPackFile(); } catch (e) { return say("fail", errText(e)); }
    if (f && S.screen === "plan-import") importPack(f);
  });
  b.title = ".archopack";
  return row(T("A pack"), T("a pack you can launch; its Mods are checked first"), b);
}

// ---- share: the current list in its formats, and packs (C1) ----
// Each export is its own row builder, so export options (what a pack carries, revisions)
// can be added to one without touching the others.

function shareRows(card) {
  card.append(listExportRow(), packExportRow());
}
function listExportRow() {
  const id = S.shell?.plan?.id || 0, name = S.shell?.plans?.find(p => p.id === id)?.name || "";
  const ex = button(name ? Tf("export %s…", name) : T("export…"), "", () => id && ui.exportList(id, "import"));
  ex.disabled = !id;
  return row(T("The current list"), T("RimSort, RimCrow, RimPy, .rml, a share code or Workshop links"), ex);
}
function packExportRow() {
  const r = row(T("A pack"), T("the pack as a file (.archopack), or only its list"));
  const right = r.querySelector(".r"), sub = r.querySelector(".sub");
  api.Packs().then(l => {
    const names = (l?.rows || []).map(x => x.name);
    if (!names.length) { sub.textContent = T("no packs yet"); return; }
    let pick = names[0];
    const sel = choiceControl(T("pack"), names.map(n => ({ value: n, label: n })), pick);
    sel.onchange = () => { pick = sel.value; };
    const one = (list) => async () => {
      try { const t = await api.PackExport(pick, "", list); if (t) say("ok", t); }
      catch {
        pop.ask(list ? T("export the list") : T("export the pack"), T("the new file") + ": " + pick + (list ? ".archolist.json" : ".archopack"), async (file) => {
          if (!file) return;
          try { say("ok", await api.PackExport(pick, file, list)); } catch (e2) { say("fail", errText(e2)); }
        });
      }
    };
    right.append(sel, button(T("export the list"), "", one(true)), button(T("export the pack"), "", one(false)));
  }).catch(e => { sub.textContent = errText(e); });
  return r;
}

// ---- everything from another manager ----

// M is the moving-in state of this page: what the survey found, the managers picked above,
// the parts ticked below and the run in progress.
let M = null;
function freshM() { return { managers: null, error: "", shown: new Set(), on: new Set(), mode: new Map(), open: new Set(), extra: M?.extra || [], run: null }; }

// managerRows: one row per manager found (drawRows fills them in), then "another folder or
// file…" as the section's last row.
function managerRows(card) {
  M = freshM();
  const more = button(T("another folder or file…"), "", async () => {
    let f = "";
    try { f = await api.PickManagerImport(); } catch (e) { return say("fail", errText(e)); }
    if (!f || S.screen !== "plan-import") return;
    const kind = await api.ManagerFileKind(f).catch(() => "list");
    M.extra.push({ manager: kind === "list" ? T("a file") : "", kind, path: f });
    survey();
  });
  card.append(el("div", "xc-mrows"), row(T("Somewhere else"), T("a data folder or file kept elsewhere; it is only read"), more));
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
    const b = el("button", "xc-mrow" + (on ? " on" : "") + (m.imported && !nws.length && !on ? " seen" : ""));
    b.setAttribute("role", "checkbox");
    b.setAttribute("aria-checked", st === "checked" ? "true" : st === "mixed" ? "mixed" : "false");
    const t = el("span", "t", m.name);
    if (m.imported) t.append(el("span", "when", Tf("imported %s", dayText(m.imported))));
    const s = el("span", "s");
    if (running) s.append(el("span", "pick", runLine()));
    else if (st) s.append(el("span", "pick", Tf("bringing over %s", pickLine(m))));
    else if (on) s.append(ps.some(p => p.state === "listed") ? T("nothing new to choose; tick a list below to import it again") : T("all of it is already in RimArcho"));
    else if (m.imported && nws.length) { const n = el("span", "nline"); n.title = nws.join(", "); n.append(el("span", "chip-n", T("new")), el("span", "nt", nws.join(", "))); s.append(n); }
    else if (m.imported) s.append(T("nothing new since then"));
    else s.append(holds(m));
    b.append(box(st), mark(m.name, 24), t, s);
    if (running && M.run.state === "running") b.append(progBar(runPct()));
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
  if (M.shown.size) $("#screen .xc-preview")?.scrollIntoView({ behavior: "smooth", block: "nearest" });
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
function groupLabel(g) {
  const label = GROUP()[g.kind];
  if (g.kind === "player" && /aux_metadata\.db$/i.test(g.path || "")) return label + "  " + (g.path.split(/[\\/]/).slice(-2, -1)[0] || "");
  return label;
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
  if (!list.length) { if (res.dataset.mode === "move") { res.hidden = true; res.textContent = ""; delete res.dataset.mode; } return; }
  res.dataset.mode = "move"; res.hidden = false; res.textContent = "";
  res.classList.remove("pad");
  const bar = el("div", "xc-stepbar"); bar.innerHTML = icon.import;
  bar.append(el("span", "", T("Everything from another manager")), el("b", "", Tf("what comes over from %s", list.map(m => m.name).join(", "))));
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
      const row = treeRow(1, M.open.has(gk), gp.length ? gk : "", gst, !toggleable(gp).length && !gp.some(p => M.on.has(p.id)), ic, groupLabel(g), "", [], right, () => toggle(gp, gst));
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
  res.append(bar, tree, sum, acts);
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
  return row;
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
  const res = $("#screen .xc-preview"), r = M.run;
  if (!res || !r) return;
  res.dataset.mode = "move"; res.hidden = false; res.textContent = ""; res.classList.remove("pad");
  const items = r.task?.items || r.picks.map(p => ({ ref: p.id, state: "wait" }));
  const fails = items.filter(i => i.state === "fail");
  const done = items.filter(i => i.state === "ok").length;
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
    row.append(si, el("span", "lb", p.name), el("span", "rs", text || ""));
    pg.append(row);
  });
  const f = el("div", "xc-run-f");
  if (r.state === "running") {
    f.append(el("span", "sum", T("Running as a background task. You can leave this page; it goes on.")),
      button(T("background tasks"), "", () => ui.go("tasks")),
      button(T("stop"), "", () => api.TaskAct("stop", [r.id], "").catch(e => say("fail", errText(e)))));
  } else if (r.state === "stopped") {
    const rest = r.picks.filter((_, i) => items[i]?.state !== "ok" && items[i]?.state !== "fail");
    f.append(el("span", "sum", Tf("Finished parts are kept. %d not done.", rest.length)),
      button(T("choose again"), "", again), button(Tf("continue %d", rest.length), "", () => startRun(rest), "primary"));
  } else {
    const failed = r.picks.filter((_, i) => items[i]?.state === "fail");
    f.append(el("span", "sum", (failed.length ? Tf("%d failed; the rest is in RimArcho.", failed.length) : T("Everything is in RimArcho.")) + " " + T("Notes and tags show in the Mod library; lists under lists; Game Configs in the switcher.")));
    if (failed.length) f.append(button(Tf("retry %d", failed.length), "", () => startRun(failed)));
    if (r.picks.some(p => p.kind === "lists")) f.append(button(T("open lists"), "", () => ui.go("plans")));
    f.append(button(T("done"), "", again, "primary"));
  }
  res.append(head, pg, f);
}

// again closes the run: the page asks the managers again, so what came in shows as imported
function again() {
  const after = session?.from === "guide" && M.run?.state === "finished" ? ui.afterImport : null;
  M.run = null; M.on.clear(); M.mode.clear(); M.shown.clear();
  const res = $("#screen .xc-preview");
  if (res) { res.hidden = true; res.textContent = ""; delete res.dataset.mode; res.classList.add("pad"); }
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

// importPack adds a pack as a background task and shows it in place.
async function importPack(path) {
  const active = session, result = $("#screen .xc-preview"), seq = ++previewSeq;
  result.dataset.mode = "pack"; result.hidden = false; result.textContent = ""; result.classList.remove("pad");
  let id;
  try { id = await api.StartPackImport(path); } catch (e) { result.classList.add("pad"); result.textContent = errText(e); return say("fail", errText(e)); }
  say("", T("adding the pack: a background task; you can leave this page"));
  const name = path.replace(/^.*[\\/]/, "");
  const tick = async () => {
    if (!live(active) || seq !== previewSeq) return;
    let t = null;
    try { t = await api.TaskByID(id); } catch { /* try again */ }
    if (!live(active) || seq !== previewSeq) return;
    const running = !t || t.state === "run" || t.state === "queue";
    result.textContent = "";
    const head = el("div", "xc-run-h"), h = el("span");
    h.append(el("span", "t", name), " ", el("span", "n", running ? T("adding the pack…") : t.state === "done" ? T("added to your packs") : T("not added")));
    head.append(h, el("span"), progBar(running ? 0 : 100, running));
    const pg = el("div", "xc-pg");
    if (!running) pg.append(el("pre", "xc-out", t.result || ""));
    const f = el("div", "xc-run-f");
    if (running) f.append(el("span", "sum", T("Running as a background task. You can leave this page; it goes on.")), button(T("background tasks"), "", () => ui.go("tasks")));
    else if (t.state === "done") f.append(el("span", "sum", T("The pack is ready to launch.")), button(T("open packs"), "enter", () => ui.go("packs"), "primary"));
    else f.append(el("span", "sum", ""), button(T("choose another pack…"), "", async () => { const p = await api.PickPackFile().catch(() => ""); if (p) importPack(p); }));
    result.append(head, pg, f);
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

// ---- import, step 2: the preview ----

async function preview(kind, src, fileName) {
  const body = $("#screen .xc-preview");
  const active = session, seq = ++previewSeq;
  const current = () => S.screen === "plan-import" && session === active && S.screenSeq === active.epoch && seq === previewSeq;
  if (!body) return;
  delete body.dataset.fieldError;
  say("busy", kind === "text" && /^\s*\S+\s*$/.test(src) && /steamcommunity|rentry|^\s*\d{6,}\s*$/.test(src) ? T("reading the link…") : T("reading the list…"));
  let p;
  body.dataset.mode = "list"; body.classList.add("pad");
  body.hidden = false; body.textContent = T("reading the list…");
  try { p = await api.ImportPreviewFor(active.target, kind, src); }
  catch (e) { if (current()) { body.dataset.fieldError = fileName || kind === "file" ? "file" : "text"; body.textContent = errText(e); say("fail", errText(e)); } return; }
  if (!current()) return;
  say("", "");
  if (fileName) p.source = fileName;
  body.innerHTML = "";

  showImportSummary(body, p, !!(p.sourcePath || fileName));

  // A destination is selected first; only the final import button changes data (16 §2.17).
  const ways = el("fieldset", "xc-sec");
  ways.append(el("legend", "xc-sec-t", T("import destination")));
  let destination = "new", busy = false;
  const name = el("input", "xc-input"); name.spellcheck = false;
  const base = p.name || (p.source || "").replace(/^.*[\\/]/, "").replace(/\.[^.]*$/, "");
  const free = await api.CheckPlanName(base, "", 0).catch(() => ({ free: base }));
  if (!current()) return;
  name.value = free.free;
  const why = el("div", "plan-why"); fieldError(name, why);
  const submit = button(T("import"), "enter", perform, "primary", "import-apply");
  submit.title = T("does not write game files");
  const options = [["new", T("import as a new list"), Tf("%d mods; the current list stays as it is", p.total)],
    ["replace", Tf("replace %s", active.name), p.replace.text], ["merge", T("add the ones you lack"), p.merge.text]];
  for (const [value, title, text] of options) {
    const row = el("label", "xc-check");
    const radio = el("input"); radio.type = "radio"; radio.name = "destination"; radio.checked = value === destination;
    radio.onchange = () => { destination = value; name.disabled = value !== "new"; checkName(); };
    row.append(radio, el("span", "", title), el("span", "xc-note", text)); ways.append(row);
  }
  let nameSeq = 0;
  async function checkName() {
    const n = ++nameSeq;
    if (destination !== "new") { why.textContent = ""; submit.disabled = busy; return true; }
    submit.disabled = true;
    try {
      const r = await api.CheckPlanName("", name.value, 0);
      if (!current() || n !== nameSeq) return false;
      why.textContent = r.problem; submit.disabled = busy || !!r.problem; return !r.problem;
    } catch (e) { if (current()) why.textContent = errText(e); return false; }
  }
  name.oninput = checkName;
  name.onkeydown = e => { if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); perform(); } };
  ways.append(name, why); body.append(ways);
  let follow = true;
  async function perform() {
    if (busy || !(await checkName()) || !current()) return;
    busy = true; submit.disabled = true;
    try {
      await api.ImportFollow(follow);
      const r = destination === "new" ? await api.ImportAsPlan(name.value) : await api.ImportApplyTo(active.target, destination === "merge");
      if (!current()) return;
      for (const c of S.cols) { c.sel.clear(); c.query = ""; c.cursor = 0; }
      const after = active.from === "guide" ? ui.afterImport : null;
      ui.afterImport = null;
      await ui.reload();
      if (after) return after();
      const id = destination === "new" ? +r.page.slice(5) : active.target;
      await ui.manageLists(id);
      say("ok", destination === "new" ? r.text : r, destination !== "new" ? async () => {
        try { await api.OpenPlan(id); const t = await api.Undo(); await ui.reload(); await ui.manageLists(id); say("ok", t); }
        catch (e) { say("fail", errText(e)); }
      } : false);
    } catch (e) { if (current()) { say("fail", errText(e)); busy = false; checkName(); } }
  }
  checkName();
  // a Steam Workshop collection: the list can follow it (07 §7.6); ticked by default
  if (p.collection) {
    const lb = el("label", "xc-check");
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = true;
    cb.onchange = () => { follow = cb.checked; };
    lb.append(cb, el("span", "", T("follow this collection from now on")), el("span", "xc-hint", T("RimArcho tells you when it changes; catching up is one undoable step")));
    body.append(lb);
  }
  body.append(submit);
  body.onkeydown = e => {
    if (e.key === "Enter" && !e.target.matches("button, a, select")) { e.preventDefault(); e.stopPropagation(); perform(); }
  };

  showImportContents(body, p);
}

// Shared by the standalone destination flow and the new-list form's inline source.
// sourceIsPath: the source is a file the player picked, so it copies on a click (16 §3.1 rule 12).
export function showImportSummary(body, p, sourceIsPath = !!p.sourcePath) {
  const top = el("div", "xc-sum");
  top.append(el("div", "xc-big", p.summary));
  const facts = [sourceIsPath && p.source ? { text: p.source, paths: [p.source] } : p.source];
  if (p.name) facts.push(Tf("list: %s", p.name));
  if (p.game) facts.push("RimWorld " + p.game);
  if (p.allowed) facts.push(Tf("suggested game versions: %s", p.allowed));
  top.append(factText(el("div", "xc-note"), facts.filter(Boolean)));
  if (p.gameNote) top.append(el("div", "xc-note warn", p.gameNote));
  body.append(top);
}

export function showImportContents(body, p) {
  if (p.missing.length) {
    const sec = section(body, Tf("not installed: %d", p.missing.length));
    subscribeAll(sec, p.missing);
    const list = el("div", "xc-list");
    for (const r of p.missing) {
      const d = el("div", "xc-mod");
      const n = el("div", "n");
      n.append(el("span", "", r.name));
      if (r.package && r.package !== r.name) n.append(el("span", "dim", r.package));
      d.append(n);
      if (r.instead) {
        const i = el("div", "xc-instead", Tf("replaced by %s", r.instead));
        if (r.insteadUrl) i.append(link(T("Workshop page"), r.insteadUrl));
        d.append(i);
      }
      if (r.url) d.append(link(T("subscribe"), r.url, true));
      else d.append(el("span", "dim", T("not on the Workshop list")));
      list.append(d);
    }
    sec.append(list);
  }
  if (p.installed.length) {
    // a RimArcho list names each mod's version: the ones installed in another version are
    // marked, and the list opens to show them
    const det = el("details", "xc-sec");
    let sum = Tf("installed: %d (%d enabled now)", p.installed.length, p.installed.filter((r) => r.enabled).length);
    if (p.other) { sum += "  " + Tf("%d in another version", p.other); det.open = true; }
    det.append(el("summary", "word-facts", sum));
    const list = el("div", "xc-list");
    p.installed.forEach((r, i) => {
      const d = el("div", "xc-mod");
      d.append(el("span", "pos", String(i + 1)), el("span", r.enabled ? "" : "accent", r.name));
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

// subscribeAll gets every missing mod that has a Workshop page in one go (a background task,
// 16 §2.16); once downloaded they take their place in the list by themselves.
function subscribeAll(sec, rows) {
  const ids = workshopIDs(rows);
  const bar = el("div", "xc-subscribe");
  const rest = rows.length - ids.length;
  bar.append(el("span", "xc-note", ids.length
    ? T("subscribed mods take their place in the list once downloaded") + (rest ? "  " + Tf("%d have no Workshop page: look for them yourself", rest) : "")
    : T("none of them has a known Workshop page: look for them yourself")));
  if (ids.length) {
    const go = button(Tf("subscribe all %d", ids.length), "", () => getAll(ids, go), "primary small", "subscribe");
    go.type = "button";
    bar.append(go);
  }
  sec.append(bar);
}

function link(text, url, button) {
  const a = el(button ? "button" : "a", button ? "btn small" : "xc-link", text);
  a.title = url;
  a.onclick = (e) => { e.preventDefault(); api.OpenURL(url).catch((x) => say("fail", errText(x))); };
  return a;
}

function section(body, title) {
  const s = el("div", "xc-sec");
  s.append(el("div", "xc-sec-t", title)); body.append(s); return s;
}
