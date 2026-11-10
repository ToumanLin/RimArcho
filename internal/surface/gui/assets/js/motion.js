// Motion (16 §7.1): short, and mostly opacity and transform, so WebView2 composites it
// without layout or paint work (the sidebar's sub-entries grow their grid row); nothing
// moves under prefers-reduced-motion or settings › motion: reduced (ui.motion, the root's
// data-motion). The CSS side (the tokens --t-press, --t-enter, --t-layout and the
// keyframes) is in style.css.
import { S, $$, key, ROW_H } from "./state.js";

const reduce = matchMedia("(prefers-reduced-motion: reduce)");
export const still = () => reduce.matches || globalThis.document?.documentElement?.dataset?.motion === "reduced";

const LAYOUT_MS = 250;
const FLASH_MS = 400;

// ---- the lists: FLIP the visible rows to their new place, flash the changed ones ----

// snapshot remembers where each visible row of the three lists is on screen, by mod.
// Only the rows the virtual list has drawn are looked at (a few dozen at most).
export function snapshot() {
  if (still()) return null;
  const out = [new Map(), new Map(), new Map()];
  for (const k of [0, 1, 2]) {
    const box = document.querySelector(`.area[data-area="${k}"] .rows`);
    if (!box || !box.clientHeight) continue;
    const c = S.cols[k];
    for (const d of box.querySelectorAll(":scope > .row")) {
      const it = c.items[c.view[+d.dataset.i]];
      if (it) out[k].set(key(it), parseFloat(d.style.top || "0") - box.scrollTop);
    }
  }
  return out;
}

// play moves each row that was on screen from where it was to where it is now, and
// flashes the rows that changed: the mods acted on (ids) when known, else the rows that
// moved or came into the list.
export function play(before, ids) {
  if (!before || still()) return;
  const acted = new Set(ids || []);
  const moves = [];
  for (const k of [0, 1, 2]) {
    const box = document.querySelector(`.area[data-area="${k}"] .rows`);
    if (!box || !box.clientHeight) continue;
    const c = S.cols[k];
    const was = before[k];
    for (const d of box.querySelectorAll(":scope > .row")) {
      const it = c.items[c.view[+d.dataset.i]];
      if (!it) continue;
      const kk = key(it);
      const now = parseFloat(d.style.top || "0") - box.scrollTop;
      const y0 = was.get(kk);
      const dy = y0 == null ? 0 : y0 - now;
      if (dy && Math.abs(dy) < box.clientHeight + ROW_H) moves.push([d, dy]);
      if (acted.size ? acted.has(it.id) : y0 == null ? was.size > 0 : dy !== 0) flash(d);
    }
  }
  if (!moves.length) return;
  // Invert every row first, let the browser take that in once, then play them all: one
  // style recalculation for the batch, and no wait for an animation frame.
  for (const [d, dy] of moves) {
    d.style.transition = "none";
    d.style.transform = `translateY(${dy}px)`;
  }
  void document.body.offsetHeight;
  for (const [d] of moves) {
    d.style.transition = `transform ${LAYOUT_MS}ms var(--ease)`;
    d.style.transform = "";
  }
  setTimeout(() => { for (const [d] of moves) if (!d.style.transform) d.style.transition = ""; }, LAYOUT_MS + 40);
}

function flash(d) {
  d.classList.remove("flash");
  void d.offsetWidth; // restart the animation on a row flashed a moment ago
  d.classList.add("flash");
  setTimeout(() => d.classList.remove("flash"), FLASH_MS + 40);
}

// ---- numbers: a badge or a count bumps when its number changes ----

const seen = new Map();

// bump marks e when its number for name changed since the last call (not the first time).
export function bump(name, e, value) {
  const v = String(value ?? "");
  const was = seen.get(name);
  seen.set(name, v);
  if (!e || was === undefined || was === v || still()) return;
  e.classList.remove("bump");
  void e.offsetWidth;
  e.classList.add("bump");
}

// ---- reordered entries slide to their new place (the sidebar's lists, Manage lists's rows) ----

// places remembers where each element matching sel under root is on screen, by its data-id:
// a redraw replaces the elements, and slideFrom moves the new ones from there.
export function places(root, sel) {
  if (!root || still()) return null;
  const m = new Map();
  for (const e of root.querySelectorAll(sel)) if (e.dataset.id && e.offsetParent) m.set(e.dataset.id, e.getBoundingClientRect().top);
  return m;
}

// slideFrom moves each element that was in places() from where it was to where it is now,
// like the lists' rows (one style recalculation for the batch).
export function slideFrom(root, sel, before) {
  if (!root || !before || !before.size || still()) return;
  const moves = [];
  for (const e of root.querySelectorAll(sel)) {
    const y0 = before.get(e.dataset.id);
    if (y0 == null || !e.offsetParent) continue;
    const dy = y0 - e.getBoundingClientRect().top;
    if (Math.abs(dy) >= 1 && Math.abs(dy) < 800) moves.push([e, dy]);
  }
  if (!moves.length) return;
  for (const [e, dy] of moves) {
    e.style.transition = "none";
    e.style.transform = `translateY(${dy}px)`;
  }
  void document.body.offsetHeight;
  for (const [e] of moves) {
    e.style.transition = `transform ${LAYOUT_MS}ms var(--ease)`;
    e.style.transform = "";
  }
  setTimeout(() => { for (const [e] of moves) if (!e.style.transform) e.style.transition = ""; }, LAYOUT_MS + 40);
}

// ---- the sidebar: one highlight that slides to the open entry ----

// slideNav moves the sidebar's highlights (like the view switch's thumb): one under the
// entry that is on, and in an open entry one under the sub-entry that is on. Their size
// follows at once; only their place animates.
export function slideNav(nav, jump) {
  let th = nav.querySelector(":scope > .nav-thumb");
  if (!th) { th = document.createElement("span"); th.className = "nav-thumb"; nav.prepend(th); }
  place(th, nav.querySelector(":scope > .nav-item.on"), nav, true, jump);
  for (const box of nav.querySelectorAll(".kids-in")) place(box.querySelector(".kid-hl"), box.querySelector(".nav-kid.on"), box, false, jump);
}

// place puts the highlight h under on, inside box. In the sidebar itself the sub-entries
// above on may be opening or closing: its place is where it will be when they are done.
function place(h, on, box, outer, jump) {
  if (!h) return;
  if (!on) { h.style.opacity = "0"; delete h.dataset.placed; return; }
  const first = jump || !h.dataset.placed;
  let y = on.offsetTop;
  if (outer) {
    for (const w of box.querySelectorAll(":scope > .kids-wrap")) {
      if (!(w.compareDocumentPosition(on) & Node.DOCUMENT_POSITION_FOLLOWING)) break;
      const end = w.classList.contains("open") ? w.firstElementChild.scrollHeight : 0;
      y += end - w.offsetHeight;
    }
  }
  h.style.transition = first || still() ? "none" : "";
  h.style.width = on.offsetWidth + "px";
  h.style.height = on.offsetHeight + "px";
  h.style.transform = `translate(${on.offsetLeft}px, ${y}px)`;
  h.style.opacity = "1";
  h.dataset.placed = "1";
}

// ---- the status line fades each new sentence in, in place (no slide: it moved the bar) ----

const statusAnimations = new WeakMap();
export function statusIn(s) {
  statusAnimations.get(s)?.cancel();
  statusAnimations.delete(s);
  if (still() || !s.textContent) return;
  // Read only animation tokens: restarting through offsetWidth lays out the lists
  // just changed by an edit. Web Animations restarts the same fade/slide directly.
  const style = getComputedStyle(s), time = style.getPropertyValue("--t-enter").trim();
  const duration = parseFloat(time) * (time.endsWith("ms") ? 1 : 1000);
  statusAnimations.set(s, s.animate([
    { opacity: 0 },
    { opacity: 1 },
  ], { duration, easing: style.getPropertyValue("--ease").trim() }));
}

// ---- a page that loads lazily shows a shimmer, not a "loading" line ----

// shimmer is a placeholder of n lines; label is what a screen reader hears.
export function shimmer(label, n = 4) {
  const box = document.createElement("div");
  box.className = "skel";
  box.setAttribute("role", "progressbar");
  box.setAttribute("aria-label", label);
  box.title = label;
  for (let i = 0; i < n; i++) {
    const b = document.createElement("span");
    b.className = "bar";
    b.style.width = [88, 72, 94, 60, 80, 66][i % 6] + "%";
    box.append(b);
  }
  return box;
}

// rows: the list rows a FLIP may move (tests and the scroll check).
export const visibleRows = () => $$("#board .rows > .row");
