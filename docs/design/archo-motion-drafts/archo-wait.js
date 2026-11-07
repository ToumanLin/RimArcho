// Draft of the product's js/archo.js: Archo keeps the player company during a wait.
//
//   const w = archoWait(slot);   // slot: an empty element of fixed size in the loading card
//   ...                          // the read runs
//   w.end();                     // read finished (or failed): the figure fades out
//
// Cost control, in order of what happens during one wait:
//   0 – delay      nothing at all: fast reads never show Archo (no flash, no work)
//   delay          a still WebP fades in (one <img>, ~18 KB, cached after the first time)
//   liveAfter      only if the wait goes on, motion is allowed and WebGL2 works: the one
//                  shared puppet is mounted (created the first time, ~0.15 ms CPU per frame
//                  with the small package) and cross-fades over the still
//   end            fade out; the puppet's canvas leaves the page and stops drawing
//   end + idle     the puppet is destroyed: textures and the WebGL context are released
// Expressions rotate round-robin over POOL, one per wait, and every `rotate` ms in a long one.

export const POOL = ['neutral', 'smile', 'laugh', 'wink', 'think', 'ponder', 'yum'];

export const config = {
  mode: 'live',        // live | still | off   (settings › interface, ui.archo.motion)
  delay: 400,          // ms before anything shows
  liveAfter: 1200,     // ms before the still becomes the live figure
  rotate: 4000,        // ms per expression in a long wait
  idle: 10000,         // ms the puppet is kept after a wait, so back-to-back reads reuse it
  size: 120,           // CSS px
  view: 'shoulders',
  base: '/archo/',
  pack: 'archo-s/',    // half-resolution package for figures up to 160 px: 238 KB, 2.4 MB of textures
  reducedMotion: () => matchMedia('(prefers-reduced-motion: reduce)').matches,
};

// what the perf readout shows
export const stats = { puppets: 0, created: 0, destroyed: 0, frames: 0, frameMs: 0, live: false, textureKB: 0 };

let turn = 0;              // round-robin position, carried across waits
let puppet = null;         // the one live figure
let creating = null;       // its creation promise
let idleTimer = 0;
const still = (expr) => `${config.base}${config.pack}stills/${expr}@${config.size <= 120 ? 240 : 360}.webp`;

const webgl2 = (() => { let ok; return () => ok ??= !!document.createElement('canvas').getContext('webgl2'); })();

async function ensurePuppet() {
  clearTimeout(idleTimer);
  if (puppet) return puppet;
  creating ??= (async () => {
    const { ArchoPuppet } = await import(`${config.base}archo-gl.js`);
    const parking = document.createElement('div');
    const p = await ArchoPuppet.create(parking, { base: config.base + config.pack, size: config.size, view: config.view, fps: 24 });
    p.unmount();
    // measure the CPU side of each frame for the readout
    const render = p.render.bind(p);
    p.render = (pose) => { const t = performance.now(); render(pose); stats.frames++; stats.frameMs = stats.frameMs * 0.95 + (performance.now() - t) * 0.05; };
    stats.textureKB = Math.round(textureBytes(p) / 1024);
    stats.created++; stats.puppets = 1;
    puppet = p; creating = null;
    return p;
  })();
  return creating;
}

function textureBytes(p) {
  let n = 0;
  for (const b of [p.items, p.patchItems, p.gestureItems, p.fxItems]) for (const it of Object.values(b)) n += it.box.w * it.box.h * 4 * 4 / 3;
  return n;
}

function release() {
  clearTimeout(idleTimer);
  idleTimer = setTimeout(() => {
    if (!puppet || puppet.el?.isConnected && puppet.cv.isConnected) return;
    puppet.destroy(); puppet = null; stats.puppets = 0; stats.destroyed++; stats.live = false;
  }, config.idle);
}

/** Start keeping the player company in `slot`; returns { end(), expr }. */
export function archoWait(slot, { pool = POOL } = {}) {
  const h = { ended: false, expr: pool[turn++ % pool.length] };
  if (config.mode === 'off') return { end() {}, expr: null };
  slot.classList.add('archo-slot');
  slot.style.setProperty('--archo-size', config.size + 'px');
  const img = new Image();
  img.className = 'archo-still'; img.alt = ''; img.decoding = 'async';
  img.width = img.height = config.size;
  const timers = [];
  const later = (ms, f) => timers.push(setTimeout(() => { if (!h.ended) f(); }, ms));
  const reduced = config.reducedMotion();

  later(config.delay, () => {
    img.src = still(h.expr);
    slot.append(img);
    img.decode().catch(() => {}).then(() => { if (!h.ended) slot.classList.add('on'); });
  });

  const live = config.mode === 'live' && !reduced && webgl2();
  if (live) later(config.liveAfter, async () => {
    const p = await ensurePuppet();
    if (h.ended) { release(); return; }
    if (p.cv.isConnected) return;   // another wait has the figure: this one keeps its still
    p.setExpression(h.expr);
    p.mount(slot);
    stats.live = true;
    p.cv.classList.add('archo-live');
    requestAnimationFrame(() => slot.classList.add('live'));
  });

  // a long wait moves on to the next expression
  if (!reduced) {
    const step = () => later(config.rotate, () => {
      h.expr = pool[turn++ % pool.length];
      if (slot.classList.contains('live') && puppet) puppet.setExpression(h.expr);
      else { const next = still(h.expr); const pre = new Image(); pre.src = next; pre.decode().catch(() => {}).then(() => { if (!h.ended) img.src = next; }); }
      step();
    });
    step();
  }

  h.end = () => {
    if (h.ended) return; h.ended = true;
    timers.forEach(clearTimeout);
    slot.classList.remove('on');
    const done = () => {
      if (puppet && puppet.cv.parentNode === slot) { puppet.unmount(); stats.live = false; }
      img.remove(); slot.classList.remove('live');
      if (puppet || creating) release();
    };
    if (slot.isConnected && getComputedStyle(slot).opacity !== '0') setTimeout(done, 150); else done();
  };
  return h;
}

/** Render one expression to an image (gallery stills drawn by the live renderer). */
export async function snapshot(expr, { size = 120, view = 'shoulders', at = 1.2, state } = {}) {
  const { ArchoPuppet } = await import(`${config.base}archo-gl.js`);
  const host = document.createElement('div');
  const p = await ArchoPuppet.create(host, { base: config.base + config.pack, size, view, manual: true, dpr: 2, state });
  if (expr) p.motion.setExpression(expr);
  for (let t = 0; t < at; t += 1 / 30) p.advance(1 / 30);
  const url = p.cv.toDataURL('image/png');
  p.destroy();
  return url;
}

/** Drop the shared figure now (its size changed). */
export function resetFigure() {
  clearTimeout(idleTimer);
  if (puppet) { puppet.destroy(); puppet = null; stats.puppets = 0; stats.destroyed++; stats.live = false; }
}

/** Render several stills with one figure: [{expr} | {state}] -> data URLs. */
export async function snapshots(items, { size = 84, view = 'shoulders', at = 1.4 } = {}) {
  const { ArchoPuppet } = await import(`${config.base}archo-gl.js`);
  const p = await ArchoPuppet.create(document.createElement('div'), { base: config.base + config.pack, size, view, manual: true, dpr: 2 });
  const out = [];
  for (const it of items) {
    if (it.state) p.motion.setState(it.state); else { p.motion.setState('idle'); p.motion.setExpression(it.expr); }
    for (let t = 0; t < at; t += 1 / 30) p.advance(1 / 30);
    out.push(p.cv.toDataURL('image/png'));
  }
  p.destroy();
  return out;
}
