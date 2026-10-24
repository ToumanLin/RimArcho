// Drive RimArcho's real WebView2 window over the DevTools protocol (scripts/vm/README.md).
//
//   node scripts/vm/cdp.mjs <host:port> eval "<js expression>"   # prints the JSON result
//   node scripts/vm/cdp.mjs <host:port> shot out.png               # screenshot of the page
//   node scripts/vm/cdp.mjs <host:port> key "Shift+Digit4"         # a key press (code names)
//   node scripts/vm/cdp.mjs <host:port> mouse back                 # a mouse side button (back|forward)
//
// The page's own functions are reachable from eval, e.g. clicking by selector:
//   eval "document.querySelector('.hm-go').click(), 1"
// Needs Node 22+ (global WebSocket). Nothing to install. smoke.mjs imports connect().
import { writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const VK = { ArrowLeft: 37, ArrowUp: 38, ArrowRight: 39, ArrowDown: 40, Enter: 13, Escape: 27, Tab: 9, Backspace: 8 };

// connect opens the page target; the guest reports its own 127.0.0.1 address, so the
// WebSocket goes through the forwarded port instead.
export async function connect(addr) {
  const targets = await (await fetch(`http://${addr}/json/list`)).json();
  const page = targets.find((t) => t.type === "page" && !t.url.startsWith("devtools:"));
  if (!page) throw new Error("no page target");
  const ws = new WebSocket(page.webSocketDebuggerUrl.replace(/ws:\/\/[^/]+/, `ws://${addr}`));
  await new Promise((ok, fail) => { ws.onopen = ok; ws.onerror = () => fail(new Error("DevTools WebSocket failed")); });
  let seq = 0;
  const waiting = new Map(), listeners = [];
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data);
    if (waiting.has(d.id)) { waiting.get(d.id)(d); waiting.delete(d.id); }
    else if (d.method) listeners.forEach((f) => f(d));
  };
  const call = (method, params = {}) => new Promise((ok) => { const id = ++seq; waiting.set(id, ok); ws.send(JSON.stringify({ id, method, params })); });
  const evaluate = async (expression) => {
    const r = await call("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (r.result?.exceptionDetails) throw new Error(r.result.exceptionDetails.exception?.description || r.result.exceptionDetails.text);
    return r.result?.result?.value;
  };
  const key = async (combo) => {
    const parts = combo.split("+"), code = parts.pop();
    const mods = { Alt: 1, Control: 2, Meta: 4, Shift: 8 };
    const modifiers = parts.reduce((m, p) => m | (mods[p] || 0), 0);
    const k = code.startsWith("Key") ? code.slice(3).toLowerCase() : code.startsWith("Digit") ? code.slice(5) : code;
    const vk = VK[k] || (k.length === 1 ? k.toUpperCase().charCodeAt(0) : 0);
    for (const type of ["rawKeyDown", "keyUp"]) await call("Input.dispatchKeyEvent", { type, code, key: k, modifiers, windowsVirtualKeyCode: vk });
  };
  // A real side-button press and release, as WebView2 would get it from the mouse.
  const mouse = async (button) => {
    const buttons = button === "back" ? 8 : 16;
    await call("Input.dispatchMouseEvent", { type: "mousePressed", x: 400, y: 300, button, buttons, clickCount: 1 });
    await call("Input.dispatchMouseEvent", { type: "mouseReleased", x: 400, y: 300, button, buttons: 0, clickCount: 1 });
  };
  const shot = async (file) => {
    const r = await call("Page.captureScreenshot", { format: "png" });
    writeFileSync(file, Buffer.from(r.result.data, "base64"));
  };
  return { call, evaluate, key, mouse, shot, on: (f) => listeners.push(f), close: () => ws.close() };
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [addr, cmd, arg] = process.argv.slice(2);
  if (!addr || !cmd) { console.error("usage: cdp.mjs host:port eval|shot|key|mouse <arg>"); process.exit(2); }
  const c = await connect(addr);
  try {
    if (cmd === "eval") console.log(JSON.stringify(await c.evaluate(arg), null, 1));
    else if (cmd === "shot") { await c.shot(arg || "page.png"); console.log(arg || "page.png"); }
    else if (cmd === "key") { await c.key(arg); console.log("ok"); }
    else if (cmd === "mouse") { await c.mouse(arg || "back"); console.log("ok"); }
    else { console.error("unknown command " + cmd); process.exitCode = 2; }
  } catch (e) { console.error(e.message); process.exitCode = 1; }
  finally { c.close(); }
}
