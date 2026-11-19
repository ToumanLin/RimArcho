// Archo's figure keeps the player company while RimArcho works (16 §7.1 "Archo figure"):
// the start-up screen, page-loading cards and the first-run guide.
//
//   const w = archoWait(slot);          // a loading card: nothing for 0.21 s, then the figure
//   w.end();                            // the read finished or failed: the figure fades out
//   const f = archoFigure(slot, "wink"); f.set("think"); f.end();   // shown at once (the guide)
//
// Cost (16 §10): nothing is loaded at start. A still (<img>, ~20 KB WebP, 360 px) shows
// first; with ui.archo.motion "live" and motion allowed, the expression's loop (an animated
// WebP, ~230 KB, 18 frames at 8.3 fps) is fetched and decoded beside it and replaces it.
// The loop's first frame is the still, so the swap does not jump. A figure shown while
// nothing runs (the guide) plays its loop a few times and rests on the still again.
// Removing the <img> frees its frames.
import { api } from "./state.js";

// The calm expressions that rotate while something loads.
export const POOL = ["neutral", "smug", "laugh", "wink", "think", "ponder", "yum"];

const PACK = "/archo/archo-s/";
const DELAY = 210, ROTATE = 4000, TIP_ROTATE = 8000, LOOP = 2160, REST_LOOPS = 3;
const still = (expr) => `${PACK}stills/${expr}@360.webp`;
const loopOf = (expr) => `${PACK}anim/${expr}.webp`;
const reduced = () => !!globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches || globalThis.document?.documentElement?.dataset?.motion === "reduced";
const image = () => { const i = document.createElement("img"); i.alt = ""; i.decoding = "async"; i.draggable = false; return i; };
const decoded = (i) => i.decode?.().catch(() => {}) || Promise.resolve();

let mode = "live";
try { mode = localStorage.getItem("ui.archo.motion") || "live"; } catch { /* Init supplies it. */ }

/** setMode follows ui.archo.motion (off | still | live); a cached copy lets the start-up
 *  screen choose before Init answers. */
export function setMode(m) {
  mode = ["off", "still", "live"].includes(m) ? m : "live";
  try { localStorage.setItem("ui.archo.motion", mode); } catch { /* storage can be off */ }
}
export const archoMode = () => mode;
const canMove = () => mode === "live" && !reduced();

// Round-robin over POOL, carried across waits and runs, so loads do not repeat a face.
let turn = 0;
try { turn = +localStorage.getItem("archo.turn") || 0; } catch { /* starts at 0 */ }
export function nextExpr(pool = POOL) {
  const e = pool[turn % pool.length];
  turn = (turn + 1) % 1000;
  try { localStorage.setItem("archo.turn", turn); } catch { /* storage can be off */ }
  return e;
}

// ---- a figure in a slot ----------------------------------------------------------------
// show is shared by waits and figures: delay before anything appears, the still, then the
// loop when allowed, and rotation over pool every ROTATE ms (rotate: false keeps one face).
// rest: the loop plays REST_LOOPS times and the still comes back (nothing moves at rest,
// 16 §10); a new expression plays again.
function show(slot, { delay = 0, expr, pool = POOL, rotate = true, rest = false, img: given } = {}) {
  const h = { ended: false, expr: expr || nextExpr(pool), slot };
  if (mode === "off") return { ...h, end() {}, set() {} };
  slot.classList.add("archo-slot");
  slot.setAttribute("aria-hidden", "true");
  const img = given || image();
  const timers = [];
  let restTimer = 0, shown = false;
  const later = (ms, f) => timers.push(setTimeout(() => { if (!h.ended) f(); }, ms));

  // put shows expression e: its still (at once on first), then its loop once decoded
  const put = (e, first) => {
    const s = still(e), mine = () => !h.ended && h.expr === e;
    clearTimeout(restTimer);
    const pre = image();
    pre.src = s;
    if (first) img.src = s;
    decoded(pre).then(() => {
      if (!mine()) return;
      img.src = s;
      if (!canMove()) return;
      const anim = image();
      anim.src = loopOf(e);
      return decoded(anim).then(() => {
        if (!mine()) return;
        img.src = anim.src;
        if (rest) restTimer = setTimeout(() => { if (mine()) img.src = s; }, LOOP * REST_LOOPS);
      });
    });
  };
  const appear = () => {
    shown = true;
    if (!given) slot.append(img);
    put(h.expr, !given);
    decoded(img).then(() => { if (!h.ended) slot.classList.add("on"); });
  };
  if (delay) later(delay, appear); else appear();

  h.set = (e) => {
    if (h.ended || !e || e === h.expr) return;
    h.expr = e;
    if (shown) put(e, false);
  };
  if (rotate && !reduced()) {
    const step = () => later(ROTATE, () => { h.set(nextExpr(pool)); step(); });
    later(delay, step);
  }

  h.end = () => {
    if (h.ended) return;
    h.ended = true;
    timers.forEach(clearTimeout);
    clearTimeout(restTimer);
    slot.classList.remove("on");
    const done = () => img.remove(); // frees the loop's frames
    if (slot.isConnected && globalThis.getComputedStyle?.(slot).opacity !== "0") setTimeout(done, 150); else done();
  };
  return h;
}

/** archoWait keeps the player company in a loading card: nothing for the first 0.21 s, so a
 *  fast read shows nothing new, then the figure, with the expression changing every 4 s. */
export const archoWait = (slot, opts = {}) => show(slot, { delay: DELAY, ...opts });

/** archoFigure shows Archo at once with one expression (the first-run guide); its loop
 *  plays a few times on each new expression, then rests on the still. */
export const archoFigure = (slot, expr) => show(slot, { expr, rotate: false, rest: true });

// ---- usage tips --------------------------------------------------------------------------
let tips = null, tipsRead = null, tipTurn = -1;
try { tipTurn = +localStorage.getItem("archo.tip"); } catch { /* random start below */ }

/** loadTips reads the tips once per window (view.Tips, already in the player's language). */
export function loadTips() {
  tipsRead ??= Promise.resolve().then(() => api.Tips()).then((t) => { tips = t || []; return tips; }, () => { tips = []; return tips; });
  return tipsRead;
}

/** nextTip is the following tip for when ("any" or "first"); "" before the tips arrive. */
export function nextTip(when = "any") {
  if (!tips?.length) return "";
  const list = tips.filter((t) => t.when === when); // "first" tips name the guide: only there
  if (!list.length) return "";
  if (!(tipTurn >= 0)) tipTurn = Math.floor(Math.random() * list.length);
  const t = list[tipTurn % list.length];
  tipTurn = (tipTurn + 1) % 1000;
  try { localStorage.setItem("archo.tip", tipTurn); } catch { /* storage can be off */ }
  return t.text;
}

/** tipLine fills line with a tip after delay ms (or once the tips arrive) and, in a long
 *  wait, the next one every 8 s. Returns stop(). */
export function tipLine(line, when = "any", delay = 0) {
  let stopped = false, timer = 0;
  const put = (text) => {
    if (stopped || !text) return;
    if (!line.textContent) { line.textContent = text; line.classList.add("on"); return; }
    line.classList.remove("on");
    setTimeout(() => { if (!stopped) { line.textContent = text; line.classList.add("on"); } }, 150);
  };
  const loop = () => { timer = setTimeout(() => { put(nextTip(when)); loop(); }, TIP_ROTATE); };
  const begin = () => {
    if (stopped) return;
    if (tips) put(nextTip(when)); else loadTips().then(() => put(nextTip(when)));
    if (!reduced()) loop();
  };
  if (delay) timer = setTimeout(begin, delay); else begin();
  return () => { stopped = true; clearTimeout(timer); };
}

// ---- the start-up screen -----------------------------------------------------------------
// index.html holds #boot with a still chosen by boot.js; it appears after 0.21 s by CSS alone.
// bootLive turns it into the live figure with a tip; bootDone fades it out once the first
// page has drawn (or the start failed, or a dialog needs the window).
let boot = null, bootTimer = 0;
export function bootLive() {
  const box = document.getElementById("boot");
  if (!box || boot || bootTimer) return;
  if (mode === "off") { box.remove(); return; }
  // the same 0.21 s as a loading card: a fast start never pays for the figure or the tips
  bootTimer = setTimeout(() => {
    if (!box.isConnected) return;
    const slot = box.querySelector(".archo-slot"), line = box.querySelector(".boot-tip");
    const img = slot.querySelector("img");
    boot = { figure: show(slot, { expr: img?.dataset.expr, rotate: false, img }), stop: tipLine(line) };
  }, Math.max(0, DELAY - performance.now()));
}
export function bootDone() {
  const box = document.getElementById("boot");
  clearTimeout(bootTimer);
  if (!box) return;
  boot?.stop(); boot?.figure.end();
  const o = globalThis.getComputedStyle?.(box).opacity;
  if (!o || o === "0") { box.remove(); return; } // not shown yet
  box.classList.add("out");
  setTimeout(() => box.remove(), 160);
}
