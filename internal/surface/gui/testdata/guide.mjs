import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, deferred } from "./dom.mjs";

const info = (description = "old", refreshing = true, error = "") => ({
  steamcmd: "fake", games: "D:\\Games", refreshing, error,
  branches: [{ name: "public", description }, { name: "version-1.5", version: "1.5", description }],
});

async function setup() {
  const screen = new Element("div"), S = { screen: "guide" };
  let now = 0, calls = 0, response = info(), switchWay;
  const timers = [], delays = [];
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const guide = await loadModules("./guide.js", {
    "./fields.js": { copyablePath: (n) => n, pathContent: (n) => n },
    "./state.js": { S, $: () => screen, el, api: {
      GuideFind: async () => [], GuideDownloadInfo: async () => { calls++; return await response; },
    }, T: s => s, Tf: s => s, errText: String, say() {} },
    "./screens.js": {
      page: () => el("div"),
      button: (label, _, action) => { const b = el("button", "", label); b.onclick = action; return b; },
      seg: (_, __, action) => { switchWay = action; return el("div"); },
      pickFolder() {}, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }),
    },
    "./exchange.js": { exchange() {}, importFrom() {} },
    "./environment.js": { withEnvironmentSwitch() {} },
    "./api.js": { on() {} },
  }, { Date: { now: () => now }, setTimeout: (fn, ms) => { timers.push({fn,ms}); delays.push(ms); } });
  guide.initGuide({ renderWhere() {} });
  await guide.start(); switchWay(1); await flush();
  return {
    screen, S, timers, delays, guide,
    calls: () => calls, now: () => now, respond: value => { response = value; },
    switchWay: n => switchWay(n),
    async tick() { assert.ok(timers.length); const {fn,ms} = timers.shift(); now += ms; fn(); await flush(); },
  };
}

test("refresh polling backs off to five seconds and stops at two minutes", async () => {
  const h = await setup();
  assert.match(h.screen.textContent, /refreshing branch list…/);
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 40, "polling did not stop"); await h.tick(); }
  assert.deepEqual(h.delays.slice(0, 6), [250, 500, 1000, 2000, 4000, 5000]);
  assert.ok(Math.max(...h.delays) <= 5000);
  assert.equal(h.now(), 120000);
  assert.ok(h.calls() < 30);
  const calls = h.calls(); await flush(); assert.equal(h.calls(), calls);
  assert.equal(h.screen.querySelectorAll(".branch").length, 2);
  h.respond(info("fresh", false));
  h.guide.resume(); await flush();
  assert.match(h.screen.querySelector(".branches").textContent, /fresh/);
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
});

test("refresh completion keeps selected branch and typed directory", async () => {
  const h = await setup();
  h.screen.querySelectorAll(".branch")[1].onclick();
  const dir = h.screen.querySelectorAll("input")[0];
  dir.value = "D:/custom"; dir.oninput();
  h.respond(info("fresh", false)); await h.tick();
  assert.match(h.screen.querySelector(".branches").textContent, /fresh/);
  assert.match(h.screen.querySelector(".branch.on").textContent, /1.5/);
  assert.equal(h.screen.querySelectorAll("input")[0].value, "D:/custom");
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
  assert.equal(h.timers.length, 0);
});

test("failed refresh retains branches and shows its error", async () => {
  const h = await setup();
  h.respond(info("old", false, "offline")); await h.tick();
  assert.equal(h.screen.querySelectorAll(".branch").length, 2);
  assert.match(h.screen.textContent, /offline/);
  assert.doesNotMatch(h.screen.textContent, /refreshing branch list…/);
  assert.equal(h.timers.length, 0);
});

test("leaving the page or switching away stops polling", async () => {
  for (const way of ["page", "tab"]) {
    const h = await setup(); const calls = h.calls();
    if (way === "page") h.S.screen = "home"; else h.switchWay(0);
    await h.tick();
    assert.equal(h.calls(), calls); assert.equal(h.timers.length, 0);
  }
});

test("a response arriving after navigation cannot redraw the page", async () => {
  const h = await setup(); const reply = deferred(); h.respond(reply.promise);
  await h.tick(); h.S.screen = "home";
  const before = h.screen.textContent;
  reply.resolve(info("late", false)); await flush();
  assert.equal(h.screen.textContent, before); assert.equal(h.timers.length, 0);
});
