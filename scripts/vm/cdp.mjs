// Drive RimArcho's real WebView2 window over the DevTools protocol (scripts/vm/README.md).
//
//   node scripts/vm/cdp.mjs <host:port> eval "<js expression>"   # prints the JSON result
//   node scripts/vm/cdp.mjs <host:port> shot out.png               # screenshot of the page
//   node scripts/vm/cdp.mjs <host:port> key "Shift+Digit4"         # a key press (code names)
//
// The page's own functions are reachable from eval, e.g. clicking by selector:
//   eval "document.querySelector('.hm-go').click(), 1"
// Needs Node 22+ (global WebSocket). Nothing to install.
import { writeFileSync } from "node:fs";

const [addr, cmd, arg] = process.argv.slice(2);
if (!addr || !cmd) { console.error("usage: cdp.mjs host:port eval|shot|key <arg>"); process.exit(2); }

const targets = await (await fetch(`http://${addr}/json/list`)).json();
const page = targets.find((t) => t.type === "page" && !t.url.startsWith("devtools:"));
if (!page) { console.error("no page target", targets); process.exit(1); }
// The guest reports its own 127.0.0.1 address; reach it through the forwarded port instead.
const wsURL = page.webSocketDebuggerUrl.replace(/ws:\/\/[^/]+/, `ws://${addr}`);
const ws = new WebSocket(wsURL);
await new Promise((ok, fail) => { ws.onopen = ok; ws.onerror = fail; });
let seq = 0; const waiting = new Map();
ws.onmessage = (m) => { const d = JSON.parse(m.data); if (waiting.has(d.id)) { waiting.get(d.id)(d); waiting.delete(d.id); } };
const call = (method, params = {}) => new Promise((ok) => { const id = ++seq; waiting.set(id, ok); ws.send(JSON.stringify({ id, method, params })); });

try {
  if (cmd === "eval") {
    const r = await call("Runtime.evaluate", { expression: arg, awaitPromise: true, returnByValue: true });
    if (r.result?.exceptionDetails) { console.error(JSON.stringify(r.result.exceptionDetails, null, 1)); process.exitCode = 1; }
    else console.log(JSON.stringify(r.result?.result?.value, null, 1));
  } else if (cmd === "shot") {
    const r = await call("Page.captureScreenshot", { format: "png" });
    writeFileSync(arg || "page.png", Buffer.from(r.result.data, "base64"));
    console.log(arg || "page.png");
  } else if (cmd === "key") {
    const parts = arg.split("+"), code = parts.pop();
    const mods = { Alt: 1, Control: 2, Meta: 4, Shift: 8 };
    const modifiers = parts.reduce((m, p) => m | (mods[p] || 0), 0);
    const key = code.startsWith("Key") ? code.slice(3).toLowerCase() : code.startsWith("Digit") ? code.slice(5) : code;
    for (const type of ["keyDown", "keyUp"]) await call("Input.dispatchKeyEvent", { type, code, key, modifiers, windowsVirtualKeyCode: key.length === 1 ? key.toUpperCase().charCodeAt(0) : 0 });
    console.log("ok");
  } else { console.error("unknown command " + cmd); process.exitCode = 2; }
} finally { ws.close(); }
