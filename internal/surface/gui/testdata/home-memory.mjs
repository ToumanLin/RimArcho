import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// The launch page's memory refresh (16 §2.16.1 G19) runs every 30 s, also while the game
// runs. It must not redraw the page: that restarts the running dot's blinks (16 §2.3) and
// costs a frame each time (16 §10).
async function setup({ running = false, severity = "" } = {}) {
  const body = new Element("body"), app = new Element("div"), main = new Element("div"), screen = new Element("div");
  app.id = "app"; main.id = "main"; screen.id = "screen";
  body.append(app); app.append(main); main.append(screen);
  const timers = [];
  const S = { screen: "home", shell: { envs: 1 }, live: { running, errors: 0 }, busy: {}, envSeq: 1, popup: null };
  const memory = { severity };
  const api = {
    Home: async () => ({ featured: 0, rows: [{ kind: "plan", envName: "1.6", plan: 1, name: "Main", current: true, mods: 5, level: "ok", env: "1.6  1.6.4871", page: "plan" }] }),
    HomeBackground: async () => null,
    Setting: async () => "",
    HomeMemory: async () => ({ env: "1.6", plan: 1, token: "t", assessment: { estimated: true, stale: false, ramSeverity: memory.severity } }),
  };
  const el = (tag, cls = "", text = "") => new Element(tag, cls, text);
  const q = await loadModules("./home.js", {
    "./feedbacksemantics.js": { progress() {} },
    "./state.js": { S, el, api, T: (s) => s, Tf: (s, ...a) => a.reduce((t, v) => t.replace(/%[ds]/, v), s), errText: String, say() {} },
    "./screens.js": { openScreen: () => screen, page: (name) => el("div", "page " + name), changesPlan() {}, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }) },
    "./packs.js": { launchPack() {} },
    "./environment.js": { withEnvironmentSwitch() {} },
    "./motion.js": { places: () => new Map(), slideFrom() {} },
    "./popups.js": { memoryConfirm() {} },
  }, {
    document: { querySelector: (s) => body.querySelector(s), querySelectorAll: (s) => body.querySelectorAll(s), activeElement: null },
    addEventListener() {}, getComputedStyle: () => ({ fontSize: "96px" }),
    setTimeout: (fn, ms) => timers.push({ fn, ms }), clearTimeout() {},
  });
  q.initHome({});
  await q.home();
  await flush();
  // the next 30 s refresh
  const tick = async () => {
    const i = timers.findLastIndex((t) => t.ms === 30000);
    assert.ok(i >= 0, "a 30 s refresh is waiting");
    const [t] = timers.splice(i, 1);
    t.fn();
    await flush();
  };
  return { q, S, screen, memory, tick };
}

test("memory refreshes while the game runs leave the page and its running dot alone", async () => {
  const h = await setup({ running: true });
  const pg = h.screen.firstChild, dot = h.screen.querySelector(".hm-go .dot");
  assert.ok(dot, "the running game's dot is shown");
  for (let i = 0; i < 3; i++) await h.tick();
  assert.equal(h.screen.firstChild, pg, "the page is not rebuilt");
  assert.equal(h.screen.querySelector(".hm-go .dot"), dot, "the dot keeps its element, so its blinks do not restart");
});

test("a changed RAM state redraws only step 03; an unchanged one writes nothing", async () => {
  const h = await setup();
  const pg = h.screen.firstChild, hero = h.screen.querySelector(".hm-hero"), go = h.screen.querySelector(".hm-go");
  assert.ok(!go.classList.contains("memory-severe"));
  await h.tick();
  assert.equal(h.screen.querySelector(".hm-go"), go, "the same estimate writes nothing");
  h.memory.severity = "severe";
  await h.tick();
  const now = h.screen.querySelector(".hm-go");
  assert.notEqual(now, go);
  assert.ok(now.classList.contains("memory-severe"));
  assert.equal(h.screen.firstChild, pg);
  assert.equal(h.screen.querySelector(".hm-hero"), hero, "the rest of the page stays");
  await h.tick();
  assert.equal(h.screen.querySelector(".hm-go"), now);
});
