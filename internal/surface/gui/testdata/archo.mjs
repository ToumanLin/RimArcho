import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Archo's figure (16 §7.1) with a fake clock: programs of spliced clips while animated (never
// a still), joined only at seams, the idle at rest and its pause, the stills under still or
// reduced motion, the rotation, the off setting and the tips.
// Element with the tree getters archoSpot uses
class Node extends Element {
  get parentElement() { return this.parent || null; }
  get nextSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) + 1] || null : null; }
  get previousElementSibling() { const c = this.parent?.children; return c ? c[c.indexOf(this) - 1] || null : null; }
}

// a stand-in animated WebP: RIFF, VP8X, ANIM (loop count 0 = for ever), then one ANMF chunk
// naming the clip (a real clip has one per frame)
const enc = new TextEncoder(), dec = new TextDecoder();
function chunk(t, body) {
  const b = new Uint8Array(8 + body.length + (body.length & 1)), v = new DataView(b.buffer);
  for (let i = 0; i < 4; i++) b[i] = t.charCodeAt(i);
  v.setUint32(4, body.length, true); b.set(body, 8);
  return b;
}
function webp(url) {
  const name = url.match(/anim\/([\w-]+)\.webp$/)?.[1] || url;
  const parts = [chunk("VP8X", new Uint8Array(10)), chunk("ANIM", new Uint8Array(6)), chunk("ANMF", enc.encode(name))];
  const n = parts.reduce((s, p) => s + p.length, 0), b = new Uint8Array(12 + n), v = new DataView(b.buffer);
  b.set(enc.encode("RIFFxxxxWEBP")); v.setUint32(4, n + 4, true);
  let p = 12;
  for (const x of parts) { b.set(x, p); p += x.length; }
  return b;
}
// read: the clips of a spliced WebP and its loop count
function read(b) {
  const v = new DataView(b.buffer, b.byteOffset, b.byteLength), clips = [];
  let plays = -1;
  for (let p = 12; p + 8 <= b.length;) {
    const t = dec.decode(b.subarray(p, p + 4)), n = v.getUint32(p + 4, true);
    if (t === "ANIM") plays = v.getUint16(p + 12, true);
    if (t === "ANMF") clips.push(dec.decode(b.subarray(p + 8, p + 8 + n)));
    p += 8 + n + (n & 1);
  }
  return { clips: clips.join(" "), plays };
}
const L = 36 * 66, C = 12 * 66; // a loop pass; an in, out or bridge

async function setup({ mode = "live", tips = [], reduce = false } = {}) {
  let now = 0, serial = 0, blobs = 0;
  const timers = new Map();
  const store = new Map(mode ? [["ui.archo.motion", mode]] : []);
  const live = new Map(); // object URL -> { clips, plays }
  const advance = async (ms) => {
    const until = now + ms;
    for (;;) {
      const due = [...timers].filter(([, t]) => t.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      timers.delete(due[0]); now = due[1].at; due[1].fn(); await flush();
    }
    now = until;
  };
  const docListeners = {};
  const document = {
    createElement: (tag) => new Node(tag), documentElement: { dataset: reduce ? { motion: "reduced" } : {} }, body: null, hidden: false,
    addEventListener: (t, fn) => { (docListeners[t] ||= []).push(fn); },
  };
  class Blob {
    constructor(parts) {
      this.bytes = new Uint8Array(parts.reduce((s, p) => s + p.length, 0));
      let o = 0;
      for (const p of parts) { this.bytes.set(p, o); o += p.length; }
    }
  }
  const archo = await loadModules("./archo.js", { "./state.js": { api: { Tips: async () => tips } } }, {
    document,
    getComputedStyle: () => ({ overflowX: "visible", overflowY: "visible", opacity: "1" }),
    localStorage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, String(v)) },
    setTimeout: (fn, ms = 0) => { timers.set(++serial, { fn, at: now + ms }); return serial; },
    clearTimeout: (id) => timers.delete(id), setInterval: () => 0, clearInterval() {},
    requestAnimationFrame: (fn) => fn(),
    performance: { now: () => now },
    fetch: async (url) => ({ ok: !url.includes("missing"), status: url.includes("missing") ? 404 : 200, arrayBuffer: async () => webp(url).buffer }),
    Blob,
    URL: {
      createObjectURL: (b) => { const u = `blob:${++blobs}`; live.set(u, read(b.bytes)); return u; },
      revokeObjectURL: (u) => live.delete(u),
    },
    Promise,
  });
  // settle lets fetch, the splice and decode resolve
  const settle = async () => { for (let i = 0; i < 12; i++) await flush(); };
  // step: advance in 100 ms steps, settling each, as a page's clock and promises interleave
  const step = async (ms) => { for (let left = ms; left > 0; left -= 100) { await advance(Math.min(100, left)); await settle(); } };
  const shown = (slot) => slot.children.map((i) => live.get(i.src) || i.src);
  const hide = (h) => { document.hidden = h; for (const fn of docListeners.visibilitychange || []) fn(); };
  return { archo, advance, settle, step, store, live, shown, hide, pending: () => timers.size };
}

test("a wait shows nothing for 0.21 s, then the expression's loop for ever, never a still", async () => {
  const { archo, advance, settle, shown, live } = await setup();
  const slot = new Element("div");
  const w = archo.archoWait(slot, { rotate: false });
  await advance(209);
  assert.equal(slot.children.length, 0);
  await advance(1); await settle();
  assert.equal(slot.children.length, 1);
  assert.ok(slot.classList.contains("on"));
  assert.deepEqual(shown(slot), [{ clips: w.expr, plays: 0 }]);
  w.end();
  assert.ok(!slot.classList.contains("on"));
  await advance(150);
  assert.equal(slot.children.length, 0);
  assert.equal(live.size, 0, "the program is revoked with its image");
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

test("a figure at rest plays its face three times, goes out to the idle and idles for ever", async () => {
  const { archo, step, settle, shown, live } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  assert.deepEqual(shown(slot), [{ clips: "wink wink wink wink-out bridge", plays: 1 }]);
  await step(3 * L + 2 * C - 100);
  assert.deepEqual(shown(slot), [{ clips: "wink wink wink wink-out bridge", plays: 1 }]);
  await step(200);
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }], "the idle follows where the bridge ends");
  await step(60000);
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }], "she stays alive: the idle loops, nothing freezes");
  assert.equal(live.size, 1, "only the shown program is kept");
  f.end();
});

test("a new face waits for the idle's next seam, then plays in, three loops, out, back to the idle", async () => {
  const { archo, step, settle, shown } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle(); await step(3 * L + 2 * C + 100); // the idle since 8.71 s, shown at about 8.8 s
  await step(1000);
  f.set("salute");
  await step(L - 1300);
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }], "no cut in the middle of the idle");
  await step(400);
  assert.deepEqual(shown(slot), [{ clips: "salute-in salute salute salute salute-out bridge", plays: 1 }]);
  f.end();
});

test("a new face during a loop leaves at the loop's end: out, bridge, in, with no crossfade", async () => {
  const { archo, step, settle, shown } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle(); await step(500);
  f.set("salute");
  await step(L - 700);
  assert.deepEqual(shown(slot), [{ clips: "wink wink wink wink-out bridge", plays: 1 }]);
  await step(300);
  assert.equal(slot.children.length, 1, "one image: the next program replaces it where they meet");
  assert.deepEqual(shown(slot), [{ clips: "wink-out bridge salute-in salute salute salute salute-out bridge", plays: 1 }]);
  f.end();
});

test("a face that changes again before its seam is dropped; a missing clip falls back to the still", async () => {
  const { archo, step, settle, shown, live } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle();
  f.set("salute"); await settle(); f.set("cry");
  await step(L + 100);
  assert.deepEqual(shown(slot), [{ clips: "wink-out bridge cry-in cry cry cry cry-out bridge", plays: 1 }]);
  assert.equal(live.size, 2, "the shown program and the idle that will follow it");
  f.set("missing");
  await step(L);
  assert.match(slot.children[0].src, /stills\/missing@360\.webp$/);
  f.end();
});

test("the idle stops while the window is hidden and starts again from where it stopped", async () => {
  const { archo, step, settle, shown, hide } = await setup();
  const slot = new Element("div");
  const f = archo.archoFigure(slot, "wink");
  await settle(); await step(3 * L + 2 * C + 100);
  hide(true); await settle();
  assert.match(slot.children[0].src, /stills\/idle@360\.webp$/);
  await step(30000);
  assert.match(slot.children[0].src, /stills\/idle@360\.webp$/);
  hide(false); await settle();
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }]);
  f.set("think");
  await step(L + 200);
  assert.deepEqual(shown(slot), [{ clips: "think-in think think think think-out bridge", plays: 1 }]);
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

test("waits take turns through the seven expressions, and a long wait moves on every 4 s through the idle", async () => {
  const { archo, step, shown } = await setup();
  const first = archo.archoWait(new Element("div")).expr, second = archo.archoWait(new Element("div")).expr;
  assert.equal(archo.POOL.indexOf(second), (archo.POOL.indexOf(first) + 1) % 7);
  const slot = new Element("div");
  const w = archo.archoWait(slot);
  await step(400);
  const before = w.expr;
  await step(4000); // the rotation at 4.21 s; the loop's next seam at 0.21 + 2 x 2.38 s
  assert.notEqual(w.expr, before);
  await step(600);
  assert.deepEqual(shown(slot), [{ clips: `${before}-out bridge ${w.expr}-in`, plays: 1 }]);
  await step(3 * C);
  assert.deepEqual(shown(slot), [{ clips: w.expr, plays: 0 }]);
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
  assert.equal(read(b).plays, 3);
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

test("a spot stands in its host, plays each state's face and idles; a state expression never cycles", async () => {
  const { archo, step, settle, shown, pending } = await setup();
  const col = new Node("div");
  const spot = archo.archoSpot("t1", { size: 176, calm: ["neutral", "sleepy"] });
  spot.stand(col, "think");
  await settle();
  const slot = col.children.at(-1);
  assert.ok(slot.classList.contains("archo-stand"));
  assert.deepEqual(shown(slot), [{ clips: "think think think think-out bridge", plays: 1 }]);
  // a redraw places the same figure again: same state, no new program
  const img = slot.children[0];
  const col2 = new Node("div");
  spot.stand(col2, "think");
  await settle();
  assert.equal(col2.children.at(-1), slot);
  assert.deepEqual(slot.children, [img]);
  spot.set("thumbsup");
  await step(L);
  assert.deepEqual(shown(slot), [{ clips: "think-out bridge thumbsup-in thumbsup thumbsup thumbsup thumbsup-out bridge", plays: 1 }]);
  await step(120000); // a state, not calm: she idles, and nothing else is left running
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }]);
  assert.equal(spot.expr, "thumbsup");
  assert.equal(pending(), 0);
  spot.end();
  assert.equal(col2.children.length, 0);
});

test("a peek hangs from the element before its card; in a calm state the calm faces take turns every 45 s", async () => {
  const { archo, step, shown } = await setup();
  const page = new Node("div"), label = new Node("div"), card = new Node("section");
  page.append(label, card);
  const spot = archo.archoSpot("t2", { size: 140, calm: ["neutral", "ponder", "wink", "smirk"] });
  spot.peek(card, "neutral", true);
  const slot = label.children.at(-1);
  assert.ok(slot.classList.contains("archo-peek") && label.classList.contains("archo-anchor"));
  await step(44900);
  assert.equal(spot.expr, "neutral");
  assert.deepEqual(shown(slot), [{ clips: "idle", plays: 0 }]);
  await step(100);
  assert.equal(spot.expr, "ponder");
  await step(L);
  assert.deepEqual(shown(slot), [{ clips: "ponder-in ponder ponder ponder ponder-out bridge", plays: 1 }]);
  // a redraw in the same calm state keeps the turn
  spot.peek(card, "neutral", true);
  assert.equal(spot.expr, "ponder");
  await step(45000);
  assert.equal(spot.expr, "wink");
  // a state's face is not cycled away
  spot.set("shake");
  await step(180000);
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
