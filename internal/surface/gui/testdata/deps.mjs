import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush } from "./dom.mjs";

const install = { label: "install", argv: ["deps", "install", "steamcmd"], task: "install-steamcmd", pending: "downloading…", confirm: "download?" };
const remove = { label: "remove", argv: ["deps", "remove", "steamcmd"], task: "remove-dependency", pending: "removing…", confirm: "remove?" };
const row = (value, editValue, actions) => ({ label: "SteamCMD", value, editValue, edit: "path", file: true, argv: ["config", "set", "steamcmd.path", "{}"], actions });

async function setup(item) {
  const events = {}, calls = [], confirmations = [];
  class Node extends Element {
    get childNodes() { return this.children; }
    prepend(n) { this.insertBefore(n, this.firstChild); }
    removeAttribute(k) { delete this.attributes[k]; }
  }
  const el = (tag, cls, text) => new Node(tag, cls, text);
  const screen = el("div"), document = { activeElement: null };
  let groups = [{ title: "external tools", items: [item] }], programPicks = 0;
  const S = { screen: "settings", screenSeq: 1, busy: {} };
  const api = {
    Settings: async () => groups, Tasks: async () => ({ rows: [] }),
    ApplySetting: async (...args) => { calls.push(args); return ["done"]; },
    PickProgram: async () => { programPicks++; return "C:/tools/custom.exe"; },
    PickFolder: async () => { assert.fail("program editor opened a folder dialog"); },
  };
  const screens = await loadModules("./screens.js", {
    "./state.js": { S, $: sel => sel === "#screen" ? screen : screen.querySelector(sel.replace(/^#screen /, "")), el, api, T: s => s, Tf: s => s, errText: e => e.message, say() {}, cur() {}, slideThumb() {} },
    "./icons.js": { icon: {}, statusIcon() {} },
    "./motion.js": { bump() {}, shimmer: () => el("div") },
    "./popups.js": { confirm: (title, lines, label, submit) => { confirmations.push(lines); submit(); } },
    "./bg.js": { bgSection: async () => el("div") },
    "./virtualrows.js": { virtualRows() {} },
    "./api.js": { on: (name, fn) => { events[name] = fn; } },
    "./separator.js": { wireSeparator() {} },
    "./fulldetail.js": { full() {}, fullKey() {}, fullSection() {} },
  }, { document });
  screens.initScreens({ reload: async () => {} });
  await screens.settings();
  return { screen, calls, confirmations, events, document, programPicks: () => programPicks, setItem: it => { groups = [{ title: "external tools", items: [it] }]; } };
}

test("tool status is separate from its editable path and uses a program picker", async () => {
  const h = await setup(row("0.4.1", "", [install]));
  assert.equal(h.screen.querySelector("input").value, "");
  assert.equal(h.screen.querySelector(".setting-value").textContent, "0.4.1");
  const choose = h.screen.querySelectorAll("button").find(b => b.textContent === "choose…");
  choose.onclick(event("")); await flush();
  assert.equal(h.programPicks(), 1);
  assert.equal(h.calls[0][1], "C:/tools/custom.exe");
  assert.deepEqual(Array.from(h.calls[0][0]), ["config", "set", "steamcmd.path", "{}"]);
});

test("download completion adds removal controls while preserving the unfinished path edit", async () => {
  const h = await setup(row("not installed", "", [install]));
  const input = h.screen.querySelector("input");
  input.value = "C:/unfinished"; h.document.activeElement = input;
  h.setItem(row("installed", "", [{ ...install, label: "update" }, remove]));
  h.events.tasks({ ended: true }); await flush();
  assert.equal(h.screen.querySelector("input"), input);
  assert.equal(input.value, "C:/unfinished");
  assert.equal(h.document.activeElement, input);
  assert.equal(h.screen.querySelector(".setting-value").textContent, "installed");
  const button = h.screen.querySelectorAll("[data-setting-action]").find(b => b.dataset.settingAction === "deps remove steamcmd");
  assert.ok(button);
  button.onclick(event("")); await flush();
  assert.deepEqual(Array.from(h.calls[0][0]), ["deps", "remove", "steamcmd"]);
  assert.equal(h.confirmations[0][0], "remove?");
  h.setItem(row("not installed", "", [install]));
  h.events.tasks({ ended: true }); await flush();
  assert.ok(!h.screen.querySelectorAll("[data-setting-action]").some(b => b.dataset.settingAction === "deps remove steamcmd"));
  assert.equal(h.screen.querySelector("input"), input);
});
