import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush, deferred } from "./dom.mjs";

// The launch page's reads (16 §2.3, §2.14): the start's early read, the menu art that comes
// in later, the window's focus, and the live state that says whose game runs.
async function setup(opts = {}) {
  const body = new Element("body"), app = new Element("div"), main = new Element("div"), screen = new Element("div");
  app.id = "app"; main.id = "main"; screen.id = "screen";
  body.append(app); app.append(main); main.append(screen);
  const listeners = {};
  const S = { screen: "", shell: { envs: 1 }, live: { running: false, errors: 0 }, busy: {}, envSeq: 1, popup: null };
  const calls = { home: 0, bg: 0 };
  const art = { url: "" };
  const homeReads = [], applies = [];
  const api = {
    Home: () => {
      calls.home++;
      const d = deferred();
      homeReads.push(d);
      return d.promise;
    },
    HomeBackground: async () => { calls.bg++; return art.url ? { url: art.url, dim: 0, blur: 0, focus: "" } : null; },
    Setting: async () => "",
    HomeMemory: async () => null,
    HomeApply: async (...args) => { applies.push(args); return { stale: true }; },
  };
  const el = (tag, cls = "", text = "") => new Element(tag, cls, text);
  const q = await loadModules("./home.js", {
    "./feedbacksemantics.js": { progress() {} },
    "./state.js": { S, el, api, T: (s) => s, Tf: (s, ...a) => a.reduce((t, v) => t.replace(/%[ds]/, v), s), errText: String, say() {} },
    "./screens.js": { openScreen: () => screen, page: (name) => el("div", "page " + name), changesPlan() {}, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }) },
    "./packs.js": { launchPack() {} },
    "./environment.js": { withEnvironmentSwitch: async (_, run) => { await run(() => true); return true; } },
    "./motion.js": { places: () => new Map(), slideFrom() {} },
    "./popups.js": { memoryConfirm() {} },
    "./selfupdate.js": { goSelfUpdate() {} },
  }, {
    document: { querySelector: (s) => body.querySelector(s), querySelectorAll: (s) => body.querySelectorAll(s), activeElement: null },
    addEventListener(name, fn) { listeners[name] = fn; }, getComputedStyle: () => ({ fontSize: "96px" }),
    setTimeout() {}, clearTimeout() {},
    ...(opts.globals || {}),
  });
  q.initHome({ go() {}, ...(opts.ui || {}) });
  const page = opts.page || { featured: 0, rows: [{ kind: "plan", envName: "1.6", plan: 1, name: "Main", current: true, mods: 5, level: "ok", env: "1.6  1.6.4871", page: "plan" }] };
  // answer the oldest Home read still waiting
  const answer = async (p = page) => { homeReads.shift().resolve(structuredClone(p)); await flush(); await flush(); };
  return { q, S, body, screen, calls, art, listeners, answer, homeReads, applies };
}

test("menu art that comes in before the page's first read is drawn (confirmed 2026-11-14)", async () => {
  const h = await setup();
  h.q.readEarly(); // at start, before the shell is in: the art is not prepared yet
  await flush();
  h.art.url = "/bg?k=core";
  h.q.backgroundChanged(); // the backgrounds event, before the page opened
  await flush();
  h.S.screen = "home";
  const run = h.q.home();
  await h.answer();
  await run;
  const img = h.body.querySelector("#backdrop .img");
  assert.ok(img, "the backdrop is drawn");
  assert.equal(img.style.backgroundImage, 'url("/bg?k=core")');
  assert.equal(h.calls.home, 1, "the early page read is still used");
});

test("a focus while a later read is on its way makes one more read", async () => {
  const h = await setup();
  h.S.screen = "home";
  const first = h.q.home();
  await h.answer();
  await first;
  const second = h.q.home();
  await flush();
  h.listeners.focus();
  h.listeners.focus(); // coalesced with the one before
  await h.answer();
  await flush();
  assert.equal(h.homeReads.length, 1, "one more read follows");
  await h.answer();
  await second;
  assert.equal(h.calls.home, 3);
});

test("the focus a new window gets during the start's read asks for nothing more", async () => {
  const h = await setup();
  h.q.readEarly();
  h.S.screen = "home";
  const run = h.q.home();
  h.listeners.focus();
  await h.answer();
  await run;
  await flush();
  assert.equal(h.calls.home, 1);
  assert.equal(h.homeReads.length, 0);
});

test("an unused start read is forgotten: a later visit reads afresh", async () => {
  const h = await setup();
  h.q.readEarly();
  h.q.forgetEarly();
  h.homeReads.shift(); // the start's read, never drawn
  h.S.screen = "home";
  const run = h.q.home();
  assert.equal(h.calls.home, 2, "the visit asked again");
  await h.answer();
  await run;
});

test("another Game Config's running game shows on the last game line too", async () => {
  const h = await setup();
  h.S.screen = "home";
  const run = h.q.home();
  await h.answer();
  await run;
  h.S.live = { running: false, blocked: "Game Config Other is running", errors: 0 };
  h.q.liveChanged();
  const title = h.screen.querySelector(".hm-go .title");
  assert.ok(title.textContent.includes("Game Config Other is running"), title.textContent);
  const line = h.screen.querySelector(".hm-actline .x");
  assert.ok(line && line.textContent === "Game Config Other is running", line && line.textContent);
});

// 16 §2.3 01 Game: picking another Game Config applies its current list, not the list played
// last anywhere (lists are shared; VM round 2A 2026-11-23: picking 1.7 made r11, played last
// in a 1.6 Game Config, the 1.7 one's list).
test("picking another game applies that Game Config's current list", async () => {
  const row = (envName, plan, name, current, otherEnv) => ({ kind: "plan", envName, plan, name, current, otherEnv, mods: 5, level: "ok", env: envName, page: "plan", gameVersion: envName });
  const h = await setup({ page: { featured: 0, rows: [row("1.6", 1, "r11", true, false), row("1.7", 1, "r11", false, true), row("1.7", 2, "1.7", true, true)] } });
  h.S.screen = "home";
  const run = h.q.home();
  await h.answer();
  await run;
  const pick = h.body.querySelectorAll("button").find((b) => b.dataset.game === "1.7");
  assert.ok(pick, "the 1.7 game is on the page");
  pick.onclick();
  await flush();
  assert.deepEqual(h.applies, [["plan", "1.7", 2, ""]]);
});

// Release 1.1.4 VM round: Launch asked ReadNewLog for a pending last run, but another read had
// parsed the log first, so ReadNewLog read nothing and the page kept "reading the log…".
test("a pending last run that another read parsed is read again once", async () => {
  const asks = [];
  const h = await setup({ ui: { readNewLog: async () => { asks.push(1); return false; } }, globals: { requestAnimationFrame: (fn) => fn() } });
  h.S.screen = "home";
  const pending = { ...structuredClone(rowsPage()), lastRun: "the list  Nov 24 21:05", lastRunPending: true, checkLastRun: true };
  const done = { ...structuredClone(rowsPage()), lastRun: "the list  Nov 24 21:05" };
  const run = h.q.home();
  await h.answer(pending);
  await run;
  await flush(); await flush();
  assert.equal(asks.length, 1);
  assert.equal(h.homeReads.length, 1, "the page is read again");
  await h.answer(done);
  await flush(); await flush();
  assert.doesNotMatch(h.screen.textContent, /reading the log/);
  assert.equal(asks.length, 1, "a settled run asks nothing more");
  assert.equal(h.homeReads.length, 0);
});

function rowsPage() {
  return { featured: 0, rows: [{ kind: "plan", envName: "1.6", plan: 1, name: "Main", current: true, mods: 5, level: "ok", env: "1.6  1.6.4871", page: "plan" }] };
}
