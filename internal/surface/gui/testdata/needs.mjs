import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush } from "./dom.mjs";

// Can it run (16 §2.4, §5): a question asked in place belongs to its item. When a refresh
// resolves the item, enter must not run the action whose question is no longer shown.
test("enter after the asked item vanished runs nothing", async () => {
  class Node extends Element {
    prepend(n) { this.insertBefore(n, this.firstChild); }
    checkVisibility() { return true; }
    constructor(...args) { super(...args); this.style.removeProperty = (k) => { delete this.style[k]; }; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div"), actions = [];
  const remove = { kind: "remove-file", label: "delete", key: "D", confirm: true, warn: "delete it?" };
  let items = [{ id: "stale-dll", section: 0, status: "error", title: "an old DLL", counted: true, actions: [remove] }];
  const S = { screen: "needs", screenSeq: 1, busy: {}, board: { env: "game-a" }, live: null };
  const api = { Needs: async () => ({ items, memory: { estimated: true } }), DoAction: async (...args) => { actions.push(args); return "done"; } };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, "")), el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon: () => "", bowMark: () => "" },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": {},
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows() {} },
    "./api.js": { on() {} },
    "./separator.js": { wireSeparator() {} },
    "./needtext.js": { initNeedText() {}, needText: (tag, cls, text) => el(tag, cls, text), needParts: () => el("div"), needEvidence: () => el("div"), evidenceMark: () => el("div") },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document: { activeElement: null, dispatchEvent() {}, querySelectorAll: s => screen.querySelectorAll(s) }, URL, CSS: { escape: s => s }, requestAnimationFrame: fn => fn() });
  screens.initScreens({ reload: async () => {}, renderHints() {}, renderWhere() {}, edited: async () => null });
  await screens.needs(false);
  screen.querySelectorAll("button").find(b => b.textContent.startsWith("delete")).onclick(event(""));
  await flush();
  assert.ok(screen.querySelector(".need-confirm"), "the question is asked in the item");
  // a background refresh resolves the item: its question goes with it
  items = [];
  await screens.needs(true);
  assert.equal(screen.querySelector(".need-confirm"), null);
  screens.needsKey(event("Enter", { target: screen }));
  await flush();
  assert.deepEqual(actions, []);
});

// 16 §8.9: the files' check is an icon with its words and time in the tooltip; check again
// is off and reads "up to date" while the shown check is current, and is busy and off while
// a check runs, whoever started it; the start event changes only the page head (no read).
test("the check's state is an icon and check again is spare while the check is current", async () => {
  class Node extends Element {
    prepend(n) { this.insertBefore(n, this.firstChild); }
    checkVisibility() { return true; }
    constructor(...args) { super(...args); this.style.removeProperty = (k) => { delete this.style[k]; }; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div");
  let reads = 0, stale = true;
  const S = { screen: "needs", screenSeq: 1, busy: {}, board: { env: "game-a" }, live: null };
  const api = { Needs: async () => { reads++; return { items: [], stale, checked: "last checked 21:04", memory: { estimated: true } }; } };
  const query = sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, ""));
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: query, el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon: () => "", bowMark: () => "" },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": {},
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows() {} },
    "./api.js": { on() {} },
    "./separator.js": { wireSeparator() {} },
    "./needtext.js": { initNeedText() {}, needText: (tag, cls, text) => el(tag, cls, text), needParts: () => el("div"), needEvidence: () => el("div"), evidenceMark: () => el("div") },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document: { activeElement: null, dispatchEvent() {}, querySelector: s => screen.querySelector(s), querySelectorAll: s => screen.querySelectorAll(s) }, URL, CSS: { escape: s => s }, requestAnimationFrame: fn => fn() });
  screens.initScreens({ reload: async () => {}, renderHints() {}, renderWhere() {}, edited: async () => null });
  const again = () => screen.querySelector(".analyze-again");
  const state = () => screen.querySelector(".check-state");
  // stale (a check failed or left it so): the icon says so with the time, the button works
  await screens.needs(false);
  assert.ok(state().classList.contains("stale"));
  assert.equal(state().getAttribute("aria-label"), "the files changed since the last check, last checked 21:04");
  assert.ok(!screen.querySelectorAll(".state-note").some(n => n.textContent.includes("files changed")), "no words on the page");
  assert.equal(again().disabled, false);
  assert.ok(again().textContent.startsWith("check again"));
  // a check starts (Go started it, or a click): checking, busy and off, no read of the page
  S.busy.analyze = true;
  screens.analyzeEvent({ line: "reading Defs" });
  assert.equal(again().disabled, true);
  assert.equal(again().getAttribute("aria-busy"), "true");
  assert.ok(state().classList.contains("checking"));
  assert.equal(reads, 1, "a start reads nothing");
  // done: the page reads once; the check is current, so the button is off and says so
  S.busy.analyze = false;
  stale = false;
  screens.analyzeEvent({ done: true, ok: true });
  await flush();
  assert.equal(reads, 2);
  assert.ok(state().classList.contains("current"));
  assert.equal(again().disabled, true);
  assert.equal(again().getAttribute("aria-busy"), null);
  assert.ok(again().textContent.startsWith("up to date"));
});
