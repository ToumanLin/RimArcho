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

// The editor's problems button reads "1 problem" for one (it said "1 problems" in the 1.1
// VM round, 16 §4.2); two stay plural.
test("the editor's problems button is singular for one problem", async () => {
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const S = { board: { problems: { severity: "error", items: [{ counted: true, section: 0 }] } } };
  const T = s => "T:" + s, Tf = (s, n) => "Tf:" + s.replace("%d", n);
  const problems = await loadModules("./problems.js", {
    "./state.js": { S, el, T, Tf, say() {}, errText: e => e.message, keyCaps: () => el("span"), factText() {} },
    "./api.js": { api: {} },
    "./icons.js": { statusIcon: () => "" },
    "./needtext.js": { needText() {}, needParts() {}, needEvidence() {} },
    "./popups.js": {},
    "./screens.js": { runNeedAction() {}, needActionWrite() {} },
  }, { document: { addEventListener() {} } });
  assert.equal(problems.chip().querySelector(".t").textContent, "T:1 problem");
  S.board.problems.items.push({ counted: true, section: 0 });
  assert.equal(problems.chip().querySelector(".t").textContent, "Tf:2 problems");
});

// Can it run's head names the list it checks as a pill (16 §2.4): plain when it is the
// current list, amber with the current list in its tooltip when the player opened another
// one here; the pill takes the name's place in the lead, which stays the line's sentence.
test("the head's list pill shows a list opened over the current one", async () => {
  class Node extends Element {
    prepend(n) { this.insertBefore(n, this.firstChild); }
    checkVisibility() { return true; }
    constructor(...args) { super(...args); this.style.removeProperty = (k) => { delete this.style[k]; }; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div");
  const def = { id: 1, name: "default" }, r11 = { id: 2, name: "r11" };
  let items = [], plan = 1;
  const S = { screen: "needs", screenSeq: 1, busy: {}, board: { env: "game-a" }, live: null };
  const api = { Needs: async () => ({ items, plan, memory: { estimated: true } }) };
  const query = sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, ""));
  const Tf = (s, ...a) => { let i = 0; return s.replace(/%[sd]/g, () => String(a[i++])); };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: query, el, api, T: s => s, Tf, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
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
  const chip = () => screen.querySelector(".page-head .list-chip");
  // followed: the window's open list is the current one
  S.shell = { plans: [{ ...def, current: true, open: true }, r11], plan: { ...def, current: true, open: true } };
  await screens.needs(false);
  assert.equal(chip().textContent, "default");
  assert.ok(!chip().classList.contains("other"));
  assert.equal(chip().title, "current list");
  const lead = screen.querySelector(".page-head .lead");
  assert.equal(lead.getAttribute("aria-label"), "default can run: nothing to handle.");
  assert.equal(lead.textContent, "default can run: nothing to handle.", "the pill takes the name's place");
  // opened here while r11 is current: amber, and its tooltip names r11
  S.shell = { plans: [{ ...r11, current: true }, { ...def, open: true }], plan: { ...def, open: true }, current: { ...r11, current: true } };
  await screens.needs(true);
  assert.equal(chip().textContent, "default");
  assert.ok(chip().classList.contains("other"));
  assert.equal(chip().title, "the game is launched with r11; this list is only open here");
  // something to handle: the run summary's list is the same pill
  items = [{ id: "x", section: 0, status: "error", title: "a missing Mod", counted: true, actions: [] }];
  await screens.needs(true);
  assert.ok(screen.querySelector(".lead.run-sum"));
  assert.equal(chip().textContent, "default");
  assert.ok(chip().classList.contains("other"));
  // switched from outside while the window stayed in front (VM 2026-11-23): the read checks
  // r2b, which the sidebar does not have yet; the shell is read again and the pill names it
  const r2b = { id: 3, name: "r2b" };
  items = []; plan = 3;
  S.shell = { plans: [{ ...def, current: true, open: true }], plan: { ...def, current: true, open: true } };
  let reloads = 0;
  screens.initScreens({ reload: async () => { reloads++; S.shell = { plans: [{ ...r2b, current: true, open: true }, def], plan: { ...r2b, current: true, open: true } }; }, renderHints() {}, renderWhere() {}, edited: async () => null });
  await screens.needs(true);
  assert.equal(reloads, 1);
  assert.equal(chip().textContent, "r2b");
  assert.ok(!chip().classList.contains("other"));
  await screens.needs(true);
  assert.equal(reloads, 1, "a shell that knows the list is not read again");
  // the open list became current outside (archo plan use default) while it was open over
  // r2b: same list, but the data's stamp moved on; the shell is read and the pill is plain
  plan = 1; S.board.stamp = "1";
  S.shell = { plans: [{ ...r2b, current: true }, { ...def, open: true }], plan: { ...def, open: true }, current: { ...r2b, current: true } };
  api.Stamp = async () => "2";
  screens.initScreens({ reload: async () => { reloads++; S.board.stamp = "2"; S.shell = { plans: [r2b, { ...def, current: true, open: true }], plan: { ...def, current: true, open: true } }; }, renderHints() {}, renderWhere() {}, edited: async () => null });
  await screens.needs(true);
  assert.equal(reloads, 2);
  assert.equal(chip().textContent, "default");
  assert.ok(!chip().classList.contains("other"));
});
