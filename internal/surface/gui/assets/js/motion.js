// Motion (06 §7.1): short, and only opacity and transform, so WebView2 composites it
// without layout or paint work; nothing moves under prefers-reduced-motion. The CSS side
// (the tokens --t-press, --t-enter, --t-layout and the keyframes) is in style.css.
import { S, $$, key, ROW_H } from "./state.js";

const reduce = matchMedia("(prefers-reduced-motion: reduce)");
export const still = () => reduce.matches;

const LAYOUT_MS = 260;
const FLASH_MS = 600;

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
      if (it) out[k].set(key(it), +d.dataset.i * ROW_H - box.scrollTop);
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
      const now = +d.dataset.i * ROW_H - box.scrollTop;
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

// ---- the sidebar: one highlight that slides to the open entry ----

// slideNav moves the sidebar's highlight under the entry that is on (like the view
// switch's thumb). Its size follows at once; only its place animates.
export function slideNav(nav) {
  let th = nav.querySelector(":scope > .nav-thumb");
  if (!th) { th = document.createElement("span"); th.className = "nav-thumb"; nav.prepend(th); }
  const on = nav.querySelector(".nav-item.on, .nav-kid.on");
  if (!on) { th.style.opacity = "0"; return; }
  const first = !th.dataset.placed;
  const r = on.getBoundingClientRect(), n = nav.getBoundingClientRect();
  const x = r.left - n.left + nav.scrollLeft, y = r.top - n.top + nav.scrollTop;
  if (first || still()) th.style.transition = "none";
  else th.style.transition = "";
  th.style.width = r.width + "px";
  th.style.height = r.height + "px";
  th.style.borderRadius = getComputedStyle(on).borderRadius;
  th.style.transform = `translate(${x}px, ${y}px)`;
  th.style.opacity = "1";
  th.dataset.placed = "1";
}

// ---- the status line slides in from below ----

export function statusIn(s) {
  if (still() || !s.textContent) return;
  s.classList.remove("in");
  void s.offsetWidth;
  s.classList.add("in");
}

// ---- a page that loads lazily shows a shimmer, not a "loading" line ----

// shimmer is a placeholder of n lines; label is what a screen reader hears.
export function shimmer(label, n = 4) {
  const box = document.createElement("div");
  box.className = "skel";
  box.setAttribute("role", "status");
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
