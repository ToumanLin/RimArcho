import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

// A click on a multi-select list's row (16 §3.4, GitHub issue #9): the editor's lists, the
// Mod library and background tasks all go through pickClick.
const { pickClick } = await loadModules("./select.js", {});

// rows a..f; position 2 is a tag group's header standing for rows 3 and 4
const rows = ["a", "b", null, "d", "e", "f"];
const range = (j) => (rows[j] ? [rows[j]] : []);
const one = (j) => (j === 2 ? ["d", "e"] : range(j));
const plain = {}, ctrl = { ctrlKey: true }, shift = { shiftKey: true }, both = { ctrlKey: true, shiftKey: true };
function list(sel = [], cursor = 0, anchor = -1) {
  const l = { sel: new Set(sel), cursor, anchor };
  l.click = (at, e, seed) => { l.anchor = pickClick(l.sel, e, { at, cursor: l.cursor, anchor: l.anchor, one, range, seed }); l.cursor = at; return l; };
  return l;
}
const picked = (l) => [...l.sel].sort();

test("a plain click selects only the clicked row, even with several selected", () => {
  const l = list(["a", "b", "f"], 5).click(3, plain);
  assert.deepEqual(picked(l), [], "nothing left selected: the actions take the cursor row");
  assert.equal(l.anchor, 3);
});

test("ctrl+click adds to the cursor row the actions took, and toggles", () => {
  const l = list([], 0).click(3, ctrl);
  assert.deepEqual(picked(l), ["a", "d"], "the cursor row joins: it was the target");
  l.click(5, ctrl);
  assert.deepEqual(picked(l), ["a", "d", "f"]);
  l.click(3, ctrl);
  assert.deepEqual(picked(l), ["a", "f"], "again: unselected");
  assert.equal(l.anchor, 3);
});

test("ctrl+click on the cursor row itself, or in a list just focused, takes only that row", () => {
  assert.deepEqual(picked(list([], 3).click(3, ctrl)), ["d"]);
  assert.deepEqual(picked(list([], 0).click(3, ctrl, false)), ["d"]);
});

test("ctrl+click on a tag header toggles its rows; a header under the cursor seeds nothing", () => {
  const l = list([], 2).click(2, ctrl);
  assert.deepEqual(picked(l), ["d", "e"]);
  l.click(2, ctrl);
  assert.deepEqual(picked(l), []);
  assert.deepEqual(picked(list([], 2).click(5, ctrl)), ["f"]);
});

test("shift+click takes the range from the anchor and replaces the selection", () => {
  const l = list(["f"], 0).click(1, plain).click(4, shift);
  assert.deepEqual(picked(l), ["b", "d", "e"], "headers are not taken by a range");
  l.click(0, shift);
  assert.deepEqual(picked(l), ["a", "b"], "the anchor stays where the range started");
  assert.equal(l.anchor, 1);
});

test("shift+click without an anchor ranges from the cursor", () => {
  assert.deepEqual(picked(list([], 3).click(5, shift)), ["d", "e", "f"]);
});

test("ctrl+shift+click adds the range to the selection", () => {
  const l = list([], 0).click(0, plain).click(3, ctrl).click(5, both);
  assert.deepEqual(picked(l), ["a", "d", "e", "f"], "the range runs from the ctrl+clicked row");
});
