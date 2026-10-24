// 库 › 贴图优化 (08 §5.2, 06 §2.13): the current plan's (or every mod's) PNG textures, what
// RimArcho converted and what went stale, what converting at a scale would take and save,
// then the run in the background ("textures" events), stop, and take everything back. The
// encoder (todds) is downloaded only after the player saw where from and how big.
import { pathContent } from "./fields.js";
import { factText, S, $, el, api, T, Tf, errText, say } from "./state.js";
import { beginPageRead, pageLoading, page, button } from "./screens.js";
import * as pop from "./popups.js";
import { createTable } from "./datatable.js";
import { textureColumns } from "./tabledefs.js";

const X = { scope: "plan", scale: 100, page: null, prog: null, seq: 0, loading: false, error: "" };

export async function open() {
  const read = beginPageRead("textures", { label: "reading textures…", retry: open });
  return draw(true, false, read);
}

// event is a "textures" event: the run's progress, then its end.
export function event(ev) {
  if (ev.done) {
    X.prog = null;
    say(ev.ok ? "ok" : "fail", ev.ok ? ev.text : ev.error);
    if (S.screen === "textures") draw(true);
    return;
  }
  X.prog = ev;
  if (S.screen !== "textures") { say("busy", T("converting textures") + "  " + ev.text); return; }
  $("#screen").querySelectorAll("select").forEach((s) => { s.disabled = true; });
  const f = $("#screen .tex-foot");
  if (f) f.replaceWith(slot(foot(X.page), "foot"));
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

// select: a choice that measures again; f names it, so a redraw keeps the focus on it.
function select(f, values, labels, value, onchange) {
  const s = el("select");
  s.dataset.f = f;
  values.forEach((v, i) => {
    const o = el("option", "", labels[i]);
    o.value = String(v);
    if (String(v) === String(value)) o.selected = true;
    s.append(o);
  });
  s.onchange = () => onchange(s.value);
  s.disabled = !!X.prog;
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

async function draw(fetch, measurements = false, entryRead = null) {
  if (S.screen !== "textures") return;
  if (fetch) {
    const read = entryRead || beginPageRead("textures", { label: "reading textures…", retry: open });
    const n = ++X.seq;
    const progress = X.prog;
    X.loading = true; X.error = "";
    if (X.page) X.page = { ...X.page, loading: true, rows: (X.page.rows || []).map((r) => ({ ...r, pending: true })) };
    if (!pageLoading() && !entryRead) draw(false);
    const current = () => n === X.seq && read.current();
    try {
      let p = await api.Textures(X.scope, X.scale);
      if (!current()) return;
      if (X.prog === progress) X.prog = p.running || null;
      // Both the request and the page visit must still match after every await (06 §3.1).
      while (current() && p) {
        const changed = JSON.stringify(X.page) !== JSON.stringify(p);
        X.page = p;
        X.loading = !!p.loading;
        X.error = p.error || "";
        if (changed) draw(false, true);
        if (X.error) say("fail", X.error);
        if (!p.loading) { read.finish(); return; }
        await new Promise((resolve) => setTimeout(resolve, 100));
        if (!current()) return;
        p = await api.TextureMeasure(p.request);
        if (!current()) return;
      }
    } catch (e) {
      if (!current()) return;
      X.loading = false;
      X.error = errText(e); read.fail(e);
      if (pageLoading()) return;
      if (X.page) X.page = { ...X.page, loading: false, rows: (X.page.rows || []).map((r) => ({ ...r, pending: false })) };
    }
    if (!current()) return;
    X.loading = false;
    read.finish();
  }
  const p = X.page;
  const sc = $("#screen");
  if (!p) {
    sc.innerHTML = "";
    const w = page("textures", X.error);
    w.setAttribute("aria-busy", String(X.loading));
    if (!X.error) w.append(pending(3));
    sc.append(w);
    return;
  }
  const top = sc.scrollTop;
  const focused = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.f;
  const oldPage = sc.querySelector(".textures");
  const previous = measurements && oldPage && oldPage.querySelector("[data-tex]") && oldPage;
  if (!previous) sc.innerHTML = "";
  const w = page("textures", "");
  if (!previous) sc.append(w);
  w.setAttribute("aria-busy", String(X.loading));
  const error = el("div", "muted", X.error);
  error.hidden = !X.error;
  w.append(slot(error, "error"));

  // the encoder
  w.append(el("div", "label section", T("encoder")));
  const enc = el("section", "card settings");
  if (!p.tool) setRow(enc, T("the DDS encoder"), null, X.error ? null : pending());
  else {
    setRow(enc, T("the DDS encoder"), p.toolOk ? null : [p.offer, p.offerTo && { text: "→ " + p.offerTo, paths: [p.offerTo] }], p.toolOk ? factText(el("span", "muted"), { text: p.tool, paths: [p.toolPath] }) : null,
      p.toolOk ? null : button(T("download todds…"), "", () => install(p), "primary"));
    if (!p.toolOk) enc.querySelector(".name").after(pathContent(el("div", "sub warn"), p.tool, [p.toolPath]));
  }
  w.append(slot(enc, "encoder"));

  // what is there
  w.append(el("div", "label section", T("what is there")));
  const now = el("section", "card settings");
  const mods = setRow(now, T("mods"), Tf("%d mods", p.mods),
    select("scope", ["plan", "all"], [T("the current plan"), T("every installed mod")], X.scope, (v) => { X.scope = v; draw(true); }));
  const modCount = mods.querySelector(".sub");
  modCount.dataset.texMods = "";
  fillModCount(modCount, p);
  slot(setRow(now, "PNG", p.loading ? null : p.png, p.loading ? pending() : null), "png");
  const ours = slot(setRow(now, T("RimArcho's DDS"), [p.ours, p.staleText], button(T("remove every converted texture…"), "", () => clean(), "danger", "textures-clean")), "ours");
  ours.hidden = !p.ours || !!p.loading;
  w.append(now);

  // what converting does
  w.append(el("div", "label section", T("what converting does")));
  const est = el("section", "card settings");
  setRow(est, T("scale"), T("scaling shrinks them further; never below 128 px, masks stay full size"),
    select("scale", p.scales, p.labels, X.scale, (v) => { X.scale = Number(v); draw(true); }));
  const facts = [...(p.estFacts || [])];
  if (p.upToDate) facts.push(Tf("%d up to date", p.upToDate));
  if (p.leftAlone) facts.push(Tf("%d left alone (a DDS of the mod's own, or another format of the same name)", p.leftAlone));
  slot(setRow(est, T("convert"), p.loading ? null : facts, p.loading ? pending() : null), "estimate");
  setRow(est, "", T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back"));
  w.append(est);
  w.append(slot(foot(p), "foot"));

  if (previous) textureRows(p, previous.querySelector('[data-tex="rows"]'));
  else w.append(slot(textureRows(p), "rows"));
  if (previous) {
    // Keep live select elements in place while estimates arrive (06 §3.1).
    previous.setAttribute("aria-busy", String(X.loading));
    for (const next of w.querySelectorAll("[data-tex]")) updateMeasurement(previous.querySelector(`[data-tex="${next.dataset.tex}"]`), next);
    fillModCount(previous.querySelector("[data-tex-mods]"), p);
    return;
  }
  sc.scrollTop = top;
  if (focused) { const n = sc.querySelector(`[data-f="${focused}"]`); if (n) n.focus(); }
}

const textureTable = createTable({ key: "ui.gui.textures.columns", columns: textureColumns,
  state: { cols: null, sort: "estimate", dir: 1 }, redraw: () => draw(false, true),
});
let columnsLoaded = false;
function textureRows(p, old = null) {
  if (!columnsLoaded) {
    columnsLoaded = true;
    textureTable.load().then(() => { if (S.screen === "textures" && X.page) draw(false, true); });
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

function fillModCount(node, p) {
  if (p.loading && p.rows == null && node.querySelector(".tex-pending")) return;
  const unknown = p.loading && p.rows == null;
  node.textContent = unknown ? "" : Tf("%d mods", p.mods);
  if (unknown) node.append(pending());
}

// foot: the run's progress and stop, or the convert button.
function foot(p) {
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
  const go = button(T("convert"), "", () => convert(p), "primary", "textures-run");
  go.disabled = X.loading || !!p.loading || !!X.error || !p.toolOk || !p.convert;
  f.append(go);
  return f;
}

function convert(p) {
  const lines = [...(p.estFacts || []), T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back")];
  pop.confirm(T("convert textures to DDS"), lines, T("convert"), async () => {
    try { say("ok", await api.TexturesRun(X.scope, X.scale, [])); } catch (e) { say("fail", errText(e)); return; }
    X.prog = { text: T("converting textures"), frac: 0 };
    draw(false);
  });
}

function install(p) {
  pop.confirm(T("download todds…"), [p.offer, "→ " + p.offerTo], T("download todds…"), async () => {
    say("busy", T("download todds…"));
    try { say("ok", await api.TexturesInstall()); } catch (e) { say("fail", errText(e)); }
    draw(true);
  });
}

function clean() {
  pop.confirm(T("remove every converted texture…"), [T("the DDS RimArcho wrote are deleted; the PNGs stay and the game uses them again")], T("remove every converted texture…"), async () => {
    try { say("ok", await api.TexturesClean(X.scope)); } catch (e) { say("fail", errText(e)); }
    draw(true);
  });
}
