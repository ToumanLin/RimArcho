import { choiceControl } from "./choices.js";
// Library > Texture optimisation (08 §5.2, 06 §2.13): the current plan's (or every mod's) PNG textures, what
// RimArcho converted and what went stale, what converting at a scale would take and save,
// then the run in the background ("textures" events), stop, and take everything back. The
// encoder (todds) is downloaded only after the player saw where from and how big.
// Reading every PNG takes a while, so the page opens with only the encoder and the mods:
// count the textures reads them, and changing a choice never does (the last count stays, marked old).
import { pathContent } from "./fields.js";
import { factText, S, $, el, api, T, Tf, errText, say } from "./state.js";
import { beginPageRead, pageLoading, page, button } from "./screens.js";
import * as pop from "./popups.js";
import { createTable } from "./datatable.js";
import { textureColumns } from "./tabledefs.js";

// info: the encoder and the mods (no texture read); page: the last count, at: what it was
// counted with; old: the textures changed since (a run, a clean).
const X = { scope: "plan", scale: 100, info: null, page: null, at: null, old: false, prog: null, seq: 0, infoSeq: 0, loading: false, error: "" };

const context = () => JSON.stringify([S.board?.env, S.shell?.plan?.id]);
// counted: the count on screen is for the choices on screen.
const counted = () => !!(X.page && X.at && X.at.scope === X.scope && X.at.scale === X.scale && !X.old);

export async function open() {
  const read = beginPageRead("textures", { label: "loading…", retry: open });
  // A count of another game or list, or one cut short by leaving, is no count.
  if (X.at && (X.at.ctx !== context() || X.page?.loading)) { X.page = null; X.at = null; X.old = false; }
  X.loading = false;
  return readInfo(read);
}

// readInfo asks for the encoder and the scope's mods: quick, no texture is read.
async function readInfo(entryRead = null) {
  const read = entryRead || beginPageRead("textures", { keep: true, label: "loading…", retry: open });
  const n = ++X.infoSeq;
  const progress = X.prog;
  try {
    const p = await api.TexturesInfo(X.scope, X.scale);
    if (!read.current() || n !== X.infoSeq) return;
    X.info = p;
    if (X.prog === progress) X.prog = p.running || null;
    if (!X.loading) X.error = "";
  } catch (e) {
    if (!read.current() || n !== X.infoSeq) return;
    X.error = errText(e); read.fail(e);
    if (pageLoading()) return;
  }
  read.finish();
  draw();
}

// event is a "textures" event: the run's progress, then its end.
export function event(ev) {
  if (ev.done) {
    X.prog = null;
    if (X.page) X.old = true;
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    if (S.screen === "textures") readInfo();
    return;
  }
  X.prog = ev;
  if (S.screen !== "textures") { say("busy", T("converting textures") + "  " + ev.text); return; }
  $("#screen").querySelectorAll(".choice-control").forEach((s) => { s.disabled = true; });
  const f = $("#screen .tex-foot");
  if (f && X.info) f.replaceWith(slot(foot(X.info, X.page), "foot"));
}

function setRow(card, name, sub, ...right) {
  const row = el("div", "set");
  const l = el("div", "l");
  l.append(el("div", "name", name));
  if (sub) for (const s of [].concat(sub)) if (s) l.append(el("div", "sub word-facts", s));
  const r = el("div", "r");
  for (const x of right) if (x) r.append(x);
  row.append(l, r);
  card.append(row);
  return row;
}

// f names the choice so a redraw keeps focus on it.
function select(f, values, labels, value, onchange) {
  const s = choiceControl(T(f === "scope" ? "mods" : "scale"), values.map((v, i) => ({ value: v, label: labels[i] })), value);
  s.dataset.f = f;
  s.onchange = () => onchange(s.value);
  s.disabled = !!X.prog || X.loading;
  return s;
}

// Pending values use the shared shimmer, with text for screen readers (06 §7.1).
function pending(lines = 1) {
  const s = el("span", "skel tex-pending");
  s.setAttribute("role", "img");
  s.setAttribute("aria-label", T("reading textures…"));
  for (let i = 0; i < lines; i++) s.append(el("span", "bar"));
  return s;
}

// count reads every PNG of the scope (count the textures): rows fill in as mods finish; a later
// count, a stop or leaving the page drops this one (06 §3.1).
async function count() {
  if (S.screen !== "textures" || X.loading) return;
  const read = X.countRead = beginPageRead("textures", { keep: true, label: "reading textures…", retry: open });
  const n = ++X.seq;
  const before = X.before = { page: X.page, at: X.at, old: X.old };
  const at = { scope: X.scope, scale: X.scale, ctx: context() };
  X.loading = true; X.error = "";
  if (X.page) X.page = { ...X.page, loading: true, rows: (X.page.rows || []).map((r) => ({ ...r, pending: true })) };
  draw();
  const current = () => n === X.seq && read.current();
  try {
    let p = await api.Textures(at.scope, at.scale);
    if (!current()) return;
    // Both the request and the page visit must still match after every await (06 §3.1).
    while (current() && p) {
      const changed = JSON.stringify(X.page) !== JSON.stringify(p);
      X.page = p; X.at = at; X.old = false;
      X.loading = !!p.loading;
      X.error = p.error || "";
      if (changed) draw(true);
      if (X.error) { say("fail", X.error); break; }
      if (!p.loading) break;
      await new Promise((resolve) => setTimeout(resolve, 100));
      if (!current()) return;
      p = await api.TextureMeasure(p.request);
      if (!current()) return;
    }
  } catch (e) {
    if (!current()) return;
    X.error = errText(e); say("fail", X.error);
    Object.assign(X, before);
  }
  if (!current()) return;
  if (X.error) { const e = X.error; Object.assign(X, before); X.error = e; }
  X.loading = false;
  read.finish();
  draw();
}

// stopCount drops the count going on; the last finished one comes back.
function stopCount() {
  ++X.seq;
  api.Leave("textures").catch(() => {});
  Object.assign(X, X.before || {}, { loading: false });
  X.countRead?.finish();
  draw();
}

function draw(measurements = false) {
  if (S.screen !== "textures") return;
  const I = X.info, p = X.page;
  const sc = $("#screen");
  if (!I) {
    sc.innerHTML = "";
    const w = page("textures", X.error);
    w.setAttribute("aria-busy", String(X.loading));
    if (!X.error) w.append(pending(3));
    sc.append(w);
    return;
  }
  const busy = X.loading || !!p?.loading;
  const top = sc.scrollTop;
  const focused = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.f;
  const oldPage = sc.querySelector(".textures");
  const previous = measurements && oldPage && oldPage.querySelector("[data-tex]") && oldPage;
  if (!previous) sc.innerHTML = "";
  const w = page("textures", "");
  if (!previous) sc.append(w);
  w.setAttribute("aria-busy", String(busy));
  const error = el("div", "muted", X.error);
  error.hidden = !X.error;
  w.append(slot(error, "error"));

  // the encoder
  w.append(el("div", "label section", T("encoder")));
  const enc = el("section", "card settings");
  setRow(enc, T("the DDS encoder"), I.toolOk ? null : [I.offer, I.offerTo && { text: "→ " + I.offerTo, paths: [I.offerTo] }], I.toolOk ? factText(el("span", "muted"), { text: I.tool, paths: [I.toolPath] }) : null,
    I.toolOk ? null : button(T("download todds…"), "", () => install(I), "primary"));
  if (!I.toolOk) enc.querySelector(".name").after(pathContent(el("div", "sub warn"), I.tool, [I.toolPath]));
  w.append(slot(enc, "encoder"));

  // what is there
  w.append(el("div", "label section", T("what is there")));
  const now = el("section", "card settings");
  setRow(now, T("mods"), Tf("%d mods", I.mods),
    select("scope", ["plan", "all"], [T("the current plan"), T("every installed mod")], X.scope, (v) => { X.scope = v; readInfo(); }));
  const png = slot(setRow(now, "PNG", p && !p.loading ? p.png : null, p?.loading ? pending() : null), "png");
  png.hidden = !p;
  const ours = slot(setRow(now, T("RimArcho's DDS"), p ? [p.ours, p.staleText] : null, button(T("remove every converted texture…"), "", () => clean(), "danger", "textures-clean")), "ours");
  ours.hidden = !p || !p.ours || !!p.loading;
  w.append(now);

  // what converting does
  w.append(el("div", "label section", T("what converting does")));
  const est = el("section", "card settings");
  setRow(est, T("scale"), T("scaling shrinks them further; never below 128 px, masks stay full size"),
    select("scale", I.scales, I.labels, X.scale, (v) => { X.scale = Number(v); draw(); }));
  const facts = [...(p?.estFacts || [])];
  if (p?.upToDate) facts.push(Tf("%d up to date", p.upToDate));
  if (p?.leftAlone) facts.push(Tf("%d left alone (a DDS of the mod's own, or another format of the same name)", p.leftAlone));
  const estimate = slot(setRow(est, T("convert"), p && !p.loading ? facts : null, p?.loading ? pending() : null), "estimate");
  estimate.hidden = !p;
  setRow(est, "", T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back"));
  w.append(est);
  w.append(slot(foot(I, p), "foot"));

  const rows = p || { rows: [] };
  if (previous) textureRows(rows, previous.querySelector('[data-tex="rows"]'));
  else w.append(slot(textureRows(rows), "rows"));
  if (previous) {
    // Keep live choice controls in place while estimates arrive (06 §3.1).
    previous.setAttribute("aria-busy", String(busy));
    for (const next of w.querySelectorAll("[data-tex]")) updateMeasurement(previous.querySelector(`[data-tex="${next.dataset.tex}"]`), next);
    return;
  }
  sc.scrollTop = top;
  if (focused) { const n = sc.querySelector(`[data-f="${focused}"]`); if (n) n.focus(); }
}

const textureTable = createTable({ key: "ui.gui.textures.columns", columns: textureColumns,
  state: { cols: null, sort: "estimate", dir: 1 }, redraw: () => draw(true),
});
let columnsLoaded = false;
function textureRows(p, old = null) {
  if (!columnsLoaded) {
    columnsLoaded = true;
    textureTable.load().then(() => { if (S.screen === "textures" && X.info) draw(true); });
  }
  const t = old || el("section", "card data-table tex-rows");
  t.tabIndex = 0;
  t.setAttribute("role", "table");
  t.hidden = !(p.rows || []).length;
  const head = el("div", "lib-th");
  textureTable.draw(t, head);
  // Keep the pointer capture target during updates (06 §3.9).
  if (old && old.children[0].dataset.columns === head.dataset.columns) textureTable.adopt(t, old.children[0]);
  else if (old) old.children[0].replaceWith(head);
  else t.append(head);
  const keep = new Map([...t.children].slice(1).map(row => [row.dataset.ref, row]));
  const stamp = JSON.stringify(textureTable.visible().map(c => [c.key, T(textureTable.column(c.key).title)]));
  const nodes = [];
  for (const r of textureTable.sorted(p.rows || [])) {
    const signature = stamp + JSON.stringify(r);
    const was = keep.get(r.ref);
    if (was && was.texSignature === signature) { nodes.push(was); continue; }
    const tr = el("div", "data-row" + (r.stale ? " stale" : ""));
    tr.setAttribute("role", "row");
    tr.dataset.ref = r.ref;
    tr.texSignature = signature;
    for (const c of textureTable.visible()) {
      if (c.key !== "name" && r.pending) { tr.append(pending()); continue; }
      const cell = el("span", "cell" + (c.key === "name" ? " n" : "") + (textureTable.column(c.key).num ? " num" : ""));
      if (c.key === "state") {
        factText(cell, [r.stale ? Tf("%d stale", r.stale) : "", r.convert ? Tf("%d to convert", r.convert) : !r.stale ? T(r.dds ? "up to date" : "nothing to convert") : "",
          r.dds ? T("RimArcho's DDS") + ": " + r.dds : "", r.own ? Tf("%d own DDS", r.own) : ""].filter(Boolean));
        if (r.stale) cell.classList.add("warn");
      } else factText(cell, String(r[c.key] ?? "—"));
      cell.title = cell.textContent;
      tr.append(cell);
    }
    nodes.push(tr);
  }
  // Reuse untouched cells and their shimmer; sorting moves only existing rows (06 §2.13).
  nodes.forEach((row, i) => { if (t.children[i + 1] !== row) t.insertBefore(row, t.children[i + 1] || null); });
  while (t.children.length > nodes.length + 1) t.lastChild.remove();
  return t;
}
export function key(e) { return textureTable.key(e); }

function slot(node, key) { node.dataset.tex = key; return node; }

// Preserve unchanged placeholders so their shimmer can finish (06 §7.1).
function updateMeasurement(old, next) {
  if (!old.isEqualNode(next)) { next.scrollLeft = old.scrollLeft; old.replaceWith(next); }
}

// foot: the run's progress and stop; else what the count on screen is, count the textures (or its
// stop) and the convert button.
function foot(I, p) {
  const f = el("div", "tex-foot");
  if (X.prog) {
    const w = el("div", "prog");
    const bar = el("div", "bar");
    const fill = el("i");
    fill.style.width = Math.round((X.prog.frac || 0) * 100) + "%";
    bar.append(fill);
    w.append(bar, el("div", "ptext word-facts", X.prog.text || T("converting textures")));
    f.append(w, button(T("stop"), "", async () => {
      try { say("ok", await api.TexturesStop()); } catch (e) { say("fail", errText(e)); }
    }));
    return f;
  }
  let note = "";
  if (!X.loading) {
    if (!p) note = T("not counted yet: “count the textures” reads every PNG to show what is there and what converting saves");
    else if (X.old) note = T("the textures changed since this count; “count the textures” counts again");
    else if (!counted()) note = T("the choices changed since this count; “count the textures” counts again");
  }
  f.append(el("div", "muted word-facts tex-note", note));
  const scan = X.loading
    ? button(T("stop counting"), "", () => stopCount())
    : button(T("count the textures"), "", () => count(), counted() ? "" : "primary");
  const go = button(T("convert"), "", () => convert(), counted() ? "primary" : "", "textures-run");
  // Without a count the run still converts only what is new or changed.
  go.disabled = X.loading || !!X.error || !I.toolOk || (counted() && !p.convert);
  f.append(scan, go);
  return f;
}

function convert() {
  const lines = counted() ? [...(X.page.estFacts || [])] : [T("not counted yet: only new or changed PNGs are converted, those already done are skipped")];
  lines.push(T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back"));
  pop.confirm(T("convert textures to DDS"), lines, T("convert"), async () => {
    try { say("ok", await api.TexturesRun(X.scope, X.scale, [])); } catch (e) { say("fail", errText(e)); return; }
    X.prog = { text: T("converting textures"), frac: 0 };
    draw();
  });
}

function install(p) {
  pop.confirm(T("download todds…"), [p.offer, "→ " + p.offerTo], T("download todds…"), async () => {
    say("busy", T("download todds…"));
    try { say("ok", await api.TexturesInstall()); } catch (e) { say("fail", errText(e)); }
    readInfo();
  });
}

function clean() {
  pop.confirm(T("remove every converted texture…"), [T("the DDS RimArcho wrote are deleted; the PNGs stay and the game uses them again")], T("remove every converted texture…"), async () => {
    try { say("ok", await api.TexturesClean(X.scope)); } catch (e) { say("fail", errText(e)); }
    if (X.page) X.old = true;
    readInfo();
  });
}
