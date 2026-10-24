import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, deferred, flush } from "./dom.mjs";

async function setup() {
  const screen = new Element("div"), requests = [], polls = [], timers = [];
  const S = { screen: "plans", screenSeq: 1, envSeq: 0 };
  const el = (tag, cls, text) => Object.assign(new Element(tag, cls, text), { addEventListener() {}, select() {} });
  const $ = selector => selector === "#screen" ? screen : screen.querySelector(selector);
  const detail = id => ({ id, name: "List " + id, page: "plan:" + id, versions: [], follows: [], facts: "", stale: false });
  const api = {
    PlanManage: async id => ({ sel: detail(id || 1), rows: [1, 2].map(id => ({ ...detail(id), meta: "" })) }),
    PlanLoadCost: id => { const d = deferred(); requests.push({ ...d, id }); return d.promise; },
    PlanLoadCostMeasure: request => { const d = deferred(); polls.push({ ...d, request }); return d.promise; },
    PlanSources: async () => ({ copy: "copy list", versions: [], saves: [] }),
    CheckPlanName: async () => ({ free: "New list" }),
  };
  const button = (label, key, onclick, cls) => Object.assign(el("button", cls, label), { onclick });
  const deps = {
    "./state.js": { S, $, el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {} },
    "./screens.js": { openScreen: () => screen, page: cls => el("div", cls), button, act: button, area() {}, seg() {},
      beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }) },
    "./icons.js": { icon: {} },
    "./popups.js": {},
    "./plans.js": {},
    "./separator.js": { wireSeparator() {} },
  };
  const pm = await loadModules("./planmanage.js", deps, { document: { activeElement: null }, setTimeout: f => timers.push(f), requestAnimationFrame: f => f() });
  pm.initPlanManage({ renderHints() {}, go: async () => {} });
  return { pm, screen, requests, polls, timers, S };
}

const result = (request, label) => ({
  request, loading: false, summary: label, detail: "baseline and mods", mods: "2 mods",
  parts: [{ category: "textures", files: "2 files", disk: "1 MB", ram: "0–1 MB", vram: "1–4 MB" }],
  top: [{ name: "Largest mod", ram: "1 MB", vram: "4 MB" }], notes: ["estimate notes"],
});

for (const rejected of [false, true]) test("changing lists ignores a late " + (rejected ? "error" : "result"), async () => {
  const h = await setup();
  await h.pm.open(1);
  const shimmer = h.screen.querySelector(".skel");
  assert.ok(shimmer, "renders before measurement completes");
  h.requests[0].resolve({ request: 1, loading: true, progress: "read 0 / 2 mods" });
  await flush();
  assert.ok(h.screen.textContent.includes("read 0 / 2 mods"));
  h.timers.shift()(); await flush();
  h.polls[0].resolve({ request: 1, loading: true, progress: "read 1 / 2 mods" });
  await flush();
  assert.equal(h.screen.querySelector(".skel"), shimmer, "progress preserves shimmer");
  h.timers.shift()(); await flush();
  await h.pm.open(2);
  h.requests[1].resolve(result(2, "new range")); await flush();
  if (rejected) h.polls[1].reject(new Error("old error"));
  else h.polls[1].resolve(result(1, "old range"));
  await flush();
  assert.ok(h.screen.textContent.includes("new range"));
  assert.ok(!h.screen.textContent.includes("old range"));
  assert.ok(!h.screen.textContent.includes("old error"));
  assert.equal(h.screen.querySelector(".skel"), null);
  const detail = h.screen.querySelector("details");
  assert.ok(detail);
  assert.equal(detail.open, false, "parts are collapsed initially");
  assert.ok(detail.textContent.includes("textures"));
  assert.ok(detail.textContent.includes("Largest mod"));
  assert.ok(detail.textContent.includes("estimate notes"));
});

test("leaving the page ignores measurements and stops polling", async () => {
  const h = await setup();
  await h.pm.open(1);
  h.S.screen = "home"; h.S.screenSeq++;
  h.requests[0].resolve(result(1, "abandoned range")); await flush();
  assert.ok(!h.screen.textContent.includes("abandoned range"));
  assert.equal(h.timers.length, 0);
});

test("cancelling a new-list form resumes the selected list measurement", async () => {
  const h = await setup();
  await h.pm.open(1);
  await h.pm.newList();
  h.requests[0].resolve(result(1, "hidden range")); await flush();
  h.pm.key({ key: "Escape", preventDefault() {} }); await flush();
  assert.equal(h.requests.length, 2, "returning to the detail restarts its measurement");
  h.requests[1].resolve(result(2, "resumed range")); await flush();
  assert.ok(h.screen.textContent.includes("resumed range"));
  assert.equal(h.screen.querySelector(".skel"), null);
});
