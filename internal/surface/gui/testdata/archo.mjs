import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Archo's figure (16 §7.1) with a fake clock: loops only while animated (never a still), each
// loop a copy with its own play count, the stills under still or reduced motion, the rotation,
// the off setting and the tips.
// Element with the tree getters archoSpot uses
class Node extends Element {
  get parentElement() { return this.parent || null; }
  get nextSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) + 1] || null : null; }
  get previousElementSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) - 1] || null : null; }
}

// a stand-in animated WebP: RIFF, VP8X, ANIM (loop count 0 = for ever), then a chunk naming the file
function webp(url) {
  const name = new TextEncoder().encode(url), pad = name.length & 1;
  const b = new Uint8Array(12 + 18 + 14 + 8 + name.length + pad), v = new DataView(b.buffer);
  const tag = (p, s) => { for (let i = 0; i < 4; i++) b[p + i] = s.charCodeAt(i); };
  tag(0, "RIFF"); v.setUint32(4, b.length - 8, true); tag(8, "WEBP");
  tag(12, "VP8X"); v.setUint32(16, 10, true);
  tag(30, "ANIM"); v.setUint32(34, 6, true);
  tag(44, "XURL"); v.setUint32(48, name.length, true); b.set(name, 52);
  return b;
}
const plays = (b) => new DataView(b.buffer, b.byteOffset).getUint16(42, true);

async function setup({ mode = "live", tips = [], reduce = false } = {}) {
  let now = 0, serial = 0, blobs = 0;
  const timers = new Map();
  const store = new Map(mode ? [["ui.archo.motion", mode]] : []);
  const live = new Map(); // object URL -> { expr, plays }
  const advance = async (ms) => {
    const until = now + ms;
    for (;;) {
      const due = [...timers].filter(([, t]) => t.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      timers.delete(due[0]); now = due[1].at; due[1].fn(); await flush();
    }
    now = until;
  };
  class Blob { constructor(parts) { this.bytes = parts[0]; } }
  const archo = await loadModules("./archo.js", { "./state.js": { api: { Tips: async () => tips } } }, {
    document: { createElement: (tag) => new Node(tag), documentElement: { dataset: reduce ? { motion: "reduced" } : {} }, body: null },
    getComputedStyle: () => ({ overflowX: "visible", overflowY: "visible", opacity: "1" }),
    localStorage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, String(v)) },
    setTimeout: (fn, ms = 0) => { timers.set(++serial, { fn, at: now + ms }); return serial; },
    clearTimeout: (id) => timers.delete(id), setInterval: () => 0, clearInterval() {},
    requestAnimationFrame: (fn) => fn(),
    fetch: async (url) => ({ ok: !url.includes("missing"), status: url.includes("missing") ? 404 : 200, arrayBuffer: async () => webp(url).buffer }),
    Blob,
    URL: {
      createObjectURL: (b) => {
        const name = new TextDecoder().decode(b.bytes.subarray(52, 52 + new DataView(b.bytes.buffer).getUint32(48, true)));
        const u = `blob:${++blobs}`;
        live.set(u, { expr: name.match(/anim\/(\w+)\.webp$/)[1], plays: plays(b.bytes) });
        return u;
      },
      revokeObjectURL: (u) => live.delete(u),
    },
    Promise,
  });
  // settle lets fetch, the copy and decode resolve
  const settle = async () => { for (let i = 0; i < 8; i++) await flush(); };
  const shown = (slot) => slot.children.map((i) => live.get(i.src) || i.src);
  return { archo, advance, settle, store, live, shown, pending: () => timers.size };
}

test("a wait shows nothing for 0.21 s, then the expression's loop, never a still", async () => {
  const { archo, advance, settle, shown, live } = await setup();
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await advance(209);
  assert.equal(slot.children.length, 0);
  await advance(1); await settle();
  assert.equal(slot.children.length, 1);
  assert.ok(slot.classList.contains("on"));
  assert.deepEqual(shown(slot), [{ expr: w.expr, plays: 0 }]); // a wait loops for ever
  w.end();
  assert.ok(!slot.classList.contains("on"));
  await advance(150);
  assert.equal(slot.children.length, 0);
  assert.equal(live.size, 0, "the copy is revoked with its image");
});

test("still and reduced motion show only the still", async () => {
  for (const opts of [{ mode: "still" }, { reduce: true }]) {
    const { archo, advance, settle } = await setup(opts);
    const slot = new Element("div");
    archo.archoWait(slot);
    await advance(210); await settle();
    assert.match(slot.children[0].src, /\/archo\/archo-s\/stills\/\w+@360\.webp$/);
  }
});

test("a figure at rest gets a copy that plays three times; nothing replaces it, a new face fades in over it", async () => {
  const { archo, advance, settle, shown, live } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  assert.deepEqual(shown(slot), [{ expr: "wink", plays: 3 }]);
  const first = slot.children[0];
  await advance(60000); await settle();
  assert.deepEqual(slot.children, [first], "the loop's own end is the rest: no still, no swap");
  f.set("salute");
  await settle();
  assert.deepEqual(shown(slot), [{ expr: "wink", plays: 3 }, { expr: "salute", plays: 3 }]);
  assert.ok(slot.children[1].classList.contains("archo-in"));
  await advance(200);
  assert.deepEqual(shown(slot), [{ expr: "salute", plays: 3 }]);
  assert.ok(!slot.children[0].classList.contains("archo-in"), "a redraw that moves the slot does not fade again");
  assert.equal(live.size, 1, "the old copy is revoked");
  f.end();
});

test("the same face again is a fresh copy, so it plays from the start", async () => {
  const { archo, advance, settle, live } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  const a = slot.children[0].src;
  f.set("salute"); await settle(); await advance(200);
  f.set("wink"); await settle(); await advance(200);
  assert.equal(slot.children.length, 1);
  assert.notEqual(slot.children[0].src, a);
  assert.deepEqual(live.get(slot.children[0].src), { expr: "wink", plays: 3 });
  f.end();
});

test("a face that changes again before its loop is ready is dropped", async () => {
  const { archo, advance, settle, shown, live } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  f.set("salute"); f.set("cry");
  await settle(); await advance(200);
  assert.deepEqual(shown(slot), [{ expr: "cry", plays: 3 }]);
  assert.equal(live.size, 1);
  f.end();
});

test("a loop that cannot be read falls back to the still", async () => {
  const { archo, advance, settle } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  f.set("missing");
  await settle(); await advance(200);
  assert.equal(slot.children.length, 1);
  assert.match(slot.children[0].src, /stills\/missing@360\.webp$/);
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
  const { archo, advance, settle, shown } = await setup();
  const first = archo.archoWait(new Element("div")).expr, second = archo.archoWait(new Element("div")).expr;
  assert.equal(archo.POOL.indexOf(second), (archo.POOL.indexOf(first) + 1) % 7);
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await advance(400); await settle();
  const before = w.expr;
  await advance(4000); await settle();
  assert.notEqual(w.expr, before);
  await advance(200);
  assert.deepEqual(shown(slot), [{ expr: w.expr, plays: 0 }]);
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

test("setPlays writes the loop count of an animated WebP", async () => {
  const { archo } = await setup();
  const b = webp("/x.webp");
  assert.ok(archo.setPlays(b, 3));
  assert.equal(plays(b), 3);
  assert.ok(!archo.setPlays(new Uint8Array(40), 3), "no ANIM chunk: unchanged");
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

test("a spot stands in its host, plays three loops per state and holds; a state expression never cycles", async () => {
  const { archo, advance, settle, shown, pending } = await setup();
  const col = new Node("div");
  const spot = archo.archoSpot("t1", { size: 176, calm: ["neutral", "sleepy"] });
  spot.stand(col, "think");
  await settle();
  const slot = col.children.at(-1);
  assert.ok(slot.classList.contains("archo-stand"));
  assert.deepEqual(shown(slot), [{ expr: "think", plays: 3 }]);
  // a redraw places the same figure again: same state, no new loop
  const img = slot.children[0];
  const col2 = new Node("div");
  spot.stand(col2, "think");
  await settle();
  assert.equal(col2.children.at(-1), slot);
  assert.deepEqual(slot.children, [img]);
  spot.set("thumbsup");
  await settle(); await advance(200);
  assert.deepEqual(shown(slot), [{ expr: "thumbsup", plays: 3 }]);
  await advance(120000); // a state, not calm: nothing changes, nothing is left running
  assert.deepEqual(shown(slot), [{ expr: "thumbsup", plays: 3 }]);
  assert.equal(pending(), 0);
  spot.end();
  assert.equal(col2.children.length, 0);
});

test("a peek hangs from the element before its card; in a calm state the calm faces take turns every 45 s", async () => {
  const { archo, advance, settle, shown } = await setup();
  const page = new Node("div"), label = new Node("div"), card = new Node("section");
  page.append(label, card);
  const spot = archo.archoSpot("t2", { size: 140, calm: ["neutral", "ponder", "wink", "smirk"] });
  spot.peek(card, "neutral", true);
  const slot = label.children.at(-1);
  assert.ok(slot.classList.contains("archo-peek") && label.classList.contains("archo-anchor"));
  await advance(44999);
  assert.equal(spot.expr, "neutral");
  await advance(1); await settle();
  assert.equal(spot.expr, "ponder");
  // the next face fades in over the held one: two loops for the fade, never a still
  assert.deepEqual(shown(slot), [{ expr: "neutral", plays: 3 }, { expr: "ponder", plays: 3 }]);
  await advance(200);
  assert.deepEqual(shown(slot), [{ expr: "ponder", plays: 3 }]);
  // a redraw in the same calm state keeps the turn
  spot.peek(card, "neutral", true);
  assert.equal(spot.expr, "ponder");
  await advance(45000);
  assert.equal(spot.expr, "wink");
  // a state's face holds
  spot.set("shake");
  await advance(180000);
  assert.equal(spot.expr, "shake");
  spot.end();
});

test("still and reduced motion stop the cycling", async () => {
  for (const opts of [{ mode: "still" }, { reduce: true }]) {
    const { archo, advance, settle } = await setup(opts);
    const col = new Node("div");
    const spot = archo.archoSpot("t4", { calm: ["neutral", "ponder", "wink"] });
    spot.stand(col, "neutral", true);
    await advance(200000); await settle();
    assert.equal(spot.expr, "neutral");
    assert.match(col.children[0].children[0].src, /stills\/neutral@360\.webp$/);
    spot.end();
  }
});

test("off places no spot", async () => {
  const { archo } = await setup({ mode: "off" });
  const col = new Node("div");
  archo.archoSpot("t3").stand(col, "think");
  assert.equal(col.children.length, 0);
});
