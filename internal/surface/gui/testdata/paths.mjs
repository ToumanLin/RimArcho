import assert from "node:assert/strict";
import { test } from "node:test";
import { Element, loadModules, event, flush } from "./dom.mjs";

async function setup() {
  const copied = [], browser = [], timers = new Map();
  let native = true, serial = 0;
  const document = { activeElement: null };
  const el = (tag, cls, text) => new Element(tag, cls, text);
  const fields = await loadModules("./fields.js", {
    "./state.js": { el, esc: s => s, api: {}, T: s => s === "copied" ? "已复制" : s, errText: e => e.message, say() {} },
    "/wails/runtime.js": { Clipboard: { SetText: async text => { if (!native) throw Error("no native clipboard"); copied.push(text); } } },
  }, { document, navigator: { clipboard: { writeText: async text => browser.push(text) } },
    setTimeout: fn => { timers.set(++serial, fn); return serial; }, clearTimeout: id => timers.delete(id) });
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

test("fallback recognises rooted paths and complete URLs without treating keys or IDs as paths", async () => {
  const h = await setup();
  for (const path of ["C:\\Games\\RimWorld", "\\\\server\\share\\mods", "/home/player/mods", "https://example.invalid/a/b?x=1&y=2#section"]) {
    assert.equal(h.pathContent(h.el("div"), path).querySelector(".path-copy").dataset.path, path);
  }
  assert.equal(h.pathContent(h.el("div"), "backup written to C:\\data\\archo.db").querySelector(".path-copy").dataset.path, "C:\\data\\archo.db");
  // A Markdown link's ")" and a sentence's full stop are not part of the address; a balanced
  // pair inside it is (VM round 2026-11-05: release notes copied ".../CHANGELOG.md)").
  for (const [text, url] of [["[notes](https://github.com/a/b/blob/v1/CHANGELOG.md)", "https://github.com/a/b/blob/v1/CHANGELOG.md"],
    ["see https://example.invalid/x.", "https://example.invalid/x"],
    ["https://en.wikipedia.org/wiki/A_(b)", "https://en.wikipedia.org/wiki/A_(b)"]]) {
    assert.equal(h.pathContent(h.el("div"), text).querySelector(".path-copy").dataset.path, url, text);
  }
  for (const text of ["ctrl+/", "↑↓ / enter", "1.6 / 1.7", "ludeon.rimworld", "Mods/", "from github.com/a/b", "moving mods"]) {
    assert.equal(h.pathContent(h.el("div"), text).querySelector(".path-copy"), null, text);
  }
});

test("address text has no copy action; its button copies the full payload and resets feedback", async () => {
  const h = await setup(), path = "D:\\a very long path\\mod";
  const node = h.copyablePath(h.el("span", "", "D:\\…\\mod"), path);
  h.root.append(node);
  assert.equal(node.attributes.role, undefined);
  assert.equal(node.title, path);
  node.dispatchEvent(event("", { type: "click" }));
  node.dispatchEvent(event("Enter", { type: "keydown" }));
  await flush(); assert.deepEqual(h.copied, []);
  const button = node.querySelector(".address-copy");
  assert.equal(button.tagName, "BUTTON"); assert.equal(button.type, "button");
  assert.equal(button.title, "copy");
  for (let i = 0; i < 3; i++) {
    let prevented = false, stopped = false;
    button.dispatchEvent(event("", { type: "click", preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } }));
    await flush();
    assert.ok(prevented && stopped);
    assert.equal(button.dataset.copied, "true");
    assert.equal(button.querySelector(".address-status").textContent, "已复制");
    assert.equal(h.timers.size, 1);
  }
  assert.deepEqual(h.copied, [path, path, path]);
  for (const key of ["Escape", "ArrowDown", "PageDown", "Tab"]) {
    button.dispatchEvent(event(key, { type: "keydown", preventDefault() { assert.fail(key); }, stopPropagation() { assert.fail(key); } }));
  }
  for (const fn of h.timers.values()) fn();
  assert.equal(button.dataset.copied, undefined); assert.equal(button.title, "copy");
  h.noNative(); await h.copy(path, button); assert.deepEqual(h.browser, [path]);
});

test("detail identifiers copy only from their trailing icon, like addresses", async () => {
  const h = await setup();
  const box = h.fieldList([
    { kind: "id", label: "Archo ID", value: "#4k2p" },
    { kind: "package", label: "package ID", value: "ceteam.combatextended" },
    { kind: "workshop", label: "Workshop", value: "2890901044" },
    { kind: "alias", label: "alias", value: "CE" },
  ]);
  const [id, pkg, ws, alias] = box.querySelectorAll(".frow");
  for (const [row, value] of [[id, "#4k2p"], [pkg, "ceteam.combatextended"], [ws, "2890901044"]]) {
    const node = row.querySelector(".path-copy");
    assert.equal(node.dataset.path, value);
    assert.equal(node.attributes.role, undefined);
    node.dispatchEvent(event("", { type: "click" }));
    node.dispatchEvent(event("Enter", { type: "keydown" }));
    await flush(); assert.deepEqual(h.copied, []);
    node.querySelector(".address-copy").dispatchEvent(event("", { type: "click" }));
    await flush(); assert.deepEqual(h.copied, [value]); h.copied.length = 0;
  }
  assert.equal(alias.querySelector(".address-copy"), null);
});

test("decorating an existing address twice leaves one copy button and retains its display label", async () => {
  const h = await setup();
  const node = h.copyablePath(h.el("span", "", "Defs/file.xml"), "D:/mods/Alpha/Defs/file.xml");
  h.copyablePath(node, "D:/mods/Alpha/Defs/file.xml");
  assert.equal(node.querySelectorAll(".address-copy").length, 1);
  assert.equal(node.querySelector(".address-text").dataset.address, "Defs/file.xml");
});

test("middle ellipsis fits measured width and restores the original when there is space", async () => {
  const { middleEllipsis } = await loadModules("./address.js", {});
  const measure = text => Array.from(text).reduce((n, c) => n + (/[^\x00-\x7f]/.test(c) ? 14 : 7), 0);
  for (const text of ["C:\\Fixture\\Games\\RimWorld\\Mods\\Alpha\\Defs\\items.xml", "\\\\server\\share\\模组\\中文目录\\😀纹理.png", "https://example.invalid/project/data/rules.json?branch=main&lang=zh"]) {
    for (const width of [14, 35, 100, 240, 400]) {
      const shown = middleEllipsis(text, width, measure);
      assert.ok(measure(shown) <= width, shown);
      assert.ok(!shown.includes("\uFFFD"));
      if (shown.includes("…")) {
        const [head, tail] = shown.split("…");
        assert.ok(text.startsWith(head) && text.endsWith(tail));
      }
    }
    assert.equal(middleEllipsis(text, measure(text), measure), text);
  }
  assert.equal(middleEllipsis("short", 1, measure), "");
});
