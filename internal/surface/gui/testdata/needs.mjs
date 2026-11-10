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
    "./needtext.js": { initNeedText() {}, needText: (tag, cls, text) => el(tag, cls, text), needParts: () => el("div"), needEvidence: () => el("div") },
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
