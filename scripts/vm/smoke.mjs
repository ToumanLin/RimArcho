// The GUI half of smoke.ps1: checks the real RimArcho window over DevTools and prints one
// PASS / FAIL line per check, nothing else. Exit code = number of failures.
//
//   node scripts/vm/smoke.mjs <host:port> [screenshot.png]
import { connect } from "./cdp.mjs";

const [addr, shotFile] = process.argv.slice(2);
let fails = 0;
const report = (ok, name, why = "") => { if (!ok) fails++; console.log(`${ok ? "PASS" : "FAIL"} gui: ${name}${!ok && why ? ` (${why})` : ""}`); };

let c;
try { c = await connect(addr); } catch (e) { report(false, "DevTools connection", e.message); process.exit(1); }

// Script errors are collected inside the page: turning on DevTools' Runtime domain changed
// how the window handled fast back / forward key presses, so the test stays out of it.
await c.evaluate(`window.__smoke = []; addEventListener("error", (e) => __smoke.push(String(e.message))); addEventListener("unhandledrejection", (e) => __smoke.push(String(e.reason?.message || e.reason))); 1`);

const page = () => c.evaluate("document.querySelector('#nav .nav-item.on, .nav-item.on')?.dataset.id || ''");
async function until(fn, ms) {
  const end = Date.now() + ms;
  let v;
  while (Date.now() < end) { try { v = await fn(); if (v) return v; } catch { /* page reloading */ } await new Promise((r) => setTimeout(r, 200)); }
  return v;
}
async function onPage(name, id, act, ms = 5000) {
  try { await act(); const got = await until(async () => (await page()) === id && id, ms); report(got === id, name, `on ${(await page()) || "?"}`); }
  catch (e) { report(false, name, e.message); }
}

try {
  const [title, host] = await c.evaluate("[document.title, location.host]");
  report(title === "RimArcho" && host === "wails.localhost", "window loaded", `${title} @ ${host}`);
  const onboarding = await c.evaluate("!!document.querySelector('.page.guide, [data-screen=guide]') || /\u5f00\u59cb\u4f7f\u7528|Get started/.test(document.body.innerText.slice(0, 300))");
  if (onboarding) report(false, "past first-run guide", "the guide is open: no game added yet");

  await onPage("shift+1 opens Start", "home", () => c.key("Shift+Digit1"));
  const photo = await until(() => c.evaluate("document.querySelector('#app').classList.contains('photo')"), 60000);
  report(!!photo, "menu background shown on Start", "plain background after 60 s");
  await onPage("shift+2 opens Can it run", "needs", () => c.key("Shift+Digit2"));
  await onPage("alt+← goes back", "home", () => c.key("Alt+ArrowLeft"));
  await onPage("alt+→ goes forward", "needs", () => c.key("Alt+ArrowRight"));
  await onPage("mouse back button goes back", "home", () => c.mouse("back"));
  await onPage("mouse forward button goes forward", "needs", () => c.mouse("forward"));
  const host2 = await c.evaluate("location.host");
  report(host2 === "wails.localhost", "side buttons stay in the app", host2);
  const errors = await c.evaluate("window.__smoke || []");
  report(errors.length === 0, "no script errors", errors[0]?.split("\n")[0]);
  if (shotFile) { await c.shot(shotFile); console.log(`SHOT ${shotFile}`); }
} catch (e) {
  report(false, "unexpected", e.message);
} finally {
  c.close();
}
process.exit(fails);
