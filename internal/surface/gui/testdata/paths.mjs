import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush } from "./dom.mjs";

async function setup() {
  const copied = [], browser = [], timers = [];
  let native = true;
  const document = { activeElement: null };
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const fields = await loadModules("./fields.js", {
    "./state.js": { el, esc: s => s, api: {}, T: s => s === "copied" ? "已复制" : s, errText: e => e.message, say() {} },
    "/wails/runtime.js": { Clipboard: { SetText: async text => { if (!native) throw Error("no native clipboard"); copied.push(text); } } },
  }, { document, navigator: { clipboard: { writeText: async text => browser.push(text) } }, setTimeout: fn => timers.push(fn) });
  const root = el("div");
  return { ...fields, el, root, copied, browser, timers, noNative: () => { native = false; } };
}

test("path markers preserve spaces, relative paths, mixed facts and repeated paths", async () => {
  const h = await setup();
  const file = h.pathContent(h.el("div"), "Mod › Defs/a.xml", [{ text: "Mod › Defs/a.xml", path: "D:/mods/Mod/Defs/a.xml" }]);
  assert.equal(file.querySelector(".path-copy").dataset.path, "D:/mods/Mod/Defs/a.xml");
  const path = "C:\\Games\\two  spaces\\模组";
  const node = h.pathContent(h.el("div"), `Steam has one at ${path}  3 mods  again ${path}`, [path]);
  const paths = node.querySelectorAll(".path-copy");
  assert.equal(paths.length, 2);
  for (const p of paths) { assert.equal(p.dataset.path, path); assert.equal(p.textContent, path); }
  assert.equal(h.pathContent(h.el("div"), "saved to lists/export.json", ["lists/export.json"]).querySelector(".path-copy").dataset.path, "lists/export.json");
});

test("fallback recognizes absolute paths while prose, URLs, keys and IDs stay plain", async () => {
  const h = await setup();
  for (const path of ["C:\\Games\\RimWorld", "\\\\server\\share\\mods", "/home/player/mods"]) {
    assert.equal(h.pathContent(h.el("div"), path).querySelector(".path-copy").dataset.path, path);
  }
  assert.equal(h.pathContent(h.el("div"), "backup written to C:\\data\\archo.db").querySelector(".path-copy").dataset.path, "C:\\data\\archo.db");
  for (const text of ["https://example.com/a/b", "ctrl+/", "↑↓ / enter", "1.6 / 1.7", "ludeon.rimworld", "Mods/", "from github.com/a/b", "moving mods"]) {
    assert.equal(h.pathContent(h.el("div"), text).querySelector(".path-copy"), null, text);
  }
});

test("click, enter and space copy the full value locally and show feedback", async () => {
  const h = await setup(), path = "D:\\a very long path\\mod";
  const node = h.copyablePath(h.el("span", "", "D:\\…\\mod"), path);
  h.root.append(node);
  assert.equal(node.tabIndex, 0);
  assert.equal(node.attributes.role, "button");
  assert.ok(node.title.includes(path) && node.title.includes("click to copy"));
  for (const type of ["click", "keydown", "keydown"]) {
    const key = h.copied.length === 1 ? "Enter" : " ";
    let prevented = false, stopped = false;
    node.dispatchEvent(event(key, { type, preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } }));
    await flush();
    assert.ok(prevented && stopped);
    assert.equal(h.root.querySelectorAll(".copied").length, 1);
    assert.equal(h.root.querySelector(".copied").textContent, "已复制");
  }
  assert.deepEqual(h.copied, [path, path, path]);
  for (const key of ["Escape", "ArrowDown", "PageDown", "Tab"]) {
    node.dispatchEvent(event(key, { type: "keydown", preventDefault() { assert.fail(key); }, stopPropagation() { assert.fail(key); } }));
  }
  for (const fn of h.timers) fn();
  assert.equal(h.root.querySelector(".copied"), null);
  h.noNative();
  await h.copy(path, node);
  assert.deepEqual(h.browser, [path]);
});

test("a click that ends a text selection inside the path keeps the selection instead of copying", async () => {
  const copied = [];
  let sel = null;
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const fields = await loadModules("./fields.js", {
    "./state.js": { el, esc: s => s, api: {}, T: s => s, errText: e => e.message, say() {} },
    "/wails/runtime.js": { Clipboard: { SetText: async text => { copied.push(text); } } },
  }, { document: { activeElement: null }, getSelection: () => sel, setTimeout() {} });
  const root = el("div");
  const node = fields.copyablePath(el("span", "", "D:\mods\A"), "D:\mods\A");
  root.append(node);
  sel = { isCollapsed: false, toString: () => "mods", anchorNode: node };
  node.dispatchEvent(event("", { type: "click", preventDefault() {}, stopPropagation() {} }));
  await flush();
  assert.deepEqual(copied, []);
  sel = { isCollapsed: true, toString: () => "", anchorNode: null };
  node.dispatchEvent(event("", { type: "click", preventDefault() {}, stopPropagation() {} }));
  await flush();
  assert.deepEqual(copied, ["D:\mods\A"]);
});
