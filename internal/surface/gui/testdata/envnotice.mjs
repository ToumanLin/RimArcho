import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush } from "./dom.mjs";

// The shared-game-folder notice on the Game Config page: drawing it is not reading it. It
// stays across redraws until the player presses got it, and only that marks it read.
test("the alias notice is acknowledged by got it, not by drawing", async () => {
  class Node extends Element {
    get childNodes() { return this.children; }
    prepend(n) { this.insertBefore(n, this.firstChild); }
    removeAttribute(k) { delete this.attributes[k]; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div"), document = { activeElement: null };
  const S = { screen: "env", screenSeq: 1, busy: {} };
  let acks = 0, notices = ["these two Game Configs use the same game folder: legacy, main"];
  const api = {
    Environment: async () => ({ envs: [{ name: "main", game: "1.6", path: "C:/game", current: true }], groups: [], notices }),
    AcknowledgeAliasNotice: async () => { acks++; notices = []; },
    Tasks: async () => ({ rows: [] }),
  };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, "")), el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon() {}, bowMark: () => "" },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": {},
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows() {} },
    "./api.js": { on() {} },
    "./separator.js": { wireSeparator() {} },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document, URL, requestAnimationFrame: fn => fn() });
  screens.initScreens({ reload: async () => {} });
  for (let i = 0; i < 2; i++) {
    await screens.environment(); await flush();
    assert.ok(screen.querySelector(".notice"), "the notice is drawn");
    assert.equal(acks, 0, "drawing the notice does not acknowledge it");
  }
  screen.querySelector(".notice").querySelectorAll("button").find(b => b.textContent === "got it").onclick(event(""));
  await flush();
  assert.equal(acks, 1);
  assert.equal(screen.querySelector(".notice"), null, "got it removes the notice");
  await screens.environment(); await flush();
  assert.equal(screen.querySelector(".notice"), null);
});
