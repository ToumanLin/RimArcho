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
import { still } from "./motion.js";
import { splitBar, diffMarks, stateMark, visTip } from "./membar.js";
import { archoSpot } from "./archo.js";

let ui = null;
export function initExchange(u) {
  ui = u;
  wireDrop();
}

let info = null;
let firstRun = false;
let pasted = "";
let session = null;
let previewSeq = 0;
// kind: the import kind picked in the page's first column (16 §2.18); kept while the window lives
let kind = "";

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
  // ↑↓ in the kinds column move the kind, as a vertical tab list does
  if ((e.key === "ArrowDown" || e.key === "ArrowUp") && e.target?.closest?.(".xc-kinds")) {
    e.preventDefault();
    const ids = KINDS().map(x => x.id), i = ids.indexOf(kind);
    pickKind(ids[(i + (e.key === "ArrowDown" ? 1 : ids.length - 1)) % ids.length], true);
  }
}
// tab and enter only mean something while a list preview is shown
export const hintList = () => [...($("#screen .xc-preview")?.dataset.mode === "list" ? [{ key: "tab", label: T("import destination") }, { key: "enter", label: T("import") }] : []), { key: "esc", label: T("cancel") }, { key: "?", label: T("all keys") }];

// layout: the page's arrangement while the user compares the variants (design note
// 2026-11-20_import-page): "2col" (kinds | source over preview) or "3col" (kinds | source |
// preview). <html data-import-layout> or ?import-layout= picks it.
export function layout() {
  const v = document.documentElement?.dataset?.importLayout || new URLSearchParams(globalThis.location?.search || "").get("import-layout") || "";
  return v === "3col" ? "3col" : "2col";
}

// KINDS: what can be imported, one entry each in the first column: an icon and a short name;
// what it takes is its tooltip (16 §7 principle 9: visuals first, the words on hover).
const KINDS = () => [
  { id: "game", icon: "home", name: T("game's list"), tip: T("the game's current list (ModsConfig.xml), compared with yours") },
  { id: "file", icon: "plan", name: T("list file"), tip: T("RimSort, RimCrow and RimPy lists, RimWorld .rml, ModsConfig.xml or a save (.rws)") },
  { id: "paste", icon: "ext", name: T("link or code"), tip: T("a Steam collection or Rentry link, a share code, or a copied list") },
  { id: "pack", icon: "packs", name: T("pack"), tip: T("a pack you can launch; its Mods are checked first") },
  { id: "managers", icon: "import", name: T("other managers"), tip: T("lists, notes and tags, sorting rules and, from RimCrow, Game Configs; the files are only read") },
];

// The Import page (16 §2.18): the sidebar's Import, `i`, a dropped file, Settings and the guide
// all open it. The first column lists the kinds of import; the selected kind's source (a
// button, a drop area, a field, the managers found) and the preview of what it brings, compared
// with what the player has now, fill the rest. Moving in and adding a pack run as background
// tasks shown in the preview (16 §2.16).
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
  const lay = layout();
  // esc leaves (the hint line says so); only the first run names its way out, "skip"
  const w = page("plans pm-import xc-page xc-" + lay, firstRun ? T("Where does your first list come from?") : T("What do you want to bring in?"),
    firstRun ? button(T("skip"), "esc", cancel) : null);
  const grid = el("div", "xc-lay"); grid.dataset.layout = lay;
  const side = el("div", "xc-side"), nav = el("div", "xc-kinds");
  nav.setAttribute("role", "tablist"); nav.setAttribute("aria-orientation", "vertical"); nav.setAttribute("aria-label", T("what to import"));
  side.append(nav);
  const [srcLab, srcCard] = sectionCard("");
  const srcBlk = block(srcLab, srcCard, "xc-srcblk");
  // the result area (a preview, the tree, a task); empty, it shows where a drop goes
  const resLab = el("div", "label section xc-res-label");
  const result = el("section", "card settings xc-sec xc-preview xc-res"); result.hidden = true;
  const resBlk = el("div", "xc-blk xc-resblk");
  resBlk.append(resLab, result, emptyState());
  grid.append(side, srcBlk, resBlk);
  w.append(grid);
  sc.append(w);
  area(nav, null, () => $("#screen .xc-kind.on")?.focus({ preventScroll: true })); area(srcCard, srcLab); area(result, resLab);
  labelText(resLab, T("Preview"));
  M = freshM();
  if (firstRun || !kind) kind = firstRun || info?.game ? "game" : "file";
  drawKinds();
  drawSource();
  // Archo stands at the foot of the kinds column, in its blank space (16 §7.1 "Placements")
  spot().stand(side, "neutral", true);
  read.finish(); ui.renderWhere(); ui.renderHints();
  if (!firstRun) survey();
}

const live = (active) => session === active && S.screen === "plan-import";

// Archo stands under the kinds (16 §7.1 "Placements"): thinking while something is read or
// moved in, pleased with a preview, a thumbs-up when it is in, a shake at an error or a refusal.
const spot = () => archoSpot("import", { size: 176, calm: ["neutral", "ponder", "think", "smirk", "yum"] });
// nothing read or running is the calm state: her idle faces follow only there
const face = (e) => spot().set(e, e === "neutral");

function sectionCard(title) {
  return [el("div", "label section", title), el("section", "card settings xc-sec")];
}
// block: a section's label and card as one cell of the page's grid
function block(lab, card, cls = "") {
  const b = el("div", "xc-blk" + (cls ? " " + cls : ""));
  b.append(lab, card);
  return b;
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

// ---- the kinds column ----

// drawKinds draws the kinds: an icon, a short name and, where there is one, a count (the game
// list's Mods, the managers found; a dot when a manager has something new).
function drawKinds() {
  const nav = $("#screen .xc-kinds");
  if (!nav) return;
  nav.textContent = "";
  const res = $("#screen .xc-preview");
  for (const k of KINDS()) {
    const b = el("button", "xc-kind" + (k.id === kind ? " on" : ""));
    b.dataset.kind = k.id;
    b.setAttribute("role", "tab");
    b.setAttribute("aria-selected", String(k.id === kind));
    b.tabIndex = k.id === kind ? 0 : -1;
    b.title = k.tip;
    const ic = el("span", "ki"); ic.innerHTML = icon[k.icon] || "";
    b.append(ic, el("span", "kn", k.name));
    const n = kindCount(k.id);
    if (n) { const c = el("span", "kc num", n.text); if (n.tip) c.title = n.tip; if (n.news) c.classList.add("news"); b.append(c); }
    // the kind the preview came from carries a mark while the player looks at another one
    if (res && !res.hidden && res.dataset.kind === k.id && k.id !== kind) b.append(stateMark("dot", T("shown in the preview")));
    b.onclick = () => pickKind(k.id, false);
    nav.append(b);
  }
}
function kindCount(id) {
  if (id === "game") return info?.game ? { text: String(info.game.mods), tip: Tf("%d mods", info.game.mods) } : null;
  if (id === "managers") {
    if (!M?.managers?.length) return null;
    const fresh = M.managers.filter(m => m.imported && news(m).length).length;
    return { text: String(M.managers.length), tip: M.managers.map(m => m.name).join(", "), news: fresh > 0 };
  }
  return null;
}
// pickKind shows a kind's source. The game's list is read at once (one click to its preview);
// the others wait for their file, text or manager.
function pickKind(id, focus) {
  if (!KINDS().some(k => k.id === id)) return;
  const again = id === kind;
  kind = id;
  drawKinds(); drawSource();
  if (focus) $(`#screen .xc-kind[data-kind="${id}"]`)?.focus();
  if (id === "game" && !again && !firstRun && info?.game && $("#screen .xc-preview")?.dataset.kind !== "game") preview("file", info.game.path);
  if (id === "managers") drawTree();
}

// ---- the source: the selected kind's controls ----

function drawSource() {
  const card = $("#screen .xc-srcblk .xc-sec"), lab = $("#screen .xc-srcblk .label");
  if (!card) return;
  const k = KINDS().find(x => x.id === kind);
  labelText(lab, k.name);
  card.textContent = "";
  card.dataset.src = kind === "managers" ? "m" : "in";
  card.dataset.kind = kind;
  if (kind === "game") card.append(gameRow());
  else if (kind === "file") card.append(dropArea("file"));
  else if (kind === "paste") card.append(pasteBox());
  else if (kind === "pack") card.append(dropArea("pack"));
  else { managerRows(card); drawRows(); }
}

// The game's current list: its ModsConfig.xml in the current Game Config. On the first run
// it is the list the scan already made, so "use this list" ends the guide; later it is
// previewed like any other list.
function gameRow() {
  const g = info?.game;
  const use = firstRun
    ? button(T("use this list"), "", () => { const after = ui.afterImport; ui.afterImport = null; firstRun = false; return after?.(T("use the game's list as it is")); }, "primary")
    : button(T("read it"), "", () => g && preview("file", g.path));
  use.disabled = !g && !firstRun;
  // the file's name is not a word to learn: the row says how many Mods, the path is on hover
  const r = row(T("The game's current list"), g ? Tf("%d mods", g.mods) : T("the game has no ModsConfig.xml yet"), use);
  if (g) r.title = use.title = g.path;
  if (firstRun) r.classList.add("on");
  return r;
}

// EXT: what each drop area takes, as chips; the sentence is the area's tooltip
const EXT = { file: [".xml", ".rml", ".json", ".txt", ".rws"], pack: [".archopack"] };

// dropArea: a list file or a pack, as a dashed area with one button. A file can be dropped on
// it, or anywhere on the window (wireDrop); a click on the area opens the dialog too.
function dropArea(which) {
  const fileInput = el("input"); fileInput.type = "file"; fileInput.hidden = true;
  fileInput.accept = EXT.file.join(",");
  fileInput.onchange = () => { if (fileInput.files[0]) readFile(fileInput.files[0]); };
  const pick = which === "pack"
    ? button(T("choose a pack…"), "", async () => {
      let f = "";
      try { f = await api.PickPackFile(); } catch (e) { return say("fail", errText(e)); }
      if (f && S.screen === "plan-import") importPack(f);
    })
    : button(T("choose a file…"), "", async () => {
      let f = "";
      try { f = await api.PickAnyImport(); } catch (e) { return say("fail", errText(e)); }
      if (f && S.screen === "plan-import") openPicked(f);
      else if (!info.dialogs) fileInput.click();
    });
  const zone = el("div", "xc-dz");
  zone.dataset.drop = which;
  zone.title = (which === "pack" ? ".archopack" : T("RimSort, RimCrow and RimPy lists, RimWorld .rml, ModsConfig.xml or a save (.rws)")) + ". " + T("a file can also be dropped anywhere on the window");
  const ic = el("span", "dz-i"); ic.innerHTML = icon[which === "pack" ? "packs" : "import"];
  const chips = el("span", "dz-x");
  for (const x of EXT[which]) chips.append(el("span", "chip", x));
  zone.append(ic, el("span", "dz-t", T("drop it here")), chips, pick, fileInput);
  zone.onclick = e => { if (!e.target.closest("button")) pick.click(); };
  if (which === "file") {
    const result = () => $("#screen .xc-preview");
    fieldError(pick, result(), () => result()?.dataset.fieldError === "file");
  }
  return zone;
}

// pasteBox: a share code, a Rentry or collection link, or a copied list
function pasteBox() {
  const ta = el("textarea", "field xc-paste");
  ta.spellcheck = false; ta.rows = 3;
  ta.placeholder = T("a link, a share code or a list");
  ta.setAttribute("aria-label", T("a Steam collection or Rentry link, a share code, or a copied list"));
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
  const box = el("div", "xc-pastebox");
  const acts = el("div", "xc-pacts");
  acts.append(clip, go);
  box.append(ta, acts);
  return box;
}

// emptyState: the preview before anything is read: where a drop goes, as a picture
function emptyState() {
  const e = el("div", "xc-empty");
  const ic = el("span", "dz-i"); ic.innerHTML = icon.import;
  e.append(ic, el("span", "dz-t", T("drop a list, a pack or a manager's file anywhere")));
  e.title = T("the preview shows what comes in next to what you have: new, already there, another version, not installed");
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
  for (const k of KINDS().filter(k => k.id === "file" || k.id === "pack" || k.id === "managers")) {
    const s = el("span", "ki"); s.innerHTML = icon[k.icon]; s.title = k.name; kinds.append(s);
  }
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
// with its kind picked. A form that takes drops itself (the lists' new / import) handles them
// first. Under WebView2 composition hosting the native window receives no drops yet (16 §8.2);
// the browser harness does.
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
  pickKind(k === "list" ? "file" : k === "pack" ? "pack" : "managers", false);
  if (d.path) return k === "folder" ? addManagerSource(d.path) : openPicked(d.path);
  if (k === "list") return readFile(d.file);
  // the browser gives no path: the dialog does
  return refuse(d.name, k === "folder" ? T("choose the folder with “another folder or file…”") : T("choose this file with “choose a file…”"), "warn");
}

// refuse says why a drop is not imported, in the preview, and keeps the page as it was
function refuse(name, why = "", cls = "") {
  const res = showResult("refused", "", T("Not imported"));
  if (!res) return;
  const r = errorRow(name, why || T("RimArcho imports lists (.xml, .rml, .json, .txt, .rws), packs (.archopack) and RimSort, RimCrow or RimPy data"));
  if (cls) r.classList.add(cls);
  // a refusal reads at a glance: the file's name after a stop mark (an ask for the dialog: a warning)
  const mk = el("span", "xc-refuse" + (cls ? " " + cls : "")); mk.innerHTML = cls ? icon.warn : icon.err;
  r.querySelector(".name").prepend(mk);
  res.append(r, footer("", button(T("close"), "", hideResult)));
  face("shake");
  say("fail", (why ? "" : T("not imported") + ": ") + (why || name));
}

// ---- everything from another manager ----

// M is the moving-in state of this page: what the survey found, the managers picked above,
// the parts ticked below and the run in progress.
let M = null;
function freshM() { return { managers: null, error: "", shown: new Set(), on: new Set(), mode: new Map(), open: new Set(), extra: M?.extra || [], run: null }; }

// managerRows: one row per manager found (drawRows fills them in), then "another folder or
// file…" as the section's last row.
function managerRows(card) {
  const more = button(T("another folder or file…"), "", async () => {
    let f = "";
    try { f = await api.PickManagerImport(); } catch (e) { return say("fail", errText(e)); }
    if (!f || S.screen !== "plan-import") return;
    addManagerSource(f);
  });
  const elsewhere = row(T("Somewhere else"), "", more);
  elsewhere.title = T("a data folder or file kept elsewhere; it is only read");
  card.append(el("div", "xc-mrows"), elsewhere);
}

// addManagerSource adds a manager's folder or file kept elsewhere (the dialog, or a dropped one)
async function addManagerSource(f) {
  const k = await api.ManagerFileKind(f).catch(() => "list");
  M.extra.push({ manager: k === "list" ? T("a file") : "", kind: k, path: f });
  return survey();
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
  drawRows(); drawTree(); drawKinds();
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
    b.append(box(st), mark(m.name, 24), t, s);
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
  if (/\.archopack$/i.test(path)) { pickKind("pack", false); return importPack(path); }
  let kind = "list";
  try { kind = await api.ManagerFileKind(path); } catch { /* read it as a list */ }
  if (!live(active)) return;
  if (kind === "list") return preview("file", path);
  M.extra.push({ manager: "", kind, path });
  if (S.screen === "plan-import" && live(active)) pickKind("managers", false);
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
  const result = showResult("pack", "in", T("Adding a pack"));
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

// ---- import, step 2: the preview ----
// The preview is a card of rows under the section it came from (16 §2.18): the source on top,
// the Mods (not installed, installed), where it goes, the Mod settings, and a footer with the
// page's one filled button.

async function preview(kind, src, fileName) {
  const body = $("#screen .xc-preview");
  const active = session, seq = ++previewSeq;
  const current = () => S.screen === "plan-import" && session === active && S.screenSeq === active.epoch && seq === previewSeq;
  if (!body) return;
  delete body.dataset.fieldError;
  const link = kind === "text" && /^\s*\S+\s*$/.test(src) && /steamcommunity|rentry|^\s*\d{6,}\s*$/.test(src);
  say("busy", link ? T("reading the link…") : T("reading the list…"));
  let p;
  showResult("list", "in", T("Preview"));
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
    body.append(errorRow(T("could not read it"), errText(e)));
    face("shake");
    say("fail", errText(e)); reveal(); return;
  }
  if (!current()) return;
  say("", "");
  if (fileName) p.source = fileName;
  body.textContent = "";

  face("smug");
  showImportSummary(body, p, !!(p.sourcePath || fileName));
  body.append(compare(p, active.name));
  showImportContents(body, p);
  // the game's own list, or any list, that the list already is: nothing to import (16 §2.18)
  const fromGame = kind === "file" && !!info?.game && src === info.game.path;
  if (p.replace?.same && !p.settings && !p.collection) {
    body.append(row(fromGame ? Tf("%s already matches the game.", active.name) : Tf("%s already is this list.", active.name), ""));
    body.append(footer("", button(T("close"), "", hideResult)));
    reveal();
    return;
  }

  // A destination is selected first; only the final import button changes data (16 §2.17).
  let destination = "new", busy = false;
  const name = el("input", "field xc-name"); name.spellcheck = false;
  name.setAttribute("aria-label", T("new list name"));
  // the game's list is named after its Game Config, not after its file (ModsConfig)
  const gameName = fromGame ? (S.board?.environmentRows || []).find(e => e.current)?.name : "";
  const base = gameName || p.name || (p.source || "").replace(/^.*[\\/]/, "").replace(/\.[^.]*$/, "");
  const free = await api.CheckPlanName(base, "", 0).catch(() => ({ free: base }));
  if (!current()) return;
  name.value = free.free;
  const why = el("div", "sub warn plan-why"); fieldError(name, why);
  const submit = button(T("import"), "enter", perform, "primary", "import-apply");
  submit.title = T("does not write game files");
  body.append(groupHead(T("import destination")));
  const ways = el("div", "xc-dests"); ways.setAttribute("role", "radiogroup"); ways.setAttribute("aria-label", T("import destination"));
  const options = [["new", T("import as a new list"), Tf("%d mods; the current list stays as it is", p.total)],
    ["replace", Tf("replace %s", active.name), p.replace], ["merge", T("add the ones you lack"), p.merge]];
  const choices = [];
  for (const [value, title, text] of options) {
    // a destination's effect as + − ↕ marks, named by its sentence (V5); nothing to change stays words
    const marks = typeof text === "object" && !text.same;
    const r = choiceRow("radio", title, typeof text === "object" ? (marks ? "" : text.text) : text, value === destination, () => {
      destination = value;
      for (const c of choices) c.set(c.value === value);
      name.disabled = value !== "new"; checkName();
    });
    r.value = value; choices.push(r);
    if (marks) r.el.querySelector(".l").append(diffMarks(text));
    if (value === "new") {
      const lab = el("label", "xc-namef"); lab.append(el("span", "", T("new list name")), name);
      lab.onclick = e => e.stopPropagation();
      r.el.querySelector(".r").append(lab);
      r.el.querySelector(".l").append(why);
    }
    ways.append(r.el);
  }
  body.append(ways);
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
  let follow = true;
  // a Steam Workshop collection: the list can follow it (07 §7.6); ticked by default
  if (p.collection) {
    const c = choiceRow("checkbox", T("follow this collection from now on"), T("RimArcho tells you when it changes; catching up is one undoable step"), true, () => { follow = !follow; c.set(follow); });
    ways.append(c.el);
  }
  // a RimArcho list's Mod settings (07 §7.3): offered, applied after the import
  const settings = p.settings ? settingsChoice(body, current) : null;
  async function perform() {
    if (busy || !(await checkName()) || !current()) return;
    busy = true; submit.disabled = true;
    // the import runs: the button says so and Archo thinks until it is in
    submit.setAttribute("aria-busy", "true"); submit.classList.add("xc-working");
    const spin = el("span", "icon spin"); spin.innerHTML = icon.rescan; submit.prepend(spin);
    say("busy", T("importing…")); face("think");
    try {
      await api.ImportFollow(follow);
      const r = destination === "new" ? await api.ImportAsPlan(name.value) : await api.ImportApplyTo(active.target, destination === "merge");
      if (!current()) return;
      let extra = "";
      if (settings && settings.on()) {
        try { extra = settingsLine(await api.ImportSettings(settings.mods(), settings.replace(), false)); }
        catch (e) { extra = errText(e); }
      }
      for (const c of S.cols) { c.sel.clear(); c.query = ""; c.cursor = 0; }
      const after = active.from === "guide" ? ui.afterImport : null;
      ui.afterImport = null;
      await ui.reload();
      if (after) return after();
      const id = destination === "new" ? +r.page.slice(5) : active.target;
      await ui.manageLists(id);
      say("ok", (destination === "new" ? r.text : r) + (extra ? "  " + extra : ""), destination !== "new" ? async () => {
        try { await api.OpenPlan(id); const t = await api.Undo(); await ui.reload(); await ui.manageLists(id); say("ok", t); }
        catch (e) { say("fail", errText(e)); }
      } : false);
    } catch (e) { if (current()) { say("fail", errText(e)); busy = false; submit.removeAttribute("aria-busy"); submit.classList.remove("xc-working"); spin.remove(); face("shake"); checkName(); } }
  }
  checkName();
  body.append(footer("", button(T("close"), "", hideResult), submit));
  body.onkeydown = e => {
    if (e.key === "Enter" && !e.target.matches("button, a, select, textarea")) { e.preventDefault(); e.stopPropagation(); perform(); }
  };
  reveal();
}

// compare: what the list brings next to what the player has now (16 §2.18 "Preview"), as one
// bar of four kinds with an icon and a count each, then what it does to the current list as
// + − ↕ ⇄ marks. Each part is named by its words (V5); the words are on hover and for agents.
export function compareParts(p) {
  const inst = p.installed || [];
  const other = inst.filter(r => r.other).length;
  const inList = inst.filter(r => r.enabled && !r.other).length;
  return [
    { k: "have", n: inList, word: Tf("%d already in your list", inList) },
    { k: "new", n: inst.length - inList - other, word: Tf("%d installed, not in your list", inst.length - inList - other) },
    { k: "other", n: other, word: Tf("%d in another version", other) },
    { k: "miss", n: (p.missing || []).length, word: Tf("%d not installed", (p.missing || []).length) },
  ];
}
const SIGN = { have: "ok", new: "plus", other: "vers", miss: "close" };
function compare(p, name) {
  const parts = compareParts(p), r = el("div", "set xc-cmp"), l = el("div", "l");
  const words = parts.filter(x => x.n).map(x => x.word).join(", ");
  const bar = el("span", "xc-cbar");
  visTip(bar, words, "img", false);
  for (const x of parts) if (x.n) { const i = el("i", "c-" + x.k); i.style.flexGrow = String(x.n); bar.append(i); }
  const counts = el("span", "ccounts");
  for (const x of parts) {
    const c = el("span", "cn c-" + x.k + (x.n ? "" : " zero"));
    visTip(c, x.word, "img", false);
    const ic = el("span", "ci"); ic.innerHTML = icon[SIGN[x.k]] || "";
    c.append(ic, el("span", "num", String(x.n)));
    counts.append(c);
  }
  l.append(bar, counts);
  r.append(l);
  // against the current list: the replace destination's effect, the list named on hover
  if (p.replace && !p.replace.same && name) {
    const rr = el("div", "r xc-vs");
    const lb = el("span", "vs-n", name);
    lb.title = Tf("compared with %s", name);
    rr.append(lb, diffMarks(p.replace));
    r.append(rr);
  }
  return r;
}

// ---- the result area: a card of rows under the section that produced it ----

// showResult empties the result area for one mode, names it and marks the kind it came from
// (at: "m" for the managers, else the kind picked now).
function showResult(mode, at, label) {
  const res = $("#screen .xc-preview"), lab = $("#screen .xc-res-label");
  if (!res) return null;
  res.dataset.mode = mode; res.hidden = false; res.textContent = "";
  res.dataset.kind = at === "m" ? "managers" : kind;
  if (lab) labelText(lab, label);
  drawKinds();
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
  if (lab) labelText(lab, T("Preview"));
  drawKinds();
  face("neutral");
  ui?.renderHints?.();
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
// choiceRow: a row picked with a click, with a radio or a check box drawn like the managers'
// (16 §2.18); text is facts parted by two spaces.
function choiceRow(kind, title, text, on, pick) {
  const r = el("div", "set xc-choice"), l = el("div", "l"), rr = el("div", "r");
  const b = kind === "radio" ? el("button", "xc-rb") : box(on ? "checked" : "", "button");
  b.setAttribute("role", kind);
  const t = el("div", "name");
  const sub = text ? factText(el("div", "sub"), text) : null;
  l.append(t); t.append(title); if (sub) l.append(sub);
  r.append(b, l, rr);
  const set = v => {
    r.classList.toggle("on", v);
    b.setAttribute("aria-checked", v ? "true" : "false");
    if (kind === "radio") b.classList.toggle("on", v); else b.classList.toggle("on", v);
  };
  set(on);
  r.onclick = e => { if (e.target.closest(".r input, .r .btn")) return; pick(); };
  b.onclick = e => { e.stopPropagation(); pick(); };
  return { el: r, set };
}

// settingsChoice is the import's "apply settings" (07 §7.3): off at first; on, it shows the
// per-Mod preview (what is written, kept or replaced) with a box per Mod.
function settingsChoice(body, current) {
  body.append(groupHead(T("Mod settings")));
  const off = new Set();
  let on = false, replace = false;
  const apply = choiceRow("checkbox", T("apply settings"), T("after the import; settings files you have are kept unless you replace them"), false, () => {
    on = !on; apply.set(on); rep.el.hidden = list.hidden = !on; if (on) draw();
  });
  apply.el.classList.add("xc-apply");
  const rep = choiceRow("checkbox", T("replace settings files I have"), T("the old ones go to the recycle bin"), false, () => { replace = !replace; rep.set(replace); draw(); });
  const list = el("div", "xc-list xc-mods");
  rep.el.hidden = list.hidden = true;
  let all = [];
  async function draw() {
    let pl;
    try { pl = await api.ImportSettings([], replace, true); } catch (e) { list.textContent = errText(e); return; }
    if (!current()) return;
    all = pl.mods;
    list.textContent = "";
    for (const m of pl.mods) {
      const d = el("div", "xc-mod" + (m.missing ? " dis" : ""));
      const mc = box(!m.missing && !off.has(m.package) ? "checked" : "", "button");
      mc.disabled = !!m.missing;
      const flip = () => {
        if (m.missing) return;
        if (off.has(m.package)) off.delete(m.package); else off.add(m.package);
        const v = !off.has(m.package);
        mc.classList.toggle("on", v); mc.setAttribute("aria-checked", v ? "true" : "false");
      };
      mc.onclick = e => { e.stopPropagation(); flip(); };
      d.onclick = flip;
      d.append(mc, el("span", "n", m.mod), el("span", "dim", settingsNote(m)));
      list.append(d);
    }
  }
  body.append(apply.el, rep.el, list);
  return {
    on: () => on && all.some(m => !m.missing && !off.has(m.package)),
    mods: () => all.filter(m => !m.missing && !off.has(m.package)).map(m => m.package),
    replace: () => replace,
  };
}

// settingsNote says what applying does to one Mod's files.
function settingsNote(m) {
  if (m.missing) return T("not installed");
  let n = 0, kept = 0, same = 0, repl = 0;
  for (const f of m.files) {
    if (f.same) same++;
    else if (f.write && f.exists) repl++;
    else if (f.write) n++;
    else kept++;
  }
  return [n && Tf("%d new", n), repl && Tf("%d replaced", repl), kept && Tf("%d kept", kept), same && Tf("%d the same", same)].filter(Boolean).join(", ");
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
