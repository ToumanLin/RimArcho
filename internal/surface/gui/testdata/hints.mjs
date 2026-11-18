import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

// 16 §2.2, §3.1 rule 4: renderHints trims through hintLine, which keeps ? all keys last.
const { hintLine, HINT_MAX } = await loadModules("./hintline.js", {});
const acts = (n) => Array.from({ length: n }, (_, i) => ({ key: "k" + i, label: "a" + i }));
const help = { key: "?", label: "all keys" };
const keys = (list) => Array.from(list, (a) => a.key); // an array of this realm

test("a short line is kept as it is", () => {
  assert.deepEqual(keys(hintLine([...acts(3), help])), ["k0", "k1", "k2", "?"]);
});

test("a long line keeps its first actions and ends with ?", () => {
  const line = hintLine([...acts(9), help]);
  assert.equal(line.length, HINT_MAX);
  assert.deepEqual(keys(line), ["k0", "k1", "k2", "k3", "k4", "k5", "?"]);
});

test("the editor's esc goes before ?, and the actions give way to it", () => {
  const line = hintLine([...acts(6), help], [{ key: "esc", label: "clear the selection" }]);
  assert.deepEqual(keys(line), ["k0", "k1", "k2", "k3", "k4", "esc", "?"]);
});

test("a form without ? (its field takes ? as text) gets none", () => {
  assert.deepEqual(keys(hintLine(acts(9))), ["k0", "k1", "k2", "k3", "k4", "k5", "k6"]);
});
