import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Archo's figure (16 §7.1) with a fake clock: the stills, their loops, their timing, the
// rotation, the off setting and the tips.
// Element with the tree getters archoSpot uses
class Node extends Element {
  get parentElement() { return this.parent || null; }
  get nextSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) + 1] || null : null; }
  get previousElementSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) - 1] || null : null; }
}

async function setup({ mode = "live", tips = [], reduce = false } = {}) {
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
    document: { createElement: (tag) => new Node(tag), documentElement: { dataset: reduce ? { motion: "reduced" } : {} }, body: null },
    getComputedStyle: () => ({ overflowX: "visible", overflowY: "visible", opacity: "1" }),
    localStorage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, String(v)) },
    setTimeout: (fn, ms = 0) => { timers.set(++serial, { fn, at: now + ms }); return serial; },
    clearTimeout: (id) => timers.delete(id), setInterval: () => 0, clearInterval() {},
    requestAnimationFrame: (fn) => fn(),
    Promise,
  });
  return { archo, advance, store, pending: () => timers.size };
}

test("a wait shows nothing for 0.21 s, then a still, then the expression's loop", async () => {
  const { archo, advance } = await setup();
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await advance(209);
  assert.equal(slot.children.length, 0);
  await advance(1);
  assert.equal(slot.children.length, 1);
  assert.ok(slot.classList.contains("on"));
  assert.equal(slot.children[0].src, `/archo/archo-s/anim/${w.expr}.webp`);
  w.end();
  assert.ok(!slot.classList.contains("on"));
});

test("still and reduced motion show only the still", async () => {
  for (const opts of [{ mode: "still" }, { reduce: true }]) {
    const { archo, advance } = await setup(opts);
    const slot = new Element("div");
    archo.archoWait(slot);
    await advance(210);
    assert.match(slot.children[0].src, /\/archo\/archo-s\/stills\/\w+@360\.webp$/);
  }
});

test("a figure at rest plays its loop three times, then rests on the still; a new expression plays again", async () => {
  const { archo, advance } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await flush(); await flush(); await flush();
  assert.match(slot.children[0].src, /anim\/wink\.webp$/);
  await advance(3 * 2160);
  assert.match(slot.children[0].src, /stills\/wink@360\.webp$/);
  f.set("salute");
  await flush(); await flush(); await flush();
  assert.match(slot.children[0].src, /anim\/salute\.webp$/);
  f.end();
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
  assert.match(slot.children[0].src, /salute/);
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

test("a spot stands in its host, plays three loops per state and rests; idle cycling is off", async () => {
  const { archo, advance, pending } = await setup();
  const col = new Node("div");
  const spot = archo.archoSpot("t1", { size: 176, calm: ["neutral", "sleepy"] });
  spot.stand(col, "think");
  await flush(); await flush(); await flush();
  const slot = col.children.at(-1);
  assert.ok(slot.classList.contains("archo-stand"));
  assert.match(slot.children[0].src, /anim\/think\.webp$/);
  await advance(3 * 2160);
  assert.match(slot.children[0].src, /stills\/think@360\.webp$/);
  // a redraw places the same figure again: same state, no new loop
  const col2 = new Node("div");
  spot.stand(col2, "think");
  assert.equal(col2.children.at(-1), slot);
  assert.match(slot.children[0].src, /stills\/think@360\.webp$/);
  spot.set("thumbsup");
  await flush(); await flush(); await flush();
  assert.match(slot.children[0].src, /anim\/thumbsup\.webp$/);
  await advance(3 * 2160);
  await advance(120000); // no cycling: nothing changes, nothing is left running
  assert.match(slot.children[0].src, /stills\/thumbsup@360\.webp$/);
  assert.equal(pending(), 0);
  spot.end();
  assert.equal(col2.children.length, 0);
});

test("a peek hangs from the element before its card; with cycling a calm face moves on every 45 s", async () => {
  const { archo, advance } = await setup();
  const page = new Node("div"), label = new Node("div"), card = new Node("section");
  page.append(label, card);
  const spot = archo.archoSpot("t2", { size: 140, calm: ["neutral", "ponder"], cycle: true });
  spot.peek(card, "neutral");
  const slot = label.children.at(-1);
  assert.ok(slot.classList.contains("archo-peek") && label.classList.contains("archo-anchor"));
  await advance(45000);
  assert.equal(spot.expr, "ponder");
  spot.set("shake"); // a state's face does not cycle
  await advance(90000);
  assert.equal(spot.expr, "shake");
  spot.end();
});

test("off places no spot", async () => {
  const { archo } = await setup({ mode: "off" });
  const col = new Node("div");
  archo.archoSpot("t3").stand(col, "think");
  assert.equal(col.children.length, 0);
});
