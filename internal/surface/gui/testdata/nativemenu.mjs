import assert from "node:assert/strict";
import { test } from "node:test";
import { keepsNativeMenu, blockNativeMenu } from "../assets/js/nativemenu.js";

const node = (tagName, props = {}, attrs = {}) => ({ nodeType: 1, tagName, getAttribute: (k) => attrs[k] ?? null, ...props });

test("the browser menu stays only on editable text fields", () => {
  assert.equal(keepsNativeMenu(node("DIV")), false);
  assert.equal(keepsNativeMenu(node("BUTTON")), false);
  assert.equal(keepsNativeMenu(node("IMG")), false);
  assert.equal(keepsNativeMenu(node("A")), false);
  assert.equal(keepsNativeMenu(node("INPUT")), true);
  assert.equal(keepsNativeMenu(node("INPUT", {}, { type: "Search" })), true);
  assert.equal(keepsNativeMenu(node("INPUT", {}, { type: "checkbox" })), false);
  assert.equal(keepsNativeMenu(node("INPUT", {}, { type: "range" })), false);
  assert.equal(keepsNativeMenu(node("INPUT", { readOnly: true })), false);
  assert.equal(keepsNativeMenu(node("TEXTAREA", { disabled: true })), false);
  assert.equal(keepsNativeMenu(node("TEXTAREA")), true);
  assert.equal(keepsNativeMenu(node("DIV", { isContentEditable: true })), true);
  assert.equal(keepsNativeMenu({ nodeType: 3 }), false);
  assert.equal(keepsNativeMenu(null), false);
});

test("a right-click outside text fields cancels the browser menu", () => {
  let handler;
  blockNativeMenu({ addEventListener: (type, fn) => { assert.equal(type, "contextmenu"); handler = fn; } });
  const fire = (target) => { let prevented = false; handler({ target, preventDefault: () => { prevented = true; } }); return prevented; };
  assert.equal(fire(node("DIV")), true);
  assert.equal(fire(node("INPUT")), false);
});
