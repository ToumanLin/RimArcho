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
