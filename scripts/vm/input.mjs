// Real Windows input in the test VM: the host side of input-agent.ps1 (started by
// `vm.ps1 input start`). Unlike cdp.mjs, whose events go to the page only, this input goes
// through Windows: the native window frame (drag, snap, Alt+F4), WebView2 as a real mouse
// and keyboard, the clipboard, Windows Terminal (the TUI), dialogs. Node 22+, no dependencies.
//
//   node scripts/vm/input.mjs <ip> screen | windows [process] | focus <process>
//   node scripts/vm/input.mjs <ip> click <x> <y> [button] | move <x> <y> | drag <x1> <y1> <x2> <y2>
//   node scripts/vm/input.mjs <ip> key <combo> [repeat]   (ctrl+shift+k, win+up, alt+f4, enter)
//   node scripts/vm/input.mjs <ip> type <text> | clip | setclip <text> | text <process>
//   node scripts/vm/input.mjs <ip> realclick <selector> [button] [ctrl+...]   (needs DevTools too)
//   node scripts/vm/input.mjs <ip> realdrag <from-selector> <to-selector> [holdMs]
//
// <ip> is the guest IP (`vm.ps1 status`); ports default to 9231 (input) and 9223 (DevTools).
import net from 'node:net';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { connect as connectCDP, sleep } from './cdp.mjs';

export const INPUT_PORT = 9231;
export const TOKEN_FILE = process.env.RIMARCHO_VM_INPUT_TOKEN_FILE || 'D:\\RimArcho-test\\transfer\\rimarcho\\Player-Test\\input-token.txt';

// Virtual keys. ext: the extended-key flag, which Windows needs for these to read as the
// navigation keys and not the numpad ones.
const KEYS = {
  ctrl: [0x11], control: [0x11], shift: [0x10], alt: [0x12], win: [0x5b, true], meta: [0x5b, true],
  enter: [0x0d], return: [0x0d], esc: [0x1b], escape: [0x1b], tab: [0x09], space: [0x20], backspace: [0x08],
  delete: [0x2e, true], del: [0x2e, true], insert: [0x2d, true], home: [0x24, true], end: [0x23, true],
  pageup: [0x21, true], pagedown: [0x22, true], left: [0x25, true], up: [0x26, true], right: [0x27, true], down: [0x28, true],
  arrowleft: [0x25, true], arrowup: [0x26, true], arrowright: [0x27, true], arrowdown: [0x28, true],
  apps: [0x5d, true], contextmenu: [0x5d, true], printscreen: [0x2c, true], capslock: [0x14],
  ';': [0xba], semicolon: [0xba], '=': [0xbb], equal: [0xbb], ',': [0xbc], comma: [0xbc], '-': [0xbd], minus: [0xbd],
  '.': [0xbe], period: [0xbe], '/': [0xbf], slash: [0xbf], '`': [0xc0], backquote: [0xc0], '[': [0xdb], bracketleft: [0xdb],
  '\\': [0xdc], backslash: [0xdc], ']': [0xdd], bracketright: [0xdd], "'": [0xde], quote: [0xde],
};
const MODIFIERS = new Set([0x11, 0x10, 0x12, 0x5b]);

// One key name to {vk, ext}. Accepts the names above, a letter or digit, f1–f24, and the
// DOM codes cdp.mjs uses (KeyK, Digit2, ArrowLeft, Escape).
export function keyOf(name) {
  let n = String(name).trim();
  if (/^Key[A-Z]$/.test(n)) n = n.slice(3);
  else if (/^Digit[0-9]$/.test(n)) n = n.slice(5);
  const lower = n.toLowerCase();
  if (KEYS[lower]) return { vk: KEYS[lower][0], ext: !!KEYS[lower][1] };
  if (KEYS[n]) return { vk: KEYS[n][0], ext: !!KEYS[n][1] };
  if (/^[a-z0-9]$/.test(lower)) return { vk: lower.toUpperCase().charCodeAt(0), ext: false };
  const f = /^f([1-9]|1[0-9]|2[0-4])$/.exec(lower);
  if (f) return { vk: 0x6f + Number(f[1]), ext: false };
  throw Error(`unknown key ${name}`);
}

// "ctrl+shift+k" to the key events that press it: modifiers down in order, the key down and
// up, modifiers up in reverse. A lone "+" is the plus key's own name: "ctrl++" is not supported;
// write "ctrl+shift+=".
export function comboEvents(combo, repeat = 1) {
  const parts = String(combo).split('+').map(s => s.trim()).filter(Boolean);
  if (!parts.length) throw Error('empty key combo');
  const keys = parts.map(keyOf);
  const last = keys.pop();
  for (const k of keys) if (!MODIFIERS.has(k.vk)) throw Error(`${combo}: only the last key may be a non-modifier`);
  const ev = keys.map(k => ({ ...k, up: false }));
  for (let i = 0; i < repeat; i++) ev.push({ ...last, up: false }, { ...last, up: true });
  for (const k of [...keys].reverse()) ev.push({ ...k, up: true });
  return ev;
}

// Points along a drag, endpoints included. The first step is a few pixels so the page sees a
// drag start (most drag code waits for a small threshold) before the long move.
export function dragPath(x1, y1, x2, y2, steps = 20) {
  const pts = [[x1, y1]];
  const dx = x2 - x1, dy = y2 - y1, len = Math.hypot(dx, dy) || 1;
  pts.push([Math.round(x1 + 6 * dx / len), Math.round(y1 + 6 * dy / len)]);
  for (let i = 1; i <= steps; i++) pts.push([Math.round(x1 + dx * i / steps), Math.round(y1 + dy * i / steps)]);
  return pts.filter((p, i) => i === 0 || p[0] !== pts[i - 1][0] || p[1] !== pts[i - 1][1]);
}

// Page (CSS) coordinates to screen pixels from two probes: screen point s, where the page
// saw it at c. One axis at a time: screen = offset + scale * css.
export function solveAxis(s1, c1, s2, c2) {
  if (c1 === c2) throw Error('calibration points share a page coordinate');
  const scale = (s2 - s1) / (c2 - c1);
  return { scale, offset: s1 - scale * c1 };
}
export function toScreen(cal, x, y) {
  return [Math.round(cal.x.offset + cal.x.scale * x), Math.round(cal.y.offset + cal.y.scale * y)];
}

export function parseAddr(addr, port) {
  const m = /^(.+?)(?::(\d+))?$/.exec(addr);
  return { host: m[1], port: m[2] ? Number(m[2]) : port };
}

export async function connectInput(addr, { token, timeoutMs = 15000 } = {}) {
  const { host, port } = parseAddr(addr, INPUT_PORT);
  token ??= process.env.RIMARCHO_VM_INPUT_TOKEN || readFileSync(TOKEN_FILE, 'utf8').trim();
  const sock = net.connect({ host, port });
  sock.setNoDelay(true);
  await new Promise((ok, fail) => {
    const timer = setTimeout(() => { sock.destroy(); fail(Error(`input agent at ${host}:${port} timed out (vm.ps1 input start; vm.ps1 forward)`)); }, timeoutMs);
    sock.once('connect', () => { clearTimeout(timer); ok(); });
    sock.once('error', e => { clearTimeout(timer); fail(Error(`input agent at ${host}:${port}: ${e.message} (vm.ps1 input start; vm.ps1 forward)`)); });
  });
  let seq = 0, buf = '';
  const pending = new Map();
  const failAll = why => { for (const p of pending.values()) { clearTimeout(p.timer); p.fail(Error(why)); } pending.clear(); };
  sock.setEncoding('utf8');
  sock.on('data', d => {
    buf += d;
    let i;
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i).trim(); buf = buf.slice(i + 1);
      if (!line) continue;
      const r = JSON.parse(line), p = pending.get(r.id);
      if (!p) continue;
      clearTimeout(p.timer); pending.delete(r.id);
      r.ok ? p.ok(r) : p.fail(Error(`${p.cmd}: ${r.error}`));
    }
  });
  sock.on('close', () => failAll('input agent disconnected'));
  sock.on('error', () => {});
  const call = (cmd, args = {}, ms = timeoutMs) => new Promise((ok, fail) => {
    if (sock.destroyed) return fail(Error('input agent disconnected'));
    const id = ++seq;
    const timer = setTimeout(() => { pending.delete(id); fail(Error(`${cmd} timed out after ${ms}ms`)); }, ms);
    pending.set(id, { ok, fail, timer, cmd });
    sock.write(JSON.stringify({ id, cmd, ...args }) + '\n');
  });
  await call('hello', { token });

  const keyEvents = async (events, delayMs = 15) => {
    for (const e of events) { await call('key', e); if (delayMs) await sleep(delayMs); }
  };
  // Hold modifiers (['ctrl']) while fn runs; released even when fn throws.
  const hold = async (mods, fn) => {
    const keys = (mods || []).map(keyOf);
    for (const k of keys) await call('key', { ...k, up: false });
    try { return await fn(); }
    finally { for (const k of [...keys].reverse()) await call('key', { ...k, up: true }).catch(() => {}); }
  };
  const move = (x, y) => call('move', { x, y });
  const button = (b, down) => call('button', { button: b, down });
  const click = async (x, y, { button: b = 'left', count = 1, mods = [] } = {}) => hold(mods, async () => {
    await move(x, y); await sleep(30);
    for (let i = 0; i < count; i++) { await button(b, true); await sleep(30); await button(b, false); await sleep(40); }
  });
  // A held drag: press, move along the path, hold at the end (autoscroll, hover targets), release.
  // during(x, y) runs at the end before release when given, e.g. to press Esc or check the page.
  const drag = async (x1, y1, x2, y2, { steps = 20, ms = 400, holdMs = 150, button: b = 'left', mods = [], during } = {}) => hold(mods, async () => {
    const path = dragPath(x1, y1, x2, y2, steps);
    await move(x1, y1); await sleep(50);
    await button(b, true);
    try {
      await sleep(60);
      for (const [x, y] of path.slice(1)) { await move(x, y); await sleep(ms / path.length); }
      if (holdMs) await sleep(holdMs);
      if (during) await during(x2, y2);
    } finally { await button(b, false).catch(() => {}); }
  });
  const key = (combo, repeat = 1) => keyEvents(comboEvents(combo, repeat));
  const type = (text, delay = 5) => call('unicode', { text, delay }, Math.max(timeoutMs, text.length * (delay + 5) + 5000));
  const windows = async process => (await call('windows', { process })).windows;
  // The process's largest visible top-level window.
  const windowOf = async process => {
    const ws = (await windows(process)).filter(w => !w.minimized);
    if (!ws.length) throw Error(`no visible window of ${process}`);
    return ws.sort((a, b) => b.rect[2] * b.rect[3] - a.rect[2] * a.rect[3])[0];
  };
  const focus = async process => {
    const w = await windowOf(process);
    if (!(await call('focus', { hwnd: w.hwnd })).foreground) throw Error(`${process} did not come to the foreground`);
    return w;
  };
  const text = async process => (await call('uiatext', { pid: (await windowOf(process)).pid })).texts;
  return {
    call, move, button, click, drag, key, keyEvents, hold, type, windows, windowOf, focus, text,
    screen: () => call('screen'),
    clip: async () => (await call('clip')).text,
    setclip: t => call('setclip', { text: t }),
    release: () => call('release'),
    close: () => { failAll('input closed'); sock.end(); },
  };
}

// Page coordinates to screen pixels for the RimArcho window: move the real cursor to two
// points inside the window and read where the page saw them. This needs no knowledge of the
// frame, the WebView's place in the window, page zoom or display scaling, and it fails loudly
// when the page is covered or the window is somewhere else.
export async function calibrate(cdp, inp, { process: proc = 'RimArcho' } = {}) {
  const w = await inp.focus(proc); // a window in front of RimArcho would take the probe moves
  const [L, T, W, H] = w.client[2] ? [w.client[0], w.client[1], w.client[2], w.client[3]] : w.rect;
  await cdp.evaluate(`(() => { if (!window.__archoInputProbe) { window.__archoInputProbe = true; addEventListener('mousemove', e => { window.__archoInputLast = [e.clientX, e.clientY]; }, true); } window.__archoInputLast = null; })()`);
  const probe = async (sx, sy) => {
    await inp.move(sx - 4, sy - 4); // a move onto the cursor's own position fires no event
    await sleep(30);
    await cdp.evaluate('window.__archoInputLast = null');
    await inp.move(sx, sy);
    for (let i = 0; i < 40; i++) {
      const p = await cdp.evaluate('window.__archoInputLast');
      if (p) return p;
      await sleep(50);
    }
    throw Error(`the page saw no mouse move at screen ${sx},${sy} (is another window over RimArcho? input.mjs ${'<ip>'} windows)`);
  };
  const a = [Math.round(L + W * 0.35), Math.round(T + H * 0.45)];
  const b = [Math.round(L + W * 0.65), Math.round(T + H * 0.75)];
  const pa = await probe(...a), pb = await probe(...b);
  const cal = { x: solveAxis(a[0], pa[0], b[0], pb[0]), y: solveAxis(a[1], pa[1], b[1], pb[1]), window: w };
  if (!(cal.x.scale > 0.4 && cal.x.scale < 5) || Math.abs(cal.x.scale - cal.y.scale) > 0.05)
    throw Error(`calibration looks wrong: scale ${cal.x.scale.toFixed(3)} × ${cal.y.scale.toFixed(3)}`);
  return cal;
}

// The centre of the first hittable element matching selector, in page coordinates.
export async function pagePoint(cdp, selector) {
  const point = await cdp.evaluate(`(() => { for (const e of document.querySelectorAll(${JSON.stringify(selector)})) { if (e.disabled || e.closest('[inert]')) continue; e.scrollIntoView({ block: 'nearest', inline: 'nearest' }); const b = e.getBoundingClientRect(); const x = b.x + b.width / 2, y = b.y + b.height / 2; const hit = document.elementFromPoint(x, y); if (b.width && b.height && (hit === e || e.contains(hit))) return [x, y]; } return null; })()`);
  if (!point) throw Error(`control not hittable: ${selector}`);
  return point;
}

// Both connections plus a calibration, for tests: real.click('#nav [data-id=home]').
export async function connectReal(ip, { cdpPort = 9223, inputPort = INPUT_PORT, token } = {}) {
  const cdp = await connectCDP(`${ip}:${cdpPort}`);
  let inp;
  try {
    inp = await connectInput(`${ip}:${inputPort}`, { token });
    let cal = await calibrate(cdp, inp);
    const at = async selector => toScreen(cal, ...(await pagePoint(cdp, selector)));
    return {
      cdp, input: inp,
      get calibration() { return cal; },
      recalibrate: async () => (cal = await calibrate(cdp, inp)),
      at,
      click: async (selector, opts) => { const p = await at(selector); await inp.focus('RimArcho').catch(() => {}); await inp.click(...p, opts); return p; },
      drag: async (from, to, opts = {}) => {
        const a = await at(from);
        const b = typeof to === 'string' ? await at(to) : [a[0] + (to.dx || 0), a[1] + (to.dy || 0)];
        await inp.drag(...a, ...b, opts);
        return [a, b];
      },
      close: () => { inp.close(); cdp.close(); },
    };
  } catch (e) { inp?.close(); cdp.close(); throw e; }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [addr, cmd, ...a] = process.argv.slice(2);
  if (!addr || !cmd) { console.error('usage: input.mjs <ip>[:port] screen|windows|focus|click|move|drag|key|type|clip|setclip|text|realclick|realdrag ...'); process.exit(2); }
  const ip = parseAddr(addr, INPUT_PORT).host;
  const out = v => console.log(typeof v === 'string' ? v : JSON.stringify(v, null, 1));
  if (cmd === 'realclick' || cmd === 'realdrag') {
    const r = await connectReal(ip, { inputPort: parseAddr(addr, INPUT_PORT).port });
    try {
      if (cmd === 'realclick') out({ screen: await r.click(a[0], { button: a[1] || 'left', mods: a[2] ? a[2].split('+') : [] }) });
      else out({ path: await r.drag(a[0], a[1], { holdMs: a[2] ? Number(a[2]) : 150 }) });
    } finally { r.close(); }
  } else {
    const c = await connectInput(addr);
    try {
      switch (cmd) {
        case 'ping': out('ok'); break;
        case 'screen': out(await c.screen()); break;
        case 'windows': out(await c.windows(a[0] || '')); break;
        case 'focus': out(await c.focus(a[0])); break;
        case 'move': out((await c.move(+a[0], +a[1])).cursor); break;
        case 'click': await c.click(+a[0], +a[1], { button: a[2] || 'left', count: a[3] ? +a[3] : 1 }); break;
        case 'drag': await c.drag(+a[0], +a[1], +a[2], +a[3]); break;
        case 'key': await c.key(a[0], a[1] ? +a[1] : 1); break;
        case 'type': await c.type(a.join(' ')); break;
        case 'clip': out(await c.clip()); break;
        case 'setclip': await c.setclip(a.join(' ')); break;
        case 'text': for (const t of await c.text(a[0])) out(t); break;
        case 'release': await c.release(); break;
        default: throw Error(`unknown command ${cmd}`);
      }
    } finally { c.close(); }
  }
}
