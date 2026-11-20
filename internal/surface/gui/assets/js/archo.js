// Archo's figure keeps the player company while RimArcho works (16 §7.1 "Archo figure"):
// the start-up screen, page-loading cards, the first-run guide and five page placements.
//
//   const w = archoWait(slot);          // a loading card: nothing for 0.21 s, then the figure
//   w.end();                            // the read finished or failed: the figure fades out
//   const f = archoFigure(slot, "wink"); f.set("think"); f.end();   // shown at once (the guide)
//
// Cost (16 §10): nothing is loaded at start. With ui.archo.motion "live" and motion allowed a
// figure only ever shows baked clips (animated WebP, 300 x 202, 66 ms a frame), all on one
// 2.376 s clock of 36 phases, so they join without a cut (scripts/archopack/build.py):
//   <e>      the expression's loop          <e>-in   from the calm idle to <e> (12 frames)
//   <e>-out  from <e> back to the idle      bridge   idle phases between an out and an in
//   idle     the calm idle (breath, two blinks a 4.75 s pass, slight sway; 8.4 frames a second)
// A figure plays a program: clips spliced into one animated WebP (each clip starts with a key
// frame over the whole canvas), in an <img> that replaces the shown one only where the two
// meet: at the end of a loop pass or of an idle pass. A figure at rest (the guide, placements)
// plays a new expression as in, three loops, out, bridge, then the idle for ever; the idle
// stops (a still of its first frame, where it would start again) while the window is hidden,
// she is off screen, or the window has been in the background for a minute. Waits loop their
// expression for ever. With motion still, or reduced, a still (~20 KB WebP, 360 px) is shown.
// Removing the <img> frees its frames.
import { api } from "./state.js";

// The calm expressions that rotate while something loads.
export const POOL = ["neutral", "smug", "laugh", "wink", "think", "ponder", "yum"];

const PACK = "/archo/archo-s/";
const DELAY = 210, ROTATE = 4000, TIP_ROTATE = 8000, REST_LOOPS = 3;
const FRAME = 66, LOOP = 36 * FRAME, CLIP = 12 * FRAME, LEAD = 300, AWAY = 60000;
const still = (expr) => `${PACK}stills/${expr}@360.webp`;
const clipURL = (c) => `${PACK}anim/${c}.webp`;
const reduced = () => !!globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches || globalThis.document?.documentElement?.dataset?.motion === "reduced";
const image = () => { const i = document.createElement("img"); i.alt = ""; i.decoding = "async"; i.draggable = false; return i; };
const decoded = (i) => i.decode?.().catch(() => {}) || Promise.resolve();
const now = () => globalThis.performance?.now?.() ?? Date.now();

// ---- clips ---------------------------------------------------------------------------------
// clip reads one clip file once per window: its chunks before the first frame (VP8X, ANIM)
// and its frames (ANMF chunks, padding included). A few recent ones are kept.
const clips = new Map();
function clip(name) {
  let p = clips.get(name);
  if (p) { clips.delete(name); clips.set(name, p); return p; }
  p = fetch(clipURL(name)).then((r) => {
    if (!r.ok) throw Error(`${clipURL(name)}: ${r.status}`);
    return r.arrayBuffer();
  }).then((b) => chunks(new Uint8Array(b)));
  p.catch(() => clips.delete(name));
  clips.set(name, p);
  if (clips.size > 12) clips.delete(clips.keys().next().value);
  return p;
}
const tag = (b, p) => String.fromCharCode(b[p], b[p + 1], b[p + 2], b[p + 3]);
export function chunks(b) {
  const v = new DataView(b.buffer, b.byteOffset, b.byteLength), head = [], frames = [];
  for (let p = 12; p + 8 <= b.length;) {
    const end = Math.min(b.length, p + 8 + v.getUint32(p + 4, true) + (v.getUint32(p + 4, true) & 1));
    (tag(b, p) === "ANMF" ? frames : frames.length ? [] : head).push(b.subarray(p, end));
    p = end;
  }
  return { head, frames };
}
// splice joins clips ([{c: chunks, n: times}]) into one animated WebP that plays `plays` times
// (0: for ever). The first clip's VP8X and ANIM lead; the ANIM's loop count is set.
export function splice(parts, plays) {
  const head = parts[0].c.head.map((h) => h.slice());
  for (const h of head) if (tag(h, 0) === "ANIM") new DataView(h.buffer).setUint16(12, plays, true);
  const body = [...head];
  for (const { c, n } of parts) for (let i = 0; i < n; i++) body.push(...c.frames);
  const size = 4 + body.reduce((s, x) => s + x.length, 0);
  const riff = new Uint8Array(12), v = new DataView(riff.buffer);
  riff.set([82, 73, 70, 70]); v.setUint32(4, size, true); riff.set([87, 69, 66, 80], 8);
  return new Blob([riff, ...body], { type: "image/webp" });
}
// setPlays writes the loop count of an animated WebP: the ANIM chunk holds the background
// colour (4 bytes), then the count (2 bytes, little-endian; 0 = for ever).
export function setPlays(b, n) {
  const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
  for (let p = 12; p + 14 <= b.length;) {
    const size = v.getUint32(p + 4, true);
    if (tag(b, p) === "ANIM") { v.setUint16(p + 12, n, true); return true; }
    p += 8 + size + (size & 1);
  }
  return false;
}

// ---- programs ------------------------------------------------------------------------------
// A program is a list of steps [clip, times]; `then` is the program that follows it for ever
// (null: the program itself loops for ever). Seams: where a figure may switch, by time from the
// program's start, with the pose there: an expression (the end of one of its loop passes) or
// "hub" (the end of an idle pass or of the bridge: the idle's phase where an in may start).
const ms = (c) => c === "idle" ? 2 * LOOP : c.endsWith("-in") || c.endsWith("-out") || c === "bridge" ? CLIP : LOOP;
function seams(steps) {
  const out = [];
  let t = 0;
  for (const [c, n] of steps) for (let i = 0; i < n; i++) {
    if (c === "idle") { out.push([t + LOOP, "hub"], [t + 2 * LOOP, "hub"]); t += 2 * LOOP; continue; }
    t += ms(c);
    if (c === "bridge") out.push([t, "hub"]);
    else if (!c.endsWith("-in") && !c.endsWith("-out")) out.push([t, c]);
  }
  return { out, total: t };
}
// from: the pose at a seam ("hub" or an expression); to: the expression to show, or "idle".
// rest: back to the idle after REST_LOOPS loops; otherwise the expression loops for ever.
function program(from, to, rest) {
  const steps = from !== "hub" && from !== to ? [[from + "-out", 1], ["bridge", 1]] : [];
  const idle = { steps: [["idle", 1]], then: null };
  if (to === "idle") return from === "hub" ? idle : { steps, then: idle };
  if (from !== to) steps.push([to + "-in", 1]);
  if (!rest) return steps.length ? { steps, then: { steps: [[to, 1]], then: null } } : { steps: [[to, 1]], then: null };
  steps.push([to, REST_LOOPS], [to + "-out", 1], ["bridge", 1]);
  return { steps, then: idle };
}

// pausing: a figure at rest stops in the idle while nobody can see it move
let focusedAt = 0, blurredAt = globalThis.document?.hasFocus?.() === false ? now() : 0;
const awake = new Set();
globalThis.addEventListener?.("blur", () => { blurredAt = now(); setTimeout(() => awake.forEach((f) => f()), AWAY + 50); });
globalThis.addEventListener?.("focus", () => { blurredAt = 0; focusedAt = now(); awake.forEach((f) => f()); });
globalThis.document?.addEventListener?.("visibilitychange", () => awake.forEach((f) => f()));
const away = () => !!globalThis.document?.hidden || (blurredAt > focusedAt && now() - blurredAt >= AWAY);

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
// show is shared by waits and figures: delay before anything appears, then the expression
// (baked clips when motion is allowed, its still otherwise), and rotation over pool every
// ROTATE ms (rotate: false keeps one face). rest: a new expression plays in, REST_LOOPS loops,
// out, and the idle follows for ever (16 §7.1); without rest the expression loops for ever.
// A new expression waits for the shown program's next seam (at most one loop away).
// img: an image already in the slot (the start-up screen's), kept when it shows expr.
function show(slot, { delay = 0, expr, pool = POOL, rotate = true, rest = false, img: given } = {}) {
  const h = { ended: false, expr: expr || nextExpr(pool), slot };
  if (mode === "off") return { ...h, end() {}, set() {} };
  slot.classList.add("archo-slot");
  slot.setAttribute("aria-hidden", "true");
  const timers = [];
  let cur = given || null, url = "", shown = false;
  // run: the program on screen and when it started ({still: true} for a paused idle);
  // want: an expression waiting for a seam; token: the latest plan, so older ones drop out
  let run = null, want = null, token = 0, swap = 0, offscreen = false, ready = "";
  // drop cancels a program that is decoded but not shown yet
  const drop = () => { clearTimeout(swap); if (ready) URL.revokeObjectURL(ready); ready = ""; };
  const later = (ms, f) => timers.push(setTimeout(() => { if (!h.ended) f(); }, ms));
  const reveal = () => decoded(cur).then(() => { if (!h.ended) slot.classList.add("on"); });

  // place makes next the figure's image at once, in place of the old one (both show the
  // same pose there, so nothing fades), and revokes the old program's blob
  const place = (next, nextURL) => {
    const old = cur, oldURL = url;
    cur = next; url = nextURL;
    slot.append(next);
    if (old !== next) old?.remove();
    if (oldURL) URL.revokeObjectURL(oldURL);
    if (!slot.classList.contains("on")) reveal();
  };
  const stillOf = (e) => {
    const mine = ++token, i = image();
    drop();
    i.src = still(e);
    return decoded(i).then(() => { if (mine === token && !h.ended) { place(i, ""); run = null; } });
  };
  // start shows prog at time `at` (performance.now; null: as soon as it is decoded). An aimed
  // program (then: run when it shows) that is not ready by its seam aims at the next one; a
  // program that follows another shows late instead (the other holds its last frame till then).
  const start = (prog, at, then) => {
    const mine = ++token;
    drop();
    Promise.all(prog.steps.map(([c]) => clip(c))).then((cs) => {
      if (mine !== token || h.ended) return;
      const u = URL.createObjectURL(splice(prog.steps.map(([, n], i) => ({ c: cs[i], n })), prog.then ? 1 : 0));
      const i = image();
      i.src = u;
      ready = u;
      return decoded(i).then(() => {
        const go = () => {
          if (mine !== token || h.ended) return;
          ready = "";
          place(i, u);
          run = { prog, t0: now() };
          then?.();
          if (prog.then) start(prog.then, run.t0 + seams(prog.steps).total);
          check();
          if (want) aim();
        };
        const wait = at == null ? 0 : at - now();
        if (mine !== token || h.ended) return;
        if (then && wait < -FRAME) { drop(); aim(); }
        else if (wait > 0) swap = setTimeout(go, wait);
        else go();
      });
    }).catch(() => { if (mine === token && !h.ended) stillOf(prog.steps.map(([c]) => c).find((c) => !/-|idle|bridge/.test(c)) || "idle"); });
  };
  // aim plays `want` from the shown program's first seam at least LEAD ms away. A program
  // that ends before such a seam hands over to what follows it, which aims again.
  const aim = () => {
    if (!want || h.ended || !run || run.still) return;
    const { out, total } = seams(run.prog.steps), soon = now() + LEAD - run.t0;
    let seam = out.find(([t]) => t >= soon);
    if (!seam && !run.prog.then) { // a program that loops for ever: its seams come again
      const k = Math.floor((soon - out[0][0]) / total);
      seam = [k, k + 1].flatMap((n) => out.map(([t, p]) => [t + n * total, p])).find(([t]) => t >= soon);
    }
    if (!seam) { if (run.prog.then) start(run.prog.then, run.t0 + total); return; }
    const e = want;
    start(program(seam[1], e, rest), run.t0 + seam[0], () => { if (want === e) want = null; });
  };
  // check stops a resting figure's idle while she cannot be seen (at once) or the window has
  // been in the background for AWAY ms (at the idle's next seam), and starts it again
  const idleProgram = { steps: [["idle", 1]], then: null };
  const check = () => {
    if (h.ended || !rest || !canMove() || !shown) return;
    const unseen = offscreen || !!globalThis.document?.hidden;
    if (run?.still) {
      if (!unseen && !away()) { run = null; start(idleProgram, null); }
      return;
    }
    if (!run) return;
    const idling = run.prog === idleProgram || (run.prog.steps.length === 1 && run.prog.steps[0][0] === "idle");
    if (!unseen && !(away() && idling)) return;
    const mine = ++token, i = image();
    drop();
    want = null;
    i.src = still("idle");
    const at = unseen ? now() : run.t0 + Math.ceil((now() + LEAD - run.t0) / LOOP) * LOOP;
    decoded(i).then(() => {
      const go = () => { if (mine === token && !h.ended) { place(i, ""); run = { still: true }; } };
      const wait = at - now();
      if (wait > 0) swap = setTimeout(go, wait); else go();
    });
  };
  awake.add(check);
  const io = rest && typeof IntersectionObserver === "function" ? new IntersectionObserver((es) => { offscreen = !es.at(-1).isIntersecting; check(); }) : null;
  io?.observe(slot);

  const first = (e) => rest ? { steps: [[e, REST_LOOPS], [e + "-out", 1], ["bridge", 1]], then: idleProgram } : { steps: [[e, 1]], then: null };
  const appear = () => {
    shown = true;
    const kept = given?.src && given.dataset.expr === h.expr && (given.dataset.live === "1") === canMove();
    if (kept) { reveal(); if (canMove()) run = { prog: { steps: [[h.expr, 1]], then: null }, t0: now() }; }
    else if (!canMove()) stillOf(h.expr);
    else start(first(h.expr), null);
  };
  if (delay) later(delay, appear); else appear();

  h.set = (e) => {
    if (h.ended || !e || e === h.expr) return;
    h.expr = e;
    if (!shown) return;
    if (!canMove()) return stillOf(e);
    want = e;
    if (run?.still) { run = null; want = null; start(program("hub", e, rest), null); return; }
    if (!run) { start(first(e), null); want = null; return; }
    aim();
  };
  if (rotate && !reduced()) {
    const step = () => later(ROTATE, () => { h.set(nextExpr(pool)); step(); });
    later(delay, step);
  }

  h.end = () => {
    if (h.ended) return;
    h.ended = true;
    token++;
    drop();
    timers.forEach(clearTimeout);
    awake.delete(check);
    io?.disconnect();
    slot.classList.remove("on");
    const done = () => { // frees the program's frames
      for (const i of [...slot.children]) if (i.tagName === "IMG") i.remove();
      if (url) URL.revokeObjectURL(url);
    };
    if (slot.isConnected && globalThis.getComputedStyle?.(slot).opacity !== "0") setTimeout(done, 150); else done();
  };
  return h;
}

export const archoWait = (slot, opts = {}) => show(slot, { delay: DELAY, ...opts });

/** archoFigure shows Archo at once with one expression (the first-run guide); its loop
 *  plays a few times on each new expression, then stops on its last frame. */
export const archoFigure = (slot, expr) => show(slot, { expr, rotate: false, rest: true });

// ---- placements on pages (16 §7.1 "Placements") ----------------------------------------
// archoSpot keeps one figure per page across redraws: the page calls peek or stand on every
// draw with the expression for its state, and whether that state is a calm one. A new
// expression plays its loop three times and stops on its last frame (as in the guide). In a
// calm state, left alone, she moves to the next of the page's calm expressions every 45 s,
// again three loops; a state expression (an error, a run) is never cycled away.
// Cycling stops with ui.archo.motion still or off and under reduced motion.
//
//   const spot = archoSpot("feedback", { size: 128, calm: ["neutral", "ponder", "wink"] });
//   spot.peek(card, "think");          // behind card's top edge at its right end
//   spot.stand(column, "thumbsup", true);   // last in column; a calm state: cycling follows
//
// She is never cut: where the space is too narrow or too short, or a box would clip her,
// the slot is hidden (visibility) until it fits again.
const CYCLE = 45000, PEEK_RIGHT = 24, PEEK_OVER = 12;
export const ARCHO_CYCLE = true; // idle cycling in calm states (16 §7.1)
const spots = new Map();

export function archoSpot(name, { size = 140, calm = ["neutral"], cycle = ARCHO_CYCLE } = {}) {
  const old = spots.get(name);
  if (old) return old;
  const slot = document.createElement("div");
  slot.className = "archo-spot";
  slot.style.setProperty("--archo-size", size + "px");
  let figure = null, timer = 0, card = null, turn = 0, base = "", quiet = false;
  const s = { slot, expr: calm[0] };
  const fit = () => {
    if (!slot.isConnected) return;
    if (card) {
      // room above the card for her head: what the page lacks is added as padding on the
      // element before it (the only layout change), and only while she is shown
      const anchor = slot.parentElement, sc = scroller(anchor);
      const pad = parseFloat(anchor.style.paddingTop) || 0;
      const room = card.getBoundingClientRect().top - (sc ? sc.getBoundingClientRect().top - sc.scrollTop : 0) - pad;
      const need = Math.ceil(size * 27 / 40 - PEEK_OVER + 6);
      anchor.style.paddingTop = card.getBoundingClientRect().width >= size * 4 && room < need ? need - room + "px" : "";
      // hang from that element: her bottom PEEK_OVER px below the card's top
      const a = anchor.getBoundingClientRect(), c = card.getBoundingClientRect();
      slot.style.bottom = (a.bottom - c.top - PEEK_OVER) + "px";
      slot.style.right = (a.right - c.right + PEEK_RIGHT) + "px";
    }
    slot.classList.toggle("archo-hide", !fits(slot, card ? size * 4 : 0, card));
  };
  const ro = typeof ResizeObserver === "function" ? new ResizeObserver(() => fit()) : null;
  const put = (host, expr, calmState) => {
    if (mode === "off" || !host) { slot.parentElement?.style.removeProperty("padding-top"); slot.remove(); return; }
    if (slot.parentElement !== host || slot.nextSibling) host.append(slot);
    ro?.disconnect?.(); ro?.observe?.(host); if (card) ro?.observe?.(card);
    setTimeout(fit, 0); // once the page holding host is in the document
    s.set(expr || base || s.expr, calmState);
  };
  s.peek = (c, expr, calmState = false) => {
    const anchor = c?.previousElementSibling;
    if (!anchor) return s.stand(c?.parentElement, expr, calmState);
    card = c;
    slot.classList.add("archo-peek"); slot.classList.remove("archo-stand");
    anchor.classList.add("archo-anchor");
    put(anchor, expr, calmState);
  };
  s.stand = (host, expr, calmState = false) => {
    card = null;
    slot.classList.add("archo-stand"); slot.classList.remove("archo-peek");
    slot.style.bottom = slot.style.right = "";
    put(host, expr, calmState);
  };
  // show puts expression e on the figure (three loops, then her last frame)
  const showExpr = (e) => {
    s.expr = e;
    if (mode === "off" || !slot.parentElement) return; // shown on the next peek or stand
    if (!figure || figure.ended) figure = show(slot, { expr: e, rotate: false, rest: true });
    else figure.set(e);
  };
  // set: the page's state. The same calm state again (a redraw) keeps the cycle where it is.
  s.set = (expr, calmState = false) => {
    if (calmState && quiet && expr === base && figure && !figure.ended) return;
    base = expr; quiet = calmState;
    showExpr(expr);
    clearTimeout(timer);
    if (quiet) timer = setTimeout(idle, CYCLE);
  };
  const idle = () => {
    if (!slot.isConnected) return s.end();
    if (!quiet) return;
    if (cycle && calm.length > 1 && canMove()) {
      const at = calm.indexOf(s.expr);
      turn = at >= 0 ? (at + 1) % calm.length : turn % calm.length;
      showExpr(calm[turn] === s.expr ? calm[(turn + 1) % calm.length] : calm[turn]);
    }
    timer = setTimeout(idle, CYCLE);
  };
  s.end = () => {
    clearTimeout(timer); ro?.disconnect?.();
    figure?.end(); figure = null;
    slot.remove(); spots.delete(name);
  };
  spots.set(name, s);
  return s;
}

const scroller = (e) => { for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) if (/auto|scroll/.test(getComputedStyle(p).overflowY)) return p; return null; };

// fits: the slot lies inside every box that clips it (scrollers only across), and a peek's
// card is wide enough for her beside its own content.
function fits(slot, minCard, card) {
  if (card && card.getBoundingClientRect().width < minCard) return false;
  const r = slot.getBoundingClientRect();
  for (let p = slot.parentElement; p && p !== document.body; p = p.parentElement) {
    const cs = getComputedStyle(p);
    if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
    const b = p.getBoundingClientRect();
    if (r.left < b.left - 1 || r.right > b.right + 1) return false;
    const scrolls = /auto|scroll/.test(cs.overflowY);
    if (scrolls ? r.top + p.scrollTop < b.top - 1 : (r.top < b.top - 1 || r.bottom > b.bottom + 1)) return false;
  }
  return true;
}

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
// index.html holds #boot with an image chosen by boot.js (the loop with motion, else the
// still); it appears after 0.21 s by CSS alone.
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
