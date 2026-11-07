// Archo motion drafts: example data only, nothing is read or written.
import { icon } from '/app/js/icons.js';
import { archoWait, config, stats, snapshot, snapshots, resetFigure } from './archo-wait.js';

const $ = (s, r = document) => r.querySelector(s);
const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; };
const qs = new URLSearchParams(location.search);
const state = { scene: qs.get('scene') || 'page', layout: qs.get('layout') || 'a' };
const EMOJI = { neutral: '🙂', smile: '😊', laugh: '😆', wink: '😜', think: '🧐', ponder: '🤔', yum: '😋' };

// ---- review bar ----
function seg(box, items, get, set) {
  const bs = items.map(([v, label]) => {
    const b = el('button', '', label);
    b.onclick = () => { set(v); bs.forEach((x, i) => x.classList.toggle('on', items[i][0] === get())); };
    b.classList.toggle('on', v === get());
    $(box).append(b); return b;
  });
}
seg('#g-scene', [['page', 'page loading'], ['task', 'long tasks'], ['other', 'other places']], () => state.scene, (v) => { state.scene = v; draw(); });
seg('#g-mode', [['live', 'live'], ['still', 'still'], ['off', 'off']], () => config.mode, (v) => { config.mode = v; draw(); });
seg('#g-size', [[96, '96'], [120, '120'], [160, '160']], () => config.size, (v) => { config.size = v; resetPuppet(); draw(); });
seg('#g-layout', [['a', 'A centred'], ['b', 'B beside skeleton']], () => state.layout, (v) => { state.layout = v; draw(); });
let reduced = false;
config.reducedMotion = () => reduced || matchMedia('(prefers-reduced-motion: reduce)').matches;
const misc = $('#g-misc');
const rb = el('button', '', 'simulate reduced motion'); rb.onclick = () => { reduced = !reduced; rb.classList.toggle('on', reduced); draw(); }; misc.append(rb);
const tb = el('button', '', 'dark / light');
tb.onclick = () => globalThis.archoTheme.apply(document.documentElement.dataset.scheme === 'dark' ? 'light' : 'dark');
misc.append(tb);

// the shared figure is sized at creation; a size change starts over
function resetPuppet() { endAll(); resetFigure(); }

setInterval(() => {
  $('#perf').textContent = `live ${stats.puppets}/1   CPU/frame ${stats.frameMs.toFixed(2)} ms   textures ${stats.puppets ? (stats.textureKB / 1024).toFixed(1) + ' MB' : '0'}   created ${stats.created}  released ${stats.destroyed}`;
}, 250);

// ---- shell ----
const NAV = [['Start', [['home', 'Start']]], ['Game', [['plan', 'current list'], ['needs', 'Can it run'], ['log', 'game log'], ['saves', 'saves']]],
  ['Mod', [['library', 'Mod library'], ['textures', 'textures'], ['packs', 'packs']]], ['', [['tasks', 'tasks'], ['settings', 'settings']]]];
function side(cur) {
  const s = $('#side'); s.replaceChildren();
  for (const [g, items] of NAV) {
    if (g) s.append(el('div', 'dx-grp', g));
    for (const [id, label] of items) {
      const b = el('button', 'nav-item' + (id === cur ? ' on' : ''));
      const ic = el('span', 'ic'); ic.innerHTML = icon[id] || icon.plan;
      b.append(ic, el('span', 'lbl', label)); s.append(b);
    }
  }
}

// ---- timeline of one wait ----
let tlTimer = 0;
function timeline(t0, total) {
  clearInterval(tlTimer);
  const tl = $('#timeline');
  const marks = [[0, 'read starts'], [config.delay, `${config.delay / 1000} s still fades in`],
    ...(config.mode === 'live' && !config.reducedMotion() ? [[config.liveAfter, `${config.liveAfter / 1000} s becomes live`]] : []),
    [config.delay + config.rotate, `every ${config.rotate / 1000} s a new expression`]];
  const paint = () => {
    const t = performance.now() - t0;
    tl.replaceChildren(...marks.map(([at, label]) => el('span', at <= t && (total == null || at <= total) ? 'now' : '', label)),
      el('b', '', total != null && t >= total ? `read done at ${(total / 1000).toFixed(1)} s, fades out, released after ${config.idle / 1000} s` : `waited ${(t / 1000).toFixed(1)} s`));
    if (total != null && t >= total + 200) clearInterval(tlTimer);
  };
  paint(); tlTimer = setInterval(paint, 100);
}

// ---- scene: page loading ----
const waits = new Set();
function endAll() { for (const w of waits) w.end(); waits.clear(); }

function loadingCard(say) {
  const card = el('section', 'card page-loading');
  const slot = el('div');
  const skel = el('div', 'skel');
  skel.setAttribute('role', 'progressbar'); skel.setAttribute('aria-label', say);
  for (const w of [88, 72, 94, 60, 80, 66]) { const b = el('span', 'bar'); b.style.width = w + '%'; skel.append(b); }
  if (state.layout === 'a') {
    const box = el('div', 'loading-a'); box.append(slot, skel); card.append(box);
  } else {
    const box = el('div', 'loading-b'); box.append(slot, skel); card.append(box);
  }
  return { card, slot };
}

function pageScene() {
  side('library'); $('#crumb').textContent = 'Mod library';
  const scr = $('#scr'); scr.replaceChildren();
  const ctl = el('div', 'dx-note');
  ctl.append('Demo: open the Mod library; the read takes ');
  for (const [ms, label] of [[200, '0.2 s'], [1000, '1 s'], [4000, '4 s'], [15000, '15 s'], [-3000, 'fails after 3 s']]) {
    const b = el('button', 'btn quiet', label); b.onclick = () => open(ms); ctl.append(b);
  }
  scr.append(ctl);
  const host = el('div'); scr.append(host);
  let read = 0;
  const open = (ms) => {
    endAll();
    const mine = ++read;
    const total = Math.abs(ms);
    const { card, slot } = loadingCard('reading the Mod library…');
    host.replaceChildren(card);
    const w = archoWait(slot); waits.add(w);
    $('#status').textContent = w.expr ? `this time: ${EMOJI[w.expr]} ${w.expr}` : '';
    timeline(performance.now(), total);
    setTimeout(() => {
      if (mine !== read) return;   // a newer read replaced this one
      w.end(); waits.delete(w);
      setTimeout(() => {
        if (mine !== read) return;
        if (ms < 0) host.replaceChildren(Object.assign(el('section', 'card page-err'), { textContent: 'read failed: the database is busy, r to retry' }));
        else {
          const c = el('section', 'card page-real');
          ['Harmony', 'HugsLib', 'Vanilla Expanded Framework', 'Humanoid Alien Races', 'Prison Labor', 'RimHUD', 'Dubs Bad Hygiene']
            .forEach((n, i) => { const r = el('div', 'dx-row'); r.append(el('span', 'dx-n', String(i + 1)), el('span', '', n)); c.append(r); });
          host.replaceChildren(c);
        }
      }, 150);
    }, total);
  };
  open(4000);
}

// ---- scene: long tasks ----
function taskScene() {
  side('tasks'); $('#crumb').textContent = 'tasks';
  const scr = $('#scr'); scr.replaceChildren();
  scr.append(el('div', 'dx-note', 'Demo: on the tasks page Archo sits left of a running task; one task is live, the others still. On finish or failure a reaction shows for 2.4 s, then fades.'));
  const run = (title, total, fail, pool) => {
    const card = el('section', 'card task');
    const slot = el('div');
    const body = el('div', 'dx-body');
    const t = el('div', 'dx-t', title), m = el('div', 'dx-m', 'preparing…');
    const prog = el('div', 'prog'); const bar = el('div', 'bar'); const fill = el('i'); bar.append(fill); prog.append(bar);
    body.append(t, m, prog); card.append(slot, body); scr.append(card);
    const w = archoWait(slot, { pool }); waits.add(w);
    const t0 = performance.now();
    const scene = sceneSeq;
    const tick = setInterval(() => {
      if (scene !== sceneSeq) { clearInterval(tick); return; }
      const u = Math.min(1, (performance.now() - t0) / total);
      fill.style.width = (u * 100).toFixed(1) + '%';
      m.textContent = fail && u > 0.6 ? 'the 3rd Mod failed to download' : `${Math.round(u * 1354)} / 1354`;
      if (fail && u > 0.6) { bar.classList.add('bad'); finish(false); }
      else if (u >= 1) finish(true);
    }, 100);
    let done = false;
    async function finish(ok) {
      if (done) return; done = true; clearInterval(tick);
      m.textContent = ok ? 'done: saves 1.8 GB of VRAM' : 'failed: the 3rd Mod failed to download; retry or skip';
      // reaction: the shared figure plays a state; stills use a one-off snapshot
      w.end(); waits.delete(w);
      const img = new Image(); img.width = img.height = config.size;
      img.src = await snapshot(null, { size: config.size, state: ok ? 'success' : 'error', at: 0.7 });
      const s2 = el('div', 'archo-slot on'); s2.style.setProperty('--archo-size', config.size + 'px'); s2.append(img);
      setTimeout(() => slot.replaceWith(s2), 160);
      setTimeout(() => s2.classList.remove('on'), 2400);
    }
  };
  run('count textures', 18000, false, ['think', 'ponder']);
  run('download 3 Mods', 9000, true, ['think', 'ponder']);
  timeline(performance.now(), null);
}

// ---- scene: other places ----
const OTHER = [
  ['page', 'page loading', 'every shimmer placeholder (Mod library, log, saves, details…), only after 0.4 s', 'neutral smile laugh wink think ponder yum in turn, live', null],
  ['task', 'long background tasks', 'texture count / conversion, log analysis, downloads, pack builds, scans', '🧐 🤔 alternating, live; one live at a time', 'think'],
  ['ok', 'task finished', 'tasks page and task rows, 2.4 s then fades', '😁 with a hop (success)', 'smug'],
  ['fail', 'task failed / page read failed', 'left of the error card, still', '😭 (task) / 😳 (page read)', 'cry'],
  ['empty', 'empty states', 'one-sentence empty states (staging empty, no logs, no saves), a small still', '😪', 'sleepy'],
  ['guide', 'first-run guide', 'welcome and scanning, a nod when the scan is done', '😜 → 🧐 → 🫡', 'wink'],
  ['log', 'ask Archo to analyze', 'the game log page card: running / found problems / nothing found', '🤔 / 😳 / 😊', 'ponder'],
  ['panel', 'Archo panel (1.1)', 'the large figure atop the panel, following the agent state', 'all 16 states', 'salute'],
  ['about', 'settings › about', 'a click on Archo changes the expression, an easter egg', '🤪 😆 🥰 at random', 'zany'],
];
async function otherScene() {
  side('home'); $('#crumb').textContent = 'other places';
  const scr = $('#scr'); scr.replaceChildren();
  scr.append(el('div', 'dx-note', 'Besides loading, these places suit Archo. The images are stills drawn by the live renderer.'));
  const g = el('div', 'gallery'); scr.append(g);
  const imgs = OTHER.map(([, title, where, expr]) => {
    const c = el('section', 'card gcard');
    const img = new Image(); img.alt = '';
    const t = el('div'); t.append(el('div', 'dx-t', title), el('div', 'dx-m', where), el('div', 'dx-k', expr));
    c.append(img, t); g.append(c); return img;
  });
  const urls = await snapshots(OTHER.map(([, , , , e]) => e ? { expr: e } : { expr: 'smile' }));
  urls.forEach((u, i) => { imgs[i].src = u; });
  $('#timeline').replaceChildren(el('span', '', 'Stills come from the same assets; motion is only for waits and the Archo panel.'));
}

let sceneSeq = 0;
function draw() {
  sceneSeq++;
  endAll(); clearInterval(tlTimer);
  history.replaceState(null, '', `?scene=${state.scene}&layout=${state.layout}`);
  ({ page: pageScene, task: taskScene, other: otherScene })[state.scene]();
}
draw();
