import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, deferred } from "./dom.mjs";

const info = (description = "old", refreshing = true, error = "") => ({
  steamcmd: "fake", games: "D:\\Games", refreshing, error,
  branches: [{ name: "public", description }, { name: "version-1.5", version: "1.5", description }],
});

async function setup({ folder = { done: true }, space = (path) => ({ path, free: 50e9 }), scan = async () => true, from } = {}) {
  const screen = new Element("div"), S = { screen: "guide", busy: {} }, chosen = [];
  let now = 0, calls = 0, response = info(), switchWay, pathEditor;
  const timers = [], delays = [], gameCalls = [], writes = [], went = [];
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const guide = await loadModules("./guide.js", {
    "./fields.js": { copyablePath: (n) => n, editablePath: (n) => n, pathContent: (n) => n },
    "./state.js": { S, $: () => screen, el, api: {
      GuideFind: async () => [], GuideAdd: async (games) => { writes.push("add"); return games.map((g) => g.name); }, GuideScan: (names) => { writes.push("scan"); return scan(names); }, GuideDownloadInfo: async () => { calls++; return await response; },
      ManagedFolder: async () => folder, FolderSpace: async (path) => space(path), ChooseManagedFolder: async (path) => { chosen.push(path); writes.push("folder"); },
      // as view.GameAt: the version's first name not among the environments or pending
      GuideGame: async (path, pending) => {
        gameCalls.push([path, pending]);
        let name = "1.6-2";
        for (let i = 3; pending.includes(name); i++) name = "1.6-" + i;
        return { name, game: path, version: "1.6.4871", launch: "direct" };
      },
    }, T: s => s, Tf: s => s, errText: String, say() {} },
    "./screens.js": {
      page: () => el("div"),
      button: (label, _, action) => { const b = el("button", "", label); b.onclick = action; return b; },
      seg: (_, __, action) => { switchWay = action; return el("div"); },
      chooseLocalPath: (value, done) => { pathEditor = { value, done }; }, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }),
    },
    "./exchange.js": { exchange() {}, importFrom() {} },
    "./environment.js": { withEnvironmentSwitch: async (_, run) => { await run(); return true; } },
    "./api.js": { on() {} },
  }, { Date: { now: () => now }, setTimeout: (fn, ms) => { timers.push({fn,ms}); delays.push(ms); } });
  guide.initGuide({ renderWhere() {}, go: (id) => went.push(id) });
  await guide.start(from);
  if (folder.done && !from) { switchWay(1); await flush(); }
  return {
    screen, S, timers, delays, guide, chosen, gameCalls, writes, went, choosePath: (value) => pathEditor.done(value),
    calls: () => calls, now: () => now, respond: value => { response = value; },
    switchWay: n => switchWay(n),
    changeDirectory(value) { screen.querySelectorAll("button").find(b => b.textContent === "change…").onclick(); pathEditor.done(value); },
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
  h.changeDirectory("D:/custom");
  h.respond(info("fresh", false)); await h.tick();
  assert.match(h.screen.querySelector(".branches").textContent, /fresh/);
  assert.match(h.screen.querySelector(".branch.on").textContent, /1.5/);
  assert.ok(h.screen.textContent.includes("D:/custom"));
  assert.equal(h.screen.querySelectorAll("input").length, 1, "only the account remains an input");
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

test("a first run without a managed folder starts on that step, which has no way to skip it", async () => {
  const button = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label);
  const h = await setup({ folder: { path: "C:\RimArcho", free: 50e9, done: false }, space: (path) => ({ path, free: 2e9, low: true }) });
  assert.equal(h.screen.querySelectorAll(".st").length, 4);
  assert.match(h.screen.querySelector(".st.cur").textContent, /managed folders/);
  assert.ok(h.screen.textContent.includes("C:\RimArcho"));
  assert.equal(button(h, "not now"), undefined);
  assert.equal(button(h, "back"), undefined);
  // another folder: its free space and the low-space warning; nothing is saved yet
  button(h, "change…").onclick(); h.choosePath("E:\Archo"); await flush();
  assert.ok(h.screen.textContent.includes("E:\Archo"));
  assert.match(h.screen.textContent, /Less than 10 GB/);
  assert.deepEqual(h.chosen, []);
  // confirming saves it and moves on to finding the game; the step stays, done
  button(h, "use this folder").onclick(); await flush();
  assert.deepEqual(h.chosen, ["E:\Archo"]);
  assert.match(h.screen.querySelector(".st.cur").textContent, /find the game/);
  assert.match(h.screen.querySelector(".st.done").textContent, /managed folders/);
  assert.ok(button(h, "back"), "the folder step can be revisited");
});

test("a managed folder already chosen, or adding another game, has no folder step", async () => {
  const h = await setup();
  assert.equal(h.screen.querySelectorAll(".st").length, 3);
  assert.match(h.screen.querySelector(".st.cur").textContent, /find the game/);
});

test("two folders of one version added in one pass get different names", async () => {
  const h = await setup();
  h.switchWay(0); await flush();
  const add = () => h.screen.querySelectorAll("button").find(b => b.textContent === "add this folder");
  add().onclick(); await h.choosePath("D:\\Game2"); await flush();
  add().onclick(); await h.choosePath("D:\\Game3"); await flush();
  assert.deepEqual(h.gameCalls.map(([, pending]) => pending), [[], ["1.6-2"]]);
  assert.deepEqual(h.screen.querySelectorAll(".game .field").map(i => i.value), ["1.6-2", "1.6-3"]);
});

for (const way of ["throws", "refuses"]) test(`a first scan that ${way} gives back the scan flag and ends the step`, async () => {
  const h = await setup({ scan: async () => { if (way === "throws") throw "scan failed"; return false; } });
  h.switchWay(0); await flush();
  h.screen.querySelectorAll("button").find(b => b.textContent === "add this folder").onclick(); await h.choosePath("D:\Game2"); await flush();
  await h.screen.querySelectorAll("button").find(b => b.textContent === "add and continue").onclick(); await flush();
  assert.ok(!h.S.busy.scan, "no scan runs");
  assert.match(h.screen.textContent, way === "throws" ? /scan failed/ : /a scan is already running/);
  if (way === "throws") assert.ok(h.screen.querySelectorAll("button").some(b => b.textContent === "scan again"));
});

test("the preview shows the first run from the folder step and writes nothing", async () => {
  const button = (h, label) => h.screen.querySelectorAll("button").find(b => b.textContent === label);
  const h = await setup({ from: "preview", folder: { path: "C:\\RimArcho", free: 50e9, done: true } });
  // the folder was chosen long ago, yet the preview starts there, as a first run does
  assert.equal(h.screen.querySelectorAll(".st").length, 4);
  assert.match(h.screen.querySelector(".st.cur").textContent, /managed folders/);
  assert.match(h.screen.textContent, /nothing you choose here is saved/);
  button(h, "use this folder").onclick(); await flush();
  assert.match(h.screen.querySelector(".st.cur").textContent, /find the game/);
  h.switchWay(0); await flush();
  button(h, "add this folder").onclick(); await h.choosePath("D:\\Game2"); await flush();
  await button(h, "add and continue").onclick(); await flush();
  // the scan plays out on timers
  assert.match(h.screen.querySelector(".st.cur").textContent, /read the mods/);
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 40, "the made-up scan did not end"); await h.tick(); }
  assert.match(h.screen.textContent, /✓/);
  button(h, "next").onclick(); await flush();
  assert.match(h.screen.querySelector(".st.cur").textContent, /where to start/);
  await button(h, "use this list").onclick(); await flush();
  assert.deepEqual(h.writes, []);
  assert.deepEqual(h.went, ["settings"]);
});

test("the preview's download plays out without SteamCMD", async () => {
  const h = await setup({ from: "preview" });
  h.respond({ games: "", offer: { bytes: 1e6, dir: "C:\\x" } });
  h.screen.querySelectorAll("button").find(b => b.textContent === "use this folder").onclick(); await flush();
  h.switchWay(1); await flush();
  h.screen.querySelectorAll("button").find(b => b.textContent === "download SteamCMD").onclick(); await flush();
  assert.equal(h.screen.querySelectorAll(".branch").length, 3);
  await h.screen.querySelectorAll("button").find(b => b.textContent === "download and add").onclick(); await flush();
  for (let i = 0; h.timers.length; i++) { assert.ok(i < 60, "the made-up download did not end"); await h.tick(); }
  assert.match(h.screen.querySelector(".st.cur").textContent, /read the mods/);
  assert.match(h.screen.textContent, /✓/);
  assert.deepEqual(h.writes, []);
});
