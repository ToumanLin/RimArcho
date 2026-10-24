import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event } from "./dom.mjs";

async function setup(items, blocked) {
  const screen = new Element("div"), launches = [];
  const S = { screen: "packs", screenSeq: 1 };
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const $ = (sel) => sel === "#screen" ? screen : screen.querySelector(sel);
  const report = { name: "test", facts: ["2 mods", "created today"], mods: [], dir: "packs/test", check: { items }, blocked, why: blocked ? "missing DLC: Biotech" : "" };
  const button = (text, key, run, cls) => Object.assign(el("button", cls, text), { onclick: run });
  const page = (cls, lead, ...actions) => {
    const w = el("div", cls), head = el("div", "page-head");
    head.append(el("p", "lead", lead), ...actions);
    w.append(head);
    return w;
  };
  const packs = await loadModules("./packs.js", {
    "./state.js": { S, $, el, api: { Pack: async () => report, PackLaunch: async (...args) => { launches.push(args); return {}; } }, T: (s) => s, Tf: (s) => s, errText: (e) => e.message, say() {} },
    "./motion.js": { shimmer() {} },
    "./screens.js": { page, button, act: (text, icon, run) => button(text, "", run), area() {}, openScreen: () => screen, beginPageRead: () => ({ current: () => true, finish() {}, fail(e) { throw e; } }) },
    "./popups.js": { confirm() {}, text() {} },
    "./datatable.js": { createTable: () => ({ load: async () => {}, sorted: (rows) => rows, draw() {}, visible: () => [], key: () => false }) },
    "./tabledefs.js": { packColumns: [], packModColumns: [] },
  });
  packs.initPacks({ renderHints() {}, reload() {} });
  await packs.pack("test");
  return { screen, packs, launches, play: screen.querySelector(".primary") };
}

test("build warnings appear after the header and leave launch enabled", async () => {
  const warning = "built for 1.6.4633 rev1261, this game is 1.6.4871 rev590";
  const info = "installed DLC not used by the pack: Royalty";
  const h = await setup([{ level: "warning", text: warning }, { level: "info", text: info }], false);
  const note = h.screen.querySelector(".warn"), head = h.screen.querySelector(".page-head");
  assert.equal(note.textContent, warning);
  assert.ok(note.title.includes(info));
  assert.ok(!h.screen.textContent.includes(info), "info belongs only in the tooltip");
  assert.equal(note.parent.children.indexOf(note), head.parent.children.indexOf(head) + 1);
  assert.equal(h.play.disabled, false);
  await h.play.onclick();
  assert.equal(h.launches.length, 1);
});

test("missing DLC is red and blocks both the button and keyboard launch", async () => {
  const h = await setup([{ level: "error", text: "missing DLC: Biotech" }], true);
  assert.equal(h.screen.querySelector(".err").textContent, "missing DLC: Biotech");
  assert.equal(h.play.disabled, true);
  assert.equal(h.play.title, "missing DLC: Biotech");
  await h.play.onclick();
  await h.packs.packsKey(event("L"));
  assert.equal(h.launches.length, 0);
});

test("an informational report adds no visible comparison line", async () => {
  const h = await setup([{ level: "info", text: "build not recorded" }], false);
  assert.equal(h.screen.querySelector(".warn"), null);
  assert.equal(h.screen.querySelector(".err"), null);
  assert.equal(h.screen.querySelector(".lead").title, "build not recorded");
  assert.equal(h.play.disabled, false);
});
