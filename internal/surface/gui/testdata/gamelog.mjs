import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, flush } from "./dom.mjs";

// Game log page (16 §2.10): switching between Player.log and Archo Monitor redraws the page
// below its head. page() leaves an empty head out, so the redraw must not keep "the first
// child" as the head: that kept the old subbar under the new one (two file pickers), and an
// empty head left a blank band between the title and the subbar (2026-11-19).
class Node extends Element {
  constructor(...a) { super(...a); this.style.removeProperty = (k) => { delete this.style[k]; }; }
  get nextElementSibling() { const p = this.parent; return p ? p.children[p.children.indexOf(this) + 1] || null : null; }
  getBoundingClientRect() { return { left: 0, right: 100, top: 0, bottom: 28, width: 100, height: 28 }; }
}

async function logScreen(selected) {
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div"), app = el("div");
  const $ = (sel, root) => {
    if (sel === "#screen") return screen;
    if (sel === "#app") return app;
    return (root || screen).querySelector(sel.replace(/#screen /g, ""));
  };
  const S = { screen: null, screenSeq: 1, envSeq: 0, busy: {}, board: { env: "main" }, shell: { plan: { id: "p" } } };
  const page = {
    path: "C:/logs/Player.log", selected, facts: "1.6.4871 rev591  781 lines",
    files: [{ label: "Player.log", when: "Nov 7 10:14", path: "C:/logs/Player.log" }, { label: "ArchoMonitor.log", when: "Nov 7 10:14", of: "C:/logs/Player.log" }],
    rows: [{ cluster: "E1", class: "xml", count: 1, text: "XML error", line: "1" }],
    monitor: { label: "ArchoMonitor.log", title: "Archo Monitor", facts: "10:14–10:14", entries: [{ key: "a", time: "10:14:11", text: "patch error", count: 1 }] },
  };
  const api = {
    LogPage: async () => structuredClone(page), LogCluster: async () => [], LogPattern: async () => null,
    SelectLog: async () => "", Leave: async () => {},
  };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $, el, api, T: (s) => s, Tf: (s) => s, errText: (e) => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon: () => "", bowMark: () => "" },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": { menuOpener() {} },
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows: () => ({ select() {}, destroy() {}, snapshot: () => ({}), update() {} }) },
    "./api.js": { on() {} },
    "./separator.js": { wireSeparator() {} },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document: { activeElement: null }, URL, requestAnimationFrame: (fn) => fn(), setTimeout: (fn) => setImmediate(fn), clearTimeout() {}, matchMedia: () => ({ matches: false }) });
  screens.initScreens({ reload: async () => {}, renderWhere() {}, renderHints() {} });
  await screens.openGameLog("C:/logs/Player.log"); await flush();
  return { screen, logs: () => screen.querySelector(".page.logs") };
}

function assertOneSubbar(w, when) {
  const kids = w.children.map((c) => c.className);
  assert.equal(w.children.filter((c) => c.classList.contains("subbar")).length, 1, when + ": one subbar, got " + JSON.stringify(kids));
  for (const h of w.children.filter((c) => c.classList.contains("page-head")))
    assert.ok(h.children.length, when + ": no empty head (a blank band above the subbar)");
}

for (const selected of [true, false]) {
  test(`switching log tabs keeps one subbar and no empty head (head ${selected ? "empty" : "with an action"})`, async () => {
    const { logs } = await logScreen(selected);
    assertOneSubbar(logs(), "first draw");
    for (let i = 0; i < 4; i++) {
      const tabs = logs().querySelectorAll(".ltabs button");
      assert.equal(tabs.length, 2, "Player.log and Archo Monitor tabs");
      tabs[(i + 1) % 2].onclick();
      await flush();
      assertOneSubbar(logs(), `after tab switch ${i + 1}`);
    }
  });
}
