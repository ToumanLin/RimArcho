import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Saves page (16 §2.11). The find box of "what this save needs" stays in the page while
// the right card redraws, so the text being typed, its caret and an IME composition
// survive; Check this save / colony and Copy to the saves of … wait while the window
// switches Game Configs (data-env-write), as other writes for the current config do.
class Node extends Element {
  get previousSibling() { const p = this.parent; return p ? p.children[p.children.indexOf(this) - 1] || null : null; }
  get nextSibling() { const p = this.parent; return p ? p.children[p.children.indexOf(this) + 1] || null : null; }
  before(...nodes) { for (const n of nodes) this.parent.insertBefore(n, this); }
  after(...nodes) { let at = this; for (const n of nodes) { at.parent.insertBefore(n, at.nextSibling); at = n; } }
}

async function savesPage() {
  const el = (tag, cls, text) => {
    const n = new Node(tag, cls, text);
    if (text !== undefined && typeof text === "object") { n.text = ""; n.append(text); }
    return n;
  };
  const screen = el("div");
  const handlers = {};
  const row = { ref: "sv:1", name: "Landing" };
  const page = { saves: 1, colonies: [{ key: "k1", name: "Ashfall", saves: [row] }] };
  const detail = { ref: "sv:1", name: "Landing", colony: "Ashfall", colonyKey: "k1", issues: [], mods: [], chosen: "", unseen: "in another config", copyHere: "second" };
  const content = { read: true, needs: { mods: 2, groups: [{ key: "safe", label: "safe to remove", hint: "", mods: [{ name: "Hats", packageId: "tester.hats" }] }] }, limits: [] };
  const api = {
    SavesRead: async (n) => { setTimeout(() => handlers.saves({ request: n, page })); return true; },
    SaveDetail: async () => structuredClone(detail),
    SaveContent: async () => structuredClone(content),
  };
  const globals = {
    document: { activeElement: null, body: null, createElementNS: (_, t) => el(t) },
    CSS: { escape: (s) => s }, requestAnimationFrame: (fn) => fn(), Option: class { constructor(t, v) { this.text = t; this.value = v; } },
    setTimeout: (fn) => setImmediate(fn), clearTimeout() {},
  };
  const button = (label, _k, run, cls = "", envWrite = "") => {
    const b = el("button", "btn " + cls, label);
    if (envWrite) b.dataset.envWrite = envWrite;
    b.onclick = () => run();
    return b;
  };
  const saves = await loadModules("./saves.js", {
    "./state.js": { S: { screen: "saves", screenSeq: 1, busy: {} }, $: (s) => (s === "#screen" ? screen : screen.querySelector(s)), el, api, T: (s) => s, Tf: (s) => s, errText: (e) => e.message, say() {} },
    "./api.js": { on: (name, fn) => { handlers[name] = fn; } },
    "./screens.js": {
      beginPageRead: () => ({ current: () => true, fail(e) { throw e; }, finish() {} }),
      page: (cls) => el("div", "page " + cls), area() {}, act: () => el("button"), button, hintsChanged() {}, openGameLog() {},
    },
    "./separator.js": { wireSeparator() {} },
    "./fields.js": { pathContent: (n) => n },
    "./icons.js": { icon: {} },
    "./motion.js": { shimmer: () => el("div", "shimmer") },
    "./popups.js": {},
    "./credit.js": { getChoices: () => ({}) },
    "./librarylink.js": { openLibraryInstance() {} },
    "./home.js": { applyItem() {}, focusLaunch() {} },
  }, globals);
  return { saves, screen, handlers, globals };
}

test("the find box survives a redraw of the right card", async () => {
  const { saves, screen, handlers, globals } = await savesPage();
  await saves.open();
  for (let i = 0; i < 5; i++) await flush();
  const find = screen.querySelector(".sv-nfind");
  assert.ok(find, "the find box is drawn");
  find.value = "ha";
  globals.document.activeElement = find; // the player is typing in it
  find.oninput();
  // a saves-read event for the selected save redraws the cards
  handlers["saves-read"]({ refs: ["sv:1"] });
  for (let i = 0; i < 5; i++) await flush();
  assert.equal(screen.querySelector(".sv-nfind"), find, "the same input element is kept");
  assert.equal(find.value, "ha");
  assert.ok(screen.querySelector(".sv-right").contains(find));
  assert.equal(screen.querySelectorAll(".sv-nfind").length, 1);
  // typing on after the redraw still filters by what is in the kept box
  const shown = () => screen.querySelector(".sv-needs").textContent.includes("Hats");
  find.value = "ha";
  find.oninput();
  assert.ok(shown(), "a matching Mod is listed");
  find.value = "zzz";
  find.oninput();
  assert.ok(!shown(), "typing after the redraw filters with the kept box's text");
});

test("choosing a save and copying it to the current config wait for a Game Config switch", async () => {
  const { saves, screen } = await savesPage();
  await saves.open();
  for (let i = 0; i < 5; i++) await flush();
  const of = (key) => screen.querySelectorAll("button").find((b) => b.dataset.key === key);
  assert.equal(of("choose-save")?.dataset.envWrite, "save-choice");
  assert.equal(of("choose-colony")?.dataset.envWrite, "save-choice");
  assert.equal(of("copy-here")?.dataset.envWrite, "save-copy-to");
});
