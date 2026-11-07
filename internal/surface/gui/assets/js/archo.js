// Archo's figure keeps the player company while RimArcho works (16 §7.1 "Archo figure"):
// the start-up screen, page-loading cards and the first-run guide.
//
//   const w = archoWait(slot);          // a loading card: nothing for 0.21 s, then the figure
//   w.end();                            // the read finished or failed: the figure fades out
//   const f = archoFigure(slot, "wink"); f.set("think"); f.end();   // shown at once (the guide)
//
// Cost (16 §10): nothing is loaded at start. A still (<img>, ~32 KB WebP) shows first; with
// ui.archo.motion "live", motion allowed and a hardware WebGL2 context, the one shared puppet
// (~0.15 ms CPU per frame, 2.4 MB of textures) is created, cross-fades over the still and
// moves its canvas from slot to slot. It stops drawing when hidden and is destroyed 10 s after
// the last wait, which frees its textures and its WebGL context.
import { api } from "./state.js";

// The calm expressions that rotate while something loads.
export const POOL = ["neutral", "smile", "laugh", "wink", "think", "ponder", "yum"];

const BASE = "/archo/", PACK = BASE + "archo-s/";
const DELAY = 210, ROTATE = 4000, TIP_ROTATE = 8000, IDLE = 10000, SIZE = 176;
const still = (expr) => `${PACK}stills/${expr}@360.webp`;
const reduced = () => !!globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
const image = () => { const i = document.createElement("img"); i.alt = ""; i.decoding = "async"; i.draggable = false; return i; };
const decoded = (i) => i.decode?.().catch(() => {}) || Promise.resolve();

let mode = "live";
try { mode = localStorage.getItem("ui.archo.motion") || "live"; } catch { /* Init supplies it. */ }

/** setMode follows ui.archo.motion (off | still | live); a cached copy lets the start-up
 *  screen choose before Init answers. */
export function setMode(m) {
  mode = ["off", "still", "live"].includes(m) ? m : "live";
  try { localStorage.setItem("ui.archo.motion", mode); } catch { /* storage can be off */ }
  if (mode !== "live") dropPuppet();
}
export const archoMode = () => mode;

// Round-robin over POOL, carried across waits and runs, so loads do not repeat a face.
let turn = 0;
try { turn = +localStorage.getItem("archo.turn") || 0; } catch { /* starts at 0 */ }
export function nextExpr(pool = POOL) {
  const e = pool[turn % pool.length];
  turn = (turn + 1) % 1000;
  try { localStorage.setItem("archo.turn", turn); } catch { /* storage can be off */ }
  return e;
}

// ---- the one live figure -------------------------------------------------------------
let puppet = null, creating = null, idleTimer = 0, sweepTimer = 0, owner = null, broken = false;

// A hardware context only: with GPU acceleration off, WebGL2 runs in software and costs
// far more than a still is worth.
const hardware = (() => {
  let ok;
  return () => ok ??= (() => {
    try {
      const gl = document.createElement("canvas").getContext("webgl2", { failIfMajorPerformanceCaveat: true });
      gl?.getExtension("WEBGL_lose_context")?.loseContext();
      return !!gl;
    } catch { return false; }
  })();
})();
const canMove = () => mode === "live" && !broken && !reduced() && hardware();

async function ensurePuppet() {
  clearTimeout(idleTimer);
  if (puppet) return puppet;
  creating ??= (async () => {
    try {
      const { ArchoPuppet } = await import(BASE + "archo-gl.js");
      const p = await ArchoPuppet.create(document.createElement("div"), { base: PACK, size: SIZE, view: "wide", fps: 24 });
      p.unmount();
      puppet = p;
      // a figure whose page went away without end() (the guide was left) gives the canvas up
      sweepTimer = setInterval(() => { if (owner && !owner.slot.isConnected) owner.end(); }, IDLE);
      return p;
    } catch {
      broken = true; // the stills stay for this window
      return null;
    } finally { creating = null; }
  })();
  return creating;
}

function dropPuppet() {
  clearTimeout(idleTimer); clearInterval(sweepTimer);
  owner?.slot.classList.remove("live"); // a figure shown now keeps its still
  if (puppet) { puppet.destroy(); puppet = null; }
  owner = null; waiting.clear();
}

// waiting: figures that wanted the live canvas while another had it; the newest one still
// on the page takes it when the owner ends.
const waiting = new Set();
function claim(h) {
  waiting.delete(h);
  owner?.slot.classList.remove("live"); // its still shows again
  owner = h;
  puppet.setExpression(h.expr);
  puppet.mount(h.slot);
  requestAnimationFrame(() => { if (owner === h && puppet?.cv.parentNode === h.slot) h.slot.classList.add("live"); });
}
function handOver() {
  const next = [...waiting].reverse().find((h) => !h.ended && h.slot.isConnected);
  waiting.clear();
  if (next && puppet) claim(next);
}

function releaseLater() {
  clearTimeout(idleTimer);
  idleTimer = setTimeout(() => { if (!owner) dropPuppet(); }, IDLE);
}

// ---- a figure in a slot ----------------------------------------------------------------
// show is shared by waits and figures: delay before anything appears, the still, the live
// figure when allowed, and rotation over pool every ROTATE ms (rotate: false keeps one face).
function show(slot, { delay = 0, expr, pool = POOL, rotate = true, img: given } = {}) {
  const h = { ended: false, expr: expr || nextExpr(pool), slot };
  if (mode === "off") return { ...h, end() {}, set() {} };
  slot.classList.add("archo-slot");
  slot.setAttribute("aria-hidden", "true");
  const img = given || image();
  const timers = [];
  const later = (ms, f) => timers.push(setTimeout(() => { if (!h.ended) f(); }, ms));
  const mine = () => owner === h && puppet && puppet.cv.parentNode === slot;

  const appear = async () => {
    if (!given) { img.src = still(h.expr); slot.append(img); }
    decoded(img).then(() => { if (!h.ended) slot.classList.add("on"); });
    if (!canMove()) return;
    const p = await ensurePuppet();
    if (!p || h.ended) { if (p && !owner) releaseLater(); return; }
    if (owner && !owner.ended && owner.slot.isConnected) { waiting.add(h); return; } // a still until the other ends
    if (owner) { const gone = owner; owner = null; gone.end(); } // its page went away
    claim(h);
  };
  if (delay) later(delay, appear); else appear();

  h.set = (e) => {
    if (h.ended || !e || e === h.expr) return;
    h.expr = e;
    if (mine()) { puppet.setExpression(e); return; }
    const next = still(e), pre = image();
    pre.src = next;
    decoded(pre).then(() => { if (!h.ended && h.expr === e) img.src = next; });
  };
  if (rotate && !reduced()) {
    const step = () => later(ROTATE, () => { h.set(nextExpr(pool)); step(); });
    later(delay, step);
  }

  h.end = () => {
    if (h.ended) return;
    h.ended = true;
    timers.forEach(clearTimeout);
    waiting.delete(h);
    if (owner === h) { owner = null; handOver(); } // the next figure takes the canvas at once
    slot.classList.remove("on");
    const done = () => {
      if (puppet && puppet.cv.parentNode === slot && owner === null) puppet.unmount();
      img.remove(); slot.classList.remove("live");
      if (puppet || creating) releaseLater();
    };
    if (slot.isConnected && globalThis.getComputedStyle?.(slot).opacity !== "0") setTimeout(done, 150); else done();
  };
  return h;
}

/** archoWait keeps the player company in a loading card: nothing for the first 0.21 s, so a
 *  fast read shows nothing new, then the figure, with the expression changing every 4 s. */
export const archoWait = (slot, opts = {}) => show(slot, { delay: DELAY, ...opts });

/** archoFigure shows Archo at once with one expression (the first-run guide). */
export const archoFigure = (slot, expr) => show(slot, { expr, rotate: false });

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
