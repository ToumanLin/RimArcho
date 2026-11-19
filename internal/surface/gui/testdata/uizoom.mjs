import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, flush } from "./dom.mjs";

// Interface size from anywhere (16 §2.14): ctrl+wheel and ctrl+'+' / '-' / '0' step ui.zoom
// through API.StepZoom; a wheel the Saves chart already took (cancelled) is left alone.
async function setup() {
  const listeners = {}, steps = [], said = [], shown = [];
  const window = { addEventListener: (type, fn) => { listeners[type] = fn; } };
  const segment = { showValue: (v) => shown.push(v) };
  const levels = [100, 110, 125, 150];
  let pct = 100;
  const api = { StepZoom: async (d) => {
    steps.push(d);
    pct = d === 0 ? 100 : d > 0 ? levels.find((l) => l > pct) ?? 150 : [...levels].reverse().find((l) => l < pct) ?? 100;
    return pct;
  } };
  const q = await loadModules("./uizoom.js", {
    "./api.js": { api },
    "./state.js": { say: (k, t) => said.push(t), Tf: (f, ...a) => f.replace("%s", a[0]), errText: String },
  }, { window, document: { querySelector: (s) => s.includes("ui.zoom") ? segment : null } });
  q.wireZoom();
  const wheel = (deltaY, more = {}) => {
    const e = { ctrlKey: true, deltaY, deltaMode: 0, defaultPrevented: false, prevented: false, preventDefault() { this.prevented = true; }, ...more };
    listeners.wheel(e);
    return e;
  };
  const key = (k, more = {}) => {
    const e = { key: k, code: "", ctrlKey: true, prevented: false, stopped: false, preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; }, ...more };
    listeners.keydown(e);
    return e;
  };
  return { steps, said, shown, wheel, key };
}

test("a mouse notch is one step; small touchpad deltas add up; reversing starts over", async () => {
  const h = await setup();
  assert.ok(h.wheel(-100).prevented, "ctrl+wheel is the window's, not scrolling");
  await flush();
  assert.deepEqual(h.steps, [1]);
  h.wheel(-20); h.wheel(-20);
  await flush();
  assert.deepEqual(h.steps, [1], "40 px is not a step yet");
  h.wheel(30); h.wheel(30); // the other way: the 40 up are dropped
  await flush();
  assert.deepEqual(h.steps, [1, -1]);
  assert.equal(h.said.at(-1), "interface size 100%");
  assert.deepEqual(h.shown, ["110", "100"], "Settings' segment follows");
});

test("a wheel the chart took, and a wheel without ctrl, are left alone", async () => {
  const h = await setup();
  const taken = h.wheel(-100, { defaultPrevented: true });
  const plain = h.wheel(-100, { ctrlKey: false });
  await flush();
  assert.deepEqual(h.steps, []);
  assert.equal(taken.prevented || plain.prevented, false);
});

test("ctrl+'+' '=' '-' '0' and the keypad step; other keys pass", async () => {
  const h = await setup();
  for (const k of ["=", "+", "-", "0"]) assert.ok(h.key(k).stopped, k);
  h.key("0", { code: "Numpad0", shiftKey: true });
  await flush();
  assert.deepEqual(h.steps, [1, 1, -1, 0, 0]);
  const pass = [h.key("k"), h.key("-", { altKey: true }), h.key("-", { ctrlKey: false }), h.key(")", { shiftKey: true })];
  await flush();
  assert.equal(h.steps.length, 5);
  assert.ok(pass.every((e) => !e.prevented && !e.stopped));
});
