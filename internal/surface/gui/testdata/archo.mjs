import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Archo's figure (16 §7.1) with a fake clock and no WebGL: the stills, their timing, the
// rotation, the off setting and the tips.
async function setup({ mode = "live", tips = [] } = {}) {
  let now = 0, serial = 0;
  const timers = new Map();
  const store = new Map(mode ? [["ui.archo.motion", mode]] : []);
  const advance = async (ms) => {
    const until = now + ms;
    for (;;) {
      const due = [...timers].filter(([, t]) => t.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      timers.delete(due[0]); now = due[1].at; due[1].fn(); await flush();
    }
    now = until;
  };
  const archo = await loadModules("./archo.js", { "./state.js": { api: { Tips: async () => tips } } }, {
    document: { createElement: (tag) => Object.assign(new Element(tag), tag === "canvas" ? { getContext: () => null } : {}) },
    localStorage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, String(v)) },
    setTimeout: (fn, ms = 0) => { timers.set(++serial, { fn, at: now + ms }); return serial; },
    clearTimeout: (id) => timers.delete(id), setInterval: () => 0, clearInterval() {},
    requestAnimationFrame: (fn) => fn(),
    Promise,
  });
  return { archo, advance, store, pending: () => timers.size };
}

test("a wait shows nothing for 0.21 s, then a still; with no hardware WebGL2 it stays a still", async () => {
  const { archo, advance } = await setup();
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await advance(209);
  assert.equal(slot.children.length, 0);
  await advance(1);
  assert.equal(slot.children.length, 1);
  assert.match(slot.children[0].src, /\/archo\/archo-s\/stills\/\w+@360\.webp$/);
  assert.ok(slot.classList.contains("on"));
  assert.ok(!slot.classList.contains("live"));
  w.end();
  assert.ok(!slot.classList.contains("on"));
});

test("a wait that ends within 0.21 s leaves no timers and no image", async () => {
  const { archo, advance, pending } = await setup();
  const slot = new Element("div");
  archo.archoWait(slot).end();
  await advance(5000);
  assert.equal(slot.children.length, 0);
  assert.equal(pending(), 0);
});

test("waits take turns through the seven expressions, and a long wait moves on every 4 s", async () => {
  const { archo, advance, store } = await setup();
  const first = archo.archoWait(new Element("div")).expr, second = archo.archoWait(new Element("div")).expr;
  assert.equal(archo.POOL.indexOf(second), (archo.POOL.indexOf(first) + 1) % 7);
  assert.equal(store.get("archo.turn"), "2");
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await advance(400);
  const before = w.expr;
  await advance(4000);
  assert.notEqual(w.expr, before);
  w.end();
});

test("off shows nothing and keeps the cached setting", async () => {
  const { archo, advance, store } = await setup({ mode: "off" });
  const slot = new Element("div");
  archo.archoWait(slot);
  archo.archoFigure(slot, "wink");
  await advance(1000);
  assert.equal(slot.children.length, 0);
  assert.equal(archo.archoMode(), "off");
  archo.setMode("still");
  assert.equal(store.get("ui.archo.motion"), "still");
  archo.setMode("bogus");
  assert.equal(archo.archoMode(), "live");
});

test("a figure shows its expression at once and changes it on set", async () => {
  const { archo } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  assert.match(slot.children[0].src, /wink@360/);
  f.set("salute");
  await flush();
  assert.match(slot.children[0].src, /salute@360/);
  f.end();
});

test("tips: each line takes the next one of its kind; first-run tips stay in the guide", async () => {
  const tips = [{ text: "a", when: "any" }, { text: "b", when: "any" }, { text: "g", when: "first" }];
  const { archo, advance } = await setup({ tips });
  const line = new Element("p"), guide = new Element("p");
  const stop = archo.tipLine(line, "any", 210);
  await advance(209);
  assert.equal(line.textContent, "");
  await advance(1);
  assert.ok(["a", "b"].includes(line.textContent));
  const shown = line.textContent;
  await advance(8000 + 150);
  assert.notEqual(line.textContent, shown);
  stop();
  archo.tipLine(guide, "first");
  await flush();
  assert.equal(guide.textContent, "g");
});
