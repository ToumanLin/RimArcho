// Quantities drawn, with their sentence underneath (16 §7 principle 9). A visual is an
// element with role img or meter whose accessible name is the full sentence the view model
// keeps; the same sentence is its tooltip on hover and on keyboard focus (visTip).
import { el } from "./state.js";

// ---- the shared tooltip: hover and focus show the sentence a visual stands for ----

let tipBox = null, tipFor = null, tipTimer = 0;

function tipShow(node) {
  clearTimeout(tipTimer);
  const text = node.dataset.tip;
  if (!text || !node.isConnected) return;
  if (!tipBox) {
    tipBox = el("div", "vis-tip");
    tipBox.setAttribute("aria-hidden", "true"); // the element's own name already says it
  }
  if (!tipBox.isConnected) document.body.append(tipBox);
  tipFor = node;
  tipBox.textContent = text;
  const a = node.getBoundingClientRect();
  const w = tipBox.offsetWidth, h = tipBox.offsetHeight;
  let x = Math.min(a.left, innerWidth - w - 8), y = a.bottom + 6;
  if (x < 8) x = 8;
  if (y + h > innerHeight - 8) y = Math.max(8, a.top - h - 6);
  tipBox.style.left = x + "px";
  tipBox.style.top = y + "px";
}

function tipHide(node) {
  clearTimeout(tipTimer);
  if (node && tipFor !== node) return;
  tipFor = null;
  tipBox?.remove();
}

// visTip names node by text for assistive technology and shows text as its tooltip on hover
// (after a short pause, like a title) and at once on keyboard focus. focusable: the visual
// carries a number that is not printed beside it, so the keyboard must reach it (§7.1 of the
// UI reduction design: "the visual is focusable when it carries a number").
export function visTip(node, text, role = "img", focusable = true) {
  node.setAttribute("role", role);
  node.setAttribute("aria-label", text);
  node.dataset.tip = text;
  if (focusable) node.tabIndex = 0;
  node.addEventListener("pointerenter", () => { clearTimeout(tipTimer); tipTimer = setTimeout(() => tipShow(node), 350); });
  node.addEventListener("pointerleave", () => tipHide(node));
  node.addEventListener("focus", () => { if (node.matches(":focus-visible")) tipShow(node); });
  node.addEventListener("blur", () => tipHide(node));
  node.addEventListener("keydown", (e) => { if (e.key === "Escape" && tipFor === node) { tipHide(node); e.stopPropagation(); } });
  return node;
}

// A page change or scroll takes the tooltip away with the element it was for.
globalThis.addEventListener?.("scroll", () => tipHide(), true);

// ---- V1: memory as one stacked bar per resource ----

// memBars draws view.MemoryBar values: [in use][estimate, its range hatched][free] on the
// capacity, the capacity printed after it. An estimate past the capacity runs beyond the
// track's end in red. An unknown capacity has no scale: the track is empty and dashed, and
// the estimate is printed instead (view's MemoryBar.Size).
export function memBars(bars, cls = "") {
  const box = el("div", "mbars" + (cls ? " " + cls : ""));
  for (const b of bars || []) box.append(memBar(b));
  return box;
}

function memBar(b) {
  const parts = b.parts || [];
  const segs = [];
  if (b.used > 0) segs.push(["used", b.used]);
  let range = 0;
  for (const p of parts) {
    segs.push([p.kind, Math.max(0, p.low)]);
    range += Math.max(0, p.high - p.low);
  }
  if (range > 0) segs.push(["range", range]);
  const end = segs.reduce((n, s) => n + s[1], 0);
  const scale = Math.max(b.total || 0, end) || 1;
  const row = el("div", "mbar-row" + (!parts.length ? " pending" : "") + (!b.total ? " nocap" : ""));
  visTip(row, b.text);
  const track = el("span", "mbar");
  if (b.total > 0) track.style.setProperty("--cap", (b.total / scale) * 100 + "%");
  // cut each segment at the capacity: what lies past it is over
  let at = 0;
  if (b.total > 0) for (const [kind, v] of segs) {
    if (!v) continue;
    const cut = b.total > 0 ? Math.max(0, Math.min(v, b.total - at)) : v;
    if (cut > 0) track.append(seg("m-" + kind, cut, scale));
    if (v - cut > 0) track.append(seg("m-" + kind + " m-over", v - cut, scale));
    at += v;
  }
  if (end > (b.total || Infinity)) row.classList.add("over");
  row.append(el("span", "mbar-k", b.label), track, el("span", "mbar-n num", b.size || "—"));
  if (b.note) row.append(el("span", "mbar-note", b.note));
  return row;
}

function seg(kind, v, scale) {
  const i = el("i", kind);
  i.style.width = (v / scale) * 100 + "%";
  return i;
}

// ---- V3, V4: counts as bars ----

// countBar draws a view.CountBar: a fill scaled to max (the largest of its list) on a track,
// its kinds as shades of one colour, largest first, and the total printed after it. The
// kinds' names and numbers are in the sentence it is named by.
export function countBar(c, max, cls = "") {
  const row = el("div", "cbar-row" + (cls ? " " + cls : ""));
  visTip(row, c.text);
  const track = el("span", "cbar");
  const fill = el("span", "cbar-fill");
  fill.style.width = (max > 0 ? Math.max(1, (c.n / max) * 100) : 100) + "%";
  (c.parts || []).forEach((p, i) => {
    const k = el("i", "c-k" + Math.min(i, 2));
    k.style.flexGrow = String(p.n);
    fill.append(k);
  });
  track.append(fill);
  row.append(track, el("span", "mbar-n num", String(c.n)));
  return row;
}

// countMark: a quiet bar beside a printed count, scaled to the largest of its list. The number
// is printed, so the bar is decoration: hidden from assistive technology and not focusable.
export function countMark(n, max) {
  const s = el("span", "cmark");
  s.setAttribute("aria-hidden", "true");
  const f = el("i");
  f.style.width = (max > 0 ? Math.max(2, (n / max) * 100) : 0) + "%";
  s.append(f);
  return s;
}

// stepMeter: value of max as filled steps (V4's likelihood in ten, severity in three); a
// meter named by the sentence it stands for. The number of filled steps carries the value,
// so it reads without colour.
export function stepMeter(value, max, steps, text, cls = "") {
  const m = el("span", "smeter" + (cls ? " " + cls : ""));
  visTip(m, text, "meter");
  m.setAttribute("aria-valuemin", "0");
  m.setAttribute("aria-valuemax", String(max));
  m.setAttribute("aria-valuenow", String(value));
  const on = Math.round((value / max) * steps);
  for (let i = 0; i < steps; i++) m.append(el("i", i < on ? "on" : ""));
  return m;
}

// ---- V5: what a list brings, and what it does to a list ----

// splitBar: a whole split into two kinds, the first ✓ and the second ✕, both counts printed
// after it (V5: installed / not installed). Named by the sentence it stands for.
export function splitBar(yes, no, text) {
  const row = el("div", "sbar-row");
  visTip(row, text, "img", false);
  const bar = el("span", "sbar");
  const a = el("i", "s-yes"), b = el("i", "s-no");
  a.style.flexGrow = String(yes);
  b.style.flexGrow = String(no);
  if (yes) bar.append(a);
  if (no) bar.append(b);
  row.append(bar, el("span", "s-n yes num", "✓ " + yes), el("span", "s-n no num" + (no ? "" : " zero"), "✕ " + no));
  return row;
}

// diffMarks: a list change as Saves' + − ↕ marks (and ⇄ for another version of a Mod), each
// a number; named by the sentence that says it in words.
export function diffMarks(e) {
  const m = el("span", "dmarks");
  visTip(m, e.text, "img", false);
  for (const [n, cls, sign] of [[e.added, "add", "+"], [e.removed, "del", "−"], [e.moved, "mv", "↕"], [e.swapped, "sw", "⇄"]]) {
    if (n) m.append(el("span", cls + " num", sign + n));
  }
  return m;
}

// ---- V6, V11: states as icons, their word as the name ----

// stateMark: a status icon (or a dot) named by its word, which is its tooltip too. kind is
// an icon of icons.js (ok, err, warn…), "dot" for a small round mark, or "" for no mark: the
// word then stays in the accessibility tree only (a harmless state that needs no mark).
export function stateMark(kind, word, svg = "") {
  const m = el("span", "smark" + (kind ? " k-" + kind : ""));
  if (!kind) {
    m.className = "vis-hidden";
    m.textContent = word;
    return m;
  }
  visTip(m, word, "img", false);
  if (kind === "dot" || kind === "ring") m.append(el("i"));
  else m.innerHTML = svg;
  return m;
}

// fileMarks: what changed in a Mod's files, by kind, as + ~ − counts (V6); named by the
// sentence that says it in words.
export function fileMarks(files, text) {
  const m = el("span", "fmarks");
  visTip(m, text, "img", false);
  for (const f of files) {
    const k = el("span", "fk");
    k.append(el("span", "fk-n", f.kind));
    if (f.added) k.append(el("span", "add num", "+" + f.added));
    if (f.changed) k.append(el("span", "chg num", "~" + f.changed));
    if (f.gone) k.append(el("span", "del num", "−" + f.gone));
    m.append(k);
  }
  return m;
}

// ---- V7: before and after ----

// shrinkBar: a size before and after as one bar the length of before: what stays (accent),
// then what is saved (hatched, the track), "before → after" printed after it (view.Shrink).
export function shrinkBar(s) {
  const row = el("div", "kbar-row");
  visTip(row, s.text, "img", false);
  const bar = el("span", "kbar");
  const stay = el("i", "k-stay");
  stay.style.width = (s.before > 0 ? Math.max(1, Math.min(100, (s.after / s.before) * 100)) : 0) + "%";
  bar.append(stay, el("i", "k-saved"));
  row.append(bar, el("span", "num", s.beforeText + " → " + s.afterText));
  return row;
}

// ---- V8: sizes in table cells ----

// sizeBar: a faint bar behind a printed size, scaled to the largest row (V8, 16 §7 principle
// 9); the number stays, so the bar is only a background
export function sizeBar(cell, bytes, most) {
  if (!(bytes > 0 && most > 0)) return;
  cell.classList.add("szbar");
  cell.style.setProperty("--w", Math.max(1, Math.round((bytes / most) * 100)) + "%");
}
