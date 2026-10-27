// Real WebView2 input. Node 22+, no dependencies. Runtime.enable stays off (see smoke.mjs).
import { writeFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
export const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const VK = { Delete: 46, Space: 32, Backslash: 220, ArrowLeft: 37, ArrowUp: 38, ArrowRight: 39, ArrowDown: 40, Enter: 13, Escape: 27, Tab: 9, Backspace: 8, Home: 36, End: 35, PageUp: 33, PageDown: 34, F10: 121 };
export async function connect(addr, { timeoutMs = 15000 } = {}) {
  const targets = await (await fetch(`http://${addr}/json/list`, { signal: AbortSignal.timeout(timeoutMs) })).json();
  const page = targets.find(t => t.type === 'page' && !t.url.startsWith('devtools:'));
  if (!page) throw Error('driver-not-ready: no page target');
  const ws = new WebSocket(page.webSocketDebuggerUrl.replace(/ws:\/\/[^/]+/, `ws://${addr}`));
  await new Promise((ok, fail) => {
    const timer = setTimeout(() => { ws.close(); fail(Error('DevTools connection timed out')); }, timeoutMs);
    ws.onopen = () => { clearTimeout(timer); ok(); };
    ws.onerror = () => { clearTimeout(timer); fail(Error('DevTools WebSocket failed')); };
  });
  let seq = 0;
  const pending = new Map(), listeners = [];
  const rejectAll = why => { for (const p of pending.values()) { clearTimeout(p.timer); p.fail(Error(why)); } pending.clear(); };
  ws.onclose = () => rejectAll('DevTools disconnected');
  ws.onerror = () => rejectAll('DevTools WebSocket error');
  ws.onmessage = m => {
    const d = JSON.parse(m.data), p = pending.get(d.id);
    if (p) { clearTimeout(p.timer); pending.delete(d.id); d.error ? p.fail(Error(`${p.method}: ${JSON.stringify(d.error)}`)) : p.ok(d); }
    else if (d.method) listeners.forEach(f => f(d));
  };
  const call = (method, params = {}, ms = timeoutMs) => new Promise((ok, fail) => {
    if (ws.readyState !== WebSocket.OPEN) return fail(Error('DevTools disconnected'));
    const id = ++seq;
    const timer = setTimeout(() => { pending.delete(id); fail(Error(`${method} timed out after ${ms}ms`)); }, ms);
    pending.set(id, { ok, fail, timer, method });
    try { ws.send(JSON.stringify({ id, method, params })); } catch (e) { clearTimeout(timer); pending.delete(id); fail(e); }
  });
  const evaluate = async expression => {
    const r = await call('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.result?.exceptionDetails) throw Error(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text);
    return r.result?.result?.value;
  };
  const key = async (combo, options = {}) => {
    const parts = combo.split('+'), code = parts.pop();
    const modifiers = parts.reduce((m, p) => m | ({ Alt: 1, Control: 2, Meta: 4, Shift: 8 }[p] || 0), 0);
    const punctuation={Slash:['/','?'],Semicolon:[';',':']};
    let k = punctuation[code]?.[modifiers&8?1:0] || (code.startsWith('Key') ? code.slice(3).toLowerCase() : code.startsWith('Digit') ? code.slice(5) : code === 'Space' ? ' ' : code === 'Backslash' ? '\\' : code);
    if (options.uppercase && modifiers & 8) k = k.toUpperCase();
    const params = { code, key: k, modifiers, windowsVirtualKeyCode: VK[code] || (k.length === 1 ? k.toUpperCase().charCodeAt(0) : 0) };
    for (let i = 0; i <= (options.repeats || 0); i++) await call('Input.dispatchKeyEvent', { ...params, type: 'rawKeyDown', autoRepeat: i > 0 });
    await call('Input.dispatchKeyEvent', { ...params, type: 'keyUp' });
  };
  const mouse = async (button = 'left', x = 400, y = 300, clickCount = 1) => {
    const buttons = { left: 1, right: 2, middle: 4, back: 8, forward: 16 }[button];
    if (!buttons) throw Error(`unknown mouse button ${button}`);
    await call('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y, button: 'none', buttons: 0 });
    await call('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button, buttons, clickCount });
    await call('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button, buttons: 0, clickCount });
  };
  const click = async (selector, options = {}) => {
    const point = await evaluate(`(() => { for (const e of document.querySelectorAll(${JSON.stringify(selector)})) { if (e.disabled || e.closest('[inert]')) continue; const b = e.getBoundingClientRect(); const x = b.x+b.width/2, y = b.y+b.height/2; const hit = document.elementFromPoint(x,y); if (b.width && b.height && (hit === e || e.contains(hit))) return {x,y}; } return null; })()`);
    if (!point) throw Error(`control not hittable: ${selector}`);
    await mouse(options.button || 'left', point.x, point.y, options.clickCount || 1);
    return point;
  };
  const burst = async (inputs, intervalMs = 20) => {
    const start = performance.now(), trace = [];
    for (let i = 0; i < inputs.length; i++) {
      const remaining = start + i * intervalMs - performance.now();
      if (remaining > 0) await sleep(remaining);
      const sentMs = performance.now() - start;
      await key(typeof inputs[i] === 'string' ? inputs[i] : inputs[i].combo, typeof inputs[i] === 'string' ? {} : inputs[i]);
      trace.push({ input: inputs[i], sentMs, ackMs: performance.now() - start });
    }
    return trace;
  };
  const shot = async file => { const r = await call('Page.captureScreenshot', { format: 'png' }); writeFileSync(file, Buffer.from(r.result.data, 'base64')); };
  return { call, evaluate, key, mouse, click, burst, shot, on: f => listeners.push(f), close: () => { rejectAll('DevTools closed'); ws.close(); } };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [addr, cmd, arg, x, y] = process.argv.slice(2);
  const c = await connect(addr);
  try {
    if (cmd === 'eval') console.log(JSON.stringify(await c.evaluate(arg), null, 1));
    else if (cmd === 'shot') await c.shot(arg);
    else if (cmd === 'click') await c.click(arg);
    else if (cmd === 'key') await c.key(arg);
    else if (cmd === 'mouse') await c.mouse(arg, x === undefined ? 400 : +x, y === undefined ? 300 : +y);
    else throw Error('usage: cdp.mjs host:port eval|shot|key|click|mouse <arg> [x y]');
  } finally { c.close(); }
}
